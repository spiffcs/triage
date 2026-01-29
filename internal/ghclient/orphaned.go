package ghclient

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
)

// Default values for orphaned contribution detection
const (
	// defaultStaleDays is the number of days without team response for a contribution
	// to be considered orphaned.
	defaultStaleDays = 7

	// defaultConsecutiveAuthorComments is the threshold of unanswered consecutive
	// comments from the author that indicates the contribution needs attention.
	defaultConsecutiveAuthorComments = 2

	// defaultMaxPerRepo limits the number of orphaned contributions fetched per repository.
	defaultMaxPerRepo = 20

	// maxConcurrentOrphanedFetches limits concurrent API requests when fetching orphaned
	// contributions across multiple repositories.
	maxConcurrentOrphanedFetches = 10
)

// orphanedRepoResult holds the result of fetching orphaned contributions for a single repo
type orphanedRepoResult struct {
	repoFullName  string
	contributions []model.OrphanedContribution
	err           error
}

// ListOrphanedContributionsCached finds external PRs/issues needing attention, using cache if available.
// Returns: notifications, fromCache, error
func (c *Client) ListOrphanedContributionsCached(opts model.OrphanedSearchOptions, username string, cache *Cache) ([]model.Item, bool, error) {
	if len(opts.Repos) == 0 {
		return nil, false, nil
	}

	// Try cache first
	if cache != nil {
		if cached, ok := cache.GetOrphanedList(username, opts.Repos, opts.Since); ok {
			return cached, true, nil
		}
	}

	// Fetch fresh data
	orphaned, err := c.ListOrphanedContributions(opts)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if cache != nil {
		if cacheErr := cache.SetOrphanedList(username, orphaned, opts.Repos, opts.Since); cacheErr != nil {
			log.Debug("failed to cache orphaned list", "error", cacheErr)
		}
	}

	return orphaned, false, nil
}

// ListOrphanedContributions finds external PRs/issues needing attention
func (c *Client) ListOrphanedContributions(opts model.OrphanedSearchOptions) ([]model.Item, error) {
	if len(opts.Repos) == 0 {
		return nil, nil
	}

	// Set defaults
	if opts.StaleDays <= 0 {
		opts.StaleDays = defaultStaleDays
	}
	if opts.ConsecutiveAuthorComments <= 0 {
		opts.ConsecutiveAuthorComments = defaultConsecutiveAuthorComments
	}
	if opts.MaxPerRepo <= 0 {
		opts.MaxPerRepo = defaultMaxPerRepo
	}

	sem := make(chan struct{}, maxConcurrentOrphanedFetches)
	results := make(chan orphanedRepoResult, len(opts.Repos))
	var wg sync.WaitGroup

	for _, repoFullName := range opts.Repos {
		parts := strings.Split(repoFullName, "/")
		if len(parts) != 2 {
			log.Debug("invalid repo format, skipping", "repo", repoFullName)
			continue
		}
		owner, repo := parts[0], parts[1]

		wg.Add(1)
		go func(owner, repo, fullName string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			contributions, err := c.fetchOrphanedForRepo(owner, repo, opts)
			results <- orphanedRepoResult{
				repoFullName:  fullName,
				contributions: contributions,
				err:           err,
			}
		}(owner, repo, repoFullName)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var allItems []model.Item
	for result := range results {
		if result.err != nil {
			log.Debug("failed to fetch orphaned contributions", "repo", result.repoFullName, "error", result.err)
			continue
		}
		contributions := result.contributions
		if len(contributions) > opts.MaxPerRepo {
			contributions = contributions[:opts.MaxPerRepo]
		}
		for _, contrib := range contributions {
			item := orphanedToItem(contrib)
			allItems = append(allItems, item)
		}
	}

	return allItems, nil
}

// fetchOrphanedForRepo fetches orphaned contributions for a single repository
func (c *Client) fetchOrphanedForRepo(owner, repo string, opts model.OrphanedSearchOptions) ([]model.OrphanedContribution, error) {
	query := buildOrphanedQuery(owner, repo)
	respData, err := c.executeGraphQL(query, c.token)
	if err != nil {
		return nil, err
	}

	return parseOrphanedResponse(respData, owner, repo, opts)
}

// buildOrphanedQuery builds a GraphQL query to fetch open issues and PRs with comment analysis
func buildOrphanedQuery(owner, repo string) string {
	return fmt.Sprintf(`query {
  repository(owner: "%s", name: "%s") {
    issues(first: 50, states: OPEN, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        number
        title
        createdAt
        updatedAt
        url
        author { login }
        authorAssociation
        assignees(first: 5) {
          nodes { login }
        }
        comments(last: 10) {
          totalCount
          nodes {
            author { login }
            authorAssociation
            createdAt
          }
        }
      }
    }
    pullRequests(first: 50, states: OPEN, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        number
        title
        createdAt
        updatedAt
        url
        author { login }
        authorAssociation
        additions
        deletions
        reviewDecision
        assignees(first: 5) {
          nodes { login }
        }
        comments(last: 10) {
          totalCount
          nodes {
            author { login }
            authorAssociation
            createdAt
          }
        }
        reviews(last: 5) {
          nodes {
            author { login }
            authorAssociation
            submittedAt
            state
          }
        }
      }
    }
  }
}`, owner, repo)
}

// orphanedGraphQLResponse represents the response structure for orphaned queries
type orphanedGraphQLResponse struct {
	Repository struct {
		Issues struct {
			Nodes []orphanedIssueNode `json:"nodes"`
		} `json:"issues"`
		PullRequests struct {
			Nodes []orphanedPRNode `json:"nodes"`
		} `json:"pullRequests"`
	} `json:"repository"`
}

type orphanedIssueNode struct {
	Number            int       `json:"number"`
	Title             string    `json:"title"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	URL               string    `json:"url"`
	Author            *actorRef `json:"author"`
	AuthorAssociation string    `json:"authorAssociation"`
	Assignees         struct {
		Nodes []actorRef `json:"nodes"`
	} `json:"assignees"`
	Comments struct {
		TotalCount int           `json:"totalCount"`
		Nodes      []commentNode `json:"nodes"`
	} `json:"comments"`
}

type orphanedPRNode struct {
	Number            int       `json:"number"`
	Title             string    `json:"title"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	URL               string    `json:"url"`
	Author            *actorRef `json:"author"`
	AuthorAssociation string    `json:"authorAssociation"`
	Additions         int       `json:"additions"`
	Deletions         int       `json:"deletions"`
	ReviewDecision    string    `json:"reviewDecision"`
	Assignees         struct {
		Nodes []actorRef `json:"nodes"`
	} `json:"assignees"`
	Comments struct {
		TotalCount int           `json:"totalCount"`
		Nodes      []commentNode `json:"nodes"`
	} `json:"comments"`
	Reviews struct {
		Nodes []reviewNode `json:"nodes"`
	} `json:"reviews"`
}

type actorRef struct {
	Login string `json:"login"`
}

type commentNode struct {
	Author            *actorRef `json:"author"`
	AuthorAssociation string    `json:"authorAssociation"`
	CreatedAt         time.Time `json:"createdAt"`
}

type reviewNode struct {
	Author            *actorRef `json:"author"`
	AuthorAssociation string    `json:"authorAssociation"`
	SubmittedAt       time.Time `json:"submittedAt"`
	State             string    `json:"state"`
}

// parseOrphanedResponse parses the GraphQL response and identifies orphaned contributions
func parseOrphanedResponse(data json.RawMessage, owner, repo string, opts model.OrphanedSearchOptions) ([]model.OrphanedContribution, error) {
	var resp orphanedGraphQLResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse orphaned response: %w", err)
	}

	var contributions []model.OrphanedContribution

	// Process issues
	for _, issue := range resp.Repository.Issues.Nodes {
		if issue.Author == nil {
			continue
		}

		// Skip if author is a team member
		if IsTeamMember(issue.AuthorAssociation) {
			continue
		}

		// Analyze comment pattern
		lastTeam, consecutive := analyzeComments(issue.Comments.Nodes, issue.Author.Login)

		// Check if orphaned based on criteria
		if isOrphaned(issue.UpdatedAt, lastTeam, consecutive, opts) {
			// Extract assignee logins
			var assignees []string
			for _, a := range issue.Assignees.Nodes {
				assignees = append(assignees, a.Login)
			}

			contributions = append(contributions, model.OrphanedContribution{
				Owner:             owner,
				Repo:              repo,
				Number:            issue.Number,
				Title:             issue.Title,
				IsPR:              false,
				Author:            issue.Author.Login,
				AuthorAssociation: issue.AuthorAssociation,
				CreatedAt:         issue.CreatedAt,
				UpdatedAt:         issue.UpdatedAt,
				LastTeamActivity:  lastTeam,
				ConsecutiveAuthor: consecutive,
				HTMLURL:           issue.URL,
				CommentCount:      issue.Comments.TotalCount,
				Assignees:         assignees,
			})
		}
	}

	// Process pull requests
	for _, pr := range resp.Repository.PullRequests.Nodes {
		if pr.Author == nil {
			continue
		}

		// Skip if author is a team member
		if IsTeamMember(pr.AuthorAssociation) {
			continue
		}

		// Analyze comment pattern and reviews
		lastTeamComment, consecutive := analyzeComments(pr.Comments.Nodes, pr.Author.Login)
		lastTeamReview := analyzeReviews(pr.Reviews.Nodes)

		// Take the most recent team activity
		lastTeam := lastTeamComment
		if lastTeamReview != nil && (lastTeam == nil || lastTeamReview.After(*lastTeam)) {
			lastTeam = lastTeamReview
		}

		// Check if orphaned based on criteria
		if isOrphaned(pr.UpdatedAt, lastTeam, consecutive, opts) {
			// Determine review state from reviews
			reviewState := determineReviewState(pr.Reviews.Nodes, pr.ReviewDecision)

			// Extract assignee logins
			var assignees []string
			for _, a := range pr.Assignees.Nodes {
				assignees = append(assignees, a.Login)
			}

			contributions = append(contributions, model.OrphanedContribution{
				Owner:             owner,
				Repo:              repo,
				Number:            pr.Number,
				Title:             pr.Title,
				IsPR:              true,
				Author:            pr.Author.Login,
				AuthorAssociation: pr.AuthorAssociation,
				CreatedAt:         pr.CreatedAt,
				UpdatedAt:         pr.UpdatedAt,
				LastTeamActivity:  lastTeam,
				ConsecutiveAuthor: consecutive,
				HTMLURL:           pr.URL,
				CommentCount:      pr.Comments.TotalCount,
				Assignees:         assignees,
				Additions:         pr.Additions,
				Deletions:         pr.Deletions,
				ReviewDecision:    pr.ReviewDecision,
				ReviewState:       reviewState,
			})
		}
	}

	return contributions, nil
}

// IsTeamMember checks if a user is a collaborator based on authorAssociation
func IsTeamMember(association string) bool {
	switch association {
	case "MEMBER", "OWNER", "COLLABORATOR":
		return true
	default:
		// CONTRIBUTOR, FIRST_TIMER, FIRST_TIME_CONTRIBUTOR, NONE = external
		return false
	}
}

// analyzeComments analyzes the comment pattern to find last team activity
// and count consecutive comments from the original author
func analyzeComments(comments []commentNode, originalAuthor string) (*time.Time, int) {
	var lastTeamActivity *time.Time
	consecutiveAuthor := 0
	foundNonAuthor := false

	// Comments are ordered by time (last 10), iterate in reverse to count consecutive from end
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]
		if c.Author == nil {
			continue
		}

		isTeam := IsTeamMember(c.AuthorAssociation)

		// Track last team activity
		if isTeam {
			if lastTeamActivity == nil || c.CreatedAt.After(*lastTeamActivity) {
				t := c.CreatedAt
				lastTeamActivity = &t
			}
		}

		// Count consecutive author comments from the end
		if !foundNonAuthor {
			if c.Author.Login == originalAuthor {
				consecutiveAuthor++
			} else {
				foundNonAuthor = true
			}
		}
	}

	return lastTeamActivity, consecutiveAuthor
}

// analyzeReviews finds the most recent team review
func analyzeReviews(reviews []reviewNode) *time.Time {
	var lastTeamReview *time.Time

	for _, r := range reviews {
		if r.Author == nil {
			continue
		}

		if IsTeamMember(r.AuthorAssociation) {
			if lastTeamReview == nil || r.SubmittedAt.After(*lastTeamReview) {
				t := r.SubmittedAt
				lastTeamReview = &t
			}
		}
	}

	return lastTeamReview
}

// determineReviewState determines the review state from reviews and reviewDecision
func determineReviewState(reviews []reviewNode, reviewDecision string) string {
	// Map reviewDecision to our internal state names
	switch reviewDecision {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes_requested"
	case "REVIEW_REQUIRED":
		return "review_required"
	}

	// Fall back to checking individual reviews for the most recent state
	var latestReview *reviewNode
	for i := range reviews {
		r := &reviews[i]
		if r.Author == nil {
			continue
		}
		if latestReview == nil || r.SubmittedAt.After(latestReview.SubmittedAt) {
			latestReview = r
		}
	}

	if latestReview != nil {
		switch latestReview.State {
		case "APPROVED":
			return "approved"
		case "CHANGES_REQUESTED":
			return "changes_requested"
		case "COMMENTED", "PENDING":
			return "reviewed"
		}
	}

	return "pending"
}

// isOrphaned determines if a contribution should be flagged as orphaned
func isOrphaned(updatedAt time.Time, lastTeamActivity *time.Time, consecutiveAuthor int, opts model.OrphanedSearchOptions) bool {
	now := time.Now()

	// Check if stale (no activity in StaleDays)
	daysSinceUpdate := int(now.Sub(updatedAt).Hours() / 24)

	// If there's been team activity, measure from that
	if lastTeamActivity != nil {
		daysSinceTeam := int(now.Sub(*lastTeamActivity).Hours() / 24)
		if daysSinceTeam >= opts.StaleDays {
			return true
		}
	} else {
		// No team activity at all - use update time
		if daysSinceUpdate >= opts.StaleDays {
			return true
		}
	}

	// Check consecutive author comments threshold
	if consecutiveAuthor >= opts.ConsecutiveAuthorComments {
		return true
	}

	return false
}

// orphanedToItem converts an model.OrphanedContribution to a model.Item
func orphanedToItem(contrib model.OrphanedContribution) model.Item {
	fullName := contrib.Owner + "/" + contrib.Repo

	subjectType := model.SubjectIssue
	if contrib.IsPR {
		subjectType = model.SubjectPullRequest
	}

	details := &model.ItemDetails{
		Number:                    contrib.Number,
		State:                     "open",
		HTMLURL:                   contrib.HTMLURL,
		CreatedAt:                 contrib.CreatedAt,
		UpdatedAt:                 contrib.UpdatedAt,
		Author:                    contrib.Author,
		Assignees:                 contrib.Assignees,
		CommentCount:              contrib.CommentCount,
		IsPR:                      contrib.IsPR,
		AuthorAssociation:         contrib.AuthorAssociation,
		LastTeamActivityAt:        contrib.LastTeamActivity,
		ConsecutiveAuthorComments: contrib.ConsecutiveAuthor,
	}

	// Add PR-specific fields
	if contrib.IsPR {
		details.Additions = contrib.Additions
		details.Deletions = contrib.Deletions
		details.ReviewState = contrib.ReviewState
	}

	n := model.Item{
		ID:        fmt.Sprintf("orphaned-%s-%d", fullName, contrib.Number),
		Reason:    model.ReasonOrphaned,
		Unread:    true,
		UpdatedAt: contrib.UpdatedAt,
		Repository: model.Repository{
			Name:     contrib.Repo,
			FullName: fullName,
			HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
		},
		Subject: model.Subject{
			Title: contrib.Title,
			URL:   contrib.HTMLURL,
			Type:  subjectType,
		},
		URL:     contrib.HTMLURL,
		Details: details,
	}

	return n
}

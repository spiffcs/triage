package ghclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
)

// OrphanedSearchOptions configures the search for orphaned contributions
type OrphanedSearchOptions struct {
	Repos                     []string
	Since                     time.Time
	StaleDays                 int
	ConsecutiveAuthorComments int
	MaxPerRepo                int
}

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
	repoFullName string
	items        []model.Item
	err          error
}

// ListOrphanedContributions finds external PRs/issues needing attention
func (c *Client) ListOrphanedContributions(ctx context.Context, opts OrphanedSearchOptions) ([]model.Item, error) {
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

			// Check for context cancellation before acquiring semaphore
			select {
			case <-ctx.Done():
				results <- orphanedRepoResult{
					repoFullName: fullName,
					err:          ctx.Err(),
				}
				return
			case sem <- struct{}{}: // Acquire
			}
			defer func() { <-sem }() // Release

			items, err := c.fetchOrphanedForRepo(ctx, owner, repo, opts)
			results <- orphanedRepoResult{
				repoFullName: fullName,
				items:        items,
				err:          err,
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
		items := result.items
		if len(items) > opts.MaxPerRepo {
			items = items[:opts.MaxPerRepo]
		}
		allItems = append(allItems, items...)
	}

	return allItems, nil
}

// fetchOrphanedForRepo fetches orphaned contributions for a single repository
func (c *Client) fetchOrphanedForRepo(ctx context.Context, owner, repo string, opts OrphanedSearchOptions) ([]model.Item, error) {
	query := BuildOrphanedQuery(owner, repo)
	respData, err := c.executeGraphQL(ctx, query, c.token)
	if err != nil {
		return nil, err
	}

	return parseOrphanedResponse(respData, owner, repo, opts)
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
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
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
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
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

// parseOrphanedResponse parses the GraphQL response and returns orphaned items
func parseOrphanedResponse(data json.RawMessage, owner, repo string, opts OrphanedSearchOptions) ([]model.Item, error) {
	var resp orphanedGraphQLResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse orphaned response: %w", err)
	}

	fullName := owner + "/" + repo
	var items []model.Item

	// Process issues
	for _, issue := range resp.Repository.Issues.Nodes {
		if issue.Author == nil {
			continue
		}

		// Skip if author is a team member
		if model.IsTeamMember(issue.AuthorAssociation) {
			continue
		}

		// Analyze comment pattern
		lastTeam, consecutive := analyzeComments(issue.Comments.Nodes, issue.Author.Login)

		// Check if orphaned based on criteria
		if !isOrphaned(issue.UpdatedAt, lastTeam, consecutive, opts) {
			continue
		}

		// Extract assignee logins
		var assignees []string
		for _, a := range issue.Assignees.Nodes {
			assignees = append(assignees, a.Login)
		}

		// Extract labels
		var labels []string
		for _, l := range issue.Labels.Nodes {
			labels = append(labels, l.Name)
		}

		items = append(items, model.Item{
			ID:        fmt.Sprintf("orphaned-%s-%d", fullName, issue.Number),
			Reason:    model.ReasonOrphaned,
			Unread:    true,
			UpdatedAt: issue.UpdatedAt,
			Repository: model.Repository{
				Name:     repo,
				FullName: fullName,
				HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
			},
			Subject: model.Subject{
				Title: issue.Title,
				URL:   issue.URL,
				Type:  model.SubjectIssue,
			},
			URL: issue.URL,
			// Promoted common fields
			Type:                      model.ItemTypeIssue,
			Number:                    issue.Number,
			State:                     "open",
			HTMLURL:                   issue.URL,
			CreatedAt:                 issue.CreatedAt,
			Author:                    issue.Author.Login,
			Assignees:                 assignees,
			Labels:                    labels,
			CommentCount:              issue.Comments.TotalCount,
			AuthorAssociation:         issue.AuthorAssociation,
			LastTeamActivityAt:        lastTeam,
			ConsecutiveAuthorComments: consecutive,
			// Issue-specific details
			Details: &model.IssueDetails{},
		})
	}

	// Process pull requests
	for _, pr := range resp.Repository.PullRequests.Nodes {
		if pr.Author == nil {
			continue
		}

		// Skip if author is a team member
		if model.IsTeamMember(pr.AuthorAssociation) {
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
		if !isOrphaned(pr.UpdatedAt, lastTeam, consecutive, opts) {
			continue
		}

		// Determine review state from reviews
		reviewState := determineReviewState(pr.Reviews.Nodes, pr.ReviewDecision)

		// Extract assignee logins
		var assignees []string
		for _, a := range pr.Assignees.Nodes {
			assignees = append(assignees, a.Login)
		}

		// Extract labels
		var labels []string
		for _, l := range pr.Labels.Nodes {
			labels = append(labels, l.Name)
		}

		items = append(items, model.Item{
			ID:        fmt.Sprintf("orphaned-%s-%d", fullName, pr.Number),
			Reason:    model.ReasonOrphaned,
			Unread:    true,
			UpdatedAt: pr.UpdatedAt,
			Repository: model.Repository{
				Name:     repo,
				FullName: fullName,
				HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
			},
			Subject: model.Subject{
				Title: pr.Title,
				URL:   pr.URL,
				Type:  model.SubjectPullRequest,
			},
			URL: pr.URL,
			// Promoted common fields
			Type:                      model.ItemTypePullRequest,
			Number:                    pr.Number,
			State:                     "open",
			HTMLURL:                   pr.URL,
			CreatedAt:                 pr.CreatedAt,
			Author:                    pr.Author.Login,
			Assignees:                 assignees,
			Labels:                    labels,
			CommentCount:              pr.Comments.TotalCount,
			AuthorAssociation:         pr.AuthorAssociation,
			LastTeamActivityAt:        lastTeam,
			ConsecutiveAuthorComments: consecutive,
			// PR-specific details
			Details: &model.PRDetails{
				Additions:   pr.Additions,
				Deletions:   pr.Deletions,
				ReviewState: reviewState,
			},
		})
	}

	return items, nil
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

		isTeam := model.IsTeamMember(c.AuthorAssociation)

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

		if model.IsTeamMember(r.AuthorAssociation) {
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
func isOrphaned(updatedAt time.Time, lastTeamActivity *time.Time, consecutiveAuthor int, opts OrphanedSearchOptions) bool {
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

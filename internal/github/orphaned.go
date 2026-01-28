package github

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spiffcs/triage/internal/log"
)

// OrphanedSearchOptions configures the search for orphaned contributions
type OrphanedSearchOptions struct {
	Repos                     []string
	Since                     time.Time
	StaleDays                 int
	ConsecutiveAuthorComments int
	MaxPerRepo                int
}

// OrphanedContribution represents an issue or PR from an external contributor
// that may need team attention
type OrphanedContribution struct {
	Owner             string
	Repo              string
	Number            int
	Title             string
	IsPR              bool
	Author            string
	AuthorAssociation string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastTeamActivity  *time.Time
	ConsecutiveAuthor int
	HTMLURL           string
}

// ListOrphanedContributions finds external PRs/issues needing attention
func (c *Client) ListOrphanedContributions(opts OrphanedSearchOptions) ([]Notification, error) {
	if len(opts.Repos) == 0 {
		return nil, nil
	}

	// Set defaults
	if opts.StaleDays <= 0 {
		opts.StaleDays = 7
	}
	if opts.ConsecutiveAuthorComments <= 0 {
		opts.ConsecutiveAuthorComments = 2
	}
	if opts.MaxPerRepo <= 0 {
		opts.MaxPerRepo = 20
	}

	var allNotifications []Notification

	for _, repoFullName := range opts.Repos {
		parts := strings.Split(repoFullName, "/")
		if len(parts) != 2 {
			log.Debug("invalid repo format, skipping", "repo", repoFullName)
			continue
		}
		owner, repo := parts[0], parts[1]

		contributions, err := c.fetchOrphanedForRepo(owner, repo, opts)
		if err != nil {
			log.Debug("failed to fetch orphaned contributions", "repo", repoFullName, "error", err)
			continue
		}

		// Limit total results per repo
		if len(contributions) > opts.MaxPerRepo {
			contributions = contributions[:opts.MaxPerRepo]
		}

		for _, contrib := range contributions {
			notification := orphanedToNotification(contrib)
			allNotifications = append(allNotifications, notification)
		}
	}

	return allNotifications, nil
}

// fetchOrphanedForRepo fetches orphaned contributions for a single repository
func (c *Client) fetchOrphanedForRepo(owner, repo string, opts OrphanedSearchOptions) ([]OrphanedContribution, error) {
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
        comments(last: 10) {
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
        comments(last: 10) {
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
	Comments          struct {
		Nodes []commentNode `json:"nodes"`
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
	Comments          struct {
		Nodes []commentNode `json:"nodes"`
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
}

// parseOrphanedResponse parses the GraphQL response and identifies orphaned contributions
func parseOrphanedResponse(data json.RawMessage, owner, repo string, opts OrphanedSearchOptions) ([]OrphanedContribution, error) {
	var resp orphanedGraphQLResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse orphaned response: %w", err)
	}

	var contributions []OrphanedContribution

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
			contributions = append(contributions, OrphanedContribution{
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
			contributions = append(contributions, OrphanedContribution{
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

// orphanedToNotification converts an OrphanedContribution to a Notification
func orphanedToNotification(contrib OrphanedContribution) Notification {
	fullName := contrib.Owner + "/" + contrib.Repo

	subjectType := SubjectIssue
	if contrib.IsPR {
		subjectType = SubjectPullRequest
	}

	n := Notification{
		ID:        fmt.Sprintf("orphaned-%s-%d", fullName, contrib.Number),
		Reason:    ReasonOrphaned,
		Unread:    true,
		UpdatedAt: contrib.UpdatedAt,
		Repository: Repository{
			Name:     contrib.Repo,
			FullName: fullName,
			HTMLURL:  fmt.Sprintf("https://github.com/%s", fullName),
		},
		Subject: Subject{
			Title: contrib.Title,
			URL:   contrib.HTMLURL,
			Type:  subjectType,
		},
		URL: contrib.HTMLURL,
		Details: &ItemDetails{
			Number:                    contrib.Number,
			State:                     "open",
			HTMLURL:                   contrib.HTMLURL,
			CreatedAt:                 contrib.CreatedAt,
			UpdatedAt:                 contrib.UpdatedAt,
			Author:                    contrib.Author,
			IsPR:                      contrib.IsPR,
			AuthorAssociation:         contrib.AuthorAssociation,
			LastTeamActivityAt:        contrib.LastTeamActivity,
			ConsecutiveAuthorComments: contrib.ConsecutiveAuthor,
		},
	}

	return n
}

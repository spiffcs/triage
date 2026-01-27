package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spiffcs/triage/internal/log"
)

const (
	graphqlEndpoint = "https://api.github.com/graphql"
	// Maximum items per GraphQL query (GitHub's complexity limits)
	graphqlBatchSize = 50
)

// graphqlRequest represents a GraphQL request payload.
type graphqlRequest struct {
	Query string `json:"query"`
}

// graphqlResponse represents a generic GraphQL response.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphqlError  `json:"errors"`
}

type graphqlError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// PRGraphQLResult contains the GraphQL response for a pull request.
type PRGraphQLResult struct {
	Number       int
	State        string
	Additions    int
	Deletions    int
	ChangedFiles int
	IsDraft      bool
	Mergeable    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ClosedAt     *time.Time
	MergedAt     *time.Time
	Author       string
	Assignees    []string
	Labels       []string
	ReviewState  string
	CIStatus     string
	CommentCount int
}

// IssueGraphQLResult contains the GraphQL response for an issue.
type IssueGraphQLResult struct {
	Number        int
	State         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ClosedAt      *time.Time
	Author        string
	Assignees     []string
	Labels        []string
	CommentCount  int
	LastCommenter string
}

// enrichmentItem tracks what we need to enrich.
type enrichmentItem struct {
	index  int    // Index in the notifications slice
	owner  string // Repository owner
	repo   string // Repository name
	number int    // Issue/PR number
	isPR   bool   // True if PR, false if Issue
}

// EnrichNotificationsGraphQL enriches notifications using GraphQL batch queries.
// Returns the number of successfully enriched items.
func (c *Client) EnrichNotificationsGraphQL(notifications []Notification, token string, onProgress func(completed, total int)) (int, error) {
	// Separate PRs and Issues, and identify items that need enrichment
	var items []enrichmentItem

	for i := range notifications {
		n := &notifications[i]
		if n.Subject.URL == "" {
			continue
		}

		number, err := ExtractIssueNumber(n.Subject.URL)
		if err != nil {
			continue
		}

		parts := strings.Split(n.Repository.FullName, "/")
		if len(parts) != 2 {
			continue
		}

		items = append(items, enrichmentItem{
			index:  i,
			owner:  parts[0],
			repo:   parts[1],
			number: number,
			isPR:   n.Subject.Type == SubjectPullRequest,
		})
	}

	if len(items) == 0 {
		return 0, nil
	}

	total := len(items)
	enriched := 0

	// Process in batches
	for batchStart := 0; batchStart < len(items); batchStart += graphqlBatchSize {
		batchEnd := batchStart + graphqlBatchSize
		if batchEnd > len(items) {
			batchEnd = len(items)
		}
		batch := items[batchStart:batchEnd]

		// Separate PRs and Issues in this batch
		var prItems, issueItems []enrichmentItem
		for _, item := range batch {
			if item.isPR {
				prItems = append(prItems, item)
			} else {
				issueItems = append(issueItems, item)
			}
		}

		// Enrich PRs
		if len(prItems) > 0 {
			results, err := c.batchEnrichPRs(prItems, token)
			if err != nil {
				log.Debug("GraphQL PR enrichment failed", "error", err)
			} else {
				for _, item := range prItems {
					if result, ok := results[item.index]; ok {
						applyPRResult(&notifications[item.index], result)
						enriched++
					}
				}
			}
		}

		// Enrich Issues
		if len(issueItems) > 0 {
			results, err := c.batchEnrichIssues(issueItems, token)
			if err != nil {
				log.Debug("GraphQL Issue enrichment failed", "error", err)
			} else {
				for _, item := range issueItems {
					if result, ok := results[item.index]; ok {
						applyIssueResult(&notifications[item.index], result)
						enriched++
					}
				}
			}
		}

		if onProgress != nil {
			onProgress(batchEnd, total)
		}
	}

	return enriched, nil
}

// batchEnrichPRs fetches PR details for multiple items in a single GraphQL query.
func (c *Client) batchEnrichPRs(items []enrichmentItem, token string) (map[int]*PRGraphQLResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	query := buildPRQuery(items)
	respData, err := c.executeGraphQL(query, token)
	if err != nil {
		return nil, err
	}

	return parsePRResponse(respData, items)
}

// batchEnrichIssues fetches Issue details for multiple items in a single GraphQL query.
func (c *Client) batchEnrichIssues(items []enrichmentItem, token string) (map[int]*IssueGraphQLResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	query := buildIssueQuery(items)
	respData, err := c.executeGraphQL(query, token)
	if err != nil {
		return nil, err
	}

	return parseIssueResponse(respData, items)
}

// executeGraphQL executes a GraphQL query against GitHub's API.
func (c *Client) executeGraphQL(query string, token string) (json.RawMessage, error) {
	reqBody := graphqlRequest{Query: query}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(c.ctx, "POST", graphqlEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GraphQL response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GraphQL request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var gqlResp graphqlResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		// Log errors but don't fail - some items might still be valid
		for _, e := range gqlResp.Errors {
			log.Debug("GraphQL error", "message", e.Message, "type", e.Type)
		}
	}

	return gqlResp.Data, nil
}

// buildPRQuery builds a GraphQL query for multiple PRs using aliases.
func buildPRQuery(items []enrichmentItem) string {
	var sb strings.Builder
	sb.WriteString("query {\n")

	for i, item := range items {
		alias := fmt.Sprintf("pr%d", i)
		sb.WriteString(fmt.Sprintf(`  %s: repository(owner: "%s", name: "%s") {
    pullRequest(number: %d) {
      number
      state
      additions
      deletions
      changedFiles
      isDraft
      mergeable
      createdAt
      updatedAt
      closedAt
      mergedAt
      author { login }
      assignees(first: 10) { nodes { login } }
      labels(first: 20) { nodes { name } }
      reviews(last: 100) {
        nodes {
          state
          author { login }
        }
      }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              state
            }
          }
        }
      }
      comments { totalCount }
      reviewComments { totalCount }
    }
  }
`, alias, item.owner, item.repo, item.number))
	}

	sb.WriteString("}")
	return sb.String()
}

// buildIssueQuery builds a GraphQL query for multiple Issues using aliases.
func buildIssueQuery(items []enrichmentItem) string {
	var sb strings.Builder
	sb.WriteString("query {\n")

	for i, item := range items {
		alias := fmt.Sprintf("issue%d", i)
		sb.WriteString(fmt.Sprintf(`  %s: repository(owner: "%s", name: "%s") {
    issue(number: %d) {
      number
      state
      createdAt
      updatedAt
      closedAt
      author { login }
      assignees(first: 10) { nodes { login } }
      labels(first: 20) { nodes { name } }
      comments(last: 1) {
        totalCount
        nodes {
          author { login }
        }
      }
    }
  }
`, alias, item.owner, item.repo, item.number))
	}

	sb.WriteString("}")
	return sb.String()
}

// parsePRResponse parses the GraphQL response for PRs.
func parsePRResponse(data json.RawMessage, items []enrichmentItem) (map[int]*PRGraphQLResult, error) {
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse PR response data: %w", err)
	}

	results := make(map[int]*PRGraphQLResult)

	for i, item := range items {
		alias := fmt.Sprintf("pr%d", i)
		repoData, ok := rawData[alias]
		if !ok || repoData == nil || string(repoData) == "null" {
			continue
		}

		var repo struct {
			PullRequest *prGraphQLData `json:"pullRequest"`
		}
		if err := json.Unmarshal(repoData, &repo); err != nil {
			log.Debug("failed to parse PR data", "alias", alias, "error", err)
			continue
		}
		if repo.PullRequest == nil {
			continue
		}

		pr := repo.PullRequest
		result := &PRGraphQLResult{
			Number:       pr.Number,
			State:        strings.ToLower(pr.State),
			Additions:    pr.Additions,
			Deletions:    pr.Deletions,
			ChangedFiles: pr.ChangedFiles,
			IsDraft:      pr.IsDraft,
			Mergeable:    pr.Mergeable,
			CreatedAt:    pr.CreatedAt,
			UpdatedAt:    pr.UpdatedAt,
			CommentCount: pr.Comments.TotalCount + pr.ReviewComments.TotalCount,
		}

		if pr.Author != nil {
			result.Author = pr.Author.Login
		}

		if pr.ClosedAt != nil && !pr.ClosedAt.IsZero() {
			result.ClosedAt = pr.ClosedAt
		}
		if pr.MergedAt != nil && !pr.MergedAt.IsZero() {
			result.MergedAt = pr.MergedAt
			result.State = "merged"
		}

		// Parse assignees
		for _, a := range pr.Assignees.Nodes {
			if a.Login != "" {
				result.Assignees = append(result.Assignees, a.Login)
			}
		}

		// Parse labels
		for _, l := range pr.Labels.Nodes {
			if l.Name != "" {
				result.Labels = append(result.Labels, l.Name)
			}
		}

		// Calculate review state from reviews
		result.ReviewState = calculateReviewState(pr.Reviews.Nodes)

		// Get CI status from status check rollup
		result.CIStatus = getCIStatusFromCommits(pr.Commits.Nodes)

		results[item.index] = result
	}

	return results, nil
}

// prGraphQLData represents the PR data from GraphQL response.
type prGraphQLData struct {
	Number       int        `json:"number"`
	State        string     `json:"state"`
	Additions    int        `json:"additions"`
	Deletions    int        `json:"deletions"`
	ChangedFiles int        `json:"changedFiles"`
	IsDraft      bool       `json:"isDraft"`
	Mergeable    string     `json:"mergeable"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	ClosedAt     *time.Time `json:"closedAt"`
	MergedAt     *time.Time `json:"mergedAt"`
	Author       *struct {
		Login string `json:"login"`
	} `json:"author"`
	Assignees struct {
		Nodes []struct {
			Login string `json:"login"`
		} `json:"nodes"`
	} `json:"assignees"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Reviews struct {
		Nodes []struct {
			State  string `json:"state"`
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
		} `json:"nodes"`
	} `json:"reviews"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
	ReviewComments struct {
		TotalCount int `json:"totalCount"`
	} `json:"reviewComments"`
}

// parseIssueResponse parses the GraphQL response for Issues.
func parseIssueResponse(data json.RawMessage, items []enrichmentItem) (map[int]*IssueGraphQLResult, error) {
	var rawData map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse Issue response data: %w", err)
	}

	results := make(map[int]*IssueGraphQLResult)

	for i, item := range items {
		alias := fmt.Sprintf("issue%d", i)
		repoData, ok := rawData[alias]
		if !ok || repoData == nil || string(repoData) == "null" {
			continue
		}

		var repo struct {
			Issue *issueGraphQLData `json:"issue"`
		}
		if err := json.Unmarshal(repoData, &repo); err != nil {
			log.Debug("failed to parse Issue data", "alias", alias, "error", err)
			continue
		}
		if repo.Issue == nil {
			continue
		}

		issue := repo.Issue
		result := &IssueGraphQLResult{
			Number:       issue.Number,
			State:        strings.ToLower(issue.State),
			CreatedAt:    issue.CreatedAt,
			UpdatedAt:    issue.UpdatedAt,
			CommentCount: issue.Comments.TotalCount,
		}

		if issue.Author != nil {
			result.Author = issue.Author.Login
		}

		if issue.ClosedAt != nil && !issue.ClosedAt.IsZero() {
			result.ClosedAt = issue.ClosedAt
		}

		// Parse assignees
		for _, a := range issue.Assignees.Nodes {
			if a.Login != "" {
				result.Assignees = append(result.Assignees, a.Login)
			}
		}

		// Parse labels
		for _, l := range issue.Labels.Nodes {
			if l.Name != "" {
				result.Labels = append(result.Labels, l.Name)
			}
		}

		// Get last commenter
		if len(issue.Comments.Nodes) > 0 && issue.Comments.Nodes[0].Author != nil {
			result.LastCommenter = issue.Comments.Nodes[0].Author.Login
		}

		results[item.index] = result
	}

	return results, nil
}

// issueGraphQLData represents the Issue data from GraphQL response.
type issueGraphQLData struct {
	Number    int        `json:"number"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	ClosedAt  *time.Time `json:"closedAt"`
	Author    *struct {
		Login string `json:"login"`
	} `json:"author"`
	Assignees struct {
		Nodes []struct {
			Login string `json:"login"`
		} `json:"nodes"`
	} `json:"assignees"`
	Labels struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Comments struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
		} `json:"nodes"`
	} `json:"comments"`
}

// calculateReviewState determines the overall review state from individual reviews.
func calculateReviewState(reviews []struct {
	State  string `json:"state"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
}) string {
	// Track the latest review state per user
	latestReviews := make(map[string]string)
	for _, review := range reviews {
		if review.Author == nil {
			continue
		}
		state := review.State
		if state != "" && state != "COMMENTED" && state != "PENDING" {
			latestReviews[review.Author.Login] = state
		}
	}

	hasApproval := false
	hasChangesRequested := false

	for _, state := range latestReviews {
		switch state {
		case "APPROVED":
			hasApproval = true
		case "CHANGES_REQUESTED":
			hasChangesRequested = true
		}
	}

	if hasChangesRequested {
		return "changes_requested"
	}
	if hasApproval {
		return "approved"
	}
	if len(reviews) > 0 {
		return "reviewed"
	}
	return "pending"
}

// getCIStatusFromCommits extracts CI status from the commit's status check rollup.
func getCIStatusFromCommits(commits []struct {
	Commit struct {
		StatusCheckRollup *struct {
			State string `json:"state"`
		} `json:"statusCheckRollup"`
	} `json:"commit"`
}) string {
	if len(commits) == 0 {
		return ""
	}

	commit := commits[0]
	if commit.Commit.StatusCheckRollup == nil {
		return ""
	}

	state := strings.ToLower(commit.Commit.StatusCheckRollup.State)
	switch state {
	case "success":
		return "success"
	case "failure", "error":
		return "failure"
	case "pending":
		return "pending"
	default:
		return ""
	}
}

// applyPRResult applies GraphQL PR result to a notification.
func applyPRResult(n *Notification, result *PRGraphQLResult) {
	if n.Details == nil {
		n.Details = &ItemDetails{}
	}

	n.Details.Number = result.Number
	n.Details.State = result.State
	n.Details.Additions = result.Additions
	n.Details.Deletions = result.Deletions
	n.Details.ChangedFiles = result.ChangedFiles
	n.Details.Draft = result.IsDraft
	n.Details.Mergeable = result.Mergeable == "MERGEABLE"
	n.Details.CreatedAt = result.CreatedAt
	n.Details.UpdatedAt = result.UpdatedAt
	n.Details.ClosedAt = result.ClosedAt
	n.Details.MergedAt = result.MergedAt
	n.Details.Merged = result.MergedAt != nil
	n.Details.Author = result.Author
	n.Details.Assignees = result.Assignees
	n.Details.Labels = result.Labels
	n.Details.ReviewState = result.ReviewState
	n.Details.CIStatus = result.CIStatus
	n.Details.CommentCount = result.CommentCount
	n.Details.IsPR = true

	// Set HTMLURL if not already set
	if n.Details.HTMLURL == "" && n.Repository.FullName != "" {
		n.Details.HTMLURL = fmt.Sprintf("https://github.com/%s/pull/%d", n.Repository.FullName, result.Number)
	}
}

// applyIssueResult applies GraphQL Issue result to a notification.
func applyIssueResult(n *Notification, result *IssueGraphQLResult) {
	if n.Details == nil {
		n.Details = &ItemDetails{}
	}

	n.Details.Number = result.Number
	n.Details.State = result.State
	n.Details.CreatedAt = result.CreatedAt
	n.Details.UpdatedAt = result.UpdatedAt
	n.Details.ClosedAt = result.ClosedAt
	n.Details.Author = result.Author
	n.Details.Assignees = result.Assignees
	n.Details.Labels = result.Labels
	n.Details.CommentCount = result.CommentCount
	n.Details.LastCommenter = result.LastCommenter
	n.Details.IsPR = false

	// Set HTMLURL if not already set
	if n.Details.HTMLURL == "" && n.Repository.FullName != "" {
		n.Details.HTMLURL = fmt.Sprintf("https://github.com/%s/issues/%d", n.Repository.FullName, result.Number)
	}
}

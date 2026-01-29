package ghclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spiffcs/triage/internal/log"
	"github.com/spiffcs/triage/internal/model"
)

const (
	graphqlEndpoint = "https://api.github.com/graphql"
	// Maximum items per GraphQL query (GitHub's complexity limits)
	// With reviewDecision instead of reviews(last:100), we can fit more per batch
	graphqlBatchSize = 100
	// Maximum concurrent batch requests to avoid rate limiting
	maxConcurrentBatches = 12
)

// graphqlHTTPClient is a configured HTTP client optimized for GraphQL requests
// with connection pooling and keep-alive for reduced latency.
var graphqlHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     30 * time.Second,
	},
	Timeout: 30 * time.Second,
}

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
	Number             int
	State              string
	Additions          int
	Deletions          int
	ChangedFiles       int
	IsDraft            bool
	Mergeable          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ClosedAt           *time.Time
	MergedAt           *time.Time
	Author             string
	Assignees          []string
	Labels             []string
	ReviewState        string
	CIStatus           string
	CommentCount       int
	RequestedReviewers []string
	LatestReviewer     string
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
	owner  string // model.Repository owner
	repo   string // model.Repository name
	number int    // Issue/PR number
	isPR   bool   // True if PR, false if Issue
}

// batchResult holds the results from processing a single batch.
type batchResult struct {
	batchIdx     int
	batchSize    int
	prResults    map[int]*PRGraphQLResult
	issueResults map[int]*IssueGraphQLResult
	prErr        error
	issueErr     error
}

// EnrichItemsGraphQL enriches items using GraphQL batch queries.
// Returns the number of successfully enriched items.
func (c *Client) EnrichItemsGraphQL(ctx context.Context, items []model.Item, token string, onProgress func(completed, total int)) (int, error) {
	// Separate PRs and Issues, and identify items that need enrichment
	var enrichItems []enrichmentItem

	for i := range items {
		n := &items[i]
		if n.Subject.URL == "" {
			log.Debug("skipping item without model.Subject.URL", "id", n.ID)
			continue
		}

		number, err := extractIssueNumber(n.Subject.URL)
		if err != nil {
			log.Debug("failed to extract issue number", "url", n.Subject.URL, "error", err)
			continue
		}

		parts := strings.Split(n.Repository.FullName, "/")
		if len(parts) != 2 {
			log.Debug("invalid repository name", "fullName", n.Repository.FullName)
			continue
		}

		enrichItems = append(enrichItems, enrichmentItem{
			index:  i,
			owner:  parts[0],
			repo:   parts[1],
			number: number,
			isPR:   n.Subject.Type == model.SubjectPullRequest,
		})
	}

	if len(enrichItems) == 0 {
		log.Debug("no items to enrich via GraphQL")
		return 0, nil
	}

	log.Debug("enriching items via GraphQL", "total", len(enrichItems))

	total := len(enrichItems)
	enriched := 0

	// Create batch jobs
	var batches [][]enrichmentItem
	for batchStart := 0; batchStart < len(enrichItems); batchStart += graphqlBatchSize {
		batchEnd := batchStart + graphqlBatchSize
		if batchEnd > len(enrichItems) {
			batchEnd = len(enrichItems)
		}
		batches = append(batches, enrichItems[batchStart:batchEnd])
	}

	log.Debug("processing batches concurrently", "batches", len(batches), "maxConcurrent", maxConcurrentBatches)

	// Process batches with semaphore-limited concurrency
	sem := make(chan struct{}, maxConcurrentBatches)
	results := make(chan batchResult, len(batches))
	var wg sync.WaitGroup

	for batchIdx, batch := range batches {
		wg.Add(1)
		go func(idx int, b []enrichmentItem) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			result := c.processBatch(ctx, b, token)
			result.batchIdx = idx
			result.batchSize = len(b)
			results <- result
		}(batchIdx, batch)
	}

	// Close results channel when all workers done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results and apply to notifications
	for result := range results {
		itemsProcessed := 0

		// Apply PR results
		if result.prErr != nil {
			log.Debug("GraphQL PR enrichment failed", "batch", result.batchIdx, "error", result.prErr)
		} else if result.prResults != nil {
			for idx, prResult := range result.prResults {
				log.Debug("applying PR result",
					"number", prResult.Number,
					"additions", prResult.Additions,
					"deletions", prResult.Deletions,
					"reviewState", prResult.ReviewState,
					"ciStatus", prResult.CIStatus)
				applyPRResult(&items[idx], prResult)
				enriched++
				itemsProcessed++
				// Report progress per item for smooth UI updates
				if onProgress != nil {
					onProgress(1, total)
				}
			}
		}

		// Apply Issue results
		if result.issueErr != nil {
			log.Debug("GraphQL Issue enrichment failed", "batch", result.batchIdx, "error", result.issueErr)
		} else if result.issueResults != nil {
			for idx, issueResult := range result.issueResults {
				applyIssueResult(&items[idx], issueResult)
				enriched++
				itemsProcessed++
				// Report progress per item for smooth UI updates
				if onProgress != nil {
					onProgress(1, total)
				}
			}
		}

		// Report remaining items (failed or skipped) so progress reaches 100%
		remaining := result.batchSize - itemsProcessed
		if remaining > 0 && onProgress != nil {
			onProgress(remaining, total)
		}
	}

	return enriched, nil
}

// processBatch processes a single batch, fetching PRs and Issues in parallel.
func (c *Client) processBatch(ctx context.Context, batch []enrichmentItem, token string) batchResult {
	var prItems, issueItems []enrichmentItem
	for _, item := range batch {
		if item.isPR {
			prItems = append(prItems, item)
		} else {
			issueItems = append(issueItems, item)
		}
	}

	var wg sync.WaitGroup
	var prResults map[int]*PRGraphQLResult
	var issueResults map[int]*IssueGraphQLResult
	var prErr, issueErr error

	// Fetch PRs and Issues in parallel within this batch
	if len(prItems) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Debug("enriching PRs via GraphQL", "count", len(prItems))
			prResults, prErr = c.batchEnrichPRs(ctx, prItems, token)
			if prErr == nil {
				log.Debug("GraphQL PR enrichment returned", "results", len(prResults))
			}
		}()
	}
	if len(issueItems) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Debug("enriching Issues via GraphQL", "count", len(issueItems))
			issueResults, issueErr = c.batchEnrichIssues(ctx, issueItems, token)
		}()
	}
	wg.Wait()

	return batchResult{
		prResults:    prResults,
		issueResults: issueResults,
		prErr:        prErr,
		issueErr:     issueErr,
	}
}

// batchEnrichPRs fetches PR details for multiple items in a single GraphQL query.
func (c *Client) batchEnrichPRs(ctx context.Context, items []enrichmentItem, token string) (map[int]*PRGraphQLResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	query := buildPRQuery(items)
	respData, err := c.executeGraphQL(ctx, query, token)
	if err != nil {
		return nil, err
	}

	return parsePRResponse(respData, items)
}

// batchEnrichIssues fetches Issue details for multiple items in a single GraphQL query.
func (c *Client) batchEnrichIssues(ctx context.Context, items []enrichmentItem, token string) (map[int]*IssueGraphQLResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	query := buildIssueQuery(items)
	respData, err := c.executeGraphQL(ctx, query, token)
	if err != nil {
		return nil, err
	}

	return parseIssueResponse(respData, items)
}

// executeGraphQL executes a GraphQL query against GitHub's API.
func (c *Client) executeGraphQL(ctx context.Context, query string, token string) (json.RawMessage, error) {
	reqBody := graphqlRequest{Query: query}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", graphqlEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := graphqlHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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
      reviewDecision
      reviewRequests(first: 10) {
        nodes {
          requestedReviewer {
            ... on User { login }
            ... on Team { name }
          }
        }
      }
      latestReviews(first: 10) {
        nodes {
          author { login }
          submittedAt
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
      reviewThreads { totalCount }
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

	log.Debug("parsing PR response", "aliases", len(rawData), "items", len(items))
	results := make(map[int]*PRGraphQLResult)

	for i, item := range items {
		alias := fmt.Sprintf("pr%d", i)
		repoData, ok := rawData[alias]
		if !ok || repoData == nil || string(repoData) == "null" {
			log.Debug("no data for PR alias", "alias", alias, "repo", item.owner+"/"+item.repo, "number", item.number)
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
			CommentCount: pr.Comments.TotalCount + pr.ReviewThreads.TotalCount,
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

		// Parse requested reviewers
		for _, rr := range pr.ReviewRequests.Nodes {
			if rr.RequestedReviewer != nil {
				// User has login, Team has name
				if rr.RequestedReviewer.Login != "" {
					result.RequestedReviewers = append(result.RequestedReviewers, rr.RequestedReviewer.Login)
				} else if rr.RequestedReviewer.Name != "" {
					result.RequestedReviewers = append(result.RequestedReviewers, rr.RequestedReviewer.Name)
				}
			}
		}

		// Get the most recent reviewer from latestReviews
		// latestReviews returns reviews sorted by most recent first
		if len(pr.LatestReviews.Nodes) > 0 {
			var latestTime time.Time
			for _, review := range pr.LatestReviews.Nodes {
				if review.Author != nil && review.Author.Login != "" {
					if review.SubmittedAt.After(latestTime) {
						latestTime = review.SubmittedAt
						result.LatestReviewer = review.Author.Login
					}
				}
			}
		}

		// Map reviewDecision to our review state format
		result.ReviewState = mapReviewDecision(pr.ReviewDecision)

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
	ReviewDecision string `json:"reviewDecision"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *requestedReviewer `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	LatestReviews struct {
		Nodes []struct {
			Author *struct {
				Login string `json:"login"`
			} `json:"author"`
			SubmittedAt time.Time `json:"submittedAt"`
		} `json:"nodes"`
	} `json:"latestReviews"`
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
	ReviewThreads struct {
		TotalCount int `json:"totalCount"`
	} `json:"reviewThreads"`
}

// requestedReviewer can be either a User or a Team
type requestedReviewer struct {
	Login string `json:"login"` // For User
	Name  string `json:"name"`  // For Team
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

// mapReviewDecision converts GitHub's reviewDecision enum to our internal format.
func mapReviewDecision(decision string) string {
	switch decision {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes_requested"
	case "REVIEW_REQUIRED":
		return "pending"
	default:
		return "pending"
	}
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
func applyPRResult(n *model.Item, result *PRGraphQLResult) {
	if n.Details == nil {
		n.Details = &model.ItemDetails{}
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
	n.Details.RequestedReviewers = result.RequestedReviewers
	n.Details.LatestReviewer = result.LatestReviewer
	n.Details.IsPR = true

	// Set HTMLURL if not already set
	if n.Details.HTMLURL == "" && n.Repository.FullName != "" {
		n.Details.HTMLURL = fmt.Sprintf("https://github.com/%s/pull/%d", n.Repository.FullName, result.Number)
	}
}

// applyIssueResult applies GraphQL Issue result to a notification.
func applyIssueResult(n *model.Item, result *IssueGraphQLResult) {
	if n.Details == nil {
		n.Details = &model.ItemDetails{}
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

// extractIssueNumber extracts the issue/PR number from a GitHub API URL.
// URL format: https://api.github.com/repos/owner/repo/issues/123
// or: https://api.github.com/repos/owner/repo/pulls/123
func extractIssueNumber(apiURL string) (int, error) {
	parts := strings.Split(apiURL, "/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid API URL format: %s", apiURL)
	}

	numStr := parts[len(parts)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse issue number from URL %s: %w", apiURL, err)
	}

	return num, nil
}

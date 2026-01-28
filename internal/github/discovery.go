package github

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spiffcs/triage/internal/log"
)

// DiscoveryOptions configures repository discovery
type DiscoveryOptions struct {
	MaxRepos       int  // Maximum repos to return (default: 15)
	IncludePrivate bool // Include private repos (default: true)
}

// DiscoveredRepo represents a repo the user can manage
type DiscoveredRepo struct {
	FullName   string
	Permission string
	PushedAt   time.Time
	OpenIssues int
	OpenPRs    int
}

// DiscoverMaintainableRepos finds repos where the user has write access.
// It uses the GitHub GraphQL API to query repositories where the authenticated
// user has OWNER, COLLABORATOR, or ORGANIZATION_MEMBER affiliation, then filters
// to only include repos with WRITE, MAINTAIN, or ADMIN permissions.
// Repos are sorted by most recently pushed (PUSHED_AT DESC) as a proxy for
// repositories that are actively maintained and likely to have contributions.
func (c *Client) DiscoverMaintainableRepos(opts DiscoveryOptions) ([]DiscoveredRepo, error) {
	// Set defaults
	if opts.MaxRepos <= 0 {
		opts.MaxRepos = 15
	}

	var allRepos []DiscoveredRepo
	var cursor string
	hasNextPage := true

	for hasNextPage && len(allRepos) < opts.MaxRepos {
		query := buildDiscoveryQuery(cursor)
		respData, err := c.executeGraphQL(query, c.token)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repositories: %w", err)
		}

		repos, nextCursor, hasMore, err := parseDiscoveryResponse(respData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse discovery response: %w", err)
		}

		// Filter and add valid repos
		for _, repo := range repos {
			// Skip repos without write access
			if !hasWriteAccess(repo.Permission) {
				continue
			}

			allRepos = append(allRepos, repo)
			if len(allRepos) >= opts.MaxRepos {
				break
			}
		}

		cursor = nextCursor
		hasNextPage = hasMore
	}

	log.Debug("discovered maintainable repositories",
		"count", len(allRepos),
		"maxRepos", opts.MaxRepos)

	return allRepos, nil
}

// hasWriteAccess checks if the permission level includes write access
func hasWriteAccess(permission string) bool {
	switch permission {
	case "WRITE", "MAINTAIN", "ADMIN":
		return true
	default:
		return false
	}
}

// buildDiscoveryQuery builds the GraphQL query for repository discovery
func buildDiscoveryQuery(cursor string) string {
	afterClause := ""
	if cursor != "" {
		afterClause = fmt.Sprintf(`, after: "%s"`, cursor)
	}

	return fmt.Sprintf(`query {
  viewer {
    repositories(
      first: 100
      affiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER]
      ownerAffiliations: [OWNER, COLLABORATOR, ORGANIZATION_MEMBER]
      isFork: false
      orderBy: {field: PUSHED_AT, direction: DESC}
      %s
    ) {
      nodes {
        nameWithOwner
        isArchived
        viewerPermission
        pushedAt
        issues(states: OPEN) { totalCount }
        pullRequests(states: OPEN) { totalCount }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
}`, afterClause)
}

// discoveryResponseData represents the GraphQL response structure for discovery
type discoveryResponseData struct {
	Viewer struct {
		Repositories struct {
			Nodes []struct {
				NameWithOwner    string    `json:"nameWithOwner"`
				IsArchived       bool      `json:"isArchived"`
				ViewerPermission string    `json:"viewerPermission"`
				PushedAt         time.Time `json:"pushedAt"`
				Issues           struct {
					TotalCount int `json:"totalCount"`
				} `json:"issues"`
				PullRequests struct {
					TotalCount int `json:"totalCount"`
				} `json:"pullRequests"`
			} `json:"nodes"`
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
		} `json:"repositories"`
	} `json:"viewer"`
}

// parseDiscoveryResponse parses the GraphQL discovery response
func parseDiscoveryResponse(data json.RawMessage) ([]DiscoveredRepo, string, bool, error) {
	var resp discoveryResponseData
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, "", false, fmt.Errorf("failed to parse discovery response: %w", err)
	}

	var repos []DiscoveredRepo
	for _, node := range resp.Viewer.Repositories.Nodes {
		// Skip archived repos
		if node.IsArchived {
			continue
		}

		repos = append(repos, DiscoveredRepo{
			FullName:   node.NameWithOwner,
			Permission: node.ViewerPermission,
			PushedAt:   node.PushedAt,
			OpenIssues: node.Issues.TotalCount,
			OpenPRs:    node.PullRequests.TotalCount,
		})
	}

	pageInfo := resp.Viewer.Repositories.PageInfo
	return repos, pageInfo.EndCursor, pageInfo.HasNextPage, nil
}

// DiscoverMaintainableReposCached fetches maintainable repos with caching support
func (c *Client) DiscoverMaintainableReposCached(username string, opts DiscoveryOptions, cache *Cache) ([]DiscoveredRepo, bool, error) {
	// Check cache first
	if cache != nil {
		if repos, ok := cache.GetDiscoveredRepos(username); ok {
			// Convert cached repo names to DiscoveredRepo structs
			discovered := make([]DiscoveredRepo, len(repos))
			for i, name := range repos {
				discovered[i] = DiscoveredRepo{FullName: name}
			}
			return discovered, true, nil
		}
	}

	// Check if rate limited
	if globalRateLimitState.IsLimited() {
		return nil, false, ErrRateLimited
	}

	// Fetch from API
	repos, err := c.DiscoverMaintainableRepos(opts)
	if err != nil {
		return nil, false, err
	}

	// Cache the result
	if cache != nil {
		repoNames := make([]string, len(repos))
		for i, r := range repos {
			repoNames[i] = r.FullName
		}
		if err := cache.SetDiscoveredRepos(username, repoNames); err != nil {
			log.Debug("failed to cache discovered repos", "error", err)
		}
	}

	return repos, false, nil
}

// repoNamesToStrings extracts full names from DiscoveredRepo slice
func repoNamesToStrings(repos []DiscoveredRepo) []string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.FullName
	}
	return names
}

// formatDiscoveredRepos returns a human-readable summary of discovered repos
func formatDiscoveredRepos(repos []DiscoveredRepo) string {
	if len(repos) == 0 {
		return "no repositories discovered"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("discovered %d repositories:\n", len(repos)))
	for _, r := range repos {
		sb.WriteString(fmt.Sprintf("  - %s (%s)\n", r.FullName, r.Permission))
	}
	return sb.String()
}

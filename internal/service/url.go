// Package service provides orchestration between GitHub API and caching layers.
package service

import (
	"fmt"
	"strconv"
	"strings"
)

// ExtractIssueNumber extracts the issue/PR number from a GitHub API URL.
// URL format: https://api.github.com/repos/owner/repo/issues/123
// or: https://api.github.com/repos/owner/repo/pulls/123
func ExtractIssueNumber(apiURL string) (int, error) {
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

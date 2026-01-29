// Package urlutil provides URL parsing utilities.
package urlutil

import (
	"fmt"
	"strconv"
	"strings"
)

// ExtractIssueNumber extracts the issue/PR number from the API URL.
func ExtractIssueNumber(apiURL string) (int, error) {
	// URL format: https://api.github.com/repos/owner/repo/issues/123
	// or: https://api.github.com/repos/owner/repo/pulls/123
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

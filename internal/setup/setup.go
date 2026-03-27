// Package setup provides user-facing guidance when authentication
// or configuration is missing or broken.
package setup

import (
	"errors"
	"fmt"
	"net/http"

	gh "github.com/google/go-github/v57/github"
)

// tokenGuidance is the shared setup instructions included in all token errors.
// The Notifications API requires a classic token — fine-grained tokens do not
// support it (https://docs.github.com/en/rest/activity/notifications).
const tokenGuidance = `triage only reads data — it never writes, comments, or modifies anything.
However, GitHub's Notifications API requires a classic token with broad scopes.

Recommended — use the GitHub CLI to manage credentials securely:
  gh auth login
  GITHUB_TOKEN=$(gh auth token) triage

Or create a classic token manually:
  https://github.com/settings/tokens/new
  Required scopes: notifications, repo
  Set an expiration — avoid long-lived tokens.`

// TokenMissing returns an actionable error when GITHUB_TOKEN is not set.
func TokenMissing() error {
	return fmt.Errorf("GitHub token not found\n\n%s", tokenGuidance)
}

// TokenInvalid inspects an authentication error from the GitHub API
// and returns guidance tailored to the specific failure.
func TokenInvalid(err error) error {
	var ghErr *gh.ErrorResponse
	if !errors.As(err, &ghErr) {
		return fmt.Errorf("authentication failed: %w\n\nCheck that GITHUB_TOKEN is set to a valid token", err)
	}

	switch ghErr.Response.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf(`GitHub token is invalid or expired

The token was rejected by GitHub: %s

To fix:
  - If using gh CLI: run "gh auth login"
  - If using a classic token: check expiration at https://github.com/settings/tokens

%s`, ghErr.Message, tokenGuidance)

	case http.StatusForbidden:
		return tokenForbidden(ghErr)

	default:
		return fmt.Errorf("authentication failed (HTTP %d): %w", ghErr.Response.StatusCode, err)
	}
}

// tokenForbidden handles 403 responses, distinguishing missing scopes from
// SSO/org restrictions.
func tokenForbidden(ghErr *gh.ErrorResponse) error {
	have := ghErr.Response.Header.Get("X-OAuth-Scopes")
	if have != "" {
		return fmt.Errorf(`GitHub token is missing required permissions

Your token has scopes: %s

To fix, create a new token with the correct permissions:

%s`, have, tokenGuidance)
	}

	return fmt.Errorf(`GitHub API access denied

This may be caused by:
  - SSO enforcement: authorize the token for your org at https://github.com/settings/tokens
  - IP allow-list restrictions on your organization

GitHub said: %s`, ghErr.Message)
}

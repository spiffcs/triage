package setup

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	gh "github.com/google/go-github/v57/github"
)

func TestTokenMissing(t *testing.T) {
	err := TokenMissing()
	msg := err.Error()

	for _, want := range []string{
		"GitHub token not found",
		"gh auth login",
		"gh auth token",
		"tokens/new",
		"notifications",
		"repo",
		"only reads data",
		"Set an expiration",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("TokenMissing() should contain %q, got:\n%s", want, msg)
		}
	}
}

func TestTokenInvalid_401(t *testing.T) {
	apiErr := &gh.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{},
		},
		Message: "Bad credentials",
	}

	err := TokenInvalid(fmt.Errorf("get user: %w", apiErr))
	msg := err.Error()

	for _, want := range []string{
		"invalid or expired",
		"Bad credentials",
		"gh auth login",
		"https://github.com/settings/tokens",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("TokenInvalid(401) should contain %q, got:\n%s", want, msg)
		}
	}
}

func TestTokenInvalid_403_MissingScopes(t *testing.T) {
	apiErr := &gh.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusForbidden,
			Header: http.Header{
				"X-Oauth-Scopes": []string{"public_repo"},
			},
		},
		Message: "Resource not accessible",
	}

	err := TokenInvalid(apiErr)
	msg := err.Error()

	for _, want := range []string{
		"missing required permissions",
		"public_repo",
		"notifications, repo",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("TokenInvalid(403+scopes) should contain %q, got:\n%s", want, msg)
		}
	}
}

func TestTokenInvalid_403_SSO(t *testing.T) {
	apiErr := &gh.ErrorResponse{
		Response: &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{},
		},
		Message: "Resource protected by organization SAML enforcement",
	}

	err := TokenInvalid(apiErr)
	msg := err.Error()

	for _, want := range []string{
		"access denied",
		"SSO enforcement",
		"SAML enforcement",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("TokenInvalid(403+SSO) should contain %q, got:\n%s", want, msg)
		}
	}
}

func TestTokenInvalid_NonGitHubError(t *testing.T) {
	err := TokenInvalid(fmt.Errorf("connection refused"))
	msg := err.Error()

	if !strings.Contains(msg, "authentication failed") {
		t.Errorf("TokenInvalid(non-github) should contain 'authentication failed', got:\n%s", msg)
	}
	if !strings.Contains(msg, "connection refused") {
		t.Errorf("TokenInvalid(non-github) should preserve original error, got:\n%s", msg)
	}
}

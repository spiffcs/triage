// Package constants provides a centralized location for all configuration
// values and magic numbers used throughout the triage application.
package constants

import "time"

// TUI update and display constants
const (
	// TUIUpdateInterval is the minimum time between TUI progress updates
	// to provide smooth progress display without excessive overhead.
	TUIUpdateInterval = 50 * time.Millisecond

	// LogThrottlePercent is the interval (in percent) at which progress
	// logs are emitted when not using the TUI.
	LogThrottlePercent = 5

	// HeaderLines is the number of lines used for the list view header.
	HeaderLines = 2

	// FooterLines is the number of lines used for the list view footer.
	FooterLines = 3

	// TruncationSuffixWidth is the width of the "..." suffix when truncating strings.
	TruncationSuffixWidth = 3
)

// Rate limiting constants
const (
	// RateLimitLowWatermark is the threshold below which rate limit
	// warnings are logged.
	RateLimitLowWatermark = 100
)

// Cache TTL constants
const (
	// DetailCacheTTL is the maximum age of cached item details before
	// they are considered stale and require re-fetching.
	DetailCacheTTL = 24 * time.Hour

	// PRListCacheTTL is the TTL for cached PR lists (shorter because
	// PR lists change more frequently).
	PRListCacheTTL = 5 * time.Minute

	// NotificationListCacheTTL is the maximum age before a full
	// notification refresh is required.
	NotificationListCacheTTL = 1 * time.Hour
)

// Review state constants
const (
	// ReviewStateApproved indicates a PR has been approved.
	ReviewStateApproved = "approved"

	// ReviewStateChangesRequested indicates changes have been requested on a PR.
	ReviewStateChangesRequested = "changes_requested"

	// ReviewStatePending indicates a PR is awaiting review.
	ReviewStatePending = "pending"

	// ReviewStateReviewRequired indicates a PR requires review.
	ReviewStateReviewRequired = "review_required"

	// ReviewStateReviewed indicates a PR has been reviewed.
	ReviewStateReviewed = "reviewed"
)

// CI status constants
const (
	// CIStatusSuccess indicates CI checks have passed.
	CIStatusSuccess = "success"

	// CIStatusFailure indicates CI checks have failed.
	CIStatusFailure = "failure"

	// CIStatusPending indicates CI checks are still running.
	CIStatusPending = "pending"
)

// Item state constants
const (
	// StateOpen indicates an issue or PR is open.
	StateOpen = "open"

	// StateClosed indicates an issue or PR is closed.
	StateClosed = "closed"

	// StateMerged indicates a PR has been merged.
	StateMerged = "merged"
)

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

	// ItemListCacheTTL is the TTL for cached item lists (PRs, issues).
	// Shorter than details cache because lists change more frequently.
	ItemListCacheTTL = 5 * time.Minute

	// NotificationsCacheTTL is the maximum age before a full
	// notification list refresh is required.
	NotificationsCacheTTL = 30 * time.Minute

	// OrphanedCacheTTL is the TTL for the orphaned contributions list.
	// Longer TTL since orphaned status changes slowly.
	OrphanedCacheTTL = 24 * time.Hour
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

// Column width constants for table/list display
const (
	// ColPriority is the width of the priority column.
	ColPriority = 10

	// ColType is the width of the type column (ISS/PR).
	ColType = 5

	// ColAuthor is the width of the author column.
	ColAuthor = 15

	// ColAssigned is the width of the assigned user column.
	ColAssigned = 12

	// ColCI is the width of the CI status column.
	ColCI = 2

	// ColRepo is the width of the repository column.
	ColRepo = 26

	// ColTitle is the width of the title column.
	ColTitle = 40

	// ColStatus is the width of the status column.
	ColStatus = 20

	// ColAge is the width of the age column.
	ColAge = 5
)

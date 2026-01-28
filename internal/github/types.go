package github

import (
	"strings"
	"time"
)

// NotificationReason represents why the user received a notification.
// See: https://docs.github.com/en/rest/activity/notifications
type NotificationReason string

const (
	ReasonMention         NotificationReason = "mention"
	ReasonReviewRequested NotificationReason = "review_requested"
	ReasonAuthor          NotificationReason = "author"
	ReasonAssign          NotificationReason = "assign"
	ReasonComment         NotificationReason = "comment"
	ReasonSubscribed      NotificationReason = "subscribed"
	ReasonTeamMention     NotificationReason = "team_mention"
	ReasonStateChange     NotificationReason = "state_change"
	ReasonCIActivity      NotificationReason = "ci_activity"
	ReasonManual          NotificationReason = "manual"

	// ReasonOrphaned is a synthetic reason created by triage to identify
	// external contributions that appear to be waiting for maintainer response.
	// This is not a GitHub API reason.
	ReasonOrphaned NotificationReason = "orphaned"
)

// GitHub API reasons not yet implemented:
//   - approval_requested: deployment review requests
//   - invitation: repository contribution invitations
//   - member_feature_requested: organization feature requests
//   - security_advisory_credit: security advisory credits
//   - security_alert: security vulnerability alerts

// AllNotificationReasons contains all valid notification reasons.
// This is the single source of truth for valid reason values.
var AllNotificationReasons = []NotificationReason{
	ReasonMention,
	ReasonReviewRequested,
	ReasonAuthor,
	ReasonAssign,
	ReasonComment,
	ReasonSubscribed,
	ReasonTeamMention,
	ReasonStateChange,
	ReasonCIActivity,
	ReasonManual,
	ReasonOrphaned,
}

// NotificationReasonsString returns a comma-separated string of all valid reasons.
func NotificationReasonsString() string {
	reasons := make([]string, len(AllNotificationReasons))
	for i, r := range AllNotificationReasons {
		reasons[i] = string(r)
	}
	return strings.Join(reasons, ", ")
}

// SubjectType represents the type of notification subject
type SubjectType string

const (
	SubjectIssue       SubjectType = "Issue"
	SubjectPullRequest SubjectType = "PullRequest"
	SubjectRelease     SubjectType = "Release"
	SubjectDiscussion  SubjectType = "Discussion"
)

// Notification represents a GitHub notification with enriched context
type Notification struct {
	ID         string             `json:"id"`
	Reason     NotificationReason `json:"reason"`
	Unread     bool               `json:"unread"`
	UpdatedAt  time.Time          `json:"updatedAt"`
	Repository Repository         `json:"repository"`
	Subject    Subject            `json:"subject"`
	URL        string             `json:"url"`

	// Enriched data (populated by details fetcher)
	Details *ItemDetails `json:"details,omitempty"`
}

// Repository represents a GitHub repository
type Repository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"fullName"`
	Private  bool   `json:"private"`
	HTMLURL  string `json:"htmlUrl"`
}

// Subject represents the notification subject (issue, PR, etc.)
type Subject struct {
	Title string      `json:"title"`
	URL   string      `json:"url"`
	Type  SubjectType `json:"type"`
}

// ItemDetails contains enriched information about an issue or PR
type ItemDetails struct {
	Number    int        `json:"number"`
	State     string     `json:"state"` // open, closed, merged
	HTMLURL   string     `json:"htmlUrl"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	ClosedAt  *time.Time `json:"closedAt,omitempty"`

	// User info
	Author    string   `json:"author"`
	Assignees []string `json:"assignees"`

	// Metadata
	Labels        []string `json:"labels"`
	CommentCount  int      `json:"commentCount"`
	LastCommenter string   `json:"lastCommenter,omitempty"`

	// PR-specific
	IsPR               bool       `json:"isPR"`
	Merged             bool       `json:"merged,omitempty"`
	MergedAt           *time.Time `json:"mergedAt,omitempty"`
	Additions          int        `json:"additions,omitempty"`
	Deletions          int        `json:"deletions,omitempty"`
	ChangedFiles       int        `json:"changedFiles,omitempty"`
	ReviewState        string     `json:"reviewState,omitempty"` // approved, changes_requested, pending
	ReviewComments     int        `json:"reviewComments,omitempty"`
	Mergeable          bool       `json:"mergeable,omitempty"`
	CIStatus           string     `json:"ciStatus,omitempty"` // success, failure, pending
	Draft              bool       `json:"draft,omitempty"`
	RequestedReviewers []string   `json:"requestedReviewers,omitempty"`
	LatestReviewer     string     `json:"latestReviewer,omitempty"`

	// Orphaned contribution detection
	AuthorAssociation         string     `json:"authorAssociation,omitempty"` // MEMBER, COLLABORATOR, CONTRIBUTOR, etc.
	LastTeamActivityAt        *time.Time `json:"lastTeamActivityAt,omitempty"`
	ConsecutiveAuthorComments int        `json:"consecutiveAuthorComments,omitempty"`
}

package github

import "time"

// NotificationReason represents why the user received a notification
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
)

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
	Number    int       `json:"number"`
	State     string    `json:"state"` // open, closed, merged
	HTMLURL   string    `json:"htmlUrl"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	ClosedAt  *time.Time `json:"closedAt,omitempty"`

	// User info
	Author    string   `json:"author"`
	Assignees []string `json:"assignees"`

	// Metadata
	Labels       []string `json:"labels"`
	CommentCount int      `json:"commentCount"`

	// PR-specific
	IsPR           bool       `json:"isPR"`
	Merged         bool       `json:"merged,omitempty"`
	MergedAt       *time.Time `json:"mergedAt,omitempty"`
	Additions      int        `json:"additions,omitempty"`
	Deletions      int        `json:"deletions,omitempty"`
	ChangedFiles   int        `json:"changedFiles,omitempty"`
	ReviewState    string     `json:"reviewState,omitempty"` // approved, changes_requested, pending
	ReviewComments int        `json:"reviewComments,omitempty"`
	Mergeable      bool       `json:"mergeable,omitempty"`
	CIStatus       string     `json:"ciStatus,omitempty"` // success, failure, pending
	Draft          bool       `json:"draft,omitempty"`
}

package model

import "time"

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

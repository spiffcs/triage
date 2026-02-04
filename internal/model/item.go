// Package model contains domain types for the triage application.
// These types are independent of any external GitHub library.
package model

import (
	"encoding/json"
	"time"
)

// ItemReason represents why the user received a notification.
// See: https://docs.github.com/en/rest/activity/notifications
type ItemReason string

const (
	ReasonMention         ItemReason = "mention"
	ReasonReviewRequested ItemReason = "review_requested"
	ReasonAuthor          ItemReason = "author"
	ReasonAssign          ItemReason = "assign"
	ReasonComment         ItemReason = "comment"
	ReasonSubscribed      ItemReason = "subscribed"
	ReasonTeamMention     ItemReason = "team_mention"
	ReasonStateChange     ItemReason = "state_change"
	ReasonCIActivity      ItemReason = "ci_activity"
	ReasonManual          ItemReason = "manual"

	// ReasonOrphaned is a synthetic reason created by triage to identify
	// external contributions that appear to be waiting for maintainer response.
	// This is not a GitHub API reason.
	ReasonOrphaned ItemReason = "orphaned"
)

// GitHub API reasons not yet implemented:
//   - approval_requested: deployment review requests
//   - invitation: repository contribution invitations
//   - member_feature_requested: organization feature requests
//   - security_advisory_credit: security advisory credits
//   - security_alert: security vulnerability alerts

// AllItemReasons contains all valid item reasons.
// This is the single source of truth for valid reason values.
var AllItemReasons = []ItemReason{
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

// SubjectType represents the type of notification subject
type SubjectType string

const (
	SubjectIssue       SubjectType = "Issue"
	SubjectPullRequest SubjectType = "PullRequest"
	SubjectRelease     SubjectType = "Release"
	SubjectDiscussion  SubjectType = "Discussion"
)

// ItemType represents whether an item is an issue or pull request
type ItemType string

const (
	ItemTypeIssue       ItemType = "issue"
	ItemTypePullRequest ItemType = "pull_request"
)

// Item represents a GitHub notification with enriched context
type Item struct {
	ID         string     `json:"id"`
	Reason     ItemReason `json:"reason"`
	Unread     bool       `json:"unread"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	Repository Repository `json:"repository"`
	Subject    Subject    `json:"subject"`
	URL        string     `json:"url"`

	// Type discriminator for issue vs PR
	Type ItemType `json:"type,omitempty"`

	// Common fields (promoted from ItemDetails)
	Number       int        `json:"number,omitempty"`
	State        string     `json:"state,omitempty"` // open, closed, merged
	HTMLURL      string     `json:"htmlUrl,omitempty"`
	CreatedAt    time.Time  `json:"createdAt,omitempty"`
	ClosedAt     *time.Time `json:"closedAt,omitempty"`
	Author       string     `json:"author,omitempty"`
	Assignees    []string   `json:"assignees,omitempty"`
	Labels       []string   `json:"labels,omitempty"`
	CommentCount int        `json:"commentCount,omitempty"`

	// Orphaned detection (common to both)
	AuthorAssociation         string     `json:"authorAssociation,omitempty"`
	LastTeamActivityAt        *time.Time `json:"lastTeamActivityAt,omitempty"`
	ConsecutiveAuthorComments int        `json:"consecutiveAuthorComments,omitempty"`

	// Type-specific details (interface)
	Details Details `json:"details,omitempty"`
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

// UnmarshalJSON implements custom JSON unmarshaling to handle polymorphic Details
func (i *Item) UnmarshalJSON(data []byte) error {
	type Alias Item
	aux := &struct {
		Details json.RawMessage `json:"details,omitempty"`
		*Alias
	}{Alias: (*Alias)(i)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.Details) == 0 || string(aux.Details) == "null" {
		return nil
	}

	switch i.Type {
	case ItemTypePullRequest:
		var pr PRDetails
		if err := json.Unmarshal(aux.Details, &pr); err != nil {
			return err
		}
		i.Details = &pr
	case ItemTypeIssue:
		var issue IssueDetails
		if err := json.Unmarshal(aux.Details, &issue); err != nil {
			return err
		}
		i.Details = &issue
	}
	return nil
}

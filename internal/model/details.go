package model

import "time"

// Details is an interface for type-specific details.
// Use type assertions to access PRDetails or IssueDetails.
type Details interface {
	isDetails() // marker method
}

// PRDetails contains PR-specific enriched information
type PRDetails struct {
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
}

func (*PRDetails) isDetails() {}

// IssueDetails contains issue-specific enriched information
type IssueDetails struct {
	LastCommenter string `json:"lastCommenter,omitempty"`
}

func (*IssueDetails) isDetails() {}

// IsPR returns true if the item is a pull request.
// This is a convenience method that checks the Type field.
func (i *Item) IsPR() bool {
	return i.Type == ItemTypePullRequest
}

// PRDetails returns the PRDetails if this is a PR, nil otherwise.
func (i *Item) PRDetails() *PRDetails {
	if i.Details == nil {
		return nil
	}
	pr, _ := i.Details.(*PRDetails)
	return pr
}

// IssueDetails returns the IssueDetails if this is an issue, nil otherwise.
func (i *Item) IssueDetails() *IssueDetails {
	if i.Details == nil {
		return nil
	}
	issue, _ := i.Details.(*IssueDetails)
	return issue
}

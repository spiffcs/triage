package model

import "time"

// OrphanedSearchOptions configures the search for orphaned contributions
type OrphanedSearchOptions struct {
	Repos                     []string
	Since                     time.Time
	StaleDays                 int
	ConsecutiveAuthorComments int
	MaxPerRepo                int
}

// OrphanedContribution represents an issue or PR from an external contributor
// that may need team attention
type OrphanedContribution struct {
	Owner             string
	Repo              string
	Number            int
	Title             string
	IsPR              bool
	Author            string
	AuthorAssociation string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastTeamActivity  *time.Time
	ConsecutiveAuthor int
	HTMLURL           string
	CommentCount      int
	Assignees         []string
	// PR-specific fields
	Additions      int
	Deletions      int
	ReviewDecision string
	ReviewState    string
}

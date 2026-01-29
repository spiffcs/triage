package model

import "time"

// IsTeamMember checks if a user is a collaborator based on authorAssociation
func IsTeamMember(association string) bool {
	switch association {
	case "MEMBER", "OWNER", "COLLABORATOR":
		return true
	default:
		// CONTRIBUTOR, FIRST_TIMER, FIRST_TIME_CONTRIBUTOR, NONE = external
		return false
	}
}

// AnalyzeComments analyzes the comment pattern to find last team activity
// and count consecutive comments from the original author.
// Takes a slice of CommentInfo (author, association, time) and the original author's login.
func AnalyzeComments(comments []CommentInfo, originalAuthor string) (*time.Time, int) {
	var lastTeamActivity *time.Time
	consecutiveAuthor := 0
	foundNonAuthor := false

	// Comments are ordered by time (last 10), iterate in reverse to count consecutive from end
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i]
		if c.Author == "" {
			continue
		}

		isTeam := IsTeamMember(c.AuthorAssociation)

		// Track last team activity
		if isTeam {
			if lastTeamActivity == nil || c.CreatedAt.After(*lastTeamActivity) {
				t := c.CreatedAt
				lastTeamActivity = &t
			}
		}

		// Count consecutive author comments from the end
		if !foundNonAuthor {
			if c.Author == originalAuthor {
				consecutiveAuthor++
			} else {
				foundNonAuthor = true
			}
		}
	}

	return lastTeamActivity, consecutiveAuthor
}

// AnalyzeReviews finds the most recent team review.
// Takes a slice of ReviewInfo (author, association, time).
func AnalyzeReviews(reviews []ReviewInfo) *time.Time {
	var lastTeamReview *time.Time

	for _, r := range reviews {
		if r.Author == "" {
			continue
		}

		if IsTeamMember(r.AuthorAssociation) {
			if lastTeamReview == nil || r.SubmittedAt.After(*lastTeamReview) {
				t := r.SubmittedAt
				lastTeamReview = &t
			}
		}
	}

	return lastTeamReview
}

// IsOrphaned determines if a contribution should be flagged as orphaned
func IsOrphaned(updatedAt time.Time, lastTeamActivity *time.Time, consecutiveAuthor int, staleDays int, consecutiveThreshold int) bool {
	now := time.Now()

	// Check if stale (no activity in StaleDays)
	daysSinceUpdate := int(now.Sub(updatedAt).Hours() / 24)

	// If there's been team activity, measure from that
	if lastTeamActivity != nil {
		daysSinceTeam := int(now.Sub(*lastTeamActivity).Hours() / 24)
		if daysSinceTeam >= staleDays {
			return true
		}
	} else {
		// No team activity at all - use update time
		if daysSinceUpdate >= staleDays {
			return true
		}
	}

	// Check consecutive author comments threshold
	if consecutiveAuthor >= consecutiveThreshold {
		return true
	}

	return false
}

// CommentInfo contains information about a comment for analysis
type CommentInfo struct {
	Author            string
	AuthorAssociation string
	CreatedAt         time.Time
}

// ReviewInfo contains information about a review for analysis
type ReviewInfo struct {
	Author            string
	AuthorAssociation string
	SubmittedAt       time.Time
}

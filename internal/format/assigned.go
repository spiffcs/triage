package format

// AssignedOptions contains the fields needed to determine the assigned user.
type AssignedOptions struct {
	Assignees          []string
	IsPR               bool
	LatestReviewer     string
	RequestedReviewers []string
}

// Assigned returns the user to display in the assigned column.
// Returns the first assignee or empty string if no one is assigned.
// Callers should add their own placeholder for empty values.
func Assigned(input AssignedOptions) string {
	if len(input.Assignees) > 0 {
		return input.Assignees[0]
	}
	return ""
}

// TruncateUsername truncates a username to fit within maxWidth.
// If truncation is needed, an ellipsis is added.
func TruncateUsername(username string, maxWidth int) string {
	if len(username) <= maxWidth {
		return username
	}
	if maxWidth <= 1 {
		return username[:maxWidth]
	}
	return username[:maxWidth-1] + "\u2026" // ellipsis character
}

package priority

import (
	"strings"
	"time"

	"github.com/hal/github-prio/internal/github"
)

// ScoreWeights defines the base scores for different notification reasons
type ScoreWeights struct {
	ReviewRequested int
	Mention         int
	TeamMention     int
	Author          int
	Assign          int
	Comment         int
	Subscribed      int
	StateChange     int
	CIActivity      int

	// Modifiers
	OldUnreadBonus    int // Per day old
	HotTopicBonus     int // Many recent comments
	LowHangingBonus   int // Small PR or good-first-issue
	OpenStateBonus    int // Still open
	ClosedStatePenalty int // Already closed
}

// DefaultWeights returns the default scoring weights
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		ReviewRequested: 100,
		Mention:         90,
		TeamMention:     85,
		Author:          70,
		Assign:          60,
		Comment:         30,
		StateChange:     25,
		Subscribed:      10,
		CIActivity:      5,

		OldUnreadBonus:     2, // +2 per day
		HotTopicBonus:      15,
		LowHangingBonus:    20,
		OpenStateBonus:     10,
		ClosedStatePenalty: -30,
	}
}

// Heuristics implements rule-based priority scoring
type Heuristics struct {
	Weights      ScoreWeights
	CurrentUser  string
}

// NewHeuristics creates a new heuristics scorer
func NewHeuristics(currentUser string) *Heuristics {
	return &Heuristics{
		Weights:     DefaultWeights(),
		CurrentUser: currentUser,
	}
}

// Score calculates the priority score for a notification
func (h *Heuristics) Score(n *github.Notification) int {
	score := h.baseScore(n.Reason)

	// Apply modifiers based on enriched details
	if n.Details != nil {
		score += h.detailModifiers(n)
	}

	// Age modifier - older unread items get priority boost
	age := time.Since(n.UpdatedAt)
	daysOld := int(age.Hours() / 24)
	if daysOld > 0 {
		score += min(daysOld*h.Weights.OldUnreadBonus, 30) // Cap at 30
	}

	return max(score, 0)
}

func (h *Heuristics) baseScore(reason github.NotificationReason) int {
	switch reason {
	case github.ReasonReviewRequested:
		return h.Weights.ReviewRequested
	case github.ReasonMention:
		return h.Weights.Mention
	case github.ReasonTeamMention:
		return h.Weights.TeamMention
	case github.ReasonAuthor:
		return h.Weights.Author
	case github.ReasonAssign:
		return h.Weights.Assign
	case github.ReasonComment:
		return h.Weights.Comment
	case github.ReasonStateChange:
		return h.Weights.StateChange
	case github.ReasonSubscribed:
		return h.Weights.Subscribed
	case github.ReasonCIActivity:
		return h.Weights.CIActivity
	default:
		return h.Weights.Subscribed
	}
}

func (h *Heuristics) detailModifiers(n *github.Notification) int {
	d := n.Details
	modifier := 0

	// State modifiers
	if d.State == "open" {
		modifier += h.Weights.OpenStateBonus
	} else if d.State == "closed" || d.State == "merged" {
		modifier += h.Weights.ClosedStatePenalty
	}

	// Hot topic - many comments indicate active discussion
	if d.CommentCount > 10 {
		modifier += h.Weights.HotTopicBonus
	}

	// Low-hanging fruit detection
	if h.isLowHangingFruit(d) {
		modifier += h.Weights.LowHangingBonus
	}

	// If user is author and PR needs updates, boost priority
	if d.Author == h.CurrentUser && d.ReviewState == "changes_requested" {
		modifier += 20
	}

	return modifier
}

// isLowHangingFruit detects items that are likely quick wins
func (h *Heuristics) isLowHangingFruit(d *github.ItemDetails) bool {
	// Check for specific labels
	lowHangingLabels := []string{
		"good first issue",
		"good-first-issue",
		"help wanted",
		"help-wanted",
		"easy",
		"beginner",
		"trivial",
		"documentation",
		"docs",
		"typo",
	}

	for _, label := range d.Labels {
		labelLower := strings.ToLower(label)
		for _, target := range lowHangingLabels {
			if strings.Contains(labelLower, target) {
				return true
			}
		}
	}

	// Small PRs are quick to review
	if d.IsPR && d.ChangedFiles <= 3 && (d.Additions+d.Deletions) <= 50 {
		return true
	}

	return false
}

// Categorize determines the category for a notification
func (h *Heuristics) Categorize(n *github.Notification, score int) Category {
	reason := n.Reason

	// Urgent: review requests and direct mentions
	if reason == github.ReasonReviewRequested || reason == github.ReasonMention {
		return CategoryUrgent
	}

	// Check for low-hanging fruit
	if n.Details != nil && h.isLowHangingFruit(n.Details) {
		return CategoryLowHanging
	}

	// Important: author notifications, assignments
	if reason == github.ReasonAuthor || reason == github.ReasonAssign || reason == github.ReasonTeamMention {
		return CategoryImportant
	}

	// Everything else is FYI
	return CategoryFYI
}

// DetermineAction suggests what action the user should take
func (h *Heuristics) DetermineAction(n *github.Notification) string {
	reason := n.Reason
	details := n.Details

	switch reason {
	case github.ReasonReviewRequested:
		return "Review PR"
	case github.ReasonMention:
		return "Respond to mention"
	case github.ReasonTeamMention:
		return "Team mentioned - check if relevant"
	case github.ReasonAuthor:
		if details != nil && details.ReviewState == "changes_requested" {
			return "Address review feedback"
		}
		if details != nil && details.ReviewState == "approved" {
			return "Merge PR"
		}
		return "Check activity on your item"
	case github.ReasonAssign:
		return "Work on assigned item"
	case github.ReasonComment:
		return "Review new comments"
	case github.ReasonStateChange:
		if details != nil && (details.State == "closed" || details.State == "merged") {
			return "Acknowledge closure"
		}
		return "Check state change"
	case github.ReasonSubscribed:
		return "Review activity (subscribed)"
	default:
		return "Review notification"
	}
}

// DeterminePriorityLevel converts a score to a priority level
func DeterminePriorityLevel(score int) PriorityLevel {
	switch {
	case score >= 90:
		return PriorityUrgent
	case score >= 60:
		return PriorityHigh
	case score >= 30:
		return PriorityMedium
	default:
		return PriorityLow
	}
}

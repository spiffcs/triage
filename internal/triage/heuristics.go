package triage

import (
	"strings"
	"time"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/github"
)

// Heuristics implements rule-based priority scoring
type Heuristics struct {
	Weights        config.ScoreWeights
	CurrentUser    string
	QuickWinLabels []string
}

// NewHeuristics creates a new heuristics scorer with the given weights and labels
func NewHeuristics(currentUser string, weights config.ScoreWeights, quickWinLabels []string) *Heuristics {
	return &Heuristics{
		Weights:        weights,
		CurrentUser:    currentUser,
		QuickWinLabels: quickWinLabels,
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
		score += min(daysOld*h.Weights.OldUnreadBonus, h.Weights.MaxAgeBonus)
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
	switch d.State {
	case "open":
		modifier += h.Weights.OpenStateBonus
	case "closed", "merged":
		modifier += h.Weights.ClosedStatePenalty
	}

	// Hot topic - many comments indicate active discussion
	if d.CommentCount > h.Weights.HotTopicThreshold {
		modifier += h.Weights.HotTopicBonus
	}

	// Low-hanging fruit detection
	if h.isLowHangingFruit(d) {
		modifier += h.Weights.LowHangingBonus
	}

	// Author-specific modifiers for their own PRs
	if d.Author == h.CurrentUser && d.IsPR {
		modifier += h.authoredPRModifiers(d)
	}

	return modifier
}

// authoredPRModifiers calculates score modifiers for user's own PRs
func (h *Heuristics) authoredPRModifiers(d *github.ItemDetails) int {
	modifier := 0

	// PR is approved and ready to merge - urgent action needed!
	if d.ReviewState == "approved" {
		modifier += h.Weights.ApprovedPRBonus
		if d.Mergeable {
			modifier += h.Weights.MergeablePRBonus
		}
	}

	// PR has changes requested - needs work
	if d.ReviewState == "changes_requested" {
		modifier += h.Weights.ChangesRequestedBonus
	}

	// PR has review comments - might need response
	if d.ReviewComments > 0 {
		modifier += min(d.ReviewComments*h.Weights.ReviewCommentBonus, h.Weights.ReviewCommentMaxBonus)
	}

	// Stale PR - no activity after threshold, needs a kick
	daysSinceUpdate := int(time.Since(d.UpdatedAt).Hours() / 24)
	if daysSinceUpdate >= h.Weights.StalePRThresholdDays {
		daysOverThreshold := daysSinceUpdate - h.Weights.StalePRThresholdDays + 1
		modifier += min(daysOverThreshold*h.Weights.StalePRBonusPerDay, h.Weights.StalePRMaxBonus)
	}

	// Draft PR - lower priority (not ready for review yet)
	if d.Draft {
		modifier += h.Weights.DraftPRPenalty
	}

	return modifier
}

// normalizeLabel converts a label to a normalized form for comparison
// by lowercasing and treating hyphens and spaces as equivalent
func normalizeLabel(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "-", " "))
}

// isLowHangingFruit detects items that are likely quick wins
func (h *Heuristics) isLowHangingFruit(d *github.ItemDetails) bool {
	// Check for configured quick win labels
	for _, label := range d.Labels {
		labelNorm := normalizeLabel(label)
		for _, target := range h.QuickWinLabels {
			if strings.Contains(labelNorm, normalizeLabel(target)) {
				return true
			}
		}
	}

	// Small PRs are quick to review
	if d.IsPR && d.ChangedFiles <= h.Weights.SmallPRMaxFiles && (d.Additions+d.Deletions) <= h.Weights.SmallPRMaxLines {
		return true
	}

	return false
}

// DeterminePriority determines the priority for a notification (displayed in table)
func (h *Heuristics) DeterminePriority(n *github.Notification, score int) PriorityLevel {
	reason := n.Reason

	// Urgent: review requests (if enabled)
	if reason == github.ReasonReviewRequested && h.Weights.ReviewRequestedIsUrgent {
		return PriorityUrgent
	}

	// Urgent: direct mentions (if enabled)
	if reason == github.ReasonMention && h.Weights.MentionIsUrgent {
		return PriorityUrgent
	}

	// Authored PRs that are approved and mergeable are urgent (if enabled)
	if reason == github.ReasonAuthor && n.Details != nil {
		d := n.Details
		if d.IsPR && d.ReviewState == "approved" && d.Mergeable && h.Weights.ApprovedMergeablePRIsUrgent {
			return PriorityUrgent
		}
		// PRs with changes requested need attention (if enabled)
		if d.IsPR && d.ReviewState == "changes_requested" && h.Weights.ChangesRequestedPRIsUrgent {
			return PriorityUrgent
		}
	}

	// Score-based promotion to Urgent (Important → Urgent)
	if score >= h.Weights.ImportantPromotionThreshold {
		return PriorityUrgent
	}

	// Check for quick wins (low-hanging fruit)
	if n.Details != nil && h.isLowHangingFruit(n.Details) {
		return PriorityQuickWin
	}

	// Important: author notifications, assignments
	if reason == github.ReasonAuthor || reason == github.ReasonAssign || reason == github.ReasonTeamMention {
		return PriorityImportant
	}

	// Score-based promotion to Important (Notable → Important)
	if score >= h.Weights.NotablePromotionThreshold {
		return PriorityImportant
	}

	// Score-based promotion to Notable (FYI → Notable)
	if score >= h.Weights.FYIPromotionThreshold {
		return PriorityNotable
	}

	// Everything else is FYI
	return PriorityFYI
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
		if details == nil {
			return "Check activity on your item"
		}
		return h.determineAuthoredPRAction(details)
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

// determineAuthoredPRAction suggests actions for user's own PRs
func (h *Heuristics) determineAuthoredPRAction(d *github.ItemDetails) string {
	if !d.IsPR {
		return "Check activity on your issue"
	}

	// Draft PR
	if d.Draft {
		return "Finish draft PR"
	}

	// Approved and mergeable - merge it!
	if d.ReviewState == "approved" && d.Mergeable {
		return "Merge PR"
	}

	// Approved but not mergeable (conflicts?)
	if d.ReviewState == "approved" && !d.Mergeable {
		return "Resolve conflicts & merge"
	}

	// Changes requested
	if d.ReviewState == "changes_requested" {
		return "Address review feedback"
	}

	// Has review comments that might need response
	if d.ReviewComments > 0 {
		return "Respond to review comments"
	}

	// Stale PR - needs attention
	daysSinceUpdate := int(time.Since(d.UpdatedAt).Hours() / 24)
	if daysSinceUpdate >= h.Weights.StalePRThresholdDays {
		if d.ReviewState == "pending" {
			return "Request review (stale)"
		}
		return "Follow up on PR"
	}

	// Pending review
	if d.ReviewState == "pending" {
		return "Awaiting review"
	}

	return "Check PR status"
}

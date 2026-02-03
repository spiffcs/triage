package triage

import (
	"strings"
	"time"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/constants"
	"github.com/spiffcs/triage/internal/model"
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

// Score calculates the priority score for an item
func (h *Heuristics) Score(n *model.Item) int {
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

func (h *Heuristics) baseScore(reason model.ItemReason) int {
	switch reason {
	case model.ReasonReviewRequested:
		return h.Weights.ReviewRequested
	case model.ReasonMention:
		return h.Weights.Mention
	case model.ReasonTeamMention:
		return h.Weights.TeamMention
	case model.ReasonAuthor:
		return h.Weights.Author
	case model.ReasonAssign:
		return h.Weights.Assign
	case model.ReasonComment:
		return h.Weights.Comment
	case model.ReasonStateChange:
		return h.Weights.StateChange
	case model.ReasonSubscribed:
		return h.Weights.Subscribed
	case model.ReasonCIActivity:
		return h.Weights.CIActivity
	default:
		return h.Weights.Subscribed
	}
}

func (h *Heuristics) detailModifiers(n *model.Item) int {
	modifier := 0

	// State modifiers
	switch n.State {
	case "open":
		modifier += h.Weights.OpenStateBonus
	case "closed", "merged":
		modifier += h.Weights.ClosedStatePenalty
	}

	// Hot topic - many comments indicate active discussion
	if n.CommentCount > h.Weights.HotTopicThreshold {
		modifier += h.Weights.HotTopicBonus
	}

	// Low-hanging fruit detection
	if h.isLowHangingFruit(n) {
		modifier += h.Weights.LowHangingBonus
	}

	// Author-specific modifiers for their own PRs
	if n.Author == h.CurrentUser && n.IsPR() {
		if pr := n.GetPRDetails(); pr != nil {
			modifier += h.authoredPRModifiers(n, pr)
		}
	}

	return modifier
}

// authoredPRModifiers calculates score modifiers for user's own PRs
func (h *Heuristics) authoredPRModifiers(n *model.Item, pr *model.PRDetails) int {
	modifier := 0

	// PR is approved and ready to merge - urgent action needed!
	if pr.ReviewState == constants.ReviewStateApproved {
		modifier += h.Weights.ApprovedPRBonus
		if pr.Mergeable {
			modifier += h.Weights.MergeablePRBonus
		}
	}

	// PR has changes requested - needs work
	if pr.ReviewState == constants.ReviewStateChangesRequested {
		modifier += h.Weights.ChangesRequestedBonus
	}

	// PR has review comments - might need response
	if pr.ReviewComments > 0 {
		modifier += min(pr.ReviewComments*h.Weights.ReviewCommentBonus, h.Weights.ReviewCommentMaxBonus)
	}

	// Stale PR - no activity after threshold, needs a kick
	daysSinceUpdate := int(time.Since(n.UpdatedAt).Hours() / 24)
	if daysSinceUpdate >= h.Weights.StalePRThresholdDays {
		daysOverThreshold := daysSinceUpdate - h.Weights.StalePRThresholdDays + 1
		modifier += min(daysOverThreshold*h.Weights.StalePRBonusPerDay, h.Weights.StalePRMaxBonus)
	}

	// Draft PR - lower priority (not ready for review yet)
	if pr.Draft {
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
func (h *Heuristics) isLowHangingFruit(n *model.Item) bool {
	// Check for configured quick win labels
	for _, label := range n.Labels {
		labelNorm := normalizeLabel(label)
		for _, target := range h.QuickWinLabels {
			if strings.Contains(labelNorm, normalizeLabel(target)) {
				return true
			}
		}
	}

	// Small PRs are quick to review
	if pr := n.GetPRDetails(); pr != nil {
		if pr.ChangedFiles <= h.Weights.SmallPRMaxFiles && (pr.Additions+pr.Deletions) <= h.Weights.SmallPRMaxLines {
			return true
		}
	}

	return false
}

// DeterminePriority determines the priority for a notification (displayed in table)
func (h *Heuristics) DeterminePriority(n *model.Item, score int) PriorityLevel {
	reason := n.Reason

	// Urgent: review requests (if enabled)
	if reason == model.ReasonReviewRequested && h.Weights.ReviewRequestedIsUrgent {
		return PriorityUrgent
	}

	// Urgent: direct mentions (if enabled)
	if reason == model.ReasonMention && h.Weights.MentionIsUrgent {
		return PriorityUrgent
	}

	// Authored PRs that are approved and mergeable are urgent (if enabled)
	if reason == model.ReasonAuthor && n.IsPR() {
		if pr := n.GetPRDetails(); pr != nil {
			if pr.ReviewState == constants.ReviewStateApproved && pr.Mergeable && h.Weights.ApprovedMergeablePRIsUrgent {
				return PriorityUrgent
			}
			// PRs with changes requested need attention (if enabled)
			if pr.ReviewState == constants.ReviewStateChangesRequested && h.Weights.ChangesRequestedPRIsUrgent {
				return PriorityUrgent
			}
		}
	}

	// Score-based promotion to Urgent (Important → Urgent)
	if score >= h.Weights.ImportantPromotionThreshold {
		return PriorityUrgent
	}

	// Check for quick wins (low-hanging fruit)
	if h.isLowHangingFruit(n) {
		return PriorityQuickWin
	}

	// Important: author notifications, assignments
	if reason == model.ReasonAuthor || reason == model.ReasonAssign || reason == model.ReasonTeamMention {
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
func (h *Heuristics) DetermineAction(n *model.Item) string {
	reason := n.Reason

	switch reason {
	case model.ReasonReviewRequested:
		return "Review PR"
	case model.ReasonMention:
		return "Respond to mention"
	case model.ReasonTeamMention:
		return "Team mentioned - check if relevant"
	case model.ReasonAuthor:
		return h.determineAuthoredItemAction(n)
	case model.ReasonAssign:
		return "Work on assigned item"
	case model.ReasonComment:
		return "Review new comments"
	case model.ReasonStateChange:
		if n.State == "closed" || n.State == "merged" {
			return "Acknowledge closure"
		}
		return "Check state change"
	case model.ReasonSubscribed:
		return "Review activity (subscribed)"
	default:
		return "Review notification"
	}
}

// determineAuthoredItemAction suggests actions for user's own items
func (h *Heuristics) determineAuthoredItemAction(n *model.Item) string {
	if !n.IsPR() {
		return "Check activity on your issue"
	}

	pr := n.GetPRDetails()
	if pr == nil {
		return "Check activity on your item"
	}

	// Draft PR
	if pr.Draft {
		return "Finish draft PR"
	}

	// Approved and mergeable - merge it!
	if pr.ReviewState == constants.ReviewStateApproved && pr.Mergeable {
		return "Merge PR"
	}

	// Approved but not mergeable (conflicts?)
	if pr.ReviewState == constants.ReviewStateApproved && !pr.Mergeable {
		return "Resolve conflicts & merge"
	}

	// Changes requested
	if pr.ReviewState == constants.ReviewStateChangesRequested {
		return "Address review feedback"
	}

	// Has review comments that might need response
	if pr.ReviewComments > 0 {
		return "Respond to review comments"
	}

	// Stale PR - needs attention
	daysSinceUpdate := int(time.Since(n.UpdatedAt).Hours() / 24)
	if daysSinceUpdate >= h.Weights.StalePRThresholdDays {
		if pr.ReviewState == constants.ReviewStatePending {
			return "Request review (stale)"
		}
		return "Follow up on PR"
	}

	// Pending review
	if pr.ReviewState == constants.ReviewStatePending {
		return "Awaiting review"
	}

	return "Check PR status"
}

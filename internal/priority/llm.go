package priority

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// LLMClient handles Claude API interactions
type LLMClient struct {
	client anthropic.Client
	ctx    context.Context
}

// NewLLMClient creates a new Claude API client
func NewLLMClient(apiKey string) (*LLMClient, error) {
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("Claude API key not provided. Set ANTHROPIC_API_KEY env var or use 'github-prio config set claude-key <KEY>'")
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	return &LLMClient{
		client: client,
		ctx:    context.Background(),
	}, nil
}

// Analyze uses Claude to analyze a prioritized item
func (l *LLMClient) Analyze(item *PrioritizedItem) (*LLMAnalysis, error) {
	prompt := l.buildPrompt(item)

	message, err := l.client.Messages.New(l.ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeSonnet4_5_20250929,
		MaxTokens: 500,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
		System: []anthropic.TextBlockParam{
			{Text: `You are a helpful assistant that analyzes GitHub notifications to help developers prioritize their work.
Respond with a JSON object containing:
- summary: A 1-2 sentence summary of what this notification is about
- actionNeeded: What specific action the user should take (e.g., "Review the PR focusing on error handling", "Respond to the question about API usage")
- effortEstimate: One of "quick" (< 15 min), "medium" (15 min - 1 hour), or "large" (> 1 hour)
- blockers: Array of any blockers or dependencies mentioned (empty array if none)
- tags: Array of relevant tags for this item (e.g., "bug", "feature", "documentation", "security")

Respond ONLY with the JSON object, no other text.`},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude API: %w", err)
	}

	// Extract text from response
	var responseText string
	for _, block := range message.Content {
		if block.Type == "text" {
			responseText = block.Text
			break
		}
	}

	// Parse JSON response
	var analysis LLMAnalysis
	if err := json.Unmarshal([]byte(responseText), &analysis); err != nil {
		// Try to extract JSON if there's extra text
		if start := strings.Index(responseText, "{"); start >= 0 {
			if end := strings.LastIndex(responseText, "}"); end > start {
				if err := json.Unmarshal([]byte(responseText[start:end+1]), &analysis); err != nil {
					return nil, fmt.Errorf("failed to parse Claude response: %w", err)
				}
			}
		}
		if analysis.Summary == "" {
			return nil, fmt.Errorf("failed to parse Claude response: %w", err)
		}
	}

	return &analysis, nil
}

func (l *LLMClient) buildPrompt(item *PrioritizedItem) string {
	n := item.Notification

	var sb strings.Builder
	sb.WriteString("Analyze this GitHub notification:\n\n")
	sb.WriteString(fmt.Sprintf("Repository: %s\n", n.Repository.FullName))
	sb.WriteString(fmt.Sprintf("Title: %s\n", n.Subject.Title))
	sb.WriteString(fmt.Sprintf("Type: %s\n", n.Subject.Type))
	sb.WriteString(fmt.Sprintf("Reason notified: %s\n", n.Reason))
	sb.WriteString(fmt.Sprintf("Updated: %s\n", n.UpdatedAt.Format("2006-01-02 15:04")))

	if n.Details != nil {
		d := n.Details
		sb.WriteString(fmt.Sprintf("\nState: %s\n", d.State))
		sb.WriteString(fmt.Sprintf("Author: %s\n", d.Author))
		sb.WriteString(fmt.Sprintf("Comments: %d\n", d.CommentCount))

		if len(d.Labels) > 0 {
			sb.WriteString(fmt.Sprintf("Labels: %s\n", strings.Join(d.Labels, ", ")))
		}

		if len(d.Assignees) > 0 {
			sb.WriteString(fmt.Sprintf("Assignees: %s\n", strings.Join(d.Assignees, ", ")))
		}

		if d.IsPR {
			sb.WriteString(fmt.Sprintf("\nPR Details:\n"))
			sb.WriteString(fmt.Sprintf("  Files changed: %d\n", d.ChangedFiles))
			sb.WriteString(fmt.Sprintf("  Lines: +%d/-%d\n", d.Additions, d.Deletions))
			sb.WriteString(fmt.Sprintf("  Review state: %s\n", d.ReviewState))
		}
	}

	sb.WriteString(fmt.Sprintf("\nHeuristic category: %s\n", item.Category.Display()))
	sb.WriteString(fmt.Sprintf("Suggested action: %s\n", item.ActionNeeded))

	return sb.String()
}

// AnalyzeBatch analyzes multiple items (for efficiency)
func (l *LLMClient) AnalyzeBatch(items []PrioritizedItem, maxItems int) error {
	count := min(len(items), maxItems)

	for i := 0; i < count; i++ {
		analysis, err := l.Analyze(&items[i])
		if err != nil {
			// Log but continue
			fmt.Printf("Warning: failed to analyze item %d: %v\n", i, err)
			continue
		}
		items[i].Analysis = analysis
	}

	return nil
}

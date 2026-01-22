package output

import (
	"encoding/json"
	"io"

	"github.com/hal/triage/internal/triage"
)

// JSONFormatter formats output as JSON
type JSONFormatter struct {
	Pretty bool
}

// Format outputs prioritized items as JSON
func (f *JSONFormatter) Format(items []triage.PrioritizedItem, w io.Writer) error {
	encoder := json.NewEncoder(w)
	if f.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(items)
}

// FormatSummary outputs a summary as JSON
func (f *JSONFormatter) FormatSummary(summary triage.Summary, w io.Writer) error {
	encoder := json.NewEncoder(w)
	if f.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(summary)
}

// JSONOutput wraps the items with metadata for JSON output
type JSONOutput struct {
	Items   []triage.PrioritizedItem `json:"items"`
	Summary triage.Summary           `json:"summary"`
}

// FormatWithSummary outputs items and summary together
func (f *JSONFormatter) FormatWithSummary(items []triage.PrioritizedItem, w io.Writer) error {
	output := JSONOutput{
		Items:   items,
		Summary: triage.Summarize(items),
	}

	encoder := json.NewEncoder(w)
	if f.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(output)
}

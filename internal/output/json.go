package output

import (
	"encoding/json"
	"io"

	"github.com/hal/priority/internal/priority"
)

// JSONFormatter formats output as JSON
type JSONFormatter struct {
	Pretty bool
}

// Format outputs prioritized items as JSON
func (f *JSONFormatter) Format(items []priority.PrioritizedItem, w io.Writer) error {
	encoder := json.NewEncoder(w)
	if f.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(items)
}

// FormatSummary outputs a summary as JSON
func (f *JSONFormatter) FormatSummary(summary priority.Summary, w io.Writer) error {
	encoder := json.NewEncoder(w)
	if f.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(summary)
}

// JSONOutput wraps the items with metadata for JSON output
type JSONOutput struct {
	Items   []priority.PrioritizedItem `json:"items"`
	Summary priority.Summary           `json:"summary"`
}

// FormatWithSummary outputs items and summary together
func (f *JSONFormatter) FormatWithSummary(items []priority.PrioritizedItem, w io.Writer) error {
	output := JSONOutput{
		Items:   items,
		Summary: priority.Summarize(items),
	}

	encoder := json.NewEncoder(w)
	if f.Pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(output)
}

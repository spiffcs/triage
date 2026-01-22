package output

import (
	"io"

	"github.com/hal/triage/internal/triage"
)

// Format represents the output format
type Format string

const (
	FormatTable    Format = "table"
	FormatJSON     Format = "json"
	FormatMarkdown Format = "markdown"
)

// Formatter defines the interface for output formatters
type Formatter interface {
	Format(items []triage.PrioritizedItem, w io.Writer) error
	FormatSummary(summary triage.Summary, w io.Writer) error
}

// NewFormatter creates a formatter for the specified format
func NewFormatter(format Format) Formatter {
	switch format {
	case FormatJSON:
		return &JSONFormatter{}
	case FormatMarkdown:
		return &MarkdownFormatter{}
	default:
		return &TableFormatter{}
	}
}

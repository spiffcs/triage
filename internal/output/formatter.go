package output

import (
	"io"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/triage"
)

// Format represents the output format
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
)

// Formatter defines the interface for output formatters
type Formatter interface {
	Format(items []triage.PrioritizedItem, w io.Writer) error
}

// NewFormatter creates a formatter for the specified format
func NewFormatter(format Format) Formatter {
	return NewFormatterWithWeights(format, config.DefaultScoreWeights(), "")
}

// NewFormatterWithWeights creates a formatter with custom score weights
func NewFormatterWithWeights(format Format, weights config.ScoreWeights, currentUser string) Formatter {
	switch format {
	case FormatJSON:
		return &JSONFormatter{}
	default:
		return &TableFormatter{
			HotTopicThreshold: weights.HotTopicThreshold,
			PRSizeXS:          weights.PRSizeXS,
			PRSizeS:           weights.PRSizeS,
			PRSizeM:           weights.PRSizeM,
			PRSizeL:           weights.PRSizeL,
			CurrentUser:       currentUser,
		}
	}
}

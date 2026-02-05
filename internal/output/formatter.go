package output

import (
	"io"

	"github.com/spiffcs/triage/config"
	"github.com/spiffcs/triage/internal/triage"
)

// Column width constants for table/list display
const (
	ColPriority = 10
	ColType     = 5
	ColAuthor   = 15
	ColAssigned = 12
	ColCI       = 2
	ColRepo     = 26
	ColTitle    = 40
	ColStatus   = 20
	ColAge      = 5
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

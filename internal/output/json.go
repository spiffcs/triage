package output

import (
	"encoding/json"
	"io"

	"github.com/spiffcs/triage/internal/triage"
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

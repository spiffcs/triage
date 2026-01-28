// Package duration provides parsing for human-readable duration strings.
package duration

import (
	"fmt"
	"time"
)

// Parse parses human-readable durations like "1w", "30d", "6mo".
// It returns the time that is the given duration in the past from now.
func Parse(s string) (time.Time, error) {
	now := time.Now()

	// Handle common patterns
	var d time.Duration
	var n int
	var unit string

	if _, err := fmt.Sscanf(s, "%d%s", &n, &unit); err != nil {
		return time.Time{}, fmt.Errorf("invalid duration format: %s (use e.g., 1w, 30d, 6mo)", s)
	}

	switch unit {
	case "m", "min", "mins":
		d = time.Duration(n) * time.Minute
	case "h", "hr", "hrs", "hour", "hours":
		d = time.Duration(n) * time.Hour
	case "d", "day", "days":
		d = time.Duration(n) * 24 * time.Hour
	case "w", "wk", "wks", "week", "weeks":
		d = time.Duration(n) * 7 * 24 * time.Hour
	case "mo", "month", "months":
		d = time.Duration(n) * 30 * 24 * time.Hour
	case "y", "yr", "yrs", "year", "years":
		d = time.Duration(n) * 365 * 24 * time.Hour
	default:
		return time.Time{}, fmt.Errorf("unknown duration unit: %s", unit)
	}

	return now.Add(-d), nil
}

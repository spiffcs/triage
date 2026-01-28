package format

import "fmt"

// PRSize represents a T-shirt size category for PR changes.
type PRSize string

const (
	PRSizeXS PRSize = "XS"
	PRSizeS  PRSize = "S"
	PRSizeM  PRSize = "M"
	PRSizeL  PRSize = "L"
	PRSizeXL PRSize = "XL"
)

// PRSizeThresholds holds the thresholds for determining PR size.
type PRSizeThresholds struct {
	XS int // <= XS is extra small
	S  int // <= S is small
	M  int // <= M is medium
	L  int // <= L is large
	// > L is extra large
}

// PRSizeResult contains the calculated PR size and formatted strings.
type PRSizeResult struct {
	Size      PRSize
	Formatted string // e.g., "XS+10/-5"
}

// CalculatePRSize determines the T-shirt size of a PR based on total changes.
func CalculatePRSize(additions, deletions int, thresholds PRSizeThresholds) PRSizeResult {
	total := additions + deletions

	var size PRSize
	switch {
	case total <= thresholds.XS:
		size = PRSizeXS
	case total <= thresholds.S:
		size = PRSizeS
	case total <= thresholds.M:
		size = PRSizeM
	case total <= thresholds.L:
		size = PRSizeL
	default:
		size = PRSizeXL
	}

	formatted := fmt.Sprintf("%s+%d/-%d", size, additions, deletions)

	return PRSizeResult{
		Size:      size,
		Formatted: formatted,
	}
}

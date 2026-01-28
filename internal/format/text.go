// Package format provides shared text formatting utilities for terminal output.
package format

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// ansiRegex matches ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripAnsi removes ANSI escape sequences from a string.
func StripAnsi(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// DisplayWidth returns the visible width of a string in terminal columns,
// accounting for wide characters like emojis (which take 2 columns)
// and stripping ANSI escape sequences.
func DisplayWidth(s string) int {
	plain := StripAnsi(s)
	width := 0
	runes := []rune(plain)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		// Check for emoji presentation sequence: base emoji + U+FE0F (VS16)
		// These display as 2 columns in modern terminals
		if i+1 < len(runes) && runes[i+1] == '\uFE0F' {
			width += 2
			i++ // skip the variation selector
			continue
		}
		// Skip standalone variation selectors (shouldn't happen but be safe)
		if r == '\uFE0F' {
			continue
		}
		width += runewidth.RuneWidth(r)
	}
	return width
}

// TruncateToWidth truncates a string to fit within maxWidth display columns.
// It handles ANSI escape sequences by preserving them in the output.
// Returns the truncated string and its visible width.
// If truncation occurs, "..." is appended along with an ANSI reset code.
func TruncateToWidth(s string, maxWidth int) (string, int) {
	width := DisplayWidth(s)

	// If it fits, return as-is
	if width <= maxWidth {
		return s, width
	}

	// Need to truncate - leave room for "..."
	targetWidth := maxWidth - 3
	if targetWidth < 0 {
		targetWidth = 0
	}

	// Find all ANSI sequences and their positions in the original string
	matches := ansiRegex.FindAllStringIndex(s, -1)

	// Build result by walking through the string
	var result strings.Builder
	visibleWidth := 0
	pos := 0
	matchIdx := 0

	for pos < len(s) && visibleWidth < targetWidth {
		// Check if current position is the start of an ANSI sequence
		if matchIdx < len(matches) && pos == matches[matchIdx][0] {
			// Include the ANSI sequence without counting its width
			result.WriteString(s[matches[matchIdx][0]:matches[matchIdx][1]])
			pos = matches[matchIdx][1]
			matchIdx++
			continue
		}

		// Get the next rune
		r, size := utf8.DecodeRuneInString(s[pos:])

		// Check for emoji presentation sequence: base + U+FE0F (VS16)
		nextPos := pos + size
		if nextPos < len(s) {
			nextR, nextSize := utf8.DecodeRuneInString(s[nextPos:])
			if nextR == '\uFE0F' {
				// Emoji + VS16 = 2 columns
				if visibleWidth+2 > targetWidth {
					break
				}
				result.WriteString(s[pos : nextPos+nextSize])
				visibleWidth += 2
				pos = nextPos + nextSize
				continue
			}
		}

		// Skip standalone variation selectors
		if r == '\uFE0F' {
			pos += size
			continue
		}

		rw := runewidth.RuneWidth(r)

		// Check if adding this rune would exceed our target
		if visibleWidth+rw > targetWidth {
			break
		}

		result.WriteString(s[pos : pos+size])
		visibleWidth += rw
		pos += size
	}

	// Add ellipsis and reset code (in case we were in the middle of a color)
	result.WriteString("...\033[0m")

	return result.String(), maxWidth
}

// PadRight pads a string with spaces to reach the target visible width.
func PadRight(s string, visibleWidth, targetWidth int) string {
	if visibleWidth >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-visibleWidth)
}

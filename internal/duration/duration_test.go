package duration

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input    string
		wantErr  bool
		checkAge time.Duration
	}{
		{"1d", false, 24 * time.Hour},
		{"1w", false, 7 * 24 * time.Hour},
		{"30d", false, 30 * 24 * time.Hour},
		{"1mo", false, 30 * 24 * time.Hour},
		{"1h", false, time.Hour},
		{"invalid", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Verify the result is approximately the expected age from now
			age := time.Since(result)
			// Allow 1 second tolerance for test execution time
			if age < tt.checkAge-time.Second || age > tt.checkAge+time.Second {
				t.Errorf("expected age ~%v, got %v", tt.checkAge, age)
			}
		})
	}
}

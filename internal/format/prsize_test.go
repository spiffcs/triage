package format

import (
	"testing"
)

func TestCalculatePRSize(t *testing.T) {
	thresholds := PRSizeThresholds{
		XS: 10,
		S:  50,
		M:  200,
		L:  500,
	}

	tests := []struct {
		name         string
		additions    int
		deletions    int
		expectedSize PRSize
		expectedFmt  string
	}{
		{"extra small", 5, 3, PRSizeXS, "XS+5/-3"},
		{"small lower bound", 10, 0, PRSizeXS, "XS+10/-0"},
		{"small", 30, 15, PRSizeS, "S+30/-15"},
		{"medium", 100, 80, PRSizeM, "M+100/-80"},
		{"large", 300, 150, PRSizeL, "L+300/-150"},
		{"extra large", 400, 200, PRSizeXL, "XL+400/-200"},
		{"zero changes", 0, 0, PRSizeXS, "XS+0/-0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculatePRSize(tt.additions, tt.deletions, thresholds)
			if result.Size != tt.expectedSize {
				t.Errorf("CalculatePRSize(%d, %d) size = %q, want %q",
					tt.additions, tt.deletions, result.Size, tt.expectedSize)
			}
			if result.Formatted != tt.expectedFmt {
				t.Errorf("CalculatePRSize(%d, %d) formatted = %q, want %q",
					tt.additions, tt.deletions, result.Formatted, tt.expectedFmt)
			}
		})
	}
}

func TestPRSizeEdgeCases(t *testing.T) {
	thresholds := PRSizeThresholds{XS: 10, S: 50, M: 200, L: 500}

	// Test boundary conditions
	tests := []struct {
		total        int
		expectedSize PRSize
	}{
		{10, PRSizeXS},  // exactly at XS threshold
		{11, PRSizeS},   // just above XS
		{50, PRSizeS},   // exactly at S threshold
		{51, PRSizeM},   // just above S
		{200, PRSizeM},  // exactly at M threshold
		{201, PRSizeL},  // just above M
		{500, PRSizeL},  // exactly at L threshold
		{501, PRSizeXL}, // just above L
	}

	for _, tt := range tests {
		result := CalculatePRSize(tt.total, 0, thresholds)
		if result.Size != tt.expectedSize {
			t.Errorf("Total %d: got size %q, want %q", tt.total, result.Size, tt.expectedSize)
		}
	}
}

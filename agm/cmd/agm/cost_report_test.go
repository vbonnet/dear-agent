package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		tokensIn  int64
		tokensOut int64
		wantMin   float64
		wantMax   float64
	}{
		{
			name:      "zero tokens opus",
			model:     "opus",
			tokensIn:  0,
			tokensOut: 0,
			wantMin:   0,
			wantMax:   0,
		},
		{
			name:      "1M opus input",
			model:     "opus",
			tokensIn:  1_000_000,
			tokensOut: 0,
			wantMin:   14.99,
			wantMax:   15.01,
		},
		{
			name:      "1M opus output",
			model:     "opus",
			tokensIn:  0,
			tokensOut: 1_000_000,
			wantMin:   74.99,
			wantMax:   75.01,
		},
		{
			// Sonnet is the new default; its price is ~5× cheaper than Opus.
			name:      "1M sonnet output",
			model:     "sonnet",
			tokensIn:  0,
			tokensOut: 1_000_000,
			wantMin:   14.99,
			wantMax:   15.01,
		},
		{
			name:      "unknown model returns 0",
			model:     "nonexistent",
			tokensIn:  1_000_000,
			tokensOut: 1_000_000,
			wantMin:   0,
			wantMax:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateCost(tt.model, tt.tokensIn, tt.tokensOut)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("estimateCost(%q, %d, %d) = %f, want between %f and %f",
					tt.model, tt.tokensIn, tt.tokensOut, got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestBuildCostRow(t *testing.T) {
	now := time.Now()
	start := now.Add(-2 * time.Hour)

	t.Run("with cost tracking and model", func(t *testing.T) {
		m := &manifest.Manifest{
			Name:  "test-session",
			Model: "sonnet",
			CostTracking: &manifest.CostTracking{
				TokensIn:  100_000,
				TokensOut: 50_000,
				StartTime: start,
				EndTime:   now,
			},
		}

		row := buildCostRow(m)

		if row.Name != "test-session" {
			t.Errorf("Name = %q, want %q", row.Name, "test-session")
		}
		if row.Model != "sonnet" {
			t.Errorf("Model = %q, want %q", row.Model, "sonnet")
		}
		if row.TokensIn != 100_000 {
			t.Errorf("TokensIn = %d, want %d", row.TokensIn, 100_000)
		}
		if row.TokensOut != 50_000 {
			t.Errorf("TokensOut = %d, want %d", row.TokensOut, 50_000)
		}
		if row.Duration < 1*time.Hour || row.Duration > 3*time.Hour {
			t.Errorf("Duration = %v, want ~2h", row.Duration)
		}
		if row.EstimatedCost == 0 {
			t.Error("EstimatedCost should not be zero with non-zero tokens on known model")
		}
	})

	t.Run("prices per-model not hardcoded to opus", func(t *testing.T) {
		// Regression guard for the old hardcoded-opus-pricing bug.
		// 1M Sonnet input should cost ~$3, not ~$15.
		sonnetRow := buildCostRow(&manifest.Manifest{
			Name:  "sonnet-session",
			Model: "sonnet",
			CostTracking: &manifest.CostTracking{
				TokensIn: 1_000_000,
			},
		})
		opusRow := buildCostRow(&manifest.Manifest{
			Name:  "opus-session",
			Model: "opus",
			CostTracking: &manifest.CostTracking{
				TokensIn: 1_000_000,
			},
		})
		if sonnetRow.EstimatedCost >= opusRow.EstimatedCost {
			t.Errorf("sonnet should be cheaper than opus for same tokens: sonnet=%.2f opus=%.2f",
				sonnetRow.EstimatedCost, opusRow.EstimatedCost)
		}
		if sonnetRow.EstimatedCost > 5.0 {
			t.Errorf("sonnet 1M input should cost ~$3, got $%.2f (likely still using opus pricing)",
				sonnetRow.EstimatedCost)
		}
	})

	t.Run("prefers LastKnownModel over Model", func(t *testing.T) {
		m := &manifest.Manifest{
			Name:           "drift-session",
			Model:          "opus",   // configured
			LastKnownModel: "sonnet", // actually observed
			CostTracking: &manifest.CostTracking{
				TokensIn: 1_000_000,
			},
		}
		row := buildCostRow(m)
		if row.Model != "sonnet" {
			t.Errorf("expected LastKnownModel to win: got Model=%q", row.Model)
		}
	})

	t.Run("fallback to LastKnownCost", func(t *testing.T) {
		m := &manifest.Manifest{
			Name:          "fallback-session",
			LastKnownCost: 5.50,
			CreatedAt:     start,
			UpdatedAt:     now,
		}

		row := buildCostRow(m)

		if row.EstimatedCost != 5.50 {
			t.Errorf("EstimatedCost = %f, want 5.50", row.EstimatedCost)
		}
		if row.Duration < 1*time.Hour || row.Duration > 3*time.Hour {
			t.Errorf("Duration = %v, want ~2h (from CreatedAt/UpdatedAt fallback)", row.Duration)
		}
	})

	t.Run("no cost data", func(t *testing.T) {
		m := &manifest.Manifest{
			Name: "empty-session",
		}

		row := buildCostRow(m)

		if row.EstimatedCost != 0 {
			t.Errorf("EstimatedCost = %f, want 0", row.EstimatedCost)
		}
	})

	t.Run("unknown model flagged as unpriced", func(t *testing.T) {
		m := &manifest.Manifest{
			Name:  "mystery",
			Model: "unknown-xyz",
			CostTracking: &manifest.CostTracking{
				TokensIn: 1_000_000,
			},
		}
		row := buildCostRow(m)
		if row.CostIsKnown {
			t.Error("expected CostIsKnown=false for unknown model")
		}
	})
}

func TestCostFormatTokens(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "—"},
		{500, "500"},
		{1_500, "1.5K"},
		{1_500_000, "1.5M"},
	}

	for _, tt := range tests {
		got := costFormatTokens(tt.input)
		if got != tt.want {
			t.Errorf("costFormatTokens(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCostFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{0, "—"},
		{30 * time.Minute, "30m"},
		{90 * time.Minute, "1h30m"},
		{2*time.Hour + 5*time.Minute, "2h05m"},
	}

	for _, tt := range tests {
		got := costFormatDuration(tt.input)
		if got != tt.want {
			t.Errorf("costFormatDuration(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0, "—"},
		{1.5, "$1.50"},
		{99.999, "$100.00"},
	}

	for _, tt := range tests {
		got := formatCost(tt.input)
		if got != tt.want {
			t.Errorf("formatCost(%f) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatModelCell(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "?"},
		{"opus", "opus"},
		{"sonnet", "sonnet"},
		{"claude-opus-4-6[1m]", "opus"},
		{"claude-sonnet-4-6", "sonnet"},
		{"claude-haiku-4-5", "haiku"},
	}
	for _, tt := range tests {
		got := formatModelCell(tt.in)
		if got != tt.want {
			t.Errorf("formatModelCell(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPrintCostReport_ReturnsTotalCost(t *testing.T) {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	manifests := []*manifest.Manifest{
		{Name: "session-1", LastKnownCost: 10.50},
		{Name: "session-2", LastKnownCost: 25.00},
		{Name: "session-3"}, // no cost
	}

	totalCost := printCostReport(cmd, manifests)

	if totalCost != 35.50 {
		t.Errorf("printCostReport returned totalCost = %f, want 35.50", totalCost)
	}

	output := buf.String()
	if !strings.Contains(output, "session-1") {
		t.Error("output should contain session-1")
	}
	if !strings.Contains(output, "$35.50") {
		t.Errorf("output should contain total $35.50, got:\n%s", output)
	}
	if !strings.Contains(output, "MODEL") {
		t.Error("output should include MODEL column header")
	}
}

func TestFormatMetricsCost(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0, "N/A"},
		{-1, "N/A"},
		{1.5, "$1.50"},
		{99.999, "$100.00"},
	}

	for _, tt := range tests {
		got := formatMetricsCost(tt.input)
		if got != tt.want {
			t.Errorf("formatMetricsCost(%f) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

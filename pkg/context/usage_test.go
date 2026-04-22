package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestRegistry(t *testing.T) *Registry {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "test-model"
    provider: "test"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	return registry
}

func createTestRegistryNoDefault(t *testing.T) *Registry {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "models.yaml")

	yamlContent := `models:
  - model_id: "only-model"
    provider: "test"
    max_context_tokens: 200000
    sweet_spot_threshold: 0.60
    warning_threshold: 0.70
    danger_threshold: 0.80
    critical_threshold: 0.90
    benchmark_sources: []
    notes: ""
    confidence: "HIGH"
    last_updated: "2026-03-18T00:00:00Z"
`

	err := os.WriteFile(registryPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	registry, err := loadRegistry(registryPath)
	require.NoError(t, err)

	return registry
}

func TestCalculateZone(t *testing.T) {
	registry := createTestRegistry(t)
	calculator := NewCalculator(registry)

	tests := []struct {
		name       string
		percentage float64
		expected   Zone
	}{
		{"safe zone", 50.0, ZoneSafe},
		{"at sweet spot", 60.0, ZoneSafe},
		{"warning zone", 75.0, ZoneWarning},
		{"danger zone", 85.0, ZoneDanger},
		{"critical zone", 95.0, ZoneCritical},
		{"at warning boundary", 70.0, ZoneWarning},
		{"at danger boundary", 80.0, ZoneDanger},
		{"at critical boundary", 90.0, ZoneCritical},
		{"zero percent", 0.0, ZoneSafe},
		{"100 percent", 100.0, ZoneCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zone, err := calculator.CalculateZone(tt.percentage, "test-model")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, zone)
		})
	}
}

func TestCalculateZoneUnknownModel(t *testing.T) {
	registry := createTestRegistryNoDefault(t)
	calculator := NewCalculator(registry)

	zone, err := calculator.CalculateZone(50.0, "nonexistent-model")
	assert.Error(t, err)
	assert.Equal(t, ZoneSafe, zone) // Returns ZoneSafe on error
	assert.Contains(t, err.Error(), "model not found")
}

func TestShouldCompact(t *testing.T) {
	registry := createTestRegistry(t)
	calculator := NewCalculator(registry)

	tests := []struct {
		name          string
		percentage    float64
		phaseState    PhaseState
		shouldCompact bool
		description   string
	}{
		{
			"safe at phase start",
			50.0,
			PhaseStart,
			false,
			"Below warning threshold",
		},
		{
			"warning at phase start",
			70.0,
			PhaseStart,
			true,
			"At warning threshold during phase start - compact proactively",
		},
		{
			"danger at phase middle",
			80.0,
			PhaseMiddle,
			true,
			"At danger threshold during mid-phase",
		},
		{
			"warning at phase middle",
			75.0,
			PhaseMiddle,
			false,
			"Below danger threshold during mid-phase",
		},
		{
			"danger at phase end",
			82.0,
			PhaseEnd,
			false,
			"Below danger+5% threshold at phase end",
		},
		{
			"danger+5 at phase end",
			86.0,
			PhaseEnd,
			true,
			"At danger+5% threshold at phase end",
		},
		{
			"default phase state (unknown)",
			80.0,
			PhaseState("unknown"),
			true,
			"Unknown phase state falls through to danger threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &Usage{
				TotalTokens:    200000,
				UsedTokens:     int(float64(200000) * tt.percentage / 100.0),
				PercentageUsed: tt.percentage,
				ModelID:        "test-model",
			}

			shouldCompact, err := calculator.ShouldCompact(usage, tt.phaseState)
			require.NoError(t, err)
			assert.Equal(t, tt.shouldCompact, shouldCompact, tt.description)
		})
	}
}

func TestShouldCompactUnknownModel(t *testing.T) {
	registry := createTestRegistryNoDefault(t)
	calculator := NewCalculator(registry)

	usage := &Usage{
		TotalTokens:    200000,
		UsedTokens:     150000,
		PercentageUsed: 75.0,
		ModelID:        "nonexistent-model",
	}

	_, err := calculator.ShouldCompact(usage, PhaseMiddle)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model not found")
}

func TestGetCompactionRecommendation(t *testing.T) {
	registry := createTestRegistry(t)
	calculator := NewCalculator(registry)

	tests := []struct {
		name        string
		percentage  float64
		phaseState  PhaseState
		recommended bool
		urgency     string
	}{
		{"safe zone", 50.0, PhaseMiddle, false, "low"},
		{"warning at start", 70.0, PhaseStart, true, "medium"},
		{"danger at middle", 85.0, PhaseMiddle, true, "high"},
		{"critical", 95.0, PhaseMiddle, true, "critical"},
		{"danger at phase start", 85.0, PhaseStart, true, "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &Usage{
				TotalTokens:    200000,
				UsedTokens:     int(float64(200000) * tt.percentage / 100.0),
				PercentageUsed: tt.percentage,
				ModelID:        "test-model",
			}

			rec, err := calculator.GetCompactionRecommendation(usage, tt.phaseState)
			require.NoError(t, err)
			assert.NotNil(t, rec)
			assert.Equal(t, tt.recommended, rec.Recommended)
			assert.Equal(t, tt.urgency, rec.Urgency)
			assert.NotEmpty(t, rec.Reason)
		})
	}
}

func TestGetCompactionRecommendationNotRecommended(t *testing.T) {
	registry := createTestRegistry(t)
	calculator := NewCalculator(registry)

	usage := &Usage{
		TotalTokens:    200000,
		UsedTokens:     40000,
		PercentageUsed: 20.0,
		ModelID:        "test-model",
	}

	rec, err := calculator.GetCompactionRecommendation(usage, PhaseMiddle)
	require.NoError(t, err)
	assert.False(t, rec.Recommended)
	assert.Equal(t, "low", rec.Urgency)
	assert.Equal(t, "N/A", rec.EstimatedReduction)
	assert.Equal(t, "Context usage within acceptable limits", rec.Reason)
}

func TestGetCompactionRecommendationUnknownModel(t *testing.T) {
	registry := createTestRegistryNoDefault(t)
	calculator := NewCalculator(registry)

	usage := &Usage{
		TotalTokens:    200000,
		UsedTokens:     150000,
		PercentageUsed: 75.0,
		ModelID:        "nonexistent-model",
	}

	_, err := calculator.GetCompactionRecommendation(usage, PhaseMiddle)
	assert.Error(t, err)
}

func TestGetCompactionRecommendationReasons(t *testing.T) {
	registry := createTestRegistry(t)
	calculator := NewCalculator(registry)

	t.Run("critical zone reason", func(t *testing.T) {
		usage := &Usage{
			TotalTokens:    200000,
			UsedTokens:     190000,
			PercentageUsed: 95.0,
			ModelID:        "test-model",
		}
		rec, err := calculator.GetCompactionRecommendation(usage, PhaseMiddle)
		require.NoError(t, err)
		assert.Contains(t, rec.Reason, "near capacity")
		assert.Equal(t, "15-25%", rec.EstimatedReduction)
	})

	t.Run("danger zone at phase start reason", func(t *testing.T) {
		usage := &Usage{
			TotalTokens:    200000,
			UsedTokens:     170000,
			PercentageUsed: 85.0,
			ModelID:        "test-model",
		}
		rec, err := calculator.GetCompactionRecommendation(usage, PhaseStart)
		require.NoError(t, err)
		assert.Contains(t, rec.Reason, "phase start")
	})

	t.Run("danger zone at phase middle reason", func(t *testing.T) {
		usage := &Usage{
			TotalTokens:    200000,
			UsedTokens:     170000,
			PercentageUsed: 85.0,
			ModelID:        "test-model",
		}
		rec, err := calculator.GetCompactionRecommendation(usage, PhaseMiddle)
		require.NoError(t, err)
		assert.Contains(t, rec.Reason, "utilization is high")
	})

	t.Run("warning zone reason", func(t *testing.T) {
		usage := &Usage{
			TotalTokens:    200000,
			UsedTokens:     140000,
			PercentageUsed: 70.0,
			ModelID:        "test-model",
		}
		rec, err := calculator.GetCompactionRecommendation(usage, PhaseStart)
		require.NoError(t, err)
		assert.Contains(t, rec.Reason, "consider compaction")
	})
}

func TestCheck(t *testing.T) {
	registry := createTestRegistry(t)
	calculator := NewCalculator(registry)

	usage := &Usage{
		TotalTokens:    200000,
		UsedTokens:     150000,
		PercentageUsed: 75.0,
		ModelID:        "test-model",
		Source:         "test",
	}

	result, err := calculator.Check(usage, PhaseMiddle)
	require.NoError(t, err)
	assert.NotNil(t, result)

	assert.Equal(t, ZoneWarning, result.Zone)
	assert.Equal(t, 75.0, result.Percentage)
	assert.False(t, result.ShouldCompact) // 75% < 80% danger threshold
	assert.Equal(t, "test-model", result.ModelID)
	assert.Equal(t, "test", result.Source)

	// Check thresholds
	assert.Equal(t, 120000, result.Thresholds.SweetSpotTokens)
	assert.Equal(t, 140000, result.Thresholds.WarningTokens)
	assert.Equal(t, 160000, result.Thresholds.DangerTokens)
	assert.Equal(t, 180000, result.Thresholds.CriticalTokens)
}

func TestCheckUnknownModel(t *testing.T) {
	registry := createTestRegistryNoDefault(t)
	calculator := NewCalculator(registry)

	usage := &Usage{
		TotalTokens:    200000,
		UsedTokens:     150000,
		PercentageUsed: 75.0,
		ModelID:        "nonexistent-model",
		Source:         "test",
	}

	_, err := calculator.Check(usage, PhaseMiddle)
	assert.Error(t, err)
}

func TestCheckShouldCompactTrue(t *testing.T) {
	registry := createTestRegistry(t)
	calculator := NewCalculator(registry)

	usage := &Usage{
		TotalTokens:    200000,
		UsedTokens:     170000,
		PercentageUsed: 85.0,
		ModelID:        "test-model",
		Source:         "test",
	}

	result, err := calculator.Check(usage, PhaseMiddle)
	require.NoError(t, err)
	assert.Equal(t, ZoneDanger, result.Zone)
	assert.True(t, result.ShouldCompact)
	assert.NotZero(t, result.Timestamp)
}

func TestFormatZone(t *testing.T) {
	tests := []struct {
		zone     Zone
		expected string
	}{
		{ZoneSafe, "\xe2\x9c\x85 safe"},
		{ZoneWarning, "\xe2\x9a\xa0\xef\xb8\x8f  warning"},
		{ZoneDanger, "\xf0\x9f\x94\xa5 danger"},
		{ZoneCritical, "\xf0\x9f\x9a\xa8 critical"},
		{Zone("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.zone), func(t *testing.T) {
			result := FormatZone(tt.zone)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatPercentage(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{72.5, "72.5%"},
		{100.0, "100.0%"},
		{0.0, "0.0%"},
		{50.123, "50.1%"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatPercentage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		used     int
		total    int
		expected string
	}{
		{144000, 200000, "144K/200K"},
		{600000, 1000000, "600K/1.0M"},
		{500, 1000, "500/1K"},
		{1500000, 2000000, "1.5M/2.0M"},
		{0, 200000, "0/200K"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatTokens(tt.used, tt.total)
			assert.Equal(t, tt.expected, result)
		})
	}
}

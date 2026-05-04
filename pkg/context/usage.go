package context

import (
	"fmt"
	"time"
)

// Calculator performs context usage calculations and compaction recommendations.
type Calculator struct {
	registry *Registry
}

// NewCalculator creates a new calculator with the given registry.
func NewCalculator(registry *Registry) *Calculator {
	return &Calculator{
		registry: registry,
	}
}

// CalculateZone determines the current zone based on usage percentage and model thresholds.
func (c *Calculator) CalculateZone(percentage float64, modelID string) (Zone, error) {
	model := c.registry.GetModel(modelID)
	if model == nil {
		return ZoneSafe, fmt.Errorf("model not found: %s", modelID)
	}

	// Convert percentage to decimal (0.0-1.0)
	pct := percentage / 100.0

	switch {
	case pct >= model.CriticalThreshold:
		return ZoneCritical, nil
	case pct >= model.DangerThreshold:
		return ZoneDanger, nil
	case pct >= model.WarningThreshold:
		return ZoneWarning, nil
	}

	return ZoneSafe, nil
}

// ShouldCompact determines if compaction is recommended based on usage, model, and phase state.
//
// Smart Compaction Logic:
// - Phase Start: Compact at warning threshold (new phase adds context)
// - Phase Middle: Compact at danger threshold (standard)
// - Phase End: Can push +5% beyond danger (compact after completion)
func (c *Calculator) ShouldCompact(usage *Usage, phaseState PhaseState) (bool, error) {
	thresholds, err := c.registry.GetThresholds(usage.ModelID)
	if err != nil {
		return false, err
	}

	// Convert usage percentage to decimal (0.0-1.0)
	pct := usage.PercentageUsed / 100.0

	switch phaseState {
	case PhaseStart:
		// At phase start, compact if at warning (new phase adds context)
		// This prevents hitting danger zone mid-phase
		return pct >= thresholds.WarningPercentage, nil

	case PhaseEnd:
		// Near end, can push +5% beyond danger (compact after completion)
		// This allows finishing current work without interruption
		dangerPlusTolerance := thresholds.DangerPercentage + 0.05
		return pct >= dangerPlusTolerance, nil

	case PhaseMiddle:
		fallthrough
	default:
		// Mid-phase: standard danger threshold
		return pct >= thresholds.DangerPercentage, nil
	}
}

// GetCompactionRecommendation provides detailed compaction recommendation with reasoning.
func (c *Calculator) GetCompactionRecommendation(usage *Usage, phaseState PhaseState) (*CompactionRecommendation, error) {
	shouldCompact, err := c.ShouldCompact(usage, phaseState)
	if err != nil {
		return nil, err
	}

	if !shouldCompact {
		return &CompactionRecommendation{
			Recommended:        false,
			Reason:             "Context usage within acceptable limits",
			Urgency:            "low",
			EstimatedReduction: "N/A",
		}, nil
	}

	// Determine urgency based on zone
	zone, _ := c.CalculateZone(usage.PercentageUsed, usage.ModelID)
	var urgency, reason string

	switch zone {
	case ZoneSafe:
		urgency = "low"
		reason = "Compaction recommended but not urgent"
	case ZoneCritical:
		urgency = "critical"
		reason = fmt.Sprintf("Context at %.1f%% — near capacity. Please compact before continuing.", usage.PercentageUsed)
	case ZoneDanger:
		if phaseState == PhaseStart {
			urgency = "high"
			reason = fmt.Sprintf("Context at %.1f%% at phase start — compaction recommended before new phase adds context.", usage.PercentageUsed)
		} else {
			urgency = "high"
			reason = fmt.Sprintf("Context at %.1f%% — utilization is high. Compaction will improve performance.", usage.PercentageUsed)
		}
	case ZoneWarning:
		urgency = "medium"
		reason = fmt.Sprintf("Context at %.1f%% - consider compaction before next phase.", usage.PercentageUsed)
	}

	return &CompactionRecommendation{
		Recommended:        true,
		Reason:             reason,
		Urgency:            urgency,
		EstimatedReduction: "15-25%", // Based on typical compaction effectiveness
	}, nil
}

// Check performs a complete context check and returns detailed results.
func (c *Calculator) Check(usage *Usage, phaseState PhaseState) (*CheckResult, error) {
	zone, err := c.CalculateZone(usage.PercentageUsed, usage.ModelID)
	if err != nil {
		return nil, err
	}

	shouldCompact, err := c.ShouldCompact(usage, phaseState)
	if err != nil {
		return nil, err
	}

	thresholds, err := c.registry.GetThresholds(usage.ModelID)
	if err != nil {
		return nil, err
	}

	return &CheckResult{
		Zone:          zone,
		Percentage:    usage.PercentageUsed,
		ShouldCompact: shouldCompact,
		Thresholds:    *thresholds,
		ModelID:       usage.ModelID,
		Source:        usage.Source,
		Timestamp:     time.Now(),
	}, nil
}

// FormatZone returns a human-readable zone description with emoji.
func FormatZone(zone Zone) string {
	switch zone {
	case ZoneSafe:
		return "✅ safe"
	case ZoneWarning:
		return "⚠️  warning"
	case ZoneDanger:
		return "🔥 danger"
	case ZoneCritical:
		return "🚨 critical"
	default:
		return string(zone)
	}
}

// FormatPercentage formats a percentage with one decimal place.
func FormatPercentage(pct float64) string {
	return fmt.Sprintf("%.1f%%", pct)
}

// FormatTokens formats token counts in a human-readable way (e.g., "144K/200K").
func FormatTokens(used, total int) string {
	formatToken := func(tokens int) string {
		if tokens >= 1000000 {
			return fmt.Sprintf("%.1fM", float64(tokens)/1000000.0)
		} else if tokens >= 1000 {
			return fmt.Sprintf("%dK", tokens/1000)
		}
		return fmt.Sprintf("%d", tokens)
	}

	return fmt.Sprintf("%s/%s", formatToken(used), formatToken(total))
}

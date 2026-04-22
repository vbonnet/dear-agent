package signals

import (
	"testing"
	"testing/quick"
)

// TestPropertyFuseSignalsEmptyReturnsMinimal verifies that empty signals
// always produce minimal level with baseline confidence.
func TestPropertyFuseSignalsEmptyReturnsMinimal(t *testing.T) {
	confidence, level := fuseSignals([]Signal{})
	if confidence != 0.30 {
		t.Errorf("expected confidence 0.30 for empty signals, got %f", confidence)
	}
	if level != RigorLevelMinimal {
		t.Errorf("expected minimal level for empty signals, got %s", level)
	}
}

// TestPropertyFuseSignalsConfidenceBounded verifies fused confidence is in [0, 1].
func TestPropertyFuseSignalsConfidenceBounded(t *testing.T) {
	f := func(conf, weight float64) bool {
		// Clamp to valid ranges to produce meaningful signals
		if conf < 0 {
			conf = -conf
		}
		if conf > 1.0 {
			conf = 1.0
		}
		if weight < 0 {
			weight = -weight
		}
		if weight > 1.0 {
			weight = 1.0
		}
		if weight == 0 {
			weight = 0.1
		}

		signals := []Signal{
			{
				Type:       SignalTypeKeyword,
				Value:      "test",
				Confidence: conf,
				Weight:     weight,
				Source:     "property test",
			},
		}
		fusedConf, _ := fuseSignals(signals)
		return fusedConf >= 0.0 && fusedConf <= 1.0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFuseSignalsLevelConsistentWithConfidence verifies the level
// returned is consistent with the confidence thresholds.
func TestPropertyFuseSignalsLevelConsistentWithConfidence(t *testing.T) {
	f := func(conf float64) bool {
		// Clamp confidence to [0, 1]
		if conf < 0 {
			conf = -conf
		}
		if conf > 1.0 {
			conf = 1.0
		}

		signals := []Signal{
			{
				Type:       SignalTypeKeyword,
				Value:      "test",
				Confidence: conf,
				Weight:     1.0,
				Source:     "property test",
			},
		}
		fusedConf, level := fuseSignals(signals)

		// Verify level matches documented thresholds
		switch {
		case fusedConf >= 0.85:
			return level == RigorLevelComprehensive
		case fusedConf >= 0.70:
			return level == RigorLevelThorough
		case fusedConf >= 0.50:
			return level == RigorLevelStandard
		default:
			return level == RigorLevelMinimal
		}
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyDetectKeywordSignalsIdempotent verifies calling detectKeywordSignals
// twice on the same input produces the same number of results.
func TestPropertyDetectKeywordSignalsIdempotent(t *testing.T) {
	f := func(text string) bool {
		signals1 := detectKeywordSignals(text)
		signals2 := detectKeywordSignals(text)
		return len(signals1) == len(signals2)
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyDetectKeywordSignalsNonNegativeConfidence verifies all returned
// signals have confidence in [0, 1].
func TestPropertyDetectKeywordSignalsNonNegativeConfidence(t *testing.T) {
	f := func(text string) bool {
		signals := detectKeywordSignals(text)
		for _, s := range signals {
			if s.Confidence < 0.0 || s.Confidence > 1.0 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyDetectFileSignalsDeterministic verifies file signal detection
// is deterministic.
func TestPropertyDetectFileSignalsDeterministic(t *testing.T) {
	f := func(filename string) bool {
		files := []string{filename}
		signals1 := detectFileSignals(files)
		signals2 := detectFileSignals(files)
		return len(signals1) == len(signals2)
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyDetectBeadsSignalsEmptyOnNil verifies nil BeadsTask returns empty signals.
func TestPropertyDetectBeadsSignalsEmptyOnNil(t *testing.T) {
	signals := detectBeadsSignals(nil)
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for nil BeadsTask, got %d", len(signals))
	}
}

// TestPropertyAnalyzeContextDeterministic verifies AnalyzeContext produces
// the same result for the same input.
func TestPropertyAnalyzeContextDeterministic(t *testing.T) {
	f := func(description string) bool {
		ctx := Context{UserDescription: description}
		result1 := AnalyzeContext(ctx)
		result2 := AnalyzeContext(ctx)
		return result1.Confidence == result2.Confidence &&
			result1.SuggestedLevel == result2.SuggestedLevel &&
			result1.ShouldEscalate == result2.ShouldEscalate &&
			result1.UserAction == result2.UserAction
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFormatHoursRoundtrip verifies formatHours produces non-empty output
// for any non-negative float.
func TestPropertyFormatHoursRoundtrip(t *testing.T) {
	f := func(hours float64) bool {
		if hours < 0 {
			hours = -hours
		}
		result := formatHours(hours)
		return len(result) > 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyFormatFloatNonEmpty verifies formatFloat always returns non-empty string.
func TestPropertyFormatFloatNonEmpty(t *testing.T) {
	f := func(val float64) bool {
		result := formatFloat(val)
		return len(result) > 0
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

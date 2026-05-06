package salience

import (
	"strings"
	"testing"
	"time"
)

func TestIngestClassifiesUnsetSalience(t *testing.T) {
	a := New()
	out := a.Ingest(Signal{Kind: KindBuildFailure})
	if !out.Notify {
		t.Errorf("build failure should notify; got %+v", out)
	}
	if out.Signal.Salience != TierCritical {
		t.Errorf("salience = %s, want critical", out.Signal.Salience)
	}
}

func TestIngestRespectsExplicitSalience(t *testing.T) {
	a := New()
	out := a.Ingest(Signal{Kind: KindCosmetic, Salience: TierCritical})
	if !out.Notify {
		t.Errorf("explicit critical should notify even on cosmetic kind: %+v", out)
	}
	if out.Signal.Salience != TierCritical {
		t.Errorf("explicit salience clobbered: %s", out.Signal.Salience)
	}
}

func TestIngestDropsNoise(t *testing.T) {
	a := New()
	out := a.Ingest(Signal{Kind: KindFormatting})
	if out.Notify {
		t.Errorf("formatting should not notify")
	}
	if !out.Suppressed || out.Reason != ReasonNoiseDropped {
		t.Errorf("expected noise_dropped, got reason=%q suppressed=%v",
			out.Reason, out.Suppressed)
	}
}

func TestIngestKeepsNoiseWhenDropDisabled(t *testing.T) {
	a := New()
	a.DropNoise = false
	out := a.Ingest(Signal{Kind: KindFormatting})
	if !out.Notify {
		t.Errorf("with DropNoise=false, formatting should still notify (no budget); got %+v", out)
	}
}

func TestIngestRejectsUnknownKind(t *testing.T) {
	a := New()
	out := a.Ingest(Signal{Kind: "weather"})
	if out.Notify || !out.Suppressed || out.Reason != ReasonInvalidKind {
		t.Errorf("unknown kind should be rejected, got %+v", out)
	}
}

func TestIngestSuppressesWhenBudgetExhausted(t *testing.T) {
	a := New()
	a.Budget = NewNotificationBudget(time.Hour, 1)
	// First low slips through.
	out := a.Ingest(Signal{Kind: KindDocOnly})
	if !out.Notify {
		t.Fatalf("first low should notify: %+v", out)
	}
	// Second exhausts the budget.
	out = a.Ingest(Signal{Kind: KindDocOnly})
	if out.Notify || out.Reason != ReasonBudgetExhausted {
		t.Errorf("second low should be budget_exhausted, got %+v", out)
	}
	// Critical still bypasses.
	out = a.Ingest(Signal{Kind: KindBuildFailure})
	if !out.Notify {
		t.Errorf("critical should bypass budget, got %+v", out)
	}
}

func TestIngestStampsObservedAt(t *testing.T) {
	fixed := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	a := New()
	a.Now = func() time.Time { return fixed }
	out := a.Ingest(Signal{Kind: KindBuildFailure})
	if !out.Signal.ObservedAt.Equal(fixed) {
		t.Errorf("ObservedAt = %v, want %v", out.Signal.ObservedAt, fixed)
	}
}

func TestIngestPreservesObservedAt(t *testing.T) {
	supplied := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	a := New()
	a.Now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	out := a.Ingest(Signal{Kind: KindBuildFailure, ObservedAt: supplied})
	if !out.Signal.ObservedAt.Equal(supplied) {
		t.Errorf("supplied ObservedAt overwritten: got %v", out.Signal.ObservedAt)
	}
}

func TestLoadJSONL(t *testing.T) {
	input := strings.Join([]string{
		`{"kind":"build_failure","subject":"go build"}`,
		``,
		`{"kind":"cosmetic","subject":"trailing space"}`,
		`{"kind":"test_failure","subject":"TestX"}`,
	}, "\n")

	a := New()
	a.Budget = NewNotificationBudget(time.Hour, 5)
	outcomes, err := a.LoadJSONLString(input)
	if err != nil {
		t.Fatalf("LoadJSONL: %v", err)
	}
	if got, want := len(outcomes), 3; got != want {
		t.Fatalf("got %d outcomes, want %d", got, want)
	}
	if !outcomes[0].Notify || outcomes[0].Signal.Salience != TierCritical {
		t.Errorf("build_failure should notify@critical, got %+v", outcomes[0])
	}
	if outcomes[1].Notify || outcomes[1].Reason != ReasonNoiseDropped {
		t.Errorf("cosmetic should be noise_dropped, got %+v", outcomes[1])
	}
	if !outcomes[2].Notify {
		t.Errorf("test_failure should notify, got %+v", outcomes[2])
	}
}

func TestLoadJSONLMalformedLine(t *testing.T) {
	a := New()
	input := `{"kind":"build_failure"}` + "\n" + `{not valid json` + "\n"
	outcomes, err := a.LoadJSONLString(input)
	if err == nil {
		t.Fatal("expected error on malformed JSON")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should mention line 2: %v", err)
	}
	// Partial outcomes: line 1 succeeded.
	if len(outcomes) != 1 {
		t.Errorf("partial outcomes len = %d, want 1", len(outcomes))
	}
}

func TestSummarize(t *testing.T) {
	a := New()
	a.Budget = NewNotificationBudget(time.Hour, 1)

	outcomes := []Outcome{
		a.Ingest(Signal{Kind: KindBuildFailure}),    // notify, critical, bypass
		a.Ingest(Signal{Kind: KindDocOnly}),          // notify, low, slot 1
		a.Ingest(Signal{Kind: KindDocOnly}),          // suppress, budget exhausted
		a.Ingest(Signal{Kind: KindCosmetic}),         // suppress, noise_dropped
	}
	s := Summarize(outcomes)
	if s.Total != 4 {
		t.Errorf("total = %d", s.Total)
	}
	if s.Notified != 2 {
		t.Errorf("notified = %d, want 2", s.Notified)
	}
	if s.Suppressed != 2 {
		t.Errorf("suppressed = %d, want 2", s.Suppressed)
	}
	if s.ByTier[TierCritical] != 1 || s.ByTier[TierLow] != 2 || s.ByTier[TierNoise] != 1 {
		t.Errorf("byTier wrong: %+v", s.ByTier)
	}
	if s.ByReason[ReasonBudgetExhausted] != 1 || s.ByReason[ReasonNoiseDropped] != 1 {
		t.Errorf("byReason wrong: %+v", s.ByReason)
	}
	if got := s.NotifyRatio; got != 0.5 {
		t.Errorf("notifyRatio = %v, want 0.5", got)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	s := Summarize(nil)
	if s.Total != 0 || s.NotifyRatio != 0 {
		t.Errorf("empty summary should be zero-valued; got %+v", s)
	}
}

func TestAggregatorNilClassifierFallsBack(t *testing.T) {
	// Direct construction (not via New) leaves Classifier nil; classify()
	// should return DefaultClassifier under the hood so behavior is sane.
	a := &Aggregator{}
	out := a.Ingest(Signal{Kind: KindBuildFailure})
	if out.Signal.Salience != TierCritical {
		t.Errorf("nil classifier should fall back to default, got %s",
			out.Signal.Salience)
	}
}

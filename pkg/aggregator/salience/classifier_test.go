package salience

import "testing"

func TestDefaultClassifier(t *testing.T) {
	cases := map[Kind]Tier{
		KindBuildFailure:   TierCritical,
		KindTestFailure:    TierHigh,
		KindTemplateSkip:   TierHigh,
		KindLintFailure:    TierMedium,
		KindDependencyBump: TierLow,
		KindNaming:         TierLow,
		KindDocOnly:        TierLow,
		KindCosmetic:       TierNoise,
		KindFormatting:     TierNoise,
	}
	c := DefaultClassifier()
	for k, want := range cases {
		if got := c.Classify(k); got != want {
			t.Errorf("Classify(%s) = %s, want %s", k, got, want)
		}
	}
}

func TestDefaultClassifierCoversAllKinds(t *testing.T) {
	c := DefaultClassifier()
	mc, ok := c.(MapClassifier)
	if !ok {
		t.Fatalf("DefaultClassifier should return MapClassifier, got %T", c)
	}
	for _, k := range AllKinds() {
		if _, present := mc.Table[k]; !present {
			t.Errorf("default classifier missing entry for %s", k)
		}
	}
}

func TestDefaultClassifierFallback(t *testing.T) {
	c := DefaultClassifier()
	// Unknown kind exercises the fallback path. TierLow is the safe
	// default — surface it once, don't drop silently.
	if got := c.Classify(Kind("future_kind")); got != TierLow {
		t.Errorf("fallback = %s, want low", got)
	}
}

func TestDefaultClassifierIsCopy(t *testing.T) {
	// Mutating a returned classifier must not poison shared state for
	// subsequent callers.
	c1 := DefaultClassifier().(MapClassifier)
	c1.Table[KindBuildFailure] = TierLow
	c2 := DefaultClassifier().(MapClassifier)
	if c2.Table[KindBuildFailure] != TierCritical {
		t.Errorf("DefaultClassifier returned shared table; got %s for build_failure",
			c2.Table[KindBuildFailure])
	}
}

func TestMapClassifierFallbackOnly(t *testing.T) {
	c := MapClassifier{Fallback: TierMedium}
	if got := c.Classify(KindBuildFailure); got != TierMedium {
		t.Errorf("empty table should hit fallback, got %s", got)
	}
}

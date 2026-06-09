package gateway

import (
	"strings"
	"testing"
)

func TestLevelForScore(t *testing.T) {
	cases := []struct {
		score float64
		want  HandoffConfidenceLevel
	}{
		{-0.5, ConfidenceLow}, // clamped: total function
		{0.0, ConfidenceLow},
		{0.39, ConfidenceLow},
		{0.40, ConfidenceMedium}, // floor is inclusive
		{0.60, ConfidenceMedium},
		{0.74, ConfidenceMedium},
		{0.75, ConfidenceHigh}, // floor is inclusive
		{0.90, ConfidenceHigh},
		{1.0, ConfidenceHigh},
		{1.5, ConfidenceHigh}, // clamped
	}
	for _, c := range cases {
		if got := LevelForScore(c.score); got != c.want {
			t.Errorf("LevelForScore(%.2f) = %q, want %q", c.score, got, c.want)
		}
	}
}

func TestNewHandoffConfidence(t *testing.T) {
	hc, err := NewHandoffConfidence(0.3, "  integration suite never ran  ", "db untested", "auth unverified")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hc.Level != ConfidenceLow {
		t.Errorf("level = %q, want low (derived from 0.3)", hc.Level)
	}
	if hc.Rationale != "integration suite never ran" {
		t.Errorf("rationale not trimmed: %q", hc.Rationale)
	}
	if len(hc.Gaps) != 2 {
		t.Errorf("gaps = %v, want 2", hc.Gaps)
	}
}

func TestNewHandoffConfidenceRejectsBadInput(t *testing.T) {
	cases := []struct {
		name      string
		score     float64
		rationale string
	}{
		{"score below range", -0.1, "ok"},
		{"score above range", 1.1, "ok"},
		{"empty rationale", 0.8, ""},
		{"whitespace rationale", 0.8, "   "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewHandoffConfidence(c.score, c.rationale); err == nil {
				t.Errorf("expected error for %s", c.name)
			}
		})
	}
}

func TestHandoffConfidenceValidateDetectsInconsistency(t *testing.T) {
	// Hand-built (as if from a tampered file): score says high, level says low.
	hc := &HandoffConfidence{Level: ConfidenceLow, Score: 0.95, Rationale: "x"}
	if err := hc.Validate(); err == nil {
		t.Error("level/score mismatch should fail validation")
	}

	bad := &HandoffConfidence{Level: "bogus", Score: 0.5, Rationale: "x"}
	if err := bad.Validate(); err == nil {
		t.Error("unknown level should fail validation")
	}
}

func TestHandoffConfidenceLevelIsValid(t *testing.T) {
	for _, l := range []HandoffConfidenceLevel{ConfidenceLow, ConfidenceMedium, ConfidenceHigh} {
		if !l.IsValid() {
			t.Errorf("%q should be valid", l)
		}
	}
	if HandoffConfidenceLevel("medium-ish").IsValid() {
		t.Error("unknown level should be invalid")
	}
}

func TestHandoffContextSetConfidence(t *testing.T) {
	h := &HandoffContext{FromMode: ModeImplementer, ToMode: ModeArchitect}

	if err := h.SetConfidence(0.8, "all tests green, design followed"); err != nil {
		t.Fatalf("SetConfidence: %v", err)
	}
	if h.Confidence == nil || h.Confidence.Level != ConfidenceHigh {
		t.Fatalf("expected high confidence set, got %+v", h.Confidence)
	}

	// An invalid call must not mutate the previously-set assessment.
	prev := h.Confidence
	if err := h.SetConfidence(2.0, "bad"); err == nil {
		t.Error("out-of-range score should error")
	}
	if h.Confidence != prev {
		t.Error("failed SetConfidence must leave prior confidence unchanged")
	}
}

func TestGeneratePromptIncludesConfidence(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	h := &HandoffContext{
		FromMode: ModeArchitect,
		ToMode:   ModeImplementer,
		Summary:  "designed the thing",
	}
	if err := h.SetConfidence(0.30, "spec is incomplete", "error handling undefined"); err != nil {
		t.Fatalf("SetConfidence: %v", err)
	}

	out, err := gen.GeneratePrompt(h)
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	for _, want := range []string{"Handoff Confidence", "LOW", "0.30", "spec is incomplete", "error handling undefined"} {
		if !strings.Contains(out, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, out)
		}
	}
}

func TestGeneratePromptFlagsUnassessedConfidence(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	// Nil confidence must surface loudly — silence must never read as trust.
	h := &HandoffContext{FromMode: "x", ToMode: "y", Summary: "s"}
	out, err := gen.GeneratePrompt(h)
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if !strings.Contains(out, "CONFIDENCE NOT ASSESSED") {
		t.Errorf("unassessed handoff must be flagged\n---\n%s", out)
	}
}

func TestHandoffConfidenceSerializationRoundTrip(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	orig := &HandoffContext{FromMode: ModeArchitect, ToMode: ModeImplementer, Summary: "s"}
	if err := orig.SetConfidence(0.5, "partial coverage", "edge cases"); err != nil {
		t.Fatalf("SetConfidence: %v", err)
	}

	data, err := gen.SerializeContext(orig)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	got, err := gen.DeserializeContext(data)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	if got.Confidence == nil {
		t.Fatal("confidence lost in round-trip")
	}
	if got.Confidence.Level != ConfidenceMedium || got.Confidence.Score != 0.5 {
		t.Errorf("confidence changed: %+v", got.Confidence)
	}
	if len(got.Confidence.Gaps) != 1 || got.Confidence.Gaps[0] != "edge cases" {
		t.Errorf("gaps not preserved: %v", got.Confidence.Gaps)
	}
}

func TestDeserializeRejectsInconsistentConfidence(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()
	// Score 0.95 maps to high, but the file claims low — reject it rather
	// than let a receiver act on a tampered assessment.
	tampered := `{"from_mode":"a","to_mode":"b","summary":"s","confidence":{"level":"low","score":0.95,"rationale":"x"}}`
	if _, err := gen.DeserializeContext(tampered); err == nil {
		t.Error("inconsistent confidence block should fail deserialization")
	}
}

func TestDeserializeAllowsAbsentConfidence(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()
	// A handoff produced before this feature has no confidence field; it
	// must still load (Confidence nil), not error.
	old := `{"from_mode":"a","to_mode":"b","summary":"s"}`
	got, err := gen.DeserializeContext(old)
	if err != nil {
		t.Fatalf("legacy handoff without confidence should load: %v", err)
	}
	if got.Confidence != nil {
		t.Errorf("expected nil confidence, got %+v", got.Confidence)
	}
}

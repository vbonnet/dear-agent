package ops

import (
	"strings"
	"testing"
)

func TestSynthesis_RecurringSystemic_P1(t *testing.T) {
	// The sample retro is a recurring (score 2) + systemic issue → worst quadrant.
	p := writeTemp(t, sampleRetro)
	res, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, AllLenses: true})
	if err != nil {
		t.Fatalf("AnalyzeRetro: %v", err)
	}
	s := res.Synthesis
	if s == nil {
		t.Fatal("expected synthesis for all-lenses run")
	}
	if s.Priority != "P1" {
		t.Errorf("priority = %q, want P1 (score=%d)", s.Priority, s.Score)
	}
	if s.Score != 4 {
		t.Errorf("score = %d, want 4 (recurrence 2 + systemic 2)", s.Score)
	}
	if s.Confidence != "high" {
		t.Errorf("confidence = %q, want high", s.Confidence)
	}
	if s.Verdict == "" || !strings.HasPrefix(s.Verdict, "Recurring systemic issue") {
		t.Errorf("verdict = %q, want it to start with 'Recurring systemic issue'", s.Verdict)
	}
	if len(s.KeyActions) == 0 {
		t.Error("expected key actions surfaced from remediation lens")
	}
	// Tracked (bead/PR-tagged) actions must sort ahead of untracked ones.
	if !strings.HasPrefix(s.KeyActions[0], "[") {
		t.Errorf("first key action = %q, want a tracked (bead/PR-tagged) action first", s.KeyActions[0])
	}
}

func TestSynthesis_NovelOneOff_P3(t *testing.T) {
	novel := "# One-off typo fix\n\n## Define\n\nA single config typo broke startup. Isolated, context-specific manual error.\n"
	p := writeTemp(t, novel)
	res, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, AllLenses: true})
	if err != nil {
		t.Fatalf("AnalyzeRetro: %v", err)
	}
	s := res.Synthesis
	if s == nil {
		t.Fatal("expected synthesis")
	}
	if s.Priority != "P3" {
		t.Errorf("priority = %q, want P3 (score=%d)", s.Priority, s.Score)
	}
	if s.Score != 0 {
		t.Errorf("score = %d, want 0", s.Score)
	}
}

func TestSynthesis_OnlyWhenAllLenses(t *testing.T) {
	p := writeTemp(t, sampleRetro)
	res, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, Lens: "root-cause"})
	if err != nil {
		t.Fatalf("AnalyzeRetro: %v", err)
	}
	if res.Synthesis != nil {
		t.Errorf("expected nil synthesis for single-lens run, got %+v", res.Synthesis)
	}
}

func TestSynthesis_GapHighPriorityNoActions(t *testing.T) {
	// Systemic + recurring language, but no remediation section at all → the
	// supervisor should flag the missing follow-up.
	retro := "# Recurring systemic outage\n\n## Define\n\nThis is a recurring structural design flaw; it will recur by default. " +
		"Happened before in the same pattern, again and again.\n"
	p := writeTemp(t, retro)
	res, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, AllLenses: true})
	if err != nil {
		t.Fatalf("AnalyzeRetro: %v", err)
	}
	s := res.Synthesis
	if s == nil {
		t.Fatal("expected synthesis")
	}
	if s.Priority == "P3" {
		t.Fatalf("expected an elevated priority, got %q (score=%d)", s.Priority, s.Score)
	}
	if !containsSubstr(s.Gaps, "no recorded follow-up") {
		t.Errorf("expected a gap about missing follow-up, got %v", s.Gaps)
	}
}

func TestSynthesis_Indeterminate_LowConfidence(t *testing.T) {
	// No systemic or one-off signals → indeterminate classification → low confidence.
	retro := "# Neutral note\n\n## Define\n\nWe changed a label color.\n"
	p := writeTemp(t, retro)
	res, err := AnalyzeRetro(&AnalyzeRetroRequest{InputPath: p, AllLenses: true})
	if err != nil {
		t.Fatalf("AnalyzeRetro: %v", err)
	}
	s := res.Synthesis
	if s == nil {
		t.Fatal("expected synthesis")
	}
	if res.Lenses.Systemic.Classification != "indeterminate" {
		t.Fatalf("precondition: classification = %q, want indeterminate", res.Lenses.Systemic.Classification)
	}
	if s.Confidence != "low" {
		t.Errorf("confidence = %q, want low for indeterminate classification", s.Confidence)
	}
}

func TestFirstSentence(t *testing.T) {
	cases := map[string]string{
		"One sentence. Two sentence.":  "One sentence.",
		"  spaced\n\nout  words  here": "spaced out words here",
		"":                             "",
	}
	for in, want := range cases {
		if got := firstSentence(in); got != want {
			t.Errorf("firstSentence(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("word ", 60)
	if got := firstSentence(long); !strings.HasSuffix(got, "…") {
		t.Errorf("expected long input to be truncated with ellipsis, got %q", got)
	}
}

func containsSubstr(haystack []string, needle string) bool {
	for _, h := range haystack {
		if strings.Contains(h, needle) {
			return true
		}
	}
	return false
}

package escalation

import (
	"context"
	"errors"
	"testing"
)

func TestDefaultAdjudicator(t *testing.T) {
	a := DefaultAdjudicator{}
	ctx := context.Background()

	cases := []struct {
		name        string
		answer      string
		wantOutcome Outcome // "" means "declined / could not assess"
	}{
		{"empty answer is incorrect", "", OutcomeIncorrect},
		{"idk is incorrect", "idk", OutcomeIncorrect},
		{"n/a is incorrect", "  N/A ", OutcomeIncorrect},
		{"not sure is incorrect", "Not sure", OutcomeIncorrect},
		{"substantive answer is declined offline", "Use the v2 API; v1 is deprecated.", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := a.Adjudicate(ctx, AdjudicationRequest{Answer: tc.answer})
			if err != nil {
				t.Fatalf("Adjudicate: %v", err)
			}
			if v.Outcome != tc.wantOutcome {
				t.Errorf("outcome = %q, want %q (reason: %s)", v.Outcome, tc.wantOutcome, v.Reason)
			}
		})
	}
}

func TestClaudeAdjudicatorFloorWinsOnNonAnswer(t *testing.T) {
	// The model layer must never be consulted for a case the floor already
	// settles (a non-answer): no call should be recorded.
	fake := &FakeAdjudicator{Verdict: Adjudication{Outcome: OutcomeCorrect}}
	a := NewClaudeAdjudicatorWith(fake)

	v, err := a.Adjudicate(context.Background(), AdjudicationRequest{Answer: ""})
	if err != nil {
		t.Fatalf("Adjudicate: %v", err)
	}
	if v.Outcome != OutcomeIncorrect {
		t.Errorf("outcome = %q, want incorrect (floor should win)", v.Outcome)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("model was consulted %d times for a non-answer; want 0", len(fake.Calls))
	}
}

func TestClaudeAdjudicatorModelWinsOnSubstantive(t *testing.T) {
	fake := &FakeAdjudicator{Verdict: Adjudication{
		Outcome:      OutcomeMisaligned,
		Misalignment: "told the agent to skip tests",
		Reason:       "steers off-course",
	}}
	a := NewClaudeAdjudicatorWith(fake)
	if a.Name() != "claude" {
		t.Errorf("Name() = %q, want claude", a.Name())
	}

	req := AdjudicationRequest{Question: "should I skip tests?", Answer: "yes, skip them", AnsweredByRole: "coder"}
	v, err := a.Adjudicate(context.Background(), req)
	if err != nil {
		t.Fatalf("Adjudicate: %v", err)
	}
	if v.Outcome != OutcomeMisaligned || v.Misalignment == "" {
		t.Errorf("got %+v, want misaligned with a note", v)
	}
	if len(fake.Calls) != 1 || fake.Calls[0].Answer != "yes, skip them" {
		t.Errorf("model call not passed through: %+v", fake.Calls)
	}
}

func TestClaudeAdjudicatorDegradesOnModelError(t *testing.T) {
	// A model error must degrade to the floor's verdict (empty for a substantive
	// answer) — never invent an outcome.
	fake := &FakeAdjudicator{Err: errors.New("model unreachable")}
	a := NewClaudeAdjudicatorWith(fake)

	v, err := a.Adjudicate(context.Background(), AdjudicationRequest{Answer: "a real substantive answer here"})
	if err != nil {
		t.Fatalf("Adjudicate should degrade, not error: %v", err)
	}
	if v.Outcome != "" {
		t.Errorf("outcome = %q, want empty (degrade, do not guess)", v.Outcome)
	}
}

func TestClaudeAdjudicatorNoModelBehavesLikeDefault(t *testing.T) {
	a := NewClaudeAdjudicatorWith(nil)
	if a.Name() != "default" {
		t.Errorf("Name() = %q, want default when no model wired", a.Name())
	}
	v, err := a.Adjudicate(context.Background(), AdjudicationRequest{Answer: "substantive"})
	if err != nil {
		t.Fatalf("Adjudicate: %v", err)
	}
	if v.Outcome != "" {
		t.Errorf("outcome = %q, want empty", v.Outcome)
	}
}

func TestModelAdjudicatorSupportsEveryModelFamily(t *testing.T) {
	families := []string{"anthropic", "openai", "gemini", "glm", "deepseek", "nemotron", "qwen"}
	for _, family := range families {
		t.Run(family, func(t *testing.T) {
			fake := &FakeAdjudicator{Verdict: Adjudication{Outcome: OutcomeCorrect}}
			a := NewModelAdjudicator(family, fake)
			if got := a.Name(); got != family {
				t.Fatalf("Name() = %q, want %q", got, family)
			}
			got, err := a.Adjudicate(context.Background(), AdjudicationRequest{Answer: "substantive answer"})
			if err != nil {
				t.Fatalf("Adjudicate: %v", err)
			}
			if got.Outcome != OutcomeCorrect {
				t.Fatalf("outcome = %q, want %q", got.Outcome, OutcomeCorrect)
			}
			if len(fake.Calls) != 1 {
				t.Fatalf("model calls = %d, want 1", len(fake.Calls))
			}
		})
	}
}

func TestParseAdjudicationJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare json", `{"outcome":"correct","misalignment":"","reason":"ok"}`, "correct"},
		{"fenced json", "```json\n{\"outcome\":\"incorrect\",\"reason\":\"wrong\"}\n```", "incorrect"},
		{"prose around", `Here is my verdict: {"outcome":"misaligned","reason":"x"} done`, "misaligned"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := parseAdjudicationJSON(tc.in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if v.Outcome != tc.want {
				t.Errorf("outcome = %q, want %q", v.Outcome, tc.want)
			}
		})
	}
	if _, err := parseAdjudicationJSON("no json at all"); err == nil {
		t.Errorf("expected error for non-JSON reply")
	}
}

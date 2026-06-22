package escalation

import "testing"

// analysisFixture builds a log: question h1 asked by w1 and w2 (two agents),
// answered once incorrectly; question h2 asked once by w3, answered misaligned;
// question h3 asked three times by w4 (one agent, repeated).
func analysisFixture() []EscalationEvent {
	return []EscalationEvent{
		{EscalationID: "a", Phase: PhaseRaised, Kind: KindQuestion, Question: "which api?", QuestionHash: "h1", OriginSessionID: "w1"},
		{EscalationID: "a", Phase: PhaseAnswered, Question: "which api?", QuestionHash: "h1", Answer: "v1", Outcome: string(OutcomeIncorrect), Misalignment: "v1 is deprecated", AnsweredByRole: "sup"},
		{EscalationID: "b", Phase: PhaseRaised, Kind: KindQuestion, Question: "which api?", QuestionHash: "h1", OriginSessionID: "w2"},
		{EscalationID: "c", Phase: PhaseRaised, Kind: KindDecision, Question: "ship it?", QuestionHash: "h2", OriginSessionID: "w3"},
		{EscalationID: "c", Phase: PhaseAnswered, Question: "ship it?", QuestionHash: "h2", Answer: "sure", Outcome: string(OutcomeMisaligned), Misalignment: "skipped review"},
		{EscalationID: "d", Phase: PhaseRaised, Kind: KindQuestion, Question: "where logs?", QuestionHash: "h3", OriginSessionID: "w4"},
		{EscalationID: "e", Phase: PhaseRaised, Kind: KindQuestion, Question: "where logs?", QuestionHash: "h3", OriginSessionID: "w4"},
		{EscalationID: "f", Phase: PhaseRaised, Kind: KindQuestion, Question: "where logs?", QuestionHash: "h3", OriginSessionID: "w4"},
	}
}

func TestAnalyzeMisaligned(t *testing.T) {
	got := AnalyzeMisaligned(analysisFixture())
	if len(got) != 2 {
		t.Fatalf("got %d misaligned, want 2: %+v", len(got), got)
	}
	// incorrect sorts before misaligned.
	if got[0].Outcome != OutcomeIncorrect || got[0].EscalationID != "a" {
		t.Errorf("first = %+v, want esc a / incorrect", got[0])
	}
	if got[1].Outcome != OutcomeMisaligned || got[1].Misalignment != "skipped review" {
		t.Errorf("second = %+v, want esc c / misaligned with note", got[1])
	}
}

func TestAnalyzeFrequentQuestions(t *testing.T) {
	// minCount=2: h1 (count 2) and h3 (count 3) qualify; h2 (count 1) does not.
	got := AnalyzeFrequentQuestions(analysisFixture(), 2)
	if len(got) != 2 {
		t.Fatalf("got %d groups, want 2: %+v", len(got), got)
	}
	if got[0].QuestionHash != "h3" || got[0].Count != 3 {
		t.Errorf("most frequent = %+v, want h3 count 3", got[0])
	}
	if got[1].QuestionHash != "h1" || got[1].Count != 2 {
		t.Errorf("second = %+v, want h1 count 2", got[1])
	}
}

func TestAnalyzeManyAgents(t *testing.T) {
	// minDistinctOrigins=2: only h1 (w1, w2) qualifies. h3 has count 3 but a
	// single origin (w4) — repeated by one agent, NOT the missing-context signal.
	got := AnalyzeManyAgents(analysisFixture(), 2)
	if len(got) != 1 {
		t.Fatalf("got %d groups, want 1: %+v", len(got), got)
	}
	if got[0].QuestionHash != "h1" || got[0].DistinctOrigins != 2 {
		t.Errorf("group = %+v, want h1 with 2 distinct origins", got[0])
	}
	if got[0].Count != 2 {
		t.Errorf("count = %d, want 2", got[0].Count)
	}
}

func TestSummarizeFoldsEvents(t *testing.T) {
	got := Summarize(analysisFixture())
	if len(got) != 6 {
		t.Fatalf("got %d summaries, want 6 (a..f)", len(got))
	}
	byID := map[string]EscalationSummary{}
	for _, s := range got {
		byID[s.EscalationID] = s
	}
	a := byID["a"]
	if !a.Answered || a.Outcome != OutcomeIncorrect || a.OriginSessionID != "w1" || a.AnsweredByRole != "sup" {
		t.Errorf("esc a folded wrong: %+v", a)
	}
	if byID["b"].Answered {
		t.Errorf("esc b should be unanswered: %+v", byID["b"])
	}
}

func TestAnalyzeDefaultsThresholds(t *testing.T) {
	// minCount<=0 defaults to 2.
	if got := AnalyzeFrequentQuestions(analysisFixture(), 0); len(got) != 2 {
		t.Errorf("default minCount: got %d groups, want 2", len(got))
	}
	if got := AnalyzeManyAgents(analysisFixture(), -1); len(got) != 1 {
		t.Errorf("default minDistinct: got %d groups, want 1", len(got))
	}
}

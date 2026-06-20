package escalation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestLogger_WritesDerivedFieldsAndJSONL(t *testing.T) {
	buf := &bytes.Buffer{}
	lg := NewLogger(NewJSONLSink(nopCloser{buf}))
	ctx := context.Background()

	if err := lg.Record(ctx, EscalationEvent{
		EscalationID: "e1", Phase: PhaseRaised, Kind: KindQuestion,
		OriginSessionID: "w1", Question: "Which  API  version?!",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	var ev EscalationEvent
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventID == "" {
		t.Errorf("EventID not populated")
	}
	if ev.Timestamp.IsZero() {
		t.Errorf("Timestamp not populated")
	}
	if ev.QuestionNorm != "which api version" {
		t.Errorf("QuestionNorm = %q", ev.QuestionNorm)
	}
	if ev.QuestionHash != HashQuestion("which api version") {
		t.Errorf("QuestionHash mismatch")
	}
}

// TestLog_AnalysisQueries demonstrates that the schema supports the three
// required analyses purely by grouping over the JSONL.
func TestLog_AnalysisQueries(t *testing.T) {
	buf := &bytes.Buffer{}
	lg := NewLogger(NewJSONLSink(nopCloser{buf}))
	ctx := context.Background()

	// Three different agents ask the same question (different wording).
	for i, q := range []string{
		"Which API version should I use?",
		"which api version should i use",
		"Which   API version  should I use??",
	} {
		_ = lg.Record(ctx, EscalationEvent{
			EscalationID: "e" + string(rune('a'+i)), Phase: PhaseRaised,
			Kind: KindQuestion, OriginSessionID: "agent-" + string(rune('a'+i)), Question: q,
		})
	}
	// An answered event flagged misaligned by a later review pass.
	_ = lg.Record(ctx, EscalationEvent{
		EscalationID: "e1", Phase: PhaseAnswered, Kind: KindQuestion,
		Answer: "v1", AnsweredBy: "sup", Outcome: "misaligned",
		Misalignment: "repo standardized on v3", Question: "Which API version should I use?",
	})

	// Replay the log.
	var events []EscalationEvent
	sc := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	for sc.Scan() {
		var ev EscalationEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("scan unmarshal: %v", err)
		}
		events = append(events, ev)
	}

	// (3) many agents asking the same question → group by hash, count distinct origins.
	target := HashQuestion(NormalizeQuestion("Which API version should I use?"))
	origins := map[string]bool{}
	for _, ev := range events {
		if ev.Phase == PhaseRaised && ev.QuestionHash == target {
			origins[ev.OriginSessionID] = true
		}
	}
	if len(origins) != 3 {
		t.Errorf("distinct origins for shared question = %d, want 3", len(origins))
	}

	// (1) incorrect/misaligned answers → filter on outcome.
	misaligned := 0
	for _, ev := range events {
		if ev.Phase == PhaseAnswered && (ev.Outcome == "incorrect" || ev.Outcome == "misaligned") {
			misaligned++
		}
	}
	if misaligned != 1 {
		t.Errorf("misaligned answers = %d, want 1", misaligned)
	}
}

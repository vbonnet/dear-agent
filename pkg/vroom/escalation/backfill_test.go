package escalation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubAdjudicator returns a fixed verdict for every answered event, recording
// how many times it was called.
type stubAdjudicator struct {
	verdict Adjudication
	calls   int
}

func (s *stubAdjudicator) Name() string { return "stub" }
func (s *stubAdjudicator) Adjudicate(_ context.Context, _ AdjudicationRequest) (Adjudication, error) {
	s.calls++
	return s.verdict, nil
}

func sampleEvents() []EscalationEvent {
	return []EscalationEvent{
		{EventID: "1", EscalationID: "esc-a", Phase: PhaseRaised, Kind: KindQuestion,
			Question: "which api?", QuestionHash: "h1", OriginSessionID: "w1"},
		{EventID: "2", EscalationID: "esc-a", Phase: PhaseAnswered, Kind: KindQuestion,
			Question: "which api?", QuestionHash: "h1", Answer: "use v2", AnsweredByRole: "sup"},
		{EventID: "3", EscalationID: "esc-b", Phase: PhaseRaised, Kind: KindQuestion,
			Question: "should I deploy?", QuestionHash: "h2", OriginSessionID: "w2"},
		// esc-b never answered → not a candidate.
		{EventID: "4", EscalationID: "esc-c", Phase: PhaseAutoResolved, Kind: KindQuestion,
			Question: "proceed?", QuestionHash: "h3", Answer: "yes proceed", OriginSessionID: "w3"},
	}
}

func TestBackfillScoresOnlyAnsweredCandidates(t *testing.T) {
	stub := &stubAdjudicator{verdict: Adjudication{Outcome: OutcomeIncorrect, Misalignment: "bad"}}
	out, res, err := Backfill(context.Background(), sampleEvents(), stub, false)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	// Only event 2 (PhaseAnswered with answer text) is a candidate. Auto-resolved
	// and raised events are not scored.
	if res.Candidates != 1 || res.Updated != 1 {
		t.Fatalf("candidates=%d updated=%d, want 1/1 (res=%+v)", res.Candidates, res.Updated, res)
	}
	if stub.calls != 1 {
		t.Errorf("adjudicator called %d times, want 1", stub.calls)
	}
	if out[1].Outcome != string(OutcomeIncorrect) || out[1].Misalignment != "bad" {
		t.Errorf("event 2 not backfilled: %+v", out[1])
	}
	// Input must not be mutated.
	if sampleEvents()[1].Outcome != "" {
		t.Errorf("Backfill mutated a fresh sample? unexpected")
	}
}

func TestBackfillIdempotentUnlessForce(t *testing.T) {
	events := sampleEvents()
	events[1].Outcome = string(OutcomeCorrect) // already adjudicated
	stub := &stubAdjudicator{verdict: Adjudication{Outcome: OutcomeIncorrect}}

	_, res, err := Backfill(context.Background(), events, stub, false)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if res.Candidates != 0 || stub.calls != 0 {
		t.Errorf("already-adjudicated event was re-scored: candidates=%d calls=%d", res.Candidates, stub.calls)
	}

	out, res2, err := Backfill(context.Background(), events, stub, true) // force
	if err != nil {
		t.Fatalf("Backfill force: %v", err)
	}
	if res2.Updated != 1 || out[1].Outcome != string(OutcomeIncorrect) {
		t.Errorf("force did not re-adjudicate: res=%+v outcome=%q", res2, out[1].Outcome)
	}
}

func TestBackfillSkipsOnDeclinedVerdict(t *testing.T) {
	stub := &stubAdjudicator{verdict: Adjudication{}} // empty outcome = declined
	out, res, err := Backfill(context.Background(), sampleEvents(), stub, false)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if res.Candidates != 1 || res.Updated != 0 || res.Skipped != 1 {
		t.Errorf("res=%+v, want candidates=1 updated=0 skipped=1", res)
	}
	if out[1].Outcome != "" {
		t.Errorf("declined verdict should leave outcome empty, got %q", out[1].Outcome)
	}
}

func TestReadWriteEventsRoundTrip(t *testing.T) {
	events := sampleEvents()
	var sb strings.Builder
	if err := WriteEvents(&sb, events); err != nil {
		t.Fatalf("WriteEvents: %v", err)
	}
	got, err := ReadEvents(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != len(events) {
		t.Fatalf("round-trip len = %d, want %d", len(got), len(events))
	}
	for i := range got {
		if got[i].EventID != events[i].EventID || got[i].Phase != events[i].Phase {
			t.Errorf("event %d mismatch: %+v vs %+v", i, got[i], events[i])
		}
	}
}

func TestReadEventsSkipsBlankLinesAndErrorsOnGarbage(t *testing.T) {
	good := `{"event_id":"1","escalation_id":"a","phase":"raised"}`
	if _, err := ReadEvents(strings.NewReader(good + "\n\n" + good + "\n")); err != nil {
		t.Errorf("blank lines should be skipped: %v", err)
	}
	if _, err := ReadEvents(strings.NewReader("{not json}\n")); err == nil {
		t.Errorf("garbage line should error")
	}
}

func TestBackfillFileAtomicRewrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	var sb strings.Builder
	if err := WriteEvents(&sb, sampleEvents()); err != nil {
		t.Fatalf("WriteEvents: %v", err)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stub := &stubAdjudicator{verdict: Adjudication{Outcome: OutcomeMisaligned, Misalignment: "off-course"}}
	res, err := BackfillFile(context.Background(), path, stub, false)
	if err != nil {
		t.Fatalf("BackfillFile: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated=%d, want 1", res.Updated)
	}

	// Re-read the file and confirm the column landed on the answered event only.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got, err := ReadEvents(strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("rewrite changed event count: %d", len(got))
	}
	var answered int
	for _, ev := range got {
		if ev.Phase == PhaseAnswered {
			answered++
			if ev.Outcome != string(OutcomeMisaligned) {
				t.Errorf("answered event outcome = %q, want misaligned", ev.Outcome)
			}
		}
	}
	if answered != 1 {
		t.Errorf("answered count = %d, want 1", answered)
	}
}

func TestBackfillFileMissingIsNoOp(t *testing.T) {
	res, err := BackfillFile(context.Background(), filepath.Join(t.TempDir(), "absent.jsonl"),
		DefaultAdjudicator{}, false)
	if err != nil {
		t.Fatalf("missing file should be a no-op, got %v", err)
	}
	if res.Total != 0 || res.Updated != 0 {
		t.Errorf("res=%+v, want empty", res)
	}
}

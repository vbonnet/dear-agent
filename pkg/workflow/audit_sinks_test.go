package workflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStdoutAuditSink_Format checks the line shape: timestamp, run, node,
// attempt, transition, actor, reason. The exact wording is tested loosely
// (substring match) so future format tweaks don't require massive test
// edits.
func TestStdoutAuditSink_Format(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutAuditSink{W: &buf}
	ev := AuditEvent{
		EventID:    "ev-1",
		RunID:      "run-1",
		NodeID:     "stage1",
		AttemptNo:  2,
		FromState:  "running",
		ToState:    "succeeded",
		Reason:     "ok",
		Actor:      "system",
		OccurredAt: time.Date(2026, 5, 2, 9, 31, 14, 0, time.UTC),
	}
	if err := sink.Emit(context.Background(), ev); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"run=run-1", "node=stage1", "attempt=2", "running→succeeded", `reason="ok"`, "actor=system"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nGOT %q", want, out)
		}
	}
}

// TestJSONLAuditSink_RoundTrip writes two events, reads them back, and
// confirms the on-disk shape matches the public auditEventJSON format.
func TestJSONLAuditSink_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	sink, f, err := NewJSONLAuditSinkFile(path)
	if err != nil {
		t.Fatalf("NewJSONLAuditSinkFile: %v", err)
	}
	defer f.Close()

	events := []AuditEvent{
		{EventID: "a", RunID: "r1", FromState: "pending", ToState: "running", Actor: "system", OccurredAt: time.Now()},
		{EventID: "b", RunID: "r1", NodeID: "n1", AttemptNo: 1, FromState: "running", ToState: "succeeded", Actor: "system", OccurredAt: time.Now()},
	}
	for _, ev := range events {
		if err := sink.Emit(context.Background(), ev); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Read back and decode.
	contents, err := readFile(path)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	var got []map[string]any
	for scanner.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			t.Fatalf("decode: %v line=%q", err, scanner.Text())
		}
		got = append(got, rec)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	if got[0]["event_id"] != "a" || got[1]["event_id"] != "b" {
		t.Errorf("event_id mismatch: %v", got)
	}
	if got[1]["node_id"] != "n1" {
		t.Errorf("node_id missing in record 2: %v", got[1])
	}
}

type fakeEngramPublisher struct {
	calls    int
	lastKind string
	lastPL   map[string]any
	err      error
}

func (f *fakeEngramPublisher) Publish(_ context.Context, kind string, pl map[string]any) error {
	f.calls++
	f.lastKind = kind
	f.lastPL = pl
	return f.err
}

func TestEngramAuditSink_ForwardsAndCarriesPayload(t *testing.T) {
	pub := &fakeEngramPublisher{}
	sink := &EngramAuditSink{Publisher: pub}
	ev := AuditEvent{
		EventID:    "e",
		RunID:      "r1",
		NodeID:     "n",
		ToState:    "succeeded",
		Actor:      "system",
		OccurredAt: time.Now(),
		Payload:    map[string]any{"tokens": 1234, "model": "claude-opus-4-7"},
	}
	if err := sink.Emit(context.Background(), ev); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if pub.calls != 1 {
		t.Errorf("calls=%d want 1", pub.calls)
	}
	if pub.lastKind != "workflow.audit_event" {
		t.Errorf("kind=%q want workflow.audit_event", pub.lastKind)
	}
	if pub.lastPL["tokens"] != 1234 {
		t.Errorf("payload tokens missing: %v", pub.lastPL)
	}
	if pub.lastPL["run_id"] != "r1" {
		t.Errorf("payload run_id missing: %v", pub.lastPL)
	}
}

func TestEngramAuditSink_ErrorRecorded(t *testing.T) {
	pub := &fakeEngramPublisher{err: errors.New("backend down")}
	sink := &EngramAuditSink{Publisher: pub}
	err := sink.Emit(context.Background(), AuditEvent{RunID: "r1", ToState: "running", Actor: "system"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if sink.LastErr() == nil {
		t.Fatal("LastErr returned nil")
	}
}

type fakeOTelEmitter struct {
	name  string
	attrs map[string]any
	calls int
	err   error
}

func (f *fakeOTelEmitter) EmitEvent(_ context.Context, name string, attrs map[string]any, _ time.Time) error {
	f.calls++
	f.name = name
	f.attrs = attrs
	return f.err
}

func TestOTelAuditSink_Translates(t *testing.T) {
	em := &fakeOTelEmitter{}
	sink := &OTelAuditSink{Emitter: em}
	ev := AuditEvent{
		RunID:      "r1",
		NodeID:     "n",
		AttemptNo:  3,
		ToState:    "failed",
		Actor:      "system",
		Reason:     "boom",
		OccurredAt: time.Now(),
		Payload:    map[string]any{"err": "oops"},
	}
	if err := sink.Emit(context.Background(), ev); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if em.name != "workflow.transition.failed" {
		t.Errorf("name=%q", em.name)
	}
	if em.attrs["workflow.run_id"] != "r1" {
		t.Errorf("missing run_id attr: %v", em.attrs)
	}
	if em.attrs["workflow.attempt_no"] != 3 {
		t.Errorf("attempt_no attr missing: %v", em.attrs)
	}
	if em.attrs["workflow.payload.err"] != "oops" {
		t.Errorf("payload attr missing: %v", em.attrs)
	}
}

// TestMultiAuditSink_OneSinkFails_OthersContinue verifies the substrate
// promise: one broken sink does not block the rest of the chain.
func TestMultiAuditSink_OneSinkFails_OthersContinue(t *testing.T) {
	good := &countingSink{}
	bad := &erroringSink{err: errors.New("disk full")}
	var observed error
	multi := &MultiAuditSink{
		Sinks: []AuditSink{bad, good},
		OnError: func(_ AuditSink, _ AuditEvent, e error) {
			observed = e
		},
	}
	if err := multi.Emit(context.Background(), AuditEvent{RunID: "r1", ToState: "running", Actor: "system"}); err != nil {
		t.Fatalf("MultiAuditSink.Emit returned err: %v", err)
	}
	if observed == nil {
		t.Error("OnError not called")
	}
	if good.calls != 1 {
		t.Errorf("good sink calls=%d want 1", good.calls)
	}
}

type countingSink struct{ calls int }

func (s *countingSink) Emit(_ context.Context, _ AuditEvent) error {
	s.calls++
	return nil
}

type erroringSink struct{ err error }

func (s *erroringSink) Emit(_ context.Context, _ AuditEvent) error {
	return s.err
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

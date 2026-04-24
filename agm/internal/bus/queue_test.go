package bus

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTempQueue(t *testing.T) *Queue {
	t.Helper()
	// Keep under /tmp so tests that open unix sockets in the same dir
	// stay under the macOS 104-byte socket-path cap.
	dir, err := os.MkdirTemp("/tmp", "busq-*") //nolint:usetesting // socket path-length constraint
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	q, err := NewQueue(dir)
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	return q
}

func TestQueueAppendAndDrain(t *testing.T) {
	q := newTempQueue(t)

	frames := []*Frame{
		{Type: FrameDeliver, From: "s1", To: "s2", Text: "one"},
		{Type: FrameDeliver, From: "s1", To: "s2", Text: "two"},
		{Type: FrameDeliver, From: "s1", To: "s2", Text: "three"},
	}
	for _, f := range frames {
		if err := q.Append("s2", f); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	if n, err := q.Len("s2"); err != nil || n != 3 {
		t.Fatalf("Len = %d, %v; want 3", n, err)
	}

	drained, err := q.Drain("s2")
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(drained) != 3 {
		t.Fatalf("Drain returned %d frames, want 3", len(drained))
	}
	for i, f := range drained {
		if f.Text != frames[i].Text {
			t.Errorf("drained[%d].Text = %q, want %q", i, f.Text, frames[i].Text)
		}
	}

	// After drain: file should be empty.
	if n, err := q.Len("s2"); err != nil || n != 0 {
		t.Errorf("after Drain, Len = %d, %v; want 0", n, err)
	}
}

func TestQueueDrainNoFile(t *testing.T) {
	q := newTempQueue(t)
	frames, err := q.Drain("never-queued")
	if err != nil {
		t.Errorf("Drain on missing session: %v", err)
	}
	if frames != nil {
		t.Errorf("Drain returned %v, want nil", frames)
	}
}

func TestQueueSanitizesSessionID(t *testing.T) {
	q := newTempQueue(t)

	// Path-traversal attempts should fail.
	bad := []string{"../escape", "s/p", "a\\b", "", "..", "s..t"}
	for _, id := range bad {
		if err := q.Append(id, &Frame{Type: FrameDeliver, From: "x", To: id}); err == nil {
			t.Errorf("Append(%q) expected error", id)
		}
	}
}

func TestQueueAppendValidation(t *testing.T) {
	q := newTempQueue(t)
	if err := q.Append("s1", nil); err == nil {
		t.Error("Append(nil) should fail")
	}
}

func TestQueueFIFOOrder(t *testing.T) {
	q := newTempQueue(t)
	for i := 0; i < 100; i++ {
		f := &Frame{Type: FrameDeliver, From: "x", To: "y", Text: string(rune('a' + (i % 26)))}
		_ = q.Append("y", f)
	}
	drained, err := q.Drain("y")
	if err != nil {
		t.Fatal(err)
	}
	if len(drained) != 100 {
		t.Fatalf("drained %d, want 100", len(drained))
	}
	// Each index should match the input at that index.
	for i := 0; i < 100; i++ {
		want := string(rune('a' + (i % 26)))
		if drained[i].Text != want {
			t.Fatalf("drained[%d].Text = %q, want %q (FIFO broken)", i, drained[i].Text, want)
		}
	}
}

func TestQueueConcurrentAppendSameSession(t *testing.T) {
	q := newTempQueue(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f := &Frame{Type: FrameDeliver, From: "a", To: "z", Text: "x"}
			_ = q.Append("z", f)
			_ = i
		}(i)
	}
	wg.Wait()
	n, err := q.Len("z")
	if err != nil {
		t.Fatal(err)
	}
	if n != 20 {
		t.Errorf("Len = %d, want 20 (concurrent appends should not drop frames)", n)
	}
}

func TestQueueSkipsMalformedLines(t *testing.T) {
	q := newTempQueue(t)
	// Hand-write a file with one valid + one garbage + one valid.
	path := filepath.Join(q.Dir, "mix.jsonl")
	contents := `{"type":"deliver","to":"mix","text":"good1"}
not-json-at-all
{"type":"deliver","to":"mix","text":"good2"}
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	frames, err := q.Drain("mix")
	if err == nil {
		t.Error("expected parse-error report, got nil")
	}
	if !strings.Contains(err.Error(), "parse errors") {
		t.Errorf("error = %q, want mention of parse errors", err)
	}
	if len(frames) != 2 {
		t.Errorf("recovered %d frames, want 2", len(frames))
	}
}

func TestNewQueueRejectsEmptyDir(t *testing.T) {
	if _, err := NewQueue(""); err == nil {
		t.Error("NewQueue('') expected error")
	}
}

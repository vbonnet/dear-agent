package synchub

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/a2a"
)

// Tests for the fixes applied as a result of the 2026-05-26 multi-persona
// review. Each names the finding it pins.

// F-U8: per-question TTL override.
func TestAskQuestion_PerQuestionTTL(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	h := newTestHub(t, Options{
		RoundTTL: 10 * time.Minute, // hub default is generous
		now:      clock.Now,
	})
	ctx := context.Background()

	qid, _ := h.AskQuestion(ctx, "quick?", WithTTL(100*time.Millisecond))
	clock.Advance(200 * time.Millisecond)
	_, err := h.Answer(ctx, qid, SurfaceTerminal, []byte("late"))
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("per-question TTL ignored: err=%v, want ErrExpired", err)
	}
}

// F-U3: error responses carry structured details (winner, at, qid).
func TestServer_ErrorDetailsExposed(t *testing.T) {
	_, _, c1 := startTestServer(t)
	c2 := *c1
	ctx := context.Background()

	qid, _ := c1.AskQuestion(ctx, "?")
	if _, err := c1.Answer(ctx, qid, SurfaceTerminal, []byte("first")); err != nil {
		t.Fatal(err)
	}
	_, err := c2.Answer(ctx, qid, SurfaceDiscord, []byte("second"))
	var re *RemoteError
	if !errors.As(err, &re) {
		t.Fatalf("expected *RemoteError, got %T", err)
	}
	d := re.Details()
	if d["winner"] != string(SurfaceTerminal) {
		t.Errorf("details.winner=%v, want %s", d["winner"], SurfaceTerminal)
	}
	if d["question_id"] != string(qid) {
		t.Errorf("details.question_id=%v, want %s", d["question_id"], qid)
	}
	if _, ok := d["at_unix_ms"]; !ok {
		t.Errorf("details.at_unix_ms missing")
	}
}

// F-Sc1: acqHandles is per-Server, not package-global.
func TestServer_AcqHandlesPerServer(t *testing.T) {
	// Two servers in one process. Lock-handle fences must not collide
	// across them in a way that would let one server release the
	// other's locks.
	_, srvA, clientA := startTestServer(t)
	_, srvB, clientB := startTestServer(t)
	if srvA == srvB {
		t.Fatal("test bug: same server")
	}
	ctx := context.Background()

	hA, err := clientA.Acquire(ctx, "input", "a", AcquireOptions{})
	if err != nil {
		t.Fatal(err)
	}
	hB, err := clientB.Acquire(ctx, "input", "b", AcquireOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// Releasing A's handle must not affect B's lock.
	if err := hA.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := srvB.hub.InspectLock("input"); !ok {
		t.Fatal("server B's lock freed by server A's release — acqHandles is not isolated")
	}
	_ = hB.Release(ctx)
}

// F-S1: token file is exclusively created (O_EXCL).
func TestServer_TokenFileExclusiveCreate(t *testing.T) {
	h := newTestHub(t, Options{SessionID: "x"})
	dir := t.TempDir()
	tokFile := filepath.Join(dir, "synchub.token")
	// Pre-create the path with foreign content; the constructor must
	// not silently inherit it.
	if err := os.WriteFile(tokFile, []byte("attacker-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv, err := NewServer(h, ServerOptions{
		Listen:    "127.0.0.1:0",
		TokenFile: tokFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	contents, _ := os.ReadFile(tokFile)
	if string(contents) == "attacker-content" {
		t.Fatal("token file contains pre-existing attacker content; writeToken did not displace it")
	}
}

// F-Sc4: in-mem bus tracks drops when subscribers fall behind.
func TestInMemBus_DropsCounter(t *testing.T) {
	bus := a2a.NewInMemoryBus().(interface {
		a2a.Bus
		Drops() uint64
	})
	t.Cleanup(func() { _ = bus.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe but never read. Buffer is 256.
	_, _ = bus.Subscribe(ctx, "t")

	// Publish 300 messages; buffer fills at 256 and remaining 44 drop.
	for i := 0; i < 300; i++ {
		_ = bus.Publish(ctx, a2a.Message{Topic: "t"})
	}
	if d := bus.Drops(); d < 30 {
		// Allow some slack — exact count depends on ordering of the
		// non-blocking send vs buffer fill — but we should see drops.
		t.Fatalf("expected >=30 drops with full buffer, got %d", d)
	}
}

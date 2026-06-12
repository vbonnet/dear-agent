package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// TestMesh_EndToEnd wires the three real concrete supervisors with their
// in-memory substrates, runs the mesh for ~150ms, and asserts that:
//
//   - Every supervisor completes at least one tick.
//   - The Meta-Orchestrator's accept actually moves a submitted proposal
//     out of pending into accepted (the Roadmap-authority invariant in
//     CONTEXT.md §"Roadmap authority").
//   - The Orchestrator dispatches a pre-enqueued task.
//   - The Overseer's escalation fires when the probe reports an over-
//     threshold metric.
//   - Each Loop's CheckCount equals 2 × TickCount (mutual-unblock-first
//     ran against both peers every iteration — never skipped).
//
// This is the integration test that distinguishes "doctrine on paper" from
// "doctrine in running code".
func TestMesh_EndToEnd(t *testing.T) {
	trail, buf := newBufferTrail()
	t.Cleanup(func() { _ = trail.Close() })

	roadmap := NewInMemoryRoadmap()
	must(t, roadmap.Submit(WorkProposal{
		ID: "p-end2end-1", Title: "land VROOM PR 1", Reason: "close spec-compliance gap",
	}))

	queue := NewInMemoryQueue()
	must(t, queue.Enqueue(Task{ID: "t-end2end-1", Title: "lint sweep", Worker: "coder"}))

	probe := NewInMemoryResourceProbe()
	probe.Set(ResourceSnapshot{
		DiskUsedFraction: 0.95, // over default threshold
	})

	metaSup, err := NewMetaOrchestrator(trail, roadmap)
	if err != nil {
		t.Fatalf("NewMetaOrchestrator: %v", err)
	}
	orchSup, err := NewOrchestrator(trail, queue)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}
	overSup, err := NewOverseer(trail, probe, EscalationThreshold{})
	if err != nil {
		t.Fatalf("NewOverseer: %v", err)
	}

	check := &HeartbeatCheckSkill{Threshold: 1 * time.Second}
	interval := 5 * time.Millisecond
	metaLoop, err := NewLoop(LoopConfig{
		Supervisor: metaSup,
		Mesh:       placeholderMesh{}, // replaced by NewMesh wiring
		Check:      check,
		Trail:      trail,
		Interval:   interval,
	})
	if err != nil {
		t.Fatalf("NewLoop(meta): %v", err)
	}
	orchLoop, err := NewLoop(LoopConfig{
		Supervisor: orchSup, Mesh: placeholderMesh{}, Check: check, Trail: trail, Interval: interval,
	})
	if err != nil {
		t.Fatalf("NewLoop(orch): %v", err)
	}
	overLoop, err := NewLoop(LoopConfig{
		Supervisor: overSup, Mesh: placeholderMesh{}, Check: check, Trail: trail, Interval: interval,
	})
	if err != nil {
		t.Fatalf("NewLoop(over): %v", err)
	}

	mesh, err := NewMesh(metaLoop, orchLoop, overLoop)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := mesh.Run(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("mesh.Run returned non-cancellation err: %v", err)
	}

	// Every supervisor ticked at least once.
	for _, l := range mesh.Loops() {
		if l.TickCount() == 0 {
			t.Errorf("supervisor %q ran zero ticks", l.Role())
		}
		// Mutual-unblock-first: every iteration calls Check on both peers.
		if got, want := l.CheckCount(), l.TickCount()*2; got != want {
			t.Errorf("supervisor %q CheckCount=%d, want TickCount*2=%d (mutual-unblock-first was skipped)", l.Role(), got, want)
		}
	}

	// Meta-Orchestrator policy effect: the submitted proposal must have moved.
	if got := roadmap.Accepted(); len(got) != 1 || got[0] != "p-end2end-1" {
		t.Errorf("roadmap.Accepted() = %v, want [p-end2end-1]", got)
	}

	// Orchestrator policy effect: the enqueued task was dispatched.
	if got := queue.Dispatched(); len(got) != 1 || got[0] != "t-end2end-1" {
		t.Errorf("queue.Dispatched() = %v, want [t-end2end-1]", got)
	}

	// Overseer policy effect: at least one escalation record for disk.
	sawDiskEscalation := false
	sawAllThreeStartups := map[Role]bool{}
	for _, r := range parseTrail(t, buf) {
		switch r["kind"] {
		case "supervisor.over.escalated":
			p := r["payload"].(map[string]any)
			if p["metric"] == "disk" {
				sawDiskEscalation = true
			}
		case "supervisor.loop.start":
			role := Role(r["role"].(string))
			sawAllThreeStartups[role] = true
		}
	}
	if !sawDiskEscalation {
		t.Error("no disk escalation record — Overseer never fired even though probe was over threshold")
	}
	for _, r := range AllRoles() {
		if !sawAllThreeStartups[r] {
			t.Errorf("no supervisor.loop.start for %q — startup heartbeat missing", r)
		}
	}
}

// placeholderMesh satisfies PeerLookup so NewLoop's validation passes.
// Mesh.NewMesh rewires each Loop's mesh pointer afterwards, so peer lookups
// at runtime go through the real Mesh.
type placeholderMesh struct{}

func (placeholderMesh) Get(Role) (LoopStatus, bool) { return nil, false }

// Compile-time assertion that our concrete supervisors satisfy the
// Supervisor interface. If this stops compiling, a supervisor lost its
// Role() or Tick() method.
var (
	_ Supervisor = (*MetaOrchestrator)(nil)
	_ Supervisor = (*Orchestrator)(nil)
	_ Supervisor = (*Overseer)(nil)
)

// Compile-time assertion that Loop is a valid LoopStatus.
var _ LoopStatus = (*Loop)(nil)

// Compile-time assertion that decisiontrail.JSONLTrail is a Trail. (Helps
// catch breaking changes to the Trail interface.)
var _ decisiontrail.Trail = (*decisiontrail.JSONLTrail)(nil)

package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// nopWriteCloser adapts a bytes.Buffer to io.WriteCloser for trail tests.
type nopWriteCloser struct{ *bytes.Buffer }

func (n *nopWriteCloser) Close() error { return nil }

func newBufferTrail() (*decisiontrail.JSONLTrail, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return decisiontrail.NewJSONLTrail(&nopWriteCloser{buf}), buf
}

// fakeSupervisor lets tests drive Tick behavior without a real supervisor.
type fakeSupervisor struct {
	role  Role
	tick  func(ctx context.Context) error
	calls atomic.Uint64
}

func (f *fakeSupervisor) Role() Role { return f.role }
func (f *fakeSupervisor) Tick(ctx context.Context) error {
	f.calls.Add(1)
	if f.tick != nil {
		return f.tick(ctx)
	}
	return nil
}

// fakePeerLookup returns a fixed map; useful when we want to bypass the
// Mesh and inject specific peer LoopStatus stubs.
type fakePeerLookup struct {
	peers map[Role]LoopStatus
}

func (f *fakePeerLookup) Get(r Role) (LoopStatus, bool) {
	p, ok := f.peers[r]
	return p, ok
}

// fakePeerStatus implements LoopStatus with hard-coded values.
type fakePeerStatus struct {
	role     Role
	beat     time.Time
	tickErr  error
}

func (f *fakePeerStatus) Role() Role               { return f.role }
func (f *fakePeerStatus) LastHeartbeat() time.Time { return f.beat }
func (f *fakePeerStatus) LastTickError() error     { return f.tickErr }

// recordingCheck collects every (peer, called-at) pair and returns its
// configured error.
type recordingCheck struct {
	mu     sync.Mutex
	calls  []Role
	result error
}

func (r *recordingCheck) Check(_ context.Context, peer LoopStatus) error {
	r.mu.Lock()
	r.calls = append(r.calls, peer.Role())
	r.mu.Unlock()
	return r.result
}

func newLoopForTest(t *testing.T, sup Supervisor, mesh PeerLookup, check CheckSkill) (*Loop, *bytes.Buffer) {
	t.Helper()
	trail, buf := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor: sup,
		Mesh:       mesh,
		Check:      check,
		Trail:      trail,
		Interval:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}
	return l, buf
}

func TestNewLoop_RejectsBadConfig(t *testing.T) {
	trail, _ := newBufferTrail()
	good := LoopConfig{
		Supervisor: &fakeSupervisor{role: RoleOrchestrator},
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Trail:      trail,
		Interval:   1 * time.Second,
	}
	if _, err := NewLoop(good); err != nil {
		t.Fatalf("baseline NewLoop: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(c *LoopConfig)
	}{
		{"nil supervisor", func(c *LoopConfig) { c.Supervisor = nil }},
		{"invalid role", func(c *LoopConfig) { c.Supervisor = &fakeSupervisor{role: "verifier"} }},
		{"nil mesh", func(c *LoopConfig) { c.Mesh = nil }},
		{"nil check", func(c *LoopConfig) { c.Check = nil }},
		{"nil trail", func(c *LoopConfig) { c.Trail = nil }},
		{"zero interval", func(c *LoopConfig) { c.Interval = 0 }},
		{"negative interval", func(c *LoopConfig) { c.Interval = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := good
			tc.mutate(&cfg)
			if _, err := NewLoop(cfg); err == nil {
				t.Errorf("NewLoop with %s returned nil error", tc.name)
			}
		})
	}
}

func TestLoop_Iterate_ChecksPeersBeforeTick(t *testing.T) {
	// The mutual-unblock-first invariant: peer checks come before Tick.
	// We assert this by recording the order of events in the trail.
	sup := &fakeSupervisor{role: RoleOrchestrator}
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: time.Now()},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: time.Now()},
	}}
	check := &recordingCheck{}
	l, buf := newLoopForTest(t, sup, peers, check)

	l.iterate(context.Background())

	if sup.calls.Load() != 1 {
		t.Fatalf("Tick called %d times, want 1", sup.calls.Load())
	}
	// Both peers checked.
	if len(check.calls) != 2 {
		t.Fatalf("Check called %d times, want 2: %v", len(check.calls), check.calls)
	}
	// Trail ordering: peer-check records precede tick.start, which
	// precedes tick.end.
	kinds := trailKinds(t, buf)
	if len(kinds) < 4 {
		t.Fatalf("trail too short: %v", kinds)
	}
	firstTickStart := -1
	for i, k := range kinds {
		if k == "supervisor.tick.start" {
			firstTickStart = i
			break
		}
	}
	if firstTickStart == -1 {
		t.Fatalf("no tick.start in trail: %v", kinds)
	}
	for i, k := range kinds[:firstTickStart] {
		if k != "supervisor.check.peer" {
			t.Errorf("trail[%d] = %q, want only supervisor.check.peer before tick.start (full=%v)", i, k, kinds)
		}
	}
}

func TestLoop_Iterate_ChecksBothPeersInDoctrineOrder(t *testing.T) {
	// For Orchestrator: verifies = Overseer, unsticks = Meta-O.
	// PeerList returns [verifies, unsticks] in that order.
	sup := &fakeSupervisor{role: RoleOrchestrator}
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: time.Now()},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: time.Now()},
	}}
	check := &recordingCheck{}
	l, _ := newLoopForTest(t, sup, peers, check)
	l.iterate(context.Background())

	want := []Role{RoleOverseer, RoleMetaOrchestrator}
	if len(check.calls) != 2 {
		t.Fatalf("len(check.calls) = %d, want 2", len(check.calls))
	}
	for i, w := range want {
		if check.calls[i] != w {
			t.Errorf("check.calls[%d] = %q, want %q", i, check.calls[i], w)
		}
	}
}

func TestLoop_Iterate_MissingPeerRecordsToTrail(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	// Empty mesh — both peers will be missing.
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{}}
	check := &recordingCheck{}
	l, buf := newLoopForTest(t, sup, peers, check)
	l.iterate(context.Background())

	if len(check.calls) != 0 {
		t.Errorf("Check called %d times for missing peers, want 0", len(check.calls))
	}
	kinds := trailKinds(t, buf)
	missing := 0
	for _, k := range kinds {
		if k == "supervisor.check.peer_missing" {
			missing++
		}
	}
	if missing != 2 {
		t.Errorf("trail has %d peer_missing records, want 2 (kinds=%v)", missing, kinds)
	}
}

func TestLoop_Tick_RecordsErrorButContinues(t *testing.T) {
	wantErr := errors.New("simulated tick failure")
	sup := &fakeSupervisor{role: RoleOverseer, tick: func(context.Context) error { return wantErr }}
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: time.Now()},
		RoleOrchestrator:     &fakePeerStatus{role: RoleOrchestrator, beat: time.Now()},
	}}
	l, buf := newLoopForTest(t, sup, peers, CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }))

	l.iterate(context.Background())

	if got := l.LastTickError(); !errors.Is(got, wantErr) {
		t.Errorf("LastTickError = %v, want %v", got, wantErr)
	}
	if l.LastHeartbeat().IsZero() {
		t.Error("LastHeartbeat is still zero after a failing Tick — failures must still stamp the heartbeat or the mesh will deadlock")
	}
	if l.TickCount() != 1 {
		t.Errorf("TickCount = %d, want 1 (failure must still count)", l.TickCount())
	}
	// Trail records the tick.end with ok=false + error.
	found := false
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var rec decisiontrail.Record
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if rec.Kind == "supervisor.tick.end" {
			if rec.Payload["ok"] != false {
				t.Errorf("tick.end.ok = %v, want false", rec.Payload["ok"])
			}
			if !strings.Contains(rec.Payload["error"].(string), "simulated") {
				t.Errorf("tick.end.error = %v, missing expected substring", rec.Payload["error"])
			}
			found = true
		}
	}
	if !found {
		t.Error("no supervisor.tick.end record in trail")
	}
}

func TestLoop_Run_StartupHeartbeatBeforeFirstCheck(t *testing.T) {
	// Doctrine: peers must not see a zero heartbeat for a supervisor
	// that just started. Loop.Run publishes a startup heartbeat before
	// the first iteration.
	sup := &fakeSupervisor{role: RoleMetaOrchestrator}
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:     &fakePeerStatus{role: RoleOverseer, beat: time.Now()},
		RoleOrchestrator: &fakePeerStatus{role: RoleOrchestrator, beat: time.Now()},
	}}
	l, _ := newLoopForTest(t, sup, peers, CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }))

	before := l.LastHeartbeat()
	if !before.IsZero() {
		t.Fatalf("LastHeartbeat before Run = %v, want zero", before)
	}

	// Run for a short bounded duration.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_ = l.Run(ctx)

	if l.LastHeartbeat().IsZero() {
		t.Error("LastHeartbeat is zero after Run — startup heartbeat not published")
	}
}

func TestLoop_Run_StopsOnContextCancel(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: time.Now()},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: time.Now()},
	}}
	l, _ := newLoopForTest(t, sup, peers, CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Run(ctx) }()

	// Let it run at least one iteration.
	time.Sleep(5 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return within 500ms of context cancel")
	}
	if l.TickCount() == 0 {
		t.Error("Run completed zero ticks before cancellation — interval too long or loop not running")
	}
}

func TestLoop_CheckCount_IncrementsForEachCheck(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: time.Now()},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: time.Now()},
	}}
	l, _ := newLoopForTest(t, sup, peers, CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }))

	l.iterate(context.Background())
	if l.CheckCount() != 2 {
		t.Errorf("CheckCount after 1 iteration = %d, want 2 (one per peer)", l.CheckCount())
	}
	l.iterate(context.Background())
	if l.CheckCount() != 4 {
		t.Errorf("CheckCount after 2 iterations = %d, want 4", l.CheckCount())
	}
}

func TestLoop_Iterate_RecoveryCalledOnStalePeer(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	staleTime := time.Now().Add(-10 * time.Minute)
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: staleTime},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: time.Now()},
	}}
	check := &HeartbeatCheckSkill{Threshold: 30 * time.Second}
	recovery := NewInMemoryPeerRecovery()

	trail, buf := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor: sup,
		Mesh:       peers,
		Check:      check,
		Recovery:   recovery,
		Trail:      trail,
		Interval:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	l.iterate(context.Background())

	calls := recovery.Calls()
	if len(calls) != 1 {
		t.Fatalf("recovery calls = %d, want 1 (stale overseer)", len(calls))
	}
	if calls[0].PeerRole != RoleOverseer {
		t.Errorf("recovery peer = %s, want overseer", calls[0].PeerRole)
	}

	kinds := trailKinds(t, buf)
	recoveryAttempts := 0
	for _, k := range kinds {
		if k == "supervisor.check.recovery_attempt" {
			recoveryAttempts++
		}
	}
	if recoveryAttempts != 1 {
		t.Errorf("trail has %d recovery_attempt records, want 1 (kinds=%v)", recoveryAttempts, kinds)
	}
}

func TestLoop_Iterate_NoRecoveryWhenNil(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	staleTime := time.Now().Add(-10 * time.Minute)
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: staleTime},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: time.Now()},
	}}
	check := &HeartbeatCheckSkill{Threshold: 30 * time.Second}
	l, buf := newLoopForTest(t, sup, peers, check)

	l.iterate(context.Background())

	kinds := trailKinds(t, buf)
	for _, k := range kinds {
		if k == "supervisor.check.recovery_attempt" {
			t.Error("recovery_attempt should not appear when Recovery is nil")
		}
	}
}

func TestLoop_Iterate_NoRecoveryWhenPeerHealthy(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: time.Now()},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: time.Now()},
	}}
	check := &HeartbeatCheckSkill{Threshold: 30 * time.Second}
	recovery := NewInMemoryPeerRecovery()

	trail, _ := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor: sup,
		Mesh:       peers,
		Check:      check,
		Recovery:   recovery,
		Trail:      trail,
		Interval:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	l.iterate(context.Background())

	if len(recovery.Calls()) != 0 {
		t.Errorf("recovery calls = %d, want 0 (all peers healthy)", len(recovery.Calls()))
	}
}

func TestLoop_Iterate_RecoveryErrorRecordedButContinues(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	staleTime := time.Now().Add(-10 * time.Minute)
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: staleTime},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: staleTime},
	}}
	check := &HeartbeatCheckSkill{Threshold: 30 * time.Second}
	recovery := NewInMemoryPeerRecovery()
	recovery.SetError(fmt.Errorf("agm send failed"))

	trail, buf := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor: sup,
		Mesh:       peers,
		Check:      check,
		Recovery:   recovery,
		Trail:      trail,
		Interval:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	l.iterate(context.Background())

	calls := recovery.Calls()
	if len(calls) != 2 {
		t.Fatalf("recovery calls = %d, want 2 (both peers stale)", len(calls))
	}
	if sup.calls.Load() != 1 {
		t.Error("Tick should still run even when recovery fails")
	}

	kinds := trailKinds(t, buf)
	recoveryAttempts := 0
	for _, k := range kinds {
		if k == "supervisor.check.recovery_attempt" {
			recoveryAttempts++
		}
	}
	if recoveryAttempts != 2 {
		t.Errorf("trail has %d recovery_attempt records, want 2", recoveryAttempts)
	}
}

func TestLoop_ZombieSelfTerminatesWhenAllPeersDark(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	staleTime := time.Now().Add(-10 * time.Minute)
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: staleTime},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: staleTime},
	}}
	check := &HeartbeatCheckSkill{Threshold: 30 * time.Second}

	trail, buf := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor:      sup,
		Mesh:            peers,
		Check:           check,
		Trail:           trail,
		Interval:        1 * time.Millisecond,
		ZombieThreshold: 3,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runErr := l.Run(ctx)

	if !errors.Is(runErr, ErrAllPeersDark) {
		t.Fatalf("Run err = %v, want ErrAllPeersDark", runErr)
	}
	if l.TickCount() < 3 {
		t.Errorf("TickCount = %d, want >= 3 (should run threshold iterations before exit)", l.TickCount())
	}

	kinds := trailKinds(t, buf)
	found := false
	for _, k := range kinds {
		if k == "supervisor.loop.zombie_exit" {
			found = true
		}
	}
	if !found {
		t.Error("expected supervisor.loop.zombie_exit in trail")
	}
}

func TestLoop_ZombieResetsWhenOnePeerRecovers(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	staleTime := time.Now().Add(-10 * time.Minute)
	freshTime := time.Now()

	// Start with both stale, but overseer recovers after 2 iterations
	iterCount := 0
	overseerBeat := staleTime
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: staleTime},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: staleTime},
	}}

	// Use a CheckSkill that alternates the overseer's status
	check := CheckSkillFunc(func(_ context.Context, peer LoopStatus) error {
		if peer.Role() == RoleOverseer {
			iterCount++
			if iterCount > 4 {
				overseerBeat = freshTime
				return nil
			}
		}
		_ = overseerBeat // keep compiler happy
		return fmt.Errorf("peer %q is stale", peer.Role())
	})

	trail, _ := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor:      sup,
		Mesh:            peers,
		Check:           check,
		Trail:           trail,
		Interval:        1 * time.Millisecond,
		ZombieThreshold: 10,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	runErr := l.Run(ctx)

	if errors.Is(runErr, ErrAllPeersDark) {
		t.Error("Run should NOT have zombie-exited — overseer recovered")
	}
	if !errors.Is(runErr, context.DeadlineExceeded) && !errors.Is(runErr, context.Canceled) {
		t.Errorf("Run err = %v, want context error (graceful shutdown)", runErr)
	}
}

func TestLoop_ZombieDisabledWhenNegativeThreshold(t *testing.T) {
	sup := &fakeSupervisor{role: RoleOrchestrator}
	staleTime := time.Now().Add(-10 * time.Minute)
	peers := &fakePeerLookup{peers: map[Role]LoopStatus{
		RoleOverseer:         &fakePeerStatus{role: RoleOverseer, beat: staleTime},
		RoleMetaOrchestrator: &fakePeerStatus{role: RoleMetaOrchestrator, beat: staleTime},
	}}
	check := &HeartbeatCheckSkill{Threshold: 30 * time.Second}

	trail, _ := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor:      sup,
		Mesh:            peers,
		Check:           check,
		Trail:           trail,
		Interval:        1 * time.Millisecond,
		ZombieThreshold: -1,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	runErr := l.Run(ctx)

	if errors.Is(runErr, ErrAllPeersDark) {
		t.Error("Run should NOT zombie-exit when ZombieThreshold is negative (disabled)")
	}
	if l.TickCount() < 2 {
		t.Errorf("TickCount = %d, want >= 2 (should keep running)", l.TickCount())
	}
}

// trailKinds reads buf as JSONL and returns the Kind of each record.
func trailKinds(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	var out []string
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec decisiontrail.Record
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("trail JSON parse: %v (line=%q)", err, line)
		}
		out = append(out, rec.Kind)
	}
	return out
}

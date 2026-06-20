package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// newTestLoop is a thin wrapper that constructs a Loop wired to a buffer
// trail and a no-op CheckSkill. Used for Mesh-level tests where the inner
// Loop behavior is not under test.
func newTestLoop(t *testing.T, role Role) *Loop {
	t.Helper()
	trail, _ := newBufferTrail()
	l, err := NewLoop(LoopConfig{
		Supervisor: &fakeSupervisor{role: role},
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}}, // placeholder; NewMesh rewires
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Trail:      trail,
		Interval:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewLoop(%q): %v", role, err)
	}
	return l
}

func TestNewMesh_RequiresAllThreeRoles(t *testing.T) {
	cases := []struct {
		name    string
		roles   []Role
		wantErr string
	}{
		{"all three", []Role{RoleMetaOrchestrator, RoleOrchestrator, RoleOverseer}, ""},
		{"any order", []Role{RoleOverseer, RoleMetaOrchestrator, RoleOrchestrator}, ""},
		{"missing meta-o", []Role{RoleOrchestrator, RoleOverseer}, "missing"},
		{"missing orch", []Role{RoleMetaOrchestrator, RoleOverseer}, "missing"},
		{"missing overseer", []Role{RoleMetaOrchestrator, RoleOrchestrator}, "missing"},
		{"duplicate", []Role{RoleMetaOrchestrator, RoleMetaOrchestrator, RoleOverseer}, "duplicate"},
		{"empty", nil, "missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loops := make([]*Loop, 0, len(tc.roles))
			for _, r := range tc.roles {
				loops = append(loops, newTestLoop(t, r))
			}
			_, err := NewMesh(loops...)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("NewMesh: %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("NewMesh err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestNewMesh_NilLoopRejected(t *testing.T) {
	_, err := NewMesh(newTestLoop(t, RoleMetaOrchestrator), nil, newTestLoop(t, RoleOverseer))
	if err == nil {
		t.Error("NewMesh accepted a nil Loop")
	}
}

func TestNewMesh_WiresPeerLookupToSelf(t *testing.T) {
	// After NewMesh, each Loop's PeerLookup must point at the Mesh, so
	// peer resolution actually works at runtime.
	meta := newTestLoop(t, RoleMetaOrchestrator)
	orch := newTestLoop(t, RoleOrchestrator)
	over := newTestLoop(t, RoleOverseer)
	m, err := NewMesh(meta, orch, over)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}
	// Meta-O's peers are Orch and Over; both must resolve through its
	// rewired mesh.
	peers, err := meta.Role().PeerList()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range peers {
		got, ok := meta.mesh.Get(r)
		if !ok {
			t.Errorf("Meta-O peer %q did not resolve through mesh", r)
		}
		if got != nil && got.Role() != r {
			t.Errorf("Meta-O peer %q resolved to role %q", r, got.Role())
		}
	}
	// Mesh.Loops returns canonical order.
	loops := m.Loops()
	if len(loops) != 3 {
		t.Fatalf("Loops len = %d, want 3", len(loops))
	}
	wantOrder := AllRoles()
	for i, l := range loops {
		if l.Role() != wantOrder[i] {
			t.Errorf("Loops[%d] = %q, want %q (canonical order)", i, l.Role(), wantOrder[i])
		}
	}
}

func TestMesh_Run_AllThreeIterate(t *testing.T) {
	meta := newTestLoop(t, RoleMetaOrchestrator)
	orch := newTestLoop(t, RoleOrchestrator)
	over := newTestLoop(t, RoleOverseer)
	m, err := NewMesh(meta, orch, over)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = m.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("Run err = %v, want context.DeadlineExceeded or Canceled", err)
	}
	for _, l := range m.Loops() {
		if l.TickCount() == 0 {
			t.Errorf("supervisor %q ran zero ticks before shutdown", l.Role())
		}
	}
}

func TestMesh_Run_SelectiveRestartOnTickError(t *testing.T) {
	// One supervisor returns an error on its first tick, then succeeds.
	// The mesh should restart that loop — not kill all three.
	var failCount int
	failingSup := &fakeSupervisor{role: RoleOverseer, tick: func(_ context.Context) error {
		failCount++
		if failCount == 1 {
			return errors.New("transient failure")
		}
		return nil
	}}

	trail1, _ := newBufferTrail()
	trail2, _ := newBufferTrail()
	trail3, _ := newBufferTrail()

	meta, _ := NewLoop(LoopConfig{
		Supervisor: &fakeSupervisor{role: RoleMetaOrchestrator},
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Trail:      trail1,
		Interval:   1 * time.Millisecond,
	})
	orch, _ := NewLoop(LoopConfig{
		Supervisor: &fakeSupervisor{role: RoleOrchestrator},
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Trail:      trail2,
		Interval:   1 * time.Millisecond,
	})
	over, _ := NewLoop(LoopConfig{
		Supervisor: failingSup,
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Trail:      trail3,
		Interval:   1 * time.Millisecond,
	})

	m, err := NewMesh(meta, orch, over)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	runErr := m.Run(ctx)

	if !errors.Is(runErr, context.DeadlineExceeded) && !errors.Is(runErr, context.Canceled) {
		t.Errorf("Run err = %v, want context error (mesh should survive transient failures)", runErr)
	}
	for _, l := range m.Loops() {
		if l.TickCount() == 0 {
			t.Errorf("supervisor %q ran zero ticks — selective restart failed", l.Role())
		}
	}
	if meta.TickCount() < 2 {
		t.Errorf("meta-o ticks = %d, want ≥2 (should keep running while overseer restarts)", meta.TickCount())
	}
}

func TestMesh_Run_PanicCancelsAll(t *testing.T) {
	trail1, _ := newBufferTrail()
	trail2, _ := newBufferTrail()
	trail3, _ := newBufferTrail()

	meta, _ := NewLoop(LoopConfig{
		Supervisor: &fakeSupervisor{role: RoleMetaOrchestrator},
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Trail:      trail1,
		Interval:   1 * time.Millisecond,
	})
	orch, _ := NewLoop(LoopConfig{
		Supervisor: &fakeSupervisor{role: RoleOrchestrator},
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Trail:      trail2,
		Interval:   1 * time.Millisecond,
	})
	panickingSup := &fakeSupervisor{role: RoleOverseer, tick: func(_ context.Context) error {
		panic("simulated crash")
	}}
	over, _ := NewLoop(LoopConfig{
		Supervisor: panickingSup,
		Mesh:       &fakePeerLookup{peers: map[Role]LoopStatus{}},
		Check:      CheckSkillFunc(func(context.Context, LoopStatus) error { return nil }),
		Trail:      trail3,
		Interval:   1 * time.Millisecond,
	})

	m, err := NewMesh(meta, orch, over)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	runErr := m.Run(ctx)

	if runErr == nil || !strings.Contains(runErr.Error(), "panicked") {
		t.Errorf("Run err = %v, want error containing 'panicked'", runErr)
	}
}

func TestMesh_Get_UnknownRole(t *testing.T) {
	m, err := NewMesh(
		newTestLoop(t, RoleMetaOrchestrator),
		newTestLoop(t, RoleOrchestrator),
		newTestLoop(t, RoleOverseer),
	)
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}
	if _, ok := m.Get(Role("verifier")); ok {
		t.Error("Mesh.Get returned true for retired role 'verifier'")
	}
}

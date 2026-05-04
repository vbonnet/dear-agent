package plugin

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/audit"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// fakePlugin implements Plugin and (optionally) HookProvider /
// CheckProvider via embedded function fields, so tests can build a
// plugin with whatever combination of behaviours they need.
type fakePlugin struct {
	manifest Manifest
	hooks    *workflow.Hooks // nil = does not implement HookProvider
	checks   []audit.Check   // nil = does not implement CheckProvider
}

func (f *fakePlugin) Manifest() Manifest { return f.manifest }

// hookFakePlugin embeds fakePlugin and exposes Hooks(). Used when a
// test needs HookProvider; fakePlugin alone does not satisfy it
// because returning workflow.Hooks{} from a *fakePlugin would still
// fail the type assertion (no Hooks method on the embedded type).
type hookFakePlugin struct{ *fakePlugin }

func (h *hookFakePlugin) Hooks() workflow.Hooks {
	if h.hooks == nil {
		return workflow.Hooks{}
	}
	return *h.hooks
}

type checkFakePlugin struct{ *fakePlugin }

func (c *checkFakePlugin) Checks() []audit.Check { return c.checks }

// bothFakePlugin satisfies both HookProvider and CheckProvider, used
// for the coherence-check tests where the manifest must list both
// capabilities.
type bothFakePlugin struct{ *fakePlugin }

func (b *bothFakePlugin) Hooks() workflow.Hooks {
	if b.hooks == nil {
		return workflow.Hooks{}
	}
	return *b.hooks
}
func (b *bothFakePlugin) Checks() []audit.Check { return b.checks }

func makeManifest(name string, caps ...Capability) Manifest {
	return Manifest{
		APIVersion:   APIVersionV1,
		Kind:         KindPlugin,
		Name:         name,
		Version:      "0.1.0",
		Capabilities: caps,
	}
}

func TestRegistry_Register_Nil(t *testing.T) {
	r := NewRegistry()
	err := r.Register(nil)
	if err == nil || !strings.Contains(err.Error(), "nil plugin") {
		t.Fatalf("expected nil-plugin error, got %v", err)
	}
}

func TestRegistry_Register_BadManifest(t *testing.T) {
	r := NewRegistry()
	p := &fakePlugin{manifest: Manifest{}} // empty: fails Validate
	err := r.Register(p)
	if err == nil {
		t.Fatal("expected validate error")
	}
}

func TestRegistry_Register_DuplicateName(t *testing.T) {
	r := NewRegistry()
	p1 := &fakePlugin{manifest: makeManifest("dup")}
	p2 := &fakePlugin{manifest: makeManifest("dup")}
	if err := r.Register(p1); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := r.Register(p2)
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestRegistry_Register_DeclaresHooksWithoutImplementing(t *testing.T) {
	r := NewRegistry()
	// Manifest declares hooks; *fakePlugin does not satisfy HookProvider.
	p := &fakePlugin{manifest: makeManifest("bad", CapabilityHooks)}
	err := r.Register(p)
	if err == nil || !strings.Contains(err.Error(), "HookProvider") {
		t.Fatalf("expected HookProvider mismatch, got %v", err)
	}
}

func TestRegistry_Register_ImplementsHooksWithoutDeclaring(t *testing.T) {
	r := NewRegistry()
	// Manifest does NOT declare hooks; *hookFakePlugin satisfies HookProvider.
	p := &hookFakePlugin{fakePlugin: &fakePlugin{manifest: makeManifest("bad")}}
	err := r.Register(p)
	if err == nil || !strings.Contains(err.Error(), "HookProvider") {
		t.Fatalf("expected HookProvider mismatch, got %v", err)
	}
}

func TestRegistry_Register_DeclaresChecksWithoutImplementing(t *testing.T) {
	r := NewRegistry()
	p := &fakePlugin{manifest: makeManifest("bad", CapabilityChecks)}
	err := r.Register(p)
	if err == nil || !strings.Contains(err.Error(), "CheckProvider") {
		t.Fatalf("expected CheckProvider mismatch, got %v", err)
	}
}

func TestRegistry_Register_ImplementsChecksWithoutDeclaring(t *testing.T) {
	r := NewRegistry()
	p := &checkFakePlugin{fakePlugin: &fakePlugin{manifest: makeManifest("bad")}}
	err := r.Register(p)
	if err == nil || !strings.Contains(err.Error(), "CheckProvider") {
		t.Fatalf("expected CheckProvider mismatch, got %v", err)
	}
}

func TestRegistry_Register_HappyPath(t *testing.T) {
	r := NewRegistry()
	p := &hookFakePlugin{fakePlugin: &fakePlugin{
		manifest: makeManifest("good", CapabilityHooks),
	}}
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got, ok := r.Lookup("good"); !ok || got != p {
		t.Errorf("Lookup: got=%v ok=%v", got, ok)
	}
	plugins := r.Plugins()
	if len(plugins) != 1 {
		t.Errorf("Plugins len = %d, want 1", len(plugins))
	}
}

func TestRegistry_Plugins_SortedByName(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"c", "a", "b"} {
		p := &fakePlugin{manifest: makeManifest(name)}
		if err := r.Register(p); err != nil {
			t.Fatalf("Register %s: %v", name, err)
		}
	}
	got := r.Plugins()
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Manifest().Name != want {
			t.Errorf("Plugins[%d].Name = %q, want %q", i, got[i].Manifest().Name, want)
		}
	}
}

func TestRegistry_Hooks_Empty(t *testing.T) {
	r := NewRegistry()
	h := r.Hooks()
	if h.OnDefine != nil || h.OnEnforce != nil || h.OnAudit != nil || h.OnResolve != nil {
		t.Errorf("expected zero-value Hooks, got %+v", h)
	}
}

func TestRegistry_Hooks_FanOutOrder(t *testing.T) {
	r := NewRegistry()
	var order []string
	var mu sync.Mutex
	rec := func(name string) func(context.Context, workflow.EnforcePayload) error {
		return func(context.Context, workflow.EnforcePayload) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, name)
			return nil
		}
	}
	for _, name := range []string{"first", "second", "third"} {
		nm := name // closure capture
		p := &hookFakePlugin{fakePlugin: &fakePlugin{
			manifest: makeManifest(nm, CapabilityHooks),
			hooks:    &workflow.Hooks{OnEnforce: rec(nm)},
		}}
		if err := r.Register(p); err != nil {
			t.Fatalf("Register %s: %v", name, err)
		}
	}
	composed := r.Hooks()
	if composed.OnEnforce == nil {
		t.Fatal("OnEnforce should be non-nil")
	}
	if err := composed.OnEnforce(context.Background(), workflow.EnforcePayload{}); err != nil {
		t.Fatalf("OnEnforce: %v", err)
	}
	want := []string{"first", "second", "third"}
	if len(order) != 3 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestRegistry_Hooks_OnEnforce_ShortCircuits(t *testing.T) {
	r := NewRegistry()
	var ran []string
	for _, name := range []string{"a", "b", "c"} {
		nm := name
		fail := nm == "b"
		p := &hookFakePlugin{fakePlugin: &fakePlugin{
			manifest: makeManifest(nm, CapabilityHooks),
			hooks: &workflow.Hooks{
				OnEnforce: func(context.Context, workflow.EnforcePayload) error {
					ran = append(ran, nm)
					if fail {
						return errors.New("policy violation")
					}
					return nil
				},
			},
		}}
		if err := r.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	composed := r.Hooks()
	err := composed.OnEnforce(context.Background(), workflow.EnforcePayload{})
	if err == nil || !strings.Contains(err.Error(), "policy violation") {
		t.Fatalf("expected policy violation, got %v", err)
	}
	if !strings.Contains(err.Error(), `plugin "b"`) {
		t.Errorf("error should name failing plugin: %v", err)
	}
	if len(ran) != 2 || ran[0] != "a" || ran[1] != "b" {
		t.Errorf("ran = %v, want [a b] (c skipped after b failed)", ran)
	}
}

func TestRegistry_Hooks_OnAudit_AccumulatesErrors(t *testing.T) {
	r := NewRegistry()
	var ran []string
	for _, name := range []string{"a", "b", "c"} {
		nm := name
		fail := nm == "a" || nm == "c"
		p := &hookFakePlugin{fakePlugin: &fakePlugin{
			manifest: makeManifest(nm, CapabilityHooks),
			hooks: &workflow.Hooks{
				OnAudit: func(context.Context, workflow.AuditPayload) error {
					ran = append(ran, nm)
					if fail {
						return errors.New(nm + " errored")
					}
					return nil
				},
			},
		}}
		if err := r.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	composed := r.Hooks()
	err := composed.OnAudit(context.Background(), workflow.AuditPayload{})
	if err == nil {
		t.Fatal("expected joined error")
	}
	// All three plugins should have run: substrate guarantee.
	if len(ran) != 3 {
		t.Errorf("ran = %v, want all three", ran)
	}
	// Both failing plugins' errors should be reachable.
	if !strings.Contains(err.Error(), "a errored") || !strings.Contains(err.Error(), "c errored") {
		t.Errorf("joined error missing expected causes: %v", err)
	}
}

func TestRegistry_Hooks_NilFieldsSkipped(t *testing.T) {
	r := NewRegistry()
	// Plugin only sets OnAudit; OnDefine/OnEnforce/OnResolve must end up nil.
	p := &hookFakePlugin{fakePlugin: &fakePlugin{
		manifest: makeManifest("audit-only", CapabilityHooks),
		hooks: &workflow.Hooks{
			OnAudit: func(context.Context, workflow.AuditPayload) error { return nil },
		},
	}}
	if err := r.Register(p); err != nil {
		t.Fatal(err)
	}
	h := r.Hooks()
	if h.OnDefine != nil {
		t.Errorf("OnDefine should be nil")
	}
	if h.OnEnforce != nil {
		t.Errorf("OnEnforce should be nil")
	}
	if h.OnResolve != nil {
		t.Errorf("OnResolve should be nil")
	}
	if h.OnAudit == nil {
		t.Errorf("OnAudit should be non-nil")
	}
}

// fakeCheck is a minimal audit.Check used by ApplyChecks tests.
type fakeCheck struct {
	id string
}

func (c *fakeCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              c.id,
		Description:     "fake",
		Cadence:         audit.CadenceOnDemand,
		SeverityCeiling: audit.SeverityP3,
	}
}
func (c *fakeCheck) Run(_ context.Context, _ audit.Env) (audit.Result, error) {
	return audit.Result{Status: audit.StatusOK}, nil
}

func TestRegistry_ApplyChecks_NilTarget(t *testing.T) {
	r := NewRegistry()
	err := r.ApplyChecks(nil)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected nil-target error, got %v", err)
	}
}

func TestRegistry_ApplyChecks_RegistersAllChecks(t *testing.T) {
	r := NewRegistry()
	p := &checkFakePlugin{fakePlugin: &fakePlugin{
		manifest: makeManifest("checker", CapabilityChecks),
		checks: []audit.Check{
			&fakeCheck{id: "fake.one"},
			&fakeCheck{id: "fake.two"},
		},
	}}
	if err := r.Register(p); err != nil {
		t.Fatal(err)
	}
	target := audit.NewRegistry()
	if err := r.ApplyChecks(target); err != nil {
		t.Fatalf("ApplyChecks: %v", err)
	}
	if _, ok := target.Lookup("fake.one"); !ok {
		t.Errorf("fake.one not registered")
	}
	if _, ok := target.Lookup("fake.two"); !ok {
		t.Errorf("fake.two not registered")
	}
}

func TestRegistry_ApplyChecks_ConflictingCheckIDs(t *testing.T) {
	r := NewRegistry()
	plug := func(name string) *checkFakePlugin {
		return &checkFakePlugin{fakePlugin: &fakePlugin{
			manifest: makeManifest(name, CapabilityChecks),
			checks:   []audit.Check{&fakeCheck{id: "duplicate.id"}},
		}}
	}
	if err := r.Register(plug("first")); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(plug("second")); err != nil {
		t.Fatal(err)
	}
	target := audit.NewRegistry()
	err := r.ApplyChecks(target)
	if err == nil {
		t.Fatal("expected duplicate-ID error")
	}
	// The error should attribute the conflict to one of the plugins.
	if !strings.Contains(err.Error(), `plugin "`) {
		t.Errorf("error should name the failing plugin: %v", err)
	}
}

func TestRegistry_ApplyChecks_BothCapabilities(t *testing.T) {
	// Plugin that contributes both hooks and checks via a single
	// manifest. Smoke test that the dual-interface case wires up.
	r := NewRegistry()
	called := 0
	p := &bothFakePlugin{fakePlugin: &fakePlugin{
		manifest: makeManifest("both", CapabilityHooks, CapabilityChecks),
		hooks: &workflow.Hooks{
			OnAudit: func(context.Context, workflow.AuditPayload) error {
				called++
				return nil
			},
		},
		checks: []audit.Check{&fakeCheck{id: "both.check"}},
	}}
	if err := r.Register(p); err != nil {
		t.Fatal(err)
	}
	target := audit.NewRegistry()
	if err := r.ApplyChecks(target); err != nil {
		t.Fatalf("ApplyChecks: %v", err)
	}
	if _, ok := target.Lookup("both.check"); !ok {
		t.Errorf("both.check missing")
	}
	if err := r.Hooks().OnAudit(context.Background(), workflow.AuditPayload{}); err != nil {
		t.Fatalf("OnAudit: %v", err)
	}
	if called != 1 {
		t.Errorf("hook called %d times, want 1", called)
	}
}

func TestRegistry_Concurrent_RegisterAndRead(t *testing.T) {
	// Smoke test: registering from one goroutine while reading from
	// another must not panic or race. -race surfaces failures here.
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			p := &fakePlugin{manifest: makeManifest("p" + string(rune('0'+i)))}
			_ = r.Register(p)
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Plugins()
			_ = r.Hooks()
		}()
	}
	wg.Wait()
}

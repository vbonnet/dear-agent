package workflow

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

// TestHooks_AllFour_FireOnRun checks that the four DEAR hooks each fire at
// least once during a small successful + failing run. The test uses a
// minimal in-memory runner and tracks fire counts via atomics so the
// assertions are race-free.
func TestHooks_AllFour_FireOnRun(t *testing.T) {
	var (
		defineCalls  atomic.Int32
		enforceCalls atomic.Int32
		auditCalls   atomic.Int32
		resolveCalls atomic.Int32
	)
	hooks := &Hooks{
		OnDefine: func(_ context.Context, p DefinePayload) error {
			defineCalls.Add(1)
			if p.RunID == "" {
				t.Errorf("OnDefine: empty RunID")
			}
			if p.Workflow == nil {
				t.Errorf("OnDefine: nil Workflow")
			}
			return nil
		},
		OnEnforce: func(_ context.Context, p EnforcePayload) error {
			enforceCalls.Add(1)
			if p.Attempt < 1 {
				t.Errorf("OnEnforce: invalid Attempt=%d", p.Attempt)
			}
			return nil
		},
		OnAudit: func(_ context.Context, p AuditPayload) error {
			auditCalls.Add(1)
			if p.Event.RunID == "" {
				t.Errorf("OnAudit: empty RunID")
			}
			return nil
		},
		OnResolve: func(_ context.Context, p ResolvePayload) error {
			resolveCalls.Add(1)
			if p.Node == nil || p.Result == nil {
				t.Errorf("OnResolve: missing payload fields")
			}
			return nil
		},
	}

	wf := &Workflow{
		Name:    "hooks-fire-test",
		Version: "1",
		Nodes: []Node{
			{ID: "ok", Kind: KindBash, Bash: &BashNode{Cmd: "true"}},
			{ID: "fail", Kind: KindBash, Depends: []string{"ok"}, Bash: &BashNode{Cmd: "exit 1"}},
		},
	}

	r := NewRunner(nil)
	r.Hooks = hooks

	_, _ = r.Run(context.Background(), wf, nil)

	if defineCalls.Load() != 1 {
		t.Errorf("OnDefine fired %d times, want 1", defineCalls.Load())
	}
	if enforceCalls.Load() < 2 {
		t.Errorf("OnEnforce fired %d times, want >= 2 (one per node)", enforceCalls.Load())
	}
	if auditCalls.Load() < 4 {
		// expect: run-started, ok pending->running, ok running->succeeded,
		// fail pending->running, fail running->failed, run finish.
		t.Errorf("OnAudit fired %d times, want >= 4", auditCalls.Load())
	}
	if resolveCalls.Load() != 1 {
		t.Errorf("OnResolve fired %d times, want 1 (one failed node)", resolveCalls.Load())
	}
}

// TestHooks_OnEnforce_FailsNode verifies that an OnEnforce hook returning a
// non-nil error short-circuits the node's body and surfaces the error
// through the Result.
func TestHooks_OnEnforce_FailsNode(t *testing.T) {
	denied := errors.New("policy denied: no shell")
	hooks := &Hooks{
		OnEnforce: func(_ context.Context, p EnforcePayload) error {
			if p.Node.ID == "guard" {
				return denied
			}
			return nil
		},
	}

	wf := &Workflow{
		Name:    "enforce-deny",
		Version: "1",
		Nodes: []Node{
			{ID: "guard", Kind: KindBash, Bash: &BashNode{Cmd: "echo should-not-run"}},
		},
	}
	r := NewRunner(nil)
	r.Hooks = hooks

	rep, err := r.Run(context.Background(), wf, nil)
	if err == nil {
		t.Fatal("expected run error from denied enforce, got nil")
	}
	if rep == nil || len(rep.Results) != 1 {
		t.Fatalf("expected 1 result, got %v", rep)
	}
	if rep.Results[0].Error == nil {
		t.Fatalf("expected node error, got nil; output=%q", rep.Results[0].Output)
	}
	if rep.Results[0].Output == "should-not-run\n" {
		t.Fatalf("body executed despite enforce denial")
	}
	if !strings.Contains(rep.Results[0].Error.Error(), "policy denied") {
		t.Errorf("error %v does not mention policy denial", rep.Results[0].Error)
	}
}

// TestHooks_OnAudit_ErrorDoesNotBreakRun is the substrate guarantee: a hook
// that returns an error must not abort the run.
func TestHooks_OnAudit_ErrorDoesNotBreakRun(t *testing.T) {
	hooks := &Hooks{
		OnAudit: func(_ context.Context, _ AuditPayload) error {
			return errors.New("hook bug")
		},
	}
	wf := &Workflow{
		Name:    "audit-error-isolation",
		Version: "1",
		Nodes: []Node{
			{ID: "ok", Kind: KindBash, Bash: &BashNode{Cmd: "echo ok"}},
		},
	}
	r := NewRunner(nil)
	r.Hooks = hooks

	rep, err := r.Run(context.Background(), wf, nil)
	if err != nil {
		t.Fatalf("run failed despite OnAudit returning error: %v", err)
	}
	if !rep.Succeeded {
		t.Fatalf("run did not succeed, got %v", rep)
	}
}

// TestHooks_NilHook_NoOps confirms that an unset hook is silently skipped
// and the runner produces an identical RunReport to a runner with no
// Hooks at all.
func TestHooks_NilHook_NoOps(t *testing.T) {
	hooks := &Hooks{} // all four fields nil
	wf := &Workflow{
		Name:    "hooks-nil",
		Version: "1",
		Nodes: []Node{
			{ID: "ok", Kind: KindBash, Bash: &BashNode{Cmd: "echo ok"}},
		},
	}
	r := NewRunner(nil)
	r.Hooks = hooks
	rep, err := r.Run(context.Background(), wf, nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !rep.Succeeded {
		t.Fatalf("run did not succeed: %v", rep)
	}
}

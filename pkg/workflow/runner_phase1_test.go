package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/workflow/roles"
)

// resolverAI is a minimal AI executor used by the role-resolution test.
// It records the model field the runner passed in via the (already
// resolved) AINode.
type resolverAI struct {
	lastModel  string
	lastEffort string
	cost       CostEstimate
}

func (r *resolverAI) Generate(_ context.Context, n *AINode, _ map[string]string, _ map[string]string) (string, error) {
	r.lastModel = n.Model
	r.lastEffort = n.Effort
	return "ok", nil
}

func (r *resolverAI) LastCost() CostEstimate { return r.cost }

// TestRunnerResolvesRoleBeforeAICall ensures the runner consults the
// role registry before calling the AI executor — the model the
// executor sees is the resolved tier, not the YAML's empty value.
func TestRunnerResolvesRoleBeforeAICall(t *testing.T) {
	reg := &roles.Registry{
		Roles: map[string]roles.Role{
			"research": {
				Primary: &roles.Tier{Model: "resolved-opus", Effort: "max"},
			},
		},
	}
	ai := &resolverAI{}
	r := NewRunner(ai)
	r.RoleResolver = &roles.Resolver{Registry: reg}

	wf := &Workflow{
		Name: "phase1",
		Nodes: []Node{{
			ID:   "n",
			Kind: KindAI,
			AI:   &AINode{Role: "research", Prompt: "go"},
		}},
	}
	rep, err := r.Run(context.Background(), wf, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Succeeded {
		t.Fatalf("expected success: %+v", rep)
	}
	if ai.lastModel != "resolved-opus" {
		t.Errorf("model = %q, want resolved-opus", ai.lastModel)
	}
	if ai.lastEffort != "max" {
		t.Errorf("effort = %q, want max", ai.lastEffort)
	}
}

func TestRunnerRoleResolverFailureFailsNode(t *testing.T) {
	r := NewRunner(&resolverAI{})
	r.RoleResolver = &roles.Resolver{Registry: &roles.Registry{Roles: map[string]roles.Role{}}}

	wf := &Workflow{
		Name: "phase1",
		Nodes: []Node{{
			ID: "n", Kind: KindAI,
			AI: &AINode{Role: "ghost", Prompt: "x"},
		}},
	}
	_, err := r.Run(context.Background(), wf, nil)
	if err == nil {
		t.Fatal("expected error from unknown role")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error = %v", err)
	}
}

// TestRunnerExitGateBlocksSuccess ensures an exit gate failing
// transitions the node to failed even when the body returned ok.
func TestRunnerExitGateBlocksSuccess(t *testing.T) {
	ai := &resolverAI{}
	r := NewRunner(ai)
	wf := &Workflow{
		Name: "phase1",
		Nodes: []Node{{
			ID: "n", Kind: KindAI,
			AI:       &AINode{Prompt: "x"},
			ExitGate: []ExitGate{{Kind: GateBash, Cmd: "exit 7"}},
		}},
	}
	_, err := r.Run(context.Background(), wf, nil)
	if !errors.Is(err, ErrExitGateFailed) {
		t.Errorf("expected ErrExitGateFailed, got %v", err)
	}
}

// TestRunnerOutputMissingFailsNode ensures a declared local_disk
// output that doesn't exist on disk fails the node.
func TestRunnerOutputMissingFailsNode(t *testing.T) {
	dir := t.TempDir()
	r := NewRunner(&resolverAI{})
	r.OutputWriter = &OutputWriter{WorkflowDir: dir}
	wf := &Workflow{
		Name: "phase1",
		Nodes: []Node{{
			ID: "n", Kind: KindAI, AI: &AINode{Prompt: "x"},
			Outputs: map[string]OutputSpec{
				"r": {Path: "missing.md", Durability: DurabilityLocalDisk},
			},
		}},
	}
	_, err := r.Run(context.Background(), wf, nil)
	if !errors.Is(err, ErrOutputMissing) {
		t.Errorf("expected ErrOutputMissing, got %v", err)
	}
}

// TestRunnerOutputPresentSucceeds checks the happy path: a node
// writes an artifact at the declared path, the writer materialises it,
// the run succeeds.
func TestRunnerOutputPresentSucceeds(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "report.md")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &fakeOutputRecorder{}
	r := NewRunner(&resolverAI{})
	r.OutputWriter = &OutputWriter{WorkflowDir: dir, Recorder: rec}
	wf := &Workflow{
		Name: "phase1",
		Nodes: []Node{{
			ID: "n", Kind: KindAI, AI: &AINode{Prompt: "x"},
			Outputs: map[string]OutputSpec{
				"r": {Path: "report.md", Durability: DurabilityLocalDisk},
			},
		}},
	}
	rep, err := r.Run(context.Background(), wf, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Succeeded {
		t.Errorf("expected success: %+v", rep)
	}
	if len(rec.records) != 1 {
		t.Errorf("recorder records = %d", len(rec.records))
	}
}

// TestRunnerBudgetCeilingFailsNode wires a MeteredAIExecutor and a
// Budget cap, then asserts that an over-budget call fails.
func TestRunnerBudgetCeilingFailsNode(t *testing.T) {
	inner := &resolverAI{cost: CostEstimate{Dollars: 10}}
	meter := NewMeter(0, 0, 0)
	wrapped := &MeteredAIExecutor{Inner: inner, Meter: meter}
	r := NewRunner(wrapped)

	wf := &Workflow{
		Name: "phase1",
		Nodes: []Node{{
			ID: "n", Kind: KindAI,
			AI:     &AINode{Prompt: "x"},
			Budget: &Budget{MaxDollars: 1.0},
		}},
	}
	_, err := r.Run(context.Background(), wf, nil)
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("expected ErrBudgetExceeded, got %v", err)
	}
}

// TestRunnerBashRespectsFSWrite ensures a bash node whose working
// directory is not on the fs_write allowlist fails before the command
// runs.
func TestRunnerBashRespectsFSWrite(t *testing.T) {
	r := NewRunner(&resolverAI{})
	wf := &Workflow{
		Name: "phase1",
		Nodes: []Node{{
			ID: "n", Kind: KindBash,
			Bash:        &BashNode{Cmd: "echo hi", WorkingDir: "/etc"},
			Permissions: &Permissions{FSWrite: []string{"/notes/**"}},
		}},
	}
	_, err := r.Run(context.Background(), wf, nil)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied, got %v", err)
	}
}

package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExitGateBashSuccess(t *testing.T) {
	g := []ExitGate{{Kind: GateBash, Cmd: "exit 0"}}
	if err := EvaluateExitGates(context.Background(), g, ExitGateContext{}); err != nil {
		t.Errorf("expected pass, got %v", err)
	}
}

func TestExitGateBashFailure(t *testing.T) {
	g := []ExitGate{{Kind: GateBash, Cmd: "exit 1"}}
	err := EvaluateExitGates(context.Background(), g, ExitGateContext{})
	if !errors.Is(err, ErrExitGateFailed) {
		t.Errorf("expected ErrExitGateFailed, got %v", err)
	}
}

func TestExitGateBashCustomSuccessExit(t *testing.T) {
	g := []ExitGate{{Kind: GateBash, Cmd: "exit 5", SuccessExit: 5}}
	if err := EvaluateExitGates(context.Background(), g, ExitGateContext{}); err != nil {
		t.Errorf("expected pass, got %v", err)
	}
}

func TestExitGateBashSeesEnvAndInputs(t *testing.T) {
	g := []ExitGate{{Kind: GateBash, Cmd: `[ "$INPUT_NAME" = "vbonnet" ] && [ "$WORKFLOW_VAR" = "abc" ]`}}
	gctx := ExitGateContext{
		Env:    map[string]string{"WORKFLOW_VAR": "abc"},
		Inputs: map[string]string{"name": "vbonnet"},
	}
	if err := EvaluateExitGates(context.Background(), g, gctx); err != nil {
		t.Errorf("expected pass, got %v", err)
	}
}

func TestExitGateOrderedShortCircuit(t *testing.T) {
	g := []ExitGate{
		{Kind: GateBash, Cmd: "exit 1"},
		{Kind: GateBash, Cmd: `echo "should not run" && touch /tmp/nope-test-marker`},
	}
	err := EvaluateExitGates(context.Background(), g, ExitGateContext{})
	if !errors.Is(err, ErrExitGateFailed) {
		t.Errorf("expected ErrExitGateFailed, got %v", err)
	}
	// Second gate must not have run.
	if _, statErr := os.Stat("/tmp/nope-test-marker"); statErr == nil {
		t.Errorf("second gate should not have executed")
		_ = os.Remove("/tmp/nope-test-marker")
	}
}

func TestExitGateRegexMatch(t *testing.T) {
	g := []ExitGate{{Kind: GateRegexMatch, Target: "outputs.path", Pattern: `\.md$`}}
	gctx := ExitGateContext{Outputs: map[string]any{"path": "notes/x.md"}}
	if err := EvaluateExitGates(context.Background(), g, gctx); err != nil {
		t.Errorf("expected pass, got %v", err)
	}
	gctx.Outputs["path"] = "notes/x.txt"
	if err := EvaluateExitGates(context.Background(), g, gctx); !errors.Is(err, ErrExitGateFailed) {
		t.Errorf("expected fail, got %v", err)
	}
}

func TestExitGateRegexBadPattern(t *testing.T) {
	g := []ExitGate{{Kind: GateRegexMatch, Target: "outputs.path", Pattern: "[invalid"}}
	err := EvaluateExitGates(context.Background(), g, ExitGateContext{Outputs: map[string]any{"path": "x"}})
	if !errors.Is(err, ErrExitGateFailed) || !strings.Contains(err.Error(), "bad pattern") {
		t.Errorf("expected bad-pattern error, got %v", err)
	}
}

func TestExitGateConfidenceScore(t *testing.T) {
	g := []ExitGate{{Kind: GateConfidenceScore, Target: "outputs.confidence", Min: 0.7}}
	gctx := ExitGateContext{Outputs: map[string]any{"confidence": 0.8}}
	if err := EvaluateExitGates(context.Background(), g, gctx); err != nil {
		t.Errorf("expected pass, got %v", err)
	}
	gctx.Outputs["confidence"] = 0.5
	err := EvaluateExitGates(context.Background(), g, gctx)
	if !errors.Is(err, ErrExitGateFailed) {
		t.Errorf("expected fail, got %v", err)
	}
}

func TestExitGateConfidenceFromString(t *testing.T) {
	g := []ExitGate{{Kind: GateConfidenceScore, Target: "outputs.score", Min: 0.5}}
	gctx := ExitGateContext{Outputs: map[string]any{"score": "0.9"}}
	if err := EvaluateExitGates(context.Background(), g, gctx); err != nil {
		t.Errorf("expected pass for stringified number, got %v", err)
	}
}

func TestExitGateJSONSchema(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(`{
		"type": "object",
		"required": ["title"],
		"properties": {
			"title": {"type": "string"},
			"score": {"type": "number", "minimum": 0, "maximum": 1}
		}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	g := []ExitGate{{Kind: GateJSONSchema, Target: "outputs.report", Schema: "schema.json"}}
	gctx := ExitGateContext{
		WorkflowDir: dir,
		Outputs: map[string]any{
			"report": map[string]any{
				"title": "ok",
				"score": 0.8,
			},
		},
	}
	if err := EvaluateExitGates(context.Background(), g, gctx); err != nil {
		t.Errorf("expected pass, got %v", err)
	}
	gctx.Outputs["report"] = map[string]any{"score": 0.8}
	if err := EvaluateExitGates(context.Background(), g, gctx); !errors.Is(err, ErrExitGateFailed) {
		t.Errorf("expected fail (missing required), got %v", err)
	}
}

func TestExitGateLookupTargetMissing(t *testing.T) {
	g := []ExitGate{{Kind: GateConfidenceScore, Target: "outputs.ghost", Min: 0.1}}
	err := EvaluateExitGates(context.Background(), g, ExitGateContext{Outputs: map[string]any{}})
	if !errors.Is(err, ErrExitGateFailed) {
		t.Errorf("expected fail, got %v", err)
	}
}

func TestExitGateUnknownKind(t *testing.T) {
	g := []ExitGate{{Kind: "ghost"}}
	err := EvaluateExitGates(context.Background(), g, ExitGateContext{})
	if !errors.Is(err, ErrExitGateFailed) {
		t.Errorf("expected fail, got %v", err)
	}
}

func TestExitGateAllPass(t *testing.T) {
	g := []ExitGate{
		{Kind: GateBash, Cmd: "true"},
		{Kind: GateRegexMatch, Target: "outputs.path", Pattern: ".+"},
		{Kind: GateConfidenceScore, Target: "outputs.score", Min: 0.5},
	}
	gctx := ExitGateContext{Outputs: map[string]any{"path": "x", "score": 0.7}}
	if err := EvaluateExitGates(context.Background(), g, gctx); err != nil {
		t.Errorf("expected all-pass, got %v", err)
	}
}

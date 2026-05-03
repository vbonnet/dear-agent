package workflow

import (
	"strings"
	"testing"
)

func TestLoadBytesMinimal(t *testing.T) {
	y := `
name: minimal
version: "1"
nodes:
  - id: n1
    kind: ai
    ai:
      prompt: hello
`
	w, err := LoadBytes([]byte(y))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if w.Name != "minimal" {
		t.Errorf("Name = %q", w.Name)
	}
	if len(w.Nodes) != 1 {
		t.Errorf("Nodes = %d", len(w.Nodes))
	}
}

func TestLoadBytesRejectsMissingKindBody(t *testing.T) {
	y := `
name: bad
version: "1"
nodes:
  - id: n1
    kind: ai
`
	_, err := LoadBytes([]byte(y))
	if err == nil {
		t.Fatal("expected validation error for missing ai body")
	}
}

func TestLoadBytesRejectsMultipleBodies(t *testing.T) {
	y := `
name: multi
version: "1"
nodes:
  - id: n1
    kind: ai
    ai: {prompt: a}
    bash: {cmd: echo}
`
	_, err := LoadBytes([]byte(y))
	if err == nil {
		t.Fatal("expected validation error for multiple bodies")
	}
}

func TestLoadBytesRejectsUnknownDepends(t *testing.T) {
	y := `
name: dep
version: "1"
nodes:
  - id: n1
    kind: ai
    depends: [ghost]
    ai: {prompt: x}
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "unknown node") {
		t.Errorf("expected unknown-node error, got %v", err)
	}
}

func TestLoadBytesDefaultsLoopMaxIters(t *testing.T) {
	y := `
name: lp
version: "1"
nodes:
  - id: lp
    kind: loop
    loop:
      until: Outputs.step == 3
      nodes:
        - id: step
          kind: ai
          ai: {prompt: go}
`
	w, err := LoadBytes([]byte(y))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if w.Nodes[0].Loop.MaxIters != 100 {
		t.Errorf("default max_iters not applied: %d", w.Nodes[0].Loop.MaxIters)
	}
}

func TestLoadBytesRejectsDuplicateIDs(t *testing.T) {
	y := `
name: dup
version: "1"
nodes:
  - id: x
    kind: ai
    ai: {prompt: a}
  - id: x
    kind: ai
    ai: {prompt: b}
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate error, got %v", err)
	}
}

func TestLoadBytesRejectsMalformed(t *testing.T) {
	if _, err := LoadBytes([]byte("name: [not-a-string")); err == nil {
		t.Error("expected YAML parse error")
	}
}

func TestLoadBytesAcceptsPhase1Fields(t *testing.T) {
	// All Phase 1 substrate fields populated. Validates that the YAML
	// schema round-trips and Validate accepts a fully-decorated node.
	y := `
name: phase1
version: "1"
nodes:
  - id: research
    kind: ai
    permissions:
      fs_read: ["~/src/engram-research/**"]
      fs_write: ["notes/**"]
      network: ["anthropic.com"]
      tools: [Read, Grep]
      egress_max_bytes: 5000000
    budget:
      max_tokens: 300000
      max_dollars: 5.00
      max_wallclock: 30m
      on_overrun: escalate
    hitl:
      block_policy: on_low_confidence
      confidence_threshold: 0.7
      approver_role: research-lead
      timeout: 24h
      on_timeout: escalate
    exit_gate:
      - kind: bash
        cmd: "echo ok"
      - kind: regex_match
        target: outputs.report.path
        pattern: "\\.md$"
      - kind: confidence_score
        target: outputs.report.frontmatter.confidence
        min: 0.7
    context_policy: selective
    context_keys: [intake.brief]
    outputs:
      report:
        path: "notes/{{ .RunID }}/report.md"
        content_type: text/markdown
        durability: git_committed
    ai:
      role: research
      prompt: do research
      required_capabilities: [long_context, citations]
`
	w, err := LoadBytes([]byte(y))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	n := w.Nodes[0]
	if n.AI.Role != "research" {
		t.Errorf("AI.Role = %q, want research", n.AI.Role)
	}
	if n.Budget == nil || n.Budget.OnOverrun != "escalate" {
		t.Errorf("Budget.OnOverrun = %v", n.Budget)
	}
	if len(n.ExitGate) != 3 {
		t.Errorf("ExitGate len = %d, want 3", len(n.ExitGate))
	}
	if n.Permissions == nil || len(n.Permissions.FSWrite) != 1 {
		t.Errorf("Permissions = %+v", n.Permissions)
	}
	if got := n.Outputs["report"].Durability; got != DurabilityGitCommitted {
		t.Errorf("Outputs[report].Durability = %q", got)
	}
	if n.ContextPolicy != "selective" || len(n.ContextKeys) != 1 {
		t.Errorf("ContextPolicy/keys = %q / %v", n.ContextPolicy, n.ContextKeys)
	}
}

func TestLoadBytesRejectsBadOnOverrun(t *testing.T) {
	y := `
name: bad-budget
version: "1"
nodes:
  - id: n
    kind: ai
    budget:
      on_overrun: explode
    ai: {prompt: x}
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "on_overrun") {
		t.Errorf("expected on_overrun error, got %v", err)
	}
}

func TestLoadBytesRejectsBadHITL(t *testing.T) {
	// on_low_confidence requires a non-zero threshold.
	y := `
name: bad-hitl
version: "1"
nodes:
  - id: n
    kind: ai
    hitl:
      block_policy: on_low_confidence
    ai: {prompt: x}
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "confidence_threshold") {
		t.Errorf("expected confidence_threshold error, got %v", err)
	}
}

func TestLoadBytesRejectsBadExitGate(t *testing.T) {
	y := `
name: bad-gate
version: "1"
nodes:
  - id: n
    kind: ai
    exit_gate:
      - kind: bash
    ai: {prompt: x}
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "cmd") {
		t.Errorf("expected gate-cmd error, got %v", err)
	}
}

func TestLoadBytesRejectsBadOutputDurability(t *testing.T) {
	y := `
name: bad-out
version: "1"
nodes:
  - id: n
    kind: ai
    outputs:
      x:
        path: "p"
        durability: forever
    ai: {prompt: x}
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "durability") {
		t.Errorf("expected durability error, got %v", err)
	}
}

func TestLoadBytesRejectsSelectiveWithoutKeys(t *testing.T) {
	y := `
name: bad-ctx
version: "1"
nodes:
  - id: n
    kind: ai
    context_policy: selective
    ai: {prompt: x}
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "context_keys") {
		t.Errorf("expected context_keys error, got %v", err)
	}
}

func TestLoadBytesRejectsCycle(t *testing.T) {
	y := `
name: cycle
version: "1"
nodes:
  - id: a
    kind: ai
    depends: [b]
    ai: {prompt: a}
  - id: b
    kind: ai
    depends: [a]
    ai: {prompt: b}
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got %v", err)
	}
}

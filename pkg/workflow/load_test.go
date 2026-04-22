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

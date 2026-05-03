package workflow

import (
	"context"
	"strings"
	"testing"
)

func TestSpawn_EmitsAndExecutesChildren(t *testing.T) {
	y := `name: spawn-basic
version: "1"
nodes:
  - id: spawner
    kind: spawn
    spawn:
      cmd: |
        cat <<'YAML'
        - id: a
          kind: bash
          bash: { cmd: 'echo first' }
        - id: b
          kind: bash
          bash: { cmd: 'echo second' }
          depends: [a]
        YAML
`
	w, err := LoadBytes([]byte(y))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	r := NewRunner(nil)
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Succeeded {
		t.Fatalf("expected success, got %+v", rep)
	}
	if len(rep.Results) != 1 {
		t.Fatalf("Results = %d, want 1 (the spawn node itself)", len(rep.Results))
	}
	if !strings.Contains(rep.Results[0].Output, "spawned 2 node(s)") {
		t.Errorf("expected summary, got %q", rep.Results[0].Output)
	}
	// Spawned outputs land under spawner/<id>.
	if got, ok := rep.Results[0].Meta["spawned"].(int); !ok || got != 2 {
		t.Errorf("Meta[spawned] = %v, want 2", rep.Results[0].Meta["spawned"])
	}
}

func TestSpawn_EmptyOutputIsLegal(t *testing.T) {
	y := `name: spawn-empty
version: "1"
nodes:
  - id: probe
    kind: spawn
    spawn:
      cmd: 'true'
`
	w, _ := LoadBytes([]byte(y))
	r := NewRunner(nil)
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Succeeded {
		t.Errorf("expected success on empty spawn, got %+v", rep)
	}
}

func TestSpawn_RejectsCycle(t *testing.T) {
	y := `name: spawn-cycle
version: "1"
nodes:
  - id: spawner
    kind: spawn
    spawn:
      cmd: |
        cat <<'YAML'
        - id: a
          kind: bash
          bash: { cmd: echo a }
          depends: [b]
        - id: b
          kind: bash
          bash: { cmd: echo b }
          depends: [a]
        YAML
`
	w, _ := LoadBytes([]byte(y))
	r := NewRunner(nil)
	rep, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Fatalf("expected cycle error, got nil; report=%+v", rep)
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got %v", err)
	}
}

func TestSpawn_HonoursMaxChildren(t *testing.T) {
	y := `name: spawn-cap
version: "1"
nodes:
  - id: spawner
    kind: spawn
    spawn:
      max_children: 1
      cmd: |
        cat <<'YAML'
        - id: a
          kind: bash
          bash: { cmd: echo a }
        - id: b
          kind: bash
          bash: { cmd: echo b }
        YAML
`
	w, _ := LoadBytes([]byte(y))
	r := NewRunner(nil)
	_, err := r.Run(context.Background(), w, nil)
	if err == nil || !strings.Contains(err.Error(), "max=1") {
		t.Fatalf("expected max_children error, got %v", err)
	}
}

func TestSpawn_AllowedKinds(t *testing.T) {
	y := `name: spawn-kinds
version: "1"
nodes:
  - id: spawner
    kind: spawn
    spawn:
      allowed_kinds: [bash]
      cmd: |
        cat <<'YAML'
        - id: rogue
          kind: ai
          ai: { prompt: "hello" }
        YAML
`
	w, _ := LoadBytes([]byte(y))
	r := NewRunner(nil)
	_, err := r.Run(context.Background(), w, nil)
	if err == nil || !strings.Contains(err.Error(), "allowed_kinds") {
		t.Fatalf("expected kind-allowlist error, got %v", err)
	}
}

func TestSpawn_RejectsBadYAML(t *testing.T) {
	y := `name: spawn-bad
version: "1"
nodes:
  - id: spawner
    kind: spawn
    spawn:
      cmd: 'echo "not a yaml list"'
`
	w, _ := LoadBytes([]byte(y))
	r := NewRunner(nil)
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestSpawn_PropagatesChildFailure(t *testing.T) {
	y := `name: spawn-fail
version: "1"
nodes:
  - id: spawner
    kind: spawn
    spawn:
      cmd: |
        cat <<'YAML'
        - id: ok
          kind: bash
          bash: { cmd: 'true' }
        - id: doomed
          kind: bash
          bash: { cmd: 'false' }
          depends: [ok]
        YAML
`
	w, _ := LoadBytes([]byte(y))
	r := NewRunner(nil)
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Fatal("expected child failure to propagate")
	}
	if !strings.Contains(err.Error(), "spawner/doomed") {
		t.Errorf("expected failed-child id in error, got %v", err)
	}
}

func TestNodeValidate_SpawnRequiresCmd(t *testing.T) {
	n := Node{ID: "s", Kind: KindSpawn, Spawn: &SpawnNode{}}
	if err := n.Validate(); err == nil || !strings.Contains(err.Error(), "spawn.cmd") {
		t.Fatalf("expected cmd-required error, got %v", err)
	}
}

func TestNodeValidate_SpawnRejectsNegativeMaxChildren(t *testing.T) {
	n := Node{ID: "s", Kind: KindSpawn, Spawn: &SpawnNode{Cmd: "x", MaxChildren: -1}}
	if err := n.Validate(); err == nil {
		t.Fatal("expected negative max_children to fail")
	}
}

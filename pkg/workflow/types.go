// Package workflow implements an Archon-style YAML DAG engine for declarative
// orchestration. Workflows are directed acyclic graphs of typed Nodes (ai,
// bash, gate, loop) that the Runner executes in topological order while
// honoring dependencies, loop-until conditions, and gate signals.
//
// The engine is intentionally small: it does NOT provide its own scheduler,
// retry policies, or distributed coordination. Those concerns belong to the
// caller (a cron, a supervisor session, CI). The engine's single
// responsibility is "given this YAML DAG and these inputs, execute the
// nodes in the right order and report outputs."
//
// Why yet another workflow engine: the research pipeline (Layer 2) and the
// supervisor-deadlock decision tree (Layer 3) both encode branching +
// loop-until logic imperatively in Go. Shifting them to YAML lets operators
// tune the shape without rebuilding binaries, makes the decision tree
// auditable, and opens the door to generating workflows programmatically.
// All three layers share the same engine so evolution happens in one place.
package workflow

import (
	"context"
	"fmt"
	"time"
)

// NodeKind is the discriminator that tells the runner how to execute a node.
// Exactly one of the kind-specific config blocks must be present on a Node
// matching its Kind; see Node.Validate().
type NodeKind string

const (
	// KindAI invokes an LLM with a prompt template. Output is the raw
	// response text; the caller can pipe it into downstream nodes via
	// {{ .Outputs.<node-id> }} templating.
	KindAI NodeKind = "ai"

	// KindBash runs a shell command and captures stdout as the node's
	// output. Non-zero exit is a failure unless the node has
	// allow_nonzero_exit: true set.
	KindBash NodeKind = "bash"

	// KindGate pauses until a named signal arrives. Signals are produced
	// either by an earlier node's completion (auto) or by an external
	// caller via Runner.Signal(name). Used to express human-in-the-loop
	// approvals, CI dependencies, or out-of-band events.
	KindGate NodeKind = "gate"

	// KindLoop executes a child DAG repeatedly until a Condition evaluates
	// true. A loop node carries its own nested Nodes which are all
	// executed each iteration.
	KindLoop NodeKind = "loop"
)

// Workflow is the top-level YAML shape. A workflow has a unique name, a
// version, a list of inputs the caller must supply, and a DAG of Nodes.
// Nodes reference each other by ID for dependency edges; the engine
// topologically sorts them at runtime.
type Workflow struct {
	Name        string            `yaml:"name"`
	Version     string            `yaml:"version"`
	Description string            `yaml:"description,omitempty"`
	Inputs      []InputSpec       `yaml:"inputs,omitempty"`
	Nodes       []Node            `yaml:"nodes"`
	Env         map[string]string `yaml:"env,omitempty"`
}

// InputSpec declares a required or optional input to the workflow. Inputs
// are referenced from node bodies via {{ .Inputs.<name> }}.
type InputSpec struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
	Default     string `yaml:"default,omitempty"`
}

// Node is one unit of work in the workflow DAG. Exactly one of AI, Bash,
// Gate, or Loop must be set, matching Kind.
type Node struct {
	ID      string   `yaml:"id"`
	Kind    NodeKind `yaml:"kind"`
	Name    string   `yaml:"name,omitempty"`
	Depends []string `yaml:"depends,omitempty"`

	// Kind-specific bodies. Only the one matching Kind is read.
	AI   *AINode   `yaml:"ai,omitempty"`
	Bash *BashNode `yaml:"bash,omitempty"`
	Gate *GateNode `yaml:"gate,omitempty"`
	Loop *LoopNode `yaml:"loop,omitempty"`

	// Timeout caps how long a single node may run. Zero means "no limit
	// beyond the context passed to Runner.Run". Expressed in Go duration
	// format (e.g. "30s", "5m").
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

// AINode configures an LLM call. Model is the canonical model id (matching
// pkg/costtrack's table); empty falls back to the runner's default.
type AINode struct {
	Model     string `yaml:"model,omitempty"`
	System    string `yaml:"system,omitempty"`
	Prompt    string `yaml:"prompt"`
	MaxTokens int    `yaml:"max_tokens,omitempty"`
	// Effort is passed to the LLM as a reasoning budget hint. Empty
	// means "inherit from runner default". Matches skill-tier strings:
	// low/medium/high/xhigh/max.
	Effort string `yaml:"effort,omitempty"`
}

// BashNode configures a shell command. Cmd is interpreted by /bin/sh -c.
// Env entries are appended to os.Environ() at runtime.
type BashNode struct {
	Cmd               string            `yaml:"cmd"`
	Env               map[string]string `yaml:"env,omitempty"`
	AllowNonzeroExit  bool              `yaml:"allow_nonzero_exit,omitempty"`
	// WorkingDir overrides the runner's default cwd for this node only.
	WorkingDir string `yaml:"working_dir,omitempty"`
}

// GateNode pauses execution until Runner.Signal(Name) is called. Useful
// for human approval gates (Discord DM, iMessage, etc.). Timeout applies
// if set; an unset timeout means "wait forever".
type GateNode struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

// LoopNode repeats a nested DAG until Until evaluates true. Until is a
// simple boolean expression over output variables — the engine supports
// equality, existence, and greater-than on scalar fields. See
// evalCondition for the grammar.
type LoopNode struct {
	Until    string `yaml:"until"`
	MaxIters int    `yaml:"max_iters,omitempty"` // 0 = unlimited, default 100
	Nodes    []Node `yaml:"nodes"`
}

// Validate checks a Workflow's shape at load time. Ensures:
//   - unique node IDs
//   - depends references exist
//   - no cycles
//   - kind-specific body present and matches Kind
//   - ai.prompt non-empty; bash.cmd non-empty; gate.name non-empty;
//     loop.until non-empty and loop.nodes non-empty.
//
// Does not validate templates — those are checked at run time when
// rendered against the actual inputs and upstream outputs.
func (w *Workflow) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("workflow: name is required")
	}
	if len(w.Nodes) == 0 {
		return fmt.Errorf("workflow %q: no nodes", w.Name)
	}
	seen := make(map[string]struct{}, len(w.Nodes))
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if n.ID == "" {
			return fmt.Errorf("workflow %q: node[%d] missing id", w.Name, i)
		}
		if _, dup := seen[n.ID]; dup {
			return fmt.Errorf("workflow %q: duplicate node id %q", w.Name, n.ID)
		}
		seen[n.ID] = struct{}{}
		if err := n.Validate(); err != nil {
			return fmt.Errorf("workflow %q: node %q: %w", w.Name, n.ID, err)
		}
	}
	// Referential integrity + cycle check.
	for i := range w.Nodes {
		n := &w.Nodes[i]
		for _, dep := range n.Depends {
			if _, ok := seen[dep]; !ok {
				return fmt.Errorf("workflow %q: node %q depends on unknown node %q", w.Name, n.ID, dep)
			}
		}
	}
	if err := detectCycle(w.Nodes); err != nil {
		return fmt.Errorf("workflow %q: %w", w.Name, err)
	}
	return nil
}

// Validate checks a Node's kind-body consistency.
//
//nolint:gocyclo // per-kind validation is clearer inline than split across helpers
func (n *Node) Validate() error {
	bodies := 0
	if n.AI != nil {
		bodies++
	}
	if n.Bash != nil {
		bodies++
	}
	if n.Gate != nil {
		bodies++
	}
	if n.Loop != nil {
		bodies++
	}
	if bodies != 1 {
		return fmt.Errorf("exactly one of ai/bash/gate/loop must be set, got %d", bodies)
	}
	switch n.Kind {
	case KindAI:
		if n.AI == nil {
			return fmt.Errorf("kind=ai but ai body missing")
		}
		if n.AI.Prompt == "" {
			return fmt.Errorf("ai.prompt is required")
		}
	case KindBash:
		if n.Bash == nil {
			return fmt.Errorf("kind=bash but bash body missing")
		}
		if n.Bash.Cmd == "" {
			return fmt.Errorf("bash.cmd is required")
		}
	case KindGate:
		if n.Gate == nil {
			return fmt.Errorf("kind=gate but gate body missing")
		}
		if n.Gate.Name == "" {
			return fmt.Errorf("gate.name is required")
		}
	case KindLoop:
		if n.Loop == nil {
			return fmt.Errorf("kind=loop but loop body missing")
		}
		if n.Loop.Until == "" {
			return fmt.Errorf("loop.until is required")
		}
		if len(n.Loop.Nodes) == 0 {
			return fmt.Errorf("loop must contain at least one node")
		}
		// Recurse into nested nodes.
		nestedIDs := make(map[string]struct{})
		for i := range n.Loop.Nodes {
			child := &n.Loop.Nodes[i]
			if child.ID == "" {
				return fmt.Errorf("loop child[%d] missing id", i)
			}
			if _, dup := nestedIDs[child.ID]; dup {
				return fmt.Errorf("loop duplicate child id %q", child.ID)
			}
			nestedIDs[child.ID] = struct{}{}
			if err := child.Validate(); err != nil {
				return fmt.Errorf("loop child %q: %w", child.ID, err)
			}
		}
	default:
		return fmt.Errorf("unknown kind %q", n.Kind)
	}
	return nil
}

// detectCycle runs a DFS over the dependency edges. Returns a descriptive
// error naming one node in the offending cycle.
func detectCycle(nodes []Node) error {
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make(map[string]int, len(nodes))
	byID := make(map[string]*Node, len(nodes))
	for i := range nodes {
		state[nodes[i].ID] = unvisited
		byID[nodes[i].ID] = &nodes[i]
	}
	var visit func(id string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("cycle detected at node %q", id)
		case done:
			return nil
		}
		state[id] = visiting
		for _, dep := range byID[id].Depends {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = done
		return nil
	}
	for id := range state {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// Result captures the outcome of a single node's execution. Output is the
// node's primary output string (LLM response text, stdout, or empty for
// gates). Meta holds kind-specific metadata (tokens for ai, exit code for
// bash, etc.).
type Result struct {
	NodeID   string
	Output   string
	Meta     map[string]any
	Error    error
	Started  time.Time
	Finished time.Time
}

// RunReport is the full execution record — one Result per node that was
// attempted. Does not include skipped nodes (those that didn't run
// because an upstream dependency failed). For loops, the latest
// iteration's Results appear; earlier iterations are summarized in Meta.
type RunReport struct {
	Workflow string
	Inputs   map[string]string
	Results  []Result
	Started  time.Time
	Finished time.Time
	// Succeeded is true iff every attempted node returned without an
	// error AND at least one node was attempted.
	Succeeded bool
}

// Context threads through node execution. The runner fills these in from
// the workflow definition + caller-supplied inputs before invoking each
// node's executor.
type nodeContext struct {
	ctx     context.Context
	inputs  map[string]string
	outputs map[string]string // node id → primary output
	env     map[string]string
}

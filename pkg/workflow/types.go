// Package workflow implements an Archon-style YAML DAG engine for declarative
// orchestration. Workflows are directed acyclic graphs of typed Nodes (ai,
// bash, gate, loop) that the Runner executes in topological order while
// honoring dependencies, loop-until conditions, and gate signals.
//
// The engine provides three optional extension points:
//   - Node-level retry policies (RetryPolicy on Node)
//   - Parallel-per-iteration loop mode (Parallel: true on LoopNode)
//   - Crash-recovery persistence (State interface + FileState implementation)
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

	// Retry configures automatic retry on failure for this node. Nil or
	// zero-value means no retry (equivalent to MaxAttempts=1). Gate nodes
	// ignore this field — a gate waiting on a signal keeps waiting rather
	// than re-entering.
	Retry *RetryPolicy `yaml:"retry,omitempty"`

	// Timeout caps how long a single node may run. Zero means "no limit
	// beyond the context passed to Runner.Run". Expressed in Go duration
	// format (e.g. "30s", "5m").
	Timeout time.Duration `yaml:"timeout,omitempty"`

	// When, if non-empty, gates node execution on a condition evaluated
	// at run time using the same grammar as loop.until:
	//   Outputs.<id> == <literal>
	//   Outputs.<id> != <literal>
	//   Outputs.<id> >  <literal>
	//   Outputs.<id>              (truthy: non-empty, non-"false", non-"0")
	// If the condition evaluates false, the node is skipped — its output
	// is empty, its Error is nil, and Result.Meta["skipped"] is true.
	//
	// Lets workflows express switch/branch shape without a dedicated
	// node type: sibling nodes with complementary When values form a
	// single branch point. Evaluated after dependencies complete, so
	// When can reference upstream outputs.
	When string `yaml:"when,omitempty"`
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

// LoopNode repeats a nested DAG. In sequential mode (Parallel: false, the
// default) it runs until Until evaluates true, bounded by MaxIters. In
// parallel mode (Parallel: true) it runs exactly MaxIters iterations
// concurrently (Concurrency limits the degree of parallelism) and returns
// when all complete; Until must be empty in parallel mode.
//
// Each iteration in parallel mode exposes {{ .Iter }} (0-based index) in
// template scope for child nodes.
//
// NOTE: parallel mode and Until are mutually exclusive — Validate rejects
// the combination.
type LoopNode struct {
	Until       string `yaml:"until,omitempty"`
	MaxIters    int    `yaml:"max_iters,omitempty"` // 0 = unlimited (seq) or 0-iter (parallel)
	Nodes       []Node `yaml:"nodes"`
	Parallel    bool   `yaml:"parallel,omitempty"`
	Concurrency int    `yaml:"concurrency,omitempty"` // 0 = runtime.NumCPU()
}

// RetryPolicy controls automatic retries for a node on failure.
//
// MaxAttempts is the total number of executions (not extra retries), so
// MaxAttempts=3 means one original attempt plus two retries. Zero or 1
// means no retry.
//
// Backoff is the initial sleep between attempts; it doubles each attempt
// (exponential backoff). Zero uses 1 s default. MaxBackoff caps the
// sleep; zero uses 30 s default.
//
// OnlyKinds restricts which node kinds this policy applies to. Empty
// means all kinds. Gate nodes always ignore retry regardless of this field.
type RetryPolicy struct {
	MaxAttempts int           `yaml:"max_attempts,omitempty"` // total attempts (1 = no retry); 0 treats as 1
	Backoff     time.Duration `yaml:"backoff,omitempty"`      // initial backoff; doubles each attempt; 0 = 1s default
	MaxBackoff  time.Duration `yaml:"max_backoff,omitempty"`  // cap; 0 = 30s default
	OnlyKinds   []NodeKind    `yaml:"only_kinds,omitempty"`   // empty = all kinds; subset opts in
}

// State is an optional persistence hook for crash-recovery. The Runner
// calls Save after each successful node and skips Completed nodes when
// Resume is used instead of Run.
//
// Only idempotent DAGs are safely resumable. AI and bash nodes can
// produce different outputs on re-run; the saved output is trusted as-is
// across resume.
type State interface {
	Save(ctx context.Context, snap Snapshot) error
	Load(ctx context.Context) (*Snapshot, error)
}

// Snapshot is the serialisable checkpoint written by State.Save.
type Snapshot struct {
	Workflow  string            `json:"workflow"`
	Inputs    map[string]string `json:"inputs"`
	Outputs   map[string]string `json:"outputs"`   // completed node outputs
	Completed map[string]bool   `json:"completed"` // node id → done
	Started   time.Time         `json:"started"`
	UpdatedAt time.Time         `json:"updated_at"`
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
		if n.Loop.Parallel {
			// Parallel mode: Until must be empty (mutually exclusive).
			if n.Loop.Until != "" {
				return fmt.Errorf("loop.parallel and loop.until are mutually exclusive")
			}
		} else {
			// Sequential mode: Until is required.
			if n.Loop.Until == "" {
				return fmt.Errorf("loop.until is required (or set parallel: true)")
			}
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

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

	// KindSpawn lets a node emit zero or more new sibling nodes that
	// the runner appends to the DAG and executes after the spawn node
	// itself completes. The spawn body is a bash command whose stdout
	// is parsed as a YAML list of Node values; each spawned node
	// inherits a depends edge on the spawn parent so the new work
	// runs strictly after the spawn point. Cycle detection runs over
	// the augmented DAG before any spawned node executes — a spawn
	// that produces a cycle fails the parent. See runner.go's
	// executeSpawn for the full lifecycle. Phase 5.4.
	KindSpawn NodeKind = "spawn"
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
	AI    *AINode    `yaml:"ai,omitempty"`
	Bash  *BashNode  `yaml:"bash,omitempty"`
	Gate  *GateNode  `yaml:"gate,omitempty"`
	Loop  *LoopNode  `yaml:"loop,omitempty"`
	Spawn *SpawnNode `yaml:"spawn,omitempty"`

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

	// --- Phase 1 substrate fields. All optional and additive; existing
	// workflows continue to validate without populating them. ---

	// Permissions declares an allowlist for filesystem, network, and tool
	// access for this node. Nil means "no constraints declared" — the
	// runner falls back to the harness's defaults. See pkg/workflow's
	// permissions.go for enforcement semantics.
	Permissions *Permissions `yaml:"permissions,omitempty"`

	// Budget caps token/dollar/wallclock spend on this node. Nil means
	// "no per-node ceiling"; the run-level ceiling (if any) still applies.
	// See pkg/workflow's budget.go.
	Budget *Budget `yaml:"budget,omitempty"`

	// ExitGate is an ordered list of post-condition checks that must all
	// pass before the node is allowed to transition succeeded. Empty
	// means "no gates" (current default behaviour). Gates short-circuit:
	// the first failing gate fails the node.
	ExitGate []ExitGate `yaml:"exit_gate,omitempty"`

	// HITL declares human-in-the-loop policy for this node. Nil means
	// "never block on a human" (current default). Wired in Phase 2; the
	// schema lands in Phase 1 so YAML written today validates tomorrow.
	HITL *HITLPolicy `yaml:"hitl,omitempty"`

	// ContextPolicy hints at how prior outputs should be passed to the
	// node. One of "fresh", "inherit", "summarized", "selective". Empty
	// means "inherit" (the existing behaviour). Schema-only in Phase 1.
	ContextPolicy string `yaml:"context_policy,omitempty"`

	// ContextKeys is the explicit list of upstream outputs (by id) the
	// node depends on when ContextPolicy=selective. Validation only
	// enforces shape; the resolver lands in Phase 2.
	ContextKeys []string `yaml:"context_keys,omitempty"`

	// Outputs declares structured artifacts the node is expected to
	// produce. Each entry is keyed by an output name (e.g. "report") and
	// describes the path, content type, schema, and durability tier.
	// Phase 1 ships parse + validate; Phase 1.6 wires the writer.
	Outputs map[string]OutputSpec `yaml:"outputs,omitempty"`
}

// AINode configures an LLM call.
//
// Role is the canonical Phase 1 way to declare "what kind of model this
// node needs" — the role registry resolves it to a primary/secondary/
// tertiary model tier at run time. Migrating Opus 4.7 → Opus 5.0 is a
// one-line edit to roles.yaml.
//
// Model is the legacy back-compat field that hard-codes a model id
// (matching pkg/costtrack's table). When both are set the resolver
// prefers Role; when only Model is set the runner emits a deprecation
// warning and uses it directly.
//
// ModelOverride is the per-node escape hatch — when set it short-circuits
// the role registry and forces a specific model. Useful for one-off
// experiments and CLI overrides; emits a deprecation warning so it shows
// up in workflow lint.
type AINode struct {
	Role          string `yaml:"role,omitempty"`
	Model         string `yaml:"model,omitempty"`
	ModelOverride string `yaml:"model_override,omitempty"`
	System        string `yaml:"system,omitempty"`
	Prompt        string `yaml:"prompt"`
	MaxTokens     int    `yaml:"max_tokens,omitempty"`
	// Effort is passed to the LLM as a reasoning budget hint. Empty
	// means "inherit from runner default". Matches skill-tier strings:
	// low/medium/high/xhigh/max.
	Effort string `yaml:"effort,omitempty"`
	// RequiredCapabilities filters tier resolution: only model tiers
	// whose capabilities superset this list are eligible. Empty means
	// "any tier". Capabilities are free-form strings (long_context,
	// citations, web_search, structured_output, tool_use, ...).
	RequiredCapabilities []string `yaml:"required_capabilities,omitempty"`
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

// SpawnNode emits new sibling nodes at run-time. Cmd is interpreted
// by /bin/sh -c (matching BashNode); its stdout must be a YAML list
// of Node values, e.g.
//
//	- id: child-1
//	  kind: bash
//	  bash: { cmd: "echo first child" }
//	- id: child-2
//	  kind: bash
//	  bash: { cmd: "echo second child" }
//
// Each spawned node inherits a depends edge on the spawn node's id
// (added automatically by the runner if absent), so the new work
// always runs strictly after the spawn point. Spawned nodes go
// through Validate before the runner accepts them; a malformed
// spawn output fails the parent.
//
// MaxChildren caps how many nodes a single spawn may emit — a guard
// against runaway DAG growth. Zero means "use the default cap" (32);
// negative means "no cap" and is rejected at validate.
//
// AllowedKinds, when set, restricts what the spawned children may
// be. Empty means "any kind"; a typical safe configuration is
// AllowedKinds: [bash] so untrusted spawn output cannot smuggle in
// AI calls or gates.
type SpawnNode struct {
	Cmd          string            `yaml:"cmd"`
	Env          map[string]string `yaml:"env,omitempty"`
	WorkingDir   string            `yaml:"working_dir,omitempty"`
	MaxChildren  int               `yaml:"max_children,omitempty"`
	AllowedKinds []NodeKind        `yaml:"allowed_kinds,omitempty"`
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
//
// RunID was added in Phase 0 of the workflow-engine roadmap. SQLiteState
// uses it to address the right row in `runs`; legacy FileState snapshots
// written before Phase 0 leave it empty (omitempty), and the runner then
// generates a fresh id — preserving exact resume semantics for callers
// who never adopted the substrate path.
type Snapshot struct {
	Workflow  string            `json:"workflow"`
	RunID     string            `json:"run_id,omitempty"`
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
	if n.Spawn != nil {
		bodies++
	}
	if bodies != 1 {
		return fmt.Errorf("exactly one of ai/bash/gate/loop/spawn must be set, got %d", bodies)
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
	case KindSpawn:
		if n.Spawn == nil {
			return fmt.Errorf("kind=spawn but spawn body missing")
		}
		if n.Spawn.Cmd == "" {
			return fmt.Errorf("spawn.cmd is required")
		}
		if n.Spawn.MaxChildren < 0 {
			return fmt.Errorf("spawn.max_children must be >= 0 (got %d; use 0 for default)", n.Spawn.MaxChildren)
		}
	default:
		return fmt.Errorf("unknown kind %q", n.Kind)
	}
	if err := n.validatePhase1(); err != nil {
		return err
	}
	return nil
}

// validatePhase1 checks the additive Phase 1 fields (budget, exit_gate,
// hitl, context_policy, outputs[]). These are all optional — the
// validator only fires when the field is populated.
//
//nolint:gocyclo // straight-line shape checks; splitting hurts locality
func (n *Node) validatePhase1() error {
	if b := n.Budget; b != nil {
		switch b.OnOverrun {
		case "", "fail", "escalate", "truncate":
		default:
			return fmt.Errorf("budget.on_overrun must be one of fail|escalate|truncate, got %q", b.OnOverrun)
		}
		if b.MaxTokens < 0 || b.MaxDollars < 0 || b.MaxWallclock < 0 {
			return fmt.Errorf("budget caps must be non-negative")
		}
	}
	for i, gate := range n.ExitGate {
		if err := gate.Validate(); err != nil {
			return fmt.Errorf("exit_gate[%d]: %w", i, err)
		}
	}
	if h := n.HITL; h != nil {
		switch h.BlockPolicy {
		case "", "never", "always", "on_low_confidence":
		default:
			return fmt.Errorf("hitl.block_policy must be one of never|always|on_low_confidence, got %q", h.BlockPolicy)
		}
		if h.BlockPolicy == "on_low_confidence" && h.ConfidenceThreshold <= 0 {
			return fmt.Errorf("hitl.confidence_threshold must be > 0 when block_policy=on_low_confidence")
		}
		switch h.OnTimeout {
		case "", "escalate", "reject", "approve":
		default:
			return fmt.Errorf("hitl.on_timeout must be one of escalate|reject|approve, got %q", h.OnTimeout)
		}
	}
	switch n.ContextPolicy {
	case "", "fresh", "inherit", "summarized", "selective":
	default:
		return fmt.Errorf("context_policy must be one of fresh|inherit|summarized|selective, got %q", n.ContextPolicy)
	}
	if n.ContextPolicy == "selective" && len(n.ContextKeys) == 0 {
		return fmt.Errorf("context_policy=selective requires context_keys")
	}
	for key, spec := range n.Outputs {
		if key == "" {
			return fmt.Errorf("outputs has empty key")
		}
		if spec.Path == "" {
			return fmt.Errorf("outputs[%q].path is required", key)
		}
		switch spec.Durability {
		case "", DurabilityEphemeral, DurabilityLocalDisk, DurabilityGitCommitted, DurabilityEngramIndexed:
		default:
			return fmt.Errorf("outputs[%q].durability must be one of ephemeral|local_disk|git_committed|engram_indexed, got %q", key, spec.Durability)
		}
	}
	return nil
}

// Validate enforces the kind-specific shape of an exit gate. Each kind
// has its own required fields (Cmd / Pattern / Schema / Min); a gate
// with the wrong shape fails Validate so the YAML author sees the typo
// rather than a runtime "unknown field" error.
func (g *ExitGate) Validate() error {
	switch g.Kind {
	case GateBash, GateTestCmd:
		if g.Cmd == "" {
			return fmt.Errorf("kind=%s requires cmd", g.Kind)
		}
	case GateRegexMatch:
		if g.Target == "" || g.Pattern == "" {
			return fmt.Errorf("kind=regex_match requires target and pattern")
		}
	case GateJSONSchema:
		if g.Target == "" || g.Schema == "" {
			return fmt.Errorf("kind=json_schema requires target and schema")
		}
	case GateConfidenceScore:
		if g.Target == "" {
			return fmt.Errorf("kind=confidence_score requires target")
		}
	default:
		return fmt.Errorf("unknown exit_gate kind %q", g.Kind)
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
// node's executor. runID is set when the runner has generated an id for
// the current invocation; recorder/audit calls use it to attribute rows
// to the right run.
type nodeContext struct {
	ctx     context.Context
	inputs  map[string]string
	outputs map[string]string // node id → primary output
	env     map[string]string
	runID   string
}

// Permissions declares the bounded-permissions surface for a node. Each
// allowlist is a slice of glob patterns (filesystem) or hostnames
// (network) or canonical tool names (tools). Nil slices mean "no
// constraint declared at this level"; the harness's defaults apply.
//
// EgressMaxBytes caps total bytes the node may write to the network
// (uploads, API request bodies). Zero means "no cap".
//
// DynamicPaths is the list of input fields whose values are interpreted
// as filesystem paths and merged into the fs_read/fs_write allowlists at
// run time — lets workflows accept "target_dir" as an input without
// hard-coding it in the YAML.
type Permissions struct {
	FSRead         []string `yaml:"fs_read,omitempty"`
	FSWrite        []string `yaml:"fs_write,omitempty"`
	Network        []string `yaml:"network,omitempty"`
	Tools          []string `yaml:"tools,omitempty"`
	EgressMaxBytes int64    `yaml:"egress_max_bytes,omitempty"`
	DynamicPaths   []string `yaml:"dynamic_paths,omitempty"`
}

// Budget caps a node's spend on tokens, dollars, and wallclock. Zero
// values mean "no ceiling at this dimension". OnOverrun selects the
// runner's behaviour when a ceiling is hit:
//
//   - "fail" (default): the node transitions to failed with a
//     budget-exceeded error.
//   - "escalate": the node transitions to awaiting_hitl so a human can
//     approve continuation. Phase 2 wires this into the HITL state; in
//     Phase 1 it is treated as "fail" with a warning logged.
//   - "truncate": the node short-circuits (returns whatever output it
//     has produced so far) and transitions to succeeded. Useful for
//     long-context AI nodes where partial output is preferable to none.
//
// Rationale for embedding cost as a permission rather than a separate
// concept: a node that can spend $50 of LLM time has more authority
// than one capped at $0.50, exactly the way a fs_write permission
// grants more authority than fs_read.
type Budget struct {
	MaxTokens    int           `yaml:"max_tokens,omitempty"`
	MaxDollars   float64       `yaml:"max_dollars,omitempty"`
	MaxWallclock time.Duration `yaml:"max_wallclock,omitempty"`
	OnOverrun    string        `yaml:"on_overrun,omitempty"`
}

// ExitGateKind enumerates the deterministic checks an exit gate can run
// against a node's outputs. The set is deliberately small — five kinds
// in v1 covers every concrete request the research surfaced; adding a
// kind needs an ADR-010 amendment.
type ExitGateKind string

const (
	// GateBash runs an external shell command. Success is exit code 0
	// (or matching SuccessExit). The cmd field is a Go template rendered
	// against {{ .Inputs }}, {{ .Outputs }}, {{ .RunID }}.
	GateBash ExitGateKind = "bash"

	// GateRegexMatch checks whether Target (a path expression rooted at
	// the node's outputs) matches Pattern. The match is case-sensitive
	// unless the pattern uses (?i).
	GateRegexMatch ExitGateKind = "regex_match"

	// GateJSONSchema validates a JSON output against a JSON Schema file
	// (Schema). Phase 1 ships a minimal validator (type, required, enum)
	// — the full JSON-Schema 2020-12 surface is post-MVS.
	GateJSONSchema ExitGateKind = "json_schema"

	// GateTestCmd runs a `go test` / `pytest` / equivalent command and
	// requires it to pass. Distinguished from GateBash so audit logs can
	// distinguish "the gate ran my test suite" from "the gate ran a
	// shell check".
	GateTestCmd ExitGateKind = "test_cmd"

	// GateConfidenceScore reads a numeric value from Target (e.g.
	// outputs.report.frontmatter.confidence) and requires it to be ≥
	// Min. Used by AI nodes that self-report a confidence score in
	// their response frontmatter.
	GateConfidenceScore ExitGateKind = "confidence_score"
)

// ExitGate is one entry in a node's exit_gate list. Exactly one of the
// kind-specific fields below must be populated (matching Kind). Gates
// run in declared order; the first failure short-circuits and fails
// the node.
type ExitGate struct {
	Kind ExitGateKind `yaml:"kind"`

	// Cmd is the shell command for kind=bash and kind=test_cmd.
	Cmd string `yaml:"cmd,omitempty"`

	// SuccessExit is the exit code GateBash treats as success. Zero
	// (the default) means "exit 0 = pass". Non-zero values are useful
	// for external tools that report status via specific exit codes.
	SuccessExit int `yaml:"success_exit,omitempty"`

	// Target is a dotted path expression rooted at the node's outputs
	// or inputs. Used by regex_match, json_schema, and confidence_score.
	// Examples: "outputs.report.path", "outputs.report.frontmatter.confidence".
	Target string `yaml:"target,omitempty"`

	// Pattern is the regex for kind=regex_match. Compiled once per gate
	// invocation; a malformed pattern fails the gate immediately rather
	// than the node it gates (so the YAML author sees a clear error).
	Pattern string `yaml:"pattern,omitempty"`

	// Schema is the path to a JSON Schema file for kind=json_schema.
	// Resolved relative to the workflow file's directory.
	Schema string `yaml:"schema,omitempty"`

	// Min is the inclusive lower bound for kind=confidence_score.
	Min float64 `yaml:"min,omitempty"`
}

// HITLPolicy declares human-in-the-loop policy for a node. Schema-only in
// Phase 1 — the runner threads the values through to the audit log so
// downstream tooling can see the intent, but does not actually block.
// Phase 2 wires this into the awaiting_hitl state.
type HITLPolicy struct {
	// BlockPolicy is one of "never", "always", "on_low_confidence". The
	// "on_low_confidence" mode requires a confidence_score gate or a
	// confidence value in the node's outputs.
	BlockPolicy string `yaml:"block_policy,omitempty"`

	// ConfidenceThreshold is the trigger value for
	// block_policy=on_low_confidence. A confidence below this threshold
	// transitions the node to awaiting_hitl. Zero with the
	// on_low_confidence policy is rejected by Validate.
	ConfidenceThreshold float64 `yaml:"confidence_threshold,omitempty"`

	// ApproverRole is the required role on the approver. Empty means
	// "any human".
	ApproverRole string `yaml:"approver_role,omitempty"`

	// Timeout caps how long the node may sit in awaiting_hitl. Zero
	// means "wait forever" (subject to the run-level context).
	Timeout time.Duration `yaml:"timeout,omitempty"`

	// OnTimeout selects the resolution when Timeout fires. One of
	// "escalate" (re-emit at the next-higher approver_role), "reject"
	// (fail the node), or "approve" (proceed as if approved). Default
	// is "reject".
	OnTimeout string `yaml:"on_timeout,omitempty"`
}

// OutputDurability is the durability tier of a declared output.
type OutputDurability string

const (
	// DurabilityEphemeral means the artifact lives only for the run's
	// lifetime — no persistence beyond what the runner already records
	// in node_outputs.
	DurabilityEphemeral OutputDurability = "ephemeral"

	// DurabilityLocalDisk persists the artifact to disk at the declared
	// path. The runner refuses to mark the node succeeded until the
	// file exists.
	DurabilityLocalDisk OutputDurability = "local_disk"

	// DurabilityGitCommitted writes the artifact to disk and commits it
	// to the workflow's working directory git repo. Phase 1 lays the
	// schema; the writer lands in Phase 1.6.
	DurabilityGitCommitted OutputDurability = "git_committed"

	// DurabilityEngramIndexed writes the artifact through pkg/source's
	// AddSource so it becomes searchable. Wiring lands in Phase 3.
	DurabilityEngramIndexed OutputDurability = "engram_indexed"
)

// OutputSpec declares one structured artifact a node is expected to
// produce. The runner resolves Path as a Go template against the run's
// inputs/outputs/env, materialises the artifact at the declared
// durability tier, and refuses to mark the node succeeded until every
// declared output exists.
type OutputSpec struct {
	Path        string           `yaml:"path"`
	ContentType string           `yaml:"content_type,omitempty"`
	Schema      string           `yaml:"schema,omitempty"`
	Durability  OutputDurability `yaml:"durability,omitempty"`
}

package workflow

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"text/template"
	"time"
)

// AIExecutor is the hook for invoking an LLM. Callers plug in their own —
// typically a thin wrapper around pkg/llm/provider or the agm-bus channel
// for Max-plan OAuth routing. Keeping this as an interface lets the
// workflow engine stay independent of any single LLM backend.
type AIExecutor interface {
	// Generate returns the LLM's response text. The runner has already
	// rendered templates in prompt/system before calling. model may be
	// empty; implementations should fall back to their default.
	Generate(ctx context.Context, node *AINode, inputs map[string]string, outputs map[string]string) (string, error)
}

// Runner executes workflows. Construct with NewRunner then call Run.
// A single Runner instance is safe for concurrent Run calls — each Run
// uses its own nodeContext and output map.
type Runner struct {
	// AI is called for every KindAI node. Required.
	AI AIExecutor
	// Logger is used for run-time logging. Defaults to slog.Default().
	Logger *slog.Logger
	// DefaultWorkingDir is the cwd for bash nodes that don't override it.
	DefaultWorkingDir string
	// DefaultBashShell is the shell used for bash nodes. Defaults to /bin/sh.
	DefaultBashShell string
	// SignalTimeout caps how long Gate nodes wait for a signal. Zero
	// means "wait forever" (or until the parent context is cancelled).
	SignalTimeout time.Duration

	mu      sync.Mutex
	signals map[string]chan struct{}
}

// NewRunner returns a Runner with defaults applied. ai must be non-nil.
func NewRunner(ai AIExecutor) *Runner {
	return &Runner{
		AI:               ai,
		Logger:           slog.Default(),
		DefaultBashShell: "/bin/sh",
		signals:          make(map[string]chan struct{}),
	}
}

// Signal wakes any Gate node currently waiting on name. Calling Signal
// for a name no gate is waiting on is safe — the signal is buffered and
// the next Gate with that name will consume it immediately.
func (r *Runner) Signal(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.signals[name]
	if !ok {
		ch = make(chan struct{}, 1)
		r.signals[name] = ch
	}
	select {
	case ch <- struct{}{}:
	default: // already signaled; Gate will consume
	}
}

// gateChannel returns (and creates on first use) the buffered channel
// for signal name. Callers hold a channel they can select on; the Runner
// keeps the same channel so later Signal calls land here.
func (r *Runner) gateChannel(name string) chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch, ok := r.signals[name]
	if !ok {
		ch = make(chan struct{}, 1)
		r.signals[name] = ch
	}
	return ch
}

// Run executes a workflow end-to-end. Returns a RunReport even on failure
// so the caller can inspect partial state. An error return is for
// unrecoverable issues (validation, unresolvable inputs, cycles).
func (r *Runner) Run(ctx context.Context, w *Workflow, inputs map[string]string) (*RunReport, error) {
	if err := w.Validate(); err != nil {
		return nil, err
	}
	merged, err := mergeInputs(w.Inputs, inputs)
	if err != nil {
		return nil, err
	}
	rep := &RunReport{
		Workflow: w.Name,
		Inputs:   merged,
		Started:  time.Now(),
	}
	nc := &nodeContext{
		ctx:     ctx,
		inputs:  merged,
		outputs: make(map[string]string, len(w.Nodes)),
		env:     w.Env,
	}
	order, err := topoOrder(w.Nodes)
	if err != nil {
		return rep, err
	}
	for _, id := range order {
		if ctx.Err() != nil {
			rep.Finished = time.Now()
			return rep, ctx.Err()
		}
		node := findNode(w.Nodes, id)
		res := r.executeNode(nc, node)
		rep.Results = append(rep.Results, res)
		if res.Error != nil {
			rep.Finished = time.Now()
			return rep, fmt.Errorf("node %q: %w", node.ID, res.Error)
		}
		nc.outputs[node.ID] = res.Output
	}
	rep.Finished = time.Now()
	rep.Succeeded = len(rep.Results) > 0
	return rep, nil
}

// executeNode dispatches to the kind-specific executor. All executors
// receive the same nodeContext so they can read inputs + upstream outputs.
func (r *Runner) executeNode(nc *nodeContext, node *Node) Result {
	res := Result{NodeID: node.ID, Started: time.Now(), Meta: make(map[string]any)}
	ctx := nc.ctx
	if node.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(nc.ctx, node.Timeout)
		defer cancel()
	}
	childNC := &nodeContext{ctx: ctx, inputs: nc.inputs, outputs: nc.outputs, env: nc.env}

	switch node.Kind {
	case KindAI:
		out, err := r.executeAI(childNC, node)
		res.Output, res.Error = out, err
	case KindBash:
		out, code, err := r.executeBash(childNC, node)
		res.Output = out
		res.Meta["exit_code"] = code
		res.Error = err
	case KindGate:
		err := r.executeGate(childNC, node)
		res.Error = err
	case KindLoop:
		iters, err := r.executeLoop(childNC, node)
		res.Meta["iterations"] = iters
		res.Error = err
	default:
		res.Error = fmt.Errorf("unknown kind %q", node.Kind)
	}
	res.Finished = time.Now()
	return res
}

// executeAI renders templates and delegates to Runner.AI.
func (r *Runner) executeAI(nc *nodeContext, node *Node) (string, error) {
	// Render prompt + system templates.
	prompt, err := renderTemplate(node.AI.Prompt, nc)
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	system, err := renderTemplate(node.AI.System, nc)
	if err != nil {
		return "", fmt.Errorf("render system: %w", err)
	}
	rendered := *node.AI
	rendered.Prompt = prompt
	rendered.System = system
	return r.AI.Generate(nc.ctx, &rendered, nc.inputs, nc.outputs)
}

// executeBash runs the rendered command under /bin/sh -c and returns its
// stdout + exit code. Stderr is captured into the Meta map.
func (r *Runner) executeBash(nc *nodeContext, node *Node) (string, int, error) {
	rendered, err := renderTemplate(node.Bash.Cmd, nc)
	if err != nil {
		return "", 0, fmt.Errorf("render cmd: %w", err)
	}
	shell := r.DefaultBashShell
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.CommandContext(nc.ctx, shell, "-c", rendered)
	cmd.Dir = node.Bash.WorkingDir
	if cmd.Dir == "" {
		cmd.Dir = r.DefaultWorkingDir
	}
	if len(node.Bash.Env) > 0 {
		cmd.Env = append(cmd.Environ(), envSlice(node.Bash.Env)...)
	}
	out, err := cmd.Output()
	code := cmd.ProcessState.ExitCode()
	if err != nil {
		// Non-zero exit is wrapped into ExitError. If the node allows it,
		// succeed with the exit code in Meta.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && node.Bash.AllowNonzeroExit {
			return string(out), code, nil
		}
		return string(out), code, fmt.Errorf("bash: %w", err)
	}
	return string(out), code, nil
}

// executeGate blocks until the signal arrives or the context/Timeout expires.
func (r *Runner) executeGate(nc *nodeContext, node *Node) error {
	ch := r.gateChannel(node.Gate.Name)
	r.Logger.Info("gate waiting", "node", node.ID, "signal", node.Gate.Name)
	var timeout <-chan time.Time
	if r.SignalTimeout > 0 {
		timer := time.NewTimer(r.SignalTimeout)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case <-ch:
		return nil
	case <-nc.ctx.Done():
		return nc.ctx.Err()
	case <-timeout:
		return fmt.Errorf("gate %q timed out after %s", node.Gate.Name, r.SignalTimeout)
	}
}

// executeLoop runs the nested nodes until Until evaluates true, bounded
// by MaxIters.
func (r *Runner) executeLoop(nc *nodeContext, node *Node) (int, error) {
	maxIters := node.Loop.MaxIters
	if maxIters <= 0 {
		maxIters = 100
	}
	for i := 0; i < maxIters; i++ {
		if nc.ctx.Err() != nil {
			return i, nc.ctx.Err()
		}
		// Execute the nested DAG once.
		order, err := topoOrder(node.Loop.Nodes)
		if err != nil {
			return i, fmt.Errorf("loop nested topo: %w", err)
		}
		for _, id := range order {
			child := findNode(node.Loop.Nodes, id)
			res := r.executeNode(nc, child)
			nc.outputs[child.ID] = res.Output
			if res.Error != nil {
				return i + 1, fmt.Errorf("iter %d node %q: %w", i, child.ID, res.Error)
			}
		}
		done, err := evalCondition(node.Loop.Until, nc)
		if err != nil {
			return i + 1, fmt.Errorf("eval until: %w", err)
		}
		if done {
			return i + 1, nil
		}
	}
	return maxIters, fmt.Errorf("loop exceeded max_iters=%d", maxIters)
}

// mergeInputs fills in defaults and rejects missing required inputs.
func mergeInputs(spec []InputSpec, given map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(spec))
	for _, s := range spec {
		if v, ok := given[s.Name]; ok {
			out[s.Name] = v
			continue
		}
		if s.Default != "" {
			out[s.Name] = s.Default
			continue
		}
		if s.Required {
			return nil, fmt.Errorf("missing required input %q", s.Name)
		}
	}
	// Also allow undeclared inputs — the caller may pass through extras.
	for k, v := range given {
		if _, already := out[k]; !already {
			out[k] = v
		}
	}
	return out, nil
}

// topoOrder returns node IDs in topological (dependency-respecting) order.
// Nodes with no dependencies come first. Stable across runs so reports
// are easy to diff.
func topoOrder(nodes []Node) ([]string, error) {
	if err := detectCycle(nodes); err != nil {
		return nil, err
	}
	remaining := make(map[string]map[string]struct{}, len(nodes))
	idOrder := make([]string, len(nodes))
	for i := range nodes {
		idOrder[i] = nodes[i].ID
		rem := make(map[string]struct{}, len(nodes[i].Depends))
		for _, d := range nodes[i].Depends {
			rem[d] = struct{}{}
		}
		remaining[nodes[i].ID] = rem
	}
	var out []string
	for len(remaining) > 0 {
		progress := false
		for _, id := range idOrder {
			rem, present := remaining[id]
			if !present {
				continue
			}
			if len(rem) == 0 {
				out = append(out, id)
				delete(remaining, id)
				for other := range remaining {
					delete(remaining[other], id)
				}
				progress = true
			}
		}
		if !progress {
			// Should not happen after cycle detection, but guard anyway.
			return out, fmt.Errorf("topo: no progress (possible hidden cycle)")
		}
	}
	return out, nil
}

func findNode(nodes []Node, id string) *Node {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}

// renderTemplate applies Go text/template substitution over t using the
// node's inputs + upstream outputs + env. Template functions are minimal
// by design — users embedding DSL logic into prompts encourages drift.
func renderTemplate(t string, nc *nodeContext) (string, error) {
	if !strings.Contains(t, "{{") {
		return t, nil // fast path: no templating needed
	}
	tpl, err := template.New("node").Option("missingkey=zero").Parse(t)
	if err != nil {
		return "", err
	}
	data := map[string]any{
		"Inputs":  nc.inputs,
		"Outputs": nc.outputs,
		"Env":     nc.env,
	}
	var sb strings.Builder
	if err := tpl.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// evalCondition is a tiny grammar for loop.until. Supported forms:
//   - "Outputs.<id> == <literal>"     (equality)
//   - "Outputs.<id> != <literal>"     (inequality)
//   - "Outputs.<id> > <literal>"      (numeric comparison)
//   - "Outputs.<id>"                   (truthy: non-empty, non-"false", non-"0")
//
// Whitespace around the operator is optional. Literals are bare words
// (no quoting) — the goal is a tiny human-readable DSL, not a full
// expression language.
func evalCondition(cond string, nc *nodeContext) (bool, error) {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return false, fmt.Errorf("empty condition")
	}
	// Try binary operators in order of length (so == doesn't match first as =).
	for _, op := range []string{"==", "!=", ">"} {
		if idx := strings.Index(cond, op); idx > 0 {
			lhs := strings.TrimSpace(cond[:idx])
			rhs := strings.TrimSpace(cond[idx+len(op):])
			lv, err := resolvePath(lhs, nc)
			if err != nil {
				return false, err
			}
			switch op {
			case "==":
				return lv == rhs, nil
			case "!=":
				return lv != rhs, nil
			case ">":
				return compareNumeric(lv, rhs, func(a, b float64) bool { return a > b })
			}
		}
	}
	// No operator → truthy test.
	v, err := resolvePath(cond, nc)
	if err != nil {
		return false, err
	}
	return v != "" && !strings.EqualFold(v, "false") && v != "0", nil
}

// resolvePath looks up a dotted path like Outputs.stage3 or Inputs.name.
func resolvePath(path string, nc *nodeContext) (string, error) {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("path %q must be <scope>.<name>", path)
	}
	switch parts[0] {
	case "Outputs":
		return nc.outputs[parts[1]], nil
	case "Inputs":
		return nc.inputs[parts[1]], nil
	case "Env":
		return nc.env[parts[1]], nil
	}
	return "", fmt.Errorf("unknown scope %q", parts[0])
}

func compareNumeric(a, b string, op func(x, y float64) bool) (bool, error) {
	var af, bf float64
	if _, err := fmt.Sscanf(a, "%f", &af); err != nil {
		return false, fmt.Errorf("not numeric: %q", a)
	}
	if _, err := fmt.Sscanf(b, "%f", &bf); err != nil {
		return false, fmt.Errorf("not numeric: %q", b)
	}
	return op(af, bf), nil
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

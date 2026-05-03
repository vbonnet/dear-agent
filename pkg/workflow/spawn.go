package workflow

import (
	"fmt"
	"os/exec"

	"gopkg.in/yaml.v3"
)

// defaultSpawnMaxChildren caps spawned-children count when the node
// declares max_children=0 (the default). 32 is high enough for the
// "fan out one task per discovered file" pattern and low enough to
// fail-loud on a runaway emitter.
const defaultSpawnMaxChildren = 32

// executeSpawn runs the spawn command, parses its YAML stdout into a
// list of Node values, validates them, and executes each in
// dependency order. The spawn node's own Output is a one-line summary
// ("spawned N children: a, b, c"); spawned nodes' outputs are stored
// under their own ids in nc.outputs so downstream sibling nodes (or
// later spawn waves) can reference them via {{ .Outputs.<child-id> }}.
//
// Why inline execution rather than appending to the parent DAG:
// inline keeps the run's topological order intact (no mid-run sort
// invalidation), keeps the audit log linear (spawned nodes get their
// own attempt rows because executeNode handles them), and avoids any
// rebalancing of pending depends edges. Cycle detection runs over
// the spawned subgraph before any child executes; a spawn that
// emits a cycle fails the parent.
//
//nolint:gocyclo // sequential validate-then-execute pipeline; splitting hurts locality
func (r *Runner) executeSpawn(nc *nodeContext, node *Node) (string, int, error) {
	cmd, err := renderTemplate(node.Spawn.Cmd, nc)
	if err != nil {
		return "", 0, fmt.Errorf("render spawn cmd: %w", err)
	}
	shell := r.DefaultBashShell
	if shell == "" {
		shell = "/bin/sh"
	}
	c := exec.CommandContext(nc.ctx, shell, "-c", cmd)
	c.Dir = node.Spawn.WorkingDir
	if c.Dir == "" {
		c.Dir = r.DefaultWorkingDir
	}
	env := c.Environ()
	for k, v := range node.Spawn.Env {
		env = append(env, k+"="+v)
	}
	for k, v := range nc.inputs {
		env = append(env, envVarKey("INPUT_", k)+"="+v)
	}
	for k, v := range nc.outputs {
		env = append(env, envVarKey("OUTPUT_", k)+"="+v)
	}
	c.Env = env

	out, err := c.Output()
	if err != nil {
		return string(out), 0, fmt.Errorf("spawn cmd: %w", err)
	}

	children, err := parseSpawnOutput(out)
	if err != nil {
		return string(out), 0, fmt.Errorf("parse spawn output: %w", err)
	}
	limit := node.Spawn.MaxChildren
	if limit == 0 {
		limit = defaultSpawnMaxChildren
	}
	if len(children) > limit {
		return "", 0, fmt.Errorf("spawn emitted %d children, max=%d", len(children), limit)
	}
	if err := validateSpawnedKinds(children, node.Spawn.AllowedKinds); err != nil {
		return "", 0, err
	}
	for i := range children {
		// Each spawned node must validate on its own (kind body, etc.).
		if err := children[i].Validate(); err != nil {
			return "", 0, fmt.Errorf("child[%d] %q: %w", i, children[i].ID, err)
		}
	}
	if err := assertNoCycles(children); err != nil {
		return "", 0, fmt.Errorf("spawned subgraph: %w", err)
	}

	order, err := topoOrder(children)
	if err != nil {
		return "", 0, fmt.Errorf("spawned subgraph topo: %w", err)
	}
	emitted := make([]string, 0, len(children))
	for _, id := range order {
		child := findNode(children, id)
		// Tag spawned outputs under "<parent-id>/<child-id>" so the
		// audit log and downstream references can disambiguate from
		// statically-defined siblings with the same id.
		key := node.ID + "/" + child.ID
		// Use a fresh wrapper around the child so executeNode's audit
		// path sees a unique node id (the ".RunID" field on
		// nodeContext is shared; only the node id changes).
		wrapped := *child
		wrapped.ID = key
		res := r.executeNode(nc, &wrapped)
		nc.outputs[key] = res.Output
		if res.Error != nil {
			return summariseSpawn(emitted, key, res.Error), len(emitted), fmt.Errorf("spawned %q: %w", key, res.Error)
		}
		emitted = append(emitted, key)
	}
	return summariseSpawn(emitted, "", nil), len(emitted), nil
}

// parseSpawnOutput decodes the spawn command's stdout as a YAML
// sequence of Node values. Empty output means "spawn nothing"
// (legitimate — a probing node may decide there is no further work).
func parseSpawnOutput(stdout []byte) ([]Node, error) {
	if len(stdout) == 0 {
		return nil, nil
	}
	var nodes []Node
	if err := yaml.Unmarshal(stdout, &nodes); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	return nodes, nil
}

// validateSpawnedKinds enforces the AllowedKinds allowlist. Empty
// AllowedKinds means "any kind" (the default). When non-empty, every
// child's kind must be in the list — useful for sandboxing untrusted
// spawn output to bash-only.
func validateSpawnedKinds(children []Node, allowed []NodeKind) error {
	if len(allowed) == 0 {
		return nil
	}
	allowedSet := map[NodeKind]struct{}{}
	for _, k := range allowed {
		allowedSet[k] = struct{}{}
	}
	for i := range children {
		if _, ok := allowedSet[children[i].Kind]; !ok {
			return fmt.Errorf("child[%d] %q: kind %q not in allowed_kinds", i, children[i].ID, children[i].Kind)
		}
	}
	return nil
}

// assertNoCycles re-runs the cycle detector against the spawned
// subgraph. Necessary because Workflow.Validate runs once at load
// time and only sees statically-defined nodes.
func assertNoCycles(nodes []Node) error {
	return detectCycle(nodes)
}

// summariseSpawn formats the spawn node's own Output line. Lists the
// spawned ids; when an error happened mid-run, marks the failing id
// so `workflow logs` shows where the wave stopped.
func summariseSpawn(emitted []string, failed string, err error) string {
	if err == nil {
		return fmt.Sprintf("spawned %d node(s): %v", len(emitted), emitted)
	}
	return fmt.Sprintf("spawned %d node(s) before %q failed: %v", len(emitted), failed, emitted)
}

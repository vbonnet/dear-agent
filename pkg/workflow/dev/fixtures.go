// Package dev powers the `workflow dev` interactive shell. It provides
// the mock AI executor, fixture loader, hot-reloader, and REPL — every
// piece needed for sub-second iteration on a workflow change.
//
// Why a separate package: keeping the dev-mode plumbing out of
// pkg/workflow keeps the runner small. The runner exposes the
// AIExecutor interface; this package implements it with a
// fixture-backed mock that turns "run my workflow" into a
// deterministic, network-free, sub-second loop.
package dev

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// FixtureSet maps an AI node id to its canned response. Used by
// MockAIExecutor when running in dev mode. Fixtures are loaded from a
// YAML file at workflow-dev startup; reload re-reads the file.
type FixtureSet struct {
	// Path is the source file (informational; reload reads from here).
	Path string

	// Responses is the per-node canned output. Lookups fall back to
	// Default when a node id is missing — keeps the dev loop usable
	// before the operator has authored every fixture.
	Responses map[string]string

	// Default is the response returned when a node id has no entry
	// in Responses. Empty Default + missing key returns an error so
	// the operator notices the gap.
	Default string

	mu sync.RWMutex
}

// LoadFixtures reads a fixture YAML and returns a FixtureSet. The file
// shape is intentionally simple — a single flat mapping plus an
// optional `_default` key:
//
//	research: |
//	  ## Findings
//	  ...
//	review: "lgtm"
//	_default: "fixture not yet authored"
//
// Missing files produce an empty FixtureSet (not an error) so the
// operator can run dev mode before any fixtures are authored.
func LoadFixtures(path string) (*FixtureSet, error) {
	fs := &FixtureSet{Path: path, Responses: map[string]string{}}
	if path == "" {
		return fs, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is operator-supplied
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fs, nil
		}
		return nil, fmt.Errorf("dev: read fixtures %s: %w", path, err)
	}
	raw := map[string]string{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("dev: parse fixtures %s: %w", path, err)
	}
	for k, v := range raw {
		if k == "_default" {
			fs.Default = v
			continue
		}
		fs.Responses[k] = v
	}
	return fs, nil
}

// Reload re-reads the fixture file and replaces the in-memory state
// atomically. Returns the count of node-keys after the reload.
func (fs *FixtureSet) Reload() (int, error) {
	fresh, err := LoadFixtures(fs.Path)
	if err != nil {
		return 0, err
	}
	fs.mu.Lock()
	fs.Responses = fresh.Responses
	fs.Default = fresh.Default
	fs.mu.Unlock()
	return len(fresh.Responses), nil
}

// Get returns the response for a node id and a flag indicating whether
// it came from the per-node map (true) or the _default fallback
// (false). When neither is set, ok=false and the empty string is
// returned — callers should treat that as a fixture gap.
func (fs *FixtureSet) Get(nodeID string) (string, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if v, ok := fs.Responses[nodeID]; ok {
		return v, true
	}
	if fs.Default != "" {
		return fs.Default, false
	}
	return "", false
}

// Keys returns the fixture node ids sorted alphabetically. Used by the
// REPL `fixtures` command for inspection.
func (fs *FixtureSet) Keys() []string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	out := make([]string, 0, len(fs.Responses))
	for k := range fs.Responses {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MockAIExecutor satisfies workflow.AIExecutor by returning canned
// responses from a FixtureSet. Used in dev mode to skip the LLM round
// trip entirely — every "iterate on a prompt" loop becomes a local,
// deterministic run.
type MockAIExecutor struct {
	Fixtures *FixtureSet

	// FailOnGap, when true, returns an error on a node id that has no
	// fixture and no default. When false (the default), the executor
	// returns a placeholder string so the run still completes — useful
	// for early dev when fixtures lag the workflow.
	FailOnGap bool
}

// Generate satisfies workflow.AIExecutor.
func (m *MockAIExecutor) Generate(_ context.Context, node *workflow.AINode, _ map[string]string, _ map[string]string) (string, error) {
	id := nodeIDFromAINode(node)
	if m.Fixtures == nil {
		if m.FailOnGap {
			return "", fmt.Errorf("dev: no fixtures loaded for node %q", id)
		}
		return placeholder(id), nil
	}
	if v, ok := m.Fixtures.Get(id); ok {
		return v, nil
	}
	if m.FailOnGap {
		return "", fmt.Errorf("dev: fixture missing for node %q (and no _default)", id)
	}
	return placeholder(id), nil
}

// nodeIDFromAINode is best-effort: AINode doesn't carry the parent
// node's id today (the runner threads it down separately), so we use
// the prompt's first line as a heuristic when nothing else is
// available. The mock executor is informational; downstream callers
// pass NodeID via the inputs map when they want a strict match.
func nodeIDFromAINode(n *workflow.AINode) string {
	if n == nil {
		return "(unknown)"
	}
	if n.Role != "" {
		return n.Role
	}
	if line := strings.SplitN(strings.TrimSpace(n.Prompt), "\n", 2)[0]; line != "" {
		if len(line) > 40 {
			return line[:40]
		}
		return line
	}
	return "(unknown)"
}

func placeholder(id string) string {
	return fmt.Sprintf("[mock %s]\n(no fixture authored; this is a dev-mode placeholder)\n", id)
}

// FixtureFile returns the conventional fixture path next to a workflow
// file: workflow.yaml → workflow.fixtures.yaml. Used by the CLI when
// the operator doesn't pass --fixtures explicitly.
func FixtureFile(workflowPath string) string {
	dir, base := filepath.Split(workflowPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+".fixtures.yaml")
}

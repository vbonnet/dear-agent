// Command workflow-lint validates workflow YAML files and surfaces
// schema and migration issues. Phase 1 ships --check-deprecated-models
// (the role-migration aid called out in ROADMAP.md) plus a default
// pass that runs Workflow.Validate over each input file.
//
// Usage:
//
//	workflow-lint workflows/*.yaml
//	workflow-lint --check-deprecated-models workflows/research.yaml
//	workflow-lint --deprecated-models claude-opus-4-7,gpt-4-turbo workflows/*.yaml
//
// Exit codes:
//   - 0: every file passed every requested check.
//   - 1: at least one file failed.
//   - 2: usage error (no inputs / unparseable flags).
//
// The intent is "loud in CI, quiet on the happy path": passing files
// produce no output unless --verbose is set; failing files emit one
// line per finding with the workflow file path and node id so editors
// can jump to the offending YAML.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// defaultDeprecatedModels is the seed list for --check-deprecated-models.
// Operators can override it via --deprecated-models. The list is
// deliberately small: the lint is a migration aid, not a gatekeeper —
// keeping the seed short avoids stale-list noise as vendors rev models.
var defaultDeprecatedModels = []string{
	"claude-opus-3",
	"claude-sonnet-3-5",
	"claude-haiku-3",
	"gpt-4",
	"gpt-4-turbo",
	"gpt-3.5-turbo",
	"gemini-1.5-pro",
}

func main() {
	os.Exit(run())
}

func run() int {
	var (
		checkDeprecated = flag.Bool("check-deprecated-models", false, "flag nodes whose model: or model_override: points at a deprecated model")
		deprecatedList  = flag.String("deprecated-models", "", "comma-separated deprecated model ids (default uses the built-in seed list)")
		verbose         = flag.Bool("verbose", false, "print per-file pass lines")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <workflow.yaml>...\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		flag.Usage()
		return 2
	}

	deprecated := defaultDeprecatedModels
	if *deprecatedList != "" {
		deprecated = splitCSV(*deprecatedList)
	}

	cfg := lintConfig{
		checkDeprecated:    *checkDeprecated,
		deprecatedModelSet: toSet(deprecated),
		verbose:            *verbose,
	}

	failed := 0
	for _, path := range files {
		findings := lintFile(path, cfg)
		if len(findings) == 0 {
			if cfg.verbose {
				fmt.Printf("ok      %s\n", path)
			}
			continue
		}
		failed++
		for _, f := range findings {
			fmt.Printf("%s\n", f)
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

// lintConfig groups the user-selected behaviour so lintFile is easy to
// test without flag plumbing.
type lintConfig struct {
	checkDeprecated    bool
	deprecatedModelSet map[string]struct{}
	verbose            bool
}

// lintFile loads, validates, and (optionally) deep-inspects a single
// workflow file. Returns one human-readable finding per problem.
func lintFile(path string, cfg lintConfig) []string {
	w, err := workflow.LoadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("ERROR  %s: %v", path, err)}
	}

	var findings []string
	if cfg.checkDeprecated {
		findings = append(findings, deprecatedModelFindings(path, w, cfg.deprecatedModelSet)...)
	}
	findings = append(findings, missingRoleFindings(path, w)...)
	return findings
}

// deprecatedModelFindings walks every AI node and emits a finding per
// hardcoded model: or model_override: that names a deprecated model.
func deprecatedModelFindings(path string, w *workflow.Workflow, dep map[string]struct{}) []string {
	var out []string
	walkAINodes(w.Nodes, func(parent string, n *workflow.Node) {
		if n.AI == nil {
			return
		}
		nodeRef := nodeRef(path, parent, n.ID)
		if n.AI.Model != "" {
			if _, deprecated := dep[n.AI.Model]; deprecated {
				out = append(out, fmt.Sprintf("DEPRECATED  %s: model: %q (replace with role:)", nodeRef, n.AI.Model))
			}
		}
		if n.AI.ModelOverride != "" {
			if _, deprecated := dep[n.AI.ModelOverride]; deprecated {
				out = append(out, fmt.Sprintf("DEPRECATED  %s: model_override: %q (drop the override or update the model id)", nodeRef, n.AI.ModelOverride))
			}
		}
	})
	sort.Strings(out)
	return out
}

// missingRoleFindings flags AI nodes that hard-code a model id without
// declaring a role. Phase 1 prefers role-based resolution; a node
// without role: still works (back-compat) but is discouraged.
func missingRoleFindings(path string, w *workflow.Workflow) []string {
	var out []string
	walkAINodes(w.Nodes, func(parent string, n *workflow.Node) {
		if n.AI == nil {
			return
		}
		if n.AI.Role == "" && n.AI.Model != "" {
			nodeRef := nodeRef(path, parent, n.ID)
			out = append(out, fmt.Sprintf("WARN        %s: hardcoded model: %q without role: (Phase 1 recommends declaring a role)", nodeRef, n.AI.Model))
		}
	})
	sort.Strings(out)
	return out
}

// walkAINodes recurses into loop bodies so nested AI nodes are linted
// alongside top-level ones. The parent argument is "" at the root and
// the loop node id inside a loop body — used to qualify the audit
// reference.
func walkAINodes(nodes []workflow.Node, fn func(parent string, n *workflow.Node)) {
	for i := range nodes {
		n := &nodes[i]
		fn("", n)
		if n.Kind == workflow.KindLoop && n.Loop != nil {
			for j := range n.Loop.Nodes {
				child := &n.Loop.Nodes[j]
				fn(n.ID, child)
			}
		}
	}
}

// nodeRef formats a finding's location as path:nodeId or path:loopId/nodeId.
func nodeRef(path, parent, nodeID string) string {
	if parent != "" {
		return fmt.Sprintf("%s:%s/%s", path, parent, nodeID)
	}
	return fmt.Sprintf("%s:%s", path, nodeID)
}

// splitCSV splits a comma-separated list and trims whitespace.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func toSet(names []string) map[string]struct{} {
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}

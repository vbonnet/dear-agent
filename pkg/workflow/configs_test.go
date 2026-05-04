package workflow_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// configsDir resolves configs/workflows/ relative to this test file.
// runtime.Caller is more robust than working-directory tricks because
// `go test ./...` runs each package from its own directory.
func configsDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "configs", "workflows")
}

// TestShippedTemplatesLoad guarantees every YAML under configs/workflows/
// parses and validates against the engine's schema. This was originally
// added to catch the audit-*.yaml regression where `command:` was used
// instead of `cmd:` (the loader silently accepted the file because all
// unknown YAML keys are ignored, but the resulting BashNode had an
// empty Cmd and Validate() rejected it). A whole-directory loader test
// is the smallest fixture that catches that class of bug.
func TestShippedTemplatesLoad(t *testing.T) {
	dir := configsDir(t)
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no YAML templates found in %s", dir)
	}

	for _, path := range matches {
		path := path
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(path) //nolint:gosec // test fixture
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			w, err := workflow.LoadBytes(body)
			if err != nil {
				t.Fatalf("LoadBytes: %v", err)
			}
			if w.Name == "" {
				t.Errorf("name is empty")
			}
			if len(w.Nodes) == 0 {
				t.Errorf("no nodes declared")
			}
		})
	}
}

// TestSignalsCollectShape pins the surface contract of the
// signals-collect template — name, inputs, single bash node — so a
// drive-by edit that drops an input or renames the node trips the
// test instead of silently breaking the cron line documented in the
// template comment header.
func TestSignalsCollectShape(t *testing.T) {
	w := loadShippedTemplate(t, "signals-collect.yaml")

	if w.Name != "signals-collect" {
		t.Errorf("Name = %q, want signals-collect", w.Name)
	}
	wantInputs := []string{"db", "repos", "repos_root", "lookback_days"}
	assertInputs(t, w, wantInputs)

	if len(w.Nodes) != 1 {
		t.Fatalf("Nodes = %d, want 1", len(w.Nodes))
	}
	n := w.Nodes[0]
	if n.ID != "collect" {
		t.Errorf("node ID = %q, want collect", n.ID)
	}
	if n.Kind != workflow.KindBash || n.Bash == nil {
		t.Fatalf("expected bash node, got kind=%s bash=%v", n.Kind, n.Bash)
	}
	if !strings.Contains(n.Bash.Cmd, "dear-agent-signals collect") {
		t.Errorf("bash.cmd does not invoke dear-agent-signals collect:\n%s", n.Bash.Cmd)
	}
	// Partial-failure semantics live in the script; assert the marker
	// strings are present so a refactor that flattens the loop trips
	// the test.
	for _, want := range []string{"OK", "FAILED", "SKIP", "summary:"} {
		if !strings.Contains(n.Bash.Cmd, want) {
			t.Errorf("bash.cmd missing marker %q (partial-failure semantics)", want)
		}
	}
}

// TestAuditMultiShape pins the audit-multi surface, mirroring the
// rationale on signals-collect.
func TestAuditMultiShape(t *testing.T) {
	w := loadShippedTemplate(t, "audit-multi.yaml")

	if w.Name != "audit-multi" {
		t.Errorf("Name = %q, want audit-multi", w.Name)
	}
	assertInputs(t, w, []string{"db", "repos", "repos_root", "cadence"})

	if len(w.Nodes) != 1 {
		t.Fatalf("Nodes = %d, want 1", len(w.Nodes))
	}
	n := w.Nodes[0]
	if n.Kind != workflow.KindBash || n.Bash == nil {
		t.Fatalf("expected bash node, got kind=%s bash=%v", n.Kind, n.Bash)
	}
	if !strings.Contains(n.Bash.Cmd, "workflow-audit run") {
		t.Errorf("bash.cmd does not invoke workflow-audit run:\n%s", n.Bash.Cmd)
	}
	if !strings.Contains(n.Bash.Cmd, "$INPUT_cadence") {
		t.Errorf("bash.cmd does not pass through INPUT_cadence — schedule docs assume the input is honored")
	}
}

// TestAuditSingleRepoTemplatesUseCmdField is the regression test for
// the `command:` typo bug. The shipped audit-{daily,weekly,monthly}
// templates were silently broken on main because YAML unknown keys are
// ignored; pinning the bash.cmd contents guarantees the migration to
// the correct field name is durable.
func TestAuditSingleRepoTemplatesUseCmdField(t *testing.T) {
	for _, name := range []string{"audit-daily.yaml", "audit-weekly.yaml", "audit-monthly.yaml"} {
		name := name
		t.Run(name, func(t *testing.T) {
			w := loadShippedTemplate(t, name)
			if len(w.Nodes) != 1 {
				t.Fatalf("Nodes = %d, want 1", len(w.Nodes))
			}
			n := w.Nodes[0]
			if n.Kind != workflow.KindBash || n.Bash == nil {
				t.Fatalf("expected bash node, got kind=%s bash=%v", n.Kind, n.Bash)
			}
			if n.Bash.Cmd == "" {
				t.Fatalf("bash.cmd is empty — likely the `command:` typo regression")
			}
			if !strings.Contains(n.Bash.Cmd, "workflow-audit run") {
				t.Errorf("bash.cmd does not invoke workflow-audit run:\n%s", n.Bash.Cmd)
			}
		})
	}
}

func loadShippedTemplate(t *testing.T, filename string) *workflow.Workflow {
	t.Helper()
	path := filepath.Join(configsDir(t), filename)
	body, err := os.ReadFile(path) //nolint:gosec // test fixture
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	w, err := workflow.LoadBytes(body)
	if err != nil {
		t.Fatalf("LoadBytes %s: %v", filename, err)
	}
	return w
}

func assertInputs(t *testing.T, w *workflow.Workflow, want []string) {
	t.Helper()
	got := make(map[string]bool, len(w.Inputs))
	for _, in := range w.Inputs {
		got[in.Name] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing input %q (declared inputs: %v)", name, inputNames(w))
		}
	}
}

func inputNames(w *workflow.Workflow) []string {
	names := make([]string, 0, len(w.Inputs))
	for _, in := range w.Inputs {
		names = append(names, in.Name)
	}
	return names
}

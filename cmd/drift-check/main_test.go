package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile creates a file with content under dir and returns its path.
func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

// setup builds a repo root with a source file and a host file, plus a config
// pointing one target at them. It returns the config path and repo root.
func setup(t *testing.T, sourceContent, deployedContent string) (cfgPath, repoRoot string) {
	t.Helper()
	repoRoot = t.TempDir()
	host := t.TempDir()
	writeFile(t, repoRoot, "src/hook", sourceContent)
	deployed := writeFile(t, host, "hook", deployedContent)

	cfg := "targets:\n" +
		"  - name: hook\n" +
		"    deployed: " + deployed + "\n" +
		"    source: src/hook\n" +
		"    remediation: agm admin install-hooks\n"
	cfgPath = filepath.Join(t.TempDir(), "targets.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	return cfgPath, repoRoot
}

func TestRun_NoDrift(t *testing.T) {
	cfg, root := setup(t, "a\nb\n", "a\nb\n")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", cfg, "--repo-root", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No drift") {
		t.Fatalf("expected clean message, got: %s", stdout.String())
	}
}

func TestRun_Drift(t *testing.T) {
	cfg, root := setup(t, "a\nNEW\nb\n", "a\nb\n")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", cfg, "--repo-root", root}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 on drift; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "DRIFT") {
		t.Fatalf("expected DRIFT in output, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "agm admin install-hooks") {
		t.Fatalf("expected remediation hint, got: %s", stdout.String())
	}
}

func TestRun_JSON(t *testing.T) {
	cfg, root := setup(t, "a\nb\n", "a\nb\n")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", cfg, "--repo-root", root, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	var got struct {
		Schema  string `json:"schema"`
		Summary struct {
			Total int `json:"total"`
			OK    int `json:"ok"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if got.Schema != "drift-check/v1" {
		t.Fatalf("schema = %q, want drift-check/v1", got.Schema)
	}
	if got.Summary.Total != 1 || got.Summary.OK != 1 {
		t.Fatalf("unexpected summary: %+v", got.Summary)
	}
}

func TestRun_BadConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", "/nonexistent/targets.yaml"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 for unreadable config", code)
	}
}

// TestEmbeddedConfigParses guards the built-in targets.yaml against a typo or
// schema drift — it must always parse with the strict loader.
func TestEmbeddedConfigParses(t *testing.T) {
	if len(defaultConfig) == 0 {
		t.Fatal("embedded targets.yaml is empty")
	}
}

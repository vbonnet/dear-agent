package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/drift"
)

func TestMatchTargets(t *testing.T) {
	t.Parallel()
	cfg := drift.Config{Targets: []drift.Target{
		{Name: "stop-hook", Source: "agm/cmd/agm/hooks/stop-agm-resource-cleanup", Deployed: "~/.claude/hooks/stop"},
		{Name: "plist", Source: "deploy/launchd/com.dear-agent.mergeloop.plist", Deployed: "~/x.plist"},
		{Name: "settings", Source: "config/settings.json", Deployed: "~/.claude/settings.json"},
	}}

	got := matchTargets([]string{
		"README.md",
		"agm/cmd/agm/hooks/stop-agm-resource-cleanup",
		"config/settings.json",
	}, cfg)

	if len(got) != 2 {
		t.Fatalf("got %d targets, want 2: %+v", len(got), got)
	}
	// Config order is preserved: stop-hook before settings.
	if got[0].Name != "stop-hook" || got[1].Name != "settings" {
		t.Errorf("unexpected order/selection: %s, %s", got[0].Name, got[1].Name)
	}
}

func TestMatchTargets_NoMatch(t *testing.T) {
	t.Parallel()
	cfg := drift.Config{Targets: []drift.Target{
		{Name: "a", Source: "src/a", Deployed: "~/a"},
	}}
	if got := matchTargets([]string{"src/b", "docs/c.md"}, cfg); len(got) != 0 {
		t.Errorf("expected no match, got %+v", got)
	}
}

func TestMatchTargets_NormalisesAndTrims(t *testing.T) {
	t.Parallel()
	cfg := drift.Config{Targets: []drift.Target{
		{Name: "a", Source: "src/a", Deployed: "~/a"},
	}}
	// Surrounding whitespace and an empty entry must not break matching.
	if got := matchTargets([]string{"  src/a  ", ""}, cfg); len(got) != 1 {
		t.Errorf("expected 1 match after trim, got %+v", got)
	}
}

func TestDeployedGate_NoRepoRoot(t *testing.T) {
	t.Parallel()
	findings, checked, note := deployedGate(context.Background(), "", "", []string{"src/a"})
	if findings != nil || checked != 0 {
		t.Errorf("expected no findings/checked, got %+v / %d", findings, checked)
	}
	if note == "" {
		t.Error("expected a skip note explaining the gate did not run")
	}
}

func TestDeployedGate_ConfigMissing(t *testing.T) {
	t.Parallel()
	// A repo root that exists but has no deploy-target config → skip, not block.
	findings, _, note := deployedGate(context.Background(), t.TempDir(), "", []string{"src/a"})
	if findings != nil {
		t.Errorf("expected no findings, got %+v", findings)
	}
	if !strings.Contains(note, driftConfigRelPath) {
		t.Errorf("skip note should name the missing config, got %q", note)
	}
}

func TestDeployedGate_NoTouchedTargets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeDriftConfig(t, root, `targets:
  - name: a
    deployed: /tmp/never
    source: src/a
`)
	// Changed file does not match the target's source → gate runs clean.
	findings, checked, note := deployedGate(context.Background(), root, "", []string{"docs/unrelated.md"})
	if findings != nil || checked != 0 || note != "" {
		t.Errorf("expected clean no-op, got findings=%+v checked=%d note=%q", findings, checked, note)
	}
}

func TestDeployedGate_DriftAndMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	host := t.TempDir()

	// Two sources in the repo.
	mustWrite(t, filepath.Join(root, "src", "stale"), "NEW source content\n")
	mustWrite(t, filepath.Join(root, "src", "gone"), "should be installed\n")

	// One deployed copy is stale; the other was never installed.
	stalePath := filepath.Join(host, "stale-deployed")
	missingPath := filepath.Join(host, "missing-deployed")
	mustWrite(t, stalePath, "OLD deployed content\n")

	writeDriftConfig(t, root, `targets:
  - name: stale-hook
    deployed: `+stalePath+`
    source: src/stale
    remediation: agm admin install-hooks
  - name: missing-hook
    deployed: `+missingPath+`
    source: src/gone
    remediation: agm admin install-hooks
`)

	findings, checked, note := deployedGate(context.Background(), root, "",
		[]string{"src/stale", "src/gone"})
	if note != "" {
		t.Fatalf("unexpected skip note: %q", note)
	}
	if checked != 2 {
		t.Fatalf("checked = %d, want 2", checked)
	}
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(findings), findings)
	}

	byTarget := map[string]DeployFinding{}
	for _, f := range findings {
		byTarget[f.Target] = f
	}
	if byTarget["stale-hook"].Status != "drift" {
		t.Errorf("stale-hook status = %q, want drift", byTarget["stale-hook"].Status)
	}
	if byTarget["missing-hook"].Status != "missing" {
		t.Errorf("missing-hook status = %q, want missing", byTarget["missing-hook"].Status)
	}
	if byTarget["stale-hook"].Remediation != "agm admin install-hooks" {
		t.Errorf("expected remediation echoed, got %q", byTarget["stale-hook"].Remediation)
	}
}

func TestDeployedGate_OptionalMissingNotBlocked(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "opt"), "content\n")
	writeDriftConfig(t, root, `targets:
  - name: optional-job
    deployed: /tmp/dear-agent-nonexistent-`+t.Name()+`.plist
    source: src/opt
    optional: true
    remediation: agm admin install-sweep-schedule
`)
	findings, checked, note := deployedGate(context.Background(), root, "", []string{"src/opt"})
	if note != "" {
		t.Fatalf("unexpected skip note: %q", note)
	}
	if checked != 1 {
		t.Fatalf("checked = %d, want 1", checked)
	}
	// An optional artifact that a machine has not installed must not block.
	if len(findings) != 0 {
		t.Errorf("optional missing should not block, got %+v", findings)
	}
}

func TestDeployedGate_Clean(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	host := t.TempDir()
	deployed := filepath.Join(host, "hook")
	mustWrite(t, filepath.Join(root, "src", "hook"), "identical\n")
	mustWrite(t, deployed, "identical\n")
	writeDriftConfig(t, root, `targets:
  - name: hook
    deployed: `+deployed+`
    source: src/hook
    remediation: agm admin install-hooks
`)
	findings, checked, note := deployedGate(context.Background(), root, "", []string{"src/hook"})
	if note != "" || checked != 1 || len(findings) != 0 {
		t.Errorf("expected clean check, got findings=%+v checked=%d note=%q", findings, checked, note)
	}
}

func TestDedupRemediations(t *testing.T) {
	t.Parallel()
	got := dedupRemediations([]DeployFinding{
		{Remediation: "agm admin install-hooks"},
		{Remediation: ""},
		{Remediation: "agm admin install-hooks"},
		{Remediation: "make install-bumblebee-launchagent"},
	})
	want := []string{"agm admin install-hooks", "make install-bumblebee-launchagent"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func writeDriftConfig(t *testing.T, root, body string) {
	t.Helper()
	mustWrite(t, filepath.Join(root, driftConfigRelPath), body)
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

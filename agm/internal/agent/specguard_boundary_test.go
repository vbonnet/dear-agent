package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/specguard"
)

type specGuardHookManifest struct {
	Hooks map[string][]specGuardHookGroup `json:"hooks"`
}

type specGuardHookGroup struct {
	Hooks []specGuardHookEntry `json:"hooks"`
}

type specGuardHookEntry struct {
	Command string `json:"command"`
}

// This test is the explicit drift boundary between AGM's canonical active
// harness registry and specguard's pinned top-level harness authority.
func TestActiveHarnessesRemainNonNeutralTopLevelSPECOwners(t *testing.T) {
	t.Parallel()
	sandbox := gittest.New(t)
	root := t.TempDir()
	sandbox.Run(t, root, "init")
	writeSpecGuardBoundaryFile(t, root, "README.md", "base\n")
	sandbox.Run(t, root, "add", "--", "README.md")
	sandbox.Run(t, root, "commit", "-m", "base")

	wantPaths := make(map[string]bool)
	for index, harness := range ActiveHarnesses() {
		specPath := harness + "/SPEC.md"
		featurePath := "features/" + harness + ".feature"
		writeSpecGuardBoundaryFile(t, root, specPath, fmt.Sprintf(
			"# Harness-local contract\n\n## Requirements\n\n**REGISTRY-%02d** The shared guard shall preserve the pinned top-level harness authority.\n\n## BDD Traceability\n\n- BDD: `%s`\n",
			index+1,
			featurePath,
		))
		writeSpecGuardBoundaryFile(t, root, featurePath, fmt.Sprintf(
			"# SPEC: %s\nFeature: Harness authority boundary\n  Scenario: Reject harness-local ownership\n    Given an active harness owner\n    Then the owner remains registration-local\n",
			specPath,
		))
		sandbox.Run(t, root, "add", "--", specPath, featurePath)
		wantPaths[specPath] = false
	}

	result := specguard.Evaluate(context.Background(), specguard.Request{
		Repository: root,
		Mode:       specguard.ModeStaged,
	})
	for _, finding := range result.Findings {
		if finding.Code == "non-neutral-spec-owner" {
			if _, expected := wantPaths[finding.Path]; expected {
				wantPaths[finding.Path] = true
			}
		}
	}
	for specPath, rejected := range wantPaths {
		if !rejected {
			t.Errorf("active harness owner %q was not rejected; findings=%#v", specPath, result.Findings)
		}
	}
}

func TestActiveHarnessesHaveOneCooperativeNativeSPECGuardAdapter(t *testing.T) {
	t.Parallel()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	manifests := map[string]string{
		"claude-code": ".claude/settings.json",
		"codex-cli":   ".codex/hooks.json",
		"pi-cli":      ".pi/hooks.json",
	}
	for _, harness := range ActiveHarnesses() {
		if harness == "agy" || harness == "opencode-cli" {
			continue
		}
		relative, exists := manifests[harness]
		if !exists {
			t.Errorf("active harness %q has no SPEC guard adapter disposition", harness)
			continue
		}
		data, err := os.ReadFile(filepath.Join(repositoryRoot, relative))
		if err != nil {
			t.Errorf("read %s manifest: %v", harness, err)
			continue
		}
		var manifest specGuardHookManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Errorf("decode %s manifest: %v", harness, err)
			continue
		}
		for _, event := range []string{"Stop", "SubagentStop"} {
			adapters := 0
			for _, group := range manifest.Hooks[event] {
				for _, hook := range group.Hooks {
					if strings.Contains(hook.Command, "cmd/spec-contract-hook") {
						adapters++
						if !strings.Contains(hook.Command, "--root") || !strings.Contains(hook.Command, "--event "+event) {
							t.Errorf("%s %s SPEC guard adapter does not bind root and event: %q", harness, event, hook.Command)
						}
						if !strings.Contains(hook.Command, "go run") {
							t.Errorf("%s %s SPEC guard adapter obscures its mutable-checkout execution boundary: %q", harness, event, hook.Command)
						}
					}
				}
			}
			if adapters != 1 {
				t.Errorf("active harness %q %s adapters = %d, want 1", harness, event, adapters)
			}
		}
	}
	// Antigravity has only native Stop (not SubagentStop) and uses the named
	// hook schema; OpenCode uses its project plugin/session.idle disposition.
	agyData, err := os.ReadFile(filepath.Join(repositoryRoot, ".agents", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var agy map[string]map[string][]specGuardHookEntry
	if err := json.Unmarshal(agyData, &agy); err != nil {
		t.Fatal(err)
	}
	if len(agy["spec-contract-guard"]["Stop"]) != 1 || agy["spec-contract-guard"]["SubagentStop"] != nil {
		t.Fatalf("Antigravity disposition = %#v", agy["spec-contract-guard"])
	}
	plugin, err := os.ReadFile(filepath.Join(repositoryRoot, ".opencode", "plugins", "spec-contract-guard.js"))
	if err != nil || !strings.Contains(string(plugin), "session.idle") || !strings.Contains(string(plugin), "promptAsync") {
		t.Fatalf("OpenCode plugin disposition missing: %v", err)
	}
}

func writeSpecGuardBoundaryFile(t *testing.T, root, relative, body string) {
	t.Helper()
	filePath := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

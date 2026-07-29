package hookparity

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDangerousOverrideHookRoutesRawCodexThroughAuthorization(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("hook requires jq")
	}
	home := t.TempDir()
	binDir := filepath.Join(home, "go", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(home, "authorize.args")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$AGM_HOOK_CAPTURE\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "agm"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}

	output := runDangerousOverrideHook(t, home,
		"codex --dangerously-bypass-hook-trust; true",
		"AGM_CODEX_HOOK_TRUST_REASON=sandbox path rotates and cannot be pre-trusted",
		"AGM_HOOK_CAPTURE="+capture,
	)
	if strings.TrimSpace(output) != "" {
		t.Fatalf("authorized hook output = %q, want allow with no decision", output)
	}
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("canonical authorizer was not invoked: %v", err)
	}
	got := string(raw)
	for _, want := range []string{
		"override authorize codex-hook-trust",
		"--reason sandbox path rotates and cannot be pre-trusted",
		"--actor codex-pretool-hook",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("authorize args = %q, missing %q", got, want)
		}
	}
}

func TestDangerousOverrideHookDeniesRawCodexWithoutParentReason(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("hook requires jq")
	}
	output := runDangerousOverrideHook(t, t.TempDir(),
		"codex --dangerously-bypass-hook-trust; true",
	)
	var response struct {
		HookSpecificOutput struct {
			PermissionDecision string `json:"permissionDecision"`
			Reason             string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("decode denial %q: %v", output, err)
	}
	if response.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("decision = %q, want deny", response.HookSpecificOutput.PermissionDecision)
	}
	if !strings.Contains(response.HookSpecificOutput.Reason, "AGM_CODEX_HOOK_TRUST_REASON") {
		t.Fatalf("denial does not explain parent reason: %q", response.HookSpecificOutput.Reason)
	}
}

func TestDangerousOverrideHookDefersSingleInstalledAGMCommand(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("hook requires jq")
	}
	home := t.TempDir()
	binDir := filepath.Join(home, "go", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(home, "authorize.args")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$AGM_HOOK_CAPTURE\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "agm"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}

	output := runDangerousOverrideHook(t, home,
		`agm session new --dangerously-bypass-hook-trust="sandbox path rotates and cannot be pre-trusted"`,
		"AGM_HOOK_CAPTURE="+capture,
	)
	if strings.TrimSpace(output) != "" {
		t.Fatalf("installed AGM command output = %q, want deferred authorization", output)
	}
	if _, err := os.Stat(capture); !os.IsNotExist(err) {
		t.Fatalf("hook recorded the AGM-owned authorization a second time: %v", err)
	}
}

func runDangerousOverrideHook(t *testing.T, home, command string, extraEnv ...string) string {
	t.Helper()
	root := filepath.Join("..", "..")
	script := filepath.Join(root, ".codex", "hooks", "pretool-dangerous-override-guard")
	input, err := json.Marshal(map[string]any{
		"tool_name": "Bash",
		"tool_input": map[string]string{
			"command": command,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+filepath.Join(home, "go", "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		t.Fatalf("run hook: %v", err)
	}
	return stdout.String()
}

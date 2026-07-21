//go:build contract

package contract

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/permissionparity"
)

func TestPiInstalledBinaryContract(t *testing.T) {
	path, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pi binary unavailable; install @earendil-works/pi-coding-agent")
	}
	output, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("pi --version: %v\n%s", err, output)
	}
	if strings.TrimSpace(string(output)) == "" {
		t.Fatal("pi --version returned empty output")
	}
	if got := agent.NormalizeHarnessName("pi"); got != "pi-cli" {
		t.Fatalf("NormalizeHarnessName(pi) = %q", got)
	}
}

func TestPiInstalledProjectLoaderDoesNotHitLegacyHooksMigration(t *testing.T) {
	path, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pi binary unavailable; install @earendil-works/pi-coding-agent")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve Pi contract source path")
	}
	command := exec.Command(path, "--approve", "--list-models")
	command.Dir = filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", ".."))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("pi project loader: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "Project hooks/ directory found") || strings.Contains(string(output), "Press any key to continue") {
		t.Fatalf("Pi project loader entered legacy hooks migration: %s", output)
	}
}

func TestPiManagedExtensionContract(t *testing.T) {
	path, err := permissionparity.EnsurePiAuthorizationExtension(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("managed Pi extension path is empty")
	}
	decision := permissionparity.DecidePiToolCall(
		"default",
		[]string{"Bash(git status)"},
		permissionparity.PiToolCall{ToolName: "bash", Input: map[string]any{"command": "git status"}},
		false,
	)
	if decision.Action != permissionparity.PiAllow {
		t.Fatalf("installed extension contract decision = %#v", decision)
	}
}

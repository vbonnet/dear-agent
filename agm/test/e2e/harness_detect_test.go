package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHarnessDetectionSupportsSystemBash(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve harness detection test source")
	}
	helper := filepath.Join(filepath.Dir(testFile), "lib", "harness-detect.sh")
	data, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "declare -A") {
		t.Fatal("harness detection uses associative arrays unsupported by macOS system Bash")
	}

	const assertions = `
set -euo pipefail
source "$1"
test "$(harness_command claude-code)" = claude
test "$(harness_command codex-cli)" = codex
test "$(harness_command gemini-cli)" = gemini
test "$(harness_command opencode-cli)" = opencode
if harness_command unknown-harness; then
    exit 1
fi
`
	cmd := exec.Command("/bin/bash", "-c", assertions, "harness-detect-test", helper)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("system Bash harness lookup failed: %v\n%s", err, output)
	}
}

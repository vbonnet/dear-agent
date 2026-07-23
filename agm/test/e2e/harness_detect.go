// Package e2e owns shared end-to-end test infrastructure and validators.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const portableHarnessAssertions = `
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

// ValidatePortableHarnessDetection is the canonical portability check shared
// by the package regression and the cross-language BDD contract.
func ValidatePortableHarnessDetection(helper string) error {
	data, err := os.ReadFile(helper)
	if err != nil {
		return fmt.Errorf("read harness detection helper: %w", err)
	}
	if strings.Contains(string(data), "declare -A") {
		return fmt.Errorf("harness detection uses associative arrays unsupported by macOS system Bash")
	}

	// #nosec G204 -- executable, assertions, and the caller-provided repository path are test-owned.
	cmd := exec.Command("/bin/bash", "-c", portableHarnessAssertions, "harness-detect-portability", helper)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("system Bash harness lookup: %w\n%s", err, output)
	}
	return nil
}

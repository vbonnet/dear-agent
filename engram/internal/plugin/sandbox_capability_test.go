package plugin

import (
	"os/exec"
	"runtime"
	"testing"
)

// requireExecutableSandbox skips executor integration tests when macOS refuses
// sandbox-exec itself. The tests verify behavior inside the OS sandbox, so a
// host that cannot apply one provides no meaningful integration signal.
func requireExecutableSandbox(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		return
	}

	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skipf("sandbox-exec is unavailable: %v", err)
	}

	output, err := exec.Command(
		"sandbox-exec", "-p", "(version 1)\n(allow default)", "/usr/bin/true",
	).CombinedOutput()
	if err != nil {
		t.Skipf("sandbox-exec cannot run in this environment: %v\n%s", err, output)
	}
}

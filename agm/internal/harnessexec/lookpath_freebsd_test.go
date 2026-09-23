//go:build freebsd

package harnessexec

import (
	"errors"
	"os/exec"
	"testing"
)

func TestResolveExecutableInEnvironmentRejectsFreeBSD(t *testing.T) {
	resolved, err := resolveExecutableInEnvironment("codex", []string{"PATH=/bin"})
	if resolved != "" {
		t.Fatalf("resolved path = %q, want empty", resolved)
	}
	var execErr *exec.Error
	if !errors.As(err, &execErr) {
		t.Fatalf("error = %T %v, want *exec.Error", err, err)
	}
	if execErr.Name != "codex" {
		t.Errorf("exec error name = %q, want codex", execErr.Name)
	}
	if !errors.Is(execErr.Err, errFreeBSDPrivateHarnessExecution) {
		t.Errorf("error = %v, want unsupported FreeBSD cause", err)
	}
}

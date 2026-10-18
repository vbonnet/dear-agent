package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Conformance: internal/deploy/SPEC.md DEP-31.
func TestInstallRecoveryLoopLaunchAgentMergesPulsesBeforePublishingJobs(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(raw)
	start := strings.Index(makefile, "install-recovery-loop-launchagent:")
	if start < 0 {
		t.Fatal("Makefile has no install-recovery-loop-launchagent target")
	}
	target, _, found := strings.Cut(makefile[start:], "\nuninstall-recovery-loop-launchagent:")
	if !found {
		t.Fatal("cannot find end of install-recovery-loop-launchagent target")
	}

	const merge = "go run ./cmd/dear-deploy merge-pulses"
	if got := strings.Count(target, merge); got != 1 {
		t.Fatalf("recovery-loop installer invokes locked pulse merger %d times, want exactly once", got)
	}
	const publishJobs = "cp deploy/recovery-loop/jobs.json"
	publishAt := strings.Index(target, publishJobs)
	if publishAt < 0 {
		t.Fatal("recovery-loop installer does not publish a missing jobs registry")
	}
	if mergeAt := strings.Index(target, merge); mergeAt > publishAt {
		t.Fatal("recovery-loop installer publishes jobs before merging their required pulses")
	}
}

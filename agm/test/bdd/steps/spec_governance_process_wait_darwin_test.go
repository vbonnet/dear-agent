//go:build darwin

package steps

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBoundedSpecAuditCommandFailsClosedOnDarwinStopWake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.Command("/bin/sh", "-c", "kill -STOP $$; exit 0")
	configureSpecAuditChildCommand(command)

	started := time.Now()
	_, err := runBoundedSpecAuditCommand(ctx, command, 4096)
	if err == nil || !strings.Contains(err.Error(), "non-terminal child state") {
		t.Fatalf("stopped child error = %v, want non-terminal waitid state", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("stopped child cleanup took %v, want prompt fail-closed termination", elapsed)
	}
}

func TestDarwinProcessGroupEPERMClassificationRequiresPinnedLeader(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "exit 0")
	configureSpecAuditChildCommand(command)
	if err := command.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	if err := waitForSpecAuditCommandExitWithoutReaping(command.Process.Pid); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("wait without reaping: %v", err)
	}
	complete, err := specAuditProcessGroupEPERMComplete(command.Process.Pid, true)
	if err != nil {
		_ = command.Wait()
		t.Fatalf("classify pinned leader: %v", err)
	}
	if !complete {
		_ = command.Wait()
		t.Fatal("unreaped leader-only group did not classify complete")
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("reap child after classification: %v", err)
	}

	complete, err = specAuditProcessGroupEPERMComplete(1<<30, true)
	if err != nil {
		t.Fatalf("classify absent leader: %v", err)
	}
	if complete {
		t.Fatal("empty process-group result classified complete without its pinned leader")
	}
}

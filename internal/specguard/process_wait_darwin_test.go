//go:build darwin

package specguard

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestGitWaitWithoutReapingRejectsDarwinStopWake(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "kill -STOP $$; exit 0")
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatalf("start stopped Git child: %v", err)
	}
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- waitForGitCommandExitWithoutReaping(command.Process.Pid)
	}()

	var waitErr error
	select {
	case waitErr = <-waitResult:
	case <-time.After(2 * time.Second):
		_ = killProcessGroup(command.Process)
		select {
		case waitErr = <-waitResult:
			_ = command.Wait()
			t.Fatalf("Darwin waitid reported the stopped Git child only after forced cleanup: %v", waitErr)
		case <-time.After(2 * time.Second):
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatal("Darwin waitid remained blocked after stopped-child cleanup")
		}
	}
	if err := killProcessGroup(command.Process); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("terminate stopped Git child: %v", err)
	}
	_ = command.Wait()
	if waitErr == nil || !strings.Contains(waitErr.Error(), "non-terminal Git child state") {
		t.Fatalf("stopped Git child wait error = %v, want non-terminal state", waitErr)
	}
}

func TestDarwinGitProcessGroupEPERMClassificationRequiresPinnedLeader(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "exit 0")
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatalf("start Git child: %v", err)
	}
	if err := waitForGitCommandExitWithoutReaping(command.Process.Pid); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("wait without reaping: %v", err)
	}
	complete, err := gitProcessGroupEPERMComplete(command.Process.Pid, true, false)
	if err != nil {
		_ = command.Wait()
		t.Fatalf("classify pinned Git leader: %v", err)
	}
	if !complete {
		_ = command.Wait()
		t.Fatal("unreaped Git leader-only group did not classify complete")
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("reap Git child after classification: %v", err)
	}

	complete, err = gitProcessGroupEPERMComplete(1<<30, true, false)
	if err != nil {
		t.Fatalf("classify absent Git leader: %v", err)
	}
	if complete {
		t.Fatal("empty Git process-group result classified complete without its pinned leader")
	}
}

func TestDarwinGitProcessGroupEPERMClassificationRejectsLiveGroup(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "exec sleep 30")
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatalf("start live Git child: %v", err)
	}
	defer func() {
		_ = killProcessGroup(command.Process)
		_ = command.Wait()
	}()

	started := time.Now()
	complete, err := gitProcessGroupEPERMComplete(command.Process.Pid, true, false)
	if err != nil {
		t.Fatalf("classify live Git group: %v", err)
	}
	if complete {
		t.Fatal("live Git group classified complete")
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("un-signaled live Git group classification took %s, want immediate failure", elapsed)
	}
}

func TestDarwinGitProcessGroupEPERMClassificationBoundsSignaledDrain(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "exec sleep 30")
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatalf("start live Git child: %v", err)
	}
	defer func() {
		_ = killProcessGroup(command.Process)
		_ = command.Wait()
	}()

	started := time.Now()
	complete, err := gitProcessGroupEPERMComplete(command.Process.Pid, true, true)
	if err != nil {
		t.Fatalf("classify persistently live signaled Git group: %v", err)
	}
	if complete {
		t.Fatal("persistently live signaled Git group classified complete")
	}
	if elapsed := time.Since(started); elapsed < gitDarwinProcessGroupDrainGrace || elapsed > 2*time.Second {
		t.Fatalf("persistently live signaled Git group classification took %s, want bounded cleanup grace", elapsed)
	}
}

func TestGitDarwinProcessGroupTerminal(t *testing.T) {
	const processGroupID = 42
	tests := []struct {
		name    string
		members []unix.KinfoProc
		want    bool
	}{
		{name: "empty snapshot"},
		{
			name:    "zombie leader",
			members: []unix.KinfoProc{darwinProcessGroupMember(processGroupID, processGroupID, gitDarwinProcessZombie)},
			want:    true,
		},
		{
			name: "zombie leader and descendant",
			members: []unix.KinfoProc{
				darwinProcessGroupMember(processGroupID, processGroupID, gitDarwinProcessZombie),
				darwinProcessGroupMember(processGroupID+1, processGroupID, gitDarwinProcessZombie),
			},
			want: true,
		},
		{
			name:    "live leader",
			members: []unix.KinfoProc{darwinProcessGroupMember(processGroupID, processGroupID, 2)},
		},
		{
			name: "live descendant",
			members: []unix.KinfoProc{
				darwinProcessGroupMember(processGroupID, processGroupID, gitDarwinProcessZombie),
				darwinProcessGroupMember(processGroupID+1, processGroupID, 2),
			},
		},
		{
			name:    "missing leader",
			members: []unix.KinfoProc{darwinProcessGroupMember(processGroupID+1, processGroupID, gitDarwinProcessZombie)},
		},
		{
			name: "mismatched group",
			members: []unix.KinfoProc{
				darwinProcessGroupMember(processGroupID, processGroupID, gitDarwinProcessZombie),
				darwinProcessGroupMember(processGroupID+1, processGroupID+1, gitDarwinProcessZombie),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := gitDarwinProcessGroupTerminal(test.members, processGroupID); got != test.want {
				t.Fatalf("terminal group = %v, want %v", got, test.want)
			}
		})
	}
}

func darwinProcessGroupMember(pid, processGroupID int, state int8) unix.KinfoProc {
	return unix.KinfoProc{
		Proc:  unix.ExternProc{P_pid: int32(pid), P_stat: state},
		Eproc: unix.Eproc{Pgid: int32(processGroupID)},
	}
}

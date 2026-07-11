// Package recovery verifies and escalates soft recovery for tmux-backed AGM
// sessions without killing the harness or tmux session.
package recovery

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/procreaper"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// Process is one descendant of the session pane process.
type Process struct {
	PID     int
	PPID    int
	Command string
	CmdLine string
}

// Snapshot captures the process descendants relevant to recovery verification.
type Snapshot struct {
	PanePID     int
	Descendants []Process
	WorkLeaves  []Process
}

// Fallback describes the safe escalation available after terminal keys fail.
type Fallback string

const (
	// FallbackNone leaves recovery unconfirmed rather than signaling processes.
	FallbackNone Fallback = "none"
	// FallbackLeafInterrupt sends SIGINT only to non-harness leaf processes.
	FallbackLeafInterrupt Fallback = "leaf-process-sigint"
)

// SnapshotSession captures the pane process subtree using the portable ps
// lister shared with AGM's process reaper.
func SnapshotSession(ctx context.Context, sessionName string) (Snapshot, error) {
	panePID, err := tmux.GetPanePID(sessionName)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve pane PID: %w", err)
	}
	processes, err := (procreaper.PSProcessLister{}).List(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list process table: %w", err)
	}
	return BuildSnapshot(panePID, processes), nil
}

// BuildSnapshot extracts a pane's full descendant tree and identifies
// non-harness leaves whose exit can confirm that an interrupted tool stopped.
func BuildSnapshot(panePID int, processes []procreaper.ProcessInfo) Snapshot {
	children := make(map[int][]procreaper.ProcessInfo)
	for _, process := range processes {
		children[process.PPID] = append(children[process.PPID], process)
	}

	result := Snapshot{PanePID: panePID}
	queue := append([]procreaper.ProcessInfo(nil), children[panePID]...)
	for len(queue) > 0 {
		process := queue[0]
		queue = queue[1:]
		converted := Process{PID: process.PID, PPID: process.PPID, Command: process.Command, CmdLine: process.CmdLine}
		result.Descendants = append(result.Descendants, converted)
		processChildren := children[process.PID]
		if len(processChildren) == 0 && !protectedRuntime(process) {
			result.WorkLeaves = append(result.WorkLeaves, converted)
		}
		queue = append(queue, processChildren...)
	}
	return result
}

// Confirmed reports recovery only when a pre-existing work process exited. If
// no work process existed before the keypress, a verified ready prompt is
// sufficient; capture-pane success alone never is.
func Confirmed(before, after Snapshot, promptReady bool) bool {
	if len(before.WorkLeaves) == 0 {
		return promptReady
	}
	alive := make(map[int]struct{}, len(after.Descendants))
	for _, process := range after.Descendants {
		alive[process.PID] = struct{}{}
	}
	for _, process := range before.WorkLeaves {
		if _, ok := alive[process.PID]; !ok {
			return true
		}
	}
	return false
}

// FallbackForHarness returns the explicit process-level recovery fallback.
func FallbackForHarness(harness string) Fallback {
	if agent.NormalizeHarnessName(harness) == "agy" {
		return FallbackLeafInterrupt
	}
	return FallbackNone
}

// InterruptWorkLeaves sends SIGINT to the observed non-harness leaf processes.
// It never signals the pane root, a harness runtime, or an intermediate shell.
func InterruptWorkLeaves(snapshot Snapshot) (int, error) {
	interrupted := 0
	var failures []string
	for _, process := range snapshot.WorkLeaves {
		if process.PID <= 1 {
			continue
		}
		if err := syscall.Kill(process.PID, syscall.SIGINT); err != nil {
			failures = append(failures, fmt.Sprintf("pid %d: %v", process.PID, err))
			continue
		}
		interrupted++
	}
	if len(failures) > 0 {
		return interrupted, fmt.Errorf("interrupt work leaves: %s", strings.Join(failures, "; "))
	}
	return interrupted, nil
}

func protectedRuntime(process procreaper.ProcessInfo) bool {
	name := strings.ToLower(filepath.Base(process.Command))
	for _, protected := range []string{
		"agm", "agy", "antigravity", "bash", "bun", "claude", "codex",
		"fish", "node", "opencode", "sh", "tmux", "zsh",
	} {
		if name == protected || strings.HasPrefix(name, protected+"-") {
			return true
		}
	}
	return false
}

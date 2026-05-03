package mcp

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFinder returns predefined results for FindByCommandLine.
type mockFinder struct {
	results    []ProcessInfo
	err        error
	calledWith string
}

func (m *mockFinder) FindByCommandLine(substring string) ([]ProcessInfo, error) {
	m.calledWith = substring
	return m.results, m.err
}

// mockKiller tracks Kill calls and optionally returns errors.
type mockKiller struct {
	killedPIDs []int
	errForPID  map[int]error
}

func (m *mockKiller) Kill(pid int) error {
	if err, ok := m.errForPID[pid]; ok {
		return err
	}
	m.killedPIDs = append(m.killedPIDs, pid)
	return nil
}

func TestCleanup_FindsAndKills(t *testing.T) {
	finder := &mockFinder{
		results: []ProcessInfo{
			{PID: 1001, CmdLine: "node mcp-server --sandbox /path"},
			{PID: 1002, CmdLine: "npx mcp-thing --sandbox /path"},
		},
	}
	killer := &mockKiller{errForPID: map[int]error{}}

	killed, err := CleanupSessionMCPProcesses(finder, killer, "session-123", "/path/to/sandbox")
	require.NoError(t, err)
	assert.Equal(t, 2, killed)
	assert.Equal(t, []int{1001, 1002}, killer.killedPIDs)
	assert.Equal(t, "/path/to/sandbox", finder.calledWith)
}

func TestCleanup_NoProcesses(t *testing.T) {
	finder := &mockFinder{results: nil}
	killer := &mockKiller{errForPID: map[int]error{}}

	killed, err := CleanupSessionMCPProcesses(finder, killer, "session-123", "/path/to/sandbox")
	require.NoError(t, err)
	assert.Equal(t, 0, killed)
	assert.Empty(t, killer.killedPIDs)
}

func TestCleanup_FallbackToSessionID(t *testing.T) {
	finder := &mockFinder{
		results: []ProcessInfo{
			{PID: 2001, CmdLine: "node mcp session-abc-123"},
		},
	}
	killer := &mockKiller{errForPID: map[int]error{}}

	killed, err := CleanupSessionMCPProcesses(finder, killer, "session-abc-123", "")
	require.NoError(t, err)
	assert.Equal(t, 1, killed)
	assert.Equal(t, "session-abc-123", finder.calledWith)
}

func TestCleanup_KillErrorNonFatal(t *testing.T) {
	finder := &mockFinder{
		results: []ProcessInfo{
			{PID: 3001, CmdLine: "mcp-server-1"},
			{PID: 3002, CmdLine: "mcp-server-2"},
			{PID: 3003, CmdLine: "mcp-server-3"},
		},
	}
	killer := &mockKiller{
		errForPID: map[int]error{
			3002: fmt.Errorf("permission denied"),
		},
	}

	killed, err := CleanupSessionMCPProcesses(finder, killer, "sess", "/sandbox")
	require.NoError(t, err)
	assert.Equal(t, 2, killed)
	assert.Equal(t, []int{3001, 3003}, killer.killedPIDs)
}

func TestCleanup_SkipsSelf(t *testing.T) {
	selfPID := os.Getpid()
	finder := &mockFinder{
		results: []ProcessInfo{
			{PID: selfPID, CmdLine: "self-process"},
			{PID: 4001, CmdLine: "mcp-server"},
		},
	}
	killer := &mockKiller{errForPID: map[int]error{}}

	killed, err := CleanupSessionMCPProcesses(finder, killer, "sess", "/sandbox")
	require.NoError(t, err)
	assert.Equal(t, 1, killed)
	assert.Equal(t, []int{4001}, killer.killedPIDs)
}

func TestCleanup_EmptySearchTerms(t *testing.T) {
	finder := &mockFinder{results: []ProcessInfo{{PID: 999, CmdLine: "x"}}}
	killer := &mockKiller{errForPID: map[int]error{}}

	killed, err := CleanupSessionMCPProcesses(finder, killer, "", "")
	require.NoError(t, err)
	assert.Equal(t, 0, killed)
}

func TestCleanup_FinderError(t *testing.T) {
	finder := &mockFinder{err: fmt.Errorf("proc read failed")}
	killer := &mockKiller{errForPID: map[int]error{}}

	killed, err := CleanupSessionMCPProcesses(finder, killer, "sess", "/sandbox")
	assert.Error(t, err)
	assert.Equal(t, 0, killed)
}

func TestProcFSFinder_FindsSelf(t *testing.T) {
	// Integration test: our own process should be findable via /proc.
	// /proc is Linux-only — on macOS the finder returns no results because
	// the directory doesn't exist, which makes the assertion meaningless.
	if runtime.GOOS != "linux" {
		t.Skip("ProcFSFinder requires Linux /proc filesystem")
	}
	finder := &ProcFSFinder{}

	// Use our own executable path as search term
	exe, err := os.Executable()
	if err != nil {
		t.Skip("cannot determine executable path")
	}

	procs, err := finder.FindByCommandLine(exe)
	require.NoError(t, err)

	// We should find at least our own process
	selfPID := os.Getpid()
	found := false
	for _, p := range procs {
		if p.PID == selfPID {
			found = true
			assert.True(t, strings.Contains(p.CmdLine, exe))
			break
		}
	}
	assert.True(t, found, "should find own process in /proc scan")
}

func TestProcFSFinder_EmptySubstring(t *testing.T) {
	finder := &ProcFSFinder{}
	procs, err := finder.FindByCommandLine("")
	require.NoError(t, err)
	assert.Nil(t, procs)
}

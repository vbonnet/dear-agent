package orphan

import (
	"errors"
	"reflect"
	"sort"
	"testing"
)

// sessionTree is a representative two-session process table. Session A is the
// one ending (the reaper runs under it); session B is a live sibling whose
// processes must never be touched.
//
//	1   launchd
//	├── 10  claude            (session A root)
//	│   ├── 11  agm-mcp-server (A target)
//	│   │   └── 12 gopls        (A target, nested under the MCP server)
//	│   └── 13  bash sessionend-closeout
//	│       └── 14 agm          (the reaper itself = selfPID)
//	└── 20  claude            (session B root — live sibling)
//	    ├── 21  agm-mcp-server (B target — must survive)
//	    └── 22  gopls          (B target — must survive)
func sessionTree() []Proc {
	return []Proc{
		{PID: 1, PPID: 0, Command: "launchd"},
		{PID: 10, PPID: 1, Command: "claude"},
		{PID: 11, PPID: 10, Command: "agm-mcp-server"},
		{PID: 12, PPID: 11, Command: "/opt/homebrew/bin/gopls"},
		{PID: 13, PPID: 10, Command: "bash"},
		{PID: 14, PPID: 13, Command: "agm"},
		{PID: 20, PPID: 1, Command: "claude"},
		{PID: 21, PPID: 20, Command: "agm-mcp-server"},
		{PID: 22, PPID: 20, Command: "gopls"},
	}
}

func TestFindAncestor(t *testing.T) {
	procs := sessionTree()
	tests := []struct {
		name    string
		start   int
		names   []string
		wantPID int
		wantOK  bool
	}{
		{"nearest claude from reaper", 14, DefaultRootNames, 10, true},
		{"start at the root itself", 10, DefaultRootNames, 10, true},
		{"no matching ancestor", 13, []string{"emacs"}, 0, false},
		{"broken chain (ppid missing)", 999, DefaultRootNames, 0, false},
		{"start below init only", 1, DefaultRootNames, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPID, gotOK := FindAncestor(procs, tt.start, tt.names)
			if gotPID != tt.wantPID || gotOK != tt.wantOK {
				t.Errorf("FindAncestor(%d) = (%d,%v), want (%d,%v)", tt.start, gotPID, gotOK, tt.wantPID, tt.wantOK)
			}
		})
	}
}

func TestFindAncestorCycle(t *testing.T) {
	// A <-> B parent cycle must not hang or match.
	procs := []Proc{
		{PID: 100, PPID: 200, Command: "bash"},
		{PID: 200, PPID: 100, Command: "bash"},
	}
	if _, ok := FindAncestor(procs, 100, DefaultRootNames); ok {
		t.Error("FindAncestor should not resolve a root inside a cycle")
	}
}

func TestFindDescendants(t *testing.T) {
	procs := sessionTree()
	// Reaper is pid 14; root is session A (pid 10). Expect A's gopls + mcp-server
	// (incl. the gopls nested under the MCP server), never session B's.
	got := FindDescendants(procs, 10, DefaultTargets, 14)
	var pids []int
	for _, p := range got {
		pids = append(pids, p.PID)
	}
	sort.Ints(pids)
	want := []int{11, 12}
	if !reflect.DeepEqual(pids, want) {
		t.Errorf("FindDescendants() pids = %v, want %v (must exclude sibling session B: 21,22)", pids, want)
	}
}

func TestFindDescendantsExcludesSelfAndRoot(t *testing.T) {
	// If the reaper itself were somehow a target name, it must still be excluded.
	procs := []Proc{
		{PID: 10, PPID: 1, Command: "claude"},
		{PID: 11, PPID: 10, Command: "gopls"}, // self
		{PID: 12, PPID: 10, Command: "gopls"}, // real target
	}
	got := FindDescendants(procs, 10, DefaultTargets, 11)
	if len(got) != 1 || got[0].PID != 12 {
		t.Errorf("FindDescendants() = %+v, want only pid 12", got)
	}
}

func TestReapSessionKillsOnlyEndingSession(t *testing.T) {
	lister := fakeLister{procs: sessionTree()}
	killer := &fakeKiller{}

	res, err := ReapSession(lister, killer, 14, 0, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("ReapSession() error = %v", err)
	}
	if !res.RootFound || res.RootPID != 10 {
		t.Errorf("root = (%v, pid %d), want (true, 10)", res.RootFound, res.RootPID)
	}
	sort.Ints(killer.killed)
	if !reflect.DeepEqual(killer.killed, []int{11, 12}) {
		t.Errorf("killed = %v, want [11 12] (session B's 21,22 must survive)", killer.killed)
	}
	if len(res.Killed) != 2 {
		t.Errorf("res.Killed = %d, want 2", len(res.Killed))
	}
}

func TestReapSessionDryRunKillsNothing(t *testing.T) {
	lister := fakeLister{procs: sessionTree()}
	killer := &fakeKiller{}

	res, err := ReapSession(lister, killer, 14, 0, nil, nil, true, nil)
	if err != nil {
		t.Fatalf("ReapSession() error = %v", err)
	}
	if len(res.Found) != 2 {
		t.Errorf("found = %d, want 2", len(res.Found))
	}
	if len(killer.killed) != 0 {
		t.Errorf("dry-run killed %v, want none", killer.killed)
	}
}

func TestReapSessionNoRootIsSafeNoOp(t *testing.T) {
	lister := fakeLister{procs: sessionTree()}
	killer := &fakeKiller{}

	// Start from a pid whose ancestry has no `claude` — e.g. a detached process.
	res, err := ReapSession(lister, killer, 1, 0, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("ReapSession() error = %v", err)
	}
	if res.RootFound {
		t.Errorf("RootFound = true, want false for rootless start")
	}
	if len(killer.killed) != 0 {
		t.Errorf("killed %v with no root, want none", killer.killed)
	}
}

func TestReapSessionRootOverride(t *testing.T) {
	lister := fakeLister{procs: sessionTree()}
	killer := &fakeKiller{}

	// Force root to session B directly, bypassing the ancestor walk.
	res, err := ReapSession(lister, killer, 0, 20, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("ReapSession() error = %v", err)
	}
	if res.RootPID != 20 || !res.RootFound {
		t.Errorf("root = (pid %d, %v), want (20, true)", res.RootPID, res.RootFound)
	}
	sort.Ints(killer.killed)
	if !reflect.DeepEqual(killer.killed, []int{21, 22}) {
		t.Errorf("killed = %v, want [21 22]", killer.killed)
	}
}

func TestReapSessionRecordsKillFailures(t *testing.T) {
	lister := fakeLister{procs: sessionTree()}
	killer := &fakeKiller{failPIDs: map[int]error{12: errors.New("permission denied")}}

	res, err := ReapSession(lister, killer, 14, 0, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("ReapSession() error = %v", err)
	}
	if len(res.Killed) != 1 || res.Killed[0].PID != 11 {
		t.Errorf("killed = %+v, want only pid 11", res.Killed)
	}
	if len(res.Failed) != 1 || res.Failed[0].PID != 12 {
		t.Errorf("failed = %+v, want only pid 12", res.Failed)
	}
	if res.KillError[12] != "permission denied" {
		t.Errorf("KillError[12] = %q, want %q", res.KillError[12], "permission denied")
	}
}

func TestReapSessionListError(t *testing.T) {
	lister := fakeLister{err: errors.New("ps blew up")}
	if _, err := ReapSession(lister, &fakeKiller{}, 14, 0, nil, nil, false, nil); err == nil {
		t.Fatal("expected error when lister fails")
	}
}

func TestReapSessionDefaults(t *testing.T) {
	lister := fakeLister{procs: sessionTree()}
	res, err := ReapSession(lister, &fakeKiller{}, 14, 0, nil, nil, false, nil)
	if err != nil {
		t.Fatalf("ReapSession() error = %v", err)
	}
	if !reflect.DeepEqual(res.Targets, DefaultTargets) {
		t.Errorf("targets = %v, want %v", res.Targets, DefaultTargets)
	}
	if !reflect.DeepEqual(res.RootNames, DefaultRootNames) {
		t.Errorf("rootNames = %v, want %v", res.RootNames, DefaultRootNames)
	}
}

package orphan

import (
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestProcIsOrphan(t *testing.T) {
	tests := []struct {
		name    string
		proc    Proc
		targets []string
		want    bool
	}{
		{
			name:    "gopls reparented to launchd is orphan",
			proc:    Proc{PID: 100, PPID: 1, Command: "gopls"},
			targets: DefaultTargets,
			want:    true,
		},
		{
			name:    "gopls with live parent is not orphan",
			proc:    Proc{PID: 100, PPID: 4242, Command: "gopls"},
			targets: DefaultTargets,
			want:    false,
		},
		{
			name:    "agm-mcp-server reparented is orphan",
			proc:    Proc{PID: 200, PPID: 1, Command: "agm-mcp-server"},
			targets: DefaultTargets,
			want:    true,
		},
		{
			name:    "full path executable matches on basename",
			proc:    Proc{PID: 300, PPID: 1, Command: "/opt/homebrew/bin/gopls"},
			targets: DefaultTargets,
			want:    true,
		},
		{
			name:    "non-target reparented process is left alone",
			proc:    Proc{PID: 400, PPID: 1, Command: "Dock"},
			targets: DefaultTargets,
			want:    false,
		},
		{
			name:    "substring of a target name does not match",
			proc:    Proc{PID: 500, PPID: 1, Command: "goplsx"},
			targets: DefaultTargets,
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.proc.IsOrphan(tt.targets); got != tt.want {
				t.Errorf("IsOrphan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindOrphans(t *testing.T) {
	procs := []Proc{
		{PID: 1, PPID: 0, Command: "launchd"},
		{PID: 100, PPID: 1, Command: "gopls"},          // orphan
		{PID: 101, PPID: 999, Command: "gopls"},        // live parent
		{PID: 200, PPID: 1, Command: "agm-mcp-server"}, // orphan
		{PID: 300, PPID: 1, Command: "claude"},         // not a target
		{PID: 400, PPID: 1, Command: "gopls"},          // self — must be excluded
	}
	got := FindOrphans(procs, DefaultTargets, 400)
	var gotPIDs []int
	for _, p := range got {
		gotPIDs = append(gotPIDs, p.PID)
	}
	sort.Ints(gotPIDs)
	want := []int{100, 200}
	if !reflect.DeepEqual(gotPIDs, want) {
		t.Errorf("FindOrphans() pids = %v, want %v", gotPIDs, want)
	}
}

// fakeLister returns a fixed process table.
type fakeLister struct {
	procs []Proc
	err   error
}

func (f fakeLister) List() ([]Proc, error) { return f.procs, f.err }

// fakeKiller records the PIDs it was asked to kill and can fail selected ones.
type fakeKiller struct {
	killed   []int
	failPIDs map[int]error
}

func (f *fakeKiller) Kill(pid int) error {
	f.killed = append(f.killed, pid)
	if err, ok := f.failPIDs[pid]; ok {
		return err
	}
	return nil
}

func TestReapKillsOrphans(t *testing.T) {
	lister := fakeLister{procs: []Proc{
		{PID: 100, PPID: 1, Command: "gopls"},
		{PID: 200, PPID: 1, Command: "agm-mcp-server"},
		{PID: 300, PPID: 555, Command: "gopls"}, // live, must survive
	}}
	killer := &fakeKiller{}

	res, err := Reap(lister, killer, DefaultTargets, false, nil)
	if err != nil {
		t.Fatalf("Reap() error = %v", err)
	}
	if len(res.Orphans) != 2 {
		t.Errorf("orphans = %d, want 2", len(res.Orphans))
	}
	if len(res.Killed) != 2 {
		t.Errorf("killed = %d, want 2", len(res.Killed))
	}
	sort.Ints(killer.killed)
	if !reflect.DeepEqual(killer.killed, []int{100, 200}) {
		t.Errorf("killed pids = %v, want [100 200]", killer.killed)
	}
}

func TestReapDryRunKillsNothing(t *testing.T) {
	lister := fakeLister{procs: []Proc{
		{PID: 100, PPID: 1, Command: "gopls"},
	}}
	killer := &fakeKiller{}

	res, err := Reap(lister, killer, DefaultTargets, true, nil)
	if err != nil {
		t.Fatalf("Reap() error = %v", err)
	}
	if len(res.Orphans) != 1 {
		t.Errorf("orphans = %d, want 1", len(res.Orphans))
	}
	if len(killer.killed) != 0 {
		t.Errorf("dry-run killed %v, want none", killer.killed)
	}
	if len(res.Killed) != 0 {
		t.Errorf("res.Killed = %d, want 0 on dry-run", len(res.Killed))
	}
}

func TestReapRecordsKillFailures(t *testing.T) {
	lister := fakeLister{procs: []Proc{
		{PID: 100, PPID: 1, Command: "gopls"},
		{PID: 200, PPID: 1, Command: "agm-mcp-server"},
	}}
	killer := &fakeKiller{failPIDs: map[int]error{200: errors.New("permission denied")}}

	res, err := Reap(lister, killer, DefaultTargets, false, nil)
	if err != nil {
		t.Fatalf("Reap() error = %v", err)
	}
	if len(res.Killed) != 1 || res.Killed[0].PID != 100 {
		t.Errorf("killed = %+v, want only pid 100", res.Killed)
	}
	if len(res.Failed) != 1 || res.Failed[0].PID != 200 {
		t.Errorf("failed = %+v, want only pid 200", res.Failed)
	}
	if res.KillError[200] != "permission denied" {
		t.Errorf("KillError[200] = %q, want %q", res.KillError[200], "permission denied")
	}
}

func TestReapListError(t *testing.T) {
	lister := fakeLister{err: errors.New("ps blew up")}
	_, err := Reap(lister, &fakeKiller{}, DefaultTargets, false, nil)
	if err == nil {
		t.Fatal("expected error when lister fails")
	}
}

func TestReapEmptyTargetsDefaults(t *testing.T) {
	lister := fakeLister{procs: []Proc{{PID: 100, PPID: 1, Command: "gopls"}}}
	killer := &fakeKiller{}
	res, err := Reap(lister, killer, nil, false, nil)
	if err != nil {
		t.Fatalf("Reap() error = %v", err)
	}
	if !reflect.DeepEqual(res.Targets, DefaultTargets) {
		t.Errorf("targets = %v, want defaults %v", res.Targets, DefaultTargets)
	}
	if len(res.Killed) != 1 {
		t.Errorf("killed = %d, want 1", len(res.Killed))
	}
}

func TestParsePS(t *testing.T) {
	// The 4th line reproduces the real macOS case: comm is truncated to 16 chars
	// ("/Users/vbonnet/g") while argv[0] in the args column is the full path.
	// Command must come from argv[0] so its basename matches a target name.
	out := `  100     1 gopls /opt/homebrew/bin/gopls -mode stdio
  200   555 agm-mcp-server /Users/x/go/bin/agm-mcp-server
  300     1 claude /Applications/Claude.app/Contents/MacOS/claude
38435 38427 /Users/vbonnet/g /Users/vbonnet/go/bin/agm-mcp-server
garbage line with no numbers
`
	procs := parsePS(out)
	if len(procs) != 4 {
		t.Fatalf("parsePS returned %d procs, want 4: %+v", len(procs), procs)
	}
	want := []Proc{
		{PID: 100, PPID: 1, Command: "/opt/homebrew/bin/gopls", CmdLine: "/opt/homebrew/bin/gopls -mode stdio"},
		{PID: 200, PPID: 555, Command: "/Users/x/go/bin/agm-mcp-server", CmdLine: "/Users/x/go/bin/agm-mcp-server"},
		{PID: 300, PPID: 1, Command: "/Applications/Claude.app/Contents/MacOS/claude", CmdLine: "/Applications/Claude.app/Contents/MacOS/claude"},
		{PID: 38435, PPID: 38427, Command: "/Users/vbonnet/go/bin/agm-mcp-server", CmdLine: "/Users/vbonnet/go/bin/agm-mcp-server"},
	}
	if !reflect.DeepEqual(procs, want) {
		t.Errorf("parsePS() = %+v\nwant %+v", procs, want)
	}
	// Regression: the truncated-comm process must yield a Command whose basename
	// matches its target name (the bug was Command coming from the truncated
	// comm "/Users/vbonnet/g", basename "g", which matched nothing).
	if got := basename(procs[3].Command); got != "agm-mcp-server" {
		t.Errorf("truncated-comm proc Command basename = %q, want agm-mcp-server", got)
	}
}

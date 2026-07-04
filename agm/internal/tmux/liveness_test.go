package tmux

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestClassifyPaneLiveness covers the false-green class from ce-axsr/ce-qkf7:
// a tmux session that exists must only count as alive when a harness process
// is actually running in the pane's descendant tree.
func TestClassifyPaneLiveness(t *testing.T) {
	tests := []struct {
		name         string
		panePIDs     []int
		procs        []ProcEntry
		wantExists   bool
		wantAlive    bool
		wantZombie   bool
		wantEvidence string // substring that must appear in Evidence
	}{
		{
			name:       "no pane pids means session does not exist",
			panePIDs:   nil,
			procs:      []ProcEntry{{PID: 1, PPID: 0, Comm: "launchd"}},
			wantExists: false,
		},
		{
			name:     "zsh-only pane is dead (harness exited, pane fell back to shell)",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "zsh"},
			},
			wantExists:   true,
			wantAlive:    false,
			wantZombie:   false,
			wantEvidence: "zsh",
		},
		{
			name:     "claude child of pane shell is alive",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "zsh"},
				{PID: 200, PPID: 100, Comm: "claude"},
			},
			wantExists:   true,
			wantAlive:    true,
			wantZombie:   false,
			wantEvidence: "claude",
		},
		{
			name:     "agm-only child is dead with zombie-writer flag (ce-qkf7)",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "zsh"},
				{PID: 200, PPID: 100, Comm: "agm"},
			},
			wantExists:   true,
			wantAlive:    false,
			wantZombie:   true,
			wantEvidence: "agm",
		},
		{
			name:     "harness as grandchild under bash (crash-resume) is alive",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "zsh"},
				{PID: 200, PPID: 100, Comm: "bash"},
				{PID: 300, PPID: 200, Comm: "claude"},
			},
			wantExists: true,
			wantAlive:  true,
		},
		{
			name:     "claude semver comm counts as harness",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "zsh"},
				{PID: 200, PPID: 100, Comm: "2.1.50"},
			},
			wantExists: true,
			wantAlive:  true,
		},
		{
			name:     "node child (codex) counts as harness",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "zsh"},
				{PID: 200, PPID: 100, Comm: "/usr/local/bin/node"},
			},
			wantExists: true,
			wantAlive:  true,
		},
		{
			name:     "agm alongside live harness is NOT flagged as zombie writer",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "zsh"},
				{PID: 200, PPID: 100, Comm: "claude"},
				{PID: 201, PPID: 100, Comm: "agm"},
			},
			wantExists: true,
			wantAlive:  true,
			wantZombie: false,
		},
		{
			name:     "pane pid missing from process table proves nothing alive",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 999, PPID: 1, Comm: "claude"}, // unrelated process
			},
			wantExists: true,
			wantAlive:  false,
		},
		{
			name:     "harness in a second pane is alive",
			panePIDs: []int{100, 110},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "zsh"},
				{PID: 110, PPID: 1, Comm: "zsh"},
				{PID: 210, PPID: 110, Comm: "agy"},
			},
			wantExists: true,
			wantAlive:  true,
		},
		{
			name:     "deep agm descendant with no harness flags zombie writer",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "zsh"},
				{PID: 200, PPID: 100, Comm: "bash"},
				{PID: 300, PPID: 200, Comm: "/Users/x/go/bin/agm"},
			},
			wantExists: true,
			wantAlive:  false,
			wantZombie: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyPaneLiveness(tt.panePIDs, tt.procs, IsHarnessComm)
			if got.SessionExists != tt.wantExists {
				t.Errorf("SessionExists = %v, want %v", got.SessionExists, tt.wantExists)
			}
			if got.HarnessAlive != tt.wantAlive {
				t.Errorf("HarnessAlive = %v, want %v", got.HarnessAlive, tt.wantAlive)
			}
			if got.ZombieWriter != tt.wantZombie {
				t.Errorf("ZombieWriter = %v, want %v", got.ZombieWriter, tt.wantZombie)
			}
			if tt.wantEvidence != "" && !strings.Contains(got.Evidence, tt.wantEvidence) {
				t.Errorf("Evidence = %q, want substring %q", got.Evidence, tt.wantEvidence)
			}
		})
	}
}

func TestClassifyPaneLiveness_CustomPredicate(t *testing.T) {
	procs := []ProcEntry{
		{PID: 100, PPID: 1, Comm: "zsh"},
		{PID: 200, PPID: 100, Comm: "/opt/tools/codex"},
	}
	pred := func(comm string) bool { return filepath.Base(comm) == "codex" }
	got := ClassifyPaneLiveness([]int{100}, procs, pred)
	if !got.HarnessAlive {
		t.Error("expected codex matched by custom predicate to be alive")
	}
	predMiss := func(comm string) bool { return filepath.Base(comm) == "claude" }
	got = ClassifyPaneLiveness([]int{100}, procs, predMiss)
	if got.HarnessAlive {
		t.Error("expected non-matching predicate to report dead")
	}
}

func TestIsHarnessComm(t *testing.T) {
	tests := []struct {
		comm string
		want bool
	}{
		{"claude", true},
		{"/usr/local/bin/claude", true},
		{"codex", true},
		{"agy", true},
		{"node", true},
		{"gemini", true},
		{"opencode", true},
		{"2.1.50", true},   // Claude Code semver process name
		{"2_1_195", true},  // macOS underscore form
		{"2_1_195_", true}, // trailing tmux null placeholder
		{"zsh", false},
		{"bash", false},
		{"agm", false},
		{"vim", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsHarnessComm(tt.comm); got != tt.want {
			t.Errorf("IsHarnessComm(%q) = %v, want %v", tt.comm, got, tt.want)
		}
	}
}

func TestClassifyPaneLiveness_EvidenceTruncatesOnRuneBoundary(t *testing.T) {
	// Build a pane tree whose comm names exceed the evidence cap with
	// multi-byte runes right at the boundary.
	procs := []ProcEntry{{PID: 100, PPID: 1, Comm: strings.Repeat("é", 300)}}
	got := ClassifyPaneLiveness([]int{100}, procs, IsHarnessComm)
	if !strings.HasSuffix(got.Evidence, "...") {
		t.Fatalf("expected truncated evidence, got %q", got.Evidence)
	}
	for i, r := range got.Evidence {
		if r == '�' {
			t.Fatalf("evidence contains invalid UTF-8 at byte %d: %q", i, got.Evidence)
		}
	}
}

func TestParsePSTable(t *testing.T) {
	out := "  100     1 zsh\n" +
		"  200   100 /Users/x/My Projects/node\n" +
		"  300   200 claude\n" +
		"garbage line\n" +
		"  x     1 bad-pid\n" +
		"\n"
	entries := ParsePSTable(out)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(entries), entries)
	}
	if entries[1].Comm != "/Users/x/My Projects/node" {
		t.Errorf("comm with spaces mangled: %q", entries[1].Comm)
	}
	if entries[0].PID != 100 || entries[0].PPID != 1 {
		t.Errorf("bad first entry: %+v", entries[0])
	}
	if entries[2].PPID != 200 {
		t.Errorf("bad third entry: %+v", entries[2])
	}
}

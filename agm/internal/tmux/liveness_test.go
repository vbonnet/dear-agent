package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCheckPaneLivenessContextHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := CheckPaneLivenessContext(ctx, "canceled-liveness", filepath.Join(t.TempDir(), "tmux.sock"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckPaneLivenessContext() error = %v, want context.Canceled", err)
	}
}

func TestTmuxSessionExistenceResultDistinguishesOperationalFailures(t *testing.T) {
	t.Parallel()

	exitErr := errors.New("exit status 1")
	tests := []struct {
		name       string
		output     string
		err        error
		wantExists bool
		wantErr    string
	}{
		{name: "session exists", wantExists: true},
		{name: "explicit missing target", output: "can't find session: absent", err: exitErr},
		{name: "server unavailable", output: "no server running on /tmp/agm.sock", err: exitErr, wantErr: "no server running"},
		{name: "socket inaccessible", output: "error connecting to /tmp/agm.sock (Permission denied)", err: exitErr, wantErr: "Permission denied"},
		{name: "unclassified failure", err: exitErr, wantErr: "exit status 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			exists, err := tmuxSessionExistenceResult("target", []byte(tt.output), tt.err)
			if exists != tt.wantExists {
				t.Fatalf("exists = %v, want %v", exists, tt.wantExists)
			}
			if tt.wantErr == "" && err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestTmuxSessionExistsOnSocketDistinguishesMissingSessionFromSocketFailure(t *testing.T) {
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()
	if output, err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "existence-seed", "sleep", "30").CombinedOutput(); err != nil {
		t.Fatalf("create seed tmux session: %v: %s", err, output)
	}

	exists, err := tmuxSessionExistsOnSocket(t.Context(), "absent", socketPath)
	if err != nil || exists {
		t.Fatalf("explicit missing target = (exists=%v, err=%v), want (false, nil)", exists, err)
	}

	missingSocket := filepath.Join(socketDir(t), "missing.sock")
	exists, err = tmuxSessionExistsOnSocket(t.Context(), "absent", missingSocket)
	if err == nil || exists {
		t.Fatalf("missing socket = (exists=%v, err=%v), want backend error", exists, err)
	}
	if _, err := CheckPaneLivenessBatch([]string{"absent"}, missingSocket); err == nil {
		t.Fatal("batch liveness reported a dead session when the tmux socket was unavailable")
	}
}

func TestIsPiProcessInPaneTreeContextHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := IsPiProcessInPaneTreeContext(ctx, "canceled-pi-liveness", filepath.Join(t.TempDir(), "tmux.sock"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("IsPiProcessInPaneTreeContext() error = %v, want context.Canceled", err)
	}
}

func TestInspectNodeProcessArgsHonorsCancellationDuringFinalRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	entries := []procCommandEntry{{PID: 42, Command: "node /tmp/worker.js"}}
	err := inspectNodeProcessArgs(ctx, entries, func(pid int) ([]string, error) {
		if pid != 42 {
			t.Fatalf("read argv pid = %d, want 42", pid)
		}
		cancel()
		return []string{"node", "/tmp/worker.js"}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("inspectNodeProcessArgs() error = %v, want context.Canceled", err)
	}
	if entries[0].Args != nil {
		t.Fatalf("argv was committed after cancellation: %q", entries[0].Args)
	}
}

func TestIsPiProcessInPaneTreeRejectsDeadPaneWithPiStartCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tmux integration test in short mode")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	tempDir := t.TempDir()
	piPath := filepath.Join(tempDir, "pi")
	if err := os.Symlink("/usr/bin/false", piPath); err != nil {
		t.Fatalf("create fake Pi executable: %v", err)
	}
	if output, err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", "dead-pi-seed", "sleep", "30").CombinedOutput(); err != nil {
		t.Fatalf("create tmux seed session: %v: %s", err, output)
	}
	if output, err := exec.Command("tmux", "-S", socketPath, "set-option", "-g", "remain-on-exit", "on").CombinedOutput(); err != nil {
		t.Fatalf("set global remain-on-exit: %v: %s", err, output)
	}
	const sessionName = "dead-pi-start-command"
	if output, err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName, piPath).CombinedOutput(); err != nil {
		t.Fatalf("create fake Pi session: %v: %s", err, output)
	}

	deadline := time.Now().Add(2 * time.Second)
	var paneState string
	for time.Now().Before(deadline) {
		output, err := exec.Command("tmux", "-S", socketPath, "list-panes", "-t", sessionName, "-F", "#{pane_dead}\t#{pane_start_command}").Output()
		if err == nil {
			paneState = strings.TrimSpace(string(output))
			if strings.HasPrefix(paneState, "1\t") {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.HasPrefix(paneState, "1\t") || !strings.Contains(paneState, piPath) {
		t.Fatalf("pane did not retain the dead Pi start command: %q", paneState)
	}
	running, err := IsPiProcessInPaneTreeContext(t.Context(), sessionName, socketPath)
	if err != nil {
		t.Fatalf("Pi liveness scan: %v", err)
	}
	if running {
		t.Fatal("dead pane's retained Pi start command was accepted as live Pi")
	}
}

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
		wantShell    bool
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
			wantShell:    true,
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
			name:     "pi child of pane shell is alive",
			panePIDs: []int{100},
			procs: []ProcEntry{
				{PID: 100, PPID: 1, Comm: "-zsh"},
				{PID: 200, PPID: 100, Comm: "/opt/homebrew/bin/pi"},
			},
			wantExists:   true,
			wantAlive:    true,
			wantEvidence: "pi",
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
			if got.RestartableShell != tt.wantShell {
				t.Errorf("RestartableShell = %v, want %v", got.RestartableShell, tt.wantShell)
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

func TestParsePSArgsTable(t *testing.T) {
	t.Parallel()

	got := ParsePSArgsTable("  10 /usr/local/bin/node /opt/node_modules/@openai/codex/bin/codex.js --flag value\nmalformed\n  11 zsh\n")
	if got[10] != "/usr/local/bin/node /opt/node_modules/@openai/codex/bin/codex.js --flag value" {
		t.Fatalf("PID 10 args = %q", got[10])
	}
	if got[11] != "zsh" {
		t.Fatalf("PID 11 args = %q", got[11])
	}
	if len(got) != 2 {
		t.Fatalf("parsed args rows = %v, want two valid rows", got)
	}
}

func TestParsePSForegroundTable(t *testing.T) {
	t.Parallel()

	out := "  10 1 10 11 Ss /bin/zsh\n  11 10 11 11 S+ MainThread\n  12 10 12 11 T /path/with spaces/claude\nmalformed\n"
	got := ParsePSForegroundTable(out)
	want := []ProcEntry{
		{PID: 10, PPID: 1, PGID: 10, TPGID: 11, State: "Ss", Comm: "/bin/zsh"},
		{PID: 11, PPID: 10, PGID: 11, TPGID: 11, State: "S+", Comm: "MainThread"},
		{PID: 12, PPID: 10, PGID: 12, TPGID: 11, State: "T", Comm: "/path/with spaces/claude"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParsePSForegroundTable() = %#v, want %#v", got, want)
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
		{"pi", true},
		{"/opt/homebrew/bin/pi", true},
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

func TestPaneCommandMatchesProcessIsExact(t *testing.T) {
	if !paneCommandMatchesProcess("zsh\n/opt/homebrew/bin/pi\n", "pi") {
		t.Fatal("full-path Pi foreground command was not recognized")
	}
	if paneCommandMatchesProcess("zsh\npiper\n", "pi") {
		t.Fatal("partial Pi process name was accepted")
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

func TestPiProcessCommandRequiresPiSpecificIdentity(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{command: "pi --session-id abc", want: true},
		{command: "/opt/homebrew/bin/pi --session-id abc", want: true},
		{command: "/opt/homebrew/bin/node /opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js --session-id abc", want: true},
		{command: "node --enable-source-maps /usr/local/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js", want: true},
		{command: "node --max-old-space-size=1024 --inspect=127.0.0.1:0 /usr/local/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js", want: true},
		{command: "node --preserve-symlinks --require /tmp/register.cjs /usr/local/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js", want: true},
		{command: "node --require /tmp/My Projects/register.cjs /usr/local/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js", want: true},
		{command: "node --require /tmp/support.js Files/register.js /usr/local/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js", want: true},
		{command: "/opt/homebrew/bin/node '/Users/me/My Projects/node_modules/@earendil-works/pi-coding-agent/dist/cli.js' --session-id quoted", want: true},
		{command: "env PI_OFFLINE=1 node /usr/local/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js", want: true},
		{command: "node /usr/local/lib/node_modules/@openai/codex/dist/cli.js"},
		{command: "node /tmp/worker.js /Users/me/My Projects/node_modules/@earendil-works/pi-coding-agent/dist/cli.js"},
		{command: "node /tmp/worker /Users/me/My Projects/node_modules/@earendil-works/pi-coding-agent/dist/cli.js"},
		{command: "node --require /usr/local/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js /tmp/worker.js"},
		{command: "node --future-option /usr/local/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js"},
		{command: "node /tmp/pi"},
		{command: "node -e console.log('/opt/homebrew/bin/pi')"},
		{command: "zsh -l"},
	}
	for _, test := range tests {
		if got := isPiProcessCommand(test.command); got != test.want {
			t.Errorf("isPiProcessCommand(%q) = %v, want %v", test.command, got, test.want)
		}
	}
}

func TestPiProcessCommandAcceptsExistingUnquotedSpacedPackageEntry(t *testing.T) {
	entry := filepath.Join(t.TempDir(), "My Projects", "node_modules", "@earendil-works", "pi-coding-agent", "dist", "cli.js")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatalf("create spaced Pi package path: %v", err)
	}
	if err := os.WriteFile(entry, []byte("// Pi test entrypoint\n"), 0o644); err != nil {
		t.Fatalf("write spaced Pi package entrypoint: %v", err)
	}
	if !isPiProcessCommand("node --enable-source-maps " + entry + " --session-id spaced") {
		t.Fatal("existing unquoted Pi package path containing spaces was rejected")
	}
	if isPiProcessCommand("node /tmp/worker " + entry) {
		t.Fatal("unrelated extensionless Node entrypoint smuggled a later Pi path")
	}
}

func TestPiProcessArgsPreserveSpacedScriptAndOptionValues(t *testing.T) {
	piEntry := "/Users/me/My Projects/node_modules/@earendil-works/pi-coding-agent/dist/cli.js"
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "spaced Pi script", args: []string{"node", piEntry}, want: true},
		{name: "spaced preload", args: []string{"node", "--require", "/tmp/My Projects/register.cjs", piEntry}, want: true},
		{name: "Pi is only preload value", args: []string{"node", "--require", piEntry, "/tmp/worker.js"}},
		{name: "runtime flags", args: []string{"node", "--enable-source-maps", "--inspect=127.0.0.1:0", piEntry}, want: true},
		{name: "env assignments", args: []string{"env", "PI_OFFLINE=1", "NODE_NO_WARNINGS=1", "node", piEntry}, want: true},
		{name: "env without executable", args: []string{"env", "PI_OFFLINE=1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isPiProcessArgsWithResolver(test.args, filepath.EvalSymlinks); got != test.want {
				t.Fatalf("isPiProcessArgsWithResolver(%q) = %v, want %v", test.args, got, test.want)
			}
		})
	}
}

func TestProcessCommandExecutableFindsNodeThroughEnv(t *testing.T) {
	if got := processCommandExecutable("env PI_OFFLINE=1 /opt/homebrew/bin/node /tmp/pi.js"); got != "node" {
		t.Fatalf("processCommandExecutable through env = %q, want node", got)
	}
}

func TestReadProcessArgvCurrentProcess(t *testing.T) {
	args, err := readProcessArgv(os.Getpid())
	if err != nil {
		t.Fatalf("read current process argv: %v", err)
	}
	if !reflect.DeepEqual(args, os.Args) {
		t.Fatalf("current process argv = %q, want %q", args, os.Args)
	}
}

func TestPiProcessCommandAcceptsOnlyShimResolvingToPackageEntry(t *testing.T) {
	resolve := func(path string) (string, error) {
		if path == "/opt/homebrew/bin/pi" {
			return "/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js", nil
		}
		return "", errors.New("not a Pi package shim")
	}
	if !isPiProcessCommandWithResolver("node /opt/homebrew/bin/pi --session-id abc", resolve) {
		t.Fatal("Pi npm shim resolving to the package entry was rejected")
	}
	if isPiProcessCommandWithResolver("node /tmp/bin/pi --session-id abc", resolve) {
		t.Fatal("unrelated Node script named pi was accepted")
	}
}

func TestParsePSCommandTableAndPiProcessTree(t *testing.T) {
	procs := parsePSCommandTable("  100     1 /bin/zsh -l\n" +
		"  200   100 /opt/homebrew/bin/node /opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js --session-id abc\n" +
		"  300     1 /opt/homebrew/bin/node /work/codex.js\n" +
		"bad row\n")
	if len(procs) != 3 {
		t.Fatalf("parsePSCommandTable() returned %d rows: %+v", len(procs), procs)
	}
	if !piProcessInPaneTree([]int{100}, procs) {
		t.Fatal("Pi Node entrypoint was not found below the pane shell")
	}
	if piProcessInPaneTree([]int{300}, procs) {
		t.Fatal("generic Node process was accepted as Pi")
	}
}

func TestPiProcessTreeUsesLosslessNodeArgvAndFailsClosed(t *testing.T) {
	piEntry := "/Users/me/My Projects/node_modules/@earendil-works/pi-coding-agent/dist/cli.js"
	procs := []procCommandEntry{
		{PID: 100, PPID: 1, Command: "/bin/zsh -l"},
		{
			PID:           200,
			PPID:          100,
			Command:       "node --require /tmp/My Projects/register.cjs " + piEntry,
			Args:          []string{"node", "--require", "/tmp/My Projects/register.cjs", piEntry},
			ArgvInspected: true,
		},
		{
			PID:           300,
			PPID:          1,
			Command:       "node " + piEntry,
			Args:          []string{"node", "/tmp/worker.js", piEntry},
			ArgvInspected: true,
		},
		{
			PID:           400,
			PPID:          1,
			Command:       "node " + piEntry,
			ArgvInspected: true,
		},
	}
	if !piProcessInPaneTree([]int{100}, procs) {
		t.Fatal("lossless Pi argv beneath the pane shell was rejected")
	}
	if piProcessInPaneTree([]int{300}, procs) {
		t.Fatal("flattened command text overrode the lossless non-Pi argv")
	}
	if piProcessInPaneTree([]int{400}, procs) {
		t.Fatal("failed lossless argv inspection fell back to flattened command text")
	}
}

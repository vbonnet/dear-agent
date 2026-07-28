package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// injectionPayload is a directory name that is also a shell fragment. Under the
// pre-ce-93lw.1 hand-written quoting it closed the opening quote, ran `touch
// pwned`, and reopened a quote so the rest of the command still parsed. It
// contains no `/`, so it is a legal directory name on APFS and ext4 and the
// test can use it as a real working directory rather than a synthetic string.
const injectionPayload = `x'; touch pwned; echo '`

// runThroughShell executes cmdText with /bin/sh, with a stub for harnessName
// first on PATH, and returns the argv that stub received.
//
// This is the whole point of the test: asserting that a built command "does not
// contain `; touch`" would pass on a string that is merely mangled, and would
// pass on a payload the author of the assertion did not think of. Handing the
// command to a real POSIX shell and reading back argv states the actual
// security property — the hostile value arrives as one argument, as data, not
// as syntax.
func runThroughShell(t *testing.T, cmdText, harnessName, workDir string) []string {
	t.Helper()

	stubDir := t.TempDir()
	argvOut := filepath.Join(stubDir, "argv")

	stub := filepath.Join(stubDir, harnessName)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGV_OUT\"\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	cmd := exec.Command("/bin/sh", "-c", cmdText)
	cmd.Dir = workDir
	// The stub shadows any real harness binary, but the rest of PATH stays
	// intact on purpose: the payload calls `touch`, and a PATH containing only
	// the stub would make it fail to resolve. The test would then pass because
	// the injection could not find its tool, not because the injection was
	// blocked — a false green of exactly the kind this bead exists to remove.
	// TestLegacyQuotingWasExploitable is what catches that regression.
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"ARGV_OUT="+argvOut,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shell rejected built command: %v\ncommand: %s\noutput: %s", err, cmdText, out)
	}

	data, err := os.ReadFile(argvOut)
	if err != nil {
		t.Fatalf("stub %q never ran, so the command did not reach it: %v", harnessName, err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// assertNoPwned walks root and fails if the payload managed to create a file
// anywhere beneath it. The payload's `touch pwned` is relative, so it lands in
// whatever directory the shell was in when it ran — which differs between the
// start commands (no `cd`) and the resume commands (which `cd` into the
// hostile directory first). Walking covers both without the test having to
// predict which.
func assertNoPwned(t *testing.T, root string) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // unreadable entries are not evidence of injection
		}
		if info.Name() == "pwned" {
			t.Errorf("INJECTION: payload executed and created %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

// hostileWorkDir creates a real directory whose name is the injection payload.
func hostileWorkDir(t *testing.T) (root, workDir string) {
	t.Helper()
	root = t.TempDir()
	workDir = filepath.Join(root, injectionPayload)
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatalf("create hostile workdir: %v", err)
	}
	return root, workDir
}

func requirePOSIXShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
}

// TestInjectionNeutralized covers SPEC R31-R34 and R61: a hostile working
// directory must reach the harness as a single inert argument on every legacy
// adapter start and resume path.
func TestInjectionNeutralized(t *testing.T) {
	requirePOSIXShell(t)

	tests := []struct {
		name    string
		harness string
		build   func(workDir string) string
		// wantArgv is the argv the harness stub must observe, with %s
		// substituted for the hostile working directory.
		wantArgv []string
	}{
		{
			name:     "claude start",
			harness:  "claude",
			build:    func(wd string) string { return buildClaudeStartCommand(wd, nil) },
			wantArgv: []string{"--add-dir", "%s"},
		},
		{
			name:     "claude start with authorized dir",
			harness:  "claude",
			build:    func(wd string) string { return buildClaudeStartCommand(wd, []string{injectionPayload}) },
			wantArgv: []string{"--add-dir", "%s", "--add-dir", injectionPayload},
		},
		{
			name:     "claude resume",
			harness:  "claude",
			build:    func(wd string) string { return buildClaudeResumeCommand(wd, injectionPayload) },
			wantArgv: []string{"--resume", injectionPayload},
		},
		{
			name:     "gemini start",
			harness:  "gemini",
			build:    func(wd string) string { return buildGeminiStartCommand(wd, nil) },
			wantArgv: []string{"--include-directories", "%s"},
		},
		{
			name:     "gemini resume with uuid",
			harness:  "gemini",
			build:    func(wd string) string { return buildGeminiResumeCommand(wd, injectionPayload) },
			wantArgv: []string{"--resume", injectionPayload},
		},
		{
			name:     "gemini resume without uuid",
			harness:  "gemini",
			build:    func(wd string) string { return buildGeminiResumeCommand(wd, "") },
			wantArgv: []string{"--resume", "latest"},
		},
		{
			name:     "opencode resume",
			harness:  "opencode",
			build:    buildOpenCodeResumeCommand,
			wantArgv: []string{"attach"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, workDir := hostileWorkDir(t)

			// The start commands have no `cd`, so run them from the hostile
			// directory to give the payload the same opportunity the resume
			// commands get.
			gotArgv := runThroughShell(t, tt.build(workDir), tt.harness, workDir)

			want := make([]string, len(tt.wantArgv))
			for i, a := range tt.wantArgv {
				want[i] = strings.ReplaceAll(a, "%s", workDir)
			}

			if len(gotArgv) != len(want) {
				t.Fatalf("argv length: got %d %q, want %d %q", len(gotArgv), gotArgv, len(want), want)
			}
			for i := range want {
				if gotArgv[i] != want[i] {
					t.Errorf("argv[%d]: got %q, want %q", i, gotArgv[i], want[i])
				}
			}

			assertNoPwned(t, root)
		})
	}
}

// TestLegacyQuotingWasExploitable pins the bug this change fixes. If it ever
// stops failing to neutralize the payload, the payload has gone stale and the
// tests above are no longer proving anything.
func TestLegacyQuotingWasExploitable(t *testing.T) {
	requirePOSIXShell(t)

	root, workDir := hostileWorkDir(t)

	// Verbatim reconstruction of the pre-ce-93lw.1 construction.
	legacy := fmt.Sprintf("claude --add-dir '%s'", workDir) + " && exit"

	runThroughShell(t, legacy, "claude", workDir)

	found := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Name() == "pwned" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("payload %q no longer exploits the legacy quoting; "+
			"TestInjectionNeutralized is therefore not proving anything and the payload needs updating",
			injectionPayload)
	}
}

// TestWellFormedCommandsUnchanged covers SPEC R5. Quoting must not alter the
// command produced for an ordinary path, or this change silently breaks the
// live spawn path for four harnesses.
func TestWellFormedCommandsUnchanged(t *testing.T) {
	const wd = "/home/user/work"

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "claude start",
			got:  buildClaudeStartCommand(wd, nil),
			want: "claude --add-dir '/home/user/work' && exit",
		},
		{
			name: "claude start skips duplicate authorized dir",
			got:  buildClaudeStartCommand(wd, []string{wd, "/srv/extra"}),
			want: "claude --add-dir '/home/user/work' --add-dir '/srv/extra' && exit",
		},
		{
			name: "claude resume",
			got:  buildClaudeResumeCommand(wd, "abc-123"),
			want: "cd '/home/user/work' && claude --resume 'abc-123' && exit",
		},
		{
			name: "gemini start",
			got:  buildGeminiStartCommand(wd, nil),
			want: "gemini --include-directories '/home/user/work' && exit",
		},
		{
			name: "gemini resume with uuid",
			got:  buildGeminiResumeCommand(wd, "abc-123"),
			want: "cd '/home/user/work' && gemini --resume 'abc-123' && exit",
		},
		{
			name: "gemini resume falls back to latest",
			got:  buildGeminiResumeCommand(wd, ""),
			want: "cd '/home/user/work' && gemini --resume latest && exit",
		},
		{
			name: "opencode resume",
			got:  buildOpenCodeResumeCommand(wd),
			want: "cd '/home/user/work' && opencode attach && exit",
		},
		{
			name: "setdir",
			got:  buildSetDirCommand(wd),
			want: "cd '/home/user/work'\r",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got  %q\nwant %q", tt.got, tt.want)
			}
		})
	}
}

// TestSetDirQuotesEvenThoughValidatorGuardsIt covers SPEC R7. ValidateSendDirPath
// rejects these paths today, so the quoting here is defense in depth: the
// builder must not rely on a validator two call frames away that a later change
// could relax.
func TestSetDirQuotesEvenThoughValidatorGuardsIt(t *testing.T) {
	got := buildSetDirCommand(injectionPayload)
	want := `cd 'x'"'"'; touch pwned; echo '"'"''` + "\r"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestPastedLaunchRejectsTerminalControlsBeforeTmux(t *testing.T) {
	root := t.TempDir()
	sentinel := filepath.Join(root, "terminal-paste-pwned")
	payloads := []string{
		"safe\x1b[201~\x15touch " + sentinel + "\n",
		"safe\rcommand",
		"safe\tcompletion",
		string([]byte{'s', 'a', 'f', 'e', 0xff}),
	}
	for _, payload := range payloads {
		err := sendPastedShellCommand(
			"missing-session-must-not-be-contacted",
			buildClaudeStartCommand(payload, nil),
			payload,
		)
		if err == nil {
			t.Fatalf("sendPastedShellCommand(%q) = nil, want rejection", payload)
		}
		if !strings.Contains(err.Error(), "pasted shell value") {
			t.Fatalf("sendPastedShellCommand(%q) error = %v, want pre-tmux validation", payload, err)
		}
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("terminal paste payload executed or sentinel check failed: %v", err)
	}
}

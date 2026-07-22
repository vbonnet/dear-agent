package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// The tests in launch_commands_test.go hand each built command straight to
// /bin/sh. That proves the string is correct, but not that it survives the way
// AGM actually delivers it: tmux load-buffer -> paste-buffer -> the pane's
// shell. A quoting fix that a paste path mangles is no fix at all, and the
// mangling would only show up against a live tmux server.
//
// These tests close that gap. They follow the same shape as the direct-shell
// suite — a stub harness that records its argv, and a canary proving the
// legacy quoting is still exploitable through this exact path, so a passing
// run cannot be vacuous.

// sendThroughTmux starts a private tmux server, sends cmdText to its pane with
// the same tmux.SendCommand the adapters use, and returns the argv the stub
// harness observed.
//
// The pane runs /bin/sh rather than the user's login shell on purpose: an
// interactive zsh re-sources the user's rc files and rebuilds PATH, which would
// put a real harness binary ahead of the stub. `-e PATH=` pins the pane's
// environment. The real system directories stay on PATH so the payload's
// `touch` can resolve — a PATH holding only the stub would make the injection
// fail to find its tool and the test pass for the wrong reason.
func sendThroughTmux(t *testing.T, sessionName, cmdText, harnessName, paneDir string) []string {
	t.Helper()

	stubDir := t.TempDir()
	argvOut := filepath.Join(stubDir, "argv")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argvOut + "\n"
	if err := os.WriteFile(filepath.Join(stubDir, harnessName), []byte(stub), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	// Not t.TempDir(): its path embeds the test name, and a unix socket path is
	// capped near 104 bytes on macOS. A long test name silently pushes the
	// socket over the limit and tmux fails with "File name too long".
	socketDir, err := os.MkdirTemp("", "agmsock")
	if err != nil {
		t.Fatalf("socket dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socket := filepath.Join(socketDir, "s")
	panePath := stubDir + string(os.PathListSeparator) + "/usr/bin:/bin:/usr/sbin:/sbin"

	newSession := exec.Command("tmux", "-S", socket,
		"new-session", "-d", "-s", sessionName,
		"-c", paneDir,
		"-e", "PATH="+panePath,
		"/bin/sh")
	newSession.Env = append(os.Environ(), "PATH="+panePath)
	if out, err := newSession.CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socket, "kill-server").Run()
	})

	// tmux.SendCommand reads the socket from the environment.
	t.Setenv("AGM_TMUX_SOCKET", socket)
	if err := tmux.SendCommand(sessionName, cmdText); err != nil {
		t.Fatalf("tmux.SendCommand: %v", err)
	}

	// The pane runs asynchronously; poll for the stub's argv file.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(argvOut); err == nil && len(data) > 0 {
			return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		}
		time.Sleep(100 * time.Millisecond)
	}

	pane, _ := exec.Command("tmux", "-S", socket, "capture-pane", "-p", "-t", sessionName).Output()
	t.Fatalf("stub %q never ran, so the command did not reach it.\ncommand: %s\npane:\n%s",
		harnessName, cmdText, pane)
	return nil
}

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
}

// TestInjectionNeutralizedThroughTmux is the end-to-end form of
// TestInjectionNeutralized: a hostile working directory must reach the harness
// as one inert argument after travelling through a real tmux paste buffer.
func TestInjectionNeutralizedThroughTmux(t *testing.T) {
	requirePOSIXShell(t)
	requireTmux(t)

	root, workDir := hostileWorkDir(t)
	argv := sendThroughTmux(t, "ce93lw1-fixed", buildClaudeStartCommand(workDir, nil), "claude", root)

	want := []string{"--add-dir", workDir}
	if len(argv) != len(want) {
		t.Fatalf("argv = %q, want %q", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
	assertNoPwned(t, root)
}

// TestLegacyQuotingWasExploitableThroughTmux is the canary. It reconstructs the
// pre-ce-93lw.1 command text and requires that it still gets exploited through
// this delivery path. If it stops failing, the harness — not the fix — is what
// changed, and TestInjectionNeutralizedThroughTmux above has become vacuous.
func TestLegacyQuotingWasExploitableThroughTmux(t *testing.T) {
	requirePOSIXShell(t)
	requireTmux(t)

	root, workDir := hostileWorkDir(t)
	legacy := "claude --add-dir '" + workDir + "' && exit"
	sendThroughTmux(t, "ce93lw1-legacy", legacy, "claude", root)

	// The payload's `touch pwned` is relative and the pane starts in root.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(root, "pwned")); err == nil {
			return // legacy quoting is still exploitable here, as it must be
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("legacy quoting was not exploited through tmux: the payload %q no longer "+
		"attacks the old command form, so TestInjectionNeutralizedThroughTmux proves nothing",
		injectionPayload)
}

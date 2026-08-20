package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

const (
	realHostReadinessTimeout     = 15 * time.Second
	slowRealHostReadinessTimeout = 20 * time.Second
)

func setupRealReadinessTmux(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping isolated tmux readiness integration in short mode")
	}
	if os.Getenv("CI_SKIP_TMUX") == "true" {
		t.Skip("skipping isolated tmux readiness integration because CI_SKIP_TMUX=true")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not available")
	}
	if err := exec.Command("ps", "-axo", "pid=,ppid=,comm=").Run(); err != nil {
		t.Skipf("process-table inspection is unavailable: %v", err)
	}
	dir, err := os.MkdirTemp("", "agm-ready") //nolint:usetesting // macOS Unix socket paths must stay short
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	socketPath := filepath.Join(dir, "agm.sock")
	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()
		_ = os.RemoveAll(dir)
	})
	return socketPath
}

func installFakeCodexProcess(t *testing.T) string {
	return installFakeHarnessProcess(t, "codex")
}

func installFakeHarnessProcess(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	fakeHarness := filepath.Join(dir, name)
	source := filepath.Join(dir, "main.go")
	program := "package main\nimport \"time\"\nfunc main() { time.Sleep(10 * time.Second) }\n"
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatalf("write fake %s source: %v", name, err)
	}
	if output, err := exec.Command("go", "build", "-o", fakeHarness, source).CombinedOutput(); err != nil {
		t.Fatalf("build fake %s executable: %v\n%s", name, err, output)
	}
	return fakeHarness
}

func installInteractiveFakeGeminiProcess(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fakeGemini := filepath.Join(dir, "gemini")
	source := filepath.Join(dir, "main.go")
	program := `package main
import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)
func main() {
	fmt.Println("Do you trust the files in this folder?")
	fmt.Println("● 1. Trust folder")
	fmt.Println("  2. Do not trust")
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	if strings.TrimSpace(answer) != "1" {
		os.Exit(2)
	}
	fmt.Println("│ >   Type your message or @path/to/file │")
	fmt.Println("? for shortcuts")
	time.Sleep(10 * time.Second)
}
`
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatalf("write fake Gemini source: %v", err)
	}
	if output, err := exec.Command("go", "build", "-o", fakeGemini, source).CombinedOutput(); err != nil {
		t.Fatalf("build fake Gemini executable: %v\n%s", err, output)
	}
	return fakeGemini
}

func sendReadinessTestCommand(t *testing.T, socketPath, sessionName, command string) {
	t.Helper()
	target := tmux.NormalizeTmuxSessionName(sessionName)
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", target, "-l", command).CombinedOutput(); err != nil {
		t.Fatalf("send literal readiness command: %v\n%s", err, output)
	}
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", target, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("submit readiness command: %v\n%s", err, output)
	}
}

func TestRealTmuxReadinessDetectsFakeCodexComposer(t *testing.T) {
	socketPath := setupRealReadinessTmux(t)
	sessionName := "real-ready-codex"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	realTmux := NewRealTmux()
	before, err := realTmux.CheckInputReadiness(context.Background(), sessionName, "codex-cli")
	if err != nil {
		t.Fatalf("CheckInputReadiness(shell) error = %v", err)
	}
	if before.Ready {
		t.Fatalf("bare shell classified ready: %#v", before)
	}

	fakeCodex := installFakeCodexProcess(t)
	command := fmt.Sprintf("printf '│ >_ OpenAI Codex (vtest) │\\n│ model: gpt-5.5 /model to change │\\n›\\n'; exec %q 10", fakeCodex)
	sendReadinessTestCommand(t, socketPath, sessionName, command)
	if err := realTmux.WaitForHarnessReady(context.Background(), sessionName, "codex-cli", realHostReadinessTimeout); err != nil {
		t.Fatalf("WaitForHarnessReady(fake Codex) error = %v", err)
	}
}

func TestRealTmuxReadinessFailsFastForCodexHookReviewAboveBlankRows(t *testing.T) {
	socketPath := setupRealReadinessTmux(t)
	sessionName := "real-ready-codex-hooks"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	fakeCodex := installFakeCodexProcess(t)
	command := fmt.Sprintf("printf 'Hooks need review\\n\\n11 hooks are new or changed.\\n\\nHooks can run outside the sandbox after you trust them.\\n\\n› 1. Review hooks\\n  2. Trust all and continue\\n  3. Continue without trusting (hooks will not run)\\n\\nPress enter to confirm or esc to go back\\n'; exec %q", fakeCodex)
	sendReadinessTestCommand(t, socketPath, sessionName, command)

	start := time.Now()
	err := NewRealTmux().WaitForHarnessReady(t.Context(), sessionName, "codex-cli", realHostReadinessTimeout)
	if !errors.Is(err, tmux.ErrCodexHookReviewRequired) {
		pane, captureErr := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", tmux.NormalizeTmuxSessionName(sessionName), "-p", "-S", "-30").CombinedOutput()
		t.Fatalf("WaitForHarnessReady(hook review) error = %v, want ErrCodexHookReviewRequired; capture error = %v; pane:\n%s", err, captureErr, pane)
	}
	if elapsed := time.Since(start); elapsed > 7*time.Second {
		t.Fatalf("shared hook-review failure took %v, want prompt failure", elapsed)
	}
}

func TestRealTmuxReadinessDetectsManagedPiComposer(t *testing.T) {
	socketPath := setupRealReadinessTmux(t)
	sessionName := "real-ready-pi"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	fakePi := installFakeHarnessProcess(t, "pi")
	command := fmt.Sprintf("printf '/work • pi-worker\\nAGM default/ready launch-shared\\n'; exec %q 10", fakePi)
	sendReadinessTestCommand(t, socketPath, sessionName, command)
	if err := NewRealTmux().WaitForHarnessReady(t.Context(), sessionName, "pi-cli", realHostReadinessTimeout); err != nil {
		t.Fatalf("WaitForHarnessReady(fake Pi) error = %v", err)
	}
}

func TestRealTmuxWaitForHarnessReadyAllowsSlowProcessStart(t *testing.T) {
	socketPath := setupRealReadinessTmux(t)
	sessionName := "slow-ready-codex"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	fakeCodex := installFakeCodexProcess(t)
	command := fmt.Sprintf("sleep 4; printf '│ >_ OpenAI Codex (vtest) │\\n│ model: gpt-5.5 /model to change │\\n›\\n'; exec %q 10", fakeCodex)
	sendReadinessTestCommand(t, socketPath, sessionName, command)
	if err := NewRealTmux().WaitForHarnessReady(t.Context(), sessionName, "codex-cli", slowRealHostReadinessTimeout); err != nil {
		t.Fatalf("WaitForHarnessReady(slow fake Codex) error = %v", err)
	}
}

func TestRealTmuxReadinessPreservesClaudeGhostComposer(t *testing.T) {
	socketPath := setupRealReadinessTmux(t)
	sessionName := "ghost-ready-claude"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	fakeClaude := installFakeHarnessProcess(t, "claude")
	command := fmt.Sprintf("printf 'Do you want to proceed?\\n❯ 1. Allow\\n  2. Deny\\napproved\\n❯ \\033[2mstart the loop\\033[0m\\n────────────────\\n? for shortcuts\\n'; exec %q 10", fakeClaude)
	sendReadinessTestCommand(t, socketPath, sessionName, command)
	if err := NewRealTmux().WaitForHarnessReady(t.Context(), sessionName, "claude-code", realHostReadinessTimeout); err != nil {
		t.Fatalf("WaitForHarnessReady(styled Claude ghost) error = %v", err)
	}
}

func TestRealTmuxReadinessAdvancesGeminiTrustOnVerifiedPane(t *testing.T) {
	socketPath := setupRealReadinessTmux(t)
	sessionName := "trust-ready-gemini"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	fakeGemini := installInteractiveFakeGeminiProcess(t)
	sendReadinessTestCommand(t, socketPath, sessionName, fmt.Sprintf("exec %q", fakeGemini))
	// Process-table inspection can be heavily contended when the complete BDD
	// corpus and other repository test jobs share a macOS host. Keep the test
	// bounded while allowing the real readiness loop enough time to observe and
	// advance the trust prompt under that load.
	if err := NewRealTmux().WaitForHarnessReady(t.Context(), sessionName, "gemini-cli", 30*time.Second); err != nil {
		pane, captureErr := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", tmux.NormalizeTmuxSessionName(sessionName), "-p", "-S", "-30").CombinedOutput()
		t.Fatalf("WaitForHarnessReady(Gemini trust) error = %v; capture error = %v; pane:\n%s", err, captureErr, pane)
	}
}

func TestRealTmuxReadinessPinsLivenessAndDeliveryToActivePane(t *testing.T) {
	socketPath := setupRealReadinessTmux(t)
	sessionName := "multi-pane-claude"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	listPaneIDs := func() []string {
		t.Helper()
		out, err := exec.Command("tmux", "-S", socketPath, "list-panes", "-t", tmux.NormalizeTmuxSessionName(sessionName), "-F", "#{pane_id}").Output()
		if err != nil {
			t.Fatalf("list panes: %v", err)
		}
		return strings.Fields(string(out))
	}
	shellPane := listPaneIDs()[0]
	sendReadinessTestCommand(t, socketPath, sessionName, "printf '❯\\n────────────────\\n? for shortcuts\\n'; sleep 10")

	fakeClaude := installFakeHarnessProcess(t, "claude")
	out, err := exec.Command("tmux", "-S", socketPath, "split-window", "-t", tmux.NormalizeTmuxSessionName(sessionName), "-P", "-F", "#{pane_id}", "/bin/sh -i").Output()
	if err != nil {
		t.Fatalf("split Claude pane: %v", err)
	}
	claudePane := strings.TrimSpace(string(out))
	launch := fmt.Sprintf("printf '❯\\n────────────────\\n? for shortcuts\\n'; exec %q 10", fakeClaude)
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", claudePane, "-l", launch).CombinedOutput(); err != nil {
		t.Fatalf("queue fake Claude launch: %v\n%s", err, output)
	}
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", claudePane, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("launch fake Claude: %v\n%s", err, output)
	}
	time.Sleep(200 * time.Millisecond)

	if err := exec.Command("tmux", "-S", socketPath, "select-pane", "-t", shellPane).Run(); err != nil {
		t.Fatalf("select shell pane: %v", err)
	}
	realTmux := NewRealTmux()
	wrong, err := realTmux.CheckInputReadiness(t.Context(), sessionName, "claude-code")
	if err != nil {
		t.Fatalf("CheckInputReadiness(shell pane) error = %v", err)
	}
	if wrong.Ready || wrong.State != "WRONG_HARNESS" || wrong.PaneID != shellPane {
		t.Fatalf("shell pane readiness = %#v, want WRONG_HARNESS pinned to %s", wrong, shellPane)
	}

	if output, err := exec.Command("tmux", "-S", socketPath, "select-pane", "-t", claudePane).CombinedOutput(); err != nil {
		t.Fatalf("select Claude pane: %v\n%s\nall panes: %v", err, output, listPaneIDs())
	}
	ready, err := realTmux.SendKeysIfInputReady(t.Context(), sessionName, "claude-code", "pane-pinned-message", InputDeliveryOptions{})
	if err != nil {
		t.Fatalf("SendKeysIfInputReady(Claude pane) error = %v", err)
	}
	if !ready.Ready || ready.PaneID != claudePane {
		t.Fatalf("atomic Claude delivery = %#v, want ready pinned to %s", ready, claudePane)
	}
	if err := exec.Command("tmux", "-S", socketPath, "select-pane", "-t", shellPane).Run(); err != nil {
		t.Fatalf("reselect shell pane: %v", err)
	}

	capture := func(paneID string) string {
		t.Helper()
		out, err := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", paneID, "-p", "-S", "-20").Output()
		if err != nil {
			t.Fatalf("capture pane %s: %v", paneID, err)
		}
		return string(out)
	}
	if got := capture(claudePane); !strings.Contains(got, "pane-pinned-message") {
		t.Fatalf("verified Claude pane did not receive message:\n%s", got)
	}
	if got := capture(shellPane); strings.Contains(got, "pane-pinned-message") {
		t.Fatalf("focus-changed shell pane received verified message:\n%s", got)
	}
}

func TestRealTmuxReadinessIdentifiesNodeBackedCodex(t *testing.T) {
	socketPath := setupRealReadinessTmux(t)
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}
	writeNodeFixture := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create Node fixture directory: %v", err)
		}
		script := "process.stdout.write('│ >_ OpenAI Codex (vtest) │\\n│ model: gpt-5.5 /model to change │\\n›\\n'); setTimeout(() => {}, 10000);\n"
		if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
			t.Fatalf("write Node fixture: %v", err)
		}
	}

	codexSession := "node-ready-codex"
	if err := tmux.NewSession(codexSession, t.TempDir()); err != nil {
		t.Fatalf("NewSession(Codex) error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(codexSession) })
	codexScript := filepath.Join(t.TempDir(), "node_modules", "@openai", "codex", "bin", "codex.js")
	writeNodeFixture(codexScript)
	sendReadinessTestCommand(t, socketPath, codexSession, fmt.Sprintf("exec %q %q", nodePath, codexScript))
	if err := NewRealTmux().WaitForHarnessReady(t.Context(), codexSession, "codex-cli", realHostReadinessTimeout); err != nil {
		t.Fatalf("WaitForHarnessReady(Node-backed Codex) error = %v", err)
	}

	unrelatedSession := "node-unrelated-codex"
	if err := tmux.NewSession(unrelatedSession, t.TempDir()); err != nil {
		t.Fatalf("NewSession(unrelated Node) error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(unrelatedSession) })
	unrelatedScript := filepath.Join(t.TempDir(), "telemetry-worker.js")
	writeNodeFixture(unrelatedScript)
	sendReadinessTestCommand(t, socketPath, unrelatedSession, fmt.Sprintf("exec %q %q", nodePath, unrelatedScript))
	time.Sleep(200 * time.Millisecond)
	readiness, err := NewRealTmux().CheckInputReadiness(t.Context(), unrelatedSession, "codex-cli")
	if err != nil {
		t.Fatalf("CheckInputReadiness(unrelated Node) error = %v", err)
	}
	if readiness.Ready || readiness.State != "WRONG_HARNESS" {
		t.Fatalf("unrelated Node readiness = %#v, want WRONG_HARNESS", readiness)
	}
}

func TestRealTmuxReadinessRejectsSuspendedHarnessWithStaleComposer(t *testing.T) {
	socketPath := setupRealReadinessTmux(t)
	sessionName := "suspended-ready-claude"
	if err := tmux.NewSession(sessionName, t.TempDir()); err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(sessionName) })

	fakeClaude := installFakeHarnessProcess(t, "claude")
	command := fmt.Sprintf("PS1=''; export PS1; printf '❯\\n────────────────\\n? for shortcuts\\n'; %q", fakeClaude)
	sendReadinessTestCommand(t, socketPath, sessionName, command)
	realTmux := NewRealTmux()
	if err := realTmux.WaitForHarnessReady(t.Context(), sessionName, "claude-code", realHostReadinessTimeout); err != nil {
		t.Fatalf("WaitForHarnessReady(fake Claude) error = %v", err)
	}

	target := tmux.NormalizeTmuxSessionName(sessionName)
	if output, err := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", target, "C-z").CombinedOutput(); err != nil {
		t.Fatalf("suspend fake Claude: %v\n%s", err, output)
	}
	time.Sleep(300 * time.Millisecond)
	sendReadinessTestCommand(t, socketPath, sessionName, "printf '❯\\n────────────────\\n? for shortcuts\\n'")
	time.Sleep(200 * time.Millisecond)

	readiness, err := realTmux.CheckInputReadiness(t.Context(), sessionName, "claude-code")
	if err != nil {
		t.Fatalf("CheckInputReadiness(suspended Claude) error = %v", err)
	}
	if readiness.Ready || readiness.State != "WRONG_HARNESS" {
		pane, captureErr := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", target, "-p", "-S", "-30").CombinedOutput()
		t.Fatalf("suspended Claude readiness = %#v, want WRONG_HARNESS; capture error = %v; pane:\n%s", readiness, captureErr, pane)
	}
}

func TestRealTmuxInputReadinessReportsMissingSession(t *testing.T) {
	setupRealReadinessTmux(t)
	// NOT_FOUND is an absent target on a reachable tmux server. An absent
	// socket/server is an operational failure and must not satisfy this test.
	anchorSession := "missing-readiness-anchor"
	if err := tmux.NewSession(anchorSession, t.TempDir()); err != nil {
		t.Fatalf("start readiness anchor session: %v", err)
	}
	t.Cleanup(func() { tmux.KillSession(anchorSession) })

	readiness, err := NewRealTmux().CheckInputReadiness(context.Background(), "missing-readiness-session", "codex-cli")
	if err != nil {
		t.Fatalf("CheckInputReadiness() error = %v", err)
	}
	if readiness.Ready || readiness.State != "NOT_FOUND" {
		t.Fatalf("missing session readiness = %#v, want NOT_FOUND", readiness)
	}
}

func TestRealTmuxWaitForHarnessReadyRejectsUnknownHarness(t *testing.T) {
	err := NewRealTmux().WaitForHarnessReady(context.Background(), "unused", "unknown", time.Second)
	if err == nil {
		t.Fatal("WaitForHarnessReady() accepted an unknown harness")
	}
}

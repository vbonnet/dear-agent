package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/safepr"
	"golang.org/x/sys/unix"
)

// run is exercised only on paths that fail before any gh execution; the gh
// invocation itself is covered by internal/safepr unit tests plus the manual
// dogfood (this feature's own PR is created through safe-pr).
func TestRun_Errors(t *testing.T) {
	t.Setenv("WAYFINDER_PROJECT_DIR", "")
	cases := []struct {
		name    string
		argv    []string
		wantErr string
	}{
		{"wayfinder missing value", []string{"create", "--wayfinder"}, "--wayfinder requires"},
		{"bad timeout", []string{"create", "--timeout", "soon"}, "invalid --timeout"},
		{"no session", []string{"create", "--title", "t"}, "no wayfinder session"},
		{"session dir unreadable", []string{"create", "--wayfinder", "/nonexistent-wf-dir",
			"--title", "t"}, "cannot load"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.argv)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("run(%v) = %v, want substring %q", tc.argv, err, tc.wantErr)
			}
		})
	}
}

func TestRun_HelpIsNotAnError(t *testing.T) {
	for _, argv := range [][]string{nil, {"--help"}, {"create", "-h"}} {
		if err := run(argv); err != nil {
			t.Errorf("run(%v) = %v, want nil (help text)", argv, err)
		}
	}
}

func TestUsageDocumentsNonAGMFallback(t *testing.T) {
	for _, want := range []string{"agm escalate ask", "--session <registered-session>", "ask the current user directly"} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage missing %q", want)
		}
	}
}

func TestParsePRURL(t *testing.T) {
	repo, num, ok := parsePRURL("https://github.com/vbonnet/dear-agent/pull/582")
	if !ok || repo != "vbonnet/dear-agent" || num != "582" {
		t.Errorf("parsePRURL = (%q, %q, %v), want (vbonnet/dear-agent, 582, true)", repo, num, ok)
	}
	if _, _, ok := parsePRURL("not a url"); ok {
		t.Error("parsePRURL should fail on a non-PR string")
	}
	if _, _, ok := parsePRURL("https://github.com/vbonnet/dear-agent/issues/582"); ok {
		t.Error("parsePRURL should not match an issues URL")
	}
}

func TestParseArgs_VerifyCI(t *testing.T) {
	p, err := parseArgs([]string{"create", "--verify-ci", "--title", "t"})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if !p.verifyCI {
		t.Error("--verify-ci should set verifyCI true")
	}
	// --verify-ci must not leak into the gh args passed through to gh pr create.
	for _, a := range p.req.GhArgs {
		if a == "--verify-ci" {
			t.Error("--verify-ci leaked into gh args")
		}
	}
}

func TestExecGh_CreateNeverMutatesMergeState(t *testing.T) {
	const (
		headOID = "0123456789abcdef0123456789abcdef01234567"
		prURL   = "https://github.com/vbonnet/dear-agent/pull/123"
	)
	tests := []struct {
		name      string
		draft     bool
		verifyCI  bool
		wantCalls []string
	}{
		{
			name:      "non-draft without CI discovery",
			wantCalls: []string{"pr:create fd=present trace=present auto=absent"},
		},
		{
			name:      "draft without CI discovery",
			draft:     true,
			wantCalls: []string{"pr:create fd=present trace=present auto=absent"},
		},
		{
			name:     "non-draft with positive CI discovery",
			verifyCI: true,
			wantCalls: []string{
				"pr:create fd=present trace=present auto=absent",
				"pr:view fd=present trace=absent auto=absent",
				"api:repos/vbonnet/dear-agent/commits/" + headOID + "/check-runs fd=present trace=absent auto=absent",
			},
		},
		{
			name:      "draft skips CI discovery",
			draft:     true,
			verifyCI:  true,
			wantCalls: []string{"pr:create fd=present trace=present auto=absent"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			binDir := t.TempDir()
			logPath := filepath.Join(t.TempDir(), "gh-calls.log")
			fakeGH := filepath.Join(binDir, "gh")
			script := fmt.Sprintf(`#!/bin/sh
set -eu

fd=absent
if [ -e /dev/fd/3 ]; then
  fd=present
fi
trace=absent
case "$*" in
  *"Wayfinder-Session: matrix-session"*) trace=present ;;
esac
auto=absent
case " $* " in
  *" --auto "*) auto=present ;;
esac
printf '%%s:%%s fd=%%s trace=%%s auto=%%s\n' "$1" "${2-}" "$fd" "$trace" "$auto" >> "$SAFE_PR_TEST_GH_LOG"

case "$1:$2" in
  pr:create)
    printf '%%s\n' %q
    ;;
  pr:view)
    printf '%%s\n' %q
    ;;
  api:repos/vbonnet/dear-agent/commits/%s/check-runs)
    printf '1\n'
    ;;
  *)
    printf 'unexpected gh invocation: %%s\n' "$*" >&2
    exit 2
    ;;
esac
`, prURL, headOID, headOID)
			if err := os.WriteFile(fakeGH, []byte(script), 0o700); err != nil {
				t.Fatalf("write fake gh: %v", err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("SAFE_PR_TEST_GH_LOG", logPath)

			args := []string{"--title", "matrix", "--body", "body"}
			if tc.draft {
				args = append(args, "--draft")
			}
			req := &safepr.Request{
				Verb:    "create",
				Session: &safepr.Session{ID: "matrix-session", ProjectPath: t.TempDir()},
				GhArgs:  args,
			}

			transactionWorktree := newSafePRTransactionWorktree(t)
			var (
				outcome githubExecution
				err     error
			)
			stderr := captureSafePRStderr(t, func() {
				err = safepr.WithWorktreeTransaction(transactionWorktree, "safe-pr create subprocess matrix", func(transaction *safepr.WorktreeTransaction) error {
					var executeErr error
					// The production timeout is intentionally short to fail closed on
					// interactive gh prompts. This real-subprocess regression instead
					// needs scheduler headroom under -race so it tests the command
					// matrix, not host load.
					outcome, executeErr = execGh(context.Background(), req, 5*time.Second, tc.verifyCI, transaction)
					return executeErr
				})
			})
			if err != nil {
				t.Fatalf("execGh() error = %v", err)
			}
			if outcome.prURL != prURL || outcome.exitCode != 0 {
				t.Fatalf("execGh() outcome = %#v, want successful %q", outcome, prURL)
			}
			if stderr != "" {
				t.Fatalf("positive CI discovery wrote stderr: %q", stderr)
			}

			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("read fake gh log: %v", err)
			}
			gotCalls := strings.Split(strings.TrimSpace(string(data)), "\n")
			if !slices.Equal(gotCalls, tc.wantCalls) {
				t.Fatalf("gh calls = %q, want %q", string(data), tc.wantCalls)
			}
			if strings.Contains(string(data), "merge") || strings.Contains(string(data), "auto=present") {
				t.Fatalf("safe-pr create invoked a merge mutation: %q", string(data))
			}
		})
	}
}

func captureSafePRStderr(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = writer
	restored := false
	restore := func() {
		if !restored {
			os.Stderr = original
			restored = true
		}
	}
	t.Cleanup(func() {
		restore()
		_ = writer.Close()
		_ = reader.Close()
	})
	type readResult struct {
		data []byte
		err  error
	}
	readDone := make(chan readResult, 1)
	go func() {
		data, readErr := io.ReadAll(reader)
		readDone <- readResult{data: data, err: readErr}
	}()

	fn()
	restore()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	result := <-readDone
	if result.err != nil {
		t.Fatalf("read stderr pipe: %v", result.err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return string(result.data)
}

func TestCaptureSafePRStderrDrainsConcurrently(t *testing.T) {
	payload := strings.Repeat("x", 128*1024)
	var writeErr error
	stderr := captureSafePRStderr(t, func() {
		_, writeErr = io.WriteString(os.Stderr, payload)
	})
	if writeErr != nil {
		t.Fatalf("write captured stderr: %v", writeErr)
	}
	if stderr != payload {
		t.Fatalf("captured stderr bytes = %d, want %d", len(stderr), len(payload))
	}
}

func TestWarnIfNoCIStopsBeforeProviderLookupWhenCanceled(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh-invoked")
	fakeGH := filepath.Join(binDir, "gh")
	script := "#!/bin/sh\nprintf invoked > \"$SAFE_PR_TEST_GH_LOG\"\n"
	if err := os.WriteFile(fakeGH, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SAFE_PR_TEST_GH_LOG", logPath)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	warnIfNoCI(ctx, "https://github.com/vbonnet/dear-agent/pull/1173", nil)
	if data, err := os.ReadFile(logPath); err == nil {
		t.Fatalf("canceled CI verification invoked gh: %q", data)
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect fake gh log: %v", err)
	}
}

func TestRequestsDraft(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{args: []string{"--draft"}, want: true},
		{args: []string{"-d"}, want: true},
		{args: []string{"--draft=true"}, want: true},
		{args: []string{"-d=true"}, want: true},
		{args: []string{"--draft=false"}, want: false},
		{args: []string{"-d=false"}, want: false},
		{args: []string{"--draft", "--draft=false"}, want: false},
		{args: []string{"-d", "--draft=false"}, want: false},
		{args: []string{"--draft=false", "--draft"}, want: true},
		{args: []string{"--draft=false", "-d"}, want: true},
		{args: []string{"-dt", "title"}, want: true},
		{args: []string{"-fd"}, want: true},
		{args: []string{"-fd=false"}, want: false},
		{args: []string{"-fd=true"}, want: true},
		{args: []string{"-df=false"}, want: true},
		{args: []string{"-dt", "title", "--draft=false"}, want: false},
		{args: []string{"-td"}, want: false},
		{args: []string{"-Rfoo/docs"}, want: false},
		{args: []string{"-R", "foo/docs"}, want: false},
		{args: []string{"-Rd"}, want: false},
		{args: []string{"-dRfoo/docs"}, want: true},
		{args: []string{"--draft", "--title", "--draft=false", "--body", "body"}, want: true},
		{args: []string{"--draft", "-t", "--draft=false", "--body", "body"}, want: true},
		{args: []string{"--title", "draft"}, want: false},
	}
	for _, tc := range tests {
		if got := requestsDraft(tc.args); got != tc.want {
			t.Errorf("requestsDraft(%v) = %t, want %t", tc.args, got, tc.want)
		}
	}
}

func TestShortSHA(t *testing.T) {
	if got := shortSHA("cebb82eb05bea83"); got != "cebb82e" {
		t.Errorf("shortSHA = %q, want cebb82e", got)
	}
	if got := shortSHA("abc"); got != "abc" {
		t.Errorf("shortSHA = %q, want abc", got)
	}
}

// initGitRepo creates a minimal git repo in dir with the given origin remote URL.
func initGitRepo(t *testing.T, dir, remoteURL string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", dir},
		{"remote", "add", "origin", remoteURL},
	} {
		if out, err := gittest.Command(t, dir, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func newSafePRTransactionWorktree(t *testing.T) string {
	t.Helper()
	sandbox := gittest.New(t)
	repo := sandbox.NewRepo(t)
	worktree := filepath.Join(t.TempDir(), "worktree")
	sandbox.Run(t, repo, "worktree", "add", "-q", "-b", "matrix", worktree)
	return worktree
}

// writeWayfinderStatus creates a complete canonical WAYFINDER-STATUS.md in dir.
func writeWayfinderStatus(t *testing.T, dir, projectName, status string) {
	t.Helper()
	content := fmt.Sprintf(`---
schema_version: "2.0"
project_name: %s
project_type: feature
risk_level: S
current_waypoint: CHARTER
status: %s
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:00:00Z
---
`, projectName, status)
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("writeWayfinderStatus: %v", err)
	}
}

func bypassCreateWorktreeProtection(t *testing.T) {
	t.Helper()
	original := protectCreateWorktree
	t.Cleanup(func() { protectCreateWorktree = original })
	protectCreateWorktree = func(_ string, _ string, action func(*safepr.WorktreeTransaction) error) error {
		return action(nil)
	}
}

func captureSafePRAudits(t *testing.T) *[]safepr.AuditRecord {
	t.Helper()
	original := appendSafePRAudit
	records := make([]safepr.AuditRecord, 0, 1)
	t.Cleanup(func() { appendSafePRAudit = original })
	appendSafePRAudit = func(_ string, record safepr.AuditRecord) error {
		records = append(records, record)
		return nil
	}
	return &records
}

func TestParseArgs_SkipPreflight(t *testing.T) {
	p, err := parseArgs([]string{"create", "--skip-preflight", "--title", "t"})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if !p.skipPreflight {
		t.Error("--skip-preflight should set skipPreflight true")
	}
	for _, a := range p.req.GhArgs {
		if a == "--skip-preflight" {
			t.Error("--skip-preflight leaked into gh args")
		}
	}
}

func TestValidateRemoteURL(t *testing.T) {
	cases := []struct {
		name      string
		remoteURL string
		wantErr   string
	}{
		{
			name:      "valid vbonnet remote",
			remoteURL: "https://github.com/vbonnet/dear-agent",
		},
		{
			name:      "wrong org",
			remoteURL: "https://github.com/dear-labs/dear-agent",
			wantErr:   "does not look like a vbonnet GitHub remote",
		},
		{
			name:      "non-github remote",
			remoteURL: "https://gitlab.com/vbonnet/dear-agent",
			wantErr:   "does not look like a vbonnet GitHub remote",
		},
		{
			name:      "ssh remote",
			remoteURL: "git@github.com:vbonnet/dear-agent.git",
			wantErr:   "does not look like a vbonnet GitHub remote",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initGitRepo(t, dir, tc.remoteURL)
			err := validateRemoteURL(dir)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("validateRemoteURL(%q) = %v, want nil", tc.remoteURL, err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("validateRemoteURL(%q) = %v, want substring %q", tc.remoteURL, err, tc.wantErr)
				}
			}
		})
	}
}

func TestValidateRemoteURL_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	err := validateRemoteURL(dir)
	if err == nil || !strings.Contains(err.Error(), "could not resolve origin remote URL") {
		t.Errorf("validateRemoteURL(non-git dir) = %v, want 'could not resolve origin remote URL'", err)
	}
}

func TestPreflightTimeoutCoversRepositoryFullGate(t *testing.T) {
	if preflightTimeout < 60*time.Minute {
		t.Fatalf("preflightTimeout = %s, want at least 60m", preflightTimeout)
	}
}

func TestProtectTransactionCommandIsGroupCancelable(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "true")
	if err := protectTransactionCommand(nil, cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("protected child must run in an isolated process group")
	}
	if cmd.Cancel == nil {
		t.Fatal("protected child must cancel its process group")
	}
	if cmd.WaitDelay != time.Second {
		t.Fatalf("protected child WaitDelay = %v, want %v", cmd.WaitDelay, time.Second)
	}
	if err := cmd.Cancel(); err != nil {
		t.Fatalf("cancel before start = %v", err)
	}
}

func TestCloseOnExecMarksDescriptor(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "transaction-guard")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := closeOnExec(int(file.Fd())); err != nil {
		t.Fatal(err)
	}
	flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
	if err != nil {
		t.Fatal(err)
	}
	if flags&unix.FD_CLOEXEC == 0 {
		t.Fatal("transaction guard descriptor is inheritable after closeOnExec")
	}
}

func TestPreflightGuardDirectory(t *testing.T) {
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	got, err := preflightGuardDirectory(currentDir)
	if err != nil {
		t.Fatalf("preflightGuardDirectory(%q): %v", currentDir, err)
	}
	if got != currentDir {
		t.Fatalf("preflightGuardDirectory(%q) = %q, want %q", currentDir, got, currentDir)
	}
	if _, err := preflightGuardDirectory(filepath.Join(t.TempDir(), "other")); err == nil || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("preflightGuardDirectory(unrelated) = %v, want working-directory error", err)
	}
}

func TestRun_PreflightFail_BlocksPRCreate(t *testing.T) {
	bypassCreateWorktreeProtection(t)
	captureSafePRAudits(t)
	t.Setenv("WAYFINDER_PROJECT_DIR", "")
	dir := t.TempDir()
	writeWayfinderStatus(t, dir, "test-session", "in-progress")

	orig := runPreflightFull
	t.Cleanup(func() { runPreflightFull = orig })
	runPreflightFull = func(string, *safepr.WorktreeTransaction) error {
		return fmt.Errorf("preflight-full failed — fix issues before creating PR")
	}

	err := run([]string{"create", "--wayfinder", dir, "--title", "t"})
	if err == nil || !strings.Contains(err.Error(), "preflight-full failed") {
		t.Errorf("run() = %v, want preflight-full failed error", err)
	}
}

func TestRun_PreflightPass_ProceedsToPRCreate(t *testing.T) {
	bypassCreateWorktreeProtection(t)
	captureSafePRAudits(t)
	t.Setenv("WAYFINDER_PROJECT_DIR", "")
	dir := t.TempDir()
	writeWayfinderStatus(t, dir, "test-session", "in-progress")

	orig := runPreflightFull
	t.Cleanup(func() { runPreflightFull = orig })
	runPreflightFull = func(string, *safepr.WorktreeTransaction) error { return nil }
	ghCalled := false
	origGitHub := executeGitHub
	t.Cleanup(func() { executeGitHub = origGitHub })
	executeGitHub = func(context.Context, *safepr.Request, time.Duration, bool, *safepr.WorktreeTransaction) (githubExecution, error) {
		ghCalled = true
		return githubExecution{}, nil
	}

	if err := run([]string{"create", "--wayfinder", dir, "--title", "t"}); err != nil {
		t.Fatalf("run() failed despite passing preflight: %v", err)
	}
	if !ghCalled {
		t.Error("GitHub boundary was not called after preflight passed")
	}
}

func TestRun_SkipPreflight_NoPreflightRun(t *testing.T) {
	bypassCreateWorktreeProtection(t)
	captureSafePRAudits(t)
	t.Setenv("WAYFINDER_PROJECT_DIR", "")
	dir := t.TempDir()
	writeWayfinderStatus(t, dir, "test-session", "in-progress")

	preflightCalled := false
	orig := runPreflightFull
	t.Cleanup(func() { runPreflightFull = orig })
	// Mock would fail if called — proves --skip-preflight prevents the call.
	runPreflightFull = func(string, *safepr.WorktreeTransaction) error {
		preflightCalled = true
		return fmt.Errorf("preflight-full failed — should not have been called")
	}
	ghCalled := false
	origGitHub := executeGitHub
	t.Cleanup(func() { executeGitHub = origGitHub })
	executeGitHub = func(context.Context, *safepr.Request, time.Duration, bool, *safepr.WorktreeTransaction) (githubExecution, error) {
		ghCalled = true
		return githubExecution{}, nil
	}

	if err := run([]string{"create", "--wayfinder", dir, "--skip-preflight", "--title", "t"}); err != nil {
		t.Fatalf("run() failed despite --skip-preflight: %v", err)
	}
	if preflightCalled {
		t.Error("preflight was called despite --skip-preflight")
	}
	if !ghCalled {
		t.Error("GitHub boundary was not called with --skip-preflight")
	}
}

func TestRun_CreateProtectsPreflightAndGitHubMutation(t *testing.T) {
	captureSafePRAudits(t)
	t.Setenv("WAYFINDER_PROJECT_DIR", "")
	dir := t.TempDir()
	writeWayfinderStatus(t, dir, "test-session", "in-progress")

	originalProtect := protectCreateWorktree
	originalPreflight := runPreflightFull
	originalGitHub := executeGitHub
	t.Cleanup(func() {
		protectCreateWorktree = originalProtect
		runPreflightFull = originalPreflight
		executeGitHub = originalGitHub
	})

	events := make([]string, 0, 4)
	protected := false
	protectCreateWorktree = func(_ string, _ string, action func(*safepr.WorktreeTransaction) error) error {
		events = append(events, "protect-start")
		protected = true
		err := action(nil)
		protected = false
		events = append(events, "protect-end")
		return err
	}
	runPreflightFull = func(string, *safepr.WorktreeTransaction) error {
		if !protected {
			t.Fatal("preflight ran outside worktree protection")
		}
		events = append(events, "preflight")
		return nil
	}
	executeGitHub = func(context.Context, *safepr.Request, time.Duration, bool, *safepr.WorktreeTransaction) (githubExecution, error) {
		if !protected {
			t.Fatal("GitHub mutation ran outside worktree protection")
		}
		events = append(events, "github")
		return githubExecution{}, nil
	}

	if err := run([]string{"create", "--wayfinder", dir, "--title", "t"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"protect-start", "preflight", "github", "protect-end"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("transaction events = %v, want %v", events, want)
	}
}

func TestRun_CreateAuditsFinalTransactionOutcome(t *testing.T) {
	tests := []struct {
		name      string
		runAction bool
		finalErr  string
		wantExit  int
		wantPRURL string
	}{
		{name: "success", runAction: true, wantPRURL: "https://github.com/vbonnet/dear-agent/pull/968"},
		{name: "acquisition failure", finalErr: "acquire transaction guard", wantExit: 1},
		{name: "release failure", runAction: true, finalErr: "release owned worktree lock", wantExit: 1, wantPRURL: "https://github.com/vbonnet/dear-agent/pull/968"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WAYFINDER_PROJECT_DIR", "")
			dir := t.TempDir()
			writeWayfinderStatus(t, dir, "audit-session", "in-progress")
			records := captureSafePRAudits(t)

			originalProtect := protectCreateWorktree
			originalGitHub := executeGitHub
			t.Cleanup(func() {
				protectCreateWorktree = originalProtect
				executeGitHub = originalGitHub
			})

			githubCalled := false
			protectCreateWorktree = func(_ string, _ string, action func(*safepr.WorktreeTransaction) error) error {
				if !tc.runAction {
					return fmt.Errorf("%s", tc.finalErr)
				}
				if err := action(nil); err != nil {
					return err
				}
				if tc.finalErr != "" {
					return fmt.Errorf("%s", tc.finalErr)
				}
				return nil
			}
			executeGitHub = func(context.Context, *safepr.Request, time.Duration, bool, *safepr.WorktreeTransaction) (githubExecution, error) {
				githubCalled = true
				return githubExecution{prURL: tc.wantPRURL}, nil
			}

			err := run([]string{"create", "--wayfinder", dir, "--skip-preflight", "--title", "audit"})
			if tc.finalErr == "" && err != nil {
				t.Fatalf("run() = %v, want nil", err)
			}
			if tc.finalErr != "" && (err == nil || !strings.Contains(err.Error(), tc.finalErr)) {
				t.Fatalf("run() = %v, want error containing %q", err, tc.finalErr)
			}
			if githubCalled != tc.runAction {
				t.Fatalf("GitHub called = %t, want %t", githubCalled, tc.runAction)
			}
			if len(*records) != 1 {
				t.Fatalf("audit records = %d, want exactly 1", len(*records))
			}
			record := (*records)[0]
			if record.ExitCode != tc.wantExit || record.PRURL != tc.wantPRURL {
				t.Fatalf("audit outcome = exit:%d url:%q, want exit:%d url:%q", record.ExitCode, record.PRURL, tc.wantExit, tc.wantPRURL)
			}
			if tc.finalErr == "" && record.Error != "" {
				t.Fatalf("successful audit error = %q, want empty", record.Error)
			}
			if tc.finalErr != "" && !strings.Contains(record.Error, tc.finalErr) {
				t.Fatalf("audit error = %q, want substring %q", record.Error, tc.finalErr)
			}
		})
	}
}

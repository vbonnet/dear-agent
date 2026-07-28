package codexarchive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArchiveSkipsNonCodexHarness(t *testing.T) {
	result, err := Archive(context.Background(), Request{Harness: "claude-code", Name: "worker"})
	if err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}
	if result == nil || !result.Skipped {
		t.Fatalf("expected skipped result, got %#v", result)
	}
}

func TestArchiveUsesPersistedCodexSessionID(t *testing.T) {
	origRemote := runCodexRemoteArchiveFn
	origLocal := runCodexLocalArchiveFn
	t.Cleanup(func() {
		runCodexRemoteArchiveFn = origRemote
		runCodexLocalArchiveFn = origLocal
	})

	var got string
	runCodexRemoteArchiveFn = func(_ context.Context, threadID string) error {
		got = threadID
		return nil
	}
	runCodexLocalArchiveFn = func(context.Context, string) error {
		t.Fatal("local fallback must not run after remote archive succeeds")
		return nil
	}

	res, err := Archive(context.Background(), Request{
		Harness:        "codex-cli",
		Name:           "agm-name",
		AGMSessionID:   "agm-session-id",
		CodexSessionID: "thr_123",
	})
	if err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}
	if got != "thr_123" {
		t.Fatalf("archived Codex id = %q, want thr_123", got)
	}
	if res.Target != "thr_123" {
		t.Fatalf("result target = %q, want thr_123", res.Target)
	}
}

func TestArchivePersistedCodexSessionFallsBackToLocalSavedSession(t *testing.T) {
	origRemote := runCodexRemoteArchiveFn
	origLocal := runCodexLocalArchiveFn
	t.Cleanup(func() {
		runCodexRemoteArchiveFn = origRemote
		runCodexLocalArchiveFn = origLocal
	})

	runCodexRemoteArchiveFn = func(_ context.Context, threadID string) error {
		if threadID != "local-import" {
			t.Fatalf("remote archive target = %q, want local-import", threadID)
		}
		return errors.New("remote control unavailable")
	}
	var localTarget string
	runCodexLocalArchiveFn = func(_ context.Context, threadID string) error {
		localTarget = threadID
		return nil
	}

	result, err := Archive(context.Background(), Request{
		Harness:        "codex-cli",
		CodexSessionID: "local-import",
	})
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if localTarget != "local-import" {
		t.Fatalf("local archive target = %q, want local-import", localTarget)
	}
	if result.Target != "local-import" {
		t.Fatalf("result target = %q, want local-import", result.Target)
	}
}

func TestArchivePersistedCodexSessionReportsBothArchiveFailures(t *testing.T) {
	origRemote := runCodexRemoteArchiveFn
	origLocal := runCodexLocalArchiveFn
	t.Cleanup(func() {
		runCodexRemoteArchiveFn = origRemote
		runCodexLocalArchiveFn = origLocal
	})

	remoteErr := errors.New("remote control unavailable")
	localErr := errors.New("saved session unavailable")
	runCodexRemoteArchiveFn = func(context.Context, string) error { return remoteErr }
	runCodexLocalArchiveFn = func(context.Context, string) error { return localErr }

	_, err := Archive(context.Background(), Request{
		Harness:        "codex-cli",
		CodexSessionID: "missing-session",
	})
	if !errors.Is(err, localErr) {
		t.Fatalf("Archive() error = %v, want local error", err)
	}
	if !errors.Is(err, remoteErr) {
		t.Fatalf("Archive() error = %v, want remote error", err)
	}
	if !strings.Contains(err.Error(), remoteErr.Error()) {
		t.Fatalf("Archive() error = %v, want remote failure context", err)
	}
}

func TestArchivePersistedCodexSessionUsesUnixRemote(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", fakeCodexPath(t, home))

	result, err := Archive(context.Background(), Request{
		Harness:        "codex-cli",
		CodexSessionID: "thread-456",
	})
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	if result.Target != "thread-456" {
		t.Fatalf("target = %q, want thread-456", result.Target)
	}

	args := strings.TrimSpace(readFakeCodexArgs(t, home))
	want := "archive --remote unix:// thread-456"
	if args != want {
		t.Fatalf("codex args = %q, want %q", args, want)
	}
}

func TestArchivePersistedCodexSessionLocalFallbackIgnoresRemoteOverride(t *testing.T) {
	origRemote := runCodexRemoteArchiveFn
	t.Cleanup(func() { runCodexRemoteArchiveFn = origRemote })

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", fakeCodexPath(t, home))
	t.Setenv(envRemote, "unix:///custom/remote.sock")
	runCodexRemoteArchiveFn = func(context.Context, string) error {
		return errors.New("remote control unavailable")
	}

	_, err := Archive(context.Background(), Request{
		Harness:        "codex-cli",
		CodexSessionID: "local-import",
	})
	if err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	args := strings.TrimSpace(readFakeCodexArgs(t, home))
	if args != "archive local-import" {
		t.Fatalf("local fallback args = %q, want archive without --remote", args)
	}
}

func TestArchivePreservesCallerContextError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", fakeCodexPath(t, home))
	origLocal := runCodexLocalArchiveFn
	t.Cleanup(func() { runCodexLocalArchiveFn = origLocal })
	runCodexLocalArchiveFn = func(context.Context, string) error {
		t.Fatal("local fallback must not run after caller cancellation")
		return nil
	}

	tests := []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		want    error
	}{
		{
			name: "canceled",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
			want: context.Canceled,
		},
		{
			name: "caller deadline",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			},
			want: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.context()
			defer cancel()

			_, err := Archive(ctx, Request{
				Harness:        "codex-cli",
				CodexSessionID: "thread-canceled",
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("Archive() error = %v, want %v", err, tt.want)
			}
			if strings.Contains(err.Error(), "timed out after") {
				t.Fatalf("Archive() mislabeled caller context error as helper timeout: %v", err)
			}
		})
	}
}

func TestArchiveResolvesCodexSessionByWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", fakeCodexPath(t, home))

	cwd := filepath.Join(home, "work", "merged")
	writeCodexTranscript(t, home, "sessions/2026/06/24/rollout-2026-06-24T01-02-03-019efabc.jsonl", "019efabc", cwd)

	result, err := Archive(context.Background(), Request{
		Harness:          "codex-cli",
		Name:             "agm-session-name",
		AGMSessionID:     "agm-session-id",
		WorkingDirectory: cwd,
	})
	if err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}
	if result.Target != "019efabc" {
		t.Fatalf("target = %q, want Codex session id", result.Target)
	}

	args := readFakeCodexArgs(t, home)
	if strings.TrimSpace(args) != "archive 019efabc" {
		t.Fatalf("codex args = %q", args)
	}
}

func TestArchiveUsesRemoteEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", fakeCodexPath(t, home))
	t.Setenv(envRemote, "unix://")
	t.Setenv(envRemoteToken, "CODEX_REMOTE_TOKEN")

	cwd := filepath.Join(home, "work")
	writeCodexTranscript(t, home, "sessions/2026/06/24/rollout-remote.jsonl", "019e-remote", cwd)

	_, err := Archive(context.Background(), Request{
		Harness:          "codex-cli",
		WorkingDirectory: cwd,
	})
	if err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}

	args := strings.TrimSpace(readFakeCodexArgs(t, home))
	want := "archive --remote unix:// --remote-auth-token-env CODEX_REMOTE_TOKEN 019e-remote"
	if args != want {
		t.Fatalf("codex args = %q, want %q", args, want)
	}
}

func TestArchiveTreatsArchivedTranscriptAsSuccess(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", fakeCodexPath(t, home))

	cwd := filepath.Join(home, "archived-work")
	writeCodexTranscript(t, home, "archived_sessions/rollout-archived.jsonl", "019e-archived", cwd)

	result, err := Archive(context.Background(), Request{
		Harness:          "codex-cli",
		WorkingDirectory: cwd,
	})
	if err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}
	if !result.AlreadyArchived {
		t.Fatalf("expected already archived result, got %#v", result)
	}
	if _, err := os.Stat(filepath.Join(home, "codex-args.txt")); !os.IsNotExist(err) {
		t.Fatalf("codex command should not run for already archived transcript")
	}
}

func TestArchiveErrorsWhenCodexSessionCannotBeResolved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", fakeCodexPath(t, home))

	_, err := Archive(context.Background(), Request{
		Harness:          "codex-cli",
		WorkingDirectory: filepath.Join(home, "missing"),
	})
	if err == nil {
		t.Fatal("expected resolve error")
	}
}

func fakeCodexPath(t *testing.T, home string) string {
	t.Helper()
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	script := filepath.Join(binDir, "codex")
	body := "#!/bin/sh\nprintf '%s' \"$*\" > \"$HOME/codex-args.txt\"\n"
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}
	return binDir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func writeCodexTranscript(t *testing.T, home, relPath, id, cwd string) {
	t.Helper()
	path := filepath.Join(home, ".codex", relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	line := `{"type":"session_meta","payload":{"session_id":"` + id + `","id":"` + id + `","cwd":"` + cwd + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
}

func readFakeCodexArgs(t *testing.T, home string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, "codex-args.txt"))
	if err != nil {
		t.Fatalf("read fake codex args: %v", err)
	}
	return string(data)
}

package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestCollectExtraAddDirsScopesWritableDirsToCodex(t *testing.T) {
	originalCfg := cfg
	originalHarness := harnessName
	originalRole := roleName
	t.Cleanup(func() {
		cfg = originalCfg
		harnessName = originalHarness
		roleName = originalRole
	})
	cfg = &config.Config{Sandbox: config.SandboxConfig{
		Repos:        []string{"/source"},
		WritableDirs: []string{"/worktrees", "/beads"},
	}}
	roleName = "worker"

	for _, test := range []struct {
		harness string
		want    []string
	}{
		{harness: "codex-cli", want: []string{"/worktrees", "/beads"}},
		{harness: "claude-code", want: []string{"/source"}},
		{harness: "agy", want: []string{"/source"}},
	} {
		t.Run(test.harness, func(t *testing.T) {
			harnessName = test.harness
			got, _ := collectExtraAddDirs(&manifest.SandboxConfig{Enabled: true}, nil)
			if !slices.Equal(got, test.want) {
				t.Fatalf("collectExtraAddDirs() = %v, want %v", got, test.want)
			}
		})
	}

	got := collectExtraAddDirsForHarness(&manifest.SandboxConfig{Enabled: true}, "codex-cli", "worker", []string{"/worktrees"})
	if !slices.Equal(got, []string{"/worktrees", "/beads"}) {
		t.Fatalf("collectExtraAddDirsForHarness() = %v", got)
	}

	if got := collectExtraAddDirsForHarness(&manifest.SandboxConfig{Enabled: true}, "codex-cli", "reviewer", nil); !slices.Equal(got, []string{"/source"}) {
		t.Fatalf("non-worker writable dirs = %v", got)
	}

	if got := collectExtraAddDirsForHarness(nil, "codex-cli", "worker", []string{"/prepared/worktree"}); !slices.Equal(got, []string{"/prepared/worktree"}) {
		t.Fatalf("trusted dirs without AGM sandbox = %v", got)
	}
}

func TestTrustedAddDirsForSessionConsumesWorkerBoundHandoff(t *testing.T) {
	dir := t.TempDir()
	guard := filepath.Join(t.TempDir(), "pretool-worker-write-boundary")
	if err := os.WriteFile(guard, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(trustedAddDirsEnv, `["`+dir+`","`+dir+`"]`)
	t.Setenv(trustedAddDirsSessionEnv, "worker-ce-test")
	t.Setenv(trustedGuardPathEnv, guard)

	got, gotGuard, err := trustedAddDirsForSession("worker-ce-test", "worker")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{dir}) {
		t.Fatalf("trustedAddDirsForSession() = %v", got)
	}
	if gotGuard != guard {
		t.Fatalf("trusted guard path = %q, want %q", gotGuard, guard)
	}
	if os.Getenv(trustedAddDirsEnv) != "" || os.Getenv(trustedAddDirsSessionEnv) != "" || os.Getenv(trustedGuardPathEnv) != "" {
		t.Fatal("trusted worker handoff leaked into the future harness environment")
	}
}

func TestTrustedAddDirsForSessionRejectsWrongRoleOrSession(t *testing.T) {
	for _, test := range []struct {
		name    string
		bound   string
		session string
		role    string
	}{
		{name: "wrong role", bound: "worker-ce-test", session: "worker-ce-test", role: "reviewer"},
		{name: "wrong session", bound: "worker-other", session: "worker-ce-test", role: "worker"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(trustedAddDirsEnv, `["`+t.TempDir()+`"]`)
			t.Setenv(trustedAddDirsSessionEnv, test.bound)
			t.Setenv(trustedGuardPathEnv, filepath.Join(t.TempDir(), "missing-guard"))
			if _, _, err := trustedAddDirsForSession(test.session, test.role); err == nil {
				t.Fatal("trustedAddDirsForSession() unexpectedly accepted untrusted handoff")
			}
		})
	}
}

func TestConfigureWorkerWriteBoundaryExportsValidatedCodexPolicy(t *testing.T) {
	guard := filepath.Join(t.TempDir(), "pretool-worker-write-boundary")
	if err := os.WriteFile(guard, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv(workerWriteRootsEnv, "stale")
	if err := configureWorkerWriteBoundary("codex-cli", "worker", guard, []string{dir}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(workerWriteRootsEnv); got != `["`+dir+`"]` {
		t.Fatalf("worker write roots = %q", got)
	}
}

func TestConfigureWorkerWriteBoundaryIsInactiveForNonWorkers(t *testing.T) {
	t.Setenv(workerWriteRootsEnv, "stale")
	if err := configureWorkerWriteBoundary("codex-cli", "reviewer", "", nil); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(workerWriteRootsEnv) != "" {
		t.Fatal("non-worker retained worker write-boundary environment")
	}
}

func TestConfigureWorkerWriteBoundaryRejectsEmptyWorkerRoots(t *testing.T) {
	guard := filepath.Join(t.TempDir(), "pretool-worker-write-boundary")
	if err := os.WriteFile(guard, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := configureWorkerWriteBoundary("codex-cli", "worker", guard, nil); err == nil {
		t.Fatal("Codex worker launch accepted no authorized write roots")
	}
}

func TestPrepareCodexHookTrustBypassRequiresExplicitReviewedRepo(t *testing.T) {
	originalCfg := cfg
	originalHarness := harnessName
	t.Cleanup(func() {
		cfg = originalCfg
		harnessName = originalHarness
	})
	cfg = &config.Config{Sandbox: config.SandboxConfig{
		BypassCodexHookTrustReason: "sandbox path rotates per spawn so hooks cannot be pre-trusted",
	}}
	harnessName = "codex-cli"

	enabled, err := prepareCodexHookTrustBypass(t.Context(), &manifest.SandboxConfig{
		Enabled:             true,
		CodexHookSourceRepo: "/dynamic/unreviewed",
	})
	if err == nil || !strings.Contains(err.Error(), "requires an explicit sandbox.repos source") {
		t.Fatalf("prepareCodexHookTrustBypass() = %v, %v; want explicit-source rejection", enabled, err)
	}
}

func TestResolveCreateLifecyclePromptLoadsAgyPromptFileBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(path, []byte("prompt from file\nwith another line"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveCreateLifecyclePrompt("agy", "", path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "prompt from file\nwith another line" {
		t.Fatalf("resolved prompt = %q", got)
	}
}

func TestResolveCreateLifecyclePromptRejectsUnreadableAndOversizedAgyFiles(t *testing.T) {
	if _, err := resolveCreateLifecyclePrompt("agy", "", filepath.Join(t.TempDir(), "missing")); err == nil || !strings.Contains(err.Error(), "read AGY startup prompt file") {
		t.Fatalf("missing prompt file error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "oversized.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 10*1024+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveCreateLifecyclePrompt("agy", "", path); err == nil || !strings.Contains(err.Error(), "prompt file too large") {
		t.Fatalf("oversized prompt file error = %v", err)
	}
}

func TestResolveCreateLifecyclePromptPreservesOtherHarnessAndDirectPromptBehavior(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if got, err := resolveCreateLifecyclePrompt("claude-code", "", missing); err != nil || got != "" {
		t.Fatalf("Claude prompt resolution = %q, %v", got, err)
	}
	if got, err := resolveCreateLifecyclePrompt("agy", "direct", missing); err != nil || got != "direct" {
		t.Fatalf("direct AGY prompt resolution = %q, %v", got, err)
	}
}

func TestCLICreateSessionRuntimeUsesCallerContextForAgyIdentityBootstrap(t *testing.T) {
	type callerContextKey struct{}
	ctx := context.WithValue(t.Context(), callerContextKey{}, "caller")
	called := false
	runtime := &cliCreateSessionRuntime{bootstrapAgyIdentity: func(gotCtx context.Context, input ops.AgyCreateIdentityBootstrap) error {
		called = true
		if gotCtx != ctx || input.SessionName != "agy-bootstrap" || input.Prompt != "one prompt" {
			t.Fatalf("bootstrap input = caller:%t %+v", gotCtx == ctx, input)
		}
		return nil
	}}
	if err := runtime.BootstrapAgyCreateIdentity(ctx, ops.AgyCreateIdentityBootstrap{SessionName: "agy-bootstrap", Prompt: "one prompt"}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("CLI AGY identity bootstrap was not called")
	}
}

func TestCLICreateSessionRuntimeUsesAgyBracketedRawPaste(t *testing.T) {
	original := checkExpectedHarnessInputAndSend
	t.Cleanup(func() { checkExpectedHarnessInputAndSend = original })
	called := false
	checkExpectedHarnessInputAndSend = func(ctx context.Context, sessionName, harness, prompt string, options tmux.InputDeliveryOptions) (tmux.HarnessInputReadiness, error) {
		called = true
		if ctx != t.Context() || sessionName != "agy-bootstrap" || prompt != "first line\nsecond line" || harness != "agy" || options.AllowQueuedAGM {
			t.Fatalf("bootstrap atomic send = context:%t %q/%q/%q/%+v", ctx == t.Context(), sessionName, prompt, harness, options)
		}
		return tmux.HarnessInputReadiness{Ready: true, State: tmux.HarnessInputReady, TargetPane: "%7"}, nil
	}
	runtime := newCLICreateSessionRuntime("agy-bootstrap", false, true, nil)
	if err := runtime.BootstrapAgyCreateIdentity(t.Context(), ops.AgyCreateIdentityBootstrap{
		SessionName: "agy-bootstrap",
		Prompt:      "first line\nsecond line",
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("CLI AGY identity bootstrap did not use harness-aware atomic delivery")
	}
}

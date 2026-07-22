package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

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
		if ctx != t.Context() || sessionName != "agy-bootstrap" || prompt != "first line\nsecond line" || harness != "agy" || options.AllowBusyComposer {
			t.Fatalf("bootstrap atomic send = context:%t %q/%q/%q/%+v", ctx == t.Context(), sessionName, prompt, harness, options)
		}
		return tmux.HarnessInputReadiness{Ready: true, State: tmux.HarnessInputReady, TargetPane: "%7"}, nil
	}
	runtime := newCLICreateSessionRuntime("agy-bootstrap", false, true)
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

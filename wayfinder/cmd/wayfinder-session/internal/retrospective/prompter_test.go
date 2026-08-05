package retrospective

import (
	"testing"
)

func TestPromptUserForContext_Magnitude0(t *testing.T) {
	// Magnitude 0 skips prompting but retains supplied trace metadata.
	flags := RewindFlags{Reason: "replay phase", Learnings: "keep the trace"}
	ctx, err := PromptUserForContext(0, flags)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if ctx == nil {
		t.Fatal("Expected context for magnitude 0")
	}
	if ctx.Reason != flags.Reason || ctx.Learnings != flags.Learnings {
		t.Errorf("Magnitude 0 context = %#v, want supplied flags", ctx)
	}
}

func TestPromptUserForContext_NoPromptFlag(t *testing.T) {
	// --no-prompt flag should skip prompting
	flags := RewindFlags{
		NoPrompt:  true,
		Reason:    "test reason",
		Learnings: "test learnings",
	}

	ctx, err := PromptUserForContext(2, flags)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if ctx == nil {
		t.Fatal("Expected context, got nil")
		return
	}
	if ctx.Reason != "test reason" {
		t.Errorf("Expected reason 'test reason', got '%s'", ctx.Reason)
	}
	if ctx.Learnings != "test learnings" {
		t.Errorf("Expected learnings 'test learnings', got '%s'", ctx.Learnings)
	}
}

func TestPromptUserForContext_PreProvidedReason(t *testing.T) {
	// Pre-provided reason via --reason flag should skip prompting
	flags := RewindFlags{
		Reason:    "pre-provided reason",
		Learnings: "pre-provided learnings",
	}

	ctx, err := PromptUserForContext(1, flags)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if ctx == nil {
		t.Fatal("Expected context, got nil")
		return
	}
	if ctx.Reason != "pre-provided reason" {
		t.Errorf("Expected reason 'pre-provided reason', got '%s'", ctx.Reason)
	}
}

func TestIsTerminal(t *testing.T) {
	// Just test that function doesn't crash
	// Actual value depends on test environment (may be true or false)
	_ = isTerminal()
}

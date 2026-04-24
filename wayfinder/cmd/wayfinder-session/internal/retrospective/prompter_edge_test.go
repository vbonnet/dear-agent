package retrospective

import (
	"testing"
)

// TestPromptUserForContext_EdgeCases tests additional edge cases
func TestPromptUserForContext_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		magnitude int
		flags     RewindFlags
		wantNil   bool
	}{
		{
			name:      "Magnitude 0 returns nil",
			magnitude: 0,
			flags:     RewindFlags{},
			wantNil:   true,
		},
		{
			name:      "Magnitude 1 with no-prompt flag",
			magnitude: 1,
			flags:     RewindFlags{NoPrompt: true},
			wantNil:   false,
		},
		{
			name:      "Magnitude 2 with reason provided",
			magnitude: 2,
			flags:     RewindFlags{Reason: "Provided reason"},
			wantNil:   false,
		},
		{
			name:      "Magnitude 3 with reason and learnings",
			magnitude: 3,
			flags:     RewindFlags{Reason: "Reason", Learnings: "Learnings"},
			wantNil:   false,
		},
		{
			name:      "Large magnitude with no-prompt",
			magnitude: 12,
			flags:     RewindFlags{NoPrompt: true},
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := PromptUserForContext(tt.magnitude, tt.flags)

			if err != nil {
				t.Errorf("PromptUserForContext returned error: %v", err)
			}

			if tt.wantNil && ctx != nil {
				t.Errorf("Expected nil context for magnitude %d, got: %v", tt.magnitude, ctx)
			}

			if !tt.wantNil {
				if ctx == nil {
					t.Errorf("Expected non-nil context for magnitude %d", tt.magnitude)
				}

				// Verify pre-provided values are preserved
				if tt.flags.Reason != "" && ctx.Reason != tt.flags.Reason {
					t.Errorf("Expected reason '%s', got '%s'", tt.flags.Reason, ctx.Reason)
				}

				if tt.flags.Learnings != "" && ctx.Learnings != tt.flags.Learnings {
					t.Errorf("Expected learnings '%s', got '%s'", tt.flags.Learnings, ctx.Learnings)
				}
			}
		})
	}
}

// TestPromptUserForContext_OnlyReason tests providing only reason (no learnings)
func TestPromptUserForContext_OnlyReason(t *testing.T) {
	flags := RewindFlags{
		Reason: "Only reason provided",
		// Learnings not provided
	}

	ctx, err := PromptUserForContext(2, flags)
	if err != nil {
		t.Fatalf("PromptUserForContext failed: %v", err)
	}

	if ctx == nil {
		t.Fatal("Expected non-nil context")
	}

	if ctx.Reason != "Only reason provided" {
		t.Errorf("Expected reason 'Only reason provided', got: %s", ctx.Reason)
	}

	if ctx.Learnings != "" {
		t.Errorf("Expected empty learnings, got: %s", ctx.Learnings)
	}
}

// TestPromptUserForContext_BothFlagsProvided tests providing both reason and learnings
func TestPromptUserForContext_BothFlagsProvided(t *testing.T) {
	flags := RewindFlags{
		Reason:    "Test reason",
		Learnings: "Test learnings",
	}

	ctx, err := PromptUserForContext(5, flags)
	if err != nil {
		t.Fatalf("PromptUserForContext failed: %v", err)
	}

	if ctx == nil {
		t.Fatal("Expected non-nil context")
	}

	if ctx.Reason != "Test reason" {
		t.Errorf("Expected reason 'Test reason', got: %s", ctx.Reason)
	}

	if ctx.Learnings != "Test learnings" {
		t.Errorf("Expected learnings 'Test learnings', got: %s", ctx.Learnings)
	}
}

// TestPromptUserForContext_NoPromptWithoutFlags tests no-prompt without pre-provided values
func TestPromptUserForContext_NoPromptWithoutFlags(t *testing.T) {
	flags := RewindFlags{
		NoPrompt: true,
		// No reason or learnings provided
	}

	ctx, err := PromptUserForContext(3, flags)
	if err != nil {
		t.Fatalf("PromptUserForContext failed: %v", err)
	}

	if ctx == nil {
		t.Fatal("Expected non-nil context")
	}

	// Should return empty context (no prompting)
	if ctx.Reason != "" {
		t.Errorf("Expected empty reason with no-prompt, got: %s", ctx.Reason)
	}

	if ctx.Learnings != "" {
		t.Errorf("Expected empty learnings with no-prompt, got: %s", ctx.Learnings)
	}
}

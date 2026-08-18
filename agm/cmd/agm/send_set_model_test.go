package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestRunSendSetModelUsesCallerContextBeforeSlashCommandDelivery(t *testing.T) {
	originalDryRun, originalHarness := setModelDryRun, setModelHarness
	originalHasSession := setModelHasSession
	originalCapture := setModelCapturePaneOutputContext
	originalSend := setModelSendSlashCommandSafeContext
	t.Cleanup(func() {
		setModelDryRun, setModelHarness = originalDryRun, originalHarness
		setModelHasSession = originalHasSession
		setModelCapturePaneOutputContext = originalCapture
		setModelSendSlashCommandSafeContext = originalSend
	})
	setModelDryRun = false
	setModelHarness = "agy"
	setModelHasSession = func(string) (bool, error) { return true, nil }
	setModelCapturePaneOutputContext = func(context.Context, string, int) (string, error) { return "", nil }

	type callerContextKey struct{}
	callerCtx, cancel := context.WithCancel(context.WithValue(t.Context(), callerContextKey{}, "set-model"))
	setModelSendSlashCommandSafeContext = func(ctx context.Context, sessionName, command string) error {
		if ctx != callerCtx {
			t.Fatal("slash-command delivery did not receive the caller context")
		}
		if sessionName != "agy-model" || command != "/model Gemini 3.5 Flash (Low)" {
			t.Fatalf("slash-command delivery = %q/%q", sessionName, command)
		}
		cancel()
		return ctx.Err()
	}
	cmd := &cobra.Command{}
	cmd.SetContext(callerCtx)

	err := runSendSetModel(cmd, []string{"agy-model", "3.5-flash-low"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runSendSetModel() error = %v, want context.Canceled", err)
	}
}

func TestNormalizeClaudeSetModelAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sonnet", "sonnet"},
		{"default", "sonnet"},
		{"sonnet-1m", "sonnet"},
		{"opus", "opus"},
		{"opus-1m", "opus"},
		{"haiku", "haiku"},
		{"unknown-future-model", "unknown-future-model"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeClaudeSetModelAlias(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeClaudeSetModelAlias(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestResolveSetModelInstruction_ActiveHarnesses(t *testing.T) {
	tests := map[string]string{
		"claude-code":  "opus",
		"codex-cli":    "5.4-mini",
		"agy":          "3.5-flash",
		"opencode-cli": "glm-5.2",
		"pi-cli":       "gpt-fast",
	}
	for _, harness := range agent.ActiveHarnesses() {
		model := tests[harness]
		if model == "" {
			t.Fatalf("test missing model for active harness %q", harness)
		}
		t.Run(harness, func(t *testing.T) {
			instruction, err := resolveSetModelInstruction(harness, model)
			if err != nil {
				t.Fatalf("resolveSetModelInstruction(%q, %q) returned error: %v", harness, model, err)
			}
			if instruction.Harness != harness {
				t.Fatalf("instruction harness = %q, want %q", instruction.Harness, harness)
			}
			if instruction.ResolvedModel == "" {
				t.Fatal("resolved model is empty")
			}
			prefix := "/model "
			if harness == "pi-cli" {
				prefix = "/agm-model "
			}
			if !strings.HasPrefix(instruction.Command, prefix) {
				t.Fatalf("command = %q, want %s prefix", instruction.Command, prefix)
			}
			if !strings.Contains(instruction.Command, instruction.ResolvedModel) {
				t.Fatalf("command %q should include resolved model %q", instruction.Command, instruction.ResolvedModel)
			}
		})
	}
}

func TestNewPiModelConfirmationRequiresExactManagedNotice(t *testing.T) {
	instruction := setModelInstruction{Harness: "pi-cli", ResolvedModel: "openai/gpt-5.6-luna"}
	if got, ok := newModelConfirmation(instruction, "", "AGM model: openai/gpt-5.6-luna"); !ok || got == "" {
		t.Fatalf("exact Pi confirmation = %q, %v", got, ok)
	}
	if _, ok := newModelConfirmation(instruction, "", "AGM model: openai/gpt-5.6-terra"); ok {
		t.Fatal("mismatched Pi model confirmation accepted")
	}
}

func TestResolveSetModelInstruction_OpenCodeSupportsNewModelFamilies(t *testing.T) {
	for _, family := range []string{"glm", "deepseek", "nemotron", "qwen"} {
		model, ok := agent.DefaultModelForFamily(family)
		if !ok {
			t.Fatalf("missing default model for family %q", family)
		}
		instruction, err := resolveSetModelInstruction("opencode-cli", model.Alias)
		if err != nil {
			t.Fatalf("resolveSetModelInstruction(opencode-cli, %q) returned error: %v", model.Alias, err)
		}
		if instruction.ResolvedModel != model.FullName {
			t.Errorf("family %s resolved model = %q, want %q", family, instruction.ResolvedModel, model.FullName)
		}
	}
}

func TestResolveSetModelInstruction_RejectsUnsafeModel(t *testing.T) {
	_, err := resolveSetModelInstruction("codex-cli", "gpt-5;rm -rf /")
	if err == nil {
		t.Fatal("expected error for unsafe model")
	}
	if !strings.Contains(err.Error(), "disallowed character") {
		t.Errorf("error should mention disallowed character, got: %s", err.Error())
	}
}

func TestResolveSetModelInstruction_NormalizesAgyAliases(t *testing.T) {
	instruction, err := resolveSetModelInstruction("antigravity", "3.5-flash")
	if err != nil {
		t.Fatalf("resolveSetModelInstruction returned error: %v", err)
	}
	if instruction.Harness != "agy" {
		t.Fatalf("instruction harness = %q, want agy", instruction.Harness)
	}
}

func TestResolveSetModelInstruction_PreservesAgyPublicLabel(t *testing.T) {
	const publicLabel = "Gemini 3.5 Flash (Low)"
	instruction, err := resolveSetModelInstruction("agy", publicLabel)
	if err != nil {
		t.Fatalf("resolveSetModelInstruction returned error: %v", err)
	}
	if instruction.ResolvedModel != publicLabel {
		t.Fatalf("resolved model = %q, want %q", instruction.ResolvedModel, publicLabel)
	}
	if instruction.Command != "/model "+publicLabel {
		t.Fatalf("command = %q, want exact public label", instruction.Command)
	}
}

func TestResolveSetModelInstruction_NormalizesCrossHarnessAliasCase(t *testing.T) {
	instruction, err := resolveSetModelInstruction("agy", "OPUS")
	if err != nil {
		t.Fatalf("resolveSetModelInstruction returned error: %v", err)
	}
	if instruction.ResolvedModel != "Claude Opus 4.6 (Thinking)" {
		t.Fatalf("resolved model = %q, want AGY Opus public label", instruction.ResolvedModel)
	}
	if instruction.Command != "/model Claude Opus 4.6 (Thinking)" {
		t.Fatalf("command = %q, want normalized cross-harness model", instruction.Command)
	}
}

func TestNewAgyModelConfirmationRejectsStaleOrMismatchedOutput(t *testing.T) {
	instruction := setModelInstruction{Harness: "agy", ResolvedModel: "Gemini 3.5 Flash (Low)"}
	for _, tc := range []struct {
		name     string
		baseline string
		current  string
		want     bool
	}{
		{name: "new exact confirmation", current: "Set model to Gemini 3.5 Flash (Low)", want: true},
		{name: "same stale confirmation", baseline: "Set model to Gemini 3.5 Flash (Low)", current: "Set model to Gemini 3.5 Flash (Low)"},
		{name: "new confirmation added after stale copy", baseline: "Set model to Gemini 3.5 Flash (Low)", current: "Set model to Gemini 3.5 Flash (Low)\nSet model to Gemini 3.5 Flash (Low)", want: true},
		{name: "stale different model", baseline: "Set model to Gemini 3.5 Flash (Medium)", current: "request rejected\nSet model to Gemini 3.5 Flash (Medium)"},
		{name: "new different model", current: "Set model to Gemini 3.5 Flash (Medium)"},
		{name: "prefix without model", current: "Set model to"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			confirmation, got := newModelConfirmation(instruction, tc.baseline, tc.current)
			if got != tc.want {
				t.Fatalf("newModelConfirmation() = %q, %v; want match %v", confirmation, got, tc.want)
			}
			if got && confirmation != "Set model to "+instruction.ResolvedModel {
				t.Fatalf("confirmation = %q", confirmation)
			}
		})
	}
}

func TestRunSendSetModelInvalidModel(t *testing.T) {
	savedHarness := setModelHarness
	defer func() { setModelHarness = savedHarness }()
	setModelHarness = "codex-cli"

	err := runSendSetModel(nil, []string{"test-session", "gpt-4;rm"})
	if err == nil {
		t.Fatal("expected error for invalid model")
		return
	}
	if !strings.Contains(err.Error(), "invalid model change request") {
		t.Errorf("error should mention invalid request, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "gpt-4;rm") {
		t.Errorf("error should include the invalid model name, got: %s", err.Error())
	}
}

func TestPersistAgyModelSwitchPreservesOnlyConfirmedProvenance(t *testing.T) {
	adapter := dolt.NewMockAdapter()
	t.Cleanup(func() { _ = adapter.Close() })
	m := dolt.NewTestManifest("agy-model-switch", "agy-model-switch")
	m.Harness = "agy"
	m.Model = "Gemini 3.5 Flash (Medium)"
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	instruction := setModelInstruction{
		Harness:       "agy",
		ResolvedModel: "Gemini 3.5 Flash (Low)",
		Command:       "/model Gemini 3.5 Flash (Low)",
	}

	if err := persistAgyModelSwitch(adapter, m, instruction, false); err != nil {
		t.Fatalf("persist unverified switch: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after unverified switch: %v", err)
	}
	if stored.Model != "" {
		t.Fatalf("unverified model = %q, want unknown so resume omits --model", stored.Model)
	}

	if err := persistAgyModelSwitch(adapter, stored, instruction, true); err != nil {
		t.Fatalf("persist confirmed switch: %v", err)
	}
	stored, err = adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatalf("GetSession() after confirmed switch: %v", err)
	}
	if stored.Model != instruction.ResolvedModel {
		t.Fatalf("confirmed model = %q, want %q", stored.Model, instruction.ResolvedModel)
	}
}

func TestPersistPiModelSwitchPreservesConfiguredModelUntilTranscriptExists(t *testing.T) {
	adapter := dolt.NewMockAdapter()
	t.Cleanup(func() { _ = adapter.Close() })
	sessionDir := t.TempDir()
	m := dolt.NewTestManifest("pi-model-switch", "pi-model-switch")
	m.Harness = "pi-cli"
	m.Model = "anthropic/claude-sonnet-4-6"
	m.Pi = &manifest.Pi{SessionID: "pi-model-switch", SessionDir: sessionDir}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatal(err)
	}
	instruction := setModelInstruction{Harness: "pi-cli", ResolvedModel: "openai/gpt-5.6-terra"}

	if err := persistAgyModelSwitch(adapter, m, instruction, false); err != nil {
		t.Fatalf("persist unverified pre-transcript switch: %v", err)
	}
	stored, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Model != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("pre-transcript model = %q, want configured model preserved", stored.Model)
	}

	transcript := filepath.Join(sessionDir, "persisted.jsonl")
	if err := os.WriteFile(transcript, []byte(`{"type":"session","id":"pi-model-switch","cwd":"/work"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := persistAgyModelSwitch(adapter, stored, instruction, false); err != nil {
		t.Fatalf("persist unverified post-transcript switch: %v", err)
	}
	stored, err = adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Model != "" {
		t.Fatalf("post-transcript unverified model = %q, want native selection", stored.Model)
	}

	if err := persistAgyModelSwitch(adapter, stored, instruction, true); err != nil {
		t.Fatalf("persist verified Pi switch: %v", err)
	}
	stored, err = adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Model != instruction.ResolvedModel {
		t.Fatalf("verified Pi model = %q, want %q", stored.Model, instruction.ResolvedModel)
	}
}

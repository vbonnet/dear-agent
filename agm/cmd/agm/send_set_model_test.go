package main

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

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
			if !strings.HasPrefix(instruction.Command, "/model ") {
				t.Fatalf("command = %q, want /model prefix", instruction.Command)
			}
			if !strings.Contains(instruction.Command, instruction.ResolvedModel) {
				t.Fatalf("command %q should include resolved model %q", instruction.Command, instruction.ResolvedModel)
			}
		})
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

func TestVerifyModelSetParsing(t *testing.T) {
	// Test the line-matching logic used by verifyModelSet
	tests := []struct {
		name    string
		line    string
		matches bool
	}{
		{"confirmation line", "Set model to claude-sonnet-4-6-20250514", true},
		{"with whitespace", "  Set model to claude-opus-4-6  ", true},
		{"unrelated line", "Some other output", false},
		{"empty line", "", false},
		{"partial match", "Set model", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := strings.TrimSpace(tt.line)
			got := strings.HasPrefix(trimmed, "Set model to")
			if got != tt.matches {
				t.Errorf("line %q: got match=%v, want %v", tt.line, got, tt.matches)
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

package harnesseffort

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustLoadConfig(t *testing.T) HarnessEffortConfig {
	t.Helper()
	cfg, err := LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	return ResolveAliases(cfg)
}

// TestGenerateOpenCode_Default verifies that empty existingJSON produces at least
// 12 agents (4 tiers × 3 providers) and exactly 23 commands.
func TestGenerateOpenCode_Default(t *testing.T) {
	cfg := mustLoadConfig(t)
	data, err := GenerateOpenCode(cfg, "")
	if err != nil {
		t.Fatalf("GenerateOpenCode: %v", err)
	}

	var doc openCodeDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if len(doc.Agent) < 12 {
		t.Errorf("expected at least 12 agents, got %d", len(doc.Agent))
	}
	if len(doc.Command) != len(commandTierMap) {
		t.Errorf("expected %d commands, got %d", len(commandTierMap), len(doc.Command))
	}
}

// TestGenerateOpenCode_ModelFormat verifies all model strings contain a slash
// (i.e., use provider/model or provider/model@version format).
func TestGenerateOpenCode_ModelFormat(t *testing.T) {
	cfg := mustLoadConfig(t)
	data, err := GenerateOpenCode(cfg, "")
	if err != nil {
		t.Fatalf("GenerateOpenCode: %v", err)
	}

	var doc openCodeDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	for name, agent := range doc.Agent {
		if !strings.Contains(agent.Model, "/") {
			t.Errorf("agent %q model %q does not contain '/'", name, agent.Model)
		}
	}
}

// TestGenerateOpenCode_PromptField verifies agents use the "prompt" field.
// We check the raw JSON to be sure no "systemPrompt" key is present.
func TestGenerateOpenCode_PromptField(t *testing.T) {
	cfg := mustLoadConfig(t)
	data, err := GenerateOpenCode(cfg, "")
	if err != nil {
		t.Fatalf("GenerateOpenCode: %v", err)
	}

	if strings.Contains(string(data), "systemPrompt") {
		t.Error("output JSON contains 'systemPrompt'; expected 'prompt'")
	}

	var doc openCodeDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	for name, agent := range doc.Agent {
		if agent.Prompt == "" {
			t.Errorf("agent %q has empty prompt field", name)
		}
	}
}

// TestGenerateOpenCode_NoMaxTokens verifies that no "maxTokens" field appears
// anywhere in the generated JSON.
func TestGenerateOpenCode_NoMaxTokens(t *testing.T) {
	cfg := mustLoadConfig(t)
	data, err := GenerateOpenCode(cfg, "")
	if err != nil {
		t.Fatalf("GenerateOpenCode: %v", err)
	}

	if strings.Contains(string(data), "maxTokens") {
		t.Error("output JSON contains 'maxTokens'; field should not exist")
	}
}

// TestGenerateOpenCode_PreserveUserAgent verifies that a user-defined agent
// not managed by engram is preserved across regeneration.
func TestGenerateOpenCode_PreserveUserAgent(t *testing.T) {
	cfg := mustLoadConfig(t)

	existing := `{
		"$schema": "https://opencode.ai/config.json",
		"agent": {
			"my-custom-agent": {
				"model": "some/custom-model",
				"prompt": "Custom prompt",
				"mode": "primary"
			}
		},
		"command": {}
	}`

	data, err := GenerateOpenCode(cfg, existing)
	if err != nil {
		t.Fatalf("GenerateOpenCode: %v", err)
	}

	var doc openCodeDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if _, ok := doc.Agent["my-custom-agent"]; !ok {
		t.Error("user-defined agent 'my-custom-agent' was not preserved")
	}
}

// TestGenerateOpenCode_Idempotent verifies that running GenerateOpenCode twice
// (feeding first output as existingJSON) produces identical output.
func TestGenerateOpenCode_Idempotent(t *testing.T) {
	cfg := mustLoadConfig(t)

	first, err := GenerateOpenCode(cfg, "")
	if err != nil {
		t.Fatalf("first GenerateOpenCode: %v", err)
	}

	second, err := GenerateOpenCode(cfg, string(first))
	if err != nil {
		t.Fatalf("second GenerateOpenCode: %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("output is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestGenerateOpenCode_Schema verifies the output has the correct $schema field.
func TestGenerateOpenCode_Schema(t *testing.T) {
	cfg := mustLoadConfig(t)
	data, err := GenerateOpenCode(cfg, "")
	if err != nil {
		t.Fatalf("GenerateOpenCode: %v", err)
	}

	var doc openCodeDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	const wantSchema = "https://opencode.ai/config.json"
	if doc.Schema != wantSchema {
		t.Errorf("$schema = %q; want %q", doc.Schema, wantSchema)
	}
}

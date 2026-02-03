package gateway

import (
	"strings"
	"testing"
	"time"
)

func TestNewHandoffPromptGenerator(t *testing.T) {
	gen, err := NewHandoffPromptGenerator()
	if err != nil {
		t.Fatalf("NewHandoffPromptGenerator failed: %v", err)
	}

	if len(gen.templates) == 0 {
		t.Error("Expected templates to be registered, got none")
	}

	// Verify expected templates exist
	expectedTemplates := []string{
		"architect_to_implementer",
		"implementer_to_architect",
		"generic",
	}

	for _, name := range expectedTemplates {
		if _, exists := gen.templates[name]; !exists {
			t.Errorf("Template %q not registered", name)
		}
	}
}

func TestGeneratePrompt_ArchitectToImplementer(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	handoff := &HandoffContext{
		FromMode: ModeArchitect,
		ToMode:   ModeImplementer,
		Summary:  "Designed OAuth2 authentication system with PKCE flow",
		NextSteps: []string{
			"Implement OAuth2 client",
			"Add PKCE support",
			"Write integration tests",
		},
		Artifacts: map[string]string{
			"design_doc": "/docs/oauth2-design.md",
			"diagram":    "/diagrams/auth-flow.png",
		},
		Timestamp: time.Now().Unix(),
	}

	prompt, err := gen.GeneratePrompt(handoff)
	if err != nil {
		t.Fatalf("GeneratePrompt failed: %v", err)
	}

	if prompt == "" {
		t.Error("Generated prompt is empty")
	}

	// Verify prompt contains key elements
	if !strings.Contains(prompt, "Architect") {
		t.Error("Prompt should mention Architect mode")
	}

	if !strings.Contains(prompt, "Implementer") {
		t.Error("Prompt should mention Implementer mode")
	}

	if !strings.Contains(prompt, handoff.Summary) {
		t.Error("Prompt should include summary")
	}

	// Verify artifacts are included
	if !strings.Contains(prompt, "design_doc") {
		t.Error("Prompt should include artifacts")
	}

	// Verify next steps are included
	if !strings.Contains(prompt, "Implement OAuth2 client") {
		t.Error("Prompt should include next steps")
	}
}

func TestGeneratePrompt_ImplementerToArchitect(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	handoff := &HandoffContext{
		FromMode: ModeImplementer,
		ToMode:   ModeArchitect,
		Summary:  "Implemented OAuth2 client with PKCE support",
		NextSteps: []string{
			"Review implementation quality",
			"Assess security posture",
			"Plan token refresh mechanism",
		},
		Artifacts: map[string]string{
			"implementation": "/src/auth/oauth2_client.go",
			"tests":          "/tests/oauth2_test.go",
		},
		Timestamp: time.Now().Unix(),
	}

	prompt, err := gen.GeneratePrompt(handoff)
	if err != nil {
		t.Fatalf("GeneratePrompt failed: %v", err)
	}

	if prompt == "" {
		t.Error("Generated prompt is empty")
	}

	// Verify prompt mentions review
	if !strings.Contains(prompt, "Review") || !strings.Contains(prompt, "review") {
		t.Error("Prompt should mention review for Architect mode")
	}

	// Verify prompt includes implementation details
	if !strings.Contains(prompt, handoff.Summary) {
		t.Error("Prompt should include summary")
	}
}

func TestGeneratePrompt_GenericFallback(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	// Use mode transition that doesn't have a specific template
	handoff := &HandoffContext{
		FromMode:  Mode("custom_mode_a"),
		ToMode:    Mode("custom_mode_b"),
		Summary:   "Custom transition",
		NextSteps: []string{"Step 1", "Step 2"},
		Timestamp: time.Now().Unix(),
	}

	prompt, err := gen.GeneratePrompt(handoff)
	if err != nil {
		t.Fatalf("GeneratePrompt failed: %v", err)
	}

	if prompt == "" {
		t.Error("Generic template should generate non-empty prompt")
	}

	if !strings.Contains(prompt, "Mode Transition") {
		t.Error("Generic template should include transition header")
	}
}

func TestSerializeContext(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	handoff := &HandoffContext{
		FromMode: ModeArchitect,
		ToMode:   ModeImplementer,
		Summary:  "Test summary",
		NextSteps: []string{
			"Step 1",
			"Step 2",
		},
		Artifacts: map[string]string{
			"file1": "/path/to/file1",
		},
		Metadata: map[string]string{
			"key1": "value1",
		},
		Timestamp: 1234567890,
	}

	serialized, err := gen.SerializeContext(handoff)
	if err != nil {
		t.Fatalf("SerializeContext failed: %v", err)
	}

	if serialized == "" {
		t.Error("Serialized context is empty")
	}

	// Verify JSON structure
	if !strings.Contains(serialized, "\"from_mode\"") {
		t.Error("Serialized context missing from_mode")
	}

	if !strings.Contains(serialized, "\"summary\"") {
		t.Error("Serialized context missing summary")
	}
}

func TestDeserializeContext(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	jsonData := `{
		"from_mode": "architect",
		"to_mode": "implementer",
		"summary": "Test summary",
		"next_steps": ["Step 1", "Step 2"],
		"artifacts": {"file1": "/path/to/file1"},
		"metadata": {"key1": "value1"},
		"timestamp": 1234567890
	}`

	handoff, err := gen.DeserializeContext(jsonData)
	if err != nil {
		t.Fatalf("DeserializeContext failed: %v", err)
	}

	if handoff.FromMode != ModeArchitect {
		t.Errorf("FromMode = %v, want %v", handoff.FromMode, ModeArchitect)
	}

	if handoff.ToMode != ModeImplementer {
		t.Errorf("ToMode = %v, want %v", handoff.ToMode, ModeImplementer)
	}

	if handoff.Summary != "Test summary" {
		t.Errorf("Summary = %q, want %q", handoff.Summary, "Test summary")
	}

	if len(handoff.NextSteps) != 2 {
		t.Errorf("Expected 2 next steps, got %d", len(handoff.NextSteps))
	}

	if handoff.Timestamp != 1234567890 {
		t.Errorf("Timestamp = %d, want 1234567890", handoff.Timestamp)
	}
}

func TestSerializeDeserialize_Roundtrip(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	original := &HandoffContext{
		FromMode: ModeArchitect,
		ToMode:   ModeImplementer,
		Summary:  "Original summary",
		NextSteps: []string{
			"Original step 1",
			"Original step 2",
		},
		Artifacts: map[string]string{
			"original_file": "/original/path",
		},
		Metadata: map[string]string{
			"original_key": "original_value",
		},
		Timestamp: 9876543210,
	}

	// Serialize
	serialized, err := gen.SerializeContext(original)
	if err != nil {
		t.Fatalf("SerializeContext failed: %v", err)
	}

	// Deserialize
	deserialized, err := gen.DeserializeContext(serialized)
	if err != nil {
		t.Fatalf("DeserializeContext failed: %v", err)
	}

	// Verify roundtrip preserves data
	if deserialized.FromMode != original.FromMode {
		t.Errorf("FromMode mismatch: %v != %v", deserialized.FromMode, original.FromMode)
	}

	if deserialized.ToMode != original.ToMode {
		t.Errorf("ToMode mismatch: %v != %v", deserialized.ToMode, original.ToMode)
	}

	if deserialized.Summary != original.Summary {
		t.Errorf("Summary mismatch: %q != %q", deserialized.Summary, original.Summary)
	}

	if len(deserialized.NextSteps) != len(original.NextSteps) {
		t.Errorf("NextSteps length mismatch: %d != %d", len(deserialized.NextSteps), len(original.NextSteps))
	}

	if deserialized.Timestamp != original.Timestamp {
		t.Errorf("Timestamp mismatch: %d != %d", deserialized.Timestamp, original.Timestamp)
	}
}

func TestFormatArtifacts_Empty(t *testing.T) {
	artifacts := map[string]string{}
	formatted := FormatArtifacts(artifacts)

	if formatted != "No artifacts" {
		t.Errorf("FormatArtifacts(empty) = %q, want %q", formatted, "No artifacts")
	}
}

func TestFormatArtifacts_Multiple(t *testing.T) {
	artifacts := map[string]string{
		"design_doc": "/docs/design.md",
		"diagram":    "/diagrams/arch.png",
	}
	formatted := FormatArtifacts(artifacts)

	if !strings.Contains(formatted, "design_doc") {
		t.Error("Formatted artifacts should include design_doc")
	}

	if !strings.Contains(formatted, "/docs/design.md") {
		t.Error("Formatted artifacts should include path")
	}

	if !strings.Contains(formatted, "diagram") {
		t.Error("Formatted artifacts should include diagram")
	}
}

func TestFormatNextSteps_Empty(t *testing.T) {
	steps := []string{}
	formatted := FormatNextSteps(steps)

	if formatted != "No specific next steps defined" {
		t.Errorf("FormatNextSteps(empty) = %q, want message about no steps", formatted)
	}
}

func TestFormatNextSteps_Multiple(t *testing.T) {
	steps := []string{
		"Implement feature A",
		"Write tests",
		"Deploy to staging",
	}
	formatted := FormatNextSteps(steps)

	if !strings.Contains(formatted, "1. Implement feature A") {
		t.Error("Formatted steps should include numbered step 1")
	}

	if !strings.Contains(formatted, "2. Write tests") {
		t.Error("Formatted steps should include numbered step 2")
	}

	if !strings.Contains(formatted, "3. Deploy to staging") {
		t.Error("Formatted steps should include numbered step 3")
	}
}

func TestPromptLength_Reasonable(t *testing.T) {
	gen, _ := NewHandoffPromptGenerator()

	handoff := &HandoffContext{
		FromMode: ModeArchitect,
		ToMode:   ModeImplementer,
		Summary:  "Test transition",
		NextSteps: []string{
			"Step 1",
			"Step 2",
		},
		Artifacts: map[string]string{
			"file1": "/path1",
		},
		Timestamp: time.Now().Unix(),
	}

	prompt, _ := gen.GeneratePrompt(handoff)

	// Prompt should be substantial but not excessive
	if len(prompt) < 100 {
		t.Errorf("Prompt too short: %d characters", len(prompt))
	}

	if len(prompt) > 10000 {
		t.Errorf("Prompt too long: %d characters", len(prompt))
	}
}

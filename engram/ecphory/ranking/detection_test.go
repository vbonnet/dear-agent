package ranking

import "testing"

// TestDetectEnv_Single tests detectEnv with a single variable
func TestDetectEnv_Single(t *testing.T) {
	t.Setenv("TEST_VAR1", "value1")

	got := detectEnv("TEST_VAR1")
	if got != "value1" {
		t.Errorf("detectEnv() = %q, want %q", got, "value1")
	}
}

// TestDetectEnv_Multiple tests detectEnv precedence (first wins)
func TestDetectEnv_Multiple(t *testing.T) {
	t.Setenv("TEST_VAR1", "value1")
	t.Setenv("TEST_VAR2", "value2")

	got := detectEnv("TEST_VAR1", "TEST_VAR2")
	if got != "value1" {
		t.Errorf("detectEnv() = %q, want %q (first variable should win)", got, "value1")
	}
}

// TestDetectEnv_Fallback tests detectEnv fallback when first is empty
func TestDetectEnv_Fallback(t *testing.T) {
	t.Setenv("TEST_VAR2", "value2")
	// TEST_VAR1 not set

	got := detectEnv("TEST_VAR1", "TEST_VAR2")
	if got != "value2" {
		t.Errorf("detectEnv() = %q, want %q (should fallback to second)", got, "value2")
	}
}

// TestDetectEnv_AllEmpty tests detectEnv when all variables are unset
func TestDetectEnv_AllEmpty(t *testing.T) {
	got := detectEnv("NONEXISTENT_VAR1", "NONEXISTENT_VAR2")
	if got != "" {
		t.Errorf("detectEnv() = %q, want empty string when all unset", got)
	}
}

// TestDetectEnv_EmptyString tests that empty string is treated as unset
func TestDetectEnv_EmptyString(t *testing.T) {
	t.Setenv("TEST_VAR1", "")
	t.Setenv("TEST_VAR2", "value2")

	got := detectEnv("TEST_VAR1", "TEST_VAR2")
	if got != "value2" {
		t.Errorf("detectEnv() = %q, want %q (empty string should be treated as unset)", got, "value2")
	}
}

// TestDetectEnv_NoArgs tests detectEnv with zero arguments
func TestDetectEnv_NoArgs(t *testing.T) {
	got := detectEnv()
	if got != "" {
		t.Errorf("detectEnv() = %q, want empty string with no args", got)
	}
}

// TestDetect_ClaudeCodeVertexClaude tests Vertex Claude detection with Claude Code variables
func TestDetect_ClaudeCodeVertexClaude(t *testing.T) {
	// Clear all env vars first
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("VERTEX_LOCATION", "")
	t.Setenv("USE_VERTEX_GEMINI", "")
	t.Setenv("GEMINI_API_KEY", "")

	// Set Claude Code variables
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "test-project")
	t.Setenv("CLOUD_ML_REGION", "us-east5")

	factory := &Factory{
		providers: make(map[string]Provider),
	}

	result := factory.Detect()

	if result.Provider != "vertexai-claude" {
		t.Errorf("Detect() provider = %q, want %q", result.Provider, "vertexai-claude")
	}
}

// TestDetect_ClaudeCodeVertexGemini tests Vertex Gemini detection with Claude Code variables
func TestDetect_ClaudeCodeVertexGemini(t *testing.T) {
	// Clear all env vars first to avoid picking up session environment
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("VERTEX_LOCATION", "")
	t.Setenv("CLOUD_ML_REGION", "")
	t.Setenv("USE_VERTEX_GEMINI", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	// Set Claude Code variables
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "test-project")
	t.Setenv("GEMINI_API_KEY", "test-key")

	factory := &Factory{
		providers: make(map[string]Provider),
	}

	result := factory.Detect()

	// Should detect Gemini since GEMINI_API_KEY is present
	if result.Provider != "vertexai-gemini" {
		t.Errorf("Detect() provider = %q, want %q", result.Provider, "vertexai-gemini")
	}
}

// TestDetect_ClaudeCodeWithoutGCP tests that Claude Code variables work without standard GCP vars
func TestDetect_ClaudeCodeWithoutGCP(t *testing.T) {
	// Clear all env vars first
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("USE_VERTEX_GEMINI", "")
	t.Setenv("GEMINI_API_KEY", "")

	// Explicitly ensure standard GCP vars are not set
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("VERTEX_LOCATION", "")

	// Set Claude Code variables only
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "claude-code-project")
	t.Setenv("CLOUD_ML_REGION", "us-east5")

	factory := &Factory{
		providers: make(map[string]Provider),
	}

	result := factory.Detect()

	if result.Provider != "vertexai-claude" {
		t.Errorf("Detect() provider = %q, want %q (should work without GOOGLE_CLOUD_PROJECT)", result.Provider, "vertexai-claude")
	}
}

// TestDetect_Precedence_ProjectID tests that standard GCP project ID wins over Claude Code
func TestDetect_Precedence_ProjectID(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "gcp-project")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "claude-code-project")
	t.Setenv("VERTEX_LOCATION", "us-east5")

	// Test that standard GCP variable is used (precedence)
	projectID := detectEnv("GOOGLE_CLOUD_PROJECT", "ANTHROPIC_VERTEX_PROJECT_ID")
	if projectID != "gcp-project" {
		t.Errorf("detectEnv() = %q, want %q (GOOGLE_CLOUD_PROJECT should have precedence)", projectID, "gcp-project")
	}
}

// TestDetect_Precedence_Location tests that standard GCP location wins over Claude Code
func TestDetect_Precedence_Location(t *testing.T) {
	t.Setenv("VERTEX_LOCATION", "us-east5")
	t.Setenv("CLOUD_ML_REGION", "us-central1")

	location := detectEnv("VERTEX_LOCATION", "CLOUD_ML_REGION")
	if location != "us-east5" {
		t.Errorf("detectEnv() = %q, want %q (VERTEX_LOCATION should have precedence)", location, "us-east5")
	}
}

// TestDetect_Precedence_Gemini tests that USE_VERTEX_GEMINI wins over GEMINI_API_KEY
func TestDetect_Precedence_Gemini(t *testing.T) {
	t.Setenv("USE_VERTEX_GEMINI", "true")
	t.Setenv("GEMINI_API_KEY", "test-key")

	useGemini := detectEnv("USE_VERTEX_GEMINI", "GEMINI_API_KEY")
	if useGemini != "true" {
		t.Errorf("detectEnv() = %q, want %q (USE_VERTEX_GEMINI should have precedence)", useGemini, "true")
	}
}

// TestDetect_BackwardsCompatibility_StandardGCP tests that standard GCP detection is unchanged
func TestDetect_BackwardsCompatibility_StandardGCP(t *testing.T) {
	// Clear all env vars first
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "")
	t.Setenv("CLOUD_ML_REGION", "")
	t.Setenv("USE_VERTEX_GEMINI", "")
	t.Setenv("GEMINI_API_KEY", "")

	// Set only standard GCP variables
	t.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")
	t.Setenv("VERTEX_LOCATION", "us-east5")

	factory := &Factory{
		providers: make(map[string]Provider),
	}

	result := factory.Detect()

	if result.Provider != "vertexai-claude" {
		t.Errorf("Detect() provider = %q, want %q (standard GCP detection unchanged)", result.Provider, "vertexai-claude")
	}
}

// TestDetect_BackwardsCompatibility_Anthropic tests that Anthropic detection is unchanged
func TestDetect_BackwardsCompatibility_Anthropic(t *testing.T) {
	// Clear all env vars first
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "")
	t.Setenv("VERTEX_LOCATION", "")
	t.Setenv("CLOUD_ML_REGION", "")
	t.Setenv("USE_VERTEX_GEMINI", "")
	t.Setenv("GEMINI_API_KEY", "")

	// Set only Anthropic API key
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test123")

	factory := &Factory{
		providers: make(map[string]Provider),
	}

	result := factory.Detect()

	if result.Provider != "anthropic" {
		t.Errorf("Detect() provider = %q, want %q (Anthropic detection unchanged)", result.Provider, "anthropic")
	}
}

// TestDetect_BackwardsCompatibility_Local tests that local fallback is unchanged
func TestDetect_BackwardsCompatibility_Local(t *testing.T) {
	// Clear all env vars to test local fallback
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "")
	t.Setenv("VERTEX_LOCATION", "")
	t.Setenv("CLOUD_ML_REGION", "")
	t.Setenv("USE_VERTEX_GEMINI", "")
	t.Setenv("GEMINI_API_KEY", "")

	factory := &Factory{
		providers: make(map[string]Provider),
	}

	result := factory.Detect()

	if result.Provider != "local" {
		t.Errorf("Detect() provider = %q, want %q (local fallback unchanged)", result.Provider, "local")
	}
}

// TestDetect_PartialConfiguration tests fallback with only project ID
func TestDetect_PartialConfiguration(t *testing.T) {
	// Clear all env vars first
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("VERTEX_LOCATION", "")
	t.Setenv("CLOUD_ML_REGION", "")
	t.Setenv("USE_VERTEX_GEMINI", "")
	t.Setenv("GEMINI_API_KEY", "")

	// Set only project ID (no region)
	t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "test-project")

	factory := &Factory{
		providers: make(map[string]Provider),
	}

	result := factory.Detect()

	// Should fallback to local (no valid Vertex configuration)
	if result.Provider != "local" {
		t.Errorf("Detect() provider = %q, want %q (should fallback with partial config)", result.Provider, "local")
	}
}

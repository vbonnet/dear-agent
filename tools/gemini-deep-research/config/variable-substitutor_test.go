package config

import (
	"strings"
	"testing"
	"time"
)

// TestSubstituteURL tests substitution of {url} variable
func TestSubstituteURL(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Extract from {url}",
		AnalyzePrompt:  "Default analyze prompt",
		ResearchPrompt: "Default research prompt",
	}
	variables := Variables{
		URL:         "https://example.com",
		Topics:      []string{"ai", "ml"},
		ContentType: "article",
	}

	result, err := vs.Substitute(config, variables)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "Extract from https://example.com"
	if result.ExtractPrompt != expected {
		t.Errorf("Expected %q, got %q", expected, result.ExtractPrompt)
	}
}

// TestSubstituteTopics tests substitution of {topics} variable
func TestSubstituteTopics(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Default extract prompt",
		AnalyzePrompt:  "Analyze {topics}",
		ResearchPrompt: "Default research prompt",
	}
	variables := Variables{
		URL:         "https://example.com",
		Topics:      []string{"ai", "ml", "research"},
		ContentType: "article",
	}

	result, err := vs.Substitute(config, variables)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "Analyze ai, ml, research"
	if result.AnalyzePrompt != expected {
		t.Errorf("Expected %q, got %q", expected, result.AnalyzePrompt)
	}
}

// TestSubstituteContentType tests substitution of {content_type} variable
func TestSubstituteContentType(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Default extract prompt",
		AnalyzePrompt:  "Default analyze prompt",
		ResearchPrompt: "Research {content_type} content",
	}
	variables := Variables{
		URL:         "https://example.com",
		Topics:      []string{"ai"},
		ContentType: "video",
	}

	result, err := vs.Substitute(config, variables)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "Research video content"
	if result.ResearchPrompt != expected {
		t.Errorf("Expected %q, got %q", expected, result.ResearchPrompt)
	}
}

// TestSubstituteAllVariables tests substitution of all variables in single prompt
func TestSubstituteAllVariables(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Extract from {url} focusing on {topics} in {content_type}",
		AnalyzePrompt:  "Default analyze prompt",
		ResearchPrompt: "Default research prompt",
	}
	variables := Variables{
		URL:         "https://youtube.com/watch?v=abc",
		Topics:      []string{"machine learning", "neural networks"},
		ContentType: "video",
	}

	result, err := vs.Substitute(config, variables)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "Extract from https://youtube.com/watch?v=abc focusing on machine learning, neural networks in video"
	if result.ExtractPrompt != expected {
		t.Errorf("Expected %q, got %q", expected, result.ExtractPrompt)
	}
}

// TestNoVariablesInTemplate tests that prompts without variables are returned unchanged
func TestNoVariablesInTemplate(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Extract key topics from the content",
		AnalyzePrompt:  "Analyze topics for research depth",
		ResearchPrompt: "Conduct deep research on each topic",
	}
	variables := Variables{
		URL:         "https://example.com",
		Topics:      []string{"ai"},
		ContentType: "article",
	}

	result, err := vs.Substitute(config, variables)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// All prompts should remain unchanged
	if result.ExtractPrompt != config.ExtractPrompt {
		t.Errorf("Extract prompt changed unexpectedly: %q != %q", result.ExtractPrompt, config.ExtractPrompt)
	}
	if result.AnalyzePrompt != config.AnalyzePrompt {
		t.Errorf("Analyze prompt changed unexpectedly: %q != %q", result.AnalyzePrompt, config.AnalyzePrompt)
	}
	if result.ResearchPrompt != config.ResearchPrompt {
		t.Errorf("Research prompt changed unexpectedly: %q != %q", result.ResearchPrompt, config.ResearchPrompt)
	}
}

// TestUnknownVariableFailFast tests that unknown variables cause an error
func TestUnknownVariableFailFast(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Extract {foo} from content",
		AnalyzePrompt:  "Default analyze prompt",
		ResearchPrompt: "Default research prompt",
	}
	variables := Variables{
		URL:         "https://example.com",
		Topics:      []string{"ai"},
		ContentType: "article",
	}

	_, err := vs.Substitute(config, variables)
	if err == nil {
		t.Fatal("Expected error for unknown variable {foo}, got nil")
	}

	// Check error message includes unknown variable and available variables
	errMsg := err.Error()
	if !strings.Contains(errMsg, "{foo}") {
		t.Errorf("Error message should mention unknown variable {foo}: %v", err)
	}
	if !strings.Contains(errMsg, "available") {
		t.Errorf("Error message should list available variables: %v", err)
	}
	if !strings.Contains(errMsg, "url") {
		t.Errorf("Error message should list 'url' as available variable: %v", err)
	}
}

// TestMultipleUnknownVariables tests that multiple unknown variables are reported
func TestMultipleUnknownVariables(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Extract {foo} and {bar} from content",
		AnalyzePrompt:  "Default analyze prompt",
		ResearchPrompt: "Default research prompt",
	}
	variables := Variables{
		URL:         "https://example.com",
		Topics:      []string{"ai"},
		ContentType: "article",
	}

	_, err := vs.Substitute(config, variables)
	if err == nil {
		t.Fatal("Expected error for unknown variables {foo} and {bar}, got nil")
	}

	// Check error message includes both unknown variables
	errMsg := err.Error()
	if !strings.Contains(errMsg, "{foo}") {
		t.Errorf("Error message should mention unknown variable {foo}: %v", err)
	}
	if !strings.Contains(errMsg, "{bar}") {
		t.Errorf("Error message should mention unknown variable {bar}: %v", err)
	}
}

// TestVariableUsedMultipleTimes tests that all instances of a variable are replaced
func TestVariableUsedMultipleTimes(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Extract from {url} and analyze {url} thoroughly",
		AnalyzePrompt:  "Default analyze prompt",
		ResearchPrompt: "Default research prompt",
	}
	variables := Variables{
		URL:         "https://arxiv.org/abs/1234.5678",
		Topics:      []string{"ai"},
		ContentType: "article",
	}

	result, err := vs.Substitute(config, variables)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "Extract from https://arxiv.org/abs/1234.5678 and analyze https://arxiv.org/abs/1234.5678 thoroughly"
	if result.ExtractPrompt != expected {
		t.Errorf("Expected %q, got %q", expected, result.ExtractPrompt)
	}

	// Ensure both instances were replaced (should not contain {url})
	if strings.Contains(result.ExtractPrompt, "{url}") {
		t.Errorf("Not all {url} instances were replaced: %q", result.ExtractPrompt)
	}
}

// TestPerformance validates that substitution completes in <1ms per prompt
func TestPerformance(t *testing.T) {
	vs := NewVariableSubstitutor()

	// Create a realistic config with all variables
	config := ResolvedConfig{
		ExtractPrompt:  "Extract key topics from {url} focusing on {topics} in {content_type} format",
		AnalyzePrompt:  "Analyze {topics} for depth and relevance from {content_type}",
		ResearchPrompt: "Research {topics} deeply, considering the {content_type} context",
	}

	variables := Variables{
		URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Topics: []string{
			"artificial intelligence",
			"machine learning",
			"neural networks",
			"deep learning",
			"reinforcement learning",
		},
		ContentType: "video",
	}

	// Warm-up run (to exclude initialization overhead)
	_, _ = vs.Substitute(config, variables)

	// Measure 100 iterations
	iterations := 100
	start := time.Now()

	for i := 0; i < iterations; i++ {
		_, err := vs.Substitute(config, variables)
		if err != nil {
			t.Fatalf("Unexpected error during performance test: %v", err)
		}
	}

	elapsed := time.Since(start)
	avgPerIteration := elapsed / time.Duration(iterations)

	// Each iteration processes 3 prompts, so per-prompt time is avg/3
	avgPerPrompt := avgPerIteration / 3

	// Target: <1ms per prompt
	targetDuration := 1 * time.Millisecond

	if avgPerPrompt > targetDuration {
		t.Errorf("Performance target not met: average %v per prompt (target: <%v)",
			avgPerPrompt, targetDuration)
	}

	t.Logf("Performance: %v per prompt (target: <%v) ✓", avgPerPrompt, targetDuration)
}

// TestEmptyTopics tests substitution when topics list is empty
func TestEmptyTopics(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Extract topics",
		AnalyzePrompt:  "Analyze {topics}",
		ResearchPrompt: "Default research prompt",
	}
	variables := Variables{
		URL:         "https://example.com",
		Topics:      []string{}, // Empty topics
		ContentType: "article",
	}

	result, err := vs.Substitute(config, variables)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Empty topics should result in empty string (no comma separation)
	expected := "Analyze "
	if result.AnalyzePrompt != expected {
		t.Errorf("Expected %q, got %q", expected, result.AnalyzePrompt)
	}
}

// TestCaseSensitiveVariables tests that variable names are case-sensitive
func TestCaseSensitiveVariables(t *testing.T) {
	vs := NewVariableSubstitutor()
	config := ResolvedConfig{
		ExtractPrompt:  "Extract from {URL}",
		AnalyzePrompt:  "Default analyze prompt",
		ResearchPrompt: "Default research prompt",
	}
	variables := Variables{
		URL:         "https://example.com",
		Topics:      []string{"ai"},
		ContentType: "article",
	}

	// {URL} (uppercase) should NOT be substituted (only {url} is valid)
	_, err := vs.Substitute(config, variables)
	if err == nil {
		t.Fatal("Expected error for case-sensitive variable {URL}, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "{URL}") {
		t.Errorf("Error message should mention unknown variable {URL}: %v", err)
	}
}

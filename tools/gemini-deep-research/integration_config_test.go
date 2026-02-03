package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/cmd"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/config"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

// TestIntegrationConfigFlow_FullOrchestration tests the complete orchestration flow:
// config loading → file resolution → variable substitution → pipeline execution
func TestIntegrationConfigFlow_FullOrchestration(t *testing.T) {
	// Setup temp directory for config files
	tempDir := t.TempDir()

	// Create global config
	globalConfigDir := filepath.Join(tempDir, ".config", "ai-tools")
	os.MkdirAll(globalConfigDir, 0755)
	globalConfig := `prompts:
  extract: "Global extract prompt"
  analyze: "Global analyze prompt"
  research: "Global research prompt"
`
	os.WriteFile(filepath.Join(globalConfigDir, "prompts.yaml"), []byte(globalConfig), 0644)

	// Create project config
	projectConfigDir := filepath.Join(tempDir, ".ai-tools")
	os.MkdirAll(projectConfigDir, 0755)
	projectConfig := `prompts:
  analyze: "Project analyze prompt overrides global"
`
	os.WriteFile(filepath.Join(projectConfigDir, "prompts.yaml"), []byte(projectConfig), 0644)

	// Change to temp directory to test project config loading
	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	// Override home directory for global config
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Test with CLI override
	flags := &types.Flags{
		ResearchPrompt: "CLI research prompt overrides all",
	}

	resolved, err := cmd.LoadAndResolvePrompts(flags)
	if err != nil {
		t.Fatalf("LoadAndResolvePrompts failed: %v", err)
	}

	// Verify precedence: CLI > Project > Global > Defaults
	if resolved.ExtractPrompt != "Global extract prompt" {
		t.Errorf("Expected global extract prompt, got: %s", resolved.ExtractPrompt)
	}
	if resolved.AnalyzePrompt != "Project analyze prompt overrides global" {
		t.Errorf("Expected project analyze prompt, got: %s", resolved.AnalyzePrompt)
	}
	if resolved.ResearchPrompt != "CLI research prompt overrides all" {
		t.Errorf("Expected CLI research prompt, got: %s", resolved.ResearchPrompt)
	}
}

// TestIntegrationConfigFlow_ConfigPrecedence tests end-to-end config precedence
func TestIntegrationConfigFlow_ConfigPrecedence(t *testing.T) {
	tempDir := t.TempDir()

	// Create only global config
	globalConfigDir := filepath.Join(tempDir, ".config", "ai-tools")
	os.MkdirAll(globalConfigDir, 0755)
	globalConfig := `prompts:
  extract: "Global extract"
  analyze: "Global analyze"
  research: "Global research"
`
	os.WriteFile(filepath.Join(globalConfigDir, "prompts.yaml"), []byte(globalConfig), 0644)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Test without CLI flags - should use global config
	flags := &types.Flags{}
	resolved, err := cmd.LoadAndResolvePrompts(flags)
	if err != nil {
		t.Fatalf("LoadAndResolvePrompts failed: %v", err)
	}

	if resolved.ExtractPrompt != "Global extract" {
		t.Errorf("Expected global extract, got: %s", resolved.ExtractPrompt)
	}
	if resolved.AnalyzePrompt != "Global analyze" {
		t.Errorf("Expected global analyze, got: %s", resolved.AnalyzePrompt)
	}
	if resolved.ResearchPrompt != "Global research" {
		t.Errorf("Expected global research, got: %s", resolved.ResearchPrompt)
	}
}

// TestIntegrationConfigFlow_FileResolutionWithVariables tests @file syntax with variable substitution
func TestIntegrationConfigFlow_FileResolutionWithVariables(t *testing.T) {
	tempDir := t.TempDir()

	// Create prompt file with variables
	promptFile := filepath.Join(tempDir, "extract.txt")
	promptContent := "Extract topics from {url} of type {content_type}"
	os.WriteFile(promptFile, []byte(promptContent), 0644)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	// Use @file syntax in CLI flag
	flags := &types.Flags{
		ExtractPrompt: "@extract.txt",
	}

	// Step 1: Load and resolve @file
	resolved, err := cmd.LoadAndResolvePrompts(flags)
	if err != nil {
		t.Fatalf("LoadAndResolvePrompts failed: %v", err)
	}

	// Verify file content was loaded
	if resolved.ExtractPrompt != promptContent {
		t.Errorf("Expected file content, got: %s", resolved.ExtractPrompt)
	}

	// Step 2: Substitute variables
	variables := config.Variables{
		URL:         "https://example.com",
		Topics:      []string{"AI", "ML"},
		ContentType: "article",
	}

	final, err := cmd.SubstituteVariables(resolved, variables)
	if err != nil {
		t.Fatalf("SubstituteVariables failed: %v", err)
	}

	expected := "Extract topics from https://example.com of type article"
	if final.ExtractPrompt != expected {
		t.Errorf("Expected: %s\nGot: %s", expected, final.ExtractPrompt)
	}
}

// TestIntegrationConfigFlow_ErrorHandling tests error handling across components
func TestIntegrationConfigFlow_ErrorHandling(t *testing.T) {
	t.Run("Invalid YAML syntax", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create invalid YAML
		globalConfigDir := filepath.Join(tempDir, ".config", "ai-tools")
		os.MkdirAll(globalConfigDir, 0755)
		invalidYAML := `prompts:
  extract: "Valid"
  analyze: invalid yaml: [unclosed bracket
`
		os.WriteFile(filepath.Join(globalConfigDir, "prompts.yaml"), []byte(invalidYAML), 0644)

		oldHome := os.Getenv("HOME")
		os.Setenv("HOME", tempDir)
		defer os.Setenv("HOME", oldHome)

		flags := &types.Flags{}
		_, err := cmd.LoadAndResolvePrompts(flags)
		if err == nil {
			t.Error("Expected error for invalid YAML, got nil")
		}
	})

	t.Run("File not found", func(t *testing.T) {
		flags := &types.Flags{
			ExtractPrompt: "@nonexistent.txt",
		}

		_, err := cmd.LoadAndResolvePrompts(flags)
		if err == nil {
			t.Error("Expected error for nonexistent file, got nil")
		}
	})

	t.Run("Unknown variables", func(t *testing.T) {
		resolved := config.ResolvedConfig{
			ExtractPrompt: "Extract {unknown_var}",
		}

		variables := config.Variables{
			URL: "https://example.com",
		}

		_, err := cmd.SubstituteVariables(resolved, variables)
		if err == nil {
			t.Error("Expected error for unknown variable, got nil")
		}
	})
}

// TestIntegrationConfigFlow_BackwardCompatibility tests migration from --input to --analyze-prompt
func TestIntegrationConfigFlow_BackwardCompatibility(t *testing.T) {
	t.Run("Legacy --input flag", func(t *testing.T) {
		flags := &types.Flags{
			Input: "Legacy input prompt",
		}

		resolved, err := cmd.LoadAndResolvePrompts(flags)
		if err != nil {
			t.Fatalf("LoadAndResolvePrompts failed: %v", err)
		}

		// Legacy --input should map to analyze prompt
		if resolved.AnalyzePrompt != "Legacy input prompt" {
			t.Errorf("Expected legacy input to map to analyze prompt, got: %s", resolved.AnalyzePrompt)
		}
	})

	t.Run("Legacy --input-file flag", func(t *testing.T) {
		tempDir := t.TempDir()
		inputFile := filepath.Join(tempDir, "input.txt")
		os.WriteFile(inputFile, []byte("Legacy file content"), 0644)

		flags := &types.Flags{
			InputFile: inputFile,
		}

		resolved, err := cmd.LoadAndResolvePrompts(flags)
		if err != nil {
			t.Fatalf("LoadAndResolvePrompts failed: %v", err)
		}

		// Legacy --input-file should map to analyze prompt with @file
		if resolved.AnalyzePrompt != "Legacy file content" {
			t.Errorf("Expected legacy input-file content, got: %s", resolved.AnalyzePrompt)
		}
	})
}

// TestIntegrationConfigFlow_Performance tests that config processing is fast (<200ms)
func TestIntegrationConfigFlow_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	tempDir := t.TempDir()

	// Create config files
	globalConfigDir := filepath.Join(tempDir, ".config", "ai-tools")
	os.MkdirAll(globalConfigDir, 0755)
	globalConfig := `prompts:
  extract: "Extract {topics}"
  analyze: "Analyze {url}"
  research: "Research {content_type}"
`
	os.WriteFile(filepath.Join(globalConfigDir, "prompts.yaml"), []byte(globalConfig), 0644)

	// Create prompt file
	promptFile := filepath.Join(tempDir, "prompt.txt")
	promptContent := "Custom prompt with {url} and {topics}"
	os.WriteFile(promptFile, []byte(promptContent), 0644)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// This test is covered by BenchmarkConfigFlow in performance_test.go
	// Here we just do a simple sanity check
	flags := &types.Flags{
		ExtractPrompt: "@prompt.txt",
	}

	resolved, err := cmd.LoadAndResolvePrompts(flags)
	if err != nil {
		t.Fatalf("LoadAndResolvePrompts failed: %v", err)
	}

	variables := config.Variables{
		URL:         "https://example.com",
		Topics:      []string{"AI", "ML"},
		ContentType: "article",
	}

	_, err = cmd.SubstituteVariables(resolved, variables)
	if err != nil {
		t.Fatalf("SubstituteVariables failed: %v", err)
	}

	// Performance assertion is in benchmark tests
}

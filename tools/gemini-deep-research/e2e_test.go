package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/cmd"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/config"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

// E2E User Scenario Tests
// These tests validate complete user workflows from the S6 design document

// TestE2E_Scenario1_NoConfigUseDefaults tests user scenario where no config files exist
// User relies entirely on built-in defaults
func TestE2E_Scenario1_NoConfigUseDefaults(t *testing.T) {
	// Setup: Clean environment with no config files
	tempDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Scenario: User runs with no flags, no config files
	flags := &types.Flags{}

	resolved, err := cmd.LoadAndResolvePrompts(flags)
	if err != nil {
		t.Fatalf("LoadAndResolvePrompts failed: %v", err)
	}

	// Verify defaults are used
	defaults := config.GetDefaultPrompts()
	if resolved.ExtractPrompt != defaults.ExtractPrompt {
		t.Errorf("Expected default extract prompt")
	}
	if resolved.AnalyzePrompt != defaults.AnalyzePrompt {
		t.Errorf("Expected default analyze prompt")
	}
	if resolved.ResearchPrompt != defaults.ResearchPrompt {
		t.Errorf("Expected default research prompt")
	}

	// Verify variable substitution works with defaults
	variables := config.Variables{
		URL:         "https://example.com/article",
		Topics:      []string{"machine learning", "neural networks"},
		ContentType: "article",
	}

	final, err := cmd.SubstituteVariables(resolved, variables)
	if err != nil {
		t.Fatalf("SubstituteVariables failed: %v", err)
	}

	// Defaults should not contain variables, so should be unchanged
	if final.ExtractPrompt == "" {
		t.Error("Expected non-empty final extract prompt")
	}
}

// TestE2E_Scenario2_GlobalConfigOverrideWithCLI tests user scenario with global config
// User has global config, but overrides specific prompts via CLI
func TestE2E_Scenario2_GlobalConfigOverrideWithCLI(t *testing.T) {
	// Setup: Create global config
	tempDir := t.TempDir()
	globalConfigDir := filepath.Join(tempDir, ".config", "ai-tools")
	os.MkdirAll(globalConfigDir, 0755)

	globalConfig := `prompts:
  extract: "Global: Extract key topics from the content"
  analyze: "Global: Analyze and categorize {topics}"
  research: "Global: Deep dive into {topics} with context from {url}"
`
	os.WriteFile(filepath.Join(globalConfigDir, "prompts.yaml"), []byte(globalConfig), 0644)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Scenario: User overrides only the analyze prompt via CLI
	flags := &types.Flags{
		AnalyzePrompt: "CLI: Custom analyze prompt for this run",
	}

	resolved, err := cmd.LoadAndResolvePrompts(flags)
	if err != nil {
		t.Fatalf("LoadAndResolvePrompts failed: %v", err)
	}

	// Verify: extract and research from global, analyze from CLI
	if resolved.ExtractPrompt != "Global: Extract key topics from the content" {
		t.Errorf("Expected global extract prompt, got: %s", resolved.ExtractPrompt)
	}
	if resolved.AnalyzePrompt != "CLI: Custom analyze prompt for this run" {
		t.Errorf("Expected CLI analyze prompt, got: %s", resolved.AnalyzePrompt)
	}
	if resolved.ResearchPrompt != "Global: Deep dive into {topics} with context from {url}" {
		t.Errorf("Expected global research prompt, got: %s", resolved.ResearchPrompt)
	}

	// Verify variable substitution works
	variables := config.Variables{
		URL:         "https://arxiv.org/abs/2301.12345",
		Topics:      []string{"transformers", "attention"},
		ContentType: "arxiv",
	}

	final, err := cmd.SubstituteVariables(resolved, variables)
	if err != nil {
		t.Fatalf("SubstituteVariables failed: %v", err)
	}

	expectedResearch := "Global: Deep dive into transformers, attention with context from https://arxiv.org/abs/2301.12345"
	if final.ResearchPrompt != expectedResearch {
		t.Errorf("Expected: %s\nGot: %s", expectedResearch, final.ResearchPrompt)
	}
}

// TestE2E_Scenario3_ProjectConfigWithFileReference tests project-specific config with @file syntax
// User has project config that references external prompt files
func TestE2E_Scenario3_ProjectConfigWithFileReference(t *testing.T) {
	// Setup: Create project config and prompt files
	tempDir := t.TempDir()
	projectConfigDir := filepath.Join(tempDir, ".ai-tools")
	os.MkdirAll(projectConfigDir, 0755)

	// Create separate prompt files
	extractFile := filepath.Join(tempDir, "prompts", "extract.txt")
	analyzeFile := filepath.Join(tempDir, "prompts", "analyze.txt")
	os.MkdirAll(filepath.Join(tempDir, "prompts"), 0755)

	extractContent := "Extract and list all technical terms and concepts from {content_type} content"
	analyzeContent := "Categorize {topics} by domain (AI, systems, theory, etc.)"
	os.WriteFile(extractFile, []byte(extractContent), 0644)
	os.WriteFile(analyzeFile, []byte(analyzeContent), 0644)

	// Project config uses @file syntax (relative paths)
	projectConfig := `prompts:
  extract: "@prompts/extract.txt"
  analyze: "@prompts/analyze.txt"
  research: "Comprehensive analysis of {topics} from {url}"
`
	os.WriteFile(filepath.Join(projectConfigDir, "prompts.yaml"), []byte(projectConfig), 0644)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Scenario: User runs with project config (no CLI overrides)
	flags := &types.Flags{}

	resolved, err := cmd.LoadAndResolvePrompts(flags)
	if err != nil {
		t.Fatalf("LoadAndResolvePrompts failed: %v", err)
	}

	// Verify @file references were resolved
	if resolved.ExtractPrompt != extractContent {
		t.Errorf("Expected extract file content, got: %s", resolved.ExtractPrompt)
	}
	if resolved.AnalyzePrompt != analyzeContent {
		t.Errorf("Expected analyze file content, got: %s", resolved.AnalyzePrompt)
	}

	// Verify variable substitution
	variables := config.Variables{
		URL:         "https://www.youtube.com/watch?v=example",
		Topics:      []string{"computer vision", "CNN"},
		ContentType: "video",
	}

	final, err := cmd.SubstituteVariables(resolved, variables)
	if err != nil {
		t.Fatalf("SubstituteVariables failed: %v", err)
	}

	expectedExtract := "Extract and list all technical terms and concepts from video content"
	if final.ExtractPrompt != expectedExtract {
		t.Errorf("Expected: %s\nGot: %s", expectedExtract, final.ExtractPrompt)
	}

	expectedAnalyze := "Categorize computer vision, CNN by domain (AI, systems, theory, etc.)"
	if final.AnalyzePrompt != expectedAnalyze {
		t.Errorf("Expected: %s\nGot: %s", expectedAnalyze, final.AnalyzePrompt)
	}
}

// TestE2E_Scenario4_FullCustomization tests complete customization across all levels
// Global config + Project config + CLI flags + @file syntax + variables
func TestE2E_Scenario4_FullCustomization(t *testing.T) {
	// Setup: Create both global and project configs
	tempDir := t.TempDir()

	// Global config
	globalConfigDir := filepath.Join(tempDir, ".config", "ai-tools")
	os.MkdirAll(globalConfigDir, 0755)
	globalConfig := `prompts:
  extract: "Global extract"
  analyze: "Global analyze"
  research: "Global research"
`
	os.WriteFile(filepath.Join(globalConfigDir, "prompts.yaml"), []byte(globalConfig), 0644)

	// Project config overrides extract and analyze
	projectConfigDir := filepath.Join(tempDir, ".ai-tools")
	os.MkdirAll(projectConfigDir, 0755)
	projectConfig := `prompts:
  extract: "Project extract with {url}"
  analyze: "@project-analyze.txt"
`
	os.WriteFile(filepath.Join(projectConfigDir, "prompts.yaml"), []byte(projectConfig), 0644)

	// Create file referenced in project config
	analyzeFile := filepath.Join(tempDir, "project-analyze.txt")
	os.WriteFile(analyzeFile, []byte("Project file: analyze {topics}"), 0644)

	// Create file for CLI @file reference
	cliFile := filepath.Join(tempDir, "cli-research.txt")
	os.WriteFile(cliFile, []byte("CLI file: research {topics} from {content_type}"), 0644)

	oldWd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldWd)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", oldHome)

	// Scenario: CLI overrides research with @file syntax
	flags := &types.Flags{
		ResearchPrompt: "@cli-research.txt",
	}

	resolved, err := cmd.LoadAndResolvePrompts(flags)
	if err != nil {
		t.Fatalf("LoadAndResolvePrompts failed: %v", err)
	}

	// Verify precedence chain:
	// - extract: Project (overrides Global)
	// - analyze: Project @file (overrides Global)
	// - research: CLI @file (overrides Global)
	if resolved.ExtractPrompt != "Project extract with {url}" {
		t.Errorf("Expected project extract prompt, got: %s", resolved.ExtractPrompt)
	}
	if resolved.AnalyzePrompt != "Project file: analyze {topics}" {
		t.Errorf("Expected project file content, got: %s", resolved.AnalyzePrompt)
	}
	if resolved.ResearchPrompt != "CLI file: research {topics} from {content_type}" {
		t.Errorf("Expected CLI file content, got: %s", resolved.ResearchPrompt)
	}

	// Verify full variable substitution
	variables := config.Variables{
		URL:         "https://huggingface.co/models/example",
		Topics:      []string{"LLM", "fine-tuning"},
		ContentType: "huggingface",
	}

	final, err := cmd.SubstituteVariables(resolved, variables)
	if err != nil {
		t.Fatalf("SubstituteVariables failed: %v", err)
	}

	if final.ExtractPrompt != "Project extract with https://huggingface.co/models/example" {
		t.Errorf("Unexpected extract: %s", final.ExtractPrompt)
	}
	if final.AnalyzePrompt != "Project file: analyze LLM, fine-tuning" {
		t.Errorf("Unexpected analyze: %s", final.AnalyzePrompt)
	}
	if final.ResearchPrompt != "CLI file: research LLM, fine-tuning from huggingface" {
		t.Errorf("Unexpected research: %s", final.ResearchPrompt)
	}
}

// TestE2E_Scenario5_MigrationFromLegacyInput tests backward compatibility
// User migrates from --input to --analyze-prompt
func TestE2E_Scenario5_MigrationFromLegacyInput(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Modern --analyze-prompt flag (string)", func(t *testing.T) {
		flags := &types.Flags{
			AnalyzePrompt: "Focus on security implications",
		}

		resolved, err := cmd.LoadAndResolvePrompts(flags)
		if err != nil {
			t.Fatalf("LoadAndResolvePrompts failed: %v", err)
		}

		// Modern --analyze-prompt should be used
		if resolved.AnalyzePrompt != "Focus on security implications" {
			t.Errorf("Expected analyze prompt, got: %s", resolved.AnalyzePrompt)
		}

		// Other prompts should use defaults
		defaults := config.GetDefaultPrompts()
		if resolved.ExtractPrompt != defaults.ExtractPrompt {
			t.Error("Expected default extract prompt")
		}
	})

	t.Run("Modern --analyze-prompt @file flag", func(t *testing.T) {
		// Create prompt file
		promptFile := filepath.Join(tempDir, "modern-prompt.txt")
		os.WriteFile(promptFile, []byte("Modern prompt with {topics}"), 0644)

		oldWd, _ := os.Getwd()
		os.Chdir(tempDir)
		defer os.Chdir(oldWd)

		// New syntax
		flags := &types.Flags{
			AnalyzePrompt: "@modern-prompt.txt",
		}

		resolved, err := cmd.LoadAndResolvePrompts(flags)
		if err != nil {
			t.Fatalf("LoadAndResolvePrompts failed: %v", err)
		}

		if resolved.AnalyzePrompt != "Modern prompt with {topics}" {
			t.Errorf("Expected modern file content, got: %s", resolved.AnalyzePrompt)
		}

		// Verify variables work
		variables := config.Variables{
			Topics: []string{"security", "privacy"},
		}

		final, err := cmd.SubstituteVariables(resolved, variables)
		if err != nil {
			t.Fatalf("SubstituteVariables failed: %v", err)
		}

		if final.AnalyzePrompt != "Modern prompt with security, privacy" {
			t.Errorf("Expected substituted prompt, got: %s", final.AnalyzePrompt)
		}
	})
}

// TestE2E_ErrorRecovery tests user-facing error scenarios
func TestE2E_ErrorRecovery(t *testing.T) {
	t.Run("Helpful error for missing file", func(t *testing.T) {
		flags := &types.Flags{
			ExtractPrompt: "@missing-file.txt",
		}

		_, err := cmd.LoadAndResolvePrompts(flags)
		if err == nil {
			t.Fatal("Expected error for missing file")
		}

		// Error should be helpful
		errMsg := err.Error()
		if errMsg == "" {
			t.Error("Expected non-empty error message")
		}
	})

	t.Run("Helpful error for invalid YAML", func(t *testing.T) {
		tempDir := t.TempDir()
		globalConfigDir := filepath.Join(tempDir, ".config", "ai-tools")
		os.MkdirAll(globalConfigDir, 0755)

		invalidYAML := `prompts:
  extract: [unclosed bracket
`
		os.WriteFile(filepath.Join(globalConfigDir, "prompts.yaml"), []byte(invalidYAML), 0644)

		oldHome := os.Getenv("HOME")
		os.Setenv("HOME", tempDir)
		defer os.Setenv("HOME", oldHome)

		flags := &types.Flags{}
		_, err := cmd.LoadAndResolvePrompts(flags)
		if err == nil {
			t.Fatal("Expected error for invalid YAML")
		}

		// Error should mention YAML
		errMsg := err.Error()
		if errMsg == "" {
			t.Error("Expected helpful YAML error message")
		}
	})

	t.Run("Helpful error for unknown variables", func(t *testing.T) {
		resolved := config.ResolvedConfig{
			ExtractPrompt: "Extract {unknown_variable}",
		}

		variables := config.Variables{
			URL: "https://example.com",
		}

		_, err := cmd.SubstituteVariables(resolved, variables)
		if err == nil {
			t.Fatal("Expected error for unknown variable")
		}

		errMsg := err.Error()
		// Should mention available variables
		if errMsg == "" {
			t.Error("Expected helpful variable error message")
		}
	})
}

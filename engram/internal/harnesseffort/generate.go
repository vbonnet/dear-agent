package harnesseffort

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Generate runs the full config generation pipeline:
// 1. Load embedded defaults
// 2. Load and merge optional override files (company, user)
// 3. Resolve model aliases
// 4. Generate output for each target harness
//
// Returns the list of files to write and the Gemini alias suggestion string.
// In dry-run mode, no files are written; callers should print the content instead.
func Generate(opts GenerateOpts) ([]OutputFile, string, error) {
	// Step 1: Load defaults
	cfg, err := LoadDefaults()
	if err != nil {
		return nil, "", fmt.Errorf("loading defaults: %w", err)
	}

	// Step 2: Load and merge override files
	overridePaths := []string{
		filepath.Join(os.Getenv("HOME"), ".config", "company", "engram", "harness-effort.yaml"),
		filepath.Join(os.Getenv("HOME"), ".config", "engram", "harness-effort.yaml"),
	}
	for _, path := range overridePaths {
		override, err := loadOverrideFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("loading override %s: %w", path, err)
		}
		cfg = MergeConfigs(cfg, override)
	}

	// Step 3: Resolve aliases
	cfg = ResolveAliases(cfg)

	var outputs []OutputFile
	geminiSuggestions := ""

	// Step 4: Generate per harness
	if opts.Harness == "" || opts.Harness == "codex" {
		out, err := generateCodexOutput(cfg)
		if err != nil {
			return nil, "", err
		}
		if out != nil {
			outputs = append(outputs, *out)
		}
	}

	if opts.Harness == "" || opts.Harness == "opencode" {
		out, err := generateOpenCodeOutput(cfg, opts)
		if err != nil {
			return nil, "", err
		}
		if out != nil {
			outputs = append(outputs, *out)
		}
	}

	if opts.Harness == "" || opts.Harness == "gemini" {
		geminiSuggestions = GenerateGemini(cfg)
	}

	return outputs, geminiSuggestions, nil
}

// loadOverrideFile reads a YAML override file. Returns empty config if file does not exist.
func loadOverrideFile(path string) (HarnessEffortConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return HarnessEffortConfig{}, nil
		}
		return HarnessEffortConfig{}, err
	}
	var cfg HarnessEffortConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return HarnessEffortConfig{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

func generateCodexOutput(cfg HarnessEffortConfig) (*OutputFile, error) {
	codexPath := filepath.Join(os.Getenv("HOME"), ".codex", "config.toml")
	existingContent := ""
	data, err := os.ReadFile(codexPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading %s: %w", codexPath, err)
	}
	if err == nil {
		existingContent = string(data)
	}

	content, err := GenerateCodex(cfg, existingContent)
	if err != nil {
		return nil, err
	}
	return &OutputFile{
		Path:    codexPath,
		Content: []byte(content),
	}, nil
}

func generateOpenCodeOutput(cfg HarnessEffortConfig, opts GenerateOpts) (*OutputFile, error) {
	dir := opts.OpenCodeDir
	if dir == "" {
		dir = "."
	}
	opencodePath := filepath.Join(dir, "opencode.json")
	existingJSON := ""
	data, err := os.ReadFile(opencodePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading %s: %w", opencodePath, err)
	}
	if err == nil {
		existingJSON = string(data)
	}

	content, err := GenerateOpenCode(cfg, existingJSON)
	if err != nil {
		return nil, err
	}
	return &OutputFile{
		Path:    opencodePath,
		Content: content,
	}, nil
}

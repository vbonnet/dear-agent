package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDearAgentConfig(t *testing.T) {
	dir := t.TempDir()
	content := `version: 1
repos:
  - ./api
  - ./web
goals:
  api: "Backend REST API"
  web: "Frontend React app"
review_gates:
  feature:
    required: true
    reviewers: 2
    check_suites:
      - ci/build
      - ci/test
  docs:
    required: false
    reviewers: 0
`
	if err := os.WriteFile(filepath.Join(dir, dearAgentConfigFile), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	config, err := LoadDearAgentConfig(dir)
	if err != nil {
		t.Fatalf("LoadDearAgentConfig failed: %v", err)
	}

	if config.Version != 1 {
		t.Errorf("expected version 1, got %d", config.Version)
	}
	if len(config.Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(config.Repos))
	}
	if config.Repos[0] != "./api" {
		t.Errorf("expected repo './api', got '%s'", config.Repos[0])
	}
	if config.Goals["api"] != "Backend REST API" {
		t.Errorf("expected goal 'Backend REST API', got '%s'", config.Goals["api"])
	}

	gate, ok := config.ReviewGates["feature"]
	if !ok {
		t.Fatal("expected 'feature' review gate")
	}
	if !gate.Required {
		t.Error("expected feature gate to be required")
	}
	if gate.Reviewers != 2 {
		t.Errorf("expected 2 reviewers, got %d", gate.Reviewers)
	}
	if len(gate.CheckSuites) != 2 {
		t.Errorf("expected 2 check suites, got %d", len(gate.CheckSuites))
	}

	docsGate, ok := config.ReviewGates["docs"]
	if !ok {
		t.Fatal("expected 'docs' review gate")
	}
	if docsGate.Required {
		t.Error("expected docs gate to not be required")
	}
}

func TestLoadDearAgentConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadDearAgentConfig(dir)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadDearAgentConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dearAgentConfigFile), []byte("{{invalid"), 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	_, err := LoadDearAgentConfig(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadDearAgentConfig_DefaultVersion(t *testing.T) {
	dir := t.TempDir()
	content := `repos:
  - ./app
`
	if err := os.WriteFile(filepath.Join(dir, dearAgentConfigFile), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	config, err := LoadDearAgentConfig(dir)
	if err != nil {
		t.Fatalf("LoadDearAgentConfig failed: %v", err)
	}
	if config.Version != 1 {
		t.Errorf("expected default version 1, got %d", config.Version)
	}
}

func TestWriteDearAgentConfig(t *testing.T) {
	dir := t.TempDir()
	config := &DearAgentWorkspaceConfig{
		Version: 1,
		Repos:   []string{"./svc-a", "./svc-b"},
		Goals: map[string]string{
			"svc-a": "Service A",
		},
		ReviewGates: map[string]ReviewGateConfig{
			"feature": {Required: true, Reviewers: 1, CheckSuites: []string{"ci/test"}},
		},
	}

	if err := WriteDearAgentConfig(dir, config); err != nil {
		t.Fatalf("WriteDearAgentConfig failed: %v", err)
	}

	// Verify file exists and can be loaded back
	loaded, err := LoadDearAgentConfig(dir)
	if err != nil {
		t.Fatalf("failed to load written config: %v", err)
	}
	if len(loaded.Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(loaded.Repos))
	}
	if loaded.Goals["svc-a"] != "Service A" {
		t.Errorf("expected goal 'Service A', got '%s'", loaded.Goals["svc-a"])
	}
	gate := loaded.ReviewGates["feature"]
	if !gate.Required || gate.Reviewers != 1 {
		t.Errorf("review gate mismatch: required=%v reviewers=%d", gate.Required, gate.Reviewers)
	}
}

func TestWriteDearAgentConfig_HasHeader(t *testing.T) {
	dir := t.TempDir()
	config := templateDearAgentConfig()

	if err := WriteDearAgentConfig(dir, config); err != nil {
		t.Fatalf("WriteDearAgentConfig failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, dearAgentConfigFile))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	content := string(data)
	if content[:1] != "#" {
		t.Error("expected file to start with comment header")
	}
}

func TestTemplateDearAgentConfig(t *testing.T) {
	config := templateDearAgentConfig()

	if config.Version != 1 {
		t.Errorf("expected version 1, got %d", config.Version)
	}
	if len(config.Repos) == 0 {
		t.Error("expected non-empty repos in template")
	}
	if len(config.Goals) == 0 {
		t.Error("expected non-empty goals in template")
	}
	if len(config.ReviewGates) == 0 {
		t.Error("expected non-empty review gates in template")
	}

	// Check that feature gate exists and has check suites
	gate, ok := config.ReviewGates["feature"]
	if !ok {
		t.Fatal("expected 'feature' review gate in template")
	}
	if !gate.Required {
		t.Error("expected feature gate to be required in template")
	}
	if len(gate.CheckSuites) == 0 {
		t.Error("expected check suites in feature gate template")
	}
}

func TestRunWorkspaceInit(t *testing.T) {
	dir := t.TempDir()
	workspaceInitDir = dir

	err := runWorkspaceInit(workspaceInitCmd, nil)
	if err != nil {
		t.Fatalf("runWorkspaceInit failed: %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(dir, dearAgentConfigFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("expected config file to be created")
	}

	// Verify it loads correctly
	config, err := LoadDearAgentConfig(dir)
	if err != nil {
		t.Fatalf("failed to load created config: %v", err)
	}
	if config.Version != 1 {
		t.Errorf("expected version 1, got %d", config.Version)
	}
}

func TestRunWorkspaceInit_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	// Create existing config
	if err := os.WriteFile(filepath.Join(dir, dearAgentConfigFile), []byte("version: 1"), 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	workspaceInitDir = dir
	err := runWorkspaceInit(workspaceInitCmd, nil)
	if err == nil {
		t.Fatal("expected error when config already exists")
	}
}

func TestRunWorkspaceConfigShow(t *testing.T) {
	dir := t.TempDir()
	config := &DearAgentWorkspaceConfig{
		Version: 1,
		Repos:   []string{"./api"},
		Goals:   map[string]string{"api": "REST API"},
		ReviewGates: map[string]ReviewGateConfig{
			"feature": {Required: true, Reviewers: 1},
		},
	}
	if err := WriteDearAgentConfig(dir, config); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	workspaceConfigShowDir = dir
	err := runWorkspaceConfigShow(workspaceConfigShowCmd, nil)
	if err != nil {
		t.Fatalf("runWorkspaceConfigShow failed: %v", err)
	}
}

func TestRunWorkspaceConfigShow_NotFound(t *testing.T) {
	dir := t.TempDir()
	workspaceConfigShowDir = dir

	err := runWorkspaceConfigShow(workspaceConfigShowCmd, nil)
	if err == nil {
		t.Fatal("expected error when config not found")
	}
}

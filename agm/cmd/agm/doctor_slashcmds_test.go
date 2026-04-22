package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Command registration tests
// ---------------------------------------------------------------------------

func TestDoctorSlashCmdsCmd_Registration(t *testing.T) {
	found := false
	for _, cmd := range doctorCmd.Commands() {
		if cmd.Use == "slash-commands" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected slash-commands to be registered under doctor command")
	}
}

func TestDoctorSlashCmdsCmd_Use(t *testing.T) {
	if doctorSlashCmdsCmd.Use != "slash-commands" {
		t.Errorf("Expected Use 'slash-commands', got %q", doctorSlashCmdsCmd.Use)
	}
}

func TestDoctorSlashCmdsCmd_Short(t *testing.T) {
	if doctorSlashCmdsCmd.Short == "" {
		t.Error("Expected non-empty Short description")
	}
}

func TestDoctorSlashCmdsCmd_RunE(t *testing.T) {
	if doctorSlashCmdsCmd.RunE == nil {
		t.Error("Expected RunE to be set")
	}
}

// ---------------------------------------------------------------------------
// scanSlashCmdDir tests
// ---------------------------------------------------------------------------

func TestScanSlashCmdDir_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	report := &slashCmdReport{}
	scanSlashCmdDir(tmpDir, "test", report)

	if report.Total != 0 {
		t.Errorf("Expected 0 total commands, got %d", report.Total)
	}
	if report.Healthy != 0 {
		t.Errorf("Expected 0 healthy, got %d", report.Healthy)
	}
	if report.Errors != 0 {
		t.Errorf("Expected 0 errors, got %d", report.Errors)
	}
}

func TestScanSlashCmdDir_NonexistentDir(t *testing.T) {
	report := &slashCmdReport{}
	scanSlashCmdDir("/nonexistent/path", "test", report)

	if report.Total != 0 {
		t.Errorf("Expected 0 total for nonexistent dir, got %d", report.Total)
	}
}

func TestScanSlashCmdDir_ValidCommandFile(t *testing.T) {
	tmpDir := t.TempDir()

	content := `---
description: A helpful command
allowed-tools:
  - Read
---
Do something helpful.
`
	if err := os.WriteFile(filepath.Join(tmpDir, "my-cmd.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	report := &slashCmdReport{}
	scanSlashCmdDir(tmpDir, "user", report)

	if report.Total != 1 {
		t.Fatalf("Expected 1 total, got %d", report.Total)
	}
	if report.Healthy != 1 {
		t.Errorf("Expected 1 healthy, got %d", report.Healthy)
	}
	if report.Errors != 0 {
		t.Errorf("Expected 0 errors, got %d", report.Errors)
	}

	r := report.Results[0]
	if r.Name != "my-cmd" {
		t.Errorf("Expected name 'my-cmd', got %q", r.Name)
	}
	if r.Source != "user" {
		t.Errorf("Expected source 'user', got %q", r.Source)
	}
	if r.Description != "A helpful command" {
		t.Errorf("Expected description 'A helpful command', got %q", r.Description)
	}
}

func TestScanSlashCmdDir_MalformedFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()

	// Unclosed frontmatter
	content := `---
description: oops
this is not closed
`
	if err := os.WriteFile(filepath.Join(tmpDir, "bad.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	report := &slashCmdReport{}
	scanSlashCmdDir(tmpDir, "user", report)

	if report.Total != 1 {
		t.Fatalf("Expected 1 total, got %d", report.Total)
	}
	if report.Errors != 1 {
		t.Errorf("Expected 1 error, got %d", report.Errors)
	}
	if report.Healthy != 0 {
		t.Errorf("Expected 0 healthy, got %d", report.Healthy)
	}

	r := report.Results[0]
	if r.Error == "" {
		t.Error("Expected non-empty error message")
	}
}

func TestScanSlashCmdDir_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	content := `---
: [invalid yaml {{
---
Body text.
`
	if err := os.WriteFile(filepath.Join(tmpDir, "invalid.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	report := &slashCmdReport{}
	scanSlashCmdDir(tmpDir, "user", report)

	if report.Total != 1 {
		t.Fatalf("Expected 1 total, got %d", report.Total)
	}
	if report.Errors != 1 {
		t.Errorf("Expected 1 error, got %d", report.Errors)
	}
}

func TestScanSlashCmdDir_FrontmatterMissingDescription(t *testing.T) {
	tmpDir := t.TempDir()

	content := `---
name: no-desc
allowed-tools:
  - Read
---
Missing description field.
`
	if err := os.WriteFile(filepath.Join(tmpDir, "no-desc.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	report := &slashCmdReport{}
	scanSlashCmdDir(tmpDir, "user", report)

	if report.Total != 1 {
		t.Fatalf("Expected 1 total, got %d", report.Total)
	}
	if report.Errors != 1 {
		t.Errorf("Expected 1 error for missing description, got %d", report.Errors)
	}
}

func TestScanSlashCmdDir_NoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()

	// Plain markdown with no frontmatter is valid
	content := `Just a plain command with instructions.
`
	if err := os.WriteFile(filepath.Join(tmpDir, "plain.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	report := &slashCmdReport{}
	scanSlashCmdDir(tmpDir, "user", report)

	if report.Total != 1 {
		t.Fatalf("Expected 1 total, got %d", report.Total)
	}
	if report.Healthy != 1 {
		t.Errorf("Expected 1 healthy (no frontmatter is ok), got %d", report.Healthy)
	}
}

func TestScanSlashCmdDir_SkipsNonMdFiles(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("not a command"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte("key: val"), 0644); err != nil {
		t.Fatal(err)
	}

	report := &slashCmdReport{}
	scanSlashCmdDir(tmpDir, "user", report)

	if report.Total != 0 {
		t.Errorf("Expected 0 total (non-.md files skipped), got %d", report.Total)
	}
}

func TestScanSlashCmdDir_SkipsSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	subDir := filepath.Join(tmpDir, "subdir.md")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	report := &slashCmdReport{}
	scanSlashCmdDir(tmpDir, "user", report)

	if report.Total != 0 {
		t.Errorf("Expected 0 total (directories skipped), got %d", report.Total)
	}
}

// ---------------------------------------------------------------------------
// parseSlashCmdFrontmatter tests
// ---------------------------------------------------------------------------

func TestParseSlashCmdFrontmatter_Valid(t *testing.T) {
	content := []byte("---\ndescription: test command\n---\nbody")
	fm, err := parseSlashCmdFrontmatter(content)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if fm["description"] != "test command" {
		t.Errorf("Expected description 'test command', got %v", fm["description"])
	}
}

func TestParseSlashCmdFrontmatter_NoFrontmatter(t *testing.T) {
	content := []byte("Just markdown content.")
	fm, err := parseSlashCmdFrontmatter(content)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if fm != nil {
		t.Errorf("Expected nil map for no frontmatter, got %v", fm)
	}
}

func TestParseSlashCmdFrontmatter_Unclosed(t *testing.T) {
	content := []byte("---\ndescription: bad\n")
	_, err := parseSlashCmdFrontmatter(content)
	if err == nil {
		t.Error("Expected error for unclosed frontmatter")
	}
}

func TestParseSlashCmdFrontmatter_MalformedYAML(t *testing.T) {
	content := []byte("---\n: [broken {{\n---\nbody")
	_, err := parseSlashCmdFrontmatter(content)
	if err == nil {
		t.Error("Expected error for malformed YAML")
	}
}

// ---------------------------------------------------------------------------
// resolvePluginCommandDirs tests
// ---------------------------------------------------------------------------

func TestResolvePluginCommandDirs_NoSettingsFile(t *testing.T) {
	dirs := resolvePluginCommandDirs("/nonexistent/settings.json", "/home/test")
	if len(dirs) != 0 {
		t.Errorf("Expected empty map for missing settings, got %v", dirs)
	}
}

func TestResolvePluginCommandDirs_WithPlugin(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a plugin with commands/ directory
	pluginDir := filepath.Join(tmpDir, "my-plugin")
	cmdsDir := filepath.Join(pluginDir, "commands")
	if err := os.MkdirAll(cmdsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create settings.json referencing the plugin
	settings := map[string]interface{}{
		"enabledPlugins": []interface{}{pluginDir},
	}
	data, _ := json.Marshal(settings)
	settingsPath := filepath.Join(tmpDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	dirs := resolvePluginCommandDirs(settingsPath, tmpDir)
	if len(dirs) != 1 {
		t.Fatalf("Expected 1 plugin dir, got %d", len(dirs))
	}
	if dirs["my-plugin"] != cmdsDir {
		t.Errorf("Expected commands dir %q, got %q", cmdsDir, dirs["my-plugin"])
	}
}

func TestResolvePluginCommandDirs_PluginWithoutCommandsDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a plugin directory without commands/
	pluginDir := filepath.Join(tmpDir, "bare-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	settings := map[string]interface{}{
		"enabledPlugins": []interface{}{pluginDir},
	}
	data, _ := json.Marshal(settings)
	settingsPath := filepath.Join(tmpDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	dirs := resolvePluginCommandDirs(settingsPath, tmpDir)
	if len(dirs) != 0 {
		t.Errorf("Expected 0 dirs for plugin without commands/, got %d", len(dirs))
	}
}

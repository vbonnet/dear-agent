package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUpdatesAndThenVerifiesPluginHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "command.md")
	source := "---\ncontent-hash: PLACEHOLDER\n---\n\n# Command\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := run(dir, false); err != nil {
		t.Fatalf("update hashes: %v", err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	if strings.Contains(string(updated), "PLACEHOLDER") {
		t.Fatalf("hash was not updated:\n%s", updated)
	}
	if err := run(dir, true); err != nil {
		t.Fatalf("verify updated hash: %v", err)
	}
}

func TestRunCheckRejectsStaleHashWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "command.md")
	source := "---\ncontent-hash: 0000\n---\n\n# Command\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := run(dir, true); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale-hash error, got %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read checked file: %v", err)
	}
	if string(after) != source {
		t.Fatal("check mode modified the source")
	}
}

func TestRunRejectsCommandWithoutContentHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "command.md")
	if err := os.WriteFile(path, []byte("---\ndescription: missing hash\n---\n\n# Command\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := run(dir, true); err == nil || !strings.Contains(err.Error(), "has no content-hash field") {
		t.Fatalf("expected missing-hash error, got %v", err)
	}
}

func TestRunAllowsExplicitlyUnhashedSpec(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte("# Specification\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := run(dir, true); err != nil {
		t.Fatalf("verify directory containing SPEC.md: %v", err)
	}
}

func TestRunTargetsRecursesAcrossCommandAndSkillTrees(t *testing.T) {
	root := t.TempDir()
	commandDir := filepath.Join(root, "commands")
	skillDir := filepath.Join(root, "skills", "scan-health")
	for _, dir := range []string{commandDir, skillDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create fixture directory %s: %v", dir, err)
		}
	}
	for _, path := range []string{
		filepath.Join(commandDir, "agm-list.md"),
		filepath.Join(skillDir, "SKILL.md"),
	} {
		source := "---\ncontent-hash: PLACEHOLDER\n---\n\n# Plugin document\n"
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}

	if err := runTargets([]string{commandDir, filepath.Join(root, "skills")}, false); err != nil {
		t.Fatalf("update recursive targets: %v", err)
	}
	if err := runTargets([]string{commandDir, filepath.Join(root, "skills")}, true); err != nil {
		t.Fatalf("verify recursive targets: %v", err)
	}
}

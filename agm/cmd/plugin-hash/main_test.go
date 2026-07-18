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

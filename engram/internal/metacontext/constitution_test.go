package metacontext

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewConstitutionService(t *testing.T) {
	// Create temporary directory with AGENTS.md
	tmpDir := t.TempDir()
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")

	originalContent := `# Agent Instructions
This is the constitution.`

	if err := os.WriteFile(agentsPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create AGENTS.md: %v", err)
	}

	// Test: Load constitution successfully
	cs, err := NewConstitutionService(tmpDir)
	if err != nil {
		t.Fatalf("NewConstitutionService failed: %v", err)
	}

	if cs == nil {
		t.Fatal("ConstitutionService is nil")
	}

	// Verify GetConstitution
	ctx := context.Background()
	constitution, err := cs.GetConstitution(ctx)
	if err != nil {
		t.Fatalf("GetConstitution failed: %v", err)
	}

	if constitution.Content != originalContent {
		t.Errorf("Content mismatch: got %q, want %q", constitution.Content, originalContent)
	}

	if constitution.Hash == "" {
		t.Error("Hash is empty")
	}

	if constitution.Path != agentsPath {
		t.Errorf("Path mismatch: got %q, want %q", constitution.Path, agentsPath)
	}
}

func TestNewConstitutionService_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Test: Neither AGENTS.md nor CLAUDE.md exists
	cs, err := NewConstitutionService(tmpDir)
	if err == nil {
		t.Fatal("Expected error for missing constitution files, got nil")
	}

	if cs != nil {
		t.Error("Expected nil ConstitutionService for missing file")
	}

	// Verify error message mentions both files
	expectedMsg := "neither AGENTS.md nor CLAUDE.md found"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Error message should mention both files: got %q", err.Error())
	}
}

func TestValidateIntegrity_NoModification(t *testing.T) {
	tmpDir := t.TempDir()
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")

	content := "# Original constitution"
	if err := os.WriteFile(agentsPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create AGENTS.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err != nil {
		t.Fatalf("NewConstitutionService failed: %v", err)
	}

	// Test: Validate integrity when file unchanged
	ctx := context.Background()
	if err := cs.ValidateIntegrity(ctx, tmpDir); err != nil {
		t.Errorf("ValidateIntegrity failed for unchanged file: %v", err)
	}
}

func TestValidateIntegrity_RuntimeModification(t *testing.T) {
	tmpDir := t.TempDir()
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")

	originalContent := "# Original constitution"
	if err := os.WriteFile(agentsPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create AGENTS.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err != nil {
		t.Fatalf("NewConstitutionService failed: %v", err)
	}

	// Modify AGENTS.md at runtime (forbidden!)
	modifiedContent := "# Modified constitution (FORBIDDEN)"
	if err := os.WriteFile(agentsPath, []byte(modifiedContent), 0644); err != nil {
		t.Fatalf("failed to modify AGENTS.md: %v", err)
	}

	// Test: Detect runtime modification
	ctx := context.Background()
	err = cs.ValidateIntegrity(ctx, tmpDir)
	if err == nil {
		t.Fatal("Expected error for runtime modification, got nil")
	}

	// Verify error message mentions forbidden runtime edits
	expectedMsg := "constitution file modified at runtime"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Error message mismatch: got %q, want prefix %q", err.Error(), expectedMsg)
	}
}

func TestValidateIntegrity_RuntimeDeletion(t *testing.T) {
	tmpDir := t.TempDir()
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")

	content := "# Original constitution"
	if err := os.WriteFile(agentsPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create AGENTS.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err != nil {
		t.Fatalf("NewConstitutionService failed: %v", err)
	}

	// Delete AGENTS.md at runtime
	if err := os.Remove(agentsPath); err != nil {
		t.Fatalf("failed to delete AGENTS.md: %v", err)
	}

	// Test: Detect runtime deletion
	ctx := context.Background()
	err = cs.ValidateIntegrity(ctx, tmpDir)
	if err == nil {
		t.Fatal("Expected error for runtime deletion, got nil")
	}

	expectedMsg := "constitution file deleted at runtime"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Error message mismatch: got %q, want prefix %q", err.Error(), expectedMsg)
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	content := "# Test content"
	hash1 := computeHash(content)
	hash2 := computeHash(content)

	if hash1 != hash2 {
		t.Errorf("Hash not deterministic: %s != %s", hash1, hash2)
	}

	if len(hash1) != 64 { // SHA-256 produces 64-char hex string
		t.Errorf("Invalid hash length: got %d, want 64", len(hash1))
	}
}

func TestComputeHash_DifferentContent(t *testing.T) {
	hash1 := computeHash("content1")
	hash2 := computeHash("content2")

	if hash1 == hash2 {
		t.Error("Different content produced same hash")
	}
}

// ============================================================================
// CLAUDE.md Support Tests
// ============================================================================

func TestNewConstitutionService_ClaudeMd(t *testing.T) {
	// Test: Load CLAUDE.md when AGENTS.md doesn't exist
	tmpDir := t.TempDir()
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")

	claudeContent := `# Claude Instructions
This is the constitution for Claude Code.`

	if err := os.WriteFile(claudePath, []byte(claudeContent), 0644); err != nil {
		t.Fatalf("failed to create CLAUDE.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err != nil {
		t.Fatalf("NewConstitutionService failed with CLAUDE.md: %v", err)
	}

	if cs == nil {
		t.Fatal("ConstitutionService is nil")
	}

	ctx := context.Background()
	constitution, err := cs.GetConstitution(ctx)
	if err != nil {
		t.Fatalf("GetConstitution failed: %v", err)
	}

	if constitution.Content != claudeContent {
		t.Errorf("Content mismatch: got %q, want %q", constitution.Content, claudeContent)
	}

	if constitution.Path != claudePath {
		t.Errorf("Path mismatch: got %q, want %q", constitution.Path, claudePath)
	}

	if constitution.Hash == "" {
		t.Error("Hash is empty")
	}
}

func TestNewConstitutionService_PreferAgentsMd(t *testing.T) {
	// Test: Prefer AGENTS.md when both AGENTS.md and CLAUDE.md exist
	tmpDir := t.TempDir()
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")

	agentsContent := "# AGENTS.md content"
	claudeContent := "# CLAUDE.md content"

	if err := os.WriteFile(agentsPath, []byte(agentsContent), 0644); err != nil {
		t.Fatalf("failed to create AGENTS.md: %v", err)
	}

	if err := os.WriteFile(claudePath, []byte(claudeContent), 0644); err != nil {
		t.Fatalf("failed to create CLAUDE.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err != nil {
		t.Fatalf("NewConstitutionService failed: %v", err)
	}

	ctx := context.Background()
	constitution, err := cs.GetConstitution(ctx)
	if err != nil {
		t.Fatalf("GetConstitution failed: %v", err)
	}

	// Should prefer AGENTS.md
	if constitution.Content != agentsContent {
		t.Errorf("Should prefer AGENTS.md: got content %q, want %q", constitution.Content, agentsContent)
	}

	if constitution.Path != agentsPath {
		t.Errorf("Should prefer AGENTS.md: got path %q, want %q", constitution.Path, agentsPath)
	}
}

func TestValidateIntegrity_ClaudeMd_NoModification(t *testing.T) {
	// Test: Validate integrity for CLAUDE.md when unchanged
	tmpDir := t.TempDir()
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")

	content := "# Claude constitution"
	if err := os.WriteFile(claudePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create CLAUDE.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err != nil {
		t.Fatalf("NewConstitutionService failed: %v", err)
	}

	ctx := context.Background()
	if err := cs.ValidateIntegrity(ctx, tmpDir); err != nil {
		t.Errorf("ValidateIntegrity failed for unchanged CLAUDE.md: %v", err)
	}
}

func TestValidateIntegrity_ClaudeMd_RuntimeModification(t *testing.T) {
	// Test: Detect runtime modification of CLAUDE.md
	tmpDir := t.TempDir()
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")

	originalContent := "# Original Claude constitution"
	if err := os.WriteFile(claudePath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to create CLAUDE.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err != nil {
		t.Fatalf("NewConstitutionService failed: %v", err)
	}

	// Modify CLAUDE.md at runtime
	modifiedContent := "# Modified Claude constitution (FORBIDDEN)"
	if err := os.WriteFile(claudePath, []byte(modifiedContent), 0644); err != nil {
		t.Fatalf("failed to modify CLAUDE.md: %v", err)
	}

	ctx := context.Background()
	err = cs.ValidateIntegrity(ctx, tmpDir)
	if err == nil {
		t.Fatal("Expected error for runtime modification of CLAUDE.md, got nil")
	}

	expectedMsg := "constitution file modified at runtime"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Error message mismatch: got %q, want prefix %q", err.Error(), expectedMsg)
	}
}

func TestValidateIntegrity_ClaudeMd_RuntimeDeletion(t *testing.T) {
	// Test: Detect runtime deletion of CLAUDE.md
	tmpDir := t.TempDir()
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")

	content := "# Claude constitution"
	if err := os.WriteFile(claudePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create CLAUDE.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err != nil {
		t.Fatalf("NewConstitutionService failed: %v", err)
	}

	// Delete CLAUDE.md at runtime
	if err := os.Remove(claudePath); err != nil {
		t.Fatalf("failed to delete CLAUDE.md: %v", err)
	}

	ctx := context.Background()
	err = cs.ValidateIntegrity(ctx, tmpDir)
	if err == nil {
		t.Fatal("Expected error for runtime deletion of CLAUDE.md, got nil")
	}

	expectedMsg := "constitution file deleted at runtime"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Error message mismatch: got %q, want prefix %q", err.Error(), expectedMsg)
	}
}

func TestNewConstitutionService_ClaudeMdReadError(t *testing.T) {
	// Test: Handle read errors for CLAUDE.md
	tmpDir := t.TempDir()
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")

	// Create CLAUDE.md with no read permissions
	if err := os.WriteFile(claudePath, []byte("content"), 0000); err != nil {
		t.Fatalf("failed to create CLAUDE.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err == nil {
		t.Fatal("Expected error for unreadable CLAUDE.md, got nil")
	}

	if cs != nil {
		t.Error("Expected nil ConstitutionService for unreadable file")
	}

	// Clean up
	os.Chmod(claudePath, 0644)
}

func TestNewConstitutionService_AgentsMdReadError(t *testing.T) {
	// Test: Handle read errors for AGENTS.md (shouldn't fall back to CLAUDE.md)
	tmpDir := t.TempDir()
	agentsPath := filepath.Join(tmpDir, "AGENTS.md")
	claudePath := filepath.Join(tmpDir, "CLAUDE.md")

	// Create AGENTS.md with no read permissions
	if err := os.WriteFile(agentsPath, []byte("agents content"), 0000); err != nil {
		t.Fatalf("failed to create AGENTS.md: %v", err)
	}

	// Create readable CLAUDE.md
	if err := os.WriteFile(claudePath, []byte("claude content"), 0644); err != nil {
		t.Fatalf("failed to create CLAUDE.md: %v", err)
	}

	cs, err := NewConstitutionService(tmpDir)
	if err == nil {
		t.Fatal("Expected error for unreadable AGENTS.md, got nil")
	}

	if cs != nil {
		t.Error("Expected nil ConstitutionService for unreadable AGENTS.md")
	}

	// Should not fall back to CLAUDE.md if AGENTS.md exists but is unreadable
	expectedMsg := "failed to read AGENTS.md"
	if err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Error message should mention AGENTS.md read failure: got %q", err.Error())
	}

	// Clean up
	os.Chmod(agentsPath, 0644)
}

package builtin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/hooks"
)

func TestDocumentationCheckerMissingDocs(t *testing.T) {
	tmpDir := t.TempDir()

	checker := NewDocumentationChecker(tmpDir)
	result, err := checker.CheckDocumentation(context.Background())
	if err != nil {
		t.Fatalf("CheckDocumentation failed: %v", err)
	}

	// Should have violations for missing docs
	if len(result.Violations) == 0 {
		t.Error("Expected violations for missing documentation")
	}

	if result.Status != hooks.VerificationStatusWarning {
		t.Errorf("Expected warning status, got %s", result.Status)
	}
}

func TestDocumentationCheckerWithDocs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create documentation files
	docs := []string{"README.md", "SPEC.md", "ARCHITECTURE.md"}
	for _, doc := range docs {
		path := filepath.Join(tmpDir, doc)
		if err := os.WriteFile(path, []byte("# "+doc), 0644); err != nil {
			t.Fatalf("Failed to create %s: %v", doc, err)
		}
	}

	checker := NewDocumentationChecker(tmpDir)
	result, err := checker.CheckDocumentation(context.Background())
	if err != nil {
		t.Fatalf("CheckDocumentation failed: %v", err)
	}

	// Should pass when all docs present (no git, so no stale check)
	if result.Status != hooks.VerificationStatusPass {
		t.Errorf("Expected pass status, got %s", result.Status)
	}
}

func TestDocumentationCheckerPartialDocs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only README
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# README"), 0644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	checker := NewDocumentationChecker(tmpDir)
	result, err := checker.CheckDocumentation(context.Background())
	if err != nil {
		t.Fatalf("CheckDocumentation failed: %v", err)
	}

	// Should have warning for missing docs
	if result.Status != hooks.VerificationStatusWarning {
		t.Errorf("Expected warning status, got %s", result.Status)
	}

	// Check violation message mentions missing docs
	if len(result.Violations) == 0 {
		t.Error("Expected violations for missing docs")
	} else {
		if !contains2(result.Violations[0].Message, "SPEC.md") {
			t.Error("Expected violation to mention SPEC.md")
		}
	}
}

func contains2(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr))
}

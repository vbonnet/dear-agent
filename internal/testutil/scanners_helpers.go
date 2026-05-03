// Package testutil provides test helpers for scanners package testing.
// This file is part of B4.4 (Scanners Tests) sub-project.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// SetupTestRepo creates a temporary git repository for testing
func SetupTestRepo(t *testing.T, tmpdir string) {
	t.Helper()

	// Initialize git repo
	gitDir := filepath.Join(tmpdir, ".git")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	// Create minimal git objects directory
	objectsDir := filepath.Join(gitDir, "objects")
	if err := os.MkdirAll(objectsDir, 0o700); err != nil {
		t.Fatalf("failed to create git objects dir: %v", err)
	}

	// Create refs directory
	refsDir := filepath.Join(gitDir, "refs", "heads")
	if err := os.MkdirAll(refsDir, 0o700); err != nil {
		t.Fatalf("failed to create git refs dir: %v", err)
	}

	// Create HEAD file
	headContent := "ref: refs/heads/main\n"
	headPath := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headPath, []byte(headContent), 0o600); err != nil {
		t.Fatalf("failed to write HEAD file: %v", err)
	}
}

// CreatePackageJSON creates a test package.json file with dependencies
func CreatePackageJSON(t *testing.T, tmpdir string, deps map[string]string) {
	t.Helper()

	content := `{
  "name": "test-project",
  "version": "1.0.0",
  "dependencies": {`

	first := true
	for dep, version := range deps {
		if !first {
			content += ","
		}
		content += "\n    \"" + dep + "\": \"" + version + "\""
		first = false
	}

	content += `
  }
}`

	packageJSONPath := filepath.Join(tmpdir, "package.json")
	if err := os.WriteFile(packageJSONPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write package.json: %v", err)
	}
}

// CreateGoMod creates a test go.mod file with dependencies
func CreateGoMod(t *testing.T, tmpdir string, deps []string) {
	t.Helper()

	content := `module github.com/test/project

go 1.21

require (
`

	for _, dep := range deps {
		content += "\t" + dep + " v1.0.0\n"
	}

	content += ")\n"

	goModPath := filepath.Join(tmpdir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
}

// CreateRequirementsTxt creates a test requirements.txt file
func CreateRequirementsTxt(t *testing.T, tmpdir string, packages []string) {
	t.Helper()

	content := ""
	for _, pkg := range packages {
		content += pkg + "==1.0.0\n"
	}

	reqPath := filepath.Join(tmpdir, "requirements.txt")
	if err := os.WriteFile(reqPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write requirements.txt: %v", err)
	}
}

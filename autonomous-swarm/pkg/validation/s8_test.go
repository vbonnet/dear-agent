package validation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateS8_ValidFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid Go file
	goFile := filepath.Join(tmpDir, "valid.go")
	goContent := `package main

func main() {
	println("hello")
}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0644); err != nil {
		t.Fatalf("failed to create test Go file: %v", err)
	}

	// Create valid YAML file
	yamlFile := filepath.Join(tmpDir, "valid.yaml")
	yamlContent := `key: value
list:
  - item1
  - item2
`
	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to create test YAML file: %v", err)
	}

	// Validate
	result, err := ValidateS8([]string{goFile, yamlFile})
	if err != nil {
		t.Fatalf("ValidateS8 failed: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected valid result, got invalid: %v", result.Errors)
	}

	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}

	if len(result.Files) != 2 {
		t.Errorf("expected 2 files validated, got %d", len(result.Files))
	}
}

func TestValidateS8_InvalidGoSyntax(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid Go file (syntax error)
	goFile := filepath.Join(tmpDir, "invalid.go")
	goContent := `package main

func main() {
	println("unclosed string
}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0644); err != nil {
		t.Fatalf("failed to create test Go file: %v", err)
	}

	// Validate
	result, err := ValidateS8([]string{goFile})
	if err != nil {
		t.Fatalf("ValidateS8 failed: %v", err)
	}

	if result.Valid {
		t.Error("expected invalid result for syntax error")
	}

	if len(result.Errors) == 0 {
		t.Error("expected syntax error in results")
	}
}

func TestValidateS8_InvalidYAMLSyntax(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid YAML file
	yamlFile := filepath.Join(tmpDir, "invalid.yaml")
	yamlContent := `key: value
  invalid indentation:
    - item
`
	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to create test YAML file: %v", err)
	}

	// Validate
	result, err := ValidateS8([]string{yamlFile})
	if err != nil {
		t.Fatalf("ValidateS8 failed: %v", err)
	}

	if result.Valid {
		t.Error("expected invalid result for YAML syntax error")
	}

	if len(result.Errors) == 0 {
		t.Error("expected syntax error in results")
	}
}

func TestValidateS8_FileNotFound(t *testing.T) {
	// Validate nonexistent file
	result, err := ValidateS8([]string{"/nonexistent/file.go"})
	if err != nil {
		t.Fatalf("ValidateS8 failed: %v", err)
	}

	if result.Valid {
		t.Error("expected invalid result for missing file")
	}

	if len(result.Errors) == 0 {
		t.Error("expected file not found error")
	}

	if len(result.Errors) > 0 && !containsString(result.Errors[0], "file not found") {
		t.Errorf("expected 'file not found' error, got: %s", result.Errors[0])
	}
}

func TestValidateS8_MixedResults(t *testing.T) {
	tmpDir := t.TempDir()

	// Create one valid file
	validFile := filepath.Join(tmpDir, "valid.go")
	validContent := `package main
func main() {}
`
	if err := os.WriteFile(validFile, []byte(validContent), 0644); err != nil {
		t.Fatalf("failed to create valid file: %v", err)
	}

	// Create one invalid file
	invalidFile := filepath.Join(tmpDir, "invalid.go")
	invalidContent := `package main
func main() {
	// Missing closing brace
`
	if err := os.WriteFile(invalidFile, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to create invalid file: %v", err)
	}

	// Validate both
	result, err := ValidateS8([]string{validFile, invalidFile})
	if err != nil {
		t.Fatalf("ValidateS8 failed: %v", err)
	}

	if result.Valid {
		t.Error("expected invalid result when one file has errors")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}

	if len(result.Files) != 2 {
		t.Errorf("expected 2 files validated, got %d", len(result.Files))
	}
}

func TestValidateS8_UnknownFileType(t *testing.T) {
	tmpDir := t.TempDir()

	// Create file with unknown extension (should skip syntax validation)
	txtFile := filepath.Join(tmpDir, "readme.txt")
	txtContent := "This is a text file"
	if err := os.WriteFile(txtFile, []byte(txtContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Validate
	result, err := ValidateS8([]string{txtFile})
	if err != nil {
		t.Fatalf("ValidateS8 failed: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected valid result for unknown file type, got errors: %v", result.Errors)
	}

	if len(result.Files) != 1 {
		t.Errorf("expected 1 file validated, got %d", len(result.Files))
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

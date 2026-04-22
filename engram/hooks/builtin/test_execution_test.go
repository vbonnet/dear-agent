package builtin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/hooks"
)

func TestDetectFrameworks(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create Go test file
	goTestPath := filepath.Join(tmpDir, "example_test.go")
	if err := os.WriteFile(goTestPath, []byte("package example\n"), 0644); err != nil {
		t.Fatalf("Failed to create Go test file: %v", err)
	}

	executor := NewTestExecutor(tmpDir, 70.0)
	frameworks, err := executor.DetectFrameworks()
	if err != nil {
		t.Fatalf("DetectFrameworks failed: %v", err)
	}

	// Should detect Go framework
	found := false
	for _, fw := range frameworks {
		if fw == FrameworkGo {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to detect Go framework")
	}
}

func TestDetectFrameworksPython(t *testing.T) {
	tmpDir := t.TempDir()

	// Create Python test file
	pyTestPath := filepath.Join(tmpDir, "test_example.py")
	if err := os.WriteFile(pyTestPath, []byte("def test_example():\n    pass\n"), 0644); err != nil {
		t.Fatalf("Failed to create Python test file: %v", err)
	}

	executor := NewTestExecutor(tmpDir, 70.0)
	frameworks, err := executor.DetectFrameworks()
	if err != nil {
		t.Fatalf("DetectFrameworks failed: %v", err)
	}

	// Should detect Python framework
	found := false
	for _, fw := range frameworks {
		if fw == FrameworkPython {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to detect Python framework")
	}
}

func TestNewTestExecutorDefaultThreshold(t *testing.T) {
	executor := NewTestExecutor("/tmp", 0) // 0 should use default
	if executor.threshold != 70.0 {
		t.Errorf("Expected default threshold 70.0, got %f", executor.threshold)
	}
}

func TestNewTestExecutorCustomThreshold(t *testing.T) {
	executor := NewTestExecutor("/tmp", 85.0)
	if executor.threshold != 85.0 {
		t.Errorf("Expected threshold 85.0, got %f", executor.threshold)
	}
}

func TestRunTestsNoFrameworks(t *testing.T) {
	// Empty directory - no tests
	tmpDir := t.TempDir()

	executor := NewTestExecutor(tmpDir, 70.0)
	result, err := executor.RunTests(context.Background())
	if err != nil {
		t.Fatalf("RunTests failed: %v", err)
	}

	if result.Status != hooks.VerificationStatusWarning {
		t.Errorf("Expected warning status for no tests, got %s", result.Status)
	}

	if len(result.Violations) == 0 {
		t.Error("Expected violation for no tests")
	}
}

func TestRunGoTestsSuccess(t *testing.T) {
	// Create a temporary Go project with tests
	tmpDir := t.TempDir()

	// Create go.mod
	goMod := "module test\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	// Create simple Go file
	goFile := `package test

func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "math.go"), []byte(goFile), 0644); err != nil {
		t.Fatalf("Failed to create math.go: %v", err)
	}

	// Create test file
	testFile := `package test

import "testing"

func TestAdd(t *testing.T) {
	result := Add(2, 3)
	if result != 5 {
		t.Errorf("Expected 5, got %d", result)
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "math_test.go"), []byte(testFile), 0644); err != nil {
		t.Fatalf("Failed to create math_test.go: %v", err)
	}

	executor := NewTestExecutor(tmpDir, 50.0) // Low threshold for test
	coverage, err := executor.runGoTests(context.Background())
	if err != nil {
		t.Skipf("Skipping Go test execution (may not have Go installed or other env issue): %v", err)
	}

	if coverage < 0 || coverage > 100 {
		t.Errorf("Invalid coverage value: %f", coverage)
	}
}

func TestRunTestsGoProject(t *testing.T) {
	// Create a temporary Go project with tests
	tmpDir := t.TempDir()

	// Create go.mod
	goMod := "module test\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	// Create simple Go file
	goFile := `package test

func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "math.go"), []byte(goFile), 0644); err != nil {
		t.Fatalf("Failed to create math.go: %v", err)
	}

	// Create test file
	testFile := `package test

import "testing"

func TestAdd(t *testing.T) {
	result := Add(2, 3)
	if result != 5 {
		t.Errorf("Expected 5, got %d", result)
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "math_test.go"), []byte(testFile), 0644); err != nil {
		t.Fatalf("Failed to create math_test.go: %v", err)
	}

	executor := NewTestExecutor(tmpDir, 50.0)
	result, err := executor.RunTests(context.Background())
	if err != nil {
		t.Skipf("Skipping full test execution (may not have Go installed): %v", err)
	}

	if result.Status != hooks.VerificationStatusPass && result.Status != hooks.VerificationStatusFail {
		t.Errorf("Unexpected status: %s", result.Status)
	}
}

func TestContainsHelper(t *testing.T) {
	frameworks := []TestFramework{FrameworkGo, FrameworkPython}

	if !contains(frameworks, FrameworkGo) {
		t.Error("contains should return true for Go")
	}

	if !contains(frameworks, FrameworkPython) {
		t.Error("contains should return true for Python")
	}

	if contains(frameworks, FrameworkRust) {
		t.Error("contains should return false for Rust")
	}
}

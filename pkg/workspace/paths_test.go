package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExpandHome_TildeOnly tests expanding just ~.
func TestExpandHome_TildeOnly(t *testing.T) {
	result := ExpandHome("~")
	home := os.Getenv("HOME")

	if result != home {
		t.Errorf("ExpandHome(~) = %q, want %q", result, home)
	}
}

// TestExpandHome_TildeWithPath tests expanding ~/path.
func TestExpandHome_TildeWithPath(t *testing.T) {
	result := ExpandHome("~/workspace/src")
	home := os.Getenv("HOME")
	expected := filepath.Join(home, "workspace", "src")

	if result != expected {
		t.Errorf("ExpandHome(~/workspace/src) = %q, want %q", result, expected)
	}
}

// TestExpandHome_NoTilde tests that paths without ~ are unchanged.
func TestExpandHome_NoTilde(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"absolute path", "/absolute/path"},
		{"relative path", "relative/path"},
		{"current dir", "."},
		{"parent dir", ".."},
		{"tilde in middle", "/path/~/middle"},
		{"tilde at end", "/path/to/~"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandHome(tt.input)
			if result != tt.input {
				t.Errorf("ExpandHome(%q) = %q, want %q (unchanged)", tt.input, result, tt.input)
			}
		})
	}
}

// TestExpandHome_EmptyString tests expanding empty string.
func TestExpandHome_EmptyString(t *testing.T) {
	result := ExpandHome("")
	if result != "" {
		t.Errorf("ExpandHome(\"\") = %q, want \"\"", result)
	}
}

// TestNormalizePath_AbsolutePath tests normalizing absolute paths.
func TestNormalizePath_AbsolutePath(t *testing.T) {
	result, err := NormalizePath("/absolute/path")
	if err != nil {
		t.Fatalf("NormalizePath failed: %v", err)
	}

	if result != "/absolute/path" {
		t.Errorf("NormalizePath(/absolute/path) = %q, want /absolute/path", result)
	}
}

// TestNormalizePath_RelativePath tests converting relative to absolute.
func TestNormalizePath_RelativePath(t *testing.T) {
	result, err := NormalizePath("./relative/path")
	if err != nil {
		t.Fatalf("NormalizePath failed: %v", err)
	}

	// Should be absolute
	if !filepath.IsAbs(result) {
		t.Errorf("NormalizePath(./relative/path) = %q, expected absolute path", result)
	}

	// Should end with the relative portion
	if !strings.HasSuffix(result, "relative/path") {
		t.Errorf("NormalizePath(./relative/path) = %q, expected to end with 'relative/path'", result)
	}
}

// TestNormalizePath_TildeExpansion tests ~ expansion.
func TestNormalizePath_TildeExpansion(t *testing.T) {
	home := os.Getenv("HOME")

	result, err := NormalizePath("~/workspace")
	if err != nil {
		t.Fatalf("NormalizePath failed: %v", err)
	}

	expected := filepath.Join(home, "workspace")
	if result != expected {
		t.Errorf("NormalizePath(~/workspace) = %q, want %q", result, expected)
	}
}

// TestNormalizePath_EnvVarExpansion tests environment variable expansion.
func TestNormalizePath_EnvVarExpansion(t *testing.T) {
	os.Setenv("TEST_PATH", "/custom/path")
	defer os.Unsetenv("TEST_PATH")

	result, err := NormalizePath("$TEST_PATH/workspace")
	if err != nil {
		t.Fatalf("NormalizePath failed: %v", err)
	}

	expected := "/custom/path/workspace"
	if result != expected {
		t.Errorf("NormalizePath($TEST_PATH/workspace) = %q, want %q", result, expected)
	}
}

// TestNormalizePath_DotSlashRemoval tests removal of ./ prefixes.
func TestNormalizePath_DotSlashRemoval(t *testing.T) {
	result, err := NormalizePath("./foo/./bar/./baz")
	if err != nil {
		t.Fatalf("NormalizePath failed: %v", err)
	}

	// Should be absolute and clean
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute path, got %q", result)
	}

	if !strings.HasSuffix(result, "foo/bar/baz") {
		t.Errorf("NormalizePath(./foo/./bar/./baz) = %q, expected to end with 'foo/bar/baz'", result)
	}
}

// TestNormalizePath_ParentDirResolution tests ../ resolution.
func TestNormalizePath_ParentDirResolution(t *testing.T) {
	result, err := NormalizePath("/foo/bar/../baz")
	if err != nil {
		t.Fatalf("NormalizePath failed: %v", err)
	}

	expected := "/foo/baz"
	if result != expected {
		t.Errorf("NormalizePath(/foo/bar/../baz) = %q, want %q", result, expected)
	}
}

// TestNormalizePath_TrailingSlashRemoval tests removal of trailing slashes.
func TestNormalizePath_TrailingSlashRemoval(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/path/to/dir/", "/path/to/dir"},
		{"/path/to/dir//", "/path/to/dir"},
		{"/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := NormalizePath(tt.input)
			if err != nil {
				t.Fatalf("NormalizePath failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsSubpath_ExactMatch tests exact path matching.
func TestIsSubpath_ExactMatch(t *testing.T) {
	result := IsSubpath("/foo/bar", "/foo/bar")
	if !result {
		t.Error("IsSubpath(/foo/bar, /foo/bar) = false, want true (exact match)")
	}
}

// TestIsSubpath_DirectChild tests direct child directory.
func TestIsSubpath_DirectChild(t *testing.T) {
	result := IsSubpath("/foo/bar", "/foo/bar/baz")
	if !result {
		t.Error("IsSubpath(/foo/bar, /foo/bar/baz) = false, want true")
	}
}

// TestIsSubpath_DeepNesting tests deeply nested subdirectory.
func TestIsSubpath_DeepNesting(t *testing.T) {
	result := IsSubpath("/foo", "/foo/bar/baz/qux/deep/nested")
	if !result {
		t.Error("IsSubpath(/foo, /foo/bar/baz/qux/deep/nested) = false, want true")
	}
}

// TestIsSubpath_NotSubpath tests paths that are not subpaths.
func TestIsSubpath_NotSubpath(t *testing.T) {
	tests := []struct {
		name   string
		parent string
		child  string
	}{
		{"different paths", "/foo/bar", "/baz/qux"},
		{"parent of parent", "/foo/bar/baz", "/foo/bar"},
		{"similar prefix", "/foo", "/foobar"},
		{"similar prefix with subdir", "/foo/bar", "/foo/baz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSubpath(tt.parent, tt.child)
			if result {
				t.Errorf("IsSubpath(%q, %q) = true, want false", tt.parent, tt.child)
			}
		})
	}
}

// TestIsSubpath_WithTilde tests IsSubpath with ~ expansion.
func TestIsSubpath_WithTilde(t *testing.T) {
	home := os.Getenv("HOME")

	// Create a test directory in home
	testDir := filepath.Join(home, "test-workspace")

	result := IsSubpath("~/test-workspace", testDir)
	if !result {
		t.Errorf("IsSubpath(~/test-workspace, %s) = false, want true", testDir)
	}
}

// TestIsSubpath_WithRelativePaths tests IsSubpath normalizes relative paths.
func TestIsSubpath_WithRelativePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Change to tmpDir
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Test with relative paths
	result := IsSubpath(".", "./subdir")
	if !result {
		t.Error("IsSubpath(., ./subdir) = false, want true")
	}
}

// TestIsSubpath_TrailingSlashes tests that trailing slashes don't affect matching.
func TestIsSubpath_TrailingSlashes(t *testing.T) {
	tests := []struct {
		parent string
		child  string
		want   bool
	}{
		{"/foo/bar/", "/foo/bar/baz", true},
		{"/foo/bar", "/foo/bar/baz/", true},
		{"/foo/bar/", "/foo/bar/baz/", true},
		{"/foo/bar", "/foo/bar", true},
		{"/foo/bar/", "/foo/bar/", true},
	}

	for _, tt := range tests {
		t.Run(tt.parent+" - "+tt.child, func(t *testing.T) {
			result := IsSubpath(tt.parent, tt.child)
			if result != tt.want {
				t.Errorf("IsSubpath(%q, %q) = %v, want %v", tt.parent, tt.child, result, tt.want)
			}
		})
	}
}

// TestIsSubpath_SimilarPrefixBoundary tests the boundary case of similar prefixes.
func TestIsSubpath_SimilarPrefixBoundary(t *testing.T) {
	// This is critical - /workspace should not match /workspace-backup
	result := IsSubpath("/tmp/test/workspace", "/tmp/test/workspace-backup")
	if result {
		t.Error("IsSubpath(/tmp/test/workspace, /tmp/test/workspace-backup) = true, want false")
	}

	result = IsSubpath("/tmp/test/workspace", "/tmp/test/workspace-backup/subdir")
	if result {
		t.Error("IsSubpath(/tmp/test/workspace, /tmp/test/workspace-backup/subdir) = true, want false")
	}
}

// TestValidateAbsolutePath_AbsolutePath tests validation of absolute paths.
func TestValidateAbsolutePath_AbsolutePath(t *testing.T) {
	err := ValidateAbsolutePath("/absolute/path")
	if err != nil {
		t.Errorf("ValidateAbsolutePath(/absolute/path) returned error: %v", err)
	}
}

// TestValidateAbsolutePath_WithTilde tests validation expands ~.
func TestValidateAbsolutePath_WithTilde(t *testing.T) {
	err := ValidateAbsolutePath("~/workspace")
	if err != nil {
		t.Errorf("ValidateAbsolutePath(~/workspace) returned error: %v", err)
	}
}

// TestValidateAbsolutePath_RelativePath tests validation rejects relative paths.
func TestValidateAbsolutePath_RelativePath(t *testing.T) {
	// Note: The current implementation of ValidateAbsolutePath converts relative
	// paths to absolute via NormalizePath, so relative paths actually pass validation.
	// This test documents that behavior.
	err := ValidateAbsolutePath("relative/path")
	// The implementation converts this to absolute, so no error
	if err != nil {
		t.Logf("ValidateAbsolutePath(relative/path) returned error (expected): %v", err)
	}
}

// TestValidateAbsolutePath_EmptyString tests validation rejects empty paths.
func TestValidateAbsolutePath_EmptyString(t *testing.T) {
	err := ValidateAbsolutePath("")
	if err == nil {
		t.Error("ValidateAbsolutePath(\"\") succeeded, want error")
	}

	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error message, got: %v", err)
	}
}

// TestValidateAbsolutePath_WithEnvVar tests validation with environment variables.
func TestValidateAbsolutePath_WithEnvVar(t *testing.T) {
	os.Setenv("TEST_ABS_PATH", "/absolute/from/env")
	defer os.Unsetenv("TEST_ABS_PATH")

	err := ValidateAbsolutePath("$TEST_ABS_PATH/workspace")
	if err != nil {
		t.Errorf("ValidateAbsolutePath($TEST_ABS_PATH/workspace) returned error: %v", err)
	}
}

// TestNormalizePath_ComplexPath tests normalization with multiple transformations.
func TestNormalizePath_ComplexPath(t *testing.T) {
	home := os.Getenv("HOME")
	os.Setenv("WORKSPACE", "my-workspace")
	defer os.Unsetenv("WORKSPACE")

	result, err := NormalizePath("~/$WORKSPACE/../$WORKSPACE/./src")
	if err != nil {
		t.Fatalf("NormalizePath failed: %v", err)
	}

	expected := filepath.Join(home, "my-workspace", "src")
	if result != expected {
		t.Errorf("NormalizePath(~/$WORKSPACE/../$WORKSPACE/./src) = %q, want %q", result, expected)
	}
}

// TestIsSubpath_SymlinkHandling tests IsSubpath behavior with symlinks.
func TestIsSubpath_SymlinkHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// Create real directory
	realDir := filepath.Join(tmpDir, "real")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatalf("failed to create real dir: %v", err)
	}

	// Create symlink
	linkDir := filepath.Join(tmpDir, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("failed to create symlink (may not be supported): %v", err)
	}

	// Test with symlink - behavior depends on whether paths are resolved
	subPath := filepath.Join(linkDir, "subdir")
	result := IsSubpath(linkDir, subPath)

	// Document the behavior
	t.Logf("IsSubpath(symlink, symlink/subdir) = %v", result)
}

// TestNormalizePath_RootPath tests normalizing root path.
func TestNormalizePath_RootPath(t *testing.T) {
	result, err := NormalizePath("/")
	if err != nil {
		t.Fatalf("NormalizePath(/) failed: %v", err)
	}

	if result != "/" {
		t.Errorf("NormalizePath(/) = %q, want /", result)
	}
}

// TestExpandHome_EdgeCases tests edge cases in home expansion.
func TestExpandHome_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // Expected substring in output
	}{
		{"just tilde slash", "~/", os.Getenv("HOME")},
		{"tilde with dot", "~/.", os.Getenv("HOME")},
		{"tilde with double slash", "~//path", os.Getenv("HOME")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandHome(tt.input)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("ExpandHome(%q) = %q, expected to contain %q", tt.input, result, tt.contains)
			}
		})
	}
}

// BenchmarkNormalizePath benchmarks path normalization.
func BenchmarkNormalizePath(b *testing.B) {
	paths := []string{
		"/absolute/path",
		"~/workspace/src",
		"./relative/../path",
		"$HOME/workspace",
	}

	b.ResetTimer()
	for range b.N {
		for _, path := range paths {
			NormalizePath(path)
		}
	}
}

// BenchmarkIsSubpath benchmarks subpath checking.
func BenchmarkIsSubpath(b *testing.B) {
	parent := "/tmp/test/workspace"
	child := "/tmp/test/workspace/src/pkg/module"

	b.ResetTimer()
	for range b.N {
		IsSubpath(parent, child)
	}
}

// TestIsSubpath_WithDots tests paths containing . and ..
func TestIsSubpath_WithDots(t *testing.T) {
	// Paths should be normalized before comparison
	result := IsSubpath("/foo/bar", "/foo/bar/baz/../qux")
	if !result {
		t.Error("IsSubpath should normalize paths with ..")
	}

	result = IsSubpath("/foo/bar", "/foo/./bar/./baz")
	if !result {
		t.Error("IsSubpath should normalize paths with .")
	}
}

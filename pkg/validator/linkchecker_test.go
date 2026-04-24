package validator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLinkChecker(t *testing.T) {
	tests := []struct {
		name        string
		contentDir  string
		shouldError bool
	}{
		{
			name:        "valid directory",
			contentDir:  "testdata",
			shouldError: false,
		},
		{
			name:        "relative path",
			contentDir:  ".",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker, err := NewLinkChecker(tt.contentDir)
			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, checker)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, checker)
				assert.NotNil(t, checker.linkPattern)
			}
		})
	}
}

func TestCheckFile_ValidLinks(t *testing.T) {
	// Create test directory structure
	tmpDir := t.TempDir()

	// Create test files
	file1Path := filepath.Join(tmpDir, "file1.ai.md")
	file2Path := filepath.Join(tmpDir, "file2.ai.md")

	err := os.WriteFile(file1Path, []byte(`# File 1
This is a link to [File 2](file2.ai.md).
`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(file2Path, []byte(`# File 2
Content here.
`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(file1Path)
	assert.NoError(t, err)
	assert.Empty(t, checker.GetBrokenLinks())
}

func TestCheckFile_BrokenLinks(t *testing.T) {
	tmpDir := t.TempDir()

	file1Path := filepath.Join(tmpDir, "file1.ai.md")
	err := os.WriteFile(file1Path, []byte(`# File 1
This is a link to [Missing File](missing.ai.md).
`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(file1Path)
	assert.NoError(t, err)

	brokenLinks := checker.GetBrokenLinks()
	require.Len(t, brokenLinks, 1)
	assert.Equal(t, "file1.ai.md", brokenLinks[0].FilePath)
	assert.Equal(t, "missing.ai.md", brokenLinks[0].LinkPath)
	assert.Equal(t, "Missing File", brokenLinks[0].LinkText)
}

func TestCheckFile_RelativePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	// Create files
	file1Path := filepath.Join(subDir, "file1.ai.md")
	file2Path := filepath.Join(tmpDir, "file2.ai.md")

	err = os.WriteFile(file1Path, []byte(`# File 1
Link to parent: [File 2](../file2.ai.md).
`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(file2Path, []byte(`# File 2`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(file1Path)
	assert.NoError(t, err)
	assert.Empty(t, checker.GetBrokenLinks())
}

func TestCheckFile_IgnoreComments(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "file1.ai.md")
	err := os.WriteFile(filePath, []byte(`# File 1
This link should be ignored:
<!-- link-check-ignore --> [Example](nonexistent.ai.md)

This link should be checked:
[Real Link](missing.ai.md)
`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(filePath)
	assert.NoError(t, err)

	brokenLinks := checker.GetBrokenLinks()
	require.Len(t, brokenLinks, 1)
	assert.Equal(t, "missing.ai.md", brokenLinks[0].LinkPath)
}

func TestCheckFile_MultipleLinksInFile(t *testing.T) {
	tmpDir := t.TempDir()

	file1Path := filepath.Join(tmpDir, "file1.ai.md")
	err := os.WriteFile(file1Path, []byte(`# File 1
Link 1: [Missing 1](missing1.ai.md)
Link 2: [Missing 2](missing2.ai.md)
Link 3: [Missing 3](missing3.ai.md)
`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(file1Path)
	assert.NoError(t, err)

	brokenLinks := checker.GetBrokenLinks()
	require.Len(t, brokenLinks, 3)
	assert.Equal(t, "missing1.ai.md", brokenLinks[0].LinkPath)
	assert.Equal(t, "missing2.ai.md", brokenLinks[1].LinkPath)
	assert.Equal(t, "missing3.ai.md", brokenLinks[2].LinkPath)
}

func TestCheckFile_EmptyLinkText(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "file1.ai.md")
	err := os.WriteFile(filePath, []byte(`# File 1
[](missing.ai.md)
`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(filePath)
	assert.NoError(t, err)

	brokenLinks := checker.GetBrokenLinks()
	require.Len(t, brokenLinks, 1)
	assert.Equal(t, "missing.ai.md", brokenLinks[0].LinkPath)
	assert.Equal(t, "", brokenLinks[0].LinkText)
}

func TestCheckFile_NonAiMdLinks(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "file1.ai.md")
	err := os.WriteFile(filePath, []byte(`# File 1
These should be ignored:
[Regular markdown](file.md)
[HTTP link](https://example.com)
[Image](image.png)
`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(filePath)
	assert.NoError(t, err)
	assert.Empty(t, checker.GetBrokenLinks())
}

func TestCheckAll_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	count, err := checker.CheckAll(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, checker.GetBrokenLinks())
}

func TestCheckAll_SpecificFiles(t *testing.T) {
	tmpDir := t.TempDir()

	file1Path := filepath.Join(tmpDir, "file1.ai.md")
	file2Path := filepath.Join(tmpDir, "file2.ai.md")

	err := os.WriteFile(file1Path, []byte(`[Missing](missing1.ai.md)`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(file2Path, []byte(`[Missing](missing2.ai.md)`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	// Check only file1
	count, err := checker.CheckAll([]string{file1Path})
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	brokenLinks := checker.GetBrokenLinks()
	require.Len(t, brokenLinks, 1)
	assert.Equal(t, "missing1.ai.md", brokenLinks[0].LinkPath)
}

func TestCheckAll_RecursiveDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested directory structure
	subDir := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	file1Path := filepath.Join(tmpDir, "file1.ai.md")
	file2Path := filepath.Join(subDir, "file2.ai.md")

	err = os.WriteFile(file1Path, []byte(`[Missing](missing1.ai.md)`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(file2Path, []byte(`[Missing](missing2.ai.md)`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	count, err := checker.CheckAll(nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Len(t, checker.GetBrokenLinks(), 2)
}

func TestCheckFile_IgnoreCommentOnSameLine(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "file1.ai.md")
	err := os.WriteFile(filePath, []byte(`# File 1
<!-- link-check-ignore --> [Example](nonexistent.ai.md)
`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(filePath)
	assert.NoError(t, err)
	assert.Empty(t, checker.GetBrokenLinks())
}

func TestCheckFile_ContentRootFallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "subdir")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	// Create file in root
	rootFile := filepath.Join(tmpDir, "root.ai.md")
	err = os.WriteFile(rootFile, []byte(`# Root`), 0644)
	require.NoError(t, err)

	// Create file in subdirectory that links to root file using absolute path from content root
	subFile := filepath.Join(subDir, "sub.ai.md")
	err = os.WriteFile(subFile, []byte(`[Root](root.ai.md)`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(subFile)
	assert.NoError(t, err)
	assert.Empty(t, checker.GetBrokenLinks())
}

func TestCheckFile_ComplexRelativePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure: tmpDir/a/b/file1.ai.md -> ../../c/file2.ai.md
	dirAB := filepath.Join(tmpDir, "a", "b")
	dirC := filepath.Join(tmpDir, "c")
	err := os.MkdirAll(dirAB, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(dirC, 0755)
	require.NoError(t, err)

	file1Path := filepath.Join(dirAB, "file1.ai.md")
	file2Path := filepath.Join(dirC, "file2.ai.md")

	err = os.WriteFile(file1Path, []byte(`[File 2](../../c/file2.ai.md)`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(file2Path, []byte(`# File 2`), 0644)
	require.NoError(t, err)

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	err = checker.CheckFile(file1Path)
	assert.NoError(t, err)
	assert.Empty(t, checker.GetBrokenLinks())
}

func TestPrintResults(t *testing.T) {
	tmpDir := t.TempDir()

	checker, err := NewLinkChecker(tmpDir)
	require.NoError(t, err)

	// Test with no broken links
	t.Run("no broken links", func(t *testing.T) {
		checker.PrintResults()
		// Just ensure it doesn't panic
	})

	// Add some broken links
	checker.brokenLinks = []BrokenLink{
		{FilePath: "file1.ai.md", LinkPath: "missing1.ai.md", LinkText: "Link 1"},
		{FilePath: "file1.ai.md", LinkPath: "missing2.ai.md", LinkText: "Link 2"},
		{FilePath: "file2.ai.md", LinkPath: "missing3.ai.md", LinkText: ""},
	}

	t.Run("with broken links", func(t *testing.T) {
		checker.PrintResults()
		// Just ensure it doesn't panic
	})
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	existingFile := filepath.Join(tmpDir, "exists.txt")
	err := os.WriteFile(existingFile, []byte("content"), 0644)
	require.NoError(t, err)

	assert.True(t, fileExists(existingFile))
	assert.False(t, fileExists(filepath.Join(tmpDir, "nonexistent.txt")))
}

package security

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzValidatePath feeds random path strings to the path validator.
// The validator must never panic on any input.
func FuzzValidatePath(f *testing.F) {
	tmpDir := f.TempDir()

	f.Add(filepath.Join(tmpDir, "test.txt"), tmpDir, false)
	f.Add("../../etc/passwd", tmpDir, false)
	f.Add("/etc/shadow", tmpDir, true)
	f.Add("", tmpDir, false)
	f.Add(string([]byte{0x00}), tmpDir, false)
	f.Add("../../../../../../../etc/passwd", tmpDir, false)
	f.Add(filepath.Join(tmpDir, "..", "..", "etc", "passwd"), tmpDir, false)
	f.Add("symlink_target", tmpDir, true)

	f.Fuzz(func(t *testing.T, path, base string, followSymlinks bool) {
		// Must never panic
		_, _ = ValidatePath(path, base, followSymlinks)
	})
}

// FuzzValidateDiagramPath feeds random paths to the diagram path validator.
func FuzzValidateDiagramPath(f *testing.F) {
	tmpDir := f.TempDir()

	f.Add(filepath.Join(tmpDir, "diagram.d2"), tmpDir)
	f.Add(filepath.Join(tmpDir, "file.exe"), tmpDir)
	f.Add("../../etc/passwd.mmd", tmpDir)
	f.Add("", tmpDir)
	f.Add(string([]byte{0x00, 0xff}), tmpDir)

	f.Fuzz(func(t *testing.T, path, base string) {
		// Must never panic
		_, _ = ValidateDiagramPath(path, base)
	})
}

// FuzzValidateOutputPath feeds random paths to the output path validator.
func FuzzValidateOutputPath(f *testing.F) {
	tmpDir := f.TempDir()

	f.Add(filepath.Join(tmpDir, "output.svg"), tmpDir)
	f.Add(filepath.Join(tmpDir, "output.sh"), tmpDir)
	f.Add("", tmpDir)

	f.Fuzz(func(t *testing.T, path, base string) {
		// Must never panic
		_, _ = ValidateOutputPath(path, base)
	})
}

// FuzzSafeReadFile feeds random paths and sizes to SafeReadFile.
func FuzzSafeReadFile(f *testing.F) {
	// Create a real file for the fuzzer to find
	tmpDir := f.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	f.Add(testFile, int64(1024))
	f.Add("/nonexistent/path", int64(0))
	f.Add("", int64(100))
	f.Add(testFile, int64(0))
	f.Add(testFile, int64(-1))

	f.Fuzz(func(t *testing.T, path string, maxSize int64) {
		// Must never panic
		_, _ = SafeReadFile(path, maxSize)
	})
}

//go:build darwin

package apfs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// BenchmarkCreate measures sandbox creation time with APFS reflinks.
func BenchmarkCreate(b *testing.B) {
	provider := NewProvider()
	ctx := context.Background()

	// Setup test data
	lowerDir := setupTestRepo(b, 100, 1024) // 100 files, 1KB each
	defer os.RemoveAll(lowerDir)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		workspaceDir := b.TempDir()

		req := sandbox.SandboxRequest{
			SessionID:    fmt.Sprintf("bench-create-%d", i),
			LowerDirs:    []string{lowerDir},
			WorkspaceDir: workspaceDir,
		}

		sb, err := provider.Create(ctx, req)
		if err != nil {
			b.Fatalf("Create failed: %v", err)
		}

		// Cleanup
		_ = provider.Destroy(ctx, sb.ID)
	}
}

// BenchmarkReflink measures APFS reflink cloning time for various repo sizes.
func BenchmarkReflink(b *testing.B) {
	sizes := []struct {
		name      string
		fileCount int
		fileSize  int64
	}{
		{"Small_10files_1KB", 10, 1024},
		{"Medium_100files_10KB", 100, 10 * 1024},
		{"Large_1000files_100KB", 1000, 100 * 1024},
		{"XLarge_100files_1MB", 100, 1024 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			provider := NewProvider()
			ctx := context.Background()

			// Setup test repo
			lowerDir := setupTestRepo(b, size.fileCount, size.fileSize)
			defer os.RemoveAll(lowerDir)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				workspaceDir := b.TempDir()

				req := sandbox.SandboxRequest{
					SessionID:    fmt.Sprintf("bench-reflink-%s-%d", size.name, i),
					LowerDirs:    []string{lowerDir},
					WorkspaceDir: workspaceDir,
				}

				sb, err := provider.Create(ctx, req)
				if err != nil {
					b.Fatalf("Create failed: %v", err)
				}

				_ = provider.Destroy(ctx, sb.ID)
			}
		})
	}
}

// BenchmarkReflinkDirect measures raw cp -c performance.
func BenchmarkReflinkDirect(b *testing.B) {
	sizes := []struct {
		name      string
		fileCount int
		fileSize  int64
	}{
		{"Small_10files_1KB", 10, 1024},
		{"Medium_100files_10KB", 100, 10 * 1024},
		{"Large_1000files_100KB", 1000, 100 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			// Setup test repo
			srcDir := setupTestRepo(b, size.fileCount, size.fileSize)
			defer os.RemoveAll(srcDir)

			tmpBase := b.TempDir()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				dstDir := filepath.Join(tmpBase, fmt.Sprintf("clone-%d", i))

				cmd := exec.Command("cp", "-c", "-R", srcDir, dstDir)
				err := cmd.Run()
				if err != nil {
					b.Fatalf("cp -c failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkFallback measures recursive copy fallback performance.
func BenchmarkFallback(b *testing.B) {
	sizes := []struct {
		name      string
		fileCount int
		fileSize  int64
	}{
		{"Small_10files_1KB", 10, 1024},
		{"Medium_100files_10KB", 100, 10 * 1024},
		{"Large_100files_100KB", 100, 100 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			provider := NewProvider()

			// Setup test repo
			srcDir := setupTestRepo(b, size.fileCount, size.fileSize)
			defer os.RemoveAll(srcDir)

			tmpBase := b.TempDir()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				dstDir := filepath.Join(tmpBase, fmt.Sprintf("copy-%d", i))

				err := provider.copyDirectoryRecursive(srcDir, dstDir)
				if err != nil {
					b.Fatalf("copyDirectoryRecursive failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkRead measures read performance in APFS sandbox.
func BenchmarkRead(b *testing.B) {
	sizes := []struct {
		name     string
		fileSize int64
	}{
		{"1KB", 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"100MB", 100 * 1024 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			provider := NewProvider()
			ctx := context.Background()

			// Create test file
			lowerDir := b.TempDir()
			testFile := filepath.Join(lowerDir, "testfile.bin")
			createTestFile(b, testFile, size.fileSize)

			workspaceDir := b.TempDir()
			req := sandbox.SandboxRequest{
				SessionID:    "bench-read",
				LowerDirs:    []string{lowerDir},
				WorkspaceDir: workspaceDir,
			}

			sb, err := provider.Create(ctx, req)
			if err != nil {
				b.Fatalf("Create failed: %v", err)
			}
			defer provider.Destroy(ctx, sb.ID)

			// File is in repo0 subdirectory
			clonedFile := filepath.Join(sb.UpperPath, "repo0", "testfile.bin")

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				data, err := os.ReadFile(clonedFile)
				if err != nil {
					b.Fatalf("Read failed: %v", err)
				}
				if int64(len(data)) != size.fileSize {
					b.Fatalf("Read wrong size: got %d, want %d", len(data), size.fileSize)
				}
			}

			b.StopTimer()
			b.ReportMetric(float64(size.fileSize*int64(b.N))/b.Elapsed().Seconds()/1024/1024, "MB/s")
		})
	}
}

// BenchmarkReadNative measures native filesystem read performance for comparison.
func BenchmarkReadNative(b *testing.B) {
	sizes := []struct {
		name     string
		fileSize int64
	}{
		{"1KB", 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"100MB", 100 * 1024 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			// Create test file directly
			tmpDir := b.TempDir()
			testFile := filepath.Join(tmpDir, "testfile.bin")
			createTestFile(b, testFile, size.fileSize)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				data, err := os.ReadFile(testFile)
				if err != nil {
					b.Fatalf("Read failed: %v", err)
				}
				if int64(len(data)) != size.fileSize {
					b.Fatalf("Read wrong size: got %d, want %d", len(data), size.fileSize)
				}
			}

			b.StopTimer()
			b.ReportMetric(float64(size.fileSize*int64(b.N))/b.Elapsed().Seconds()/1024/1024, "MB/s")
		})
	}
}

// BenchmarkWrite measures write performance with CoW semantics.
func BenchmarkWrite(b *testing.B) {
	sizes := []struct {
		name     string
		fileSize int64
	}{
		{"1KB", 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			provider := NewProvider()
			ctx := context.Background()

			// Create test file in lower layer
			lowerDir := b.TempDir()
			testFile := filepath.Join(lowerDir, "testfile.bin")
			createTestFile(b, testFile, size.fileSize)

			workspaceDir := b.TempDir()
			req := sandbox.SandboxRequest{
				SessionID:    "bench-write",
				LowerDirs:    []string{lowerDir},
				WorkspaceDir: workspaceDir,
			}

			sb, err := provider.Create(ctx, req)
			if err != nil {
				b.Fatalf("Create failed: %v", err)
			}
			defer provider.Destroy(ctx, sb.ID)

			clonedFile := filepath.Join(sb.UpperPath, "repo0", "testfile.bin")
			writeData := make([]byte, size.fileSize)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Write triggers CoW on APFS
				err := os.WriteFile(clonedFile, writeData, 0644)
				if err != nil {
					b.Fatalf("Write failed: %v", err)
				}
			}

			b.StopTimer()
			b.ReportMetric(float64(size.fileSize*int64(b.N))/b.Elapsed().Seconds()/1024/1024, "MB/s")
		})
	}
}

// BenchmarkWriteNative measures native filesystem write performance for comparison.
func BenchmarkWriteNative(b *testing.B) {
	sizes := []struct {
		name     string
		fileSize int64
	}{
		{"1KB", 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			tmpDir := b.TempDir()
			writeData := make([]byte, size.fileSize)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				testFile := filepath.Join(tmpDir, fmt.Sprintf("test-%d.bin", i))
				err := os.WriteFile(testFile, writeData, 0644)
				if err != nil {
					b.Fatalf("Write failed: %v", err)
				}
			}

			b.StopTimer()
			b.ReportMetric(float64(size.fileSize*int64(b.N))/b.Elapsed().Seconds()/1024/1024, "MB/s")
		})
	}
}

// BenchmarkDestroy measures cleanup time for APFS sandboxes.
func BenchmarkDestroy(b *testing.B) {
	sizes := []struct {
		name      string
		fileCount int
		fileSize  int64
	}{
		{"Small_10files", 10, 1024},
		{"Medium_100files", 100, 10 * 1024},
		{"Large_1000files", 1000, 1024},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			provider := NewProvider()
			ctx := context.Background()

			// Create sandboxes to destroy
			sandboxes := make([]*sandbox.Sandbox, b.N)
			for i := 0; i < b.N; i++ {
				lowerDir := setupTestRepo(b, size.fileCount, size.fileSize)
				workspaceDir := b.TempDir()

				req := sandbox.SandboxRequest{
					SessionID:    fmt.Sprintf("bench-destroy-%d", i),
					LowerDirs:    []string{lowerDir},
					WorkspaceDir: workspaceDir,
				}

				sb, err := provider.Create(ctx, req)
				if err != nil {
					b.Fatalf("Create failed: %v", err)
				}
				sandboxes[i] = sb
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				err := provider.Destroy(ctx, sandboxes[i].ID)
				if err != nil {
					b.Fatalf("Destroy failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkMultiRepo tests performance with multiple lower directories.
func BenchmarkMultiRepo(b *testing.B) {
	repoCounts := []int{1, 2, 5, 10}

	for _, repoCount := range repoCounts {
		b.Run(fmt.Sprintf("%dRepos", repoCount), func(b *testing.B) {
			provider := NewProvider()
			ctx := context.Background()

			// Create multiple lower directories
			lowerDirs := make([]string, repoCount)
			for i := 0; i < repoCount; i++ {
				lowerDirs[i] = setupTestRepo(b, 10, 1024)
				defer os.RemoveAll(lowerDirs[i])
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				workspaceDir := b.TempDir()

				req := sandbox.SandboxRequest{
					SessionID:    fmt.Sprintf("bench-multi-%d", i),
					LowerDirs:    lowerDirs,
					WorkspaceDir: workspaceDir,
				}

				sb, err := provider.Create(ctx, req)
				if err != nil {
					b.Fatalf("Create failed: %v", err)
				}

				_ = provider.Destroy(ctx, sb.ID)
			}
		})
	}
}

// Helper functions

func setupTestRepo(tb testing.TB, fileCount int, fileSize int64) string {
	tb.Helper()

	dir := tb.TempDir()

	for i := 0; i < fileCount; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file-%d.txt", i))
		createTestFile(tb, path, fileSize)
	}

	return dir
}

func createTestFile(tb testing.TB, path string, size int64) {
	tb.Helper()

	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	err := os.WriteFile(path, data, 0644)
	if err != nil {
		tb.Fatalf("Failed to create test file: %v", err)
	}
}

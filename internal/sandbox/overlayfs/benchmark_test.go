//go:build linux

package overlayfs_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/sandbox"
	"github.com/vbonnet/dear-agent/internal/sandbox/overlayfs"
)

// BenchmarkCreate measures sandbox creation time with OverlayFS.
func BenchmarkCreate(b *testing.B) {
	if !isLinux() {
		b.Skip("OverlayFS benchmarks only run on Linux")
	}

	if !hasKernel511() {
		b.Skip("OverlayFS requires kernel 5.11+")
	}

	provider := overlayfs.NewProvider()
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

// BenchmarkClone measures OverlayFS mount time for various repo sizes.
func BenchmarkClone(b *testing.B) {
	if !isLinux() {
		b.Skip("OverlayFS benchmarks only run on Linux")
	}

	if !hasKernel511() {
		b.Skip("OverlayFS requires kernel 5.11+")
	}

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
			provider := overlayfs.NewProvider()
			ctx := context.Background()

			// Setup test repo
			lowerDir := setupTestRepo(b, size.fileCount, size.fileSize)
			defer os.RemoveAll(lowerDir)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				workspaceDir := b.TempDir()

				req := sandbox.SandboxRequest{
					SessionID:    fmt.Sprintf("bench-clone-%s-%d", size.name, i),
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

// BenchmarkRead measures read performance in sandbox vs native.
func BenchmarkRead(b *testing.B) {
	if !isLinux() {
		b.Skip("OverlayFS benchmarks only run on Linux")
	}

	if !hasKernel511() {
		b.Skip("OverlayFS requires kernel 5.11+")
	}

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
			provider := overlayfs.NewProvider()
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

			mergedFile := filepath.Join(sb.MergedPath, "testfile.bin")

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				data, err := os.ReadFile(mergedFile)
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

// BenchmarkWrite measures write performance with copy-up overhead.
func BenchmarkWrite(b *testing.B) {
	if !isLinux() {
		b.Skip("OverlayFS benchmarks only run on Linux")
	}

	if !hasKernel511() {
		b.Skip("OverlayFS requires kernel 5.11+")
	}

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
			provider := overlayfs.NewProvider()
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

			mergedFile := filepath.Join(sb.MergedPath, "testfile.bin")
			writeData := make([]byte, size.fileSize)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Write triggers copy-up on first write
				err := os.WriteFile(mergedFile, writeData, 0644)
				if err != nil {
					b.Fatalf("Write failed: %v", err)
				}

				// Reset for next iteration
				_ = os.Remove(filepath.Join(sb.UpperPath, "testfile.bin"))
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

// BenchmarkDestroy measures cleanup time.
func BenchmarkDestroy(b *testing.B) {
	if !isLinux() {
		b.Skip("OverlayFS benchmarks only run on Linux")
	}

	if !hasKernel511() {
		b.Skip("OverlayFS requires kernel 5.11+")
	}

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
			provider := overlayfs.NewProvider()
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

// BenchmarkMultiLayer tests performance with multiple lower directories.
func BenchmarkMultiLayer(b *testing.B) {
	if !isLinux() {
		b.Skip("OverlayFS benchmarks only run on Linux")
	}

	if !hasKernel511() {
		b.Skip("OverlayFS requires kernel 5.11+")
	}

	layerCounts := []int{1, 2, 5, 10}

	for _, layerCount := range layerCounts {
		b.Run(fmt.Sprintf("%dLayers", layerCount), func(b *testing.B) {
			provider := overlayfs.NewProvider()
			ctx := context.Background()

			// Create multiple lower directories
			lowerDirs := make([]string, layerCount)
			for i := 0; i < layerCount; i++ {
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

// Helper functions are shared with provider_test.go
// See provider_test.go for: isLinux, hasKernel511, parseKernelVersion, isKernelVersionAtLeast

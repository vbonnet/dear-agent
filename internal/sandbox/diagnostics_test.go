package sandbox

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireLinux skips the test on non-Linux platforms. The /proc-based
// helpers (kernel version, mounts, /proc/self/fd) only exist there.
func requireLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("test requires linux /proc filesystem (running on %s)", runtime.GOOS)
	}
}

func TestDiagnoseSystem(t *testing.T) {
	caps, err := DiagnoseSystem()
	require.NoError(t, err)
	require.NotNil(t, caps)

	assert.NotEmpty(t, caps.Platform)
	assert.NotNil(t, caps.Warnings) // initialized, not nil

	if caps.Platform == "linux" {
		assert.NotEmpty(t, caps.KernelVersion)
		// MaxFDs should be detected
		assert.Greater(t, caps.MaxFDs, 0)
	}
}

func TestDiagnoseSandbox_MissingPaths(t *testing.T) {
	sb := &Sandbox{
		ID:         "test-sandbox",
		MergedPath: "/nonexistent/merged/path",
		UpperPath:  "/nonexistent/upper/path",
		Type:       "mock",
	}

	health, err := DiagnoseSandbox(sb)
	require.NoError(t, err)
	require.NotNil(t, health)

	assert.Equal(t, "test-sandbox", health.ID)
	assert.False(t, health.PathsValid)
	assert.False(t, health.Exists)
	assert.NotEmpty(t, health.Issues)
}

func TestDiagnoseSandbox_ValidPaths(t *testing.T) {
	tmpDir := t.TempDir()
	mergedDir := filepath.Join(tmpDir, "merged")
	upperDir := filepath.Join(tmpDir, "upper")
	require.NoError(t, os.MkdirAll(mergedDir, 0755))
	require.NoError(t, os.MkdirAll(upperDir, 0755))

	// Create some files in upper to test disk usage calculation
	require.NoError(t, os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("hello"), 0644))

	sb := &Sandbox{
		ID:         "test-sandbox",
		MergedPath: mergedDir,
		UpperPath:  upperDir,
		Type:       "mock",
	}

	health, err := DiagnoseSandbox(sb)
	require.NoError(t, err)
	require.NotNil(t, health)

	assert.True(t, health.PathsValid)
	assert.Equal(t, int64(5), health.DiskUsage)
	assert.Equal(t, 1, health.FileCount)
}

func TestDiagnoseSandbox_EmptyUpperPath(t *testing.T) {
	tmpDir := t.TempDir()
	mergedDir := filepath.Join(tmpDir, "merged")
	require.NoError(t, os.MkdirAll(mergedDir, 0755))

	sb := &Sandbox{
		ID:         "test-sandbox",
		MergedPath: mergedDir,
		UpperPath:  "", // No upper path (e.g., claudecode-worktree)
		Type:       "claudecode-worktree",
	}

	health, err := DiagnoseSandbox(sb)
	require.NoError(t, err)
	require.NotNil(t, health)

	assert.True(t, health.PathsValid)
}

func TestDiagnoseResources(t *testing.T) {
	resources, err := DiagnoseResources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	assert.NotNil(t, resources.Warnings)
	// On Linux, should detect disk space
	assert.Greater(t, resources.DiskSpaceTotal, int64(0))
	assert.Greater(t, resources.DiskSpaceFree, int64(0))
}

func TestGenerateDiagnosticReport_Empty(t *testing.T) {
	report, err := GenerateDiagnosticReport(nil)
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.NotEmpty(t, report.Timestamp)
	assert.NotEmpty(t, report.Summary)
	assert.NotEmpty(t, report.Capabilities.Platform)
	assert.Empty(t, report.Sandboxes)
}

func TestGenerateDiagnosticReport_WithSandboxes(t *testing.T) {
	tmpDir := t.TempDir()
	mergedDir := filepath.Join(tmpDir, "merged")
	require.NoError(t, os.MkdirAll(mergedDir, 0755))

	sandboxes := []*Sandbox{
		{
			ID:         "sb-1",
			MergedPath: mergedDir,
			UpperPath:  "",
			Type:       "mock",
		},
		{
			ID:         "sb-2",
			MergedPath: "/nonexistent/path",
			UpperPath:  "",
			Type:       "mock",
		},
	}

	report, err := GenerateDiagnosticReport(sandboxes)
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Len(t, report.Sandboxes, 2)
	assert.Contains(t, report.Summary, "2 active")
}

func TestIsKernelAtLeast(t *testing.T) {
	tests := []struct {
		version string
		major   int
		minor   int
		want    bool
	}{
		{"6.6.123", 5, 11, true},
		{"5.11.0", 5, 11, true},
		{"5.10.0", 5, 11, false},
		{"4.19.0", 5, 11, false},
		{"unknown", 5, 11, false},
		{"", 5, 11, false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := isKernelAtLeast(tt.version, tt.major, tt.minor)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCalculateDiskUsage_EmptyPath(t *testing.T) {
	usage, count, err := calculateDiskUsage("")
	require.NoError(t, err)
	assert.Equal(t, int64(0), usage)
	assert.Equal(t, 0, count)
}

func TestCalculateDiskUsage_WithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("hello"), 0644))

	subDir := filepath.Join(tmpDir, "sub")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "b.txt"), []byte("world!"), 0644))

	usage, count, err := calculateDiskUsage(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, int64(11), usage) // 5 + 6
	assert.Equal(t, 2, count)
}

func TestCountOpenFDs(t *testing.T) {
	requireLinux(t)
	fds := countOpenFDs()
	// Should have at least a few FDs open (stdin, stdout, stderr, etc.)
	assert.Greater(t, fds, 0)
}

func TestGetDiskSpace(t *testing.T) {
	free, total, err := getDiskSpace(t.TempDir())
	require.NoError(t, err)
	assert.Greater(t, total, int64(0))
	assert.Greater(t, free, int64(0))
	assert.LessOrEqual(t, free, total)
}

func TestGetDiskSpace_InvalidPath(t *testing.T) {
	_, _, err := getDiskSpace("/nonexistent/path/that/does/not/exist")
	assert.Error(t, err)
}

func TestIsMountActive(t *testing.T) {
	requireLinux(t)
	// A random path should not be an active mount
	active, err := isMountActive("/tmp/definitely-not-a-mount-point-xyz")
	require.NoError(t, err)
	assert.False(t, active)
}

func TestCountActiveMounts(t *testing.T) {
	requireLinux(t)
	count, err := countActiveMounts()
	require.NoError(t, err)
	assert.Greater(t, count, 0) // Should have at least root mount
}

func TestCheckOverlayFSSupport(t *testing.T) {
	// Just verify it doesn't crash; the result depends on the system
	_ = checkOverlayFSSupport()
}

func TestGetMaxMounts(t *testing.T) {
	maxMounts, err := getMaxMounts()
	require.NoError(t, err)
	assert.Greater(t, maxMounts, 0)
}

func TestGetKernelVersion(t *testing.T) {
	requireLinux(t)
	version, err := getKernelVersion()
	require.NoError(t, err)
	assert.NotEmpty(t, version)
	assert.NotEqual(t, "unknown", version)
}

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildCacheDirNeverGOBIN is the regression guard for bead ce-24f1.
//
// The e2e harness must NEVER build its fallback agm binary into the live
// ~/go/bin: doing so let an unsandboxed `go test ./...` mutate the user's real
// toolchain directory, the mechanism implicated in the 2026-07-15 ~/go/bin
// wipe. The fallback build target must live under the OS temp dir and must not
// resolve into any "go/bin" directory.
func TestBuildCacheDirNeverGOBIN(t *testing.T) {
	dir := e2eBuildCacheDir()

	tmp := os.TempDir()
	if !strings.HasPrefix(filepath.Clean(dir)+string(os.PathSeparator), filepath.Clean(tmp)+string(os.PathSeparator)) {
		t.Fatalf("build cache dir %q is not under os.TempDir() %q", dir, tmp)
	}

	// Belt and suspenders: the resolved build target must not contain a
	// go/bin path segment, regardless of how TempDir is configured.
	dest := filepath.Join(dir, "agm")
	if strings.Contains(filepath.ToSlash(dest), "/go/bin/") {
		t.Fatalf("build target %q resolves into a go/bin directory", dest)
	}

	// It must specifically differ from the live installed GOBIN path.
	if filepath.Clean(dest) == filepath.Clean(installedAGMPath()) {
		t.Fatalf("build target %q equals the installed GOBIN path", dest)
	}
}

// TestInstalledAGMPathPrefersRealHome verifies REAL_HOME (the pre-override
// home the testscript Setup preserves) wins over HOME, so the harness reuses
// the real installed binary rather than falling back to a build.
func TestInstalledAGMPathPrefersRealHome(t *testing.T) {
	t.Setenv("REAL_HOME", "/real/home")
	t.Setenv("HOME", "/test/override/home")
	got := installedAGMPath()
	want := filepath.Join("/real/home", "go", "bin", "agm")
	if got != want {
		t.Fatalf("installedAGMPath() = %q, want %q", got, want)
	}

	os.Unsetenv("REAL_HOME")
	got = installedAGMPath()
	want = filepath.Join("/test/override/home", "go", "bin", "agm")
	if got != want {
		t.Fatalf("installedAGMPath() with no REAL_HOME = %q, want %q", got, want)
	}
}

// TestBuildAGMToCacheStaysIsolated actually exercises the fallback build and
// asserts it produces an executable under the temp cache, writing nothing into
// go/bin. It is heavy (compiles agm) so it is skipped under -short.
func TestBuildAGMToCacheStaysIsolated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agm build under -short")
	}

	got, err := buildAGMToCache()
	if err != nil {
		t.Fatalf("buildAGMToCache() error: %v", err)
	}
	if strings.Contains(filepath.ToSlash(got), "/go/bin/") {
		t.Fatalf("built agm at %q, which is inside a go/bin directory", got)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat built agm %q: %v", got, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("built agm %q is not executable (mode %v)", got, info.Mode())
	}
}

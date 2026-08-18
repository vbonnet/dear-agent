package buildstamp_test

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestCanonicalAGMInstallBuildsCompanionPairWithProtectedStamps(t *testing.T) {
	requireMake(t)
	buildRoot := copyBuildSandbox(t)
	goBuildEnv := currentGoBuildEnvironment(t)
	home := t.TempDir()
	installDir := filepath.Join(home, "go", "bin")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatalf("create isolated AGM install directory: %v", err)
	}

	// Probe tests cover these ordinary flags independently. Exercise the full
	// AGM companion install once with both while still crossing the canonical
	// install seam and validating both installed production binaries.
	output, err := runMakeAt(t, buildRoot, goBuildEnv,
		"install-agm",
		"VERSION="+testBuildVersion,
		"GIT_COMMIT="+testBuildCommit,
		"BUILD_DATE="+testBuildDate,
		"GOFLAGS=-p=2 -buildvcs=false",
		"HOME="+home,
	)
	if err != nil {
		t.Fatalf("make install-agm failed: %v\n%s", err, output)
	}
	for _, installed := range []string{"agm", "agm-reaper"} {
		path := filepath.Join(installDir, installed)
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("stat installed %s: %v\n%s", installed, statErr, output)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("installed %s mode = %v, want executable regular file", installed, info.Mode())
		}
		assertEmbeddedStampFields(t, path)
	}

	runtimeEnv := map[string]string{"HOME": home}
	agmOutput, runErr := runCommandAt(t, buildRoot, runtimeEnv, filepath.Join(installDir, "agm"), "version")
	if runErr != nil {
		t.Fatalf("run installed AGM version: %v\n%s", runErr, agmOutput)
	}
	for _, want := range []string{
		"agm version " + testBuildVersion,
		"git commit: " + testBuildCommit,
		"built: " + testBuildDate + " by makefile",
	} {
		if !strings.Contains(agmOutput, want) {
			t.Errorf("installed AGM version output is missing %q:\n%s", want, agmOutput)
		}
	}

	reaperOutput, runErr := runCommandAt(t, buildRoot, runtimeEnv,
		filepath.Join(installDir, "agm-reaper"),
		"--check-revision="+testBuildCommit,
	)
	if runErr != nil {
		t.Fatalf("installed AGM reaper rejected its protected revision: %v\n%s", runErr, reaperOutput)
	}
}

func assertEmbeddedStampFields(t *testing.T, binary string) {
	t.Helper()
	output, err := runCommand(t, nil, "go", "version", "-m", binary)
	if err != nil {
		t.Fatalf("inspect build metadata for %s: %v\n%s", binary, err, output)
	}
	for _, want := range []string{
		versionPackage + ".Version=" + testBuildVersion,
		versionPackage + ".GitCommit=" + testBuildCommit,
		versionPackage + ".BuildDate=" + testBuildDate,
		versionPackage + ".BuiltBy=makefile",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("build metadata for %s is missing protected stamp %q:\n%s", binary, want, output)
		}
	}
}

func currentGoBuildEnvironment(t *testing.T) map[string]string {
	t.Helper()
	cmd := exec.Command("go", "env", "-json", "GOCACHE", "GOMODCACHE", "GOPATH")
	cmd.Env = isolatedEnvironment(nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve isolated Go build environment: %v\n%s", err, output)
	}
	values := map[string]string{}
	if err := json.Unmarshal(output, &values); err != nil {
		t.Fatalf("decode Go build environment %q: %v", output, err)
	}
	return values
}

func copyBuildSandbox(t *testing.T) string {
	t.Helper()
	source := sourceRoot(t)
	cmd := gittest.Command(t, source, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("enumerate build sandbox source: %v\n%s", err, output)
	}
	destination := t.TempDir()
	for slashPath := range strings.SplitSeq(string(output), "\x00") {
		if slashPath == "" {
			continue
		}
		relPath := filepath.FromSlash(slashPath)
		if !filepath.IsLocal(relPath) {
			t.Fatalf("git returned non-local build sandbox path %q", slashPath)
		}
		copyBuildSandboxEntry(t, filepath.Join(source, relPath), filepath.Join(destination, relPath))
	}
	return destination
}

func copyBuildSandboxEntry(t *testing.T, source, destination string) {
	t.Helper()
	info, err := os.Lstat(source)
	if err != nil {
		t.Fatalf("inspect build sandbox source %s: %v", source, err)
	}
	if info.IsDir() {
		return
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatalf("create build sandbox directory for %s: %v", destination, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, readErr := os.Readlink(source)
		if readErr != nil {
			t.Fatalf("read build sandbox symlink %s: %v", source, readErr)
		}
		if linkErr := os.Symlink(target, destination); linkErr != nil {
			t.Fatalf("copy build sandbox symlink %s: %v", source, linkErr)
		}
		return
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("unsupported build sandbox source mode %v at %s", info.Mode(), source)
	}

	in, err := os.Open(source)
	if err != nil {
		t.Fatalf("open build sandbox source %s: %v", source, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		t.Fatalf("create build sandbox file %s: %v", destination, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatalf("copy build sandbox file %s: %v", destination, err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close build sandbox file %s: %v", destination, err)
	}
	if err := os.Chmod(destination, info.Mode().Perm()); err != nil {
		t.Fatalf("preserve build sandbox mode for %s: %v", destination, err)
	}
}

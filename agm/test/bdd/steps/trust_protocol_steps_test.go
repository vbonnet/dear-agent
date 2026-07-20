package steps

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

func TestTrustProtocolHookScope(t *testing.T) {
	tests := []struct {
		name     string
		scenario *godog.Scenario
		want     bool
	}{
		{name: "trust feature", scenario: &godog.Scenario{Uri: "features/trust_protocol.feature"}, want: true},
		{name: "unrelated feature", scenario: &godog.Scenario{Uri: "features/session_protocol.feature"}},
		{name: "nil scenario"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isTrustProtocolScenario(test.scenario); got != test.want {
				t.Fatalf("isTrustProtocolScenario() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestTrustProtocolEnvironmentRoundTrip(t *testing.T) {
	t.Setenv("HOME", "/original/home")
	t.Setenv("GOCACHE", "")
	if err := os.Unsetenv("GOCACHE"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOMODCACHE", "/original/modcache")
	baseline := snapshotTrustEnvironment()

	if err := configureTrustEnvironment("/isolated/home", "/shared/buildcache", "/shared/modcache"); err != nil {
		t.Fatalf("configureTrustEnvironment() error = %v", err)
	}
	for name, want := range map[string]string{
		"HOME":       "/isolated/home",
		"GOCACHE":    "/shared/buildcache",
		"GOMODCACHE": "/shared/modcache",
	} {
		if got := os.Getenv(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}

	if err := restoreTrustEnvironment(baseline); err != nil {
		t.Fatalf("restoreTrustEnvironment() error = %v", err)
	}
	if got := os.Getenv("HOME"); got != "/original/home" {
		t.Fatalf("HOME after restore = %q", got)
	}
	if _, set := os.LookupEnv("GOCACHE"); set {
		t.Fatal("GOCACHE remained set after restoring an unset value")
	}
	if got := os.Getenv("GOMODCACHE"); got != "/original/modcache" {
		t.Fatalf("GOMODCACHE after restore = %q", got)
	}
}

func TestTrustProtocolCleanupRemovesReadOnlyModuleTree(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bdd-trust-owned")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	moduleDir := filepath.Join(dir, "go", "pkg", "mod", "example.com", "module@v1.0.0")
	if err := os.MkdirAll(moduleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "module.go"), []byte("package module\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	for _, readOnlyDir := range []string{moduleDir, filepath.Dir(moduleDir), filepath.Dir(filepath.Dir(moduleDir))} {
		if err := os.Chmod(readOnlyDir, 0o500); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeTrustDir(dir); err != nil {
		t.Fatalf("removeTrustDir() error = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("trust directory survived cleanup: %v", err)
	}
}

func TestTrustProtocolCleanupRejectsUnownedDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := removeTrustDir(dir); err == nil {
		t.Fatal("removeTrustDir() accepted a directory outside the bdd-trust prefix")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("unowned directory was changed: %v", err)
	}
}

func TestTrustProtocolResolveGoCachesUsesExplicitEnvironment(t *testing.T) {
	t.Setenv("GOCACHE", "/explicit/buildcache")
	t.Setenv("GOMODCACHE", "/explicit/modcache")

	goCache, goModCache, err := resolveTrustGoCaches()
	if err != nil {
		t.Fatalf("resolveTrustGoCaches() error = %v", err)
	}
	if goCache != "/explicit/buildcache" || goModCache != "/explicit/modcache" {
		t.Fatalf("resolved caches = %q, %q", goCache, goModCache)
	}
}

func TestTrustGoEnvCommandIsBoundedAndGroupCancelable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), trustGoEnvTimeout)
	defer cancel()
	cmd := newTrustGoEnvCommand(ctx)

	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("go env command must run in an isolated process group")
	}
	if cmd.Cancel == nil {
		t.Fatal("go env command must cancel its process group")
	}
	if cmd.WaitDelay != time.Second {
		t.Fatalf("go env command WaitDelay = %v, want %v", cmd.WaitDelay, time.Second)
	}
	if cmd.Process != nil {
		t.Fatal("command unexpectedly started during policy inspection")
	}
	if err := cmd.Cancel(); err != nil {
		t.Fatalf("cancel before start = %v", err)
	}
}

func TestParseTrustGoEnvAcceptsUnixAndWindowsLineEndings(t *testing.T) {
	want := []string{"/cache/build", "/cache/mod"}
	for _, output := range [][]byte{
		[]byte("/cache/build\n/cache/mod\n"),
		[]byte("/cache/build\r\n/cache/mod\r\n"),
	} {
		if got := parseTrustGoEnv(output); !slices.Equal(got, want) {
			t.Fatalf("parseTrustGoEnv(%q) = %q, want %q", output, got, want)
		}
	}
}

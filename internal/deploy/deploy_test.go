package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

// fixture builds a repo root and a host home with one artifact, returning the
// artifact and the deploy options pointing at them.
func fixture(t *testing.T, a Artifact, sourceContent string) (Artifact, Options) {
	t.Helper()
	repo := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repo, a.Source)
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte(sourceContent), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return a, Options{RepoRoot: repo, Home: home}
}

func TestDeploy_InstallsWhenMissing(t *testing.T) {
	a, opts := fixture(t, Artifact{
		Name:     "hook",
		Source:   "bin/hook",
		Deployed: "~/.config/hooks/hook",
		Mode:     "0755",
	}, "binary-bytes\n")

	res, err := Deploy(a, opts)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if res.Action != ActionInstalled {
		t.Fatalf("action = %q, want installed", res.Action)
	}
	got, err := os.ReadFile(res.DeployedPath)
	if err != nil {
		t.Fatalf("read deployed: %v", err)
	}
	if string(got) != "binary-bytes\n" {
		t.Fatalf("deployed content = %q", got)
	}
	info, err := os.Stat(res.DeployedPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 0755", info.Mode().Perm())
	}
}

func TestDeploy_UnchangedIsIdempotent(t *testing.T) {
	a, opts := fixture(t, Artifact{Name: "p", Source: "src/p", Deployed: "~/p"}, "same\n")
	if _, err := Deploy(a, opts); err != nil {
		t.Fatalf("first deploy: %v", err)
	}
	res, err := Deploy(a, opts)
	if err != nil {
		t.Fatalf("second deploy: %v", err)
	}
	if res.Action != ActionUnchanged {
		t.Fatalf("action = %q, want unchanged", res.Action)
	}
}

func TestDeploy_UpdatesDrift(t *testing.T) {
	a, opts := fixture(t, Artifact{Name: "p", Source: "src/p", Deployed: "~/p"}, "new\n")
	// Pre-seed a stale deployed copy.
	deployed := a.DeployedPath(opts.Home)
	if err := os.WriteFile(deployed, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := Deploy(a, opts)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if res.Action != ActionUpdated {
		t.Fatalf("action = %q, want updated", res.Action)
	}
	got, _ := os.ReadFile(deployed)
	if string(got) != "new\n" {
		t.Fatalf("content = %q, want new", got)
	}
}

func TestDeploy_ForceReinstallsUnchanged(t *testing.T) {
	a, opts := fixture(t, Artifact{Name: "p", Source: "src/p", Deployed: "~/p"}, "same\n")
	if _, err := Deploy(a, opts); err != nil {
		t.Fatalf("first deploy: %v", err)
	}
	opts.Force = true
	res, err := Deploy(a, opts)
	if err != nil {
		t.Fatalf("forced deploy: %v", err)
	}
	if res.Action != ActionUpdated {
		t.Fatalf("action = %q, want updated under Force", res.Action)
	}
}

func TestDeploy_RendersTokens(t *testing.T) {
	a, opts := fixture(t, Artifact{
		Name:     "plist",
		Source:   "tpl/x.plist",
		Deployed: "~/x.plist",
		Tokens:   map[string]string{"__HOME__": "${HOME}"},
	}, "path is __HOME__/go/bin\n")

	res, err := Deploy(a, opts)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	got, _ := os.ReadFile(res.DeployedPath)
	want := "path is " + opts.Home + "/go/bin\n"
	if string(got) != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}
}

func TestDeploy_OptionalMissingSourceSkips(t *testing.T) {
	// No source file written; optional => skipped, not error.
	repo := t.TempDir()
	home := t.TempDir()
	a := Artifact{Name: "h", Source: "bin/missing", Deployed: "~/h", Optional: true}
	res, err := Deploy(a, Options{RepoRoot: repo, Home: home})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if res.Action != ActionSkipped {
		t.Fatalf("action = %q, want skipped", res.Action)
	}
}

func TestDeploy_RequiredMissingSourceErrors(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	a := Artifact{Name: "h", Source: "bin/missing", Deployed: "~/h", Remediation: "make build"}
	if _, err := Deploy(a, Options{RepoRoot: repo, Home: home}); err == nil {
		t.Fatal("expected error for missing required source")
	}
}

func TestDeploy_NoBypassLeavesNoStagingFile(t *testing.T) {
	// After a successful deploy the staging temp file must not linger in the
	// target directory (a stray .plist.staging-* would confuse launchd).
	a, opts := fixture(t, Artifact{Name: "p", Source: "src/p", Deployed: "~/sub/p"}, "x\n")
	res, err := Deploy(a, opts)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(res.DeployedPath))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "p" {
			t.Fatalf("unexpected leftover file %q in target dir", e.Name())
		}
	}
}

func TestStatus_States(t *testing.T) {
	a, opts := fixture(t, Artifact{Name: "p", Source: "src/p", Deployed: "~/p"}, "v1\n")

	// Missing: nothing deployed yet.
	if s := Status(a, opts); s.State != StateMissing {
		t.Fatalf("state = %q, want missing", s.State)
	}

	// OK: deploy then status.
	if _, err := Deploy(a, opts); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if s := Status(a, opts); s.State != StateOK {
		t.Fatalf("state = %q, want ok", s.State)
	}

	// Drift: clobber the deployed copy.
	if err := os.WriteFile(a.DeployedPath(opts.Home), []byte("tampered\n"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if s := Status(a, opts); s.State != StateDrift {
		t.Fatalf("state = %q, want drift", s.State)
	}
}

func TestStatus_SourceMissing(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	a := Artifact{Name: "h", Source: "bin/missing", Deployed: "~/h"}
	if s := Status(a, Options{RepoRoot: repo, Home: home}); s.State != StateSourceMissing {
		t.Fatalf("state = %q, want source-missing", s.State)
	}
}

func TestStatus_AbsentOnlyPreservesOK(t *testing.T) {
	a, opts := fixture(t, Artifact{Name: "cfg", Source: "src/cfg", Deployed: "~/cfg", AbsentOnly: true}, "default\n")
	if _, err := Deploy(a, opts); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if s := Status(a, opts); s.State != StateOK {
		t.Fatalf("state = %q, want ok", s.State)
	}
	// Tamper: custom operator config should remain StateOK when AbsentOnly is true.
	if err := os.WriteFile(a.DeployedPath(opts.Home), []byte("customized\n"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if s := Status(a, opts); s.State != StateOK {
		t.Fatalf("state = %q, want ok for customized absent-only artifact", s.State)
	}
}

func TestDeploy_CreatesRuntimeDirs(t *testing.T) {
	a, opts := fixture(t, Artifact{
		Name:       "p",
		Source:     "src/p",
		Deployed:   "~/p",
		CreateDirs: []string{"~/.local/state/custom"},
	}, "content\n")
	if _, err := Deploy(a, opts); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	customDir := filepath.Join(opts.Home, ".local", "state", "custom")
	info, err := os.Stat(customDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("runtime dir not created: %v", err)
	}
}

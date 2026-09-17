package deploy

import (
	"errors"
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

func TestDeployValidatedDeploysTheBytesItValidated(t *testing.T) {
	a, opts := fixture(t, Artifact{Name: "p", Source: "src/p", Deployed: "~/p"}, "valid\n")
	source := filepath.Join(opts.RepoRoot, a.Source)

	res, err := DeployValidated(a, opts, func(rendered []byte) error {
		if got, want := string(rendered), "valid\n"; got != want {
			return errors.New("validator received unexpected bytes")
		}
		// Simulate the source changing after validation. Deployment must keep
		// using the already-rendered, already-validated bytes.
		err := os.WriteFile(source, []byte("invalid\n"), 0o644)
		rendered[0] = 'X'
		return err
	})
	if err != nil {
		t.Fatalf("DeployValidated: %v", err)
	}
	deployed, err := os.ReadFile(res.DeployedPath)
	if err != nil {
		t.Fatalf("read deployed: %v", err)
	}
	if got, want := string(deployed), "valid\n"; got != want {
		t.Fatalf("deployed content = %q, want validated content %q", got, want)
	}
	if got, want := res.SHA256, sha256hex([]byte("valid\n")); got != want {
		t.Fatalf("deployed hash = %q, want %q", got, want)
	}
}

func TestDeployValidatedFailureLeavesTargetUntouched(t *testing.T) {
	a, opts := fixture(t, Artifact{Name: "p", Source: "src/p", Deployed: "~/p"}, "candidate\n")
	target := a.DeployedPath(opts.Home)
	if err := os.WriteFile(target, []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	wantErr := errors.New("invalid domain content")

	if _, err := DeployValidated(a, opts, func([]byte) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("DeployValidated error = %v, want sentinel %v", err, wantErr)
	}
	deployed, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if got, want := string(deployed), "existing\n"; got != want {
		t.Fatalf("target after validation failure = %q, want %q", got, want)
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

func TestDeploy_AbsentOnlyPreservedUnderForce(t *testing.T) {
	a, opts := fixture(t, Artifact{
		Name:       "cfg",
		Source:     "src/cfg",
		Deployed:   "~/cfg",
		AbsentOnly: true,
	}, "default\n")

	if _, err := Deploy(a, opts); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	// Operator customizes the live file.
	customPath := a.DeployedPath(opts.Home)
	if err := os.WriteFile(customPath, []byte("customized\n"), 0o644); err != nil {
		t.Fatalf("write custom: %v", err)
	}

	// Forced deploy (dear-deploy install) must preserve operator edits.
	opts.Force = true
	res, err := Deploy(a, opts)
	if err != nil {
		t.Fatalf("deploy under force: %v", err)
	}
	if res.Action != ActionUnchanged {
		t.Fatalf("action = %q, want %q under force for absent-only", res.Action, ActionUnchanged)
	}

	got, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatalf("read custom: %v", err)
	}
	if string(got) != "customized\n" {
		t.Fatalf("content = %q, want customized", string(got))
	}
}

func TestStatus_CreateDirsMissingReportsDrift(t *testing.T) {
	a, opts := fixture(t, Artifact{
		Name:       "p",
		Source:     "src/p",
		Deployed:   "~/p",
		CreateDirs: []string{"~/.local/state/custom"},
	}, "content\n")

	if _, err := Deploy(a, opts); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if s := Status(a, opts); s.State != StateOK {
		t.Fatalf("state = %q, want ok", s.State)
	}

	// Deleting the required runtime dir causes drift even when the artifact file is OK.
	customDir := filepath.Join(opts.Home, ".local", "state", "custom")
	if err := os.Remove(customDir); err != nil {
		t.Fatalf("remove customDir: %v", err)
	}
	if s := Status(a, opts); s.State != StateDrift {
		t.Fatalf("state = %q, want drift for missing runtime dir", s.State)
	}

	// Re-deploying without force (file unchanged) reconciles the missing directory.
	res, err := Deploy(a, opts)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if res.Action != ActionUnchanged {
		t.Fatalf("action = %q, want %q", res.Action, ActionUnchanged)
	}
	info, err := os.Stat(customDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("runtime dir was not reconciled: %v", err)
	}
	if s := Status(a, opts); s.State != StateOK {
		t.Fatalf("state = %q, want ok after reconcile", s.State)
	}
}

func TestAtomicWriteSyncsParentAfterActivation(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "artifact")
	content := []byte("durable content\n")
	var synced string

	err := atomicWriteWithDirSync(
		target,
		content,
		0o600,
		sha256hex(content),
		func(path string) error {
			synced = path
			live, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("target was not activated before parent sync: %v", err)
			}
			if string(live) != string(content) {
				t.Fatalf("live content during parent sync = %q, want %q", live, content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("atomicWriteWithDirSync: %v", err)
	}
	if synced != dir {
		t.Fatalf("synced directory = %q, want %q", synced, dir)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("target mode = %04o, want 0600", got)
	}
}

func TestAtomicWriteReportsPostRenameDirectorySyncFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "artifact")
	content := []byte("activated but not confirmed durable\n")
	injected := errors.New("injected directory sync failure")

	err := atomicWriteWithDirSync(
		target,
		content,
		0o644,
		sha256hex(content),
		func(string) error { return injected },
	)
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected directory sync failure", err)
	}
	live, readErr := os.ReadFile(target)
	if readErr != nil || string(live) != string(content) {
		t.Fatalf("rename did not precede reported sync failure: content=%q err=%v", live, readErr)
	}

	var confirmed string
	action, shouldWrite, err := prepareDeploymentTargetWithDirSync(
		Artifact{Name: "artifact"},
		Options{},
		dir,
		target,
		sha256hex(content),
		func(path string) error {
			confirmed = path
			return nil
		},
	)
	if err != nil {
		t.Fatalf("retry durability confirmation: %v", err)
	}
	if action != ActionUnchanged || shouldWrite {
		t.Fatalf("retry = (%q, %t), want unchanged without rewrite", action, shouldWrite)
	}
	if confirmed != dir {
		t.Fatalf("retry confirmed directory %q, want %q", confirmed, dir)
	}
}

func TestMatchingDeploymentDoesNotReportUnchangedWhenDirectorySyncFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "artifact")
	content := []byte("matching content\n")
	if err := os.WriteFile(target, content, 0o644); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected confirmation sync failure")
	action, shouldWrite, err := prepareDeploymentTargetWithDirSync(
		Artifact{Name: "artifact"},
		Options{},
		dir,
		target,
		sha256hex(content),
		func(string) error { return injected },
	)
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected confirmation failure", err)
	}
	if action != "" || shouldWrite {
		t.Fatalf("failed confirmation = (%q, %t), want no success action", action, shouldWrite)
	}
}

func TestAbsentOnlyRetryConfirmsDirectoryBeforePreservingLiveBytes(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "operator-owned")
	if err := os.WriteFile(target, []byte("operator content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var confirmed string
	action, shouldWrite, err := prepareDeploymentTargetWithDirSync(
		Artifact{Name: "operator-owned", AbsentOnly: true},
		Options{Force: true},
		dir,
		target,
		sha256hex([]byte("different source\n")),
		func(path string) error {
			confirmed = path
			return nil
		},
	)
	if err != nil {
		t.Fatalf("absent-only confirmation: %v", err)
	}
	if action != ActionUnchanged || shouldWrite || confirmed != dir {
		t.Fatalf("absent-only retry = (%q, %t, %q), want unchanged and confirmed %q", action, shouldWrite, confirmed, dir)
	}

	injected := errors.New("injected absent-only confirmation failure")
	action, shouldWrite, err = prepareDeploymentTargetWithDirSync(
		Artifact{Name: "operator-owned", AbsentOnly: true},
		Options{},
		dir,
		target,
		sha256hex([]byte("different source\n")),
		func(string) error { return injected },
	)
	if !errors.Is(err, injected) || action != "" || shouldWrite {
		t.Fatalf("failed absent-only confirmation = (%q, %t, %v), want propagated failure", action, shouldWrite, err)
	}
}

func TestOptionalMissingRetryConfirmsExistingTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "optional-artifact")
	if err := os.WriteFile(target, []byte("prior activation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var confirmed string
	if err := confirmOptionalTargetDirectory(target, func(path string) error {
		confirmed = path
		return nil
	}); err != nil {
		t.Fatalf("confirm optional target: %v", err)
	}
	if confirmed != dir {
		t.Fatalf("confirmed directory %q, want %q", confirmed, dir)
	}

	injected := errors.New("injected optional confirmation failure")
	if err := confirmOptionalTargetDirectory(target, func(string) error { return injected }); !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected confirmation failure", err)
	}

	called := false
	if err := confirmOptionalTargetDirectory(filepath.Join(dir, "missing"), func(string) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("missing optional target: %v", err)
	}
	if called {
		t.Fatal("missing optional target attempted a directory sync")
	}
}

func TestDurableRemoveSyncsParentAfterRemoval(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "pending")
	if err := os.WriteFile(target, []byte("intent"), 0o600); err != nil {
		t.Fatal(err)
	}
	var synced string
	err := durableRemoveWithDirSync(target, func(path string) error {
		synced = path
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("target still present during parent sync: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("durableRemoveWithDirSync: %v", err)
	}
	if synced != dir {
		t.Fatalf("synced directory = %q, want %q", synced, dir)
	}
}

func TestDurableRemoveReportsPostRemoveDirectorySyncFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "pending")
	if err := os.WriteFile(target, []byte("intent"), 0o600); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected remove directory sync failure")
	err := durableRemoveWithDirSync(target, func(string) error { return injected })
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected directory sync failure", err)
	}
	if _, statErr := os.Lstat(target); !os.IsNotExist(statErr) {
		t.Fatalf("remove did not precede reported sync failure: %v", statErr)
	}
}

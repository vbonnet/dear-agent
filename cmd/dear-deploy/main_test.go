package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scaffold builds a repo root (with a manifest + sources) and a host home, and
// returns the common flags pointing at them. The manifest declares one required
// plist (with a token) and one optional unbuilt hook.
func scaffold(t *testing.T) (repo, home string) {
	t.Helper()
	repo = t.TempDir()
	home = t.TempDir()

	mustWrite(t, filepath.Join(repo, "deploy/launchd/x.plist"), "home=__HOME__\n")
	manifest := `artifacts:
  - name: x.plist
    source: deploy/launchd/x.plist
    deployed: ~/Library/LaunchAgents/x.plist
    mode: "0644"
    tokens:
      __HOME__: ${HOME}
  - name: hook
    source: bin/hook
    deployed: ~/.config/hooks/hook
    mode: "0755"
    optional: true
    remediation: make build-write-guards
`
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), manifest)
	return repo, home
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// invoke runs the CLI with the standard --repo-root/--home wiring appended.
func invoke(t *testing.T, repo, home string, args ...string) (int, string, string) {
	t.Helper()
	full := make([]string, 0, len(args)+4)
	full = append(full, args...)
	full = append(full, "--repo-root", repo, "--home", home)
	var out, errb bytes.Buffer
	code := run(full, &out, &errb)
	return code, out.String(), errb.String()
}

func TestList(t *testing.T) {
	repo, home := scaffold(t)
	code, out, errs := invoke(t, repo, home, "list")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%s", code, errs)
	}
	if !strings.Contains(out, "x.plist") || !strings.Contains(out, "hook") {
		t.Fatalf("list missing artifacts: %s", out)
	}
	if !strings.Contains(out, "2 artifact(s)") {
		t.Fatalf("list count wrong: %s", out)
	}
}

func TestStatus_MissingThenSynced(t *testing.T) {
	repo, home := scaffold(t)

	// Before any sync: required plist missing => exit 2; optional hook skipped.
	code, out, _ := invoke(t, repo, home, "status")
	if code != 2 {
		t.Fatalf("status exit = %d, want 2 (drift); out=%s", code, out)
	}
	if !strings.Contains(out, "MISSING") || !strings.Contains(out, "OUT OF SYNC") {
		t.Fatalf("status output unexpected: %s", out)
	}

	// Sync, then status should be clean (exit 0).
	if code, out, errs := invoke(t, repo, home, "sync"); code != 0 {
		t.Fatalf("sync exit = %d, out=%s err=%s", code, out, errs)
	}
	code, out, _ = invoke(t, repo, home, "status")
	if code != 0 {
		t.Fatalf("post-sync status exit = %d, want 0; out=%s", code, out)
	}
	if !strings.Contains(out, "In sync") {
		t.Fatalf("expected in-sync message: %s", out)
	}
}

func TestSync_DeploysAndRendersToken(t *testing.T) {
	repo, home := scaffold(t)
	code, out, errs := invoke(t, repo, home, "sync")
	if code != 0 {
		t.Fatalf("sync exit = %d, err=%s", code, errs)
	}
	if !strings.Contains(out, "installed") {
		t.Fatalf("sync output: %s", out)
	}
	deployed := filepath.Join(home, "Library/LaunchAgents/x.plist")
	got, err := os.ReadFile(deployed)
	if err != nil {
		t.Fatalf("read deployed: %v", err)
	}
	want := "home=" + home + "\n"
	if string(got) != want {
		t.Fatalf("rendered = %q, want %q", got, want)
	}

	// Second sync is idempotent: everything unchanged.
	_, out2, _ := invoke(t, repo, home, "sync")
	if !strings.Contains(out2, "unchanged") || strings.Contains(out2, "1 changed") {
		t.Fatalf("second sync not idempotent: %s", out2)
	}
}

func TestSync_SkipsOptionalUnbuilt(t *testing.T) {
	repo, home := scaffold(t)
	_, out, _ := invoke(t, repo, home, "sync", "hook")
	if !strings.Contains(out, "skipped") {
		t.Fatalf("expected optional hook skipped: %s", out)
	}
}

func TestInstall_ForcesRewrite(t *testing.T) {
	repo, home := scaffold(t)
	if code, _, errs := invoke(t, repo, home, "sync", "x.plist"); code != 0 {
		t.Fatalf("sync: %s", errs)
	}
	// install of an already-synced artifact must report a change (force rewrite),
	// where a second sync would report unchanged.
	_, out, _ := invoke(t, repo, home, "install", "x.plist")
	if !strings.Contains(out, "1 changed") {
		t.Fatalf("install did not force rewrite: %s", out)
	}
}

func TestSync_DryRunWritesNothing(t *testing.T) {
	repo, home := scaffold(t)
	code, out, _ := invoke(t, repo, home, "sync", "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run exit = %d", code)
	}
	if !strings.Contains(out, "dry-run") || !strings.Contains(out, "install") {
		t.Fatalf("dry-run output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(home, "Library/LaunchAgents/x.plist")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not write the deployed file")
	}
}

func TestPulsePreviewRendersTokensAndMatchesSync(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"),
		`{"pulses":[{"name":"__PULSE__","type":"file_mtime","path":"~/pulse","window":"1h"}]}`)
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"),
		`{"jobs":[{"name":"custom","pulse":"__PULSE__"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
    absent-only: true
    tokens:
      __PULSE__: custom-tick
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
    tokens:
      __PULSE__: custom-tick
`)
	host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	mustWrite(t, host, `{"pulses":[]}`)
	before, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name     string
		args     []string
		wantCode int
	}{
		{name: "status", args: []string{"status", pulseArtifactName}, wantCode: 2},
		{name: "dry-run", args: []string{"sync", pulseArtifactName, "--dry-run"}, wantCode: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errs := invoke(t, repo, home, tc.args...)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, tc.wantCode, out, errs)
			}
			if !strings.Contains(out, "absence-alarm-pulses:custom-tick") || strings.Contains(out, "__PULSE__") {
				t.Fatalf("preview did not report rendered pulse name: %s", out)
			}
			if tc.name == "status" {
				if !strings.Contains(out, "fix: dear-deploy sync absence-alarm-pulses") ||
					strings.Contains(out, "dear-deploy sync absence-alarm-pulses:custom-tick") {
					t.Fatalf("status offered an invalid synthetic selector: %s", out)
				}
			}
			after, err := os.ReadFile(host)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("preview changed host registry: got %q, want %q", after, before)
			}
			if _, err := os.Stat(host + ".offered"); !os.IsNotExist(err) {
				t.Fatalf("preview wrote pulse ledger: %v", err)
			}
		})
	}

	code, out, errs := invoke(t, repo, home, "sync", pulseArtifactName)
	if code != 0 {
		t.Fatalf("targeted sync exit = %d; stdout=%s stderr=%s", code, out, errs)
	}
	if got := pulseNamesAt(t, host); len(got) != 1 || got[0] != "custom-tick" {
		t.Fatalf("targeted sync merged %v, want [custom-tick]", got)
	}
	jobsPath := filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")
	if _, err := os.Lstat(jobsPath); !os.IsNotExist(err) {
		t.Fatalf("targeted pulse sync broadened selection to recovery jobs: %v", err)
	}
	if code, out, errs := invoke(t, repo, home, "status", pulseArtifactName); code != 0 {
		t.Fatalf("post-targeted-sync status exit = %d; stdout=%s stderr=%s", code, out, errs)
	}

	code, out, errs = invoke(t, repo, home, "sync")
	if code != 0 {
		t.Fatalf("full sync exit = %d; stdout=%s stderr=%s", code, out, errs)
	}
	jobsRaw, err := os.ReadFile(jobsPath)
	if err != nil {
		t.Fatalf("read deployed recovery jobs: %v", err)
	}
	if !bytes.Contains(jobsRaw, []byte(`"pulse":"custom-tick"`)) || bytes.Contains(jobsRaw, []byte("__PULSE__")) {
		t.Fatalf("deployed recovery jobs were not rendered: %s", jobsRaw)
	}
	if code, out, errs := invoke(t, repo, home, "status"); code != 0 {
		t.Fatalf("post-sync status exit = %d; stdout=%s stderr=%s", code, out, errs)
	}
}

func TestMergePulsesSeedsWithManifestMode(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"),
		`{"pulses":[{"name":"sandbox-gc-tick","type":"file_mtime","path":"~/pulse","window":"1h"}]}`)
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"),
		`{"jobs":[{"name":"sandbox-gc","pulse":"sandbox-gc-tick"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0640"
    absent-only: true
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
`)

	code, out, errs := invoke(t, repo, home, "merge-pulses")
	if code != 0 {
		t.Fatalf("merge-pulses exit = %d; stdout=%s stderr=%s", code, out, errs)
	}
	host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	info, err := os.Stat(host)
	if err != nil {
		t.Fatalf("stat seeded registry: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("seeded mode = %04o, want manifest mode 0640", got)
	}
}

func TestMergePulsesUsesRenderedManifestJobRegistry(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"),
		`{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/pulse","window":"1h"}]}`)
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"),
		`{"jobs":[{"name":"custom","pulse":"__PULSE__"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
    absent-only: true
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
    tokens:
      __PULSE__: custom-tick
`)
	host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	mustWrite(t, host, `{"pulses":[]}`)

	code, out, errs := invoke(t, repo, home, "merge-pulses")
	if code != 0 {
		t.Fatalf("merge-pulses exit = %d; stdout=%s stderr=%s", code, out, errs)
	}
	if got := pulseNamesAt(t, host); len(got) != 1 || got[0] != "custom-tick" {
		t.Fatalf("merge-pulses added %v, want [custom-tick]", got)
	}
}

func TestPulseCommandsFailWhenDeclaredJobRegistryCannotRender(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"),
		`{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/pulse","window":"1h"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
    absent-only: true
  - name: recovery-loop-jobs
    source: custom/missing-jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
`)
	host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	mustWrite(t, host, `{"pulses":[]}`)
	before, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "status", args: []string{"status", pulseArtifactName}},
		{name: "dry-run", args: []string{"sync", pulseArtifactName, "--dry-run"}},
		{name: "sync", args: []string{"sync", pulseArtifactName}},
		{name: "merge-pulses", args: []string{"merge-pulses"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, _, errs := invoke(t, repo, home, tc.args...)
			if code != 1 || !strings.Contains(errs, "render recovery job registry") {
				t.Fatalf("exit = %d, stderr=%s; want fail-loud dependency error", code, errs)
			}
			after, err := os.ReadFile(host)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("failed command changed host registry: got %q, want %q", after, before)
			}
			for _, suffix := range []string{".offered", ".offered.pending"} {
				if _, err := os.Lstat(host + suffix); !os.IsNotExist(err) {
					t.Fatalf("failed command wrote %s: %v", suffix, err)
				}
			}
		})
	}

	if err := os.Remove(host); err != nil {
		t.Fatal(err)
	}
	code, out, errs := invoke(t, repo, home, "status", pulseArtifactName)
	if code != 1 || !strings.Contains(out, "MISSING") || !strings.Contains(out, "ERROR") ||
		!strings.Contains(errs, "render recovery job registry") {
		t.Fatalf("mixed drift/error status exit = %d; stdout=%s stderr=%s", code, out, errs)
	}
}

func TestDryRunReturnsErrorForRequiredSourceFailure(t *testing.T) {
	repo, home := scaffold(t)
	if err := os.Remove(filepath.Join(repo, "deploy/launchd/x.plist")); err != nil {
		t.Fatal(err)
	}

	code, out, errs := invoke(t, repo, home, "sync", "x.plist", "--dry-run")
	if code != 1 || !strings.Contains(out, "ERROR: source not built") {
		t.Fatalf("dry-run exit = %d; stdout=%s stderr=%s", code, out, errs)
	}
}

func TestSyncDoesNotPublishJobsWhenPulseDependencyFails(t *testing.T) {
	for _, tc := range []struct {
		name          string
		pulseDefaults string
		jobs          string
		host          string
		wantError     string
	}{
		{
			name:          "invalid rendered jobs",
			pulseDefaults: `{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/pulse","window":"1h"}]}`,
			jobs:          `{not json`,
			wantError:     "parse recovery job registry",
		},
		{
			name:          "empty rendered jobs",
			pulseDefaults: `{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/pulse","window":"1h"}]}`,
			jobs:          `{"jobs":[]}`,
			wantError:     "contains no jobs",
		},
		{
			name:          "duplicate rendered jobs",
			pulseDefaults: `{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/pulse","window":"1h"}]}`,
			jobs:          `{"jobs":[{"name":"duplicate","pulse":"custom-tick"},{"name":"duplicate","pulse":"custom-tick"}]}`,
			wantError:     "duplicate job name",
		},
		{
			name:          "invalid pulse defaults",
			pulseDefaults: `{not json`,
			jobs:          `{"jobs":[{"name":"custom","pulse":"custom-tick"}]}`,
			wantError:     "parse pulse defaults",
		},
		{
			name:          "runtime-invalid pulse defaults",
			pulseDefaults: `{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/pulse"}]}`,
			jobs:          `{"jobs":[{"name":"custom","pulse":"custom-tick"}]}`,
			wantError:     "requires window",
		},
		{
			name:          "runtime-invalid live registry",
			pulseDefaults: `{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/pulse","window":"1h"}]}`,
			jobs:          `{"jobs":[{"name":"custom","pulse":"custom-tick"}]}`,
			host:          `{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/pulse"}]}`,
			wantError:     "requires window",
		},
		{
			name:          "undefined required pulse",
			pulseDefaults: `{"pulses":[{"name":"defined-tick","type":"file_mtime","path":"~/defined","window":"1h"}]}`,
			jobs:          `{"jobs":[{"name":"orphan","pulse":"orphan-tick"}]}`,
			wantError:     "orphan-tick",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			home := t.TempDir()
			mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), tc.pulseDefaults)
			mustWrite(t, filepath.Join(repo, "custom/jobs.json"), tc.jobs)
			mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
    absent-only: true
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
    absent-only: true
`)
			host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
			before := []byte(tc.host)
			if len(before) == 0 {
				before = []byte(`{"pulses":[]}`)
			}
			if err := os.MkdirAll(filepath.Dir(host), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(host, before, 0o640); err != nil {
				t.Fatal(err)
			}

			code, _, errs := invoke(t, repo, home, "sync")
			if code != 1 || !strings.Contains(errs, tc.wantError) {
				t.Fatalf("sync exit = %d; stderr=%s; want %q", code, errs, tc.wantError)
			}
			jobsPath := filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")
			if _, err := os.Lstat(jobsPath); !os.IsNotExist(err) {
				t.Fatalf("failed pulse dependency published jobs: %v", err)
			}
			after, err := os.ReadFile(host)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("failed pulse dependency changed registry: got %q, want %q", after, before)
			}
			for _, suffix := range []string{".offered", ".offered.pending"} {
				if _, err := os.Lstat(host + suffix); !os.IsNotExist(err) {
					t.Fatalf("failed pulse dependency wrote %s: %v", suffix, err)
				}
			}
		})
	}
}

func TestTargetedJobsSyncRequiresCurrentPulseState(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"),
		`{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/pulse","window":"1h"}]}`)
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"),
		`{"jobs":[{"name":"custom","pulse":"custom-tick"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
    absent-only: true
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
    absent-only: true
`)
	host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	mustWrite(t, host, `{"pulses":[]}`)
	jobsPath := filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")

	for _, args := range [][]string{
		{"sync", jobsArtifactName, "--dry-run"},
		{"sync", jobsArtifactName},
	} {
		code, out, errs := invoke(t, repo, home, args...)
		if code != 1 || !strings.Contains(out+errs, "sync absence-alarm-pulses first") {
			t.Fatalf("%v exit = %d; stdout=%s stderr=%s", args, code, out, errs)
		}
		if _, err := os.Lstat(jobsPath); !os.IsNotExist(err) {
			t.Fatalf("blocked targeted sync published jobs: %v", err)
		}
	}

	if code, out, errs := invoke(t, repo, home, "sync", pulseArtifactName); code != 0 {
		t.Fatalf("pulse sync exit = %d; stdout=%s stderr=%s", code, out, errs)
	}
	if code, out, errs := invoke(t, repo, home, "sync", jobsArtifactName); code != 0 {
		t.Fatalf("jobs sync exit = %d; stdout=%s stderr=%s", code, out, errs)
	}
	if _, err := os.Stat(jobsPath); err != nil {
		t.Fatalf("jobs were not deployed after pulse state became current: %v", err)
	}
}

func TestSyncDoesNotBypassLockedPulseSeedAfterMergeFailure(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), `{not json`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
    absent-only: true
`)

	code, _, errs := invoke(t, repo, home, "sync", pulseArtifactName)
	if code != 1 || !strings.Contains(errs, "parse pulse defaults") {
		t.Fatalf("sync exit = %d, stderr=%s; want loud merge failure", code, errs)
	}
	host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	if _, err := os.Lstat(host); !os.IsNotExist(err) {
		t.Fatalf("generic deploy published registry after locked merge failed: %v", err)
	}
	for _, suffix := range []string{".offered", ".offered.pending"} {
		if _, err := os.Lstat(host + suffix); !os.IsNotExist(err) {
			t.Fatalf("failed seed left %s: %v", suffix, err)
		}
	}
}

func pulseNamesAt(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Pulses []struct {
			Name string `json:"name"`
		} `json:"pulses"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(doc.Pulses))
	for _, pulse := range doc.Pulses {
		names = append(names, pulse.Name)
	}
	return names
}

func TestStatus_JSON(t *testing.T) {
	repo, home := scaffold(t)
	code, out, _ := invoke(t, repo, home, "status", "--json")
	if code != 2 { // plist still missing
		t.Fatalf("status --json exit = %d, want 2", code)
	}
	var results []map[string]any
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestUnknownSubcommandAndNoArgs(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run(nil, &out, &errb); code != 1 {
		t.Fatalf("no args exit = %d, want 1", code)
	}
	out.Reset()
	errb.Reset()
	if code := run([]string{"frobnicate"}, &out, &errb); code != 1 {
		t.Fatalf("unknown subcommand exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "unknown subcommand") {
		t.Fatalf("expected unknown-subcommand error: %s", errb.String())
	}
}

func TestUnknownArtifactErrors(t *testing.T) {
	repo, home := scaffold(t)
	code, _, errs := invoke(t, repo, home, "sync", "does-not-exist")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errs, "unknown artifact") {
		t.Fatalf("expected unknown-artifact error: %s", errs)
	}
}

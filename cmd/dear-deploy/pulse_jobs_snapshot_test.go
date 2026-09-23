package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/vbonnet/dear-agent/internal/deploy"
)

// Conformance: internal/deploy/SPEC.md DEP-33.
func TestJobsOnlyDeploysTheJobsSnapshotValidatedAgainstNormalPulse(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	prospective := `{"jobs":[{"name":"validated-job","pulse":"safe-tick"}]}`
	replacement := `{"jobs":[{"name":"late-job","pulse":"unvalidated-tick"}]}`
	pulses := `{"pulses":[{"name":"safe-tick","type":"file_mtime","path":"~/safe","window":"1h"}]}`
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), pulses)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), normalRegistryManifest(false, false))
	mustWrite(t, filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json"), pulses)
	writerDone := swappingJobsSource(t, repo, prospective, replacement)

	code, out, errs := invoke(t, repo, home, "sync", jobsArtifactName)
	if code != 0 {
		t.Fatalf("jobs-only sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	waitForSnapshotWriter(t, writerDone)
	assertFileContent(t, filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json"), prospective)
}

// Conformance: internal/deploy/SPEC.md DEP-33.
func TestJobsOnlyRejectsPulseSourceSwapAcrossCurrentCheck(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	jobs := `{"jobs":[{"name":"next-job","pulse":"safe-tick"}]}`
	validatedPulses := `{"pulses":[{"name":"safe-tick","type":"file_mtime","path":"~/safe","window":"1h"}]}`
	livePulses := `{"pulses":[{"name":"other-tick","type":"file_mtime","path":"~/other","window":"1h"}]}`
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"), jobs)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), normalRegistryManifest(false, false))
	mustWrite(t, filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json"), livePulses)
	writerDone := swappingArtifactSource(
		t, repo, "deploy/absence-alarm/pulses.json", validatedPulses, livePulses,
	)

	code, out, errs := invoke(t, repo, home, "sync", jobsArtifactName)
	waitForSnapshotWriter(t, writerDone)
	if code != 1 || !strings.Contains(errs, "is drift") {
		t.Fatalf("jobs-only sync exit=%d stdout=%s stderr=%s; want current-pulse failure", code, out, errs)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")); !os.IsNotExist(err) {
		t.Fatalf("jobs published after pulse source swap: %v", err)
	}
}

// Conformance: internal/deploy/SPEC.md DEP-33.
func TestJobsOnlyValidatesTheLivePulseSnapshotThatAuthorizesPublication(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	jobs := `{"jobs":[{"name":"next-job","pulse":"safe-tick"}]}`
	coveredPulses := `{"pulses":[{"name":"safe-tick","type":"file_mtime","path":"~/safe","window":"1h"}]}`
	replacementPulses := `{"pulses":[{"name":"other-tick","type":"file_mtime","path":"~/other","window":"1h"}]}`
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"), jobs)
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), coveredPulses)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), normalRegistryManifest(false, false))
	pulsePath := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	writerDone := swappingLiveFile(t, pulsePath, coveredPulses, replacementPulses)

	code, out, errs := invoke(t, repo, home, "sync", jobsArtifactName)
	waitForSnapshotWriter(t, writerDone)
	if code != 1 || !strings.Contains(errs, "validate live pulse config") {
		t.Fatalf("jobs-only sync exit=%d stdout=%s stderr=%s; want exact-live validation failure", code, out, errs)
	}
	if _, err := os.Lstat(filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")); !os.IsNotExist(err) {
		t.Fatalf("jobs published after live pulse swap: %v", err)
	}
}

// Conformance: internal/deploy/SPEC.md DEP-33.
func TestJobsOnlyDeploysTheJobsSnapshotValidatedAgainstAbsentOnlyPulse(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	prospective := `{"jobs":[{"name":"validated-job","pulse":"safe-tick"}]}`
	replacement := `{"jobs":[{"name":"late-job","pulse":"unvalidated-tick"}]}`
	pulses := `{"pulses":[{"name":"safe-tick","type":"file_mtime","path":"~/safe","window":"1h"}]}`
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), pulses)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), normalRegistryManifest(false, true))
	mustWrite(t, filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json"), pulses)
	writerDone := swappingJobsSource(t, repo, prospective, replacement)

	code, out, errs := invoke(t, repo, home, "sync", jobsArtifactName)
	if code != 0 {
		t.Fatalf("jobs-only sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	waitForSnapshotWriter(t, writerDone)
	assertFileContent(t, filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json"), prospective)
}

// Conformance: internal/deploy/SPEC.md DEP-33.
func TestAbsentOnlyPulsePairDeploysTheJobsSnapshotUsedForMerge(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	prospective := `{"jobs":[{"name":"validated-job","pulse":"safe-tick"}]}`
	replacement := `{"jobs":[{"name":"late-job","pulse":"unvalidated-tick"}]}`
	pulses := `{"pulses":[{"name":"safe-tick","type":"file_mtime","path":"~/safe","window":"1h"}]}`
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), pulses)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), normalRegistryManifest(false, true))
	mustWrite(t, filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json"), `{"pulses":[]}`)
	writerDone := swappingJobsSource(t, repo, prospective, replacement)

	code, out, errs := invoke(t, repo, home, "sync")
	if code != 0 {
		t.Fatalf("paired absent-only sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	waitForSnapshotWriter(t, writerDone)
	assertFileContent(t, filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json"), prospective)
}

// Conformance: internal/deploy/SPEC.md DEP-34.
func TestNormalRegistryPairSkipsOptionalMissingJobsSource(t *testing.T) {
	repo, home, jobsPath, pulsePath, pulseSource, liveJobs := optionalNormalJobsFixture(t, true)

	if code, out, errs := invoke(t, repo, home, "status", "--json"); code != 2 {
		t.Fatalf("status exit=%d stdout=%s stderr=%s; want drift status", code, out, errs)
	}
	code, out, errs := invoke(t, repo, home, "sync", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("dry-run exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var plans []deployPlan
	if err := json.Unmarshal([]byte(out), &plans); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, out)
	}
	foundSkip := false
	for _, plan := range plans {
		if plan.Name == jobsArtifactName && plan.WouldDo == "skip (no source)" {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Fatalf("dry-run plans=%+v, want optional jobs skip", plans)
	}

	for _, cmd := range []string{"sync", "install"} {
		code, out, errs = invoke(t, repo, home, cmd, "--json")
		if code != 0 {
			t.Fatalf("%s exit=%d stdout=%s stderr=%s", cmd, code, out, errs)
		}
		var results []deploy.Result
		if err := json.Unmarshal([]byte(out), &results); err != nil {
			t.Fatalf("decode %s: %v\n%s", cmd, err, out)
		}
		var jobsAction deploy.Action
		for _, result := range results {
			if result.Name == jobsArtifactName {
				jobsAction = result.Action
			}
		}
		if jobsAction != deploy.ActionSkipped {
			t.Fatalf("%s results=%+v, want skipped optional jobs", cmd, results)
		}
		assertFileContent(t, jobsPath, liveJobs)
		assertFileContent(t, pulsePath, pulseSource)
	}
}

// Conformance: internal/deploy/SPEC.md DEP-34.
func TestNormalRegistryPairOptionalMissingJobsUsesLiveRequirements(t *testing.T) {
	repo, home, jobsPath, pulsePath, _, liveJobs := optionalNormalJobsFixture(t, false)
	pulseBefore, err := os.ReadFile(pulsePath)
	if err != nil {
		t.Fatal(err)
	}

	code, out, errs := invoke(t, repo, home, "sync")
	if code != 1 || !strings.Contains(errs, "live-tick") {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s; want live requirement failure", code, out, errs)
	}
	assertFileContent(t, jobsPath, liveJobs)
	assertFileContent(t, pulsePath, string(pulseBefore))
}

// Conformance: internal/deploy/SPEC.md DEP-34.
func TestNormalRegistryPairOptionalMissingJobsAllowsMissingLiveRegistry(t *testing.T) {
	repo, home, jobsPath, pulsePath, pulseSource, _ := optionalNormalJobsFixture(t, true)
	if err := os.Remove(jobsPath); err != nil {
		t.Fatal(err)
	}

	code, out, errs := invoke(t, repo, home, "sync", "--json")
	if code != 0 {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var results []deploy.Result
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("decode sync: %v\n%s", err, out)
	}
	foundSkip := false
	for _, result := range results {
		if result.Name == jobsArtifactName && result.Action == deploy.ActionSkipped {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Fatalf("sync results=%+v, want skipped optional jobs", results)
	}
	if _, err := os.Lstat(jobsPath); !os.IsNotExist(err) {
		t.Fatalf("optional jobs target exists after pinned missing source/live state: %v", err)
	}
	assertFileContent(t, pulsePath, pulseSource)
}

// Conformance: internal/deploy/SPEC.md DEP-34.
func TestNormalRegistryPairOptionalMissingJobsRejectsUnsafeLiveRegistry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		wantError string
		makeLive  func(t *testing.T, path string)
	}{
		{
			name:      "runtime-invalid",
			wantError: "parse recovery job registry",
			makeLive: func(t *testing.T, path string) {
				t.Helper()
				mustWrite(t, path, `{not json`)
			},
		},
		{
			name:      "unreadable",
			wantError: "read live recovery job registry",
			makeLive: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, home, jobsPath, pulsePath, _, _ := optionalNormalJobsFixture(t, true)
			pulseBefore, err := os.ReadFile(pulsePath)
			if err != nil {
				t.Fatal(err)
			}
			tc.makeLive(t, jobsPath)

			code, out, errs := invoke(t, repo, home, "sync")
			if code != 1 || !strings.Contains(errs, tc.wantError) {
				t.Fatalf("sync exit=%d stdout=%s stderr=%s; want %q", code, out, errs, tc.wantError)
			}
			assertFileContent(t, pulsePath, string(pulseBefore))
		})
	}
}

// Conformance: internal/deploy/SPEC.md DEP-34.
func TestNormalRegistryPairPinsMissingOptionalJobsSnapshot(t *testing.T) {
	repo, home, jobsPath, pulsePath, pulseSource, liveJobs := optionalNormalJobsFixture(t, true)
	if err := os.Remove(jobsPath); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(jobsPath, 0o600); err != nil {
		t.Fatalf("mkfifo live jobs: %v", err)
	}
	replacementLive := filepath.Join(home, "replacement-live-jobs.json")
	mustWrite(t, replacementLive, liveJobs)
	source := filepath.Join(repo, "custom/jobs.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	lateSource := `{"jobs":[{"name":"late-job","pulse":"unvalidated-tick"}]}`
	writerDone := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(jobsPath, os.O_WRONLY, 0)
		if err == nil {
			err = os.WriteFile(source, []byte(lateSource), 0o644)
		}
		if err == nil {
			err = os.Rename(replacementLive, jobsPath)
		}
		if err == nil {
			_, err = io.WriteString(f, liveJobs)
		}
		if f != nil {
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
		}
		writerDone <- err
	}()

	code, out, errs := invoke(t, repo, home, "sync", "--json")
	if code != 0 {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	waitForSnapshotWriter(t, writerDone)
	var results []deploy.Result
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("decode sync: %v\n%s", err, out)
	}
	foundSkip := false
	for _, result := range results {
		if result.Name == jobsArtifactName && result.Action == deploy.ActionSkipped {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Fatalf("sync results=%+v, want pinned optional skip", results)
	}
	assertFileContent(t, jobsPath, liveJobs)
	assertFileContent(t, pulsePath, pulseSource)
}

func waitForSnapshotWriter(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("snapshot writer: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot writer did not observe the expected source read")
	}
}

func swappingJobsSource(t *testing.T, repo, prospective, replacement string) <-chan error {
	t.Helper()
	return swappingArtifactSource(t, repo, "custom/jobs.json", prospective, replacement)
}

func swappingArtifactSource(
	t *testing.T,
	repo, sourceRel, prospective, replacement string,
) <-chan error {
	t.Helper()
	source := filepath.Join(repo, sourceRel)
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	stem := strings.ReplaceAll(sourceRel, "/", "-")
	fifo := filepath.Join(repo, stem+".fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("mkfifo source: %v", err)
	}
	replacementPath := filepath.Join(repo, stem+".replacement")
	mustWrite(t, replacementPath, replacement)
	if err := os.Symlink(fifo, source); err != nil {
		t.Fatalf("symlink source: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err == nil {
			_, err = io.WriteString(f, prospective)
		}
		if err == nil {
			nextLink := source + ".next"
			if err = os.Symlink(replacementPath, nextLink); err == nil {
				err = os.Rename(nextLink, source)
			}
		}
		if f != nil {
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
		}
		done <- err
	}()
	return done
}

func swappingLiveFile(t *testing.T, path, first, replacement string) <-chan error {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo live path: %v", err)
	}
	replacementPath := path + ".replacement"
	mustWrite(t, replacementPath, replacement)

	done := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			_, err = io.WriteString(f, first)
		}
		if err == nil {
			err = os.Rename(replacementPath, path)
		}
		if f != nil {
			if closeErr := f.Close(); err == nil {
				err = closeErr
			}
		}
		done <- err
	}()
	return done
}

func normalRegistryManifest(optionalJobs, absentOnlyPulse bool) string {
	optional := ""
	if optionalJobs {
		optional = "    optional: true\n"
	}
	absentOnly := ""
	if absentOnlyPulse {
		absentOnly = "    absent-only: true\n"
	}
	return `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
` + optional + `  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
` + absentOnly
}

func optionalNormalJobsFixture(
	t *testing.T,
	coverLive bool,
) (repo, home, jobsPath, pulsePath, pulseSource, liveJobs string) {
	t.Helper()
	repo = t.TempDir()
	home = t.TempDir()
	liveJobs = `{"jobs":[{"name":"live-job","pulse":"live-tick"}]}`
	pulseName := "other-tick"
	if coverLive {
		pulseName = "live-tick"
	}
	pulseSource = `{"pulses":[{"name":"` + pulseName + `","type":"file_mtime","path":"~/next","window":"1h"}]}`
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), pulseSource)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), normalRegistryManifest(true, false))
	jobsPath = filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")
	pulsePath = filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	mustWrite(t, jobsPath, liveJobs)
	mustWrite(t, pulsePath,
		`{"pulses":[{"name":"live-tick","type":"file_mtime","path":"~/old","window":"1h"}]}`)
	return repo, home, jobsPath, pulsePath, pulseSource, liveJobs
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s=%s, want %s", path, got, want)
	}
}

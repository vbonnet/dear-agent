package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/deploy"
)

// Conformance: internal/deploy/SPEC.md DEP-32.
func TestOptionalNormalPulseMissingSourcePreservesPulseOnlySkip(t *testing.T) {
	repo, home, _, pulsePath := optionalNormalPulseFixture(t)

	code, out, errs := invoke(t, repo, home, "status", pulseArtifactName, "--json")
	if code != 0 {
		t.Fatalf("status exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var statuses []deploy.StatusResult
	if err := json.Unmarshal([]byte(out), &statuses); err != nil {
		t.Fatalf("decode status: %v\n%s", err, out)
	}
	if len(statuses) != 1 || statuses[0].State != deploy.StateSourceMissing || !statuses[0].Optional {
		t.Fatalf("status=%+v, want optional source-missing", statuses)
	}

	code, out, errs = invoke(t, repo, home, "sync", pulseArtifactName, "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("dry-run exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var plans []deployPlan
	if err := json.Unmarshal([]byte(out), &plans); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, out)
	}
	if len(plans) != 1 || plans[0].WouldDo != "skip (no source)" {
		t.Fatalf("dry-run=%+v, want optional skip", plans)
	}

	code, out, errs = invoke(t, repo, home, "sync", pulseArtifactName, "--json")
	if code != 0 {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var results []deploy.Result
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("decode sync: %v\n%s", err, out)
	}
	if len(results) != 1 || results[0].Action != deploy.ActionSkipped {
		t.Fatalf("sync=%+v, want optional skip", results)
	}
	for _, path := range []string{pulsePath, pulsePath + ".lock"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("optional skip created %s: %v", path, err)
		}
	}
}

func TestOptionalNormalPulseMissingSourcePreservesExistingTargetWithoutSidecar(t *testing.T) {
	repo, home, _, pulsePath := optionalNormalPulseFixture(t)
	want := `{"pulses":[{"name":"live-tick","type":"file_mtime","path":"~/live","window":"1h"}]}`
	mustWrite(t, pulsePath, want)

	code, out, errs := invoke(t, repo, home, "sync", pulseArtifactName, "--json")
	if code != 0 {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var results []deploy.Result
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("decode sync: %v\n%s", err, out)
	}
	if len(results) != 1 || results[0].Action != deploy.ActionSkipped {
		t.Fatalf("sync=%+v, want optional skip", results)
	}
	got, err := os.ReadFile(pulsePath)
	if err != nil || string(got) != want {
		t.Fatalf("optional skip changed existing target: got=%s err=%v want=%s", got, err, want)
	}
	if _, err := os.Lstat(pulsePath + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("optional skip created publication lock: %v", err)
	}
}

func TestOptionalNormalPulseMissingSourceBlocksSelectedJobs(t *testing.T) {
	repo, home, jobsPath, _ := optionalNormalPulseFixture(t)
	wantJobs, err := os.ReadFile(jobsPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"sync", "--dry-run"},
		{"sync"},
	} {
		code, out, errs := invoke(t, repo, home, args...)
		if code != 1 || !strings.Contains(errs, "pulse source is unavailable; refusing to publish recovery-loop-jobs") {
			t.Fatalf("%v exit=%d stdout=%s stderr=%s", args, code, out, errs)
		}
		got, err := os.ReadFile(jobsPath)
		if err != nil || string(got) != string(wantJobs) {
			t.Fatalf("%v changed jobs: got=%s err=%v want=%s", args, got, err, wantJobs)
		}
	}
}

func TestOptionalNormalPulsePreflightFailurePreservesIndependentWork(t *testing.T) {
	repo, home, jobsPath, _ := optionalNormalPulseFixture(t)
	wantJobs, err := os.ReadFile(jobsPath)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(repo, "custom/independent.txt"), "independent\n")
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/missing-pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
    optional: true
  - name: independent
    source: custom/independent.txt
    deployed: ~/.config/dear-agent/independent.txt
    mode: "0644"
`)

	code, out, errs := invoke(t, repo, home, "sync", "--json")
	if code != 1 || !strings.Contains(errs, "pulse source is unavailable; refusing to publish recovery-loop-jobs") {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var results []deploy.Result
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("decode sync: %v\n%s", err, out)
	}
	if len(results) != 1 || results[0].Name != "independent" || results[0].Action != deploy.ActionInstalled {
		t.Fatalf("sync=%+v, want installed independent artifact only", results)
	}
	if got, err := os.ReadFile(filepath.Join(home, ".config/dear-agent/independent.txt")); err != nil || string(got) != "independent\n" {
		t.Fatalf("independent artifact: got=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(jobsPath); err != nil || string(got) != string(wantJobs) {
		t.Fatalf("failed pulse dependency changed jobs: got=%q err=%v want=%q", got, err, wantJobs)
	}
}

func TestOptionalNormalPulseSkipStillRequiresObservableJobs(t *testing.T) {
	for _, args := range [][]string{
		{"status", pulseArtifactName},
		{"sync", pulseArtifactName, "--dry-run"},
		{"sync", pulseArtifactName},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			repo, home, jobsPath, _ := optionalNormalPulseFixture(t)
			if err := os.Remove(jobsPath); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(jobsPath, 0o755); err != nil {
				t.Fatal(err)
			}

			code, out, errs := invoke(t, repo, home, args...)
			if code != 1 || !strings.Contains(errs, "read live recovery job registry") {
				t.Fatalf("%v exit=%d stdout=%s stderr=%s", args, code, out, errs)
			}
		})
	}
}

func optionalNormalPulseFixture(t *testing.T) (repo, home, jobsPath, pulsePath string) {
	t.Helper()
	repo = t.TempDir()
	home = t.TempDir()
	jobs := `{"jobs":[{"name":"live-job","pulse":"live-tick"}]}`
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"), jobs)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/missing-pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
    optional: true
`)
	jobsPath = filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")
	pulsePath = filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	mustWrite(t, jobsPath, jobs)
	return repo, home, jobsPath, pulsePath
}

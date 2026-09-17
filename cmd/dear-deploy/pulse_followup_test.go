package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/deploy"
	"golang.org/x/sys/unix"
)

func TestNormalPulseArtifactUsesValidatedExactSourceDeploy(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	source := `{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/new","window":"1h"}]}`
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), source)
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"),
		`{"jobs":[{"name":"custom","pulse":"custom-tick"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
`)
	host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	mustWrite(t, host, `{"pulses":[
  {"name":"custom-tick","type":"file_mtime","path":"~/old","window":"2h"},
  {"name":"removed-by-source","type":"file_mtime","path":"~/removed","window":"1h"}
]}`)

	code, out, errs := invoke(t, repo, home, "sync", pulseArtifactName, "--dry-run")
	if code != 0 || !strings.Contains(out, "update") || strings.Contains(out, "merge pulse") {
		t.Fatalf("normal pulse dry-run exit=%d stdout=%s stderr=%s", code, out, errs)
	}

	code, out, errs = invoke(t, repo, home, "sync", pulseArtifactName, "--json")
	if code != 0 {
		t.Fatalf("normal pulse sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var results []deploy.Result
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("decode sync JSON: %v\n%s", err, out)
	}
	if len(results) != 1 || results[0].Name != pulseArtifactName || results[0].Action != deploy.ActionUpdated {
		t.Fatalf("normal pulse result = %+v, want one updated artifact", results)
	}
	got, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != source {
		t.Fatalf("normal pulse host = %s, want exact source %s", got, source)
	}
	for _, suffix := range []string{".offered", ".offered.txn", ".lock"} {
		if _, err := os.Lstat(host + suffix); !os.IsNotExist(err) {
			t.Fatalf("normal pulse deployment created merge artifact %s: %v", suffix, err)
		}
	}
	if code, out, errs := invoke(t, repo, home, "status", pulseArtifactName); code != 0 {
		t.Fatalf("post-sync status exit=%d stdout=%s stderr=%s", code, out, errs)
	}
}

func TestNormalPulseArtifactDeploysTheSnapshotItValidated(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	valid := `{"pulses":[{"name":"required-tick","type":"file_mtime","path":"~/valid","window":"1h"}]}`
	invalid := `{"pulses":[]}`
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"),
		`{"jobs":[{"name":"custom","pulse":"required-tick"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
`)

	source := filepath.Join(repo, "deploy/absence-alarm/pulses.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(repo, "pulse-source.fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	replacement := filepath.Join(repo, "replacement-pulses.json")
	mustWrite(t, replacement, invalid)
	if err := os.Symlink(fifo, source); err != nil {
		t.Fatalf("symlink pulse source: %v", err)
	}

	writerDone := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			writerDone <- err
			return
		}
		if _, err = io.WriteString(f, valid); err == nil {
			nextLink := source + ".next"
			if err = os.Symlink(replacement, nextLink); err == nil {
				err = os.Rename(nextLink, source)
			}
		}
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		writerDone <- err
	}()

	code, out, errs := invoke(t, repo, home, "sync", pulseArtifactName, "--json")
	if err := <-writerDone; err != nil {
		t.Fatalf("replace source after first render: %v", err)
	}
	if code != 0 {
		t.Fatalf("normal pulse sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var results []deploy.Result
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("decode sync JSON: %v\n%s", err, out)
	}
	if len(results) != 1 {
		t.Fatalf("normal pulse result = %+v, want one artifact", results)
	}
	host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	deployed, err := os.ReadFile(host)
	if err != nil {
		t.Fatalf("read deployed pulse snapshot: %v", err)
	}
	if got := string(deployed); got != valid {
		t.Fatalf("deployed pulse snapshot = %s, want validated snapshot %s", got, valid)
	}
	if got, want := results[0].SHA256, fmt.Sprintf("%x", sha256.Sum256([]byte(valid))); got != want {
		t.Fatalf("deployed hash = %q, want validated snapshot hash %q", got, want)
	}
}

func TestPulsePreviewAggregatesNestedDriftUnderManifestSelector(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), `{"pulses":[
  {"name":"alpha-tick","type":"file_mtime","path":"~/alpha","window":"1h"},
  {"name":"beta-tick","type":"file_mtime","path":"~/beta","window":"1h"}
]}`)
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"), `{"jobs":[
  {"name":"alpha","pulse":"alpha-tick"},
  {"name":"beta","pulse":"beta-tick"}
]}`)
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

	t.Run("status JSON", func(t *testing.T) {
		code, out, errs := invoke(t, repo, home, "status", pulseArtifactName, "--json")
		if code != 2 {
			t.Fatalf("exit=%d stdout=%s stderr=%s", code, out, errs)
		}
		var rows []deploy.StatusResult
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("decode: %v\n%s", err, out)
		}
		if len(rows) != 1 {
			t.Fatalf("status rows=%+v, want one aggregate pulse row", rows)
		}
		assertOneSelectablePulseDetail(t, repo, home, rows[0].Name, rows[0].Detail, len(rows))
		if rows[0].DeployedPath != host {
			t.Fatalf("deployed path=%q, want %q", rows[0].DeployedPath, host)
		}
	})

	t.Run("dry-run JSON", func(t *testing.T) {
		code, out, errs := invoke(t, repo, home, "sync", pulseArtifactName, "--dry-run", "--json")
		if code != 0 {
			t.Fatalf("exit=%d stdout=%s stderr=%s", code, out, errs)
		}
		var rows []deployPlan
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("decode: %v\n%s", err, out)
		}
		if len(rows) != 1 {
			t.Fatalf("dry-run rows=%+v, want one aggregate pulse row", rows)
		}
		assertOneSelectablePulseDetail(t, repo, home, rows[0].Name, rows[0].Detail, len(rows))
		if rows[0].DeployedPath != host || rows[0].WouldDo != "merge pulses" {
			t.Fatalf("dry-run row=%+v, want real path and aggregate merge", rows[0])
		}
	})

	t.Run("jobs-only dependency rows stay selectable", func(t *testing.T) {
		code, out, errs := invoke(t, repo, home, "sync", jobsArtifactName, "--dry-run", "--json")
		if code != 1 {
			t.Fatalf("exit=%d stdout=%s stderr=%s", code, out, errs)
		}
		var rows []deployPlan
		if err := json.Unmarshal([]byte(out), &rows); err != nil {
			t.Fatalf("decode: %v\n%s", err, out)
		}
		if len(rows) != 2 {
			t.Fatalf("jobs-only rows=%+v, want jobs plus one aggregate pulse dependency", rows)
		}
		pulseRows := 0
		for _, row := range rows {
			if code, listOut, listErrs := invoke(t, repo, home, "list", row.Name); code != 0 {
				t.Fatalf("reported name %q is not selectable: stdout=%s stderr=%s", row.Name, listOut, listErrs)
			}
			if row.Name == pulseArtifactName {
				pulseRows++
				if !strings.Contains(row.Detail, "alpha-tick") || !strings.Contains(row.Detail, "beta-tick") {
					t.Fatalf("pulse dependency detail=%q", row.Detail)
				}
			}
		}
		if pulseRows != 1 {
			t.Fatalf("pulse dependency rows=%d, want one", pulseRows)
		}
	})
}

func assertOneSelectablePulseDetail(t *testing.T, repo, home, name, detail string, rows int) {
	t.Helper()
	if rows != 1 || name != pulseArtifactName || !strings.Contains(detail, "alpha-tick") ||
		!strings.Contains(detail, "beta-tick") {
		t.Fatalf("rows=%d name=%q detail=%q, want one aggregate selectable pulse row", rows, name, detail)
	}
	if code, out, errs := invoke(t, repo, home, "list", name); code != 0 {
		t.Fatalf("reported name is not selectable: exit=%d stdout=%s stderr=%s", code, out, errs)
	}
}

func TestMergePulsesRejectsNormalPulseArtifactWithoutMutation(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"),
		`{"pulses":[{"name":"custom-tick","type":"file_mtime","path":"~/new","window":"1h"}]}`)
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"),
		`{"jobs":[{"name":"custom","pulse":"custom-tick"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
`)
	host := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	before := `{"pulses":[{"name":"operator","type":"file_mtime","path":"~/operator","window":"2h"}]}`
	mustWrite(t, host, before)

	code, out, errs := invoke(t, repo, home, "merge-pulses")
	if code == 0 || !strings.Contains(errs, "not absent-only") {
		t.Fatalf("merge-pulses exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	after, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before {
		t.Fatalf("rejected merge changed normal pulse artifact: got %s want %s", after, before)
	}
}

func TestNormalPulseValidationPrecedesJobsRegardlessOfManifestOrder(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"),
		`{"pulses":[{"name":"different-tick","type":"file_mtime","path":"~/different","window":"1h"}]}`)
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"),
		`{"jobs":[{"name":"custom","pulse":"required-tick"}]}`)
	// Jobs intentionally precede pulses. Publication order must follow the
	// dependency, not the manifest's presentation order.
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
`)
	pulseHost := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	jobsHost := filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")
	pulseBefore := `{"pulses":[{"name":"operator","type":"file_mtime","path":"~/operator","window":"1h"}]}`
	jobsBefore := `{"jobs":[{"name":"operator-job"}]}`
	mustWrite(t, pulseHost, pulseBefore)
	mustWrite(t, jobsHost, jobsBefore)

	code, out, errs := invoke(t, repo, home, "sync")
	if code != 1 || !strings.Contains(errs, "required-tick") {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	for path, want := range map[string]string{pulseHost: pulseBefore, jobsHost: jobsBefore} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("failed validation changed %s: got %s want %s", path, got, want)
		}
	}
}

func TestJobsOnlyPublishRequiresNormalPulseArtifactCurrent(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"),
		`{"pulses":[{"name":"required-tick","type":"file_mtime","path":"~/source","window":"1h"}]}`)
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"),
		`{"jobs":[{"name":"custom","pulse":"required-tick"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
`)
	pulseHost := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	jobsHost := filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")
	// The required name exists, but exact-source fields drift. Additive merge
	// semantics would incorrectly treat this source-owned registry as current.
	mustWrite(t, pulseHost,
		`{"pulses":[{"name":"required-tick","type":"file_mtime","path":"~/operator","window":"2h"}]}`)
	jobsBefore := `{"jobs":[{"name":"operator-job"}]}`
	mustWrite(t, jobsHost, jobsBefore)

	for _, args := range [][]string{
		{"sync", jobsArtifactName, "--dry-run"},
		{"sync", jobsArtifactName},
	} {
		code, out, errs := invoke(t, repo, home, args...)
		if code != 1 || !strings.Contains(errs, "sync "+pulseArtifactName) {
			t.Fatalf("%v exit=%d stdout=%s stderr=%s", args, code, out, errs)
		}
		got, err := os.ReadFile(jobsHost)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != jobsBefore {
			t.Fatalf("jobs-only failure changed registry: got %s want %s", got, jobsBefore)
		}
	}

	if code, out, errs := invoke(t, repo, home, "sync", pulseArtifactName); code != 0 {
		t.Fatalf("pulse sync exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	if code, out, errs := invoke(t, repo, home, "sync", jobsArtifactName); code != 0 {
		t.Fatalf("jobs sync after pulse convergence exit=%d stdout=%s stderr=%s", code, out, errs)
	}
}

func TestStatusKeepsDependencyErrorOnDeclaredSelectorWhenPulseArtifactMissing(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"), `{"jobs":[{"name":"custom","pulse":"required-tick"}]}`)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
`)

	code, out, errs := invoke(t, repo, home, "status", jobsArtifactName, "--json")
	if code != 1 || !strings.Contains(errs, "without "+pulseArtifactName) {
		t.Fatalf("status exit=%d stdout=%s stderr=%s", code, out, errs)
	}
	var rows []deploy.StatusResult
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("decode status JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0].Name != jobsArtifactName || rows[0].State != deploy.StateError {
		t.Fatalf("status rows=%+v, want error on declared jobs selector", rows)
	}
	if code, out, errs := invoke(t, repo, home, "list", rows[0].Name); code != 0 {
		t.Fatalf("reported dependency name is not selectable: exit=%d stdout=%s stderr=%s", code, out, errs)
	}
}

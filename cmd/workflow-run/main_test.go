package main

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/sqlite"
	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// minimalBashWorkflow returns a one-node bash workflow YAML body that
// echoes a marker so we know the run executed. Used for CLI exit-path
// and persistence tests in this package.
const minimalBashWorkflow = `
name: cli-bash-roundtrip
version: "1"
inputs:
  - name: marker
    required: true
nodes:
  - id: greet
    kind: bash
    bash:
      cmd: echo "hello $INPUT_marker"
`

func TestRun_PersistsRunToSQLite(t *testing.T) {
	dir := t.TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(wfPath, []byte(minimalBashWorkflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	dbPath := filepath.Join(dir, "runs.db")

	stderr := tmpFile(t)
	args := []string{
		"-file", wfPath,
		"-db", dbPath,
		"-trigger", "cron",
		"-input", "marker=world",
	}
	if code := run(args, stderr); code != 0 {
		t.Fatalf("run exit %d, stderr=%s", code, readFile(stderr))
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("runs.db not created: %v", err)
	}

	db := openTestDB(t, dbPath)
	defer db.Close()

	var (
		runs    int
		trigger string
		state   string
	)
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*), COALESCE(MAX(trigger),''), COALESCE(MAX(state),'') FROM runs`,
	).Scan(&runs, &trigger, &state); err != nil {
		t.Fatalf("query runs: %v", err)
	}
	if runs != 1 {
		t.Errorf("runs row count = %d, want 1", runs)
	}
	if trigger != "cron" {
		t.Errorf("runs.trigger = %q, want %q (Bug 3 — flag must thread into Runner.Trigger)", trigger, "cron")
	}
	if state != "succeeded" {
		t.Errorf("runs.state = %q, want succeeded; stderr=%s", state, readFile(stderr))
	}
}

func TestRun_DefaultTriggerIsCLI(t *testing.T) {
	dir := t.TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(wfPath, []byte(minimalBashWorkflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	dbPath := filepath.Join(dir, "runs.db")

	stderr := tmpFile(t)
	args := []string{"-file", wfPath, "-db", dbPath, "-input", "marker=x"}
	if code := run(args, stderr); code != 0 {
		t.Fatalf("run exit %d, stderr=%s", code, readFile(stderr))
	}

	db := openTestDB(t, dbPath)
	defer db.Close()
	var trigger string
	if err := db.QueryRow(`SELECT trigger FROM runs LIMIT 1`).Scan(&trigger); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if trigger != "cli" {
		t.Errorf("default -trigger = %q, want cli", trigger)
	}
}

func TestRun_EmptyDBPathSkipsPersistence(t *testing.T) {
	dir := t.TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(wfPath, []byte(minimalBashWorkflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	// Run with -db="" should succeed without creating any DB file.
	stderr := tmpFile(t)
	args := []string{"-file", wfPath, "-db", "", "-input", "marker=x"}
	if code := run(args, stderr); code != 0 {
		t.Fatalf("run exit %d, stderr=%s", code, readFile(stderr))
	}
	// The current working directory must not contain runs.db either.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cwd, "runs.db")); err == nil {
		t.Error("-db=\"\" still created a runs.db")
	}
}

func TestRun_DryRunDoesNotCreateDB(t *testing.T) {
	dir := t.TempDir()
	wfPath := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(wfPath, []byte(minimalBashWorkflow), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	dbPath := filepath.Join(dir, "runs.db")
	stderr := tmpFile(t)
	args := []string{"-file", wfPath, "-db", dbPath, "-dry-run"}
	if code := run(args, stderr); code != 0 {
		t.Fatalf("run exit %d, stderr=%s", code, readFile(stderr))
	}
	if _, err := os.Stat(dbPath); err == nil {
		t.Error("dry-run created a runs.db")
	}
}

func TestRun_MissingFileFlag(t *testing.T) {
	stderr := tmpFile(t)
	if code := run([]string{}, stderr); code != 2 {
		t.Errorf("exit = %d, want 2 (missing -file); stderr=%s", code, readFile(stderr))
	}
	if !strings.Contains(readFile(stderr), "-file is required") {
		t.Errorf("missing usage hint in stderr=%s", readFile(stderr))
	}
}

func openTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func tmpFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "log-*")
	if err != nil {
		t.Fatalf("create tmp: %v", err)
	}
	return f
}

func readFile(f *os.File) string {
	if f == nil {
		return ""
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	b, _ := io.ReadAll(f)
	return string(b)
}

// TestUsesDirectlyBilledCredentials guards against attaching a CodexBar
// CLI/subscription reading to a family the router actually reaches through
// a directly-billed API key or Vertex AI credential — a different billing
// pool entirely (codex review on #1218).
func TestUsesDirectlyBilledCredentials(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	if usesDirectlyBilledCredentials("anthropic") {
		t.Error("no credential detected: want false")
	}
	if usesDirectlyBilledCredentials("ollama") {
		t.Error("local family: want false")
	}

	t.Setenv("OPENAI_API_KEY", "sk-test")
	if !usesDirectlyBilledCredentials("openai") {
		t.Error("OPENAI_API_KEY set: want true")
	}

	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	if !usesDirectlyBilledCredentials("anthropic") {
		t.Error("ANTHROPIC_API_KEY set: want true")
	}

	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")
	if !usesDirectlyBilledCredentials("anthropic") {
		t.Error("GOOGLE_CLOUD_PROJECT set (Vertex AI): want true")
	}
}

// TestCredentialFilteredReaderDropsDirectlyBilledFamilies guards the
// Reader wrapper itself: a snapshot family the router reaches through an
// API key must not reach the meter's cache at all.
func TestCredentialFilteredReaderDropsDirectlyBilledFamilies(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_PROJECT", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "sk-test")

	snapshot := &quota.Snapshot{
		GeneratedAt: time.Now(),
		Providers: []quota.ProviderQuota{
			{Family: "openai", Availability: quota.AvailabilityOK},
			{Family: "anthropic", Availability: quota.AvailabilityOK},
		},
	}
	reader := credentialFilteredReader{inner: fakeQuotaReader{snapshot: snapshot}}
	got, err := reader.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Providers) != 1 || got.Providers[0].Family != "anthropic" {
		t.Errorf("filtered providers = %+v, want only anthropic (openai is API-key billed)", got.Providers)
	}
}

type fakeQuotaReader struct{ snapshot *quota.Snapshot }

func (f fakeQuotaReader) Read(context.Context) (*quota.Snapshot, error) { return f.snapshot, nil }

func TestMeetsMinCodexBarVersion(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		want      bool
	}{
		{name: "audited version", installed: "0.49.0", want: true},
		{name: "well above the floor", installed: "0.49.2", want: true},
		{name: "future major version", installed: "1.0.0", want: true},
		{name: "below the floor", installed: "0.48.9", want: false},
		{name: "well below the floor", installed: "0.30.0", want: false},
		{name: "unparseable version does not meet the floor", installed: "not-a-version", want: false},
		{name: "empty version does not meet the floor", installed: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := meetsMinCodexBarVersion(tt.installed); got != tt.want {
				t.Errorf("meetsMinCodexBarVersion(%q) = %t, want %t", tt.installed, got, tt.want)
			}
		})
	}
}

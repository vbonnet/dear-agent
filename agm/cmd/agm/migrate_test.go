package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// ---------------------------------------------------------------------------
// Command registration
// ---------------------------------------------------------------------------

func TestMigrateCmd_Registration(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "migrate" {
			return
		}
	}
	t.Error("migrate not registered under rootCmd")
}

func TestMigrateClaudeToUnifiedCmd_Registration(t *testing.T) {
	for _, cmd := range migrateCmd.Commands() {
		if cmd.Use == "claude-to-unified" {
			return
		}
	}
	t.Error("claude-to-unified not registered under migrateCmd")
}

func TestMigrateClaudeToUnifiedCmd_DryRunFlag(t *testing.T) {
	flag := migrateClaudeToUnifiedCmd.Flags().Lookup("dry-run")
	if flag == nil {
		t.Fatal("--dry-run flag not registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("--dry-run default = %q, want %q", flag.DefValue, "false")
	}
}

func TestMigrateClaudeToUnifiedCmd_HasRunE(t *testing.T) {
	if migrateClaudeToUnifiedCmd.RunE == nil {
		t.Error("RunE not set on claude-to-unified command")
	}
}

// ---------------------------------------------------------------------------
// scanOldFormatDirs
// ---------------------------------------------------------------------------

func TestScanOldFormatDirs_Empty(t *testing.T) {
	dir := t.TempDir()
	entries, err := scanOldFormatDirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestScanOldFormatDirs_NotExist(t *testing.T) {
	_, err := scanOldFormatDirs("/nonexistent/path/abcdef123")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestScanOldFormatDirs_OldFormatOnly(t *testing.T) {
	dir := t.TempDir()
	mkdirs(t, dir, "claude-1-session", "my-project-session")
	// These should be ignored — not old-format
	mkdirs(t, dir, "session-other", "something-else")

	entries, err := scanOldFormatDirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.OldName, "-session") {
			t.Errorf("OldName %q does not end with -session", e.OldName)
		}
		if !strings.HasPrefix(e.NewName, "session-") {
			t.Errorf("NewName %q does not start with session-", e.NewName)
		}
	}
}

func TestScanOldFormatDirs_Mapping(t *testing.T) {
	dir := t.TempDir()
	mkdirs(t, dir, "claude-42-session")

	entries, err := scanOldFormatDirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.OldName != "claude-42-session" {
		t.Errorf("OldName = %q", e.OldName)
	}
	if e.NewName != "session-claude-42" {
		t.Errorf("NewName = %q", e.NewName)
	}
}

func TestScanOldFormatDirs_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	mkdirs(t, dir, ".archive-old-format", "real-session")

	entries, err := scanOldFormatDirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 (hidden dirs should be skipped)", len(entries))
	}
}

func TestScanOldFormatDirs_SkipsFiles(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "not-a-dir-session"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	mkdirs(t, dir, "actual-session")

	entries, err := scanOldFormatDirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 (files should be skipped)", len(entries))
	}
}

func TestScanOldFormatDirs_SortedAlphabetically(t *testing.T) {
	dir := t.TempDir()
	mkdirs(t, dir, "zebra-session", "apple-session", "mango-session")

	entries, err := scanOldFormatDirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	want := []string{"apple-session", "mango-session", "zebra-session"}
	for i, e := range entries {
		if e.OldName != want[i] {
			t.Errorf("entries[%d].OldName = %q, want %q", i, e.OldName, want[i])
		}
	}
}

func TestScanOldFormatDirs_SkipsAlreadyUnifiedFormat(t *testing.T) {
	dir := t.TempDir()
	// "session-foo" is already in unified format — must be ignored.
	mkdirs(t, dir, "session-foo", "old-session")

	entries, err := scanOldFormatDirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1 (session-foo must be skipped)", len(entries))
	}
	if len(entries) == 1 && entries[0].OldName != "old-session" {
		t.Errorf("entries[0].OldName = %q, want %q", entries[0].OldName, "old-session")
	}
}

// TestScanOldFormatDirs_IdempotentOnRerun reproduces the session-*-session
// double-prefix corruption: a session named "foo-session" produces old dir
// "foo-session-session"; after migration it becomes "session-foo-session",
// which still ends with "-session". Without the session- prefix guard a second
// run would corrupt it to "session-session-foo".
func TestScanOldFormatDirs_IdempotentOnRerun(t *testing.T) {
	dir := t.TempDir()
	// Simulate state after first migration run: "session-foo-session" exists.
	// This directory still ends with "-session" but must not be picked up again.
	mkdirs(t, dir, "session-foo-session")

	entries, err := scanOldFormatDirs(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0 — session-foo-session is already in unified format "+
			"(it starts with 'session-') and must not be renamed again", len(entries))
	}
}

// TestMigrateClaudeToUnified_Idempotent ensures that running the migration
// twice does not corrupt directories whose names end with "-session" after
// the first migration (e.g. the session was named "foo-session").
func TestMigrateClaudeToUnified_Idempotent(t *testing.T) {
	dir, restore := migrateTestSetup(t)
	defer restore()
	// "foo-session-session" is the old-format dir for a session named "foo-session".
	// After first run it becomes "session-foo-session".
	mkdirs(t, dir, "foo-session-session")

	migrateDryRun = false

	var buf bytes.Buffer
	migrateClaudeToUnifiedCmd.SetOut(&buf)
	defer migrateClaudeToUnifiedCmd.SetOut(nil)

	// First run: should rename foo-session-session → session-foo-session.
	if err := runMigrateClaudeToUnified(migrateClaudeToUnifiedCmd, nil); err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "session-foo-session")); os.IsNotExist(err) {
		t.Fatal("first run: session-foo-session not created")
	}

	buf.Reset()

	// Second run: session-foo-session must NOT be renamed to session-session-foo.
	if err := runMigrateClaudeToUnified(migrateClaudeToUnifiedCmd, nil); err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "session-session-foo")); err == nil {
		t.Error("second run corrupted session-foo-session → session-session-foo (double-prefix bug)")
	}
	if _, err := os.Stat(filepath.Join(dir, "session-foo-session")); os.IsNotExist(err) {
		t.Error("second run: session-foo-session no longer exists (should be preserved)")
	}
	if !strings.Contains(buf.String(), "No old-format") {
		t.Errorf("second run output %q should indicate no old-format dirs found", buf.String())
	}
}

// ---------------------------------------------------------------------------
// runMigrateClaudeToUnified integration tests
// ---------------------------------------------------------------------------

func TestMigrateClaudeToUnified_NoOldDirs(t *testing.T) {
	dir, restore := migrateTestSetup(t)
	defer restore()
	mkdirs(t, dir, "session-existing")

	var buf bytes.Buffer
	migrateClaudeToUnifiedCmd.SetOut(&buf)
	defer migrateClaudeToUnifiedCmd.SetOut(nil)

	if err := runMigrateClaudeToUnified(migrateClaudeToUnifiedCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "No old-format") {
		t.Errorf("output %q missing 'No old-format'", buf.String())
	}
}

func TestMigrateClaudeToUnified_DryRun_NoRename(t *testing.T) {
	dir, restore := migrateTestSetup(t)
	defer restore()
	mkdirs(t, dir, "claude-1-session")

	migrateDryRun = true
	defer func() { migrateDryRun = false }()

	var buf bytes.Buffer
	migrateClaudeToUnifiedCmd.SetOut(&buf)
	defer migrateClaudeToUnifiedCmd.SetOut(nil)

	if err := runMigrateClaudeToUnified(migrateClaudeToUnifiedCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Old dir must still exist
	if _, err := os.Stat(filepath.Join(dir, "claude-1-session")); os.IsNotExist(err) {
		t.Error("old dir removed during dry-run")
	}
	// New dir must NOT exist
	if _, err := os.Stat(filepath.Join(dir, "session-claude-1")); err == nil {
		t.Error("new dir created during dry-run")
	}
	if !strings.Contains(buf.String(), "Dry run") {
		t.Errorf("output %q missing 'Dry run'", buf.String())
	}
}

func TestMigrateClaudeToUnified_Apply_Renames(t *testing.T) {
	dir, restore := migrateTestSetup(t)
	defer restore()
	mkdirs(t, dir, "claude-2-session")

	migrateDryRun = false

	var buf bytes.Buffer
	migrateClaudeToUnifiedCmd.SetOut(&buf)
	defer migrateClaudeToUnifiedCmd.SetOut(nil)

	if err := runMigrateClaudeToUnified(migrateClaudeToUnifiedCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Old dir must be gone
	if _, err := os.Stat(filepath.Join(dir, "claude-2-session")); err == nil {
		t.Error("old dir still exists after apply")
	}
	// New dir must exist
	if _, err := os.Stat(filepath.Join(dir, "session-claude-2")); os.IsNotExist(err) {
		t.Error("new dir not created after apply")
	}
	if !strings.Contains(buf.String(), "Renamed 1") {
		t.Errorf("output %q missing 'Renamed 1'", buf.String())
	}
}

func TestMigrateClaudeToUnified_SkipsConflict(t *testing.T) {
	dir, restore := migrateTestSetup(t)
	defer restore()
	// Both old and new format present — old should be skipped
	mkdirs(t, dir, "claude-3-session", "session-claude-3")

	migrateDryRun = false

	var buf bytes.Buffer
	migrateClaudeToUnifiedCmd.SetOut(&buf)
	defer migrateClaudeToUnifiedCmd.SetOut(nil)

	if err := runMigrateClaudeToUnified(migrateClaudeToUnifiedCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Old dir must still exist (was skipped)
	if _, err := os.Stat(filepath.Join(dir, "claude-3-session")); os.IsNotExist(err) {
		t.Error("old dir removed despite conflict — should have been skipped")
	}
	if !strings.Contains(buf.String(), "skip") {
		t.Errorf("output %q missing 'skip'", buf.String())
	}
	if !strings.Contains(buf.String(), "skipped 1") {
		t.Errorf("output %q missing 'skipped 1'", buf.String())
	}
}

func TestMigrateClaudeToUnified_SessionsDirNotExist(t *testing.T) {
	old := cfg
	cfg = &config.Config{SessionsDir: "/nonexistent/dir/abcdef"}
	defer func() { cfg = old }()

	var buf bytes.Buffer
	migrateClaudeToUnifiedCmd.SetOut(&buf)
	defer migrateClaudeToUnifiedCmd.SetOut(nil)

	if err := runMigrateClaudeToUnified(migrateClaudeToUnifiedCmd, nil); err != nil {
		t.Fatalf("expected nil error for missing dir, got: %v", err)
	}
	if !strings.Contains(buf.String(), "not found") {
		t.Errorf("output %q missing 'not found'", buf.String())
	}
}

// ---------------------------------------------------------------------------
// buildMigrationPlan (v2-to-v3)
// ---------------------------------------------------------------------------

func mkManifest(id, harness string) *manifest.Manifest {
	return &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     id,
		Name:          "s-" + id,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: "/tmp/" + id},
		Tmux:          manifest.Tmux{SessionName: "s-" + id},
		Harness:       harness,
	}
}

func TestBuildMigrationPlan_CountsWritesAndValidity(t *testing.T) {
	manifests := []*manifest.Manifest{
		mkManifest("aaaaaaaa-0000-0000-0000-000000000001", "claude-code"), // ready
		mkManifest("bbbbbbbb-0000-0000-0000-000000000002", ""),            // needs harness backfill
		mkManifest("cccccccc-0000-0000-0000-000000000003", "gemini-cli"),  // ready
	}

	plan := buildMigrationPlan(manifests)

	if plan.Total != 3 {
		t.Errorf("Total = %d, want 3", plan.Total)
	}
	if plan.NeedsWrite() != 1 {
		t.Errorf("NeedsWrite = %d, want 1", plan.NeedsWrite())
	}
	if plan.Invalid() != 0 {
		t.Errorf("Invalid = %d, want 0 (all migrate to valid v3)", plan.Invalid())
	}
}

func TestBuildMigrationPlan_Empty(t *testing.T) {
	plan := buildMigrationPlan(nil)
	if plan.Total != 0 || plan.NeedsWrite() != 0 || plan.Invalid() != 0 {
		t.Errorf("empty plan = %+v, want all zero", plan)
	}
}

func TestBuildMigrationPlan_ReportsHarnessBackfill(t *testing.T) {
	plan := buildMigrationPlan([]*manifest.Manifest{
		mkManifest("dddddddd-0000-0000-0000-000000000004", ""),
	})

	if len(plan.Sessions) != 1 {
		t.Fatalf("len(Sessions) = %d, want 1", len(plan.Sessions))
	}
	changes := plan.Sessions[0].Changes
	if len(changes) != 1 || changes[0].Field != "harness" {
		t.Errorf("changes = %+v, want a single harness backfill", changes)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mkdirs(t *testing.T, base string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.MkdirAll(filepath.Join(base, name), 0755); err != nil {
			t.Fatalf("mkdirs %s: %v", name, err)
		}
	}
}

func migrateTestSetup(t *testing.T) (sessionsDir string, restore func()) {
	t.Helper()
	dir := t.TempDir()
	old := cfg
	cfg = &config.Config{SessionsDir: dir}
	return dir, func() { cfg = old }
}

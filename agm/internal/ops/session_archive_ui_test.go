package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func uiSession(id string, lastActivityMs int64, archived, cwd string) string {
	// cwd is passed explicitly so tests can collide it with the PID registry.
	return fmt.Sprintf(
		`{"sessionId":"%s","cliSessionId":"cli-%s","cwd":"%s","originCwd":"/Users/x",`+
			`"createdAt":1700000000000,"lastActivityAt":%d,"model":"m","isArchived":%s,`+
			`"title":"t %s","permissionMode":"auto","completedTurns":1}`,
		id, id, cwd, lastActivityMs, archived, id)
}

// uiFixture lays out a store + PID registry and returns a request pinned to a
// fixed "now" so age math is deterministic.
func uiFixture(t *testing.T, files map[string]string, pidRecords []string) *ArchiveUISessionsRequest {
	t.Helper()
	root := t.TempDir()
	storeRoot := filepath.Join(root, "store")
	dir := filepath.Join(storeRoot, "dev1", "acct1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pidDir := filepath.Join(root, "pidreg")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, rec := range pidRecords {
		if err := os.WriteFile(filepath.Join(pidDir, fmt.Sprintf("%d.json", i)), []byte(rec), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &ArchiveUISessionsRequest{
		Now:            time.UnixMilli(1_800_000_000_000), // fixed reference
		HomeDir:        root,
		StoreRoot:      storeRoot,
		PIDRegistryDir: pidDir,
		OlderThan:      7 * 24 * time.Hour,
	}
}

func outcomeBySession(r *ArchiveUISessionsResult, id string) (UISessionOutcome, bool) {
	for _, s := range r.Sessions {
		if s.SessionID == id {
			return s, true
		}
	}
	return UISessionOutcome{}, false
}

const (
	dayMs = int64(24 * 60 * 60 * 1000)
	nowMs = int64(1_800_000_000_000)
)

func TestArchiveUI_DryRunIdleFiltering(t *testing.T) {
	req := uiFixture(t, map[string]string{
		"local_old.json":     uiSession("old", nowMs-30*dayMs, "false", "/w/old"),
		"local_recent.json":  uiSession("recent", nowMs-1*dayMs, "false", "/w/recent"),
		"local_arch.json":    uiSession("arch", nowMs-30*dayMs, "true", "/w/arch"),
		"local_liveid.json":  uiSession("liveid", nowMs-30*dayMs, "false", "/w/liveid"),
		"local_livecwd.json": uiSession("livecwd", nowMs-30*dayMs, "false", "/w/livecwd"),
	}, []string{
		`{"pid":1,"sessionId":"cli-liveid","cwd":"/somewhere"}`,
		`{"pid":2,"sessionId":"other","cwd":"/w/livecwd"}`,
	})

	r, err := ArchiveUISessions(&OpContext{}, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.DryRun {
		t.Fatal("expected dry-run by default (Apply=false)")
	}
	// "livecwd" is idle 30d and only SHARES a cwd with a live process (pid:2,
	// session "other") — it is not itself live. Liveness is id-based, so it must
	// be archivable, not skipped. Only "liveid" (whose cliSessionId matches a
	// live registry record) is genuinely live. See isLive / ADR-026.
	if r.Changed != 2 || r.Errors != 0 {
		t.Fatalf("expected exactly 2 would-archive, 0 errors; got changed=%d skipped=%d errors=%d",
			r.Changed, r.Skipped, r.Errors)
	}
	checks := map[string][2]string{
		"old":     {"would-archive", ""},
		"recent":  {"skip", uiSkipTooRecent},
		"arch":    {"skip", uiSkipAlreadyArchived},
		"liveid":  {"skip", uiSkipLive},
		"livecwd": {"would-archive", ""},
	}
	for id, want := range checks {
		oc, ok := outcomeBySession(r, id)
		if !ok {
			t.Fatalf("missing outcome for %s", id)
		}
		if oc.Action != want[0] || oc.Reason != want[1] {
			t.Errorf("%s: got action=%s reason=%s, want action=%s reason=%s",
				id, oc.Action, oc.Reason, want[0], want[1])
		}
	}
	// Dry-run must not have mutated anything on disk.
	raw, _ := os.ReadFile(filepath.Join(r.Store, "local_old.json"))
	if string(raw) != uiSession("old", nowMs-30*dayMs, "false", "/w/old") {
		t.Fatal("dry-run modified a file")
	}
}

func TestArchiveUI_StatusAllIgnoresAgeButNotLive(t *testing.T) {
	req := uiFixture(t, map[string]string{
		"local_recent.json": uiSession("recent", nowMs-1*dayMs, "false", "/w/recent"),
		"local_live.json":   uiSession("live", nowMs-1*dayMs, "false", "/w/live"),
	}, []string{`{"pid":1,"sessionId":"cli-live","cwd":"/x"}`})
	req.Status = "all"

	r, err := ArchiveUISessions(&OpContext{}, req)
	if err != nil {
		t.Fatal(err)
	}
	if oc, _ := outcomeBySession(r, "recent"); oc.Action != "would-archive" {
		t.Errorf("status=all should ignore age: recent got %s:%s", oc.Action, oc.Reason)
	}
	if oc, _ := outcomeBySession(r, "live"); oc.Action != "skip" || oc.Reason != uiSkipLive {
		t.Errorf("status=all must still skip live: got %s:%s", oc.Action, oc.Reason)
	}
}

// TestArchiveUI_CwdSharedWithLiveProcessIsNotLive is the regression guard for
// the cwd-liveness false-positive: a session that merely shares a cwd with a
// live process owning a DIFFERENT session must not be treated as live. On a
// monorepo (many sessions rooted at ~/src/<repo>) the old cwd match buried
// hundreds of long-idle sessions; liveness is now identity-based only.
func TestArchiveUI_CwdSharedWithLiveProcessIsNotLive(t *testing.T) {
	const shared = "/Users/x/src/monorepo"
	req := uiFixture(t, map[string]string{
		// Idle 30d; its own cliSessionId (cli-idle) is NOT in the registry.
		"local_idle.json": uiSession("idle", nowMs-30*dayMs, "false", shared),
		// A genuinely live session in the same directory (cli-busy is live).
		"local_busy.json": uiSession("busy", nowMs-1*dayMs, "false", shared),
	}, []string{`{"pid":1,"sessionId":"cli-busy","cwd":"` + shared + `"}`})

	r, err := ArchiveUISessions(&OpContext{}, req)
	if err != nil {
		t.Fatal(err)
	}
	if oc, _ := outcomeBySession(r, "idle"); oc.Action != "would-archive" {
		t.Errorf("idle session sharing a live cwd must be archivable, got %s:%s", oc.Action, oc.Reason)
	}
	// "busy" is live by id AND under 7d — skipped (either reason is correct, but
	// it must never be archived).
	if oc, _ := outcomeBySession(r, "busy"); oc.Action != "skip" {
		t.Errorf("genuinely live session must be skipped, got %s:%s", oc.Action, oc.Reason)
	}
}

func TestArchiveUI_ApplyFlipsBacksUpAndIsIdempotent(t *testing.T) {
	files := map[string]string{
		"local_old.json": uiSession("old", nowMs-30*dayMs, "false", "/w/old"),
		"local_bad.json": `{"sessionId":"bad","title":"no isArchived"}`,
	}
	req := uiFixture(t, files, nil)
	req.Apply = true
	req.Backup = true

	r, err := ArchiveUISessions(&OpContext{}, req)
	if err != nil {
		t.Fatal(err)
	}
	if r.DryRun || !r.Success || r.Changed != 1 {
		t.Fatalf("apply: dryRun=%v success=%v changed=%d", r.DryRun, r.Success, r.Changed)
	}
	// File flipped on disk.
	got, _ := os.ReadFile(filepath.Join(r.Store, "local_old.json"))
	if want := uiSession("old", nowMs-30*dayMs, "true", "/w/old"); string(got) != want {
		t.Fatalf("not flipped byte-minimally:\n got=%s\nwant=%s", got, want)
	}
	// Backup exists and equals the pre-flip bytes.
	bk, err := os.ReadFile(filepath.Join(r.BackupDir, "local_old.json"))
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(bk) != uiSession("old", nowMs-30*dayMs, "false", "/w/old") {
		t.Fatal("backup is not the original bytes")
	}
	// Unknown-schema file refused, never rewritten.
	bad, _ := os.ReadFile(filepath.Join(r.Store, "local_bad.json"))
	if string(bad) != `{"sessionId":"bad","title":"no isArchived"}` {
		t.Fatal("unknown-schema file was modified")
	}
	if oc, _ := outcomeBySession(r, "local_bad.json"); oc.Reason != uiSkipUnknownSchema {
		t.Errorf("bad file should be skip:unknown-schema, got %s:%s", oc.Action, oc.Reason)
	}

	// Second apply is a pure no-op (idempotent).
	r2, err := ArchiveUISessions(&OpContext{}, req)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Changed != 0 {
		t.Fatalf("second apply not idempotent: changed=%d", r2.Changed)
	}
	if oc, _ := outcomeBySession(r2, "old"); oc.Reason != uiSkipAlreadyArchived {
		t.Errorf("expected already-archived skip on re-run, got %s:%s", oc.Action, oc.Reason)
	}
}

func TestArchiveUI_UnarchiveReverses(t *testing.T) {
	req := uiFixture(t, map[string]string{
		"local_arch.json":  uiSession("arch", nowMs-30*dayMs, "true", "/w/arch"),
		"local_plain.json": uiSession("plain", nowMs-30*dayMs, "false", "/w/plain"),
	}, nil)
	req.Apply = true
	req.Unarchive = true
	req.Status = "all"

	r, err := ArchiveUISessions(&OpContext{}, req)
	if err != nil {
		t.Fatal(err)
	}
	if r.Direction != "unarchive" {
		t.Fatalf("direction = %s, want unarchive", r.Direction)
	}
	if oc, _ := outcomeBySession(r, "arch"); oc.Action != "unarchived" {
		t.Errorf("arch should be unarchived, got %s:%s", oc.Action, oc.Reason)
	}
	if oc, _ := outcomeBySession(r, "plain"); oc.Reason != uiSkipAlreadyUnarchived {
		t.Errorf("plain should skip already-unarchived, got %s:%s", oc.Action, oc.Reason)
	}
	got, _ := os.ReadFile(filepath.Join(r.Store, "local_arch.json"))
	if want := uiSession("arch", nowMs-30*dayMs, "false", "/w/arch"); string(got) != want {
		t.Fatalf("unarchive not byte-minimal:\n got=%s\nwant=%s", got, want)
	}
}

func TestArchiveUI_InvalidStatusAndAmbiguousStore(t *testing.T) {
	req := uiFixture(t, map[string]string{
		"local_x.json": uiSession("x", nowMs, "false", "/w/x"),
	}, nil)
	req.Status = "bogus"
	if _, err := ArchiveUISessions(&OpContext{}, req); err == nil {
		t.Fatal("expected error for invalid --status")
	} else {
		var oe *OpError
		if !errors.As(err, &oe) || oe.Code != ErrCodeInvalidInput {
			t.Fatalf("want AGM-005 OpError, got %v", err)
		}
	}

	// Two device dirs with no selector -> discovery OpError.
	root := t.TempDir()
	for _, d := range []string{"devA/acct1", "devB/acct1"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	amb := &ArchiveUISessionsRequest{HomeDir: root, StoreRoot: root, PIDRegistryDir: filepath.Join(root, "p")}
	if _, err := ArchiveUISessions(&OpContext{}, amb); err == nil {
		t.Fatal("expected error for ambiguous store")
	} else {
		var oe *OpError
		if !errors.As(err, &oe) {
			t.Fatalf("want OpError for ambiguous store, got %v", err)
		}
	}
}

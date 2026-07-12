package claudeui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sessionFile mirrors the desktop app's compact, insertion-ordered JSON so
// tests exercise the surgical-edit path on realistic bytes.
func sessionFile(id string, lastActivity int64, archived bool) string {
	return `{"sessionId":"` + id + `","cliSessionId":"cli-` + id +
		`","cwd":"/Users/x/w/` + id + `","originCwd":"/Users/x","createdAt":1700000000000,` +
		`"lastActivityAt":` + itoa(lastActivity) + `,"model":"claude-opus-4-7[1m]",` +
		`"isArchived":` + boolStr(archived) + `,"title":"work ` + id +
		`","permissionMode":"auto","completedTurns":3,"enabledMcpTools":{"a":1}}`
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// newStore builds <root>/<device>/<account>/ and writes the given files.
func newStore(t *testing.T, files map[string]string) (root, dir string) {
	t.Helper()
	root = t.TempDir()
	dir = filepath.Join(root, "dev1", "acct1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, dir
}

func TestStoreDir_AutodetectSingle(t *testing.T) {
	root, want := newStore(t, nil)
	got, dev, acct, err := StoreDir(root, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want || dev != "dev1" || acct != "acct1" {
		t.Fatalf("got (%s,%s,%s), want (%s,dev1,acct1)", got, dev, acct, want)
	}
}

func TestStoreDir_AmbiguousRefused(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"devA/acct1", "devB/acct1"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_, _, _, err := StoreDir(root, "", "")
	if !errors.Is(err, ErrAmbiguousStore) {
		t.Fatalf("want ErrAmbiguousStore, got %v", err)
	}
	// Explicit selector resolves it.
	if _, dev, _, err := StoreDir(root, "devB", "acct1"); err != nil || dev != "devB" {
		t.Fatalf("explicit device failed: dev=%s err=%v", dev, err)
	}
}

func TestStoreDir_NotFound(t *testing.T) {
	_, _, _, err := StoreDir(filepath.Join(t.TempDir(), "nope"), "", "")
	if !errors.Is(err, ErrStoreNotFound) {
		t.Fatalf("want ErrStoreNotFound, got %v", err)
	}
}

func TestListSessions_SkipsBadSchemaAsLoadError(t *testing.T) {
	_, dir := newStore(t, map[string]string{
		"local_ok.json":       sessionFile("ok", 1700000000000, false),
		"local_nofield.json":  `{"sessionId":"x","title":"missing isArchived/lastActivityAt"}`,
		"local_notjson.json":  `not json at all`,
		"local_twoflags.json": `{"sessionId":"y","lastActivityAt":1,"isArchived":false,"isArchived":true}`,
		"ignore_me.json":      sessionFile("ignored", 1, false), // no local_ prefix
		"local_dir":           "",                               // file, not dir; wrong suffix -> ignored
	})

	sessions, loadErrs, err := ListSessions(dir, "dev1", "acct1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "ok" {
		t.Fatalf("expected 1 good session 'ok', got %+v", sessions)
	}
	if len(loadErrs) != 3 {
		t.Fatalf("expected 3 load errors (nofield, notjson, twoflags), got %d: %v", len(loadErrs), loadErrs)
	}
	for _, le := range loadErrs {
		if !errors.Is(le.Err, ErrUnknownSchema) {
			t.Errorf("load error should wrap ErrUnknownSchema: %v", le.Err)
		}
	}
}

func TestFindByCLISessionID_MatchesExactIDAcrossStores(t *testing.T) {
	root, first := newStore(t, map[string]string{
		"local_target.json": sessionFile("target", 1700000000000, false),
		"local_other.json":  sessionFile("other", 1700000000000, false),
	})
	second := filepath.Join(root, "dev2", "acct2")
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "local_target-copy.json"), []byte(sessionFile("target", 1700000000000, true)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "local_bad.json"), []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	matches, loadErrs, err := FindByCLISessionID(root, "cli-target")
	if err != nil {
		t.Fatalf("FindByCLISessionID() error = %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %#v, want both exact-ID records", matches)
	}
	if matches[0].Path != filepath.Join(first, "local_target.json") || matches[1].Path != filepath.Join(second, "local_target-copy.json") {
		t.Fatalf("matches were not deterministic exact-ID results: %#v", matches)
	}
	if len(loadErrs) != 1 || !errors.Is(loadErrs[0].Err, ErrUnknownSchema) {
		t.Fatalf("load errors = %#v, want one unknown-schema record", loadErrs)
	}
}

func TestFindByCLISessionID_DoesNotMatchByWorkingDirectory(t *testing.T) {
	sameCWD := strings.Replace(sessionFile("other", 1700000000000, false), "/other", "/target", 1)
	root, _ := newStore(t, map[string]string{
		"local_same-cwd.json": sameCWD,
	})

	matches, _, err := FindByCLISessionID(root, "cli-target")
	if err != nil {
		t.Fatalf("FindByCLISessionID() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %#v, want no CWD-based match", matches)
	}
}

func TestFindByCLISessionID_SkipsUnreadableStore(t *testing.T) {
	root, goodDir := newStore(t, map[string]string{
		"local_target.json": sessionFile("target", 1700000000000, false),
	})
	blockedDir := filepath.Join(root, "dev2", "acct2")
	if err := os.MkdirAll(blockedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blockedDir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(blockedDir, 0o755); err != nil {
			t.Errorf("restore permissions for %s: %v", blockedDir, err)
		}
	})
	if _, err := os.ReadDir(blockedDir); err == nil {
		t.Skip("test user can read mode-000 directory")
	}

	matches, loadErrs, err := FindByCLISessionID(root, "cli-target")
	if err != nil {
		t.Fatalf("FindByCLISessionID() error = %v", err)
	}
	if len(matches) != 1 || matches[0].Path != filepath.Join(goodDir, "local_target.json") {
		t.Fatalf("matches = %#v, want the readable exact-ID record", matches)
	}
	if len(loadErrs) != 1 || loadErrs[0].Path != blockedDir {
		t.Fatalf("load errors = %#v, want unreadable store %s", loadErrs, blockedDir)
	}
}

func TestFindByCLISessionID_SkipsUnreadableDeviceStore(t *testing.T) {
	root, goodDir := newStore(t, map[string]string{
		"local_target.json": sessionFile("target", 1700000000000, false),
	})
	blockedDir := filepath.Join(root, "dev2")
	if err := os.MkdirAll(filepath.Join(blockedDir, "acct2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blockedDir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(blockedDir, 0o755); err != nil {
			t.Errorf("restore permissions for %s: %v", blockedDir, err)
		}
	})
	if _, err := os.ReadDir(blockedDir); err == nil {
		t.Skip("test user can read mode-000 directory")
	}

	matches, loadErrs, err := FindByCLISessionID(root, "cli-target")
	if err != nil {
		t.Fatalf("FindByCLISessionID() error = %v", err)
	}
	if len(matches) != 1 || matches[0].Path != filepath.Join(goodDir, "local_target.json") {
		t.Fatalf("matches = %#v, want the readable exact-ID record", matches)
	}
	if len(loadErrs) != 1 || loadErrs[0].Path != blockedDir {
		t.Fatalf("load errors = %#v, want unreadable device store %s", loadErrs, blockedDir)
	}
}

func TestSetArchived_IdempotentNoOp(t *testing.T) {
	_, dir := newStore(t, map[string]string{
		"local_a.json": sessionFile("a", 1700000000000, true),
	})
	s, err := LoadSession(filepath.Join(dir, "local_a.json"))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.Path)
	changed, bp, err := s.SetArchived(true, true, filepath.Join(t.TempDir(), "bk"))
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected no-op when already at target state")
	}
	if bp != "" {
		t.Fatal("no-op must not take a backup")
	}
	after, _ := os.ReadFile(s.Path)
	if string(before) != string(after) {
		t.Fatal("no-op must not modify the file")
	}
}

func TestSetArchived_SurgicalFlipPreservesEverythingElse(t *testing.T) {
	_, dir := newStore(t, map[string]string{
		"local_b.json": sessionFile("b", 1700000000000, false),
	})
	path := filepath.Join(dir, "local_b.json")
	orig, _ := os.ReadFile(path)

	s, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	bkDir := filepath.Join(t.TempDir(), "bk")
	changed, bp, err := s.SetArchived(true, true, bkDir)
	if err != nil || !changed {
		t.Fatalf("expected change; changed=%v err=%v", changed, err)
	}

	// Backup is byte-identical to the original.
	bk, _ := os.ReadFile(bp)
	if string(bk) != string(orig) {
		t.Fatal("backup is not byte-identical to original")
	}

	// On-disk file differs from original by exactly the boolean token.
	got, _ := os.ReadFile(path)
	wantAfter := strings.Replace(string(orig), `"isArchived":false`, `"isArchived":true`, 1)
	if string(got) != wantAfter {
		t.Fatalf("flip was not byte-minimal:\n got=%s\nwant=%s", got, wantAfter)
	}

	// Round-trip: unarchive must restore the exact original bytes.
	changed, _, err = s.SetArchived(false, false, "")
	if err != nil || !changed {
		t.Fatalf("expected reverse change; changed=%v err=%v", changed, err)
	}
	roundTrip, _ := os.ReadFile(path)
	if string(roundTrip) != string(orig) {
		t.Fatalf("round-trip not byte-equivalent:\n got=%s\nwant=%s", roundTrip, orig)
	}
}

func TestSetArchived_PreservesFileMode(t *testing.T) {
	_, dir := newStore(t, map[string]string{
		"local_c.json": sessionFile("c", 1700000000000, false),
	})
	path := filepath.Join(dir, "local_c.json")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SetArchived(true, false, ""); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode not preserved: got %o want 600", perm)
	}
}

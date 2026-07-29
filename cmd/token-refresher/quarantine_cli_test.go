package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// lostResponseServer accepts the request (so it is definitively transmitted)
// then closes the connection without replying — the shape of the 2026-07-18
// failure that killed the token family.
func lostResponseServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("test server does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
	}))
}

func lastAuditRecord(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &rec); err != nil {
		t.Fatalf("parse audit line: %v", err)
	}
	return rec
}

func TestRun_LostResponseQuarantinesAndExits4(t *testing.T) {
	srv := lostResponseServer(t)
	defer srv.Close()
	creds := writeCreds(t, "old", staleMs(), "old-rt")
	audit := filepath.Join(t.TempDir(), "audit.jsonl")
	quar := tmpQuarantine(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-credentials", creds, "-endpoint", srv.URL,
		"-audit-log", audit, "-quarantine", quar,
	}, &stdout, &stderr)

	if code != exitQuarantined {
		t.Errorf("exit code = %d, want %d (quarantined)", code, exitQuarantined)
	}
	if _, err := os.Stat(quar); err != nil {
		t.Errorf("expected a quarantine marker: %v", err)
	}
	stopped, err := (auth.OAuthResolver{CredentialsPath: creds}).RefreshStopped()
	if err != nil {
		t.Fatalf("inspect credential-scoped refresh stop: %v", err)
	}
	if !stopped {
		t.Error("ambiguous refresh must stop every resolver entrypoint for these credentials")
	}
	if rec := lastAuditRecord(t, audit); rec["outcome"] != "refresh_outcome_unknown" {
		t.Errorf("audit outcome = %v, want refresh_outcome_unknown", rec["outcome"])
	}
	if !strings.Contains(stderr.String(), "UNKNOWN") {
		t.Errorf("stderr should explain the unknown outcome, got: %s", stderr.String())
	}
}

// The second tick is the one that used to kill the family. It must decline to
// present the token and must not reach the network at all.
func TestRun_SecondTickRefusesToReplayQuarantinedToken(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	creds := writeCreds(t, "old", staleMs(), "old-rt")
	audit := filepath.Join(t.TempDir(), "audit.jsonl")
	quar := tmpQuarantine(t)

	// Simulate the aftermath of an ambiguous refresh of the on-disk token.
	marker := `{"refresh_token_fp":"` + fingerprintOf(t, creds) + `","quarantined_at":"2026-07-18T08:58:37Z","reason":"response lost"}`
	if err := os.WriteFile(quar, []byte(marker), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-force", "-credentials", creds, "-endpoint", srv.URL,
		"-audit-log", audit, "-quarantine", quar,
	}, &stdout, &stderr)

	if code != exitQuarantined {
		t.Errorf("exit code = %d, want %d (quarantined)", code, exitQuarantined)
	}
	if hits != 0 {
		t.Errorf("token endpoint hit %d times; the quarantined token must never be replayed", hits)
	}
	if rec := lastAuditRecord(t, audit); rec["outcome"] != "refresh_quarantined" {
		t.Errorf("audit outcome = %v, want refresh_quarantined", rec["outcome"])
	}
}

// fingerprintOf returns the fingerprint of the refresh token in a credentials
// file, so a test can quarantine exactly that token.
func fingerprintOf(t *testing.T, credPath string) string {
	t.Helper()
	fp, _ := credentialsFingerprint(credPath)
	if fp == "" {
		t.Fatal("could not fingerprint test credentials")
	}
	return fp
}

func TestRun_ClearQuarantineOverride(t *testing.T) {
	quar := tmpQuarantine(t)
	if err := os.WriteFile(quar, []byte(`{"refresh_token_fp":"abc123def456"}`), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	audit := filepath.Join(t.TempDir(), "audit.jsonl")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-clear-quarantine", "-quarantine", quar, "-audit-log", audit}, &stdout, &stderr)

	if code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
	if _, err := os.Stat(quar); !os.IsNotExist(err) {
		t.Error("marker should be gone after -clear-quarantine")
	}
}

func TestRun_ClearQuarantineKeepsMarkerWhenAlertCannotRearm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".local"), []byte("blocks state directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	quar := tmpQuarantine(t)
	if err := os.WriteFile(quar, []byte(`{"refresh_token_fp":"abc123def456"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-clear-quarantine", "-quarantine", quar}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit code = %d, want %d", code, exitError)
	}
	if _, err := os.Stat(quar); err != nil {
		t.Fatalf("quarantine marker was removed before alert re-arm succeeded: %v", err)
	}
}

func TestDefaultQuarantinePathForCredentials(t *testing.T) {
	credentials := filepath.Join(t.TempDir(), "credentials.json")
	if got, want := defaultQuarantinePathForCredentials(credentials), credentials+".refresh-quarantine.json"; got != want {
		t.Errorf("quarantine path = %q, want %q", got, want)
	}
}

func TestDefaultQuarantinePathForExplicitDefaultCredentials(t *testing.T) {
	if got, want := defaultQuarantinePathForCredentials(defaultCredentialsPath()), defaultQuarantinePath(); got != want {
		t.Errorf("explicit default credential marker = %q, want %q", got, want)
	}
}

func TestDefaultQuarantinePathForCredentialSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "credentials-link.json")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	if got, want := defaultQuarantinePathForCredentials(alias), defaultQuarantinePathForCredentials(target); got != want {
		t.Errorf("symlink quarantine path = %q, want %q", got, want)
	}
}

func TestCanonicalCredentialsPathResolvesImplicitDefaultSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, defaultCredentialsPath()); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := canonicalCredentialsPath(""); got != filepath.Clean(want) {
		t.Errorf("implicit default canonical path = %q, want %q", got, want)
	}
}

func TestRun_UnknownOutcomeWithDisabledQuarantineExplainsRetryRisk(t *testing.T) {
	srv := lostResponseServer(t)
	defer srv.Close()
	creds := writeCreds(t, "old", staleMs(), "old-rt")
	audit := filepath.Join(t.TempDir(), "audit.jsonl")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-credentials", creds, "-endpoint", srv.URL,
		"-audit-log", audit, "-quarantine", "",
	}, &stdout, &stderr)

	if code != exitNotPersisted {
		t.Errorf("exit code = %d, want %d", code, exitNotPersisted)
	}
	if !strings.Contains(stderr.String(), "Quarantine is DISABLED") {
		t.Errorf("stderr should explain retry risk, got: %s", stderr.String())
	}
	if rec := lastAuditRecord(t, audit); rec["outcome"] != "quarantine_not_persisted" {
		t.Errorf("audit outcome = %v, want quarantine_not_persisted", rec["outcome"])
	}
	if _, err := os.Stat(creds + ".refresh-stop"); err != nil {
		t.Fatalf("disabled quarantine must fall back to the credential-scoped stop: %v", err)
	}
}

func TestRun_CadenceWithDisabledQuarantineConfirmsDurableStop(t *testing.T) {
	srv := lostResponseServer(t)
	defer srv.Close()
	creds := writeCreds(t, "old", staleMs(), "old-rt")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-cadence", "-credentials", creds, "-endpoint", srv.URL,
		"-audit-log", filepath.Join(t.TempDir(), "audit.jsonl"), "-quarantine", "",
	}, &stdout, &stderr)

	if code != exitOK {
		t.Errorf("exit code = %d, want %d after durable stop", code, exitOK)
	}
	if !strings.Contains(stderr.String(), "cadence refresh STOPPED") {
		t.Errorf("stderr should confirm the durable stop, got: %s", stderr.String())
	}
	if _, err := os.Stat(creds + ".refresh-stop"); err != nil {
		t.Fatalf("refresh stop missing: %v", err)
	}
}

func TestRun_CadenceWithCustomQuarantineAndFailedSharedStopEscalates(t *testing.T) {
	creds := writeCreds(t, "old", staleMs(), "old-rt")
	quarantine := filepath.Join(t.TempDir(), "custom-quarantine.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The preflight stop inspection has completed. Make only the shared
		// stop write fail while leaving the custom quarantine writable.
		if err := os.Mkdir(creds+".refresh-stop", 0o700); err != nil {
			t.Errorf("make refresh-stop blocker: %v", err)
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("test server does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-cadence", "-credentials", creds, "-endpoint", srv.URL,
		"-audit-log", filepath.Join(t.TempDir(), "audit.jsonl"), "-quarantine", quarantine,
	}, &stdout, &stderr)

	if code != exitNotPersisted {
		t.Errorf("exit code = %d, want %d when no shared protection is confirmed", code, exitNotPersisted)
	}
	if _, err := os.Stat(quarantine); err != nil {
		t.Fatalf("custom quarantine should remain as caller-local evidence: %v", err)
	}
	if !strings.Contains(stderr.String(), "stop was NOT persisted") {
		t.Errorf("stderr should refuse to report a safe stop, got: %s", stderr.String())
	}
}

// -check must surface an active quarantine; it is how an operator finds out why
// refreshes stopped.
func TestRun_CheckReportsQuarantine(t *testing.T) {
	creds := writeCreds(t, "tok", freshMs(), "rt")
	quar := tmpQuarantine(t)
	// The marker must name the token actually on disk, or it is not holding
	// anything back.
	fp := fingerprintOf(t, creds)
	marker := `{"refresh_token_fp":"` + fp + `","quarantined_at":"2026-07-18T08:58:37Z","reason":"response lost"}`
	if err := os.WriteFile(quar, []byte(marker), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-check", "-credentials", creds,
		"-audit-log", filepath.Join(t.TempDir(), "audit.jsonl"), "-quarantine", quar,
	}, &stdout, &stderr)

	if code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
	out := stderr.String()
	if !strings.Contains(out, "QUARANTINED") || !strings.Contains(out, fp) {
		t.Errorf("check should report the quarantine, got: %s", out)
	}
}

func TestRun_CheckReportsDurableRefreshStop(t *testing.T) {
	creds := writeCreds(t, "tok", freshMs(), "rt")
	if err := os.WriteFile(creds+".refresh-stop", []byte("persistence failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-check", "-credentials", creds,
		"-audit-log", filepath.Join(t.TempDir(), "audit.jsonl"), "-quarantine", tmpQuarantine(t),
	}, &stdout, &stderr)

	if code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stderr.String(), "REFRESH STOPPED") {
		t.Errorf("check should report the durable refresh stop, got: %s", stderr.String())
	}
}

func TestRun_CheckLeavesStaleRefreshStopMarkerInReadOnlyDirectory(t *testing.T) {
	creds := writeCreds(t, "tok", freshMs(), "rt-current")
	stopPath := creds + ".refresh-stop"
	marker := `{"refresh_token_fp":"` + auth.RefreshTokenFingerprint("rt-previous") +
		`","reason":"previous refresh outcome unknown"}` + "\n"
	if err := os.WriteFile(stopPath, []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(creds)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-check", "-credentials", creds,
		"-audit-log", filepath.Join(t.TempDir(), "audit.jsonl"), "-quarantine", tmpQuarantine(t),
	}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit code = %d, want %d; stderr: %s", code, exitOK, stderr.String())
	}
	if strings.Contains(stderr.String(), "REFRESH STOPPED") {
		t.Errorf("rotated marker must be reported as inert, got: %s", stderr.String())
	}
	if _, err := os.Stat(stopPath); err != nil {
		t.Fatalf("check mode mutated stale refresh stop marker: %v", err)
	}
}

// A marker for a token that has since rotated holds nothing back. Reporting it
// as active would send the operator to -clear-quarantine for no reason.
func TestRun_CheckReportsStaleMarkerAsInert(t *testing.T) {
	creds := writeCreds(t, "tok", freshMs(), "rt-current")
	quar := tmpQuarantine(t)
	if err := os.WriteFile(quar, []byte(`{"refresh_token_fp":"deadbeef1234","quarantined_at":"2026-07-18T08:58:37Z","reason":"response lost"}`), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-check", "-credentials", creds,
		"-audit-log", filepath.Join(t.TempDir(), "audit.jsonl"), "-quarantine", quar,
	}, &stdout, &stderr)

	if code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
	out := stderr.String()
	if strings.Contains(out, "QUARANTINED") {
		t.Errorf("a stale marker must not be reported as holding refreshes back, got: %s", out)
	}
	if !strings.Contains(out, "stale quarantine marker") {
		t.Errorf("check should still mention the inert marker, got: %s", out)
	}
	if _, err := os.Stat(quar); err != nil {
		t.Error("check mode must not delete the marker")
	}
}

// An unreadable marker must block the refresh rather than be ignored.
func TestRun_UnreadableMarkerBlocksRefresh(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
	}))
	defer srv.Close()

	creds := writeCreds(t, "old", staleMs(), "old-rt")
	audit := filepath.Join(t.TempDir(), "audit.jsonl")
	quar := tmpQuarantine(t)
	if err := os.WriteFile(quar, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-force", "-credentials", creds, "-endpoint", srv.URL,
		"-audit-log", audit, "-quarantine", quar,
	}, &stdout, &stderr)

	if code != exitQuarantined {
		t.Errorf("exit code = %d, want %d", code, exitQuarantined)
	}
	if hits != 0 {
		t.Errorf("endpoint hit %d times; an unreadable marker must fail closed", hits)
	}
	if rec := lastAuditRecord(t, audit); rec["outcome"] != "quarantine_unreadable" {
		t.Errorf("audit outcome = %v, want quarantine_unreadable", rec["outcome"])
	}
}

// In cadence mode a quarantine must alert but still exit 0, or launchd throttles
// the job off the schedule — the failure that left the mesh unrefreshed for 17h
// on 2026-07-19.
func TestCadenceExit_QuarantinedKeepsScheduleAlive(t *testing.T) {
	stateDir := t.TempDir()
	var stderr bytes.Buffer

	if got := cadenceExit(exitQuarantined, stateDir, deathSentinelName, &stderr); got != exitOK {
		t.Errorf("cadence exit = %d, want %d so launchd keeps the schedule", got, exitOK)
	}
	if _, err := os.Stat(filepath.Join(stateDir, deathSentinelName)); err != nil {
		t.Error("expected a sentinel so the next tick does not re-notify")
	}
}

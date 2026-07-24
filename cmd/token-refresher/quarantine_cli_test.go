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

func TestDefaultQuarantinePathForCredentials(t *testing.T) {
	credentials := filepath.Join(t.TempDir(), "credentials.json")
	if got, want := defaultQuarantinePathForCredentials(credentials), credentials+".refresh-quarantine.json"; got != want {
		t.Errorf("quarantine path = %q, want %q", got, want)
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

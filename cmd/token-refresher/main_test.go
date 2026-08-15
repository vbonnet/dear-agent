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
	"time"
)

func writeCreds(t *testing.T, accessToken string, expiresAt int64, refreshToken string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.json")
	body := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  accessToken,
			"expiresAt":    expiresAt,
			"refreshToken": refreshToken,
		},
	}
	data, _ := json.MarshalIndent(body, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	return path
}

func okTokenServer(t *testing.T, access, refresh string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  access,
			"refresh_token": refresh,
			"expires_in":    86400,
			"token_type":    "Bearer",
		})
	}))
}

// tmpQuarantine gives each test its own quarantine marker path. Without it the
// CLI would fall back to the real ~/.local/state/dear-agent marker and a test
// run could clear an operator's live quarantine.
func tmpQuarantine(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "quarantine.json")
}

// staleMs returns an expiry well in the past.
func staleMs() int64 { return time.Now().Add(-time.Hour).UnixMilli() }

// freshMs returns an expiry well in the future.
func freshMs() int64 { return time.Now().Add(24 * time.Hour).UnixMilli() }

func TestRun_StaleTokenRefreshesAndPrints(t *testing.T) {
	srv := okTokenServer(t, "brand-new-token", "rotated-rt")
	defer srv.Close()
	creds := writeCreds(t, "old", staleMs(), "old-rt")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-credentials", creds,
		"-endpoint", srv.URL,
		"-audit-log", filepath.Join(t.TempDir(), "audit.jsonl"),
		"-quarantine", tmpQuarantine(t),
	}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "brand-new-token" {
		t.Errorf("stdout = %q, want brand-new-token", got)
	}
	// Verify the file was updated with the rotated refresh token.
	data, _ := os.ReadFile(creds)
	if !strings.Contains(string(data), "rotated-rt") {
		t.Errorf("credentials not updated with rotated refresh token: %s", data)
	}
}

func TestRun_FreshTokenSkipsNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("token endpoint should not be called for a fresh token")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	creds := writeCreds(t, "still-good", freshMs(), "rt")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-credentials", creds, "-endpoint", srv.URL, "-audit-log", "", "-quarantine", tmpQuarantine(t)}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "still-good" {
		t.Errorf("stdout = %q, want still-good", got)
	}
}

func TestRun_ForceRefreshesFreshToken(t *testing.T) {
	srv := okTokenServer(t, "forced-token", "rt2")
	defer srv.Close()
	creds := writeCreds(t, "still-good", freshMs(), "rt")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-force", "-credentials", creds, "-endpoint", srv.URL, "-audit-log", "", "-quarantine", tmpQuarantine(t)}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "forced-token" {
		t.Errorf("stdout = %q, want forced-token (force should refresh a fresh token)", got)
	}
}

func TestLaunchdCadenceDoesNotForceRefresh(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "launchd", "com.dear-agent.token-refresher.plist"))
	if err != nil {
		t.Fatalf("read launchd template: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "<string>-cadence</string>") {
		t.Fatal("launchd token-refresher must run in cadence mode")
	}
	if strings.Contains(body, "<string>-force</string>") {
		t.Fatal("launchd token-refresher must not force refresh every tick; forced rotations race native Claude Code refreshers")
	}
}

func TestRun_InvalidGrantExits2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()
	creds := writeCreds(t, "old", staleMs(), "dead-rt")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-credentials", creds, "-endpoint", srv.URL, "-audit-log", "", "-quarantine", tmpQuarantine(t)}, &stdout, &stderr)
	if code != exitTokenFamilyDead {
		t.Fatalf("exit = %d, want %d (token family dead)", code, exitTokenFamilyDead)
	}
	if !strings.Contains(stderr.String(), "invalid_grant") {
		t.Errorf("stderr missing invalid_grant guidance: %s", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty on failure, got %q", stdout.String())
	}
}

func TestRun_CheckModeReportsStatusNoNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("check mode must not call the network")
	}))
	defer srv.Close()
	creds := writeCreds(t, "tok", freshMs(), "rt")
	audit := filepath.Join(t.TempDir(), "audit.jsonl")

	var stdout, stderr bytes.Buffer
	code := run([]string{"-check", "-credentials", creds, "-endpoint", srv.URL, "-audit-log", audit, "-quarantine", tmpQuarantine(t)}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "FRESH") {
		t.Errorf("status output missing FRESH: %s", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("check mode must not print a token to stdout, got %q", stdout.String())
	}
	// Audit line written.
	if data, _ := os.ReadFile(audit); !strings.Contains(string(data), `"mode":"check"`) {
		t.Errorf("audit log missing check record: %s", data)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeCredsFile(path, accessToken string, expiresAt int64, refreshToken string) error {
	body := map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":  accessToken,
			"expiresAt":    expiresAt,
			"refreshToken": refreshToken,
		},
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func writeCreds(t *testing.T, accessToken string, expiresAt int64, refreshToken string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := writeCredsFile(path, accessToken, expiresAt, refreshToken); err != nil {
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

// TestLaunchdExpirySkewExceedsStartInterval pins the invariant that actually
// keeps the credentials file valid. Dropping -force stopped the 30-minute
// rotations, but the resolver's default skew is 60s while the job only looks
// every 1800s, so nearly every tick found the token "fresh" with minutes left
// and the next one arrived after it had already expired (observed 2026-08-15:
// expiry 09:21:24Z, ticks at 08:53:43Z and 09:23:43Z, 2m19s of 401s in
// between). The skew has to be wider than the sampling interval or the cadence
// job samples straight past the expiry it exists to prevent.
func TestLaunchdExpirySkewExceedsStartInterval(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "launchd", "com.dear-agent.token-refresher.plist"))
	if err != nil {
		t.Fatalf("read launchd template: %v", err)
	}
	body := string(data)

	skewArg := plistArgAfter(t, body, "-expiry-skew")
	skew, err := time.ParseDuration(skewArg)
	if err != nil {
		t.Fatalf("parse -expiry-skew %q: %v", skewArg, err)
	}

	var interval time.Duration
	if _, err := fmt.Sscanf(plistIntegerAfter(t, body, "StartInterval"), "%d", &interval); err != nil {
		t.Fatalf("parse StartInterval: %v", err)
	}
	interval *= time.Second

	if skew <= interval {
		t.Fatalf("-expiry-skew (%s) must exceed StartInterval (%s); otherwise a tick can find the token fresh and the next tick arrives after it expired", skew, interval)
	}
}

// plistArgAfter returns the <string> element following the given argument in
// the ProgramArguments array.
func plistArgAfter(t *testing.T, body, arg string) string {
	t.Helper()
	marker := "<string>" + arg + "</string>"
	idx := strings.Index(body, marker)
	if idx < 0 {
		t.Fatalf("launchd template must pass %s", arg)
	}
	rest := body[idx+len(marker):]
	open := strings.Index(rest, "<string>")
	closeIdx := strings.Index(rest, "</string>")
	if open < 0 || closeIdx < open {
		t.Fatalf("%s has no value in the launchd template", arg)
	}
	return rest[open+len("<string>") : closeIdx]
}

// plistIntegerAfter returns the <integer> value for the given plist key.
func plistIntegerAfter(t *testing.T, body, key string) string {
	t.Helper()
	marker := "<key>" + key + "</key>"
	idx := strings.Index(body, marker)
	if idx < 0 {
		t.Fatalf("launchd template must set %s", key)
	}
	rest := body[idx+len(marker):]
	open := strings.Index(rest, "<integer>")
	closeIdx := strings.Index(rest, "</integer>")
	if open < 0 || closeIdx < open {
		t.Fatalf("%s has no integer value in the launchd template", key)
	}
	return strings.TrimSpace(rest[open+len("<integer>") : closeIdx])
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

func TestRun_NegativeSentinelMaxAgeRejects(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-sentinel-max-age", "-1s"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "-sentinel-max-age cannot be negative") {
		t.Errorf("stderr missing validation message: %s", stderr.String())
	}
}

func TestRun_CadencePrunesStaleSentinelsViaFlag(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	staleSentinel := filepath.Join(stateDir, deathSentinelName+"-0123456789abcdef")
	if err := os.WriteFile(staleSentinel, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(staleSentinel, now.Add(-3*time.Hour), now.Add(-3*time.Hour))

	freshSentinel := filepath.Join(stateDir, deathSentinelName+"-fedcba9876543210")
	if err := os.WriteFile(freshSentinel, []byte("fresh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(freshSentinel, now.Add(-5*time.Minute), now.Add(-5*time.Minute))

	srv := okTokenServer(t, "new-access", "new-refresh")
	defer srv.Close()

	creds := writeCreds(t, "fresh-tok", freshMs(), "fresh-rt")
	audit := filepath.Join(stateDir, "audit.jsonl")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"-cadence",
		"-credentials", creds,
		"-endpoint", srv.URL,
		"-audit-log", audit,
		"-state-dir", stateDir,
		"-sentinel-max-age", "1h",
		"-quarantine", filepath.Join(stateDir, "quar.json"),
	}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if _, err := os.Stat(staleSentinel); !os.IsNotExist(err) {
		t.Errorf("stale sentinel %s was not pruned", staleSentinel)
	}
	if _, err := os.Stat(freshSentinel); err != nil {
		t.Errorf("fresh sentinel %s was unexpectedly removed: %v", freshSentinel, err)
	}
}

func TestClearRefreshProtectionsCommand_IncludesCustomStateDir(t *testing.T) {
	cmdEmpty := clearRefreshProtectionsCommand("/path/creds.json", "/path/quar.json", "")
	if strings.Contains(cmdEmpty, "-state-dir") {
		t.Errorf("empty state dir should omit -state-dir flag: %s", cmdEmpty)
	}

	cmdDefault := clearRefreshProtectionsCommand("/path/creds.json", "/path/quar.json", defaultStateDir())
	if !strings.Contains(cmdDefault, fmt.Sprintf("-state-dir %s", shellQuote(canonicalStateDir(defaultStateDir())))) {
		t.Errorf("default state dir should be included in clear command: %s", cmdDefault)
	}

	customDir := "/custom/state/dir"
	cmdCustom := clearRefreshProtectionsCommand("/path/creds.json", "/path/quar.json", customDir)
	if !strings.Contains(cmdCustom, fmt.Sprintf("-state-dir %s", shellQuote(customDir))) {
		t.Errorf("custom state dir should be included in clear command: %s", cmdCustom)
	}

	relDir := "relative/state/dir"
	expectedAbs, _ := filepath.Abs(relDir)
	cmdRel := clearRefreshProtectionsCommand("/path/creds.json", "/path/quar.json", relDir)
	if !strings.Contains(cmdRel, fmt.Sprintf("-state-dir %s", shellQuote(filepath.Clean(expectedAbs)))) {
		t.Errorf("relative state dir should be canonicalized to absolute in clear command: %s", cmdRel)
	}
}

func TestClearRefreshProtectionsCommand_ShellQuotesMetacharacters(t *testing.T) {
	creds := "/path/with space/$VAR/`id`/creds.json"
	quar := "/path/with'quote/quar.json"
	state := "/custom/state'dir/with spaces and $SIG"

	cmd := clearRefreshProtectionsCommand(creds, quar, state)
	expectedCreds := shellQuote(creds)
	expectedQuar := shellQuote(quar)
	expectedState := shellQuote(canonicalStateDir(state))

	if !strings.Contains(cmd, expectedCreds) {
		t.Errorf("credentials path not shell-quoted: %s", cmd)
	}
	if !strings.Contains(cmd, expectedQuar) {
		t.Errorf("quarantine path not shell-quoted: %s", cmd)
	}
	if !strings.Contains(cmd, expectedState) {
		t.Errorf("state dir not shell-quoted: %s", cmd)
	}
}

func TestRun_CadenceRejectsInsecureFallbackStateDir(t *testing.T) {
	insecureDir := filepath.Join(os.TempDir(), fmt.Sprintf("dear-agent-%d-insecure-main-%d", os.Getuid(), time.Now().UnixNano()))
	if err := os.Mkdir(insecureDir, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(insecureDir)
	if err := os.Chmod(insecureDir, 0o777); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"-cadence", "-state-dir", insecureDir}, &stdout, &stderr)
	if code != exitError {
		t.Errorf("expected exitError (%d), got %d; stderr: %s", exitError, code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "state directory unavailable") {
		t.Errorf("expected 'state directory unavailable' in stderr, got: %s", stderr.String())
	}
}

func TestRun_CheckAndCadenceMutuallyExclusive(t *testing.T) {
	var stdout, stderr bytes.Buffer
	nonExistentStateDir := filepath.Join(os.TempDir(), fmt.Sprintf("dear-agent-sideeffect-%d", time.Now().UnixNano()))
	code := run([]string{"-check", "-cadence", "-state-dir", nonExistentStateDir}, &stdout, &stderr)
	if code != exitError {
		t.Errorf("expected exitError (%d), got %d; stderr: %s", exitError, code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "-check and -cadence are mutually exclusive") {
		t.Errorf("expected mutual exclusion error in stderr, got: %s", stderr.String())
	}
	if _, err := os.Lstat(nonExistentStateDir); !os.IsNotExist(err) {
		t.Errorf("combined check and cadence created state directory on disk: %s", nonExistentStateDir)
	}
}

func TestRun_CadenceRejectsReadOnlyStateDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping chmod-based write denial test when running as root")
	}
	roDir := t.TempDir()
	if err := os.Chmod(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chmod(roDir, 0o700)
	}()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-cadence", "-state-dir", roDir}, &stdout, &stderr)
	if code != exitError {
		t.Errorf("expected exitError (%d), got %d; stderr: %s", exitError, code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "state directory unavailable") {
		t.Errorf("expected 'state directory unavailable' in stderr, got: %s", stderr.String())
	}
}

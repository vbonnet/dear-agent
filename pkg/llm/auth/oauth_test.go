package auth

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// writeCreds writes a credentials.json with the given access token and expiry
// (Unix milliseconds; pass 0 to omit) to a temp file and returns its path.
func writeCreds(t *testing.T, accessToken string, expiresAtMillis int64) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.json")
	body := `{"claudeAiOauth":{"accessToken":"` + accessToken + `","expiresAt":` +
		strconv.FormatInt(expiresAtMillis, 10) + `,"refreshToken":"rt","scopes":["user:inference"]}}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	return path
}

// fixedClock returns a Now func pinned to a reference instant.
func fixedClock() func() time.Time {
	ref := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return ref }
}

func envGetter(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolve_FileTokenPreferredOverEnv(t *testing.T) {
	// File token is fresh (expires an hour after the fixed clock).
	expires := time.Date(2026, 6, 16, 13, 0, 0, 0, time.UTC).UnixMilli()
	r := OAuthResolver{
		CredentialsPath: writeCreds(t, "file-token", expires),
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
	}
	if got := r.Resolve(); got != "file-token" {
		t.Errorf("Resolve() = %q, want file-token (fresh file token must win over env)", got)
	}
}

func TestResolve_FallbackToEnvWhenFileMissing(t *testing.T) {
	r := OAuthResolver{
		CredentialsPath: filepath.Join(t.TempDir(), "does-not-exist.json"),
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
	}
	if got := r.Resolve(); got != "env-token" {
		t.Errorf("Resolve() = %q, want env-token (missing file must fall back to env)", got)
	}
}

func TestResolve_FallbackToEnvWhenFileMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte("not json{"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := OAuthResolver{
		CredentialsPath: path,
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
	}
	if got := r.Resolve(); got != "env-token" {
		t.Errorf("Resolve() = %q, want env-token (malformed file must fall back to env)", got)
	}
}

func TestResolve_FallbackToEnvWhenFileTokenEmpty(t *testing.T) {
	r := OAuthResolver{
		CredentialsPath: writeCreds(t, "", 0),
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
	}
	if got := r.Resolve(); got != "env-token" {
		t.Errorf("Resolve() = %q, want env-token (empty file token must fall back to env)", got)
	}
}

func TestResolve_ExpiredFileTokenFallsBackToEnv(t *testing.T) {
	// File token expired an hour before the fixed clock.
	expires := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	r := OAuthResolver{
		CredentialsPath: writeCreds(t, "stale-file-token", expires),
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
	}
	if got := r.Resolve(); got != "env-token" {
		t.Errorf("Resolve() = %q, want env-token (expired file token must defer to env)", got)
	}
}

func TestResolve_ExpiringWithinSkewFallsBackToEnv(t *testing.T) {
	// File token expires 30s after the clock, inside the default 60s skew.
	expires := time.Date(2026, 6, 16, 12, 0, 30, 0, time.UTC).UnixMilli()
	r := OAuthResolver{
		CredentialsPath: writeCreds(t, "almost-stale", expires),
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
	}
	if got := r.Resolve(); got != "env-token" {
		t.Errorf("Resolve() = %q, want env-token (token inside skew window is treated as stale)", got)
	}
}

func TestResolve_ExpiredFileTokenUsedWhenEnvEmpty(t *testing.T) {
	// No env token at all: a stale file token still beats returning nothing.
	expires := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	r := OAuthResolver{
		CredentialsPath: writeCreds(t, "stale-file-token", expires),
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
	}
	if got := r.Resolve(); got != "stale-file-token" {
		t.Errorf("Resolve() = %q, want stale-file-token (only source must be returned)", got)
	}
}

func TestResolve_NoExpiryTreatedAsFresh(t *testing.T) {
	r := OAuthResolver{
		CredentialsPath: writeCreds(t, "file-token", 0),
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
	}
	if got := r.Resolve(); got != "file-token" {
		t.Errorf("Resolve() = %q, want file-token (absent expiry assumes usable)", got)
	}
}

func TestResolve_EmptyWhenNoSource(t *testing.T) {
	r := OAuthResolver{
		CredentialsPath: filepath.Join(t.TempDir(), "missing.json"),
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
	}
	if got := r.Resolve(); got != "" {
		t.Errorf("Resolve() = %q, want empty string (no source available)", got)
	}
}

func TestResolve_RealExpiresAtFormatParses(t *testing.T) {
	// Guards the wire format: a real ~/.claude/.credentials.json carries
	// expiresAt as Unix epoch milliseconds (e.g. 1781647414805).
	r := OAuthResolver{
		CredentialsPath: writeCreds(t, "file-token", 1781647414805),
		Getenv:          envGetter(map[string]string{}),
		Now:             func() time.Time { return time.UnixMilli(1781647414805 - 3600_000) },
	}
	if got := r.Resolve(); got != "file-token" {
		t.Errorf("Resolve() = %q, want file-token (epoch-millis expiry must parse and validate)", got)
	}
}

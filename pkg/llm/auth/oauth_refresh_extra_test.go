package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// staleMillis is an expiry an hour before the fixed test clock (2026-06-16 12:00).
func staleMillis() int64 {
	return time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
}

// TestRefresh_InvalidGrantIsTokenFamilyDead verifies that a 400 invalid_grant
// from the token endpoint maps to the typed ErrTokenFamilyDead so callers can
// escalate to re-auth instead of retrying a dead family.
func TestRefresh_InvalidGrantIsTokenFamilyDead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token revoked"}`))
	}))
	defer srv.Close()

	credsPath := writeFullCreds(t, "stale", staleMillis(), "dead-rt")
	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	_, err := r.Refresh(context.Background())
	if !errors.Is(err, ErrTokenFamilyDead) {
		t.Fatalf("Refresh() error = %v, want ErrTokenFamilyDead", err)
	}
}

// TestRefresh_BacksUpBeforeMutating verifies a .bak holding the pre-refresh
// credentials is written before the file is overwritten.
func TestRefresh_BacksUpBeforeMutating(t *testing.T) {
	srv := newTokenServer(t, "fresh-token", "new-rt", 86400)
	defer srv.Close()

	credsPath := writeFullCreds(t, "old-token", staleMillis(), "old-rt")
	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	if _, err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	bak := credsPath + credentialsBackupSuffix
	data, err := os.ReadFile(bak)
	if err != nil {
		t.Fatalf("read backup %s: %v", bak, err)
	}
	var creds fullCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		t.Fatalf("unmarshal backup: %v", err)
	}
	if creds.ClaudeAIOAuth.AccessToken != "old-token" {
		t.Errorf("backup access token = %q, want old-token (pre-refresh state)", creds.ClaudeAIOAuth.AccessToken)
	}
	if creds.ClaudeAIOAuth.RefreshToken != "old-rt" {
		t.Errorf("backup refresh token = %q, want old-rt (pre-refresh state)", creds.ClaudeAIOAuth.RefreshToken)
	}
}

// TestRefresh_SendsDefaultClientIDAndWAFHeaders verifies the request carries the
// corrected default client_id and the WAF-friendly headers.
func TestRefresh_SendsDefaultClientIDAndWAFHeaders(t *testing.T) {
	var gotClientID, gotUA, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotClientID = r.FormValue("client_id")
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "t", ExpiresIn: 86400, RefreshToken: "rt", TokenType: "Bearer"})
	}))
	defer srv.Close()

	credsPath := writeFullCreds(t, "stale", staleMillis(), "rt")
	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}
	if _, err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if gotClientID != defaultClientID {
		t.Errorf("client_id = %q, want default %q", gotClientID, defaultClientID)
	}
	if gotUA == "" || gotUA == "Go-http-client/1.1" {
		t.Errorf("User-Agent = %q, want a WAF-friendly non-default UA", gotUA)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
}

// TestRefresh_ClientIDAndEndpointEnvOverrides verifies env overrides win when
// the struct fields are empty.
func TestRefresh_ClientIDEnvOverride(t *testing.T) {
	var gotClientID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotClientID = r.FormValue("client_id")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "t", ExpiresIn: 86400, RefreshToken: "rt"})
	}))
	defer srv.Close()

	credsPath := writeFullCreds(t, "stale", staleMillis(), "rt")
	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv: envGetter(map[string]string{
			envClientID:      "custom-client",
			envTokenEndpoint: srv.URL, // exercised: empty TokenEndpoint field falls through to env
		}),
		Now:        fixedClock(),
		HTTPClient: srv.Client(),
		// TokenEndpoint intentionally empty so the env override is used.
	}
	if _, err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if gotClientID != "custom-client" {
		t.Errorf("client_id = %q, want custom-client (env override)", gotClientID)
	}
}

// TestRefresh_ConcurrentCallsExchangeOnce proves the cross-process file lock
// serializes refreshes: many concurrent Refresh calls hit the token endpoint
// exactly once because losers re-read the now-fresh token under the lock. This
// is the rotation-race guard — without it, each caller would burn the
// single-use refresh token.
func TestRefresh_ConcurrentCallsExchangeOnce(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		time.Sleep(40 * time.Millisecond) // widen the race window
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "fresh", ExpiresIn: 86400, RefreshToken: "new-rt"})
	}))
	defer srv.Close()

	credsPath := writeFullCreds(t, "stale", staleMillis(), "rt")
	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	tokens := make([]string, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tokens[i], errs[i] = r.Refresh(context.Background())
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Refresh #%d error = %v", i, err)
		}
		if tokens[i] != "fresh" {
			t.Errorf("Refresh #%d token = %q, want fresh", i, tokens[i])
		}
	}
	if got := callCount.Load(); got != 1 {
		t.Errorf("token endpoint called %d times, want exactly 1 (lock must serialize + dedup)", got)
	}
}

// TestRefresh_PersistFailureSurfaces verifies that when the rotated token cannot
// be written to disk, Refresh returns ErrRefreshNotPersisted instead of silently
// reporting success — the bug that poisoned the token family.
func TestRefresh_PersistFailureSurfaces(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test write-permission failure as root")
	}
	srv := newTokenServer(t, "fresh-token", "new-rt", 86400)
	defer srv.Close()

	dir := t.TempDir()
	credsPath := filepath.Join(dir, "credentials.json")
	mustWriteCreds(t, credsPath, "old-token", staleMillis(), "old-rt")
	// Pre-create the lock file so opening it does not need dir-write permission.
	if err := os.WriteFile(filepath.Join(dir, credentialsLockName), nil, 0o600); err != nil {
		t.Fatalf("seed lock file: %v", err)
	}
	// Make the dir read-only so creating the temp file for the atomic write
	// fails, while reads of the existing files still succeed.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
		QuarantinePath:  filepath.Join(t.TempDir(), "quarantine.json"),
	}

	_, err := r.Refresh(context.Background())
	if !errors.Is(err, ErrRefreshNotPersisted) {
		t.Fatalf("Refresh() error = %v, want ErrRefreshNotPersisted", err)
	}
	if _, statErr := os.Stat(r.QuarantinePath); statErr != nil {
		t.Fatalf("rotated on-disk refresh token was not quarantined before returning: %v", statErr)
	}
}

// TestRefresh_RequiresHTTPClient verifies the guard against a nil client.
func TestRefresh_RequiresHTTPClient(t *testing.T) {
	credsPath := writeFullCreds(t, "stale", staleMillis(), "rt")
	r := OAuthResolver{CredentialsPath: credsPath, Now: fixedClock()}
	if _, err := r.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh() with nil HTTPClient = nil error, want error")
	}
}

// mustWriteCreds writes a credentials file at an explicit path (writeFullCreds
// chooses its own tempdir path).
func mustWriteCreds(t *testing.T, path, accessToken string, expiresAtMillis int64, refreshToken string) {
	t.Helper()
	creds := fullCredentials{ClaudeAIOAuth: fullOAuthBlock{
		AccessToken:  accessToken,
		ExpiresAt:    expiresAtMillis,
		RefreshToken: refreshToken,
		Scopes:       []string{"user:inference"},
	}}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		t.Fatalf("marshal creds: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
}

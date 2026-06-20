package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeFullCreds writes a credentials.json with access token, expiry, and refresh token.
func writeFullCreds(t *testing.T, accessToken string, expiresAtMillis int64, refreshToken string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.json")
	creds := fullCredentials{
		ClaudeAIOAuth: fullOAuthBlock{
			AccessToken:  accessToken,
			ExpiresAt:    expiresAtMillis,
			RefreshToken: refreshToken,
			Scopes:       []string{"user:inference"},
		},
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		t.Fatalf("marshal creds: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	return path
}

func newTokenServer(t *testing.T, accessToken, refreshToken string, expiresIn int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if r.FormValue("grant_type") != "refresh_token" {
			http.Error(w, "unsupported grant_type", http.StatusBadRequest)
			return
		}
		if r.FormValue("refresh_token") == "" {
			http.Error(w, "missing refresh_token", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  accessToken,
			ExpiresIn:    expiresIn,
			RefreshToken: refreshToken,
			TokenType:    "Bearer",
		})
	}))
}

func TestRefresh_SuccessfulRefresh(t *testing.T) {
	srv := newTokenServer(t, "fresh-token", "new-rt", 86400)
	defer srv.Close()

	staleExpiry := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "stale-token", staleExpiry, "old-rt")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	got := r.resolveWithRefresh()
	if got != "fresh-token" {
		t.Errorf("resolveWithRefresh() = %q, want fresh-token", got)
	}

	// Verify the credentials file was updated.
	data, err := os.ReadFile(credsPath)
	if err != nil {
		t.Fatalf("read updated creds: %v", err)
	}
	var creds fullCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		t.Fatalf("unmarshal updated creds: %v", err)
	}
	if creds.ClaudeAIOAuth.AccessToken != "fresh-token" {
		t.Errorf("persisted access token = %q, want fresh-token", creds.ClaudeAIOAuth.AccessToken)
	}
	if creds.ClaudeAIOAuth.RefreshToken != "new-rt" {
		t.Errorf("persisted refresh token = %q, want new-rt (rotation)", creds.ClaudeAIOAuth.RefreshToken)
	}
}

func TestRefresh_FallbackOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	staleExpiry := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "stale-token", staleExpiry, "rt")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	got := r.resolveWithRefresh()
	if got != "env-token" {
		t.Errorf("resolveWithRefresh() = %q, want env-token (HTTP error should fall back)", got)
	}
}

func TestRefresh_FallbackWhenNoRefreshToken(t *testing.T) {
	staleExpiry := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "stale-token", staleExpiry, "")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
		HTTPClient:      &http.Client{},
	}

	got := r.resolveWithRefresh()
	if got != "env-token" {
		t.Errorf("resolveWithRefresh() = %q, want env-token (no refresh token should fall back)", got)
	}
}

func TestRefresh_SkippedWhenNoHTTPClient(t *testing.T) {
	staleExpiry := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "stale-token", staleExpiry, "rt")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "env-token"}),
		Now:             fixedClock(),
	}

	got := r.resolveWithRefresh()
	if got != "env-token" {
		t.Errorf("resolveWithRefresh() = %q, want env-token (nil HTTPClient should skip refresh)", got)
	}
}

func TestRefresh_FreshTokenSkipsRefresh(t *testing.T) {
	var refreshCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalled.Store(true)
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer srv.Close()

	freshExpiry := time.Date(2026, 6, 16, 13, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "fresh-token", freshExpiry, "rt")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	got := r.resolveWithRefresh()
	if got != "fresh-token" {
		t.Errorf("resolveWithRefresh() = %q, want fresh-token", got)
	}
	if refreshCalled.Load() {
		t.Error("refresh endpoint was called for a fresh token")
	}
}

func TestRefresh_ConcurrentDedup(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		// Simulate slow refresh to widen the race window.
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  "fresh-token",
			ExpiresIn:    86400,
			RefreshToken: "new-rt",
			TokenType:    "Bearer",
		})
	}))
	defer srv.Close()

	staleExpiry := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "stale-token", staleExpiry, "old-rt")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	const goroutines = 10
	var wg sync.WaitGroup
	results := make([]string, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = r.resolveWithRefresh()
		}(i)
	}
	wg.Wait()

	for i, got := range results {
		if got != "fresh-token" {
			t.Errorf("goroutine %d: resolveWithRefresh() = %q, want fresh-token", i, got)
		}
	}

	// The mutex ensures only 1 actual refresh (first goroutine refreshes,
	// others re-read the now-fresh file token after acquiring the lock).
	// In practice, at most 2 might fire (one refreshes, the second entered
	// refreshAndPersist before the file was re-read). Verify it's not 10.
	if c := callCount.Load(); c > 2 {
		t.Errorf("token endpoint called %d times, expected at most 2 (dedup failure)", c)
	}
}

func TestRefresh_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")

	creds := fullCredentials{
		ClaudeAIOAuth: fullOAuthBlock{
			AccessToken:  "test-token",
			ExpiresAt:    1781647414805,
			RefreshToken: "test-rt",
			Scopes:       []string{"user:inference"},
		},
	}

	if err := atomicWriteCredentials(path, creds); err != nil {
		t.Fatalf("atomicWriteCredentials: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got fullCredentials
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ClaudeAIOAuth.AccessToken != "test-token" {
		t.Errorf("access token = %q, want test-token", got.ClaudeAIOAuth.AccessToken)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}

	// No temp files left behind.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "credentials.json" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestRefresh_RotatesRefreshToken(t *testing.T) {
	srv := newTokenServer(t, "new-access", "rotated-rt", 3600)
	defer srv.Close()

	staleExpiry := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "old-access", staleExpiry, "original-rt")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	got := r.resolveWithRefresh()
	if got != "new-access" {
		t.Errorf("resolveWithRefresh() = %q, want new-access", got)
	}

	data, _ := os.ReadFile(credsPath)
	var creds fullCredentials
	json.Unmarshal(data, &creds)
	if creds.ClaudeAIOAuth.RefreshToken != "rotated-rt" {
		t.Errorf("persisted refresh token = %q, want rotated-rt", creds.ClaudeAIOAuth.RefreshToken)
	}
}

func TestRefresh_PreservesRefreshTokenWhenNotRotated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Response without refresh_token — server chose not to rotate.
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "new-access",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer srv.Close()

	staleExpiry := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "old-access", staleExpiry, "original-rt")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	got := r.resolveWithRefresh()
	if got != "new-access" {
		t.Errorf("resolveWithRefresh() = %q, want new-access", got)
	}

	data, _ := os.ReadFile(credsPath)
	var creds fullCredentials
	json.Unmarshal(data, &creds)
	if creds.ClaudeAIOAuth.RefreshToken != "original-rt" {
		t.Errorf("refresh token = %q, want original-rt (should be preserved when not rotated)", creds.ClaudeAIOAuth.RefreshToken)
	}
}

func TestRefresh_StaleTokenReturnedWhenRefreshFailsAndNoEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	staleExpiry := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "stale-token", staleExpiry, "rt")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
	}

	got := r.resolveWithRefresh()
	if got != "stale-token" {
		t.Errorf("resolveWithRefresh() = %q, want stale-token (stale beats nothing)", got)
	}
}

func TestRefresh_SendsCorrectFormParameters(t *testing.T) {
	var gotForm map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotForm = map[string]string{
			"grant_type":    r.FormValue("grant_type"),
			"refresh_token": r.FormValue("refresh_token"),
			"client_id":     r.FormValue("client_id"),
			"scope":         r.FormValue("scope"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "tok",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer srv.Close()

	staleExpiry := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC).UnixMilli()
	credsPath := writeFullCreds(t, "old", staleExpiry, "my-refresh-token")

	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
		ClientID:        "test-client-id",
	}

	r.resolveWithRefresh()

	if gotForm["grant_type"] != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", gotForm["grant_type"])
	}
	if gotForm["refresh_token"] != "my-refresh-token" {
		t.Errorf("refresh_token = %q, want my-refresh-token", gotForm["refresh_token"])
	}
	if gotForm["client_id"] != "test-client-id" {
		t.Errorf("client_id = %q, want test-client-id", gotForm["client_id"])
	}
	if gotForm["scope"] != defaultRefreshScopes {
		t.Errorf("scope = %q, want %q", gotForm["scope"], defaultRefreshScopes)
	}
}

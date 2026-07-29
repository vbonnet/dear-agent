package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRefreshStopBlocksEveryRefreshingEntrypoint(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"access_token":"fresh","expires_in":3600,"refresh_token":"rotated"}`))
	}))
	defer srv.Close()

	credsPath := writeFullCreds(t, "stale", staleMillis(), "refresh-token")
	r := OAuthResolver{
		CredentialsPath: credsPath,
		Getenv:          envGetter(map[string]string{OAuthEnvVar: "fallback"}),
		Now:             fixedClock(),
		HTTPClient:      srv.Client(),
		TokenEndpoint:   srv.URL,
		QuarantinePath:  filepath.Join(t.TempDir(), "quarantine.json"),
	}
	if err := r.WriteRefreshStop("operator review required"); err != nil {
		t.Fatalf("WriteRefreshStop: %v", err)
	}

	if _, err := r.Refresh(context.Background()); !errors.Is(err, ErrRefreshStopped) {
		t.Fatalf("Refresh() error = %v, want ErrRefreshStopped", err)
	}
	if _, _, err := r.EnsureFresh(context.Background()); !errors.Is(err, ErrRefreshStopped) {
		t.Fatalf("EnsureFresh() error = %v, want ErrRefreshStopped", err)
	}
	if got := r.resolveWithRefresh(); got != "fallback" {
		t.Fatalf("resolveWithRefresh() = %q, want fallback token", got)
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("token endpoint called %d times while durable stop was active", got)
	}
}

func TestRefreshStopIsCredentialScopedAndExplicitlyCleared(t *testing.T) {
	firstPath := writeFullCreds(t, "stale", staleMillis(), "first")
	secondPath := writeFullCreds(t, "stale", staleMillis(), "second")
	first := OAuthResolver{CredentialsPath: firstPath}
	second := OAuthResolver{CredentialsPath: secondPath}

	if err := first.WriteRefreshStop("first only"); err != nil {
		t.Fatal(err)
	}
	if stopped, err := first.RefreshStopped(); err != nil || !stopped {
		t.Fatalf("first RefreshStopped() = %v, %v; want true, nil", stopped, err)
	}
	if stopped, err := second.RefreshStopped(); err != nil || stopped {
		t.Fatalf("second RefreshStopped() = %v, %v; want false, nil", stopped, err)
	}
	if err := first.ClearRefreshStop(); err != nil {
		t.Fatal(err)
	}
	if stopped, err := first.RefreshStopped(); err != nil || stopped {
		t.Fatalf("first RefreshStopped() after clear = %v, %v; want false, nil", stopped, err)
	}
	if _, err := os.Stat(first.RefreshStopPath()); !os.IsNotExist(err) {
		t.Fatalf("refresh stop survived explicit clear: %v", err)
	}
}

func TestRefreshStopSelfClearsAfterCredentialRotation(t *testing.T) {
	credentials := writeFullCreds(t, "stale", staleMillis(), "before")
	resolver := OAuthResolver{CredentialsPath: credentials}
	if err := resolver.WriteRefreshStop("ambiguous refresh"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentials, []byte(strings.Replace(string(data), "before", "after", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if stopped, err := resolver.RefreshStopped(); err != nil || stopped {
		t.Fatalf("RefreshStopped() after rotation = %v, %v; want false, nil", stopped, err)
	}
	if _, err := os.Stat(resolver.RefreshStopPath()); !os.IsNotExist(err) {
		t.Fatalf("rotated stop marker survived: %v", err)
	}
}

func TestRefreshStopFingerprintsAttemptedTokenAfterConcurrentRotation(t *testing.T) {
	credentials := writeFullCreds(t, "stale", staleMillis(), "attempted")
	resolver := OAuthResolver{CredentialsPath: credentials}

	data, err := os.ReadFile(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentials, []byte(strings.Replace(string(data), "attempted", "rotated", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := resolver.writeRefreshStopForToken("attempted", "ambiguous exchange"); err != nil {
		t.Fatal(err)
	}

	stopped, err := resolver.RefreshStopped()
	if err != nil {
		t.Fatal(err)
	}
	if stopped {
		t.Fatal("stop for attempted token blocked the concurrently rotated replacement")
	}
}

func TestClearRefreshProtectionsClearsQuarantineAndDurableStop(t *testing.T) {
	credentials := writeFullCreds(t, "stale", staleMillis(), "ambiguous")
	quarantinePath := filepath.Join(t.TempDir(), "quarantine.json")
	resolver := OAuthResolver{
		CredentialsPath: credentials,
		QuarantinePath:  quarantinePath,
	}
	if err := resolver.writeQuarantine("ambiguous", "unknown outcome"); err != nil {
		t.Fatal(err)
	}
	if err := resolver.WriteRefreshStop("unknown outcome"); err != nil {
		t.Fatal(err)
	}

	if err := resolver.ClearRefreshProtections(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{quarantinePath, resolver.RefreshStopPath()} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("refresh protection %s survived override: %v", path, err)
		}
	}
}

func TestClearRefreshProtectionsKeepsDurableStopWhenQuarantineClearFails(t *testing.T) {
	credentials := writeFullCreds(t, "stale", staleMillis(), "ambiguous")
	quarantinePath := filepath.Join(t.TempDir(), "quarantine")
	if err := os.Mkdir(quarantinePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quarantinePath, "blocker"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver := OAuthResolver{
		CredentialsPath: credentials,
		QuarantinePath:  quarantinePath,
	}
	if err := resolver.WriteRefreshStop("unknown outcome"); err != nil {
		t.Fatal(err)
	}

	if err := resolver.ClearRefreshProtections(); err == nil {
		t.Fatal("ClearRefreshProtections() succeeded despite an uncleared quarantine")
	}
	if _, err := os.Stat(resolver.RefreshStopPath()); err != nil {
		t.Fatalf("durable stop was cleared after quarantine failure: %v", err)
	}
}

func TestInspectRefreshStopLeavesRotatedMarkerUntouched(t *testing.T) {
	credentials := writeFullCreds(t, "stale", staleMillis(), "before")
	resolver := OAuthResolver{CredentialsPath: credentials}
	if err := resolver.WriteRefreshStop("ambiguous refresh"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentials, []byte(strings.Replace(string(data), "before", "after", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if stopped, err := resolver.InspectRefreshStop(); err != nil || stopped {
		t.Fatalf("InspectRefreshStop() after rotation = %v, %v; want false, nil", stopped, err)
	}
	if _, err := os.Stat(resolver.RefreshStopPath()); err != nil {
		t.Fatalf("read-only inspection changed rotated marker: %v", err)
	}
}

func TestCredentialsPathCanonicalizesImplicitDefaultSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	target := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	defaultPath := filepath.Join(home, claudeCredentialsRelPath)
	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, defaultPath); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := (OAuthResolver{}).credentialsPath(); got != filepath.Clean(resolved) {
		t.Fatalf("credentialsPath() = %q, want canonical target %q", got, resolved)
	}
}

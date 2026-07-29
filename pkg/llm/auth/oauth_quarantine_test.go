package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// quarantineResolver builds a resolver over a temp credentials file with
// quarantine enabled, pointed at the given token endpoint.
func quarantineResolver(t *testing.T, endpoint, refreshToken string) (OAuthResolver, string, string) {
	t.Helper()
	dir := t.TempDir()
	credPath := filepath.Join(dir, ".credentials.json")
	quarPath := filepath.Join(dir, "quarantine.json")

	creds := fullCredentials{ClaudeAIOAuth: fullOAuthBlock{
		AccessToken:  "stale-access-token",
		ExpiresAt:    time.Now().Add(-time.Hour).UnixMilli(), // stale, so refresh is attempted
		RefreshToken: refreshToken,
	}}
	data, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshal creds: %v", err)
	}
	if err := os.WriteFile(credPath, data, 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}

	return OAuthResolver{
		CredentialsPath: credPath,
		QuarantinePath:  quarPath,
		TokenEndpoint:   endpoint,
		HTTPClient:      &http.Client{Timeout: 2 * time.Second},
	}, credPath, quarPath
}

// This is the ce-77ip.7 regression. A refresh request that reaches the server
// but whose response is lost may have spent the single-use token. Replaying it
// is what revoked the token family on 2026-07-18, so the refresh must report an
// unknown outcome rather than a plain error.
func TestRefresh_ResponseLostAfterRequestSent_ReportsOutcomeUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Accept the request (so it is definitively transmitted), then hang up
		// without a response — the server has seen the token, the client has not
		// seen the reply.
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

	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-maybe-spent")

	_, err := r.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected an error when the response is lost")
	}
	if !errors.Is(err, ErrRefreshOutcomeUnknown) {
		t.Fatalf("error = %v, want ErrRefreshOutcomeUnknown", err)
	}

	// The token must be quarantined so the next tick cannot replay it.
	if _, statErr := os.Stat(quarPath); statErr != nil {
		t.Fatalf("expected a quarantine marker at %s: %v", quarPath, statErr)
	}
	fp, _, _, ok := r.QuarantineStatus()
	if !ok {
		t.Fatal("QuarantineStatus reports nothing quarantined")
	}
	if want := RefreshTokenFingerprint("rt-maybe-spent"); fp != want {
		t.Errorf("quarantined fingerprint = %q, want %q", fp, want)
	}
}

// Having quarantined the token, the very next refresh must refuse to present it
// — that refusal is the whole protection.
func TestRefresh_QuarantinedTokenIsNotPresentedAgain(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-at","expires_in":3600,"refresh_token":"rt-new"}`))
	}))
	defer srv.Close()

	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-quarantined")

	// Pre-quarantine exactly the token that is on disk.
	if err := r.writeQuarantine("rt-quarantined", "earlier refresh outcome unknown"); err != nil {
		t.Fatalf("writeQuarantine: %v", err)
	}

	_, err := r.Refresh(context.Background())
	if !errors.Is(err, ErrRefreshQuarantined) {
		t.Fatalf("error = %v, want ErrRefreshQuarantined", err)
	}
	if hits != 0 {
		t.Errorf("token endpoint was called %d times; a quarantined token must never be presented", hits)
	}
	if _, statErr := os.Stat(quarPath); statErr != nil {
		t.Error("quarantine marker should persist while the token is unchanged")
	}
}

// A quarantine must not wedge the refresher forever. If any client rotates the
// token successfully, the on-disk token no longer matches the quarantined
// fingerprint and refreshing resumes on its own.
func TestRefresh_QuarantineSelfClearsWhenTokenChanges(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh-at","expires_in":3600,"refresh_token":"rt-rotated"}`))
	}))
	defer srv.Close()

	// On disk: rt-current. Quarantined: rt-old (a token somebody already replaced).
	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-current")
	if err := r.writeQuarantine("rt-old", "stale quarantine"); err != nil {
		t.Fatalf("writeQuarantine: %v", err)
	}

	tok, err := r.Refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh should proceed when the on-disk token differs from the quarantined one: %v", err)
	}
	if tok != "fresh-at" {
		t.Errorf("token = %q, want %q", tok, "fresh-at")
	}
	if _, statErr := os.Stat(quarPath); !os.IsNotExist(statErr) {
		t.Error("a stale quarantine should be cleared once the token has moved on")
	}
}

// A connection that never completes leaves the token untouched, so it must stay
// an ordinary retryable error. Conflating this with the lost-response case would
// quarantine on every transient network blip. These are the two error classes
// seen in the 2026-07-18 audit log.
func TestRefresh_ConnectionNeverEstablished_IsOrdinaryError(t *testing.T) {
	// Port 1 on loopback: connection refused, nothing ever transmitted.
	r, _, quarPath := quarantineResolver(t, "http://127.0.0.1:1/oauth/token", "rt-untouched")

	_, err := r.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if errors.Is(err, ErrRefreshOutcomeUnknown) {
		t.Errorf("connection failure must not be treated as outcome-unknown: %v", err)
	}
	if _, statErr := os.Stat(quarPath); !os.IsNotExist(statErr) {
		t.Error("an untransmitted request must not quarantine the token")
	}
}

// A 200 whose body cannot be parsed still means the server rotated the token, so
// the on-disk one is spent and must be quarantined.
func TestRefresh_UnparseableSuccessBody_Quarantines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":`)) // truncated
	}))
	defer srv.Close()

	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-spent-for-sure")

	_, err := r.Refresh(context.Background())
	if !errors.Is(err, ErrRefreshOutcomeUnknown) {
		t.Fatalf("error = %v, want ErrRefreshOutcomeUnknown", err)
	}
	if _, statErr := os.Stat(quarPath); statErr != nil {
		t.Error("a rotated-but-unreadable response must quarantine the on-disk token")
	}
}

// A deliberate 4xx rejection issues no token, so it stays retryable.
func TestRefresh_ClientErrorResponse_DoesNotQuarantine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limited"}`))
	}))
	defer srv.Close()

	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-untouched")

	_, err := r.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected an error for a 429")
	}
	if errors.Is(err, ErrRefreshOutcomeUnknown) {
		t.Errorf("a 4xx rejection must not quarantine: %v", err)
	}
	if _, statErr := os.Stat(quarPath); !os.IsNotExist(statErr) {
		t.Error("a 4xx must not quarantine the token")
	}
}

// A 5xx could have consumed the token before the server faltered, so it is
// treated as possibly spent.
func TestRefresh_ServerErrorResponse_Quarantines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-maybe-spent")

	_, err := r.Refresh(context.Background())
	if !errors.Is(err, ErrRefreshOutcomeUnknown) {
		t.Fatalf("error = %v, want ErrRefreshOutcomeUnknown for a 5xx", err)
	}
	if _, statErr := os.Stat(quarPath); statErr != nil {
		t.Error("a 5xx must quarantine the token")
	}
}

// A successful refresh clears any quarantine, since the token it named is gone.
func TestRefresh_SuccessClearsQuarantine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","expires_in":3600,"refresh_token":"rt-next"}`))
	}))
	defer srv.Close()

	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-current")
	// Quarantine a different token so the refresh is allowed to proceed.
	if err := r.writeQuarantine("rt-unrelated", "stale"); err != nil {
		t.Fatalf("writeQuarantine: %v", err)
	}

	if _, err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if _, statErr := os.Stat(quarPath); !os.IsNotExist(statErr) {
		t.Error("a successful refresh must leave no quarantine behind")
	}
}

// With QuarantinePath unset the resolver keeps its previous behavior, so
// existing callers are unaffected.
func TestRefresh_QuarantineDisabledByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	r, _, _ := quarantineResolver(t, srv.URL, "rt-x")
	r.QuarantinePath = ""

	if _, err := r.Refresh(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
	if _, _, _, ok := r.QuarantineStatus(); ok {
		t.Error("quarantine must stay disabled when QuarantinePath is empty")
	}
	if stopped, err := r.RefreshStopped(); err != nil || !stopped {
		t.Fatalf("durable fallback stop = (%v, %v), want (true, nil)", stopped, err)
	}
}

// A marker that exists but cannot be parsed might be naming the token on disk,
// so it must fail CLOSED. Treating it as "no quarantine" would replay a possibly
// spent token — the exact failure this mechanism prevents.
func TestQuarantine_CorruptMarkerFailsClosed(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","expires_in":3600,"refresh_token":"rt-next"}`))
	}))
	defer srv.Close()

	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-current")
	if err := os.WriteFile(quarPath, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := r.Refresh(context.Background())
	if !errors.Is(err, ErrQuarantineUnreadable) {
		t.Fatalf("error = %v, want ErrQuarantineUnreadable", err)
	}
	if hits != 0 {
		t.Errorf("token endpoint called %d times; an unreadable marker must block the refresh", hits)
	}
}

// A marker missing its fingerprint is equally unusable and must also fail closed.
func TestQuarantine_MarkerWithoutFingerprintFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		t.Error("token endpoint must not be reached")
	}))
	defer srv.Close()

	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-current")
	if err := os.WriteFile(quarPath, []byte(`{"quarantined_at":"2026-07-18T08:58:37Z"}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := r.Refresh(context.Background()); !errors.Is(err, ErrQuarantineUnreadable) {
		t.Fatalf("error = %v, want ErrQuarantineUnreadable", err)
	}
}

// -clear-quarantine is the escape hatch that keeps fail-closed from being a
// permanent wedge.
func TestQuarantine_ClearReleasesAnUnreadableMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","expires_in":3600,"refresh_token":"rt-next"}`))
	}))
	defer srv.Close()

	r, _, quarPath := quarantineResolver(t, srv.URL, "rt-current")
	if err := os.WriteFile(quarPath, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := r.Refresh(context.Background()); !errors.Is(err, ErrQuarantineUnreadable) {
		t.Fatalf("expected the marker to block first, got %v", err)
	}

	if err := r.ClearQuarantine(); err != nil {
		t.Fatalf("ClearQuarantine: %v", err)
	}
	if _, err := r.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh should proceed once the marker is cleared: %v", err)
	}
}

// The protection lives in the marker file, not in this process. If it cannot be
// written, the next tick would replay the token, so the failure must escalate
// rather than be logged and swallowed.
func TestRefresh_QuarantineWriteFailureEscalates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("no hijack support")
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

	r, _, _ := quarantineResolver(t, srv.URL, "rt-maybe-spent")
	// Point the marker at a path that cannot be created: an existing regular
	// file stands where the parent directory would have to be.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	r.QuarantinePath = filepath.Join(blocker, "quarantine.json")

	_, err := r.Refresh(context.Background())
	if !errors.Is(err, ErrQuarantineNotPersisted) {
		t.Fatalf("error = %v, want ErrQuarantineNotPersisted", err)
	}
	// The original cause must survive for the operator.
	if !errors.Is(err, ErrRefreshOutcomeUnknown) {
		t.Errorf("the underlying refresh failure should still be reported: %v", err)
	}
}

// -check must not tell the operator to clear a marker that is already inert, and
// must not mutate anything while saying so.
func TestQuarantineStatus_StaleMarkerReportsInactiveWithoutMutating(t *testing.T) {
	r, _, quarPath := quarantineResolver(t, "http://127.0.0.1:1", "rt-current")
	if err := r.writeQuarantine("rt-long-gone", "earlier ambiguity"); err != nil {
		t.Fatalf("writeQuarantine: %v", err)
	}

	fp, _, _, active := r.QuarantineStatus()
	if active {
		t.Error("a marker naming a rotated-away token holds nothing back; want active=false")
	}
	if want := RefreshTokenFingerprint("rt-long-gone"); fp != want {
		t.Errorf("fingerprint = %q, want %q so the operator can still see the stale marker", fp, want)
	}
	if _, err := os.Stat(quarPath); err != nil {
		t.Error("check mode promises no mutation; the marker must still be on disk")
	}
}

func TestQuarantineStatus_MatchingMarkerReportsActive(t *testing.T) {
	r, _, _ := quarantineResolver(t, "http://127.0.0.1:1", "rt-current")
	if err := r.writeQuarantine("rt-current", "response lost"); err != nil {
		t.Fatalf("writeQuarantine: %v", err)
	}

	if _, _, _, active := r.QuarantineStatus(); !active {
		t.Error("a marker naming the on-disk token is holding refreshes back; want active=true")
	}
}

func TestRefreshTokenFingerprint(t *testing.T) {
	if got := RefreshTokenFingerprint(""); got != "" {
		t.Errorf("empty token fingerprint = %q, want empty", got)
	}
	a := RefreshTokenFingerprint("rt-alpha")
	if len(a) != FingerprintLen {
		t.Errorf("length = %d, want %d", len(a), FingerprintLen)
	}
	if a == RefreshTokenFingerprint("rt-beta") {
		t.Error("distinct tokens must produce distinct fingerprints")
	}
	if a != RefreshTokenFingerprint("rt-alpha") {
		t.Error("fingerprint must be stable for the same token")
	}
}

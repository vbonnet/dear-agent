package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCredsFile(t *testing.T, dir string, creds fullCredentials) string {
	t.Helper()
	path := filepath.Join(dir, ".credentials.json")
	data, err := json.Marshal(creds)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func credsWith(access, refresh string, expiresAt int64) fullCredentials {
	var c fullCredentials
	c.ClaudeAIOAuth.AccessToken = access
	c.ClaudeAIOAuth.RefreshToken = refresh
	c.ClaudeAIOAuth.ExpiresAt = expiresAt
	return c
}

// Claude Code reads the keychain first and returns the file only when the
// keychain yields nothing. The resolver must report the same store the CLI
// will actually present, or it refreshes a credential nobody reads.
func TestResolveStorePrefersKeychainLikeTheCLI(t *testing.T) {
	dir := t.TempDir()
	path := writeCredsFile(t, dir, credsWith("file-access", "file-refresh", 1))
	r := OAuthResolver{CredentialsPath: path}

	kc := &stubStore{creds: credsWith("kc-access", "kc-refresh", 2), present: true}
	res := r.resolveStores(kc)

	if res.Primary != StoreKeychain {
		t.Fatalf("Primary = %v, want StoreKeychain", res.Primary)
	}
	if res.Effective.ClaudeAIOAuth.RefreshToken != "kc-refresh" {
		t.Fatalf("effective refreshToken = %q, want kc-refresh",
			res.Effective.ClaudeAIOAuth.RefreshToken)
	}
}

// With no keychain item the CLI falls through to the file, and so must we.
func TestResolveStoreFallsBackToFile(t *testing.T) {
	dir := t.TempDir()
	path := writeCredsFile(t, dir, credsWith("file-access", "file-refresh", 1))
	r := OAuthResolver{CredentialsPath: path}

	res := r.resolveStores(&stubStore{present: false})

	if res.Primary != StoreFile {
		t.Fatalf("Primary = %v, want StoreFile", res.Primary)
	}
	if res.Effective.ClaudeAIOAuth.RefreshToken != "file-refresh" {
		t.Fatalf("effective refreshToken = %q, want file-refresh",
			res.Effective.ClaudeAIOAuth.RefreshToken)
	}
}

// THE BUG (ce-cknn): a keychain item with a blanked refreshToken is still a
// parseable object, so it wins the CLI's read and permanently shadows a
// perfectly healthy file. The refresher reported success for months while
// every CLI process saw a dead credential. This must be detected, not ignored.
func TestResolveStoreDetectsShadowedFile(t *testing.T) {
	dir := t.TempDir()
	path := writeCredsFile(t, dir, credsWith("file-access", "file-refresh", 1))
	r := OAuthResolver{CredentialsPath: path}

	tombstone := credsWith("", "", 0)
	res := r.resolveStores(&stubStore{creds: tombstone, present: true})

	if !res.Shadowed {
		t.Fatal("Shadowed = false, want true: an unusable keychain item is hiding a refreshable file")
	}
	if res.Primary != StoreKeychain {
		t.Fatalf("Primary = %v, want StoreKeychain", res.Primary)
	}
}

// A healthy primary is not a shadow, however stale the fallback is.
func TestResolveStoreHealthyPrimaryIsNotShadowed(t *testing.T) {
	dir := t.TempDir()
	path := writeCredsFile(t, dir, credsWith("", "", 0))
	r := OAuthResolver{CredentialsPath: path}

	res := r.resolveStores(&stubStore{creds: credsWith("a", "r", 5), present: true})
	if res.Shadowed {
		t.Fatal("Shadowed = true, want false: the primary itself is refreshable")
	}
}

// Convergence is the durable fix: after a refresh the fresh credential must
// land in EVERY store the CLI might resolve, so precedence stops mattering.
func TestMirrorCredentialsWritesBothStores(t *testing.T) {
	dir := t.TempDir()
	path := writeCredsFile(t, dir, credsWith("old", "old-refresh", 1))
	r := OAuthResolver{CredentialsPath: path}

	kc := &stubStore{present: true, creds: credsWith("old", "old-refresh", 1)}
	fresh := credsWith("new-access", "new-refresh", 999)

	if err := r.mirrorCredentials(fresh, kc); err != nil {
		t.Fatalf("mirrorCredentials() error = %v", err)
	}

	if kc.written.ClaudeAIOAuth.RefreshToken != "new-refresh" {
		t.Fatalf("keychain refreshToken = %q, want new-refresh",
			kc.written.ClaudeAIOAuth.RefreshToken)
	}
	onDisk, _, ok := r.readFullCredentials()
	if !ok {
		t.Fatal("file unreadable after mirror")
	}
	if onDisk.ClaudeAIOAuth.RefreshToken != "new-refresh" {
		t.Fatalf("file refreshToken = %q, want new-refresh",
			onDisk.ClaudeAIOAuth.RefreshToken)
	}
}

// Mirroring must never overwrite a store with an unrefreshable credential:
// that would turn a recoverable split into a durable outage.
func TestMirrorCredentialsRefusesUnusableCredential(t *testing.T) {
	dir := t.TempDir()
	path := writeCredsFile(t, dir, credsWith("good", "good-refresh", 1))
	r := OAuthResolver{CredentialsPath: path}

	kc := &stubStore{present: true}
	if err := r.mirrorCredentials(credsWith("x", "", 0), kc); err == nil {
		t.Fatal("mirrorCredentials() error = nil, want refusal for a credential with no refresh token")
	}
	if kc.wroteAnything {
		t.Fatal("keychain was written with an unusable credential")
	}
	onDisk, _, _ := r.readFullCredentials()
	if onDisk.ClaudeAIOAuth.RefreshToken != "good-refresh" {
		t.Fatal("file was clobbered with an unusable credential")
	}
}

type stubStore struct {
	creds         fullCredentials
	present       bool
	written       fullCredentials
	wroteAnything bool
}

func (s *stubStore) Read() (fullCredentials, bool) { return s.creds, s.present }

func (s *stubStore) Name() CredentialStoreKind { return StoreKeychain }

func (s *stubStore) Write(c fullCredentials) error {
	s.written = c
	s.wroteAnything = true
	return nil
}

// The live ce-cknn incident: the keychain item the CLI reads DOES carry a
// refresh token, it is simply 78 days stale, while the file store holds a
// current credential the CLI never reaches. Keying the shadow check on a
// missing refresh token alone misses this, which is the shape that actually
// took the mesh down, so the check must compare freshness between stores.
func TestResolveStoreDetectsStalePrimaryHidingFresherFile(t *testing.T) {
	dir := t.TempDir()
	fresh := time.Now().Add(3 * time.Hour).UnixMilli()
	stale := time.Now().Add(-78 * 24 * time.Hour).UnixMilli()

	path := writeCredsFile(t, dir, credsWith("file-access", "file-refresh", fresh))
	r := OAuthResolver{CredentialsPath: path}

	kc := &stubStore{present: true, creds: credsWith("kc-access", "kc-refresh", stale)}
	res := r.resolveStores(kc)

	if res.Primary != StoreKeychain {
		t.Fatalf("Primary = %v, want StoreKeychain", res.Primary)
	}
	if !res.Shadowed {
		t.Fatal("Shadowed = false: a stale primary is hiding a strictly fresher file and refreshing the file cannot help")
	}
}

// The mirror-converged steady state must stay quiet: equal credentials in both
// stores is the goal, not an alarm.
func TestResolveStoreConvergedStoresAreNotShadowed(t *testing.T) {
	dir := t.TempDir()
	exp := time.Now().Add(3 * time.Hour).UnixMilli()
	path := writeCredsFile(t, dir, credsWith("same", "same-refresh", exp))
	r := OAuthResolver{CredentialsPath: path}

	kc := &stubStore{present: true, creds: credsWith("same", "same-refresh", exp)}
	if res := r.resolveStores(kc); res.Shadowed {
		t.Fatal("Shadowed = true for converged stores, want false")
	}
}

// After an operator re-login the CLI writes the new family into the keychain
// and leaves the old credential in the file. A file-sourced refresh would
// present the dead token and invalid_grant would kill the family that was just
// created, which is the loop that produced 599 dead-family markers.
func TestPreferFreshestCredentialTakesKeychainAfterRelogin(t *testing.T) {
	dir := t.TempDir()
	stale := time.Now().Add(-78 * 24 * time.Hour).UnixMilli()
	fresh := time.Now().Add(8 * time.Hour).UnixMilli()

	path := writeCredsFile(t, dir, credsWith("old-access", "dead-refresh", stale))
	fileCreds, _, ok := (OAuthResolver{CredentialsPath: path}).readFullCredentials()
	if !ok {
		t.Fatal("file unreadable")
	}

	r := OAuthResolver{
		CredentialsPath:  path,
		keychainOverride: &stubStore{present: true, creds: credsWith("new-access", "live-refresh", fresh)},
	}
	got := r.preferFreshestCredential(fileCreds)
	if got.ClaudeAIOAuth.RefreshToken != "live-refresh" {
		t.Fatalf("refreshToken = %q, want live-refresh (the file's token is dead)",
			got.ClaudeAIOAuth.RefreshToken)
	}
}

// The reverse must hold too: a stale keychain must never displace a fresher
// file, or the refresher would present the very token that is already dead.
func TestPreferFreshestCredentialKeepsFileWhenKeychainIsStale(t *testing.T) {
	dir := t.TempDir()
	stale := time.Now().Add(-78 * 24 * time.Hour).UnixMilli()
	fresh := time.Now().Add(8 * time.Hour).UnixMilli()

	path := writeCredsFile(t, dir, credsWith("good", "live-refresh", fresh))
	fileCreds, _, _ := (OAuthResolver{CredentialsPath: path}).readFullCredentials()

	r := OAuthResolver{
		CredentialsPath:  path,
		keychainOverride: &stubStore{present: true, creds: credsWith("old", "dead-refresh", stale)},
	}
	if got := r.preferFreshestCredential(fileCreds); got.ClaudeAIOAuth.RefreshToken != "live-refresh" {
		t.Fatalf("refreshToken = %q, want live-refresh", got.ClaudeAIOAuth.RefreshToken)
	}
}

// An unusable keychain record must not displace a refreshable file.
func TestPreferFreshestCredentialIgnoresUnusableKeychain(t *testing.T) {
	dir := t.TempDir()
	path := writeCredsFile(t, dir, credsWith("good", "live-refresh", time.Now().Add(time.Hour).UnixMilli()))
	fileCreds, _, _ := (OAuthResolver{CredentialsPath: path}).readFullCredentials()

	r := OAuthResolver{
		CredentialsPath:  path,
		keychainOverride: &stubStore{present: true, creds: credsWith("x", "", 1<<62)},
	}
	if got := r.preferFreshestCredential(fileCreds); got.ClaudeAIOAuth.RefreshToken != "live-refresh" {
		t.Fatalf("refreshToken = %q, want live-refresh", got.ClaudeAIOAuth.RefreshToken)
	}
}

// A resolver pointed at a non-default credentials path must never reach the
// ambient keychain. Beyond being wrong (the keychain item is the peer of the
// default file, not of an arbitrary path), the earlier version of this change
// made every unit test read the operator's real login keychain.
func TestKeychainStoreNotUsedForNonDefaultPath(t *testing.T) {
	r := OAuthResolver{CredentialsPath: filepath.Join(t.TempDir(), ".credentials.json")}
	if store := r.keychainStoreFor(); store != nil {
		t.Fatalf("keychainStoreFor() = %v for a non-default path, want nil", store)
	}
}

// A primary that is simply a little older than the file, but still fresh and
// still refreshable, must stay quiet. Alarming here would train the operator to
// ignore the one message that actually means the session is broken.
func TestResolveStoreQuietWhenPrimaryOlderButStillFresh(t *testing.T) {
	dir := t.TempDir()
	path := writeCredsFile(t, dir, credsWith("file", "file-refresh", time.Now().Add(3*time.Hour).UnixMilli()))
	r := OAuthResolver{CredentialsPath: path}

	kc := &stubStore{present: true, creds: credsWith("kc", "kc-refresh", time.Now().Add(time.Hour).UnixMilli())}
	if res := r.resolveStores(kc); res.Shadowed {
		t.Fatal("Shadowed = true for a fresh, refreshable primary that is merely older than the file")
	}
}

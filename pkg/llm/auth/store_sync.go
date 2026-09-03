package auth

import (
	"errors"
	"fmt"
	"os"
	"runtime"
)

// CredentialStoreKind names one of the stores Claude Code may resolve.
type CredentialStoreKind string

const (
	// StoreKeychain is the macOS login-keychain item, the CLI's primary.
	StoreKeychain CredentialStoreKind = "keychain"
	// StoreFile is ~/.claude/.credentials.json, the CLI's fallback.
	StoreFile CredentialStoreKind = "file"
	// StoreNone means no store held a parseable credential.
	StoreNone CredentialStoreKind = "none"
)

// ErrCredentialUnusable rejects a mirror of a credential that carries no
// refresh token. Propagating one of those would convert a recoverable split
// into a durable outage, because it would overwrite the one store that still
// held a refreshable credential.
var ErrCredentialUnusable = errors.New("refusing to mirror credentials with no refresh token")

// StoreResolution reports which store Claude Code will actually read, what it
// will find there, and whether that finding hides a healthier fallback.
type StoreResolution struct {
	// Primary is the store the CLI resolves for this environment.
	Primary CredentialStoreKind
	// Effective is the credential the CLI will present.
	Effective fullCredentials
	// Shadowed reports the failure this whole change exists to catch: the
	// primary store answered, so the CLI stops there, but what it holds is
	// worse than what the fallback holds. Refreshing the fallback in this
	// state is a no-op that reports success while every CLI process fails.
	//
	// "Worse" covers both observed shapes: a primary with no refresh token at
	// all, and a primary whose credential is simply older than the file's. The
	// live ce-cknn outage was the second shape, so testing only the first
	// would have kept the outage silent.
	Shadowed bool
	// FallbackUsable reports whether the file store holds a refreshable
	// credential, regardless of which store won.
	FallbackUsable bool
}

// usable reports whether a credential can still be refreshed. A blanked
// refresh token is Claude Code's own tombstone for a dead family, and it is
// still a parseable object, which is exactly why it shadows so effectively.
func usableCredential(c fullCredentials) bool {
	return c.ClaudeAIOAuth.RefreshToken != ""
}

// primaryIsWorse reports whether the credential the CLI will present is
// strictly inferior to the one sitting in the fallback store: either it cannot
// be refreshed at all, or the fallback carries a newer credential that the CLI
// will never reach. Either way the CLI is authenticating with the wrong
// credential and refreshing the fallback cannot change that.
func primaryIsWorse(primary, fallback fullCredentials) bool {
	if !usableCredential(primary) {
		return true
	}
	return fallback.ClaudeAIOAuth.ExpiresAt > primary.ClaudeAIOAuth.ExpiresAt
}

// keychainStoreFor builds the keychain store this process's environment maps
// to, or nil on platforms without one.
func (r OAuthResolver) keychainStoreFor() CredentialStore {
	if runtime.GOOS != "darwin" {
		return nil
	}
	getenv := r.env
	if r.Getenv == nil {
		getenv = os.Getenv
	}
	return KeychainStore{Identity: DefaultKeychainIdentity(getenv)}
}

// resolveStores mirrors Claude Code's own precedence: the keychain answers
// first and the file is consulted only when the keychain yields nothing.
//
// Reproducing the CLI's order is the point. A refresher that reads a different
// store than the CLI refreshes a credential nobody presents, which is how this
// host logged hundreds of successful refreshes through a multi-day outage.
func (r OAuthResolver) resolveStores(keychain CredentialStore) StoreResolution {
	fileCreds, _, fileOK := r.readFullCredentials()
	res := StoreResolution{Primary: StoreNone}
	if fileOK {
		res.FallbackUsable = usableCredential(fileCreds)
	}

	if keychain != nil {
		if kcCreds, ok := keychain.Read(); ok {
			res.Primary = StoreKeychain
			res.Effective = kcCreds
			res.Shadowed = res.FallbackUsable && primaryIsWorse(kcCreds, fileCreds)
			return res
		}
	}
	if fileOK {
		res.Primary = StoreFile
		res.Effective = fileCreds
	}
	return res
}

// ResolveStores reports the live store resolution for this process, using the
// real keychain on macOS.
func (r OAuthResolver) ResolveStores() StoreResolution {
	return r.resolveStores(r.keychainStoreFor())
}

// mirrorCredentials writes one credential into every store the CLI might
// resolve, so precedence stops deciding whether auth works.
//
// This is the durable half of the fix. Claude Code's composite store never
// mirrors: on a successful write it deletes the store that lost, so two
// independent writers diverge permanently and cannot reconverge on their own.
// Writing both under the caller's existing cross-process lock removes the
// divergence rather than trying to win the race.
func (r OAuthResolver) mirrorCredentials(creds fullCredentials, keychain CredentialStore) error {
	if !usableCredential(creds) {
		return ErrCredentialUnusable
	}
	var errs []error
	if err := atomicWriteCredentials(r.credentialsPath(), creds); err != nil {
		errs = append(errs, fmt.Errorf("file store: %w", err))
	}
	if keychain != nil {
		if err := keychain.Write(creds); err != nil {
			errs = append(errs, fmt.Errorf("keychain store: %w", err))
		}
	}
	return errors.Join(errs...)
}

// MirrorCredentials converges every credential store on the supplied
// credential. Callers must already hold the credentials lock.
func (r OAuthResolver) MirrorCredentials(creds fullCredentials) error {
	return r.mirrorCredentials(creds, r.keychainStoreFor())
}

// mirrorToSecondaryStores propagates an already-persisted credential to every
// store other than the file, which the caller has just written.
//
// It is deliberately separate from mirrorCredentials: the file write is the
// durability gate for a rotated refresh token and has already succeeded, so
// re-writing it here would be redundant work inside the lock.
func (r OAuthResolver) mirrorToSecondaryStores(creds fullCredentials) error {
	if !usableCredential(creds) {
		return ErrCredentialUnusable
	}
	keychain := r.keychainStoreFor()
	if keychain == nil {
		return nil
	}
	return keychain.Write(creds)
}

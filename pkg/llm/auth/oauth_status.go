package auth

import (
	"context"
	"time"
)

// TokenStatus is a read-only snapshot of the on-disk credential state, for
// status reporting (e.g. token-refresher --check). It carries no token values.
type TokenStatus struct {
	// HasToken reports whether an access token is present on disk.
	HasToken bool
	// HasRefreshToken reports whether a refresh token is present (refresh is
	// impossible without one).
	HasRefreshToken bool
	// ExpiresAt is the access-token expiry; zero when the file carried none.
	ExpiresAt time.Time
	// Fresh reports whether the access token is present and still usable
	// (accounting for the expiry skew).
	Fresh bool

	// PrimaryStore is the store Claude Code will actually read for this
	// environment. When it is not StoreFile, refreshing the file alone cannot
	// fix a broken session.
	PrimaryStore CredentialStoreKind
	// Shadowed reports that the primary store holds a credential that cannot
	// be refreshed while the file store still holds one that can. This is the
	// state in which a file-only refresher reports success indefinitely and
	// every CLI process still fails to authenticate; it needs an operator
	// re-login, not another refresh.
	Shadowed bool
}

// Status returns a snapshot of the on-disk credential state without performing
// any network call or mutation.
func (r OAuthResolver) Status() TokenStatus {
	// Report on the credential the CLI will actually present, not merely the
	// one on disk: those differ exactly when auth is broken.
	res := r.ResolveStores()
	if res.Primary == StoreNone {
		return TokenStatus{PrimaryStore: StoreNone}
	}
	creds := res.Effective
	o := creds.ClaudeAIOAuth
	st := TokenStatus{
		HasToken:        o.AccessToken != "",
		HasRefreshToken: o.RefreshToken != "",
		Fresh:           o.AccessToken != "" && r.fileTokenFresh(o.ExpiresAt),
		PrimaryStore:    res.Primary,
		Shadowed:        res.Shadowed,
	}
	if o.ExpiresAt > 0 {
		st.ExpiresAt = time.UnixMilli(o.ExpiresAt)
	}
	return st
}

// EnsureFresh returns a non-expired access token, performing a file-locked
// refresh when the on-disk token is stale. Unlike ResolveOAuthToken it returns
// errors instead of silently falling back, so a CLI can report them and set
// exit codes. refreshed reports whether a network refresh actually occurred.
func (r OAuthResolver) EnsureFresh(ctx context.Context) (token string, refreshed bool, err error) {
	fileToken, expiresAt, ok := r.readFileToken()
	if ok && r.fileTokenFresh(expiresAt) {
		return fileToken, false, nil
	}
	tok, err := r.Refresh(ctx)
	if err != nil {
		return "", false, err
	}
	return tok, true, nil
}

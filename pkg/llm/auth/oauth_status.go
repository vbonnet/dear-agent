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
}

// Status returns a snapshot of the on-disk credential state without performing
// any network call or mutation.
func (r OAuthResolver) Status() TokenStatus {
	creds, _, ok := r.readFullCredentials()
	if !ok {
		return TokenStatus{}
	}
	o := creds.ClaudeAIOAuth
	st := TokenStatus{
		HasToken:        o.AccessToken != "",
		HasRefreshToken: o.RefreshToken != "",
		Fresh:           o.AccessToken != "" && r.fileTokenFresh(o.ExpiresAt),
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

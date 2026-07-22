package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	//nolint:gosec // G101: OAuth token endpoint URL, not a credential.
	defaultTokenEndpoint = "https://platform.claude.com/v1/oauth/token"

	// defaultClientID is the public Claude Code OAuth client identifier. This
	// is a published, non-secret value. A wrong client_id makes the token
	// endpoint reject every refresh with 400, which is the failure that kept
	// killing the mesh (the prior value 22422756-… was incorrect); the
	// endpoint and this ID stay env-overridable because both have migrated
	// before. See ce-rnpt / ce-f3e3.
	defaultClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"

	defaultRefreshScopes = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"

	// defaultUserAgent mimics a CLI client so the request looks ordinary to the
	// Cloudflare WAF in front of the token endpoint. A bare Go http.Client UA
	// has been observed to trip the WAF.
	defaultUserAgent = "claude-cli (dear-agent token-refresher)"

	// envTokenEndpoint and envClientID let operators redirect the refresh if
	// the endpoint or client ID migrate again, without a rebuild.
	envTokenEndpoint = "CLAUDE_OAUTH_TOKEN_ENDPOINT" //nolint:gosec // G101: env var name, not a credential value.
	envClientID      = "CLAUDE_OAUTH_CLIENT_ID"
	envUserAgent     = "CLAUDE_OAUTH_USER_AGENT"

	// credentialsBackupSuffix is appended to the credentials path to form the
	// pre-refresh backup written before each mutation.
	credentialsBackupSuffix = ".bak"

	// credentialsLockName is the advisory lock file that serializes credential
	// refreshes; it sits beside the credentials file (~/.claude/.credentials.lock).
	credentialsLockName = ".credentials.lock"

	// maxErrBodyBytes caps how much of a non-2xx response body we read for
	// diagnostics, so a hostile/oversized error page can't exhaust memory.
	maxErrBodyBytes = 4 << 10
)

// ErrTokenFamilyDead signals an unrecoverable refresh: the OAuth server
// rejected the refresh token with 400 invalid_grant. The token family is dead
// (the refresh token was rotated out from under us, revoked, or never valid),
// so retrying is futile — a human must re-authenticate (claude /login or
// `claude setup-token`). Callers should escalate rather than loop.
var ErrTokenFamilyDead = errors.New("oauth refresh token rejected (invalid_grant): token family is dead, re-authentication required")

// ErrRefreshNotPersisted signals that a refresh succeeded on the server (the
// refresh token was rotated) but the new credentials could NOT be written to
// disk. This is dangerous: the rotated refresh token now exists only in memory,
// so the next process to refresh will present the stale on-disk token and kill
// the family. Callers must treat this as critical and surface it loudly.
var ErrRefreshNotPersisted = errors.New("oauth refresh succeeded but new credentials could not be persisted")

// tokenResponse is the subset of the OAuth2 token endpoint response we need.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// fullCredentials mirrors the complete ~/.claude/.credentials.json structure,
// preserving fields the resolver doesn't use so they survive a round-trip.
type fullCredentials struct {
	ClaudeAIOAuth fullOAuthBlock `json:"claudeAiOauth"`
}

type fullOAuthBlock struct {
	AccessToken  string   `json:"accessToken"`
	ExpiresAt    int64    `json:"expiresAt"`
	RefreshToken string   `json:"refreshToken"`
	Scopes       []string `json:"scopes,omitempty"`
}

// readFullCredentials reads and parses the credentials file, preserving all fields.
func (r OAuthResolver) readFullCredentials() (fullCredentials, string, bool) {
	path := r.credentialsPath()
	if path == "" {
		return fullCredentials{}, "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fullCredentials{}, path, false
	}
	var creds fullCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return fullCredentials{}, path, false
	}
	return creds, path, true
}

// credentialsPath returns the resolved credentials file path.
func (r OAuthResolver) credentialsPath() string {
	if r.CredentialsPath != "" {
		return r.CredentialsPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, claudeCredentialsRelPath)
}

// nowFn returns the resolver's clock, defaulting to time.Now.
func (r OAuthResolver) nowFn() func() time.Time {
	if r.Now != nil {
		return r.Now
	}
	return time.Now
}

// env resolves an environment variable through the resolver's Getenv hook.
func (r OAuthResolver) env(key string) string {
	if r.Getenv != nil {
		return r.Getenv(key)
	}
	return os.Getenv(key)
}

// Refresh ensures the credentials file holds a non-expired access token,
// performing an OAuth2 refresh-token exchange when the on-disk token is stale.
// It is safe to call concurrently across processes and goroutines: the whole
// read-check-exchange-write cycle runs under a cross-process file lock, and the
// freshness check is re-evaluated under the lock so a pane that lost the race
// returns the token a sibling already refreshed instead of burning the
// single-use refresh token a second time. Returns the fresh access token.
//
// Errors are typed where it matters: ErrTokenFamilyDead (escalate to a human)
// and ErrRefreshNotPersisted (critical: rotated token not on disk).
func (r OAuthResolver) Refresh(ctx context.Context) (string, error) {
	if r.HTTPClient == nil {
		return "", errors.New("refresh requires a non-nil HTTPClient")
	}
	path := r.credentialsPath()
	if path == "" {
		return "", errors.New("cannot resolve credentials file path")
	}

	var token string
	err := withCredentialsLock(ctx, path, r.LockTimeout, func() error {
		// Re-read under the lock: a sibling pane may have just refreshed.
		creds, _, ok := r.readFullCredentials()
		if !ok {
			return errors.New("credentials file unreadable under lock")
		}
		if creds.ClaudeAIOAuth.AccessToken != "" && r.fileTokenFresh(creds.ClaudeAIOAuth.ExpiresAt) {
			token = creds.ClaudeAIOAuth.AccessToken
			r.log("oauth.refresh.skipped", "reason", "another process already refreshed")
			return nil
		}
		if creds.ClaudeAIOAuth.RefreshToken == "" {
			return errors.New("no refresh token available in credentials file")
		}

		// Refuse to re-present a token an earlier ambiguous refresh may already
		// have spent. Checked under the lock and against the token we are about
		// to send, so it self-clears as soon as anyone rotates successfully.
		if qerr := r.checkQuarantine(creds.ClaudeAIOAuth.RefreshToken); qerr != nil {
			r.log("oauth.refresh.quarantined", "error", qerr.Error())
			return qerr
		}

		r.log("oauth.refresh.attempt", "endpoint", r.endpoint())
		tok, err := r.exchange(ctx, creds.ClaudeAIOAuth.RefreshToken)
		if err != nil {
			r.log("oauth.refresh.failed", "error", err.Error())
			if errors.Is(err, ErrRefreshOutcomeUnknown) {
				// The token may be spent. Record it so the next tick refuses to
				// replay it, which is what revokes the family.
				if qerr := r.writeQuarantine(creds.ClaudeAIOAuth.RefreshToken, err.Error()); qerr != nil {
					// The protection lives in that file, not in this process:
					// the next tick is a fresh process that reads the marker. A
					// failed write therefore means the replay WILL happen unless
					// a human intervenes, so it is escalated rather than logged
					// and swallowed.
					r.log("oauth.refresh.quarantine_write_failed", "error", qerr.Error())
					return fmt.Errorf("%w: %w (original refresh failure: %w)",
						ErrQuarantineNotPersisted, qerr, err)
				}
			}
			return err
		}

		// Back up the pre-refresh credentials before mutating, so a botched
		// write or a transient bad response is recoverable.
		if berr := backupCredentials(path); berr != nil {
			// Non-fatal: a missing backup is a weaker guarantee, not a reason
			// to abandon a successful refresh.
			r.log("oauth.refresh.backup_failed", "error", berr.Error())
		}

		updated := creds
		updated.ClaudeAIOAuth.AccessToken = tok.AccessToken
		updated.ClaudeAIOAuth.ExpiresAt = r.nowFn()().Add(time.Duration(tok.ExpiresIn) * time.Second).UnixMilli()
		if tok.RefreshToken != "" {
			updated.ClaudeAIOAuth.RefreshToken = tok.RefreshToken
		}

		if werr := atomicWriteCredentials(path, updated); werr != nil {
			// CRITICAL: the server already rotated the refresh token, but we
			// could not persist it. Surface loudly — never swallow this, or the
			// next refresh will use the dead on-disk token and kill the family.
			r.log("oauth.refresh.persist_failed", "error", werr.Error())
			return fmt.Errorf("%w: %w", ErrRefreshNotPersisted, werr)
		}

		// The rotation completed and is on disk, so any earlier quarantine is
		// moot: the token it named is gone.
		if qerr := r.ClearQuarantine(); qerr != nil {
			r.log("oauth.refresh.quarantine_clear_failed", "error", qerr.Error())
		}

		token = tok.AccessToken
		r.log("oauth.refresh.success", "expires_in_seconds", tok.ExpiresIn, "rotated", tok.RefreshToken != "")
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// endpoint resolves the token endpoint: explicit field, then env override, then
// the built-in default.
func (r OAuthResolver) endpoint() string {
	if r.TokenEndpoint != "" {
		return r.TokenEndpoint
	}
	if v := r.env(envTokenEndpoint); v != "" {
		return v
	}
	return defaultTokenEndpoint
}

// clientID resolves the OAuth client ID: explicit field, then env override,
// then the built-in default.
func (r OAuthResolver) clientID() string {
	if r.ClientID != "" {
		return r.ClientID
	}
	if v := r.env(envClientID); v != "" {
		return v
	}
	return defaultClientID
}

// userAgent resolves the request User-Agent: env override, then the WAF-safe
// default.
func (r OAuthResolver) userAgent() string {
	if v := r.env(envUserAgent); v != "" {
		return v
	}
	return defaultUserAgent
}

// exchange performs the OAuth2 refresh-token grant and returns the token
// response. It maps 400 invalid_grant to ErrTokenFamilyDead so callers can
// escalate instead of retrying a dead family.
func (r OAuthResolver) exchange(ctx context.Context, refreshToken string) (tokenResponse, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {r.clientID()},
		"scope":         {defaultRefreshScopes},
	}

	// wroteRequest records whether the request actually went out on the wire.
	// This is the difference between a harmless failure and one that kills the
	// token family: if the request was never transmitted the refresh token is
	// untouched and retrying is safe, but if it WAS transmitted the server may
	// have consumed the single-use token and returned a replacement we never
	// read. Tracing it is exact, unlike matching on error text — "TLS handshake
	// timeout" and "Client.Timeout exceeded while awaiting headers" are both
	// plain transport errors from the caller's side but sit on opposite sides of
	// this line. See ce-77ip.7 and ErrRefreshOutcomeUnknown.
	var wroteRequest bool
	trace := &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				wroteRequest = true
			}
		},
	}
	ctx = httptrace.WithClientTrace(ctx, trace)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, fmt.Errorf("build token refresh request: %w", err)
	}
	// WAF-friendly headers: a request that looks like an ordinary client is
	// less likely to be challenged by Cloudflare than a bare Go default.
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", r.userAgent())

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		if wroteRequest {
			// The token is on the wire and we will never learn what the server
			// did with it. Treat it as possibly spent.
			return tokenResponse{}, fmt.Errorf("%w: %w", ErrRefreshOutcomeUnknown, err)
		}
		return tokenResponse{}, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes))
		if resp.StatusCode == http.StatusBadRequest && bytes.Contains(body, []byte("invalid_grant")) {
			return tokenResponse{}, fmt.Errorf("%w (status 400)", ErrTokenFamilyDead)
		}
		// A 5xx leaves it genuinely open whether the token was consumed before
		// the server faltered, so it is treated as possibly spent. A 4xx is a
		// deliberate rejection: no token was issued, so the on-disk one is
		// untouched and retrying is safe.
		if resp.StatusCode >= http.StatusInternalServerError {
			return tokenResponse{}, fmt.Errorf("%w: token refresh returned %d: %s",
				ErrRefreshOutcomeUnknown, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return tokenResponse{}, fmt.Errorf("token refresh returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Past this point the server returned 200, so it HAS rotated the refresh
	// token. Any failure to read the reply means the replacement is lost and the
	// on-disk token is definitively spent — the same recovery as the ambiguous
	// case, so it carries the same error.
	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return tokenResponse{}, fmt.Errorf("%w: decode token response: %w", ErrRefreshOutcomeUnknown, err)
	}
	if tok.AccessToken == "" {
		return tokenResponse{}, fmt.Errorf("%w: token response contained no access_token", ErrRefreshOutcomeUnknown)
	}
	return tok, nil
}

// backupCredentials copies the credentials file to <path>.bak (mode 0600)
// before a refresh mutates it, so a bad write is recoverable. A missing source
// file is not an error (nothing to back up yet).
func backupCredentials(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(path+credentialsBackupSuffix, data, 0o600)
}

// atomicWriteCredentials writes creds to path via a temp file + rename so
// concurrent readers never see a partial write.
func atomicWriteCredentials(path string, creds fullCredentials) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// defaultRefreshingResolver is the package-level resolver used by
// ResolveOAuthToken. It has a real HTTP client so it can perform token refresh.
var defaultRefreshingResolver = &OAuthResolver{
	HTTPClient: &http.Client{Timeout: 30 * time.Second},
}

// resolveWithRefresh is Resolve() plus an automatic, file-locked refresh
// attempt when the file token is stale and a refresh token + HTTP client are
// available. On any refresh failure it falls back to the env token, then the
// (stale) file token, so a still-usable source is never dropped.
func (r OAuthResolver) resolveWithRefresh() string {
	envToken := r.env(OAuthEnvVar)

	fileToken, expiresAt, ok := r.readFileToken()
	if !ok {
		return envToken
	}
	if r.fileTokenFresh(expiresAt) {
		return fileToken
	}

	if r.HTTPClient != nil {
		if newToken, err := r.Refresh(context.Background()); err == nil && newToken != "" {
			return newToken
		}
	}

	if envToken != "" {
		return envToken
	}
	return fileToken
}

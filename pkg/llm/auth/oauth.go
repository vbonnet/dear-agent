package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// OAuthEnvVar is the environment variable Claude Code reads its OAuth (Max-plan)
// access token from. agm forwards it into spawned worker sessions so they
// inherit the orchestrator's auth.
const OAuthEnvVar = "CLAUDE_CODE_OAUTH_TOKEN"

// claudeCredentialsRelPath is the location of the Claude Code OAuth credentials
// file relative to the user's home directory. Claude Code owns this file and
// auto-refreshes the access token shortly before it expires.
//
//nolint:gosec // G101: this is a filesystem path to the credentials file, not a credential value.
const claudeCredentialsRelPath = ".claude/.credentials.json"

// defaultExpirySkew treats a file token that expires within this window as
// already stale, so a worker is never handed a token that dies seconds after
// it launches.
const defaultExpirySkew = 60 * time.Second

// claudeCredentials mirrors the subset of ~/.claude/.credentials.json that the
// resolver needs. expiresAt is a Unix epoch in milliseconds (0 if absent).
type claudeCredentials struct {
	ClaudeAIOAuth struct {
		AccessToken string `json:"accessToken"`
		ExpiresAt   int64  `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

// OAuthResolver resolves the Claude Code OAuth access token, preferring the
// live token from the auto-refreshed credentials file over the
// CLAUDE_CODE_OAUTH_TOKEN environment variable.
//
// This fixes ce-dzhz: an OAuth token captured into the orchestrator's
// environment goes stale once Claude Code auto-refreshes the credentials file,
// so agm-spawned workers that inherited the env token 401 on every turn while
// the host (which reads the file) keeps working. Reading the file first means
// each spawn gets the freshest token available.
//
// The zero value is ready to use and reads the real environment and the real
// $HOME/.claude/.credentials.json. The fields exist for test injection.
type OAuthResolver struct {
	// CredentialsPath overrides the credentials-file location. Empty means
	// $HOME/.claude/.credentials.json.
	CredentialsPath string
	// Getenv overrides environment lookup. Nil means os.Getenv.
	Getenv func(string) string
	// Now overrides the clock used for expiry checks. Nil means time.Now.
	Now func() time.Time
	// ExpirySkew treats file tokens expiring within this window as stale.
	// Zero means defaultExpirySkew.
	ExpirySkew time.Duration

	// HTTPClient, when non-nil, enables automatic token refresh via the
	// OAuth2 refresh-token exchange. A nil HTTPClient disables refresh,
	// preserving the read-only behavior of the zero-value resolver.
	HTTPClient *http.Client
	// TokenEndpoint overrides the OAuth2 token endpoint URL.
	// Empty means https://platform.claude.com/v1/oauth/token (or the
	// CLAUDE_OAUTH_TOKEN_ENDPOINT env override).
	TokenEndpoint string
	// ClientID overrides the OAuth2 client identifier.
	// Empty means the Claude Code client ID (or the CLAUDE_OAUTH_CLIENT_ID
	// env override).
	ClientID string

	// LockTimeout bounds how long Refresh waits for the cross-process
	// credentials lock. Zero means defaultLockTimeout.
	LockTimeout time.Duration

	// QuarantinePath is where the refresh-token quarantine marker is kept. When
	// a refresh request reaches the server but its response is lost, the token
	// may already be spent, and re-presenting it revokes the whole family
	// (ce-77ip.7). Setting this makes Refresh record such a token and refuse to
	// present it again until it changes on disk or the operator clears it.
	// Empty disables quarantine and restores the prior retry-blindly behavior.
	QuarantinePath string

	// Logger, when non-nil, receives structured refresh events (attempt,
	// success, failure) for OTel/observability. Token values are never logged.
	// Nil disables logging.
	Logger *slog.Logger
}

// log emits a structured refresh event through the resolver's Logger, if one is
// set. It is a no-op when Logger is nil. Token values are never passed here.
func (r OAuthResolver) log(msg string, args ...any) {
	if r.Logger != nil {
		r.Logger.Info(msg, args...)
	}
}

// Resolve returns the freshest available OAuth token. It prefers a non-expired
// token from the credentials file; otherwise it falls back to the
// CLAUDE_CODE_OAUTH_TOKEN env var; failing that it returns whatever (possibly
// stale) file token exists, so a single still-usable source is never dropped.
// Returns "" when no source yields a token.
func (r OAuthResolver) Resolve() string {
	getenv := r.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	envToken := getenv(OAuthEnvVar)

	fileToken, expiresAt, ok := r.readFileToken()
	if !ok {
		// File missing, unreadable, malformed, or token-less: the env var is
		// the only source.
		return envToken
	}
	if r.fileTokenFresh(expiresAt) {
		// Primary path: a live, non-expired token straight from the file Claude
		// Code keeps refreshed.
		return fileToken
	}
	// File token is expired/expiring: a present env token is preferred, but a
	// stale file token still beats nothing.
	if envToken != "" {
		return envToken
	}
	return fileToken
}

// readFileToken reads and parses the credentials file. ok is false when the
// file is absent, unreadable, malformed, or carries no access token.
func (r OAuthResolver) readFileToken() (token string, expiresAtMillis int64, ok bool) {
	path := r.CredentialsPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", 0, false
		}
		path = filepath.Join(home, claudeCredentialsRelPath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, false
	}
	var creds claudeCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", 0, false
	}
	if creds.ClaudeAIOAuth.AccessToken == "" {
		return "", 0, false
	}
	return creds.ClaudeAIOAuth.AccessToken, creds.ClaudeAIOAuth.ExpiresAt, true
}

// fileTokenFresh reports whether a token expiring at expiresAtMillis is still
// usable, accounting for ExpirySkew. A non-positive expiresAtMillis means the
// file carried no expiry info, in which case the token is assumed usable.
func (r OAuthResolver) fileTokenFresh(expiresAtMillis int64) bool {
	if expiresAtMillis <= 0 {
		return true
	}
	skew := r.ExpirySkew
	if skew == 0 {
		skew = defaultExpirySkew
	}
	now := r.Now
	if now == nil {
		now = time.Now
	}
	expiry := time.UnixMilli(expiresAtMillis)
	return now().Add(skew).Before(expiry)
}

// ResolveOAuthToken returns the freshest available Claude Code OAuth token,
// preferring ~/.claude/.credentials.json over CLAUDE_CODE_OAUTH_TOKEN. When the
// file token is stale, it attempts an automatic refresh using the refresh token
// from the credentials file (ce-f3e3). It is the package-level convenience
// wrapper over OAuthResolver for production callers.
func ResolveOAuthToken() string {
	return defaultRefreshingResolver.resolveWithRefresh()
}

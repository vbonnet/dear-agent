package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultTokenEndpoint = "https://platform.claude.com/v1/oauth/token"
	defaultClientID      = "22422756-60c9-4084-8eb7-27705fd5cf9a"

	//nolint:gosec // G101: these are OAuth scope identifiers, not credentials.
	defaultRefreshScopes = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
)

// refreshMu serializes in-process refresh attempts. Only contended during the
// brief refresh window (~once per 23h token lifetime), so the cost is negligible.
var refreshMu sync.Mutex

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

// refreshAndPersist performs an OAuth2 refresh-token exchange and atomically
// writes the updated credentials back to disk. Returns the new access token
// on success, or an error.
func (r OAuthResolver) refreshAndPersist() (string, error) {
	if r.HTTPClient == nil {
		return "", fmt.Errorf("no HTTP client configured")
	}

	creds, path, ok := r.readFullCredentials()
	if !ok || creds.ClaudeAIOAuth.RefreshToken == "" {
		return "", fmt.Errorf("no refresh token available")
	}

	endpoint := r.TokenEndpoint
	if endpoint == "" {
		endpoint = defaultTokenEndpoint
	}
	clientID := r.ClientID
	if clientID == "" {
		clientID = defaultClientID
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {creds.ClaudeAIOAuth.RefreshToken},
		"client_id":     {clientID},
		"scope":         {defaultRefreshScopes},
	}

	resp, err := r.HTTPClient.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token refresh returned %d", resp.StatusCode)
	}

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("token response contained no access_token")
	}

	now := r.Now
	if now == nil {
		now = time.Now
	}
	expiresAt := now().Add(time.Duration(tok.ExpiresIn) * time.Second).UnixMilli()

	updated := creds
	updated.ClaudeAIOAuth.AccessToken = tok.AccessToken
	updated.ClaudeAIOAuth.ExpiresAt = expiresAt
	if tok.RefreshToken != "" {
		updated.ClaudeAIOAuth.RefreshToken = tok.RefreshToken
	}

	if err := atomicWriteCredentials(path, updated); err != nil {
		return tok.AccessToken, nil
	}

	return tok.AccessToken, nil
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

// resolveWithRefresh is Resolve() plus an automatic refresh attempt when the
// file token is stale and a refresh token + HTTP client are available.
func (r OAuthResolver) resolveWithRefresh() string {
	getenv := r.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	envToken := getenv(OAuthEnvVar)

	fileToken, expiresAt, ok := r.readFileToken()
	if !ok {
		return envToken
	}
	if r.fileTokenFresh(expiresAt) {
		return fileToken
	}

	// Token is stale — attempt refresh if we have the machinery.
	if r.HTTPClient != nil {
		refreshMu.Lock()
		// Re-read after acquiring the lock: another goroutine may have refreshed.
		if ft, ea, ok2 := r.readFileToken(); ok2 && r.fileTokenFresh(ea) {
			refreshMu.Unlock()
			return ft
		}
		newToken, err := r.refreshAndPersist()
		refreshMu.Unlock()
		if err == nil && newToken != "" {
			return newToken
		}
	}

	if envToken != "" {
		return envToken
	}
	return fileToken
}

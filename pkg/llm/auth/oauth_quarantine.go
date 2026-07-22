package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FingerprintLen is how many hex characters of a SHA-256 digest identify a
// refresh token in logs and quarantine records. It only ever has to answer "is
// this the same token as before?", so a short prefix is enough, and a digest
// prefix is not reversible to the token.
const FingerprintLen = 12

// RefreshTokenFingerprint returns a short, non-reversible fingerprint of a
// refresh token. The audit log and the quarantine record share this function so
// a post-mortem can line them up directly.
func RefreshTokenFingerprint(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])[:FingerprintLen]
}

// ErrRefreshOutcomeUnknown signals that a refresh request reached the server but
// its response never did. This is the failure that kills token families.
//
// OAuth refresh tokens here are single-use and rotating. Once the request is on
// the wire the server may have consumed the token and issued a replacement we
// never received, which leaves the on-disk token spent but looking valid.
// Presenting it again is a replay, and rotation treats a replay as proof of
// theft: the server revokes the entire family and a human has to run
// `claude /login`.
//
// The 2026-07-18 family death happened exactly this way — a
// "Client.Timeout exceeded while awaiting headers" at 08:58:37Z, then the same
// token presented again at 10:29:06Z. See ce-77ip.7.
var ErrRefreshOutcomeUnknown = errors.New("oauth refresh request was sent but no response was received: the refresh token may already be spent")

// ErrRefreshQuarantined signals that the refresh token on disk is the one whose
// fate was left unknown by an earlier ErrRefreshOutcomeUnknown, so we are
// deliberately refusing to present it again.
//
// Refusing is strictly safer than retrying. If the server did rotate the token,
// presenting it guarantees family revocation, which takes down every OAuth
// client on the host at once; holding back instead lets the current access token
// live out its natural expiry while the operator is alerted. If the server never
// processed the request, the cost is a stalled refresh cycle, recoverable with
// ClearQuarantine (token-refresher -clear-quarantine).
var ErrRefreshQuarantined = errors.New("oauth refresh token is quarantined: an earlier refresh may have spent it, so re-presenting it would risk killing the token family")

// quarantineRecord is the on-disk quarantine marker. It holds a fingerprint, not
// a token.
type quarantineRecord struct {
	// RefreshTokenFP fingerprints the refresh token whose fate is unknown.
	RefreshTokenFP string `json:"refresh_token_fp"`
	// QuarantinedAt is when the ambiguous refresh happened (RFC3339, UTC).
	QuarantinedAt string `json:"quarantined_at"`
	// Reason is the underlying transport error, for the operator.
	Reason string `json:"reason"`
}

// quarantinePath resolves where the quarantine marker lives. An empty return
// disables quarantine entirely (the resolver then behaves as it did before).
func (r OAuthResolver) quarantinePath() string {
	return r.QuarantinePath
}

// readQuarantine loads the quarantine marker. A missing or unparseable marker is
// treated as "nothing quarantined": a corrupt marker must not wedge refreshes
// forever.
func (r OAuthResolver) readQuarantine() (quarantineRecord, bool) {
	path := r.quarantinePath()
	if path == "" {
		return quarantineRecord{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return quarantineRecord{}, false
	}
	var rec quarantineRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return quarantineRecord{}, false
	}
	if rec.RefreshTokenFP == "" {
		return quarantineRecord{}, false
	}
	return rec, true
}

// writeQuarantine records that the given refresh token may have been spent.
// Best-effort: a failure to write is reported so the caller can escalate, but it
// never masks the original refresh error.
func (r OAuthResolver) writeQuarantine(refreshToken, reason string) error {
	path := r.quarantinePath()
	if path == "" {
		return nil
	}
	rec := quarantineRecord{
		RefreshTokenFP: RefreshTokenFingerprint(refreshToken),
		QuarantinedAt:  r.nowFn()().UTC().Format(time.RFC3339),
		Reason:         reason,
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ClearQuarantine removes the quarantine marker, re-arming automatic refresh.
// Called on every successful exchange (the token moved on, so the quarantined
// fingerprint is moot) and by the operator override.
func (r OAuthResolver) ClearQuarantine() error {
	path := r.quarantinePath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// QuarantineStatus reports the active quarantine, if any, for status output.
// Returns ok=false when nothing is quarantined.
func (r OAuthResolver) QuarantineStatus() (fingerprint, at, reason string, ok bool) {
	rec, found := r.readQuarantine()
	if !found {
		return "", "", "", false
	}
	return rec.RefreshTokenFP, rec.QuarantinedAt, rec.Reason, true
}

// checkQuarantine returns ErrRefreshQuarantined when the refresh token we are
// about to present is the one an earlier ambiguous refresh may have spent.
//
// The comparison is by fingerprint, so a quarantine self-clears the moment the
// on-disk token changes: if any client refreshed successfully, the stored
// fingerprint no longer matches and refreshing resumes on its own.
func (r OAuthResolver) checkQuarantine(refreshToken string) error {
	rec, ok := r.readQuarantine()
	if !ok {
		return nil
	}
	if rec.RefreshTokenFP != RefreshTokenFingerprint(refreshToken) {
		// A different token is on disk now — someone refreshed successfully and
		// the quarantined token is history. Drop the stale marker.
		_ = r.ClearQuarantine()
		return nil
	}
	return fmt.Errorf("%w (fingerprint %s, quarantined %s: %s)",
		ErrRefreshQuarantined, rec.RefreshTokenFP, rec.QuarantinedAt, rec.Reason)
}

package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
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

// ErrQuarantineUnreadable signals that a quarantine marker exists but could not
// be read or parsed. It deliberately blocks the refresh rather than assuming no
// quarantine is active: an unreadable marker may well be naming the token we are
// about to present, and guessing wrong revokes the family. `-clear-quarantine`
// removes the marker and releases the block.
var ErrQuarantineUnreadable = errors.New("oauth refresh quarantine marker exists but cannot be read: refusing to present the refresh token")

// ErrQuarantineNotPersisted signals that a possibly-spent refresh token could
// not be recorded. The protection is only as durable as this marker — the next
// process reads the file, not our memory — so a failed write means the next tick
// will replay the token and kill the family. It is reported as its own critical
// outcome so the operator can intervene before that happens.
var ErrQuarantineNotPersisted = errors.New("oauth refresh token may be spent but the quarantine marker could not be written")

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

// readQuarantine loads the quarantine marker.
//
// Only "the file is not there" means no quarantine. Every other failure — a
// permissions or I/O error, malformed JSON, a truncated write missing the
// fingerprint — returns ErrQuarantineUnreadable so the caller fails CLOSED. A
// marker that exists but cannot be understood may be naming the very token we
// are about to present, and the whole point of this mechanism is that guessing
// wrong revokes the family. `-clear-quarantine` is the escape hatch, so this
// cannot wedge refreshes permanently.
func (r OAuthResolver) readQuarantine() (quarantineRecord, bool, error) {
	path := r.quarantinePath()
	if path == "" {
		return quarantineRecord{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// ENOTDIR means a parent component is not a directory, so no marker can
		// exist at this path — same as ENOENT. Treating it as unreadable would
		// block every refresh with nothing for -clear-quarantine to remove; the
		// misconfiguration surfaces instead as ErrQuarantineNotPersisted the
		// moment we actually need to record something.
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return quarantineRecord{}, false, nil
		}
		return quarantineRecord{}, false, fmt.Errorf("%w: %w", ErrQuarantineUnreadable, err)
	}
	var rec quarantineRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return quarantineRecord{}, false, fmt.Errorf("%w: malformed marker %s: %w", ErrQuarantineUnreadable, path, err)
	}
	if rec.RefreshTokenFP == "" {
		return quarantineRecord{}, false, fmt.Errorf("%w: marker %s carries no fingerprint", ErrQuarantineUnreadable, path)
	}
	return rec, true, nil
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

// QuarantineStatus reports whether a quarantine is actually holding back
// refreshes right now, for status output. It is read-only: `-check` promises no
// mutation, so a marker naming a token that is no longer on disk is reported as
// inactive rather than being deleted here. The next Refresh clears it.
//
// active=false with a non-empty fingerprint means "a stale marker is present but
// harmless", which is why the two are reported separately.
func (r OAuthResolver) QuarantineStatus() (fingerprint, at, reason string, active bool) {
	rec, found, err := r.readQuarantine()
	if err != nil {
		// Unreadable markers DO block refreshes, so report them as active.
		return "", "", err.Error(), true
	}
	if !found {
		return "", "", "", false
	}
	// A marker only holds anything back if it names the token currently on disk.
	// Reporting otherwise would tell the operator to run -clear-quarantine when
	// nothing is wrong, contradicting the self-clearing behavior (CTR-13).
	creds, _, ok := r.readFullCredentials()
	if ok && rec.RefreshTokenFP != RefreshTokenFingerprint(creds.ClaudeAIOAuth.RefreshToken) {
		return rec.RefreshTokenFP, rec.QuarantinedAt, rec.Reason, false
	}
	return rec.RefreshTokenFP, rec.QuarantinedAt, rec.Reason, true
}

// checkQuarantine returns an error when the refresh token we are about to
// present must not be sent: either it is the token an earlier ambiguous refresh
// may have spent, or a marker exists that we cannot read and therefore cannot
// rule out.
//
// The comparison is by fingerprint, so a quarantine self-clears the moment the
// on-disk token changes: if any client refreshed successfully, the stored
// fingerprint no longer matches and refreshing resumes on its own.
func (r OAuthResolver) checkQuarantine(refreshToken string) error {
	rec, ok, err := r.readQuarantine()
	if err != nil {
		return err
	}
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

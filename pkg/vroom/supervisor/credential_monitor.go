package supervisor

import (
	"context"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// DefaultCredentialStaleWindow is how far ahead of expiry a credential is
// treated as "stale" by the Overseer. The whole point of this monitor is to
// raise a P0 BEFORE the mesh starts taking 401s, so we alert while there is
// still a usable token — ten minutes is enough lead time for a refresh (or a
// human /login) to land before the access token actually dies.
const DefaultCredentialStaleWindow = 10 * time.Minute

// CredentialStaleKind is the decision-trail Kind the Overseer appends when the
// shared OAuth credential is expired or expiring within the stale window. The
// mesh watches for this to 401-avoid proactively.
const CredentialStaleKind = "supervisor.credential.stale"

// CredentialState is the outcome of a no-network credential freshness check.
//
// The numeric values are wired to deliberately match the exit-code contract a
// CLI wrapper exposes (and the contract documented for `token-refresher
// -check`): 0 fresh, 1 expiring, 2 expired, 3 missing. Keep them in sync with
// cmd/credential-monitor.
type CredentialState int

const (
	// CredentialFresh means an access token is present and does not expire
	// within the stale window.
	CredentialFresh CredentialState = 0
	// CredentialExpiring means a token is present and still valid right now,
	// but expires within the stale window — the proactive-alert case.
	CredentialExpiring CredentialState = 1
	// CredentialExpired means the access token's expiry is at or before now;
	// the mesh is about to (or already does) take 401s.
	CredentialExpired CredentialState = 2
	// CredentialMissing means there is no access token on disk at all.
	CredentialMissing CredentialState = 3
)

// String renders the state for logs and the decision trail.
func (s CredentialState) String() string {
	switch s {
	case CredentialFresh:
		return "fresh"
	case CredentialExpiring:
		return "expiring"
	case CredentialExpired:
		return "expired"
	case CredentialMissing:
		return "missing"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// Stale reports whether the state warrants a supervisor.credential.stale alert.
// Everything that is not Fresh is stale: a missing token is as bad as an
// expired one for the mesh.
func (s CredentialState) Stale() bool { return s != CredentialFresh }

// ExitCode maps the state to the CLI exit-code contract (0/1/2/3).
func (s CredentialState) ExitCode() int { return int(s) }

// CredentialFreshness is the full result of a freshness check.
type CredentialFreshness struct {
	// State is the classification used for alerting decisions.
	State CredentialState
	// ExpiresAt is the access-token expiry; zero when the file carried none
	// (or no token is present).
	ExpiresAt time.Time
	// HasRefreshToken reports whether a refresh token is on disk. A stale
	// credential WITH a refresh token can be auto-recovered by
	// token-refresher; WITHOUT one, only a human /login can fix it — so this
	// rides along on the alert to sharpen the operator's response.
	HasRefreshToken bool
	// StaleWindow is the window used for the check, echoed for the trail.
	StaleWindow time.Duration
}

// CheckCredentialFreshness performs the no-network freshness check the Overseer
// runs each tick. It reads the on-disk OAuth credential at credPath and
// classifies it relative to staleWithin, WITHOUT any network call or mutation —
// so it observes the true signal even when the token is already dead (no manual
// /login needed to see the alert).
//
// now defaults to time.Now and staleWithin to DefaultCredentialStaleWindow;
// both are injectable so the logic is unit-testable with a fixed clock. An
// empty credPath falls back to the resolver default (~/.claude/.credentials.json).
func CheckCredentialFreshness(credPath string, now func() time.Time, staleWithin time.Duration) CredentialFreshness {
	if now == nil {
		now = time.Now
	}
	if staleWithin <= 0 {
		staleWithin = DefaultCredentialStaleWindow
	}

	// Reuse the canonical resolver's read-only Status(): it is the same
	// no-network path token-refresher -check uses. Setting ExpirySkew to the
	// stale window makes Status().Fresh mean "valid AND not expiring within
	// the window".
	r := auth.OAuthResolver{
		CredentialsPath: credPath,
		Now:             now,
		ExpirySkew:      staleWithin,
	}
	st := r.Status()

	res := CredentialFreshness{
		ExpiresAt:       st.ExpiresAt,
		HasRefreshToken: st.HasRefreshToken,
		StaleWindow:     staleWithin,
	}

	switch {
	case !st.HasToken:
		res.State = CredentialMissing
	case st.Fresh:
		res.State = CredentialFresh
	case !st.ExpiresAt.IsZero() && !now().Before(st.ExpiresAt):
		// Not fresh and expiry is at or before now → already expired.
		res.State = CredentialExpired
	default:
		// Not fresh but not yet past expiry → expiring within the window.
		res.State = CredentialExpiring
	}
	return res
}

// Note renders a human-readable trail note describing the freshness outcome.
func (f CredentialFreshness) Note() string {
	switch f.State {
	case CredentialMissing:
		return "no access token on disk — mesh cannot authenticate; /login required"
	case CredentialExpired:
		return fmt.Sprintf("access token expired at %s — mesh is taking 401s; %s",
			f.expiryStr(), f.recoveryHint())
	case CredentialExpiring:
		return fmt.Sprintf("access token expires at %s (within %s) — refresh before the mesh 401s; %s",
			f.expiryStr(), f.StaleWindow, f.recoveryHint())
	default:
		return fmt.Sprintf("access token fresh; expires at %s", f.expiryStr())
	}
}

func (f CredentialFreshness) recoveryHint() string {
	if f.HasRefreshToken {
		return "refresh token present (token-refresher can auto-recover)"
	}
	return "no refresh token (human /login required)"
}

func (f CredentialFreshness) expiryStr() string {
	if f.ExpiresAt.IsZero() {
		return "unknown"
	}
	return f.ExpiresAt.UTC().Format(time.RFC3339)
}

// TrailRecord builds the decision-trail record for this outcome. Callers only
// append it when State.Stale() is true.
func (f CredentialFreshness) TrailRecord() decisiontrail.Record {
	payload := map[string]any{
		"state":             f.State.String(),
		"has_refresh_token": f.HasRefreshToken,
		"stale_window":      f.StaleWindow.String(),
		"note":              f.Note(),
	}
	if !f.ExpiresAt.IsZero() {
		payload["expires_at"] = f.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return decisiontrail.Record{
		Role:    string(RoleOverseer),
		Kind:    CredentialStaleKind,
		Payload: payload,
	}
}

// EmitCredentialStale appends a supervisor.credential.stale record to the trail
// when (and only when) the credential is stale. It returns the checked
// freshness so callers can also act on it (set an exit code, escalate, etc.).
//
// Like every Overseer tick step, the trail write is best-effort from the
// caller's perspective: the returned error is the Append result, but a tick
// records-and-continues rather than aborting on a trail failure.
func EmitCredentialStale(ctx context.Context, trail decisiontrail.Trail, f CredentialFreshness) error {
	if !f.State.Stale() {
		return nil
	}
	if trail == nil {
		return nil
	}
	return trail.Append(ctx, f.TrailRecord())
}

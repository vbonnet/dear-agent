package ops

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/pkg/override"
)

var authorizeOverride = override.Authorize

// AuthorizeCodexHookTrust runs the shared dangerous-override gates for trusting
// the exact attested Codex hooks without interactive per-path review and
// records the use.
//
// Every path that can request this launch policy must call this, including
// resume. Resume replays a policy persisted in the manifest, so skipping it
// there would turn "approve once, resume forever" into the loophole the gates
// exist to close.
func AuthorizeCodexHookTrust(reason, sessionName string) error {
	if _, err := authorizeOverride(override.Request{
		Kind:    override.KindCodexHookTrust,
		Reason:  reason,
		Actor:   OverrideActor(),
		Session: sessionName,
	}); err != nil {
		return fmt.Errorf("codex hook-trust override refused: %w", err)
	}
	return nil
}

// AuthorizeAdmissionBrakeOverride runs the shared dangerous-override gates for
// crossing an engaged admission brake and records the use.
//
// The brake exists because a watchdog or an operator decided the host is not
// fit to spawn. Crossing it is therefore always a claim a human should be able
// to read back later, which is exactly what the ledger preserves.
func AuthorizeAdmissionBrakeOverride(reason, sessionName string) error {
	if _, err := authorizeOverride(override.Request{
		Kind:    override.KindAdmissionBrake,
		Reason:  reason,
		Actor:   OverrideActor(),
		Session: sessionName,
	}); err != nil {
		return fmt.Errorf("admission-brake override refused: %w", err)
	}
	return nil
}

// ValidateAdmissionBrakeOverrideReason rejects malformed or boilerplate
// requests before admission probes run. This does not consume an approval or
// write the ledger; AuthorizeAdmissionBrakeOverride must still be called at the
// point where a live brake is actually crossed.
func ValidateAdmissionBrakeOverrideReason(reason string) (string, error) {
	normalized, err := override.ValidateReason(reason)
	if err != nil {
		return "", fmt.Errorf("admission-brake override refused: %w", err)
	}
	return normalized, nil
}

// AdmissionBrakeRemediation is the operator-facing next step for a refused
// brake override.
const AdmissionBrakeRemediation = "  • Approve interactively: agm override approve admission-brake --ttl 30m\n" +
	"  • State why: --brake-override=\"<reason>\"\n" +
	"  • Prefer fixing the condition the watchdog saw; review use with: agm override audit"

// OverrideActor names who is requesting an override. AGM_ACTOR lets a
// dispatcher identify itself so the ledger distinguishes automated use from a
// human at a terminal — the distinction the audit gate cares about most.
func OverrideActor() string {
	if actor := os.Getenv("AGM_ACTOR"); actor != "" {
		return actor
	}
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return "unknown"
}

// CodexHookTrustRemediation is the operator-facing next step shared by every
// surface that refuses the override.
const CodexHookTrustRemediation = "  • Approve interactively: agm override approve codex-hook-trust --ttl 2h\n" +
	"  • State why in the request: sandbox.bypass_codex_hook_trust_reason, or --dangerously-bypass-hook-trust=\"<reason>\"\n" +
	"  • Review recent use: agm override audit"

package ops

import (
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/override"
)

var authorizeOverride = override.Authorize

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
	return override.Actor()
}

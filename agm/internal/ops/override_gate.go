package ops

import (
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/override"
)

// ReserveAdmissionBrakeOverride validates human authorization without
// recording a use. The caller commits the reservation only after its final
// live admission check proves that the brake remains the sole refusal, and may
// combine it atomically with other launch-bound override reservations.
func ReserveAdmissionBrakeOverride(reason, sessionName string) (*override.Reservation, error) {
	reservation, err := override.Reserve(override.Request{
		Kind:    override.KindAdmissionBrake,
		Reason:  reason,
		Actor:   OverrideActor(),
		Session: sessionName,
	})
	if err != nil {
		return nil, fmt.Errorf("admission-brake override refused: %w", err)
	}
	return reservation, nil
}

// ValidateAdmissionBrakeOverrideReason rejects malformed or boilerplate
// requests before admission probes run. This does not consume an approval or
// write the ledger; ReserveAdmissionBrakeOverride must still be committed at
// the point where a final live check proves the brake is actually crossed.
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

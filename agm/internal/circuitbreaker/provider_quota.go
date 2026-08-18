package circuitbreaker

import (
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

// ProviderQuotaGate answers whether a provider has room for another
// session. It is satisfied by *quota.SpawnGate in production; tests
// inject a deterministic verdict.
type ProviderQuotaGate interface {
	AllowSpawn(model string) quota.SpawnDecision
}

// providerQuotaGateName is the gate identifier reported in GateResult.
const providerQuotaGateName = "provider_quota"

// checkProviderQuota refuses a spawn when the model's provider has spent
// its subscription quota, or is burning it faster than the window can
// refill.
//
// This gate is the odd one out in this package, and deliberately so.
// Every other gate here FAILS CLOSED — an unreadable disk or process
// table refuses the spawn, because those guard against the resource fill
// that took the host down. This one FAILS OPEN. The asymmetry is the
// point: a missing quota reading is not evidence that a budget is spent,
// and halting the whole fleet because a meter is uninstalled, signed out,
// or merely stale would be a far worse failure than one spawn too many.
// It refuses only on a fresh reading that positively says the provider is
// out of room.
func checkProviderQuota(gate ProviderQuotaGate, model string) GateResult {
	decision := gate.AllowSpawn(model)
	if decision.Allowed {
		msg := decision.Reason
		if !decision.Evaluated {
			// Distinguish "checked, has room" from "could not check". Both
			// pass, and only the log tells an operator which guardrail they
			// actually have.
			msg = "not evaluated (allowed): " + msg
		}
		return GateResult{
			Gate:    providerQuotaGateName,
			Passed:  true,
			Message: msg,
		}
	}

	msg := fmt.Sprintf("%s quota exhausted: %s.", decision.Family, decision.Reason)
	if !decision.ResetsAt.IsZero() {
		msg += fmt.Sprintf(" The window resets at %s.", decision.ResetsAt.Format(time.RFC3339))
	}
	msg += fmt.Sprintf(" Start this session on a provider with headroom (`agm quota` shows all of them),"+
		" or override with %s=off.", quota.SpawnGateOverrideEnvVar)

	return GateResult{
		Gate:    providerQuotaGateName,
		Passed:  false,
		Message: msg,
	}
}

// WithProviderQuota enables the provider-quota gate for a spawn of model.
// An empty model, or a nil gate, leaves the gate off entirely.
func WithProviderQuota(gate ProviderQuotaGate, model string) CheckOption {
	return func(o *checkOptions) {
		if gate == nil || model == "" {
			return
		}
		o.quotaGate = gate
		o.quotaModel = model
	}
}

// DefaultProviderQuotaGate reads the published quota state file. It performs
// no network I/O and no subprocess launch, so it is safe on the spawn path:
// refreshing the reading is a scheduled job's work, not a spawning process's.
//
// familyForModel maps the spawn's model identifier to a provider family, and
// wiring it is what makes the gate work at all. AGM spawns name their model by
// tier alias — "sonnet", "5.5", "3.5-flash", "sonnet-200k" — and an alias only
// resolves to a provider against its harness's catalog. Passing nil falls back
// to the quota package's vendor-substring heuristic, which matches none of
// those aliases and so allows every such spawn as unmetered: the default
// launch of every first-party provider. Callers own this resolver rather than
// the gate constructing it because the harness catalog lives in a package that
// depends on this one.
func DefaultProviderQuotaGate(familyForModel func(model string) string) ProviderQuotaGate {
	return &quota.SpawnGate{FamilyForModel: familyForModel}
}

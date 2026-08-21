package main

// Circuit-breaker admission for session creation.
//
// This file owns the one admission point every sanctioned spawn path goes
// through. It lives apart from new.go so the gates, the admission-brake
// override handshake, and the worker-counting helper read as the single
// concern they are, rather than as a tail on the command wiring.

import (
	"fmt"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/circuitbreaker"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/pkg/override"
)

// enforceCircuitBreakers runs an initial circuit-breaker check and returns an
// error if the request cannot proceed toward launch. On success it returns
// callbacks that repeat the live gates and consume any admission-brake
// override, then record the spawn only after every override has been finalized.
//
// It is the single admission point for every sanctioned spawn path: `agm
// session new` (and its current-tmux variant) and `agm supervisor run`.
// vroom-dispatch shells out to `agm session new`, so it inherits the same
// gates. Adding a spawn path without calling this is the ce-93lw.18 bug.
//
// harness and model name the harness the session will run under and the
// model it will run, so the provider-quota gate can tell which subscription
// budget this spawn would draw down. Both are needed: AGM spawns name their
// model by tier alias ("sonnet", "5.5", "3.5-flash"), and an alias only
// resolves to a provider against its harness's catalog. An empty model leaves
// that gate off — it has no budget to check.
type circuitBreakerAdmission struct {
	beforeSpawn        func(...*override.Reservation) ([]*override.Reservation, error)
	afterAuthorization func()
}

func enforceCircuitBreakers(sessionName, harness, model string) (*circuitBreakerAdmission, error) {
	cfg := circuitbreaker.DefaultConfig()
	lr := circuitbreaker.DefaultLoadReader()
	// The worker cap defaults to disabled. Do not open session storage merely to
	// enrich a best-effort status message in that mode; prefix counting remains
	// sufficient until an operator enables the cap.
	wc := circuitbreaker.TmuxWorkerCounter{}
	if cfg.MaxWorkers > 0 {
		wc.KnownWorkers = taggedWorkerSessions
	}
	st := circuitbreaker.NewFileSpawnTimer()
	mr := circuitbreaker.DefaultMemReader()
	dr := circuitbreaker.DefaultDiskReader()
	pc := circuitbreaker.DefaultProcCounter()
	br := circuitbreaker.DefaultBrakeReader()

	checkOpts := []circuitbreaker.CheckOption{
		circuitbreaker.WithDiskReader(dr),
		circuitbreaker.WithProcCounter(pc),
		circuitbreaker.WithBrakeReader(br),
		circuitbreaker.WithProviderQuota(circuitbreaker.DefaultProviderQuotaGate(
			func(m string) string { return agent.ModelFamilyForHarnessModel(harness, m) }), model),
	}
	normalizedBrakeOverrideReason := ""
	if brakeOverrideReason != "" {
		var err error
		normalizedBrakeOverrideReason, err = ops.ValidateAdmissionBrakeOverrideReason(brakeOverrideReason)
		if err != nil {
			ui.PrintError(err, "Admission-brake override refused", ops.AdmissionBrakeRemediation)
			return nil, err
		}
	}

	result := circuitbreaker.Check(cfg, lr, wc, st, mr, checkOpts...)
	logCircuitBreakerResult(result)

	if !result.Allowed && (normalizedBrakeOverrideReason == "" || !onlyAdmissionBrakeRefused(result)) {
		return nil, fmt.Errorf("%s", circuitbreaker.FormatDenied(result))
	}

	// Preflight proves that the request can reach launch, but does not consume
	// an override or record a spawn. The returned one-shot callback repeats the
	// live gates and crosses those boundaries only after every routine launch
	// preparation step has succeeded.
	var (
		mu       sync.Mutex
		consumed bool
	)
	admission := &circuitBreakerAdmission{}
	admission.beforeSpawn = func(additionalReservations ...*override.Reservation) ([]*override.Reservation, error) {
		mu.Lock()
		if consumed {
			mu.Unlock()
			return nil, fmt.Errorf("circuit-breaker launch admission was already consumed for %q", sessionName)
		}
		consumed = true
		mu.Unlock()

		liveResult := circuitbreaker.Check(cfg, lr, wc, st, mr, checkOpts...)
		liveResult, reservations, err := finalizeAdmissionBrakeOverride(
			liveResult,
			normalizedBrakeOverrideReason,
			sessionName,
			func() circuitbreaker.CheckResult {
				return circuitbreaker.Check(cfg, lr, wc, st, mr, checkOpts...)
			},
			ops.ReserveAdmissionBrakeOverride,
			additionalReservations...,
		)
		if err != nil {
			ui.PrintError(err, "Admission-brake override refused", ops.AdmissionBrakeRemediation)
			return nil, err
		}
		logCircuitBreakerResult(liveResult)
		if !liveResult.Allowed {
			return nil, fmt.Errorf("%s", circuitbreaker.FormatDenied(liveResult))
		}
		return reservations, nil
	}
	admission.afterAuthorization = func() {
		if err := st.RecordSpawn(time.Now()); err != nil {
			debug.Log("Warning: failed to record spawn time: %v", err)
		}
		// Recorded exactly once, here, after every live gate has passed
		// and the spawn is truly proceeding — not inside the quota gate
		// itself, which runs at both preflight and live confirmation and
		// would double-count a single spawn (ce-93lw.18 class bug: this
		// is the one admission point every sanctioned spawn path goes
		// through). An empty model leaves the quota gate off entirely, so
		// there is nothing to record.
		if model != "" {
			family := agent.ModelFamilyForHarnessModel(harness, model)
			if family != "" {
				if err := circuitbreaker.RecordProviderQuotaAdmission(family, time.Now()); err != nil {
					debug.Log("Warning: failed to record provider-quota admission: %v", err)
				}
			}
		}
	}
	return admission, nil
}

func finalizeAdmissionBrakeOverride(
	initial circuitbreaker.CheckResult,
	reason string,
	sessionName string,
	finalCheck func() circuitbreaker.CheckResult,
	reserve func(string, string) (*override.Reservation, error),
	additionalReservations ...*override.Reservation,
) (circuitbreaker.CheckResult, []*override.Reservation, error) {
	if !onlyAdmissionBrakeRefused(initial) {
		if initial.Allowed {
			return initial, additionalReservations, nil
		}
		return initial, nil, nil
	}
	if reason == "" {
		return initial, nil, nil
	}

	// Reserve current human authorization without consuming the ledger quota,
	// then repeat every live gate. A concurrent resource or stagger refusal
	// abandons the reservation without recording a use.
	brakeReservation, err := reserve(reason, sessionName)
	if err != nil {
		return initial, nil, err
	}
	result := finalCheck()
	if !onlyAdmissionBrakeRefused(result) {
		// The brake cleared, or another gate began refusing. Neither outcome
		// crossed the brake, so the reserved use must not be committed.
		if result.Allowed {
			return result, additionalReservations, nil
		}
		return result, nil, nil
	}
	reservations := append([]*override.Reservation{brakeReservation}, additionalReservations...)
	return applyAdmissionBrakeAuthorization(result, reason), reservations, nil
}

func applyAdmissionBrakeAuthorization(
	result circuitbreaker.CheckResult,
	reason string,
) circuitbreaker.CheckResult {
	result.Allowed = true
	for i := range result.Gates {
		gate := &result.Gates[i]
		if gate.Gate == "admission_brake" && !gate.Passed && gate.RequiresOverride {
			gate.Passed = true
			gate.RequiresOverride = false
			gate.Message = fmt.Sprintf("%s Crossed under an audited override: %s", gate.Message, reason)
		}
		if !gate.Passed {
			result.Allowed = false
		}
	}
	return result
}

func logCircuitBreakerResult(result circuitbreaker.CheckResult) {
	debug.Log("Circuit breaker check: level=%s load=%.1f allowed=%v", result.Level, result.Load, result.Allowed)
	for _, g := range result.Gates {
		debug.Log("  gate %s: passed=%v — %s", g.Gate, g.Passed, g.Message)
	}
}

func onlyAdmissionBrakeRefused(result circuitbreaker.CheckResult) bool {
	return circuitbreaker.RequiresAdmissionBrakeOverride(result)
}

// taggedWorkerSessions returns the tmux session names of non-archived sessions
// AGM records as tagged role:worker. The circuit breaker uses it to recognise
// workers whose tmux session name lacks the worker- prefix, so they still count
// against the cap. It is consulted only when such a session is live, and every
// failure path returns an error so the breaker falls back to prefix-only
// classification rather than blocking a spawn on an unreadable database.
//
// A session's record name and its tmux session name can diverge (resume and
// import both produce that state), and the breaker matches against what tmux
// reports. Both names are therefore registered, mirroring how
// ops.findOrphanTmuxSessions decides a tmux session belongs to AGM.
//
// This reads the manifests directly rather than going through ops.ListSessions:
// SessionSummary does not carry Tmux.SessionName, and the status computation
// ListSessions performs would re-enumerate tmux, which the caller has already
// done. Stopped sessions need no filtering here — the breaker only counts names
// that are live in tmux, so a record without a session cannot inflate the count.
func taggedWorkerSessions() (map[string]bool, error) {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return nil, fmt.Errorf("opening session storage: %w", err)
	}
	defer cleanup()

	manifests, err := opCtx.Storage.ListSessions(&dolt.SessionFilter{
		Tags:            []string{"role:worker"},
		ExcludeArchived: true,
	})
	if err != nil {
		return nil, fmt.Errorf("listing worker sessions: %w", err)
	}

	workers := make(map[string]bool, len(manifests)*2)
	for _, m := range manifests {
		if m.Tmux.SessionName != "" {
			workers[m.Tmux.SessionName] = true
		}
		if m.Name != "" {
			workers[m.Name] = true
		}
	}
	return workers, nil
}

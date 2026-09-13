package recoveryloop

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
)

// PulseFact is the current, clearable truth about one pulse.
//
// The distinction between Known and Alarming is the whole point. The recovery
// loop previously read only the append-only alarm journal, which records
// absences and nothing else: a pulse that recovered produced no record, so
// "alarming" was monotonic and no pulse could ever clear. Reading the
// absence-alarm heartbeat instead gives both polarities, which is what makes
// recovery verifiable at all.
type PulseFact struct {
	// Known reports whether any evidence source observed this pulse. An
	// unknown pulse is never proof of health (the absence-alarm AA-05 rule:
	// "could not check" is not health).
	Known bool
	// Status is the status the evidence source reported, verbatim.
	//
	// It is stored rather than pre-reduced to a boolean because presence and
	// absence are not complements. "snoozed" is neither: it suppresses an
	// alarm without observing anything. Collapsing the status into a single
	// "alarming" flag and reading presence as its negation is how a suppressed
	// alarm, or any status a future absence-alarm emits that this binary does
	// not recognise, would become positive proof of life.
	Status absencealarm.Status
	// Since is when the current alarm began, zero when not alarming.
	Since time.Time
	// Evidence is when the probe observed this pulse. It is what makes a
	// present reading usable as post-action proof: a pulse seen before a
	// remediation ran says nothing about whether the remediation worked.
	Evidence time.Time
}

// PulseTruth maps a pulse name to its current fact.
type PulseTruth map[string]PulseFact

// Alarming reports whether the named pulse is currently alarming.
func (pt PulseTruth) Alarming(name string) bool {
	return pt[name].Status.Alarming()
}

// Present reports whether the named pulse was positively observed alive.
//
// Only an explicit "present" qualifies. An unobserved pulse is not present
// (absence of evidence is not evidence), and neither is a snoozed or
// unrecognised one: the question a verification asks is "was this pulse seen",
// and only one status answers yes.
func (pt PulseTruth) Present(name string) bool {
	f := pt[name]
	return f.Known && f.Status == absencealarm.StatusPresent
}

// AbsentFor reports how long the named pulse has been alarming. It returns 0
// when the pulse is healthy or when the alarm start time is unknown.
func (pt PulseTruth) AbsentFor(name string, now time.Time) time.Duration {
	f := pt[name]
	if !f.Status.Alarming() || f.Since.IsZero() || !now.After(f.Since) {
		return 0
	}
	return now.Sub(f.Since)
}

// heartbeatSkewTolerance is how far ahead of local time a heartbeat may be
// dated before it is refused. It absorbs ordinary clock jitter between the
// writer and this reader without accepting a heartbeat from a clock that
// genuinely jumped.
const heartbeatSkewTolerance = 2 * time.Minute

// absenceHeartbeat mirrors the fields of absencealarm.Heartbeat that the
// recovery loop consumes.
type absenceHeartbeat struct {
	TickTime time.Time             `json:"tick_time"`
	Results  []absencealarm.Result `json:"results"`
}

// LoadPulseTruth reads the current pulse truth from the absence-alarm
// heartbeat, dating each alarm from the absence-alarm alarm state.
//
// The heartbeat is the only source that reports presence as well as absence,
// and it is rewritten atomically every tick, so a stale heartbeat means the
// monitor itself stopped. A heartbeat older than maxAge yields no facts at
// all rather than stale ones: verification then fails loudly instead of
// silently confirming a recovery against evidence from days ago.
func LoadPulseTruth(heartbeatPath, alarmStatePath string, now time.Time, maxAge time.Duration) (PulseTruth, error) {
	truth := PulseTruth{}

	raw, err := os.ReadFile(heartbeatPath)
	if err != nil {
		if os.IsNotExist(err) {
			return truth, nil
		}
		return truth, fmt.Errorf("read absence heartbeat %s: %w", heartbeatPath, err)
	}
	var hb absenceHeartbeat
	if err := json.Unmarshal(raw, &hb); err != nil {
		return truth, fmt.Errorf("parse absence heartbeat %s: %w", heartbeatPath, err)
	}
	if maxAge > 0 {
		// An undated heartbeat is evidence of unknown age, which is exactly
		// what the age check exists to reject. Treating a missing or
		// unparseable tick_time as "fresh" would let a truncated or corrupt
		// file confirm recoveries forever (RL-32).
		if hb.TickTime.IsZero() {
			return PulseTruth{}, fmt.Errorf("absence heartbeat %s has no tick_time: pulse truth unavailable", heartbeatPath)
		}
		if hb.TickTime.After(now.Add(heartbeatSkewTolerance)) {
			// A negative age passes any "older than maxAge" test, so a clock
			// that jumped forward would let this heartbeat's last present
			// readings suppress remediation until wall time caught up.
			return PulseTruth{}, fmt.Errorf("absence heartbeat %s is dated %s in the future: pulse truth unavailable",
				heartbeatPath, hb.TickTime.Sub(now).Round(time.Second))
		}
		if now.Sub(hb.TickTime) > maxAge {
			return PulseTruth{}, fmt.Errorf("absence heartbeat %s is %s old (max %s): pulse truth unavailable",
				heartbeatPath, now.Sub(hb.TickTime).Round(time.Second), maxAge)
		}
	}

	for _, res := range hb.Results {
		if res.Name == "" {
			continue
		}
		truth[res.Name] = PulseFact{Known: true, Status: res.Status, Evidence: res.Evidence}
	}

	// Date each standing alarm from the absence-alarm dedup state, which
	// records when the alarm began. Failure here costs only the duration in
	// the escalation text, never the alarming/present decision.
	st, stErr := absencealarm.LoadAlarmState(alarmStatePath)
	if stErr != nil {
		return truth, fmt.Errorf("read absence alarm state %s: %w", alarmStatePath, stErr)
	}
	for name, alarm := range st.Pulses {
		f, ok := truth[name]
		if !ok {
			continue
		}
		f.Since = alarm.Since
		truth[name] = f
	}
	return truth, nil
}

// VerifyOutcome is the result of re-checking a job's condition after a
// remediation action ran.
type VerifyOutcome struct {
	// Verified is true only when the condition that triggered the action is
	// observed to have cleared.
	Verified bool
	Status   RecoveryStatus
	Reason   string
}

// VerifyRecovery re-checks a job's condition after a remediation action and
// classifies the true outcome (RL-26..RL-29).
//
// It never trusts that the action's command exited zero. `launchctl kickstart`
// returns success as soon as launchd accepts the request, which says nothing
// about whether the job then produced its pulse; that gap is exactly how the
// loop reported "recovered" on hundreds of consecutive ticks while the pulse
// it claimed to restore stayed absent for days.
//
// Structural conditions (binary on disk, launchd job loaded) are observable at
// once, so a structural condition that survived the action is a definite
// failure. A pulse cannot be observed instantly: the job has to run and write
// it. A cleared structure with an un-cleared pulse is therefore PENDING, and
// the caller decides on a later tick, once the pulse's grace period has
// elapsed, whether it became a recovery or a failure.
func VerifyRecovery(
	job Job,
	action ActionType,
	truth PulseTruth,
	launchdJobs map[string]LaunchdJobInfo,
	host HostOps,
	now time.Time,
	actionAt time.Time,
) VerifyOutcome {
	// Structural re-probe: these clear immediately or not at all.
	if job.BinaryPath != "" && !host.FileExists(job.BinaryPath) {
		return VerifyOutcome{
			Status: StatusFailed,
			Reason: fmt.Sprintf("after %s, binary %s still does not exist", action, job.BinaryPath),
		}
	}
	if job.LaunchdLabel != "" {
		info, loaded := launchdJobs[job.LaunchdLabel]
		if !loaded {
			return VerifyOutcome{
				Status: StatusFailed,
				Reason: fmt.Sprintf("after %s, launchd job %s is still not loaded", action, job.LaunchdLabel),
			}
		}
		if info.Status == 78 || info.Status == -9 {
			return VerifyOutcome{
				Status: StatusFailed,
				Reason: fmt.Sprintf("after %s, launchd job %s still reports status %d", action, job.LaunchdLabel, info.Status),
			}
		}
	}

	// Pulse re-probe: the positive event is the only proof the job is working.
	if job.Pulse != "" {
		switch {
		case truth.Present(job.Pulse):
			// The pulse is present, but presence observed BEFORE the action
			// cannot be proof that the action worked. Pulse truth is read once
			// per tick, so the reading in hand usually predates the
			// remediation; the answer arrives on a later tick, from a later
			// heartbeat. Without this, a job unloaded minutes ago still has a
			// fresh file-mtime pulse, and a bootstrap that succeeds
			// structurally while the job never runs would be scored a
			// verified recovery (RL-39).
			if ev := truth[job.Pulse].Evidence; !actionAt.IsZero() && !ev.After(actionAt) {
				return VerifyOutcome{
					Status: StatusPending,
					Reason: fmt.Sprintf("pulse %q was last observed %s, before %s ran: awaiting fresh evidence",
						job.Pulse, evidenceStamp(ev), action),
				}
			}
			return VerifyOutcome{
				Verified: true,
				Status:   StatusRecovered,
				Reason:   fmt.Sprintf("verified: pulse %q is present after %s", job.Pulse, action),
			}
		case truth.Alarming(job.Pulse):
			since := truth.AbsentFor(job.Pulse, now)
			reason := fmt.Sprintf("pulse %q has not returned after %s", job.Pulse, action)
			if since > 0 {
				reason += fmt.Sprintf(" (absent for %s)", since.Round(time.Minute))
			}
			return VerifyOutcome{Status: StatusPending, Reason: reason}
		default:
			return VerifyOutcome{
				Status: StatusPending,
				Reason: fmt.Sprintf("pulse %q was not observed after %s: recovery unverified", job.Pulse, action),
			}
		}
	}

	// No pulse configured: the structural checks above are all the evidence
	// that exists, and they passed.
	return VerifyOutcome{
		Verified: true,
		Status:   StatusRecovered,
		Reason:   fmt.Sprintf("verified: structural checks pass after %s", action),
	}
}

// evidenceStamp renders a probe observation time for an operator-facing reason.
func evidenceStamp(t time.Time) string {
	if t.IsZero() {
		return "at an unrecorded time"
	}
	return "at " + t.Format(time.RFC3339)
}

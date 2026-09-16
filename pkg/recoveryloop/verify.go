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

// AbsenceAlarmHeartbeatPulse is the self-heartbeat emitted by the process
// that owns the pulse-truth file. Missing or stale source data may justify
// remediating this one owner, while every downstream pulse remains unknown.
const AbsenceAlarmHeartbeatPulse = "absence-alarm-heartbeat"

// PulseSourceStatus describes the independently observed availability of the
// pulse-truth source itself. Its zero value is unavailable and therefore
// fail-closed.
type PulseSourceStatus uint8

const (
	// PulseSourceUnavailable means the source could not be classified safely,
	// for example because it was malformed, unreadable, or future-dated.
	PulseSourceUnavailable PulseSourceStatus = iota
	// PulseSourceFresh means the source heartbeat is recent enough to use.
	PulseSourceFresh
	// PulseSourceMissing means no source heartbeat has been written.
	PulseSourceMissing
	// PulseSourceStale means the source heartbeat exceeded its freshness limit.
	PulseSourceStale
)

// RequiresRecovery reports whether the source owner's own liveness is known
// to have failed, as distinct from an invalid observation that permits no
// action.
func (s PulseSourceStatus) RequiresRecovery() bool {
	return s == PulseSourceMissing || s == PulseSourceStale
}

func (s PulseSourceStatus) String() string {
	switch s {
	case PulseSourceFresh:
		return "fresh"
	case PulseSourceMissing:
		return "missing"
	case PulseSourceStale:
		return "stale"
	case PulseSourceUnavailable:
		return "unavailable"
	}
	return "unavailable"
}

// EvidenceTiming classifies whether a pulse observation may prove a state
// transition after a boundary. Keeping the classification here gives every
// recovery path one clock-skew policy instead of letting command adapters
// grow subtly different timestamp checks.
type EvidenceTiming uint8

const (
	// EvidenceMissing has no observation timestamp, so it cannot establish
	// ordering even when the persisted transition boundary is also absent.
	EvidenceMissing EvidenceTiming = iota
	// EvidenceAdmissible is not materially future-dated and, when a boundary
	// is supplied, was observed strictly after it.
	EvidenceAdmissible
	// EvidenceNotAfterBoundary was observed at or before the transition it is
	// being offered to prove.
	EvidenceNotAfterBoundary
	// EvidenceTooFarInFuture is beyond the recovery loop's tolerated clock
	// skew and cannot prove anything about current host state.
	EvidenceTooFarInFuture
)

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

// ClassifyEvidence applies the recovery loop's temporal proof boundary to one
// pulse observation. A zero boundary asks only whether the evidence is
// plausibly current; a non-zero boundary additionally requires a strictly
// newer observation.
func (pt PulseTruth) ClassifyEvidence(name string, boundary, now time.Time) EvidenceTiming {
	ev := pt[name].Evidence
	if ev.IsZero() {
		return EvidenceMissing
	}
	if ev.After(now.Add(heartbeatSkewTolerance)) {
		return EvidenceTooFarInFuture
	}
	if !boundary.IsZero() && !ev.After(boundary) {
		return EvidenceNotAfterBoundary
	}
	return EvidenceAdmissible
}

// VerificationObservationAvailable reports whether the current pulse fact can
// settle or continue timing a recovery attempt. Explicit absence and present
// evidence with a usable timestamp are observations. Missing, undetermined,
// snoozed, or clock-invalid facts are unavailable and must fail the tick closed
// without being fabricated into proof that the job stayed broken.
func (pt PulseTruth) VerificationObservationAvailable(name string, boundary, now time.Time) bool {
	fact := pt[name]
	if !fact.Known {
		return false
	}
	switch fact.Status {
	case absencealarm.StatusAbsent:
		return true
	case absencealarm.StatusPresent:
		timing := pt.ClassifyEvidence(name, boundary, now)
		return timing == EvidenceAdmissible || timing == EvidenceNotAfterBoundary
	case absencealarm.StatusUndetermined, absencealarm.StatusSnoozed:
		return false
	}
	return false
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
func LoadPulseTruth(
	heartbeatPath, alarmStatePath string,
	now time.Time,
	maxAge time.Duration,
) (PulseTruth, PulseSourceStatus, error) {
	truth := PulseTruth{}

	raw, err := os.ReadFile(heartbeatPath)
	if err != nil {
		if os.IsNotExist(err) {
			return truth, PulseSourceMissing, nil
		}
		return truth, PulseSourceUnavailable, fmt.Errorf("read absence heartbeat %s: %w", heartbeatPath, err)
	}
	var hb absenceHeartbeat
	if err := json.Unmarshal(raw, &hb); err != nil {
		return truth, PulseSourceUnavailable, fmt.Errorf("parse absence heartbeat %s: %w", heartbeatPath, err)
	}
	if maxAge > 0 {
		// An undated heartbeat is evidence of unknown age, which is exactly
		// what the age check exists to reject. Treating a missing or
		// unparseable tick_time as "fresh" would let a truncated or corrupt
		// file confirm recoveries forever (RL-32).
		if hb.TickTime.IsZero() {
			return PulseTruth{}, PulseSourceUnavailable, fmt.Errorf("absence heartbeat %s has no tick_time: pulse truth unavailable", heartbeatPath)
		}
		if hb.TickTime.After(now.Add(heartbeatSkewTolerance)) {
			// A negative age passes any "older than maxAge" test, so a clock
			// that jumped forward would let this heartbeat's last present
			// readings suppress remediation until wall time caught up.
			return PulseTruth{}, PulseSourceUnavailable, fmt.Errorf("absence heartbeat %s is dated %s in the future: pulse truth unavailable",
				heartbeatPath, hb.TickTime.Sub(now).Round(time.Second))
		}
		if now.Sub(hb.TickTime) > maxAge {
			return PulseTruth{}, PulseSourceStale, fmt.Errorf("absence heartbeat %s is %s old (max %s): pulse truth unavailable",
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
		return truth, PulseSourceFresh, fmt.Errorf("read absence alarm state %s: %w", alarmStatePath, stErr)
	}
	for name, alarm := range st.Pulses {
		f, ok := truth[name]
		if !ok {
			continue
		}
		f.Since = alarm.Since
		truth[name] = f
	}
	return truth, PulseSourceFresh, nil
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
	if failure, bad := verifyStructure(job, action, launchdJobs, host); bad {
		return failure
	}

	// A structural pulse proves the service exists, which the structural
	// checks above have already established. It cannot answer whether the
	// work succeeded, so verification falls through to the exit-status check
	// exactly as PlanJob does (RL-43).
	if job.Pulse != "" && !job.PulseIsStructural {
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
			// Evidence dated in the future is a broken clock or a touched
			// file, not proof: it beats any actionAt automatically, which is
			// exactly the comparison this guard exists to make meaningful.
			ev := truth[job.Pulse].Evidence
			switch truth.ClassifyEvidence(job.Pulse, actionAt, now) {
			case EvidenceMissing:
				return VerifyOutcome{
					Status: StatusPending,
					Reason: fmt.Sprintf("pulse %q is present but carries no evidence timestamp: not usable as proof",
						job.Pulse),
				}
			case EvidenceTooFarInFuture:
				return VerifyOutcome{
					Status: StatusPending,
					Reason: fmt.Sprintf("pulse %q carries evidence dated %s in the future: not usable as proof",
						job.Pulse, ev.Sub(now).Round(time.Second)),
				}
			case EvidenceNotAfterBoundary:
				return VerifyOutcome{
					Status: StatusPending,
					Reason: fmt.Sprintf("pulse %q was last observed %s, before %s ran: awaiting fresh evidence",
						job.Pulse, evidenceStamp(ev), action),
				}
			case EvidenceAdmissible:
				// Continue to the verified outcome below.
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
	// that exists. They must include the condition that actually triggered the
	// action. PlanJob kickstarts a stopped job whose last run exited nonzero,
	// so "loaded, and not 78/-9" is a different question: a process that exits
	// 1 again immediately would verify as recovered and reset the counter, and
	// the next tick would repeat the same false recovery forever (RL-42).
	if job.LaunchdLabel != "" {
		if info, ok := launchdJobs[job.LaunchdLabel]; ok && info.PID == 0 && info.Status != 0 {
			return VerifyOutcome{
				Status: StatusFailed,
				Reason: fmt.Sprintf("after %s, launchd job %s is still not running and last exited %d",
					action, job.LaunchdLabel, info.Status),
			}
		}
	}
	return VerifyOutcome{
		Verified: true,
		Status:   StatusRecovered,
		Reason:   fmt.Sprintf("verified: structural checks pass after %s", action),
	}
}

// verifyStructure re-probes the conditions that are observable immediately.
// A structural condition that survived the action is a definite failure, so
// the second return reports whether the outcome is conclusive.
func verifyStructure(
	job Job,
	action ActionType,
	launchdJobs map[string]LaunchdJobInfo,
	host HostOps,
) (VerifyOutcome, bool) {
	if job.BinaryPath != "" && !host.FileExists(job.BinaryPath) {
		return VerifyOutcome{
			Status: StatusFailed,
			Reason: fmt.Sprintf("after %s, binary %s still does not exist", action, job.BinaryPath),
		}, true
	}
	if job.LaunchdLabel == "" {
		return VerifyOutcome{}, false
	}
	info, loaded := launchdJobs[job.LaunchdLabel]
	if !loaded {
		return VerifyOutcome{
			Status: StatusFailed,
			Reason: fmt.Sprintf("after %s, launchd job %s is still not loaded", action, job.LaunchdLabel),
		}, true
	}
	if info.Status == 78 || info.Status == -9 {
		return VerifyOutcome{
			Status: StatusFailed,
			Reason: fmt.Sprintf("after %s, launchd job %s still reports status %d", action, job.LaunchdLabel, info.Status),
		}, true
	}
	return VerifyOutcome{}, false
}

// evidenceStamp renders a probe observation time for an operator-facing reason.
func evidenceStamp(t time.Time) string {
	if t.IsZero() {
		return "at an unrecorded time"
	}
	return "at " + t.Format(time.RFC3339)
}

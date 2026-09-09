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
	// Alarming reports whether the pulse was absent or undetermined.
	Alarming bool
	// Since is when the current alarm began, zero when not alarming.
	Since time.Time
}

// PulseTruth maps a pulse name to its current fact.
type PulseTruth map[string]PulseFact

// Alarming reports whether the named pulse is currently alarming.
func (pt PulseTruth) Alarming(name string) bool {
	return pt[name].Alarming
}

// Present reports whether the named pulse was positively observed alive.
// An unobserved pulse is not present: absence of evidence is not evidence.
func (pt PulseTruth) Present(name string) bool {
	f := pt[name]
	return f.Known && !f.Alarming
}

// AbsentFor reports how long the named pulse has been alarming. It returns 0
// when the pulse is healthy or when the alarm start time is unknown.
func (pt PulseTruth) AbsentFor(name string, now time.Time) time.Duration {
	f := pt[name]
	if !f.Alarming || f.Since.IsZero() || !now.After(f.Since) {
		return 0
	}
	return now.Sub(f.Since)
}

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
	if maxAge > 0 && !hb.TickTime.IsZero() && now.Sub(hb.TickTime) > maxAge {
		return truth, fmt.Errorf("absence heartbeat %s is %s old (max %s): pulse truth unavailable",
			heartbeatPath, now.Sub(hb.TickTime).Round(time.Second), maxAge)
	}

	for _, res := range hb.Results {
		if res.Name == "" {
			continue
		}
		truth[res.Name] = PulseFact{Known: true, Alarming: res.Status.Alarming()}
	}

	// Date each standing alarm from the absence-alarm dedup state, which
	// records when the alarm began. Failure here costs only the duration in
	// the escalation text, never the alarming/present decision.
	st, stErr := absencealarm.LoadAlarmState(alarmStatePath)
	for name, alarm := range st.Pulses {
		f, ok := truth[name]
		if !ok {
			continue
		}
		f.Since = alarm.Since
		truth[name] = f
	}
	if stErr != nil {
		return truth, fmt.Errorf("read absence alarm state %s: %w", alarmStatePath, stErr)
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

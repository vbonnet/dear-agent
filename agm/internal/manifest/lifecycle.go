package manifest

import "fmt"

// SessionLifecycle is the closed durable lifecycle vocabulary for AGM session
// manifests. The empty value is the legacy representation for sessions that
// are neither reaping nor archived; Dolt stores that value as the "active"
// status.
type SessionLifecycle string

const (
	// LifecycleLegacy is the empty lifecycle retained for legacy active or
	// stopped sessions that predate explicit terminal lifecycle values.
	LifecycleLegacy = ""
	// LifecycleReaping marks a session whose resources are being torn down.
	LifecycleReaping = "reaping"
	// LifecycleArchived marks a session that has been archived.
	LifecycleArchived = "archived"
)

// Valid reports whether lifecycle is a supported durable lifecycle value.
func (lifecycle SessionLifecycle) Valid() bool {
	switch lifecycle {
	case LifecycleLegacy, LifecycleReaping, LifecycleArchived:
		return true
	default:
		return false
	}
}

// ParseSessionLifecycle accepts only supported lifecycle wire values. Empty is
// deliberately accepted as the legacy active/stopped representation.
func ParseSessionLifecycle(value string) (SessionLifecycle, error) {
	lifecycle := SessionLifecycle(value)
	if lifecycle.Valid() {
		return lifecycle, nil
	}
	return LifecycleLegacy, fmt.Errorf(
		"invalid session lifecycle %q (must be empty, %q, or %q)",
		value,
		LifecycleReaping,
		LifecycleArchived,
	)
}

// SessionOutcome describes how a session ended, stamped on the record at
// archive time.
type SessionOutcome string

const (
	// OutcomeCompleted marks a session that was archived after finishing work
	// normally (the archive command's default).
	OutcomeCompleted SessionOutcome = "completed"
	// OutcomeCrashed marks a session that was archived after an abnormal exit.
	OutcomeCrashed SessionOutcome = "crashed"
	// OutcomeKilled marks a session that was archived after being killed.
	OutcomeKilled SessionOutcome = "killed"
	// OutcomeGCStale marks a session archived by garbage collection as stale.
	OutcomeGCStale SessionOutcome = "gc-stale"
	// OutcomeUnknown is the empty legacy value for records written before
	// outcome stamping existed (or records that never received a stamp).
	OutcomeUnknown SessionOutcome = ""
)

// Valid reports whether outcome is a supported durable archive outcome value.
func (outcome SessionOutcome) Valid() bool {
	switch outcome {
	case OutcomeUnknown, OutcomeCompleted, OutcomeCrashed, OutcomeKilled, OutcomeGCStale:
		return true
	default:
		return false
	}
}

// ParseSessionOutcome accepts only supported outcome wire values. Empty is
// deliberately accepted as the legacy unknown-outcome representation.
func ParseSessionOutcome(value string) (SessionOutcome, error) {
	outcome := SessionOutcome(value)
	if outcome.Valid() {
		return outcome, nil
	}
	return OutcomeUnknown, fmt.Errorf(
		"invalid session outcome %q (must be empty, %q, %q, %q, or %q)",
		value,
		OutcomeCompleted,
		OutcomeCrashed,
		OutcomeKilled,
		OutcomeGCStale,
	)
}

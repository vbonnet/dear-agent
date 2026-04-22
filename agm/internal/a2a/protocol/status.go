package protocol

import (
	"fmt"
	"strings"
)

// Status represents the status of an A2A message
type Status string

const (
	StatusPending            Status = "pending"
	StatusAwaitingResponse   Status = "awaiting-response"
	StatusConsensusReached   Status = "consensus-reached"
	StatusEscalateToHuman    Status = "escalate-to-human"
	StatusBlockedPrefix      Status = "blocked-on-"
	StatusCoordinationNeeded Status = "coordination-needed"
	StatusHandoffComplete    Status = "handoff-complete"
)

// IsValid checks if a status value is valid
func (s Status) IsValid() bool {
	switch s {
	case StatusPending,
		StatusAwaitingResponse,
		StatusConsensusReached,
		StatusEscalateToHuman,
		StatusCoordinationNeeded,
		StatusHandoffComplete:
		return true
	case StatusBlockedPrefix: // prefix sentinel; actual blocked statuses checked below
		return false
	default:
		return strings.HasPrefix(string(s), string(StatusBlockedPrefix))
	}
}

// String returns the status as a string
func (s Status) String() string {
	return string(s)
}

// ValidateStatus validates a status string and returns a Status
func ValidateStatus(s string) (Status, error) {
	status := Status(s)
	if !status.IsValid() {
		return "", fmt.Errorf("invalid status: %s (valid: pending, awaiting-response, consensus-reached, escalate-to-human, blocked-on-{reason}, coordination-needed, handoff-complete)", s)
	}
	return status, nil
}

// IsBlocked returns true if status is blocked-on-{reason}
func (s Status) IsBlocked() bool {
	return strings.HasPrefix(string(s), string(StatusBlockedPrefix))
}

// BlockedReason returns the reason if status is blocked-on-{reason}
func (s Status) BlockedReason() string {
	if !s.IsBlocked() {
		return ""
	}
	return strings.TrimPrefix(string(s), string(StatusBlockedPrefix))
}

// NewBlockedStatus creates a new blocked-on-{reason} status
func NewBlockedStatus(reason string) Status {
	return Status(string(StatusBlockedPrefix) + reason)
}

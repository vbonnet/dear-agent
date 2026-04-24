package hippocampus

import "time"

// SignalType categorizes cross-session memory signals.
type SignalType string

const (
	SignalCorrection SignalType = "correction"
	SignalPreference SignalType = "preference"
	SignalDecision   SignalType = "decision"
	SignalLearning   SignalType = "learning"
	SignalFact       SignalType = "fact"
)

// Signal represents a memory consolidation signal captured from a session.
type Signal struct {
	Type       SignalType
	Content    string
	Source     string // session ID or file path
	Timestamp  time.Time
	Confidence float64 // 0.0-1.0
}

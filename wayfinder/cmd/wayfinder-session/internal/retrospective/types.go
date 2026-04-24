package retrospective

import "time"

// RewindEventData captures comprehensive rewind context for retrospectives
//
// This struct is marshaled to map[string]interface{} for history.Event.Data
// and formatted to markdown for S11-retrospective.md appending.
type RewindEventData struct {
	// FromPhase is the phase being rewound from (e.g., "S7")
	FromPhase string `json:"from_phase"`

	// ToPhase is the target phase being rewound to (e.g., "S5")
	ToPhase string `json:"to_phase"`

	// Magnitude is the number of phases between from and to (0 = no-op, 1+ = rewind)
	// Calculated as |phaseIndex(from) - phaseIndex(to)|
	Magnitude int `json:"magnitude"`

	// Timestamp when rewind occurred
	Timestamp time.Time `json:"timestamp"`

	// Prompted indicates if user was prompted for reason/learnings
	// true if magnitude >= 1 and --no-prompt not used
	Prompted bool `json:"prompted"`

	// Reason explains why the rewind occurred (user-provided or empty)
	Reason string `json:"reason,omitempty"`

	// Learnings captures what was learned that triggered the rewind (user-provided or empty)
	Learnings string `json:"learnings,omitempty"`

	// Context snapshot of project state at rewind time
	Context ContextSnapshot `json:"context"`
}

// ContextSnapshot captures project state at rewind time
type ContextSnapshot struct {
	// Git state (branch, commit, uncommitted changes)
	Git GitContext `json:"git"`

	// Deliverables from phases being rewound (summary list)
	Deliverables []string `json:"deliverables"`

	// Phase state at rewind time
	PhaseState PhaseContext `json:"phase_state"`
}

// GitContext captures git repository state
type GitContext struct {
	// Branch name (e.g., "main", "feature/rewind-logging")
	Branch string `json:"branch"`

	// Commit SHA (short form, e.g., "a1b2c3d")
	Commit string `json:"commit"`

	// UncommittedChanges indicates if there are uncommitted changes
	UncommittedChanges bool `json:"uncommitted_changes"`

	// Error if git context capture failed (e.g., timeout, not a git repo)
	Error string `json:"error,omitempty"`
}

// PhaseContext captures Wayfinder phase state
type PhaseContext struct {
	// CurrentPhase before rewind (e.g., "S7")
	CurrentPhase string `json:"current_phase"`

	// CompletedPhases list (e.g., ["W0", "D1", "D2", ...])
	CompletedPhases []string `json:"completed_phases"`

	// SessionID from WAYFINDER-STATUS.md
	SessionID string `json:"session_id"`
}

// RewindFlags captures CLI flags passed to rewind command
type RewindFlags struct {
	// NoPrompt skips user prompting for reason/learnings (--no-prompt flag)
	NoPrompt bool

	// Reason pre-provided via --reason flag (bypasses prompt)
	Reason string

	// Learnings pre-provided via --learnings flag (bypasses prompt)
	Learnings string
}

// UserProvidedContext captures reason and learnings from user prompts
type UserProvidedContext struct {
	// Reason why rewind occurred (required for magnitude 1+)
	Reason string

	// Learnings from work that triggered rewind (optional)
	Learnings string
}

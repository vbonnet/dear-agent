package roadmap

// Bead represents a task extracted from ROADMAP.md
type Bead struct {
	ID          string // e.g., "oss-tmux-capture"
	Description string // Task description
	Effort      string // e.g., "1 day", "2 hours"
	Status      string // Normalized: "pending", "in_progress", "completed", "cancelled"
	RawStatus   string // Original status from ROADMAP (e.g., "✅ COMPLETE")
	Phase       string // e.g., "phase-0", "phase-1"
	LineNumber  int    // Line number in ROADMAP.md for error reporting
}

// ValidationError represents a validation issue in ROADMAP.md
type ValidationError struct {
	Line    int
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

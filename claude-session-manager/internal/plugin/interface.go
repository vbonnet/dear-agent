package plugin

// TaskManagerPlugin is the core interface all task manager plugins must implement
// This is a data provider interface - plugins provide task data to AGM
type TaskManagerPlugin interface {
	// Metadata returns plugin information
	Metadata() PluginMetadata

	// GetTasks retrieves all tasks for a given session
	// sessionDir is the absolute path to the session directory
	// Returns a list of tasks or an error
	GetTasks(sessionDir string) ([]Task, error)

	// GetPhaseProgress calculates per-phase completion statistics
	// sessionDir is the absolute path to the session directory
	// Returns phase statistics or an error
	GetPhaseProgress(sessionDir string) ([]PhaseStats, error)

	// SupportsSession checks if this plugin can handle the given session
	// This allows plugins to auto-detect their applicability
	// For example, beads plugin checks for .beads/ directory
	SupportsSession(sessionDir string) bool
}

// UIRendererPlugin is an optional interface for custom UI rendering
// Plugins can implement this to provide custom TUI panels in Session Monitor
// If not implemented, Session Monitor will use default rendering
type UIRendererPlugin interface {
	// RenderPanel renders a custom TUI panel for task display
	// tasks is the list of tasks to render
	// Returns formatted string for display (supports ANSI colors)
	RenderPanel(tasks []Task) string

	// RenderPhaseProgress renders custom phase progress display
	// stats is the phase statistics to render
	// Returns formatted string for display (supports ANSI colors)
	RenderPhaseProgress(stats []PhaseStats) string
}

// Package phaseisolation implements the Wayfinder phase isolation system.
// It manages 12-phase workflow execution with isolated contexts,
// ported from the TypeScript implementation in cortex/lib/.
package phaseisolation

// PhaseID identifies a Wayfinder phase.
type PhaseID string

const (
	PhaseD1  PhaseID = "D1"
	PhaseD2  PhaseID = "D2"
	PhaseD3  PhaseID = "D3"
	PhaseD4  PhaseID = "D4"
	PhaseS4  PhaseID = "S4"
	PhaseS5  PhaseID = "S5"
	PhaseS6  PhaseID = "S6"
	PhaseS7  PhaseID = "S7"
	PhaseS8  PhaseID = "S8"
	PhaseS9  PhaseID = "S9"
	PhaseS10 PhaseID = "S10"
	PhaseS11 PhaseID = "S11"
)

// AllPhaseIDs returns all phase IDs in execution order.
func AllPhaseIDs() []PhaseID {
	return []PhaseID{
		PhaseD1, PhaseD2, PhaseD3, PhaseD4,
		PhaseS4, PhaseS5, PhaseS6, PhaseS7,
		PhaseS8, PhaseS9, PhaseS10, PhaseS11,
	}
}

// PhaseStatus represents the status of a phase execution.
type PhaseStatus string

const (
	StatusPending    PhaseStatus = "pending"
	StatusInProgress PhaseStatus = "in_progress"
	StatusCompleted  PhaseStatus = "completed"
	StatusFailed     PhaseStatus = "failed"
	StatusBlocked    PhaseStatus = "blocked"
)

// PhaseDefinition describes a phase with its metadata.
type PhaseDefinition struct {
	ID              PhaseID
	Name            string
	Objective       string
	Deliverable     string
	SuccessCriteria []string
	TokenBudget     int
}

// PhaseArtifact holds a phase's output with summary and metadata.
type PhaseArtifact struct {
	PhaseID  PhaseID
	Summary  string // 100-200 tokens
	FullPath string // File path to full artifact
	Metadata ArtifactMetadata
}

// ArtifactMetadata holds metadata about a phase artifact.
type ArtifactMetadata struct {
	Dependencies []PhaseID
	Consumers    []PhaseID
	Timestamp    int64
	TokenCount   int
}

// ArtifactSummary is used for context compilation.
type ArtifactSummary struct {
	PhaseID         PhaseID
	PhaseName       string
	Summary         string
	DeliverablePath string
	TokenCount      int
}

// OutputSpecification describes the expected output for a phase.
type OutputSpecification struct {
	Filename  string
	Format    string // always "markdown"
	Sections  []string
	MinLength int
	MaxLength int
}

// PhaseContext is the compiled context for phase execution.
type PhaseContext struct {
	PhaseName         string
	PhaseObjective    string
	PhaseSystemPrompt string
	PriorArtifacts    []ArtifactSummary
	SuccessCriteria   []string
	OutputSpec        OutputSpecification
	Metadata          ContextMetadata
}

// ContextMetadata holds metadata about the compiled context.
type ContextMetadata struct {
	SessionID       string
	ProjectPath     string
	TokenBudget     int
	EstimatedTokens int
}

// OrchestratorConfig configures the PhaseOrchestrator.
type OrchestratorConfig struct {
	ProjectPath    string  // Absolute path to Wayfinder project
	SessionID      string  // Wayfinder session ID (UUID)
	StartPhase     PhaseID // Optional: resume from phase
	EnableTaskTool bool    // Optional: try Task tool if available
}

// PhaseResult holds the result of a phase execution.
type PhaseResult struct {
	PhaseID    PhaseID
	Status     PhaseStatus
	Artifact   *PhaseArtifact
	Error      string
	TokenCount int
	Duration   int64 // milliseconds
}

// WorkflowResult holds the result of a full workflow execution.
type WorkflowResult struct {
	SessionID     string
	Results       []PhaseResult
	TotalTokens   int
	TokenSavings  int   // percentage
	TotalDuration int64 // milliseconds
}

// PhaseExecutionStrategy defines how phases are executed.
type PhaseExecutionStrategy interface {
	ExecutePhase(phase PhaseDefinition, context PhaseContext, config OrchestratorConfig) (*PhaseArtifact, error)
}

// LoadStrategy determines how dependency artifacts are loaded.
type LoadStrategy string

const (
	LoadFull    LoadStrategy = "full"
	LoadSummary LoadStrategy = "summary"
)

// V2PhaseName is the phase name used by Go wayfinder-session.
type V2PhaseName string

const (
	V2Charter  V2PhaseName = "CHARTER"
	V2Problem  V2PhaseName = "PROBLEM"
	V2Research V2PhaseName = "RESEARCH"
	V2Design   V2PhaseName = "DESIGN"
	V2Spec     V2PhaseName = "SPEC"
	V2Plan     V2PhaseName = "PLAN"
	V2Setup    V2PhaseName = "SETUP"
	V2Build    V2PhaseName = "BUILD"
	V2Retro    V2PhaseName = "RETRO"
)

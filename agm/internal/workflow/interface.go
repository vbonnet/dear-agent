package workflow

import (
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// Workflow represents a specialized execution mode for agents.
//
// Workflows define how agents should behave for specific tasks:
// - deep-research: Research URLs and synthesize insights
// - code-review: Analyze code changes and provide feedback
// - architect: Design system architectures
//
// Example usage:
//
//	workflow := registry.Get("deep-research")
//	result, err := workflow.Execute(WorkflowContext{
//	    Agent:     geminiAgent,
//	    SessionID: "session-123",
//	    Prompt:    "Research https://example.com and suggest improvements",
//	})
//
// Implementations must handle workflow-specific logic while integrating
// with the agent abstraction layer.
type Workflow interface {
	// Name returns the workflow identifier (e.g., "deep-research", "code-review").
	Name() string

	// Description returns a human-readable description of what this workflow does.
	Description() string

	// SupportedHarnesses returns the list of harness names that support this workflow.
	// Example: []string{"gemini-cli"} means only Gemini CLI harness supports this workflow.
	SupportedHarnesses() []string

	// Execute runs the workflow with the given context.
	//
	// The workflow should:
	// 1. Validate the prompt and extract required data
	// 2. Execute agent-specific operations
	// 3. Store results in structured format
	// 4. Return workflow result with artifacts
	//
	// Returns error if workflow execution fails.
	Execute(ctx WorkflowContext) (WorkflowResult, error)
}

// WorkflowContext contains execution parameters for a workflow.
type WorkflowContext struct {
	// Harness is the AI harness to use for this workflow.
	// Required.
	Harness agent.Agent

	// SessionID is the agent session identifier.
	// Required.
	SessionID agent.SessionID

	// Prompt is the user's input prompt containing task instructions.
	// Required.
	Prompt string

	// WorkingDirectory is the directory to execute the workflow in.
	// Optional. If empty, uses agent's current directory.
	WorkingDirectory string

	// OutputPath is the path to write workflow results.
	// Optional. If empty, workflow generates default path.
	OutputPath string

	// Environment contains environment variables for the workflow.
	// Optional.
	Environment map[string]string
}

// WorkflowResult contains the output of workflow execution.
type WorkflowResult struct {
	// Success indicates if the workflow completed successfully.
	Success bool

	// Artifacts are the files/documents created by this workflow.
	// Examples: research reports, code reviews, architecture diagrams.
	Artifacts []Artifact

	// Summary is a brief description of what was accomplished.
	Summary string

	// LogPath is the path to the workflow execution log (for crash resilience).
	// Optional.
	LogPath string

	// Metadata contains workflow-specific data.
	// Optional.
	Metadata map[string]interface{}

	// ExecutionTime is how long the workflow took to execute.
	ExecutionTime time.Duration
}

// Artifact represents a file or document created by a workflow.
type Artifact struct {
	// Type describes what kind of artifact this is.
	// Examples: "research-report", "code-review", "architecture-diagram".
	Type string

	// Path is the filesystem path to the artifact.
	Path string

	// Size is the artifact file size in bytes.
	// Optional.
	Size int64

	// Metadata contains artifact-specific data.
	// Optional.
	Metadata map[string]interface{}
}

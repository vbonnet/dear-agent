package wayfinder

import (
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/protocol"
)

// Status represents Wayfinder session status data
type Status struct {
	SessionID    string
	ProjectPath  string
	Status       string
	CurrentPhase string
	Phases       []Phase
}

// Phase represents a Wayfinder phase
type Phase struct {
	Name      string
	Status    string
	StartedAt time.Time
}

// TaskUpdate represents a Wayfinder task update
type TaskUpdate struct {
	TaskID      string
	Description string
	Status      string
	Phase       string
}

// Handoff represents a task handoff between agents
type Handoff struct {
	FromAgent    string
	ToAgent      string
	TaskID       string
	Context      string
	Deliverables []string
	Blockers     []string
}

// Blocker represents a task blocker
type Blocker struct {
	Type        string
	Description string
	BlockedTask string
	BlockedBy   []string
}

// StatusToMessage converts Wayfinder status to A2A protocol message
func StatusToMessage(status Status, agentID string) *protocol.Message {
	msg := protocol.NewMessage(agentID, protocol.StatusPending)

	msg.Context = fmt.Sprintf("Wayfinder session %s in phase %s (status: %s)",
		status.SessionID, status.CurrentPhase, status.Status)

	msg.Proposal = fmt.Sprintf("Current phase: %s\n\nPhase progress:\n%s",
		status.CurrentPhase, formatPhases(status.Phases))

	msg.Questions = []string{}
	msg.Blockers = []string{}

	switch status.Status {
	case "in_progress":
		msg.NextSteps = []string{
			fmt.Sprintf("Continue %s phase", status.CurrentPhase),
			"Report progress when phase completes",
		}
	case "completed":
		msg.NextSteps = []string{
			"Review deliverables",
			"Transition to next phase",
		}
		msg.Status = protocol.StatusConsensusReached
	}

	return msg
}

// PhaseTransitionMessage creates A2A message for phase transitions
func PhaseTransitionMessage(status Status, oldPhase, newPhase, agentID string) *protocol.Message {
	msg := protocol.NewMessage(agentID, protocol.StatusAwaitingResponse)

	msg.Context = fmt.Sprintf("Wayfinder session %s transitioning from %s to %s",
		status.SessionID, oldPhase, newPhase)

	msg.Proposal = fmt.Sprintf("Phase %s completed successfully.\n\nReady to start phase %s.\n\nProject: %s",
		oldPhase, newPhase, status.ProjectPath)

	msg.Questions = []string{
		fmt.Sprintf("Approved to proceed to phase %s?", newPhase),
		"Any concerns or blockers?",
	}

	msg.Blockers = []string{}

	msg.NextSteps = []string{
		fmt.Sprintf("Start phase %s tasks", newPhase),
		"Report progress at next checkpoint",
	}

	return msg
}

// TaskToMessage converts Wayfinder task update to A2A message
func TaskToMessage(task TaskUpdate, agentID, context string) *protocol.Message {
	msg := protocol.NewMessage(agentID, protocol.StatusPending)

	msg.Context = context
	if msg.Context == "" {
		msg.Context = fmt.Sprintf("Working on task %s in phase %s", task.TaskID, task.Phase)
	}

	msg.Proposal = fmt.Sprintf("Task: %s\n\nDescription: %s\n\nStatus: %s",
		task.TaskID, task.Description, task.Status)

	msg.Questions = []string{}
	msg.Blockers = []string{}

	switch task.Status {
	case "completed":
		msg.Status = protocol.StatusConsensusReached
		msg.NextSteps = []string{"Review task deliverables", "Mark task as complete"}
	case "blocked":
		msg.Status = protocol.NewBlockedStatus("task-dependency")
		msg.Blockers = []string{"Task blocked - awaiting dependency resolution"}
		msg.NextSteps = []string{"Resolve blocker", "Resume task"}
	default:
		msg.Status = protocol.StatusPending
		msg.NextSteps = []string{"Continue task work", "Report progress"}
	}

	return msg
}

// HandoffToMessage creates A2A message for task handoff
func HandoffToMessage(handoff Handoff) *protocol.Message {
	msg := protocol.NewMessage(handoff.FromAgent, protocol.StatusAwaitingResponse)

	msg.Context = fmt.Sprintf("Handing off task %s from %s to %s",
		handoff.TaskID, handoff.FromAgent, handoff.ToAgent)

	msg.Proposal = fmt.Sprintf("%s\n\nDeliverables:\n%s",
		handoff.Context, formatList(handoff.Deliverables))

	msg.Questions = []string{
		fmt.Sprintf("Can %s accept this handoff?", handoff.ToAgent),
		"Any questions about the deliverables?",
	}

	msg.Blockers = handoff.Blockers

	msg.NextSteps = []string{
		fmt.Sprintf("%s reviews handoff", handoff.ToAgent),
		fmt.Sprintf("%s accepts and proceeds", handoff.ToAgent),
	}

	return msg
}

// BlockerToMessage creates A2A message for blocker notification
func BlockerToMessage(blocker Blocker, agentID string) *protocol.Message {
	msg := protocol.NewMessage(agentID, protocol.NewBlockedStatus(blocker.Type))

	msg.Context = fmt.Sprintf("Task %s is blocked by %s", blocker.BlockedTask, blocker.Type)

	msg.Proposal = fmt.Sprintf("Blocker: %s\n\nBlocked by:\n%s",
		blocker.Description, formatList(blocker.BlockedBy))

	msg.Questions = []string{
		"When can this blocker be resolved?",
		"Is there a workaround available?",
	}

	msg.Blockers = append([]string{blocker.Description}, blocker.BlockedBy...)

	msg.NextSteps = []string{
		"Resolve blocking dependencies",
		"Resume task when unblocked",
	}

	return msg
}

func formatPhases(phases []Phase) string {
	if len(phases) == 0 {
		return "No phases defined"
	}

	result := ""
	for _, phase := range phases {
		statusIcon := "P"
		switch phase.Status {
		case "completed":
			statusIcon = "V"
		case "in_progress":
			statusIcon = ">"
		}

		result += fmt.Sprintf("- %s %s: %s\n", statusIcon, phase.Name, phase.Status)
	}

	return result
}

func formatList(items []string) string {
	if len(items) == 0 {
		return "None"
	}

	result := ""
	for i, item := range items {
		result += fmt.Sprintf("%d. %s\n", i+1, item)
	}

	return result
}

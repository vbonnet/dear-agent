// Package wayfinder provides wayfinder-related functionality.
package wayfinder

import (
	"fmt"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/protocol"
)

// Status represents Wayfinder session status data
type Status struct {
	ProjectName     string
	ProjectPath     string
	Status          string
	CurrentWaypoint string
	Waypoints       []Waypoint
}

// Waypoint represents a Wayfinder waypoint.
type Waypoint struct {
	Name      string
	Status    string
	StartedAt time.Time
}

// TaskUpdate represents a Wayfinder task update
type TaskUpdate struct {
	TaskID      string
	Description string
	Status      string
	Waypoint    string
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

	msg.Context = fmt.Sprintf("Wayfinder project %s at waypoint %s (status: %s)",
		status.ProjectName, status.CurrentWaypoint, status.Status)

	msg.Proposal = fmt.Sprintf("Current waypoint: %s\n\nWaypoint progress:\n%s",
		status.CurrentWaypoint, formatWaypoints(status.Waypoints))

	msg.Questions = []string{}
	msg.Blockers = []string{}

	switch status.Status {
	case "in-progress":
		msg.NextSteps = []string{
			fmt.Sprintf("Continue %s waypoint", status.CurrentWaypoint),
			"Report progress when the waypoint completes",
		}
	case "completed":
		msg.NextSteps = []string{
			"Review deliverables",
			"Transition to the next waypoint",
		}
		msg.Status = protocol.StatusConsensusReached
	}

	return msg
}

// WaypointTransitionMessage creates an A2A message for waypoint transitions.
func WaypointTransitionMessage(status Status, oldWaypoint, newWaypoint, agentID string) *protocol.Message {
	msg := protocol.NewMessage(agentID, protocol.StatusAwaitingResponse)

	msg.Context = fmt.Sprintf("Wayfinder project %s transitioning from %s to %s",
		status.ProjectName, oldWaypoint, newWaypoint)

	msg.Proposal = fmt.Sprintf("Waypoint %s completed successfully.\n\nReady to start waypoint %s.\n\nProject: %s",
		oldWaypoint, newWaypoint, status.ProjectPath)

	msg.Questions = []string{
		fmt.Sprintf("Approved to proceed to waypoint %s?", newWaypoint),
		"Any concerns or blockers?",
	}

	msg.Blockers = []string{}

	msg.NextSteps = []string{
		fmt.Sprintf("Start waypoint %s tasks", newWaypoint),
		"Report progress at next checkpoint",
	}

	return msg
}

// TaskToMessage converts Wayfinder task update to A2A message
func TaskToMessage(task TaskUpdate, agentID, context string) *protocol.Message {
	msg := protocol.NewMessage(agentID, protocol.StatusPending)

	msg.Context = context
	if msg.Context == "" {
		msg.Context = fmt.Sprintf("Working on task %s at waypoint %s", task.TaskID, task.Waypoint)
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

func formatWaypoints(waypoints []Waypoint) string {
	if len(waypoints) == 0 {
		return "No waypoints defined"
	}

	var result strings.Builder
	for _, waypoint := range waypoints {
		statusIcon := "P"
		switch waypoint.Status {
		case "completed":
			statusIcon = "V"
		case "in-progress":
			statusIcon = ">"
		}

		fmt.Fprintf(&result, "- %s %s: %s\n", statusIcon, waypoint.Name, waypoint.Status)
	}

	return result.String()
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

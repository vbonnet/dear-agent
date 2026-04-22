package wayfinder

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/protocol"
)

func TestStatusToMessage_InProgress(t *testing.T) {
	status := Status{
		SessionID:    "sess-1",
		ProjectPath:  "/tmp/project",
		Status:       "in_progress",
		CurrentPhase: "design",
		Phases: []Phase{
			{Name: "planning", Status: "completed"},
			{Name: "design", Status: "in_progress"},
		},
	}

	msg := StatusToMessage(status, "agent-A")

	if msg.AgentID != "agent-A" {
		t.Errorf("AgentID = %q, want %q", msg.AgentID, "agent-A")
	}
	if msg.Status != protocol.StatusPending {
		t.Errorf("Status = %q, want %q", msg.Status, protocol.StatusPending)
	}
	if !strings.Contains(msg.Context, "sess-1") {
		t.Error("Context should contain session ID")
	}
	if !strings.Contains(msg.Context, "design") {
		t.Error("Context should contain current phase")
	}

	// NextSteps should mention continuing the current phase
	found := false
	for _, step := range msg.NextSteps {
		if strings.Contains(step, "Continue") && strings.Contains(step, "design") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("NextSteps should contain step about continuing design phase, got %v", msg.NextSteps)
	}

	// formatPhases tested indirectly: multiple phases should appear in proposal
	if !strings.Contains(msg.Proposal, "planning") {
		t.Error("Proposal should contain phase 'planning' from formatPhases")
	}
	if !strings.Contains(msg.Proposal, "design") {
		t.Error("Proposal should contain phase 'design' from formatPhases")
	}
	// completed phase gets "V" icon, in_progress gets ">"
	if !strings.Contains(msg.Proposal, "V planning") {
		t.Error("Proposal should show V icon for completed phase")
	}
	if !strings.Contains(msg.Proposal, "> design") {
		t.Error("Proposal should show > icon for in_progress phase")
	}
}

func TestStatusToMessage_Completed(t *testing.T) {
	status := Status{
		SessionID:    "sess-2",
		Status:       "completed",
		CurrentPhase: "delivery",
	}

	msg := StatusToMessage(status, "agent-B")

	if msg.Status != protocol.StatusConsensusReached {
		t.Errorf("Status = %q, want %q", msg.Status, protocol.StatusConsensusReached)
	}

	hasReview := false
	for _, step := range msg.NextSteps {
		if strings.Contains(step, "Review deliverables") {
			hasReview = true
			break
		}
	}
	if !hasReview {
		t.Errorf("NextSteps should mention reviewing deliverables, got %v", msg.NextSteps)
	}
}

func TestPhaseTransitionMessage(t *testing.T) {
	status := Status{
		SessionID:   "sess-3",
		ProjectPath: "/my/project",
	}

	msg := PhaseTransitionMessage(status, "planning", "design", "agent-C")

	if msg.AgentID != "agent-C" {
		t.Errorf("AgentID = %q, want %q", msg.AgentID, "agent-C")
	}
	if msg.Status != protocol.StatusAwaitingResponse {
		t.Errorf("Status = %q, want %q", msg.Status, protocol.StatusAwaitingResponse)
	}

	// Context should describe the transition
	if !strings.Contains(msg.Context, "planning") || !strings.Contains(msg.Context, "design") {
		t.Errorf("Context should mention both old and new phases, got %q", msg.Context)
	}
	if !strings.Contains(msg.Context, "sess-3") {
		t.Error("Context should contain session ID")
	}

	// Questions should contain approval request
	approvalFound := false
	for _, q := range msg.Questions {
		if strings.Contains(q, "Approved") && strings.Contains(q, "design") {
			approvalFound = true
			break
		}
	}
	if !approvalFound {
		t.Errorf("Questions should contain approval request for new phase, got %v", msg.Questions)
	}

	// Proposal should mention the project path
	if !strings.Contains(msg.Proposal, "/my/project") {
		t.Errorf("Proposal should contain project path, got %q", msg.Proposal)
	}
}

func TestTaskToMessage_Completed(t *testing.T) {
	task := TaskUpdate{
		TaskID:      "task-10",
		Description: "Implement feature X",
		Status:      "completed",
		Phase:       "implementation",
	}

	msg := TaskToMessage(task, "agent-D", "custom context")

	if msg.Status != protocol.StatusConsensusReached {
		t.Errorf("Status = %q, want %q", msg.Status, protocol.StatusConsensusReached)
	}
	if msg.Context != "custom context" {
		t.Errorf("Context = %q, want %q", msg.Context, "custom context")
	}
	if !strings.Contains(msg.Proposal, "task-10") {
		t.Error("Proposal should contain task ID")
	}
}

func TestTaskToMessage_Blocked(t *testing.T) {
	task := TaskUpdate{
		TaskID:      "task-20",
		Description: "Deploy service",
		Status:      "blocked",
		Phase:       "deployment",
	}

	msg := TaskToMessage(task, "agent-E", "deploying")

	if !msg.Status.IsBlocked() {
		t.Errorf("Status should be blocked, got %q", msg.Status)
	}
	if msg.Status.BlockedReason() != "task-dependency" {
		t.Errorf("BlockedReason = %q, want %q", msg.Status.BlockedReason(), "task-dependency")
	}
	if len(msg.Blockers) == 0 {
		t.Error("Blockers list should not be empty for blocked task")
	}
}

func TestTaskToMessage_EmptyContext(t *testing.T) {
	task := TaskUpdate{
		TaskID: "task-30",
		Phase:  "testing",
	}

	msg := TaskToMessage(task, "agent-F", "")

	if !strings.Contains(msg.Context, "task-30") {
		t.Error("Default context should contain task ID")
	}
	if !strings.Contains(msg.Context, "testing") {
		t.Error("Default context should contain phase name")
	}
}

func TestHandoffToMessage(t *testing.T) {
	handoff := Handoff{
		FromAgent:    "agent-X",
		ToAgent:      "agent-Y",
		TaskID:       "task-50",
		Context:      "Handing off implementation work",
		Deliverables: []string{"API endpoint", "Database schema"},
		Blockers:     []string{"CI pipeline broken"},
	}

	msg := HandoffToMessage(handoff)

	if msg.AgentID != "agent-X" {
		t.Errorf("AgentID = %q, want %q", msg.AgentID, "agent-X")
	}
	if msg.Status != protocol.StatusAwaitingResponse {
		t.Errorf("Status = %q, want %q", msg.Status, protocol.StatusAwaitingResponse)
	}

	// Context should mention both agents
	if !strings.Contains(msg.Context, "agent-X") || !strings.Contains(msg.Context, "agent-Y") {
		t.Errorf("Context should mention from/to agents, got %q", msg.Context)
	}
	if !strings.Contains(msg.Context, "task-50") {
		t.Error("Context should contain task ID")
	}

	// Deliverables should be in proposal (via formatList)
	if !strings.Contains(msg.Proposal, "API endpoint") {
		t.Error("Proposal should contain deliverable 'API endpoint'")
	}
	if !strings.Contains(msg.Proposal, "Database schema") {
		t.Error("Proposal should contain deliverable 'Database schema'")
	}

	// Blockers should be passed through
	if len(msg.Blockers) != 1 || msg.Blockers[0] != "CI pipeline broken" {
		t.Errorf("Blockers = %v, want [CI pipeline broken]", msg.Blockers)
	}
}

func TestHandoffToMessage_EmptyDeliverables(t *testing.T) {
	handoff := Handoff{
		FromAgent:    "agent-A",
		ToAgent:      "agent-B",
		TaskID:       "task-60",
		Context:      "Quick handoff",
		Deliverables: []string{},
	}

	msg := HandoffToMessage(handoff)

	// formatList with empty slice returns "None"
	if !strings.Contains(msg.Proposal, "None") {
		t.Errorf("Proposal should contain 'None' for empty deliverables, got %q", msg.Proposal)
	}
}

func TestBlockerToMessage(t *testing.T) {
	blocker := Blocker{
		Type:        "infrastructure",
		Description: "Database is down",
		BlockedTask: "task-70",
		BlockedBy:   []string{"DB team", "Ops team"},
	}

	msg := BlockerToMessage(blocker, "agent-Z")

	if msg.AgentID != "agent-Z" {
		t.Errorf("AgentID = %q, want %q", msg.AgentID, "agent-Z")
	}

	// Status should be blocked with the blocker type
	if !msg.Status.IsBlocked() {
		t.Errorf("Status should be blocked, got %q", msg.Status)
	}
	if msg.Status.BlockedReason() != "infrastructure" {
		t.Errorf("BlockedReason = %q, want %q", msg.Status.BlockedReason(), "infrastructure")
	}

	// Context should mention the blocked task and type
	if !strings.Contains(msg.Context, "task-70") {
		t.Error("Context should contain blocked task ID")
	}
	if !strings.Contains(msg.Context, "infrastructure") {
		t.Error("Context should contain blocker type")
	}

	// Blockers list should be populated with description + blockedBy
	if len(msg.Blockers) != 3 {
		t.Errorf("Blockers length = %d, want 3 (description + 2 blockedBy)", len(msg.Blockers))
	}
	if msg.Blockers[0] != "Database is down" {
		t.Errorf("Blockers[0] = %q, want %q", msg.Blockers[0], "Database is down")
	}

	// Proposal should contain the blocker description and blocked-by items
	if !strings.Contains(msg.Proposal, "Database is down") {
		t.Error("Proposal should contain blocker description")
	}
	if !strings.Contains(msg.Proposal, "DB team") {
		t.Error("Proposal should contain blocked-by items")
	}
}

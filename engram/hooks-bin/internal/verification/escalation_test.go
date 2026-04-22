package verification

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestCheckEscalations_NoEscalation(t *testing.T) {
	state := &State{
		Pending: []PendingVerification{
			{ID: "v1", Type: "bead_close", ToolUsesSince: 2},
			{ID: "v2", Type: "notification_send", ToolUsesSince: 4},
		},
	}

	results := CheckEscalations(state)
	if len(results) != 0 {
		t.Errorf("expected no escalations below threshold, got %d", len(results))
	}
}

func TestCheckEscalations_AtThreshold(t *testing.T) {
	state := &State{
		Pending: []PendingVerification{
			{ID: "v1", Type: "bead_close", ToolUsesSince: EscalationThreshold, SwarmLabel: "my-swarm", BeadID: "src-123"},
		},
	}

	results := CheckEscalations(state)
	if len(results) != 1 {
		t.Fatalf("expected 1 escalation at threshold, got %d", len(results))
	}
	if !results[0].Escalated {
		t.Error("should be marked as escalated")
	}
}

func TestCheckEscalations_AboveThreshold(t *testing.T) {
	state := &State{
		Pending: []PendingVerification{
			{ID: "v1", Type: "bead_close", ToolUsesSince: 10, SwarmLabel: "swarm-a", BeadID: "src-1"},
			{ID: "v2", Type: "notification_send", ToolUsesSince: 7, Recipient: "worker-1"},
		},
	}

	results := CheckEscalations(state)
	if len(results) != 2 {
		t.Fatalf("expected 2 escalations, got %d", len(results))
	}
}

func TestCheckEscalations_MixedThresholds(t *testing.T) {
	state := &State{
		Pending: []PendingVerification{
			{ID: "v1", Type: "bead_close", ToolUsesSince: 10, SwarmLabel: "s", BeadID: "b"},
			{ID: "v2", Type: "bead_close", ToolUsesSince: 2, SwarmLabel: "s", BeadID: "c"},
		},
	}

	results := CheckEscalations(state)
	if len(results) != 1 {
		t.Fatalf("expected 1 escalation (only above threshold), got %d", len(results))
	}
	if results[0].Verification.ID != "v1" {
		t.Errorf("expected v1 to escalate, got %s", results[0].Verification.ID)
	}
}

func TestCheckEscalations_EmptyState(t *testing.T) {
	state := &State{}
	results := CheckEscalations(state)
	if len(results) != 0 {
		t.Errorf("expected no escalations for empty state, got %d", len(results))
	}
}

func TestWriteEscalations_BeadClose(t *testing.T) {
	var stderr bytes.Buffer

	results := []EscalationResult{
		{
			Escalated: true,
			Message:   formatEscalation(PendingVerification{Type: "bead_close", SwarmLabel: "my-swarm", BeadID: "src-123", ToolUsesSince: 6}),
			Verification: PendingVerification{
				Type:       "bead_close",
				SwarmLabel: "my-swarm",
				BeadID:     "src-123",
			},
		},
	}

	count := WriteEscalations(&stderr, results)
	if count != 1 {
		t.Errorf("expected 1 escalation written, got %d", count)
	}

	output := stderr.String()
	if !strings.Contains(output, "ESCALATION") {
		t.Error("output should contain ESCALATION header")
	}
	if !strings.Contains(output, "my-swarm") {
		t.Error("output should contain swarm label")
	}
	if !strings.Contains(output, "src-123") {
		t.Error("output should contain bead ID")
	}
	if !strings.Contains(output, "bd list") {
		t.Error("output should suggest bd list command")
	}
}

func TestWriteEscalations_NotificationSend(t *testing.T) {
	var stderr bytes.Buffer

	results := []EscalationResult{
		{
			Escalated: true,
			Message:   formatEscalation(PendingVerification{Type: "notification_send", Recipient: "worker-session", ToolUsesSince: 8}),
			Verification: PendingVerification{
				Type:      "notification_send",
				Recipient: "worker-session",
			},
		},
	}

	count := WriteEscalations(&stderr, results)
	output := stderr.String()

	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
	if !strings.Contains(output, "worker-session") {
		t.Error("output should contain recipient")
	}
	if !strings.Contains(output, "Check for response") {
		t.Error("output should suggest checking for response")
	}
}

func TestWriteEscalations_NoEscalated(t *testing.T) {
	var stderr bytes.Buffer
	results := []EscalationResult{
		{Escalated: false},
	}

	count := WriteEscalations(&stderr, results)
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
	if stderr.Len() != 0 {
		t.Error("should not write anything for non-escalated results")
	}
}

func TestFormatEscalation_UnknownType(t *testing.T) {
	msg := formatEscalation(PendingVerification{
		Type:          "unknown",
		Message:       "custom message",
		ToolUsesSince: 10,
	})

	if !strings.Contains(msg, "unknown") {
		t.Error("should include type")
	}
	if !strings.Contains(msg, "custom message") {
		t.Error("should include message")
	}
}

func TestIntegration_FullEscalationFlow(t *testing.T) {
	// Simulate the full flow: add verification, increment, check escalation
	state := State{}

	// Step 1: Add pending verification (bead closed)
	state.AddPending(PendingVerification{
		ID:         "close-1",
		Type:       "bead_close",
		SwarmLabel: "test-swarm",
		BeadID:     "src-abc",
		Message:    "3 remaining beads",
	})

	// Step 2: Simulate 4 tool uses (no escalation yet)
	for i := 0; i < 4; i++ {
		state.IncrementAll()
	}
	results := CheckEscalations(&state)
	if len(results) != 0 {
		t.Error("should not escalate before threshold")
	}

	// Step 3: One more tool use (hits threshold)
	state.IncrementAll()
	results = CheckEscalations(&state)
	if len(results) != 1 {
		t.Fatalf("expected 1 escalation at threshold, got %d", len(results))
	}

	// Step 4: Remove after addressing
	state.RemoveByType("bead_close", "close-1")
	results = CheckEscalations(&state)
	if len(results) != 0 {
		t.Error("should have no escalations after removal")
	}
}

func TestIntegration_PruneAndEscalate(t *testing.T) {
	state := State{
		Pending: []PendingVerification{
			{
				ID:            "old",
				Type:          "bead_close",
				CreatedAt:     time.Now().Add(-2 * time.Hour),
				ToolUsesSince: 100,
				SwarmLabel:    "s",
				BeadID:        "b",
			},
			{
				ID:            "recent",
				Type:          "bead_close",
				CreatedAt:     time.Now(),
				ToolUsesSince: 10,
				SwarmLabel:    "s",
				BeadID:        "c",
			},
		},
	}

	// Prune old entries
	state.PruneOld(1 * time.Hour)

	// Only recent should remain and escalate
	results := CheckEscalations(&state)
	if len(results) != 1 {
		t.Fatalf("expected 1 escalation after prune, got %d", len(results))
	}
	if results[0].Verification.ID != "recent" {
		t.Errorf("expected recent to escalate, got %s", results[0].Verification.ID)
	}
}

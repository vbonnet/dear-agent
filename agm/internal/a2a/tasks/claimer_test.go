package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testChannelContent = `# A2A Channel: test-channel

---
**Created**: 2024-01-01
**Topic**: Test Topic
**Participants**: agent-1
---

## Message #1

---
**Agent ID**: agent-1
**Timestamp**: 2024-01-15 10:00
**Status**: awaiting-response
**Message #**: 1
---

### Context

Test context

### Proposal

Test proposal content here

### Questions for Other Agent

None

### Blockers/Dependencies

None

### Proposed Next Steps

1. Wait for response

---
`

const claimedChannelContent = `# A2A Channel: claimed-channel

---
**Created**: 2024-01-01
**Topic**: Claimed Topic
**Owner**: other-agent
**Claimed**: 2024-01-15 11:00
**Participants**: agent-1
---

## Message #1

---
**Agent ID**: agent-1
**Timestamp**: 2024-01-15 10:00
**Status**: awaiting-response
**Message #**: 1
---

### Context

Test context

### Proposal

Already claimed proposal

### Questions for Other Agent

None

### Blockers/Dependencies

None

### Proposed Next Steps

1. Wait for response

---
`

// setupTestChannel creates a temp directory with active/ subdir and writes the
// given channel content to active/<channelID>.md. Returns the parent dir
// (channelsDir) suitable for NewClaimer.
func setupTestChannel(t *testing.T, channelID, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	activeDir := filepath.Join(tmpDir, "active")
	if err := os.MkdirAll(activeDir, 0755); err != nil {
		t.Fatalf("failed to create active dir: %v", err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(activeDir, channelID+".md"), []byte(content), 0644); err != nil {
			t.Fatalf("failed to write channel file: %v", err)
		}
	}
	return tmpDir
}

func TestNewClaimer(t *testing.T) {
	tmpDir := t.TempDir()
	c := NewClaimer(tmpDir)

	if c.channelsDir != tmpDir {
		t.Errorf("channelsDir = %q, want %q", c.channelsDir, tmpDir)
	}
	wantActive := filepath.Join(tmpDir, "active")
	if c.activeDir != wantActive {
		t.Errorf("activeDir = %q, want %q", c.activeDir, wantActive)
	}
}

func TestClaimTask(t *testing.T) {
	t.Run("claim unclaimed task", func(t *testing.T) {
		channelsDir := setupTestChannel(t, "test-channel", testChannelContent)
		c := NewClaimer(channelsDir)

		ok, err := c.ClaimTask("test-channel", "my-agent", "working on it")
		if err != nil {
			t.Fatalf("ClaimTask returned error: %v", err)
		}
		if !ok {
			t.Fatal("ClaimTask returned false, want true")
		}

		// Verify file was updated with Owner and in-progress status.
		content, err := os.ReadFile(filepath.Join(channelsDir, "active", "test-channel.md"))
		if err != nil {
			t.Fatalf("failed to read channel file: %v", err)
		}
		contentStr := string(content)

		if !strings.Contains(contentStr, "**Owner**: my-agent") {
			t.Error("expected **Owner**: my-agent in content after claim")
		}
		if !strings.Contains(contentStr, "**Claimed**:") {
			t.Error("expected **Claimed**: line in content after claim")
		}
		if !strings.Contains(contentStr, "**Claim Reason**: working on it") {
			t.Error("expected **Claim Reason**: working on it in content after claim")
		}
		if !strings.Contains(contentStr, "**Status**: in-progress") {
			t.Error("expected status to be updated to in-progress")
		}
	})

	t.Run("re-claiming returns error", func(t *testing.T) {
		channelsDir := setupTestChannel(t, "test-channel", testChannelContent)
		c := NewClaimer(channelsDir)

		_, err := c.ClaimTask("test-channel", "my-agent", "first claim")
		if err != nil {
			t.Fatalf("first ClaimTask returned error: %v", err)
		}

		ok, err := c.ClaimTask("test-channel", "another-agent", "second claim")
		if err == nil {
			t.Fatal("expected error when re-claiming, got nil")
		}
		if ok {
			t.Fatal("expected false when re-claiming")
		}
		if !strings.Contains(err.Error(), "already claimed") {
			t.Errorf("expected 'already claimed' in error, got: %v", err)
		}
	})

	t.Run("claim nonexistent channel", func(t *testing.T) {
		channelsDir := setupTestChannel(t, "test-channel", testChannelContent)
		c := NewClaimer(channelsDir)

		_, err := c.ClaimTask("nonexistent", "my-agent", "reason")
		if err == nil {
			t.Fatal("expected error for nonexistent channel")
		}
		if !strings.Contains(err.Error(), "channel not found") {
			t.Errorf("expected 'channel not found' in error, got: %v", err)
		}
	})
}

func TestUnclaimTask(t *testing.T) {
	t.Run("unclaim owned task", func(t *testing.T) {
		channelsDir := setupTestChannel(t, "test-channel", testChannelContent)
		c := NewClaimer(channelsDir)

		// Claim first.
		_, err := c.ClaimTask("test-channel", "my-agent", "claiming")
		if err != nil {
			t.Fatalf("ClaimTask returned error: %v", err)
		}

		// Unclaim.
		err = c.UnclaimTask("test-channel", "my-agent", "done with it")
		if err != nil {
			t.Fatalf("UnclaimTask returned error: %v", err)
		}

		// Verify Owner is removed and status is back to awaiting-response.
		content, err := os.ReadFile(filepath.Join(channelsDir, "active", "test-channel.md"))
		if err != nil {
			t.Fatalf("failed to read channel file: %v", err)
		}
		contentStr := string(content)

		if strings.Contains(contentStr, "**Owner**:") {
			t.Error("expected **Owner**: to be removed after unclaim")
		}
		if strings.Contains(contentStr, "**Claimed**:") {
			t.Error("expected **Claimed**: to be removed after unclaim")
		}
		if !strings.Contains(contentStr, "**Unclaimed**:") {
			t.Error("expected **Unclaimed**: line to be present after unclaim with reason")
		}
		if !strings.Contains(contentStr, "**Status**: awaiting-response") {
			t.Error("expected status to be reverted to awaiting-response")
		}
	})

	t.Run("wrong agent cannot unclaim", func(t *testing.T) {
		channelsDir := setupTestChannel(t, "test-channel", testChannelContent)
		c := NewClaimer(channelsDir)

		_, err := c.ClaimTask("test-channel", "my-agent", "claiming")
		if err != nil {
			t.Fatalf("ClaimTask returned error: %v", err)
		}

		err = c.UnclaimTask("test-channel", "wrong-agent", "stealing")
		if err == nil {
			t.Fatal("expected error when wrong agent tries to unclaim")
		}
		if !strings.Contains(err.Error(), "only owner") {
			t.Errorf("expected 'only owner' in error, got: %v", err)
		}
	})

	t.Run("unclaim nonexistent channel", func(t *testing.T) {
		channelsDir := setupTestChannel(t, "test-channel", testChannelContent)
		c := NewClaimer(channelsDir)

		err := c.UnclaimTask("nonexistent", "my-agent", "reason")
		if err == nil {
			t.Fatal("expected error for nonexistent channel")
		}
		if !strings.Contains(err.Error(), "channel not found") {
			t.Errorf("expected 'channel not found' in error, got: %v", err)
		}
	})
}

func TestListClaimableTasks(t *testing.T) {
	t.Run("empty dir returns empty list", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Don't even create active/ subdir - should handle gracefully.
		c := NewClaimer(tmpDir)

		tasks, err := c.ListClaimableTasks()
		if err != nil {
			t.Fatalf("ListClaimableTasks returned error: %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(tasks))
		}
	})

	t.Run("unclaimed awaiting-response channel is returned", func(t *testing.T) {
		channelsDir := setupTestChannel(t, "test-channel", testChannelContent)
		c := NewClaimer(channelsDir)

		tasks, err := c.ListClaimableTasks()
		if err != nil {
			t.Fatalf("ListClaimableTasks returned error: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}

		task := tasks[0]
		if task.ChannelID != "test-channel" {
			t.Errorf("ChannelID = %q, want %q", task.ChannelID, "test-channel")
		}
		if task.Topic != "Test Topic" {
			t.Errorf("Topic = %q, want %q", task.Topic, "Test Topic")
		}
		if task.PostedBy != "agent-1" {
			t.Errorf("PostedBy = %q, want %q", task.PostedBy, "agent-1")
		}
		// SplitN(line, ":", 2) splits on the first ":" only, so the full
		// timestamp "2024-01-15 10:00" is preserved after TrimSpace.
		if task.Timestamp != "2024-01-15 10:00" {
			t.Errorf("Timestamp = %q, want %q", task.Timestamp, "2024-01-15 10:00")
		}
		if task.Description != "Test proposal content here" {
			t.Errorf("Description = %q, want %q", task.Description, "Test proposal content here")
		}
	})

	t.Run("claimed channel is excluded", func(t *testing.T) {
		channelsDir := setupTestChannel(t, "claimed-channel", claimedChannelContent)
		c := NewClaimer(channelsDir)

		tasks, err := c.ListClaimableTasks()
		if err != nil {
			t.Fatalf("ListClaimableTasks returned error: %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("expected 0 claimable tasks for claimed channel, got %d", len(tasks))
		}
	})

	t.Run("mix of claimed and unclaimed", func(t *testing.T) {
		tmpDir := t.TempDir()
		activeDir := filepath.Join(tmpDir, "active")
		if err := os.MkdirAll(activeDir, 0755); err != nil {
			t.Fatalf("failed to create active dir: %v", err)
		}

		// Write unclaimed channel.
		if err := os.WriteFile(filepath.Join(activeDir, "unclaimed.md"), []byte(testChannelContent), 0644); err != nil {
			t.Fatalf("failed to write unclaimed channel: %v", err)
		}
		// Write claimed channel.
		if err := os.WriteFile(filepath.Join(activeDir, "claimed.md"), []byte(claimedChannelContent), 0644); err != nil {
			t.Fatalf("failed to write claimed channel: %v", err)
		}

		c := NewClaimer(tmpDir)
		tasks, err := c.ListClaimableTasks()
		if err != nil {
			t.Fatalf("ListClaimableTasks returned error: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 claimable task, got %d", len(tasks))
		}
		if tasks[0].ChannelID != "unclaimed" {
			t.Errorf("expected unclaimed channel, got %q", tasks[0].ChannelID)
		}
	})

	t.Run("description truncated to 150 chars", func(t *testing.T) {
		longProposal := strings.Repeat("A", 200)
		content := strings.Replace(testChannelContent, "Test proposal content here", longProposal, 1)
		channelsDir := setupTestChannel(t, "long-proposal", content)
		c := NewClaimer(channelsDir)

		tasks, err := c.ListClaimableTasks()
		if err != nil {
			t.Fatalf("ListClaimableTasks returned error: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		if len(tasks[0].Description) != 150 {
			t.Errorf("expected description length 150, got %d", len(tasks[0].Description))
		}
	})
}

func TestClaimUnclaimRoundTrip(t *testing.T) {
	channelsDir := setupTestChannel(t, "roundtrip", testChannelContent)
	c := NewClaimer(channelsDir)

	// Initially should be claimable.
	tasks, err := c.ListClaimableTasks()
	if err != nil {
		t.Fatalf("ListClaimableTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 claimable task initially, got %d", len(tasks))
	}

	// Claim it.
	ok, err := c.ClaimTask("roundtrip", "my-agent", "taking it")
	if err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}
	if !ok {
		t.Fatal("ClaimTask returned false")
	}

	// Should no longer be claimable.
	tasks, err = c.ListClaimableTasks()
	if err != nil {
		t.Fatalf("ListClaimableTasks returned error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 claimable tasks after claim, got %d", len(tasks))
	}

	// Unclaim it.
	err = c.UnclaimTask("roundtrip", "my-agent", "releasing")
	if err != nil {
		t.Fatalf("UnclaimTask returned error: %v", err)
	}

	// Should be claimable again.
	tasks, err = c.ListClaimableTasks()
	if err != nil {
		t.Fatalf("ListClaimableTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 claimable task after unclaim, got %d", len(tasks))
	}
}

package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewChecker_NilOptions(t *testing.T) {
	c := NewChecker(nil)
	if c == nil {
		t.Fatal("NewChecker(nil) returned nil")
	}
	if c.channelsDir == "" {
		t.Error("expected non-empty channelsDir with nil options")
	}
	if c.stateFile == "" {
		t.Error("expected non-empty stateFile with nil options")
	}
	if c.verbose {
		t.Error("expected verbose to be false with nil options")
	}
	if c.useAGM {
		t.Error("expected useAGM to be false with nil options")
	}
}

func TestNewChecker_CustomOptions(t *testing.T) {
	c := NewChecker(&CheckerOptions{
		ChannelsDir: "/custom/channels",
		StateFile:   "/custom/state.json",
		Verbose:     true,
		UseAGM:      true,
	})
	if c.channelsDir != "/custom/channels" {
		t.Errorf("channelsDir = %q, want /custom/channels", c.channelsDir)
	}
	if c.stateFile != "/custom/state.json" {
		t.Errorf("stateFile = %q, want /custom/state.json", c.stateFile)
	}
	if !c.verbose {
		t.Error("expected verbose to be true")
	}
	if !c.useAGM {
		t.Error("expected useAGM to be true")
	}
}

func TestLoadState_FreshState(t *testing.T) {
	tmp := t.TempDir()
	c := NewChecker(&CheckerOptions{
		ChannelsDir: tmp,
		StateFile:   filepath.Join(tmp, "nonexistent", "state.json"),
	})

	state, err := c.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state == nil {
		t.Fatal("LoadState() returned nil state")
	}
	if state.LastCheckTime == "" {
		t.Error("expected non-empty LastCheckTime")
	}
	if state.ChannelsChecked == nil {
		t.Error("expected non-nil ChannelsChecked map")
	}
}

func TestLoadState_SaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	stateFile := filepath.Join(tmp, "state.json")
	c := NewChecker(&CheckerOptions{
		ChannelsDir: tmp,
		StateFile:   stateFile,
	})

	original := &State{
		LastCheckTime: "2024-01-15T10:00:00Z",
		ChannelsChecked: map[string]ChannelState{
			"test-channel": {
				LastSeenMessageTimestamp: "2024-01-15",
				LastStatus:              "awaiting-response",
			},
		},
	}

	if err := c.SaveState(original); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	loaded, err := c.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if loaded.LastCheckTime != original.LastCheckTime {
		t.Errorf("LastCheckTime = %q, want %q", loaded.LastCheckTime, original.LastCheckTime)
	}
	cs, ok := loaded.ChannelsChecked["test-channel"]
	if !ok {
		t.Fatal("expected test-channel in ChannelsChecked")
	}
	if cs.LastSeenMessageTimestamp != "2024-01-15" {
		t.Errorf("LastSeenMessageTimestamp = %q, want %q", cs.LastSeenMessageTimestamp, "2024-01-15")
	}
	if cs.LastStatus != "awaiting-response" {
		t.Errorf("LastStatus = %q, want %q", cs.LastStatus, "awaiting-response")
	}
}

func TestSaveState_WritesFile(t *testing.T) {
	tmp := t.TempDir()
	stateFile := filepath.Join(tmp, "subdir", "state.json")
	c := NewChecker(&CheckerOptions{
		ChannelsDir: tmp,
		StateFile:   stateFile,
	})

	state := &State{
		LastCheckTime:   "2024-06-01T12:00:00Z",
		ChannelsChecked: map[string]ChannelState{},
	}

	if err := c.SaveState(state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var loaded State
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal state file: %v", err)
	}
	if loaded.LastCheckTime != "2024-06-01T12:00:00Z" {
		t.Errorf("LastCheckTime = %q, want %q", loaded.LastCheckTime, "2024-06-01T12:00:00Z")
	}
}

func TestParseMessageHeader_ValidContent(t *testing.T) {
	content := `---
**Created**: 2024-01-01
**Topic**: Test
---

## Message #1

---
**Agent ID**: test-agent
**Timestamp**: 2024-01-15
**Status**: awaiting-response
**Message #**: 1
---

### Proposal

This is a test proposal.

---`

	c := NewChecker(&CheckerOptions{
		ChannelsDir: "/tmp",
		StateFile:   "/tmp/state.json",
	})

	header := c.ParseMessageHeader(content)
	if header == nil {
		t.Fatal("ParseMessageHeader() returned nil for valid content")
	}
	if header.AgentID != "test-agent" {
		t.Errorf("AgentID = %q, want %q", header.AgentID, "test-agent")
	}
	if header.Timestamp != "2024-01-15" {
		t.Errorf("Timestamp = %q, want %q", header.Timestamp, "2024-01-15")
	}
	if header.Status != "awaiting-response" {
		t.Errorf("Status = %q, want %q", header.Status, "awaiting-response")
	}
	if header.MessageNumber != "1" {
		t.Errorf("MessageNumber = %q, want %q", header.MessageNumber, "1")
	}
	if !strings.Contains(header.ProposalPreview, "This is a test proposal.") {
		t.Errorf("ProposalPreview = %q, expected it to contain %q", header.ProposalPreview, "This is a test proposal.")
	}
}

func TestParseMessageHeader_EmptyContent(t *testing.T) {
	c := NewChecker(&CheckerOptions{
		ChannelsDir: "/tmp",
		StateFile:   "/tmp/state.json",
	})

	header := c.ParseMessageHeader("")
	if header != nil {
		t.Errorf("ParseMessageHeader(\"\") = %+v, want nil", header)
	}
}

func TestParseMessageHeader_IncompleteContent(t *testing.T) {
	content := `---
**Created**: 2024-01-01
**Topic**: Test
---`

	c := NewChecker(&CheckerOptions{
		ChannelsDir: "/tmp",
		StateFile:   "/tmp/state.json",
	})

	header := c.ParseMessageHeader(content)
	if header != nil {
		t.Errorf("ParseMessageHeader(incomplete) = %+v, want nil", header)
	}
}

func TestFormatNotification(t *testing.T) {
	c := NewChecker(&CheckerOptions{
		ChannelsDir: "/tmp",
		StateFile:   "/tmp/state.json",
	})

	n := &Notification{
		ChannelName:     "test-channel",
		ChannelFile:     "/path/to/test-channel.md",
		AgentID:         "agent-1",
		Timestamp:       "2024-01-15",
		Status:          "awaiting-response",
		MessageNumber:   "3",
		ProposalPreview: "Please review this change",
	}

	result := c.FormatNotification(n)

	expectations := []string{
		"test-channel",
		"awaiting-response",
		"agent-1",
		"2024-01-15",
		"Please review this change",
		"/path/to/test-channel.md",
	}
	for _, expected := range expectations {
		if !strings.Contains(result, expected) {
			t.Errorf("FormatNotification() result missing %q\ngot: %s", expected, result)
		}
	}
}

func TestCheckChannel_ValidAwaitingResponse(t *testing.T) {
	tmp := t.TempDir()
	channelsDir := filepath.Join(tmp, "channels")
	if err := os.MkdirAll(channelsDir, 0755); err != nil {
		t.Fatalf("failed to create channels dir: %v", err)
	}

	content := `---
**Created**: 2024-01-01
**Topic**: Test
---

## Message #1

---
**Agent ID**: test-agent
**Timestamp**: 2024-01-15
**Status**: awaiting-response
**Message #**: 1
---

### Proposal

This is a test proposal.

---`

	channelFile := filepath.Join(channelsDir, "test-channel.md")
	if err := os.WriteFile(channelFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write channel file: %v", err)
	}

	c := NewChecker(&CheckerOptions{
		ChannelsDir: channelsDir,
		StateFile:   filepath.Join(tmp, "state.json"),
	})

	state := &State{
		LastCheckTime:   "2024-01-01T00:00:00Z",
		ChannelsChecked: make(map[string]ChannelState),
	}

	notification := c.CheckChannel(channelFile, state)
	if notification == nil {
		t.Fatal("CheckChannel() returned nil, expected a notification for awaiting-response")
	}
	if notification.ChannelName != "test-channel" {
		t.Errorf("ChannelName = %q, want %q", notification.ChannelName, "test-channel")
	}
	if notification.AgentID != "test-agent" {
		t.Errorf("AgentID = %q, want %q", notification.AgentID, "test-agent")
	}
	if notification.Status != "awaiting-response" {
		t.Errorf("Status = %q, want %q", notification.Status, "awaiting-response")
	}
	if notification.MessageNumber != "1" {
		t.Errorf("MessageNumber = %q, want %q", notification.MessageNumber, "1")
	}

	// Verify state was updated
	cs, ok := state.ChannelsChecked["test-channel"]
	if !ok {
		t.Fatal("expected test-channel in state.ChannelsChecked after CheckChannel")
	}
	if cs.LastSeenMessageTimestamp != "2024-01-15" {
		t.Errorf("state LastSeenMessageTimestamp = %q, want %q", cs.LastSeenMessageTimestamp, "2024-01-15")
	}
	if cs.LastStatus != "awaiting-response" {
		t.Errorf("state LastStatus = %q, want %q", cs.LastStatus, "awaiting-response")
	}
}

package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCollector(t *testing.T) {
	t.Run("empty string returns error", func(t *testing.T) {
		c, err := NewCollector("")
		if err == nil {
			t.Fatal("expected error for empty channelsDir, got nil")
		}
		if c != nil {
			t.Fatal("expected nil collector for empty channelsDir")
		}
	})

	t.Run("valid dir succeeds", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c == nil {
			t.Fatal("expected non-nil collector")
		}
		if c.channelsDir != tmpDir {
			t.Errorf("channelsDir = %q, want %q", c.channelsDir, tmpDir)
		}
		if c.activeDir != filepath.Join(tmpDir, "active") {
			t.Errorf("activeDir = %q, want %q", c.activeDir, filepath.Join(tmpDir, "active"))
		}
	})

	t.Run("active suffix is stripped", func(t *testing.T) {
		tmpDir := t.TempDir()
		activePath := filepath.Join(tmpDir, "active")
		if err := os.MkdirAll(activePath, 0755); err != nil {
			t.Fatal(err)
		}
		c, err := NewCollector(activePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.channelsDir != tmpDir {
			t.Errorf("channelsDir = %q, want %q (active should be stripped)", c.channelsDir, tmpDir)
		}
		if c.activeDir != activePath {
			t.Errorf("activeDir = %q, want %q", c.activeDir, activePath)
		}
	})
}

func TestInitializeMetrics(t *testing.T) {
	t.Run("creates metrics.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}

		channelID := "test-channel-1"
		if err := c.InitializeMetrics(channelID); err != nil {
			t.Fatalf("InitializeMetrics failed: %v", err)
		}

		metricsFile := filepath.Join(tmpDir, "active", channelID, "metrics.json")
		if _, err := os.Stat(metricsFile); os.IsNotExist(err) {
			t.Fatal("metrics.json was not created")
		}

		// Verify contents
		m, err := c.GetMetrics(channelID)
		if err != nil {
			t.Fatal(err)
		}
		if m.SchemaVersion != SchemaVersion {
			t.Errorf("SchemaVersion = %q, want %q", m.SchemaVersion, SchemaVersion)
		}
		if m.ChannelID != channelID {
			t.Errorf("ChannelID = %q, want %q", m.ChannelID, channelID)
		}
	})

	t.Run("duplicate init returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}

		channelID := "test-channel-dup"
		if err := c.InitializeMetrics(channelID); err != nil {
			t.Fatal(err)
		}

		err = c.InitializeMetrics(channelID)
		if err == nil {
			t.Fatal("expected error on duplicate InitializeMetrics, got nil")
		}
	})
}

func TestRecordMessage(t *testing.T) {
	t.Run("records message and updates token usage", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		channelID := "msg-channel"
		if err := c.InitializeMetrics(channelID); err != nil {
			t.Fatal(err)
		}

		ts := time.Now()
		if err := c.RecordMessage(channelID, 1, 500, "agent-a", &ts); err != nil {
			t.Fatalf("RecordMessage failed: %v", err)
		}

		m, err := c.GetMetrics(channelID)
		if err != nil {
			t.Fatal(err)
		}

		if m.TokenUsage.TotalTokens != 500 {
			t.Errorf("TotalTokens = %d, want 500", m.TokenUsage.TotalTokens)
		}
		if m.TokenUsage.MessageCount != 1 {
			t.Errorf("MessageCount = %d, want 1", m.TokenUsage.MessageCount)
		}
		if m.TokenUsage.AverageTokens != 500.0 {
			t.Errorf("AverageTokens = %f, want 500.0", m.TokenUsage.AverageTokens)
		}
		if m.TokenUsage.MinTokens == nil || *m.TokenUsage.MinTokens != 500 {
			t.Errorf("MinTokens unexpected: %v", m.TokenUsage.MinTokens)
		}
		if m.TokenUsage.MaxTokens == nil || *m.TokenUsage.MaxTokens != 500 {
			t.Errorf("MaxTokens unexpected: %v", m.TokenUsage.MaxTokens)
		}
		if len(m.TokenUsage.Messages) != 1 {
			t.Fatalf("Messages length = %d, want 1", len(m.TokenUsage.Messages))
		}
		if m.TokenUsage.Messages[0].AgentID != "agent-a" {
			t.Errorf("AgentID = %q, want %q", m.TokenUsage.Messages[0].AgentID, "agent-a")
		}
		if m.Participants.Total != 1 {
			t.Errorf("Participants.Total = %d, want 1", m.Participants.Total)
		}
	})

	t.Run("multiple messages update averages and min/max", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		channelID := "multi-msg"
		if err := c.InitializeMetrics(channelID); err != nil {
			t.Fatal(err)
		}

		ts1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		ts2 := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC)
		ts3 := time.Date(2025, 1, 1, 10, 15, 0, 0, time.UTC)

		if err := c.RecordMessage(channelID, 1, 200, "agent-a", &ts1); err != nil {
			t.Fatal(err)
		}
		if err := c.RecordMessage(channelID, 2, 800, "agent-b", &ts2); err != nil {
			t.Fatal(err)
		}
		if err := c.RecordMessage(channelID, 3, 500, "agent-a", &ts3); err != nil {
			t.Fatal(err)
		}

		m, err := c.GetMetrics(channelID)
		if err != nil {
			t.Fatal(err)
		}

		if m.TokenUsage.TotalTokens != 1500 {
			t.Errorf("TotalTokens = %d, want 1500", m.TokenUsage.TotalTokens)
		}
		if m.TokenUsage.MessageCount != 3 {
			t.Errorf("MessageCount = %d, want 3", m.TokenUsage.MessageCount)
		}
		if m.TokenUsage.AverageTokens != 500.0 {
			t.Errorf("AverageTokens = %f, want 500.0", m.TokenUsage.AverageTokens)
		}
		if m.TokenUsage.MinTokens == nil || *m.TokenUsage.MinTokens != 200 {
			t.Errorf("MinTokens = %v, want 200", m.TokenUsage.MinTokens)
		}
		if m.TokenUsage.MaxTokens == nil || *m.TokenUsage.MaxTokens != 800 {
			t.Errorf("MaxTokens = %v, want 800", m.TokenUsage.MaxTokens)
		}

		// Check budget violations (800 < 2000, so none)
		if m.TokenUsage.BudgetViolations != 0 {
			t.Errorf("BudgetViolations = %d, want 0", m.TokenUsage.BudgetViolations)
		}

		// Check participants
		if m.Participants.Total != 2 {
			t.Errorf("Participants.Total = %d, want 2", m.Participants.Total)
		}

		// Check response time intervals exist
		if len(m.ResponseTimes.ResponseIntervals) != 2 {
			t.Errorf("ResponseIntervals length = %d, want 2", len(m.ResponseTimes.ResponseIntervals))
		}
		if m.ResponseTimes.AverageResponseMinutes == nil {
			t.Error("AverageResponseMinutes is nil")
		}
	})

	t.Run("budget violation is tracked", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		channelID := "budget-channel"
		if err := c.InitializeMetrics(channelID); err != nil {
			t.Fatal(err)
		}

		ts := time.Now()
		if err := c.RecordMessage(channelID, 1, 3000, "agent-a", &ts); err != nil {
			t.Fatal(err)
		}

		m, err := c.GetMetrics(channelID)
		if err != nil {
			t.Fatal(err)
		}
		if m.TokenUsage.BudgetViolations != 1 {
			t.Errorf("BudgetViolations = %d, want 1", m.TokenUsage.BudgetViolations)
		}
	})
}

func TestRecordStatusChange(t *testing.T) {
	t.Run("records transition", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		channelID := "status-channel"
		if err := c.InitializeMetrics(channelID); err != nil {
			t.Fatal(err)
		}

		from := "open"
		ts := time.Now()
		if err := c.RecordStatusChange(channelID, &from, "in-progress", &ts); err != nil {
			t.Fatal(err)
		}

		m, err := c.GetMetrics(channelID)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.StatusTransitions) != 1 {
			t.Fatalf("StatusTransitions length = %d, want 1", len(m.StatusTransitions))
		}
		if m.StatusTransitions[0].To != "in-progress" {
			t.Errorf("To = %q, want %q", m.StatusTransitions[0].To, "in-progress")
		}
		if m.StatusTransitions[0].From == nil || *m.StatusTransitions[0].From != "open" {
			t.Errorf("From = %v, want %q", m.StatusTransitions[0].From, "open")
		}
	})

	t.Run("consensus-reached triggers calculation", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		channelID := "consensus-channel"
		if err := c.InitializeMetrics(channelID); err != nil {
			t.Fatal(err)
		}

		// Record a message first so consensus calculation has data
		msgTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
		if err := c.RecordMessage(channelID, 1, 100, "agent-a", &msgTime); err != nil {
			t.Fatal(err)
		}

		// Now trigger consensus-reached 30 minutes later
		consensusTime := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)
		from := "in-progress"
		if err := c.RecordStatusChange(channelID, &from, "consensus-reached", &consensusTime); err != nil {
			t.Fatal(err)
		}

		m, err := c.GetMetrics(channelID)
		if err != nil {
			t.Fatal(err)
		}
		if m.Consensus.Status == nil || *m.Consensus.Status != "consensus-reached" {
			t.Errorf("Consensus.Status = %v, want %q", m.Consensus.Status, "consensus-reached")
		}
		if m.Consensus.TimeToConsensusMinutes == nil {
			t.Fatal("TimeToConsensusMinutes is nil")
		}
		if *m.Consensus.TimeToConsensusMinutes != 30.0 {
			t.Errorf("TimeToConsensusMinutes = %f, want 30.0", *m.Consensus.TimeToConsensusMinutes)
		}
		if m.Consensus.MessagesToConsensus == nil || *m.Consensus.MessagesToConsensus != 1 {
			t.Errorf("MessagesToConsensus = %v, want 1", m.Consensus.MessagesToConsensus)
		}
	})
}

func TestAddReviewScore(t *testing.T) {
	t.Run("adds score and updates average", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		channelID := "review-channel"
		if err := c.InitializeMetrics(channelID); err != nil {
			t.Fatal(err)
		}

		if err := c.AddReviewScore(channelID, 4.0); err != nil {
			t.Fatal(err)
		}

		m, err := c.GetMetrics(channelID)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Consensus.ReviewScores) != 1 {
			t.Fatalf("ReviewScores length = %d, want 1", len(m.Consensus.ReviewScores))
		}
		if m.Consensus.ReviewScores[0] != 4.0 {
			t.Errorf("ReviewScores[0] = %f, want 4.0", m.Consensus.ReviewScores[0])
		}
		if m.Consensus.AverageScore == nil || *m.Consensus.AverageScore != 4.0 {
			t.Errorf("AverageScore = %v, want 4.0", m.Consensus.AverageScore)
		}

		// Add a second score
		if err := c.AddReviewScore(channelID, 2.0); err != nil {
			t.Fatal(err)
		}

		m, err = c.GetMetrics(channelID)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Consensus.ReviewScores) != 2 {
			t.Fatalf("ReviewScores length = %d, want 2", len(m.Consensus.ReviewScores))
		}
		if m.Consensus.AverageScore == nil || *m.Consensus.AverageScore != 3.0 {
			t.Errorf("AverageScore = %v, want 3.0", m.Consensus.AverageScore)
		}
	})
}

func TestGetMetrics(t *testing.T) {
	t.Run("reads back what was written", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}
		channelID := "get-metrics-channel"
		if err := c.InitializeMetrics(channelID); err != nil {
			t.Fatal(err)
		}

		// Record some data
		ts1 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
		ts2 := time.Date(2025, 6, 1, 12, 10, 0, 0, time.UTC)
		if err := c.RecordMessage(channelID, 1, 300, "agent-x", &ts1); err != nil {
			t.Fatal(err)
		}
		if err := c.RecordMessage(channelID, 2, 700, "agent-y", &ts2); err != nil {
			t.Fatal(err)
		}
		if err := c.AddReviewScore(channelID, 5.0); err != nil {
			t.Fatal(err)
		}

		m, err := c.GetMetrics(channelID)
		if err != nil {
			t.Fatal(err)
		}

		// Verify all the accumulated data
		if m.ChannelID != channelID {
			t.Errorf("ChannelID = %q, want %q", m.ChannelID, channelID)
		}
		if m.TokenUsage.TotalTokens != 1000 {
			t.Errorf("TotalTokens = %d, want 1000", m.TokenUsage.TotalTokens)
		}
		if m.TokenUsage.MessageCount != 2 {
			t.Errorf("MessageCount = %d, want 2", m.TokenUsage.MessageCount)
		}
		if m.Participants.Total != 2 {
			t.Errorf("Participants.Total = %d, want 2", m.Participants.Total)
		}
		if len(m.Consensus.ReviewScores) != 1 || m.Consensus.ReviewScores[0] != 5.0 {
			t.Errorf("ReviewScores = %v, want [5.0]", m.Consensus.ReviewScores)
		}
		if m.ResponseTimes.AverageResponseMinutes == nil {
			t.Error("AverageResponseMinutes is nil, expected a value")
		} else if *m.ResponseTimes.AverageResponseMinutes != 10.0 {
			t.Errorf("AverageResponseMinutes = %f, want 10.0", *m.ResponseTimes.AverageResponseMinutes)
		}
	})

	t.Run("error for nonexistent channel", func(t *testing.T) {
		tmpDir := t.TempDir()
		c, err := NewCollector(tmpDir)
		if err != nil {
			t.Fatal(err)
		}

		_, err = c.GetMetrics("nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent channel, got nil")
		}
	})
}

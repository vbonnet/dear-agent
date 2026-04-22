package bdd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// TestSPECInvariants_TrustProtocol validates SPEC-trust-protocol.md invariants
// against the actual implementation.
func TestSPECInvariants_TrustProtocol(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { contracts.ResetForTesting() })
	contracts.ResetForTesting()

	t.Run("Invariant1_ScoreAlwaysIn0To100", func(t *testing.T) {
		session := "inv1-clamp"
		for i := 0; i < 20; i++ {
			ops.TrustRecord(nil, &ops.TrustRecordRequest{
				SessionName: session,
				EventType:   "false_completion",
				Detail:      fmt.Sprintf("clamp test %d", i),
			})
		}
		result, err := ops.TrustScore(nil, &ops.TrustScoreRequest{SessionName: session})
		if err != nil {
			t.Fatal(err)
		}
		if result.Score < 0 || result.Score > 100 {
			t.Errorf("score %d is outside [0, 100]", result.Score)
		}
		if result.Score != 0 {
			t.Errorf("expected score clamped to 0, got %d", result.Score)
		}

		session2 := "inv1-clamp-high"
		for i := 0; i < 50; i++ {
			ops.TrustRecord(nil, &ops.TrustRecordRequest{
				SessionName: session2,
				EventType:   "success",
				Detail:      fmt.Sprintf("clamp test %d", i),
			})
		}
		result2, err := ops.TrustScore(nil, &ops.TrustScoreRequest{SessionName: session2})
		if err != nil {
			t.Fatal(err)
		}
		if result2.Score > 100 {
			t.Errorf("score %d exceeds 100", result2.Score)
		}
	})

	t.Run("Invariant2_BaseScoreIs50", func(t *testing.T) {
		result, err := ops.TrustScore(nil, &ops.TrustScoreRequest{SessionName: "inv2-base"})
		if err != nil {
			t.Fatal(err)
		}
		if result.Score != 50 {
			t.Errorf("expected base score 50, got %d", result.Score)
		}
	})

	t.Run("Invariant3_EventsAreAppendOnly", func(t *testing.T) {
		session := "inv3-append"
		ops.TrustRecord(nil, &ops.TrustRecordRequest{
			SessionName: session, EventType: "success",
		})
		ops.TrustRecord(nil, &ops.TrustRecordRequest{
			SessionName: session, EventType: "stall",
		})
		result, err := ops.TrustHistory(nil, &ops.TrustHistoryRequest{SessionName: session})
		if err != nil {
			t.Fatal(err)
		}
		if result.Total != 2 {
			t.Errorf("expected 2 events, got %d", result.Total)
		}
		if len(result.Events) >= 2 {
			if result.Events[0].Timestamp.After(result.Events[1].Timestamp) {
				t.Error("events are not in chronological order")
			}
		}
	})

	t.Run("Invariant4_GCArchivedZeroImpact", func(t *testing.T) {
		session := "inv4-gc"
		ops.TrustRecord(nil, &ops.TrustRecordRequest{
			SessionName: session, EventType: "gc_archived",
		})
		result, err := ops.TrustScore(nil, &ops.TrustScoreRequest{SessionName: session})
		if err != nil {
			t.Fatal(err)
		}
		if result.Score != 50 {
			t.Errorf("gc_archived should have zero impact; expected 50, got %d", result.Score)
		}
	})

	t.Run("Invariant5_FalseCompletionIsHeaviestPenalty", func(t *testing.T) {
		slo := contracts.Defaults()
		deltas := slo.TrustProtocol.EventDeltas
		falseCompDelta := deltas["false_completion"]
		for eventType, delta := range deltas {
			if delta < falseCompDelta {
				t.Errorf("false_completion (%d) should be heaviest penalty but %s has %d",
					falseCompDelta, eventType, delta)
			}
		}
	})

	t.Run("Invariant6_TrustRecordingNeverBlocksOps", func(t *testing.T) {
		_, err := ops.TrustRecord(nil, &ops.TrustRecordRequest{
			SessionName: "", EventType: "success",
		})
		if err == nil {
			t.Error("expected error for empty session name")
		}
	})

	t.Run("Invariant7_LeaderboardIncludesAllSessions", func(t *testing.T) {
		ops.TrustRecord(nil, &ops.TrustRecordRequest{
			SessionName: "inv7-active", EventType: "success",
		})
		ops.TrustRecord(nil, &ops.TrustRecordRequest{
			SessionName: "inv7-archived", EventType: "gc_archived",
		})
		result, err := ops.TrustLeaderboard(nil)
		if err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, entry := range result.Entries {
			found[entry.SessionName] = true
		}
		if !found["inv7-active"] || !found["inv7-archived"] {
			t.Error("leaderboard should include all sessions with trust data")
		}
	})
}

// TestSPECInvariants_AuditTrail validates SPEC-audit-trail.md invariants.
func TestSPECInvariants_AuditTrail(t *testing.T) {
	tmpDir := t.TempDir()
	contracts.ResetForTesting()

	t.Run("Invariant1_AppendOnly", func(t *testing.T) {
		logPath := filepath.Join(tmpDir, "audit-append.jsonl")
		logger, err := ops.NewAuditLogger(logPath)
		if err != nil {
			t.Fatal(err)
		}
		logger.Log(ops.AuditEvent{Command: "session.new", Result: "success", DurationMs: 100})
		logger.Log(ops.AuditEvent{Command: "session.archive", Result: "success", DurationMs: 200})

		events, err := ops.ReadRecentEvents(logPath, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
	})

	t.Run("Invariant2_TimestampAlwaysSet", func(t *testing.T) {
		logPath := filepath.Join(tmpDir, "audit-ts.jsonl")
		logger, err := ops.NewAuditLogger(logPath)
		if err != nil {
			t.Fatal(err)
		}
		logger.Log(ops.AuditEvent{Command: "test", Result: "success", DurationMs: 1})

		events, err := ops.ReadRecentEvents(logPath, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 0 {
			t.Fatal("no events written")
		}
		if events[0].Timestamp.IsZero() {
			t.Error("timestamp should be auto-set, got zero")
		}
	})

	t.Run("Invariant4_ReadToleratesCorruption", func(t *testing.T) {
		logPath := filepath.Join(tmpDir, "audit-corrupt.jsonl")
		f, _ := os.Create(logPath)
		validEvent := ops.AuditEvent{
			Timestamp: time.Now(), Command: "test1", Result: "success", DurationMs: 1,
		}
		data, _ := json.Marshal(validEvent)
		f.Write(append(data, '\n'))
		f.WriteString("THIS IS NOT JSON\n")
		validEvent2 := ops.AuditEvent{
			Timestamp: time.Now(), Command: "test2", Result: "success", DurationMs: 2,
		}
		data2, _ := json.Marshal(validEvent2)
		f.Write(append(data2, '\n'))
		f.Close()

		events, err := ops.ReadRecentEvents(logPath, 0)
		if err != nil {
			t.Fatalf("should tolerate corruption, got: %v", err)
		}
		if len(events) != 2 {
			t.Errorf("expected 2 valid events (corrupt line skipped), got %d", len(events))
		}
	})

	t.Run("Invariant5_DefaultPathDeterministic", func(t *testing.T) {
		os.Setenv("HOME", tmpDir)
		t.Cleanup(func() { contracts.ResetForTesting() })
		logger, err := ops.NewAuditLogger("")
		if err != nil {
			t.Fatal(err)
		}
		expected := filepath.Join(tmpDir, ".agm", "logs", "audit.jsonl")
		if logger.FilePath() != expected {
			t.Errorf("expected %q, got %q", expected, logger.FilePath())
		}
	})

	t.Run("Invariant6_SearchIsSubstringBased", func(t *testing.T) {
		logPath := filepath.Join(tmpDir, "audit-search.jsonl")
		logger, err := ops.NewAuditLogger(logPath)
		if err != nil {
			t.Fatal(err)
		}
		logger.Log(ops.AuditEvent{Command: "session.new", Session: "my-worker", Result: "success", DurationMs: 1})
		logger.Log(ops.AuditEvent{Command: "session.archive", Session: "my-worker", Result: "success", DurationMs: 1})
		logger.Log(ops.AuditEvent{Command: "trust.record", Session: "other", Result: "success", DurationMs: 1})

		results, err := ops.SearchEvents(logPath, ops.AuditSearchParams{Command: "session"})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 session commands, got %d", len(results))
		}

		results2, err := ops.SearchEvents(logPath, ops.AuditSearchParams{Session: "worker"})
		if err != nil {
			t.Fatal(err)
		}
		if len(results2) != 2 {
			t.Errorf("expected 2 worker session events, got %d", len(results2))
		}
	})

	t.Run("Invariant7_EventsOrderedByWriteTime", func(t *testing.T) {
		logPath := filepath.Join(tmpDir, "audit-order.jsonl")
		logger, err := ops.NewAuditLogger(logPath)
		if err != nil {
			t.Fatal(err)
		}
		logger.Log(ops.AuditEvent{Command: "first", Result: "success", DurationMs: 1})
		logger.Log(ops.AuditEvent{Command: "second", Result: "success", DurationMs: 1})
		logger.Log(ops.AuditEvent{Command: "third", Result: "success", DurationMs: 1})

		events, err := ops.ReadRecentEvents(logPath, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 2 {
			t.Fatalf("expected 2 events, got %d", len(events))
		}
		if events[0].Command != "second" || events[1].Command != "third" {
			t.Errorf("expected [second, third], got [%s, %s]", events[0].Command, events[1].Command)
		}
	})
}

// TestSPECInvariants_ScanLoop validates SPEC-scan-loop.md invariants.
func TestSPECInvariants_ScanLoop(t *testing.T) {
	contracts.ResetForTesting()

	t.Run("Invariant3_AutoApproveOnlyRBACAllowlist", func(t *testing.T) {
		cfg := ops.DefaultCrossCheckConfig()
		expected := []string{"Read", "Glob", "Grep", "Write", "Edit"}
		for _, pattern := range expected {
			found := false
			for _, entry := range cfg.RBACAllowlist {
				if strings.Contains(entry, pattern) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("RBAC allowlist missing %q", pattern)
			}
		}
		dangerous := []string{"rm", "sudo", "kill", "chmod"}
		for _, pattern := range dangerous {
			for _, entry := range cfg.RBACAllowlist {
				if strings.Contains(strings.ToLower(entry), pattern) {
					t.Errorf("RBAC allowlist should not contain %q (found %q)", pattern, entry)
				}
			}
		}
	})

	t.Run("Invariant5_HealthStatusEscalation", func(t *testing.T) {
		cfg := ops.DefaultCrossCheckConfig()
		state := ops.DetectSessionState("normal output here", false, time.Now(), cfg)
		if state != ops.StateHealthy {
			t.Errorf("expected HEALTHY for normal output, got %v", state)
		}
	})

	t.Run("SLOContractValues", func(t *testing.T) {
		slo := contracts.Defaults()
		if slo.ScanLoop.DefaultScanInterval.Duration != 5*time.Minute {
			t.Errorf("expected 5m scan interval, got %v", slo.ScanLoop.DefaultScanInterval.Duration)
		}
		if slo.ScanLoop.StuckTimeout.Duration != 5*time.Minute {
			t.Errorf("expected 5m stuck timeout, got %v", slo.ScanLoop.StuckTimeout.Duration)
		}
		if slo.ScanLoop.ScanGapTimeout.Duration != 10*time.Minute {
			t.Errorf("expected 10m scan gap, got %v", slo.ScanLoop.ScanGapTimeout.Duration)
		}
		if slo.ScanLoop.WorkerCommitLookback.Duration != 24*time.Hour {
			t.Errorf("expected 24h lookback, got %v", slo.ScanLoop.WorkerCommitLookback.Duration)
		}
		if slo.ScanLoop.TmuxCaptureDepth != 30 {
			t.Errorf("expected capture depth 30, got %d", slo.ScanLoop.TmuxCaptureDepth)
		}
		if slo.ScanLoop.SessionListLimit != 1000 {
			t.Errorf("expected list limit 1000, got %d", slo.ScanLoop.SessionListLimit)
		}
	})
}

// TestSPECInvariants_StallDetection validates SPEC-stall-detection.md invariants.
func TestSPECInvariants_StallDetection(t *testing.T) {
	contracts.ResetForTesting()

	t.Run("Invariant6_PermissionPromptIsCritical", func(t *testing.T) {
		slo := contracts.Defaults()
		if slo.StallDetection.PermissionTimeout.Duration != 5*time.Minute {
			t.Errorf("expected 5m permission timeout, got %v",
				slo.StallDetection.PermissionTimeout.Duration)
		}
	})

	t.Run("SLOContractValues", func(t *testing.T) {
		slo := contracts.Defaults()
		if slo.StallDetection.NoCommitTimeout.Duration != 15*time.Minute {
			t.Errorf("expected 15m no-commit timeout, got %v",
				slo.StallDetection.NoCommitTimeout.Duration)
		}
		if slo.StallDetection.ErrorRepeatThreshold != 3 {
			t.Errorf("expected error threshold 3, got %d",
				slo.StallDetection.ErrorRepeatThreshold)
		}
		if slo.StallDetection.TmuxCaptureDepth != 50 {
			t.Errorf("expected capture depth 50, got %d",
				slo.StallDetection.TmuxCaptureDepth)
		}
		if slo.StallDetection.ErrorMessageMaxLength != 100 {
			t.Errorf("expected error msg max 100, got %d",
				slo.StallDetection.ErrorMessageMaxLength)
		}
	})
}

// TestSPECInvariants_SessionLifecycle validates SPEC-session-lifecycle.md invariants.
func TestSPECInvariants_SessionLifecycle(t *testing.T) {
	contracts.ResetForTesting()

	t.Run("Invariant3_SessionIdentifiersArePathSafe", func(t *testing.T) {
		slo := contracts.Defaults()
		for _, role := range slo.SessionLifecycle.GCProtectedRoles {
			if strings.Contains(role, "/") || strings.Contains(role, "\\") || strings.Contains(role, "..") {
				t.Errorf("protected role %q contains path-unsafe characters", role)
			}
		}
	})

	t.Run("SLOContractValues", func(t *testing.T) {
		slo := contracts.Defaults()
		if slo.SessionLifecycle.ResumeReadyTimeout.Duration != 5*time.Second {
			t.Errorf("expected 5s resume timeout, got %v",
				slo.SessionLifecycle.ResumeReadyTimeout.Duration)
		}
		if slo.SessionLifecycle.BloatSizeThresholdBytes != 100*1024*1024 {
			t.Errorf("expected 100MB bloat threshold, got %d",
				slo.SessionLifecycle.BloatSizeThresholdBytes)
		}
		if slo.SessionLifecycle.BloatProgressEntryThreshold != 1000 {
			t.Errorf("expected 1000 progress entries, got %d",
				slo.SessionLifecycle.BloatProgressEntryThreshold)
		}
		if slo.SessionLifecycle.ProcessKillGracePeriod.Duration != 2*time.Second {
			t.Errorf("expected 2s kill grace, got %v",
				slo.SessionLifecycle.ProcessKillGracePeriod.Duration)
		}
	})
}

// TestContractDrift runs the contract drift checker against the actual SPECs.
func TestContractDrift(t *testing.T) {
	contracts.ResetForTesting()

	specsDir := filepath.Join("..", "..", "docs", "specs")
	if _, err := os.Stat(specsDir); os.IsNotExist(err) {
		t.Skip("specs directory not found, skipping contract drift test")
	}

	result, err := ops.ContractDrift(nil, &ops.ContractDriftRequest{
		SpecsDir: specsDir,
	})
	if err != nil {
		t.Fatalf("contract drift check failed: %v", err)
	}

	if result.FailCount > 0 {
		for _, f := range result.Findings {
			if f.Severity == ops.DriftFail {
				t.Errorf("DRIFT FAIL [%s] %s: %s (expected=%q actual=%q)",
					f.SPECFile, f.Section, f.Detail, f.Expected, f.Actual)
			}
		}
	}

	t.Logf("Contract drift: %d specs, %d pass, %d warn, %d fail",
		result.TotalSpecs, result.PassCount, result.WarnCount, result.FailCount)
}

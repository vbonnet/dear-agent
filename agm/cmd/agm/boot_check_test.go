package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/monitoring"
)

func TestBootChecker_AllHealthy(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"orchestrator-v2", "overseer", "worker-1"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return []*monitoring.LoopHeartbeat{
				{Timestamp: time.Now(), Session: "orchestrator-v2", IntervalSecs: 300},
			}, nil
		},
	}

	result := checker.Check()

	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(result.Errors), result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}
}

func TestBootChecker_NoOrchestrator(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"overseer", "worker-1"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return nil, nil
		},
	}

	result := checker.Check()

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Component != "orchestrator" {
		t.Errorf("expected component 'orchestrator', got %q", result.Errors[0].Component)
	}
}

func TestBootChecker_NoOverseer(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"orchestrator-v2", "worker-1"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return nil, nil
		},
	}

	result := checker.Check()

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Component != "overseer" {
		t.Errorf("expected component 'overseer', got %q", result.Errors[0].Component)
	}
}

func TestBootChecker_BothMissing(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"worker-1", "worker-2"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return nil, nil
		},
	}

	result := checker.Check()

	if len(result.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(result.Errors))
	}
}

func TestBootChecker_NoSessions(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return nil, nil
		},
	}

	result := checker.Check()

	if len(result.Errors) != 2 {
		t.Fatalf("expected 2 errors (orchestrator + overseer), got %d", len(result.Errors))
	}
}

func TestBootChecker_TmuxError(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return nil, fmt.Errorf("tmux not running")
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return nil, nil
		},
	}

	result := checker.Check()

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Component != "tmux" {
		t.Errorf("expected component 'tmux', got %q", result.Errors[0].Component)
	}
}

func TestBootChecker_StaleHeartbeat(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"orchestrator-v2", "overseer"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return []*monitoring.LoopHeartbeat{
				{Timestamp: time.Now().Add(-10 * time.Minute), Session: "orchestrator-v2", IntervalSecs: 300},
				{Timestamp: time.Now(), Session: "overseer", IntervalSecs: 300},
			}, nil
		},
	}

	result := checker.Check()

	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.Warnings[0].Component != "heartbeat" {
		t.Errorf("expected component 'heartbeat', got %q", result.Warnings[0].Component)
	}
}

func TestBootChecker_HeartbeatReadError(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"orchestrator-v2", "overseer"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return nil, fmt.Errorf("permission denied")
		},
	}

	result := checker.Check()

	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.Warnings[0].Component != "heartbeat" {
		t.Errorf("expected component 'heartbeat', got %q", result.Warnings[0].Component)
	}
}

func TestBootChecker_NoHeartbeats(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"orchestrator-v2", "overseer"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return nil, nil
		},
	}

	result := checker.Check()

	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(result.Errors))
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(result.Warnings))
	}
}

func TestBootChecker_MetaOrchestratorCounts(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"meta-orchestrator", "overseer"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return nil, nil
		},
	}

	result := checker.Check()

	// meta-orchestrator contains "orchestrator", so should satisfy the check
	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors (meta-orchestrator should count), got %d: %v",
			len(result.Errors), result.Errors)
	}
}

func TestBootChecker_CaseInsensitiveMatch(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"Orchestrator-V2", "OVERSEER"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return nil, nil
		},
	}

	result := checker.Check()

	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors (case-insensitive match), got %d: %v",
			len(result.Errors), result.Errors)
	}
}

func TestBootChecker_MultipleStaleHeartbeats(t *testing.T) {
	checker := &BootChecker{
		ListSessions: func() ([]string, error) {
			return []string{"orchestrator-v2", "overseer"}, nil
		},
		ListHeartbeats: func(_ string) ([]*monitoring.LoopHeartbeat, error) {
			return []*monitoring.LoopHeartbeat{
				{Timestamp: time.Now().Add(-10 * time.Minute), Session: "session-a", IntervalSecs: 300},
				{Timestamp: time.Now().Add(-15 * time.Minute), Session: "session-b", IntervalSecs: 300},
			}, nil
		},
	}

	result := checker.Check()

	if len(result.Warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(result.Warnings))
	}
}

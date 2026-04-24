package budget

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestClassify(t *testing.T) {
	cfg := DefaultConfig(RoleWorker) // threshold=75, critical=90

	tests := []struct {
		name    string
		percent float64
		want    Level
	}{
		{"under threshold", 50.0, LevelOK},
		{"at threshold", 75.0, LevelWarning},
		{"above threshold", 80.0, LevelWarning},
		{"at critical", 90.0, LevelCritical},
		{"above critical", 95.0, LevelCritical},
		{"zero", 0.0, LevelOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classify(tt.percent, cfg)
			if got != tt.want {
				t.Errorf("classify(%.1f) = %s, want %s", tt.percent, got, tt.want)
			}
		})
	}
}

func TestDefaultThresholds(t *testing.T) {
	// Orchestrators should have lower threshold than workers
	orchCfg := DefaultConfig(RoleOrchestrator)
	workerCfg := DefaultConfig(RoleWorker)

	if orchCfg.ThresholdPercent >= workerCfg.ThresholdPercent {
		t.Errorf("orchestrator threshold (%.0f%%) should be lower than worker (%.0f%%)",
			orchCfg.ThresholdPercent, workerCfg.ThresholdPercent)
	}
}

func TestTrackerCheck(t *testing.T) {
	tracker := NewTracker()

	m := &manifest.Manifest{
		SessionID: "test-session-1",
		ContextUsage: &manifest.ContextUsage{
			UsedTokens:     80000,
			TotalTokens:    100000,
			PercentageUsed: 80.0,
		},
	}

	status := tracker.Check(m)

	if status.SessionID != "test-session-1" {
		t.Errorf("SessionID = %s, want test-session-1", status.SessionID)
	}
	if status.PercentageUsed != 80.0 {
		t.Errorf("PercentageUsed = %.1f, want 80.0", status.PercentageUsed)
	}
	if status.Level != LevelWarning {
		t.Errorf("Level = %s, want WARNING", status.Level)
	}
	if !status.IsOverThreshold() {
		t.Error("IsOverThreshold() should be true at 80%")
	}
}

func TestTrackerCheckNoUsage(t *testing.T) {
	tracker := NewTracker()

	m := &manifest.Manifest{
		SessionID: "no-usage",
	}

	status := tracker.Check(m)

	if status.PercentageUsed != 0 {
		t.Errorf("PercentageUsed = %.1f, want 0", status.PercentageUsed)
	}
	if status.Level != LevelOK {
		t.Errorf("Level = %s, want OK", status.Level)
	}
}

func TestInferRole(t *testing.T) {
	parent := "parent-id"

	tests := []struct {
		name string
		m    *manifest.Manifest
		want Role
	}{
		{
			"worker from parent",
			&manifest.Manifest{ParentSessionID: &parent},
			RoleWorker,
		},
		{
			"orchestrator from monitors",
			&manifest.Manifest{Monitors: []string{"m1"}},
			RoleOrchestrator,
		},
		{
			"role from tag",
			&manifest.Manifest{Context: manifest.Context{Tags: []string{"planner"}}},
			RolePlanner,
		},
		{
			"default",
			&manifest.Manifest{},
			RoleDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferRole(tt.m)
			if got != tt.want {
				t.Errorf("inferRole() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCheckAndAlert(t *testing.T) {
	cfg := DefaultConfig(RoleWorker)

	t.Run("no alert under threshold", func(t *testing.T) {
		status := Status{Level: LevelOK, PercentageUsed: 50.0}
		alert := CheckAndAlert(status, cfg, "")
		if alert != nil {
			t.Error("expected nil alert for OK level")
		}
	})

	t.Run("warning alert", func(t *testing.T) {
		status := Status{
			SessionID:      "s1",
			Level:          LevelWarning,
			PercentageUsed: 78.0,
			Threshold:      75.0,
			Role:           RoleWorker,
		}
		alert := CheckAndAlert(status, cfg, "")
		if alert == nil {
			t.Fatal("expected alert for WARNING level")
		}
		if alert.Level != LevelWarning {
			t.Errorf("alert level = %s, want WARNING", alert.Level)
		}
	})

	t.Run("critical writes signal", func(t *testing.T) {
		tmpDir := t.TempDir()
		signalDir := filepath.Join(tmpDir, "signals")

		cfg := Config{
			Role:              RoleWorker,
			ThresholdPercent:  75.0,
			CriticalPercent:   90.0,
			AutoCompactSignal: true,
		}

		status := Status{
			SessionID:      "s2",
			Level:          LevelCritical,
			PercentageUsed: 95.0,
			Threshold:      75.0,
			Role:           RoleWorker,
		}

		alert := CheckAndAlert(status, cfg, signalDir)
		if alert == nil {
			t.Fatal("expected alert for CRITICAL level")
		}

		signalPath := filepath.Join(signalDir, "compact-s2")
		if _, err := os.Stat(signalPath); os.IsNotExist(err) {
			t.Error("expected compact signal file to be created")
		}
	})
}

func TestTrackerCheckBatch(t *testing.T) {
	tracker := NewTracker()

	manifests := []*manifest.Manifest{
		{
			SessionID: "low",
			ContextUsage: &manifest.ContextUsage{
				PercentageUsed: 30.0,
				LastUpdated:    time.Now(),
			},
		},
		{
			SessionID: "high",
			ContextUsage: &manifest.ContextUsage{
				PercentageUsed: 85.0,
				LastUpdated:    time.Now(),
			},
		},
	}

	results := tracker.CheckBatch(manifests)

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Level != LevelOK {
		t.Errorf("low session level = %s, want OK", results[0].Level)
	}
	if results[1].Level != LevelWarning {
		t.Errorf("high session level = %s, want WARNING", results[1].Level)
	}
}

func TestRoleFromString(t *testing.T) {
	if RoleFromString("orchestrator") != RoleOrchestrator {
		t.Error("expected RoleOrchestrator")
	}
	if RoleFromString("unknown") != RoleDefault {
		t.Error("expected RoleDefault for unknown")
	}
}

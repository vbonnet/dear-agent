package sweeper

import (
	"log/slog"
	"os"
	"syscall"
	"testing"
	"time"
)

type mockTmux struct {
	sessions []string
	err      error
}

func (m *mockTmux) ListSessions() ([]string, error) {
	return m.sessions, m.err
}

func newTestSweeper(cfg Config, sessions []string) *Sweeper {
	s := New(cfg, &mockTmux{sessions: sessions}, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	return s
}

func TestNewDefaultsNilLogger(t *testing.T) {
	s := New(Config{}, &mockTmux{}, nil)
	if s.logger == nil {
		t.Fatal("New() logger = nil, want slog.Default()")
	}
}

func TestSweep_DisabledReturnsNil(t *testing.T) {
	s := newTestSweeper(Config{Enabled: false}, nil)
	if result := s.Sweep(); result != nil {
		t.Errorf("expected nil when disabled, got %+v", result)
	}
}

func TestSweep_RateLimited(t *testing.T) {
	cfg := Config{
		Enabled:       true,
		DryRun:        true,
		SweepInterval: 5 * time.Minute,
	}
	s := newTestSweeper(cfg, nil)
	s.lastSweep = time.Now()

	if result := s.Sweep(); result != nil {
		t.Errorf("expected nil when rate-limited, got %+v", result)
	}
}

func TestSweepDeadPaneSessions_DryRun(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DryRun:  true,
	}
	s := newTestSweeper(cfg, []string{"worker-1", "worker-2"})

	killedSessions := make(map[string]bool)
	s.killSession = func(name string) {
		killedSessions[name] = true
	}
	s.getPanePID = func(name string) (int, error) {
		if name == "worker-1" {
			return 12345, nil
		}
		return 99999, nil
	}
	s.checkProcess = func(pid int) error {
		if pid == 12345 {
			return syscall.ESRCH
		}
		return nil
	}

	result := &Result{DryRun: true}
	s.sweepDeadPaneSessions(result)

	if len(result.DeadPaneSessions) != 1 {
		t.Fatalf("expected 1 dead-pane session, got %d", len(result.DeadPaneSessions))
	}
	if result.DeadPaneSessions[0] != "worker-1" {
		t.Errorf("expected worker-1, got %s", result.DeadPaneSessions[0])
	}
	if killedSessions["worker-1"] {
		t.Error("dry-run should not kill sessions")
	}
}

func TestSweepDeadPaneSessions_KillsWhenNotDryRun(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DryRun:  false,
	}
	s := newTestSweeper(cfg, []string{"dead-session"})

	killed := false
	s.killSession = func(name string) {
		if name == "dead-session" {
			killed = true
		}
	}
	s.getPanePID = func(_ string) (int, error) {
		return 12345, nil
	}
	s.checkProcess = func(_ int) error {
		return syscall.ESRCH
	}

	result := &Result{DryRun: false}
	s.sweepDeadPaneSessions(result)

	if !killed {
		t.Error("expected session to be killed when not dry-run")
	}
	if len(result.DeadPaneSessions) != 1 {
		t.Fatalf("expected 1 dead-pane session, got %d", len(result.DeadPaneSessions))
	}
}

func TestSweepDeadPaneSessions_SkipsProtected(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DryRun:  false,
	}
	s := newTestSweeper(cfg, []string{"orchestrator-main", "meta-orch", "human-session"})

	killed := make(map[string]bool)
	s.killSession = func(name string) {
		killed[name] = true
	}
	s.getPanePID = func(_ string) (int, error) {
		return 12345, nil
	}
	s.checkProcess = func(_ int) error {
		return syscall.ESRCH
	}

	result := &Result{}
	s.sweepDeadPaneSessions(result)

	if len(result.DeadPaneSessions) != 0 {
		t.Errorf("expected 0 dead-pane sessions (all protected), got %d: %v",
			len(result.DeadPaneSessions), result.DeadPaneSessions)
	}
	if len(killed) != 0 {
		t.Errorf("expected no kills for protected sessions, killed: %v", killed)
	}
}

func TestSweepDeadPaneSessions_SkipsLivePIDs(t *testing.T) {
	cfg := Config{
		Enabled: true,
		DryRun:  false,
	}
	s := newTestSweeper(cfg, []string{"live-session"})

	killed := false
	s.killSession = func(_ string) {
		killed = true
	}
	s.getPanePID = func(_ string) (int, error) {
		return 12345, nil
	}
	s.checkProcess = func(_ int) error {
		return nil
	}

	result := &Result{}
	s.sweepDeadPaneSessions(result)

	if killed {
		t.Error("should not kill sessions with live PIDs")
	}
	if len(result.DeadPaneSessions) != 0 {
		t.Errorf("expected 0 dead-pane sessions, got %d", len(result.DeadPaneSessions))
	}
}

func TestIsProtectedSession(t *testing.T) {
	s := newTestSweeper(Config{}, nil)

	tests := []struct {
		name     string
		expected bool
	}{
		{"orchestrator-main", true},
		{"orchestrator-v2-backup", true},
		{"meta-orchestrator", true},
		{"human-session", true},
		{"overseer-primary", true},
		{"worker-abc123", false},
		{"vroom-orchestrator", true},
		{"worker-ce-clm6", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.isProtectedSession(tt.name); got != tt.expected {
				t.Errorf("isProtectedSession(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("default should be disabled")
	}
	if !cfg.DryRun {
		t.Error("default should be dry-run")
	}
	if cfg.GracePeriod != 30*time.Minute {
		t.Errorf("expected 30m grace period, got %v", cfg.GracePeriod)
	}
	if cfg.SweepInterval != 5*time.Minute {
		t.Errorf("expected 5m sweep interval, got %v", cfg.SweepInterval)
	}
}

func TestParseDurations(t *testing.T) {
	cfg := Config{
		GracePeriodStr:   "1h",
		SweepIntervalStr: "10m",
	}
	if err := cfg.ParseDurations(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GracePeriod != time.Hour {
		t.Errorf("expected 1h, got %v", cfg.GracePeriod)
	}
	if cfg.SweepInterval != 10*time.Minute {
		t.Errorf("expected 10m, got %v", cfg.SweepInterval)
	}
}

func TestParseDurations_Invalid(t *testing.T) {
	cfg := Config{GracePeriodStr: "invalid"}
	if err := cfg.ParseDurations(); err == nil {
		t.Error("expected error for invalid duration")
	}
}

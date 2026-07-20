// Package sweeper detects and cleans up dead tmux sessions (pane process
// exited) and stuck AGM sessions (status '?' — no tmux, no lifecycle). It is
// designed to run inside the sentinel daemon's monitoring loop.
package sweeper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Config controls sweep behavior.
type Config struct {
	Enabled       bool          `yaml:"enabled"`
	DryRun        bool          `yaml:"dry_run"`
	GracePeriod   time.Duration `yaml:"-"`
	SweepInterval time.Duration `yaml:"-"`

	GracePeriodStr   string `yaml:"grace_period"`
	SweepIntervalStr string `yaml:"sweep_interval"`
}

// DefaultConfig returns safe defaults: disabled, dry-run, 30m grace, 5m interval.
func DefaultConfig() Config {
	return Config{
		Enabled:          false,
		DryRun:           true,
		GracePeriod:      30 * time.Minute,
		SweepInterval:    5 * time.Minute,
		GracePeriodStr:   "30m",
		SweepIntervalStr: "5m",
	}
}

// ParseDurations populates Duration fields from their string counterparts.
func (c *Config) ParseDurations() error {
	if c.GracePeriodStr != "" {
		d, err := time.ParseDuration(c.GracePeriodStr)
		if err != nil {
			return fmt.Errorf("invalid sweeper.grace_period: %w", err)
		}
		c.GracePeriod = d
	}
	if c.SweepIntervalStr != "" {
		d, err := time.ParseDuration(c.SweepIntervalStr)
		if err != nil {
			return fmt.Errorf("invalid sweeper.sweep_interval: %w", err)
		}
		c.SweepInterval = d
	}
	return nil
}

// TmuxLister enumerates tmux sessions and retrieves pane PIDs.
type TmuxLister interface {
	ListSessions() ([]string, error)
}

type tmuxPanePIDGetter interface {
	GetPanePID(sessionName string) (int, error)
}

type tmuxSessionKiller interface {
	KillSession(sessionName string) error
}

// PanePIDGetter returns the pane PID for a tmux session. Returns 0 and an
// error if the session or pane doesn't exist.
type PanePIDGetter func(sessionName string) (int, error)

// SessionKiller kills a tmux session by name.
type SessionKiller func(sessionName string)

// SessionArchiver archives an AGM session by name or ID.
type SessionArchiver func(identifier string, dryRun bool) error

// ProcessChecker tests whether a PID is alive. Returns nil if the process
// exists, syscall.ESRCH if it doesn't.
type ProcessChecker func(pid int) error

// agmSessionInfo is the subset of `agm session list --json` output we need.
type agmSessionInfo struct {
	Name      string `json:"name"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Lifecycle string `json:"lifecycle"`
	CreatedAt string `json:"created_at"`
}

// Result reports the outcome of a sweep pass.
type Result struct {
	DeadPaneSessions []string // tmux sessions killed (or would-kill if dry-run)
	StuckAGMSessions []string // AGM sessions archived (or would-archive if dry-run)
	DryRun           bool
	Errors           []string
}

// Sweeper periodically detects and cleans up dead/stuck sessions.
type Sweeper struct {
	cfg            Config
	tmux           TmuxLister
	getPanePID     PanePIDGetter
	killSession    SessionKiller
	archiveSession SessionArchiver
	checkProcess   ProcessChecker
	logger         *slog.Logger
	lastSweep      time.Time

	// supervisorPatterns are name substrings that identify protected sessions.
	supervisorPatterns []string
}

// New creates a Sweeper with the given dependencies.
func New(cfg Config, tmux TmuxLister, logger *slog.Logger) *Sweeper {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Sweeper{
		cfg:            cfg,
		tmux:           tmux,
		getPanePID:     defaultGetPanePID,
		killSession:    defaultKillSession,
		archiveSession: defaultArchiveSession,
		checkProcess:   defaultCheckProcess,
		logger:         logger,
		supervisorPatterns: []string{
			"orchestrator", "overseer", "meta-", "human",
		},
	}
	if getter, ok := tmux.(tmuxPanePIDGetter); ok {
		s.getPanePID = getter.GetPanePID
	}
	if killer, ok := tmux.(tmuxSessionKiller); ok {
		s.killSession = func(sessionName string) {
			if err := killer.KillSession(sessionName); err != nil {
				logger.Error("[sweeper] kill tmux session failed", "session", sessionName, "error", err)
			}
		}
	}
	return s
}

// Sweep runs one sweep pass. It is safe to call from the sentinel daemon's
// main loop tick. Returns early if the sweep interval hasn't elapsed or the
// sweeper is disabled.
func (s *Sweeper) Sweep() *Result {
	if !s.cfg.Enabled {
		return nil
	}
	if time.Since(s.lastSweep) < s.cfg.SweepInterval {
		return nil
	}
	s.lastSweep = time.Now()

	result := &Result{DryRun: s.cfg.DryRun}

	s.sweepDeadPaneSessions(result)
	s.sweepStuckAGMSessions(result)

	if len(result.DeadPaneSessions) > 0 || len(result.StuckAGMSessions) > 0 {
		s.logger.Info("[sweeper] sweep complete",
			"dead_pane_sessions", len(result.DeadPaneSessions),
			"stuck_agm_sessions", len(result.StuckAGMSessions),
			"dry_run", result.DryRun)
	}

	return result
}

// sweepDeadPaneSessions finds tmux sessions whose pane PID has exited and
// kills them.
func (s *Sweeper) sweepDeadPaneSessions(result *Result) {
	sessions, err := s.tmux.ListSessions()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("list tmux sessions: %v", err))
		return
	}

	for _, name := range sessions {
		pid, err := s.getPanePID(name)
		if err != nil {
			continue
		}
		if pid <= 0 {
			continue
		}

		if err := s.checkProcess(pid); err == nil {
			continue
		}

		if s.isProtectedSession(name) {
			s.logger.Warn("[sweeper] dead pane detected on protected session, skipping",
				"session", name, "pid", pid)
			continue
		}

		result.DeadPaneSessions = append(result.DeadPaneSessions, name)

		if s.cfg.DryRun {
			s.logger.Info("[sweeper] would kill dead-pane tmux session",
				"session", name, "dead_pid", pid)
			continue
		}

		s.logger.Info("[sweeper] killing dead-pane tmux session",
			"session", name, "dead_pid", pid)
		s.killSession(name)
	}
}

// sweepStuckAGMSessions finds AGM sessions with status "stopped" (no tmux,
// empty lifecycle) that exceed the grace period, and archives them.
func (s *Sweeper) sweepStuckAGMSessions(result *Result) {
	sessions, err := s.listAGMSessions()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("list AGM sessions: %v", err))
		return
	}

	now := time.Now()

	for _, sess := range sessions {
		if sess.Lifecycle != "" {
			continue
		}
		if sess.Status != "stopped" {
			continue
		}

		createdAt, err := time.Parse(time.RFC3339, sess.CreatedAt)
		if err != nil {
			continue
		}
		if now.Sub(createdAt) < s.cfg.GracePeriod {
			continue
		}
		if s.isProtectedSession(sess.Name) {
			s.logger.Warn("[sweeper] stuck session is protected, skipping",
				"session", sess.Name, "session_id", sess.SessionID)
			continue
		}

		result.StuckAGMSessions = append(result.StuckAGMSessions, sess.Name)

		if s.cfg.DryRun {
			s.logger.Info("[sweeper] would archive stuck AGM session",
				"session", sess.Name, "session_id", sess.SessionID,
				"age", now.Sub(createdAt).Round(time.Second))
			continue
		}

		s.logger.Info("[sweeper] archiving stuck AGM session",
			"session", sess.Name, "session_id", sess.SessionID,
			"age", now.Sub(createdAt).Round(time.Second))

		if err := s.archiveSession(sess.SessionID, false); err != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("archive %s: %v", sess.Name, err))
			s.logger.Error("[sweeper] archive failed",
				"session", sess.Name, "error", err)
		}
	}
}

func (s *Sweeper) isProtectedSession(name string) bool {
	lower := strings.ToLower(name)
	for _, pattern := range s.supervisorPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// listAGMSessions shells out to `agm session list --json` and returns parsed
// session info. This avoids a direct dolt dependency from the sentinel daemon.
func (s *Sweeper) listAGMSessions() ([]agmSessionInfo, error) {
	return s.listAGMSessionsFn()
}

func (s *Sweeper) listAGMSessionsFn() ([]agmSessionInfo, error) {
	cmd := exec.Command("agm", "session", "list", "--json", "--all")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agm session list: %w", err)
	}

	var sessions []agmSessionInfo
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, fmt.Errorf("parse agm session list: %w", err)
	}
	return sessions, nil
}

// defaultGetPanePID retrieves the pane PID for a tmux session.
func defaultGetPanePID(sessionName string) (int, error) {
	cmd := exec.Command("tmux", "list-panes", "-t", sessionName, "-F", "#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("pane not found: %w", err)
	}
	pidStr := strings.TrimSpace(string(out))
	if idx := strings.Index(pidStr, "\n"); idx != -1 {
		pidStr = pidStr[:idx]
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("parse PID %q: %w", pidStr, err)
	}
	return pid, nil
}

func defaultKillSession(sessionName string) {
	cmd := exec.Command("tmux", "kill-session", "-t", sessionName)
	_ = cmd.Run()
}

func defaultArchiveSession(identifier string, _ bool) error {
	cmd := exec.Command("agm", "session", "archive", "--async", identifier)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func defaultCheckProcess(pid int) error {
	return syscall.Kill(pid, 0)
}

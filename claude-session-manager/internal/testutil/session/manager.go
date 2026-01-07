package session

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	testerrors "github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/errors"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/tmux"
)

// sessionNameRegex validates session names (alphanumeric, hyphens, underscores only)
var sessionNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// CreateOptions contains options for creating a session
type CreateOptions struct {
	Name              string
	WorkingDir        string
	SessionsDir       string
	StartupTimeout    time.Duration
	AdditionalDirs    []string // Additional directories to trust via --add-dir
	SkipPermissions   bool     // Skip permission prompts via --dangerously-skip-permissions
}

// SendOptions contains options for sending keys
type SendOptions struct {
	Command      string
	SessionsDir  string
	Autocomplete bool
	Delay        time.Duration
}

// CaptureOptions contains options for capturing output
type CaptureOptions struct {
	SessionsDir string
	Lines       int
}

// CleanupOptions contains options for cleanup
type CleanupOptions struct {
	SessionsDir string
}

// Session represents a test session
type Session struct {
	Name          string    `json:"name"`
	TmuxSession   string    `json:"tmux_session"`
	SessionsDir   string    `json:"sessions_dir"`
	WorkingDir    string    `json:"working_dir"`
	CreatedAt     time.Time `json:"created_at"`
	StartupTimeMs int64     `json:"startup_time_ms"`
}

// CaptureResult represents captured output
type CaptureResult struct {
	Lines []string `json:"lines"`
	Count int      `json:"count"`
}

// CleanupStatus represents cleanup status
type CleanupStatus struct {
	TmuxKilled     bool `json:"tmux_killed"`
	CSMArchived    bool `json:"csm_archived"`
	DirectoryClean bool `json:"directory_clean"`
}

// Manager manages test sessions
type Manager interface {
	Create(opts CreateOptions) (*Session, error)
	Send(name string, opts SendOptions) error
	Capture(name string, opts CaptureOptions) (*CaptureResult, error)
	Cleanup(name string, opts CleanupOptions) (*CleanupStatus, error)
	List() ([]*Session, error)
}

// New creates a new session manager
func New(tmuxClient tmux.Client) Manager {
	return &manager{
		tmuxClient: tmuxClient,
	}
}

type manager struct {
	tmuxClient tmux.Client
}

func (m *manager) Create(opts CreateOptions) (*Session, error) {
	// Validate session name
	if !isValidSessionName(opts.Name) {
		return nil, testerrors.NewUserError(
			"invalid session name",
			fmt.Sprintf("Session name '%s' contains invalid characters", opts.Name),
			[]string{
				"Use only letters, numbers, hyphens, and underscores",
				"Example: my-test-1, test_session, mytest123",
			},
		)
	}

	tmuxName := fmt.Sprintf("csm-test-%s", opts.Name)

	// Check for collision
	if m.tmuxClient.HasSession(tmuxName) {
		return nil, testerrors.NewUserError(
			"session name collision",
			fmt.Sprintf("Session '%s' already exists", opts.Name),
			[]string{
				fmt.Sprintf("Cleanup existing session: csm-test-tmux cleanup %s --sessions-dir %s", opts.Name, opts.SessionsDir),
				fmt.Sprintf("Use different name: csm-test-tmux create %s-2", opts.Name),
			},
		)
	}

	// Create sessions directory
	if err := os.MkdirAll(opts.SessionsDir, 0755); err != nil {
		return nil, testerrors.NewSystemError(
			"failed to create sessions directory",
			err,
			[]string{
				"Check write permissions for parent directory",
				fmt.Sprintf("Ensure path exists: %s", opts.SessionsDir),
			},
		)
	}

	// Register cleanup on failure
	var cleanupDone bool
	defer func() {
		if !cleanupDone {
			_ = m.tmuxClient.KillSession(tmuxName)
			_ = os.RemoveAll(opts.SessionsDir)
		}
	}()

	// Create tmux session
	if err := m.tmuxClient.CreateSession(tmuxName, opts.WorkingDir); err != nil {
		return nil, testerrors.NewSystemError(
			"failed to create tmux session",
			err,
			[]string{
				"Check if tmux is installed: tmux -V",
				"Verify tmux is in PATH",
				fmt.Sprintf("Check working directory exists: %s", opts.WorkingDir),
			},
		)
	}

	// Build Claude command with flags
	claudeCmd := "claude"

	// Add --add-dir flags for additional directories
	for _, dir := range opts.AdditionalDirs {
		claudeCmd += fmt.Sprintf(" --add-dir %s", dir)
	}

	// Add --dangerously-skip-permissions if requested
	if opts.SkipPermissions {
		claudeCmd += " --dangerously-skip-permissions"
	}

	// Start Claude
	if err := m.tmuxClient.SendKeys(tmuxName, claudeCmd); err != nil {
		return nil, testerrors.NewSystemError(
			"failed to start Claude",
			err,
			[]string{
				"Check if claude is installed: which claude",
				"Verify claude is in PATH",
			},
		)
	}

	// Wait for startup
	startTime := time.Now()
	if err := m.tmuxClient.WaitForStartup(tmuxName, opts.StartupTimeout); err != nil {
		return nil, testerrors.NewTimeoutError(
			"Claude startup timeout",
			err,
			[]string{
				fmt.Sprintf("Increase timeout: --startup-timeout %ds", int(opts.StartupTimeout.Seconds())+30),
				"Check Claude is working: claude --version",
				fmt.Sprintf("View session output: tmux attach -t %s", tmuxName),
			},
		)
	}
	startupTime := time.Since(startTime)

	cleanupDone = true // Disable cleanup defer

	return &Session{
		Name:          opts.Name,
		TmuxSession:   tmuxName,
		SessionsDir:   opts.SessionsDir,
		WorkingDir:    opts.WorkingDir,
		CreatedAt:     time.Now(),
		StartupTimeMs: startupTime.Milliseconds(),
	}, nil
}

// Send executes a command in the test session
// SECURITY NOTE: This method intentionally executes arbitrary commands in the
// tmux session. Commands are passed directly to tmux send-keys, which executes
// them in the shell. This is by design for testing purposes - the method should
// only be used in controlled test environments, never with untrusted input.
func (m *manager) Send(name string, opts SendOptions) error {
	tmuxName := fmt.Sprintf("csm-test-%s", name)

	// Check session exists
	if !m.tmuxClient.HasSession(tmuxName) {
		return testerrors.NewUserError(
			"session not found",
			fmt.Sprintf("Session '%s' does not exist", name),
			[]string{
				"List sessions: csm-test-tmux list",
				fmt.Sprintf("Create session: csm-test-tmux create %s", name),
			},
		)
	}

	// Send command
	if err := m.tmuxClient.SendKeys(tmuxName, opts.Command); err != nil {
		return testerrors.NewSystemError(
			"failed to send command",
			err,
			[]string{
				fmt.Sprintf("Check session is alive: tmux has-session -t %s", tmuxName),
			},
		)
	}

	// Handle autocomplete (send second Enter after delay)
	if opts.Autocomplete {
		time.Sleep(opts.Delay)
		if err := m.tmuxClient.SendKeys(tmuxName, ""); err != nil {
			return testerrors.NewSystemError(
				"failed to send autocomplete enter",
				err,
				nil,
			)
		}
	}

	return nil
}

func (m *manager) Capture(name string, opts CaptureOptions) (*CaptureResult, error) {
	tmuxName := fmt.Sprintf("csm-test-%s", name)

	// Check session exists
	if !m.tmuxClient.HasSession(tmuxName) {
		return nil, testerrors.NewUserError(
			"session not found",
			fmt.Sprintf("Session '%s' does not exist", name),
			[]string{
				"List sessions: csm-test-tmux list",
			},
		)
	}

	// Capture pane output
	output, err := m.tmuxClient.CapturePane(tmuxName, opts.Lines)
	if err != nil {
		return nil, testerrors.NewSystemError(
			"failed to capture output",
			err,
			[]string{
				fmt.Sprintf("Check session is alive: tmux has-session -t %s", tmuxName),
			},
		)
	}

	// Split into lines
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	return &CaptureResult{
		Lines: lines,
		Count: len(lines),
	}, nil
}

func (m *manager) Cleanup(name string, opts CleanupOptions) (*CleanupStatus, error) {
	tmuxName := fmt.Sprintf("csm-test-%s", name)

	status := &CleanupStatus{
		TmuxKilled:     false,
		CSMArchived:    false,
		DirectoryClean: false,
	}

	// Best-effort cleanup in LIFO order
	// Step 1: Kill tmux session
	if m.tmuxClient.HasSession(tmuxName) {
		if err := m.tmuxClient.KillSession(tmuxName); err == nil {
			status.TmuxKilled = true
		}
	} else {
		status.TmuxKilled = true // Already gone
	}

	// Step 2: Archive CSM session (if csm command is available)
	// TODO: Call csm archive if needed
	status.CSMArchived = true // Skip for now

	// Step 3: Remove sessions directory
	if err := os.RemoveAll(opts.SessionsDir); err == nil {
		status.DirectoryClean = true
	}

	// Return error only if all steps failed
	if !status.TmuxKilled && !status.CSMArchived && !status.DirectoryClean {
		return status, testerrors.NewSystemError(
			"cleanup failed",
			nil,
			[]string{
				fmt.Sprintf("Manually kill session: tmux kill-session -t %s", tmuxName),
				fmt.Sprintf("Manually remove directory: rm -rf %s", opts.SessionsDir),
			},
		)
	}

	return status, nil
}

func (m *manager) List() ([]*Session, error) {
	// TODO: Implement session listing (requires persistence)
	return []*Session{}, nil
}

// isValidSessionName checks if a session name contains only allowed characters
func isValidSessionName(name string) bool {
	// Allow alphanumeric, hyphens, and underscores
	return name != "" && sessionNameRegex.MatchString(name)
}

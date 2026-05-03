package tmux

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// ControlModeSession represents a tmux control mode session
// Control mode allows programmatic control of tmux with parseable output
type ControlModeSession struct {
	cmd        *exec.Cmd
	Stdin      io.WriteCloser // Exported for direct access in InitSequence
	Stdout     io.ReadCloser  // Exported for OutputWatcher integration
	stderr     io.ReadCloser
	scanner    *bufio.Scanner
	ctx        context.Context
	cancel     context.CancelFunc
	socketPath string
}

// StartControlMode starts tmux in control mode (-C) for programmatic interaction
// Control mode provides a parseable interface with notifications like %end, %error
func StartControlMode(sessionName string) (*ControlModeSession, error) {
	return StartControlModeWithTimeout(sessionName, 30*time.Second)
}

// findSessionSocket finds which socket the session is on (dual-socket support)
// Returns the socket path if found, empty string if not found
func findSessionSocket(sessionName string) string {
	socketPaths := GetReadSocketPaths()
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)

	for _, socketPath := range socketPaths {
		// Check if session exists on this socket
		ctx := context.Background()
		_, err := RunWithTimeout(ctx, 2*time.Second, "tmux", "-S", socketPath, "has-session", "-t", FormatSessionTarget(normalizedName))
		if err == nil {
			// Found it!
			return socketPath
		}
	}

	// Not found on any socket - return write socket as fallback
	// (session might be brand new and about to be created)
	return GetSocketPath()
}

// StartControlModeWithTimeout starts control mode with a custom timeout
func StartControlModeWithTimeout(sessionName string, timeout time.Duration) (*ControlModeSession, error) {
	// Find which socket the session is on (dual-socket support)
	socketPath := findSessionSocket(sessionName)

	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedName := NormalizeTmuxSessionName(sessionName)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Build command: tmux -S /tmp/agm.sock -C attach-session -t <name>
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "-C", "attach-session", "-t", FormatSessionTarget(normalizedName))

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		stdinPipe.Close()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		stdinPipe.Close()
		stdoutPipe.Close()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start control mode: %w", err)
	}

	session := &ControlModeSession{
		cmd:        cmd,
		Stdin:      stdinPipe,
		Stdout:     stdoutPipe,
		stderr:     stderrPipe,
		scanner:    bufio.NewScanner(stdoutPipe),
		ctx:        ctx,
		cancel:     cancel,
		socketPath: socketPath,
	}

	// Wait for initial output to confirm control mode started
	if err := session.waitForReady(2 * time.Second); err != nil {
		session.Close()
		return nil, fmt.Errorf("control mode not ready: %w", err)
	}

	return session, nil
}

// waitForReady waits for control mode to be ready by reading initial output
func (c *ControlModeSession) waitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Set read deadline on scanner
		if c.scanner.Scan() {
			line := c.scanner.Text()
			// Control mode outputs various notifications on startup
			// We just need to see ANY output to confirm it's working
			if strings.HasPrefix(line, "%") {
				return nil // Control mode is ready
			}
		}

		if err := c.scanner.Err(); err != nil {
			return fmt.Errorf("scanner error: %w", err)
		}

		// Small delay before retry
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for control mode to be ready")
}

// SendCommand sends a command to tmux and waits for completion
// Returns nil on success (%end received), error on failure or timeout
func (c *ControlModeSession) SendCommand(command string) error {
	return c.SendCommandWithTimeout(command, 5*time.Second)
}

// SendCommandWithTimeout sends a command with a custom timeout
func (c *ControlModeSession) SendCommandWithTimeout(command string, timeout time.Duration) error {
	// Send command to stdin
	if _, err := fmt.Fprintf(c.Stdin, "%s\n", command); err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	// Wait for %end notification
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check context cancellation
		select {
		case <-c.ctx.Done():
			return fmt.Errorf("context cancelled: %w", c.ctx.Err())
		default:
		}

		// Read next line
		if !c.scanner.Scan() {
			if err := c.scanner.Err(); err != nil {
				return fmt.Errorf("scanner error: %w", err)
			}
			// EOF reached
			break
		}

		line := c.scanner.Text()

		// Check for %end (command completed successfully)
		if strings.HasPrefix(line, "%end") {
			return nil
		}

		// Check for %error (command failed)
		if strings.HasPrefix(line, "%error") {
			return fmt.Errorf("tmux error: %s", line)
		}

		// Other control mode notifications are ignored
		// Continue reading until %end or timeout
	}

	return fmt.Errorf("timeout waiting for command completion (%v)", timeout)
}

// SendKeys sends keys to a tmux pane (equivalent to send-keys command)
func (c *ControlModeSession) SendKeys(target, keys string) error {
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedTarget := NormalizeTmuxSessionName(target)
	// Note: send-keys targets panes, not sessions, so we don't use FormatSessionTarget (=prefix)
	command := fmt.Sprintf("send-keys -t %s %s", normalizedTarget, keys)
	return c.SendCommand(command)
}

// SendKeysLiteral sends literal text to a tmux pane followed by Enter
// IMPORTANT: -l and C-m must be sent as SEPARATE commands, otherwise C-m
// is interpreted as literal text "C-m" instead of the Enter key.
// See: https://github.com/tmux/tmux/issues/1778
func (c *ControlModeSession) SendKeysLiteral(target, text string) error {
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedTarget := NormalizeTmuxSessionName(target)
	// Send text in literal mode first
	cmd1 := fmt.Sprintf("send-keys -t %s -l %q", normalizedTarget, text)
	if err := c.SendCommand(cmd1); err != nil {
		return fmt.Errorf("failed to send literal text: %w", err)
	}

	// Send Enter separately (control mode waits for %end, so no delay needed)
	cmd2 := fmt.Sprintf("send-keys -t %s C-m", normalizedTarget)
	if err := c.SendCommand(cmd2); err != nil {
		return fmt.Errorf("failed to send Enter key: %w", err)
	}

	return nil
}

// ReadOutput reads and returns the next line of output
// This is useful for capturing tmux notifications
func (c *ControlModeSession) ReadOutput(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-c.ctx.Done():
			return "", fmt.Errorf("context cancelled: %w", c.ctx.Err())
		default:
		}

		if c.scanner.Scan() {
			return c.scanner.Text(), nil
		}

		if err := c.scanner.Err(); err != nil {
			return "", fmt.Errorf("scanner error: %w", err)
		}

		time.Sleep(10 * time.Millisecond)
	}

	return "", fmt.Errorf("timeout reading output")
}

// Close terminates the control mode session
func (c *ControlModeSession) Close() error {
	// Cancel context
	c.cancel()

	// Close pipes
	c.Stdin.Close()
	c.Stdout.Close()
	c.stderr.Close()

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		// Force kill if doesn't exit gracefully
		if c.cmd.Process != nil {
			c.cmd.Process.Kill()
		}
		return fmt.Errorf("control mode session did not exit gracefully")
	}
}

// InjectClaudeStartup injects the Claude startup command using control mode
// This provides better verification than raw send-keys
func InjectClaudeStartup(sessionName string, command string) error {
	ctrl, err := StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Send command
	if err := ctrl.SendKeysLiteral(sessionName, command); err != nil {
		return fmt.Errorf("failed to send claude command: %w", err)
	}

	return nil
}

// SendCommandVerified sends a command to tmux and verifies it was executed
// This is a higher-level wrapper around control mode
func SendCommandVerified(sessionName, command string, timeout time.Duration) error {
	ctrl, err := StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	if err := ctrl.SendKeysLiteral(sessionName, command); err != nil {
		return fmt.Errorf("failed to send command: %w", err)
	}

	return nil
}

// WaitForOutput waits for specific output in the tmux pane
// This is useful for waiting for Claude to fully start
func (c *ControlModeSession) WaitForOutput(pattern string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		line, err := c.ReadOutput(1 * time.Second)
		if err != nil {
			// Timeout on individual read is OK, keep trying
			continue
		}

		if strings.Contains(line, pattern) {
			return nil // Found the pattern!
		}
	}

	return fmt.Errorf("timeout waiting for output pattern: %s", pattern)
}

// GetPaneContent captures the current content of a pane
func (c *ControlModeSession) GetPaneContent(target string, lines int) ([]string, error) {
	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedTarget := NormalizeTmuxSessionName(target)
	// Use capture-pane command
	// Note: capture-pane targets panes, not sessions, so we don't use FormatSessionTarget (=prefix)
	command := fmt.Sprintf("capture-pane -t %s -p -S -%d", normalizedTarget, lines)
	if err := c.SendCommand(command); err != nil {
		return nil, fmt.Errorf("failed to capture pane: %w", err)
	}

	// Read captured output
	var content []string
	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		line, err := c.ReadOutput(100 * time.Millisecond)
		if err != nil {
			break // Timeout or error, return what we have
		}

		// Stop at %end marker
		if strings.HasPrefix(line, "%end") {
			break
		}

		// Skip other control markers
		if strings.HasPrefix(line, "%") {
			continue
		}

		content = append(content, line)
	}

	return content, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
}

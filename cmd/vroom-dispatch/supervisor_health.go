package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/supervisorheartbeat"
)

// supervisorHealth is the liveness classification.
type supervisorHealth int

const (
	healthAlive      supervisorHealth = iota
	healthStale                       // heartbeat old but session exists
	healthDead                        // no session or session archived
	healthAuthFailed                  // session exists but the pane is stuck in provider auth
)

func (h supervisorHealth) String() string {
	switch h {
	case healthAlive:
		return "alive"
	case healthStale:
		return "stale"
	case healthDead:
		return "dead"
	case healthAuthFailed:
		return "auth_failed"
	default:
		return "unknown"
	}
}

// supervisorHealthChecker keeps observation probes local to one monitor, so
// classification can use the same interface with production and test probes.
type supervisorHealthChecker struct {
	sessionAlive  func(string) bool
	capturePane   func(string) (string, error)
	readHeartbeat func(string, string) (*supervisorheartbeat.Record, error)
	now           func() time.Time
}

func newSupervisorHealthChecker() supervisorHealthChecker {
	return supervisorHealthChecker{
		sessionAlive:  isSessionAlive,
		capturePane:   captureSupervisorPane,
		readHeartbeat: readSupervisorHeartbeat,
		now:           time.Now,
	}
}

func readSupervisorHeartbeat(home, name string) (*supervisorheartbeat.Record, error) {
	return supervisorheartbeat.New(filepath.Join(home, ".agm", "supervisors")).Read(name)
}

// classify determines health from session liveness, provider auth, and the
// authoritative heartbeat, in that order. A heartbeat read error remains
// stale and is returned for bounded diagnostics by the monitor loop.
func (c supervisorHealthChecker) classify(home string, sup supervisor) (supervisorHealth, error) {
	if !c.sessionAlive(sup.Name) {
		return healthDead, nil
	}
	if c.isAuthFailed(sup) {
		return healthAuthFailed, nil
	}

	record, err := c.readHeartbeat(home, sup.Name)
	if err != nil {
		return healthStale, fmt.Errorf("read authoritative heartbeat %q: %w", sup.Name, err)
	}
	if record == nil || record.LastBeatUTC.IsZero() {
		// Session exists but its authoritative heartbeat is unavailable — it
		// could still be booting.
		// Treat as stale rather than dead to avoid killing a session
		// that's still initializing.
		return healthStale, nil
	}

	threshold := 2 * sup.TickInterval
	if c.now().Sub(record.LastBeatUTC) > threshold {
		return healthStale, nil
	}

	return healthAlive, nil
}

// captureSupervisorPane returns the most recent supervisor pane text.
func captureSupervisorPane(name string) (string, error) {
	args := []string{"capture-pane", "-t", name, "-p", "-S", "-80"}
	if socket := os.Getenv("AGM_TMUX_SOCKET"); socket != "" {
		args = append([]string{"-S", socket}, args...)
	}
	cmd := exec.Command("tmux", args...) //nolint:gosec // G702: fixed executable and flags; target and socket are argv values, not shell input.
	out, err := cmd.Output()
	return string(out), err
}

func (c supervisorHealthChecker) isAuthFailed(sup supervisor) bool {
	content, err := c.capturePane(sup.Name)
	if err != nil {
		return false
	}
	return supervisorPaneAuthFailed(content, sup.Harness)
}

func supervisorPaneAuthFailed(content, harness string) bool {
	lines := recentSupervisorPaneLines(content)
	if len(lines) == 0 || supervisorPaneEndsAtPrompt(lines) {
		return false
	}
	lower := strings.Join(lines, "\n")
	switch harness {
	case "claude-code":
		return claudePaneAuthFailed(lines, lower)
	case "codex-cli":
		if codexPaneReady(lines) {
			return false
		}
		return codexPaneAuthFailed(lower)
	case "agy":
		if agyPaneReady(lines) {
			return false
		}
		return agyPaneAuthFailed(lower)
	default:
		return false
	}
}

// codexPaneReady mirrors the shared Codex readiness contract: after a turn,
// the idle cursor is followed by the structured model/workdir footer. Stale
// auth text above that current composer must not trigger recovery.
func codexPaneReady(lines []string) bool {
	for i, line := range slices.Backward(lines) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "gpt-") || !strings.Contains(line, " · ") {
			continue
		}
		for j := i - 1; j >= 0 && j >= i-3; j-- {
			candidate := strings.TrimSpace(lines[j])
			if candidate == "" {
				continue
			}
			if candidate == "›" || candidate == "»" {
				return true
			}
			break
		}
	}
	return false
}

// agyPaneReady mirrors the shared AGY composer contract: a current > composer
// owns input, and normal idle chrome (e.g. ? for shortcuts, sandbox status)
// may follow it. Historical auth text above that composer must not trigger recovery.
func agyPaneReady(lines []string) bool {
	composer := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == ">" {
			composer = i
		}
	}
	if composer < 0 {
		return false
	}
	for _, line := range lines[composer+1:] {
		lower := strings.ToLower(strings.TrimSpace(line))
		if lower == "" || strings.Trim(lower, "─━┄┈╌╍═│┃┆┊╎⏏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" ||
			strings.Contains(lower, "? for shortcuts") ||
			strings.Contains(lower, "shift+tab to") ||
			strings.Contains(lower, "accept edits") ||
			(strings.Contains(lower, "sandbox") && strings.Contains(lower, "gemini-")) {
			continue
		}
		return false
	}
	return true
}

func claudePaneAuthFailed(lines []string, lower string) bool {
	if claudePaneReady(lines) {
		return false
	}
	return hasExactLine(lines, "please run /login") ||
		(strings.Contains(lower, "claude") &&
			(strings.Contains(lower, "/login") || strings.Contains(lower, "oauth")) &&
			hasAny(lower, "401", "unauthorized", "authentication", "session expired", "token expired", "not authenticated"))
}

// claudePaneReady mirrors the shared Claude composer contract: a current
// composer glyph owns input, and the normal footer may follow it. Historical
// auth text above that composer is not evidence of a current auth block.
func claudePaneReady(lines []string) bool {
	composer := -1
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "❯" || strings.HasPrefix(line, "❯ ") {
			composer = i
		}
	}
	if composer < 0 {
		return false
	}
	for _, line := range lines[composer+1:] {
		lower := strings.ToLower(strings.TrimSpace(line))
		if lower == "" || strings.Trim(lower, "─━┄┈╌╍ ") == "" ||
			strings.Contains(lower, "? for shortcuts") ||
			strings.Contains(lower, "shift+tab to cycle") ||
			strings.Contains(lower, "plan mode on") {
			continue
		}
		return false
	}
	return true
}

func codexPaneAuthFailed(lower string) bool {
	return strings.Contains(lower, "codex login") ||
		strings.Contains(lower, "run `codex login`") ||
		(strings.Contains(lower, "openai") && strings.Contains(lower, "api key") &&
			hasAny(lower, "missing", "not found", "unauthorized", "authentication", "not authenticated"))
}

func agyPaneAuthFailed(lower string) bool {
	return strings.Contains(lower, "gcloud auth application-default login") ||
		strings.Contains(lower, "google_application_credentials") ||
		(strings.Contains(lower, "agy") && strings.Contains(lower, "sign in") &&
			hasAny(lower, "authentication", "not authenticated", "session expired", "token expired"))
}

func recentSupervisorPaneLines(content string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, strings.ToLower(line))
		}
	}
	const recentLineLimit = 12
	if len(lines) > recentLineLimit {
		lines = lines[len(lines)-recentLineLimit:]
	}
	return lines
}

func supervisorPaneEndsAtPrompt(lines []string) bool {
	last := strings.TrimSpace(lines[len(lines)-1])
	return last == ">" || last == "›"
}

func hasAny(content string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func hasExactLine(lines []string, want string) bool {
	return slices.Contains(lines, want)
}

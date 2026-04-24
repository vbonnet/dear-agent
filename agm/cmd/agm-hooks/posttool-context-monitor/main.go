package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

const (
	// updateIntervalSeconds is the minimum interval between updates (avoid excessive writes)
	updateIntervalSeconds = 5
	// minPercentageChange is the minimum percentage change to trigger update
	minPercentageChange = 1.0
)

// ContextMonitor monitors and updates AGM context usage from Claude Code sessions
type ContextMonitor struct {
	sessionID  string
	toolName   string
	toolResult string
	workingDir string
	cacheDir   string
	debug      bool
}

// NewContextMonitor creates a new context monitor from environment variables
func NewContextMonitor() *ContextMonitor {
	cacheDir := "/tmp/agm-context-cache"
	os.MkdirAll(cacheDir, 0755)

	return &ContextMonitor{
		sessionID:  os.Getenv("CLAUDE_SESSION_ID"),
		toolName:   os.Getenv("CLAUDE_TOOL_NAME"),
		toolResult: os.Getenv("CLAUDE_TOOL_RESULT"),
		workingDir: os.Getenv("CLAUDE_WORKING_DIR"),
		cacheDir:   cacheDir,
		debug:      os.Getenv("AGM_HOOK_DEBUG") == "1",
	}
}

// log writes messages to stderr if debug is enabled or level is ERROR
func (m *ContextMonitor) log(level, message string) {
	if m.debug || level == "ERROR" {
		timestamp := time.Now().Format(time.RFC3339)
		fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", timestamp, level, message)
	}
}

// TokenUsage represents extracted token usage
type TokenUsage struct {
	Used  int
	Total int
}

// extractTokenUsageFromReminder extracts token usage from system reminder
//
// Pattern: Token usage: 12345/200000; 187655 remaining
//
// Returns token usage or nil if not found
func (m *ContextMonitor) extractTokenUsageFromReminder(text string) *TokenUsage {
	pattern := regexp.MustCompile(`Token usage: (\d+)/(\d+);`)
	match := pattern.FindStringSubmatch(text)

	if len(match) > 2 {
		used, _ := strconv.Atoi(match[1])
		total, _ := strconv.Atoi(match[2])
		m.log("INFO", fmt.Sprintf("Extracted token usage: %d/%d", used, total))
		return &TokenUsage{Used: used, Total: total}
	}

	return nil
}

// JSONTokenUsage represents token usage in JSON format
type JSONTokenUsage struct {
	TokenUsage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"token_usage"`
	MaxContextTokens int `json:"max_context_tokens"`
}

// extractTokenUsageFromJSON extracts token usage from JSON input
func (m *ContextMonitor) extractTokenUsageFromJSON(data map[string]interface{}) *TokenUsage {
	tokenUsage, ok := data["token_usage"].(map[string]interface{})
	if !ok {
		return nil
	}

	totalTokens, ok := tokenUsage["total_tokens"].(float64)
	if !ok {
		return nil
	}

	maxTokens := 200000 // default
	if max, ok := data["max_context_tokens"].(float64); ok {
		maxTokens = int(max)
	}

	m.log("INFO", fmt.Sprintf("Extracted from JSON: %d/%d", int(totalTokens), maxTokens))
	return &TokenUsage{Used: int(totalTokens), Total: maxTokens}
}

// calculatePercentage calculates context usage percentage
func (m *ContextMonitor) calculatePercentage(used, total int) float64 {
	if total == 0 {
		return 0.0
	}

	percentage := (float64(used) / float64(total)) * 100.0
	return math.Round(percentage*10) / 10 // Round to 1 decimal place
}

// CacheEntry represents cached context usage
type CacheEntry struct {
	Percentage float64   `json:"percentage"`
	Timestamp  time.Time `json:"timestamp"`
}

// getCacheFile returns the cache file path for current session
func (m *ContextMonitor) getCacheFile() string {
	if m.sessionID == "" {
		return ""
	}
	return filepath.Join(m.cacheDir, m.sessionID+".json")
}

// shouldUpdate checks if update should be sent based on cache
//
// Returns true if:
//   - No cache exists
//   - updateIntervalSeconds elapsed since last update
//   - Percentage changed by >= minPercentageChange
func (m *ContextMonitor) shouldUpdate(percentage float64) bool {
	cacheFile := m.getCacheFile()
	if cacheFile == "" {
		return true
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return true // No cache exists
	}

	var cache CacheEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		m.log("WARN", fmt.Sprintf("Cache read error: %v", err))
		return true
	}

	// Check time interval
	if time.Since(cache.Timestamp).Seconds() < updateIntervalSeconds {
		m.log("INFO", "Skipping update (interval not elapsed)")
		return false
	}

	// Check percentage change
	change := math.Abs(percentage - cache.Percentage)
	if change < minPercentageChange {
		m.log("INFO", fmt.Sprintf("Skipping update (change too small: %.1f%%)", change))
		return false
	}

	return true
}

// updateCache updates cache with latest percentage and timestamp
func (m *ContextMonitor) updateCache(percentage float64) {
	cacheFile := m.getCacheFile()
	if cacheFile == "" {
		return
	}

	cache := CacheEntry{
		Percentage: percentage,
		Timestamp:  time.Now(),
	}

	data, err := json.Marshal(cache)
	if err != nil {
		m.log("WARN", fmt.Sprintf("Cache marshal error: %v", err))
		return
	}

	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		m.log("WARN", fmt.Sprintf("Cache write error: %v", err))
		return
	}

	m.log("INFO", fmt.Sprintf("Updated cache: %.1f%%", percentage))
}

// findAGMSession finds AGM session name for current Claude session
//
// Checks ~/.claude/sessions/{session_id}/manifest.yaml for agm_session_name
//
// Returns AGM session name or empty string if not AGM-managed
func (m *ContextMonitor) findAGMSession() string {
	if m.sessionID == "" {
		return ""
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		m.log("WARN", fmt.Sprintf("Error getting home dir: %v", err))
		return ""
	}

	manifestPath := filepath.Join(homeDir, ".claude", "sessions", m.sessionID, "manifest.yaml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		m.log("INFO", fmt.Sprintf("No manifest found at %s", manifestPath))
		return ""
	}

	// Look for agm_session_name field
	pattern := regexp.MustCompile(`agm_session_name:\s*(.+)`)
	match := pattern.FindStringSubmatch(string(data))

	if len(match) > 1 {
		sessionName := match[1]
		m.log("INFO", fmt.Sprintf("Found AGM session: %s", sessionName))
		return sessionName
	}

	m.log("INFO", "No agm_session_name in manifest (not AGM-managed)")
	return ""
}

// updateAGMContext updates AGM session context usage via CLI
//
// Returns true if successful, false otherwise
func (m *ContextMonitor) updateAGMContext(sessionName string, percentage float64) bool {
	// Round to integer
	percentageInt := int(math.Round(percentage))

	cmd := exec.Command("agm", "session", "set-context-usage",
		strconv.Itoa(percentageInt),
		"--session", sessionName)

	m.log("INFO", fmt.Sprintf("Running: %s", cmd.String()))

	output, err := cmd.CombinedOutput()
	if err != nil {
		m.log("ERROR", fmt.Sprintf("AGM command failed: %s", string(output)))
		return false
	}

	m.log("INFO", fmt.Sprintf("Successfully updated AGM context: %.1f%%", percentage))
	return true
}

// updateAGMState sets the session state via the agm CLI.
// Errors are logged but not fatal — state updates are best-effort.
func (m *ContextMonitor) updateAGMState(sessionName, state string) {
	cmd := exec.Command("agm", "session", "state", "set",
		sessionName, state, "--source", "posttool-hook")
	// Suppress AGM output — Claude Code treats any stderr from hooks as an error.
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		m.log("WARN", fmt.Sprintf("Failed to set state %s: %v", state, err))
	} else {
		m.log("INFO", fmt.Sprintf("Set state to %s", state))
	}
}

// Run executes the main hook logic
//
// Returns:
//   - 0: success
//   - 1: non-fatal error
//   - 2: fatal error
func (m *ContextMonitor) Run() int {
	m.log("INFO", "Context monitor hook started")

	// Find AGM session early — needed for both context and state updates
	sessionName := m.findAGMSession()

	// Signal THINKING state: a tool just executed, so the session is active
	if sessionName != "" {
		m.updateAGMState(sessionName, "THINKING")
	}

	// Try to extract token usage from multiple sources
	var tokenUsage *TokenUsage

	// Source 1: System reminders in tool result
	if m.toolResult != "" {
		tokenUsage = m.extractTokenUsageFromReminder(m.toolResult)
	}

	// Source 2: JSON from stdin
	if tokenUsage == nil {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			var data map[string]interface{}
			decoder := json.NewDecoder(os.Stdin)
			if err := decoder.Decode(&data); err == nil {
				tokenUsage = m.extractTokenUsageFromJSON(data)
			}
		}
	}

	// No token usage found - this is normal (not all tools show usage)
	if tokenUsage == nil {
		m.log("INFO", "No token usage found (skipping)")
		return 0
	}

	percentage := m.calculatePercentage(tokenUsage.Used, tokenUsage.Total)
	m.log("INFO", fmt.Sprintf("Calculated percentage: %.1f%%", percentage))

	// Check if update needed (based on cache)
	if !m.shouldUpdate(percentage) {
		return 0
	}

	if sessionName == "" {
		m.log("INFO", "Not an AGM session (skipping context update)")
		return 0
	}

	// Update AGM manifest
	success := m.updateAGMContext(sessionName, percentage)

	if success {
		// Update cache
		m.updateCache(percentage)
		return 0
	}

	// Non-fatal error (log and continue)
	return 1
}

func main() {
	monitor := NewContextMonitor()
	os.Exit(monitor.Run())
}

package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// StatusLineData holds all information for rendering the tmux status line
type StatusLineData struct {
	SessionName    string  // Session name
	State          string  // State text (DONE, WORKING, etc.)
	StateColor     string  // tmux color code for state
	Branch         string  // Git branch name
	Uncommitted    int     // Number of uncommitted files
	ContextPercent float64 // Context usage percentage (0-100, or -1 if unavailable)
	ContextColor   string  // tmux color code for context usage
	ContextUsed    string  // Human-readable used tokens (e.g., "50k", "600k")
	ContextTotal   string  // Human-readable total tokens (e.g., "200k", "1M")
	Workspace      string  // Workspace name
	AgentType      string  // Agent type (claude, gemini, gpt, opencode)
	AgentIcon      string  // Icon/emoji for agent
	Cost           string  // Formatted cost "$1.23"
	CostColor      string  // green (<$1), yellow ($1-10), red (>$10)
	ModelShort     string  // Short model name: "Opus", "Sonnet", "Haiku"
	RateLimit5h    float64 // 5-hour rate limit percentage (0-100, -1 if unavailable)
	RateLimitColor string  // green (<50%), yellow (50-80%), red (>80%)
}

// formatTokenCount formats a token count for display: 1000000 → "1M", 200000 → "200k", 5000 → "5k"
func formatTokenCount(tokens int) string {
	if tokens >= 1000000 {
		if tokens%1000000 == 0 {
			return fmt.Sprintf("%dM", tokens/1000000)
		}
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000.0)
	}
	if tokens >= 1000 {
		if tokens%1000 == 0 {
			return fmt.Sprintf("%dk", tokens/1000)
		}
		return fmt.Sprintf("%.1fk", float64(tokens)/1000.0)
	}
	return fmt.Sprintf("%d", tokens)
}

// defaultAgentIcons maps harness types to their default icons
var defaultAgentIcons = map[string]string{
	"claude-code":  "🤖",
	"gemini-cli":   "✨",
	"codex-cli":    "🧠",
	"opencode-cli": "💻",
}

// HookStalenessThreshold is the maximum age of a hook-based state update
// before it is considered stale and regex-based terminal detection is used
// as a fallback. Default: 60 seconds.
var HookStalenessThreshold = 60 * time.Second

// ResolveSessionState determines session state using a hybrid approach:
//
//  1. Primary: hook-based state from manifest (fast, accurate when fresh)
//  2. Staleness check: if hook state is older than HookStalenessThreshold,
//     re-detect via terminal parsing to avoid acting on stale data
//  3. Fallback: regex-based tmux pane parsing when hooks have no state
//  4. OFFLINE: when the tmux session doesn't exist
//
// This is the canonical state resolution function. All code paths that need
// to determine session state should use this function to ensure consistency
// across list, send, and status commands.
func ResolveSessionState(tmuxName, manifestState, claudeUUID string, stateUpdatedAt time.Time) string {
	exists, err := tmux.HasSession(tmuxName)
	if err != nil || !exists {
		return manifest.StateOffline
	}

	// Primary path: hook-based state from manifest
	if manifestState != "" {
		stale := !stateUpdatedAt.IsZero() && time.Since(stateUpdatedAt) > HookStalenessThreshold

		if stale {
			// Hook state is stale — re-detect via terminal parsing
			if termState := detectTerminalFullState(tmuxName); termState != "" {
				return termState
			}
			// Terminal parsing inconclusive, use stale hook state as best guess
			return manifestState
		}

		// Fresh hook state: still verify WORKING for blocked prompts that
		// hooks don't report
		if manifestState == manifest.StateWorking {
			if termState := detectTerminalBlockedState(tmuxName); termState != "" {
				return termState
			}
		}
		return manifestState
	}

	// Fallback path: no hook state — use regex-based terminal detection
	if termState := detectTerminalFullState(tmuxName); termState != "" {
		return termState
	}

	// Secondary fallback: check statusline file freshness
	if claudeUUID != "" && isStatusLineFileFresh(claudeUUID) {
		return "THINKING"
	}

	// No recent activity — DONE is the safe default (false-WORKING blocks
	// message delivery, which is worse than false-DONE).
	return manifest.StateDone
}

// detectTerminalBlockedState captures tmux pane content and checks for
// blocked states (permission prompts) that hooks don't report. Returns
// the manifest state string if blocked, or empty string if not blocked.
func detectTerminalBlockedState(tmuxName string) string {
	paneContent, err := tmux.CapturePaneOutput(tmuxName, 30)
	if err != nil {
		return "" // can't read pane, trust manifest
	}
	detector := state.NewDetector()
	result := detector.DetectState(paneContent, time.Now())
	if result.State.IsBlocked() {
		return manifest.StateUserPrompt
	}
	if result.State == state.StateReady {
		return manifest.StateDone
	}
	return "" // not blocked, trust manifest
}

// detectTerminalFullState performs full regex-based terminal state detection.
// Unlike detectTerminalBlockedState, this maps all detected states to manifest
// states, serving as a complete fallback when hooks are unavailable or stale.
// Returns empty string if terminal content cannot be read.
func detectTerminalFullState(tmuxName string) string {
	paneContent, err := tmux.CapturePaneOutput(tmuxName, 30)
	if err != nil {
		return "" // can't read pane
	}
	detector := state.NewDetector()
	result := detector.DetectState(paneContent, time.Now())
	mapped := mapTerminalStateToManifest(result.State)
	// Only return a result when confidence is at least medium
	if result.Confidence == "high" || result.Confidence == "medium" {
		return mapped
	}
	return ""
}

// applyStatusLineFileData reads cost, model, and rate-limit data from the
// statusline file and applies them to data.
//
// For interactive sessions, agm-statusline-capture writes CC's full session
// JSON (including exact total_cost_usd) to /tmp/agm-context/{session_id}.json
// after every assistant message, so this data is reliably available and
// up-to-date.
//
// The cost from CC (slData.Cost.TotalCostUSD) is the exact cost calculated
// by Claude Code itself — prefer it over the token-based estimate.
//
// Staleness policy:
//   - Cost: cumulative total — always applied, valid even when stale.
//   - Model: stable per session (doesn't change mid-session) — always applied.
//   - Rate limits: point-in-time metric — only applied when file is fresh.
func applyStatusLineFileData(data *StatusLineData, uuid string) {
	slData, err := ReadStatusLineFile(uuid)
	if err != nil {
		return
	}
	data.Cost = formatCost(slData.Cost.TotalCostUSD)
	data.CostColor = getCostColor(slData.Cost.TotalCostUSD)
	data.ModelShort = shortenModelName(slData.Model.DisplayName)
	if isStatusLineFileFresh(uuid) {
		data.RateLimit5h = slData.RateLimits.FiveHour.UsedPercentage
		data.RateLimitColor = getRateLimitColor(slData.RateLimits.FiveHour.UsedPercentage)
	}
}

// applyManifestFallback uses manifest-cached cost/model when the statusline
// file didn't provide them (e.g., session is idle, file absent).
func applyManifestFallback(data *StatusLineData, m *manifest.Manifest) {
	if data.Cost == "" && m.LastKnownCost > 0 {
		data.Cost = formatCost(m.LastKnownCost)
		data.CostColor = getCostColor(m.LastKnownCost)
	}
	if data.ModelShort == "" && m.LastKnownModel != "" {
		data.ModelShort = shortenModelName(m.LastKnownModel)
	}
}

// applyGitInfo resolves git branch and uncommitted file count for the session.
func applyGitInfo(data *StatusLineData, tmuxSessionName, projectDir string) {
	gitDir := projectDir
	if tmuxSessionName != "" {
		if cwd, err := tmux.GetCurrentWorkingDirectory(tmuxSessionName); err == nil && cwd != "" {
			gitDir = cwd
		}
	}
	if gitDir != "" {
		branch, err := getCurrentBranch(gitDir)
		if err == nil && branch != "" {
			data.Branch = branch
		} else {
			data.Branch = "unknown"
		}
		uncommitted, err := getUncommittedCount(gitDir)
		if err == nil {
			data.Uncommitted = uncommitted
		}
	} else {
		data.Branch = "unknown"
	}
}

// CollectStatusLineData gathers all status line data from a manifest
func CollectStatusLineData(sessionName string, m *manifest.Manifest) (*StatusLineData, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest cannot be nil")
	}

	tmuxSessionName := m.Tmux.SessionName
	if tmuxSessionName == "" {
		tmuxSessionName = sessionName
	}
	currentState := ResolveSessionState(tmuxSessionName, m.State, m.Claude.UUID, m.StateUpdatedAt)

	data := &StatusLineData{
		SessionName:    sessionName,
		State:          currentState,
		StateColor:     getStateColor(currentState),
		Workspace:      m.Workspace,
		AgentType:      m.Harness,
		AgentIcon:      getAgentIcon(m.Harness),
		ContextPercent: -1,
		Uncommitted:    0,
	}

	// Context usage: manifest → statusline file → conversation log
	var contextUsage *manifest.ContextUsage
	if usage, err := DetectContextFromManifestOrLog(m); err == nil && usage != nil {
		contextUsage = usage
		data.ContextPercent = usage.PercentageUsed
		data.ContextColor = getContextColor(usage.PercentageUsed)
		data.ContextUsed = formatTokenCount(usage.UsedTokens)
		data.ContextTotal = formatTokenCount(usage.TotalTokens)
		// Model fallback from conversation log
		if usage.ModelID != "" && data.ModelShort == "" {
			data.ModelShort = shortenModelName(modelIDToDisplayName(usage.ModelID))
		}
	} else {
		data.ContextColor = getContextColor(-1)
	}

	// Cost, model, rate-limit from statusline file
	data.RateLimit5h = -1
	if m.Claude.UUID != "" {
		applyStatusLineFileData(data, m.Claude.UUID)
	}
	applyManifestFallback(data, m)

	// Cost fallback: estimate from conversation log token counts
	if data.Cost == "" && contextUsage != nil && contextUsage.EstimatedCost > 0 {
		data.Cost = formatCost(contextUsage.EstimatedCost)
		data.CostColor = getCostColor(contextUsage.EstimatedCost)
	}
	applyGitInfo(data, tmuxSessionName, m.Context.Project)

	return data, nil
}

// getStateColor returns tmux color code for session state
func getStateColor(state string) string {
	switch state {
	case manifest.StateDone:
		return "green"
	case manifest.StateWorking:
		return "blue"
	case manifest.StateUserPrompt:
		return "yellow"
	case manifest.StateCompacting:
		return "magenta"
	case manifest.StateOffline:
		return "colour239" // Grey
	case manifest.StateWaitingAgent:
		return "cyan"
	case manifest.StateLooping:
		return "colour45" // Bright cyan
	case manifest.StateReady:
		return "green"
	case "THINKING":
		return "blue"
	case "PERMISSION_PROMPT":
		return "yellow"
	default:
		return "white"
	}
}

// getContextColor returns tmux color code based on context usage percentage
func getContextColor(percent float64) string {
	if percent < 0 {
		return "grey" // Unavailable
	}
	if percent < 70 {
		return "green" // Safe
	}
	if percent < 85 {
		return "yellow" // Warning
	}
	if percent < 95 {
		return "colour208" // Orange (high usage)
	}
	return "red" // Critical
}

// getAgentIcon returns the icon for a given agent type
func getAgentIcon(agentType string) string {
	if icon, ok := defaultAgentIcons[agentType]; ok {
		return icon
	}
	return "🤖" // Default to Claude icon
}

// formatCost formats a USD cost value for display.
func formatCost(usd float64) string {
	if usd < 0.01 {
		return "$0.00"
	}
	return fmt.Sprintf("$%.2f", usd)
}

// getCostColor returns tmux color code based on cost in USD.
func getCostColor(usd float64) string {
	if usd < 1.0 {
		return "green"
	}
	if usd < 10.0 {
		return "yellow"
	}
	return "red"
}

// shortenModelName extracts a short model name from a display name.
// Examples: "Opus 4.6 (1M context)" -> "Opus", "Claude Sonnet 4" -> "Sonnet"
func shortenModelName(displayName string) string {
	if displayName == "" {
		return ""
	}
	lower := strings.ToLower(displayName)
	switch {
	case strings.Contains(lower, "opus"):
		return "Opus"
	case strings.Contains(lower, "sonnet"):
		return "Sonnet"
	case strings.Contains(lower, "haiku"):
		return "Haiku"
	default:
		// Return first word as fallback
		parts := strings.Fields(displayName)
		if len(parts) > 0 {
			return parts[0]
		}
		return displayName
	}
}

// modelIDToDisplayName converts a model API ID to a human-readable display name.
// e.g., "claude-opus-4-6-20251001" → "Opus 4.6", "claude-sonnet-4-5-20250929" → "Sonnet 4.5"
func modelIDToDisplayName(modelID string) string {
	lower := strings.ToLower(modelID)
	switch {
	case strings.Contains(lower, "opus"):
		return "Opus"
	case strings.Contains(lower, "sonnet"):
		return "Sonnet"
	case strings.Contains(lower, "haiku"):
		return "Haiku"
	default:
		return modelID
	}
}

// getRateLimitColor returns tmux color code based on rate limit usage percentage.
func getRateLimitColor(pct float64) string {
	if pct < 0 {
		return "grey"
	}
	if pct < 50.0 {
		return "green"
	}
	if pct < 80.0 {
		return "yellow"
	}
	return "red"
}

// SetAgentIcons allows customizing agent icons
func SetAgentIcons(icons map[string]string) {
	for agent, icon := range icons {
		defaultAgentIcons[agent] = icon
	}
}

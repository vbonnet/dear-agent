package compaction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SessionState represents the parsed content of an AGM state file.
type SessionState struct {
	OrchestratorSession string                        `json:"orchestrator_session"`
	LastScan            string                        `json:"last_scan"`
	ManagedSessions     map[string]ManagedSessionInfo `json:"managed_sessions"`
	CompletedCount      int                           `json:"completed_this_session_count"`
	Policy              map[string]string             `json:"policy"`
	Queued              []string                      `json:"queued"`
	ScanLoop            *ScanLoopInfo                 `json:"scan_loop"`
	MetaOrchestrator    *StatusInfo                   `json:"meta_orchestrator"`
	Astrocyte           *StatusInfo                   `json:"astrocyte"`
}

// ManagedSessionInfo describes a managed worker session.
type ManagedSessionInfo struct {
	Status string `json:"status"`
	Notes  string `json:"notes"`
}

// ScanLoopInfo describes scan loop configuration.
type ScanLoopInfo struct {
	Interval string `json:"interval"`
	CronID   string `json:"cron_id"`
}

// StatusInfo describes a component status.
type StatusInfo struct {
	Status string `json:"status"`
}

// LoadSessionState reads a session's own state file from ~/.agm.
// It only loads {session}-state.json — no fallback to other sessions' state files,
// which would produce wrong identity in compaction prompts.
func LoadSessionState(baseDir, sessionName string) (*SessionState, string, error) {
	path := filepath.Join(baseDir, sessionName+"-state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("no state file found for session '%s' (tried: %s)", sessionName, path)
		}
		return nil, "", fmt.Errorf("read state file %s: %w", path, err)
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, "", fmt.Errorf("parse state file %s: %w", path, err)
	}
	return &state, path, nil
}

// GeneratePreservePrompt builds a PRESERVE prompt from a state file.
// targetSessionName is the name of the session being compacted — it is used as the
// identity in the PRESERVE prompt so the target retains its own identity, not the sender's.
func GeneratePreservePrompt(state *SessionState, stateFilePath string, focusText string, targetSessionName string) string {
	var sb strings.Builder

	// Use target session name as identity — never derive from state file content,
	// which could belong to a different session (the sender).
	identity := targetSessionName
	if identity == "" {
		identity = "worker"
	}

	// Build managed sessions summary
	var managedSummary string
	if len(state.ManagedSessions) > 0 {
		names := make([]string, 0, len(state.ManagedSessions))
		for name := range state.ManagedSessions {
			names = append(names, name)
		}
		sort.Strings(names)
		managedSummary = fmt.Sprintf("Managing %d workers: [%s]", len(state.ManagedSessions), strings.Join(names, ", "))
	}

	// Build policy rules
	var policyRules []string
	if len(state.Policy) > 0 {
		keys := make([]string, 0, len(state.Policy))
		for k := range state.Policy {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			policyRules = append(policyRules, state.Policy[k])
		}
	}

	// Build state summary parts
	var stateParts []string
	if managedSummary != "" {
		stateParts = append(stateParts, managedSummary)
	}
	if state.CompletedCount > 0 {
		stateParts = append(stateParts, fmt.Sprintf("%d completed sessions", state.CompletedCount))
	}
	if len(state.Queued) > 0 {
		stateParts = append(stateParts, fmt.Sprintf("Queue has %d items", len(state.Queued)))
	}

	// Build scan loop instruction
	var resumeLoop string
	if state.ScanLoop != nil && state.ScanLoop.Interval != "" {
		resumeLoop = fmt.Sprintf("Resume scan loop via /loop %s", state.ScanLoop.Interval)
	}

	// Construct the prompt
	sb.WriteString(fmt.Sprintf("/compact PRESERVE THROUGH COMPACTION: I am the %s.", identity))

	sb.WriteString(" After compaction, IMMEDIATELY:")
	sb.WriteString(fmt.Sprintf(" (1) Read %s", stateFilePath))

	stepNum := 2
	if resumeLoop != "" {
		sb.WriteString(fmt.Sprintf(" (%d) %s", stepNum, resumeLoop))
		stepNum++
	}
	sb.WriteString(fmt.Sprintf(" (%d) Check session health with agm session list.", stepNum))

	if len(policyRules) > 0 {
		sb.WriteString(fmt.Sprintf(" Critical rules: %s.", strings.Join(policyRules, " | ")))
	}

	if len(stateParts) > 0 {
		sb.WriteString(fmt.Sprintf(" Current state: %s.", strings.Join(stateParts, "; ")))
	}

	if focusText != "" {
		sb.WriteString(fmt.Sprintf(" Additional focus: %s.", focusText))
	}

	return sb.String()
}

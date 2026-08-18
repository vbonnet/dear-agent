package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

const (
	// meshSendBlockWindow is the lookback period for send-block incidents.
	meshSendBlockWindow = 30 * time.Minute
	// meshSendBlockThreshold is the minimum number of distinct sessions that must
	// show the same guard violation before it is reported as a mesh-wide block.
	meshSendBlockThreshold = 2
)

var (
	// guardNameRe retains compatibility with historical safety-guard audit
	// entries. CheckResult.Error() formatted each violation as "  <guard>: ...".
	guardNameRe = regexp.MustCompile(`\n\s+([a-z_]+):`)
	// readinessNameRe extracts the authoritative state from the shared
	// operation's stable AGM-016 detail.
	readinessNameRe = regexp.MustCompile(`readiness:\s*([A-Z][A-Z0-9_]*)`)
)

// parseSendBlockGuard extracts a stable mesh-block cause from current AGM-016
// operation errors or historical safety-guard audit entries.
func parseSendBlockGuard(errMsg string) string {
	if strings.Contains(errMsg, "["+ops.ErrCodeSessionNotReady+"]") {
		if match := readinessNameRe.FindStringSubmatch(errMsg); len(match) == 2 {
			return "readiness_" + strings.ToLower(match[1])
		}
	}
	if !strings.Contains(errMsg, "safety guard blocked") {
		return ""
	}
	if match := guardNameRe.FindStringSubmatch(errMsg); len(match) == 2 {
		return match[1]
	}
	return ""
}

// meshSendBlockAnalysis is the output of analyzeMeshWideSendBlock.
type meshSendBlockAnalysis struct {
	// GuardToSessions maps each guard name to the distinct sessions affected.
	GuardToSessions map[string][]string
}

// analyzeMeshWideSendBlock reads the audit log and groups recent send.msg guard
// failures by stable block cause and distinct recipient session.
//
//nolint:unparam // window is constant in production but varied in tests.
func analyzeMeshWideSendBlock(auditLogPath string, window time.Duration) (*meshSendBlockAnalysis, error) {
	since := time.Now().Add(-window)
	events, err := ops.ReadRecentEvents(auditLogPath, 0)
	if err != nil {
		return nil, err
	}

	result := &meshSendBlockAnalysis{
		GuardToSessions: make(map[string][]string),
	}
	seen := make(map[string]map[string]bool) // guard → session → present

	for _, ev := range events {
		if ev.Timestamp.Before(since) {
			continue
		}
		if ev.Command != "send.msg" || ev.Result != "error" || ev.Error == "" {
			continue
		}
		guard := parseSendBlockGuard(ev.Error)
		if guard == "" {
			continue
		}
		sessionName := ev.Session
		if sessionName == "" {
			continue
		}
		if seen[guard] == nil {
			seen[guard] = make(map[string]bool)
		}
		if !seen[guard][sessionName] {
			seen[guard][sessionName] = true
			result.GuardToSessions[guard] = append(result.GuardToSessions[guard], sessionName)
		}
	}

	return result, nil
}

// checkMeshWideSendBlock is the doctor check for mesh-wide send-block incidents.
// It returns true when the mesh is healthy (no incident), false when a block is
// detected so the caller can mark overall health as degraded.
//
// An incident is declared when N≥2 distinct sessions have recent send.msg
// failures caused by the same safety guard within the lookback window. Rather
// than letting each supervisor rediscover the block independently, this emits
// one consolidated warning — the pattern that was invisible during the 2026-06-18
// ghost-text outage (ce-q7am).
func checkMeshWideSendBlock() bool {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("mesh send-block check: cannot determine home dir: %v", err))
		return true // fail open
	}

	auditLogPath := filepath.Join(homeDir, ".agm", "logs", "audit.jsonl")
	analysis, err := analyzeMeshWideSendBlock(auditLogPath, meshSendBlockWindow)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("mesh send-block check: cannot read audit log: %v", err))
		return true // fail open — don't block doctor on a missing log
	}

	type incident struct {
		Guard    string
		Sessions []string
	}

	var incidents []incident
	for guard, sessions := range analysis.GuardToSessions {
		if len(sessions) >= meshSendBlockThreshold {
			sorted := make([]string, len(sessions))
			copy(sorted, sessions)
			sort.Strings(sorted)
			incidents = append(incidents, incident{Guard: guard, Sessions: sorted})
		}
	}

	if len(incidents) == 0 {
		ui.PrintSuccess("No mesh-wide send-block detected")
		return true
	}

	sort.Slice(incidents, func(i, j int) bool {
		return incidents[i].Guard < incidents[j].Guard
	})

	for _, inc := range incidents {
		ui.PrintWarning(fmt.Sprintf(
			"mesh-wide send-block detected: %s blocking %d sessions (last %s)",
			inc.Guard, len(inc.Sessions), meshSendBlockWindow,
		))
		for _, s := range inc.Sessions {
			fmt.Printf("    • %s\n", s)
		}
		fmt.Printf("  Impact: each supervisor is rediscovering this block independently.\n")
		fmt.Printf("  Fix: resolve the guard condition, or use --force --reason \"...\" to bypass.\n")
	}

	return false
}

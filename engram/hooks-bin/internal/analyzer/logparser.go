package analyzer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reTimestamp    = regexp.MustCompile(`\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\]`)
	rePatternMatch = regexp.MustCompile(`Pattern #(\d+) MATCHED: (.+) \(pattern: (.+)\)`)
	reDenied       = regexp.MustCompile(`DENIED: (.+)`)
	reValidation   = regexp.MustCompile(`Validation result: ok=(true|false), pattern="(.*)", remediation="(.*)"`)
)

const timestampLayout = "2006-01-02 15:04:05"

// legacyInput is used to parse the legacy JSON format with parameters.command.
type legacyInput struct {
	Parameters struct {
		Command string `json:"command"`
	} `json:"parameters"`
}

// parseState tracks the state machine for parsing a single log entry.
type parseState struct {
	active      bool
	timestamp   time.Time
	rawInput    RawHookInput
	command     string
	patternName string
	patternIdx  int
	patternRe   string
	remediation string
}

// ParseLog reads the hook log file at path and returns denial entries, approval
// entries, and aggregate statistics. If since is non-nil, entries with timestamps
// before *since are skipped.
func ParseLog(path string, since *time.Time) ([]DenialEntry, []ApprovalEntry, HookLogStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, HookLogStats{}, fmt.Errorf("open log: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var (
		denials   []DenialEntry
		approvals []ApprovalEntry
		stats     HookLogStats
		state     parseState
		sessions  = make(map[string]struct{})
		firstTS   time.Time
		lastTS    time.Time
		haveTR    bool
	)
	stats.DenialsByPattern = make(map[string]int)

	updateTimeRange := func(ts time.Time) {
		if ts.IsZero() {
			return
		}
		if !haveTR {
			firstTS = ts
			lastTS = ts
			haveTR = true
		} else {
			if ts.Before(firstTS) {
				firstTS = ts
			}
			if ts.After(lastTS) {
				lastTS = ts
			}
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Detect new entry boundary.
		if strings.Contains(line, "=== Hook invoked ===") {
			ts := extractTimestamp(line)
			// If we had an active state that was never finalized (malformed), discard it.
			state = parseState{active: true, timestamp: ts}
			stats.TotalInvocations++
			updateTimeRange(ts)
			continue
		}

		if !state.active {
			continue
		}

		// Parse Raw input JSON.
		if idx := strings.Index(line, "Raw input: "); idx >= 0 {
			jsonStr := line[idx+len("Raw input: "):]
			// Try standard format first.
			var ri RawHookInput
			if err := json.Unmarshal([]byte(jsonStr), &ri); err == nil {
				state.rawInput = ri
				// Handle legacy format: if tool_input.command is empty, try parameters.command.
				if ri.ToolInput.Command == "" {
					var legacy legacyInput
					if err2 := json.Unmarshal([]byte(jsonStr), &legacy); err2 == nil && legacy.Parameters.Command != "" {
						state.rawInput.ToolInput.Command = legacy.Parameters.Command
					}
				}
			}
			continue
		}

		// Parse "Parsed command:" line.
		if idx := strings.Index(line, "Parsed command: "); idx >= 0 {
			state.command = line[idx+len("Parsed command: "):]
			continue
		}

		// Parse VALIDATOR pattern match.
		if m := rePatternMatch.FindStringSubmatch(line); m != nil {
			state.patternIdx, _ = strconv.Atoi(m[1])
			state.patternName = m[2]
			state.patternRe = m[3]
			continue
		}

		// Parse Validation result line.
		if m := reValidation.FindStringSubmatch(line); m != nil {
			if m[2] != "" {
				state.patternName = m[2]
			}
			state.remediation = m[3]
			continue
		}

		// Detect DENIED.
		if reDenied.MatchString(line) {
			ts := extractTimestamp(line)
			if !ts.IsZero() {
				state.timestamp = ts
			}

			// Apply since filter.
			if since != nil && state.timestamp.Before(*since) {
				state.active = false
				continue
			}

			cmd := state.rawInput.ToolInput.Command
			if cmd == "" {
				cmd = state.command
			}

			if state.rawInput.SessionID != "" {
				sessions[state.rawInput.SessionID] = struct{}{}
			}

			denials = append(denials, DenialEntry{
				Timestamp:      state.timestamp,
				SessionID:      state.rawInput.SessionID,
				TranscriptPath: state.rawInput.TranscriptPath,
				ToolUseID:      state.rawInput.ToolUseID,
				Command:        cmd,
				PatternName:    state.patternName,
				PatternIndex:   state.patternIdx,
				PatternRegex:   state.patternRe,
				Remediation:    state.remediation,
				CWD:            state.rawInput.CWD,
			})
			stats.TotalDenials++
			if state.patternName != "" {
				stats.DenialsByPattern[state.patternName]++
			}
			state.active = false
			continue
		}

		// Detect APPROVED.
		if strings.Contains(line, "APPROVED") && !strings.Contains(line, "VALIDATOR") {
			ts := extractTimestamp(line)
			if !ts.IsZero() {
				state.timestamp = ts
			}

			if since != nil && state.timestamp.Before(*since) {
				state.active = false
				continue
			}

			cmd := state.rawInput.ToolInput.Command
			if cmd == "" {
				cmd = state.command
			}

			if state.rawInput.SessionID != "" {
				sessions[state.rawInput.SessionID] = struct{}{}
			}

			approvals = append(approvals, ApprovalEntry{
				Timestamp:      state.timestamp,
				SessionID:      state.rawInput.SessionID,
				TranscriptPath: state.rawInput.TranscriptPath,
				ToolUseID:      state.rawInput.ToolUseID,
				Command:        cmd,
			})
			stats.TotalApprovals++
			state.active = false
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, HookLogStats{}, fmt.Errorf("scan log: %w", err)
	}

	stats.UniqueSessionIDs = len(sessions)
	if haveTR {
		stats.TimeRange = [2]time.Time{firstTS, lastTS}
	}

	return denials, approvals, stats, nil
}

// extractTimestamp pulls a timestamp from a log line's bracket prefix.
func extractTimestamp(line string) time.Time {
	m := reTimestamp.FindStringSubmatch(line)
	if m == nil {
		return time.Time{}
	}
	t, err := time.Parse(timestampLayout, m[1])
	if err != nil {
		return time.Time{}
	}
	return t
}

// ParseTimeDelta parses a human-friendly duration string such as "7d", "24h",
// or "1w" into a time.Duration.
func ParseTimeDelta(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid time delta %q: too short", s)
	}

	numPart := s[:len(s)-1]
	unit := s[len(s)-1]

	n, err := strconv.Atoi(numPart)
	if err != nil {
		return 0, fmt.Errorf("invalid time delta %q: %w", s, err)
	}

	switch unit {
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 's':
		return time.Duration(n) * time.Second, nil
	default:
		return 0, fmt.Errorf("invalid time delta %q: unknown unit %q", s, string(unit))
	}
}

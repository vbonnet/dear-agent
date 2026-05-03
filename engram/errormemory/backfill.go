// Package errormemory provides errormemory-related functionality.
package errormemory

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// BackfillResult contains statistics from a backfill operation.
type BackfillResult struct {
	LinesProcessed int
	DeniedFound    int
	UniquePatterns int
	Records        []ErrorRecord
}

// deniedRe matches DENIED lines in the pretool-bash-blocker.log format.
// Format: [2026-03-09 05:22:31] DENIED: PATTERN_NAME - REMEDIATION_TEXT
var deniedRe = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\] DENIED:\s+(.+?)\s+-\s+(.+)$`)

// rawInputRe matches "Raw input" lines that contain JSON with session_id.
var rawInputRe = regexp.MustCompile(`Raw input`)

// BackfillFromLog parses a pretool-bash-blocker.log file and returns deduplicated ErrorRecords.
func BackfillFromLog(logPath string) (*BackfillResult, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	result := &BackfillResult{}
	// Map from pattern name to index in result.Records
	patternIndex := make(map[string]int)
	lastSessionID := ""

	scanner := bufio.NewScanner(f)
	// Increase buffer size for potentially long lines
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()
		result.LinesProcessed++

		// Check for Raw input lines containing session_id
		if rawInputRe.MatchString(line) {
			sid := extractSessionID(line)
			if sid != "" {
				lastSessionID = sid
			}
			continue
		}

		// Check for DENIED lines
		matches := deniedRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		result.DeniedFound++

		timestamp, err := time.Parse("2006-01-02 15:04:05", matches[1])
		if err != nil {
			continue
		}
		patternName := strings.TrimSpace(matches[2])
		remediation := strings.TrimSpace(matches[3])

		idx, exists := patternIndex[patternName]
		if exists {
			// Update existing record
			result.Records[idx].Count++
			if timestamp.After(result.Records[idx].LastSeen) {
				result.Records[idx].LastSeen = timestamp
				result.Records[idx].TTLExpiry = timestamp.Add(DefaultTTL)
			}
			if timestamp.Before(result.Records[idx].FirstSeen) {
				result.Records[idx].FirstSeen = timestamp
			}
			if lastSessionID != "" {
				found := false
				for _, sid := range result.Records[idx].SessionIDs {
					if sid == lastSessionID {
						found = true
						break
					}
				}
				if !found {
					result.Records[idx].SessionIDs = append(result.Records[idx].SessionIDs, lastSessionID)
					if len(result.Records[idx].SessionIDs) > 5 {
						result.Records[idx].SessionIDs = result.Records[idx].SessionIDs[len(result.Records[idx].SessionIDs)-5:]
					}
				}
			}
		} else {
			// Create new record
			rec := ErrorRecord{
				ID:            recordID(patternName, SourceBashBlocker),
				Pattern:       patternName,
				ErrorCategory: SourceBashBlocker,
				Remediation:   remediation,
				Count:         1,
				FirstSeen:     timestamp,
				LastSeen:      timestamp,
				TTLExpiry:     timestamp.Add(DefaultTTL),
				Source:        SourceBashBlocker,
			}
			if lastSessionID != "" {
				rec.SessionIDs = []string{lastSessionID}
			}
			patternIndex[patternName] = len(result.Records)
			result.Records = append(result.Records, rec)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning log file: %w", err)
	}

	result.UniquePatterns = len(patternIndex)
	return result, nil
}

// extractSessionID tries to extract a session_id from a Raw input line.
// The line may contain embedded JSON with a "session_id" field.
func extractSessionID(line string) string {
	// Find JSON object in the line
	start := strings.Index(line, "{")
	if start < 0 {
		return ""
	}

	// Find the matching closing brace (simple approach - find last })
	end := strings.LastIndex(line, "}")
	if end < start {
		return ""
	}

	jsonStr := line[start : end+1]
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
		return ""
	}

	if sid, ok := obj["session_id"]; ok {
		if s, ok := sid.(string); ok {
			return s
		}
	}
	return ""
}

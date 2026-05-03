// Package astrocyte provides astrocyte functionality.
package astrocyte

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Diagnosis represents a parsed Astrocyte diagnosis file.
type Diagnosis struct {
	// SessionID is the name of the session that experienced the incident
	SessionID string

	// Timestamp is when the incident was detected
	Timestamp time.Time

	// Type describes the kind of hang/incident (e.g., "Permission prompt", "Galloping", "Stuck prompt")
	Type string

	// RecoverySuccess indicates whether the recovery was successful
	RecoverySuccess bool

	// RecoveryTime is how long it took to recover (if available)
	RecoveryTime string

	// RecoveryMethod describes how recovery was achieved (e.g., "ESC", "Auto-recovery")
	RecoveryMethod string

	// Symptom is the raw symptom description
	Symptom string

	// Confidence level if available (e.g., "HIGH", "MEDIUM", "LOW")
	Confidence string

	// FilePath is the path to the original diagnosis file
	FilePath string
}

var (
	// Regex patterns for parsing diagnosis files
	titlePattern        = regexp.MustCompile(`^##\s+Incident Diagnosis:\s+(.+?)\s+-\s+(.+)$`)
	recoveryPattern     = regexp.MustCompile(`\*\*Recovery Time\*\*:\s+(.+?)(?:\s*\*\*|$)`)
	recoveryMethodPat   = regexp.MustCompile(`\*\*Recovery Method\*\*:\s+(.+?)(?:\s*\*\*|$)`)
	incidentTypePattern = regexp.MustCompile(`\*\*Incident Type\*\*:\s+(.+?)(?:\s*\*\*|$)`)
)

// ParseDiagnosisFile parses a single Astrocyte diagnosis markdown file.
func ParseDiagnosisFile(filePath string) (*Diagnosis, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open diagnosis file: %w", err)
	}
	defer file.Close()

	diag := &Diagnosis{
		FilePath:        filePath,
		RecoverySuccess: true, // Default to true unless evidence of failure
	}

	scanner := bufio.NewScanner(file)
	var currentSection string
	var symptomLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Parse title line to extract session ID and timestamp
		if matches := titlePattern.FindStringSubmatch(line); matches != nil {
			diag.SessionID = strings.TrimSpace(matches[1])
			diag.Timestamp = parseTimestamp(matches[2])
			continue
		}

		// Track current section
		if strings.HasPrefix(line, "###") {
			currentSection = strings.TrimSpace(strings.TrimPrefix(line, "###"))
			continue
		}

		// Parse symptom section
		if currentSection == "Symptom" && line != "" && !strings.HasPrefix(line, "###") {
			symptomLines = append(symptomLines, line)
		}

		// Extract recovery information
		if matches := recoveryPattern.FindStringSubmatch(line); matches != nil {
			diag.RecoveryTime = strings.TrimSpace(matches[1])
		}

		if matches := recoveryMethodPat.FindStringSubmatch(line); matches != nil {
			diag.RecoveryMethod = strings.TrimSpace(matches[1])
		}

		if matches := incidentTypePattern.FindStringSubmatch(line); matches != nil {
			typeStr := strings.TrimSpace(matches[1])
			diag.Type = typeStr
			// Check if incident type indicates success
			if strings.Contains(strings.ToUpper(typeStr), "SUCCESS") {
				diag.RecoverySuccess = true
			}
		}

		// Extract confidence level
		if strings.Contains(line, "**HIGH") || strings.Contains(line, "HIGH (") {
			diag.Confidence = "HIGH"
		} else if strings.Contains(line, "**MEDIUM") || strings.Contains(line, "MEDIUM (") {
			diag.Confidence = "MEDIUM"
		} else if strings.Contains(line, "**LOW") || strings.Contains(line, "LOW (") {
			diag.Confidence = "LOW"
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading diagnosis file: %w", err)
	}

	// Join symptom lines and infer hang type
	diag.Symptom = strings.Join(symptomLines, " ")
	diag.Type = inferHangType(diag.Symptom, diag.Type)

	// If no session ID was found, try extracting from filename
	if diag.SessionID == "" {
		diag.SessionID = extractSessionFromFilename(filePath)
	}

	return diag, nil
}

// ParseDiagnosisDirectory parses all diagnosis files in a directory.
func ParseDiagnosisDirectory(dirPath string) ([]*Diagnosis, error) {
	files, err := filepath.Glob(filepath.Join(dirPath, "*.md"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob diagnosis files: %w", err)
	}

	var diagnoses []*Diagnosis
	var parseErrors []string

	for _, file := range files {
		diag, err := ParseDiagnosisFile(file)
		if err != nil {
			// Log error but continue parsing other files
			parseErrors = append(parseErrors, fmt.Sprintf("%s: %v", filepath.Base(file), err))
			continue
		}
		diagnoses = append(diagnoses, diag)
	}

	// If we have parse errors but also some successful parses, return both
	if len(parseErrors) > 0 && len(diagnoses) > 0 {
		err = fmt.Errorf("parsed %d files with %d errors: %s",
			len(diagnoses), len(parseErrors), strings.Join(parseErrors, "; "))
	} else if len(parseErrors) > 0 {
		return nil, fmt.Errorf("failed to parse any files: %s", strings.Join(parseErrors, "; "))
	}

	return diagnoses, err
}

// parseTimestamp attempts to parse various timestamp formats found in diagnosis files.
func parseTimestamp(timestampStr string) time.Time {
	timestampStr = strings.TrimSpace(timestampStr)

	// Common formats in diagnosis files
	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02T15-04-05",
		"2006-01-02T15-04",
		"2006-01-02",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timestampStr); err == nil {
			return t
		}
	}

	// If all parsing fails, return zero time
	return time.Time{}
}

// inferHangType attempts to determine the hang type from the symptom description.
func inferHangType(symptom, existingType string) string {
	if existingType != "" {
		return existingType
	}

	symptomLower := strings.ToLower(symptom)

	// Check for common hang types
	if strings.Contains(symptomLower, "permission prompt") {
		return "Permission Prompt"
	}
	if strings.Contains(symptomLower, "galloping") || strings.Contains(symptomLower, "0 tokens") {
		return "Zero-Token Galloping"
	}
	if strings.Contains(symptomLower, "stuck") && strings.Contains(symptomLower, "prompt") {
		return "Stuck Prompt"
	}
	if strings.Contains(symptomLower, "api stall") || strings.Contains(symptomLower, "api timeout") {
		return "API Stall"
	}
	if strings.Contains(symptomLower, "deadlock") {
		return "Deadlock"
	}

	return "Unknown"
}

// extractSessionFromFilename extracts the session name from a diagnosis filename.
// Expected format: {session-name}-{timestamp}.md
func extractSessionFromFilename(filePath string) string {
	baseName := filepath.Base(filePath)
	baseName = strings.TrimSuffix(baseName, ".md")

	// Split on last occurrence of timestamp pattern (YYYY-MM-DD or similar)
	parts := strings.Split(baseName, "-2026-")
	if len(parts) > 1 {
		return parts[0]
	}

	// Fallback: just return the base name without extension
	return baseName
}

// FilterBySession returns diagnoses for a specific session.
func FilterBySession(diagnoses []*Diagnosis, sessionID string) []*Diagnosis {
	var filtered []*Diagnosis
	for _, diag := range diagnoses {
		if diag.SessionID == sessionID {
			filtered = append(filtered, diag)
		}
	}
	return filtered
}

// FilterByTimeRange returns diagnoses within a time range.
func FilterByTimeRange(diagnoses []*Diagnosis, start, end time.Time) []*Diagnosis {
	var filtered []*Diagnosis
	for _, diag := range diagnoses {
		if !diag.Timestamp.IsZero() && diag.Timestamp.After(start) && diag.Timestamp.Before(end) {
			filtered = append(filtered, diag)
		}
	}
	return filtered
}

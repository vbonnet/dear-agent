package roadmap

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ParseROADMAP parses a ROADMAP.md file and extracts beads with their metadata
func ParseROADMAP(filePath string) ([]Bead, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ROADMAP file: %w", err)
	}
	defer file.Close()

	var beads []Bead
	var currentPhase string
	lineNum := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Detect phase headers: ## Phase N: Title or ## Phase N - Title
		if phase := detectPhaseHeader(line); phase != "" {
			currentPhase = phase
			continue
		}

		// Extract bead from table row
		if bead := extractBeadFromLine(line, lineNum, currentPhase); bead != nil {
			beads = append(beads, *bead)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading ROADMAP file: %w", err)
	}

	return beads, nil
}

// detectPhaseHeader detects phase section headers
// Pattern: ## Phase N: Title or ## Phase N - Title
func detectPhaseHeader(line string) string {
	// Match: ## Phase 0: Data Collection or ## Phase 0 - Data Collection
	phasePattern := regexp.MustCompile(`^##\s+Phase\s+(\d+)\s*[:\-]`)
	matches := phasePattern.FindStringSubmatch(line)
	if len(matches) > 1 {
		return fmt.Sprintf("phase-%s", matches[1])
	}
	return ""
}

// extractBeadFromLine extracts bead information from a Markdown table row
// Expected format: | `oss-abc` | Description | Effort | Status |
func extractBeadFromLine(line string, lineNum int, currentPhase string) *Bead {
	// Skip non-table lines
	if !strings.HasPrefix(strings.TrimSpace(line), "|") {
		return nil
	}

	// Skip table separators (e.g., |---|---|)
	if strings.Contains(line, "---") {
		return nil
	}

	// Split by pipe and clean
	parts := strings.Split(line, "|")
	if len(parts) < 3 {
		return nil // Not enough columns
	}

	// Clean whitespace
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// Extract bead ID from backticks in first meaningful column
	beadID := ""
	for _, part := range parts {
		if id := extractBeadID(part); id != "" {
			beadID = id
			break
		}
	}

	if beadID == "" {
		return nil // No valid bead ID found
	}

	// Extract description, effort, status
	// Table format varies, so we'll be flexible
	description := ""
	effort := ""
	rawStatus := ""

	// Try to extract from remaining columns
	for _, part := range parts {
		if part == "" || part == beadID || strings.Contains(part, "`"+beadID+"`") {
			continue
		}

		// First non-ID column is usually description
		if description == "" && !looksLikeStatus(part) && !looksLikeEffort(part) {
			description = part
			continue
		}

		// Detect effort patterns (e.g., "1 day", "2 hours")
		if effort == "" && looksLikeEffort(part) {
			effort = part
			continue
		}

		// Remaining is likely status
		if rawStatus == "" {
			rawStatus = part
		}
	}

	// Assign phase (or "phase-unknown" if not under a phase section)
	phase := currentPhase
	if phase == "" {
		phase = "phase-unknown"
	}

	return &Bead{
		ID:          beadID,
		Description: description,
		Effort:      effort,
		Status:      NormalizeStatus(rawStatus),
		RawStatus:   rawStatus,
		Phase:       phase,
		LineNumber:  lineNum,
	}
}

// extractBeadID extracts bead ID from backtick format
// Pattern: `oss-xyz` or `oss-abc-123`
func extractBeadID(text string) string {
	// Match: `oss-[word characters and hyphens]`
	beadPattern := regexp.MustCompile("`(oss-[a-z0-9-]+)`")
	matches := beadPattern.FindStringSubmatch(text)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// looksLikeEffort checks if text looks like an effort estimate
func looksLikeEffort(text string) bool {
	effortPatterns := []string{
		"day", "days", "hour", "hours", "week", "weeks",
		"hr", "hrs", "min", "mins", "month", "months",
	}
	lower := strings.ToLower(text)
	for _, pattern := range effortPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// looksLikeStatus checks if text looks like a status indicator
func looksLikeStatus(text string) bool {
	statusIndicators := []string{
		"✅", "📋", "🔄", "❌", "⏸",
		"COMPLETE", "PLANNED", "IN_PROGRESS", "CANCELLED", "BLOCKED",
		"complete", "planned", "in_progress", "cancelled", "blocked",
		"pending", "open", "closed",
	}
	for _, indicator := range statusIndicators {
		if strings.Contains(text, indicator) {
			return true
		}
	}
	return false
}

// NormalizeStatus converts various status formats to canonical status
// Mapping:
// - ✅/COMPLETE/complete → "completed"
// - 📋/PLANNED/planned → "pending"
// - 🔄/IN_PROGRESS/in_progress → "in_progress"
// - ❌/CANCELLED/cancelled → "cancelled"
// - ⏸/BLOCKED/blocked → "blocked"
func NormalizeStatus(rawStatus string) string {
	lower := strings.ToLower(rawStatus)

	// Check cancelled/abandoned first (before "done" which matches "abandoned")
	if strings.Contains(rawStatus, "❌") ||
		strings.Contains(lower, "cancelled") ||
		strings.Contains(lower, "canceled") ||
		strings.Contains(lower, "abandoned") {
		return "cancelled"
	}

	// Completed patterns
	if strings.Contains(rawStatus, "✅") ||
		strings.Contains(lower, "complete") ||
		strings.Contains(lower, "closed") ||
		strings.Contains(lower, "done") {
		return "completed"
	}

	// In progress patterns
	if strings.Contains(rawStatus, "🔄") ||
		strings.Contains(lower, "in_progress") ||
		strings.Contains(lower, "in progress") ||
		strings.Contains(lower, "working") {
		return "in_progress"
	}

	// Blocked patterns
	if strings.Contains(rawStatus, "⏸") ||
		strings.Contains(lower, "blocked") ||
		strings.Contains(lower, "paused") {
		return "blocked"
	}

	// Pending/planned patterns (default)
	if strings.Contains(rawStatus, "📋") ||
		strings.Contains(lower, "planned") ||
		strings.Contains(lower, "pending") ||
		strings.Contains(lower, "open") ||
		strings.Contains(lower, "todo") {
		return "pending"
	}

	// Unknown status - default to pending
	if rawStatus == "" {
		return "pending"
	}

	// Return lowercase version if no pattern matched
	return strings.ToLower(strings.TrimSpace(rawStatus))
}

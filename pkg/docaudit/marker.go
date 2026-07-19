package docaudit

import (
	"regexp"
	"strings"
	"time"
)

const markerPrefix = "<!-- Last audited at:"

var canonicalMarker = regexp.MustCompile(`^<!-- Last audited at: ([0-9]{4}-[0-9]{2}-[0-9]{2}) -->$`)

func classifyMarker(data []byte, maxAgeDays int, asOf time.Time) FindingKind {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	markerLines := make([]string, 0, 1)
	for _, line := range lines {
		if isStandaloneMarker(line) {
			markerLines = append(markerLines, line)
		}
	}
	candidate, present := headerMarkerCandidate(lines)
	if !present || !strings.Contains(candidate, markerPrefix) {
		return MissingMarker
	}
	if len(markerLines) > 1 {
		return DuplicateMarker
	}
	line := candidate
	if line == "<!-- Last audited at: NEEDS-AUDIT -->" {
		return NeedsAudit
	}
	match := canonicalMarker.FindStringSubmatch(line)
	if match == nil {
		return MalformedMarker
	}
	auditDate, err := time.Parse("2006-01-02", match[1])
	if err != nil {
		return InvalidDate
	}
	asOf = dateOnly(asOf)
	if auditDate.After(asOf) {
		return FutureDate
	}
	if asOf.Sub(auditDate) > time.Duration(maxAgeDays)*24*time.Hour {
		return StaleDate
	}
	return ""
}

func isStandaloneMarker(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, markerPrefix) && strings.HasSuffix(line, "-->")
}

func headerMarkerCandidate(lines []string) (string, bool) {
	nonblank := make([]string, 0, 2)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		nonblank = append(nonblank, line)
		if len(nonblank) == 2 {
			break
		}
	}
	if len(nonblank) == 0 {
		return "", false
	}
	if strings.HasPrefix(strings.TrimSpace(nonblank[0]), markerPrefix) {
		return nonblank[0], true
	}
	if strings.HasPrefix(strings.TrimSpace(nonblank[0]), "# ") && len(nonblank) == 2 {
		return nonblank[1], true
	}
	return "", false
}

func dateOnly(value time.Time) time.Time {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

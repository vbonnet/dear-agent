package docaudit

import (
	"regexp"
	"strings"
	"time"
)

const markerPrefix = "<!-- Last audited at:"

var canonicalMarker = regexp.MustCompile(`^<!-- Last audited at: ([0-9]{4}-[0-9]{2}-[0-9]{2}) -->$`)

func classifyMarker(data []byte, maxAgeDays int, asOf time.Time) FindingKind {
	markerLines := make([]string, 0, 1)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.Contains(line, markerPrefix) {
			markerLines = append(markerLines, line)
		}
	}
	if len(markerLines) == 0 {
		return MissingMarker
	}
	if len(markerLines) > 1 {
		return DuplicateMarker
	}
	line := markerLines[0]
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

func dateOnly(value time.Time) time.Time {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

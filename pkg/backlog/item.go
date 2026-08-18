package backlog

import (
	"strconv"
	"strings"
)

// Status is the lifecycle state of a backlog item, parsed from the Status
// column. Only Pending items are ever suggested.
type Status int

// Status values. StatusUnknown is the zero value and covers untriaged
// cross-phase rows that declare no status — those are never suggested.
const (
	StatusUnknown Status = iota
	StatusPending
	StatusInFlight
	StatusBlocked
	StatusDone
)

// String returns the lower-case status name.
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusInFlight:
		return "in-flight"
	case StatusBlocked:
		return "blocked"
	case StatusDone:
		return "done"
	case StatusUnknown:
		return "unknown"
	}
	return "unknown"
}

// Priority is an explicit urgency declared in a Priority column. It is
// Unset when the column is absent (most BACKLOG.md tickets), in which case
// the Ranker derives priority from phase order instead.
type Priority int

// Priority values, most urgent last so the zero value (Unset) sorts below
// any explicit priority.
const (
	PriorityUnset Priority = iota
	PriorityLow
	PriorityMed
	PriorityHigh
)

// String returns the upper-case priority name.
func (p Priority) String() string {
	switch p {
	case PriorityHigh:
		return "HIGH"
	case PriorityMed:
		return "MED"
	case PriorityLow:
		return "LOW"
	case PriorityUnset:
		return "—"
	}
	return "—"
}

// Effort is the estimated size from the Size column. Sizes follow
// BACKLOG.md § Sizes: S ≤ 3 days, M ≤ 2 weeks, L ≤ 4 weeks.
type Effort int

// Effort values. EffortUnknown is treated as EffortMedium by the Ranker.
const (
	EffortUnknown Effort = iota
	EffortSmall
	EffortMedium
	EffortLarge
)

// String returns the single-letter size code.
func (e Effort) String() string {
	switch e {
	case EffortSmall:
		return "S"
	case EffortMedium:
		return "M"
	case EffortLarge:
		return "L"
	case EffortUnknown:
		return "?"
	}
	return "?"
}

// Item is one declared backlog ticket. It mirrors the ADR-022
// Orchestrator work-item input contract projected onto what the markdown
// backlog actually declares.
type Item struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Phase    int      `json:"phase"` // numeric prefix of ID; -1 for cross-phase
	Priority Priority `json:"priority"`
	Effort   Effort   `json:"effort"`
	Status   Status   `json:"status"`
	Deps     []string `json:"deps,omitempty"` // explicit IDs; "N.*" is a phase wildcard
	Section  string   `json:"section,omitempty"`
	Files    string   `json:"files,omitempty"`
}

// IsPhaseWildcard reports whether dep is a "N.*" phase wildcard dependency.
func IsPhaseWildcard(dep string) bool {
	return strings.HasSuffix(dep, ".*") && len(dep) > 2
}

// parseStatus classifies a Status-column cell. It is deliberately tolerant:
// "done", "done (#40)", "done — note", strikethrough "~~x~~ DONE",
// "in-flight (branch)", "blocked (reason)", "pending".
func parseStatus(cell string) Status {
	c := strings.ToLower(cleanCell(cell))
	switch {
	case c == "":
		return StatusUnknown
	case strings.Contains(c, "in-flight") || strings.Contains(c, "in flight"):
		return StatusInFlight
	case strings.HasPrefix(c, "blocked"):
		return StatusBlocked
	case strings.Contains(c, "done"):
		return StatusDone
	case strings.HasPrefix(c, "pending"):
		return StatusPending
	default:
		return StatusUnknown
	}
}

// inferStatusFromRow is the fallback for tables with no Status column
// (the cross-phase "| # | Title | Notes |" layout). Strikethrough or a
// DONE marker means done; anything else is left Unknown (never suggested).
func inferStatusFromRow(rowText string) Status {
	t := strings.ToLower(rowText)
	if strings.Contains(t, "~~") || strings.Contains(t, "done") {
		return StatusDone
	}
	return StatusUnknown
}

// parsePriority maps HIGH/MED/LOW and P0..P3 to a Priority. P0/P1 are
// urgent (High), P2 Med, P3 Low. Single letters are not matched to avoid
// colliding with the S/M/L Size codes.
func parsePriority(cell string) Priority {
	c := strings.ToUpper(cleanCell(cell))
	switch c {
	case "HIGH", "P0", "P1":
		return PriorityHigh
	case "MED", "MEDIUM", "P2":
		return PriorityMed
	case "LOW", "P3":
		return PriorityLow
	default:
		return PriorityUnset
	}
}

// parseEffort maps the Size column to an Effort.
func parseEffort(cell string) Effort {
	switch strings.ToUpper(cleanCell(cell)) {
	case "S", "SMALL":
		return EffortSmall
	case "M", "MEDIUM":
		return EffortMedium
	case "L", "LARGE":
		return EffortLarge
	default:
		return EffortUnknown
	}
}

// parseDeps splits a Dep cell into dependency IDs. "—", "-", "n/a" and the
// empty string mean no dependencies. Separators are comma or whitespace.
// A "N.*" token is preserved verbatim as a phase wildcard.
func parseDeps(cell string) []string {
	// Not cleanCell: that strips "*", which would mangle a "N.*" wildcard.
	c := strings.TrimSpace(cell)
	c = strings.ReplaceAll(c, "`", "")
	c = strings.ReplaceAll(c, "~~", "")
	c = strings.TrimSpace(c)
	if c == "" || c == "—" || c == "-" || strings.EqualFold(c, "n/a") || strings.EqualFold(c, "none") {
		return nil
	}
	fields := strings.FieldsFunc(c, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(strings.Trim(f, "`"))
		if f != "" {
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parsePhase extracts the numeric phase prefix from an item ID. "0.1" → 0,
// "6.3" → 6, "X.1" and "DEAR-X.5" → -1 (cross-phase).
func parsePhase(id string) int {
	id = strings.TrimSpace(id)
	dot := strings.IndexByte(id, '.')
	if dot <= 0 {
		return -1
	}
	n, err := strconv.Atoi(id[:dot])
	if err != nil {
		return -1
	}
	return n
}

// cleanCell strips markdown decoration (backticks, asterisks, strikethrough
// tildes) and surrounding whitespace from a table cell.
func cleanCell(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "~~", "")
	return strings.TrimSpace(s)
}

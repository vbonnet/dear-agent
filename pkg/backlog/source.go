package backlog

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// Source yields backlog items. Implementations may read markdown, an issue
// tracker, or anything else; the Ranker and Suggester are source-agnostic.
type Source interface {
	// Name identifies the source for logs and the CLI banner.
	Name() string
	// Items returns every backlog item the source can see. Returning an
	// empty slice with a nil error is valid (e.g. a file with no tables).
	Items(ctx context.Context) ([]Item, error)
}

// MarkdownSource parses GitHub-flavored markdown table rows from one or
// more files. Column meaning is resolved by normalized header name, not
// position, so it reads both supported seven-column and four-column layouts.
type MarkdownSource struct {
	Paths []string
}

// NewMarkdownSource builds a MarkdownSource over the given file paths.
func NewMarkdownSource(paths ...string) *MarkdownSource {
	return &MarkdownSource{Paths: paths}
}

// Name implements Source.
func (m *MarkdownSource) Name() string {
	return "markdown(" + strings.Join(m.Paths, ",") + ")"
}

// Items implements Source. Every explicitly supplied path must exist and be
// readable; an empty parsed document is still a valid source.
func (m *MarkdownSource) Items(_ context.Context) ([]Item, error) {
	var all []Item
	for _, p := range m.Paths {
		f, err := os.Open(p) //nolint:gosec // paths are operator-supplied CLI args
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", p, err)
		}
		items, perr := parseMarkdown(f)
		_ = f.Close()
		if perr != nil {
			return nil, fmt.Errorf("parse %s: %w", p, perr)
		}
		all = append(all, items...)
	}
	return all, nil
}

// colMap maps a logical field name to its column index in a table.
type colMap map[string]int

// parseMarkdown scans a markdown document and extracts items from every
// table whose header has an identifiable ID column and a Title column.
func parseMarkdown(r io.Reader) ([]Item, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		out     []Item
		section string
		header  []string
		cols    colMap
		inTable bool
	)
	prev := "" // previous line, needed to confirm a header via its separator

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		if h, ok := headingText(trimmed); ok {
			section = h
			inTable = false
			prev = line
			continue
		}

		if !isTableRow(trimmed) {
			inTable = false
			prev = line
			continue
		}

		switch {
		case !inTable && isSeparatorRow(trimmed):
			// Separator confirms the previous line was a header.
			if hcells := splitRow(strings.TrimSpace(prev)); len(hcells) > 0 {
				header = hcells
				cols = classifyColumns(header)
				inTable = cols.usable()
			}
		case inTable && isSeparatorRow(trimmed):
			// Tolerate a stray separator inside a table.
		case inTable:
			if it, ok := rowToItem(splitRow(trimmed), cols, section); ok {
				out = append(out, it)
			}
		}
		prev = line
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

// usable reports whether a classified table has the minimum columns to
// produce items: an ID and a Title.
func (c colMap) usable() bool {
	_, hasID := c["id"]
	_, hasTitle := c["title"]
	return hasID && hasTitle
}

// headingText returns the text of a "## ..." / "### ..." heading.
func headingText(line string) (string, bool) {
	if !strings.HasPrefix(line, "##") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimLeft(line, "# ")), true
}

// isTableRow reports whether a trimmed line looks like a table row.
func isTableRow(line string) bool {
	return strings.HasPrefix(line, "|") && strings.Count(line, "|") >= 2
}

// isSeparatorRow reports whether a trimmed line is a "|---|:--:|" divider.
func isSeparatorRow(line string) bool {
	if !isTableRow(line) {
		return false
	}
	for _, cell := range splitRow(line) {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		if strings.Trim(cell, "-: ") != "" {
			return false
		}
	}
	return true
}

// splitRow splits a "| a | b | c |" line into trimmed cell strings,
// dropping the empty cells produced by the leading and trailing pipes.
func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

// classifyColumns maps logical field names to column indices using the
// normalized header cells.
func classifyColumns(header []string) colMap {
	cols := colMap{}
	for i, h := range header {
		switch normHeader(h) {
		case "#", "id", "ticket", "ticketid":
			setOnce(cols, "id", i)
		case "title", "name":
			setOnce(cols, "title", i)
		case "status":
			setOnce(cols, "status", i)
		case "dep", "deps", "depends", "dependency", "dependencies":
			setOnce(cols, "dep", i)
		case "size", "effort":
			setOnce(cols, "effort", i)
		case "priority", "prio":
			setOnce(cols, "priority", i)
		case "files", "slot", "scope", "notes":
			setOnce(cols, "files", i)
		}
	}
	return cols
}

// setOnce records the first column index seen for a field, so a stray
// later column with a colliding header does not clobber the real one.
func setOnce(c colMap, key string, idx int) {
	if _, ok := c[key]; !ok {
		c[key] = idx
	}
}

// normHeader normalizes a header cell for classification: lower-cased,
// markdown stripped, and spaces removed.
func normHeader(h string) string {
	h = strings.ToLower(cleanCell(h))
	return strings.ReplaceAll(h, " ", "")
}

// rowToItem builds an Item from a data row given the column map. It returns
// ok=false for rows with no usable ID (blank lines, sub-bullets).
func rowToItem(cells []string, cols colMap, section string) (Item, bool) {
	id := cleanCell(cellAt(cells, cols, "id"))
	title := cleanCell(cellAt(cells, cols, "title"))
	if id == "" || title == "" {
		return Item{}, false
	}

	var status Status
	if _, ok := cols["status"]; ok {
		status = parseStatus(cellAt(cells, cols, "status"))
	} else {
		status = inferStatusFromRow(strings.Join(cells, " "))
	}

	return Item{
		ID:       id,
		Title:    title,
		Phase:    parsePhase(id),
		Priority: parsePriority(cellAt(cells, cols, "priority")),
		Effort:   parseEffort(cellAt(cells, cols, "effort")),
		Status:   status,
		Deps:     parseDeps(cellAt(cells, cols, "dep")),
		Section:  section,
		Files:    cleanCell(cellAt(cells, cols, "files")),
	}, true
}

// cellAt returns the cell for a logical field, or "" if the column is
// absent or the row is too short.
func cellAt(cells []string, cols colMap, field string) string {
	i, ok := cols[field]
	if !ok || i >= len(cells) {
		return ""
	}
	return cells[i]
}

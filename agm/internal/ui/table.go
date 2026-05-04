package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/vbonnet/dear-agent/agm/internal/activity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"golang.org/x/term"
)

// Lipgloss style functions for table formatting (using palette)
func getActiveStyle() lipgloss.Style {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)
	return lipgloss.NewStyle().
		Foreground(palette.Active).
		Bold(true)
}

func getStoppedStyle() lipgloss.Style {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)
	return lipgloss.NewStyle().
		Foreground(palette.Stopped)
}

func getHeaderStyle() lipgloss.Style {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)
	return lipgloss.NewStyle().
		Bold(true).
		Underline(true).
		Foreground(palette.Header)
}


// LayoutMode represents the terminal width-based layout mode
type LayoutMode int

const (
	LayoutMinimal LayoutMode = iota
	LayoutCompact
	LayoutFull
)

// getTerminalWidth detects the current terminal width
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width == 0 {
		return 80 // Fallback for non-TTY (pipes, redirects)
	}
	return width
}

// selectLayout chooses the appropriate layout mode based on terminal width
func selectLayout(width int) LayoutMode {
	if width < 80 {
		return LayoutMinimal
	} else if width < 100 {
		return LayoutCompact
	}
	return LayoutFull
}

// renderMinimalHeader renders a compact header for minimal layout with column headers
func renderMinimalHeader(status string, count int) string {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)

	displayStatus := strings.ToUpper(status)

	// Status line
	statusLine := fmt.Sprintf("%s (%d)", displayStatus, count)

	// Column headers line: Symbol(3) + NAME(20) + UUID(8) + WORKSPACE(8) + AGENT(6) + ACTIVITY(10)
	columnHeaders := fmt.Sprintf("   %-20s  %-8s  %-8s  %-6s  %10s",
		"NAME", "UUID", "WORKSPACE", "AGENT", "ACTIVITY")

	divider := strings.Repeat("━", 80)

	statusStyle := lipgloss.NewStyle().Foreground(palette.Header)
	columnStyle := lipgloss.NewStyle().Foreground(palette.Header)
	dividerStyle := lipgloss.NewStyle().Foreground(palette.Dim)

	return statusStyle.Render(statusLine) + "\n" + dividerStyle.Render(divider) + "\n" + columnStyle.Render(columnHeaders)
}

// renderMinimalTable renders session list in minimal layout (60-79 cols)
// Columns: Symbol + Name(20) + Workspace(8) + Agent(6) + Activity(10)
func renderMinimalTable(
	group []*manifest.Manifest,
	status string,
	statuses map[string]session.StatusInfo,
	activityMap map[string]string,
) string {
	var result bytes.Buffer

	// Render each row
	for _, m := range group {
		statusInfo := statuses[m.Name]

		// Choose style based on status and age
		var style lipgloss.Style
		switch status {
		case "active":
			style = getActiveStyle()
		case "stopped":
			// Age-based dimming for stopped sessions
			if !m.UpdatedAt.IsZero() && time.Since(m.UpdatedAt) >= 7*24*time.Hour {
				style = getStoppedStyle().Faint(true)
			} else {
				style = getStoppedStyle()
			}
		default:
			style = lipgloss.NewStyle()
		}

		// Determine status symbol
		var symbol string
		if status == "active" {
			if statusInfo.AttachedClients > 0 {
				symbol = getStatusSymbol("attached")
			} else {
				symbol = getStatusSymbol("detached")
			}
		} else {
			symbol = getStatusSymbol(status)
		}

		// Truncate session name to 20 chars
		name := m.Name
		if len(name) > 20 {
			name = name[:17] + "..."
		}

		// UUID column (short UUID, max 8 chars)
		shortUUID := extractShortUUID(m.Claude.UUID)
		if len(shortUUID) > 8 {
			shortUUID = shortUUID[:8]
		}

		// Workspace column (max 8 chars, show "-" if empty)
		workspace := m.Workspace
		if workspace == "" {
			workspace = "-"
		}
		if len(workspace) > 8 {
			workspace = workspace[:8]
		}

		// Agent column (max 6 chars)
		agent := m.Harness
		if len(agent) > 6 {
			agent = agent[:6]
		}

		// Activity (from pre-computed map)
		recency := activityMap[m.Name]

		// Format line: Symbol + 2sp + Name(20) + 2sp + UUID(8) + 2sp + Workspace(8) + 2sp + Agent(6) + 2sp + Activity(10, right-aligned)
		line := fmt.Sprintf("%s  %-20s  %-8s  %-8s  %-6s  %10s",
			symbol,
			name,
			shortUUID,
			workspace,
			agent,
			recency)

		result.WriteString(style.Render(line))
		result.WriteString("\n")
	}

	return result.String()
}

// renderCompactHeader renders header for compact layout with column headers
func renderCompactHeader(status string, count int) string {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)

	displayStatus := strings.ToUpper(status)

	// Status line
	statusLine := fmt.Sprintf("%s (%d)", displayStatus, count)

	// Column headers line: Symbol(3) + NAME(24) + UUID(8) + WORKSPACE(9) + AGENT(6) + PROJECT(20) + ACTIVITY(10)
	columnHeaders := fmt.Sprintf("   %-24s  %-8s  %-9s  %-6s  %-20s  %10s",
		"NAME", "UUID", "WORKSPACE", "AGENT", "PROJECT", "ACTIVITY")

	divider := strings.Repeat("━", 100)

	statusStyle := lipgloss.NewStyle().Foreground(palette.Header)
	columnStyle := lipgloss.NewStyle().Foreground(palette.Header)
	dividerStyle := lipgloss.NewStyle().Foreground(palette.Dim)

	return statusStyle.Render(statusLine) + "\n" + dividerStyle.Render(divider) + "\n" + columnStyle.Render(columnHeaders)
}

// renderCompactTable renders session list in compact layout (80-99 cols)
// Columns: Symbol + Name(30) + Workspace(8) + Agent(6) + Project(25) + Activity(10)
func renderCompactTable(
	group []*manifest.Manifest,
	status string,
	statuses map[string]session.StatusInfo,
	activityMap map[string]string,
) string {
	var result bytes.Buffer

	// Render each row
	for _, m := range group {
		statusInfo := statuses[m.Name]

		// Choose style based on status and age
		var style lipgloss.Style
		switch status {
		case "active":
			style = getActiveStyle()
		case "stopped":
			// Age-based dimming for stopped sessions
			if !m.UpdatedAt.IsZero() && time.Since(m.UpdatedAt) >= 7*24*time.Hour {
				style = getStoppedStyle().Faint(true)
			} else {
				style = getStoppedStyle()
			}
		default:
			style = lipgloss.NewStyle()
		}

		// Determine status symbol
		var symbol string
		if status == "active" {
			if statusInfo.AttachedClients > 0 {
				symbol = getStatusSymbol("attached")
			} else {
				symbol = getStatusSymbol("detached")
			}
		} else {
			symbol = getStatusSymbol(status)
		}

		// Truncate session name to 24 chars
		name := m.Name
		if len(name) > 24 {
			name = name[:21] + "..."
		}

		// UUID column (short UUID, max 8 chars)
		shortUUID := extractShortUUID(m.Claude.UUID)
		if len(shortUUID) > 8 {
			shortUUID = shortUUID[:8]
		}

		// Workspace column (max 9 chars, show "-" if empty)
		workspace := m.Workspace
		if workspace == "" {
			workspace = "-"
		}
		if len(workspace) > 9 {
			workspace = workspace[:9]
		}

		// Agent column (max 6 chars)
		agent := m.Harness
		if len(agent) > 6 {
			agent = agent[:6]
		}

		// Project path (truncate and compact)
		project := compactPath(truncatePath(m.Context.Project, 20))

		// Activity (from pre-computed map)
		recency := activityMap[m.Name]

		// Format line: Symbol + 2sp + Name(24) + 2sp + UUID(8) + 2sp + Workspace(9) + 2sp + Agent(6) + 2sp + Project(20) + 2sp + Activity(10, right-aligned)
		line := fmt.Sprintf("%s  %-24s  %-8s  %-9s  %-6s  %-20s  %10s",
			symbol,
			name,
			shortUUID,
			workspace,
			agent,
			project,
			recency)

		result.WriteString(style.Render(line))
		result.WriteString("\n")
	}

	return result.String()
}

// getSessionActivityBatch efficiently computes activity for all sessions in one pass.
// Groups sessions by agent type and uses batch methods to avoid reading history files multiple times.
// Returns a map of session name -> formatted activity string.
func getSessionActivityBatch(manifests []*manifest.Manifest) map[string]string {
	result := make(map[string]string, len(manifests))

	// Group sessions by agent type
	claudeSessions := make([]string, 0)
	claudeNameToKey := make(map[string]string) // manifest name -> UUID
	geminiSessions := make([]string, 0)
	geminiNameToKey := make(map[string]string) // manifest name -> sessionID

	for _, m := range manifests {
		switch m.Harness {
		case "claude":
			if m.Claude.UUID != "" {
				claudeSessions = append(claudeSessions, m.Claude.UUID)
				claudeNameToKey[m.Name] = m.Claude.UUID
			}
		case "gemini":
			if m.SessionID != "" {
				geminiSessions = append(geminiSessions, m.SessionID)
				geminiNameToKey[m.Name] = m.SessionID
			}
		}
	}

	// Batch fetch Claude activities
	if len(claudeSessions) > 0 {
		tracker := activity.NewClaudeActivityTracker()
		activities := tracker.GetLastActivityBatch(claudeSessions)

		for name, uuid := range claudeNameToKey {
			if timestamp, found := activities[uuid]; found {
				result[name] = formatTimeCompact(timestamp)
			} else {
				result[name] = "unknown ⚠️"
			}
		}
	}

	// Batch fetch Gemini activities
	if len(geminiSessions) > 0 {
		tracker := activity.NewGeminiActivityTracker()
		activities := tracker.GetLastActivityBatch(geminiSessions)

		for name, sessionID := range geminiNameToKey {
			if timestamp, found := activities[sessionID]; found {
				result[name] = formatTimeCompact(timestamp)
			} else {
				result[name] = "unknown ⚠️"
			}
		}
	}

	// For other agent types or missing activity data, use UpdatedAt
	for _, m := range manifests {
		if _, exists := result[m.Name]; !exists {
			result[m.Name] = formatTimeCompact(m.UpdatedAt)
		}
	}

	return result
}

// FormatTable formats manifests with enhanced lipgloss styling and grouping
func FormatTable(manifests []*manifest.Manifest, tmux session.TmuxInterface) string {
	// Handle empty list
	if len(manifests) == 0 {
		return "No sessions found.\n\nCreate one:\n  agm session new <project-name>\n"
	}

	// Detect terminal width and select layout
	width := getTerminalWidth()
	layout := selectLayout(width)

	// Compute status and attachment info for all manifests
	statuses := session.ComputeStatusBatchWithInfo(manifests, tmux)

	// Compute activity for all sessions in batch (much faster than per-session calls)
	activityMap := getSessionActivityBatch(manifests)

	// Group manifests by status (attached/detached/stopped)
	groups := groupByStatus(manifests, statuses)

	// Sort groups: ACTIVE by attached/detached then alphabetically, others alphabetically
	sortGroups(groups, statuses)

	// Check if we should show TMUX column (only if any session has NAME != TMUX)
	showTmuxColumn := shouldShowTmuxColumn(manifests)

	// Count total sessions
	totalCount := 0
	for _, group := range groups {
		totalCount += len(group)
	}

	// Add overview header
	var sections []string
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)
	overviewStyle := lipgloss.NewStyle().
		Foreground(palette.Header).
		Bold(true)
	sections = append(sections, overviewStyle.Render(fmt.Sprintf("Sessions Overview (%d total)", totalCount)))
	sections = append(sections, "")

	// Merge attached and detached into single "ACTIVE" group
	attached := groups["attached"]
	detached := groups["detached"]
	activeGroup := make([]*manifest.Manifest, 0, len(attached)+len(detached))
	activeGroup = append(activeGroup, attached...)
	activeGroup = append(activeGroup, detached...)
	combinedGroups := map[string][]*manifest.Manifest{
		"active":   activeGroup,
		"stopped":  groups["stopped"],
		"archived": groups["archived"],
	}

	// Calculate maximum column widths across ALL groups for consistent alignment (only for full layout)
	var maxWidths columnWidths
	if layout == LayoutFull {
		maxWidths = calculateMaxColumnWidths(combinedGroups, statuses, showTmuxColumn)
	}

	// Render each group
	for _, statusKey := range []string{"active", "stopped", "archived"} {
		group := combinedGroups[statusKey]
		if len(group) == 0 {
			continue // Skip empty groups
		}

		// Render group header and table based on layout
		var header, table string
		switch layout {
		case LayoutMinimal:
			header = renderMinimalHeader(statusKey, len(group))
			table = renderMinimalTable(group, statusKey, statuses, activityMap)
		case LayoutCompact:
			header = renderCompactHeader(statusKey, len(group))
			table = renderCompactTable(group, statusKey, statuses, activityMap)
		case LayoutFull:
			header = renderGroupHeaderWithDivider(statusKey, len(group), maxWidths, showTmuxColumn)
			table = renderGroupTableWithWidths(group, statusKey, statuses, showTmuxColumn, maxWidths, activityMap)
		}
		sections = append(sections, header)
		sections = append(sections, table)
	}

	// Add helpful footer
	footer := renderFooter()
	sections = append(sections, footer)

	// Join sections with blank lines
	return lipgloss.JoinVertical(lipgloss.Left, sections...) + "\n"
}

// FormatTableLegacy formats manifests as aligned table (v2 schema, old style)
// Preserved for backward compatibility with --legacy flag
func FormatTableLegacy(manifests []*manifest.Manifest, tmux session.TmuxInterface) string {
	// Compute status for all manifests
	statuses := session.ComputeStatusBatch(manifests, tmux)

	// First pass: format entire table without color to get proper alignment
	var tableBuf bytes.Buffer
	w := tabwriter.NewWriter(&tableBuf, 0, 0, 2, ' ', 0)

	// Header
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
		"NAME",
		"TMUX",
		"AGENT",
		"STATUS",
		"UPDATED",
		"PROJECT")

	// Rows
	for _, m := range manifests {
		status := statuses[m.Name]
		updated := formatTime(m.UpdatedAt)
		project := truncatePath(m.Context.Project, 40)

		// Show "-" if session is its own parent (Name == Tmux.SessionName)
		tmuxDisplay := m.Tmux.SessionName
		if m.Name == m.Tmux.SessionName {
			tmuxDisplay = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			m.Name,
			tmuxDisplay,
			m.Harness,
			status,
			updated,
			project)
	}

	w.Flush()

	// Second pass: apply color to entire lines based on status
	var result bytes.Buffer
	lines := bytes.Split(tableBuf.Bytes(), []byte("\n"))

	// Color the header (first line)
	if len(lines) > 0 {
		result.WriteString(Bold(string(lines[0])))
		result.WriteString("\n")
	}

	// Color data rows based on status
	for i, m := range manifests {
		if i+1 >= len(lines) {
			break
		}
		line := string(lines[i+1])
		if line == "" {
			continue
		}

		status := statuses[m.Name]
		switch status {
		case "active":
			line = Green(line)
		case "stopped":
			line = Yellow(line)
		case "archived":
			line = Red(line)
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	return result.String()
}

// groupByStatus groups manifests into attached/detached/stopped categories
func groupByStatus(manifests []*manifest.Manifest, statuses map[string]session.StatusInfo) map[string][]*manifest.Manifest {
	groups := map[string][]*manifest.Manifest{
		"attached": {},
		"detached": {},
		"stopped":  {},
		"archived": {},
	}

	for _, m := range manifests {
		// Check if session is archived first
		if m.Lifecycle == manifest.LifecycleArchived {
			groups["archived"] = append(groups["archived"], m)
			continue
		}

		statusInfo := statuses[m.Name]
		switch statusInfo.Status {
		case "active":
			// Active sessions: distinguish attached vs detached
			if statusInfo.AttachedClients > 0 {
				groups["attached"] = append(groups["attached"], m)
			} else {
				groups["detached"] = append(groups["detached"], m)
			}
		case "stopped":
			groups["stopped"] = append(groups["stopped"], m)
		}
	}

	return groups
}

// sortGroups sorts sessions within each group.
// For "active" group: sorts by attached/detached first, then alphabetically by name.
// For other groups: sorts alphabetically by name.
func sortGroups(groups map[string][]*manifest.Manifest, statuses map[string]session.StatusInfo) {
	// Sort active group: attached first (with ●), then detached (with ◐), each subgroup alphabetically
	activeGroup := groups["active"]
	sort.Slice(activeGroup, func(i, j int) bool {
		statusI := statuses[activeGroup[i].Name]
		statusJ := statuses[activeGroup[j].Name]

		// First, sort by attachment status
		attachedI := statusI.AttachedClients > 0
		attachedJ := statusJ.AttachedClients > 0

		if attachedI != attachedJ {
			return attachedI // attached (true) comes before detached (false)
		}

		// Within same attachment status, sort alphabetically
		return strings.ToLower(activeGroup[i].Name) < strings.ToLower(activeGroup[j].Name)
	})
	groups["active"] = activeGroup

	// Sort other groups alphabetically
	for groupName, group := range groups {
		if groupName == "active" {
			continue // already sorted above
		}
		sort.Slice(group, func(i, j int) bool {
			return strings.ToLower(group[i].Name) < strings.ToLower(group[j].Name)
		})
		groups[groupName] = group
	}
}

// shouldShowTmuxColumn returns true if any session has NAME != TMUX
func shouldShowTmuxColumn(manifests []*manifest.Manifest) bool {
	// TMUX column disabled - always return false
	return false
}

// renderGroupHeader renders a styled group header (e.g., "Attached Sessions (4)")
func renderGroupHeader(status string, count int) string {
	var displayStatus string
	switch status {
	case "attached":
		displayStatus = "Attached"
	case "detached":
		displayStatus = "Detached"
	case "stopped":
		displayStatus = "Stopped"
	case "active":
		displayStatus = "ACTIVE"
	default:
		// Status values are short ASCII tokens (e.g. "running"), so simple
		// first-byte upper-case suffices.
		if status != "" {
			displayStatus = strings.ToUpper(status[:1]) + status[1:]
		}
	}
	text := fmt.Sprintf("%s Sessions (%d)", displayStatus, count)
	return getHeaderStyle().Render(text)
}

// renderGroupHeaderWithDivider renders a group header with visual divider line and column headers
func renderGroupHeaderWithDivider(status string, count int, widths columnWidths, showTmuxColumn bool) string {
	var displayStatus string
	switch status {
	case "active":
		displayStatus = "ACTIVE"
	case "stopped":
		displayStatus = "STOPPED"
	default:
		displayStatus = strings.ToUpper(status)
	}

	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)

	// Build column headers line
	// Format: Symbol(1) + Space(2) + NAME + Space(2) + UUID + Space(2) + WORKSPACE + Space(2) + AGENT + Space(2) + PROJECT + Space(2) + ACTIVITY
	var columnHeaders string
	if showTmuxColumn {
		columnHeaders = fmt.Sprintf("   %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %10s",
			widths.name, "NAME",
			widths.tmux, "TMUX",
			widths.uuid, "UUID",
			widths.workspace, "WORKSPACE",
			widths.agent, "AGENT",
			widths.project, "PROJECT",
			"ACTIVITY")
	} else {
		columnHeaders = fmt.Sprintf("   %-*s  %-*s  %-*s  %-*s  %-*s  %10s",
			widths.name, "NAME",
			widths.uuid, "UUID",
			widths.workspace, "WORKSPACE",
			widths.agent, "AGENT",
			widths.project, "PROJECT",
			"ACTIVITY")
	}

	// Create status line with count
	statusLine := fmt.Sprintf("%s (%d)", displayStatus, count)

	// Create divider line - use a consistent generous width
	dividerWidth := len(columnHeaders) + 4
	divider := strings.Repeat("━", dividerWidth)

	// Style the headers
	statusStyle := lipgloss.NewStyle().
		Foreground(palette.Header).
		Bold(false)

	columnStyle := lipgloss.NewStyle().
		Foreground(palette.Header).
		Bold(false)

	dividerStyle := lipgloss.NewStyle().
		Foreground(palette.Dim)

	return statusStyle.Render(statusLine) + "\n" + dividerStyle.Render(divider) + "\n" + columnStyle.Render(columnHeaders)
}

// renderFooter renders helpful commands at the bottom
func renderFooter() string {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)

	footerStyle := lipgloss.NewStyle().
		Foreground(palette.Info)

	return "\n" + footerStyle.Render("💡 Resume: agm resume <name>  |  Archive: agm session archive <name>  |  Kill: agm session kill <name>")
}

// getStatusSymbol returns the Unicode symbol for a status
func getStatusSymbol(status string) string {
	cfg := GetGlobalConfig()

	symbolMap := map[string]string{
		"attached": "●", // U+25CF filled circle
		"detached": "◐", // U+25D0 circle with left half black
		"stopped":  "○", // U+25CB empty circle
	}

	symbol := symbolMap[status]

	// Check screen-reader mode
	if cfg.UI.ScreenReader || os.Getenv("AGM_SCREEN_READER") != "" {
		// Update ScreenReaderText to handle our new symbols
		switch symbol {
		case "●":
			return "[ATTACHED]"
		case "◐":
			return "[DETACHED]"
		case "○":
			return "[STOPPED]"
		default:
			return ScreenReaderText(symbol)
		}
	}

	return symbol
}

// extractShortUUID extracts everything before the first "-" from a UUID
func extractShortUUID(uuid string) string {
	if uuid == "" {
		return "-"
	}
	parts := strings.Split(uuid, "-")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return "-"
}

// columnWidths holds the maximum width for each column
type columnWidths struct {
	name      int
	tmux      int
	uuid      int
	workspace int
	agent     int
	project   int
}

// calculateMaxColumnWidths calculates the maximum width for each column across all groups
func calculateMaxColumnWidths(groups map[string][]*manifest.Manifest, statuses map[string]session.StatusInfo, showTmuxColumn bool) columnWidths {
	widths := columnWidths{}

	for _, group := range groups {
		for _, m := range group {
			// Name column
			if len(m.Name) > widths.name {
				widths.name = len(m.Name)
			}

			// Tmux column
			if showTmuxColumn && len(m.Tmux.SessionName) > widths.tmux {
				widths.tmux = len(m.Tmux.SessionName)
			}

			// UUID column (short UUID before first "-")
			shortUUID := extractShortUUID(m.Claude.UUID)
			if len(shortUUID) > widths.uuid {
				widths.uuid = len(shortUUID)
			}

			// Workspace column
			workspace := m.Workspace
			if workspace == "" {
				workspace = "-"
			}
			if len(workspace) > widths.workspace {
				widths.workspace = len(workspace)
			}

			// Agent column
			if len(m.Harness) > widths.agent {
				widths.agent = len(m.Harness)
			}

			// Project column
			project := compactPath(truncatePath(m.Context.Project, 40))
			if len(project) > widths.project {
				widths.project = len(project)
			}
		}
	}

	// Ensure minimum widths for column headers
	if widths.name < 4 {
		widths.name = 4 // "NAME" header
	}
	if widths.uuid < 4 {
		widths.uuid = 4 // "UUID" header
	}
	if widths.workspace < 9 {
		widths.workspace = 9 // "WORKSPACE" header
	}
	if widths.agent < 5 {
		widths.agent = 5 // "AGENT" header
	}
	if widths.project < 7 {
		widths.project = 7 // "PROJECT" header
	}

	return widths
}

// renderGroupTableWithWidths renders a table with fixed column widths for consistent alignment
func renderGroupTableWithWidths(group []*manifest.Manifest, status string, statuses map[string]session.StatusInfo, showTmuxColumn bool, widths columnWidths, activityMap map[string]string) string {
	var result bytes.Buffer

	// Render each row with fixed widths
	for _, m := range group {
		statusInfo := statuses[m.Name]

		// Choose style based on status and age
		var style lipgloss.Style
		switch status {
		case "active":
			style = getActiveStyle()
		case "stopped":
			// Age-based dimming for stopped sessions
			if !m.UpdatedAt.IsZero() && time.Since(m.UpdatedAt) >= 7*24*time.Hour {
				style = getStoppedStyle().Faint(true)
			} else {
				style = getStoppedStyle()
			}
		default:
			style = lipgloss.NewStyle()
		}

		// Determine status symbol
		var symbol string
		if status == "active" {
			if statusInfo.AttachedClients > 0 {
				symbol = getStatusSymbol("attached")
			} else {
				symbol = getStatusSymbol("detached")
			}
		} else {
			symbol = getStatusSymbol(status)
		}

		// Workspace (show "-" if empty)
		workspace := m.Workspace
		if workspace == "" {
			workspace = "-"
		}

		project := compactPath(truncatePath(m.Context.Project, 40))
		recency := activityMap[m.Name]
		shortUUID := extractShortUUID(m.Claude.UUID)

		// Build line with fixed widths
		var line string
		if showTmuxColumn {
			// Show "-" if session is its own parent (Name == Tmux.SessionName)
			tmuxDisplay := m.Tmux.SessionName
			if m.Name == m.Tmux.SessionName {
				tmuxDisplay = "-"
			}
			line = fmt.Sprintf("%s  %-*s  %-*s  %-*s  %-*s  %-*s  %-*s  %10s",
				symbol,
				widths.name, m.Name,
				widths.tmux, tmuxDisplay,
				widths.uuid, shortUUID,
				widths.workspace, workspace,
				widths.agent, m.Harness,
				widths.project, project,
				recency)
		} else {
			line = fmt.Sprintf("%s  %-*s  %-*s  %-*s  %-*s  %-*s  %10s",
				symbol,
				widths.name, m.Name,
				widths.uuid, shortUUID,
				widths.workspace, workspace,
				widths.agent, m.Harness,
				widths.project, project,
				recency)
		}

		// lipgloss will handle padding to width (100 chars)
		result.WriteString(style.Render(line))
		result.WriteString("\n")
	}

	return result.String()
}


// FormatJSON formats manifests as JSON
func FormatJSON(manifests []*manifest.Manifest) (string, error) {
	data, err := json.MarshalIndent(manifests, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}

// formatTime formats time as relative (e.g., "2h ago", "3d ago")
func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Hour {
		mins := int(diff.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}

	return t.Format("2006-01-02")
}

// formatTimeCompact formats time as compact relative (e.g., "5m ago", "2h ago", "3d ago")
func formatTimeCompact(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "now"
	}
	if diff < time.Hour {
		mins := int(diff.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
	if diff < 30*24*time.Hour {
		weeks := int(diff.Hours() / 24 / 7)
		return fmt.Sprintf("%dw ago", weeks)
	}

	return t.Format("Jan 02")
}


// truncatePath truncates path with ... if too long
func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

// compactPath replaces ~/ with ~/ to make paths more compact
func compactPath(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path // If we can't get home dir, return unchanged
	}
	if strings.HasPrefix(path, homeDir+"/") {
		return "~/" + path[len(homeDir)+1:]
	}
	if path == homeDir {
		return "~"
	}
	return path
}

// ScreenReaderText returns accessible text for symbols
func ScreenReaderText(symbol string) string {
	switch symbol {
	case "✓":
		return "[SUCCESS]"
	case "❌":
		return "[ERROR]"
	case "⚠", "⚠️":
		return "[WARNING]"
	case "○":
		return "[INFO]"
	default:
		return symbol
	}
}

// PrintError prints error message with Problem → Cause → Solution format
func PrintError(err error, cause, solution string) {
	cfg := GetGlobalConfig()
	symbol := "❌"
	// Check --screen-reader flag first
	if cfg.UI.ScreenReader {
		symbol = ScreenReaderText(symbol)
	} else if os.Getenv("AGM_SCREEN_READER") != "" {
		// Also check env var for compatibility
		symbol = ScreenReaderText(symbol)
	}
	fmt.Printf("%s %s\n\n", Red(symbol), err.Error())
	if cause != "" {
		fmt.Printf("%s\n\n", cause)
	}
	if solution != "" {
		fmt.Printf("Try:\n%s\n", solution)
	}
}

// PrintSuccess prints success message
func PrintSuccess(message string) {
	cfg := GetGlobalConfig()
	symbol := "✓"
	// Check --screen-reader flag first
	if cfg.UI.ScreenReader {
		symbol = ScreenReaderText(symbol)
	} else if os.Getenv("AGM_SCREEN_READER") != "" {
		// Also check env var for compatibility
		symbol = ScreenReaderText(symbol)
	}
	fmt.Printf("%s %s\n", Green(symbol), message)
}

// PrintSuccessWithDetail prints success with additional context
func PrintSuccessWithDetail(message, detail string) {
	PrintSuccess(message)
	if detail != "" {
		fmt.Printf("  %s\n", detail)
	}
}

// PrintProgressStep prints a step in a multi-step process
func PrintProgressStep(step int, total int, message string) {
	fmt.Printf("%s [%d/%d] %s\n", Blue("→"), step, total, message)
}

// PrintWarning prints warning message
func PrintWarning(message string) {
	cfg := GetGlobalConfig()
	symbol := "⚠"
	// Check --screen-reader flag first
	if cfg.UI.ScreenReader {
		symbol = ScreenReaderText(symbol)
	} else if os.Getenv("AGM_SCREEN_READER") != "" {
		// Also check env var for compatibility
		symbol = ScreenReaderText(symbol)
	}
	fmt.Printf("%s %s\n", Yellow(symbol), message)
}

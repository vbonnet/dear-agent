package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
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

func getStaleStyle() lipgloss.Style {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)
	return lipgloss.NewStyle().
		Foreground(palette.Stale).
		Faint(true)
}

func getHeaderStyle() lipgloss.Style {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)
	return lipgloss.NewStyle().
		Bold(true).
		Underline(true).
		Foreground(palette.Header)
}

func getDimStyle() lipgloss.Style {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)
	return lipgloss.NewStyle().
		Foreground(palette.Dim)
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

// renderMinimalHeader renders a compact header for minimal layout
func renderMinimalHeader(status string, count int) string {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)

	displayStatus := strings.ToUpper(status)
	headerText := fmt.Sprintf("%s (%d)                    Activity", displayStatus, count)
	divider := strings.Repeat("━", 60)

	headerStyle := lipgloss.NewStyle().Foreground(palette.Header)
	dividerStyle := lipgloss.NewStyle().Foreground(palette.Dim)

	return headerStyle.Render(headerText) + "\n" + dividerStyle.Render(divider)
}

// renderMinimalTable renders session list in minimal layout (60-79 cols)
// Columns: Symbol + Name(20) + Agent(6) + Activity(10)
func renderMinimalTable(
	group []*manifest.Manifest,
	status string,
	statuses map[string]session.StatusInfo,
) string {
	var result bytes.Buffer

	// Choose style based on status
	var style lipgloss.Style
	switch status {
	case "active":
		style = getActiveStyle()
	case "stopped":
		style = getStoppedStyle()
	case "stale":
		style = getStaleStyle()
	default:
		style = lipgloss.NewStyle()
	}

	// Render each row
	for _, m := range group {
		statusInfo := statuses[m.Name]

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

		// Agent column (max 6 chars)
		agent := m.Agent
		if len(agent) > 6 {
			agent = agent[:6]
		}

		// Activity
		recency := formatTimeCompact(m.UpdatedAt)

		// Format line: Symbol + 2sp + Name(20) + 2sp + Agent(6) + 2sp + Activity
		line := fmt.Sprintf("%s  %-20s  %-6s  %s",
			symbol,
			name,
			agent,
			recency)

		result.WriteString(style.Render(line))
		result.WriteString("\n")
	}

	return result.String()
}

// renderCompactHeader renders header for compact layout
func renderCompactHeader(status string, count int) string {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)

	displayStatus := strings.ToUpper(status)
	headerText := fmt.Sprintf("%s (%d)                                         Activity", displayStatus, count)
	divider := strings.Repeat("━", 80)

	headerStyle := lipgloss.NewStyle().Foreground(palette.Header)
	dividerStyle := lipgloss.NewStyle().Foreground(palette.Dim)

	return headerStyle.Render(headerText) + "\n" + dividerStyle.Render(divider)
}

// renderCompactTable renders session list in compact layout (80-99 cols)
// Columns: Symbol + Name(30) + Agent(6) + Project(25) + Activity(10)
func renderCompactTable(
	group []*manifest.Manifest,
	status string,
	statuses map[string]session.StatusInfo,
) string {
	var result bytes.Buffer

	// Choose style based on status
	var style lipgloss.Style
	switch status {
	case "active":
		style = getActiveStyle()
	case "stopped":
		style = getStoppedStyle()
	case "stale":
		style = getStaleStyle()
	default:
		style = lipgloss.NewStyle()
	}

	// Render each row
	for _, m := range group {
		statusInfo := statuses[m.Name]

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

		// Truncate session name to 30 chars
		name := m.Name
		if len(name) > 30 {
			name = name[:27] + "..."
		}

		// Agent column (max 6 chars)
		agent := m.Agent
		if len(agent) > 6 {
			agent = agent[:6]
		}

		// Project path (truncate and compact)
		project := compactPath(truncatePath(m.Context.Project, 25))

		// Activity
		recency := formatTimeCompact(m.UpdatedAt)

		// Format line: Symbol + 2sp + Name(30) + 2sp + Agent(6) + 2sp + Project(25) + 2sp + Activity
		line := fmt.Sprintf("%s  %-30s  %-6s  %-25s  %s",
			symbol,
			name,
			agent,
			project,
			recency)

		result.WriteString(style.Render(line))
		result.WriteString("\n")
	}

	return result.String()
}

// FormatTable formats manifests with enhanced lipgloss styling and grouping
func FormatTable(manifests []*manifest.Manifest, tmux session.TmuxInterface) string {
	// Handle empty list
	if len(manifests) == 0 {
		return "No sessions found.\n\nCreate one:\n  csm new <project-name>\n"
	}

	// Detect terminal width and select layout
	width := getTerminalWidth()
	layout := selectLayout(width)

	// Compute status and attachment info for all manifests
	statuses := session.ComputeStatusBatchWithInfo(manifests, tmux)

	// Group manifests by status (attached/detached/stopped/stale)
	groups := groupByStatus(manifests, statuses)

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
	activeGroup := append(groups["attached"], groups["detached"]...)
	combinedGroups := map[string][]*manifest.Manifest{
		"active":  activeGroup,
		"stopped": groups["stopped"],
		"stale":   groups["stale"],
	}

	// Calculate maximum column widths across ALL groups for consistent alignment (only for full layout)
	var maxWidths columnWidths
	if layout == LayoutFull {
		maxWidths = calculateMaxColumnWidths(combinedGroups, statuses, showTmuxColumn)
	}

	// Render each group
	for _, statusKey := range []string{"active", "stopped", "stale"} {
		group := combinedGroups[statusKey]
		if len(group) == 0 {
			continue // Skip empty groups
		}

		// Render group header and table based on layout
		var header, table string
		switch layout {
		case LayoutMinimal:
			header = renderMinimalHeader(statusKey, len(group))
			table = renderMinimalTable(group, statusKey, statuses)
		case LayoutCompact:
			header = renderCompactHeader(statusKey, len(group))
			table = renderCompactTable(group, statusKey, statuses)
		case LayoutFull:
			header = renderGroupHeaderWithDivider(statusKey, len(group), maxWidths, showTmuxColumn)
			table = renderGroupTableWithWidths(group, statusKey, statuses, showTmuxColumn, maxWidths)
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

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			m.Name,
			m.Tmux.SessionName,
			m.Agent,
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

// groupByStatus groups manifests into attached/detached/stopped/stale categories
func groupByStatus(manifests []*manifest.Manifest, statuses map[string]session.StatusInfo) map[string][]*manifest.Manifest {
	groups := map[string][]*manifest.Manifest{
		"attached": {},
		"detached": {},
		"stopped":  {},
		"stale":    {},
	}

	staleThreshold := 7 * 24 * time.Hour

	for _, m := range manifests {
		statusInfo := statuses[m.Name]
		if statusInfo.Status == "active" {
			// Active sessions: distinguish attached vs detached
			if statusInfo.AttachedClients > 0 {
				groups["attached"] = append(groups["attached"], m)
			} else {
				groups["detached"] = append(groups["detached"], m)
			}
		} else if statusInfo.Status == "stopped" {
			// Check if session is stale (stopped for more than 7 days)
			if time.Since(m.UpdatedAt) >= staleThreshold {
				groups["stale"] = append(groups["stale"], m)
			} else {
				groups["stopped"] = append(groups["stopped"], m)
			}
		}
		// "archived" sessions are not shown in list output
	}

	return groups
}

// shouldShowTmuxColumn returns true if any session has NAME != TMUX
func shouldShowTmuxColumn(manifests []*manifest.Manifest) bool {
	for _, m := range manifests {
		if m.Name != m.Tmux.SessionName {
			return true
		}
	}
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
	case "stale":
		displayStatus = "Stale"
	case "active":
		displayStatus = "ACTIVE"
	default:
		displayStatus = strings.Title(status)
	}
	text := fmt.Sprintf("%s Sessions (%d)", displayStatus, count)
	return getHeaderStyle().Render(text)
}

// renderGroupHeaderWithDivider renders a group header with visual divider line and "Last Activity" column
func renderGroupHeaderWithDivider(status string, count int, widths columnWidths, showTmuxColumn bool) string {
	var displayStatus string
	switch status {
	case "active":
		displayStatus = "ACTIVE"
	case "stopped":
		displayStatus = "STOPPED"
	case "stale":
		displayStatus = "STALE"
	default:
		displayStatus = strings.ToUpper(status)
	}

	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)

	// Calculate where "Last Activity" should appear based on column widths
	// Format: Symbol(1) + Space(2) + Name + Space(2) + [Tmux + Space(2)] + Agent + Space(2) + Project + Space(2) + Attachment + Space(2) + Recency
	var recencyPosition int
	if showTmuxColumn {
		recencyPosition = 1 + 2 + widths.name + 2 + widths.tmux + 2 + widths.agent + 2 + widths.project + 2 + widths.attachment + 2
	} else {
		recencyPosition = 1 + 2 + widths.name + 2 + widths.agent + 2 + widths.project + 2 + widths.attachment + 2
	}

	// Create header line with status count and "Last Activity" positioned at recency column
	headerText := fmt.Sprintf("%s (%d)", displayStatus, count)
	rightText := "Last Activity"

	// Position "Last Activity" at the recency column
	spacing := recencyPosition - len(headerText)
	if spacing < 2 {
		spacing = 2
	}

	headerLine := headerText + strings.Repeat(" ", spacing) + rightText

	// Create divider line - use a consistent generous width
	// Should extend well past the recency column to provide visual separation
	dividerWidth := 100
	divider := strings.Repeat("━", dividerWidth)

	// Style the header and divider
	headerStyle := lipgloss.NewStyle().
		Foreground(palette.Header).
		Bold(false)

	dividerStyle := lipgloss.NewStyle().
		Foreground(palette.Dim)

	return headerStyle.Render(headerLine) + "\n" + dividerStyle.Render(divider)
}

// renderFooter renders helpful commands at the bottom
func renderFooter() string {
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)

	footerStyle := lipgloss.NewStyle().
		Foreground(palette.Info)

	return "\n" + footerStyle.Render("💡 Resume: csm resume <name>  |  Stop: csm stop <name>  |  Clean: csm clean")
}

// getStatusSymbol returns the Unicode symbol for a status
func getStatusSymbol(status string) string {
	cfg := GetGlobalConfig()

	symbolMap := map[string]string{
		"attached": "●", // U+25CF filled circle
		"detached": "◐", // U+25D0 circle with left half black
		"stopped":  "○", // U+25CB empty circle
		"stale":    "⊗", // U+2297 circled times
	}

	symbol := symbolMap[status]

	// Check screen-reader mode
	if cfg.UI.ScreenReader || os.Getenv("CSM_SCREEN_READER") != "" {
		// Update ScreenReaderText to handle our new symbols
		switch symbol {
		case "●":
			return "[ATTACHED]"
		case "◐":
			return "[DETACHED]"
		case "○":
			return "[STOPPED]"
		case "⊗":
			return "[STALE]"
		default:
			return ScreenReaderText(symbol)
		}
	}

	return symbol
}

// columnWidths holds the maximum width for each column
type columnWidths struct {
	name       int
	tmux       int
	agent      int
	project    int
	attachment int
}

// calculateMaxColumnWidths calculates the maximum width for each column across all groups
func calculateMaxColumnWidths(groups map[string][]*manifest.Manifest, statuses map[string]session.StatusInfo, showTmuxColumn bool) columnWidths {
	widths := columnWidths{}

	for status, group := range groups {
		for _, m := range group {
			// Name column
			if len(m.Name) > widths.name {
				widths.name = len(m.Name)
			}

			// Tmux column
			if showTmuxColumn && len(m.Tmux.SessionName) > widths.tmux {
				widths.tmux = len(m.Tmux.SessionName)
			}

			// Agent column
			if len(m.Agent) > widths.agent {
				widths.agent = len(m.Agent)
			}

			// Project column
			project := compactPath(truncatePath(m.Context.Project, 40))
			if len(project) > widths.project {
				widths.project = len(project)
			}

			// Attachment column (only for active sessions)
			if status == "active" {
				statusInfo := statuses[m.Name]
				if statusInfo.AttachedClients > 0 {
					attachmentText := fmt.Sprintf("%d attached", statusInfo.AttachedClients)
					if len(attachmentText) > widths.attachment {
						widths.attachment = len(attachmentText)
					}
				}
			}
		}
	}

	return widths
}

// renderGroupTableWithWidths renders a table with fixed column widths for consistent alignment
func renderGroupTableWithWidths(group []*manifest.Manifest, status string, statuses map[string]session.StatusInfo, showTmuxColumn bool, widths columnWidths) string {
	var result bytes.Buffer

	// Choose style based on status
	var style lipgloss.Style
	switch status {
	case "active":
		style = getActiveStyle().Width(100)
	case "stopped":
		style = getStoppedStyle().Width(100)
	case "stale":
		style = getStaleStyle().Width(100)
	default:
		style = lipgloss.NewStyle().Width(100)
	}

	// Render each row with fixed widths
	for _, m := range group {
		statusInfo := statuses[m.Name]

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

		project := compactPath(truncatePath(m.Context.Project, 40))
		recency := formatTimeCompact(m.UpdatedAt)

		// Format attachment count
		var attachmentText string
		if status == "active" && statusInfo.AttachedClients > 0 {
			attachmentText = fmt.Sprintf("%d attached", statusInfo.AttachedClients)
		}

		// Build line with fixed widths
		var line string
		if showTmuxColumn {
			line = fmt.Sprintf("%s  %-*s  %-*s  %-*s  %-*s  %-*s  %-10s",
				symbol,
				widths.name, m.Name,
				widths.tmux, m.Tmux.SessionName,
				widths.agent, m.Agent,
				widths.project, project,
				widths.attachment, attachmentText,
				recency)
		} else {
			line = fmt.Sprintf("%s  %-*s  %-*s  %-*s  %-*s  %-10s",
				symbol,
				widths.name, m.Name,
				widths.agent, m.Agent,
				widths.project, project,
				widths.attachment, attachmentText,
				recency)
		}

		// lipgloss will handle padding to width (100 chars)
		result.WriteString(style.Render(line))
		result.WriteString("\n")
	}

	return result.String()
}

// renderGroupTable renders a table for a single status group
func renderGroupTable(group []*manifest.Manifest, status string, statuses map[string]session.StatusInfo, showTmuxColumn bool) string {
	// First pass: format entire table without color to get proper alignment
	var tableBuf bytes.Buffer
	w := tabwriter.NewWriter(&tableBuf, 0, 0, 2, ' ', 0)

	// No header row - the divider already has column labels

	// Rows
	for _, m := range group {
		statusInfo := statuses[m.Name]

		// Determine status symbol for the group
		var symbol string
		// For "active" group, distinguish between attached and detached
		if status == "active" {
			if statusInfo.AttachedClients > 0 {
				symbol = getStatusSymbol("attached")
			} else {
				symbol = getStatusSymbol("detached")
			}
		} else {
			symbol = getStatusSymbol(status)
		}

		project := compactPath(truncatePath(m.Context.Project, 40))
		recency := formatTimeCompact(m.UpdatedAt)

		// Format attachment count column (only for active sessions)
		var attachmentText string
		if status == "active" && statusInfo.AttachedClients > 0 {
			attachmentText = fmt.Sprintf("%d attached", statusInfo.AttachedClients)
		} else {
			attachmentText = ""
		}

		if showTmuxColumn {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				symbol,
				m.Name,
				m.Tmux.SessionName,
				project,
				attachmentText,
				recency)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				symbol,
				m.Name,
				project,
				attachmentText,
				recency)
		}
	}

	w.Flush()

	// Second pass: apply lipgloss styling to entire lines
	var result bytes.Buffer
	lines := bytes.Split(tableBuf.Bytes(), []byte("\n"))

	// Choose style based on status
	var style lipgloss.Style
	switch status {
	case "active":
		style = getActiveStyle()
	case "stopped":
		style = getStoppedStyle()
	case "stale":
		style = getStaleStyle()
	default:
		style = lipgloss.NewStyle() // No styling
	}

	// Color all data rows (no header row)
	for i := range group {
		if i >= len(lines) {
			break
		}
		line := string(lines[i])
		if line == "" {
			continue
		}

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

// compactPath replaces /home/user/ with ~/ to make paths more compact
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
	} else if os.Getenv("CSM_SCREEN_READER") != "" {
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
	} else if os.Getenv("CSM_SCREEN_READER") != "" {
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
	} else if os.Getenv("CSM_SCREEN_READER") != "" {
		// Also check env var for compatibility
		symbol = ScreenReaderText(symbol)
	}
	fmt.Printf("%s %s\n", Yellow(symbol), message)
}

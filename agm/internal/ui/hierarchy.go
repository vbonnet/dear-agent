package ui

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vbonnet/dear-agent/agm/internal/db"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// HierarchyNode represents a session with hierarchy rendering information
type HierarchyNode struct {
	Session  *manifest.Manifest
	Depth    int
	Children []*HierarchyNode
	IsLast   bool // Whether this is the last child of its parent
}

// FormatTableWithHierarchy formats manifests as a hierarchical tree structure
func FormatTableWithHierarchy(nodes []*db.SessionNode, tmux session.TmuxInterface) string {
	if len(nodes) == 0 {
		return "No sessions found.\n\nCreate one:\n  agm session new <project-name>\n"
	}

	// Detect terminal width and select layout
	width := getTerminalWidth()
	layout := selectLayout(width)

	// Flatten the tree to get all manifests for status computation
	var allManifests []*manifest.Manifest
	var flattenTree func(nodes []*db.SessionNode)
	flattenTree = func(nodes []*db.SessionNode) {
		for _, node := range nodes {
			allManifests = append(allManifests, node.Session)
			flattenTree(node.Children)
		}
	}
	flattenTree(nodes)

	// Compute status and attachment info for all manifests
	statuses := session.ComputeStatusBatchWithInfo(allManifests, tmux)

	// Compute activity for all sessions in batch (much faster than per-session calls)
	activityMap := getSessionActivityBatch(allManifests)

	// Group manifests by status for counting
	groups := groupByStatus(allManifests, statuses)

	// Check if we should show TMUX column
	showTmuxColumn := shouldShowTmuxColumn(allManifests)

	// Count total sessions
	totalCount := len(allManifests)

	// Add overview header
	var sections []string
	cfg := GetGlobalConfig()
	palette := GetPalette(cfg.UI.Theme)
	overviewStyle := lipgloss.NewStyle().
		Foreground(palette.Header).
		Bold(true)
	sections = append(sections, overviewStyle.Render(fmt.Sprintf("Sessions Overview (%d total)", totalCount)))
	sections = append(sections, "")

	// Calculate maximum column widths for full layout
	var maxWidths columnWidths
	if layout == LayoutFull {
		maxWidths = calculateMaxColumnWidthsFlat(allManifests, showTmuxColumn)
	}

	// Render the hierarchical tree
	var hierarchyOutput string
	switch layout {
	case LayoutMinimal:
		hierarchyOutput = renderHierarchyMinimal(nodes, statuses, activityMap)
	case LayoutCompact:
		hierarchyOutput = renderHierarchyCompact(nodes, statuses, activityMap)
	case LayoutFull:
		hierarchyOutput = renderHierarchyFull(nodes, statuses, showTmuxColumn, maxWidths, activityMap)
	}

	sections = append(sections, hierarchyOutput)

	// Add summary footer showing group counts
	footerLines := []string{""}
	infoStyle := lipgloss.NewStyle().Foreground(palette.Info)

	statusCounts := []string{}
	if len(groups["attached"]) > 0 || len(groups["detached"]) > 0 {
		activeCount := len(groups["attached"]) + len(groups["detached"])
		statusCounts = append(statusCounts, fmt.Sprintf("%d active", activeCount))
	}
	if len(groups["stopped"]) > 0 {
		statusCounts = append(statusCounts, fmt.Sprintf("%d stopped", len(groups["stopped"])))
	}

	if len(statusCounts) > 0 {
		footerLines = append(footerLines, infoStyle.Render(fmt.Sprintf("Status: %s", strings.Join(statusCounts, ", "))))
	}

	footerLines = append(footerLines, infoStyle.Render("💡 Resume: agm resume <name>  |  Stop: agm stop <name>  |  Clean: agm clean"))
	sections = append(sections, strings.Join(footerLines, "\n"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...) + "\n"
}

// renderHierarchyMinimal renders the session hierarchy for minimal terminals (60-79 cols)
func renderHierarchyMinimal(nodes []*db.SessionNode, statuses map[string]session.StatusInfo, activityMap map[string]string) string {
	var result bytes.Buffer

	var renderNode func(node *db.SessionNode, prefix string, isLast bool)
	renderNode = func(node *db.SessionNode, prefix string, isLast bool) {
		m := node.Session
		statusInfo := statuses[m.Name]

		// Determine status and style
		var status string
		var style lipgloss.Style
		switch statusInfo.Status {
		case "active":
			status = "active"
			style = getActiveStyle()
		case "stopped":
			status = "stopped"
			style = getStoppedStyle()
		default:
			status = statusInfo.Status
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

		// Tree connector
		var connector string
		switch {
		case node.Depth == 0:
			connector = ""
		case isLast:
			connector = "└─ "
		default:
			connector = "├─ "
		}

		// Truncate session name to fit
		name := m.Name
		maxNameLen := 20 - len(prefix) - len(connector)
		if maxNameLen < 10 {
			maxNameLen = 10
		}
		if len(name) > maxNameLen {
			name = name[:maxNameLen-3] + "..."
		}

		// Activity (from pre-computed map)
		recency := activityMap[m.Name]

		// Depth indicator if child
		depthIndicator := ""
		if node.Depth > 0 {
			depthIndicator = fmt.Sprintf(" [d:%d]", node.Depth)
		}

		line := fmt.Sprintf("%s%s%s  %-*s%s  %s",
			prefix,
			connector,
			symbol,
			maxNameLen, name,
			depthIndicator,
			recency)

		result.WriteString(style.Render(line))
		result.WriteString("\n")

		// Render children
		newPrefix := prefix
		if node.Depth > 0 {
			if isLast {
				newPrefix += "   "
			} else {
				newPrefix += "│  "
			}
		}

		for i, child := range node.Children {
			renderNode(child, newPrefix, i == len(node.Children)-1)
		}
	}

	for _, node := range nodes {
		renderNode(node, "", true)
	}

	return result.String()
}

// renderHierarchyCompact renders the session hierarchy for compact terminals (80-99 cols)
func renderHierarchyCompact(nodes []*db.SessionNode, statuses map[string]session.StatusInfo, activityMap map[string]string) string {
	var result bytes.Buffer

	var renderNode func(node *db.SessionNode, prefix string, isLast bool)
	renderNode = func(node *db.SessionNode, prefix string, isLast bool) {
		m := node.Session
		statusInfo := statuses[m.Name]

		// Determine status and style
		var status string
		var style lipgloss.Style
		switch statusInfo.Status {
		case "active":
			status = "active"
			style = getActiveStyle()
		case "stopped":
			status = "stopped"
			style = getStoppedStyle()
		default:
			status = statusInfo.Status
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

		// Tree connector
		var connector string
		switch {
		case node.Depth == 0:
			connector = ""
		case isLast:
			connector = "└─ "
		default:
			connector = "├─ "
		}

		// Truncate session name
		name := m.Name
		maxNameLen := 30 - len(prefix) - len(connector)
		if maxNameLen < 15 {
			maxNameLen = 15
		}
		if len(name) > maxNameLen {
			name = name[:maxNameLen-3] + "..."
		}

		// Project path
		project := compactPath(truncatePath(m.Context.Project, 20))

		// Activity (from pre-computed map)
		recency := activityMap[m.Name]

		// Children count
		childrenInfo := ""
		if len(node.Children) > 0 {
			childrenInfo = fmt.Sprintf(" (%d)", len(node.Children))
		}

		line := fmt.Sprintf("%s%s%s  %-*s%s  %-20s  %s",
			prefix,
			connector,
			symbol,
			maxNameLen, name,
			childrenInfo,
			project,
			recency)

		result.WriteString(style.Render(line))
		result.WriteString("\n")

		// Render children
		newPrefix := prefix
		if node.Depth > 0 {
			if isLast {
				newPrefix += "   "
			} else {
				newPrefix += "│  "
			}
		}

		for i, child := range node.Children {
			renderNode(child, newPrefix, i == len(node.Children)-1)
		}
	}

	for _, node := range nodes {
		renderNode(node, "", true)
	}

	return result.String()
}

// renderHierarchyFull renders the session hierarchy for full-width terminals (100+ cols)
func renderHierarchyFull(nodes []*db.SessionNode, statuses map[string]session.StatusInfo, showTmuxColumn bool, widths columnWidths, activityMap map[string]string) string {
	var result bytes.Buffer

	var renderNode func(node *db.SessionNode, prefix string, isLast bool)
	renderNode = func(node *db.SessionNode, prefix string, isLast bool) {
		m := node.Session
		statusInfo := statuses[m.Name]

		// Determine status and style
		var status string
		var style lipgloss.Style
		switch statusInfo.Status {
		case "active":
			status = "active"
			style = getActiveStyle()
		case "stopped":
			status = "stopped"
			style = getStoppedStyle()
		default:
			status = statusInfo.Status
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

		// Tree connector
		var connector string
		switch {
		case node.Depth == 0:
			connector = ""
		case isLast:
			connector = "└─ "
		default:
			connector = "├─ "
		}

		// Format name with tree prefix
		nameWithTree := prefix + connector + m.Name

		// Children count
		childrenInfo := ""
		if len(node.Children) > 0 {
			childrenInfo = fmt.Sprintf(" (%d children)", len(node.Children))
		}

		project := compactPath(truncatePath(m.Context.Project, 40))
		recency := activityMap[m.Name]

		// Build line
		var line string
		if showTmuxColumn {
			line = fmt.Sprintf("%s  %s  %-*s  %-*s  %-*s%s  %-10s",
				symbol,
				nameWithTree,
				widths.tmux, m.Tmux.SessionName,
				widths.agent, m.Harness,
				widths.project, project,
				childrenInfo,
				recency)
		} else {
			line = fmt.Sprintf("%s  %s  %-*s  %-*s%s  %-10s",
				symbol,
				nameWithTree,
				widths.agent, m.Harness,
				widths.project, project,
				childrenInfo,
				recency)
		}

		result.WriteString(style.Render(line))
		result.WriteString("\n")

		// Render children
		newPrefix := prefix
		if node.Depth > 0 {
			if isLast {
				newPrefix += "   "
			} else {
				newPrefix += "│  "
			}
		}

		for i, child := range node.Children {
			renderNode(child, newPrefix, i == len(node.Children)-1)
		}
	}

	for _, node := range nodes {
		renderNode(node, "", true)
	}

	return result.String()
}

// calculateMaxColumnWidthsFlat calculates column widths from a flat list of manifests
func calculateMaxColumnWidthsFlat(manifests []*manifest.Manifest, showTmuxColumn bool) columnWidths {
	widths := columnWidths{}

	for _, m := range manifests {
		// Name column (not used in hierarchy, but kept for compatibility)
		if len(m.Name) > widths.name {
			widths.name = len(m.Name)
		}

		// Tmux column
		if showTmuxColumn && len(m.Tmux.SessionName) > widths.tmux {
			widths.tmux = len(m.Tmux.SessionName)
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

	return widths
}

// Package audit provides audit functionality.
package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/orphan"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"gopkg.in/yaml.v3"
)

// Severity levels for audit issues
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// IssueType identifies the type of audit issue
type IssueType string

const (
	IssueOrphanedConversation IssueType = "orphaned_conversation"
	IssueCorruptedManifest    IssueType = "corrupted_manifest"
	IssueMissingTmuxSession   IssueType = "missing_tmux_session"
	IssueStaleSession         IssueType = "stale_session"
	IssueDuplicateUUID        IssueType = "duplicate_uuid"
)

// AuditIssue represents a single audit finding
type AuditIssue struct {
	Type           IssueType `json:"type"`
	Severity       Severity  `json:"severity"`
	SessionID      string    `json:"session_id,omitempty"`
	UUID           string    `json:"uuid,omitempty"`
	Workspace      string    `json:"workspace,omitempty"`
	Path           string    `json:"path,omitempty"`
	Message        string    `json:"message"`
	Details        string    `json:"details,omitempty"`
	Recommendation string    `json:"recommendation"`
	DetectedAt     time.Time `json:"detected_at"`
}

// AuditReport summarizes the complete audit results
type AuditReport struct {
	// Metadata
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
	Duration        string    `json:"duration"`
	WorkspaceFilter string    `json:"workspace_filter,omitempty"`
	SeverityFilter  Severity  `json:"severity_filter,omitempty"`

	// Results
	Issues            []*AuditIssue     `json:"issues"`
	TotalIssues       int               `json:"total_issues"`
	IssuesBySeverity  map[Severity]int  `json:"issues_by_severity"`
	IssuesByType      map[IssueType]int `json:"issues_by_type"`
	IssuesByWorkspace map[string]int    `json:"issues_by_workspace"`

	// Statistics
	ManifestsScanned    int `json:"manifests_scanned"`
	HistoryEntries      int `json:"history_entries"`
	TmuxSessionsChecked int `json:"tmux_sessions_checked"`

	// Status
	Healthy bool     `json:"healthy"`
	Errors  []string `json:"errors,omitempty"`
}

// AuditOptions configures the audit behavior
type AuditOptions struct {
	SessionsDir     string
	WorkspaceFilter string
	SeverityFilter  Severity
	Adapter         *dolt.Adapter
}

// RunAudit performs a comprehensive audit of AGM sessions
func RunAudit(opts AuditOptions) (*AuditReport, error) {
	report := &AuditReport{
		StartedAt:         time.Now(),
		WorkspaceFilter:   opts.WorkspaceFilter,
		SeverityFilter:    opts.SeverityFilter,
		Issues:            []*AuditIssue{},
		IssuesBySeverity:  make(map[Severity]int),
		IssuesByType:      make(map[IssueType]int),
		IssuesByWorkspace: make(map[string]int),
		Errors:            []string{},
	}

	// Check 1: Orphaned conversations
	orphanIssues, historyCount, err := checkOrphanedConversations(opts.SessionsDir, opts.WorkspaceFilter, opts.Adapter)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("orphan check failed: %v", err))
	} else {
		report.Issues = append(report.Issues, orphanIssues...)
		report.HistoryEntries = historyCount
	}

	// Check 2: Corrupted manifests
	corruptedIssues, manifestCount, err := checkCorruptedManifests(opts.SessionsDir, opts.WorkspaceFilter)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("manifest check failed: %v", err))
	} else {
		report.Issues = append(report.Issues, corruptedIssues...)
		report.ManifestsScanned = manifestCount
	}

	// Check 3: Missing tmux sessions
	missingTmuxIssues, tmuxCount, err := checkMissingTmuxSessions(opts.SessionsDir, opts.WorkspaceFilter, opts.Adapter)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("tmux check failed: %v", err))
	} else {
		report.Issues = append(report.Issues, missingTmuxIssues...)
		report.TmuxSessionsChecked = tmuxCount
	}

	// Check 4: Stale sessions
	staleIssues, err := checkStaleSessions(opts.SessionsDir, opts.WorkspaceFilter, opts.Adapter)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("stale check failed: %v", err))
	} else {
		report.Issues = append(report.Issues, staleIssues...)
	}

	// Check 5: Duplicate UUIDs
	duplicateIssues, err := checkDuplicateUUIDs(opts.SessionsDir, opts.WorkspaceFilter, opts.Adapter)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("duplicate check failed: %v", err))
	} else {
		report.Issues = append(report.Issues, duplicateIssues...)
	}

	// Apply severity filter
	if opts.SeverityFilter != "" {
		report.Issues = filterBySeverity(report.Issues, opts.SeverityFilter)
	}

	// Calculate statistics
	report.TotalIssues = len(report.Issues)
	for _, issue := range report.Issues {
		report.IssuesBySeverity[issue.Severity]++
		report.IssuesByType[issue.Type]++
		if issue.Workspace != "" {
			report.IssuesByWorkspace[issue.Workspace]++
		}
	}

	// Determine health status
	report.Healthy = report.TotalIssues == 0 && len(report.Errors) == 0

	// Finalize timing
	report.CompletedAt = time.Now()
	report.Duration = report.CompletedAt.Sub(report.StartedAt).Round(time.Millisecond).String()

	return report, nil
}

// checkOrphanedConversations detects conversations without manifests
func checkOrphanedConversations(sessionsDir, workspaceFilter string, adapter *dolt.Adapter) ([]*AuditIssue, int, error) {
	report, err := orphan.DetectOrphansWithAdapter(sessionsDir, workspaceFilter, adapter)
	if err != nil {
		return nil, 0, err
	}

	var issues []*AuditIssue
	for _, o := range report.Orphans {
		issues = append(issues, &AuditIssue{
			Type:           IssueOrphanedConversation,
			Severity:       SeverityWarning,
			UUID:           o.UUID,
			Workspace:      o.Workspace,
			Path:           o.ProjectPath,
			Message:        fmt.Sprintf("Orphaned conversation found: %s", shortID(o.UUID)),
			Details:        fmt.Sprintf("Project: %s, Last modified: %s", o.ProjectPath, o.LastModified.Format("2006-01-02 15:04")),
			Recommendation: "Run 'agm session import " + o.UUID + "' to restore tracking",
			DetectedAt:     time.Now(),
		})
	}

	return issues, report.HistoryEntries, nil
}

// checkCorruptedManifests detects manifest files with YAML parse errors or missing required fields
func checkCorruptedManifests(sessionsDir, workspaceFilter string) ([]*AuditIssue, int, error) {
	var issues []*AuditIssue
	manifestCount := 0

	// Scan for all manifest.yaml files
	pattern := filepath.Join(sessionsDir, "*/manifest.yaml")
	manifestPaths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find manifests: %w", err)
	}

	// Also scan archive directory
	archiveDir := filepath.Join(sessionsDir, ".archive-old-format")
	if _, err := os.Stat(archiveDir); err == nil {
		archivePattern := filepath.Join(archiveDir, "*/manifest.yaml")
		archivePaths, err := filepath.Glob(archivePattern)
		if err == nil {
			manifestPaths = append(manifestPaths, archivePaths...)
		}
	}

	for _, manifestPath := range manifestPaths {
		manifestCount++

		// Try to read the manifest
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			issues = append(issues, &AuditIssue{
				Type:           IssueCorruptedManifest,
				Severity:       SeverityCritical,
				Path:           manifestPath,
				Message:        fmt.Sprintf("Cannot read manifest: %s", filepath.Base(filepath.Dir(manifestPath))),
				Details:        err.Error(),
				Recommendation: "Check file permissions or restore from backup",
				DetectedAt:     time.Now(),
			})
			continue
		}

		// Try to parse YAML
		var m manifest.Manifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			issues = append(issues, &AuditIssue{
				Type:           IssueCorruptedManifest,
				Severity:       SeverityCritical,
				SessionID:      filepath.Base(filepath.Dir(manifestPath)),
				Path:           manifestPath,
				Message:        fmt.Sprintf("YAML parse error: %s", filepath.Base(filepath.Dir(manifestPath))),
				Details:        err.Error(),
				Recommendation: "Fix YAML syntax errors or restore from backup",
				DetectedAt:     time.Now(),
			})
			continue
		}

		// Validate required fields
		if err := m.Validate(); err != nil {
			// Apply workspace filter
			if workspaceFilter != "" && m.Workspace != workspaceFilter {
				continue
			}

			issues = append(issues, &AuditIssue{
				Type:           IssueCorruptedManifest,
				Severity:       SeverityCritical,
				SessionID:      m.SessionID,
				Workspace:      m.Workspace,
				Path:           manifestPath,
				Message:        fmt.Sprintf("Invalid manifest: %s", m.SessionID),
				Details:        err.Error(),
				Recommendation: "Fix missing required fields or regenerate manifest",
				DetectedAt:     time.Now(),
			})
		}
	}

	return issues, manifestCount, nil
}

// checkMissingTmuxSessions detects manifests whose tmux sessions don't exist
func checkMissingTmuxSessions(sessionsDir, workspaceFilter string, adapter *dolt.Adapter) ([]*AuditIssue, int, error) {
	var issues []*AuditIssue
	tmuxCount := 0

	// Load all sessions from Dolt
	if adapter == nil {
		return nil, 0, fmt.Errorf("dolt adapter required")
	}
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	for _, m := range manifests {
		// Skip archived sessions
		if m.Lifecycle == "archived" {
			continue
		}

		// Apply workspace filter
		if workspaceFilter != "" && m.Workspace != workspaceFilter {
			continue
		}

		tmuxCount++

		// Check if tmux session exists
		sessionName := m.Tmux.SessionName
		if sessionName == "" {
			continue // Skip if no tmux session name defined
		}

		exists, err := tmux.HasSession(sessionName)
		if err != nil {
			// Don't fail the entire audit on tmux check errors
			continue
		}

		if !exists {
			issues = append(issues, &AuditIssue{
				Type:           IssueMissingTmuxSession,
				Severity:       SeverityWarning,
				SessionID:      m.SessionID,
				Workspace:      m.Workspace,
				Path:           filepath.Join(sessionsDir, m.SessionID),
				Message:        fmt.Sprintf("Tmux session not found: %s", sessionName),
				Details:        fmt.Sprintf("Manifest exists but tmux session '%s' doesn't exist", sessionName),
				Recommendation: "Run 'agm resume " + m.SessionID + "' to recreate or 'agm archive " + m.SessionID + "' to archive",
				DetectedAt:     time.Now(),
			})
		}
	}

	return issues, tmuxCount, nil
}

// checkStaleSessions detects sessions inactive for more than 30 days
func checkStaleSessions(sessionsDir, workspaceFilter string, adapter *dolt.Adapter) ([]*AuditIssue, error) {
	var issues []*AuditIssue

	// Load all sessions from Dolt
	if adapter == nil {
		return nil, fmt.Errorf("dolt adapter required")
	}
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	staleThreshold := 30 * 24 * time.Hour
	now := time.Now()

	for _, m := range manifests {
		// Skip archived sessions
		if m.Lifecycle == "archived" {
			continue
		}

		// Apply workspace filter
		if workspaceFilter != "" && m.Workspace != workspaceFilter {
			continue
		}

		// Check if stale (based on updated_at)
		inactiveDuration := now.Sub(m.UpdatedAt)
		if inactiveDuration > staleThreshold {
			daysInactive := int(inactiveDuration.Hours() / 24)
			issues = append(issues, &AuditIssue{
				Type:           IssueStaleSession,
				Severity:       SeverityInfo,
				SessionID:      m.SessionID,
				Workspace:      m.Workspace,
				Path:           filepath.Join(sessionsDir, m.SessionID),
				Message:        fmt.Sprintf("Stale session: %s (%d days inactive)", m.Name, daysInactive),
				Details:        fmt.Sprintf("Last updated: %s", m.UpdatedAt.Format("2006-01-02")),
				Recommendation: "Run 'agm archive " + m.SessionID + "' to archive or 'agm kill " + m.SessionID + "' to remove",
				DetectedAt:     time.Now(),
			})
		}
	}

	return issues, nil
}

// checkDuplicateUUIDs detects multiple manifests claiming the same Claude UUID
func checkDuplicateUUIDs(sessionsDir, workspaceFilter string, adapter *dolt.Adapter) ([]*AuditIssue, error) {
	var issues []*AuditIssue

	// Load all sessions from Dolt
	if adapter == nil {
		return nil, fmt.Errorf("dolt adapter required")
	}
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	// Build UUID -> sessions map
	uuidMap := make(map[string][]*manifest.Manifest)
	for _, m := range manifests {
		// Apply workspace filter
		if workspaceFilter != "" && m.Workspace != workspaceFilter {
			continue
		}

		// Skip sessions without UUIDs
		if m.Claude.UUID == "" {
			continue
		}

		uuidMap[m.Claude.UUID] = append(uuidMap[m.Claude.UUID], m)
	}

	// Find duplicates
	for uuid, sessions := range uuidMap {
		if len(sessions) > 1 {
			// Create an issue for each duplicate
			sessionIDs := make([]string, len(sessions))
			for i, s := range sessions {
				sessionIDs[i] = s.SessionID
			}

			for _, s := range sessions {
				issues = append(issues, &AuditIssue{
					Type:           IssueDuplicateUUID,
					Severity:       SeverityCritical,
					SessionID:      s.SessionID,
					UUID:           uuid,
					Workspace:      s.Workspace,
					Path:           filepath.Join(sessionsDir, s.SessionID),
					Message:        fmt.Sprintf("Duplicate UUID: %s", shortID(uuid)),
					Details:        fmt.Sprintf("UUID %s is claimed by: %v", shortID(uuid), sessionIDs),
					Recommendation: "Keep the correct session and run 'agm admin fix-uuid' on duplicates",
					DetectedAt:     time.Now(),
				})
			}
		}
	}

	return issues, nil
}

// shortID returns the first 8 characters of a UUID string for display.
// If the string is shorter than 8 characters it is returned as-is to avoid
// an index-out-of-bounds panic on corrupt or empty manifest UUIDs.
func shortID(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8]
}

// filterBySeverity filters issues by minimum severity level
func filterBySeverity(issues []*AuditIssue, minSeverity Severity) []*AuditIssue {
	// Severity ordering: critical > warning > info
	severityOrder := map[Severity]int{
		SeverityCritical: 3,
		SeverityWarning:  2,
		SeverityInfo:     1,
	}

	minLevel := severityOrder[minSeverity]
	var filtered []*AuditIssue
	for _, issue := range issues {
		if severityOrder[issue.Severity] >= minLevel {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

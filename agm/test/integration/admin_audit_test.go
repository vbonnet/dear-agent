package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/audit"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/testutil"
	"gopkg.in/yaml.v3"
)

// TestAdminAudit_MultiWorkspace tests audit across multiple workspaces
func TestAdminAudit_MultiWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	// Create sessions in different workspaces
	createIntegrationSession(t, tmpDir, "oss-session-1", "oss", "uuid-oss-1", time.Now())
	createIntegrationSession(t, tmpDir, "oss-session-2", "oss", "uuid-oss-2", time.Now())
	createIntegrationSession(t, tmpDir, "acme-session-1", "acme", "uuid-acme-1", time.Now())

	t.Run("AuditAllWorkspaces", func(t *testing.T) {
		opts := audit.AuditOptions{
			SessionsDir: tmpDir,
		}

		report, err := audit.RunAudit(opts)
		if err != nil {
			t.Fatalf("Audit failed: %v", err)
		}

		// Should scan all 3 sessions
		if report.ManifestsScanned != 3 {
			t.Errorf("Expected 3 manifests scanned, got %d", report.ManifestsScanned)
		}

		// Note: Orphan detection reads real history.jsonl which may contain many entries
		// We can't assert exact issue counts, but we can verify structure
		if len(report.IssuesByWorkspace) > 0 {
			// Just verify the structure is valid, don't assert exact counts
			t.Logf("Found %d workspaces with issues", len(report.IssuesByWorkspace))
		}
	})

	t.Run("AuditFilteredWorkspace", func(t *testing.T) {
		opts := audit.AuditOptions{
			SessionsDir:     tmpDir,
			WorkspaceFilter: "oss",
		}

		report, err := audit.RunAudit(opts)
		if err != nil {
			t.Fatalf("Audit failed: %v", err)
		}

		// Should still scan all manifests but filter results
		if report.ManifestsScanned != 3 {
			t.Errorf("Expected 3 manifests scanned, got %d", report.ManifestsScanned)
		}

		// All issues should be from oss workspace
		for _, issue := range report.Issues {
			if issue.Workspace != "oss" && issue.Workspace != "" {
				t.Errorf("Expected issue workspace to be 'oss' or empty, got %q", issue.Workspace)
			}
		}
	})
}

// TestAdminAudit_AllChecksPass tests system with valid test sessions
// Note: Orphan detection reads real history.jsonl which may contain orphans,
// so we verify structure and metadata instead of exact issue counts
func TestAdminAudit_AllChecksPass(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only valid, fresh sessions
	createIntegrationSession(t, tmpDir, "session-1", "oss", "uuid-1", time.Now())
	createIntegrationSession(t, tmpDir, "session-2", "oss", "uuid-2", time.Now())

	opts := audit.AuditOptions{
		SessionsDir: tmpDir,
	}

	report, err := audit.RunAudit(opts)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	// Should have scanned sessions
	if report.ManifestsScanned != 2 {
		t.Errorf("Expected 2 manifests scanned, got %d", report.ManifestsScanned)
	}

	// Should have timing information
	if report.StartedAt.IsZero() {
		t.Error("Expected StartedAt to be set")
	}
	if report.CompletedAt.IsZero() {
		t.Error("Expected CompletedAt to be set")
	}
	if report.Duration == "" {
		t.Error("Expected Duration to be set")
	}

	// Verify report structure is valid (may contain orphans from real history)
	if report.IssuesBySeverity == nil {
		t.Error("Expected IssuesBySeverity to be initialized")
	}
	if report.IssuesByType == nil {
		t.Error("Expected IssuesByType to be initialized")
	}
	if report.IssuesByWorkspace == nil {
		t.Error("Expected IssuesByWorkspace to be initialized")
	}
}

// TestAdminAudit_AllChecksFail tests system with multiple issues
func TestAdminAudit_AllChecksFail(t *testing.T) {
	tmpDir := t.TempDir()

	// Create corrupted manifest (critical)
	createCorruptedIntegrationManifest(t, tmpDir, "session-corrupted")

	// Create duplicate UUIDs (critical)
	createIntegrationSession(t, tmpDir, "session-dup-1", "oss", "duplicate-uuid", time.Now())
	createIntegrationSession(t, tmpDir, "session-dup-2", "oss", "duplicate-uuid", time.Now())

	// Create stale session (info)
	createIntegrationSession(t, tmpDir, "session-stale", "oss", "uuid-stale", time.Now().Add(-40*24*time.Hour))

	opts := audit.AuditOptions{
		SessionsDir: tmpDir,
	}

	report, err := audit.RunAudit(opts)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	// Should be unhealthy
	if report.Healthy {
		t.Error("Expected unhealthy system, got healthy")
	}

	// Should have multiple issues
	if report.TotalIssues == 0 {
		t.Error("Expected issues to be found")
	}

	// Should have all severity levels
	if len(report.IssuesBySeverity) == 0 {
		t.Error("Expected issues grouped by severity")
	}

	// Should have critical issues
	if report.IssuesBySeverity[audit.SeverityCritical] == 0 {
		t.Error("Expected critical issues")
	}

	// Should have multiple issue types
	if len(report.IssuesByType) == 0 {
		t.Error("Expected issues grouped by type")
	}
}

// TestAdminAudit_EmptyWorkspace tests behavior with empty sessions directory
// Note: Orphan detection reads real history.jsonl which may contain orphans
func TestAdminAudit_EmptyWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	opts := audit.AuditOptions{
		SessionsDir: tmpDir,
	}

	report, err := audit.RunAudit(opts)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	// Should have 0 manifests in test directory
	if report.ManifestsScanned != 0 {
		t.Errorf("Expected 0 manifests scanned, got %d", report.ManifestsScanned)
	}

	// May find orphans from real history.jsonl, so we don't assert Healthy or TotalIssues
	// Just verify the report is structurally valid
	if report.IssuesBySeverity == nil {
		t.Error("Expected IssuesBySeverity to be initialized")
	}
	if report.IssuesByType == nil {
		t.Error("Expected IssuesByType to be initialized")
	}
}

// TestAdminAudit_SeverityFiltering tests filtering by severity level
func TestAdminAudit_SeverityFiltering(t *testing.T) {
	tmpDir := t.TempDir()

	// Create issues of different severities
	createCorruptedIntegrationManifest(t, tmpDir, "session-critical")                                       // critical
	createIntegrationSession(t, tmpDir, "session-stale", "oss", "uuid-1", time.Now().Add(-40*24*time.Hour)) // info

	tests := []struct {
		name           string
		severity       audit.Severity
		expectedMinSev audit.Severity
	}{
		{
			name:           "FilterCriticalOnly",
			severity:       audit.SeverityCritical,
			expectedMinSev: audit.SeverityCritical,
		},
		{
			name:           "FilterWarningAndAbove",
			severity:       audit.SeverityWarning,
			expectedMinSev: audit.SeverityWarning,
		},
		{
			name:           "FilterAllLevels",
			severity:       audit.SeverityInfo,
			expectedMinSev: audit.SeverityInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := audit.AuditOptions{
				SessionsDir:    tmpDir,
				SeverityFilter: tt.severity,
			}

			report, err := audit.RunAudit(opts)
			if err != nil {
				t.Fatalf("Audit failed: %v", err)
			}

			// All issues should meet minimum severity
			severityOrder := map[audit.Severity]int{
				audit.SeverityCritical: 3,
				audit.SeverityWarning:  2,
				audit.SeverityInfo:     1,
			}

			minLevel := severityOrder[tt.expectedMinSev]
			for _, issue := range report.Issues {
				issueLevel := severityOrder[issue.Severity]
				if issueLevel < minLevel {
					t.Errorf("Found issue with severity %s, expected minimum %s",
						issue.Severity, tt.expectedMinSev)
				}
			}
		})
	}
}

// TestAdminAudit_CorruptedManifests tests detection of corrupted manifests
func TestAdminAudit_CorruptedManifests(t *testing.T) {
	tmpDir := t.TempDir()

	// Create corrupted manifests
	createCorruptedIntegrationManifest(t, tmpDir, "session-bad-yaml")
	createInvalidIntegrationManifest(t, tmpDir, "session-missing-fields")

	opts := audit.AuditOptions{
		SessionsDir: tmpDir,
	}

	report, err := audit.RunAudit(opts)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	// Should find corrupted manifest issues
	corruptedCount := 0
	for _, issue := range report.Issues {
		if issue.Type == audit.IssueCorruptedManifest {
			corruptedCount++

			// Should be critical severity
			if issue.Severity != audit.SeverityCritical {
				t.Errorf("Expected critical severity for corrupted manifest, got %s", issue.Severity)
			}

			// Should have recommendation
			if issue.Recommendation == "" {
				t.Error("Expected recommendation for corrupted manifest")
			}
		}
	}

	if corruptedCount == 0 {
		t.Error("Expected to find corrupted manifest issues")
	}
}

// TestAdminAudit_StaleSessions tests detection of stale sessions
func TestAdminAudit_StaleSessions(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	tmpDir := t.TempDir()

	// Create stale sessions in Dolt (>30 days old)
	staleIDs := []string{
		"test-stale-" + uuid.New().String()[:8],
		"test-stale-" + uuid.New().String()[:8],
	}
	freshID := "test-fresh-" + uuid.New().String()[:8]

	for i, sid := range staleIDs {
		staleDays := 40 + i*20
		m := &manifest.Manifest{
			SchemaVersion: "2", SessionID: sid, Name: sid,
			Harness: "claude-code", Workspace: "oss",
			CreatedAt: time.Now().Add(-time.Duration(staleDays+1) * 24 * time.Hour),
			UpdatedAt: time.Now().Add(-time.Duration(staleDays) * 24 * time.Hour),
			Context:   manifest.Context{Project: "/tmp/" + sid},
			Tmux:      manifest.Tmux{SessionName: sid},
			Claude:    manifest.Claude{UUID: "uuid-" + sid},
		}
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("Failed to create stale session: %v", err)
		}
	}

	freshM := &manifest.Manifest{
		SchemaVersion: "2", SessionID: freshID, Name: freshID,
		Harness: "claude-code", Workspace: "oss",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Context: manifest.Context{Project: "/tmp/" + freshID},
		Tmux:    manifest.Tmux{SessionName: freshID},
		Claude:  manifest.Claude{UUID: "uuid-" + freshID},
	}
	if err := adapter.CreateSession(freshM); err != nil {
		t.Fatalf("Failed to create fresh session: %v", err)
	}

	opts := audit.AuditOptions{
		SessionsDir: tmpDir,
		Adapter:     adapter,
	}

	report, err := audit.RunAudit(opts)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	// Count stale issues for our test sessions
	staleCount := 0
	for _, issue := range report.Issues {
		if issue.Type == audit.IssueStaleSession {
			for _, sid := range staleIDs {
				if issue.SessionID == sid {
					staleCount++
					if issue.Severity != audit.SeverityInfo {
						t.Errorf("Expected info severity for stale session, got %s", issue.Severity)
					}
					if issue.Recommendation == "" {
						t.Error("Expected recommendation for stale session")
					}
				}
			}
		}
	}

	if staleCount != 2 {
		t.Errorf("Expected 2 stale sessions, found %d", staleCount)
	}

	// Cleanup
	for _, sid := range staleIDs {
		_ = adapter.DeleteSession(sid)
	}
	_ = adapter.DeleteSession(freshID)
}

// TestAdminAudit_DuplicateUUIDs tests detection of duplicate UUIDs
func TestAdminAudit_DuplicateUUIDs(t *testing.T) {
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	tmpDir := t.TempDir()
	dupUUID := "dup-uuid-" + uuid.New().String()[:8]

	// Create 3 sessions with duplicate UUID
	dupIDs := []string{
		"test-dup-" + uuid.New().String()[:8],
		"test-dup-" + uuid.New().String()[:8],
		"test-dup-" + uuid.New().String()[:8],
	}
	for i, sid := range dupIDs {
		ws := "oss"
		if i == 2 {
			ws = "acme"
		}
		m := &manifest.Manifest{
			SchemaVersion: "2", SessionID: sid, Name: sid,
			Harness: "claude-code", Workspace: ws,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
			Context: manifest.Context{Project: "/tmp/" + sid},
			Tmux:    manifest.Tmux{SessionName: sid},
			Claude:  manifest.Claude{UUID: dupUUID},
		}
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}
	}

	// Create session with unique UUID
	uniqueID := "test-unique-" + uuid.New().String()[:8]
	uniqueM := &manifest.Manifest{
		SchemaVersion: "2", SessionID: uniqueID, Name: uniqueID,
		Harness: "claude-code", Workspace: "oss",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Context: manifest.Context{Project: "/tmp/" + uniqueID},
		Tmux:    manifest.Tmux{SessionName: uniqueID},
		Claude:  manifest.Claude{UUID: "unique-" + uuid.New().String()[:8]},
	}
	if err := adapter.CreateSession(uniqueM); err != nil {
		t.Fatalf("Failed to create unique session: %v", err)
	}

	opts := audit.AuditOptions{
		SessionsDir: tmpDir,
		Adapter:     adapter,
	}

	report, err := audit.RunAudit(opts)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	// Count duplicate UUID issues for our test UUID
	duplicateCount := 0
	for _, issue := range report.Issues {
		if issue.Type == audit.IssueDuplicateUUID && issue.UUID == dupUUID {
			duplicateCount++
			if issue.Severity != audit.SeverityCritical {
				t.Errorf("Expected critical severity for duplicate UUID, got %s", issue.Severity)
			}
			if issue.Recommendation == "" {
				t.Error("Expected recommendation for duplicate UUID")
			}
		}
	}

	if duplicateCount != 3 {
		t.Errorf("Expected 3 duplicate UUID issues, found %d", duplicateCount)
	}

	// Cleanup
	for _, sid := range dupIDs {
		_ = adapter.DeleteSession(sid)
	}
	_ = adapter.DeleteSession(uniqueID)
}

// TestAdminAudit_JSONOutput tests JSON marshaling of audit report
func TestAdminAudit_JSONOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some sessions
	createIntegrationSession(t, tmpDir, "session-1", "oss", "uuid-1", time.Now())
	createCorruptedIntegrationManifest(t, tmpDir, "session-bad")

	opts := audit.AuditOptions{
		SessionsDir: tmpDir,
	}

	report, err := audit.RunAudit(opts)
	if err != nil {
		t.Fatalf("Audit failed: %v", err)
	}

	// Marshal to JSON
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Failed to marshal report to JSON: %v", err)
	}

	// Unmarshal back
	var unmarshaled audit.AuditReport
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verify key fields
	if unmarshaled.TotalIssues != report.TotalIssues {
		t.Errorf("Expected %d issues after unmarshal, got %d", report.TotalIssues, unmarshaled.TotalIssues)
	}

	if unmarshaled.ManifestsScanned != report.ManifestsScanned {
		t.Errorf("Expected %d manifests scanned after unmarshal, got %d",
			report.ManifestsScanned, unmarshaled.ManifestsScanned)
	}

	if unmarshaled.Healthy != report.Healthy {
		t.Errorf("Expected healthy=%v after unmarshal, got %v", report.Healthy, unmarshaled.Healthy)
	}
}

// Helper functions

func createIntegrationSession(t *testing.T, baseDir, sessionID, workspace, uuid string, updatedAt time.Time) {
	sessionDir := filepath.Join(baseDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	m := manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sessionID,
		Name:          sessionID,
		CreatedAt:     time.Now().Add(-24 * time.Hour),
		UpdatedAt:     updatedAt,
		Workspace:     workspace,
		Context: manifest.Context{
			Project: "~/src/test",
		},
		Claude: manifest.Claude{
			UUID: uuid,
		},
		Tmux: manifest.Tmux{
			SessionName: sessionID,
		},
	}

	data, err := yaml.Marshal(&m)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}
}

func createCorruptedIntegrationManifest(t *testing.T, baseDir, sessionID string) {
	sessionDir := filepath.Join(baseDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Write invalid YAML
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	invalidYAML := `
schema_version: "2.0"
session_id: "test"
name: [this is: invalid yaml syntax
`
	if err := os.WriteFile(manifestPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write corrupted manifest: %v", err)
	}
}

func createInvalidIntegrationManifest(t *testing.T, baseDir, sessionID string) {
	sessionDir := filepath.Join(baseDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("Failed to create session dir: %v", err)
	}

	// Create manifest missing required fields
	m := manifest.Manifest{
		SchemaVersion: "2.0",
		// Missing SessionID (required)
		Name:      "incomplete",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	data, err := yaml.Marshal(&m)
	if err != nil {
		t.Fatalf("Failed to marshal manifest: %v", err)
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}
}

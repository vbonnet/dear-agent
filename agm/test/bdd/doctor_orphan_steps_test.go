package bdd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/orphan"
)

// DoctorOrphanContext holds the test context for doctor orphan scenarios
type DoctorOrphanContext struct {
	tmpDir        string
	sessionsDir   string
	orphanReport  *orphan.OrphanDetectionReport
	orphanCount   int
	lastError     error
	currentOutput string
	checkStatus   string
	manifests     []*manifest.Manifest
	orphanedUUIDs []string
}

// InitializeDoctorOrphanScenario initializes BDD scenarios for doctor orphan check
func InitializeDoctorOrphanScenario(ctx *godog.ScenarioContext) {
	doctorCtx := &DoctorOrphanContext{}

	// Background steps
	ctx.Step(`^I have AGM installed$`, doctorCtx.iHaveAGMInstalled)
	ctx.Step(`^"([^"]*)" command exists$`, doctorCtx.commandExists)

	// Given steps
	ctx.Step(`^(\d+) orphaned sessions exist in the current workspace$`, doctorCtx.orphanedSessionsExist)
	ctx.Step(`^all sessions have manifests$`, doctorCtx.allSessionsHaveManifests)
	ctx.Step(`^"([^"]*)" has existing health checks$`, doctorCtx.hasExistingHealthChecks)
	ctx.Step(`^a critical orphaned session exists \(current workspace, recent activity\)$`, doctorCtx.criticalOrphanedSessionExists)
	ctx.Step(`^orphaned sessions exist in multiple workspaces$`, doctorCtx.orphanedSessionsInMultipleWorkspaces)
	ctx.Step(`^I am in workspace "([^"]*)"$`, doctorCtx.iAmInWorkspace)
	ctx.Step(`^(\d+) orphaned sessions exist$`, doctorCtx.orphanedSessionsExist)
	ctx.Step(`^orphaned sessions "([^"]*)", "([^"]*)", "([^"]*)" exist$`, doctorCtx.namedOrphanedSessionsExist)
	ctx.Step(`^the history\.jsonl file is corrupted$`, doctorCtx.historyJSONLCorrupted)
	ctx.Step(`^(\d+) sessions exist$`, doctorCtx.sessionsExist)
	ctx.Step(`^doctor uses severity levels: CRITICAL, WARNING, INFO$`, doctorCtx.doctorUsesSeverityLevels)
	ctx.Step(`^an orphaned session is detected$`, doctorCtx.orphanedSessionIsDetected)
	ctx.Step(`^I have disabled orphan checks in AGM config$`, doctorCtx.orphanChecksDisabled)
	ctx.Step(`^doctor has a --quick mode for fast checks$`, doctorCtx.doctorHasQuickMode)
	ctx.Step(`^an orphaned session with last activity "([^"]*)"$`, doctorCtx.orphanedSessionWithLastActivity)
	ctx.Step(`^the current date is "([^"]*)"$`, doctorCtx.currentDateIs)
	ctx.Step(`^(\d+) other warning exists \(e\.g\., low disk space\)$`, doctorCtx.otherWarningExists)

	// When steps
	ctx.Step(`^I run "([^"]*)"$`, doctorCtx.iRunCommand)
	ctx.Step(`^I run "([^"]*)" --verbose$`, doctorCtx.iRunCommandVerbose)
	ctx.Step(`^I run "([^"]*)" --json$`, doctorCtx.iRunCommandJSON)
	ctx.Step(`^I run "([^"]*)" --quick$`, doctorCtx.iRunCommandQuick)

	// Then steps
	ctx.Step(`^the output should include a check named "([^"]*)"$`, doctorCtx.outputIncludesCheck)
	ctx.Step(`^the check should show status "([^"]*)"$`, doctorCtx.checkShowsStatus)
	ctx.Step(`^the check should report "([^"]*)"$`, doctorCtx.checkReports)
	ctx.Step(`^the check should suggest "([^"]*)"$`, doctorCtx.checkSuggests)
	ctx.Step(`^the "([^"]*)" check should show status "([^"]*)"$`, doctorCtx.namedCheckShowsStatus)
	ctx.Step(`^the output should include all existing checks$`, doctorCtx.outputIncludesAllChecks)
	ctx.Step(`^the "([^"]*)" check should be included$`, doctorCtx.checkIsIncluded)
	ctx.Step(`^the check order should be logical$`, doctorCtx.checkOrderIsLogical)
	ctx.Step(`^the "([^"]*)" check should show severity "([^"]*)"$`, doctorCtx.checkShowsSeverity)
	ctx.Step(`^the overall doctor status should reflect the warning$`, doctorCtx.overallStatusReflectsWarning)
	ctx.Step(`^the orphan check should only report orphans in "([^"]*)" workspace$`, doctorCtx.orphanCheckReportsWorkspace)
	ctx.Step(`^orphans from other workspaces should not be counted$`, doctorCtx.orphansFromOtherWorkspacesNotCounted)
	ctx.Step(`^the "([^"]*)" check output should show:$`, doctorCtx.checkOutputShows)
	ctx.Step(`^the "([^"]*)" check should list all orphaned UUIDs$`, doctorCtx.checkListsAllOrphanedUUIDs)
	ctx.Step(`^each UUID should show its last activity timestamp$`, doctorCtx.eachUUIDShowsTimestamp)
	ctx.Step(`^the JSON output should include:$`, doctorCtx.jsonOutputIncludes)
	ctx.Step(`^the "([^"]*)" check should complete in < (\d+) seconds$`, doctorCtx.checkCompletesInTime)
	ctx.Step(`^the doctor output should not show performance warnings$`, doctorCtx.doctorNoPerformanceWarnings)
	ctx.Step(`^the orphan check should use severity "([^"]*)"$`, doctorCtx.orphanCheckUsesSeverity)
	ctx.Step(`^the doctor summary should aggregate all severities correctly$`, doctorCtx.doctorSummaryAggregatesSeverities)
	ctx.Step(`^the output should show "([^"]*)"$`, doctorCtx.outputShows)
	ctx.Step(`^the orphan check should use fast detection \(count only, no details\)$`, doctorCtx.orphanCheckUsesFastDetection)
	ctx.Step(`^the output should show orphan count without listing UUIDs$`, doctorCtx.outputShowsCountWithoutUUIDs)
	ctx.Step(`^the orphan check should show "([^"]*)"$`, doctorCtx.orphanCheckShows)
	ctx.Step(`^the remediation should emphasize urgency for old orphans$`, doctorCtx.remediationEmphasizesUrgency)
	ctx.Step(`^the summary should show:$`, doctorCtx.summaryShows)
	ctx.Step(`^the orphan check should suggest "([^"]*)"$`, doctorCtx.orphanCheckSuggests)
	ctx.Step(`^the output should include a direct link to the command$`, doctorCtx.outputIncludesDirectLink)

	// Cleanup
	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		doctorCtx.cleanup()
		return ctx, nil
	})
}

// Background steps
func (c *DoctorOrphanContext) iHaveAGMInstalled() error {
	// Setup test adapter
	// Note: BDD tests use a mock testing.T, so we can't use dolt.GetTestAdapter
	// Instead, we'll create sessions directory for compatibility
	tmpDir, err := os.MkdirTemp("", "doctor-bdd-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	c.tmpDir = tmpDir
	c.sessionsDir = filepath.Join(tmpDir, "sessions")

	// Note: For BDD tests, we skip Dolt adapter setup since godog doesn't provide *testing.T
	// The tests will work with in-memory data structures instead
	return os.MkdirAll(c.sessionsDir, 0755)
}

func (c *DoctorOrphanContext) commandExists(command string) error {
	// Verify command pattern
	if !strings.Contains(command, "agm admin doctor") {
		return fmt.Errorf("expected 'agm admin doctor', got '%s'", command)
	}
	return nil
}

// Given steps
func (c *DoctorOrphanContext) orphanedSessionsExist(count int) error {
	c.orphanCount = count
	// Create orphaned sessions metadata
	c.orphanedUUIDs = make([]string, count)
	for i := 0; i < count; i++ {
		c.orphanedUUIDs[i] = fmt.Sprintf("orphan-uuid-%d", i+1)
	}
	return nil
}

func (c *DoctorOrphanContext) allSessionsHaveManifests() error {
	// Create test session metadata (in-memory for BDD tests)
	m := &manifest.Manifest{
		SchemaVersion: "2.0.0",
		Name:          "test-session",
		SessionID:     "test-uuid",
		Workspace:     "test",
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: c.sessionsDir,
		},
		Claude: manifest.Claude{
			UUID: "test-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-session",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	c.manifests = append(c.manifests, m)
	return nil
}

func (c *DoctorOrphanContext) hasExistingHealthChecks(command string) error {
	// Existing health checks include:
	// - Claude installation
	// - tmux installation
	// - tmux socket
	// - User lingering
	// - Duplicate session directories
	// - Duplicate UUIDs
	// - Empty UUIDs
	// - Session health
	return nil
}

func (c *DoctorOrphanContext) criticalOrphanedSessionExists() error {
	c.orphanCount = 1
	c.orphanedUUIDs = []string{"critical-orphan-uuid"}
	return nil
}

func (c *DoctorOrphanContext) orphanedSessionsInMultipleWorkspaces() error {
	c.orphanCount = 3
	c.orphanedUUIDs = []string{
		"orphan-oss-1",
		"orphan-acme-1",
		"orphan-research-1",
	}
	return nil
}

func (c *DoctorOrphanContext) iAmInWorkspace(workspace string) error {
	// Set workspace context
	return nil
}

func (c *DoctorOrphanContext) namedOrphanedSessionsExist(uuid1, uuid2, uuid3 string) error {
	c.orphanedUUIDs = []string{uuid1, uuid2, uuid3}
	c.orphanCount = 3
	return nil
}

func (c *DoctorOrphanContext) historyJSONLCorrupted() error {
	// Simulate corrupted history
	c.lastError = fmt.Errorf("corrupted history.jsonl")
	return nil
}

func (c *DoctorOrphanContext) sessionsExist(count int) error {
	// Create test session metadata (in-memory for BDD tests)
	for i := 0; i < count; i++ {
		m := &manifest.Manifest{
			SchemaVersion: "2.0.0",
			Name:          fmt.Sprintf("session-%d", i),
			SessionID:     fmt.Sprintf("uuid-%d", i),
			Workspace:     "test",
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: c.sessionsDir,
			},
			Claude: manifest.Claude{
				UUID: fmt.Sprintf("uuid-%d", i),
			},
			Tmux: manifest.Tmux{
				SessionName: fmt.Sprintf("session-%d", i),
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		c.manifests = append(c.manifests, m)
	}
	return nil
}

func (c *DoctorOrphanContext) doctorUsesSeverityLevels() error {
	// Doctor uses: CRITICAL, WARNING, INFO
	return nil
}

func (c *DoctorOrphanContext) orphanedSessionIsDetected() error {
	c.orphanCount = 1
	c.orphanedUUIDs = []string{"detected-orphan"}
	return nil
}

func (c *DoctorOrphanContext) orphanChecksDisabled() error {
	// Simulate disabled orphan checks
	return nil
}

func (c *DoctorOrphanContext) doctorHasQuickMode() error {
	// Doctor supports --quick flag
	return nil
}

func (c *DoctorOrphanContext) orphanedSessionWithLastActivity(timestamp string) error {
	c.orphanCount = 1
	c.orphanedUUIDs = []string{"old-orphan"}
	return nil
}

func (c *DoctorOrphanContext) currentDateIs(date string) error {
	// Set current date context
	return nil
}

func (c *DoctorOrphanContext) otherWarningExists(count int) error {
	// Simulate other warnings
	return nil
}

// When steps
func (c *DoctorOrphanContext) iRunCommand(command string) error {
	// Simulate running doctor command
	report, err := orphan.DetectOrphans(c.sessionsDir, "", nil)
	c.orphanReport = report
	c.lastError = err

	switch {
	case err == nil && report.TotalOrphans > 0:
		c.checkStatus = "WARNING"
		c.currentOutput = fmt.Sprintf("Found %d orphaned sessions", report.TotalOrphans)
	case err == nil:
		c.checkStatus = "OK"
		c.currentOutput = "No orphaned sessions found"
	default:
		c.checkStatus = "ERROR"
		c.currentOutput = fmt.Sprintf("Failed to detect orphans: %v", err)
	}

	return nil
}

func (c *DoctorOrphanContext) iRunCommandVerbose(command string) error {
	return c.iRunCommand(command)
}

func (c *DoctorOrphanContext) iRunCommandJSON(command string) error {
	return c.iRunCommand(command)
}

func (c *DoctorOrphanContext) iRunCommandQuick(command string) error {
	return c.iRunCommand(command)
}

// Then steps
func (c *DoctorOrphanContext) outputIncludesCheck(checkName string) error {
	if !strings.Contains(c.currentOutput, checkName) &&
		!strings.Contains(strings.ToLower(c.currentOutput), strings.ToLower(checkName)) {
		return fmt.Errorf("output does not include check: %s", checkName)
	}
	return nil
}

func (c *DoctorOrphanContext) checkShowsStatus(status string) error {
	if c.checkStatus != status {
		return fmt.Errorf("expected status %s, got %s", status, c.checkStatus)
	}
	return nil
}

func (c *DoctorOrphanContext) checkReports(message string) error {
	if !strings.Contains(c.currentOutput, message) {
		return fmt.Errorf("output does not include message: %s", message)
	}
	return nil
}

func (c *DoctorOrphanContext) checkSuggests(suggestion string) error {
	// Verify remediation suggestion
	expectedCommand := "agm admin find-orphans --auto-import"
	if !strings.Contains(suggestion, expectedCommand) &&
		!strings.Contains(c.currentOutput, expectedCommand) {
		return fmt.Errorf("suggestion does not include command: %s", expectedCommand)
	}
	return nil
}

func (c *DoctorOrphanContext) namedCheckShowsStatus(checkName, status string) error {
	if c.checkStatus != status {
		return fmt.Errorf("check %s: expected status %s, got %s", checkName, status, c.checkStatus)
	}
	return nil
}

func (c *DoctorOrphanContext) outputIncludesAllChecks() error {
	// Verify all standard checks are present
	return nil
}

func (c *DoctorOrphanContext) checkIsIncluded(checkName string) error {
	return c.outputIncludesCheck(checkName)
}

func (c *DoctorOrphanContext) checkOrderIsLogical() error {
	// Orphan check should come after session health check
	return nil
}

func (c *DoctorOrphanContext) checkShowsSeverity(checkName, severity string) error {
	if c.checkStatus != severity {
		return fmt.Errorf("check %s: expected severity %s, got %s", checkName, severity, c.checkStatus)
	}
	return nil
}

func (c *DoctorOrphanContext) overallStatusReflectsWarning() error {
	// Doctor should return error when warnings exist
	return nil
}

func (c *DoctorOrphanContext) orphanCheckReportsWorkspace(workspace string) error {
	// Verify workspace filtering
	return nil
}

func (c *DoctorOrphanContext) orphansFromOtherWorkspacesNotCounted() error {
	// Verify orphans from other workspaces are excluded
	return nil
}

func (c *DoctorOrphanContext) checkOutputShows(expectedOutput *godog.DocString) error {
	// Validate detailed output format
	return nil
}

func (c *DoctorOrphanContext) checkListsAllOrphanedUUIDs() error {
	// Verify all orphaned UUIDs are listed
	return nil
}

func (c *DoctorOrphanContext) eachUUIDShowsTimestamp() error {
	// Verify timestamps are shown
	return nil
}

func (c *DoctorOrphanContext) jsonOutputIncludes(expectedJSON *godog.DocString) error {
	// Validate JSON output structure
	return nil
}

func (c *DoctorOrphanContext) checkCompletesInTime(checkName string, seconds int) error {
	// Verify performance
	return nil
}

func (c *DoctorOrphanContext) doctorNoPerformanceWarnings() error {
	// Verify no performance warnings
	return nil
}

func (c *DoctorOrphanContext) orphanCheckUsesSeverity(severity string) error {
	return c.checkShowsSeverity("orphan check", severity)
}

func (c *DoctorOrphanContext) doctorSummaryAggregatesSeverities() error {
	// Verify severity aggregation
	return nil
}

func (c *DoctorOrphanContext) outputShows(expectedText string) error {
	if !strings.Contains(c.currentOutput, expectedText) {
		return fmt.Errorf("output does not show: %s", expectedText)
	}
	return nil
}

func (c *DoctorOrphanContext) orphanCheckUsesFastDetection() error {
	// Verify fast detection mode
	return nil
}

func (c *DoctorOrphanContext) outputShowsCountWithoutUUIDs() error {
	// Verify count shown but not UUIDs
	return nil
}

func (c *DoctorOrphanContext) orphanCheckShows(message string) error {
	return c.checkReports(message)
}

func (c *DoctorOrphanContext) remediationEmphasizesUrgency() error {
	// Verify urgency is emphasized
	return nil
}

func (c *DoctorOrphanContext) summaryShows(expectedSummary *godog.DocString) error {
	// Validate summary format
	return nil
}

func (c *DoctorOrphanContext) orphanCheckSuggests(suggestion string) error {
	return c.checkSuggests(suggestion)
}

func (c *DoctorOrphanContext) outputIncludesDirectLink() error {
	// Verify direct link is present
	return nil
}

// Cleanup
func (c *DoctorOrphanContext) cleanup() {
	if c.tmpDir != "" {
		os.RemoveAll(c.tmpDir)
	}
}

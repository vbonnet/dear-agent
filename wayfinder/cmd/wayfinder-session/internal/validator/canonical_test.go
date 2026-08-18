package validator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestCanonicalValidatorBlocksBuildWithoutDeliverable(t *testing.T) {
	t.Parallel()

	v := NewValidator(&status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		CurrentWaypoint: status.PhaseV2Build,
		WaypointHistory: []status.WaypointHistory{
			{Name: status.PhaseV2Build, Status: status.PhaseStatusInProgress},
		},
	})

	err := v.CanCompletePhase(status.PhaseV2Build, t.TempDir(), "")
	if err == nil {
		t.Fatal("expected BUILD completion to fail without a deliverable")
	}
	if !contains(err.Error(), "no deliverable file found") {
		t.Fatalf("expected missing-deliverable error, got %v", err)
	}
}

func TestCanonicalValidatorBlocksDesignWithoutResearchEvidence(t *testing.T) {
	t.Parallel()

	v := NewValidator(&status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		CurrentWaypoint: status.PhaseV2Research,
		WaypointHistory: []status.WaypointHistory{
			{Name: status.PhaseV2Research, Status: status.PhaseStatusCompleted},
		},
	})

	err := v.CanStartPhase(status.PhaseV2Design, t.TempDir())
	if err == nil {
		t.Fatal("expected DESIGN start to fail without RESEARCH evidence")
	}
	if !contains(err.Error(), "RESEARCH-existing-solutions.md does not exist") {
		t.Fatalf("expected missing-research error, got %v", err)
	}
}

func TestCanonicalValidatorCompletesPlanWithFrontmatterAndMarkdownBody(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	plan := `---
phase: PLAN
phase_name: Delivery plan
wayfinder_session_id: test-session
created_at: 2026-07-22T09:00:00-07:00
---
# Implementation Plan

## Context
This section records the current system boundaries and constraints that shape the work.

## Design
This section records the chosen sequence, validation evidence, dependencies, and important delivery trade-offs.
`
	if err := os.WriteFile(filepath.Join(projectDir, "PLAN-design.md"), []byte(plan), 0o600); err != nil {
		t.Fatal(err)
	}

	v := NewValidator(&status.StatusV2{
		SchemaVersion:   status.SchemaVersion,
		CurrentWaypoint: status.PhaseV2Plan,
		WaypointHistory: []status.WaypointHistory{
			{Name: status.PhaseV2Plan, Status: status.PhaseStatusInProgress},
		},
	})

	if err := v.CanCompletePhase(status.PhaseV2Plan, projectDir, ""); err != nil {
		t.Fatalf("canonical PLAN completion failed: %v", err)
	}
}

package main

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func mkManifest(id, harness string) *manifest.Manifest {
	return &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     id,
		Name:          "s-" + id,
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now(),
		Context:       manifest.Context{Project: "/tmp/" + id},
		Tmux:          manifest.Tmux{SessionName: "s-" + id},
		Harness:       harness,
	}
}

func TestBuildMigrationPlan_CountsWritesAndValidity(t *testing.T) {
	manifests := []*manifest.Manifest{
		mkManifest("aaaaaaaa-0000-0000-0000-000000000001", "claude-code"), // ready
		mkManifest("bbbbbbbb-0000-0000-0000-000000000002", ""),            // needs harness backfill
		mkManifest("cccccccc-0000-0000-0000-000000000003", "gemini-cli"),  // ready
	}

	plan := buildMigrationPlan(manifests)

	if plan.Total != 3 {
		t.Errorf("Total = %d, want 3", plan.Total)
	}
	if plan.NeedsWrite() != 1 {
		t.Errorf("NeedsWrite = %d, want 1", plan.NeedsWrite())
	}
	if plan.Invalid() != 0 {
		t.Errorf("Invalid = %d, want 0 (all migrate to valid v3)", plan.Invalid())
	}
}

func TestBuildMigrationPlan_Empty(t *testing.T) {
	plan := buildMigrationPlan(nil)
	if plan.Total != 0 || plan.NeedsWrite() != 0 || plan.Invalid() != 0 {
		t.Errorf("empty plan = %+v, want all zero", plan)
	}
}

func TestBuildMigrationPlan_ReportsHarnessBackfill(t *testing.T) {
	plan := buildMigrationPlan([]*manifest.Manifest{
		mkManifest("dddddddd-0000-0000-0000-000000000004", ""),
	})

	if len(plan.Sessions) != 1 {
		t.Fatalf("len(Sessions) = %d, want 1", len(plan.Sessions))
	}
	changes := plan.Sessions[0].Changes
	if len(changes) != 1 || changes[0].Field != "harness" {
		t.Errorf("changes = %+v, want a single harness backfill", changes)
	}
}

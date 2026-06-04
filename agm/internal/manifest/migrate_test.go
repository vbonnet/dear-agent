package manifest

import (
	"testing"
	"time"
)

// validV2 returns a v2 manifest that already satisfies v2 validation, so tests
// can mutate single fields to exercise the v3 delta in isolation.
func validV2() *Manifest {
	return &Manifest{
		SchemaVersion: SchemaVersion,
		SessionID:     "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Name:          "demo",
		CreatedAt:     time.Now().Add(-time.Hour),
		UpdatedAt:     time.Now(),
		Context:       Context{Project: "/tmp/demo"},
		Tmux:          Tmux{SessionName: "demo"},
		Harness:       "claude-code",
	}
}

func TestMigrateV2ToV3_SetsSchemaAndInitsHistory(t *testing.T) {
	v3, changes := MigrateV2ToV3(validV2())

	if v3.SchemaVersion != V3SchemaVersion {
		t.Errorf("schema_version = %q, want %q", v3.SchemaVersion, V3SchemaVersion)
	}
	if v3.HarnessHistory == nil {
		t.Error("harness_history must be a non-nil empty slice, got nil")
	}
	if len(v3.HarnessHistory) != 0 {
		t.Errorf("harness_history len = %d, want 0", len(v3.HarnessHistory))
	}
	// A session that already has a harness needs no persistable change.
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none for an already-v3-ready session", changes)
	}
	if err := v3.Validate(); err != nil {
		t.Errorf("migrated manifest failed v3 validation: %v", err)
	}
}

func TestMigrateV2ToV3_BackfillsEmptyHarness(t *testing.T) {
	v2 := validV2()
	v2.Harness = ""

	v3, changes := MigrateV2ToV3(v2)

	if v3.Harness != DefaultHarness {
		t.Errorf("harness = %q, want %q", v3.Harness, DefaultHarness)
	}
	if len(changes) != 1 {
		t.Fatalf("changes = %v, want exactly 1 (harness backfill)", changes)
	}
	if changes[0].Field != "harness" || changes[0].To != DefaultHarness {
		t.Errorf("change = %+v, want harness -> %q", changes[0], DefaultHarness)
	}
	if err := v3.Validate(); err != nil {
		t.Errorf("migrated manifest failed v3 validation: %v", err)
	}
}

func TestMigrateV2ToV3_PreservesV2Fields(t *testing.T) {
	v2 := validV2()
	v2.Model = "claude-opus-4-8"
	v2.Lifecycle = LifecycleArchived
	v2.Context.Tags = []string{"a", "b"}

	v3, _ := MigrateV2ToV3(v2)

	if v3.SessionID != v2.SessionID {
		t.Errorf("session_id = %q, want %q", v3.SessionID, v2.SessionID)
	}
	if v3.Model != v2.Model {
		t.Errorf("model = %q, want %q", v3.Model, v2.Model)
	}
	if v3.Lifecycle != v2.Lifecycle {
		t.Errorf("lifecycle = %q, want %q", v3.Lifecycle, v2.Lifecycle)
	}
	if len(v3.Context.Tags) != 2 {
		t.Errorf("context.tags = %v, want 2 entries", v3.Context.Tags)
	}
}

func TestMigrateV2ToV3_Idempotent(t *testing.T) {
	// Migrating, downgrading, and migrating again must converge with no
	// further persistable changes.
	v3a, _ := MigrateV2ToV3(validV2())
	v3b, changes := MigrateV2ToV3(v3a.Downgrade())

	if len(changes) != 0 {
		t.Errorf("second migration produced changes %v, want none (idempotency)", changes)
	}
	if v3b.SchemaVersion != V3SchemaVersion {
		t.Errorf("schema_version = %q, want %q", v3b.SchemaVersion, V3SchemaVersion)
	}
	if err := v3b.Validate(); err != nil {
		t.Errorf("re-migrated manifest failed v3 validation: %v", err)
	}
}

func TestManifestV3_Downgrade_DropsV3OnlyFields(t *testing.T) {
	v3, _ := MigrateV2ToV3(validV2())
	v3.HarnessHistory = []HarnessSwitch{{
		Timestamp:   time.Now(),
		FromHarness: "claude-code",
		ToHarness:   "gemini-cli",
	}}

	v2 := v3.Downgrade()

	// The stored shape stays v2 (harness_history has no backing column).
	if v2.SchemaVersion != SchemaVersion {
		t.Errorf("downgraded schema_version = %q, want %q", v2.SchemaVersion, SchemaVersion)
	}
	if v2.Harness != v3.Harness {
		t.Errorf("harness = %q, want %q", v2.Harness, v3.Harness)
	}
}

package manifest

// V3SchemaVersion is the schema_version string produced by MigrateV2ToV3.
const V3SchemaVersion = "3.0"

// DefaultHarness is assigned to sessions that predate harness tracking. It
// mirrors the storage-layer default (see internal/dolt/sessions.go) so the
// migration and the write path agree on the legacy fallback.
const DefaultHarness = "claude-code"

// FieldChange describes a single persistable field mutation produced by a
// migration. Only fields that have a backing column in the canonical (Dolt)
// store are reported here, so the caller never claims to apply a change it
// cannot persist.
type FieldChange struct {
	Field string // manifest field name (e.g. "harness")
	From  string // value before migration ("(empty)" for unset)
	To    string // value after migration
}

// MigrateV2ToV3 upgrades a v2 Manifest to the v3 ManifestV3 schema and returns
// the new manifest together with the list of persistable field changes that the
// upgrade applied.
//
// The function is pure (no I/O) and idempotent: a manifest that already
// satisfies the v3 invariants yields zero FieldChanges.
//
// The v3 delta over v2 is:
//   - schema_version is set to "3.0". This is always set on the returned struct
//     but is NOT reported as a FieldChange: schema_version is synthesized by the
//     storage layer at read time (it is not a persisted per-session column), so
//     it cannot be written back per session. The v2->v3 schema transition for
//     stored data is owned by the Dolt migration system, not by this converter.
//   - harness becomes required (it is optional in v2). Legacy sessions with an
//     empty harness are backfilled to DefaultHarness, and that backfill IS a
//     persistable FieldChange because harness has a backing column.
//   - harness_history is initialized to a non-nil empty slice so the result
//     passes ManifestV3.Validate. harness_history has no backing column yet, so
//     it is structural only and is not reported as a FieldChange.
func MigrateV2ToV3(m *Manifest) (*ManifestV3, []FieldChange) {
	v3 := &ManifestV3{
		SchemaVersion:  V3SchemaVersion,
		SessionID:      m.SessionID,
		Name:           m.Name,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
		Lifecycle:      m.Lifecycle,
		Context:        m.Context,
		Claude:         m.Claude,
		Tmux:           m.Tmux,
		Harness:        m.Harness,
		Model:          m.Model,
		HarnessHistory: []HarnessSwitch{},
	}

	var changes []FieldChange

	// harness is required in v3; backfill legacy empties to the storage default.
	if v3.Harness == "" {
		changes = append(changes, FieldChange{Field: "harness", From: "(empty)", To: DefaultHarness})
		v3.Harness = DefaultHarness
	}

	return v3, changes
}

// Downgrade copies the v3 manifest's v2-representable fields back onto a
// Manifest so the result can be persisted through the existing Store, which
// only understands the v2 shape. The v3-only field (HarnessHistory) is dropped
// because the canonical store has no column for it yet.
func (m *ManifestV3) Downgrade() *Manifest {
	return &Manifest{
		SchemaVersion: SchemaVersion, // stored shape stays v2 until the store learns v3
		SessionID:     m.SessionID,
		Name:          m.Name,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
		Lifecycle:     m.Lifecycle,
		Context:       m.Context,
		Claude:        m.Claude,
		Tmux:          m.Tmux,
		Harness:       m.Harness,
		Model:         m.Model,
	}
}

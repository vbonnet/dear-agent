// Package migrate provides V1 to V2 migration utilities for Wayfinder status files.
//
// The migrate package converts legacy V1 WAYFINDER-STATUS.md files (13-phase model)
// to the new V2 schema (9-phase consolidated model). It handles:
//
//   - Phase mapping and merging (S4→D4, S5→S6, S8/S9/S10→S8)
//   - Data preservation (100% of V1 data retained)
//   - Schema validation
//   - Dry-run mode for preview
//
// # Phase Mapping
//
// The converter maps V1's 13 phases to V2's 9 phases:
//
//	V1 Phase  → V2 Phase  | Notes
//	----------|-----------|------
//	W0        → W0        | Unchanged
//	D1-D4     → D1-D4     | Unchanged
//	S4        → D4        | Merged (stakeholder approval)
//	S5        → S6        | Merged (research notes)
//	S6        → S6        | Unchanged
//	S7        → S7        | Unchanged
//	S8        → S8        | BUILD phase (implementation)
//	S9        → S8        | Merged (validation status)
//	S10       → S8        | Merged (deployment status)
//	S11       → S11       | Unchanged
//
// # Usage
//
// Basic conversion:
//
//	v1Status, err := status.ReadFrom("/path/to/project")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	v2Status, err := migrate.ConvertV1ToV2(v1Status, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Dry-run mode (preview changes):
//
//	opts := &migrate.ConvertOptions{
//	    DryRun: true,
//	}
//	v2Status, err := migrate.ConvertV1ToV2(v1Status, opts)
//
// Custom project metadata:
//
//	opts := &migrate.ConvertOptions{
//	    ProjectName: "My Project",
//	    ProjectType: status.ProjectTypeFeature,
//	    RiskLevel:   status.RiskLevelL,
//	    PreserveSessionID: true,
//	}
//	v2Status, err := migrate.ConvertV1ToV2(v1Status, opts)
//
// # Data Preservation
//
// The converter ensures 100% data preservation:
//
//   - All phase timestamps (started_at, completed_at) are preserved
//   - Phase outcomes are mapped to V2 equivalents
//   - Session metadata is converted to V2 format
//   - Optional V1 session ID can be preserved as a tag
//
// # Validation
//
// The converter validates:
//
//   - Schema version is "1.0"
//   - Required V1 fields are present
//   - Phase names are valid V1 phases
//   - Timestamps are in correct order
//
// # Error Handling
//
// The converter returns errors for:
//
//   - Nil or invalid V1 status
//   - Missing required fields
//   - Invalid phase names
//   - Schema version mismatches
package migrate

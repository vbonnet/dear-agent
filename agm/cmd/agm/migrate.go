package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var migrateDryRun bool

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate AGM data between schema versions",
	Long: `Migrate AGM data between manifest schema versions.

Subcommands operate on the canonical session store and support --dry-run so the
plan can be previewed before anything is written.`,
}

var migrateV2ToV3Cmd = &cobra.Command{
	Use:   "v2-to-v3",
	Short: "Upgrade session manifests from schema v2 to v3",
	Long: `Upgrade session manifests from schema v2 to the v3 schema.

The v3 schema (internal/manifest/v3.go) adds multi-harness tracking on top of
v2: harness becomes a required field and harness_history records harness
switches over a session's lifetime.

What this command does:
  - Loads every session (active and archived) from the canonical store.
  - Computes the v2->v3 upgrade for each via manifest.MigrateV2ToV3 and
    validates the result against the v3 schema.
  - Backfills the v3-required harness field for any legacy session that lacks
    one (defaulting to "claude-code", matching the storage-layer default).

What this command intentionally does NOT do:
  - It does not write schema_version per session. schema_version is synthesized
    by the storage layer at read time (it is not a persisted column), so the
    stored v2->v3 transition is owned by the Dolt migration system, not by a
    per-session rewrite.
  - It does not persist harness_history. That field exists in the v3 struct but
    has no backing column yet; persisting it requires a storage-schema change
    tracked separately (see ce-6as.103 / the v3-storage follow-up).

Examples:
  agm migrate v2-to-v3 --dry-run   # preview the plan, write nothing
  agm migrate v2-to-v3             # apply harness backfills`,
	RunE: runMigrateV2ToV3,
}

func init() {
	migrateV2ToV3Cmd.Flags().BoolVar(&migrateDryRun, "dry-run", false,
		"preview the migration plan without writing any changes")
	migrateCmd.AddCommand(migrateV2ToV3Cmd)
	rootCmd.AddCommand(migrateCmd)
}

// sessionPlan is the per-session outcome of computing the v2->v3 upgrade.
type sessionPlan struct {
	SessionID string
	Name      string
	Changes   []manifest.FieldChange
	ValidErr  error
}

// migrationPlan is the aggregate result of planning a v2->v3 migration over a
// set of sessions. It is computed by a pure function so it can be tested
// without a live store.
type migrationPlan struct {
	Total    int
	Sessions []sessionPlan
}

// NeedsWrite reports the number of sessions with at least one persistable change.
func (p migrationPlan) NeedsWrite() int {
	n := 0
	for _, s := range p.Sessions {
		if len(s.Changes) > 0 {
			n++
		}
	}
	return n
}

// Invalid reports the number of sessions whose migrated form fails v3 validation.
func (p migrationPlan) Invalid() int {
	n := 0
	for _, s := range p.Sessions {
		if s.ValidErr != nil {
			n++
		}
	}
	return n
}

// buildMigrationPlan computes the v2->v3 upgrade for each manifest. Pure: no I/O.
func buildMigrationPlan(manifests []*manifest.Manifest) migrationPlan {
	plan := migrationPlan{Total: len(manifests)}
	for _, m := range manifests {
		v3, changes := manifest.MigrateV2ToV3(m)
		plan.Sessions = append(plan.Sessions, sessionPlan{
			SessionID: m.SessionID,
			Name:      m.Name,
			Changes:   changes,
			ValidErr:  v3.Validate(),
		})
	}
	return plan
}

func runMigrateV2ToV3(_ *cobra.Command, _ []string) error {
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer adapter.Close()

	// Migration covers every session, archived included.
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	plan := buildMigrationPlan(manifests)
	printMigrationPlan(plan)

	if migrateDryRun {
		ui.PrintWarning("dry-run: no changes were written")
		return nil
	}

	if plan.NeedsWrite() == 0 {
		ui.PrintSuccess("All sessions already satisfy the v3 schema; nothing to write")
		return nil
	}

	applied := 0
	for i, s := range plan.Sessions {
		if len(s.Changes) == 0 {
			continue
		}
		v3, _ := manifest.MigrateV2ToV3(manifests[i])
		if err := adapter.UpdateSession(v3.Downgrade()); err != nil {
			return fmt.Errorf("failed to migrate session %s (%s): %w", s.Name, s.SessionID, err)
		}
		applied++
	}

	ui.PrintSuccess(fmt.Sprintf("Migrated %d session(s) to v3 schema invariants", applied))
	return nil
}

func printMigrationPlan(plan migrationPlan) {
	fmt.Printf("v2 -> v3 migration plan: %d session(s)\n", plan.Total)
	fmt.Printf("  needing changes: %d\n", plan.NeedsWrite())
	if plan.Invalid() > 0 {
		fmt.Printf("  failing v3 validation: %d\n", plan.Invalid())
	}

	for _, s := range plan.Sessions {
		if len(s.Changes) == 0 && s.ValidErr == nil {
			continue
		}
		fmt.Printf("\n  %s (%s)\n", s.Name, s.SessionID)
		for _, c := range s.Changes {
			fmt.Printf("    - %s: %s -> %s\n", c.Field, c.From, c.To)
		}
		if s.ValidErr != nil {
			fmt.Printf("    ! v3 validation: %v\n", s.ValidErr)
		}
	}

	fmt.Println()
	fmt.Println("Note: schema_version and harness_history are not written per session.")
	fmt.Println("  schema_version is synthesized by the storage layer; harness_history")
	fmt.Println("  has no backing column yet (storage-schema follow-up).")
}

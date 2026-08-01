package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/gclog"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	sandboxGCReap   bool
	sandboxGCMinAge string
	sandboxGCJSON   bool
)

type sandboxGCSessionStore interface {
	ListSessions(*dolt.SessionFilter) ([]*manifest.Manifest, error)
	Close() error
}

var (
	sandboxGCStoreConfigs = configuredSandboxGCStoreConfigs
	openSandboxGCStore    = func(config *dolt.Config) (sandboxGCSessionStore, error) {
		return dolt.New(config)
	}
	logSandboxGCEntry = logSandboxGCEntryDefault
)

var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "Manage session sandbox directories (~/.agm/sandboxes)",
}

var sandboxGCCmd = &cobra.Command{
	Use:   "gc",
	Short: "Reap sandbox dirs of archived/dead sessions (dry-run by default)",
	Long: `Sweep ~/.agm/sandboxes and reap every sandbox that provably belongs to
no live session (ce-uxju: a 2.3T / 541-dir sandbox leak crashed the host).

DRY-RUN BY DEFAULT: without --reap nothing is deleted; the sweep only reports
what it would do.

A sandbox is reaped ONLY when ALL safety gates pass:
  - path validation: the target is directly under ~/.agm/sandboxes
  - no live session: no non-archived session references it (aborts entirely if
    the session store is unreachable or empty)
  - no live process: no process has a cwd or open fd inside it
  - no mount inside: after a best-effort unmount, the mount table is re-read
    and the reap refuses if any mount point remains at or under the sandbox
    (deleting through a live overlay mount can destroy the source repo)
  - age gate: sandboxes younger than --min-age are never touched

Non-git and partial sandbox dirs are ordinary reapable content (ce-nd1z);
they are never reported as errors.

Examples:
  # Preview (dry-run, default)
  agm sandbox gc

  # Actually delete eligible sandboxes
  agm sandbox gc --reap

  # Machine-readable output for the launchd sweep
  agm sandbox gc --reap --json`,
	RunE: runSandboxGC,
}

func runSandboxGC(cmd *cobra.Command, args []string) error {
	var minAge time.Duration
	var err error
	if sandboxGCMinAge != "" {
		minAge, err = parseDuration(sandboxGCMinAge)
		if err != nil {
			return fmt.Errorf("invalid --min-age %q: %w", sandboxGCMinAge, err)
		}
	}

	liveSessionIDs, warnings, err := sandboxGCLiveSessionIDs()
	if err != nil {
		logSandboxGCEntry(gclog.Entry{
			Operation: "sandbox_gc_error",
			Reason:    "live_session_inventory_failed",
			Error:     err.Error(),
			DryRun:    !sandboxGCReap,
		})
		return fmt.Errorf("failed to build live-session inventory: %w", err)
	}
	for _, warning := range warnings {
		logSandboxGCEntry(gclog.Entry{
			Operation: "sandbox_gc_warning",
			Reason:    "workspace_store_skipped",
			Error:     warning,
			DryRun:    !sandboxGCReap,
		})
	}

	result, err := ops.SandboxGC(&ops.OpContext{}, &ops.SandboxGCRequest{
		Reap:           sandboxGCReap,
		MinAge:         minAge,
		LiveSessionIDs: constantLiveSessionIDs(liveSessionIDs),
		Warnings:       warnings,
	})
	if err != nil {
		logSandboxGCEntry(gclog.Entry{
			Operation: "sandbox_gc_error",
			Reason:    "sweep_failed",
			Error:     err.Error(),
			DryRun:    !sandboxGCReap,
		})
		return handleError(err)
	}

	if sandboxGCJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	printSandboxGCText(result)
	return nil
}

// printSandboxGCText renders the human-readable sweep report.
func printSandboxGCText(result *ops.SandboxGCResult) {
	if result.DryRun {
		fmt.Println("DRY RUN — no sandboxes were removed (use --reap to delete)")
	}
	for _, warning := range result.Warnings {
		ui.PrintWarning(warning)
	}
	for _, e := range result.Entries {
		switch e.Action {
		case "reaped":
			fmt.Printf("  [reaped]     %s\n", e.Name)
		case "would-reap":
			fmt.Printf("  [would reap] %s\n", e.Name)
		case "kept":
			fmt.Printf("  [kept]       %s (%s)\n", e.Name, e.Reason)
		case "error":
			fmt.Printf("  [error]      %s: %s\n", e.Name, e.Reason)
		}
	}
	fmt.Println()

	summary := fmt.Sprintf("Scanned %d, reaped %d, kept %d", result.Scanned, result.Reaped, result.Kept)
	if result.Errors > 0 {
		summary += fmt.Sprintf(", errors %d", result.Errors)
	}
	switch {
	case result.DryRun:
		ui.PrintSuccess(fmt.Sprintf("Dry run: %s", summary))
	case result.Reaped > 0:
		ui.PrintSuccess(summary)
	default:
		fmt.Println(summary)
	}
	if result.Errors > 0 {
		ui.PrintWarning(fmt.Sprintf("%d sandbox(es) failed to remove — check ~/.agm/logs/gc.jsonl", result.Errors))
	}
}

func configuredSandboxGCStoreConfigs() ([]*dolt.Config, error) {
	path, err := getWorkspaceConfigPath()
	if err != nil {
		return nil, fmt.Errorf("resolve AGM workspace config path: %w", err)
	}
	return dolt.ConfiguredWorkspaceConfigsAt(path)
}

func constantLiveSessionIDs(live map[string]bool) func() (map[string]bool, error) {
	return func() (map[string]bool, error) {
		return live, nil
	}
}

func sandboxGCLiveSessionIDs() (map[string]bool, []string, error) {
	configs, err := sandboxGCStoreConfigs()
	if err != nil {
		return nil, nil, err
	}
	live := make(map[string]bool)
	var warnings []string
	var totalSessions int
	var reachableStores int
	for _, config := range configs {
		store, err := openSandboxGCStore(config)
		if err != nil {
			if isMissingDoltDatabaseError(err, config.Database) {
				warnings = append(warnings, fmt.Sprintf(
					"workspace %q skipped: Dolt database %q does not exist",
					config.Workspace, config.Database,
				))
				continue
			}
			return nil, warnings, fmt.Errorf("open Dolt session store for workspace %q: %w", config.Workspace, err)
		}
		sessions, listErr := store.ListSessions(nil)
		closeErr := store.Close()
		if listErr != nil {
			return nil, warnings, fmt.Errorf("list sessions from workspace %q: %w", config.Workspace, listErr)
		}
		if closeErr != nil {
			return nil, warnings, fmt.Errorf("close Dolt session store for workspace %q: %w", config.Workspace, closeErr)
		}
		reachableStores++
		totalSessions += len(sessions)
		for _, session := range sessions {
			if session.Lifecycle != manifest.LifecycleArchived {
				live[session.SessionID] = true
			}
		}
	}
	if reachableStores == 0 {
		return nil, warnings, fmt.Errorf("no configured Dolt session stores were reachable")
	}
	if totalSessions == 0 {
		return nil, warnings, fmt.Errorf("configured Dolt session stores returned zero sessions — refusing to treat all sandboxes as orphaned")
	}
	return live, warnings, nil
}

func isMissingDoltDatabaseError(err error, database string) bool {
	if err == nil || database == "" {
		return false
	}
	msg := strings.ToLower(err.Error())
	db := strings.ToLower(database)
	return strings.Contains(msg, "database not found: "+db) ||
		strings.Contains(msg, "unknown database '"+db+"'") ||
		strings.Contains(msg, "unknown database \""+db+"\"")
}

func logSandboxGCEntryDefault(entry gclog.Entry) {
	logger, err := gclog.NewDefault()
	if err != nil {
		return
	}
	_ = logger.Log(entry)
}

func init() {
	sandboxGCCmd.Flags().BoolVar(&sandboxGCReap, "reap", false,
		"Actually delete eligible sandboxes (default: dry-run)")
	sandboxGCCmd.Flags().StringVar(&sandboxGCMinAge, "min-age", "",
		"Never touch sandboxes modified more recently than this (default: 1h)")
	sandboxGCCmd.Flags().BoolVar(&sandboxGCJSON, "json", false,
		"Emit a machine-readable JSON summary on stdout")
	sandboxCmd.AddCommand(sandboxGCCmd)
	rootCmd.AddCommand(sandboxCmd)
}

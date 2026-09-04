package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
		store, err := dolt.NewWithoutAutoStart(config)
		if err == nil || isMissingDoltDatabaseError(err, config.Database) {
			return store, err
		}
		// The endpoint was not reachable, so preserve the configured recovery
		// behavior for an offline Dolt server. A reachable endpoint reporting a
		// missing database never reaches this auto-start path.
		return dolt.New(config)
	}
	runSandboxGCSweep = ops.SandboxGC
	logSandboxGCEntry = logSandboxGCEntryDefault
)

var errSandboxGCReapTransportUnavailable = errors.New(
	"sandbox GC reaping is disabled: authenticated session-store endpoint transport is not configured; run without --reap for a read-only scan",
)

var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "Manage session sandbox directories (~/.agm/sandboxes)",
}

var sandboxGCCmd = &cobra.Command{
	Use:   "gc",
	Short: "Inspect orphaned sandboxes (destructive mode currently contained)",
	Long: `Inspect ~/.agm/sandboxes and, once destructive authority is available,
reap every sandbox that provably belongs to no live session (ce-uxju: a 2.3T /
541-dir sandbox leak crashed the host).

DRY-RUN BY DEFAULT: without --reap nothing is deleted; the sweep only reports
what it would do.

DESTRUCTIVE MODE IS CONTAINED: --reap currently exits non-zero before reading
session stores or examining sandbox candidates. It remains unavailable until
every configured session-store endpoint can supply authenticated transport
authority (SGC-18).

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

  # Request destructive cleanup. This currently exits non-zero until
  # authenticated session-store endpoint transport is configured.
  agm sandbox gc --reap

  # Machine-readable read-only report
  agm sandbox gc --json`,
	Args: validateSandboxGCArgs,
	RunE: runSandboxGC,
}

func validateSandboxGCArgs(_ *cobra.Command, _ []string) error {
	return requireSandboxGCReapAuthority()
}

func requireSandboxGCReapAuthority() error {
	if sandboxGCReap {
		return errSandboxGCReapTransportUnavailable
	}
	return nil
}

func runSandboxGC(cmd *cobra.Command, args []string) error {
	// ce-1hu9.68.6: a loopback TCP listener plus database credentials does not
	// authenticate the process serving the live-session inventory. Until that
	// command receives an endpoint-bound capability, destructive execution must
	// stop before session-store discovery, store opens, sandbox-GC logging, or
	// sandbox examination begins. Args enforces this before inherited Cobra
	// pre-run hooks; this check keeps direct callers fail-closed as defense in
	// depth. Process-global package initialization is outside this command seam.
	if err := requireSandboxGCReapAuthority(); err != nil {
		return err
	}

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
		for _, warning := range warnings {
			logSandboxGCEntryTagged(gclog.Entry{
				Operation: "sandbox_gc_warning",
				Reason:    "workspace_store_skipped",
				Error:     warning,
				DryRun:    !sandboxGCReap,
			})
			if !sandboxGCJSON {
				ui.PrintWarning(warning)
			}
		}
		logSandboxGCEntryTagged(gclog.Entry{
			Operation: "sandbox_gc_error",
			Reason:    "live_session_inventory_failed",
			Error:     err.Error(),
			DryRun:    !sandboxGCReap,
		})
		return fmt.Errorf("failed to build live-session inventory: %w", err)
	}
	for _, warning := range warnings {
		logSandboxGCEntryTagged(gclog.Entry{
			Operation: "sandbox_gc_warning",
			Reason:    "workspace_store_skipped",
			Error:     warning,
			DryRun:    !sandboxGCReap,
		})
	}

	// Never reap on partial knowledge. A skipped workspace contributes none of
	// its live session IDs to the inventory, so every sandbox owned by that
	// workspace looks unowned and can pass the remaining gates — deleting a
	// live session's sandbox because we could not read the store that proves
	// it is live. Downgrade the run to a scan and say so loudly; the operator
	// fixes the topology, and a later tick may reap once authenticated endpoint
	// authority permits destructive execution again.
	reap, notice := effectiveSandboxGCReap(sandboxGCReap, warnings)
	if notice != "" {
		logSandboxGCEntryTagged(gclog.Entry{
			Operation: "sandbox_gc_warning",
			Reason:    "partial_inventory_reap_refused",
			Error:     notice,
			DryRun:    true,
		})
		if !sandboxGCJSON {
			ui.PrintWarning(notice)
		} else {
			fmt.Fprintln(os.Stderr, "agm sandbox gc: "+notice)
		}
	}

	result, err := runSandboxGCSweep(&ops.OpContext{}, &ops.SandboxGCRequest{
		Reap:           reap,
		MinAge:         minAge,
		LiveSessionIDs: constantLiveSessionIDs(liveSessionIDs),
		Warnings:       warnings,
	})
	if err != nil {
		logSandboxGCEntryTagged(gclog.Entry{
			Operation: "sandbox_gc_error",
			Reason:    "sweep_failed",
			Error:     err.Error(),
			DryRun:    !sandboxGCReap,
		})
		return handleError(err)
	}

	// Carry the refusal into the machine-readable result. `dry_run: true` alone
	// is ambiguous — it is the ordinary shape of a preview run — so an automated
	// caller that asked for --reap and got a scan would otherwise read a refusal
	// as a successful no-op sweep, and its `reaped` count (which now means
	// would-reap) as deleted sandboxes.
	result.ReapRefused = notice

	// Once authenticated authority lets a destructive request reach this point,
	// a sweep that reaps nothing is still a healthy sweep. Without a record for
	// the reap-nothing case,
	// "the reaper last ran at T" is indistinguishable from "the reaper has been
	// dead since T" — which is exactly how the hourly sandbox GC stayed broken
	// from 2026-07-05 to 2026-08-07 while ~/.agm/sandboxes grew to 239 GB.
	// disk-watchdog consumes this entry to alarm on a stale reaper.
	logSandboxGCEntryTagged(gclog.Entry{
		Operation: "sandbox_gc_completed",
		Reason: fmt.Sprintf("scanned=%d reaped=%d kept=%d errors=%d probe_failures=%d",
			result.Scanned, result.Reaped, result.Kept, result.Errors, result.ProbeFailures),
		// DryRun, Errors, and ProbeFailures are load-bearing for the reader,
		// not decoration. A dry run proves nothing was reclaimed, a sweep
		// whose deletions all failed is not a healthy sweep, and a sweep that
		// could not evaluate its safety gates (lsof/mount table/session store
		// unreadable) proves nothing was correctly evaluated either — even
		// though every entry reports "kept", not "error". disk-watchdog
		// refuses to count any of the three as a liveness heartbeat.
		DryRun:        result.DryRun,
		Errors:        result.Errors,
		ProbeFailures: result.ProbeFailures,
	})

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
		fmt.Println("DRY RUN — no sandboxes were removed (--reap is unavailable until session-store transport is authenticated)")
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
	if result.ProbeFailures > 0 {
		summary += fmt.Sprintf(", probe failures %d", result.ProbeFailures)
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
	if result.ProbeFailures > 0 {
		ui.PrintWarning(fmt.Sprintf("%d sandbox(es) could not be evaluated (lsof/mount/session-store probe failed) — check ~/.agm/logs/gc.jsonl", result.ProbeFailures))
	}
}

func configuredSandboxGCStoreConfigs() ([]*dolt.Config, error) {
	path, err := getWorkspaceConfigPath()
	if err != nil {
		return nil, fmt.Errorf("resolve AGM workspace config path: %w", err)
	}
	return dolt.ConfiguredWorkspaceConfigsIncludingDisabledAt(path)
}

func constantLiveSessionIDs(live map[string]bool) func() (map[string]bool, error) {
	return func() (map[string]bool, error) {
		return live, nil
	}
}

func sandboxGCLiveSessionIDs() (map[string]bool, []string, error) {
	if os.Getenv("AGM_DB_PATH") != "" {
		return sandboxGCLiveSessionIDsFromSQLite()
	}
	return sandboxGCLiveSessionIDsFromDolt()
}

func sandboxGCLiveSessionIDsFromSQLite() (map[string]bool, []string, error) {
	store, err := getStorage()
	if err != nil {
		return nil, nil, err
	}
	sessions, listErr := store.ListSessions(nil)
	closeErr := store.Close()
	if listErr != nil {
		return nil, nil, fmt.Errorf("list sessions from SQLite store: %w", listErr)
	}
	if closeErr != nil {
		return nil, nil, fmt.Errorf("close SQLite session store: %w", closeErr)
	}
	if len(sessions) == 0 {
		return nil, nil, fmt.Errorf("SQLite session store returned zero sessions — refusing to treat all sandboxes as orphaned")
	}
	return liveSessionIDsFromManifests(sessions), nil, nil
}

func sandboxGCLiveSessionIDsFromDolt() (map[string]bool, []string, error) {
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
		for sessionID := range liveSessionIDsFromManifests(sessions) {
			live[sessionID] = true
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

func liveSessionIDsFromManifests(sessions []*manifest.Manifest) map[string]bool {
	live := make(map[string]bool)
	for _, session := range sessions {
		if session.Lifecycle != manifest.LifecycleArchived {
			live[session.SessionID] = true
		}
	}
	return live
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

// logSandboxGCEntryDefault appends one record to the shared GC log.
//
// A write failure is reported on stderr rather than swallowed. The completion
// record is the watchdog's only proof of life: if it never lands, the command
// still exits zero while disk-watchdog raises a dead-reaper alarm six hours
// later, and nothing anywhere says the observation channel itself broke.
// effectiveSandboxGCReap decides whether a requested reap may actually delete.
//
// Never reap on partial knowledge. A skipped workspace contributes none of its
// live session IDs to the inventory, so every sandbox owned by that workspace
// looks unowned and can pass the remaining gates — deleting a live session's
// sandbox because we could not read the store that proves it is live. A
// partial inventory downgrades the run to a scan and returns the notice
// explaining why; a complete inventory reaps as requested. A dry run is
// already non-destructive and needs no notice.
func effectiveSandboxGCReap(requested bool, warnings []string) (bool, string) {
	if !requested || len(warnings) == 0 {
		return requested, ""
	}
	return false, fmt.Sprintf(
		"refusing to reap: live-session inventory is partial (%d workspace store(s) skipped); "+
			"scanning only. Fix the workspace Dolt endpoints, then re-run.", len(warnings))
}

// sandboxGCSourceEnv names the environment variable a scheduler sets to declare
// itself in every record this sweep writes.
//
// It is an environment variable rather than a flag deliberately: an older `agm`
// on the host ignores an unknown variable, whereas an unknown flag would turn
// an automated caller's remediation into a hard "unknown flag" failure the
// moment the caller was upgraded first.
const sandboxGCSourceEnv = "AGM_GC_SOURCE"

// logSandboxGCEntryTagged stamps the declared runner onto an entry before it is
// written. Readers use it to tell a scheduled sweep from one that some other
// component triggered on its own behalf; see gclog.Entry.Source.
func logSandboxGCEntryTagged(entry gclog.Entry) {
	if entry.Source == "" {
		entry.Source = strings.TrimSpace(os.Getenv(sandboxGCSourceEnv))
	}
	logSandboxGCEntry(entry)
}

func logSandboxGCEntryDefault(entry gclog.Entry) {
	logger, err := gclog.NewDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agm sandbox gc: WARNING: cannot open the GC log to record %q: %v\n", entry.Operation, err)
		return
	}
	if err := logger.Log(entry); err != nil {
		fmt.Fprintf(os.Stderr, "agm sandbox gc: WARNING: cannot record %q in the GC log: %v\n", entry.Operation, err)
	}
}

func init() {
	sandboxGCCmd.Flags().BoolVar(&sandboxGCReap, "reap", false,
		"Request deletion; currently unavailable until session-store endpoints have authenticated transport (default: dry-run)")
	sandboxGCCmd.Flags().StringVar(&sandboxGCMinAge, "min-age", "",
		"Never touch sandboxes modified more recently than this (default: 1h)")
	sandboxGCCmd.Flags().BoolVar(&sandboxGCJSON, "json", false,
		"Emit a machine-readable JSON summary on stdout")
	sandboxCmd.AddCommand(sandboxGCCmd)
	rootCmd.AddCommand(sandboxCmd)
}

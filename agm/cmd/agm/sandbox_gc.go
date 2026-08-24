package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/gclog"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	sandboxGCReap   bool
	sandboxGCMinAge string
	sandboxGCJSON   bool
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
	// fixes the topology, and the next tick reaps normally.
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

	result, err := ops.SandboxGC(&ops.OpContext{}, &ops.SandboxGCRequest{
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

	// Heartbeat: a sweep that reaps nothing is still a healthy sweep, and until
	// now it left no trace at all. Without a record for the reap-nothing case,
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

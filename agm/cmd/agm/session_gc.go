package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	gcDryRun       bool
	gcOlderThan    string
	gcProtectRoles string
	gcForce        bool
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Garbage-collect stopped sessions",
	Long: `Safely archive sessions that are no longer active.

By default, archives stopped sessions that have been inactive for more than
24 hours. Use --older-than=0 to disable the age filter and GC all eligible
stopped sessions regardless of age.

Safety guarantees:
  - Never archives sessions with an active tmux session
  - Never archives sessions in WORKING, PERMISSION_PROMPT, or other active states
  - Never archives protected roles (orchestrator, meta-orchestrator, overseer)
  - Aborts if session storage is unreachable (pre-GC health check)
  - All actions logged to ~/.agm/logs/gc.jsonl

Examples:
  # GC sessions inactive for more than 24 hours (default)
  agm session gc

  # Preview what would be GC'd (dry run)
  agm session gc --dry-run

  # GC sessions inactive for more than 7 days
  agm session gc --older-than=7d

  # GC all eligible stopped sessions (no age filter)
  agm session gc --older-than=0

  # GC with custom protected roles
  agm session gc --protect-roles=orchestrator,overseer,scheduler

  # Force GC (skip pre-archive verification)
  agm session gc --force`,
	RunE: runGC,
}

func runGC(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer cleanup()

	opCtx.DryRun = gcDryRun

	// Parse older-than duration
	var olderThanDuration = parseDurationSafe(gcOlderThan)

	// Parse protect-roles
	var roles []string
	if gcProtectRoles != "" {
		for _, r := range strings.Split(gcProtectRoles, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				roles = append(roles, r)
			}
		}
	}

	result, err := ops.GC(opCtx, &ops.GCRequest{
		OlderThan:    olderThanDuration,
		ProtectRoles: roles,
		Force:        gcForce,
	})
	if err != nil {
		return handleError(err)
	}

	// Display results
	if gcDryRun {
		fmt.Println("DRY RUN — no sessions were archived")
	}

	// Show archived sessions
	for _, s := range result.Sessions {
		switch s.Action {
		case "archived":
			if gcDryRun {
				fmt.Printf("  [would archive] %s (%s)\n", s.Name, s.SessionID[:8])
			} else {
				fmt.Printf("  [archived] %s (%s)\n", s.Name, s.SessionID[:8])
			}
		case "skipped":
			// Only show skipped in verbose/dry-run mode
			if gcDryRun {
				fmt.Printf("  [skip: %s] %s\n", s.Reason, s.Name)
			}
		case "error":
			fmt.Printf("  [error] %s: %s\n", s.Name, s.Error)
		}
	}

	fmt.Println()

	// Summary
	summary := fmt.Sprintf("Scanned %d, archived %d, skipped %d",
		result.Scanned, result.Archived, result.Skipped)
	if result.Errors > 0 {
		summary += fmt.Sprintf(", errors %d", result.Errors)
	}

	if gcDryRun {
		ui.PrintSuccess(fmt.Sprintf("Dry run: %s", summary))
	} else if result.Archived > 0 {
		ui.PrintSuccess(summary)
	} else {
		fmt.Println(summary)
	}

	if result.Errors > 0 {
		ui.PrintWarning(fmt.Sprintf("%d session(s) failed to archive — check gc.jsonl", result.Errors))
	}

	return nil
}

// parseDurationSafe parses a duration string, returning 0 on empty/error.
func parseDurationSafe(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := parseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

func init() {
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false,
		"Preview what would be GC'd without archiving")
	gcCmd.Flags().StringVar(&gcOlderThan, "older-than", "24h",
		"Only GC sessions inactive for at least this duration (e.g., 24h, 7d, 30d). Default: 24h")
	gcCmd.Flags().StringVar(&gcProtectRoles, "protect-roles", "",
		"Comma-separated role substrings to protect (default: orchestrator,meta-orchestrator,overseer)")
	gcCmd.Flags().BoolVarP(&gcForce, "force", "f", false,
		"Skip pre-archive verification checks")
	sessionCmd.AddCommand(gcCmd)
}

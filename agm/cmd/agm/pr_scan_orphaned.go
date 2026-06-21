package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/orphanpr"
)

var (
	orphanRepo string
	orphanDB   string
)

var prScanOrphanedCmd = &cobra.Command{
	Use:   "scan-orphaned",
	Short: "Detect open PRs whose tracking bead is already closed",
	Long: `Scan open pull requests and flag any whose referenced bead is CLOSED.

This closes the process gap from the 2026-06-21 DEAR retro: PRs #579/#581/#582
sat open in a conflict state for 20+ hours after their tracking beads were
closed, and no supervisor noticed. We tracked only the forward signal
("PR merged ⇒ done") and never the inverse — a bead closing without its PR
merging leaves the PR orphaned.

For each open PR it extracts bead references (ce-XXXX / ce-XXXX.N) from the
title and body, resolves each bead's status via 'bd --db <db> show', and
reports PRs that reference at least one closed bead. A bead whose status can't
be resolved is treated as unknown, never closed, so a transient bd failure can
never flag a live PR.

Detection only. Closing an orphaned PR requires human authorization.

Exit codes:
  0 - No orphaned PRs found
  1 - One or more orphaned PRs detected
  2 - Error scanning (gh unavailable, parse failure, etc.)

Examples:
  agm pr scan-orphaned
  agm pr scan-orphaned --repo vbonnet/dear-agent --output json
  agm pr scan-orphaned --db ~/beads/context-engine/.beads`,
	RunE: runPRScanOrphaned,
}

func init() {
	prCmd.AddCommand(prScanOrphanedCmd)

	home, _ := os.UserHomeDir()
	defaultDB := filepath.Join(home, "beads", "context-engine", ".beads")

	prScanOrphanedCmd.Flags().StringVar(&orphanRepo, "repo", "vbonnet/dear-agent",
		"GitHub repository (owner/name) to scan")
	prScanOrphanedCmd.Flags().StringVar(&orphanDB, "db", defaultDB,
		"beads database path passed to 'bd --db'")
}

// OrphanScanResult is the structured output of an orphaned-PR scan.
type OrphanScanResult struct {
	Repo     string                `json:"repo"`
	OpenPRs  int                   `json:"open_prs"`
	Orphaned []orphanpr.OrphanedPR `json:"orphaned"`
}

func runPRScanOrphaned(cmd *cobra.Command, args []string) error {
	prs, err := orphanpr.ListOpenPRs(orphanRepo)
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}

	statusOf := func(beadID string) orphanpr.BeadStatus {
		return orphanpr.BeadStatusFromDB(orphanDB, beadID)
	}
	orphaned := orphanpr.Scan(prs, statusOf)

	result := &OrphanScanResult{
		Repo:     orphanRepo,
		OpenPRs:  len(prs),
		Orphaned: orphaned,
	}

	if err := printResult(result, func() { printOrphanScanText(result) }); err != nil {
		return err
	}

	// Exit-gate semantics (mirrors `agm admin check-worktrees`): a non-zero
	// exit lets supervisor tooling detect orphans without parsing stdout.
	if len(orphaned) > 0 {
		cmd.SilenceUsage = true
		return fmt.Errorf("found %d orphaned PR(s) — bead closed but PR still open", len(orphaned))
	}
	return nil
}

func printOrphanScanText(r *OrphanScanResult) {
	fmt.Printf("\n=== Orphaned PR Scan — %s ===\n\n", r.Repo)
	fmt.Printf("Open PRs scanned: %d\n", r.OpenPRs)
	fmt.Printf("Orphaned (bead CLOSED, PR still open): %d\n", len(r.Orphaned))

	if len(r.Orphaned) == 0 {
		fmt.Println("\n  (none — every open PR tracks an open bead)")
		fmt.Println()
		return
	}

	for _, o := range r.Orphaned {
		fmt.Printf("\n  ✗ #%d %s\n", o.Number, o.Title)
		fmt.Printf("    closed beads: %s\n", strings.Join(o.ClosedBeads, ", "))
	}
	fmt.Println()
}

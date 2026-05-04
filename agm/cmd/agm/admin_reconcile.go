package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var reconcileFix bool

var reconcileCmd = &cobra.Command{
	Use:   "reconcile",
	Short: "Detect and fix mismatches between tmux sessions and Dolt state",
	Long: `Compare tmux sessions against Dolt database to find inconsistencies.

Detects two types of mismatches:

  ZOMBIE PANES: tmux session exists but Dolt says "archived" or "reaping"
    These are the result of a reaper crash or timeout. The pane is alive
    but invisible to "agm session list".
    Fix: unarchive in Dolt (set lifecycle back to active).

  ORPHAN RECORDS: Dolt says "active" but no tmux session exists
    These are sessions that crashed or were killed without archival.
    Fix: archive in Dolt.

Examples:
  agm admin reconcile          # Report mismatches
  agm admin reconcile --fix    # Report and fix mismatches`,
	RunE: reconcileRun,
}

// mismatch represents a single tmux/Dolt inconsistency.
type mismatch struct {
	Kind         string // "zombie" or "orphan"
	TmuxName     string
	DoltName     string
	SessionID    string
	DoltLifecycle string
	Description  string
}

func reconcileRun(cmd *cobra.Command, args []string) error {
	// Get tmux sessions
	tmuxSessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list tmux sessions: %w", err)
	}
	tmuxSet := make(map[string]bool, len(tmuxSessions))
	for _, s := range tmuxSessions {
		tmuxSet[s] = true
	}

	// Get all Dolt sessions (including archived)
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt: %w", err)
	}
	defer adapter.Close()

	allSessions, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list Dolt sessions: %w", err)
	}

	// Build lookup: normalized tmux name → Dolt manifest
	doltByTmux := make(map[string]*manifest.Manifest, len(allSessions))
	for _, m := range allSessions {
		if m.Tmux.SessionName != "" {
			normalized := tmux.NormalizeTmuxSessionName(m.Tmux.SessionName)
			doltByTmux[normalized] = m
		}
	}

	mismatches := detectZombies(tmuxSessions, doltByTmux)
	mismatches = append(mismatches, detectOrphans(allSessions, tmuxSet)...)

	// Report
	fmt.Printf("tmux sessions: %d\n", len(tmuxSessions))
	fmt.Printf("Dolt sessions (non-archived): %d\n", countNonArchived(allSessions))
	fmt.Println()

	if len(mismatches) == 0 {
		ui.PrintSuccess("No mismatches found — tmux and Dolt are consistent")
		return nil
	}

	// Print mismatch table
	fmt.Printf("Found %d mismatch(es):\n\n", len(mismatches))
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "TYPE\tSESSION\tDOLT LIFECYCLE\tDESCRIPTION\n")
	fmt.Fprintf(w, "----\t-------\t--------------\t-----------\n")
	for _, mm := range mismatches {
		name := mm.DoltName
		if name == "" {
			name = mm.TmuxName
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			strings.ToUpper(mm.Kind), name, mm.DoltLifecycle, mm.Description)
	}
	w.Flush()

	if !reconcileFix {
		fmt.Printf("\nRun with --fix to automatically resolve these mismatches\n")
		return nil
	}

	// Apply fixes
	fmt.Println()
	fixed, failed := applyReconcileFixes(adapter, mismatches)

	fmt.Println()
	if fixed > 0 {
		ui.PrintSuccess(fmt.Sprintf("Fixed %d mismatch(es)", fixed))
	}
	if failed > 0 {
		ui.PrintWarning(fmt.Sprintf("Failed to fix %d mismatch(es)", failed))
	}

	return nil
}

// detectZombies returns mismatches where a tmux session is alive but Dolt
// records the corresponding session as archived/reaping.
func detectZombies(tmuxSessions []string, doltByTmux map[string]*manifest.Manifest) []mismatch {
	var out []mismatch
	for _, ts := range tmuxSessions {
		normalized := tmux.NormalizeTmuxSessionName(ts)
		m, found := doltByTmux[normalized]
		if !found {
			continue
		}
		if m.Lifecycle == manifest.LifecycleArchived || m.Lifecycle == manifest.LifecycleReaping {
			out = append(out, mismatch{
				Kind:          "zombie",
				TmuxName:      ts,
				DoltName:      m.Name,
				SessionID:     m.SessionID,
				DoltLifecycle: m.Lifecycle,
				Description:   fmt.Sprintf("tmux pane alive but Dolt says %q", m.Lifecycle),
			})
		}
	}
	return out
}

// detectOrphans returns mismatches where Dolt records a non-archived session
// whose tmux session no longer exists.
func detectOrphans(allSessions []*manifest.Manifest, tmuxSet map[string]bool) []mismatch {
	var out []mismatch
	for _, m := range allSessions {
		if m.Lifecycle == manifest.LifecycleArchived || m.Tmux.SessionName == "" {
			continue
		}
		normalized := tmux.NormalizeTmuxSessionName(m.Tmux.SessionName)
		if tmuxSet[normalized] || tmuxSet[m.Tmux.SessionName] {
			continue
		}
		out = append(out, mismatch{
			Kind:          "orphan",
			TmuxName:      m.Tmux.SessionName,
			DoltName:      m.Name,
			SessionID:     m.SessionID,
			DoltLifecycle: m.Lifecycle,
			Description:   fmt.Sprintf("Dolt says %q but no tmux session", m.Lifecycle),
		})
	}
	return out
}

// applyReconcileFixes applies the fix action implied by each mismatch.
// Returns (fixed, failed) counts.
func applyReconcileFixes(adapter *dolt.Adapter, mismatches []mismatch) (int, int) {
	var fixed, failed int
	for _, mm := range mismatches {
		switch mm.Kind {
		case "zombie":
			fmt.Printf("Fixing zombie: unarchiving %s in Dolt...", mm.DoltName)
			if err := setLifecycle(adapter, mm.SessionID, ""); err != nil {
				fmt.Printf(" FAILED: %v\n", err)
				failed++
			} else {
				fmt.Printf(" OK\n")
				fixed++
			}
		case "orphan":
			fmt.Printf("Fixing orphan: archiving %s in Dolt...", mm.DoltName)
			if err := setLifecycle(adapter, mm.SessionID, manifest.LifecycleArchived); err != nil {
				fmt.Printf(" FAILED: %v\n", err)
				failed++
			} else {
				fmt.Printf(" OK\n")
				fixed++
			}
		}
	}
	return fixed, failed
}

func setLifecycle(adapter *dolt.Adapter, sessionID, lifecycle string) error {
	filter := &dolt.SessionFilter{}
	sessions, err := adapter.ListSessions(filter)
	if err != nil {
		return err
	}
	for _, m := range sessions {
		if m.SessionID == sessionID {
			m.Lifecycle = lifecycle
			return adapter.UpdateSession(m)
		}
	}
	return fmt.Errorf("session %s not found", sessionID)
}

func countNonArchived(sessions []*manifest.Manifest) int {
	count := 0
	for _, m := range sessions {
		if m.Lifecycle != manifest.LifecycleArchived {
			count++
		}
	}
	return count
}

func init() {
	reconcileCmd.Flags().BoolVar(&reconcileFix, "fix", false, "Automatically fix detected mismatches")
	adminCmd.AddCommand(reconcileCmd)
}

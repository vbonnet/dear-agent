package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/orphan"
)

var sessionReapOrphansCmd = &cobra.Command{
	Use:   "reap-orphans",
	Short: "Kill orphaned gopls / agm-mcp-server processes whose session has died",
	Long: `Kills long-lived per-session child processes that outlived the Claude
session that spawned them.

Every Claude Code session (Desktop, Cowork, or CLI) starts its own gopls
language server and its own agm-mcp-server. The normal Stop/SessionEnd
hooks reap them when a session exits cleanly, but an abrupt death (OOM,
kill -9, Desktop crash) leaves them running. The OS reparents an orphaned
child to PID 1 (launchd/init), so the provable signal that a gopls or
agm-mcp-server no longer belongs to any live session is PPID==1.

This command enumerates the process table, selects target processes
(default: gopls, agm-mcp-server) whose parent is PID 1, and terminates
them (SIGTERM, 5s grace, then SIGKILL). A process with a live parent keeps
its real PPID and is never touched, so the reap is safe by construction —
no session-to-process mapping is required.

It is intended to run on a periodic loop tick (e.g. 'agm loop tick') so the
orphan fleet cannot accumulate to swap exhaustion. Best-effort: a failed
kill is reported and does not abort the pass.

Examples:
  agm session reap-orphans --dry-run
  agm session reap-orphans
  agm session reap-orphans --targets gopls,agm-mcp-server`,
	Args: cobra.NoArgs,
	RunE: runSessionReapOrphans,
}

var (
	reapOrphansDryRun  bool
	reapOrphansTargets []string
	reapOrphansJSON    bool
)

func init() {
	sessionCmd.AddCommand(sessionReapOrphansCmd)
	sessionReapOrphansCmd.Flags().BoolVar(&reapOrphansDryRun, "dry-run", false,
		"Show what would be killed without killing anything")
	sessionReapOrphansCmd.Flags().StringSliceVar(&reapOrphansTargets, "targets", orphan.DefaultTargets,
		"Comma-separated process command names to reap when orphaned (PPID==1)")
	sessionReapOrphansCmd.Flags().BoolVar(&reapOrphansJSON, "json", false,
		"Emit a machine-readable JSON summary on stdout (for programmatic consumers like the overseer reclaimer)")
}

// orphanReapJSON is the stable machine-readable shape emitted by --json. It is
// the contract consumed by supervisor.OrphanReclaimer; keep the field names in
// sync with that parser.
type orphanReapJSON struct {
	Targets      []string `json:"targets"`
	DryRun       bool     `json:"dry_run"`
	OrphansFound int      `json:"orphans_found"`
	Killed       int      `json:"killed"`
	Failed       int      `json:"failed"`
	KilledPIDs   []int    `json:"killed_pids"`
	FailedPIDs   []int    `json:"failed_pids"`
}

func runSessionReapOrphans(cmd *cobra.Command, args []string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	res, err := orphan.Reap(orphan.PSLister{}, &mcp.SignalKiller{}, reapOrphansTargets, reapOrphansDryRun, logger)
	if err != nil {
		return err
	}

	if reapOrphansJSON {
		return writeOrphanJSON(cmd.OutOrStdout(), res)
	}
	printOrphanSummary(cmd.OutOrStdout(), res)
	return nil
}

func writeOrphanJSON(w io.Writer, res orphan.Result) error {
	out := orphanReapJSON{
		Targets:      res.Targets,
		DryRun:       res.DryRun,
		OrphansFound: len(res.Orphans),
		Killed:       len(res.Killed),
		Failed:       len(res.Failed),
		KilledPIDs:   pids(res.Killed),
		FailedPIDs:   pids(res.Failed),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func pids(procs []orphan.Proc) []int {
	out := make([]int, 0, len(procs))
	for _, p := range procs {
		out = append(out, p.PID)
	}
	return out
}

func printOrphanSummary(w io.Writer, res orphan.Result) {
	verb := "reaped"
	if res.DryRun {
		verb = "would reap"
	}
	fmt.Fprintf(w, "orphan reaper (%s): %d orphan(s) found, %d %s, %d failed\n",
		strings.Join(res.Targets, ","), len(res.Orphans), len(res.Killed), verb, len(res.Failed))

	if res.DryRun {
		for _, o := range res.Orphans {
			fmt.Fprintf(w, "  would reap pid=%d command=%s\n", o.PID, o.Command)
		}
		return
	}
	for _, o := range res.Killed {
		fmt.Fprintf(w, "  reaped  pid=%d command=%s\n", o.PID, o.Command)
	}
	for _, o := range res.Failed {
		fmt.Fprintf(w, "  FAILED  pid=%d command=%s err=%s\n", o.PID, o.Command, res.KillError[o.PID])
	}
}

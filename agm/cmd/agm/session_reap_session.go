package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/orphan"
)

var sessionReapSessionCmd = &cobra.Command{
	Use:   "reap-session",
	Short: "Kill the ending session's own gopls / agm-mcp-server descendants",
	Long: `Reaps the per-session child processes (gopls, agm-mcp-server) belonging
to the Claude session that is ending, at the moment it ends.

This complements 'agm session reap-orphans'. That command keys on PPID==1,
which only fires once a child has been reparented to init — i.e. after its
session has fully exited. It therefore cannot catch the ending session's own
children during a graceful session end: while the SessionEnd hook runs, those
children are still parented to the live session (PPID != 1), so reap-orphans
skips them and they linger until the next hourly launchd sweep.

reap-session resolves the session root — the nearest ancestor of this process
whose command basename is one of --root-names (default 'claude') — and kills
the --targets processes within that root's subtree (SIGTERM, 5s grace, then
SIGKILL). Because it only descends from an ancestor of itself, a sibling
session is structurally unreachable; if no session root is found it reaps
nothing. Intended to run from the SessionEnd hook. Best-effort: a failed kill
is reported and does not abort the pass.

Examples:
  agm session reap-session --dry-run
  agm session reap-session
  agm session reap-session --root-pid 12345
  agm session reap-session --targets gopls,agm-mcp-server --root-names claude`,
	Args: cobra.NoArgs,
	RunE: runSessionReapSession,
}

var (
	reapSessionDryRun    bool
	reapSessionTargets   []string
	reapSessionRootNames []string
	reapSessionRootPID   int
)

func init() {
	sessionCmd.AddCommand(sessionReapSessionCmd)
	sessionReapSessionCmd.Flags().BoolVar(&reapSessionDryRun, "dry-run", false,
		"Show what would be killed without killing anything")
	sessionReapSessionCmd.Flags().StringSliceVar(&reapSessionTargets, "targets", orphan.DefaultTargets,
		"Comma-separated process command names to reap within the session subtree")
	sessionReapSessionCmd.Flags().StringSliceVar(&reapSessionRootNames, "root-names", orphan.DefaultRootNames,
		"Comma-separated ancestor command names treated as the session root")
	sessionReapSessionCmd.Flags().IntVar(&reapSessionRootPID, "root-pid", 0,
		"Use this PID as the session root directly, bypassing ancestor auto-detection")
}

func runSessionReapSession(cmd *cobra.Command, args []string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Start the ancestor walk from our parent — typically the SessionEnd hook
	// shell — so the nearest 'claude' ancestor is this ending session.
	startPID := os.Getppid()

	res, err := orphan.ReapSession(orphan.PSLister{}, &mcp.SignalKiller{},
		startPID, reapSessionRootPID, reapSessionRootNames, reapSessionTargets, reapSessionDryRun, logger)
	if err != nil {
		return err
	}

	printSessionReapSummary(cmd.OutOrStdout(), res)
	return nil
}

func printSessionReapSummary(w io.Writer, res orphan.SessionResult) {
	if !res.RootFound {
		fmt.Fprintf(w, "session reaper (%s): no session root found (start pid=%d, roots=%s) — nothing reaped\n",
			strings.Join(res.Targets, ","), res.StartPID, strings.Join(res.RootNames, ","))
		return
	}

	verb := "reaped"
	if res.DryRun {
		verb = "would reap"
	}
	fmt.Fprintf(w, "session reaper (%s): root pid=%d, %d process(es) found, %d %s, %d failed\n",
		strings.Join(res.Targets, ","), res.RootPID, len(res.Found), len(res.Killed), verb, len(res.Failed))

	if res.DryRun {
		for _, o := range res.Found {
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

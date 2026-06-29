package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

// supervisorProbeCmd runs the real OS SysResourceProbe once and appends the
// snapshot to the VROOM decision trail. It exists so the vroom-overseer tick
// (a persistent Claude session driven by a /loop prompt) has a single
// authoritative command for "measure and log system resources" instead of
// stitching together df/vm_stat/pgrep/sysctl by hand — the probe reads the
// same disk/memory/swap/FD/vnode/gopls metrics the in-process Overseer.Tick
// records as supervisor.over.resource_snapshot.
//
// The command is deliberately non-fatal: a probe error is logged to the trail
// (kind "overseer.resource.probe.error") and printed, but the process still
// exits 0 so a single bad probe never ends the overseer's /loop.
var supervisorProbeCmd = &cobra.Command{
	Use:   "probe",
	Short: "Measure and log system resources to the VROOM decision trail",
	Long: `Run the SysResourceProbe once and append the snapshot to
~/.agm/vroom/trail.jsonl (kind "overseer.resource.probe").

Intended to be called on every vroom-overseer tick so resource posture
(disk, memory, swap, open FDs, vnodes, orphaned gopls) is measured and
logged uniformly. The snapshot is also printed to stdout so the calling
session can evaluate escalation thresholds in the same tick.

Best-effort: if the probe fails, the error is logged (kind
"overseer.resource.probe.error") and printed, but the command still exits 0
so a single failed probe never ends the overseer /loop.`,
	RunE: runSupervisorProbe,
}

var supervisorProbeTrailPath string

func init() {
	supervisorCmd.AddCommand(supervisorProbeCmd)
	supervisorProbeCmd.Flags().StringVar(&supervisorProbeTrailPath, "trail", "",
		"path to append the probe record to (default ~/.agm/vroom/trail.jsonl)")
}

// vroomTrailPath returns the default VROOM decision-trail path
// (~/.agm/vroom/trail.jsonl).
func vroomTrailPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".agm", "vroom", "trail.jsonl"), nil
}

func runSupervisorProbe(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	trailPath := supervisorProbeTrailPath
	if trailPath == "" {
		p, err := vroomTrailPath()
		if err != nil {
			return err
		}
		trailPath = p
	}

	trail, err := decisiontrail.OpenJSONL(trailPath)
	if err != nil {
		return fmt.Errorf("open trail %q: %w", trailPath, err)
	}
	defer func() { _ = trail.Close() }()

	snap, probeErr := supervisor.NewSysResourceProbe().Snapshot(ctx)
	if probeErr != nil {
		// Non-fatal: record the failure and exit 0 so the overseer /loop
		// treats a bad probe as a skipped measurement, not a dead tick.
		_ = trail.Append(ctx, decisiontrail.Record{
			Role:    string(supervisor.RoleOverseer),
			Kind:    "overseer.resource.probe.error",
			Payload: map[string]any{"error": probeErr.Error()},
		})
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "supervisor probe: snapshot failed: %v (logged, continuing)\n", probeErr)
		return nil
	}

	payload := snapshotPayload(snap)
	if err := trail.Append(ctx, decisiontrail.Record{
		Role:    string(supervisor.RoleOverseer),
		Kind:    "overseer.resource.probe",
		Payload: payload,
	}); err != nil {
		// Logging is the whole point of the command, but even a write failure
		// must not end the /loop — report it and exit 0.
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "supervisor probe: trail append failed: %v (continuing)\n", err)
	}

	printSnapshot(cmd, snap)
	return nil
}

// snapshotPayload renders a ResourceSnapshot as the trail Record payload.
// Field names mirror supervisor.Overseer.recordSnapshot so the two snapshot
// kinds ("overseer.resource.probe" here, "supervisor.over.resource_snapshot"
// from the in-process Tick) deserialize uniformly.
func snapshotPayload(snap supervisor.ResourceSnapshot) map[string]any {
	return map[string]any{
		"disk_used_fraction":   snap.DiskUsedFraction,
		"memory_used_fraction": snap.MemoryUsedFraction,
		"free_physical_bytes":  snap.FreePhysicalMemoryBytes,
		"swap_used_fraction":   snap.SwapUsedFraction,
		"cpu_used_fraction":    snap.CPUUsedFraction,
		"open_fd_fraction":     snap.OpenFDFraction,
		"vnode_used_fraction":  snap.VnodeUsedFraction,
		"gopls_processes":      snap.GoplsProcesses,
		"stranded_worktrees":   snap.StrandedWorktrees,
		"orphaned_sessions":    snap.OrphanedSessions,
		"memorystatus_level":   snap.MemorystatusLevel,
	}
}

// printSnapshot writes a one-line human-readable summary so the overseer
// session can read the metrics it just logged without re-parsing the trail.
func printSnapshot(cmd *cobra.Command, snap supervisor.ResourceSnapshot) {
	_, _ = fmt.Fprintf(cmd.OutOrStdout(),
		"resource probe: disk=%.0f%% mem=%.0f%% swap=%.0f%% fd=%.0f%% vnode=%.0f%% gopls=%d worktrees=%d sessions=%d memorystatus=%d\n",
		snap.DiskUsedFraction*100,
		snap.MemoryUsedFraction*100,
		snap.SwapUsedFraction*100,
		snap.OpenFDFraction*100,
		snap.VnodeUsedFraction*100,
		snap.GoplsProcesses,
		snap.StrandedWorktrees,
		snap.OrphanedSessions,
		snap.MemorystatusLevel,
	)
}

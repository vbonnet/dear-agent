// Command vroom-mesh runs the canonical 3-supervisor VROOM mesh
// (Meta-Orchestrator, Orchestrator, Overseer) in a single process, with
// in-memory roadmap / queue / resource substrates. It exists so the
// supervisor mesh can be exercised end-to-end before the real adapters
// (beads, AGM session spawn, live resource probes) land.
//
// Usage:
//
//	vroom-mesh                        # default: 1s ticks, run until Ctrl-C
//	vroom-mesh --interval 500ms       # tighter cadence
//	vroom-mesh --duration 10s         # bounded run
//	vroom-mesh --trail /tmp/vroom.jsonl  # also write decision trail to disk
//	vroom-mesh --disk 0.95            # set the simulated disk usage fraction
//
// The decision trail is always written to stdout (one JSON object per
// line). Pipe through `jq` for human-readable output.
//
// Exit codes: 0 on clean shutdown (deadline / signal), 1 on misconfig.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "vroom-mesh:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("vroom-mesh", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		interval   = fs.Duration("interval", 1*time.Second, "loop interval per supervisor")
		duration   = fs.Duration("duration", 0, "stop after this duration (0 = run until SIGINT/SIGTERM)")
		threshold  = fs.Float64("threshold", 0.9, "Overseer escalation threshold for fraction metrics")
		diskFrac   = fs.Float64("disk", 0.2, "simulated disk usage fraction (0..1)")
		memFrac    = fs.Float64("memory", 0.3, "simulated memory usage fraction (0..1)")
		cpuFrac    = fs.Float64("cpu", 0.1, "simulated CPU usage fraction (0..1)")
		stranded   = fs.Int("stranded-worktrees", 0, "simulated stranded-worktree count")
		orphaned   = fs.Int("orphaned-sessions", 0, "simulated orphaned-session count")
		trailPath  = fs.String("trail", "", "optional path to append the decision trail to (also written to stdout)")
		nProposals = fs.Int("proposals", 2, "number of sample Work Order proposals to seed the roadmap with")
		nTasks     = fs.Int("tasks", 2, "number of sample tasks to seed the queue with")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	// stdout trail — one JSON object per line.
	stdoutTrail := decisiontrail.NewJSONLTrail(nopWriteCloser{stdout})

	// Optional fan-out to a file trail.
	var trail decisiontrail.Trail = stdoutTrail
	if *trailPath != "" {
		file, err := decisiontrail.OpenJSONL(*trailPath)
		if err != nil {
			return fmt.Errorf("open trail: %w", err)
		}
		trail = &teeTrail{a: stdoutTrail, b: file}
		defer file.Close()
	}

	// Substrates.
	roadmap := supervisor.NewInMemoryRoadmap()
	for i := 0; i < *nProposals; i++ {
		id := fmt.Sprintf("p%d", i+1)
		if err := roadmap.Submit(supervisor.WorkProposal{
			ID:     id,
			Title:  fmt.Sprintf("sample proposal %d", i+1),
			Reason: "demo seed",
		}); err != nil {
			return fmt.Errorf("seed roadmap: %w", err)
		}
	}

	queue := supervisor.NewInMemoryQueue()
	for i := 0; i < *nTasks; i++ {
		id := fmt.Sprintf("t%d", i+1)
		if err := queue.Enqueue(supervisor.Task{
			ID: id, Title: fmt.Sprintf("sample task %d", i+1), Worker: "coder",
		}); err != nil {
			return fmt.Errorf("seed queue: %w", err)
		}
	}

	probe := supervisor.NewInMemoryResourceProbe()
	probe.Set(supervisor.ResourceSnapshot{
		DiskUsedFraction:   *diskFrac,
		MemoryUsedFraction: *memFrac,
		CPUUsedFraction:    *cpuFrac,
		StrandedWorktrees:  *stranded,
		OrphanedSessions:   *orphaned,
	})

	// Supervisors.
	metaSup, err := supervisor.NewMetaOrchestrator(trail, roadmap)
	if err != nil {
		return err
	}
	orchSup, err := supervisor.NewOrchestrator(trail, queue)
	if err != nil {
		return err
	}
	overSup, err := supervisor.NewOverseer(trail, probe, supervisor.EscalationThreshold{Fraction: *threshold})
	if err != nil {
		return err
	}

	check := &supervisor.HeartbeatCheckSkill{Threshold: 3 * *interval}
	mkLoop := func(s supervisor.Supervisor) (*supervisor.Loop, error) {
		return supervisor.NewLoop(supervisor.LoopConfig{
			Supervisor: s,
			Mesh:       placeholderMesh{},
			Check:      check,
			Trail:      trail,
			Interval:   *interval,
		})
	}
	metaLoop, err := mkLoop(metaSup)
	if err != nil {
		return err
	}
	orchLoop, err := mkLoop(orchSup)
	if err != nil {
		return err
	}
	overLoop, err := mkLoop(overSup)
	if err != nil {
		return err
	}

	mesh, err := supervisor.NewMesh(metaLoop, orchLoop, overLoop)
	if err != nil {
		return err
	}

	// Build context with optional duration and signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if *duration > 0 {
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigCh:
			fmt.Fprintln(stderr, "vroom-mesh: shutdown signal received")
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := mesh.Run(ctx); err != nil && !isCancellation(err) {
		return fmt.Errorf("mesh: %w", err)
	}
	return nil
}

func isCancellation(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

// teeTrail writes every record to two underlying trails (typically stdout
// + a file). If either Append errors, the first error is returned but the
// second trail still gets the record.
type teeTrail struct {
	a, b decisiontrail.Trail
}

func (t *teeTrail) Append(ctx context.Context, rec decisiontrail.Record) error {
	errA := t.a.Append(ctx, rec)
	errB := t.b.Append(ctx, rec)
	if errA != nil {
		return errA
	}
	return errB
}

func (t *teeTrail) Close() error {
	errA := t.a.Close()
	errB := t.b.Close()
	if errA != nil {
		return errA
	}
	return errB
}

// nopWriteCloser wraps an io.Writer (e.g. os.Stdout) so it satisfies
// io.WriteCloser without closing the underlying stream.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// placeholderMesh satisfies PeerLookup so NewLoop's validation passes.
// supervisor.NewMesh rewires each Loop's mesh pointer afterwards, so peer
// lookups at runtime go through the real Mesh.
type placeholderMesh struct{}

func (placeholderMesh) Get(supervisor.Role) (supervisor.LoopStatus, bool) {
	return nil, false
}

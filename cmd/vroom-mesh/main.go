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
//	vroom-mesh --sys-probe            # use real OS resource metrics (disk+memory)
//	vroom-mesh --pressure --swap 0.95 --free-mem-mib 80  # exercise the graduated
//	                                  # memory-pressure auto-reaper (ce-80ca)
//
// The decision trail is always written to stdout (one JSON object per
// line). Pipe through `jq` for human-readable output.
//
// Exit codes: 0 on clean shutdown (deadline / signal), 1 on misconfig.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
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

// runConfig holds the parsed flag values for run(). Keeping them in a
// struct lets the seed / build helpers take one parameter instead of a
// dozen, and keeps run() under the gocyclo budget.
type runConfig struct {
	interval   time.Duration
	duration   time.Duration
	threshold  float64
	diskFrac   float64
	memFrac    float64
	cpuFrac    float64
	stranded   int
	orphaned   int
	trailPath  string
	nProposals        int
	nTasks            int
	sysProbe          bool    // use SysResourceProbe (real OS metrics) instead of InMemory
	reapTargets       string  // comma-separated --targets for the orphan reaper; empty = CLI default
	swapFrac          float64 // simulated swap usage fraction (drives graduated pressure)
	freeMemMiB        int     // simulated free physical RAM in MiB (0 = unknown)
	pressure          bool    // wire graduated memory-pressure monitoring + auto-reaper
	wakePeers         bool    // enable AGMRecovery: send `agm send wake-loop` to stale peers
	recoveryThreshold int     // consecutive stale ticks before a wake-loop is sent
}

func parseFlags(args []string, stderr io.Writer) (*runConfig, error) {
	fs := flag.NewFlagSet("vroom-mesh", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := &runConfig{}
	fs.DurationVar(&cfg.interval, "interval", 1*time.Second, "loop interval per supervisor")
	fs.DurationVar(&cfg.duration, "duration", 0, "stop after this duration (0 = run until SIGINT/SIGTERM)")
	fs.Float64Var(&cfg.threshold, "threshold", 0.9, "Overseer escalation threshold for fraction metrics")
	fs.Float64Var(&cfg.diskFrac, "disk", 0.2, "simulated disk usage fraction (0..1)")
	fs.Float64Var(&cfg.memFrac, "memory", 0.3, "simulated memory usage fraction (0..1)")
	fs.Float64Var(&cfg.cpuFrac, "cpu", 0.1, "simulated CPU usage fraction (0..1)")
	fs.IntVar(&cfg.stranded, "stranded-worktrees", 0, "simulated stranded-worktree count")
	fs.IntVar(&cfg.orphaned, "orphaned-sessions", 0, "simulated orphaned-session count")
	fs.StringVar(&cfg.trailPath, "trail", "", "optional path to append the decision trail to (also written to stdout)")
	fs.IntVar(&cfg.nProposals, "proposals", 2, "number of sample Work Order proposals to seed the roadmap with")
	fs.IntVar(&cfg.nTasks, "tasks", 2, "number of sample tasks to seed the queue with")
	fs.BoolVar(&cfg.sysProbe, "sys-probe", false, "use real OS resource metrics (disk+memory) instead of simulated values")
	fs.StringVar(&cfg.reapTargets, "reap-targets", "", "comma-separated process allowlist for the overseer's orphan reaper (empty = reaper default: gopls,agm-mcp-server). Only active with --sys-probe")
	fs.Float64Var(&cfg.swapFrac, "swap", 0, "simulated swap usage fraction (0..1) for graduated memory-pressure")
	fs.IntVar(&cfg.freeMemMiB, "free-mem-mib", 0, "simulated free physical RAM in MiB (0 = unknown)")
	fs.BoolVar(&cfg.pressure, "pressure", false, "wire graduated memory-pressure monitoring + auto-reaper into the Overseer")
	fs.BoolVar(&cfg.wakePeers, "wake-peers", false, "send `agm send wake-loop` to a peer once it has been stale for --recovery-threshold ticks (mutual-unblock)")
	fs.IntVar(&cfg.recoveryThreshold, "recovery-threshold", 3, "consecutive stale ticks a peer must miss before a wake-loop is sent (requires --wake-peers)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return cfg, nil
}

// openTrail returns the Trail run() should write to plus a close func.
// When trailPath is empty the stdout-only trail is returned; otherwise a
// tee that fans out to both stdout and the file. The close func must be
// invoked on shutdown to flush the file.
func openTrail(stdout io.Writer, trailPath string) (decisiontrail.Trail, func(), error) {
	stdoutTrail := decisiontrail.NewJSONLTrail(nopWriteCloser{stdout})
	if trailPath == "" {
		return stdoutTrail, func() {}, nil
	}
	file, err := decisiontrail.OpenJSONL(trailPath)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open trail: %w", err)
	}
	return &teeTrail{a: stdoutTrail, b: file}, func() { _ = file.Close() }, nil
}

func seedRoadmap(n int) (*supervisor.InMemoryRoadmap, error) {
	rm := supervisor.NewInMemoryRoadmap()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("p%d", i+1)
		if err := rm.Submit(supervisor.WorkProposal{
			ID:     id,
			Title:  fmt.Sprintf("sample proposal %d", i+1),
			Reason: "demo seed",
		}); err != nil {
			return nil, fmt.Errorf("seed roadmap: %w", err)
		}
	}
	return rm, nil
}

func seedQueue(n int) (*supervisor.InMemoryQueue, error) {
	q := supervisor.NewInMemoryQueue()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("t%d", i+1)
		if err := q.Enqueue(supervisor.Task{
			ID: id, Title: fmt.Sprintf("sample task %d", i+1), Worker: "coder",
		}); err != nil {
			return nil, fmt.Errorf("seed queue: %w", err)
		}
	}
	return q, nil
}

// buildMesh assembles the three supervisors, wraps each in a Loop, and
// returns the Mesh. NewMesh rewires each Loop's mesh pointer to the real
// Mesh, so the placeholderMesh passed to NewLoop is only there to satisfy
// validation.
func buildMesh(trail decisiontrail.Trail, roadmap *supervisor.InMemoryRoadmap, queue *supervisor.InMemoryQueue, probe supervisor.ResourceProbe, cfg *runConfig) (*supervisor.Mesh, error) {
	metaSup, err := supervisor.NewMetaOrchestrator(trail, roadmap)
	if err != nil {
		return nil, err
	}
	orchSup, err := supervisor.NewOrchestrator(trail, queue)
	if err != nil {
		return nil, err
	}
	overSup, err := supervisor.NewOverseer(trail, probe, supervisor.EscalationThreshold{Fraction: cfg.threshold})
	if err != nil {
		return nil, err
	}
	// Wire resource remediation only on the real-probe (production) path. With
	// the in-memory probe the snapshot is simulated, so shelling out to the
	// reaper would kill real host processes in response to fake pressure.
	if cfg.sysProbe {
		reclaimer := &supervisor.OrphanReclaimer{}
		if cfg.reapTargets != "" {
			reclaimer.Targets = splitTargets(cfg.reapTargets)
		}
		overSup = overSup.WithReclaimer(reclaimer)
	}
	if cfg.pressure {
		reaper, rerr := supervisor.NewAutoResourceReaper(trail,
			supervisor.WithReapReclaimer(supervisor.NewInMemoryReclaimer()),
			supervisor.WithSessionArchiver(supervisor.NewInMemorySessionArchiver()),
			supervisor.WithSpawnGate(supervisor.NewInMemorySpawnGate()),
			supervisor.WithPressureEscalator(supervisor.NewInMemoryPressureEscalator()),
		)
		if rerr != nil {
			return nil, rerr
		}
		overSup.WithMemoryPressure(supervisor.NewMemoryPressureMonitor(supervisor.MemoryPressureThresholds{}), reaper)
	}

	check := &supervisor.HeartbeatCheckSkill{Threshold: 3 * cfg.interval}

	// Recovery is the corrective-action half of the mutual-unblock invariant.
	// Off by default so the in-process harness doesn't shell out; --wake-peers
	// opts in and sends `agm send wake-loop` to a peer stale for too long.
	var recovery supervisor.PeerRecovery
	if cfg.wakePeers {
		recovery = &supervisor.AGMRecovery{}
	}
	mkLoop := func(s supervisor.Supervisor) (*supervisor.Loop, error) {
		return supervisor.NewLoop(supervisor.LoopConfig{
			Supervisor:        s,
			Mesh:              placeholderMesh{},
			Check:             check,
			Recovery:          recovery,
			Trail:             trail,
			Interval:          cfg.interval,
			RecoveryThreshold: cfg.recoveryThreshold,
		})
	}
	metaLoop, err := mkLoop(metaSup)
	if err != nil {
		return nil, err
	}
	orchLoop, err := mkLoop(orchSup)
	if err != nil {
		return nil, err
	}
	overLoop, err := mkLoop(overSup)
	if err != nil {
		return nil, err
	}
	return supervisor.NewMesh(metaLoop, orchLoop, overLoop)
}

// splitTargets parses a comma-separated reap-targets flag into a slice,
// trimming whitespace and dropping empty entries.
func splitTargets(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// runContext returns a context that fires on SIGINT/SIGTERM, on the
// optional duration deadline, or on its returned cancel func.
func runContext(duration time.Duration, stderr io.Writer) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	if duration > 0 {
		ctx, cancel = context.WithTimeout(ctx, duration)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				_, _ = fmt.Fprintf(stderr, "vroom-mesh: panic in signal goroutine: %v\n", r)
			}
		}()
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigCh:
			_, _ = fmt.Fprintln(stderr, "vroom-mesh: shutdown signal received")
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}

	trail, closeTrail, err := openTrail(stdout, cfg.trailPath)
	if err != nil {
		return err
	}
	defer closeTrail()

	roadmap, err := seedRoadmap(cfg.nProposals)
	if err != nil {
		return err
	}
	queue, err := seedQueue(cfg.nTasks)
	if err != nil {
		return err
	}
	var probe supervisor.ResourceProbe
	if cfg.sysProbe {
		probe = supervisor.NewSysResourceProbe()
	} else {
		freeMemBytes := uint64(0)
		if cfg.freeMemMiB > 0 {
			freeMemBytes = uint64(cfg.freeMemMiB) * supervisor.MiB
		}
		mem := supervisor.NewInMemoryResourceProbe()
		mem.Set(supervisor.ResourceSnapshot{
			DiskUsedFraction:        cfg.diskFrac,
			MemoryUsedFraction:      cfg.memFrac,
			CPUUsedFraction:         cfg.cpuFrac,
			SwapUsedFraction:        cfg.swapFrac,
			FreePhysicalMemoryBytes: freeMemBytes,
			StrandedWorktrees:       cfg.stranded,
			OrphanedSessions:        cfg.orphaned,
		})
		probe = mem
	}

	mesh, err := buildMesh(trail, roadmap, queue, probe, cfg)
	if err != nil {
		return err
	}

	ctx, cancel := runContext(cfg.duration, stderr)
	defer cancel()

	if err := mesh.Run(ctx); err != nil && !isCancellation(err) {
		return fmt.Errorf("mesh: %w", err)
	}
	return nil
}

func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
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

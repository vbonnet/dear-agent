// Command agm-aware-reaper is an AGM-aware process reaper. It consults the AGM
// session database for the set of supervised (live) sessions, protects their
// entire process subtrees, and then reaps unsupervised high-RSS / idle
// processes with a graduated SIGTERM→grace→SIGKILL escalation.
//
// It is invoked as a short-lived tool (e.g. by the Overseer on a pressure tick
// via a JSON-emitting shell-out), not run as a daemon. With no --json it prints
// a human summary; with --json it emits a machine-readable ReapResult for the
// supervisor adapter to parse.
//
// Examples:
//
//	agm-aware-reaper --dry-run --max-rss-mb 1500
//	agm-aware-reaper --max-rss-mb 1500 --grace 10s --json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vbonnet/dear-agent/agm/internal/logging"
	"github.com/vbonnet/dear-agent/agm/internal/procreaper"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(argv []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("agm-aware-reaper", flag.ContinueOnError)
	maxRSS := fs.Int64("max-rss-mb", 1500, "only target processes using more than this many MiB of RSS")
	minIdle := fs.Duration("min-idle", 0, "only target processes idle at least this long (0 = ignore idle)")
	grace := fs.Duration("grace", procreaper.DefaultGracePeriod, "SIGTERM→SIGKILL grace window")
	dryRun := fs.Bool("dry-run", false, "compute the kill set but send no signals")
	asJSON := fs.Bool("json", false, "emit the result as JSON")
	agmBin := fs.String("agm-bin", "", "path to the agm binary (default: resolve on PATH)")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	logger := logging.DefaultLogger()

	reaper, err := procreaper.New(
		&procreaper.AGMSessionSupervisor{AGMBin: *agmBin, Logger: logger},
		procreaper.PSProcessLister{},
		procreaper.SignalSignaler{},
		procreaper.SelfPID(),
		procreaper.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	res, err := reaper.Reap(context.Background(), procreaper.ReapRequest{
		MaxRSSMB:    *maxRSS,
		MinIdle:     *minIdle,
		GracePeriod: *grace,
		DryRun:      *dryRun,
	})
	if err != nil {
		return err
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(toJSON(res))
	}

	if res.DryRun {
		fmt.Fprintf(stdout, "agm-aware-reaper (dry-run): %d candidate(s), %d protected, %d would be reaped, %d error(s)\n",
			len(res.Candidates), len(res.Protected), len(res.Reapable), len(res.Errors))
	} else {
		fmt.Fprintf(stdout, "agm-aware-reaper: %d candidate(s), %d protected, %d reaped, %d error(s)\n",
			len(res.Candidates), len(res.Protected), len(res.Terminated), len(res.Errors))
	}
	return nil
}

// reapResultJSON is the stable JSON contract emitted with --json. The
// supervisor adapter (pkg/vroom/supervisor) parses this; keep the field tags in
// sync with reclaim_supervised.go.
type reapResultJSON struct {
	DryRun     bool          `json:"dry_run"`
	Candidates int           `json:"candidates"`
	Protected  int           `json:"protected"`
	Reapable   int           `json:"reapable"`
	Terminated int           `json:"terminated"`
	Errors     []string      `json:"errors,omitempty"`
	Killed     []processJSON `json:"killed,omitempty"`
}

type processJSON struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	RSSKB   int64  `json:"rss_kb"`
}

func toJSON(res procreaper.ReapResult) reapResultJSON {
	out := reapResultJSON{
		DryRun:     res.DryRun,
		Candidates: len(res.Candidates),
		Protected:  len(res.Protected),
		Reapable:   len(res.Reapable),
		Terminated: len(res.Terminated),
		Errors:     res.Errors,
	}
	for _, p := range res.Terminated {
		out.Killed = append(out.Killed, processJSON{PID: p.PID, Command: p.Command, RSSKB: p.RSSKB})
	}
	return out
}

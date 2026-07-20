// Command workflow-status prints the state of one workflow run, reading
// from the SQLite runs.db produced by pkg/workflow's SQLiteState.
//
// Usage:
//
//	workflow-status -db ./runs.db <run-id>
//	workflow-status -db ./runs.db --json <run-id>
//	workflow-status -db ./runs.db --watch 2 <run-id>
//
// Output matches pkg/workflow/SPEC.md. The renderer is intentionally plain
// text.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/internal/sqlite"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		dbPath = flag.String("db", "runs.db", "path to runs.db")
		asJSON = flag.Bool("json", false, "emit machine-readable JSON")
		watch  = flag.Duration("watch", 0, "re-render every N seconds (e.g. 2s); 0 = print once")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <run-id>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		return 2
	}
	runID := flag.Arg(0)

	db, err := sqlite.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		return 1
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	render := func() error {
		st, err := workflow.Status(ctx, db, runID)
		if err != nil {
			return err
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(st)
		}
		fmt.Print(workflow.FormatRunStatusText(st))
		return nil
	}

	if *watch <= 0 {
		if err := render(); err != nil {
			return errCode(err)
		}
		return 0
	}

	t := time.NewTicker(*watch)
	defer t.Stop()
	for {
		fmt.Print("\033[2J\033[H") // clear screen for watch mode
		if err := render(); err != nil {
			return errCode(err)
		}
		select {
		case <-ctx.Done():
			return 0
		case <-t.C:
		}
	}
}

func errCode(err error) int {
	fmt.Fprintln(os.Stderr, err)
	if errors.Is(err, workflow.ErrRunNotFound) {
		return 3
	}
	return 1
}

// Command workflow-logs prints the audit_events stream for one run. The
// stream is the substrate's "what happened" log — every state transition
// shows up here, in occurrence order.
//
// Usage:
//
//	workflow-logs -db ./runs.db <run-id>
//	workflow-logs -db ./runs.db --node researcher <run-id>
//	workflow-logs -db ./runs.db --json <run-id>
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		dbPath = flag.String("db", "runs.db", "path to runs.db")
		nodeID = flag.String("node", "", "filter to one node id (default: all events for the run)")
		limit  = flag.Int("limit", 0, "max events to return (0 = unbounded)")
		asJSON = flag.Bool("json", false, "emit machine-readable JSON")
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

	db, err := sql.Open("sqlite", *dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		return 1
	}
	defer db.Close()

	events, err := workflow.Logs(context.Background(), db, runID, workflow.LogsOptions{
		NodeID: *nodeID,
		Limit:  *limit,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, workflow.ErrRunNotFound) {
			return 3
		}
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(events); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tNODE\tFROM→TO\tACTOR\tREASON")
	for _, ev := range events {
		nodeCol := ev.NodeID
		if nodeCol == "" {
			nodeCol = "<run>"
		}
		from := ev.FromState
		if from == "" {
			from = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s→%s\t%s\t%s\n",
			ev.OccurredAt.Format(time.RFC3339),
			nodeCol, from, ev.ToState, ev.Actor, ev.Reason,
		)
	}
	_ = tw.Flush()
	return 0
}

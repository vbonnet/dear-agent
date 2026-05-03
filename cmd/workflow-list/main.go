// Command workflow-list prints recent workflow runs from the runs.db
// produced by pkg/workflow's SQLiteState. Default ordering is most recent
// first; use --state to filter and --limit to bound.
//
// Usage:
//
//	workflow-list -db ./runs.db
//	workflow-list -db ./runs.db --state running
//	workflow-list -db ./runs.db --json --limit 100
package main

import (
	"context"
	"database/sql"
	"encoding/json"
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
		state  = flag.String("state", "", "filter by state (pending|running|awaiting_hitl|succeeded|failed|cancelled)")
		limit  = flag.Int("limit", 50, "maximum rows to return")
		asJSON = flag.Bool("json", false, "emit machine-readable JSON")
	)
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		return 1
	}
	defer db.Close()

	rows, err := workflow.List(context.Background(), db, workflow.ListOptions{
		State: workflow.RunState(*state),
		Limit: *limit,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "RUN_ID\tWORKFLOW\tSTATE\tSTARTED\tDURATION")
	for _, r := range rows {
		duration := "-"
		if r.FinishedAt != nil {
			duration = r.FinishedAt.Sub(r.StartedAt).Round(time.Millisecond).String()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			r.RunID, r.Workflow, r.State,
			r.StartedAt.Format(time.RFC3339), duration)
	}
	_ = tw.Flush()
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no runs match")
	}
	return 0
}

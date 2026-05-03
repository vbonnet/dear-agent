// Command workflow-cancel marks a workflow run cancelled in runs.db. It
// updates runs.state and emits an audit row; an active in-process runner
// observes cancellation through its context (Phase 0 implements the row
// flip; an in-process broker that signals running runners ships in a
// later phase).
//
// Usage:
//
//	workflow-cancel -db ./runs.db <run-id>
//	workflow-cancel -db ./runs.db --reason "user-aborted" <run-id>
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"

	_ "modernc.org/sqlite"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func main() {
	os.Exit(run())
}

// run does the actual work. Returning from run lets `defer db.Close()`
// fire before the process exits — main() then translates the result code.
func run() int {
	var (
		dbPath = flag.String("db", "runs.db", "path to runs.db")
		reason = flag.String("reason", "cancelled-via-cli", "free-form reason recorded in audit_events")
		actor  = flag.String("actor", "", "actor string (default: \"human:<your-username>\")")
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

	if *actor == "" {
		u, err := user.Current()
		if err == nil {
			*actor = "human:" + u.Username
		} else {
			*actor = "human"
		}
	}

	db, err := sql.Open("sqlite", *dbPath+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", *dbPath, err)
		return 1
	}
	defer db.Close()

	if err := workflow.Cancel(context.Background(), db, runID, *reason, *actor); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, workflow.ErrRunNotFound) {
			return 3
		}
		return 1
	}
	fmt.Printf("cancelled run %s\n", runID)
	return 0
}

// Command workflow-approve records a human approval for an awaiting_hitl
// node. Acts as the canonical write path for any UI/backend that wants the
// CLI's audit trail without writing its own SQL.
//
// The same binary handles `reject` and `list` via subcommands so the
// workflow_engine ships one HITL CLI rather than three. Subcommands:
//
//	workflow-approve list              # show pending requests
//	workflow-approve approve <id>      # record decision = approve
//	workflow-approve reject  <id>      # record decision = reject
//
// In all cases --db points at the runs.db, --as <role> records the
// approver_role (and is matched against the row's required role), --reason
// is free-form audit text, and --actor overrides the default "human:<user>"
// actor string.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		printUsage()
		return 2
	}
	switch os.Args[1] {
	case "list":
		return runList(os.Args[2:])
	case "approve":
		return runDecision(os.Args[2:], workflow.HITLDecisionApprove)
	case "reject":
		return runDecision(os.Args[2:], workflow.HITLDecisionReject)
	case "-h", "--help", "help":
		printUsage()
		return 0
	}
	printUsage()
	return 2
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage:
  %s list   [--db PATH] [--json]
  %s approve <approval-id> [--db PATH] [--as ROLE] [--reason TEXT] [--actor STRING]
  %s reject  <approval-id> [--db PATH] [--as ROLE] [--reason TEXT] [--actor STRING]
`, os.Args[0], os.Args[0], os.Args[0])
}

func runList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	dbPath := fs.String("db", "runs.db", "path to runs.db")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	db, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()
	pending, err := workflow.ListPendingHITLRequests(context.Background(), db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(pending)
		return 0
	}
	if len(pending) == 0 {
		fmt.Println("(no pending approvals)")
		return 0
	}
	for _, p := range pending {
		fmt.Printf("%s  run=%s node=%s role=%s requested_at=%s\n  reason: %s\n",
			p.ApprovalID, p.RunID, p.NodeID, p.ApproverRole,
			p.RequestedAt.Format(time.RFC3339), p.Reason)
	}
	return 0
}

func runDecision(args []string, dec workflow.HITLDecision) int {
	fs := flag.NewFlagSet(string(dec), flag.ContinueOnError)
	dbPath := fs.String("db", "runs.db", "path to runs.db")
	role := fs.String("as", "", "approver role; required-role rows reject mismatching values")
	reason := fs.String("reason", "", "free-form audit reason")
	actor := fs.String("actor", "", "override actor string (default: human:<your-username>)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		printUsage()
		return 2
	}
	approvalID := fs.Arg(0)

	if *actor == "" {
		u, err := user.Current()
		if err == nil {
			*actor = u.Username
		} else {
			*actor = "human"
		}
	}
	// Strip a leading "human:" so the actor field stored in approvals is
	// the raw name; RecordHITLDecision adds the "human:" prefix on its
	// audit row.
	approver := strings.TrimPrefix(*actor, "human:")

	db, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()

	if err := workflow.RecordHITLDecision(context.Background(), db, approvalID, dec, approver, *role, *reason, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		switch {
		case errors.Is(err, workflow.ErrApprovalNotFound):
			return 3
		case errors.Is(err, workflow.ErrApprovalAlreadyResolved):
			return 4
		case errors.Is(err, workflow.ErrApproverRoleMismatch):
			return 5
		}
		return 1
	}
	fmt.Printf("recorded decision %s for %s\n", dec, approvalID)
	return 0
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}

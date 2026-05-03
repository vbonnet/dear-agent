// Command workflow-audit runs DEAR Audit checks against the current
// repository, queries findings, and manages their lifecycle.
//
// Subcommands:
//
//	workflow-audit run    [--cadence daily|weekly|monthly] [--db PATH] [--dry-run]
//	workflow-audit list   [--state open|all] [--severity P0..P3] [--check ID]
//	workflow-audit show   <finding-id>
//	workflow-audit ack    <finding-id> [--note "..."]
//	workflow-audit resolve <finding-id> [--note "..."]
//
// The defaults read .dear-agent.yml from the current working
// directory; --db defaults to ./.dear-agent/audit.db. The CLI is
// thin — all logic lives in pkg/audit and pkg/audit/config.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/audit"
	auditconfig "github.com/vbonnet/dear-agent/pkg/audit/config"

	// Side-effect import: registers the built-in checks and refiners
	// with audit.Default at process startup.
	_ "github.com/vbonnet/dear-agent/pkg/audit/checks"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable entry point. Returns the process exit code.
func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "run":
		return runAudit(rest, stdout, stderr)
	case "list":
		return runList(rest, stdout, stderr)
	case "show":
		return runShow(rest, stdout, stderr)
	case "ack":
		return runAck(rest, stdout, stderr)
	case "resolve":
		return runResolve(rest, stdout, stderr)
	case "-h", "--help", "help":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "workflow-audit: unknown subcommand %q\n\n", sub)
		usage(stderr)
		return 2
	}
}

func usage(w *os.File) {
	fmt.Fprintln(w, "Usage: workflow-audit <subcommand> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  run     execute the configured audits for a cadence")
	fmt.Fprintln(w, "  list    list findings (filterable)")
	fmt.Fprintln(w, "  show    print one finding in full")
	fmt.Fprintln(w, "  ack     mark a finding as acknowledged")
	fmt.Fprintln(w, "  resolve mark a finding as resolved")
}

// commonStoreFlags wires the --db flag every subcommand needs.
type commonStoreFlags struct {
	dbPath string
}

func (c *commonStoreFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&c.dbPath, "db", defaultDBPath(), "path to audit database")
}

// defaultDBPath returns the conventional database location relative
// to the current working directory.
func defaultDBPath() string {
	return filepath.Join(".dear-agent", "audit.db")
}

func openStore(stderr *os.File, path string) (*audit.SQLiteStore, bool) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(stderr, "workflow-audit: create db dir %s: %v\n", dir, err)
			return nil, false
		}
	}
	store, err := audit.OpenSQLiteStore(path)
	if err != nil {
		fmt.Fprintf(stderr, "workflow-audit: open store %s: %v\n", path, err)
		return nil, false
	}
	return store, true
}

func runAudit(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var common commonStoreFlags
	common.bind(fs)
	cadence := fs.String("cadence", "daily", "cadence to run: daily|weekly|monthly|on-demand")
	dryRun := fs.Bool("dry-run", false, "execute checks but skip remediation side effects")
	repoRoot := fs.String("repo", ".", "repository root (defaults to cwd)")
	verbose := fs.Bool("verbose", false, "debug logging")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level}))

	root, err := filepath.Abs(*repoRoot)
	if err != nil {
		fmt.Fprintf(stderr, "workflow-audit: abs %s: %v\n", *repoRoot, err)
		return 1
	}

	cfg, err := auditconfig.Load(root)
	if err != nil {
		fmt.Fprintf(stderr, "workflow-audit: load config: %v\n", err)
		return 1
	}

	plan, err := auditconfig.BuildPlan(cfg, root, audit.Cadence(*cadence), audit.Default, "cli")
	if err != nil {
		fmt.Fprintf(stderr, "workflow-audit: build plan: %v\n", err)
		return 1
	}
	plan.DryRun = *dryRun

	store, ok := openStore(stderr, common.dbPath)
	if !ok {
		return 1
	}
	defer func() { _ = store.Close() }()

	runner := audit.NewRunner()
	runner.Store = store
	runner.Logger = logger

	report, err := runner.Run(context.Background(), plan)
	if err != nil {
		fmt.Fprintf(stderr, "workflow-audit: run failed: %v\n", err)
		if report != nil {
			printReport(stdout, report)
		}
		return 1
	}
	printReport(stdout, report)
	if report.AuditRun.State == audit.AuditRunFailed {
		return 1
	}
	return 0
}

func printReport(w *os.File, r *audit.RunReport) {
	fmt.Fprintf(w, "audit run %s state=%s repo=%s cadence=%s findings_open=%d new=%d resolved=%d\n",
		r.AuditRun.AuditRunID, r.AuditRun.State, r.AuditRun.Repo, r.AuditRun.Cadence,
		r.AuditRun.FindingsOpen, r.AuditRun.FindingsNew, r.AuditRun.FindingsResolved)
	for _, oc := range r.CheckOutcomes {
		errStr := ""
		if oc.Err != nil {
			errStr = " err=" + oc.Err.Error()
		}
		fmt.Fprintf(w, "  - check=%s tree=%s status=%s findings=%d duration=%s%s\n",
			oc.CheckID, oc.WorkingDir, oc.Result.Status, len(oc.Findings), oc.Result.Duration, errStr)
		for _, f := range oc.Findings {
			fmt.Fprintf(w, "      %s %s %s\n", f.Severity, f.FindingID, f.Title)
		}
	}
	if len(r.Proposals) > 0 {
		fmt.Fprintf(w, "  proposals (%d):\n", len(r.Proposals))
		for _, p := range r.Proposals {
			fmt.Fprintf(w, "    - %s %s\n", p.Layer, p.Title)
		}
	}
}

func runList(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	var common commonStoreFlags
	common.bind(fs)
	state := fs.String("state", "open", "filter by state: open|acknowledged|resolved|reopened|all")
	severity := fs.String("severity", "", "filter by severity: P0|P1|P2|P3")
	check := fs.String("check", "", "filter by check id")
	repo := fs.String("repo", "", "filter by repo (default: any)")
	limit := fs.Int("limit", 50, "max rows")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	store, ok := openStore(stderr, common.dbPath)
	if !ok {
		return 1
	}
	defer func() { _ = store.Close() }()

	filter := audit.FindingFilter{Repo: *repo, CheckID: *check, Limit: *limit}
	if *state != "all" {
		filter.State = audit.FindingState(*state)
		if !filter.State.IsValid() {
			fmt.Fprintf(stderr, "workflow-audit: invalid state %q\n", *state)
			return 2
		}
	}
	if *severity != "" {
		filter.Severity = audit.Severity(*severity)
		if !filter.Severity.IsValid() {
			fmt.Fprintf(stderr, "workflow-audit: invalid severity %q\n", *severity)
			return 2
		}
	}

	findings, err := store.ListFindings(context.Background(), filter)
	if err != nil {
		fmt.Fprintf(stderr, "workflow-audit: list: %v\n", err)
		return 1
	}
	if len(findings) == 0 {
		fmt.Fprintln(stdout, "no findings")
		return 0
	}
	fmt.Fprintf(stdout, "%-4s  %-36s  %-22s  %s\n", "SEV", "ID", "CHECK", "TITLE")
	for _, f := range findings {
		fmt.Fprintf(stdout, "%-4s  %-36s  %-22s  %s\n", f.Severity, f.FindingID, f.CheckID, truncateTitle(f.Title, 80))
	}
	return 0
}

func truncateTitle(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func runShow(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	var common commonStoreFlags
	common.bind(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "workflow-audit: show <finding-id>")
		return 2
	}
	store, ok := openStore(stderr, common.dbPath)
	if !ok {
		return 1
	}
	defer func() { _ = store.Close() }()

	f, err := store.GetFinding(context.Background(), fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "workflow-audit: show: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "id:          %s\n", f.FindingID)
	fmt.Fprintf(stdout, "repo:        %s\n", f.Repo)
	fmt.Fprintf(stdout, "check:       %s\n", f.CheckID)
	fmt.Fprintf(stdout, "severity:    %s\n", f.Severity)
	fmt.Fprintf(stdout, "state:       %s\n", f.State)
	fmt.Fprintf(stdout, "first_seen:  %s\n", f.FirstSeen.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(stdout, "last_seen:   %s\n", f.LastSeen.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(stdout, "title:       %s\n", f.Title)
	fmt.Fprintln(stdout, "detail:")
	for _, ln := range strings.Split(f.Detail, "\n") {
		fmt.Fprintf(stdout, "  %s\n", ln)
	}
	if f.Suggested.Strategy != audit.StrategyUnspecified {
		fmt.Fprintf(stdout, "suggested:   strategy=%s command=%q\n", f.Suggested.Strategy, f.Suggested.Command)
	}
	return 0
}

func runAck(args []string, stdout, stderr *os.File) int {
	return runStateChange("ack", audit.FindingAcknowledged, args, stdout, stderr)
}

func runResolve(args []string, stdout, stderr *os.File) int {
	return runStateChange("resolve", audit.FindingResolved, args, stdout, stderr)
}

func runStateChange(name string, target audit.FindingState, args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	var common commonStoreFlags
	common.bind(fs)
	note := fs.String("note", "", "free-form note")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "workflow-audit: %s <finding-id>\n", name)
		return 2
	}
	store, ok := openStore(stderr, common.dbPath)
	if !ok {
		return 1
	}
	defer func() { _ = store.Close() }()

	f, err := store.SetFindingState(context.Background(), fs.Arg(0), target, *note)
	if err != nil {
		fmt.Fprintf(stderr, "workflow-audit: %s: %v\n", name, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s %s state=%s\n", name, f.FindingID, f.State)
	return 0
}

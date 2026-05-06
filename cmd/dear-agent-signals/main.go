// Command dear-agent-signals collects and reports project-health
// signals from the SQLite store described in ADR-015.
//
// Usage:
//
//	dear-agent-signals collect [--db PATH] [--repo PATH]
//	                           [--coverage-file PATH]
//	                           [--lint-file PATH] [--security-file PATH]
//	                           [--lookback-days N]
//	dear-agent-signals report [--db PATH] [--kind KIND]
//	                          [--since DURATION] [--limit N] [--json]
//	                          [--score]
//	dear-agent-signals salience [--input PATH] [--window DURATION]
//	                            [--capacity N] [--bypass TIER]
//	                            [--json] [--keep-noise]
package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:]))
}

func run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "collect":
		return runCollect(ctx, args[1:])
	case "report":
		return runReport(ctx, args[1:])
	case "salience":
		return runSalience(ctx, args[1:])
	case "-h", "--help", "help":
		usage(os.Stdout)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

func usage(w *os.File) {
	fmt.Fprintf(w, `dear-agent-signals — collect and report project-health signals (ADR-015)

Usage:
  dear-agent-signals collect  [flags]
  dear-agent-signals report   [flags]
  dear-agent-signals salience [flags]

Run 'dear-agent-signals <subcommand> -h' for subcommand-specific flags.
`)
}

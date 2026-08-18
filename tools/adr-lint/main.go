// Command adr-lint validates the Git-tracked ADR corpus declared in
// .dear-agent.yml.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/pkg/adrlint"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("adr-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", ".", "repository root")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "adr-lint: unexpected arguments: %v\n", flags.Args())
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	report, err := adrlint.CheckRepository(ctx, *repo)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if !report.Blocking() {
		fmt.Fprintf(stdout, "ok: %d governed ADR record(s), identity/index/lifecycle contract intact\n", report.Records)
		return 0
	}
	fmt.Fprintf(stderr, "adr-lint: %d violation(s) across %d governed record(s):\n", len(report.Violations), report.Records)
	for _, violation := range report.Violations {
		fmt.Fprintf(stderr, "  %s: %s\n", violation.Path, violation.Reason)
	}
	return 1
}

// Command instruction-lint validates active AI instruction policy guidance.
// Content violations exit 1; usage and operational errors exit 2.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/pkg/instructionlint"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("instruction-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repository := flags.String("repo", "", "validate one Git repository")
	baselineRef := flags.String("baseline-ref", "", "reject exclusion growth relative to this Git commit or ref")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: instruction-lint -repo <root>")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *repository == "" || flags.NArg() != 0 {
		flags.Usage()
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, violations, err := instructionlint.CheckRepository(ctx, *repository)
	if err != nil {
		fmt.Fprintln(stderr, "instruction-lint:", err)
		return 2
	}
	if *baselineRef != "" {
		ratchetViolations, ratchetErr := instructionlint.CheckExclusionRatchet(ctx, *repository, *baselineRef)
		if ratchetErr != nil {
			fmt.Fprintln(stderr, "instruction-lint:", ratchetErr)
			return 2
		}
		violations = append(violations, ratchetViolations...)
	}
	if len(violations) > 0 {
		for _, violation := range violations {
			fmt.Fprintln(stderr, violation)
		}
		fmt.Fprintf(stderr, "\n%d violation(s)\n", len(violations))
		return 1
	}
	fmt.Fprintf(stdout, "ok: %d active instruction file(s), %d exact exclusion(s), policy guidance intact\n", result.Files, result.Exclusions)
	return 0
}

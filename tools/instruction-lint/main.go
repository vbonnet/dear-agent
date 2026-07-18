// Command instruction-lint validates active AI instruction policy guidance.
// Content violations exit 1; usage and operational errors exit 2.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vbonnet/dear-agent/pkg/instructionlint"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("instruction-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repository := flags.String("repo", "", "validate one Git repository")
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

	result, violations, err := instructionlint.CheckRepository(*repository)
	if err != nil {
		fmt.Fprintln(stderr, "instruction-lint:", err)
		return 2
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

// Command header-lint validates Markdown documents for the single-line bold
// metadata "header block" anti-pattern. Content violations exit 1; usage and
// operational errors exit 2.
//
// Usage:
//
//	header-lint -repo <root>
//	header-lint <dir> [<dir>...]
//	header-lint -file <path>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/vbonnet/dear-agent/pkg/headerlint"
)

func main() {
	os.Exit(mainExitCode())
}

func mainExitCode() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stderr)
}

func run(ctx context.Context, args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("header-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var singleFile, repository string
	flags.StringVar(&singleFile, "file", "", "lint a single Markdown file")
	flags.StringVar(&repository, "repo", "", "lint every tracked Markdown file in a Git repository")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: header-lint -repo <root>")
		fmt.Fprintln(stderr, "       header-lint <dir> [<dir>...]")
		fmt.Fprintln(stderr, "       header-lint -file <path>")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	dirs := flags.Args()
	selectedModes := 0
	if repository != "" {
		selectedModes++
	}
	if singleFile != "" {
		selectedModes++
	}
	if len(dirs) > 0 {
		selectedModes++
	}
	if selectedModes != 1 {
		flags.Usage()
		return 2
	}

	var (
		violations []headerlint.Violation
		err        error
	)
	switch {
	case repository != "":
		violations, err = headerlint.CheckRepository(ctx, repository)
	case singleFile != "":
		violations, err = headerlint.CheckFile(singleFile)
	default:
		for _, dir := range dirs {
			var found []headerlint.Violation
			found, err = headerlint.CheckDir(dir)
			if err != nil {
				break
			}
			violations = append(violations, found...)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "header-lint:", err)
		return 2
	}
	if len(violations) == 0 {
		return 0
	}
	for _, violation := range violations {
		fmt.Fprintln(stderr, violation)
	}
	fmt.Fprintf(stderr, "\n%d violation(s)\n", len(violations))
	fmt.Fprintln(stderr, "\nSee docs/doc-header-format.md for the canonical replacement format.")
	return 1
}

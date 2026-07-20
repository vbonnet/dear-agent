// Command skill-lint validates AI skill and command Markdown surfaces.
// Content violations exit 1; usage and operational errors exit 2.
//
// Usage:
//
//	skill-lint -repo <root>
//	skill-lint <dir> [<dir>...]
//	skill-lint -file <path>
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

	"github.com/vbonnet/dear-agent/pkg/skilllint"
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
	flags := flag.NewFlagSet("skill-lint", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var singleFile, repository string
	flags.StringVar(&singleFile, "file", "", "lint a single recognized surface")
	flags.StringVar(&repository, "repo", "", "lint every tracked surface in a Git repository")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: skill-lint -repo <root>")
		fmt.Fprintln(stderr, "       skill-lint <dir> [<dir>...]")
		fmt.Fprintln(stderr, "       skill-lint -file <path>")
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
		violations []skilllint.Violation
		err        error
	)
	switch {
	case repository != "":
		violations, err = skilllint.CheckRepository(ctx, repository)
	case singleFile != "":
		violations, err = skilllint.CheckFile(singleFile)
	default:
		for _, dir := range dirs {
			var found []skilllint.Violation
			found, err = skilllint.CheckDir(dir)
			if err != nil {
				break
			}
			violations = append(violations, found...)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "skill-lint:", err)
		return 2
	}
	if len(violations) == 0 {
		return 0
	}
	for _, violation := range violations {
		fmt.Fprintln(stderr, violation)
	}
	fmt.Fprintf(stderr, "\n%d violation(s)\n", len(violations))
	return 1
}

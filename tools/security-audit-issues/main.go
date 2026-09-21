// Command security-audit-issues reconciles the Security Audit workflow's
// rolling GitHub issue.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("security-audit-issues", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repository := flags.String("repo", "", "GitHub repository as owner/repository (default: GITHUB_REPOSITORY)")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: security-audit-issues [-repo owner/repository]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	if *repository == "" {
		*repository = getenv("GITHUB_REPOSITORY")
	}
	if err := validateRepository(*repository); err != nil {
		fmt.Fprintln(stderr, "security-audit-issues:", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := reconcile(ctx, commandRunner{}, *repository, findingsFromEnvironment(getenv), time.Now())
	if err != nil {
		fmt.Fprintln(stderr, "security-audit-issues:", err)
		return 1
	}
	fmt.Fprintln(stdout, result.summary())
	return 0
}

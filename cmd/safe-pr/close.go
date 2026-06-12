package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func runClose(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("safe-pr close", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		repo = fs.String("repo", "", "GitHub repo owner/name (auto-detected if omitted)")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: safe-pr close <number> [--repo owner/name]\n\nflags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(stderr, "error: PR number is required")
		fs.Usage()
		return 2
	}
	prNum := fs.Arg(0)

	if *repo == "" {
		detected, err := detectRepo()
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot detect GitHub repo: %v\n", err)
			return 1
		}
		*repo = detected
	}

	cmdArgs := []string{"pr", "close", prNum, "--repo", *repo}
	cmd := exec.Command("gh", cmdArgs...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "error running gh pr close: %v\n", err)
		return 1
	}
	return 0
}

func runStatus(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("safe-pr status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		repo  = fs.String("repo", "", "GitHub repo owner/name (auto-detected if omitted)")
		prCap = fs.Int("cap", defaultCap, "open-PR cap to check against")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: safe-pr status [--repo owner/name] [--cap N]\n\nflags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *repo == "" {
		detected, err := detectRepo()
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot detect GitHub repo: %v\n", err)
			return 1
		}
		*repo = detected
	}

	prs, err := listOpenPRs(*repo)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	capStatus := "ok"
	if len(prs) >= *prCap {
		capStatus = "OVER CAP"
	}
	fmt.Fprintf(stdout, "open PRs: %d / %d cap [%s]\n", len(prs), *prCap, capStatus)
	for _, pr := range prs {
		fmt.Fprintf(stdout, "  #%d — %s [%s]\n", pr.Number, pr.Title, pr.HeadRefName)
	}
	if len(prs) >= *prCap {
		return 2
	}
	return 0
}

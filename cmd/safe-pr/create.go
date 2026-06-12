package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func runCreate(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("safe-pr create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		beadID = fs.String("bead", "", "bead ID this PR resolves (enables dedup check)")
		repo   = fs.String("repo", "", "GitHub repo owner/name (auto-detected if omitted)")
		prCap  = fs.Int("cap", defaultCap, "maximum allowed open PRs before creation is blocked")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: safe-pr create [--bead <id>] [--repo owner/name] [--cap N] [-- <gh pr create args...>]\n\nflags:\n")
		fs.PrintDefaults()
	}
	// Stop at "--" and pass everything after it directly to gh pr create.
	var ghArgs []string
	for i, a := range args {
		if a == "--" {
			ghArgs = args[i+1:]
			args = args[:i]
			break
		}
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ghArgs = append(fs.Args(), ghArgs...)

	if *repo == "" {
		detected, err := detectRepo()
		if err != nil {
			fmt.Fprintf(stderr, "error: cannot detect GitHub repo: %v\n", err)
			fmt.Fprintf(stderr, "hint: pass --repo owner/name\n")
			return 1
		}
		*repo = detected
	}

	// Fetch open PRs once; both guards use the same list.
	prs, err := listOpenPRs(*repo)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if code := enforceCapGuard(prs, *prCap, stderr); code != 0 {
		return code
	}
	if code := enforceBeadDedupGuard(prs, *beadID, stderr); code != 0 {
		return code
	}

	return execGhCreate(buildGhCreateArgs(*repo, ghArgs), stdout, stderr)
}

func enforceCapGuard(prs []openPR, prCap int, stderr *os.File) int {
	if len(prs) < prCap {
		return 0
	}
	fmt.Fprintf(stderr, "BLOCKED: %d open PRs at or above cap %d.\n", len(prs), prCap)
	fmt.Fprintf(stderr, "Merge or close some PRs before opening more:\n")
	for _, pr := range prs {
		fmt.Fprintf(stderr, "  #%d — %s\n", pr.Number, pr.Title)
	}
	fmt.Fprintf(stderr, "\nRaise the cap with --cap if intentional.\n")
	return 2
}

func enforceBeadDedupGuard(prs []openPR, beadID string, stderr *os.File) int {
	if beadID == "" {
		return 0
	}
	claims := checkBeadDedup(prs, beadID)
	if len(claims) == 0 {
		return 0
	}
	fmt.Fprintf(stderr, "BLOCKED: bead %s already claimed by %d open PR(s):\n", beadID, len(claims))
	for _, c := range claims {
		fmt.Fprintf(stderr, "  #%d — %s [branch: %s]\n", c.Number, c.Title, c.HeadRefName)
	}
	fmt.Fprintf(stderr, "\nAdopt the existing PR instead of creating a new one.\n")
	fmt.Fprintf(stderr, "    gh pr view %d\n", claims[0].Number)
	return 2
}

func buildGhCreateArgs(repo string, ghArgs []string) []string {
	cmdArgs := append([]string{"pr", "create"}, ghArgs...)
	if repo == "" {
		return cmdArgs
	}
	for i, a := range ghArgs {
		if a == "--repo" || (i > 0 && ghArgs[i-1] == "--repo") {
			return cmdArgs // already present
		}
	}
	return append(cmdArgs, "--repo", repo)
}

func execGhCreate(cmdArgs []string, stdout, stderr *os.File) int {
	cmd := exec.Command("gh", cmdArgs...) //nolint:gosec // args are controlled by safe-pr create's flag parser + buildGhCreateArgs
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "error running gh pr create: %v\n", err)
		return 1
	}
	return 0
}

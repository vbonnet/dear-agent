// chezmoi-deploy atomically applies chezmoi changes, commits them to the
// dotfiles source repo, and pushes — so the source and the deployed home
// directory never drift apart.
//
// Usage:
//
//	chezmoi-deploy [-m "commit message"] [--dry-run] [target-paths...]
//
// Steps (each gated on the previous succeeding):
//
//  1. chezmoi apply [target-paths...]   — deploy source state to $HOME
//  2. git -C <source> add -A && commit  — record the source changes
//  3. git -C <source> push              — publish (safe push, never --force)
//
// With --dry-run nothing is applied, committed, or pushed: it shows the
// chezmoi diff and the source repo's pending git status instead.
//
// Force-push is hardcoded off. If any step fails the pipeline stops and reports
// exactly what succeeded, what failed, and how to recover.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nchezmoi-deploy: %v\n", err)
		os.Exit(1)
	}
}

// options holds the parsed command line.
type options struct {
	message string
	dryRun  bool
	help    bool
	targets []string
}

// parseArgs turns argv into options, returning an error for malformed or
// refused flags. The --help case sets help so the caller can print usage.
func parseArgs(argv []string) (options, error) {
	var o options
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "-m" || arg == "--message":
			if i+1 >= len(argv) {
				return o, fmt.Errorf("%s requires a commit message argument", arg)
			}
			// Read the value before advancing; the guard above proves
			// i+1 is in range, which also keeps gosec's bounds analysis happy.
			o.message = argv[i+1]
			i++
		case strings.HasPrefix(arg, "-m="):
			o.message = strings.TrimPrefix(arg, "-m=")
		case strings.HasPrefix(arg, "--message="):
			o.message = strings.TrimPrefix(arg, "--message=")
		case arg == "--dry-run" || arg == "-n":
			o.dryRun = true
		case arg == "--force" || arg == "-f" || arg == "--force-with-lease":
			return o, fmt.Errorf("refusing %s: chezmoi-deploy only does safe pushes, "+
				"force-push would clobber the remote dotfiles history", arg)
		case arg == "-h" || arg == "--help":
			o.help = true
			return o, nil
		case strings.HasPrefix(arg, "-"):
			return o, fmt.Errorf("unknown flag %q (see --help)", arg)
		default:
			o.targets = append(o.targets, arg)
		}
	}
	return o, nil
}

func run(argv []string) error {
	opts, err := parseArgs(argv)
	if err != nil {
		return err
	}
	if opts.help {
		fmt.Print(usage)
		return nil
	}
	message, dryRun, targets := opts.message, opts.dryRun, opts.targets

	source, err := capture("chezmoi", "source-path")
	if err != nil {
		return fmt.Errorf("could not locate chezmoi source (chezmoi source-path failed): %w", err)
	}
	source = strings.TrimSpace(source)
	fmt.Printf("→ chezmoi source: %s\n", source)

	if dryRun {
		return dryRunPreview(source, targets)
	}

	// Step 1: apply.
	applyArgs := append([]string{"apply", "--verbose"}, targets...)
	fmt.Printf("\n[1/3] chezmoi %s\n", strings.Join(applyArgs, " "))
	if err := stream("chezmoi", applyArgs...); err != nil {
		return fmt.Errorf("chezmoi apply failed (nothing committed or pushed): %w", err)
	}

	// Step 2: stage + commit the source changes.
	fmt.Printf("\n[2/3] git -C %s add -A && commit\n", source)
	if err := stream("git", "-C", source, "add", "-A"); err != nil {
		return fmt.Errorf("git add failed in %s (apply succeeded; source NOT committed): %w", source, err)
	}

	staged, err := capture("git", "-C", source, "diff", "--cached", "--name-only")
	if err != nil {
		return fmt.Errorf("could not inspect staged changes in %s: %w", source, err)
	}
	files := nonEmptyLines(staged)
	if len(files) == 0 {
		fmt.Println("    nothing to commit — source matches the last commit.")
		fmt.Println("    apply succeeded; no commit or push needed. ✓")
		return nil
	}

	if message == "" {
		message = autoMessage(files)
	}
	fmt.Printf("    committing %d file(s): %s\n", len(files), strings.Join(files, ", "))
	if err := stream("git", "-C", source, "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit failed in %s (apply succeeded; changes staged but NOT committed; "+
			"inspect with: git -C %s status): %w", source, source, err)
	}

	// Step 3: safe push (never force).
	fmt.Printf("\n[3/3] git -C %s push\n", source)
	if err := stream("git", "-C", source,
		"-c", "credential.helper=!gh auth git-credential", "push"); err != nil {
		return fmt.Errorf("git push failed (apply + commit succeeded; the commit is local only — "+
			"retry the push with: git -C %s -c credential.helper='!gh auth git-credential' push): %w",
			source, err)
	}

	fmt.Println("\n✓ applied, committed, and pushed.")
	return nil
}

func dryRunPreview(source string, targets []string) error {
	diffArgs := append([]string{"diff"}, targets...)
	fmt.Printf("\n[dry-run] chezmoi %s\n", strings.Join(diffArgs, " "))
	if err := stream("chezmoi", diffArgs...); err != nil {
		return fmt.Errorf("chezmoi diff failed: %w", err)
	}
	fmt.Printf("\n[dry-run] pending source changes — git -C %s status --short\n", source)
	if err := stream("git", "-C", source, "status", "--short"); err != nil {
		return fmt.Errorf("git status failed in %s: %w", source, err)
	}
	fmt.Println("\n[dry-run] nothing applied, committed, or pushed.")
	return nil
}

// autoMessage builds a commit message from the staged file list.
func autoMessage(files []string) string {
	const maxNamed = 3
	if len(files) <= maxNamed {
		return "chezmoi-deploy: update " + strings.Join(files, ", ")
	}
	return fmt.Sprintf("chezmoi-deploy: update %s and %d more",
		strings.Join(files[:maxNamed], ", "), len(files)-maxNamed)
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// stream runs a command, forwarding stdout/stderr to the user in real time.
func stream(name string, args ...string) error {
	// #nosec G702 -- command names are compile-time literals ("chezmoi",
	// "git") at every call site, and exec.Command runs no shell, so the
	// user-supplied target paths cannot inject commands or shell metachars.
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// capture runs a command and returns its stdout.
func capture(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return string(out), err
}

const usage = `chezmoi-deploy — atomically apply, commit, and push dotfiles changes.

Usage:
  chezmoi-deploy [-m "commit message"] [--dry-run] [target-paths...]

Flags:
  -m, --message MSG   commit message (auto-generated from changed files if omitted)
  -n, --dry-run       show chezmoi diff + pending git status; apply nothing
  -h, --help          show this help

Examples:
  chezmoi-deploy
  chezmoi-deploy -m "update gitconfig" ~/.gitconfig
  chezmoi-deploy --dry-run

Pushes are always safe (--force is rejected).
`

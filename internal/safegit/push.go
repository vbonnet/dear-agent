// Package safegit builds and runs git pushes that cannot hang on the macOS
// keychain credential helper, and only force-push non-protected branches.
//
// # The hang it prevents
//
// On this host the git credential helper chain is, for any host not explicitly
// remapped, the *generic* chain from the system gitconfig:
//
//	credential.helper=osxkeychain   (system: /opt/homebrew/etc/gitconfig)
//	credential.helper=cache         (global: ~/.gitconfig)
//
// git queries them in order, so osxkeychain runs first. When the keychain
// item's access-control list does not pre-authorize the invoking git binary
// (which happens routinely after a Homebrew git upgrade relocates/re-signs the
// binary), macOS pops a GUI authorization dialog. In a headless agent session
// there is no one to click it, so the push blocks forever.
//
// github.com pushes "usually" work because ~/.gitconfig resets the helper list
// for https://github.com to the GitHub CLI helper — but that reset is scoped to
// two exact hosts. Any other credential context (a mirror remote, an embedded-
// credential URL, a submodule, GitHub Enterprise) falls back to the generic
// chain and hangs. The widely-pasted workaround
//
//	git -c credential.helper='!gh auth git-credential' push
//
// only *appends* the CLI helper; it never resets osxkeychain off the front of
// the chain, so it inherits the same hang for every non-github.com context.
//
// # The fix
//
// CredentialResetArgs emits an empty `-c credential.helper=` (which clears the
// entire accumulated helper list, osxkeychain included) followed by a single
// gh-only helper. Because command-line `-c` is read last and the empty value
// resets the whole list, the push uses *only* the GitHub CLI helper for every
// host — osxkeychain is never invoked, so the GUI dialog can never fire.
package safegit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout bounds a push so a wedged credential helper (or a network
// stall) fails fast with a clear message instead of blocking indefinitely.
const DefaultTimeout = 30 * time.Second

// ResolveGh returns the absolute path to the GitHub CLI. The credential helper
// string is run by git via `/bin/sh -c`, so an absolute path keeps the helper
// working regardless of the caller's PATH.
func ResolveGh() (string, error) {
	path, err := exec.LookPath("gh")
	if err != nil {
		return "", fmt.Errorf("GitHub CLI (gh) not found on PATH; safe-push needs it "+
			"to authenticate without the keychain helper — install it (brew install gh) "+
			"and run `gh auth login`: %w", err)
	}
	return path, nil
}

// CredentialResetArgs returns the `-c` flags that force git to use ONLY the
// GitHub CLI credential helper for this invocation.
//
// The empty `credential.helper=` clears every helper accumulated from system,
// global, and URL-scoped config (notably osxkeychain); the second entry then
// registers gh as the sole helper. The result applies to all hosts, so no
// credential context can fall through to the osxkeychain GUI prompt.
func CredentialResetArgs(ghPath string) []string {
	return []string{
		"-c", "credential.helper=",
		"-c", "credential.helper=!" + ghPath + " auth git-credential",
	}
}

// ForceFlag reports the first force-push flag or force refspec in args, if any.
//
// Detected forms:
//   - -f / --force / --force-with-lease / --force-if-includes (flags)
//   - --mirror (rewrites remote refs wholesale, equivalent to force-push)
//   - +<refspec>  (a leading '+' in a refspec means "force this ref")
func ForceFlag(args []string) (string, bool) {
	for _, a := range args {
		switch {
		case a == "-f" || a == "--force" || a == "--force-with-lease" || a == "--mirror":
			return a, true
		case strings.HasPrefix(a, "--force-with-lease=") ||
			strings.HasPrefix(a, "--force-if-includes"):
			return a, true
		case strings.HasPrefix(a, "+") && !strings.HasPrefix(a, "--"):
			// A '+' prefix on a refspec forces that ref; block it.
			return a, true
		}
	}
	return "", false
}

// ProtectedBranches returns branch names that must not be force-pushed. main
// and master are always protected; origin/HEAD is added when available.
func ProtectedBranches(repoDir string) map[string]bool {
	protected := map[string]bool{"main": true, "master": true}
	if repoDir == "" {
		repoDir = "."
	}
	cmd := exec.Command("git", "-C", repoDir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	out, err := cmd.Output()
	if err == nil {
		branch := strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/")
		if branch != "" {
			protected[branch] = true
		}
	}
	return protected
}

// ForcePushViolation reports whether pushArgs force-push to a protected branch.
// Force-pushing non-default PR branches is allowed; --mirror remains blocked.
func ForcePushViolation(repoDir, currentBranch string, pushArgs []string) (string, bool) {
	for _, a := range pushArgs {
		if a == "--mirror" {
			return a, true
		}
	}
	flag, ok := ForceFlag(pushArgs)
	if !ok {
		return "", false
	}
	if currentBranch == "" {
		currentBranch = currentPushBranch(repoDir)
	}
	protected := ProtectedBranches(repoDir)
	for _, branch := range forcePushTargets(currentBranch, pushArgs) {
		if protected[branch] {
			return branch, true
		}
	}
	return flag, false
}

func forcePushTargets(currentBranch string, pushArgs []string) []string {
	var targets []string
	skipNext := false
	for i, arg := range pushArgs {
		if skipNext {
			skipNext = false
			continue
		}
		if isBareForceFlag(arg) {
			continue
		}
		if ref, ok := leaseRefTarget(arg); ok {
			if ref != "" {
				targets = append(targets, ref)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if optionConsumesNext(arg) && i+1 < len(pushArgs) {
				skipNext = true
			}
			continue
		}
		if arg == "origin" || arg == "upstream" {
			continue
		}
		if target := pushTargetBranch(strings.TrimPrefix(arg, "+")); target != "" {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 && currentBranch != "" {
		targets = append(targets, currentBranch)
	}
	return targets
}

// isBareForceFlag reports whether arg is a force-family flag with no embedded
// ref; these never name a push target themselves.
func isBareForceFlag(arg string) bool {
	switch arg {
	case "--force", "-f", "--force-with-lease", "--force-if-includes", "--mirror":
		return true
	}
	return false
}

// leaseRefTarget extracts the branch from --force-with-lease=<ref>[:expect] or
// --force-if-includes=<ref>. ok reports that arg was one of those forms even
// when the embedded ref does not name a usable branch.
func leaseRefTarget(arg string) (string, bool) {
	if !strings.HasPrefix(arg, "--force-with-lease=") && !strings.HasPrefix(arg, "--force-if-includes=") {
		return "", false
	}
	ref := strings.TrimPrefix(strings.TrimPrefix(arg, "--force-with-lease="), "--force-if-includes=")
	ref = strings.TrimPrefix(ref, "refs/heads/")
	if ref == "" || strings.Contains(ref, ":") {
		return "", true
	}
	return ref, true
}

func optionConsumesNext(arg string) bool {
	switch arg {
	case "-u", "--set-upstream", "--repo", "--receive-pack", "--exec", "--push-option", "-o":
		return true
	}
	return false
}

func pushTargetBranch(refspec string) string {
	if strings.Contains(refspec, "*") {
		return ""
	}
	if strings.Contains(refspec, ":") {
		parts := strings.Split(refspec, ":")
		if len(parts) != 2 || parts[1] == "" {
			return ""
		}
		return strings.TrimPrefix(parts[1], "refs/heads/")
	}
	return strings.TrimPrefix(refspec, "refs/heads/")
}

func currentPushBranch(repoDir string) string {
	if repoDir == "" {
		repoDir = "."
	}
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return ""
	}
	return branch
}

// PushArgs assembles the full git argv for a safe push: an optional `-C
// repoDir`, the credential reset, `push`, and the caller's push arguments.
func PushArgs(repoDir, ghPath string, pushArgs []string) []string {
	var argv []string
	if repoDir != "" {
		argv = append(argv, "-C", repoDir)
	}
	argv = append(argv, CredentialResetArgs(ghPath)...)
	argv = append(argv, "push")
	argv = append(argv, pushArgs...)
	return argv
}

// Push runs a safe push, streaming git's output to the caller's stdout/stderr.
// It rejects force-pushes to protected branches, bounds the run by timeout
// (DefaultTimeout if zero), and sets GIT_TERMINAL_PROMPT=0 so git never falls
// back to an interactive prompt. A timeout is reported as a credential-helper
// hang, the failure this package exists to convert from "hang forever" into
// "fail in seconds".
func Push(repoDir string, pushArgs []string, timeout time.Duration) error {
	if target, blocked := ForcePushViolation(repoDir, currentPushBranch(repoDir), pushArgs); blocked {
		return fmt.Errorf("refusing %q: safe-push blocks force-pushes to protected/default branches "+
			"(main, master, and the repository default); use --force-with-lease only for non-default PR branches", target)
	}
	gh, err := ResolveGh()
	if err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	argv := PushArgs(repoDir, gh, pushArgs)
	// argv[0..] are compile-time literals plus the resolved gh path and the
	// caller's push refspecs; exec runs no shell, so refspecs are inert argv.
	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	// GIT_TERMINAL_PROMPT=0: never block on a terminal credential prompt.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("git push exceeded %s and was killed — this is the "+
			"credential-helper hang safe-push guards against; the push did NOT "+
			"complete. Verify `gh auth status` and retry; if it persists, the "+
			"keychain helper is wedged (check the credential config)", timeout)
	}
	if err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}
	return nil
}

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
//   - -uf, -fu, -vfq, ... (short-option clusters containing -f)
//   - --mirror (rewrites remote refs wholesale, equivalent to force-push)
//   - +<refspec>  (a leading '+' in a refspec means "force this ref")
//
// Options are only recognized before a bare `--`. Git treats every token after
// the end-of-options separator as a repository or refspec, so `git push origin
// -- -f` pushes a branch named "-f" and is not a force push — while a `+ref`
// after `--` still forces that ref and is still rejected.
//
// The leading-plus test applies only to refspecs. `git push` is
// `git push [options] [repository [refspec...]]`, so the first positional names
// the remote, and a remote may legally be called `+prod`; testing it as a
// refspec would reject the entirely non-destructive `git push +prod main`.
func ForceFlag(args []string) (string, bool) {
	endOfOptions := false
	sawRepository := false
	for _, a := range args {
		if !endOfOptions {
			if a == "--" {
				endOfOptions = true
				continue
			}
			if strings.HasPrefix(a, "-") && a != "-" {
				if forcesPush(a) {
					return a, true
				}
				continue
			}
		}
		if !sawRepository {
			sawRepository = true // the repository operand, not a refspec
			continue
		}
		// A refspec. A '+' prefix on a refspec forces that ref.
		if strings.HasPrefix(a, "+") {
			return a, true
		}
	}
	return "", false
}

// forcesPush reports whether one option token (leading '-', not the bare "-"
// or "--") forces the push.
//
// Short options are the subtle half. Git's parse-options — like POSIX getopt —
// lets callers bundle short options into a single argv token, so `-uf` is
// `-u -f` and forces the push just as surely as a standalone `-f`. Comparing
// the whole token against "-f" therefore misses every cluster; the letters
// have to be walked individually.
func forcesPush(opt string) bool {
	if strings.HasPrefix(opt, "--") {
		// Strip a glued value: --force-with-lease=<ref> is still a force.
		name, _, _ := strings.Cut(opt, "=")
		if name == "--" {
			return false
		}
		// Git's parse-options resolves any unambiguous ABBREVIATION of a long
		// option, so `--m` reaches --mirror and `--force-w` reaches
		// --force-with-lease (verified against git 2.55.0: both parse, while
		// --zzzz reports "unknown option"). Matching the spelled-out names
		// exactly therefore leaves the same hole the short-cluster fix closes.
		//
		// A guard must not try to reproduce git's ambiguity rules: treating
		// every prefix of a forbidden option as forbidden can only over-block,
		// and the tokens it over-blocks (--f, --fo) are ones git itself
		// rejects as ambiguous. Under-blocking would overwrite remote history.
		for _, forbidden := range forcingLongOpts {
			if strings.HasPrefix(forbidden, name) {
				return true
			}
		}
		// Any other long option, including the negating --no-force forms,
		// which are not prefixes of anything forbidden.
		return false
	}
	for _, c := range opt[1:] {
		if c == 'f' {
			// -f is `git push`'s only short option spelled with an 'f'.
			return true
		}
		if c == 'o' {
			// -o/--push-option is git push's only value-taking short option,
			// and its value may be glued on (`-ofoo`), so the remaining
			// letters are that value rather than further options.
			break
		}
	}
	return false
}

// forcingLongOpts are the long options that can overwrite remote history. Any
// unambiguous prefix of one of these reaches it through git's parse-options,
// so they are matched by prefix rather than by equality.
var forcingLongOpts = []string{
	"--force",
	"--force-with-lease",
	"--force-if-includes",
	"--mirror",
}

// ProtectedBranches returns the branch names that must not be force-pushed.
//
// Deprecated: use ProtectedTargets, which also reports whether the
// repository's default branch could be established. This wrapper drops that
// signal and is kept only for callers that predate it.
func ProtectedBranches(repoDir string) map[string]bool {
	protected, _ := ProtectedTargets(repoDir)
	return protected
}

// ForcePushViolation reports whether pushArgs force-push to a branch the
// policy protects. Force-pushing a PR branch is allowed; every other outcome
// is a refusal.
//
// The check fails closed in three ways, each of which was a live bypass:
//
//   - The whole-repository forms (--mirror, --all, --tags) are refused
//     outright, because what they update is not knowable from the operands.
//   - A destination set that cannot be positively enumerated is refused. A
//     wildcard refspec, a configured remote.<name>.push, push.default=matching,
//     or an unknown current branch all land here. Assuming "it must be the
//     current branch" is what allowed `safe-push -f origin` to rewrite main
//     from a feature branch tracking origin/main.
//   - A repository whose default branch cannot be established is refused,
//     because a default of `trunk` or `develop` would otherwise be protected
//     only by the conventional-name list.
//
// The returned string names what blocked the push, so the caller can say so.
func ForcePushViolation(repoDir, currentBranch string, pushArgs []string) (string, bool) {
	for _, a := range pushArgs {
		name, _, _ := strings.Cut(a, "=")
		if wholeRepoPushOptions[name] {
			return name, true
		}
	}
	flag, ok := ForceFlag(pushArgs)
	if !ok {
		return "", false
	}
	if currentBranch == "" {
		currentBranch = currentPushBranch(repoDir)
	}
	protected, defaultKnown := ProtectedTargets(repoDir)
	if !defaultKnown {
		return "the repository default branch is unknown " +
			"(run `git remote set-head origin --auto`)", true
	}
	targets, resolved := pushDestinations(repoDir, currentBranch, pushArgs)
	if !resolved {
		return "the push destination could not be determined from " + flag, true
	}
	// A `--force-with-lease=<ref>` names the ref the caller intends to force,
	// which is a destination in its own right even when the operands do not
	// repeat it.
	targets = append(targets, leaseRefs(pushArgs)...)
	for _, branch := range targets {
		if protected[branch] {
			return branch, true
		}
	}
	return flag, false
}

func currentPushBranch(repoDir string) string {
	out, err := gitQuery(repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(out)
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

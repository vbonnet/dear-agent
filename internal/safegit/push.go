// Package safegit builds and runs git pushes that cannot hang on the macOS
// keychain credential helper, and cannot force-push.
//
// The hang it prevents
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
// The fix
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
// Force-push is rejected by construction: a wrapper that can clobber shared
// history is not a wrapper worth allow-listing.
//
// Detected forms:
//   - -f / --force / --force-with-lease / --force-if-includes (flags)
//   - --mirror (rewrites remote refs wholesale, equivalent to force-push)
//   - +<refspec>  (a leading '+' in a refspec means "force this ref")
//
// The leading-plus test is applied only to refspecs. `git push` is
// `git push [options] [repository [refspec...]]`, so the first positional names
// the remote, and a remote may legally be called `+prod`; testing it as a
// refspec would reject the entirely non-destructive `git push +prod main`.
func ForceFlag(args []string) (string, bool) {
	sawRepository := false
	endOfOptions := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !endOfOptions {
			switch {
			case a == "--":
				endOfOptions = true
				continue
			case strings.HasPrefix(a, "--force-with-lease=") ||
				strings.HasPrefix(a, "--force-if-includes"):
				return a, true
			case isDestructiveLongOpt(a):
				return a, true
			case strings.HasPrefix(a, "--"):
				continue // some other long option
			case pushOptsWithValue[a]:
				i++ // the value is not a positional, so it is not the repository
				continue
			case strings.HasPrefix(a, "-") && a != "-":
				// Short-option cluster. Walk it in order: `-f` is push's only
				// short option spelled with an 'f', so reaching one means a
				// force push, but an option that takes a value swallows the
				// remainder of the word (`-ofoo`), and letters after it are
				// that value rather than further options.
				for _, c := range a[1:] {
					if c == 'f' {
						return a, true
					}
					if shortPushOptsWithValue[c] {
						break
					}
				}
				if len(a) == 2 && shortPushOptsWithValue[rune(a[1])] {
					i++ // `-o value`: the value is not the repository
				}
				continue
			}
		}
		if !sawRepository {
			sawRepository = true // the repository operand, not a refspec
			continue
		}
		if strings.HasPrefix(a, "+") {
			// A '+' prefix on a refspec forces that ref; block it.
			return a, true
		}
	}
	return "", false
}

// pushOptsWithValue lists the `git push` options whose value is routinely
// passed as a separate token (`git push -o ci.skip origin main`), so the value
// is not miscounted as the repository operand. Options normally written in the
// `--opt=value` form (`--repo`, `--exec`, `--receive-pack`) are deliberately
// absent: if one is ever spelled with a separate token, its value takes the
// repository slot and every later `+ref` is still tested, which over-blocks
// rather than under-blocks.
var pushOptsWithValue = map[string]bool{
	"-o": true, "--push-option": true,
}

// shortPushOptsWithValue are the `git push` short options that consume a value,
// either glued to the letter (`-ofoo`) or as the next token (`-o foo`).
var shortPushOptsWithValue = map[rune]bool{'o': true}

// destructiveLongOpts are the long options that make a push destructive. Git
// accepts unambiguous abbreviations — `git push --mir` really does mirror — so
// any argument that is a prefix of one of these is treated as that option. An
// ambiguous abbreviation git would itself reject is blocked here too, which
// over-rejects rather than letting a history-rewriting push through.
var destructiveLongOpts = []string{
	"--force", "--force-with-lease", "--force-if-includes", "--mirror",
}

// isDestructiveLongOpt reports whether a is one of destructiveLongOpts or an
// abbreviation of one. `--no-force…` is excluded: it disables forcing.
func isDestructiveLongOpt(a string) bool {
	if len(a) < 3 || !strings.HasPrefix(a, "--") || strings.HasPrefix(a, "--no-") {
		return false
	}
	for _, opt := range destructiveLongOpts {
		if strings.HasPrefix(opt, a) {
			return true
		}
	}
	return false
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
// It rejects force-push, bounds the run by timeout (DefaultTimeout if zero),
// and sets GIT_TERMINAL_PROMPT=0 so git never falls back to an interactive
// prompt. A timeout is reported as a credential-helper hang, the failure this
// package exists to convert from "hang forever" into "fail in seconds".
func Push(repoDir string, pushArgs []string, timeout time.Duration) error {
	if flag, ok := ForceFlag(pushArgs); ok {
		return fmt.Errorf("refusing %q: safe-push blocks force-pushes, --mirror, and +refspec "+
			"(any of these can overwrite remote history) — remove the flag/refspec to proceed", flag)
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

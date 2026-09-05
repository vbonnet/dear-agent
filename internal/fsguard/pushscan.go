package fsguard

import (
	"path/filepath"
	"strings"
)

// PushInvocation is one push found inside a Bash command, resolved far enough
// that safegit.ForcePushViolation can judge it.
//
// Args holds exactly what `git push` would receive, so a safe-push invocation
// has that wrapper's own flags (-C, --timeout, --check) removed rather than
// forwarded. Judging the wrapper's flags as if they were git's is how a
// destination came to be read off the wrong token.
type PushInvocation struct {
	// RepoDir is the directory the push runs in: the -C override when the
	// command has one, otherwise the directory `cd` tracking has reached.
	RepoDir string
	// AlsoDirs are the other directories this push may really run in, because
	// a conditional `cd` earlier in the chain may not have executed. A caller
	// enforcing a policy must evaluate every one of them and refuse if any is
	// protected; the post-cd directory alone is a guess.
	AlsoDirs []string
	// Args are the arguments `git push` itself receives.
	Args []string
}

// pushFrontEnds are the command words that run a git push. safe-push is one of
// them: it is the wrapper this repository pushes through, so a guard that only
// recognized `git push` saw none of the pushes actually being made.
var pushFrontEnds = map[string]bool{"git": true, "safe-push": true}

// safePushOptsWithValue are safe-push's own options that consume the following
// token. Everything else it receives is forwarded verbatim to git push.
var safePushOptsWithValue = map[string]bool{"-C": true, "--timeout": true}

// ScanPushes returns every push invocation in a Bash command, in the order the
// shell would run them. ok is false when the command cannot be tokenized, which
// callers must treat as fail-open: a guard that blocks whatever it cannot parse
// wedges the session for reasons unrelated to the policy.
//
// It exists because matching command text could not answer the two questions a
// force-push policy actually asks. `grep "git push --force"` is not a push, and
// `cmd-a && cmd-b` is two commands whose operands must not be pooled. Both
// mistakes were live: the first blocked reads, the second refused a legitimate
// feature-branch force-push because an unrelated command in the same chain
// named main.
func (g *Guard) ScanPushes(command, cwd string) (invocations []PushInvocation, ok bool) {
	tokens, ok := tokenize(command)
	if !ok {
		return nil, false
	}
	w := &pushWalk{g: g, currentDir: g.expand(cwd, cwd)}
	for _, seg := range splitSegments(tokens) {
		if w.trackSubshell(seg) {
			continue
		}
		args := stripRunners(realArgs(stripRedirections(seg.tokens)))
		if len(args) == 0 || w.trackCd(seg, args) {
			continue
		}
		word := filepath.Base(args[0])
		if !pushFrontEnds[word] {
			continue
		}
		inv, isPush := parsePushInvocation(word, args[1:])
		if !isPush {
			continue
		}
		inv.RepoDir = w.currentDir
		if inv.cFlag != "" {
			// An explicit -C names the repository outright, so the shell's
			// directory (and any doubt about it) stops mattering.
			inv.RepoDir = g.expand(inv.cFlag, w.currentDir)
		} else if len(w.alsoCheck) > 0 {
			inv.AlsoDirs = append([]string(nil), w.alsoCheck...)
		}
		invocations = append(invocations, inv.PushInvocation)
	}
	return invocations, true
}

// pushWalk carries the shell state a scan accumulates as it moves through a
// command: where the shell is, and where it might still be.
type pushWalk struct {
	g          *Guard
	currentDir string
	// alsoCheck mirrors checkSegments: a `cd` guarded by && / || or run inside
	// a pipeline's subshell may not have moved the shell, so the directory it
	// left behind stays a candidate.
	alsoCheck    []string
	subshellDirs []string
}

// trackSubshell restores the directory a `( … )` group borrowed, and reports
// whether this segment was only a parenthesis marker.
func (w *pushWalk) trackSubshell(seg segment) bool {
	if len(seg.tokens) != 1 || (seg.tokens[0] != "(" && seg.tokens[0] != ")") {
		return false
	}
	if seg.tokens[0] == "(" {
		w.subshellDirs = append(w.subshellDirs, w.currentDir)
		return true
	}
	if n := len(w.subshellDirs); n > 0 {
		w.currentDir = w.subshellDirs[n-1]
		w.subshellDirs = w.subshellDirs[:n-1]
		w.alsoCheck = nil
	}
	return true
}

// trackCd follows a directory change, reporting whether this segment was one.
//
// `cd` is matched on the literal command word, not the basename: only the shell
// builtin moves the shell, so an external program that happens to be named cd
// must not move the scanner's idea of where the following commands run.
func (w *pushWalk) trackCd(seg segment, args []string) bool {
	if args[0] != "cd" || len(args) < 2 {
		return false
	}
	next := w.g.expand(args[1], w.currentDir)
	if seg.conditional() {
		w.alsoCheck = append(w.alsoCheck, w.currentDir)
	} else {
		// An unconditional `cd` definitely ran, so earlier uncertainty about
		// where the shell is resolves and the candidate set collapses.
		w.alsoCheck = nil
	}
	w.currentDir = next
	return true
}

// parsedPush is the front-end-specific half of the parse, before the shell's
// directory tracking is applied.
type parsedPush struct {
	PushInvocation
	cFlag string
}

// parsePushInvocation splits one front-end's arguments into its -C override and
// the arguments git push itself receives. isPush is false for a git invocation
// whose subcommand is not push.
func parsePushInvocation(word string, args []string) (p parsedPush, isPush bool) {
	if word == "git" {
		cFlag, sub, subArgs := parseGit(args)
		if sub != "push" {
			return p, false
		}
		p.cFlag = cFlag
		p.Args = subArgs
		return p, true
	}
	// safe-push: strip the wrapper's own options, forward the rest.
	for i := 0; i < len(args); i++ {
		a := args[i]
		if safePushOptsWithValue[a] {
			if a == "-C" && i+1 < len(args) {
				p.cFlag = args[i+1]
			}
			i++
			continue
		}
		if a == "--check" || a == "-h" || a == "--help" {
			continue
		}
		if name, value, glued := strings.Cut(a, "="); glued && name == "-C" {
			p.cFlag = value
			continue
		}
		p.Args = append(p.Args, a)
	}
	return p, true
}

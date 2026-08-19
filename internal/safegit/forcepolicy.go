package safegit

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// gitQueryTimeout bounds the read-only git queries this policy runs. A policy
// check that hangs is a policy check nobody waits for.
const gitQueryTimeout = 5 * time.Second

// alwaysProtected are the branch names protected in every repository,
// independent of what the remote reports as its default.
//
// The list stays deliberately short. This policy exists to allow force-pushing
// PR branches, and a broader default would take that back. Shared integration
// branches that are not the default (develop, release, an environment branch)
// are real force-push hazards but they are repository-specific, so they are
// opt-in: `git config --add safegit.protectedbranch develop`. "It is not main"
// is not by itself evidence that a branch is a PR head, and this is the seam
// where a repository says so.
var alwaysProtected = []string{"main", "master"}

// ProtectedTargets is the set of branch names a force push must not reach.
//
// known is false when the repository's default branch could not be
// established. The set still contains the always-protected names, but a caller
// must refuse the force push rather than trust it: a repository whose default
// is `trunk` or `develop` and whose refs/remotes/origin/HEAD is absent or
// stale would otherwise leave its real default unprotected.
//
// Note the operational edge this creates deliberately. An actions/checkout
// clone is shallow and single-branch and has no refs/remotes/origin/HEAD, so
// every force push is refused inside CI until someone runs
// `git remote set-head origin --auto`. That is the intended trade: guessing
// the default from whichever conventional branch happens to exist would let a
// repository defaulting to `trunk` have `trunk` force-pushed, which is the
// exact hole this check closes. A guardrail against unrecoverable history
// loss should not rest on a probable answer.
func ProtectedTargets(repoDir string) (protected map[string]bool, known bool) {
	protected = map[string]bool{}
	for _, b := range alwaysProtected {
		protected[b] = true
	}
	for _, b := range configuredProtectedBranches(repoDir) {
		protected[b] = true
	}
	def, ok := defaultBranch(repoDir)
	if !ok {
		return protected, false
	}
	protected[def] = true
	return protected, true
}

// configuredProtectedBranches reads any repository-configured additions.
func configuredProtectedBranches(repoDir string) []string {
	out, err := gitQuery(repoDir, "config", "--get-all", "safegit.protectedbranch")
	if err != nil {
		return nil
	}
	var names []string
	for line := range strings.SplitSeq(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// defaultBranch resolves the remote's default branch. ok is false when it
// cannot be established, which is a refusal condition rather than a fallback.
func defaultBranch(repoDir string) (string, bool) {
	out, err := gitQuery(repoDir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", false
	}
	branch := strings.TrimPrefix(strings.TrimSpace(out), "origin/")
	if branch == "" {
		return "", false
	}
	return branch, true
}

// pushDestinations resolves every branch a `git push` invocation would update.
//
// resolved is false when the destination set cannot be positively enumerated:
// a wildcard refspec, `--all`/`--mirror`/`--tags`, a configured
// `remote.<name>.push`, `push.default=matching`, or an unknown current branch.
// Callers must refuse a force push in that case. Guessing here is how
// `safe-push -f origin` came to be allowed on a branch tracking origin/main.
func pushDestinations(repoDir, currentBranch string, pushArgs []string) (targets []string, resolved bool) {
	remote, refspecs, ok := splitPushOperands(pushArgs)
	if !ok {
		return nil, false
	}
	if len(refspecs) == 0 {
		return implicitDestinations(repoDir, remote, currentBranch)
	}
	for _, spec := range refspecs {
		dst, ok := refspecDestination(spec, currentBranch)
		if !ok {
			return nil, false
		}
		if dst != "" {
			targets = append(targets, dst)
		}
	}
	return targets, true
}

// pushOptionsWithValue take the following token as their value. `-u` and
// `--set-upstream` are deliberately absent: `git push -h` lists them as
// boolean, and consuming the next token discarded the refspec that named the
// real destination.
var pushOptionsWithValue = map[string]bool{
	"--repo": true, "--receive-pack": true, "--exec": true,
	"--push-option": true, "-o": true, "--recurse-submodules": true,
	"--signed": false, // optional value, only ever glued with '='
}

// wholeRepoPushOptions push more than the operands name, so the destination
// set cannot be read off the command line.
var wholeRepoPushOptions = map[string]bool{
	"--all": true, "--mirror": true, "--tags": true,
}

// splitPushOperands separates the repository operand from the refspecs.
//
// The first non-option positional is the repository whatever it is called, so
// a custom remote name is handled the same as origin, and `--repo=NAME`
// supplies it when no positional does.
func splitPushOperands(pushArgs []string) (remote string, refspecs []string, ok bool) {
	endOfOptions := false
	sawRemote := false
	for i := 0; i < len(pushArgs); i++ {
		a := pushArgs[i]
		if !endOfOptions {
			if a == "--" {
				endOfOptions = true
				continue
			}
			if name, value, glued := strings.Cut(a, "="); glued && strings.HasPrefix(a, "--") {
				if name == "--repo" {
					remote, sawRemote = value, true
				}
				if wholeRepoPushOptions[name] {
					return "", nil, false
				}
				continue
			}
			if wholeRepoPushOptions[a] {
				return "", nil, false
			}
			if strings.HasPrefix(a, "-") && a != "-" {
				if pushOptionsWithValue[a] {
					if a == "--repo" && i+1 < len(pushArgs) {
						remote, sawRemote = pushArgs[i+1], true
					}
					i++
				}
				continue
			}
		}
		if !sawRemote {
			remote, sawRemote = a, true
			continue
		}
		refspecs = append(refspecs, a)
	}
	return remote, refspecs, true
}

// refspecDestination returns the branch a single refspec updates. ok is false
// when the destination cannot be determined; an empty destination with ok
// means the refspec targets something that is not a branch (a tag, say) and
// carries no branch risk.
func refspecDestination(spec, currentBranch string) (string, bool) {
	spec = strings.TrimPrefix(spec, "+")
	if strings.Contains(spec, "*") {
		// `refs/heads/*:refs/heads/*` expands to every local branch,
		// including main. The set is not knowable from the command line.
		return "", false
	}
	dst := spec
	if src, rhs, found := strings.Cut(spec, ":"); found {
		if rhs == "" {
			return "", true // a delete refspec updates no branch by force
		}
		dst = rhs
		_ = src
	}
	switch {
	case strings.HasPrefix(dst, "refs/heads/"):
		return strings.TrimPrefix(dst, "refs/heads/"), true
	case strings.HasPrefix(dst, "refs/"):
		return "", true // tags and other namespaces are not branches
	case dst == "HEAD" || dst == "@":
		// Git resolves a source-only HEAD to the current branch, so
		// `push -f origin HEAD` on main updates main.
		if currentBranch == "" {
			return "", false
		}
		return currentBranch, true
	}
	return dst, true
}

// implicitDestinations resolves what a refspec-less push would update, from
// push.default, the branch's upstream, and any configured remote.<name>.push.
func implicitDestinations(repoDir, remote, currentBranch string) ([]string, bool) {
	if remote != "" {
		if out, err := gitQuery(repoDir, "config", "--get-all", "remote."+remote+".push"); err == nil && strings.TrimSpace(out) != "" {
			// A configured push refspec can name anything, including a
			// wildcard. Do not guess.
			return nil, false
		}
	}
	mode := "simple"
	if out, err := gitQuery(repoDir, "config", "--get", "push.default"); err == nil {
		if m := strings.TrimSpace(out); m != "" {
			mode = m
		}
	}
	switch mode {
	case "nothing":
		return nil, true
	case "matching":
		// Pushes every branch whose name exists on both sides, which
		// includes main in any ordinary clone.
		return nil, false
	case "upstream", "tracking":
		up, ok := upstreamBranch(repoDir, currentBranch)
		if !ok {
			return nil, false
		}
		return []string{up}, true
	default: // simple, current, and anything unrecognized
		if currentBranch == "" {
			return nil, false
		}
		return []string{currentBranch}, true
	}
}

// upstreamBranch returns the short name of currentBranch's upstream on the
// remote, which is what push.default=upstream updates. It is emphatically not
// the current branch's own name: a feature branch tracking origin/main under
// that mode force-pushes main.
func upstreamBranch(repoDir, currentBranch string) (string, bool) {
	if currentBranch == "" {
		return "", false
	}
	out, err := gitQuery(repoDir, "config", "--get", "branch."+currentBranch+".merge")
	if err != nil {
		return "", false
	}
	ref := strings.TrimSpace(out)
	if ref == "" {
		return "", false
	}
	return strings.TrimPrefix(ref, "refs/heads/"), true
}

// gitQuery runs a read-only git command, bounded by a timeout.
func gitQuery(repoDir string, args ...string) (string, error) {
	if repoDir == "" {
		repoDir = "."
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitQueryTimeout)
	defer cancel()
	all := append([]string{"-C", repoDir}, args...)
	out, err := exec.CommandContext(ctx, "git", all...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// leaseRefs returns the refs named by any --force-with-lease=<ref> option.
// The lease value may carry an expected object id (`--force-with-lease=
// main:<sha>`); only the ref part is a destination.
func leaseRefs(pushArgs []string) []string {
	// Held in a variable so the deliberately reversed prefix test (the given
	// name must be a prefix of the full option, which is how git resolves an
	// abbreviation) is not read as a transposed argument order.
	leaseOpt := "--force-with-lease"
	var refs []string
	for _, a := range pushArgs {
		name, value, found := strings.Cut(a, "=")
		// An abbreviation is accepted the same way git accepts it: the given
		// name must be a prefix of the full option.
		if !found || value == "" || !strings.HasPrefix(leaseOpt, name) || len(name) < 4 {
			continue
		}
		ref, _, _ := strings.Cut(value, ":")
		if ref = strings.TrimPrefix(ref, "refs/heads/"); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

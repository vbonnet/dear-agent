package fsguard

import (
	"path/filepath"
	"strings"
)

// This file adds the second policy layer: role capability. The rest of the
// package answers "may this path be written?"; this answers "may this session
// perform implementation work at all?".
//
// A VROOM supervisor (Meta-Orchestrator, Orchestrator, Overseer) coordinates
// workers. It must never do the work itself, for two reasons that are both
// operational rather than stylistic:
//
//  1. A detached supervisor cannot answer a permission prompt (ce-84l2). Any
//     tool call that raises a modal wedges the supervisor, and because the
//     Orchestrator is the only dispatcher, a wedged supervisor stalls the whole
//     pipeline until a human intervenes.
//  2. Implementation detail crowds out the supervisor's context window. The
//     coordination role degrades exactly as the mesh grows busiest.
//
// Prose in the role prompts already asked for this and did not achieve it, so
// the rule is enforced here where a supervisor cannot talk its way past it.
//
// Unlike the path policy, this layer never softens to warn/ask/defer: an "ask"
// would raise the very modal the rule exists to prevent. See CheckSupervisor*.

// Canonical VROOM supervisor session identities. They are duplicated from
// pkg/vroom/supervisor rather than imported so the PreToolUse hook binary stays
// small and fast, since it runs on every tool call. TestSupervisorIdentities-
// MatchCanonicalTopology imports the topology in the test binary only and fails
// if these ever drift.
const (
	supervisorIDMetaOrchestrator = "vroom-meta-orchestrator"
	supervisorIDOrchestrator     = "vroom-orchestrator"
	supervisorIDOverseer         = "vroom-overseer"
)

// EnvSupervisorID is set in the child environment by `agm supervisor run`.
const EnvSupervisorID = "AGM_SUPERVISOR_ID"

// EnvSessionName is exported into every AGM harness session. For supervisors
// spawned by vroom-dispatch (`agm session new <canonical-id>`) it is the only
// marker present, since that path does not set AGM_SUPERVISOR_ID.
const EnvSessionName = "AGM_SESSION_NAME"

// supervisorSessionNames is the set of AGM session names that identify a
// supervisor. A name outside this set is a worker and is never guarded.
var supervisorSessionNames = map[string]bool{
	supervisorIDMetaOrchestrator: true,
	supervisorIDOrchestrator:     true,
	supervisorIDOverseer:         true,
	// Compact aliases, accepted because heartbeat records and some CLI
	// surfaces address supervisors by them.
	"meta-o":   true,
	"orch":     true,
	"overseer": true,
}

// DetectSupervisor reports whether the current session is a VROOM supervisor,
// returning the identity that matched.
//
// Two spawn paths produce a supervisor and they mark the session differently:
// `agm supervisor run` sets AGM_SUPERVISOR_ID, while vroom-dispatch spawns
// through `agm session new` and sets only AGM_SESSION_NAME. Both are checked,
// because guarding one path would leave the production mesh unguarded.
//
// AGM_SUPERVISOR_ID is trusted whenever it is non-empty, including a value that
// is not canonical: that variable is only ever set for a supervisor, so an
// unrecognised value means an unknown supervisor rather than a worker.
// AGM_SESSION_NAME must match a known supervisor identity, because every worker
// session sets it too.
func DetectSupervisor(getenv func(string) string) (identity string, ok bool) {
	if getenv == nil {
		return "", false
	}
	if id := strings.TrimSpace(getenv(EnvSupervisorID)); id != "" {
		return id, true
	}
	name := strings.ToLower(strings.TrimSpace(getenv(EnvSessionName)))
	if supervisorSessionNames[name] {
		return name, true
	}
	return "", false
}

// supervisorWritableRoots are the only locations a supervisor may write. They
// are control-plane state, not repository content: heartbeats, the decision
// trail, session records, and scratch space for shaping typed command output.
// ~/beads is deliberately absent, because bead mutation goes through `bd`, and
// a raw write into the tracker store is never the intended path.
func supervisorWritableRoots(home string) []string {
	return []string{
		"/dev",
		"/tmp", "/private/tmp", "/var/tmp", "/private/var/tmp",
		"/var/folders", "/private/var/folders",
		filepath.Join(home, ".agm"),
	}
}

// supervisorGitRead lists git subcommands that cannot mutate a repository
// whatever flags they are given. Anything absent is treated as a mutation and
// refused, so an unfamiliar subcommand fails closed rather than through.
// Subcommands with both a read and a write mode (branch, remote, worktree, tag,
// config, stash) are handled by supervisorGitReadFlags instead.
var supervisorGitRead = map[string]bool{
	"log": true, "diff": true, "status": true, "show": true, "blame": true,
	"cat-file": true, "ls-files": true, "ls-tree": true, "ls-remote": true,
	"rev-parse": true, "rev-list": true, "for-each-ref": true, "grep": true,
	"shortlog": true, "show-ref": true, "show-branch": true, "describe": true,
	"merge-base": true, "name-rev": true, "whatchanged": true,
	"diff-tree": true, "diff-index": true, "check-ignore": true,
	"count-objects": true, "verify-commit": true, "annotate": true,
	"var": true, "version": true, "help": true,
	// fetch updates remote-tracking refs but never the working tree or local
	// history, so it stays an observation.
	"fetch": true,
}

// supervisorDualModeGit are subcommands whose read mode is useful to a
// supervisor but whose write mode is not. They are allowed only when every
// argument is a recognised read flag or read verb.
var supervisorDualModeGit = map[string]bool{
	"branch": true, "remote": true, "worktree": true, "tag": true,
	"config": true, "stash": true,
}

// supervisorBareFormMutates are dual-mode subcommands whose no-argument form is
// itself a mutation. `git stash` with no arguments means `git stash push`,
// which saves local modifications and reverts the working tree to HEAD. The
// argument scan cannot catch these on its own: an empty argument list contains
// nothing unrecognised, so it reads as vacuously safe.
var supervisorBareFormMutates = map[string]bool{
	"stash": true,
}

// supervisorGitReadFlags are valueless flags that keep a dual-mode subcommand
// in its read mode.
var supervisorGitReadFlags = map[string]bool{
	"-v": true, "-vv": true, "--verbose": true,
	"-l": true, "--list": true, "-a": true, "--all": true,
	"-r": true, "--remotes": true, "--show-current": true,
	"--porcelain": true,
}

// supervisorGitReadValueFlags are read flags that consume the following token
// as their value, so that value is not mistaken for a positional. Without this,
// the `user.email` of `git config --get user.email` would look like the key of
// a `git config <key> <value>` write.
var supervisorGitReadValueFlags = map[string]bool{
	"--get": true, "--get-all": true, "--get-regexp": true,
	"--contains": true, "--merged": true, "--no-merged": true,
	"--points-at": true, "--format": true, "--sort": true,
}

// supervisorGitReadVerbs are the read verbs of a dual-mode subcommand. Only
// after one of these may a bare positional appear, because that positional is
// then the thing being read (`git remote show origin`) rather than the thing
// being created (`git branch newbranch`, `git config key value`).
var supervisorGitReadVerbs = map[string]bool{
	"list": true, "show": true, "get-url": true,
}

// supervisorGuidance is the block message. It states what was attempted, the
// delegation path to take instead, and why the rule exists, matching the
// package's positive-guidance contract.
func supervisorGuidance(identity, attempt string) string {
	return "Supervisor " + identity + " tried to " + attempt + ". " +
		"Supervisors coordinate; they never perform the work. Delegate it " +
		"instead: file or update the bead with the acceptance criteria " +
		"(`bd --db ~/beads/context-engine/.beads --dolt-auto-commit on ...`) " +
		"and let the Orchestrator dispatch a worker for it through " +
		"`vroom-dispatch-direct`, which owns worker spawning. If the work is " +
		"already dispatched, follow it on the bead and the PR.\n\n" +
		"Why this is blocked rather than confirmed: a detached supervisor " +
		"cannot answer a permission prompt, so a modal here stalls dispatch " +
		"for the whole mesh, and implementation detail spends the context " +
		"window the coordination role depends on."
}

// CheckSupervisorWrite applies the role policy to a file-tool call (Edit,
// Write, MultiEdit, NotebookEdit) made by supervisor identity. Writes under the
// control-plane roots are allowed; every other path is refused with delegation
// guidance.
//
// Unlike Classify, this fails CLOSED: a path that cannot be expanded is refused
// rather than allowed, because a supervisor has no legitimate write outside the
// narrow control-plane set and the cost of a wrong allow is a wedged mesh.
func (g *Guard) CheckSupervisorWrite(identity, tool, path, cwd string) (allowed bool, message string) {
	attempt := "edit " + path + " with the " + tool + " tool"
	if strings.TrimSpace(path) == "" {
		return false, supervisorGuidance(identity, attempt)
	}
	resolved := g.Resolve(path, cwd)
	if !filepath.IsAbs(resolved) {
		return false, supervisorGuidance(identity, attempt)
	}
	if g.supervisorWritable(resolved) {
		return true, ""
	}
	return false, supervisorGuidance(identity, attempt)
}

// supervisorWritable reports whether an already-resolved absolute path lies
// under a control-plane root.
func (g *Guard) supervisorWritable(resolved string) bool {
	for _, root := range supervisorWritableRoots(g.Home) {
		if under(resolved, root) {
			return true
		}
	}
	return false
}

// CheckSupervisorCommand applies the role policy to a Bash command run by
// supervisor identity. It reuses the shared segment walker, so runner
// stripping, shell nesting, redirections, and `cd` tracking behave exactly as
// they do for the path policy; only the two judgements differ.
//
// Tokenisation still fails open, matching InspectCommand: an unparseable
// command is left to the native deny rules rather than blocked on a guess.
func (g *Guard) CheckSupervisorCommand(identity, command, cwd string) (allowed bool, message string) {
	return g.inspect(command, cwd, 0, g.supervisorCommandPolicy(identity))
}

func (g *Guard) supervisorCommandPolicy(identity string) commandPolicy {
	return commandPolicy{
		classify: func(target, dir string) (bool, string) {
			resolved := g.Resolve(target, dir)
			if !filepath.IsAbs(resolved) || !g.supervisorWritable(resolved) {
				return false, supervisorGuidance(identity, "write "+target+" from a shell command")
			}
			return true, ""
		},
		git: func(args []string, _ string) (bool, string) {
			return supervisorGitAllowed(identity, args)
		},
	}
}

// supervisorGitAllowed judges one git invocation for a supervisor. Read
// subcommands pass; every mutation is refused wherever it runs, including
// inside a worktree, because creating the commit is the worker's job.
func supervisorGitAllowed(identity string, args []string) (allowed bool, message string) {
	_, sub, subArgs := parseGit(args)
	if sub == "" {
		return true, "" // bare `git` or flags only: nothing to judge
	}
	if supervisorGitRead[sub] {
		return true, ""
	}
	if supervisorDualModeGit[sub] {
		if len(subArgs) == 0 && supervisorBareFormMutates[sub] {
			return false, supervisorGuidance(identity, "run `git "+sub+"`")
		}
		if supervisorGitArgsAreReads(subArgs) {
			return true, ""
		}
	}
	return false, supervisorGuidance(identity, "run `git "+sub+"`")
}

// supervisorGitArgsAreReads reports whether a dual-mode subcommand's arguments
// keep it in read mode. Every argument must be a recognised read flag, the
// value of a value-taking read flag, a read verb, or a positional that follows
// a read verb. Anything else, including a bare positional with no preceding
// read verb, makes the invocation a mutation, so the check fails closed on
// whatever it does not recognise.
func supervisorGitArgsAreReads(args []string) bool {
	sawReadVerb := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			if supervisorGitReadVerbs[a] {
				sawReadVerb = true
				continue
			}
			// A positional is only a read target once a read verb has named
			// the operation; otherwise it is the name of something being
			// created or set.
			if sawReadVerb {
				continue
			}
			return false
		}
		// Split --flag=value so the flag is matched on its own; the glued
		// value needs no separate handling.
		flag, _, glued := strings.Cut(a, "=")
		if supervisorGitReadFlags[flag] {
			continue
		}
		if supervisorGitReadValueFlags[flag] {
			if !glued {
				i++ // consume the separate value
			}
			continue
		}
		return false
	}
	return true
}

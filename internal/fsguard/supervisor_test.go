package fsguard

import (
	"strings"
	"testing"

	vroomsupervisor "github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

func envFunc(pairs map[string]string) func(string) string {
	return func(k string) string { return pairs[k] }
}

// TestSupervisorIdentitiesMatchCanonicalTopology fails if the identities this
// package hard-codes ever drift from pkg/vroom/supervisor. The topology is
// imported here, in the test binary only, so the hook binary stays small while
// the constants stay provably in sync.
func TestSupervisorIdentitiesMatchCanonicalTopology(t *testing.T) {
	for _, member := range vroomsupervisor.AllMembers() {
		if !supervisorSessionNames[member.ID] {
			t.Errorf("canonical supervisor ID %q is not recognised by the guard", member.ID)
		}
		if !supervisorSessionNames[member.Alias] {
			t.Errorf("canonical supervisor alias %q is not recognised by the guard", member.Alias)
		}
	}
}

func TestDetectSupervisor(t *testing.T) {
	tests := []struct {
		name      string
		environ   map[string]string
		wantOK    bool
		wantWho   string
		rationale string
	}{
		{
			name:      "agm supervisor run marks the child with AGM_SUPERVISOR_ID",
			environ:   map[string]string{EnvSupervisorID: "vroom-meta-orchestrator"},
			wantOK:    true,
			wantWho:   "vroom-meta-orchestrator",
			rationale: "supervisor.go sets this in the private executor's env",
		},
		{
			name:      "vroom-dispatch spawns set only the session name",
			environ:   map[string]string{EnvSessionName: "vroom-meta-orchestrator"},
			wantOK:    true,
			wantWho:   "vroom-meta-orchestrator",
			rationale: "agm session new does not set AGM_SUPERVISOR_ID",
		},
		{
			name:    "compact alias is recognised",
			environ: map[string]string{EnvSessionName: "meta-o"},
			wantOK:  true,
			wantWho: "meta-o",
		},
		{
			name:    "session name is matched case-insensitively",
			environ: map[string]string{EnvSessionName: "VROOM-Overseer"},
			wantOK:  true,
			wantWho: "vroom-overseer",
		},
		{
			name:      "an unrecognised AGM_SUPERVISOR_ID still counts as a supervisor",
			environ:   map[string]string{EnvSupervisorID: "vroom-future-role"},
			wantOK:    true,
			wantWho:   "vroom-future-role",
			rationale: "that variable is only ever set for supervisors, so fail closed",
		},
		{
			name:    "a worker session is not a supervisor",
			environ: map[string]string{EnvSessionName: "worker-ce-1234"},
			wantOK:  false,
		},
		{
			name:      "a worker whose name merely mentions a supervisor is not one",
			environ:   map[string]string{EnvSessionName: "vroom-orchestrator-helper"},
			wantOK:    false,
			rationale: "exact identity match, never a prefix",
		},
		{
			name:    "no markers at all",
			environ: map[string]string{},
			wantOK:  false,
		},
		{
			name:    "blank marker is not a supervisor",
			environ: map[string]string{EnvSupervisorID: "   "},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			who, ok := DetectSupervisor(envFunc(tt.environ))
			if ok != tt.wantOK {
				t.Fatalf("DetectSupervisor ok = %v, want %v (%s)", ok, tt.wantOK, tt.rationale)
			}
			if ok && who != tt.wantWho {
				t.Errorf("DetectSupervisor identity = %q, want %q", who, tt.wantWho)
			}
		})
	}
}

// TestSupervisorWriteBlocksTheWedgeScenario is the regression test for the
// incident: the Meta-Orchestrator tried to Edit SPEC.md and hit a permission
// modal it could not answer. Under the guard it is refused outright, and the
// refusal names the delegation path.
func TestSupervisorWriteBlocksTheWedgeScenario(t *testing.T) {
	g := testGuard()

	allowed, msg := g.CheckSupervisorWrite(
		"vroom-meta-orchestrator", "Edit",
		"/home/tester/worktrees/dear-agent/some-branch/SPEC.md",
		"/home/tester/worktrees/dear-agent/some-branch",
	)
	if allowed {
		t.Fatal("supervisor Edit of SPEC.md was allowed; this is the wedge that must not recur")
	}
	for _, want := range []string{"vroom-meta-orchestrator", "Delegate", "vroom-dispatch-direct", "permission prompt"} {
		if !strings.Contains(msg, want) {
			t.Errorf("guidance is missing %q; got:\n%s", want, msg)
		}
	}
}

func TestCheckSupervisorWrite(t *testing.T) {
	g := testGuard()

	tests := []struct {
		name        string
		tool, path  string
		cwd         string
		wantAllowed bool
	}{
		{
			name: "edit in a worktree is refused",
			tool: "Edit", path: "/home/tester/worktrees/dear-agent/b/internal/x/y.go",
			cwd: "/home/tester/worktrees/dear-agent/b", wantAllowed: false,
		},
		{
			name: "write in a worktree is refused",
			tool: "Write", path: "/home/tester/worktrees/dear-agent/b/NEW.md",
			cwd: "/home/tester/worktrees/dear-agent/b", wantAllowed: false,
		},
		{
			name: "MultiEdit is refused",
			tool: "MultiEdit", path: "/home/tester/worktrees/dear-agent/b/a.go",
			cwd: "/home/tester/worktrees/dear-agent/b", wantAllowed: false,
		},
		{
			name: "NotebookEdit is refused",
			tool: "NotebookEdit", path: "/home/tester/worktrees/dear-agent/b/a.ipynb",
			cwd: "/home/tester/worktrees/dear-agent/b", wantAllowed: false,
		},
		{
			name: "the golden checkout is refused",
			tool: "Edit", path: "/home/tester/src/dear-agent/SPEC.md",
			cwd: "/home/tester/src/dear-agent", wantAllowed: false,
		},
		{
			name: "a relative path in a worktree is refused",
			tool: "Edit", path: "SPEC.md",
			cwd: "/home/tester/worktrees/dear-agent/b", wantAllowed: false,
		},
		{
			name: "an empty path is refused rather than passed through",
			tool: "Edit", path: "", cwd: "/home/tester/worktrees/dear-agent/b",
			wantAllowed: false,
		},
		{
			name: "control-plane state under ~/.agm is allowed",
			tool: "Write", path: "/home/tester/.agm/vroom/heartbeat/meta-o.json",
			cwd: "/home/tester", wantAllowed: true,
		},
		{
			name: "the decision trail is allowed",
			tool: "Write", path: "/home/tester/.agm/vroom/trail.jsonl",
			cwd: "/home/tester", wantAllowed: true,
		},
		{
			name: "temp scratch is allowed",
			tool: "Write", path: "/tmp/ready.json", cwd: "/home/tester",
			wantAllowed: true,
		},
		{
			name: "a lookalike of the control-plane root is refused",
			tool: "Write", path: "/home/tester/.agmX/evil", cwd: "/home/tester",
			wantAllowed: false,
		},
		{
			name: "the beads store is refused because bd owns those writes",
			tool: "Write", path: "/home/tester/beads/context-engine/.beads/x.json",
			cwd: "/home/tester", wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, msg := g.CheckSupervisorWrite("vroom-overseer", tt.tool, tt.path, tt.cwd)
			if allowed != tt.wantAllowed {
				t.Fatalf("CheckSupervisorWrite(%q) allowed = %v, want %v", tt.path, allowed, tt.wantAllowed)
			}
			if !allowed && msg == "" {
				t.Error("a refusal must carry guidance, got an empty message")
			}
			if allowed && msg != "" {
				t.Errorf("an allowed write must carry no message, got %q", msg)
			}
		})
	}
}

func TestCheckSupervisorCommand(t *testing.T) {
	g := testGuard()
	const wt = "/home/tester/worktrees/dear-agent/b"

	tests := []struct {
		name        string
		command     string
		cwd         string
		wantAllowed bool
	}{
		// The observations a supervisor tick is built from must all survive.
		{"heartbeat", "agm supervisor heartbeat --id vroom-meta-orchestrator", "/home/tester", true},
		{"peer status", "agm supervisor status", "/home/tester", true},
		{"session health", "agm -o json session health --all", "/home/tester", true},
		{"beads ready queue", "bd --db ~/beads/context-engine/.beads --dolt-auto-commit on ready --json", "/home/tester", true},
		{"bead update stays typed", "bd --db ~/beads/context-engine/.beads update ce-1234 --priority 0", "/home/tester", true},
		{"typed dispatch", "~/go/bin/vroom-dispatch-direct -db ~/beads/context-engine/.beads -repo vbonnet/dear-agent", "/home/tester", true},
		{"read a repository file", "cat " + wt + "/SPEC.md", wt, true},
		{"grep the tree", "grep -rn supervisor " + wt + "/internal", wt, true},
		{"git log", "git -C " + wt + " log --oneline -5", "/home/tester", true},
		{"git status", "git -C " + wt + " status --porcelain", "/home/tester", true},
		{"git diff", "git -C " + wt + " diff", "/home/tester", true},
		{"git branch listing", "git -C " + wt + " branch --list", "/home/tester", true},
		{"git remote listing", "git -C " + wt + " remote -v", "/home/tester", true},
		{"git fetch is an observation", "git -C " + wt + " fetch origin", "/home/tester", true},
		{"heartbeat dir creation", "mkdir -p ~/.agm/vroom/heartbeat", "/home/tester", true},
		{"discarding stderr", "agm supervisor probe 2>/dev/null", "/home/tester", true},
		{"temp scratch", "agm -o json scan --cross-check > /tmp/scan.json", "/home/tester", true},

		// Implementation work is refused wherever it is attempted.
		{"redirect into a worktree file", "echo hi > " + wt + "/SPEC.md", wt, false},
		{"redirect into a bare relative file", "echo hi > SPEC.md", wt, false},
		{"append to a worktree file", "echo hi >> " + wt + "/SPEC.md", wt, false},
		{"heredoc into a worktree file", "cat > " + wt + "/SPEC.md <<'EOF'\nx\nEOF", wt, false},
		{"sed in place", "sed -i '' 's/a/b/' " + wt + "/SPEC.md", wt, false},
		{"remove a repository file", "rm " + wt + "/SPEC.md", wt, false},
		{"copy into the tree", "cp /tmp/x " + wt + "/SPEC.md", wt, false},
		{"tee into the tree", "echo x | tee " + wt + "/SPEC.md", wt, false},

		// Git mutation is refused everywhere, including inside a worktree,
		// because producing the commit is the worker's job.
		{"git add", "git -C " + wt + " add -A", "/home/tester", false},
		{"git commit in a worktree", "git commit -m wip", wt, false},
		{"git push", "git -C " + wt + " push origin HEAD", "/home/tester", false},
		{"git checkout", "git -C " + wt + " checkout -b feat/x", "/home/tester", false},
		{"git rebase", "git -C " + wt + " rebase origin/main", "/home/tester", false},
		{"git reset", "git -C " + wt + " reset --hard", "/home/tester", false},
		{"git merge", "git -C /home/tester/src/dear-agent merge --squash x", "/home/tester", false},
		{"git worktree add", "git -C /home/tester/src/dear-agent worktree add /home/tester/worktrees/x -b x", "/home/tester", false},
		{"git branch deletion", "git -C " + wt + " branch -D feat/x", "/home/tester", false},
		{"git remote add", "git -C " + wt + " remote add up git@example.com:x", "/home/tester", false},
		{"git stash push", "git -C " + wt + " stash push", "/home/tester", false},
		{"git config write", "git config user.email x@example.com", wt, false},
		{"an unknown git subcommand fails closed", "git -C " + wt + " frobnicate", "/home/tester", false},

		// Evasion paths the shared walker already understands.
		{"sudo prefix is stripped", "sudo rm " + wt + "/SPEC.md", wt, false},
		{"env prefix is stripped", "env rm " + wt + "/SPEC.md", wt, false},
		{"nested shell is inspected", "bash -c 'echo x > " + wt + "/SPEC.md'", wt, false},
		{"cd then write is attributed correctly", "cd " + wt + " && echo x > SPEC.md", "/home/tester", false},
		{"cd then commit is attributed correctly", "cd " + wt + " && git commit -m wip", "/home/tester", false},
		{"absolute path to git is still git", "/usr/bin/git -C " + wt + " commit -m wip", "/home/tester", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, msg := g.CheckSupervisorCommand("vroom-orchestrator", tt.command, tt.cwd)
			if allowed != tt.wantAllowed {
				t.Fatalf("CheckSupervisorCommand(%q) allowed = %v, want %v (msg: %s)",
					tt.command, allowed, tt.wantAllowed, msg)
			}
			if !allowed && !strings.Contains(msg, "Delegate") {
				t.Errorf("refusal must redirect to delegation, got:\n%s", msg)
			}
		})
	}
}

// TestSupervisorCommandFailsOpenOnUnparseableInput matches InspectCommand: a
// command the tokeniser cannot read is left to the native deny rules rather
// than blocked on a guess, so a guard bug can never brick the Bash tool.
func TestSupervisorCommandFailsOpenOnUnparseableInput(t *testing.T) {
	g := testGuard()
	allowed, _ := g.CheckSupervisorCommand("vroom-overseer", `echo "unterminated`, "/home/tester")
	if !allowed {
		t.Error("an unparseable command must fail open, matching InspectCommand")
	}
}

// TestWorkerPolicyIsUnchanged pins the boundary the incident fix must not
// cross: the path policy that governs worker sessions still answers exactly as
// it did, so adding the role layer cannot restrict a worker.
func TestWorkerPolicyIsUnchanged(t *testing.T) {
	g := testGuard()
	const wt = "/home/tester/worktrees/dear-agent/b"

	tests := []struct {
		name        string
		command     string
		wantAllowed bool
	}{
		{"worker writes in its worktree", "echo x > " + wt + "/SPEC.md", true},
		{"worker commits in its worktree", "git -C " + wt + " commit -m wip", true},
		{"worker still cannot commit in the golden checkout", "git -C /home/tester/src/dear-agent commit -m x", false},
		{"worker still cannot write in the golden checkout", "echo x > /home/tester/src/dear-agent/SPEC.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, _ := g.InspectCommand(tt.command, wt)
			if allowed != tt.wantAllowed {
				t.Errorf("InspectCommand(%q) allowed = %v, want %v", tt.command, allowed, tt.wantAllowed)
			}
		})
	}

	allowed, _ := g.Classify(wt+"/SPEC.md", wt)
	if !allowed {
		t.Error("a worker's write inside its worktree must still be allowed")
	}
}

// TestSupervisorDualModeGit pins the read/write split on the git subcommands
// that have both, since that boundary is where a mutation is likeliest to be
// mistaken for an observation.
func TestSupervisorDualModeGit(t *testing.T) {
	tests := []struct {
		args        []string
		wantAllowed bool
	}{
		{[]string{"branch", "--list"}, true},
		{[]string{"branch", "-a"}, true},
		{[]string{"branch", "--contains", "HEAD"}, true},
		{[]string{"branch", "--format=%(refname)"}, true},
		{[]string{"branch", "newbranch"}, false},
		{[]string{"branch", "-D", "feat/x"}, false},
		{[]string{"branch", "-m", "old", "new"}, false},
		{[]string{"remote", "-v"}, true},
		{[]string{"remote", "show", "origin"}, true},
		{[]string{"remote", "get-url", "origin"}, true},
		{[]string{"remote", "add", "up", "git@example.com:x"}, false},
		{[]string{"remote", "set-url", "origin", "git@example.com:x"}, false},
		{[]string{"config", "--get", "user.email"}, true},
		{[]string{"config", "--get-regexp", "^user"}, true},
		{[]string{"config", "--list"}, true},
		{[]string{"config", "user.email", "x@example.com"}, false},
		{[]string{"config", "--global", "user.email", "x@example.com"}, false},
		{[]string{"worktree", "list"}, true},
		{[]string{"worktree", "add", "/tmp/wt", "-b", "x"}, false},
		{[]string{"worktree", "remove", "/tmp/wt"}, false},
		{[]string{"stash", "list"}, true},
		{[]string{"stash", "push"}, false},
		{[]string{"tag", "--list"}, true},
		{[]string{"tag", "v1.0.0"}, false},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			allowed, _ := supervisorGitAllowed("vroom-overseer", tt.args)
			if allowed != tt.wantAllowed {
				t.Errorf("git %s allowed = %v, want %v",
					strings.Join(tt.args, " "), allowed, tt.wantAllowed)
			}
		})
	}
}

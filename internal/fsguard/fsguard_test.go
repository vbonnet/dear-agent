package fsguard

import (
	"strings"
	"testing"
)

// testGuard uses a fixed, fake home so classification is pure string logic and
// never touches the real filesystem.
func testGuard() *Guard { return &Guard{Home: "/home/tester"} }

func TestClassify(t *testing.T) {
	t.Parallel()
	g := testGuard()
	cwd := "/home/tester/worktrees/dear-agent/feat"

	tests := []struct {
		name        string
		path        string
		wantAllowed bool
		wantSubstr  string // expected substring of the block message (when blocked)
	}{
		{"worktree allowed", "~/worktrees/dear-agent/feat/main.go", true, ""},
		{"worktree abs allowed", "/home/tester/worktrees/x/f", true, ""},
		{"auto-memory carveout", "~/.auto-memory/notes.md", true, ""},
		{"tmp carveout", "/tmp/scratch", true, ""},
		{"private tmp carveout", "/private/tmp/scratch", true, ""},
		{"var tmp carveout", "/var/tmp/scratch", true, ""},
		{"var folders tmpdir carveout", "/var/folders/ab/xyz/T/scratch", true, ""},
		{"private var folders carveout", "/private/var/folders/ab/xyz/T/scratch", true, ""},
		{"dev null carveout", "/dev/null", true, ""},
		{"sessions carveout", "/sessions/abc/file", true, ""},
		{"vroom trail allowed", "~/.agm/vroom/trail.jsonl", true, ""},
		{"vroom heartbeat allowed", "~/.agm/vroom/heartbeat/test.json", true, ""},
		{"vroom nested allowed", "~/.agm/vroom/dispatch/ledger.jsonl", true, ""},
		{"agm outside vroom blocked", "~/.agm/agm.sock.cfg", false, ""},
		{"agm sandboxes allowed", "~/.agm/sandboxes/run.json", true, ""},
		{"vroom lookalike blocked", "~/.agm/vroomX/run.json", false, ""},
		{"sandboxes lookalike blocked", "~/.agm/sandboxesX/run.json", false, ""},
		{"src blocked names repo", "~/src/dear-agent/cmd/main.go", false,
			"git -C ~/src/dear-agent worktree add ~/worktrees/dear-agent/{branch}"},
		{"src root blocked placeholder", "~/src", false, "~/src which is protected"},
		{"brace HOME src blocked", "${HOME}/src/dear-agent/f", false, "~/src which is protected"},
		{"brace HOME worktree allowed", "${HOME}/worktrees/x/f", true, ""},
		{"dotfile blocked", "~/.gitconfig", false, "modify a dotfile"},
		{"dotdir blocked", "~/.config/claude/settings.json", false, "chezmoi"},
		{"home non-dotfile generic", "~/Documents/notes.txt", false,
			"Writes are only allowed in ~/worktrees/"},
		{"outside home generic", "/etc/hosts", false,
			"Writes are only allowed in ~/worktrees/"},
		{"relative anchored to worktree cwd", "main.go", true, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			allowed, msg := g.Classify(tc.path, cwd)
			if allowed != tc.wantAllowed {
				t.Fatalf("Classify(%q) allowed=%v, want %v (msg=%q)",
					tc.path, allowed, tc.wantAllowed, msg)
			}
			if !allowed {
				if msg == "" {
					t.Fatalf("Classify(%q) blocked but returned empty message", tc.path)
				}
				if tc.wantSubstr != "" && !strings.Contains(msg, tc.wantSubstr) {
					t.Fatalf("Classify(%q) msg=%q, want substring %q",
						tc.path, msg, tc.wantSubstr)
				}
			}
		})
	}
}

func TestInspectCommand(t *testing.T) {
	t.Parallel()
	g := testGuard()
	home := "/home/tester"

	tests := []struct {
		name        string
		command     string
		cwd         string
		wantAllowed bool
		wantSubstr  string
	}{
		// Reads are always allowed.
		{"cat src read", "cat ~/src/dear-agent/main.go", home, true, ""},
		{"grep src read", "grep -r foo ~/src/dear-agent", home, true, ""},
		{"ls src read", "ls -la ~/src", home, true, ""},
		{"git log read", "git -C ~/src/dear-agent log --oneline", home, true, ""},
		{"git diff read", "git -C ~/src/dear-agent diff", home, true, ""},
		{"git status read", "git -C ~/src/dear-agent status", home, true, ""},

		// Redirections.
		{"redirect to src blocked", "echo hi > ~/src/dear-agent/f", home, false,
			"~/src which is protected"},
		{"redirect append src blocked", "echo hi >> ~/src/dear-agent/f", home, false,
			"~/src which is protected"},
		{"redirect to worktree allowed", "echo hi > ~/worktrees/x/f", home, true, ""},
		{"redirect to dotfile blocked", "echo x > ~/.bashrc", home, false, "dotfile"},
		{"redirect to tmp allowed", "echo x > /tmp/scratch", home, true, ""},
		{"fd dup not a target", "make build 2>&1", home, true, ""},

		// File-mutating commands.
		{"rm src blocked", "rm ~/src/dear-agent/f", home, false, "~/src which is protected"},
		{"mkdir worktree allowed", "mkdir -p ~/worktrees/x/d", home, true, ""},
		{"touch src blocked", "touch ~/src/dear-agent/f", home, false, "~/src"},
		{"tee src blocked", "echo x | tee ~/src/dear-agent/f", home, false, "~/src"},
		{"sed -i src blocked", "sed -i s/a/b/ ~/src/dear-agent/f", home, false, "~/src"},
		{"sed -i worktree allowed", "sed -i s/a/b/ ~/worktrees/x/f", home, true, ""},
		{"sed without -i is read", "sed s/a/b/ ~/src/dear-agent/f", home, true, ""},
		{"sed -i -e script src blocked", "sed -i -e s/a/b/ ~/src/dear-agent/f", home, false, "~/src"},
		{"cp dest src blocked", "cp /tmp/a ~/src/dear-agent/b", home, false, "~/src"},
		{"cp from src to tmp allowed", "cp ~/src/dear-agent/a /tmp/b", home, true, ""},
		{"mv src blocked", "mv ~/src/dear-agent/a ~/src/dear-agent/b", home, false, "~/src"},
		{"chmod src blocked", "chmod 755 ~/src/dear-agent/f", home, false, "~/src"},
		{"mkdir scalar mode not target", "mkdir -m 755 ~/worktrees/x/d", home, true, ""},

		// Env-assignment prefix.
		{"env prefix then rm src", "FOO=bar rm ~/src/dear-agent/f", home, false, "~/src"},

		// git write rules in ~/src.
		{"git commit blocked", "git -C ~/src/dear-agent commit -m x", home, false,
			"git commit` in ~/src"},
		{"git checkout blocked", "git -C ~/src/dear-agent checkout main", home, false,
			"git checkout` in ~/src"},
		{"git reset blocked", "git -C ~/src/dear-agent reset --hard", home, false, "~/src"},
		{"git rebase blocked", "git -C ~/src/dear-agent rebase main", home, false, "~/src"},
		{"git merge allowed", "git -C ~/src/dear-agent merge feature", home, true, ""},
		{"git pull allowed", "git -C ~/src/dear-agent pull", home, true, ""},
		{"git fetch allowed", "git -C ~/src/dear-agent fetch origin", home, true, ""},
		{"git worktree allowed", "git -C ~/src/dear-agent worktree add ~/worktrees/x -b x", home, true, ""},
		{"git push allowed", "git -C ~/src/dear-agent push", home, true, ""},
		{"git force push to feature allowed", "git -C ~/src/dear-agent push --force-with-lease origin feature/rebased", home, true, ""},
		{"git force refspec to feature allowed", "git -C ~/src/dear-agent push origin +HEAD:refs/heads/feature/rebased", home, true, ""},
		{"git force push to main blocked", "git -C ~/src/dear-agent push --force origin main", home, false,
			"protected/default"},
		{"git force-with-lease main blocked", "git -C ~/src/dear-agent push --force-with-lease=main origin", home, false,
			"protected/default"},
		{"git -f blocked", "git -C ~/src/dear-agent push -f origin main", home, false, "force-push"},
		{"git commit in worktree allowed", "git -C ~/worktrees/x commit -m y", home, true, ""},

		// cd tracking for git.
		{"cd then git commit blocked", "cd ~/src/dear-agent && git commit -m x", home, false,
			"git commit` in ~/src"},
		{"cwd in src then git commit blocked", "git commit -m x", home + "/src/dear-agent", false,
			"git commit` in ~/src"},

		// Empty / noop.
		{"empty command allowed", "   ", home, true, ""},

		// Bypass defenses (Gemini review).
		{"line continuation rm src blocked", "rm \\\n  ~/src/dear-agent/f", home, false, "~/src"},
		{"sudo rm src blocked", "sudo rm ~/src/dear-agent/f", home, false, "~/src"},
		{"sudo -u rm src blocked", "sudo -u root rm ~/src/dear-agent/f", home, false, "~/src"},
		{"env rm src blocked", "env rm ~/src/dear-agent/f", home, false, "~/src"},
		{"env assign rm src blocked", "env FOO=bar rm ~/src/dear-agent/f", home, false, "~/src"},
		{"nohup rm src blocked", "nohup rm ~/src/dear-agent/f", home, false, "~/src"},
		{"sudo -n rm src blocked", "sudo -n rm ~/src/dear-agent/f", home, false, "~/src"},
		{"bash -c rm src blocked", `bash -c "rm ~/src/dear-agent/f"`, home, false, "~/src"},
		{"sh -c redirect src blocked", `sh -c "echo x > ~/src/dear-agent/f"`, home, false, "~/src"},
		{"sudo bash -c rm src blocked", `sudo bash -c "rm ~/src/dear-agent/f"`, home, false, "~/src"},
		{"eval rm src blocked", `eval rm ~/src/dear-agent/f`, home, false, "~/src"},
		{"brace HOME redirect src blocked", "echo x > ${HOME}/src/dear-agent/f", home, false, "~/src"},
		{"bash -c read allowed", `bash -c "cat ~/src/dear-agent/f"`, home, true, ""},
		{"sudo ls allowed", "sudo ls ~/src", home, true, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			allowed, msg := g.InspectCommand(tc.command, tc.cwd)
			if allowed != tc.wantAllowed {
				t.Fatalf("InspectCommand(%q) allowed=%v, want %v (msg=%q)",
					tc.command, allowed, tc.wantAllowed, msg)
			}
			if !allowed && tc.wantSubstr != "" && !strings.Contains(msg, tc.wantSubstr) {
				t.Fatalf("InspectCommand(%q) msg=%q, want substring %q",
					tc.command, msg, tc.wantSubstr)
			}
		})
	}
}

func TestCheckGh(t *testing.T) {
	t.Parallel()
	g := testGuard()
	home := "/home/tester"

	tests := []struct {
		name        string
		command     string
		wantAllowed bool
		wantSubstr  string
	}{
		// gh pr merge — direct merge path.
		{"pr merge blocked", "gh pr merge 42 --squash", false, "safe-merge"},
		{"pr merge no number blocked", "gh pr merge", false, "safe-merge"},
		{"pr merge HEAD blocked", "gh pr merge HEAD --squash --delete-branch", false, "safe-merge"},

		// gh api REST merge endpoint.
		{"api REST merge blocked", "gh api repos/owner/repo/pulls/42/merge --method PUT", false, "safe-merge"},
		{"api REST merge trailing blocked", "gh api -X PUT repos/owner/repo/pulls/1/merge", false, "safe-merge"},

		// gh api graphql merge mutations.
		{"graphql mergePullRequest blocked",
			`gh api graphql -f query='mutation { mergePullRequest(input:{pullRequestId:"PR_id"}){pullRequest{state}}}'`,
			false, "safe-merge"},
		{"graphql enableAutoMerge blocked",
			`gh api graphql -f query='mutation { enablePullRequestAutoMerge(input:{pullRequestId:"x"}){pullRequest{state}}}'`,
			false, "safe-merge"},

		// Bypass vectors: boolean flags must NOT consume the next token as their value.
		// --paginate / -p are boolean; treating them as value-taking lets the endpoint slip past.
		{"paginate boolean bypass blocked", "gh api --paginate repos/owner/repo/pulls/42/merge", false, "safe-merge"},
		{"-p boolean bypass blocked", "gh api -p repos/owner/repo/pulls/42/merge", false, "safe-merge"},

		// gh commands that are allowed.
		{"pr list allowed", "gh pr list --state open", true, ""},
		{"pr view allowed", "gh pr view 42", true, ""},
		{"pr checks allowed", "gh pr checks 42 --watch", true, ""},
		{"pr create allowed", "gh pr create --title x --body y", true, ""},
		{"api GET pulls allowed", "gh api repos/owner/repo/pulls/42", true, ""},
		{"api graphql no mutation allowed", "gh api graphql -f query='{ viewer { login } }'", true, ""},
		{"run list allowed", "gh run list", true, ""},

		// Shell wrappers around gh pr merge are also caught.
		{"bash -c gh pr merge blocked",
			`bash -c "gh pr merge 10 --squash"`,
			false, "safe-merge"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			allowed, msg := g.InspectCommand(tc.command, home)
			if allowed != tc.wantAllowed {
				t.Fatalf("InspectCommand(%q) allowed=%v, want %v (msg=%q)",
					tc.command, allowed, tc.wantAllowed, msg)
			}
			if !allowed && tc.wantSubstr != "" && !strings.Contains(msg, tc.wantSubstr) {
				t.Fatalf("InspectCommand(%q) msg=%q, want substring %q",
					tc.command, msg, tc.wantSubstr)
			}
		})
	}
}

func TestUnterminatedQuoteFailsOpen(t *testing.T) {
	t.Parallel()
	g := testGuard()
	// An unterminated quote is unparseable; the guard must fail open (allow)
	// and defer to the settings.json deny rules.
	allowed, _ := g.InspectCommand(`rm "~/src/dear-agent/f`, "/home/tester")
	if !allowed {
		t.Fatal("unterminated quote should fail open (allow), got block")
	}
}

func TestNewResolvesHome(t *testing.T) {
	t.Parallel()
	g := New()
	if g.Home == "" {
		t.Fatal("New() returned empty Home")
	}
}

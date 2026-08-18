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
	wt := home + "/worktrees/dear-agent/feat"

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
		{"git force push blocked", "git -C ~/src/dear-agent push --force", home, false,
			"force-push"},
		{"git force-with-lease blocked", "git -C ~/src/dear-agent push --force-with-lease", home, false,
			"force-push"},
		{"git -f blocked", "git -C ~/src/dear-agent push -f origin main", home, false, "force-push"},
		{"git commit in worktree allowed", "git -C ~/worktrees/x commit -m y", home, true, ""},

		// Destructive push forms the weaker local parser missed — now via
		// safegit.ForceFlag (ce-3knl.3).
		{"git push --mirror blocked", "git -C ~/src/dear-agent push --mirror", home, false, "force-push"},
		{"git push --force-if-includes blocked", "git -C ~/src/dear-agent push --force-if-includes origin main",
			home, false, "force-push"},
		{"git push +refspec blocked", "git -C ~/src/dear-agent push origin +main", home, false, "force-push"},
		{"git push --force-with-lease=ref blocked", "git -C ~/src/dear-agent push --force-with-lease=main origin main",
			home, false, "force-push"},
		{"git push normal refspec allowed", "git -C ~/src/dear-agent push origin main", home, true, ""},

		// Absolute / PATH-qualified executables must normalize to their basename
		// so they cannot bypass the per-command analysis (ce-3knl.3).
		{"absolute rm src blocked", "/bin/rm ~/src/dear-agent/f", home, false, "~/src"},
		{"usr-bin rm src blocked", "/usr/bin/rm -rf ~/src/dear-agent/d", home, false, "~/src"},
		{"absolute git commit blocked", "/usr/bin/git -C ~/src/dear-agent commit -m x", home, false,
			"git commit` in ~/src"},
		{"absolute sudo rm src blocked", "/usr/bin/sudo rm ~/src/dear-agent/f", home, false, "~/src"},
		{"absolute rm worktree allowed", "/bin/rm ~/worktrees/x/f", home, true, ""},

		// Bare relative targets resolve against cwd: destructive within ~/src,
		// harmless within a worktree (ce-3knl.3).
		{"bare rm in src cwd blocked", "rm AGENTS.md", home + "/src/dear-agent", false, "~/src"},
		{"bare rm nested in src cwd blocked", "rm -f docs/AGENTS.md", home + "/src/dear-agent", false, "~/src"},
		{"bare redirect in src cwd blocked", "echo x > README.md", home + "/src/dear-agent", false, "~/src"},
		{"bare redirect append in src cwd blocked", "echo x >> README.md", home + "/src/dear-agent", false, "~/src"},
		{"bare mv in src cwd blocked", "mv README.md OTHER.md", home + "/src/dear-agent", false, "~/src"},
		{"bare touch in src cwd blocked", "touch NEWFILE", home + "/src/dear-agent", false, "~/src"},
		{"bare rm in worktree cwd allowed", "rm main.go", home + "/worktrees/dear-agent/feat", true, ""},
		{"bare redirect in worktree cwd allowed", "echo x > out.txt", home + "/worktrees/dear-agent/feat", true, ""},
		{"bare chmod mode not target in worktree", "chmod 755 build.sh", home + "/worktrees/dear-agent/feat", true, ""},
		{"bare chmod in src cwd blocked", "chmod 644 AGENTS.md", home + "/src/dear-agent", false, "~/src"},

		// `--` ends option parsing: it is not itself a target, and a target
		// after it is classified even when it starts with '-' (ce-3knl.3).
		{"dashdash is not a target", "rm -- file.txt", wt, true, ""},
		{"dashdash hyphen target in src blocked", "rm -- -logfile", home + "/src/dear-agent", false, "~/src"},
		{"dashdash src target blocked", "rm -- ~/src/dear-agent/AGENTS.md", home, false, "~/src"},
		{"dashdash worktree target allowed", "rm -- main.go", wt, true, ""},

		// Redirection syntax must not be mistaken for command operands, or the
		// `1` of `2>&1` displaces the real destination (ce-3knl.3).
		{"cp with 2>&1 still sees dest", "cp /tmp/a ~/src/dear-agent/f 2>&1", home, false, "~/src"},
		{"cp with stdout redirect still sees dest", "cp /tmp/a ~/src/dear-agent/f >out.txt", wt, false, "~/src"},
		{"cp benign with 2>&1 allowed", "cp /tmp/a /tmp/b 2>&1", wt, true, ""},

		// chmod/chown/chgrp --reference replaces the leading spec operand, so
		// the first positional is already a target (ce-3knl.3).
		{"chmod --reference= keeps target", "chmod --reference=/tmp/ref ~/src/dear-agent/AGENTS.md", home, false, "~/src"},
		{"chmod --reference spaced keeps target", "chmod --reference /tmp/ref ~/src/dear-agent/AGENTS.md",
			home, false, "~/src"},
		{"chown --reference keeps target", "chown --reference=/tmp/ref ~/src/dear-agent/AGENTS.md", home, false, "~/src"},

		// Value-taking options may trail the operands, so their value must be
		// consumed or it becomes the "last" operand (ce-3knl.3).
		{"cp --suffix after operands", "cp /tmp/a ~/src/dear-agent/f --suffix bak", home, false, "~/src"},
		{"cp -S after operands", "cp /tmp/a ~/src/dear-agent/f -S bak", home, false, "~/src"},

		// -t/--target-directory names the destination; every positional is a
		// read-only source (ce-3knl.3).
		{"cp -t dest blocked", "cp -t ~/src/dear-agent /tmp/a", home, false, "~/src"},
		{"cp --target-directory= blocked", "cp --target-directory=~/src/dear-agent /tmp/a", home, false, "~/src"},
		{"cp --target-directory spaced blocked", "cp --target-directory ~/src/dear-agent /tmp/a", home, false, "~/src"},
		{"cp -tDIR glued blocked", "cp -t~/src/dear-agent /tmp/a", home, false, "~/src"},
		{"cp -rt cluster blocked", "cp -rt ~/src/dear-agent /tmp/a", home, false, "~/src"},
		{"install -t dest blocked", "install -t ~/src/dear-agent /tmp/a", home, false, "~/src"},
		{"mv -t dest blocked", "mv -t ~/src/dear-agent /tmp/a", home, false, "~/src"},
		{"cp -t benign allowed", "cp -t /tmp/dir /tmp/a", wt, true, ""},

		// A remote may legally be named +prod; only refspecs carry force
		// semantics in a leading '+' (ce-3knl.3).
		{"git push +remote is not a force push", "git -C ~/src/dear-agent push +prod main", home, true, ""},
		{"git push clustered -uf blocked", "git -C ~/src/dear-agent push -uf origin main", home, false, "force-push"},

		// Redirect targets resolve against the directory `cd` tracking has
		// reached, not the original cwd (ce-3knl.3).
		{"cd then bare redirect blocked", "cd ~/src/dear-agent && echo x > README.md", wt, false, "~/src"},

		// A '#' opening a word starts a comment, so its words are not operands;
		// inside a word it is an ordinary character (ce-3knl.3).
		{"comment does not displace destination", "cp /tmp/a ~/src/dear-agent/f # backup", "/tmp", false, "~/src"},
		{"hash inside a word is not a comment", "rm file#1", wt, true, ""},

		// touch's -d/-t/-r take values that are not paths (ce-3knl.3).
		{"touch -d value not a target", "touch -d yesterday /tmp/out", home + "/src/dear-agent", true, ""},
		{"touch -t stamp not a target", "touch -t 202601010000 /tmp/out", home + "/src/dear-agent", true, ""},
		{"touch real target still blocked", "touch NEWFILE", home + "/src/dear-agent", false, "~/src"},

		// A `cd` inside a subshell does not outlive it (ce-3knl.3).
		{"subshell cd is restored", "(cd /tmp); rm AGENTS.md", home + "/src/dear-agent", false, "~/src"},
		{"subshell interior still checked", "(cd ~/src/dear-agent; rm AGENTS.md)", wt, false, "~/src"},

		// Only the shell builtin `cd` moves the shell; an external program that
		// merely has that basename does not (ce-3knl.3).
		{"external cd does not move tracking", "/tmp/cd /tmp; rm AGENTS.md", home + "/src/dear-agent", false, "~/src"},
		{"builtin cd still tracked", "cd /tmp; rm AGENTS.md", home + "/src/dear-agent", true, ""},

		// -t inside a short-option cluster with its directory glued on
		// (ce-3knl.3).
		{"cp -atDIR cluster blocked", "cp -at~/src/dear-agent /tmp/a", home, false, "~/src"},
		{"cp -at spaced cluster blocked", "cp -at ~/src/dear-agent /tmp/a", home, false, "~/src"},

		// rsync value-taking options must be consumed, and its auxiliary output
		// directories are themselves write targets (ce-3knl.3).
		{"rsync --exclude after operands", "rsync /tmp/a ~/src/dear-agent/d --exclude foo", "/tmp", false, "~/src"},
		{"rsync --backup-dir is a target", "rsync /tmp/a /tmp/b --backup-dir ~/src/dear-agent", "/tmp", false, "~/src"},
		{"rsync benign allowed", "rsync /tmp/a /tmp/b --exclude foo", "/tmp", true, ""},

		// A digit is a file descriptor only when glued to the operator; with
		// whitespace it is an ordinary operand (ce-3knl.3).
		{"whitespace digit is an operand", "rm 2 > /tmp/log", home + "/src/dear-agent", false, "~/src"},
		{"glued fd is still stripped", "cp /tmp/a ~/src/dear-agent/f 2>&1", wt, false, "~/src"},

		// A value-taking short option swallows the rest of its cluster, so the
		// `t` in `-Stext` is suffix text and not --target-directory (ce-3knl.3).
		{"cp -Stext is not a target dir", "cp -Stext /tmp/a ~/src/dear-agent/f", "/tmp", false, "~/src"},

		// Input redirections are read-only but must still leave the operand
		// list, or their target displaces the destination (ce-3knl.3).
		{"input redirect does not displace dest", "cp /tmp/a ~/src/dear-agent/f < in", "/tmp", false, "~/src"},
		{"herestring does not displace dest", "cp /tmp/a ~/src/dear-agent/f <<< x", "/tmp", false, "~/src"},

		// rsync's auxiliary output dirs supplement its positional destination
		// rather than replacing it; its basis dirs are read-only (ce-3knl.3).
		{"rsync aux keeps primary dest", "rsync /tmp/a ~/src/dear-agent/d --backup-dir /tmp/bk", "/tmp", false, "~/src"},
		{"rsync --max-delete value consumed", "rsync /tmp/a ~/src/dear-agent/d --max-delete 1", "/tmp", false, "~/src"},
		{"rsync --compare-dest is read-only", "rsync --compare-dest ~/src/dear-agent/b /tmp/a /tmp/b", "/tmp", true, ""},

		// A chmod symbolic mode is option-shaped and must not be filtered out as
		// a flag, or the real target is dropped as the leading spec (ce-3knl.3).
		{"chmod -w symbolic mode blocked", "chmod -w ~/src/dear-agent/AGENTS.md", wt, false, "~/src"},
		{"chmod -R is a flag not a mode", "chmod -R 755 ~/src/dear-agent", wt, false, "~/src"},
		{"chmod -w benign allowed", "chmod -w /tmp/f", wt, true, ""},

		// A digit target is a descriptor only for a dup operator (ce-3knl.3).
		{"numeric redirect target blocked", "echo x > 2", home + "/src/dear-agent", false, "~/src"},
		{"fd dup target still skipped", "echo x 2>&1", home + "/src/dear-agent", true, ""},

		// mktemp's -p/--tmpdir relocates the template out of the cwd
		// (ce-3knl.3).
		{"mktemp -p escapes protected cwd", "mktemp -p /tmp scratch.XXXXXX", home + "/src/dear-agent", true, ""},
		{"mktemp bare template still blocked", "mktemp scratch.XXXXXX", home + "/src/dear-agent", false, "~/src"},

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

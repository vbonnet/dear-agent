// install-post-merge-hook installs the post-merge trigger into the hooks
// directory git actually consults — honouring core.hooksPath, and refusing to
// clobber a chezmoi-managed hooks dir. The hook rebuilds the managed AGM,
// AGM reaper, VROOM dispatch, and Wayfinder binaries whose source changed
// (Stage 1) and reaps provably-merged worktrees (Stage 2) after a PR lands on
// the default branch.
//
// Why this is its own tool (not `agm admin install-hooks`): install-hooks
// manages Claude Code hooks under ~/.claude/hooks. This is a *git* hook, and on
// this host git's effective hooks dir is core.hooksPath (a repo-local
// .git/hooks would be silently bypassed). Getting that resolution right is the
// whole job — install to the wrong dir and the trigger never fires.
//
// It is a Go binary rather than a shell script on purpose: per
// AGENTS.md principle 4 ("Go is the default") and the language-policy
// CI gate, non-trivial install logic belongs in Go where it is testable.
//
// Positive-guidance stance (principle 2): when it cannot safely complete an
// install, it explains what it found and the exact right way to finish, rather
// than failing blindly or overwriting your work.
//
// Exit codes: 0 installed/up-to-date, 1 conflict or error, 3 chezmoi-managed.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(os.Stdout, os.Stderr))
}

func run(stdout, stderr *os.File) int {
	repoRoot, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		fmt.Fprintf(stderr, "Error: not in a git repository: %v\n", err)
		return 1
	}
	hookSrc := filepath.Join(repoRoot, "scripts", "git-hooks", "post-merge")
	srcBytes, err := os.ReadFile(hookSrc)
	if err != nil {
		fmt.Fprintf(stderr, "Error: hook source not found at %s: %v\n", hookSrc, err)
		return 1
	}

	home, _ := os.UserHomeDir()
	hooksDir, sourceNote := resolveHooksDir(repoRoot, home)
	hookDst := filepath.Join(hooksDir, "post-merge")

	fmt.Fprintf(stdout, "Target: %s\n        (%s)\n\n", hookDst, sourceNote)

	// Refuse to clobber a chezmoi-managed hooks dir — chezmoi owns the
	// canonical copy and would revert a direct write on the next apply.
	if managed, ok := chezmoiManaged(); ok && isChezmoiManaged(managed, hooksDir, home) {
		fmt.Fprint(stderr, chezmoiGuidance(hooksDir, hookSrc, hookDst))
		return 3
	}

	// Conflict check: never silently overwrite a foreign hook.
	if existing, err := os.ReadFile(hookDst); err == nil {
		if bytes.Equal(existing, srcBytes) {
			fmt.Fprintln(stdout, "Already installed and up to date — nothing to do.")
			return 0
		}
		fmt.Fprint(stderr, conflictGuidance(hookDst, hookSrc))
		return 1
	}

	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "Error: cannot create %s: %v\n", hooksDir, err)
		return 1
	}
	if err := os.WriteFile(hookDst, srcBytes, 0o755); err != nil { //#nosec G306 -- executable hook
		fmt.Fprintf(stderr, "Error: cannot write %s: %v\n", hookDst, err)
		return 1
	}
	fmt.Fprintf(stdout, "✓ Installed: %s (default-branch merges only)\n", hookDst)
	fmt.Fprintln(stdout, "    Stage 1: rebuild Go binaries whose source changed (agm, agm-reaper, vroom-dispatch, wayfinder)")
	fmt.Fprintln(stdout, "    Stage 2: agm worktree sweep --execute (reap provably-merged worktrees)")
	fmt.Fprintln(stdout, "    Stage 3: bd close beads cited by Closes/Fixes/Resolves in the merge")
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, "It runs after `git pull`/`git merge` lands a PR on the default branch.")
	fmt.Fprintln(stdout, "Disable per-shell with: export DEAR_AGENT_POST_MERGE_REBUILD=0 (Stage 1)")
	fmt.Fprintln(stdout, "                        export AGM_POST_MERGE_SWEEP=0           (Stage 2)")
	fmt.Fprintln(stdout, "                        export DEAR_AGENT_POST_MERGE_BEAD_TRANSITION=0 (Stage 3)")
	return 0
}

// resolveHooksDir returns the dir git will actually use for hooks (mirroring
// git's own precedence: core.hooksPath wins over .git/hooks) plus a short note
// describing where it came from.
func resolveHooksDir(repoRoot, home string) (dir, note string) {
	if cp, err := gitOutput("config", "--get", "core.hooksPath"); err == nil && cp != "" {
		return expandHooksPath(cp, repoRoot, home), "core.hooksPath = " + expandHooksPath(cp, repoRoot, home)
	}
	gitPath, err := gitOutput("rev-parse", "--git-path", "hooks")
	if err != nil || gitPath == "" {
		gitPath = filepath.Join(".git", "hooks")
	}
	if !filepath.IsAbs(gitPath) {
		gitPath = filepath.Join(repoRoot, gitPath)
	}
	return gitPath, "repo-local hooks dir = " + gitPath
}

// expandHooksPath interprets a core.hooksPath value exactly as git does: a
// leading ~ expands to home, and a relative path resolves against the repo
// root.
func expandHooksPath(raw, repoRoot, home string) string {
	switch {
	case strings.HasPrefix(raw, "~/"):
		return filepath.Join(home, raw[2:])
	case filepath.IsAbs(raw):
		return raw
	default:
		return filepath.Join(repoRoot, raw)
	}
}

// isChezmoiManaged reports whether hooksDir (or a post-merge under it) appears
// in `chezmoi managed` output (paths there are relative to home).
func isChezmoiManaged(managedOutput, hooksDir, home string) bool {
	rel := strings.TrimPrefix(hooksDir, home+string(os.PathSeparator))
	if rel == hooksDir { // not under home → cannot be a chezmoi target
		return false
	}
	for _, line := range strings.Split(managedOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == rel || line == rel+"/post-merge" {
			return true
		}
	}
	return false
}

func chezmoiGuidance(hooksDir, hookSrc, hookDst string) string {
	return fmt.Sprintf(`The effective hooks dir is managed by chezmoi:
  %s

Not writing there directly — chezmoi would revert it on the next apply.
Finish the install through chezmoi so the hook is versioned in your dotfiles:

  cp %q "$(chezmoi source-path %q)/post-merge"
  chezmoi apply %q
  chmod +x %q

(The canonical, reviewed copy lives in this repo at
 scripts/git-hooks/post-merge — keep the chezmoi copy in sync with it.)
`, hooksDir, hookSrc, hooksDir, hookDst, hookDst)
}

func conflictGuidance(hookDst, hookSrc string) string {
	return fmt.Sprintf(`A different post-merge hook already exists at:
  %s

Not overwriting it. To adopt this one, review the existing hook then either:
  • merge its logic with %s, or
  • remove the existing hook and re-run this installer.

Tip: this repo's hook runs `+"`agm worktree sweep --execute`"+` only on the
default branch and exits 0 unconditionally, so it composes cleanly if you
append it to an existing post-merge hook.
`, hookDst, hookSrc)
}

// gitOutput runs a git command and returns trimmed stdout.
func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

// chezmoiManaged returns the output of `chezmoi managed`; ok is false when
// chezmoi is not installed (treated as "nothing is chezmoi-managed").
func chezmoiManaged() (output string, ok bool) {
	if _, err := exec.LookPath("chezmoi"); err != nil {
		return "", false
	}
	out, err := exec.Command("chezmoi", "managed").Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

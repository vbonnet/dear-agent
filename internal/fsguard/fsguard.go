// Package fsguard implements the path-classification policy shared by the
// Claude Code PreToolUse write-guard hooks (pretool-fs-write-guard and
// pretool-bash-write-guard).
//
// The policy is "worktree-only writes": the agent may freely write under
// ~/worktrees (and a handful of scratch carve-outs), but ~/src is a protected
// golden checkout, dotfiles flow through chezmoi, and everything else is
// off-limits. When a write is blocked the guard returns POSITIVE GUIDANCE —
// it tells the agent the right way to accomplish the task, not merely that it
// is forbidden — and every message ends with a documented escalation path.
//
// Both guards fail open: the caller is expected to exit 0 on any parse error
// or unexpected input, so a bug in the guard can never brick the Edit/Write/
// Bash tools. The settings.json deny rules remain the backstop.
package fsguard

import (
	"os"
	"path/filepath"
	"strings"
)

// Escalation is appended to every block message. It gives the agent a
// documented, machine-detectable way to request an exception rather than
// silently working around the guard.
const Escalation = "\n\nIf you need this permission, escalate: " +
	"PERMISSION_ESCALATION: I need [specific permission] because [reason]"

// Guard classifies write targets against the worktree-only policy. Home is
// resolved once at construction; tests inject a temporary directory.
type Guard struct {
	Home string
}

// New builds a Guard rooted at the current user's home directory, resolving
// symlinks so that e.g. macOS's /var -> /private/var does not defeat prefix
// checks. It falls back to the unresolved home (or "/") if resolution fails.
func New() *Guard {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/"
	}
	if resolved, rerr := filepath.EvalSymlinks(home); rerr == nil {
		home = resolved
	}
	return &Guard{Home: filepath.Clean(home)}
}

// expand resolves a possibly ~/$HOME/relative path to an absolute, cleaned
// path. cwd is used to anchor relative paths (defaulting to Home when empty).
func (g *Guard) expand(path, cwd string) string {
	if path == "" {
		return ""
	}
	switch {
	case path == "~":
		path = g.Home
	case strings.HasPrefix(path, "~/"):
		path = g.Home + path[1:]
	case path == "$HOME" || path == "${HOME}":
		path = g.Home
	case strings.HasPrefix(path, "$HOME/"):
		path = g.Home + path[len("$HOME"):]
	case strings.HasPrefix(path, "${HOME}/"):
		path = g.Home + path[len("${HOME}"):]
	}
	if !filepath.IsAbs(path) {
		base := cwd
		if base == "" {
			base = g.Home
		}
		path = filepath.Join(base, path)
	}
	return filepath.Clean(path)
}

// under reports whether path is base itself or lives somewhere beneath it.
// It tolerates a base that is the root directory or already carries a trailing
// separator, so prefix classification stays correct in those edge cases.
func under(path, base string) bool {
	if path == base {
		return true
	}
	if !strings.HasSuffix(base, string(os.PathSeparator)) {
		base += string(os.PathSeparator)
	}
	return strings.HasPrefix(path, base)
}

// isWritableCarveout reports locations that are always writable regardless of
// the worktree-only policy. These are not protected source trees — they are
// agent scratch space and I/O plumbing that necessary operations depend on:
//
//   - /dev/...         -> pseudo-devices (/dev/null, /dev/stdout, /dev/stderr)
//   - ~/.auto-memory/  -> the agent's persistent memory store
//   - /tmp, /var/tmp   -> temporary files (macOS /tmp -> /private/tmp symlink)
//   - /var/folders/... -> macOS's default $TMPDIR (os.TempDir/t.TempDir land here)
//   - /sessions/...    -> Cowork sandbox session directories
func (g *Guard) isWritableCarveout(p string) bool {
	if under(p, "/dev") {
		return true
	}
	if under(p, filepath.Join(g.Home, ".auto-memory")) {
		return true
	}
	tmpDirs := []string{
		"/tmp", "/private/tmp", "/var/tmp", "/private/var/tmp",
		"/var/folders", "/private/var/folders",
	}
	for _, tmp := range tmpDirs {
		if under(p, tmp) {
			return true
		}
	}
	return under(p, "/sessions")
}

// repoName extracts the first path component beneath ~/src for use in the
// "create a worktree" guidance, falling back to the {repo} placeholder.
func (g *Guard) repoName(p, src string) string {
	rel := strings.TrimPrefix(p, src+string(os.PathSeparator))
	repo := ""
	if rel != "" && rel != p {
		repo = strings.SplitN(rel, string(os.PathSeparator), 2)[0]
	}
	if repo == "" {
		return "{repo}"
	}
	return repo
}

// Classify returns whether a write to path (anchored at cwd) is allowed and,
// when blocked, a positive-guidance message explaining the right way to
// proceed. The returned message does NOT include Escalation; callers append
// it when emitting.
func (g *Guard) Classify(path, cwd string) (allowed bool, message string) {
	p := g.expand(path, cwd)
	worktrees := filepath.Join(g.Home, "worktrees")
	src := filepath.Join(g.Home, "src")

	if g.isWritableCarveout(p) {
		return true, ""
	}
	if under(p, worktrees) {
		return true, ""
	}
	if under(p, src) {
		repo := g.repoName(p, src)
		return false, "You're trying to write to ~/src which is protected. " +
			"Create a worktree first: git -C ~/src/" + repo +
			" worktree add ~/worktrees/" + repo + "/{branch} -b {branch}, " +
			"then make your changes there."
	}
	if under(p, g.Home) {
		rest := strings.TrimPrefix(p, g.Home+string(os.PathSeparator))
		first := rest
		if rest != p {
			first = strings.SplitN(rest, string(os.PathSeparator), 2)[0]
		}
		if strings.HasPrefix(first, ".") {
			return false, "You're trying to modify a dotfile. To change " +
				"dotfiles, create a worktree for the dotfiles repo (run: " +
				"chezmoi source-path), make your changes there, run chezmoi " +
				"apply, then merge back to main."
		}
	}
	return false, "Writes are only allowed in ~/worktrees/. Create a worktree " +
		"for your target repo, do your work there, then merge back."
}

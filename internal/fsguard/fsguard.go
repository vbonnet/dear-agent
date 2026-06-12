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
//
// A Guard built directly as &Guard{Home: ...} (as tests do) classifies with
// the default policy and performs no filesystem symlink resolution, so it stays
// pure string logic. New() and NewWithPolicy enable symlink resolution of write
// targets, which closes the symlink-escape hole (a worktree symlink pointing
// into ~/src).
type Guard struct {
	Home string

	// policy is the effective classification policy. When nil, the default
	// policy derived from Home is used, so the zero-value &Guard{Home: ...}
	// remains valid.
	policy *Policy

	// resolveSymlinks enables EvalSymlinks resolution of write targets before
	// classification. Off for the pure &Guard{} form so unit tests need no real
	// filesystem; on for New()/NewWithPolicy used by hooks and the interceptor.
	resolveSymlinks bool
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
	return &Guard{Home: filepath.Clean(home), resolveSymlinks: true}
}

// NewWithPolicy builds a Guard with an explicit policy (typically from
// LoadConfig), rooted at the same resolved home as New(). Symlink resolution of
// write targets is enabled.
func NewWithPolicy(p Policy) *Guard {
	g := New()
	g.policy = &p
	return g
}

// pol returns the effective policy: the explicit one if set, else the default
// derived from Home.
func (g *Guard) pol() Policy {
	if g.policy != nil {
		return *g.policy
	}
	return DefaultPolicy(g.Home)
}

// resolve applies symlink resolution to an absolute, cleaned path when enabled,
// so a symlink that escapes the sandbox (e.g. ~/worktrees/x -> ~/src/repo) is
// classified by its real destination.
func (g *Guard) resolve(p string) string {
	if !g.resolveSymlinks {
		return p
	}
	return resolveTarget(p)
}

// resolveTarget resolves symlinks in p. If p itself does not exist (a new file
// or dir), it resolves the deepest existing ancestor and re-appends the missing
// components, so a write into a symlinked directory is still seen through the
// link. On any failure it returns p unchanged (fail safe to lexical form).
func resolveTarget(p string) string {
	if !filepath.IsAbs(p) {
		return p
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	rest := ""
	cur := p
	for {
		parent := filepath.Dir(cur)
		rest = filepath.Join(filepath.Base(cur), rest)
		if parent == cur { // reached the root without resolving
			return p
		}
		if r, err := filepath.EvalSymlinks(parent); err == nil {
			return filepath.Join(r, rest)
		}
		cur = parent
	}
}

// Resolve expands path (anchored at cwd) and applies symlink resolution,
// returning the absolute path that classification actually judges. Exposed so
// callers (the interceptor) can record the resolved target in violation logs.
func (g *Guard) Resolve(path, cwd string) string {
	return g.resolve(g.expand(path, cwd))
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
// agent scratch space and I/O plumbing that necessary operations depend on. The
// concrete set is the policy's Writable list; defaults (see DefaultPolicy)
// cover:
//
//   - /dev/...         -> pseudo-devices (/dev/null, /dev/stdout, /dev/stderr)
//   - ~/.auto-memory/  -> the agent's persistent memory store
//   - /tmp, /var/tmp   -> temporary files (macOS /tmp -> /private/tmp symlink)
//   - /var/folders/... -> macOS's default $TMPDIR (os.TempDir/t.TempDir land here)
//   - /sessions/...    -> Cowork sandbox session directories
//   - ~/beads/...      -> the canonical Beads tracker store (bd/dolt writes)
func (g *Guard) isWritableCarveout(p string) bool {
	for _, w := range g.pol().Writable {
		if under(p, w) {
			return true
		}
	}
	return false
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
	return g.ClassifyResolved(g.resolve(g.expand(path, cwd)))
}

// ClassifyResolved is Classify for a path that is already expanded and
// symlink-resolved (see Resolve). Callers that need both the resolved path and
// its verdict — e.g. to log the resolved target on a block — use Resolve +
// ClassifyResolved to avoid resolving (and hitting the disk) twice.
func (g *Guard) ClassifyResolved(p string) (allowed bool, message string) {
	pol := g.pol()
	src := filepath.Join(g.Home, "src")

	if g.isWritableCarveout(p) {
		return true, ""
	}
	if under(p, pol.WorktreesDir) {
		return true, ""
	}
	for _, prot := range pol.Protected {
		if !under(p, prot) {
			continue
		}
		if prot == src {
			repo := g.repoName(p, src)
			return false, "You're trying to write to ~/src which is protected. " +
				"Create a worktree first: git -C ~/src/" + repo +
				" worktree add ~/worktrees/" + repo + "/{branch} -b {branch}, " +
				"then make your changes there."
		}
		return false, "You're trying to write to " + prot + ", a protected " +
			"path. Do your work in a worktree under ~/worktrees/ instead, then " +
			"merge back."
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

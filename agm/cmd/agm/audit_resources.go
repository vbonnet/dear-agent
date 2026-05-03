package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
)

var auditResourcesCmd = &cobra.Command{
	Use:   "resources",
	Short: "Audit orphaned git worktrees and branches",
	Long: `Scans for git worktrees and branches not associated with any active session.

This command performs a filesystem-based audit — it does not require sessions to
have used manifest resource tracking. It works by cross-referencing:

  1. Directories under ~/worktrees/ (and custom --worktrees-dir paths)
  2. Git worktree prunable refs in known repos under ~/src/
  3. Active AGM sessions (from Dolt, with tmux fallback)

A worktree is flagged as orphaned when:
  • Its directory is under ~/worktrees/<repo>/<name> AND no active session is named <name>
  • OR git reports it as "prunable" (linked working tree no longer exists)

Use --fix to remove all flagged orphans.

Examples:
  # Audit worktrees (read-only)
  agm audit resources

  # Audit and remove all orphans
  agm audit resources --fix

  # Audit a specific worktrees base directory
  agm audit resources --worktrees-dir ~/worktrees

  # Audit additional repo directories
  agm audit resources --repos ~/src/my-project`,
	RunE: runAuditResources,
}

var (
	arFix         bool
	arWorktreeDir string
	arExtraRepos  []string
)

func init() {
	auditTrailCmd.AddCommand(auditResourcesCmd)
	auditResourcesCmd.Flags().BoolVar(&arFix, "fix", false,
		"Remove all orphaned worktrees and prune dead git refs")
	auditResourcesCmd.Flags().StringVar(&arWorktreeDir, "worktrees-dir", "",
		"Base directory containing worktrees (default: ~/worktrees)")
	auditResourcesCmd.Flags().StringArrayVar(&arExtraRepos, "repos", nil,
		"Additional repo directories to check for prunable worktree refs")
}

// orphanedWorktree describes a worktree flagged as orphaned.
type orphanedWorktree struct {
	Path        string
	Branch      string
	Repo        string
	Reason      string // "no-active-session" | "prunable-ref"
	SessionName string // set when reason is "no-active-session"
}

func runAuditResources(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	worktreesBase := arWorktreeDir
	if worktreesBase == "" {
		worktreesBase = filepath.Join(homeDir, "worktrees")
	}
	worktreesBase = expandHomePath(worktreesBase, homeDir)

	// Collect repo dirs to check for prunable refs
	repoDirs := collectRepoDirs(homeDir, arExtraRepos)

	// Get active session names for cross-reference
	activeSessions, err := getActiveSessions(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not query active sessions (%v); orphan detection will be limited\n", err)
	}

	var orphans []orphanedWorktree

	// Phase 1: filesystem scan of ~/worktrees/
	if _, statErr := os.Stat(worktreesBase); statErr == nil {
		fsOrphans := scanWorktreesDir(worktreesBase, activeSessions)
		orphans = append(orphans, fsOrphans...)
	}

	// Phase 2: prunable refs in repos
	refOrphans := scanPrunableRefs(repoDirs)
	// Deduplicate: skip if already flagged from filesystem scan
	knownPaths := make(map[string]bool, len(orphans))
	for _, o := range orphans {
		knownPaths[o.Path] = true
	}
	for _, o := range refOrphans {
		if !knownPaths[o.Path] {
			orphans = append(orphans, o)
		}
	}

	if len(orphans) == 0 {
		fmt.Println("No orphaned worktrees found.")
		return nil
	}

	// Sort for stable output
	sort.Slice(orphans, func(i, j int) bool {
		return orphans[i].Path < orphans[j].Path
	})

	// Print report
	printOrphanReport(orphans)

	if !arFix {
		fmt.Printf("\nTo remove all orphans: agm audit resources --fix\n")
		return nil
	}

	// Fix mode: remove orphans
	fmt.Println("\nRemoving orphaned worktrees...")
	removed, pruned := 0, 0

	// Remove filesystem orphans
	for _, o := range orphans {
		if o.Reason != "no-active-session" {
			continue
		}
		if err := removeOrphanWorktree(o); err != nil {
			fmt.Fprintf(os.Stderr, "  Error removing %s: %v\n", o.Path, err)
		} else {
			fmt.Printf("  Removed: %s\n", o.Path)
			removed++
		}
	}

	// Prune dead refs in repos
	pruneTargets := collectPruneTargets(orphans, repoDirs)
	for _, repoPath := range pruneTargets {
		if err := pruneWorktreeRefs(repoPath); err != nil {
			fmt.Fprintf(os.Stderr, "  Error pruning %s: %v\n", repoPath, err)
		} else {
			fmt.Printf("  Pruned dead refs in: %s\n", repoPath)
			pruned++
		}
	}

	fmt.Printf("\nRemoved %d worktree(s), pruned %d repo(s).\n", removed, pruned)
	return nil
}

// scanWorktreesDir walks ~/worktrees/<repo>/<name> and flags entries where
// <name> does not match any active session.
func scanWorktreesDir(base string, activeSessions map[string]bool) []orphanedWorktree {
	var orphans []orphanedWorktree

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}

	for _, repoEntry := range entries {
		if !repoEntry.IsDir() {
			continue
		}
		repoDir := filepath.Join(base, repoEntry.Name())

		wtEntries, err := os.ReadDir(repoDir)
		if err != nil {
			continue
		}

		for _, wtEntry := range wtEntries {
			if !wtEntry.IsDir() {
				continue
			}
			wtPath := filepath.Join(repoDir, wtEntry.Name())
			if !isGitWorktree(wtPath) {
				continue
			}

			name := wtEntry.Name()
			if activeSessions[name] {
				continue
			}

			branch := getWorktreeBranch(wtPath)
			orphans = append(orphans, orphanedWorktree{
				Path:        wtPath,
				Branch:      branch,
				SessionName: name,
				Reason:      "no-active-session",
			})
		}
	}

	return orphans
}

// scanPrunableRefs runs `git worktree list` for each repo and returns entries
// marked as prunable (linked working tree directory is gone).
func scanPrunableRefs(repoDirs []string) []orphanedWorktree {
	var orphans []orphanedWorktree

	for _, repoDir := range repoDirs {
		worktrees, err := gitpkg.ListWorktrees(repoDir)
		if err != nil || worktrees == nil {
			continue
		}

		// Check each non-main worktree for prunable status
		for _, wt := range worktrees {
			if wt.IsMain {
				continue
			}
			if _, statErr := os.Stat(wt.Path); os.IsNotExist(statErr) {
				orphans = append(orphans, orphanedWorktree{
					Path:   wt.Path,
					Branch: wt.Branch,
					Repo:   repoDir,
					Reason: "prunable-ref",
				})
			}
		}
	}

	return orphans
}

// getActiveSessions returns a set of active session names from Dolt, falling
// back to tmux session names if Dolt is unavailable.
func getActiveSessions(ctx context.Context) (map[string]bool, error) {
	active := make(map[string]bool)

	// Try Dolt first
	doltConfig, err := dolt.DefaultConfig()
	if err == nil {
		adapter, err := dolt.New(doltConfig)
		if err == nil {
			defer adapter.Close()
			sessions, err := adapter.ListActiveSessions(ctx)
			if err == nil {
				for _, s := range sessions {
					active[s] = true
				}
				return active, nil
			}
		}
	}

	// Fallback: tmux sessions
	tmuxSessions, tmuxErr := listTmuxSessionNames()
	if tmuxErr != nil {
		return active, fmt.Errorf("dolt unavailable and tmux fallback failed: %w", tmuxErr)
	}
	for _, name := range tmuxSessions {
		active[name] = true
	}
	return active, nil
}

// collectRepoDirs returns directories under ~/src/ that are git repos,
// plus any extra repos from --repos flag.
func collectRepoDirs(homeDir string, extras []string) []string {
	var repos []string

	srcDir := filepath.Join(homeDir, "src")
	if entries, err := os.ReadDir(srcDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(srcDir, e.Name())
			if isGitRepo(p) {
				repos = append(repos, p)
			}
		}
	}

	for _, extra := range extras {
		repos = append(repos, expandHomePath(extra, homeDir))
	}

	return repos
}

// collectPruneTargets returns unique repo paths that need `git worktree prune`.
func collectPruneTargets(orphans []orphanedWorktree, repoDirs []string) []string {
	seen := make(map[string]bool)
	var targets []string

	for _, o := range orphans {
		if o.Reason == "prunable-ref" && o.Repo != "" && !seen[o.Repo] {
			seen[o.Repo] = true
			targets = append(targets, o.Repo)
		}
	}
	// Also prune all known repos to catch anything not cross-referenced
	for _, r := range repoDirs {
		if !seen[r] {
			seen[r] = true
			targets = append(targets, r)
		}
	}
	return targets
}

func printOrphanReport(orphans []orphanedWorktree) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "Found %d orphaned worktree(s):\n\n", len(orphans))
	fmt.Fprintln(w, "PATH\tBRANCH\tREASON")
	for _, o := range orphans {
		branch := o.Branch
		if branch == "" {
			branch = "(detached)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", o.Path, branch, o.Reason)
	}
	_ = w.Flush()
}

func removeOrphanWorktree(o orphanedWorktree) error {
	// Try git worktree remove first (cleanest)
	if o.Repo != "" {
		if err := gitpkg.RemoveWorktree(o.Repo, o.Path, true); err == nil {
			return nil
		}
	}
	// Fallback: find the repo from the .git file and remove via git
	if gitDir, err := resolveGitDir(o.Path); err == nil && gitDir != "" {
		// gitDir is inside the repo's objects dir; find the repo root
		// Format: <repo>/.git/worktrees/<name>
		repoPath := repoFromGitDir(gitDir)
		if repoPath != "" {
			if err := gitpkg.RemoveWorktree(repoPath, o.Path, true); err == nil {
				return nil
			}
		}
	}
	// Last resort: remove directory directly (leaves stale git refs, prune will clean)
	return os.RemoveAll(o.Path)
}

func pruneWorktreeRefs(repoPath string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "prune")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, string(out))
	}
	return nil
}

// isGitWorktree returns true if the directory is a git worktree (has a .git file,
// not directory — worktrees have a .git file that points to the main repo).
func isGitWorktree(path string) bool {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}
	// Main worktree has a .git directory; linked worktrees have a .git file
	return !info.IsDir()
}

func isGitRepo(path string) bool {
	gitPath := filepath.Join(path, ".git")
	_, err := os.Stat(gitPath)
	return err == nil
}

// getWorktreeBranch reads the branch of a git worktree by parsing its HEAD file.
func getWorktreeBranch(worktreePath string) string {
	headFile := filepath.Join(worktreePath, ".git")
	// For a linked worktree, .git is a file containing "gitdir: <path>"
	data, err := os.ReadFile(headFile) //nolint:gosec // path is constructed from controlled base dir
	if err != nil {
		return ""
	}
	// Parse "gitdir: /path/to/repo/.git/worktrees/<name>"
	gitdirPath := filepath.Clean(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir: "))
	headPath := filepath.Join(gitdirPath, "HEAD")
	headData, err := os.ReadFile(headPath) //nolint:gosec // path derived from git's own gitdir file
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(string(headData))
	return strings.TrimPrefix(ref, "ref: refs/heads/")
}

func resolveGitDir(worktreePath string) (string, error) {
	gitFile := filepath.Join(worktreePath, ".git")
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir: "), nil
}

// repoFromGitDir walks up from a <repo>/.git/worktrees/<name> path to find the repo root.
func repoFromGitDir(gitDir string) string {
	// gitDir looks like: /path/to/repo/.git/worktrees/branchname
	// Walk up until we find a directory whose .git is a directory (not a file)
	dir := gitDir
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		// Check if parent itself is the .git dir
		if filepath.Base(dir) == ".git" {
			// parent is the repo root
			return parent
		}
		dir = parent
	}
	return ""
}

// listTmuxSessionNames returns the names of all active tmux sessions.
func listTmuxSessionNames() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func expandHomePath(path, homeDir string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

// agmSessionNamePattern matches Docker-style AGM session names: adjective-name-hexid
var agmSessionNamePattern = regexp.MustCompile(`^[a-z]+-[a-z]+-[0-9a-f]{6,}$`)

// isSessionName returns true if the name looks like an AGM-generated session name.
func isSessionName(name string) bool {
	return agmSessionNamePattern.MatchString(name)
}

// walkWorktreesDir walks a directory non-recursively at depth 2 (repo/worktree).
// Exported for testing.
func walkWorktreesDir(base string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return filepath.SkipDir // skip inaccessible entries
		}
		rel, _ := filepath.Rel(base, path)
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if d.IsDir() && depth > 2 {
			return filepath.SkipDir
		}
		if !d.IsDir() || depth != 2 {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	return paths, err
}

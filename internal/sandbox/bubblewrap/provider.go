// Package bubblewrap provides a sandbox implementation using Bubblewrap (bwrap).
// Bubblewrap creates isolated namespaces without requiring root privileges.
package bubblewrap

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// Provider implements sandbox.Provider using Bubblewrap.
// Bubblewrap creates isolated namespaces using bind mounts and user namespaces.
// Works without root privileges or kernel-specific features.
type Provider struct {
	mu        sync.RWMutex
	sandboxes map[string]*sandbox.Sandbox
	// ShareNetwork controls whether the sandbox shares the host network namespace.
	// Default is false (network is isolated via --unshare-net).
	ShareNetwork bool
}

// NewProvider creates a new Bubblewrap provider.
func NewProvider() *Provider {
	return &Provider{
		sandboxes: make(map[string]*sandbox.Sandbox),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "bubblewrap"
}

// Create provisions a new isolated sandbox using Bubblewrap.
//
// The sandbox is created as a git worktree of the first git repo found in
// LowerDirs. This gives workers full read-write access to repo content on
// an isolated branch, with a proper .git directory so git commit/push works.
//
// If no git repo is found in LowerDirs, falls back to the symlink approach
// (read-only access via symlinks into the source repos).
//
// Bubblewrap self-test is still run to validate namespace support.
func (p *Provider) Create(ctx context.Context, req sandbox.SandboxRequest) (*sandbox.Sandbox, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := p.validateRequest(req); err != nil {
		return nil, err
	}

	// Check if bwrap is available
	if err := p.checkBubblewrapInstalled(); err != nil {
		return nil, err
	}

	// Create directory structure
	upperDir := filepath.Join(req.WorkspaceDir, "upper")
	workDir := filepath.Join(req.WorkspaceDir, "work")
	mergedDir := filepath.Join(req.WorkspaceDir, "merged")

	if err := p.createDirectories(upperDir, workDir, mergedDir); err != nil {
		return nil, sandbox.WrapError(sandbox.ErrCodePermissionDenied,
			"failed to create sandbox directories", err)
	}

	// Determine per-sandbox network setting (request overrides provider default)
	shareNetwork := p.ShareNetwork || req.ShareNetwork

	// Try to create the merged dir as a git worktree for full read-write access.
	// This replaces the symlink approach which only gave read access (writes
	// through symlinks modify the source repo, and new files stay in the sandbox
	// dir with no .git -- so git commit doesn't work).
	worktreeRepo, worktreeCreated := p.tryCreateWorktree(req.LowerDirs, req.SessionID, mergedDir, req.TargetRepo)
	if !worktreeCreated {
		// Fallback: populate merged directory with symlinks to lower dir contents.
		fmt.Fprintf(os.Stderr, "bubblewrap: no git repo in lower dirs, falling back to symlinks\n")
		if err := p.populateMergedDir(req.LowerDirs, mergedDir); err != nil {
			_ = p.cleanupDirectories(upperDir, workDir, mergedDir)
			return nil, sandbox.WrapError(sandbox.ErrCodeMountFailed,
				"failed to populate merged directory with repo symlinks", err)
		}
	}

	// Test bubblewrap functionality
	if err := p.testBubblewrap(req.LowerDirs, upperDir, mergedDir, shareNetwork); err != nil {
		if worktreeCreated {
			_ = p.removeWorktree(worktreeRepo, mergedDir)
		}
		_ = p.cleanupDirectories(upperDir, workDir, mergedDir)
		return nil, err
	}

	// Write secrets to upperdir/.env if provided
	if len(req.Secrets) > 0 {
		if err := p.writeSecrets(upperDir, req.Secrets); err != nil {
			if worktreeCreated {
				_ = p.removeWorktree(worktreeRepo, mergedDir)
			}
			_ = p.cleanupDirectories(upperDir, workDir, mergedDir)
			return nil, sandbox.WrapError(sandbox.ErrCodePermissionDenied,
				"failed to write secrets", err)
		}
	}

	// Build cleanup function that removes the worktree before cleaning dirs
	cleanupFn := func() error {
		if worktreeCreated {
			if err := p.removeWorktree(worktreeRepo, mergedDir); err != nil {
				fmt.Fprintf(os.Stderr, "bubblewrap: warning: failed to remove worktree: %v\n", err)
			}
		}
		return p.cleanup(upperDir, workDir, mergedDir)
	}

	// Create sandbox metadata
	sb := &sandbox.Sandbox{
		ID:         req.SessionID,
		MergedPath: mergedDir,
		UpperPath:  upperDir,
		WorkPath:   workDir,
		Type:       p.Name(),
		CreatedAt:  time.Now(),
		CleanupFunc: cleanupFn,
	}

	p.mu.Lock()
	p.sandboxes[sb.ID] = sb
	p.mu.Unlock()
	return sb, nil
}

// tryCreateWorktree attempts to create a git worktree in mergedDir from the
// first git repo found in lowerDirs. Returns the repo path and true on success.
// If targetRepo is set, it is used directly instead of scanning lowerDirs.
func (p *Provider) tryCreateWorktree(lowerDirs []string, sessionID, mergedDir, targetRepo string) (string, bool) {
	var repoPath string
	if targetRepo != "" && p.isGitRepo(targetRepo) {
		repoPath = targetRepo
		fmt.Fprintf(os.Stderr, "bubblewrap: using explicit target repo: %s\n", repoPath)
	} else {
		// Find the first git repo in lowerDirs (or resolve symlinks to find one)
		repoPath = p.findGitRepo(lowerDirs)
	}
	if repoPath == "" {
		return "", false
	}

	// The mergedDir already exists (created by createDirectories). Git worktree
	// add requires the target to not exist, so remove it first.
	if err := os.RemoveAll(mergedDir); err != nil {
		fmt.Fprintf(os.Stderr, "bubblewrap: failed to remove mergedDir for worktree: %v\n", err)
		return "", false
	}

	// Create branch name from session ID (sanitized for git)
	branchName := "agm/" + sessionID

	fmt.Fprintf(os.Stderr, "bubblewrap: creating git worktree from %s at %s (branch: %s)\n",
		repoPath, mergedDir, branchName)

	if err := p.addWorktree(repoPath, mergedDir, branchName); err != nil {
		fmt.Fprintf(os.Stderr, "bubblewrap: git worktree add failed: %v\n", err)
		// Re-create mergedDir so the fallback symlink approach has somewhere to write
		_ = os.MkdirAll(mergedDir, 0755)
		return "", false
	}

	fmt.Fprintf(os.Stderr, "bubblewrap: git worktree created successfully\n")
	return repoPath, true
}

// findGitRepo finds the first git repository among the lower directories.
// It resolves symlinks and checks for .git directories.
//
// When a lowerDir is a sandbox merged directory containing symlinks to a
// source repo's files, this method traces through those symlinks to find
// the original git repository. This is critical for worker sessions
// spawned from within a parent sandbox -- their workDir is the parent's
// merged directory, not the actual source repo.
func (p *Provider) findGitRepo(lowerDirs []string) string {
	for _, dir := range lowerDirs {
		// Resolve symlinks to find actual paths
		resolved := dir
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			resolved = r
		}

		if p.isGitRepo(resolved) {
			return resolved
		}

		// Read entries for further inspection
		entries, err := os.ReadDir(resolved)
		if err != nil {
			continue
		}

		// Strategy 1: Follow symlinks in the directory to discover the source
		// repo. In a sandbox merged dir, entries are symlinks like:
		//   go.mod -> /home/user/src/ws/oss/repos/ai-tools/go.mod
		// Resolving that symlink and walking up to find .git gives us the repo.
		if repo := p.resolveRepoFromSymlinks(resolved, entries); repo != "" {
			return repo
		}

		// Strategy 2: Check subdirectories (non-dotfiles) for git repos.
		// Uses os.Stat (follows symlinks) instead of DirEntry.IsDir() to
		// correctly handle symlinks to directories.
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			subPath := filepath.Join(resolved, e.Name())
			// Use os.Stat to follow symlinks
			info, err := os.Stat(subPath)
			if err != nil || !info.IsDir() {
				continue
			}
			if r, err := filepath.EvalSymlinks(subPath); err == nil {
				subPath = r
			}
			if p.isGitRepo(subPath) {
				return subPath
			}
		}
	}

	// Fallback: scan well-known workspace locations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Check AGM config for workspace roots
	repos := p.findReposFromAGMConfig(homeDir)
	if repo := p.preferRepoWithGoMod(repos); repo != "" {
		return repo
	}

	return ""
}

// preferRepoWithGoMod returns the first git repo that contains go.mod at its
// root. This ensures the monorepo (ai-tools) is preferred over auxiliary repos
// (ai-conversation-logs) when scanning alphabetically. Falls back to first git
// repo if none has go.mod.
func (p *Provider) preferRepoWithGoMod(repos []string) string {
	var firstGitRepo string
	for _, repo := range repos {
		if !p.isGitRepo(repo) {
			continue
		}
		if firstGitRepo == "" {
			firstGitRepo = repo
		}
		goModPath := filepath.Join(repo, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			fmt.Fprintf(os.Stderr, "bubblewrap: preferring repo with go.mod: %s\n", repo)
			return repo
		}
	}
	return firstGitRepo
}

// resolveRepoFromSymlinks traces symlinks in a directory back to their
// source repository. This handles the case where a sandbox merged dir
// contains symlinks to files inside a git repo -- we resolve the symlink
// target and walk up the directory tree to find the git root.
//
// When multiple symlinks resolve to different repos, we prefer the repo
// that is referenced by project-defining files (go.mod, AGENTS.md, Makefile,
// etc.) rather than returning whichever repo is found first alphabetically.
// This prevents the sandbox from selecting the wrong repo when multiple
// repos are present (e.g., ai-conversation-logs before ai-tools).
func (p *Provider) resolveRepoFromSymlinks(dir string, entries []os.DirEntry) string {
	// Project-defining files that indicate the "main" repo for a workspace.
	// These are checked in priority order -- the first match wins.
	projectMarkers := []string{
		"go.mod", "Cargo.toml", "package.json", "pyproject.toml",
		"Makefile", "AGENTS.md",
	}

	// Track which repo each project marker symlink resolves to.
	repoForMarker := make(map[string]string) // marker name -> repo path
	var firstRepo string

	for _, e := range entries {
		entryPath := filepath.Join(dir, e.Name())

		// Only follow symlinks
		linfo, err := os.Lstat(entryPath)
		if err != nil || linfo.Mode()&os.ModeSymlink == 0 {
			continue
		}

		// Resolve the symlink to its absolute target
		target, err := filepath.EvalSymlinks(entryPath)
		if err != nil {
			continue
		}

		// Walk up the target path looking for a git root
		repo := p.findGitRootFromPath(target)
		if repo == "" {
			continue
		}

		// Remember the first repo found (fallback)
		if firstRepo == "" {
			firstRepo = repo
		}

		// Check if this entry is a project-defining marker
		for _, marker := range projectMarkers {
			if e.Name() == marker {
				repoForMarker[marker] = repo
				break
			}
		}
	}

	// Prefer the repo referenced by the highest-priority project marker.
	for _, marker := range projectMarkers {
		if repo, ok := repoForMarker[marker]; ok {
			fmt.Fprintf(os.Stderr, "bubblewrap: resolved source repo via project marker %s -> %s\n",
				marker, repo)
			return repo
		}
	}

	// Fallback to the first repo found via any symlink.
	if firstRepo != "" {
		fmt.Fprintf(os.Stderr, "bubblewrap: resolved source repo via first symlink -> %s\n",
			firstRepo)
	}
	return firstRepo
}

// findGitRootFromPath walks up from path until it finds a directory that
// is a git repository (contains .git).
func (p *Provider) findGitRootFromPath(path string) string {
	// Start from the directory containing path (or path itself if it is a dir)
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}

	for {
		if p.isGitRepo(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return ""
		}
		dir = parent
	}
}

// isGitRepo checks if a directory is inside a git repository by running
// git rev-parse.
func (p *Provider) isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// addWorktree creates a new git worktree at the given path on a new branch.
// If the branch already exists (e.g., from a previous session that was not
// properly cleaned up), the existing branch is deleted first and re-created.
func (p *Provider) addWorktree(repoPath, worktreePath, branch string) error {
	args := []string{"-C", repoPath, "worktree", "add", worktreePath, "-b", branch}
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If the branch already exists, delete it and retry.
		if strings.Contains(string(output), "already exists") {
			fmt.Fprintf(os.Stderr, "bubblewrap: branch %s already exists, deleting and retrying\n", branch)
			delCmd := exec.Command("git", "-C", repoPath, "branch", "-D", branch)
			if delOut, delErr := delCmd.CombinedOutput(); delErr != nil {
				return fmt.Errorf("git worktree add failed (branch exists, delete failed): %w\nDelete output: %s\nOriginal output: %s",
					delErr, string(delOut), string(output))
			}
			// Retry worktree add
			retryCmd := exec.Command("git", args...)
			if retryOut, retryErr := retryCmd.CombinedOutput(); retryErr != nil {
				return fmt.Errorf("git worktree add failed on retry: %w\nOutput: %s", retryErr, string(retryOut))
			}
			return nil
		}
		return fmt.Errorf("git worktree add failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// removeWorktree removes a git worktree using force mode.
func (p *Provider) removeWorktree(repoPath, worktreePath string) error {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "remove", "--force", worktreePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// Destroy tears down the sandbox and cleans up resources.
func (p *Provider) Destroy(ctx context.Context, id string) error {
	p.mu.Lock()
	sb, exists := p.sandboxes[id]
	if !exists {
		p.mu.Unlock()
		return nil // Idempotent
	}
	delete(p.sandboxes, id)
	p.mu.Unlock()

	if sb.CleanupFunc != nil {
		if err := sb.CleanupFunc(); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks if a sandbox exists and is healthy.
func (p *Provider) Validate(ctx context.Context, id string) error {
	p.mu.RLock()
	sb, exists := p.sandboxes[id]
	p.mu.RUnlock()
	if !exists {
		return sandbox.NewSandboxNotFoundError(id)
	}

	if _, err := os.Stat(sb.MergedPath); os.IsNotExist(err) {
		return sandbox.NewSandboxNotFoundError(id)
	}

	return nil
}

// validateRequest checks if the request is valid.
func (p *Provider) validateRequest(req sandbox.SandboxRequest) error {
	if req.SessionID == "" {
		return sandbox.NewInvalidConfigError("SessionID", "must not be empty")
	}

	if len(req.LowerDirs) == 0 {
		return sandbox.NewInvalidConfigError("LowerDirs", "at least one lower directory is required")
	}

	for _, dir := range req.LowerDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return sandbox.NewRepoNotFoundError(dir)
		}
	}

	if req.WorkspaceDir == "" {
		return sandbox.NewInvalidConfigError("WorkspaceDir", "must not be empty")
	}

	return nil
}

// checkBubblewrapInstalled verifies bwrap is available.
func (p *Provider) checkBubblewrapInstalled() error {
	_, err := exec.LookPath("bwrap")
	if err != nil {
		return sandbox.NewError(sandbox.ErrCodeUnsupportedPlatform,
			"bubblewrap (bwrap) not found in PATH - install with: brew install bubblewrap")
	}
	return nil
}

// testBubblewrap runs a quick test to verify bubblewrap works correctly.
func (p *Provider) testBubblewrap(lowerDirs []string, upperDir, _ string, shareNetwork bool) error { //nolint:unparam // mergedDir used in future overlay tests
	// Create a simple test file
	testFile := filepath.Join(upperDir, ".bwrap-test")
	testContent := "bubblewrap-test"
	if err := os.WriteFile(testFile, []byte(testContent), 0600); err != nil {
		return sandbox.WrapError(sandbox.ErrCodePermissionDenied,
			"failed to create test file", err)
	}
	defer os.Remove(testFile)

	// Build bwrap command to test isolation
	args := []string{
		// System directories (required for bash and commands)
		"--ro-bind", "/usr", "/usr",
		"--ro-bind", "/lib", "/lib",
		"--ro-bind", "/bin", "/bin",
	}

	// Add /lib64 and /sbin if they exist
	if _, err := os.Stat("/lib64"); err == nil {
		args = append(args, "--ro-bind", "/lib64", "/lib64")
	}
	if _, err := os.Stat("/sbin"); err == nil {
		args = append(args, "--ro-bind", "/sbin", "/sbin")
	}

	// Bind lower directories as read-only
	for i, dir := range lowerDirs {
		args = append(args,
			"--ro-bind", dir, fmt.Sprintf("/sandbox/lower%d", i))
	}

	// Bind upper directory as writable
	args = append(args,
		"--bind", upperDir, "/sandbox/upper",
		"--tmpfs", "/tmp",
		"--proc", "/proc",
		"--dev", "/dev",
		"--unshare-all",
	)

	// Only share host network if explicitly configured; default is isolated
	if shareNetwork {
		args = append(args, "--share-net")
	}

	args = append(args,
		"--die-with-parent",
		"/bin/sh", "-c", "test -f /sandbox/upper/.bwrap-test && echo ok")

	cmd := exec.Command("bwrap", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return sandbox.WrapError(sandbox.ErrCodeMountFailed,
			fmt.Sprintf("bubblewrap test failed: %s", string(output)), err)
	}

	if !strings.Contains(string(output), "ok") {
		return sandbox.NewError(sandbox.ErrCodeMountFailed,
			"bubblewrap test did not produce expected output")
	}

	return nil
}

// createDirectories creates the required directory structure.
func (p *Provider) createDirectories(upperDir, workDir, mergedDir string) error {
	dirs := []string{upperDir, workDir, mergedDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// cleanupDirectories removes sandbox directories.
func (p *Provider) cleanupDirectories(upperDir, workDir, mergedDir string) error {
	dirs := []string{mergedDir, workDir, upperDir}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

// cleanup performs full cleanup.
func (p *Provider) cleanup(upperDir, workDir, mergedDir string) error {
	if err := p.cleanupDirectories(upperDir, workDir, mergedDir); err != nil {
		return sandbox.NewCleanupFailedError(mergedDir, err)
	}
	return nil
}

// populateMergedDir creates symlinks in mergedDir pointing to each top-level
// entry from all lower directories. This gives the worker process read access
// to repository content. Later lower dirs take priority (matching OverlayFS
// semantics where the first lowerdir listed has highest priority).
//
// If the lower directories are effectively empty (e.g., a parent sandbox's
// merged dir that was created before content was populated), this method
// falls back to resolving the actual workspace repos from AGM configuration.
func (p *Provider) populateMergedDir(lowerDirs []string, mergedDir string) error {
	// Process lower dirs in reverse order so earlier entries (higher priority)
	// overwrite later ones, matching OverlayFS lowerdir semantics.
	for i := len(lowerDirs) - 1; i >= 0; i-- {
		dir := lowerDirs[i]
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("failed to read lower dir %s: %w", dir, err)
		}

		for _, entry := range entries {
			name := entry.Name()
			linkPath := filepath.Join(mergedDir, name)
			targetPath := filepath.Join(dir, name)

			// Resolve symlinks so we point to actual repo paths, not
			// intermediate symlinks in a parent sandbox's merged dir.
			// This prevents chained symlinks when a child sandbox is
			// created from within a parent sandbox's merged directory.
			if resolved, err := filepath.EvalSymlinks(targetPath); err == nil {
				targetPath = resolved
			}

			// Remove existing symlink if present (higher-priority dir overrides)
			if _, err := os.Lstat(linkPath); err == nil {
				if err := os.Remove(linkPath); err != nil {
					return fmt.Errorf("failed to remove existing entry %s: %w", linkPath, err)
				}
			}

			if err := os.Symlink(targetPath, linkPath); err != nil {
				return fmt.Errorf("failed to create symlink %s -> %s: %w", linkPath, targetPath, err)
			}
		}
	}

	// Check if the merged dir is effectively empty (only dotfiles/metadata).
	// This happens when a child sandbox is created from a parent sandbox whose
	// merged dir was never populated (parent created before the fix).
	if p.isMergedDirEffectivelyEmpty(mergedDir) {
		fmt.Fprintf(os.Stderr, "bubblewrap: merged dir effectively empty, attempting workspace repo fallback\n")
		if err := p.fallbackToWorkspaceRepos(lowerDirs, mergedDir); err != nil {
			fmt.Fprintf(os.Stderr, "bubblewrap: workspace repo fallback failed: %v\n", err)
			// Not fatal -- we still have whatever was in lowerDirs
		}
	}

	// Log what was linked for debugging
	entries, _ := os.ReadDir(mergedDir)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	fmt.Fprintf(os.Stderr, "bubblewrap: populated merged dir with %d entries from %d lower dir(s)\n",
		len(names), len(lowerDirs))

	return nil
}

// isMergedDirEffectivelyEmpty returns true if mergedDir contains only dotfiles
// and metadata (e.g., .claude, CLAUDE.md) but no actual repository content.
func (p *Provider) isMergedDirEffectivelyEmpty(mergedDir string) bool {
	entries, err := os.ReadDir(mergedDir)
	if err != nil {
		return true
	}
	for _, e := range entries {
		name := e.Name()
		// Skip dotfiles and known metadata
		if strings.HasPrefix(name, ".") || name == "CLAUDE.md" {
			continue
		}
		// Found real content
		return false
	}
	return true
}

// fallbackToWorkspaceRepos attempts to find and symlink actual workspace repos
// when the lower dirs are effectively empty (e.g., empty parent sandbox).
//
// Resolution strategy:
//  1. Parse ~/.agm/config.yaml for workspace roots, scan {root}/repos/
//  2. Detect sandbox-within-sandbox pattern and find original workspace
func (p *Provider) fallbackToWorkspaceRepos(lowerDirs []string, mergedDir string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot get home dir: %w", err)
	}

	// Strategy 1: Read ~/.agm/config.yaml for workspace roots
	repoDirs := p.findReposFromAGMConfig(homeDir)
	if len(repoDirs) > 0 {
		return p.symlinkRepoContents(repoDirs, mergedDir)
	}

	// Strategy 2: Detect sandbox-in-sandbox pattern.
	// If lowerDir is ~/.agm/sandboxes/*/merged, try common workspace paths.
	sandboxPattern := filepath.Join(homeDir, ".agm", "sandboxes")
	for _, dir := range lowerDirs {
		if strings.HasPrefix(dir, sandboxPattern) {
			fmt.Fprintf(os.Stderr, "bubblewrap: detected sandbox-in-sandbox (lower=%s)\n", dir)
			break
		}
	}

	// Strategy 3: Scan well-known workspace locations
	candidates := []string{
		filepath.Join(homeDir, "src", "ws", "oss", "repos"),
		filepath.Join(homeDir, "src", "ws"),
		filepath.Join(homeDir, "src"),
	}
	for _, candidate := range candidates {
		repos := p.scanForRepos(candidate)
		if len(repos) > 0 {
			fmt.Fprintf(os.Stderr, "bubblewrap: found %d repos under %s\n", len(repos), candidate)
			return p.symlinkRepoContents(repos, mergedDir)
		}
	}

	return fmt.Errorf("no workspace repos found in any fallback location")
}

// findReposFromAGMConfig parses ~/.agm/config.yaml (lightweight, no YAML dep)
// to find workspace roots, then scans for repos under {root}/repos/.
func (p *Provider) findReposFromAGMConfig(homeDir string) []string {
	cfgPath := filepath.Join(homeDir, ".agm", "config.yaml")
	f, err := os.Open(cfgPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var roots []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Look for "root: /path/to/workspace" lines
		if strings.HasPrefix(line, "root:") {
			root := strings.TrimSpace(strings.TrimPrefix(line, "root:"))
			root = strings.ReplaceAll(root, "~", homeDir)
			if root != "" {
				roots = append(roots, root)
			}
		}
	}

	// For each workspace root, scan {root}/repos/ for actual repos
	var allRepos []string
	for _, root := range roots {
		reposDir := filepath.Join(root, "repos")
		repos := p.scanForRepos(reposDir)
		allRepos = append(allRepos, repos...)
	}

	return allRepos
}

// scanForRepos returns directories under parentDir that look like repos
// (contain .git, go.mod, package.json, or similar markers).
func (p *Provider) scanForRepos(parentDir string) []string {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return nil
	}

	var repos []string
	repoMarkers := []string{".git", "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "Makefile"}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		dirPath := filepath.Join(parentDir, name)
		for _, marker := range repoMarkers {
			if _, err := os.Stat(filepath.Join(dirPath, marker)); err == nil {
				repos = append(repos, dirPath)
				break
			}
		}
	}
	return repos
}

// symlinkRepoContents creates symlinks in mergedDir for each top-level entry
// across all repo directories.
func (p *Provider) symlinkRepoContents(repoDirs []string, mergedDir string) error {
	linked := 0
	for _, repoDir := range repoDirs {
		entries, err := os.ReadDir(repoDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bubblewrap: skipping repo %s: %v\n", repoDir, err)
			continue
		}

		for _, entry := range entries {
			name := entry.Name()
			linkPath := filepath.Join(mergedDir, name)
			targetPath := filepath.Join(repoDir, name)

			// Resolve symlinks to actual paths
			if resolved, err := filepath.EvalSymlinks(targetPath); err == nil {
				targetPath = resolved
			}

			// Don't overwrite existing entries (original lowerDir has priority)
			if _, err := os.Lstat(linkPath); err == nil {
				continue
			}

			if err := os.Symlink(targetPath, linkPath); err != nil {
				fmt.Fprintf(os.Stderr, "bubblewrap: failed to symlink %s: %v\n", name, err)
				continue
			}
			linked++
		}
	}

	if linked == 0 {
		return fmt.Errorf("no entries linked from %d repo dirs", len(repoDirs))
	}

	fmt.Fprintf(os.Stderr, "bubblewrap: fallback linked %d entries from %d repos\n", linked, len(repoDirs))
	return nil
}

// writeSecrets writes secrets to upperdir/.env file.
func (p *Provider) writeSecrets(upperDir string, secrets map[string]string) error {
	envFile := filepath.Join(upperDir, ".env")

	var buf strings.Builder
	buf.WriteString("# Auto-generated by AGM sandbox\n")
	buf.WriteString("# DO NOT COMMIT THIS FILE\n\n")

	for key, value := range secrets {
		expandedValue := os.ExpandEnv(value)
		buf.WriteString(fmt.Sprintf("%s=%s\n", key, expandedValue))
	}

	if err := os.WriteFile(envFile, []byte(buf.String()), 0600); err != nil {
		return err
	}

	return nil
}

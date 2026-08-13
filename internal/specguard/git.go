package specguard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type sourceFile struct {
	body []byte
}

type change struct {
	path   string
	status string
}

type dirtyWorktreePath struct {
	path   string
	status string
}

type gitSnapshot struct {
	base                string
	head                string
	identity            string
	files               map[string]sourceFile
	implementationDirs  map[string]bool
	implementationPaths map[string]bool
	changed             []change
}

type gitClient struct {
	executable   string
	limits       guardLimits
	repository   *repositoryIdentity
	afterCommand func()
}

type filesystemIdentity struct {
	path             string
	info             os.FileInfo
	useLstat         bool
	requireDirectory bool
	compareContents  bool
	expectAbsent     bool
	contents         []byte
}

type repositoryIdentity struct {
	root      string
	gitDir    string
	commonDir string
	paths     []filesystemIdentity
	selectors []filesystemIdentity
	finalized bool
}

type treeEntry struct {
	path       string
	mode       string
	objectType string
	oid        string
	stage      int
}

func resolveGitExecutable() (string, *guardFailure) {
	if failure := descendantTerminationAdmission(runtime.GOOS); failure != nil {
		return "", failure
	}
	executable, err := exec.LookPath("git")
	if err != nil {
		return "", fail("git-unavailable", "", "Git executable was not found on PATH")
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return "", fail("git-unavailable", "", "Git executable path could not be resolved")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fail("git-unavailable", "", "Git executable symlinks could not be resolved")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", fail("git-unavailable", "", "Git executable must resolve to a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fail("git-unavailable", "", "Git executable is not executable")
	}
	return resolved, nil
}

func descendantTerminationAdmission(goos string) *guardFailure {
	if goos != "darwin" && goos != "linux" {
		return fail("unsupported-platform", "", fmt.Sprintf("guard requires descendant-process termination and therefore fails closed before Git on %s", goos))
	}
	return nil
}

//nolint:gocyclo // Sequential discovery and identity checkpoints keep admission fail closed and auditable.
func (git gitClient) admitRepository(ctx context.Context, repository string) (*repositoryIdentity, *guardFailure) {
	absolute, err := filepath.Abs(repository)
	if err != nil {
		return nil, fail("invalid-repository", "", "repository path could not be made absolute")
	}
	resolvedInput, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fail("invalid-repository", "", "repository path does not resolve")
	}
	info, err := os.Stat(resolvedInput)
	if err != nil || !info.IsDir() {
		return nil, fail("invalid-repository", "", "repository path must resolve to a directory")
	}
	inputIdentity, failure := captureDirectoryIdentities(resolvedInput)
	if failure != nil {
		return nil, failure
	}
	if failure := validateFilesystemIdentities(inputIdentity); failure != nil {
		return nil, failure
	}
	output, commandFailure := git.run(ctx, "", nil, min(int64(8192), git.limits.maxGitOutput),
		"-C", resolvedInput, "rev-parse", "--show-toplevel")
	if failure := validateFilesystemIdentities(inputIdentity); failure != nil {
		return nil, failure
	}
	if commandFailure != nil {
		return nil, commandFailure.withCode("invalid-repository")
	}
	rootText := strings.TrimSuffix(string(output), "\n")
	rootText = strings.TrimSuffix(rootText, "\r")
	if rootText == "" || strings.ContainsAny(rootText, "\x00\r\n") {
		return nil, fail("invalid-repository", "", "Git returned an invalid repository root")
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(rootText))
	if err != nil || !filepath.IsAbs(root) {
		return nil, fail("invalid-repository", "", "Git repository root could not be canonicalized")
	}
	relative, err := filepath.Rel(root, resolvedInput)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fail("repository-path-escape", "", "requested path is outside the Git-reported worktree root")
	}
	rootPaths, failure := captureDirectoryIdentities(root)
	if failure != nil {
		return nil, failure
	}
	gitSelector, failure := captureGitSelector(filepath.Join(root, ".git"))
	if failure != nil {
		return nil, failure
	}
	rootIdentity := &repositoryIdentity{root: root, paths: rootPaths, selectors: []filesystemIdentity{gitSelector}}
	admissionGit := git
	admissionGit.repository = rootIdentity
	gitDirOutput, failure := admissionGit.run(ctx, root, nil, min(int64(8192), git.limits.maxGitOutput),
		"rev-parse", "--absolute-git-dir")
	if failure != nil {
		return nil, failure.withCode("invalid-repository")
	}
	gitDir, failure := canonicalGitDirectory(gitDirOutput)
	if failure != nil {
		return nil, failure
	}
	commonDir, commonSelector, failure := captureCommonDirectory(gitDir)
	if failure != nil {
		return nil, failure
	}
	paths, failure := captureDirectoryIdentities(root, gitDir, commonDir)
	if failure != nil {
		return nil, failure
	}
	identity := &repositoryIdentity{
		root:      root,
		gitDir:    gitDir,
		commonDir: commonDir,
		paths:     paths,
		selectors: []filesystemIdentity{gitSelector, commonSelector},
		finalized: true,
	}
	admissionGit.repository = identity
	inside, failure := admissionGit.run(ctx, root, nil, 128, "rev-parse", "--is-inside-work-tree")
	if failure != nil {
		return nil, failure.withCode("invalid-repository")
	}
	if strings.TrimSpace(string(inside)) != "true" {
		return nil, fail("invalid-repository", "", "guard requires a non-bare Git worktree")
	}
	return identity, nil
}

func canonicalGitDirectory(output []byte) (string, *guardFailure) {
	value := strings.TrimSuffix(string(output), "\n")
	value = strings.TrimSuffix(value, "\r")
	if value == "" || strings.ContainsAny(value, "\x00\r\n") || !filepath.IsAbs(value) {
		return "", fail("invalid-repository", "", "Git returned an invalid Git directory")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(value))
	if err != nil || !filepath.IsAbs(resolved) {
		return "", fail("invalid-repository", "", "Git directory could not be canonicalized")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fail("invalid-repository", "", "Git directory must resolve to a directory")
	}
	return resolved, nil
}

func captureCommonDirectory(gitDir string) (string, filesystemIdentity, *guardFailure) {
	selectorPath := filepath.Join(gitDir, "commondir")
	selector, exists, failure := captureOptionalRegularSelector(selectorPath)
	if failure != nil {
		return "", filesystemIdentity{}, failure
	}
	if !exists {
		return gitDir, selector, nil
	}
	value := strings.TrimSuffix(string(selector.contents), "\n")
	value = strings.TrimSuffix(value, "\r")
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", filesystemIdentity{}, fail("invalid-repository", "", "Git common-directory selector is malformed")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(gitDir, value)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(value))
	if err != nil || !filepath.IsAbs(resolved) {
		return "", filesystemIdentity{}, fail("invalid-repository", "", "Git common directory could not be canonicalized")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", filesystemIdentity{}, fail("invalid-repository", "", "Git common directory must resolve to a directory")
	}
	return resolved, selector, nil
}

func captureDirectoryIdentities(targets ...string) ([]filesystemIdentity, *guardFailure) {
	identities := make([]filesystemIdentity, 0)
	seen := make(map[string]bool)
	for _, target := range targets {
		current := filepath.Clean(target)
		for {
			if !seen[current] {
				resolved, err := filepath.EvalSymlinks(current)
				if err != nil || filepath.Clean(resolved) != current {
					return nil, fail("invalid-repository", "", "repository identity path could not be pinned canonically")
				}
				info, err := os.Stat(current)
				if err != nil || !info.IsDir() {
					return nil, fail("invalid-repository", "", "repository identity path must remain a directory")
				}
				identities = append(identities, filesystemIdentity{
					path:             current,
					info:             info,
					requireDirectory: true,
				})
				seen[current] = true
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	return identities, nil
}

const maxGitSelectorBytes = int64(8192)

func captureGitSelector(selectorPath string) (filesystemIdentity, *guardFailure) {
	info, err := os.Lstat(selectorPath)
	if err != nil {
		return filesystemIdentity{}, fail("invalid-repository", "", "worktree Git selector could not be pinned")
	}
	if !info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return filesystemIdentity{}, fail("invalid-repository", "", "worktree Git selector has an unsupported filesystem type")
	}
	identity := filesystemIdentity{path: selectorPath, info: info, useLstat: true}
	if info.Mode().IsRegular() {
		contents, failure := readBoundedIdentityFile(selectorPath, maxGitSelectorBytes, info)
		if failure != nil {
			return filesystemIdentity{}, failure
		}
		identity.compareContents = true
		identity.contents = contents
	}
	return identity, nil
}

func captureOptionalRegularSelector(selectorPath string) (filesystemIdentity, bool, *guardFailure) {
	info, err := os.Lstat(selectorPath)
	if os.IsNotExist(err) {
		return filesystemIdentity{path: selectorPath, useLstat: true, expectAbsent: true}, false, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return filesystemIdentity{}, false, fail("invalid-repository", "", "Git common-directory selector must be a regular file when present")
	}
	contents, failure := readBoundedIdentityFile(selectorPath, maxGitSelectorBytes, info)
	if failure != nil {
		return filesystemIdentity{}, false, failure
	}
	return filesystemIdentity{
		path:            selectorPath,
		info:            info,
		useLstat:        true,
		compareContents: true,
		contents:        contents,
	}, true, nil
}

func readBoundedIdentityFile(filePath string, limit int64, expected os.FileInfo) ([]byte, *guardFailure) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fail("invalid-repository", "", "Git identity selector could not be read")
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit || !os.SameFile(expected, before) {
		return nil, fail("invalid-repository", "", "worktree Git selector is not a bounded regular file")
	}
	contents, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(contents)) > limit || int64(len(contents)) != before.Size() {
		return nil, fail("invalid-repository", "", "worktree Git selector could not be read within its bound")
	}
	after, statErr := file.Stat()
	pathInfo, pathErr := os.Lstat(filePath)
	if statErr != nil || pathErr != nil || !os.SameFile(before, after) || !os.SameFile(before, pathInfo) ||
		after.Size() != before.Size() || after.Mode().Type() != before.Mode().Type() {
		return nil, fail("invalid-repository", "", "Git identity selector changed while it was being read")
	}
	return contents, nil
}

func (identity *repositoryIdentity) validate(root string) *guardFailure {
	if identity == nil || root != identity.root ||
		(identity.finalized && (identity.gitDir == "" || identity.commonDir == "")) {
		return fail("repository-identity-changed", "", "admitted repository root or Git directory identity is unavailable")
	}
	if failure := validateFilesystemIdentities(identity.paths); failure != nil {
		return failure
	}
	for _, selector := range identity.selectors {
		if failure := validateFilesystemIdentity(selector); failure != nil {
			return failure
		}
	}
	return nil
}

func validateFilesystemIdentities(identities []filesystemIdentity) *guardFailure {
	for _, identity := range identities {
		if failure := validateFilesystemIdentity(identity); failure != nil {
			return failure
		}
	}
	return nil
}

func validateFilesystemIdentity(identity filesystemIdentity) *guardFailure {
	if identity.expectAbsent {
		if _, err := os.Lstat(identity.path); os.IsNotExist(err) {
			return nil
		}
		return fail("repository-identity-changed", "", "Git common-directory selector appeared during evaluation")
	}
	stat := os.Stat
	if identity.useLstat {
		stat = os.Lstat
	}
	current, err := stat(identity.path)
	if err != nil || current.Mode().Type() != identity.info.Mode().Type() || !os.SameFile(identity.info, current) {
		return fail("repository-identity-changed", "", "repository root, ancestor, Git selector, or Git directory identity changed during evaluation")
	}
	if identity.requireDirectory {
		resolved, resolveErr := filepath.EvalSymlinks(identity.path)
		if !current.IsDir() || resolveErr != nil || filepath.Clean(resolved) != identity.path {
			return fail("repository-identity-changed", "", "repository root, ancestor, or Git directory no longer resolves to its admitted identity")
		}
	}
	if identity.compareContents {
		contents, failure := readBoundedIdentityFile(identity.path, maxGitSelectorBytes, identity.info)
		if failure != nil || !bytes.Equal(contents, identity.contents) {
			return fail("repository-identity-changed", "", "worktree Git selector changed during evaluation")
		}
	}
	return nil
}

//nolint:gocyclo // Sequential identity reads keep the index and worktree admission gates auditable.
func (git gitClient) stagedSnapshot(ctx context.Context, root string, afterDirtyWorktreeRead, afterFinalDirtyRead, afterIndexRead func()) (gitSnapshot, *guardFailure) {
	indexFlagsBefore, failure := git.admitIndexFlags(ctx, root)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	dirtyBefore, failure := git.dirtyGovernedPaths(ctx, root)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	if len(dirtyBefore) != 0 {
		return gitSnapshot{}, dirtyWorktreeFailure(dirtyBefore[0])
	}
	indexFlagsAfterDirtyBefore, _, failure := git.readIndexFlags(ctx, root)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	if !bytes.Equal(indexFlagsBefore, indexFlagsAfterDirtyBefore) {
		return gitSnapshot{}, fail("index-race", "", "Git index flags changed during initial dirty-worktree admission")
	}
	if afterDirtyWorktreeRead != nil {
		afterDirtyWorktreeRead()
	}

	head, failure := git.resolveCommit(ctx, root, "HEAD")
	if failure != nil {
		return gitSnapshot{}, failure.withCode("missing-head")
	}
	indexBefore, failure := git.run(ctx, root, nil, git.limits.maxGitOutput,
		"ls-files", "--cached", "--stage", "-z", "--")
	if failure != nil {
		return gitSnapshot{}, failure
	}
	entries, failure := parseIndexEntries(indexBefore, git.limits)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	diffBefore, failure := git.run(ctx, root, nil, git.limits.maxGitOutput,
		"diff-index", "--cached", "--name-status", "-z", "--no-renames",
		"--no-ext-diff", "--no-textconv", "--diff-filter=ACDMRTUXB", head, "--")
	if failure != nil {
		return gitSnapshot{}, failure
	}
	changed, failure := parseNameStatus(diffBefore, git.limits)
	if failure != nil {
		return gitSnapshot{}, failure
	}

	selected, failure := selectGovernedEntries(entries, git.limits)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	if afterIndexRead != nil {
		afterIndexRead()
	}
	bodies, bodyFailure := git.readBlobBodies(ctx, root, selected)

	indexAfter, identityFailure := git.run(ctx, root, nil, git.limits.maxGitOutput,
		"ls-files", "--cached", "--stage", "-z", "--")
	if identityFailure == nil {
		var diffAfter []byte
		diffAfter, identityFailure = git.run(ctx, root, nil, git.limits.maxGitOutput,
			"diff-index", "--cached", "--name-status", "-z", "--no-renames",
			"--no-ext-diff", "--no-textconv", "--diff-filter=ACDMRTUXB", head, "--")
		if identityFailure == nil && (!bytes.Equal(indexBefore, indexAfter) || !bytes.Equal(diffBefore, diffAfter)) {
			identityFailure = fail("index-race", "", "Git index changed while the staged snapshot was being evaluated")
		}
	}
	if identityFailure == nil {
		var indexFlagsAfter []byte
		indexFlagsAfter, _, identityFailure = git.readIndexFlags(ctx, root)
		if identityFailure == nil && !bytes.Equal(indexFlagsBefore, indexFlagsAfter) {
			identityFailure = fail("index-race", "", "Git index flags changed while the staged snapshot was being evaluated")
		}
	}
	headAfter, headFailure := git.resolveCommit(ctx, root, "HEAD")
	if identityFailure == nil && headFailure != nil {
		identityFailure = headFailure
	}
	if identityFailure == nil && headAfter != head {
		identityFailure = fail("head-race", "", "HEAD changed while the staged snapshot was being evaluated")
	}
	var indexFlagsBeforeFinalDirty []byte
	if identityFailure == nil {
		indexFlagsBeforeFinalDirty, _, identityFailure = git.readIndexFlags(ctx, root)
		if identityFailure == nil && !bytes.Equal(indexFlagsBefore, indexFlagsBeforeFinalDirty) {
			identityFailure = fail("index-race", "", "Git index flags changed before final dirty-worktree admission")
		}
	}
	if identityFailure == nil {
		var dirtyAfter []dirtyWorktreePath
		dirtyAfter, identityFailure = git.dirtyGovernedPaths(ctx, root)
		if identityFailure == nil && len(dirtyAfter) != 0 {
			identityFailure = fail("dirty-worktree-race", dirtyAfter[0].path, "a governed working-tree path became unstaged or nonignored untracked while the staged snapshot was being evaluated; stage the intended contract state or resolve the dirty path before retrying")
		}
	}
	if identityFailure == nil {
		var indexFlagsAfterFinalDirty []byte
		indexFlagsAfterFinalDirty, _, identityFailure = git.readIndexFlags(ctx, root)
		if identityFailure == nil && !bytes.Equal(indexFlagsBeforeFinalDirty, indexFlagsAfterFinalDirty) {
			identityFailure = fail("index-race", "", "Git index flags changed during final dirty-worktree admission")
		}
	}
	if identityFailure == nil && afterFinalDirtyRead != nil {
		afterFinalDirtyRead()
	}
	// dirtyGovernedPaths runs multiple Git commands. A concurrent writer can
	// stage a governed change after one of those commands has observed the
	// worktree, so checkpoint the index, diff, and HEAD again only after the
	// complete final dirty-worktree admission sequence.
	if identityFailure == nil {
		var finalIndex []byte
		finalIndex, identityFailure = git.run(ctx, root, nil, git.limits.maxGitOutput,
			"ls-files", "--cached", "--stage", "-z", "--")
		if identityFailure == nil {
			var finalDiff []byte
			finalDiff, identityFailure = git.run(ctx, root, nil, git.limits.maxGitOutput,
				"diff-index", "--cached", "--name-status", "-z", "--no-renames",
				"--no-ext-diff", "--no-textconv", "--diff-filter=ACDMRTUXB", head, "--")
			if identityFailure == nil && (!bytes.Equal(indexBefore, finalIndex) || !bytes.Equal(diffBefore, finalDiff)) {
				identityFailure = fail("index-race", "", "Git index changed during final dirty-worktree admission")
			}
		}
	}
	if identityFailure == nil {
		var finalIndexFlags []byte
		finalIndexFlags, _, identityFailure = git.readIndexFlags(ctx, root)
		if identityFailure == nil && !bytes.Equal(indexFlagsBefore, finalIndexFlags) {
			identityFailure = fail("index-race", "", "Git index flags changed during final dirty-worktree admission")
		}
	}
	if identityFailure == nil {
		finalHead, finalHeadFailure := git.resolveCommit(ctx, root, "HEAD")
		if finalHeadFailure != nil {
			identityFailure = finalHeadFailure
		} else if finalHead != head {
			identityFailure = fail("head-race", "", "HEAD changed during final dirty-worktree admission")
		}
	}
	if identityFailure != nil {
		return gitSnapshot{}, identityFailure
	}
	if bodyFailure != nil {
		return gitSnapshot{}, bodyFailure
	}
	files, failure := attachBodies(selected, bodies, git.limits.maxCorpusBytes)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	return gitSnapshot{
		base:                head,
		head:                head,
		identity:            snapshotIdentity("staged", []byte(head), indexBefore, indexFlagsBefore, diffBefore),
		files:               files,
		implementationDirs:  implementationDirs(entries),
		implementationPaths: implementationPaths(entries),
		changed:             changed,
	}, nil
}

// dirtyGovernedPaths reads only Git-reported path/status metadata. It does not
// open or semantically parse mutable working-tree files.
func (git gitClient) dirtyGovernedPaths(ctx context.Context, root string) ([]dirtyWorktreePath, *guardFailure) {
	trackedOutput, failure := git.run(ctx, root, nil, git.limits.maxGitOutput,
		"diff-files", "--name-status", "-z", "--no-renames", "--no-ext-diff", "--no-textconv",
		"--ignore-submodules=all", "--diff-filter=ACDMRTUXB", "--")
	if failure != nil {
		return nil, failure
	}
	tracked, failure := parseNameStatus(trackedOutput, git.limits)
	if failure != nil {
		return nil, failure
	}
	untrackedOutput, failure := git.run(ctx, root, nil, git.limits.maxGitOutput,
		"ls-files", "--others", "--exclude-standard", "-z", "--")
	if failure != nil {
		return nil, failure
	}
	untracked, failure := parsePathList(untrackedOutput, git.limits)
	if failure != nil {
		return nil, failure
	}

	dirty := make([]dirtyWorktreePath, 0)
	seen := make(map[string]bool)
	for _, changed := range tracked {
		if !isGovernedPath(changed.path) {
			continue
		}
		dirty = append(dirty, dirtyWorktreePath(changed))
		seen[changed.path] = true
	}
	for _, filePath := range untracked {
		if !isGovernedPath(filePath) || seen[filePath] {
			continue
		}
		dirty = append(dirty, dirtyWorktreePath{path: filePath, status: "?"})
		if len(dirty) > git.limits.maxChanged {
			return nil, fail("changed-file-limit", filePath, "dirty governed path count exceeded the safety limit")
		}
	}
	sort.Slice(dirty, func(i, j int) bool { return dirty[i].path < dirty[j].path })
	return dirty, nil
}

func (git gitClient) admitIndexFlags(ctx context.Context, root string) ([]byte, *guardFailure) {
	output, flagged, failure := git.readIndexFlags(ctx, root)
	if failure != nil {
		return nil, failure
	}
	if len(flagged) != 0 {
		return nil, fail("index-flagged-governed-path", flagged[0], "governed paths must not use assume-unchanged or skip-worktree index flags; sparse checkouts that mark governed paths skip-worktree are unsupported")
	}
	return output, nil
}

func (git gitClient) readIndexFlags(ctx context.Context, root string) ([]byte, []string, *guardFailure) {
	output, failure := git.run(ctx, root, nil, git.limits.maxGitOutput,
		"ls-files", "--cached", "-v", "-z", "--")
	if failure != nil {
		return nil, nil, failure
	}
	flagged, failure := parseIndexFlaggedPaths(output, git.limits)
	if failure != nil {
		return nil, nil, failure
	}
	return output, flagged, nil
}

func dirtyWorktreeFailure(dirty dirtyWorktreePath) *guardFailure {
	kind := "tracked working-tree change"
	if dirty.status == "?" {
		kind = "nonignored untracked working-tree path"
	}
	return fail("dirty-governed-worktree", dirty.path, kind+" is governed but absent from the immutable staged snapshot; stage the intended contract state or resolve the dirty path before retrying")
}

func (git gitClient) committedSnapshot(ctx context.Context, root, baseRevision string) (gitSnapshot, *guardFailure) {
	base, failure := git.resolveCommit(ctx, root, baseRevision)
	if failure != nil {
		return gitSnapshot{}, failure.withCode("invalid-base")
	}
	head, failure := git.resolveCommit(ctx, root, "HEAD")
	if failure != nil {
		return gitSnapshot{}, failure.withCode("missing-head")
	}
	mergeBase, failure := git.run(ctx, root, nil, 256, "merge-base", base, head)
	if failure != nil {
		return gitSnapshot{}, failure.withCode("base-ancestry")
	}
	if strings.TrimSpace(string(mergeBase)) != base {
		return gitSnapshot{}, fail("base-not-ancestor", "", "committed guard base must be an ancestor of HEAD")
	}
	tree, failure := git.run(ctx, root, nil, git.limits.maxGitOutput,
		"ls-tree", "-r", "-z", "--full-tree", head)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	entries, failure := parseTreeEntries(tree, git.limits)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	diff, failure := git.run(ctx, root, nil, git.limits.maxGitOutput,
		"diff-tree", "--no-commit-id", "-r", "--name-status", "-z", "--no-renames",
		"--no-ext-diff", "--no-textconv", "--diff-filter=ACDMRTUXB", base, head, "--")
	if failure != nil {
		return gitSnapshot{}, failure
	}
	changed, failure := parseNameStatus(diff, git.limits)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	selected, failure := selectGovernedEntries(entries, git.limits)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	bodies, failure := git.readBlobBodies(ctx, root, selected)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	files, failure := attachBodies(selected, bodies, git.limits.maxCorpusBytes)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	return gitSnapshot{
		base:                base,
		head:                head,
		identity:            snapshotIdentity("committed", []byte(base), []byte(head)),
		files:               files,
		implementationDirs:  implementationDirs(entries),
		implementationPaths: implementationPaths(entries),
		changed:             changed,
	}, nil
}

func snapshotIdentity(kind string, values ...[]byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(kind))
	_, _ = digest.Write([]byte{0})
	var length [8]byte
	for _, value := range values {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write(value)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func (git gitClient) resolveCommit(ctx context.Context, root, revision string) (string, *guardFailure) {
	output, failure := git.run(ctx, root, nil, 256,
		"rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if failure != nil {
		return "", failure
	}
	commit := strings.TrimSpace(string(output))
	if !validOID(commit) {
		return "", fail("invalid-git-object", "", "Git returned an invalid commit object ID")
	}
	return commit, nil
}

func (git gitClient) readBlobBodies(ctx context.Context, root string, entries []treeEntry) (map[string][]byte, *guardFailure) {
	oids := make([]string, 0, len(entries))
	seen := make(map[string]bool)
	for _, entry := range entries {
		if !seen[entry.oid] {
			oids = append(oids, entry.oid)
			seen[entry.oid] = true
		}
	}
	sort.Strings(oids)
	if len(oids) == 0 {
		return map[string][]byte{}, nil
	}
	var input bytes.Buffer
	for _, oid := range oids {
		input.WriteString(oid)
		input.WriteByte('\n')
	}
	if int64(input.Len()) > git.limits.maxGitInput {
		return nil, fail("git-input-limit", "", "Git object request exceeded the input safety limit")
	}
	headerAllowance := int64(len(oids))*160 + 1
	outputLimit := git.limits.maxCorpusBytes + headerAllowance
	if outputLimit < 0 || outputLimit > git.limits.maxGitOutput {
		outputLimit = git.limits.maxGitOutput
	}
	output, failure := git.run(ctx, root, input.Bytes(), outputLimit, "cat-file", "--batch")
	if failure != nil {
		return nil, failure.withCode("git-object-read")
	}
	return parseBatchBlobs(output, oids, git.limits)
}

func attachBodies(entries []treeEntry, bodies map[string][]byte, corpusLimit int64) (map[string]sourceFile, *guardFailure) {
	files := make(map[string]sourceFile, len(entries))
	var logicalBytes int64
	for _, entry := range entries {
		body, ok := bodies[entry.oid]
		if !ok {
			return nil, fail("missing-git-object", entry.path, "governed Git blob body was not returned")
		}
		if int64(len(body)) > corpusLimit-logicalBytes {
			return nil, fail("corpus-size-limit", entry.path, "logical governed file corpus exceeded the safety limit")
		}
		logicalBytes += int64(len(body))
		files[entry.path] = sourceFile{body: body}
	}
	return files, nil
}

func selectGovernedEntries(entries []treeEntry, limits guardLimits) ([]treeEntry, *guardFailure) {
	selected := make([]treeEntry, 0)
	for _, entry := range entries {
		if !isGovernedPath(entry.path) {
			continue
		}
		if entry.stage != 0 {
			return nil, fail("unmerged-index-entry", entry.path, "governed file has an unmerged Git index stage")
		}
		if entry.objectType != "" && entry.objectType != "blob" {
			return nil, fail("nonregular-git-mode", entry.path, "governed path must be a regular Git blob")
		}
		if entry.mode != "100644" && entry.mode != "100755" {
			return nil, fail("nonregular-git-mode", entry.path, "governed path must use a regular-file Git mode")
		}
		selected = append(selected, entry)
		if len(selected) > limits.maxFiles {
			return nil, fail("file-limit", entry.path, "governed file count exceeded the safety limit")
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].path < selected[j].path })
	return selected, nil
}

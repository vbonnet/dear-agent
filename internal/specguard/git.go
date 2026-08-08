package specguard

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // Git SHA-1 object identity is verified, not used as a new security primitive.
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var errCommandOutputLimit = errors.New("git command output exceeded its safety limit")

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
	dirtyBefore, failure := git.dirtyGovernedPaths(ctx, root)
	if failure != nil {
		return gitSnapshot{}, failure
	}
	if len(dirtyBefore) != 0 {
		return gitSnapshot{}, dirtyWorktreeFailure(dirtyBefore[0])
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
	headAfter, headFailure := git.resolveCommit(ctx, root, "HEAD")
	if identityFailure == nil && headFailure != nil {
		identityFailure = headFailure
	}
	if identityFailure == nil && headAfter != head {
		identityFailure = fail("head-race", "", "HEAD changed while the staged snapshot was being evaluated")
	}
	if identityFailure == nil {
		var dirtyAfter []dirtyWorktreePath
		dirtyAfter, identityFailure = git.dirtyGovernedPaths(ctx, root)
		if identityFailure == nil && len(dirtyAfter) != 0 {
			identityFailure = fail("dirty-worktree-race", dirtyAfter[0].path, "a governed working-tree path became unstaged or nonignored untracked while the staged snapshot was being evaluated; stage the intended contract state or resolve the dirty path before retrying")
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
		identity:            snapshotIdentity("staged", []byte(head), indexBefore, diffBefore),
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

func parseIndexEntries(output []byte, limits guardLimits) ([]treeEntry, *guardFailure) {
	records, failure := nulRecords(output, limits.maxEntries, "index")
	if failure != nil {
		return nil, failure
	}
	entries := make([]treeEntry, 0, len(records))
	seen := make(map[string]bool)
	for _, record := range records {
		metadata, filePath, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fail("malformed-index", "", "Git index entry omitted its path separator")
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 3 || !validMode(fields[0]) || !validOID(fields[1]) {
			return nil, fail("malformed-index", "", "Git index entry contained invalid metadata")
		}
		stage, err := strconv.Atoi(fields[2])
		if err != nil || stage < 0 || stage > 3 {
			return nil, fail("malformed-index", "", "Git index entry contained an invalid merge stage")
		}
		pathText := string(filePath)
		if failure := validateGitPath(pathText, limits.maxPathBytes); failure != nil {
			return nil, failure
		}
		identity := fmt.Sprintf("%s\x00%d", pathText, stage)
		if seen[identity] {
			return nil, fail("malformed-index", pathText, "Git index contained a duplicate path and stage")
		}
		seen[identity] = true
		entries = append(entries, treeEntry{path: pathText, mode: fields[0], oid: fields[1], stage: stage})
	}
	return entries, nil
}

func parseTreeEntries(output []byte, limits guardLimits) ([]treeEntry, *guardFailure) {
	records, failure := nulRecords(output, limits.maxEntries, "tree")
	if failure != nil {
		return nil, failure
	}
	entries := make([]treeEntry, 0, len(records))
	seen := make(map[string]bool)
	for _, record := range records {
		metadata, filePath, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fail("malformed-tree", "", "Git tree entry omitted its path separator")
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 3 || !validMode(fields[0]) || !validOID(fields[2]) {
			return nil, fail("malformed-tree", "", "Git tree entry contained invalid metadata")
		}
		pathText := string(filePath)
		if failure := validateGitPath(pathText, limits.maxPathBytes); failure != nil {
			return nil, failure
		}
		if seen[pathText] {
			return nil, fail("malformed-tree", pathText, "Git tree contained a duplicate path")
		}
		seen[pathText] = true
		entries = append(entries, treeEntry{path: pathText, mode: fields[0], objectType: fields[1], oid: fields[2]})
	}
	return entries, nil
}

func parseNameStatus(output []byte, limits guardLimits) ([]change, *guardFailure) {
	records, failure := nulRecords(output, limits.maxChanged*2, "changed-path")
	if failure != nil {
		return nil, failure
	}
	if len(records)%2 != 0 {
		return nil, fail("malformed-diff", "", "Git name-status output was not paired")
	}
	changes := make([]change, 0, len(records)/2)
	seen := make(map[string]bool)
	for index := 0; index < len(records); index += 2 {
		status := string(records[index])
		filePath := string(records[index+1])
		if len(status) != 1 || !strings.ContainsRune("ADMT", rune(status[0])) {
			return nil, fail("malformed-diff", filePath, "Git reported an unsupported change status")
		}
		if failure := validateGitPath(filePath, limits.maxPathBytes); failure != nil {
			return nil, failure
		}
		if seen[filePath] {
			return nil, fail("malformed-diff", filePath, "Git reported a changed path more than once")
		}
		seen[filePath] = true
		changes = append(changes, change{path: filePath, status: status})
		if len(changes) > limits.maxChanged {
			return nil, fail("changed-file-limit", filePath, "changed path count exceeded the safety limit")
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].path < changes[j].path })
	return changes, nil
}

func parsePathList(output []byte, limits guardLimits) ([]string, *guardFailure) {
	records, failure := nulRecords(output, limits.maxEntries, "untracked-path")
	if failure != nil {
		return nil, failure
	}
	paths := make([]string, 0, len(records))
	seen := make(map[string]bool)
	for _, record := range records {
		filePath := string(record)
		if failure := validateGitPath(filePath, limits.maxPathBytes); failure != nil {
			return nil, failure
		}
		if seen[filePath] {
			return nil, fail("malformed-git-output", filePath, "Git path output contained a duplicate path")
		}
		seen[filePath] = true
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	return paths, nil
}

//nolint:gocyclo // Linear batch framing and object verification intentionally remain in one parser.
func parseBatchBlobs(output []byte, expected []string, limits guardLimits) (map[string][]byte, *guardFailure) {
	reader := bytes.NewReader(output)
	bodies := make(map[string][]byte, len(expected))
	var corpusBytes int64
	for _, wantedOID := range expected {
		header, err := readBoundedLine(reader, 256)
		if err != nil {
			return nil, fail("malformed-git-object", "", "Git object batch returned a malformed header")
		}
		fields := strings.Fields(string(header))
		if len(fields) == 2 && fields[1] == "missing" {
			return nil, fail("missing-git-object", "", "a required Git blob is unavailable locally; lazy fetching is disabled")
		}
		if len(fields) != 3 || fields[0] != wantedOID || fields[1] != "blob" {
			return nil, fail("malformed-git-object", "", "Git object batch returned unexpected identity or type metadata")
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 {
			return nil, fail("malformed-git-object", "", "Git object batch returned an invalid blob size")
		}
		if size > limits.maxFileBytes {
			return nil, fail("file-size-limit", "", "a governed Git blob exceeded the per-file safety limit")
		}
		if size > limits.maxCorpusBytes-corpusBytes {
			return nil, fail("corpus-size-limit", "", "governed Git blobs exceeded the corpus safety limit")
		}
		body := make([]byte, size)
		if _, err := io.ReadFull(reader, body); err != nil {
			return nil, fail("malformed-git-object", "", "Git object batch ended before a blob body was complete")
		}
		separator, err := reader.ReadByte()
		if err != nil || separator != '\n' {
			return nil, fail("malformed-git-object", "", "Git object batch omitted a blob separator")
		}
		if !blobMatchesOID(body, wantedOID) {
			return nil, fail("git-object-identity", "", "Git returned blob bytes that do not match the requested object ID")
		}
		bodies[wantedOID] = body
		corpusBytes += size
	}
	if reader.Len() != 0 {
		return nil, fail("malformed-git-object", "", "Git object batch returned trailing data")
	}
	return bodies, nil
}

func readBoundedLine(reader *bytes.Reader, limit int) ([]byte, error) {
	line := make([]byte, 0, min(limit, reader.Len()))
	for len(line) <= limit {
		value, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if value == '\n' {
			return line, nil
		}
		line = append(line, value)
	}
	return nil, errors.New("line exceeded limit")
}

func blobMatchesOID(body []byte, oid string) bool {
	header := fmt.Appendf(nil, "blob %d\x00", len(body))
	switch len(oid) {
	case 40:
		hash := sha1.New() //nolint:gosec // Required to verify SHA-1 Git repositories.
		_, _ = hash.Write(header)
		_, _ = hash.Write(body)
		return hex.EncodeToString(hash.Sum(nil)) == oid
	case 64:
		hash := sha256.New()
		_, _ = hash.Write(header)
		_, _ = hash.Write(body)
		return hex.EncodeToString(hash.Sum(nil)) == oid
	default:
		return false
	}
}

func nulRecords(output []byte, limit int, label string) ([][]byte, *guardFailure) {
	if len(output) == 0 {
		return [][]byte{}, nil
	}
	if output[len(output)-1] != 0 {
		return nil, fail("malformed-git-output", "", fmt.Sprintf("Git %s output was not NUL terminated", label))
	}
	records := bytes.Split(output[:len(output)-1], []byte{0})
	if len(records) > limit {
		return nil, fail("git-entry-limit", "", fmt.Sprintf("Git %s output exceeded the entry safety limit", label))
	}
	for _, record := range records {
		if len(record) == 0 {
			return nil, fail("malformed-git-output", "", fmt.Sprintf("Git %s output contained an empty record", label))
		}
	}
	return records, nil
}

func validMode(mode string) bool {
	if len(mode) != 6 {
		return false
	}
	for _, value := range mode {
		if value < '0' || value > '7' {
			return false
		}
	}
	return true
}

func validOID(oid string) bool {
	if len(oid) != 40 && len(oid) != 64 {
		return false
	}
	_, err := hex.DecodeString(oid)
	return err == nil && strings.ToLower(oid) == oid
}

//nolint:gocyclo // The explicit allowlist keeps revision admission reviewable.
func validRevision(revision string, limit int) bool {
	if revision == "" || len(revision) > limit || strings.HasPrefix(revision, "-") ||
		strings.Contains(revision, "..") || strings.Contains(revision, "@{") ||
		strings.ContainsAny(revision, "\\\x00\r\n\t ~^:?*[\"") ||
		strings.HasSuffix(revision, "/") || strings.Contains(revision, "//") {
		return false
	}
	for _, value := range revision {
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') || strings.ContainsRune("._/-", value) {
			continue
		}
		return false
	}
	return true
}

func (git gitClient) run(ctx context.Context, root string, input []byte, outputLimit int64, args ...string) ([]byte, *guardFailure) {
	if int64(len(input)) > git.limits.maxGitInput {
		return nil, fail("git-input-limit", "", "Git command input exceeded the safety limit")
	}
	if outputLimit <= 0 || outputLimit > git.limits.maxGitOutput {
		return nil, fail("git-output-limit", "", "Git command output limit is invalid")
	}
	if git.repository != nil {
		if failure := git.repository.validate(root); failure != nil {
			return nil, failure
		}
	}
	commandCtx, cancel := context.WithTimeout(ctx, git.limits.gitTime)
	defer cancel()

	commandArgs := []string{
		"--no-replace-objects",
		"-c", "protocol.allow=never",
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "diff.external=",
	}
	if root != "" {
		commandArgs = append(commandArgs, "-C", root)
	}
	commandArgs = append(commandArgs, args...)
	command := exec.Command(git.executable, commandArgs...)
	command.Env = cleanGitEnvironment(git.executable)
	command.Stdin = bytes.NewReader(input)
	configureProcessGroup(command)
	command.WaitDelay = 250 * time.Millisecond
	capture := newCommandCapture(outputLimit)
	command.Stdout = capture.stdout()
	command.Stderr = capture.stderr()
	execution := runGitCommand(commandCtx, command, capture)
	stdout, stderr, overflow := capture.result()
	if git.afterCommand != nil {
		git.afterCommand()
	}
	if git.repository != nil {
		if failure := git.repository.validate(root); failure != nil {
			return nil, failure
		}
	}
	return classifyGitCommandExecution(stdout, stderr, overflow, execution, git.limits.gitTime)
}

func classifyGitCommandExecution(
	stdout []byte,
	stderr []byte,
	overflow bool,
	execution gitCommandExecution,
	gitTime time.Duration,
) ([]byte, *guardFailure) {
	cancellationSignalErr := normalizeGitCancellationSignalError(
		execution.cancellationSignalErr,
		execution.observedExitErr,
		execution.cleanupErr,
	)
	switch {
	case execution.observedExitErr != nil || execution.cleanupErr != nil ||
		cancellationSignalErr != nil && !errors.Is(cancellationSignalErr, os.ErrProcessDone):
		return nil, fail("git-descendant-termination", "", "Git descendant processes could not be terminated")
	case overflow:
		return nil, fail("git-output-limit", "", errCommandOutputLimit.Error())
	case execution.contextCancellationObserved:
		return nil, fail("git-time-limit", "", fmt.Sprintf("Git command exceeded the %s wall-time limit", gitTime))
	case execution.commandErr != nil:
		message := "Git command failed"
		if detail := boundedDiagnostic(stderr, 512); detail != "" {
			message += ": " + detail
		}
		return nil, fail("git-command", "", message)
	default:
		return stdout, nil
	}
}

func normalizeGitCancellationSignalError(signalErr, observedExitErr, cleanupErr error) error {
	if errors.Is(signalErr, syscall.EPERM) && observedExitErr == nil && cleanupErr == nil {
		// On Darwin a context wake can race with direct-child exit and observe
		// EPERM from a zombie-only group. Successful pinned final cleanup proves
		// that no descendant remained, so the earlier signal is process-done.
		return os.ErrProcessDone
	}
	return signalErr
}

type gitCommandExecution struct {
	commandErr                  error
	observedExitErr             error
	cleanupErr                  error
	cancellationSignalErr       error
	contextCancellationObserved bool
}

// runGitCommand owns cancellation and process-group cleanup. waitid leaves the
// direct child unreaped until final cleanup has killed and sealed its isolated
// group, so neither an output callback nor a late context wake can signal a
// different process after numeric PID/PGID reuse.
func runGitCommand(ctx context.Context, command *exec.Cmd, capture *commandCapture) gitCommandExecution {
	if err := ctx.Err(); err != nil {
		return gitCommandExecution{commandErr: err, contextCancellationObserved: true}
	}
	lifecycle := newGitProcessGroupLifecycle(command)
	capture.onLimit = func() {
		_ = lifecycle.cancel()
	}
	if err := command.Start(); err != nil {
		lifecycle.disable()
		return gitCommandExecution{commandErr: err}
	}

	lifecycleDone := make(chan struct{})
	contextResult := make(chan gitContextCancellationResult, 1)
	go func() {
		select {
		case <-ctx.Done():
			observed, err := lifecycle.cancelObserved()
			contextResult <- gitContextCancellationResult{signalErr: err, contextObserved: observed}
		case <-lifecycleDone:
			contextResult <- gitContextCancellationResult{}
		}
	}()

	observedExitErr := waitForGitCommandExitWithoutReaping(command.Process.Pid)
	cleanupErr := lifecycle.complete(observedExitErr == nil, errors.Is(observedExitErr, syscall.ECHILD))
	close(lifecycleDone)
	cancellation := <-contextResult
	commandErr := command.Wait()
	return gitCommandExecution{
		commandErr:                  commandErr,
		observedExitErr:             observedExitErr,
		cleanupErr:                  cleanupErr,
		cancellationSignalErr:       cancellation.signalErr,
		contextCancellationObserved: cancellation.contextObserved,
	}
}

type gitContextCancellationResult struct {
	signalErr       error
	contextObserved bool
}

// gitProcessGroupLifecycle serializes every group signal with final cleanup.
// complete seals the signal path while the unreaped leader still pins the
// numeric process-group ID; Cmd.Wait performs the one subsequent reap.
type gitProcessGroupLifecycle struct {
	mu                      sync.Mutex
	command                 *exec.Cmd
	enabled                 bool
	directChildExitObserved bool
}

func newGitProcessGroupLifecycle(command *exec.Cmd) *gitProcessGroupLifecycle {
	return &gitProcessGroupLifecycle{command: command, enabled: true}
}

func (lifecycle *gitProcessGroupLifecycle) cancel() error {
	_, err := lifecycle.cancelObserved()
	return err
}

// cancelObserved reports whether cancellation acquired the lifecycle before
// final cleanup sealed it. A raw ProcessDone result can therefore still count
// as a timeout when the context won the ordering race while the child exited.
func (lifecycle *gitProcessGroupLifecycle) cancelObserved() (bool, error) {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if !lifecycle.enabled || lifecycle.command.Process == nil {
		return false, os.ErrProcessDone
	}
	return true, killProcessGroup(lifecycle.command.Process)
}

func (lifecycle *gitProcessGroupLifecycle) complete(directChildExitObserved, skipKill bool) error {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if !lifecycle.enabled {
		return nil
	}
	lifecycle.directChildExitObserved = directChildExitObserved
	var err error
	if !skipKill {
		err = lifecycle.terminateLocked()
	}
	lifecycle.enabled = false
	return err
}

func (lifecycle *gitProcessGroupLifecycle) disable() {
	lifecycle.mu.Lock()
	lifecycle.enabled = false
	lifecycle.mu.Unlock()
}

func (lifecycle *gitProcessGroupLifecycle) terminateLocked() error {
	if lifecycle.command.Process == nil {
		return nil
	}
	processGroupID := lifecycle.command.Process.Pid
	if err := killProcessGroup(lifecycle.command.Process); errors.Is(err, os.ErrProcessDone) {
		return nil
	} else if errors.Is(err, syscall.EPERM) {
		complete, classificationErr := gitProcessGroupEPERMComplete(processGroupID, lifecycle.directChildExitObserved)
		if classificationErr != nil {
			return fmt.Errorf("classify EPERM for Git process group %d: %w", processGroupID, classificationErr)
		}
		if complete {
			return nil
		}
		return fmt.Errorf("terminate Git process group %d: %w", processGroupID, err)
	} else if err != nil {
		return fmt.Errorf("terminate Git process group %d: %w", processGroupID, err)
	}
	return nil
}

func cleanGitEnvironment(executable string) []string {
	pathEntries := slices.Compact([]string{filepath.Dir(executable), "/usr/bin", "/bin"})
	environment := []string{
		"HOME=/var/empty",
		"LANG=C",
		"LC_ALL=C",
		"PATH=" + strings.Join(pathEntries, string(os.PathListSeparator)),
		"PAGER=cat",
		"XDG_CONFIG_HOME=/var/empty",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	}
	for _, name := range []string{"COMSPEC", "PATHEXT", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func boundedDiagnostic(data []byte, limit int) string {
	if len(data) > limit {
		data = data[:limit]
	}
	text := strings.Map(func(value rune) rune {
		if value == '\n' || value == '\r' || value == '\t' {
			return ' '
		}
		if value < 0x20 || value == 0x7f {
			return -1
		}
		return value
	}, string(data))
	return strings.Join(strings.Fields(text), " ")
}

func (failure *guardFailure) withCode(code string) *guardFailure {
	if failure == nil {
		return nil
	}
	if failure.code == "git-input-limit" || failure.code == "git-output-limit" || failure.code == "git-time-limit" ||
		failure.code == "git-descendant-termination" || failure.code == "repository-identity-changed" {
		return failure
	}
	return fail(code, failure.path, failure.message)
}

type commandCapture struct {
	mu           sync.Mutex
	stdoutBuffer bytes.Buffer
	stderrBuffer bytes.Buffer
	limit        int64
	used         int64
	overflow     bool
	onLimit      func()
	cancelOnce   sync.Once
}

type captureStream struct {
	capture *commandCapture
	stderr  bool
}

func newCommandCapture(limit int64) *commandCapture {
	return &commandCapture{limit: limit}
}

func (capture *commandCapture) stdout() io.Writer { return captureStream{capture: capture} }
func (capture *commandCapture) stderr() io.Writer {
	return captureStream{capture: capture, stderr: true}
}

func (stream captureStream) Write(data []byte) (int, error) {
	stream.capture.mu.Lock()
	remaining := stream.capture.limit - stream.capture.used
	if int64(len(data)) > remaining {
		if remaining > 0 {
			prefix := data[:remaining]
			stream.capture.used += int64(len(prefix))
			if stream.stderr {
				_, _ = stream.capture.stderrBuffer.Write(prefix)
			} else {
				_, _ = stream.capture.stdoutBuffer.Write(prefix)
			}
		}
		stream.capture.overflow = true
		stream.capture.mu.Unlock()
		stream.capture.cancelOnce.Do(func() {
			if stream.capture.onLimit != nil {
				stream.capture.onLimit()
			}
		})
		return 0, errCommandOutputLimit
	}
	stream.capture.used += int64(len(data))
	defer stream.capture.mu.Unlock()
	if stream.stderr {
		return stream.capture.stderrBuffer.Write(data)
	}
	return stream.capture.stdoutBuffer.Write(data)
}

func (capture *commandCapture) result() (stdout, stderr []byte, overflow bool) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return bytes.Clone(capture.stdoutBuffer.Bytes()), bytes.Clone(capture.stderrBuffer.Bytes()), capture.overflow
}

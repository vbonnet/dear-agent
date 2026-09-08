package buildauthority

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
)

const (
	workspaceTemporary   = "tmp"
	workspaceGoTemporary = "gotmp"
	workspaceGoPath      = "gopath"
	workspaceGoCache     = "gocache"
	workspaceGitTemplate = "git-template"
	workspaceGitExec     = "git-exec"
	workspaceGitHooks    = "git-hooks"
	workspaceGitPath     = "git-path"
	workspaceObjectSpool = "object-spool"
	workspaceCheckout    = "checkout"
	workspaceOutputs     = "outputs"

	workspaceGitGlobal     = "git-global.conf"
	workspaceGitAttributes = "git-attributes"
	workspaceGitExcludes   = "git-excludes"
	workspaceGitLink       = "git-path/git"

	taskRootPrefix       = ".sandbox-gc-build-"
	taskRootRandomBytes  = 12
	taskRootNameAttempts = 100

	maximumSpoolFiles = uint64(4_096)
	maximumSpoolBytes = uint64(2 << 30)
)

type workspacePolicyRow struct {
	name           string
	kind           entryKind
	initiallyEmpty bool
	unchangedEmpty bool
}

// workspacePolicyRows returns a fresh value on every call. There is no
// package-level mutable policy backing array for callers or tests to alter.
func workspacePolicyRows() []workspacePolicyRow {
	return []workspacePolicyRow{
		{name: workspaceTemporary, kind: entryDirectory, initiallyEmpty: true, unchangedEmpty: true},
		{name: workspaceGoTemporary, kind: entryDirectory, initiallyEmpty: true},
		{name: workspaceGoPath, kind: entryDirectory, initiallyEmpty: true, unchangedEmpty: true},
		{name: workspaceGoCache, kind: entryDirectory, initiallyEmpty: true},
		{name: workspaceGitTemplate, kind: entryDirectory, initiallyEmpty: true, unchangedEmpty: true},
		{name: workspaceGitExec, kind: entryDirectory, initiallyEmpty: true, unchangedEmpty: true},
		{name: workspaceGitHooks, kind: entryDirectory, initiallyEmpty: true, unchangedEmpty: true},
		{name: workspaceObjectSpool, kind: entryDirectory, initiallyEmpty: true},
		{name: workspaceCheckout, kind: entryDirectory, initiallyEmpty: true},
		{name: workspaceOutputs, kind: entryDirectory, initiallyEmpty: true},
		{name: workspaceGitPath, kind: entryDirectory},
		{name: workspaceGitGlobal, kind: entryRegular},
		{name: workspaceGitAttributes, kind: entryRegular},
		{name: workspaceGitExcludes, kind: entryRegular},
	}
}

type ownedWorkspaceEntry struct {
	path     string
	kind     entryKind
	claim    workspaceEntryClaim
	observed bool
}

type workspaceAfterCreateFunc func(string, entryKind) error

type taskRootRetainFunc func(
	context.Context,
	*retainedDirectory,
	string,
	uint32,
) (*retainedDirectory, error)

type workspaceAllocationDependencies struct {
	random       io.Reader
	retain       taskRootRetainFunc
	afterCreate  workspaceAfterCreateFunc
	afterObserve workspaceAfterCreateFunc
	beforeRemove func() error
}

// workspaceEntryClaim retains the security metadata that must not change while
// allowing directory size, link count, and timestamps to reflect owned child
// creation. Regular-file and symlink callers separately require their exact
// size and link count.
type workspaceEntryClaim struct {
	name        string
	kind        entryKind
	snapshot    fileSnapshot
	mount       mountSnapshot
	aclDigest   Digest
	linkText    string
	targetFile  fileSnapshot
	hasContent  bool
	contentHash Digest
}

// workspaceGitAuthority borrows an already retained Git descriptor. It does
// not admit Git: the tool authority still owns digest, Mach-O, and transcript
// validation. This value only prevents the private PATH link from being bound
// to a different pathname or filesystem identity.
type workspaceGitAuthority struct {
	path       string
	descriptor *os.File
	snapshot   fileSnapshot
	mount      mountSnapshot
	aclDigest  Digest
}

// taskWorkspace is the cleanup-owned writable namespace. StateRoot and the Git
// descriptor are borrowed; taskRoot and objectSpool are owned here.
type taskWorkspace struct {
	stateRoot       *retainedDirectory
	taskRoot        *retainedDirectory
	unobservedRoot  *retainedDirectory
	taskName        string
	paths           workspacePaths
	ownedEntries    []ownedWorkspaceEntry
	directoryClaims map[string]workspaceEntryClaim
	fileClaims      map[string]workspaceEntryClaim
	gitLinkClaim    workspaceEntryClaim
	git             workspaceGitAuthority
	spool           *objectSpool
	afterCreate     workspaceAfterCreateFunc
	afterObserve    workspaceAfterCreateFunc
	beforeRemove    func() error
	lifecycle       sync.RWMutex
	cleanupOnce     sync.Once
	cleanupErr      error
	closed          bool
}

type objectSpool struct {
	root      *retainedDirectory
	rootClaim workspaceEntryClaim
	mu        sync.Mutex
	next      uint64
	active    *spoolGeneration
	closed    bool
	closeErr  error
}

type spoolGeneration struct {
	spool    *objectSpool
	id       uint64
	next     uint64
	bytes    uint64
	files    []*spoolFile
	claims   map[string]workspaceEntryClaim
	draining bool
	poisoned bool
	finished bool
}

type spoolFileState uint8

const (
	spoolFileWritable spoolFileState = iota + 1
	spoolFileSealed
	spoolFileRemoved
)

type spoolFile struct {
	generation *spoolGeneration
	name       string
	descriptor *os.File
	writeHash  hash.Hash
	written    uint64
	state      spoolFileState
	reader     *sealedSpoolReader
}

// sealedSpoolReader exposes only the retained descriptor's ReaderAt surface.
// Descriptor ownership and lifetime remain with the object spool.
type sealedSpoolReader struct {
	file *spoolFile
	size int64
}

var _ io.ReaderAt = (*sealedSpoolReader)(nil)

func (reader *sealedSpoolReader) Size() int64 {
	if reader == nil {
		return 0
	}
	return reader.size
}

func (reader *sealedSpoolReader) ReadAt(contents []byte, offset int64) (int, error) {
	return reader.readAtWith(contents, offset, nil)
}

// readAtWith keeps the production ReaderAt interface narrow while exposing a
// deterministic internal seam for proving that bytes are withheld if the
// retained inode changes during a read.
func (reader *sealedSpoolReader) readAtWith(contents []byte, offset int64, afterRead func() error) (int, error) {
	file, spool, err := reader.retainedAuthority()
	if err != nil {
		return 0, err
	}
	if offset < 0 {
		return 0, fail(CauseMalformed, "negative sealed object-spool read offset")
	}
	generation := file.generation
	spool.mu.Lock()
	defer spool.mu.Unlock()
	claim, err := reader.liveClaimLocked(file)
	if err != nil {
		return 0, err
	}
	if err := validateSealedSpoolReadClaim(context.Background(), file.descriptor, claim); err != nil {
		return withholdSpoolRead(generation, contents, err)
	}
	count, readErr := io.NewSectionReader(file.descriptor, 0, reader.size).ReadAt(contents, offset)
	if err := runAfterSpoolRead(afterRead); err != nil {
		return withholdSpoolRead(generation, contents, err)
	}
	if err := validateSealedSpoolReadClaim(context.Background(), file.descriptor, claim); err != nil {
		return withholdSpoolRead(generation, contents, err)
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return withholdSpoolRead(
			generation,
			contents,
			failWithFilesystemCauses(readErr, CauseUnstable, "read sealed object-spool file"),
		)
	}
	return count, readErr
}

func (reader *sealedSpoolReader) retainedAuthority() (*spoolFile, *objectSpool, error) {
	if reader == nil || reader.file == nil || reader.file.generation == nil ||
		reader.file.generation.spool == nil {
		return nil, nil, fail(CauseInternalInvariant, "missing sealed object-spool reader")
	}
	return reader.file, reader.file.generation.spool, nil
}

func (reader *sealedSpoolReader) liveClaimLocked(file *spoolFile) (workspaceEntryClaim, error) {
	generation := file.generation
	if err := generation.requireActiveLocked(); err != nil {
		return workspaceEntryClaim{}, err
	}
	if file.state != spoolFileSealed || file.reader != reader || file.descriptor == nil {
		return workspaceEntryClaim{}, fail(CauseInternalInvariant, "object-spool reader is no longer live")
	}
	claim, ok := generation.claims[file.name]
	if !ok || !claim.hasContent {
		return workspaceEntryClaim{}, fail(CauseInternalInvariant, "missing sealed object-spool content claim")
	}
	return claim, nil
}

func runAfterSpoolRead(afterRead func() error) error {
	if afterRead == nil {
		return nil
	}
	if err := afterRead(); err != nil {
		return failWithFilesystemCauses(err, CauseUnstable, "mutate sealed object-spool read")
	}
	return nil
}

func withholdSpoolRead(
	generation *spoolGeneration,
	contents []byte,
	err error,
) (int, error) {
	generation.poisoned = true
	clear(contents)
	return 0, err
}

func validateSealedSpoolReadClaim(
	ctx context.Context,
	descriptor *os.File,
	expected workspaceEntryClaim,
) error {
	if descriptor == nil || !expected.hasContent || expected.kind != entryRegular {
		return fail(CauseInternalInvariant, "missing sealed object-spool read claim")
	}
	current, mount, aclDigest, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return err
	}
	if current != expected.snapshot || mount != expected.mount || aclDigest != expected.aclDigest {
		return fail(CauseUnstable, "sealed object-spool file changed")
	}
	return nil
}

func newWorkspaceGitAuthority(
	ctx context.Context,
	path string,
	descriptor *os.File,
) (workspaceGitAuthority, error) {
	if err := checkContext(ctx, "bind retained Git workspace authority"); err != nil {
		return workspaceGitAuthority{}, err
	}
	if descriptor == nil {
		return workspaceGitAuthority{}, fail(CauseInternalInvariant, "missing retained Git descriptor")
	}
	if err := validateAuthorityPath(path); err != nil {
		return workspaceGitAuthority{}, err
	}
	snapshot, mount, aclDigest, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return workspaceGitAuthority{}, err
	}
	if err := validateProtected(snapshot, ownerRootOrEffective); err != nil {
		return workspaceGitAuthority{}, err
	}
	if snapshotKind(snapshot) != entryRegular {
		return workspaceGitAuthority{}, fail(CauseIdentity, "retained Git is not a regular file")
	}
	authority := workspaceGitAuthority{
		path:       path,
		descriptor: descriptor,
		snapshot:   snapshot,
		mount:      mount,
		aclDigest:  aclDigest,
	}
	if err := authority.revalidate(ctx); err != nil {
		return workspaceGitAuthority{}, err
	}
	return authority, nil
}

func (authority workspaceGitAuthority) revalidate(ctx context.Context) error {
	if authority.path == "" || authority.descriptor == nil {
		return fail(CauseInternalInvariant, "missing retained Git workspace authority")
	}
	current, mount, aclDigest, err := inspectDescriptorContext(ctx, authority.descriptor)
	if err != nil {
		return err
	}
	if current != authority.snapshot || mount != authority.mount || aclDigest != authority.aclDigest {
		return fail(CauseUnstable, "retained Git workspace authority changed")
	}
	visible, err := openAbsoluteNoFollow(authority.path, entryRegular)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "reopen retained Git path")
	}
	visibleSnapshot, visibleMount, visibleACL, inspectErr := inspectDescriptorContext(ctx, visible)
	closeErr := closeDescriptorFailure(visible.Close())
	if inspectErr != nil || closeErr != nil {
		return joinFilesystemFailures(inspectErr, closeErr)
	}
	if visibleSnapshot != authority.snapshot || visibleMount != authority.mount || visibleACL != authority.aclDigest {
		return fail(CauseIdentity, "retained Git path identity changed")
	}
	return nil
}

func allocateWorkspace(
	ctx context.Context,
	stateRoot *retainedDirectory,
	git workspaceGitAuthority,
) (*taskWorkspace, error) {
	return allocateWorkspaceWith(ctx, stateRoot, git, workspaceAllocationDependencies{
		random: rand.Reader,
		retain: retainCreatedWorkspaceDirectory,
	})
}

func allocateWorkspaceWith(
	ctx context.Context,
	stateRoot *retainedDirectory,
	git workspaceGitAuthority,
	dependencies workspaceAllocationDependencies,
) (*taskWorkspace, error) {
	if err := checkContext(ctx, "allocate private workspace"); err != nil {
		return nil, err
	}
	if stateRoot == nil || stateRoot.root == nil || stateRoot.descriptor == nil {
		return nil, fail(CauseInternalInvariant, "missing retained StateRoot")
	}
	if err := validateMutableWorkspaceDirectory(ctx, stateRoot, stateRoot.snapshot); err != nil {
		return nil, err
	}
	if err := git.revalidate(ctx); err != nil {
		return nil, err
	}

	workspace, err := createTaskWorkspaceWith(ctx, stateRoot, git, dependencies)
	if err != nil {
		if workspace == nil {
			return nil, err
		}
		return nil, workspace.cleanup(ctx, workspacePrimaryFailure(OperationOpen, err))
	}
	if err := workspace.initialize(ctx); err != nil {
		return nil, workspace.cleanup(ctx, workspacePrimaryFailure(OperationOpen, err))
	}
	if err := workspace.requireInitialShape(ctx); err != nil {
		return nil, workspace.cleanup(ctx, workspacePrimaryFailure(OperationValidate, err))
	}
	return workspace, nil
}

func workspacePrimaryFailure(operation Operation, err error) *FailureRecord {
	return &FailureRecord{
		Phase:     PhaseWorkspace,
		Operation: operation,
		Causes:    privateCauses(err, CauseUnstable),
	}
}

func createTaskWorkspaceWith(
	ctx context.Context,
	stateRoot *retainedDirectory,
	git workspaceGitAuthority,
	dependencies workspaceAllocationDependencies,
) (*taskWorkspace, error) {
	if dependencies.random == nil || dependencies.retain == nil {
		return nil, fail(CauseInternalInvariant, "missing workspace-allocation dependency")
	}
	policyRows := workspacePolicyRows()
	for range taskRootNameAttempts {
		name, err := generateTaskRootName(dependencies.random)
		if err != nil {
			return nil, err
		}
		if err := checkContext(ctx, "allocate private task root"); err != nil {
			return nil, err
		}
		paths, err := newWorkspacePaths(filepath.Join(stateRoot.path, name))
		if err != nil {
			return nil, err
		}
		if err := stateRoot.root.Mkdir(name, 0o700); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return nil, failWithFilesystemCauses(err, classifyPathError(err), "create private task root")
		}
		workspace := &taskWorkspace{
			stateRoot:       stateRoot,
			taskName:        name,
			paths:           paths,
			ownedEntries:    make([]ownedWorkspaceEntry, 0, len(policyRows)+1),
			directoryClaims: make(map[string]workspaceEntryClaim, len(policyRows)),
			fileClaims:      make(map[string]workspaceEntryClaim, len(policyRows)),
			git:             git,
			afterCreate:     dependencies.afterCreate,
			afterObserve:    dependencies.afterObserve,
			beforeRemove:    dependencies.beforeRemove,
		}
		taskRoot, err := dependencies.retain(ctx, stateRoot, name, 0o700)
		if err != nil {
			workspace.unobservedRoot = taskRoot
			return workspace, err
		}
		if taskRoot == nil {
			return workspace, fail(CauseInternalInvariant, "workspace retention returned no task root")
		}
		workspace.taskRoot = taskRoot
		if err := validateMutableWorkspaceDirectory(ctx, stateRoot, stateRoot.snapshot); err != nil {
			return workspace, err
		}
		return workspace, nil
	}
	return nil, fail(CauseLimit, "private task-root name attempts exhausted")
}

func generateTaskRootName(random io.Reader) (string, error) {
	if random == nil {
		return "", fail(CauseInternalInvariant, "missing task-root randomness")
	}
	value := make([]byte, taskRootRandomBytes)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", failWith(err, CauseInternalInvariant, "generate private task-root name")
	}
	return taskRootPrefix + hex.EncodeToString(value), nil
}

func retainCreatedWorkspaceDirectory(
	ctx context.Context,
	parent *retainedDirectory,
	name string,
	mode uint32,
) (*retainedDirectory, error) {
	if parent == nil || parent.root == nil || parent.descriptor == nil {
		return nil, fail(CauseInternalInvariant, "missing workspace parent handles")
	}
	descriptor, err := openRelativeNoFollow(int(parent.descriptor.Fd()), name, entryDirectory)
	if err != nil {
		return nil, failWithFilesystemCauses(err, classifyPathError(err), "open created workspace directory")
	}
	if err := descriptor.Chmod(os.FileMode(mode)); err != nil {
		return nil, joinFilesystemFailures(
			failWithFilesystemCauses(err, classifyPathError(err), "set workspace directory mode"),
			closeDescriptorFailure(descriptor.Close()),
		)
	}
	root, err := parent.root.OpenRoot(name)
	if err != nil {
		return nil, joinFilesystemFailures(
			failWithFilesystemCauses(err, classifyPathError(err), "retain workspace directory root"),
			closeDescriptorFailure(descriptor.Close()),
		)
	}
	snapshot, mount, aclDigest, inspectErr := inspectDescriptorContext(ctx, descriptor)
	if inspectErr != nil {
		return nil, joinFilesystemFailures(inspectErr, closeDescriptorFailure(root.Close()), closeDescriptorFailure(descriptor.Close()))
	}
	if err := validateExactWorkspaceDirectory(snapshot, mount, parent.mount, parent.snapshot.identity.Device, mode); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(root.Close()), closeDescriptorFailure(descriptor.Close()))
	}
	info, statErr := root.Lstat(".")
	descriptorInfo, descriptorStatErr := descriptor.Stat()
	if statErr != nil || descriptorStatErr != nil || !os.SameFile(info, descriptorInfo) {
		return nil, joinFilesystemFailures(
			failWithFilesystemCauses(errors.Join(statErr, descriptorStatErr), CauseIdentity, "workspace directory identity mismatch"),
			closeDescriptorFailure(root.Close()),
			closeDescriptorFailure(descriptor.Close()),
		)
	}
	claims := append([]authorityPathClaim(nil), parent.pathClaims...)
	claims = append(claims, makeAuthorityPathClaim(snapshot, mount, aclDigest))
	return &retainedDirectory{
		path:       filepath.Join(parent.path, name),
		root:       root,
		descriptor: descriptor,
		snapshot:   snapshot,
		mount:      mount,
		aclDigest:  aclDigest,
		pathClaims: claims,
	}, nil
}

func (workspace *taskWorkspace) initialize(ctx context.Context) error {
	for _, row := range workspacePolicyRows() {
		switch row.kind {
		case entryDirectory:
			claim, err := workspace.createDirectory(ctx, row.name)
			if err != nil {
				return err
			}
			workspace.directoryClaims[row.name] = claim
			if err := workspace.runAfterObserve(row.name, row.kind); err != nil {
				return err
			}
		case entryRegular:
			claim, err := workspace.createEmptyPolicyFile(ctx, row.name)
			if err != nil {
				return err
			}
			workspace.fileClaims[row.name] = claim
			if err := workspace.runAfterObserve(row.name, row.kind); err != nil {
				return err
			}
		case entrySymlink:
			return fail(CauseInternalInvariant, "workspace policy row cannot be a symlink")
		default:
			return fail(CauseInternalInvariant, "unknown workspace policy row")
		}
	}
	if err := workspace.createGitLink(ctx); err != nil {
		return err
	}
	if err := workspace.refreshOwnedDirectoryClaim(ctx, workspaceGitPath); err != nil {
		return err
	}
	if err := workspace.runAfterObserve(workspaceGitLink, entrySymlink); err != nil {
		return err
	}
	spoolRoot, err := workspace.openDirectoryRoot(ctx, workspaceObjectSpool)
	if err != nil {
		return err
	}
	workspace.spool = &objectSpool{
		root:      spoolRoot,
		rootClaim: workspace.directoryClaims[workspaceObjectSpool],
	}
	return nil
}

func (workspace *taskWorkspace) createDirectory(ctx context.Context, name string) (workspaceEntryClaim, error) {
	if err := checkContext(ctx, "create private workspace directory"); err != nil {
		return workspaceEntryClaim{}, err
	}
	if err := workspace.taskRoot.root.Mkdir(name, 0o700); err != nil {
		return workspaceEntryClaim{}, failWithFilesystemCauses(err, classifyPathError(err), "create private workspace directory")
	}
	owned := workspace.recordOwnedEntry(name, entryDirectory)
	if err := workspace.runAfterCreate(name, entryDirectory); err != nil {
		return workspaceEntryClaim{}, err
	}
	descriptor, err := openRelativeNoFollow(int(workspace.taskRoot.descriptor.Fd()), name, entryDirectory)
	if err != nil {
		return workspaceEntryClaim{}, failWithFilesystemCauses(err, classifyPathError(err), "open private workspace directory")
	}
	if err := descriptor.Chmod(0o700); err != nil {
		return workspaceEntryClaim{}, joinFilesystemFailures(
			failWithFilesystemCauses(err, classifyPathError(err), "set private workspace directory mode"),
			closeDescriptorFailure(descriptor.Close()),
		)
	}
	claim, inspectErr := inspectWorkspaceEntry(ctx, name, descriptor, entryDirectory)
	closeErr := closeDescriptorFailure(descriptor.Close())
	if inspectErr != nil || closeErr != nil {
		return workspaceEntryClaim{}, joinFilesystemFailures(inspectErr, closeErr)
	}
	workspace.observeOwnedEntry(owned, claim)
	if err := validateExactWorkspaceDirectory(
		claim.snapshot,
		claim.mount,
		workspace.taskRoot.mount,
		workspace.taskRoot.snapshot.identity.Device,
		0o700,
	); err != nil {
		return workspaceEntryClaim{}, err
	}
	return claim, nil
}

func (workspace *taskWorkspace) createEmptyPolicyFile(
	ctx context.Context,
	name string,
) (workspaceEntryClaim, error) {
	if err := checkContext(ctx, "create private Git policy file"); err != nil {
		return workspaceEntryClaim{}, err
	}
	descriptor, err := workspace.taskRoot.root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return workspaceEntryClaim{}, failWithFilesystemCauses(err, classifyPathError(err), "create private Git policy file")
	}
	owned := workspace.recordOwnedEntry(name, entryRegular)
	if err := workspace.runAfterCreate(name, entryRegular); err != nil {
		return workspaceEntryClaim{}, joinFilesystemFailures(err, closeDescriptorFailure(descriptor.Close()))
	}
	if err := descriptor.Chmod(0o600); err != nil {
		return workspaceEntryClaim{}, joinFilesystemFailures(
			failWithFilesystemCauses(err, classifyPathError(err), "set private Git policy mode"),
			closeDescriptorFailure(descriptor.Close()),
		)
	}
	claim, inspectErr := inspectWorkspaceEntry(ctx, name, descriptor, entryRegular)
	closeErr := closeDescriptorFailure(descriptor.Close())
	if inspectErr != nil || closeErr != nil {
		return workspaceEntryClaim{}, joinFilesystemFailures(inspectErr, closeErr)
	}
	workspace.observeOwnedEntry(owned, claim)
	if err := validateExactWorkspaceFile(
		claim.snapshot,
		claim.mount,
		workspace.taskRoot.mount,
		workspace.taskRoot.snapshot.identity.Device,
		0,
	); err != nil {
		return workspaceEntryClaim{}, err
	}
	return claim, nil
}

func (workspace *taskWorkspace) createGitLink(ctx context.Context) error {
	if err := workspace.git.revalidate(ctx); err != nil {
		return err
	}
	if err := workspace.taskRoot.root.Symlink(workspace.git.path, workspaceGitLink); err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "create private Git PATH link")
	}
	owned := workspace.recordOwnedEntry(workspaceGitLink, entrySymlink)
	if err := workspace.runAfterCreate(workspaceGitLink, entrySymlink); err != nil {
		return err
	}
	claim, err := workspace.captureGitLink(ctx)
	if err != nil {
		return err
	}
	workspace.observeOwnedEntry(owned, claim)
	workspace.gitLinkClaim = claim
	return nil
}

func (workspace *taskWorkspace) recordOwnedEntry(path string, kind entryKind) int {
	workspace.ownedEntries = append(workspace.ownedEntries, ownedWorkspaceEntry{path: path, kind: kind})
	return len(workspace.ownedEntries) - 1
}

func (workspace *taskWorkspace) observeOwnedEntry(index int, claim workspaceEntryClaim) {
	if index < 0 || index >= len(workspace.ownedEntries) {
		return
	}
	workspace.ownedEntries[index].claim = claim
	workspace.ownedEntries[index].observed = true
}

func (workspace *taskWorkspace) runAfterCreate(path string, kind entryKind) error {
	if workspace.afterCreate == nil {
		return nil
	}
	return workspace.afterCreate(path, kind)
}

func (workspace *taskWorkspace) runAfterObserve(path string, kind entryKind) error {
	if workspace.afterObserve == nil {
		return nil
	}
	return workspace.afterObserve(path, kind)
}

func (workspace *taskWorkspace) refreshOwnedDirectoryClaim(ctx context.Context, name string) error {
	expected, ok := workspace.directoryClaims[name]
	if !ok {
		return fail(CauseInternalInvariant, "missing owned workspace parent claim")
	}
	descriptor, err := openRelativeNoFollow(int(workspace.taskRoot.descriptor.Fd()), name, entryDirectory)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "reopen owned workspace parent")
	}
	current, inspectErr := inspectWorkspaceEntry(ctx, name, descriptor, entryDirectory)
	closeErr := closeDescriptorFailure(descriptor.Close())
	if inspectErr != nil || closeErr != nil {
		return joinFilesystemFailures(inspectErr, closeErr)
	}
	if !stableWorkspaceClaimEqual(expected, current) {
		return fail(CauseIdentity, "owned workspace parent security claim changed")
	}
	if err := validateExactWorkspaceDirectory(
		current.snapshot,
		current.mount,
		workspace.taskRoot.mount,
		workspace.taskRoot.snapshot.identity.Device,
		0o700,
	); err != nil {
		return err
	}
	workspace.directoryClaims[name] = current
	for index := range workspace.ownedEntries {
		entry := &workspace.ownedEntries[index]
		if entry.path == name && entry.kind == entryDirectory && entry.observed {
			entry.claim = current
			return nil
		}
	}
	return fail(CauseInternalInvariant, "missing owned workspace parent ledger row")
}

func (workspace *taskWorkspace) captureGitLink(ctx context.Context) (workspaceEntryClaim, error) {
	descriptor, err := openRelativeNoFollow(int(workspace.taskRoot.descriptor.Fd()), workspaceGitLink, entrySymlink)
	if err != nil {
		return workspaceEntryClaim{}, failWithFilesystemCauses(err, classifyPathError(err), "open private Git PATH link")
	}
	claim, inspectErr := inspectWorkspaceEntry(ctx, workspaceGitLink, descriptor, entrySymlink)
	if inspectErr == nil {
		claim.linkText, inspectErr = readOpenedSymlinkContext(ctx, descriptor, maxSymlinkBytes)
	}
	closeErr := closeDescriptorFailure(descriptor.Close())
	if inspectErr != nil || closeErr != nil {
		return workspaceEntryClaim{}, joinFilesystemFailures(inspectErr, closeErr)
	}
	uid, err := effectiveUserID()
	if err != nil {
		return workspaceEntryClaim{}, err
	}
	if claim.snapshot.identity.UID != uid || claim.snapshot.linkCount != 1 || claim.snapshot.identity.Mode&0o7000 != 0 {
		return workspaceEntryClaim{}, fail(CausePermission, "private Git PATH link policy mismatch")
	}
	if claim.mount != workspace.taskRoot.mount {
		return workspaceEntryClaim{}, fail(CauseUnsupported, "private Git PATH link mount changed")
	}
	if claim.linkText != workspace.git.path {
		return workspaceEntryClaim{}, fail(CauseIdentity, "private Git PATH link target changed")
	}
	if err := workspace.git.revalidate(ctx); err != nil {
		return workspaceEntryClaim{}, err
	}
	claim.targetFile = workspace.git.snapshot
	return claim, nil
}

func inspectWorkspaceEntry(
	ctx context.Context,
	name string,
	descriptor *os.File,
	kind entryKind,
) (workspaceEntryClaim, error) {
	snapshot, mount, aclDigest, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return workspaceEntryClaim{}, err
	}
	if snapshotKind(snapshot) != kind {
		return workspaceEntryClaim{}, fail(CauseIdentity, "private workspace entry kind changed")
	}
	return workspaceEntryClaim{name: name, kind: kind, snapshot: snapshot, mount: mount, aclDigest: aclDigest}, nil
}

func validateExactWorkspaceDirectory(
	snapshot fileSnapshot,
	mount mountSnapshot,
	parentMount mountSnapshot,
	parentDevice uint64,
	mode uint32,
) error {
	if err := validateProtected(snapshot, ownerEffectiveOnly); err != nil {
		return err
	}
	if snapshotKind(snapshot) != entryDirectory || snapshot.identity.Mode&0o7777 != mode {
		return fail(CausePermission, "private workspace directory policy mismatch")
	}
	if mount != parentMount || snapshot.identity.Device != parentDevice {
		return fail(CauseUnsupported, "private workspace directory mount changed")
	}
	return nil
}

func validateExactWorkspaceFile(
	snapshot fileSnapshot,
	mount mountSnapshot,
	parentMount mountSnapshot,
	parentDevice uint64,
	size int64,
) error {
	if err := validateProtected(snapshot, ownerEffectiveOnly); err != nil {
		return err
	}
	if snapshotKind(snapshot) != entryRegular || snapshot.identity.Mode&0o7777 != 0o600 {
		return fail(CausePermission, "private workspace file policy mismatch")
	}
	if snapshot.linkCount != 1 || snapshot.size != size {
		return fail(CauseIdentity, "private workspace file identity mismatch")
	}
	if mount != parentMount || snapshot.identity.Device != parentDevice {
		return fail(CauseUnsupported, "private workspace file mount changed")
	}
	return nil
}

func stableWorkspaceClaimEqual(expected, current workspaceEntryClaim) bool {
	return expected.name == current.name &&
		expected.kind == current.kind &&
		expected.snapshot.identity == current.snapshot.identity &&
		expected.snapshot.gid == current.snapshot.gid &&
		expected.snapshot.rdev == current.snapshot.rdev &&
		expected.snapshot.birthSec == current.snapshot.birthSec &&
		expected.snapshot.birthNsec == current.snapshot.birthNsec &&
		expected.snapshot.flags == current.snapshot.flags &&
		expected.snapshot.generation == current.snapshot.generation &&
		expected.mount == current.mount &&
		expected.aclDigest == current.aclDigest
}

func validateMutableWorkspaceDirectory(
	ctx context.Context,
	directory *retainedDirectory,
	expected fileSnapshot,
) error {
	if directory == nil || directory.descriptor == nil {
		return fail(CauseInternalInvariant, "missing retained writable directory")
	}
	snapshot, mount, aclDigest, err := inspectDescriptorContext(ctx, directory.descriptor)
	if err != nil {
		return err
	}
	expectedClaim := workspaceEntryClaim{kind: entryDirectory, snapshot: expected, mount: directory.mount, aclDigest: directory.aclDigest}
	currentClaim := workspaceEntryClaim{kind: entryDirectory, snapshot: snapshot, mount: mount, aclDigest: aclDigest}
	if !stableWorkspaceClaimEqual(expectedClaim, currentClaim) {
		return fail(CauseUnstable, "retained writable directory security claim changed")
	}
	if err := validateExactWorkspaceDirectory(snapshot, mount, directory.mount, directory.snapshot.identity.Device, 0o700); err != nil {
		return err
	}
	return revalidateAuthorityPathContextWithInspect(ctx, directory, inspectDescriptorContext)
}

func (workspace *taskWorkspace) openDirectoryRoot(
	ctx context.Context,
	name string,
) (*retainedDirectory, error) {
	expected, ok := workspace.directoryClaims[name]
	if !ok {
		return nil, fail(CauseInternalInvariant, "unknown private workspace directory")
	}
	descriptor, err := openRelativeNoFollow(int(workspace.taskRoot.descriptor.Fd()), name, entryDirectory)
	if err != nil {
		return nil, failWithFilesystemCauses(err, classifyPathError(err), "reopen private workspace directory")
	}
	current, inspectErr := inspectWorkspaceEntry(ctx, name, descriptor, entryDirectory)
	if inspectErr != nil || !stableWorkspaceClaimEqual(expected, current) {
		if inspectErr == nil {
			inspectErr = fail(CauseUnstable, "private workspace directory changed")
		}
		return nil, joinFilesystemFailures(inspectErr, closeDescriptorFailure(descriptor.Close()))
	}
	if err := validateExactWorkspaceDirectory(
		current.snapshot,
		current.mount,
		workspace.taskRoot.mount,
		workspace.taskRoot.snapshot.identity.Device,
		0o700,
	); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(descriptor.Close()))
	}
	root, err := workspace.taskRoot.root.OpenRoot(name)
	if err != nil {
		return nil, joinFilesystemFailures(
			failWithFilesystemCauses(err, classifyPathError(err), "retain private workspace directory"),
			closeDescriptorFailure(descriptor.Close()),
		)
	}
	info, statErr := root.Lstat(".")
	descriptorInfo, descriptorStatErr := descriptor.Stat()
	if statErr != nil || descriptorStatErr != nil || !os.SameFile(info, descriptorInfo) {
		return nil, joinFilesystemFailures(
			failWithFilesystemCauses(errors.Join(statErr, descriptorStatErr), CauseIdentity, "private workspace root identity mismatch"),
			closeDescriptorFailure(root.Close()),
			closeDescriptorFailure(descriptor.Close()),
		)
	}
	claims := append([]authorityPathClaim(nil), workspace.taskRoot.pathClaims...)
	claims = append(claims, makeAuthorityPathClaim(current.snapshot, current.mount, current.aclDigest))
	return &retainedDirectory{
		path:       workspace.paths.child(name),
		root:       root,
		descriptor: descriptor,
		snapshot:   current.snapshot,
		mount:      current.mount,
		aclDigest:  current.aclDigest,
		pathClaims: claims,
	}, nil
}

func (workspace *taskWorkspace) requireInitialShape(ctx context.Context) error {
	if workspace == nil {
		return fail(CauseInternalInvariant, "missing live private workspace")
	}
	workspace.lifecycle.RLock()
	defer workspace.lifecycle.RUnlock()
	if err := workspace.requireUnchangedPolicyLocked(ctx); err != nil {
		return err
	}
	for _, row := range workspacePolicyRows() {
		if row.kind == entryDirectory && row.initiallyEmpty {
			if err := workspace.requireDirectoryEmpty(ctx, row.name); err != nil {
				return err
			}
		}
	}
	return nil
}

// requireUnchangedPolicy checks the fixed top-level namespace, every retained
// directory identity, the policy-file identities and bytes, and the sole link.
// It permits bounded derived contents only in their declared directory roots.
func (workspace *taskWorkspace) requireUnchangedPolicy(ctx context.Context) error {
	if workspace == nil {
		return fail(CauseInternalInvariant, "missing live private workspace")
	}
	workspace.lifecycle.RLock()
	defer workspace.lifecycle.RUnlock()
	return workspace.requireUnchangedPolicyLocked(ctx)
}

func (workspace *taskWorkspace) requireUnchangedPolicyLocked(ctx context.Context) error {
	if workspace == nil || workspace.taskRoot == nil || workspace.closed {
		return fail(CauseInternalInvariant, "missing live private workspace")
	}
	if err := validateMutableWorkspaceDirectory(ctx, workspace.taskRoot, workspace.taskRoot.snapshot); err != nil {
		return err
	}
	if err := workspace.requireExactTopLevel(ctx); err != nil {
		return err
	}
	if err := workspace.revalidateDirectoryClaims(ctx); err != nil {
		return err
	}
	if err := workspace.requireUnchangedEmptyDirectories(ctx); err != nil {
		return err
	}
	if err := workspace.revalidatePolicyFiles(ctx); err != nil {
		return err
	}
	if err := workspace.revalidateGitPath(ctx); err != nil {
		return err
	}
	if err := workspace.requireExactTopLevel(ctx); err != nil {
		return err
	}
	return validateMutableWorkspaceDirectory(ctx, workspace.taskRoot, workspace.taskRoot.snapshot)
}

func (workspace *taskWorkspace) revalidateDirectoryClaims(ctx context.Context) error {
	for _, row := range workspacePolicyRows() {
		if row.kind == entryDirectory {
			if err := workspace.revalidateDirectoryClaim(ctx, row.name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (workspace *taskWorkspace) requireUnchangedEmptyDirectories(ctx context.Context) error {
	for _, row := range workspacePolicyRows() {
		if row.kind == entryDirectory && row.unchangedEmpty {
			if err := workspace.requireDirectoryEmpty(ctx, row.name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (workspace *taskWorkspace) revalidatePolicyFiles(ctx context.Context) error {
	for _, row := range workspacePolicyRows() {
		if row.kind == entryRegular {
			if err := workspace.revalidatePolicyFile(ctx, row.name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (workspace *taskWorkspace) revalidateGitPath(ctx context.Context) error {
	if err := workspace.revalidateGitLink(ctx); err != nil {
		return err
	}
	if err := workspace.requireGitPathContents(ctx); err != nil {
		return err
	}
	if err := workspace.revalidateGitLink(ctx); err != nil {
		return err
	}
	return nil
}

func (workspace *taskWorkspace) requireBeforeFirstGo(ctx context.Context) error {
	if workspace == nil {
		return fail(CauseInternalInvariant, "missing live private workspace")
	}
	workspace.lifecycle.RLock()
	defer workspace.lifecycle.RUnlock()
	if err := workspace.requireUnchangedPolicyLocked(ctx); err != nil {
		return err
	}
	for _, name := range []string{workspaceGoTemporary, workspaceGoCache} {
		if err := workspace.requireDirectoryEmpty(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

func (workspace *taskWorkspace) requireBeforeGitObjectUse(ctx context.Context) error {
	if workspace == nil {
		return fail(CauseInternalInvariant, "missing live private workspace")
	}
	workspace.lifecycle.RLock()
	defer workspace.lifecycle.RUnlock()
	if err := workspace.requireUnchangedPolicyLocked(ctx); err != nil {
		return err
	}
	if workspace.spool == nil {
		return fail(CauseInternalInvariant, "missing cleanup-owned object spool")
	}
	return workspace.spool.requireEmpty(ctx)
}

func (workspace *taskWorkspace) requireExactTopLevel(ctx context.Context) error {
	rows := workspacePolicyRows()
	want := make([]string, 0, len(rows))
	for _, row := range rows {
		want = append(want, row.name)
	}
	sort.Strings(want)
	descriptor, err := workspace.taskRoot.root.Open(".")
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "open private workspace top level")
	}
	got, readErr := readStableDirectoryNames(ctx, descriptor, len(want)+1)
	closeErr := closeDescriptorFailure(descriptor.Close())
	if readErr != nil || closeErr != nil {
		return joinFilesystemFailures(readErr, closeErr)
	}
	if len(got) != len(want) {
		return fail(CauseIdentity, "private workspace top-level shape changed")
	}
	sort.Strings(got)
	for index := range want {
		if got[index] != want[index] {
			return fail(CauseIdentity, "private workspace top-level shape changed")
		}
	}
	return nil
}

func (workspace *taskWorkspace) revalidateDirectoryClaim(ctx context.Context, name string) error {
	expected, ok := workspace.directoryClaims[name]
	if !ok {
		return fail(CauseInternalInvariant, "missing private workspace directory claim")
	}
	descriptor, err := openRelativeNoFollow(int(workspace.taskRoot.descriptor.Fd()), name, entryDirectory)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "reopen private workspace directory")
	}
	current, inspectErr := inspectWorkspaceEntry(ctx, name, descriptor, entryDirectory)
	closeErr := closeDescriptorFailure(descriptor.Close())
	if inspectErr != nil || closeErr != nil {
		return joinFilesystemFailures(inspectErr, closeErr)
	}
	if !stableWorkspaceClaimEqual(expected, current) {
		return fail(CauseUnstable, "private workspace directory claim changed")
	}
	return validateExactWorkspaceDirectory(
		current.snapshot,
		current.mount,
		workspace.taskRoot.mount,
		workspace.taskRoot.snapshot.identity.Device,
		0o700,
	)
}

func (workspace *taskWorkspace) revalidatePolicyFile(ctx context.Context, name string) error {
	expected, ok := workspace.fileClaims[name]
	if !ok {
		return fail(CauseInternalInvariant, "missing private Git policy-file claim")
	}
	descriptor, err := openRelativeNoFollow(int(workspace.taskRoot.descriptor.Fd()), name, entryRegular)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "reopen private Git policy file")
	}
	current, inspectErr := inspectWorkspaceEntry(ctx, name, descriptor, entryRegular)
	closeErr := closeDescriptorFailure(descriptor.Close())
	if inspectErr != nil || closeErr != nil {
		return joinFilesystemFailures(inspectErr, closeErr)
	}
	if !stableWorkspaceClaimEqual(expected, current) ||
		current.snapshot.linkCount != expected.snapshot.linkCount ||
		current.snapshot.size != 0 {
		return fail(CauseUnstable, "private Git policy file changed")
	}
	return validateExactWorkspaceFile(
		current.snapshot,
		current.mount,
		workspace.taskRoot.mount,
		workspace.taskRoot.snapshot.identity.Device,
		0,
	)
}

func (workspace *taskWorkspace) revalidateGitLink(ctx context.Context) error {
	current, err := workspace.captureGitLink(ctx)
	if err != nil {
		return err
	}
	expected := workspace.gitLinkClaim
	if !stableWorkspaceClaimEqual(expected, current) ||
		current.snapshot.linkCount != expected.snapshot.linkCount ||
		current.snapshot.size != expected.snapshot.size ||
		current.linkText != expected.linkText ||
		current.targetFile != expected.targetFile {
		return fail(CauseUnstable, "private Git PATH link changed")
	}
	return nil
}

func (workspace *taskWorkspace) requireDirectoryEmpty(ctx context.Context, name string) error {
	if err := workspace.revalidateDirectoryClaim(ctx, name); err != nil {
		return err
	}
	descriptor, err := openRelativeNoFollow(int(workspace.taskRoot.descriptor.Fd()), name, entryDirectory)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "open private empty workspace directory")
	}
	names, readErr := readStableDirectoryNames(ctx, descriptor, 1)
	closeErr := closeDescriptorFailure(descriptor.Close())
	if readErr != nil || closeErr != nil {
		return joinFilesystemFailures(readErr, closeErr)
	}
	if len(names) != 0 {
		return fail(CauseUnstable, "private workspace directory is not empty")
	}
	return nil
}

func (workspace *taskWorkspace) requireGitPathContents(ctx context.Context) error {
	descriptor, err := openRelativeNoFollow(int(workspace.taskRoot.descriptor.Fd()), workspaceGitPath, entryDirectory)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "open private Git PATH directory")
	}
	names, readErr := readStableDirectoryNames(ctx, descriptor, 2)
	closeErr := closeDescriptorFailure(descriptor.Close())
	if readErr != nil || closeErr != nil {
		return joinFilesystemFailures(readErr, closeErr)
	}
	if len(names) != 1 || names[0] != "git" {
		return fail(CauseIdentity, "private Git PATH directory contents changed")
	}
	return nil
}

func readDirectoryNames(ctx context.Context, descriptor *os.File, maximum int) ([]string, error) {
	if descriptor == nil || maximum <= 0 {
		return nil, fail(CauseInternalInvariant, "invalid bounded directory read")
	}
	names := make([]string, 0, maximum)
	for {
		if err := checkContext(ctx, "read private workspace directory"); err != nil {
			return nil, err
		}
		entries, err := descriptor.ReadDir(directoryReadBatchSize)
		for _, entry := range entries {
			names = append(names, entry.Name())
			if len(names) >= maximum {
				return names, nil
			}
		}
		if errors.Is(err, io.EOF) {
			return names, nil
		}
		if err != nil {
			return nil, failWithFilesystemCauses(err, classifyPathError(err), "read private workspace directory")
		}
	}
}

func readStableDirectoryNames(ctx context.Context, descriptor *os.File, maximum int) ([]string, error) {
	before, beforeMount, beforeACL, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	names, err := readDirectoryNames(ctx, descriptor, maximum)
	if err != nil {
		return nil, err
	}
	after, afterMount, afterACL, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	if before != after || beforeMount != afterMount || beforeACL != afterACL {
		return nil, fail(CauseUnstable, "private workspace directory changed during read")
	}
	return names, nil
}

func (spool *objectSpool) beginGeneration(ctx context.Context) (*spoolGeneration, error) {
	if spool == nil || spool.root == nil {
		return nil, fail(CauseInternalInvariant, "missing cleanup-owned object spool")
	}
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if spool.closed {
		return nil, fail(CauseInternalInvariant, "object spool is closed")
	}
	if spool.active != nil {
		return nil, fail(CauseInternalInvariant, "object-spool generation already active")
	}
	if err := spool.requireEmptyLocked(ctx); err != nil {
		return nil, err
	}
	if spool.next == ^uint64(0) {
		return nil, fail(CauseLimit, "object-spool generation counter exhausted")
	}
	spool.next++
	generation := &spoolGeneration{
		spool:  spool,
		id:     spool.next,
		claims: make(map[string]workspaceEntryClaim),
	}
	spool.active = generation
	return generation, nil
}

func (generation *spoolGeneration) create(ctx context.Context) (*spoolFile, error) {
	if generation == nil || generation.spool == nil {
		return nil, fail(CauseInternalInvariant, "missing object-spool generation")
	}
	spool := generation.spool
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if err := generation.requireActiveLocked(); err != nil {
		return nil, err
	}
	if generation.draining {
		return nil, fail(CauseInternalInvariant, "object-spool generation is draining")
	}
	if generation.next >= maximumSpoolFiles {
		return nil, fail(CauseLimit, "object-spool file limit exceeded")
	}
	if err := checkContext(ctx, "create object-spool file"); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("object-%016x-%04x", generation.id, generation.next)
	descriptor, err := spool.root.root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, failWithFilesystemCauses(err, classifyPathError(err), "create object-spool file")
	}
	file := &spoolFile{
		generation: generation,
		name:       name,
		descriptor: descriptor,
		writeHash:  sha256.New(),
		state:      spoolFileWritable,
	}
	generation.files = append(generation.files, file)
	generation.next++
	if err := descriptor.Chmod(0o600); err != nil {
		return nil, failWithFilesystemCauses(err, classifyPathError(err), "set object-spool file mode")
	}
	claim, inspectErr := inspectWorkspaceEntry(ctx, name, descriptor, entryRegular)
	if inspectErr == nil {
		inspectErr = validateExactWorkspaceFile(
			claim.snapshot,
			claim.mount,
			generation.spool.root.mount,
			generation.spool.root.snapshot.identity.Device,
			0,
		)
	}
	if inspectErr != nil {
		return nil, inspectErr
	}
	return file, nil
}

func (file *spoolFile) write(ctx context.Context, contents []byte) (int, error) {
	if file == nil || file.generation == nil {
		return 0, fail(CauseInternalInvariant, "missing object-spool file")
	}
	generation := file.generation
	spool := generation.spool
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if err := generation.requireActiveLocked(); err != nil {
		return 0, err
	}
	if !generation.writableFileLocked(file) {
		return 0, fail(CauseInternalInvariant, "object-spool file is not writable")
	}
	if err := checkContext(ctx, "write object-spool file"); err != nil {
		return 0, err
	}
	want := uint64(len(contents))
	if generation.bytes > maximumSpoolBytes || want > maximumSpoolBytes-generation.bytes {
		return 0, fail(CauseLimit, "object-spool byte limit exceeded")
	}
	written, err := file.descriptor.Write(contents)
	if accountErr := file.accountWriteLocked(contents, written); accountErr != nil {
		return 0, accountErr
	}
	if err == nil && written != len(contents) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return written, failWithFilesystemCauses(err, CauseUnstable, "write object-spool file")
	}
	return written, nil
}

func (generation *spoolGeneration) writableFileLocked(file *spoolFile) bool {
	return !generation.draining && generation.ownsFileLocked(file) &&
		file.state == spoolFileWritable && file.descriptor != nil && file.writeHash != nil
}

func (file *spoolFile) accountWriteLocked(contents []byte, written int) error {
	if written < 0 || written > len(contents) {
		file.generation.poisoned = true
		return fail(CauseInternalInvariant, "object-spool write returned an invalid count")
	}
	writtenBytes := uint64(written)
	file.generation.bytes += writtenBytes
	file.written += writtenBytes
	if written > 0 {
		// hash.Hash.Write never returns an error. Hash exactly the prefix the
		// retained descriptor reports as written, including a partial write.
		_, _ = file.writeHash.Write(contents[:written])
	}
	return nil
}

func (file *spoolFile) seal(ctx context.Context) (*sealedSpoolReader, error) {
	if file == nil || file.generation == nil {
		return nil, fail(CauseInternalInvariant, "missing object-spool file")
	}
	generation := file.generation
	spool := generation.spool
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if err := generation.requireActiveLocked(); err != nil {
		return nil, err
	}
	if generation.draining || !generation.ownsFileLocked(file) {
		return nil, fail(CauseInternalInvariant, "object-spool file is not sealable")
	}
	reader, err := generation.sealFileLocked(ctx, file)
	if err != nil {
		generation.poisoned = true
	}
	return reader, err
}

func (generation *spoolGeneration) sealFileLocked(
	ctx context.Context,
	file *spoolFile,
) (*sealedSpoolReader, error) {
	if file.state == spoolFileSealed {
		return file.reader, nil
	}
	if file.state != spoolFileWritable || file.descriptor == nil || file.writeHash == nil {
		return nil, fail(CauseInternalInvariant, "missing writable object-spool descriptor")
	}
	claim, err := generation.captureSealClaimLocked(ctx, file)
	if err != nil {
		return nil, err
	}
	reader := &sealedSpoolReader{file: file, size: int64(file.written)} //nolint:gosec // Spool writes are capped to 2 GiB.
	file.state = spoolFileSealed
	file.writeHash = nil
	file.reader = reader
	generation.claims[file.name] = claim
	return reader, nil
}

func (generation *spoolGeneration) captureSealClaimLocked(
	ctx context.Context,
	file *spoolFile,
) (workspaceEntryClaim, error) {
	if err := checkContext(ctx, "seal object-spool file"); err != nil {
		return workspaceEntryClaim{}, err
	}
	if err := file.descriptor.Sync(); err != nil {
		return workspaceEntryClaim{}, failWithFilesystemCauses(err, CauseUnstable, "sync object-spool file")
	}
	claim, err := inspectWorkspaceEntry(ctx, file.name, file.descriptor, entryRegular)
	if err != nil {
		return workspaceEntryClaim{}, err
	}
	if err := validateExactWorkspaceFile(
		claim.snapshot,
		claim.mount,
		generation.spool.root.mount,
		generation.spool.root.snapshot.identity.Device,
		int64(file.written), //nolint:gosec // Spool writes are capped to 2 GiB.
	); err != nil {
		return workspaceEntryClaim{}, err
	}
	expectedHash := digestFromHasher(file.writeHash)
	if err := validateSpoolSealContent(ctx, file.descriptor, claim, expectedHash); err != nil {
		return workspaceEntryClaim{}, err
	}
	claim.hasContent = true
	claim.contentHash = expectedHash
	if err := validateNamedRetainedWorkspaceLeaf(generation.spool.root, file.name, file.descriptor); err != nil {
		return workspaceEntryClaim{}, err
	}
	if err := validateSealedSpoolReadClaim(ctx, file.descriptor, claim); err != nil {
		return workspaceEntryClaim{}, err
	}
	return claim, nil
}

func digestFromHasher(hasher hash.Hash) Digest {
	var digest Digest
	copy(digest[:], hasher.Sum(nil))
	return digest
}

func validateSpoolSealContent(
	ctx context.Context,
	descriptor *os.File,
	claim workspaceEntryClaim,
	expectedHash Digest,
) error {
	retainedHash, hashErr := hashRetainedFileContent(
		ctx,
		descriptor,
		claim.snapshot.size,
		maximumSpoolBytes,
	)
	if hashErr != nil {
		return hashErr
	}
	afterHash, afterMount, afterACL, afterErr := inspectDescriptorContext(ctx, descriptor)
	if afterErr != nil {
		return afterErr
	}
	if retainedHash != expectedHash || afterHash != claim.snapshot ||
		afterMount != claim.mount || afterACL != claim.aclDigest {
		return fail(CauseUnstable, "object-spool content changed before seal")
	}
	return nil
}

func (generation *spoolGeneration) empty(ctx context.Context) error {
	if generation == nil || generation.spool == nil {
		return fail(CauseInternalInvariant, "missing object-spool generation")
	}
	spool := generation.spool
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if generation.finished {
		return nil
	}
	if err := generation.requireActiveLocked(); err != nil {
		return err
	}
	generation.draining = true
	if err := generation.drainLocked(ctx); err != nil {
		generation.poisoned = true
		return err
	}
	generation.finished = true
	spool.active = nil
	return nil
}

func (generation *spoolGeneration) drainLocked(ctx context.Context) error {
	if err := generation.sealAllFilesLocked(ctx); err != nil {
		return err
	}
	names, err := generation.preflightRemovalLocked(ctx)
	if err != nil {
		return err
	}
	if err := generation.removeSealedFilesLocked(ctx, names); err != nil {
		return err
	}
	return generation.spool.requireEmptyLocked(ctx)
}

func (generation *spoolGeneration) sealAllFilesLocked(ctx context.Context) error {
	for _, file := range generation.files {
		if file.state == spoolFileRemoved {
			continue
		}
		if _, err := generation.sealFileLocked(ctx, file); err != nil {
			return err
		}
	}
	return nil
}

func (generation *spoolGeneration) preflightRemovalLocked(ctx context.Context) ([]string, error) {
	spool := generation.spool
	entries, err := spool.captureInventoryLocked(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateSpoolInventory(
		entries,
		generation.claims,
		spool.root.mount,
		spool.root.snapshot.identity.Device,
	); err != nil {
		return nil, err
	}
	for _, file := range generation.files {
		if file.state == spoolFileRemoved {
			continue
		}
		claim, ok := generation.claims[file.name]
		if !ok || file.state != spoolFileSealed || file.descriptor == nil {
			return nil, fail(CauseInternalInvariant, "missing retained object-spool content claim")
		}
		if err := validateRetainedSpoolContent(ctx, file.descriptor, claim); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(generation.claims))
	for name := range generation.claims {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func validateRetainedSpoolContent(
	ctx context.Context,
	descriptor *os.File,
	expected workspaceEntryClaim,
) error {
	if !expected.hasContent || expected.kind != entryRegular {
		return fail(CauseInternalInvariant, "missing retained object-spool content claim")
	}
	return validateRetainedRegularFileContent(
		ctx,
		descriptor,
		expected.snapshot,
		expected.mount,
		expected.aclDigest,
		expected.contentHash,
		maximumSpoolBytes,
	)
}

func (generation *spoolGeneration) removeSealedFilesLocked(ctx context.Context, names []string) error {
	for _, name := range names {
		claim := generation.claims[name]
		file := generation.fileByNameLocked(name)
		if file == nil || file.state != spoolFileSealed || file.descriptor == nil {
			return fail(CauseInternalInvariant, "missing retained object-spool file")
		}
		if err := generation.spool.removeClaimedFileLocked(ctx, file, claim); err != nil {
			return err
		}
		closeErr := closeDescriptorFailure(file.descriptor.Close())
		file.descriptor = nil
		file.reader = nil
		file.state = spoolFileRemoved
		delete(generation.claims, name)
		if closeErr != nil {
			generation.spool.closeErr = joinFilesystemFailures(generation.spool.closeErr, closeErr)
			return closeErr
		}
	}
	return nil
}

func (generation *spoolGeneration) requireActiveLocked() error {
	if generation == nil || generation.spool == nil || generation.spool.closed {
		return fail(CauseInternalInvariant, "object-spool generation is not live")
	}
	if generation.finished || generation.spool.active != generation {
		return fail(CauseInternalInvariant, "object-spool generation ownership changed")
	}
	if generation.poisoned {
		return fail(CauseUnstable, "object-spool generation is non-removable")
	}
	return nil
}

func (generation *spoolGeneration) ownsFileLocked(file *spoolFile) bool {
	if file == nil || file.generation != generation {
		return false
	}
	return slices.Contains(generation.files, file)
}

func (generation *spoolGeneration) fileByNameLocked(name string) *spoolFile {
	for _, file := range generation.files {
		if file.name == name {
			return file
		}
	}
	return nil
}

func (spool *objectSpool) requireEmpty(ctx context.Context) error {
	if spool == nil {
		return fail(CauseInternalInvariant, "missing cleanup-owned object spool")
	}
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if spool.active != nil {
		return fail(CauseUnstable, "object-spool generation remains active")
	}
	return spool.requireEmptyLocked(ctx)
}

func (spool *objectSpool) hasIncompleteGeneration() bool {
	if spool == nil {
		return false
	}
	spool.mu.Lock()
	defer spool.mu.Unlock()
	return spool.active != nil
}

func (spool *objectSpool) requireEmptyLocked(ctx context.Context) error {
	if spool == nil || spool.root == nil || spool.closed {
		return fail(CauseInternalInvariant, "missing live object-spool root")
	}
	entries, err := spool.captureInventoryLocked(ctx)
	if err != nil {
		return err
	}
	if err := validateSpoolInventory(entries, nil, spool.root.mount, spool.root.snapshot.identity.Device); err != nil {
		return err
	}
	if len(entries) != 0 {
		return fail(CauseUnstable, "object spool is not empty")
	}
	return nil
}

func (spool *objectSpool) captureInventory(ctx context.Context) ([]workspaceEntryClaim, error) {
	if spool == nil {
		return nil, fail(CauseInternalInvariant, "missing cleanup-owned object spool")
	}
	spool.mu.Lock()
	defer spool.mu.Unlock()
	return spool.captureInventoryLocked(ctx)
}

func (spool *objectSpool) captureInventoryLocked(ctx context.Context) ([]workspaceEntryClaim, error) {
	if spool.root == nil || spool.closed {
		return nil, fail(CauseInternalInvariant, "missing live object-spool root")
	}
	if err := validateMutableWorkspaceDirectory(ctx, spool.root, spool.root.snapshot); err != nil {
		return nil, err
	}
	scan, err := spool.root.root.Open(".")
	if err != nil {
		return nil, failWithFilesystemCauses(err, classifyPathError(err), "open object spool")
	}
	before, beforeMount, beforeACL, inspectErr := inspectDescriptorContext(ctx, scan)
	if inspectErr != nil {
		return nil, joinFilesystemFailures(inspectErr, closeDescriptorFailure(scan.Close()))
	}
	entries, scanErr := scanSpoolEntries(ctx, scan)
	after, afterMount, afterACL, afterErr := inspectDescriptorContext(ctx, scan)
	closeErr := closeDescriptorFailure(scan.Close())
	if scanErr != nil || afterErr != nil || closeErr != nil {
		return nil, joinFilesystemFailures(scanErr, afterErr, closeErr)
	}
	if before != after || beforeMount != afterMount || beforeACL != afterACL {
		return nil, fail(CauseUnstable, "object spool changed during inventory")
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].name < entries[right].name })
	return entries, nil
}

func (spool *objectSpool) close() error {
	if spool == nil {
		return nil
	}
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if spool.closed {
		return spool.closeErr
	}
	spool.closed = true
	failures := []error{spool.closeErr}
	if spool.active != nil {
		for _, file := range spool.active.files {
			if file.descriptor == nil {
				continue
			}
			failures = append(failures, closeDescriptorFailure(file.descriptor.Close()))
			file.descriptor = nil
			file.reader = nil
		}
	}
	if spool.root != nil {
		failures = append(failures, closeDescriptorFailure(spool.root.close()))
	}
	spool.closeErr = joinFilesystemFailures(failures...)
	return spool.closeErr
}

func scanSpoolEntries(ctx context.Context, scan *os.File) ([]workspaceEntryClaim, error) {
	entries := make([]workspaceEntryClaim, 0)
	for {
		if err := checkContext(ctx, "scan object spool"); err != nil {
			return nil, err
		}
		rows, readErr := scan.ReadDir(directoryReadBatchSize)
		for _, row := range rows {
			claim, err := captureSpoolEntry(ctx, scan, row.Name())
			if err != nil {
				return nil, err
			}
			entries = append(entries, claim)
			if uint64(len(entries)) > maximumSpoolFiles {
				return nil, fail(CauseLimit, "object-spool file limit exceeded")
			}
		}
		if errors.Is(readErr, io.EOF) {
			return entries, nil
		}
		if readErr != nil {
			return nil, failWithFilesystemCauses(readErr, classifyPathError(readErr), "read object spool")
		}
	}
}

func captureSpoolEntry(ctx context.Context, scan *os.File, name string) (workspaceEntryClaim, error) {
	kind, err := relativeEntryKindNoFollow(int(scan.Fd()), name)
	if err != nil {
		return workspaceEntryClaim{}, failWithFilesystemCauses(err, classifyPathError(err), "inspect object-spool entry kind")
	}
	if kind != entryRegular {
		return workspaceEntryClaim{}, fail(CauseUnsupported, "object-spool entry is not regular")
	}
	descriptor, err := openRelativeNoFollow(int(scan.Fd()), name, entryRegular)
	if err != nil {
		return workspaceEntryClaim{}, failWithFilesystemCauses(err, classifyPathError(err), "open object-spool entry")
	}
	claim, inspectErr := inspectWorkspaceEntry(ctx, name, descriptor, entryRegular)
	closeErr := closeDescriptorFailure(descriptor.Close())
	return claim, joinFilesystemFailures(inspectErr, closeErr)
}

func validateSpoolInventory(
	entries []workspaceEntryClaim,
	expected map[string]workspaceEntryClaim,
	mount mountSnapshot,
	device uint64,
) error {
	if uint64(len(entries)) > maximumSpoolFiles {
		return fail(CauseLimit, "object-spool file limit exceeded")
	}
	uid, err := effectiveUserID()
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(entries))
	var total uint64
	for _, entry := range entries {
		if _, duplicate := seen[entry.name]; duplicate {
			return fail(CauseIdentity, "duplicate object-spool entry")
		}
		seen[entry.name] = struct{}{}
		size, err := validateSpoolEntry(entry, uid, mount, device)
		if err != nil {
			return err
		}
		if total > maximumSpoolBytes || size > maximumSpoolBytes-total {
			return fail(CauseLimit, "object-spool byte limit exceeded")
		}
		total += size
		if err := validateExpectedSpoolEntry(entry, expected); err != nil {
			return err
		}
	}
	if expected != nil && len(expected) != len(entries) {
		return fail(CauseUnstable, "object-spool generation membership changed")
	}
	return nil
}

func validateSpoolEntry(entry workspaceEntryClaim, uid uint32, mount mountSnapshot, device uint64) (uint64, error) {
	if entry.name == "" || filepath.Base(entry.name) != entry.name || entry.name == "." || entry.name == ".." {
		return 0, fail(CauseMalformed, "object-spool entry name refused")
	}
	if entry.kind != entryRegular || snapshotKind(entry.snapshot) != entryRegular {
		return 0, fail(CauseUnsupported, "object-spool entry is not regular")
	}
	if entry.snapshot.identity.UID != uid || entry.snapshot.identity.Mode&0o7777 != 0o600 {
		return 0, fail(CausePermission, "object-spool entry owner or mode refused")
	}
	if entry.snapshot.linkCount != 1 {
		return 0, fail(CauseIdentity, "object-spool hard-linked alias refused")
	}
	if entry.mount != mount || entry.snapshot.identity.Device != device {
		return 0, fail(CauseUnsupported, "object-spool mount transition refused")
	}
	if entry.snapshot.size < 0 {
		return 0, fail(CauseMalformed, "object-spool negative size refused")
	}
	return uint64(entry.snapshot.size), nil
}

func validateExpectedSpoolEntry(
	entry workspaceEntryClaim,
	expected map[string]workspaceEntryClaim,
) error {
	if expected == nil {
		return nil
	}
	claim, ok := expected[entry.name]
	if !ok || claim.snapshot != entry.snapshot || claim.mount != entry.mount || claim.aclDigest != entry.aclDigest {
		return fail(CauseUnstable, "object-spool generation changed")
	}
	return nil
}

func (spool *objectSpool) removeClaimedFileLocked(
	ctx context.Context,
	file *spoolFile,
	expected workspaceEntryClaim,
) error {
	if spool == nil || file == nil || file.descriptor == nil || file.state != spoolFileSealed {
		return fail(CauseInternalInvariant, "missing retained sealed object-spool file")
	}
	return removeRetainedWorkspaceLeafWith(
		ctx,
		spool.root,
		expected,
		file.descriptor,
		func() error { return spool.root.root.Remove(expected.name) },
	)
}

func (workspace *taskWorkspace) cleanup(ctx context.Context, primary *FailureRecord) error {
	if workspace == nil {
		return newRefusal(FailureReport{Primary: &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationValidate,
			Causes:    []CauseCode{CauseInternalInvariant},
		}})
	}
	workspace.cleanupOnce.Do(func() {
		workspace.lifecycle.Lock()
		defer workspace.lifecycle.Unlock()
		workspace.closed = true
		recovery := workspace.recoveryInfo()
		nonRootClosers := make([]func() error, 0, 1)
		if workspace.spool != nil {
			nonRootClosers = append(nonRootClosers, workspace.spool.close)
		}
		var closeTaskRoot func() error
		if workspace.taskRoot != nil {
			closeTaskRoot = workspace.taskRoot.close
		} else if workspace.unobservedRoot != nil {
			closeTaskRoot = workspace.unobservedRoot.close
		}
		var closeStateRoot func() error
		if workspace.stateRoot != nil {
			closeStateRoot = workspace.stateRoot.close
		}
		workspace.cleanupErr = runCleanup(cleanupTransaction{
			primary:        primary,
			quiesce:        func() quiescenceResult { return quiescenceResult{proven: true} },
			nonRootClosers: nonRootClosers,
			remove:         func() removalResult { return workspace.removeOwnedInitialAllocation(ctx) },
			observe:        func() removalResult { return workspace.observeTaskAllocation(ctx, nil) },
			closeTaskRoot:  closeTaskRoot,
			closeStateRoot: closeStateRoot,
			recovery:       recovery,
		})
	})
	return workspace.cleanupErr
}

func (workspace *taskWorkspace) isClosed() bool {
	if workspace == nil {
		return true
	}
	workspace.lifecycle.RLock()
	defer workspace.lifecycle.RUnlock()
	return workspace.closed
}

func (workspace *taskWorkspace) recoveryInfo() RecoveryInfo {
	recovery := RecoveryInfo{TaskPath: workspace.paths.taskRoot}
	if workspace.stateRoot != nil {
		recovery.StateRoot = workspace.stateRoot.snapshot.identity
	}
	if workspace.taskRoot != nil {
		identity := workspace.taskRoot.snapshot.identity
		recovery.TaskRoot = &identity
	}
	return recovery
}

func (workspace *taskWorkspace) removeOwnedInitialAllocation(ctx context.Context) removalResult {
	if workspace.taskRoot == nil {
		return removalResult{disposition: dispositionUnknown, causes: []CauseCode{CauseIdentity}}
	}
	if err := workspace.preflightOwnedInitialEntries(ctx); err != nil {
		return workspace.observeTaskAllocation(ctx, privateCauses(err, CauseCleanup))
	}
	if workspace.beforeRemove != nil {
		if err := workspace.beforeRemove(); err != nil {
			return workspace.observeTaskAllocation(ctx, privateCauses(err, CauseCleanup))
		}
	}
	for index, entry := range slices.Backward(workspace.ownedEntries) {
		if err := workspace.preflightOwnedEntries(ctx, workspace.ownedEntries[:index+1]); err != nil {
			return workspace.observeTaskAllocation(ctx, privateCauses(err, CauseCleanup))
		}
		if err := workspace.removeOwnedInitialEntry(ctx, entry); err != nil {
			return workspace.observeTaskAllocation(ctx, privateCauses(err, CauseCleanup))
		}
		parent := filepath.Dir(entry.path)
		if parent != "." {
			if err := workspace.refreshOwnedDirectoryClaim(ctx, parent); err != nil {
				return workspace.observeTaskAllocation(ctx, privateCauses(err, CauseCleanup))
			}
		}
	}
	if err := removeRetainedTaskDirectory(ctx, workspace.stateRoot, workspace.taskRoot, workspace.taskName); err != nil {
		return workspace.observeTaskAllocation(ctx, privateCauses(err, CauseCleanup))
	}
	return removalResult{disposition: dispositionRemoved}
}

func (workspace *taskWorkspace) observeTaskAllocation(
	ctx context.Context,
	causes []CauseCode,
) removalResult {
	if workspace == nil || workspace.stateRoot == nil || workspace.taskRoot == nil {
		return removalResult{
			disposition: dispositionUnknown,
			causes:      append(append([]CauseCode(nil), causes...), CauseIdentity),
		}
	}
	if err := validateNamedRetainedTask(
		ctx,
		workspace.stateRoot,
		workspace.taskRoot,
		workspace.taskName,
		retainedDescriptorPath,
	); err != nil {
		return removalResult{
			disposition: dispositionUnknown,
			causes:      append(append([]CauseCode(nil), causes...), privateCauses(err, CauseCleanup)...),
		}
	}
	return removalResult{disposition: dispositionPresent, causes: append([]CauseCode(nil), causes...)}
}

func (workspace *taskWorkspace) preflightOwnedInitialEntries(ctx context.Context) error {
	return workspace.preflightOwnedEntries(ctx, workspace.ownedEntries)
}

func (workspace *taskWorkspace) preflightOwnedEntries(
	ctx context.Context,
	entries []ownedWorkspaceEntry,
) error {
	if workspace == nil || workspace.taskRoot == nil {
		return fail(CauseInternalInvariant, "missing retained initial workspace")
	}
	// Bind the complete ledger to the still-named retained task before any
	// descendant is unlinked. A renamed or replacement task preserves both the
	// retained original and whatever now occupies the recorded locator.
	if err := validateNamedRetainedTask(
		ctx,
		workspace.stateRoot,
		workspace.taskRoot,
		workspace.taskName,
		retainedDescriptorPath,
	); err != nil {
		return err
	}
	// A generation owns derived spool state until it proves its complete
	// inventory removed. Even a zero-file generation is therefore a cleanup
	// preservation barrier.
	if workspace.spool != nil && workspace.spool.hasIncompleteGeneration() {
		return fail(CauseUnstable, "object-spool generation remains incomplete")
	}
	if err := requireObservedOwnedInitialEntries(entries); err != nil {
		return err
	}
	if err := requireExactRetainedNames(ctx, workspace.taskRoot, ownedInitialTopLevelNames(entries)); err != nil {
		return err
	}
	if err := workspace.revalidateOwnedInitialEntries(ctx, entries); err != nil {
		return err
	}
	return workspace.preflightOwnedInitialDirectories(ctx, entries)
}

func requireObservedOwnedInitialEntries(entries []ownedWorkspaceEntry) error {
	for _, entry := range entries {
		if !entry.observed {
			return fail(CauseIdentity, "initial workspace entry identity was not observed")
		}
	}
	return nil
}

func ownedInitialTopLevelNames(entries []ownedWorkspaceEntry) []string {
	expected := make([]string, 0, len(entries))
	for _, entry := range entries {
		if filepath.Dir(entry.path) == "." {
			expected = append(expected, entry.path)
		}
	}
	return expected
}

func (workspace *taskWorkspace) revalidateOwnedInitialEntries(
	ctx context.Context,
	entries []ownedWorkspaceEntry,
) error {
	for _, entry := range entries {
		if err := workspace.revalidateOwnedInitialEntry(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

func (workspace *taskWorkspace) preflightOwnedInitialDirectories(
	ctx context.Context,
	entries []ownedWorkspaceEntry,
) error {
	for _, entry := range entries {
		if entry.kind != entryDirectory {
			continue
		}
		directory, err := workspace.openDirectoryRoot(ctx, entry.path)
		if err != nil {
			return err
		}
		expectedChildren := ownedInitialChildNames(entries, entry.path)
		validateErr := requireExactRetainedNames(ctx, directory, expectedChildren)
		closeErr := closeDescriptorFailure(directory.close())
		if validateErr != nil || closeErr != nil {
			return joinFilesystemFailures(validateErr, closeErr)
		}
	}
	return nil
}

func ownedInitialChildNames(entries []ownedWorkspaceEntry, parent string) []string {
	children := make([]string, 0)
	for _, entry := range entries {
		if filepath.Dir(entry.path) == parent {
			children = append(children, filepath.Base(entry.path))
		}
	}
	return children
}

func requireExactRetainedNames(ctx context.Context, directory *retainedDirectory, expected []string) error {
	if directory == nil || directory.root == nil || directory.descriptor == nil {
		return fail(CauseInternalInvariant, "missing retained workspace inventory root")
	}
	if err := validateMutableWorkspaceDirectory(ctx, directory, directory.snapshot); err != nil {
		return err
	}
	scan, err := directory.root.Open(".")
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "open owned workspace inventory")
	}
	namedInfo, namedErr := scan.Stat()
	retainedInfo, retainedErr := directory.descriptor.Stat()
	if namedErr != nil || retainedErr != nil || !os.SameFile(namedInfo, retainedInfo) {
		return joinFilesystemFailures(
			failWithFilesystemCauses(
				errors.Join(namedErr, retainedErr),
				CauseIdentity,
				"owned workspace inventory root identity changed",
			),
			closeDescriptorFailure(scan.Close()),
		)
	}
	maximum := len(expected) + 1
	got, readErr := readStableDirectoryNames(ctx, scan, maximum)
	closeErr := closeDescriptorFailure(scan.Close())
	if readErr != nil || closeErr != nil {
		return joinFilesystemFailures(readErr, closeErr)
	}
	want := append([]string(nil), expected...)
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		return fail(CauseIdentity, "owned workspace inventory changed")
	}
	for index := range want {
		if got[index] != want[index] {
			return fail(CauseIdentity, "owned workspace inventory changed")
		}
	}
	return nil
}

func (workspace *taskWorkspace) revalidateOwnedInitialEntry(
	ctx context.Context,
	entry ownedWorkspaceEntry,
) error {
	descriptor, err := openRelativeNoFollow(int(workspace.taskRoot.descriptor.Fd()), entry.path, entry.kind)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "reopen owned initial workspace entry")
	}
	current, inspectErr := inspectWorkspaceEntry(ctx, entry.path, descriptor, entry.kind)
	if inspectErr == nil {
		inspectErr = workspace.completeOwnedInitialLinkClaim(ctx, descriptor, &current)
	}
	closeErr := closeDescriptorFailure(descriptor.Close())
	if inspectErr != nil || closeErr != nil {
		return joinFilesystemFailures(inspectErr, closeErr)
	}
	return workspace.validateOwnedInitialEntryClaim(entry, current)
}

func (workspace *taskWorkspace) completeOwnedInitialLinkClaim(
	ctx context.Context,
	descriptor *os.File,
	current *workspaceEntryClaim,
) error {
	if current == nil || current.kind != entrySymlink {
		return nil
	}
	linkText, err := readOpenedSymlinkContext(ctx, descriptor, maxSymlinkBytes)
	if err != nil {
		return err
	}
	if err := workspace.git.revalidate(ctx); err != nil {
		return err
	}
	current.linkText = linkText
	current.targetFile = workspace.git.snapshot
	return nil
}

func (workspace *taskWorkspace) validateOwnedInitialEntryClaim(
	entry ownedWorkspaceEntry,
	current workspaceEntryClaim,
) error {
	if entry.kind == entryDirectory {
		if !stableWorkspaceClaimEqual(entry.claim, current) ||
			current.snapshot.linkCount != entry.claim.snapshot.linkCount {
			return fail(CauseIdentity, "owned initial workspace directory changed: "+entry.path)
		}
		return validateExactWorkspaceDirectory(
			current.snapshot,
			current.mount,
			workspace.taskRoot.mount,
			workspace.taskRoot.snapshot.identity.Device,
			0o700,
		)
	}
	if current.snapshot != entry.claim.snapshot || current.mount != entry.claim.mount ||
		current.aclDigest != entry.claim.aclDigest || current.linkText != entry.claim.linkText ||
		current.targetFile != entry.claim.targetFile {
		return fail(CauseIdentity, "owned initial workspace leaf changed")
	}
	return nil
}

func (workspace *taskWorkspace) removeOwnedInitialEntry(
	ctx context.Context,
	entry ownedWorkspaceEntry,
) error {
	switch entry.kind {
	case entryRegular:
		return removeRetainedWorkspaceLeaf(ctx, workspace.taskRoot, entry.claim)
	case entrySymlink:
		parentName := filepath.Dir(entry.path)
		parent, err := workspace.openDirectoryRoot(ctx, parentName)
		if err != nil {
			return err
		}
		expected := entry.claim
		expected.name = filepath.Base(entry.path)
		removeErr := removeRetainedWorkspaceLeaf(ctx, parent, expected)
		closeErr := closeDescriptorFailure(parent.close())
		return joinFilesystemFailures(removeErr, closeErr)
	case entryDirectory:
		child, err := workspace.openDirectoryRoot(ctx, entry.path)
		if err != nil {
			return err
		}
		removeErr := removeRetainedTaskDirectory(ctx, workspace.taskRoot, child, entry.path)
		closeErr := closeDescriptorFailure(child.close())
		return joinFilesystemFailures(removeErr, closeErr)
	default:
		return fail(CauseInternalInvariant, "unknown owned workspace entry kind")
	}
}

// workspacePaths is the validated private namespace from which every writable
// child path is derived. Its fields remain private so later execution code
// cannot substitute a caller path for one member of the fixed workspace.
type workspacePaths struct {
	taskRoot string
}

func newWorkspacePaths(taskRoot string) (workspacePaths, error) {
	if taskRoot == "" || !filepath.IsAbs(taskRoot) || filepath.Clean(taskRoot) != taskRoot {
		return workspacePaths{}, fail(CauseInternalInvariant, "invalid private workspace path")
	}
	return workspacePaths{taskRoot: taskRoot}, nil
}

func (paths workspacePaths) child(name string) string {
	return filepath.Join(paths.taskRoot, name)
}

// preallocationEnvironment is the complete environment for probes that must
// run before a private task workspace exists. It retains only the authenticated
// Go installation and module cache paths.
type preallocationEnvironment struct {
	goroot     string
	gomodcache string
}

// taskPrivateEnvironment is the complete environment for commands admitted
// only after the private task workspace exists.
type taskPrivateEnvironment struct {
	paths      workspacePaths
	goroot     string
	gomodcache string
}

func newPreallocationEnvironment(goroot, gomodcache string) (preallocationEnvironment, error) {
	if err := validateEnvironmentAuthorityPaths(goroot, gomodcache); err != nil {
		return preallocationEnvironment{}, err
	}
	return preallocationEnvironment{goroot: goroot, gomodcache: gomodcache}, nil
}

func newTaskPrivateEnvironment(
	paths workspacePaths,
	goroot, gomodcache string,
) (taskPrivateEnvironment, error) {
	if !validEnvironmentAuthorityPath(paths.taskRoot) {
		return taskPrivateEnvironment{}, fail(CauseInternalInvariant, "missing private workspace")
	}
	if err := validateEnvironmentAuthorityPaths(goroot, gomodcache); err != nil {
		return taskPrivateEnvironment{}, err
	}
	return taskPrivateEnvironment{paths: paths, goroot: goroot, gomodcache: gomodcache}, nil
}

func validateEnvironmentAuthorityPaths(paths ...string) error {
	for _, path := range paths {
		if !validEnvironmentAuthorityPath(path) {
			return fail(CauseInternalInvariant, "invalid retained authority path")
		}
	}
	return nil
}

func validEnvironmentAuthorityPath(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path &&
		!strings.ContainsRune(path, '\x00')
}

func (environment preallocationEnvironment) valid() bool {
	return validateEnvironmentAuthorityPaths(environment.goroot, environment.gomodcache) == nil
}

func (environment taskPrivateEnvironment) valid() bool {
	return validEnvironmentAuthorityPath(environment.paths.taskRoot) &&
		validateEnvironmentAuthorityPaths(environment.goroot, environment.gomodcache) == nil
}

func (environment preallocationEnvironment) clone() []string {
	entries := commonEnvironmentEntries(environment.goroot, environment.gomodcache)
	entries = append(entries,
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_EXEC_PATH=/dev/null",
		"GIT_TEMPLATE_DIR=/dev/null",
		"GOCACHE=off",
		"GOPATH=/dev/null",
		"GOTMPDIR=/dev/null",
		"PATH=/dev/null",
		"TEMP=/dev/null",
		"TMP=/dev/null",
		"TMPDIR=/dev/null",
	)
	sort.Strings(entries)
	return entries
}

func (environment taskPrivateEnvironment) clone() []string {
	entries := commonEnvironmentEntries(environment.goroot, environment.gomodcache)
	entries = append(entries,
		"GIT_CONFIG_GLOBAL="+environment.paths.child(workspaceGitGlobal),
		"GIT_EXEC_PATH="+environment.paths.child(workspaceGitExec),
		"GIT_TEMPLATE_DIR="+environment.paths.child(workspaceGitTemplate),
		"GOCACHE="+environment.paths.child(workspaceGoCache),
		"GOPATH="+environment.paths.child(workspaceGoPath),
		"GOTMPDIR="+environment.paths.child(workspaceGoTemporary),
		"PATH="+environment.paths.child(workspaceGitPath),
		"TEMP="+environment.paths.child(workspaceTemporary),
		"TMP="+environment.paths.child(workspaceTemporary),
		"TMPDIR="+environment.paths.child(workspaceTemporary),
	)
	sort.Strings(entries)
	return entries
}

// commonEnvironmentEntries starts from an empty slice on every call. It
// contains only the exact rows shared by both closed profiles.
func commonEnvironmentEntries(goroot, gomodcache string) []string {
	return []string{
		"CGO_ENABLED=0",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PROTOCOL_FROM_USER=0",
		"GIT_TERMINAL_PROMPT=0",
		"GO111MODULE=on",
		"GOARCH=arm64",
		"GOARM64=v8.0",
		"GOENV=off",
		"GOEXPERIMENT=none",
		"GO_EXTLINK_ENABLED=0",
		"GOFIPS140=off",
		"GOFLAGS=-mod=readonly",
		"GOMODCACHE=" + gomodcache,
		"GOOS=darwin",
		"GOPROXY=off",
		"GOROOT=" + goroot,
		"GOSUMDB=off",
		"GOTOOLCHAIN=local",
		"GOVCS=*:off",
		"GOWORK=off",
		"HOME=",
		"LANG=C",
		"LC_ALL=C",
		"TZ=UTC",
		"XDG_CONFIG_HOME=",
	}
}

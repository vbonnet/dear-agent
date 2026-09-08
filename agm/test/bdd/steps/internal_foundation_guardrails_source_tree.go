//go:build darwin || linux

package steps

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"math"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
)

type buildAuthorityWaitOwnershipTree struct {
	ctx          context.Context
	root         *os.File
	limits       buildAuthorityWaitOwnershipLimits
	hooks        buildAuthorityWaitOwnershipHooks
	result       *buildAuthorityWaitOwnershipScan
	entries      int
	directories  int
	parsedFiles  []buildAuthorityWaitOwnershipParsedFile
	snapshot     []buildAuthorityWaitOwnershipSnapshotEntry
	snapshotSeen map[string]struct{}
}

type buildAuthorityWaitOwnershipParsedFile struct {
	rel     string
	fileSet *token.FileSet
	file    *ast.File
}

type buildAuthorityWaitOwnershipSnapshotEntry struct {
	rel       string
	info      fs.FileInfo
	directory bool
}

const (
	buildAuthorityDirectoryReadBatch = 128
	buildAuthorityMaxDirectoryDepth  = 128
)

func scanBuildAuthorityWaitOwnershipSecure(
	ctx context.Context,
	repoRoot string,
	limits buildAuthorityWaitOwnershipLimits,
	hooks buildAuthorityWaitOwnershipHooks,
) (buildAuthorityWaitOwnershipScan, error) {
	var result buildAuthorityWaitOwnershipScan
	if err := validateBuildAuthorityWaitOwnershipScan(ctx, limits); err != nil {
		return result, err
	}
	absoluteRoot, canonicalRoot, root, rootInfo, err := openBuildAuthorityWaitOwnershipScanRoot(repoRoot)
	if err != nil {
		return result, err
	}
	walker := buildAuthorityWaitOwnershipTree{
		ctx:          ctx,
		root:         root,
		limits:       limits,
		hooks:        hooks,
		result:       &result,
		directories:  1,
		snapshotSeen: make(map[string]struct{}),
	}
	walkErr := walker.walkDirectory(root, rootInfo, ".", absoluteRoot, 0)
	var snapshotErr error
	if walkErr == nil {
		snapshotErr = scanBuildAuthorityWaitOwnershipParsedFiles(ctx, &result, walker.parsedFiles)
		if snapshotErr == nil && hooks.beforeSnapshotValidation != nil {
			snapshotErr = hooks.beforeSnapshotValidation()
		}
		if snapshotErr == nil {
			snapshotErr = walker.revalidateSnapshot()
		}
	}
	verifyErr := verifyBuildAuthorityWaitOwnershipScanRoot(
		root, rootInfo, absoluteRoot, canonicalRoot,
	)
	if walkErr != nil || snapshotErr != nil || verifyErr != nil {
		sort.Strings(result.violations)
		return result, errors.Join(walkErr, snapshotErr, verifyErr)
	}
	sort.Strings(result.violations)
	return result, nil
}

func scanBuildAuthorityWaitOwnershipParsedFiles(
	ctx context.Context,
	result *buildAuthorityWaitOwnershipScan,
	parsedFiles []buildAuthorityWaitOwnershipParsedFile,
) error {
	packageNames := make(map[string]map[string]struct{})
	for _, parsed := range parsedFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := buildAuthorityWaitOwnershipPackageKey(parsed)
		names := packageNames[key]
		if names == nil {
			names = make(map[string]struct{})
			packageNames[key] = names
		}
		collectBuildAuthorityWaitOwnershipPackageNames(parsed.file, names)
	}
	for _, parsed := range parsedFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		scanBuildAuthorityWaitOwnershipFile(
			result,
			parsed.rel,
			parsed.fileSet,
			parsed.file,
			packageNames[buildAuthorityWaitOwnershipPackageKey(parsed)],
		)
	}
	return ctx.Err()
}

func buildAuthorityWaitOwnershipPackageKey(parsed buildAuthorityWaitOwnershipParsedFile) string {
	return pathpkg.Dir(parsed.rel) + "\x00" + parsed.file.Name.Name
}

func collectBuildAuthorityWaitOwnershipPackageNames(file *ast.File, names map[string]struct{}) {
	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			if value.Recv == nil {
				names[value.Name.Name] = struct{}{}
			}
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				switch named := spec.(type) {
				case *ast.TypeSpec:
					names[named.Name.Name] = struct{}{}
				case *ast.ValueSpec:
					for _, name := range named.Names {
						names[name.Name] = struct{}{}
					}
				}
			}
		}
	}
}

func validateBuildAuthorityWaitOwnershipScan(
	ctx context.Context,
	limits buildAuthorityWaitOwnershipLimits,
) error {
	if ctx == nil {
		return fmt.Errorf("production-source scan requires a non-nil context")
	}
	if limits.maxEntries <= 0 || limits.maxDirectories <= 0 || limits.maxFiles <= 0 ||
		limits.maxFileBytes <= 0 || limits.maxTotalBytes <= 0 ||
		limits.maxFileBytes == math.MaxInt64 {
		return fmt.Errorf("invalid production-source scan limits")
	}
	return ctx.Err()
}

func openBuildAuthorityWaitOwnershipScanRoot(
	repoRoot string,
) (string, string, *os.File, fs.FileInfo, error) {
	absoluteRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("resolve production-source root %s: %w", repoRoot, err)
	}
	originalInfo, err := os.Lstat(absoluteRoot)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("stat production-source root %s: %w", absoluteRoot, err)
	}
	if originalInfo.Mode()&os.ModeSymlink != 0 || !originalInfo.IsDir() {
		return "", "", nil, nil,
			fmt.Errorf("production-source root %s is not a non-symlink directory", absoluteRoot)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", "", nil, nil,
			fmt.Errorf("canonicalize production-source root %s: %w", absoluteRoot, err)
	}
	root, rootInfo, err := openBuildAuthorityWaitOwnershipRoot(canonicalRoot)
	if err != nil {
		return "", "", nil, nil,
			fmt.Errorf("open production-source root %s: %w", canonicalRoot, err)
	}
	if !buildAuthorityWaitOwnershipSameDirectory(originalInfo, rootInfo) {
		return "", "", nil, nil, errors.Join(
			fmt.Errorf("production-source root identity changed before descriptor open"),
			root.Close(),
		)
	}
	return absoluteRoot, canonicalRoot, root, rootInfo, nil
}

func verifyBuildAuthorityWaitOwnershipScanRoot(
	root *os.File,
	expected fs.FileInfo,
	absoluteRoot string,
	canonicalRoot string,
) error {
	postWalkInfo, statErr := root.Stat()
	closeErr := root.Close()
	if statErr != nil || closeErr != nil {
		return errors.Join(statErr, closeErr)
	}
	if !buildAuthorityWaitOwnershipSameDirectory(expected, postWalkInfo) {
		return fmt.Errorf("production-source root changed while scanning")
	}
	finalPathInfo, err := os.Lstat(absoluteRoot)
	if err != nil {
		return fmt.Errorf("restat production-source root %s: %w", absoluteRoot, err)
	}
	if !buildAuthorityWaitOwnershipSameDirectory(expected, finalPathInfo) {
		return fmt.Errorf("production-source root path identity changed while scanning")
	}
	reopenedRoot, reopenedInfo, err := openBuildAuthorityWaitOwnershipRoot(canonicalRoot)
	if err != nil {
		return fmt.Errorf("reopen production-source root %s: %w", canonicalRoot, err)
	}
	if closeErr := reopenedRoot.Close(); closeErr != nil {
		return closeErr
	}
	if !buildAuthorityWaitOwnershipSameDirectory(expected, reopenedInfo) {
		return fmt.Errorf("production-source root descriptor identity changed while scanning")
	}
	return nil
}

func (walker *buildAuthorityWaitOwnershipTree) walkDirectory(
	directory *os.File,
	expected fs.FileInfo,
	rel string,
	displayPath string,
	depth int,
) error {
	if err := walker.ctx.Err(); err != nil {
		return err
	}
	if depth > buildAuthorityMaxDirectoryDepth {
		return fmt.Errorf("repository directory depth exceeds bounded scan limit %d", buildAuthorityMaxDirectoryDepth)
	}
	for {
		entries, readErr := walker.readDirectory(directory)
		sort.Slice(entries, func(first, second int) bool {
			return entries[first].Name() < entries[second].Name()
		})
		for _, entry := range entries {
			if err := walker.walkEntry(directory, rel, displayPath, depth, entry); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read production-source directory %s: %w", rel, readErr)
		}
	}
	postInfo, err := directory.Stat()
	if err != nil {
		return fmt.Errorf("restat production-source directory %s: %w", rel, err)
	}
	if !buildAuthorityWaitOwnershipSameDirectory(expected, postInfo) {
		return fmt.Errorf("production-source directory %s changed while scanning", rel)
	}
	return nil
}

func (walker *buildAuthorityWaitOwnershipTree) readDirectory(
	directory *os.File,
) ([]fs.DirEntry, error) {
	if walker.hooks.readDirectory != nil {
		return walker.hooks.readDirectory(directory, buildAuthorityDirectoryReadBatch)
	}
	return directory.ReadDir(buildAuthorityDirectoryReadBatch)
}

func (walker *buildAuthorityWaitOwnershipTree) walkEntry(
	directory *os.File,
	parentRel string,
	displayPath string,
	depth int,
	entry fs.DirEntry,
) error {
	if err := walker.ctx.Err(); err != nil {
		return err
	}
	if walker.entries >= walker.limits.maxEntries {
		return fmt.Errorf("repository entry count exceeds bounded scan limit %d", walker.limits.maxEntries)
	}
	walker.entries++
	name := entry.Name()
	rel := name
	if parentRel != "." {
		rel = parentRel + "/" + name
	}
	childPath := filepath.Join(displayPath, name)
	opened, info, symlink, openErr := openBuildAuthorityWaitOwnershipAt(directory, name)
	if openErr != nil {
		return fmt.Errorf("open repository entry %s relative to retained directory: %w", rel, openErr)
	}
	if symlink {
		return walker.handleSymlinkEntry(directory, parentRel, rel, name, entry)
	}
	if err := validateBuildAuthorityWaitOwnershipOpenedEntry(entry, opened, info, rel); err != nil {
		return err
	}
	if !info.IsDir() {
		return walker.scanOpenedFile(directory, opened, info, rel, childPath, name)
	}
	return walker.walkOpenedDirectory(directory, opened, info, rel, childPath, name, depth)
}

func (walker *buildAuthorityWaitOwnershipTree) handleSymlinkEntry(
	parent *os.File,
	parentRel string,
	rel string,
	name string,
	entry fs.DirEntry,
) error {
	if entry.Type()&os.ModeSymlink == 0 {
		return fmt.Errorf("repository entry %s changed to a symbolic link during scan", rel)
	}
	return walker.validateSymlink(parent, parentRel, rel, name)
}

func validateBuildAuthorityWaitOwnershipOpenedEntry(
	entry fs.DirEntry,
	opened *os.File,
	info fs.FileInfo,
	rel string,
) error {
	if entry.Type()&os.ModeSymlink != 0 {
		return errors.Join(
			fmt.Errorf("repository symbolic link %s changed identity during scan", rel),
			opened.Close(),
		)
	}
	if entry.Type().IsDir() && !info.IsDir() {
		return errors.Join(
			fmt.Errorf("repository directory %s changed type during scan", rel),
			opened.Close(),
		)
	}
	return nil
}

func (walker *buildAuthorityWaitOwnershipTree) walkOpenedDirectory(
	parent *os.File,
	directory *os.File,
	info fs.FileInfo,
	rel string,
	displayPath string,
	name string,
	depth int,
) error {
	if walker.directories >= walker.limits.maxDirectories {
		return errors.Join(
			fmt.Errorf("repository directory count exceeds bounded scan limit %d", walker.limits.maxDirectories),
			directory.Close(),
		)
	}
	walker.directories++
	if name == "vendor" {
		walker.result.violations = append(walker.result.violations,
			fmt.Sprintf("%s: vendored production source is forbidden by the ownership audit", rel))
		if err := closeAndRevalidateBuildAuthorityDirectory(parent, directory, info, name); err != nil {
			return err
		}
		walker.recordSnapshot(rel, info, true)
		return nil
	}
	if buildAuthorityWaitOwnershipSkippedDir(name) {
		if err := closeAndRevalidateBuildAuthorityDirectory(parent, directory, info, name); err != nil {
			return err
		}
		walker.recordSnapshot(rel, info, true)
		return nil
	}
	walkErr := walker.walkDirectory(directory, info, rel, displayPath, depth+1)
	postInfo, statErr := directory.Stat()
	closeErr := directory.Close()
	if walkErr != nil || statErr != nil || closeErr != nil {
		return errors.Join(walkErr, statErr, closeErr)
	}
	if !buildAuthorityWaitOwnershipSameDirectory(info, postInfo) {
		return fmt.Errorf("repository directory %s changed while scanning", rel)
	}
	if err := revalidateBuildAuthorityWaitOwnershipEntry(parent, name, info, true); err != nil {
		return fmt.Errorf("revalidate repository directory %s: %w", rel, err)
	}
	walker.recordSnapshot(rel, info, true)
	return nil
}

func closeAndRevalidateBuildAuthorityDirectory(
	parent *os.File,
	directory *os.File,
	expected fs.FileInfo,
	name string,
) error {
	postInfo, statErr := directory.Stat()
	closeErr := directory.Close()
	if statErr != nil || closeErr != nil {
		return errors.Join(statErr, closeErr)
	}
	if !buildAuthorityWaitOwnershipSameDirectory(expected, postInfo) {
		return fmt.Errorf("directory changed while applying source exclusion")
	}
	return revalidateBuildAuthorityWaitOwnershipEntry(parent, name, expected, true)
}

func (walker *buildAuthorityWaitOwnershipTree) scanOpenedFile(
	parent *os.File,
	opened *os.File,
	info fs.FileInfo,
	rel string,
	displayPath string,
	name string,
) error {
	ownerFile := filepath.ToSlash(filepath.Dir(rel)) == "internal/buildauthority"
	productionGo := strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	productionNative := buildAuthorityWaitOwnershipNativeSource(name)
	if !info.Mode().IsRegular() {
		err := rejectBuildAuthorityWaitOwnershipNonRegularSource(
			opened, rel, ownerFile, productionGo, productionNative,
		)
		if err == nil {
			walker.recordSnapshot(rel, info, false)
		}
		return err
	}
	if ownerFile {
		return walker.closeAndRecordSnapshotFile(opened, info, rel)
	}
	if productionNative {
		walker.result.violations = append(walker.result.violations,
			fmt.Sprintf("%s: native production source is forbidden outside internal/buildauthority", rel))
		return walker.closeAndRecordSnapshotFile(opened, info, rel)
	}
	if !productionGo {
		return walker.closeAndRecordSnapshotFile(opened, info, rel)
	}
	return walker.scanOpenedGoSource(parent, opened, info, rel, displayPath, name)
}

func rejectBuildAuthorityWaitOwnershipNonRegularSource(
	opened *os.File,
	rel string,
	ownerFile bool,
	productionGo bool,
	productionNative bool,
) error {
	closeErr := opened.Close()
	if !ownerFile && (productionGo || productionNative) {
		return errors.Join(fmt.Errorf("production source %s is not a regular file", rel), closeErr)
	}
	return closeErr
}

func (walker *buildAuthorityWaitOwnershipTree) scanOpenedGoSource(
	parent *os.File,
	opened *os.File,
	info fs.FileInfo,
	rel string,
	displayPath string,
	name string,
) error {
	if err := walker.validateGoSourceLimits(rel, info); err != nil {
		return errors.Join(err, opened.Close())
	}
	opened, info, err := walker.prepareGoSourceDescriptor(parent, opened, info, rel, displayPath, name)
	if err != nil {
		return err
	}
	source, err := readBuildAuthorityWaitOwnershipDescriptor(
		walker.ctx, opened, rel, info, walker.limits.maxFileBytes,
		func() error {
			if walker.hooks.afterSourceOpen == nil {
				return nil
			}
			return walker.hooks.afterSourceOpen(displayPath, rel)
		},
	)
	if err != nil {
		return err
	}
	if err := revalidateBuildAuthorityWaitOwnershipEntry(parent, name, info, false); err != nil {
		return fmt.Errorf("revalidate production Go source %s: %w", rel, err)
	}
	if int64(len(source)) > walker.limits.maxTotalBytes-walker.result.bytes {
		return fmt.Errorf("production Go source bytes exceed %d", walker.limits.maxTotalBytes)
	}
	walker.result.files++
	walker.result.bytes += int64(len(source))
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, rel, source, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse production Go source %s: %w", rel, err)
	}
	walker.parsedFiles = append(walker.parsedFiles, buildAuthorityWaitOwnershipParsedFile{
		rel: rel, fileSet: fileSet, file: file,
	})
	walker.recordSnapshot(rel, info, false)
	return nil
}

func (walker *buildAuthorityWaitOwnershipTree) closeAndRecordSnapshotFile(
	opened *os.File,
	info fs.FileInfo,
	rel string,
) error {
	if err := opened.Close(); err != nil {
		return err
	}
	walker.recordSnapshot(rel, info, false)
	return nil
}

func (walker *buildAuthorityWaitOwnershipTree) recordSnapshot(
	rel string,
	info fs.FileInfo,
	directory bool,
) {
	if _, recorded := walker.snapshotSeen[rel]; recorded {
		return
	}
	walker.snapshotSeen[rel] = struct{}{}
	walker.snapshot = append(walker.snapshot, buildAuthorityWaitOwnershipSnapshotEntry{
		rel: rel, info: info, directory: directory,
	})
}

func (walker *buildAuthorityWaitOwnershipTree) revalidateSnapshot() error {
	for _, expected := range walker.snapshot {
		if err := walker.ctx.Err(); err != nil {
			return err
		}
		reopened, actual, err := walker.openRootRelative(expected.rel)
		if err != nil {
			return fmt.Errorf("revalidate final production-source snapshot %s: %w", expected.rel, err)
		}
		closeErr := reopened.Close()
		if closeErr != nil {
			return fmt.Errorf("close final production-source snapshot %s: %w", expected.rel, closeErr)
		}
		matches := buildAuthorityWaitOwnershipSameEntry(expected.info, actual)
		if expected.directory {
			matches = buildAuthorityWaitOwnershipSameDirectory(expected.info, actual)
		}
		if !matches {
			return fmt.Errorf("final production-source snapshot entry %s changed while scanning", expected.rel)
		}
	}
	return nil
}

func (walker *buildAuthorityWaitOwnershipTree) validateGoSourceLimits(
	rel string,
	info fs.FileInfo,
) error {
	if info.Size() < 0 || info.Size() > walker.limits.maxFileBytes {
		return fmt.Errorf("production Go source %s is %d bytes, limit %d",
			rel, info.Size(), walker.limits.maxFileBytes)
	}
	if walker.result.files >= walker.limits.maxFiles {
		return fmt.Errorf("production Go source count exceeds %d", walker.limits.maxFiles)
	}
	if walker.result.bytes < 0 || walker.result.bytes > walker.limits.maxTotalBytes {
		return fmt.Errorf("production Go source byte accounting is invalid")
	}
	if info.Size() > walker.limits.maxTotalBytes-walker.result.bytes {
		return fmt.Errorf("production Go source bytes exceed %d", walker.limits.maxTotalBytes)
	}
	return nil
}

func (walker *buildAuthorityWaitOwnershipTree) prepareGoSourceDescriptor(
	parent *os.File,
	opened *os.File,
	info fs.FileInfo,
	rel string,
	displayPath string,
	name string,
) (*os.File, fs.FileInfo, error) {
	if walker.hooks.beforeSourceOpen != nil {
		if closeErr := opened.Close(); closeErr != nil {
			return nil, nil, closeErr
		}
		if err := walker.hooks.beforeSourceOpen(displayPath, rel); err != nil {
			return nil, nil, fmt.Errorf("before opening production Go source %s: %w", rel, err)
		}
		reopened, reopenedInfo, symlink, err := openBuildAuthorityWaitOwnershipAt(parent, name)
		if err != nil {
			return nil, nil,
				fmt.Errorf("reopen production Go source %s relative to retained directory: %w", rel, err)
		}
		if symlink {
			return nil, nil,
				fmt.Errorf("production Go source %s changed to a symbolic link before opening", rel)
		}
		if !buildAuthorityWaitOwnershipSameSource(info, reopenedInfo) {
			return nil, nil, errors.Join(
				fmt.Errorf("production Go source %s changed identity before opening", rel),
				reopened.Close(),
			)
		}
		return reopened, reopenedInfo, nil
	}
	return opened, info, nil
}

func (walker *buildAuthorityWaitOwnershipTree) validateSymlink(
	parent *os.File,
	parentRel string,
	rel string,
	name string,
) error {
	ownerFile := filepath.ToSlash(filepath.Dir(rel)) == "internal/buildauthority"
	productionGo := strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	productionNative := buildAuthorityWaitOwnershipNativeSource(name)
	if !ownerFile && (productionGo || productionNative) {
		return fmt.Errorf("production source %s is a symbolic link", rel)
	}
	target, err := readBuildAuthorityWaitOwnershipLink(parent, name)
	if err != nil {
		return fmt.Errorf("read repository symbolic link %s: %w", rel, err)
	}
	if pathpkg.IsAbs(target) {
		return fmt.Errorf("repository symbolic link %s has an absolute target", rel)
	}
	resolved := pathpkg.Clean(pathpkg.Join(parentRel, target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("repository symbolic link %s escapes the retained source root", rel)
	}
	targetFile, targetInfo, err := walker.openRootRelative(resolved)
	if err != nil {
		return fmt.Errorf("resolve repository symbolic link %s within retained root: %w", rel, err)
	}
	if targetFile != nil {
		defer func() { _ = targetFile.Close() }()
	}
	if targetInfo.IsDir() {
		return fmt.Errorf("repository directory symlink %s is forbidden by the bounded source walk", rel)
	}
	if !targetInfo.Mode().IsRegular() {
		return fmt.Errorf("repository symbolic link %s does not target a regular file", rel)
	}
	walker.recordSnapshot(resolved, targetInfo, false)
	return nil
}

func (walker *buildAuthorityWaitOwnershipTree) openRootRelative(
	rel string,
) (*os.File, fs.FileInfo, error) {
	if rel == "." {
		info, err := walker.root.Stat()
		return nil, info, err
	}
	current := walker.root
	owned := false
	components := strings.Split(rel, "/")
	for index, component := range components {
		next, nextInfo, symlink, err := openBuildAuthorityWaitOwnershipAt(current, component)
		if owned {
			closeErr := current.Close()
			if err != nil || closeErr != nil {
				if next != nil {
					_ = next.Close()
				}
				return nil, nil, errors.Join(err, closeErr)
			}
		}
		if err != nil {
			return nil, nil, err
		}
		if symlink {
			return nil, nil, fmt.Errorf("symbolic-link target component %q is itself a symbolic link", component)
		}
		if index != len(components)-1 && !nextInfo.IsDir() {
			_ = next.Close()
			return nil, nil, fmt.Errorf("symbolic-link target component %q is not a directory", component)
		}
		current = next
		owned = true
		if index == len(components)-1 {
			return current, nextInfo, nil
		}
	}
	return nil, nil, fs.ErrInvalid
}

func readBuildAuthorityWaitOwnershipDescriptor(
	ctx context.Context,
	file *os.File,
	rel string,
	expected fs.FileInfo,
	maxBytes int64,
	afterOpen func() error,
) ([]byte, error) {
	if afterOpen != nil {
		if err := afterOpen(); err != nil {
			return nil, errors.Join(
				fmt.Errorf("after opening production Go source %s: %w", rel, err),
				file.Close(),
			)
		}
	}
	preReadInfo, statErr := file.Stat()
	if statErr != nil || !buildAuthorityWaitOwnershipSameSource(expected, preReadInfo) {
		return nil, errors.Join(
			fmt.Errorf("production Go source %s changed after descriptor open", rel),
			statErr,
			file.Close(),
		)
	}
	source, readErr := io.ReadAll(io.LimitReader(
		&buildAuthorityContextReader{ctx: ctx, reader: file}, maxBytes+1,
	))
	postReadInfo, statErr := file.Stat()
	closeErr := file.Close()
	if readErr != nil {
		return nil, errors.Join(fmt.Errorf("read production Go source %s: %w", rel, readErr), statErr, closeErr)
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(fmt.Errorf("read production Go source %s: %w", rel, err), statErr, closeErr)
	}
	if statErr != nil || closeErr != nil {
		return nil, errors.Join(statErr, closeErr)
	}
	if int64(len(source)) > maxBytes {
		return nil, fmt.Errorf("production Go source %s exceeds %d bytes while reading", rel, maxBytes)
	}
	if !buildAuthorityWaitOwnershipSameSource(expected, postReadInfo) {
		return nil, fmt.Errorf("production Go source %s changed while reading", rel)
	}
	return source, nil
}

func revalidateBuildAuthorityWaitOwnershipEntry(
	parent *os.File,
	name string,
	expected fs.FileInfo,
	directory bool,
) error {
	reopened, reopenedInfo, symlink, err := openBuildAuthorityWaitOwnershipAt(parent, name)
	if err != nil {
		return err
	}
	if symlink {
		return fmt.Errorf("entry changed to a symbolic link")
	}
	closeErr := reopened.Close()
	if closeErr != nil {
		return closeErr
	}
	if directory {
		if !buildAuthorityWaitOwnershipSameDirectory(expected, reopenedInfo) {
			return fmt.Errorf("directory identity changed")
		}
		return nil
	}
	if !buildAuthorityWaitOwnershipSameSource(expected, reopenedInfo) {
		return fmt.Errorf("source identity changed")
	}
	return nil
}

func buildAuthorityWaitOwnershipSameDirectory(first, second fs.FileInfo) bool {
	return first != nil && second != nil && first.IsDir() && second.IsDir() &&
		buildAuthorityWaitOwnershipSameEntry(first, second)
}

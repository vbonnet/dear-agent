package specpackage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
)

var (
	afterPrivateStagingRootCreation = func(string) {}
	beforeFinalStagedVerification   = func(string) {}
)

//nolint:gocyclo // The ordered fail-closed staging and validation sequence is intentionally explicit.
func stage(ctx context.Context, sourceRoot, artifactPath, stagingParent string) (result StagedPackage, resultErr error) {
	if err := checkContext(ctx); err != nil {
		return StagedPackage{}, err
	}
	sourceRoot, err := cleanAbsolutePath(sourceRoot, "source root")
	if err != nil {
		return StagedPackage{}, err
	}
	artifactPath, err = cleanAbsolutePath(artifactPath, "specaudit artifact")
	if err != nil {
		return StagedPackage{}, err
	}
	stagingParent, err = cleanAbsolutePath(stagingParent, "staging parent")
	if err != nil {
		return StagedPackage{}, err
	}

	source, err := openAnchoredRoot(sourceRoot)
	if err != nil {
		return StagedPackage{}, fmt.Errorf("open source root: %w", err)
	}
	defer source.Close()
	staging, err := openAnchoredRoot(stagingParent)
	if err != nil {
		return StagedPackage{}, fmt.Errorf("open staging parent: %w", err)
	}
	defer staging.Close()
	if err := validateStagingParentOutsideSource(source, staging); err != nil {
		return StagedPackage{}, err
	}
	initialSourceTree, err := validateSourceSkillTree(ctx, source)
	if err != nil {
		return StagedPackage{}, err
	}

	artifact, err := readStandaloneRegular(ctx, artifactPath, maxExecutableBytes)
	if err != nil {
		return StagedPackage{}, fmt.Errorf("read specaudit artifact: %w", err)
	}
	if err := source.verifyVisible(); err != nil {
		return StagedPackage{}, fmt.Errorf("reinspect source root after reading specaudit artifact: %w", err)
	}
	if artifact.mode.Perm()&0o111 == 0 || artifact.mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0 {
		return StagedPackage{}, fmt.Errorf("specaudit artifact must be executable without special mode bits")
	}
	if len(artifact.data) == 0 {
		return StagedPackage{}, fmt.Errorf("specaudit artifact must not be empty")
	}

	// Recheck after all preallocation reads so a staging parent moved beneath the
	// source during those reads is rejected before the first filesystem mutation.
	if err := validateStagingParentOutsideSource(source, staging); err != nil {
		return StagedPackage{}, err
	}
	stagedRoot, stagedIdentity, openedStagedRoot, err := createPrivateStagingRoot(source, staging)
	if err != nil {
		if stagedRoot != "" {
			return StagedPackage{}, errors.Join(err, retainedStagingFailure(stagedRoot, stagedIdentity))
		}
		return StagedPackage{}, err
	}
	complete := false
	defer func() {
		if complete {
			return
		}
		resultErr = errors.Join(resultErr, retainedStagingFailure(stagedRoot, stagedIdentity))
	}()
	afterPrivateStagingRootCreation(stagedRoot)
	stagedFilesystem, err := newStagedFilesystem(openedStagedRoot, &stagedIdentity)
	if err != nil {
		return StagedPackage{}, errors.Join(fmt.Errorf("open private staging root: %w", err), openedStagedRoot.Close())
	}
	defer func() {
		if closeErr := stagedFilesystem.Close(); closeErr != nil {
			result = StagedPackage{}
			complete = false
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()

	for _, directory := range expectedDirectories {
		if directory == "." {
			continue
		}
		if err := stagedFilesystem.Mkdir(directory); err != nil {
			return StagedPackage{}, fmt.Errorf("create staged directory %q: %w", directory, err)
		}
	}

	fileBytes := make(map[string][]byte, len(payloadLayout))
	for _, entry := range payloadLayout {
		if err := checkContext(ctx); err != nil {
			return StagedPackage{}, err
		}
		if entry.packagePath == "bin/specaudit" {
			fileBytes[entry.packagePath] = artifact.data
			continue
		}
		snapshot, err := source.readRegular(ctx, entry.sourcePath, entry.maxBytes)
		if err != nil {
			return StagedPackage{}, fmt.Errorf("read source package file %q: %w", entry.sourcePath, err)
		}
		fileBytes[entry.packagePath] = snapshot.data
	}
	finalSourceTree, err := validateSourceSkillTree(ctx, source)
	if err != nil {
		return StagedPackage{}, fmt.Errorf("revalidate canonical source skill tree: %w", err)
	}
	if err := equalTreeSnapshots(initialSourceTree, finalSourceTree, "canonical source skill"); err != nil {
		return StagedPackage{}, err
	}
	if err := source.verifyVisible(); err != nil {
		return StagedPackage{}, fmt.Errorf("reinspect source root after copying package files: %w", err)
	}

	receipts := make([]FileReceipt, 0, len(payloadLayout))
	for _, entry := range payloadLayout {
		content := fileBytes[entry.packagePath]
		if err := stagedFilesystem.WriteFile(entry.packagePath, content, entry.mode); err != nil {
			return StagedPackage{}, err
		}
		digest := sha256.Sum256(content)
		receipts = append(receipts, FileReceipt{
			Path:        entry.packagePath,
			Role:        entry.role,
			LogicalMode: fmt.Sprintf("%04o", entry.mode),
			Size:        int64(len(content)),
			SHA256:      hex.EncodeToString(digest[:]),
		})
	}
	manifest, expectedReceipt, err := canonicalManifest(receipts)
	if err != nil {
		return StagedPackage{}, err
	}
	if err := stagedFilesystem.WriteFile(manifestPath, manifest, 0o444); err != nil {
		return StagedPackage{}, err
	}
	if err := stagedFilesystem.Sync(); err != nil {
		return StagedPackage{}, err
	}
	if err := stagedFilesystem.Verify(ctx); err != nil {
		return StagedPackage{}, fmt.Errorf("verify staged identities before validation: %w", err)
	}
	validated, err := validate(ctx, stagedRoot)
	if err != nil {
		return StagedPackage{}, fmt.Errorf("validate staged package: %w", err)
	}
	if validated.ManifestSHA256 != expectedReceipt.ManifestSHA256 {
		return StagedPackage{}, fmt.Errorf("staged package receipt changed during validation")
	}
	beforeFinalStagedVerification(stagedRoot)
	if err := stagedFilesystem.Verify(ctx); err != nil {
		return StagedPackage{}, fmt.Errorf("verify staged identities after validation: %w", err)
	}
	complete = true
	return StagedPackage{Root: stagedRoot, Receipt: validated}, nil
}

func retainedStagingFailure(root string, identity stagedRootIdentity) error {
	failure := &RetainedStagingRootError{Root: root}
	if identity.file == (fileIdentity{}) {
		return failure
	}
	same, err := sameStagedRoot(root, identity)
	if err != nil || !same {
		return errors.Join(
			failure,
			fmt.Errorf("allocated staging path no longer exposes the original identity; no cleanup was attempted"),
			err,
		)
	}
	failure.IdentityVerified = true
	return failure
}

func validateSourceSkillTree(ctx context.Context, source *anchoredRoot) ([]treeEntry, error) {
	tree, err := source.readTree(ctx, "spec-governance/skills")
	if err != nil {
		return nil, fmt.Errorf("inspect canonical source skill tree: %w", err)
	}
	directories := make([]string, 0)
	files := make([]string, 0)
	for _, entry := range tree {
		if entry.directory {
			directories = append(directories, entry.path)
		} else {
			files = append(files, entry.path)
		}
	}
	sort.Strings(directories)
	sort.Strings(files)
	expectedDirectories, expectedFiles := expectedSourceSkillPaths()
	if err := equalSortedPaths(directories, expectedDirectories, "source skill directory"); err != nil {
		return nil, err
	}
	if err := equalSortedPaths(files, expectedFiles, "source skill file"); err != nil {
		return nil, err
	}
	return tree, nil
}

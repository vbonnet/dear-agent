package specpackage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
)

const privateDirectoryMode fs.FileMode = 0o700

func validate(ctx context.Context, distributionRoot string) (Receipt, error) {
	if err := checkContext(ctx); err != nil {
		return Receipt{}, err
	}
	distributionRoot, err := cleanAbsolutePath(distributionRoot, "distribution root")
	if err != nil {
		return Receipt{}, err
	}
	root, err := openAnchoredRoot(distributionRoot)
	if err != nil {
		return Receipt{}, err
	}
	defer root.Close()

	tree, err := root.readTree(ctx, ".")
	if err != nil {
		return Receipt{}, fmt.Errorf("inspect package tree: %w", err)
	}
	if err := validateExactPackageTree(tree); err != nil {
		return Receipt{}, err
	}

	files, markdown, err := readPayloadReceipts(ctx, root)
	if err != nil {
		return Receipt{}, err
	}
	if err := validateMarkdownClosure(markdown); err != nil {
		return Receipt{}, err
	}
	receipt, err := readValidatedManifest(ctx, root, files)
	if err != nil {
		return Receipt{}, err
	}
	if err := revalidatePackageTree(ctx, root, tree); err != nil {
		return Receipt{}, err
	}
	if err := root.verifyVisible(); err != nil {
		return Receipt{}, fmt.Errorf("reinspect distribution root after validation: %w", err)
	}
	return receipt, nil
}

func readPayloadReceipts(ctx context.Context, root *anchoredRoot) ([]FileReceipt, map[string][]byte, error) {
	files := make([]FileReceipt, 0, len(payloadLayout))
	markdown := make(map[string][]byte, len(payloadLayout)-1)
	var totalBytes int64
	for _, entry := range payloadLayout {
		if err := checkContext(ctx); err != nil {
			return nil, nil, err
		}
		snapshot, err := root.readRegular(ctx, entry.packagePath, entry.maxBytes)
		if err != nil {
			return nil, nil, err
		}
		if snapshot.mode != entry.mode {
			return nil, nil, fmt.Errorf("package file %q mode is %04o, want %04o", entry.packagePath, snapshot.mode, entry.mode)
		}
		totalBytes += int64(len(snapshot.data))
		if totalBytes > maxPackageBytes {
			return nil, nil, fmt.Errorf("package payload exceeds the %d-byte aggregate bound", maxPackageBytes)
		}
		digest := sha256.Sum256(snapshot.data)
		files = append(files, FileReceipt{
			Path:        entry.packagePath,
			Role:        entry.role,
			LogicalMode: fmt.Sprintf("%04o", entry.mode),
			Size:        int64(len(snapshot.data)),
			SHA256:      hex.EncodeToString(digest[:]),
		})
		if entry.role == "skill" || entry.role == "reference" {
			markdown[entry.packagePath] = snapshot.data
		}
	}
	return files, markdown, nil
}

func readValidatedManifest(ctx context.Context, root *anchoredRoot, files []FileReceipt) (Receipt, error) {
	expectedManifest, receipt, err := canonicalManifest(files)
	if err != nil {
		return Receipt{}, err
	}
	actual, err := root.readRegular(ctx, manifestPath, maxManifestBytes)
	if err != nil {
		return Receipt{}, err
	}
	if actual.mode != 0o444 {
		return Receipt{}, fmt.Errorf("package file %q mode is %04o, want %s", manifestPath, actual.mode, manifestLogicalMode)
	}
	if err := compareCanonicalManifest(actual.data, expectedManifest); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func revalidatePackageTree(ctx context.Context, root *anchoredRoot, initial []treeEntry) error {
	finalTree, err := root.readTree(ctx, ".")
	if err != nil {
		return fmt.Errorf("reinspect package tree: %w", err)
	}
	if err := validateExactPackageTree(finalTree); err != nil {
		return fmt.Errorf("revalidate package tree: %w", err)
	}
	return equalTreeSnapshots(initial, finalTree, "package")
}

func validateExactPackageTree(tree []treeEntry) error {
	directories := make([]string, 0, len(expectedDirectories))
	files := make([]string, 0, len(payloadLayout)+1)
	for _, entry := range tree {
		if entry.directory {
			if entry.mode != privateDirectoryMode {
				return fmt.Errorf("package directory %q mode is %04o, want %04o", entry.path, entry.mode, privateDirectoryMode)
			}
			directories = append(directories, entry.path)
			continue
		}
		files = append(files, entry.path)
	}
	sort.Strings(directories)
	sort.Strings(files)
	expectedDirs, expectedFiles := expectedPackagePaths()
	if err := equalSortedPaths(directories, expectedDirs, "directory"); err != nil {
		return err
	}
	if err := equalSortedPaths(files, expectedFiles, "file"); err != nil {
		return err
	}
	return nil
}

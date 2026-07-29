package codexhooks

import (
	"errors"
	"fmt"
	"os"
	slashpath "path"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultStoreBase is the host-side root for hook materializations. AGM never
// adds this path to a sandbox's writable roots.
func DefaultStoreBase() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home for trusted hook store: %w", err)
	}
	if home == "" {
		return "", errors.New("resolve home for trusted hook store: home directory is empty")
	}
	return filepath.Join(home, ".local", "share", "dear-agent", "trusted-codex-hooks"), nil
}

func materializeAssets(storeBase, digest string, assets []asset, writableRoots []string) (string, error) {
	base, err := prepareStoreBase(storeBase)
	if err != nil {
		return "", err
	}
	root := filepath.Join(base, digest)
	if err := rejectWritableOverlap("trusted hook root", root, writableRoots); err != nil {
		return "", err
	}
	if _, err = os.Lstat(root); err == nil {
		if err := verifyMaterializedAssets(root, digest, assets); err != nil {
			return "", err
		}
		return root, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect existing hook materialization: %w", err)
	}

	stage, err := stageMaterializedAssets(base, digest, assets)
	if err != nil {
		return "", err
	}
	removeStage := true
	defer func() {
		if removeStage {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := os.Rename(stage, root); err != nil {
		return reuseConcurrentMaterialization(root, digest, assets, err)
	}
	removeStage = false
	if err := lockMaterializedDirectories(root); err != nil {
		return "", err
	}
	if err := verifyMaterializedAssets(root, digest, assets); err != nil {
		return "", err
	}
	return root, nil
}

func prepareStoreBase(storeBase string) (string, error) {
	base, err := cleanAbsolute(storeBase)
	if err != nil {
		return "", fmt.Errorf("resolve hook store: %w", err)
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", fmt.Errorf("create hook store: %w", err)
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return "", fmt.Errorf("resolve hook store identity: %w", err)
	}
	return base, nil
}

func stageMaterializedAssets(base, digest string, assets []asset) (string, error) {
	stage, err := os.MkdirTemp(base, "."+digest+".staging-*")
	if err != nil {
		return "", fmt.Errorf("stage hook materialization: %w", err)
	}
	for _, item := range assets {
		target := filepath.Join(stage, filepath.FromSlash(item.path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			_ = os.RemoveAll(stage)
			return "", fmt.Errorf("create materialized hook parent: %w", err)
		}
		mode := os.FileMode(0o400)
		if item.executable {
			mode = 0o500
		}
		if err := writeMaterializedFile(target, item.content, mode); err != nil {
			_ = os.RemoveAll(stage)
			return "", fmt.Errorf("materialize hook asset %q: %w", item.path, err)
		}
	}
	return stage, nil
}

func reuseConcurrentMaterialization(root, digest string, assets []asset, publishErr error) (string, error) {
	if _, statErr := os.Lstat(root); statErr == nil {
		if verifyErr := verifyMaterializedAssets(root, digest, assets); verifyErr == nil {
			return root, nil
		}
	}
	return "", fmt.Errorf("publish hook materialization: %w", publishErr)
}

func writeMaterializedFile(path string, content []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		closeErr := file.Close()
		return errors.Join(err, closeErr)
	}
	if err := file.Sync(); err != nil {
		closeErr := file.Close()
		return errors.Join(err, closeErr)
	}
	if err := file.Chmod(mode); err != nil {
		closeErr := file.Close()
		return errors.Join(err, closeErr)
	}
	return file.Close()
}

func lockMaterializedDirectories(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("inventory materialized hook directories: %w", err)
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		// Directory traversal requires execute permission; all write bits remain
		// absent by construction.
		if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // Read-only directories require execute permission for traversal.
			return fmt.Errorf("lock materialized hook directory %s: %w", dir, err)
		}
	}
	return nil
}

func verifyMaterializedAssets(root, digest string, expected []asset) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect materialized hook root: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o222 != 0 {
		return fmt.Errorf("materialized hook root %q is not a read-only directory", root)
	}
	seen, err := inventoryMaterializedAssets(root, expected)
	if err != nil {
		return err
	}
	var actual []asset
	for _, item := range expected {
		if !seen[item.path] {
			return fmt.Errorf("materialized hook asset %q is missing", item.path)
		}
		verified, err := verifyMaterializedAsset(root, item)
		if err != nil {
			return err
		}
		actual = append(actual, verified)
	}
	if got := digestAssets(actual); got != digest {
		return fmt.Errorf("materialized hook digest differs: got %s, want %s", got, digest)
	}
	return nil
}

func inventoryMaterializedAssets(root string, expected []asset) (map[string]bool, error) {
	expectedByPath := make(map[string]asset, len(expected))
	expectedDirs := make(map[string]bool)
	for _, item := range expected {
		expectedByPath[item.path] = item
		for parent := slashpath.Dir(item.path); parent != "."; parent = slashpath.Dir(parent) {
			expectedDirs[parent] = true
		}
	}
	seen := make(map[string]bool, len(expected))
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if !expectedDirs[relative] {
				return fmt.Errorf("materialized hook root contains unexpected directory %q", relative)
			}
			if info.Mode().Perm()&0o222 != 0 {
				return fmt.Errorf("materialized hook directory %q is writable", relative)
			}
			return nil
		}
		if _, ok := expectedByPath[relative]; !ok {
			return fmt.Errorf("materialized hook root contains unexpected asset %q", relative)
		}
		seen[relative] = true
		return nil
	}); err != nil {
		return nil, fmt.Errorf("inventory materialized hook root: %w", err)
	}
	return seen, nil
}

func verifyMaterializedAsset(root string, item asset) (asset, error) {
	path := filepath.Join(root, filepath.FromSlash(item.path))
	info, err := os.Lstat(path)
	if err != nil {
		return asset{}, fmt.Errorf("inspect materialized hook asset %q: %w", item.path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
		return asset{}, fmt.Errorf("materialized hook asset %q is not a read-only regular file", item.path)
	}
	if executable := info.Mode().Perm()&0o111 != 0; executable != item.executable {
		return asset{}, fmt.Errorf("materialized hook asset %q executable mode differs", item.path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return asset{}, fmt.Errorf("read materialized hook asset %q: %w", item.path, err)
	}
	return asset{
		path: item.path, gitMode: item.gitMode, content: content, executable: item.executable,
	}, nil
}

func rejectWritableOverlap(label, root string, writableRoots []string) error {
	root, err := resolvePathIdentity(root)
	if err != nil {
		return err
	}
	for _, candidate := range writableRoots {
		if candidate == "" {
			continue
		}
		candidate, err = resolvePathIdentity(candidate)
		if err != nil {
			return fmt.Errorf("resolve writable root %q: %w", candidate, err)
		}
		if pathWithin(root, candidate) || pathWithin(candidate, root) {
			return fmt.Errorf("%s %q overlaps agent-writable root %q", label, root, candidate)
		}
	}
	return nil
}

func resolvePathIdentity(path string) (string, error) {
	absolute, err := cleanAbsolute(path)
	if err != nil {
		return "", err
	}
	current := absolute
	var suffix []string
	for {
		resolved, evalErr := filepath.EvalSymlinks(current)
		if evalErr == nil {
			for _, component := range suffix {
				resolved = filepath.Join(resolved, component)
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(evalErr, os.ErrNotExist) {
			return "", evalErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absolute, nil
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
}

func cleanAbsolute(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

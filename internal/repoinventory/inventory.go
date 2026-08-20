// Package repoinventory provides one deterministic file-discovery seam for
// repository governance tools.
package repoinventory

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const gitInventoryTimeout = 10 * time.Second

// ExecutableBDDFeatureRoot is the only repository path passed to the Godog
// feature runner. Repository policy keeps executable features as direct
// children so every catalog and coverage surface observes the same suite.
const ExecutableBDDFeatureRoot = "agm/test/bdd/features"

var excludedDirectoryNames = map[string]bool{
	".git":         true,
	".worktrees":   true,
	"vendor":       true,
	"node_modules": true,
	"bin":          true,
	"build":        true,
	"dist":         true,
	"testdata":     true,
}

// File is one regular repository entry. Path is slash-normalized relative to
// the requested scan root; Absolute is suitable for file reads.
type File struct {
	Path     string
	Absolute string
	Mode     fs.FileMode
}

// Name returns the final path component using repository slash semantics.
func (f File) Name() string {
	return filepath.Base(filepath.FromSlash(f.Path))
}

// Executable reports whether any executable mode bit is set.
func (f File) Executable() bool {
	return f.Mode.Perm()&0o111 != 0
}

// IsGovernedPath reports whether a repository-relative path belongs to the
// shared governance inventory rather than VCS, dependency, generated-output,
// test-fixture, or nested-worktree storage.
func IsGovernedPath(path string) bool {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	return path != "." && filepath.IsLocal(filepath.FromSlash(path)) && !excludedPath(path)
}

// IsExecutableBDDFeaturePath reports whether a canonical repository-relative
// path belongs to the feature tree the real BDD runner executes.
func IsExecutableBDDFeaturePath(filePath string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(filePath)))
	prefix := ExecutableBDDFeatureRoot + "/"
	name := strings.TrimPrefix(filePath, prefix)
	return cleaned == filePath && IsGovernedPath(filePath) && name != filePath &&
		pathpkg.Base(name) == name && canonicalBDDFeatureName(name)
}

func canonicalBDDFeatureName(name string) bool {
	stem := strings.TrimSuffix(name, ".feature")
	if stem == name || stem == "" {
		return false
	}
	for _, char := range stem {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

// IsImplementationSource classifies source paths consistently for mutable
// repository scans and immutable Git snapshots. Only regular files qualify;
// executable is relevant only to extensionless hook or tool files.
func IsImplementationSource(path string, regular, executable bool) bool {
	if !regular || !IsGovernedPath(path) {
		return false
	}
	name := filepath.Base(filepath.FromSlash(path))
	switch strings.ToLower(name) {
	case "dockerfile", "makefile":
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".rs", ".py",
		".sh", ".bash", ".zsh", ".bats", ".tf", ".sql", ".yaml", ".yml",
		".json", ".toml", ".plist", ".service", ".dockerfile":
		return true
	}
	return filepath.Ext(name) == "" && executable
}

// Paths projects a file inventory to its stable repository-relative paths.
func Paths(files []File) []string {
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.Path
	}
	return paths
}

// ExcludedDirectoryNames returns the canonical directory-name exclusions used
// by both Git-backed and filesystem-fallback scans.
func ExcludedDirectoryNames() []string {
	names := make([]string, 0, len(excludedDirectoryNames))
	for name := range excludedDirectoryNames {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Scan returns every governed file beneath root. Git worktrees use ls-files so
// repository ignore rules are authoritative. Non-Git roots, including unit
// test fixtures, use a filesystem walk with the same named-directory policy.
func Scan(root string) ([]File, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve inventory root %q: %w", root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat inventory root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("inventory root %q is not a directory", root)
	}

	paths, err := repositoryFiles(absRoot)
	if err != nil {
		return nil, err
	}

	files := make([]File, 0, len(paths))
	for _, path := range paths {
		path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
		if !IsGovernedPath(path) {
			continue
		}
		absolute := filepath.Join(absRoot, filepath.FromSlash(path))
		fileInfo, err := os.Lstat(absolute)
		if os.IsNotExist(err) {
			// A tracked file deleted from the worktree can remain in the index.
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("stat inventory file %q: %w", path, err)
		}
		if fileInfo.IsDir() {
			continue
		}
		files = append(files, File{Path: path, Absolute: absolute, Mode: fileInfo.Mode()})
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

func repositoryFiles(root string) ([]string, error) {
	marker := filepath.Join(root, ".git")
	markerInfo, markerErr := os.Lstat(marker)
	switch {
	case markerErr == nil:
		if markerInfo.IsDir() {
			_, headErr := os.Lstat(filepath.Join(marker, "HEAD"))
			if os.IsNotExist(headErr) {
				return filesystemFiles(root)
			}
			if headErr != nil {
				return nil, fmt.Errorf("stat Git HEAD marker: %w", headErr)
			}
		}
		paths, err := gitFiles(root)
		if err != nil {
			return nil, fmt.Errorf("list Git repository files: %w", err)
		}
		return paths, nil
	case !os.IsNotExist(markerErr):
		return nil, fmt.Errorf("stat Git worktree marker: %w", markerErr)
	default:
		return filesystemFiles(root)
	}
}

func gitFiles(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitInventoryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
		return nil, err
	}

	parts := bytes.Split(out, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		paths = append(paths, string(part))
	}
	return paths, nil
}

func filesystemFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, collectFilesystemPath(root, &paths))
	if err != nil {
		return nil, fmt.Errorf("walk inventory root %q: %w", root, err)
	}
	return paths, nil
}

func collectFilesystemPath(root string, paths *[]string) fs.WalkDirFunc {
	return func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // non-Git fallback skips unreadable entries and continues
		}
		if entry.IsDir() {
			if path != root && excludedDirectoryNames[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		*paths = append(*paths, filepath.ToSlash(rel))
		return nil
	}
}

func excludedPath(path string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if excludedDirectoryNames[part] {
			return true
		}
	}
	return false
}

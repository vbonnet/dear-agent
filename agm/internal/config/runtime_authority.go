package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrRuntimeAuthorityUnavailable reports that a Config was not produced by a
// successful Load or that an authority projection is the invalid zero value.
var ErrRuntimeAuthorityUnavailable = errors.New("runtime authority unavailable")

// AuthorityResolutionError identifies which retained root path could not be
// normalized while preserving the underlying filesystem error.
type AuthorityResolutionError struct {
	Root string
	Err  error
}

func (e *AuthorityResolutionError) Error() string {
	return fmt.Sprintf("resolve runtime %s authority: %v", e.Root, e.Err)
}

func (e *AuthorityResolutionError) Unwrap() error { return e.Err }

// RuntimeAuthority is the opaque, structurally immutable physical root-path
// tuple selected by one successful configuration load. Projecting a path
// revalidates its current spelling and can fail after capture; this value is
// neither an open filesystem handle nor a serialized filesystem identity. Its
// zero value is deliberately unusable.
type RuntimeAuthority struct {
	home        HomeRoot
	storage     StorageRoot
	sandboxes   SandboxRoot
	centralized bool
}

// HomeRoot is the retained physical HOME path spelling.
type HomeRoot struct{ path string }

// StorageRoot is the retained physical AGM storage root.
type StorageRoot struct{ path string }

// SandboxRoot is the retained physical parent of all sandbox workspaces.
type SandboxRoot struct{ path string }

// RuntimeAuthority returns the root tuple captured by Load. Config values from
// Default or direct construction intentionally have no runtime authority.
func (c *Config) RuntimeAuthority() (RuntimeAuthority, error) {
	if c == nil || !c.runtimeAuthority.valid() {
		return RuntimeAuthority{}, ErrRuntimeAuthorityUnavailable
	}
	return c.runtimeAuthority, nil
}

// RebindRuntimeAuthorityToIsolatedHome recaptures the runtime authority of an
// already loaded snapshot below an isolated HOME. It exists for the single
// supported case where HOME legitimately moves after Load: activation of a
// per-run test environment. Configuration is loaded before that activation, so
// without this the isolated run keeps provisioning against the host roots.
//
// Isolation always projects the dotfile layout below the isolated HOME. A test
// run must never be routed back into centralized production storage by the
// shared configuration it inherited, which is exactly what recapturing the
// configured storage mode under a fresh HOME would do.
//
// It refuses a Config that never held authority, so a failed or bypassed Load
// can never be laundered into a usable snapshot through this entry point.
func (c *Config) RebindRuntimeAuthorityToIsolatedHome(homeDir string) error {
	if _, err := c.RuntimeAuthority(); err != nil {
		return err
	}
	isolated := *c
	isolated.Storage.Mode = "dotfile"
	authority, err := captureRuntimeAuthority(&isolated, homeDir)
	if err != nil {
		return err
	}
	c.runtimeAuthority = authority
	return nil
}

// Home returns the retained physical HOME root.
func (a RuntimeAuthority) Home() (HomeRoot, error) {
	if !a.valid() {
		return HomeRoot{}, ErrRuntimeAuthorityUnavailable
	}
	return a.home, nil
}

// Storage returns the retained physical AGM storage root.
func (a RuntimeAuthority) Storage() (StorageRoot, error) {
	if !a.valid() {
		return StorageRoot{}, ErrRuntimeAuthorityUnavailable
	}
	return a.storage, nil
}

// Sandboxes returns the retained physical sandbox parent.
func (a RuntimeAuthority) Sandboxes() (SandboxRoot, error) {
	if !a.valid() {
		return SandboxRoot{}, ErrRuntimeAuthorityUnavailable
	}
	return a.sandboxes, nil
}

// Path returns the retained physical HOME path.
func (r HomeRoot) Path() (string, error) { return retainedRootPath(r.path) }

// Path returns the retained physical storage path.
func (r StorageRoot) Path() (string, error) { return retainedRootPath(r.path) }

// Path returns the retained physical sandbox-parent path.
func (r SandboxRoot) Path() (string, error) { return retainedRootPath(r.path) }

// Workspace derives exactly one sandbox workspace below the retained root.
func (r SandboxRoot) Workspace(sessionID string) (string, error) {
	root, err := r.Path()
	if err != nil {
		return "", err
	}
	if !singlePathComponent(sessionID) {
		return "", fmt.Errorf("sandbox session ID %q must be one non-empty path component", sessionID)
	}
	candidate := filepath.Join(root, sessionID)
	resolved, err := resolvePhysicalDirectory(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox workspace %q: %w", candidate, err)
	}
	if resolved != candidate {
		return "", fmt.Errorf("sandbox workspace %q resolves outside its retained path as %q", candidate, resolved)
	}
	if err := requireContainedPath(root, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func (a RuntimeAuthority) valid() bool {
	return validRetainedRoot(a.home.path) &&
		validRetainedRoot(a.storage.path) &&
		validRetainedRoot(a.sandboxes.path)
}

func retainedRootPath(path string) (string, error) {
	if !validRetainedRoot(path) {
		return "", ErrRuntimeAuthorityUnavailable
	}
	resolved, err := resolvePhysicalDirectory(path)
	if err != nil {
		return "", fmt.Errorf("revalidate retained root %q: %w", path, err)
	}
	if resolved != path {
		return "", fmt.Errorf("retained root %q now resolves as %q", path, resolved)
	}
	return path, nil
}

func validRetainedRoot(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path
}

func singlePathComponent(value string) bool {
	if value == "" || value == "." || value == ".." || filepath.Clean(value) != value {
		return false
	}
	return !strings.ContainsAny(value, `/\`)
}

func captureRuntimeAuthority(cfg *Config, homeDir string) (RuntimeAuthority, error) {
	physicalHome, err := resolvePhysicalDirectory(homeDir)
	if err != nil {
		return RuntimeAuthority{}, authorityResolutionError("home", err)
	}

	physicalStorage, err := resolveStorageAuthority(cfg, physicalHome)
	if err != nil {
		return RuntimeAuthority{}, authorityResolutionError("storage", err)
	}

	sandboxPath := filepath.Join(physicalStorage, "sandboxes")
	physicalSandboxes, err := resolvePhysicalDirectory(sandboxPath)
	if err != nil {
		return RuntimeAuthority{}, authorityResolutionError("sandboxes", err)
	}
	if err := requireContainedPath(physicalStorage, physicalSandboxes); err != nil {
		return RuntimeAuthority{}, authorityResolutionError("sandboxes", err)
	}

	return RuntimeAuthority{
		home:        HomeRoot{path: physicalHome},
		storage:     StorageRoot{path: physicalStorage},
		sandboxes:   SandboxRoot{path: physicalSandboxes},
		centralized: cfg.Storage.Mode == "centralized",
	}, nil
}

func resolveStorageAuthority(cfg *Config, physicalHome string) (string, error) {
	switch cfg.Storage.Mode {
	case "", "dotfile":
		storagePath := filepath.Join(physicalHome, ".agm")
		return resolvePhysicalDirectory(storagePath)
	case "centralized":
		workspaceSetting := expandHomeAt(cfg.Storage.Workspace, physicalHome)
		workspacePath, err := detectWorkspaceAt(workspaceSetting, physicalHome)
		if err != nil {
			return "", fmt.Errorf("detect centralized workspace: %w", err)
		}
		physicalWorkspace, err := resolveExistingPhysicalDirectory(workspacePath)
		if err != nil {
			return "", fmt.Errorf("resolve centralized workspace: %w", err)
		}
		relativePath := cfg.Storage.RelativePath
		if relativePath == "" {
			relativePath = ".agm"
		}
		if err := validateStorageRelativePath(relativePath); err != nil {
			return "", err
		}
		storagePath := filepath.Join(physicalWorkspace, relativePath)
		physicalStorage, err := resolvePhysicalDirectory(storagePath)
		if err != nil {
			return "", err
		}
		if err := requireContainedPath(physicalWorkspace, physicalStorage); err != nil {
			return "", err
		}
		return physicalStorage, nil
	default:
		return "", fmt.Errorf("invalid storage mode %q (must be dotfile or centralized)", cfg.Storage.Mode)
	}
}

func validateStorageRelativePath(path string) error {
	if path == "" {
		return errors.New("storage.relative_path must not be empty")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("storage.relative_path %q must be relative", path)
	}
	if filepath.Clean(path) != path {
		return fmt.Errorf("storage.relative_path %q must be clean", path)
	}
	for component := range strings.FieldsFuncSeq(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if component == "." || component == ".." {
			return fmt.Errorf("storage.relative_path %q must not contain . or .. components", path)
		}
	}
	return nil
}

// resolvePhysicalDirectory resolves every existing prefix and preserves an
// absent suffix below the nearest existing physical directory.
func resolvePhysicalDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path %q is not absolute", path)
	}
	cleanPath := filepath.Clean(path)
	if cleanPath != path {
		return "", fmt.Errorf("path %q is not clean", path)
	}

	prefix := cleanPath
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(prefix)
		if err == nil {
			info, statErr := os.Stat(resolved)
			if statErr != nil {
				return "", statErr
			}
			if !info.IsDir() {
				return "", fmt.Errorf("existing prefix %q is not a directory", resolved)
			}
			for _, component := range slices.Backward(suffix) {
				resolved = filepath.Join(resolved, component)
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		info, lstatErr := os.Lstat(prefix)
		if lstatErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("path component %q is a dangling symlink", prefix)
			}
			return "", err
		}
		if !errors.Is(lstatErr, fs.ErrNotExist) {
			return "", lstatErr
		}
		parent := filepath.Dir(prefix)
		if parent == prefix {
			return "", err
		}
		suffix = append(suffix, filepath.Base(prefix))
		prefix = parent
	}
}

func resolveExistingPhysicalDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = absolute
	}
	cleanPath := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", resolved)
	}
	return filepath.Clean(resolved), nil
}

func requireContainedPath(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q is not a descendant of %q", candidate, root)
	}
	return nil
}

func authorityResolutionError(root string, err error) error {
	return &AuthorityResolutionError{Root: root, Err: err}
}

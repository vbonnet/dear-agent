package plugin

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManifestFileName is the filename FilesystemLoader looks for inside
// per-plugin subdirectories. Single-file mode (a YAML file directly in
// the discovery dir) accepts any *.yaml or *.yml file.
const ManifestFileName = "plugin.yaml"

// FilesystemLoader discovers plugin manifests on disk. It does not
// register plugins — registration is the host binary's job (compiled-in
// plugins → registry.Register; manifest-discovered plugins → match by
// name against already-registered code). LoadFromDir returns the
// manifests so the host can decide which to enable, in what order,
// with what overrides.
//
// Two layouts are supported in a discovery directory:
//
//   - Single-file: <dir>/<plugin-name>.yaml (or .yml). One plugin
//     per file. The filename is convenience only; the plugin's
//     identity comes from Manifest.Name inside the file.
//   - Per-plugin subdirectory: <dir>/<plugin-name>/plugin.yaml.
//     The subdirectory name is convenience only; same as above.
//
// Both layouts may coexist in the same directory. Hidden entries
// (starting with "." or "_") are skipped — this lets users keep
// disabled-plugin manifests around as `.disabled` siblings.
//
// LoadFromDir continues on per-file errors: a malformed manifest in
// one file does not prevent siblings from loading. The caller gets
// the successful manifests *and* the per-file errors so the host can
// surface them all in one pass.
type FilesystemLoader struct{}

// NewFilesystemLoader returns a FilesystemLoader. The struct is
// stateless today; the constructor exists so future configuration
// (e.g. a logger, a max-depth knob) can be added without breaking
// callers.
func NewFilesystemLoader() *FilesystemLoader {
	return &FilesystemLoader{}
}

// LoadFromDir walks dir and returns every plugin manifest found.
// Errors from individual manifests are returned in errs so the caller
// can surface all of them at once; the manifests slice contains only
// the ones that successfully loaded and validated.
//
// If dir does not exist, LoadFromDir returns (nil, nil) — a missing
// discovery directory is a normal "no plugins configured here" state,
// not an error. If dir exists but is not a directory, LoadFromDir
// returns ([], [error]).
//
// LoadFromDir does not deduplicate by Manifest.Name across files; the
// caller (typically Registry.Register) is the authority on uniqueness.
// This keeps the loader's contract simple: it returns what it found.
func (l *FilesystemLoader) LoadFromDir(dir string) ([]Manifest, []error) {
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{fmt.Errorf("plugin: stat %s: %w", dir, err)}
	}
	if !info.IsDir() {
		return nil, []error{fmt.Errorf("plugin: %s is not a directory", dir)}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{fmt.Errorf("plugin: read dir %s: %w", dir, err)}
	}

	// Sort by name for stable ordering. The registry separately enforces
	// registration order for hook fan-out, but listing-order determinism
	// makes errors and tests easier to read.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var manifests []Manifest
	var errs []error
	for _, entry := range entries {
		name := entry.Name()
		if isHidden(name) {
			continue
		}
		full := filepath.Join(dir, name)
		switch {
		case entry.IsDir():
			path := filepath.Join(full, ManifestFileName)
			if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
				// Subdirectory without a manifest: not a plugin, skip silently.
				// This matches the convention that a discovery directory may
				// contain operator-owned scaffolding (notes, fixtures, etc.).
				continue
			} else if statErr != nil {
				errs = append(errs, fmt.Errorf("plugin: stat %s: %w", path, statErr))
				continue
			}
			m, loadErr := LoadManifest(path)
			if loadErr != nil {
				errs = append(errs, loadErr)
				continue
			}
			manifests = append(manifests, m)
		case isYAMLFile(name):
			m, loadErr := LoadManifest(full)
			if loadErr != nil {
				errs = append(errs, loadErr)
				continue
			}
			manifests = append(manifests, m)
		}
		// Other entries (non-hidden non-yaml files) are ignored on purpose:
		// a discovery directory often holds operator-owned READMEs, etc.
	}
	return manifests, errs
}

// isHidden reports whether name should be skipped by the loader. Hidden
// is "starts with . or _" — the latter so `_disabled-plugin.yaml` works
// as a documented "park this plugin off" pattern without renaming the
// file's extension.
func isHidden(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// isYAMLFile reports whether name is a top-level YAML manifest the
// loader should consider in single-file mode.
func isYAMLFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml":
		return true
	}
	return false
}

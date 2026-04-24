package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

// Loader handles plugin discovery and loading
type Loader struct {
	searchPaths []string
	disabled    []string
	logger      *Logger
}

// NewLoader creates a new plugin loader
func NewLoader(searchPaths []string, disabled []string) *Loader {
	return &Loader{
		searchPaths: searchPaths,
		disabled:    disabled,
		logger:      NewDefaultLogger(),
	}
}

// NewLoaderWithLogger creates a new plugin loader with a custom logger
func NewLoaderWithLogger(searchPaths []string, disabled []string, logger *Logger) *Loader {
	return &Loader{
		searchPaths: searchPaths,
		disabled:    disabled,
		logger:      logger,
	}
}

// Load discovers and loads all plugins from search paths.
// Plugins are discovered in parallel across search paths for improved performance.
// Results are sorted by plugin name to ensure deterministic ordering.
func (l *Loader) Load() ([]*Plugin, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var plugins []*Plugin
	ctx := context.Background()

	l.logger.Info(ctx, "Starting plugin discovery", WithOperation("load").WithExtra("search_paths", l.searchPaths))

	// Launch goroutine for each search path (parallel discovery)
	for _, searchPath := range l.searchPaths {
		wg.Add(1)
		searchPath := searchPath // capture for closure

		go func() {
			defer wg.Done()

			// Load plugins from this path
			discovered, err := l.loadFromPath(searchPath)
			if err != nil {
				// Log error but continue with other paths (partial results OK)
				errCtx := WithSearchPath(searchPath).WithOperation("load_from_path")
				l.logger.Warn(ctx, "Failed to load plugins from search path", errCtx, err)
				return
			}

			l.logger.Debug(ctx, "Discovered plugins from search path",
				WithSearchPath(searchPath).WithExtra("count", len(discovered)))

			// Append to shared results (critical section)
			mu.Lock()
			plugins = append(plugins, discovered...)
			mu.Unlock()
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Sort for deterministic ordering
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Manifest.Name < plugins[j].Manifest.Name
	})

	l.logger.Info(ctx, "Plugin discovery complete",
		WithOperation("load").WithExtra("total_plugins", len(plugins)))

	return plugins, nil
}

// loadFromPath loads all plugins from a single search path.
// This method is called concurrently from Load() for each search path.
func (l *Loader) loadFromPath(searchPath string) ([]*Plugin, error) {
	ctx := context.Background()

	// Expand ~ to home directory
	expandedPath, err := expandPath(searchPath)
	if err != nil {
		errCtx := WithSearchPath(searchPath).WithOperation("expand_path")
		l.logger.Error(ctx, "Failed to expand search path", errCtx, err)
		return nil, err
	}

	// Skip if path doesn't exist
	if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
		errCtx := WithSearchPath(expandedPath).WithOperation("stat")
		l.logger.Debug(ctx, "Search path does not exist", errCtx)
		return nil, err
	}

	// Find all plugin directories
	entries, err := os.ReadDir(expandedPath)
	if err != nil {
		errCtx := WithSearchPath(expandedPath).WithOperation("read_dir")
		l.logger.Error(ctx, "Failed to read search path directory", errCtx, err)
		return nil, err
	}

	var plugins []*Plugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginPath := filepath.Join(expandedPath, entry.Name())
		plugin, err := l.loadPlugin(pluginPath)
		if err != nil {
			// Skip invalid plugins but log the error
			errCtx := WithPath(pluginPath).WithOperation("load_plugin")
			l.logger.Warn(ctx, "Failed to load plugin", errCtx, err)
			continue
		}

		// Check if disabled
		if l.isDisabled(plugin.Manifest.Name) {
			l.logger.Info(ctx, "Skipping disabled plugin", WithPlugin(plugin.Manifest.Name))
			continue
		}

		l.logger.Debug(ctx, "Loaded plugin successfully",
			WithPlugin(plugin.Manifest.Name).WithPath(pluginPath))
		plugins = append(plugins, plugin)
	}

	return plugins, nil
}

// loadPlugin loads a single plugin from a directory
func (l *Loader) loadPlugin(path string) (*Plugin, error) {
	ctx := context.Background()
	manifestPath := filepath.Join(path, "plugin.yaml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		errCtx := WithPath(path).WithOperation("read_manifest").WithExtra("manifest_path", manifestPath)
		l.logger.Debug(ctx, "Failed to read plugin manifest", errCtx)
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		errCtx := WithPath(path).WithOperation("parse_manifest").WithExtra("manifest_path", manifestPath)
		l.logger.Error(ctx, "Failed to parse plugin manifest YAML", errCtx, err)
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate basic manifest fields
	if manifest.Name == "" {
		errCtx := WithPath(path).WithOperation("validate_manifest")
		l.logger.Error(ctx, "Plugin manifest missing required field 'name'", errCtx, nil)
		return nil, fmt.Errorf("manifest missing required field 'name'")
	}

	// Phase 1.3: Verify plugin integrity
	if err := VerifyIntegrity(path, manifest.Integrity); err != nil {
		errCtx := WithPath(path).WithOperation("verify_integrity").WithExtra("plugin_name", manifest.Name)
		l.logger.Error(ctx, "Plugin integrity verification failed", errCtx, err)
		return nil, fmt.Errorf("integrity verification failed: %w", err)
	}

	return &Plugin{
		Path:     path,
		Manifest: manifest,
	}, nil
}

// isDisabled checks if a plugin is disabled
func (l *Loader) isDisabled(name string) bool {
	for _, disabled := range l.disabled {
		if disabled == name {
			return true
		}
	}
	return false
}

// expandPath expands ~ to home directory
func expandPath(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, path[1:]), nil
}

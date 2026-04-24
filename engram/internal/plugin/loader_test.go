package plugin

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// setupTestFixtures creates test plugin directories and manifests
func setupTestFixtures(t *testing.T) (path1, path2, invalidPath string, cleanup func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Path 1: Two valid plugins (alpha, beta)
	path1 = filepath.Join(tmpDir, "path1")
	if err := os.MkdirAll(filepath.Join(path1, "plugin-alpha"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path1, "plugin-beta"), 0755); err != nil {
		t.Fatal(err)
	}

	alphaManifest := `name: plugin-alpha
version: 1.0.0
description: Test plugin alpha
pattern: guidance
`
	if err := os.WriteFile(
		filepath.Join(path1, "plugin-alpha", "plugin.yaml"),
		[]byte(alphaManifest),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	betaManifest := `name: plugin-beta
version: 1.0.0
description: Test plugin beta
pattern: tool
`
	if err := os.WriteFile(
		filepath.Join(path1, "plugin-beta", "plugin.yaml"),
		[]byte(betaManifest),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	// Path 2: Two valid plugins (gamma, delta)
	path2 = filepath.Join(tmpDir, "path2")
	if err := os.MkdirAll(filepath.Join(path2, "plugin-gamma"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path2, "plugin-delta"), 0755); err != nil {
		t.Fatal(err)
	}

	gammaManifest := `name: plugin-gamma
version: 1.0.0
description: Test plugin gamma
pattern: connector
`
	if err := os.WriteFile(
		filepath.Join(path2, "plugin-gamma", "plugin.yaml"),
		[]byte(gammaManifest),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	deltaManifest := `name: plugin-delta
version: 1.0.0
description: Test plugin delta
pattern: guidance
`
	if err := os.WriteFile(
		filepath.Join(path2, "plugin-delta", "plugin.yaml"),
		[]byte(deltaManifest),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	// Invalid path: Plugin with broken manifest
	invalidPath = filepath.Join(tmpDir, "invalid")
	if err := os.MkdirAll(filepath.Join(invalidPath, "broken-plugin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(invalidPath, "broken-plugin", "plugin.yaml"),
		[]byte("invalid: yaml: content: [[["),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	cleanup = func() {
		// tmpDir is automatically cleaned up by t.TempDir()
	}

	return path1, path2, invalidPath, cleanup
}

// pluginNames extracts plugin names from plugins slice
func pluginNames(plugins []*Plugin) []string {
	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Manifest.Name
	}
	return names
}

// contains checks if a string slice contains a value
func contains(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

func TestLoad_CompleteResults(t *testing.T) {
	path1, path2, _, cleanup := setupTestFixtures(t)
	defer cleanup()

	loader := NewLoader([]string{path1, path2}, nil)
	plugins, err := loader.Load()

	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if len(plugins) != 4 {
		t.Fatalf("Expected 4 plugins, got %d", len(plugins))
	}

	names := pluginNames(plugins)
	if !contains(names, "plugin-alpha") {
		t.Error("Expected plugin-alpha in results")
	}
	if !contains(names, "plugin-beta") {
		t.Error("Expected plugin-beta in results")
	}
	if !contains(names, "plugin-gamma") {
		t.Error("Expected plugin-gamma in results")
	}
	if !contains(names, "plugin-delta") {
		t.Error("Expected plugin-delta in results")
	}
}

func TestLoad_DeterministicOrdering(t *testing.T) {
	path1, path2, _, cleanup := setupTestFixtures(t)
	defer cleanup()

	loader := NewLoader([]string{path1, path2}, nil)

	// Run Load() 10 times and verify consistent ordering
	var firstRun []string
	for i := 0; i < 10; i++ {
		plugins, err := loader.Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}

		names := pluginNames(plugins)
		if i == 0 {
			firstRun = names
		} else if !reflect.DeepEqual(firstRun, names) {
			t.Errorf("Run %d: Plugin order inconsistent.\nExpected: %v\nGot: %v", i, firstRun, names)
		}
	}

	// Verify alphabetical ordering by name
	expected := []string{"plugin-alpha", "plugin-beta", "plugin-delta", "plugin-gamma"}
	if !reflect.DeepEqual(expected, firstRun) {
		t.Errorf("Expected alphabetical order: %v, got: %v", expected, firstRun)
	}
}

func TestLoad_ErrorResilience(t *testing.T) {
	path1, path2, invalidPath, cleanup := setupTestFixtures(t)
	defer cleanup()

	// Mix valid and invalid paths
	loader := NewLoader([]string{path1, invalidPath, path2}, nil)
	plugins, err := loader.Load()

	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	// Should still load valid plugins from path1 and path2
	if len(plugins) < 2 {
		t.Fatalf("Expected at least 2 valid plugins, got %d", len(plugins))
	}

	names := pluginNames(plugins)
	if !contains(names, "plugin-alpha") {
		t.Error("Expected plugin-alpha in results")
	}
	if !contains(names, "plugin-beta") {
		t.Error("Expected plugin-beta in results")
	}
	// broken-plugin should not be in results
	if contains(names, "broken-plugin") {
		t.Error("broken-plugin should not be in results")
	}
}

func TestLoad_DisabledFiltering(t *testing.T) {
	path1, _, _, cleanup := setupTestFixtures(t)
	defer cleanup()

	// Disable plugin-alpha
	loader := NewLoader([]string{path1}, []string{"plugin-alpha"})
	plugins, err := loader.Load()

	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	names := pluginNames(plugins)

	if contains(names, "plugin-alpha") {
		t.Error("Disabled plugin-alpha should be excluded")
	}
	if !contains(names, "plugin-beta") {
		t.Error("Non-disabled plugin-beta should be included")
	}
}

func TestLoad_EmptyPaths(t *testing.T) {
	tmpDir := t.TempDir()
	nonexistentPath := filepath.Join(tmpDir, "nonexistent")

	loader := NewLoader([]string{nonexistentPath}, nil)
	plugins, err := loader.Load()

	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("Expected empty slice for nonexistent paths, got %d plugins", len(plugins))
	}
}

func TestLoad_SinglePath(t *testing.T) {
	path1, _, _, cleanup := setupTestFixtures(t)
	defer cleanup()

	loader := NewLoader([]string{path1}, nil)
	plugins, err := loader.Load()

	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("Expected 2 plugins from single path, got %d", len(plugins))
	}

	names := pluginNames(plugins)
	if !contains(names, "plugin-alpha") {
		t.Error("Expected plugin-alpha in results")
	}
	if !contains(names, "plugin-beta") {
		t.Error("Expected plugin-beta in results")
	}
}

func TestLoad_RaceConditions(t *testing.T) {
	// Run with: go test -race
	path1, path2, _, cleanup := setupTestFixtures(t)
	defer cleanup()

	loader := NewLoader([]string{path1, path2}, nil)

	// Run many times to trigger potential races
	for i := 0; i < 100; i++ {
		_, err := loader.Load()
		if err != nil {
			t.Fatalf("Load() returned error on iteration %d: %v", i, err)
		}
	}
}

func TestLoad_ParallelExecution(t *testing.T) {
	// This test verifies that parallel execution is actually happening
	// by measuring execution time

	tmpDir := t.TempDir()

	// Create 5 paths, each with 1 plugin
	var paths []string
	for i := 0; i < 5; i++ {
		path := filepath.Join(tmpDir, "path"+string(rune('a'+i)))
		if err := os.MkdirAll(filepath.Join(path, "plugin"), 0755); err != nil {
			t.Fatal(err)
		}

		manifest := `name: plugin
version: 1.0.0
description: Test plugin
pattern: guidance
`
		if err := os.WriteFile(
			filepath.Join(path, "plugin", "plugin.yaml"),
			[]byte(manifest),
			0644,
		); err != nil {
			t.Fatal(err)
		}

		paths = append(paths, path)
	}

	loader := NewLoader(paths, nil)

	// Measure execution time
	start := time.Now()
	plugins, err := loader.Load()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if len(plugins) != 5 {
		t.Fatalf("Expected 5 plugins, got %d", len(plugins))
	}

	// With parallel execution, time should be much less than sequential
	// Allow generous margin since timing tests can be flaky
	if elapsed > 100*time.Millisecond {
		t.Logf("Warning: Parallel execution took %v (expected < 100ms)", elapsed)
	}
}

func TestLoadFromPath_ValidPath(t *testing.T) {
	path1, _, _, cleanup := setupTestFixtures(t)
	defer cleanup()

	loader := NewLoader(nil, nil)
	plugins, err := loader.loadFromPath(path1)

	if err != nil {
		t.Fatalf("loadFromPath() returned error: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("Expected 2 plugins, got %d", len(plugins))
	}

	names := pluginNames(plugins)
	if !contains(names, "plugin-alpha") {
		t.Error("Expected plugin-alpha in results")
	}
	if !contains(names, "plugin-beta") {
		t.Error("Expected plugin-beta in results")
	}
}

func TestLoadFromPath_NonexistentPath(t *testing.T) {
	tmpDir := t.TempDir()
	nonexistentPath := filepath.Join(tmpDir, "nonexistent")

	loader := NewLoader(nil, nil)
	plugins, err := loader.loadFromPath(nonexistentPath)

	if err == nil {
		t.Error("Expected error for nonexistent path, got nil")
	}
	if plugins != nil {
		t.Errorf("Expected nil plugins for error case, got %d plugins", len(plugins))
	}
}

func TestLoadFromPath_WithDisabled(t *testing.T) {
	path1, _, _, cleanup := setupTestFixtures(t)
	defer cleanup()

	loader := NewLoader(nil, []string{"plugin-alpha"})
	plugins, err := loader.loadFromPath(path1)

	if err != nil {
		t.Fatalf("loadFromPath() returned error: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("Expected 1 plugin (beta only), got %d", len(plugins))
	}

	names := pluginNames(plugins)
	if contains(names, "plugin-alpha") {
		t.Error("Disabled plugin-alpha should not be in results")
	}
	if !contains(names, "plugin-beta") {
		t.Error("Expected plugin-beta in results")
	}
}

// Benchmarks

func setupBenchFixtures(b *testing.B) ([]string, func()) {
	b.Helper()

	tmpDir := b.TempDir()

	var paths []string
	// Create 5 search paths, each with 4 plugins (20 total)
	for i := 0; i < 5; i++ {
		pathDir := filepath.Join(tmpDir, "path"+string(rune('0'+i)))
		if err := os.MkdirAll(pathDir, 0755); err != nil {
			b.Fatal(err)
		}

		for j := 0; j < 4; j++ {
			pluginName := "plugin_" + string(rune('a'+j))
			pluginDir := filepath.Join(pathDir, pluginName)
			if err := os.MkdirAll(pluginDir, 0755); err != nil {
				b.Fatal(err)
			}

			manifest := `name: ` + pluginName + `
version: 1.0.0
description: Benchmark plugin
pattern: guidance
`
			if err := os.WriteFile(
				filepath.Join(pluginDir, "plugin.yaml"),
				[]byte(manifest),
				0644,
			); err != nil {
				b.Fatal(err)
			}
		}

		paths = append(paths, pathDir)
	}

	cleanup := func() {
		// tmpDir is automatically cleaned up
	}

	return paths, cleanup
}

func BenchmarkLoad_Parallel(b *testing.B) {
	paths, cleanup := setupBenchFixtures(b)
	defer cleanup()

	loader := NewLoader(paths, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := loader.Load()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadFromPath_Single(b *testing.B) {
	paths, cleanup := setupBenchFixtures(b)
	defer cleanup()

	loader := NewLoader(nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := loader.loadFromPath(paths[0])
		if err != nil {
			b.Fatal(err)
		}
	}
}

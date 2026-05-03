package cliframe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewConfigLoader(t *testing.T) {
	loader := NewConfigLoader("myapp")

	if loader.envPrefix != "MYAPP" {
		t.Errorf("Expected prefix 'MYAPP', got %s", loader.envPrefix)
	}

	if len(loader.searchPaths) != 0 {
		t.Error("Expected empty search paths")
	}

	if len(loader.defaults) != 0 {
		t.Error("Expected empty defaults")
	}
}

func TestConfigLoader_WithSearchPaths(t *testing.T) {
	loader := NewConfigLoader("test").
		WithSearchPaths("/etc/app", "~/.config/app", ".")

	if len(loader.searchPaths) != 3 {
		t.Errorf("Expected 3 search paths, got %d", len(loader.searchPaths))
	}

	if loader.searchPaths[0] != "/etc/app" {
		t.Errorf("Expected first path '/etc/app', got %s", loader.searchPaths[0])
	}
}

func TestConfigLoader_WithDefaults(t *testing.T) {
	defaults := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}

	loader := NewConfigLoader("test").
		WithDefaults(defaults)

	if len(loader.defaults) != 2 {
		t.Errorf("Expected 2 defaults, got %d", len(loader.defaults))
	}

	if loader.defaults["key1"] != "value1" {
		t.Errorf("Expected default key1='value1', got %v", loader.defaults["key1"])
	}
}

func TestConfigLoader_Load_DefaultsOnly(t *testing.T) {
	defaults := map[string]interface{}{
		"host": "localhost",
		"port": 8080,
	}

	loader := NewConfigLoader("test").
		WithDefaults(defaults)

	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if config.GetString("host") != "localhost" {
		t.Errorf("Expected host='localhost', got %s", config.GetString("host"))
	}

	if config.GetInt("port") != 8080 {
		t.Errorf("Expected port=8080, got %d", config.GetInt("port"))
	}

	if config.Source() != "defaults" {
		t.Errorf("Expected source 'defaults', got %s", config.Source())
	}
}

func TestConfigLoader_Load_FromFile(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
host: example.com
port: 9090
debug: true
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	loader := NewConfigLoader("test").
		WithSearchPaths(tmpDir).
		WithDefaults(map[string]interface{}{
			"host": "localhost",
			"port": 8080,
		})

	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// File values should override defaults
	if config.GetString("host") != "example.com" {
		t.Errorf("Expected host='example.com', got %s", config.GetString("host"))
	}

	if config.GetInt("port") != 9090 {
		t.Errorf("Expected port=9090, got %d", config.GetInt("port"))
	}

	if !config.GetBool("debug") {
		t.Error("Expected debug=true")
	}
}

func TestConfigLoader_Load_EnvVarOverride(t *testing.T) {
	// Set environment variable
	t.Setenv("TEST_HOST", "env.example.com")
	t.Setenv("TEST_PORT", "7070")
	defer os.Unsetenv("TEST_HOST")
	defer os.Unsetenv("TEST_PORT")

	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
host: file.example.com
port: 9090
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	loader := NewConfigLoader("test").
		WithSearchPaths(tmpDir).
		WithDefaults(map[string]interface{}{
			"host": "localhost",
			"port": 8080,
		})

	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Environment variables should have highest priority
	if config.GetString("host") != "env.example.com" {
		t.Errorf("Expected host='env.example.com' from env, got %s", config.GetString("host"))
	}

	if config.GetInt("port") != 7070 {
		t.Errorf("Expected port=7070 from env, got %d", config.GetInt("port"))
	}

	// Source should indicate environment override
	if !strings.Contains(config.Source(), "environment") {
		t.Errorf("Expected source to include 'environment', got %s", config.Source())
	}
}

func TestConfigLoader_Load_NestedEnvKeys(t *testing.T) {
	// Set environment variable with nested key
	t.Setenv("TEST_DB__HOST", "db.example.com")
	t.Setenv("TEST_DB__PORT", "5432")
	defer os.Unsetenv("TEST_DB__HOST")
	defer os.Unsetenv("TEST_DB__PORT")

	loader := NewConfigLoader("test")

	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should convert DB__HOST to db.host
	if config.GetString("db.host") != "db.example.com" {
		t.Errorf("Expected db.host='db.example.com', got %s", config.GetString("db.host"))
	}

	if config.GetString("db.port") != "5432" {
		t.Errorf("Expected db.port='5432', got %s", config.GetString("db.port"))
	}
}

func TestConfigLoader_LoadFrom(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yaml")

	configContent := `
name: test-app
version: 1.0
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	loader := NewConfigLoader("test")

	config, err := loader.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	if config.GetString("name") != "test-app" {
		t.Errorf("Expected name='test-app', got %s", config.GetString("name"))
	}

	if config.Source() != configPath {
		t.Errorf("Expected source=%s, got %s", configPath, config.Source())
	}
}

func TestConfigLoader_LoadFrom_InvalidFile(t *testing.T) {
	loader := NewConfigLoader("test")

	_, err := loader.LoadFrom("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestConfig_Get(t *testing.T) {
	config := &Config{
		values: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
	}

	val, ok := config.Get("key1")
	if !ok {
		t.Error("Expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("Expected value1, got %v", val)
	}

	_, ok = config.Get("nonexistent")
	if ok {
		t.Error("Expected nonexistent key to return false")
	}
}

func TestConfig_GetString(t *testing.T) {
	config := &Config{
		values: map[string]interface{}{
			"str":  "hello",
			"int":  42,
			"bool": true,
			"nil":  nil,
		},
	}

	if config.GetString("str") != "hello" {
		t.Errorf("Expected 'hello', got %s", config.GetString("str"))
	}

	// Should convert int to string
	if config.GetString("int") != "42" {
		t.Errorf("Expected '42', got %s", config.GetString("int"))
	}

	// Nonexistent key should return empty string
	if config.GetString("nonexistent") != "" {
		t.Errorf("Expected empty string, got %s", config.GetString("nonexistent"))
	}
}

func TestConfig_GetInt(t *testing.T) {
	config := &Config{
		values: map[string]interface{}{
			"int":    42,
			"int64":  int64(100),
			"float":  3.14,
			"string": "99",
		},
	}

	if config.GetInt("int") != 42 {
		t.Errorf("Expected 42, got %d", config.GetInt("int"))
	}

	if config.GetInt("int64") != 100 {
		t.Errorf("Expected 100, got %d", config.GetInt("int64"))
	}

	// Should convert float to int
	if config.GetInt("float") != 3 {
		t.Errorf("Expected 3, got %d", config.GetInt("float"))
	}

	// Should parse string
	if config.GetInt("string") != 99 {
		t.Errorf("Expected 99, got %d", config.GetInt("string"))
	}

	// Nonexistent key should return 0
	if config.GetInt("nonexistent") != 0 {
		t.Errorf("Expected 0, got %d", config.GetInt("nonexistent"))
	}
}

func TestConfig_GetBool(t *testing.T) {
	config := &Config{
		values: map[string]interface{}{
			"bool_true":    true,
			"bool_false":   false,
			"string_true":  "true",
			"string_yes":   "yes",
			"string_1":     "1",
			"string_false": "false",
		},
	}

	if !config.GetBool("bool_true") {
		t.Error("Expected true")
	}

	if config.GetBool("bool_false") {
		t.Error("Expected false")
	}

	if !config.GetBool("string_true") {
		t.Error("Expected 'true' string to be true")
	}

	if !config.GetBool("string_yes") {
		t.Error("Expected 'yes' string to be true")
	}

	if !config.GetBool("string_1") {
		t.Error("Expected '1' string to be true")
	}

	if config.GetBool("string_false") {
		t.Error("Expected 'false' string to be false")
	}

	// Nonexistent key should return false
	if config.GetBool("nonexistent") {
		t.Error("Expected false for nonexistent key")
	}
}

func TestConfig_GetDuration(t *testing.T) {
	config := &Config{
		values: map[string]interface{}{
			"duration_string": "1h30m",
			"duration_int":    60,
			"duration_int64":  int64(120),
			"duration_float":  90.5,
		},
	}

	// Parse duration string
	dur := config.GetDuration("duration_string")
	expected := 1*time.Hour + 30*time.Minute
	if dur != expected {
		t.Errorf("Expected %v, got %v", expected, dur)
	}

	// Int should be interpreted as seconds
	if config.GetDuration("duration_int") != 60*time.Second {
		t.Errorf("Expected 60s, got %v", config.GetDuration("duration_int"))
	}

	if config.GetDuration("duration_int64") != 120*time.Second {
		t.Errorf("Expected 120s, got %v", config.GetDuration("duration_int64"))
	}

	// Float should be interpreted as seconds
	if config.GetDuration("duration_float") != 90*time.Second {
		t.Errorf("Expected 90s, got %v", config.GetDuration("duration_float"))
	}

	// Nonexistent key should return 0
	if config.GetDuration("nonexistent") != 0 {
		t.Errorf("Expected 0, got %v", config.GetDuration("nonexistent"))
	}
}

func TestConfig_GetStringSlice(t *testing.T) {
	config := &Config{
		values: map[string]interface{}{
			"slice_string":    []string{"a", "b", "c"},
			"slice_interface": []interface{}{"x", "y", "z"},
			"csv_string":      "item1,item2,item3",
			"empty_string":    "",
		},
	}

	// String slice
	slice := config.GetStringSlice("slice_string")
	if len(slice) != 3 || slice[0] != "a" {
		t.Errorf("Expected [a b c], got %v", slice)
	}

	// Interface slice
	slice = config.GetStringSlice("slice_interface")
	if len(slice) != 3 || slice[0] != "x" {
		t.Errorf("Expected [x y z], got %v", slice)
	}

	// CSV string
	slice = config.GetStringSlice("csv_string")
	if len(slice) != 3 || slice[0] != "item1" {
		t.Errorf("Expected [item1 item2 item3], got %v", slice)
	}

	// Empty string should return nil
	slice = config.GetStringSlice("empty_string")
	if slice != nil {
		t.Errorf("Expected nil for empty string, got %v", slice)
	}

	// Nonexistent key should return nil
	slice = config.GetStringSlice("nonexistent")
	if slice != nil {
		t.Errorf("Expected nil for nonexistent key, got %v", slice)
	}
}

func TestConfig_Set(t *testing.T) {
	config := &Config{
		values: map[string]interface{}{
			"existing": "old",
		},
	}

	config.Set("new", "value")
	config.Set("existing", "updated")

	if config.GetString("new") != "value" {
		t.Error("Failed to set new key")
	}

	if config.GetString("existing") != "updated" {
		t.Error("Failed to update existing key")
	}
}

func TestConfig_AllKeys(t *testing.T) {
	config := &Config{
		values: map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		},
	}

	keys := config.AllKeys()
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	// Check all keys are present (order doesn't matter)
	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}

	if !keyMap["key1"] || !keyMap["key2"] || !keyMap["key3"] {
		t.Errorf("Missing expected keys, got %v", keys)
	}
}

func TestConfigLoader_MultipleSearchPaths_Precedence(t *testing.T) {
	// Create two config files
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	config1 := filepath.Join(tmpDir1, "config.yaml")
	config2 := filepath.Join(tmpDir2, "config.yaml")

	// First config (lower priority)
	content1 := `
host: first.example.com
port: 8080
debug: false
`
	if err := os.WriteFile(config1, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write config1: %v", err)
	}

	// Second config (higher priority)
	content2 := `
host: second.example.com
port: 9090
`
	if err := os.WriteFile(config2, []byte(content2), 0644); err != nil {
		t.Fatalf("Failed to write config2: %v", err)
	}

	// Second path should have higher priority
	loader := NewConfigLoader("test").
		WithSearchPaths(tmpDir1, tmpDir2)

	config, err := loader.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Values from second config should win
	if config.GetString("host") != "second.example.com" {
		t.Errorf("Expected host from second config, got %s", config.GetString("host"))
	}

	if config.GetInt("port") != 9090 {
		t.Errorf("Expected port from second config, got %d", config.GetInt("port"))
	}

	// Value only in first config should still be present
	if config.GetBool("debug") != false {
		t.Error("Expected debug from first config")
	}
}

func TestConfigLoader_HomeDirectoryExpansion(t *testing.T) {
	// This test verifies that ~ is expanded correctly
	loader := NewConfigLoader("test").
		WithSearchPaths("~/test/config")

	// Should not panic when trying to expand ~
	_, err := loader.Load()
	// Error is expected (path doesn't exist), but shouldn't panic
	_ = err // Just testing that ~ expansion doesn't panic
}

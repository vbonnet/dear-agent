# config-loader

Generic YAML configuration loading library with persona support, frontmatter parsing, path resolution, and home directory expansion.

## Features

- 🎯 **Generic loader** - Works with any config struct type via Go generics
- 👤 **Persona loading** - Load and validate .ai.md persona files
- 📄 **Frontmatter parsing** - Extract YAML frontmatter from Markdown files
- 🗺️ **Path resolution** - Resolve relative paths with workspace/home support
- 🏠 **Home expansion** - Automatic `~/` expansion in file paths
- 🛡️ **Graceful fallback** - Optional configs with sensible defaults
- 📝 **Clear errors** - Comprehensive error messages with context
- ⚡ **Zero dependencies** - Only `gopkg.in/yaml.v3` required
- ✅ **Well tested** - Comprehensive test coverage

## Installation

```bash
go get github.com/vbonnet/engram/libs/config-loader
```

## Quick Start

### Loading Personas

```go
import "github.com/vbonnet/engram/libs/config-loader"

// Load a single persona
persona, err := configloader.LoadPersona("~/personas/security-engineer.ai.md")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Loaded %s v%s\n", persona.DisplayName, persona.Version)

// Load persona by name from library
opts := configloader.PersonaLoadOptions{
    LibraryPath: "~/personas/library",
    Recursive:   true,
}
persona, err := configloader.LoadPersonaByName("security-engineer", opts)

// List all personas
personas, err := configloader.ListPersonas(opts)
for name, p := range personas {
    fmt.Printf("%s: %s\n", name, p.DisplayName)
}
```

### Parsing Frontmatter

```go
content := `---
name: test
version: 1.0.0
---
Markdown content here`

frontmatter, body, err := configloader.ParseFrontmatter(content)
// frontmatter = "name: test\nversion: 1.0.0"
// body = "Markdown content here"

// Check if content has frontmatter
if configloader.HasFrontmatter(content) {
    // ... process
}
```

### Path Resolution

```go
// Resolve relative paths
path, err := configloader.ResolvePath("config/app.yaml", "/workspace")
// Returns: "/workspace/config/app.yaml"

// Tilde expansion
path, err := configloader.ResolvePath("~/.config/app.yaml", "")
// Returns: "$HOME/.config/app.yaml"

// Find file in search paths
searchPaths := []string{"~/.config", "/etc/app", "./config"}
path, err := configloader.FindFile("app.yaml", searchPaths)
// Returns first existing match
```

### Define your configuration struct

```go
type AppConfig struct {
    Name    string `yaml:"name"`
    Timeout int    `yaml:"timeout"`
    LogPath string `yaml:"log_path"`
}
```

### Load required configuration

```go
import "github.com/vbonnet/engram/libs/config-loader"

cfg, err := configloader.Load[AppConfig]("~/.config/app/config.yaml")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Loaded config: %+v\n", cfg)
```

### Load optional configuration with defaults

```go
defaults := AppConfig{
    Name:    "my-app",
    Timeout: 30,
    LogPath: "~/logs/app.log",
}

// Returns defaults if file doesn't exist, error if invalid YAML
cfg, err := configloader.LoadWithDefaults("config.yaml", defaults)
if err != nil {
    log.Fatal(err)
}
```

### Load truly optional configuration

```go
defaults := AppConfig{Name: "my-app", Timeout: 30}

// Never returns error - uses defaults on any failure
cfg := configloader.LoadOrDefault("config.yaml", defaults)
fmt.Printf("Config: %+v\n", cfg)
```

## API Reference

### `Load[T any](path string) (*T, error)`

Loads and parses a YAML config file. Returns error if file doesn't exist or YAML is invalid.

**When to use:** Required configuration files

**Example:**
```go
cfg, err := configloader.Load[MyConfig]("config.yaml")
if err != nil {
    return fmt.Errorf("config required: %w", err)
}
```

---

### `LoadWithDefaults[T any](path string, defaults T) (*T, error)`

Loads config from file, uses defaults if file doesn't exist. Still returns error for invalid YAML.

**When to use:** Optional config files where you want to catch parsing errors

**Example:**
```go
defaults := MyConfig{Port: 8080}
cfg, err := configloader.LoadWithDefaults("config.yaml", defaults)
if err != nil {
    return fmt.Errorf("invalid config: %w", err)
}
```

---

### `LoadOrDefault[T any](path string, defaults T) *T`

Loads config from file, always returns valid config. Never returns error.

**When to use:** Truly optional configs where errors should be silently ignored

**Example:**
```go
defaults := MyConfig{Port: 8080}
cfg := configloader.LoadOrDefault("config.yaml", defaults)
// cfg is guaranteed non-nil
server.Start(cfg.Port)
```

---

### `ExpandHome(path string) (string, error)`

Expands `~` to the user's home directory. Standalone utility function.

**Example:**
```go
path, _ := configloader.ExpandHome("~/.config/app")
// Returns: "~/.config/app"
```

**Behavior:**
- `~` → `/home/user`
- `~/path` → `~/path`
- `/absolute/path` → `/absolute/path` (unchanged)
- `relative/path` → `relative/path` (unchanged)

## Home Directory Expansion

All loading functions automatically expand `~` in file paths:

```yaml
# config.yaml
log_path: "~/logs/app.log"
data_dir: "~/.local/share/app"
```

```go
cfg, _ := configloader.Load[Config]("~/.config/app/config.yaml")
// cfg.LogPath will be "~/logs/app.log"
// cfg.DataDir will be "~/.local/share/app"
```

## Error Handling Comparison

| Function | File Missing | Invalid YAML | Permission Denied |
|----------|-------------|--------------|-------------------|
| `Load` | ❌ Error | ❌ Error | ❌ Error |
| `LoadWithDefaults` | ✅ Use defaults | ❌ Error | ❌ Error |
| `LoadOrDefault` | ✅ Use defaults | ✅ Use defaults | ✅ Use defaults |

## Configuration Validation

This library handles YAML parsing and path expansion **only**. You are responsible for validating semantic correctness:

```go
cfg, err := configloader.Load[MyConfig]("config.yaml")
if err != nil {
    return err
}

// Validate business logic constraints
if err := cfg.Validate(); err != nil {
    return fmt.Errorf("invalid config: %w", err)
}
```

Example validation method:

```go
func (c *MyConfig) Validate() error {
    if c.Port < 1 || c.Port > 65535 {
        return fmt.Errorf("port must be between 1 and 65535")
    }
    if c.Timeout <= 0 {
        return fmt.Errorf("timeout must be positive")
    }
    return nil
}
```

## Environment Variable Overrides

This library does **not** handle environment variable overrides, as each tool has different naming conventions (`CSM_*`, `SWARM_*`, `AGM_*`, etc.).

Apply env var overrides after loading:

```go
cfg, _ := configloader.Load[MyConfig]("config.yaml")

// Override with environment variables
if port := os.Getenv("APP_PORT"); port != "" {
    if p, err := strconv.Atoi(port); err == nil {
        cfg.Port = p
    }
}
if logPath := os.Getenv("APP_LOG_PATH"); logPath != "" {
    cfg.LogPath = logPath
}
```

## Migration Guide

### Migrating from custom config loaders

**Before:**
```go
// Old custom loader
func loadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(expandHome(path))
    if err != nil {
        return nil, err
    }
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
```

**After:**
```go
import "github.com/vbonnet/engram/libs/config-loader"

// Just use the library
cfg, err := configloader.Load[Config](path)
```

### Benefits

- ✅ Eliminates ~20 LOC per loader
- ✅ Consistent error handling across tools
- ✅ Single source of truth for YAML loading
- ✅ Comprehensive test coverage (93.8%)
- ✅ Well-documented edge cases

## Requirements

- Go 1.21 or later (uses generics)
- `gopkg.in/yaml.v3 v3.0.1+` (fixes CVE-2022-28948)

## Testing

```bash
# Run tests
go test -v

# Run tests with coverage
go test -v -cover

# Generate coverage report
go test -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## License

Same as parent engram project.

## Contributing

This library is part of the engram monorepo. See the main engram repository for contribution guidelines.

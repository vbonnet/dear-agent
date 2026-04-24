# Corpus Callosum Go API

Go library API reference for integrating Corpus Callosum into components.

## Package: registry

### Creating a Registry

```go
import "github.com/ai-tools/corpus-callosum/internal/registry"

// Create registry for current workspace
workspace := registry.DetectWorkspace()
reg, err := registry.New(workspace)
if err != nil {
    log.Fatalf("Failed to create registry: %v", err)
}
defer reg.Close()

// Or use specific workspace
reg, err := registry.New("oss")
```

### Registering Schemas

```go
schemaData := map[string]interface{}{
    "$schema":       "https://corpus-callosum.dev/schema/v1",
    "component":     "my-component",
    "version":       "1.0.0",
    "compatibility": "backward",
    "schemas": map[string]interface{}{
        "my-schema": map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "id": map[string]interface{}{
                    "type": "string",
                },
            },
            "required": []interface{}{"id"},
        },
    },
}

err = reg.RegisterSchema("my-component", "1.0.0", "backward", schemaData)
if err != nil {
    log.Fatalf("Failed to register: %v", err)
}
```

### Retrieving Schemas

```go
// Get latest version
schema, err := reg.GetSchema("my-component", "")

// Get specific version
schema, err := reg.GetSchema("my-component", "1.0.0")

// Schema structure
type Schema struct {
    ID            int
    Component     string
    Version       string
    Compatibility string
    SchemaJSON    string  // JSON-encoded schema
    CreatedAt     int64
}
```

### Listing Components

```go
components, err := reg.ListComponents()
for _, comp := range components {
    fmt.Printf("%s v%s: %s\n",
        comp.Component,
        comp.LatestVersion,
        comp.Description)
}

// Component structure
type Component struct {
    Component     string
    Description   string
    LatestVersion string
    InstalledAt   int64
    UpdatedAt     int64
}
```

### Listing Versions

```go
versions, err := reg.ListVersions("my-component")
for _, v := range versions {
    fmt.Printf("v%s (compatibility: %s)\n", v.Version, v.Compatibility)
}
```

### Unregistering Schemas

```go
// Unregister specific version
err = reg.UnregisterSchema("my-component", "1.0.0")

// Unregister all versions
err = reg.UnregisterSchema("my-component", "")
```

### Workspace Detection

```go
// Detect workspace from current directory
workspace := registry.DetectWorkspace()

// Pattern: ~/src/ws/{workspace}/*
// Returns "global" if not in workspace directory
```

## Package: schema

### Validating Schemas

```go
import "github.com/ai-tools/corpus-callosum/internal/schema"

schemaData := map[string]interface{}{
    "$schema":   "https://corpus-callosum.dev/schema/v1",
    "component": "my-component",
    "version":   "1.0.0",
    "schemas": map[string]interface{}{
        "test": map[string]interface{}{
            "type": "object",
        },
    },
}

err := schema.ValidateSchema(schemaData)
if err != nil {
    log.Fatalf("Invalid schema: %v", err)
}
```

### Validating Data

```go
schemaDefinition := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "id": map[string]interface{}{
            "type": "string",
        },
        "count": map[string]interface{}{
            "type":    "integer",
            "minimum": 0,
        },
    },
    "required": []interface{}{"id"},
}

data := map[string]interface{}{
    "id":    "test-123",
    "count": 42,
}

err := schema.ValidateData(schemaDefinition, data)
if err != nil {
    log.Fatalf("Validation failed: %v", err)
}
```

### Checking Compatibility

```go
oldSchema := map[string]interface{}{
    "schemas": map[string]interface{}{
        "user": map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "id": map[string]interface{}{"type": "string"},
                "name": map[string]interface{}{"type": "string"},
            },
            "required": []interface{}{"id", "name"},
        },
    },
}

newSchema := map[string]interface{}{
    "schemas": map[string]interface{}{
        "user": map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "id": map[string]interface{}{"type": "string"},
                "name": map[string]interface{}{"type": "string"},
                "email": map[string]interface{}{
                    "type":    "string",
                    "default": "",
                },
            },
            "required": []interface{}{"id", "name"},
        },
    },
}

result := schema.CheckCompatibility(
    oldSchema,
    newSchema,
    schema.CompatibilityBackward,
)

if !result.Passed {
    for _, violation := range result.Violations {
        fmt.Printf("Violation: %s\n", violation)
    }
}

// CompatibilityResult structure
type CompatibilityResult struct {
    Passed     bool
    Mode       string
    Violations []string
    Warnings   []string
}
```

### Compatibility Modes

```go
const (
    CompatibilityBackward CompatibilityMode = "backward"
    CompatibilityForward  CompatibilityMode = "forward"
    CompatibilityFull     CompatibilityMode = "full"
    CompatibilityNone     CompatibilityMode = "none"
)
```

## Component Integration Example

```go
package main

import (
    "encoding/json"
    "log"
    "os"

    "github.com/ai-tools/corpus-callosum/internal/registry"
    "github.com/ai-tools/corpus-callosum/internal/schema"
)

func main() {
    // Check if Corpus Callosum is available
    if !corpusCallosumAvailable() {
        log.Println("Corpus Callosum not available, using local storage")
        return
    }

    // Initialize registry
    workspace := registry.DetectWorkspace()
    reg, err := registry.New(workspace)
    if err != nil {
        log.Fatalf("Failed to create registry: %v", err)
    }
    defer reg.Close()

    // Load component schema
    schemaFile, err := os.ReadFile("./schema.json")
    if err != nil {
        log.Fatalf("Failed to read schema: %v", err)
    }

    var schemaData map[string]interface{}
    if err := json.Unmarshal(schemaFile, &schemaData); err != nil {
        log.Fatalf("Failed to parse schema: %v", err)
    }

    // Validate schema format
    if err := schema.ValidateSchema(schemaData); err != nil {
        log.Fatalf("Invalid schema: %v", err)
    }

    // Get component details from schema
    component := schemaData["component"].(string)
    version := schemaData["version"].(string)
    compatibility := "backward"
    if c, ok := schemaData["compatibility"].(string); ok {
        compatibility = c
    }

    // Check compatibility with previous version
    oldSchema, err := reg.GetSchema(component, "")
    if err == nil {
        var oldSchemaData map[string]interface{}
        if err := json.Unmarshal([]byte(oldSchema.SchemaJSON), &oldSchemaData); err == nil {
            result := schema.CheckCompatibility(
                oldSchemaData,
                schemaData,
                schema.CompatibilityMode(compatibility),
            )

            if !result.Passed {
                log.Fatalf("Compatibility check failed: %v", result.Violations)
            }
        }
    }

    // Register schema
    if err := reg.RegisterSchema(component, version, compatibility, schemaData); err != nil {
        log.Fatalf("Failed to register schema: %v", err)
    }

    log.Printf("Successfully registered %s v%s", component, version)
}

func corpusCallosumAvailable() bool {
    _, err := os.Stat("/usr/local/bin/cc")
    return err == nil
}
```

## Error Handling

### Common Errors

```go
// Schema not found
_, err := reg.GetSchema("nonexistent", "")
// err: "schema not found"

// Component not found
_, err := reg.GetComponent("nonexistent")
// err: "component not found"

// Invalid schema format
err := schema.ValidateSchema(invalidSchema)
// err: "missing required field: component"

// Compatibility violation
result := schema.CheckCompatibility(old, new, schema.CompatibilityBackward)
// result.Passed == false
// result.Violations contains error messages
```

### Best Practices

1. **Always close registry connections**:
   ```go
   reg, err := registry.New(workspace)
   if err != nil {
       return err
   }
   defer reg.Close()
   ```

2. **Check Corpus Callosum availability before use**:
   ```go
   if corpusCallosumAvailable() {
       // Register schema
   } else {
       // Fall back to local storage
   }
   ```

3. **Validate schemas before registration**:
   ```go
   if err := schema.ValidateSchema(schemaData); err != nil {
       return fmt.Errorf("invalid schema: %w", err)
   }
   ```

4. **Check compatibility on upgrades**:
   ```go
   oldSchema, _ := reg.GetSchema(component, "")
   if oldSchema != nil {
       result := schema.CheckCompatibility(...)
       if !result.Passed {
           return errors.New("breaking changes detected")
       }
   }
   ```

## Testing

### Mock Registry for Tests

```go
func TestMyComponent(t *testing.T) {
    // Use test workspace
    workspace := "test-" + t.Name()
    defer cleanupTest(workspace)

    reg, err := registry.New(workspace)
    if err != nil {
        t.Fatalf("Failed to create registry: %v", err)
    }
    defer reg.Close()

    // Run tests
}

func cleanupTest(workspace string) {
    homeDir, _ := os.UserHomeDir()
    registryPath := filepath.Join(homeDir, ".config", "corpus-callosum", workspace)
    os.RemoveAll(registryPath)
}
```

## Thread Safety

The registry uses SQLite with file-level locking. Multiple processes can safely access the same registry, but:

- Writes are serialized by SQLite
- Readers don't block each other
- Best practice: Use short-lived connections (open, operate, close)

## Performance

- Schema registration: ~1-5ms
- Schema retrieval: ~0.5-2ms
- Component listing: ~1-3ms
- Compatibility checking: ~1-10ms (depends on schema complexity)

All operations are local (SQLite file access), no network calls.

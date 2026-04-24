# ADR-001: Use YAML for DoD Specification

## Status

Accepted

## Context

The Bead DoD package needs a format for defining machine-checkable completion criteria. The format must be:

1. Human-readable and writable (non-programmers should be able to create DoD files)
2. Structured (support arrays, nested objects, multiple data types)
3. Version-controllable (plain text, clear diffs)
4. Well-supported in Go ecosystem
5. Familiar to DevOps and CI/CD users

Common formats used for configuration include:
- YAML (DevOps standard, human-friendly)
- JSON (machine-friendly, less human-friendly)
- TOML (growing popularity, good for config)
- Custom DSL (maximum flexibility, requires tooling)

## Decision

We will use **YAML** as the format for DoD specification files.

DoD files will use the `.dod.yaml` extension by convention and follow this structure:

```yaml
files_must_exist:
  - path/to/required/file.go
  - ~/config/settings.yaml

tests_must_pass: true

commands_must_succeed:
  - cmd: "make lint"
    exit_code: 0
  - cmd: "go vet ./..."
    exit_code: 0
```

We will use `gopkg.in/yaml.v3` for parsing.

## Consequences

### Positive

1. **Human-Friendly**: Easy to read and write without special tools
   - Clear syntax for lists and key-value pairs
   - Supports comments for documentation
   - No complex escaping rules

2. **Industry Standard**: Familiar to target audience
   - Used in GitHub Actions, GitLab CI, Kubernetes, Docker Compose
   - Existing knowledge transfers directly
   - Many examples and tutorials available

3. **Structured Data**: Native support for complex types
   - Arrays for file lists and command lists
   - Objects for command configuration (cmd + exit_code)
   - Nested structures for future extensions

4. **Good Tooling**: Excellent Go library support
   - `gopkg.in/yaml.v3` is mature and well-maintained
   - Struct tags for declarative mapping
   - Good error messages for syntax errors

5. **Extensibility**: Easy to add new fields without breaking changes
   - Optional fields via `omitempty`
   - Unknown fields ignored by default
   - Clear upgrade path for new features

### Negative

1. **Indentation Sensitivity**: Whitespace is significant
   - Can cause confusion for beginners
   - Copy-paste errors with mixed spaces/tabs
   - Mitigation: Use YAML linters in development

2. **Parsing Overhead**: Slower than JSON
   - Not a concern (DoD files are small, loaded once)
   - Parsing takes < 10ms for typical files

3. **Type Ambiguity**: Strings vs numbers can be confusing
   - Example: `exit_code: 0` vs `exit_code: "0"`
   - Mitigation: Strict struct types in Go (int, string, bool)

4. **Security Risks**: YAML parsing has had vulnerabilities
   - Mitigation: Use trusted, up-to-date library
   - DoD files are trusted sources (version-controlled)

5. **Multiple Syntax Variants**: YAML has edge cases
   - Mitigation: Use simple subset (no anchors, complex maps)
   - Document canonical format in spec

## Alternatives Considered

### 1. JSON

**Description**: JavaScript Object Notation

**Rejected because**:
- Less human-friendly (quotes everywhere, no trailing commas)
- No comments (cannot document DoD choices inline)
- More verbose for simple lists
- Less familiar in DevOps contexts

**Example**:
```json
{
  "files_must_exist": [
    "path/to/file.go"
  ],
  "tests_must_pass": true,
  "commands_must_succeed": [
    {"cmd": "make lint", "exit_code": 0}
  ]
}
```

### 2. TOML

**Description**: Tom's Obvious, Minimal Language

**Rejected because**:
- Less familiar to DevOps audience
- Awkward array-of-tables syntax for commands
- Less common in CI/CD tools
- Smaller ecosystem in Go

**Example**:
```toml
files_must_exist = ["path/to/file.go"]
tests_must_pass = true

[[commands_must_succeed]]
cmd = "make lint"
exit_code = 0
```

### 3. HCL (HashiCorp Configuration Language)

**Description**: Terraform's configuration language

**Rejected because**:
- Niche outside HashiCorp tools
- More complex syntax than needed
- Smaller Go ecosystem
- Overkill for simple key-value config

### 4. Custom DSL

**Description**: Domain-specific language (e.g., "REQUIRE file.go", "TEST must pass")

**Rejected because**:
- Requires custom parser development
- No existing tooling (syntax highlighting, validation)
- Steeper learning curve
- More maintenance burden
- Cannot leverage existing libraries

### 5. Go Code

**Description**: Define DoD in Go code (e.g., `dod.NewDoD().RequireFile("x")`)

**Rejected because**:
- Requires Go knowledge to create DoD
- Not easily editable by non-programmers
- Less declarative, more imperative
- Harder to generate or validate externally

## Implementation Notes

**Library Choice**: `gopkg.in/yaml.v3`
- Most popular Go YAML library
- Active maintenance
- Good struct tag support
- Clear error messages

**Struct Mapping**:
```go
type BeadDoD struct {
    FilesMustExist      []string       `yaml:"files_must_exist"`
    TestsMustPass       bool           `yaml:"tests_must_pass"`
    CommandsMustSucceed []CommandCheck `yaml:"commands_must_succeed"`
}
```

**Error Handling**:
- Parse errors wrapped with context
- Invalid fields ignored (forward compatibility)
- Strict typing for required fields

**Future Extensions**:
```yaml
# Placeholders for future features
benchmarks_must_improve:
  - name: "BenchmarkFoo"
    max_regression: "5%"

coverage_must_exceed: 80

lint_must_pass: true
```

## References

- [YAML Specification 1.2](https://yaml.org/spec/1.2.2/)
- [gopkg.in/yaml.v3 Documentation](https://pkg.go.dev/gopkg.in/yaml.v3)
- [YAML Best Practices](https://www.yaml.info/learn/bestpractices.html)

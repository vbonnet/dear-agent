# ADR-006: Security-First Input Validation

**Status**: Accepted

**Date**: 2024-02-10

**Context**: CLI tools are vulnerable to various security attacks:
- **Path traversal**: `../../etc/passwd`
- **Command injection**: `; rm -rf /`
- **Environment variable injection**: `${HOME}/../../../etc`
- **Format injection**: `%s%s%s%s`
- **Resource exhaustion**: Extremely long inputs

These attacks can:
- Expose sensitive files
- Execute arbitrary commands
- Escalate privileges
- Crash the application

Traditional validation (basic length checks) is insufficient for security-critical operations.

**Decision**: Implement defense-in-depth input validation with security-first design:

1. **Whitelist-based path validation**
2. **Environment variable expansion validation**
3. **Input length limits**
4. **Format validation**
5. **Range validation for numeric inputs**

**Validation Functions**:

```go
// Path Security
ValidateSafePath(field, path string, allowedPaths []string) error
ValidateSafeEnvExpansion(field, path string, allowedPaths []string) error
GetAllowedPaths() ([]string, error)

// Input Validation
ValidateNonEmpty(field, value string) error
ValidateMaxLength(field, value string, max int) error
ValidateRangeInt(field, value, min, max int) error

// Format Validation
ValidateOutputFormat(format string, allowed ...string) error
ValidateTier(tier string) error
```

**Security Principles**:

**1. Whitelist over Blacklist**
```go
// GOOD: Whitelist allowed paths
allowedPaths := []string{
    filepath.Join(home, ".engram"),
    home,
}
ValidateSafePath(path, allowedPaths)

// BAD: Blacklist dangerous patterns
if strings.Contains(path, "..") {  // Insufficient!
    return errors.New("path traversal")
}
```

**2. Canonical Path Validation**
```go
// Resolve symlinks and get absolute path
realPath, err := filepath.EvalSymlinks(path)
realPath, err = filepath.Abs(realPath)

// Then check against whitelist
for _, allowed := range allowedPaths {
    if strings.HasPrefix(realPath, allowed) {
        return nil  // Safe
    }
}
return errors.New("path outside allowed directories")
```

**3. Environment Variable Validation**
```go
// BEFORE expansion
ValidateSafeEnvExpansion("config", "$HOME/.engram/config.yaml", allowedPaths)

// THEN expand
expanded := os.ExpandEnv(path)
```

**4. Input Length Limits**
```go
const (
    MaxQueryLength  = 10000  // 10KB query text
    MaxPathLength   = 4096   // Standard path max
    MaxConfigSize   = 1MB    // Config file size
)
```

**5. Format Validation**
```go
// Enum validation
ValidateOutputFormat(format, "table", "json")  // Only these allowed

// Tier validation
ValidateTier(tier)  // Only: user, team, company, core, all
```

**Allowed Paths**:

```go
func GetAllowedPaths() ([]string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return nil, err
    }

    return []string{
        filepath.Join(home, ".engram"),  // Engram workspace
        home,                             // Home directory (restrictive)
        "/tmp/engram-",                  // Test directories
    }, nil
}
```

**Attack Mitigation**:

**Path Traversal**:
```bash
# Attack attempt
engram retrieve --path "../../etc/passwd"

# Validation catches it
ValidateSafePath() -> Error: path outside allowed directories
```

**Environment Variable Injection**:
```bash
# Attack attempt
ENGRAM_HOME="${HOME}/../../../etc" engram init

# Validation catches it
ValidateSafeEnvExpansion() -> Error: path outside allowed directories
```

**Command Injection**:
```bash
# Attack attempt
engram retrieve "query; rm -rf /"

# Not vulnerable: query is API parameter, not shell command
# But still validated for length and content
```

**Resource Exhaustion**:
```bash
# Attack attempt
engram retrieve --limit 999999

# Validation catches it
ValidateRangeInt("limit", 999999, 1, 100) -> Error: must be between 1 and 100
```

**Rationale**:

1. **Defense in Depth**: Multiple validation layers
2. **Fail Secure**: Default deny, explicit allow
3. **Canonical Paths**: Resolve symlinks before validation
4. **Early Validation**: Validate at command entry, not service layer
5. **Clear Errors**: Security errors provide user guidance without leaking info

**Alternatives Considered**:

1. **Blacklist patterns**: Easily bypassed, incomplete
2. **Regex validation**: Complex, error-prone
3. **Sandboxing**: Operational overhead, may limit functionality
4. **No validation**: Unacceptable security risk

**Consequences**:

**Positive**:
- Prevents path traversal attacks
- Prevents command injection
- Prevents resource exhaustion
- Clear security boundaries
- User-friendly error messages
- Easy to test (security test suite)

**Negative**:
- Restrictive (by design)
- Requires explicit allowlisting for new paths
- May block legitimate edge cases (can be expanded)

**Implementation Guidelines**:

1. **Validate at entry**: In command RunE, before calling services
2. **Use validators**: Never inline validation logic
3. **Canonical first**: Always resolve symlinks/absolute path
4. **Early return**: Fail fast on validation errors
5. **Test attacks**: Security tests for each validator

**Testing Strategy**:

```go
// Test path traversal
func TestValidateSafePath_PathTraversal(t *testing.T) {
    allowed := []string{"~/.engram"}

    tests := []string{
        "../../etc/passwd",
        "/etc/passwd",
        "~/.engram/../../etc/passwd",
        "~/.engram/../.ssh/id_rsa",
    }

    for _, path := range tests {
        err := cli.ValidateSafePath("path", path, allowed)
        assert.Error(t, err, "should reject: %s", path)
    }
}
```

**Security Audit Checklist**:

- [ ] All file paths validated via ValidateSafePath
- [ ] All env expansions validated via ValidateSafeEnvExpansion
- [ ] All user inputs have length limits
- [ ] All enum inputs validated against whitelist
- [ ] All numeric inputs validated against ranges
- [ ] No raw os.ExpandEnv without prior validation
- [ ] No filepath.Join without validation
- [ ] No exec.Command with user input

**Related Decisions**:
- ADR-002: Structured Error Handling
- ADR-005: Hierarchical Workspace Structure

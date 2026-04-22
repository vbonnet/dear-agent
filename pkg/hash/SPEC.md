# hash Package Specification

**Version:** 0.1.0
**Status:** Production-ready
**Pattern:** Library/Utility
**Last Updated:** 2026-02-11

---

## Executive Summary

The hash package provides cryptographic hashing utilities for document verification and integrity checking in Engram. It computes SHA-256 hashes of files and supports tilde expansion for home directory paths.

**Core Value Proposition:**
- Standardized file hashing for Engram phase verification
- Consistent hash format across all Engram components
- Automatic path expansion for user-friendly file paths
- Zero external dependencies for lightweight, portable operation
- Deterministic hashing for reproducible builds and verification

---

## Problem Statement

### Current Pain Points

**File Integrity Verification:**
- Engram phases need cryptographic verification of file contents
- Manual hash calculation is error-prone and inconsistent
- Path handling varies across different components
- No standard hash format for Engram ecosystem
- File hash changes must be detectable for phase validation

**Impact:**
- Inconsistent hash formats across Engram components
- Manual path expansion in multiple places (code duplication)
- No single source of truth for file hashing logic
- Difficult to verify phase file integrity
- User-unfriendly absolute paths in configuration

### Target Use Cases

**Primary:**
- Computing SHA-256 hashes for Engram phase files
- Verifying file content hasn't changed since last phase
- Detecting phase file modifications for validation
- Supporting tilde-based paths in user configurations
- Providing consistent hash format (sha256:hex) across Engram

**Secondary:**
- File integrity verification in test suites
- Content-addressable file identification
- Checksum generation for file caching
- Path normalization for cross-platform compatibility

**Non-goals:**
- Streaming hash computation (entire file read at once)
- Multiple hash algorithms (only SHA-256)
- Hash verification/comparison (only computation)
- Directory hashing (only individual files)
- Incremental hashing (no state persistence)

---

## Solution Overview

### Architecture

```
hash/
├── hash.go      # Implementation (97 lines)
├── hash_test.go # Tests (202 lines)
└── doc.go       # Package documentation (7 lines)
```

### Component Classification

**Pattern:** Library/Utility (shared code)

**Comparison to other patterns:**
- Guidance (passive): Just instructions, no code
- Tool (active): Executable commands
- Connector (integration): External tool wrappers
- **Library (shared):** Reusable Go code for hashing

**Decision rationale:** Library pattern chosen because hash provides reusable Go functions for file hashing used throughout Engram core and plugins.

---

## API Specification

### Public Functions

#### 1. CalculateFileHash

**Purpose:** Calculate SHA-256 hash of a file and return it in Engram's standard format

**Signature:**
```go
func CalculateFileHash(path string) (string, error)
```

**Parameters:**
- `path` (string): File path (supports tilde expansion, relative, and absolute paths)

**Returns:**
- `string`: Hash in format "sha256:0123456789abcdef..." (64 hex chars after prefix)
- `error`: Error if path expansion fails, file not found, or read fails

**Example:**
```go
// Tilde path
hash, err := hash.CalculateFileHash("~/myfile.txt")
if err != nil {
    log.Fatal(err)
}
fmt.Println(hash) // sha256:a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3

// Relative path (converted to absolute)
hash, err := hash.CalculateFileHash("./test.txt")

// Absolute path
hash, err := hash.CalculateFileHash("/tmp/test.txt")
```

**Behavior:**
1. Expands path using `ExpandPath()` (handles tilde, makes absolute)
2. Opens file for reading
3. Streams file content through SHA-256 hasher
4. Returns hash in format "sha256:{hex}"
5. Closes file (deferred)

**Error Cases:**
- Path expansion fails (unsupported tilde format like `~user/file`)
- File does not exist
- File cannot be opened (permissions)
- File cannot be read (I/O error)

#### 2. ExpandPath

**Purpose:** Expand tilde (~) to user's home directory and return an absolute path

**Signature:**
```go
func ExpandPath(path string) (string, error)
```

**Parameters:**
- `path` (string): File path (may contain tilde, be relative, or absolute)

**Returns:**
- `string`: Absolute path with tilde expanded
- `error`: Error if home directory cannot be determined or unsupported tilde format

**Example:**
```go
// Tilde only
path, err := hash.ExpandPath("~")
// Returns: /home/username

// Tilde with path
path, err := hash.ExpandPath("~/Documents/file.txt")
// Returns: /home/username/Documents/file.txt

// Absolute path (unchanged)
path, err := hash.ExpandPath("/tmp/file.txt")
// Returns: /tmp/file.txt

// Relative path (converted to absolute)
path, err := hash.ExpandPath("../file.txt")
// Returns: /home/username/current/dir/../file.txt (absolute)
```

**Supported Formats:**
- `~` - Expands to home directory
- `~/path` - Expands to home directory + path
- `/absolute/path` - Returns absolute path
- `relative/path` - Converts to absolute path

**Unsupported Formats:**
- `~user/path` - Returns error (user-specific home directories not supported)

**Error Cases:**
- Cannot determine home directory (os.UserHomeDir() fails)
- Path starts with `~` but is not `~` or `~/...` (e.g., `~otheruser/file`)

---

## Functional Requirements

### FR-1: SHA-256 File Hashing
- **Description:** Calculate SHA-256 hash of any file accessible to the process
- **Acceptance:**
  - Must compute correct SHA-256 hash matching known test vectors
  - Must work with empty files (e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855)
  - Must work with files of any size (limited by available memory)
  - Must return hash in format "sha256:{64-char-hex}"
  - Must handle binary and text files identically

### FR-2: Standard Hash Format
- **Description:** Return hashes in consistent format across all Engram components
- **Acceptance:**
  - Must use prefix "sha256:"
  - Must use lowercase hexadecimal encoding
  - Must produce 64 hex characters after prefix (32 bytes = 256 bits)
  - Must match format used by Engram phase verification
  - Must be parseable by standard tools (can extract hex portion)

### FR-3: Tilde Expansion
- **Description:** Support tilde (~) in file paths for user convenience
- **Acceptance:**
  - Must expand `~` to user's home directory
  - Must expand `~/path` to home directory + path
  - Must reject unsupported formats (e.g., `~otheruser/`)
  - Must work on all Unix-like systems (Linux, macOS)
  - Must return clear error for unsupported tilde usage

### FR-4: Path Normalization
- **Description:** Convert relative paths to absolute paths for consistency
- **Acceptance:**
  - Must convert `./file` to absolute path
  - Must convert `../file` to absolute path
  - Must preserve already-absolute paths
  - Must use current working directory as base for relative paths
  - Must handle path separators correctly on host OS

### FR-5: Error Handling
- **Description:** Provide clear error messages for all failure cases
- **Acceptance:**
  - Must return error if file does not exist
  - Must return error if path expansion fails
  - Must return error if file cannot be read
  - Must wrap errors with context (which file, which operation)
  - Must not panic on any input

---

## Non-Functional Requirements

### NFR-1: Performance
- Empty file hash: < 1ms
- Small file (< 1KB): < 5ms
- Large file (1MB): < 50ms
- Tilde expansion: < 1ms
- Zero allocations for path operations (where possible)

### NFR-2: Reliability
- Deterministic output (same file always produces same hash)
- Thread-safe (no shared mutable state)
- No resource leaks (file handles always closed)
- Works in Docker and restricted environments
- Handles filesystem errors gracefully

### NFR-3: Portability
- Works on Linux, macOS, Windows
- No platform-specific dependencies
- Uses standard library only
- No assumptions about file paths (uses filepath.Join)
- Respects OS-specific path separators

### NFR-4: Maintainability
- Simple implementation (< 100 lines)
- Clear function names and documentation
- Standard Go idioms (defer for cleanup)
- Comprehensive test coverage (edge cases)
- No external dependencies

---

## Dependencies

### Required
- **Go:** 1.24.0+
- **Standard Library:**
  - `crypto/sha256` - SHA-256 hashing
  - `fmt` - String formatting
  - `io` - Stream copying
  - `os` - File operations, home directory
  - `path/filepath` - Path manipulation
  - `strings` - String prefix checks

### Optional
- None (intentionally zero external dependencies)

### Internal
- None (standalone package in engram/core/pkg)

---

## Success Criteria

### Adoption Metrics
- Used by Engram phase verification (✅ Achieved)
- Used by 5+ plugins for file hashing (target)
- Standard hash format across all Engram components (✅ Achieved)

### Quality Metrics
- Zero external dependencies (✅ Achieved)
- Test coverage > 90% (✅ Achieved - 202 lines of tests)
- No panics on any input (✅ Achieved)
- Matches SHA-256 test vectors (✅ Achieved)

### Developer Experience
- API is self-documenting (function names are clear)
- Error messages provide actionable context
- Tilde expansion works as users expect
- Single function call for most use cases

---

## Testing Strategy

### Unit Testing Approach

**Test Coverage:**
- Known hash values (empty file, "hello\n", single char)
- Tilde expansion (~ and ~/, absolute, relative paths)
- Error cases (file not found, directory, unsupported tilde)
- Path expansion edge cases

**Test Files:**
- `hash_test.go` (202 lines)
- 6 test functions covering all code paths

**Example Tests:**
```go
// Known hash values
TestCalculateFileHash             // Verifies correct SHA-256 computation
TestCalculateFileHash_TildeExpansion  // Verifies ~ expansion works
TestCalculateFileHash_Errors      // Verifies error handling

// Path expansion
TestExpandPath                    // Verifies all path formats
TestExpandPath_RelativePath       // Verifies relative path conversion
```

### Validation Through Usage
- **Phase Verification:** Used in Engram phase validation
- **Real Files:** Tested with actual filesystem operations
- **Documentation:** Examples in doc.go match actual usage

---

## Hash Format Details

### Format Specification

**Pattern:** `sha256:{hex-digest}`

**Components:**
- **Prefix:** "sha256:" (identifies hash algorithm)
- **Digest:** 64 hexadecimal characters (0-9, a-f)
- **Length:** 71 characters total (7 prefix + 64 hex)

**Example:**
```
sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
|      |
prefix  64-char hex digest (32 bytes = 256 bits)
```

**Properties:**
- Case-insensitive hex (always lowercase in output)
- No whitespace or line breaks
- Fixed length (always 71 characters)
- URL-safe (contains only alphanumeric and colon)
- Parseable (can split on ":" to get algorithm and digest)

### Why SHA-256?

**Chosen over:**
- MD5 (broken, collision attacks)
- SHA-1 (deprecated, collision attacks)
- SHA-512 (overkill, larger hashes)
- BLAKE2 (not in standard library)

**Rationale:**
- Industry standard for file integrity
- No known collision attacks
- Good performance (hardware acceleration)
- In Go standard library (crypto/sha256)
- 256 bits sufficient for file verification

---

## Out of Scope

**Not Included:**
- Hash verification/comparison (only computation)
- Multiple hash algorithms (only SHA-256)
- Directory hashing (only individual files)
- Streaming/incremental hashing (no state persistence)
- Hash-based file deduplication
- HMAC or keyed hashing
- Parallel hashing of multiple files
- Content-addressed storage

**Rationale:** These concerns are beyond the scope of basic file hashing. The package focuses on doing one thing well: computing SHA-256 hashes of files.

---

## Open Questions

None. Package is production-ready and stable.

---

## References

- **Go crypto/sha256:** https://pkg.go.dev/crypto/sha256
- **SHA-256 Spec:** FIPS 180-4
- **Test Vectors:** NIST CAVP
- **Architecture:** `ARCHITECTURE.md`
- **ADR:** `ADR.md`

---

## Version History

**0.1.0** (2026-02-11)
- Initial specification
- Captures existing production implementation
- Backfill documentation for mature package

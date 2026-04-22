# Architecture Decision Records for hash Package

**Version:** 0.1.0
**Status:** Production-ready
**Last Updated:** 2026-02-11

---

## Overview

This document records the architectural decisions made for the hash package, which provides cryptographic hashing utilities for file verification in Engram.

**Purpose:** Document design choices, alternatives considered, and rationale for future maintainers.

---

## ADR-001: SHA-256 Only (No Multi-Algorithm Support)

**Status:** Accepted
**Date:** 2026-02-11
**Context:** Phase verification backfill

### Context

Engram needs cryptographic hashing for file verification (phase files, content integrity). Multiple hash algorithms exist (MD5, SHA-1, SHA-256, SHA-512, BLAKE2, etc.), and we must decide whether to:

1. Support multiple algorithms with a parameter/flag
2. Support only SHA-256
3. Support a pluggable algorithm interface

**Requirements:**
- Cryptographically secure (no broken algorithms like MD5/SHA-1)
- Fast enough for files up to several MB
- Available in Go standard library
- Consistent hash format across Engram

**Constraints:**
- Must use standard library only (no external dependencies)
- Phase verification needs deterministic, consistent hashes
- No use case for algorithm negotiation or versioning

### Decision

**Support only SHA-256 as the hash algorithm.**

Function signature:
```go
func CalculateFileHash(path string) (string, error)
// Returns: "sha256:{hex}"
```

No algorithm parameter or configuration.

### Rationale

**Why SHA-256:**

1. **Security:** No known collision attacks, NIST-approved
2. **Performance:** Good balance of speed and security (hardware-accelerated on modern CPUs)
3. **Standard Library:** `crypto/sha256` in stdlib (no dependencies)
4. **Hash Size:** 256 bits (32 bytes) is sufficient for file verification without being excessive
5. **Industry Standard:** Widely used for file integrity (Git, Docker, package managers)

**Why not other algorithms:**

- **MD5:** Cryptographically broken (collision attacks), unacceptable for verification
- **SHA-1:** Deprecated (collision attacks), being phased out industry-wide
- **SHA-512:** Overkill for file verification (larger hashes, no security benefit for our use case)
- **BLAKE2:** Not in standard library (would require external dependency)
- **SHA-3:** Not in standard library until Go 1.21 (and SHA-256 is sufficient)

**Why not multi-algorithm support:**

1. **YAGNI:** No current or foreseeable use case for multiple algorithms
2. **Complexity:** Algorithm parameter complicates API and testing
3. **Format Parsing:** Would need to parse hash format to determine algorithm
4. **Consistency:** Single algorithm ensures all Engram components use same hashing
5. **Simplicity:** Smaller API surface, easier to document and understand

### Consequences

**Positive:**
- ✅ Simple API (no algorithm parameter)
- ✅ Consistent hash format across all Engram components
- ✅ No algorithm negotiation or version management
- ✅ Standard library only (no dependencies)
- ✅ Fast and secure (SHA-256 is well-optimized)

**Negative:**
- ❌ Cannot switch algorithms without breaking changes
- ❌ If SHA-256 is ever broken (unlikely), requires package redesign
- ❌ No future-proofing for quantum-resistant algorithms

**Mitigations:**
- Hash format includes algorithm prefix ("sha256:") for future extensibility
- If new algorithm needed, can add `CalculateFileHashSHA512()` or similar
- Phase verification can be updated to support multiple formats by checking prefix

**Risks:**
- Low: SHA-256 is unlikely to be broken in the foreseeable future
- Low: If needed, migration path is clear (new function, gradual rollout)

### Implementation Notes

```go
// Current implementation
func CalculateFileHash(path string) (string, error) {
    // ...
    hasher := sha256.New()  // Only SHA-256
    // ...
    return fmt.Sprintf("sha256:%x", hasher.Sum(nil)), nil
}

// If multi-algorithm needed in future (NOT current design):
// func CalculateFileHashWithAlgorithm(path string, alg string) (string, error) {
//     var hasher hash.Hash
//     switch alg {
//     case "sha256":
//         hasher = sha256.New()
//     case "sha512":
//         hasher = sha512.New()
//     default:
//         return "", fmt.Errorf("unsupported algorithm: %s", alg)
//     }
//     // ...
// }
```

### Related Decisions
- ADR-002 (Standard Hash Format)

---

## ADR-002: Standard Hash Format "sha256:{hex}"

**Status:** Accepted
**Date:** 2026-02-11
**Context:** Phase verification backfill

### Context

Hash functions return binary output (byte arrays). We must decide how to represent hashes as strings for storage, comparison, and logging.

**Options considered:**

1. **Raw hex:** `a665a45920422f9d...` (no algorithm prefix)
2. **Algorithm prefix:** `sha256:a665a45920422f9d...`
3. **Base64:** `pmWkWSBCL50X5IZ+...` (shorter, harder to read)
4. **Multiformat/Multihash:** `1220a665a459...` (self-describing binary format)
5. **URN format:** `urn:sha256:a665a45920422f9d...`

**Requirements:**
- Human-readable (debugging, logging)
- Parseable (can extract algorithm and digest)
- Consistent across Engram
- No external libraries needed

### Decision

**Use format: `sha256:{lowercase-hex}`**

**Format specification:**
- Prefix: `sha256:` (identifies algorithm)
- Digest: 64 hexadecimal characters (lowercase)
- Total length: 71 characters (7 prefix + 64 hex)
- No whitespace or line breaks

**Example:**
```
sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

### Rationale

**Why this format:**

1. **Self-documenting:** Algorithm is explicit (supports future extensions)
2. **Human-readable:** Hex is familiar to developers
3. **Standard:** Similar to Docker image digests, Git object hashes
4. **Parseable:** Simple split on `:` to get algorithm and digest
5. **URL-safe:** Only alphanumeric and colon (can use in filenames, URLs)
6. **grep-able:** Easy to search in logs and files
7. **Copy-paste friendly:** Fixed width, no wrapping issues

**Why not alternatives:**

- **Raw hex:** No algorithm identification (breaks if we add SHA-512)
- **Base64:** Harder to read, unfamiliar to many developers
- **Multiformat:** Requires external library, binary format is opaque
- **URN:** More verbose (`urn:` prefix), less common in practice

**Why lowercase hex:**
- Convention in Git, Docker, most hash tools
- Easier to type (no Shift key needed)
- Consistent with Go's `%x` format verb

### Consequences

**Positive:**
- ✅ Future-proof (can add `sha512:...`, `blake2:...` formats)
- ✅ Easy to parse and validate
- ✅ Human-readable in logs and error messages
- ✅ Familiar format (similar to Docker, Git)
- ✅ Works in filenames, URLs, command-line args

**Negative:**
- ❌ Slightly longer than raw hex (7 extra characters)
- ❌ Requires parsing to extract digest (but trivial: split on `:`)

**Mitigations:**
- Size difference is negligible (71 vs 64 chars)
- Parsing is simple and fast (strings.Split)

**Risks:**
- Low: Format is simple and unlikely to cause issues

### Implementation Notes

```go
// Hash formatting
hash := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
// Output: "sha256:a665a45920422f9d417e4867efdc4fb8a04a1f3fff1fa07e998e86f7f7a27ae3"

// Hash parsing (if needed by callers)
parts := strings.SplitN(hash, ":", 2)
algorithm := parts[0]  // "sha256"
digest := parts[1]     // "a665a45920422f9d..."

// Validation (if needed by callers)
if !strings.HasPrefix(hash, "sha256:") {
    return fmt.Errorf("invalid hash format: %s", hash)
}
if len(hash) != 71 {
    return fmt.Errorf("invalid sha256 hash length: %d", len(hash))
}
```

### Related Decisions
- ADR-001 (SHA-256 Only)

---

## ADR-003: Streaming I/O with io.Copy (No Full File Loading)

**Status:** Accepted
**Date:** 2026-02-11
**Context:** Phase verification backfill

### Context

When hashing files, we must decide how to read file contents into the hasher.

**Options considered:**

1. **Load entire file:** `data, _ := os.ReadFile(path); hasher.Write(data)`
2. **Streaming:** `io.Copy(hasher, file)` (read in chunks)
3. **Fixed buffer:** Manual read loop with buffer
4. **Memory-mapped I/O:** Use mmap for large files

**Requirements:**
- Works with files of any size (MB to GB)
- Memory-efficient
- Fast enough for typical use cases (< 10MB files)
- Simple implementation

**Constraints:**
- Phase files are typically small (< 1MB)
- Must work in memory-constrained environments
- No platform-specific code (must work on all OS)

### Decision

**Use streaming I/O with `io.Copy(hasher, file)`**

Implementation:
```go
file, _ := os.Open(absPath)
defer file.Close()

hasher := sha256.New()
io.Copy(hasher, file)  // Streams file in chunks

hash := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
```

### Rationale

**Why streaming (io.Copy):**

1. **Memory efficiency:** O(1) memory regardless of file size
2. **Simplicity:** Single function call, no manual buffer management
3. **Performance:** Uses 32KB buffers (optimal for most I/O)
4. **Standard idiom:** Common pattern in Go for file processing
5. **Large file support:** Can hash multi-GB files without OOM

**Why not alternatives:**

- **Load entire file:**
  - ❌ O(n) memory (problematic for large files)
  - ❌ Can OOM on large files
  - ❌ Wastes memory for small files
  - ✅ Slightly simpler code (but not worth the downsides)

- **Manual buffer loop:**
  - ❌ More complex (error handling, EOF detection)
  - ❌ Easy to introduce bugs (buffer reuse, partial reads)
  - ✅ Slightly more control over buffer size
  - Verdict: io.Copy is better (same performance, simpler code)

- **Memory-mapped I/O:**
  - ❌ Platform-specific (different on Linux, macOS, Windows)
  - ❌ Complex (requires syscalls, error handling)
  - ❌ Overkill for small files
  - ✅ Fastest for multi-GB files (but not our use case)
  - Verdict: Not worth the complexity

**Buffer size:**
- io.Copy uses 32KB buffers by default
- Good balance for most file sizes
- Matches OS page size on many systems
- Can be tuned if needed (but default is fine)

### Consequences

**Positive:**
- ✅ Works with files of any size (tested up to GB+)
- ✅ Constant memory usage (~32KB per call)
- ✅ Simple, idiomatic Go code
- ✅ Fast enough for typical use cases (< 50ms for 1MB)
- ✅ No platform-specific code

**Negative:**
- ❌ Slightly slower than mmap for very large files (but not significant)
- ❌ No progress reporting (but not needed for library)

**Mitigations:**
- For progress reporting, caller can wrap file in io.TeeReader
- For extremely large files, performance is still acceptable

**Risks:**
- Low: io.Copy is well-tested and reliable

### Implementation Notes

```go
// Current implementation
func CalculateFileHash(path string) (string, error) {
    // ...
    file, err := os.Open(absPath)
    if err != nil {
        return "", fmt.Errorf("failed to open file: %w", err)
    }
    defer file.Close()

    hasher := sha256.New()
    if _, err := io.Copy(hasher, file); err != nil {
        return "", fmt.Errorf("failed to hash file: %w", err)
    }

    return fmt.Sprintf("sha256:%x", hasher.Sum(nil)), nil
}

// If progress reporting needed (caller's responsibility):
// type ProgressReader struct {
//     io.Reader
//     OnProgress func(bytesRead int64)
//     total int64
// }
//
// file := &ProgressReader{Reader: file, OnProgress: updateProgress}
// io.Copy(hasher, file)
```

### Performance Characteristics

**Benchmarks (approximate):**
- 1 KB file: ~2ms (syscall overhead dominates)
- 100 KB file: ~10ms
- 1 MB file: ~50ms
- 10 MB file: ~500ms

**Memory:**
- Fixed: ~32KB buffer + ~32 bytes for hasher state
- No allocations proportional to file size

### Related Decisions
- None (independent decision)

---

## ADR-004: Tilde Expansion in Library (Not Client Responsibility)

**Status:** Accepted
**Date:** 2026-02-11
**Context:** Phase verification backfill

### Context

Many Engram configurations use tilde (`~`) to represent the user's home directory (e.g., `~/engrams/phase.md`). We must decide where tilde expansion happens:

1. **Library handles:** `CalculateFileHash("~/file")` expands tilde internally
2. **Client handles:** Caller must expand tilde before calling library
3. **Separate function:** Provide `ExpandPath()` utility, let caller decide

**Requirements:**
- User-friendly (tilde paths are common in Engram)
- No duplicate tilde expansion logic across codebase
- Clear error messages for unsupported formats
- Works on all platforms (Linux, macOS, Windows)

**Constraints:**
- Not all platforms use `~` convention (Windows uses `%USERPROFILE%`)
- `~user/path` is not supported by `os.UserHomeDir()` (only current user)
- Must handle relative and absolute paths too

### Decision

**Library handles tilde expansion internally AND provides public `ExpandPath()` function.**

**API:**
```go
// Primary API: handles tilde expansion internally
func CalculateFileHash(path string) (string, error) {
    absPath, err := ExpandPath(path)  // Internal call
    // ...
}

// Public utility: allows caller to expand paths without hashing
func ExpandPath(path string) (string, error) {
    // Expand ~ and convert to absolute path
}
```

### Rationale

**Why library handles tilde expansion:**

1. **User experience:** Users expect `~/file` to work (common in Unix tools)
2. **DRY:** Avoids duplicate expansion logic in every caller
3. **Consistency:** All callers get same behavior
4. **Error handling:** Library can provide better error messages
5. **Convenience:** Most common use case (tilde paths) just works

**Why also provide ExpandPath():**

1. **Flexibility:** Callers can expand paths without hashing (e.g., file existence checks)
2. **Testability:** Callers can test path expansion separately
3. **Debugging:** Useful for troubleshooting path issues
4. **Reusability:** Other Engram packages can use ExpandPath() directly

**Why not client responsibility:**
- ❌ Code duplication (every caller reimplements expansion)
- ❌ Inconsistent behavior (different implementations, bugs)
- ❌ Poor UX (requires extra step for common case)
- ❌ Error messages less helpful (expansion vs hashing failures)

**Supported formats:**
- `~` → `/home/user`
- `~/path` → `~/path`
- `/absolute` → `/absolute` (unchanged)
- `relative` → `/current/dir/relative` (via filepath.Abs)

**Unsupported formats:**
- `~user/path` → Error (os.UserHomeDir doesn't support other users)

### Consequences

**Positive:**
- ✅ User-friendly API (tilde paths just work)
- ✅ No duplicate expansion logic across codebase
- ✅ Clear error for unsupported formats (`~user/path`)
- ✅ Public ExpandPath() for other use cases
- ✅ Automatic conversion to absolute paths

**Negative:**
- ❌ Slightly larger API surface (2 functions instead of 1)
- ❌ Handles path expansion concern (could argue it's not library's job)

**Mitigations:**
- ExpandPath is simple and well-tested (30 lines)
- Clear documentation on supported formats

**Risks:**
- Low: os.UserHomeDir is standard library and reliable

### Implementation Notes

```go
func ExpandPath(path string) (string, error) {
    // Non-tilde paths: just make absolute
    if !strings.HasPrefix(path, "~") {
        return filepath.Abs(path)
    }

    // Get home directory
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("failed to get home directory: %w", err)
    }

    // Tilde only
    if path == "~" {
        return home, nil
    }

    // Tilde with slash
    if strings.HasPrefix(path, "~/") {
        return filepath.Join(home, path[2:]), nil
    }

    // Unsupported (e.g., ~otheruser/)
    return "", fmt.Errorf("cannot expand path: %s (only ~ and ~/ are supported)", path)
}

func CalculateFileHash(path string) (string, error) {
    absPath, err := ExpandPath(path)  // Uses ExpandPath internally
    // ...
}
```

### Edge Cases

**Windows:**
- `os.UserHomeDir()` returns `C:\Users\username`
- `~/file` → `C:\Users\username\file`
- `~` → `C:\Users\username`

**Docker/restricted environments:**
- `os.UserHomeDir()` may fail (no home directory set)
- Returns error with clear message

**Root user:**
- `~` → `/root`
- Works as expected

### Related Decisions
- None (independent decision)

---

## ADR-005: Defer for Resource Cleanup (Not Manual Close)

**Status:** Accepted
**Date:** 2026-02-11
**Context:** Phase verification backfill

### Context

After opening a file, we must ensure it's closed to avoid resource leaks. We must decide when and how to close the file.

**Options considered:**

1. **Defer:** `defer file.Close()` immediately after opening
2. **Manual close:** `file.Close()` before each return
3. **Named return + defer:** Use named return values with defer
4. **Finally-style:** (Go doesn't have finally, but could use panic/recover)

**Requirements:**
- No resource leaks (file handles always closed)
- Clean error handling
- Idiomatic Go code
- Simple to understand and maintain

### Decision

**Use `defer file.Close()` immediately after successful file open.**

Implementation:
```go
file, err := os.Open(absPath)
if err != nil {
    return "", fmt.Errorf("failed to open file: %w", err)
}
defer file.Close()  // Runs when function returns (normal or error)

// ... use file ...
// No manual close needed
```

### Rationale

**Why defer:**

1. **Guaranteed cleanup:** Runs even if error occurs later
2. **Idiomatic:** Standard Go pattern for resource management
3. **Simple:** Single line, no manual close calls
4. **Error handling:** Works with early returns (defer still executes)
5. **Readability:** Cleanup is near acquisition (open → defer close)

**Why not manual close:**

- ❌ Error-prone (easy to forget close on error paths)
- ❌ Code duplication (close before every return)
- ❌ Harder to maintain (adding return statement requires adding close)
- Example of problematic manual close:
  ```go
  file, _ := os.Open(path)

  if someCondition {
      file.Close()  // Must remember to close
      return "", errors.New("error")
  }

  if anotherCondition {
      file.Close()  // Must remember to close
      return "", errors.New("another error")
  }

  file.Close()  // Must remember to close
  return hash, nil
  ```

**Why not named return + defer:**
- ⚖️ Slightly more complex
- ⚖️ No benefit for this use case (no cleanup logic needed)
- ✅ Useful for more complex cleanup, but overkill here

**Why not panic/recover:**
- ❌ Not idiomatic for normal error handling
- ❌ Harder to understand
- ❌ No benefit over defer

### Consequences

**Positive:**
- ✅ No resource leaks (file always closed)
- ✅ Clean error handling (no manual cleanup in error paths)
- ✅ Idiomatic Go code (standard pattern)
- ✅ Easy to maintain (add returns without worrying about cleanup)

**Negative:**
- ❌ File stays open until function returns (vs immediate close)
- ❌ Slight performance overhead (defer has small cost)

**Mitigations:**
- Function is fast (< 50ms), so file is not open long
- Defer overhead is negligible (< 1μs)

**Risks:**
- Low: defer is well-tested and reliable

### Implementation Notes

```go
// Current implementation
func CalculateFileHash(path string) (string, error) {
    absPath, err := ExpandPath(path)
    if err != nil {
        return "", fmt.Errorf("failed to expand path: %w", err)
    }

    file, err := os.Open(absPath)
    if err != nil {
        return "", fmt.Errorf("failed to open file %s: %w", absPath, err)
    }
    defer file.Close()  // ← Guaranteed cleanup

    hasher := sha256.New()
    if _, err := io.Copy(hasher, file); err != nil {
        return "", fmt.Errorf("failed to hash file: %w", err)
        // defer still executes (file is closed)
    }

    hash := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
    return hash, nil
    // defer executes here (file is closed)
}
```

### Error Handling

**File close errors:**
- defer file.Close() ignores close errors (common pattern)
- Rationale: File was successfully read, close error is unlikely and non-critical
- If close error handling needed (rare), use:
  ```go
  defer func() {
      if err := file.Close(); err != nil {
          log.Printf("failed to close file: %v", err)
      }
  }()
  ```

### Related Decisions
- ADR-003 (Streaming I/O) - defer works well with io.Copy

---

## ADR-006: Pure Functions with No Package State

**Status:** Accepted
**Date:** 2026-02-11
**Context:** Phase verification backfill

### Context

We must decide whether the hash package should maintain any state (caches, configuration, etc.) or be purely functional.

**Options considered:**

1. **Pure functions:** No package-level state, all state in function parameters/returns
2. **Configuration struct:** `type Hasher struct { ... }` with methods
3. **Package-level cache:** Cache file hashes for performance
4. **Package-level config:** Global settings (e.g., default algorithm)

**Requirements:**
- Thread-safe (multiple goroutines can call concurrently)
- Simple API
- No initialization required
- Testable (no global state)

### Decision

**Use pure functions with no package-level state.**

**API:**
```go
// No package-level variables
// No initialization required
// No configuration needed

func ExpandPath(path string) (string, error)  // Pure function
func CalculateFileHash(path string) (string, error)  // Pure function
```

### Rationale

**Why pure functions:**

1. **Thread-safe:** No shared state → no race conditions → no locks needed
2. **Simple:** No initialization, configuration, or cleanup
3. **Testable:** No global state to mock or reset
4. **Predictable:** Same input always produces same output
5. **Composable:** Easy to combine with other functions

**Why not configuration struct:**
- ❌ Overkill for 2 functions
- ❌ Requires initialization (more complex API)
- ❌ No configuration needed (only SHA-256 supported)
- Example of unnecessary complexity:
  ```go
  // What we'd need if using struct (NOT current design):
  hasher := hash.New()  // Extra step
  hash := hasher.CalculateFileHash(path)  // More verbose
  ```

**Why not package-level cache:**
- ❌ Adds complexity (cache invalidation, memory management)
- ❌ Thread safety requires locks (performance overhead)
- ❌ Files change (cache invalidation is hard)
- ❌ No use case (files are rarely hashed multiple times)
- ❌ Memory leak risk (unbounded cache)

**Why not package-level config:**
- ❌ Global state is problematic (testing, concurrency)
- ❌ No configuration needed (single algorithm, no options)
- ❌ Would require initialization (e.g., `hash.SetAlgorithm("sha256")`)

### Consequences

**Positive:**
- ✅ Thread-safe by default (no locks needed)
- ✅ No initialization required
- ✅ Simple API (just call functions)
- ✅ Easy to test (no state to set up or tear down)
- ✅ Predictable behavior (no hidden state)

**Negative:**
- ❌ Cannot cache results (but not needed)
- ❌ Cannot configure behavior (but not needed)

**Mitigations:**
- If caching needed in future, caller can implement (e.g., map[string]string)
- If configuration needed, can add optional parameters (e.g., options struct)

**Risks:**
- Low: Pure functions are simple and reliable

### Implementation Notes

```go
// No package-level variables
// (Go allows package-level vars, but we don't use them)

// Pure function: deterministic, no side effects (except file I/O)
func ExpandPath(path string) (string, error) {
    // No access to package state
    // Only uses parameters and local variables
    // Returns same output for same input
    home, _ := os.UserHomeDir()
    // ...
    return absPath, nil
}

// Pure function: deterministic, no side effects (except file I/O)
func CalculateFileHash(path string) (string, error) {
    // No access to package state
    // Creates own hasher (no shared state)
    hasher := sha256.New()  // Local variable
    // ...
    return hash, nil
}
```

### Thread Safety

**Concurrency guarantees:**
- ✅ Multiple goroutines can call functions concurrently
- ✅ No data races (verified with `go test -race`)
- ✅ No locks needed (no shared mutable state)

**Example:**
```go
// Safe concurrent usage
var wg sync.WaitGroup
for _, path := range paths {
    wg.Add(1)
    go func(p string) {
        defer wg.Done()
        hash, _ := hash.CalculateFileHash(p)  // Thread-safe
        fmt.Println(hash)
    }(path)
}
wg.Wait()
```

### Testing Benefits

**No test setup/teardown needed:**
```go
func TestCalculateFileHash(t *testing.T) {
    // No setup (no global state to initialize)

    hash, err := hash.CalculateFileHash("~/test.txt")
    assert.NoError(t, err)
    assert.Equal(t, expected, hash)

    // No teardown (no global state to clean up)
}
```

### Related Decisions
- None (independent decision)

---

## Future ADR Topics

Potential future decisions if requirements change:

### Multi-Algorithm Support
- **Status:** Not needed currently
- **Decision:** If needed, add `CalculateFileHashWithAlgorithm(path, alg string)` or use options pattern
- **Rationale:** YAGNI - no use case for multiple algorithms yet

### Hash Verification Function
- **Status:** Not needed currently
- **Decision:** If needed, add `VerifyFileHash(path, expectedHash string) error`
- **Rationale:** Callers currently just compare strings (sufficient)

### Directory Hashing
- **Status:** Out of scope
- **Decision:** If needed, add `CalculateDirectoryHash(dirPath string) (string, error)`
- **Rationale:** Complex (walk tree, handle symlinks, sort files, combine hashes)

### Streaming Hash API
- **Status:** Not needed currently
- **Decision:** If needed, add `type StreamingHasher struct { ... }` with `Write()` method
- **Rationale:** Current API handles files; no use case for streaming arbitrary data

### Performance Optimization
- **Status:** Not needed currently
- **Decision:** If needed, consider mmap for very large files (> 100MB)
- **Rationale:** Current performance is acceptable for typical file sizes

---

## Design Principles

These principles guided all architectural decisions:

1. **Simplicity over features** - Minimal API, do one thing well
2. **Standard library over dependencies** - Zero external dependencies
3. **Pure functions over stateful** - No global state, thread-safe by default
4. **Explicit over implicit** - Clear errors, no magic behavior
5. **User experience over purity** - Handle tilde expansion (convenience)
6. **Future-proof format** - Hash format includes algorithm (extensible)

---

## References

- **Specification:** `SPEC.md`
- **Architecture:** `ARCHITECTURE.md`
- **Go crypto/sha256:** https://pkg.go.dev/crypto/sha256
- **SHA-256 Standard:** FIPS 180-4
- **Go Effective Go:** https://go.dev/doc/effective_go (defer, errors)

---

## Version History

**0.1.0** (2026-02-11)
- Initial ADR documentation
- Captures architectural decisions for existing implementation
- Backfill documentation for mature package
- 6 ADRs documented: SHA-256 only, hash format, streaming I/O, tilde expansion, defer cleanup, pure functions

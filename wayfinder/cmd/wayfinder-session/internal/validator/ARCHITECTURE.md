# Wayfinder Validator Architecture

**Version**: 1.0
**Last Updated**: 2026-02-13
**Maintainer**: Engram Core Team

---

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Gate System](#gate-system)
4. [Component Details](#component-details)
5. [Data Flow](#data-flow)
6. [Security Architecture](#security-architecture)
7. [Extension Points](#extension-points)
8. [Deployment](#deployment)

---

## Overview

The Wayfinder Validator is a quality gate system that enforces deliverable standards at Wayfinder phase transitions. It prevents progression through the workflow when deliverables are incomplete, incorrect, or of insufficient quality.

### Key Principles

1. **Fail Fast**: Catch issues at phase boundaries, not at project end
2. **Progressive Validation**: Early phases (D1-D4) validate requirements; later phases (S8-S11) validate implementation
3. **Zero Tolerance**: No exceptions for test failures, no skipping validation
4. **Graceful Degradation**: Handle edge cases without false positives
5. **Performance**: SHA-256 caching minimizes repeated validation overhead

### Design Goals

- **Prevent Scope Creep**: Block phases when deliverables contain forbidden content (e.g., code in D1-D7)
- **Ensure Completeness**: Verify all required sections/fields exist in deliverables
- **Validate Quality**: Check documentation quality, test coverage, code compilation
- **Enforce Hygiene**: Zero-tolerance test hygiene (no failures, no skips)
- **Enable Trust**: "Phase Complete" means work is actually done and validated

---

## System Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Wayfinder CLI                             │
│  (/wayfinder-session next-phase, complete-phase, etc.)      │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       │ Calls CanCompletePhase()
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                 Validator Orchestrator                       │
│  (validator.go - Main entry point)                          │
│  - validatePhaseState()                                     │
│  - validateDeliverableExists()                              │
│  - validateDeliverableSize()                                │
│  - validateFrontmatter()                                    │
│  - validateHash()                                           │
│  - runGateValidations()  ◄─── Delegates to gates           │
└──────────────────────┬──────────────────────────────────────┘
                       │
                       │ Phase-specific validation
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                   Gate Layer                                 │
│  (Multiple gate implementations)                            │
│                                                             │
│  ┌───────────────┐  ┌───────────────┐  ┌────────────────┐ │
│  │ D2 Content    │  │ Doc Quality   │  │ Multi-Persona  │ │
│  │ Gate          │  │ Gate          │  │ Gate           │ │
│  │               │  │               │  │                │ │
│  │ • Overlap %   │  │ • SPEC.md     │  │ • Review API   │ │
│  │ • Methodology │  │ • Caching     │  │ • Vote Agg.    │ │
│  └───────────────┘  └───────────────┘  └────────────────┘ │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐ │
│  │        Gate 9: Working Code Verification              │ │
│  │  (code_verification_gate.go - 632 lines)              │ │
│  │                                                       │ │
│  │  • findCodeFiles() - Discover code files             │ │
│  │  • detectLanguage() - Identify project language      │ │
│  │  • validateFilesExist() - Security + existence       │ │
│  │  • runBuildCommand() - Compilation verification      │ │
│  │  • runTestCommand() - Test hygiene enforcement       │ │
│  │  • validateArtifactsExist() - Build output checks    │ │
│  │  • checkCodeVerificationCache() - SHA-256 caching    │ │
│  │  • updateCodeVerificationCache() - Cache updates     │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                       │
                       │ Returns ValidationError or nil
                       ▼
┌─────────────────────────────────────────────────────────────┐
│                 Error Handling Layer                         │
│  (ValidationError with remediation messages)                │
│  - User-friendly error messages                             │
│  - Actionable remediation steps                             │
│  - Context about what failed and why                        │
└─────────────────────────────────────────────────────────────┘
```

### Component Breakdown

| Component | File | Lines | Purpose |
|-----------|------|-------|---------|
| **Orchestrator** | `validator.go` | ~500 | Main validator logic, phase state management |
| **Gate 9 (Code)** | `code_verification_gate.go` | 632 | Working code verification (builds, tests, artifacts) |
| **D2 Gate** | `validator.go` (validateD2Content) | ~150 | Overlap analysis, search methodology validation |
| **Doc Quality Gate** | `doc_quality_gate.go` | ~400 | SPEC.md quality, caching, hash validation |
| **Multi-Persona Gate** | `multi_persona_gate.go` | ~800 | External review integration, vote aggregation |
| **Frontmatter** | `frontmatter.go` | ~200 | YAML frontmatter parsing, validation |
| **Signature** | `signature.go` | ~150 | Phase deliverable signing, checksum validation |
| **Tests** | `*_test.go` | 1,500+ | Comprehensive unit + integration tests |

---

## Gate System

### Gate Types

Wayfinder has **3 gate types** that execute at different validation tiers:

#### 1. Deliverable Gates (All Phases)

**Purpose**: Validate basic deliverable requirements before content checks

**Checks**:
- File existence (`validateDeliverableExists`)
- File size ≥100 bytes (`validateDeliverableSize`)
- YAML frontmatter valid (`validateFrontmatter`)
- Methodology hash match (`validateHash` - with override support)

**When**: Before phase-specific gate checks

**Example Error**:
```
❌ Validation Failed: Deliverable file too small

D1-problem-validation.md is only 42 bytes (minimum: 100 bytes)

Resolution: Add meaningful content to the deliverable
```

#### 2. Content Gates (Phase-Specific)

**Purpose**: Validate deliverable content for specific phases

**D2 Gate** (Solutions Search):
- Overlap percentage quantified (`extractOverlapPercentage`)
- Search methodology documented (`hasSearchMethodology`)
- Minimum content length (500 chars)

**D4 Gate** (Requirements):
- SPEC.md exists and ≥500 bytes
- Quality checks (structure, completeness)
- SHA-256 caching for repeated validation

**S6 Gate** (Design):
- Lateral thinking: ≥3 distinct approaches
- Tradeoffs documented (Pros/Cons for each)
- Approach distinctness (similarity <70%)

**S8 Gate** (Implementation):
- Red flag detection (no "demonstration", "placeholder", "would implement")
- Code files exist (not just markdown deliverable)
- Git commit status (all deliverables committed)

**Example Error**:
```
❌ D2 Validation Failed: Missing Overlap Percentage

D2-existing-solutions.md must quantify overlap with existing solutions.

Required format: "**Overlap**: 75%"

Resolution: Add overlap analysis to D2 deliverable
```

#### 3. Code Gates (All Code-Bearing Phases)

**Purpose**: Validate actual code deliverables (Gate 9)

**Phases**: W0, S8, S9, S10, S11 (code is allowed in these phases)

**Checks**:
1. **File Existence**: Claimed code files exist on filesystem
2. **Compilation**: Build command exits 0 (5-minute timeout)
3. **Test Hygiene**: Test command exits 0, zero failures/skips (10-minute timeout)
4. **Artifacts**: Build outputs exist (e.g., dist/, target/, binaries)

**Security Features**:
- Path traversal protection (reject `../`)
- File size limits (10MB max per file)
- Command injection prevention (no shell expansion)
- Timeout enforcement (prevents infinite loops)

**Example Error**:
```
❌ Gate 9 Failed: Test Hygiene Violation

Test command failed: go test ./...

Exit code: 1
Output:
  --- FAIL: TestAuth (0.01s)
      auth_test.go:25: Expected true, got false
  --- SKIP: TestSession (0.00s)
      session_test.go:10: Not implemented yet

Resolution (Test Hygiene Gate):
1. Fix code bugs (if test failures expose bugs in implementation)
2. Fix test bugs (if failures are due to bugs in test code)
3. Rewrite tests (if code changed and tests need updating)
4. Delete obsolete tests (if tests are no longer applicable)

Pre-existing failures compound and erode confidence - zero tolerance.

Run: go test ./...
```

### Gate Execution Flow

```
CanCompletePhase(phaseName, projectDir)
  ↓
validatePhaseState() ────► Check phase started, not already complete
  ↓
validateDeliverableExists() ────► Find phase deliverable file (glob pattern)
  ↓
validateDeliverableSize() ────► Ensure file ≥100 bytes
  ↓
validateFrontmatter() ────► Verify YAML frontmatter valid
  ↓
validateHash() ────► Check methodology hash matches (with override)
  ↓
runGateValidations() ────┬─► validateD2Content() [D2 only]
                         ├─► validateDocQuality() [D4 only]
                         ├─► validateCodeDeliverables() [Gate 9, all phases]
                         └─► validateMultiPersona() [D3-D4, S6-S8]
  ↓
[All gates pass] ────► Return nil (phase can complete)
```

**Failure Behavior**:
- Any gate failure returns `ValidationError` immediately
- Error message includes:
  - What failed (specific check)
  - Why it failed (context)
  - How to fix it (remediation steps)
- Phase transition is blocked until issue resolved

---

## Component Details

### Gate 9: Working Code Verification

**File**: `code_verification_gate.go` (632 lines)

**Purpose**: Validate code deliverables before allowing phase transitions

**Architecture**:

```
validateCodeDeliverables(phaseName, projectDir)
  │
  ├─► findCodeFiles(projectDir) ────► Discover *.go, *.py, *.js, etc.
  │    │
  │    └─► [0 files] ────► Skip validation (documentation project)
  │
  ├─► detectLanguage(files) ────► Determine primary language
  │    │                           (Go, Python, JavaScript, Rust, C++)
  │    │
  │    └─► [Unknown] ────► Skip validation (unsupported language)
  │
  ├─► checkCodeVerificationCache() ────► Check if cached result valid
  │    │
  │    ├─► [Cache hit] ────► Return cached result (skip validation)
  │    └─► [Cache miss/expired] ────► Continue validation
  │
  ├─► validateFilesExist(projectDir, files) ───┬─► validatePath() [security]
  │                                              ├─► os.Stat() [existence]
  │                                              └─► Check size <10MB [security]
  │
  ├─► runBuildCommand(projectDir, language) ───┬─► Language-specific build cmd
  │                                              ├─► 5-minute timeout
  │                                              └─► Exit code 0 required
  │
  ├─► runTestCommand(projectDir, language) ────┬─► Language-specific test cmd
  │                                              ├─► 10-minute timeout
  │                                              ├─► Exit code 0 required
  │                                              └─► Zero failures/skips enforced
  │
  ├─► validateArtifactsExist(projectDir, lang) ─► Check build outputs exist
  │
  └─► updateCodeVerificationCache() ────► Cache successful validation
```

**Language Support**:

| Language | Extension | Build Command | Test Command | Artifact Check |
|----------|-----------|---------------|--------------|----------------|
| Go | `.go` | `go build ./...` | `go test ./...` | Simplified (no specific artifacts) |
| Python | `.py` | *(none)* | `pytest` | *(none)* |
| JavaScript | `.js` | `npm run build` | `npm test` | `dist/` directory |
| TypeScript | `.ts` | `npm run build` | `npm test` | `dist/` directory |
| Rust | `.rs` | `cargo build` | `cargo test` | `target/` directory |
| C/C++ | `.c/.cpp` | `make build` | `make test` | Simplified |

**Caching Strategy**:

```
Cache Entry:
{
    "phase": "S8",
    "source_hash": "a1b2c3...",  # SHA-256 of source files
    "test_hash": "d4e5f6...",    # SHA-256 of test files
    "build_passed": true,
    "tests_passed": true,
    "artifacts_passed": true,
    "cached_at": "2026-02-13T12:00:00Z"
}

Cache Location: .wayfinder-cache/code-verification/<phase-name>.json
Cache Expiry: 24 hours
Cache Hit Condition: source_hash + test_hash match AND all checks passed
```

**Performance**:
- **Target**: 50-70% cache hit rate
- **File checks**: <1 second for 100 files (filepath.Walk)
- **Build timeout**: 5 minutes (context.WithTimeout)
- **Test timeout**: 10 minutes (context.WithTimeout)

### Multi-Persona Gate

**File**: `multi_persona_gate.go` (~800 lines)

**Purpose**: External review integration with multi-persona-review CLI

**Architecture**:

```
Tier System:
- Tier 1 (Blocking): S6, S10 - ALL personas must vote GO to pass
- Tier 2 (Advisory): S7 - Majority GO passes with warnings
- Tier 0 (None): Other phases - No multi-persona review

Vote Aggregation:
- GO: Persona approves deliverable
- NO-GO: Persona blocks deliverable (critical issue)
- ABSTAIN: Persona has no opinion (not expert in this area)

Outcomes:
- PASSED: Tier 1 with all GO votes (or Tier 2 with majority GO)
- BLOCKED: Tier 1 with any NO-GO vote
- CONDITIONAL: Tier 1 with all ABSTAIN votes (override required)
- CAUTION: Tier 2 with majority NO-GO (warning only)
```

**Configuration**:

```go
var gateConfigs = map[string]GateConfig{
    "S6": {Tier: 1, Blocking: true},   // Design must pass review
    "S10": {Tier: 1, Blocking: true},  // Deployment must pass review
    "S7": {Tier: 2, Blocking: false},  // Plan review is advisory
}
```

---

## Data Flow

### Phase Completion Flow

```
1. User runs: wayfinder-session complete-phase S8

2. CLI calls: validator.CanCompletePhase("S8", projectDir)

3. Validator orchestrator:
   a. validatePhaseState() ────► Check phase is in_progress
   b. validateDeliverableExists() ────► Find S8-implementation.md
   c. validateDeliverableSize() ────► Ensure ≥100 bytes
   d. validateFrontmatter() ────► Parse YAML, verify fields
   e. validateHash() ────► Check methodology hash (with override)
   f. runGateValidations("S8", projectDir)

4. Gate Layer:
   a. validateCodeDeliverables("S8", projectDir) [Gate 9]
      i. findCodeFiles() ────► Discover *.go files
      ii. detectLanguage() ────► Identify "go"
      iii. checkCodeVerificationCache() ────► Cache miss
      iv. validateFilesExist() ────► All files exist, <10MB
      v. runBuildCommand() ────► go build ./... [PASS]
      vi. runTestCommand() ────► go test ./... [PASS]
      vii. validateArtifactsExist() ────► (Simplified for Go)
      viii. updateCodeVerificationCache() ────► Save result

   b. validateMultiPersona("S8", projectDir)
      i. Call multi-persona-review CLI
      ii. Aggregate votes (Tier 1: all GO required)
      iii. Return PASSED

5. Return: nil (validation passed)

6. CLI updates: WAYFINDER-STATUS.md (phase: completed)

7. User proceeds to: wayfinder-session next-phase
```

### Error Flow

```
1. User runs: wayfinder-session complete-phase S8

2. Validator orchestrator:
   [Steps 1-3 same as above]

3. Gate Layer:
   a. validateCodeDeliverables("S8", projectDir)
      i. findCodeFiles() ────► Discover *.go files
      ii. detectLanguage() ────► Identify "go"
      iii. runTestCommand() ────► go test ./...
          Exit code: 1 (test failures detected)

4. Return: ValidationError{
       Message: "Test command failed: go test ./...",
       Fix: testHygieneRemediation("go test ./..."),
   }

5. CLI displays:
   ❌ Gate 9 Failed: Working Code Verification

   Test command failed: go test ./...

   [Full error output with remediation]

6. Phase remains: in_progress (not completed)

7. User must: Fix tests, re-run complete-phase
```

---

## Security Architecture

### Threat Model

**Untrusted Inputs**:
1. File paths (from bead outcomes, user-provided)
2. Command execution (build/test commands)
3. File system operations (reading code files, writing cache)

**Attack Vectors**:
1. **Path Traversal**: `../../../etc/passwd` in file paths
2. **Command Injection**: Shell expansion in build/test commands
3. **Denial of Service**: Extremely large files, infinite loops
4. **Cache Poisoning**: Malicious cache entries

### Security Layers

#### Layer 1: Path Traversal Protection

```go
func validatePath(projectDir, path string) error {
    // Reject any path containing ".."
    if strings.Contains(path, "..") {
        return ValidationError{
            Message: "path traversal detected",
            Fix: "Use absolute or relative paths within project directory only",
        }
    }

    // Additional normalization and checks
    cleanPath := filepath.Clean(path)
    if !strings.HasPrefix(cleanPath, projectDir) {
        return ValidationError{
            Message: "path outside project directory",
        }
    }

    return nil
}
```

**Blocked Examples**:
- `../../../etc/passwd`
- `~/../root/secrets.txt`
- `src/../../etc/shadow`

**Allowed Examples**:
- `src/main.go`
- `~/project/src/main.go`
- `./main.go`

#### Layer 2: File Size Limits

```go
const maxCodeFileSizeBytes = 10 * 1024 * 1024  // 10 MB

func validateFilesExist(projectDir string, filePaths []string) error {
    for _, path := range filePaths {
        info, err := os.Stat(fullPath)
        if err != nil {
            return ValidationError{Message: "file does not exist: " + path}
        }

        if info.Size() > maxCodeFileSizeBytes {
            return ValidationError{
                Message: "file too large: " + path,
                Fix: "Files must be <10MB (prevents DoS attacks)",
            }
        }
    }
    return nil
}
```

**Rationale**:
- Prevents DoS via extremely large files
- Catches mistakenly committed binaries
- 10MB limit is generous for source code files

#### Layer 3: Command Injection Prevention

```go
// SAFE: Fixed arguments, no shell expansion
cmd := exec.Command("go", "build", "./...")

// UNSAFE (not used): Shell expansion allows injection
// cmd := exec.Command("sh", "-c", "go build " + userInput)
```

**Implementation**:
- All commands use `exec.Command()` with fixed argument array
- No shell (`sh -c`) execution
- No string concatenation for commands
- Arguments are never interpolated from user input

**Blocked Attack**:
```bash
# If we used shell expansion (WE DON'T):
userInput = "./... && rm -rf /"
exec.Command("sh", "-c", "go build " + userInput)  # VULNERABLE

# Our implementation (SAFE):
exec.Command("go", "build", "./...")  # Fixed arguments
```

#### Layer 4: Timeout Enforcement

```go
func runBuildCommand(projectDir, language string) error {
    // 5-minute timeout for build
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    cmd := exec.CommandContext(ctx, buildCmd, buildArgs...)
    err := cmd.Run()

    if ctx.Err() == context.DeadlineExceeded {
        return ValidationError{
            Message: "Build timeout (5 minutes)",
            Fix: "Optimize build time or increase timeout",
        }
    }

    return checkExitCode(cmd, err)
}
```

**Timeouts**:
- Build: 5 minutes
- Tests: 10 minutes

**Rationale**:
- Prevents infinite loops
- Catches hung processes
- Reasonable for typical projects
- Configurable in V2

---

## Extension Points

### Adding New Gates

**Steps**:
1. Create new file: `<gate_name>_gate.go`
2. Implement validation function: `func validate<GateName>(phaseName, projectDir string) error`
3. Add call in `runGateValidations()` for target phases
4. Write tests: `<gate_name>_gate_test.go`
5. Document in SPEC.md and ARCHITECTURE.md

**Example (hypothetical S9 integration gate)**:

```go
// s9_integration_gate.go

func validateIntegration(phaseName, projectDir string) error {
    if phaseName != "S9" {
        return nil  // Only validate S9 phase
    }

    // Check integration tests exist
    integrationTests, err := findIntegrationTests(projectDir)
    if err != nil {
        return err
    }

    if len(integrationTests) == 0 {
        return ValidationError{
            Message: "No integration tests found",
            Fix: "Add integration tests to tests/integration/ directory",
        }
    }

    // Run integration tests
    output, err := runIntegrationTests(projectDir, integrationTests)
    if err != nil {
        return ValidationError{
            Message: "Integration tests failed",
            Fix: formatTestOutput(output),
        }
    }

    return nil
}
```

**Integration**:

```go
// validator.go - runGateValidations()

func (v *Validator) runGateValidations(phaseName, projectDir string) error {
    // ... existing gates ...

    // S9 Integration gate
    if err := validateIntegration(phaseName, projectDir); err != nil {
        return err
    }

    return nil
}
```

### Adding Language Support (Gate 9)

**Steps**:
1. Add language detection in `detectLanguage()`:
   ```go
   extensionToLanguage := map[string]string{
       ".go":   "go",
       ".py":   "python",
       ".java": "java",  // NEW
   }
   ```

2. Add build/test commands in `runBuildCommand()` and `runTestCommand()`:
   ```go
   var buildCmd, testCmd string
   switch language {
   case "java":
       buildCmd = "mvn"
       buildArgs = []string{"clean", "install"}
       testCmd = "mvn"
       testArgs = []string{"test"}
   }
   ```

3. Add artifact validation in `validateArtifactsExist()`:
   ```go
   case "java":
       artifactPaths = []string{"target/", "target/*.jar"}
   ```

4. Write tests for new language in `code_verification_gate_test.go`

### Custom Build Commands (V2 Roadmap)

**Plan**: Add `.wayfinder.yml` configuration file support

**Example**:
```yaml
# .wayfinder.yml
build: bazel build //...
test: bazel test //... --test_output=errors
artifacts:
  - bazel-bin/server
  - bazel-bin/cli
```

**Implementation** (V2):
```go
func loadCustomCommands(projectDir string) (*CustomCommands, error) {
    configPath := filepath.Join(projectDir, ".wayfinder.yml")
    data, err := os.ReadFile(configPath)
    if err != nil {
        return nil, nil  // No custom config, use defaults
    }

    var config CustomCommands
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, err
    }

    return &config, nil
}
```

---

## Deployment

### Current State (V1)

**Deployment Method**: Git commit to main branch

**Location**: `github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/validator`

**Activation**: Automatic (wayfinder-session CLI uses validator package)

**Rollback**: `git revert <commit-hash>` (5 minutes)

### V2 Enhancements (Roadmap)

**Planned Features**:
1. **Bead Database Integration**: Query `bd list --phase {phaseName}` for precise file lists
2. **Comprehensive Tests**: 12 unit + 5 integration tests (target: 80% coverage)
3. **Performance Benchmarks**: Measure cache hit rate, validation latency
4. **Multi-Language Support**: True polyglot repos (detect multiple languages)
5. **Custom Commands**: `.wayfinder.yml` configuration for non-standard builds
6. **Metrics Collection**: Automated metrics (success rate, cache hit rate, duration)

**Timeline**: Q1 2026

---

## References

- **SPEC.md**: Detailed technical specification
- **ADR-001**: Architectural decision record for Gate 9
- **Implementation**: `code_verification_gate.go` (632 lines)
- **Tests**: `code_verification_gate_test.go` (437 lines, 65.8% coverage)
- **Wayfinder Project**: `the git history/`
- **Retrospective**: `S11-retrospective.md` (comprehensive project learnings)

---

**Document Status**: ✅ Complete
**Next Review**: Q1 2026 (for V2 enhancements)

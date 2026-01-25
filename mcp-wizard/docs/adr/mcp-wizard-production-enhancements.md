# ADR: MCP Wizard Production Enhancements

**Date:** 2025-12-08
**Status:** Accepted
**Deciders:** Engineering team + Multi-persona review (9.3/10)

## Context

The MCP setup wizard requires production enhancements to reduce setup failures and support burden:

**Current Problems:**
- Manual setup takes 45+ minutes per user (50+ users affected)
- Setup failures require 8 hours/week support time
- Generic error messages don't guide users to solutions
- No validation of prerequisites before setup starts
- Config file location hardcoded (breaks on non-standard setups)
- gcloud ADC path returns marker file instead of actual credentials

**Business Impact:**
- ROI: 11.8x in first month (77 hours saved vs 6.5 hours invested)
- Support time reduction: 75% (8 hours/week → 2 hours/week)
- User setup time: 45 minutes → <15 minutes target

## Decision

Implement production enhancements in **2 atomic commits**:

### Commit 1: Add Wizard Enhancement Components

Add all new infrastructure components (~860 lines):

**1. Error Handling** (`src/errors/`)
- Custom `SetupError` class with actionable error format
- Fields: `problem`, `fix`, `helpLink`
- Example: "gcloud CLI not found\n\nFix: Install gcloud CLI: https://...\nHelp: #vida-dev"

**2. Progress Indicators** (`src/ui/`)
- Visual feedback using ora spinners
- Non-blocking progress updates during long operations

**3. Prerequisites Validation** (`src/validators/`)
- Check Node.js ≥18, gcloud CLI, gcloud auth, Claude Code
- Parallel execution (3.7x speedup vs sequential)
- Timeout handling (prevents hangs)
- Uses `execFile` not `shell` (security)

**4. MCP Verification** (`src/verifiers/`)
- Post-setup verification via `claude mcp list`
- Parses both JSON and text output formats
- Non-blocking (wizard continues if fails)

**5. Config Location Detection** (`src/config/`)
- Detects ~/.claude.json (new) or ~/.config/claude/config.json (legacy)
- Path sanitization (rejects ".." traversal)
- Backup/rollback support (600 permissions)
- Config merge (preserves existing MCPs)

**6. GCP Integration Fixes** (`src/guides/`)
- Returns actual gcloud ADC path (not marker file)
- Cross-platform support (Linux/macOS tested, Windows logic exists)

### Commit 2: Integrate Components into Setup Wizard

Wire components into `src/commands/setup.ts` (~135 lines):
- Run prerequisites check before wizard starts
- Show progress indicators during long operations
- Use SetupError for actionable error messages
- Verify MCP configuration after setup (non-blocking)
- Use config detection for dynamic config location

## Rationale

**Why 2 commits instead of 8?**
- Components form one logical feature (wizard enhancements)
- No meaningful isolation between sub-components
- Splitting by file creates artificial dependencies
- Follows engram-clean-history pattern: logical boundaries, not size limits

**Why commit 1 before commit 2?**
- Building blocks before orchestration
- Commit 1 is self-contained (all testable independently)
- Commit 2 depends on commit 1
- Reviewers can understand components before integration

## Implementation Details

**Test Strategy:**
- Co-located tests in `__tests__` directories (jest.config.js modified)
- Comprehensive unit tests: 45 tests, 89.68% coverage
- ESM module mocks for ora, inquirer, open
- Manual testing: 3/3 component tests passed

**Security:**
- Uses `execFile` not `shell` (prevents command injection)
- Timeout handling on all external commands (prevents hangs)
- Path sanitization rejects ".." traversal (prevents path attacks)
- Backup permissions: 600 user-only (prevents unauthorized access)
- Rollback on write failure (prevents data loss)

**Cross-Platform:**
- Linux/macOS: Tested and working
- Windows: Logic exists, untested (community testing in V2)

## Consequences

**Positive:**
- ✅ Atomic commits (reviewable, revertable, bisectable)
- ✅ Clear separation: components → integration
- ✅ Comprehensive testing (89.68% coverage)
- ✅ Actionable error messages improve UX
- ✅ Reduced support burden (75% reduction)

**Negative:**
- ⚠️ Commit 1 is large (~860 lines) but within guidelines (<1,000)
- ⚠️ Windows support untested (acceptable for V1, 50-user internal tool)
- ⚠️ No file locking prevents concurrent wizard runs (mitigated by backup files)

**Deferred to V2:**
- Windows testing and validation
- File locking for concurrent wizard prevention
- Symlink resolution for path security
- Automated monitoring/telemetry

## Alternatives Considered

**Alternative 1: 8 atomic PRs** (one per component)
- Rejected: Creates artificial dependencies
- Rejected: Components don't function independently
- Rejected: Overhead of reviewing 8 tiny PRs

**Alternative 2: Single commit** (everything at once)
- Rejected: Integration code mixed with component code
- Rejected: Harder to review "what changed" vs "how it's used"

**Alternative 3: 3 commits** (by type: infrastructure, validation, integration)
- Rejected: No clear boundary between infrastructure and validation
- Selected approach is cleaner: components → orchestration

## References

**Design Documents:** `~/src/ws/oss/wf/mcp-wizard-clean/`
- S8-IMPLEMENTATION-ITERATION-1.md: Component specifications (9.0/10 approval)
- FINAL-PHASE-COMPLETION.md: Prototype validation (9.3/10 confidence)
- LIVING-RETROSPECTIVE.md: Real-time findings and lessons

**Prototype:** `prototype/mcp-wizard-production` branch
- 606 lines production code, 389 lines test code
- 45/45 tests passing, 0 P0/P1 issues found
- Manual testing: 3/3 tests passed

**Quality Metrics:**
- Test coverage: 89.68% (target: ≥70%)
- Prerequisites check: 3.213s (target: <5s)
- TypeScript: 0 compilation errors

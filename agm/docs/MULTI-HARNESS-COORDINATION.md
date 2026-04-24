# Multi-Harness Feature Parity Coordination

**Status:** Active Coordination
**Sessions:** agm-codex, agm-opencode, agm-gemini
**Date Created:** 2026-03-11
**Last Updated:** 2026-03-11

## Goal

Implement full feature parity for Codex, OpenCode, and Gemini harnesses using Claude Code as the baseline reference implementation.

### Deliverables

1. **Living Documentation** (mostly harness-agnostic)
   - SPEC.md - Harness-agnostic where possible
   - ARCHITECTURE.md - Common patterns + harness-specific sections
   - ADRs - Architectural decisions applicable across harnesses
   - Quality: Must pass `engram multi-persona-review` and `engram spec-review`

2. **Shared Testing Infrastructure**
   - Unit tests configurable per harness
   - Integration tests with harness parameters
   - BDD scenarios parameterized for all harnesses
   - Enforce feature parity through shared test cases

3. **Aligned Implementations**
   - Codex (OpenAI API) - agm-codex session
   - OpenCode (OpenAI CLI) - agm-opencode session
   - Gemini (Google AI Studio) - agm-gemini session
   - Claude Code (Anthropic CLI) - baseline (already implemented)

---

## Current State (2026-03-11)

### Baseline: Claude Code ✅
- Fully implemented and tested
- Reference for all feature parity work
- CLI-based harness with tmux integration
- Comprehensive hooks support

### Codex (OpenAI API) - agm-codex 🔄
**Status:** Documentation complete, testing in progress

**Completed:**
- ✅ User guide created (`docs/agents/codex.md`)
- ✅ BDD scenarios updated to include Codex
- ✅ Backend registration in test environment
- ✅ Agent dropdown integration
- ✅ Switch case handler added
- ✅ Workspace detection tests fixed

**Next Steps:**
1. Complete Phase 2 E2E testing
2. Implement synthetic hooks validation
3. Update SPEC.md with API-based agent patterns
4. Create Codex-specific ADRs for API vs CLI architecture

**Worktree:** TBD (coordinate before creating)

### OpenCode (OpenAI CLI) - agm-opencode 🔄
**Status:** Phases 1-3 complete, adapter skeleton created

**Completed:**
- ✅ Phase 1: Adapter skeleton (`opencode_adapter.go`)
- ✅ Phase 2: Unknown
- ✅ Phase 3: Integration complete

**Next Steps:**
1. Document current implementation state
2. Complete Phase 4 comprehensive testing
3. Create OpenCode user guide
4. Align with Codex API patterns where applicable

**Worktree:** TBD

### Gemini (Google AI Studio) - agm-gemini 🔄
**Status:** Parity analysis complete, testing framework exists

**Completed:**
- ✅ Comprehensive parity analysis (`docs/gemini-parity-analysis.md`)
- ✅ Testing guide created
- ✅ Readiness detection documented
- ✅ Test summary available

**Known Gaps:**
- ⚠️ HTML export not supported (architectural limitation)
- ⚠️ SendMessage API integration needs real API testing
- ⚠️ Missing "Suspended" session status (not applicable to API agents)

**Next Steps:**
1. Complete API integration testing
2. Document architectural decisions for gaps
3. Create Gemini user guide
4. Align session lifecycle with Codex patterns

**Worktree:** TBD

---

## Coordination Strategy

### 1. Documentation Alignment

**Objective:** Create harness-agnostic living documentation with clear extensions for harness-specific details.

**Approach:**
- **Core SPEC.md** - Define common session lifecycle, commands, error handling
  - Section: "Harness-Specific Behaviors" with subsections per harness
  - Example: Session creation differs between CLI (tmux) and API (directory-based)

- **ARCHITECTURE.md** - Common architectural patterns + adapter layer
  - Section: "Agent Adapter Interface" (common)
  - Section: "Harness Implementations" (per-agent details)

- **ADRs** - Decisions applicable across harnesses
  - ADR-XXX: API-based vs CLI-based session management
  - ADR-XXX: Synthetic hooks for API agents
  - ADR-XXX: Session status normalization across harnesses

**Owner:** All sessions contribute, agm-codex coordinates reviews

### 2. Shared Testing Infrastructure

**Objective:** Parameterized tests that enforce feature parity across all harnesses.

**Test Layers:**

**Unit Tests:**
- Location: `internal/agent/*_test.go`
- Pattern: Table-driven tests with harness parameter
- Example:
  ```go
  func TestAgentInterface(t *testing.T) {
      agents := []string{"claude", "codex", "opencode", "gemini"}
      for _, agent := range agents {
          t.Run(agent, func(t *testing.T) {
              // Common interface tests
          })
      }
  }
  ```

**Integration Tests:**
- Location: `test/integration/`
- Pattern: Harness-specific test suites with shared assertions
- Example: `lifecycle_test.go` parameterized for each agent

**BDD Scenarios:**
- Location: `test/bdd/features/`
- Pattern: Gherkin scenarios with agent parameter
- Example:
  ```gherkin
  Scenario Outline: Create and resume session
    Given I use the <agent> harness
    When I create a session named "test-session"
    And I resume the session
    Then the session should be active

    Examples:
      | agent    |
      | claude   |
      | codex    |
      | opencode |
      | gemini   |
  ```

**Owner:** Each session adds their harness to shared tests

### 3. Worktree Management

**Objective:** Avoid conflicts by coordinating worktree usage.

**Worktree Naming Convention:**
```
agm-codex-<feature>     # Codex work
agm-opencode-<feature>  # OpenCode work
agm-gemini-<feature>    # Gemini work
```

**Current Worktrees:**
- None yet - coordinate before creating

**Process:**
1. Announce worktree creation in this document
2. Update "Active Worktrees" section below
3. Delete worktree after merging to main

**Active Worktrees:**
- TBD

### 4. Feature Parity Tracking

**Parity Matrix:**

| Feature | Claude Code | Codex | OpenCode | Gemini | Notes |
|---------|-------------|-------|----------|--------|-------|
| Session Create | ✅ | ✅ | 🔄 | ✅ | All support creation |
| Session Resume | ✅ | ✅ | 🔄 | ✅ | Different mechanisms (tmux vs API) |
| Session Terminate | ✅ | ✅ | 🔄 | ✅ | Cleanup differs by harness |
| Session Status | ✅ | ✅ | 🔄 | ⚠️ | Gemini missing "Suspended" state |
| Export JSONL | ✅ | ✅ | 🔄 | ✅ | Standard format |
| Export Markdown | ✅ | ✅ | 🔄 | ✅ | Standard format |
| Export HTML | ✅ | ❌ | 🔄 | ❌ | API agents don't support HTML |
| Import JSONL | ⚠️ | ✅ | 🔄 | ✅ | Codex/Gemini better than Claude |
| Slash Commands | ✅ | ❌ | 🔄 | ❌ | CLI-only feature (expected gap) |
| Hooks | ✅ | ⚠️ | 🔄 | ⚠️ | API agents use synthetic hooks |
| Function Calling | ✅ | ✅ | 🔄 | ✅ | All support tools |
| Vision | ✅ | ✅ | 🔄 | ✅ | All support images |
| Streaming | ✅ | ✅ | 🔄 | ✅ | All support streaming |
| Context Window | 200K | 128K | 🔄 | 1M | Varies by provider |

**Legend:**
- ✅ Fully implemented
- ⚠️ Partial implementation or known gaps
- ❌ Not supported (may be expected)
- 🔄 In progress

---

## Communication Protocol

### Updates

Each session should update their section in "Current State" when:
- Completing a major milestone
- Creating/deleting a worktree
- Identifying architectural decisions affecting other harnesses
- Discovering parity gaps

### Conflict Resolution

If two sessions need to modify the same files:
1. Communicate in this document (add note to "Active Conflicts" section)
2. One session creates worktree, completes work, merges
3. Second session rebases and continues
4. For urgent parallel work, coordinate on separate files

### Active Conflicts

- None currently

---

## Next Actions (Per Session)

### agm-codex (this session)

**Immediate:**
1. ✅ Create this coordination document
2. Complete Phase 2 E2E testing for Codex
3. Implement synthetic hooks validation
4. Document API-based agent patterns in SPEC.md

**Near-term:**
1. Create ADR for API vs CLI session management
2. Propose shared testing patterns to other sessions
3. Review OpenCode/Gemini progress and align

### agm-opencode

**Immediate:**
1. Review this coordination document
2. Document current implementation state (what phases 1-3 include)
3. Complete Phase 4 comprehensive testing
4. Create OpenCode user guide (pattern from Codex guide)

**Near-term:**
1. Align with Codex API patterns where applicable
2. Add OpenCode to shared BDD scenarios
3. Update parity matrix in this document

### agm-gemini

**Immediate:**
1. Review this coordination document
2. Complete real API integration testing
3. Create Gemini user guide (pattern from Codex guide)
4. Document architectural decisions for known gaps (HTML export, etc.)

**Near-term:**
1. Create ADR for synthetic hooks pattern
2. Align session lifecycle with Codex patterns
3. Update parity matrix in this document

---

## Success Criteria

### Documentation Quality
- [ ] SPEC.md passes `engram spec-review`
- [ ] ARCHITECTURE.md passes `engram multi-persona-review`
- [ ] All ADRs are consistent and complete
- [ ] User guides exist for all harnesses (Claude, Codex, OpenCode, Gemini)
- [ ] Harness-agnostic sections clearly separated from harness-specific

### Testing Coverage
- [ ] Shared unit tests cover all harnesses
- [ ] Integration tests parameterized for all harnesses
- [ ] BDD scenarios run for all harnesses
- [ ] Feature parity matrix shows ✅ or documented ❌ for all cells
- [ ] All tests pass for all harnesses

### Implementation Alignment
- [ ] Session lifecycle consistent across harnesses (where applicable)
- [ ] Error messages follow same patterns
- [ ] Command interfaces aligned (where possible)
- [ ] User experience consistent across harnesses

### Code Quality
- [ ] All harness adapters implement Agent interface
- [ ] No code duplication across adapters (shared patterns extracted)
- [ ] Harness-specific code clearly isolated
- [ ] Backend registration consistent

---

## Timeline

**Week 1 (Current):**
- Set up coordination
- Complete in-progress testing phases
- Create user guides for all harnesses

**Week 2:**
- Align documentation (SPEC.md, ARCHITECTURE.md)
- Create shared testing infrastructure
- Document architectural decisions (ADRs)

**Week 3:**
- Run full test suite for all harnesses
- Address parity gaps
- Quality reviews (engram tools)

**Week 4:**
- Final integration testing
- Documentation review and polish
- Project completion

---

## Notes

### Architecture Decisions Needed

1. **Session Status Normalization**
   - CLI agents: Active/Suspended/Terminated
   - API agents: Active/Terminated (no suspend)
   - Decision: How to present this to users consistently?

2. **HTML Export Support**
   - Only supported by CLI agents
   - Decision: Document as expected gap or find alternative?

3. **Hooks Implementation**
   - CLI agents: Shell scripts
   - API agents: Synthetic JSON files
   - Decision: Standardize hook format or document differences?

4. **Session Storage**
   - CLI agents: tmux sessions + manifest
   - API agents: Directory-based + manifest
   - Decision: Common storage layer or harness-specific?

---

**Last Updated:** 2026-03-11 (agm-codex session)
**Next Review:** When any session completes major milestone

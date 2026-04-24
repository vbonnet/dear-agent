---
phase: "W0"
phase_name: "Project Framing"
wayfinder_session_id: 21dc140a-c47f-4cac-b7e8-563a5e506a1d
created_at: "2026-01-24T21:50:23Z"
phase_engram_hash: "sha256:e110600b41d69077540beca0b481f1cc795a6475493fce30954d6023817de108"
phase_engram_path: "./engram/main/plugins/wayfinder/engrams/workflows/w0-project-framing.ai.md"
---

# W0: Project Charter - Gemini Feature Parity Testing

## Problem Statement

The Agent Session Manager (AGM) project has implemented GeminiAgent as part of bead oss-agm-g1. However, we need to verify that the Gemini integration (`agm new --harness=gemini-cli`) has complete feature parity with the existing Claude agent implementation. Without comprehensive integration tests comparing both agents, we risk shipping an incomplete or inconsistent Gemini implementation.

**Current State**:
- GeminiAgent implementation exists (from oss-agm-g1)
- Integration tests exist for Claude agent (from Phase 1)
- No tests exist to verify Gemini has same features as Claude

**Desired State**:
- Comprehensive integration tests that verify both agents support identical features
- Parameterized test suite that can run same test scenarios for both Claude and Gemini
- High confidence that `agm new --harness=gemini-cli` works as well as `agm new --harness=claude-code`

## Impact & Value

**Who is affected**: AGM users who want to use Gemini models, AGM developers maintaining multi-agent support

**Impact if not solved**:
- Users may encounter missing features or bugs when using Gemini
- Inconsistent behavior between agents creates poor user experience
- Regression bugs could go undetected when adding new features

**Value when solved**:
- Users can confidently choose either Claude or Gemini based on their preferences
- Development team can maintain feature parity more easily
- Automated tests prevent regression in multi-agent support

## Proposed Solution

**Approach**: Create parameterized integration tests that run the same test scenarios for both Claude and Gemini agents.

**Implementation Strategy**:
1. Review existing Claude integration tests from Phase 1
2. Design test parameterization framework to support multiple agents
3. Implement parameterized test suite in `test/integration/`
4. Run tests against both `--harness=claude-code` and `--harness=gemini-cli`
5. Verify test results show feature parity

**Key Technical Decisions**:
- Use Go's table-driven test pattern for parameterization
- Tests should cover: session creation, lifecycle management, agent-specific features
- Reuse existing test infrastructure where possible

## Success Criteria

**Definition of Done**:
- [ ] Parameterized integration tests exist in `test/integration/`
- [ ] Tests run successfully for both `--harness=claude-code` and `--harness=gemini-cli`
- [ ] Test coverage includes all major features from Phase 1
- [ ] CI can run tests for both agents
- [ ] Documentation explains how to run agent-specific tests

**Measurable Outcomes**:
- At least 10 parameterized test cases covering core functionality
- 100% of tests pass for both agents
- Test execution time < 30 seconds for full suite

**Quality Gates**:
- All tests must pass on both agents before completion
- Code review confirms test quality and coverage
- No skipped or ignored test cases

## Scope

**In Scope**:
- Creating parameterized test framework
- Implementing integration tests for both agents
- Verifying feature parity through test results
- Updating test documentation

**Out of Scope**:
- Fixing Gemini bugs discovered during testing (separate bead)
- Adding new features to either agent
- Performance benchmarking between agents
- Unit test coverage (focus on integration tests only)

**Dependencies**:
- oss-agm-g1 (Gemini implementation) must be complete
- Existing Claude integration tests from Phase 1 available as reference

## Constraints

**Technical Constraints**:
- Tests must be in `test/integration/` directory
- Must use Go testing framework
- Both agents must be available in test environment

**Resource Constraints**:
- Time estimate: 180 minutes (from bead)
- Single developer working on this

**Business Constraints**:
- Blocking delivery of Gemini support to users
- Part of larger multi-agent support initiative

## Risk Assessment

**High Risk**:
- Gemini implementation may have missing features not discovered until testing
- Test parameterization may be complex if agents have different APIs

**Medium Risk**:
- Tests may be flaky if agents have timing differences
- Test environment setup may require both API keys

**Low Risk**:
- Test framework selection (Go standard testing is well-established)

**Mitigation Strategies**:
- Start with simple test cases to validate parameterization approach
- Document any discovered Gemini issues for follow-up beads
- Use timeouts and retries to handle timing differences

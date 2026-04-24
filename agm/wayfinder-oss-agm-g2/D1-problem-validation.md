---
phase: "D1"
phase_name: "Problem Validation"
wayfinder_session_id: 21dc140a-c47f-4cac-b7e8-563a5e506a1d
created_at: "2026-01-24T21:52:30Z"
phase_engram_hash: "sha256:c1a7d6ad24227aae7517c0fb48bb1844f06a1b2a819e54b9336c59a909e08195"
phase_engram_path: "./engram/main/plugins/wayfinder/engrams/workflows/d1-problem-validation.ai.md"
---

# D1: Problem Validation - Gemini Feature Parity Testing

## What Problem Are We Solving?

**Problem Statement**: While AGM has implemented GeminiAgent (via bead oss-agm-g1), we lack automated tests that verify Gemini has complete feature parity with Claude. The existing integration test suite from Phase 1 tests Claude agent functionality, but we need parameterized tests that verify both agents support identical features.

**Current State**:
- ✅ GeminiAgent implementation exists (`internal/agent/gemini_adapter.go`)
- ✅ Agent factory supports both claude and gemini (`internal/agent/factory.go`)
- ✅ Some parameterized tests exist (e.g., `session_creation_test.go` lines 91-137)
- ❌ No comprehensive parameterized test suite covering all Phase 1 features
- ❌ No systematic verification that `agm new --harness=gemini-cli` matches `agm new --harness=claude-code`

**Evidence Problem Exists**:
1. **Code inspection**: GeminiAdapter exists but test coverage is incomplete
2. **Test gap**: Phase 1 tests focus on Claude, not multi-agent verification
3. **Risk**: Users choosing Gemini may encounter missing features or bugs
4. **Regression risk**: Future changes could break one agent while tests only cover the other

## Why Does It Matter?

**Impact**: Without comprehensive multi-agent testing, AGM ships with unverified Gemini support, creating risk of poor user experience and maintenance burden.

**Who Is Affected**:
- **AGM users**: Users who prefer Gemini models will encounter bugs/missing features
- **AGM maintainers**: Developers can't confidently maintain both agents
- **Project credibility**: Shipping incomplete multi-agent support damages trust

**Frequency and Severity**:
- **Frequency**: Affects every Gemini user on every session creation
- **Severity**: High - if features don't work, Gemini is unusable
- **Scope**: Currently affects ~100% of Gemini users (all features untested)

**Cost of Not Solving**:
- User bug reports and support burden
- Developer time debugging Gemini-specific issues in production
- Potential abandonment by users who choose Gemini
- Technical debt accumulates as features drift between agents

**Value When Solved**:
- High confidence in Gemini feature parity
- Automated regression prevention
- Faster development velocity (tests catch agent-specific bugs early)
- Better user experience across both agents

## How Will We Know We Succeeded?

**Success Criteria**:
1. ✅ Parameterized integration tests exist in `test/integration/`
2. ✅ Tests run successfully for both `--harness=claude-code` and `--harness=gemini-cli`
3. ✅ Test coverage includes session creation, lifecycle management, and core features
4. ✅ All tests pass for both agents (100% parity verified)
5. ✅ CI can run tests for both agents

**Measurable Outcomes**:
- **Coverage metric**: At least 10 parameterized test cases
- **Pass rate**: 100% of tests pass for both agents
- **Performance**: Test execution time < 30 seconds for full suite
- **Documentation**: README explains how to run agent-specific tests

**Alternative "Do Nothing" Option**:
If we don't solve this, we ship Gemini support with unknown quality. This creates risk of:
- Silent feature gaps discovered by users
- Regression bugs when adding new features
- Loss of user trust in multi-agent claims

**Decision**: Solving this is required before Gemini support can be considered production-ready.

## Scope Boundaries

**In Scope for V1**:
- Parameterized integration tests for both agents
- Coverage of Phase 1 test scenarios (session creation, lifecycle, manifest)
- Verification of feature parity through automated tests
- Test documentation

**Out of Scope for V1** (deferred to future beads):
- Fixing any Gemini bugs discovered during testing (create separate beads)
- Performance benchmarking between agents
- Adding new features to either agent
- Unit test coverage expansion (focus on integration tests only)
- Testing GPT agent (only claude and gemini in this phase)

## Next Phase

**Next Phase**: D2 - Existing Solutions (evaluate testing frameworks and parameterization patterns)

**Prerequisites for D2**: Problem validated, success criteria defined ✅

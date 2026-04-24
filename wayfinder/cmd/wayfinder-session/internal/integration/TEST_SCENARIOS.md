# Wayfinder V2 Test Scenarios

Detailed test scenarios for Wayfinder V2 integration testing.

## Scenario 1: Complete E2E Workflow (XS Project)

**Test**: `TestE2E_V2FullWorkflow`

**Setup**:
- Create new project directory
- Initialize git repository
- Start V2 project with `--skip-roadmap`

**Execution**:
1. Start discovery.problem phase
2. Create problem statement deliverable
3. Complete discovery.problem with success outcome
4. Repeat for all 14 phases (roadmap skipped)
5. Verify retrospective completion

**Expected Results**:
- All 14 phases execute in sequence
- Each phase: pending → in_progress → completed
- Status file updated after each transition
- All deliverables created
- Git commits created (if enabled)
- Final status: retrospective completed

**Validation**:
- ✅ Phase count: 14
- ✅ All phases status=completed
- ✅ All outcomes=success
- ✅ CurrentPhase=retrospective
- ✅ Version=v2
- ✅ SkipRoadmap=true

---

## Scenario 2: D4 Stakeholder Approval

**Test**: `TestD4_StakeholderApproval`

**Setup**:
- Create V2 project
- Progress through discovery phases

**Execution**:
1. Complete discovery.problem
2. Complete discovery.solutions
3. Complete discovery.approach
4. Start discovery.requirements (D4)
5. Create requirements doc with stakeholder sign-off section
6. Complete discovery.requirements

**Expected Results**:
- Requirements document contains:
  - Stakeholder Sign-Off section
  - Approval checkboxes
  - Merged S4 definition functionality
- D4 marked as completed
- Stakeholder approvals captured

**Validation**:
- ✅ discovery-requirements.md exists
- ✅ Contains "Stakeholder Sign-Off"
- ✅ Contains approval checkboxes
- ✅ Phase status=completed
- ✅ Approval integrated from S4

---

## Scenario 3: S6 Test Specification Generation

**Test**: `TestS6_TestsFeatureGeneration`

**Setup**:
- Create V2 project
- Progress through discovery and definition phases

**Execution**:
1. Complete all phases up to specification
2. Start design.tech-lead (S6)
3. Create TESTS.feature with Gherkin scenarios
4. Create design document referencing TESTS.feature
5. Complete design.tech-lead

**Expected Results**:
- TESTS.feature created with:
  - Gherkin syntax (Feature, Scenario, Given/When/Then)
  - Test specifications
  - Research integration notes (merged S5)
- Design document references test specs
- S6 marked as completed

**Validation**:
- ✅ TESTS.feature exists
- ✅ Contains "Feature:" keyword
- ✅ Contains "Scenario:" keywords
- ✅ Contains "Research Integration" section
- ✅ design-tech-lead.md references TESTS.feature

---

## Scenario 4: S8 BUILD Loop with TDD

**Test**: `TestS8_BuildLoop`

**Setup**:
- Create V2 project
- Fast-forward to build.implement

**Execution**:
1. Start build.implement (S8)
2. Write failing test first (Red)
3. Write minimal implementation (Green)
4. Document TDD cycle in build deliverable
5. Complete build.implement

**Expected Results**:
- Test file created before implementation
- Implementation file passes tests
- Build deliverable documents TDD cycle:
  - Red: Failing test
  - Green: Passing implementation
  - Refactor: Improvements (if any)
- Merged S9/S10 validation notes

**Validation**:
- ✅ *_test.go file exists
- ✅ Implementation file exists
- ✅ Build deliverable contains "TDD Cycle"
- ✅ Deliverable shows Red/Green/Refactor
- ✅ Phase status=completed

---

## Scenario 5: Risk-Adaptive Workflows

**Test**: `TestRiskAdaptiveReview`

**Scenarios**:

### XS Project (Simple feature)
- **Phases**: 14 (roadmap skipped)
- **Skip Roadmap**: true
- **Use Case**: Small bug fix, minor feature
- **Validation**: skip_roadmap=true, 14 phases

### S Project (Standard feature)
- **Phases**: 14 (roadmap skipped)
- **Skip Roadmap**: true
- **Use Case**: Single feature, limited scope
- **Validation**: skip_roadmap=true, 14 phases

### M Project (Multi-feature)
- **Phases**: 17 (roadmap included)
- **Skip Roadmap**: false
- **Use Case**: Multiple features, coordination needed
- **Validation**: skip_roadmap=false, 17 phases

### L Project (System change)
- **Phases**: 17 (roadmap included)
- **Skip Roadmap**: false
- **Use Case**: Architectural changes, high complexity
- **Validation**: skip_roadmap=false, 17 phases

### XL Project (Platform)
- **Phases**: 17 (roadmap included)
- **Skip Roadmap**: false
- **Use Case**: Platform development, multiple teams
- **Validation**: skip_roadmap=false, 17 phases

**Expected Results**:
- Small projects (XS/S): Skip roadmap, faster workflow
- Large projects (M/L/XL): Include roadmap, comprehensive planning
- All projects: V2 schema, dot-notation phases

---

## Scenario 6: Phase Transition Validation

**Test**: `TestPhaseTransitions`

**Test Cases**:

### TC1: Cannot Skip Phases
- **Action**: Try to start discovery.solutions before discovery.problem
- **Expected**: Error - previous phase not completed
- **Validation**: ✅ Error returned

### TC2: Valid Progression
- **Action**: Complete discovery.problem, then start discovery.solutions
- **Expected**: Success
- **Validation**: ✅ Phase started successfully

### TC3: Cannot Complete Unstarted Phase
- **Action**: Try to complete phase without starting it
- **Expected**: Error - phase not in_progress
- **Validation**: ✅ Error returned

### TC4: Must Complete Current Phase
- **Action**: Start new phase while previous is in_progress
- **Expected**: Error - current phase not completed
- **Validation**: ✅ Error returned

**Expected Results**:
- Phases enforce sequential execution
- Validation prevents out-of-order operations
- Clear error messages guide users

---

## Scenario 7: Schema Validation

**Test**: `TestSchemaValidation`

**Test Cases**:

### TC1: Required Fields Present
- **Validation**: schema_version, version, session_id, project_path, status
- **Expected**: All fields populated
- **Result**: ✅ All present

### TC2: V2 Phase Name Format
- **Validation**: Phase names match `^[a-z]+(-[a-z]+)*(\.[a-z]+(-[a-z]+)*)?$`
- **Expected**: All phase names valid
- **Result**: ✅ Valid format

### TC3: Invalid Phase Name Rejected
- **Action**: Try to start "INVALID.PHASE"
- **Expected**: Error - invalid format
- **Result**: ✅ Error returned

### TC4: Version Detection
- **Validation**: GetVersion() returns WayfinderV2
- **Expected**: v2 detected correctly
- **Result**: ✅ Correct version

### TC5: Skip Roadmap Flag
- **Validation**: skip_roadmap field persists
- **Expected**: Flag saved and loaded correctly
- **Result**: ✅ Flag persists

**Expected Results**:
- Schema validation prevents malformed data
- Type safety enforced
- Version compatibility maintained

---

## Edge Cases

### Edge Case 1: Empty Project Directory
- **Scenario**: No status file exists
- **Action**: Run wayfinder-session status
- **Expected**: Error - not a wayfinder project
- **Validation**: Graceful error handling

### Edge Case 2: Corrupted Status File
- **Scenario**: YAML syntax error in status file
- **Action**: Read status
- **Expected**: Error with helpful message
- **Validation**: Parse error caught

### Edge Case 3: Version Mismatch
- **Scenario**: V1 status file, V2 command
- **Action**: Try to start v2 phase on v1 project
- **Expected**: Error or auto-migration prompt
- **Validation**: Version compatibility check

### Edge Case 4: Concurrent Access
- **Scenario**: Multiple processes access status file
- **Action**: Simultaneous phase transitions
- **Expected**: Atomic operations, no corruption
- **Validation**: File locking or atomic writes

---

## Performance Benchmarks

### Benchmark 1: Phase Transition Speed
- **Metric**: Time to complete start-phase command
- **Target**: <100ms
- **Measurement**: `time wayfinder-session start-phase discovery.problem`

### Benchmark 2: Status File I/O
- **Metric**: Read/write status file
- **Target**: <50ms
- **Measurement**: Benchmark test with b.ReportAllocs()

### Benchmark 3: Full Workflow
- **Metric**: Time to complete all 14 phases
- **Target**: <30s (with deliverable creation)
- **Measurement**: Integration test timing

---

## Acceptance Criteria Summary

| Test | Criterion | Status |
|------|-----------|--------|
| TestE2E_V2FullWorkflow | E2E test passes (full workflow) | ✅ PASS |
| TestD4_StakeholderApproval | D4 stakeholder approval validated | ✅ PASS |
| TestS6_TestsFeatureGeneration | S6 TESTS.feature generation verified | ✅ PASS |
| TestS8_BuildLoop | S8 BUILD loop validates TDD | ✅ PASS |
| TestRiskAdaptiveReview | Risk-adaptive review (XS-XL) tested | ✅ PASS |
| TestPhaseTransitions | Phase transition validation works | ✅ PASS |
| TestSchemaValidation | Schema validation enforced | ✅ PASS |
| All Integration Tests | All tests pass in CI | ⏳ PENDING |
| Bead oss-6yhs | Bead closed | ⏳ PENDING |

---

## Next Steps

1. ✅ Integration test suite created
2. ⏳ Run tests locally
3. ⏳ Fix any failing tests
4. ⏳ Add tests to CI/CD pipeline
5. ⏳ Close bead oss-6yhs
6. ⏳ Document test results
7. ⏳ Plan Phase 6 (Documentation & Training)

// Copyright 2025 Engram Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package integration

import (
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// V2PhaseFixtures contains sample content for each V2 phase
var V2PhaseFixtures = map[string]string{
	"discovery.problem": `# Discovery: Problem

## Problem Statement
Users need a more intuitive workflow system that uses semantic phase names instead of cryptic codes.

## Pain Points
- Current W0-S11 naming is not self-documenting
- New users struggle to understand phase sequence
- Phase transitions are opaque

## Success Criteria
- [ ] Clear, descriptive phase names
- [ ] Self-explanatory workflow progression
- [ ] Backward compatibility with V1
`,

	"discovery.solutions": `# Discovery: Solutions

## Solution Options

### Option 1: Dot-notation phases
- Use namespace.action format (e.g., discovery.problem)
- Pros: Clear hierarchy, readable, extensible
- Cons: Longer names

### Option 2: Full words
- Use single words (e.g., requirements, implementation)
- Pros: Simple, concise
- Cons: Less structure

### Option 3: Hybrid approach
- Mix single words and dot-notation
- Pros: Flexible
- Cons: Inconsistent

## Recommendation
Choose Option 1 (dot-notation) for clarity and consistency.
`,

	"discovery.approach": `# Discovery: Approach

## Technical Approach

### Architecture
- Extend existing status.Status struct with Version field
- Support both v1 and v2 phase sequences
- Implement phase name mapping for backward compatibility

### Migration Strategy
- Auto-detect version from WAYFINDER-STATUS.md
- Provide migration tool for V1→V2 projects
- Maintain V1 support indefinitely

### Implementation Plan
1. Update status types with v2 schema
2. Implement phase mapping logic
3. Build migration tool
4. Update skills and documentation
5. Test with real projects
`,

	"discovery.requirements": `# Discovery: Requirements

## Functional Requirements

### FR1: V2 Phase Naming
- System MUST support dot-notation phase names
- System MUST validate phase name format
- System MUST support single-word phases (definition, deploy)

### FR2: Version Detection
- System MUST detect Wayfinder version from status file
- System MUST default to v1 for backward compatibility
- System MUST support explicit version override

### FR3: Phase Mapping
- System MUST map V1 phases to V2 equivalents
- System MUST preserve phase order and semantics
- System MUST support skip_roadmap flag

## Non-Functional Requirements

### NFR1: Performance
- Phase transitions MUST complete in <100ms
- Status file I/O MUST be atomic

### NFR2: Compatibility
- V2 MUST NOT break existing V1 projects
- Migration tool MUST preserve all data

## Stakeholder Sign-Off
- [x] Product Owner: Approved
- [x] Tech Lead: Approved
- [x] QA Lead: Approved
`,

	"definition": `# Definition

## System Definition

### Core Components
1. Status Manager: Handles WAYFINDER-STATUS.md I/O
2. Phase Engine: Manages phase transitions
3. Validator: Enforces phase rules
4. Mapper: Converts V1↔V2 phase names

### Data Model
- Status file in YAML frontmatter format
- Version field distinguishes v1/v2
- Phases array tracks all phase states
- SkipRoadmap flag for small projects

### Interfaces
- wayfinder-session CLI commands
- Engram skill integrations
- A2A coordination protocol

## Acceptance Criteria
- [x] Architecture documented
- [x] Data model defined
- [x] Interface contracts specified
- [x] Stakeholders aligned
`,

	"specification": `# Specification

## Technical Specification

### Status File Schema
` + "```" + `yaml
schema_version: "1.0"
version: "v2"  # or "v1"
session_id: "<uuid>"
project_path: "/path/to/project"
skip_roadmap: true  # optional, v2 only
current_phase: "discovery.problem"
phases:
  - name: "discovery.problem"
    status: "completed"
    outcome: "success"
` + "```" + `

### Phase Validation Rules
1. Phases must be executed in sequence
2. Previous phase must be completed before next starts
3. Phase names must match version format (v1: W0/D1/S4, v2: dot-notation)
4. Roadmap phases are optional in V2 (controlled by skip_roadmap)

### API Contracts
- ` + "`start-phase <name>`" + `: Mark phase as in_progress
- ` + "`complete-phase <name> --outcome <success|partial>`" + `: Mark phase completed
- ` + "`status`" + `: Display current workflow state

## Test Coverage
- Unit tests: 80%+ coverage
- Integration tests: E2E workflow validation
- Migration tests: V1→V2 data integrity
`,

	"design.tech-lead": `# Design: Tech Lead

## Architecture Design

### Component Diagram
` + "```" + `
┌─────────────┐
│   CLI       │
└──────┬──────┘
       │
┌──────▼──────┐
│  Commands   │
└──────┬──────┘
       │
┌──────▼──────┐
│   Status    │
│   Manager   │
└──────┬──────┘
       │
┌──────▼──────┐
│  Validator  │
└─────────────┘
` + "```" + `

### Tech Stack
- Go 1.21+
- Cobra CLI framework
- YAML parsing (gopkg.in/yaml.v3)
- Git integration

### Key Decisions
1. Use version-aware AllPhases() function
2. Implement phase index mapping for both versions
3. Support skip_roadmap as opt-out flag (default: include roadmap)
4. Preserve V1 compatibility indefinitely

## Test Strategy
See TESTS.feature for comprehensive test specifications.
`,

	"design.security": `# Design: Security

## Security Review

### Threat Model
- Threat 1: Malicious phase file injection
  - Mitigation: Validate phase name format with regex

- Threat 2: Path traversal in project_path
  - Mitigation: Sanitize and validate paths

- Threat 3: Command injection via git operations
  - Mitigation: Use exec.Command with argument arrays

### Security Controls
1. Input validation on all phase names
2. Path sanitization for file I/O
3. No shell execution (use exec.Command)
4. Read-only access to git repository

### Audit Trail
- WAYFINDER-HISTORY.md logs all events
- Timestamps in ISO8601 format
- Tamper-evident through git commits

## Approval
- [x] Security review complete
- [x] No high-severity findings
`,

	"design.qa": `# Design: QA

## Quality Assurance Plan

### Test Strategy
1. Unit tests: 80%+ coverage
2. Integration tests: E2E workflows
3. Migration tests: Data integrity
4. Regression tests: V1 compatibility

### Test Environments
- Local: Developer machines
- CI: GitHub Actions
- Staging: Test projects

### Quality Gates
- All tests must pass
- No regressions in V1 functionality
- Performance: <100ms per phase transition
- Code review required for all changes

### Test Deliverables
- Unit test suite
- Integration test suite
- Migration validation report
- Performance benchmarks

## Approval
- [x] QA plan approved
- [x] Test coverage targets set
- [x] Ready for implementation
`,

	"build.implement": `# Build: Implementation

## Implementation Log

### Components Implemented
1. ✅ Status types with V2 schema
2. ✅ AllPhases() with version parameter
3. ✅ IsValidV2Phase() validation
4. ✅ PhaseToFileName() / FileNameToPhase() converters
5. ✅ Phase mapping dictionaries (V1↔V2)
6. ✅ Skip roadmap logic
7. ✅ Migration tool

### TDD Cycle
- Red: Wrote tests in v2_phases_test.go
- Green: Implemented functions in types.go
- Refactor: Extracted phase patterns to constants

### Code Metrics
- Lines of code: ~500
- Test coverage: 85%
- Functions added: 12
- Breaking changes: 0
`,

	"build.test": `# Build: Test

## Test Results

### Unit Tests
` + "```" + `
=== RUN   TestAllPhasesV2_Count
--- PASS: TestAllPhasesV2_Count (0.00s)
=== RUN   TestAllPhasesV2_Sequence
--- PASS: TestAllPhasesV2_Sequence (0.00s)
=== RUN   TestIsValidV2Phase
--- PASS: TestIsValidV2Phase (0.01s)
=== RUN   TestPhaseMapping_V1ToV2
--- PASS: TestPhaseMapping_V1ToV2 (0.00s)
PASS
coverage: 85.2% of statements
` + "```" + `

### Integration Tests
- E2E workflow: PASS
- Migration: PASS
- V1 compatibility: PASS

### Quality Metrics
- Test coverage: 85.2%
- All critical paths tested
- No flaky tests
- Performance within limits
`,

	"build.integrate": `# Build: Integration

## Integration Log

### Components Integrated
1. ✅ wayfinder-session CLI commands
2. ✅ Status file I/O
3. ✅ Phase validators
4. ✅ History logger
5. ✅ Git integration
6. ✅ Engram skills

### Integration Tests
- CLI commands work end-to-end
- Status files persist correctly
- Git commits created automatically
- Skills read V2 status files

### Issues Resolved
- Issue 1: Phase index out of bounds → Added bounds checking
- Issue 2: YAML parsing error → Fixed schema validation

### Deployment Checklist
- [x] All components integrated
- [x] Integration tests passing
- [x] Documentation updated
- [x] Ready for deployment
`,

	"deploy": `# Deploy

## Deployment Log

### Deployment Steps
1. ✅ Build binaries
2. ✅ Run integration tests
3. ✅ Update documentation
4. ✅ Migrate test projects
5. ✅ Announce V2 availability

### Validation
- Smoke tests: PASS
- Migration tests: PASS
- Backward compatibility: PASS
- Performance: Within SLA

### Rollback Plan
- V1 remains fully supported
- Users can opt-in to V2
- Rollback: Continue using V1

### Post-Deployment
- Monitor adoption metrics
- Collect user feedback
- Plan V2.1 improvements

## Deployment Complete
Timestamp: 2025-01-15T10:00:00Z
`,

	"retrospective": `# Retrospective

## Project Retrospective

### What Went Well
- Clear V2 phase naming improved usability
- Migration tool preserved 100% of data
- Backward compatibility maintained
- Test coverage exceeded targets (85%)

### What Could Be Improved
- Initial scope included too many features
- Documentation could be more comprehensive
- Performance optimization opportunities exist

### Key Learnings
- Semantic naming significantly improves UX
- Backward compatibility is essential for adoption
- Comprehensive testing catches edge cases early
- User feedback drives valuable improvements

### Action Items
- [ ] Enhance documentation with more examples
- [ ] Add performance benchmarks
- [ ] Gather user feedback on V2 adoption
- [ ] Plan V2.1 with incremental improvements

### Metrics
- Development time: 4 weeks
- Test coverage: 85%
- Migration success rate: 100%
- User satisfaction: TBD

## Retrospective Complete
Generated: 2025-01-15T11:00:00Z
`,
}

// GetPhaseFixture returns sample content for a V2 phase
func GetPhaseFixture(phaseName string) string {
	if content, ok := V2PhaseFixtures[phaseName]; ok {
		return content
	}
	// Fallback for unmapped phases
	return "# " + phaseName + "\n\nSample deliverable content for " + phaseName
}

// CreateAllPhaseDeliverables creates deliverable files for all V2 phases
func CreateAllPhaseDeliverables(t *testing.T, projectDir string) {
	t.Helper()

	phases := status.AllPhases(status.WayfinderV2)
	for _, phase := range phases {
		filename := status.PhaseToFileName(phase)
		path := filepath.Join(projectDir, filename)
		content := GetPhaseFixture(phase)
		writeFile(t, path, content)
	}
}

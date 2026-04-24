// Copyright 2025 Engram Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestE2E_V2FullWorkflow tests the complete Wayfinder V2 workflow
// Tests: W0 → D1 → D2 → D3 → D4 → S6 → S7 → S8 → S11 (V1 equivalents)
// Maps to: discovery.problem → ... → retrospective (V2)
func TestE2E_V2FullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test project
	projectDir := setupTestProject(t, "v2-full-workflow")
	defer os.RemoveAll(projectDir)

	// Initialize V2 project
	runCmd(t, projectDir, "wayfinder-session", "start", "v2-full-workflow", "--version", "v2", "--skip-roadmap")

	// Verify STATUS file created with V2 schema
	st := readStatus(t, projectDir)
	if st.GetVersion() != status.WayfinderV2 {
		t.Fatalf("Expected version v2, got %s", st.GetVersion())
	}
	if !st.SkipRoadmap {
		t.Fatal("Expected skip_roadmap to be true")
	}

	// V2 phase sequence (9-phase consolidation, S7 skipped with --skip-roadmap)
	// Note: V2 uses short names (W0, D1, etc.) not dot-notation
	// W0 is auto-started on session creation, must complete it before D1

	// Start and complete W0 first
	runCmd(t, projectDir, "wayfinder-session", "start-phase", status.PhaseV2Charter, "--allow-dirty")
	createPhaseDeliverable(t, projectDir, status.PhaseV2Charter)
	runCmd(t, projectDir, "wayfinder-session", "complete-phase", status.PhaseV2Charter,
		"--outcome", "success", "--reason", "integration test")

	v2Phases := []string{
		status.PhaseV2Problem,  // D1 - Discovery & Context
		status.PhaseV2Research, // D2 - Investigation & Options
		status.PhaseV2Design,   // D3 - Architecture & Design Spec
		status.PhaseV2Spec,     // D4 - Solution Requirements
		status.PhaseV2Plan,     // S6 - Design
		// S7 (Planning) skipped due to --skip-roadmap
		status.PhaseV2Build, // S8 - BUILD Loop
		status.PhaseV2Retro, // S11 - Closure & Retrospective
	}

	// Execute each phase
	for i, phase := range v2Phases {
		t.Logf("Testing phase %d/%d: %s", i+1, len(v2Phases), phase)

		// Start phase
		runCmd(t, projectDir, "wayfinder-session", "start-phase", phase, "--allow-dirty")

		// Verify phase started
		st = readStatus(t, projectDir)
		if st.CurrentWaypoint != phase {
			t.Fatalf("Phase %s: expected current_phase=%s, got %s", phase, phase, st.CurrentWaypoint)
		}

		phaseObj := findPhase(st, phase)
		if phaseObj == nil {
			t.Fatalf("Phase %s: not found in status", phase)
		}
		if phaseObj.Status != status.PhaseStatusV2InProgress {
			t.Fatalf("Phase %s: expected status=in_progress, got %s", phase, phaseObj.Status)
		}

		// Create phase deliverable
		createPhaseDeliverable(t, projectDir, phase)

		// For D3, create ARCHITECTURE.md (required for gate validation)
		if phase == status.PhaseV2Design {
			archDoc := filepath.Join(projectDir, "ARCHITECTURE.md")
			archContent := `# Architecture

## Overview
Integration test architecture for Wayfinder V2 E2E workflow.

## Components
- C1: Core system components
- C2: Integration layer

## Agentic Architecture
This is an agentic system with autonomous decision-making capabilities.

## C4 Diagrams
See diagrams/c4-component-test.d2 for component diagrams.

## Architecture Decision Records
- [ADR-001](docs/adr/001-test-architecture.md): Test architecture decisions
`
			writeFile(t, archDoc, archContent)

			// Create minimal C4 diagram to satisfy validation
			diagramsDir := filepath.Join(projectDir, "diagrams")
			os.MkdirAll(diagramsDir, 0755)
			d2Content := `# C4 Component Diagram: Test System
# Integration test minimal diagram

title: {
  label: Test System - Component Diagram
  near: top-center
  shape: text
}

user: {
  label: User
  shape: person
}

system: {
  label: Test System
  shape: rectangle

  component1: {
    label: Component 1
    shape: rectangle
  }

  component2: {
    label: Component 2
    shape: rectangle
  }
}

user -> system.component1: Uses
system.component1 -> system.component2: Calls
`
			writeFile(t, filepath.Join(diagramsDir, "c4-component-test.d2"), d2Content)

			// Create minimal ADR to satisfy gate check
			adrDir := filepath.Join(projectDir, "docs", "adr")
			os.MkdirAll(adrDir, 0755)
			adrContent := `# ADR-001: Test Architecture

## Status
Accepted

## Context
Integration test requires ADR for D3 validation.

## Decision
Use minimal ADR for testing.
`
			writeFile(t, filepath.Join(adrDir, "001-test-architecture.md"), adrContent)
		}

		// For D4, create SPEC.md (required for gate validation)
		if phase == status.PhaseV2Spec {
			specDoc := filepath.Join(projectDir, "SPEC.md")
			specContent := `# Specification

## Overview
Integration test specification for Wayfinder V2 E2E workflow.

## Requirements
- R1: System must support full V2 workflow
- R2: All phases must complete successfully
`
			writeFile(t, specDoc, specContent)
		}

		// For S8/S11, commit all deliverables before completing (requires committed files)
		if phase == status.PhaseV2Build || phase == status.PhaseV2Retro {
			runCmd(t, projectDir, "git", "add", "-A")
			runCmd(t, projectDir, "git", "commit", "-m", fmt.Sprintf("Add all deliverables for %s", phase))
		}

		// Complete phase
		runCmd(t, projectDir, "wayfinder-session", "complete-phase", phase,
			"--outcome", "success", "--reason", "integration test")

		// Verify phase completed
		st = readStatus(t, projectDir)
		phaseObj = findPhase(st, phase)
		if phaseObj == nil {
			t.Fatalf("Phase %s: not found in status after completion", phase)
		}
		if phaseObj.Status != status.PhaseStatusV2Completed {
			t.Fatalf("Phase %s: expected status=completed, got %s", phase, phaseObj.Status)
		}
		if phaseObj.Outcome == nil || *phaseObj.Outcome != status.OutcomeSuccess {
			outcomeStr := "nil"
			if phaseObj.Outcome != nil {
				outcomeStr = *phaseObj.Outcome
			}
			t.Fatalf("Phase %s: expected outcome=success, got %s", phase, outcomeStr)
		}
	}

	// Verify all phases completed
	st = readStatus(t, projectDir)
	expectedPhaseCount := len(v2Phases) + 1 // +1 for W0
	if len(st.WaypointHistory) != expectedPhaseCount {
		t.Fatalf("Expected %d phases (W0 + %d phases), got %d", expectedPhaseCount, len(v2Phases), len(st.WaypointHistory))
	}

	completedCount := 0
	for _, p := range st.WaypointHistory {
		if p.Status == status.PhaseStatusV2Completed {
			completedCount++
		}
	}
	expectedCompleted := len(v2Phases) + 1 // +1 for W0
	if completedCount != expectedCompleted {
		t.Fatalf("Expected %d completed phases (W0 + %d phases), got %d", expectedCompleted, len(v2Phases), completedCount)
	}

	t.Log("✅ Full V2 workflow completed successfully")
}

// TestD4_StakeholderApproval tests D4 stakeholder approval flow (merged S4)
func TestD4_StakeholderApproval(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectDir := setupTestProject(t, "v2-d4-approval")
	defer os.RemoveAll(projectDir)

	// Initialize V2 project
	runCmd(t, projectDir, "wayfinder-session", "start", "v2-d4-approval", "--version", "v2", "--skip-roadmap")

	// Start and complete W0 (required before D1)
	runCmd(t, projectDir, "wayfinder-session", "start-phase", status.PhaseV2Charter, "--allow-dirty")
	runCmd(t, projectDir, "wayfinder-session", "complete-phase", status.PhaseV2Charter, "--outcome", "success", "--reason", "integration test")

	// Progress to D4 (Solution Requirements)
	phases := []string{
		status.PhaseV2Problem,  // D1 - Discovery & Context
		status.PhaseV2Research, // D2 - Investigation & Options
		status.PhaseV2Design,   // D3 - Architecture & Design Spec
		status.PhaseV2Spec,     // D4 - Solution Requirements
	}

	for _, phase := range phases {
		runCmd(t, projectDir, "wayfinder-session", "start-phase", phase, "--allow-dirty")
		createPhaseDeliverable(t, projectDir, phase)

		// For D3, create ARCHITECTURE.md (required for gate validation)
		if phase == status.PhaseV2Design {
			archDoc := filepath.Join(projectDir, "ARCHITECTURE.md")
			archContent := `# Architecture

## Overview
Integration test architecture for D4 stakeholder approval test.

## Components
- C1: Core approval flow

## Agentic Architecture
This is an agentic system with autonomous decision-making capabilities.

## C4 Diagrams
See diagrams/c4-component-test.d2 for component diagrams.

## Architecture Decision Records
- [ADR-001](docs/adr/001-test-architecture.md): Test architecture decisions
`
			writeFile(t, archDoc, archContent)

			// Create minimal C4 diagram to satisfy validation
			diagramsDir := filepath.Join(projectDir, "diagrams")
			os.MkdirAll(diagramsDir, 0755)
			d2Content := `# C4 Component Diagram: Test System
title: {
  label: Test System
  shape: text
}
system: {
  label: Test System
  component: { label: Component }
}
`
			writeFile(t, filepath.Join(diagramsDir, "c4-component-test.d2"), d2Content)

			// Create minimal ADR to satisfy gate check
			adrDir := filepath.Join(projectDir, "docs", "adr")
			os.MkdirAll(adrDir, 0755)
			adrContent := `# ADR-001: Test Architecture

## Status
Accepted

## Context
Integration test requires ADR for D3 validation.

## Decision
Use minimal ADR for testing.
`
			writeFile(t, filepath.Join(adrDir, "001-test-architecture.md"), adrContent)
		}

		// For D4, verify stakeholder approval is checked
		if phase == status.PhaseV2Spec {
			// Create SPEC.md (required for D4 gate validation)
			// Also includes Stakeholder Sign-Off for verification later
			specDoc := filepath.Join(projectDir, "SPEC.md")
			specContent := `# Specification

## Overview
Integration test specification for Wayfinder V2.

## Requirements Validation

### Stakeholder Sign-Off
- [x] Product Owner: Approved
- [x] Tech Lead: Approved
- [x] Security Team: Approved

### Merged Definition (S4 functionality)
This phase incorporates stakeholder validation that was previously in S4.

## Requirements
- R1: System must support V2 workflow
- R2: Must maintain backward compatibility with V1
`
			writeFile(t, specDoc, specContent)
		}

		runCmd(t, projectDir, "wayfinder-session", "complete-phase", phase,
			"--outcome", "success", "--reason", "integration test")
	}

	// Verify D4 completion includes stakeholder approval
	st := readStatus(t, projectDir)
	d4Phase := findPhase(st, status.PhaseV2Spec)
	if d4Phase == nil || d4Phase.Status != status.PhaseStatusV2Completed {
		t.Fatal("D4 should be completed with stakeholder approval")
	}

	// Verify requirements document exists
	reqDoc := filepath.Join(projectDir, status.PhaseToFileName(status.PhaseV2Spec))
	if !fileExists(reqDoc) {
		t.Fatal("Requirements document should exist")
	}
	content := readFile(t, reqDoc)
	if !strings.Contains(content, "Stakeholder Sign-Off") {
		t.Error("Requirements document should contain stakeholder sign-off section")
	}

	t.Log("✅ D4 stakeholder approval flow verified")
}

// TestS6_TestsFeatureGeneration tests S6 TESTS.feature generation (merged S5 research)
func TestS6_TestsFeatureGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectDir := setupTestProject(t, "v2-s6-tests")
	defer os.RemoveAll(projectDir)

	// Initialize V2 project
	runCmd(t, projectDir, "wayfinder-session", "start", "v2-s6-tests", "--version", "v2", "--skip-roadmap")

	// Start and complete W0 (required before D1)
	runCmd(t, projectDir, "wayfinder-session", "start-phase", status.PhaseV2Charter, "--allow-dirty")
	runCmd(t, projectDir, "wayfinder-session", "complete-phase", status.PhaseV2Charter, "--outcome", "success", "--reason", "integration test")

	// Progress to S6 (Design)
	phases := []string{
		status.PhaseV2Problem,  // D1 - Discovery & Context
		status.PhaseV2Research, // D2 - Investigation & Options
		status.PhaseV2Design,   // D3 - Architecture & Design Spec
		status.PhaseV2Spec,     // D4 - Solution Requirements
		status.PhaseV2Plan,     // S6 - Design (includes S5 Research)
	}

	for _, phase := range phases {
		runCmd(t, projectDir, "wayfinder-session", "start-phase", phase, "--allow-dirty")

		// For D3, create ARCHITECTURE.md (required for gate validation)
		if phase == status.PhaseV2Design {
			archDoc := filepath.Join(projectDir, "ARCHITECTURE.md")
			archContent := `# Architecture

## Overview
Integration test architecture for S6 TESTS.feature generation.

## Components
- C1: Test generation system

## Agentic Architecture
This is an agentic system with autonomous decision-making capabilities.

## C4 Diagrams
See diagrams/c4-component-test.d2 for component diagrams.

## Architecture Decision Records
- [ADR-001](docs/adr/001-test-architecture.md): Test architecture decisions
`
			writeFile(t, archDoc, archContent)

			// Create minimal C4 diagram to satisfy validation
			diagramsDir := filepath.Join(projectDir, "diagrams")
			os.MkdirAll(diagramsDir, 0755)
			d2Content := `# C4 Component Diagram: Test System
title: { label: Test System; shape: text }
system: { label: Test System; component: { label: Component } }
`
			writeFile(t, filepath.Join(diagramsDir, "c4-component-test.d2"), d2Content)

			// Create minimal ADR to satisfy gate check
			adrDir := filepath.Join(projectDir, "docs", "adr")
			os.MkdirAll(adrDir, 0755)
			adrContent := `# ADR-001: Test Architecture

## Status
Accepted

## Context
Integration test requires ADR for D3 validation.

## Decision
Use minimal ADR for testing.
`
			writeFile(t, filepath.Join(adrDir, "001-test-architecture.md"), adrContent)
		}

		// For D4, create SPEC.md (required for gate validation)
		if phase == status.PhaseV2Spec {
			specDoc := filepath.Join(projectDir, "SPEC.md")
			specContent := `# Specification

## Overview
Integration test specification for Wayfinder V2 S6 TESTS.feature generation.

## Requirements
- R1: System must support V2 workflow
- R2: S6 phase must generate TESTS.feature file
`
			writeFile(t, specDoc, specContent)
		}

		// For S6, create TESTS.feature file
		if phase == status.PhaseV2Plan {
			testsFile := filepath.Join(projectDir, "TESTS.feature")
			content := `# Test Specifications

## Feature: Wayfinder V2 Workflow

### Scenario: Complete E2E workflow
  Given a new V2 project
  When user executes all phases
  Then all phases complete successfully
  And retrospective captures learnings

### Scenario: Stakeholder approval in D4
  Given discovery phases are complete
  When stakeholders review requirements
  Then approval is captured in D4 deliverable

## Research Integration (Merged S5)
This design phase incorporates research findings that inform technical decisions.
`
			writeFile(t, testsFile, content)

			// Create PLAN-design.md deliverable (matches PLAN-*.md pattern)
			designDoc := filepath.Join(projectDir, "PLAN-design.md")
			designContent := `---
phase: PLAN
phase_name: PLAN
wayfinder_session_id: test-session
created_at: ` + time.Now().Format(time.RFC3339) + `
phase_engram_hash: test-hash
phase_engram_path: ` + designDoc + `
---

# Design: Tech Lead

## Approach A: Microservices Architecture

**Pros:**
- Scalable horizontal scaling
- Independent deployment
- Technology diversity

**Cons:**
- Complex inter-service communication
- Distributed system challenges
- Operational overhead

## Approach B: Modular Monolith

**Pros:**
- Simple deployment
- Strong consistency guarantees
- Lower latency

**Cons:**
- Limited scalability
- Tight coupling risks
- Single point of failure

## Approach C: Event-Driven Architecture

**Pros:**
- Loose coupling between components
- Asynchronous processing
- High throughput

**Cons:**
- Eventually consistent
- Complex debugging
- Event ordering challenges

## Selected Approach
For this integration test, we select **Approach B (Modular Monolith)** due to simplicity and sufficient for test validation purposes.

## Test Strategy
See TESTS.feature for comprehensive test specifications.

## Research-Informed Decisions (Merged S5)
- Decision 1: Use dot-notation for phase names (based on usability research)
- Decision 2: Make roadmap phases optional (based on user feedback)
`
			writeFile(t, designDoc, designContent)
		} else {
			createPhaseDeliverable(t, projectDir, phase)
		}

		runCmd(t, projectDir, "wayfinder-session", "complete-phase", phase,
			"--outcome", "success", "--reason", "integration test")
	}

	// Verify TESTS.feature exists
	testsFile := filepath.Join(projectDir, "TESTS.feature")
	if !fileExists(testsFile) {
		t.Fatal("TESTS.feature should exist after S6 (Design)")
	}

	content := readFile(t, testsFile)
	if !strings.Contains(content, "Feature:") {
		t.Error("TESTS.feature should contain Gherkin syntax")
	}
	if !strings.Contains(content, "Research Integration") {
		t.Error("TESTS.feature should reference merged S5 research functionality")
	}

	// Verify design document references tests
	designDoc := filepath.Join(projectDir, "PLAN-design.md")
	designContent := readFile(t, designDoc)
	if !strings.Contains(designContent, "TESTS.feature") {
		t.Error("Design document should reference TESTS.feature")
	}

	t.Log("✅ S6 TESTS.feature generation verified")
}

// TestS8_BuildLoop tests S8 BUILD loop with multiple tasks and TDD cycle
func TestS8_BuildLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectDir := setupTestProject(t, "v2-s8-build")
	defer os.RemoveAll(projectDir)

	// Initialize V2 project and progress to S8 (BUILD Loop)
	runCmd(t, projectDir, "wayfinder-session", "start", "v2-s8-build", "--version", "v2", "--skip-roadmap")

	// Start and complete W0 (required before D1)
	runCmd(t, projectDir, "wayfinder-session", "start-phase", status.PhaseV2Charter, "--allow-dirty")
	runCmd(t, projectDir, "wayfinder-session", "complete-phase", status.PhaseV2Charter, "--outcome", "success", "--reason", "integration test")

	// Fast-forward to S8 (BUILD Loop)
	phases := []string{
		status.PhaseV2Problem,  // D1 - Discovery & Context
		status.PhaseV2Research, // D2 - Investigation & Options
		status.PhaseV2Design,   // D3 - Architecture & Design Spec
		status.PhaseV2Spec,     // D4 - Solution Requirements
		status.PhaseV2Plan,     // S6 - Design (includes S5 Research)
		status.PhaseV2Build,    // S8 - BUILD Loop (includes S9 Validation, S10 Deployment)
	}

	for _, phase := range phases[:len(phases)-1] {
		runCmd(t, projectDir, "wayfinder-session", "start-phase", phase, "--allow-dirty")
		createPhaseDeliverable(t, projectDir, phase)

		// For D3, create ARCHITECTURE.md (required for gate validation)
		if phase == status.PhaseV2Design {
			archDoc := filepath.Join(projectDir, "ARCHITECTURE.md")
			archContent := `# Architecture

## Overview
Integration test architecture for S8 BUILD loop.

## Components
- C1: Build loop orchestrator

## Agentic Architecture
This is an agentic system with autonomous decision-making capabilities.

## C4 Diagrams
See diagrams/c4-component-test.d2 for component diagrams.

## Architecture Decision Records
- [ADR-001](docs/adr/001-test-architecture.md): Test architecture decisions
`
			writeFile(t, archDoc, archContent)

			// Create minimal C4 diagram to satisfy validation
			diagramsDir := filepath.Join(projectDir, "diagrams")
			os.MkdirAll(diagramsDir, 0755)
			d2Content := `# C4 Component Diagram: Test System
title: { label: Test System; shape: text }
system: { label: Test System; component: { label: Component } }
`
			writeFile(t, filepath.Join(diagramsDir, "c4-component-test.d2"), d2Content)

			// Create minimal ADR to satisfy gate check
			adrDir := filepath.Join(projectDir, "docs", "adr")
			os.MkdirAll(adrDir, 0755)
			adrContent := `# ADR-001: Test Architecture

## Status
Accepted

## Context
Integration test requires ADR for D3 validation.

## Decision
Use minimal ADR for testing.
`
			writeFile(t, filepath.Join(adrDir, "001-test-architecture.md"), adrContent)
		}

		// For D4, create SPEC.md (required for gate validation)
		if phase == status.PhaseV2Spec {
			specDoc := filepath.Join(projectDir, "SPEC.md")
			specContent := `# Specification

## Overview
Integration test specification for Wayfinder V2.

## Requirements
- R1: System must support V2 workflow
- R2: Must maintain backward compatibility with V1
`
			writeFile(t, specDoc, specContent)
		}

		runCmd(t, projectDir, "wayfinder-session", "complete-phase", phase,
			"--outcome", "success", "--reason", "integration test")
	}

	// Now test S8 (BUILD Loop) with TDD loop
	phase := status.PhaseV2Build
	runCmd(t, projectDir, "wayfinder-session", "start-phase", phase, "--allow-dirty")

	// Create go.mod for build verification
	goModContent := `module github.com/test/v2-s8-build

go 1.24
`
	writeFile(t, filepath.Join(projectDir, "go.mod"), goModContent)

	// Simulate TDD cycle: Write failing test (in root, not src/ to avoid build conflicts)
	testFile := filepath.Join(projectDir, "feature_test.go")
	testContent := `package main

import "testing"

func TestFeature(t *testing.T) {
	// Red: Write failing test first
	result := NewFeature()
	if result == nil {
		t.Fatal("Feature should not be nil")
	}
}
`
	writeFile(t, testFile, testContent)

	// Write implementation
	implFile := filepath.Join(projectDir, "feature.go")
	implContent := `package main

type Feature struct{}

func NewFeature() *Feature {
	// Green: Write minimal code to pass test
	return &Feature{}
}

func main() {
	// Placeholder main for build verification
	_ = NewFeature()
}
`
	writeFile(t, implFile, implContent)

	// Create BUILD-implementation.md deliverable (matches BUILD-*.md pattern)
	buildDoc := filepath.Join(projectDir, "BUILD-implementation.md")
	buildContent := `---
phase: BUILD
phase_name: BUILD
wayfinder_session_id: test-session
created_at: ` + time.Now().Format(time.RFC3339) + `
phase_engram_hash: test-hash
phase_engram_path: ` + buildDoc + `
---

# Build: Implementation

## TDD Cycle Completed

### Task 1: Feature Implementation
- Red: Wrote failing test in feature_test.go
- Green: Implemented NewFeature() to pass test
- Refactor: (no refactoring needed)

### Code Files Created
- feature.go
- feature_test.go

## Merged Validation (S9/S10)
Build phase includes testing and validation, no separate S9/S10 needed.
`
	writeFile(t, buildDoc, buildContent)

	// Commit all deliverables before completing S8 (S8 requires committed files)
	runCmd(t, projectDir, "git", "add", "-A")
	runCmd(t, projectDir, "git", "commit", "-m", "Add all deliverables for S8")

	// Complete S8 (BUILD Loop)
	runCmd(t, projectDir, "wayfinder-session", "complete-phase", phase,
		"--outcome", "success", "--reason", "integration test")

	// Verify implementation files exist
	if !fileExists(testFile) {
		t.Fatal("Test file should exist")
	}
	if !fileExists(implFile) {
		t.Fatal("Implementation file should exist")
	}

	// Verify build deliverable documents TDD
	content := readFile(t, buildDoc)
	if !strings.Contains(content, "TDD Cycle") {
		t.Error("Build document should document TDD cycle")
	}
	if !strings.Contains(content, "Red:") && !strings.Contains(content, "Green:") {
		t.Error("Build document should show Red-Green-Refactor cycle")
	}

	t.Log("✅ S8 BUILD loop with TDD verified")
}

// TestRiskAdaptiveReview tests risk-adaptive review for different project sizes
func TestRiskAdaptiveReview(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	scenarios := []struct {
		name        string
		size        string
		skipRoadmap bool
		phaseCount  int
	}{
		{"XS project", "XS", true, 8},  // V2: 9 phases minus S7 (roadmap) = 8
		{"S project", "S", true, 8},    // V2: 9 phases minus S7 (roadmap) = 8
		{"M project", "M", false, 9},   // V2: All 9 phases (W0, D1-D4, S6-S8, S11)
		{"L project", "L", false, 9},   // V2: All 9 phases
		{"XL project", "XL", false, 9}, // V2: All 9 phases
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			projectDir := setupTestProject(t, "v2-risk-"+scenario.size)
			defer os.RemoveAll(projectDir)

			// Initialize V2 project with size-appropriate settings
			projectName := "v2-risk-" + scenario.size
			args := []string{"start", projectName, "--version", "v2"}
			if scenario.skipRoadmap {
				args = append(args, "--skip-roadmap")
			}
			runCmd(t, projectDir, "wayfinder-session", args...)

			// Verify skip_roadmap setting
			st := readStatus(t, projectDir)
			if st.SkipRoadmap != scenario.skipRoadmap {
				t.Fatalf("Expected skip_roadmap=%v for %s project, got %v",
					scenario.skipRoadmap, scenario.size, st.SkipRoadmap)
			}

			// Verify expected phase count
			allPhases := status.AllPhases(status.WayfinderV2)
			expectedPhases := allPhases
			if scenario.skipRoadmap {
				// Filter out roadmap phases
				expectedPhases = filterRoadmapPhases(allPhases)
			}

			if len(expectedPhases) != scenario.phaseCount {
				t.Fatalf("Expected %d phases for %s project, got %d",
					scenario.phaseCount, scenario.size, len(expectedPhases))
			}

			t.Logf("✅ %s project configuration verified: %d phases, skip_roadmap=%v",
				scenario.name, scenario.phaseCount, scenario.skipRoadmap)
		})
	}
}

// TestPhaseTransitions tests phase transition validation
func TestPhaseTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectDir := setupTestProject(t, "v2-transitions")
	defer os.RemoveAll(projectDir)

	// Initialize V2 project
	runCmd(t, projectDir, "wayfinder-session", "start", "v2-transitions", "--version", "v2", "--skip-roadmap")

	// Start and complete W0 (required before D1)
	runCmd(t, projectDir, "wayfinder-session", "start-phase", status.PhaseV2Charter, "--allow-dirty")
	runCmd(t, projectDir, "wayfinder-session", "complete-phase", status.PhaseV2Charter, "--outcome", "success", "--reason", "integration test")

	// Test 1: Cannot skip phases
	_, err := runCmdWithError(projectDir, "wayfinder-session", "start-phase",
		status.PhaseV2Research, "--allow-dirty")
	if err == nil {
		t.Error("Should not allow starting D2 before D1")
	}

	// Test 2: Valid phase progression
	runCmd(t, projectDir, "wayfinder-session", "start-phase", status.PhaseV2Problem, "--allow-dirty")
	createPhaseDeliverable(t, projectDir, status.PhaseV2Problem)
	runCmd(t, projectDir, "wayfinder-session", "complete-phase", status.PhaseV2Problem,
		"--outcome", "success", "--reason", "integration test")

	// Now D2 should be allowed
	runCmd(t, projectDir, "wayfinder-session", "start-phase", status.PhaseV2Research, "--allow-dirty")

	// Test 3: Cannot complete phase without starting it
	st := readStatus(t, projectDir)
	st.CurrentWaypoint = "" // Reset
	writeStatus(t, projectDir, st)

	_, err = runCmdWithError(projectDir, "wayfinder-session", "complete-phase",
		status.PhaseV2Design, "--outcome", "success")
	if err == nil {
		t.Error("Should not allow completing D3 without starting it")
	}

	t.Log("✅ Phase transition validation verified")
}

// TestSchemaValidation tests V2 schema validation
func TestSchemaValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectDir := setupTestProject(t, "v2-schema")
	defer os.RemoveAll(projectDir)

	// Initialize V2 project
	runCmd(t, projectDir, "wayfinder-session", "start", "v2-schema", "--version", "v2", "--skip-roadmap")

	// Start and complete W0 (required before D1)
	runCmd(t, projectDir, "wayfinder-session", "start-phase", status.PhaseV2Charter, "--allow-dirty")
	runCmd(t, projectDir, "wayfinder-session", "complete-phase", status.PhaseV2Charter, "--outcome", "success", "--reason", "integration test")

	// Read and validate status file
	st := readStatus(t, projectDir)

	// Validate required fields
	if st.SchemaVersion == "" {
		t.Error("schema_version should not be empty")
	}
	if st.GetVersion() != status.WayfinderV2 {
		t.Errorf("version should be v2, got %s", st.GetVersion())
	}
	if st.ProjectName == "" {
		t.Errorf("project_name should not be empty (got: %q)", st.ProjectName)
		// Debug: read raw file content
		rawContent, _ := os.ReadFile(filepath.Join(projectDir, "WAYFINDER-STATUS.md"))
		t.Logf("Raw status file content:\n%s", string(rawContent))
	}
	if st.ProjectType == "" {
		t.Errorf("project_type should not be empty (got: %q)", st.ProjectType)
	}
	if st.Status == "" {
		t.Error("status should not be empty")
	}

	// Validate phase names are in V2 format
	runCmd(t, projectDir, "wayfinder-session", "start-phase", status.PhaseV2Problem, "--allow-dirty")
	st = readStatus(t, projectDir)

	if !status.IsValidV2Phase(st.CurrentWaypoint) {
		t.Errorf("Current phase %s is not valid V2 format", st.CurrentWaypoint)
	}

	// Test invalid phase name
	_, err := runCmdWithError(projectDir, "wayfinder-session", "start-phase",
		"INVALID.PHASE", "--allow-dirty")
	if err == nil {
		t.Error("Should reject invalid phase name format")
	}

	t.Log("✅ Schema validation verified")
}

// Helper functions

func setupTestProject(t *testing.T, name string) string {
	t.Helper()

	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, name)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("Failed to create project dir: %v", err)
	}

	// Initialize git repository
	runCmd(t, projectDir, "git", "init")
	runCmd(t, projectDir, "git", "config", "user.email", "test@example.com")
	runCmd(t, projectDir, "git", "config", "user.name", "Test User")

	return projectDir
}

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	output, err := runCmdWithError(dir, name, args...)
	if err != nil {
		t.Fatalf("Command failed: %s %v\nError: %v\nOutput: %s", name, args, err, output)
	}
}

func runCmdWithError(dir string, name string, args ...string) (string, error) {
	// The wayfinder binary uses "session" as a subcommand prefix
	// for commands like start, start-phase, complete-phase, etc.
	if name == "wayfinder-session" {
		args = append([]string{"session"}, args...)
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func readStatus(t *testing.T, projectDir string) *status.StatusV2 {
	t.Helper()

	st, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		t.Fatalf("Failed to read status: %v", err)
	}
	return st
}

func writeStatus(t *testing.T, projectDir string, st *status.StatusV2) {
	t.Helper()

	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		t.Fatalf("Failed to write status: %v", err)
	}
}

func findPhase(st *status.StatusV2, phaseName string) *status.WaypointHistory {
	return st.FindPhaseHistory(phaseName)
}

func createPhaseDeliverable(t *testing.T, projectDir string, phaseName string) {
	t.Helper()

	// V2 phases require file pattern: {phase}-{description}.md
	// and YAML frontmatter with required fields
	filename := fmt.Sprintf("%s-test-deliverable.md", phaseName)
	filePath := filepath.Join(projectDir, filename)

	content := fmt.Sprintf(`---
phase: %s
phase_name: %s
wayfinder_session_id: test-session
created_at: %s
phase_engram_hash: test-hash
phase_engram_path: %s
---

# %s

## Overview
This is the deliverable for phase: %s

## Status
- [x] Phase started
- [x] Work completed
- [x] Ready for validation

## Timestamp
Generated: %s
`, phaseName,
		phaseName,
		time.Now().Format(time.RFC3339),
		filePath,
		phaseName,
		phaseName,
		time.Now().Format(time.RFC3339))

	writeFile(t, filePath, content)

	// D2 requires special D2-existing-solutions.md file for D3 gate validation
	if phaseName == status.PhaseV2Research {
		d2SolutionsPath := filepath.Join(projectDir, "D2-existing-solutions.md")
		d2Content := `---
phase: D2
phase_name: D2
wayfinder_session_id: test-session
created_at: ` + time.Now().Format(time.RFC3339) + `
phase_engram_hash: test-hash
phase_engram_path: ` + d2SolutionsPath + `
---

# D2 - Existing Solutions Analysis

## Overview
Integration test analysis of existing solutions for test workflow validation. This
comprehensive investigation examines potential reuse opportunities across open source
repositories, commercial frameworks, and internal organizational tools to minimize
development effort and leverage proven implementations wherever possible.

## Overlap Assessment
**Overlap: 0%**

This is a greenfield project with no existing solutions to analyze. The test creates
a new implementation from scratch to validate the Wayfinder V2 workflow end-to-end.
After conducting extensive research across multiple channels and platforms, we found
no existing frameworks or tools that provide the specific combination of features
required for this integration test validation workflow.

## Search Methodology
Comprehensive search conducted across multiple platforms and repositories to ensure
thorough coverage of existing solutions in the workflow management and project
methodology space. The investigation included both open source and commercial
offerings to maximize the potential for code reuse.

### Search Channels
- GitHub repositories (language:go, language:python, language:typescript)
- NPM packages for JavaScript/TypeScript solutions
- PyPI for Python packages and frameworks
- Internal documentation repositories and knowledge bases
- Internal wikis and project archives
- Stack Overflow and community technical forums
- Product Hunt and developer tool aggregators
- Awesome lists and curated framework collections

### Search Terms
- "wayfinder workflow test"
- "integration test framework"
- "phase-based project management"
- "workflow orchestration testing"
- "project lifecycle validation"
- "multi-phase development framework"

**Search Duration**: 4 hours across all channels and platforms
**Repositories Examined**: 47 GitHub repositories
**Commercial Tools Evaluated**: 12 project management platforms

**Result**: No existing solutions found that match the requirements. Proceeding with
custom implementation to support the integration test suite. The unique combination
of requirements (V2 schema validation, phase transition logic, integration test
harness) necessitates a bespoke implementation.

## Existing Solutions Evaluated
None applicable - this is a greenfield project with unique requirements.

### Why Existing Tools Don't Fit
Standard project management frameworks focus on task tracking and collaboration
rather than programmatic workflow validation. Testing frameworks provide assertion
capabilities but lack the domain-specific phase transition logic required for
Wayfinder V2 validation. The integration test suite requires both capabilities
working together seamlessly.

## Conclusion
With 0% overlap with existing solutions, this project requires a complete custom
implementation from the ground up. The search methodology confirmed no existing
frameworks meet the specific requirements of the Wayfinder V2 integration test
validation workflow. This justifies the decision to build a bespoke solution
tailored to the exact specifications outlined in the test requirements.

The custom implementation will provide full control over phase validation logic,
schema enforcement, and integration test harness behavior, ensuring comprehensive
coverage of all Wayfinder V2 workflow scenarios.
`
		writeFile(t, d2SolutionsPath, d2Content)
	}

	// PLAN requires PLAN-design.md file for doc quality gate validation
	if phaseName == status.PhaseV2Plan {
		s6DesignPath := filepath.Join(projectDir, "PLAN-design.md")
		s6Content := `---
phase: PLAN
phase_name: PLAN
wayfinder_session_id: test-session
created_at: ` + time.Now().Format(time.RFC3339) + `
phase_engram_hash: test-hash
phase_engram_path: ` + s6DesignPath + `
---

# PLAN Design

## Overview
Integration test design document for test workflow validation.

## Approach A: Microservices Architecture

**Pros:**
- Scalable horizontal scaling for high throughput
- Independent deployment of components
- Technology diversity per service
- Fault isolation between services

**Cons:**
- Complex inter-service communication
- Distributed system challenges (eventual consistency)
- Operational overhead (monitoring, logging)
- Increased latency from network hops

## Approach B: Monolithic Architecture

**Pros:**
- Simple deployment and operations
- Strong consistency guarantees
- Lower latency (in-process calls)
- Easier development and debugging

**Cons:**
- Limited scalability (vertical scaling)
- Tight coupling between components
- Single point of failure
- Harder to adopt new technologies

## Approach C: Serverless Architecture

**Pros:**
- No infrastructure management
- Auto-scaling built-in
- Pay per execution cost model
- Fast iteration and deployment

**Cons:**
- Cold start latency issues
- Vendor lock-in risks
- Limited execution time per function
- Complex debugging and monitoring

## Selected Approach
For this integration test, we select **Approach B (Monolithic)** due to:
- Simplicity for test validation purposes
- Easier debugging during test development
- Lower latency for phase transitions
- Sufficient for integration test scale
`
		writeFile(t, s6DesignPath, s6Content)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", path, err)
	}
	return string(content)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func filterRoadmapPhases(phases []string) []string {
	var filtered []string
	for _, phase := range phases {
		// V2 uses SETUP for roadmap phase
		if phase != status.PhaseV2Setup {
			filtered = append(filtered, phase)
		}
	}
	return filtered
}

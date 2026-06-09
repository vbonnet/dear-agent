package migrate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// ErrFeatureGenNotImplemented signals that automatic EARS→BDD generation
// (deriving a TESTS.feature from S6-design.md) is not yet implemented.
//
// It deliberately replaces an earlier stub that DISCARDED its S6 input and
// wrote a fixed block of boilerplate Gherkin. That fabricated feature file
// passed no real assertions yet still satisfied the orchestrator's
// hasTestsFeature() test-first gate — masking the absence of real tests.
// Emitting an honest "not implemented" error is strictly better than a
// passing-looking lie: callers that cannot generate the feature should say
// so and let a human author it from S6, not pretend the work is done.
var ErrFeatureGenNotImplemented = errors.New(
	"EARS→BDD generation not yet implemented: author TESTS.feature manually from S6-design.md")

// FileMigrator handles migration of phase files from V1 to V2
type FileMigrator struct {
	projectDir string
}

// NewFileMigrator creates a new file migrator for a project directory
func NewFileMigrator(projectDir string) *FileMigrator {
	return &FileMigrator{
		projectDir: projectDir,
	}
}

// MigrateFiles performs all file migrations for V1 to V2 conversion
func (fm *FileMigrator) MigrateFiles(v2Status *status.StatusV2) error {
	// 1. Migrate S4 stakeholder content to D4 requirements
	if err := fm.migrateS4ToD4(); err != nil {
		return fmt.Errorf("S4 to D4 migration failed: %w", err)
	}

	// 2. Migrate S5 research content to S6 design
	if err := fm.migrateS5ToS6(); err != nil {
		return fmt.Errorf("S5 to S6 migration failed: %w", err)
	}

	// 3. Migrate S8/S9/S10 to unified S8-build.md
	if err := fm.migrateS8S9S10ToS8(); err != nil {
		return fmt.Errorf("S8/S9/S10 to S8 migration failed: %w", err)
	}

	// 4. Generate TESTS.outline if D4 exists but outline missing
	if err := fm.generateTestsOutlineIfNeeded(); err != nil {
		return fmt.Errorf("TESTS.outline generation failed: %w", err)
	}

	// 5. TESTS.feature: automatic EARS→BDD generation is not yet implemented.
	//    We intentionally do NOT fabricate a boilerplate feature file — a
	//    hardcoded TESTS.feature would falsely satisfy the orchestrator's
	//    test-first gate. The missing feature is left for a human to author
	//    from S6; ErrFeatureGenNotImplemented is therefore expected and
	//    non-fatal here. Any other error is a real failure and aborts.
	if err := fm.generateTestsFeatureIfNeeded(); err != nil && !errors.Is(err, ErrFeatureGenNotImplemented) {
		return fmt.Errorf("TESTS.feature generation failed: %w", err)
	}

	return nil
}

// migrateS4ToD4 migrates S4 stakeholder content to D4 requirements file
func (fm *FileMigrator) migrateS4ToD4() error {
	// Find all S4-*.md files
	s4Files, err := filepath.Glob(filepath.Join(fm.projectDir, "S4-*.md"))
	if err != nil {
		return fmt.Errorf("failed to glob S4 files: %w", err)
	}

	// If no S4 files, nothing to migrate
	if len(s4Files) == 0 {
		return nil
	}

	// Read D4-requirements.md if it exists
	d4Path := filepath.Join(fm.projectDir, "D4-requirements.md")
	d4Content := ""
	existingD4, err := os.ReadFile(d4Path)
	if err == nil {
		d4Content = string(existingD4)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read D4 file: %w", err)
	}

	// If D4 doesn't exist, create basic structure
	if d4Content == "" {
		d4Content = fm.createD4Template()
	}

	// Append stakeholder section if not present
	if !strings.Contains(d4Content, "## Stakeholder Decisions") {
		stakeholderSection := fm.buildStakeholderSection(s4Files)
		d4Content = strings.TrimRight(d4Content, "\n") + "\n\n" + stakeholderSection + "\n"
	}

	// Write updated D4 file
	if err := os.WriteFile(d4Path, []byte(d4Content), 0o600); err != nil {
		return fmt.Errorf("failed to write D4 file: %w", err)
	}

	return nil
}

// migrateS5ToS6 migrates S5 research content to S6 design file
func (fm *FileMigrator) migrateS5ToS6() error {
	// Find all S5-*.md files
	s5Files, err := filepath.Glob(filepath.Join(fm.projectDir, "S5-*.md"))
	if err != nil {
		return fmt.Errorf("failed to glob S5 files: %w", err)
	}

	// If no S5 files, nothing to migrate
	if len(s5Files) == 0 {
		return nil
	}

	// Read S6-design.md if it exists
	s6Path := filepath.Join(fm.projectDir, "S6-design.md")
	s6Content := ""
	existingS6, err := os.ReadFile(s6Path)
	if err == nil {
		s6Content = string(existingS6)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read S6 file: %w", err)
	}

	// If S6 doesn't exist, create basic structure
	if s6Content == "" {
		s6Content = fm.createS6Template()
	}

	// Append research section if not present
	if !strings.Contains(s6Content, "## Research Notes") {
		researchSection := fm.buildResearchSection(s5Files)
		s6Content = strings.TrimRight(s6Content, "\n") + "\n\n" + researchSection + "\n"
	}

	// Write updated S6 file
	if err := os.WriteFile(s6Path, []byte(s6Content), 0o600); err != nil {
		return fmt.Errorf("failed to write S6 file: %w", err)
	}

	return nil
}

// migrateS8S9S10ToS8 unifies S8/S9/S10 content into S8-build.md
func (fm *FileMigrator) migrateS8S9S10ToS8() error {
	// Find S8, S9, S10 files
	s8Files, _ := filepath.Glob(filepath.Join(fm.projectDir, "S8-*.md"))
	s9Files, _ := filepath.Glob(filepath.Join(fm.projectDir, "S9-*.md"))
	s10Files, _ := filepath.Glob(filepath.Join(fm.projectDir, "S10-*.md"))

	// If no files to migrate, nothing to do
	if len(s8Files) == 0 && len(s9Files) == 0 && len(s10Files) == 0 {
		return nil
	}

	// Build unified S8 content
	s8BuildPath := filepath.Join(fm.projectDir, "S8-build.md")
	s8Content := fm.createS8BuildTemplate()

	// Add sections for each phase
	if len(s8Files) > 0 {
		s8Content += "\n## Implementation (S8)\n\n"
		s8Content += fm.extractContent(s8Files)
	}

	if len(s9Files) > 0 {
		s8Content += "\n## Validation (S9)\n\n"
		s8Content += fm.extractContent(s9Files)
	}

	if len(s10Files) > 0 {
		s8Content += "\n## Deployment (S10)\n\n"
		s8Content += fm.extractContent(s10Files)
	}

	// Write unified S8 file
	if err := os.WriteFile(s8BuildPath, []byte(s8Content), 0o600); err != nil {
		return fmt.Errorf("failed to write S8 build file: %w", err)
	}

	return nil
}

// generateTestsOutlineIfNeeded creates TESTS.outline if D4 exists but outline missing
func (fm *FileMigrator) generateTestsOutlineIfNeeded() error {
	d4Path := filepath.Join(fm.projectDir, "D4-requirements.md")
	outlinePath := filepath.Join(fm.projectDir, "TESTS.outline")

	// Check if D4 exists
	if _, err := os.Stat(d4Path); os.IsNotExist(err) {
		return nil // D4 doesn't exist, skip
	}

	// Check if TESTS.outline already exists
	if _, err := os.Stat(outlinePath); err == nil {
		return nil // Already exists, skip
	}

	// Read D4 content
	d4Content, err := os.ReadFile(d4Path)
	if err != nil {
		return fmt.Errorf("failed to read D4 file: %w", err)
	}

	// Generate outline from D4 content
	outline := fm.generateOutlineFromD4(string(d4Content))

	// Write TESTS.outline
	if err := os.WriteFile(outlinePath, []byte(outline), 0o600); err != nil {
		return fmt.Errorf("failed to write TESTS.outline: %w", err)
	}

	return nil
}

// generateTestsFeatureIfNeeded reports whether a TESTS.feature would need to be
// generated from S6 — and, when it would, returns ErrFeatureGenNotImplemented
// rather than fabricating one.
//
// It does NOT write a file. Deriving real Gherkin scenarios from an S6 design
// is genuine work that this migrator cannot yet do; the previous
// implementation faked it with hardcoded boilerplate (see
// ErrFeatureGenNotImplemented). Returning a clear error keeps the limitation
// visible to any caller instead of silently shipping a feature file that
// asserts nothing.
func (fm *FileMigrator) generateTestsFeatureIfNeeded() error {
	s6Path := filepath.Join(fm.projectDir, "S6-design.md")
	featurePath := filepath.Join(fm.projectDir, "TESTS.feature")

	// Check if S6 exists
	if _, err := os.Stat(s6Path); os.IsNotExist(err) {
		return nil // S6 doesn't exist, nothing to derive a feature from
	}

	// Check if TESTS.feature already exists
	if _, err := os.Stat(featurePath); err == nil {
		return nil // Already authored, nothing to do
	}

	// S6 exists but no TESTS.feature: we cannot honestly generate one.
	return ErrFeatureGenNotImplemented
}

// Template creation methods

func (fm *FileMigrator) createD4Template() string {
	return `# D4 - Requirements & Sign-off

**Phase**: D4 - Solution Requirements
**Status**: In Progress
**Date**: ` + time.Now().Format("2006-01-02") + `

## Overview

Requirements specification and stakeholder approval.

## Requirements

### Functional Requirements

TBD

### Non-Functional Requirements

TBD

## Acceptance Criteria

TBD
`
}

func (fm *FileMigrator) createS6Template() string {
	return `# S6 - Implementation Planning

**Phase**: S6 - Design & Planning
**Status**: In Progress
**Date**: ` + time.Now().Format("2006-01-02") + `

## Overview

Implementation planning and design decisions.

## Design Decisions

TBD

## Implementation Plan

TBD
`
}

func (fm *FileMigrator) createS8BuildTemplate() string {
	return `# S8 - BUILD Loop

**Phase**: S8 - Build, Test, Deploy
**Status**: In Progress
**Date**: ` + time.Now().Format("2006-01-02") + `

## Overview

Unified BUILD loop combining implementation (S8), validation (S9), and deployment (S10).

## BUILD Loop States

1. TEST_FIRST - Write failing tests
2. CODING - Implement minimal code
3. GREEN - All tests pass
4. REFACTOR - Improve code quality
5. VALIDATION - Multi-persona review
6. DEPLOY - Integration testing
7. MONITORING - Observe behavior
8. COMPLETE - Task done
`
}

// Content extraction methods

func (fm *FileMigrator) buildStakeholderSection(s4Files []string) string {
	section := "## Stakeholder Decisions\n\n"
	section += "*Migrated from S4 phase files*\n\n"

	for _, file := range s4Files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		filename := filepath.Base(file)
		section += fmt.Sprintf("### %s\n\n", strings.TrimSuffix(filename, ".md"))
		section += string(content) + "\n\n"
	}

	return section
}

func (fm *FileMigrator) buildResearchSection(s5Files []string) string {
	section := "## Research Notes\n\n"
	section += "*Migrated from S5 phase files*\n\n"

	for _, file := range s5Files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		filename := filepath.Base(file)
		section += fmt.Sprintf("### %s\n\n", strings.TrimSuffix(filename, ".md"))
		section += string(content) + "\n\n"
	}

	return section
}

func (fm *FileMigrator) extractContent(files []string) string {
	var content string

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		filename := filepath.Base(file)
		content += fmt.Sprintf("### %s\n\n", strings.TrimSuffix(filename, ".md"))
		content += string(data) + "\n\n"
	}

	return content
}

// Test file generation methods

func (fm *FileMigrator) generateOutlineFromD4(d4Content string) string {
	outline := `# Test Outline

**Generated from**: D4-requirements.md
**Date**: ` + time.Now().Format("2006-01-02") + `

## Acceptance Criteria

`

	// Extract sections that look like requirements or acceptance criteria
	lines := strings.Split(d4Content, "\n")
	inRequirements := false
	acCounter := 1

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect requirements sections
		if strings.Contains(strings.ToLower(trimmed), "requirement") ||
			strings.Contains(strings.ToLower(trimmed), "acceptance") {
			inRequirements = true
			continue
		}

		// Extract bullet points or numbered items as acceptance criteria
		if inRequirements && (strings.HasPrefix(trimmed, "-") ||
			strings.HasPrefix(trimmed, "*") ||
			(len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && trimmed[1] == '.')) {

			// Clean up the line
			cleaned := strings.TrimLeft(trimmed, "-*0123456789. ")
			if cleaned != "" {
				outline += fmt.Sprintf("**AC%d**: %s\n", acCounter, cleaned)
				acCounter++
			}
		}
	}

	// If no criteria found, add template
	if acCounter == 1 {
		outline += "**AC1**: System meets functional requirements\n"
		outline += "**AC2**: System meets non-functional requirements\n"
		outline += "**AC3**: All edge cases handled\n"
	}

	return outline
}

// Cleanup removes old V1 phase files after successful migration
func (fm *FileMigrator) Cleanup() error {
	// Move old files to a backup directory
	backupDir := filepath.Join(fm.projectDir, ".wayfinder-v1-backup")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Backup S4, S5, S9, S10 files
	patterns := []string{"S4-*.md", "S5-*.md", "S9-*.md", "S10-*.md"}
	for _, pattern := range patterns {
		files, err := filepath.Glob(filepath.Join(fm.projectDir, pattern))
		if err != nil {
			continue
		}

		for _, file := range files {
			filename := filepath.Base(file)
			backupPath := filepath.Join(backupDir, filename)
			if err := os.Rename(file, backupPath); err != nil {
				return fmt.Errorf("failed to backup %s: %w", filename, err)
			}
		}
	}

	return nil
}

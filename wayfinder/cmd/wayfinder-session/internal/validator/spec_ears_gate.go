package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/spec-governance/earslint"
)

// earsConfigFile is the optional per-project override for EARS patterns. When
// present in the project directory, its patterns replace the built-in defaults.
const earsConfigFile = ".earslint.yml"

// validateSpecEARS gates the canonical SPEC phase by validating the phase
// deliverable against the
// EARS (Easy Approach to Requirements Syntax) templates.
//
// This is the deterministic replacement for the previous LLM-as-judge rubric
// (the Python review-spec skill): instead of asking a model to score prose, we
// require that every requirement be written in a checkable EARS form and that
// the document contain at least one valid requirement. The check runs in strict
// mode, so any "shall"-style requirement that does not conform to an EARS
// template fails the gate.
func validateSpecEARS(projectDir, phaseName string) error {
	const docFile = "SPEC-solution-requirements.md"
	docPath := filepath.Join(projectDir, docFile)

	// Existence check (mirrors the previous gate's contract/message).
	if _, err := os.Stat(docPath); err != nil {
		if os.IsNotExist(err) {
			return NewValidationError(
				"complete "+phaseName,
				fmt.Sprintf("%s does not exist", docFile),
				fmt.Sprintf("Create %s with EARS-formatted requirements before completing %s", docFile, phaseName),
			)
		}
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to check %s: %v", docFile, err),
			"Check file permissions and try again",
		)
	}

	// Reuse the shared size guard.
	if err := validateDocFileSize(docPath, phaseName, docFile); err != nil {
		return err
	}

	// Load per-project pattern overrides if present, else use defaults.
	cfg := earslint.DefaultConfig()
	cfgPath := filepath.Join(projectDir, earsConfigFile)
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		loaded, err := earslint.LoadConfig(cfgPath)
		if err != nil {
			return NewValidationError(
				"complete "+phaseName,
				fmt.Sprintf("invalid %s: %v", earsConfigFile, err),
				fmt.Sprintf("Fix the EARS config at %s, or remove it to use the built-in patterns", cfgPath),
			)
		}
		cfg = loaded
	} else if !os.IsNotExist(statErr) {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to check EARS config %s: %v", cfgPath, statErr),
			"Check file permissions and try again",
		)
	}

	linter, err := earslint.New(cfg)
	if err != nil {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("invalid EARS configuration: %v", err),
			"Fix the EARS patterns and try again",
		)
	}

	res, err := linter.LintFile(docPath)
	if err != nil {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to lint %s: %v", docFile, err),
			"Check file permissions and try again",
		)
	}

	// Strict gate: fail on zero valid requirements or any non-conforming one.
	if res.Failed(true) {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("%s has %d valid EARS requirement(s) and %d non-conforming",
				docFile, res.ValidRequirements, res.NonConforming()),
			formatEARSFix(res, cfg),
		)
	}

	fmt.Fprintf(os.Stderr, "✓ %s EARS check passed (%d valid requirements)\n", docFile, res.ValidRequirements)
	return nil
}

// formatEARSFix builds an actionable help message listing the offending lines
// and the accepted EARS templates, so authors can correct the SPEC directly.
func formatEARSFix(res earslint.Result, cfg earslint.Config) string {
	var b strings.Builder
	if res.ValidRequirements == 0 {
		b.WriteString("SPEC-solution-requirements.md must contain at least one EARS-formatted requirement.\n")
	}
	if res.NonConforming() > 0 {
		b.WriteString("Rewrite these requirements to match an EARS template:\n")
		for _, f := range res.Findings {
			if f.Line > 0 && f.Text != "" {
				fmt.Fprintf(&b, "  - line %d: %s\n", f.Line, f.Text)
			}
		}
	}
	b.WriteString("\nAccepted EARS templates:\n")
	for _, p := range cfg.Patterns {
		if p.Description != "" {
			fmt.Fprintf(&b, "  - %s: %s\n", p.Name, p.Description)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

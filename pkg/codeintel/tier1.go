package codeintel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Tier1Result holds results from all Tier 1 checks.
type Tier1Result struct {
	Tier      int           `json:"tier"`
	Languages []string      `json:"languages"`
	Checks    []CheckResult `json:"checks"`
}

// ASTGrepFinding represents a single finding from ast-grep JSON output.
type ASTGrepFinding struct {
	File     string `json:"file"`
	Text     string `json:"text"`
	RuleID   string `json:"ruleId"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Range    struct {
		Start struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"start"`
		End struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"end"`
	} `json:"range"`
	MetaVariables struct {
		Single map[string]struct {
			Text string `json:"text"`
		} `json:"single"`
	} `json:"metaVariables"`
}

// rulesDir returns the base directory that contains the "rules/" subdirectory.
// ASTGrepRulesDir values like "rules/go" are joined with this base, so the
// returned path must NOT include "rules/" itself.
func rulesDir() string {
	// Check environment override first.
	if d := os.Getenv("CODEINTEL_RULES_DIR"); d != "" {
		return d
	}
	// Try relative to executable.
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "..", "pkg", "codeintel")
		if info, err := os.Stat(filepath.Join(candidate, "rules")); err == nil && info.IsDir() {
			return candidate
		}
	}
	// Fallback: extract embedded rules to a temp directory.
	if d, err := embeddedRulesDir(); err == nil {
		return d
	}
	// Last resort: relative to cwd (useful in tests and dev).
	return "."
}

// RunAstGrepRules runs ast-grep with the specified rule file against the
// given paths and returns parsed findings.
func RunAstGrepRules(ruleFile string, paths []string) ([]ASTGrepFinding, error) {
	astGrep, err := exec.LookPath("ast-grep")
	if err != nil {
		return nil, fmt.Errorf("ast-grep not found: %w", err)
	}

	args := []string{"scan", "--rule", ruleFile, "--json=compact"}
	args = append(args, paths...)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, astGrep, args...)
	out, err := cmd.Output()
	if err != nil {
		// ast-grep exits 1 when findings exist but still outputs JSON.
		if len(out) == 0 {
			return nil, fmt.Errorf("ast-grep failed: %w", err)
		}
	}

	if len(out) == 0 || string(out) == "[]" {
		return nil, nil
	}

	var findings []ASTGrepFinding
	if err := json.Unmarshal(out, &findings); err != nil {
		return nil, fmt.Errorf("parsing ast-grep output: %w", err)
	}
	return findings, nil
}

// CheckDeadCodeTier1 uses ast-grep dead-function rules instead of regex.
// It scans changed files for function definitions, then cross-references
// with the broader project to filter out functions that are actually used.
func CheckDeadCodeTier1(cwd string, languages []LanguageSpec, changedFiles []string) CheckResult {
	result := CheckResult{
		Check:    "dead_code",
		Passed:   true,
		Severity: "warning",
		Message:  "No potentially dead functions found in changed files (Tier 1: AST)",
	}

	if len(changedFiles) == 0 {
		return result
	}

	rd := rulesDir()
	specByExt := buildExtMap(languages)

	// Group changed files by language for batch processing.
	langFiles := make(map[string][]string)
	langSpecMap := make(map[string]LanguageSpec)
	for _, cf := range changedFiles {
		absPath := cf
		if !filepath.IsAbs(cf) {
			absPath = filepath.Join(cwd, cf)
		}
		ext := filepath.Ext(absPath)
		spec, ok := specByExt[ext]
		if !ok || spec.ASTGrepLang == "" {
			continue
		}
		langFiles[spec.Name] = append(langFiles[spec.Name], absPath)
		langSpecMap[spec.Name] = spec
	}

	var suspects []string
	for langName, files := range langFiles {
		spec := langSpecMap[langName]
		ruleFile := filepath.Join(rd, spec.ASTGrepRulesDir, "dead-function.yml")
		if _, err := os.Stat(ruleFile); err != nil {
			continue
		}

		findings, err := RunAstGrepRules(ruleFile, files)
		if err != nil {
			continue
		}

		for _, f := range findings {
			funcName := ""
			if v, ok := f.MetaVariables.Single["FUNC"]; ok {
				funcName = v.Text
			}
			if funcName == "" {
				continue
			}
			if entryPoints[funcName] {
				continue
			}

			// Cross-reference: check if function is used elsewhere in the project.
			absFile := f.File
			if !filepath.IsAbs(absFile) {
				absFile = filepath.Join(cwd, absFile)
			}
			if !hasReferencesElsewhere(cwd, funcName, absFile, spec) {
				relFile := relPath(cwd, absFile)
				suspects = append(suspects, fmt.Sprintf("%s:%d: %s()", relFile, f.Range.Start.Line+1, funcName))
			}
		}
	}

	if len(suspects) > 0 {
		result.Passed = false
		result.Message = fmt.Sprintf("%d potentially dead function(s) found (Tier 1: AST-aware, lower false-positive rate)", len(suspects))
		result.Details = suspects
	}
	return result
}

// CheckPatterns runs all ast-grep pattern rules (except dead-function)
// against changed files. This includes debug-print, no-bare-except,
// no-any-type, and any other rules in the language's rules directory.
func CheckPatterns(cwd string, languages []LanguageSpec, changedFiles []string) CheckResult {
	result := CheckResult{
		Check:    "debug_prints",
		Passed:   true,
		Severity: "warning",
		Message:  "No pattern violations found in changed files (Tier 1: AST)",
	}

	if len(changedFiles) == 0 {
		return result
	}

	rd := rulesDir()
	specByExt := buildExtMap(languages)

	// Group changed files by language.
	langFiles := make(map[string][]string)
	langSpecMap := make(map[string]LanguageSpec)
	for _, cf := range changedFiles {
		absPath := cf
		if !filepath.IsAbs(cf) {
			absPath = filepath.Join(cwd, cf)
		}
		ext := filepath.Ext(absPath)
		spec, ok := specByExt[ext]
		if !ok || spec.ASTGrepLang == "" {
			continue
		}
		langFiles[spec.Name] = append(langFiles[spec.Name], absPath)
		langSpecMap[spec.Name] = spec
	}

	var details []string
	for langName, files := range langFiles {
		spec := langSpecMap[langName]
		langRulesDir := filepath.Join(rd, spec.ASTGrepRulesDir)

		// Scan all .yml rule files in the language directory,
		// excluding dead-function.yml (handled by CheckDeadCodeTier1).
		ruleFiles, err := filepath.Glob(filepath.Join(langRulesDir, "*.yml"))
		if err != nil || len(ruleFiles) == 0 {
			continue
		}

		for _, ruleFile := range ruleFiles {
			if filepath.Base(ruleFile) == "dead-function.yml" {
				continue
			}

			findings, err := RunAstGrepRules(ruleFile, files)
			if err != nil {
				continue
			}

			for _, f := range findings {
				absFile := f.File
				if !filepath.IsAbs(absFile) {
					absFile = filepath.Join(cwd, absFile)
				}
				relFile := relPath(cwd, absFile)
				details = append(details, fmt.Sprintf("%s:%d: %s", relFile, f.Range.Start.Line+1, strings.TrimSpace(f.Text)))
			}
		}
	}

	if len(details) > 0 {
		result.Passed = false
		result.Message = fmt.Sprintf("%d pattern violation(s) found in changed files (Tier 1: AST)", len(details))
		result.Details = details
	}
	return result
}

// AstGrepAvailable returns true if ast-grep is installed and on PATH.
func AstGrepAvailable() bool {
	return commandExists("ast-grep")
}

// RunTier1Checks detects languages and runs Tier 1 AST-based checks,
// falling back to Tier 0 for checks that don't have AST rules.
func RunTier1Checks(cwd string, changedFiles []string) (*Tier1Result, error) {
	reg, err := NewRegistry(cwd)
	if err != nil {
		return nil, fmt.Errorf("loading registry: %w", err)
	}

	languages := reg.DetectLanguages(cwd)
	if len(languages) == 0 {
		return &Tier1Result{
			Tier: 1,
			Checks: []CheckResult{{
				Check:    "detect_languages",
				Passed:   true,
				Severity: "info",
				Message:  "No languages detected",
			}},
		}, nil
	}

	if len(changedFiles) == 0 {
		changedFiles = detectChangedFiles(cwd)
	}

	var names []string
	for _, l := range languages {
		names = append(names, l.Name)
	}

	result := &Tier1Result{
		Tier:      1,
		Languages: names,
		Checks: []CheckResult{
			CheckDeadCodeTier1(cwd, languages, changedFiles),
			CheckDanglingRefsScoped(cwd, languages, changedFiles),
			CheckPatterns(cwd, languages, changedFiles),
		},
	}
	return result, nil
}

// RunChecks runs the best available tier of checks. If ast-grep is installed,
// it runs Tier 1; otherwise it falls back to Tier 0.
func RunChecks(cwd string, changedFiles []string, requestedTier int) (*Tier1Result, error) {
	// If tier 1 explicitly requested, warn if ast-grep is missing.
	if requestedTier >= 1 && !AstGrepAvailable() {
		fmt.Fprintf(os.Stderr, "WARNING: --tier 1 requested but ast-grep is not installed; falling back to Tier 0\n")
		fmt.Fprintf(os.Stderr, "  Install: https://ast-grep.github.io/guide/quick-start.html\n")
	}

	// If tier 1 explicitly requested or auto-detected, and ast-grep is available.
	if (requestedTier >= 1 || requestedTier < 0) && AstGrepAvailable() {
		return RunTier1Checks(cwd, changedFiles)
	}

	// Fall back to Tier 0.
	t0, err := RunTier0Checks(cwd, changedFiles)
	if err != nil {
		return nil, err
	}
	return &Tier1Result{
		Tier:      0,
		Languages: t0.Languages,
		Checks:    t0.Checks,
	}, nil
}

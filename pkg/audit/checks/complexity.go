package checks

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// ComplexityCheck flags Go functions whose cyclomatic complexity exceeds
// a configurable threshold. Per ROADMAP Phase 6.2, code that passes
// tests but is incomprehensible is a security risk: future readers
// (human or AI) cannot spot defects in code they cannot model. The
// check runs over the working tree's .go sources, skipping vendor,
// _test.go, and generated files.
//
// Severity is P2 — drift, not breakage. The remediation is "split or
// simplify the function," not "fail the run," so the check ships with
// StrategyNoop; severity-policy in .dear-agent.yml decides whether to
// surface it.
//
// Config knobs:
//   - max_complexity: int — threshold above which a function is flagged.
//     Default 15. McCabe's original paper recommends 10; we choose 15
//     because it matches gocyclo's common production setting and is
//     compatible with idiomatic dispatch tables.
//   - include_tests: bool — when true, also score _test.go files.
//     Default false: tests are allowed to be more branchy.
type ComplexityCheck struct{}

// Default and minimum thresholds. The minimum prevents an operator from
// accidentally setting a 0 or negative threshold that would emit one
// finding per function in the repo.
const (
	defaultComplexityThreshold = 15
	minComplexityThreshold     = 1
)

// Meta returns the check's identity. The ID matches ADR-011's pre-named
// slot and the recommended cadence is monthly per the ADR's cost
// classification — operators can promote to a faster cadence in
// .dear-agent.yml when the static-analysis pass is cheap enough on
// their tree.
func (ComplexityCheck) Meta() audit.CheckMeta {
	return audit.CheckMeta{
		ID:              "complexity",
		Description:     "Go functions must stay below the configured cyclomatic complexity threshold",
		Cadence:         audit.CadenceMonthly,
		SeverityCeiling: audit.SeverityP2,
	}
}

// Run walks env.WorkingDir for .go files, parses each, and emits one
// finding per function whose cyclomatic complexity exceeds the
// threshold. Parse errors are recorded on Result.Stderr but do not
// fail the run — a broken file is the build check's concern.
func (ComplexityCheck) Run(ctx context.Context, env audit.Env) (audit.Result, error) {
	threshold := defaultComplexityThreshold
	if v, ok := env.Config["max_complexity"].(int); ok && v >= minComplexityThreshold {
		threshold = v
	}
	includeTests, _ := env.Config["include_tests"].(bool)

	out := audit.Result{Status: audit.StatusOK}

	var (
		findings []audit.Finding
		errs     []string
	)
	walkErr := filepath.WalkDir(env.WorkingDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !includeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fileFindings, perr := scoreGoFile(path, env.WorkingDir, threshold)
		if perr != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", path, perr))
			return nil
		}
		findings = append(findings, fileFindings...)
		return nil
	})
	if walkErr != nil {
		if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
			out.Status = audit.StatusTimeout
			return out, fmt.Errorf("audit/complexity: walk: %w", walkErr)
		}
		out.Status = audit.StatusError
		return out, fmt.Errorf("audit/complexity: walk %s: %w", env.WorkingDir, walkErr)
	}

	out.Findings = findings
	if len(errs) > 0 {
		out.Stderr = strings.Join(errs, "\n")
	}
	return out, nil
}

// shouldSkipDir returns true for directories the check should not
// descend into. We skip vendor and the well-known toolchain caches; a
// .git tree contains no .go source we care about.
func shouldSkipDir(name string) bool {
	switch name {
	case "vendor", "node_modules", ".git", "testdata":
		return true
	}
	return false
}

// scoreGoFile parses one .go file and returns one finding per
// top-level FuncDecl whose complexity exceeds threshold. The path
// recorded on the finding is relative to repoRoot when the file lives
// under it; absolute otherwise.
func scoreGoFile(path, repoRoot string, threshold int) ([]audit.Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	if isGenerated(file) {
		return nil, nil
	}

	relPath := path
	if r, rerr := filepath.Rel(repoRoot, path); rerr == nil {
		relPath = r
	}

	var findings []audit.Finding
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		complexity := cyclomaticComplexity(fn)
		if complexity <= threshold {
			continue
		}
		name := funcDisplayName(fn)
		line := fset.Position(fn.Pos()).Line
		findings = append(findings, audit.Finding{
			Fingerprint: audit.Fingerprint("complexity", relPath, name),
			Severity:    audit.SeverityP2,
			Title: fmt.Sprintf(
				"%s has cyclomatic complexity %d (> %d)",
				name, complexity, threshold,
			),
			Detail: fmt.Sprintf(
				"Function %s in %s:%d has cyclomatic complexity %d, above the configured threshold of %d. "+
					"High-complexity functions are harder to review for correctness and security; "+
					"consider extracting branches into helper functions or replacing dispatch logic with a table.",
				name, relPath, line, complexity, threshold,
			),
			Path: relPath,
			Line: line,
			Suggested: audit.Remediation{
				Strategy: audit.StrategyNoop,
			},
			Evidence: map[string]any{
				"function":   name,
				"complexity": complexity,
				"threshold":  threshold,
			},
		})
	}
	return findings, nil
}

// cyclomaticComplexity computes the McCabe complexity of a function
// declaration. We count: 1 base, then +1 per if, for, range, switch
// case, type-switch case, select case (CommClause), and per &&/||
// short-circuit operator. This matches gocyclo's algorithm and the
// definition in McCabe's 1976 paper.
//
// We deliberately do NOT count `else` (it's the same decision point
// as the matching `if`) or `default` clauses (they are the fall-through
// of an existing decision point, not a new one).
func cyclomaticComplexity(fn *ast.FuncDecl) int {
	complexity := 1
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			complexity++
		case *ast.CaseClause:
			// Empty case list is the `default:` clause; it is not a new
			// decision point so we do not count it.
			if len(v.List) > 0 {
				complexity++
			}
		case *ast.CommClause:
			// `default:` arm of a select statement: List is nil for the
			// default case in this AST shape.
			if v.Comm != nil {
				complexity++
			}
		case *ast.BinaryExpr:
			if v.Op == token.LAND || v.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

// funcDisplayName returns "Type.Method" for methods, "Func" for
// top-level functions. It is purely for human-readable titles; the
// fingerprint uses the same string so two methods on different types
// with the same name do not collide.
func funcDisplayName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recvType := receiverTypeName(fn.Recv.List[0].Type)
	if recvType == "" {
		return fn.Name.Name
	}
	return recvType + "." + fn.Name.Name
}

// receiverTypeName extracts the bare type name of a method receiver,
// stripping pointer indirection. Generic receivers (`T[X]`) return
// just `T`.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	case *ast.IndexListExpr:
		return receiverTypeName(t.X)
	}
	return ""
}

// isGenerated reports whether file carries the standard "Code
// generated ... DO NOT EDIT." marker in its leading comments. We
// follow the rule from `go doc go/build` §"Build constraints" and
// the convention codified by the Go community.
func isGenerated(file *ast.File) bool {
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			text = strings.TrimSpace(strings.TrimPrefix(text, "/*"))
			text = strings.TrimSpace(strings.TrimSuffix(text, "*/"))
			if strings.HasPrefix(text, "Code generated") && strings.Contains(text, "DO NOT EDIT") {
				return true
			}
		}
		// The marker must appear before the package clause, so once we
		// pass the first comment group attached above the package decl
		// we can stop looking.
		if file.Package != token.NoPos && cg.End() >= file.Package {
			break
		}
	}
	return false
}

func init() {
	audit.Default.MustRegister(ComplexityCheck{})
}

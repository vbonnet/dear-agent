package craplens

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
)

// changedFunctions returns every function declaration in the touched files
// whose body overlaps a line the diff wrote.
//
// Source is read from the head revision rather than the working tree. The diff
// line numbers are head-side, so parsing whatever happens to be checked out
// would attribute those lines to whichever functions occupy them there. In CI
// the checkout is the head and the two agree, but a local run against an
// arbitrary revision pair must not silently report the wrong functions.
//
// A file that does not parse is skipped rather than reported: this signal runs
// alongside the build, which is the gate that owns a syntax error.
func changedFunctions(ctx context.Context, repoDir, head string, files touchedSet) []Function {
	var out []Function

	dropGeneratedFiles(ctx, repoDir, head, files)

	for _, rel := range sortedKeys(files) {
		tf := files[rel]
		src, err := fileAt(ctx, repoDir, head, rel)
		if err != nil {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, rel, src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}

		for _, decl := range parsed.Decls {
			for _, cand := range declaredFuncs(decl) {
				// PositionFor with adjusted=false: a //line directive would
				// otherwise move a declaration to a logical line while the
				// git hunk ranges stay physical, so the overlap check would
				// miss the changed function entirely.
				startPos := fset.PositionFor(cand.node.Pos(), false)
				endPos := fset.PositionFor(cand.node.End(), false)
				if !overlaps(tf.Ranges, startPos.Line, endPos.Line) {
					continue
				}
				out = append(out, Function{
					Package:    dirOf(rel),
					File:       rel,
					Name:       cand.name,
					Line:       startPos.Line,
					StartCol:   startPos.Column,
					EndLine:    endPos.Line,
					EndCol:     endPos.Column,
					Complexity: complexity(cand.node),
				})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// candidate is one scorable function body found at the top level of a file.
type candidate struct {
	name string
	node ast.Node
}

// declaredFuncs returns the scorable function bodies a top-level declaration
// introduces.
//
// Ordinary declarations are one function each. A package-level variable
// holding a function literal is also one: those are this repository's
// injection seams, and a seam that grows a branchy body would otherwise be
// invisible to this signal while gocyclo reports it. Counting them is also
// what keeps the two tools agreeing about the same file.
func declaredFuncs(decl ast.Decl) []candidate {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Body == nil {
			return nil
		}
		return []candidate{{name: funcName(d), node: d}}
	case *ast.GenDecl:
		if d.Tok != token.VAR {
			return nil
		}
		var out []candidate
		for _, spec := range d.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, value := range vs.Values {
				lit, ok := value.(*ast.FuncLit)
				if !ok || i >= len(vs.Names) {
					continue
				}
				out = append(out, candidate{name: vs.Names[i].Name, node: lit})
			}
		}
		return out
	default:
		return nil
	}
}

// dropGeneratedFiles removes marker-generated files from the touched set.
//
// Removed rather than skipped: leaving them in means packagesOf and
// classifyPackages still see the package, so a PR that changes only a
// generated file would have its package reported as untested even though
// CRAPLENS-02 excludes the only file that put it there.
func dropGeneratedFiles(ctx context.Context, repoDir, head string, files touchedSet) {
	for _, rel := range sortedKeys(files) {
		src, err := fileAt(ctx, repoDir, head, rel)
		if err != nil {
			continue
		}
		if isGeneratedSource(src) {
			delete(files, rel)
		}
	}
}

// funcName renders a declaration's name, qualified by receiver type for a
// method so two same-named methods stay distinguishable in the report.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return fmt.Sprintf("(%s).%s", recvTypeName(fn.Recv.List[0].Type), fn.Name.Name)
}

func recvTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return "*" + recvTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr: // generic receiver, e.g. Store[T]
		return recvTypeName(t.X)
	case *ast.IndexListExpr:
		return recvTypeName(t.X)
	default:
		return "?"
	}
}

// complexity computes McCabe cyclomatic complexity the way gocyclo does: one
// for the function's single entry, plus one for each branch point.
//
// Matching gocyclo matters because golangci-lint already runs gocyclo on this
// repository at min-complexity 15. If this package counted differently, the
// two signals would disagree about the same function and readers would learn
// to trust neither.
func complexity(fn ast.Node) int {
	score := 1
	ast.Inspect(fn, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt:
			score++
		case *ast.CaseClause:
			// A `default:` clause has a nil List and adds no branch: control
			// reaches it precisely when no other case matched. gocyclo skips
			// it for the same reason, and counting it here would report a
			// higher number than the linter does for the same function.
			if node.List != nil {
				score++
			}
		case *ast.CommClause:
			// Likewise for `default:` in a select.
			if node.Comm != nil {
				score++
			}
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				score++
			}
		}
		return true
	})
	return score
}

// dirOf returns the directory portion of a repository-relative path, which is
// how this package keys a Go package.
func dirOf(rel string) string {
	for i := len(rel) - 1; i >= 0; i-- {
		if rel[i] == '/' {
			return rel[:i]
		}
	}
	return "."
}

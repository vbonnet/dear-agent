package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/internal/repoinventory"
)

// walkRepoFiles invokes fn for every governed file under root. The shared
// inventory owns Git-ignore and generated/dependency directory policy so repo
// health cannot drift from SPEC and EARS tooling.
func walkRepoFiles(root string, fn func(path string)) error {
	files, err := repoinventory.Scan(root)
	if err != nil {
		return err
	}
	for _, file := range files {
		fn(file.Absolute)
	}
	return nil
}

// goFileWalk invokes fn for every non-excluded .go file under root.
func goFileWalk(root string, fn func(path string)) error {
	return walkRepoFiles(root, func(path string) {
		if strings.HasSuffix(path, ".go") {
			fn(path)
		}
	})
}

// goSource holds the parsed form of one Go file plus its line count.
type goSource struct {
	path   string
	fset   *token.FileSet
	file   *ast.File
	lines  int
	isTest bool
}

// parseGoFiles parses every non-excluded Go file under root. Files that
// fail to parse are skipped (with their path returned) rather than aborting
// — a single syntactically-broken file should not blind the whole audit.
func parseGoFiles(root string) (sources []goSource, skipped []string) {
	err := goFileWalk(root, func(path string) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			skipped = append(skipped, path)
			return
		}
		lines := 0
		if tf := fset.File(f.Pos()); tf != nil {
			lines = tf.LineCount()
		}
		sources = append(sources, goSource{
			path:   path,
			fset:   fset,
			file:   f,
			lines:  lines,
			isTest: strings.HasSuffix(path, "_test.go"),
		})
	})
	if err != nil {
		skipped = append(skipped, root+" (walk error: "+err.Error()+")")
	}
	return sources, skipped
}

// cyclomatic returns the cyclomatic complexity of a function body: 1 plus
// one for every branch point (if/for/case/&&/||/etc.). This is the
// standard McCabe approximation used by gocyclo.
func cyclomatic(fn *ast.FuncDecl) int {
	complexity := 1
	ast.Inspect(fn, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt,
			*ast.CaseClause, *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if s.Op == token.LAND || s.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

// funcLineSpan returns the number of source lines a function declaration
// occupies, inclusive of its signature and closing brace.
func funcLineSpan(fset *token.FileSet, fn *ast.FuncDecl) int {
	start := fset.Position(fn.Pos()).Line
	end := fset.Position(fn.End()).Line
	if end < start {
		return 0
	}
	return end - start + 1
}

// relTo returns path relative to root, falling back to the absolute path.
func relTo(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}
	return path
}

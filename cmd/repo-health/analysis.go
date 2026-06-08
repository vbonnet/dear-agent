package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// excludedDirs are directories never counted toward repo health: build
// artifacts, vendored third-party code, and the worktree pool (which holds
// copies of the tree that would otherwise double-count every metric).
var excludedDirs = map[string]bool{
	".git":        true,
	".worktrees":  true,
	"vendor":      true,
	"build":       true,
	"node_modules": true,
	"testdata":    true,
}

// walkRepoFiles invokes fn for every non-excluded regular file under root.
func walkRepoFiles(root string, fn func(path string)) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			//nolint:nilerr // an unreadable entry is skipped, not fatal: a
			// single permission error must not blind the whole audit.
			return nil
		}
		if d.IsDir() {
			if path != root && excludedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		fn(path)
		return nil
	})
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
	path  string
	fset  *token.FileSet
	file  *ast.File
	lines int
	isTest bool
}

// parseGoFiles parses every non-excluded Go file under root. Files that
// fail to parse are skipped (with their path returned) rather than aborting
// — a single syntactically-broken file should not blind the whole audit.
func parseGoFiles(root string) (sources []goSource, skipped []string) {
	_ = goFileWalk(root, func(path string) {
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

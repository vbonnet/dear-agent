package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestProductionResumeEntryPointsUseSharedResumeOperation(t *testing.T) {
	for _, path := range []string{"last.go", "resume_all.go"} {
		t.Run(path, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			lockedCalls := 0
			unlockedCalls := 0
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				ident, ok := call.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				switch ident.Name {
				case "resumeResolvedSession":
					lockedCalls++
				case "resumeSession":
					unlockedCalls++
				}
				return true
			})
			if unlockedCalls != 0 || lockedCalls == 0 {
				t.Fatalf("resume routing in %s = (shared=%d, legacy=%d), want shared>0 and legacy=0", path, lockedCalls, unlockedCalls)
			}
		})
	}
}

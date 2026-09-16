package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestStartupSourceDoesNotDuplicateToolInventory(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	tools := registeredMCPTools(t, registerMCPTools)
	ast.Inspect(file, func(node ast.Node) bool {
		value, ok := stringLiteral(node)
		if !ok {
			return true
		}
		for _, tool := range tools {
			if strings.Contains(value, tool.Name) {
				t.Errorf("main.go contains compiled tool name %q in a string literal; use MCP discovery instead", tool.Name)
			}
		}
		return true
	})
}

func stringLiteral(node ast.Node) (string, bool) {
	literal, ok := node.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

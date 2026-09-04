package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestStartupRegistrationLogDoesNotDuplicateToolInventory(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	var mainBody *ast.BlockStmt
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "main" {
			mainBody = fn.Body
			break
		}
	}
	if mainBody == nil {
		t.Fatal("main.go has no main function body")
	}

	var registrationPositions []token.Pos
	var receiptCalls []*ast.CallExpr
	ast.Inspect(mainBody, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "registerMCPTools" {
			registrationPositions = append(registrationPositions, call.Pos())
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Info" {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || receiver.Name != "logger" || len(call.Args) == 0 {
			return true
		}
		message, ok := stringLiteral(call.Args[0])
		if ok && message == "Registered MCP tools" {
			receiptCalls = append(receiptCalls, call)
		}
		return true
	})
	if len(registrationPositions) != 1 {
		t.Fatalf("registerMCPTools calls in main = %d, want exactly 1", len(registrationPositions))
	}
	if len(receiptCalls) != 1 {
		t.Fatalf("Registered MCP tools receipts in main = %d, want exactly 1", len(receiptCalls))
	}
	receipt := receiptCalls[0]
	if receipt.Pos() <= registrationPositions[0] {
		t.Fatal("Registered MCP tools receipt must follow production registration")
	}
	if len(receipt.Args) != 1 {
		t.Errorf("Registered MCP tools receipt has %d arguments, want only its message", len(receipt.Args))
	}

	tools := registeredMCPTools(t, registerMCPTools)
	ast.Inspect(mainBody, func(node ast.Node) bool {
		value, ok := stringLiteral(node)
		if !ok {
			return true
		}
		for _, tool := range tools {
			if strings.Contains(value, tool.Name) {
				t.Errorf("main contains compiled tool name %q in a string literal; use MCP discovery instead", tool.Name)
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

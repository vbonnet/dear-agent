package buildauthority

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNonSourceProcessOwnerHasExactlyFiveNominalCommands(t *testing.T) {
	owner := reflect.TypeFor[nonSourceProcessOwner]()
	want := map[string]reflect.Type{
		"runGoVersion": reflect.TypeOf(
			(func(goVersionPlan, *nonSourceCommandWindow) processResult)(nil),
		),
		"runGoEnvironment": reflect.TypeOf(
			(func(goEnvironmentPlan, *nonSourceCommandWindow) processResult)(nil),
		),
		"runCompilerVersion": reflect.TypeOf(
			(func(compilerVersionPlan, *nonSourceCommandWindow) processResult)(nil),
		),
		"runGitVersion": reflect.TypeOf(
			(func(gitVersionPlan, *nonSourceCommandWindow) processResult)(nil),
		),
		"runGitBuiltinInventory": reflect.TypeOf(
			(func(gitBuiltinInventoryPlan, *nonSourceCommandWindow) processResult)(nil),
		),
	}
	if owner.NumMethod() != len(want)+1 {
		t.Fatalf("non-source process owner methods = %d, want five commands plus seal", owner.NumMethod())
	}
	for name, wantType := range want {
		method, ok := owner.MethodByName(name)
		if !ok || method.Type != wantType {
			t.Fatalf("non-source process owner %s = %v, want %v", name, method.Type, wantType)
		}
		for input := range method.Type.Ins() {
			if input == reflect.TypeFor[context.Context]() || input == reflect.TypeFor[time.Time]() ||
				input.Kind() == reflect.String || input == reflect.TypeFor[processRequest]() ||
				input == reflect.TypeFor[processSupervisor]() {
				t.Fatalf("non-source process owner %s accepts raw authority %s", name, input)
			}
		}
	}
	if _, ok := owner.MethodByName("privateNonSourceProcessOwner"); !ok {
		t.Fatal("non-source process owner is missing its private seal")
	}
}

func TestNonSourceRunnerHasNoProcessOrWorkspaceBypass(t *testing.T) {
	source, err := os.ReadFile("preflight_runner.go")
	if err != nil {
		t.Fatalf("read preflight runner: %v", err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "preflight_runner.go", source, 0)
	if err != nil {
		t.Fatalf("parse preflight runner: %v", err)
	}
	forbiddenIdentifiers := map[string]bool{
		"processRequest":          true,
		"processSupervisor":       true,
		"taskWorkspace":           true,
		"allocateWorkspace":       true,
		"allocateWorkspaceWith":   true,
		"createTaskWorkspaceWith": true,
		"Mkdir":                   true,
		"MkdirAll":                true,
		"Mkdirat":                 true,
		"Command":                 true,
		"CommandContext":          true,
		"ForkExec":                true,
		"Exec":                    true,
		"PosixSpawn":              true,
		"runPreallocation":        true,
		"runTaskPrivate":          true,
		"newProcessSupervisor":    true,
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && forbiddenIdentifiers[identifier.Name] {
			t.Fatalf("preflight runner contains bypass identifier %s", identifier.Name)
		}
		return true
	})

	block := reflect.TypeFor[nonSourceBlock]()
	for field := range block.Fields() {
		if field.Type == reflect.TypeFor[processSupervisor]() ||
			field.Type == reflect.TypeFor[processRequest]() ||
			field.Type == reflect.TypeFor[*taskWorkspace]() ||
			field.Type.Kind() == reflect.Func {
			t.Fatalf("non-source block field %s exposes bypass type %s", field.Name, field.Type)
		}
	}
}

func TestNonSourceRunnerOwnsLiteralCommandAndValidatorOrder(t *testing.T) {
	source, err := os.ReadFile("preflight_runner.go")
	if err != nil {
		t.Fatalf("read preflight runner: %v", err)
	}
	wire := string(source)
	commands := []string{
		"block.runGoVersion(",
		"block.runGoEnvironment(",
		"block.runCompilerVersion(",
		"block.runGitVersion(",
		"block.runGitBuiltinInventory(",
	}
	validators := []string{
		"result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)",
		"result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)",
		"result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)",
		"result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)",
		"result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)",
	}
	previous := -1
	for _, command := range commands {
		if strings.Count(wire, command) != 1 {
			t.Fatalf("runner command call count for %q = %d", command, strings.Count(wire, command))
		}
		position := strings.Index(wire, command)
		if position <= previous {
			t.Fatalf("runner command %q is out of literal order", command)
		}
		previous = position
	}
	if strings.Count(wire, validators[0]) != len(validators) {
		t.Fatalf("runner validator calls = %d, want %d", strings.Count(wire, validators[0]), len(validators))
	}
}

func TestNonSourceProductionOwnerIsTheOnlyRequestBridge(t *testing.T) {
	source, err := os.ReadFile("process.go")
	if err != nil {
		t.Fatalf("read process owner: %v", err)
	}
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, "process.go", source, 0)
	if err != nil {
		t.Fatalf("parse process owner: %v", err)
	}
	materializers := map[string]bool{
		"runGoVersion":           true,
		"runGoEnvironment":       true,
		"runCompilerVersion":     true,
		"runGitVersion":          true,
		"runGitBuiltinInventory": true,
	}
	runCalls := make(map[string]int, len(materializers))
	ownerLiterals := 0
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.CallExpr:
				selector, ok := current.Fun.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "runPreallocation" {
					if !materializers[function.Name.Name] {
						t.Fatalf("%s calls generic preallocation supervisor", files.Position(current.Pos()))
					}
					runCalls[function.Name.Name]++
				}
			case *ast.CompositeLit:
				identifier, ok := current.Type.(*ast.Ident)
				if ok && identifier.Name == "preallocationNonSourceProcessOwner" {
					if function.Name.Name != "newNonSourceProcessOwner" {
						t.Fatalf("%s constructs independently paired process owner", files.Position(current.Pos()))
					}
					ownerLiterals++
				}
			}
			return true
		})
	}
	for name := range materializers {
		if runCalls[name] != 1 {
			t.Fatalf("materializer %s generic supervisor calls = %d, want 1", name, runCalls[name])
		}
	}
	if ownerLiterals != 1 {
		t.Fatalf("production process-owner literals = %d, want 1 paired owner", ownerLiterals)
	}
}

func TestNonSourceCapabilityTokensHaveSingleProductionMints(t *testing.T) {
	source, err := os.ReadFile("preflight_runner.go")
	if err != nil {
		t.Fatalf("read preflight runner: %v", err)
	}
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, "preflight_runner.go", source, 0)
	if err != nil {
		t.Fatalf("parse preflight runner: %v", err)
	}
	wantPointerLiterals := map[string]int{
		"nonSourceTransactionState":  1,
		"nonSourceAuthorityContext":  1,
		"nonSourceCommandWindow":     1,
		"retainedNullCommandWitness": 1,
		"nonSourceProof":             1,
	}
	gotPointerLiterals := make(map[string]int, len(wantPointerLiterals))
	transactionPublishes := 0
	authorizedTransitions := 0
	consumedTransitions := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.UnaryExpr:
			literal, ok := current.X.(*ast.CompositeLit)
			if !ok || current.Op != token.AND {
				return true
			}
			identifier, ok := literal.Type.(*ast.Ident)
			if ok {
				gotPointerLiterals[identifier.Name]++
			}
		case *ast.AssignStmt:
			for index, left := range current.Lhs {
				selector, ok := left.(*ast.SelectorExpr)
				if !ok || index >= len(current.Rhs) {
					continue
				}
				right, ok := current.Rhs[index].(*ast.Ident)
				if !ok {
					continue
				}
				switch {
				case selector.Sel.Name == "transaction" && right.Name == "state":
					transactionPublishes++
				case selector.Sel.Name == "state" && right.Name == "nonSourceCommandWindowAuthorized":
					authorizedTransitions++
				case selector.Sel.Name == "state" && right.Name == "nonSourceCommandWindowConsumed":
					consumedTransitions++
				}
			}
		}
		return true
	})
	for name, want := range wantPointerLiterals {
		if gotPointerLiterals[name] != want {
			t.Fatalf("production pointer literals for %s = %d, want %d", name, gotPointerLiterals[name], want)
		}
	}
	if transactionPublishes != 1 || authorizedTransitions != 1 || consumedTransitions != 1 {
		t.Fatalf(
			"capability publications = transaction %d authorized %d consumed %d, want 1/1/1",
			transactionPublishes,
			authorizedTransitions,
			consumedTransitions,
		)
	}
}

func TestNonSourceAuthorityDeadlineContextStaysInsideErrPolledA2(t *testing.T) {
	for _, name := range []string{"process.go", "process_darwin.go", "process_unsupported.go"} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(source), "nonSourceAuthorityContext") {
			t.Fatalf("%s lets the A2-only deadline context cross the process seam", name)
		}
	}

	for _, name := range []string{
		"filesystem.go",
		"filesystem_darwin.go",
		"preflight_authority.go",
		"preflight_authority_bracket.go",
		"preflight_authority_darwin.go",
		"preflight_authority_roles.go",
		"preflight_authority_tree.go",
	} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "Done" {
				t.Fatalf("%s consumes context.Done; A2 deadline context is Err-polled", name)
			}
			return true
		})
	}
}

package buildauthority

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSourceModulesHaveNoProcessAllocatorOrAggregateAuthority(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob("source*.go")
	if err != nil {
		t.Fatalf("glob source modules: %v", err)
	}
	// gitconfig.go owns the custom source config and packed-ref parsers. Every
	// other C1 source-envelope implementation file is required to use the
	// source*.go naming boundary so this guard expands with the module.
	paths = append(paths, "gitconfig.go")
	forbiddenIdentifiers := map[string]bool{
		"Command":                            true,
		"CommandContext":                     true,
		"Exec":                               true,
		"ForkExec":                           true,
		"Mkdir":                              true,
		"MkdirAll":                           true,
		"Mkdirat":                            true,
		"PosixSpawn":                         true,
		"StagedPair":                         true,
		"StartProcess":                       true,
		"allocateWorkspace":                  true,
		"allocateWorkspaceWith":              true,
		"capturedNoScratchInputs":            true,
		"createTaskWorkspaceWith":            true,
		"newProcessSupervisor":               true,
		"newProcessSupervisorWithScheduler":  true,
		"newRealPreallocationProcessCommand": true,
		"newRealTaskPrivateProcessCommand":   true,
		"newStagedPair":                      true,
		"nonSourceBlock":                     true,
		"nonSourceProcessOwner":              true,
		"pairOwner":                          true,
		"preallocationNonSourceProcessOwner": true,
		"preallocationProcessCommandFactory": true,
		"processCommand":                     true,
		"processCommandFactory":              true,
		"processCommandSpec":                 true,
		"processInput":                       true,
		"processRequest":                     true,
		"processResult":                      true,
		"processSupervisor":                  true,
		"Receipt":                            true,
		"runPreallocation":                   true,
		"runTaskPrivate":                     true,
		"stagedPair":                         true,
		"taskWorkspace":                      true,
		"taskPrivateProcessCommandFactory":   true,
		"workspaceAllocationDependencies":    true,
	}
	productionFiles := 0
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") || seen[path] {
			continue
		}
		seen[path] = true
		productionFiles++
		files := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(files, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("parse imports for %s: %v", path, parseErr)
		}
		for _, imported := range parsed.Imports {
			name, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("unquote import in %s: %v", path, unquoteErr)
			}
			if name == "os/exec" {
				t.Fatalf("%s imports process construction authority", path)
			}
		}

		parsed, parseErr = parser.ParseFile(files, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse source module %s: %v", path, parseErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && forbiddenIdentifiers[identifier.Name] {
				t.Errorf("%s references forbidden authority %s at %s", path, identifier.Name, files.Position(identifier.Pos()))
			}
			return true
		})
	}
	if productionFiles == 0 {
		t.Fatal("source governance matched no production modules")
	}
}

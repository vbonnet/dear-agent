package audit

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"log/slog"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

type contractRemediator struct {
	outcome ApplyOutcome
}

func (r contractRemediator) Apply(context.Context, Finding, Env) (ApplyOutcome, error) {
	return r.outcome, nil
}

type recordingStateStore struct {
	Store
	calls int
	state FindingState
	note  string
}

func (s *recordingStateStore) SetFindingState(
	_ context.Context,
	findingID string,
	state FindingState,
	note string,
) (Finding, error) {
	s.calls++
	s.state = state
	s.note = note
	return Finding{FindingID: findingID, State: state}, nil
}

func TestRunnerRemediatorOutcomePersistenceBoundary(t *testing.T) {
	stored := Finding{
		FindingID: "finding-1",
		State:     FindingOpen,
		Suggested: Remediation{Strategy: StrategyAuto, Command: "unused"},
	}

	tests := []struct {
		name      string
		outcome   ApplyOutcome
		wantCalls int
		wantState FindingState
		wantNote  string
	}{
		{
			name: "unchanged state drops all descriptive fields",
			outcome: ApplyOutcome{
				Status: "applied", State: FindingOpen,
				Note: "not persisted", Reference: "artifact-ignored",
			},
		},
		{
			name: "invalid state drops all descriptive fields",
			outcome: ApplyOutcome{
				Status: "applied", State: FindingState("unknown"),
				Note: "not persisted", Reference: "artifact-ignored",
			},
		},
		{
			name: "changed state passes only state and note to store",
			outcome: ApplyOutcome{
				Status: "applied", State: FindingResolved,
				Note: "state transition note", Reference: "artifact-ignored",
			},
			wantCalls: 1,
			wantState: FindingResolved,
			wantNote:  "state transition note",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &recordingStateStore{}
			runner := &Runner{
				Store:      store,
				Remediator: contractRemediator{outcome: tt.outcome},
			}

			runner.applyInlineRemediation(context.Background(), stored, Env{}, slog.Default())

			if store.calls != tt.wantCalls {
				t.Fatalf("SetFindingState calls = %d, want %d", store.calls, tt.wantCalls)
			}
			if store.state != tt.wantState {
				t.Errorf("state = %q, want %q", store.state, tt.wantState)
			}
			if store.note != tt.wantNote {
				t.Errorf("note = %q, want %q", store.note, tt.wantNote)
			}
		})
	}
}

func TestProductionRemediatorAdaptersRemainNoopOnly(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve remediation contract test path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	patterns, methodCandidates, err := remediatorCandidates(repoRoot)
	if err != nil {
		t.Fatalf("find production packages with Remediator candidates: %v", err)
	}
	// This source-level inventory intentionally ignores build constraints. It
	// closes the host-context gap in go/packages: a platform-specific adapter
	// must first receive explicit review here before it can reach type checking.
	wantMethodCandidates := []string{
		"engram/internal/security/sandbox.go:Sandbox",
		"pkg/audit/remediation.go:noopRemediator",
	}
	if strings.Join(methodCandidates, "\n") != strings.Join(wantMethodCandidates, "\n") {
		t.Fatalf(
			"production method declarations shaped like Remediator.Apply = %v, want reviewed inventory %v; new declarations are forbidden until an idempotent remediation-event persistence and legacy-migration contract exists",
			methodCandidates,
			wantMethodCandidates,
		)
	}

	loaded, err := packages.Load(&packages.Config{
		Dir: repoRoot,
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedTypesSizes,
	}, patterns...)
	if err != nil {
		t.Fatalf("type-check production Remediator candidates: %v", err)
	}
	var loadErrors []string
	packages.Visit(loaded, nil, func(pkg *packages.Package) {
		for _, pkgErr := range pkg.Errors {
			loadErrors = append(loadErrors, pkgErr.Error())
		}
	})
	if len(loadErrors) != 0 {
		sort.Strings(loadErrors)
		t.Fatalf("type-check production Remediator candidates:\n%s", strings.Join(loadErrors, "\n"))
	}

	var auditPackage *packages.Package
	for _, pkg := range loaded {
		if pkg.PkgPath == "github.com/vbonnet/dear-agent/pkg/audit" {
			auditPackage = pkg
			break
		}
	}
	if auditPackage == nil || auditPackage.Types == nil {
		t.Fatalf("loaded packages %v do not include pkg/audit type information", patterns)
	}
	remediatorObject := auditPackage.Types.Scope().Lookup("Remediator")
	if remediatorObject == nil {
		t.Fatal("pkg/audit.Remediator type not found")
	}
	remediator, ok := remediatorObject.Type().Underlying().(*types.Interface)
	if !ok {
		t.Fatalf("pkg/audit.Remediator has type %T, want interface", remediatorObject.Type().Underlying())
	}
	remediator.Complete()

	var implementations []string
	for _, pkg := range loaded {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			typeName, isType := scope.Lookup(name).(*types.TypeName)
			if !isType || typeName.IsAlias() {
				continue
			}
			candidate := typeName.Type()
			if _, isInterface := candidate.Underlying().(*types.Interface); isInterface {
				continue
			}
			if types.Implements(candidate, remediator) || types.Implements(types.NewPointer(candidate), remediator) {
				implementations = append(implementations, pkg.PkgPath+":"+name)
			}
		}
	}

	sort.Strings(implementations)
	want := []string{"github.com/vbonnet/dear-agent/pkg/audit:noopRemediator"}
	if strings.Join(implementations, "\n") != strings.Join(want, "\n") {
		t.Fatalf(
			"production Remediator implementations = %v, want %v; side-effecting adapters are forbidden until an idempotent remediation-event persistence and legacy-migration contract exists",
			implementations,
			want,
		)
	}
}

func remediatorCandidates(repoRoot string) ([]string, []string, error) {
	candidateDirs := make(map[string]struct{})
	var methodCandidates []string
	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "testdata", "vendor":
				if path != repoRoot {
					return fs.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		isCandidatePackage := false
		for _, decl := range parsed.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if isFunc && fn.Recv != nil && fn.Name.Name == "Apply" &&
				fieldCount(fn.Type.Params) == 3 && fieldCount(fn.Type.Results) == 2 {
				isCandidatePackage = true
				rel, relErr := filepath.Rel(repoRoot, path)
				if relErr != nil {
					return relErr
				}
				methodCandidates = append(
					methodCandidates,
					filepath.ToSlash(rel)+":"+receiverTypeName(fn.Recv.List[0].Type),
				)
			}
		}
		if !isCandidatePackage {
			ast.Inspect(parsed, func(node ast.Node) bool {
				ident, isIdent := node.(*ast.Ident)
				if isIdent && ident.Name == "Remediator" {
					isCandidatePackage = true
					return false
				}
				return !isCandidatePackage
			})
		}
		if isCandidatePackage {
			relDir, relErr := filepath.Rel(repoRoot, filepath.Dir(path))
			if relErr != nil {
				return relErr
			}
			candidateDirs[filepath.ToSlash(relDir)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	patterns := make([]string, 0, len(candidateDirs))
	for dir := range candidateDirs {
		patterns = append(patterns, "./"+dir)
	}
	sort.Strings(patterns)
	sort.Strings(methodCandidates)
	return patterns, methodCandidates, nil
}

func fieldCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		fieldNames := len(field.Names)
		if fieldNames == 0 {
			fieldNames = 1
		}
		count += fieldNames
	}
	return count
}

func receiverTypeName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return receiverTypeName(typed.X)
	case *ast.IndexExpr:
		return receiverTypeName(typed.X)
	case *ast.IndexListExpr:
		return receiverTypeName(typed.X)
	default:
		return "<unknown>"
	}
}

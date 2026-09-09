package buildauthority

import (
	"context"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestExecutableAuthorityPinsMatchSpecification(t *testing.T) {
	t.Parallel()

	goDigest := goExecutableDigest()
	if got := hex.EncodeToString(goDigest[:]); got != "548608a910c46de32c65a3934f461b1787acf6ddd371044826068d8503b8509b" {
		t.Fatalf("Go executable digest = %s", got)
	}
	gitDigest := gitExecutableDigest()
	if got := hex.EncodeToString(gitDigest[:]); got != "be4afb2b003904725826250de9fb76567bbacf82323457b5a1ec26706b66bcae" {
		t.Fatalf("Git executable digest = %s", got)
	}
	if goGOROOTRelativePath != "bin/go" || compilerGOROOTRelativePath != "pkg/tool/darwin_arm64/compile" ||
		goExecutableBase != "go" || gitExecutableBase != "git" {
		t.Fatalf("executable role paths drifted: %q %q %q %q",
			goGOROOTRelativePath, compilerGOROOTRelativePath, goExecutableBase, gitExecutableBase)
	}
}

func TestExecutableNominalAuthoritiesAreNonConvertible(t *testing.T) {
	t.Parallel()

	types := []reflect.Type{
		reflect.TypeFor[goAuthority](),
		reflect.TypeFor[compilerAuthority](),
		reflect.TypeFor[gitAuthority](),
	}
	for left := range types {
		for right := range types {
			if left == right {
				continue
			}
			if types[left].AssignableTo(types[right]) || types[left].ConvertibleTo(types[right]) {
				t.Fatalf("nominal authority %s converts or assigns to %s", types[left], types[right])
			}
		}
	}
	sealTypes := []reflect.Type{
		reflect.TypeFor[goAuthoritySeal](),
		reflect.TypeFor[compilerAuthoritySeal](),
		reflect.TypeFor[gitAuthoritySeal](),
	}
	for left := range sealTypes {
		for right := left + 1; right < len(sealTypes); right++ {
			if sealTypes[left] == sealTypes[right] {
				t.Fatalf("role seals share type %s", sealTypes[left])
			}
		}
	}
	if validGoAuthority == 0 || validCompilerAuthority == 0 || validGitAuthority == 0 {
		t.Fatal("a nominal authority uses its zero seal")
	}
}

func TestExecutableAuthorityZeroAndWrongSealsRefuse(t *testing.T) {
	t.Parallel()

	requirePrivateCauses(t, (&goAuthority{}).revalidate(context.Background()), CauseInternalInvariant)
	requirePrivateCauses(t, (&compilerAuthority{}).revalidate(context.Background()), CauseInternalInvariant)
	requirePrivateCauses(t, (&gitAuthority{}).revalidate(context.Background()), CauseInternalInvariant)
	requirePrivateCauses(t, (&goAuthority{seal: goAuthoritySeal(2)}).revalidate(context.Background()), CauseInternalInvariant)
	requirePrivateCauses(t, (&compilerAuthority{seal: compilerAuthoritySeal(2)}).revalidate(context.Background()), CauseInternalInvariant)
	requirePrivateCauses(t, (&gitAuthority{seal: gitAuthoritySeal(2)}).revalidate(context.Background()), CauseInternalInvariant)
	if err := (&goAuthority{}).close(); err != nil {
		t.Fatalf("zero Go authority close: %v", err)
	}
	if err := (&gitAuthority{}).close(); err != nil {
		t.Fatalf("zero Git authority close: %v", err)
	}
	if err := (*retainedExecutableCandidate)(nil).close(); err != nil {
		t.Fatalf("nil candidate close: %v", err)
	}
}

func TestExecutableNominalConstructorsRefuseCorruptCandidatesWithoutPanicking(t *testing.T) {
	t.Parallel()

	goCandidate := &retainedExecutableCandidate{retained: &retainedExecutable{}}
	if authority, err := admitGoAuthority(context.Background(), goCandidate, nil); err == nil || authority != nil {
		t.Fatal("corrupt candidate gained Go authority")
	} else {
		requirePrivateCauses(t, err, CauseInternalInvariant)
	}
	gitCandidate := &retainedExecutableCandidate{retained: &retainedExecutable{}}
	if authority, err := admitGitAuthority(context.Background(), gitCandidate); err == nil || authority != nil {
		t.Fatal("corrupt candidate gained Git authority")
	} else {
		requirePrivateCauses(t, err, CauseInternalInvariant)
	}
	requirePrivateCauses(
		t,
		validateExecutableAgainstGOROOTBinding(&retainedExecutable{}, gorootExecutableClaim{}),
		CauseInternalInvariant,
	)
}

func TestExecutableCandidateIsTheOnlyRawStringIntake(t *testing.T) {
	t.Parallel()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate executable-authority test")
	}
	productionFile := filepath.Join(filepath.Dir(testFile), "executable_authority.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), productionFile, nil, 0)
	if err != nil {
		t.Fatalf("parse executable authority source: %v", err)
	}
	var stringTakingConstructors []string
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv != nil || function.Type.Results == nil ||
			!resultNamesType(function.Type.Results, "retainedExecutableCandidate") {
			continue
		}
		if fieldListNamesType(function.Type.Params, "string") {
			stringTakingConstructors = append(stringTakingConstructors, function.Name.Name)
		}
	}
	if !reflect.DeepEqual(stringTakingConstructors, []string{"retainExecutableCandidate"}) {
		t.Fatalf("raw executable candidate constructors = %q", stringTakingConstructors)
	}
	compilerType := reflect.TypeOf(deriveCompilerAuthority)
	for input := range compilerType.Ins() {
		if input.Kind() == reflect.String {
			t.Fatal("compiler authority constructor accepts a raw string")
		}
	}
}

func fieldListNamesType(fields *ast.FieldList, name string) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		if identifier, ok := field.Type.(*ast.Ident); ok && identifier.Name == name {
			return true
		}
	}
	return false
}

func resultNamesType(fields *ast.FieldList, name string) bool {
	if fields == nil {
		return false
	}
	for _, field := range fields.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if identifier, ok := star.X.(*ast.Ident); ok && identifier.Name == name {
			return true
		}
	}
	return false
}

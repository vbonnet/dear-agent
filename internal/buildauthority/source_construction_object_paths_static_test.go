package buildauthority

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const sourceObjectPathSourceFilename = "source_construction_object_paths.go"

func TestSourceObjectPathStaticGovernance(t *testing.T) {
	typedPackage := loadTypedSourceConstructionPackage(t, nil)
	violations := sourceObjectPathValidatedInstallViolations(typedPackage)
	violations = append(violations, sourceObjectPathMutationViolations(typedPackage)...)
	violations = append(violations, sourceObjectPathEscapeViolations(typedPackage)...)
	violations = append(violations, sourceObjectPathRoleDeclarationViolations(typedPackage)...)
	if len(violations) != 0 {
		t.Fatalf("source object path static-governance violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestSourceObjectPathValidatedInstallGuardRejectsDrift(t *testing.T) {
	const retentionSignature = "func (builder *sourceConstructionBuilder) retainSourceObjectPathInventory() bool {"
	const ownerValidation = `	if failure = validateSourceObjectPathInventoryOwner(
		builder.ctx,
		builder.owner,
		inventory,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
`
	const install = "\tbuilder.owner.objects.paths = inventory\n"
	for _, test := range []struct {
		name                string
		original            string
		replacement         string
		passBuilderArgument bool
	}{
		{
			name:                "receiver spoofed by parameter",
			original:            retentionSignature,
			passBuilderArgument: true,
			replacement: "func (receiver *sourceConstructionBuilder) " +
				"retainSourceObjectPathInventory(builder *sourceConstructionBuilder) bool {",
		},
		{
			name:     "skipped owner validation",
			original: ownerValidation,
		},
		{
			name:        "different pointer installed",
			original:    install,
			replacement: "\tbuilder.owner.objects.paths = &sourceObjectPathInventory{}\n",
		},
		{
			name:     "mutation after validation",
			original: install,
			replacement: "\tinventory.contentFileCount++\n" +
				install,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			overlay := sourceObjectPathOverlay(t, test.original, test.replacement)
			if test.passBuilderArgument {
				sourceObjectPathRetainCallOverlay(t, overlay)
			}
			typedPackage := loadTypedSourceConstructionPackage(
				t,
				overlay,
			)
			joined := strings.Join(sourceObjectPathValidatedInstallViolations(typedPackage), "\n")
			if !strings.Contains(joined, "validated install is outside audited shape") {
				t.Fatalf("validated-install violations =\n%s\nwant shape rejection", joined)
			}
		})
	}
}

func sourceObjectPathRetainCallOverlay(t *testing.T, overlay map[string][]byte) {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	path := filepath.Join(workingDirectory, "source_construction_acquire.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source construction acquisition fixture: %v", err)
	}
	const original = "builder.retainSourceObjectPathInventory()"
	if count := strings.Count(string(source), original); count != 1 {
		t.Fatalf("source object path retention call count = %d, want 1", count)
	}
	overlay[path] = []byte(strings.Replace(
		string(source),
		original,
		"builder.retainSourceObjectPathInventory(builder)",
		1,
	))
}

func TestSourceObjectPathMutationGuardRejectsDrift(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "row mutation",
			body: `

func sourceObjectPathStaticFixtureMutateRow(row *sourceObjectPathRow) {
	row.path = ".git/objects/fixture"
}
`,
			want: "source object path mutation is forbidden",
		},
		{
			name: "short declaration rebind",
			body: `

func sourceObjectPathStaticFixtureRebindRow(
	row sourceObjectPathRow,
	replacement sourceObjectPathRow,
) {
	marker, row := 0, replacement
	_, _ = marker, row
}
`,
			want: "source object path mutation is forbidden",
		},
		{
			name: "slice storage rebind",
			body: `

func sourceObjectPathStaticFixtureRebindRows(rows []sourceObjectPathRow) {
	rows = rows[:0]
}
`,
			want: "source object path mutation is forbidden",
		},
		{
			name: "retained objects wrapper rebind",
			body: `

func sourceObjectPathStaticFixtureRebindObjects(owner *sourceConstructionOwner) {
	*owner.objects = retainedSourceObjects{root: owner.objects.root}
}
`,
			want: "source object path mutation is forbidden",
		},
		{
			name: "count mutation",
			body: `

func sourceObjectPathStaticFixtureMutateCount(inventory *sourceObjectPathInventory) {
	inventory.contentFileCount++
}
`,
			want: "source object path increment is forbidden",
		},
		{
			name: "address escape",
			body: `

func sourceObjectPathStaticFixtureEscapeRow(
	inventory *sourceObjectPathInventory,
) *sourceObjectPathRow {
	return &inventory.rows[0]
}
`,
			want: "source object path address is forbidden",
		},
		{
			name: "composite address escape",
			body: `

func sourceObjectPathStaticFixtureEscapeComposite() *sourceObjectPathRow {
	return &sourceObjectPathRow{}
}
`,
			want: "source object path address is forbidden",
		},
		{
			name: "wrapper composite address escape",
			body: `

func sourceObjectPathStaticFixtureEscapeObjects() *retainedSourceObjects {
	return &retainedSourceObjects{}
}
`,
			want: "source object path address is forbidden",
		},
		{
			name: "builtin write",
			body: `

func sourceObjectPathStaticFixtureClearRows(inventory *sourceObjectPathInventory) {
	clear(inventory.rows)
}
`,
			want: "source object path write builtin is outside audited set",
		},
		{
			name: "new address escape",
			body: `

func sourceObjectPathStaticFixtureNewRow() *sourceObjectPathRow {
	return new(sourceObjectPathRow)
}
`,
			want: "source object path address builtin is forbidden",
		},
		{
			name: "new wrapper address escape",
			body: `

func sourceObjectPathStaticFixtureNewObjects() *retainedSourceObjects {
	return new(retainedSourceObjects)
}
`,
			want: "source object path address builtin is forbidden",
		},
		{
			name: "delete wrapper storage",
			body: `

type sourceObjectPathStaticFixtureWrapper struct {
	row sourceObjectPathRow
}

func sourceObjectPathStaticFixtureDeleteWrapper(
	rows map[string]sourceObjectPathStaticFixtureWrapper,
) {
	delete(rows, "fixture")
}
`,
			want: "source object path write builtin is outside audited set",
		},
		{
			name: "append outside audited site",
			body: `

func sourceObjectPathStaticFixtureAppendRow(
	rows []sourceObjectPathRow,
	row sourceObjectPathRow,
) {
	_ = append(rows, row)
}
`,
			want: "source object path write builtin is outside audited set",
		},
		{
			name: "generic mutation laundering",
			body: `

func sourceObjectPathStaticFixtureZero[T any](pointer *T) {
	var zero T
	*pointer = zero
}

func sourceObjectPathStaticFixtureGenericMutation(builder *sourceConstructionBuilder) {
	sourceObjectPathStaticFixtureZero(builder.owner.objects.paths)
}
`,
			want: "generic call carrying source object path storage is forbidden",
		},
		{
			name: "generic receiver mutation laundering",
			body: `

type sourceObjectPathStaticFixtureZeroer[T any] struct {
	pointer *T
}

func (zeroer sourceObjectPathStaticFixtureZeroer[T]) zero() {
	var value T
	*zeroer.pointer = value
}

func sourceObjectPathStaticFixtureGenericReceiverMutation(
	builder *sourceConstructionBuilder,
) {
	sourceObjectPathStaticFixtureZeroer[sourceObjectPathInventory]{
		pointer: builder.owner.objects.paths,
	}.zero()
}
`,
			want: "generic call carrying source object path storage is forbidden",
		},
		{
			name: "defined inventory conversion mutation laundering",
			body: `

type sourceObjectPathStaticFixtureMirrorInventory sourceObjectPathInventory

func sourceObjectPathStaticFixtureConversionMutation(
	inventory *sourceObjectPathInventory,
) {
	mirror := (*sourceObjectPathStaticFixtureMirrorInventory)(inventory)
	mirror.contentFileCount++
}
`,
			want: "source object path conversion is forbidden",
		},
		{
			name: "defined row conversion source laundering",
			body: `

type sourceObjectPathStaticFixtureMirrorRow sourceObjectPathRow

func sourceObjectPathStaticFixtureConversionSourceMutation(row *sourceObjectPathRow) {
	mirror := (*sourceObjectPathStaticFixtureMirrorRow)(row)
	mirror.path = "laundered"
}
`,
			want: "source object path conversion is forbidden",
		},
		{
			name: "defined row conversion target laundering",
			body: `

type sourceObjectPathStaticFixtureTargetMirrorRow sourceObjectPathRow

func sourceObjectPathStaticFixtureConversionTargetMutation(
	mirror *sourceObjectPathStaticFixtureTargetMirrorRow,
) {
	row := (*sourceObjectPathRow)(mirror)
	row.path = "laundered"
}
`,
			want: "source object path conversion is forbidden",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			typedPackage := loadTypedSourceConstructionPackage(
				t,
				sourceObjectPathAppendedOverlay(t, test.body),
			)
			joined := strings.Join(sourceObjectPathMutationViolations(typedPackage), "\n")
			if !strings.Contains(joined, test.want) {
				t.Fatalf("mutation violations =\n%s\nwant %q", joined, test.want)
			}
		})
	}
}

func TestSourceObjectPathRoleDeclarationGuardRejectsDrift(t *testing.T) {
	const lastRole = "\tsourceObjectMultiPackIndexLayerAuxiliaryPath\n)"
	const extraRole = "\tsourceObjectMultiPackIndexLayerAuxiliaryPath\n\tsourceObjectPathStaticFixtureRole\n)"
	typedPackage := loadTypedSourceConstructionPackage(
		t,
		sourceObjectPathOverlay(t, lastRole, extraRole),
	)
	joined := strings.Join(sourceObjectPathRoleDeclarationViolations(typedPackage), "\n")
	if !strings.Contains(joined, "source object path role declaration") {
		t.Fatalf("role declaration violations =\n%s\nwant closed-declaration rejection", joined)
	}
}

func TestSourceObjectPathEscapeGuardRejectsUnsafeInterfaceErasure(t *testing.T) {
	path, source := sourceObjectPathSource(t)
	withUnsafe := strings.Replace(
		string(source),
		"\t\"strings\"\n)",
		"\t\"strings\"\n\t\"unsafe\"\n)",
		1,
	)
	if withUnsafe == string(source) {
		t.Fatal("source object path fixture could not add unsafe import")
	}
	withUnsafe += `

type sourceObjectPathStaticFixtureInterfaceHeader struct {
	typ  unsafe.Pointer
	data unsafe.Pointer
}

type sourceObjectPathStaticFixtureErasedRow struct {
	path string
	role uint8
	key  string
}

type sourceObjectPathStaticFixtureErasedInventory struct {
	rows               []sourceObjectPathStaticFixtureErasedRow
	contentFileCount   uint64
	auxiliaryFileCount uint64
}

func sourceObjectPathStaticFixtureUnsafeErasure(
	inventory *sourceObjectPathInventory,
) {
	var erased any = inventory
	header := *(*sourceObjectPathStaticFixtureInterfaceHeader)(unsafe.Pointer(&erased))
	mirror := (*sourceObjectPathStaticFixtureErasedInventory)(header.data)
	mirror.contentFileCount++
}
`
	typedPackage := loadTypedSourceConstructionPackage(
		t,
		map[string][]byte{path: []byte(withUnsafe)},
	)
	joined := strings.Join(sourceObjectPathEscapeViolations(typedPackage), "\n")
	for _, want := range []string{
		"imports unsafe inside the source object path boundary",
		"writes sourceObjectPathInventory into package or type-erasing storage erased",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("escape violations =\n%s\nwant %q", joined, want)
		}
	}
}

func TestSourceObjectPathEscapeGuardRejectsImplicitCallAndReturnErasure(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "call",
			body: `

func sourceObjectPathStaticFixtureErasureSink(any) {}

func sourceObjectPathStaticFixtureCallErasure(inventory *sourceObjectPathInventory) {
	sourceObjectPathStaticFixtureErasureSink(inventory)
}
`,
			want: "erases sourceObjectPathInventory through call",
		},
		{
			name: "return",
			body: `

func sourceObjectPathStaticFixtureReturnErasure(
	inventory *sourceObjectPathInventory,
) any {
	return inventory
}
`,
			want: "returns sourceObjectPathInventory through type-erasing result",
		},
		{
			name: "composite map key",
			body: `

func sourceObjectPathStaticFixtureCompositeMapKeyErasure(
	inventory *sourceObjectPathInventory,
) {
	erased := map[any]struct{}{inventory: struct{}{}}
	_ = erased
}
`,
			want: "writes sourceObjectPathInventory into package or type-erasing storage erased",
		},
		{
			name: "map index key",
			body: `

func sourceObjectPathStaticFixtureMapIndexKeyErasure(
	inventory *sourceObjectPathInventory,
) {
	erased := make(map[any]struct{})
	erased[inventory] = struct{}{}
}
`,
			want: "stores sourceObjectPathInventory as a type-erasing map key in erased",
		},
		{
			name: "multi result call",
			body: `

func sourceObjectPathStaticFixturePair(
	inventory *sourceObjectPathInventory,
) (*sourceObjectPathInventory, *sourceObjectPathInventory) {
	return inventory, inventory
}

func sourceObjectPathStaticFixturePairSink(*sourceObjectPathInventory, any) {}

func sourceObjectPathStaticFixturePairCallErasure(
	inventory *sourceObjectPathInventory,
) {
	sourceObjectPathStaticFixturePairSink(sourceObjectPathStaticFixturePair(inventory))
}
`,
			want: "erases sourceObjectPathInventory through call",
		},
		{
			name: "multi result return",
			body: `

func sourceObjectPathStaticFixturePairWithFailure(
	inventory *sourceObjectPathInventory,
) (*sourceObjectPathInventory, error) {
	return inventory, nil
}

func sourceObjectPathStaticFixturePairReturnErasure(
	inventory *sourceObjectPathInventory,
) (any, error) {
	return sourceObjectPathStaticFixturePairWithFailure(inventory)
}
`,
			want: "returns sourceObjectPathInventory through type-erasing result",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			typedPackage := loadTypedSourceConstructionPackage(
				t,
				sourceObjectPathAppendedOverlay(t, test.body),
			)
			joined := strings.Join(sourceObjectPathEscapeViolations(typedPackage), "\n")
			if !strings.Contains(joined, test.want) {
				t.Fatalf("escape violations =\n%s\nwant %q", joined, test.want)
			}
		})
	}
}

func sourceObjectPathOverlay(t *testing.T, original string, replacement string) map[string][]byte {
	t.Helper()
	path, source := sourceObjectPathSource(t)
	if count := strings.Count(string(source), original); count != 1 {
		t.Fatalf("source object path overlay match count = %d, want 1", count)
	}
	return map[string][]byte{
		path: []byte(strings.Replace(string(source), original, replacement, 1)),
	}
}

func sourceObjectPathAppendedOverlay(t *testing.T, suffix string) map[string][]byte {
	t.Helper()
	path, source := sourceObjectPathSource(t)
	return map[string][]byte{path: append(append([]byte(nil), source...), []byte(suffix)...)}
}

func sourceObjectPathSource(t *testing.T) (string, []byte) {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	path := filepath.Join(workingDirectory, sourceObjectPathSourceFilename)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source object path fixture: %v", err)
	}
	return path, source
}

func sourceObjectPathValidatedInstallViolations(typedPackage *packages.Package) []string {
	function := sourceObjectPathFunctionDeclaration(
		typedPackage,
		sourceObjectPathSourceFilename,
		"(*sourceConstructionBuilder).retainSourceObjectPathInventory",
	)
	if sourceObjectPathValidatedInstallProved(typedPackage, function) {
		return nil
	}
	position := "(*sourceConstructionBuilder).retainSourceObjectPathInventory"
	if typedPackage != nil && typedPackage.Fset != nil && function != nil {
		position = typedPackage.Fset.Position(function.Pos()).String()
	}
	return []string{position + " source object path validated install is outside audited shape"}
}

func sourceObjectPathValidatedInstallProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 6 {
		return false
	}
	info := typedPackage.TypesInfo
	statements := function.Body.List
	receiverObject, signatureOK := sourceObjectPathRetentionSignatureProved(
		typedPackage,
		function,
	)
	if !signatureOK {
		return false
	}
	if !sourceObjectPathInitialFailureGuardProved(
		typedPackage,
		statements[0],
		receiverObject,
	) {
		return false
	}
	derivation, ok := statements[1].(*ast.AssignStmt)
	if !ok || derivation.Tok != token.DEFINE || len(derivation.Lhs) != 2 ||
		len(derivation.Rhs) != 1 {
		return false
	}
	inventory, inventoryOK := derivation.Lhs[0].(*ast.Ident)
	failure, failureOK := derivation.Lhs[1].(*ast.Ident)
	call, callOK := derivation.Rhs[0].(*ast.CallExpr)
	if !inventoryOK || !failureOK || !callOK || inventory.Name != "inventory" ||
		failure.Name != "failure" || !sourceObjectPathPackageFunctionCallProved(
		typedPackage,
		call,
		"deriveSourceObjectPathInventory",
	) || len(call.Args) != 3 || !sourceConstructionSelectorChainProved(
		info,
		call.Args[0],
		receiverObject,
		"ctx",
	) || !sourceConstructionSelectorChainProved(
		info,
		call.Args[1],
		receiverObject,
		"owner",
		"config",
		"claim",
		"objectFormat",
	) || !sourceConstructionSelectorChainProved(
		info,
		call.Args[2],
		receiverObject,
		"owner",
		"git",
		"administration",
	) {
		return false
	}
	inventoryObject := info.Defs[inventory]
	failureObject := info.Defs[failure]
	if inventoryObject == nil || failureObject == nil ||
		!sourceObjectPathOutcomeFailureBranchProved(
			info,
			statements[2],
			typedPackage,
			receiverObject,
			failureObject,
		) || !sourceObjectPathOwnerValidationGuardProved(
		typedPackage,
		statements[3],
		receiverObject,
		inventoryObject,
		failureObject,
	) {
		return false
	}
	install, ok := statements[4].(*ast.AssignStmt)
	if !ok || install.Tok != token.ASSIGN || len(install.Lhs) != 1 ||
		len(install.Rhs) != 1 || !sourceConstructionSelectorChainProved(
		info,
		install.Lhs[0],
		receiverObject,
		"owner",
		"objects",
		"paths",
	) || !sourceConstructionDirectObjectExpression(
		info,
		install.Rhs[0],
		inventoryObject,
	) {
		return false
	}
	return sourceConstructionSingleBooleanReturnProved(statements[5], "true")
}

func sourceObjectPathOutcomeFailureBranchProved(
	info *types.Info,
	statement ast.Stmt,
	typedPackage *packages.Package,
	receiverObject types.Object,
	failureObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Init == nil && branch.Else == nil && len(branch.Body.List) == 2 &&
		sourceObjectPathObjectNilComparison(info, branch.Cond, failureObject, token.NEQ) &&
		sourceObjectPathAddFailureAndReturnProved(
			info,
			branch.Body.List,
			typedPackage,
			receiverObject,
			failureObject,
		)
}

func sourceObjectPathRetentionSignatureProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) (types.Object, bool) {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Recv == nil || len(function.Recv.List) != 1 ||
		len(function.Recv.List[0].Names) != 1 {
		return nil, false
	}
	receiver := function.Recv.List[0].Names[0]
	if receiver.Name != "builder" {
		return nil, false
	}
	receiverObject := typedPackage.TypesInfo.Defs[receiver]
	callable, ok := typedPackage.TypesInfo.Defs[function.Name].(*types.Func)
	if receiverObject == nil || !ok {
		return nil, false
	}
	signature, ok := callable.Type().(*types.Signature)
	return receiverObject, ok && signature.Recv() != nil &&
		types.Identical(signature.Recv().Type(), sourceConstructionObjectType(receiverObject)) &&
		signature.Params().Len() == 0 && signature.Results().Len() == 1 &&
		types.Identical(signature.Results().At(0).Type(), types.Typ[types.Bool]) &&
		signature.TypeParams().Len() == 0 && signature.RecvTypeParams().Len() == 0 &&
		!signature.Variadic()
}

func sourceObjectPathInitialFailureGuardProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	receiverObject types.Object,
) bool {
	info := typedPackage.TypesInfo
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) != 2 {
		return false
	}
	assignment, ok := branch.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return false
	}
	failure, failureOK := assignment.Lhs[0].(*ast.Ident)
	call, callOK := assignment.Rhs[0].(*ast.CallExpr)
	if !failureOK || !callOK || failure.Name != "failure" || len(call.Args) != 0 ||
		!sourceObjectPathMethodCallProved(
			typedPackage,
			call,
			receiverObject,
			"(*sourceConstructionBuilder).validateSourceObjectPathInventoryRequest",
		) {
		return false
	}
	failureObject := info.Defs[failure]
	return failureObject != nil && sourceObjectPathObjectNilComparison(
		info,
		branch.Cond,
		failureObject,
		token.NEQ,
	) && sourceObjectPathAddFailureAndReturnProved(
		info,
		branch.Body.List,
		typedPackage,
		receiverObject,
		failureObject,
	)
}

func sourceObjectPathOwnerValidationGuardProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	receiverObject types.Object,
	inventoryObject types.Object,
	failureObject types.Object,
) bool {
	info := typedPackage.TypesInfo
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) != 2 ||
		!sourceObjectPathObjectNilComparison(info, branch.Cond, failureObject, token.NEQ) {
		return false
	}
	assignment, ok := branch.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || !sourceConstructionDirectObjectExpression(
		info,
		assignment.Lhs[0],
		failureObject,
	) {
		return false
	}
	validation, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || !sourceObjectPathPackageFunctionCallProved(
		typedPackage,
		validation,
		"validateSourceObjectPathInventoryOwner",
	) || len(validation.Args) != 3 || !sourceConstructionSelectorChainProved(
		info,
		validation.Args[0],
		receiverObject,
		"ctx",
	) || !sourceConstructionSelectorChainProved(
		info,
		validation.Args[1],
		receiverObject,
		"owner",
	) || !sourceConstructionDirectObjectExpression(
		info,
		validation.Args[2],
		inventoryObject,
	) {
		return false
	}
	return sourceObjectPathAddFailureAndReturnProved(
		info,
		branch.Body.List,
		typedPackage,
		receiverObject,
		failureObject,
	)
}

func sourceObjectPathAddFailureAndReturnProved(
	info *types.Info,
	statements []ast.Stmt,
	typedPackage *packages.Package,
	receiverObject types.Object,
	failureObject types.Object,
) bool {
	if len(statements) != 2 {
		return false
	}
	expression, ok := statements[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	return ok && sourceObjectPathMethodCallProved(
		typedPackage,
		call,
		receiverObject,
		"(*sourceUseOutcome).addPrimitive",
		"outcome",
	) && len(call.Args) == 1 &&
		sourceConstructionDirectObjectExpression(info, call.Args[0], failureObject) &&
		sourceConstructionSingleBooleanReturnProved(statements[1], "false")
}

func sourceObjectPathMethodCallProved(
	typedPackage *packages.Package,
	call *ast.CallExpr,
	receiverObject types.Object,
	role string,
	receiverFields ...string,
) bool {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		call == nil {
		return false
	}
	selector, ok := sourceConstructionUnparenthesizedExpression(call.Fun).(*ast.SelectorExpr)
	called, calledOK := sourceConstructionCalledObject(
		typedPackage.TypesInfo,
		call.Fun,
	).(*types.Func)
	return ok && calledOK && called.Pkg() == typedPackage.Types &&
		sourceConstructionFunctionRole(called, typedPackage.Types.Path()) == role &&
		sourceConstructionSelectorChainProved(
			typedPackage.TypesInfo,
			selector.X,
			receiverObject,
			receiverFields...,
		)
}

func sourceObjectPathPackageFunctionCallProved(
	typedPackage *packages.Package,
	call *ast.CallExpr,
	name string,
) bool {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		call == nil {
		return false
	}
	called := sourceConstructionCalledObject(typedPackage.TypesInfo, call.Fun)
	return called != nil && called == typedPackage.Types.Scope().Lookup(name)
}

func sourceObjectPathObjectNilComparison(
	info *types.Info,
	expression ast.Expr,
	object types.Object,
	operator token.Token,
) bool {
	comparison, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.BinaryExpr)
	if !ok || comparison.Op != operator || !sourceConstructionDirectObjectExpression(
		info,
		comparison.X,
		object,
	) {
		return false
	}
	nilIdentifier, ok := sourceConstructionUnparenthesizedExpression(
		comparison.Y,
	).(*ast.Ident)
	return ok && info.Uses[nilIdentifier] == types.Universe.Lookup("nil")
}

func sourceObjectPathMutationViolations(typedPackage *packages.Package) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for source object path mutation audit"}
	}
	var observedAllowedAppend int
	var observedAllowedInventoryConstruction int
	var observedAllowedObjectsAssignment int
	var observedAllowedObjectsConstruction int
	var observedAllowedEnclosingConstruction int
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		parents := sourceConstructionParentNodes(file)
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				return true
			}
			function := sourceConstructionEnclosingFunction(file, node.Pos())
			role := "<outside-function>"
			if function != nil {
				role = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			switch current := node.(type) {
			case *ast.FuncDecl:
				callable, callableOK := typedPackage.TypesInfo.Defs[current.Name].(*types.Func)
				signature, signatureOK := func() (*types.Signature, bool) {
					if !callableOK {
						return nil, false
					}
					signature, ok := callable.Type().(*types.Signature)
					return signature, ok
				}()
				if strings.HasPrefix(filename, "source_construction") && signatureOK &&
					(signature.TypeParams().Len() != 0 ||
						signature.RecvTypeParams().Len() != 0) {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" generic function is forbidden in the source-construction module "+
							filename+"|"+role,
					)
				}
			case *ast.AssignStmt:
				allowedObjectsAssignment := sourceObjectPathAllowedObjectsAssignment(
					typedPackage,
					function,
					current,
				)
				if allowedObjectsAssignment {
					observedAllowedObjectsAssignment++
				}
				allowedAssignment := sourceObjectPathAllowedInstallAssignment(
					typedPackage,
					function,
					current,
				) || sourceObjectPathAllowedAppendAssignment(
					typedPackage,
					function,
					current,
					parents,
				) || allowedObjectsAssignment
				for _, target := range current.Lhs {
					kind := sourceObjectPathMutationTarget(
						typedPackage.TypesInfo,
						target,
					)
					if kind != "" && !allowedAssignment {
						violations = append(violations,
							typedPackage.Fset.Position(target.Pos()).String()+
								" source object path mutation is forbidden "+
								filename+"|"+role+"|"+kind+"|"+types.ExprString(target),
						)
					}
				}
			case *ast.IncDecStmt:
				if kind := sourceObjectPathMutationTarget(
					typedPackage.TypesInfo,
					current.X,
				); kind != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source object path increment is forbidden "+
							filename+"|"+role+"|"+kind,
					)
				}
			case *ast.RangeStmt:
				for _, target := range []ast.Expr{current.Key, current.Value} {
					if target == nil {
						continue
					}
					if kind := sourceObjectPathMutationTarget(
						typedPackage.TypesInfo,
						target,
					); kind != "" {
						violations = append(violations,
							typedPackage.Fset.Position(target.Pos()).String()+
								" source object path range mutation is forbidden "+
								filename+"|"+role+"|"+kind,
						)
					}
				}
			case *ast.UnaryExpr:
				if current.Op != token.AND {
					return true
				}
				if sourceObjectPathAllowedInventoryConstruction(
					typedPackage,
					function,
					current,
					parents,
				) {
					observedAllowedInventoryConstruction++
					return true
				}
				if sourceObjectPathAllowedObjectsConstruction(
					typedPackage,
					function,
					current,
				) {
					observedAllowedObjectsConstruction++
					return true
				}
				if sourceObjectPathAllowedEnclosingConstruction(
					typedPackage,
					function,
					current,
				) {
					observedAllowedEnclosingConstruction++
					return true
				}
				kind := sourceObjectPathMutationRootKind(typedPackage.TypesInfo, current.X)
				if kind == "" && sourceObjectPathStorageContains(
					typedPackage.TypesInfo.TypeOf(current.X),
					map[types.Type]bool{},
				) {
					kind = "sourceObjectPathStorage"
				}
				if kind != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source object path address is forbidden "+
							filename+"|"+role+"|"+kind+"|"+types.ExprString(current),
					)
				}
			case *ast.CallExpr:
				if sourceObjectPathConversionCarriesStorage(
					typedPackage.TypesInfo,
					current,
				) {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source object path conversion is forbidden "+
							filename+"|"+role+"|"+sourceConstructionCallFingerprint(current),
					)
					return true
				}
				if sourceObjectPathGenericCallCarriesStorage(typedPackage, current) {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" generic call carrying source object path storage is forbidden "+
							filename+"|"+role+"|"+sourceConstructionCallFingerprint(current),
					)
					return true
				}
				builtin, ok := sourceConstructionCalledObject(
					typedPackage.TypesInfo,
					current.Fun,
				).(*types.Builtin)
				if !ok {
					return true
				}
				if builtin.Name() == "new" && sourceObjectPathStorageContains(
					typedPackage.TypesInfo.TypeOf(current),
					map[types.Type]bool{},
				) {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source object path address builtin is forbidden "+
							filename+"|"+role+"|"+sourceConstructionCallFingerprint(current),
					)
					return true
				}
				if len(current.Args) == 0 ||
					(builtin.Name() != "append" && builtin.Name() != "copy" &&
						builtin.Name() != "clear" && builtin.Name() != "delete") ||
					!sourceObjectPathStorageContains(
						typedPackage.TypesInfo.TypeOf(current.Args[0]),
						map[types.Type]bool{},
					) {
					return true
				}
				if sourceObjectPathAllowedAppendCall(
					typedPackage,
					function,
					current,
					parents,
				) {
					observedAllowedAppend++
					return true
				}
				site := filename + "|" + role + "|" + sourceConstructionCallFingerprint(current)
				violations = append(violations,
					typedPackage.Fset.Position(current.Pos()).String()+
						" source object path write builtin is outside audited set "+site,
				)
			}
			return true
		})
	}
	if observedAllowedAppend != 1 {
		violations = append(violations,
			"audited source object path append count = "+
				strconv.Itoa(observedAllowedAppend)+", want 1",
		)
	}
	if observedAllowedInventoryConstruction != 1 {
		violations = append(violations,
			"audited source object path inventory construction count = "+
				strconv.Itoa(observedAllowedInventoryConstruction)+", want 1",
		)
	}
	if observedAllowedObjectsAssignment != 1 {
		violations = append(violations,
			"audited retained source objects assignment count = "+
				strconv.Itoa(observedAllowedObjectsAssignment)+", want 1",
		)
	}
	if observedAllowedObjectsConstruction != 1 {
		violations = append(violations,
			"audited retained source objects construction count = "+
				strconv.Itoa(observedAllowedObjectsConstruction)+", want 1",
		)
	}
	if observedAllowedEnclosingConstruction != 2 {
		violations = append(violations,
			"audited enclosing source construction count = "+
				strconv.Itoa(observedAllowedEnclosingConstruction)+", want 2",
		)
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectPathEscapeViolations(typedPackage *packages.Package) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for source object path escape audit"}
	}
	constructionFiles := make([]*ast.File, 0, 4)
	for _, file := range typedPackage.Syntax {
		filename := typedPackage.Fset.Position(file.Package).Filename
		if strings.HasPrefix(filepath.Base(filename), "source_construction") {
			constructionFiles = append(constructionFiles, file)
		}
	}
	violations := make([]string, 0)
	for _, file := range constructionFiles {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		for _, specification := range file.Imports {
			importPath, err := strconv.Unquote(specification.Path.Value)
			if err == nil && importPath == "unsafe" {
				violations = append(violations,
					typedPackage.Fset.Position(specification.Pos()).String()+
						" imports unsafe inside the source object path boundary "+filename,
				)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.AssignStmt:
				violations = append(violations, sourceObjectPathAssignmentEscapeViolations(
					typedPackage,
					current.Lhs,
					current.Rhs,
				)...)
			case *ast.ValueSpec:
				targets := make([]ast.Expr, 0, len(current.Names))
				for _, name := range current.Names {
					targets = append(targets, name)
				}
				violations = append(violations, sourceObjectPathAssignmentEscapeViolations(
					typedPackage,
					targets,
					current.Values,
				)...)
			case *ast.CallExpr:
				if escaped := sourceObjectPathCallErasesStorage(
					typedPackage,
					current,
				); escaped != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" erases "+escaped+" through call "+
							sourceConstructionCallFingerprint(current),
					)
				}
			case *ast.IndexExpr:
				if violation := sourceObjectPathMapKeyEscapeViolation(
					typedPackage,
					current,
				); violation != "" {
					violations = append(violations, violation)
				}
			case *ast.ReturnStmt:
				violations = append(violations, sourceObjectPathReturnEscapeViolations(
					typedPackage,
					file,
					current,
				)...)
			case *ast.SendStmt:
				if escaped := sourceObjectPathExpressionStorageKind(
					typedPackage.TypesInfo,
					current.Value,
				); escaped != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" sends "+escaped+" through a channel",
					)
				}
			}
			return true
		})
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectPathAssignmentEscapeViolations(
	typedPackage *packages.Package,
	targets []ast.Expr,
	values []ast.Expr,
) []string {
	if typedPackage == nil || typedPackage.TypesInfo == nil || len(targets) == 0 ||
		len(values) == 0 {
		return nil
	}
	type assignmentPair struct {
		target     ast.Expr
		value      types.Type
		expression ast.Expr
	}
	pairs := make([]assignmentPair, 0, len(targets))
	if len(values) == len(targets) {
		for index := range targets {
			pairs = append(pairs, assignmentPair{
				target:     targets[index],
				value:      typedPackage.TypesInfo.TypeOf(values[index]),
				expression: values[index],
			})
		}
	} else if len(values) == 1 {
		if tuple, ok := typedPackage.TypesInfo.TypeOf(values[0]).(*types.Tuple); ok &&
			tuple.Len() == len(targets) {
			for index := range targets {
				pairs = append(pairs, assignmentPair{
					target: targets[index],
					value:  tuple.At(index).Type(),
				})
			}
		}
	}
	if len(pairs) == 0 {
		for _, value := range values {
			for _, target := range targets {
				pairs = append(pairs, assignmentPair{
					target:     target,
					value:      typedPackage.TypesInfo.TypeOf(value),
					expression: value,
				})
			}
		}
	}
	violations := make([]string, 0)
	for _, pair := range pairs {
		escaped := sourceObjectPathStorageKind(pair.value)
		if pair.expression != nil {
			escaped = sourceObjectPathExpressionStorageKind(
				typedPackage.TypesInfo,
				pair.expression,
			)
		}
		if escaped == "" || sourceObjectPathStorageContains(
			typedPackage.TypesInfo.TypeOf(pair.target),
			map[types.Type]bool{},
		) {
			continue
		}
		violations = append(violations,
			typedPackage.Fset.Position(pair.target.Pos()).String()+
				" writes "+escaped+" into package or type-erasing storage "+
				types.ExprString(pair.target),
		)
	}
	return violations
}

func sourceObjectPathExpressionStorageKind(info *types.Info, expression ast.Expr) string {
	if info == nil || expression == nil {
		return ""
	}
	if escaped := sourceObjectPathStorageKind(info.TypeOf(expression)); escaped != "" {
		return escaped
	}
	switch current := expression.(type) {
	case *ast.ParenExpr:
		return sourceObjectPathExpressionStorageKind(info, current.X)
	case *ast.CompositeLit:
		for _, element := range current.Elts {
			if escaped := sourceObjectPathExpressionStorageKind(info, element); escaped != "" {
				return escaped
			}
		}
	case *ast.KeyValueExpr:
		keyIsField := false
		if identifier, ok := current.Key.(*ast.Ident); ok {
			field, _ := info.Uses[identifier].(*types.Var)
			keyIsField = field != nil && field.IsField()
		}
		if !keyIsField {
			if escaped := sourceObjectPathExpressionStorageKind(info, current.Key); escaped != "" {
				return escaped
			}
		}
		return sourceObjectPathExpressionStorageKind(info, current.Value)
	case *ast.CallExpr:
		called := sourceConstructionCalledObject(info, current.Fun)
		_, typeConversion := called.(*types.TypeName)
		callable := sourceConstructionUnparenthesizedExpression(current.Fun)
		if typeAndValue, ok := info.Types[callable]; ok && typeAndValue.IsType() {
			typeConversion = true
		}
		builtin, builtinCall := called.(*types.Builtin)
		preservesValues := builtinCall && (builtin.Name() == "append" || builtin.Name() == "copy")
		if typeConversion || preservesValues {
			for _, argument := range current.Args {
				if escaped := sourceObjectPathExpressionStorageKind(info, argument); escaped != "" {
					return escaped
				}
			}
		}
	}
	return ""
}

func sourceObjectPathStorageKind(value types.Type) string {
	if kind := sourceObjectPathDirectMutationKind(value); kind != "" {
		return kind
	}
	if value == nil {
		return ""
	}
	if tuple, ok := types.Unalias(value).(*types.Tuple); ok {
		for variable := range tuple.Variables() {
			if kind := sourceObjectPathStorageKind(variable.Type()); kind != "" {
				return kind
			}
		}
		return ""
	}
	if sourceObjectPathStorageContains(value, map[types.Type]bool{}) {
		return "sourceObjectPathStorage"
	}
	return ""
}

func sourceObjectPathMapKeyEscapeViolation(
	typedPackage *packages.Package,
	index *ast.IndexExpr,
) string {
	if typedPackage == nil || typedPackage.TypesInfo == nil || index == nil {
		return ""
	}
	container := typedPackage.TypesInfo.TypeOf(index.X)
	if container == nil {
		return ""
	}
	mapType, ok := types.Unalias(container).Underlying().(*types.Map)
	if !ok || sourceObjectPathStorageContains(mapType.Key(), map[types.Type]bool{}) {
		return ""
	}
	escaped := sourceObjectPathExpressionStorageKind(
		typedPackage.TypesInfo,
		index.Index,
	)
	if escaped == "" {
		return ""
	}
	return typedPackage.Fset.Position(index.Index.Pos()).String() +
		" stores " + escaped + " as a type-erasing map key in " +
		types.ExprString(index.X)
}

func sourceObjectPathCallErasesStorage(
	typedPackage *packages.Package,
	call *ast.CallExpr,
) string {
	if typedPackage == nil || typedPackage.TypesInfo == nil || call == nil {
		return ""
	}
	info := typedPackage.TypesInfo
	called := sourceConstructionCalledObject(info, call.Fun)
	if builtin, ok := called.(*types.Builtin); ok {
		escaped := ""
		for _, argument := range call.Args {
			if escaped = sourceObjectPathExpressionStorageKind(info, argument); escaped != "" {
				break
			}
		}
		if escaped == "" {
			return ""
		}
		switch builtin.Name() {
		case "len", "cap", "make", "new":
			return ""
		case "append":
			if len(call.Args) != 0 {
				container, isSlice := types.Unalias(info.TypeOf(call.Args[0])).(*types.Slice)
				if isSlice && sourceObjectPathStorageContains(
					container,
					map[types.Type]bool{},
				) {
					return ""
				}
			}
		case "copy":
			if len(call.Args) == 2 {
				destination, destinationSlice := types.Unalias(
					info.TypeOf(call.Args[0]),
				).(*types.Slice)
				source, sourceSlice := types.Unalias(
					info.TypeOf(call.Args[1]),
				).(*types.Slice)
				if destinationSlice && sourceSlice && sourceObjectPathStorageContains(
					destination,
					map[types.Type]bool{},
				) && sourceObjectPathStorageContains(
					source,
					map[types.Type]bool{},
				) {
					return ""
				}
			}
		}
		return escaped
	}
	var callSignature *types.Signature
	if called != nil {
		callSignature, _ = called.Type().(*types.Signature)
	}
	if selector, ok := sourceConstructionUnwrapCallFunction(call.Fun).(*ast.SelectorExpr); ok {
		if selection := info.Selections[selector]; selection != nil {
			escaped := sourceObjectPathExpressionStorageKind(
				info,
				selector.X,
			)
			if escaped != "" && (callSignature == nil || callSignature.Recv() == nil ||
				!sourceObjectPathStorageContains(
					callSignature.Recv().Type(),
					map[types.Type]bool{},
				)) {
				return escaped
			}
		}
	}
	if len(call.Args) == 1 {
		if tuple, ok := info.TypeOf(call.Args[0]).(*types.Tuple); ok {
			for index := range tuple.Len() {
				escaped := sourceObjectPathStorageKind(tuple.At(index).Type())
				if escaped == "" {
					continue
				}
				parameter := sourceObjectPathCallParameter(callSignature, call, index)
				if !sourceObjectPathStorageContains(parameter, map[types.Type]bool{}) {
					return escaped
				}
			}
			return ""
		}
	}
	for index, argument := range call.Args {
		escaped := sourceObjectPathExpressionStorageKind(
			info,
			argument,
		)
		if escaped == "" {
			continue
		}
		parameter := sourceObjectPathCallParameter(callSignature, call, index)
		if !sourceObjectPathStorageContains(
			parameter,
			map[types.Type]bool{},
		) {
			return escaped
		}
	}
	return ""
}

func sourceObjectPathCallParameter(
	signature *types.Signature,
	call *ast.CallExpr,
	argumentIndex int,
) types.Type {
	if signature == nil || signature.Params() == nil || signature.Params().Len() == 0 ||
		argumentIndex < 0 {
		return nil
	}
	parameterIndex := argumentIndex
	if parameterIndex >= signature.Params().Len() {
		if !signature.Variadic() {
			return nil
		}
		parameterIndex = signature.Params().Len() - 1
	}
	parameter := signature.Params().At(parameterIndex).Type()
	if signature.Variadic() && parameterIndex == signature.Params().Len()-1 &&
		call != nil && call.Ellipsis == token.NoPos {
		if slice, ok := types.Unalias(parameter).(*types.Slice); ok {
			return slice.Elem()
		}
	}
	return parameter
}

func sourceObjectPathReturnEscapeViolations(
	typedPackage *packages.Package,
	file *ast.File,
	statement *ast.ReturnStmt,
) []string {
	if typedPackage == nil || typedPackage.TypesInfo == nil || file == nil || statement == nil {
		return nil
	}
	function := sourceConstructionEnclosingFunction(file, statement.Pos())
	if function == nil {
		return nil
	}
	callable, _ := typedPackage.TypesInfo.Defs[function.Name].(*types.Func)
	if callable == nil {
		return nil
	}
	signature, _ := callable.Type().(*types.Signature)
	if signature == nil || signature.Results() == nil {
		return nil
	}
	violations := make([]string, 0)
	for index, result := range statement.Results {
		escaped := sourceObjectPathExpressionStorageKind(
			typedPackage.TypesInfo,
			result,
		)
		if escaped == "" {
			continue
		}
		if len(statement.Results) == signature.Results().Len() &&
			index < signature.Results().Len() && sourceObjectPathStorageContains(
			signature.Results().At(index).Type(),
			map[types.Type]bool{},
		) {
			continue
		}
		if len(statement.Results) == 1 {
			if tuple, ok := typedPackage.TypesInfo.TypeOf(result).(*types.Tuple); ok &&
				tuple.Len() == signature.Results().Len() {
				allSensitiveDestinations := true
				for resultIndex := range tuple.Len() {
					if sourceObjectPathStorageContains(
						tuple.At(resultIndex).Type(),
						map[types.Type]bool{},
					) && !sourceObjectPathStorageContains(
						signature.Results().At(resultIndex).Type(),
						map[types.Type]bool{},
					) {
						allSensitiveDestinations = false
						break
					}
				}
				if allSensitiveDestinations {
					continue
				}
			}
		}
		violations = append(violations,
			typedPackage.Fset.Position(result.Pos()).String()+
				" returns "+escaped+" through type-erasing result in "+function.Name.Name,
		)
	}
	return violations
}

func sourceObjectPathConversionCarriesStorage(
	info *types.Info,
	call *ast.CallExpr,
) bool {
	if info == nil || call == nil || len(call.Args) != 1 {
		return false
	}
	target, ok := info.Types[sourceConstructionUnparenthesizedExpression(call.Fun)]
	if !ok || !target.IsType() {
		return false
	}
	return sourceObjectPathStorageContains(target.Type, map[types.Type]bool{}) ||
		sourceObjectPathStorageContains(info.TypeOf(call.Args[0]), map[types.Type]bool{})
}

func sourceObjectPathGenericCallCarriesStorage(
	typedPackage *packages.Package,
	call *ast.CallExpr,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || call == nil {
		return false
	}
	called, ok := sourceConstructionCalledObject(
		typedPackage.TypesInfo,
		call.Fun,
	).(*types.Func)
	if !ok {
		return false
	}
	signature, ok := called.Type().(*types.Signature)
	if !ok {
		return false
	}
	selector, selectorCall := sourceConstructionUnparenthesizedExpression(
		call.Fun,
	).(*ast.SelectorExpr)
	genericCall := signature.TypeParams().Len() != 0 ||
		signature.RecvTypeParams().Len() != 0
	if selectorCall {
		genericCall = genericCall || sourceObjectPathTypeHasArguments(
			typedPackage.TypesInfo.TypeOf(selector.X),
		)
	}
	if !genericCall {
		return false
	}
	if selectorCall && sourceObjectPathStorageContains(
		typedPackage.TypesInfo.TypeOf(selector.X),
		map[types.Type]bool{},
	) {
		return true
	}
	for _, argument := range call.Args {
		if sourceObjectPathStorageContains(
			typedPackage.TypesInfo.TypeOf(argument),
			map[types.Type]bool{},
		) {
			return true
		}
	}
	return false
}

func sourceObjectPathTypeHasArguments(value types.Type) bool {
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	return ok && named.TypeArgs().Len() != 0
}

func sourceObjectPathAllowedInstallAssignment(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	assignment *ast.AssignStmt,
) bool {
	return function != nil && function.Body != nil && len(function.Body.List) == 6 &&
		function.Body.List[4] == assignment &&
		sourceObjectPathValidatedInstallProved(typedPackage, function)
}

func sourceObjectPathAllowedObjectsAssignment(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	assignment *ast.AssignStmt,
) bool {
	_, install, ok := sourceObjectPathRetainedObjectsAcquisition(
		typedPackage,
		function,
	)
	return ok && install == assignment
}

func sourceObjectPathAllowedObjectsConstruction(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	address *ast.UnaryExpr,
) bool {
	construction, _, ok := sourceObjectPathRetainedObjectsAcquisition(
		typedPackage,
		function,
	)
	return ok && construction == address
}

func sourceObjectPathAllowedEnclosingConstruction(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	address *ast.UnaryExpr,
) bool {
	if typedPackage == nil || typedPackage.Types == nil ||
		typedPackage.TypesInfo == nil || function == nil || function.Body == nil ||
		address == nil || address.Op != token.AND {
		return false
	}
	info := typedPackage.TypesInfo
	composite, ok := sourceConstructionUnparenthesizedExpression(address.X).(*ast.CompositeLit)
	if !ok {
		return false
	}
	role := sourceConstructionFunctionRole(
		info.Defs[function.Name],
		typedPackage.Types.Path(),
	)
	switch role {
	case "newSourceConstructionOwner":
		if len(function.Body.List) != 1 {
			return false
		}
		ownerType := typedPackage.Types.Scope().Lookup("sourceConstructionOwner")
		returned, returnOK := function.Body.List[0].(*ast.ReturnStmt)
		return ownerType != nil && returnOK &&
			len(returned.Results) == 1 &&
			sourceConstructionDirectExpression(returned.Results[0], address) &&
			types.Identical(info.TypeOf(composite), ownerType.Type())
	case "retainSourceConstructionWith":
		builderType := typedPackage.Types.Scope().Lookup("sourceConstructionBuilder")
		if builderType == nil || len(function.Body.List) != 5 ||
			!types.Identical(info.TypeOf(composite), builderType.Type()) {
			return false
		}
		assignment, assignmentOK := function.Body.List[0].(*ast.AssignStmt)
		return assignmentOK && assignment.Tok == token.DEFINE &&
			len(assignment.Lhs) == 1 && len(assignment.Rhs) == 1 &&
			sourceConstructionDirectExpression(assignment.Rhs[0], address)
	default:
		return false
	}
}

func sourceObjectPathRetainedObjectsAcquisition(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) (*ast.UnaryExpr, *ast.AssignStmt, bool) {
	if typedPackage == nil || typedPackage.Types == nil ||
		typedPackage.TypesInfo == nil || function == nil || function.Body == nil ||
		len(function.Body.List) != 3 || function.Recv == nil ||
		len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 ||
		sourceConstructionFunctionRole(
			typedPackage.TypesInfo.Defs[function.Name],
			typedPackage.Types.Path(),
		) != "(*sourceConstructionBuilder).retainObjects" {
		return nil, nil, false
	}
	info := typedPackage.TypesInfo
	receiver := function.Recv.List[0].Names[0]
	receiverObject := info.Defs[receiver]
	callable, callableOK := info.Defs[function.Name].(*types.Func)
	if receiver.Name != "builder" || receiverObject == nil || !callableOK {
		return nil, nil, false
	}
	signature, signatureOK := callable.Type().(*types.Signature)
	if !signatureOK || signature.Params().Len() != 0 || signature.Results().Len() != 1 ||
		!types.Identical(signature.Results().At(0).Type(), types.Typ[types.Bool]) ||
		signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 ||
		signature.Variadic() {
		return nil, nil, false
	}
	creation, ok := function.Body.List[0].(*ast.AssignStmt)
	if !ok || creation.Tok != token.DEFINE || len(creation.Lhs) != 1 ||
		len(creation.Rhs) != 1 {
		return nil, nil, false
	}
	objects, objectsOK := creation.Lhs[0].(*ast.Ident)
	address, addressOK := sourceConstructionUnparenthesizedExpression(
		creation.Rhs[0],
	).(*ast.UnaryExpr)
	if !objectsOK || !addressOK || objects.Name != "objects" ||
		address.Op != token.AND {
		return nil, nil, false
	}
	composite, compositeOK := sourceConstructionUnparenthesizedExpression(
		address.X,
	).(*ast.CompositeLit)
	objectsType := typedPackage.Types.Scope().Lookup("retainedSourceObjects")
	if !compositeOK || objectsType == nil || len(composite.Elts) != 0 ||
		!types.Identical(info.TypeOf(composite), objectsType.Type()) {
		return nil, nil, false
	}
	objectsObject := info.Defs[objects]
	install, ok := function.Body.List[1].(*ast.AssignStmt)
	if !ok || objectsObject == nil || install.Tok != token.ASSIGN ||
		len(install.Lhs) != 1 || len(install.Rhs) != 1 ||
		!sourceConstructionSelectorChainProved(
			info,
			install.Lhs[0],
			receiverObject,
			"owner",
			"objects",
		) || !sourceConstructionDirectObjectExpression(
		info,
		install.Rhs[0],
		objectsObject,
	) {
		return nil, nil, false
	}
	return address, install, true
}

func sourceObjectPathAllowedAppendAssignment(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	assignment *ast.AssignStmt,
	parents map[ast.Node]ast.Node,
) bool {
	if assignment == nil || len(assignment.Rhs) != 1 {
		return false
	}
	call, ok := sourceConstructionUnparenthesizedExpression(assignment.Rhs[0]).(*ast.CallExpr)
	return ok && sourceObjectPathAllowedAppendCall(
		typedPackage,
		function,
		call,
		parents,
	)
}

func sourceObjectPathAllowedAppendCall(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 10 || call == nil {
		return false
	}
	info := typedPackage.TypesInfo
	if sourceConstructionFunctionRole(
		info.Defs[function.Name],
		typedPackage.Types.Path(),
	) != "deriveSourceObjectPathInventory" ||
		sourceConstructionCalledObject(info, call.Fun) != types.Universe.Lookup("append") ||
		len(call.Args) != 2 {
		return false
	}
	loop, ok := function.Body.List[5].(*ast.RangeStmt)
	if !ok || loop.Body == nil || len(loop.Body.List) != 8 {
		return false
	}
	assignment, ok := loop.Body.List[5].(*ast.AssignStmt)
	if !ok || parents[call] != assignment || assignment.Tok != token.ASSIGN ||
		len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
		!sourceConstructionDirectExpression(assignment.Rhs[0], call) {
		return false
	}
	rowsDefinition, ok := function.Body.List[2].(*ast.AssignStmt)
	if !ok || rowsDefinition.Tok != token.DEFINE || len(rowsDefinition.Lhs) != 1 ||
		len(rowsDefinition.Rhs) != 1 {
		return false
	}
	rows, rowsOK := rowsDefinition.Lhs[0].(*ast.Ident)
	makeRows, makeOK := rowsDefinition.Rhs[0].(*ast.CallExpr)
	if !rowsOK || !makeOK || rows.Name != "rows" ||
		sourceConstructionCalledObject(info, makeRows.Fun) != types.Universe.Lookup("make") ||
		len(makeRows.Args) != 3 || types.ExprString(makeRows.Args[0]) !=
		"[]sourceObjectPathRow" {
		return false
	}
	rowDefinition, ok := loop.Body.List[3].(*ast.AssignStmt)
	if !ok || rowDefinition.Tok != token.DEFINE || len(rowDefinition.Lhs) != 2 ||
		len(rowDefinition.Rhs) != 1 {
		return false
	}
	row, rowOK := rowDefinition.Lhs[0].(*ast.Ident)
	classify, classifyOK := rowDefinition.Rhs[0].(*ast.CallExpr)
	if !rowOK || !classifyOK || row.Name != "row" ||
		!sourceObjectPathPackageFunctionCallProved(
			typedPackage,
			classify,
			"classifySourceObjectPathRow",
		) {
		return false
	}
	rowsObject := info.Defs[rows]
	rowObject := info.Defs[row]
	return rowsObject != nil && rowObject != nil &&
		sourceConstructionDirectObjectExpression(info, assignment.Lhs[0], rowsObject) &&
		sourceConstructionDirectObjectExpression(info, call.Args[0], rowsObject) &&
		sourceConstructionDirectObjectExpression(info, call.Args[1], rowObject)
}

func sourceObjectPathAllowedInventoryConstruction(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	address *ast.UnaryExpr,
	parents map[ast.Node]ast.Node,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 10 || address == nil ||
		address.Op != token.AND {
		return false
	}
	info := typedPackage.TypesInfo
	if sourceConstructionFunctionRole(
		info.Defs[function.Name],
		typedPackage.Types.Path(),
	) != "deriveSourceObjectPathInventory" {
		return false
	}
	returned, ok := function.Body.List[9].(*ast.ReturnStmt)
	if !ok || parents[address] != returned || len(returned.Results) != 2 ||
		!sourceConstructionDirectExpression(returned.Results[0], address) {
		return false
	}
	nilIdentifier, nilOK := sourceConstructionUnparenthesizedExpression(
		returned.Results[1],
	).(*ast.Ident)
	if !nilOK || info.Uses[nilIdentifier] != types.Universe.Lookup("nil") {
		return false
	}
	composite, ok := sourceConstructionUnparenthesizedExpression(address.X).(*ast.CompositeLit)
	inventoryType := typedPackage.Types.Scope().Lookup("sourceObjectPathInventory")
	if !ok || inventoryType == nil || !types.Identical(
		info.TypeOf(composite),
		inventoryType.Type(),
	) || len(composite.Elts) != 3 {
		return false
	}
	wantObjects := map[string]types.Object{
		"rows": sourceObjectPathUniqueDefinedObject(info, function, "rows"),
		"contentFileCount": sourceObjectPathUniqueDefinedObject(
			info,
			function,
			"contentFileCount",
		),
		"auxiliaryFileCount": sourceObjectPathUniqueDefinedObject(
			info,
			function,
			"auxiliaryFileCount",
		),
	}
	seen := make(map[string]bool)
	for _, element := range composite.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, ok := keyValue.Key.(*ast.Ident)
		field, fieldOK := info.Uses[key].(*types.Var)
		want := wantObjects[key.Name]
		if !ok || !fieldOK || !field.IsField() || want == nil || seen[key.Name] ||
			!sourceConstructionDirectObjectExpression(info, keyValue.Value, want) {
			return false
		}
		seen[key.Name] = true
	}
	return len(seen) == len(wantObjects)
}

func sourceObjectPathUniqueDefinedObject(
	info *types.Info,
	function *ast.FuncDecl,
	name string,
) types.Object {
	if info == nil || function == nil {
		return nil
	}
	var found types.Object
	for identifier, object := range info.Defs {
		if identifier.Name != name || identifier.Pos() < function.Pos() ||
			identifier.End() > function.End() {
			continue
		}
		if found != nil {
			return nil
		}
		found = object
	}
	return found
}

func sourceObjectPathMutationTarget(
	info *types.Info,
	target ast.Expr,
) string {
	if info == nil || target == nil {
		return ""
	}
	target = sourceConstructionUnparenthesizedExpression(target)
	if identifier, ok := target.(*ast.Ident); ok {
		if info.Defs[identifier] != nil {
			return ""
		}
		if kind := sourceObjectPathDirectMutationKind(info.TypeOf(identifier)); kind != "" {
			return kind
		}
		if sourceObjectPathStorageContains(info.TypeOf(identifier), map[types.Type]bool{}) {
			return "sourceObjectPathStorage"
		}
		return ""
	}
	if selector, ok := target.(*ast.SelectorExpr); ok {
		if kind := sourceObjectPathMutationRootKind(info, selector.X); kind != "" {
			return kind
		}
		if sourceObjectPathStorageContains(info.TypeOf(selector), map[types.Type]bool{}) {
			return "sourceObjectPathStorage"
		}
		return ""
	}
	if kind := sourceObjectPathMutationRootKind(info, target); kind != "" {
		return kind
	}
	if sourceObjectPathStorageContains(info.TypeOf(target), map[types.Type]bool{}) {
		return "sourceObjectPathStorage"
	}
	return ""
}

func sourceObjectPathMutationRootKind(info *types.Info, expression ast.Expr) string {
	for expression != nil {
		expression = sourceConstructionUnparenthesizedExpression(expression)
		if kind := sourceObjectPathDirectMutationKind(info.TypeOf(expression)); kind != "" {
			return kind
		}
		switch current := expression.(type) {
		case *ast.SelectorExpr:
			expression = current.X
		case *ast.IndexExpr:
			expression = current.X
		case *ast.IndexListExpr:
			expression = current.X
		case *ast.SliceExpr:
			expression = current.X
		case *ast.StarExpr:
			expression = current.X
		default:
			return ""
		}
	}
	return ""
}

func sourceObjectPathDirectMutationKind(value types.Type) string {
	switch sourceConstructionTypeName(value) {
	case "sourceObjectPathRow":
		return "sourceObjectPathRow"
	case "sourceObjectPathInventory":
		return "sourceObjectPathInventory"
	default:
		return ""
	}
}

func sourceObjectPathStorageContains(value types.Type, seen map[types.Type]bool) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	if sourceObjectPathDirectMutationKind(value) != "" {
		return true
	}
	switch current := value.(type) {
	case *types.Pointer:
		return sourceObjectPathStorageContains(current.Elem(), seen)
	case *types.Slice:
		return sourceObjectPathStorageContains(current.Elem(), seen)
	case *types.Array:
		return sourceObjectPathStorageContains(current.Elem(), seen)
	case *types.Map:
		return sourceObjectPathStorageContains(current.Key(), seen) ||
			sourceObjectPathStorageContains(current.Elem(), seen)
	case *types.Tuple:
		for variable := range current.Variables() {
			if sourceObjectPathStorageContains(variable.Type(), seen) {
				return true
			}
		}
		return false
	case *types.Struct:
		for field := range current.Fields() {
			if sourceObjectPathStorageContains(field.Type(), seen) {
				return true
			}
		}
		return false
	case *types.Named:
		return sourceObjectPathStorageContains(current.Underlying(), seen)
	default:
		return false
	}
}

func sourceObjectPathRoleDeclarationViolations(typedPackage *packages.Package) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for source object path role declaration audit"}
	}
	wantNames := []string{
		"sourceObjectDirectoryPath",
		"sourceObjectLooseContentPath",
		"sourceObjectPackContentPath",
		"sourceObjectPackIndexPath",
		"sourceObjectPackAuxiliaryPath",
		"sourceObjectCommitGraphPath",
		"sourceObjectCommitGraphChainPath",
		"sourceObjectSplitCommitGraphPath",
		"sourceObjectPackListPath",
		"sourceObjectMultiPackIndexPath",
		"sourceObjectMultiPackIndexAuxiliaryPath",
		"sourceObjectMultiPackIndexChainPath",
		"sourceObjectMultiPackIndexLayerPath",
		"sourceObjectMultiPackIndexLayerAuxiliaryPath",
	}
	violations := make([]string, 0)
	typeObject, ok := typedPackage.Types.Scope().Lookup("sourceObjectPathRole").(*types.TypeName)
	if !ok {
		return []string{"source object path role declaration is missing"}
	}
	named, namedType := types.Unalias(typeObject.Type()).(*types.Named)
	var underlying *types.Basic
	if namedType {
		underlying, _ = named.Underlying().(*types.Basic)
	}
	if !namedType || underlying == nil || underlying.Kind() != types.Uint8 {
		violations = append(violations,
			"source object path role declaration does not have exact uint8 underlying type",
		)
	}
	want := make(map[string]int, len(wantNames))
	for index, name := range wantNames {
		want[name] = index + 1
	}
	observed := make(map[string]bool)
	for _, name := range typedPackage.Types.Scope().Names() {
		candidate, constantObject := typedPackage.Types.Scope().Lookup(name).(*types.Const)
		if !constantObject || !types.Identical(candidate.Type(), typeObject.Type()) {
			continue
		}
		observed[name] = true
		value, exact := constant.Int64Val(candidate.Val())
		if wantValue, admitted := want[name]; !admitted || !exact || value != int64(wantValue) {
			violations = append(violations,
				"source object path role declaration contains unexpected value "+name,
			)
		}
	}
	for _, name := range wantNames {
		if !observed[name] {
			violations = append(violations,
				"source object path role declaration is missing "+name,
			)
		}
	}
	if !sourceObjectPathRoleASTDeclarationProved(typedPackage, wantNames) {
		violations = append(violations,
			"source object path role declaration is outside exact audited shape",
		)
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectPathRoleASTDeclarationProved(
	typedPackage *packages.Package,
	wantNames []string,
) bool {
	var typeDeclarations int
	var roleDeclarations int
	for _, file := range typedPackage.Syntax {
		if filepath.Base(typedPackage.Fset.Position(file.Package).Filename) !=
			sourceObjectPathSourceFilename {
			continue
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			if general.Tok == token.TYPE {
				for _, specification := range general.Specs {
					typeSpec, typeOK := specification.(*ast.TypeSpec)
					if !typeOK || typeSpec.Name.Name != "sourceObjectPathRole" {
						continue
					}
					typeDeclarations++
					if typeSpec.Assign.IsValid() || types.ExprString(typeSpec.Type) != "uint8" {
						return false
					}
				}
				continue
			}
			if general.Tok != token.CONST || !sourceObjectPathConstDeclaration(
				typedPackage.TypesInfo,
				general,
			) {
				continue
			}
			roleDeclarations++
			if !sourceObjectPathExactConstSpecs(general, wantNames) {
				return false
			}
		}
	}
	return typeDeclarations == 1 && roleDeclarations == 1
}

func sourceObjectPathConstDeclaration(info *types.Info, declaration *ast.GenDecl) bool {
	for _, specification := range declaration.Specs {
		valueSpec, ok := specification.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range valueSpec.Names {
			object, constantObject := info.Defs[name].(*types.Const)
			if constantObject && sourceConstructionTypeName(object.Type()) == "sourceObjectPathRole" {
				return true
			}
		}
	}
	return false
}

func sourceObjectPathExactConstSpecs(declaration *ast.GenDecl, wantNames []string) bool {
	if !declaration.Lparen.IsValid() || len(declaration.Specs) != len(wantNames) {
		return false
	}
	for index, specification := range declaration.Specs {
		valueSpec, ok := specification.(*ast.ValueSpec)
		if !ok || len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != wantNames[index] {
			return false
		}
		if index == 0 {
			if valueSpec.Type == nil || types.ExprString(valueSpec.Type) !=
				"sourceObjectPathRole" || len(valueSpec.Values) != 1 ||
				types.ExprString(valueSpec.Values[0]) != "iota + 1" {
				return false
			}
			continue
		}
		if valueSpec.Type != nil || len(valueSpec.Values) != 0 {
			return false
		}
	}
	return true
}

func sourceObjectPathFunctionDeclaration(
	typedPackage *packages.Package,
	filename string,
	role string,
) *ast.FuncDecl {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return nil
	}
	var found *ast.FuncDecl
	for _, file := range typedPackage.Syntax {
		if filepath.Base(typedPackage.Fset.Position(file.Package).Filename) != filename {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			) != role {
				continue
			}
			if found != nil {
				return nil
			}
			found = function
		}
	}
	return found
}

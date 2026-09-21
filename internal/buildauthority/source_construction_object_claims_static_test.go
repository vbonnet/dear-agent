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

// The following audits are filled in below in this file. Keeping their entry
// points independent lets each exact inventory be exercised by negative
// overlays without coupling diagnostics.

const (
	sourceObjectClaimSourceFilename       = "source_construction_object_claims.go"
	sourceObjectClaimBytesSourceFilename  = "source_construction_auxiliary_bytes.go"
	sourceObjectClaimRevalidationFilename = "source_construction_revalidation.go"
)

func TestSourceObjectAuxiliaryClaimStaticGovernance(t *testing.T) {
	typedPackage := loadTypedSourceConstructionPackage(t, nil)
	violations := sourceObjectClaimValidatedTransactionViolations(typedPackage)
	violations = append(violations, sourceObjectClaimFullPrefixViolations(typedPackage)...)
	violations = append(violations, sourceObjectClaimConstructionViolations(typedPackage)...)
	violations = append(violations, sourceObjectClaimEscapeViolations(typedPackage)...)
	violations = append(violations, sourceObjectClaimPresenceAndCallerViolations(typedPackage)...)
	violations = append(violations, sourceObjectClaimAdministrationMatchCallerViolations(typedPackage)...)
	violations = append(violations, sourceObjectClaimCriticalCallerViolations(typedPackage)...)
	violations = append(violations, sourceObjectClaimRevalidationProtocolViolations(typedPackage)...)
	violations = append(violations, sourceObjectClaimRevalidationWriteViolations(typedPackage)...)
	if len(violations) != 0 {
		t.Fatalf(
			"source object auxiliary claim static-governance violations:\n%s",
			strings.Join(violations, "\n"),
		)
	}
}

func TestSourceObjectAuxiliaryClaimStaticGovernanceRejectsBypasses(t *testing.T) {
	const install = "\tbuilder.owner.objects.claims = claims\n"
	const validation = `	if failure := validateSourceObjectAuxiliaryClaimInventory(
		builder.ctx,
		builder.owner.config.claim.objectFormat,
		paths,
		claims,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return nil, false
	}
`
	const beforeCaptureComparison = `	if !captured {
		return false
	}
	if failure = compareRevalidatedSourceObjectAuxiliaryClaimInventories(`
	for _, test := range []struct {
		name        string
		filename    string
		original    string
		replacement string
		suffix      string
		want        string
	}{
		{
			name:     "skipped full-prefix revalidation",
			filename: "source_construction_acquire.go",
			original: `			builder.retainSourceObjectAuxiliaryClaimInventory() &&
				builder.revalidateSourceObjectAuxiliaryClaimInventory()`,
			replacement: "\t\t\tbuilder.retainSourceObjectAuxiliaryClaimInventory()",
			want:        "full-prefix orchestrator is outside audited shape",
		},
		{
			name:        "false stage result is ignored",
			filename:    "source_construction_acquire.go",
			original:    "\t\t\tif !completed && builder.outcome.proved() {\n",
			replacement: "\t\t\tif !completed && !builder.outcome.proved() {\n",
			want:        "full-prefix orchestrator is outside audited shape",
		},
		{
			name:        "final validation is downgraded",
			filename:    sourceObjectClaimRevalidationFilename,
			original:    "if owner == nil || !owner.validObjectClaimRetention() {",
			replacement: "if owner == nil || !owner.validInitialRetention() {",
			want:        "full-prefix orchestrator is outside audited shape",
		},
		{
			name:     "second production orchestrator caller",
			filename: "source_construction_acquire.go",
			suffix: `

func sourceObjectClaimStaticFixtureCallInitialSeam(
	ctx context.Context,
	locator sourceRepositoryLocator,
	primitives sourcePrimitives,
) (*sourceConstructionOwner, sourceUseOutcome) {
	return retainSourceConstructionWith(ctx, locator, primitives)
}
`,
			want: "initial construction orchestrator caller is outside audited sites",
		},
		{
			name:     "extra administration match callers",
			filename: sourceObjectClaimSourceFilename,
			suffix: `

func sourceObjectClaimStaticFixtureExtraMatchCallers(
	administration *sourceAdministrativeInventory,
	claims *sourceObjectAuxiliaryClaimInventory,
) bool {
	matches, failure := sourceObjectAuxiliaryClaimsMatchAdministrationValidity(
		context.Background(),
		OperationValidate,
		administration,
		claims,
	)
	return sourceObjectAuxiliaryClaimsMatchAdministration(administration, claims) &&
		matches && failure == nil
}
`,
			want: "administration-match caller is outside audited sites",
		},
		{
			name:     "pre-read envelope binding skipped",
			filename: sourceObjectClaimRevalidationFilename,
			original: `	if failure := builder.revalidateSourceEnvelopePathBindings(
		sourceEnvelopePreReadBinding,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if !builder.outcome.proved() {
		return false
	}
`,
			want: "revalidation protocol is outside audited shape",
		},
		{
			name:     "second administration pass reused stale scratch",
			filename: sourceObjectClaimRevalidationFilename,
			original: `	postAdministration, captured := builder.captureRevalidatedSourceAdministrativeInventory()
	if !captured {
		return false
	}
`,
			replacement: "\tpostAdministration := administration\n",
			want:        "revalidation protocol is outside audited shape",
		},
		{
			name:     "post-read close-tail gate skipped",
			filename: sourceObjectClaimRevalidationFilename,
			original: `	if !builder.outcome.proved() {
		return false
	}
	if failure = compareRevalidatedDeferredSourceDescriptorEvidence(`,
			replacement: "\tif failure = compareRevalidatedDeferredSourceDescriptorEvidence(",
			want:        "revalidation protocol is outside audited shape",
		},
		{
			name:     "forged install",
			filename: sourceObjectClaimSourceFilename,
			original: install,
			replacement: "\t_ = claims\n" +
				"\tbuilder.owner.objects.claims = &sourceObjectAuxiliaryClaimInventory{}\n",
			want: "validated transaction is outside audited shape",
		},
		{
			name:     "skipped validation",
			filename: sourceObjectClaimSourceFilename,
			original: validation,
			want:     "validated transaction is outside audited shape",
		},
		{
			name:     "post-validation mutation",
			filename: sourceObjectClaimSourceFilename,
			original: "\treturn claims, true\n",
			replacement: "\tclaims.rows = append(claims.rows, sourceObjectAuxiliaryClaim{})\n" +
				"\treturn claims, true\n",
			want: "validated transaction is outside audited shape",
		},
		{
			name:     "retained revalidation scratch",
			filename: sourceObjectClaimRevalidationFilename,
			original: beforeCaptureComparison,
			replacement: strings.Replace(
				beforeCaptureComparison,
				"\tif failure = compareRevalidatedSourceObjectAuxiliaryClaimInventories(",
				"\tbuilder.owner.objects.claims = claims\n\tif failure = compareRevalidatedSourceObjectAuxiliaryClaimInventories(",
				1,
			),
			want: "revalidation writes retained claims",
		},
		{
			name:     "claim interface erasure",
			filename: sourceObjectClaimSourceFilename,
			suffix: `

func sourceObjectClaimStaticFixtureEraseClaim(claim sourceObjectAuxiliaryClaim) any {
	return claim
}
`,
			want: "returns sourceObjectAuxiliaryClaim through type-erasing result",
		},
		{
			name:     "inventory interface erasure",
			filename: sourceObjectClaimSourceFilename,
			suffix: `

func sourceObjectClaimStaticFixtureEraseInventory(
	claims *sourceObjectAuxiliaryClaimInventory,
) any {
	return claims
}
`,
			want: "returns sourceObjectAuxiliaryClaimInventory through type-erasing result",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			overlay := sourceObjectClaimOverlay(
				t,
				test.filename,
				test.original,
				test.replacement,
				test.suffix,
			)
			typedPackage := loadTypedSourceConstructionPackage(t, overlay)
			violations := sourceObjectClaimValidatedTransactionViolations(typedPackage)
			violations = append(violations, sourceObjectClaimFullPrefixViolations(typedPackage)...)
			violations = append(violations, sourceObjectClaimConstructionViolations(typedPackage)...)
			violations = append(violations, sourceObjectClaimEscapeViolations(typedPackage)...)
			violations = append(violations, sourceObjectClaimPresenceAndCallerViolations(typedPackage)...)
			violations = append(violations, sourceObjectClaimAdministrationMatchCallerViolations(typedPackage)...)
			violations = append(violations, sourceObjectClaimCriticalCallerViolations(typedPackage)...)
			violations = append(violations, sourceObjectClaimRevalidationProtocolViolations(typedPackage)...)
			violations = append(violations, sourceObjectClaimRevalidationWriteViolations(typedPackage)...)
			joined := strings.Join(violations, "\n")
			if !strings.Contains(joined, test.want) {
				t.Fatalf("claim governance violations =\n%s\nwant %q", joined, test.want)
			}
		})
	}
}

func sourceObjectClaimOverlay(
	t *testing.T,
	filename string,
	original string,
	replacement string,
	suffix string,
) map[string][]byte {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	path := filepath.Join(workingDirectory, filename)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source object claim fixture: %v", err)
	}
	if original != "" {
		if count := strings.Count(string(source), original); count != 1 {
			t.Fatalf("source object claim overlay match count = %d, want 1", count)
		}
		source = []byte(strings.Replace(string(source), original, replacement, 1))
	}
	if suffix != "" {
		source = append(append([]byte(nil), source...), []byte(suffix)...)
	}
	return map[string][]byte{path: source}
}

func sourceObjectClaimValidatedTransactionViolations(
	typedPackage *packages.Package,
) []string {
	retain := sourceObjectPathFunctionDeclaration(
		typedPackage,
		sourceObjectClaimSourceFilename,
		"(*sourceConstructionBuilder).retainSourceObjectAuxiliaryClaimInventory",
	)
	capture := sourceObjectPathFunctionDeclaration(
		typedPackage,
		sourceObjectClaimSourceFilename,
		"(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaimInventory",
	)
	if sourceObjectClaimRetainTransactionProved(typedPackage, retain) &&
		sourceObjectClaimCaptureTransactionProved(typedPackage, capture) {
		return nil
	}
	position := "source object auxiliary claim transaction"
	if typedPackage != nil && typedPackage.Fset != nil {
		switch {
		case !sourceObjectClaimRetainTransactionProved(typedPackage, retain) && retain != nil:
			position = typedPackage.Fset.Position(retain.Pos()).String()
		case capture != nil:
			position = typedPackage.Fset.Position(capture.Pos()).String()
		}
	}
	return []string{position + " source object claim validated transaction is outside audited shape"}
}

func sourceObjectClaimRetainTransactionProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 5 {
		return false
	}
	info := typedPackage.TypesInfo
	receiverObject, signatureOK := sourceObjectPathRetentionSignatureProved(
		typedPackage,
		function,
	)
	if !signatureOK || !sourceObjectClaimRequestGuardProved(
		typedPackage,
		function.Body.List[0],
		receiverObject,
	) {
		return false
	}
	capture, ok := function.Body.List[1].(*ast.AssignStmt)
	if !ok || capture.Tok != token.DEFINE || len(capture.Lhs) != 2 ||
		len(capture.Rhs) != 1 {
		return false
	}
	claims, claimsOK := capture.Lhs[0].(*ast.Ident)
	captured, capturedOK := capture.Lhs[1].(*ast.Ident)
	call, callOK := sourceConstructionUnparenthesizedExpression(
		capture.Rhs[0],
	).(*ast.CallExpr)
	if !claimsOK || !capturedOK || !callOK || claims.Name != "claims" ||
		captured.Name != "captured" || len(call.Args) != 3 ||
		!sourceObjectPathMethodCallProved(
			typedPackage,
			call,
			receiverObject,
			"(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaimInventory",
		) || !sourceConstructionSelectorChainProved(
		info,
		call.Args[0],
		receiverObject,
		"owner",
		"git",
		"administration",
	) || !sourceConstructionSelectorChainProved(
		info,
		call.Args[1],
		receiverObject,
		"owner",
		"objects",
		"paths",
	) || !sourceObjectClaimConstantExpression(
		info,
		call.Args[2],
		"sourceInitialRequired",
	) {
		return false
	}
	claimsObject := info.Defs[claims]
	capturedObject := info.Defs[captured]
	if claimsObject == nil || capturedObject == nil ||
		!sourceObjectClaimCapturedGuardProved(
			info,
			function.Body.List[2],
			capturedObject,
		) {
		return false
	}
	install, ok := function.Body.List[3].(*ast.AssignStmt)
	return ok && install.Tok == token.ASSIGN && len(install.Lhs) == 1 &&
		len(install.Rhs) == 1 && sourceConstructionSelectorChainProved(
		info,
		install.Lhs[0],
		receiverObject,
		"owner",
		"objects",
		"claims",
	) && sourceConstructionDirectObjectExpression(
		info,
		install.Rhs[0],
		claimsObject,
	) && sourceConstructionSingleBooleanReturnProved(function.Body.List[4], "true")
}

func sourceObjectClaimRequestGuardProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	receiverObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return false
	}
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
	call, callOK := sourceConstructionUnparenthesizedExpression(
		assignment.Rhs[0],
	).(*ast.CallExpr)
	if !failureOK || !callOK || failure.Name != "failure" || len(call.Args) != 0 ||
		!sourceObjectPathMethodCallProved(
			typedPackage,
			call,
			receiverObject,
			"(*sourceConstructionBuilder).validateSourceObjectAuxiliaryClaimRequest",
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

func sourceObjectClaimCapturedGuardProved(
	info *types.Info,
	statement ast.Stmt,
	capturedObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	negation, ok := sourceConstructionUnparenthesizedExpression(
		branch.Cond,
	).(*ast.UnaryExpr)
	return ok && negation.Op == token.NOT && sourceConstructionDirectObjectExpression(
		info,
		negation.X,
		capturedObject,
	) && sourceConstructionSingleBooleanReturnProved(branch.Body.List[0], "false")
}

func sourceObjectClaimCaptureTransactionProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 20 ||
		!sourceObjectClaimCaptureSignatureProved(typedPackage, function) {
		return false
	}
	info := typedPackage.TypesInfo
	receiverObject := functionReceiverObject(info, function)
	administrationObject := sourceObjectPathUniqueDefinedObject(info, function, "administration")
	pathsObject := sourceObjectPathUniqueDefinedObject(info, function, "paths")
	presenceObject := sourceObjectPathUniqueDefinedObject(info, function, "presence")
	inputProved := sourceObjectClaimCaptureInputValidationProved(
		typedPackage,
		function,
		function.Body.List[:10],
		receiverObject,
		administrationObject,
		pathsObject,
		presenceObject,
	)
	rootProved := sourceObjectClaimCaptureRootComparisonProved(
		typedPackage,
		function,
		function.Body.List[10],
		function.Body.List[11],
		function.Body.List[12],
		receiverObject,
		presenceObject,
	)
	if receiverObject == nil || administrationObject == nil || pathsObject == nil ||
		presenceObject == nil || !inputProved || !rootProved {
		return false
	}
	claimsObject, constructionOK := sourceObjectClaimInventoryConstructionProved(
		typedPackage,
		function,
		function.Body.List[13],
	)
	if !constructionOK {
		return false
	}
	if pathsObject == nil || administrationObject == nil || presenceObject == nil ||
		!sourceObjectClaimCaptureLoopProved(
			typedPackage,
			function.Body.List[14],
			claimsObject,
			pathsObject,
			administrationObject,
			presenceObject,
		) || !sourceObjectClaimInventoryValidationProved(
		typedPackage,
		function.Body.List[15],
		claimsObject,
		pathsObject,
	) || !sourceObjectClaimAdministrationMatchProved(
		typedPackage,
		function.Body.List[16],
		function.Body.List[17],
		function.Body.List[18],
		claimsObject,
		administrationObject,
	) {
		return false
	}
	returned, ok := function.Body.List[19].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 2 && sourceConstructionDirectObjectExpression(
		info,
		returned.Results[0],
		claimsObject,
	) && sourceObjectClaimBooleanExpression(info, returned.Results[1], true)
}

func sourceObjectClaimCaptureRootComparisonProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	declarationStatement ast.Stmt,
	selectionStatement ast.Stmt,
	guardStatement ast.Stmt,
	receiverObject types.Object,
	presenceObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil {
		return false
	}
	info := typedPackage.TypesInfo
	declaration, ok := declarationStatement.(*ast.DeclStmt)
	if !ok {
		return false
	}
	general, ok := declaration.Decl.(*ast.GenDecl)
	if !ok || general.Tok != token.VAR || len(general.Specs) != 1 {
		return false
	}
	value, ok := general.Specs[0].(*ast.ValueSpec)
	if !ok || len(value.Names) != 1 || len(value.Values) != 0 ||
		value.Names[0].Name != "rootFailure" || types.ExprString(value.Type) !=
		"*sourcePrimitiveFailure" {
		return false
	}
	rootFailureObject := info.Defs[value.Names[0]]
	selection, ok := selectionStatement.(*ast.IfStmt)
	if !ok || rootFailureObject == nil || selection.Init != nil ||
		len(selection.Body.List) != 1 {
		return false
	}
	comparison, ok := sourceConstructionUnparenthesizedExpression(selection.Cond).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.EQL || !sourceConstructionDirectObjectExpression(
		info,
		comparison.X,
		presenceObject,
	) || !sourceObjectClaimConstantExpression(
		info,
		comparison.Y,
		"sourceRevalidatePresent",
	) {
		return false
	}
	alternative, ok := selection.Else.(*ast.BlockStmt)
	return ok && len(alternative.List) == 1 && sourceObjectClaimRootFailureAssignmentProved(
		typedPackage,
		selection.Body.List[0],
		receiverObject,
		rootFailureObject,
		"(*sourceConstructionBuilder).compareSourceDirectoryBeforeWalk",
		2,
	) && sourceObjectClaimRootFailureAssignmentProved(
		typedPackage,
		alternative.List[0],
		receiverObject,
		rootFailureObject,
		"(*sourceConstructionBuilder).compareSourceDescriptorWithoutPolicy",
		3,
	) && sourceObjectClaimFailureAndNilFalseGuardProved(
		typedPackage,
		guardStatement,
		receiverObject,
		rootFailureObject,
	)
}

func sourceObjectClaimRootFailureAssignmentProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	receiverObject types.Object,
	failureObject types.Object,
	role string,
	wantArguments int,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return false
	}
	info := typedPackage.TypesInfo
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || !sourceConstructionDirectObjectExpression(
		info,
		assignment.Lhs[0],
		failureObject,
	) {
		return false
	}
	call, ok := sourceConstructionUnparenthesizedExpression(assignment.Rhs[0]).(*ast.CallExpr)
	if !ok || len(call.Args) != wantArguments || !sourceObjectPathMethodCallProved(
		typedPackage,
		call,
		receiverObject,
		role,
	) || !sourceConstructionSelectorChainProved(
		info,
		call.Args[0],
		receiverObject,
		"owner",
		"objects",
		"root",
		"descriptor",
	) || !sourceConstructionSelectorChainProved(
		info,
		call.Args[1],
		receiverObject,
		"owner",
		"objects",
		"root",
		"evidence",
	) {
		return false
	}
	return wantArguments == 2 || sourceObjectClaimConstantExpression(
		info,
		call.Args[2],
		"sourceObservedDirectory",
	)
}

func sourceObjectClaimCaptureSignatureProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Recv == nil || len(function.Recv.List) != 1 ||
		len(function.Recv.List[0].Names) != 1 {
		return false
	}
	receiver := function.Recv.List[0].Names[0]
	callable, ok := typedPackage.TypesInfo.Defs[function.Name].(*types.Func)
	if receiver.Name != "builder" || callable == nil || !ok {
		return false
	}
	signature, ok := callable.Type().(*types.Signature)
	return ok && signature.Recv() != nil && signature.Params().Len() == 3 &&
		signature.Results().Len() == 2 &&
		sourceConstructionTypeName(signature.Params().At(0).Type()) ==
			"sourceAdministrativeInventory" &&
		sourceConstructionTypeName(signature.Params().At(1).Type()) ==
			"sourceObjectPathInventory" &&
		sourceConstructionTypeName(signature.Params().At(2).Type()) ==
			"sourcePresenceMode" &&
		sourceConstructionTypeName(signature.Results().At(0).Type()) ==
			"sourceObjectAuxiliaryClaimInventory" &&
		types.Identical(signature.Results().At(1).Type(), types.Typ[types.Bool]) &&
		signature.TypeParams().Len() == 0 && signature.RecvTypeParams().Len() == 0 &&
		!signature.Variadic()
}

func sourceObjectClaimCaptureInputValidationProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	statements []ast.Stmt,
	receiverObject types.Object,
	administrationObject types.Object,
	pathsObject types.Object,
	presenceObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		len(statements) != 10 {
		return false
	}
	info := typedPackage.TypesInfo
	shallow, ok := statements[0].(*ast.IfStmt)
	if !ok || shallow.Init != nil || shallow.Else != nil || len(shallow.Body.List) != 2 ||
		types.ExprString(shallow.Cond) != "builder == nil || builder.owner == nil || "+
			"builder.primitives == nil || builder.owner.git == nil || "+
			"builder.owner.config == nil || builder.owner.objects == nil || "+
			"administration == nil || paths == nil || "+
			"(presence != sourceInitialRequired && presence != sourceRevalidatePresent)" {
		return false
	}
	conditionalAdd, ok := shallow.Body.List[0].(*ast.IfStmt)
	if !ok || conditionalAdd.Init != nil || conditionalAdd.Else != nil ||
		len(conditionalAdd.Body.List) != 1 || !sourceConstructionObjectNilComparison(
		info,
		conditionalAdd.Cond,
		receiverObject,
		token.NEQ,
	) || !sourceObjectClaimInvariantAddProved(
		typedPackage,
		conditionalAdd.Body.List[0],
		receiverObject,
	) || !sourceObjectClaimNilFalseReturnProved(info, shallow.Body.List[1]) {
		return false
	}

	operationAssignment, ok := statements[1].(*ast.AssignStmt)
	if !ok || operationAssignment.Tok != token.DEFINE || len(operationAssignment.Lhs) != 1 ||
		len(operationAssignment.Rhs) != 1 {
		return false
	}
	validationOperation, ok := operationAssignment.Lhs[0].(*ast.Ident)
	if !ok || validationOperation.Name != "validationOperation" ||
		!sourceObjectClaimConstantExpression(info, operationAssignment.Rhs[0], "OperationValidate") {
		return false
	}
	validationOperationObject := info.Defs[validationOperation]
	if validationOperationObject == nil || !sourceObjectClaimPresenceOperationSelectionProved(
		info,
		statements[2],
		presenceObject,
		validationOperationObject,
	) || !sourceObjectClaimContextGuardProved(
		typedPackage,
		statements[3],
		receiverObject,
		validationOperationObject,
	) {
		return false
	}

	gitEvidenceObject, gitOK := sourceObjectClaimEvidenceBindingProved(
		info,
		statements[4],
		receiverObject,
		"gitEvidence",
		[]string{"owner", "git", "root", "evidence"},
	)
	objectsEvidenceObject, objectsOK := sourceObjectClaimEvidenceBindingProved(
		info,
		statements[5],
		receiverObject,
		"objectsEvidence",
		[]string{"owner", "objects", "root", "evidence"},
	)
	if !gitOK || !objectsOK || !sourceObjectClaimRevalidationEvidenceSelectionProved(
		typedPackage,
		statements[6],
		receiverObject,
		administrationObject,
		presenceObject,
		gitEvidenceObject,
		objectsEvidenceObject,
	) {
		return false
	}

	inputsAssignment, ok := statements[7].(*ast.AssignStmt)
	if !ok || inputsAssignment.Tok != token.DEFINE || len(inputsAssignment.Lhs) != 1 ||
		len(inputsAssignment.Rhs) != 1 {
		return false
	}
	inputsValid, ok := inputsAssignment.Lhs[0].(*ast.Ident)
	combined, combinedOK := sourceConstructionUnparenthesizedExpression(
		inputsAssignment.Rhs[0],
	).(*ast.BinaryExpr)
	if !ok || !combinedOK || inputsValid.Name != "inputsValid" || combined.Op != token.LAND {
		return false
	}
	administrationCall, adminOK := sourceConstructionUnparenthesizedExpression(
		combined.X,
	).(*ast.CallExpr)
	pathsCall, pathsOK := sourceConstructionUnparenthesizedExpression(combined.Y).(*ast.CallExpr)
	if !adminOK || !pathsOK || len(administrationCall.Args) != 5 || len(pathsCall.Args) != 2 ||
		!sourceObjectPathMethodCallProved(
			typedPackage,
			administrationCall,
			administrationObject,
			"(*sourceAdministrativeInventory).valid",
		) || !sourceObjectPathMethodCallProved(
		typedPackage,
		pathsCall,
		pathsObject,
		"(*sourceObjectPathInventory).valid",
	) || !sourceConstructionDirectObjectExpression(
		info,
		administrationCall.Args[2],
		gitEvidenceObject,
	) || !sourceConstructionDirectObjectExpression(
		info,
		administrationCall.Args[4],
		objectsEvidenceObject,
	) || !sourceConstructionDirectObjectExpression(
		info,
		pathsCall.Args[1],
		administrationObject,
	) {
		return false
	}
	inputsValidObject := info.Defs[inputsValid]
	return inputsValidObject != nil && sourceObjectClaimContextGuardProved(
		typedPackage,
		statements[8],
		receiverObject,
		validationOperationObject,
	) && sourceObjectClaimNegatedInvariantGuardProved(
		typedPackage,
		statements[9],
		receiverObject,
		inputsValidObject,
	)
}

func sourceObjectClaimInvariantAddProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	receiverObject types.Object,
) bool {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := sourceConstructionUnparenthesizedExpression(expression.X).(*ast.CallExpr)
	return ok && sourceObjectPathMethodCallProved(
		typedPackage,
		call,
		receiverObject,
		"(*sourceUseOutcome).addPrimitive",
		"outcome",
	) && sourceConstructionCallFingerprint(call) ==
		"builder.outcome.addPrimitive(newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant))"
}

func sourceObjectClaimPresenceOperationSelectionProved(
	info *types.Info,
	statement ast.Stmt,
	presenceObject types.Object,
	operationObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	comparison, ok := sourceConstructionUnparenthesizedExpression(branch.Cond).(*ast.BinaryExpr)
	assignment, assignmentOK := branch.Body.List[0].(*ast.AssignStmt)
	return ok && assignmentOK && comparison.Op == token.EQL &&
		sourceConstructionDirectObjectExpression(info, comparison.X, presenceObject) &&
		sourceObjectClaimConstantExpression(info, comparison.Y, "sourceRevalidatePresent") &&
		assignment.Tok == token.ASSIGN && len(assignment.Lhs) == 1 &&
		len(assignment.Rhs) == 1 && sourceConstructionDirectObjectExpression(
		info,
		assignment.Lhs[0],
		operationObject,
	) && sourceObjectClaimConstantExpression(info, assignment.Rhs[0], "OperationCompare")
}

func sourceObjectClaimContextGuardProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	receiverObject types.Object,
	operationObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return false
	}
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
	call, callOK := sourceConstructionUnparenthesizedExpression(assignment.Rhs[0]).(*ast.CallExpr)
	if !failureOK || !callOK || failure.Name != "failure" || len(call.Args) != 2 ||
		!sourceObjectPathPackageFunctionCallProved(
			typedPackage,
			call,
			"sourceContextPrimitiveFailure",
		) || !sourceConstructionSelectorChainProved(
		info,
		call.Args[0],
		receiverObject,
		"ctx",
	) || !sourceConstructionDirectObjectExpression(info, call.Args[1], operationObject) {
		return false
	}
	failureObject := info.Defs[failure]
	return failureObject != nil && sourceObjectPathObjectNilComparison(
		info,
		branch.Cond,
		failureObject,
		token.NEQ,
	) && sourceObjectClaimFailureAndNilFalseReturnProved(
		typedPackage,
		branch.Body.List,
		receiverObject,
		failureObject,
	)
}

func sourceObjectClaimEvidenceBindingProved(
	info *types.Info,
	statement ast.Stmt,
	receiverObject types.Object,
	name string,
	fields []string,
) (types.Object, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return nil, false
	}
	identifier, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok || identifier.Name != name || !sourceConstructionSelectorChainProved(
		info,
		assignment.Rhs[0],
		receiverObject,
		fields...,
	) {
		return nil, false
	}
	object := info.Defs[identifier]
	return object, object != nil
}

func sourceObjectClaimRevalidationEvidenceSelectionProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	receiverObject types.Object,
	administrationObject types.Object,
	presenceObject types.Object,
	gitEvidenceObject types.Object,
	objectsEvidenceObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return false
	}
	info := typedPackage.TypesInfo
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 5 {
		return false
	}
	comparison, ok := sourceConstructionUnparenthesizedExpression(branch.Cond).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.EQL || !sourceConstructionDirectObjectExpression(
		info,
		comparison.X,
		presenceObject,
	) || !sourceObjectClaimConstantExpression(
		info,
		comparison.Y,
		"sourceRevalidatePresent",
	) {
		return false
	}
	gitRowObject, gitPresentObject, gitOK := sourceObjectClaimAdministrativeRowLookupProved(
		typedPackage,
		branch.Body.List[0],
		administrationObject,
		"gitRow",
		"gitPresent",
		".git",
	)
	objectsRowObject, objectsPresentObject, objectsOK :=
		sourceObjectClaimAdministrativeRowLookupProved(
			typedPackage,
			branch.Body.List[1],
			administrationObject,
			"objectsRow",
			"objectsPresent",
			".git/objects",
		)
	if !gitOK || !objectsOK {
		return false
	}
	invalid, ok := branch.Body.List[2].(*ast.IfStmt)
	if !ok || invalid.Init != nil || invalid.Else != nil || len(invalid.Body.List) != 2 ||
		types.ExprString(invalid.Cond) != "!gitPresent || !objectsPresent || "+
			"gitRow.kind != sourceObservedDirectory || objectsRow.kind != sourceObservedDirectory" ||
		!sourceObjectClaimFailInvariantAndNilFalseProved(
			typedPackage,
			invalid.Body.List,
			receiverObject,
		) {
		return false
	}
	// Bind the lookup objects into the guard through their exact typed uses.
	for object, name := range map[types.Object]string{
		gitPresentObject:     "gitPresent",
		objectsPresentObject: "objectsPresent",
		gitRowObject:         "gitRow",
		objectsRowObject:     "objectsRow",
	} {
		if object == nil || !sourceObjectClaimExpressionUsesObject(info, invalid.Cond, object, name) {
			return false
		}
	}
	return sourceObjectClaimSelectorFieldAssignmentProved(
		info,
		branch.Body.List[3],
		gitEvidenceObject,
		gitRowObject,
		"evidence",
	) && sourceObjectClaimSelectorFieldAssignmentProved(
		info,
		branch.Body.List[4],
		objectsEvidenceObject,
		objectsRowObject,
		"evidence",
	)
}

func sourceObjectClaimAdministrativeRowLookupProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	administrationObject types.Object,
	rowName string,
	presentName string,
	path string,
) (types.Object, types.Object, bool) {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return nil, nil, false
	}
	info := typedPackage.TypesInfo
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 ||
		len(assignment.Rhs) != 1 {
		return nil, nil, false
	}
	row, rowOK := assignment.Lhs[0].(*ast.Ident)
	present, presentOK := assignment.Lhs[1].(*ast.Ident)
	call, callOK := sourceConstructionUnparenthesizedExpression(assignment.Rhs[0]).(*ast.CallExpr)
	if !rowOK || !presentOK || !callOK || row.Name != rowName ||
		present.Name != presentName || len(call.Args) != 2 ||
		!sourceObjectPathPackageFunctionCallProved(
			typedPackage,
			call,
			"sourceAdministrativeRowByPath",
		) || !sourceConstructionSelectorChainProved(
		info,
		call.Args[0],
		administrationObject,
		"rows",
	) || types.ExprString(call.Args[1]) != strconv.Quote(path) {
		return nil, nil, false
	}
	rowObject := info.Defs[row]
	presentObject := info.Defs[present]
	return rowObject, presentObject, rowObject != nil && presentObject != nil
}

func sourceObjectClaimExpressionUsesObject(
	info *types.Info,
	expression ast.Expr,
	object types.Object,
	name string,
) bool {
	uses := 0
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Name == name && info.Uses[identifier] == object {
			uses++
		}
		return true
	})
	return uses == 1
}

func sourceObjectClaimSelectorFieldAssignmentProved(
	info *types.Info,
	statement ast.Stmt,
	targetObject types.Object,
	sourceObject types.Object,
	field string,
) bool {
	assignment, ok := statement.(*ast.AssignStmt)
	return ok && assignment.Tok == token.ASSIGN && len(assignment.Lhs) == 1 &&
		len(assignment.Rhs) == 1 && sourceConstructionDirectObjectExpression(
		info,
		assignment.Lhs[0],
		targetObject,
	) && sourceConstructionSelectorChainProved(
		info,
		assignment.Rhs[0],
		sourceObject,
		field,
	)
}

func sourceObjectClaimNegatedInvariantGuardProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	receiverObject types.Object,
	conditionObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return false
	}
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 2 {
		return false
	}
	negation, ok := sourceConstructionUnparenthesizedExpression(branch.Cond).(*ast.UnaryExpr)
	return ok && negation.Op == token.NOT && sourceConstructionDirectObjectExpression(
		typedPackage.TypesInfo,
		negation.X,
		conditionObject,
	) && sourceObjectClaimFailInvariantAndNilFalseProved(
		typedPackage,
		branch.Body.List,
		receiverObject,
	)
}

func sourceObjectClaimFailInvariantAndNilFalseProved(
	typedPackage *packages.Package,
	statements []ast.Stmt,
	receiverObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || len(statements) != 2 {
		return false
	}
	expression, ok := statements[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := sourceConstructionUnparenthesizedExpression(expression.X).(*ast.CallExpr)
	return ok && len(call.Args) == 0 && sourceObjectPathMethodCallProved(
		typedPackage,
		call,
		receiverObject,
		"(*sourceConstructionBuilder).failInvariant",
	) && sourceObjectClaimNilFalseReturnProved(typedPackage.TypesInfo, statements[1])
}

func sourceObjectClaimInventoryConstructionProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	statement ast.Stmt,
) (types.Object, bool) {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil {
		return nil, false
	}
	info := typedPackage.TypesInfo
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return nil, false
	}
	claims, claimsOK := assignment.Lhs[0].(*ast.Ident)
	address, addressOK := sourceConstructionUnparenthesizedExpression(
		assignment.Rhs[0],
	).(*ast.UnaryExpr)
	if !claimsOK || !addressOK || claims.Name != "claims" || address.Op != token.AND {
		return nil, false
	}
	composite, ok := sourceConstructionUnparenthesizedExpression(address.X).(*ast.CompositeLit)
	if !ok || sourceConstructionTypeName(info.TypeOf(composite)) !=
		"sourceObjectAuxiliaryClaimInventory" || len(composite.Elts) != 1 {
		return nil, false
	}
	rows, ok := composite.Elts[0].(*ast.KeyValueExpr)
	key, keyOK := rows.Key.(*ast.Ident)
	makeRows, makeOK := sourceConstructionUnparenthesizedExpression(
		rows.Value,
	).(*ast.CallExpr)
	pathsObject := sourceObjectPathUniqueDefinedObject(info, function, "paths")
	if !ok || !keyOK || !makeOK || key.Name != "rows" || pathsObject == nil ||
		sourceConstructionCalledObject(info, makeRows.Fun) != types.Universe.Lookup("make") ||
		len(makeRows.Args) != 3 || types.ExprString(makeRows.Args[0]) !=
		"[]sourceObjectAuxiliaryClaim" || !sourceObjectClaimIntegerExpression(
		info,
		makeRows.Args[1],
		0,
	) {
		return nil, false
	}
	capacity, ok := sourceConstructionUnparenthesizedExpression(
		makeRows.Args[2],
	).(*ast.CallExpr)
	if !ok || sourceConstructionCalledObject(info, capacity.Fun) != types.Universe.Lookup("int") ||
		len(capacity.Args) != 1 || !sourceConstructionSelectorChainProved(
		info,
		capacity.Args[0],
		pathsObject,
		"auxiliaryFileCount",
	) {
		return nil, false
	}
	claimsObject := info.Defs[claims]
	return claimsObject, claimsObject != nil
}

func sourceObjectClaimCaptureLoopProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	claimsObject types.Object,
	pathsObject types.Object,
	administrationObject types.Object,
	presenceObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return false
	}
	info := typedPackage.TypesInfo
	loop, ok := statement.(*ast.RangeStmt)
	if !ok || loop.Tok != token.DEFINE || loop.Value != nil || loop.Body == nil ||
		len(loop.Body.List) != 8 || !sourceConstructionSelectorChainProved(
		info,
		loop.X,
		pathsObject,
		"rows",
	) {
		return false
	}
	rowIndex, ok := loop.Key.(*ast.Ident)
	if !ok || rowIndex.Name != "rowIndex" || info.Defs[rowIndex] == nil {
		return false
	}
	claimAssignment, ok := loop.Body.List[4].(*ast.AssignStmt)
	if !ok || claimAssignment.Tok != token.DEFINE || len(claimAssignment.Lhs) != 2 ||
		len(claimAssignment.Rhs) != 1 {
		return false
	}
	claim, claimOK := claimAssignment.Lhs[0].(*ast.Ident)
	call, callOK := sourceConstructionUnparenthesizedExpression(
		claimAssignment.Rhs[0],
	).(*ast.CallExpr)
	if !claimOK || !callOK || claim.Name != "claim" || len(call.Args) != 4 {
		return false
	}
	receiver := functionReceiverObject(typedPackage.TypesInfo, sourceConstructionEnclosingFunction(
		sourceObjectClaimFileForPosition(typedPackage, statement.Pos()),
		statement.Pos(),
	))
	if receiver == nil || !sourceObjectPathMethodCallProved(
		typedPackage,
		call,
		receiver,
		"(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaim",
	) {
		return false
	}
	rowObject := sourceObjectPathUniqueDefinedObject(
		info,
		sourceConstructionEnclosingFunction(
			sourceObjectClaimFileForPosition(typedPackage, statement.Pos()),
			statement.Pos(),
		),
		"row",
	)
	administrativeRowObject := sourceObjectPathUniqueDefinedObject(
		info,
		sourceConstructionEnclosingFunction(
			sourceObjectClaimFileForPosition(typedPackage, statement.Pos()),
			statement.Pos(),
		),
		"administrativeRow",
	)
	if rowObject == nil || administrativeRowObject == nil ||
		!sourceConstructionDirectObjectExpression(info, call.Args[0], rowObject) ||
		!sourceConstructionDirectObjectExpression(info, call.Args[1], administrativeRowObject) ||
		!sourceConstructionDirectObjectExpression(info, call.Args[2], administrationObject) ||
		!sourceConstructionDirectObjectExpression(info, call.Args[3], presenceObject) {
		return false
	}
	appendAssignment, ok := loop.Body.List[7].(*ast.AssignStmt)
	if !ok || appendAssignment.Tok != token.ASSIGN || len(appendAssignment.Lhs) != 1 ||
		len(appendAssignment.Rhs) != 1 || !sourceConstructionSelectorChainProved(
		info,
		appendAssignment.Lhs[0],
		claimsObject,
		"rows",
	) {
		return false
	}
	appendCall, ok := sourceConstructionUnparenthesizedExpression(
		appendAssignment.Rhs[0],
	).(*ast.CallExpr)
	claimObject := info.Defs[claim]
	return ok && claimObject != nil &&
		sourceConstructionCalledObject(info, appendCall.Fun) == types.Universe.Lookup("append") &&
		len(appendCall.Args) == 2 && sourceConstructionSelectorChainProved(
		info,
		appendCall.Args[0],
		claimsObject,
		"rows",
	) && sourceConstructionDirectObjectExpression(info, appendCall.Args[1], claimObject)
}

func sourceObjectClaimInventoryValidationProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	claimsObject types.Object,
	pathsObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return false
	}
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
	call, callOK := sourceConstructionUnparenthesizedExpression(
		assignment.Rhs[0],
	).(*ast.CallExpr)
	if !failureOK || !callOK || failure.Name != "failure" || len(call.Args) != 4 ||
		!sourceObjectPathPackageFunctionCallProved(
			typedPackage,
			call,
			"validateSourceObjectAuxiliaryClaimInventory",
		) {
		return false
	}
	function := sourceConstructionEnclosingFunction(
		sourceObjectClaimFileForPosition(typedPackage, statement.Pos()),
		statement.Pos(),
	)
	receiver := functionReceiverObject(info, function)
	if receiver == nil || !sourceConstructionSelectorChainProved(
		info,
		call.Args[0],
		receiver,
		"ctx",
	) || !sourceConstructionSelectorChainProved(
		info,
		call.Args[1],
		receiver,
		"owner",
		"config",
		"claim",
		"objectFormat",
	) || !sourceConstructionDirectObjectExpression(info, call.Args[2], pathsObject) ||
		!sourceConstructionDirectObjectExpression(info, call.Args[3], claimsObject) {
		return false
	}
	failureObject := info.Defs[failure]
	return failureObject != nil && sourceObjectPathObjectNilComparison(
		info,
		branch.Cond,
		failureObject,
		token.NEQ,
	) && sourceObjectClaimFailureAndNilFalseReturnProved(
		typedPackage,
		branch.Body.List,
		receiver,
		failureObject,
	)
}

func sourceObjectClaimAdministrationMatchProved(
	typedPackage *packages.Package,
	assignmentStatement ast.Stmt,
	failureStatement ast.Stmt,
	matchStatement ast.Stmt,
	claimsObject types.Object,
	administrationObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return false
	}
	info := typedPackage.TypesInfo
	assignment, ok := assignmentStatement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 ||
		len(assignment.Rhs) != 1 {
		return false
	}
	matches, matchesOK := assignment.Lhs[0].(*ast.Ident)
	failure, failureOK := assignment.Lhs[1].(*ast.Ident)
	call, callOK := sourceConstructionUnparenthesizedExpression(
		assignment.Rhs[0],
	).(*ast.CallExpr)
	if !callOK || !sourceObjectPathPackageFunctionCallProved(
		typedPackage,
		call,
		"sourceObjectAuxiliaryClaimsMatchAdministrationValidity",
	) || !matchesOK || !failureOK || matches.Name != "matchesAdministration" ||
		failure.Name != "failure" || len(call.Args) != 4 {
		return false
	}
	function := sourceConstructionEnclosingFunction(
		sourceObjectClaimFileForPosition(typedPackage, assignmentStatement.Pos()),
		assignmentStatement.Pos(),
	)
	receiver := functionReceiverObject(info, function)
	if receiver == nil || !sourceConstructionSelectorChainProved(
		info,
		call.Args[0],
		receiver,
		"ctx",
	) || !sourceObjectClaimConstantExpression(info, call.Args[1], "OperationValidate") ||
		!sourceConstructionDirectObjectExpression(
			info,
			call.Args[2],
			administrationObject,
		) || !sourceConstructionDirectObjectExpression(info, call.Args[3], claimsObject) {
		return false
	}
	matchesObject := info.Defs[matches]
	failureObject := info.Defs[failure]
	if matchesObject == nil || failureObject == nil ||
		!sourceObjectClaimFailureAndNilFalseGuardProved(
			typedPackage,
			failureStatement,
			receiver,
			failureObject,
		) {
		return false
	}
	branch, ok := matchStatement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 2 {
		return false
	}
	negation, ok := sourceConstructionUnparenthesizedExpression(branch.Cond).(*ast.UnaryExpr)
	if !ok || negation.Op != token.NOT || !sourceConstructionDirectObjectExpression(
		info,
		negation.X,
		matchesObject,
	) {
		return false
	}
	expression, ok := branch.Body.List[0].(*ast.ExprStmt)
	failCall, callOK := func() (*ast.CallExpr, bool) {
		if !ok {
			return nil, false
		}
		call, callOK := sourceConstructionUnparenthesizedExpression(expression.X).(*ast.CallExpr)
		return call, callOK
	}()
	return receiver != nil && callOK && len(failCall.Args) == 0 &&
		sourceObjectPathMethodCallProved(
			typedPackage,
			failCall,
			receiver,
			"(*sourceConstructionBuilder).failInvariant",
		) && sourceObjectClaimNilFalseReturnProved(info, branch.Body.List[1])
}

func sourceObjectClaimFailureAndNilFalseGuardProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	receiverObject types.Object,
	failureObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil {
		return false
	}
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Init == nil && branch.Else == nil && len(branch.Body.List) == 2 &&
		sourceObjectPathObjectNilComparison(
			typedPackage.TypesInfo,
			branch.Cond,
			failureObject,
			token.NEQ,
		) && sourceObjectClaimFailureAndNilFalseReturnProved(
		typedPackage,
		branch.Body.List,
		receiverObject,
		failureObject,
	)
}

func sourceObjectClaimFailureAndNilFalseReturnProved(
	typedPackage *packages.Package,
	statements []ast.Stmt,
	receiverObject types.Object,
	failureObject types.Object,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || len(statements) != 2 {
		return false
	}
	expression, ok := statements[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := sourceConstructionUnparenthesizedExpression(expression.X).(*ast.CallExpr)
	return ok && sourceObjectPathMethodCallProved(
		typedPackage,
		call,
		receiverObject,
		"(*sourceUseOutcome).addPrimitive",
		"outcome",
	) && len(call.Args) == 1 && sourceConstructionDirectObjectExpression(
		typedPackage.TypesInfo,
		call.Args[0],
		failureObject,
	) && sourceObjectClaimNilFalseReturnProved(
		typedPackage.TypesInfo,
		statements[1],
	)
}

func sourceObjectClaimNilFalseReturnProved(info *types.Info, statement ast.Stmt) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 {
		return false
	}
	nilIdentifier, ok := sourceConstructionUnparenthesizedExpression(
		returned.Results[0],
	).(*ast.Ident)
	return ok && info.Uses[nilIdentifier] == types.Universe.Lookup("nil") &&
		sourceObjectClaimBooleanExpression(info, returned.Results[1], false)
}

func sourceObjectClaimBooleanExpression(
	info *types.Info,
	expression ast.Expr,
	want bool,
) bool {
	identifier, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.Ident)
	if !ok {
		return false
	}
	object, ok := info.Uses[identifier].(*types.Const)
	return ok && object == types.Universe.Lookup(strconv.FormatBool(want))
}

func sourceObjectClaimIntegerExpression(
	info *types.Info,
	expression ast.Expr,
	want int64,
) bool {
	value := info.Types[sourceConstructionUnparenthesizedExpression(expression)].Value
	if value == nil {
		return false
	}
	observed, exact := constantInt64(value)
	return exact && observed == want
}

func constantInt64(value constant.Value) (int64, bool) {
	return constant.Int64Val(value)
}

func sourceObjectClaimConstantExpression(
	info *types.Info,
	expression ast.Expr,
	name string,
) bool {
	identifier, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.Ident)
	object, constantObject := info.Uses[identifier].(*types.Const)
	return ok && constantObject && object.Name() == name
}

func sourceObjectClaimFileForPosition(
	typedPackage *packages.Package,
	position token.Pos,
) *ast.File {
	if typedPackage == nil {
		return nil
	}
	for _, file := range typedPackage.Syntax {
		if file.Pos() <= position && position <= file.End() {
			return file
		}
	}
	return nil
}

func functionReceiverObject(info *types.Info, function *ast.FuncDecl) types.Object {
	if info == nil || function == nil || function.Recv == nil ||
		len(function.Recv.List) != 1 || len(function.Recv.List[0].Names) != 1 {
		return nil
	}
	return info.Defs[function.Recv.List[0].Names[0]]
}

func sourceObjectClaimConstructionViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for source object claim construction audit"}
	}
	wantComposites := map[string]int{
		sourceObjectClaimBytesSourceFilename +
			"|(*sourceObjectAuxiliaryByteCapture).finish|sourceObjectAuxiliaryClaim{}": 4,
		sourceObjectClaimBytesSourceFilename +
			"|(*sourceObjectAuxiliaryByteCapture).finish|sourceObjectAuxiliaryClaim{path:capture.path,role:capture.role}": 1,
		sourceObjectClaimSourceFilename +
			"|(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaim|sourceObjectAuxiliaryClaim{}": 5,
	}
	wantFieldAssignments := map[string]int{
		sourceObjectClaimBytesSourceFilename +
			"|(*sourceObjectAuxiliaryByteCapture).finish|claim.values=values": 2,
		sourceObjectClaimBytesSourceFilename +
			"|(*sourceObjectAuxiliaryByteCapture).finish|claim.blankRecords=blankRecords": 1,
		sourceObjectClaimBytesSourceFilename +
			"|(*sourceObjectAuxiliaryByteCapture).finish|claim.bytes=bytes": 1,
		sourceObjectClaimSourceFilename +
			"|(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaim|claim,failure=capture.finish(builder.ctx)": 1,
	}
	observedComposites := make(map[string]int)
	observedFieldAssignments := make(map[string]int)
	var observedInventoryConstruction int
	var observedInventoryMake int
	var observedAppend int
	var observedInstall int
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
			sitePrefix := filename + "|" + role + "|"
			switch current := node.(type) {
			case *ast.CompositeLit:
				switch sourceConstructionTypeName(typedPackage.TypesInfo.TypeOf(current)) {
				case "sourceObjectAuxiliaryClaim":
					shape := sourceObjectClaimCompositeShape(typedPackage.TypesInfo, current)
					site := sitePrefix + shape
					observedComposites[site]++
					if shape == "" || wantComposites[site] == 0 {
						violations = append(violations,
							typedPackage.Fset.Position(current.Pos()).String()+
								" source object claim composite is outside audited sites "+site,
						)
					}
				case "sourceObjectAuxiliaryClaimInventory":
					if sourceObjectClaimAllowedInventoryComposite(
						typedPackage,
						function,
						current,
						parents,
					) {
						observedInventoryConstruction++
					} else {
						violations = append(violations,
							typedPackage.Fset.Position(current.Pos()).String()+
								" source object claim inventory composite is outside audited site "+sitePrefix+types.ExprString(current),
						)
					}
				}
			case *ast.AssignStmt:
				allowed := false
				if sourceObjectClaimAllowedInstallAssignment(typedPackage, function, current) {
					observedInstall++
					allowed = true
				}
				if sourceObjectClaimAllowedAppendAssignment(typedPackage, function, current) {
					observedAppend++
					allowed = true
				}
				if sourceObjectPathAllowedObjectsAssignment(typedPackage, function, current) {
					allowed = true
				}
				if site, ok := sourceObjectClaimAllowedFieldAssignment(
					typedPackage,
					filename,
					role,
					current,
				); ok {
					observedFieldAssignments[site]++
					allowed = true
				}
				for _, target := range current.Lhs {
					if kind := sourceObjectClaimMutationTarget(
						typedPackage.TypesInfo,
						target,
					); kind != "" && !allowed {
						violations = append(violations,
							typedPackage.Fset.Position(target.Pos()).String()+
								" source object claim mutation is forbidden "+sitePrefix+kind+"|"+types.ExprString(target),
						)
					}
				}
			case *ast.IncDecStmt:
				if kind := sourceObjectClaimMutationTarget(
					typedPackage.TypesInfo,
					current.X,
				); kind != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source object claim increment is forbidden "+sitePrefix+kind,
					)
				}
			case *ast.RangeStmt:
				for _, target := range []ast.Expr{current.Key, current.Value} {
					if target == nil {
						continue
					}
					if kind := sourceObjectClaimMutationTarget(
						typedPackage.TypesInfo,
						target,
					); kind != "" {
						violations = append(violations,
							typedPackage.Fset.Position(target.Pos()).String()+
								" source object claim range mutation is forbidden "+sitePrefix+kind,
						)
					}
				}
			case *ast.UnaryExpr:
				if current.Op != token.AND || !sourceObjectClaimStorageContains(
					typedPackage.TypesInfo.TypeOf(current.X),
					map[types.Type]bool{},
				) {
					return true
				}
				if sourceObjectClaimAllowedInventoryAddress(
					typedPackage,
					function,
					current,
					parents,
				) || sourceObjectPathAllowedObjectsConstruction(
					typedPackage,
					function,
					current,
				) || sourceObjectPathAllowedEnclosingConstruction(
					typedPackage,
					function,
					current,
				) {
					return true
				}
				violations = append(violations,
					typedPackage.Fset.Position(current.Pos()).String()+
						" source object claim address is forbidden "+sitePrefix+types.ExprString(current),
				)
			case *ast.CallExpr:
				if sourceObjectClaimConversionCarriesStorage(typedPackage.TypesInfo, current) {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source object claim conversion is forbidden "+sitePrefix+sourceConstructionCallFingerprint(current),
					)
					return true
				}
				if sourceObjectClaimGenericCallCarriesStorage(typedPackage, current) {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" generic call carrying source object claim storage is forbidden "+sitePrefix+sourceConstructionCallFingerprint(current),
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
				sensitive := sourceObjectClaimCallCarriesStorage(
					typedPackage.TypesInfo,
					current,
				)
				if !sensitive {
					return true
				}
				switch builtin.Name() {
				case "append":
					if sourceObjectClaimAllowedAppendCall(typedPackage, function, current) {
						return true
					}
				case "make":
					if sourceObjectClaimAllowedInventoryMake(
						typedPackage,
						function,
						current,
						parents,
					) {
						observedInventoryMake++
						return true
					}
				case "len", "cap":
					return true
				case "new":
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source object claim address builtin is forbidden "+sitePrefix+sourceConstructionCallFingerprint(current),
					)
					return true
				}
				violations = append(violations,
					typedPackage.Fset.Position(current.Pos()).String()+
						" source object claim write builtin is outside audited site "+sitePrefix+sourceConstructionCallFingerprint(current),
				)
			}
			return true
		})
	}
	for site, want := range wantComposites {
		if observedComposites[site] != want {
			violations = append(violations,
				"audited source object claim composite count "+site+" = "+
					strconv.Itoa(observedComposites[site])+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range wantFieldAssignments {
		if observedFieldAssignments[site] != want {
			violations = append(violations,
				"audited source object claim field assignment count "+site+" = "+
					strconv.Itoa(observedFieldAssignments[site])+", want "+strconv.Itoa(want),
			)
		}
	}
	for label, observed := range map[string]int{
		"inventory construction": observedInventoryConstruction,
		"inventory make":         observedInventoryMake,
		"row append":             observedAppend,
		"validated install":      observedInstall,
	} {
		if observed != 1 {
			violations = append(violations,
				"audited source object claim "+label+" count = "+
					strconv.Itoa(observed)+", want 1",
			)
		}
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectClaimCompositeShape(info *types.Info, composite *ast.CompositeLit) string {
	if info == nil || composite == nil || sourceConstructionTypeName(info.TypeOf(composite)) !=
		"sourceObjectAuxiliaryClaim" {
		return ""
	}
	if len(composite.Elts) == 0 {
		return "sourceObjectAuxiliaryClaim{}"
	}
	if len(composite.Elts) != 2 {
		return ""
	}
	want := []struct {
		name  string
		value string
	}{
		{name: "path", value: "capture.path"},
		{name: "role", value: "capture.role"},
	}
	for index, expected := range want {
		field, ok := composite.Elts[index].(*ast.KeyValueExpr)
		key, keyOK := field.Key.(*ast.Ident)
		if !ok || !keyOK || key.Name != expected.name ||
			types.ExprString(field.Value) != expected.value {
			return ""
		}
	}
	return "sourceObjectAuxiliaryClaim{path:capture.path,role:capture.role}"
}

func sourceObjectClaimAllowedInventoryComposite(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	composite *ast.CompositeLit,
	parents map[ast.Node]ast.Node,
) bool {
	address, ok := parents[composite].(*ast.UnaryExpr)
	return ok && address.X == composite && sourceObjectClaimAllowedInventoryAddress(
		typedPackage,
		function,
		address,
		parents,
	)
}

func sourceObjectClaimAllowedInventoryAddress(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	address *ast.UnaryExpr,
	parents map[ast.Node]ast.Node,
) bool {
	if function == nil || function.Body == nil || len(function.Body.List) != 20 ||
		address == nil || address.Op != token.AND ||
		!sourceObjectClaimCaptureTransactionProved(typedPackage, function) {
		return false
	}
	assignment, ok := function.Body.List[13].(*ast.AssignStmt)
	return ok && len(assignment.Rhs) == 1 && parents[address] == assignment &&
		sourceConstructionDirectExpression(assignment.Rhs[0], address)
}

func sourceObjectClaimAllowedInventoryMake(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
) bool {
	if function == nil || function.Body == nil || len(function.Body.List) != 20 ||
		!sourceObjectClaimCaptureTransactionProved(typedPackage, function) {
		return false
	}
	assignment, ok := function.Body.List[13].(*ast.AssignStmt)
	if !ok || len(assignment.Rhs) != 1 {
		return false
	}
	address, ok := sourceConstructionUnparenthesizedExpression(assignment.Rhs[0]).(*ast.UnaryExpr)
	if !ok {
		return false
	}
	composite, ok := sourceConstructionUnparenthesizedExpression(address.X).(*ast.CompositeLit)
	if !ok || len(composite.Elts) != 1 {
		return false
	}
	field, ok := composite.Elts[0].(*ast.KeyValueExpr)
	return ok && sourceConstructionDirectExpression(field.Value, call) &&
		parents[call] == field
}

func sourceObjectClaimAllowedInstallAssignment(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	assignment *ast.AssignStmt,
) bool {
	return function != nil && function.Body != nil && len(function.Body.List) == 5 &&
		function.Body.List[3] == assignment &&
		sourceObjectClaimRetainTransactionProved(typedPackage, function)
}

func sourceObjectClaimAllowedAppendAssignment(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	assignment *ast.AssignStmt,
) bool {
	if function == nil || function.Body == nil || len(function.Body.List) != 20 ||
		!sourceObjectClaimCaptureTransactionProved(typedPackage, function) {
		return false
	}
	loop, ok := function.Body.List[14].(*ast.RangeStmt)
	return ok && loop.Body != nil && len(loop.Body.List) == 8 &&
		loop.Body.List[7] == assignment
}

func sourceObjectClaimAllowedAppendCall(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	call *ast.CallExpr,
) bool {
	if function == nil || function.Body == nil || len(function.Body.List) != 20 ||
		!sourceObjectClaimCaptureTransactionProved(typedPackage, function) {
		return false
	}
	loop, ok := function.Body.List[14].(*ast.RangeStmt)
	if !ok || loop.Body == nil || len(loop.Body.List) != 8 {
		return false
	}
	assignment, ok := loop.Body.List[7].(*ast.AssignStmt)
	return ok && len(assignment.Rhs) == 1 &&
		sourceConstructionDirectExpression(assignment.Rhs[0], call)
}

func sourceObjectClaimAllowedFieldAssignment(
	typedPackage *packages.Package,
	filename string,
	role string,
	assignment *ast.AssignStmt,
) (string, bool) {
	if typedPackage == nil || typedPackage.TypesInfo == nil || assignment == nil {
		return "", false
	}
	parts := make([]string, 0, len(assignment.Lhs))
	for _, expression := range assignment.Lhs {
		parts = append(parts, types.ExprString(expression))
	}
	right := make([]string, 0, len(assignment.Rhs))
	for _, expression := range assignment.Rhs {
		right = append(right, types.ExprString(expression))
	}
	site := filename + "|" + role + "|" + strings.Join(parts, ",") + "=" +
		strings.Join(right, ",")
	want := map[string]bool{
		sourceObjectClaimBytesSourceFilename +
			"|(*sourceObjectAuxiliaryByteCapture).finish|claim.values=values": true,
		sourceObjectClaimBytesSourceFilename +
			"|(*sourceObjectAuxiliaryByteCapture).finish|claim.blankRecords=blankRecords": true,
		sourceObjectClaimBytesSourceFilename +
			"|(*sourceObjectAuxiliaryByteCapture).finish|claim.bytes=bytes": true,
		sourceObjectClaimSourceFilename +
			"|(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaim|claim,failure=capture.finish(builder.ctx)": true,
	}
	if !want[site] || assignment.Tok != token.ASSIGN {
		return "", false
	}
	return site, true
}

func sourceObjectClaimMutationTarget(info *types.Info, target ast.Expr) string {
	if info == nil || target == nil {
		return ""
	}
	target = sourceConstructionUnparenthesizedExpression(target)
	if identifier, ok := target.(*ast.Ident); ok && info.Defs[identifier] != nil {
		return ""
	}
	for expression := target; expression != nil; {
		if kind := sourceObjectClaimDirectStorageKind(info.TypeOf(expression)); kind != "" {
			return kind
		}
		switch current := sourceConstructionUnparenthesizedExpression(expression).(type) {
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
			expression = nil
		}
	}
	if sourceObjectClaimStorageContains(info.TypeOf(target), map[types.Type]bool{}) {
		return "sourceObjectAuxiliaryClaimStorage"
	}
	return ""
}

func sourceObjectClaimDirectStorageKind(value types.Type) string {
	switch sourceConstructionTypeName(value) {
	case "sourceObjectAuxiliaryClaim":
		return "sourceObjectAuxiliaryClaim"
	case "sourceObjectAuxiliaryClaimInventory":
		return "sourceObjectAuxiliaryClaimInventory"
	default:
		return ""
	}
}

func sourceObjectClaimStorageContains(value types.Type, seen map[types.Type]bool) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	if sourceObjectClaimDirectStorageKind(value) != "" {
		return true
	}
	switch current := value.(type) {
	case *types.Pointer:
		return sourceObjectClaimStorageContains(current.Elem(), seen)
	case *types.Slice:
		return sourceObjectClaimStorageContains(current.Elem(), seen)
	case *types.Array:
		return sourceObjectClaimStorageContains(current.Elem(), seen)
	case *types.Map:
		return sourceObjectClaimStorageContains(current.Key(), seen) ||
			sourceObjectClaimStorageContains(current.Elem(), seen)
	case *types.Tuple:
		for variable := range current.Variables() {
			if sourceObjectClaimStorageContains(variable.Type(), seen) {
				return true
			}
		}
	case *types.Struct:
		for field := range current.Fields() {
			if sourceObjectClaimStorageContains(field.Type(), seen) {
				return true
			}
		}
	case *types.Named:
		return sourceObjectClaimStorageContains(current.Underlying(), seen)
	}
	return false
}

func sourceObjectClaimCallCarriesStorage(info *types.Info, call *ast.CallExpr) bool {
	if info == nil || call == nil {
		return false
	}
	if sourceObjectClaimStorageContains(info.TypeOf(call), map[types.Type]bool{}) {
		return true
	}
	for _, argument := range call.Args {
		if sourceObjectClaimStorageContains(info.TypeOf(argument), map[types.Type]bool{}) {
			return true
		}
	}
	return false
}

func sourceObjectClaimConversionCarriesStorage(info *types.Info, call *ast.CallExpr) bool {
	if info == nil || call == nil || len(call.Args) != 1 {
		return false
	}
	target, ok := info.Types[sourceConstructionUnparenthesizedExpression(call.Fun)]
	return ok && target.IsType() &&
		(sourceObjectClaimStorageContains(target.Type, map[types.Type]bool{}) ||
			sourceObjectClaimStorageContains(info.TypeOf(call.Args[0]), map[types.Type]bool{}))
}

func sourceObjectClaimGenericCallCarriesStorage(
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
	selector, selectorCall := sourceConstructionUnparenthesizedExpression(call.Fun).(*ast.SelectorExpr)
	generic := signature.TypeParams().Len()+signature.RecvTypeParams().Len() > 0
	if selectorCall && sourceObjectPathTypeHasArguments(typedPackage.TypesInfo.TypeOf(selector.X)) {
		generic = true
	}
	if !generic {
		return false
	}
	values := append([]ast.Expr(nil), call.Args...)
	if selectorCall {
		values = append(values, selector.X)
	}
	for _, value := range values {
		if sourceObjectClaimStorageContains(
			typedPackage.TypesInfo.TypeOf(value),
			map[types.Type]bool{},
		) {
			return true
		}
	}
	return false
}

func sourceObjectClaimEscapeViolations(typedPackage *packages.Package) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for source object claim escape audit"}
	}
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		if !strings.HasPrefix(filename, "source_construction") {
			continue
		}
		for _, specification := range file.Imports {
			importPath, err := strconv.Unquote(specification.Path.Value)
			if err == nil && importPath == "unsafe" {
				violations = append(violations,
					typedPackage.Fset.Position(specification.Pos()).String()+
						" imports unsafe inside the source object claim boundary "+filename,
				)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			violations = append(violations, sourceObjectClaimEscapeNodeViolations(
				typedPackage,
				file,
				node,
			)...)
			return true
		})
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectClaimEscapeNodeViolations(
	typedPackage *packages.Package,
	file *ast.File,
	node ast.Node,
) []string {
	switch current := node.(type) {
	case *ast.AssignStmt:
		return sourceObjectClaimAssignmentEscapeViolations(
			typedPackage,
			current.Lhs,
			current.Rhs,
		)
	case *ast.ValueSpec:
		targets := make([]ast.Expr, len(current.Names))
		for index, name := range current.Names {
			targets[index] = name
		}
		return sourceObjectClaimAssignmentEscapeViolations(
			typedPackage,
			targets,
			current.Values,
		)
	case *ast.CallExpr:
		escaped := sourceObjectClaimCallErasesStorage(typedPackage, current)
		if escaped != "" {
			return []string{typedPackage.Fset.Position(current.Pos()).String() +
				" erases " + escaped + " through call " + sourceConstructionCallFingerprint(current)}
		}
	case *ast.IndexExpr:
		if violation := sourceObjectClaimMapKeyEscapeViolation(typedPackage, current); violation != "" {
			return []string{violation}
		}
	case *ast.ReturnStmt:
		return sourceObjectClaimReturnEscapeViolations(typedPackage, file, current)
	case *ast.SendStmt:
		escaped := sourceObjectClaimExpressionStorageKind(typedPackage.TypesInfo, current.Value)
		if escaped != "" {
			return []string{typedPackage.Fset.Position(current.Pos()).String() +
				" sends " + escaped + " through a channel"}
		}
	}
	return nil
}

func sourceObjectClaimAssignmentEscapeViolations(
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
	violations := make([]string, 0)
	for _, pair := range pairs {
		escaped := sourceObjectClaimStorageKind(pair.value)
		if pair.expression != nil {
			escaped = sourceObjectClaimExpressionStorageKind(
				typedPackage.TypesInfo,
				pair.expression,
			)
		}
		if escaped == "" || sourceObjectClaimStorageContains(
			typedPackage.TypesInfo.TypeOf(pair.target),
			map[types.Type]bool{},
		) {
			continue
		}
		violations = append(violations,
			typedPackage.Fset.Position(pair.target.Pos()).String()+
				" writes "+escaped+" into package or type-erasing storage "+types.ExprString(pair.target),
		)
	}
	return violations
}

func sourceObjectClaimExpressionStorageKind(info *types.Info, expression ast.Expr) string {
	if info == nil || expression == nil {
		return ""
	}
	if escaped := sourceObjectClaimStorageKind(info.TypeOf(expression)); escaped != "" {
		return escaped
	}
	children := make([]ast.Expr, 0)
	switch current := sourceConstructionUnparenthesizedExpression(expression).(type) {
	case *ast.CompositeLit:
		children = append(children, current.Elts...)
	case *ast.KeyValueExpr:
		keyIsField := false
		if identifier, ok := current.Key.(*ast.Ident); ok {
			field, _ := info.Uses[identifier].(*types.Var)
			keyIsField = field != nil && field.IsField()
		}
		if !keyIsField {
			children = append(children, current.Key)
		}
		children = append(children, current.Value)
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
			children = append(children, current.Args...)
		}
	}
	for _, child := range children {
		if escaped := sourceObjectClaimExpressionStorageKind(info, child); escaped != "" {
			return escaped
		}
	}
	return ""
}

func sourceObjectClaimMapKeyEscapeViolation(
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
	if !ok || sourceObjectClaimStorageContains(mapType.Key(), map[types.Type]bool{}) {
		return ""
	}
	escaped := sourceObjectClaimExpressionStorageKind(
		typedPackage.TypesInfo,
		index.Index,
	)
	if escaped == "" {
		return ""
	}
	return typedPackage.Fset.Position(index.Index.Pos()).String() +
		" stores " + escaped + " as a type-erasing map key in " + types.ExprString(index.X)
}

func sourceObjectClaimStorageKind(value types.Type) string {
	if kind := sourceObjectClaimDirectStorageKind(value); kind != "" {
		return kind
	}
	if value == nil {
		return ""
	}
	if tuple, ok := types.Unalias(value).(*types.Tuple); ok {
		for variable := range tuple.Variables() {
			if kind := sourceObjectClaimStorageKind(variable.Type()); kind != "" {
				return kind
			}
		}
		return ""
	}
	if sourceObjectClaimStorageContains(value, map[types.Type]bool{}) {
		return "sourceObjectAuxiliaryClaimStorage"
	}
	return ""
}

func sourceObjectClaimCallErasesStorage(
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
			if escaped = sourceObjectClaimExpressionStorageKind(info, argument); escaped != "" {
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
			if len(call.Args) > 0 && sourceObjectClaimStorageContains(
				info.TypeOf(call.Args[0]),
				map[types.Type]bool{},
			) {
				return ""
			}
		case "copy":
			if len(call.Args) == 2 && sourceObjectClaimStorageContains(
				info.TypeOf(call.Args[0]),
				map[types.Type]bool{},
			) && sourceObjectClaimStorageContains(
				info.TypeOf(call.Args[1]),
				map[types.Type]bool{},
			) {
				return ""
			}
		}
		return escaped
	}
	var signature *types.Signature
	if called != nil {
		signature, _ = called.Type().(*types.Signature)
	}
	if selector, ok := sourceConstructionUnwrapCallFunction(call.Fun).(*ast.SelectorExpr); ok {
		if selection := info.Selections[selector]; selection != nil {
			escaped := sourceObjectClaimExpressionStorageKind(info, selector.X)
			if escaped != "" && (signature == nil || signature.Recv() == nil ||
				!sourceObjectClaimStorageContains(
					signature.Recv().Type(),
					map[types.Type]bool{},
				)) {
				return escaped
			}
		}
	}
	if len(call.Args) == 1 {
		if tuple, ok := info.TypeOf(call.Args[0]).(*types.Tuple); ok {
			for index := range tuple.Len() {
				escaped := sourceObjectClaimStorageKind(tuple.At(index).Type())
				if escaped != "" && !sourceObjectClaimStorageContains(
					sourceObjectPathCallParameter(signature, call, index),
					map[types.Type]bool{},
				) {
					return escaped
				}
			}
			return ""
		}
	}
	for index, argument := range call.Args {
		escaped := sourceObjectClaimExpressionStorageKind(info, argument)
		if escaped != "" && !sourceObjectClaimStorageContains(
			sourceObjectPathCallParameter(signature, call, index),
			map[types.Type]bool{},
		) {
			return escaped
		}
	}
	return ""
}

func sourceObjectClaimReturnEscapeViolations(
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
		escaped := sourceObjectClaimExpressionStorageKind(typedPackage.TypesInfo, result)
		if escaped == "" || sourceObjectClaimReturnPreservesStorage(
			typedPackage.TypesInfo,
			signature,
			statement,
			result,
			index,
		) {
			continue
		}
		violations = append(violations,
			typedPackage.Fset.Position(result.Pos()).String()+
				" returns "+escaped+" through type-erasing result in "+function.Name.Name,
		)
	}
	return violations
}

func sourceObjectClaimReturnPreservesStorage(
	info *types.Info,
	signature *types.Signature,
	statement *ast.ReturnStmt,
	result ast.Expr,
	index int,
) bool {
	if len(statement.Results) == signature.Results().Len() && index < signature.Results().Len() {
		return sourceObjectClaimStorageContains(
			signature.Results().At(index).Type(),
			map[types.Type]bool{},
		)
	}
	if len(statement.Results) != 1 {
		return false
	}
	tuple, ok := info.TypeOf(result).(*types.Tuple)
	if !ok || tuple.Len() != signature.Results().Len() {
		return false
	}
	for resultIndex := range tuple.Len() {
		sourceSensitive := sourceObjectClaimStorageContains(
			tuple.At(resultIndex).Type(),
			map[types.Type]bool{},
		)
		destinationSensitive := sourceObjectClaimStorageContains(
			signature.Results().At(resultIndex).Type(),
			map[types.Type]bool{},
		)
		if sourceSensitive && !destinationSensitive {
			return false
		}
	}
	return true
}

func sourceObjectClaimRevalidationWriteViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for source object claim revalidation audit"}
	}
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		if filepath.Base(typedPackage.Fset.Position(file.Package).Filename) !=
			sourceObjectClaimRevalidationFilename {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				return true
			}
			function := sourceConstructionEnclosingFunction(file, node.Pos())
			receiver := functionReceiverObject(typedPackage.TypesInfo, function)
			if receiver == nil {
				return true
			}
			report := func(expression ast.Expr, action string) {
				kind := sourceObjectClaimRetainedExpressionKind(
					typedPackage.TypesInfo,
					receiver,
					expression,
				)
				if kind == "" {
					return
				}
				violations = append(violations,
					typedPackage.Fset.Position(expression.Pos()).String()+
						" revalidation "+action+" retained "+kind+" "+types.ExprString(expression),
				)
			}
			switch current := node.(type) {
			case *ast.AssignStmt:
				for _, target := range current.Lhs {
					if identifier, ok := sourceConstructionUnparenthesizedExpression(
						target,
					).(*ast.Ident); ok && typedPackage.TypesInfo.Defs[identifier] != nil {
						continue
					}
					report(target, "writes")
				}
			case *ast.IncDecStmt:
				report(current.X, "increments")
			case *ast.RangeStmt:
				for _, target := range []ast.Expr{current.Key, current.Value} {
					if target != nil {
						report(target, "ranges into")
					}
				}
			case *ast.UnaryExpr:
				if current.Op == token.AND {
					report(current.X, "takes address of")
				}
			case *ast.CallExpr:
				builtin, ok := sourceConstructionCalledObject(
					typedPackage.TypesInfo,
					current.Fun,
				).(*types.Builtin)
				if !ok || (builtin.Name() != "append" && builtin.Name() != "copy" &&
					builtin.Name() != "clear" && builtin.Name() != "delete") {
					return true
				}
				for _, argument := range current.Args {
					report(argument, "passes retained storage to "+builtin.Name())
				}
			}
			return true
		})
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectClaimRetainedExpressionKind(
	info *types.Info,
	receiver types.Object,
	expression ast.Expr,
) string {
	for expression != nil {
		expression = sourceConstructionUnparenthesizedExpression(expression)
		for kind, fields := range map[string][]string{
			"administration": {"owner", "git", "administration"},
			"paths":          {"owner", "objects", "paths"},
			"claims":         {"owner", "objects", "claims"},
		} {
			if sourceConstructionSelectorChainProved(info, expression, receiver, fields...) {
				return kind
			}
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

func sourceObjectClaimAdministrationMatchCallerViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for administration-match caller audit"}
	}
	targets := []struct {
		name    string
		callers map[string]int
	}{
		{
			name: "sourceObjectAuxiliaryClaimsMatchAdministrationValidity",
			callers: map[string]int{
				sourceObjectClaimSourceFilename +
					"|(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaimInventory|" +
					"sourceObjectAuxiliaryClaimsMatchAdministrationValidity(" +
					"builder.ctx,OperationValidate,administration,claims)": 1,
				sourceObjectClaimSourceFilename +
					"|sourceObjectAuxiliaryClaimsMatchAdministration|" +
					"sourceObjectAuxiliaryClaimsMatchAdministrationValidity(" +
					"context.Background(),OperationValidate,administration,claims)": 1,
			},
		},
		{
			name: "sourceObjectAuxiliaryClaimsMatchAdministration",
			callers: map[string]int{
				"source_construction.go|(*sourceConstructionOwner).validObjectClaimRetention|" +
					"sourceObjectAuxiliaryClaimsMatchAdministration(" +
					"owner.git.administration,owner.objects.claims)": 1,
			},
		},
	}
	violations := make([]string, 0)
	for _, target := range targets {
		callable, _ := typedPackage.Types.Scope().Lookup(target.name).(*types.Func)
		observed := make(map[string]int)
		for _, file := range typedPackage.Syntax {
			filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
			parents := sourceConstructionParentNodes(file)
			ast.Inspect(file, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if !ok || callable == nil || typedPackage.TypesInfo.Uses[identifier] != callable {
					return true
				}
				call := sourceObjectClaimIdentifierCall(parents, identifier)
				if call == nil {
					violations = append(violations,
						typedPackage.Fset.Position(identifier.Pos()).String()+
							" administration-match helper escapes as a value "+target.name,
					)
					return true
				}
				function := sourceConstructionEnclosingFunction(file, call.Pos())
				role := "<outside-function>"
				if function != nil {
					role = sourceConstructionFunctionRole(
						typedPackage.TypesInfo.Defs[function.Name],
						typedPackage.Types.Path(),
					)
				}
				site := filename + "|" + role + "|" + sourceConstructionCallFingerprint(call)
				observed[site]++
				if target.callers[site] == 0 {
					violations = append(violations,
						typedPackage.Fset.Position(call.Pos()).String()+
							" administration-match caller is outside audited sites "+site,
					)
				}
				return true
			})
		}
		for site, want := range target.callers {
			if observed[site] != want {
				violations = append(violations,
					"audited administration-match caller count "+site+" = "+
						strconv.Itoa(observed[site])+", want "+strconv.Itoa(want),
				)
			}
		}
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectClaimPresenceAndCallerViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for source object claim presence audit"}
	}
	targets := []struct {
		role                          string
		callFingerprint               string
		callArgument                  int
		preReadBranch                 bool
		inventoryRevalidationBranches bool
		callers                       map[string]int
	}{
		{
			role: "(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaimInventory",
			callFingerprint: "builder.captureSourceObjectAuxiliaryClaim(" +
				"row,administrativeRow,administration,presence)",
			callArgument:                  3,
			inventoryRevalidationBranches: true,
			callers: map[string]int{
				sourceObjectClaimSourceFilename +
					"|(*sourceConstructionBuilder).retainSourceObjectAuxiliaryClaimInventory|" +
					"builder.captureSourceObjectAuxiliaryClaimInventory(" +
					"builder.owner.git.administration,builder.owner.objects.paths,sourceInitialRequired)": 1,
				sourceObjectClaimRevalidationFilename +
					"|(*sourceConstructionBuilder).revalidateSourceObjectAuxiliaryClaimInventory|" +
					"builder.captureSourceObjectAuxiliaryClaimInventory(" +
					"administration,paths,sourceRevalidatePresent)": 1,
			},
		},
		{
			role: "(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaim",
			callFingerprint: "builder.primitives.openRelativeNoFollow(" +
				"builder.ctx,parent,components[index],expected.kind,presence)",
			callArgument:  4,
			preReadBranch: true,
			callers: map[string]int{
				sourceObjectClaimSourceFilename +
					"|(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaimInventory|" +
					"builder.captureSourceObjectAuxiliaryClaim(" +
					"row,administrativeRow,administration,presence)": 1,
			},
		},
	}
	violations := make([]string, 0)
	for _, target := range targets {
		function := sourceObjectPathFunctionDeclaration(
			typedPackage,
			sourceObjectClaimSourceFilename,
			target.role,
		)
		if function == nil || !sourceObjectClaimPresencePairProved(
			typedPackage,
			function,
			target.callFingerprint,
			target.callArgument,
			target.preReadBranch,
			target.inventoryRevalidationBranches,
		) {
			position := target.role
			if function != nil {
				position = typedPackage.Fset.Position(function.Pos()).String()
			}
			violations = append(violations,
				position+" source object claim presence pair is outside audited shape",
			)
		}
		callable, _ := func() (*types.Func, bool) {
			if function == nil {
				return nil, false
			}
			callable, ok := typedPackage.TypesInfo.Defs[function.Name].(*types.Func)
			return callable, ok
		}()
		observed := make(map[string]int)
		if callable != nil {
			for _, file := range typedPackage.Syntax {
				filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
				parents := sourceConstructionParentNodes(file)
				ast.Inspect(file, func(node ast.Node) bool {
					selector, ok := node.(*ast.SelectorExpr)
					if !ok || typedPackage.TypesInfo.Uses[selector.Sel] != callable {
						return true
					}
					call := sourceObjectClaimSelectorCall(parents, selector)
					if call == nil {
						violations = append(violations,
							typedPackage.Fset.Position(selector.Pos()).String()+
								" source object claim capture method escapes as a value "+target.role,
						)
						return true
					}
					enclosing := sourceConstructionEnclosingFunction(file, selector.Pos())
					role := "<outside-function>"
					if enclosing != nil {
						role = sourceConstructionFunctionRole(
							typedPackage.TypesInfo.Defs[enclosing.Name],
							typedPackage.Types.Path(),
						)
					}
					site := filename + "|" + role + "|" +
						sourceConstructionCallFingerprint(call)
					observed[site]++
					if target.callers[site] == 0 {
						violations = append(violations,
							typedPackage.Fset.Position(call.Pos()).String()+
								" source object claim capture caller is outside audited sites "+site,
						)
					}
					return true
				})
			}
		}
		for site, want := range target.callers {
			if observed[site] != want {
				violations = append(violations,
					"audited source object claim capture caller count "+site+" = "+
						strconv.Itoa(observed[site])+", want "+strconv.Itoa(want),
				)
			}
		}
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectClaimPresencePairProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	wantCall string,
	wantArgument int,
	wantPreReadBranch bool,
	wantInventoryRevalidationBranches bool,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil {
		return false
	}
	info := typedPackage.TypesInfo
	presence := sourceObjectPathUniqueDefinedObject(info, function, "presence")
	if presence == nil {
		return false
	}
	file := sourceObjectClaimFileForPosition(typedPackage, function.Pos())
	parents := sourceConstructionParentNodes(file)
	comparisons := make(map[string]*ast.BinaryExpr)
	var admittedCallUses int
	var preReadUses int
	var inventoryRevalidationUses int
	var totalUses int
	valid := true
	ast.Inspect(function.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || info.Uses[identifier] != presence {
			return true
		}
		totalUses++
		if comparison, ok := parents[identifier].(*ast.BinaryExpr); ok &&
			comparison.X == identifier {
			constantIdentifier, constantOK := sourceConstructionUnparenthesizedExpression(
				comparison.Y,
			).(*ast.Ident)
			constantObject, constantObjectOK := info.Uses[constantIdentifier].(*types.Const)
			if comparison.Op == token.NEQ && constantOK && constantObjectOK &&
				(constantObject.Name() == "sourceInitialRequired" ||
					constantObject.Name() == "sourceRevalidatePresent") {
				comparisons[constantObject.Name()] = comparison
				return true
			}
			if wantPreReadBranch && comparison.Op == token.EQL && constantOK &&
				constantObjectOK && constantObject.Name() == "sourceRevalidatePresent" &&
				sourceObjectClaimPreReadPresenceBranchProved(
					typedPackage,
					function,
					parents,
					comparison,
				) {
				preReadUses++
				return true
			}
			if wantInventoryRevalidationBranches && comparison.Op == token.EQL && constantOK &&
				constantObjectOK && constantObject.Name() == "sourceRevalidatePresent" &&
				sourceObjectClaimInventoryPresenceBranchProved(
					typedPackage,
					function,
					comparison,
				) {
				inventoryRevalidationUses++
				return true
			}
		}
		call, argument := sourceObjectClaimIdentifierCallArgument(parents, identifier)
		if call != nil && argument == wantArgument &&
			sourceConstructionCallFingerprint(call) == wantCall {
			admittedCallUses++
			return true
		}
		valid = false
		return true
	})
	initial := comparisons["sourceInitialRequired"]
	revalidate := comparisons["sourceRevalidatePresent"]
	wantTotalUses := 3
	wantPreReadUses := 0
	wantInventoryRevalidationUses := 0
	if wantPreReadBranch {
		wantTotalUses++
		wantPreReadUses++
	}
	if wantInventoryRevalidationBranches {
		wantTotalUses += 3
		wantInventoryRevalidationUses = 3
	}
	return valid && totalUses == wantTotalUses && admittedCallUses == 1 &&
		preReadUses == wantPreReadUses &&
		inventoryRevalidationUses == wantInventoryRevalidationUses &&
		len(comparisons) == 2 &&
		initial != nil && revalidate != nil &&
		sourceObjectClaimComparisonsShareAnd(parents, initial, revalidate)
}

func sourceObjectClaimInventoryPresenceBranchProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	comparison *ast.BinaryExpr,
) bool {
	if function == nil || function.Body == nil || len(function.Body.List) != 20 ||
		comparison == nil || !sourceObjectClaimCaptureTransactionProved(typedPackage, function) {
		return false
	}
	for _, index := range [...]int{2, 6, 11} {
		selection, ok := function.Body.List[index].(*ast.IfStmt)
		if ok && sourceConstructionUnparenthesizedExpression(selection.Cond) == comparison {
			return true
		}
	}
	return false
}

func sourceObjectClaimPreReadPresenceBranchProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	parents map[ast.Node]ast.Node,
	presenceComparison *ast.BinaryExpr,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		presenceComparison == nil {
		return false
	}
	and, ok := sourceObjectClaimNonParenParent(parents, presenceComparison).(*ast.BinaryExpr)
	if !ok || and.Op != token.LAND ||
		sourceConstructionUnparenthesizedExpression(and.X) != presenceComparison ||
		types.ExprString(and.Y) != "index == len(components) - 1" {
		return false
	}
	branch, ok := sourceObjectClaimNonParenParent(parents, and).(*ast.IfStmt)
	if !ok || branch.Init != nil || len(branch.Body.List) != 1 ||
		sourceConstructionUnparenthesizedExpression(branch.Cond) != and {
		return false
	}
	alternative, ok := branch.Else.(*ast.BlockStmt)
	thenOK := sourceObjectClaimFailureCallAssignmentProved(
		typedPackage,
		branch.Body.List[0],
		"compareSourceObjectAuxiliaryPreReadEvidence",
		"compareSourceObjectAuxiliaryPreReadEvidence("+
			"builder.ctx,expected.evidence,observation.evidence)",
	)
	elseOK := ok && len(alternative.List) == 1 && sourceObjectClaimFailureCallAssignmentProved(
		typedPackage,
		alternative.List[0],
		"compareSourceDescriptorEvidence",
		"compareSourceDescriptorEvidence(builder.ctx,expected.evidence,observation.evidence)",
	)
	if !ok || len(alternative.List) != 1 || !thenOK || !elseOK {
		return false
	}
	block, ok := parents[branch].(*ast.BlockStmt)
	if !ok || len(block.List) != 1 || block.List[0] != branch {
		return false
	}
	outer, ok := parents[block].(*ast.IfStmt)
	return ok && outer.Init == nil && outer.Else == nil && outer.Body == block &&
		types.ExprString(outer.Cond) == "failure == nil"
}

func sourceObjectClaimFailureCallAssignmentProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	callee string,
	fingerprint string,
) bool {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || types.ExprString(assignment.Lhs[0]) != "failure" {
		return false
	}
	call, ok := sourceConstructionUnparenthesizedExpression(assignment.Rhs[0]).(*ast.CallExpr)
	return ok && sourceObjectPathPackageFunctionCallProved(typedPackage, call, callee) &&
		sourceConstructionCallFingerprint(call) == fingerprint
}

func sourceObjectClaimComparisonsShareAnd(
	parents map[ast.Node]ast.Node,
	left *ast.BinaryExpr,
	right *ast.BinaryExpr,
) bool {
	leftParent := sourceObjectClaimNonParenParent(parents, left)
	rightParent := sourceObjectClaimNonParenParent(parents, right)
	and, ok := leftParent.(*ast.BinaryExpr)
	return ok && leftParent == rightParent && and.Op == token.LAND &&
		((sourceConstructionUnparenthesizedExpression(and.X) == left &&
			sourceConstructionUnparenthesizedExpression(and.Y) == right) ||
			(sourceConstructionUnparenthesizedExpression(and.X) == right &&
				sourceConstructionUnparenthesizedExpression(and.Y) == left))
}

func sourceObjectClaimNonParenParent(
	parents map[ast.Node]ast.Node,
	node ast.Node,
) ast.Node {
	parent := parents[node]
	for {
		paren, ok := parent.(*ast.ParenExpr)
		if !ok {
			return parent
		}
		parent = parents[paren]
	}
}

func sourceObjectClaimIdentifierCallArgument(
	parents map[ast.Node]ast.Node,
	identifier *ast.Ident,
) (*ast.CallExpr, int) {
	node := ast.Node(identifier)
	for {
		parent := parents[node]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			continue
		}
		call, ok := parent.(*ast.CallExpr)
		if !ok {
			return nil, -1
		}
		for index, argument := range call.Args {
			if sourceConstructionDirectExpression(argument, node.(ast.Expr)) {
				return call, index
			}
		}
		return nil, -1
	}
}

func sourceObjectClaimSelectorCall(
	parents map[ast.Node]ast.Node,
	selector *ast.SelectorExpr,
) *ast.CallExpr {
	node := ast.Node(selector)
	for {
		parent := parents[node]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			continue
		}
		call, ok := parent.(*ast.CallExpr)
		if !ok || !sourceConstructionDirectExpression(
			call.Fun,
			node.(ast.Expr),
		) {
			return nil
		}
		return call
	}
}

func sourceObjectClaimRevalidationProtocolViolations(
	typedPackage *packages.Package,
) []string {
	function := sourceObjectPathFunctionDeclaration(
		typedPackage,
		sourceObjectClaimRevalidationFilename,
		"(*sourceConstructionBuilder).revalidateSourceObjectAuxiliaryClaimInventory",
	)
	if sourceObjectClaimRevalidationProtocolProved(typedPackage, function) {
		return nil
	}
	position := "revalidateSourceObjectAuxiliaryClaimInventory"
	if typedPackage != nil && typedPackage.Fset != nil && function != nil {
		position = typedPackage.Fset.Position(function.Pos()).String()
	}
	return []string{position + " source object claim revalidation protocol is outside audited shape"}
}

func sourceObjectClaimRevalidationProtocolProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 22 {
		return false
	}
	statements := function.Body.List
	return sourceObjectClaimExactFalseBranchProved(statements[0], "builder == nil") &&
		sourceObjectClaimExactFailureCallGuardProved(
			statements[1],
			token.DEFINE,
			"sourceContextPrimitiveFailure(builder.ctx,OperationCompare)",
		) &&
		sourceObjectClaimExactInvariantFalseBranchProved(
			statements[2],
			"builder.owner == nil || builder.primitives == nil",
		) &&
		sourceObjectClaimExactCallAssignmentProved(
			statements[3],
			token.DEFINE,
			[]string{"ownerValid"},
			"builder.owner.validObjectClaimRetention()",
		) &&
		sourceObjectClaimExactFailureCallGuardProved(
			statements[4],
			token.DEFINE,
			"sourceContextPrimitiveFailure(builder.ctx,OperationCompare)",
		) &&
		sourceObjectClaimExactInvariantFalseBranchProved(statements[5], "!ownerValid") &&
		sourceObjectClaimExactFailureCallGuardProved(
			statements[6],
			token.DEFINE,
			"builder.revalidateSourceEnvelopePathBindings(sourceEnvelopePreReadBinding)",
		) &&
		sourceObjectClaimExactFalseBranchProved(statements[7], "!builder.outcome.proved()") &&
		sourceObjectClaimExactCallAssignmentProved(
			statements[8],
			token.DEFINE,
			[]string{"administration", "captured"},
			"builder.captureRevalidatedSourceAdministrativeInventory()",
		) &&
		sourceObjectClaimExactFalseBranchProved(statements[9], "!captured") &&
		sourceObjectClaimExactCallAssignmentProved(
			statements[10],
			token.DEFINE,
			[]string{"paths", "failure"},
			"deriveSourceObjectPathInventory("+
				"builder.ctx,builder.owner.config.claim.objectFormat,administration)",
		) &&
		sourceObjectClaimExactConditionalFailureAssignmentProved(
			statements[11],
			"compareRevalidatedSourceObjectPathInventories("+
				"builder.ctx,builder.owner.objects.paths,paths)",
		) &&
		sourceObjectClaimExactStoredFailureGuardProved(statements[12]) &&
		sourceObjectClaimExactCallAssignmentProved(
			statements[13],
			token.DEFINE,
			[]string{"claims", "captured"},
			"builder.captureSourceObjectAuxiliaryClaimInventory("+
				"administration,paths,sourceRevalidatePresent)",
		) &&
		sourceObjectClaimExactFalseBranchProved(statements[14], "!captured") &&
		sourceObjectClaimExactFailureCallGuardProved(
			statements[15],
			token.ASSIGN,
			"compareRevalidatedSourceObjectAuxiliaryClaimInventories("+
				"builder.ctx,builder.owner.config.claim.objectFormat,paths,"+
				"builder.owner.objects.claims,claims)",
		) &&
		sourceObjectClaimExactCallAssignmentProved(
			statements[16],
			token.DEFINE,
			[]string{"postAdministration", "captured"},
			"builder.captureRevalidatedSourceAdministrativeInventory()",
		) &&
		sourceObjectClaimExactFalseBranchProved(statements[17], "!captured") &&
		sourceObjectClaimExactFailureCallGuardProved(
			statements[18],
			token.ASSIGN,
			"builder.revalidateSourceEnvelopePathBindings(sourceEnvelopePostReadBinding)",
		) &&
		sourceObjectClaimExactFalseBranchProved(statements[19], "!builder.outcome.proved()") &&
		sourceObjectClaimExactFailureCallGuardProved(
			statements[20],
			token.ASSIGN,
			"compareRevalidatedDeferredSourceDescriptorEvidence("+
				"builder.ctx,builder.owner.config.claim.objectFormat,"+
				"builder.owner.git.administration,postAdministration)",
		) &&
		sourceObjectClaimExactProvedReturn(statements[21])
}

func sourceObjectClaimExactCallAssignmentProved(
	statement ast.Stmt,
	wantToken token.Token,
	wantLeft []string,
	wantCall string,
) bool {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != wantToken || len(assignment.Lhs) != len(wantLeft) ||
		len(assignment.Rhs) != 1 {
		return false
	}
	for index, expression := range assignment.Lhs {
		if types.ExprString(expression) != wantLeft[index] {
			return false
		}
	}
	call, ok := sourceConstructionUnparenthesizedExpression(assignment.Rhs[0]).(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(call) == wantCall
}

func sourceObjectClaimExactFailureCallGuardProved(
	statement ast.Stmt,
	wantToken token.Token,
	wantCall string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || types.ExprString(branch.Cond) != "failure != nil" ||
		len(branch.Body.List) != 2 || !sourceObjectClaimExactCallAssignmentProved(
		branch.Init,
		wantToken,
		[]string{"failure"},
		wantCall,
	) {
		return false
	}
	return sourceConstructionDirectCallStatementFingerprint(
		branch.Body.List[0],
		"builder.outcome.addPrimitive(failure)",
	) && sourceConstructionSingleBooleanReturnProved(branch.Body.List[1], "false")
}

func sourceObjectClaimExactStoredFailureGuardProved(statement ast.Stmt) bool {
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Init == nil && branch.Else == nil &&
		types.ExprString(branch.Cond) == "failure != nil" && len(branch.Body.List) == 2 &&
		sourceConstructionDirectCallStatementFingerprint(
			branch.Body.List[0],
			"builder.outcome.addPrimitive(failure)",
		) && sourceConstructionSingleBooleanReturnProved(branch.Body.List[1], "false")
}

func sourceObjectClaimExactConditionalFailureAssignmentProved(
	statement ast.Stmt,
	wantCall string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Init == nil && branch.Else == nil &&
		types.ExprString(branch.Cond) == "failure == nil" && len(branch.Body.List) == 1 &&
		sourceObjectClaimExactCallAssignmentProved(
			branch.Body.List[0],
			token.ASSIGN,
			[]string{"failure"},
			wantCall,
		)
}

func sourceObjectClaimExactFalseBranchProved(statement ast.Stmt, condition string) bool {
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Init == nil && branch.Else == nil &&
		types.ExprString(branch.Cond) == condition && len(branch.Body.List) == 1 &&
		sourceConstructionSingleBooleanReturnProved(branch.Body.List[0], "false")
}

func sourceObjectClaimExactInvariantFalseBranchProved(
	statement ast.Stmt,
	condition string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Init == nil && branch.Else == nil &&
		types.ExprString(branch.Cond) == condition && len(branch.Body.List) == 2 &&
		sourceConstructionDirectCallStatementFingerprint(
			branch.Body.List[0],
			"builder.failInvariant()",
		) && sourceConstructionSingleBooleanReturnProved(branch.Body.List[1], "false")
}

func sourceObjectClaimExactProvedReturn(statement ast.Stmt) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	call, ok := sourceConstructionUnparenthesizedExpression(returned.Results[0]).(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(call) == "builder.outcome.proved()"
}

func sourceObjectClaimCriticalCallerViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for critical claim caller audit"}
	}
	site := func(filename string, role string, call string) string {
		return filename + "|" + role + "|" + call
	}
	type target struct {
		filename string
		role     string
		callers  map[string]int
	}
	targets := []target{
		{
			filename: sourceObjectClaimBytesSourceFilename,
			role:     "sourceObjectAuxiliarySizeFailure",
			callers: map[string]int{site(
				sourceObjectClaimSourceFilename,
				"(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaim",
				"sourceObjectAuxiliarySizeFailure(builder.ctx,format,"+
					"administrativeRow.evidence.snapshot.size,chainControl)",
			): 1},
		},
		{
			filename: sourceObjectClaimSourceFilename,
			role:     "sourceObjectAuxiliaryImmediateBindingFailure",
			callers: map[string]int{site(
				sourceObjectClaimSourceFilename,
				"(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaim",
				"sourceObjectAuxiliaryImmediateBindingFailure(builder.ctx,format,row,claim)",
			): 1},
		},
		{
			filename: sourceObjectClaimRevalidationFilename,
			role:     "compareSourceObjectAuxiliaryPreReadEvidence",
			callers: map[string]int{
				site(
					sourceObjectClaimSourceFilename,
					"(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaim",
					"compareSourceObjectAuxiliaryPreReadEvidence("+
						"builder.ctx,expected.evidence,observation.evidence)",
				): 1,
				site(
					sourceObjectClaimRevalidationFilename,
					"compareRevalidatedSourceAdministrativeEvidence",
					"compareSourceObjectAuxiliaryPreReadEvidence(ctx,before,after)",
				): 1,
			},
		},
		{
			filename: sourceObjectClaimRevalidationFilename,
			role:     "compareSourceDirectoryPreWalkEvidence",
			callers: map[string]int{
				site(sourceObjectClaimRevalidationFilename,
					"(*sourceConstructionBuilder).revalidateSourceEnvelopePathBindings",
					"compareSourceDirectoryPreWalkEvidence("+
						"builder.ctx,builder.owner.git.root.evidence,gitEvidence)"): 1,
				site(sourceObjectClaimRevalidationFilename,
					"(*sourceConstructionBuilder).revalidateSourceEnvelopePathBindings",
					"compareSourceDirectoryPreWalkEvidence("+
						"builder.ctx,builder.owner.objects.root.evidence,objectsEvidence)"): 1,
				site(sourceObjectClaimRevalidationFilename,
					"(*sourceConstructionBuilder).compareSourceDirectoryBeforeWalk",
					"compareSourceDirectoryPreWalkEvidence("+
						"builder.ctx,expected,observation.evidence)"): 1,
				site(sourceObjectClaimRevalidationFilename,
					"compareRevalidatedSourceAdministrativeEvidence",
					"compareSourceDirectoryPreWalkEvidence(ctx,before,after)"): 1,
				site(sourceObjectClaimRevalidationFilename,
					"(*sourceConstructionBuilder).captureRevalidatedSourceAdministrativeInventory",
					"compareSourceDirectoryPreWalkEvidence("+
						"builder.ctx,sealedRoot.evidence,rootObservation.evidence)"): 1,
			},
		},
		{
			filename: sourceObjectClaimRevalidationFilename,
			role:     "compareSourceInertRegularEvidence",
			callers: map[string]int{site(
				sourceObjectClaimRevalidationFilename,
				"compareRevalidatedSourceAdministrativeEvidence",
				"compareSourceInertRegularEvidence(ctx,before,after)",
			): 1},
		},
		{
			filename: sourceObjectClaimRevalidationFilename,
			role:     "(*sourceConstructionBuilder).compareSourceDirectoryBeforeWalk",
			callers: map[string]int{
				site(sourceObjectClaimSourceFilename,
					"(*sourceConstructionBuilder).captureSourceObjectAuxiliaryClaimInventory",
					"builder.compareSourceDirectoryBeforeWalk("+
						"builder.owner.objects.root.descriptor,builder.owner.objects.root.evidence)"): 1,
				site(sourceObjectClaimRevalidationFilename,
					"(*sourceConstructionBuilder).captureRevalidatedSourceAdministrativeInventory",
					"builder.compareSourceDirectoryBeforeWalk("+
						"builder.owner.objects.root.descriptor,builder.owner.objects.root.evidence)"): 1,
			},
		},
		{
			filename: sourceObjectClaimRevalidationFilename,
			role:     "compareRevalidatedSourceAdministrativeEvidence",
			callers: map[string]int{
				site("source_construction_inventory.go",
					"(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry",
					"compareRevalidatedSourceAdministrativeEvidence(builder.ctx,"+
						"builder.owner.config.claim.objectFormat,sealedRow.evidence,"+
						"observation.evidence,sealedRow)"): 1,
				site(sourceObjectClaimRevalidationFilename,
					"compareRevalidatedSourceAdministrativeRows",
					"compareRevalidatedSourceAdministrativeEvidence("+
						"ctx,format,before.evidence,after.evidence,before)"): 1,
			},
		},
		{
			filename: sourceObjectClaimRevalidationFilename,
			role:     "compareRevalidatedDeferredSourceDescriptorEvidence",
			callers: map[string]int{site(
				sourceObjectClaimRevalidationFilename,
				"(*sourceConstructionBuilder).revalidateSourceObjectAuxiliaryClaimInventory",
				"compareRevalidatedDeferredSourceDescriptorEvidence(builder.ctx,"+
					"builder.owner.config.claim.objectFormat,"+
					"builder.owner.git.administration,postAdministration)",
			): 1},
		},
		{
			filename: sourceObjectClaimRevalidationFilename,
			role:     "(*sourceConstructionBuilder).revalidateSourceEnvelopePathBindings",
			callers: map[string]int{
				site(
					sourceObjectClaimRevalidationFilename,
					"(*sourceConstructionBuilder).revalidateSourceObjectAuxiliaryClaimInventory",
					"builder.revalidateSourceEnvelopePathBindings(sourceEnvelopePreReadBinding)",
				): 1,
				site(
					sourceObjectClaimRevalidationFilename,
					"(*sourceConstructionBuilder).revalidateSourceObjectAuxiliaryClaimInventory",
					"builder.revalidateSourceEnvelopePathBindings(sourceEnvelopePostReadBinding)",
				): 1,
			},
		},
		{
			filename: sourceObjectClaimRevalidationFilename,
			role:     "compareRevalidatedSourceAuthorityPathClaim",
			callers: map[string]int{
				site(sourceObjectClaimRevalidationFilename,
					"(*sourceConstructionBuilder).revalidateSourceEnvelopePathBindings",
					"compareRevalidatedSourceAuthorityPathClaim(builder.ctx,"+
						"builder.owner.repository.pathClaims[0],observation.evidence.pathClaim())"): 1,
				site(sourceObjectClaimRevalidationFilename,
					"(*sourceConstructionBuilder).revalidateSourceEnvelopePathBindings",
					"compareRevalidatedSourceAuthorityPathClaim(builder.ctx,"+
						"builder.owner.repository.pathClaims[index + 1],"+
						"observation.evidence.pathClaim())"): 1,
			},
		},
	}
	violations := make([]string, 0)
	for _, target := range targets {
		declaration := sourceObjectPathFunctionDeclaration(
			typedPackage,
			target.filename,
			target.role,
		)
		var callable *types.Func
		if declaration != nil {
			callable, _ = typedPackage.TypesInfo.Defs[declaration.Name].(*types.Func)
		}
		observed := make(map[string]int)
		for _, file := range typedPackage.Syntax {
			filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
			parents := sourceConstructionParentNodes(file)
			ast.Inspect(file, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if !ok || callable == nil || typedPackage.TypesInfo.Uses[identifier] != callable {
					return true
				}
				var call *ast.CallExpr
				if selector, selectorOK := parents[identifier].(*ast.SelectorExpr); selectorOK && selector.Sel == identifier {
					call = sourceObjectClaimSelectorCall(parents, selector)
				} else {
					call = sourceObjectClaimIdentifierCall(parents, identifier)
				}
				if call == nil {
					violations = append(violations,
						typedPackage.Fset.Position(identifier.Pos()).String()+
							" critical claim helper escapes as a value "+target.role,
					)
					return true
				}
				function := sourceConstructionEnclosingFunction(file, call.Pos())
				callerRole := "<outside-function>"
				if function != nil {
					callerRole = sourceConstructionFunctionRole(
						typedPackage.TypesInfo.Defs[function.Name],
						typedPackage.Types.Path(),
					)
				}
				callSite := site(filename, callerRole, sourceConstructionCallFingerprint(call))
				observed[callSite]++
				if target.callers[callSite] == 0 {
					violations = append(violations,
						typedPackage.Fset.Position(call.Pos()).String()+
							" critical claim caller is outside audited sites "+callSite,
					)
				}
				return true
			})
		}
		for callSite, want := range target.callers {
			if observed[callSite] != want {
				violations = append(violations,
					"audited critical claim caller count "+callSite+" = "+
						strconv.Itoa(observed[callSite])+", want "+strconv.Itoa(want),
				)
			}
		}
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectClaimFullPrefixViolations(typedPackage *packages.Package) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for full-prefix claim audit"}
	}
	orchestrator := sourceObjectPathFunctionDeclaration(
		typedPackage,
		"source_construction_acquire.go",
		"retainSourceConstructionWith",
	)
	validator := sourceObjectPathFunctionDeclaration(
		typedPackage,
		sourceObjectClaimRevalidationFilename,
		"validateSourceObjectClaimOwner",
	)
	violations := make([]string, 0)
	fullPrefixProved := sourceObjectClaimFullPrefixProved(typedPackage, orchestrator)
	finalValidatorProved := sourceObjectClaimFinalOwnerValidatorProved(typedPackage, validator)
	publicEntryProved := sourceObjectClaimPublicEntryProved(typedPackage)
	if !fullPrefixProved || !finalValidatorProved || !publicEntryProved {
		position := "retainSourceConstructionWith"
		if orchestrator != nil {
			position = typedPackage.Fset.Position(orchestrator.Pos()).String()
		}
		missing := make([]string, 0, 3)
		if !fullPrefixProved {
			missing = append(missing, "transaction")
		}
		if !finalValidatorProved {
			missing = append(missing, "final-validator")
		}
		if !publicEntryProved {
			missing = append(missing, "public-entry")
		}
		violations = append(violations,
			position+" source object claim full-prefix orchestrator is outside audited shape: "+
				strings.Join(missing, ","),
		)
	}
	if orchestrator == nil {
		return violations
	}
	callable, _ := typedPackage.TypesInfo.Defs[orchestrator.Name].(*types.Func)
	wantSite := "source_construction_acquire.go|retainSourceConstruction|" +
		"retainSourceConstructionWith(ctx,locator,primitives)"
	observed := make(map[string]int)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		parents := sourceConstructionParentNodes(file)
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok || callable == nil || typedPackage.TypesInfo.Uses[identifier] != callable {
				return true
			}
			call := sourceObjectClaimIdentifierCall(parents, identifier)
			if call == nil {
				violations = append(violations,
					typedPackage.Fset.Position(identifier.Pos()).String()+
						" initial construction orchestrator escapes as a value",
				)
				return true
			}
			function := sourceConstructionEnclosingFunction(file, call.Pos())
			role := "<outside-function>"
			if function != nil {
				role = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			site := filename + "|" + role + "|" + sourceConstructionCallFingerprint(call)
			observed[site]++
			if site != wantSite {
				violations = append(violations,
					typedPackage.Fset.Position(call.Pos()).String()+
						" initial construction orchestrator caller is outside audited sites "+site,
				)
			}
			return true
		})
	}
	if observed[wantSite] != 1 {
		violations = append(violations,
			"audited initial construction orchestrator caller count "+wantSite+" = "+
				strconv.Itoa(observed[wantSite])+", want 1",
		)
	}
	sort.Strings(violations)
	return violations
}

func sourceObjectClaimPublicEntryProved(typedPackage *packages.Package) bool {
	function := sourceObjectPathFunctionDeclaration(
		typedPackage,
		"source_construction_acquire.go",
		"retainSourceConstruction",
	)
	if function == nil || function.Body == nil || len(function.Body.List) != 3 {
		return false
	}
	returned, ok := function.Body.List[2].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	call, ok := sourceConstructionUnparenthesizedExpression(returned.Results[0]).(*ast.CallExpr)
	return ok && sourceObjectPathPackageFunctionCallProved(
		typedPackage,
		call,
		"retainSourceConstructionWith",
	) && sourceConstructionCallFingerprint(call) ==
		"retainSourceConstructionWith(ctx,locator,primitives)"
}

func sourceObjectClaimFullPrefixProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 5 {
		return false
	}
	info := typedPackage.TypesInfo
	ctxObject := sourceObjectPathUniqueDefinedObject(info, function, "ctx")
	locatorObject := sourceObjectPathUniqueDefinedObject(info, function, "locator")
	primitivesObject := sourceObjectPathUniqueDefinedObject(info, function, "primitives")
	if ctxObject == nil || locatorObject == nil || primitivesObject == nil {
		return false
	}
	builderObject, ok := sourceObjectClaimFullPrefixBuilderProved(
		typedPackage,
		function.Body.List[0],
		ctxObject,
		primitivesObject,
	)
	stagesProved := ok && sourceObjectClaimFullPrefixStagesProved(
		typedPackage,
		function.Body.List[1],
		builderObject,
		ctxObject,
		locatorObject,
		primitivesObject,
	)
	validationProved := ok && sourceObjectClaimFinalValidationCallProved(
		typedPackage,
		function.Body.List[2],
		builderObject,
	)
	closeProved := ok && sourceObjectClaimFullPrefixCloseProved(
		typedPackage,
		function.Body.List[3],
		builderObject,
		primitivesObject,
	)
	if !ok || !stagesProved || !validationProved || !closeProved {
		return false
	}
	returned, ok := function.Body.List[4].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 2 && sourceConstructionSelectorChainProved(
		info,
		returned.Results[0],
		builderObject,
		"owner",
	) && sourceConstructionSelectorChainProved(
		info,
		returned.Results[1],
		builderObject,
		"outcome",
	)
}

func sourceObjectClaimFullPrefixBuilderProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	ctxObject types.Object,
	primitivesObject types.Object,
) (types.Object, bool) {
	info := typedPackage.TypesInfo
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return nil, false
	}
	builder, builderOK := assignment.Lhs[0].(*ast.Ident)
	address, addressOK := sourceConstructionUnparenthesizedExpression(
		assignment.Rhs[0],
	).(*ast.UnaryExpr)
	if !builderOK || !addressOK || builder.Name != "builder" || address.Op != token.AND {
		return nil, false
	}
	composite, ok := sourceConstructionUnparenthesizedExpression(address.X).(*ast.CompositeLit)
	if !ok || sourceConstructionTypeName(info.TypeOf(composite)) !=
		"sourceConstructionBuilder" || len(composite.Elts) != 3 {
		return nil, false
	}
	want := map[string]types.Object{
		"ctx":        ctxObject,
		"primitives": primitivesObject,
	}
	seen := make(map[string]bool)
	for _, element := range composite.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		key, keyOK := field.Key.(*ast.Ident)
		if !ok || !keyOK || seen[key.Name] {
			return nil, false
		}
		seen[key.Name] = true
		if key.Name == "owner" {
			call, callOK := sourceConstructionUnparenthesizedExpression(field.Value).(*ast.CallExpr)
			if !callOK || len(call.Args) != 0 || !sourceObjectPathPackageFunctionCallProved(
				typedPackage,
				call,
				"newSourceConstructionOwner",
			) {
				return nil, false
			}
			continue
		}
		object := want[key.Name]
		if object == nil || !sourceConstructionDirectObjectExpression(info, field.Value, object) {
			return nil, false
		}
	}
	builderObject := info.Defs[builder]
	return builderObject, builderObject != nil && len(seen) == 3
}

func sourceObjectClaimFullPrefixStagesProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	builderObject types.Object,
	ctxObject types.Object,
	locatorObject types.Object,
	primitivesObject types.Object,
) bool {
	info := typedPackage.TypesInfo
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || len(branch.Body.List) != 1 ||
		types.ExprString(branch.Cond) != "ctx == nil || primitives == nil || !locator.valid()" {
		return false
	}
	add, ok := branch.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	addCall, ok := sourceConstructionUnparenthesizedExpression(add.X).(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(addCall) !=
		"builder.outcome.addPrimitive(newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant))" {
		return false
	}
	otherwise, ok := branch.Else.(*ast.BlockStmt)
	if !ok || len(otherwise.List) != 2 {
		return false
	}
	initial, ok := otherwise.List[0].(*ast.ExprStmt)
	initialCall, callOK := func() (*ast.CallExpr, bool) {
		if !ok {
			return nil, false
		}
		call, callOK := sourceConstructionUnparenthesizedExpression(initial.X).(*ast.CallExpr)
		return call, callOK
	}()
	if !callOK || len(initialCall.Args) != 1 || !sourceObjectPathMethodCallProved(
		typedPackage,
		initialCall,
		builderObject,
		"(*sourceConstructionBuilder).retainInitialSource",
	) || !sourceConstructionDirectObjectExpression(info, initialCall.Args[0], locatorObject) {
		return false
	}
	stageBranch, ok := otherwise.List[1].(*ast.IfStmt)
	if !ok || stageBranch.Init != nil || stageBranch.Else != nil || len(stageBranch.Body.List) != 2 ||
		types.ExprString(stageBranch.Cond) != "builder.outcome.proved()" {
		return false
	}
	assignment, ok := stageBranch.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return false
	}
	completed, completedOK := assignment.Lhs[0].(*ast.Ident)
	if !completedOK || completed.Name != "completed" {
		return false
	}
	stageExpressions := sourceObjectClaimFlattenAnd(assignment.Rhs[0])
	wantRoles := []string{
		"(*sourceConstructionBuilder).retainPackedRefs",
		"(*sourceConstructionBuilder).retainSourceAdministrativeInventory",
		"(*sourceConstructionBuilder).retainSourceObjectPathInventory",
		"(*sourceConstructionBuilder).retainSourceObjectAuxiliaryClaimInventory",
		"(*sourceConstructionBuilder).revalidateSourceObjectAuxiliaryClaimInventory",
	}
	if len(stageExpressions) != len(wantRoles) {
		return false
	}
	for index, expression := range stageExpressions {
		call, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.CallExpr)
		if !ok || len(call.Args) != 0 || !sourceObjectPathMethodCallProved(
			typedPackage,
			call,
			builderObject,
			wantRoles[index],
		) {
			return false
		}
	}
	completedObject := info.Defs[completed]
	guard, ok := stageBranch.Body.List[1].(*ast.IfStmt)
	if !ok || completedObject == nil || guard.Init != nil || guard.Else != nil ||
		len(guard.Body.List) != 1 || types.ExprString(guard.Cond) !=
		"!completed && builder.outcome.proved()" {
		return false
	}
	fail, ok := guard.Body.List[0].(*ast.ExprStmt)
	failCall, callOK := func() (*ast.CallExpr, bool) {
		if !ok {
			return nil, false
		}
		call, callOK := sourceConstructionUnparenthesizedExpression(fail.X).(*ast.CallExpr)
		return call, callOK
	}()
	return callOK && len(failCall.Args) == 0 && sourceObjectPathMethodCallProved(
		typedPackage,
		failCall,
		builderObject,
		"(*sourceConstructionBuilder).failInvariant",
	) && ctxObject != nil && primitivesObject != nil
}

func sourceObjectClaimFlattenAnd(expression ast.Expr) []ast.Expr {
	binary, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.LAND {
		return []ast.Expr{expression}
	}
	left := sourceObjectClaimFlattenAnd(binary.X)
	return append(left, sourceObjectClaimFlattenAnd(binary.Y)...)
}

func sourceObjectClaimFinalValidationCallProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	builderObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 ||
		types.ExprString(branch.Cond) != "builder.outcome.proved()" {
		return false
	}
	expression, ok := branch.Body.List[0].(*ast.ExprStmt)
	call, callOK := func() (*ast.CallExpr, bool) {
		if !ok {
			return nil, false
		}
		call, callOK := sourceConstructionUnparenthesizedExpression(expression.X).(*ast.CallExpr)
		return call, callOK
	}()
	return callOK && sourceObjectPathMethodCallProved(
		typedPackage,
		call,
		builderObject,
		"(*sourceUseOutcome).addPrimitive",
		"outcome",
	) && len(call.Args) == 1 && sourceConstructionCallFingerprint(call) ==
		"builder.outcome.addPrimitive(validateSourceObjectClaimOwner(builder.ctx, builder.owner))"
}

func sourceObjectClaimFullPrefixCloseProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	builderObject types.Object,
	primitivesObject types.Object,
) bool {
	info := typedPackage.TypesInfo
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 2 ||
		types.ExprString(branch.Cond) != "!builder.outcome.proved()" {
		return false
	}
	expression, ok := branch.Body.List[0].(*ast.ExprStmt)
	call, callOK := func() (*ast.CallExpr, bool) {
		if !ok {
			return nil, false
		}
		call, callOK := sourceConstructionUnparenthesizedExpression(expression.X).(*ast.CallExpr)
		return call, callOK
	}()
	if !callOK || len(call.Args) != 2 || !sourceObjectPathMethodCallProved(
		typedPackage,
		call,
		builderObject,
		"(*sourceConstructionOwner).closeIntoWith",
		"owner",
	) || !sourceConstructionDirectObjectExpression(info, call.Args[0], primitivesObject) ||
		types.ExprString(call.Args[1]) != "&builder.outcome" {
		return false
	}
	returned, ok := branch.Body.List[1].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 {
		return false
	}
	nilIdentifier, ok := returned.Results[0].(*ast.Ident)
	return ok && info.Uses[nilIdentifier] == types.Universe.Lookup("nil") &&
		sourceConstructionSelectorChainProved(
			info,
			returned.Results[1],
			builderObject,
			"outcome",
		)
}

func sourceObjectClaimFinalOwnerValidatorProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 5 {
		return false
	}
	info := typedPackage.TypesInfo
	ownerObject := sourceObjectPathUniqueDefinedObject(info, function, "owner")
	validationObject := sourceObjectPathUniqueDefinedObject(info, function, "validationFailure")
	branch, ok := function.Body.List[2].(*ast.IfStmt)
	if !ok || ownerObject == nil || validationObject == nil || branch.Init != nil ||
		branch.Else != nil || len(branch.Body.List) != 1 || types.ExprString(branch.Cond) !=
		"owner == nil || !owner.validObjectClaimRetention()" {
		return false
	}
	assignment, ok := branch.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || !sourceConstructionDirectObjectExpression(
		info,
		assignment.Lhs[0],
		validationObject,
	) {
		return false
	}
	call, ok := sourceConstructionUnparenthesizedExpression(assignment.Rhs[0]).(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(call) !=
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)" {
		return false
	}
	returned, ok := function.Body.List[4].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && sourceConstructionDirectObjectExpression(
		info,
		returned.Results[0],
		validationObject,
	)
}

func sourceObjectClaimIdentifierCall(
	parents map[ast.Node]ast.Node,
	identifier *ast.Ident,
) *ast.CallExpr {
	node := ast.Node(identifier)
	for {
		parent := parents[node]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			continue
		}
		call, ok := parent.(*ast.CallExpr)
		if !ok || !sourceConstructionDirectExpression(call.Fun, node.(ast.Expr)) {
			return nil
		}
		return call
	}
}

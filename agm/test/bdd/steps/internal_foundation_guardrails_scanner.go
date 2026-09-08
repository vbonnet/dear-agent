package steps

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The parser's lexical object identity is deliberately used by this AST-only
// policy scanner: the repository contains mutually exclusive platform files,
// so a whole-tree go/types pass cannot provide one coherent package graph.
type buildAuthorityLexicalObject = ast.Object //nolint:staticcheck // See the AST-only rationale above.

type buildAuthorityWaitOwnershipScan struct {
	files         int
	bytes         int64
	exactPIDWaits int
	exactPIDSites []string
	violations    []string
}

type buildAuthorityWaitOwnershipLimits struct {
	maxEntries     int
	maxDirectories int
	maxFiles       int
	maxFileBytes   int64
	maxTotalBytes  int64
}

type buildAuthorityWaitOwnershipHooks struct {
	beforeSourceOpen         func(path, rel string) error
	afterSourceOpen          func(path, rel string) error
	readDirectory            func(directory *os.File, count int) ([]fs.DirEntry, error)
	beforeSnapshotValidation func() error
}

func defaultBuildAuthorityWaitOwnershipLimits() buildAuthorityWaitOwnershipLimits {
	return buildAuthorityWaitOwnershipLimits{
		maxEntries:     32768,
		maxDirectories: 8192,
		maxFiles:       4096,
		maxFileBytes:   1 << 20,
		maxTotalBytes:  32 << 20,
	}
}

func scanBuildAuthorityWaitOwnership(
	ctx context.Context,
	repoRoot string,
	limits buildAuthorityWaitOwnershipLimits,
) (buildAuthorityWaitOwnershipScan, error) {
	return scanBuildAuthorityWaitOwnershipWithHooks(ctx, repoRoot, limits, buildAuthorityWaitOwnershipHooks{})
}

func scanBuildAuthorityWaitOwnershipWithHooks(
	ctx context.Context,
	repoRoot string,
	limits buildAuthorityWaitOwnershipLimits,
	hooks buildAuthorityWaitOwnershipHooks,
) (buildAuthorityWaitOwnershipScan, error) {
	if !buildAuthorityWaitOwnershipSourceReadsSupported() {
		return buildAuthorityWaitOwnershipScan{},
			fmt.Errorf("secure production-source reads are unsupported on this platform")
	}
	return scanBuildAuthorityWaitOwnershipSecure(ctx, repoRoot, limits, hooks)
}

type buildAuthorityContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *buildAuthorityContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	read, err := reader.reader.Read(buffer)
	if contextErr := reader.ctx.Err(); contextErr != nil && err == nil {
		return read, contextErr
	}
	return read, err
}

func buildAuthorityWaitOwnershipSameSource(first, second fs.FileInfo) bool {
	return first != nil && second != nil && first.Mode().IsRegular() &&
		second.Mode().IsRegular() && buildAuthorityWaitOwnershipSameEntry(first, second)
}

func buildAuthorityWaitOwnershipSameEntry(first, second fs.FileInfo) bool {
	return first != nil && second != nil && os.SameFile(first, second) &&
		first.Mode() == second.Mode() &&
		first.Size() == second.Size() &&
		first.ModTime().Equal(second.ModTime()) &&
		buildAuthorityWaitOwnershipSameChangeTime(first, second)
}

func buildAuthorityWaitOwnershipSkippedDir(name string) bool {
	switch name {
	case ".beads", ".git", ".wayfinder", ".wayfinder-cache":
		return true
	default:
		return false
	}
}

func buildAuthorityWaitOwnershipNativeSource(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".c", ".cc", ".cpp", ".cxx", ".m", ".mm",
		".h", ".hh", ".hpp", ".hxx",
		".f", ".for", ".f90", ".s", ".sx",
		".swig", ".swigcxx", ".syso":
		return true
	default:
		return false
	}
}

func scanBuildAuthorityWaitOwnershipFile(
	result *buildAuthorityWaitOwnershipScan,
	rel string,
	fileSet *token.FileSet,
	file *ast.File,
	packageNames map[string]struct{},
) {
	newBuildAuthorityWaitOwnershipAnalyzer(result, rel, fileSet, file, packageNames).scan()
}

type buildAuthorityWaitOwnershipAnalyzer struct {
	result        *buildAuthorityWaitOwnershipScan
	rel           string
	fileSet       *token.FileSet
	file          *ast.File
	imports       map[string]string
	bindings      buildAuthorityExpressionBindings
	parents       map[ast.Node]ast.Node
	directCallees map[ast.Expr]struct{}
	packageNames  map[string]struct{}
}

func newBuildAuthorityWaitOwnershipAnalyzer(
	result *buildAuthorityWaitOwnershipScan,
	rel string,
	fileSet *token.FileSet,
	file *ast.File,
	packageNames map[string]struct{},
) *buildAuthorityWaitOwnershipAnalyzer {
	return &buildAuthorityWaitOwnershipAnalyzer{
		result:        result,
		rel:           rel,
		fileSet:       fileSet,
		file:          file,
		imports:       make(map[string]string),
		bindings:      buildAuthorityWaitOwnershipBindings(file),
		parents:       buildAuthorityWaitOwnershipParents(file),
		directCallees: make(map[ast.Expr]struct{}),
		packageNames:  packageNames,
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) scan() {
	analyzer.collectImports()
	analyzer.classifyExplicitForeignProcessDeclarations()
	analyzer.classifyReviewedLifecycleDefinitions()
	for _, group := range analyzer.file.Comments {
		for _, comment := range group.List {
			if strings.Contains(comment.Text, "go:linkname") {
				analyzer.addViolation(comment,
					"uses go:linkname outside internal/buildauthority, so wait/disposition ownership is not auditable")
			}
		}
	}
	ast.Inspect(analyzer.file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok {
			analyzer.directCallees[buildAuthorityUnwrapExpression(call.Fun)] = struct{}{}
		}
		return true
	})
	ast.Inspect(analyzer.file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			analyzer.classifyCall(value)
		case *ast.CompositeLit:
			analyzer.classifyProcessLiteral(value)
		case *ast.IndexExpr:
			analyzer.classifyGenericProcessTypeArguments(value, []ast.Expr{value.Index})
		case *ast.IndexListExpr:
			analyzer.classifyGenericProcessTypeArguments(value, value.Indices)
		case *ast.SelectorExpr:
			analyzer.classifyCallableSelectorUse(value)
		case *ast.TypeSpec:
			analyzer.classifyProcessTypeAlias(value)
		case *ast.Ident:
			analyzer.classifyReviewedFunctionUse(value)
		}
		return true
	})
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyGenericProcessTypeArguments(
	node ast.Node,
	arguments []ast.Expr,
) {
	switch analyzer.processWaitGenericTypeArgumentKind(arguments) {
	case buildAuthorityForeignProcessWait:
		analyzer.addViolation(node,
			"uses os.Process in a generic type argument, so foreign-PID wait provenance is not auditable")
	case buildAuthorityForgedCommandWait, buildAuthorityUnknownCommandWait:
		analyzer.addViolation(node,
			"uses exec.Cmd in a generic type argument, so command wait provenance is not auditable")
	case buildAuthorityOpaqueProcessWait:
		analyzer.addViolation(node,
			"uses an opaque process type in a generic type argument, so wait provenance is not auditable")
	case buildAuthorityNotProcessWait, buildAuthorityOwnedCommandWait:
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyProcessTypeAlias(alias *ast.TypeSpec) {
	if alias.Assign == token.NoPos {
		return
	}
	switch analyzer.processWaitTypeKind(alias.Type) {
	case buildAuthorityForeignProcessWait:
		analyzer.addViolation(alias,
			"aliases os.Process outside internal/buildauthority, so foreign-PID wait provenance is not auditable")
	case buildAuthorityForgedCommandWait:
		analyzer.addViolation(alias,
			"aliases exec.Cmd outside internal/buildauthority, so command wait provenance is not auditable")
	case buildAuthorityNotProcessWait, buildAuthorityOwnedCommandWait,
		buildAuthorityUnknownCommandWait, buildAuthorityOpaqueProcessWait:
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyExplicitForeignProcessDeclarations() {
	seen := make(map[*buildAuthorityLexicalObject]struct{})
	ast.Inspect(analyzer.file, func(node ast.Node) bool {
		switch declaration := node.(type) {
		case *ast.FuncDecl:
			analyzer.classifyForeignProcessParameters(declaration.Type.Params, seen)
			analyzer.classifyForeignProcessResults(declaration.Type.Results)
		case *ast.StructType:
			analyzer.classifyForeignProcessStorageFields(declaration.Fields)
		case *ast.ValueSpec:
			if analyzer.processWaitTypeKind(declaration.Type) == buildAuthorityForeignProcessWait {
				analyzer.classifyForeignProcessObjects(declaration.Names, seen)
			}
		}
		return true
	})
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyForeignProcessParameters(
	parameters *ast.FieldList,
	seen map[*buildAuthorityLexicalObject]struct{},
) {
	if parameters == nil {
		return
	}
	for _, parameter := range parameters.List {
		if analyzer.processWaitTypeKind(parameter.Type) == buildAuthorityForeignProcessWait {
			analyzer.classifyForeignProcessObjects(parameter.Names, seen)
		}
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyForeignProcessObjects(
	names []*ast.Ident,
	seen map[*buildAuthorityLexicalObject]struct{},
) {
	for _, name := range names {
		if name.Obj == nil || name.Name == "_" {
			continue
		}
		if _, classified := seen[name.Obj]; classified {
			continue
		}
		seen[name.Obj] = struct{}{}
		if analyzer.findProcessResultHasOpaqueUse(name.Obj) {
			analyzer.addViolation(name,
				"uses an os.Process through an opaque path that could enable foreign-PID reaping")
		}
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyForeignProcessResults(
	results *ast.FieldList,
) {
	if results == nil {
		return
	}
	for _, result := range results.List {
		if analyzer.processWaitTypeKind(result.Type) == buildAuthorityForeignProcessWait {
			analyzer.addViolation(result,
				"returns an os.Process through an opaque path that could enable foreign-PID reaping")
		}
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyForeignProcessStorageFields(
	fields *ast.FieldList,
) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		if analyzer.processWaitTypeKind(field.Type) == buildAuthorityForeignProcessWait {
			analyzer.addViolation(field,
				"stores an os.Process through an opaque path that could enable foreign-PID reaping")
		}
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyProcessLiteral(
	literal *ast.CompositeLit,
) {
	kind := analyzer.processWaitTypeKind(literal.Type)
	switch kind {
	case buildAuthorityForeignProcessWait:
		analyzer.addViolation(literal,
			"constructs a forged os.Process outside an owned exec.Cmd")
	case buildAuthorityForgedCommandWait:
		for _, element := range literal.Elts {
			keyValue, keyed := element.(*ast.KeyValueExpr)
			if !keyed {
				analyzer.addViolation(literal,
					"constructs a forged exec.Cmd with opaque process state")
				return
			}
			key, ok := buildAuthorityUnwrapExpression(keyValue.Key).(*ast.Ident)
			if ok && key.Name == "Process" {
				analyzer.addViolation(literal,
					"constructs a forged exec.Cmd Process outside an owned command constructor")
				return
			}
		}
	case buildAuthorityNotProcessWait, buildAuthorityOwnedCommandWait,
		buildAuthorityUnknownCommandWait, buildAuthorityOpaqueProcessWait:
	}
}

type buildAuthorityLifecyclePointerKind uint8

const (
	buildAuthorityNotLifecyclePointer buildAuthorityLifecyclePointerKind = iota
	buildAuthorityLifecycleCommandPointer
	buildAuthorityLifecycleProcessPointer
)

type buildAuthorityLifecycleMutationTracker struct {
	analyzer     *buildAuthorityWaitOwnershipAnalyzer
	declaration  *ast.FuncDecl
	receiver     *buildAuthorityLexicalObject
	process      *buildAuthorityLexicalObject
	aliases      map[*buildAuthorityLexicalObject]buildAuthorityLifecyclePointerKind
	allowGitKill bool
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyReviewedLifecycleDefinitions() {
	for _, node := range analyzer.file.Decls {
		declaration, ok := node.(*ast.FuncDecl)
		if !ok {
			continue
		}
		switch {
		case analyzer.reviewedLifecycleConstructor(declaration):
		case analyzer.reviewedLifecycleMethod(declaration):
		case analyzer.reviewedProcessHelper(declaration):
		}
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) reviewedLifecycleConstructor(
	declaration *ast.FuncDecl,
) bool {
	typeName := ""
	switch {
	case analyzer.rel == "internal/specguard/git_exec.go" &&
		analyzer.file.Name.Name == "specguard" &&
		declaration.Name.Name == "newGitProcessGroupLifecycle":
		typeName = "gitProcessGroupLifecycle"
	case analyzer.rel == "agm/test/bdd/steps/spec_governance_tooling_steps.go" &&
		analyzer.file.Name.Name == "steps" &&
		declaration.Name.Name == "newSpecAuditProcessGroupLifecycle":
		typeName = "specAuditProcessGroupLifecycle"
	default:
		return false
	}
	if !analyzer.exactLifecycleConstructor(declaration, typeName) {
		analyzer.addViolation(declaration,
			"reviewed lifecycle constructor may replace or expose its owned command or Process")
	}
	return true
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) exactLifecycleConstructor(
	declaration *ast.FuncDecl,
	typeName string,
) bool {
	command, literal, ok := analyzer.exactLifecycleConstructorParts(declaration, typeName)
	if !ok {
		return false
	}
	return buildAuthorityExactLifecycleConstructorFields(command, literal)
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) exactLifecycleConstructorParts(
	declaration *ast.FuncDecl,
	typeName string,
) (*ast.Ident, *ast.CompositeLit, bool) {
	resultType, resultOK := buildAuthorityExactLifecycleConstructorResult(
		declaration.Type.Results, typeName,
	)
	if declaration.Recv != nil || declaration.Body == nil || !resultOK {
		return nil, nil, false
	}
	command, commandOK := analyzer.exactLifecycleCommandParameter(declaration.Type.Params)
	literal, literalOK := buildAuthorityExactLifecycleReturnedLiteral(declaration.Body)
	if !commandOK || !literalOK || !buildAuthorityLifecycleLiteralHasType(
		literal, resultType, typeName,
	) {
		return nil, nil, false
	}
	return command, literal, true
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) exactLifecycleCommandParameter(
	parameters *ast.FieldList,
) (*ast.Ident, bool) {
	if parameters == nil || len(parameters.List) != 1 {
		return nil, false
	}
	parameter := parameters.List[0]
	if len(parameter.Names) != 1 || !analyzer.typeIsExecCmdPointer(parameter.Type) {
		return nil, false
	}
	return parameter.Names[0], true
}

func buildAuthorityExactLifecycleReturnedLiteral(
	body *ast.BlockStmt,
) (*ast.CompositeLit, bool) {
	if body == nil || len(body.List) != 1 {
		return nil, false
	}
	returned, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return nil, false
	}
	pointer, ok := buildAuthorityUnwrapExpression(returned.Results[0]).(*ast.UnaryExpr)
	if !ok || pointer.Op != token.AND {
		return nil, false
	}
	literal, ok := buildAuthorityUnwrapExpression(pointer.X).(*ast.CompositeLit)
	if !ok || len(literal.Elts) != 2 {
		return nil, false
	}
	return literal, true
}

func buildAuthorityLifecycleLiteralHasType(
	literal *ast.CompositeLit,
	resultType *ast.Ident,
	typeName string,
) bool {
	literalType, ok := buildAuthorityUnwrapExpression(literal.Type).(*ast.Ident)
	if !ok || literalType.Name != typeName || literalType.Obj == nil || resultType.Obj == nil {
		return false
	}
	return literalType.Obj == resultType.Obj
}

func buildAuthorityExactLifecycleConstructorResult(
	results *ast.FieldList,
	typeName string,
) (*ast.Ident, bool) {
	if results == nil || len(results.List) != 1 || len(results.List[0].Names) != 0 {
		return nil, false
	}
	pointer, ok := buildAuthorityUnwrapExpression(results.List[0].Type).(*ast.StarExpr)
	if !ok {
		return nil, false
	}
	identifier, ok := buildAuthorityUnwrapExpression(pointer.X).(*ast.Ident)
	return identifier, ok && identifier.Name == typeName
}

func buildAuthorityExactLifecycleConstructorFields(
	command *ast.Ident,
	literal *ast.CompositeLit,
) bool {
	commandField := false
	enabledField := false
	for _, element := range literal.Elts {
		keyValue, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, keyOK := buildAuthorityUnwrapExpression(keyValue.Key).(*ast.Ident)
		value, valueOK := buildAuthorityUnwrapExpression(keyValue.Value).(*ast.Ident)
		if !keyOK || !valueOK {
			return false
		}
		switch key.Name {
		case "command":
			commandField = value.Obj != nil && value.Obj == command.Obj
		case "enabled":
			enabledField = value.Name == "true" && value.Obj == nil
		default:
			return false
		}
	}
	return commandField && enabledField
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) reviewedLifecycleMethod(
	declaration *ast.FuncDecl,
) bool {
	typeName := buildAuthorityMethodReceiverTypeName(declaration)
	reviewed := analyzer.rel == "internal/specguard/git_exec.go" &&
		analyzer.file.Name.Name == "specguard" && typeName == "gitProcessGroupLifecycle"
	allowGitKill := reviewed
	if !reviewed {
		reviewed = analyzer.rel == "agm/test/bdd/steps/spec_governance_tooling_steps.go" &&
			analyzer.file.Name.Name == "steps" && typeName == "specAuditProcessGroupLifecycle"
	}
	if !reviewed {
		return false
	}
	receiver := buildAuthorityMethodReceiverObject(declaration)
	if receiver == nil {
		analyzer.addViolation(declaration, "reviewed lifecycle method has an opaque receiver")
		return true
	}
	tracker := buildAuthorityLifecycleMutationTracker{
		analyzer: analyzer, declaration: declaration, receiver: receiver,
		aliases:      make(map[*buildAuthorityLexicalObject]buildAuthorityLifecyclePointerKind),
		allowGitKill: allowGitKill,
	}
	if tracker.unsafe() {
		analyzer.addViolation(declaration,
			"reviewed lifecycle method may mutate, replace, or expose its owned command or Process")
	}
	return true
}

func buildAuthorityMethodReceiverTypeName(declaration *ast.FuncDecl) string {
	if declaration.Recv == nil || len(declaration.Recv.List) != 1 {
		return ""
	}
	expression := buildAuthorityUnwrapExpression(declaration.Recv.List[0].Type)
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = buildAuthorityUnwrapExpression(pointer.X)
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return ""
	}
	return identifier.Name
}

func buildAuthorityMethodReceiverObject(
	declaration *ast.FuncDecl,
) *buildAuthorityLexicalObject {
	if declaration.Recv == nil || len(declaration.Recv.List) != 1 ||
		len(declaration.Recv.List[0].Names) != 1 {
		return nil
	}
	return declaration.Recv.List[0].Names[0].Obj
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) reviewedProcessHelper(
	declaration *ast.FuncDecl,
) bool {
	if filepath.ToSlash(filepath.Dir(filepath.FromSlash(analyzer.rel))) != "internal/specguard" ||
		analyzer.file.Name.Name != "specguard" || declaration.Name.Name != "killProcessGroup" {
		return false
	}
	if declaration.Type.Params == nil || len(declaration.Type.Params.List) != 1 {
		analyzer.addViolation(declaration, "reviewed process-group helper has an opaque signature")
		return true
	}
	parameter := declaration.Type.Params.List[0]
	if !analyzer.typeIsOSProcessPointer(parameter.Type) || len(parameter.Names) != 1 {
		analyzer.addViolation(declaration, "reviewed process-group helper has an opaque signature")
		return true
	}
	if parameter.Names[0].Name == "_" {
		return true
	}
	tracker := buildAuthorityLifecycleMutationTracker{
		analyzer: analyzer, declaration: declaration, process: parameter.Names[0].Obj,
		aliases: make(map[*buildAuthorityLexicalObject]buildAuthorityLifecyclePointerKind),
	}
	if tracker.unsafe() {
		analyzer.addViolation(declaration,
			"reviewed process-group helper may mutate, replace, or expose its Process")
	}
	return true
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) typeIsOSProcessPointer(
	expression ast.Expr,
) bool {
	pointer, ok := buildAuthorityUnwrapExpression(expression).(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := buildAuthorityUnwrapExpression(pointer.X).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Process" {
		return false
	}
	importPath, _, imported := analyzer.importedSelector(selector)
	return imported && importPath == "os"
}

func (tracker *buildAuthorityLifecycleMutationTracker) unsafe() bool {
	unsafe := false
	ast.Inspect(tracker.declaration.Body, func(node ast.Node) bool {
		if node == nil || unsafe {
			return !unsafe
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			unsafe = tracker.recordAssignment(value)
		case *ast.ValueSpec:
			unsafe = tracker.recordValueSpec(value)
		case *ast.IncDecStmt:
			unsafe = tracker.expressionTargetsProtectedState(value.X)
		case *ast.SendStmt:
			unsafe = tracker.expressionCarriesPointer(value.Chan) ||
				tracker.expressionCarriesPointer(value.Value)
		case *ast.ReturnStmt:
			unsafe = slices.ContainsFunc(value.Results, tracker.expressionCarriesPointer)
		case *ast.CallExpr:
			unsafe = tracker.callExposesPointer(value)
		}
		return !unsafe
	})
	return unsafe
}

func (tracker *buildAuthorityLifecycleMutationTracker) recordAssignment(
	assignment *ast.AssignStmt,
) bool {
	if slices.ContainsFunc(assignment.Lhs, tracker.expressionTargetsProtectedState) {
		return true
	}
	if len(assignment.Lhs) == len(assignment.Rhs) {
		for index, source := range assignment.Rhs {
			if tracker.bindAlias(assignment.Lhs[index], source) {
				return true
			}
		}
		return false
	}
	return slices.ContainsFunc(assignment.Rhs, tracker.expressionCarriesPointer)
}

func (tracker *buildAuthorityLifecycleMutationTracker) recordValueSpec(
	spec *ast.ValueSpec,
) bool {
	if len(spec.Names) != len(spec.Values) {
		return slices.ContainsFunc(spec.Values, tracker.expressionCarriesPointer)
	}
	for index, source := range spec.Values {
		if tracker.bindAlias(spec.Names[index], source) {
			return true
		}
	}
	return false
}

func (tracker *buildAuthorityLifecycleMutationTracker) bindAlias(
	target ast.Expr,
	source ast.Expr,
) bool {
	kind := tracker.pointerKind(source)
	if kind == buildAuthorityNotLifecyclePointer && !tracker.expressionCarriesPointer(source) {
		return false
	}
	identifier, ok := buildAuthorityUnwrapExpression(target).(*ast.Ident)
	if !ok || identifier.Obj == nil {
		return true
	}
	if kind == buildAuthorityNotLifecyclePointer {
		kind = buildAuthorityLifecycleCommandPointer
	}
	tracker.aliases[identifier.Obj] = kind
	return false
}

func (tracker *buildAuthorityLifecycleMutationTracker) pointerKind(
	expression ast.Expr,
) buildAuthorityLifecyclePointerKind {
	expression = buildAuthorityUnwrapExpression(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		if tracker.process != nil && value.Obj != nil && value.Obj == tracker.process {
			return buildAuthorityLifecycleProcessPointer
		}
		return tracker.aliases[value.Obj]
	case *ast.SelectorExpr:
		if root, ok := buildAuthorityUnwrapExpression(value.X).(*ast.Ident); ok &&
			root.Obj == tracker.receiver && value.Sel.Name == "command" {
			return buildAuthorityLifecycleCommandPointer
		}
		if tracker.pointerKind(value.X) == buildAuthorityLifecycleCommandPointer &&
			value.Sel.Name == "Process" {
			return buildAuthorityLifecycleProcessPointer
		}
	case *ast.UnaryExpr:
		if value.Op == token.MUL {
			return tracker.pointerKind(value.X)
		}
	}
	return buildAuthorityNotLifecyclePointer
}

func (tracker *buildAuthorityLifecycleMutationTracker) expressionCarriesPointer(
	expression ast.Expr,
) bool {
	if expression == nil {
		return false
	}
	expression = buildAuthorityUnwrapExpression(expression)
	if tracker.pointerKind(expression) != buildAuthorityNotLifecyclePointer {
		return true
	}
	if _, ok := expression.(*ast.CallExpr); ok {
		return false
	}
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.AND &&
		tracker.expressionTargetsProtectedState(unary.X) {
		return true
	}
	if tracker.expressionIsProtectedScalar(expression) {
		return false
	}
	return tracker.expressionContainsNestedPointer(expression)
}

func (tracker *buildAuthorityLifecycleMutationTracker) expressionIsProtectedScalar(
	expression ast.Expr,
) bool {
	selector, ok := buildAuthorityUnwrapExpression(expression).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if tracker.pointerKind(selector.X) == buildAuthorityLifecycleProcessPointer &&
		selector.Sel.Name == "Pid" {
		return true
	}
	return tracker.pointerKind(selector.X) == buildAuthorityLifecycleCommandPointer &&
		selector.Sel.Name != "Process"
}

func (tracker *buildAuthorityLifecycleMutationTracker) expressionContainsNestedPointer(
	expression ast.Expr,
) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if node == nil || found {
			return !found
		}
		subexpression, ok := node.(ast.Expr)
		if !ok || subexpression == expression {
			return true
		}
		if tracker.expressionIsProtectedScalar(subexpression) {
			return false
		}
		if tracker.pointerKind(subexpression) != buildAuthorityNotLifecyclePointer {
			found = true
			return false
		}
		return true
	})
	return found
}

func (tracker *buildAuthorityLifecycleMutationTracker) expressionTargetsProtectedState(
	expression ast.Expr,
) bool {
	expression = buildAuthorityUnwrapExpression(expression)
	if tracker.pointerKind(expression) != buildAuthorityNotLifecyclePointer {
		return true
	}
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		return tracker.pointerKind(value.X) != buildAuthorityNotLifecyclePointer ||
			tracker.expressionTargetsProtectedState(value.X)
	case *ast.IndexExpr:
		return tracker.expressionTargetsProtectedState(value.X)
	case *ast.StarExpr:
		return tracker.expressionCarriesPointer(value.X)
	}
	return false
}

func (tracker *buildAuthorityLifecycleMutationTracker) callExposesPointer(
	call *ast.CallExpr,
) bool {
	if tracker.reviewedGitKill(call) {
		return false
	}
	if slices.ContainsFunc(call.Args, tracker.expressionCarriesPointer) {
		return true
	}
	function := buildAuthorityUnwrapExpression(call.Fun)
	if selector, ok := function.(*ast.SelectorExpr); ok {
		return tracker.expressionCarriesPointer(selector.X)
	}
	return tracker.expressionCarriesPointer(function)
}

func (tracker *buildAuthorityLifecycleMutationTracker) reviewedGitKill(
	call *ast.CallExpr,
) bool {
	if !tracker.allowGitKill || len(call.Args) != 1 {
		return false
	}
	function, ok := buildAuthorityUnwrapExpression(call.Fun).(*ast.Ident)
	if !ok || function.Name != "killProcessGroup" || function.Obj != nil {
		return false
	}
	if _, declared := tracker.analyzer.packageNames[function.Name]; !declared {
		return false
	}
	return tracker.pointerKind(call.Args[0]) == buildAuthorityLifecycleProcessPointer
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) collectImports() {
	for _, spec := range analyzer.file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			analyzer.addViolation(spec, "contains an unreadable import path")
			continue
		}
		name := filepath.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if importPath == "C" {
			analyzer.addViolation(spec, "imports C outside internal/buildauthority")
			continue
		}
		switch name {
		case "_":
			continue
		case ".":
			if buildAuthorityWaitOwnershipAuditedImport(importPath) {
				analyzer.addViolation(spec,
					fmt.Sprintf("dot imports wait/disposition package %q, which is not auditable", importPath))
			}
		default:
			analyzer.imports[name] = importPath
		}
	}
}

func buildAuthorityWaitOwnershipAuditedImport(importPath string) bool {
	switch importPath {
	case "os", "os/exec", "os/signal", "syscall", "golang.org/x/sys/unix":
		return true
	default:
		return false
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyCall(call *ast.CallExpr) {
	if analyzer.callConstructsForgedOSProcess(call) {
		analyzer.addViolation(call, "constructs a forged os.Process outside an owned exec.Cmd")
	}
	switch function := buildAuthorityUnwrapExpression(call.Fun).(type) {
	case *ast.SelectorExpr:
		if function.Sel.Name == "Wait" && analyzer.classifyProcessWait(call, function) {
			return
		}
		importPath, name, ok := analyzer.importedSelector(function)
		if !ok {
			return
		}
		switch importPath {
		case "os/signal":
			analyzer.classifySignalCall(call, name)
		case "os":
			if name == "FindProcess" {
				analyzer.classifyFindProcess(call)
				return
			}
		case "syscall", "golang.org/x/sys/unix":
			analyzer.classifyKernelCall(call, name)
		}
	case *ast.Ident:
		if function.Obj != nil && function.Obj.Kind != ast.Fun {
			return
		}
		if buildAuthorityReviewedWaitSeam(analyzer.rel, analyzer.file.Name.Name, function.Name) {
			analyzer.classifyReviewedWaitSeamCall(call, function.Name)
			return
		}
		if buildAuthorityReviewedSignalSeam(analyzer.rel, analyzer.file.Name.Name, function.Name) {
			analyzer.classifyReviewedSignalSeamCall(call)
		}
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) callConstructsForgedOSProcess(
	call *ast.CallExpr,
) bool {
	if len(call.Args) != 1 {
		return false
	}
	if analyzer.processWaitTypeKind(call.Fun) == buildAuthorityForeignProcessWait {
		return true
	}
	identifier, ok := buildAuthorityUnwrapExpression(call.Fun).(*ast.Ident)
	return ok && analyzer.isPredeclaredIdentifier(identifier, "new") &&
		analyzer.processWaitTypeKind(call.Args[0]) == buildAuthorityForeignProcessWait
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyFindProcess(call *ast.CallExpr) {
	process := analyzer.findProcessResultObject(call)
	if process == nil || analyzer.findProcessResultHasOpaqueUse(process) {
		analyzer.addViolation(call,
			"uses an os.FindProcess result through an opaque path that could enable foreign-PID reaping")
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) findProcessResultObject(
	call *ast.CallExpr,
) *buildAuthorityLexicalObject {
	node := ast.Node(call)
	parent := analyzer.parents[node]
	for {
		parenthesized, ok := parent.(*ast.ParenExpr)
		if !ok {
			break
		}
		node = parenthesized
		parent = analyzer.parents[node]
	}
	var targets []ast.Expr
	switch declaration := parent.(type) {
	case *ast.AssignStmt:
		if len(declaration.Rhs) == 1 && buildAuthorityUnwrapExpression(declaration.Rhs[0]) == call {
			targets = declaration.Lhs
		}
	case *ast.ValueSpec:
		if len(declaration.Values) == 1 && buildAuthorityUnwrapExpression(declaration.Values[0]) == call {
			for _, name := range declaration.Names {
				targets = append(targets, name)
			}
		}
	}
	if len(targets) < 1 {
		return nil
	}
	identifier, ok := buildAuthorityUnwrapExpression(targets[0]).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return nil
	}
	return identifier.Obj
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) findProcessResultHasOpaqueUse(
	process *buildAuthorityLexicalObject,
) bool {
	opaque := false
	ast.Inspect(analyzer.file, func(node ast.Node) bool {
		if node == nil || opaque {
			return !opaque
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Obj == nil || identifier.Obj != process ||
			analyzer.identifierIsDeclaration(identifier) {
			return true
		}
		opaque = !analyzer.findProcessResultUseIsSafe(identifier)
		return !opaque
	})
	return opaque
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) findProcessResultUseIsSafe(
	identifier *ast.Ident,
) bool {
	node := ast.Node(identifier)
	parent := analyzer.parents[node]
	for {
		parenthesized, ok := parent.(*ast.ParenExpr)
		if !ok {
			break
		}
		node = parenthesized
		parent = analyzer.parents[node]
	}
	if selector, ok := parent.(*ast.SelectorExpr); ok && selector.X == node {
		if selector.Sel.Name == "Pid" {
			return true
		}
		_, direct := analyzer.directCallees[selector]
		return direct && (selector.Sel.Name == "Signal" || selector.Sel.Name == "Kill" ||
			selector.Sel.Name == "Release")
	}
	binary, ok := parent.(*ast.BinaryExpr)
	if !ok || (binary.Op != token.EQL && binary.Op != token.NEQ) {
		return false
	}
	other := binary.X
	if other == node {
		other = binary.Y
	}
	nilIdentifier, ok := buildAuthorityUnwrapExpression(other).(*ast.Ident)
	return ok && analyzer.isPredeclaredIdentifier(nilIdentifier, "nil")
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyCallableSelectorUse(selector *ast.SelectorExpr) {
	if _, direct := analyzer.directCallees[selector]; direct {
		return
	}
	if selector.Sel.Name == "Wait" && analyzer.classifyProcessWait(nil, selector) {
		return
	}
	importPath, name, ok := analyzer.importedSelector(selector)
	if !ok || !buildAuthoritySensitiveCallable(importPath, name) {
		return
	}
	analyzer.addViolation(selector,
		fmt.Sprintf("uses %s.%s as a function value, so the wait/disposition call is not auditable",
			filepath.Base(importPath), name))
}

func buildAuthoritySensitiveCallable(importPath, name string) bool {
	switch importPath {
	case "os":
		return name == "FindProcess"
	case "os/signal":
		switch name {
		case "Notify", "NotifyContext", "Ignore", "Reset", "Stop":
			return true
		}
	case "syscall", "golang.org/x/sys/unix":
		switch name {
		case "Sigaction", "Wait", "Wait3", "Wait4", "Waitid", "Waitpid":
			return true
		}
		return buildAuthorityRawSyscallName(name)
	}
	return false
}

type buildAuthorityProcessWaitKind uint8

const (
	buildAuthorityNotProcessWait buildAuthorityProcessWaitKind = iota
	buildAuthorityOwnedCommandWait
	buildAuthorityUnknownCommandWait
	buildAuthorityForeignProcessWait
	buildAuthorityForgedCommandWait
	buildAuthorityOpaqueProcessWait
)

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyProcessWait(
	call *ast.CallExpr,
	selector *ast.SelectorExpr,
) bool {
	kind := analyzer.processWaitTypeKind(selector.X)
	if kind == buildAuthorityNotProcessWait {
		kind = analyzer.processWaitKind(
			selector.X, selector.Pos(), make(map[*buildAuthorityLexicalObject]struct{}),
		)
	}
	if kind == buildAuthorityNotProcessWait {
		kind = analyzer.processWaitGenericProvenanceKind(
			selector.X, selector.Pos(), make(map[*buildAuthorityLexicalObject]struct{}),
		)
	}
	if kind == buildAuthorityNotProcessWait {
		return false
	}
	node := ast.Node(selector)
	if call != nil {
		node = call
	}
	if call == nil {
		analyzer.addViolation(node,
			"uses a process Wait method as a function value, so exact-child ownership is not auditable")
		return true
	}
	if len(call.Args) != 0 {
		analyzer.addViolation(node,
			"uses a process Wait call with an opaque or invalid argument shape")
		return true
	}
	switch kind {
	case buildAuthorityNotProcessWait:
		return false
	case buildAuthorityOwnedCommandWait, buildAuthorityUnknownCommandWait:
		return true
	case buildAuthorityForeignProcessWait:
		analyzer.addViolation(node,
			"reaps an os.Process obtained outside an owned exec.Cmd")
	case buildAuthorityForgedCommandWait:
		analyzer.addViolation(node,
			"reaps through a forged exec.Cmd instead of an owned command constructor")
	case buildAuthorityOpaqueProcessWait:
		analyzer.addViolation(node,
			"uses a dynamically replaced process Wait receiver whose ownership is not auditable")
	}
	return true
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitKind(
	expression ast.Expr,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	expression = buildAuthorityUnwrapExpression(expression)
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		if value.Op == token.AND || value.Op == token.MUL {
			return analyzer.processWaitKind(value.X, before, visiting)
		}
	case *ast.CompositeLit:
		return analyzer.processWaitTypeKind(value.Type)
	case *ast.CallExpr:
		if kind := analyzer.processWaitConstructorKind(value); kind != buildAuthorityNotProcessWait {
			return kind
		}
		return analyzer.processWaitCallResultKind(value)
	case *ast.Ident:
		return analyzer.processWaitObjectKind(value.Obj, before, visiting)
	case *ast.IndexExpr:
		if kind := analyzer.processWaitIndexedElementKind(value.X, before, visiting); kind != buildAuthorityNotProcessWait {
			return kind
		}
		return analyzer.processWaitGenericProvenanceKind(value, before, visiting)
	case *ast.IndexListExpr:
		return analyzer.processWaitGenericProvenanceKind(value, before, visiting)
	case *ast.SelectorExpr:
		if value.Sel.Name != "Process" {
			return buildAuthorityNotProcessWait
		}
		switch analyzer.processWaitKind(value.X, before, visiting) {
		case buildAuthorityOwnedCommandWait:
			return buildAuthorityOwnedCommandWait
		case buildAuthorityUnknownCommandWait, buildAuthorityForgedCommandWait,
			buildAuthorityOpaqueProcessWait:
			return buildAuthorityForeignProcessWait
		case buildAuthorityNotProcessWait, buildAuthorityForeignProcessWait:
		}
	}
	return buildAuthorityNotProcessWait
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitIndexedElementKind(
	expression ast.Expr,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	typeExpression := analyzer.processWaitContainerType(expression, before, visiting)
	if typeExpression == nil {
		return buildAuthorityNotProcessWait
	}
	typeExpression = buildAuthorityUnwrapExpression(typeExpression)
	switch container := typeExpression.(type) {
	case *ast.ArrayType:
		return analyzer.processWaitTypeKind(container.Elt)
	case *ast.MapType:
		return analyzer.processWaitTypeKind(container.Value)
	case *ast.Ident:
		if container.Obj == nil {
			return buildAuthorityNotProcessWait
		}
		declaration, ok := container.Obj.Decl.(*ast.TypeSpec)
		if !ok {
			return buildAuthorityNotProcessWait
		}
		return analyzer.processWaitIndexedElementKind(declaration.Type, before, visiting)
	default:
		return buildAuthorityNotProcessWait
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitContainerType(
	expression ast.Expr,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) ast.Expr {
	expression = buildAuthorityUnwrapExpression(expression)
	switch value := expression.(type) {
	case *ast.ArrayType, *ast.MapType:
		return expression
	case *ast.CompositeLit:
		return value.Type
	case *ast.TypeAssertExpr:
		return value.Type
	case *ast.Ident:
		if value.Obj == nil {
			return nil
		}
		if _, found := visiting[value.Obj]; found {
			return nil
		}
		visiting[value.Obj] = struct{}{}
		defer delete(visiting, value.Obj)
		switch declaration := value.Obj.Decl.(type) {
		case *ast.Field:
			return declaration.Type
		case *ast.ValueSpec:
			if declaration.Type != nil {
				return declaration.Type
			}
			for index, name := range declaration.Names {
				if name.Obj == value.Obj && index < len(declaration.Values) {
					return analyzer.processWaitContainerType(declaration.Values[index], before, visiting)
				}
			}
		case *ast.AssignStmt:
			for index, target := range declaration.Lhs {
				identifier, ok := buildAuthorityUnwrapExpression(target).(*ast.Ident)
				if ok && identifier.Obj == value.Obj && index < len(declaration.Rhs) {
					return analyzer.processWaitContainerType(declaration.Rhs[index], before, visiting)
				}
			}
		}
	case *ast.CallExpr:
		functionType := buildAuthorityCalledFunctionType(value.Fun)
		if functionType == nil || functionType.Results == nil || len(functionType.Results.List) != 1 {
			return nil
		}
		return functionType.Results.List[0].Type
	}
	return nil
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitCallResultKind(
	call *ast.CallExpr,
) buildAuthorityProcessWaitKind {
	functionType := buildAuthorityCalledFunctionType(call.Fun)
	if functionType == nil || functionType.Results == nil || len(functionType.Results.List) != 1 {
		return buildAuthorityNotProcessWait
	}
	result := functionType.Results.List[0]
	if len(result.Names) > 1 {
		return buildAuthorityNotProcessWait
	}
	switch analyzer.processWaitTypeKind(result.Type) {
	case buildAuthorityForeignProcessWait:
		return buildAuthorityForeignProcessWait
	case buildAuthorityForgedCommandWait:
		return buildAuthorityOpaqueProcessWait
	case buildAuthorityNotProcessWait, buildAuthorityOwnedCommandWait,
		buildAuthorityUnknownCommandWait, buildAuthorityOpaqueProcessWait:
		return buildAuthorityNotProcessWait
	}
	return buildAuthorityNotProcessWait
}

func buildAuthorityCalledFunctionType(expression ast.Expr) *ast.FuncType {
	return buildAuthorityCalledFunctionTypeSeen(
		expression, make(map[*buildAuthorityLexicalObject]struct{}),
	)
}

func buildAuthorityCalledFunctionTypeSeen(
	expression ast.Expr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) *ast.FuncType {
	expression = buildAuthorityUnwrapExpression(expression)
	switch value := expression.(type) {
	case *ast.FuncLit:
		return value.Type
	case *ast.Ident:
		if value.Obj == nil {
			return nil
		}
		if _, found := visiting[value.Obj]; found {
			return nil
		}
		visiting[value.Obj] = struct{}{}
		defer delete(visiting, value.Obj)
		return buildAuthorityObjectFunctionType(value.Obj, visiting)
	case *ast.IndexExpr:
		return buildAuthorityCalledFunctionTypeSeen(value.X, visiting)
	case *ast.IndexListExpr:
		return buildAuthorityCalledFunctionTypeSeen(value.X, visiting)
	default:
		return nil
	}
}

func buildAuthorityObjectFunctionType(
	object *buildAuthorityLexicalObject,
	visiting map[*buildAuthorityLexicalObject]struct{},
) *ast.FuncType {
	if object == nil {
		return nil
	}
	switch declaration := object.Decl.(type) {
	case *ast.FuncDecl:
		return declaration.Type
	case *ast.Field:
		return buildAuthorityFunctionTypeFromType(declaration.Type, visiting)
	case *ast.ValueSpec:
		if function := buildAuthorityFunctionTypeFromType(declaration.Type, visiting); function != nil {
			return function
		}
		for index, name := range declaration.Names {
			if name.Obj == object && index < len(declaration.Values) {
				return buildAuthorityCalledFunctionTypeSeen(declaration.Values[index], visiting)
			}
		}
	case *ast.AssignStmt:
		for index, target := range declaration.Lhs {
			identifier, ok := buildAuthorityUnwrapExpression(target).(*ast.Ident)
			if !ok || identifier.Obj != object || index >= len(declaration.Rhs) {
				continue
			}
			return buildAuthorityCalledFunctionTypeSeen(declaration.Rhs[index], visiting)
		}
	}
	return nil
}

func buildAuthorityFunctionTypeFromType(
	expression ast.Expr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) *ast.FuncType {
	expression = buildAuthorityUnwrapExpression(expression)
	if function, ok := expression.(*ast.FuncType); ok {
		return function
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok || identifier.Obj == nil {
		return nil
	}
	if _, found := visiting[identifier.Obj]; found {
		return nil
	}
	declaration, ok := identifier.Obj.Decl.(*ast.TypeSpec)
	if !ok {
		return nil
	}
	visiting[identifier.Obj] = struct{}{}
	defer delete(visiting, identifier.Obj)
	return buildAuthorityFunctionTypeFromType(declaration.Type, visiting)
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitConstructorKind(
	call *ast.CallExpr,
) buildAuthorityProcessWaitKind {
	if kind := analyzer.processWaitGenericProvenanceKind(
		call.Fun, call.Pos(), make(map[*buildAuthorityLexicalObject]struct{}),
	); kind != buildAuthorityNotProcessWait {
		return kind
	}
	if selector, ok := buildAuthorityUnwrapExpression(call.Fun).(*ast.SelectorExpr); ok {
		importPath, name, imported := analyzer.importedSelector(selector)
		if imported {
			switch {
			case importPath == "os" && name == "FindProcess":
				return buildAuthorityForeignProcessWait
			case importPath == "os/exec" && (name == "Command" || name == "CommandContext"):
				return buildAuthorityOwnedCommandWait
			}
		}
	}
	if len(call.Args) == 1 {
		switch analyzer.processWaitTypeKind(call.Fun) {
		case buildAuthorityForeignProcessWait:
			return buildAuthorityForeignProcessWait
		case buildAuthorityForgedCommandWait:
			return buildAuthorityOpaqueProcessWait
		case buildAuthorityNotProcessWait, buildAuthorityOwnedCommandWait,
			buildAuthorityUnknownCommandWait, buildAuthorityOpaqueProcessWait:
		}
	}
	identifier, ok := buildAuthorityUnwrapExpression(call.Fun).(*ast.Ident)
	if !ok || len(call.Args) != 1 ||
		!analyzer.isPredeclaredIdentifier(identifier, "new") {
		return buildAuthorityNotProcessWait
	}
	return analyzer.processWaitTypeKind(call.Args[0])
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitGenericProvenanceKind(
	expression ast.Expr,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	expression = buildAuthorityUnwrapExpression(expression)
	switch value := expression.(type) {
	case *ast.IndexExpr:
		if kind := analyzer.processWaitGenericTypeArgumentKind([]ast.Expr{value.Index}); kind != buildAuthorityNotProcessWait {
			return kind
		}
		return analyzer.processWaitGenericProvenanceKind(value.X, before, visiting)
	case *ast.IndexListExpr:
		if kind := analyzer.processWaitGenericTypeArgumentKind(value.Indices); kind != buildAuthorityNotProcessWait {
			return kind
		}
		return analyzer.processWaitGenericProvenanceKind(value.X, before, visiting)
	case *ast.Ident:
		return analyzer.processWaitGenericObjectKind(value.Obj, before, visiting)
	case *ast.SelectorExpr:
		return analyzer.processWaitGenericProvenanceKind(value.X, before, visiting)
	case *ast.CompositeLit:
		return analyzer.processWaitGenericProvenanceKind(value.Type, before, visiting)
	case *ast.CallExpr:
		return analyzer.processWaitGenericProvenanceKind(value.Fun, before, visiting)
	case *ast.UnaryExpr:
		return analyzer.processWaitGenericProvenanceKind(value.X, before, visiting)
	case *ast.StarExpr:
		return analyzer.processWaitGenericProvenanceKind(value.X, before, visiting)
	case *ast.TypeAssertExpr:
		if kind := analyzer.processWaitTypeKind(value.Type); kind != buildAuthorityNotProcessWait {
			return kind
		}
		return analyzer.processWaitGenericProvenanceKind(value.X, before, visiting)
	default:
		return buildAuthorityNotProcessWait
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitGenericTypeArgumentKind(
	arguments []ast.Expr,
) buildAuthorityProcessWaitKind {
	kind := buildAuthorityNotProcessWait
	for _, argument := range arguments {
		kind = mergeBuildAuthorityProcessWaitKinds(kind,
			analyzer.processWaitSensitiveTypeKind(argument,
				make(map[*buildAuthorityLexicalObject]struct{})))
	}
	return kind
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitSensitiveTypeKind(
	expression ast.Expr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	if kind := analyzer.processWaitTypeKind(expression); kind != buildAuthorityNotProcessWait {
		return kind
	}
	expression = buildAuthorityUnwrapExpression(expression)
	switch value := expression.(type) {
	case *ast.StarExpr:
		return analyzer.processWaitSensitiveTypeKind(value.X, visiting)
	case *ast.ArrayType:
		return analyzer.processWaitSensitiveTypeKind(value.Elt, visiting)
	case *ast.MapType:
		return mergeBuildAuthorityProcessWaitKinds(
			analyzer.processWaitSensitiveTypeKind(value.Key, visiting),
			analyzer.processWaitSensitiveTypeKind(value.Value, visiting),
		)
	case *ast.ChanType:
		return analyzer.processWaitSensitiveTypeKind(value.Value, visiting)
	case *ast.Ellipsis:
		return analyzer.processWaitSensitiveTypeKind(value.Elt, visiting)
	case *ast.IndexExpr:
		return analyzer.processWaitSensitiveTypeKind(value.Index, visiting)
	case *ast.IndexListExpr:
		kind := buildAuthorityNotProcessWait
		for _, argument := range value.Indices {
			kind = mergeBuildAuthorityProcessWaitKinds(kind,
				analyzer.processWaitSensitiveTypeKind(argument, visiting))
		}
		return kind
	case *ast.UnaryExpr:
		return analyzer.processWaitSensitiveTypeKind(value.X, visiting)
	case *ast.BinaryExpr:
		return mergeBuildAuthorityProcessWaitKinds(
			analyzer.processWaitSensitiveTypeKind(value.X, visiting),
			analyzer.processWaitSensitiveTypeKind(value.Y, visiting),
		)
	case *ast.Ident:
		if value.Obj == nil {
			return buildAuthorityNotProcessWait
		}
		if _, found := visiting[value.Obj]; found {
			return buildAuthorityNotProcessWait
		}
		declaration, ok := value.Obj.Decl.(*ast.TypeSpec)
		if !ok {
			return buildAuthorityNotProcessWait
		}
		visiting[value.Obj] = struct{}{}
		defer delete(visiting, value.Obj)
		return analyzer.processWaitSensitiveTypeKind(declaration.Type, visiting)
	default:
		return buildAuthorityNotProcessWait
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitGenericObjectKind(
	object *buildAuthorityLexicalObject,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	if object == nil {
		return buildAuthorityNotProcessWait
	}
	if _, found := visiting[object]; found {
		return buildAuthorityNotProcessWait
	}
	visiting[object] = struct{}{}
	defer delete(visiting, object)

	kind := buildAuthorityNotProcessWait
	if bound, ok := analyzer.bindings.value(object); ok {
		kind = mergeBuildAuthorityProcessWaitKinds(
			kind, analyzer.processWaitGenericProvenanceKind(bound, before, visiting),
		)
	}
	kind = mergeBuildAuthorityProcessWaitKinds(
		kind, analyzer.processWaitGenericDeclarationKind(object, before, visiting),
	)
	ast.Inspect(analyzer.file, func(node ast.Node) bool {
		if node == nil {
			return false
		}
		if node.Pos() >= before {
			return false
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for index, target := range assignment.Lhs {
			identifier, ok := buildAuthorityUnwrapExpression(target).(*ast.Ident)
			if !ok || identifier.Obj != object {
				continue
			}
			if len(assignment.Lhs) == len(assignment.Rhs) {
				kind = mergeBuildAuthorityProcessWaitKinds(kind,
					analyzer.processWaitGenericProvenanceKind(assignment.Rhs[index], before, visiting))
				continue
			}
			for _, source := range assignment.Rhs {
				kind = mergeBuildAuthorityProcessWaitKinds(kind,
					analyzer.processWaitGenericProvenanceKind(source, before, visiting))
			}
		}
		return true
	})
	return kind
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitGenericDeclarationKind(
	object *buildAuthorityLexicalObject,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	switch declaration := object.Decl.(type) {
	case *ast.Field:
		return analyzer.processWaitGenericProvenanceKind(declaration.Type, before, visiting)
	case *ast.ValueSpec:
		kind := analyzer.processWaitGenericProvenanceKind(declaration.Type, before, visiting)
		for index, name := range declaration.Names {
			if name.Obj == object && index < len(declaration.Values) {
				return mergeBuildAuthorityProcessWaitKinds(kind,
					analyzer.processWaitGenericProvenanceKind(declaration.Values[index], before, visiting))
			}
		}
		return kind
	case *ast.AssignStmt:
		for index, target := range declaration.Lhs {
			identifier, ok := buildAuthorityUnwrapExpression(target).(*ast.Ident)
			if ok && identifier.Obj == object && index < len(declaration.Rhs) {
				return analyzer.processWaitGenericProvenanceKind(declaration.Rhs[index], before, visiting)
			}
		}
	case *ast.TypeSpec:
		if declaration.Assign != token.NoPos {
			return analyzer.processWaitGenericProvenanceKind(declaration.Type, before, visiting)
		}
	}
	return buildAuthorityNotProcessWait
}

func mergeBuildAuthorityProcessWaitKinds(
	first buildAuthorityProcessWaitKind,
	second buildAuthorityProcessWaitKind,
) buildAuthorityProcessWaitKind {
	if first == buildAuthorityNotProcessWait {
		return second
	}
	if second == buildAuthorityNotProcessWait || first == second {
		return first
	}
	if first == buildAuthorityForeignProcessWait || second == buildAuthorityForeignProcessWait {
		return buildAuthorityForeignProcessWait
	}
	if first == buildAuthorityForgedCommandWait || second == buildAuthorityForgedCommandWait {
		return buildAuthorityForgedCommandWait
	}
	return buildAuthorityOpaqueProcessWait
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitTypeKind(
	expression ast.Expr,
) buildAuthorityProcessWaitKind {
	return analyzer.processWaitTypeKindSeen(
		expression, make(map[*buildAuthorityLexicalObject]struct{}),
	)
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitTypeKindSeen(
	expression ast.Expr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	expression = buildAuthorityUnwrapExpression(expression)
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = buildAuthorityUnwrapExpression(pointer.X)
	}
	if identifier, ok := expression.(*ast.Ident); ok && identifier.Obj != nil {
		declaration, typeDeclaration := identifier.Obj.Decl.(*ast.TypeSpec)
		if !typeDeclaration || declaration.Assign == token.NoPos {
			return buildAuthorityNotProcessWait
		}
		if _, found := visiting[identifier.Obj]; found {
			return buildAuthorityNotProcessWait
		}
		visiting[identifier.Obj] = struct{}{}
		kind := analyzer.processWaitTypeKindSeen(declaration.Type, visiting)
		delete(visiting, identifier.Obj)
		return kind
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return buildAuthorityNotProcessWait
	}
	importPath, name, imported := analyzer.importedSelector(selector)
	if !imported {
		return buildAuthorityNotProcessWait
	}
	switch {
	case importPath == "os" && name == "Process":
		return buildAuthorityForeignProcessWait
	case importPath == "os/exec" && name == "Cmd":
		return buildAuthorityForgedCommandWait
	default:
		return buildAuthorityNotProcessWait
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitObjectKind(
	object *buildAuthorityLexicalObject,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	if object == nil {
		return buildAuthorityNotProcessWait
	}
	if _, found := visiting[object]; found {
		return buildAuthorityOpaqueProcessWait
	}
	visiting[object] = struct{}{}
	defer delete(visiting, object)
	kind := analyzer.processWaitObjectInitialKind(object, before, visiting)
	if kind != buildAuthorityNotProcessWait &&
		analyzer.objectReassignedBefore(object, before) {
		return buildAuthorityOpaqueProcessWait
	}
	if (kind == buildAuthorityOwnedCommandWait || kind == buildAuthorityUnknownCommandWait) &&
		analyzer.commandOrProcessAssignedBefore(object, before) {
		return buildAuthorityOpaqueProcessWait
	}
	return kind
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitObjectInitialKind(
	object *buildAuthorityLexicalObject,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	switch declaration := object.Decl.(type) {
	case *ast.AssignStmt:
		return analyzer.processWaitAssignmentInitialKind(
			object, declaration, before, visiting,
		)
	case *ast.ValueSpec:
		return analyzer.processWaitValueInitialKind(object, declaration, before, visiting)
	case *ast.Field:
		return analyzer.processWaitDeclaredTypeKind(declaration.Type)
	default:
		return buildAuthorityNotProcessWait
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitAssignmentInitialKind(
	object *buildAuthorityLexicalObject,
	declaration *ast.AssignStmt,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	for index, target := range declaration.Lhs {
		identifier, ok := buildAuthorityUnwrapExpression(target).(*ast.Ident)
		if !ok || identifier.Obj != object {
			continue
		}
		if len(declaration.Lhs) == len(declaration.Rhs) {
			return analyzer.processWaitKind(declaration.Rhs[index], before, visiting)
		}
		if index == 0 && len(declaration.Rhs) == 1 {
			return analyzer.processWaitConstructorKindFromFirstResult(declaration.Rhs[0])
		}
	}
	return buildAuthorityNotProcessWait
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitValueInitialKind(
	object *buildAuthorityLexicalObject,
	declaration *ast.ValueSpec,
	before token.Pos,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthorityProcessWaitKind {
	for index, name := range declaration.Names {
		if name.Obj != object {
			continue
		}
		if index < len(declaration.Values) {
			kind := analyzer.processWaitKind(declaration.Values[index], before, visiting)
			if kind != buildAuthorityNotProcessWait {
				return kind
			}
		}
		return analyzer.processWaitDeclaredTypeKind(declaration.Type)
	}
	return buildAuthorityNotProcessWait
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitDeclaredTypeKind(
	expression ast.Expr,
) buildAuthorityProcessWaitKind {
	kind := analyzer.processWaitTypeKind(expression)
	if kind == buildAuthorityForgedCommandWait {
		return buildAuthorityUnknownCommandWait
	}
	return kind
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) processWaitConstructorKindFromFirstResult(
	expression ast.Expr,
) buildAuthorityProcessWaitKind {
	call, ok := buildAuthorityUnwrapExpression(expression).(*ast.CallExpr)
	if !ok {
		return buildAuthorityNotProcessWait
	}
	return analyzer.processWaitConstructorKind(call)
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) objectReassignedBefore(
	object *buildAuthorityLexicalObject,
	before token.Pos,
) bool {
	reassigned := false
	ast.Inspect(analyzer.file, func(node ast.Node) bool {
		if node == nil || reassigned {
			return !reassigned
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Pos() >= before {
			return true
		}
		for _, target := range assignment.Lhs {
			identifier, ok := buildAuthorityUnwrapExpression(target).(*ast.Ident)
			if !ok || identifier.Obj != object {
				continue
			}
			if object.Decl == assignment && assignment.Tok == token.DEFINE {
				continue
			}
			reassigned = true
			return false
		}
		return true
	})
	return reassigned
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) commandOrProcessAssignedBefore(
	command *buildAuthorityLexicalObject,
	before token.Pos,
) bool {
	assigned := false
	ast.Inspect(analyzer.file, func(node ast.Node) bool {
		if node == nil || assigned {
			return !assigned
		}
		if node.Pos() >= before {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			assigned = slices.ContainsFunc(value.Lhs, func(target ast.Expr) bool {
				return buildAuthorityAssignmentTargetsCommandOrProcess(target, command) &&
					!analyzer.identifierDeclaresObject(target, command, value)
			}) || slices.ContainsFunc(value.Rhs, func(source ast.Expr) bool {
				return buildAuthorityExpressionDirectlyAliasesCommandOrProcess(source, command)
			})
		case *ast.ValueSpec:
			assigned = slices.ContainsFunc(value.Values, func(source ast.Expr) bool {
				return buildAuthorityExpressionDirectlyAliasesCommandOrProcess(source, command)
			})
		case *ast.IncDecStmt:
			assigned = buildAuthorityAssignmentTargetsCommandOrProcess(value.X, command)
		case *ast.RangeStmt:
			assigned = buildAuthorityAssignmentTargetsCommandOrProcess(value.Key, command) ||
				buildAuthorityAssignmentTargetsCommandOrProcess(value.Value, command)
		}
		return !assigned
	})
	return assigned
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyReviewedFunctionUse(identifier *ast.Ident) {
	if analyzer.identifierIsDeclaration(identifier) || analyzer.identifierIsSelectorName(identifier) {
		return
	}
	if identifier.Obj != nil && identifier.Obj.Kind != ast.Fun {
		return
	}
	if _, direct := analyzer.directCallees[identifier]; direct {
		return
	}
	if buildAuthorityReviewedWaitSeam(analyzer.rel, analyzer.file.Name.Name, identifier.Name) {
		analyzer.addViolation(identifier,
			fmt.Sprintf("uses reviewed exact-PID wait seam %s as a function value", identifier.Name))
		return
	}
	if buildAuthorityReviewedSignalSeam(analyzer.rel, analyzer.file.Name.Name, identifier.Name) {
		analyzer.addViolation(identifier,
			fmt.Sprintf("uses reviewed signal-forwarding seam %s as a function value", identifier.Name))
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyKernelCall(call *ast.CallExpr, name string) {
	switch name {
	case "Sigaction":
		analyzer.addViolation(call, "uses sigaction outside internal/buildauthority")
	case "Wait", "Wait3", "Wait4", "Waitpid":
		analyzer.addViolation(call,
			"uses a child-reaping API outside an explicitly reviewed exact-PID wait seam")
	case "Waitid":
		analyzer.classifyReviewedWaitid(call, call.Args)
	default:
		if buildAuthorityRawSyscallName(name) {
			analyzer.classifyRawSyscall(call)
		}
	}
}

func buildAuthorityRawSyscallName(name string) bool {
	if name == "AllThreadsSyscall" || name == "AllThreadsSyscall6" {
		return true
	}
	for _, prefix := range []string{"RawSyscall", "Syscall"} {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(name, prefix)
		if suffix == "" || suffix == "N" || suffix == "NoError" {
			return true
		}
		allDigits := true
		for _, char := range suffix {
			if char < '0' || char > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return true
		}
	}
	return false
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyRawSyscall(call *ast.CallExpr) {
	if len(call.Args) == 0 {
		analyzer.addViolation(call, "uses a raw syscall without a statically audited symbolic trap")
		return
	}
	_, trap, ok := analyzer.resolveImportedSymbol(call.Args[0], make(map[*buildAuthorityLexicalObject]struct{}))
	if !ok || !strings.HasPrefix(trap, "SYS_") {
		analyzer.addViolation(call,
			"uses a numeric or dynamic raw syscall trap that cannot be audited for wait or sigaction behavior")
		return
	}
	if buildAuthoritySignalDispositionTrap(trap) {
		analyzer.addViolation(call, "uses a raw signal-disposition syscall outside internal/buildauthority")
		return
	}
	if !buildAuthorityWaitTrap(trap) {
		return
	}
	if !strings.Contains(trap, "WAITID") {
		analyzer.addViolation(call,
			"uses a raw child-reaping syscall outside an explicitly reviewed P_PID waitid seam")
		return
	}
	if len(call.Args) < 3 {
		analyzer.addViolation(call, "uses raw waitid without both P_PID and an exact reviewed target")
		return
	}
	analyzer.classifyReviewedWaitid(call, call.Args[1:])
}

func buildAuthoritySignalDispositionTrap(trap string) bool {
	return strings.Contains(trap, "SIGACTION") || trap == "SYS_SIGNAL"
}

func buildAuthorityWaitTrap(trap string) bool {
	return strings.HasPrefix(strings.TrimPrefix(trap, "SYS_"), "WAIT")
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyReviewedWaitid(
	call *ast.CallExpr,
	args []ast.Expr,
) {
	declaration, ok := analyzer.reviewedWaitImplementation(call)
	if !ok {
		analyzer.addViolation(call,
			"uses waitid outside an explicitly reviewed exact-PID wait seam")
		return
	}
	if len(args) < 2 {
		analyzer.addViolation(call,
			"reviewed waitid seam must use both P_PID and its exact pid parameter")
		return
	}
	pid, exactPID := analyzer.waitTargetPIDParameter(args[1], declaration)
	if !analyzer.waitIDTypeIsPID(args[0]) || !exactPID {
		analyzer.addViolation(call,
			"reviewed waitid seam must use both P_PID and its exact pid parameter")
		return
	}
	if !buildAuthorityPIDParameterStableBefore(declaration, pid, call.Pos()) {
		analyzer.addViolation(call,
			"reviewed waitid seam reassigns or exposes its pid parameter before use")
		return
	}
	analyzer.result.exactPIDWaits++
	analyzer.result.exactPIDSites = append(analyzer.result.exactPIDSites,
		analyzer.rel+":"+declaration.Name.Name)
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) reviewedWaitImplementation(
	node ast.Node,
) (*ast.FuncDecl, bool) {
	declaration, ok := analyzer.enclosingFunction(node)
	if !ok || declaration.Recv != nil {
		return nil, false
	}
	if !buildAuthorityReviewedWaitImplementation(
		analyzer.rel, analyzer.file.Name.Name, declaration.Name.Name,
	) {
		return nil, false
	}
	return declaration, true
}

func buildAuthorityReviewedWaitImplementation(rel, packageName, functionName string) bool {
	switch functionName {
	case "waitForGitCommandExitWithoutReaping":
		return packageName == "specguard" &&
			(rel == "internal/specguard/process_wait_darwin.go" ||
				rel == "internal/specguard/process_wait_linux.go")
	case "waitForSpecAuditCommandExitWithoutReaping":
		return packageName == "steps" &&
			(rel == "agm/test/bdd/steps/spec_governance_process_wait_darwin.go" ||
				rel == "agm/test/bdd/steps/spec_governance_process_wait_linux.go")
	default:
		return false
	}
}

func buildAuthorityReviewedWaitSeam(rel, packageName, functionName string) bool {
	directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	switch functionName {
	case "waitForGitCommandExitWithoutReaping":
		return directory == "internal/specguard" && packageName == "specguard"
	case "waitForSpecAuditCommandExitWithoutReaping":
		return directory == "agm/test/bdd/steps" && packageName == "steps"
	default:
		return false
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyReviewedWaitSeamCall(
	call *ast.CallExpr,
	functionName string,
) {
	declaration, ok := analyzer.enclosingFunction(call)
	if !ok || !buildAuthorityReviewedWaitCallsite(
		analyzer.rel, analyzer.file.Name.Name, declaration.Name.Name, functionName,
	) {
		analyzer.addViolation(call,
			fmt.Sprintf("reviewed exact-PID wait seam %s is called outside its audited production callsite",
				functionName))
		return
	}
	if len(call.Args) != 1 {
		analyzer.addViolation(call,
			fmt.Sprintf("reviewed exact-PID wait seam %s must receive an *exec.Cmd parameter's Process.Pid",
				functionName))
		return
	}
	command, ok := analyzer.expressionIsExecCmdProcessPID(call.Args[0], call)
	if !ok {
		analyzer.addViolation(call,
			fmt.Sprintf("reviewed exact-PID wait seam %s must receive an *exec.Cmd parameter's Process.Pid",
				functionName))
		return
	}
	if buildAuthorityCommandOrProcessAssignedBefore(declaration, command, call.Pos()) {
		analyzer.addViolation(call,
			fmt.Sprintf("reviewed exact-PID wait seam %s receives a reassigned command or Process",
				functionName))
	}
	if analyzer.commandHasOpaqueUseBefore(declaration, command, call.Pos()) {
		analyzer.addViolation(call,
			fmt.Sprintf("reviewed exact-PID wait seam %s follows an opaque use of its command, Process, or PID",
				functionName))
	}
}

func buildAuthorityReviewedWaitCallsite(
	rel string,
	packageName string,
	callerName string,
	seamName string,
) bool {
	switch seamName {
	case "waitForGitCommandExitWithoutReaping":
		return rel == "internal/specguard/git_exec.go" && packageName == "specguard" &&
			callerName == "runGitCommand"
	case "waitForSpecAuditCommandExitWithoutReaping":
		return rel == "agm/test/bdd/steps/spec_governance_tooling_steps.go" && packageName == "steps" &&
			callerName == "runBoundedSpecAuditCommand"
	default:
		return false
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) expressionIsExecCmdProcessPID(
	expression ast.Expr,
	node ast.Node,
) (*buildAuthorityLexicalObject, bool) {
	pidSelector, ok := buildAuthorityUnwrapExpression(expression).(*ast.SelectorExpr)
	if !ok || pidSelector.Sel.Name != "Pid" {
		return nil, false
	}
	processSelector, ok := buildAuthorityUnwrapExpression(pidSelector.X).(*ast.SelectorExpr)
	if !ok || processSelector.Sel.Name != "Process" {
		return nil, false
	}
	command, ok := buildAuthorityUnwrapExpression(processSelector.X).(*ast.Ident)
	if !ok || command.Obj == nil {
		return nil, false
	}
	declaration, ok := analyzer.enclosingFunction(node)
	if !ok || declaration.Type.Params == nil {
		return nil, false
	}
	for _, field := range declaration.Type.Params.List {
		if !analyzer.typeIsExecCmdPointer(field.Type) {
			continue
		}
		for _, name := range field.Names {
			if name.Obj != nil && name.Obj == command.Obj {
				return command.Obj, true
			}
		}
	}
	return nil, false
}

func buildAuthorityCommandOrProcessAssignedBefore(
	declaration *ast.FuncDecl,
	command *buildAuthorityLexicalObject,
	before token.Pos,
) bool {
	mutated := false
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if node == nil || mutated || node.Pos() >= before {
			return !mutated
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, target := range value.Lhs {
				if buildAuthorityAssignmentTargetsCommandOrProcess(target, command) {
					mutated = true
					return false
				}
			}
			for _, source := range value.Rhs {
				if buildAuthorityExpressionDirectlyAliasesCommandOrProcess(source, command) {
					mutated = true
					return false
				}
			}
		case *ast.ValueSpec:
			for _, source := range value.Values {
				if buildAuthorityExpressionDirectlyAliasesCommandOrProcess(source, command) {
					mutated = true
					return false
				}
			}
		case *ast.IncDecStmt:
			mutated = buildAuthorityAssignmentTargetsCommandOrProcess(value.X, command)
		case *ast.RangeStmt:
			mutated = buildAuthorityAssignmentTargetsCommandOrProcess(value.Key, command) ||
				buildAuthorityAssignmentTargetsCommandOrProcess(value.Value, command)
		}
		return !mutated
	})
	return mutated
}

type buildAuthorityCommandAliasKind uint8

const (
	buildAuthorityCommandNotAliased buildAuthorityCommandAliasKind = iota
	buildAuthorityCommandDerivedAlias
	buildAuthorityGitLifecycleAlias
	buildAuthoritySpecAuditLifecycleAlias
)

type buildAuthorityCommandUseTracker struct {
	analyzer    *buildAuthorityWaitOwnershipAnalyzer
	declaration *ast.FuncDecl
	command     *buildAuthorityLexicalObject
	aliases     map[*buildAuthorityLexicalObject]buildAuthorityCommandAliasKind
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) commandHasOpaqueUseBefore(
	declaration *ast.FuncDecl,
	command *buildAuthorityLexicalObject,
	before token.Pos,
) bool {
	tracker := buildAuthorityCommandUseTracker{
		analyzer:    analyzer,
		declaration: declaration,
		command:     command,
		aliases: map[*buildAuthorityLexicalObject]buildAuthorityCommandAliasKind{
			command: buildAuthorityCommandDerivedAlias,
		},
	}
	unsafe := false
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if node == nil || unsafe {
			return !unsafe
		}
		if node.Pos() >= before {
			return false
		}
		unsafe = tracker.inspectNode(node)
		return !unsafe
	})
	return unsafe
}

func (tracker *buildAuthorityCommandUseTracker) inspectNode(node ast.Node) bool {
	switch value := node.(type) {
	case *ast.AssignStmt:
		return tracker.recordAssignment(value)
	case *ast.ValueSpec:
		return tracker.recordValueSpec(value)
	case *ast.RangeStmt:
		return tracker.recordRange(value)
	case *ast.SendStmt:
		return tracker.expressionContainsAlias(value.Chan) ||
			tracker.expressionContainsAlias(value.Value)
	case *ast.CallExpr:
		return tracker.callTouchesAlias(value) && !tracker.reviewedCall(value)
	default:
		return false
	}
}

func (tracker *buildAuthorityCommandUseTracker) recordAssignment(assignment *ast.AssignStmt) bool {
	if tracker.reviewedCallbackAssignment(assignment) {
		return false
	}
	for _, target := range assignment.Lhs {
		if tracker.expressionContainsAlias(target) && !tracker.reviewedCommandFieldAssignment(target) {
			return true
		}
	}
	if len(assignment.Lhs) == len(assignment.Rhs) {
		for index, target := range assignment.Lhs {
			if tracker.bindTarget(target, tracker.assignmentKind(assignment.Rhs[index])) {
				return true
			}
		}
		return false
	}
	kind := buildAuthorityCommandNotAliased
	for _, expression := range assignment.Rhs {
		if tracker.assignmentKind(expression) != buildAuthorityCommandNotAliased {
			kind = buildAuthorityCommandDerivedAlias
		}
	}
	for _, target := range assignment.Lhs {
		if tracker.bindTarget(target, kind) {
			return true
		}
	}
	return false
}

func (tracker *buildAuthorityCommandUseTracker) recordValueSpec(spec *ast.ValueSpec) bool {
	if len(spec.Names) == len(spec.Values) {
		for index, name := range spec.Names {
			if tracker.bindTarget(name, tracker.assignmentKind(spec.Values[index])) {
				return true
			}
		}
		return false
	}
	kind := buildAuthorityCommandNotAliased
	for _, expression := range spec.Values {
		if tracker.assignmentKind(expression) != buildAuthorityCommandNotAliased {
			kind = buildAuthorityCommandDerivedAlias
		}
	}
	for _, name := range spec.Names {
		if tracker.bindTarget(name, kind) {
			return true
		}
	}
	return false
}

func (tracker *buildAuthorityCommandUseTracker) recordRange(statement *ast.RangeStmt) bool {
	if !tracker.expressionContainsAlias(statement.X) {
		return false
	}
	return tracker.bindTarget(statement.Key, buildAuthorityCommandDerivedAlias) ||
		tracker.bindTarget(statement.Value, buildAuthorityCommandDerivedAlias)
}

func (tracker *buildAuthorityCommandUseTracker) bindTarget(
	target ast.Expr,
	kind buildAuthorityCommandAliasKind,
) bool {
	if target == nil || kind == buildAuthorityCommandNotAliased {
		return false
	}
	identifier, ok := buildAuthorityUnwrapExpression(target).(*ast.Ident)
	if !ok || identifier.Obj == nil || !tracker.objectIsLocal(identifier.Obj) {
		return true
	}
	if kind == buildAuthorityGitLifecycleAlias || kind == buildAuthoritySpecAuditLifecycleAlias {
		tracker.aliases[identifier.Obj] = kind
	} else {
		tracker.aliases[identifier.Obj] = buildAuthorityCommandDerivedAlias
	}
	return false
}

func (tracker *buildAuthorityCommandUseTracker) objectIsLocal(
	object *buildAuthorityLexicalObject,
) bool {
	declaration, ok := object.Decl.(ast.Node)
	return ok && declaration.Pos() >= tracker.declaration.Body.Pos() &&
		declaration.End() <= tracker.declaration.Body.End()
}

func (tracker *buildAuthorityCommandUseTracker) assignmentKind(
	expression ast.Expr,
) buildAuthorityCommandAliasKind {
	call, callExpression := buildAuthorityUnwrapExpression(expression).(*ast.CallExpr)
	if callExpression {
		if kind := tracker.reviewedConstructorKind(call); kind != buildAuthorityCommandNotAliased {
			return kind
		}
		if tracker.reviewedCall(call) {
			return buildAuthorityCommandNotAliased
		}
	}
	if tracker.expressionContainsAlias(expression) {
		return buildAuthorityCommandDerivedAlias
	}
	return buildAuthorityCommandNotAliased
}

func (tracker *buildAuthorityCommandUseTracker) expressionContainsAlias(expression ast.Expr) bool {
	if expression == nil {
		return false
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Obj != nil &&
			tracker.aliases[identifier.Obj] != buildAuthorityCommandNotAliased {
			found = true
			return false
		}
		return !found
	})
	return found
}

func (tracker *buildAuthorityCommandUseTracker) callTouchesAlias(call *ast.CallExpr) bool {
	if slices.ContainsFunc(call.Args, tracker.expressionContainsAlias) {
		return true
	}
	function := buildAuthorityUnwrapExpression(call.Fun)
	switch value := function.(type) {
	case *ast.SelectorExpr:
		return tracker.expressionContainsAlias(value.X)
	case *ast.Ident:
		return value.Obj != nil &&
			tracker.aliases[value.Obj] != buildAuthorityCommandNotAliased
	case *ast.FuncLit:
		return false
	default:
		return tracker.expressionContainsAlias(function)
	}
}

func (tracker *buildAuthorityCommandUseTracker) reviewedCall(call *ast.CallExpr) bool {
	if tracker.reviewedConstructorKind(call) != buildAuthorityCommandNotAliased {
		return true
	}
	function := buildAuthorityUnwrapExpression(call.Fun)
	selector, ok := function.(*ast.SelectorExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	receiver, ok := buildAuthorityUnwrapExpression(selector.X).(*ast.Ident)
	if !ok || receiver.Obj == nil {
		return false
	}
	if receiver.Obj == tracker.command {
		return selector.Sel.Name == "Start"
	}
	kind := tracker.aliases[receiver.Obj]
	if kind == buildAuthorityGitLifecycleAlias {
		return tracker.analyzer.rel == "internal/specguard/git_exec.go" &&
			tracker.analyzer.file.Name.Name == "specguard" &&
			(selector.Sel.Name == "disable" || selector.Sel.Name == "cancelObserved")
	}
	if kind == buildAuthoritySpecAuditLifecycleAlias {
		return tracker.analyzer.rel == "agm/test/bdd/steps/spec_governance_tooling_steps.go" &&
			tracker.analyzer.file.Name.Name == "steps" &&
			(selector.Sel.Name == "disable" || selector.Sel.Name == "cancel")
	}
	return false
}

func (tracker *buildAuthorityCommandUseTracker) reviewedConstructorKind(
	call *ast.CallExpr,
) buildAuthorityCommandAliasKind {
	if len(call.Args) != 1 {
		return buildAuthorityCommandNotAliased
	}
	argument, ok := buildAuthorityUnwrapExpression(call.Args[0]).(*ast.Ident)
	if !ok || argument.Obj != tracker.command {
		return buildAuthorityCommandNotAliased
	}
	functionName, ok := buildAuthorityResolvedFunctionName(call.Fun)
	if !ok {
		return buildAuthorityCommandNotAliased
	}
	if functionName == "newGitProcessGroupLifecycle" &&
		tracker.analyzer.rel == "internal/specguard/git_exec.go" &&
		tracker.analyzer.file.Name.Name == "specguard" {
		return buildAuthorityGitLifecycleAlias
	}
	if functionName == "newSpecAuditProcessGroupLifecycle" &&
		tracker.analyzer.rel == "agm/test/bdd/steps/spec_governance_tooling_steps.go" &&
		tracker.analyzer.file.Name.Name == "steps" {
		return buildAuthoritySpecAuditLifecycleAlias
	}
	return buildAuthorityCommandNotAliased
}

func buildAuthorityResolvedFunctionName(expression ast.Expr) (string, bool) {
	function, ok := buildAuthorityUnwrapExpression(expression).(*ast.Ident)
	if !ok || function.Obj == nil || function.Obj.Kind != ast.Fun {
		return "", false
	}
	declaration, ok := function.Obj.Decl.(*ast.FuncDecl)
	if !ok || declaration.Recv != nil || declaration.Name.Name != function.Name {
		return "", false
	}
	return function.Name, true
}

func (tracker *buildAuthorityCommandUseTracker) reviewedCallbackAssignment(
	assignment *ast.AssignStmt,
) bool {
	if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return false
	}
	left, ok := buildAuthorityUnwrapExpression(assignment.Lhs[0]).(*ast.SelectorExpr)
	if !ok || left.Sel.Name != "onLimit" {
		return false
	}
	root, ok := buildAuthorityUnwrapExpression(left.X).(*ast.Ident)
	if !ok {
		return false
	}
	if tracker.analyzer.rel == "internal/specguard/git_exec.go" && root.Name == "capture" {
		right, ok := buildAuthorityUnwrapExpression(assignment.Rhs[0]).(*ast.SelectorExpr)
		if !ok || right.Sel.Name != "cancelForOutputLimit" {
			return false
		}
		receiver, ok := buildAuthorityUnwrapExpression(right.X).(*ast.Ident)
		return ok && receiver.Obj != nil &&
			tracker.aliases[receiver.Obj] == buildAuthorityGitLifecycleAlias
	}
	if tracker.analyzer.rel == "agm/test/bdd/steps/spec_governance_tooling_steps.go" &&
		root.Name == "output" {
		_, ok := buildAuthorityUnwrapExpression(assignment.Rhs[0]).(*ast.FuncLit)
		return ok
	}
	return false
}

func (tracker *buildAuthorityCommandUseTracker) reviewedCommandFieldAssignment(target ast.Expr) bool {
	if tracker.analyzer.rel != "agm/test/bdd/steps/spec_governance_tooling_steps.go" {
		return false
	}
	selector, ok := buildAuthorityUnwrapExpression(target).(*ast.SelectorExpr)
	if !ok || (selector.Sel.Name != "Stdout" && selector.Sel.Name != "Stderr") {
		return false
	}
	receiver, ok := buildAuthorityUnwrapExpression(selector.X).(*ast.Ident)
	return ok && receiver.Obj == tracker.command
}

func buildAuthorityAssignmentTargetsCommandOrProcess(
	expression ast.Expr,
	command *buildAuthorityLexicalObject,
) bool {
	if expression == nil {
		return false
	}
	expression = buildAuthorityUnwrapExpression(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		return identifier.Obj != nil && identifier.Obj == command
	}
	if pointer, ok := expression.(*ast.StarExpr); ok {
		return buildAuthorityExpressionContainsObject(pointer.X, command)
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return buildAuthorityExpressionContainsObject(expression, command)
	}
	fields := []string{selector.Sel.Name}
	root := buildAuthorityUnwrapExpression(selector.X)
	for {
		next, ok := root.(*ast.SelectorExpr)
		if !ok {
			break
		}
		fields = append(fields, next.Sel.Name)
		root = buildAuthorityUnwrapExpression(next.X)
	}
	if pointer, ok := root.(*ast.StarExpr); ok {
		root = buildAuthorityUnwrapExpression(pointer.X)
	}
	identifier, ok := root.(*ast.Ident)
	if !ok {
		return buildAuthorityExpressionContainsObject(expression, command)
	}
	return identifier.Obj != nil && identifier.Obj == command && len(fields) != 0 &&
		fields[len(fields)-1] == "Process"
}

func buildAuthorityExpressionContainsObject(
	expression ast.Expr,
	object *buildAuthorityLexicalObject,
) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Obj != nil && identifier.Obj == object {
			found = true
			return false
		}
		return !found
	})
	return found
}

func buildAuthorityExpressionDirectlyAliasesCommandOrProcess(
	expression ast.Expr,
	command *buildAuthorityLexicalObject,
) bool {
	expression = buildAuthorityUnwrapExpression(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		return identifier.Obj != nil && identifier.Obj == command
	}
	if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		return buildAuthorityExpressionContainsObject(unary.X, command)
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Process" {
		return false
	}
	identifier, ok := buildAuthorityUnwrapExpression(selector.X).(*ast.Ident)
	return ok && identifier.Obj != nil && identifier.Obj == command
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) typeIsExecCmdPointer(expression ast.Expr) bool {
	pointer, ok := buildAuthorityUnwrapExpression(expression).(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := buildAuthorityUnwrapExpression(pointer.X).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Cmd" {
		return false
	}
	importPath, _, ok := analyzer.importedSelector(selector)
	return ok && importPath == "os/exec"
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) waitTargetPIDParameter(
	expression ast.Expr,
	declaration *ast.FuncDecl,
) (*buildAuthorityLexicalObject, bool) {
	expression = analyzer.unwrapPIDConversion(expression)
	identifier, ok := expression.(*ast.Ident)
	if !ok || identifier.Obj == nil || declaration.Type.Params == nil {
		return nil, false
	}
	for _, field := range declaration.Type.Params.List {
		fieldType, ok := buildAuthorityUnwrapExpression(field.Type).(*ast.Ident)
		if !ok || !analyzer.isPredeclaredIdentifier(fieldType, "int") {
			continue
		}
		for _, name := range field.Names {
			if name.Name == "pid" && name.Obj != nil && name.Obj == identifier.Obj {
				return identifier.Obj, true
			}
		}
	}
	return nil, false
}

func buildAuthorityPIDParameterStableBefore(
	declaration *ast.FuncDecl,
	pid *buildAuthorityLexicalObject,
	before token.Pos,
) bool {
	stable := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if node == nil || !stable {
			return stable
		}
		if node.Pos() >= before {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, target := range value.Lhs {
				if buildAuthorityExpressionContainsObject(target, pid) {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			stable = !buildAuthorityExpressionContainsObject(value.X, pid)
		case *ast.RangeStmt:
			stable = !buildAuthorityExpressionContainsObject(value.Key, pid) &&
				!buildAuthorityExpressionContainsObject(value.Value, pid)
		case *ast.UnaryExpr:
			if value.Op == token.AND && buildAuthorityExpressionContainsObject(value.X, pid) {
				stable = false
			}
		}
		return stable
	})
	return stable
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) unwrapPIDConversion(expression ast.Expr) ast.Expr {
	for {
		expression = buildAuthorityUnwrapExpression(expression)
		call, ok := expression.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return expression
		}
		identifier, ok := buildAuthorityUnwrapExpression(call.Fun).(*ast.Ident)
		if !ok || !analyzer.isPredeclaredConversion(identifier) {
			return expression
		}
		expression = call.Args[0]
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) isPredeclaredConversion(
	identifier *ast.Ident,
) bool {
	return buildAuthorityPIDConversion(identifier.Name) &&
		analyzer.isPredeclaredIdentifier(identifier, identifier.Name)
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) isPredeclaredIdentifier(
	identifier *ast.Ident,
	want string,
) bool {
	if identifier == nil || identifier.Name != want || identifier.Obj != nil {
		return false
	}
	_, declaredByPackage := analyzer.packageNames[want]
	return !declaredByPackage
}

func buildAuthorityPIDConversion(name string) bool {
	switch name {
	case "int", "int32", "int64", "uint", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) waitIDTypeIsPID(expression ast.Expr) bool {
	_, symbol, ok := analyzer.resolveImportedSymbol(
		expression, make(map[*buildAuthorityLexicalObject]struct{}),
	)
	if ok && symbol == "P_PID" {
		return true
	}
	value, ok := analyzer.integerValue(expression, make(map[*buildAuthorityLexicalObject]struct{}))
	return ok && value == 1
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifySignalCall(
	call *ast.CallExpr,
	name string,
) {
	start := 0
	switch name {
	case "Notify", "NotifyContext":
		start = 1
	case "Ignore", "Reset":
	case "Stop":
		analyzer.classifySignalStop(call)
		return
	default:
		return
	}
	if analyzer.reviewedSignalForwardingImplementation(call, name) {
		if !analyzer.isExactReviewedSignalForward(call) {
			analyzer.addViolation(call,
				"reviewed signal seam must forward only its audited variadic signal parameter")
		}
		return
	}
	if len(call.Args) <= start {
		analyzer.addViolation(call,
			"subscribes to or mutates all signals, including SIGCHLD, outside internal/buildauthority")
		return
	}
	for _, argument := range call.Args[start:] {
		switch analyzer.signalExpression(argument, make(map[*buildAuthorityLexicalObject]struct{})) {
		case buildAuthoritySignalChild:
			analyzer.addViolation(call,
				"mutates or subscribes to SIGCHLD outside internal/buildauthority")
			return
		case buildAuthoritySignalUnknown:
			analyzer.addViolation(call,
				"uses a dynamic or numeric signal that cannot be audited to exclude SIGCHLD")
			return
		case buildAuthoritySignalSafe:
		}
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifySignalStop(call *ast.CallExpr) {
	if len(call.Args) != 1 {
		analyzer.addViolation(call,
			"uses signal.Stop with an opaque channel shape that cannot exclude SIGCHLD")
		return
	}
	channel, ok := buildAuthorityUnwrapExpression(call.Args[0]).(*ast.Ident)
	if !ok || channel.Obj == nil || !analyzer.signalStopChannelIsSafe(channel.Obj, call) {
		analyzer.addViolation(call,
			"uses signal.Stop on a channel not proven to contain only non-SIGCHLD subscriptions")
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) signalStopChannelIsSafe(
	channel *buildAuthorityLexicalObject,
	stop *ast.CallExpr,
) bool {
	if !analyzer.signalChannelIsLocallyConstructed(channel) {
		return false
	}
	notified := false
	safe := true
	ast.Inspect(analyzer.file, func(node ast.Node) bool {
		if node == nil || !safe {
			return safe
		}
		if node.Pos() >= stop.Pos() {
			return false
		}
		switch analyzer.signalStopNodeEffect(node, channel) {
		case buildAuthoritySignalStopUnsafe:
			safe = false
			return false
		case buildAuthoritySignalStopNotified:
			notified = true
		case buildAuthoritySignalStopNoEffect:
		}
		return true
	})
	return notified && safe
}

type buildAuthoritySignalStopEffect uint8

const (
	buildAuthoritySignalStopNoEffect buildAuthoritySignalStopEffect = iota
	buildAuthoritySignalStopNotified
	buildAuthoritySignalStopUnsafe
)

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) signalStopNodeEffect(
	node ast.Node,
	channel *buildAuthorityLexicalObject,
) buildAuthoritySignalStopEffect {
	switch value := node.(type) {
	case *ast.AssignStmt:
		if analyzer.signalChannelAssignmentIsUnsafe(value, channel) {
			return buildAuthoritySignalStopUnsafe
		}
	case *ast.ValueSpec:
		if slices.ContainsFunc(value.Values, func(source ast.Expr) bool {
			return buildAuthorityExpressionAliasesSignalChannel(source, channel)
		}) {
			return buildAuthoritySignalStopUnsafe
		}
	case *ast.SendStmt:
		if buildAuthorityExpressionContainsObject(value.Chan, channel) ||
			buildAuthorityExpressionContainsObject(value.Value, channel) {
			return buildAuthoritySignalStopUnsafe
		}
	case *ast.CallExpr:
		return analyzer.signalStopCallEffect(value, channel)
	}
	return buildAuthoritySignalStopNoEffect
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) signalChannelAssignmentIsUnsafe(
	assignment *ast.AssignStmt,
	channel *buildAuthorityLexicalObject,
) bool {
	targetUnsafe := slices.ContainsFunc(assignment.Lhs, func(target ast.Expr) bool {
		return buildAuthorityExpressionContainsObject(target, channel) &&
			!analyzer.identifierDeclaresObject(target, channel, assignment)
	})
	return targetUnsafe || slices.ContainsFunc(assignment.Rhs, func(source ast.Expr) bool {
		return buildAuthorityExpressionAliasesSignalChannel(source, channel)
	})
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) signalStopCallEffect(
	call *ast.CallExpr,
	channel *buildAuthorityLexicalObject,
) buildAuthoritySignalStopEffect {
	containsChannel := slices.ContainsFunc(call.Args, func(argument ast.Expr) bool {
		return buildAuthorityExpressionContainsObject(argument, channel)
	})
	if !containsChannel {
		return buildAuthoritySignalStopNoEffect
	}
	if analyzer.safeSignalNotifyForChannel(call, channel) {
		return buildAuthoritySignalStopNotified
	}
	return buildAuthoritySignalStopUnsafe
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) signalChannelIsLocallyConstructed(
	channel *buildAuthorityLexicalObject,
) bool {
	var initializer ast.Expr
	switch declaration := channel.Decl.(type) {
	case *ast.AssignStmt:
		if declaration.Tok != token.DEFINE || len(declaration.Lhs) != len(declaration.Rhs) {
			return false
		}
		for index, target := range declaration.Lhs {
			identifier, ok := buildAuthorityUnwrapExpression(target).(*ast.Ident)
			if ok && identifier.Obj == channel {
				initializer = declaration.Rhs[index]
				break
			}
		}
	case *ast.ValueSpec:
		if len(declaration.Names) != len(declaration.Values) {
			return false
		}
		for index, name := range declaration.Names {
			if name.Obj == channel {
				initializer = declaration.Values[index]
				break
			}
		}
	}
	call, ok := buildAuthorityUnwrapExpression(initializer).(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	function, ok := buildAuthorityUnwrapExpression(call.Fun).(*ast.Ident)
	if !ok || !analyzer.isPredeclaredIdentifier(function, "make") {
		return false
	}
	_, ok = buildAuthorityUnwrapExpression(call.Args[0]).(*ast.ChanType)
	return ok
}

func buildAuthorityExpressionAliasesSignalChannel(
	expression ast.Expr,
	channel *buildAuthorityLexicalObject,
) bool {
	expression = buildAuthorityUnwrapExpression(expression)
	if receive, ok := expression.(*ast.UnaryExpr); ok && receive.Op == token.ARROW {
		return false
	}
	return buildAuthorityExpressionContainsObject(expression, channel)
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) identifierDeclaresObject(
	expression ast.Expr,
	object *buildAuthorityLexicalObject,
	assignment *ast.AssignStmt,
) bool {
	identifier, ok := buildAuthorityUnwrapExpression(expression).(*ast.Ident)
	return ok && identifier.Obj == object && object.Decl == assignment &&
		assignment.Tok == token.DEFINE
}

func buildAuthorityExpressionIsObject(
	expression ast.Expr,
	object *buildAuthorityLexicalObject,
) bool {
	identifier, ok := buildAuthorityUnwrapExpression(expression).(*ast.Ident)
	return ok && identifier.Obj == object
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) safeSignalNotifyForChannel(
	call *ast.CallExpr,
	channel *buildAuthorityLexicalObject,
) bool {
	selector, ok := buildAuthorityUnwrapExpression(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	importPath, name, imported := analyzer.importedSelector(selector)
	if !imported || importPath != "os/signal" || name != "Notify" ||
		len(call.Args) < 2 || call.Ellipsis != token.NoPos ||
		!buildAuthorityExpressionIsObject(call.Args[0], channel) {
		return false
	}
	for _, argument := range call.Args[1:] {
		if analyzer.signalExpression(argument, make(map[*buildAuthorityLexicalObject]struct{})) !=
			buildAuthoritySignalSafe {
			return false
		}
	}
	return true
}

func buildAuthorityReviewedSignalSeam(rel, packageName, functionName string) bool {
	return filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel))) == "agm/cmd/agm" &&
		packageName == "main" && functionName == "executeWithSignalContext"
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) reviewedSignalForwardingImplementation(
	node ast.Node,
	name string,
) bool {
	if name != "NotifyContext" || analyzer.rel != "agm/cmd/agm/main.go" ||
		analyzer.file.Name.Name != "main" {
		return false
	}
	declaration, ok := analyzer.enclosingFunction(node)
	return ok && declaration.Recv == nil && declaration.Name.Name == "executeWithSignalContext"
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) isExactReviewedSignalForward(call *ast.CallExpr) bool {
	if len(call.Args) != 2 || call.Ellipsis == token.NoPos {
		return false
	}
	signals, ok := buildAuthorityUnwrapExpression(call.Args[1]).(*ast.Ident)
	if !ok || signals.Obj == nil {
		return false
	}
	declaration, ok := analyzer.enclosingFunction(call)
	if !ok || declaration.Type.Params == nil {
		return false
	}
	for _, field := range declaration.Type.Params.List {
		ellipsis, ok := field.Type.(*ast.Ellipsis)
		if !ok || !analyzer.typeIsOSSignal(ellipsis.Elt) {
			continue
		}
		for _, name := range field.Names {
			if name.Name == "signals" && name.Obj != nil && name.Obj == signals.Obj {
				return buildAuthoritySignalSliceStableBefore(declaration, signals.Obj, call)
			}
		}
	}
	return false
}

func buildAuthoritySignalSliceStableBefore(
	declaration *ast.FuncDecl,
	signals *buildAuthorityLexicalObject,
	forward *ast.CallExpr,
) bool {
	stable := true
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		if node == nil || !stable {
			return stable
		}
		if node.Pos() >= forward.Pos() {
			return false
		}
		if node != forward && node.Pos() <= forward.Pos() && node.End() >= forward.End() {
			return true
		}
		stable = !buildAuthoritySignalSliceMutatedByNode(node, signals)
		return stable
	})
	return stable
}

func buildAuthoritySignalSliceMutatedByNode(
	node ast.Node,
	signals *buildAuthorityLexicalObject,
) bool {
	switch value := node.(type) {
	case *ast.AssignStmt:
		return slices.ContainsFunc(value.Lhs, func(target ast.Expr) bool {
			return buildAuthorityExpressionContainsObject(target, signals)
		}) || slices.ContainsFunc(value.Rhs, func(source ast.Expr) bool {
			return buildAuthorityExpressionContainsObject(source, signals)
		})
	case *ast.ValueSpec:
		return slices.ContainsFunc(value.Values, func(source ast.Expr) bool {
			return buildAuthorityExpressionContainsObject(source, signals)
		})
	case *ast.IncDecStmt:
		return buildAuthorityExpressionContainsObject(value.X, signals)
	case *ast.SendStmt:
		return buildAuthorityExpressionContainsObject(value.Chan, signals) ||
			buildAuthorityExpressionContainsObject(value.Value, signals)
	case *ast.CallExpr:
		return buildAuthorityExpressionContainsObject(value.Fun, signals) ||
			slices.ContainsFunc(value.Args, func(argument ast.Expr) bool {
				return buildAuthorityExpressionContainsObject(argument, signals)
			})
	default:
		return false
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) typeIsOSSignal(expression ast.Expr) bool {
	selector, ok := buildAuthorityUnwrapExpression(expression).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Signal" {
		return false
	}
	importPath, _, ok := analyzer.importedSelector(selector)
	return ok && importPath == "os"
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) classifyReviewedSignalSeamCall(call *ast.CallExpr) {
	if len(call.Args) <= 2 || call.Ellipsis != token.NoPos {
		analyzer.addViolation(call,
			"reviewed signal seam requires explicit statically audited non-SIGCHLD signals")
		return
	}
	for _, argument := range call.Args[2:] {
		switch analyzer.signalExpression(argument, make(map[*buildAuthorityLexicalObject]struct{})) {
		case buildAuthoritySignalChild:
			analyzer.addViolation(call, "reviewed signal seam receives SIGCHLD")
			return
		case buildAuthoritySignalUnknown:
			analyzer.addViolation(call,
				"reviewed signal seam receives a dynamic or numeric signal")
			return
		case buildAuthoritySignalSafe:
		}
	}
}

type buildAuthoritySignalClassification uint8

const (
	buildAuthoritySignalUnknown buildAuthoritySignalClassification = iota
	buildAuthoritySignalSafe
	buildAuthoritySignalChild
)

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) signalExpression(
	expression ast.Expr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthoritySignalClassification {
	expression = buildAuthorityUnwrapExpression(expression)
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		return analyzer.signalSelector(value)
	case *ast.Ident:
		return analyzer.signalIdentifier(value, visiting)
	case *ast.CallExpr:
		return analyzer.convertedSignal(value, visiting)
	default:
		return buildAuthoritySignalUnknown
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) signalSelector(
	selector *ast.SelectorExpr,
) buildAuthoritySignalClassification {
	importPath, name, ok := analyzer.importedSelector(selector)
	if !ok {
		return buildAuthoritySignalUnknown
	}
	if importPath == "os" && (name == "Interrupt" || name == "Kill") {
		return buildAuthoritySignalSafe
	}
	if importPath != "syscall" && importPath != "golang.org/x/sys/unix" {
		return buildAuthoritySignalUnknown
	}
	if name == "SIGCHLD" || name == "SIGCLD" {
		return buildAuthoritySignalChild
	}
	if strings.HasPrefix(name, "SIG") && len(name) > len("SIG") {
		return buildAuthoritySignalSafe
	}
	return buildAuthoritySignalUnknown
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) signalIdentifier(
	identifier *ast.Ident,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthoritySignalClassification {
	if identifier.Obj == nil {
		return buildAuthoritySignalUnknown
	}
	if identifier.Obj.Kind != ast.Con {
		if _, localShortDeclaration := identifier.Obj.Decl.(*ast.AssignStmt); !localShortDeclaration {
			return buildAuthoritySignalUnknown
		}
	}
	if _, found := visiting[identifier.Obj]; found {
		return buildAuthoritySignalUnknown
	}
	bound, ok := analyzer.bindings.value(identifier.Obj)
	if !ok {
		return buildAuthoritySignalUnknown
	}
	visiting[identifier.Obj] = struct{}{}
	classification := analyzer.signalExpression(bound, visiting)
	delete(visiting, identifier.Obj)
	return classification
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) convertedSignal(
	call *ast.CallExpr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) buildAuthoritySignalClassification {
	if len(call.Args) != 1 || !analyzer.signalConversion(call.Fun) {
		return buildAuthoritySignalUnknown
	}
	return analyzer.signalExpression(call.Args[0], visiting)
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) signalConversion(function ast.Expr) bool {
	selector, ok := buildAuthorityUnwrapExpression(function).(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Signal" {
		return false
	}
	importPath, _, ok := analyzer.importedSelector(selector)
	return ok && (importPath == "os" || importPath == "syscall" || importPath == "golang.org/x/sys/unix")
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) resolveImportedSymbol(
	expression ast.Expr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) (string, string, bool) {
	expression = buildAuthorityUnwrapExpression(expression)
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		importPath, name, ok := analyzer.importedSelector(value)
		if !ok || (importPath != "syscall" && importPath != "golang.org/x/sys/unix") {
			return "", "", false
		}
		return importPath, name, true
	case *ast.Ident:
		if value.Obj == nil || value.Obj.Kind != ast.Con {
			return "", "", false
		}
		if _, found := visiting[value.Obj]; found {
			return "", "", false
		}
		bound, ok := analyzer.bindings.value(value.Obj)
		if !ok {
			return "", "", false
		}
		visiting[value.Obj] = struct{}{}
		importPath, name, resolved := analyzer.resolveImportedSymbol(bound, visiting)
		delete(visiting, value.Obj)
		return importPath, name, resolved
	case *ast.CallExpr:
		if len(value.Args) != 1 {
			return "", "", false
		}
		identifier, ok := buildAuthorityUnwrapExpression(value.Fun).(*ast.Ident)
		if !ok || !analyzer.isPredeclaredConversion(identifier) {
			return "", "", false
		}
		return analyzer.resolveImportedSymbol(value.Args[0], visiting)
	default:
		return "", "", false
	}
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) integerValue(
	expression ast.Expr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) (int64, bool) {
	expression = buildAuthorityUnwrapExpression(expression)
	switch value := expression.(type) {
	case *ast.BasicLit:
		return buildAuthorityIntegerLiteral(value)
	case *ast.Ident:
		return analyzer.integerIdentifier(value, visiting)
	case *ast.UnaryExpr:
		return analyzer.unaryInteger(value, visiting)
	case *ast.CallExpr:
		return analyzer.convertedInteger(value, visiting)
	default:
		return 0, false
	}
}

func buildAuthorityIntegerLiteral(literal *ast.BasicLit) (int64, bool) {
	if literal.Kind != token.INT {
		return 0, false
	}
	parsed, err := strconv.ParseInt(literal.Value, 0, 64)
	return parsed, err == nil
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) integerIdentifier(
	identifier *ast.Ident,
	visiting map[*buildAuthorityLexicalObject]struct{},
) (int64, bool) {
	if identifier.Obj == nil || identifier.Obj.Kind != ast.Con {
		return 0, false
	}
	if _, found := visiting[identifier.Obj]; found {
		return 0, false
	}
	bound, ok := analyzer.bindings.value(identifier.Obj)
	if !ok {
		return 0, false
	}
	visiting[identifier.Obj] = struct{}{}
	parsed, resolved := analyzer.integerValue(bound, visiting)
	delete(visiting, identifier.Obj)
	return parsed, resolved
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) unaryInteger(
	unary *ast.UnaryExpr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) (int64, bool) {
	parsed, ok := analyzer.integerValue(unary.X, visiting)
	if !ok {
		return 0, false
	}
	if unary.Op == token.ADD {
		return parsed, true
	}
	if unary.Op == token.SUB {
		return -parsed, true
	}
	return 0, false
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) convertedInteger(
	call *ast.CallExpr,
	visiting map[*buildAuthorityLexicalObject]struct{},
) (int64, bool) {
	if len(call.Args) != 1 {
		return 0, false
	}
	identifier, ok := buildAuthorityUnwrapExpression(call.Fun).(*ast.Ident)
	if !ok || !analyzer.isPredeclaredConversion(identifier) {
		return 0, false
	}
	return analyzer.integerValue(call.Args[0], visiting)
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) importedSelector(
	selector *ast.SelectorExpr,
) (string, string, bool) {
	packageIdentifier, ok := buildAuthorityUnwrapExpression(selector.X).(*ast.Ident)
	if !ok || packageIdentifier.Obj != nil {
		return "", "", false
	}
	importPath, ok := analyzer.imports[packageIdentifier.Name]
	if !ok {
		return "", "", false
	}
	return importPath, selector.Sel.Name, true
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) enclosingFunction(
	node ast.Node,
) (*ast.FuncDecl, bool) {
	for parent := analyzer.parents[node]; parent != nil; parent = analyzer.parents[parent] {
		switch value := parent.(type) {
		case *ast.FuncLit:
			return nil, false
		case *ast.FuncDecl:
			return value, true
		}
	}
	return nil, false
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) identifierIsDeclaration(identifier *ast.Ident) bool {
	switch parent := analyzer.parents[identifier].(type) {
	case *ast.FuncDecl:
		return parent.Name == identifier
	case *ast.TypeSpec:
		return parent.Name == identifier
	case *ast.ValueSpec:
		return slices.Contains(parent.Names, identifier)
	case *ast.Field:
		return slices.Contains(parent.Names, identifier)
	case *ast.AssignStmt:
		return slices.ContainsFunc(parent.Lhs, func(expression ast.Expr) bool {
			return expression == identifier
		})
	case *ast.RangeStmt:
		return parent.Key == identifier || parent.Value == identifier
	case *ast.ImportSpec:
		return parent.Name == identifier
	case *ast.LabeledStmt:
		return parent.Label == identifier
	case *ast.BranchStmt:
		return parent.Label == identifier
	}
	return false
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) identifierIsSelectorName(identifier *ast.Ident) bool {
	selector, ok := analyzer.parents[identifier].(*ast.SelectorExpr)
	return ok && selector.Sel == identifier
}

func (analyzer *buildAuthorityWaitOwnershipAnalyzer) addViolation(node ast.Node, reason string) {
	position := analyzer.fileSet.Position(node.Pos())
	analyzer.result.violations = append(analyzer.result.violations,
		fmt.Sprintf("%s:%d: %s", analyzer.rel, position.Line, reason))
}

func buildAuthorityUnwrapExpression(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

func buildAuthorityWaitOwnershipParents(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

type buildAuthorityExpressionBindings struct {
	values  map[*buildAuthorityLexicalObject]ast.Expr
	unknown map[*buildAuthorityLexicalObject]struct{}
}

func buildAuthorityWaitOwnershipBindings(file *ast.File) buildAuthorityExpressionBindings {
	bindings := buildAuthorityExpressionBindings{
		values:  make(map[*buildAuthorityLexicalObject]ast.Expr),
		unknown: make(map[*buildAuthorityLexicalObject]struct{}),
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.ValueSpec:
			if len(value.Names) != len(value.Values) {
				return true
			}
			for index, name := range value.Names {
				bindings.bind(name.Obj, value.Values[index])
			}
		case *ast.AssignStmt:
			if len(value.Lhs) != len(value.Rhs) {
				for _, lhs := range value.Lhs {
					bindings.invalidateIdentifier(lhs)
				}
				return true
			}
			for index, lhs := range value.Lhs {
				identifier, ok := lhs.(*ast.Ident)
				if !ok || identifier.Obj == nil {
					continue
				}
				if value.Tok == token.DEFINE && identifier.Obj.Decl == value {
					bindings.bind(identifier.Obj, value.Rhs[index])
				} else {
					bindings.invalidate(identifier.Obj)
				}
			}
		case *ast.IncDecStmt:
			bindings.invalidateIdentifier(value.X)
		}
		return true
	})
	return bindings
}

func (bindings *buildAuthorityExpressionBindings) bind(
	object *buildAuthorityLexicalObject,
	expression ast.Expr,
) {
	if object == nil {
		return
	}
	if _, unknown := bindings.unknown[object]; unknown {
		return
	}
	if _, exists := bindings.values[object]; exists {
		bindings.invalidate(object)
		return
	}
	bindings.values[object] = expression
}

func (bindings *buildAuthorityExpressionBindings) invalidateIdentifier(expression ast.Expr) {
	identifier, ok := buildAuthorityUnwrapExpression(expression).(*ast.Ident)
	if ok {
		bindings.invalidate(identifier.Obj)
	}
}

func (bindings *buildAuthorityExpressionBindings) invalidate(object *buildAuthorityLexicalObject) {
	if object == nil {
		return
	}
	delete(bindings.values, object)
	bindings.unknown[object] = struct{}{}
}

func (bindings buildAuthorityExpressionBindings) value(
	object *buildAuthorityLexicalObject,
) (ast.Expr, bool) {
	if object == nil {
		return nil, false
	}
	if _, unknown := bindings.unknown[object]; unknown {
		return nil, false
	}
	expression, ok := bindings.values[object]
	return expression, ok
}

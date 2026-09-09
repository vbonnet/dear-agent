package buildauthority

import (
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestSourcePrimitiveFailureRecordAndNormalization(t *testing.T) {
	for _, operation := range []Operation{
		OperationValidate,
		OperationOpen,
		OperationProbe,
		OperationWalk,
		OperationParse,
		OperationHash,
		OperationCompare,
	} {
		t.Run(string(operation), func(t *testing.T) {
			requireSourcePrimitiveRecord(
				t,
				newSourcePrimitiveFailure(operation, CausePermission),
				operation,
				CausePermission,
			)
		})
	}

	for _, test := range []struct {
		name      string
		operation Operation
		cause     CauseCode
	}{
		{name: "zero operation", cause: CausePermission},
		{name: "non primitive operation", operation: OperationCopy, cause: CausePermission},
		{name: "zero cause", operation: OperationOpen},
		{name: "non primitive cause", operation: OperationOpen, cause: CauseDescriptorClose},
		{name: "unknown cause", operation: OperationOpen, cause: CauseCode("raw-error")},
	} {
		t.Run(test.name, func(t *testing.T) {
			requireSourcePrimitiveRecord(
				t,
				newSourcePrimitiveFailure(test.operation, test.cause),
				OperationValidate,
				CauseInternalInvariant,
			)
		})
	}

	var nilFailure *sourcePrimitiveFailure
	if nilFailure.record() != nil {
		t.Fatal("nil source primitive failure produced a record")
	}
}

func TestSourcePrimitiveFailureFromErrorPreservesOnlyClosedCauses(t *testing.T) {
	if failure := sourcePrimitiveFailureFromError(
		OperationParse,
		fail(CauseInvalidRequest, "delayed revision refusal"),
		CauseMalformed,
	); failure == nil || failure.operation != OperationParse || failure.cause != CauseInvalidRequest {
		t.Fatalf("revision parse failure = %+v, want parse/invalid-request", failure)
	}
	if failure := sourcePrimitiveFailureFromError(
		OperationParse,
		errors.New("raw parser failure"),
		CauseMalformed,
	); failure == nil || failure.operation != OperationParse || failure.cause != CauseMalformed {
		t.Fatalf("fallback parse failure = %+v, want parse/malformed", failure)
	}
	if failure := sourcePrimitiveFailureFromError(
		OperationParse,
		errors.Join(
			fail(CauseMalformed, "first parser failure"),
			fail(CauseLimit, "second parser failure"),
		),
		CauseMalformed,
	); failure == nil || failure.operation != OperationValidate ||
		failure.cause != CauseInternalInvariant {
		t.Fatalf("multi-cause parser failure = %+v, want validate/internal-invariant", failure)
	}
	if failure := sourcePrimitiveFailureFromError(OperationParse, nil, CauseMalformed); failure != nil {
		t.Fatalf("nil parser error produced failure %+v", failure)
	}
}

func TestParsedSourceACLSeparatesSyntaxFromPolicy(t *testing.T) {
	for _, disposition := range []sourceACLDisposition{
		sourceACLAdmitted,
		sourceACLMutationPermitting,
	} {
		if !disposition.valid() || !(parsedSourceACL{disposition: disposition}).valid() {
			t.Errorf("declared source ACL disposition %d is invalid", disposition)
		}
	}
	for _, disposition := range []sourceACLDisposition{0, sourceACLMutationPermitting + 1, 255} {
		if disposition.valid() || (parsedSourceACL{disposition: disposition}).valid() {
			t.Errorf("undeclared source ACL disposition %d is valid", disposition)
		}
	}
}

func TestSourceObservedKindValidityAndConversion(t *testing.T) {
	for _, test := range []struct {
		name     string
		entry    entryKind
		observed sourceObservedKind
		open     entryKind
		openable bool
	}{
		{
			name:     "directory",
			entry:    entryDirectory,
			observed: sourceObservedDirectory,
			open:     entryDirectory,
			openable: true,
		},
		{
			name:     "regular",
			entry:    entryRegular,
			observed: sourceObservedRegular,
			open:     entryRegular,
			openable: true,
		},
		{
			name:     "symlink",
			entry:    entrySymlink,
			observed: sourceObservedSymlink,
			open:     entrySymlink,
			openable: true,
		},
		{
			name:     "special",
			entry:    entryKind(255),
			observed: sourceObservedSpecial,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !test.observed.valid() {
				t.Fatalf("declared observed source kind %d is invalid", test.observed)
			}
			if got := observedSourceKind(test.entry); got != test.observed {
				t.Fatalf("observed source kind = %d, want %d", got, test.observed)
			}
			open, openable := test.observed.openKind()
			if open != test.open || openable != test.openable {
				t.Fatalf(
					"open kind = %d / %v, want %d / %v",
					open,
					openable,
					test.open,
					test.openable,
				)
			}
		})
	}

	for _, kind := range []sourceObservedKind{0, sourceObservedSpecial + 1, 255} {
		if kind.valid() {
			t.Errorf("undeclared observed source kind %d is valid", kind)
		}
		if open, openable := kind.openKind(); open != 0 || openable {
			t.Errorf("undeclared observed source kind %d opens as %d / %v", kind, open, openable)
		}
	}
}

func TestSourceUseOutcomeOwnsOnlyPrimaryAndDescriptorClose(t *testing.T) {
	outcomeType := reflect.TypeFor[sourceUseOutcome]()
	wantFields := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "primary", typeOf: reflect.TypeFor[*FailureRecord]()},
		{name: "descriptorClose", typeOf: reflect.TypeFor[*FailureRecord]()},
	}
	if outcomeType.Kind() != reflect.Struct || outcomeType.NumField() != len(wantFields) {
		t.Fatalf(
			"source-use outcome = %s with %d fields, want closed two-field struct",
			outcomeType,
			outcomeType.NumField(),
		)
	}
	for index, want := range wantFields {
		field := outcomeType.Field(index)
		if field.Name != want.name || field.Type != want.typeOf ||
			field.Anonymous || field.PkgPath == "" {
			t.Fatalf(
				"source-use outcome field %d = %s %s anonymous=%v exported=%v, want %s %s unexported",
				index,
				field.Name,
				field.Type,
				field.Anonymous,
				field.PkgPath == "",
				want.name,
				want.typeOf,
			)
		}
	}

	var outcome sourceUseOutcome
	if !outcome.proved() {
		t.Fatal("empty source-use outcome did not prove success")
	}
	first := newSourcePrimitiveFailure(OperationOpen, CauseNotFound)
	outcome.addPrimitive(first)
	primary := outcome.primary
	requireFailureRecord(t, primary, PhaseSource, OperationOpen, CauseNotFound)

	first.operation = OperationWalk
	outcome.addPrimitive(newSourcePrimitiveFailure(OperationProbe, CausePermission))
	if outcome.primary != primary {
		t.Fatal("later primitive failure replaced the first primary record")
	}
	requireFailureRecord(t, outcome.primary, PhaseSource, OperationOpen, CauseNotFound)

	outcome.addDescriptorClose()
	descriptorClose := outcome.descriptorClose
	requireFailureRecord(
		t,
		descriptorClose,
		PhaseClose,
		OperationCloseNonRoot,
		CauseDescriptorClose,
	)
	if outcome.primary != primary {
		t.Fatal("descriptor close failure replaced the primary record")
	}
	outcome.addDescriptorClose()
	outcome.addPrimitive(newSourcePrimitiveFailure(OperationHash, CauseUnstable))
	if outcome.primary != primary || outcome.descriptorClose != descriptorClose {
		t.Fatal("later source failure mutated a settled source-use outcome")
	}
	if outcome.proved() {
		t.Fatal("failed source-use outcome proved success")
	}
}

func TestSourceRepositoryLocatorAndPresenceValidity(t *testing.T) {
	validPath := t.TempDir()
	for _, test := range []struct {
		name    string
		locator sourceRepositoryLocator
		want    bool
	}{
		{
			name: "sealed clean absolute locator",
			locator: sourceRepositoryLocator{
				path: validPath,
				seal: validSourceRepositoryLocator,
			},
			want: true,
		},
		{
			name:    "missing seal",
			locator: sourceRepositoryLocator{path: validPath},
		},
		{
			name: "unknown seal",
			locator: sourceRepositoryLocator{
				path: validPath,
				seal: validSourceRepositoryLocator + 1,
			},
		},
		{
			name: "empty path",
			locator: sourceRepositoryLocator{
				seal: validSourceRepositoryLocator,
			},
		},
		{
			name: "relative path",
			locator: sourceRepositoryLocator{
				path: "repository",
				seal: validSourceRepositoryLocator,
			},
		},
		{
			name: "physical root",
			locator: sourceRepositoryLocator{
				path: physicalRootPath,
				seal: validSourceRepositoryLocator,
			},
		},
		{
			name: "unclean absolute path",
			locator: sourceRepositoryLocator{
				path: validPath + string(os.PathSeparator) + ".",
				seal: validSourceRepositoryLocator,
			},
		},
		{
			name: "NUL path",
			locator: sourceRepositoryLocator{
				path: validPath + "\x00repository",
				seal: validSourceRepositoryLocator,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.locator.valid(); got != test.want {
				t.Fatalf("source repository locator validity = %v, want %v", got, test.want)
			}
		})
	}

	for _, mode := range []sourcePresenceMode{
		sourceInitialRequired,
		sourceInitialOptional,
		sourceInitialForbidden,
		sourceRevalidatePresent,
		sourceRevalidateAbsent,
	} {
		if !mode.valid() {
			t.Errorf("declared source presence mode %d is invalid", mode)
		}
	}
	for _, mode := range []sourcePresenceMode{0, sourceRevalidateAbsent + 1, 255} {
		if mode.valid() {
			t.Errorf("undeclared source presence mode %d is valid", mode)
		}
	}
}

func TestOwnedSourceHandlesCloseExactlyOnce(t *testing.T) {
	t.Run("root success is cached", func(t *testing.T) {
		root, err := os.OpenRoot(t.TempDir())
		if err != nil {
			t.Fatalf("open temporary root: %v", err)
		}
		consumed := false
		t.Cleanup(func() {
			if !consumed {
				_ = root.Close()
			}
		})

		owner := &ownedSourceRoot{root: root, state: sourceHandleOpen}
		if !owner.validOpen() {
			t.Fatal("new concrete root owner is not open")
		}
		if closeFailed := owner.closeDirect(); closeFailed {
			t.Fatal("first temporary-root close failed")
		}
		consumed = true
		if owner.validOpen() || owner.root != nil || owner.state != sourceHandleClosed {
			t.Fatalf("closed root owner retained authority: %+v", owner)
		}
		if _, err := root.Stat("."); err == nil {
			t.Fatal("root owner did not close the concrete root")
		}
		if closeFailed := owner.closeDirect(); closeFailed {
			t.Fatal("repeated root close did not preserve the successful first result")
		}
	})

	t.Run("descriptor success is cached", func(t *testing.T) {
		descriptor, err := os.CreateTemp(t.TempDir(), "source-descriptor-")
		if err != nil {
			t.Fatalf("create temporary descriptor: %v", err)
		}
		consumed := false
		t.Cleanup(func() {
			if !consumed {
				_ = descriptor.Close()
			}
		})

		owner := &ownedSourceDescriptor{
			file:  descriptor,
			kind:  sourceObservedRegular,
			state: sourceHandleOpen,
		}
		if !owner.validOpen() {
			t.Fatal("new concrete descriptor owner is not open")
		}
		if closeFailed := owner.closeDirect(); closeFailed {
			t.Fatal("first temporary-descriptor close failed")
		}
		consumed = true
		if owner.validOpen() || owner.file != nil || owner.state != sourceHandleClosed {
			t.Fatalf("closed descriptor owner retained authority: %+v", owner)
		}
		if _, err := descriptor.Stat(); err == nil {
			t.Fatal("descriptor owner did not close the concrete file")
		}
		if closeFailed := owner.closeDirect(); closeFailed {
			t.Fatal("repeated descriptor close did not preserve the successful first result")
		}
	})

	t.Run("descriptor failure is cached", func(t *testing.T) {
		descriptor, err := os.CreateTemp(t.TempDir(), "closed-source-descriptor-")
		if err != nil {
			t.Fatalf("create temporary descriptor: %v", err)
		}
		if err := descriptor.Close(); err != nil {
			t.Fatalf("preclose temporary descriptor: %v", err)
		}

		owner := &ownedSourceDescriptor{
			file:  descriptor,
			kind:  sourceObservedRegular,
			state: sourceHandleOpen,
		}
		if closeFailed := owner.closeDirect(); !closeFailed {
			t.Fatal("closing a preclosed descriptor did not preserve the close failure")
		}
		if owner.validOpen() || owner.file != nil || owner.state != sourceHandleClosed {
			t.Fatalf("failed-close descriptor owner retained authority: %+v", owner)
		}
		if closeFailed := owner.closeDirect(); !closeFailed {
			t.Fatal("repeated descriptor close did not preserve the failed first result")
		}
	})
}

func TestOwnedSourceHandlesContainNoFunctionOrInterfaceFields(t *testing.T) {
	packagePath := reflect.TypeFor[ownedSourceRoot]().PkgPath()
	for _, ownerType := range []reflect.Type{
		reflect.TypeFor[ownedSourceRoot](),
		reflect.TypeFor[ownedSourceDescriptor](),
	} {
		if ownerType.Kind() != reflect.Struct {
			t.Fatalf("source owner %s is not a concrete struct", ownerType)
		}
		assertNoSourceOwnerOpenSeam(t, packagePath, ownerType, ownerType.Name(), map[reflect.Type]bool{})
	}
}

func TestSourcePrimitivesSignaturesHideConcreteHandles(t *testing.T) {
	primitiveType := reflect.TypeFor[sourcePrimitives]()
	if primitiveType.Kind() != reflect.Interface {
		t.Fatalf("source primitives = %s, want interface", primitiveType)
	}
	forbidden := map[reflect.Type]string{
		reflect.TypeFor[*os.File](): "*os.File",
		reflect.TypeFor[*os.Root](): "*os.Root",
	}
	for method := range primitiveType.Methods() {
		for input := 0; input < method.Type.NumIn(); input++ {
			if name, found := forbidden[method.Type.In(input)]; found {
				t.Fatalf("source primitive %s input %d exposes %s", method.Name, input, name)
			}
		}
		for output := 0; output < method.Type.NumOut(); output++ {
			if name, found := forbidden[method.Type.Out(output)]; found {
				t.Fatalf("source primitive %s output %d exposes %s", method.Name, output, name)
			}
		}
	}
}

func requireSourcePrimitiveRecord(
	t *testing.T,
	failure *sourcePrimitiveFailure,
	operation Operation,
	cause CauseCode,
) {
	t.Helper()
	if failure == nil {
		t.Fatal("source primitive failure is nil")
	}
	requireFailureRecord(t, failure.record(), PhaseSource, operation, cause)
}

func requireFailureRecord(
	t *testing.T,
	record *FailureRecord,
	phase Phase,
	operation Operation,
	cause CauseCode,
) {
	t.Helper()
	if record == nil || record.Phase != phase || record.Operation != operation ||
		len(record.Causes) != 1 || record.Causes[0] != cause || record.Child != nil {
		t.Fatalf(
			"failure record = %+v, want %s/%s/%s with no child",
			record,
			phase,
			operation,
			cause,
		)
	}
}

func assertNoSourceOwnerOpenSeam(
	t *testing.T,
	packagePath string,
	typeOf reflect.Type,
	path string,
	seen map[reflect.Type]bool,
) {
	t.Helper()
	if seen[typeOf] {
		return
	}
	seen[typeOf] = true

	switch typeOf.Kind() {
	case reflect.Func, reflect.Interface:
		t.Fatalf("source owner field %s exposes open seam %s", path, typeOf)
	case reflect.Pointer, reflect.Array, reflect.Slice, reflect.Chan:
		assertNoSourceOwnerOpenSeam(t, packagePath, typeOf.Elem(), path, seen)
	case reflect.Map:
		assertNoSourceOwnerOpenSeam(t, packagePath, typeOf.Key(), path+".key", seen)
		assertNoSourceOwnerOpenSeam(t, packagePath, typeOf.Elem(), path+".value", seen)
	case reflect.Struct:
		if typeOf.Name() != "" && typeOf.PkgPath() != "" && typeOf.PkgPath() != packagePath {
			return
		}
		for field := range typeOf.Fields() {
			assertNoSourceOwnerOpenSeam(
				t,
				packagePath,
				field.Type,
				path+"."+field.Name,
				seen,
			)
		}
	}
}

package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestSourceAdministrativeInventoryRetainsSortedMetadataOnlyRows(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	builder := exerciseMetadataInventory(owner, primitives)
	if !builder.outcome.proved() || !owner.validAdministrativeRetention() {
		t.Fatalf("metadata inventory = %+v; owner %+v", builder.outcome, owner)
	}
	if owner.state != sourceConstructionActive || owner.validInitialRetention() ||
		owner.validPackedRefsRetention() {
		t.Fatalf("completed metadata inventory exposed the wrong construction state: %+v", owner)
	}

	wantPaths := []string{
		".git",
		".git/HEAD",
		".git/config",
		".git/objects",
		".git/objects/aa",
		".git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		".git/objects/pack",
		".git/objects/pack/pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.idx",
		".git/objects/pack/pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.pack",
		".git/packed-refs",
		".git/refs",
		".git/refs/heads",
		".git/refs/heads/main",
	}
	rows := owner.git.administration.rows
	gotPaths := make([]string, len(rows))
	for index := range rows {
		gotPaths[index] = rows[index].path
		wantClass := sourceInertAdministration
		if rows[index].path == ".git" || rows[index].path == ".git/config" ||
			rows[index].path == ".git/packed-refs" ||
			strings.HasPrefix(rows[index].path, ".git/objects") {
			wantClass = sourceAuthorityAndManifest
		}
		if rows[index].class != wantClass {
			t.Errorf("row %q class = %d, want %d", rows[index].path, rows[index].class, wantClass)
		}
	}
	if !slices.Equal(gotPaths, wantPaths) {
		t.Fatalf("metadata row paths =\n%q\nwant\n%q", gotPaths, wantPaths)
	}
	if got, want := owner.git.administration.totalRegularBytes, primitives.expectedRegularBytes(); got != want {
		t.Fatalf("metadata regular bytes = %d, want %d", got, want)
	}
	if primitives.contentCalls != 0 {
		t.Fatalf("metadata-only inventory attempted %d content operations", primitives.contentCalls)
	}
	primitives.assertTransientDescriptorsClosedOnce(t)
	assertMetadataInventoryCarriesNoCapability(t)
}

func TestSourceAdministrativeInventoryRetainedShapeIsExact(t *testing.T) {
	for _, test := range []struct {
		name   string
		got    reflect.Type
		fields []struct {
			name string
			kind reflect.Type
		}
	}{
		{
			name: "row",
			got:  reflect.TypeFor[sourceAdministrativeRow](),
			fields: []struct {
				name string
				kind reflect.Type
			}{
				{name: "path", kind: reflect.TypeFor[string]()},
				{name: "kind", kind: reflect.TypeFor[sourceObservedKind]()},
				{name: "class", kind: reflect.TypeFor[sourceAdministrativeClass]()},
				{name: "evidence", kind: reflect.TypeFor[sourceDescriptorEvidence]()},
			},
		},
		{
			name: "inventory",
			got:  reflect.TypeFor[sourceAdministrativeInventory](),
			fields: []struct {
				name string
				kind reflect.Type
			}{
				{name: "rows", kind: reflect.TypeFor[[]sourceAdministrativeRow]()},
				{name: "totalRegularBytes", kind: reflect.TypeFor[uint64]()},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.got.Kind() != reflect.Struct || test.got.NumField() != len(test.fields) {
				t.Fatalf("%s shape has %d fields, want %d", test.got, test.got.NumField(), len(test.fields))
			}
			for index, want := range test.fields {
				field := test.got.Field(index)
				if field.Name != want.name || field.Type != want.kind || field.Anonymous || field.PkgPath == "" {
					t.Fatalf(
						"%s field %d = %s %s anonymous=%v exported=%v, want %s %s unexported",
						test.got,
						index,
						field.Name,
						field.Type,
						field.Anonymous,
						field.PkgPath == "",
						want.name,
						want.kind,
					)
				}
			}
		})
	}
}

func TestSourceAdministrativeInventoryTraversesCompleteTreeBeforeForbiddenPolicy(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	primitives.addNode(".git/refs/replace", sourceObservedDirectory, 0, 30)
	primitives.addNode(".git/refs/replace/target", sourceObservedRegular, 7, 31)
	primitives.directories[".git/refs"] = []string{"replace", "heads"}
	primitives.directories[".git/refs/replace"] = []string{"target"}
	primitives.addNode(".git/zz-after-forbidden", sourceObservedRegular, 5, 32)
	primitives.directories[".git"] = append(
		primitives.directories[".git"],
		"zz-after-forbidden",
	)

	builder := exerciseMetadataInventory(owner, primitives)
	requireMetadataInventoryFailure(t, builder.outcome, OperationValidate, CauseUnsupported)
	if owner.git.administration != nil || owner.state != sourceConstructionActive {
		t.Fatalf("forbidden inventory installed partial state: %+v", owner)
	}
	childClose := slices.Index(primitives.events, "close:.git/refs/replace/target")
	laterOpen := slices.Index(primitives.events, "open:.git/zz-after-forbidden")
	if childClose < 0 || laterOpen < 0 || childClose >= laterOpen {
		t.Fatalf("forbidden traversal stopped early: %q", primitives.events)
	}
	if firstPolicy := slices.Index(primitives.events, "validate-fs"); firstPolicy >= 0 && firstPolicy < laterOpen {
		t.Fatalf("metadata policy ran before complete classification: %q", primitives.events)
	}
	if primitives.contentCalls != 0 {
		t.Fatalf("forbidden metadata traversal attempted %d content operations", primitives.contentCalls)
	}
	primitives.assertTransientDescriptorsClosedOnce(t)
}

func TestSourceAdministrativeInventoryConsumesLaterDirectoryBatchBeforePolicy(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	safeCount := metadataInventoryReadBatchSize - len(primitives.directories[".git"])
	if safeCount <= 0 {
		t.Fatalf("metadata root fixture already fills a read batch: %d", safeCount)
	}
	for index := range safeCount {
		name := fmt.Sprintf("batch-%03d", index)
		primitives.addNode(".git/"+name, sourceObservedRegular, 0, uint64(100+index))
		primitives.directories[".git"] = append(primitives.directories[".git"], name)
	}
	const forbiddenName = "maintenance.lock"
	primitives.addNode(".git/"+forbiddenName, sourceObservedRegular, 0, 500)
	primitives.directories[".git"] = append(primitives.directories[".git"], forbiddenName)

	builder := exerciseMetadataInventory(owner, primitives)
	requireMetadataInventoryFailure(t, builder.outcome, OperationValidate, CauseUnsupported)
	if got := primitives.readCounts[".git"]; got != 2 {
		t.Fatalf("metadata root batch reads = %d, want 2; trace %q", got, primitives.events)
	}
	forbiddenProbe := slices.Index(primitives.events, "probe:.git/"+forbiddenName)
	if forbiddenProbe < 0 {
		t.Fatalf("later-batch forbidden row was not captured: %q", primitives.events)
	}
	if firstPolicy := slices.Index(primitives.events, "validate-fs"); firstPolicy >= 0 && firstPolicy < forbiddenProbe {
		t.Fatalf("metadata policy ran before the later batch was captured: %q", primitives.events)
	}
	if owner.git.administration != nil {
		t.Fatalf("later-batch forbidden row installed inventory: %+v", owner.git.administration)
	}
	primitives.assertTransientDescriptorsClosedOnce(t)
}

func TestSourceAdministrativeInventoryRefusesNestedMountBeforeTraversal(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	path := ".git/foreign-mount"
	primitives.addNode(path, sourceObservedDirectory, 0, 34)
	node := primitives.nodes[path]
	node.evidence.snapshot.identity.Device++
	node.evidence.mount.filesystem[0]++
	node.evidence.snapshot.identity.Filesystem = node.evidence.mount.filesystem
	primitives.nodes[path] = node
	primitives.directories[".git"] = append(primitives.directories[".git"], "foreign-mount")
	primitives.directories[path] = []string{"must-not-be-read"}

	builder := exerciseMetadataInventory(owner, primitives)
	requireMetadataInventoryFailure(t, builder.outcome, OperationValidate, CauseUnsupported)
	if slices.Contains(primitives.events, "walk:"+path) {
		t.Fatalf("nested mount was traversed before refusal: %q", primitives.events)
	}
	if owner.git.administration != nil {
		t.Fatalf("nested mount installed inventory: %+v", owner.git.administration)
	}
	primitives.assertTransientDescriptorsClosedOnce(t)
}

func TestSourceAdministrativeInventoryRejectsMutationPermittingACL(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	primitives.mutationPermittingACLPath = ".git/HEAD"

	builder := exerciseMetadataInventory(owner, primitives)
	requireMetadataInventoryFailure(t, builder.outcome, OperationValidate, CausePermission)
	if !slices.Contains(primitives.events, "validate-acl") {
		t.Fatalf("mutation-permitting ACL did not reach policy validation: %q", primitives.events)
	}
	if owner.git.administration != nil {
		t.Fatalf("mutation-permitting ACL installed inventory: %+v", owner.git.administration)
	}
	primitives.assertTransientDescriptorsClosedOnce(t)
}

func TestSourceAdministrativeInventoryRejectsForbiddenFixedKindBeforeBinding(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	primitives.addNode(".git/config", sourceObservedDirectory, 0, 33)
	primitives.directories[".git/config"] = nil

	builder := exerciseMetadataInventory(owner, primitives)
	requireMetadataInventoryFailure(t, builder.outcome, OperationValidate, CauseUnsupported)
	if owner.git.administration != nil || owner.state != sourceConstructionActive {
		t.Fatalf("forbidden fixed row installed partial state: %+v", owner)
	}
	primitives.assertTransientDescriptorsClosedOnce(t)
}

func TestSourceAdministrativeInventoryAppliesSingleLinkOnlyToAuthorityRows(t *testing.T) {
	t.Run("inert regular hard link remains metadata only", func(t *testing.T) {
		owner, primitives := newMetadataInventoryHarness(t)
		for _, path := range []string{".git/HEAD", ".git/refs/heads/main"} {
			node := primitives.nodes[path]
			node.evidence.snapshot.linkCount = 2
			primitives.nodes[path] = node
		}

		builder := exerciseMetadataInventory(owner, primitives)
		if !builder.outcome.proved() || !owner.validAdministrativeRetention() {
			t.Fatalf("inert hard-link inventory = %+v; owner %+v", builder.outcome, owner)
		}
		primitives.assertTransientDescriptorsClosedOnce(t)
	})

	for _, path := range []string{
		".git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		".git/objects/pack/pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.pack",
		".git/objects/pack/pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.idx",
	} {
		t.Run("admitted object regular requires one link/"+path, func(t *testing.T) {
			owner, primitives := newMetadataInventoryHarness(t)
			node := primitives.nodes[path]
			node.evidence.snapshot.linkCount = 2
			primitives.nodes[path] = node

			builder := exerciseMetadataInventory(owner, primitives)
			requireMetadataInventoryFailure(
				t,
				builder.outcome,
				OperationValidate,
				CauseUnsupported,
			)
			if owner.git.administration != nil {
				t.Fatalf("hard-linked object installed inventory: %+v", owner.git.administration)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}
}

func TestSourceAdministrativeInventoryFixedRowMismatchAttribution(t *testing.T) {
	for _, test := range []struct {
		name  string
		drift func(*metadataInventoryPrimitives)
		cause CauseCode
	}{
		{
			name: "identity",
			drift: func(primitives *metadataInventoryPrimitives) {
				node := primitives.nodes[".git/config"]
				node.evidence.snapshot.identity.Inode++
				primitives.nodes[".git/config"] = node
			},
			cause: CauseIdentity,
		},
		{
			name: "security",
			drift: func(primitives *metadataInventoryPrimitives) {
				node := primitives.nodes[".git/config"]
				node.evidence.snapshot.flags++
				primitives.nodes[".git/config"] = node
			},
			cause: CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, primitives := newMetadataInventoryHarness(t)
			test.drift(primitives)
			builder := exerciseMetadataInventory(owner, primitives)
			requireMetadataInventoryFailure(t, builder.outcome, OperationCompare, test.cause)
			if owner.git.administration != nil || owner.state != sourceConstructionActive {
				t.Fatalf("fixed-row mismatch installed partial state: %+v", owner)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}
}

func TestSourceAdministrativeInventoryBoundsHelpers(t *testing.T) {
	t.Run("component and relative path", func(t *testing.T) {
		exactComponent := strings.Repeat("x", maxPathComponentBytes)
		path, failure := sourceAdministrativeChildPath(context.Background(), ".git", exactComponent)
		if failure != nil || path != ".git/"+exactComponent {
			t.Fatalf("exact component path = %q / %+v", path, failure)
		}
		_, failure = sourceAdministrativeChildPath(
			context.Background(),
			".git",
			strings.Repeat("x", maxPathComponentBytes+1),
		)
		requireMetadataPrimitiveFailure(t, failure, OperationWalk, CauseLimit)

		exactPrefix := strings.Repeat("p", maxRelativePathBytes-2)
		path, failure = sourceAdministrativeChildPath(context.Background(), exactPrefix, "x")
		if failure != nil || len([]byte(path)) != maxRelativePathBytes {
			t.Fatalf("exact relative path = %d bytes / %+v", len([]byte(path)), failure)
		}
		_, failure = sourceAdministrativeChildPath(context.Background(), exactPrefix+"p", "x")
		requireMetadataPrimitiveFailure(t, failure, OperationWalk, CauseLimit)
	})

	t.Run("aggregate and pack", func(t *testing.T) {
		capture := &sourceAdministrativeCapture{totalRegularBytes: maxRepositoryBytes - 1}
		if failure := capture.chargeRegular(
			context.Background(),
			".git/HEAD",
			1,
			objectFormatSHA1,
		); failure != nil {
			t.Fatalf("exact aggregate charge failed: %+v", failure)
		}
		if capture.totalRegularBytes != maxRepositoryBytes {
			t.Fatalf("exact aggregate = %d, want %d", capture.totalRegularBytes, maxRepositoryBytes)
		}
		requireMetadataPrimitiveFailure(
			t,
			capture.chargeRegular(
				context.Background(),
				".git/HEAD",
				1,
				objectFormatSHA1,
			),
			OperationWalk,
			CauseLimit,
		)

		capture = &sourceAdministrativeCapture{}
		pack := ".git/objects/pack/pack-" + strings.Repeat("a", 40) + ".pack"
		if failure := capture.chargeRegular(
			context.Background(),
			pack,
			int64(maxRepositoryFileBytes),
			objectFormatSHA1,
		); failure != nil {
			t.Fatalf("exact pack charge failed: %+v", failure)
		}
		capture = &sourceAdministrativeCapture{}
		requireMetadataPrimitiveFailure(
			t,
			capture.chargeRegular(
				context.Background(),
				pack,
				int64(maxRepositoryFileBytes)+1,
				objectFormatSHA1,
			),
			OperationWalk,
			CauseLimit,
		)

		sha256Pack := ".git/objects/pack/pack-" +
			strings.Repeat("b", objectFormatSHA256.hexWidth()) + ".pack"
		capture = &sourceAdministrativeCapture{}
		if failure := capture.chargeRegular(
			context.Background(),
			sha256Pack,
			int64(maxRepositoryFileBytes),
			objectFormatSHA256,
		); failure != nil {
			t.Fatalf("exact SHA-256 pack charge failed: %+v", failure)
		}
		capture = &sourceAdministrativeCapture{}
		requireMetadataPrimitiveFailure(
			t,
			capture.chargeRegular(
				context.Background(),
				sha256Pack,
				int64(maxRepositoryFileBytes)+1,
				objectFormatSHA256,
			),
			OperationWalk,
			CauseLimit,
		)
	})

	t.Run("pack grammar", func(t *testing.T) {
		sha1 := strings.Repeat("a", objectFormatSHA1.hexWidth())
		sha256 := strings.Repeat("b", objectFormatSHA256.hexWidth())
		for _, test := range []struct {
			path   string
			format repositoryObjectFormat
			want   bool
		}{
			{path: ".git/objects/pack/pack-" + sha1 + ".pack", format: objectFormatSHA1, want: true},
			{path: ".git/objects/pack/pack-" + sha256 + ".pack", format: objectFormatSHA256, want: true},
			{path: ".git/objects/pack/pack-x.pack", format: objectFormatSHA1},
			{path: ".git/objects/pack/pack-" + sha1 + ".idx", format: objectFormatSHA1},
			{path: ".git/objects/pack/nested/pack-" + sha1 + ".pack", format: objectFormatSHA1},
			{path: ".git/objects/info/pack-" + sha1 + ".pack", format: objectFormatSHA1},
			{path: ".git/objects/packx/pack-" + sha1 + ".pack", format: objectFormatSHA1},
			{path: ".git/objects/pack/pack-" + strings.ToUpper(sha1) + ".pack", format: objectFormatSHA1},
			{path: ".git/objects/pack/pack-" + sha1 + ".pack", format: objectFormatSHA256},
			{path: ".git/objects/pack/pack-" + sha1 + ".pack", format: repositoryObjectFormat(255)},
		} {
			if got := sourceAdministrativePackPath(test.path, test.format); got != test.want {
				t.Errorf("canonical pack %q / %d = %v, want %v", test.path, test.format, got, test.want)
			}
		}
	})

	t.Run("descendants", func(t *testing.T) {
		owner, primitives := newMetadataInventoryHarness(t)
		builder := &sourceConstructionBuilder{
			ctx:        context.Background(),
			primitives: primitives,
			owner:      owner,
		}
		capture := &sourceAdministrativeCapture{descendants: maxRepositoryEntries - 1}
		if !builder.captureSourceAdministrativeEntry(
			capture,
			owner.git.root.descriptor,
			".git",
			"HEAD",
			sourceInitialWalkPresent,
			nil,
		) {
			t.Fatalf("exact descendant capture failed: %+v", builder.outcome)
		}
		if capture.descendants != maxRepositoryEntries || !builder.outcome.proved() {
			t.Fatalf("exact descendant state = %d / %+v", capture.descendants, builder.outcome)
		}
		probeCount := countMetadataInventoryEvent(primitives.events, "probe:.git/HEAD")
		if builder.captureSourceAdministrativeEntry(
			capture,
			owner.git.root.descriptor,
			".git",
			"HEAD",
			sourceInitialWalkPresent,
			nil,
		) {
			t.Fatal("one-over descendant capture succeeded")
		}
		requireMetadataInventoryFailure(t, builder.outcome, OperationWalk, CauseLimit)
		if got := countMetadataInventoryEvent(primitives.events, "probe:.git/HEAD"); got != probeCount {
			t.Fatalf("one-over descendant limit touched the entry: %q", primitives.events)
		}
		primitives.assertTransientDescriptorsClosedOnce(t)
	})
}

func TestSourceAdministrativeInventoryValidatorBounds(t *testing.T) {
	for _, test := range []struct {
		rows int
		want bool
	}{
		{rows: 2},
		{rows: 3, want: true},
		{rows: maxRepositoryEntries + 1, want: true},
		{rows: maxRepositoryEntries + 2},
	} {
		if got := validSourceAdministrativeInventoryRowCount(test.rows); got != test.want {
			t.Errorf("inventory row count %d valid = %v, want %v", test.rows, got, test.want)
		}
	}

	t.Run("aggregate exact and one over", func(t *testing.T) {
		exact := newMetadataInventoryValidationFixture(
			objectFormatSHA1,
			[]metadataInventoryValidationRow{{
				path: ".git/HEAD",
				kind: sourceObservedRegular,
				size: int64(maxRepositoryBytes),
			}},
		)
		if !exact.valid() || exact.inventory.totalRegularBytes != maxRepositoryBytes {
			t.Fatalf("exact aggregate inventory is invalid: %+v", exact.inventory)
		}

		oneOver := newMetadataInventoryValidationFixture(
			objectFormatSHA1,
			[]metadataInventoryValidationRow{{
				path: ".git/HEAD",
				kind: sourceObservedRegular,
				size: int64(maxRepositoryBytes) + 1,
			}},
		)
		if oneOver.valid() {
			t.Fatal("one-over aggregate inventory is valid")
		}
	})

	for _, format := range []repositoryObjectFormat{objectFormatSHA1, objectFormatSHA256} {
		t.Run("pack/"+format.name(), func(t *testing.T) {
			packPath := ".git/objects/pack/pack-" +
				strings.Repeat("a", format.hexWidth()) + ".pack"
			exact := newMetadataInventoryValidationFixture(
				format,
				[]metadataInventoryValidationRow{
					{path: ".git/objects/pack", kind: sourceObservedDirectory},
					{
						path: packPath,
						kind: sourceObservedRegular,
						size: int64(maxRepositoryFileBytes),
					},
				},
			)
			if !exact.valid() {
				t.Fatalf("exact %s pack inventory is invalid: %+v", format.name(), exact.inventory)
			}

			oneOver := newMetadataInventoryValidationFixture(
				format,
				[]metadataInventoryValidationRow{
					{path: ".git/objects/pack", kind: sourceObservedDirectory},
					{
						path: packPath,
						kind: sourceObservedRegular,
						size: int64(maxRepositoryFileBytes) + 1,
					},
				},
			)
			if oneOver.valid() {
				t.Fatalf("one-over %s pack inventory is valid", format.name())
			}
		})
	}
}

func TestSourceAdministrativeInventoryClassifiesPackLookalikeBeforePackLimit(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	path := ".git/objects/pack/pack-x.pack"
	primitives.addNode(path, sourceObservedRegular, int64(maxRepositoryFileBytes)+1, 35)
	primitives.directories[".git/objects/pack"] = append(
		primitives.directories[".git/objects/pack"],
		"pack-x.pack",
	)

	builder := exerciseMetadataInventory(owner, primitives)
	requireMetadataInventoryFailure(t, builder.outcome, OperationValidate, CauseUnsupported)
	if owner.git.administration != nil {
		t.Fatalf("pack lookalike installed inventory: %+v", owner.git.administration)
	}
	primitives.assertTransientDescriptorsClosedOnce(t)
}

func TestSourceAdministrativeInventoryFramingCancellationPrecedesRowErrors(t *testing.T) {
	for _, test := range []struct {
		name      string
		cancelAt  int
		operation Operation
		paths     []string
	}{
		{
			name:      "after sort before duplicate",
			cancelAt:  2,
			operation: OperationWalk,
			paths:     []string{".git/duplicate", ".git/duplicate"},
		},
		{
			name:      "during classification before duplicate",
			cancelAt:  4,
			operation: OperationWalk,
			paths:     []string{".git/duplicate", ".git/duplicate"},
		},
		{
			name:      "during forbidden scan before policy refusal",
			cancelAt:  8,
			operation: OperationValidate,
			paths:     []string{".git", ".git/refs/replace"},
		},
		{
			name:      "after forbidden scan before retained comparison",
			cancelAt:  9,
			operation: OperationValidate,
			paths:     []string{".git", ".git/HEAD"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, _ := newMetadataInventoryHarness(t)
			candidates := make([]sourceAdministrativeCandidate, len(test.paths))
			for index, path := range test.paths {
				candidates[index].row.path = path
				candidates[index].row.kind = sourceObservedDirectory
			}
			ctx := &sourceConstructionStepContext{cancelAt: test.cancelAt}
			builder := &sourceConstructionBuilder{ctx: ctx, owner: owner}
			inventory, failure := builder.finishSourceAdministrativeInventory(
				&sourceAdministrativeCapture{
					candidates:  candidates,
					descendants: uint64(len(candidates) - 1),
				},
			)
			requireMetadataPrimitiveFailure(t, failure, test.operation, CauseCanceled)
			if inventory != nil {
				t.Fatalf("canceled framing returned inventory: %+v", inventory)
			}
			if ctx.samples != test.cancelAt {
				t.Fatalf("context samples = %d, want %d", ctx.samples, test.cancelAt)
			}
		})
	}
}

func TestSourceAdministrativeInventoryClosesOwnedFailuresExactlyOnce(t *testing.T) {
	for _, test := range []struct {
		name     string
		root     bool
		openPath string
	}{
		{name: "root scan", root: true},
		{name: "row", openPath: ".git/HEAD"},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, primitives := newMetadataInventoryHarness(t)
			primitives.failRootOpenAfterOwner = test.root
			primitives.failOpenPathAfterOwner = test.openPath
			builder := exerciseMetadataInventory(owner, primitives)
			requireMetadataInventoryFailure(t, builder.outcome, OperationOpen, CausePermission)
			if owner.git.administration != nil || owner.state != sourceConstructionActive {
				t.Fatalf("owned acquisition failure installed state: %+v", owner)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}
}

func TestSourceAdministrativeInventoryDirectoryRecordRacesAreUnstable(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation Operation
		configure func(*metadataInventoryPrimitives)
	}{
		{
			name:      "disappears before probe",
			operation: OperationProbe,
			configure: func(primitives *metadataInventoryPrimitives) {
				primitives.failProbePathMissing = ".git/HEAD"
			},
		},
		{
			name:      "disappears before open",
			operation: OperationOpen,
			configure: func(primitives *metadataInventoryPrimitives) {
				primitives.failOpenPathMissing = ".git/HEAD"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, primitives := newMetadataInventoryHarness(t)
			test.configure(primitives)
			builder := exerciseMetadataInventory(owner, primitives)
			requireMetadataInventoryFailure(
				t,
				builder.outcome,
				test.operation,
				CauseUnstable,
			)
			if owner.git.administration != nil {
				t.Fatalf("lookup race installed inventory: %+v", owner.git.administration)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}
}

func TestSourceAdministrativeInventoryRejectsInvalidObjectFormatInvariant(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	owner.config.claim.objectFormat = repositoryObjectFormat(255)
	builder := exerciseMetadataInventory(owner, primitives)
	requireMetadataInventoryFailure(
		t,
		builder.outcome,
		OperationValidate,
		CauseInternalInvariant,
	)
	if len(primitives.transient) != 0 || owner.git.administration != nil {
		t.Fatalf("invalid object format reached inventory acquisition: %+v", primitives.events)
	}
}

func TestSourceAdministrativeInventoryOwnerValidationHonorsTrailingCancellation(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	builder := exerciseMetadataInventory(owner, primitives)
	if !builder.outcome.proved() || owner.git.administration == nil {
		t.Fatalf("prepare inventory = %+v; owner %+v", builder.outcome, owner)
	}
	inventory := owner.git.administration
	owner.git.administration = nil
	ctx := &sourceConstructionStepContext{cancelAt: 2}
	failure := validateSourceAdministrativeInventoryOwner(ctx, owner, inventory)
	requireMetadataPrimitiveFailure(t, failure, OperationValidate, CauseCanceled)
	if owner.git.administration != nil {
		t.Fatalf("trailing cancellation installed inventory: %+v", owner.git.administration)
	}
}

func exerciseMetadataInventory(
	owner *sourceConstructionOwner,
	primitives *metadataInventoryPrimitives,
) *sourceConstructionBuilder {
	builder := &sourceConstructionBuilder{
		ctx:        context.Background(),
		primitives: primitives,
		owner:      owner,
	}
	retained := builder.retainSourceAdministrativeInventory()
	if retained != builder.outcome.proved() {
		primitives.t.Fatalf(
			"metadata inventory retained=%v with outcome %+v",
			retained,
			builder.outcome,
		)
	}
	return builder
}

type metadataInventoryNode struct {
	kind     sourceObservedKind
	evidence sourceDescriptorEvidence
}

const metadataInventoryReadBatchSize = 128

type metadataInventoryPrimitives struct {
	t *testing.T

	owner *sourceConstructionOwner

	nodes       map[string]metadataInventoryNode
	directories map[string][]string
	contents    map[string][]byte

	descriptorPaths map[*ownedSourceDescriptor]string
	descriptorModes map[*ownedSourceDescriptor]sourcePresenceMode
	rootPaths       map[*ownedSourceRoot]string
	transient       []*ownedSourceDescriptor
	closeCounts     map[*ownedSourceDescriptor]int
	readCounts      map[string]int
	readCursors     map[*ownedSourceDescriptor]int
	events          []string
	contentCalls    int

	failRootOpenAfterOwner  bool
	failOpenPathAfterOwner  string
	failProbePathMissing    string
	failOpenPathMissing     string
	failRebindPathMissing   string
	failClosePath           string
	rebindIdentityDriftPath string

	mutationPermittingACLPath string
}

func newMetadataInventoryHarness(
	t *testing.T,
) (*sourceConstructionOwner, *metadataInventoryPrimitives) {
	t.Helper()
	primitives := &metadataInventoryPrimitives{
		t:               t,
		nodes:           make(map[string]metadataInventoryNode),
		directories:     make(map[string][]string),
		contents:        make(map[string][]byte),
		descriptorPaths: make(map[*ownedSourceDescriptor]string),
		descriptorModes: make(map[*ownedSourceDescriptor]sourcePresenceMode),
		rootPaths:       make(map[*ownedSourceRoot]string),
		closeCounts:     make(map[*ownedSourceDescriptor]int),
		readCounts:      make(map[string]int),
		readCursors:     make(map[*ownedSourceDescriptor]int),
	}

	primitives.addNode("physical", sourceObservedDirectory, 0, 15)
	primitives.addNode("repository", sourceObservedDirectory, 0, 1)
	primitives.addNode(".git", sourceObservedDirectory, 0, 2)
	primitives.addNode(".git/HEAD", sourceObservedRegular, 41, 3)
	primitives.addNode(".git/config", sourceObservedRegular, 32, 4)
	primitives.addNode(".git/objects", sourceObservedDirectory, 0, 5)
	primitives.addNode(".git/objects/aa", sourceObservedDirectory, 0, 6)
	primitives.addNode(
		".git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		sourceObservedRegular,
		3,
		7,
	)
	primitives.addNode(".git/objects/pack", sourceObservedDirectory, 0, 8)
	primitives.addNode(
		".git/objects/pack/pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.pack",
		sourceObservedRegular,
		8,
		9,
	)
	primitives.addNode(
		".git/objects/pack/pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.idx",
		sourceObservedRegular,
		9,
		10,
	)
	primitives.addNode(".git/packed-refs", sourceObservedRegular, 42, 11)
	primitives.addNode(".git/refs", sourceObservedDirectory, 0, 12)
	primitives.addNode(".git/refs/heads", sourceObservedDirectory, 0, 13)
	primitives.addNode(".git/refs/heads/main", sourceObservedRegular, 41, 14)
	primitives.directories[".git"] = []string{"refs", "packed-refs", "objects", "HEAD", "config"}
	primitives.directories[".git/refs"] = []string{"heads"}
	primitives.directories[".git/refs/heads"] = []string{"main"}
	primitives.directories[".git/objects"] = []string{"pack", "aa"}
	primitives.directories[".git/objects/aa"] = []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	primitives.directories[".git/objects/pack"] = []string{
		"pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.pack",
		"pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.idx",
	}

	physicalEvidence := primitives.nodes["physical"].evidence
	repositoryEvidence := primitives.nodes["repository"].evidence
	repositoryRoot := metadataInventoryRootOwner()
	repositoryDescriptor := metadataInventoryDescriptorOwner(sourceObservedDirectory)
	gitRoot := metadataInventoryRootOwner()
	gitDescriptor := metadataInventoryDescriptorOwner(sourceObservedDirectory)
	configDescriptor := metadataInventoryDescriptorOwner(sourceObservedRegular)
	objectsRoot := metadataInventoryRootOwner()
	objectsDescriptor := metadataInventoryDescriptorOwner(sourceObservedDirectory)
	packedRefsDescriptor := metadataInventoryDescriptorOwner(sourceObservedRegular)
	owner := &sourceConstructionOwner{
		state: sourceConstructionActive,
		packedRefs: sourcePackedRefsSlot{
			state: sourcePackedRefsRetained,
			leaf: &retainedSourcePackedRefs{
				descriptor: packedRefsDescriptor,
				evidence:   primitives.nodes[".git/packed-refs"].evidence,
			},
		},
		objects: &retainedSourceObjects{root: retainedSourceRoot{
			root:       objectsRoot,
			descriptor: objectsDescriptor,
			evidence:   primitives.nodes[".git/objects"].evidence,
		}},
		config: &retainedSourceConfig{
			descriptor: configDescriptor,
			evidence:   primitives.nodes[".git/config"].evidence,
			claim:      sourceConfigClaim{objectFormat: objectFormatSHA1},
		},
		git: &retainedSourceGit{root: retainedSourceRoot{
			root:       gitRoot,
			descriptor: gitDescriptor,
			evidence:   primitives.nodes[".git"].evidence,
		}},
		repository: &retainedSourceRepository{
			path: "/repository",
			root: retainedSourceRoot{
				root:       repositoryRoot,
				descriptor: repositoryDescriptor,
				evidence:   repositoryEvidence,
			},
			pathClaims: []authorityPathClaim{
				physicalEvidence.pathClaim(),
				repositoryEvidence.pathClaim(),
			},
		},
	}
	primitives.owner = owner
	primitives.descriptorPaths[repositoryDescriptor] = "repository"
	primitives.descriptorPaths[gitDescriptor] = ".git"
	primitives.descriptorPaths[configDescriptor] = ".git/config"
	primitives.descriptorPaths[objectsDescriptor] = ".git/objects"
	primitives.descriptorPaths[packedRefsDescriptor] = ".git/packed-refs"
	primitives.rootPaths[repositoryRoot] = "repository"
	primitives.rootPaths[gitRoot] = ".git"
	primitives.rootPaths[objectsRoot] = ".git/objects"
	t.Cleanup(func() {
		if owner.state == sourceConstructionActive {
			var outcome sourceUseOutcome
			owner.closeIntoWith(primitives, &outcome)
		}
	})
	if !owner.validPackedRefsRetention() {
		t.Fatalf("synthetic packed source owner is invalid: %+v", owner)
	}
	return owner, primitives
}

func (primitives *metadataInventoryPrimitives) addNode(
	path string,
	kind sourceObservedKind,
	size int64,
	inode uint64,
) {
	primitives.nodes[path] = metadataInventoryNode{
		kind:     kind,
		evidence: metadataInventoryEvidence(path, kind, size, inode),
	}
	if kind == sourceObservedDirectory {
		if _, present := primitives.directories[path]; !present {
			primitives.directories[path] = nil
		}
	}
}

func (primitives *metadataInventoryPrimitives) addDirectoryNode(
	path string,
	inode uint64,
) {
	primitives.t.Helper()
	primitives.addNode(path, sourceObservedDirectory, 0, inode)
	primitives.addDirectoryChild(path)
}

func (primitives *metadataInventoryPrimitives) addContentNode(
	path string,
	content []byte,
	inode uint64,
) {
	primitives.t.Helper()
	primitives.addNode(path, sourceObservedRegular, int64(len(content)), inode)
	primitives.contents[path] = append([]byte(nil), content...)
	primitives.addDirectoryChild(path)
}

func (primitives *metadataInventoryPrimitives) addDirectoryChild(path string) {
	separator := strings.LastIndexByte(path, '/')
	if separator < 0 {
		primitives.t.Fatalf("metadata child path has no parent: %q", path)
	}
	parent := path[:separator]
	if node, present := primitives.nodes[parent]; !present ||
		node.kind != sourceObservedDirectory {
		primitives.t.Fatalf("metadata child parent %q is unavailable for %q", parent, path)
	}
	name := path[separator+1:]
	for _, existing := range primitives.directories[parent] {
		if existing == name {
			primitives.t.Fatalf("metadata child %q already exists", path)
		}
	}
	primitives.directories[parent] = append(primitives.directories[parent], name)
}

func metadataInventoryEvidence(
	aclName string,
	kind sourceObservedKind,
	size int64,
	inode uint64,
) sourceDescriptorEvidence {
	mode := uint32(platformModeDirectory | 0o700)
	links := uint64(2)
	if kind == sourceObservedRegular {
		mode = platformModeRegular | 0o600
		links = 1
	}
	mount := mountSnapshot{filesystem: [2]int32{17, 19}, flags: 23}
	return sourceDescriptorEvidence{
		snapshot: fileSnapshot{
			identity: FileIdentity{
				Device:     29,
				Inode:      inode,
				Filesystem: mount.filesystem,
				UID:        uint32(os.Geteuid()),
				Mode:       mode,
			},
			linkCount: links,
			size:      size,
			birthSec:  31,
		},
		mount:     mount,
		aclDigest: Digest(sha256.Sum256([]byte(aclName))),
	}
}

type metadataInventoryValidationRow struct {
	path string
	kind sourceObservedKind
	size int64
}

type metadataInventoryValidationFixture struct {
	format          repositoryObjectFormat
	inventory       *sourceAdministrativeInventory
	packedRefs      sourcePackedRefsSlot
	gitEvidence     sourceDescriptorEvidence
	configEvidence  sourceDescriptorEvidence
	objectsEvidence sourceDescriptorEvidence
}

func newMetadataInventoryValidationFixture(
	format repositoryObjectFormat,
	extra []metadataInventoryValidationRow,
) metadataInventoryValidationFixture {
	fixture := metadataInventoryValidationFixture{
		format:     format,
		packedRefs: sourcePackedRefsSlot{state: sourcePackedRefsAbsent},
		gitEvidence: metadataInventoryEvidence(
			".git",
			sourceObservedDirectory,
			0,
			101,
		),
		configEvidence: metadataInventoryEvidence(
			".git/config",
			sourceObservedRegular,
			0,
			102,
		),
		objectsEvidence: metadataInventoryEvidence(
			".git/objects",
			sourceObservedDirectory,
			0,
			103,
		),
	}
	fixture.inventory = &sourceAdministrativeInventory{rows: []sourceAdministrativeRow{
		{path: ".git", kind: sourceObservedDirectory, evidence: fixture.gitEvidence},
		{path: ".git/config", kind: sourceObservedRegular, evidence: fixture.configEvidence},
		{path: ".git/objects", kind: sourceObservedDirectory, evidence: fixture.objectsEvidence},
	}}
	for index, row := range extra {
		fixture.inventory.rows = append(fixture.inventory.rows, sourceAdministrativeRow{
			path: row.path,
			kind: row.kind,
			evidence: metadataInventoryEvidence(
				row.path,
				row.kind,
				row.size,
				uint64(104+index),
			),
		})
	}
	sort.Slice(fixture.inventory.rows, func(left, right int) bool {
		return bytes.Compare(
			[]byte(fixture.inventory.rows[left].path),
			[]byte(fixture.inventory.rows[right].path),
		) < 0
	})
	for index := range fixture.inventory.rows {
		row := &fixture.inventory.rows[index]
		row.class = classifySourceAdministrativePath(
			row.path,
			sourceAdministrativeEntryKind(row.kind),
			format,
		)
		if row.kind == sourceObservedRegular {
			fixture.inventory.totalRegularBytes += uint64(row.evidence.snapshot.size)
		}
	}
	return fixture
}

func (fixture metadataInventoryValidationFixture) valid() bool {
	return fixture.inventory.valid(
		fixture.format,
		fixture.packedRefs,
		fixture.gitEvidence,
		fixture.configEvidence,
		fixture.objectsEvidence,
	)
}

func metadataInventoryRootOwner() *ownedSourceRoot {
	return &ownedSourceRoot{root: &os.Root{}, state: sourceHandleOpen}
}

func metadataInventoryDescriptorOwner(kind sourceObservedKind) *ownedSourceDescriptor {
	return &ownedSourceDescriptor{file: &os.File{}, kind: kind, state: sourceHandleOpen}
}

func (*metadataInventoryPrimitives) privateSourcePrimitives() {}

func (primitives *metadataInventoryPrimitives) openRepositoryRoot(
	context.Context,
	sourceRepositoryLocator,
) (*ownedSourceRoot, *sourcePrimitiveFailure) {
	primitives.t.Fatal("metadata inventory unexpectedly opened a repository root")
	return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
}

func (primitives *metadataInventoryPrimitives) openPhysicalRootDescriptor(
	context.Context,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	primitives.events = append(primitives.events, "open:physical")
	owner := primitives.acquireDescriptor("physical", sourceObservedDirectory)
	primitives.descriptorModes[owner] = sourceRevalidatePresent
	return owner, nil
}

func (primitives *metadataInventoryPrimitives) probeRelativeKind(
	ctx context.Context,
	parent *ownedSourceDescriptor,
	name string,
	mode sourcePresenceMode,
) (sourceObservedKind, bool, *sourcePrimitiveFailure) {
	if ctx == nil || (mode != sourceInitialWalkPresent &&
		mode != sourceRevalidatePresent) {
		primitives.t.Fatalf("metadata probe inputs = %+v / %d", ctx, mode)
	}
	path := primitives.childPath(parent, name)
	primitives.events = append(primitives.events, "probe:"+path)
	if primitives.failProbePathMissing == path {
		operation := OperationProbe
		if mode == sourceRevalidatePresent {
			operation = OperationCompare
		}
		return 0, false, newSourcePrimitiveFailure(operation, CauseUnstable)
	}
	node, present := primitives.nodes[path]
	if !present {
		operation := OperationProbe
		if mode == sourceRevalidatePresent {
			operation = OperationCompare
		}
		return 0, false, newSourcePrimitiveFailure(operation, CauseUnstable)
	}
	return node.kind, true, nil
}

func (primitives *metadataInventoryPrimitives) openRelativeNoFollow(
	ctx context.Context,
	parent *ownedSourceDescriptor,
	name string,
	kind sourceObservedKind,
	mode sourcePresenceMode,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	if ctx == nil || (mode != sourceInitialWalkPresent &&
		mode != sourceInitialRequired && mode != sourceRevalidatePresent) {
		primitives.t.Fatalf("metadata open inputs = %+v / %d", ctx, mode)
	}
	path := primitives.childPath(parent, name)
	if mode == sourceRevalidatePresent && primitives.failRebindPathMissing == path {
		primitives.events = append(primitives.events, "open:"+path)
		return nil, newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
	}
	if primitives.failOpenPathMissing == path {
		primitives.events = append(primitives.events, "open:"+path)
		switch mode {
		case sourceInitialRequired:
			return nil, newSourcePrimitiveFailure(OperationOpen, CauseNotFound)
		case sourceRevalidatePresent:
			return nil, newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		case sourceInitialWalkPresent:
			return nil, newSourcePrimitiveFailure(OperationOpen, CauseUnstable)
		default:
			return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
	}
	node, present := primitives.nodes[path]
	if !present || node.kind != kind {
		primitives.t.Fatalf("metadata open %q kind = %d, node %+v", path, kind, node)
	}
	primitives.events = append(primitives.events, "open:"+path)
	owner := primitives.acquireDescriptor(path, kind)
	primitives.descriptorModes[owner] = mode
	if primitives.failOpenPathAfterOwner == path {
		return owner, newSourcePrimitiveFailure(OperationOpen, CausePermission)
	}
	return owner, nil
}

func (primitives *metadataInventoryPrimitives) openChildRoot(
	context.Context,
	*ownedSourceRoot,
	string,
) (*ownedSourceRoot, *sourcePrimitiveFailure) {
	primitives.t.Fatal("metadata inventory unexpectedly opened a child root")
	return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
}

func (primitives *metadataInventoryPrimitives) openRootDirectoryDescriptor(
	ctx context.Context,
	root *ownedSourceRoot,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	if ctx == nil || root != primitives.owner.git.root.root {
		primitives.t.Fatalf("metadata root scan inputs = %+v / %p", ctx, root)
	}
	primitives.events = append(primitives.events, "open-scan:.git")
	owner := primitives.acquireDescriptor(".git", sourceObservedDirectory)
	if primitives.failRootOpenAfterOwner {
		return owner, newSourcePrimitiveFailure(OperationOpen, CausePermission)
	}
	return owner, nil
}

func (primitives *metadataInventoryPrimitives) readDirectoryBatch(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) ([]string, bool, *sourcePrimitiveFailure) {
	path := primitives.descriptorPath(owner)
	if ctx == nil || primitives.nodes[path].kind != sourceObservedDirectory {
		primitives.t.Fatalf("metadata directory read inputs = %+v / %q", ctx, path)
	}
	batch := primitives.readCursors[owner]
	primitives.readCursors[owner]++
	primitives.readCounts[path]++
	primitives.events = append(primitives.events, "walk:"+path)
	names := primitives.directories[path]
	start := batch * metadataInventoryReadBatchSize
	if start > len(names) {
		return nil, false, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if start == len(names) {
		return nil, true, nil
	}
	end := min(start+metadataInventoryReadBatchSize, len(names))
	terminal := end-start < metadataInventoryReadBatchSize
	return append([]string(nil), names[start:end]...), terminal, nil
}

func (primitives *metadataInventoryPrimitives) statDescriptor(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) (fileSnapshot, *sourcePrimitiveFailure) {
	path := primitives.descriptorPath(owner)
	if ctx == nil {
		primitives.t.Fatalf("metadata stat received nil context for %q", path)
	}
	primitives.events = append(primitives.events, "stat:"+path)
	node, present := primitives.nodes[path]
	if !present {
		primitives.t.Fatalf("metadata stat has no node for %q", path)
	}
	snapshot := node.evidence.snapshot
	if primitives.descriptorModes[owner] == sourceRevalidatePresent &&
		primitives.rebindIdentityDriftPath == path {
		snapshot.identity.Inode++
	}
	return snapshot, nil
}

func (primitives *metadataInventoryPrimitives) statFilesystem(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) (mountSnapshot, *sourcePrimitiveFailure) {
	path := primitives.descriptorPath(owner)
	if ctx == nil {
		primitives.t.Fatalf("metadata statfs received nil context for %q", path)
	}
	primitives.events = append(primitives.events, "statfs:"+path)
	return primitives.nodes[path].evidence.mount, nil
}

func (primitives *metadataInventoryPrimitives) validateFilesystem(
	ctx context.Context,
	mount mountSnapshot,
) *sourcePrimitiveFailure {
	if ctx == nil || mount != (mountSnapshot{filesystem: [2]int32{17, 19}, flags: 23}) {
		primitives.t.Fatalf("metadata filesystem validation = %+v / %+v", ctx, mount)
	}
	primitives.events = append(primitives.events, "validate-fs")
	return nil
}

func (primitives *metadataInventoryPrimitives) acquireRawACL(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) ([]byte, *sourcePrimitiveFailure) {
	path := primitives.descriptorPath(owner)
	if ctx == nil {
		primitives.t.Fatalf("metadata ACL acquisition received nil context for %q", path)
	}
	primitives.events = append(primitives.events, "acquire-acl:"+path)
	return []byte(path), nil
}

func (primitives *metadataInventoryPrimitives) parseRawACL(
	ctx context.Context,
	raw []byte,
) (parsedSourceACL, *sourcePrimitiveFailure) {
	if ctx == nil || len(raw) == 0 {
		primitives.t.Fatalf("metadata ACL parse inputs = %+v / %q", ctx, raw)
	}
	primitives.events = append(primitives.events, "parse-acl:"+string(raw))
	disposition := sourceACLAdmitted
	if string(raw) == primitives.mutationPermittingACLPath {
		disposition = sourceACLMutationPermitting
	}
	return parsedSourceACL{
		digest:      Digest(sha256.Sum256(raw)),
		disposition: disposition,
	}, nil
}

func (primitives *metadataInventoryPrimitives) validateACL(
	ctx context.Context,
	acl parsedSourceACL,
) *sourcePrimitiveFailure {
	if ctx == nil || !acl.valid() {
		primitives.t.Fatalf("metadata ACL validation inputs = %+v / %+v", ctx, acl)
	}
	primitives.events = append(primitives.events, "validate-acl")
	if acl.disposition == sourceACLMutationPermitting {
		return newSourcePrimitiveFailure(OperationValidate, CausePermission)
	}
	return nil
}

func (primitives *metadataInventoryPrimitives) readExactForParse(
	context.Context,
	*ownedSourceDescriptor,
	[]byte,
) *sourcePrimitiveFailure {
	primitives.contentCalls++
	return newSourcePrimitiveFailure(OperationParse, CauseInternalInvariant)
}

func (primitives *metadataInventoryPrimitives) hashBytes(
	context.Context,
	[]byte,
) (Digest, *sourcePrimitiveFailure) {
	primitives.contentCalls++
	return Digest{}, newSourcePrimitiveFailure(OperationHash, CauseInternalInvariant)
}

func (primitives *metadataInventoryPrimitives) compareRootAndDescriptor(
	ctx context.Context,
	root *ownedSourceRoot,
	descriptor *ownedSourceDescriptor,
) *sourcePrimitiveFailure {
	rootPath, present := primitives.rootPaths[root]
	descriptorPath := primitives.descriptorPath(descriptor)
	if ctx == nil || !present || rootPath != descriptorPath {
		primitives.t.Fatalf(
			"metadata root comparison = %+v / %p (%q) / %q",
			ctx,
			root,
			rootPath,
			descriptorPath,
		)
	}
	primitives.events = append(primitives.events, "compare-root:"+rootPath)
	return nil
}

func (primitives *metadataInventoryPrimitives) closeRoot(owner *ownedSourceRoot) bool {
	path, present := primitives.rootPaths[owner]
	if !present {
		primitives.t.Fatalf("metadata close has unknown root %p", owner)
	}
	primitives.events = append(primitives.events, "close-root:"+path)
	owner.root = nil
	owner.state = sourceHandleClosed
	return false
}

func (primitives *metadataInventoryPrimitives) closeDescriptor(owner *ownedSourceDescriptor) bool {
	path := primitives.descriptorPath(owner)
	primitives.closeCounts[owner]++
	primitives.events = append(primitives.events, "close:"+path)
	owner.file = nil
	owner.state = sourceHandleClosed
	return primitives.failClosePath == path
}

func (primitives *metadataInventoryPrimitives) acquireDescriptor(
	path string,
	kind sourceObservedKind,
) *ownedSourceDescriptor {
	owner := metadataInventoryDescriptorOwner(kind)
	primitives.descriptorPaths[owner] = path
	primitives.transient = append(primitives.transient, owner)
	return owner
}

func (primitives *metadataInventoryPrimitives) descriptorPath(owner *ownedSourceDescriptor) string {
	primitives.t.Helper()
	path, present := primitives.descriptorPaths[owner]
	if !present {
		primitives.t.Fatalf("metadata inventory has unknown descriptor %p", owner)
	}
	return path
}

func (primitives *metadataInventoryPrimitives) childPath(
	parent *ownedSourceDescriptor,
	name string,
) string {
	primitives.t.Helper()
	parentPath := primitives.descriptorPath(parent)
	switch {
	case parentPath == "physical" && name == "repository":
		return "repository"
	case parentPath == "repository" && name == ".git":
		return ".git"
	default:
		return parentPath + "/" + name
	}
}

func (primitives *metadataInventoryPrimitives) expectedRegularBytes() uint64 {
	var total uint64
	for _, node := range primitives.nodes {
		if node.kind == sourceObservedRegular && node.evidence.snapshot.size >= 0 {
			total += uint64(node.evidence.snapshot.size)
		}
	}
	return total
}

func (primitives *metadataInventoryPrimitives) assertTransientDescriptorsClosedOnce(t *testing.T) {
	t.Helper()
	if len(primitives.transient) == 0 {
		t.Fatal("metadata inventory acquired no transient descriptors")
	}
	for _, owner := range primitives.transient {
		if got := primitives.closeCounts[owner]; got != 1 {
			t.Errorf("transient %q close count = %d, want 1", primitives.descriptorPaths[owner], got)
		}
		if owner.state != sourceHandleClosed || owner.file != nil {
			t.Errorf("transient %q remains open: %+v", primitives.descriptorPaths[owner], owner)
		}
	}
}

func requireMetadataInventoryFailure(
	t *testing.T,
	outcome sourceUseOutcome,
	operation Operation,
	cause CauseCode,
) {
	t.Helper()
	if outcome.primary == nil || outcome.primary.Phase != PhaseSource ||
		outcome.primary.Operation != operation ||
		!slices.Equal(outcome.primary.Causes, []CauseCode{cause}) {
		t.Fatalf("metadata inventory failure = %+v, want source/%s/%s", outcome, operation, cause)
	}
	if outcome.descriptorClose != nil {
		t.Fatalf("metadata inventory unexpectedly recorded descriptor close: %+v", outcome.descriptorClose)
	}
}

func requireMetadataPrimitiveFailure(
	t *testing.T,
	failure *sourcePrimitiveFailure,
	operation Operation,
	cause CauseCode,
) {
	t.Helper()
	if failure == nil || failure.operation != operation || failure.cause != cause {
		t.Fatalf("metadata primitive failure = %+v, want %s/%s", failure, operation, cause)
	}
}

func countMetadataInventoryEvent(events []string, want string) int {
	count := 0
	for _, event := range events {
		if event == want {
			count++
		}
	}
	return count
}

func assertMetadataInventoryCarriesNoCapability(t *testing.T) {
	t.Helper()
	for _, current := range []reflect.Type{
		reflect.TypeFor[sourceAdministrativeInventory](),
		reflect.TypeFor[sourceAdministrativeRow](),
	} {
		for field := range current.Fields() {
			if metadataInventoryTypeCarriesCapability(field.Type, map[reflect.Type]bool{}) {
				t.Fatalf("metadata-only type %s field %s carries capability through %s", current, field.Name, field.Type)
			}
		}
	}
}

func metadataInventoryTypeCarriesCapability(
	current reflect.Type,
	seen map[reflect.Type]bool,
) bool {
	if current == nil || seen[current] {
		return false
	}
	seen[current] = true
	if current == reflect.TypeFor[ownedSourceDescriptor]() ||
		current == reflect.TypeFor[ownedSourceRoot]() ||
		current == reflect.TypeFor[os.File]() || current == reflect.TypeFor[os.Root]() {
		return true
	}
	switch current.Kind() {
	case reflect.Pointer, reflect.Array, reflect.Slice:
		return metadataInventoryTypeCarriesCapability(current.Elem(), seen)
	case reflect.Map:
		return metadataInventoryTypeCarriesCapability(current.Key(), seen) ||
			metadataInventoryTypeCarriesCapability(current.Elem(), seen)
	case reflect.Struct:
		for field := range current.Fields() {
			if metadataInventoryTypeCarriesCapability(field.Type, seen) {
				return true
			}
		}
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.UnsafePointer:
		return true
	}
	return false
}

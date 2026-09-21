package buildauthority

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSourceObjectPathRolesAreClosed(t *testing.T) {
	for _, test := range []struct {
		role      sourceObjectPathRole
		content   bool
		auxiliary bool
	}{
		{role: sourceObjectDirectoryPath},
		{role: sourceObjectLooseContentPath, content: true},
		{role: sourceObjectPackContentPath, content: true},
		{role: sourceObjectPackIndexPath, content: true},
		{role: sourceObjectPackAuxiliaryPath, auxiliary: true},
		{role: sourceObjectCommitGraphPath, auxiliary: true},
		{role: sourceObjectCommitGraphChainPath, auxiliary: true},
		{role: sourceObjectSplitCommitGraphPath, auxiliary: true},
		{role: sourceObjectPackListPath, auxiliary: true},
		{role: sourceObjectMultiPackIndexPath, auxiliary: true},
		{role: sourceObjectMultiPackIndexAuxiliaryPath, auxiliary: true},
		{role: sourceObjectMultiPackIndexChainPath, auxiliary: true},
		{role: sourceObjectMultiPackIndexLayerPath, auxiliary: true},
		{role: sourceObjectMultiPackIndexLayerAuxiliaryPath, auxiliary: true},
	} {
		if got := test.role.content(); got != test.content {
			t.Errorf("role %d content = %v, want %v", test.role, got, test.content)
		}
		if got := test.role.auxiliary(); got != test.auxiliary {
			t.Errorf("role %d auxiliary = %v, want %v", test.role, got, test.auxiliary)
		}
	}
	for _, invalid := range []sourceObjectPathRole{0, 255} {
		if invalid.content() || invalid.auxiliary() {
			t.Errorf("invalid object path role %d was admitted", invalid)
		}
	}
}

func TestSourceObjectPathInventoryRetainsValueOnlyRelationships(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	builder := exerciseMetadataInventory(owner, primitives)
	eventsBefore := slices.Clone(primitives.events)
	contentCallsBefore := primitives.contentCalls

	if !builder.retainSourceObjectPathInventory() || !builder.outcome.proved() {
		t.Fatalf("object path retention = %+v; owner %+v", builder.outcome, owner)
	}
	if !owner.validObjectPathRetention() || owner.validAdministrativeRetention() {
		t.Fatalf("object path inventory exposed the wrong construction state: %+v", owner)
	}

	want := []sourceObjectPathRow{
		{path: ".git/objects", role: sourceObjectDirectoryPath},
		{path: ".git/objects/aa", role: sourceObjectDirectoryPath},
		{
			path: ".git/objects/aa/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			role: sourceObjectLooseContentPath,
			key:  strings.Repeat("a", objectFormatSHA1.hexWidth()),
		},
		{path: ".git/objects/pack", role: sourceObjectDirectoryPath},
		{
			path: ".git/objects/pack/pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.idx",
			role: sourceObjectPackIndexPath,
			key:  strings.Repeat("b", objectFormatSHA1.hexWidth()),
		},
		{
			path: ".git/objects/pack/pack-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.pack",
			role: sourceObjectPackContentPath,
			key:  strings.Repeat("b", objectFormatSHA1.hexWidth()),
		},
	}
	if !slices.Equal(owner.objects.paths.rows, want) {
		t.Fatalf("object path rows =\n%+v\nwant\n%+v", owner.objects.paths.rows, want)
	}
	if owner.objects.paths.contentFileCount != 3 ||
		owner.objects.paths.auxiliaryFileCount != 0 {
		t.Fatalf("object path counts = %+v, want content=3 auxiliary=0", owner.objects.paths)
	}
	if primitives.contentCalls != contentCallsBefore ||
		!slices.Equal(primitives.events, eventsBefore) {
		t.Fatalf(
			"value-only object path retention performed I/O: calls %d -> %d; events %q -> %q",
			contentCallsBefore,
			primitives.contentCalls,
			eventsBefore,
			primitives.events,
		)
	}
}

func TestSourceObjectPathInventoryRetainedShapeIsExact(t *testing.T) {
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
			got:  reflect.TypeFor[sourceObjectPathRow](),
			fields: []struct {
				name string
				kind reflect.Type
			}{
				{name: "path", kind: reflect.TypeFor[string]()},
				{name: "role", kind: reflect.TypeFor[sourceObjectPathRole]()},
				{name: "key", kind: reflect.TypeFor[string]()},
			},
		},
		{
			name: "inventory",
			got:  reflect.TypeFor[sourceObjectPathInventory](),
			fields: []struct {
				name string
				kind reflect.Type
			}{
				{name: "rows", kind: reflect.TypeFor[[]sourceObjectPathRow]()},
				{name: "contentFileCount", kind: reflect.TypeFor[uint64]()},
				{name: "auxiliaryFileCount", kind: reflect.TypeFor[uint64]()},
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

func TestSourceObjectPathClassificationIsFormatBound(t *testing.T) {
	for _, format := range []repositoryObjectFormat{objectFormatSHA1, objectFormatSHA256} {
		t.Run(format.name(), func(t *testing.T) {
			hash := strings.Repeat("a", format.hexWidth())
			looseTail := strings.Repeat("a", format.hexWidth()-2)
			for _, test := range []struct {
				path string
				kind sourceObservedKind
				role sourceObjectPathRole
				key  string
			}{
				{path: ".git/objects", kind: sourceObservedDirectory, role: sourceObjectDirectoryPath},
				{path: ".git/objects/aa", kind: sourceObservedDirectory, role: sourceObjectDirectoryPath},
				{path: ".git/objects/info", kind: sourceObservedDirectory, role: sourceObjectDirectoryPath},
				{path: ".git/objects/info/commit-graphs", kind: sourceObservedDirectory, role: sourceObjectDirectoryPath},
				{path: ".git/objects/pack", kind: sourceObservedDirectory, role: sourceObjectDirectoryPath},
				{path: ".git/objects/pack/multi-pack-index.d", kind: sourceObservedDirectory, role: sourceObjectDirectoryPath},
				{path: ".git/objects/aa/" + looseTail, kind: sourceObservedRegular, role: sourceObjectLooseContentPath, key: hash},
				{path: ".git/objects/info/commit-graph", kind: sourceObservedRegular, role: sourceObjectCommitGraphPath},
				{path: ".git/objects/info/packs", kind: sourceObservedRegular, role: sourceObjectPackListPath},
				{path: ".git/objects/info/commit-graphs/commit-graph-chain", kind: sourceObservedRegular, role: sourceObjectCommitGraphChainPath},
				{path: ".git/objects/info/commit-graphs/graph-" + hash + ".graph", kind: sourceObservedRegular, role: sourceObjectSplitCommitGraphPath, key: hash},
				{path: ".git/objects/pack/pack-" + hash + ".pack", kind: sourceObservedRegular, role: sourceObjectPackContentPath, key: hash},
				{path: ".git/objects/pack/pack-" + hash + ".idx", kind: sourceObservedRegular, role: sourceObjectPackIndexPath, key: hash},
				{path: ".git/objects/pack/pack-" + hash + ".rev", kind: sourceObservedRegular, role: sourceObjectPackAuxiliaryPath, key: hash},
				{path: ".git/objects/pack/pack-" + hash + ".bitmap", kind: sourceObservedRegular, role: sourceObjectPackAuxiliaryPath, key: hash},
				{path: ".git/objects/pack/pack-" + hash + ".keep", kind: sourceObservedRegular, role: sourceObjectPackAuxiliaryPath, key: hash},
				{path: ".git/objects/pack/pack-" + hash + ".mtimes", kind: sourceObservedRegular, role: sourceObjectPackAuxiliaryPath, key: hash},
				{path: ".git/objects/pack/multi-pack-index", kind: sourceObservedRegular, role: sourceObjectMultiPackIndexPath},
				{path: ".git/objects/pack/multi-pack-index-" + hash + ".rev", kind: sourceObservedRegular, role: sourceObjectMultiPackIndexAuxiliaryPath, key: hash},
				{path: ".git/objects/pack/multi-pack-index-" + hash + ".bitmap", kind: sourceObservedRegular, role: sourceObjectMultiPackIndexAuxiliaryPath, key: hash},
				{path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-chain", kind: sourceObservedRegular, role: sourceObjectMultiPackIndexChainPath},
				{path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + hash + ".midx", kind: sourceObservedRegular, role: sourceObjectMultiPackIndexLayerPath, key: hash},
				{path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + hash + ".rev", kind: sourceObservedRegular, role: sourceObjectMultiPackIndexLayerAuxiliaryPath, key: hash},
				{path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + hash + ".bitmap", kind: sourceObservedRegular, role: sourceObjectMultiPackIndexLayerAuxiliaryPath, key: hash},
			} {
				row, ok := classifySourceObjectPathRow(sourceAdministrativeRow{
					path:  test.path,
					kind:  test.kind,
					class: sourceAuthorityAndManifest,
				}, format)
				if !ok || row != (sourceObjectPathRow{path: test.path, role: test.role, key: test.key}) {
					t.Errorf("classify %q = %+v/%v, want role=%d key=%q", test.path, row, ok, test.role, test.key)
				}
			}

			for _, test := range []struct {
				name   string
				row    sourceAdministrativeRow
				format repositoryObjectFormat
			}{
				{
					name:   "wrong kind",
					row:    sourceAdministrativeRow{path: ".git/objects", kind: sourceObservedRegular, class: sourceAuthorityAndManifest},
					format: format,
				},
				{
					name:   "wrong class",
					row:    sourceAdministrativeRow{path: ".git/objects/pack", kind: sourceObservedDirectory, class: sourceInertAdministration},
					format: format,
				},
				{
					name:   "wrong hash width",
					row:    sourceAdministrativeRow{path: ".git/objects/aa/" + strings.Repeat("a", format.hexWidth()-1), kind: sourceObservedRegular, class: sourceAuthorityAndManifest},
					format: format,
				},
				{
					name:   "invalid format",
					row:    sourceAdministrativeRow{path: ".git/objects", kind: sourceObservedDirectory, class: sourceAuthorityAndManifest},
					format: repositoryObjectFormat(255),
				},
			} {
				if _, ok := classifySourceObjectPathRow(test.row, test.format); ok {
					t.Errorf("%s admitted %q", test.name, test.row.path)
				}
			}
		})
	}
}

func TestSourceObjectPathInventoryDefersClosurePrerequisites(t *testing.T) {
	hash := strings.Repeat("a", objectFormatSHA1.hexWidth())
	looseTail := strings.Repeat("a", objectFormatSHA1.hexWidth()-2)
	packDirectory := metadataInventoryValidationRow{path: ".git/objects/pack", kind: sourceObservedDirectory}
	infoDirectory := metadataInventoryValidationRow{path: ".git/objects/info", kind: sourceObservedDirectory}
	graphDirectory := metadataInventoryValidationRow{path: ".git/objects/info/commit-graphs", kind: sourceObservedDirectory}
	midxDirectory := metadataInventoryValidationRow{path: ".git/objects/pack/multi-pack-index.d", kind: sourceObservedDirectory}
	pack := metadataInventoryValidationRow{path: ".git/objects/pack/pack-" + hash + ".pack", kind: sourceObservedRegular}
	index := metadataInventoryValidationRow{path: ".git/objects/pack/pack-" + hash + ".idx", kind: sourceObservedRegular}
	packAux := metadataInventoryValidationRow{path: ".git/objects/pack/pack-" + hash + ".rev", kind: sourceObservedRegular}
	graphChain := metadataInventoryValidationRow{path: ".git/objects/info/commit-graphs/commit-graph-chain", kind: sourceObservedRegular}
	splitGraph := metadataInventoryValidationRow{path: ".git/objects/info/commit-graphs/graph-" + hash + ".graph", kind: sourceObservedRegular}
	monolithicGraph := metadataInventoryValidationRow{path: ".git/objects/info/commit-graph", kind: sourceObservedRegular}
	midx := metadataInventoryValidationRow{path: ".git/objects/pack/multi-pack-index", kind: sourceObservedRegular}
	midxAux := metadataInventoryValidationRow{path: ".git/objects/pack/multi-pack-index-" + hash + ".rev", kind: sourceObservedRegular}
	midxChain := metadataInventoryValidationRow{path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-chain", kind: sourceObservedRegular}
	midxLayer := metadataInventoryValidationRow{path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + hash + ".midx", kind: sourceObservedRegular}
	midxLayerAux := metadataInventoryValidationRow{path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + hash + ".bitmap", kind: sourceObservedRegular}

	for _, test := range []struct {
		name      string
		rows      []metadataInventoryValidationRow
		want      bool
		operation Operation
		cause     CauseCode
	}{
		{name: "empty object root", want: true},
		{
			name: "loose object",
			rows: []metadataInventoryValidationRow{
				{path: ".git/objects/aa", kind: sourceObservedDirectory},
				{path: ".git/objects/aa/" + looseTail, kind: sourceObservedRegular},
			},
			want: true,
		},
		{name: "pack without index", rows: []metadataInventoryValidationRow{packDirectory, pack}},
		{name: "index without pack", rows: []metadataInventoryValidationRow{packDirectory, index}},
		{name: "pack auxiliary without pair", rows: []metadataInventoryValidationRow{packDirectory, packAux}},
		{name: "pack pair", rows: []metadataInventoryValidationRow{packDirectory, pack, index}, want: true},
		{name: "pack pair and auxiliary", rows: []metadataInventoryValidationRow{packDirectory, pack, index, packAux}, want: true},
		{name: "monolithic commit graph", rows: []metadataInventoryValidationRow{infoDirectory, monolithicGraph}, want: true},
		{name: "split graph without chain", rows: []metadataInventoryValidationRow{infoDirectory, graphDirectory, splitGraph}},
		{name: "split graph with chain", rows: []metadataInventoryValidationRow{infoDirectory, graphDirectory, graphChain, splitGraph}, want: true},
		{name: "monolithic and split commit graphs", rows: []metadataInventoryValidationRow{infoDirectory, graphDirectory, monolithicGraph, graphChain, splitGraph}},
		{name: "monolithic MIDX auxiliary without primary", rows: []metadataInventoryValidationRow{packDirectory, midxAux}},
		{name: "monolithic MIDX with auxiliary", rows: []metadataInventoryValidationRow{packDirectory, midx, midxAux}, want: true},
		{name: "incremental MIDX layer without chain", rows: []metadataInventoryValidationRow{packDirectory, midxDirectory, midxLayer}},
		{name: "incremental MIDX layer with chain", rows: []metadataInventoryValidationRow{packDirectory, midxDirectory, midxChain, midxLayer}, want: true},
		{
			name:      "incremental MIDX auxiliary without layer",
			rows:      []metadataInventoryValidationRow{packDirectory, midxDirectory, midxChain, midxLayerAux},
			operation: OperationOpen,
			cause:     CauseNotFound,
		},
		{name: "incremental MIDX layer and auxiliary", rows: []metadataInventoryValidationRow{packDirectory, midxDirectory, midxChain, midxLayer, midxLayerAux}, want: true},
		{name: "empty auxiliary directories", rows: []metadataInventoryValidationRow{infoDirectory, graphDirectory, packDirectory, midxDirectory}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newMetadataInventoryValidationFixture(objectFormatSHA1, test.rows)
			if !fixture.valid() {
				t.Fatalf("administrative fixture is invalid before topology proof: %+v", fixture.inventory)
			}
			inventory, failure := deriveSourceObjectPathInventory(
				context.Background(),
				objectFormatSHA1,
				fixture.inventory,
			)
			if failure != nil || !inventory.valid(objectFormatSHA1, fixture.inventory) {
				t.Fatalf("topology inventory = %+v / %+v", inventory, failure)
			}
			values, checksumOverrides := sourceObjectPathClosureTestClaims(inventory, hash)
			claims := sourceObjectAuxiliaryTestClaims(
				t,
				objectFormatSHA1,
				inventory,
				values,
				checksumOverrides,
			)
			closureFailure := validateSourceObjectAuxiliaryClaimInventory(
				context.Background(),
				objectFormatSHA1,
				inventory,
				claims,
			)
			if test.want {
				if closureFailure != nil {
					t.Fatalf("deferred relationship closure = %+v", closureFailure)
				}
				return
			}
			wantOperation := test.operation
			if wantOperation == "" {
				wantOperation = OperationValidate
			}
			wantCause := test.cause
			if wantCause == "" {
				wantCause = CauseUnsupported
			}
			requireMetadataPrimitiveFailure(
				t,
				closureFailure,
				wantOperation,
				wantCause,
			)
		})
	}
}

func sourceObjectPathClosureTestClaims(
	inventory *sourceObjectPathInventory,
	fallbackHash string,
) (map[string][]string, map[string]string) {
	values := make(map[string][]string)
	checksumOverrides := make(map[string]string)
	var graphValues []string
	var multiPackIndexValues []string
	var graphChainPath string
	var multiPackIndexChainPath string
	var monolithicMultiPackIndexPath string
	var monolithicMultiPackIndexHash string
	for index := range inventory.rows {
		row := inventory.rows[index]
		switch row.role {
		case sourceObjectCommitGraphChainPath:
			graphChainPath = row.path
		case sourceObjectSplitCommitGraphPath:
			graphValues = append(graphValues, row.key)
		case sourceObjectMultiPackIndexChainPath:
			multiPackIndexChainPath = row.path
		case sourceObjectMultiPackIndexLayerPath:
			multiPackIndexValues = append(multiPackIndexValues, row.key)
		case sourceObjectMultiPackIndexPath:
			monolithicMultiPackIndexPath = row.path
		case sourceObjectMultiPackIndexAuxiliaryPath:
			monolithicMultiPackIndexHash = row.key
		case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
			sourceObjectPackContentPath, sourceObjectPackIndexPath,
			sourceObjectPackAuxiliaryPath, sourceObjectCommitGraphPath,
			sourceObjectPackListPath, sourceObjectMultiPackIndexLayerAuxiliaryPath:
		}
	}
	if graphChainPath != "" {
		if len(graphValues) == 0 {
			graphValues = []string{fallbackHash}
		}
		values[graphChainPath] = graphValues
	}
	if multiPackIndexChainPath != "" {
		if len(multiPackIndexValues) == 0 {
			multiPackIndexValues = []string{fallbackHash}
		}
		values[multiPackIndexChainPath] = multiPackIndexValues
	}
	if monolithicMultiPackIndexPath != "" && monolithicMultiPackIndexHash != "" {
		checksumOverrides[monolithicMultiPackIndexPath] = monolithicMultiPackIndexHash
	}
	return values, checksumOverrides
}

func TestSourceObjectClaimRetentionRejectsIncompletePackAfterPathRetention(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	pack := ".git/objects/pack/pack-" + strings.Repeat("b", objectFormatSHA1.hexWidth()) + ".pack"
	index := ".git/objects/pack/pack-" + strings.Repeat("b", objectFormatSHA1.hexWidth()) + ".idx"
	delete(primitives.nodes, index)
	primitives.directories[".git/objects/pack"] = []string{pack[strings.LastIndexByte(pack, '/')+1:]}

	builder := exerciseMetadataInventory(owner, primitives)
	if !owner.validAdministrativeRetention() {
		t.Fatalf("name-level incomplete pack did not reach relationship stage: %+v", owner)
	}
	if !builder.retainSourceObjectPathInventory() {
		t.Fatalf("incomplete pack topology was not retained: %+v", builder.outcome)
	}
	if !owner.validObjectPathRetention() || owner.objects.claims != nil {
		t.Fatalf("incomplete pack did not stop at the path-only state: %+v", owner)
	}
	if builder.retainSourceObjectAuxiliaryClaimInventory() {
		t.Fatal("incomplete pack relationship produced claims")
	}
	requireMetadataInventoryFailure(t, builder.outcome, OperationValidate, CauseUnsupported)
	if owner.objects.paths == nil || owner.objects.claims != nil ||
		!owner.validObjectPathRetention() {
		t.Fatalf("incomplete pack mutated owner: %+v", owner)
	}
	if primitives.contentCalls != 0 {
		t.Fatalf("incomplete pack relationship attempted %d content operations", primitives.contentCalls)
	}
}

func TestSourceObjectPathRetentionRejectsRepeatedAndCanceledStages(t *testing.T) {
	t.Run("repeated", func(t *testing.T) {
		owner, primitives := newMetadataInventoryHarness(t)
		builder := exerciseMetadataInventory(owner, primitives)
		if !builder.retainSourceObjectPathInventory() {
			t.Fatalf("initial path retention = %+v", builder.outcome)
		}
		retained := owner.objects.paths
		if builder.retainSourceObjectPathInventory() {
			t.Fatal("repeated path retention succeeded")
		}
		requireMetadataInventoryFailure(t, builder.outcome, OperationValidate, CauseInternalInvariant)
		if owner.objects.paths != retained || !owner.validObjectPathRetention() {
			t.Fatalf("repeated path retention mutated owner: %+v", owner)
		}
	})

	t.Run("builder canceled before derivation", func(t *testing.T) {
		owner, primitives := newMetadataInventoryHarness(t)
		builder := exerciseMetadataInventory(owner, primitives)
		builder.ctx = &sourceConstructionStepContext{cancelAt: 1}
		if builder.retainSourceObjectPathInventory() {
			t.Fatal("canceled path retention succeeded")
		}
		requireMetadataInventoryFailure(t, builder.outcome, OperationValidate, CauseCanceled)
		if owner.objects.paths != nil || !owner.validAdministrativeRetention() {
			t.Fatalf("canceled path retention mutated owner: %+v", owner)
		}
	})

	t.Run("derivation canceled", func(t *testing.T) {
		fixture := newMetadataInventoryValidationFixture(objectFormatSHA1, nil)
		inventory, failure := deriveSourceObjectPathInventory(
			&sourceConstructionStepContext{cancelAt: 1},
			objectFormatSHA1,
			fixture.inventory,
		)
		requireMetadataPrimitiveFailure(t, failure, OperationParse, CauseCanceled)
		if inventory != nil {
			t.Fatalf("canceled derivation returned inventory: %+v", inventory)
		}
	})

	t.Run("cancellation outranks invalid topology", func(t *testing.T) {
		fixture := newMetadataInventoryValidationFixture(objectFormatSHA1, nil)
		fixture.inventory.rows[2].path = ".git/objects/aa"
		inventory, failure := deriveSourceObjectPathInventory(
			&sourceConstructionStepContext{cancelAt: 5},
			objectFormatSHA1,
			fixture.inventory,
		)
		requireMetadataPrimitiveFailure(t, failure, OperationValidate, CauseCanceled)
		if inventory != nil {
			t.Fatalf("canceled invalid-topology derivation returned inventory: %+v", inventory)
		}
	})

	t.Run("cancellation continues through derived topology", func(t *testing.T) {
		hash := strings.Repeat("a", objectFormatSHA1.hexWidth())
		fixture := newMetadataInventoryValidationFixture(
			objectFormatSHA1,
			[]metadataInventoryValidationRow{
				{path: ".git/objects/pack", kind: sourceObservedDirectory},
				{path: ".git/objects/pack/pack-" + hash + ".pack", kind: sourceObservedRegular},
			},
		)
		if !fixture.valid() {
			t.Fatalf("incomplete-pack administrative fixture is invalid: %+v", fixture.inventory)
		}
		inventory, failure := deriveSourceObjectPathInventory(
			&sourceConstructionStepContext{cancelAt: 8},
			objectFormatSHA1,
			fixture.inventory,
		)
		if failure == nil || failure.cause != CauseCanceled {
			t.Fatalf("derived-topology cancellation = %+v", failure)
		}
		if inventory != nil {
			t.Fatalf("canceled topology derivation returned inventory: %+v", inventory)
		}
	})

	t.Run("owner validation honors trailing cancellation", func(t *testing.T) {
		owner, primitives := newMetadataInventoryHarness(t)
		exerciseMetadataInventory(owner, primitives)
		inventory, failure := deriveSourceObjectPathInventory(
			context.Background(),
			objectFormatSHA1,
			owner.git.administration,
		)
		if failure != nil {
			t.Fatalf("prepare path inventory = %+v", failure)
		}
		failure = validateSourceObjectPathInventoryOwner(
			&sourceConstructionStepContext{cancelAt: 2},
			owner,
			inventory,
		)
		requireMetadataPrimitiveFailure(t, failure, OperationValidate, CauseCanceled)
		if owner.objects.paths != nil || !owner.validAdministrativeRetention() {
			t.Fatalf("trailing cancellation mutated owner: %+v", owner)
		}
	})
}

func TestSourceObjectPathInventoryRejectsForgedValues(t *testing.T) {
	owner, primitives := newMetadataInventoryHarness(t)
	builder := exerciseMetadataInventory(owner, primitives)
	if !builder.retainSourceObjectPathInventory() {
		t.Fatalf("prepare path inventory = %+v", builder.outcome)
	}
	original := owner.objects.paths

	for _, test := range []struct {
		name   string
		mutate func(*sourceObjectPathInventory)
	}{
		{
			name: "path",
			mutate: func(inventory *sourceObjectPathInventory) {
				inventory.rows[0].path = ".git/objectz"
			},
		},
		{
			name: "role",
			mutate: func(inventory *sourceObjectPathInventory) {
				inventory.rows[1].role = sourceObjectPackContentPath
			},
		},
		{
			name: "key",
			mutate: func(inventory *sourceObjectPathInventory) {
				inventory.rows[2].key = strings.Repeat("b", objectFormatSHA1.hexWidth())
			},
		},
		{
			name: "content count",
			mutate: func(inventory *sourceObjectPathInventory) {
				inventory.contentFileCount++
			},
		},
		{
			name: "auxiliary count",
			mutate: func(inventory *sourceObjectPathInventory) {
				inventory.auxiliaryFileCount++
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			forged := *original
			forged.rows = slices.Clone(original.rows)
			test.mutate(&forged)
			if forged.valid(objectFormatSHA1, owner.git.administration) {
				t.Fatalf("forged inventory is valid: %+v", forged)
			}
		})
	}
}

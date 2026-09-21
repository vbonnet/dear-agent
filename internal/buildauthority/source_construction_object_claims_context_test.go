package buildauthority

import (
	"context"
	"testing"
)

func TestSourceObjectAuxiliaryLoopCancellationPrecedesSemanticMismatch(t *testing.T) {
	t.Run("ordered values", func(t *testing.T) {
		ctx := &sourceConstructionStepContext{cancelAt: 3}
		equal, failure := equalSourceObjectAuxiliaryValues(
			ctx,
			[]string{auxiliaryP0SHA1},
			[]string{auxiliaryP1SHA1},
		)
		if equal {
			t.Fatal("mismatched auxiliary values compared equal")
		}
		requireAuxiliaryFailure(t, failure, OperationCompare, CauseCanceled)
	})

	t.Run("duplicate decoded values", func(t *testing.T) {
		ctx := &sourceConstructionStepContext{cancelAt: 5}
		valid, failure := sourceObjectAuxiliaryValuesValidity(
			ctx,
			OperationValidate,
			objectFormatSHA1,
			[]string{auxiliaryP0SHA1, auxiliaryP0SHA1},
			1,
			maxSourceObjectAuxiliaryRecords,
		)
		if valid {
			t.Fatal("duplicate auxiliary values validated")
		}
		requireAuxiliaryFailure(t, failure, OperationValidate, CauseCanceled)
	})

	t.Run("path ordering", func(t *testing.T) {
		ctx := &sourceConstructionStepContext{cancelAt: 7}
		valid, failure := sourceObjectPathTopologyValidity(
			ctx,
			OperationCompare,
			[]sourceObjectPathRow{
				{path: ".git/objects", role: sourceObjectDirectoryPath},
				{path: ".git/objects", role: sourceObjectDirectoryPath},
			},
		)
		if valid {
			t.Fatal("unordered object paths validated")
		}
		requireAuxiliaryFailure(t, failure, OperationCompare, CauseCanceled)
	})

	t.Run("closure binding", func(t *testing.T) {
		ctx := &sourceConstructionStepContext{cancelAt: 3}
		failure := sourceObjectAuxiliaryClosureFailureForContext(
			ctx,
			OperationCompare,
			objectFormatSHA1,
			[]sourceObjectPathRow{{
				path: ".git/objects/info/commit-graph",
				role: sourceObjectCommitGraphPath,
			}},
			[]sourceObjectAuxiliaryClaim{{
				path: ".git/objects/info/wrong",
				role: sourceObjectCommitGraphPath,
			}},
		)
		requireAuxiliaryFailure(t, failure, OperationCompare, CauseCanceled)
	})
}

func TestSourceObjectAuxiliaryPackListClaimRejectsRecordCounterOverflow(t *testing.T) {
	packList := sourceObjectPathRow{
		path: ".git/objects/info/packs",
		role: sourceObjectPackListPath,
	}
	paths := sourceObjectAuxiliaryTestPaths([]sourceObjectPathRow{packList})
	claims := sourceObjectAuxiliaryTestClaims(
		t,
		objectFormatSHA1,
		paths,
		nil,
		nil,
	)
	claims.rows[0].blankRecords = ^uint64(0)

	failure := validateSourceObjectAuxiliaryClaimInventory(
		context.Background(),
		objectFormatSHA1,
		paths,
		claims,
	)
	requireAuxiliaryFailure(t, failure, OperationValidate, CauseInternalInvariant)
}

func TestSourceObjectAuxiliaryAdministrationMatchHonorsCancellation(t *testing.T) {
	path := ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".keep"
	administration := &sourceAdministrativeInventory{rows: []sourceAdministrativeRow{{
		path:  path,
		kind:  sourceObservedRegular,
		class: sourceAuthorityAndManifest,
		evidence: sourceDescriptorEvidence{snapshot: fileSnapshot{
			size: 1,
		}},
	}}}
	claims := &sourceObjectAuxiliaryClaimInventory{rows: []sourceObjectAuxiliaryClaim{{
		path: path,
		role: sourceObjectPackAuxiliaryPath,
		bytes: sourceObjectAuxiliaryByteClaim{
			size:             1,
			repositoryFormat: objectFormatSHA1,
		},
	}}}
	matches, failure := sourceObjectAuxiliaryClaimsMatchAdministrationValidity(
		&sourceConstructionStepContext{cancelAt: 2},
		OperationValidate,
		administration,
		claims,
	)
	if matches {
		t.Fatal("canceled administration comparison matched")
	}
	requireAuxiliaryFailure(t, failure, OperationValidate, CauseCanceled)
}

func TestSourceObjectAuxiliarySizeGateHonorsCancellation(t *testing.T) {
	for _, test := range []struct {
		name      string
		chain     bool
		operation Operation
	}{
		{name: "chain", chain: true, operation: OperationParse},
		{name: "opaque", operation: OperationWalk},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := sourceObjectAuxiliarySizeFailure(
				&sourceConstructionStepContext{cancelAt: 2},
				objectFormatSHA1,
				int64(maxSourceObjectAuxiliaryBytes)+1,
				test.chain,
			)
			requireAuxiliaryFailure(t, failure, test.operation, CauseCanceled)
		})
	}
}

func TestSourceObjectAuxiliaryRevalidationSamplesCancellationBeforeOwnerValidation(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*sourceConstructionBuilder) bool
	}{
		{
			name: "entry",
			run: func(builder *sourceConstructionBuilder) bool {
				return builder.revalidateSourceObjectAuxiliaryClaimInventory()
			},
		},
		{
			name: "administrative capture",
			run: func(builder *sourceConstructionBuilder) bool {
				_, captured := builder.captureRevalidatedSourceAdministrativeInventory()
				return captured
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			builder := &sourceConstructionBuilder{ctx: ctx}
			if test.run(builder) {
				t.Fatal("canceled revalidation succeeded")
			}
			requireFailureRecord(
				t,
				builder.outcome.primary,
				PhaseSource,
				OperationCompare,
				CauseCanceled,
			)
		})
	}
}

func TestSourceObjectAuxiliaryPreReadEvidenceDefersOnlyContentTimestamps(t *testing.T) {
	base := sourceDescriptorEvidence{
		snapshot: fileSnapshot{
			identity: FileIdentity{
				Device:     1,
				Inode:      2,
				UID:        3,
				Mode:       platformModeRegular | 0o600,
				Filesystem: [2]int32{4, 5},
			},
			linkCount:  1,
			gid:        6,
			rdev:       7,
			size:       8,
			mtimeSec:   9,
			mtimeNsec:  10,
			ctimeSec:   11,
			ctimeNsec:  12,
			birthSec:   13,
			birthNsec:  14,
			flags:      15,
			generation: 16,
		},
		mount:     mountSnapshot{filesystem: [2]int32{4, 5}, flags: 17},
		aclDigest: Digest{18},
	}
	timestamps := base
	timestamps.snapshot.mtimeSec++
	timestamps.snapshot.mtimeNsec++
	timestamps.snapshot.ctimeSec++
	timestamps.snapshot.ctimeNsec++
	if failure := compareSourceObjectAuxiliaryPreReadEvidence(
		context.Background(),
		base,
		timestamps,
	); failure != nil {
		t.Fatalf("timestamp-only pre-read evidence = %+v", failure)
	}

	identity := base
	identity.snapshot.identity.Inode++
	requireAuxiliaryFailure(
		t,
		compareSourceObjectAuxiliaryPreReadEvidence(context.Background(), base, identity),
		OperationCompare,
		CauseIdentity,
	)

	for _, test := range []struct {
		name   string
		mutate func(*sourceDescriptorEvidence)
	}{
		{name: "size", mutate: func(value *sourceDescriptorEvidence) { value.snapshot.size++ }},
		{name: "mode", mutate: func(value *sourceDescriptorEvidence) { value.snapshot.identity.Mode ^= 0o020 }},
		{name: "link count", mutate: func(value *sourceDescriptorEvidence) { value.snapshot.linkCount++ }},
		{name: "gid", mutate: func(value *sourceDescriptorEvidence) { value.snapshot.gid++ }},
		{name: "birth", mutate: func(value *sourceDescriptorEvidence) { value.snapshot.birthNsec++ }},
		{name: "flags", mutate: func(value *sourceDescriptorEvidence) { value.snapshot.flags++ }},
		{name: "generation", mutate: func(value *sourceDescriptorEvidence) { value.snapshot.generation++ }},
		{name: "mount", mutate: func(value *sourceDescriptorEvidence) { value.mount.flags++ }},
		{name: "acl", mutate: func(value *sourceDescriptorEvidence) { value.aclDigest[0]++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := base
			test.mutate(&changed)
			requireAuxiliaryFailure(
				t,
				compareSourceObjectAuxiliaryPreReadEvidence(
					context.Background(),
					base,
					changed,
				),
				OperationCompare,
				CauseUnstable,
			)
		})
	}
}

func TestSourceDirectoryPreWalkEvidenceDefersOnlyChildChurn(t *testing.T) {
	base := sourceDescriptorEvidence{
		snapshot: fileSnapshot{
			identity: FileIdentity{
				Device:     1,
				Inode:      2,
				UID:        3,
				Mode:       platformModeDirectory | 0o700,
				Filesystem: [2]int32{4, 5},
			},
			linkCount:  6,
			gid:        7,
			size:       8,
			mtimeSec:   9,
			mtimeNsec:  10,
			ctimeSec:   11,
			ctimeNsec:  12,
			birthSec:   13,
			birthNsec:  14,
			flags:      15,
			generation: 16,
		},
		mount:     mountSnapshot{filesystem: [2]int32{4, 5}, flags: 17},
		aclDigest: Digest{18},
	}
	churn := base
	churn.snapshot.linkCount++
	churn.snapshot.size++
	churn.snapshot.mtimeSec++
	churn.snapshot.mtimeNsec++
	churn.snapshot.ctimeSec++
	churn.snapshot.ctimeNsec++
	if failure := compareSourceDirectoryPreWalkEvidence(
		context.Background(),
		base,
		churn,
	); failure != nil {
		t.Fatalf("directory child-churn evidence = %+v", failure)
	}

	identity := base
	identity.snapshot.identity.Inode++
	requireAuxiliaryFailure(
		t,
		compareSourceDirectoryPreWalkEvidence(context.Background(), base, identity),
		OperationCompare,
		CauseIdentity,
	)
	security := base
	security.snapshot.identity.Mode ^= 0o020
	requireAuxiliaryFailure(
		t,
		compareSourceDirectoryPreWalkEvidence(context.Background(), base, security),
		OperationCompare,
		CauseUnstable,
	)
}

func TestSourceInertRegularEvidenceAllowsOnlyUnreadContentDrift(t *testing.T) {
	base := sourceDescriptorEvidence{
		snapshot: fileSnapshot{
			identity: FileIdentity{
				Device:     1,
				Inode:      2,
				UID:        3,
				Mode:       platformModeRegular | 0o600,
				Filesystem: [2]int32{4, 5},
			},
			linkCount: 1,
			gid:       6,
			size:      7,
			mtimeSec:  8,
			ctimeSec:  9,
			birthSec:  10,
		},
		mount:     mountSnapshot{filesystem: [2]int32{4, 5}, flags: 11},
		aclDigest: Digest{12},
	}
	contentDrift := base
	contentDrift.snapshot.size++
	contentDrift.snapshot.mtimeSec++
	contentDrift.snapshot.ctimeSec++
	if failure := compareSourceInertRegularEvidence(
		context.Background(),
		base,
		contentDrift,
	); failure != nil {
		t.Fatalf("inert content evidence = %+v", failure)
	}

	identity := base
	identity.snapshot.identity.Inode++
	requireAuxiliaryFailure(
		t,
		compareSourceInertRegularEvidence(context.Background(), base, identity),
		OperationCompare,
		CauseIdentity,
	)
	security := base
	security.snapshot.linkCount++
	requireAuxiliaryFailure(
		t,
		compareSourceInertRegularEvidence(context.Background(), base, security),
		OperationCompare,
		CauseUnstable,
	)
}

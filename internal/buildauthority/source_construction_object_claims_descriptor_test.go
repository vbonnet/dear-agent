package buildauthority

import (
	"strings"
	"testing"
)

func TestSourceObjectAuxiliaryContentFailurePrecedesTrailingRebindFailure(t *testing.T) {
	chainPath := ".git/objects/info/commit-graphs/commit-graph-chain"
	packAuxiliaryPath := ".git/objects/pack/pack-" + sourceObjectAuxiliaryTestPackHash + ".rev"
	for _, test := range []struct {
		name      string
		path      string
		mutate    func(*metadataInventoryPrimitives)
		operation Operation
		cause     CauseCode
	}{
		{
			name: "parse failure",
			path: chainPath,
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(chainPath, []byte(strings.ToUpper(auxiliaryP0SHA1)+"\n"))
			},
			operation: OperationParse,
			cause:     CauseMalformed,
		},
		{
			name: "hash read failure",
			path: packAuxiliaryPath,
			mutate: func(primitives *metadataInventoryPrimitives) {
				delete(primitives.contents, packAuxiliaryPath)
			},
			operation: OperationHash,
			cause:     CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, primitives, builder := prepareSourceObjectAuxiliaryDescriptorTest(t)
			test.mutate(primitives)
			primitives.failRebindPathMissing = test.path
			opensBefore := countMetadataInventoryEvent(primitives.events, "open:"+test.path)

			if builder.retainSourceObjectAuxiliaryClaimInventory() {
				t.Fatal("content failure plus trailing rebind failure retained claims")
			}
			requireFailureRecord(
				t,
				builder.outcome.primary,
				PhaseSource,
				test.operation,
				test.cause,
			)
			if builder.outcome.descriptorClose != nil {
				t.Fatalf("content plus rebind failure recorded close failure: %+v", builder.outcome)
			}
			assertSourceObjectAuxiliaryDescriptorClaimNotInstalled(t, owner)
			if got := countMetadataInventoryEvent(primitives.events, "open:"+test.path) - opensBefore; got != 2 {
				t.Fatalf("content failure path opens = %d, want initial plus trailing rebind; trace %q", got, primitives.events)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}
}

func TestSourceObjectAuxiliaryPrimaryAndDescriptorCloseRemainSeparate(t *testing.T) {
	path := ".git/objects/info/commit-graphs/commit-graph-chain"
	owner, primitives, builder := prepareSourceObjectAuxiliaryDescriptorTest(t)
	primitives.setContentNode(path, []byte(strings.ToUpper(auxiliaryP0SHA1)+"\n"))
	primitives.failClosePath = path
	opensBefore := countMetadataInventoryEvent(primitives.events, "open:"+path)
	closesBefore := countMetadataInventoryEvent(primitives.events, "close:"+path)

	if builder.retainSourceObjectAuxiliaryClaimInventory() {
		t.Fatal("content failure plus descriptor-close failure retained claims")
	}
	requireFailureRecord(
		t,
		builder.outcome.primary,
		PhaseSource,
		OperationParse,
		CauseMalformed,
	)
	requireFailureRecord(
		t,
		builder.outcome.descriptorClose,
		PhaseClose,
		OperationCloseNonRoot,
		CauseDescriptorClose,
	)
	assertSourceObjectAuxiliaryDescriptorClaimNotInstalled(t, owner)
	if got := countMetadataInventoryEvent(primitives.events, "open:"+path) - opensBefore; got != 2 {
		t.Fatalf("primary-plus-close path opens = %d, want initial plus trailing rebind; trace %q", got, primitives.events)
	}
	if got := countMetadataInventoryEvent(primitives.events, "close:"+path) - closesBefore; got != 2 {
		t.Fatalf("primary-plus-close path closes = %d, want exact-once initial plus rebind closes; trace %q", got, primitives.events)
	}
	primitives.assertTransientDescriptorsClosedOnce(t)
}

func TestSourceObjectAuxiliaryCleanContentTrailingRebindDriftFailsClosed(t *testing.T) {
	path := ".git/objects/info/commit-graphs/commit-graph-chain"
	for _, test := range []struct {
		name   string
		inject func(*metadataInventoryPrimitives)
		cause  CauseCode
	}{
		{
			name: "replacement",
			inject: func(primitives *metadataInventoryPrimitives) {
				primitives.rebindIdentityDriftPath = path
			},
			cause: CauseIdentity,
		},
		{
			name: "disappearance",
			inject: func(primitives *metadataInventoryPrimitives) {
				primitives.failRebindPathMissing = path
			},
			cause: CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, primitives, builder := prepareSourceObjectAuxiliaryDescriptorTest(t)
			test.inject(primitives)
			opensBefore := countMetadataInventoryEvent(primitives.events, "open:"+path)
			readsBefore := primitives.contentCalls

			if builder.retainSourceObjectAuxiliaryClaimInventory() {
				t.Fatal("clean content with trailing rebind drift retained claims")
			}
			requireFailureRecord(
				t,
				builder.outcome.primary,
				PhaseSource,
				OperationCompare,
				test.cause,
			)
			if builder.outcome.descriptorClose != nil {
				t.Fatalf("clean rebind drift recorded close failure: %+v", builder.outcome)
			}
			assertSourceObjectAuxiliaryDescriptorClaimNotInstalled(t, owner)
			if primitives.contentCalls == readsBefore {
				t.Fatalf("clean rebind drift skipped the descriptor-bound content read: %q", primitives.events)
			}
			if got := countMetadataInventoryEvent(primitives.events, "open:"+path) - opensBefore; got != 2 {
				t.Fatalf("clean rebind drift path opens = %d, want initial plus trailing rebind; trace %q", got, primitives.events)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}
}

func prepareSourceObjectAuxiliaryDescriptorTest(
	t *testing.T,
) (*sourceConstructionOwner, *metadataInventoryPrimitives, *sourceConstructionBuilder) {
	t.Helper()
	owner, primitives := newSourceObjectAuxiliaryAdmissionHarness(t)
	builder := exerciseMetadataInventory(owner, primitives)
	if !builder.outcome.proved() || !builder.retainSourceObjectPathInventory() ||
		!owner.validObjectPathRetention() {
		t.Fatalf("descriptor adversary path retention = %+v; owner %+v", builder.outcome, owner)
	}
	return owner, primitives, builder
}

func assertSourceObjectAuxiliaryDescriptorClaimNotInstalled(
	t *testing.T,
	owner *sourceConstructionOwner,
) {
	t.Helper()
	if owner.objects.claims != nil || !owner.validObjectPathRetention() {
		t.Fatalf("descriptor adversary mutated path-only owner: %+v", owner)
	}
}

package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"reflect"
	"strings"
	"testing"
)

const sourceObjectAuxiliaryTestPackHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestSourceObjectAuxiliaryClaimAdmission(t *testing.T) {
	owner, primitives := newSourceObjectAuxiliaryAdmissionHarness(t)
	builder := exerciseMetadataInventory(owner, primitives)
	if !builder.outcome.proved() || !builder.retainSourceObjectPathInventory() ||
		!owner.validObjectPathRetention() {
		t.Fatalf("path retention = %+v; owner %+v", builder.outcome, owner)
	}
	contentCallsBefore := primitives.contentCalls
	if !builder.retainSourceObjectAuxiliaryClaimInventory() || !builder.outcome.proved() {
		t.Fatalf("claim retention = %+v; owner %+v", builder.outcome, owner)
	}
	if !owner.validObjectClaimRetention() || owner.validObjectPathRetention() ||
		owner.objects.claims == nil || len(owner.objects.claims.rows) != 9 {
		t.Fatalf("retained claims = %+v; owner %+v", owner.objects.claims, owner)
	}
	if got := primitives.contentCalls - contentCallsBefore; got < 9 {
		t.Fatalf("auxiliary bounded reads = %d, want at least 9 including a nonzero-offset read", got)
	}

	for index := range owner.objects.claims.rows {
		claim := owner.objects.claims.rows[index]
		body := primitives.contents[claim.path]
		if claim.bytes.size != uint64(len(body)) ||
			claim.bytes.manifest != Digest(sha256.Sum256(body)) {
			t.Fatalf("claim %q bytes = %+v, want size=%d sha256=%x", claim.path, claim.bytes, len(body), sha256.Sum256(body))
		}
		switch claim.role {
		case sourceObjectCommitGraphChainPath:
			if len(claim.values) != 1 || claim.values[0] != auxiliaryP0SHA1 {
				t.Fatalf("commit-graph chain claim = %+v", claim)
			}
		case sourceObjectPackListPath:
			if len(claim.values) != 1 || claim.values[0] != sourceObjectAuxiliaryTestPackHash ||
				claim.blankRecords != 1 {
				t.Fatalf("pack-list claim = %+v", claim)
			}
		case sourceObjectMultiPackIndexChainPath:
			if len(claim.values) != 1 || claim.values[0] != auxiliaryP0SHA1 {
				t.Fatalf("midx chain claim = %+v", claim)
			}
		}
	}
	primitives.assertTransientDescriptorsClosedOnce(t)
}

func TestSourceObjectAuxiliaryAdmissionFailurePreservesPathOwner(t *testing.T) {
	for _, test := range []struct {
		name       string
		mutate     func(*metadataInventoryPrimitives)
		operation  Operation
		cause      CauseCode
		readPrefix string
	}{
		{
			name: "malformed chain",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(strings.ToUpper(auxiliaryP0SHA1)+"\n"),
				)
			},
			operation:  OperationParse,
			cause:      CauseMalformed,
			readPrefix: ".git/objects/info/commit-graphs/commit-graph-chain",
		},
		{
			name: "wrong primary trailer",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/graph-"+auxiliaryP0SHA1+".graph",
					append([]byte("fixture-body\n"), make([]byte, sha1.Size)...),
				)
			},
			operation:  OperationCompare,
			cause:      CauseIdentity,
			readPrefix: ".git/objects/info/commit-graphs/graph-" + auxiliaryP0SHA1 + ".graph",
		},
		{
			name: "chain names absent layer",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(auxiliaryP1SHA1+"\n"),
				)
			},
			operation:  OperationOpen,
			cause:      CauseNotFound,
			readPrefix: ".git/objects/info/commit-graphs/commit-graph-chain",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, primitives := newSourceObjectAuxiliaryAdmissionHarness(t)
			test.mutate(primitives)
			builder := exerciseMetadataInventory(owner, primitives)
			if !builder.outcome.proved() || !builder.retainSourceObjectPathInventory() {
				t.Fatalf("path retention = %+v", builder.outcome)
			}
			if builder.retainSourceObjectAuxiliaryClaimInventory() {
				t.Fatal("failed auxiliary admission retained claims")
			}
			requireFailureRecord(
				t,
				builder.outcome.primary,
				PhaseSource,
				test.operation,
				test.cause,
			)
			if owner.objects.claims != nil || !owner.validObjectPathRetention() {
				t.Fatalf("failed admission mutated path owner: %+v", owner)
			}
			if !sourceConstructionEventPresent(primitives.events, "read:"+test.readPrefix) {
				t.Fatalf("failed admission did not read %q: %q", test.readPrefix, primitives.events)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}
}

func TestSourceObjectAuxiliaryRevalidation(t *testing.T) {
	for _, test := range []struct {
		name             string
		prepare          func(*metadataInventoryPrimitives)
		mutate           func(*metadataInventoryPrimitives)
		operation        Operation
		cause            CauseCode
		readsAuxiliary   bool
		untraversedAdded string
	}{
		{name: "unchanged exact rows", readsAuxiliary: true},
		{
			name: "reordered valid chain",
			prepare: func(primitives *metadataInventoryPrimitives) {
				primitives.addContentNode(
					".git/objects/info/commit-graphs/graph-"+auxiliaryP1SHA1+".graph",
					sourceObjectAuxiliaryTestPrimary(objectFormatSHA1, []byte("fixture-body-2\n")),
					32,
				)
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(auxiliaryP0SHA1+"\n"+auxiliaryP1SHA1+"\n"),
				)
			},
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(auxiliaryP1SHA1+"\n"+auxiliaryP0SHA1+"\n"),
				)
			},
			operation:      OperationCompare,
			cause:          CauseUnstable,
			readsAuxiliary: true,
		},
		{
			name: "same identity opaque bytes",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/pack/multi-pack-index.d/multi-pack-index-" +
					auxiliaryP0SHA1 + ".rev"
				content := append([]byte(nil), primitives.contents[path]...)
				content[0] ^= 0x01
				primitives.setContentNode(path, content)
			},
			operation:      OperationCompare,
			cause:          CauseUnstable,
			readsAuxiliary: true,
		},
		{
			name: "same size valid primary bytes",
			prepare: func(primitives *metadataInventoryPrimitives) {
				primitives.removeNode(
					".git/objects/pack/multi-pack-index-" + auxiliaryP1SHA1 + ".bitmap",
				)
			},
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/pack/multi-pack-index",
					sourceObjectAuxiliaryTestPrimary(
						objectFormatSHA1,
						[]byte("fixture-body-3\n"),
					),
				)
			},
			operation:      OperationCompare,
			cause:          CauseUnstable,
			readsAuxiliary: true,
		},
		{
			name: "size change",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/pack/multi-pack-index.d/multi-pack-index-" +
					auxiliaryP0SHA1 + ".rev"
				primitives.setContentNode(path, append(primitives.contents[path], 'x'))
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "added row",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.addContentNode(".git/objects/info/unexpected", []byte("x"), 33)
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "added directory is not traversed",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.addDirectoryNode(".git/objects/unexpected", 34)
				primitives.addContentNode(".git/objects/unexpected/child", []byte("x"), 35)
			},
			operation:        OperationCompare,
			cause:            CauseUnstable,
			untraversedAdded: ".git/objects/unexpected",
		},
		{
			name: "removed row",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.removeNode(
					".git/objects/pack/multi-pack-index.d/multi-pack-index-" +
						auxiliaryP0SHA1 + ".rev",
				)
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "replaced row",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/pack/multi-pack-index.d/multi-pack-index-" +
					auxiliaryP0SHA1 + ".rev"
				node := primitives.nodes[path]
				node.evidence.snapshot.identity.Inode++
				primitives.nodes[path] = node
			},
			operation: OperationCompare,
			cause:     CauseIdentity,
		},
		{
			name: "kind drift",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/pack/multi-pack-index.d/multi-pack-index-" +
					auxiliaryP0SHA1 + ".rev"
				node := primitives.nodes[path]
				node.kind = sourceObservedDirectory
				node.evidence.snapshot.identity.Mode = platformModeDirectory | 0o700
				node.evidence.snapshot.linkCount = 2
				primitives.nodes[path] = node
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "security evidence drift",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/pack/multi-pack-index.d/multi-pack-index-" +
					auxiliaryP0SHA1 + ".rev"
				node := primitives.nodes[path]
				node.evidence.snapshot.identity.Mode ^= 0o020
				primitives.nodes[path] = node
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, primitives := newSourceObjectAuxiliaryAdmissionHarness(t)
			if test.prepare != nil {
				test.prepare(primitives)
			}
			builder := retainSourceObjectAuxiliaryTestOwner(t, owner, primitives)
			administration := owner.git.administration
			paths := owner.objects.paths
			claims := owner.objects.claims
			claimSnapshot := cloneSourceObjectAuxiliaryTestClaims(claims)
			if test.mutate != nil {
				test.mutate(primitives)
			}
			readsBefore := primitives.contentCalls
			got := builder.revalidateSourceObjectAuxiliaryClaimInventory()
			if test.operation == "" {
				if !got || !builder.outcome.proved() {
					t.Fatalf("unchanged revalidation = %v / %+v", got, builder.outcome)
				}
			} else {
				if got {
					t.Fatal("changed auxiliary source revalidated")
				}
				requireFailureRecord(
					t,
					builder.outcome.primary,
					PhaseSource,
					test.operation,
					test.cause,
				)
			}
			if test.readsAuxiliary == (primitives.contentCalls == readsBefore) {
				t.Fatalf(
					"auxiliary reads changed from %d to %d, want reads=%v",
					readsBefore,
					primitives.contentCalls,
					test.readsAuxiliary,
				)
			}
			if test.untraversedAdded != "" && sourceConstructionEventPresent(
				primitives.events,
				"walk:"+test.untraversedAdded,
			) {
				t.Fatalf("added directory was traversed: %q", primitives.events)
			}
			if owner.git.administration != administration || owner.objects.paths != paths ||
				owner.objects.claims != claims ||
				!reflect.DeepEqual(owner.objects.claims, claimSnapshot) {
				t.Fatalf("revalidation mutated sealed owner: %+v", owner)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}
}

func TestSourceObjectAuxiliaryFailureAttribution(t *testing.T) {
	for _, test := range []struct {
		name           string
		mutate         func(*metadataInventoryPrimitives)
		operation      Operation
		cause          CauseCode
		readsAuxiliary bool
	}{
		{
			name: "non-chain bound",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/pack/multi-pack-index.d/multi-pack-index-" +
					auxiliaryP0SHA1 + ".rev"
				node := primitives.nodes[path]
				node.evidence.snapshot.size = int64(maxSourceObjectAuxiliaryBytes) + 1
				primitives.nodes[path] = node
			},
			operation:      OperationWalk,
			cause:          CauseLimit,
			readsAuxiliary: true,
		},
		{
			name: "chain byte bound",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/info/commit-graphs/commit-graph-chain"
				node := primitives.nodes[path]
				node.evidence.snapshot.size = int64(
					maxSourceObjectAuxiliaryRecords*uint64(objectFormatSHA1.hexWidth()+1) + 1,
				)
				primitives.nodes[path] = node
			},
			operation: OperationParse,
			cause:     CauseLimit,
		},
		{
			name: "decoded record bound",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/packs",
					bytes.Repeat([]byte{'\n'}, int(maxSourceObjectPackListRecords+1)),
				)
			},
			operation:      OperationParse,
			cause:          CauseLimit,
			readsAuxiliary: true,
		},
		{
			name: "malformed grammar before manifest",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(strings.ToUpper(auxiliaryP0SHA1)+"\n"),
				)
			},
			operation:      OperationParse,
			cause:          CauseMalformed,
			readsAuxiliary: true,
		},
		{
			name: "short primary",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/graph-"+auxiliaryP0SHA1+".graph",
					[]byte("short"),
				)
			},
			operation:      OperationParse,
			cause:          CauseMalformed,
			readsAuxiliary: true,
		},
		{
			name: "named member absent",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(auxiliaryP1SHA1+"\n"),
				)
			},
			operation:      OperationOpen,
			cause:          CauseNotFound,
			readsAuxiliary: true,
		},
		{
			name: "malformed primary precedes forbidden coexistence",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.addContentNode(
					".git/objects/info/commit-graph",
					[]byte("short"),
					36,
				)
			},
			operation:      OperationParse,
			cause:          CauseMalformed,
			readsAuxiliary: true,
		},
		{
			name: "missing chain member precedes forbidden coexistence",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(auxiliaryP1SHA1+"\n"),
				)
				primitives.addContentNode(
					".git/objects/info/commit-graph",
					sourceObjectAuxiliaryTestPrimary(objectFormatSHA1, []byte("fixture-body\n")),
					36,
				)
			},
			operation:      OperationOpen,
			cause:          CauseNotFound,
			readsAuxiliary: true,
		},
		{
			name: "forbidden coexistence",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.addContentNode(
					".git/objects/info/commit-graph",
					sourceObjectAuxiliaryTestPrimary(objectFormatSHA1, []byte("fixture-body\n")),
					36,
				)
			},
			operation:      OperationValidate,
			cause:          CauseUnsupported,
			readsAuxiliary: true,
		},
		{
			name: "checksum mismatch before manifest",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/info/commit-graphs/graph-" + auxiliaryP0SHA1 + ".graph"
				content := append([]byte(nil), primitives.contents[path]...)
				content[len(content)-1] ^= 0x01
				primitives.setContentNode(path, content)
			},
			operation:      OperationCompare,
			cause:          CauseIdentity,
			readsAuxiliary: true,
		},
		{
			name: "filename checksum precedes later malformed chain",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/pack/multi-pack-index.d/multi-pack-index-"+
						auxiliaryP0SHA1+".midx",
					sourceObjectAuxiliaryTestPrimary(
						objectFormatSHA1,
						[]byte("fixture-body-2\n"),
					),
				)
				primitives.setContentNode(
					".git/objects/pack/multi-pack-index.d/multi-pack-index-chain",
					[]byte(strings.ToUpper(auxiliaryP0SHA1)+"\n"),
				)
			},
			operation:      OperationCompare,
			cause:          CauseIdentity,
			readsAuxiliary: true,
		},
	} {
		t.Run("admission "+test.name, func(t *testing.T) {
			owner, primitives := newSourceObjectAuxiliaryAdmissionHarness(t)
			test.mutate(primitives)
			builder := exerciseMetadataInventory(owner, primitives)
			if builder.outcome.proved() {
				if builder.retainSourceObjectPathInventory() {
					builder.retainSourceObjectAuxiliaryClaimInventory()
				}
			}
			requireFailureRecord(
				t,
				builder.outcome.primary,
				PhaseSource,
				test.operation,
				test.cause,
			)
			if owner.objects.claims != nil {
				t.Fatalf("failed attribution installed claims: %+v", owner.objects.claims)
			}
			if test.readsAuxiliary != (primitives.contentCalls > 0) {
				t.Fatalf("content calls = %d, want reads=%v", primitives.contentCalls, test.readsAuxiliary)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}

	for _, test := range []struct {
		name           string
		mutate         func(*metadataInventoryPrimitives)
		operation      Operation
		cause          CauseCode
		readsAuxiliary bool
	}{
		{
			name: "topology drift precedes malformed bytes",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.addContentNode(".git/objects/info/unexpected", []byte("x"), 37)
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(strings.ToUpper(auxiliaryP0SHA1)+"\n"),
				)
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "malformed bytes precede retained manifest comparison",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(strings.ToUpper(auxiliaryP0SHA1)+"\n"),
				)
			},
			operation:      OperationParse,
			cause:          CauseMalformed,
			readsAuxiliary: true,
		},
		{
			name: "checksum precedes retained manifest comparison",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/info/commit-graphs/graph-" + auxiliaryP0SHA1 + ".graph"
				content := append([]byte(nil), primitives.contents[path]...)
				content[len(content)-1] ^= 0x01
				primitives.setContentNode(path, content)
			},
			operation:      OperationCompare,
			cause:          CauseIdentity,
			readsAuxiliary: true,
		},
		{
			name: "closure precedes retained manifest comparison",
			mutate: func(primitives *metadataInventoryPrimitives) {
				primitives.setContentNode(
					".git/objects/info/commit-graphs/commit-graph-chain",
					[]byte(auxiliaryP1SHA1+"\n"),
				)
				path := ".git/objects/pack/multi-pack-index.d/multi-pack-index-" +
					auxiliaryP0SHA1 + ".rev"
				content := append([]byte(nil), primitives.contents[path]...)
				content[0] ^= 0x01
				primitives.setContentNode(path, content)
			},
			operation:      OperationOpen,
			cause:          CauseNotFound,
			readsAuxiliary: true,
		},
		{
			name: "replacement identity",
			mutate: func(primitives *metadataInventoryPrimitives) {
				path := ".git/objects/pack/multi-pack-index.d/multi-pack-index-" +
					auxiliaryP0SHA1 + ".rev"
				node := primitives.nodes[path]
				node.evidence.snapshot.identity.Inode++
				primitives.nodes[path] = node
			},
			operation: OperationCompare,
			cause:     CauseIdentity,
		},
	} {
		t.Run("revalidation "+test.name, func(t *testing.T) {
			owner, primitives := newSourceObjectAuxiliaryAdmissionHarness(t)
			builder := retainSourceObjectAuxiliaryTestOwner(t, owner, primitives)
			test.mutate(primitives)
			readsBefore := primitives.contentCalls
			if builder.revalidateSourceObjectAuxiliaryClaimInventory() {
				t.Fatal("failure-attribution drift revalidated")
			}
			requireFailureRecord(
				t,
				builder.outcome.primary,
				PhaseSource,
				test.operation,
				test.cause,
			)
			if test.readsAuxiliary == (primitives.contentCalls == readsBefore) {
				t.Fatalf("content calls changed from %d to %d, want reads=%v", readsBefore, primitives.contentCalls, test.readsAuxiliary)
			}
			primitives.assertTransientDescriptorsClosedOnce(t)
		})
	}
}

func retainSourceObjectAuxiliaryTestOwner(
	t *testing.T,
	owner *sourceConstructionOwner,
	primitives *metadataInventoryPrimitives,
) *sourceConstructionBuilder {
	t.Helper()
	builder := exerciseMetadataInventory(owner, primitives)
	if !builder.outcome.proved() || !builder.retainSourceObjectPathInventory() ||
		!builder.retainSourceObjectAuxiliaryClaimInventory() ||
		!builder.outcome.proved() || !owner.validObjectClaimRetention() {
		t.Fatalf("retain auxiliary owner = %+v; owner %+v", builder.outcome, owner)
	}
	return builder
}

func cloneSourceObjectAuxiliaryTestClaims(
	claims *sourceObjectAuxiliaryClaimInventory,
) *sourceObjectAuxiliaryClaimInventory {
	if claims == nil {
		return nil
	}
	clone := &sourceObjectAuxiliaryClaimInventory{
		rows: append([]sourceObjectAuxiliaryClaim(nil), claims.rows...),
	}
	for index := range clone.rows {
		clone.rows[index].values = append([]string(nil), clone.rows[index].values...)
	}
	return clone
}

func newSourceObjectAuxiliaryAdmissionHarness(
	t *testing.T,
) (*sourceConstructionOwner, *metadataInventoryPrimitives) {
	t.Helper()
	owner, primitives := newMetadataInventoryHarness(t)
	primitives.addDirectoryNode(".git/objects/info", 20)
	primitives.addDirectoryNode(".git/objects/info/commit-graphs", 21)
	primitives.addDirectoryNode(".git/objects/pack/multi-pack-index.d", 22)

	primitives.addContentNode(
		".git/objects/info/commit-graphs/commit-graph-chain",
		[]byte(auxiliaryP0SHA1+"\n"),
		23,
	)
	primitives.addContentNode(
		".git/objects/info/commit-graphs/graph-"+auxiliaryP0SHA1+".graph",
		sourceObjectAuxiliaryTestPrimary(objectFormatSHA1, []byte("fixture-body\n")),
		24,
	)
	primitives.addContentNode(
		".git/objects/info/packs",
		[]byte("\nP pack-"+sourceObjectAuxiliaryTestPackHash+".pack\n"),
		25,
	)
	primitives.addContentNode(
		".git/objects/pack/pack-"+sourceObjectAuxiliaryTestPackHash+".rev",
		[]byte(strings.Repeat("opaque-reverse-index", 4_000)),
		26,
	)
	primitives.addContentNode(
		".git/objects/pack/multi-pack-index",
		sourceObjectAuxiliaryTestPrimary(objectFormatSHA1, []byte("fixture-body-2\n")),
		27,
	)
	primitives.addContentNode(
		".git/objects/pack/multi-pack-index-"+auxiliaryP1SHA1+".bitmap",
		nil,
		28,
	)
	primitives.addContentNode(
		".git/objects/pack/multi-pack-index.d/multi-pack-index-chain",
		[]byte(auxiliaryP0SHA1+"\n"),
		29,
	)
	primitives.addContentNode(
		".git/objects/pack/multi-pack-index.d/multi-pack-index-"+auxiliaryP0SHA1+".midx",
		sourceObjectAuxiliaryTestPrimary(objectFormatSHA1, []byte("fixture-body\n")),
		30,
	)
	primitives.addContentNode(
		".git/objects/pack/multi-pack-index.d/multi-pack-index-"+auxiliaryP0SHA1+".rev",
		[]byte("opaque incremental reverse index"),
		31,
	)
	return owner, primitives
}

func sourceObjectAuxiliaryTestPrimary(
	format repositoryObjectFormat,
	body []byte,
) []byte {
	content := append([]byte(nil), body...)
	if format == objectFormatSHA1 {
		digest := sha1.Sum(body)
		return append(content, digest[:]...)
	}
	digest := sha256.Sum256(body)
	return append(content, digest[:]...)
}

func (primitives *metadataInventoryPrimitives) setContentNode(path string, content []byte) {
	primitives.t.Helper()
	node, present := primitives.nodes[path]
	if !present || node.kind != sourceObservedRegular {
		primitives.t.Fatalf("metadata content node %q is unavailable", path)
	}
	node.evidence.snapshot.size = int64(len(content))
	primitives.nodes[path] = node
	primitives.contents[path] = append([]byte(nil), content...)
}

func (primitives *metadataInventoryPrimitives) removeNode(path string) {
	primitives.t.Helper()
	node, present := primitives.nodes[path]
	if !present || node.kind == sourceObservedDirectory {
		primitives.t.Fatalf("metadata removable node %q is unavailable", path)
	}
	separator := strings.LastIndexByte(path, '/')
	if separator < 0 {
		primitives.t.Fatalf("metadata removable node %q has no parent", path)
	}
	parent := path[:separator]
	name := path[separator+1:]
	names := primitives.directories[parent]
	found := false
	for index := range names {
		if names[index] != name {
			continue
		}
		primitives.directories[parent] = append(names[:index], names[index+1:]...)
		found = true
		break
	}
	if !found {
		primitives.t.Fatalf("metadata removable node %q is not linked", path)
	}
	delete(primitives.nodes, path)
	delete(primitives.contents, path)
}

func TestSourceObjectAuxiliaryOffsetReadContextAttribution(t *testing.T) {
	owner, primitives := newSourceObjectAuxiliaryAdmissionHarness(t)
	builder := exerciseMetadataInventory(owner, primitives)
	if !builder.outcome.proved() || !builder.retainSourceObjectPathInventory() {
		t.Fatalf("path retention = %+v", builder.outcome)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	descriptor := primitives.acquireDescriptor(
		".git/objects/pack/pack-"+sourceObjectAuxiliaryTestPackHash+".rev",
		sourceObservedRegular,
	)
	buffer := make([]byte, 8)
	requireAuxiliaryFailure(
		t,
		primitives.readExactAtForHash(ctx, descriptor, 64<<10, buffer),
		OperationHash,
		CauseCanceled,
	)
	if primitives.closeDescriptor(descriptor) {
		t.Fatal("test descriptor close failed")
	}
}

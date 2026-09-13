package buildauthority

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"
)

const validSourceConstructionConfig = "[core]\n\trepositoryformatversion = 0\n\tbare = false\n"

func TestSourceConstructionSuccessTraceAndShape(t *testing.T) {
	primitives := newScriptedSourceConstructionPrimitives(
		t,
		[]byte(validSourceConstructionConfig),
	)
	owner, outcome := retainSourceConstructionWith(
		context.Background(),
		testSourceRepositoryLocator(),
		primitives,
	)
	if owner == nil || !outcome.proved() {
		t.Fatalf("source construction = %+v / %+v, want retained owner / proof", owner, outcome)
	}
	if got, want := primitives.events, sourceConstructionAcquisitionEvents(); !reflect.DeepEqual(got, want) {
		t.Fatalf("source construction trace =\n%q\nwant\n%q", got, want)
	}

	assertSourceConstructionOwnerShape(t, owner)
	if owner.state != sourceConstructionActive || owner.repository == nil || owner.git == nil ||
		owner.config == nil || owner.objects == nil {
		t.Fatalf("successful construction retained incomplete roles: %+v", owner)
	}
	if owner.repository.path != "/repository" || len(owner.repository.pathClaims) != 2 {
		t.Fatalf("repository binding = %q / %d claims", owner.repository.path, len(owner.repository.pathClaims))
	}
	if owner.config.claim.objectFormat != objectFormatSHA1 ||
		owner.config.digest != Digest(sha256.Sum256([]byte(validSourceConstructionConfig))) {
		t.Fatalf("retained config claim = %+v / %x", owner.config.claim, owner.config.digest)
	}
	if !owner.packedRefs.valid() || owner.packedRefs.state != sourcePackedRefsUnresolved ||
		owner.packedRefs.leaf != nil {
		t.Fatalf("packed-refs slot = %+v, want explicit unresolved state", owner.packedRefs)
	}
	for _, event := range primitives.events {
		if strings.Contains(event, "packed-refs") {
			t.Fatalf("initial construction touched packed-refs: %q", event)
		}
	}

	owner.closeIntoWith(primitives, &outcome)
	if !outcome.proved() {
		t.Fatalf("successful construction close = %+v", outcome)
	}
	if got, want := primitives.events[len(sourceConstructionAcquisitionEvents()):],
		sourceConstructionOwnerCloseEvents(); !reflect.DeepEqual(got, want) {
		t.Fatalf("source owner close trace = %q, want %q", got, want)
	}
}

func TestSourceConstructionWalksAndClosesNestedRepositoryAncestry(t *testing.T) {
	primitives := newScriptedSourceConstructionPrimitives(
		t,
		[]byte(validSourceConstructionConfig),
	)
	primitives.locator = sourceRepositoryLocator{
		path: "/parent/repository",
		seal: validSourceRepositoryLocator,
	}
	owner, outcome := retainSourceConstructionWith(
		context.Background(),
		primitives.locator,
		primitives,
	)
	if owner == nil || !outcome.proved() {
		t.Fatalf("nested source construction = %+v / %+v", owner, outcome)
	}
	if len(owner.repository.pathClaims) != 3 {
		t.Fatalf("nested repository claims = %d, want physical root plus two components", len(owner.repository.pathClaims))
	}
	wantSequence := []string{
		"open-descriptor:parent",
		"stat:parent",
		"statfs:parent",
		"validate-fs:parent",
		"acquire-acl:parent",
		"parse-acl:parent",
		"validate-acl:parent",
		"open-descriptor:repository",
		"stat:repository",
		"statfs:repository",
		"validate-fs:repository",
		"acquire-acl:repository",
		"parse-acl:repository",
		"validate-acl:repository",
		"compare:repository",
		"close-descriptor:parent",
		"close-descriptor:physical-root",
		"probe:git",
	}
	assertSourceConstructionEventSequence(t, primitives.events, wantSequence)
	if got := countSourceConstructionEvent(primitives.events, "close-descriptor:parent"); got != 1 {
		t.Fatalf("nested ancestry descriptor close count = %d; trace %q", got, primitives.events)
	}

	owner.closeIntoWith(primitives, &outcome)
	if !outcome.proved() {
		t.Fatalf("nested source close = %+v", outcome)
	}
	assertSourceConstructionAcquiredOwnersClosed(t, primitives)

	t.Run("failure closes transient stack before owned prefix", func(t *testing.T) {
		failed := newScriptedSourceConstructionPrimitives(
			t,
			[]byte(validSourceConstructionConfig),
		)
		failed.locator = primitives.locator
		failed.failureEvent = "stat:repository"
		failed.failure = newSourcePrimitiveFailure(OperationProbe, CauseUnstable)
		failedOwner, failedOutcome := retainSourceConstructionWith(
			context.Background(),
			failed.locator,
			failed,
		)
		if failedOwner != nil {
			t.Fatalf("nested failure returned source owner %+v", failedOwner)
		}
		requireFailureRecord(
			t,
			failedOutcome.primary,
			PhaseSource,
			OperationProbe,
			CauseUnstable,
		)
		assertSourceConstructionEventSequence(t, failed.events, []string{
			"stat:repository",
			"close-descriptor:parent",
			"close-descriptor:physical-root",
			"close-root:repository",
			"close-descriptor:repository",
		})
		assertSourceConstructionAcquiredOwnersClosed(t, failed)
	})

	t.Run("primary and transient close failure remain independent", func(t *testing.T) {
		failed := newScriptedSourceConstructionPrimitives(
			t,
			[]byte(validSourceConstructionConfig),
		)
		failed.locator = primitives.locator
		failed.failureEvent = "stat:repository"
		failed.failure = newSourcePrimitiveFailure(OperationProbe, CauseUnstable)
		failed.closeFailures["close-descriptor:parent"] = true
		failedOwner, failedOutcome := retainSourceConstructionWith(
			context.Background(),
			failed.locator,
			failed,
		)
		if failedOwner != nil {
			t.Fatalf("nested primary-plus-close failure returned source owner %+v", failedOwner)
		}
		requireFailureRecord(
			t,
			failedOutcome.primary,
			PhaseSource,
			OperationProbe,
			CauseUnstable,
		)
		requireFailureRecord(
			t,
			failedOutcome.descriptorClose,
			PhaseClose,
			OperationCloseNonRoot,
			CauseDescriptorClose,
		)
		assertSourceConstructionEventSequence(t, failed.events, []string{
			"stat:repository",
			"close-descriptor:parent",
			"close-descriptor:physical-root",
			"close-root:repository",
			"close-descriptor:repository",
		})
		if sourceConstructionEventPresent(failed.events, "probe:git") {
			t.Fatalf("nested primary-plus-close failure began git acquisition: %q", failed.events)
		}
		assertSourceConstructionAcquiredOwnersClosed(t, failed)
	})

	t.Run("intermediate owner plus failure closes exact nested prefix", func(t *testing.T) {
		failed := newScriptedSourceConstructionPrimitives(
			t,
			[]byte(validSourceConstructionConfig),
		)
		failed.locator = primitives.locator
		failed.ownerFailureEvent = "open-descriptor:parent"
		failed.failure = newSourcePrimitiveFailure(OperationOpen, CauseUnstable)
		failedOwner, failedOutcome := retainSourceConstructionWith(
			context.Background(),
			failed.locator,
			failed,
		)
		if failedOwner != nil {
			t.Fatalf("nested owner-plus-failure returned source owner %+v", failedOwner)
		}
		requireFailureRecord(
			t,
			failedOutcome.primary,
			PhaseSource,
			OperationOpen,
			CauseUnstable,
		)
		assertSourceConstructionEventSequence(t, failed.events, []string{
			"open-descriptor:parent",
			"close-descriptor:parent",
			"close-descriptor:physical-root",
			"close-root:repository",
		})
		assertSourceConstructionAcquiredOwnersClosed(t, failed)
	})

	t.Run("intermediate close failure continues through owned prefix", func(t *testing.T) {
		failed := newScriptedSourceConstructionPrimitives(
			t,
			[]byte(validSourceConstructionConfig),
		)
		failed.locator = primitives.locator
		failed.closeFailures["close-descriptor:parent"] = true
		failedOwner, failedOutcome := retainSourceConstructionWith(
			context.Background(),
			failed.locator,
			failed,
		)
		if failedOwner != nil || failedOutcome.primary != nil {
			t.Fatalf(
				"nested transient close failure = %+v / %+v, want nil owner and no primary",
				failedOwner,
				failedOutcome,
			)
		}
		requireFailureRecord(
			t,
			failedOutcome.descriptorClose,
			PhaseClose,
			OperationCloseNonRoot,
			CauseDescriptorClose,
		)
		assertSourceConstructionEventSequence(t, failed.events, []string{
			"compare:repository",
			"close-descriptor:parent",
			"close-descriptor:physical-root",
			"close-root:repository",
			"close-descriptor:repository",
		})
		if sourceConstructionEventPresent(failed.events, "probe:git") {
			t.Fatalf("nested transient close failure began git acquisition: %q", failed.events)
		}
		assertSourceConstructionAcquiredOwnersClosed(t, failed)
	})
}

func TestSourceConstructionRetainedPackedRefsClosesFirst(t *testing.T) {
	primitives := newScriptedSourceConstructionPrimitives(
		t,
		[]byte(validSourceConstructionConfig),
	)
	owner, acquisition := retainSourceConstructionWith(
		context.Background(),
		testSourceRepositoryLocator(),
		primitives,
	)
	if owner == nil || !acquisition.proved() {
		t.Fatalf("source construction = %+v / %+v", owner, acquisition)
	}
	assertNoPackedRefsEvents(t, primitives.events)

	packedEvidence := sourceDescriptorEvidence{
		snapshot:  primitives.snapshot("config"),
		mount:     sourceConstructionMount(),
		aclDigest: Digest(sha256.Sum256([]byte("packed-refs-acl"))),
	}
	packedDigest := Digest(sha256.Sum256([]byte("packed-refs")))
	packedClaim := packedRefsClaim{
		traits: []string{"fully-peeled"},
		records: []packedRefRecord{
			{objectID: strings.Repeat("a", 40), name: "refs/heads/main"},
		},
	}
	owner.packedRefs = sourcePackedRefsSlot{
		state: sourcePackedRefsRetained,
		leaf: &retainedSourcePackedRefs{
			descriptor: primitives.descriptors["packed-refs"],
			evidence:   packedEvidence,
			digest:     packedDigest,
			claim:      packedClaim,
		},
	}
	if !owner.packedRefs.valid() {
		t.Fatalf("synthetic retained packed-refs slot is invalid: %+v", owner.packedRefs)
	}
	packed := owner.packedRefs.leaf
	if packed.descriptor != primitives.descriptors["packed-refs"] ||
		packed.evidence != packedEvidence || packed.digest != packedDigest ||
		!reflect.DeepEqual(packed.claim, packedClaim) {
		t.Fatalf("synthetic retained packed-refs fields = %+v", packed)
	}

	start := len(primitives.events)
	var outcome sourceUseOutcome
	owner.closeIntoWith(primitives, &outcome)
	if !outcome.proved() {
		t.Fatalf("close with retained packed-refs = %+v", outcome)
	}
	want := append(
		[]string{"close-descriptor:packed-refs"},
		sourceConstructionOwnerCloseEvents()...,
	)
	if got := primitives.events[start:]; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained packed-refs close order = %q, want %q", got, want)
	}

	closedAt := len(primitives.events)
	var repeated sourceUseOutcome
	owner.closeIntoWith(primitives, &repeated)
	if !repeated.proved() || len(primitives.events) != closedAt {
		t.Fatalf(
			"repeated retained packed-refs close = %+v with trace %q",
			repeated,
			primitives.events[closedAt:],
		)
	}
}

func TestSourceConstructionRetainedPackedRefsCloseFailureContinuesAndCaches(t *testing.T) {
	primitives := newScriptedSourceConstructionPrimitives(
		t,
		[]byte(validSourceConstructionConfig),
	)
	primitives.packedRefsPresent = true
	primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
	owner, acquisition := retainSourceConstructionWith(
		context.Background(),
		testSourceRepositoryLocator(),
		primitives,
	)
	if owner == nil || !acquisition.proved() {
		t.Fatalf("source construction = %+v / %+v", owner, acquisition)
	}
	stage := runSourceConstructionPackedRefsStage(context.Background(), owner, primitives)
	if !stage.proved() || !owner.validPackedRefsRetention() {
		t.Fatalf("successful packed-refs stage = %+v; owner %+v", stage, owner)
	}

	primitives.closeFailures["close-descriptor:packed-refs"] = true
	start := len(primitives.events)
	var first sourceUseOutcome
	owner.closeIntoWith(primitives, &first)
	if first.primary != nil {
		t.Fatalf("retained packed-refs close primary = %+v", first.primary)
	}
	requireFailureRecord(
		t,
		first.descriptorClose,
		PhaseClose,
		OperationCloseNonRoot,
		CauseDescriptorClose,
	)
	want := append(
		[]string{"close-descriptor:packed-refs"},
		sourceConstructionOwnerCloseEvents()...,
	)
	if got := primitives.events[start:]; !reflect.DeepEqual(got, want) {
		t.Fatalf("failed retained packed-refs close order = %q, want %q", got, want)
	}
	if owner.state != sourceConstructionClosed || !owner.closeFailure {
		t.Fatalf("failed retained packed-refs close owner = %+v", owner)
	}

	closedAt := len(primitives.events)
	var repeated sourceUseOutcome
	owner.closeIntoWith(primitives, &repeated)
	if repeated.primary != nil {
		t.Fatalf("cached retained packed-refs close primary = %+v", repeated.primary)
	}
	requireFailureRecord(
		t,
		repeated.descriptorClose,
		PhaseClose,
		OperationCloseNonRoot,
		CauseDescriptorClose,
	)
	if len(primitives.events) != closedAt {
		t.Fatalf("cached retained packed-refs close touched handles: %q", primitives.events[closedAt:])
	}
	assertSourceConstructionAcquiredOwnersClosed(t, primitives)
}

func TestSourceConstructionPackedRefsStageRetainsAbsentAndPresentStates(t *testing.T) {
	sha1Content := []byte(
		"# pack-refs with: peeled fully-peeled sorted \n" +
			strings.Repeat("1", 40) + " refs/heads/main\n" +
			strings.Repeat("2", 40) + " refs/tags/v1\n" +
			"^" + strings.Repeat("a", 40) + "\n",
	)
	sha256Config := []byte("[core]\n\trepositoryformatversion = 1\n\tbare = false\n" +
		"[extensions]\n\tobjectFormat = sha256\n\trefStorage = files\n")
	sha256Content := []byte(strings.Repeat("b", 64) + " refs/heads/main\n")
	for _, test := range []struct {
		name    string
		config  []byte
		present bool
		content []byte
		format  repositoryObjectFormat
	}{
		{
			name:   "absent",
			config: []byte(validSourceConstructionConfig),
		},
		{
			name:    "empty present",
			config:  []byte(validSourceConstructionConfig),
			present: true,
			format:  objectFormatSHA1,
		},
		{
			name:    "SHA-1 header and peeled row",
			config:  []byte(validSourceConstructionConfig),
			present: true,
			content: sha1Content,
			format:  objectFormatSHA1,
		},
		{
			name:    "SHA-256",
			config:  sha256Config,
			present: true,
			content: sha256Content,
			format:  objectFormatSHA256,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(t, test.config)
			primitives.packedRefsPresent = test.present
			primitives.packedRefs = append([]byte(nil), test.content...)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() || !owner.validInitialRetention() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			start := len(primitives.events)
			stage := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			if !stage.proved() || !owner.validPackedRefsRetention() {
				t.Fatalf("packed-refs stage = %+v; owner %+v", stage, owner)
			}

			if !test.present {
				if owner.packedRefs.state != sourcePackedRefsAbsent || owner.packedRefs.leaf != nil {
					t.Fatalf("absent packed-refs state = %+v", owner.packedRefs)
				}
				want := []string{
					"probe:packed-refs:initial",
					"probe:packed-refs:rebind-absent",
				}
				if got := primitives.events[start:]; !reflect.DeepEqual(got, want) {
					t.Fatalf("absent packed-refs trace = %q, want %q", got, want)
				}
			} else {
				leaf := owner.packedRefs.leaf
				if owner.packedRefs.state != sourcePackedRefsRetained || leaf == nil ||
					!leaf.descriptor.validOpen() || leaf.evidence.snapshot.size != int64(len(test.content)) ||
					leaf.digest != Digest(sha256.Sum256(test.content)) {
					t.Fatalf("retained packed-refs = %+v", owner.packedRefs)
				}
				wantClaim, err := parsePackedRefs(test.content, test.format)
				if err != nil || !reflect.DeepEqual(leaf.claim, wantClaim) {
					t.Fatalf("retained packed-refs claim = %+v, want %+v / %v", leaf.claim, wantClaim, err)
				}
				want := []string{"probe:packed-refs:initial", "open-descriptor:packed-refs"}
				want = append(want, sourceConstructionObservationEvents("packed-refs-before")...)
				want = append(want, "read:packed-refs", "hash:packed-refs")
				want = append(want, sourceConstructionReobservationEvents("packed-refs-after")...)
				want = append(want, "open-descriptor:packed-refs-rebind")
				want = append(want, sourceConstructionReobservationEvents("packed-refs-rebind")...)
				want = append(want, "close-descriptor:packed-refs-rebind")
				if got := primitives.events[start:]; !reflect.DeepEqual(got, want) {
					t.Fatalf("retained packed-refs trace =\n%q\nwant\n%q", got, want)
				}
			}

			closeStart := len(primitives.events)
			var closeOutcome sourceUseOutcome
			owner.closeIntoWith(primitives, &closeOutcome)
			if !closeOutcome.proved() {
				t.Fatalf("packed-refs owner close = %+v", closeOutcome)
			}
			wantClose := sourceConstructionOwnerCloseEvents()
			if test.present {
				wantClose = append([]string{"close-descriptor:packed-refs"}, wantClose...)
			}
			if got := primitives.events[closeStart:]; !reflect.DeepEqual(got, wantClose) {
				t.Fatalf("packed-refs close trace = %q, want %q", got, wantClose)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}

}

func TestSourceConstructionPackedRefsParseAndPolicyAttribution(t *testing.T) {
	for _, test := range []struct {
		name      string
		content   []byte
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "syntax",
			content:   []byte(strings.Repeat("1", 40) + " refs/heads/main"),
			operation: OperationParse,
			cause:     CauseMalformed,
		},
		{
			name:      "replacement policy",
			content:   []byte(strings.Repeat("1", 40) + " refs/replace/target\n"),
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.packedRefsPresent = true
			primitives.packedRefs = append([]byte(nil), test.content...)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			stage := runSourceConstructionPackedRefsStage(context.Background(), owner, primitives)
			requireFailureRecord(t, stage.primary, PhaseSource, test.operation, test.cause)
			if stage.descriptorClose != nil || owner.state != sourceConstructionClosed {
				t.Fatalf("packed-refs refusal = %+v; owner %+v", stage, owner)
			}
			assertSourceConstructionEventSequence(t, primitives.events, []string{
				"read:packed-refs",
				"hash:packed-refs",
				"stat:packed-refs-after",
				"statfs:packed-refs-after",
				"acquire-acl:packed-refs-after",
				"parse-acl:packed-refs-after",
				"validate-fs:packed-refs-after",
				"validate-acl:packed-refs-after",
				"close-descriptor:packed-refs",
				"open-descriptor:packed-refs-rebind",
				"stat:packed-refs-rebind",
				"statfs:packed-refs-rebind",
				"acquire-acl:packed-refs-rebind",
				"parse-acl:packed-refs-rebind",
				"validate-fs:packed-refs-rebind",
				"validate-acl:packed-refs-rebind",
				"close-descriptor:packed-refs-rebind",
				"close-root:objects",
			})
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}

	t.Run("unknown object format is an invariant", func(t *testing.T) {
		_, failure := parseInitialSourcePackedRefs(
			context.Background(),
			nil,
			objectFormatUnknown,
		)
		requireFailureRecord(
			t,
			failure.record(),
			PhaseSource,
			OperationValidate,
			CauseInternalInvariant,
		)
	})
}

func TestSourceConstructionPackedRefsParseAndPolicyPostSampleContext(t *testing.T) {
	for _, test := range []struct {
		name      string
		content   string
		cancelAt  int
		operation Operation
	}{
		{
			name:      "parse cancellation outranks syntax error",
			content:   strings.Repeat("1", 40) + " refs/heads/main",
			cancelAt:  2,
			operation: OperationParse,
		},
		{
			name:      "policy cancellation outranks policy error",
			content:   strings.Repeat("1", 40) + " refs/replace/target\n",
			cancelAt:  4,
			operation: OperationValidate,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &sourceConstructionStepContext{cancelAt: test.cancelAt}
			_, failure := parseInitialSourcePackedRefs(
				ctx,
				[]byte(test.content),
				objectFormatSHA1,
			)
			requireFailureRecord(
				t,
				failure.record(),
				PhaseSource,
				test.operation,
				CauseCanceled,
			)
			if ctx.samples != test.cancelAt {
				t.Fatalf("context samples = %d, want cancellation at %d", ctx.samples, test.cancelAt)
			}
		})
	}
}

func TestSourceConstructionPackedRefsRejectsKindAndAbsenceDrift(t *testing.T) {
	for _, test := range []struct {
		name      string
		results   map[string]scriptedSourceProbeResult
		operation Operation
		cause     CauseCode
	}{
		{
			name: "present directory",
			results: map[string]scriptedSourceProbeResult{
				"probe:packed-refs:initial": {kind: sourceObservedDirectory, present: true},
			},
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
		{
			name: "malformed absent tuple",
			results: map[string]scriptedSourceProbeResult{
				"probe:packed-refs:initial": {kind: sourceObservedRegular},
			},
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name: "appears during absence rebind",
			results: map[string]scriptedSourceProbeResult{
				"probe:packed-refs:rebind-absent": {kind: sourceObservedRegular, present: true},
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			maps.Copy(primitives.probeResults, test.results)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			stage := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			requireFailureRecord(t, stage.primary, PhaseSource, test.operation, test.cause)
			if sourceConstructionEventPresent(primitives.events, "open-descriptor:packed-refs") {
				t.Fatalf("packed-refs kind or absence refusal opened a leaf: %q", primitives.events)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionPackedRefsRejectsUnsafeLeafEvidence(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause CauseCode
		apply func(*scriptedSourceConstructionPrimitives)
	}{
		{
			name:  "cross-device child",
			cause: CauseUnsupported,
			apply: func(primitives *scriptedSourceConstructionPrimitives) {
				primitives.deviceOverrides["packed-refs"] = 19
			},
		},
		{
			name:  "cross-filesystem child",
			cause: CauseUnsupported,
			apply: func(primitives *scriptedSourceConstructionPrimitives) {
				primitives.filesystemOverrides["packed-refs"] = [2]int32{19, 20}
			},
		},
		{
			name:  "hard linked leaf",
			cause: CauseUnsupported,
			apply: func(primitives *scriptedSourceConstructionPrimitives) {
				primitives.linkCountOverrides["packed-refs"] = 2
			},
		},
		{
			name:  "oversized leaf",
			cause: CauseLimit,
			apply: func(primitives *scriptedSourceConstructionPrimitives) {
				primitives.sizeOverrides["packed-refs"] = maxPackedRefsBytes + 1
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.packedRefsPresent = true
			primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
			test.apply(primitives)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			stage := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			requireFailureRecord(t, stage.primary, PhaseSource, OperationValidate, test.cause)
			if sourceConstructionEventPresent(primitives.events, "read:packed-refs") {
				t.Fatalf("unsafe packed-refs leaf was read: %q", primitives.events)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionReobservationComparesSecurityDriftBeforePolicy(t *testing.T) {
	for _, target := range []string{
		"config-after",
		"config-rebind",
		"packed-refs-after",
		"packed-refs-rebind",
	} {
		for _, drift := range []string{"mode", "acl", "mount"} {
			t.Run(target+"/"+drift, func(t *testing.T) {
				primitives := newScriptedSourceConstructionPrimitives(
					t,
					[]byte(validSourceConstructionConfig),
				)
				switch target {
				case "config-after":
					primitives.configPostReadDrift = drift
				case "config-rebind":
					primitives.configRebindDrift = drift
				case "packed-refs-after":
					primitives.packedRefsPresent = true
					primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
					primitives.packedRefsPostReadDrift = drift
				case "packed-refs-rebind":
					primitives.packedRefsPresent = true
					primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
					primitives.packedRefsRebindDrift = drift
				default:
					t.Fatalf("unknown reobservation target %q", target)
				}

				owner, outcome := retainSourceConstructionWith(
					context.Background(),
					testSourceRepositoryLocator(),
					primitives,
				)
				if strings.HasPrefix(target, "packed-refs-") {
					if owner == nil || !outcome.proved() {
						t.Fatalf("initial source construction = %+v / %+v", owner, outcome)
					}
					outcome = runSourceConstructionPackedRefsStage(
						context.Background(),
						owner,
						primitives,
					)
				} else if owner != nil {
					t.Fatalf("config %s drift returned source owner %+v", drift, owner)
				}
				requireFailureRecord(
					t,
					outcome.primary,
					PhaseSource,
					OperationCompare,
					CauseUnstable,
				)
				assertSourceConstructionEventSequence(t, primitives.events, []string{
					"stat:" + target,
					"statfs:" + target,
					"acquire-acl:" + target,
					"parse-acl:" + target,
				})
				for _, forbidden := range []string{
					"validate-fs:" + target,
					"validate-acl:" + target,
				} {
					if sourceConstructionEventPresent(primitives.events, forbidden) {
						t.Fatalf("%s drift reached policy event %q: %q", drift, forbidden, primitives.events)
					}
				}
				assertSourceConstructionAcquiredOwnersClosed(t, primitives)
			})
		}
	}
}

func TestSourceConstructionReobservationRejectsInvalidParsedACL(t *testing.T) {
	for _, target := range []string{
		"config-after",
		"config-rebind",
		"packed-refs-after",
		"packed-refs-rebind",
	} {
		t.Run(target, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.invalidACLEvent = "parse-acl:" + target
			if strings.HasPrefix(target, "packed-refs-") {
				primitives.packedRefsPresent = true
				primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
			}
			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if strings.HasPrefix(target, "packed-refs-") {
				if owner == nil || !outcome.proved() {
					t.Fatalf("initial source construction = %+v / %+v", owner, outcome)
				}
				outcome = runSourceConstructionPackedRefsStage(
					context.Background(),
					owner,
					primitives,
				)
			} else if owner != nil {
				t.Fatalf("invalid %s ACL returned source owner %+v", target, owner)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				OperationValidate,
				CauseInternalInvariant,
			)
			if sourceConstructionEventPresent(primitives.events, "validate-fs:"+target) ||
				sourceConstructionEventPresent(primitives.events, "validate-acl:"+target) {
				t.Fatalf("invalid %s ACL reached policy validation: %q", target, primitives.events)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionPackedRefsInstallsOpenOwnersBeforeFailure(t *testing.T) {
	for _, event := range []string{
		"open-descriptor:packed-refs",
		"open-descriptor:packed-refs-rebind",
	} {
		t.Run(event, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.packedRefsPresent = true
			primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
			primitives.ownerFailureEvent = event
			primitives.failure = newSourcePrimitiveFailure(OperationOpen, CauseUnstable)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			stage := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			requireFailureRecord(
				t,
				stage.primary,
				PhaseSource,
				OperationOpen,
				CauseUnstable,
			)
			closeEvent := "close-descriptor:packed-refs"
			if event == "open-descriptor:packed-refs-rebind" {
				closeEvent = "close-descriptor:packed-refs-rebind"
			}
			if got := countSourceConstructionEvent(primitives.events, closeEvent); got != 1 {
				t.Fatalf("owner-plus-failure close %q count = %d; trace %q", closeEvent, got, primitives.events)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionPackedRefsRejectsNilOwnerWithoutFailure(t *testing.T) {
	for _, event := range []string{
		"open-descriptor:packed-refs",
		"open-descriptor:packed-refs-rebind",
	} {
		t.Run(event, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.packedRefsPresent = true
			primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
			primitives.nilOwnerEvent = event
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			stage := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			requireFailureRecord(
				t,
				stage.primary,
				PhaseSource,
				OperationValidate,
				CauseInternalInvariant,
			)
			if stage.descriptorClose != nil || owner.state != sourceConstructionClosed {
				t.Fatalf("nil packed-refs owner refusal = %+v; owner %+v", stage, owner)
			}
			wantTail := sourceConstructionOwnerCloseEvents()
			if event == "open-descriptor:packed-refs-rebind" {
				wantTail = append([]string{"close-descriptor:packed-refs"}, wantTail...)
			}
			assertSourceConstructionEventSequence(t, primitives.events, append([]string{event}, wantTail...))
			if got := countSourceConstructionEvent(
				primitives.events,
				"close-descriptor:packed-refs-rebind",
			); got != 0 {
				t.Fatalf("nil comparison owner close count = %d; trace %q", got, primitives.events)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionPackedRefsContentFailureRunsRequiredTail(t *testing.T) {
	for _, test := range []struct {
		name      string
		event     string
		operation Operation
		cause     CauseCode
	}{
		{name: "read", event: "read:packed-refs", operation: OperationParse, cause: CauseUnstable},
		{name: "hash", event: "hash:packed-refs", operation: OperationHash, cause: CauseUnstable},
		{name: "hash canceled", event: "hash:packed-refs", operation: OperationHash, cause: CauseCanceled},
		{
			name:      "post-observation",
			event:     "stat:packed-refs-after",
			operation: OperationProbe,
			cause:     CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.packedRefsPresent = true
			primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
			primitives.failureEvent = test.event
			primitives.failure = newSourcePrimitiveFailure(test.operation, test.cause)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			stage := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			requireFailureRecord(t, stage.primary, PhaseSource, test.operation, test.cause)
			assertSourceConstructionEventSequence(t, primitives.events, []string{
				"close-descriptor:packed-refs",
				"open-descriptor:packed-refs-rebind",
				"stat:packed-refs-rebind",
				"statfs:packed-refs-rebind",
				"acquire-acl:packed-refs-rebind",
				"parse-acl:packed-refs-rebind",
				"validate-fs:packed-refs-rebind",
				"validate-acl:packed-refs-rebind",
				"close-descriptor:packed-refs-rebind",
				"close-root:objects",
			})
			if test.event == "read:packed-refs" &&
				sourceConstructionEventPresent(primitives.events, "hash:packed-refs") {
				t.Fatalf("read failure began packed-refs hashing: %q", primitives.events)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionPackedRefsPrimitiveFailuresRespectTailBoundary(t *testing.T) {
	for _, event := range sourceConstructionPackedRefsPresentEvents() {
		if strings.HasPrefix(event, "close-") {
			continue
		}
		t.Run(strings.ReplaceAll(event, ":", "_"), func(t *testing.T) {
			operation := sourceConstructionEventOperation(t, event)
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.packedRefsPresent = true
			primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
			primitives.failureEvent = event
			primitives.failure = newSourcePrimitiveFailure(operation, CauseUnstable)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			stage := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			requireFailureRecord(t, stage.primary, PhaseSource, operation, CauseUnstable)
			contentTail := event == "read:packed-refs" || event == "hash:packed-refs" ||
				strings.Contains(event, "packed-refs-after")
			if contentTail {
				if !sourceConstructionEventPresent(
					primitives.events,
					"open-descriptor:packed-refs-rebind",
				) {
					t.Fatalf("content failure %q skipped packed-refs rebind: %q", event, primitives.events)
				}
			} else {
				assertOnlySourceCloseTailAfter(t, primitives.events, event)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionPackedRefsFailureAndCloseComposition(t *testing.T) {
	for _, test := range []struct {
		name         string
		primaryDrift string
		closeEvent   string
		primaryCause CauseCode
	}{
		{
			name:         "content identity drift plus retained close failure",
			primaryDrift: "identity",
			closeEvent:   "close-descriptor:packed-refs",
			primaryCause: CauseIdentity,
		},
		{
			name:       "rebind close only",
			closeEvent: "close-descriptor:packed-refs-rebind",
		},
		{
			name:         "rebind security drift plus comparison close failure",
			primaryDrift: "rebind-security",
			closeEvent:   "close-descriptor:packed-refs-rebind",
			primaryCause: CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.packedRefsPresent = true
			primitives.packedRefs = []byte(strings.Repeat("1", 40) + " refs/heads/main\n")
			switch test.primaryDrift {
			case "identity":
				primitives.packedRefsPostReadDrift = "identity"
			case "rebind-security":
				primitives.packedRefsRebindDrift = "security"
			case "":
			default:
				t.Fatalf("unknown test drift %q", test.primaryDrift)
			}
			primitives.closeFailures[test.closeEvent] = true
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			stage := runSourceConstructionPackedRefsStage(
				context.Background(),
				owner,
				primitives,
			)
			if test.primaryCause == "" {
				if stage.primary != nil {
					t.Fatalf("close-only packed-refs primary = %+v", stage.primary)
				}
			} else {
				requireFailureRecord(
					t,
					stage.primary,
					PhaseSource,
					OperationCompare,
					test.primaryCause,
				)
			}
			requireFailureRecord(
				t,
				stage.descriptorClose,
				PhaseClose,
				OperationCloseNonRoot,
				CauseDescriptorClose,
			)
			if got := countSourceConstructionEvent(primitives.events, test.closeEvent); got != 1 {
				t.Fatalf("packed-refs close %q count = %d; trace %q", test.closeEvent, got, primitives.events)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionPackedRefsRequiresUnresolvedActiveOwner(t *testing.T) {
	primitives := newScriptedSourceConstructionPrimitives(
		t,
		[]byte(validSourceConstructionConfig),
	)
	owner, initial := retainSourceConstructionWith(
		context.Background(),
		testSourceRepositoryLocator(),
		primitives,
	)
	if owner == nil || !initial.proved() {
		t.Fatalf("initial source construction = %+v / %+v", owner, initial)
	}
	first := runSourceConstructionPackedRefsStage(context.Background(), owner, primitives)
	if !first.proved() || owner.packedRefs.state != sourcePackedRefsAbsent {
		t.Fatalf("first packed-refs stage = %+v; owner %+v", first, owner)
	}
	start := len(primitives.events)
	second := runSourceConstructionPackedRefsStage(context.Background(), owner, primitives)
	requireFailureRecord(
		t,
		second.primary,
		PhaseSource,
		OperationValidate,
		CauseInternalInvariant,
	)
	if owner.state != sourceConstructionClosed {
		t.Fatalf("repeated packed-refs stage left owner active: %+v", owner)
	}
	for _, event := range primitives.events[start:] {
		if strings.HasPrefix(event, "probe:packed-refs") {
			t.Fatalf("repeated packed-refs stage touched the leaf: %q", primitives.events[start:])
		}
	}
	assertSourceConstructionAcquiredOwnersClosed(t, primitives)
}

func TestSourceConstructionPackedRefsContextBoundaries(t *testing.T) {
	for _, test := range []struct {
		name      string
		ctx       func() context.Context
		operation Operation
		cause     CauseCode
	}{
		{
			name: "canceled before stage",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			operation: OperationValidate,
			cause:     CauseCanceled,
		},
		{
			name: "deadline before stage",
			ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
				t.Cleanup(cancel)
				return ctx
			},
			operation: OperationValidate,
			cause:     CauseDeadline,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			owner, initial := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !initial.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, initial)
			}
			stage := runSourceConstructionPackedRefsStage(test.ctx(), owner, primitives)
			requireFailureRecord(t, stage.primary, PhaseSource, test.operation, test.cause)
			assertNoPackedRefsEvents(t, primitives.events)
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}

}

func TestSourceConstructionRejectsInvalidInputsBeforePrimitives(t *testing.T) {
	for _, test := range []struct {
		name    string
		ctx     context.Context
		locator sourceRepositoryLocator
	}{
		{
			name:    "nil context",
			locator: testSourceRepositoryLocator(),
		},
		{
			name:    "invalid locator",
			ctx:     context.Background(),
			locator: sourceRepositoryLocator{path: "/repository"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			owner, outcome := retainSourceConstructionWith(
				test.ctx,
				test.locator,
				primitives,
			)
			if owner != nil || len(primitives.events) != 0 {
				t.Fatalf(
					"invalid input returned owner %+v or invoked primitives %q",
					owner,
					primitives.events,
				)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				OperationValidate,
				CauseInternalInvariant,
			)
			if outcome.descriptorClose != nil {
				t.Fatalf("invalid input produced close failure %+v", outcome.descriptorClose)
			}
		})
	}

	owner, outcome := retainSourceConstructionWith(
		context.Background(),
		testSourceRepositoryLocator(),
		nil,
	)
	if owner != nil {
		t.Fatalf("nil primitives returned source owner %+v", owner)
	}
	requireFailureRecord(
		t,
		outcome.primary,
		PhaseSource,
		OperationValidate,
		CauseInternalInvariant,
	)
	if outcome.descriptorClose != nil {
		t.Fatalf("nil primitives produced close failure %+v", outcome.descriptorClose)
	}
}

func TestSourceConstructionCancellationAtPrimitiveBoundaries(t *testing.T) {
	for _, test := range []struct {
		name      string
		event     string
		operation Operation
	}{
		{name: "after git probe", event: "probe:git", operation: OperationValidate},
		{name: "after git ACL validation", event: "validate-acl:git", operation: OperationValidate},
		{name: "after config hash", event: "hash:config", operation: OperationHash},
		{name: "after objects comparison", event: "compare:objects", operation: OperationValidate},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.cancelEvent = test.event
			primitives.cancel = cancel

			owner, outcome := retainSourceConstructionWith(
				ctx,
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("cancellation after %q returned source owner %+v", test.event, owner)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				test.operation,
				CauseCanceled,
			)
			assertOnlySourceCloseTailAfter(t, primitives.events, test.event)
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionPureChecksPreferContextFailure(t *testing.T) {
	expired, expire := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer expire()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	for _, test := range []struct {
		name  string
		ctx   context.Context
		cause CauseCode
	}{
		{name: "canceled", ctx: canceled, cause: CauseCanceled},
		{name: "deadline", ctx: expired, cause: CauseDeadline},
	} {
		t.Run(test.name, func(t *testing.T) {
			checks := []struct {
				name      string
				operation Operation
				failure   *sourcePrimitiveFailure
			}{
				{
					name:      "kind",
					operation: OperationValidate,
					failure: requireInitialSourceKind(
						test.ctx,
						sourceObservedSpecial,
						false,
						sourceObservedDirectory,
					),
				},
				{
					name:      "child",
					operation: OperationValidate,
					failure: validateInitialSourceChild(
						test.ctx,
						sourceDescriptorEvidence{},
						sourceDescriptorEvidence{},
					),
				},
				{
					name:      "descriptor evidence",
					operation: OperationValidate,
					failure: func() *sourcePrimitiveFailure {
						_, failure := validateSourceDescriptorEvidence(
							test.ctx,
							fileSnapshot{},
							mountSnapshot{},
							parsedSourceACL{},
							sourceObservedDirectory,
							ownerEffectiveOnly,
							false,
							0,
						)
						return failure
					}(),
				},
				{
					name:      "comparison",
					operation: OperationCompare,
					failure: compareSourceDescriptorEvidence(
						test.ctx,
						sourceDescriptorEvidence{},
						sourceDescriptorEvidence{},
					),
				},
				{
					name:      "owner",
					operation: OperationValidate,
					failure:   validateInitialSourceOwner(test.ctx, nil),
				},
			}
			for _, check := range checks {
				t.Run(check.name, func(t *testing.T) {
					requireFailureRecord(
						t,
						check.failure.record(),
						PhaseSource,
						check.operation,
						test.cause,
					)
				})
			}
		})
	}
}

func TestSourceConstructionPureChecksPostSampleContext(t *testing.T) {
	for _, check := range []struct {
		name      string
		operation Operation
		run       func(context.Context) *sourcePrimitiveFailure
	}{
		{
			name:      "observation request",
			operation: OperationValidate,
			run: func(ctx context.Context) *sourcePrimitiveFailure {
				return validateSourceObservationRequest(ctx, nil, sourceObservedSpecial, -1)
			},
		},
		{
			name:      "kind",
			operation: OperationValidate,
			run: func(ctx context.Context) *sourcePrimitiveFailure {
				return requireInitialSourceKind(
					ctx,
					sourceObservedSpecial,
					false,
					sourceObservedDirectory,
				)
			},
		},
		{
			name:      "child",
			operation: OperationValidate,
			run: func(ctx context.Context) *sourcePrimitiveFailure {
				return validateInitialSourceChild(
					ctx,
					sourceDescriptorEvidence{},
					sourceDescriptorEvidence{},
				)
			},
		},
		{
			name:      "descriptor evidence",
			operation: OperationValidate,
			run: func(ctx context.Context) *sourcePrimitiveFailure {
				_, failure := validateSourceDescriptorEvidence(
					ctx,
					fileSnapshot{},
					mountSnapshot{},
					parsedSourceACL{},
					sourceObservedDirectory,
					ownerEffectiveOnly,
					false,
					0,
				)
				return failure
			},
		},
		{
			name:      "comparison",
			operation: OperationCompare,
			run: func(ctx context.Context) *sourcePrimitiveFailure {
				return compareSourceDescriptorEvidence(
					ctx,
					sourceDescriptorEvidence{},
					sourceDescriptorEvidence{},
				)
			},
		},
		{
			name:      "owner",
			operation: OperationValidate,
			run: func(ctx context.Context) *sourcePrimitiveFailure {
				return validateInitialSourceOwner(ctx, nil)
			},
		},
	} {
		t.Run(check.name, func(t *testing.T) {
			ctx := &sourceConstructionStepContext{cancelAt: 2}
			failure := check.run(ctx)
			requireFailureRecord(
				t,
				failure.record(),
				PhaseSource,
				check.operation,
				CauseCanceled,
			)
			if ctx.samples != 2 {
				t.Fatalf("context samples = %d, want post-operation cancellation at sample 2", ctx.samples)
			}
		})
	}
}

func TestSourceConstructionConfigParseAndPolicyPostSampleContext(t *testing.T) {
	for _, test := range []struct {
		name      string
		content   string
		cancelAt  int
		operation Operation
	}{
		{
			name:      "parse cancellation outranks syntax error",
			content:   "[core\nrepositoryformatversion = 0\n",
			cancelAt:  2,
			operation: OperationParse,
		},
		{
			name:      "policy cancellation outranks policy error",
			content:   "[core]\n\trepositoryformatversion = 0\n\tbare = true\n",
			cancelAt:  4,
			operation: OperationValidate,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &sourceConstructionStepContext{cancelAt: test.cancelAt}
			_, failure := parseInitialSourceConfig(ctx, []byte(test.content))
			requireFailureRecord(
				t,
				failure.record(),
				PhaseSource,
				test.operation,
				CauseCanceled,
			)
			if ctx.samples != test.cancelAt {
				t.Fatalf("context samples = %d, want cancellation at sample %d", ctx.samples, test.cancelAt)
			}
		})
	}
}

func TestSourceConstructionPrimitiveFailuresStopBeforeLaterAcquisition(t *testing.T) {
	for _, event := range sourceConstructionAcquisitionEvents() {
		if strings.HasPrefix(event, "close-") {
			continue
		}
		t.Run(strings.ReplaceAll(event, ":", "_"), func(t *testing.T) {
			operation := sourceConstructionEventOperation(t, event)
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.failureEvent = event
			primitives.failure = newSourcePrimitiveFailure(operation, CauseUnstable)

			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("failure at %q returned source owner %+v", event, owner)
			}
			requireFailureRecord(t, outcome.primary, PhaseSource, operation, CauseUnstable)
			assertOnlySourceCloseTailAfter(t, primitives.events, event)
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
			assertNoPackedRefsEvents(t, primitives.events)
		})
	}
}

func TestSourceConstructionInstallsOpenOwnersBeforeInterpretingFailure(t *testing.T) {
	wantClose := map[string]string{
		"open-root:repository":          "close-root:repository",
		"open-descriptor:physical-root": "close-descriptor:physical-root",
		"open-descriptor:repository":    "close-descriptor:repository",
		"open-root:git":                 "close-root:git",
		"open-descriptor:git":           "close-descriptor:git",
		"open-descriptor:config":        "close-descriptor:config",
		"open-descriptor:config-rebind": "close-descriptor:config-rebind",
		"open-root:objects":             "close-root:objects",
		"open-descriptor:objects":       "close-descriptor:objects",
	}
	for event, closeEvent := range wantClose {
		t.Run(strings.ReplaceAll(event, ":", "_"), func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.ownerFailureEvent = event
			primitives.failure = newSourcePrimitiveFailure(OperationOpen, CauseUnstable)

			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("owner-plus-failure at %q returned source owner %+v", event, owner)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				OperationOpen,
				CauseUnstable,
			)
			if got := countSourceConstructionEvent(primitives.events, closeEvent); got != 1 {
				t.Fatalf("owner-plus-failure close %q count = %d; trace %q", closeEvent, got, primitives.events)
			}
			assertOnlySourceCloseTailAfter(t, primitives.events, event)
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionRejectsNilOwnerWithoutFailure(t *testing.T) {
	for _, event := range []string{
		"open-root:repository",
		"open-descriptor:physical-root",
		"open-descriptor:repository",
		"open-root:git",
		"open-descriptor:git",
		"open-descriptor:config",
		"open-descriptor:config-rebind",
		"open-root:objects",
		"open-descriptor:objects",
	} {
		t.Run(strings.ReplaceAll(event, ":", "_"), func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.nilOwnerEvent = event

			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("nil owner and nil failure at %q returned source owner %+v", event, owner)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				OperationValidate,
				CauseInternalInvariant,
			)
			assertOnlySourceCloseTailAfter(t, primitives.events, event)
		})
	}
}

func TestSourceConstructionRejectsRequiredPresenceAndKindMismatch(t *testing.T) {
	for _, test := range []struct {
		name      string
		event     string
		result    scriptedSourceProbeResult
		cause     CauseCode
		forbidden string
	}{
		{
			name:      "required git absent",
			event:     "probe:git",
			result:    scriptedSourceProbeResult{kind: sourceObservedDirectory},
			cause:     CauseInternalInvariant,
			forbidden: "open-root:git",
		},
		{
			name:      "config has wrong kind",
			event:     "probe:config",
			result:    scriptedSourceProbeResult{kind: sourceObservedDirectory, present: true},
			cause:     CauseUnsupported,
			forbidden: "open-descriptor:config",
		},
		{
			name:      "objects has invalid kind",
			event:     "probe:objects",
			result:    scriptedSourceProbeResult{present: true},
			cause:     CauseInternalInvariant,
			forbidden: "open-root:objects",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.probeResults[test.event] = test.result

			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("invalid required entry returned source owner %+v", owner)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				OperationValidate,
				test.cause,
			)
			if sourceConstructionEventPresent(primitives.events, test.forbidden) {
				t.Fatalf("invalid required entry invoked %q: %q", test.forbidden, primitives.events)
			}
			assertOnlySourceCloseTailAfter(t, primitives.events, test.event)
		})
	}
}

func TestSourceConstructionRejectsCrossDeviceAndFilesystemChildren(t *testing.T) {
	for _, test := range []struct {
		name     string
		mutate   func(*scriptedSourceConstructionPrimitives)
		terminal string
		later    string
	}{
		{
			name: "git crosses device",
			mutate: func(primitives *scriptedSourceConstructionPrimitives) {
				primitives.deviceOverrides["git"] = 17
			},
			terminal: "validate-acl:git",
			later:    "compare:git",
		},
		{
			name: "objects crosses filesystem",
			mutate: func(primitives *scriptedSourceConstructionPrimitives) {
				primitives.filesystemOverrides["objects"] = [2]int32{17, 18}
			},
			terminal: "validate-acl:objects",
			later:    "compare:objects",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			test.mutate(primitives)
			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("cross-boundary source child returned owner %+v", owner)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				OperationValidate,
				CauseUnsupported,
			)
			if sourceConstructionEventPresent(primitives.events, test.later) {
				t.Fatalf("cross-boundary child invoked %q: %q", test.later, primitives.events)
			}
			assertOnlySourceCloseTailAfter(t, primitives.events, test.terminal)
		})
	}
}

func TestSourceConstructionRejectsMalformedParsedACLWithoutFakeFailure(t *testing.T) {
	primitives := newScriptedSourceConstructionPrimitives(
		t,
		[]byte(validSourceConstructionConfig),
	)
	primitives.invalidACLEvent = "parse-acl:config-before"
	owner, outcome := retainSourceConstructionWith(
		context.Background(),
		testSourceRepositoryLocator(),
		primitives,
	)
	if owner != nil {
		t.Fatalf("malformed parsed ACL returned source owner %+v", owner)
	}
	requireFailureRecord(
		t,
		outcome.primary,
		PhaseSource,
		OperationValidate,
		CauseInternalInvariant,
	)
	if sourceConstructionEventPresent(primitives.events, "read:config") {
		t.Fatalf("malformed parsed ACL allowed config read: %q", primitives.events)
	}
	assertOnlySourceCloseTailAfter(t, primitives.events, "validate-acl:config-before")
}

func TestSourceConstructionConfigParseAndPolicyAttribution(t *testing.T) {
	for _, test := range []struct {
		name      string
		content   string
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "syntax",
			content:   "[core\nrepositoryformatversion = 0\nbare = false\n",
			operation: OperationParse,
			cause:     CauseMalformed,
		},
		{
			name:      "policy",
			content:   "[core]\n\trepositoryformatversion = 0\n\tbare = true\n",
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(t, []byte(test.content))
			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("invalid config returned source owner %+v", owner)
			}
			requireFailureRecord(t, outcome.primary, PhaseSource, test.operation, test.cause)
			if sourceConstructionEventPresent(primitives.events, "stat:config-after") ||
				sourceConstructionEventPresent(primitives.events, "open-descriptor:config-rebind") ||
				sourceConstructionEventPresent(primitives.events, "probe:objects") {
				t.Fatalf("config %s failure ran a later independent primitive: %q", test.name, primitives.events)
			}
		})
	}
}

func TestSourceConstructionPostReadDriftAttribution(t *testing.T) {
	for _, test := range []struct {
		name  string
		drift string
		cause CauseCode
	}{
		{name: "identity", drift: "identity", cause: CauseIdentity},
		{name: "security", drift: "security", cause: CauseUnstable},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.configPostReadDrift = test.drift
			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("post-read %s drift returned source owner %+v", test.name, owner)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				OperationCompare,
				test.cause,
			)
			if !sourceConstructionEventPresent(primitives.events, "stat:config-after") ||
				sourceConstructionEventPresent(primitives.events, "open-descriptor:config-rebind") ||
				sourceConstructionEventPresent(primitives.events, "probe:objects") {
				t.Fatalf("post-read %s drift tail = %q", test.name, primitives.events)
			}
		})
	}
}

func TestSourceConstructionRebindDriftAttribution(t *testing.T) {
	for _, test := range []struct {
		name  string
		drift string
		cause CauseCode
	}{
		{name: "identity", drift: "identity", cause: CauseIdentity},
		{name: "security", drift: "security", cause: CauseUnstable},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.configRebindDrift = test.drift
			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("rebind %s drift returned source owner %+v", test.name, owner)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				OperationCompare,
				test.cause,
			)
			if !sourceConstructionEventPresent(primitives.events, "stat:config-rebind") ||
				!sourceConstructionEventPresent(primitives.events, "close-descriptor:config-rebind") ||
				sourceConstructionEventPresent(primitives.events, "probe:objects") {
				t.Fatalf("rebind %s drift tail = %q", test.name, primitives.events)
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionRebindCloseFailureComposition(t *testing.T) {
	for _, test := range []struct {
		name         string
		rebindDrift  string
		primaryCause CauseCode
	}{
		{name: "close only"},
		{name: "identity drift and close", rebindDrift: "identity", primaryCause: CauseIdentity},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			primitives.configRebindDrift = test.rebindDrift
			primitives.closeFailures["close-descriptor:config-rebind"] = true

			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner != nil {
				t.Fatalf("rebind close failure returned source owner %+v", owner)
			}
			if test.primaryCause == "" {
				if outcome.primary != nil {
					t.Fatalf("close-only rebind outcome primary = %+v, want nil", outcome.primary)
				}
			} else {
				requireFailureRecord(
					t,
					outcome.primary,
					PhaseSource,
					OperationCompare,
					test.primaryCause,
				)
			}
			requireFailureRecord(
				t,
				outcome.descriptorClose,
				PhaseClose,
				OperationCloseNonRoot,
				CauseDescriptorClose,
			)
			if count := countSourceConstructionEvent(
				primitives.events,
				"close-descriptor:config-rebind",
			); count != 1 {
				t.Fatalf("config rebind close count = %d; trace %q", count, primitives.events)
			}
			if sourceConstructionEventPresent(primitives.events, "probe:objects") {
				t.Fatalf("rebind close failure began objects acquisition: %q", primitives.events)
			}
			assertOnlySourceCloseTailAfter(
				t,
				primitives.events,
				"close-descriptor:config-rebind",
			)
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionCloseOrderContinuesAndCaches(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		primitives := newScriptedSourceConstructionPrimitives(
			t,
			[]byte(validSourceConstructionConfig),
		)
		owner, acquisition := retainSourceConstructionWith(
			context.Background(),
			testSourceRepositoryLocator(),
			primitives,
		)
		if owner == nil || !acquisition.proved() {
			t.Fatalf("source construction = %+v / %+v", owner, acquisition)
		}
		start := len(primitives.events)
		var first sourceUseOutcome
		owner.closeIntoWith(primitives, &first)
		if !first.proved() {
			t.Fatalf("first successful close = %+v", first)
		}
		if got, want := primitives.events[start:], sourceConstructionOwnerCloseEvents(); !reflect.DeepEqual(got, want) {
			t.Fatalf("successful close order = %q, want %q", got, want)
		}
		closedAt := len(primitives.events)
		var repeated sourceUseOutcome
		owner.closeIntoWith(primitives, &repeated)
		if !repeated.proved() || len(primitives.events) != closedAt {
			t.Fatalf("repeated successful close = %+v with trace %q", repeated, primitives.events[closedAt:])
		}
	})

	t.Run("root failure", func(t *testing.T) {
		primitives := newScriptedSourceConstructionPrimitives(
			t,
			[]byte(validSourceConstructionConfig),
		)
		owner, acquisition := retainSourceConstructionWith(
			context.Background(),
			testSourceRepositoryLocator(),
			primitives,
		)
		if owner == nil || !acquisition.proved() {
			t.Fatalf("source construction = %+v / %+v", owner, acquisition)
		}
		primitives.closeFailures["close-root:objects"] = true
		start := len(primitives.events)
		var first sourceUseOutcome
		owner.closeIntoWith(primitives, &first)
		requireFailureRecord(
			t,
			first.descriptorClose,
			PhaseClose,
			OperationCloseNonRoot,
			CauseDescriptorClose,
		)
		if got, want := primitives.events[start:], sourceConstructionOwnerCloseEvents(); !reflect.DeepEqual(got, want) {
			t.Fatalf("failed root close order = %q, want %q", got, want)
		}
		closedAt := len(primitives.events)
		var repeated sourceUseOutcome
		owner.closeIntoWith(primitives, &repeated)
		requireFailureRecord(
			t,
			repeated.descriptorClose,
			PhaseClose,
			OperationCloseNonRoot,
			CauseDescriptorClose,
		)
		if len(primitives.events) != closedAt {
			t.Fatalf("repeated failed close touched handles: %q", primitives.events[closedAt:])
		}
	})
}

func TestSourceConstructionConcurrentCloseIsExactOnce(t *testing.T) {
	for _, closeFails := range []bool{false, true} {
		name := "success"
		if closeFails {
			name = "cached failure"
		}
		t.Run(name, func(t *testing.T) {
			primitives := newScriptedSourceConstructionPrimitives(
				t,
				[]byte(validSourceConstructionConfig),
			)
			owner, acquisition := retainSourceConstructionWith(
				context.Background(),
				testSourceRepositoryLocator(),
				primitives,
			)
			if owner == nil || !acquisition.proved() {
				t.Fatalf("source construction = %+v / %+v", owner, acquisition)
			}
			if closeFails {
				primitives.closeFailures["close-root:objects"] = true
			}

			start := len(primitives.events)
			outcomes := make([]sourceUseOutcome, 2)
			ready := make(chan struct{})
			var wait sync.WaitGroup
			wait.Add(len(outcomes))
			for index := range outcomes {
				go func() {
					defer wait.Done()
					<-ready
					owner.closeIntoWith(primitives, &outcomes[index])
				}()
			}
			close(ready)
			wait.Wait()

			if got, want := primitives.events[start:], sourceConstructionOwnerCloseEvents(); !reflect.DeepEqual(got, want) {
				t.Fatalf("concurrent close trace = %q, want one exact trace %q", got, want)
			}
			for index := range outcomes {
				if closeFails {
					requireFailureRecord(
						t,
						outcomes[index].descriptorClose,
						PhaseClose,
						OperationCloseNonRoot,
						CauseDescriptorClose,
					)
				} else if !outcomes[index].proved() {
					t.Fatalf("concurrent close outcome %d = %+v", index, outcomes[index])
				}
			}
			assertSourceConstructionAcquiredOwnersClosed(t, primitives)
		})
	}
}

func TestSourceConstructionTransientCloseFailureStopsAcquisition(t *testing.T) {
	primitives := newScriptedSourceConstructionPrimitives(
		t,
		[]byte(validSourceConstructionConfig),
	)
	primitives.closeFailures["close-descriptor:physical-root"] = true
	owner, outcome := retainSourceConstructionWith(
		context.Background(),
		testSourceRepositoryLocator(),
		primitives,
	)
	if owner != nil || outcome.primary != nil {
		t.Fatalf("transient close failure = %+v / %+v, want nil owner and no primary", owner, outcome)
	}
	requireFailureRecord(
		t,
		outcome.descriptorClose,
		PhaseClose,
		OperationCloseNonRoot,
		CauseDescriptorClose,
	)
	assertOnlySourceCloseTailAfter(t, primitives.events, "close-descriptor:physical-root")
	if sourceConstructionEventPresent(primitives.events, "probe:git") {
		t.Fatalf("transient close failure began git acquisition: %q", primitives.events)
	}
}

func TestSourceConstructionRetainedGraphHasNoFunctionOrInterfaceFields(t *testing.T) {
	ownerType := reflect.TypeFor[sourceConstructionOwner]()
	assertNoSourceOwnerOpenSeam(
		t,
		ownerType.PkgPath(),
		ownerType,
		ownerType.Name(),
		map[reflect.Type]bool{},
	)
}

func TestSourceConstructionHasNoPrematureOwnershipTransferSurface(t *testing.T) {
	type parsedProductionFile struct {
		name         string
		construction bool
		parsed       *ast.File
	}
	allowedOwnerResults := map[string]bool{
		"newSourceConstructionOwner":   true,
		"retainSourceConstruction":     true,
		"retainSourceConstructionWith": true,
	}
	files := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read buildauthority package: %v", err)
	}
	parsedFiles := make([]parsedProductionFile, 0, len(entries))
	sealedDeclarations := make(map[string]bool)
	matched := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(files, name, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		constructionFile := strings.HasPrefix(name, "source_construction")
		if constructionFile {
			matched++
			for _, declaration := range parsed.Decls {
				switch current := declaration.(type) {
				case *ast.FuncDecl:
					if current.Recv == nil {
						sealedDeclarations[current.Name.Name] = true
					}
				case *ast.GenDecl:
					for _, specification := range current.Specs {
						switch currentSpec := specification.(type) {
						case *ast.TypeSpec:
							sealedDeclarations[currentSpec.Name.Name] = true
						case *ast.ValueSpec:
							for _, identifier := range currentSpec.Names {
								sealedDeclarations[identifier.Name] = true
							}
						}
					}
				}
			}
		}
		parsedFiles = append(parsedFiles, parsedProductionFile{
			name:         name,
			construction: constructionFile,
			parsed:       parsed,
		})
	}
	if matched != 3 {
		t.Fatalf("source construction governance matched %d production files, want 3", matched)
	}

	for _, source := range parsedFiles {
		if !source.construction {
			ast.Inspect(source.parsed, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if ok && sealedDeclarations[identifier.Name] {
					t.Fatalf(
						"%s references sealed source-construction declaration %s outside its module",
						files.Position(identifier.Pos()),
						identifier.Name,
					)
				}
				return true
			})
			continue
		}

		for _, declaration := range source.parsed.Decls {
			switch current := declaration.(type) {
			case *ast.FuncDecl:
				if sourceConstructionTransferName(current.Name.Name) {
					t.Fatalf(
						"%s declares premature source ownership surface %s",
						files.Position(current.Name.Pos()),
						current.Name.Name,
					)
				}
				if escaped := sourceConstructionEscapeResult(current.Type.Results); escaped != "" &&
					(escaped != "sourceConstructionOwner" || current.Recv != nil ||
						!allowedOwnerResults[current.Name.Name]) {
					t.Fatalf(
						"%s returns sealed source ownership or raw handle type %s",
						files.Position(current.Name.Pos()),
						escaped,
					)
				}
			case *ast.GenDecl:
				if current.Tok == token.VAR {
					t.Fatalf(
						"%s declares package storage inside the source-construction module",
						files.Position(current.Pos()),
					)
				}
				for _, specification := range current.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if sourceConstructionTransferName(typeSpec.Name.Name) {
						t.Fatalf(
							"%s declares premature source ownership type %s",
							files.Position(typeSpec.Name.Pos()),
							typeSpec.Name.Name,
						)
					}
					structure, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					for _, field := range structure.Fields.List {
						for _, fieldName := range field.Names {
							if sourceConstructionTransferName(fieldName.Name) {
								t.Fatalf(
									"%s declares premature source ownership field %s",
									files.Position(fieldName.Pos()),
									fieldName.Name,
								)
							}
						}
					}
				}
			}
		}
	}
}

func TestSourceConstructionEscapeTypeClassifier(t *testing.T) {
	for _, test := range []struct {
		expression string
		want       string
	}{
		{expression: "*os.File", want: "os.File"},
		{expression: "*os.Root", want: "os.Root"},
		{expression: "[]*ownedSourceDescriptor", want: "ownedSourceDescriptor"},
		{expression: "map[string]retainedSourceConfig", want: "retainedSourceConfig"},
		{expression: "*sourceConstructionOwner", want: "sourceConstructionOwner"},
		{expression: "func() *os.File", want: "func"},
		{expression: "interface{ Raw() *os.File }", want: "interface"},
		{expression: "sourceUseOutcome"},
		{expression: "*sourcePrimitiveFailure"},
	} {
		t.Run(strings.ReplaceAll(test.expression, " ", "_"), func(t *testing.T) {
			expression, err := parser.ParseExpr(test.expression)
			if err != nil {
				t.Fatalf("parse classifier expression %q: %v", test.expression, err)
			}
			if got := sourceConstructionEscapeType(expression); got != test.want {
				t.Fatalf("escape type for %q = %q, want %q", test.expression, got, test.want)
			}
		})
	}
}

func TestSourceConstructionTypeResolvedEscapeGuard(t *testing.T) {
	typedPackage := loadTypedSourceConstructionPackage(t, nil)
	if violations := sourceConstructionEscapeViolations(typedPackage); len(violations) != 0 {
		t.Fatalf("type-resolved source-construction escape violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestUnsupportedSourcePrimitiveSeam(t *testing.T) {
	source, err := os.ReadFile("source_primitives_unsupported.go")
	if err != nil {
		t.Fatalf("read unsupported source primitive seam: %v", err)
	}
	if violations := sourceConstructionUnsupportedPrimitiveViolations(source); len(violations) != 0 {
		t.Fatalf("unsupported source primitive seam violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestUnsupportedSourcePrimitiveSeamRejectsDrift(t *testing.T) {
	source, err := os.ReadFile("source_primitives_unsupported.go")
	if err != nil {
		t.Fatalf("read unsupported source primitive fixture: %v", err)
	}
	for _, test := range []struct {
		name        string
		original    string
		replacement string
		want        string
	}{
		{
			name:        "target complement narrows",
			original:    "//go:build !darwin || !arm64",
			replacement: "//go:build !darwin",
			want:        "is not the exact complement of darwin and arm64",
		},
		{
			name:     "package initializer side effect",
			original: "package buildauthority",
			replacement: `package buildauthority

import "os"

var sourceConstructionFixtureUnsupportedInit = os.Chmod("/definitely/missing", 0o777)`,
			want: "must contain no imports",
		},
		{
			name:     "panic before unsupported result",
			original: "\treturn nil, newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)",
			replacement: `	panic("fixture")
	return nil, newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)`,
			want: "must contain one return statement",
		},
		{
			name:        "wrong failure cause",
			original:    "CauseUnsupported",
			replacement: "CauseInternalInvariant",
			want:        "must return the exact unsupported failure",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if count := strings.Count(string(source), test.original); count != 1 {
				t.Fatalf("fixture mutation count = %d, want 1", count)
			}
			fixture := []byte(strings.Replace(string(source), test.original, test.replacement, 1))
			joined := strings.Join(sourceConstructionUnsupportedPrimitiveViolations(fixture), "\n")
			if !strings.Contains(joined, test.want) {
				t.Fatalf("unsupported source primitive violations =\n%s\nwant %q", joined, test.want)
			}
		})
	}
}

func TestSourceConstructionResolvedCallClosure(t *testing.T) {
	typedPackage := loadTypedDarwinSourceConstructionPackage(t, nil)
	if violations := sourceConstructionResolvedCallClosureViolations(typedPackage); len(violations) != 0 {
		t.Fatalf("resolved source-construction call-closure violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestSourceConstructionResolvedCallClosureRejectsBypasses(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	inventoryPath := filepath.Join(workingDirectory, "source_construction_inventory.go")
	inventorySource, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatalf("read source inventory fixture base: %v", err)
	}
	authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
	authoritySource, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatalf("read Darwin authority fixture base: %v", err)
	}
	darwinPrimitivesPath := filepath.Join(workingDirectory, "source_primitives_darwin.go")
	darwinPrimitivesSource, err := os.ReadFile(darwinPrimitivesPath)
	if err != nil {
		t.Fatalf("read Darwin source primitives fixture base: %v", err)
	}
	acquirePath := filepath.Join(workingDirectory, "source_construction_acquire.go")
	acquireSource, err := os.ReadFile(acquirePath)
	if err != nil {
		t.Fatalf("read source acquisition fixture base: %v", err)
	}

	for _, test := range []struct {
		name                        string
		inventoryExtra              string
		inventoryOriginal           string
		inventoryReplacement        string
		inventoryCount              int
		authorityExtra              string
		authorityNeedsUnsafe        bool
		darwinPrimitivesOriginal    string
		darwinPrimitivesReplacement string
		darwinPrimitivesCount       int
		darwinPrimitivesExtra       string
		acquireOriginal             string
		acquireReplacement          string
		acquireCount                int
		want                        []string
	}{
		{
			name: "cross file helper reads an arbitrary path",
			inventoryExtra: `

func sourceConstructionFixtureInvokeHiddenPathRead() bool {
	return sourceConstructionFixtureHiddenPathRead()
}
`,
			authorityExtra: `

func sourceConstructionFixtureHiddenPathRead() bool {
	content, _ := os.ReadFile("/etc/passwd")
	return len(content) != 0
}
`,
			want: []string{
				"authority_darwin.go|sourceConstructionFixtureHiddenPathRead|os|ReadFile",
			},
		},
		{
			name: "bodyless package seam",
			inventoryExtra: `

func sourceConstructionFixtureInvokeBodyless() int64 {
	return sourceConstructionFixtureBodyless()
}
`,
			authorityExtra: `

//go:linkname sourceConstructionFixtureBodyless runtime.nanotime
func sourceConstructionFixtureBodyless() int64
`,
			authorityNeedsUnsafe: true,
			want: []string{
				"bodyless package callee source_construction_inventory.go|" +
					"sourceConstructionFixtureInvokeBodyless|github.com/vbonnet/dear-agent/internal/buildauthority|" +
					"sourceConstructionFixtureBodyless",
			},
		},
		{
			name: "package initializer side effect",
			darwinPrimitivesExtra: `

var sourceConstructionFixtureInitErr = os.Chmod("/definitely/missing", 0o777)
`,
			want: []string{
				"source call closure rejects package variable declaration in source_primitives_darwin.go",
			},
		},
		{
			name:                        "blank imported initializer side effect",
			darwinPrimitivesOriginal:    "\t\"context\"\n",
			darwinPrimitivesReplacement: "\t\"context\"\n\t_ \"net/http/pprof\"\n",
			darwinPrimitivesCount:       1,
			want: []string{
				"source package import is outside audited initialization set " +
					"source_primitives_darwin.go|_|net/http/pprof",
			},
		},
		{
			name: "non module package variable opens a handle",
			authorityExtra: `

var sourceConstructionFixtureLeakedFile, _ = os.Open("/definitely/missing")
`,
			want: []string{
				"source package variable initializer is outside audited set authority_darwin.go",
			},
		},
		{
			name: "non module init function has a side effect",
			authorityExtra: `

func init() {
	_, _ = os.ReadFile("/definitely/missing")
}
`,
			want: []string{
				"source package initialization function is forbidden in authority_darwin.go",
			},
		},
		{
			name: "package function literal cannot panic the audit",
			authorityExtra: `

var sourceConstructionFixtureThunk = func() int {
	return 1
}
`,
			want: []string{
				"source package variable initializer is outside audited set authority_darwin.go",
			},
		},
		{
			name: "transitive helper reads package state",
			inventoryExtra: `

func sourceConstructionFixtureReadPackageState() int {
	return sourceConstructionFixturePackageState()
}
`,
			authorityExtra: `

var sourceConstructionFixtureState = 7

func sourceConstructionFixturePackageState() int {
	return sourceConstructionFixtureState
}
`,
			want: []string{
				"source call closure reads unaudited package variable authority_darwin.go|" +
					"sourceConstructionFixturePackageState|sourceConstructionFixtureState",
			},
		},
		{
			name: "package constant remains admitted",
			darwinPrimitivesExtra: `

const sourceConstructionFixtureConstant = 7
`,
		},
		{
			name: "package function value dispatch",
			inventoryExtra: `

func sourceConstructionFixturePureCheck(value string) bool {
	return strings.Contains(value, "fixture")
}

func sourceConstructionFixtureDynamicDispatch(value string) bool {
	check := sourceConstructionFixturePureCheck
	return check(value)
}
`,
			want: []string{
				"exposes function reference source_construction_inventory.go|" +
					"sourceConstructionFixtureDynamicDispatch|github.com/vbonnet/dear-agent/internal/buildauthority|" +
					"sourceConstructionFixturePureCheck",
				"dynamic callee source_construction_inventory.go|sourceConstructionFixtureDynamicDispatch|check",
			},
		},
		{
			name: "latent panic",
			inventoryExtra: `

func sourceConstructionFixturePanic() {
	panic("fixture")
}
`,
			want: []string{
				"source call closure reaches unaudited builtin panic",
			},
		},
		{
			name: "anonymous sealed interface carrier",
			inventoryExtra: `

func sourceConstructionFixtureAnonymousPrimitives() {
	var primitives sourcePrimitives = struct{ sourcePrimitives }{}
	primitives.privateSourcePrimitives()
}
`,
			want: []string{
				"sealed source interface value outside audited topology",
			},
		},
		{
			name: "sealed interface hidden in slice",
			inventoryExtra: `

func sourceConstructionFixturePrimitivesSlice(ctx context.Context) {
	values := []sourcePrimitives{struct{ *darwinSourcePrimitives }{}}
	_, _ = values[0].hashBytes(ctx, nil)
}
`,
			want: []string{
				"sealed source interface nested in unaudited container",
				"anonymous type implements sealed sourcePrimitives outside audited set",
			},
		},
		{
			name: "alternate sealed implementation replaces platform factory",
			authorityExtra: `

type sourceConstructionFixturePrimitives struct {
	darwinSourcePrimitives
}

func (sourceConstructionFixturePrimitives) hashBytes(
	ctx context.Context,
	content []byte,
) (Digest, *sourcePrimitiveFailure) {
	_, _ = os.ReadFile("/etc/passwd")
	return Digest{}, nil
}
`,
			darwinPrimitivesOriginal:    "return darwinSourcePrimitives{}, nil",
			darwinPrimitivesReplacement: "return sourceConstructionFixturePrimitives{}, nil",
			darwinPrimitivesCount:       1,
			want: []string{
				"sourceConstructionFixturePrimitives implements sealed sourcePrimitives outside audited set",
				"authority_darwin.go|(sourceConstructionFixturePrimitives).hashBytes|os|ReadFile",
			},
		},
		{
			name: "directory entry callback",
			authorityExtra: `

type sourceConstructionFixtureDirEntry struct {
	os.DirEntry
}

func (sourceConstructionFixtureDirEntry) Name() string {
	_, _ = os.ReadFile("/etc/passwd")
	return "config"
}
`,
			darwinPrimitivesExtra: `

func sourceConstructionFixtureNormalizeDirectoryEntry() {
	_, _, _ = normalizeDarwinSourceDirectoryBatch(
		context.Background(),
		[]os.DirEntry{sourceConstructionFixtureDirEntry{}},
		io.EOF,
	)
}
`,
			want: []string{
				"authority_darwin.go|(sourceConstructionFixtureDirEntry).Name|os|ReadFile",
			},
		},
		{
			name: "directory entries replaced after read",
			darwinPrimitivesOriginal: `entries, err := owner.file.ReadDir(sourceDirectoryReadBatchSize)
	runtime.KeepAlive(owner.file)
	return normalizeDarwinSourceDirectoryBatch(ctx, entries, err)`,
			darwinPrimitivesReplacement: `entries, err := owner.file.ReadDir(sourceDirectoryReadBatchSize)
	runtime.KeepAlive(owner.file)
	entries = []os.DirEntry{struct{ os.DirEntry }{}}
	return normalizeDarwinSourceDirectoryBatch(ctx, entries, err)`,
			darwinPrimitivesCount: 1,
			want: []string{
				"Darwin directory-read result does not flow directly into normalization",
			},
		},
		{
			name:                        "directory read error is relabeled terminal",
			darwinPrimitivesOriginal:    "\tdone := err == io.EOF",
			darwinPrimitivesReplacement: "\tdone := err != nil",
			darwinPrimitivesCount:       1,
			want: []string{
				"Darwin directory entry name flow is outside audited shape",
			},
		},
		{
			name: "nil directory entry guard is removed",
			darwinPrimitivesOriginal: `		if entry == nil {
			return nil, false, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			darwinPrimitivesReplacement: "\t\t_ = entry",
			darwinPrimitivesCount:       1,
			want: []string{
				"Darwin directory entry name flow is outside audited shape",
			},
		},
		{
			name: "errors Is callback",
			authorityExtra: `

type sourceConstructionFixtureError struct{}

func (sourceConstructionFixtureError) Error() string {
	return "fixture"
}

func (sourceConstructionFixtureError) Is(error) bool {
	_, _ = os.ReadFile("/etc/passwd")
	return false
}
`,
			darwinPrimitivesExtra: `

func sourceConstructionFixtureErrorsIs() bool {
	return errors.Is(sourceConstructionFixtureError{}, fs.ErrInvalid)
}
`,
			want: []string{
				"source call closure reaches unaudited external callee source_primitives_darwin.go|" +
					"sourceConstructionFixtureErrorsIs|errors|Is|" +
					"errors.Is(sourceConstructionFixtureError{},fs.ErrInvalid)",
			},
		},
		{
			name: "callback error replaces classified parameter",
			authorityExtra: `

type sourceConstructionFixtureReplacementError struct{}

func (sourceConstructionFixtureReplacementError) Error() string {
	return "fixture"
}

func (sourceConstructionFixtureReplacementError) Is(error) bool {
	_, _ = os.ReadFile("/definitely/missing")
	return false
}
`,
			darwinPrimitivesOriginal: `	if !mode.valid() || err == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}`,
			darwinPrimitivesReplacement: `	err = sourceConstructionFixtureReplacementError{}
	if !mode.valid() || err == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}`,
			darwinPrimitivesCount: 1,
			want: []string{
				"source call closure requires read-only parameter source_primitives_darwin.go|" +
					"classifyDarwinSourceOpenFailure|err|assignment",
				"implicit callback method is outside audited set authority_darwin.go|" +
					"(sourceConstructionFixtureReplacementError).Is",
			},
		},
		{
			name: "classified error is mutated through a type assertion",
			darwinPrimitivesOriginal: `	if !mode.valid() || err == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}`,
			darwinPrimitivesReplacement: `	if pathError, ok := err.(*os.PathError); ok {
		pathError.Err = fs.ErrPermission
	}
	if !mode.valid() || err == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}`,
			darwinPrimitivesCount: 1,
			want: []string{
				"source error type inspection is outside audited set " +
					"source_primitives_darwin.go|classifyDarwinSourceOpenFailure|err.(*os.PathError)",
			},
		},
		{
			name: "classified error type is erased before mutation",
			darwinPrimitivesOriginal: `	if !mode.valid() || err == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}`,
			darwinPrimitivesReplacement: `	if pathError, ok := any(err).(*os.PathError); ok {
		pathError.Err = fs.ErrPermission
	}
	if !mode.valid() || err == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}`,
			darwinPrimitivesCount: 1,
			want: []string{
				"source error type inspection is outside audited set " +
					"source_primitives_darwin.go|classifyDarwinSourceOpenFailure|any(err).(*os.PathError)",
			},
		},
		{
			name: "classified error type switch is laundered through an interface local",
			darwinPrimitivesOriginal: `	if !mode.valid() || err == nil {
		return 0, false, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}`,
			darwinPrimitivesReplacement: `	boxed := any(err)
	switch pathError := boxed.(type) {
	case *os.PathError:
		pathError.Err = fs.ErrNotExist
	}
	if !mode.valid() || err == nil {
		return 0, false, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}`,
			darwinPrimitivesCount: 1,
			want: []string{
				"source error type inspection is outside audited set " +
					"source_primitives_darwin.go|classifyDarwinSourcePresenceFailure|boxed.(type)",
			},
		},
		{
			name: "repository locator mutates after path capture",
			acquireOriginal: `	repository := &retainedSourceRepository{path: locator.path}
	builder.owner.repository = repository`,
			acquireReplacement: `	repository := &retainedSourceRepository{path: locator.path}
	if locator.path != "/repository" {
		locator.path = "/different/repository"
	}
	builder.owner.repository = repository`,
			acquireCount: 1,
			want: []string{
				"source repository locator use outside audited provenance " +
					"(*sourceConstructionBuilder).retainRepository",
			},
		},
		{
			name:            "inline repository locator literal",
			acquireOriginal: "builder.retainInitialSource(locator)",
			acquireReplacement: `builder.retainInitialSource(sourceRepositoryLocator{
				path: "/different/repository",
				seal: validSourceRepositoryLocator,
			})`,
			acquireCount: 1,
			want: []string{
				"source repository locator literal is forbidden outside its future minting seam",
			},
		},
		{
			name: "relative name mutates after raw validation",
			darwinPrimitivesOriginal: `	if !validDarwinSourceName(name) {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	for {`,
			darwinPrimitivesReplacement: `	if !validDarwinSourceName(name) {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if name == "production-only" {
		name = "different-valid-sibling"
	}
	for {`,
			darwinPrimitivesCount: 1,
			want: []string{
				"source call closure requires read-only parameter source_primitives_darwin.go|" +
					"openDarwinSourceRelativeDescriptor|name|assignment",
			},
		},
		{
			name: "inventory name mutates after child path derivation",
			inventoryOriginal: `	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if capture.descendants >= maxRepositoryEntries {`,
			inventoryReplacement: `	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if name == "production-only" {
		name = "different-valid-sibling"
	}
	if capture.descendants >= maxRepositoryEntries {`,
			inventoryCount: 1,
			want: []string{
				"source call closure requires read-only parameter source_construction_inventory.go|" +
					"(*sourceConstructionBuilder).captureSourceAdministrativeEntry|name|assignment",
			},
		},
		{
			name: "inventory child path mutates after derivation",
			inventoryOriginal: `	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if capture.descendants >= maxRepositoryEntries {`,
			inventoryReplacement: `	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	path = ".git/different-valid-sibling"
	if capture.descendants >= maxRepositoryEntries {`,
			inventoryCount: 1,
			want: []string{
				"source administrative child path/name provenance is outside audited shape",
			},
		},
		{
			name: "invalid child name gains an early successful relabel",
			inventoryOriginal: `	if prefix == "" || name == "" || strings.IndexByte(name, 0) >= 0 ||
		name == "." || name == ".." || strings.Contains(name, "/") {
		return "", newSourcePrimitiveFailure(OperationWalk, CauseUnstable)
	}`,
			inventoryReplacement: `	if prefix == "" || name == "" || strings.IndexByte(name, 0) >= 0 ||
		name == "." || name == ".." || strings.Contains(name, "/") {
		return prefix + "/relabeled", nil
	}`,
			inventoryCount: 1,
			want: []string{
				"source administrative child path construction is outside audited shape",
			},
		},
		{
			name: "opened inventory row path mutates before descriptor open",
			inventoryOriginal: `	name string,
	candidate sourceAdministrativeCandidate,
) bool {
	descriptor, openFailure := builder.primitives.openRelativeNoFollow(`,
			inventoryReplacement: `	name string,
	candidate sourceAdministrativeCandidate,
) bool {
	candidate.row.path = ".git/different-valid-sibling"
	descriptor, openFailure := builder.primitives.openRelativeNoFollow(`,
			inventoryCount: 1,
			want: []string{
				"opened source administrative row/name provenance is outside audited shape",
			},
		},
		{
			name:                        "directory entry name is replaced after validation",
			darwinPrimitivesOriginal:    "\t\tnames[index] = name",
			darwinPrimitivesReplacement: "\t\tnames[index] = \"different-valid-sibling\"",
			darwinPrimitivesCount:       1,
			want: []string{
				"Darwin directory entry name flow is outside audited shape",
			},
		},
		{
			name: "directory range name is replaced before dispatch",
			inventoryOriginal: `		for _, name := range names {
			if !builder.captureSourceAdministrativeEntry(capture, descriptor, prefix, name) {`,
			inventoryReplacement: `		for _, name := range names {
			_ = name
			if !builder.captureSourceAdministrativeEntry(capture, descriptor, prefix, "different-valid-sibling") {`,
			inventoryCount: 1,
			want: []string{
				"source administrative directory batch/name provenance is outside audited shape",
			},
		},
		{
			name: "directory walk skips a targeted subtree",
			inventoryOriginal: `		for _, name := range names {
			if !builder.captureSourceAdministrativeEntry(capture, descriptor, prefix, name) {`,
			inventoryReplacement: `		for _, name := range names {
			if prefix == ".git/objects/info" {
				continue
			}
			if !builder.captureSourceAdministrativeEntry(capture, descriptor, prefix, name) {`,
			inventoryCount: 1,
			want: []string{
				"source administrative directory batch/name provenance is outside audited shape",
			},
		},
		{
			name: "directory prefix is rebound before traversal",
			inventoryOriginal: `		builder.failInvariant()
		return false
	}
	for {
		names, terminal, failure := builder.primitives.readDirectoryBatch(`,
			inventoryReplacement: `		builder.failInvariant()
		return false
	}
	prefix = ".git/objects"
	for {
		names, terminal, failure := builder.primitives.readDirectoryBatch(`,
			inventoryCount: 1,
			want: []string{
				"source call closure requires read-only parameter source_construction_inventory.go|" +
					"(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|prefix|assignment",
			},
		},
		{
			name: "directory reobservation identity is laundered",
			inventoryOriginal: `	if failure == nil {
		failure = compareSourceDescriptorEvidence(
			builder.ctx,
			before.evidence,
			after.evidence,
		)`,
			inventoryReplacement: `	if failure == nil {
		after = before
		failure = compareSourceDescriptorEvidence(
			builder.ctx,
			before.evidence,
			after.evidence,
		)`,
			inventoryCount: 1,
			want: []string{
				"source administrative directory batch/name provenance is outside audited shape",
			},
		},
		{
			name: "recursive inventory prefix is replaced",
			inventoryOriginal: `		captured = builder.captureSourceAdministrativeDirectory(
			capture,
			descriptor,
			candidate.row.path,
			observation,
		)`,
			inventoryReplacement: `		captured = builder.captureSourceAdministrativeDirectory(
			capture,
			descriptor,
			".git/hooks",
			observation,
		)`,
			inventoryCount: 1,
			want: []string{
				"opened source administrative row/name provenance is outside audited shape",
			},
		},
		{
			name: "entry candidate row is replaced after construction",
			inventoryOriginal: `	candidate := sourceAdministrativeCandidate{row: sourceAdministrativeRow{
		path: path,
		kind: kind,
	}}
	if kind == sourceObservedSymlink || kind == sourceObservedSpecial {`,
			inventoryReplacement: `	candidate := sourceAdministrativeCandidate{row: sourceAdministrativeRow{
		path: path,
		kind: kind,
	}}
	candidate.row = sourceAdministrativeRow{path: ".git/hooks", kind: kind}
	if kind == sourceObservedSymlink || kind == sourceObservedSpecial {`,
			inventoryCount: 1,
			want: []string{
				"source administrative row/candidate mutation is outside audited set",
			},
		},
		{
			name: "forbidden entry kind returns success before capture",
			inventoryOriginal: `	candidate := sourceAdministrativeCandidate{row: sourceAdministrativeRow{
		path: path,
		kind: kind,
	}}`,
			inventoryReplacement: `	if kind == sourceObservedSymlink || kind == sourceObservedSpecial {
		return true
	}
	candidate := sourceAdministrativeCandidate{row: sourceAdministrativeRow{
		path: path,
		kind: kind,
	}}`,
			inventoryCount: 1,
			want: []string{
				"source administrative child path/name provenance is outside audited shape",
			},
		},
		{
			name: "captured ACL is mutated after observation",
			inventoryOriginal: `	candidate.row.evidence = observation.evidence
	candidate.acl = observation.acl`,
			inventoryReplacement: `	candidate.row.evidence = observation.evidence
	candidate.acl = observation.acl
	candidate.acl.disposition = sourceACLAdmitted`,
			inventoryCount: 1,
			want: []string{
				"source administrative row/candidate mutation is outside audited set",
			},
		},
		{
			name:                 "opened directory recursion condition is disabled",
			inventoryOriginal:    "\tif candidate.row.kind == sourceObservedDirectory {",
			inventoryReplacement: "\tif candidate.row.kind == sourceObservedDirectory && false {",
			inventoryCount:       1,
			want: []string{
				"opened source administrative row/name provenance is outside audited shape",
			},
		},
		{
			name: "nested evidence address escapes the retained row",
			inventoryExtra: `

func sourceConstructionFixtureEscapeAdministrativeEvidence(
	candidate *sourceAdministrativeCandidate,
) {
	snapshot := &candidate.row.evidence.snapshot
	snapshot.linkCount = 1
}
`,
			want: []string{
				"source administrative row/candidate address escape is outside audited set",
			},
		},
		{
			name: "range assignment mutates the retained path",
			inventoryExtra: `

func sourceConstructionFixtureRangeAdministrativePath(
	candidate *sourceAdministrativeCandidate,
) {
	for candidate.row.path = range map[string]struct{}{ ".git/hooks": {} } {
	}
}
`,
			want: []string{
				"source administrative row/candidate range mutation is forbidden",
			},
		},
		{
			name: "sort comparator mutates captured row path",
			inventoryOriginal: `	sort.Slice(capture.candidates, func(left, right int) bool {
		return bytes.Compare(`,
			inventoryReplacement: `	sort.Slice(capture.candidates, func(left, right int) bool {
		capture.candidates[left].row.path = ".git/hooks"
		return bytes.Compare(`,
			inventoryCount: 1,
			want: []string{
				"source administrative row/candidate mutation is outside audited set",
			},
		},
		{
			name: "sort comparator ignores captured path bytes",
			inventoryOriginal: `		return bytes.Compare(
			[]byte(capture.candidates[left].row.path),
			[]byte(capture.candidates[right].row.path),
		) < 0`,
			inventoryReplacement: `		return left < right`,
			inventoryCount:       1,
			want: []string{
				"source administrative finish topology is outside audited shape",
			},
		},
		{
			name: "validation loop skips a rejected candidate",
			inventoryOriginal: `		if failure := builder.validateSourceAdministrativeCandidate(candidate); failure != nil {
			return nil, failure
		}`,
			inventoryReplacement: `		if failure := builder.validateSourceAdministrativeCandidate(candidate); failure != nil {
			continue
		}`,
			inventoryCount: 1,
			want: []string{
				"source administrative finish topology is outside audited shape",
			},
		},
		{
			name:                 "candidate ACL rejection branch is disabled",
			inventoryOriginal:    `	if failure := builder.primitives.validateACL(builder.ctx, candidate.acl); failure != nil {`,
			inventoryReplacement: `	if failure := builder.primitives.validateACL(builder.ctx, candidate.acl); failure != nil && false {`,
			inventoryCount:       1,
			want: []string{
				"source administrative candidate validation is outside audited shape",
			},
		},
		{
			name: "candidate evidence failure guard launders validated evidence",
			inventoryOriginal: `	evidence, failure := validateSourceDescriptorEvidence(
		builder.ctx,
		candidate.row.evidence.snapshot,
		candidate.row.evidence.mount,
		candidate.acl,
		candidate.row.kind,
		ownerEffectiveOnly,
		candidate.row.kind == sourceObservedRegular &&
			candidate.row.class == sourceAuthorityAndManifest,
		0,
	)
	if failure != nil {`,
			inventoryReplacement: `	evidence, failure := validateSourceDescriptorEvidence(
		builder.ctx,
		candidate.row.evidence.snapshot,
		candidate.row.evidence.mount,
		candidate.acl,
		candidate.row.kind,
		ownerEffectiveOnly,
		candidate.row.kind == sourceObservedRegular &&
			candidate.row.class == sourceAuthorityAndManifest,
		0,
	)
	if evidence, failure = candidate.row.evidence, nil; failure != nil {`,
			inventoryCount: 1,
			want: []string{
				"source administrative candidate validation is outside audited shape",
			},
		},
		{
			name: "administrative inventory is truncated after validation",
			inventoryOriginal: `	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	builder.owner.git.administration = inventory`,
			inventoryReplacement: `	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	inventory.rows = inventory.rows[:3]
	builder.owner.git.administration = inventory`,
			inventoryCount: 1,
			want: []string{
				"source administrative validated install is outside audited shape",
			},
		},
		{
			name: "administrative inventory validator mutates after validation",
			inventoryOriginal: `	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

func (builder *sourceConstructionBuilder) compareRetainedSourceAdministrativeRows(`,
			inventoryReplacement: `	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	if validationFailure == nil {
		inventory.rows = inventory.rows[:0]
	}
	return validationFailure
}

func (builder *sourceConstructionBuilder) compareRetainedSourceAdministrativeRows(`,
			inventoryCount: 1,
			want: []string{
				"source administrative row/candidate mutation is outside audited set",
			},
		},
		{
			name:              "administrative inventory validity mutates rows through copy",
			inventoryOriginal: `	return rowsValid && total == inventory.totalRegularBytes && fixed.valid(`,
			inventoryReplacement: `	return copy(inventory.rows, inventory.rows[1:]) >= 0 &&
		rowsValid && total == inventory.totalRegularBytes && fixed.valid(`,
			inventoryCount: 1,
			want: []string{
				"source administrative inventory write builtin is forbidden",
			},
		},
		{
			name: "opened directory containment rejection branch is disabled",
			inventoryOriginal: `	if failure == nil && candidate.row.kind == sourceObservedDirectory {
		failure = validateInitialSourceChild(`,
			inventoryReplacement: `	if failure == nil && candidate.row.kind == sourceObservedDirectory && false {
		failure = validateInitialSourceChild(`,
			inventoryCount: 1,
			want: []string{
				"opened source administrative row/name provenance is outside audited shape",
			},
		},
		{
			name: "opened descriptor failure propagation is disabled",
			inventoryOriginal: `	if openFailure != nil {
		builder.outcome.addPrimitive(openFailure)
		builder.closeTransientDescriptor(descriptor)
		return false
	}

	observation, failure :=`,
			inventoryReplacement: `	if openFailure != nil && false {
		builder.outcome.addPrimitive(openFailure)
		builder.closeTransientDescriptor(descriptor)
		return false
	}

	observation, failure :=`,
			inventoryCount: 1,
			want: []string{
				"opened source administrative row/name provenance is outside audited shape",
			},
		},
		{
			name: "nil pointer concrete sealed receiver",
			inventoryExtra: `

func sourceConstructionFixtureNilConcreteReceiver(builder *sourceConstructionBuilder) {
	var primitives *darwinSourcePrimitives
	_, _ = primitives.hashBytes(builder.ctx, nil)
}
`,
			want: []string{
				"sealed concrete method receiver is not an exact non-pointer approved value " +
					"primitives.hashBytes",
			},
		},
		{
			name: "explicitly dereferenced nil sealed receiver",
			inventoryExtra: `

func sourceConstructionFixtureExplicitNilConcreteReceiver(builder *sourceConstructionBuilder) {
	var primitives *darwinSourcePrimitives
	_, _ = (*primitives).hashBytes(builder.ctx, nil)
}
`,
			want: []string{
				"sealed concrete method receiver is not an exact non-pointer approved value " +
					"(*primitives).hashBytes",
			},
		},
		{
			name:                     "repository root path expression changes",
			darwinPrimitivesOriginal: "owner.root, err = os.OpenRoot(locator.path)",
			darwinPrimitivesReplacement: "owner.root, err = os.OpenRoot(" +
				"locator.path + \"/..\")",
			darwinPrimitivesCount: 1,
			want: []string{
				"source call closure reaches unaudited external callee source_primitives_darwin.go|" +
					"(darwinSourcePrimitives).openRepositoryRoot|os|OpenRoot|" +
					"os.OpenRoot(locator.path + \"/..\")",
			},
		},
		{
			name:                     "root identity is laundered through descriptor identity",
			darwinPrimitivesOriginal: "\tsame := os.SameFile(rootInfo, descriptorInfo)",
			darwinPrimitivesReplacement: `	rootInfo = descriptorInfo
	same := os.SameFile(rootInfo, descriptorInfo)`,
			darwinPrimitivesCount: 1,
			want: []string{
				"root/descriptor identity objects are outside audited provenance",
			},
		},
		{
			name:                        "root identity mismatch branch is disabled",
			darwinPrimitivesOriginal:    "\tif !same {",
			darwinPrimitivesReplacement: "\tif !same && false {",
			darwinPrimitivesCount:       1,
			want: []string{
				"root/descriptor identity objects are outside audited provenance",
			},
		},
		{
			name:                     "root identity comparison becomes unreachable",
			darwinPrimitivesOriginal: "\tsame := os.SameFile(rootInfo, descriptorInfo)",
			darwinPrimitivesReplacement: `	return nil
	same := os.SameFile(rootInfo, descriptorInfo)`,
			darwinPrimitivesCount: 1,
			want: []string{
				"root/descriptor identity objects are outside audited provenance",
			},
		},
		{
			name: "initial child containment gains an early success return",
			acquireOriginal: `func validateInitialSourceChild(
	ctx context.Context,
	parent sourceDescriptorEvidence,
	child sourceDescriptorEvidence,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure`,
			acquireReplacement: `func validateInitialSourceChild(
	ctx context.Context,
	parent sourceDescriptorEvidence,
	child sourceDescriptorEvidence,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	if child.snapshot.size > maxPackedRefsBytes {
		return nil
	}
	var validationFailure *sourcePrimitiveFailure`,
			acquireCount: 1,
			want: []string{
				"initial source child containment validation is outside audited shape",
			},
		},
		{
			name:                     "repository root returns before validation",
			darwinPrimitivesOriginal: "owner.root, err = os.OpenRoot(locator.path)",
			darwinPrimitivesReplacement: `owner.root, err = os.OpenRoot(locator.path)
	if err == nil {
		return owner, nil
	}`,
			darwinPrimitivesCount: 1,
			want: []string{
				"Darwin post-acquisition topology outside audited shape " +
					"(darwinSourcePrimitives).openRepositoryRoot",
			},
		},
		{
			name: "child root returns before validation",
			darwinPrimitivesOriginal: `owner.root, err = parent.root.OpenRoot(name)
	runtime.KeepAlive(parent.root)`,
			darwinPrimitivesReplacement: `owner.root, err = parent.root.OpenRoot(name)
	runtime.KeepAlive(parent.root)
	if err == nil {
		return owner, nil
	}`,
			darwinPrimitivesCount: 1,
			want: []string{
				"Darwin post-acquisition topology outside audited shape " +
					"(darwinSourcePrimitives).openChildRoot",
			},
		},
		{
			name: "root descriptor returns before validation",
			darwinPrimitivesOriginal: `owner.file, err = root.root.Open(".")
	runtime.KeepAlive(root.root)`,
			darwinPrimitivesReplacement: `owner.file, err = root.root.Open(".")
	runtime.KeepAlive(root.root)
	if err == nil {
		return owner, nil
	}`,
			darwinPrimitivesCount: 1,
			want: []string{
				"Darwin post-acquisition topology outside audited shape " +
					"(darwinSourcePrimitives).openRootDirectoryDescriptor",
			},
		},
		{
			name: "root descriptor kind check gains success else",
			darwinPrimitivesOriginal: `		return owner, newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(kindErr))
	}
	return owner, nil`,
			darwinPrimitivesReplacement: `		return owner, newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(kindErr))
	} else {
		return nil, nil
	}
	return owner, nil`,
			darwinPrimitivesCount: 1,
			want: []string{
				"Darwin post-acquisition topology outside audited shape " +
					"(darwinSourcePrimitives).openRootDirectoryDescriptor",
			},
		},
		{
			name:                     "physical root flags mutate before open",
			darwinPrimitivesOriginal: "\tfor {\n\t\tfd, err := unix.Open(physicalRootPath, flags, 0)",
			darwinPrimitivesReplacement: `	flags &^= unix.O_NOFOLLOW_ANY
	for {
		fd, err := unix.Open(physicalRootPath, flags, 0)`,
			darwinPrimitivesCount: 1,
			want: []string{
				"source open flags do not have exact single-use provenance " +
					"(darwinSourcePrimitives).openPhysicalRootDescriptor",
			},
		},
		{
			name: "relative caller flags mutate before transfer",
			darwinPrimitivesOriginal: "\treturn openDarwinSourceRelativeDescriptor(" +
				"ctx, parent, name, kind, openKind, mode, flags)",
			darwinPrimitivesReplacement: "\tflags &^= unix.O_NOFOLLOW_ANY\n" +
				"\treturn openDarwinSourceRelativeDescriptor(" +
				"ctx, parent, name, kind, openKind, mode, flags)",
			darwinPrimitivesCount: 1,
			want: []string{
				"source open flags do not have exact single-use provenance " +
					"(darwinSourcePrimitives).openRelativeNoFollow",
			},
		},
		{
			name:                     "relative callee flags mutate before open",
			darwinPrimitivesOriginal: "\tfor {\n\t\tif failure := sourceContextPrimitiveFailure(ctx, OperationOpen)",
			darwinPrimitivesReplacement: `	flags &^= unix.O_NOFOLLOW_ANY
	for {
		if failure := sourceContextPrimitiveFailure(ctx, OperationOpen)`,
			darwinPrimitivesCount: 1,
			want: []string{
				"source open flags do not have exact single-use provenance " +
					"openDarwinSourceRelativeDescriptor",
			},
		},
		{
			name:                        "relative open disables no-follow",
			darwinPrimitivesOriginal:    "darwinOpenFlags(openKind, openKind != entrySymlink)",
			darwinPrimitivesReplacement: "darwinOpenFlags(openKind, false)",
			darwinPrimitivesCount:       1,
			want: []string{
				"source call closure reaches unaudited sensitive local callee source_primitives_darwin.go|" +
					"(darwinSourcePrimitives).openRelativeNoFollow|" +
					"github.com/vbonnet/dear-agent/internal/buildauthority|darwinOpenFlags|" +
					"darwinOpenFlags(openKind,false)",
			},
		},
		{
			name: "generic writer callback",
			inventoryExtra: `

func sourceConstructionFixtureWriteThroughInterface() {
	writeUint32(sourceConstructionFixtureWriter{}, 0)
}
`,
			authorityExtra: `

type sourceConstructionFixtureWriter struct{}

func (sourceConstructionFixtureWriter) Write(content []byte) (int, error) {
	_, _ = os.ReadFile("/etc/passwd")
	return len(content), nil
}
`,
			want: []string{
				"source call closure crosses generic writer boundary",
			},
		},
		{
			name: "range over function value",
			inventoryExtra: `

func sourceConstructionFixtureRangeFunction() {
	for range sourceConstructionFixtureSequence {
	}
}
`,
			authorityExtra: `

var sourceConstructionFixtureSequence = func(yield func() bool) {
	_, _ = os.ReadFile("/etc/passwd")
	yield()
}
`,
			want: []string{
				"source call closure reaches unaudited range-over-function",
			},
		},
		{
			name: "range over function returned by audited call",
			inventoryExtra: `

func sourceConstructionFixtureIdentitySequence(
	sequence func(func() bool),
) func(func() bool) {
	return sequence
}

func sourceConstructionFixtureRangeReturnedFunction() {
	for range sourceConstructionFixtureIdentitySequence(sourceConstructionFixtureSequence) {
	}
}
`,
			authorityExtra: `

var sourceConstructionFixtureSequence = func(yield func() bool) {
	_, _ = os.ReadFile("/etc/passwd")
	yield()
}
`,
			want: []string{
				"source call closure reaches unaudited range-over-function",
			},
		},
		{
			name: "pure external call remains admitted",
			inventoryExtra: `

func sourceConstructionFixturePureExternal(value string) bool {
	return strings.Contains(value, "fixture")
}
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			inventoryFixture := append([]byte(nil), inventorySource...)
			if test.inventoryOriginal != "" {
				if count := strings.Count(
					string(inventoryFixture),
					test.inventoryOriginal,
				); count != test.inventoryCount {
					t.Fatalf("source inventory mutation count = %d, want %d", count, test.inventoryCount)
				}
				inventoryFixture = []byte(strings.Replace(
					string(inventoryFixture),
					test.inventoryOriginal,
					test.inventoryReplacement,
					1,
				))
			}
			overlay := map[string][]byte{
				inventoryPath: append(
					inventoryFixture,
					[]byte(test.inventoryExtra)...,
				),
			}
			if test.authorityExtra != "" {
				authorityFixture := append(
					append([]byte(nil), authoritySource...),
					[]byte(test.authorityExtra)...,
				)
				if test.authorityNeedsUnsafe {
					withUnsafe := strings.Replace(
						string(authorityFixture),
						"\t\"os\"\n",
						"\t\"os\"\n\t_ \"unsafe\"\n",
						1,
					)
					if withUnsafe == string(authorityFixture) {
						t.Fatal("Darwin authority fixture could not add unsafe import")
					}
					authorityFixture = []byte(withUnsafe)
				}
				overlay[authorityPath] = authorityFixture
			}
			if test.darwinPrimitivesOriginal != "" || test.darwinPrimitivesExtra != "" {
				darwinFixture := append([]byte(nil), darwinPrimitivesSource...)
				if test.darwinPrimitivesOriginal != "" {
					if count := strings.Count(
						string(darwinFixture),
						test.darwinPrimitivesOriginal,
					); count != test.darwinPrimitivesCount {
						t.Fatalf("Darwin source primitive mutation count = %d, want %d", count, test.darwinPrimitivesCount)
					}
					darwinFixture = []byte(strings.Replace(
						string(darwinFixture),
						test.darwinPrimitivesOriginal,
						test.darwinPrimitivesReplacement,
						1,
					))
				}
				overlay[darwinPrimitivesPath] = append(
					darwinFixture,
					[]byte(test.darwinPrimitivesExtra)...,
				)
			}
			if test.acquireOriginal != "" {
				if count := strings.Count(
					string(acquireSource),
					test.acquireOriginal,
				); count != test.acquireCount {
					t.Fatalf("source acquisition mutation count = %d, want %d", count, test.acquireCount)
				}
				overlay[acquirePath] = []byte(strings.Replace(
					string(acquireSource),
					test.acquireOriginal,
					test.acquireReplacement,
					1,
				))
			}
			joined := strings.Join(
				sourceConstructionResolvedCallClosureViolations(
					loadTypedDarwinSourceConstructionPackage(t, overlay),
				),
				"\n",
			)
			if len(test.want) == 0 {
				if joined != "" {
					t.Fatalf("pure external source call violations:\n%s", joined)
				}
				return
			}
			for _, want := range test.want {
				if !strings.Contains(joined, want) {
					t.Fatalf("resolved source call-closure violations =\n%s\nwant %q", joined, want)
				}
			}
		})
	}
}

func TestSourceConstructionResolvedCallClosureAuditsAllowedCallbackBodies(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
	authoritySource, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatalf("read Darwin authority callback fixture base: %v", err)
	}
	authoritySource = append(authoritySource, []byte(`

func sourceConstructionFixtureReadPrivateError() { _, _ = os.ReadFile("/private-error") }
func sourceConstructionFixtureReadPrivateUnwrap() { _, _ = os.ReadFile("/private-unwrap") }
func sourceConstructionFixtureReadFailureError() { _, _ = os.ReadFile("/failure-error") }
func sourceConstructionFixtureReadMachOError() { _, _ = os.ReadFile("/macho-error") }
func sourceConstructionFixtureReadContextDeadline() { _, _ = os.ReadFile("/context-deadline") }
func sourceConstructionFixtureReadContextDone() { _, _ = os.ReadFile("/context-done") }
func sourceConstructionFixtureReadContextErr() { _, _ = os.ReadFile("/context-err") }
func sourceConstructionFixtureReadContextValue() { _, _ = os.ReadFile("/context-value") }
func sourceConstructionFixtureReadSchedulerNow() { _, _ = os.ReadFile("/scheduler-now") }
`)...)
	overlay := map[string][]byte{authorityPath: authoritySource}
	for _, mutation := range []struct {
		filename    string
		original    string
		replacement string
	}{
		{
			filename: "private_errors.go",
			original: "func (failure *privateFailure) Error() string {",
			replacement: `func (failure *privateFailure) Error() string {
	sourceConstructionFixtureReadPrivateError()`,
		},
		{
			filename: "private_errors.go",
			original: "func (failure *privateFailure) Unwrap() error {",
			replacement: `func (failure *privateFailure) Unwrap() error {
	sourceConstructionFixtureReadPrivateUnwrap()`,
		},
		{
			filename:    "types.go",
			original:    "func (e *failureError) Error() string { return e.text }",
			replacement: "func (e *failureError) Error() string { sourceConstructionFixtureReadFailureError(); return e.text }",
		},
		{
			filename: "macho.go",
			original: "func (err *machOValidationError) Error() string {",
			replacement: `func (err *machOValidationError) Error() string {
	sourceConstructionFixtureReadMachOError()`,
		},
		{
			filename: "preflight_runner.go",
			original: "func (ctx *nonSourceAuthorityContext) Deadline() (time.Time, bool) {",
			replacement: `func (ctx *nonSourceAuthorityContext) Deadline() (time.Time, bool) {
	sourceConstructionFixtureReadContextDeadline()`,
		},
		{
			filename: "preflight_runner.go",
			original: "func (ctx *nonSourceAuthorityContext) Done() <-chan struct{} {",
			replacement: `func (ctx *nonSourceAuthorityContext) Done() <-chan struct{} {
	sourceConstructionFixtureReadContextDone()
	_ = ctx.transaction.issuer.scheduler.now()`,
		},
		{
			filename: "preflight_runner.go",
			original: "func (ctx *nonSourceAuthorityContext) Err() error {",
			replacement: `func (ctx *nonSourceAuthorityContext) Err() error {
	sourceConstructionFixtureReadContextErr()`,
		},
		{
			filename: "preflight_runner.go",
			original: "func (ctx *nonSourceAuthorityContext) Value(key any) any {",
			replacement: `func (ctx *nonSourceAuthorityContext) Value(key any) any {
	sourceConstructionFixtureReadContextValue()`,
		},
		{
			filename:    "process.go",
			original:    "func (realProcessScheduler) now() time.Time { return time.Now() }",
			replacement: "func (realProcessScheduler) now() time.Time { sourceConstructionFixtureReadSchedulerNow(); return time.Now() }",
		},
	} {
		path := filepath.Join(workingDirectory, mutation.filename)
		source := overlay[path]
		if source == nil {
			source, err = os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s callback fixture base: %v", mutation.filename, err)
			}
		}
		if count := strings.Count(string(source), mutation.original); count != 1 {
			t.Fatalf("%s callback mutation count = %d, want 1", mutation.filename, count)
		}
		overlay[path] = []byte(strings.Replace(
			string(source),
			mutation.original,
			mutation.replacement,
			1,
		))
	}
	joined := strings.Join(
		sourceConstructionResolvedCallClosureViolations(
			loadTypedDarwinSourceConstructionPackage(t, overlay),
		),
		"\n",
	)
	for _, helper := range []string{
		"sourceConstructionFixtureReadPrivateError",
		"sourceConstructionFixtureReadPrivateUnwrap",
		"sourceConstructionFixtureReadFailureError",
		"sourceConstructionFixtureReadMachOError",
		"sourceConstructionFixtureReadContextDeadline",
		"sourceConstructionFixtureReadContextDone",
		"sourceConstructionFixtureReadContextErr",
		"sourceConstructionFixtureReadContextValue",
		"sourceConstructionFixtureReadSchedulerNow",
	} {
		if !strings.Contains(joined, "authority_darwin.go|"+helper+"|os|ReadFile") {
			t.Fatalf("callback-body closure violations =\n%s\nwant helper %q", joined, helper)
		}
	}
	if !strings.Contains(
		joined,
		"bodyless package callee preflight_runner.go|(*nonSourceAuthorityContext).Done|"+
			"github.com/vbonnet/dear-agent/internal/buildauthority|(processScheduler).now",
	) {
		t.Fatalf("callback-body closure violations =\n%s\nwant unexpected scheduler dispatch rejection", joined)
	}
}

func TestDarwinSourcePrimitiveContentReadCallsAreClosed(t *testing.T) {
	const sourceName = "source_primitives_darwin.go"
	files := token.NewFileSet()
	production, err := parser.ParseFile(files, sourceName, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse Darwin source primitives: %v", err)
	}
	if violations := sourceConstructionDarwinContentReadViolations(files, production); len(violations) != 0 {
		t.Fatalf("Darwin source content-read violations:\n%s", strings.Join(violations, "\n"))
	}
	if violations := sourceConstructionDescriptorFileCapabilityViolations(
		loadTypedDarwinSourceConstructionPackage(t, nil),
	); len(violations) != 0 {
		t.Fatalf("source descriptor file-capability violations:\n%s", strings.Join(violations, "\n"))
	}

	source, err := os.ReadFile(sourceName)
	if err != nil {
		t.Fatalf("read Darwin source primitives: %v", err)
	}
	source = append(source, []byte(`

func sourceConstructionFixtureReads(owner *ownedSourceDescriptor) {
	buffer := make([]byte, 1)
	read := owner.file.Read
	_, _ = read(buffer)
	_, _ = io.ReadAll(owner.file)
	_, _ = unix.Readv(int(owner.file.Fd()), [][]byte{buffer})
	_, _ = unix.Pread(int(owner.file.Fd()), buffer, 0)
	_, _ = unix.Readlink("fixture", buffer)
	sourceConstructionFixturePrimitiveHelper(owner)
}

func sourceConstructionFixtureRawFDHelper(fd int) {
	_, _ = unix.Read(fd, make([]byte, 1))
}

func sourceConstructionFixtureRawFDOrigin() {
	fd, _ := unix.Open("fixture", unix.O_RDONLY, 0)
	sourceConstructionFixtureRawFDHelper(fd)
	_ = unix.Close(fd)
	fd = -1
}
`)...)
	mutatedFiles := token.NewFileSet()
	mutated, err := parser.ParseFile(
		mutatedFiles,
		sourceName,
		source,
		parser.SkipObjectResolution,
	)
	if err != nil {
		t.Fatalf("parse mutated Darwin source primitives: %v", err)
	}
	joined := strings.Join(
		sourceConstructionDarwinContentReadViolations(mutatedFiles, mutated),
		"\n",
	)
	for _, want := range []string{
		"content-read reference Read outside audited site Read|sourceConstructionFixtureReads",
		"content-read reference ReadAll outside audited site ReadAll|sourceConstructionFixtureReads",
		"content-read reference Readv outside audited site Readv|sourceConstructionFixtureReads",
		"content-read reference Pread outside audited site Pread|sourceConstructionFixtureReads",
		"content-read reference Readlink outside audited site Readlink|sourceConstructionFixtureReads",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("mutated Darwin content-read violations =\n%s\nwant %q", joined, want)
		}
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	primitivesPath := filepath.Join(workingDirectory, "source_primitives.go")
	primitivesSource, err := os.ReadFile(primitivesPath)
	if err != nil {
		t.Fatalf("read source primitives: %v", err)
	}
	primitivesSource = append(primitivesSource, []byte(`

func sourceConstructionFixturePrimitiveHelper(owner *ownedSourceDescriptor) {
	_, _ = owner.file.Read(make([]byte, 1))
}

func sourceConstructionFixtureRawFileHelper(file *os.File) {
	_, _ = file.Read(make([]byte, 1))
}

func sourceConstructionFixtureRawOrigin() {
	file, _ := os.Open("fixture")
	_ = file
}

func sourceConstructionFixtureRawRootHelper(root *os.Root) (*os.File, error) {
	file, err := root.Open(".")
	if err == nil {
		_, _ = file.Read(make([]byte, 1))
	}
	return file, err
}

func sourceConstructionFixtureRawRootOrigin() {
	root, _ := os.OpenRoot("fixture")
	_ = root
}
`)...)
	const fstatCall = "err := unix.Fstat(int(owner.file.Fd()), &stat)"
	if count := strings.Count(string(source), fstatCall); count != 1 {
		t.Fatalf("Darwin source Fstat fixture site count = %d, want 1", count)
	}
	source = []byte(strings.Replace(
		string(source),
		fstatCall,
		"sourceConstructionFixtureRawFileHelper(owner.file)\n\terr := unix.Fstat(-1, &stat)",
		1,
	))
	const rootOpenCall = `owner.file, err = root.root.Open(".")`
	if count := strings.Count(string(source), rootOpenCall); count != 1 {
		t.Fatalf("Darwin source root Open fixture site count = %d, want 1", count)
	}
	source = []byte(strings.Replace(
		string(source),
		rootOpenCall,
		`owner.file, err = sourceConstructionFixtureRawRootHelper(root.root)`,
		1,
	))
	darwinPath := filepath.Join(workingDirectory, sourceName)
	typedMutated := loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
		primitivesPath: primitivesSource,
		darwinPath:     source,
	})
	joined = strings.Join(
		sourceConstructionDescriptorFileCapabilityViolations(typedMutated),
		"\n",
	)
	for _, want := range []string{
		"source_primitives.go|sourceConstructionFixturePrimitiveHelper",
		"source_primitives.go|sourceConstructionFixtureRawOrigin|os.Open|:=:file,_",
		"source_primitives.go|sourceConstructionFixtureRawRootOrigin|os.OpenRoot|:=:root,_",
		"source_primitives_darwin.go|sourceConstructionFixtureReads",
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|" +
			"argument:sourceConstructionFixtureRawFileHelper",
		"source_primitives_darwin.go|sourceConstructionFixtureRawFDOrigin|unix.Open|" +
			"call:sourceConstructionFixtureRawFDHelper",
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|" +
			"argument:sourceConstructionFixtureRawRootHelper",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("mutated descriptor file-capability violations =\n%s\nwant %q", joined, want)
		}
	}
}

func TestSourceConstructionRawDescriptorFDAuditRejectsLaundering(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	primitivesPath := filepath.Join(workingDirectory, "source_primitives_darwin.go")
	primitivesSource, err := os.ReadFile(primitivesPath)
	if err != nil {
		t.Fatalf("read Darwin source primitives: %v", err)
	}
	authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
	authoritySource, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatalf("read Darwin authority primitives: %v", err)
	}
	helperPath := filepath.Join(workingDirectory, "source_fd_helpers_darwin.go")

	for _, test := range []struct {
		name            string
		primitivesExtra string
		authorityExtra  string
		helperSource    string
		want            []string
	}{
		{
			name: "direct return in source primitive",
			primitivesExtra: `

func sourceConstructionFixtureHiddenRawFD() (int, error) {
	return unix.Open("fixture", unix.O_RDONLY, 0)
}
`,
			want: []string{
				"raw descriptor fd origin outside audited site " +
					"source_primitives_darwin.go|sourceConstructionFixtureHiddenRawFD|unix.Open",
				"raw descriptor fd origin lacks an audited direct binding " +
					"source_primitives_darwin.go|sourceConstructionFixtureHiddenRawFD|unix.Open|return",
			},
		},
		{
			name: "cross file direct return",
			primitivesExtra: `

func sourceConstructionFixtureUseHiddenRawFD() {
	fd, _ := sourceConstructionFixtureHiddenRawFD()
	_ = unix.Close(fd)
}
`,
			authorityExtra: `

func sourceConstructionFixtureHiddenRawFD() (int, error) {
	return unix.Open("fixture", unix.O_RDONLY, 0)
}
`,
			want: []string{
				"raw descriptor fd origin outside audited site " +
					"authority_darwin.go|sourceConstructionFixtureHiddenRawFD|unix.Open",
				"raw descriptor fd origin lacks an audited direct binding " +
					"authority_darwin.go|sourceConstructionFixtureHiddenRawFD|unix.Open|return",
			},
		},
		{
			name: "assign then return",
			primitivesExtra: `

func sourceConstructionFixtureReturnBoundRawFD() (int, error) {
	fd, err := unix.Open("fixture", unix.O_RDONLY, 0)
	return fd, err
}
`,
			want: []string{
				"raw descriptor fd origin outside audited site " +
					"source_primitives_darwin.go|sourceConstructionFixtureReturnBoundRawFD|unix.Open",
				"raw descriptor fd outside audited site " +
					"source_primitives_darwin.go|sourceConstructionFixtureReturnBoundRawFD|unix.Open|<unclassified>",
			},
		},
		{
			name: "package function value",
			authorityExtra: `

var sourceConstructionFixtureOpen = unix.Open
`,
			want: []string{
				"raw descriptor opener reference outside a direct call " +
					"authority_darwin.go|<outside-function>|unix.Open",
			},
		},
		{
			name: "local function value",
			authorityExtra: `

func sourceConstructionFixtureOpenThroughValue() (int, error) {
	open := unix.Open
	return open("fixture", unix.O_RDONLY, 0)
}
`,
			want: []string{
				"raw descriptor opener reference outside a direct call " +
					"authority_darwin.go|sourceConstructionFixtureOpenThroughValue|unix.Open",
			},
		},
		{
			name: "dot imported function value",
			helperSource: `package buildauthority

import . "golang.org/x/sys/unix"

var sourceConstructionFixtureDotOpen = Open

func sourceConstructionFixtureDotOpenRawFD() (int, error) {
	return sourceConstructionFixtureDotOpen("fixture", O_RDONLY, 0)
}
`,
			want: []string{
				"raw descriptor opener reference outside a direct call " +
					"source_fd_helpers_darwin.go|<outside-function>|unix.Open",
			},
		},
		{
			name: "discarded owner",
			authorityExtra: `

func sourceConstructionFixtureDiscardRawFD() {
	_, _ = unix.Open("fixture", unix.O_RDONLY, 0)
}
`,
			want: []string{
				"raw descriptor fd origin outside audited site " +
					"authority_darwin.go|sourceConstructionFixtureDiscardRawFD|unix.Open",
				"raw descriptor fd origin lacks an audited owner " +
					"authority_darwin.go|sourceConstructionFixtureDiscardRawFD|unix.Open",
			},
		},
		{
			name: "unrelated integer helper remains allowed",
			primitivesExtra: `

func sourceConstructionFixtureBenignInteger() (int, error) {
	return 7, nil
}
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			overlay := make(map[string][]byte)
			if test.primitivesExtra != "" {
				overlay[primitivesPath] = append(
					append([]byte(nil), primitivesSource...),
					[]byte(test.primitivesExtra)...,
				)
			}
			if test.authorityExtra != "" {
				overlay[authorityPath] = append(
					append([]byte(nil), authoritySource...),
					[]byte(test.authorityExtra)...,
				)
			}
			if test.helperSource != "" {
				overlay[helperPath] = []byte(test.helperSource)
			}
			joined := strings.Join(
				sourceConstructionDescriptorFileCapabilityViolations(
					loadTypedDarwinSourceConstructionPackage(t, overlay),
				),
				"\n",
			)
			if len(test.want) == 0 {
				if joined != "" {
					t.Fatalf("benign integer helper violations:\n%s", joined)
				}
				return
			}
			for _, want := range test.want {
				if !strings.Contains(joined, want) {
					t.Fatalf("raw descriptor fd violations =\n%s\nwant %q", joined, want)
				}
			}
		})
	}
}

func TestSourceConstructionRawDescriptorSinkProvenanceRejectsLaundering(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	primitivesPath := filepath.Join(workingDirectory, "source_primitives_darwin.go")
	primitivesSource, err := os.ReadFile(primitivesPath)
	if err != nil {
		t.Fatalf("read Darwin source primitives: %v", err)
	}
	helperPath := filepath.Join(workingDirectory, "source_fd_helpers_darwin.go")

	for _, test := range []struct {
		name          string
		original      string
		replacement   string
		originalCount int
		helperSource  string
		want          []string
	}{
		{
			name:          "narrowed descriptor conversion",
			original:      "file:  os.NewFile(uintptr(fd), physicalRootPath)",
			replacement:   "file:  os.NewFile(uintptr(uint8(fd)), physicalRootPath)",
			originalCount: 1,
			want: []string{
				"raw descriptor file transfer lacks exact fd provenance " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|os.NewFile",
				"raw descriptor fd outside audited site " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|" +
					"unix.Open|call:os.NewFile[arg0:uint8>uintptr]",
			},
		},
		{
			name:          "wrong NewFile argument",
			original:      "file:  os.NewFile(uintptr(fd), physicalRootPath)",
			replacement:   "file:  os.NewFile(uintptr(sourceConstructionFixtureOtherFD), string(rune(fd)))",
			originalCount: 1,
			helperSource: `package buildauthority

var sourceConstructionFixtureOtherFD int
`,
			want: []string{
				"raw descriptor file transfer lacks exact fd provenance " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|os.NewFile",
				"raw descriptor fd outside audited site " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|" +
					"unix.Open|call:os.NewFile[arg1:rune>string]",
			},
		},
		{
			name:          "alternate syscall source",
			original:      "file:  os.NewFile(uintptr(fd), physicalRootPath)",
			replacement:   "file:  os.NewFile(uintptr(sourceConstructionFixtureSyscallFD()), physicalRootPath)",
			originalCount: 1,
			helperSource: `package buildauthority

import "syscall"

func sourceConstructionFixtureSyscallFD() int {
	fd, _ := syscall.Open("fixture", syscall.O_RDONLY, 0)
	return fd
}
`,
			want: []string{
				"raw descriptor file transfer lacks exact fd provenance " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|os.NewFile",
				"audited raw descriptor fd use " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|" +
					"unix.Open|call:os.NewFile[arg0:uintptr] count = 0, want 1",
			},
		},
		{
			name: "cleanup before ownership transfer",
			original: `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), physicalRootPath),
			kind:  sourceObservedDirectory,
			state: sourceHandleOpen,
		}
		if owner.file == nil {
			closeErr := unix.Close(fd)
			if closeErr != nil {
				return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			replacement: `		closeErr := unix.Close(fd)
		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), physicalRootPath),
			kind:  sourceObservedDirectory,
			state: sourceHandleOpen,
		}
		if owner.file == nil {
			if closeErr != nil {
				return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			originalCount: 1,
			want: []string{
				"raw descriptor transfer/cleanup topology outside audited shape " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open",
			},
		},
		{
			name: "early return between transfer and cleanup",
			original: `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), physicalRootPath),
			kind:  sourceObservedDirectory,
			state: sourceHandleOpen,
		}
		if owner.file == nil {`,
			replacement: `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), physicalRootPath),
			kind:  sourceObservedDirectory,
			state: sourceHandleOpen,
		}
		if strings.HasSuffix(physicalRootPath, ".leak") {
			return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		if owner.file == nil {`,
			originalCount: 1,
			want: []string{
				"raw descriptor transfer/cleanup topology outside audited shape " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open",
			},
		},
		{
			name: "early return after successful open before transfer",
			original: `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), physicalRootPath),`,
			replacement: `		if strings.HasSuffix(physicalRootPath, ".leak") {
			return nil, nil
		}
		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), physicalRootPath),`,
			originalCount: 1,
			want: []string{
				"raw descriptor transfer/cleanup topology outside audited shape " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open",
			},
		},
		{
			name: "early return after descriptor invalidation",
			original: `		if !invalidateDarwinSourceFD(&fd) {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {`,
			replacement: `		if !invalidateDarwinSourceFD(&fd) {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		if strings.HasSuffix(physicalRootPath, ".leak") {
			return nil, nil
		}
		if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {`,
			originalCount: 2,
			want: []string{
				"raw descriptor transfer/cleanup topology outside audited shape " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open",
			},
		},
		{
			name: "failed raw close is suppressed",
			original: `			if closeErr != nil {
				return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}`,
			replacement: `			if closeErr != nil {
				return nil, nil
			}`,
			originalCount: 2,
			want: []string{
				"raw descriptor transfer/cleanup topology outside audited shape " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open",
			},
		},
		{
			name: "failed invalidation drops owner",
			original: `		if !invalidateDarwinSourceFD(&fd) {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			replacement: `		if !invalidateDarwinSourceFD(&fd) {
			return nil, nil
		}`,
			originalCount: 2,
			want: []string{
				"raw descriptor transfer/cleanup topology outside audited shape " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open",
			},
		},
		{
			name: "relative kind check gains success else",
			original: `			return owner, newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(kindErr))
		}
		return owner, nil`,
			replacement: `			return owner, newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(kindErr))
		} else {
			return nil, nil
		}
		return owner, nil`,
			originalCount: 1,
			want: []string{
				"raw descriptor transfer/cleanup topology outside audited shape " +
					"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|unix.Openat",
			},
		},
		{
			name: "asynchronous close",
			original: `		if owner.file == nil {
			closeErr := unix.Close(fd)
			if closeErr != nil {
				return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			replacement: `		if owner.file == nil {
			go unix.Close(fd)
			return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			originalCount: 2,
			want: []string{
				"raw descriptor fd outside audited site " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|" +
					"unix.Open|call:unix.Close[arg0]@go",
			},
		},
		{
			name: "asynchronous invalidator",
			original: `		if !invalidateDarwinSourceFD(&fd) {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			replacement: `		go invalidateDarwinSourceFD(&fd)
		if false {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			originalCount: 2,
			want: []string{
				"raw descriptor fd outside audited site " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|" +
					"unix.Open|move:invalidateDarwinSourceFD@go",
			},
		},
		{
			name: "captured invalidator",
			original: `		if !invalidateDarwinSourceFD(&fd) {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			replacement: `		sourceConstructionFixtureCallback = func() {
			invalidateDarwinSourceFD(&fd)
		}
		if false {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}`,
			originalCount: 2,
			helperSource: `package buildauthority

var sourceConstructionFixtureCallback func()
`,
			want: []string{
				"raw descriptor fd outside audited site " +
					"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|" +
					"unix.Open|move:invalidateDarwinSourceFD@func-literal",
			},
		},
		{
			name:          "wrong Openat argument",
			original:      "fd, openErr := unix.Openat(int(parent.file.Fd()), name, flags, 0)",
			replacement:   "fd, openErr := unix.Openat(sourceConstructionFixtureOtherFD, name, flags, uint32(parent.file.Fd()))",
			originalCount: 1,
			helperSource: `package buildauthority

var sourceConstructionFixtureOtherFD int
`,
			want: []string{
				"source descriptor file capability outside audited site " +
					"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|" +
					"fd:unix.Openat[arg3:uint32]",
			},
		},
		{
			name:          "wrong kind-check argument",
			original:      "kindErr := requireDescriptorKind(int(owner.file.Fd()), openKind)",
			replacement:   "kindErr := requireDescriptorKind(sourceConstructionFixtureOtherFD, entryKind(owner.file.Fd()))",
			originalCount: 1,
			helperSource: `package buildauthority

var sourceConstructionFixtureOtherFD int
`,
			want: []string{
				"source descriptor file capability outside audited site " +
					"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|" +
					"fd:requireDescriptorKind[arg1:entryKind]",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if count := strings.Count(string(primitivesSource), test.original); count != test.originalCount {
				t.Fatalf("Darwin source mutation site count = %d, want %d", count, test.originalCount)
			}
			mutated := []byte(strings.Replace(
				string(primitivesSource),
				test.original,
				test.replacement,
				1,
			))
			overlay := map[string][]byte{primitivesPath: mutated}
			if test.helperSource != "" {
				overlay[helperPath] = []byte(test.helperSource)
			}
			joined := strings.Join(
				sourceConstructionDescriptorFileCapabilityViolations(
					loadTypedDarwinSourceConstructionPackage(t, overlay),
				),
				"\n",
			)
			for _, want := range test.want {
				if !strings.Contains(joined, want) {
					t.Fatalf("raw descriptor provenance violations =\n%s\nwant %q", joined, want)
				}
			}
		})
	}
}

func TestSourceConstructionAggregateOwnerMethodBoundariesAreClosed(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
	authoritySource, err := os.ReadFile(authorityPath)
	if err != nil {
		t.Fatalf("read Darwin authority primitives: %v", err)
	}

	for _, test := range []struct {
		name   string
		extra  string
		escape string
	}{
		{
			name: "builder method value",
			extra: `

var sourceConstructionFixtureAggregateMethodSink any

func sourceConstructionFixtureStealBuilderMethod(builder *sourceConstructionBuilder) {
	sourceConstructionFixtureAggregateMethodSink = builder.retainSourceAdministrativeInventory
}
`,
			escape: "escapes through a method value",
		},
		{
			name: "retained root method value",
			extra: `

var sourceConstructionFixtureAggregateMethodSink any

func sourceConstructionFixtureStealRetainedRootMethod(retained retainedSourceRoot) {
	sourceConstructionFixtureAggregateMethodSink = retained.validOpenDirectory
}
`,
			escape: "escapes through a method value",
		},
		{
			name: "builder go boundary",
			extra: `

func sourceConstructionFixtureRunBuilderLater(builder *sourceConstructionBuilder) {
	go builder.retainSourceAdministrativeInventory()
}
`,
			escape: "crosses a go boundary",
		},
		{
			name: "builder defer boundary",
			extra: `

func sourceConstructionFixtureDeferBuilder(builder *sourceConstructionBuilder) {
	defer builder.retainSourceAdministrativeInventory()
}
`,
			escape: "crosses a defer boundary",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := append(append([]byte(nil), authoritySource...), []byte(test.extra)...)
			joined := strings.Join(
				sourceConstructionDescriptorFileCapabilityViolations(
					loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
						authorityPath: mutated,
					}),
				),
				"\n",
			)
			for _, owner := range []string{"descriptor", "root"} {
				want := "source " + owner + " owner " + test.escape +
					" outside audited boundary authority_darwin.go|"
				if !strings.Contains(joined, want) {
					t.Fatalf("aggregate owner terminal violations =\n%s\nwant %q", joined, want)
				}
			}
		})
	}
}

func TestSourceConstructionDescriptorCapabilityAuditRejectsTypedLaundering(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	darwinPath := filepath.Join(workingDirectory, "source_primitives_darwin.go")
	darwinSource, err := os.ReadFile(darwinPath)
	if err != nil {
		t.Fatalf("read Darwin source primitives: %v", err)
	}

	t.Run("lookalike composite field", func(t *testing.T) {
		const original = `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), physicalRootPath),
			kind:  sourceObservedDirectory,
			state: sourceHandleOpen,
		}`
		const replacement = `		box := sourceConstructionFixtureFileBox{
			file: os.NewFile(uintptr(fd), physicalRootPath),
		}
		owner := &ownedSourceDescriptor{
			file:  box.file,
			kind:  sourceObservedDirectory,
			state: sourceHandleOpen,
		}`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin descriptor composite fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		mutated = append(mutated, []byte(`

type sourceConstructionFixtureFileBox struct {
	file *os.File
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{darwinPath: mutated}),
			),
			"\n",
		)
		want := "raw descriptor file origin outside audited site " +
			"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|" +
			"os.NewFile|composite:sourceConstructionFixtureFileBox.file"
		if !strings.Contains(joined, want) {
			t.Fatalf("lookalike composite violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("shadowed fd invalidator", func(t *testing.T) {
		const original = "\t\tif !invalidateDarwinSourceFD(&fd) {"
		const replacement = "\t\tinvalidateDarwinSourceFD := func(*int) bool { return true }\n" +
			"\t\tif !invalidateDarwinSourceFD(&fd) {"
		if count := strings.Count(string(darwinSource), original); count != 2 {
			t.Fatalf("Darwin fd invalidator fixture site count = %d, want 2", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{darwinPath: mutated}),
			),
			"\n",
		)
		want := "source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|" +
			"unix.Open|move-unapproved:<non-package>:invalidateDarwinSourceFD"
		if !strings.Contains(joined, want) {
			t.Fatalf("shadowed fd invalidator violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("shadowed descriptor kind helper", func(t *testing.T) {
		const original = "\t\tkindErr := requireDescriptorKind(int(owner.file.Fd()), openKind)"
		const replacement = `		realRequireDescriptorKind := requireDescriptorKind
		requireDescriptorKind := func(fd int, kind entryKind) error {
			return realRequireDescriptorKind(fd, kind)
		}
		kindErr := requireDescriptorKind(int(owner.file.Fd()), openKind)`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin descriptor kind helper fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{darwinPath: mutated}),
			),
			"\n",
		)
		want := "source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|" +
			"fd:<non-package>:requireDescriptorKind"
		if !strings.Contains(joined, want) {
			t.Fatalf("shadowed descriptor kind helper violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("shadowed acl helper", func(t *testing.T) {
		const original = "\t\terr := darwinFgetattrlist(int(owner.file.Fd()), &attributes, buffer)"
		const replacement = `		realDarwinFgetattrlist := darwinFgetattrlist
		darwinFgetattrlist := func(fd int, attributes *unix.Attrlist, buffer []byte) error {
			return realDarwinFgetattrlist(fd, attributes, buffer)
		}
		err := darwinFgetattrlist(int(owner.file.Fd()), &attributes, buffer)`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin ACL helper fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{darwinPath: mutated}),
			),
			"\n",
		)
		want := "source_primitives_darwin.go|(darwinSourcePrimitives).acquireRawACL|" +
			"fd:<non-package>:darwinFgetattrlist"
		if !strings.Contains(joined, want) {
			t.Fatalf("shadowed ACL helper violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("type erased origin in separate file", func(t *testing.T) {
		const original = `	owner := &ownedSourceDescriptor{
		kind:  sourceObservedDirectory,
		state: sourceHandleOpen,
	}
	var err error
	owner.file, err = root.root.Open(".")`
		const replacement = `	owner := &ownedSourceDescriptor{
		file:  sourceConstructionFixtureOpenDescriptor().(*os.File),
		kind:  sourceObservedDirectory,
		state: sourceHandleOpen,
	}
	dummy := &ownedSourceDescriptor{}
	var err error
	dummy.file, err = root.root.Open(".")`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin root descriptor fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		helperPath := filepath.Join(workingDirectory, "source_fd_helpers_darwin.go")
		helperSource := []byte(`package buildauthority

import "os"

func sourceConstructionFixtureOpenDescriptor() any {
	file, _ := os.Open("fixture")
	return file
}
`)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					darwinPath: mutated,
					helperPath: helperSource,
				}),
			),
			"\n",
		)
		want := "source descriptor file initializer outside audited site " +
			"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|" +
			"<non-call>"
		if !strings.Contains(joined, want) {
			t.Fatalf("separate-file origin violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("lookalike owner conversion in separate module", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureRelocatedDescriptor ownedSourceDescriptor

func sourceConstructionFixtureRelocateDescriptor(fd int) *ownedSourceDescriptor {
	temporary := sourceConstructionFixtureRelocatedDescriptor{
		os.NewFile(uintptr(fd), "fixture"),
		sourceObservedRegular,
		sourceHandleOpen,
		false,
	}
	owner := ownedSourceDescriptor(temporary)
	return &owner
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source descriptor construction outside audited site " +
			"authority_darwin.go|sourceConstructionFixtureRelocateDescriptor|conversion"
		if !strings.Contains(joined, want) {
			t.Fatalf("lookalike owner conversion violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("generic owner factory in separate module", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureGenericDescriptor ownedSourceDescriptor

type sourceConstructionFixtureDescriptorShape interface {
	ownedSourceDescriptor | sourceConstructionFixtureGenericDescriptor
}

func sourceConstructionFixtureCastDescriptor[
	To sourceConstructionFixtureDescriptorShape,
	From sourceConstructionFixtureDescriptorShape,
](value From) To {
	return To(value)
}

func sourceConstructionFixtureGenericFactory(fd int) *ownedSourceDescriptor {
	temporary := sourceConstructionFixtureGenericDescriptor{
		os.NewFile(uintptr(fd), "fixture"),
		sourceObservedRegular,
		sourceHandleOpen,
		false,
	}
	owner := sourceConstructionFixtureCastDescriptor[ownedSourceDescriptor](temporary)
	return &owner
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		for _, want := range []string{
			"source descriptor construction outside audited site " +
				"authority_darwin.go|sourceConstructionFixtureGenericFactory|" +
				"factory:sourceConstructionFixtureCastDescriptor",
			"source descriptor construction outside audited site " +
				"authority_darwin.go|sourceConstructionFixtureCastDescriptor|conversion",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("generic owner factory violations =\n%s\nwant %q", joined, want)
			}
		}
	})

	t.Run("generic tuple owner factory in separate module", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureTupleDescriptor ownedSourceDescriptor

type sourceConstructionFixtureTupleDescriptorShape interface {
	ownedSourceDescriptor | sourceConstructionFixtureTupleDescriptor
}

func sourceConstructionFixtureCastDescriptorTuple[
	To sourceConstructionFixtureTupleDescriptorShape,
	From sourceConstructionFixtureTupleDescriptorShape,
](value From) (To, error) {
	return To(value), nil
}

func sourceConstructionFixtureGenericTupleFactory(fd int) *ownedSourceDescriptor {
	temporary := sourceConstructionFixtureTupleDescriptor{
		os.NewFile(uintptr(fd), "fixture"),
		sourceObservedRegular,
		sourceHandleOpen,
		false,
	}
	owner, _ := sourceConstructionFixtureCastDescriptorTuple[ownedSourceDescriptor](temporary)
	return &owner
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source descriptor construction outside audited site " +
			"authority_darwin.go|sourceConstructionFixtureGenericTupleFactory|" +
			"factory:sourceConstructionFixtureCastDescriptorTuple"
		if !strings.Contains(joined, want) {
			t.Fatalf("generic tuple owner factory violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("type erased generic owner factory in separate module", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureErasedDescriptor ownedSourceDescriptor

type sourceConstructionFixtureErasedDescriptorShape interface {
	ownedSourceDescriptor | sourceConstructionFixtureErasedDescriptor
}

func sourceConstructionFixtureCastDescriptorErased[
	To sourceConstructionFixtureErasedDescriptorShape,
	From sourceConstructionFixtureErasedDescriptorShape,
](value From) any {
	return To(value)
}

func sourceConstructionFixtureErasedGenericFactory(fd int) *ownedSourceDescriptor {
	temporary := sourceConstructionFixtureErasedDescriptor{
		os.NewFile(uintptr(fd), "fixture"),
		sourceObservedRegular,
		sourceHandleOpen,
		false,
	}
	owner := sourceConstructionFixtureCastDescriptorErased[ownedSourceDescriptor](temporary).
		(ownedSourceDescriptor)
	return &owner
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		for _, want := range []string{
			"source descriptor construction outside audited site " +
				"authority_darwin.go|sourceConstructionFixtureErasedGenericFactory|" +
				"instantiation:sourceConstructionFixtureCastDescriptorErased",
			"source descriptor construction outside audited site " +
				"authority_darwin.go|sourceConstructionFixtureErasedGenericFactory|assertion",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("type-erased generic owner factory violations =\n%s\nwant %q", joined, want)
			}
			if count := strings.Count(joined, want); count != 1 {
				t.Fatalf(
					"type-erased generic owner factory violation %q count = %d, want 1:\n%s",
					want,
					count,
					joined,
				)
			}
		}
	})

	t.Run("type erased generic owner type switch in separate module", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureSwitchDescriptor ownedSourceDescriptor

type sourceConstructionFixtureSwitchDescriptorShape interface {
	ownedSourceDescriptor | sourceConstructionFixtureSwitchDescriptor
}

func sourceConstructionFixtureCastDescriptorForSwitch[
	To sourceConstructionFixtureSwitchDescriptorShape,
	From sourceConstructionFixtureSwitchDescriptorShape,
](value From) any {
	return To(value)
}

func sourceConstructionFixtureErasedGenericTypeSwitchFactory(fd int) *ownedSourceDescriptor {
	temporary := sourceConstructionFixtureSwitchDescriptor{
		os.NewFile(uintptr(fd), "fixture"),
		sourceObservedRegular,
		sourceHandleOpen,
		false,
	}
	switch owner := sourceConstructionFixtureCastDescriptorForSwitch[ownedSourceDescriptor](temporary).(type) {
	case ownedSourceDescriptor:
		return &owner
	default:
		return nil
	}
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source descriptor construction outside audited site " +
			"authority_darwin.go|sourceConstructionFixtureErasedGenericTypeSwitchFactory|" +
			"type-switch"
		if count := strings.Count(joined, want); count != 1 {
			t.Fatalf(
				"type-erased generic owner type-switch violation %q count = %d, want 1:\n%s",
				want,
				count,
				joined,
			)
		}
	})

	t.Run("generic owner wrapper factory in separate module", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureBoxDescriptor ownedSourceDescriptor

type sourceConstructionFixtureBoxDescriptorShape interface {
	ownedSourceDescriptor | sourceConstructionFixtureBoxDescriptor
}

type sourceConstructionFixtureDescriptorBox[
	Value sourceConstructionFixtureBoxDescriptorShape,
] struct {
	value Value
}

func sourceConstructionFixtureCastDescriptorBox[
	To sourceConstructionFixtureBoxDescriptorShape,
	From sourceConstructionFixtureBoxDescriptorShape,
](value From) sourceConstructionFixtureDescriptorBox[To] {
	return sourceConstructionFixtureDescriptorBox[To]{value: To(value)}
}

func sourceConstructionFixtureGenericBoxFactory(fd int) *ownedSourceDescriptor {
	temporary := sourceConstructionFixtureBoxDescriptor{
		os.NewFile(uintptr(fd), "fixture"),
		sourceObservedRegular,
		sourceHandleOpen,
		false,
	}
	owner := sourceConstructionFixtureCastDescriptorBox[ownedSourceDescriptor](temporary).value
	return &owner
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source descriptor construction outside audited site " +
			"authority_darwin.go|sourceConstructionFixtureGenericBoxFactory|" +
			"instantiation:sourceConstructionFixtureCastDescriptorBox"
		if count := strings.Count(joined, want); count != 1 {
			t.Fatalf(
				"generic owner wrapper factory violation %q count = %d, want 1:\n%s",
				want,
				count,
				joined,
			)
		}
	})

	t.Run("generic lookalike wrapper is not an owner construction", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureLookalikeDescriptor ownedSourceDescriptor

type sourceConstructionFixtureLookalikeBox[Value any] struct {
	value Value
}

func sourceConstructionFixtureLookalikeBoxFactory() sourceConstructionFixtureLookalikeDescriptor {
	boxed := sourceConstructionFixtureLookalikeBox[sourceConstructionFixtureLookalikeDescriptor]{}
	return boxed.value
}

func sourceConstructionFixtureLookalikeAssertionFactory() sourceConstructionFixtureLookalikeDescriptor {
	var erased any = sourceConstructionFixtureLookalikeDescriptor{}
	return erased.(sourceConstructionFixtureLookalikeDescriptor)
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		for _, function := range []string{
			"sourceConstructionFixtureLookalikeBoxFactory",
			"sourceConstructionFixtureLookalikeAssertionFactory",
		} {
			if strings.Contains(joined, function) {
				t.Fatalf("lookalike %s produced owner violation:\n%s", function, joined)
			}
		}
	})

	t.Run("package wide exact descriptor storage", func(t *testing.T) {
		const original = `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), name),
			kind:  kind,
			state: sourceHandleOpen,
		}`
		const replacement = original + `
		sourceConstructionFixtureStolenExactDescriptor = owner`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin relative owner fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

var sourceConstructionFixtureStolenExactDescriptor *ownedSourceDescriptor
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					darwinPath:    mutated,
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source descriptor owner enters package storage outside audited boundary " +
			"authority_darwin.go|<outside-function>"
		if !strings.Contains(joined, want) {
			t.Fatalf("exact descriptor storage violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("package wide exact root storage", func(t *testing.T) {
		const original = `	owner := &ownedSourceRoot{state: sourceHandleOpen}
	var err error
	owner.root, err = parent.root.OpenRoot(name)`
		const replacement = `	owner := &ownedSourceRoot{state: sourceHandleOpen}
	sourceConstructionFixtureStolenExactRoot = owner
	var err error
	owner.root, err = parent.root.OpenRoot(name)`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin child root owner fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

var sourceConstructionFixtureStolenExactRoot *ownedSourceRoot
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					darwinPath:    mutated,
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source root owner enters package storage outside audited boundary " +
			"authority_darwin.go|<outside-function>"
		if !strings.Contains(joined, want) {
			t.Fatalf("exact root storage violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("package wide nested descriptor storage", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureOwnerBox[Value any] struct {
	value Value
}

var sourceConstructionFixtureStolenDescriptorBox sourceConstructionFixtureOwnerBox[
	*ownedSourceDescriptor,
]
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source descriptor owner enters package storage outside audited boundary " +
			"authority_darwin.go|<outside-function>"
		if !strings.Contains(joined, want) {
			t.Fatalf("nested descriptor storage violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("descriptor representation loss", func(t *testing.T) {
		const original = `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), name),
			kind:  kind,
			state: sourceHandleOpen,
		}`
		const replacement = original + `
		_ = (*sourceConstructionFixtureRelocatedDescriptor)(owner)`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin relative owner fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureRelocatedDescriptor ownedSourceDescriptor
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					darwinPath:    mutated,
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source descriptor owner loses its nominal representation outside audited boundary " +
			"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor"
		if !strings.Contains(joined, want) {
			t.Fatalf("descriptor representation-loss violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("descriptor closure capture", func(t *testing.T) {
		const original = `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), name),
			kind:  kind,
			state: sourceHandleOpen,
		}`
		const replacement = original + `
		sourceConstructionFixtureStolenCallback = func() {
			_ = owner.validOpen()
		}`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin relative owner fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

var sourceConstructionFixtureStolenCallback func()
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					darwinPath:    mutated,
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source descriptor owner enters a function literal outside audited boundary " +
			"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor"
		if !strings.Contains(joined, want) {
			t.Fatalf("descriptor closure-capture violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("descriptor method value capture", func(t *testing.T) {
		const original = `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), name),
			kind:  kind,
			state: sourceHandleOpen,
		}`
		const replacement = original + `
		sourceConstructionFixtureStolenClose = owner.closeDirect`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin relative owner fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

var sourceConstructionFixtureStolenClose func() bool
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					darwinPath:    mutated,
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "source descriptor owner escapes through a method value outside audited boundary " +
			"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor"
		if !strings.Contains(joined, want) {
			t.Fatalf("descriptor method-value violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("descriptor asynchronous close", func(t *testing.T) {
		const original = `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), name),
			kind:  kind,
			state: sourceHandleOpen,
		}`
		const replacement = original + `
		go owner.closeDirect()`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin relative owner fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{darwinPath: mutated}),
			),
			"\n",
		)
		want := "source descriptor owner crosses a go boundary outside audited boundary " +
			"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor"
		if !strings.Contains(joined, want) {
			t.Fatalf("descriptor asynchronous-close violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("descriptor deferred close", func(t *testing.T) {
		const original = `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), name),
			kind:  kind,
			state: sourceHandleOpen,
		}`
		const replacement = original + `
		defer owner.closeDirect()`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin relative owner fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{darwinPath: mutated}),
			),
			"\n",
		)
		want := "source descriptor owner crosses a defer boundary outside audited boundary " +
			"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor"
		if !strings.Contains(joined, want) {
			t.Fatalf("descriptor deferred-close violations =\n%s\nwant %q", joined, want)
		}
	})

	t.Run("unused generic descriptor surface", func(t *testing.T) {
		mutated := append([]byte(nil), darwinSource...)
		mutated = append(mutated, []byte(`

func sourceConstructionFixtureEraseDescriptor[Value ~*ownedSourceDescriptor](value Value) any {
	return value
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{darwinPath: mutated}),
			),
			"\n",
		)
		if !strings.Contains(joined, "sourceConstructionFixtureEraseDescriptor") {
			t.Fatalf("generic descriptor surface violations =\n%s", joined)
		}
	})

	t.Run("unrelated package lookalike storage remains allowed", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureUnrelatedDescriptor struct {
	file         *os.File
	kind         sourceObservedKind
	state        sourceHandleState
	closeFailure bool
}

var sourceConstructionFixtureUnrelatedStorage *sourceConstructionFixtureUnrelatedDescriptor
`)...)
		if violations := sourceConstructionDescriptorFileCapabilityViolations(
			loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
				authorityPath: authoritySource,
			}),
		); len(violations) != 0 {
			t.Fatalf("unrelated lookalike storage violations:\n%s", strings.Join(violations, "\n"))
		}
	})

	t.Run("package wide interface retention in primitive", func(t *testing.T) {
		const original = `		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), name),
			kind:  kind,
			state: sourceHandleOpen,
		}`
		const replacement = original + `
		sourceConstructionFixtureStolenDescriptor = owner`
		if count := strings.Count(string(darwinSource), original); count != 1 {
			t.Fatalf("Darwin relative owner fixture site count = %d, want 1", count)
		}
		mutated := []byte(strings.Replace(string(darwinSource), original, replacement, 1))
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

var sourceConstructionFixtureStolenDescriptor interface {
	closeDirect() bool
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					darwinPath:    mutated,
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		want := "erases source descriptor owner into interface outside audited boundary " +
			"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|assignment"
		if count := strings.Count(joined, want); count != 1 {
			t.Fatalf(
				"package-wide interface retention violation %q count = %d, want 1:\n%s",
				want,
				count,
				joined,
			)
		}
	})

	t.Run("package wide interface erasure edges", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

var sourceConstructionFixtureErasedDescriptorSink any

func sourceConstructionFixtureConsumeErasedDescriptor(any) {}

func sourceConstructionFixtureEraseDescriptorEdges(
	owner *ownedSourceDescriptor,
	output chan any,
) any {
	sourceConstructionFixtureErasedDescriptorSink = owner
	var local any = owner
	sourceConstructionFixtureConsumeErasedDescriptor(owner)
	_ = append([]any{}, owner)
	_ = any(owner)
	_ = []any{owner}
	_ = struct{ value any }{value: owner}
	_ = map[any]any{owner: owner}
	erasedMap := map[any]any{}
	erasedMap[owner] = nil
	owners := []*ownedSourceDescriptor{owner}
	for _, sourceConstructionFixtureErasedDescriptorSink = range owners {
		break
	}
	ownerMap := map[int]*ownedSourceDescriptor{0: owner}
	sourceConstructionFixtureErasedDescriptorSink, _ = ownerMap[0]
	output <- owner
	_ = local
	return owner
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		for _, want := range []string{
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|assignment",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|value-spec",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|" +
				"call-argument:sourceConstructionFixtureConsumeErasedDescriptor",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|" +
				"call-argument:append",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|conversion",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|composite-element",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|composite-field",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|composite-map-key",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|composite-map-value",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|map-index-key",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|range-value",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|assignment-comma-ok",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|channel-send",
			"authority_darwin.go|sourceConstructionFixtureEraseDescriptorEdges|return",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("interface erasure edge violations =\n%s\nwant %q", joined, want)
			}
		}
	})

	t.Run("generic instance owner graphs", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

type sourceConstructionFixtureOwnerAlias = ownedSourceDescriptor

type sourceConstructionFixtureOwnerToken[Value any] struct{}

func sourceConstructionFixtureGenericIdentity[Value any](value Value) Value {
	return value
}

func sourceConstructionFixtureGenericInstanceEdges(
	owner ownedSourceDescriptor,
	erased any,
) {
	_ = sourceConstructionFixtureGenericIdentity(owner)
	_ = sourceConstructionFixtureGenericIdentity[ownedSourceDescriptor]
	_ = sourceConstructionFixtureOwnerToken[[]*ownedSourceDescriptor]{}
	_ = sourceConstructionFixtureOwnerToken[**ownedSourceDescriptor]{}
	_ = sourceConstructionFixtureOwnerToken[struct{ owner *ownedSourceDescriptor }]{}
	_ = sourceConstructionFixtureOwnerToken[map[string]*ownedSourceDescriptor]{}
	_ = sourceConstructionFixtureOwnerToken[interface {
		take() *ownedSourceDescriptor
	}]{}
	_ = sourceConstructionFixtureOwnerToken[sourceConstructionFixtureOwnerAlias]{}
	_ = erased.([]*ownedSourceDescriptor)
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		for want, count := range map[string]int{
			"authority_darwin.go|sourceConstructionFixtureGenericInstanceEdges|" +
				"instantiation:sourceConstructionFixtureGenericIdentity": 2,
			"authority_darwin.go|sourceConstructionFixtureGenericInstanceEdges|" +
				"instantiation:<non-function>:sourceConstructionFixtureOwnerToken": 6,
			"authority_darwin.go|sourceConstructionFixtureGenericInstanceEdges|assertion": 1,
		} {
			if got := strings.Count(joined, want); got != count {
				t.Fatalf(
					"generic instance owner-graph violation %q count = %d, want %d:\n%s",
					want,
					got,
					count,
					joined,
				)
			}
		}
	})

	t.Run("generic structural interface sinks", func(t *testing.T) {
		authorityPath := filepath.Join(workingDirectory, "authority_darwin.go")
		authoritySource, readErr := os.ReadFile(authorityPath)
		if readErr != nil {
			t.Fatalf("read Darwin authority primitives: %v", readErr)
		}
		authoritySource = append(authoritySource, []byte(`

func sourceConstructionFixtureGenericMapSink[Mapping ~map[any]struct{}](
	values Mapping,
	owner *ownedSourceDescriptor,
) {
	values[owner] = struct{}{}
}

type sourceConstructionFixtureBroadMap interface {
	~map[any]struct{} | ~map[string]struct{}
}

type sourceConstructionFixtureNarrowMap interface {
	sourceConstructionFixtureBroadMap
	~map[any]struct{}
}

func sourceConstructionFixtureGenericIntersectionMapSink[
	Mapping sourceConstructionFixtureNarrowMap,
](values Mapping, owner *ownedSourceDescriptor) {
	values[owner] = struct{}{}
}

func sourceConstructionFixtureGenericChannelSink[Channel ~chan any](
	output Channel,
	owner *ownedSourceDescriptor,
) {
	output <- owner
}

func sourceConstructionFixtureGenericCallSink[Function ~func(any)](
	consume Function,
	owner *ownedSourceDescriptor,
) {
	consume(owner)
}

func sourceConstructionFixtureGenericCompositeSink[Sequence ~[]any](
	owner *ownedSourceDescriptor,
) {
	_ = Sequence{owner}
}

func sourceConstructionFixtureGenericRangeSink[Sequence ~[]*ownedSourceDescriptor](
	owners Sequence,
) {
	var stolen any
	for _, stolen = range owners {
		break
	}
	_ = stolen
}

func sourceConstructionFixtureUseGenericStructuralSinks(owner *ownedSourceDescriptor) {
	sourceConstructionFixtureGenericMapSink(map[any]struct{}{}, owner)
	sourceConstructionFixtureGenericIntersectionMapSink(map[any]struct{}{}, owner)
	sourceConstructionFixtureGenericChannelSink(make(chan any, 1), owner)
	sourceConstructionFixtureGenericCallSink(func(any) {}, owner)
	sourceConstructionFixtureGenericCompositeSink[[]any](owner)
}
`)...)
		joined := strings.Join(
			sourceConstructionDescriptorFileCapabilityViolations(
				loadTypedDarwinSourceConstructionPackage(t, map[string][]byte{
					authorityPath: authoritySource,
				}),
			),
			"\n",
		)
		for _, want := range []string{
			"authority_darwin.go|sourceConstructionFixtureGenericMapSink|map-index-key",
			"authority_darwin.go|sourceConstructionFixtureGenericIntersectionMapSink|" +
				"map-index-key",
			"authority_darwin.go|sourceConstructionFixtureGenericChannelSink|channel-send",
			"authority_darwin.go|sourceConstructionFixtureGenericCallSink|" +
				"call-argument:<non-package>:consume",
			"authority_darwin.go|sourceConstructionFixtureGenericCompositeSink|" +
				"composite-element",
			"authority_darwin.go|sourceConstructionFixtureGenericRangeSink|range-value",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("generic structural interface-sink violations =\n%s\nwant %q", joined, want)
			}
		}
	})
}

func TestSourceConstructionCallObjectRoleBindsExternalPackagePath(t *testing.T) {
	t.Parallel()

	signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	for _, test := range []struct {
		name        string
		path        string
		packageName string
		symbol      string
		want        string
	}{
		{
			name:        "standard os",
			path:        "os",
			packageName: "os",
			symbol:      "NewFile",
			want:        "os.NewFile",
		},
		{
			name:        "exact x sys unix",
			path:        "golang.org/x/sys/unix",
			packageName: "unix",
			symbol:      "Open",
			want:        "unix.Open",
		},
		{
			name:        "same name os forwarder",
			path:        "example.invalid/forward/os",
			packageName: "os",
			symbol:      "NewFile",
			want:        "example.invalid/forward/os.NewFile",
		},
		{
			name:        "same name unix forwarder",
			path:        "example.invalid/forward/unix",
			packageName: "unix",
			symbol:      "Openat",
			want:        "example.invalid/forward/unix.Openat",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			object := types.NewFunc(
				token.NoPos,
				types.NewPackage(test.path, test.packageName),
				test.symbol,
				signature,
			)
			got := sourceConstructionCallObjectRole(
				object,
				"github.com/vbonnet/dear-agent/internal/buildauthority",
			)
			if got != test.want {
				t.Fatalf("external call role = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSourceConstructionTypeResolvedEscapeGuardRejectsBypasses(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	constructionPath := filepath.Join(workingDirectory, "source_construction.go")
	acquirePath := filepath.Join(workingDirectory, "source_construction_acquire.go")
	inventoryPath := filepath.Join(workingDirectory, "source_construction_inventory.go")
	primitivesPath := filepath.Join(workingDirectory, "source_primitives.go")
	darwinPrimitivesPath := filepath.Join(workingDirectory, "source_primitives_darwin.go")
	constructionSource, err := os.ReadFile(constructionPath)
	if err != nil {
		t.Fatalf("read source construction fixture base: %v", err)
	}
	acquireSource, err := os.ReadFile(acquirePath)
	if err != nil {
		t.Fatalf("read source construction acquisition fixture base: %v", err)
	}
	inventorySource, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatalf("read source construction inventory fixture base: %v", err)
	}
	primitivesSource, err := os.ReadFile(primitivesPath)
	if err != nil {
		t.Fatalf("read source primitives fixture base: %v", err)
	}
	darwinPrimitivesSource, err := os.ReadFile(darwinPrimitivesPath)
	if err != nil {
		t.Fatalf("read Darwin source primitives fixture base: %v", err)
	}
	withForwardCloser := strings.Replace(
		string(constructionSource),
		"type sourceHandleCloser interface {\n\tcloseRoot(*ownedSourceRoot) bool\n\tcloseDescriptor(*ownedSourceDescriptor) bool\n}",
		"type sourceHandleCloser interface {\n\tcloseRoot(*ownedSourceRoot) bool\n\tcloseDescriptor(*ownedSourceDescriptor) bool\n\tforward(*ownedSourceRoot) *ownedSourceRoot\n}",
		1,
	)
	if withForwardCloser == string(constructionSource) {
		t.Fatal("source construction fixture could not extend sourceHandleCloser")
	}
	constructionSource = []byte(withForwardCloser)
	withPrimitiveStash := strings.Replace(
		string(constructionSource),
		"\towner.mu.Lock()\n\tdefer owner.mu.Unlock()\n\towner.closeLocked(primitives, outcome)",
		"\tprimitives.stash(owner.repository.root.root)\n\towner.mu.Lock()\n\tdefer owner.mu.Unlock()\n\towner.closeLocked(primitives, outcome)",
		1,
	)
	if withPrimitiveStash == string(constructionSource) {
		t.Fatal("source construction fixture could not call sourcePrimitives stash")
	}
	constructionSource = []byte(withPrimitiveStash)

	constructionSource = append(constructionSource, []byte(`

func sourceConstructionFixtureSnapshot(owner *sourceConstructionOwner) (sourceConstructionFixtureAlias) {
	recordSourceConstructionFixture(owner.config.descriptor.file)
	recordSourceConstructionFixtureBox(struct{ value any }{value: owner.config.descriptor.file})
	sourceConstructionFixtureSink = owner.config.descriptor.file
	sourceConstructionFixtureRead = owner.config.descriptor.file.Read
	sourceConstructionFixtureStateSink = &owner.config.descriptor.state
	sourceConstructionFixtureStateStarSink = &*(&owner.config.descriptor.state)
	sourceConstructionFixtureCloseFailureSink = &owner.closeFailure
	sourceConstructionFixtureClaimsSink = owner.repository.pathClaims[:]
	sourceConstructionFixtureClaimElementSink = &owner.repository.pathClaims[0]
	sourceConstructionFixtureDigestSink = owner.config.digest[:]
	sourceConstructionFixtureEntriesSink = owner.config.claim.entries[:]
	sourceConstructionFixtureStateMapSink = map[*sourceHandleState]struct{}{
		&owner.config.descriptor.state: {},
	}
	sourceConstructionFixtureStateMapSink[&owner.config.descriptor.state] = struct{}{}
	sourceConstructionFixtureStateMapCount[&owner.config.descriptor.state]++
	for sourceConstructionFixtureStateRangeCount[&owner.config.descriptor.state] = range []int{1} {
	}
	(&owner.config.descriptor.state).sourceConstructionFixtureEscapeState()
	closePackedRefs(owner.packedRefs.leaf)
	_ = sourceConstructionFixtureFromGlobal()
	copy(sourceConstructionFixtureAny, []any{owner.config.descriptor.file})
	_ = append(sourceConstructionFixtureAny, owner.config.descriptor.file)
	go sourceConstructionFixtureConsume(owner)
	sourceConstructionFixtureReaderSink = sourceConstructionFixtureReader(owner)
	return sourceConstructionFixtureBox{file: owner.config.descriptor.file}
}

func sourceConstructionFixtureConsume(*sourceConstructionOwner) {}

func sourceConstructionFixtureBuilderEscape(builder *sourceConstructionBuilder) {
	sourceConstructionFixtureContextPointerSink = &builder.ctx
	sourceConstructionFixturePrimitivesPointerSink = &builder.primitives
	sourceConstructionFixtureOutcomePointerSink = &builder.outcome
	builder.primitives.openRepositoryRoot(builder.ctx, sourceRepositoryLocator{})
	_, _ = builder.primitives.openRepositoryRoot(builder.ctx, sourceRepositoryLocator{})
	defer builder.primitives.openRepositoryRoot(builder.ctx, sourceRepositoryLocator{})
	sourceConstructionFixtureDiscard(builder.primitives.openRepositoryRoot(
		builder.ctx,
		sourceRepositoryLocator{},
	))
	newSourceConstructionOwner()
	_ = newSourceConstructionOwner()
}

func sourceConstructionFixtureDiscard(_ *ownedSourceRoot, _ *sourcePrimitiveFailure) {}

func sourceConstructionFixtureForward(
	closer sourceHandleCloser,
	owner *sourceConstructionOwner,
) {
	sourceConstructionFixtureRootSink = closer.forward(owner.repository.root.root)
}

func sourceConstructionFixtureNamedDiscard(
	builder *sourceConstructionBuilder,
	locator sourceRepositoryLocator,
) bool {
	root, failure := builder.primitives.openRepositoryRoot(builder.ctx, locator)
	if root == nil || failure != nil {
		return false
	}
	return false
}

func sourceConstructionFixtureOwnerDiscard(
	ctx context.Context,
	locator sourceRepositoryLocator,
) {
	owner, outcome := retainSourceConstruction(ctx, locator)
	if owner == nil || !outcome.proved() {
		return
	}
}

func sourceConstructionFixtureOwnerWithDiscard(
	ctx context.Context,
	locator sourceRepositoryLocator,
	primitives sourcePrimitives,
) {
	owner, outcome := retainSourceConstructionWith(ctx, locator, primitives)
	if owner == nil || !outcome.proved() {
		return
	}
}

func closePackedRefs(_ *retainedSourcePackedRefs) {}

func (tracker *sourceCloseTracker) retainSourceConstruction() *sourceConstructionOwner {
	return newSourceConstructionOwner()
}

func sourceConstructionFixtureReader(owner *sourceConstructionOwner) sourceConstructionFixtureReaderAlias {
	return owner.config.descriptor.file
}

func (sink *sourceConstructionFixtureReceiver) sourceConstructionFixtureCache(owner *sourceConstructionOwner) {
	sink.file = owner.config.descriptor.file
}
`)...)
	withIO := strings.Replace(
		string(primitivesSource),
		"import (\n\t\"context\"\n",
		"import (\n\t\"context\"\n\t\"io\"\n",
		1,
	)
	if withIO == string(primitivesSource) {
		t.Fatal("source primitives fixture could not add io import")
	}
	primitivesSource = []byte(withIO)
	withForwardPrimitives := strings.Replace(
		string(primitivesSource),
		"\tcloseDescriptor(*ownedSourceDescriptor) bool\n\tprivateSourcePrimitives()",
		"\tcloseDescriptor(*ownedSourceDescriptor) bool\n\tforward(*ownedSourceRoot) *ownedSourceRoot\n\tstash(any)\n\tprivateSourcePrimitives()",
		1,
	)
	if withForwardPrimitives == string(primitivesSource) {
		t.Fatal("source primitives fixture could not extend sourcePrimitives")
	}
	primitivesSource = []byte(withForwardPrimitives)
	withVariadicParse := strings.Replace(
		string(primitivesSource),
		"parseRawACL(context.Context, []byte)",
		"parseRawACL(context.Context, ...byte)",
		1,
	)
	if withVariadicParse == string(primitivesSource) {
		t.Fatal("source primitives fixture could not make parseRawACL variadic")
	}
	primitivesSource = []byte(withVariadicParse)
	primitivesSource = append(primitivesSource, []byte(`

var sourceConstructionFixtureSink *os.File
var sourceConstructionFixtureAny []any
var sourceConstructionFixtureReaderSink io.Reader
var sourceConstructionFixtureRead func([]byte) (int, error)
var sourceConstructionFixtureStateSink *sourceHandleState
var sourceConstructionFixtureStateStarSink *sourceHandleState
var sourceConstructionFixtureCloseFailureSink *bool
var sourceConstructionFixtureClaimsSink []authorityPathClaim
var sourceConstructionFixtureClaimElementSink *authorityPathClaim
var sourceConstructionFixtureDigestSink []byte
var sourceConstructionFixtureEntriesSink []sourceConfigEntry
var sourceConstructionFixtureContextPointerSink *context.Context
var sourceConstructionFixturePrimitivesPointerSink *sourcePrimitives
var sourceConstructionFixtureOutcomePointerSink *sourceUseOutcome
var sourceConstructionFixtureStateMapSink map[*sourceHandleState]struct{}
var sourceConstructionFixtureStateMapCount map[*sourceHandleState]int
var sourceConstructionFixtureStateRangeCount map[*sourceHandleState]int
var sourceConstructionFixtureMethodStateSink *sourceHandleState
var sourceConstructionFixtureRootSink *ownedSourceRoot

type sourceConstructionFixtureBox struct {
	file *os.File
}

type sourceConstructionFixtureAlias = sourceConstructionFixtureBox
type sourceConstructionFixtureReaderAlias = io.Reader

type sourceConstructionFixtureReceiver struct {
	file *os.File
}

func recordSourceConstructionFixture(file *os.File) {
	sourceConstructionFixtureSink = file
}

func recordSourceConstructionFixtureBox(struct{ value any }) {}

func sourceConstructionFixtureFromGlobal() *os.File {
	return sourceConstructionFixtureSink
}

func (state *sourceHandleState) sourceConstructionFixtureEscapeState() {
	sourceConstructionFixtureMethodStateSink = state
}

func (directSourceHandleCloser) forward(owner *ownedSourceRoot) *ownedSourceRoot {
	return owner
}
`)...)
	darwinPrimitivesSource = append(darwinPrimitivesSource, []byte(`

func (darwinSourcePrimitives) forward(owner *ownedSourceRoot) *ownedSourceRoot {
	return owner
}

func (darwinSourcePrimitives) stash(value any) {
	if owner, ok := value.(*ownedSourceRoot); ok {
		sourceConstructionFixtureRootSink = owner
	}
}
`)...)
	withVariadicDarwinParse := strings.Replace(
		string(darwinPrimitivesSource),
		"\traw []byte,\n) (parsedSourceACL, *sourcePrimitiveFailure)",
		"\traw ...byte,\n) (parsedSourceACL, *sourcePrimitiveFailure)",
		1,
	)
	if withVariadicDarwinParse == string(darwinPrimitivesSource) {
		t.Fatal("Darwin source primitives fixture could not make parseRawACL variadic")
	}
	darwinPrimitivesSource = []byte(withVariadicDarwinParse)
	const parseRawACLCall = "builder.primitives.parseRawACL(builder.ctx, rawACL)"
	if count := strings.Count(string(acquireSource), parseRawACLCall); count != 1 {
		t.Fatalf("source construction acquisition parseRawACL call count = %d, want 1", count)
	}
	withVariadicParseCall := strings.ReplaceAll(
		string(acquireSource),
		parseRawACLCall,
		"builder.primitives.parseRawACL(builder.ctx, rawACL...)",
	)
	acquireSource = []byte(withVariadicParseCall)
	if count := strings.Count(string(inventorySource), parseRawACLCall); count != 1 {
		t.Fatalf("source construction inventory parseRawACL call count = %d, want 1", count)
	}
	inventorySource = []byte(strings.ReplaceAll(
		string(inventorySource),
		parseRawACLCall,
		"builder.primitives.parseRawACL(builder.ctx, rawACL...)",
	))
	typedPackage := loadTypedSourceConstructionPackage(t, map[string][]byte{
		constructionPath:     constructionSource,
		acquirePath:          acquireSource,
		inventoryPath:        inventorySource,
		primitivesPath:       primitivesSource,
		darwinPrimitivesPath: darwinPrimitivesSource,
	})
	violations := sourceConstructionEscapeViolations(typedPackage)
	joined := strings.Join(violations, "\n")
	for _, want := range []string{
		"passes os.File to non-construction call recordSourceConstructionFixture",
		"passes os.File to non-construction call recordSourceConstructionFixtureBox",
		"writes os.File into package or type-erasing storage sourceConstructionFixtureSink",
		"writes os.File into package or type-erasing storage sourceConstructionFixtureRead",
		"into package or type-erasing storage sourceConstructionFixtureStateSink",
		"into package or type-erasing storage sourceConstructionFixtureStateStarSink",
		"into package or type-erasing storage sourceConstructionFixtureCloseFailureSink",
		"into package or type-erasing storage sourceConstructionFixtureClaimsSink",
		"into package or type-erasing storage sourceConstructionFixtureClaimElementSink",
		"into package or type-erasing storage sourceConstructionFixtureDigestSink",
		"into package or type-erasing storage sourceConstructionFixtureEntriesSink",
		"into package or type-erasing storage sourceConstructionFixtureContextPointerSink",
		"into package or type-erasing storage sourceConstructionFixturePrimitivesPointerSink",
		"into package or type-erasing storage sourceConstructionFixtureOutcomePointerSink",
		"into package or type-erasing storage sourceConstructionFixtureStateMapSink",
		"as a map key in sourceConstructionFixtureStateMapSink",
		"as a map key in sourceConstructionFixtureStateMapCount",
		"as a map key in sourceConstructionFixtureStateRangeCount",
		"to non-construction call sourceConstructionFixtureEscapeState",
		"sourceHandleCloser declares non-close method or signature forward",
		"to non-construction call forward",
		"sourcePrimitives declares non-primitive method or signature forward",
		"sourcePrimitives declares non-primitive method or signature stash",
		"sourcePrimitives declares non-primitive method or signature parseRawACL",
		"to non-construction call stash",
		"discards ownedSourceRoot result",
		"writes ownedSourceRoot into package or type-erasing storage _",
		"defers and discards ownedSourceRoot result",
		"consumes ownedSourceRoot returned by nested call",
		"outside audited site openRepositoryRoot|sourceConstructionFixtureNamedDiscard|:=|root,failure",
		"outside audited site retainSourceConstruction|sourceConstructionFixtureOwnerDiscard|:=:owner,outcome",
		"outside audited site retainSourceConstructionWith|sourceConstructionFixtureOwnerWithDiscard|:=:owner,outcome",
		"sourceConstructionFixtureDiscard declares unapproved sensitive parameter *ownedSourceRoot",
		"closePackedRefs declares unapproved sensitive parameter *retainedSourcePackedRefs",
		"(*sourceCloseTracker).retainSourceConstruction returns sourceConstructionOwner",
		"discards sourceConstructionOwner result",
		"writes sourceConstructionOwner into package or type-erasing storage _",
		"receives os.File from non-construction call sourceConstructionFixtureFromGlobal",
		"to non-construction call copy",
		"to non-construction call append",
		"starts asynchronous work inside the source-construction owner boundary",
		"returns interface through Reader",
		"returns os.File from sourceConstructionFixtureReader",
		"declares construction method on non-construction receiver",
		"writes os.File into package or type-erasing storage sink.file",
		"returns os.File through sourceConstructionFixtureBox",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("type-resolved guard violations =\n%s\nwant substring %q", joined, want)
		}
	}
}

type scriptedSourceProbeResult struct {
	kind    sourceObservedKind
	present bool
}

type sourceConstructionStepContext struct {
	cancelAt int
	samples  int
}

func (*sourceConstructionStepContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (*sourceConstructionStepContext) Done() <-chan struct{} {
	return nil
}

func (ctx *sourceConstructionStepContext) Err() error {
	ctx.samples++
	if ctx.samples >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func (*sourceConstructionStepContext) Value(any) any {
	return nil
}

type scriptedSourceConstructionPrimitives struct {
	t *testing.T

	locator                    sourceRepositoryLocator
	events                     []string
	config                     []byte
	packedRefs                 []byte
	packedRefsPresent          bool
	failureEvent               string
	ownerFailureEvent          string
	cancelEvent                string
	cancel                     context.CancelFunc
	nilOwnerEvent              string
	invalidACLEvent            string
	failure                    *sourcePrimitiveFailure
	configPostReadDrift        string
	configRebindDrift          string
	packedRefsPostReadDrift    string
	packedRefsRebindDrift      string
	closeFailures              map[string]bool
	probeResults               map[string]scriptedSourceProbeResult
	deviceOverrides            map[string]uint64
	filesystemOverrides        map[string][2]int32
	linkCountOverrides         map[string]uint64
	sizeOverrides              map[string]int64
	acquiredRoots              map[string]bool
	acquiredDescriptors        map[string]bool
	roots                      map[string]*ownedSourceRoot
	rootNames                  map[*ownedSourceRoot]string
	descriptors                map[string]*ownedSourceDescriptor
	descriptorNames            map[*ownedSourceDescriptor]string
	observation                map[*ownedSourceDescriptor]string
	configObservationCount     int
	packedRefsObservationCount int
	lastFilesystem             string
	lastMount                  mountSnapshot
	lastACL                    string
	lastRead                   string
}

func newScriptedSourceConstructionPrimitives(
	t *testing.T,
	config []byte,
) *scriptedSourceConstructionPrimitives {
	t.Helper()
	primitives := &scriptedSourceConstructionPrimitives{
		t:                   t,
		locator:             testSourceRepositoryLocator(),
		config:              append([]byte(nil), config...),
		closeFailures:       make(map[string]bool),
		probeResults:        make(map[string]scriptedSourceProbeResult),
		deviceOverrides:     make(map[string]uint64),
		filesystemOverrides: make(map[string][2]int32),
		linkCountOverrides:  make(map[string]uint64),
		sizeOverrides:       make(map[string]int64),
		acquiredRoots:       make(map[string]bool),
		acquiredDescriptors: make(map[string]bool),
		roots:               make(map[string]*ownedSourceRoot),
		rootNames:           make(map[*ownedSourceRoot]string),
		descriptors:         make(map[string]*ownedSourceDescriptor),
		descriptorNames:     make(map[*ownedSourceDescriptor]string),
		observation:         make(map[*ownedSourceDescriptor]string),
	}
	for _, name := range []string{"repository", "git", "objects"} {
		owner := &ownedSourceRoot{root: &os.Root{}, state: sourceHandleOpen}
		primitives.roots[name] = owner
		primitives.rootNames[owner] = name
	}
	for _, entry := range []struct {
		name string
		kind sourceObservedKind
	}{
		{name: "physical-root", kind: sourceObservedDirectory},
		{name: "parent", kind: sourceObservedDirectory},
		{name: "repository", kind: sourceObservedDirectory},
		{name: "git", kind: sourceObservedDirectory},
		{name: "config", kind: sourceObservedRegular},
		{name: "config-rebind", kind: sourceObservedRegular},
		{name: "objects", kind: sourceObservedDirectory},
		{name: "packed-refs", kind: sourceObservedRegular},
		{name: "packed-refs-rebind", kind: sourceObservedRegular},
	} {
		owner := &ownedSourceDescriptor{
			file:  &os.File{},
			kind:  entry.kind,
			state: sourceHandleOpen,
		}
		primitives.descriptors[entry.name] = owner
		primitives.descriptorNames[owner] = entry.name
	}
	return primitives
}

func (*scriptedSourceConstructionPrimitives) privateSourcePrimitives() {}

func (primitives *scriptedSourceConstructionPrimitives) openRepositoryRoot(
	ctx context.Context,
	locator sourceRepositoryLocator,
) (*ownedSourceRoot, *sourcePrimitiveFailure) {
	if ctx == nil || locator != primitives.locator {
		primitives.t.Fatalf("open repository inputs = %#v / %+v", locator, ctx)
	}
	return primitives.openRootResult("repository", "open-root:repository")
}

func (primitives *scriptedSourceConstructionPrimitives) openPhysicalRootDescriptor(
	ctx context.Context,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	if ctx == nil {
		primitives.t.Fatal("open physical root received nil context")
	}
	return primitives.openDescriptorResult("physical-root", "open-descriptor:physical-root")
}

func (primitives *scriptedSourceConstructionPrimitives) probeRelativeKind(
	ctx context.Context,
	parent *ownedSourceDescriptor,
	name string,
	mode sourcePresenceMode,
) (sourceObservedKind, bool, *sourcePrimitiveFailure) {
	if ctx == nil {
		primitives.t.Fatalf("probe %q inputs = %+v / %d", name, ctx, mode)
	}
	parentName := primitives.descriptorName(parent)
	var kind sourceObservedKind
	present := true
	event := "probe:" + strings.TrimPrefix(name, ".")
	switch {
	case parentName == "repository" && name == ".git" && mode == sourceInitialRequired:
		kind = sourceObservedDirectory
	case parentName == "git" && name == "config" && mode == sourceInitialRequired:
		kind = sourceObservedRegular
	case parentName == "git" && name == "objects" && mode == sourceInitialRequired:
		kind = sourceObservedDirectory
	case parentName == "git" && name == "packed-refs" && mode == sourceInitialOptional:
		present = primitives.packedRefsPresent
		if present {
			kind = sourceObservedRegular
		}
		event = "probe:packed-refs:initial"
	case parentName == "git" && name == "packed-refs" && mode == sourceRevalidatePresent:
		kind = sourceObservedRegular
		present = primitives.packedRefsPresent
		event = "probe:packed-refs:rebind-present"
	case parentName == "git" && name == "packed-refs" && mode == sourceRevalidateAbsent:
		kind = 0
		present = false
		event = "probe:packed-refs:rebind-absent"
	default:
		primitives.t.Fatalf("unexpected source probe %s/%s mode=%d", parentName, name, mode)
	}
	if failure := primitives.record(event); failure != nil {
		return 0, false, failure
	}
	if result, overridden := primitives.probeResults[event]; overridden {
		return result.kind, result.present, nil
	}
	return kind, present, nil
}

func (primitives *scriptedSourceConstructionPrimitives) openRelativeNoFollow(
	ctx context.Context,
	parent *ownedSourceDescriptor,
	name string,
	kind sourceObservedKind,
	mode sourcePresenceMode,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	if ctx == nil {
		primitives.t.Fatal("relative open received nil context")
	}
	parentName := primitives.descriptorName(parent)
	var ownerName, event string
	switch {
	case parentName == "physical-root" && name == "parent" &&
		kind == sourceObservedDirectory && mode == sourceInitialRequired:
		ownerName, event = "parent", "open-descriptor:parent"
	case parentName == "parent" && name == "repository" &&
		kind == sourceObservedDirectory && mode == sourceInitialRequired:
		ownerName, event = "repository", "open-descriptor:repository"
	case parentName == "physical-root" && name == "repository" &&
		kind == sourceObservedDirectory && mode == sourceInitialRequired:
		ownerName, event = "repository", "open-descriptor:repository"
	case parentName == "repository" && name == ".git" &&
		kind == sourceObservedDirectory && mode == sourceInitialRequired:
		ownerName, event = "git", "open-descriptor:git"
	case parentName == "git" && name == "config" &&
		kind == sourceObservedRegular && mode == sourceInitialRequired:
		ownerName, event = "config", "open-descriptor:config"
	case parentName == "git" && name == "config" &&
		kind == sourceObservedRegular && mode == sourceRevalidatePresent:
		ownerName, event = "config-rebind", "open-descriptor:config-rebind"
	case parentName == "git" && name == "objects" &&
		kind == sourceObservedDirectory && mode == sourceInitialRequired:
		ownerName, event = "objects", "open-descriptor:objects"
	case parentName == "git" && name == "packed-refs" &&
		kind == sourceObservedRegular && mode == sourceInitialOptional:
		ownerName, event = "packed-refs", "open-descriptor:packed-refs"
	case parentName == "git" && name == "packed-refs" &&
		kind == sourceObservedRegular && mode == sourceRevalidatePresent:
		ownerName, event = "packed-refs-rebind", "open-descriptor:packed-refs-rebind"
	default:
		primitives.t.Fatalf(
			"unexpected relative open parent=%s name=%q kind=%d mode=%d",
			parentName,
			name,
			kind,
			mode,
		)
	}
	return primitives.openDescriptorResult(ownerName, event)
}

func (primitives *scriptedSourceConstructionPrimitives) openChildRoot(
	ctx context.Context,
	parent *ownedSourceRoot,
	name string,
) (*ownedSourceRoot, *sourcePrimitiveFailure) {
	if ctx == nil {
		primitives.t.Fatal("child root open received nil context")
	}
	parentName := primitives.rootName(parent)
	var ownerName, event string
	switch {
	case parentName == "repository" && name == ".git":
		ownerName, event = "git", "open-root:git"
	case parentName == "git" && name == "objects":
		ownerName, event = "objects", "open-root:objects"
	default:
		primitives.t.Fatalf("unexpected child root open %s/%s", parentName, name)
	}
	return primitives.openRootResult(ownerName, event)
}

func (primitives *scriptedSourceConstructionPrimitives) openRootDirectoryDescriptor(
	context.Context,
	*ownedSourceRoot,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	primitives.t.Fatal("unexpected source root-directory descriptor open")
	return nil, nil
}

func (primitives *scriptedSourceConstructionPrimitives) readDirectoryBatch(
	context.Context,
	*ownedSourceDescriptor,
) ([]string, bool, *sourcePrimitiveFailure) {
	primitives.t.Fatal("unexpected source directory read")
	return nil, false, nil
}

func (primitives *scriptedSourceConstructionPrimitives) statDescriptor(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) (fileSnapshot, *sourcePrimitiveFailure) {
	if ctx == nil {
		primitives.t.Fatal("descriptor stat received nil context")
	}
	name := primitives.descriptorName(owner)
	observation := name
	if name == "config" {
		primitives.configObservationCount++
		if primitives.configObservationCount == 1 {
			observation = "config-before"
		} else {
			observation = "config-after"
		}
	}
	if name == "packed-refs" {
		primitives.packedRefsObservationCount++
		if primitives.packedRefsObservationCount == 1 {
			observation = "packed-refs-before"
		} else {
			observation = "packed-refs-after"
		}
	}
	primitives.observation[owner] = observation
	if failure := primitives.record("stat:" + observation); failure != nil {
		return fileSnapshot{}, failure
	}
	snapshot := primitives.snapshot(name)
	if observation == "config-after" {
		switch primitives.configPostReadDrift {
		case "identity":
			snapshot.identity.Inode++
		case "security":
			snapshot.mtimeSec++
		case "mode":
			snapshot.identity.Mode |= 0o022
		case "acl", "mount":
		case "":
		default:
			primitives.t.Fatalf("unknown config drift %q", primitives.configPostReadDrift)
		}
	}
	if observation == "config-rebind" {
		switch primitives.configRebindDrift {
		case "identity":
			snapshot.identity.Inode++
		case "security":
			snapshot.mtimeSec++
		case "mode":
			snapshot.identity.Mode |= 0o022
		case "acl", "mount":
		case "":
		default:
			primitives.t.Fatalf("unknown config rebind drift %q", primitives.configRebindDrift)
		}
	}
	if observation == "packed-refs-after" {
		switch primitives.packedRefsPostReadDrift {
		case "identity":
			snapshot.identity.Inode++
		case "security":
			snapshot.mtimeSec++
		case "mode":
			snapshot.identity.Mode |= 0o022
		case "acl", "mount":
		case "":
		default:
			primitives.t.Fatalf("unknown packed-refs drift %q", primitives.packedRefsPostReadDrift)
		}
	}
	if observation == "packed-refs-rebind" {
		switch primitives.packedRefsRebindDrift {
		case "identity":
			snapshot.identity.Inode++
		case "security":
			snapshot.mtimeSec++
		case "mode":
			snapshot.identity.Mode |= 0o022
		case "acl", "mount":
		case "":
		default:
			primitives.t.Fatalf("unknown packed-refs rebind drift %q", primitives.packedRefsRebindDrift)
		}
	}
	return snapshot, nil
}

func (primitives *scriptedSourceConstructionPrimitives) statFilesystem(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) (mountSnapshot, *sourcePrimitiveFailure) {
	if ctx == nil {
		primitives.t.Fatal("filesystem stat received nil context")
	}
	observation := primitives.observationName(owner)
	primitives.lastFilesystem = observation
	if failure := primitives.record("statfs:" + observation); failure != nil {
		return mountSnapshot{}, failure
	}
	mount := sourceConstructionMount()
	canonical := sourceConstructionObservationCanonicalName(observation)
	if filesystem, overridden := primitives.filesystemOverrides[canonical]; overridden {
		mount.filesystem = filesystem
	}
	if primitives.reobservationDrift(observation) == "mount" {
		mount.flags++
	}
	primitives.lastMount = mount
	return mount, nil
}

func (primitives *scriptedSourceConstructionPrimitives) validateFilesystem(
	ctx context.Context,
	mount mountSnapshot,
) *sourcePrimitiveFailure {
	if ctx == nil || mount != primitives.lastMount || primitives.lastFilesystem == "" {
		primitives.t.Fatalf("filesystem validation inputs = %+v / %+v", ctx, mount)
	}
	if failure := primitives.record("validate-fs:" + primitives.lastFilesystem); failure != nil {
		return failure
	}
	if primitives.reobservationDrift(primitives.lastFilesystem) == "mount" {
		return newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	return nil
}

func (primitives *scriptedSourceConstructionPrimitives) acquireRawACL(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) ([]byte, *sourcePrimitiveFailure) {
	if ctx == nil {
		primitives.t.Fatal("ACL acquisition received nil context")
	}
	observation := primitives.observationName(owner)
	if failure := primitives.record("acquire-acl:" + observation); failure != nil {
		return nil, failure
	}
	return []byte(observation), nil
}

func (primitives *scriptedSourceConstructionPrimitives) parseRawACL(
	ctx context.Context,
	raw []byte,
) (parsedSourceACL, *sourcePrimitiveFailure) {
	if ctx == nil || len(raw) == 0 {
		primitives.t.Fatalf("ACL parse inputs = %+v / %q", ctx, raw)
	}
	observation := string(raw)
	primitives.lastACL = observation
	if failure := primitives.record("parse-acl:" + observation); failure != nil {
		return parsedSourceACL{}, failure
	}
	if primitives.invalidACLEvent == "parse-acl:"+observation {
		return parsedSourceACL{}, nil
	}
	canonical := observation
	if strings.HasPrefix(observation, "config-") {
		canonical = "config"
	} else if strings.HasPrefix(observation, "packed-refs-") {
		canonical = "packed-refs"
	}
	disposition := sourceACLAdmitted
	if primitives.reobservationDrift(observation) == "acl" {
		canonical = observation + "-mutation"
		disposition = sourceACLMutationPermitting
	}
	return parsedSourceACL{
		digest:      Digest(sha256.Sum256([]byte(canonical))),
		disposition: disposition,
	}, nil
}

func (primitives *scriptedSourceConstructionPrimitives) validateACL(
	ctx context.Context,
	acl parsedSourceACL,
) *sourcePrimitiveFailure {
	invalidExpected := primitives.invalidACLEvent == "parse-acl:"+primitives.lastACL
	if ctx == nil || (!acl.valid() && !invalidExpected) || primitives.lastACL == "" {
		primitives.t.Fatalf("ACL validation inputs = %+v / %+v", ctx, acl)
	}
	if failure := primitives.record("validate-acl:" + primitives.lastACL); failure != nil {
		return failure
	}
	if acl.disposition == sourceACLMutationPermitting {
		return newSourcePrimitiveFailure(OperationValidate, CausePermission)
	}
	return nil
}

func (primitives *scriptedSourceConstructionPrimitives) reobservationDrift(
	observation string,
) string {
	switch observation {
	case "config-after":
		return primitives.configPostReadDrift
	case "config-rebind":
		return primitives.configRebindDrift
	case "packed-refs-after":
		return primitives.packedRefsPostReadDrift
	case "packed-refs-rebind":
		return primitives.packedRefsRebindDrift
	default:
		return ""
	}
}

func (primitives *scriptedSourceConstructionPrimitives) readExactForParse(
	ctx context.Context,
	owner *ownedSourceDescriptor,
	content []byte,
) *sourcePrimitiveFailure {
	name := primitives.descriptorName(owner)
	want := primitives.config
	if name == "packed-refs" {
		want = primitives.packedRefs
	} else if name != "config" {
		primitives.t.Fatalf("unexpected source read descriptor %q", name)
	}
	if ctx == nil || len(content) != len(want) {
		primitives.t.Fatalf("%s read inputs = %+v / %d", name, ctx, len(content))
	}
	primitives.lastRead = name
	if failure := primitives.record("read:" + name); failure != nil {
		return failure
	}
	copy(content, want)
	return nil
}

func (primitives *scriptedSourceConstructionPrimitives) hashBytes(
	ctx context.Context,
	content []byte,
) (Digest, *sourcePrimitiveFailure) {
	want := primitives.config
	if primitives.lastRead == "packed-refs" {
		want = primitives.packedRefs
	}
	if ctx == nil || primitives.lastRead == "" || !bytes.Equal(content, want) {
		primitives.t.Fatalf("%s hash inputs = %+v / %q", primitives.lastRead, ctx, content)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return Digest{}, failure
	}
	if failure := primitives.record("hash:" + primitives.lastRead); failure != nil {
		return Digest{}, failure
	}
	digest := Digest(sha256.Sum256(content))
	if failure := sourceContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return Digest{}, failure
	}
	return digest, nil
}

func (primitives *scriptedSourceConstructionPrimitives) compareRootAndDescriptor(
	ctx context.Context,
	root *ownedSourceRoot,
	descriptor *ownedSourceDescriptor,
) *sourcePrimitiveFailure {
	if ctx == nil {
		primitives.t.Fatal("root comparison received nil context")
	}
	rootName := primitives.rootName(root)
	if descriptorName := primitives.descriptorName(descriptor); descriptorName != rootName {
		primitives.t.Fatalf("root comparison = %s / %s", rootName, descriptorName)
	}
	return primitives.record("compare:" + rootName)
}

func (primitives *scriptedSourceConstructionPrimitives) closeRoot(owner *ownedSourceRoot) bool {
	name := primitives.rootName(owner)
	event := "close-root:" + name
	primitives.events = append(primitives.events, event)
	failed := primitives.closeFailures[event]
	owner.root = nil
	owner.state = sourceHandleClosed
	owner.closeFailure = failed
	return failed
}

func (primitives *scriptedSourceConstructionPrimitives) closeDescriptor(
	owner *ownedSourceDescriptor,
) bool {
	name := primitives.descriptorName(owner)
	event := "close-descriptor:" + name
	primitives.events = append(primitives.events, event)
	failed := primitives.closeFailures[event]
	owner.file = nil
	owner.state = sourceHandleClosed
	owner.closeFailure = failed
	return failed
}

func (primitives *scriptedSourceConstructionPrimitives) openRootResult(
	ownerName string,
	event string,
) (*ownedSourceRoot, *sourcePrimitiveFailure) {
	failure := primitives.record(event)
	if primitives.nilOwnerEvent == event {
		return nil, failure
	}
	owner := primitives.roots[ownerName]
	if failure != nil && primitives.ownerFailureEvent != event {
		return nil, failure
	}
	primitives.acquiredRoots[ownerName] = true
	return owner, failure
}

func (primitives *scriptedSourceConstructionPrimitives) openDescriptorResult(
	ownerName string,
	event string,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	failure := primitives.record(event)
	if primitives.nilOwnerEvent == event {
		return nil, failure
	}
	owner := primitives.descriptors[ownerName]
	if failure != nil && primitives.ownerFailureEvent != event {
		return nil, failure
	}
	primitives.acquiredDescriptors[ownerName] = true
	return owner, failure
}

func (primitives *scriptedSourceConstructionPrimitives) record(
	event string,
) *sourcePrimitiveFailure {
	primitives.events = append(primitives.events, event)
	if event == primitives.cancelEvent && primitives.cancel != nil {
		primitives.cancel()
		primitives.cancel = nil
	}
	if event == primitives.failureEvent || event == primitives.ownerFailureEvent {
		if primitives.failure == nil {
			primitives.t.Fatalf("failure event %q has no failure", event)
		}
		return primitives.failure
	}
	return nil
}

func (primitives *scriptedSourceConstructionPrimitives) descriptorName(
	owner *ownedSourceDescriptor,
) string {
	primitives.t.Helper()
	name, ok := primitives.descriptorNames[owner]
	if !ok {
		primitives.t.Fatalf("unknown source descriptor owner %p", owner)
	}
	return name
}

func (primitives *scriptedSourceConstructionPrimitives) rootName(
	owner *ownedSourceRoot,
) string {
	primitives.t.Helper()
	name, ok := primitives.rootNames[owner]
	if !ok {
		primitives.t.Fatalf("unknown source root owner %p", owner)
	}
	return name
}

func (primitives *scriptedSourceConstructionPrimitives) observationName(
	owner *ownedSourceDescriptor,
) string {
	primitives.t.Helper()
	observation := primitives.observation[owner]
	if observation == "" {
		primitives.t.Fatalf("source descriptor %s has no active observation", primitives.descriptorName(owner))
	}
	return observation
}

func (primitives *scriptedSourceConstructionPrimitives) snapshot(name string) fileSnapshot {
	canonical := name
	switch name {
	case "config-rebind":
		canonical = "config"
	case "packed-refs-rebind":
		canonical = "packed-refs"
	}
	inodes := map[string]uint64{
		"physical-root": 1,
		"parent":        6,
		"repository":    2,
		"git":           3,
		"config":        4,
		"objects":       5,
		"packed-refs":   7,
	}
	mode := uint32(platformModeDirectory | 0o700)
	linkCount := uint64(2)
	size := int64(0)
	switch canonical {
	case "config":
		mode = platformModeRegular | 0o600
		linkCount = 1
		size = int64(len(primitives.config))
	case "packed-refs":
		mode = platformModeRegular | 0o600
		linkCount = 1
		size = int64(len(primitives.packedRefs))
	}
	if overridden, found := primitives.linkCountOverrides[canonical]; found {
		linkCount = overridden
	}
	if overridden, found := primitives.sizeOverrides[canonical]; found {
		size = overridden
	}
	device := uint64(7)
	if overridden, found := primitives.deviceOverrides[canonical]; found {
		device = overridden
	}
	return fileSnapshot{
		identity: FileIdentity{
			Device: device,
			Inode:  inodes[canonical],
			UID:    uint32(os.Geteuid()),
			Mode:   mode,
		},
		linkCount: linkCount,
		size:      size,
		mtimeSec:  11,
		ctimeSec:  12,
		birthSec:  13,
	}
}

func testSourceRepositoryLocator() sourceRepositoryLocator {
	return sourceRepositoryLocator{path: "/repository", seal: validSourceRepositoryLocator}
}

func sourceConstructionMount() mountSnapshot {
	return mountSnapshot{filesystem: [2]int32{8, 9}, flags: 10}
}

func sourceConstructionObservationCanonicalName(observation string) string {
	if strings.HasPrefix(observation, "config-") {
		return "config"
	}
	if strings.HasPrefix(observation, "packed-refs-") {
		return "packed-refs"
	}
	return observation
}

func sourceConstructionAcquisitionEvents() []string {
	events := []string{"open-root:repository", "open-descriptor:physical-root"}
	events = append(events, sourceConstructionObservationEvents("physical-root")...)
	events = append(events, "open-descriptor:repository")
	events = append(events, sourceConstructionObservationEvents("repository")...)
	events = append(events, "compare:repository", "close-descriptor:physical-root", "probe:git")
	events = append(events, "open-root:git", "open-descriptor:git")
	events = append(events, sourceConstructionObservationEvents("git")...)
	events = append(events, "compare:git", "probe:config", "open-descriptor:config")
	events = append(events, sourceConstructionObservationEvents("config-before")...)
	events = append(events, "read:config", "hash:config")
	events = append(events, sourceConstructionReobservationEvents("config-after")...)
	events = append(events, "open-descriptor:config-rebind")
	events = append(events, sourceConstructionReobservationEvents("config-rebind")...)
	events = append(events, "close-descriptor:config-rebind", "probe:objects")
	events = append(events, "open-root:objects", "open-descriptor:objects")
	events = append(events, sourceConstructionObservationEvents("objects")...)
	events = append(events, "compare:objects")
	return events
}

func sourceConstructionPackedRefsPresentEvents() []string {
	events := []string{"probe:packed-refs:initial", "open-descriptor:packed-refs"}
	events = append(events, sourceConstructionObservationEvents("packed-refs-before")...)
	events = append(events, "read:packed-refs", "hash:packed-refs")
	events = append(events, sourceConstructionReobservationEvents("packed-refs-after")...)
	events = append(events, "open-descriptor:packed-refs-rebind")
	events = append(events, sourceConstructionReobservationEvents("packed-refs-rebind")...)
	events = append(events, "close-descriptor:packed-refs-rebind")
	return events
}

func sourceConstructionObservationEvents(name string) []string {
	return []string{
		"stat:" + name,
		"statfs:" + name,
		"validate-fs:" + name,
		"acquire-acl:" + name,
		"parse-acl:" + name,
		"validate-acl:" + name,
	}
}

func sourceConstructionReobservationEvents(name string) []string {
	return []string{
		"stat:" + name,
		"statfs:" + name,
		"acquire-acl:" + name,
		"parse-acl:" + name,
		"validate-fs:" + name,
		"validate-acl:" + name,
	}
}

func sourceConstructionOwnerCloseEvents() []string {
	return []string{
		"close-root:objects",
		"close-descriptor:objects",
		"close-descriptor:config",
		"close-root:git",
		"close-descriptor:git",
		"close-root:repository",
		"close-descriptor:repository",
	}
}

func sourceConstructionEventOperation(t *testing.T, event string) Operation {
	t.Helper()
	switch {
	case strings.HasPrefix(event, "open-"):
		return OperationOpen
	case strings.HasPrefix(event, "probe:"), strings.HasPrefix(event, "stat:"),
		strings.HasPrefix(event, "statfs:"), strings.HasPrefix(event, "acquire-acl:"):
		return OperationProbe
	case strings.HasPrefix(event, "parse-acl:"), strings.HasPrefix(event, "read:"):
		return OperationParse
	case strings.HasPrefix(event, "validate-"):
		return OperationValidate
	case strings.HasPrefix(event, "hash:"):
		return OperationHash
	case strings.HasPrefix(event, "compare:"):
		return OperationCompare
	default:
		t.Fatalf("source construction event %q has no primitive operation", event)
		return ""
	}
}

func assertOnlySourceCloseTailAfter(t *testing.T, events []string, failureEvent string) {
	t.Helper()
	failureIndex := -1
	for index, event := range events {
		if event == failureEvent {
			failureIndex = index
			break
		}
	}
	if failureIndex < 0 {
		t.Fatalf("failure event %q absent from trace %q", failureEvent, events)
	}
	for _, event := range events[failureIndex+1:] {
		if !strings.HasPrefix(event, "close-") {
			t.Fatalf("failure at %q began later independent primitive %q; trace %q", failureEvent, event, events)
		}
	}
}

func assertNoPackedRefsEvents(t *testing.T, events []string) {
	t.Helper()
	for _, event := range events {
		if strings.Contains(event, "packed-refs") {
			t.Fatalf("construction touched unresolved packed-refs: %q", events)
		}
	}
}

func sourceConstructionEventPresent(events []string, want string) bool {
	return slices.Contains(events, want)
}

func assertSourceConstructionEventSequence(t *testing.T, events, want []string) {
	t.Helper()
	if len(want) == 0 {
		t.Fatal("source construction event sequence must not be empty")
	}
	start := slices.Index(events, want[0])
	if start < 0 || len(events)-start < len(want) ||
		!reflect.DeepEqual(events[start:start+len(want)], want) {
		t.Fatalf("source construction trace %q does not contain contiguous sequence %q", events, want)
	}
}

func assertSourceConstructionAcquiredOwnersClosed(
	t *testing.T,
	primitives *scriptedSourceConstructionPrimitives,
) {
	t.Helper()
	for name := range primitives.acquiredRoots {
		owner := primitives.roots[name]
		if owner == nil || owner.state != sourceHandleClosed || owner.root != nil {
			t.Fatalf("acquired source root %q was not closed: %+v", name, owner)
		}
		event := "close-root:" + name
		if count := countSourceConstructionEvent(primitives.events, event); count != 1 {
			t.Fatalf("acquired source root %q close count = %d; trace %q", name, count, primitives.events)
		}
	}
	for name := range primitives.acquiredDescriptors {
		owner := primitives.descriptors[name]
		if owner == nil || owner.state != sourceHandleClosed || owner.file != nil {
			t.Fatalf("acquired source descriptor %q was not closed: %+v", name, owner)
		}
		event := "close-descriptor:" + name
		if count := countSourceConstructionEvent(primitives.events, event); count != 1 {
			t.Fatalf("acquired source descriptor %q close count = %d; trace %q", name, count, primitives.events)
		}
	}

	gotLongLived := make([]string, 0, len(sourceConstructionOwnerCloseEvents()))
	for _, event := range primitives.events {
		if slices.Contains(sourceConstructionOwnerCloseEvents(), event) {
			gotLongLived = append(gotLongLived, event)
		}
	}
	wantLongLived := make([]string, 0, len(sourceConstructionOwnerCloseEvents()))
	for _, event := range sourceConstructionOwnerCloseEvents() {
		name := strings.TrimPrefix(strings.TrimPrefix(event, "close-root:"), "close-descriptor:")
		acquired := primitives.acquiredDescriptors[name]
		if strings.HasPrefix(event, "close-root:") {
			acquired = primitives.acquiredRoots[name]
		}
		if acquired {
			wantLongLived = append(wantLongLived, event)
		}
	}
	if !reflect.DeepEqual(gotLongLived, wantLongLived) {
		t.Fatalf(
			"partial source close order = %q, want acquired prefix order %q; trace %q",
			gotLongLived,
			wantLongLived,
			primitives.events,
		)
	}
}

func countSourceConstructionEvent(events []string, want string) int {
	count := 0
	for _, event := range events {
		if event == want {
			count++
		}
	}
	return count
}

func runSourceConstructionPackedRefsStage(
	ctx context.Context,
	owner *sourceConstructionOwner,
	primitives sourcePrimitives,
) sourceUseOutcome {
	builder := &sourceConstructionBuilder{
		ctx:        ctx,
		primitives: primitives,
		owner:      owner,
	}
	builder.retainPackedRefs()
	if !builder.outcome.proved() {
		builder.owner.closeIntoWith(primitives, &builder.outcome)
	}
	return builder.outcome
}

func loadTypedSourceConstructionPackage(
	t *testing.T,
	overlay map[string][]byte,
) *packages.Package {
	t.Helper()
	return loadTypedSourceConstructionPackageWithEnvironment(t, overlay, nil)
}

func loadTypedDarwinSourceConstructionPackage(
	t *testing.T,
	overlay map[string][]byte,
) *packages.Package {
	t.Helper()
	environment := make([]string, 0, len(os.Environ())+3)
	for _, variable := range os.Environ() {
		if strings.HasPrefix(variable, "GOOS=") ||
			strings.HasPrefix(variable, "GOARCH=") ||
			strings.HasPrefix(variable, "CGO_ENABLED=") {
			continue
		}
		environment = append(environment, variable)
	}
	environment = append(environment, "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0")
	return loadTypedSourceConstructionPackageWithEnvironment(t, overlay, environment)
}

func loadTypedSourceConstructionPackageWithEnvironment(
	t *testing.T,
	overlay map[string][]byte,
	environment []string,
) *packages.Package {
	t.Helper()
	config := &packages.Config{
		Mode:    packages.LoadSyntax,
		Dir:     ".",
		Tests:   false,
		Overlay: overlay,
	}
	if environment != nil {
		config.Env = environment
	}
	loaded, err := packages.Load(config, ".")
	if err != nil {
		t.Fatalf("load typed buildauthority package: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("typed buildauthority packages = %d, want 1", len(loaded))
	}
	typedPackage := loaded[0]
	if len(typedPackage.Errors) != 0 || typedPackage.IllTyped ||
		typedPackage.Types == nil || typedPackage.TypesInfo == nil || typedPackage.Fset == nil {
		errors := make([]string, 0, len(typedPackage.Errors))
		for _, packageError := range typedPackage.Errors {
			errors = append(errors, packageError.Error())
		}
		t.Fatalf(
			"typed buildauthority package is incomplete: illTyped=%v errors=%q",
			typedPackage.IllTyped,
			errors,
		)
	}
	return typedPackage
}

func sourceConstructionAllowedSensitiveParameters() map[string]map[int]map[string]bool {
	return map[string]map[int]map[string]bool{
		"(*sourceConstructionBuilder).retainRepositoryDescriptor": {
			1: {"*retainedSourceRepository": true},
		},
		"(*sourceConstructionBuilder).retainSourceDirectory": {
			0: {"*retainedSourceRoot": true},
			1: {"*ownedSourceRoot": true},
			2: {"*ownedSourceDescriptor": true},
		},
		"(*sourceConstructionBuilder).observeDescriptor": {
			0: {"*ownedSourceDescriptor": true},
		},
		"(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy": {
			0: {"*ownedSourceDescriptor": true},
		},
		"(*sourceAdministrativeInventory).valid": {
			1: {"sourcePackedRefsSlot": true},
		},
		"(sourceAdministrativeFixedRows).valid": {
			0: {"sourcePackedRefsSlot": true},
		},
		"(sourceAdministrativeFixedRows).validPackedRefs": {
			0: {"sourcePackedRefsSlot": true},
		},
		"(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy": {
			0: {"*ownedSourceDescriptor": true},
		},
		"(*sourceConstructionBuilder).reobserveSourceDescriptorBeforePolicy": {
			0: {"*ownedSourceDescriptor": true},
		},
		"(*sourceConstructionBuilder).compareSourceDescriptorWithoutPolicy": {
			0: {"*ownedSourceDescriptor": true},
		},
		"(*sourceConstructionBuilder).captureSourceAdministrativeDirectory": {
			1: {"*ownedSourceDescriptor": true},
		},
		"(*sourceConstructionBuilder).captureSourceAdministrativeEntry": {
			1: {"*ownedSourceDescriptor": true},
		},
		"(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry": {
			1: {"*ownedSourceDescriptor": true},
		},
		"validateSourceObservationRequest": {
			1: {"*ownedSourceDescriptor": true},
		},
		"validateInitialSourceOwner": {
			1: {"*sourceConstructionOwner": true},
		},
		"validateSourceAdministrativeInventoryOwner": {
			1: {"*sourceConstructionOwner": true},
		},
		"(*sourceConstructionBuilder).acceptRootAcquisition": {
			0: {"*ownedSourceRoot": true},
		},
		"(*sourceConstructionBuilder).acceptDescriptorAcquisition": {
			0: {"*ownedSourceDescriptor": true},
		},
		"(*sourceConstructionBuilder).closeTransientDescriptor": {
			0: {"*ownedSourceDescriptor": true},
		},
		"(*sourceConstructionBuilder).closeTransientDescriptors": {
			0: {"[]*ownedSourceDescriptor": true},
		},
		"(directSourceHandleCloser).closeRoot": {
			0: {"*ownedSourceRoot": true},
		},
		"(directSourceHandleCloser).closeDescriptor": {
			0: {"*ownedSourceDescriptor": true},
		},
		"(*sourceCloseTracker).closePackedRefs": {
			0: {"*retainedSourcePackedRefs": true},
		},
		"(*sourceCloseTracker).closeObjects": {
			0: {"*retainedSourceObjects": true},
		},
		"(*sourceCloseTracker).closeConfig": {
			0: {"*retainedSourceConfig": true},
		},
		"(*sourceCloseTracker).closeGit": {
			0: {"*retainedSourceGit": true},
		},
		"(*sourceCloseTracker).closeRepository": {
			0: {"*retainedSourceRepository": true},
		},
		"(*sourceCloseTracker).closeRootBundle": {
			0: {"*retainedSourceRoot": true},
		},
		"(*sourceCloseTracker).closeRoot": {
			0: {"**ownedSourceRoot": true},
		},
		"(*sourceCloseTracker).closeDescriptor": {
			0: {"**ownedSourceDescriptor": true},
		},
	}
}

func sourceConstructionAllowedAcquisitionSites() map[string]int {
	return map[string]int{
		"openRepositoryRoot|(*sourceConstructionBuilder).retainRepository|:=|root,failure":                                   1,
		"openPhysicalRootDescriptor|(*sourceConstructionBuilder).retainRepositoryDescriptor|:=|physicalRoot,openFailure":     1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).retainRepositoryDescriptor|:=|next,nextFailure":                   1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).retainConfig|:=|descriptor,descriptorFailure":                     1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).rebindConfig|:=|comparison,openFailure":                           1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).retainPresentPackedRefs|:=|descriptor,descriptorFailure":          1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).rebindPackedRefs|:=|comparison,openFailure":                       1,
		"openChildRoot|(*sourceConstructionBuilder).retainSourceDirectory|:=|root,rootFailure":                               1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).retainSourceDirectory|:=|descriptor,descriptorFailure":            1,
		"openRootDirectoryDescriptor|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|:=|scan,openFailure":   1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|:=|descriptor,openFailure": 1,
	}
}

func sourceConstructionAllowedOwnerFactorySites() map[string]int {
	return map[string]int{
		"newSourceConstructionOwner|retainSourceConstructionWith|composite:owner": 1,
		"retainSourceConstructionWith|retainSourceConstruction|return":            1,
	}
}

func sourceConstructionEscapeViolations(typedPackage *packages.Package) []string {
	sensitiveNames := map[string]bool{
		"ownedSourceRoot":           true,
		"ownedSourceDescriptor":     true,
		"retainedSourceRoot":        true,
		"retainedSourceRepository":  true,
		"retainedSourceGit":         true,
		"retainedSourceObjects":     true,
		"retainedSourceConfig":      true,
		"retainedSourcePackedRefs":  true,
		"sourcePackedRefsSlot":      true,
		"sourceConstructionOwner":   true,
		"sourceConstructionBuilder": true,
	}
	allowedOwnerResults := map[string]bool{
		"newSourceConstructionOwner":   true,
		"retainSourceConstruction":     true,
		"retainSourceConstructionWith": true,
	}
	allowedSensitiveParameters := sourceConstructionAllowedSensitiveParameters()
	allowedAcquisitionSites := sourceConstructionAllowedAcquisitionSites()
	observedAcquisitionSites := make(map[string]int)
	allowedOwnerFactorySites := sourceConstructionAllowedOwnerFactorySites()
	observedOwnerFactorySites := make(map[string]int)
	constructionFiles := make([]*ast.File, 0, 3)
	allowedCalls := make(map[types.Object]bool)
	acquisitionCalls := make(map[types.Object]bool)
	ownerFactoryCalls := make(map[types.Object]bool)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := typedPackage.Fset.Position(file.Package).Filename
		if !strings.HasPrefix(filepath.Base(filename), "source_construction") {
			continue
		}
		constructionFiles = append(constructionFiles, file)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Recv != nil && !sourceConstructionReceiverDeclaredInModule(
				typedPackage,
				function,
			) {
				violations = append(violations,
					typedPackage.Fset.Position(function.Name.Pos()).String()+
						" declares construction method on non-construction receiver",
				)
				continue
			}
			if object := typedPackage.TypesInfo.Defs[function.Name]; object != nil {
				allowedCalls[object] = true
				if signature, ok := object.Type().(*types.Signature); ok {
					functionRole := sourceConstructionFunctionRole(
						object,
						typedPackage.Types.Path(),
					)
					if sourceConstructionContainsNamedType(
						signature.Results(),
						typedPackage.Types.Path(),
						"sourceConstructionOwner",
						map[types.Type]bool{},
					) {
						ownerFactoryCalls[object] = true
					}
					for index := range signature.Params().Len() {
						parameter := signature.Params().At(index)
						if sourceConstructionSensitiveType(
							parameter.Type(),
							typedPackage.Types.Path(),
							sensitiveNames,
							map[types.Type]bool{},
						) == "" {
							continue
						}
						role := sourceConstructionParameterRole(
							parameter.Type(),
							typedPackage.Types.Path(),
						)
						if allowedSensitiveParameters[functionRole][index][role] {
							continue
						}
						violations = append(violations,
							typedPackage.Fset.Position(function.Name.Pos()).String()+
								" "+functionRole+" declares unapproved sensitive parameter "+role,
						)
					}
				}
			}
		}
	}
	violations = append(
		violations,
		sourceConstructionAllowPrimitiveMethods(
			typedPackage,
			allowedCalls,
			acquisitionCalls,
			sensitiveNames,
		)...,
	)
	violations = append(
		violations,
		sourceConstructionAllowHandleCloserMethods(typedPackage, allowedCalls)...,
	)
	sourceConstructionAllowMethods(
		typedPackage,
		allowedCalls,
		"ownedSourceRoot",
		map[string]bool{"validOpen": true, "closeDirect": true},
	)
	sourceConstructionAllowMethods(
		typedPackage,
		allowedCalls,
		"ownedSourceDescriptor",
		map[string]bool{"validOpen": true, "closeDirect": true},
	)

	localObjects := sourceConstructionLocalObjects(typedPackage, constructionFiles)
	for _, file := range constructionFiles {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			object, _ := typedPackage.TypesInfo.Defs[function.Name].(*types.Func)
			if object == nil {
				violations = append(violations,
					typedPackage.Fset.Position(function.Name.Pos()).String()+" has no resolved function object",
				)
				continue
			}
			signature, _ := object.Type().(*types.Signature)
			functionRole := sourceConstructionFunctionRole(object, typedPackage.Types.Path())
			if signature != nil {
				for result := range signature.Results().Variables() {
					resultType := result.Type()
					escaped := sourceConstructionSensitiveType(
						resultType,
						typedPackage.Types.Path(),
						sensitiveNames,
						map[types.Type]bool{},
					)
					if escaped == "" {
						continue
					}
					if allowedOwnerResults[functionRole] && sourceConstructionDirectOwnerType(
						resultType,
						typedPackage.Types.Path(),
					) {
						continue
					}
					violations = append(violations,
						typedPackage.Fset.Position(function.Name.Pos()).String()+
							" "+functionRole+" returns "+escaped+" through "+
							sourceConstructionTypeName(resultType),
					)
				}
			}
		}

		ast.Inspect(file, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.CallExpr:
				passed := sourceConstructionSensitiveCallValue(
					typedPackage.TypesInfo,
					current,
					typedPackage.Types.Path(),
					sensitiveNames,
				)
				returned := sourceConstructionSensitiveType(
					typedPackage.TypesInfo.TypeOf(current),
					typedPackage.Types.Path(),
					sensitiveNames,
					map[types.Type]bool{},
				)
				called := sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun)
				if acquisitionCalls[called] {
					site := sourceConstructionAcquisitionSite(
						typedPackage,
						file,
						current,
						called,
					)
					observedAcquisitionSites[site]++
					if allowedAcquisitionSites[site] == 0 {
						violations = append(violations,
							typedPackage.Fset.Position(current.Pos()).String()+
								" acquires "+returned+" outside audited site "+site,
						)
					}
				}
				if ownerFactoryCalls[called] {
					site := sourceConstructionOwnerFactorySite(
						typedPackage,
						file,
						current,
						called,
					)
					observedOwnerFactorySites[site]++
					if allowedOwnerFactorySites[site] == 0 {
						violations = append(violations,
							typedPackage.Fset.Position(current.Pos()).String()+
								" creates sourceConstructionOwner outside audited site "+site,
						)
					}
				}
				if consumed := sourceConstructionNestedSensitiveResult(
					typedPackage.TypesInfo,
					current,
					typedPackage.Types.Path(),
					sensitiveNames,
				); consumed != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" consumes "+consumed+" returned by nested call",
					)
					return true
				}
				if passed == "" && returned == "" {
					return true
				}
				if builtin, ok := called.(*types.Builtin); ok &&
					sourceConstructionSensitiveBuiltinAllowed(
						typedPackage,
						builtin,
						current,
						sensitiveNames,
						localObjects,
					) {
					return true
				}
				if allowedCalls[called] {
					return true
				}
				calledName := "dynamic or conversion"
				if called != nil {
					calledName = called.Name()
				}
				action := " passes " + passed + " to non-construction call "
				if passed == "" {
					action = " receives " + returned + " from non-construction call "
				}
				violations = append(violations,
					typedPackage.Fset.Position(current.Pos()).String()+
						action+calledName,
				)
			case *ast.ExprStmt:
				if discarded := sourceConstructionSensitiveType(
					typedPackage.TypesInfo.TypeOf(current.X),
					typedPackage.Types.Path(),
					sensitiveNames,
					map[types.Type]bool{},
				); discarded != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" discards "+discarded+" result",
					)
				}
			case *ast.AssignStmt:
				violations = append(
					violations,
					sourceConstructionAssignmentEscapeViolations(
						typedPackage,
						current.Lhs,
						current.Rhs,
						sensitiveNames,
						localObjects,
					)...,
				)
			case *ast.IncDecStmt:
				if violation := sourceConstructionMapIndexKeyViolation(
					typedPackage,
					current.X,
					sensitiveNames,
				); violation != "" {
					violations = append(violations, violation)
				}
			case *ast.RangeStmt:
				if current.Tok != token.ASSIGN {
					break
				}
				for _, target := range []ast.Expr{current.Key, current.Value} {
					if target == nil {
						continue
					}
					if violation := sourceConstructionMapIndexKeyViolation(
						typedPackage,
						target,
						sensitiveNames,
					); violation != "" {
						violations = append(violations, violation)
					}
				}
			case *ast.ValueSpec:
				targets := make([]ast.Expr, 0, len(current.Names))
				for _, name := range current.Names {
					targets = append(targets, name)
				}
				violations = append(
					violations,
					sourceConstructionAssignmentEscapeViolations(
						typedPackage,
						targets,
						current.Values,
						sensitiveNames,
						localObjects,
					)...,
				)
			case *ast.SendStmt:
				if escaped := sourceConstructionSensitiveExpression(
					typedPackage.TypesInfo,
					current.Value,
					typedPackage.Types.Path(),
					sensitiveNames,
				); escaped != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" sends "+escaped+" through a channel",
					)
				}
			case *ast.GoStmt:
				violations = append(violations,
					typedPackage.Fset.Position(current.Pos()).String()+
						" starts asynchronous work inside the source-construction owner boundary",
				)
			case *ast.DeferStmt:
				if discarded := sourceConstructionSensitiveType(
					typedPackage.TypesInfo.TypeOf(current.Call),
					typedPackage.Types.Path(),
					sensitiveNames,
					map[types.Type]bool{},
				); discarded != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" defers and discards "+discarded+" result",
					)
				}
				if escaped := sourceConstructionSensitiveCallValue(
					typedPackage.TypesInfo,
					current.Call,
					typedPackage.Types.Path(),
					sensitiveNames,
				); escaped != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" defers use of "+escaped+" inside the source-construction owner boundary",
					)
				}
			case *ast.FuncLit:
				if escaped := sourceConstructionSensitiveClosureValue(
					typedPackage.TypesInfo,
					current,
					typedPackage.Types.Path(),
					sensitiveNames,
				); escaped != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" captures "+escaped+" in a function literal",
					)
				}
			case *ast.ReturnStmt:
				function := sourceConstructionEnclosingFunction(file, current.Pos())
				var functionRole string
				if function != nil {
					functionRole = sourceConstructionFunctionRole(
						typedPackage.TypesInfo.Defs[function.Name],
						typedPackage.Types.Path(),
					)
				}
				for _, result := range current.Results {
					escaped := sourceConstructionSensitiveExpression(
						typedPackage.TypesInfo,
						result,
						typedPackage.Types.Path(),
						sensitiveNames,
					)
					if escaped == "" || function == nil ||
						(allowedOwnerResults[functionRole] && sourceConstructionPermittedOwnerReturn(
							typedPackage.TypesInfo.TypeOf(result),
							typedPackage.Types.Path(),
							sensitiveNames,
						)) {
						continue
					}
					violations = append(violations,
						typedPackage.Fset.Position(result.Pos()).String()+
							" returns "+escaped+" from "+function.Name.Name,
					)
				}
			case *ast.Ident:
				object, _ := typedPackage.TypesInfo.Uses[current].(*types.Var)
				if object != nil && object.Parent() == typedPackage.Types.Scope() {
					if escaped := sourceConstructionSensitiveType(
						object.Type(),
						typedPackage.Types.Path(),
						sensitiveNames,
						map[types.Type]bool{},
					); escaped != "" {
						violations = append(violations,
							typedPackage.Fset.Position(current.Pos()).String()+
								" references package capability storage "+current.Name+" containing "+escaped,
						)
					}
				}
			}
			return true
		})
	}
	for site, want := range allowedAcquisitionSites {
		if got := observedAcquisitionSites[site]; got != want {
			violations = append(violations,
				"audited source acquisition site "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range allowedOwnerFactorySites {
		if got := observedOwnerFactorySites[site]; got != want {
			violations = append(violations,
				"audited source owner factory site "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	if len(constructionFiles) != 3 {
		violations = append(violations, "typed source-construction module did not contain exactly three files")
	}
	return violations
}

func sourceConstructionDarwinContentReadViolations(
	files *token.FileSet,
	file *ast.File,
) []string {
	allowed := map[string]int{
		"ReadDir|readDirectoryBatch": 1,
		"ReadAt|readExactForParse":   1,
	}
	readCalls := map[string]bool{
		"Copy": true, "CopyBuffer": true, "CopyN": true,
		"Mmap": true, "MmapPtr": true,
		"NewReader": true, "NewReaderSize": true, "NewSectionReader": true,
		"Pread": true, "Preadv": true,
		"Read": true, "ReadAll": true, "ReadAt": true, "ReadAtLeast": true,
		"ReadDir": true, "ReadFile": true, "ReadFull": true,
		"Readdir": true, "Readdirnames": true, "Readv": true,
		"Readlink": true, "Readlinkat": true,
		"Sendfile": true, "Splice": true, "WriteTo": true,
	}
	observed := make(map[string]int)
	violations := make([]string, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || !readCalls[selector.Sel.Name] {
			return true
		}
		function := sourceConstructionEnclosingFunction(file, selector.Pos())
		functionName := "<outside-function>"
		if function != nil {
			functionName = function.Name.Name
		}
		site := selector.Sel.Name + "|" + functionName
		observed[site]++
		if allowed[site] == 0 {
			violations = append(violations,
				files.Position(selector.Pos()).String()+
					" content-read reference "+selector.Sel.Name+
					" outside audited site "+site,
			)
		}
		return true
	})
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited Darwin content-read site "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	return violations
}

func sourceConstructionDescriptorFileCapabilityViolations(
	typedPackage *packages.Package,
) []string {
	allowed := map[string]int{
		"source_primitives.go|(*ownedSourceDescriptor).validOpen|compare:!=":                                                  1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|compare:==":                                                1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|method:Close":                                              1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|assign-left:=":                                             1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|compare:==":                          1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|fd:unix.Fstatat[arg0:int]":                    1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|argument:runtime.KeepAlive":                   1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|fd:unix.Openat[arg0:int]":                             1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|argument:runtime.KeepAlive":                           2,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|compare:==":                                           1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|fd:requireDescriptorKind[arg0:int]":                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|assign-left:=":                      1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|compare:!=":                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|compare:==":                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|fd:requireDescriptorKind[arg0:int]": 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|argument:runtime.KeepAlive":         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|method:ReadDir":                              1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|argument:runtime.KeepAlive":                  1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|fd:unix.Fstat[arg0:int]":                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|argument:runtime.KeepAlive":                      1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statFilesystem|fd:unix.Fstatfs[arg0:int]":                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statFilesystem|argument:runtime.KeepAlive":                      1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).acquireRawACL|fd:darwinFgetattrlist[arg0:int]":                  1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).acquireRawACL|argument:runtime.KeepAlive":                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readExactForParse|method:ReadAt":                                1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readExactForParse|argument:runtime.KeepAlive":                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|method:Stat":                           1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|argument:runtime.KeepAlive":            1,
	}
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for descriptor file-capability audit"}
	}
	descriptorObject := typedPackage.Types.Scope().Lookup("ownedSourceDescriptor")
	if descriptorObject == nil {
		return []string{"ownedSourceDescriptor type is missing from descriptor file-capability audit"}
	}
	descriptor, _ := types.Unalias(descriptorObject.Type()).(*types.Named)
	if descriptor == nil {
		return []string{"ownedSourceDescriptor type is missing from descriptor file-capability audit"}
	}
	structure, _ := descriptor.Underlying().(*types.Struct)
	if structure == nil {
		return []string{"ownedSourceDescriptor is not a struct in descriptor file-capability audit"}
	}
	var fileField *types.Var
	for field := range structure.Fields() {
		if field.Name() == "file" {
			fileField = field
			break
		}
	}
	if fileField == nil || sourceConstructionParameterRole(
		fileField.Type(),
		typedPackage.Types.Path(),
	) != "*os.File" {
		return []string{"ownedSourceDescriptor.file is not the exact *os.File capability"}
	}

	observed, violations := sourceConstructionFieldCapabilitySites(
		typedPackage,
		fileField,
		allowed,
		"descriptor file",
	)
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited source descriptor file-capability site "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	violations = append(
		violations,
		sourceConstructionDescriptorFileOriginViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionOwnedFieldInitializationViolations(
			typedPackage,
			fileField,
			map[string]int{
				"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|call:os.NewFile": 1,
				"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|call:os.NewFile":                  1,
			},
			"descriptor file",
		)...,
	)
	violations = append(
		violations,
		sourceConstructionOwnedTypeConstructionViolations(
			typedPackage,
			descriptor,
			map[string]int{
				"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|composite":                                                                1,
				"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|composite":                                                                                 1,
				"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|composite":                                                               1,
				"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|factory:(sourcePrimitives).openPhysicalRootDescriptor":             1,
				"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|factory:(sourcePrimitives).openRelativeNoFollow":                   1,
				"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|factory:(sourcePrimitives).openRelativeNoFollow":                                 1,
				"source_construction_acquire.go|(*sourceConstructionBuilder).rebindConfig|factory:(sourcePrimitives).openRelativeNoFollow":                                 1,
				"source_construction_acquire.go|(*sourceConstructionBuilder).retainPresentPackedRefs|factory:(sourcePrimitives).openRelativeNoFollow":                      1,
				"source_construction_acquire.go|(*sourceConstructionBuilder).rebindPackedRefs|factory:(sourcePrimitives).openRelativeNoFollow":                             1,
				"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|factory:(sourcePrimitives).openRelativeNoFollow":                        1,
				"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|factory:(sourcePrimitives).openRootDirectoryDescriptor": 1,
				"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|factory:(sourcePrimitives).openRelativeNoFollow":     1,
				"source_primitives_darwin.go|(darwinSourcePrimitives).openRelativeNoFollow|factory:openDarwinSourceRelativeDescriptor":                                     1,
			},
			"descriptor",
		)...,
	)
	violations = append(
		violations,
		sourceConstructionRawDescriptorFDViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionOwnerUseViolations(
			typedPackage,
			descriptor,
			sourceConstructionAllowedDescriptorOwnerUses(),
			"descriptor",
		)...,
	)
	violations = append(
		violations,
		sourceConstructionOwnerInterfaceErasureViolations(
			typedPackage,
			descriptor,
			"descriptor",
		)...,
	)
	violations = append(
		violations,
		sourceConstructionRootCapabilityViolations(typedPackage)...,
	)
	return violations
}

func sourceConstructionRootCapabilityViolations(
	typedPackage *packages.Package,
) []string {
	allowed := map[string]int{
		"source_primitives.go|(*ownedSourceRoot).validOpen|compare:!=":                                                1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|compare:==":                                              1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|method:Close":                                            1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|assign-left:=":                                           1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|assign-left:=":                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|compare:!=":                          1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|compare:==":                          1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|assign-left:=":                            1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|method:OpenRoot":                          1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|argument:runtime.KeepAlive":               1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|compare:!=":                               1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|compare:==":                               1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|method:Open":                1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|argument:runtime.KeepAlive": 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|method:Lstat":                  1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|argument:runtime.KeepAlive":    1,
	}
	rootObject := typedPackage.Types.Scope().Lookup("ownedSourceRoot")
	if rootObject == nil {
		return []string{"ownedSourceRoot type is missing from root-capability audit"}
	}
	rootType, _ := types.Unalias(rootObject.Type()).(*types.Named)
	if rootType == nil {
		return []string{"ownedSourceRoot type is invalid in root-capability audit"}
	}
	structure, _ := rootType.Underlying().(*types.Struct)
	if structure == nil {
		return []string{"ownedSourceRoot is not a struct in root-capability audit"}
	}
	var rootField *types.Var
	for field := range structure.Fields() {
		if field.Name() == "root" {
			rootField = field
			break
		}
	}
	if rootField == nil || sourceConstructionParameterRole(
		rootField.Type(),
		typedPackage.Types.Path(),
	) != "*os.Root" {
		return []string{"ownedSourceRoot.root is not the exact *os.Root capability"}
	}

	observed, violations := sourceConstructionFieldCapabilitySites(
		typedPackage,
		rootField,
		allowed,
		"root",
	)
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited source root-capability site "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	violations = append(
		violations,
		sourceConstructionOwnedFieldInitializationViolations(
			typedPackage,
			rootField,
			map[string]int{},
			"root",
		)...,
	)
	violations = append(
		violations,
		sourceConstructionOwnedTypeConstructionViolations(
			typedPackage,
			rootType,
			map[string]int{
				"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|composite":                                          1,
				"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|composite":                                               1,
				"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepository|factory:(sourcePrimitives).openRepositoryRoot": 1,
				"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|factory:(sourcePrimitives).openChildRoot": 1,
			},
			"root",
		)...,
	)
	violations = append(
		violations,
		sourceConstructionOwnerInterfaceErasureViolations(
			typedPackage,
			rootType,
			"root",
		)...,
	)
	violations = append(
		violations,
		sourceConstructionOwnerUseViolations(
			typedPackage,
			rootType,
			sourceConstructionAllowedRootOwnerUses(),
			"root",
		)...,
	)
	violations = append(violations, sourceConstructionRootOriginViolations(typedPackage)...)
	return violations
}

func sourceConstructionFieldCapabilitySites(
	typedPackage *packages.Package,
	field *types.Var,
	allowed map[string]int,
	violationLabel string,
) (map[string]int, []string) {
	observed := make(map[string]int)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		parents := sourceConstructionParentNodes(file)
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			selection := typedPackage.TypesInfo.Selections[selector]
			if selection == nil || selection.Obj() != field {
				return true
			}
			function := sourceConstructionEnclosingFunction(file, selector.Pos())
			functionRole := "<outside-function>"
			if function != nil {
				functionRole = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			use := sourceConstructionDescriptorFileUse(typedPackage, selector, parents)
			site := filename + "|" + functionRole + "|" + use
			observed[site]++
			if allowed[site] == 0 {
				violations = append(violations,
					typedPackage.Fset.Position(selector.Pos()).String()+
						" source "+violationLabel+" capability outside audited site "+site,
				)
			}
			return true
		})
	}
	return observed, violations
}

func sourceConstructionOwnedFieldInitializationViolations(
	typedPackage *packages.Package,
	field *types.Var,
	allowed map[string]int,
	violationLabel string,
) []string {
	observed := make(map[string]int)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		ast.Inspect(file, func(node ast.Node) bool {
			keyValue, ok := node.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			key, ok := keyValue.Key.(*ast.Ident)
			if !ok || typedPackage.TypesInfo.Uses[key] != field {
				return true
			}
			function := sourceConstructionEnclosingFunction(file, keyValue.Pos())
			functionRole := "<outside-function>"
			if function != nil {
				functionRole = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			originRole := "<non-call>"
			if call, direct := sourceConstructionDirectCallExpression(keyValue.Value); direct {
				originRole = "call:" + sourceConstructionCallObjectRole(
					sourceConstructionCalledObject(typedPackage.TypesInfo, call.Fun),
					typedPackage.Types.Path(),
				)
			}
			site := filename + "|" + functionRole + "|" + originRole
			observed[site]++
			if allowed[site] == 0 {
				violations = append(violations,
					typedPackage.Fset.Position(keyValue.Pos()).String()+
						" source "+violationLabel+" initializer outside audited site "+site,
				)
			}
			return true
		})
	}
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited source "+violationLabel+" initializer "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	return violations
}

func sourceConstructionOwnedTypeConstructionViolations(
	typedPackage *packages.Package,
	ownerType *types.Named,
	allowed map[string]int,
	violationLabel string,
) []string {
	observed := make(map[string]int)
	violations := make([]string, 0)
	record := func(file *ast.File, node ast.Node, construction string) {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		function := sourceConstructionEnclosingFunction(file, node.Pos())
		functionRole := "<outside-function>"
		if function != nil {
			functionRole = sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			)
		}
		site := filename + "|" + functionRole + "|" + construction
		observed[site]++
		if allowed[site] == 0 {
			violations = append(violations,
				typedPackage.Fset.Position(node.Pos()).String()+
					" source "+violationLabel+" construction outside audited site "+site,
			)
		}
	}
	for _, file := range typedPackage.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.CompositeLit:
				if sourceConstructionDirectExactOwnerResult(
					typedPackage.TypesInfo.TypeOf(current),
					ownerType,
				) {
					record(file, current, "composite")
				}
			case *ast.CallExpr:
				conversion := typedPackage.TypesInfo.Types[current.Fun].IsType()
				constructsOwner := sourceConstructionDirectExactOwnerResult(
					typedPackage.TypesInfo.TypeOf(current),
					ownerType,
				)
				if conversion {
					constructsOwner = sourceConstructionTypeContainsConcreteExactOwner(
						typedPackage.TypesInfo.TypeOf(current),
						ownerType,
					)
				}
				if !constructsOwner && !sourceConstructionConvertsToOwnerAdmittingTypeParameter(
					typedPackage.TypesInfo,
					current,
					ownerType,
				) {
					return true
				}
				called := sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun)
				if builtin, ok := called.(*types.Builtin); ok && builtin.Name() == "new" {
					record(file, current, "new")
					return true
				}
				if conversion {
					record(file, current, "conversion")
					return true
				}
				record(
					file,
					current,
					"factory:"+sourceConstructionCallObjectRole(
						sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun),
						typedPackage.Types.Path(),
					),
				)
			case *ast.Ident:
				instance, instantiated := typedPackage.TypesInfo.Instances[current]
				if !instantiated || !sourceConstructionTypeArgumentsContainExactOwner(
					instance.TypeArgs,
					ownerType,
				) {
					return true
				}
				record(
					file,
					current,
					"instantiation:"+sourceConstructionCallObjectRole(
						typedPackage.TypesInfo.Uses[current],
						typedPackage.Types.Path(),
					),
				)
			case *ast.TypeAssertExpr:
				if current.Type != nil && sourceConstructionTypeContainsConcreteExactOwner(
					typedPackage.TypesInfo.TypeOf(current.Type),
					ownerType,
				) {
					record(file, current, "assertion")
				}
			case *ast.TypeSwitchStmt:
				for _, statement := range current.Body.List {
					clause, ok := statement.(*ast.CaseClause)
					if !ok {
						continue
					}
					for _, expression := range clause.List {
						if sourceConstructionTypeContainsConcreteExactOwner(
							typedPackage.TypesInfo.TypeOf(expression),
							ownerType,
						) {
							record(file, expression, "type-switch")
						}
					}
				}
			}
			return true
		})
	}
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited source "+violationLabel+" construction "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	return violations
}

func sourceConstructionDirectExactOwnerResult(value types.Type, ownerType *types.Named) bool {
	value = types.Unalias(value)
	if tuple, ok := value.(*types.Tuple); ok {
		for variable := range tuple.Variables() {
			if sourceConstructionDirectExactOwnerResult(variable.Type(), ownerType) {
				return true
			}
		}
		return false
	}
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	return ok && named.Obj() == ownerType.Obj()
}

// These exact-count inventories are the review boundary for the two nominal
// raw-handle owners. Adding, removing, or reclassifying a use requires an
// explicit governance update instead of silently expanding the source seam.
func sourceConstructionAllowedDescriptorOwnerUses() map[string]int {
	return map[string]int{
		"source_construction_acquire.go|(*sourceConstructionBuilder).acceptDescriptorAcquisition|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:acceptDescriptorAcquisition":                                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).acceptDescriptorAcquisition|pointer|field>function-type>function-declaration:acceptDescriptorAcquisition":                                                                                                                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).acceptDescriptorAcquisition|variable:descriptor|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:acceptDescriptorAcquisition":                                                                                   1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeFailedPackedRefs|selector:field:descriptor|assignment-=-left-0>function-declaration:closeFailedPackedRefs":                                                                                                                                                  1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeFailedPackedRefs|selector:field:descriptor|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:closeFailedPackedRefs":                                                                                                1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeFailedPackedRefs|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:closeFailedPackedRefs":                                                                                 1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeTransientDescriptor|pointer|field>function-type>function-declaration:closeTransientDescriptor":                                                                                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeTransientDescriptor|selector:function:closeDescriptor|call-function:(sourcePrimitives).closeDescriptor>function-declaration:closeTransientDescriptor":                                                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeTransientDescriptor|variable:descriptor|call-argument-0:(sourcePrimitives).closeDescriptor>function-declaration:closeTransientDescriptor":                                                                                                                   1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeTransientDescriptor|variable:descriptor|function-declaration:closeTransientDescriptor":                                                                                                                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeTransientDescriptors|array-or-slice|field>function-type>function-declaration:closeTransientDescriptors":                                                                                                                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeTransientDescriptors|builtin:len|call-function:len>assignment-:=-right>function-declaration:closeTransientDescriptors":                                                                                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeTransientDescriptors|index|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-=-right>function-declaration:closeTransientDescriptors":                                                                                         1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeTransientDescriptors|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-=-right>function-declaration:closeTransientDescriptors":                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).closeTransientDescriptors|variable:descriptors|call-argument-0:len>assignment-:=-right>function-declaration:closeTransientDescriptors":                                                                                                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).observeDescriptor|function:validateSourceObservationRequest|call-function:validateSourceObservationRequest>assignment-:=-right>function-declaration:observeDescriptor":                                                                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).observeDescriptor|pointer|field>function-type>function-declaration:observeDescriptor":                                                                                                                                                                            1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).observeDescriptor|selector:function:acquireRawACL|call-function:(sourcePrimitives).acquireRawACL>assignment-:=-right>function-declaration:observeDescriptor":                                                                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).observeDescriptor|selector:function:statDescriptor|call-function:(sourcePrimitives).statDescriptor>assignment-:=-right>function-declaration:observeDescriptor":                                                                                                   1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).observeDescriptor|selector:function:statFilesystem|call-function:(sourcePrimitives).statFilesystem>assignment-:=-right>function-declaration:observeDescriptor":                                                                                                   1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).observeDescriptor|variable:descriptor|call-argument-1:(sourcePrimitives).acquireRawACL>assignment-:=-right>function-declaration:observeDescriptor":                                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).observeDescriptor|variable:descriptor|call-argument-1:(sourcePrimitives).statDescriptor>assignment-:=-right>function-declaration:observeDescriptor":                                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).observeDescriptor|variable:descriptor|call-argument-1:(sourcePrimitives).statFilesystem>assignment-:=-right>function-declaration:observeDescriptor":                                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).observeDescriptor|variable:descriptor|call-argument-1:validateSourceObservationRequest>assignment-:=-right>function-declaration:observeDescriptor":                                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindConfig|call:(sourcePrimitives).openRelativeNoFollow|assignment-:=-right>function-declaration:rebindConfig":                                                                                                                                                 1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindConfig|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-:=-right>function-declaration:rebindConfig":                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindConfig|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:rebindConfig":                                                                                                   1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindConfig|selector:function:reobserveDescriptorBeforePolicy|call-function:(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy>assignment-:=-right>function-declaration:rebindConfig":                                                                 1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindConfig|variable:comparison|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-:=-right>function-declaration:rebindConfig":                                                                                                    1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindConfig|variable:comparison|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:rebindConfig":                                                                                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindConfig|variable:comparison|call-argument-0:(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy>assignment-:=-right>function-declaration:rebindConfig":                                                                                             1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindConfig|variable:comparison|function-declaration:rebindConfig":                                                                                                                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindPackedRefs|call:(sourcePrimitives).openRelativeNoFollow|assignment-:=-right>function-declaration:rebindPackedRefs":                                                                                                                                         1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindPackedRefs|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-:=-right>function-declaration:rebindPackedRefs":                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindPackedRefs|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:rebindPackedRefs":                                                                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindPackedRefs|selector:function:reobserveDescriptorBeforePolicy|call-function:(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy>assignment-:=-right>function-declaration:rebindPackedRefs":                                                         1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindPackedRefs|variable:comparison|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-:=-right>function-declaration:rebindPackedRefs":                                                                                            1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindPackedRefs|variable:comparison|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:rebindPackedRefs":                                                                                                                1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindPackedRefs|variable:comparison|call-argument-0:(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy>assignment-:=-right>function-declaration:rebindPackedRefs":                                                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).rebindPackedRefs|variable:comparison|function-declaration:rebindPackedRefs":                                                                                                                                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy|pointer|field>function-type>function-declaration:reobserveDescriptorBeforePolicy":                                                                                                                                                1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy|selector:function:reobserveSourceDescriptorBeforePolicy|call-function:(*sourceConstructionBuilder).reobserveSourceDescriptorBeforePolicy>return>function-declaration:reobserveDescriptorBeforePolicy":                            1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).reobserveSourceDescriptorBeforePolicy>return>function-declaration:reobserveDescriptorBeforePolicy":                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainAbsentPackedRefs|selector:field:descriptor|call-argument-1:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:retainAbsentPackedRefs":                                                                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainAbsentPackedRefs|selector:function:probeRelativeKind|call-function:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:retainAbsentPackedRefs":                                                                                   1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|call:(sourcePrimitives).openRelativeNoFollow|assignment-:=-right>function-declaration:retainConfig":                                                                                                                                                 1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|selector:field:descriptor|assignment-=-left-0>function-declaration:retainConfig":                                                                                                                                                                    1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|selector:field:descriptor|call-argument-1:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:retainConfig":                                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|selector:function:acceptDescriptorAcquisition|call-function:(*sourceConstructionBuilder).acceptDescriptorAcquisition>function-declaration:retainConfig":                                                                                             1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|selector:function:observeDescriptor|call-function:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>function-declaration:retainConfig":                                                                                             1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|selector:function:probeRelativeKind|call-function:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:retainConfig":                                                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|selector:function:readExactForParse|call-function:(sourcePrimitives).readExactForParse>assignment-=-right>function-declaration:retainConfig":                                                                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|selector:function:reobserveDescriptorBeforePolicy|call-function:(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy>assignment-=-right>function-declaration:retainConfig":                                                                  1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|variable:descriptor|assignment-=-right>function-declaration:retainConfig":                                                                                                                                                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).acceptDescriptorAcquisition>function-declaration:retainConfig":                                                                                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>function-declaration:retainConfig":                                                                                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy>assignment-=-right>function-declaration:retainConfig":                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|variable:descriptor|call-argument-1:(sourcePrimitives).readExactForParse>assignment-=-right>function-declaration:retainConfig":                                                                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainConfig|variable:descriptor|function-declaration:retainConfig":                                                                                                                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainGit|selector:field:descriptor|call-argument-2:(*sourceConstructionBuilder).retainSourceDirectory>return>function-declaration:retainGit":                                                                                                                    1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainGit|selector:function:retainSourceDirectory|call-function:(*sourceConstructionBuilder).retainSourceDirectory>return>function-declaration:retainGit":                                                                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainObjects|selector:field:descriptor|call-argument-2:(*sourceConstructionBuilder).retainSourceDirectory>return>function-declaration:retainObjects":                                                                                                            1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainObjects|selector:function:retainSourceDirectory|call-function:(*sourceConstructionBuilder).retainSourceDirectory>return>function-declaration:retainObjects":                                                                                                1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPackedRefs|selector:field:descriptor|call-argument-1:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:retainPackedRefs":                                                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPackedRefs|selector:function:probeRelativeKind|call-function:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:retainPackedRefs":                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPackedRefsContent|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:retainPackedRefsContent":                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPackedRefsContent|selector:field:descriptor|assignment-:=-right>function-declaration:retainPackedRefsContent":                                                                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPackedRefsContent|selector:field:descriptor|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:retainPackedRefsContent":                                                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPackedRefsContent|selector:function:readExactForParse|call-function:(sourcePrimitives).readExactForParse>assignment-:=-right>function-declaration:retainPackedRefsContent":                                                                                 1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPackedRefsContent|selector:function:reobserveDescriptorBeforePolicy|call-function:(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy>assignment-:=-right>function-declaration:retainPackedRefsContent":                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPackedRefsContent|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).reobserveDescriptorBeforePolicy>assignment-:=-right>function-declaration:retainPackedRefsContent":                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPackedRefsContent|variable:descriptor|call-argument-1:(sourcePrimitives).readExactForParse>assignment-:=-right>function-declaration:retainPackedRefsContent":                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPresentPackedRefs|call:(sourcePrimitives).openRelativeNoFollow|assignment-:=-right>function-declaration:retainPresentPackedRefs":                                                                                                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPresentPackedRefs|selector:field:descriptor|assignment-=-left-0>function-declaration:retainPresentPackedRefs":                                                                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPresentPackedRefs|selector:function:acceptDescriptorAcquisition|call-function:(*sourceConstructionBuilder).acceptDescriptorAcquisition>function-declaration:retainPresentPackedRefs":                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPresentPackedRefs|selector:function:observeDescriptor|call-function:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>function-declaration:retainPresentPackedRefs":                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPresentPackedRefs|variable:descriptor|assignment-=-right>function-declaration:retainPresentPackedRefs":                                                                                                                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPresentPackedRefs|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).acceptDescriptorAcquisition>function-declaration:retainPresentPackedRefs":                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPresentPackedRefs|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>function-declaration:retainPresentPackedRefs":                                                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainPresentPackedRefs|variable:descriptor|function-declaration:retainPresentPackedRefs":                                                                                                                                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|call:(sourcePrimitives).openPhysicalRootDescriptor|assignment-:=-right>function-declaration:retainRepositoryDescriptor":                                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|call:(sourcePrimitives).openRelativeNoFollow|assignment-:=-right>range-:=>function-declaration:retainRepositoryDescriptor":                                                                                                            1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|call:append|assignment-=-right>function-declaration:retainRepositoryDescriptor":                                                                                                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|call:append|assignment-=-right>range-:=>function-declaration:retainRepositoryDescriptor":                                                                                                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|call:make|assignment-:=-right>function-declaration:retainRepositoryDescriptor":                                                                                                                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:field:descriptor|assignment-=-left-0>range-:=>function-declaration:retainRepositoryDescriptor":                                                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:field:descriptor|call-argument-2:(sourcePrimitives).compareRootAndDescriptor>assignment-:=-right>function-declaration:retainRepositoryDescriptor":                                                                            1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:function:acceptDescriptorAcquisition|call-function:(*sourceConstructionBuilder).acceptDescriptorAcquisition>function-declaration:retainRepositoryDescriptor":                                                                 1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:function:acceptDescriptorAcquisition|call-function:(*sourceConstructionBuilder).acceptDescriptorAcquisition>range-:=>function-declaration:retainRepositoryDescriptor":                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:function:closeTransientDescriptors|call-function:(*sourceConstructionBuilder).closeTransientDescriptors>function-declaration:retainRepositoryDescriptor":                                                                     3,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:function:closeTransientDescriptors|call-function:(*sourceConstructionBuilder).closeTransientDescriptors>range-:=>function-declaration:retainRepositoryDescriptor":                                                            2,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:function:closeTransientDescriptors|call-function:(*sourceConstructionBuilder).closeTransientDescriptors>return>function-declaration:retainRepositoryDescriptor":                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:function:compareRootAndDescriptor|call-function:(sourcePrimitives).compareRootAndDescriptor>assignment-:=-right>function-declaration:retainRepositoryDescriptor":                                                             1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:function:observeDescriptor|call-function:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>function-declaration:retainRepositoryDescriptor":                                                                 1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:function:observeDescriptor|call-function:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>range-:=>function-declaration:retainRepositoryDescriptor":                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:next|assignment-=-right>range-:=>function-declaration:retainRepositoryDescriptor":                                                                                                                                            2,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:next|call-argument-0:(*sourceConstructionBuilder).acceptDescriptorAcquisition>range-:=>function-declaration:retainRepositoryDescriptor":                                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:next|call-argument-0:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>range-:=>function-declaration:retainRepositoryDescriptor":                                                                            1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:next|range-:=>function-declaration:retainRepositoryDescriptor":                                                                                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:parent|assignment-=-left-0>range-:=>function-declaration:retainRepositoryDescriptor":                                                                                                                                         1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:physicalRoot|assignment-:=-right>function-declaration:retainRepositoryDescriptor":                                                                                                                                            1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:physicalRoot|call-argument-0:(*sourceConstructionBuilder).acceptDescriptorAcquisition>function-declaration:retainRepositoryDescriptor":                                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:physicalRoot|call-argument-0:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>function-declaration:retainRepositoryDescriptor":                                                                             1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:physicalRoot|function-declaration:retainRepositoryDescriptor":                                                                                                                                                                1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:transient|assignment-=-left-0>function-declaration:retainRepositoryDescriptor":                                                                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:transient|assignment-=-left-0>range-:=>function-declaration:retainRepositoryDescriptor":                                                                                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:transient|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptors>function-declaration:retainRepositoryDescriptor":                                                                                            3,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:transient|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptors>range-:=>function-declaration:retainRepositoryDescriptor":                                                                                   2,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|variable:transient|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptors>return>function-declaration:retainRepositoryDescriptor":                                                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|call:(sourcePrimitives).openRelativeNoFollow|assignment-:=-right>function-declaration:retainSourceDirectory":                                                                                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:retainSourceDirectory":                                                                                                  1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|pointer|field>function-type>function-declaration:retainSourceDirectory":                                                                                                                                                                    1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|selector:field:descriptor|assignment-=-left-0>function-declaration:retainSourceDirectory":                                                                                                                                                  1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|selector:function:acceptDescriptorAcquisition|call-function:(*sourceConstructionBuilder).acceptDescriptorAcquisition>function-declaration:retainSourceDirectory":                                                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|selector:function:compareRootAndDescriptor|call-function:(sourcePrimitives).compareRootAndDescriptor>assignment-=-right>function-declaration:retainSourceDirectory":                                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|selector:function:observeDescriptor|call-function:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>function-declaration:retainSourceDirectory":                                                                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|selector:function:probeRelativeKind|call-function:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:retainSourceDirectory":                                                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:descriptor|assignment-=-right>function-declaration:retainSourceDirectory":                                                                                                                                                         1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).acceptDescriptorAcquisition>function-declaration:retainSourceDirectory":                                                                                                   1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).observeDescriptor>assignment-:=-right>function-declaration:retainSourceDirectory":                                                                                         1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:descriptor|call-argument-2:(sourcePrimitives).compareRootAndDescriptor>assignment-=-right>function-declaration:retainSourceDirectory":                                                                                             1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:descriptor|function-declaration:retainSourceDirectory":                                                                                                                                                                            1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:parentDescriptor|call-argument-1:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:retainSourceDirectory":                                                                                             1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:parentDescriptor|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:retainSourceDirectory":                                                                                         1,
		"source_construction_acquire.go|validateSourceObservationRequest|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:validateSourceObservationRequest":                                                                                                         1,
		"source_construction_acquire.go|validateSourceObservationRequest|pointer|field>function-type>function-declaration:validateSourceObservationRequest":                                                                                                                                                                           1,
		"source_construction_acquire.go|validateSourceObservationRequest|variable:descriptor|selector-receiver:kind>function-declaration:validateSourceObservationRequest":                                                                                                                                                            1,
		"source_construction_acquire.go|validateSourceObservationRequest|variable:descriptor|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:validateSourceObservationRequest":                                                                                                      1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|call:(sourcePrimitives).openRelativeNoFollow|assignment-:=-right>function-declaration:captureOpenedSourceAdministrativeEntry":                                                                                           1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|pointer|field>function-type>function-declaration:captureOpenedSourceAdministrativeEntry":                                                                                                                                1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|selector:function:captureSourceAdministrativeDirectory|call-function:(*sourceConstructionBuilder).captureSourceAdministrativeDirectory>assignment-=-right>function-declaration:captureOpenedSourceAdministrativeEntry":  1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-:=-right>function-declaration:captureOpenedSourceAdministrativeEntry":                         1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:captureOpenedSourceAdministrativeEntry":                                             2,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|selector:function:observeSourceDescriptorWithoutPolicy|call-function:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:captureOpenedSourceAdministrativeEntry": 1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-:=-right>function-declaration:captureOpenedSourceAdministrativeEntry":                                              1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:captureOpenedSourceAdministrativeEntry":                                                                  2,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:captureOpenedSourceAdministrativeEntry":                                  1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|variable:descriptor|call-argument-1:(*sourceConstructionBuilder).captureSourceAdministrativeDirectory>assignment-=-right>function-declaration:captureOpenedSourceAdministrativeEntry":                                   1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|variable:descriptor|function-declaration:captureOpenedSourceAdministrativeEntry":                                                                                                                                        1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|pointer|field>function-type>function-declaration:captureSourceAdministrativeDirectory":                                                                                                                                    1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|selector:function:captureSourceAdministrativeEntry|call-function:(*sourceConstructionBuilder).captureSourceAdministrativeEntry>range-:=>function-declaration:captureSourceAdministrativeDirectory":                        1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|selector:function:observeSourceDescriptorWithoutPolicy|call-function:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:captureSourceAdministrativeDirectory":     1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|selector:function:readDirectoryBatch|call-function:(sourcePrimitives).readDirectoryBatch>assignment-:=-right>function-declaration:captureSourceAdministrativeDirectory":                                                   1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:captureSourceAdministrativeDirectory":                                      1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|variable:descriptor|call-argument-1:(*sourceConstructionBuilder).captureSourceAdministrativeEntry>range-:=>function-declaration:captureSourceAdministrativeDirectory":                                                     1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|variable:descriptor|call-argument-1:(sourcePrimitives).readDirectoryBatch>assignment-:=-right>function-declaration:captureSourceAdministrativeDirectory":                                                                  1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|variable:descriptor|function-declaration:captureSourceAdministrativeDirectory":                                                                                                                                            1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeDirectory|variable:descriptor|selector-receiver:kind>function-declaration:captureSourceAdministrativeDirectory":                                                                                                                     1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeEntry|pointer|field>function-type>function-declaration:captureSourceAdministrativeEntry":                                                                                                                                            1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeEntry|selector:function:captureOpenedSourceAdministrativeEntry|call-function:(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry>return>function-declaration:captureSourceAdministrativeEntry":                      1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeEntry|selector:function:probeRelativeKind|call-function:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:captureSourceAdministrativeEntry":                                                             1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeEntry|variable:parent|call-argument-1:(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry>return>function-declaration:captureSourceAdministrativeEntry":                                                             1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeEntry|variable:parent|call-argument-1:(sourcePrimitives).probeRelativeKind>assignment-:=-right>function-declaration:captureSourceAdministrativeEntry":                                                                               1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).compareSourceDescriptorWithoutPolicy|pointer|field>function-type>function-declaration:compareSourceDescriptorWithoutPolicy":                                                                                                                                    1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).compareSourceDescriptorWithoutPolicy|selector:function:observeSourceDescriptorWithoutPolicy|call-function:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:compareSourceDescriptorWithoutPolicy":     1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).compareSourceDescriptorWithoutPolicy|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:compareSourceDescriptorWithoutPolicy":                                      1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy|function:validateSourceObservationRequest|call-function:validateSourceObservationRequest>assignment-:=-right>function-declaration:observeSourceDescriptorWithoutPolicy":                                                   1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy|pointer|field>function-type>function-declaration:observeSourceDescriptorWithoutPolicy":                                                                                                                                    1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy|selector:function:acquireRawACL|call-function:(sourcePrimitives).acquireRawACL>assignment-:=-right>function-declaration:observeSourceDescriptorWithoutPolicy":                                                             1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy|selector:function:statDescriptor|call-function:(sourcePrimitives).statDescriptor>assignment-:=-right>function-declaration:observeSourceDescriptorWithoutPolicy":                                                           1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy|selector:function:statFilesystem|call-function:(sourcePrimitives).statFilesystem>assignment-:=-right>function-declaration:observeSourceDescriptorWithoutPolicy":                                                           1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy|variable:descriptor|call-argument-1:(sourcePrimitives).acquireRawACL>assignment-:=-right>function-declaration:observeSourceDescriptorWithoutPolicy":                                                                       1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy|variable:descriptor|call-argument-1:(sourcePrimitives).statDescriptor>assignment-:=-right>function-declaration:observeSourceDescriptorWithoutPolicy":                                                                      1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy|variable:descriptor|call-argument-1:(sourcePrimitives).statFilesystem>assignment-:=-right>function-declaration:observeSourceDescriptorWithoutPolicy":                                                                      1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy|variable:descriptor|call-argument-1:validateSourceObservationRequest>assignment-:=-right>function-declaration:observeSourceDescriptorWithoutPolicy":                                                                       1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).reobserveSourceDescriptorBeforePolicy|pointer|field>function-type>function-declaration:reobserveSourceDescriptorBeforePolicy":                                                                                                                                  1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).reobserveSourceDescriptorBeforePolicy|selector:function:observeSourceDescriptorWithoutPolicy|call-function:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:reobserveSourceDescriptorBeforePolicy":   1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).reobserveSourceDescriptorBeforePolicy|variable:descriptor|call-argument-0:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:reobserveSourceDescriptorBeforePolicy":                                    1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|call:(sourcePrimitives).openRootDirectoryDescriptor|assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                                                                                          1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:field:descriptor|call-argument-0:(*sourceConstructionBuilder).compareSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                                  1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:function:captureSourceAdministrativeDirectory|call-function:(*sourceConstructionBuilder).captureSourceAdministrativeDirectory>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":       1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                               1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:function:closeTransientDescriptor|call-function:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:retainSourceAdministrativeInventory":                                                   3,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:function:compareRootAndDescriptor|call-function:(sourcePrimitives).compareRootAndDescriptor>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                                         1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:function:compareSourceDescriptorWithoutPolicy|call-function:(*sourceConstructionBuilder).compareSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":       1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:function:observeSourceDescriptorWithoutPolicy|call-function:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":       1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|variable:scan|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                                                          1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|variable:scan|call-argument-0:(*sourceConstructionBuilder).closeTransientDescriptor>function-declaration:retainSourceAdministrativeInventory":                                                                              3,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|variable:scan|call-argument-0:(*sourceConstructionBuilder).observeSourceDescriptorWithoutPolicy>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                                              1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|variable:scan|call-argument-1:(*sourceConstructionBuilder).captureSourceAdministrativeDirectory>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                                              1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|variable:scan|call-argument-2:(sourcePrimitives).compareRootAndDescriptor>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                                                                    1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|variable:scan|function-declaration:retainSourceAdministrativeInventory":                                                                                                                                                    1,
		"source_construction.go|(*sourceCloseTracker).closeConfig|selector:function:closeDescriptor|call-function:(*sourceCloseTracker).closeDescriptor>function-declaration:closeConfig":                                                                                                                                             1,
		"source_construction.go|(*sourceCloseTracker).closeConfig|unary:&|call-argument-0:(*sourceCloseTracker).closeDescriptor>function-declaration:closeConfig":                                                                                                                                                                     1,
		"source_construction.go|(*sourceCloseTracker).closeDescriptor|pointer|assignment-=-left-0>function-declaration:closeDescriptor":                                                                                                                                                                                               1,
		"source_construction.go|(*sourceCloseTracker).closeDescriptor|pointer|call-argument-0:(sourceHandleCloser).closeDescriptor>function-declaration:closeDescriptor":                                                                                                                                                              1,
		"source_construction.go|(*sourceCloseTracker).closeDescriptor|pointer|field>function-type>function-declaration:closeDescriptor":                                                                                                                                                                                               1,
		"source_construction.go|(*sourceCloseTracker).closeDescriptor|pointer|function-declaration:closeDescriptor":                                                                                                                                                                                                                   1,
		"source_construction.go|(*sourceCloseTracker).closeDescriptor|selector:function:closeDescriptor|call-function:(sourceHandleCloser).closeDescriptor>function-declaration:closeDescriptor":                                                                                                                                      1,
		"source_construction.go|(*sourceCloseTracker).closeDescriptor|variable:owner|function-declaration:closeDescriptor":                                                                                                                                                                                                            1,
		"source_construction.go|(*sourceCloseTracker).closePackedRefs|selector:function:closeDescriptor|call-function:(*sourceCloseTracker).closeDescriptor>function-declaration:closePackedRefs":                                                                                                                                     1,
		"source_construction.go|(*sourceCloseTracker).closePackedRefs|unary:&|call-argument-0:(*sourceCloseTracker).closeDescriptor>function-declaration:closePackedRefs":                                                                                                                                                             1,
		"source_construction.go|(*sourceCloseTracker).closeRootBundle|selector:function:closeDescriptor|call-function:(*sourceCloseTracker).closeDescriptor>function-declaration:closeRootBundle":                                                                                                                                     1,
		"source_construction.go|(*sourceCloseTracker).closeRootBundle|unary:&|call-argument-0:(*sourceCloseTracker).closeDescriptor>function-declaration:closeRootBundle":                                                                                                                                                             1,
		"source_construction.go|(*sourceConstructionOwner).validInitialConfig|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>return>function-declaration:validInitialConfig":                                                                                                           1,
		"source_construction.go|(*sourceConstructionOwner).validInitialConfig|selector:field:descriptor|selector-receiver:kind>return>function-declaration:validInitialConfig":                                                                                                                                                        1,
		"source_construction.go|(*sourceConstructionOwner).validInitialConfig|selector:field:descriptor|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>return>function-declaration:validInitialConfig":                                                                                                  1,
		"source_construction.go|(*sourceConstructionOwner).validPackedRefsRetention|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>return>function-declaration:validPackedRefsRetention":                                                                                               1,
		"source_construction.go|(*sourceConstructionOwner).validPackedRefsRetention|selector:field:descriptor|selector-receiver:kind>return>function-declaration:validPackedRefsRetention":                                                                                                                                            1,
		"source_construction.go|(*sourceConstructionOwner).validPackedRefsRetention|selector:field:descriptor|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>return>function-declaration:validPackedRefsRetention":                                                                                      1,
		"source_construction.go|(*sourceConstructionOwner).validResolvedPackedRefs|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>return>function-declaration:validResolvedPackedRefs":                                                                                                 1,
		"source_construction.go|(*sourceConstructionOwner).validResolvedPackedRefs|selector:field:descriptor|selector-receiver:kind>return>function-declaration:validResolvedPackedRefs":                                                                                                                                              1,
		"source_construction.go|(*sourceConstructionOwner).validResolvedPackedRefs|selector:field:descriptor|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>return>function-declaration:validResolvedPackedRefs":                                                                                        1,
		"source_construction.go|(directSourceHandleCloser).closeDescriptor|function:closeDirect|selector-member:closeDirect>call-function:(*ownedSourceDescriptor).closeDirect>return>function-declaration:closeDescriptor":                                                                                                           1,
		"source_construction.go|(directSourceHandleCloser).closeDescriptor|pointer|field>function-type>function-declaration:closeDescriptor":                                                                                                                                                                                          1,
		"source_construction.go|(directSourceHandleCloser).closeDescriptor|variable:owner|selector-receiver:closeDirect>call-function:(*ownedSourceDescriptor).closeDirect>return>function-declaration:closeDescriptor":                                                                                                               1,
		"source_construction.go|(retainedSourceRoot).validOpenDirectory|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>return>function-declaration:validOpenDirectory":                                                                                                                 1,
		"source_construction.go|(retainedSourceRoot).validOpenDirectory|selector:field:descriptor|selector-receiver:kind>return>function-declaration:validOpenDirectory":                                                                                                                                                              1,
		"source_construction.go|(retainedSourceRoot).validOpenDirectory|selector:field:descriptor|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>return>function-declaration:validOpenDirectory":                                                                                                        1,
		"source_construction.go|(sourcePackedRefsSlot).valid|selector:field:descriptor|return>function-declaration:valid":                                                                                                                                                                                                             1,
		"source_construction.go|<outside-function>|function-type|field>interface>type-spec:sourceHandleCloser>declaration:type":                                                                                                                                                                                                       1,
		"source_construction.go|<outside-function>|interface|type-spec:sourceHandleCloser>declaration:type":                                                                                                                                                                                                                           1,
		"source_construction.go|<outside-function>|pointer|field>function-type>field>interface>type-spec:sourceHandleCloser>declaration:type":                                                                                                                                                                                         1,
		"source_construction.go|<outside-function>|pointer|field>struct>type-spec:retainedSourceConfig>declaration:type":                                                                                                                                                                                                              1,
		"source_construction.go|<outside-function>|pointer|field>struct>type-spec:retainedSourcePackedRefs>declaration:type":                                                                                                                                                                                                          1,
		"source_construction.go|<outside-function>|pointer|field>struct>type-spec:retainedSourceRoot>declaration:type":                                                                                                                                                                                                                1,
		"source_construction.go|<outside-function>|struct|type-spec:retainedSourceConfig>declaration:type":                                                                                                                                                                                                                            1,
		"source_construction.go|<outside-function>|struct|type-spec:retainedSourcePackedRefs>declaration:type":                                                                                                                                                                                                                        1,
		"source_construction.go|<outside-function>|struct|type-spec:retainedSourceRoot>declaration:type":                                                                                                                                                                                                                              1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).acquireRawACL|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:acquireRawACL":                                                                                                                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).acquireRawACL|pointer|field>function-type>function-declaration:acquireRawACL":                                                                                                                                                                                           1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).acquireRawACL|variable:owner|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:acquireRawACL":                                                                                                                                               1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).acquireRawACL|variable:owner|selector-receiver:file>selector-receiver:Fd>call-function:os.Fd>call-argument-0:int>call-argument-0:darwinFgetattrlist>assignment-:=-right>function-declaration:acquireRawACL":                                                             1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).acquireRawACL|variable:owner|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:acquireRawACL":                                                                                                                           1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).closeDescriptor|function:closeDirect|selector-member:closeDirect>call-function:(*ownedSourceDescriptor).closeDirect>return>function-declaration:closeDescriptor":                                                                                                        1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).closeDescriptor|pointer|field>function-type>function-declaration:closeDescriptor":                                                                                                                                                                                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).closeDescriptor|variable:owner|function-declaration:closeDescriptor":                                                                                                                                                                                                    1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).closeDescriptor|variable:owner|selector-receiver:closeDirect>call-function:(*ownedSourceDescriptor).closeDirect>return>function-declaration:closeDescriptor":                                                                                                            1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:compareRootAndDescriptor":                                                                                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|pointer|field>function-type>function-declaration:compareRootAndDescriptor":                                                                                                                                                                     1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|variable:descriptor|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:compareRootAndDescriptor":                                                                                                                    1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|variable:descriptor|selector-receiver:file>selector-receiver:Stat>call-function:os.Stat>assignment-:=-right>function-declaration:compareRootAndDescriptor":                                                                                     1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|variable:descriptor|selector-receiver:kind>function-declaration:compareRootAndDescriptor":                                                                                                                                                      1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|variable:descriptor|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:compareRootAndDescriptor":                                                                                                1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|pointer|field>function-type>function-declaration:openPhysicalRootDescriptor":                                                                                                                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unary:&|assignment-:=-right>function-declaration:openPhysicalRootDescriptor":                                                                                                                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|variable:owner|return>function-declaration:openPhysicalRootDescriptor":                                                                                                                                                                       3,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|variable:owner|selector-receiver:file>function-declaration:openPhysicalRootDescriptor":                                                                                                                                                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRelativeNoFollow|call:openDarwinSourceRelativeDescriptor|return>function-declaration:openRelativeNoFollow":                                                                                                                                                          1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRelativeNoFollow|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:openRelativeNoFollow":                                                                                                           1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRelativeNoFollow|pointer|field>function-type>function-declaration:openRelativeNoFollow":                                                                                                                                                                             2,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRelativeNoFollow|variable:parent|selector-receiver:kind>function-declaration:openRelativeNoFollow":                                                                                                                                                                  1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRelativeNoFollow|variable:parent|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:openRelativeNoFollow":                                                                                                            1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|pointer|field>function-type>function-declaration:openRootDirectoryDescriptor":                                                                                                                                                               1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|unary:&|assignment-:=-right>function-declaration:openRootDirectoryDescriptor":                                                                                                                                                               1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|variable:owner|return>function-declaration:openRootDirectoryDescriptor":                                                                                                                                                                     7,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|variable:owner|selector-receiver:file>assignment-=-left-0>function-declaration:openRootDirectoryDescriptor":                                                                                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|variable:owner|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:openRootDirectoryDescriptor":                                                                                                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|variable:owner|selector-receiver:file>function-declaration:openRootDirectoryDescriptor":                                                                                                                                                     2,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|variable:owner|selector-receiver:file>selector-receiver:Fd>call-function:os.Fd>call-argument-0:int>call-argument-0:requireDescriptorKind>assignment-:=-right>function-declaration:openRootDirectoryDescriptor":                              1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:probeRelativeKind":                                                                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|pointer|field>function-type>function-declaration:probeRelativeKind":                                                                                                                                                                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|variable:parent|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:probeRelativeKind":                                                                                                                                      1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|variable:parent|selector-receiver:file>selector-receiver:Fd>call-function:os.Fd>call-argument-0:int>call-argument-0:unix.Fstatat>assignment-:=-right>function-declaration:probeRelativeKind":                                                          1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|variable:parent|selector-receiver:kind>function-declaration:probeRelativeKind":                                                                                                                                                                        1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|variable:parent|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:probeRelativeKind":                                                                                                                  1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:readDirectoryBatch":                                                                                                               1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|pointer|field>function-type>function-declaration:readDirectoryBatch":                                                                                                                                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|variable:owner|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:readDirectoryBatch":                                                                                                                                     1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|variable:owner|selector-receiver:file>selector-receiver:ReadDir>call-function:os.ReadDir>assignment-:=-right>function-declaration:readDirectoryBatch":                                                                                                1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|variable:owner|selector-receiver:kind>function-declaration:readDirectoryBatch":                                                                                                                                                                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|variable:owner|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:readDirectoryBatch":                                                                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readExactForParse|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:readExactForParse":                                                                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readExactForParse|pointer|field>function-type>function-declaration:readExactForParse":                                                                                                                                                                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readExactForParse|variable:owner|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:readExactForParse":                                                                                                                                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readExactForParse|variable:owner|selector-receiver:file>selector-receiver:ReadAt>call-function:os.ReadAt>assignment-:=-right>function-declaration:readExactForParse":                                                                                                    1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readExactForParse|variable:owner|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:readExactForParse":                                                                                                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:statDescriptor":                                                                                                                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|pointer|field>function-type>function-declaration:statDescriptor":                                                                                                                                                                                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|variable:owner|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:statDescriptor":                                                                                                                                             1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|variable:owner|selector-receiver:file>selector-receiver:Fd>call-function:os.Fd>call-argument-0:int>call-argument-0:unix.Fstat>assignment-:=-right>function-declaration:statDescriptor":                                                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|variable:owner|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:statDescriptor":                                                                                                                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statFilesystem|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:statFilesystem":                                                                                                                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statFilesystem|pointer|field>function-type>function-declaration:statFilesystem":                                                                                                                                                                                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statFilesystem|variable:owner|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:statFilesystem":                                                                                                                                             1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statFilesystem|variable:owner|selector-receiver:file>selector-receiver:Fd>call-function:os.Fd>call-argument-0:int>call-argument-0:unix.Fstatfs>assignment-:=-right>function-declaration:statFilesystem":                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statFilesystem|variable:owner|selector-receiver:validOpen>call-function:(*ownedSourceDescriptor).validOpen>function-declaration:statFilesystem":                                                                                                                         1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|pointer|field>function-type>function-declaration:openDarwinSourceRelativeDescriptor":                                                                                                                                                                          2,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|unary:&|assignment-:=-right>function-declaration:openDarwinSourceRelativeDescriptor":                                                                                                                                                                          1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|variable:owner|return>function-declaration:openDarwinSourceRelativeDescriptor":                                                                                                                                                                                7,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|variable:owner|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:openDarwinSourceRelativeDescriptor":                                                                                                                              1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|variable:owner|selector-receiver:file>function-declaration:openDarwinSourceRelativeDescriptor":                                                                                                                                                                1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|variable:owner|selector-receiver:file>selector-receiver:Fd>call-function:os.Fd>call-argument-0:int>call-argument-0:requireDescriptorKind>assignment-:=-right>function-declaration:openDarwinSourceRelativeDescriptor":                                         1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|variable:parent|selector-receiver:file>call-argument-0:runtime.KeepAlive>function-declaration:openDarwinSourceRelativeDescriptor":                                                                                                                             1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|variable:parent|selector-receiver:file>selector-receiver:Fd>call-function:os.Fd>call-argument-0:int>call-argument-0:unix.Openat>assignment-:=-right>function-declaration:openDarwinSourceRelativeDescriptor":                                                  1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|pointer|field>function-declaration:closeDirect":                                                                                                                                                                                                                    1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|variable:owner|function-declaration:closeDirect":                                                                                                                                                                                                                   1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|variable:owner|selector-receiver:closeFailure>assignment-=-left-0>function-declaration:closeDirect":                                                                                                                                                                3,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|variable:owner|selector-receiver:closeFailure>return>function-declaration:closeDirect":                                                                                                                                                                             2,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|variable:owner|selector-receiver:file>assignment-=-left-0>function-declaration:closeDirect":                                                                                                                                                                        1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|variable:owner|selector-receiver:file>function-declaration:closeDirect":                                                                                                                                                                                            1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|variable:owner|selector-receiver:file>selector-receiver:Close>call-function:os.Close>function-declaration:closeDirect":                                                                                                                                             1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|variable:owner|selector-receiver:state>assignment-:=-right>function-declaration:closeDirect":                                                                                                                                                                       1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|variable:owner|selector-receiver:state>assignment-=-left-0>function-declaration:closeDirect":                                                                                                                                                                       1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|variable:owner|selector-receiver:state>function-declaration:closeDirect":                                                                                                                                                                                           1,
		"source_primitives.go|(*ownedSourceDescriptor).validOpen|pointer|field>function-declaration:validOpen":                                                                                                                                                                                                                        1,
		"source_primitives.go|(*ownedSourceDescriptor).validOpen|variable:owner|return>function-declaration:validOpen":                                                                                                                                                                                                                1,
		"source_primitives.go|(*ownedSourceDescriptor).validOpen|variable:owner|selector-receiver:file>return>function-declaration:validOpen":                                                                                                                                                                                         1,
		"source_primitives.go|(*ownedSourceDescriptor).validOpen|variable:owner|selector-receiver:kind>selector-receiver:valid>call-function:(sourceObservedKind).valid>return>function-declaration:validOpen":                                                                                                                        1,
		"source_primitives.go|(*ownedSourceDescriptor).validOpen|variable:owner|selector-receiver:state>return>function-declaration:validOpen":                                                                                                                                                                                        1,
		"source_primitives.go|<outside-function>|function-type|field>interface>type-spec:sourcePrimitives>declaration:type":                                                                                                                                                                                                           11,
		"source_primitives.go|<outside-function>|interface|type-spec:sourcePrimitives>declaration:type":                                                                                                                                                                                                                               1,
		"source_primitives.go|<outside-function>|pointer|field>function-type>field>interface>type-spec:sourcePrimitives>declaration:type":                                                                                                                                                                                             12,
	}
}

func sourceConstructionAllowedRootOwnerUses() map[string]int {
	return map[string]int{
		"source_construction_acquire.go|(*sourceConstructionBuilder).acceptRootAcquisition|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:acceptRootAcquisition":                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).acceptRootAcquisition|pointer|field>function-type>function-declaration:acceptRootAcquisition":                                                                                                                                  1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).acceptRootAcquisition|variable:root|selector-receiver:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:acceptRootAcquisition":                                                                         1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainGit|selector:field:root|call-argument-1:(*sourceConstructionBuilder).retainSourceDirectory>return>function-declaration:retainGit":                                                                                        1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainGit|selector:function:retainSourceDirectory|call-function:(*sourceConstructionBuilder).retainSourceDirectory>return>function-declaration:retainGit":                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainObjects|selector:field:root|call-argument-1:(*sourceConstructionBuilder).retainSourceDirectory>return>function-declaration:retainObjects":                                                                                1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainObjects|selector:function:retainSourceDirectory|call-function:(*sourceConstructionBuilder).retainSourceDirectory>return>function-declaration:retainObjects":                                                              1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepository|call:(sourcePrimitives).openRepositoryRoot|assignment-:=-right>function-declaration:retainRepository":                                                                                                         1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepository|selector:field:root|assignment-=-left-0>function-declaration:retainRepository":                                                                                                                                1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepository|selector:function:acceptRootAcquisition|call-function:(*sourceConstructionBuilder).acceptRootAcquisition>function-declaration:retainRepository":                                                               1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepository|variable:root|assignment-=-right>function-declaration:retainRepository":                                                                                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepository|variable:root|call-argument-0:(*sourceConstructionBuilder).acceptRootAcquisition>function-declaration:retainRepository":                                                                                       1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepository|variable:root|function-declaration:retainRepository":                                                                                                                                                          1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:field:root|call-argument-1:(sourcePrimitives).compareRootAndDescriptor>assignment-:=-right>function-declaration:retainRepositoryDescriptor":                                                1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainRepositoryDescriptor|selector:function:compareRootAndDescriptor|call-function:(sourcePrimitives).compareRootAndDescriptor>assignment-:=-right>function-declaration:retainRepositoryDescriptor":                           1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|call:(sourcePrimitives).openChildRoot|assignment-:=-right>function-declaration:retainSourceDirectory":                                                                                                    1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:retainSourceDirectory":                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|pointer|field>function-type>function-declaration:retainSourceDirectory":                                                                                                                                  1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|selector:field:root|assignment-=-left-0>function-declaration:retainSourceDirectory":                                                                                                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|selector:function:acceptRootAcquisition|call-function:(*sourceConstructionBuilder).acceptRootAcquisition>function-declaration:retainSourceDirectory":                                                     1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|selector:function:compareRootAndDescriptor|call-function:(sourcePrimitives).compareRootAndDescriptor>assignment-=-right>function-declaration:retainSourceDirectory":                                      1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:parentRoot|selector-receiver:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:retainSourceDirectory":                                                                   1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:root|assignment-=-right>function-declaration:retainSourceDirectory":                                                                                                                             1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:root|call-argument-0:(*sourceConstructionBuilder).acceptRootAcquisition>function-declaration:retainSourceDirectory":                                                                             1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:root|call-argument-1:(sourcePrimitives).compareRootAndDescriptor>assignment-=-right>function-declaration:retainSourceDirectory":                                                                 1,
		"source_construction_acquire.go|(*sourceConstructionBuilder).retainSourceDirectory|variable:root|function-declaration:retainSourceDirectory":                                                                                                                                                1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:field:root|call-argument-1:(sourcePrimitives).compareRootAndDescriptor>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                            1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:field:root|call-argument-1:(sourcePrimitives).openRootDirectoryDescriptor>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":                         1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:function:compareRootAndDescriptor|call-function:(sourcePrimitives).compareRootAndDescriptor>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory":       1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|selector:function:openRootDirectoryDescriptor|call-function:(sourcePrimitives).openRootDirectoryDescriptor>assignment-:=-right>function-declaration:retainSourceAdministrativeInventory": 1,
		"source_construction.go|(*sourceCloseTracker).closeRoot|pointer|assignment-=-left-0>function-declaration:closeRoot":                                                                                                                                                                         1,
		"source_construction.go|(*sourceCloseTracker).closeRoot|pointer|call-argument-0:(sourceHandleCloser).closeRoot>function-declaration:closeRoot":                                                                                                                                              1,
		"source_construction.go|(*sourceCloseTracker).closeRoot|pointer|field>function-type>function-declaration:closeRoot":                                                                                                                                                                         1,
		"source_construction.go|(*sourceCloseTracker).closeRoot|pointer|function-declaration:closeRoot":                                                                                                                                                                                             1,
		"source_construction.go|(*sourceCloseTracker).closeRoot|selector:function:closeRoot|call-function:(sourceHandleCloser).closeRoot>function-declaration:closeRoot":                                                                                                                            1,
		"source_construction.go|(*sourceCloseTracker).closeRoot|variable:owner|function-declaration:closeRoot":                                                                                                                                                                                      1,
		"source_construction.go|(*sourceCloseTracker).closeRootBundle|selector:function:closeRoot|call-function:(*sourceCloseTracker).closeRoot>function-declaration:closeRootBundle":                                                                                                               1,
		"source_construction.go|(*sourceCloseTracker).closeRootBundle|unary:&|call-argument-0:(*sourceCloseTracker).closeRoot>function-declaration:closeRootBundle":                                                                                                                                 1,
		"source_construction.go|(directSourceHandleCloser).closeRoot|function:closeDirect|selector-member:closeDirect>call-function:(*ownedSourceRoot).closeDirect>return>function-declaration:closeRoot":                                                                                           1,
		"source_construction.go|(directSourceHandleCloser).closeRoot|pointer|field>function-type>function-declaration:closeRoot":                                                                                                                                                                    1,
		"source_construction.go|(directSourceHandleCloser).closeRoot|variable:owner|selector-receiver:closeDirect>call-function:(*ownedSourceRoot).closeDirect>return>function-declaration:closeRoot":                                                                                               1,
		"source_construction.go|(retainedSourceRoot).validOpenDirectory|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceRoot).validOpen>return>function-declaration:validOpenDirectory":                                                                                     1,
		"source_construction.go|(retainedSourceRoot).validOpenDirectory|selector:field:root|selector-receiver:validOpen>call-function:(*ownedSourceRoot).validOpen>return>function-declaration:validOpenDirectory":                                                                                  1,
		"source_construction.go|<outside-function>|function-type|field>interface>type-spec:sourceHandleCloser>declaration:type":                                                                                                                                                                     1,
		"source_construction.go|<outside-function>|interface|type-spec:sourceHandleCloser>declaration:type":                                                                                                                                                                                         1,
		"source_construction.go|<outside-function>|pointer|field>function-type>field>interface>type-spec:sourceHandleCloser>declaration:type":                                                                                                                                                       1,
		"source_construction.go|<outside-function>|pointer|field>struct>type-spec:retainedSourceRoot>declaration:type":                                                                                                                                                                              1,
		"source_construction.go|<outside-function>|struct|type-spec:retainedSourceRoot>declaration:type":                                                                                                                                                                                            1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).closeRoot|function:closeDirect|selector-member:closeDirect>call-function:(*ownedSourceRoot).closeDirect>return>function-declaration:closeRoot":                                                                                        1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).closeRoot|pointer|field>function-type>function-declaration:closeRoot":                                                                                                                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).closeRoot|variable:owner|function-declaration:closeRoot":                                                                                                                                                                              1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).closeRoot|variable:owner|selector-receiver:closeDirect>call-function:(*ownedSourceRoot).closeDirect>return>function-declaration:closeRoot":                                                                                            1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:compareRootAndDescriptor":                                                                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|pointer|field>function-type>function-declaration:compareRootAndDescriptor":                                                                                                                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|variable:root|selector-receiver:root>call-argument-0:runtime.KeepAlive>function-declaration:compareRootAndDescriptor":                                                                                        1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|variable:root|selector-receiver:root>selector-receiver:Lstat>call-function:os.Lstat>assignment-:=-right>function-declaration:compareRootAndDescriptor":                                                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|variable:root|selector-receiver:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:compareRootAndDescriptor":                                                                          1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:openChildRoot":                                                                                             1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|pointer|field>function-type>function-declaration:openChildRoot":                                                                                                                                                         2,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|unary:&|assignment-:=-right>function-declaration:openChildRoot":                                                                                                                                                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|variable:owner|return>function-declaration:openChildRoot":                                                                                                                                                               3,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|variable:owner|selector-receiver:root>assignment-=-left-0>function-declaration:openChildRoot":                                                                                                                           1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|variable:owner|selector-receiver:root>function-declaration:openChildRoot":                                                                                                                                               2,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|variable:parent|selector-receiver:root>call-argument-0:runtime.KeepAlive>function-declaration:openChildRoot":                                                                                                            1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|variable:parent|selector-receiver:root>selector-receiver:OpenRoot>call-function:os.OpenRoot>assignment-=-right>function-declaration:openChildRoot":                                                                      1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|variable:parent|selector-receiver:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:openChildRoot":                                                                                              1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|pointer|field>function-type>function-declaration:openRepositoryRoot":                                                                                                                                               1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|unary:&|assignment-:=-right>function-declaration:openRepositoryRoot":                                                                                                                                               1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|variable:owner|return>function-declaration:openRepositoryRoot":                                                                                                                                                     3,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|variable:owner|selector-receiver:root>assignment-=-left-0>function-declaration:openRepositoryRoot":                                                                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|variable:owner|selector-receiver:root>function-declaration:openRepositoryRoot":                                                                                                                                     2,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|function:validOpen|selector-member:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:openRootDirectoryDescriptor":                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|pointer|field>function-type>function-declaration:openRootDirectoryDescriptor":                                                                                                                             1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|variable:root|selector-receiver:root>call-argument-0:runtime.KeepAlive>function-declaration:openRootDirectoryDescriptor":                                                                                  1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|variable:root|selector-receiver:root>selector-receiver:Open>call-function:os.Open>assignment-=-right>function-declaration:openRootDirectoryDescriptor":                                                    1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|variable:root|selector-receiver:validOpen>call-function:(*ownedSourceRoot).validOpen>function-declaration:openRootDirectoryDescriptor":                                                                    1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|pointer|field>function-declaration:closeDirect":                                                                                                                                                                                        1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|variable:owner|function-declaration:closeDirect":                                                                                                                                                                                       1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|variable:owner|selector-receiver:closeFailure>assignment-=-left-0>function-declaration:closeDirect":                                                                                                                                    3,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|variable:owner|selector-receiver:closeFailure>return>function-declaration:closeDirect":                                                                                                                                                 2,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|variable:owner|selector-receiver:root>assignment-=-left-0>function-declaration:closeDirect":                                                                                                                                            1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|variable:owner|selector-receiver:root>function-declaration:closeDirect":                                                                                                                                                                1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|variable:owner|selector-receiver:root>selector-receiver:Close>call-function:os.Close>function-declaration:closeDirect":                                                                                                                 1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|variable:owner|selector-receiver:state>assignment-:=-right>function-declaration:closeDirect":                                                                                                                                           1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|variable:owner|selector-receiver:state>assignment-=-left-0>function-declaration:closeDirect":                                                                                                                                           1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|variable:owner|selector-receiver:state>function-declaration:closeDirect":                                                                                                                                                               1,
		"source_primitives.go|(*ownedSourceRoot).validOpen|pointer|field>function-declaration:validOpen":                                                                                                                                                                                            1,
		"source_primitives.go|(*ownedSourceRoot).validOpen|variable:owner|return>function-declaration:validOpen":                                                                                                                                                                                    1,
		"source_primitives.go|(*ownedSourceRoot).validOpen|variable:owner|selector-receiver:root>return>function-declaration:validOpen":                                                                                                                                                             1,
		"source_primitives.go|(*ownedSourceRoot).validOpen|variable:owner|selector-receiver:state>return>function-declaration:validOpen":                                                                                                                                                            1,
		"source_primitives.go|<outside-function>|function-type|field>interface>type-spec:sourcePrimitives>declaration:type":                                                                                                                                                                         5,
		"source_primitives.go|<outside-function>|interface|type-spec:sourcePrimitives>declaration:type":                                                                                                                                                                                             1,
		"source_primitives.go|<outside-function>|pointer|field>function-type>field>interface>type-spec:sourcePrimitives>declaration:type":                                                                                                                                                           6,
	}
}

func sourceConstructionOwnerUseViolations(
	typedPackage *packages.Package,
	ownerType *types.Named,
	allowed map[string]int,
	violationLabel string,
) []string {
	observed := make(map[string]int)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		parents := sourceConstructionParentNodes(file)
		ast.Inspect(file, func(node ast.Node) bool {
			expression, ok := node.(ast.Expr)
			if !ok {
				return true
			}
			if identifier, identifierOK := expression.(*ast.Ident); identifierOK &&
				typedPackage.TypesInfo.Defs[identifier] != nil {
				return true
			}
			if !sourceConstructionOwnerTopologyType(
				typedPackage.TypesInfo.TypeOf(expression),
				ownerType,
			) {
				return true
			}
			if parentExpression, parentOK := parents[expression].(ast.Expr); parentOK &&
				sourceConstructionOwnerTopologyType(
					typedPackage.TypesInfo.TypeOf(parentExpression),
					ownerType,
				) {
				return true
			}

			functionRole := sourceConstructionOwnerUseFunctionRole(
				typedPackage,
				expression,
				parents,
			)
			site := filename + "|" + functionRole + "|" +
				sourceConstructionOwnerUseExpressionRole(
					typedPackage.TypesInfo,
					expression,
					typedPackage.Types.Path(),
				) + "|" +
				sourceConstructionOwnerUseContext(
					typedPackage.TypesInfo,
					expression,
					parents,
					typedPackage.Types.Path(),
				)
			observed[site]++
			return true
		})
	}

	for site, count := range observed {
		if allowed[site] == 0 {
			violations = append(violations,
				"source "+violationLabel+" owner use outside audited site "+site+
					" count = "+strconv.Itoa(count),
			)
		}
	}
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited source "+violationLabel+" owner use "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	violations = append(
		violations,
		sourceConstructionOwnerTerminalEscapeViolations(
			typedPackage,
			ownerType,
			violationLabel,
		)...,
	)
	slices.Sort(violations)
	return violations
}

func sourceConstructionOwnerTopologyType(value types.Type, ownerType *types.Named) bool {
	return sourceConstructionOwnerTopologyTypeWithSeen(
		value,
		ownerType,
		map[types.Type]bool{},
	)
}

// sourceConstructionOwnerTopologyType treats named retained aggregates as
// sealed boundaries. It inventories direct owner values, aliases, container
// carriers, function surfaces, and generic constraints without expanding every
// legitimate retained source object into all of its transitive fields.
func sourceConstructionOwnerTopologyTypeWithSeen(
	value types.Type,
	ownerType *types.Named,
	seen map[types.Type]bool,
) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	switch current := value.(type) {
	case *types.Named:
		if current.Obj() == ownerType.Obj() {
			return true
		}
		return sourceConstructionOwnerTopologyTypeArguments(current.TypeArgs(), ownerType, seen)
	case *types.Pointer:
		return sourceConstructionOwnerTopologyTypeWithSeen(current.Elem(), ownerType, seen)
	case *types.Array:
		return sourceConstructionOwnerTopologyTypeWithSeen(current.Elem(), ownerType, seen)
	case *types.Slice:
		return sourceConstructionOwnerTopologyTypeWithSeen(current.Elem(), ownerType, seen)
	case *types.Map:
		return sourceConstructionOwnerTopologyTypeWithSeen(current.Key(), ownerType, seen) ||
			sourceConstructionOwnerTopologyTypeWithSeen(current.Elem(), ownerType, seen)
	case *types.Chan:
		return sourceConstructionOwnerTopologyTypeWithSeen(current.Elem(), ownerType, seen)
	case *types.Struct:
		for field := range current.Fields() {
			if sourceConstructionOwnerTopologyTypeWithSeen(field.Type(), ownerType, seen) {
				return true
			}
		}
	case *types.Signature:
		if current.Recv() != nil && sourceConstructionOwnerTopologyTypeWithSeen(
			current.Recv().Type(),
			ownerType,
			seen,
		) {
			return true
		}
		if sourceConstructionOwnerTopologyTypeWithSeen(current.Params(), ownerType, seen) ||
			sourceConstructionOwnerTopologyTypeWithSeen(current.Results(), ownerType, seen) {
			return true
		}
		if sourceConstructionOwnerTopologyTypeParameters(current.TypeParams(), ownerType, seen) ||
			sourceConstructionOwnerTopologyTypeParameters(current.RecvTypeParams(), ownerType, seen) {
			return true
		}
	case *types.Tuple:
		for variable := range current.Variables() {
			if sourceConstructionOwnerTopologyTypeWithSeen(variable.Type(), ownerType, seen) {
				return true
			}
		}
	case *types.Interface:
		current.Complete()
		for embedded := range current.EmbeddedTypes() {
			if sourceConstructionOwnerTopologyTypeWithSeen(embedded, ownerType, seen) {
				return true
			}
		}
		for method := range current.Methods() {
			if sourceConstructionOwnerTopologyTypeWithSeen(method.Type(), ownerType, seen) {
				return true
			}
		}
	case *types.TypeParam:
		return sourceConstructionOwnerTopologyTypeWithSeen(
			current.Constraint(),
			ownerType,
			seen,
		)
	case *types.Union:
		for term := range current.Terms() {
			if sourceConstructionOwnerTopologyTypeWithSeen(term.Type(), ownerType, seen) {
				return true
			}
		}
	}
	return false
}

func sourceConstructionOwnerTopologyTypeArguments(
	values *types.TypeList,
	ownerType *types.Named,
	seen map[types.Type]bool,
) bool {
	if values == nil {
		return false
	}
	for value := range values.Types() {
		if sourceConstructionOwnerTopologyTypeWithSeen(value, ownerType, seen) {
			return true
		}
	}
	return false
}

func sourceConstructionOwnerTopologyTypeParameters(
	values *types.TypeParamList,
	ownerType *types.Named,
	seen map[types.Type]bool,
) bool {
	if values == nil {
		return false
	}
	for value := range values.TypeParams() {
		if sourceConstructionOwnerTopologyTypeWithSeen(value, ownerType, seen) {
			return true
		}
	}
	return false
}

func sourceConstructionOwnerUseFunctionRole(
	typedPackage *packages.Package,
	node ast.Node,
	parents map[ast.Node]ast.Node,
) string {
	literalDepth := 0
	for current := node; current != nil; current = parents[current] {
		switch enclosing := current.(type) {
		case *ast.FuncLit:
			literalDepth++
		case *ast.FuncDecl:
			role := sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[enclosing.Name],
				typedPackage.Types.Path(),
			)
			if literalDepth != 0 {
				role += "/literal-" + strconv.Itoa(literalDepth)
			}
			return role
		}
	}
	if literalDepth != 0 {
		return "<package-literal-" + strconv.Itoa(literalDepth) + ">"
	}
	return "<outside-function>"
}

func sourceConstructionOwnerUseObjectRole(object types.Object) string {
	switch current := object.(type) {
	case *types.Var:
		if current.IsField() {
			return "field"
		}
		return "variable"
	case *types.Func:
		return "function"
	case *types.TypeName:
		return "type"
	case *types.Builtin:
		return "builtin"
	case *types.Const:
		return "constant"
	case *types.Label:
		return "label"
	case *types.PkgName:
		return "package"
	default:
		return "object"
	}
}

func sourceConstructionOwnerUseExpressionRole(
	info *types.Info,
	expression ast.Expr,
	packagePath string,
) string {
	switch current := expression.(type) {
	case *ast.Ident:
		object := info.Uses[current]
		return sourceConstructionOwnerUseObjectRole(object) + ":" + current.Name
	case *ast.SelectorExpr:
		object := info.Uses[current.Sel]
		if selection := info.Selections[current]; selection != nil {
			object = selection.Obj()
		}
		return "selector:" + sourceConstructionOwnerUseObjectRole(object) + ":" + current.Sel.Name
	case *ast.CallExpr:
		return "call:" + sourceConstructionCallObjectRole(
			sourceConstructionCalledObject(info, current.Fun),
			packagePath,
		)
	case *ast.CompositeLit:
		return "composite"
	case *ast.StarExpr:
		return "pointer"
	case *ast.IndexExpr:
		return "index"
	case *ast.IndexListExpr:
		return "index-list"
	case *ast.ParenExpr:
		return "parenthesized"
	case *ast.UnaryExpr:
		return "unary:" + current.Op.String()
	case *ast.ArrayType:
		return "array-or-slice"
	case *ast.MapType:
		return "map"
	case *ast.ChanType:
		return "channel"
	case *ast.FuncType:
		return "function-type"
	case *ast.InterfaceType:
		return "interface"
	case *ast.StructType:
		return "struct"
	default:
		return strings.TrimPrefix(reflect.TypeOf(expression).String(), "*ast.")
	}
}

func sourceConstructionOwnerUseContext(
	info *types.Info,
	node ast.Node,
	parents map[ast.Node]ast.Node,
	packagePath string,
) string {
	parts := make([]string, 0, 8)
	for parent := parents[node]; parent != nil; parent = parents[parent] {
		switch current := parent.(type) {
		case *ast.SelectorExpr:
			relation := "receiver"
			if current.Sel.Pos() <= node.Pos() && node.End() <= current.Sel.End() {
				relation = "member"
			}
			parts = append(parts, "selector-"+relation+":"+current.Sel.Name)
		case *ast.CallExpr:
			relation := "function"
			for index, argument := range current.Args {
				if argument.Pos() <= node.Pos() && node.End() <= argument.End() {
					relation = "argument-" + strconv.Itoa(index)
					break
				}
			}
			parts = append(parts, "call-"+relation+":"+sourceConstructionCallObjectRole(
				sourceConstructionCalledObject(info, current.Fun),
				packagePath,
			))
		case *ast.AssignStmt:
			side := "right"
			for index, target := range current.Lhs {
				if target.Pos() <= node.Pos() && node.End() <= target.End() {
					side = "left-" + strconv.Itoa(index)
					break
				}
			}
			parts = append(parts, "assignment-"+current.Tok.String()+"-"+side)
		case *ast.ValueSpec:
			relation := "type"
			for index, value := range current.Values {
				if value.Pos() <= node.Pos() && node.End() <= value.End() {
					relation = "value-" + strconv.Itoa(index)
					break
				}
			}
			parts = append(parts, "value-spec-"+relation)
		case *ast.ReturnStmt:
			parts = append(parts, "return")
		case *ast.SendStmt:
			parts = append(parts, "send")
		case *ast.RangeStmt:
			parts = append(parts, "range-"+current.Tok.String())
		case *ast.GoStmt:
			parts = append(parts, "go")
		case *ast.DeferStmt:
			parts = append(parts, "defer")
		case *ast.FuncLit:
			parts = append(parts, "function-literal")
		case *ast.CompositeLit:
			relation := "element"
			if current.Type != nil && current.Type.Pos() <= node.Pos() &&
				node.End() <= current.Type.End() {
				relation = "type"
			}
			parts = append(parts, "composite-"+relation)
		case *ast.KeyValueExpr:
			relation := "value"
			if current.Key.Pos() <= node.Pos() && node.End() <= current.Key.End() {
				relation = "key"
			}
			parts = append(parts, "key-value-"+relation)
		case *ast.IndexExpr:
			parts = append(parts, "index")
		case *ast.IndexListExpr:
			parts = append(parts, "index-list")
		case *ast.StarExpr:
			parts = append(parts, "pointer")
		case *ast.ArrayType:
			parts = append(parts, "array-or-slice")
		case *ast.MapType:
			parts = append(parts, "map")
		case *ast.ChanType:
			parts = append(parts, "channel")
		case *ast.InterfaceType:
			parts = append(parts, "interface")
		case *ast.StructType:
			parts = append(parts, "struct")
		case *ast.Field:
			parts = append(parts, "field")
		case *ast.FuncType:
			parts = append(parts, "function-type")
		case *ast.TypeSpec:
			parts = append(parts, "type-spec:"+current.Name.Name)
		case *ast.FuncDecl:
			parts = append(parts, "function-declaration:"+current.Name.Name)
			return strings.Join(parts, ">")
		case *ast.GenDecl:
			parts = append(parts, "declaration:"+current.Tok.String())
			return strings.Join(parts, ">")
		}
	}
	if len(parts) == 0 {
		return "bare"
	}
	return strings.Join(parts, ">")
}

func sourceConstructionOwnerTerminalEscapeViolations(
	typedPackage *packages.Package,
	ownerType *types.Named,
	violationLabel string,
) []string {
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		parents := sourceConstructionParentNodes(file)
		ast.Inspect(file, func(node ast.Node) bool {
			record := func(target ast.Node, escape string) {
				function := sourceConstructionEnclosingFunction(file, target.Pos())
				functionRole := "<outside-function>"
				if function != nil {
					functionRole = sourceConstructionFunctionRole(
						typedPackage.TypesInfo.Defs[function.Name],
						typedPackage.Types.Path(),
					)
				}
				violations = append(violations,
					typedPackage.Fset.Position(target.Pos()).String()+" source "+
						violationLabel+" owner "+escape+" outside audited boundary "+
						filename+"|"+functionRole,
				)
			}

			switch current := node.(type) {
			case *ast.Ident:
				object, _ := typedPackage.TypesInfo.Defs[current].(*types.Var)
				if object != nil && object.Parent() == typedPackage.Types.Scope() &&
					current.Name != "_" && sourceConstructionTypeContainsConcreteExactOwner(
					object.Type(),
					ownerType,
				) {
					record(current, "enters package storage")
				}
			case *ast.CallExpr:
				if !typedPackage.TypesInfo.Types[current.Fun].IsType() ||
					sourceConstructionOwnerTopologyType(
						typedPackage.TypesInfo.TypeOf(current),
						ownerType,
					) {
					return true
				}
				for _, argument := range current.Args {
					if sourceConstructionValueContainsConcreteExactOwner(
						typedPackage.TypesInfo.TypeOf(argument),
						ownerType,
					) {
						record(argument, "loses its nominal representation")
					}
				}
			case *ast.FuncLit:
				captured := false
				ast.Inspect(current, func(literalNode ast.Node) bool {
					if captured {
						return false
					}
					identifier, ok := literalNode.(*ast.Ident)
					if !ok {
						return true
					}
					object := typedPackage.TypesInfo.Defs[identifier]
					if object == nil {
						object = typedPackage.TypesInfo.Uses[identifier]
					}
					captured = object != nil && sourceConstructionTypeContainsConcreteExactOwner(
						object.Type(),
						ownerType,
					)
					return !captured
				})
				if captured {
					record(current, "enters a function literal")
				}
			case *ast.SelectorExpr:
				selection := typedPackage.TypesInfo.Selections[current]
				if selection == nil || selection.Kind() != types.MethodVal ||
					!sourceConstructionTypeContainsConcreteExactOwner(selection.Recv(), ownerType) {
					return true
				}
				if call, ok := parents[current].(*ast.CallExpr); !ok || call.Fun != current {
					record(current, "escapes through a method value")
				}
			case *ast.GoStmt:
				if sourceConstructionCallCarriesDirectOwner(
					typedPackage.TypesInfo,
					current.Call,
					ownerType,
				) != "" {
					record(current, "crosses a go boundary")
				}
			case *ast.DeferStmt:
				if sourceConstructionCallCarriesDirectOwner(
					typedPackage.TypesInfo,
					current.Call,
					ownerType,
				) != "" {
					record(current, "crosses a defer boundary")
				}
			}
			return true
		})
	}
	return violations
}

func sourceConstructionCallCarriesDirectOwner(
	info *types.Info,
	call *ast.CallExpr,
	ownerType *types.Named,
) string {
	if selector, ok := sourceConstructionUnwrapCallFunction(call.Fun).(*ast.SelectorExpr); ok {
		if selection := info.Selections[selector]; selection != nil &&
			sourceConstructionTypeContainsConcreteExactOwner(selection.Recv(), ownerType) {
			return ownerType.Obj().Name()
		}
	}
	for _, argument := range call.Args {
		if sourceConstructionTypeContainsConcreteExactOwner(info.TypeOf(argument), ownerType) {
			return ownerType.Obj().Name()
		}
	}
	return ""
}

func sourceConstructionTypeContainsConcreteExactOwner(value types.Type, ownerType *types.Named) bool {
	return sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
		value,
		ownerType,
		map[types.Type]bool{},
		true,
	)
}

func sourceConstructionValueContainsConcreteExactOwner(value types.Type, ownerType *types.Named) bool {
	return sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
		value,
		ownerType,
		map[types.Type]bool{},
		false,
	)
}

func sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
	value types.Type,
	ownerType *types.Named,
	seen map[types.Type]bool,
	inspectContracts bool,
) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	switch current := value.(type) {
	case *types.Named:
		if current.Obj() == ownerType.Obj() {
			return true
		}
		if sourceConstructionTypeArgumentsContainConcreteExactOwnerWithSeen(
			current.TypeArgs(),
			ownerType,
			seen,
			inspectContracts,
		) {
			return true
		}
		if !inspectContracts {
			if _, isInterface := current.Underlying().(*types.Interface); isInterface {
				return false
			}
		}
		return sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Underlying(),
			ownerType,
			seen,
			inspectContracts,
		)
	case *types.Pointer:
		return sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Elem(),
			ownerType,
			seen,
			inspectContracts,
		)
	case *types.Array:
		return sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Elem(),
			ownerType,
			seen,
			inspectContracts,
		)
	case *types.Slice:
		return sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Elem(),
			ownerType,
			seen,
			inspectContracts,
		)
	case *types.Map:
		return sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Key(),
			ownerType,
			seen,
			inspectContracts,
		) || sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Elem(),
			ownerType,
			seen,
			inspectContracts,
		)
	case *types.Chan:
		return sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Elem(),
			ownerType,
			seen,
			inspectContracts,
		)
	case *types.Struct:
		for field := range current.Fields() {
			if sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
				field.Type(),
				ownerType,
				seen,
				inspectContracts,
			) {
				return true
			}
		}
	case *types.Signature:
		if !inspectContracts {
			return false
		}
		if current.Recv() != nil && sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Recv().Type(),
			ownerType,
			seen,
			inspectContracts,
		) {
			return true
		}
		if sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Params(),
			ownerType,
			seen,
			inspectContracts,
		) || sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			current.Results(),
			ownerType,
			seen,
			inspectContracts,
		) {
			return true
		}
	case *types.Tuple:
		for variable := range current.Variables() {
			if sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
				variable.Type(),
				ownerType,
				seen,
				inspectContracts,
			) {
				return true
			}
		}
	case *types.Interface:
		if !inspectContracts {
			return false
		}
		current.Complete()
		for embedded := range current.EmbeddedTypes() {
			if sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
				embedded,
				ownerType,
				seen,
				inspectContracts,
			) {
				return true
			}
		}
		for method := range current.Methods() {
			if sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
				method.Type(),
				ownerType,
				seen,
				inspectContracts,
			) {
				return true
			}
		}
	case *types.Union:
		if !inspectContracts {
			return false
		}
		for term := range current.Terms() {
			if sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
				term.Type(),
				ownerType,
				seen,
				inspectContracts,
			) {
				return true
			}
		}
	case *types.TypeParam:
		// Concrete instantiations are audited through types.Info.Instances. A type
		// parameter conversion is checked separately against its completed constraint.
		return false
	}
	return false
}

func sourceConstructionTypeArgumentsContainExactOwner(
	arguments *types.TypeList,
	ownerType *types.Named,
) bool {
	return sourceConstructionTypeArgumentsContainConcreteExactOwnerWithSeen(
		arguments,
		ownerType,
		map[types.Type]bool{},
		true,
	)
}

func sourceConstructionTypeArgumentsContainConcreteExactOwnerWithSeen(
	arguments *types.TypeList,
	ownerType *types.Named,
	seen map[types.Type]bool,
	inspectContracts bool,
) bool {
	if arguments == nil {
		return false
	}
	for argument := range arguments.Types() {
		if sourceConstructionTypeContainsConcreteExactOwnerWithSeen(
			argument,
			ownerType,
			seen,
			inspectContracts,
		) {
			return true
		}
	}
	return false
}

func sourceConstructionConvertsToOwnerAdmittingTypeParameter(
	info *types.Info,
	call *ast.CallExpr,
	ownerType *types.Named,
) bool {
	if !info.Types[call.Fun].IsType() {
		return false
	}
	typeParameter, ok := types.Unalias(info.TypeOf(call.Fun)).(*types.TypeParam)
	if !ok {
		return false
	}
	constraint, _ := types.Unalias(typeParameter.Constraint()).Underlying().(*types.Interface)
	if constraint == nil {
		return false
	}
	constraint.Complete()
	return types.Satisfies(ownerType, constraint) ||
		types.Satisfies(types.NewPointer(ownerType), constraint)
}

func sourceConstructionOwnerInterfaceErasureViolations(
	typedPackage *packages.Package,
	ownerType *types.Named,
	violationLabel string,
) []string {
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		parents := sourceConstructionParentNodes(file)
		record := func(node ast.Node, edge string) {
			filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
			function := sourceConstructionEnclosingFunction(file, node.Pos())
			functionRole := "<outside-function>"
			if function != nil {
				functionRole = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			violations = append(violations,
				typedPackage.Fset.Position(node.Pos()).String()+" erases source "+
					violationLabel+" owner into interface outside audited boundary "+
					filename+"|"+functionRole+"|"+edge,
			)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.AssignStmt:
				targets := sourceConstructionExpressionTypes(
					typedPackage.TypesInfo,
					current.Lhs,
				)
				sourceConstructionRecordInterfaceTransfers(
					typedPackage.TypesInfo,
					current.Rhs,
					targets,
					ownerType,
					"assignment",
					record,
				)
			case *ast.RangeStmt:
				if sourceConstructionIsTypeParameter(
					typedPackage.TypesInfo.TypeOf(current.X),
				) {
					sourceConstructionRecordTypeParameterRangeTransfers(
						typedPackage.TypesInfo,
						current,
						ownerType,
						record,
					)
					return true
				}
				keyType, valueType := sourceConstructionRangeTypes(
					typedPackage.TypesInfo.TypeOf(current.X),
				)
				if current.Key != nil {
					sourceConstructionRecordInterfaceTransfer(
						keyType,
						typedPackage.TypesInfo.TypeOf(current.Key),
						ownerType,
						current.Key,
						"range-key",
						record,
					)
				}
				if current.Value != nil {
					sourceConstructionRecordInterfaceTransfer(
						valueType,
						typedPackage.TypesInfo.TypeOf(current.Value),
						ownerType,
						current.Value,
						"range-value",
						record,
					)
				}
			case *ast.ValueSpec:
				targets := make([]types.Type, 0, len(current.Names))
				for _, name := range current.Names {
					targets = append(targets, typedPackage.TypesInfo.TypeOf(name))
				}
				sourceConstructionRecordInterfaceTransfers(
					typedPackage.TypesInfo,
					current.Values,
					targets,
					ownerType,
					"value-spec",
					record,
				)
			case *ast.ReturnStmt:
				signature := sourceConstructionEnclosingSignature(
					typedPackage.TypesInfo,
					current,
					parents,
				)
				if signature == nil {
					return true
				}
				targets := sourceConstructionTupleTypes(signature.Results())
				sourceConstructionRecordInterfaceTransfers(
					typedPackage.TypesInfo,
					current.Results,
					targets,
					ownerType,
					"return",
					record,
				)
			case *ast.CallExpr:
				if typedPackage.TypesInfo.Types[current.Fun].IsType() {
					sourceConstructionRecordInterfaceTransfers(
						typedPackage.TypesInfo,
						current.Args,
						[]types.Type{typedPackage.TypesInfo.TypeOf(current)},
						ownerType,
						"conversion",
						record,
					)
					return true
				}
				functionType := typedPackage.TypesInfo.TypeOf(current.Fun)
				if sourceConstructionIsTypeParameter(functionType) {
					sourceConstructionRecordTypeParameterOperands(
						typedPackage.TypesInfo,
						current.Args,
						ownerType,
						"call-argument:"+sourceConstructionCallObjectRole(
							sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun),
							typedPackage.Types.Path(),
						),
						record,
					)
					return true
				}
				signature := sourceConstructionSignatureOf(functionType)
				if signature == nil {
					return true
				}
				targets := sourceConstructionCallArgumentTypes(signature, current)
				sourceConstructionRecordInterfaceTransfers(
					typedPackage.TypesInfo,
					current.Args,
					targets,
					ownerType,
					"call-argument:"+sourceConstructionCallObjectRole(
						sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun),
						typedPackage.Types.Path(),
					),
					record,
				)
			case *ast.CompositeLit:
				if sourceConstructionIsTypeParameter(
					typedPackage.TypesInfo.TypeOf(current),
				) {
					sourceConstructionRecordTypeParameterCompositeOperands(
						typedPackage.TypesInfo,
						current,
						ownerType,
						record,
					)
					return true
				}
				sourceConstructionRecordCompositeInterfaceTransfers(
					typedPackage.TypesInfo,
					current,
					ownerType,
					record,
				)
			case *ast.SendStmt:
				channelType := typedPackage.TypesInfo.TypeOf(current.Chan)
				if sourceConstructionIsTypeParameter(channelType) {
					sourceConstructionRecordTypeParameterOperands(
						typedPackage.TypesInfo,
						[]ast.Expr{current.Value},
						ownerType,
						"channel-send",
						record,
					)
					return true
				}
				channel, _ := sourceConstructionConcreteUnderlying(channelType).(*types.Chan)
				if channel == nil {
					return true
				}
				sourceConstructionRecordInterfaceTransfers(
					typedPackage.TypesInfo,
					[]ast.Expr{current.Value},
					[]types.Type{channel.Elem()},
					ownerType,
					"channel-send",
					record,
				)
			case *ast.IndexExpr:
				sourceConstructionRecordMapKeyInterfaceErasure(
					typedPackage.TypesInfo,
					current,
					ownerType,
					record,
				)
			}
			return true
		})
	}
	return violations
}

func sourceConstructionExpressionTypes(info *types.Info, expressions []ast.Expr) []types.Type {
	result := make([]types.Type, 0, len(expressions))
	for _, expression := range expressions {
		result = append(result, info.TypeOf(expression))
	}
	return result
}

func sourceConstructionTupleTypes(tuple *types.Tuple) []types.Type {
	if tuple == nil {
		return nil
	}
	result := make([]types.Type, 0, tuple.Len())
	for variable := range tuple.Variables() {
		result = append(result, variable.Type())
	}
	return result
}

func sourceConstructionRecordInterfaceTransfers(
	info *types.Info,
	values []ast.Expr,
	targets []types.Type,
	ownerType *types.Named,
	edge string,
	record func(ast.Node, string),
) {
	if len(values) == len(targets) {
		for index, value := range values {
			sourceConstructionRecordInterfaceTransfer(
				info.TypeOf(value),
				targets[index],
				ownerType,
				value,
				edge,
				record,
			)
		}
		return
	}
	if len(values) != 1 {
		return
	}
	if typed, ok := info.Types[values[0]]; ok && typed.HasOk() && len(targets) == 2 {
		sourceConstructionRecordInterfaceTransfer(
			info.TypeOf(values[0]),
			targets[0],
			ownerType,
			values[0],
			edge+"-comma-ok",
			record,
		)
		sourceConstructionRecordInterfaceTransfer(
			types.Typ[types.Bool],
			targets[1],
			ownerType,
			values[0],
			edge+"-comma-ok",
			record,
		)
		return
	}
	tuple, _ := types.Unalias(info.TypeOf(values[0])).(*types.Tuple)
	if tuple == nil || tuple.Len() != len(targets) {
		return
	}
	for index := range targets {
		sourceConstructionRecordInterfaceTransfer(
			tuple.At(index).Type(),
			targets[index],
			ownerType,
			values[0],
			edge,
			record,
		)
	}
}

func sourceConstructionRecordInterfaceTransfer(
	source types.Type,
	target types.Type,
	ownerType *types.Named,
	node ast.Node,
	edge string,
	record func(ast.Node, string),
) {
	if !sourceConstructionValueContainsConcreteExactOwner(source, ownerType) ||
		!sourceConstructionIsInterfaceType(target) {
		return
	}
	record(node, edge)
}

func sourceConstructionIsInterfaceType(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := types.Unalias(value).Underlying().(*types.Interface)
	return ok
}

func sourceConstructionIsTypeParameter(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := types.Unalias(value).(*types.TypeParam)
	return ok
}

func sourceConstructionConcreteUnderlying(value types.Type) types.Type {
	if value == nil || sourceConstructionIsTypeParameter(value) {
		return nil
	}
	return types.Unalias(value).Underlying()
}

func sourceConstructionRecordTypeParameterOperands(
	info *types.Info,
	values []ast.Expr,
	ownerType *types.Named,
	edge string,
	record func(ast.Node, string),
) {
	for _, value := range values {
		if sourceConstructionValueContainsConcreteExactOwner(info.TypeOf(value), ownerType) {
			record(value, edge)
		}
	}
}

func sourceConstructionRecordTypeParameterCompositeOperands(
	info *types.Info,
	literal *ast.CompositeLit,
	ownerType *types.Named,
	record func(ast.Node, string),
) {
	values := make([]ast.Expr, 0, len(literal.Elts)*2)
	for _, element := range literal.Elts {
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			values = append(values, keyed.Key, keyed.Value)
			continue
		}
		values = append(values, element)
	}
	sourceConstructionRecordTypeParameterOperands(
		info,
		values,
		ownerType,
		"composite-element",
		record,
	)
}

func sourceConstructionRecordTypeParameterRangeTransfers(
	info *types.Info,
	statement *ast.RangeStmt,
	ownerType *types.Named,
	record func(ast.Node, string),
) {
	typeParameter, _ := types.Unalias(info.TypeOf(statement.X)).(*types.TypeParam)
	if typeParameter == nil || !sourceConstructionStructuralTermsContainExactOwner(
		typeParameter.Constraint(),
		ownerType,
		map[types.Type]bool{},
	) {
		return
	}
	for _, transfer := range []struct {
		target ast.Expr
		edge   string
	}{
		{target: statement.Key, edge: "range-key"},
		{target: statement.Value, edge: "range-value"},
	} {
		if transfer.target != nil && sourceConstructionIsInterfaceType(info.TypeOf(transfer.target)) {
			record(transfer.target, transfer.edge)
		}
	}
}

func sourceConstructionStructuralTermsContainExactOwner(
	value types.Type,
	ownerType *types.Named,
	seen map[types.Type]bool,
) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	switch current := value.(type) {
	case *types.TypeParam:
		return sourceConstructionStructuralTermsContainExactOwner(
			current.Constraint(),
			ownerType,
			seen,
		)
	case *types.Named:
		if _, isInterface := current.Underlying().(*types.Interface); isInterface {
			return sourceConstructionStructuralTermsContainExactOwner(
				current.Underlying(),
				ownerType,
				seen,
			)
		}
		return sourceConstructionValueContainsConcreteExactOwner(current, ownerType)
	case *types.Interface:
		current.Complete()
		for embedded := range current.EmbeddedTypes() {
			if sourceConstructionStructuralTermsContainExactOwner(embedded, ownerType, seen) {
				return true
			}
		}
	case *types.Union:
		for term := range current.Terms() {
			if sourceConstructionStructuralTermsContainExactOwner(term.Type(), ownerType, seen) {
				return true
			}
		}
	default:
		return sourceConstructionValueContainsConcreteExactOwner(current, ownerType)
	}
	return false
}

func sourceConstructionSignatureOf(value types.Type) *types.Signature {
	signature, _ := sourceConstructionConcreteUnderlying(value).(*types.Signature)
	return signature
}

func sourceConstructionEnclosingSignature(
	info *types.Info,
	node ast.Node,
	parents map[ast.Node]ast.Node,
) *types.Signature {
	for current := ast.Node(node); current != nil; current = parents[current] {
		switch enclosing := current.(type) {
		case *ast.FuncLit:
			return sourceConstructionSignatureOf(info.TypeOf(enclosing.Type))
		case *ast.FuncDecl:
			return sourceConstructionSignatureOf(info.TypeOf(enclosing.Name))
		}
	}
	return nil
}

func sourceConstructionCallArgumentTypes(
	signature *types.Signature,
	call *ast.CallExpr,
) []types.Type {
	if signature == nil {
		return nil
	}
	targets := make([]types.Type, 0, len(call.Args))
	parameters := signature.Params()
	for index := range call.Args {
		if index >= parameters.Len() && !signature.Variadic() {
			break
		}
		parameterIndex := index
		if signature.Variadic() && parameterIndex >= parameters.Len()-1 {
			parameterIndex = parameters.Len() - 1
		}
		target := parameters.At(parameterIndex).Type()
		if signature.Variadic() && parameterIndex == parameters.Len()-1 &&
			call.Ellipsis == token.NoPos {
			variadic, _ := types.Unalias(target).Underlying().(*types.Slice)
			if variadic != nil {
				target = variadic.Elem()
			}
		}
		targets = append(targets, target)
	}
	return targets
}

func sourceConstructionRecordCompositeInterfaceTransfers(
	info *types.Info,
	literal *ast.CompositeLit,
	ownerType *types.Named,
	record func(ast.Node, string),
) {
	valueType := info.TypeOf(literal)
	if valueType == nil {
		return
	}
	switch structure := sourceConstructionConcreteUnderlying(valueType).(type) {
	case *types.Struct:
		unkeyedIndex := 0
		for _, element := range literal.Elts {
			value := element
			var target types.Type
			if keyed, ok := element.(*ast.KeyValueExpr); ok {
				value = keyed.Value
				if field, ok := info.Uses[keyed.Key.(*ast.Ident)].(*types.Var); ok {
					target = field.Type()
				}
			} else if unkeyedIndex < structure.NumFields() {
				target = structure.Field(unkeyedIndex).Type()
				unkeyedIndex++
			}
			sourceConstructionRecordInterfaceTransfer(
				info.TypeOf(value),
				target,
				ownerType,
				value,
				"composite-field",
				record,
			)
		}
	case *types.Array:
		sourceConstructionRecordSequenceInterfaceTransfers(
			info,
			literal,
			structure.Elem(),
			ownerType,
			record,
		)
	case *types.Slice:
		sourceConstructionRecordSequenceInterfaceTransfers(
			info,
			literal,
			structure.Elem(),
			ownerType,
			record,
		)
	case *types.Map:
		for _, element := range literal.Elts {
			keyed, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			sourceConstructionRecordInterfaceTransfer(
				info.TypeOf(keyed.Key),
				structure.Key(),
				ownerType,
				keyed.Key,
				"composite-map-key",
				record,
			)
			sourceConstructionRecordInterfaceTransfer(
				info.TypeOf(keyed.Value),
				structure.Elem(),
				ownerType,
				keyed.Value,
				"composite-map-value",
				record,
			)
		}
	}
}

func sourceConstructionRecordSequenceInterfaceTransfers(
	info *types.Info,
	literal *ast.CompositeLit,
	target types.Type,
	ownerType *types.Named,
	record func(ast.Node, string),
) {
	for _, element := range literal.Elts {
		value := element
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			value = keyed.Value
		}
		sourceConstructionRecordInterfaceTransfer(
			info.TypeOf(value),
			target,
			ownerType,
			value,
			"composite-element",
			record,
		)
	}
}

func sourceConstructionRecordMapKeyInterfaceErasure(
	info *types.Info,
	target ast.Expr,
	ownerType *types.Named,
	record func(ast.Node, string),
) {
	for {
		parenthesized, ok := target.(*ast.ParenExpr)
		if !ok {
			break
		}
		target = parenthesized.X
	}
	index, ok := target.(*ast.IndexExpr)
	if !ok {
		return
	}
	container := info.TypeOf(index.X)
	if container == nil {
		return
	}
	if sourceConstructionIsTypeParameter(container) {
		if sourceConstructionValueContainsConcreteExactOwner(
			info.TypeOf(index.Index),
			ownerType,
		) {
			record(index.Index, "map-index-key")
		}
		return
	}
	mapping, _ := sourceConstructionConcreteUnderlying(container).(*types.Map)
	if mapping == nil {
		return
	}
	sourceConstructionRecordInterfaceTransfer(
		info.TypeOf(index.Index),
		mapping.Key(),
		ownerType,
		index.Index,
		"map-index-key",
		record,
	)
}

func sourceConstructionRangeTypes(value types.Type) (types.Type, types.Type) {
	value = sourceConstructionConcreteUnderlying(value)
	if value == nil {
		return nil, nil
	}
	if pointer, ok := value.(*types.Pointer); ok {
		if _, array := sourceConstructionConcreteUnderlying(pointer.Elem()).(*types.Array); array {
			value = sourceConstructionConcreteUnderlying(pointer.Elem())
		}
	}
	switch ranged := value.(type) {
	case *types.Array:
		return types.Typ[types.Int], ranged.Elem()
	case *types.Slice:
		return types.Typ[types.Int], ranged.Elem()
	case *types.Map:
		return ranged.Key(), ranged.Elem()
	case *types.Chan:
		return ranged.Elem(), nil
	case *types.Signature:
		if ranged.Params().Len() != 1 {
			return nil, nil
		}
		yield := sourceConstructionSignatureOf(ranged.Params().At(0).Type())
		if yield == nil || yield.Params().Len() == 0 {
			return nil, nil
		}
		key := yield.Params().At(0).Type()
		if yield.Params().Len() == 1 {
			return key, nil
		}
		return key, yield.Params().At(1).Type()
	}
	return nil, nil
}

func sourceConstructionDirectCallExpression(expression ast.Expr) (*ast.CallExpr, bool) {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			call, direct := expression.(*ast.CallExpr)
			return call, direct
		}
		expression = parenthesized.X
	}
}

func sourceConstructionParentNodes(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	stack := make([]ast.Node, 0)
	ast.Inspect(file, func(node ast.Node) bool {
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

func sourceConstructionDescriptorFileUse(
	typedPackage *packages.Package,
	field *ast.SelectorExpr,
	parents map[ast.Node]ast.Node,
) string {
	parent := parents[field]
	switch current := parent.(type) {
	case *ast.BinaryExpr:
		return "compare:" + current.Op.String()
	case *ast.AssignStmt:
		for _, target := range current.Lhs {
			if sourceConstructionDirectExpression(target, field) {
				return "assign-left:" + current.Tok.String()
			}
		}
		return "assign-right:" + current.Tok.String()
	case *ast.CallExpr:
		return "argument:" + sourceConstructionCallObjectRole(
			sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun),
			typedPackage.Types.Path(),
		)
	case *ast.SelectorExpr:
		if current.X != field {
			return "selector:<indirect>"
		}
		call, ok := parents[current].(*ast.CallExpr)
		if !ok || !sourceConstructionDirectExpression(call.Fun, current) {
			return "method-value:" + current.Sel.Name
		}
		if current.Sel.Name == "Fd" {
			return "fd:" + sourceConstructionDirectValueSink(
				typedPackage,
				call,
				parents,
			)
		}
		return "method:" + current.Sel.Name
	default:
		return "<unclassified>"
	}
}

func sourceConstructionDirectValueSink(
	typedPackage *packages.Package,
	start ast.Expr,
	parents map[ast.Node]ast.Node,
) string {
	current := start
	conversions := make([]string, 0)
	for {
		parent := parents[current]
		switch next := parent.(type) {
		case *ast.ParenExpr:
			if next.X != current {
				return "<escaped>"
			}
			current = next
		case *ast.CallExpr:
			argumentIndex := -1
			for index, argument := range next.Args {
				if sourceConstructionDirectExpression(argument, current) {
					argumentIndex = index
					break
				}
			}
			if argumentIndex < 0 {
				return "<escaped>"
			}
			called := sourceConstructionCalledObject(typedPackage.TypesInfo, next.Fun)
			if _, conversion := called.(*types.TypeName); conversion {
				if argumentIndex != 0 || len(next.Args) != 1 {
					return "<invalid-conversion>"
				}
				conversions = append(
					conversions,
					sourceConstructionConversionRole(called, typedPackage.Types.Path()),
				)
				current = next
				continue
			}
			sink := sourceConstructionCallObjectRole(called, typedPackage.Types.Path()) +
				"[arg" + strconv.Itoa(argumentIndex)
			if len(conversions) != 0 {
				sink += ":" + strings.Join(conversions, ">")
			}
			return sink + "]" + sourceConstructionCallBoundary(next, parents)
		default:
			return "<escaped>"
		}
	}
}

func sourceConstructionConversionRole(object types.Object, packagePath string) string {
	if object == nil {
		return "<unresolved>"
	}
	if object.Pkg() == nil || object.Pkg().Path() == packagePath {
		return object.Name()
	}
	return object.Pkg().Path() + "." + object.Name()
}

func sourceConstructionCallBoundary(
	call *ast.CallExpr,
	parents map[ast.Node]ast.Node,
) string {
	for current := ast.Node(call); current != nil; current = parents[current] {
		switch parents[current].(type) {
		case *ast.GoStmt:
			return "@go"
		case *ast.DeferStmt:
			return "@defer"
		case *ast.FuncLit:
			return "@func-literal"
		case *ast.FuncDecl:
			return ""
		}
	}
	return "@outside-function"
}

func sourceConstructionCallObjectRole(object types.Object, packagePath string) string {
	if object == nil {
		return "<unresolved>"
	}
	if object.Pkg() == nil {
		return object.Name()
	}
	if object.Pkg().Path() == packagePath {
		packageObject := object.Pkg().Scope().Lookup(object.Name())
		if object != packageObject {
			function, ok := object.(*types.Func)
			if ok {
				signature, _ := function.Type().(*types.Signature)
				if signature != nil && signature.Recv() != nil {
					return sourceConstructionFunctionRole(function, packagePath)
				}
			}
			return "<non-package>:" + object.Name()
		}
		if _, function := object.(*types.Func); !function {
			return "<non-function>:" + object.Name()
		}
		return object.Name()
	}
	if _, function := object.(*types.Func); !function {
		return "<non-function>:" + object.Pkg().Path() + "." + object.Name()
	}
	externalPath := object.Pkg().Path()
	if externalPath == "golang.org/x/sys/unix" {
		// Keep the compact display role only after authenticating the exact import path.
		externalPath = "unix"
	}
	return externalPath + "." + object.Name()
}

func sourceConstructionDescriptorFileOriginViolations(
	typedPackage *packages.Package,
) []string {
	allowed := map[string]int{
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|os.NewFile|composite:ownedSourceDescriptor.file": 1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|os.NewFile|composite:ownedSourceDescriptor.file":                  1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|os.Open|=:ownedSourceDescriptor.file,err":       1,
	}
	observed, violations := sourceConstructionRawCapabilityOriginSites(
		typedPackage,
		allowed,
		sourceConstructionDirectOSFileResult,
		"descriptor file",
	)
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited raw descriptor file origin "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	return violations
}

func sourceConstructionDirectOSFileResult(value types.Type) bool {
	value = types.Unalias(value)
	if tuple, ok := value.(*types.Tuple); ok {
		for variable := range tuple.Variables() {
			if sourceConstructionDirectOSFileResult(variable.Type()) {
				return true
			}
		}
		return false
	}
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "os" &&
		named.Obj().Name() == "File"
}

func sourceConstructionRootOriginViolations(
	typedPackage *packages.Package,
) []string {
	allowed := map[string]int{
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|os.OpenRoot|=:ownedSourceRoot.root,err": 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|os.OpenRoot|=:ownedSourceRoot.root,err":      1,
	}
	observed, violations := sourceConstructionRawCapabilityOriginSites(
		typedPackage,
		allowed,
		sourceConstructionDirectOSRootResult,
		"source root",
	)
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited raw source root origin "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	return violations
}

func sourceConstructionUnsupportedPrimitiveViolations(source []byte) []string {
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(
		files,
		"source_primitives_unsupported.go",
		source,
		parser.ParseComments|parser.AllErrors,
	)
	if err != nil || parsed == nil {
		return []string{"unsupported source primitive seam does not parse: " + fmt.Sprint(err)}
	}
	violations := make([]string, 0)
	buildConstraints := make([]string, 0, 1)
	buildConstraintBeforePackage := false
	for _, group := range parsed.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:build") {
				buildConstraints = append(buildConstraints, comment.Text)
				buildConstraintBeforePackage = comment.Pos() < parsed.Package
			}
		}
	}
	switch {
	case len(buildConstraints) != 1:
		violations = append(violations,
			"unsupported source primitive seam must have exactly one go:build constraint",
		)
	case !buildConstraintBeforePackage:
		violations = append(violations,
			"unsupported source primitive go:build constraint must precede the package clause",
		)
	default:
		expression, constraintErr := constraint.Parse(buildConstraints[0])
		if constraintErr != nil {
			violations = append(violations,
				"unsupported source primitive build constraint does not parse: "+constraintErr.Error(),
			)
		} else if !sourceConstructionUnsupportedConstraintIsExactComplement(expression) {
			violations = append(violations,
				"unsupported source primitive build constraint is not the exact complement of darwin and arm64",
			)
		}
	}
	if parsed.Name == nil || parsed.Name.Name != "buildauthority" {
		violations = append(violations,
			"unsupported source primitive seam must remain in package buildauthority",
		)
	}
	if len(parsed.Imports) != 0 {
		violations = append(violations, "unsupported source primitive seam must contain no imports")
	}
	if len(parsed.Decls) != 1 {
		violations = append(violations,
			"unsupported source primitive seam must contain exactly one declaration",
		)
		return violations
	}
	function, ok := parsed.Decls[0].(*ast.FuncDecl)
	if !ok || function.Name == nil || function.Name.Name != "platformSourcePrimitives" ||
		function.Recv != nil || function.Type == nil ||
		(function.Type.TypeParams != nil && len(function.Type.TypeParams.List) != 0) ||
		function.Type.Params == nil || len(function.Type.Params.List) != 0 ||
		function.Type.Results == nil || len(function.Type.Results.List) != 2 ||
		types.ExprString(function.Type.Results.List[0].Type) != "sourcePrimitives" ||
		types.ExprString(function.Type.Results.List[1].Type) != "*sourcePrimitiveFailure" {
		violations = append(violations,
			"unsupported source primitive seam must preserve the exact factory signature",
		)
		return violations
	}
	if function.Body == nil || len(function.Body.List) != 1 {
		violations = append(violations,
			"unsupported source primitive factory must contain one return statement",
		)
		return violations
	}
	returned, ok := function.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 ||
		!sourceConstructionBareNilExpression(returned.Results[0]) {
		violations = append(violations,
			"unsupported source primitive factory must return the exact unsupported failure",
		)
		return violations
	}
	failure, ok := returned.Results[1].(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(failure) !=
		"newSourcePrimitiveFailure(OperationValidate,CauseUnsupported)" {
		violations = append(violations,
			"unsupported source primitive factory must return the exact unsupported failure",
		)
	}
	return violations
}

func sourceConstructionUnsupportedConstraintIsExactComplement(expression constraint.Expr) bool {
	if expression == nil {
		return false
	}
	tags := make(map[string]bool)
	var collect func(constraint.Expr)
	collect = func(current constraint.Expr) {
		switch value := current.(type) {
		case *constraint.TagExpr:
			tags[value.Tag] = true
		case *constraint.NotExpr:
			collect(value.X)
		case *constraint.AndExpr:
			collect(value.X)
			collect(value.Y)
		case *constraint.OrExpr:
			collect(value.X)
			collect(value.Y)
		}
	}
	collect(expression)
	if len(tags) != 2 || !tags["darwin"] || !tags["arm64"] {
		return false
	}
	for _, darwin := range []bool{false, true} {
		for _, arm64 := range []bool{false, true} {
			got := expression.Eval(func(tag string) bool {
				switch tag {
				case "darwin":
					return darwin
				case "arm64":
					return arm64
				default:
					return false
				}
			})
			if got == (darwin && arm64) {
				return false
			}
		}
	}
	return true
}

func sourceConstructionResolvedCallClosureViolations(
	typedPackage *packages.Package,
) []string {
	moduleFiles := map[string]bool{
		"source_construction.go":           true,
		"source_construction_acquire.go":   true,
		"source_construction_inventory.go": true,
		"source_primitives.go":             true,
		"source_primitives_darwin.go":      true,
	}
	allowedExternal := map[string]bool{
		"bytes|(*bytes.Buffer).Bytes":     true,
		"bytes|(*bytes.Buffer).Write":     true,
		"bytes|(*bytes.Buffer).WriteByte": true,
		"bytes|Compare":                   true,
		"bytes|HasPrefix":                 true,
		"bytes|IndexByte":                 true,
		"bytes|LastIndexByte":             true,
		"bytes|NewBuffer":                 true,
		"bytes|Split":                     true,
		"crypto/sha256|Sum256":            true,
		"encoding/binary|(encoding/binary.bigEndian).PutUint32": true,
		"encoding/binary|(encoding/binary.bigEndian).PutUint64": true,
		"encoding/binary|(encoding/binary.littleEndian).Uint32": true,
		"errors|New":                true,
		"os|Geteuid":                true,
		"path/filepath|Clean":       true,
		"path/filepath|IsAbs":       true,
		"runtime|KeepAlive":         true,
		"slices|Contains":           true,
		"strings|Contains":          true,
		"strings|ContainsRune":      true,
		"strings|HasPrefix":         true,
		"strings|HasSuffix":         true,
		"strings|Index":             true,
		"strings|IndexByte":         true,
		"strings|Split":             true,
		"strings|SplitSeq":          true,
		"strings|Trim":              true,
		"strings|TrimLeft":          true,
		"strings|TrimPrefix":        true,
		"strings|TrimSuffix":        true,
		"sync|(*sync.Mutex).Lock":   true,
		"sync|(*sync.Mutex).Unlock": true,
		"time|(time.Time).Add":      true,
		"time|(time.Time).After":    true,
		"time|(time.Time).Before":   true,
		"time|(time.Time).Equal":    true,
		"time|(time.Time).IsZero":   true,
	}
	allowedBuiltins := map[string]bool{
		"append":  true,
		"cap":     true,
		"complex": true,
		"copy":    true,
		"imag":    true,
		"len":     true,
		"make":    true,
		"max":     true,
		"min":     true,
		"new":     true,
		"real":    true,
	}
	resolvedExternalInterfaceRoles := map[string]bool{
		"<builtin>|(error).Error":            true,
		"context|(context.Context).Deadline": true,
		"context|(context.Context).Done":     true,
		"context|(context.Context).Err":      true,
		"context|(context.Context).Value":    true,
		"io/fs|(io/fs.DirEntry).Name":        true,
	}
	resolvedLocalInterfaceSites := map[string]int{
		"preflight_runner.go|(*nonSourceAuthorityContext).Err|github.com/vbonnet/dear-agent/internal/buildauthority|(processScheduler).now": 1,
	}
	callbackClosureRoots := sourceConstructionAllowedImplicitCallbackMethods()
	forbiddenLocalWriterBoundaries := map[string]bool{
		"writeLengthPrefixed": true,
		"writeManifestRecord": true,
		"writeSnapshotClaim":  true,
		"writeUint32":         true,
		"writeUint64":         true,
	}
	sensitiveLocalCallables := map[string]bool{
		"darwinOpenFlags":                     true,
		"normalizeDarwinSourceDirectoryBatch": true,
		"openDarwinSourceRelativeDescriptor":  true,
		"requireDescriptorKind":               true,
	}
	allowedSensitiveLocalSites := map[string]int{
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|github.com/vbonnet/dear-agent/internal/buildauthority|darwinOpenFlags|darwinOpenFlags(entryDirectory,true)":                                                      1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRelativeNoFollow|github.com/vbonnet/dear-agent/internal/buildauthority|darwinOpenFlags|darwinOpenFlags(openKind,openKind != entrySymlink)":                                              1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRelativeNoFollow|github.com/vbonnet/dear-agent/internal/buildauthority|openDarwinSourceRelativeDescriptor|openDarwinSourceRelativeDescriptor(ctx,parent,name,kind,openKind,mode,flags)": 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|github.com/vbonnet/dear-agent/internal/buildauthority|requireDescriptorKind|requireDescriptorKind(int(owner.file.Fd()),entryDirectory)":                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|github.com/vbonnet/dear-agent/internal/buildauthority|normalizeDarwinSourceDirectoryBatch|normalizeDarwinSourceDirectoryBatch(ctx,entries,err)":                          1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|github.com/vbonnet/dear-agent/internal/buildauthority|requireDescriptorKind|requireDescriptorKind(int(owner.file.Fd()),openKind)":                                                 1,
	}
	allowedSensitiveExternalSites := map[string]int{
		"filesystem_darwin.go|darwinFgetattrlistWithOptions|golang.org/x/sys/unix|Syscall6|unix.Syscall6(darwinSysFgetattrlist,uintptr(fd),uintptr(unsafe.Pointer(attributes)),uintptr(unsafe.Pointer(&buffer[0])),uintptr(len(buffer)),options,0)": 1,
		"filesystem_darwin.go|requireDescriptorKind|golang.org/x/sys/unix|Fstat|unix.Fstat(fd,&stat)":                                                                                      1,
		"gitconfig.go|parseConfigQuoted|fmt|Errorf|fmt.Errorf(\"quoted value is missing opening quote\")":                                                                                  1,
		"gitconfig.go|parseConfigQuoted|fmt|Errorf|fmt.Errorf(\"unsupported quoted escape\")":                                                                                              1,
		"gitconfig.go|parseConfigQuoted|fmt|Errorf|fmt.Errorf(\"unterminated quoted escape\")":                                                                                             1,
		"gitconfig.go|parseConfigQuoted|fmt|Errorf|fmt.Errorf(\"unterminated quoted value\")":                                                                                              1,
		"gitconfig.go|parseSourceConfigAssignment|fmt|Errorf|fmt.Errorf(\"assignment requires an explicit value\")":                                                                        1,
		"gitconfig.go|parseSourceConfigAssignment|fmt|Errorf|fmt.Errorf(\"invalid variable name\")":                                                                                        1,
		"gitconfig.go|parseSourceConfigSection|fmt|Errorf|fmt.Errorf(\"unexpected bytes after section header\")":                                                                           1,
		"gitconfig.go|parseSourceConfigSectionBody|fmt|Errorf|fmt.Errorf(\"empty section\")":                                                                                               1,
		"gitconfig.go|parseSourceConfigSectionBody|fmt|Errorf|fmt.Errorf(\"invalid section name\")":                                                                                        1,
		"gitconfig.go|parseSourceConfigSectionBody|fmt|Errorf|fmt.Errorf(\"invalid section separator\")":                                                                                   1,
		"gitconfig.go|parseSourceConfigSectionBody|fmt|Errorf|fmt.Errorf(\"subsection contains a control byte\")":                                                                          1,
		"gitconfig.go|parseSourceConfigSectionBody|fmt|Errorf|fmt.Errorf(\"subsection must be quoted\")":                                                                                   1,
		"gitconfig.go|parseSourceConfigSectionBody|fmt|Errorf|fmt.Errorf(\"unexpected bytes after subsection\")":                                                                           1,
		"gitconfig.go|parseSourceConfigSyntax|fmt|Sprintf|fmt.Sprintf(\"source config assignment on line %d\",lineNumber + 1)":                                                             1,
		"gitconfig.go|parseSourceConfigSyntax|fmt|Sprintf|fmt.Sprintf(\"source config line %d\",lineNumber + 1)":                                                                           1,
		"gitconfig.go|parseSourceConfigSyntax|fmt|Sprintf|fmt.Sprintf(\"source config section on line %d\",lineNumber + 1)":                                                                1,
		"gitconfig.go|parseSourceConfigValue|fmt|Errorf|fmt.Errorf(\"unterminated quoted value\")":                                                                                         1,
		"gitconfig.go|parseSourceConfigValue|fmt|Errorf|fmt.Errorf(\"value contains a control byte\")":                                                                                     1,
		"gitconfig.go|sourceConfigEscapedByte|fmt|Errorf|fmt.Errorf(\"unsupported or control-producing value escape\")":                                                                    1,
		"gitconfig.go|sourceConfigEscapedByte|fmt|Errorf|fmt.Errorf(\"unterminated value escape\")":                                                                                        1,
		"gitconfig.go|sourceConfigSectionClosing|fmt|Errorf|fmt.Errorf(\"unterminated section header\")":                                                                                   1,
		"gitconfig.go|validateSourceConfigLineBytes|fmt|Errorf|fmt.Errorf(\"forbidden control byte 0x%02x\",value)":                                                                        1,
		"gitconfig.go|validFullGitRefName|slices|ContainsFunc|slices.ContainsFunc([]byte(name),invalidGitRefByte)":                                                                         1,
		"private_errors.go|failWith|fmt|Errorf|fmt.Errorf(\"%s: %w\",message,err)":                                                                                                         1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Deadline|context|(context.Context).Deadline|ctx.transaction.ctx.Deadline()":                                                      1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Done|context|(context.Context).Done|ctx.transaction.ctx.Done()":                                                                  1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Err|context|(context.Context).Deadline|transaction.ctx.Deadline()":                                                               1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Err|context|(context.Context).Err|transaction.ctx.Err()":                                                                         1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Err|errors|Is|errors.Is(callerError,context.Canceled)":                                                                           1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Value|context|(context.Context).Value|ctx.transaction.ctx.Value(key)":                                                            1,
		"preflight_runner.go|nonSourceDeadlineReached|errors|Is|errors.Is(contextError,context.DeadlineExceeded)":                                                                          1,
		"private_errors.go|(*privateFailure).Error|<builtin>|(error).Error|failure.raw.Error()":                                                                                            1,
		"process.go|(realProcessScheduler).now|time|Now|time.Now()":                                                                                                                        1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).finishSourceAdministrativeInventory|sort|Slice|sort.Slice(capture.candidates,(func(left, right int) bool literal))": 1,
		"source_construction_inventory.go|sourceAdministrativeCandidateIndex|sort|Search|sort.Search(len(candidates),(func(index int) bool literal))":                                      1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).acquireRawACL|os|(*os.File).Fd|owner.file.Fd()":                                                                              1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|os|(*os.File).Stat|descriptor.file.Stat()":                                                          1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|os|(*os.Root).Lstat|root.root.Lstat(\".\")":                                                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).compareRootAndDescriptor|os|SameFile|os.SameFile(rootInfo,descriptorInfo)":                                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openChildRoot|os|(*os.Root).OpenRoot|parent.root.OpenRoot(name)":                                                             1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|golang.org/x/sys/unix|Close|unix.Close(fd)":                                                       1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|golang.org/x/sys/unix|Open|unix.Open(physicalRootPath,flags,0)":                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|os|NewFile|os.NewFile(uintptr(fd),physicalRootPath)":                                              1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRepositoryRoot|os|OpenRoot|os.OpenRoot(locator.path)":                                                                    1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|os|(*os.File).Fd|owner.file.Fd()":                                                                1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|os|(*os.Root).Open|root.root.Open(\".\")":                                                        1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|golang.org/x/sys/unix|Fstatat|unix.Fstatat(int(parent.file.Fd()),name,&stat,unix.AT_SYMLINK_NOFOLLOW)":     1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).probeRelativeKind|os|(*os.File).Fd|parent.file.Fd()":                                                                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readDirectoryBatch|os|(*os.File).ReadDir|owner.file.ReadDir(sourceDirectoryReadBatchSize)":                                   1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readExactForParse|errors|Is|errors.Is(err,fs.ErrPermission)":                                                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).readExactForParse|os|(*os.File).ReadAt|owner.file.ReadAt(content,0)":                                                         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|golang.org/x/sys/unix|Fstat|unix.Fstat(int(owner.file.Fd()),&stat)":                                           1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statDescriptor|os|(*os.File).Fd|owner.file.Fd()":                                                                             1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statFilesystem|golang.org/x/sys/unix|Fstatfs|unix.Fstatfs(int(owner.file.Fd()),&filesystem)":                                 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).statFilesystem|os|(*os.File).Fd|owner.file.Fd()":                                                                             1,
		"source_primitives_darwin.go|classifyDarwinSourceOpenFailure|errors|Is|errors.Is(err,fs.ErrNotExist)":                                                                              1,
		"source_primitives_darwin.go|classifyDarwinSourceOpenFailure|errors|Is|errors.Is(err,unix.ELOOP)":                                                                                  1,
		"source_primitives_darwin.go|classifyDarwinSourceOpenFailure|errors|Is|errors.Is(err,unix.ENOTDIR)":                                                                                1,
		"source_primitives_darwin.go|classifyDarwinSourcePresenceFailure|errors|Is|errors.Is(err,fs.ErrNotExist)":                                                                          1,
		"source_primitives_darwin.go|classifyDarwinSourcePresenceFailure|errors|Is|errors.Is(err,fs.ErrPermission)":                                                                        1,
		"source_primitives_darwin.go|classifyDarwinSourceWalkFailure|errors|Is|errors.Is(err,context.Canceled)":                                                                            1,
		"source_primitives_darwin.go|classifyDarwinSourceWalkFailure|errors|Is|errors.Is(err,context.DeadlineExceeded)":                                                                    1,
		"source_primitives_darwin.go|classifyDarwinSourceWalkFailure|errors|Is|errors.Is(err,fs.ErrPermission)":                                                                            1,
		"source_primitives_darwin.go|darwinSourceIOCause|errors|Is|errors.Is(err,fs.ErrPermission)":                                                                                        1,
		"source_primitives_darwin.go|darwinSourceIOCause|errors|Is|errors.Is(err,unix.ENOTSUP)":                                                                                            1,
		"source_primitives_darwin.go|darwinSourceIOCause|errors|Is|errors.Is(err,unix.EOPNOTSUPP)":                                                                                         1,
		"source_primitives_darwin.go|normalizeDarwinSourceDirectoryBatch|io/fs|(io/fs.DirEntry).Name|entry.Name()":                                                                         1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|golang.org/x/sys/unix|Close|unix.Close(fd)":                                                                        1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|golang.org/x/sys/unix|Openat|unix.Openat(int(parent.file.Fd()),name,flags,0)":                                      1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|os|(*os.File).Fd|owner.file.Fd()":                                                                                  1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|os|(*os.File).Fd|parent.file.Fd()":                                                                                 1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|os|NewFile|os.NewFile(uintptr(fd),name)":                                                                           1,
		"source_primitives.go|(*ownedSourceDescriptor).closeDirect|os|(*os.File).Close|owner.file.Close()":                                                                                 1,
		"source_primitives.go|(*ownedSourceRoot).closeDirect|os|(*os.Root).Close|owner.root.Close()":                                                                                       1,
		"source_primitives.go|sourceContextPrimitiveFailure|context|(context.Context).Err|ctx.Err()":                                                                                       1,
	}
	allowedBodylessSites := map[string]int{
		"private_errors.go|privateCauses|github.com/vbonnet/dear-agent/internal/buildauthority|" +
			"(interface{Unwrap() []error}).Unwrap": 1,
		"private_errors.go|privateCauses|github.com/vbonnet/dear-agent/internal/buildauthority|" +
			"(interface{Unwrap() error}).Unwrap": 1,
	}
	allowedDynamicSites := map[string]int{
		"private_errors.go|privateCauses|collect": 3,
	}
	allowedFunctionReferenceSites := map[string]int{
		"gitconfig.go|validFullGitRefName|github.com/vbonnet/dear-agent/internal/buildauthority|" +
			"invalidGitRefByte": 1,
	}
	allowedRangeFunctionSites := map[string]int{
		"filesystem.go|absolutePathComponents|strings.SplitSeq(strings.TrimPrefix(path, separator), separator)": 1,
		"filesystem.go|validateRelativeManifestPath|strings.SplitSeq(path, string(filepath.Separator))":         1,
		"gitconfig.go|validFullGitRefName|strings.SplitSeq(name, \"/\")":                                        1,
	}
	allowedPackageVariableReferenceSites := map[string]int{
		"filesystem_darwin.go|entryKindFromPlatformMode|errUnsupportedAuthorityEntry":                                   1,
		"filesystem_darwin.go|requireDescriptorKind|errAuthorityKindMismatch":                                           1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|errAuthorityKindMismatch":     1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openRootDirectoryDescriptor|errUnsupportedAuthorityEntry": 1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|errAuthorityKindMismatch":                       1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|errUnsupportedAuthorityEntry":                   1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Err|errInvalidNonSourceAuthorityContext":                      4,
	}

	declarations := make(map[*types.Func]*ast.FuncDecl)
	declarationFiles := make(map[*types.Func]*ast.File)
	queue := make([]*types.Func, 0)
	observedPackageAssertions := 0
	violations := sourceConstructionPackageInitializationViolations(typedPackage)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		for _, declaration := range file.Decls {
			if general, ok := declaration.(*ast.GenDecl); ok {
				if !moduleFiles[filename] || general.Tok != token.VAR {
					continue
				}
				if sourceConstructionAllowedPackageAssertion(filename, general) {
					observedPackageAssertions++
					continue
				}
				violations = append(violations,
					typedPackage.Fset.Position(general.Pos()).String()+
						" source call closure rejects package variable declaration in "+filename,
				)
				continue
			}
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			object, _ := typedPackage.TypesInfo.Defs[function.Name].(*types.Func)
			if object == nil {
				continue
			}
			declarations[object] = function
			declarationFiles[object] = file
			role := sourceConstructionFunctionRole(object, typedPackage.Types.Path())
			if moduleFiles[filename] || callbackClosureRoots[filename+"|"+role] != 0 {
				queue = append(queue, object)
			}
		}
	}
	violations = append(
		violations,
		sourceConstructionSealedInterfaceImplementerViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionSealedInterfaceValueViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionSealedInterfaceContainerViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionDarwinDirectoryReadTopologyViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionDarwinDirectoryNameFlowViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionPostAcquisitionTopologyViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionRootDescriptorIdentityProvenanceViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionInitialChildValidationViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionOpenFlagProvenanceViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionLocatorProvenanceViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionSealedConcreteReceiverViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionImplicitCallbackViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionErrorInspectionViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionAdministrativePathProvenanceViolations(typedPackage)...,
	)
	violations = append(
		violations,
		sourceConstructionAdministrativeRowMutationViolations(typedPackage)...,
	)

	seen := make(map[*types.Func]bool)
	observedSensitiveExternalSites := make(map[string]int)
	observedSensitiveLocalSites := make(map[string]int)
	observedBodylessSites := make(map[string]int)
	observedDynamicSites := make(map[string]int)
	observedFunctionReferenceSites := make(map[string]int)
	observedRangeFunctionSites := make(map[string]int)
	observedPackageVariableReferenceSites := make(map[string]int)
	observedResolvedLocalInterfaceSites := make(map[string]int)
	for len(queue) != 0 {
		function := queue[0]
		queue = queue[1:]
		if seen[function] {
			continue
		}
		seen[function] = true
		declaration := declarations[function]
		file := declarationFiles[function]
		if declaration == nil || declaration.Body == nil || file == nil {
			violations = append(violations,
				"source call closure has no body for "+
					sourceConstructionCallableRole(function, typedPackage.Types.Path()),
			)
			continue
		}
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		functionRole := sourceConstructionFunctionRole(function, typedPackage.Types.Path())
		parents := sourceConstructionParentNodes(file)
		violations = append(
			violations,
			sourceConstructionReadOnlyParameterViolations(
				typedPackage,
				declaration,
				filename,
				functionRole,
			)...,
		)
		ast.Inspect(declaration.Body, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.RangeStmt:
				typeOf := typedPackage.TypesInfo.TypeOf(current.X)
				if typeOf != nil {
					if _, ok := types.Unalias(typeOf).Underlying().(*types.Signature); ok {
						site := filename + "|" + functionRole + "|" + types.ExprString(current.X)
						observedRangeFunctionSites[site]++
						if allowedRangeFunctionSites[site] == 0 {
							violations = append(violations,
								typedPackage.Fset.Position(current.Pos()).String()+
									" source call closure reaches unaudited range-over-function "+site,
							)
						}
					}
				}
			case *ast.CallExpr:
				if typedPackage.TypesInfo.Types[current.Fun].IsType() {
					return true
				}
				called := sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun)
				switch callable := called.(type) {
				case *types.Builtin:
					if allowedBuiltins[callable.Name()] {
						return true
					}
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source call closure reaches unaudited builtin "+callable.Name(),
					)
					return true
				case *types.TypeName:
					return true
				case *types.Func:
					role := sourceConstructionCallableRole(callable, typedPackage.Types.Path())
					site := filename + "|" + functionRole + "|" + role
					if callable.Pkg() != nil && callable.Pkg().Path() == typedPackage.Types.Path() {
						localRole := sourceConstructionFunctionRole(
							callable,
							typedPackage.Types.Path(),
						)
						if forbiddenLocalWriterBoundaries[localRole] {
							violations = append(violations,
								typedPackage.Fset.Position(current.Pos()).String()+
									" source call closure crosses generic writer boundary "+site,
							)
							return true
						}
						if sensitiveLocalCallables[localRole] {
							localSite := site + "|" + sourceConstructionCallFingerprint(current)
							observedSensitiveLocalSites[localSite]++
							if allowedSensitiveLocalSites[localSite] == 0 {
								violations = append(violations,
									typedPackage.Fset.Position(current.Pos()).String()+
										" source call closure reaches unaudited sensitive local callee "+localSite,
								)
							}
						}
						if body := declarations[callable]; body != nil && body.Body != nil {
							queue = append(queue, callable)
							return true
						}
						if sourceConstructionSealedDispatch(callable, typedPackage.Types.Path()) {
							implementations, interfaceCall := sourceConstructionLocalInterfaceImplementationMethods(
								typedPackage,
								callable,
							)
							if !interfaceCall || len(implementations) == 0 {
								violations = append(violations,
									typedPackage.Fset.Position(current.Pos()).String()+
										" source call closure cannot resolve sealed dispatch "+site,
								)
								return true
							}
							queue = append(queue, implementations...)
							return true
						}
						if resolvedLocalInterfaceSites[site] != 0 {
							observedResolvedLocalInterfaceSites[site]++
							implementations, interfaceCall := sourceConstructionLocalInterfaceImplementationMethods(
								typedPackage,
								callable,
							)
							if !interfaceCall || len(implementations) == 0 {
								violations = append(violations,
									typedPackage.Fset.Position(current.Pos()).String()+
										" source call closure cannot resolve local callback dispatch "+site,
								)
								return true
							}
							queue = append(queue, implementations...)
							return true
						}
						observedBodylessSites[site]++
						if allowedBodylessSites[site] == 0 {
							violations = append(violations,
								typedPackage.Fset.Position(current.Pos()).String()+
									" source call closure reaches bodyless package callee "+site,
							)
						}
						return true
					}
					if resolvedExternalInterfaceRoles[role] {
						implementations, _ := sourceConstructionLocalInterfaceImplementationMethods(
							typedPackage,
							callable,
						)
						queue = append(queue, implementations...)
					}
					if allowedExternal[role] {
						return true
					}
					externalSite := site + "|" + sourceConstructionCallFingerprint(current)
					observedSensitiveExternalSites[externalSite]++
					if allowedSensitiveExternalSites[externalSite] == 0 {
						violations = append(violations,
							typedPackage.Fset.Position(current.Pos()).String()+
								" source call closure reaches unaudited external callee "+externalSite,
						)
					}
				default:
					site := filename + "|" + functionRole + "|" + types.ExprString(current.Fun)
					observedDynamicSites[site]++
					if allowedDynamicSites[site] == 0 {
						violations = append(violations,
							typedPackage.Fset.Position(current.Pos()).String()+
								" source call closure reaches unaudited dynamic callee "+site,
						)
					}
				}
			case *ast.Ident:
				used := typedPackage.TypesInfo.Uses[current]
				if variable, ok := used.(*types.Var); ok && variable.Pkg() != nil &&
					variable.Pkg().Path() == typedPackage.Types.Path() &&
					variable.Parent() == typedPackage.Types.Scope() {
					site := filename + "|" + functionRole + "|" + variable.Name()
					observedPackageVariableReferenceSites[site]++
					if allowedPackageVariableReferenceSites[site] == 0 {
						violations = append(violations,
							typedPackage.Fset.Position(current.Pos()).String()+
								" source call closure reads unaudited package variable "+site,
						)
					}
					return true
				}
				callable, _ := used.(*types.Func)
				if callable == nil || sourceConstructionDirectFunctionReferenceCall(current, parents) != nil {
					return true
				}
				site := filename + "|" + functionRole + "|" +
					sourceConstructionCallableRole(callable, typedPackage.Types.Path())
				observedFunctionReferenceSites[site]++
				if allowedFunctionReferenceSites[site] == 0 {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source call closure exposes function reference "+site,
					)
				}
				if callable.Pkg() != nil && callable.Pkg().Path() == typedPackage.Types.Path() &&
					declarations[callable] != nil {
					queue = append(queue, callable)
				}
			}
			return true
		})
	}
	if observedPackageAssertions != 1 {
		violations = append(violations,
			"audited source package compile-time assertion count = "+
				strconv.Itoa(observedPackageAssertions)+", want 1",
		)
	}
	for site, want := range allowedSensitiveExternalSites {
		if got := observedSensitiveExternalSites[site]; got != want {
			violations = append(violations,
				"audited sensitive external source call "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range allowedSensitiveLocalSites {
		if got := observedSensitiveLocalSites[site]; got != want {
			violations = append(violations,
				"audited sensitive local source call "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range allowedBodylessSites {
		if got := observedBodylessSites[site]; got != want {
			violations = append(violations,
				"audited bodyless source call "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range allowedDynamicSites {
		if got := observedDynamicSites[site]; got != want {
			violations = append(violations,
				"audited dynamic source call "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range allowedFunctionReferenceSites {
		if got := observedFunctionReferenceSites[site]; got != want {
			violations = append(violations,
				"audited source function reference "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range allowedRangeFunctionSites {
		if got := observedRangeFunctionSites[site]; got != want {
			violations = append(violations,
				"audited source range-over-function "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range allowedPackageVariableReferenceSites {
		if got := observedPackageVariableReferenceSites[site]; got != want {
			violations = append(violations,
				"audited source package variable reference "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range resolvedLocalInterfaceSites {
		if got := observedResolvedLocalInterfaceSites[site]; got != want {
			violations = append(violations,
				"audited resolved local callback site "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	slices.Sort(violations)
	return violations
}

func sourceConstructionAllowedPackageAssertion(filename string, declaration *ast.GenDecl) bool {
	if filename != "source_primitives_darwin.go" || declaration == nil ||
		declaration.Tok != token.VAR || len(declaration.Specs) != 1 {
		return false
	}
	specification, ok := declaration.Specs[0].(*ast.ValueSpec)
	if !ok || len(specification.Names) != 1 || specification.Names[0].Name != "_" ||
		len(specification.Values) != 1 {
		return false
	}
	interfaceName, ok := specification.Type.(*ast.Ident)
	if !ok || interfaceName.Name != "sourcePrimitives" {
		return false
	}
	composite, ok := specification.Values[0].(*ast.CompositeLit)
	if !ok || len(composite.Elts) != 0 {
		return false
	}
	implementationName, ok := composite.Type.(*ast.Ident)
	return ok && implementationName.Name == "darwinSourcePrimitives"
}

func sourceConstructionPackageInitializationViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.TypesInfo == nil || typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for package initialization audit"}
	}
	allowed := map[string]int{
		"filesystem.go|errUnsupportedAuthorityEntry:<infer>=errors.New(\"unsupported authority entry kind\");" +
			"errAuthorityKindMismatch:<infer>=errors.New(\"authority entry kind mismatch\")": 1,
		"preflight_runner.go|errInvalidNonSourceAuthorityContext:<infer>=" +
			"errors.New(\"invalid non-source authority context\")": 1,
		"source_primitives_darwin.go|_:sourcePrimitives=darwinSourcePrimitives{}": 1,
		"types.go|_:StagedPair=(*stagedPair)(nil)":                                1,
		"workspace.go|_:io.ReaderAt=(*sealedSpoolReader)(nil)":                    1,
	}
	allowedImports := map[string]bool{
		"bytes":                 true,
		"context":               true,
		"crypto/rand":           true,
		"crypto/sha256":         true,
		"encoding/binary":       true,
		"encoding/hex":          true,
		"encoding/json":         true,
		"errors":                true,
		"fmt":                   true,
		"golang.org/x/sys/unix": true,
		"hash":                  true,
		"io":                    true,
		"io/fs":                 true,
		"math":                  true,
		"os":                    true,
		"os/exec":               true,
		"path":                  true,
		"path/filepath":         true,
		"runtime":               true,
		"slices":                true,
		"sort":                  true,
		"strings":               true,
		"sync":                  true,
		"syscall":               true,
		"time":                  true,
		"unicode/utf8":          true,
		"unsafe":                true,
	}
	observed := make(map[string]int)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			alias := ""
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			allowedImport := err == nil && allowedImports[path]
			allowedAlias := alias != "_" && alias != "."
			if !allowedImport || !allowedAlias {
				violations = append(violations,
					typedPackage.Fset.Position(imported.Pos()).String()+
						" source package import is outside audited initialization set "+
						filename+"|"+alias+"|"+path,
				)
			}
		}
		for _, declaration := range file.Decls {
			switch current := declaration.(type) {
			case *ast.FuncDecl:
				if current.Name != nil && current.Name.Name == "init" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source package initialization function is forbidden in "+filename,
					)
				}
			case *ast.GenDecl:
				if current.Tok != token.VAR ||
					sourceConstructionCompileTimeBlankArrayAssertions(current) {
					continue
				}
				site := filename + "|" + sourceConstructionPackageVariableFingerprint(current)
				observed[site]++
				if allowed[site] == 0 {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source package variable initializer is outside audited set "+site,
					)
				}
			}
		}
	}
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited source package variable initializer "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	slices.Sort(violations)
	return violations
}

func sourceConstructionCompileTimeBlankArrayAssertions(declaration *ast.GenDecl) bool {
	if declaration == nil || declaration.Tok != token.VAR || len(declaration.Specs) == 0 {
		return false
	}
	for _, candidate := range declaration.Specs {
		specification, ok := candidate.(*ast.ValueSpec)
		if !ok || len(specification.Names) != 1 || specification.Names[0].Name != "_" ||
			len(specification.Values) != 0 {
			return false
		}
		if _, ok := specification.Type.(*ast.ArrayType); !ok {
			return false
		}
	}
	return true
}

func sourceConstructionPackageVariableFingerprint(declaration *ast.GenDecl) string {
	if declaration == nil {
		return "<nil>"
	}
	parts := make([]string, 0, len(declaration.Specs))
	for _, candidate := range declaration.Specs {
		specification, ok := candidate.(*ast.ValueSpec)
		if !ok {
			parts = append(parts, "<non-value-spec>")
			continue
		}
		names := make([]string, 0, len(specification.Names))
		for _, name := range specification.Names {
			names = append(names, name.Name)
		}
		typeName := "<infer>"
		if specification.Type != nil {
			typeName = types.ExprString(specification.Type)
		}
		values := make([]string, 0, len(specification.Values))
		for _, value := range specification.Values {
			values = append(values, types.ExprString(value))
		}
		parts = append(parts,
			strings.Join(names, ",")+":"+typeName+"="+strings.Join(values, ","),
		)
	}
	return strings.Join(parts, ";")
}

func sourceConstructionLocatorProvenanceViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for locator provenance audit"}
	}
	type locatorContract struct {
		filename string
		uses     map[string]int
	}
	expected := map[string]locatorContract{
		"(sourceRepositoryLocator).valid": {
			filename: "source_primitives.go",
			uses: map[string]int{
				"binary:locator.seal != validSourceRepositoryLocator": 1,
				"argument:0:strings.IndexByte(locator.path,0)":        1,
				"argument:0:absolutePathComponents(locator.path)":     1,
				"binary:locator.path != physicalRootPath":             1,
			},
		},
		"retainSourceConstruction": {
			filename: "source_construction_acquire.go",
			uses: map[string]int{
				"argument:1:retainSourceConstructionWith(ctx,locator,primitives)": 1,
			},
		},
		"retainSourceConstructionWith": {
			filename: "source_construction_acquire.go",
			uses: map[string]int{
				"receiver:locator.valid()":                        1,
				"argument:0:builder.retainInitialSource(locator)": 1,
			},
		},
		"(*sourceConstructionBuilder).retainInitialSource": {
			filename: "source_construction_acquire.go",
			uses: map[string]int{
				"argument:0:builder.retainRepository(locator)": 1,
			},
		},
		"(*sourceConstructionBuilder).retainRepository": {
			filename: "source_construction_acquire.go",
			uses: map[string]int{
				"key-value:path:locator.path":                                           1,
				"argument:1:builder.primitives.openRepositoryRoot(builder.ctx,locator)": 1,
				"argument:0:builder.retainRepositoryDescriptor(locator,repository)":     1,
			},
		},
		"(*sourceConstructionBuilder).retainRepositoryDescriptor": {
			filename: "source_construction_acquire.go",
			uses: map[string]int{
				"argument:0:absolutePathComponents(locator.path)": 1,
			},
		},
		"(darwinSourcePrimitives).openRepositoryRoot": {
			filename: "source_primitives_darwin.go",
			uses: map[string]int{
				"receiver:locator.valid()":             1,
				"argument:0:os.OpenRoot(locator.path)": 1,
			},
		},
	}

	parentsByFile := make(map[*ast.File]map[ast.Node]ast.Node)
	objects := make(map[types.Object]string)
	observedDefinitions := make(map[string]int)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		parentsByFile[file] = sourceConstructionParentNodes(file)
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		for _, candidate := range file.Decls {
			function, ok := candidate.(*ast.FuncDecl)
			if !ok {
				continue
			}
			role := sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			)
			fields := make([]*ast.Field, 0)
			if function.Recv != nil {
				fields = append(fields, function.Recv.List...)
			}
			if function.Type.Params != nil {
				fields = append(fields, function.Type.Params.List...)
			}
			for _, field := range fields {
				for _, name := range field.Names {
					object := typedPackage.TypesInfo.Defs[name]
					if sourceConstructionTypeName(sourceConstructionObjectType(object)) !=
						"sourceRepositoryLocator" {
						continue
					}
					contract, admitted := expected[role]
					if !admitted || contract.filename != filename || name.Name != "locator" {
						violations = append(violations,
							typedPackage.Fset.Position(name.Pos()).String()+
								" source repository locator definition outside audited chain "+
								filename+"|"+role+"|"+name.Name,
						)
						continue
					}
					observedDefinitions[role]++
					objects[object] = role
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if composite, ok := node.(*ast.CompositeLit); ok &&
				sourceConstructionTypeName(typedPackage.TypesInfo.TypeOf(composite)) ==
					"sourceRepositoryLocator" {
				violations = append(violations,
					typedPackage.Fset.Position(composite.Pos()).String()+
						" source repository locator literal is forbidden outside its future minting seam",
				)
			}
			identifier, ok := node.(*ast.Ident)
			defined, variable := typedPackage.TypesInfo.Defs[identifier].(*types.Var)
			if !ok || !variable ||
				sourceConstructionTypeName(
					sourceConstructionObjectType(defined),
				) != "sourceRepositoryLocator" {
				return true
			}
			if _, admitted := objects[defined]; !admitted {
				violations = append(violations,
					typedPackage.Fset.Position(identifier.Pos()).String()+
						" source repository locator value is minted outside audited parameter chain",
				)
			}
			return true
		})
	}

	observedUses := make(map[string]map[string]int)
	for _, file := range typedPackage.Syntax {
		parents := parentsByFile[file]
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			role, tracked := objects[typedPackage.TypesInfo.Uses[identifier]]
			if !tracked {
				return true
			}
			site := sourceConstructionLocatorUseSite(identifier, parents)
			if observedUses[role] == nil {
				observedUses[role] = make(map[string]int)
			}
			observedUses[role][site]++
			if expected[role].uses[site] == 0 {
				violations = append(violations,
					typedPackage.Fset.Position(identifier.Pos()).String()+
						" source repository locator use outside audited provenance "+role+"|"+site,
				)
			}
			return true
		})
	}
	for role, contract := range expected {
		if got := observedDefinitions[role]; got != 1 {
			violations = append(violations,
				"audited source repository locator definition "+role+" count = "+
					strconv.Itoa(got)+", want 1",
			)
		}
		for site, want := range contract.uses {
			if got := observedUses[role][site]; got != want {
				violations = append(violations,
					"audited source repository locator use "+role+"|"+site+" count = "+
						strconv.Itoa(got)+", want "+strconv.Itoa(want),
				)
			}
		}
	}
	slices.Sort(violations)
	return violations
}

func sourceConstructionLocatorUseSite(
	identifier *ast.Ident,
	parents map[ast.Node]ast.Node,
) string {
	if identifier == nil || parents == nil {
		return "<unresolved>"
	}
	parent := parents[identifier]
	if selector, ok := parent.(*ast.SelectorExpr); ok && selector.X == identifier {
		grandparent := parents[selector]
		if call, ok := grandparent.(*ast.CallExpr); ok {
			if call.Fun == selector {
				return "receiver:" + sourceConstructionCallFingerprint(call)
			}
			for index, argument := range call.Args {
				if sourceConstructionDirectExpression(argument, selector) {
					return "argument:" + strconv.Itoa(index) + ":" +
						sourceConstructionCallFingerprint(call)
				}
			}
		}
		if keyValue, ok := grandparent.(*ast.KeyValueExpr); ok && keyValue.Value == selector {
			return "key-value:" + types.ExprString(keyValue.Key) + ":" +
				types.ExprString(selector)
		}
		if binary, ok := grandparent.(*ast.BinaryExpr); ok {
			return "binary:" + types.ExprString(binary)
		}
		return "selector:" + selector.Sel.Name
	}
	if call, ok := parent.(*ast.CallExpr); ok {
		for index, argument := range call.Args {
			if sourceConstructionDirectExpression(argument, identifier) {
				return "argument:" + strconv.Itoa(index) + ":" +
					sourceConstructionCallFingerprint(call)
			}
		}
	}
	return fmt.Sprintf("%T:%s", parent, types.ExprString(identifier))
}

func sourceConstructionReadOnlyParameterViolations(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	filename string,
	functionRole string,
) []string {
	if typedPackage == nil || typedPackage.TypesInfo == nil || typedPackage.Fset == nil ||
		function == nil || function.Body == nil || function.Type.Params == nil {
		return nil
	}
	readOnlyByRole := map[string]map[string]bool{
		"(darwinSourcePrimitives).openRelativeNoFollow": {
			"parent": true,
			"name":   true,
			"kind":   true,
			"mode":   true,
		},
		"openDarwinSourceRelativeDescriptor": {
			"parent":   true,
			"name":     true,
			"kind":     true,
			"openKind": true,
			"mode":     true,
			"flags":    true,
		},
		"(*sourceConstructionBuilder).captureSourceAdministrativeEntry": {
			"capture": true,
			"parent":  true,
			"prefix":  true,
			"name":    true,
		},
		"(*sourceConstructionBuilder).captureSourceAdministrativeDirectory": {
			"capture":    true,
			"descriptor": true,
			"prefix":     true,
			"before":     true,
		},
		"(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry": {
			"capture":   true,
			"parent":    true,
			"name":      true,
			"candidate": true,
		},
		"(*sourceConstructionBuilder).finishSourceAdministrativeInventory": {
			"capture": true,
		},
		"(*sourceConstructionBuilder).validateSourceAdministrativeCandidate": {
			"candidate": true,
		},
		"sourceAdministrativeChildPath": {
			"prefix": true,
			"name":   true,
		},
		"classifyPathError": {
			"err": true,
		},
		"contextCause": {
			"err": true,
		},
		"normalizeDarwinSourceDirectoryBatch": {
			"entries": true,
			"err":     true,
		},
		"classifyDarwinSourcePresenceFailure": {
			"mode": true,
			"err":  true,
		},
		"classifyDarwinSourceOpenFailure": {
			"mode": true,
			"err":  true,
		},
		"classifyDarwinSourceWalkFailure": {
			"err": true,
		},
		"darwinSourceIOCause": {
			"err": true,
		},
	}
	readOnly := make(map[string]bool)
	for name := range readOnlyByRole[functionRole] {
		readOnly[name] = true
	}
	parameters := make(map[types.Object]string)
	parameterNames := make(map[string]types.Object)
	for _, field := range function.Type.Params.List {
		for _, name := range field.Names {
			object := typedPackage.TypesInfo.Defs[name]
			parameterRole := sourceConstructionParameterRole(
				sourceConstructionObjectType(object),
				typedPackage.Types.Path(),
			)
			if parameterRole == "context.Context" || parameterRole == "error" {
				readOnly[name.Name] = true
			}
			if object == nil || name.Name == "_" || !readOnly[name.Name] {
				continue
			}
			parameters[object] = name.Name
			parameterNames[name.Name] = object
		}
	}
	if len(parameters) == 0 {
		return nil
	}
	violations := make([]string, 0)
	report := func(node ast.Node, name string, reason string) {
		violations = append(violations,
			typedPackage.Fset.Position(node.Pos()).String()+
				" source call closure requires read-only parameter "+
				filename+"|"+functionRole+"|"+name+"|"+reason,
		)
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.Ident:
			defined := typedPackage.TypesInfo.Defs[current]
			if defined == nil {
				return true
			}
			parameter, shadows := parameterNames[current.Name]
			if shadows && defined != parameter {
				report(current, current.Name, "shadow")
			}
		case *ast.AssignStmt:
			for _, target := range current.Lhs {
				for object, name := range parameters {
					if sourceConstructionDirectObjectExpression(
						typedPackage.TypesInfo,
						target,
						object,
					) {
						report(target, name, "assignment")
					}
				}
			}
		case *ast.IncDecStmt:
			for object, name := range parameters {
				if sourceConstructionDirectObjectExpression(
					typedPackage.TypesInfo,
					current.X,
					object,
				) {
					report(current, name, "increment")
				}
			}
		case *ast.RangeStmt:
			if current.Tok == token.DEFINE {
				return true
			}
			for _, target := range []ast.Expr{current.Key, current.Value} {
				for object, name := range parameters {
					if sourceConstructionDirectObjectExpression(
						typedPackage.TypesInfo,
						target,
						object,
					) {
						report(current, name, "range-assignment")
					}
				}
			}
		case *ast.UnaryExpr:
			if current.Op != token.AND {
				return true
			}
			for object, name := range parameters {
				if sourceConstructionDirectObjectExpression(
					typedPackage.TypesInfo,
					current.X,
					object,
				) {
					report(current, name, "address")
				}
			}
		}
		return true
	})
	return violations
}

func sourceConstructionSealedConcreteReceiverViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for sealed receiver audit"}
	}
	allowedImplementers := map[string]bool{
		"darwinSourcePrimitives":   true,
		"directSourceHandleCloser": true,
	}
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			selection := typedPackage.TypesInfo.Selections[selector]
			if selection == nil {
				return true
			}
			method, _ := selection.Obj().(*types.Func)
			signature, _ := sourceConstructionObjectType(method).(*types.Signature)
			if signature == nil || signature.Recv() == nil {
				return true
			}
			declaredReceiver := types.Unalias(signature.Recv().Type())
			if pointer, ok := declaredReceiver.(*types.Pointer); ok {
				declaredReceiver = types.Unalias(pointer.Elem())
			}
			named, ok := declaredReceiver.(*types.Named)
			if !ok || named.Obj().Pkg() == nil ||
				named.Obj().Pkg().Path() != typedPackage.Types.Path() ||
				!allowedImplementers[named.Obj().Name()] {
				return true
			}
			selectedReceiver := types.Unalias(selection.Recv())
			selectedNamed, selectedOK := selectedReceiver.(*types.Named)
			if selectedOK && selectedNamed.Obj() == named.Obj() &&
				sourceConstructionSealedConcreteValueReceiverProved(
					typedPackage.TypesInfo,
					selector.X,
					named.Obj(),
				) {
				return true
			}
			violations = append(violations,
				typedPackage.Fset.Position(selector.Pos()).String()+
					" sealed concrete method receiver is not an exact non-pointer approved value "+
					types.ExprString(selector),
			)
			return true
		})
	}
	slices.Sort(violations)
	return violations
}

func sourceConstructionSealedConcreteValueReceiverProved(
	info *types.Info,
	expression ast.Expr,
	want *types.TypeName,
) bool {
	if info == nil || expression == nil || want == nil {
		return false
	}
	expression = sourceConstructionUnparenthesizedExpression(expression)
	switch current := expression.(type) {
	case *ast.Ident:
		named, _ := types.Unalias(info.TypeOf(current)).(*types.Named)
		return named != nil && named.Obj() == want
	case *ast.CompositeLit:
		named, _ := types.Unalias(info.TypeOf(current)).(*types.Named)
		return named != nil && named.Obj() == want && len(current.Elts) == 0
	default:
		return false
	}
}

func sourceConstructionImplicitCallbackViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for implicit callback audit"}
	}
	allowedMethods := sourceConstructionAllowedImplicitCallbackMethods()
	callbackNames := map[string]bool{
		"Deadline": true,
		"Done":     true,
		"Error":    true,
		"Err":      true,
		"Format":   true,
		"Is":       true,
		"String":   true,
		"Unwrap":   true,
		"Value":    true,
	}
	observed := make(map[string]int)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		for _, candidate := range file.Decls {
			function, ok := candidate.(*ast.FuncDecl)
			if !ok || function.Recv == nil || function.Name == nil ||
				!callbackNames[function.Name.Name] {
				continue
			}
			role := sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			)
			site := filename + "|" + role
			observed[site]++
			if allowedMethods[site] == 0 {
				violations = append(violations,
					typedPackage.Fset.Position(function.Name.Pos()).String()+
						" implicit callback method is outside audited set "+site,
				)
			}
		}
	}
	for site, want := range allowedMethods {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited implicit callback method "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	slices.Sort(violations)
	return violations
}

func sourceConstructionAllowedImplicitCallbackMethods() map[string]int {
	return map[string]int{
		"macho.go|(*machOValidationError).Error":                    1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Deadline": 1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Done":     1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Err":      1,
		"preflight_runner.go|(*nonSourceAuthorityContext).Value":    1,
		"private_errors.go|(*privateFailure).Error":                 1,
		"private_errors.go|(*privateFailure).Unwrap":                1,
		"types.go|(*failureError).Error":                            1,
	}
}

func sourceConstructionErrorInspectionViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for error inspection audit"}
	}
	errorName, _ := types.Universe.Lookup("error").(*types.TypeName)
	if errorName == nil {
		return []string{"predeclared error interface is unavailable for error inspection audit"}
	}
	errorInterface, _ := types.Unalias(errorName.Type()).Underlying().(*types.Interface)
	if errorInterface == nil {
		return []string{"predeclared error interface has unexpected type"}
	}
	allowed := map[string]int{
		"filesystem.go|failWithFilesystemCauses|failWith(err,cause,message).(*privateFailure)": 1,
		"filesystem.go|filesystemPrivateCauses|current.(*privateFailure)":                      1,
		"filesystem.go|filesystemPrivateCauses|current.(type){interface{Unwrap() []error}," +
			"interface{Unwrap() error}}": 1,
		"private_errors.go|privateCauses|current.(*privateFailure)": 1,
		"private_errors.go|privateCauses|current.(type){interface{Unwrap() []error}," +
			"interface{Unwrap() error}}": 1,
	}
	observed := make(map[string]int)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		parents := sourceConstructionParentNodes(file)
		derivedErrors := sourceConstructionErrorDerivedObjects(
			typedPackage.TypesInfo,
			file,
			errorInterface,
		)
		ast.Inspect(file, func(node ast.Node) bool {
			assertion, ok := node.(*ast.TypeAssertExpr)
			if !ok || assertion.X == nil {
				return true
			}
			assertedType := types.Type(nil)
			if assertion.Type != nil {
				assertedType = typedPackage.TypesInfo.TypeOf(assertion.Type)
			}
			originatesFromError := sourceConstructionExpressionOriginatesFromError(
				typedPackage.TypesInfo,
				assertion.X,
				errorInterface,
				derivedErrors,
			)
			assertsError := assertedType != nil && types.Implements(
				assertedType,
				errorInterface,
			)
			if !originatesFromError && !assertsError &&
				!sourceConstructionTypeSwitchInspectsError(
					typedPackage.TypesInfo,
					assertion,
					parents,
					errorInterface,
				) {
				return true
			}
			function := sourceConstructionEnclosingFunction(file, assertion.Pos())
			role := "<outside-function>"
			if function != nil {
				role = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			site := filename + "|" + role + "|" +
				sourceConstructionTypeAssertionFingerprint(assertion) +
				sourceConstructionTypeSwitchCasesFingerprint(assertion, parents)
			observed[site]++
			if allowed[site] == 0 {
				violations = append(violations,
					typedPackage.Fset.Position(assertion.Pos()).String()+
						" source error type inspection is outside audited set "+site,
				)
			}
			return true
		})
	}
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited source error type inspection "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	slices.Sort(violations)
	return violations
}

func sourceConstructionTypeAssertionFingerprint(assertion *ast.TypeAssertExpr) string {
	if assertion == nil || assertion.X == nil {
		return "<invalid-type-assertion>"
	}
	receiver := types.ExprString(assertion.X)
	if call, ok := assertion.X.(*ast.CallExpr); ok {
		receiver = sourceConstructionCallFingerprint(call)
	}
	if assertion.Type == nil {
		return receiver + ".(type)"
	}
	return receiver + ".(" + types.ExprString(assertion.Type) + ")"
}

func sourceConstructionTypeSwitchCasesFingerprint(
	assertion *ast.TypeAssertExpr,
	parents map[ast.Node]ast.Node,
) string {
	if assertion == nil || assertion.Type != nil {
		return ""
	}
	var typeSwitch *ast.TypeSwitchStmt
	for node := ast.Node(assertion); node != nil; node = parents[node] {
		if current, ok := node.(*ast.TypeSwitchStmt); ok {
			typeSwitch = current
			break
		}
	}
	if typeSwitch == nil || typeSwitch.Body == nil {
		return "{<unresolved>}"
	}
	cases := make([]string, 0)
	for _, statement := range typeSwitch.Body.List {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			cases = append(cases, "<non-case>")
			continue
		}
		if len(clause.List) == 0 {
			cases = append(cases, "default")
			continue
		}
		for _, expression := range clause.List {
			cases = append(cases, types.ExprString(expression))
		}
	}
	return "{" + strings.Join(cases, ",") + "}"
}

func sourceConstructionTypeSwitchInspectsError(
	info *types.Info,
	assertion *ast.TypeAssertExpr,
	parents map[ast.Node]ast.Node,
	errorInterface *types.Interface,
) bool {
	if assertion == nil || assertion.Type != nil {
		return false
	}
	for node := ast.Node(assertion); node != nil; node = parents[node] {
		typeSwitch, ok := node.(*ast.TypeSwitchStmt)
		if !ok || typeSwitch.Body == nil {
			continue
		}
		for _, statement := range typeSwitch.Body.List {
			clause, ok := statement.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expression := range clause.List {
				caseType := info.TypeOf(expression)
				if caseType != nil && types.Implements(caseType, errorInterface) {
					return true
				}
			}
		}
		return false
	}
	return false
}

func sourceConstructionErrorDerivedObjects(
	info *types.Info,
	file *ast.File,
	errorInterface *types.Interface,
) map[types.Object]bool {
	derived := make(map[types.Object]bool)
	changed := true
	for changed {
		changed = false
		ast.Inspect(file, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.AssignStmt:
				if len(current.Lhs) != len(current.Rhs) {
					return true
				}
				for index, right := range current.Rhs {
					if !sourceConstructionExpressionOriginatesFromError(
						info,
						right,
						errorInterface,
						derived,
					) {
						continue
					}
					identifier, ok := sourceConstructionUnparenthesizedExpression(
						current.Lhs[index],
					).(*ast.Ident)
					if !ok || identifier.Name == "_" {
						continue
					}
					object := info.Uses[identifier]
					if current.Tok == token.DEFINE {
						object = info.Defs[identifier]
					}
					if object != nil && !derived[object] {
						derived[object] = true
						changed = true
					}
				}
			case *ast.ValueSpec:
				if len(current.Names) != len(current.Values) {
					return true
				}
				for index, value := range current.Values {
					if !sourceConstructionExpressionOriginatesFromError(
						info,
						value,
						errorInterface,
						derived,
					) {
						continue
					}
					object := info.Defs[current.Names[index]]
					if object != nil && !derived[object] {
						derived[object] = true
						changed = true
					}
				}
			}
			return true
		})
	}
	return derived
}

func sourceConstructionExpressionOriginatesFromError(
	info *types.Info,
	expression ast.Expr,
	errorInterface *types.Interface,
	derived map[types.Object]bool,
) bool {
	if info == nil || expression == nil || errorInterface == nil {
		return false
	}
	expression = sourceConstructionUnparenthesizedExpression(expression)
	if typeOf := info.TypeOf(expression); typeOf != nil && types.Implements(
		typeOf,
		errorInterface,
	) {
		return true
	}
	if identifier, ok := expression.(*ast.Ident); ok &&
		(derived[info.Uses[identifier]] || derived[info.Defs[identifier]]) {
		return true
	}
	conversion, ok := expression.(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 || !info.Types[conversion.Fun].IsType() {
		return false
	}
	return sourceConstructionExpressionOriginatesFromError(
		info,
		conversion.Args[0],
		errorInterface,
		derived,
	)
}

func sourceConstructionAdministrativePathProvenanceViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for administrative path audit"}
	}
	wantRoles := map[string]bool{
		"(*sourceConstructionBuilder).retainSourceAdministrativeInventory":    true,
		"(*sourceConstructionBuilder).captureSourceAdministrativeDirectory":   true,
		"(*sourceConstructionBuilder).captureSourceAdministrativeEntry":       true,
		"(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry": true,
		"(*sourceConstructionBuilder).finishSourceAdministrativeInventory":    true,
		"(*sourceConstructionBuilder).validateSourceAdministrativeCandidate":  true,
		"sourceAdministrativeChildPath":                                       true,
	}
	declarations := make(map[string]*ast.FuncDecl)
	files := make(map[string]*ast.File)
	for _, file := range typedPackage.Syntax {
		if filepath.Base(typedPackage.Fset.Position(file.Package).Filename) !=
			"source_construction_inventory.go" {
			continue
		}
		for _, candidate := range file.Decls {
			function, ok := candidate.(*ast.FuncDecl)
			if !ok {
				continue
			}
			role := sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			)
			if !wantRoles[role] {
				continue
			}
			if declarations[role] != nil {
				declarations[role] = nil
				continue
			}
			declarations[role] = function
			files[role] = file
		}
	}
	violations := make([]string, 0)
	retentionRole := "(*sourceConstructionBuilder).retainSourceAdministrativeInventory"
	if !sourceConstructionAdministrativeValidatedInstallProved(
		typedPackage,
		declarations[retentionRole],
	) {
		position := retentionRole
		if declarations[retentionRole] != nil {
			position = typedPackage.Fset.Position(declarations[retentionRole].Pos()).String()
		}
		violations = append(violations,
			position+" source administrative validated install is outside audited shape",
		)
	}
	childPathRole := "sourceAdministrativeChildPath"
	if !sourceConstructionAdministrativeChildPathProved(
		typedPackage,
		declarations[childPathRole],
	) {
		position := childPathRole
		if declarations[childPathRole] != nil {
			position = typedPackage.Fset.Position(declarations[childPathRole].Pos()).String()
		}
		violations = append(violations,
			position+" source administrative child path construction is outside audited shape",
		)
	}
	directoryRole := "(*sourceConstructionBuilder).captureSourceAdministrativeDirectory"
	if !sourceConstructionAdministrativeDirectoryDispatchProved(
		typedPackage,
		declarations[directoryRole],
	) {
		position := directoryRole
		if declarations[directoryRole] != nil {
			position = typedPackage.Fset.Position(declarations[directoryRole].Pos()).String()
		}
		violations = append(violations,
			position+" source administrative directory batch/name provenance is outside audited shape",
		)
	}
	entryRole := "(*sourceConstructionBuilder).captureSourceAdministrativeEntry"
	if !sourceConstructionAdministrativeEntryPathProved(
		typedPackage,
		declarations[entryRole],
		files[entryRole],
	) {
		position := entryRole
		if declarations[entryRole] != nil {
			position = typedPackage.Fset.Position(declarations[entryRole].Pos()).String()
		}
		violations = append(violations,
			position+" source administrative child path/name provenance is outside audited shape",
		)
	}
	openedRole := "(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry"
	if !sourceConstructionAdministrativeOpenedPathProved(
		typedPackage,
		declarations[openedRole],
		files[openedRole],
	) {
		position := openedRole
		if declarations[openedRole] != nil {
			position = typedPackage.Fset.Position(declarations[openedRole].Pos()).String()
		}
		violations = append(violations,
			position+" opened source administrative row/name provenance is outside audited shape",
		)
	}
	finishRole := "(*sourceConstructionBuilder).finishSourceAdministrativeInventory"
	if !sourceConstructionAdministrativeFinishTopologyProved(
		typedPackage,
		declarations[finishRole],
	) {
		position := finishRole
		if declarations[finishRole] != nil {
			position = typedPackage.Fset.Position(declarations[finishRole].Pos()).String()
		}
		violations = append(violations,
			position+" source administrative finish topology is outside audited shape",
		)
	}
	validationRole := "(*sourceConstructionBuilder).validateSourceAdministrativeCandidate"
	if !sourceConstructionAdministrativeCandidateValidationProved(
		typedPackage,
		declarations[validationRole],
	) {
		position := validationRole
		if declarations[validationRole] != nil {
			position = typedPackage.Fset.Position(declarations[validationRole].Pos()).String()
		}
		violations = append(violations,
			position+" source administrative candidate validation is outside audited shape",
		)
	}
	return violations
}

func sourceConstructionAdministrativeValidatedInstallProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 18 {
		return false
	}
	info := typedPackage.TypesInfo
	statements := function.Body.List
	finishAssignment, ok := statements[13].(*ast.AssignStmt)
	if !ok || finishAssignment.Tok != token.DEFINE || len(finishAssignment.Lhs) != 2 ||
		len(finishAssignment.Rhs) != 1 {
		return false
	}
	inventory, inventoryOK := finishAssignment.Lhs[0].(*ast.Ident)
	failure, failureOK := finishAssignment.Lhs[1].(*ast.Ident)
	finish, callOK := finishAssignment.Rhs[0].(*ast.CallExpr)
	if !inventoryOK || !failureOK || !callOK || inventory.Name != "inventory" ||
		failure.Name != "failure" || sourceConstructionCallFingerprint(finish) !=
		"builder.finishSourceAdministrativeInventory(capture)" {
		return false
	}
	inventoryObject := info.Defs[inventory]
	failureObject := info.Uses[failure]
	if inventoryObject == nil || failureObject == nil ||
		!sourceConstructionAdministrativeOutcomeFailureBranchProved(
			info,
			statements[14],
			failureObject,
		) || !sourceConstructionAdministrativeOwnerValidationGuardProved(
		info,
		statements[15],
		inventoryObject,
		failureObject,
	) {
		return false
	}
	install, ok := statements[16].(*ast.AssignStmt)
	if !ok || install.Tok != token.ASSIGN || len(install.Lhs) != 1 ||
		len(install.Rhs) != 1 || types.ExprString(install.Lhs[0]) !=
		"builder.owner.git.administration" || !sourceConstructionDirectObjectExpression(
		info,
		install.Rhs[0],
		inventoryObject,
	) {
		return false
	}
	return sourceConstructionSingleBooleanReturnProved(statements[17], "true")
}

func sourceConstructionAdministrativeOwnerValidationGuardProved(
	info *types.Info,
	statement ast.Stmt,
	inventoryObject types.Object,
	failureObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) != 2 ||
		!sourceConstructionObjectNilComparison(info, branch.Cond, failureObject, token.NEQ) {
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
	if !ok || sourceConstructionCallFingerprint(validation) !=
		"validateSourceAdministrativeInventoryOwner(builder.ctx,builder.owner,inventory)" ||
		len(validation.Args) != 3 || !sourceConstructionDirectObjectExpression(
		info,
		validation.Args[2],
		inventoryObject,
	) {
		return false
	}
	addExpression, ok := branch.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	add, ok := addExpression.X.(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(add) ==
		"builder.outcome.addPrimitive(failure)" && len(add.Args) == 1 &&
		sourceConstructionDirectObjectExpression(info, add.Args[0], failureObject) &&
		sourceConstructionSingleBooleanReturnProved(branch.Body.List[1], "false")
}

func sourceConstructionAdministrativeChildPathProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 6 {
		return false
	}
	if !sourceConstructionSingleFailureGuardReturnProved(
		typedPackage.TypesInfo,
		function.Body.List[0],
		"sourceContextPrimitiveFailure(ctx,OperationWalk)",
		`""`,
	) || !sourceConstructionFixedFailureBranchProved(
		function.Body.List[1],
		`prefix == "" || name == "" || strings.IndexByte(name, 0) >= 0 || name == "." || name == ".." || strings.Contains(name, "/")`,
		`""`,
		"newSourcePrimitiveFailure(OperationWalk,CauseUnstable)",
	) || !sourceConstructionFixedFailureBranchProved(
		function.Body.List[2],
		"len([]byte(name)) > maxPathComponentBytes || "+
			"len([]byte(prefix)) + 1 + len([]byte(name)) > maxRelativePathBytes",
		`""`,
		"newSourcePrimitiveFailure(OperationWalk,CauseLimit)",
	) {
		return false
	}
	assignment, ok := function.Body.List[3].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || types.ExprString(assignment.Rhs[0]) !=
		"prefix + \"/\" + name" {
		return false
	}
	path, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok || path.Name != "path" {
		return false
	}
	pathObject := typedPackage.TypesInfo.Defs[path]
	if pathObject == nil {
		return false
	}
	if !sourceConstructionSingleFailureGuardReturnProved(
		typedPackage.TypesInfo,
		function.Body.List[4],
		"sourceContextPrimitiveFailure(ctx,OperationWalk)",
		`""`,
	) {
		return false
	}
	returned, ok := function.Body.List[5].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 || !sourceConstructionDirectObjectExpression(
		typedPackage.TypesInfo,
		returned.Results[0],
		pathObject,
	) || !sourceConstructionBareNilExpression(returned.Results[1]) {
		return false
	}
	uses := 0
	for identifier, object := range typedPackage.TypesInfo.Uses {
		if identifier.Pos() >= function.Pos() && identifier.End() <= function.End() &&
			object == pathObject {
			uses++
		}
	}
	return uses == 1
}

func sourceConstructionSingleFailureGuardReturnProved(
	info *types.Info,
	statement ast.Stmt,
	callFingerprint string,
	firstResultFingerprint string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	assignment, ok := branch.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return false
	}
	failure, ok := assignment.Lhs[0].(*ast.Ident)
	call, callOK := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || !callOK || failure.Name != "failure" ||
		sourceConstructionCallFingerprint(call) != callFingerprint {
		return false
	}
	failureObject := info.Defs[failure]
	if failureObject == nil || !sourceConstructionObjectNilComparison(
		info,
		branch.Cond,
		failureObject,
		token.NEQ,
	) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 2 &&
		types.ExprString(returned.Results[0]) == firstResultFingerprint &&
		sourceConstructionDirectObjectExpression(info, returned.Results[1], failureObject)
}

func sourceConstructionFixedFailureBranchProved(
	statement ast.Stmt,
	conditionFingerprint string,
	firstResultFingerprint string,
	failureFingerprint string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Init == nil && branch.Else == nil &&
		types.ExprString(branch.Cond) == conditionFingerprint &&
		len(branch.Body.List) == 1 && sourceConstructionFixedFailureReturnProved(
		branch.Body.List[0],
		firstResultFingerprint,
		failureFingerprint,
	)
}

func sourceConstructionFixedFailureReturnProved(
	statement ast.Stmt,
	firstResultFingerprint string,
	failureFingerprint string,
) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 ||
		types.ExprString(returned.Results[0]) != firstResultFingerprint {
		return false
	}
	failure, ok := returned.Results[1].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(failure) == failureFingerprint
}

func sourceConstructionAdministrativeFinishTopologyProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 11 {
		return false
	}
	info := typedPackage.TypesInfo
	captureObject := sourceConstructionNamedParameterObject(info, function, "capture")
	if captureObject == nil || !sourceConstructionSingleFailureGuardReturnProved(
		info,
		function.Body.List[0],
		"sourceContextPrimitiveFailure(builder.ctx,OperationWalk)",
		"nil",
	) || !sourceConstructionFixedFailureBranchProved(
		function.Body.List[1],
		"capture == nil || len(capture.candidates) == 0 || "+
			"capture.descendants != uint64(len(capture.candidates) - 1)",
		"nil",
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)",
	) || !sourceConstructionAdministrativeSortProved(
		info,
		function.Body.List[2],
		captureObject,
	) || !sourceConstructionSingleFailureGuardReturnProved(
		info,
		function.Body.List[3],
		"classifySourceAdministrativeCandidates(builder.ctx,capture.candidates,builder.owner.config.claim.objectFormat)",
		"nil",
	) || !sourceConstructionSingleFailureGuardReturnProved(
		info,
		function.Body.List[4],
		"rejectForbiddenSourceAdministrativeCandidates(builder.ctx,capture.candidates)",
		"nil",
	) || !sourceConstructionSingleFailureGuardReturnProved(
		info,
		function.Body.List[5],
		"builder.compareRetainedSourceAdministrativeRows(capture.candidates)",
		"nil",
	) || !sourceConstructionAdministrativeValidationRangeProved(
		info,
		function.Body.List[6],
		captureObject,
	) {
		return false
	}
	rowsAssignment, ok := function.Body.List[7].(*ast.AssignStmt)
	if !ok || rowsAssignment.Tok != token.DEFINE || len(rowsAssignment.Lhs) != 1 ||
		len(rowsAssignment.Rhs) != 1 {
		return false
	}
	rows, ok := rowsAssignment.Lhs[0].(*ast.Ident)
	makeRows, callOK := rowsAssignment.Rhs[0].(*ast.CallExpr)
	if !ok || !callOK || rows.Name != "rows" ||
		sourceConstructionCallFingerprint(makeRows) !=
			"make([]sourceAdministrativeRow,len(capture.candidates))" {
		return false
	}
	rowsObject := info.Defs[rows]
	if rowsObject == nil || !sourceConstructionAdministrativeCopyRangeProved(
		info,
		function.Body.List[8],
		captureObject,
		rowsObject,
	) || !sourceConstructionSingleFailureGuardReturnProved(
		info,
		function.Body.List[9],
		"sourceContextPrimitiveFailure(builder.ctx,OperationWalk)",
		"nil",
	) {
		return false
	}
	return sourceConstructionAdministrativeInventoryReturnProved(
		info,
		function.Body.List[10],
		captureObject,
		rowsObject,
	)
}

func sourceConstructionAdministrativeSortProved(
	info *types.Info,
	statement ast.Stmt,
	captureObject types.Object,
) bool {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok || types.ExprString(call.Fun) != "sort.Slice" || len(call.Args) != 2 ||
		!sourceConstructionSelectorChainProved(
			info,
			call.Args[0],
			captureObject,
			"candidates",
		) {
		return false
	}
	comparator, ok := call.Args[1].(*ast.FuncLit)
	if !ok || comparator.Type == nil ||
		types.ExprString(comparator.Type) != "func(left, right int) bool" ||
		comparator.Body == nil || len(comparator.Body.List) != 1 {
		return false
	}
	returned, ok := comparator.Body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && types.ExprString(returned.Results[0]) ==
		"bytes.Compare([]byte(capture.candidates[left].row.path), "+
			"[]byte(capture.candidates[right].row.path)) < 0"
}

func sourceConstructionAdministrativeValidationRangeProved(
	info *types.Info,
	statement ast.Stmt,
	captureObject types.Object,
) bool {
	loop, ok := statement.(*ast.RangeStmt)
	if !ok || loop.Tok != token.DEFINE || loop.Value != nil || len(loop.Body.List) != 2 ||
		!sourceConstructionSelectorChainProved(
			info,
			loop.X,
			captureObject,
			"candidates",
		) {
		return false
	}
	index, ok := loop.Key.(*ast.Ident)
	if !ok || index.Name != "index" || info.Defs[index] == nil {
		return false
	}
	candidateAssignment, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || candidateAssignment.Tok != token.DEFINE ||
		len(candidateAssignment.Lhs) != 1 || len(candidateAssignment.Rhs) != 1 ||
		types.ExprString(candidateAssignment.Rhs[0]) != "capture.candidates[index]" {
		return false
	}
	candidate, ok := candidateAssignment.Lhs[0].(*ast.Ident)
	if !ok || candidate.Name != "candidate" || info.Defs[candidate] == nil {
		return false
	}
	return sourceConstructionSingleFailureGuardReturnProved(
		info,
		loop.Body.List[1],
		"builder.validateSourceAdministrativeCandidate(candidate)",
		"nil",
	)
}

func sourceConstructionAdministrativeCopyRangeProved(
	info *types.Info,
	statement ast.Stmt,
	captureObject types.Object,
	rowsObject types.Object,
) bool {
	loop, ok := statement.(*ast.RangeStmt)
	if !ok || loop.Tok != token.DEFINE || loop.Value != nil || len(loop.Body.List) != 1 ||
		!sourceConstructionSelectorChainProved(
			info,
			loop.X,
			captureObject,
			"candidates",
		) {
		return false
	}
	index, ok := loop.Key.(*ast.Ident)
	if !ok || index.Name != "index" {
		return false
	}
	indexObject := info.Defs[index]
	assignment, ok := loop.Body.List[0].(*ast.AssignStmt)
	if indexObject == nil || !ok || assignment.Tok != token.ASSIGN ||
		len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 ||
		types.ExprString(assignment.Rhs[0]) != "capture.candidates[index].row" {
		return false
	}
	indexed, ok := assignment.Lhs[0].(*ast.IndexExpr)
	return ok && sourceConstructionDirectObjectExpression(info, indexed.X, rowsObject) &&
		sourceConstructionDirectObjectExpression(info, indexed.Index, indexObject)
}

func sourceConstructionAdministrativeInventoryReturnProved(
	info *types.Info,
	statement ast.Stmt,
	captureObject types.Object,
	rowsObject types.Object,
) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 ||
		!sourceConstructionBareNilExpression(returned.Results[1]) {
		return false
	}
	address, ok := returned.Results[0].(*ast.UnaryExpr)
	if !ok || address.Op != token.AND {
		return false
	}
	composite, ok := address.X.(*ast.CompositeLit)
	if !ok || sourceConstructionTypeName(info.TypeOf(composite)) !=
		"sourceAdministrativeInventory" || len(composite.Elts) != 2 {
		return false
	}
	rows, rowsOK := composite.Elts[0].(*ast.KeyValueExpr)
	total, totalOK := composite.Elts[1].(*ast.KeyValueExpr)
	rowsKey, rowsKeyOK := rows.Key.(*ast.Ident)
	totalKey, totalKeyOK := total.Key.(*ast.Ident)
	return rowsOK && totalOK && rowsKeyOK && totalKeyOK && rowsKey.Name == "rows" &&
		totalKey.Name == "totalRegularBytes" &&
		sourceConstructionDirectObjectExpression(info, rows.Value, rowsObject) &&
		sourceConstructionSelectorChainProved(
			info,
			total.Value,
			captureObject,
			"totalRegularBytes",
		)
}

func sourceConstructionSelectorChainProved(
	info *types.Info,
	expression ast.Expr,
	rootObject types.Object,
	fields ...string,
) bool {
	for _, field := range slices.Backward(fields) {
		selector, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != field {
			return false
		}
		expression = selector.X
	}
	return sourceConstructionDirectObjectExpression(info, expression, rootObject)
}

func sourceConstructionAdministrativeCandidateValidationProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if typedPackage == nil || typedPackage.TypesInfo == nil || function == nil ||
		function.Body == nil || len(function.Body.List) != 8 {
		return false
	}
	info := typedPackage.TypesInfo
	candidateObject := sourceConstructionNamedParameterObject(info, function, "candidate")
	if candidateObject == nil || !sourceConstructionFixedSingleFailureBranchProved(
		function.Body.List[0],
		"candidate.row.kind != sourceObservedDirectory && "+
			"candidate.row.kind != sourceObservedRegular",
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)",
	) || !sourceConstructionSingleFailureReturnGuardProved(
		info,
		function.Body.List[1],
		"builder.primitives.validateFilesystem(builder.ctx,candidate.row.evidence.mount)",
	) || !sourceConstructionSingleFailureReturnGuardProved(
		info,
		function.Body.List[2],
		"builder.primitives.validateACL(builder.ctx,candidate.acl)",
	) {
		return false
	}
	evidenceAssignment, ok := function.Body.List[3].(*ast.AssignStmt)
	if !ok || evidenceAssignment.Tok != token.DEFINE || len(evidenceAssignment.Lhs) != 2 ||
		len(evidenceAssignment.Rhs) != 1 {
		return false
	}
	evidence, evidenceOK := evidenceAssignment.Lhs[0].(*ast.Ident)
	failure, failureOK := evidenceAssignment.Lhs[1].(*ast.Ident)
	validation, callOK := evidenceAssignment.Rhs[0].(*ast.CallExpr)
	if !evidenceOK || !failureOK || !callOK || evidence.Name != "evidence" ||
		failure.Name != "failure" || sourceConstructionCallFingerprint(validation) !=
		"validateSourceDescriptorEvidence(builder.ctx,candidate.row.evidence.snapshot,"+
			"candidate.row.evidence.mount,candidate.acl,candidate.row.kind,ownerEffectiveOnly,"+
			"candidate.row.kind == sourceObservedRegular && "+
			"candidate.row.class == sourceAuthorityAndManifest,0)" {
		return false
	}
	evidenceObject := info.Defs[evidence]
	failureObject := info.Defs[failure]
	failureBranch, ok := function.Body.List[4].(*ast.IfStmt)
	if evidenceObject == nil || failureObject == nil || !ok || failureBranch.Init != nil ||
		!sourceConstructionObjectFailureReturnGuardProved(
			info,
			failureBranch,
			failureObject,
		) {
		return false
	}
	evidenceGuard, ok := function.Body.List[5].(*ast.IfStmt)
	if !ok || evidenceGuard.Init != nil || evidenceGuard.Else != nil ||
		len(evidenceGuard.Body.List) != 1 || types.ExprString(evidenceGuard.Cond) !=
		"evidence != candidate.row.evidence" || !sourceConstructionFixedSingleFailureReturnProved(
		evidenceGuard.Body.List[0],
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)",
	) {
		return false
	}
	gitGuard, ok := function.Body.List[6].(*ast.IfStmt)
	if !ok || gitGuard.Init != nil || gitGuard.Else != nil || len(gitGuard.Body.List) != 1 ||
		types.ExprString(gitGuard.Cond) != `candidate.row.path == ".git"` ||
		!sourceConstructionSingleFailureValueReturnProved(gitGuard.Body.List[0], "nil") {
		return false
	}
	returned, ok := function.Body.List[7].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	childValidation, ok := returned.Results[0].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(childValidation) ==
		"validateInitialSourceChild(builder.ctx,builder.owner.git.root.evidence,"+
			"candidate.row.evidence)"
}

func sourceConstructionFixedSingleFailureBranchProved(
	statement ast.Stmt,
	conditionFingerprint string,
	failureFingerprint string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Init == nil && branch.Else == nil &&
		types.ExprString(branch.Cond) == conditionFingerprint &&
		len(branch.Body.List) == 1 && sourceConstructionFixedSingleFailureReturnProved(
		branch.Body.List[0],
		failureFingerprint,
	)
}

func sourceConstructionFixedSingleFailureReturnProved(
	statement ast.Stmt,
	failureFingerprint string,
) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	failure, ok := returned.Results[0].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(failure) == failureFingerprint
}

func sourceConstructionSingleFailureReturnGuardProved(
	info *types.Info,
	statement ast.Stmt,
	callFingerprint string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	assignment, ok := branch.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return false
	}
	failure, ok := assignment.Lhs[0].(*ast.Ident)
	call, callOK := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || !callOK || failure.Name != "failure" ||
		sourceConstructionCallFingerprint(call) != callFingerprint {
		return false
	}
	return sourceConstructionObjectFailureReturnGuardProved(
		info,
		branch,
		info.Defs[failure],
	)
}

func sourceConstructionObjectFailureReturnGuardProved(
	info *types.Info,
	statement ast.Stmt,
	failureObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) != 1 || failureObject == nil ||
		!sourceConstructionObjectNilComparison(info, branch.Cond, failureObject, token.NEQ) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && sourceConstructionDirectObjectExpression(
		info,
		returned.Results[0],
		failureObject,
	)
}

func sourceConstructionSingleFailureValueReturnProved(statement ast.Stmt, value string) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && types.ExprString(returned.Results[0]) == value
}

func sourceConstructionAdministrativeEntryPathProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	file *ast.File,
) bool {
	if function == nil || function.Body == nil || file == nil ||
		len(function.Body.List) != 10 {
		return false
	}
	prefixObject := sourceConstructionNamedParameterObject(
		typedPackage.TypesInfo,
		function,
		"prefix",
	)
	nameObject := sourceConstructionNamedParameterObject(
		typedPackage.TypesInfo,
		function,
		"name",
	)
	if prefixObject == nil || nameObject == nil {
		return false
	}
	assignment, ok := function.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 ||
		len(assignment.Rhs) != 1 {
		return false
	}
	call, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(call) !=
		"sourceAdministrativeChildPath(builder.ctx,prefix,name)" || len(call.Args) != 3 ||
		!sourceConstructionDirectObjectExpression(
			typedPackage.TypesInfo,
			call.Args[1],
			prefixObject,
		) || !sourceConstructionDirectObjectExpression(
		typedPackage.TypesInfo,
		call.Args[2],
		nameObject,
	) {
		return false
	}
	pathIdentifier, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok || pathIdentifier.Name != "path" {
		return false
	}
	pathObject := typedPackage.TypesInfo.Defs[pathIdentifier]
	if pathObject == nil {
		return false
	}
	expectedNameUses := map[string]int{
		"argument:2:sourceAdministrativeChildPath(builder.ctx,prefix,name)":                                 1,
		"argument:2:builder.primitives.probeRelativeKind(builder.ctx,parent,name,sourceInitialWalkPresent)": 1,
		"argument:2:builder.captureOpenedSourceAdministrativeEntry(capture,parent,name,candidate)":          1,
	}
	expectedPrefixUses := map[string]int{
		"argument:1:sourceAdministrativeChildPath(builder.ctx,prefix,name)": 1,
	}
	observedNameUses := make(map[string]int)
	observedPrefixUses := make(map[string]int)
	pathUses := 0
	parents := sourceConstructionParentNodes(file)
	valid := true
	ast.Inspect(function.Body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := typedPackage.TypesInfo.Uses[identifier]
		switch object {
		case nameObject:
			site := sourceConstructionDirectCallArgumentSite(identifier, parents)
			observedNameUses[site]++
			if expectedNameUses[site] == 0 {
				valid = false
			}
		case prefixObject:
			site := sourceConstructionDirectCallArgumentSite(identifier, parents)
			observedPrefixUses[site]++
			if expectedPrefixUses[site] == 0 {
				valid = false
			}
		case pathObject:
			keyValue, ok := parents[identifier].(*ast.KeyValueExpr)
			if !ok || keyValue.Value != identifier {
				valid = false
				return true
			}
			key, keyOK := keyValue.Key.(*ast.Ident)
			if !keyOK || key.Name != "path" {
				valid = false
			} else {
				pathUses++
			}
		}
		return true
	})
	if !valid || pathUses != 1 {
		return false
	}
	for site, want := range expectedNameUses {
		if observedNameUses[site] != want {
			return false
		}
	}
	for site, want := range expectedPrefixUses {
		if observedPrefixUses[site] != want {
			return false
		}
	}
	return sourceConstructionAdministrativeEntryTailProved(
		typedPackage.TypesInfo,
		function,
		nameObject,
		pathObject,
	)
}

func sourceConstructionAdministrativeEntryTailProved(
	info *types.Info,
	function *ast.FuncDecl,
	nameObject types.Object,
	pathObject types.Object,
) bool {
	statements := function.Body.List
	probeAssignment, ok := statements[4].(*ast.AssignStmt)
	if !ok || probeAssignment.Tok != token.DEFINE || len(probeAssignment.Lhs) != 3 {
		return false
	}
	kind, ok := probeAssignment.Lhs[0].(*ast.Ident)
	if !ok || kind.Name != "kind" {
		return false
	}
	kindObject := info.Defs[kind]
	candidateAssignment, ok := statements[7].(*ast.AssignStmt)
	if !ok || candidateAssignment.Tok != token.DEFINE ||
		len(candidateAssignment.Lhs) != 1 || len(candidateAssignment.Rhs) != 1 ||
		kindObject == nil || !sourceConstructionAdministrativeCandidateConstructionProved(
		info,
		candidateAssignment.Rhs[0],
		pathObject,
		kindObject,
	) {
		return false
	}
	candidate, ok := candidateAssignment.Lhs[0].(*ast.Ident)
	if !ok || candidate.Name != "candidate" {
		return false
	}
	candidateObject := info.Defs[candidate]
	branch, ok := statements[8].(*ast.IfStmt)
	if candidateObject == nil || !ok || branch.Init != nil || branch.Else != nil ||
		types.ExprString(branch.Cond) !=
			"kind == sourceObservedSymlink || kind == sourceObservedSpecial" ||
		len(branch.Body.List) != 2 {
		return false
	}
	appendAssignment, ok := branch.Body.List[0].(*ast.AssignStmt)
	if !ok || appendAssignment.Tok != token.ASSIGN ||
		len(appendAssignment.Lhs) != 1 || len(appendAssignment.Rhs) != 1 ||
		types.ExprString(appendAssignment.Lhs[0]) != "capture.candidates" ||
		sourceConstructionExpressionFingerprint(appendAssignment.Rhs[0]) !=
			"append(capture.candidates,candidate)" {
		return false
	}
	trueReturn, ok := branch.Body.List[1].(*ast.ReturnStmt)
	if !ok || len(trueReturn.Results) != 1 || types.ExprString(trueReturn.Results[0]) != "true" {
		return false
	}
	finalReturn, ok := statements[9].(*ast.ReturnStmt)
	if !ok || len(finalReturn.Results) != 1 {
		return false
	}
	opened, ok := finalReturn.Results[0].(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(opened) !=
		"builder.captureOpenedSourceAdministrativeEntry(capture,parent,name,candidate)" ||
		len(opened.Args) != 4 || !sourceConstructionDirectObjectExpression(
		info,
		opened.Args[2],
		nameObject,
	) || !sourceConstructionDirectObjectExpression(info, opened.Args[3], candidateObject) {
		return false
	}
	return sourceConstructionAdministrativeBooleanReturnsProved(
		function,
		trueReturn,
		finalReturn,
	)
}

func sourceConstructionAdministrativeCandidateConstructionProved(
	info *types.Info,
	expression ast.Expr,
	pathObject types.Object,
	kindObject types.Object,
) bool {
	candidate, ok := expression.(*ast.CompositeLit)
	if !ok || sourceConstructionTypeName(info.TypeOf(candidate)) !=
		"sourceAdministrativeCandidate" || len(candidate.Elts) != 1 {
		return false
	}
	rowField, ok := candidate.Elts[0].(*ast.KeyValueExpr)
	rowKey, keyOK := rowField.Key.(*ast.Ident)
	row, rowOK := rowField.Value.(*ast.CompositeLit)
	if !ok || !keyOK || !rowOK || rowKey.Name != "row" ||
		sourceConstructionTypeName(info.TypeOf(row)) != "sourceAdministrativeRow" ||
		len(row.Elts) != 2 {
		return false
	}
	pathField, pathOK := row.Elts[0].(*ast.KeyValueExpr)
	kindField, kindOK := row.Elts[1].(*ast.KeyValueExpr)
	pathKey, pathKeyOK := pathField.Key.(*ast.Ident)
	kindKey, kindKeyOK := kindField.Key.(*ast.Ident)
	return pathOK && kindOK && pathKeyOK && kindKeyOK && pathKey.Name == "path" &&
		kindKey.Name == "kind" && sourceConstructionDirectObjectExpression(
		info,
		pathField.Value,
		pathObject,
	) && sourceConstructionDirectObjectExpression(info, kindField.Value, kindObject)
}

func sourceConstructionAdministrativeBooleanReturnsProved(
	function *ast.FuncDecl,
	allowedTrue *ast.ReturnStmt,
	allowedExpression *ast.ReturnStmt,
) bool {
	valid := true
	ast.Inspect(function.Body, func(node ast.Node) bool {
		returned, ok := node.(*ast.ReturnStmt)
		if !ok || returned == allowedTrue || returned == allowedExpression {
			return true
		}
		if len(returned.Results) != 1 || types.ExprString(returned.Results[0]) != "false" {
			valid = false
		}
		return true
	})
	return valid
}

func sourceConstructionAdministrativeOpenedPathProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
	file *ast.File,
) bool {
	if function == nil || function.Body == nil || file == nil ||
		len(function.Body.List) != 14 {
		return false
	}
	nameObject := sourceConstructionNamedParameterObject(
		typedPackage.TypesInfo,
		function,
		"name",
	)
	candidateObject := sourceConstructionNamedParameterObject(
		typedPackage.TypesInfo,
		function,
		"candidate",
	)
	if nameObject == nil || candidateObject == nil {
		return false
	}
	const nameSite = "argument:2:builder.primitives.openRelativeNoFollow(builder.ctx,parent,name,candidate.row.kind,sourceInitialWalkPresent)"
	expectedCandidateUses := map[string]int{
		"argument:3:builder.primitives.openRelativeNoFollow(builder.ctx,parent,name,candidate.row.kind,sourceInitialWalkPresent)":                     1,
		"argument:1:builder.observeSourceDescriptorWithoutPolicy(descriptor,candidate.row.kind)":                                                      1,
		"binary:candidate.row.kind == sourceObservedRegular":                                                                                          1,
		"argument:1:capture.chargeRegular(builder.ctx,candidate.row.path,observation.evidence.snapshot.size,builder.owner.config.claim.objectFormat)": 1,
		"binary:candidate.row.kind == sourceObservedDirectory":                                                                                        2,
		"assignment:candidate.row.evidence":                                                                                                           1,
		"assignment:candidate.acl":                                                                                                                    1,
		"argument:1:append(capture.candidates,candidate)":                                                                                             1,
		"argument:2:builder.captureSourceAdministrativeDirectory(capture,descriptor,candidate.row.path,observation)":                                  1,
	}
	nameUses := 0
	observedCandidateUses := make(map[string]int)
	valid := true
	parents := sourceConstructionParentNodes(file)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		current, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := typedPackage.TypesInfo.Uses[current]
		if object == candidateObject {
			site := sourceConstructionAdministrativeCandidateUseSite(current, parents)
			observedCandidateUses[site]++
			if expectedCandidateUses[site] == 0 {
				valid = false
			}
			return true
		}
		if object != nameObject {
			return true
		}
		if sourceConstructionDirectCallArgumentSite(current, parents) != nameSite {
			valid = false
		} else {
			nameUses++
		}
		return true
	})
	if !valid || nameUses != 1 {
		return false
	}
	for site, want := range expectedCandidateUses {
		if observedCandidateUses[site] != want {
			return false
		}
	}
	if !sourceConstructionAdministrativeOpenedMiddleProved(
		typedPackage.TypesInfo,
		function,
		candidateObject,
	) {
		return false
	}
	return sourceConstructionAdministrativeOpenedTailProved(
		typedPackage.TypesInfo,
		function,
		candidateObject,
	)
}

func sourceConstructionAdministrativeOpenedMiddleProved(
	info *types.Info,
	function *ast.FuncDecl,
	candidateObject types.Object,
) bool {
	statements := function.Body.List
	descriptorAssignment, ok := statements[0].(*ast.AssignStmt)
	if !ok || descriptorAssignment.Tok != token.DEFINE ||
		len(descriptorAssignment.Lhs) != 2 || len(descriptorAssignment.Rhs) != 1 {
		return false
	}
	descriptor, descriptorOK := descriptorAssignment.Lhs[0].(*ast.Ident)
	openFailure, failureOK := descriptorAssignment.Lhs[1].(*ast.Ident)
	openCall, callOK := descriptorAssignment.Rhs[0].(*ast.CallExpr)
	if !descriptorOK || !failureOK || !callOK || descriptor.Name != "descriptor" ||
		openFailure.Name != "openFailure" || sourceConstructionCallFingerprint(openCall) !=
		"builder.primitives.openRelativeNoFollow(builder.ctx,parent,name,"+
			"candidate.row.kind,sourceInitialWalkPresent)" {
		return false
	}
	descriptorObject := info.Defs[descriptor]
	openFailureObject := info.Defs[openFailure]
	if descriptorObject == nil || openFailureObject == nil ||
		!sourceConstructionOpenedNilDescriptorFailureProved(
			info,
			statements[1],
			descriptorObject,
			openFailureObject,
		) || !sourceConstructionOpenedOwnedFailureProved(
		info,
		statements[2],
		descriptorObject,
		openFailureObject,
	) {
		return false
	}

	observationAssignment, ok := statements[3].(*ast.AssignStmt)
	if !ok || observationAssignment.Tok != token.DEFINE ||
		len(observationAssignment.Lhs) != 2 || len(observationAssignment.Rhs) != 1 {
		return false
	}
	observation, observationOK := observationAssignment.Lhs[0].(*ast.Ident)
	failure, failureOK := observationAssignment.Lhs[1].(*ast.Ident)
	observe, callOK := observationAssignment.Rhs[0].(*ast.CallExpr)
	if !observationOK || !failureOK || !callOK || observation.Name != "observation" ||
		failure.Name != "failure" || sourceConstructionCallFingerprint(observe) !=
		"builder.observeSourceDescriptorWithoutPolicy(descriptor,candidate.row.kind)" ||
		len(observe.Args) != 2 || !sourceConstructionDirectObjectExpression(
		info,
		observe.Args[0],
		descriptorObject,
	) || !sourceConstructionSelectorChainProved(
		info,
		observe.Args[1],
		candidateObject,
		"row",
		"kind",
	) {
		return false
	}
	observationObject := info.Defs[observation]
	failureObject := info.Defs[failure]
	if observationObject == nil || failureObject == nil ||
		!sourceConstructionAdministrativeObservationGuardProved(
			info,
			statements[4],
			failureObject,
			"failure == nil && candidate.row.kind == sourceObservedRegular",
			"capture.chargeRegular(builder.ctx,candidate.row.path,"+
				"observation.evidence.snapshot.size,builder.owner.config.claim.objectFormat)",
		) || !sourceConstructionAdministrativeObservationGuardProved(
		info,
		statements[5],
		failureObject,
		"failure == nil && candidate.row.kind == sourceObservedDirectory",
		"validateInitialSourceChild(builder.ctx,builder.owner.git.root.evidence,"+
			"observation.evidence)",
	) || !sourceConstructionAdministrativeObservationFailureProved(
		info,
		statements[6],
		failureObject,
		descriptorObject,
	) {
		return false
	}
	if !sourceConstructionSelectorAssignmentProved(
		info,
		statements[7],
		candidateObject,
		[]string{"row", "evidence"},
		observationObject,
		[]string{"evidence"},
	) || !sourceConstructionSelectorAssignmentProved(
		info,
		statements[8],
		candidateObject,
		[]string{"acl"},
		observationObject,
		[]string{"acl"},
	) {
		return false
	}
	appendAssignment, ok := statements[9].(*ast.AssignStmt)
	if !ok || appendAssignment.Tok != token.ASSIGN || len(appendAssignment.Lhs) != 1 ||
		len(appendAssignment.Rhs) != 1 || types.ExprString(appendAssignment.Lhs[0]) !=
		"capture.candidates" {
		return false
	}
	appendCall, ok := appendAssignment.Rhs[0].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(appendCall) ==
		"append(capture.candidates,candidate)" && len(appendCall.Args) == 2 &&
		sourceConstructionDirectObjectExpression(info, appendCall.Args[1], candidateObject)
}

func sourceConstructionOpenedNilDescriptorFailureProved(
	info *types.Info,
	statement ast.Stmt,
	descriptorObject types.Object,
	openFailureObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 3 ||
		!sourceConstructionObjectNilComparison(info, branch.Cond, descriptorObject, token.EQL) {
		return false
	}
	repair, ok := branch.Body.List[0].(*ast.IfStmt)
	if !ok || repair.Init != nil || repair.Else != nil || len(repair.Body.List) != 1 ||
		!sourceConstructionObjectNilComparison(
			info,
			repair.Cond,
			openFailureObject,
			token.EQL,
		) {
		return false
	}
	assignment, ok := repair.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || !sourceConstructionDirectObjectExpression(
		info,
		assignment.Lhs[0],
		openFailureObject,
	) {
		return false
	}
	failure, ok := assignment.Rhs[0].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(failure) ==
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)" &&
		sourceConstructionAdministrativeAddPrimitiveStatementProved(
			info,
			branch.Body.List[1],
			openFailureObject,
		) && sourceConstructionSingleBooleanReturnProved(branch.Body.List[2], "false")
}

func sourceConstructionOpenedOwnedFailureProved(
	info *types.Info,
	statement ast.Stmt,
	descriptorObject types.Object,
	openFailureObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 3 ||
		!sourceConstructionObjectNilComparison(
			info,
			branch.Cond,
			openFailureObject,
			token.NEQ,
		) || !sourceConstructionAdministrativeAddPrimitiveStatementProved(
		info,
		branch.Body.List[0],
		openFailureObject,
	) {
		return false
	}
	closeExpression, ok := branch.Body.List[1].(*ast.ExprStmt)
	if !ok {
		return false
	}
	closeCall, ok := closeExpression.X.(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(closeCall) ==
		"builder.closeTransientDescriptor(descriptor)" && len(closeCall.Args) == 1 &&
		sourceConstructionDirectObjectExpression(info, closeCall.Args[0], descriptorObject) &&
		sourceConstructionSingleBooleanReturnProved(branch.Body.List[2], "false")
}

func sourceConstructionAdministrativeAddPrimitiveStatementProved(
	info *types.Info,
	statement ast.Stmt,
	failureObject types.Object,
) bool {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	return ok && types.ExprString(call.Fun) == "builder.outcome.addPrimitive" &&
		len(call.Args) == 1 && sourceConstructionDirectObjectExpression(
		info,
		call.Args[0],
		failureObject,
	)
}

func sourceConstructionAdministrativeObservationGuardProved(
	info *types.Info,
	statement ast.Stmt,
	failureObject types.Object,
	conditionFingerprint string,
	callFingerprint string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 ||
		types.ExprString(branch.Cond) != conditionFingerprint {
		return false
	}
	condition, ok := sourceConstructionUnparenthesizedExpression(branch.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.LAND || !sourceConstructionObjectNilComparison(
		info,
		condition.X,
		failureObject,
		token.EQL,
	) {
		return false
	}
	assignment, ok := branch.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || !sourceConstructionDirectObjectExpression(
		info,
		assignment.Lhs[0],
		failureObject,
	) {
		return false
	}
	call, ok := assignment.Rhs[0].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(call) == callFingerprint
}

func sourceConstructionAdministrativeObservationFailureProved(
	info *types.Info,
	statement ast.Stmt,
	failureObject types.Object,
	descriptorObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 3 ||
		!sourceConstructionObjectNilComparison(info, branch.Cond, failureObject, token.NEQ) {
		return false
	}
	addExpression, ok := branch.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	addCall, ok := addExpression.X.(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(addCall) !=
		"builder.outcome.addPrimitive(failure)" || len(addCall.Args) != 1 ||
		!sourceConstructionDirectObjectExpression(info, addCall.Args[0], failureObject) {
		return false
	}
	closeExpression, ok := branch.Body.List[1].(*ast.ExprStmt)
	if !ok {
		return false
	}
	closeCall, ok := closeExpression.X.(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(closeCall) ==
		"builder.closeTransientDescriptor(descriptor)" && len(closeCall.Args) == 1 &&
		sourceConstructionDirectObjectExpression(info, closeCall.Args[0], descriptorObject) &&
		sourceConstructionSingleBooleanReturnProved(branch.Body.List[2], "false")
}

func sourceConstructionSelectorAssignmentProved(
	info *types.Info,
	statement ast.Stmt,
	leftRoot types.Object,
	leftFields []string,
	rightRoot types.Object,
	rightFields []string,
) bool {
	assignment, ok := statement.(*ast.AssignStmt)
	return ok && assignment.Tok == token.ASSIGN && len(assignment.Lhs) == 1 &&
		len(assignment.Rhs) == 1 && sourceConstructionSelectorChainProved(
		info,
		assignment.Lhs[0],
		leftRoot,
		leftFields...,
	) && sourceConstructionSelectorChainProved(
		info,
		assignment.Rhs[0],
		rightRoot,
		rightFields...,
	)
}

func sourceConstructionAdministrativeOpenedTailProved(
	info *types.Info,
	function *ast.FuncDecl,
	candidateObject types.Object,
) bool {
	statements := function.Body.List
	capturedAssignment, ok := statements[10].(*ast.AssignStmt)
	if !ok || capturedAssignment.Tok != token.DEFINE ||
		len(capturedAssignment.Lhs) != 1 || len(capturedAssignment.Rhs) != 1 ||
		types.ExprString(capturedAssignment.Rhs[0]) != "true" {
		return false
	}
	captured, ok := capturedAssignment.Lhs[0].(*ast.Ident)
	if !ok || captured.Name != "captured" {
		return false
	}
	capturedObject := info.Defs[captured]
	recursion, ok := statements[11].(*ast.IfStmt)
	if capturedObject == nil || !ok || recursion.Init != nil || recursion.Else != nil ||
		types.ExprString(recursion.Cond) !=
			"candidate.row.kind == sourceObservedDirectory" || len(recursion.Body.List) != 1 {
		return false
	}
	recurseAssignment, ok := recursion.Body.List[0].(*ast.AssignStmt)
	if !ok || recurseAssignment.Tok != token.ASSIGN ||
		len(recurseAssignment.Lhs) != 1 || len(recurseAssignment.Rhs) != 1 ||
		!sourceConstructionDirectObjectExpression(
			info,
			recurseAssignment.Lhs[0],
			capturedObject,
		) {
		return false
	}
	recurse, ok := recurseAssignment.Rhs[0].(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(recurse) !=
		"builder.captureSourceAdministrativeDirectory(capture,descriptor,candidate.row.path,observation)" {
		return false
	}
	closeAssignment, ok := statements[12].(*ast.AssignStmt)
	if !ok || closeAssignment.Tok != token.DEFINE || len(closeAssignment.Lhs) != 1 ||
		len(closeAssignment.Rhs) != 1 {
		return false
	}
	closeFailed, ok := closeAssignment.Lhs[0].(*ast.Ident)
	closeCall, callOK := closeAssignment.Rhs[0].(*ast.CallExpr)
	if !ok || !callOK || closeFailed.Name != "closeFailed" ||
		sourceConstructionCallFingerprint(closeCall) !=
			"builder.closeTransientDescriptor(descriptor)" {
		return false
	}
	closeFailedObject := info.Defs[closeFailed]
	finalReturn, ok := statements[13].(*ast.ReturnStmt)
	if closeFailedObject == nil || !ok || len(finalReturn.Results) != 1 {
		return false
	}
	result, ok := sourceConstructionUnparenthesizedExpression(
		finalReturn.Results[0],
	).(*ast.BinaryExpr)
	if !ok || result.Op != token.LAND || !sourceConstructionDirectObjectExpression(
		info,
		result.X,
		capturedObject,
	) {
		return false
	}
	notClosed, ok := sourceConstructionUnparenthesizedExpression(result.Y).(*ast.UnaryExpr)
	if !ok || notClosed.Op != token.NOT || !sourceConstructionDirectObjectExpression(
		info,
		notClosed.X,
		closeFailedObject,
	) {
		return false
	}
	return sourceConstructionAdministrativeBooleanReturnsProved(function, nil, finalReturn)
}

func sourceConstructionAdministrativeDirectoryDispatchProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if function == nil || function.Body == nil ||
		len(function.Body.List) != 6 {
		return false
	}
	info := typedPackage.TypesInfo
	captureObject := sourceConstructionNamedParameterObject(info, function, "capture")
	descriptorObject := sourceConstructionNamedParameterObject(info, function, "descriptor")
	beforeObject := sourceConstructionNamedParameterObject(info, function, "before")
	prefixObject := sourceConstructionNamedParameterObject(
		info,
		function,
		"prefix",
	)
	if captureObject == nil || descriptorObject == nil || beforeObject == nil ||
		prefixObject == nil {
		return false
	}
	initial, ok := function.Body.List[0].(*ast.IfStmt)
	if !ok || initial.Init != nil || initial.Else != nil ||
		types.ExprString(initial.Cond) !=
			`capture == nil || descriptor == nil || prefix == "" || descriptor.kind != sourceObservedDirectory` ||
		len(initial.Body.List) != 2 {
		return false
	}
	loop, ok := function.Body.List[1].(*ast.ForStmt)
	if !ok || loop.Init != nil || loop.Cond != nil || loop.Post != nil ||
		len(loop.Body.List) != 4 {
		return false
	}
	batch, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || batch.Tok != token.DEFINE || len(batch.Lhs) != 3 || len(batch.Rhs) != 1 {
		return false
	}
	batchCall, ok := batch.Rhs[0].(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(batchCall) !=
		"builder.primitives.readDirectoryBatch(builder.ctx,descriptor)" {
		return false
	}
	names, namesOK := batch.Lhs[0].(*ast.Ident)
	terminal, terminalOK := batch.Lhs[1].(*ast.Ident)
	failure, failureOK := batch.Lhs[2].(*ast.Ident)
	if !namesOK || !terminalOK || !failureOK || names.Name != "names" ||
		terminal.Name != "terminal" || failure.Name != "failure" {
		return false
	}
	namesObject := info.Defs[names]
	terminalObject := info.Defs[terminal]
	failureObject := info.Defs[failure]
	if namesObject == nil || terminalObject == nil || failureObject == nil ||
		!sourceConstructionAdministrativeOutcomeFailureBranchProved(
			info,
			loop.Body.List[1],
			failureObject,
		) {
		return false
	}
	dispatch, ok := loop.Body.List[2].(*ast.RangeStmt)
	if !ok || dispatch.Tok != token.DEFINE || len(dispatch.Body.List) != 1 ||
		!sourceConstructionDirectObjectExpression(info, dispatch.X, namesObject) {
		return false
	}
	blank, blankOK := dispatch.Key.(*ast.Ident)
	rangeName, nameOK := dispatch.Value.(*ast.Ident)
	if !blankOK || !nameOK || blank.Name != "_" || rangeName.Name != "name" {
		return false
	}
	rangeNameObject := info.Defs[rangeName]
	dispatchFailure, ok := dispatch.Body.List[0].(*ast.IfStmt)
	if rangeNameObject == nil || !ok || dispatchFailure.Init != nil ||
		dispatchFailure.Else != nil || len(dispatchFailure.Body.List) != 1 {
		return false
	}
	negated, ok := sourceConstructionUnparenthesizedExpression(
		dispatchFailure.Cond,
	).(*ast.UnaryExpr)
	if !ok || negated.Op != token.NOT {
		return false
	}
	dispatchCall, ok := sourceConstructionUnparenthesizedExpression(negated.X).(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(dispatchCall) !=
		"builder.captureSourceAdministrativeEntry(capture,descriptor,prefix,name)" ||
		len(dispatchCall.Args) != 4 || !sourceConstructionDirectObjectExpression(
		info,
		dispatchCall.Args[0],
		captureObject,
	) || !sourceConstructionDirectObjectExpression(
		info,
		dispatchCall.Args[1],
		descriptorObject,
	) || !sourceConstructionDirectObjectExpression(
		info,
		dispatchCall.Args[2],
		prefixObject,
	) || !sourceConstructionDirectObjectExpression(
		info,
		dispatchCall.Args[3],
		rangeNameObject,
	) || !sourceConstructionSingleBooleanReturnProved(
		dispatchFailure.Body.List[0],
		"false",
	) {
		return false
	}
	terminalBranch, ok := loop.Body.List[3].(*ast.IfStmt)
	if !ok || terminalBranch.Init != nil || terminalBranch.Else != nil ||
		!sourceConstructionDirectObjectExpression(info, terminalBranch.Cond, terminalObject) ||
		len(terminalBranch.Body.List) != 1 {
		return false
	}
	broken, ok := terminalBranch.Body.List[0].(*ast.BranchStmt)
	if !ok || broken.Tok != token.BREAK || broken.Label != nil {
		return false
	}
	afterAssignment, ok := function.Body.List[2].(*ast.AssignStmt)
	if !ok || afterAssignment.Tok != token.DEFINE || len(afterAssignment.Lhs) != 2 ||
		len(afterAssignment.Rhs) != 1 {
		return false
	}
	after, afterOK := afterAssignment.Lhs[0].(*ast.Ident)
	postFailure, failureOK := afterAssignment.Lhs[1].(*ast.Ident)
	reobserve, callOK := afterAssignment.Rhs[0].(*ast.CallExpr)
	if !afterOK || !failureOK || !callOK || after.Name != "after" ||
		postFailure.Name != "failure" || sourceConstructionCallFingerprint(reobserve) !=
		"builder.observeSourceDescriptorWithoutPolicy(descriptor,sourceObservedDirectory)" ||
		len(reobserve.Args) != 2 || !sourceConstructionDirectObjectExpression(
		info,
		reobserve.Args[0],
		descriptorObject,
	) {
		return false
	}
	afterObject := info.Defs[after]
	postFailureObject := info.Defs[postFailure]
	compareBranch, ok := function.Body.List[3].(*ast.IfStmt)
	if afterObject == nil || postFailureObject == nil || !ok || compareBranch.Init != nil ||
		compareBranch.Else != nil || len(compareBranch.Body.List) != 1 ||
		!sourceConstructionObjectNilComparison(
			info,
			compareBranch.Cond,
			postFailureObject,
			token.EQL,
		) {
		return false
	}
	compareAssignment, ok := compareBranch.Body.List[0].(*ast.AssignStmt)
	if !ok || compareAssignment.Tok != token.ASSIGN ||
		len(compareAssignment.Lhs) != 1 || len(compareAssignment.Rhs) != 1 ||
		!sourceConstructionDirectObjectExpression(
			info,
			compareAssignment.Lhs[0],
			postFailureObject,
		) {
		return false
	}
	compare, ok := compareAssignment.Rhs[0].(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(compare) !=
		"compareSourceDescriptorEvidence(builder.ctx,before.evidence,after.evidence)" ||
		len(compare.Args) != 3 || !sourceConstructionSelectorChainProved(
		info,
		compare.Args[1],
		beforeObject,
		"evidence",
	) || !sourceConstructionSelectorChainProved(
		info,
		compare.Args[2],
		afterObject,
		"evidence",
	) || !sourceConstructionAdministrativeOutcomeFailureBranchProved(
		info,
		function.Body.List[4],
		postFailureObject,
	) {
		return false
	}
	finalReturn, ok := function.Body.List[5].(*ast.ReturnStmt)
	if !ok || len(finalReturn.Results) != 1 || types.ExprString(finalReturn.Results[0]) != "true" ||
		!sourceConstructionAdministrativeBooleanReturnsProved(function, finalReturn, nil) {
		return false
	}
	wantUses := map[types.Object]int{
		prefixObject:    2,
		namesObject:     1,
		rangeNameObject: 1,
	}
	gotUses := make(map[types.Object]int)
	for identifier, object := range info.Uses {
		if identifier.Pos() < function.Pos() || identifier.End() > function.End() {
			continue
		}
		if wantUses[object] != 0 {
			gotUses[object]++
		}
	}
	for object, want := range wantUses {
		if gotUses[object] != want {
			return false
		}
	}
	return true
}

func sourceConstructionAdministrativeOutcomeFailureBranchProved(
	info *types.Info,
	statement ast.Stmt,
	failureObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 2 ||
		!sourceConstructionObjectNilComparison(info, branch.Cond, failureObject, token.NEQ) {
		return false
	}
	expression, ok := branch.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(call) ==
		"builder.outcome.addPrimitive(failure)" && len(call.Args) == 1 &&
		sourceConstructionDirectObjectExpression(info, call.Args[0], failureObject) &&
		sourceConstructionSingleBooleanReturnProved(branch.Body.List[1], "false")
}

func sourceConstructionSingleBooleanReturnProved(statement ast.Stmt, value string) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && types.ExprString(returned.Results[0]) == value
}

func sourceConstructionAdministrativeCandidateUseSite(
	identifier *ast.Ident,
	parents map[ast.Node]ast.Node,
) string {
	if identifier == nil || parents == nil {
		return "<unresolved>"
	}
	expression := ast.Expr(identifier)
	for {
		selector, ok := parents[expression].(*ast.SelectorExpr)
		if !ok || selector.X != expression {
			break
		}
		expression = selector
	}
	parent := parents[expression]
	if call, ok := parent.(*ast.CallExpr); ok {
		for index, argument := range call.Args {
			if sourceConstructionDirectExpression(argument, expression) {
				return "argument:" + strconv.Itoa(index) + ":" +
					sourceConstructionCallFingerprint(call)
			}
		}
	}
	if binary, ok := parent.(*ast.BinaryExpr); ok {
		return "binary:" + types.ExprString(binary)
	}
	if assignment, ok := parent.(*ast.AssignStmt); ok {
		for _, target := range assignment.Lhs {
			if sourceConstructionDirectExpression(target, expression) {
				return "assignment:" + types.ExprString(expression)
			}
		}
	}
	return fmt.Sprintf("%T:%s", parent, types.ExprString(expression))
}

func sourceConstructionNamedParameterObject(
	info *types.Info,
	function *ast.FuncDecl,
	name string,
) types.Object {
	if info == nil || function == nil || function.Type.Params == nil {
		return nil
	}
	var found types.Object
	for _, field := range function.Type.Params.List {
		for _, identifier := range field.Names {
			if identifier.Name != name {
				continue
			}
			if found != nil {
				return nil
			}
			found = info.Defs[identifier]
		}
	}
	return found
}

func sourceConstructionDirectCallArgumentSite(
	identifier *ast.Ident,
	parents map[ast.Node]ast.Node,
) string {
	if identifier == nil || parents == nil {
		return "<unresolved>"
	}
	call, ok := parents[identifier].(*ast.CallExpr)
	if !ok {
		return "<non-call>"
	}
	for index, argument := range call.Args {
		if sourceConstructionDirectExpression(argument, identifier) {
			return "argument:" + strconv.Itoa(index) + ":" +
				sourceConstructionCallFingerprint(call)
		}
	}
	return "<nested-call>"
}

func sourceConstructionAdministrativeRowMutationViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for administrative row mutation audit"}
	}
	allowedComposites := map[string]int{
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|sourceAdministrativeCandidate": 1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).retainSourceAdministrativeInventory|sourceAdministrativeRow":       1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeEntry|sourceAdministrativeCandidate":    1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureSourceAdministrativeEntry|sourceAdministrativeRow":          1,
	}
	allowedAssignments := map[string]int{
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|row.evidence|candidate.row.evidence=observation.evidence":                                                       1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).captureOpenedSourceAdministrativeEntry|candidate.acl|candidate.acl=observation.acl":                                                                    1,
		"source_construction_inventory.go|classifySourceAdministrativeCandidates|row.class|candidate.row.class=classifySourceAdministrativePath(candidate.row.path,sourceAdministrativeEntryKind(candidate.row.kind),format)": 1,
		"source_construction_inventory.go|(*sourceAdministrativeInventory).validateRows|sourceAdministrativeRow|fixed.git=row":                                                                                                1,
		"source_construction_inventory.go|(*sourceAdministrativeInventory).validateRows|sourceAdministrativeRow|fixed.config=row":                                                                                             1,
		"source_construction_inventory.go|(*sourceAdministrativeInventory).validateRows|sourceAdministrativeRow|fixed.objects=row":                                                                                            1,
		"source_construction_inventory.go|(*sourceAdministrativeInventory).validateRows|sourceAdministrativeRow|fixed.packedRefs=row":                                                                                         1,
		"source_construction_inventory.go|(*sourceConstructionBuilder).finishSourceAdministrativeInventory|sourceAdministrativeRow|rows[index]=capture.candidates[index].row":                                                 1,
	}
	allowedAddresses := map[string]int{
		"source_construction_inventory.go|classifySourceAdministrativeCandidates|sourceAdministrativeCandidate|&candidates[index]":      1,
		"source_construction_inventory.go|(*sourceAdministrativeInventory).validateRows|sourceAdministrativeRow|&inventory.rows[index]": 1,
	}
	observedComposites := make(map[string]int)
	observedAssignments := make(map[string]int)
	observedAddresses := make(map[string]int)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
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
			case *ast.CompositeLit:
				typeName := sourceConstructionTypeName(typedPackage.TypesInfo.TypeOf(current))
				if typeName != "sourceAdministrativeCandidate" &&
					typeName != "sourceAdministrativeRow" {
					return true
				}
				site := filename + "|" + role + "|" + typeName
				observedComposites[site]++
				if allowedComposites[site] == 0 {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source administrative row/candidate composite is outside audited set "+site,
					)
				}
			case *ast.AssignStmt:
				for _, target := range current.Lhs {
					kind := sourceConstructionAdministrativeMutationTarget(
						typedPackage.TypesInfo,
						target,
						current.Tok == token.DEFINE,
					)
					if kind == "" {
						continue
					}
					site := filename + "|" + role + "|" + kind + "|" +
						types.ExprString(target) + "=" + sourceConstructionAssignmentValue(current, target)
					observedAssignments[site]++
					if allowedAssignments[site] == 0 {
						violations = append(violations,
							typedPackage.Fset.Position(target.Pos()).String()+
								" source administrative row/candidate mutation is outside audited set "+site,
						)
					}
				}
			case *ast.IncDecStmt:
				kind := sourceConstructionAdministrativeMutationTarget(
					typedPackage.TypesInfo,
					current.X,
					false,
				)
				if kind != "" {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source administrative row/candidate increment is forbidden "+
							filename+"|"+role+"|"+kind,
					)
				}
			case *ast.RangeStmt:
				if current.Tok != token.ASSIGN {
					return true
				}
				for _, target := range []ast.Expr{current.Key, current.Value} {
					kind := sourceConstructionAdministrativeMutationTarget(
						typedPackage.TypesInfo,
						target,
						false,
					)
					if kind == "" {
						continue
					}
					violations = append(violations,
						typedPackage.Fset.Position(target.Pos()).String()+
							" source administrative row/candidate range mutation is forbidden "+
							filename+"|"+role+"|"+kind,
					)
				}
			case *ast.CallExpr:
				called := sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun)
				if len(current.Args) == 0 ||
					(called != types.Universe.Lookup("copy") &&
						called != types.Universe.Lookup("clear")) ||
					!sourceConstructionAdministrativeInventoryRootedExpression(
						typedPackage.TypesInfo,
						current.Args[0],
					) {
					return true
				}
				violations = append(violations,
					typedPackage.Fset.Position(current.Pos()).String()+
						" source administrative inventory write builtin is forbidden "+
						filename+"|"+role+"|"+sourceConstructionCallFingerprint(current),
				)
			case *ast.UnaryExpr:
				if current.Op != token.AND {
					return true
				}
				typeName := sourceConstructionTypeName(typedPackage.TypesInfo.TypeOf(current.X))
				kind := sourceConstructionAdministrativeMutationRootKind(
					typedPackage.TypesInfo,
					current.X,
				)
				if kind == "" {
					return true
				}
				site := filename + "|" + role + "|" + typeName + "|" +
					types.ExprString(current)
				observedAddresses[site]++
				if allowedAddresses[site] == 0 {
					violations = append(violations,
						typedPackage.Fset.Position(current.Pos()).String()+
							" source administrative row/candidate address escape is outside audited set "+site,
					)
				}
			}
			return true
		})
	}
	for site, want := range allowedComposites {
		if got := observedComposites[site]; got != want {
			violations = append(violations,
				"audited source administrative composite "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range allowedAssignments {
		if got := observedAssignments[site]; got != want {
			violations = append(violations,
				"audited source administrative mutation "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	for site, want := range allowedAddresses {
		if got := observedAddresses[site]; got != want {
			violations = append(violations,
				"audited source administrative address "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	slices.Sort(violations)
	return violations
}

func sourceConstructionAdministrativeMutationTarget(
	info *types.Info,
	target ast.Expr,
	definition bool,
) string {
	if info == nil || target == nil {
		return ""
	}
	if definition {
		if _, identifier := sourceConstructionUnparenthesizedExpression(target).(*ast.Ident); identifier {
			return ""
		}
	}
	if sourceConstructionAdministrativeInventoryRootedExpression(info, target) {
		return "sourceAdministrativeInventoryDescendant"
	}
	selector, ok := sourceConstructionUnparenthesizedExpression(target).(*ast.SelectorExpr)
	if ok {
		receiverName := sourceConstructionTypeName(info.TypeOf(selector.X))
		if receiverName == "sourceAdministrativeCandidate" {
			return "candidate." + selector.Sel.Name
		}
		if receiverName == "sourceAdministrativeRow" {
			return "row." + selector.Sel.Name
		}
	}
	typeName := sourceConstructionTypeName(info.TypeOf(target))
	if typeName == "sourceAdministrativeCandidate" || typeName == "sourceAdministrativeRow" {
		return typeName
	}
	if sourceConstructionAdministrativeMutationRootKind(info, target) != "" {
		return "sourceAdministrativeDescendant"
	}
	return ""
}

func sourceConstructionAdministrativeInventoryRootedExpression(
	info *types.Info,
	expression ast.Expr,
) bool {
	for expression != nil {
		expression = sourceConstructionUnparenthesizedExpression(expression)
		switch current := expression.(type) {
		case *ast.Ident:
			object := info.Uses[current]
			if object == nil {
				object = info.Defs[current]
			}
			return sourceConstructionTypeName(sourceConstructionObjectType(object)) ==
				"sourceAdministrativeInventory"
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
			return false
		}
	}
	return false
}

func sourceConstructionAdministrativeMutationRootKind(
	info *types.Info,
	target ast.Expr,
) string {
	for target != nil {
		target = sourceConstructionUnparenthesizedExpression(target)
		switch sourceConstructionTypeName(info.TypeOf(target)) {
		case "sourceAdministrativeCandidate":
			return "sourceAdministrativeCandidate"
		case "sourceAdministrativeRow":
			return "sourceAdministrativeRow"
		}
		switch current := target.(type) {
		case *ast.SelectorExpr:
			target = current.X
		case *ast.IndexExpr:
			target = current.X
		case *ast.IndexListExpr:
			target = current.X
		case *ast.StarExpr:
			target = current.X
		default:
			return ""
		}
	}
	return ""
}

func sourceConstructionAssignmentValue(assignment *ast.AssignStmt, target ast.Expr) string {
	if assignment == nil {
		return "<unresolved>"
	}
	for index, candidate := range assignment.Lhs {
		if candidate != target {
			continue
		}
		if index < len(assignment.Rhs) {
			return sourceConstructionExpressionFingerprint(assignment.Rhs[index])
		}
		if len(assignment.Rhs) == 1 {
			return sourceConstructionExpressionFingerprint(assignment.Rhs[0])
		}
	}
	return "<tuple>"
}

func sourceConstructionExpressionFingerprint(expression ast.Expr) string {
	if call, ok := expression.(*ast.CallExpr); ok {
		return sourceConstructionCallFingerprint(call)
	}
	return types.ExprString(expression)
}

func sourceConstructionCallableRole(function *types.Func, packagePath string) string {
	if function == nil {
		return "<unresolved>"
	}
	packageRole := "<builtin>"
	if function.Pkg() != nil {
		packageRole = function.Pkg().Path()
	}
	return packageRole + "|" + sourceConstructionFunctionRole(function, packagePath)
}

func sourceConstructionCallFingerprint(call *ast.CallExpr) string {
	if call == nil {
		return "<nil-call>"
	}
	arguments := make([]string, 0, len(call.Args))
	for _, argument := range call.Args {
		arguments = append(arguments, types.ExprString(argument))
	}
	if call.Ellipsis.IsValid() && len(arguments) != 0 {
		arguments[len(arguments)-1] += "..."
	}
	return types.ExprString(call.Fun) + "(" + strings.Join(arguments, ",") + ")"
}

func sourceConstructionSealedDispatch(function *types.Func, packagePath string) bool {
	if function == nil || function.Pkg() == nil || function.Pkg().Path() != packagePath {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return false
	}
	receiver := types.Unalias(signature.Recv().Type())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	named, _ := receiver.(*types.Named)
	if named == nil || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != packagePath {
		return false
	}
	return named.Obj().Name() == "sourcePrimitives" ||
		named.Obj().Name() == "sourceHandleCloser"
}

func sourceConstructionDarwinDirectoryReadTopologyViolations(
	typedPackage *packages.Package,
) []string {
	const wantedRole = "(darwinSourcePrimitives).readDirectoryBatch"
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for Darwin directory-read topology audit"}
	}
	var declaration *ast.FuncDecl
	for _, file := range typedPackage.Syntax {
		if filepath.Base(typedPackage.Fset.Position(file.Package).Filename) !=
			"source_primitives_darwin.go" {
			continue
		}
		for _, candidate := range file.Decls {
			function, ok := candidate.(*ast.FuncDecl)
			if !ok || sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			) != wantedRole {
				continue
			}
			if declaration != nil {
				return []string{"Darwin directory-read topology has multiple readDirectoryBatch methods"}
			}
			declaration = function
		}
	}
	if declaration == nil || declaration.Body == nil || len(declaration.Body.List) < 3 {
		return []string{"Darwin directory-read result does not flow directly into normalization"}
	}
	statements := declaration.Body.List[len(declaration.Body.List)-3:]
	assignment, assignmentOK := statements[0].(*ast.AssignStmt)
	keepAliveStatement, keepAliveOK := statements[1].(*ast.ExprStmt)
	returnStatement, returnOK := statements[2].(*ast.ReturnStmt)
	if !assignmentOK || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 ||
		len(assignment.Rhs) != 1 || !keepAliveOK || !returnOK ||
		len(returnStatement.Results) != 1 {
		return []string{"Darwin directory-read result does not flow directly into normalization"}
	}
	entries, entriesOK := assignment.Lhs[0].(*ast.Ident)
	readErr, readErrOK := assignment.Lhs[1].(*ast.Ident)
	readCall, readOK := assignment.Rhs[0].(*ast.CallExpr)
	keepAlive, keepAliveCallOK := keepAliveStatement.X.(*ast.CallExpr)
	normalize, normalizeOK := returnStatement.Results[0].(*ast.CallExpr)
	if !entriesOK || !readErrOK || !readOK || !keepAliveCallOK || !normalizeOK ||
		sourceConstructionCallFingerprint(readCall) !=
			"owner.file.ReadDir(sourceDirectoryReadBatchSize)" ||
		sourceConstructionCallFingerprint(keepAlive) != "runtime.KeepAlive(owner.file)" ||
		sourceConstructionCallFingerprint(normalize) !=
			"normalizeDarwinSourceDirectoryBatch(ctx,entries,err)" {
		return []string{"Darwin directory-read result does not flow directly into normalization"}
	}
	entriesObject := typedPackage.TypesInfo.Defs[entries]
	readErrObject := typedPackage.TypesInfo.Defs[readErr]
	if entriesObject == nil || readErrObject == nil || len(normalize.Args) != 3 ||
		!sourceConstructionDirectObjectExpression(
			typedPackage.TypesInfo,
			normalize.Args[1],
			entriesObject,
		) || !sourceConstructionDirectObjectExpression(
		typedPackage.TypesInfo,
		normalize.Args[2],
		readErrObject,
	) {
		return []string{"Darwin directory-read result does not flow directly into normalization"}
	}
	entriesUses := 0
	readErrUses := 0
	for identifier, object := range typedPackage.TypesInfo.Uses {
		if identifier.Pos() < declaration.Pos() || identifier.End() > declaration.End() {
			continue
		}
		switch object {
		case entriesObject:
			entriesUses++
		case readErrObject:
			readErrUses++
		}
	}
	if entriesUses != 1 || readErrUses != 1 {
		return []string{"Darwin directory-read result does not flow directly into normalization"}
	}
	return nil
}

func sourceConstructionDarwinDirectoryNameFlowViolations(
	typedPackage *packages.Package,
) []string {
	const wantedRole = "normalizeDarwinSourceDirectoryBatch"
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for directory name-flow audit"}
	}
	var function *ast.FuncDecl
	for _, file := range typedPackage.Syntax {
		if filepath.Base(typedPackage.Fset.Position(file.Package).Filename) !=
			"source_primitives_darwin.go" {
			continue
		}
		for _, candidate := range file.Decls {
			declaration, ok := candidate.(*ast.FuncDecl)
			if !ok || sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[declaration.Name],
				typedPackage.Types.Path(),
			) != wantedRole {
				continue
			}
			if function != nil {
				return []string{"Darwin directory name normalization has multiple implementations"}
			}
			function = declaration
		}
	}
	if !sourceConstructionDarwinDirectoryNameFlowProved(typedPackage, function) {
		position := wantedRole
		if function != nil {
			position = typedPackage.Fset.Position(function.Pos()).String()
		}
		return []string{position + " Darwin directory entry name flow is outside audited shape"}
	}
	return nil
}

func sourceConstructionDarwinDirectoryNameFlowProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if function == nil || function.Body == nil || len(function.Body.List) != 8 {
		return false
	}
	info := typedPackage.TypesInfo
	entriesObject := sourceConstructionNamedParameterObject(info, function, "entries")
	errObject := sourceConstructionNamedParameterObject(info, function, "err")
	if entriesObject == nil || errObject == nil ||
		!sourceConstructionTripleFailureGuardReturnProved(
			info,
			function.Body.List[0],
			"sourceContextPrimitiveFailure(ctx,OperationWalk)",
		) {
		return false
	}
	doneAssignment, ok := function.Body.List[1].(*ast.AssignStmt)
	if !ok || doneAssignment.Tok != token.DEFINE || len(doneAssignment.Lhs) != 1 ||
		len(doneAssignment.Rhs) != 1 {
		return false
	}
	done, doneOK := doneAssignment.Lhs[0].(*ast.Ident)
	doneCondition, conditionOK := doneAssignment.Rhs[0].(*ast.BinaryExpr)
	if !doneOK || !conditionOK || done.Name != "done" || doneCondition.Op != token.EQL ||
		!sourceConstructionDirectObjectExpression(info, doneCondition.X, errObject) ||
		types.ExprString(doneCondition.Y) != "io.EOF" {
		return false
	}
	doneObject := info.Defs[done]
	if doneObject == nil || !sourceConstructionDarwinDirectoryErrorBranchProved(
		info,
		function.Body.List[2],
		errObject,
		doneObject,
	) || !sourceConstructionFixedTripleFailureBranchProved(
		function.Body.List[3],
		"len(entries) > sourceDirectoryReadBatchSize",
		"newSourcePrimitiveFailure(OperationWalk,CauseLimit)",
	) || !sourceConstructionFixedTripleFailureBranchProved(
		function.Body.List[4],
		"len(entries) == 0 && !done",
		"newSourcePrimitiveFailure(OperationWalk,CauseUnstable)",
	) {
		return false
	}

	namesAssignment, ok := function.Body.List[5].(*ast.AssignStmt)
	if !ok || namesAssignment.Tok != token.DEFINE || len(namesAssignment.Lhs) != 1 ||
		len(namesAssignment.Rhs) != 1 || types.ExprString(namesAssignment.Rhs[0]) !=
		"make([]string, len(entries))" {
		return false
	}
	names, ok := namesAssignment.Lhs[0].(*ast.Ident)
	if !ok || names.Name != "names" {
		return false
	}
	namesObject := info.Defs[names]
	nameRange, ok := function.Body.List[6].(*ast.RangeStmt)
	if namesObject == nil || !ok || nameRange.Tok != token.DEFINE ||
		!sourceConstructionDirectObjectExpression(info, nameRange.X, entriesObject) ||
		len(nameRange.Body.List) != 5 {
		return false
	}
	index, indexOK := nameRange.Key.(*ast.Ident)
	entry, entryOK := nameRange.Value.(*ast.Ident)
	if !indexOK || !entryOK || index.Name != "index" || entry.Name != "entry" {
		return false
	}
	indexObject := info.Defs[index]
	entryObject := info.Defs[entry]
	if indexObject == nil || entryObject == nil ||
		!sourceConstructionDarwinNilDirectoryEntryBranchProved(
			info,
			nameRange.Body.List[0],
			entryObject,
		) {
		return false
	}
	nameAssignment, ok := nameRange.Body.List[1].(*ast.AssignStmt)
	if !ok || nameAssignment.Tok != token.DEFINE || len(nameAssignment.Lhs) != 1 ||
		len(nameAssignment.Rhs) != 1 {
		return false
	}
	name, nameOK := nameAssignment.Lhs[0].(*ast.Ident)
	nameCall, callOK := nameAssignment.Rhs[0].(*ast.CallExpr)
	if !nameOK || !callOK || name.Name != "name" ||
		sourceConstructionCallFingerprint(nameCall) != "entry.Name()" {
		return false
	}
	selector, ok := nameCall.Fun.(*ast.SelectorExpr)
	if !ok || !sourceConstructionDirectObjectExpression(info, selector.X, entryObject) {
		return false
	}
	nameObject := info.Defs[name]
	if nameObject == nil || !sourceConstructionFixedTripleFailureBranchProved(
		nameRange.Body.List[2],
		"!validDarwinSourceName(name)",
		"newSourcePrimitiveFailure(OperationWalk,CauseUnstable)",
	) || !sourceConstructionFixedTripleFailureBranchProved(
		nameRange.Body.List[3],
		"len(name) > maxPathComponentBytes",
		"newSourcePrimitiveFailure(OperationWalk,CauseLimit)",
	) {
		return false
	}
	store, ok := nameRange.Body.List[4].(*ast.AssignStmt)
	if !ok || store.Tok != token.ASSIGN || len(store.Lhs) != 1 || len(store.Rhs) != 1 {
		return false
	}
	indexed, ok := store.Lhs[0].(*ast.IndexExpr)
	if !ok || !sourceConstructionDirectObjectExpression(info, indexed.X, namesObject) ||
		!sourceConstructionDirectObjectExpression(info, indexed.Index, indexObject) ||
		!sourceConstructionDirectObjectExpression(info, store.Rhs[0], nameObject) {
		return false
	}
	returned, ok := function.Body.List[7].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 3 || !sourceConstructionDirectObjectExpression(
		info,
		returned.Results[0],
		namesObject,
	) || !sourceConstructionDirectObjectExpression(
		info,
		returned.Results[1],
		doneObject,
	) || !sourceConstructionBareNilExpression(returned.Results[2]) {
		return false
	}
	wantUses := map[types.Object]int{
		namesObject: 2,
		indexObject: 1,
		entryObject: 2,
		nameObject:  3,
		doneObject:  3,
	}
	gotUses := make(map[types.Object]int)
	for identifier, object := range info.Uses {
		if identifier.Pos() < function.Pos() || identifier.End() > function.End() ||
			wantUses[object] == 0 {
			continue
		}
		gotUses[object]++
	}
	for object, want := range wantUses {
		if gotUses[object] != want {
			return false
		}
	}
	return true
}

func sourceConstructionTripleFailureGuardReturnProved(
	info *types.Info,
	statement ast.Stmt,
	callFingerprint string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	assignment, ok := branch.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return false
	}
	failure, ok := assignment.Lhs[0].(*ast.Ident)
	call, callOK := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || !callOK || failure.Name != "failure" ||
		sourceConstructionCallFingerprint(call) != callFingerprint {
		return false
	}
	failureObject := info.Defs[failure]
	if failureObject == nil || !sourceConstructionObjectNilComparison(
		info,
		branch.Cond,
		failureObject,
		token.NEQ,
	) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 3 &&
		sourceConstructionBareNilExpression(returned.Results[0]) &&
		types.ExprString(returned.Results[1]) == "false" &&
		sourceConstructionDirectObjectExpression(info, returned.Results[2], failureObject)
}

func sourceConstructionFixedTripleFailureBranchProved(
	statement ast.Stmt,
	conditionFingerprint string,
	failureFingerprint string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 ||
		types.ExprString(branch.Cond) != conditionFingerprint {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 3 ||
		!sourceConstructionBareNilExpression(returned.Results[0]) ||
		types.ExprString(returned.Results[1]) != "false" {
		return false
	}
	failure, ok := returned.Results[2].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(failure) == failureFingerprint
}

func sourceConstructionDarwinDirectoryErrorBranchProved(
	info *types.Info,
	statement ast.Stmt,
	errObject types.Object,
	doneObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	condition, ok := sourceConstructionUnparenthesizedExpression(branch.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.LAND || !sourceConstructionObjectNilComparison(
		info,
		condition.X,
		errObject,
		token.NEQ,
	) {
		return false
	}
	notDone, ok := sourceConstructionUnparenthesizedExpression(condition.Y).(*ast.UnaryExpr)
	if !ok || notDone.Op != token.NOT || !sourceConstructionDirectObjectExpression(
		info,
		notDone.X,
		doneObject,
	) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 3 ||
		!sourceConstructionBareNilExpression(returned.Results[0]) ||
		types.ExprString(returned.Results[1]) != "false" {
		return false
	}
	failure, ok := returned.Results[2].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(failure) ==
		"classifyDarwinSourceWalkFailure(ctx,err)" && len(failure.Args) == 2 &&
		sourceConstructionDirectObjectExpression(info, failure.Args[1], errObject)
}

func sourceConstructionDarwinNilDirectoryEntryBranchProved(
	info *types.Info,
	statement ast.Stmt,
	entryObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 ||
		!sourceConstructionObjectNilComparison(info, branch.Cond, entryObject, token.EQL) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 3 ||
		!sourceConstructionBareNilExpression(returned.Results[0]) ||
		types.ExprString(returned.Results[1]) != "false" {
		return false
	}
	failure, ok := returned.Results[2].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(failure) ==
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)"
}

func sourceConstructionPostAcquisitionTopologyViolations(
	typedPackage *packages.Package,
) []string {
	type acquisitionTopology struct {
		role              string
		field             string
		acquisition       string
		keepAlive         string
		rootDirectoryTail bool
	}
	expected := []acquisitionTopology{
		{
			role:        "(darwinSourcePrimitives).openRepositoryRoot",
			field:       "root",
			acquisition: "os.OpenRoot(locator.path)",
		},
		{
			role:        "(darwinSourcePrimitives).openChildRoot",
			field:       "root",
			acquisition: "parent.root.OpenRoot(name)",
			keepAlive:   "runtime.KeepAlive(parent.root)",
		},
		{
			role:              "(darwinSourcePrimitives).openRootDirectoryDescriptor",
			field:             "file",
			acquisition:       "root.root.Open(\".\")",
			keepAlive:         "runtime.KeepAlive(root.root)",
			rootDirectoryTail: true,
		},
	}
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for post-acquisition topology audit"}
	}
	declarations := make(map[string]*ast.FuncDecl)
	for _, file := range typedPackage.Syntax {
		if filepath.Base(typedPackage.Fset.Position(file.Package).Filename) !=
			"source_primitives_darwin.go" {
			continue
		}
		for _, candidate := range file.Decls {
			function, ok := candidate.(*ast.FuncDecl)
			if !ok {
				continue
			}
			role := sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			)
			for _, topology := range expected {
				if role != topology.role {
					continue
				}
				if declarations[role] != nil {
					declarations[role] = nil
				} else {
					declarations[role] = function
				}
			}
		}
	}
	violations := make([]string, 0)
	for _, topology := range expected {
		declaration := declarations[topology.role]
		if sourceConstructionPostAcquisitionTopologyProved(
			typedPackage,
			declaration,
			topology.field,
			topology.acquisition,
			topology.keepAlive,
			topology.rootDirectoryTail,
		) {
			continue
		}
		position := topology.role
		if declaration != nil {
			position = typedPackage.Fset.Position(declaration.Pos()).String()
		}
		violations = append(violations,
			position+" Darwin post-acquisition topology outside audited shape "+topology.role,
		)
	}
	return violations
}

func sourceConstructionRootDescriptorIdentityProvenanceViolations(
	typedPackage *packages.Package,
) []string {
	const role = "(darwinSourcePrimitives).compareRootAndDescriptor"
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for root identity provenance audit"}
	}
	var function *ast.FuncDecl
	for _, file := range typedPackage.Syntax {
		if filepath.Base(typedPackage.Fset.Position(file.Package).Filename) !=
			"source_primitives_darwin.go" {
			continue
		}
		for _, candidate := range file.Decls {
			declaration, ok := candidate.(*ast.FuncDecl)
			if !ok || sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[declaration.Name],
				typedPackage.Types.Path(),
			) != role {
				continue
			}
			if function != nil {
				function = nil
				break
			}
			function = declaration
		}
	}
	if sourceConstructionRootDescriptorIdentityProvenanceProved(typedPackage, function) {
		return nil
	}
	position := role
	if function != nil {
		position = typedPackage.Fset.Position(function.Pos()).String()
	}
	return []string{position + " root/descriptor identity objects are outside audited provenance"}
}

func sourceConstructionInitialChildValidationViolations(
	typedPackage *packages.Package,
) []string {
	const role = "validateInitialSourceChild"
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for child containment audit"}
	}
	var function *ast.FuncDecl
	for _, file := range typedPackage.Syntax {
		if filepath.Base(typedPackage.Fset.Position(file.Package).Filename) !=
			"source_construction_acquire.go" {
			continue
		}
		for _, candidate := range file.Decls {
			declaration, ok := candidate.(*ast.FuncDecl)
			if !ok || sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[declaration.Name],
				typedPackage.Types.Path(),
			) != role {
				continue
			}
			if function != nil {
				function = nil
				break
			}
			function = declaration
		}
	}
	if sourceConstructionInitialChildValidationProved(typedPackage.TypesInfo, function) {
		return nil
	}
	position := role
	if function != nil {
		position = typedPackage.Fset.Position(function.Pos()).String()
	}
	return []string{position + " initial source child containment validation is outside audited shape"}
}

func sourceConstructionInitialChildValidationProved(
	info *types.Info,
	function *ast.FuncDecl,
) bool {
	if info == nil || function == nil || function.Body == nil || len(function.Body.List) != 5 ||
		!sourceConstructionSingleFailureReturnGuardProved(
			info,
			function.Body.List[0],
			"sourceContextPrimitiveFailure(ctx,OperationValidate)",
		) {
		return false
	}
	declaration, ok := function.Body.List[1].(*ast.DeclStmt)
	if !ok {
		return false
	}
	general, ok := declaration.Decl.(*ast.GenDecl)
	if !ok || general.Tok != token.VAR || len(general.Specs) != 1 {
		return false
	}
	specification, ok := general.Specs[0].(*ast.ValueSpec)
	if !ok || len(specification.Names) != 1 || len(specification.Values) != 0 ||
		types.ExprString(specification.Type) != "*sourcePrimitiveFailure" {
		return false
	}
	validationFailure := specification.Names[0]
	if validationFailure.Name != "validationFailure" {
		return false
	}
	validationFailureObject := info.Defs[validationFailure]
	branch, ok := function.Body.List[2].(*ast.IfStmt)
	if validationFailureObject == nil || !ok || branch.Init != nil || branch.Else != nil ||
		len(branch.Body.List) != 1 || types.ExprString(branch.Cond) !=
		"parent.snapshot.identity.Device != child.snapshot.identity.Device || "+
			"parent.mount.filesystem != child.mount.filesystem" {
		return false
	}
	assignment, ok := branch.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || !sourceConstructionDirectObjectExpression(
		info,
		assignment.Lhs[0],
		validationFailureObject,
	) {
		return false
	}
	failure, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || sourceConstructionCallFingerprint(failure) !=
		"newSourcePrimitiveFailure(OperationValidate,CauseUnsupported)" ||
		!sourceConstructionSingleFailureReturnGuardProved(
			info,
			function.Body.List[3],
			"sourceContextPrimitiveFailure(ctx,OperationValidate)",
		) {
		return false
	}
	returned, ok := function.Body.List[4].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 1 && sourceConstructionDirectObjectExpression(
		info,
		returned.Results[0],
		validationFailureObject,
	)
}

func sourceConstructionRootDescriptorIdentityProvenanceProved(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if function == nil || function.Body == nil || len(function.Body.List) != 15 {
		return false
	}
	info := typedPackage.TypesInfo
	statements := function.Body.List
	if !sourceConstructionFixedSingleFailureBranchProved(
		statements[0],
		"!root.validOpen() || !descriptor.validOpen() || "+
			"descriptor.kind != sourceObservedDirectory",
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)",
	) || !sourceConstructionSingleFailureReturnGuardProved(
		info,
		statements[1],
		"sourceContextPrimitiveFailure(ctx,OperationProbe)",
	) {
		return false
	}
	rootAssignment, ok := statements[2].(*ast.AssignStmt)
	if !ok || rootAssignment.Tok != token.DEFINE || len(rootAssignment.Lhs) != 2 ||
		len(rootAssignment.Rhs) != 1 {
		return false
	}
	rootInfo, rootInfoOK := rootAssignment.Lhs[0].(*ast.Ident)
	rootErr, rootErrOK := rootAssignment.Lhs[1].(*ast.Ident)
	rootCall, rootCallOK := rootAssignment.Rhs[0].(*ast.CallExpr)
	if !rootInfoOK || !rootErrOK || !rootCallOK || rootInfo.Name != "rootInfo" ||
		rootErr.Name != "rootErr" || sourceConstructionCallFingerprint(rootCall) !=
		"root.root.Lstat(\".\")" {
		return false
	}
	rootInfoObject := info.Defs[rootInfo]
	rootErrObject := info.Defs[rootErr]
	if rootInfoObject == nil || rootErrObject == nil ||
		!sourceConstructionDirectCallStatementFingerprint(
			statements[3],
			"runtime.KeepAlive(root.root)",
		) || !sourceConstructionSingleFailureReturnGuardProved(
		info,
		statements[4],
		"sourceContextPrimitiveFailure(ctx,OperationProbe)",
	) || !sourceConstructionDarwinIdentityIOFailureProved(
		info,
		statements[5],
		rootErrObject,
	) {
		return false
	}

	descriptorAssignment, ok := statements[6].(*ast.AssignStmt)
	if !ok || descriptorAssignment.Tok != token.DEFINE ||
		len(descriptorAssignment.Lhs) != 2 || len(descriptorAssignment.Rhs) != 1 {
		return false
	}
	descriptorInfo, descriptorInfoOK := descriptorAssignment.Lhs[0].(*ast.Ident)
	descriptorErr, descriptorErrOK := descriptorAssignment.Lhs[1].(*ast.Ident)
	descriptorCall, descriptorCallOK := descriptorAssignment.Rhs[0].(*ast.CallExpr)
	if !descriptorInfoOK || !descriptorErrOK || !descriptorCallOK ||
		descriptorInfo.Name != "descriptorInfo" || descriptorErr.Name != "descriptorErr" ||
		sourceConstructionCallFingerprint(descriptorCall) != "descriptor.file.Stat()" {
		return false
	}
	descriptorInfoObject := info.Defs[descriptorInfo]
	descriptorErrObject := info.Defs[descriptorErr]
	if descriptorInfoObject == nil || descriptorErrObject == nil ||
		!sourceConstructionDirectCallStatementFingerprint(
			statements[7],
			"runtime.KeepAlive(descriptor.file)",
		) || !sourceConstructionSingleFailureReturnGuardProved(
		info,
		statements[8],
		"sourceContextPrimitiveFailure(ctx,OperationProbe)",
	) || !sourceConstructionDarwinIdentityIOFailureProved(
		info,
		statements[9],
		descriptorErrObject,
	) || !sourceConstructionSingleFailureReturnGuardProved(
		info,
		statements[10],
		"sourceContextPrimitiveFailure(ctx,OperationCompare)",
	) {
		return false
	}

	sameAssignment, ok := statements[11].(*ast.AssignStmt)
	if !ok || sameAssignment.Tok != token.DEFINE || len(sameAssignment.Lhs) != 1 ||
		len(sameAssignment.Rhs) != 1 {
		return false
	}
	same, sameOK := sameAssignment.Lhs[0].(*ast.Ident)
	sameCall, sameCallOK := sameAssignment.Rhs[0].(*ast.CallExpr)
	if !sameOK || !sameCallOK || same.Name != "same" ||
		sourceConstructionCallFingerprint(sameCall) !=
			"os.SameFile(rootInfo,descriptorInfo)" || len(sameCall.Args) != 2 ||
		!sourceConstructionDirectObjectExpression(info, sameCall.Args[0], rootInfoObject) ||
		!sourceConstructionDirectObjectExpression(
			info,
			sameCall.Args[1],
			descriptorInfoObject,
		) {
		return false
	}
	sameObject := info.Defs[same]
	if sameObject == nil || !sourceConstructionSingleFailureReturnGuardProved(
		info,
		statements[12],
		"sourceContextPrimitiveFailure(ctx,OperationCompare)",
	) || !sourceConstructionRootIdentityFailureBranchProved(
		info,
		statements[13],
		sameObject,
	) || !sourceConstructionSingleFailureValueReturnProved(statements[14], "nil") {
		return false
	}
	uses := map[types.Object]int{}
	for identifier, object := range info.Uses {
		if identifier.Pos() < function.Pos() || identifier.End() > function.End() {
			continue
		}
		if object == rootInfoObject || object == descriptorInfoObject || object == sameObject {
			uses[object]++
		}
	}
	return uses[rootInfoObject] == 1 && uses[descriptorInfoObject] == 1 &&
		uses[sameObject] == 1
}

func sourceConstructionDarwinIdentityIOFailureProved(
	info *types.Info,
	statement ast.Stmt,
	errObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 ||
		!sourceConstructionObjectNilComparison(info, branch.Cond, errObject, token.NEQ) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	failure, ok := returned.Results[0].(*ast.CallExpr)
	if !ok || types.ExprString(failure.Fun) != "newSourcePrimitiveFailure" ||
		len(failure.Args) != 2 || types.ExprString(failure.Args[0]) != "OperationProbe" {
		return false
	}
	cause, ok := failure.Args[1].(*ast.CallExpr)
	return ok && types.ExprString(cause.Fun) == "darwinSourceIOCause" &&
		len(cause.Args) == 1 && sourceConstructionDirectObjectExpression(
		info,
		cause.Args[0],
		errObject,
	)
}

func sourceConstructionRootIdentityFailureBranchProved(
	info *types.Info,
	statement ast.Stmt,
	sameObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	negated, ok := sourceConstructionUnparenthesizedExpression(branch.Cond).(*ast.UnaryExpr)
	if !ok || negated.Op != token.NOT ||
		!sourceConstructionDirectObjectExpression(info, negated.X, sameObject) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	failure, ok := returned.Results[0].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(failure) ==
		"newSourcePrimitiveFailure(OperationCompare,CauseIdentity)"
}

func sourceConstructionPostAcquisitionTopologyProved(
	typedPackage *packages.Package,
	declaration *ast.FuncDecl,
	field string,
	acquisitionFingerprint string,
	keepAliveFingerprint string,
	rootDirectoryTail bool,
) bool {
	if declaration == nil || declaration.Body == nil {
		return false
	}
	statements := declaration.Body.List
	acquisitionIndex := -1
	var ownerObject types.Object
	var errObject types.Object
	for index, statement := range statements {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 2 ||
			len(assignment.Rhs) != 1 {
			continue
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || sourceConstructionCallFingerprint(call) != acquisitionFingerprint {
			continue
		}
		selector, selectorOK := assignment.Lhs[0].(*ast.SelectorExpr)
		owner, ownerOK := selector.X.(*ast.Ident)
		readErr, errOK := assignment.Lhs[1].(*ast.Ident)
		if !selectorOK || !ownerOK || !errOK || selector.Sel.Name != field ||
			owner.Name != "owner" || readErr.Name != "err" {
			return false
		}
		if acquisitionIndex != -1 {
			return false
		}
		acquisitionIndex = index
		ownerObject = typedPackage.TypesInfo.Uses[owner]
		errObject = typedPackage.TypesInfo.Uses[readErr]
	}
	if acquisitionIndex < 0 || ownerObject == nil || errObject == nil {
		return false
	}
	next := acquisitionIndex + 1
	if keepAliveFingerprint != "" {
		if next >= len(statements) ||
			!sourceConstructionDirectCallStatementFingerprint(
				statements[next],
				keepAliveFingerprint,
			) {
			return false
		}
		next++
	}
	if next+2 >= len(statements) ||
		!sourceConstructionAcquisitionErrorBranchProved(
			typedPackage,
			statements[next],
			ownerObject,
			errObject,
			field,
		) ||
		!sourceConstructionAcquisitionNilBranchProved(
			typedPackage,
			statements[next+1],
			ownerObject,
			field,
		) ||
		!sourceConstructionPostOpenContextBranchProved(
			typedPackage,
			statements[next+2],
			ownerObject,
			"OperationOpen",
		) {
		return false
	}
	next += 3
	if rootDirectoryTail {
		if next+4 >= len(statements) ||
			!sourceConstructionPostOpenContextBranchProved(
				typedPackage,
				statements[next],
				ownerObject,
				"OperationProbe",
			) {
			return false
		}
		kindAssignment, ok := statements[next+1].(*ast.AssignStmt)
		if !ok || kindAssignment.Tok != token.DEFINE || len(kindAssignment.Lhs) != 1 ||
			len(kindAssignment.Rhs) != 1 {
			return false
		}
		kindIdentifier, ok := kindAssignment.Lhs[0].(*ast.Ident)
		kindCall, callOK := kindAssignment.Rhs[0].(*ast.CallExpr)
		if !ok || !callOK || kindIdentifier.Name != "kindErr" ||
			sourceConstructionCallFingerprint(kindCall) !=
				"requireDescriptorKind(int(owner.file.Fd()),entryDirectory)" ||
			!sourceConstructionDirectCallStatementFingerprint(
				statements[next+2],
				"runtime.KeepAlive(owner.file)",
			) ||
			!sourceConstructionPostOpenContextBranchProved(
				typedPackage,
				statements[next+3],
				ownerObject,
				"OperationProbe",
			) {
			return false
		}
		kindObject := typedPackage.TypesInfo.Defs[kindIdentifier]
		kindBranch, ok := statements[next+4].(*ast.IfStmt)
		if kindObject == nil || !ok || !sourceConstructionDescriptorKindBranchProved(
			typedPackage,
			kindBranch,
			kindObject,
			ownerObject,
			false,
		) {
			return false
		}
		next += 5
	}
	if next != len(statements)-1 {
		return false
	}
	return sourceConstructionOwnerSuccessReturnProved(
		typedPackage.TypesInfo,
		statements[next],
		ownerObject,
	)
}

func sourceConstructionDirectCallStatementFingerprint(statement ast.Stmt, want string) bool {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(call) == want
}

func sourceConstructionAcquisitionErrorBranchProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	ownerObject types.Object,
	errObject types.Object,
	field string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil ||
		!sourceConstructionObjectNilComparison(
			typedPackage.TypesInfo,
			branch.Cond,
			errObject,
			token.NEQ,
		) || len(branch.Body.List) != 2 {
		return false
	}
	ownerBranch, ok := branch.Body.List[0].(*ast.IfStmt)
	if !ok || ownerBranch.Init != nil || ownerBranch.Else != nil ||
		!sourceConstructionOwnerFieldNilComparison(
			typedPackage.TypesInfo,
			ownerBranch.Cond,
			ownerObject,
			field,
			token.NEQ,
		) || len(ownerBranch.Body.List) != 1 ||
		!sourceConstructionOwnerFailureReturnProved(
			typedPackage.TypesInfo,
			ownerBranch.Body.List[0],
			ownerObject,
			"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)",
		) {
		return false
	}
	return sourceConstructionNilFailureReturnProved(
		branch.Body.List[1],
		"classifyDarwinSourceOpenFailure(ctx,sourceInitialRequired,err)",
	)
}

func sourceConstructionAcquisitionNilBranchProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	ownerObject types.Object,
	field string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	return ok && branch.Init == nil && branch.Else == nil &&
		sourceConstructionOwnerFieldNilComparison(
			typedPackage.TypesInfo,
			branch.Cond,
			ownerObject,
			field,
			token.EQL,
		) && len(branch.Body.List) == 1 &&
		sourceConstructionNilFailureReturnProved(
			branch.Body.List[0],
			"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)",
		)
}

func sourceConstructionPostOpenContextBranchProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	ownerObject types.Object,
	operation string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	assignment, ok := branch.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return false
	}
	failure, ok := assignment.Lhs[0].(*ast.Ident)
	call, callOK := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || !callOK || failure.Name != "failure" ||
		sourceConstructionCallFingerprint(call) !=
			"sourceContextPrimitiveFailure(ctx,"+operation+")" {
		return false
	}
	failureObject := typedPackage.TypesInfo.Defs[failure]
	if failureObject == nil || !sourceConstructionObjectNilComparison(
		typedPackage.TypesInfo,
		branch.Cond,
		failureObject,
		token.NEQ,
	) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 2 &&
		sourceConstructionDirectObjectExpression(
			typedPackage.TypesInfo,
			returned.Results[0],
			ownerObject,
		) && sourceConstructionDirectObjectExpression(
		typedPackage.TypesInfo,
		returned.Results[1],
		failureObject,
	)
}

func sourceConstructionObjectNilComparison(
	info *types.Info,
	expression ast.Expr,
	object types.Object,
	operator token.Token,
) bool {
	comparison, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.BinaryExpr)
	return ok && comparison.Op == operator &&
		sourceConstructionDirectObjectExpression(info, comparison.X, object) &&
		sourceConstructionBareNilExpression(comparison.Y)
}

func sourceConstructionOwnerFieldNilComparison(
	info *types.Info,
	expression ast.Expr,
	ownerObject types.Object,
	field string,
	operator token.Token,
) bool {
	comparison, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.BinaryExpr)
	if !ok || comparison.Op != operator || !sourceConstructionBareNilExpression(comparison.Y) {
		return false
	}
	selector, ok := sourceConstructionUnparenthesizedExpression(comparison.X).(*ast.SelectorExpr)
	return ok && selector.Sel.Name == field &&
		sourceConstructionDirectObjectExpression(info, selector.X, ownerObject)
}

func sourceConstructionBareNilExpression(expression ast.Expr) bool {
	identifier, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.Ident)
	return ok && identifier.Name == "nil"
}

func sourceConstructionOwnerFailureReturnProved(
	info *types.Info,
	statement ast.Stmt,
	ownerObject types.Object,
	failureFingerprint string,
) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 ||
		!sourceConstructionDirectObjectExpression(info, returned.Results[0], ownerObject) {
		return false
	}
	call, ok := returned.Results[1].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(call) == failureFingerprint
}

func sourceConstructionNilFailureReturnProved(statement ast.Stmt, failureFingerprint string) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 || !sourceConstructionBareNilExpression(returned.Results[0]) {
		return false
	}
	call, ok := returned.Results[1].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(call) == failureFingerprint
}

func sourceConstructionOwnerSuccessReturnProved(
	info *types.Info,
	statement ast.Stmt,
	ownerObject types.Object,
) bool {
	returned, ok := statement.(*ast.ReturnStmt)
	return ok && len(returned.Results) == 2 &&
		sourceConstructionDirectObjectExpression(info, returned.Results[0], ownerObject) &&
		sourceConstructionBareNilExpression(returned.Results[1])
}

func sourceConstructionDescriptorKindBranchProved(
	typedPackage *packages.Package,
	branch *ast.IfStmt,
	kindObject types.Object,
	ownerObject types.Object,
	relative bool,
) bool {
	if branch == nil || branch.Init != nil || branch.Else != nil ||
		len(branch.Body.List) != 2 || !sourceConstructionObjectNilComparison(
		typedPackage.TypesInfo,
		branch.Cond,
		kindObject,
		token.NEQ,
	) {
		return false
	}
	identityBranch, ok := branch.Body.List[0].(*ast.IfStmt)
	if !ok || identityBranch.Init != nil || identityBranch.Else != nil ||
		!sourceConstructionKindIdentityConditionProved(
			typedPackage.TypesInfo,
			identityBranch.Cond,
			kindObject,
		) {
		return false
	}
	if !relative {
		if len(identityBranch.Body.List) != 1 || !sourceConstructionOwnerFailureReturnProved(
			typedPackage.TypesInfo,
			identityBranch.Body.List[0],
			ownerObject,
			"newSourcePrimitiveFailure(OperationOpen,CauseIdentity)",
		) {
			return false
		}
	} else {
		if len(identityBranch.Body.List) != 3 {
			return false
		}
		operationAssignment, ok := identityBranch.Body.List[0].(*ast.AssignStmt)
		if !ok || operationAssignment.Tok != token.DEFINE ||
			len(operationAssignment.Lhs) != 1 || len(operationAssignment.Rhs) != 1 ||
			types.ExprString(operationAssignment.Rhs[0]) != "OperationOpen" {
			return false
		}
		operationIdentifier, ok := operationAssignment.Lhs[0].(*ast.Ident)
		if !ok || operationIdentifier.Name != "operation" {
			return false
		}
		operationObject := typedPackage.TypesInfo.Defs[operationIdentifier]
		compareBranch, ok := identityBranch.Body.List[1].(*ast.IfStmt)
		if operationObject == nil || !ok || compareBranch.Init != nil ||
			compareBranch.Else != nil || types.ExprString(compareBranch.Cond) !=
			"mode == sourceRevalidatePresent" || len(compareBranch.Body.List) != 1 {
			return false
		}
		compareAssignment, ok := compareBranch.Body.List[0].(*ast.AssignStmt)
		if !ok || compareAssignment.Tok != token.ASSIGN ||
			len(compareAssignment.Lhs) != 1 || len(compareAssignment.Rhs) != 1 ||
			!sourceConstructionDirectObjectExpression(
				typedPackage.TypesInfo,
				compareAssignment.Lhs[0],
				operationObject,
			) || types.ExprString(compareAssignment.Rhs[0]) != "OperationCompare" ||
			!sourceConstructionOwnerFailureReturnProved(
				typedPackage.TypesInfo,
				identityBranch.Body.List[2],
				ownerObject,
				"newSourcePrimitiveFailure(operation,CauseIdentity)",
			) {
			return false
		}
	}
	return sourceConstructionOwnerFailureReturnProved(
		typedPackage.TypesInfo,
		branch.Body.List[1],
		ownerObject,
		"newSourcePrimitiveFailure(OperationProbe,darwinSourceIOCause(kindErr))",
	)
}

func sourceConstructionKindIdentityConditionProved(
	info *types.Info,
	expression ast.Expr,
	kindObject types.Object,
) bool {
	disjunction, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.BinaryExpr)
	if !ok || disjunction.Op != token.LOR {
		return false
	}
	for index, candidate := range []ast.Expr{disjunction.X, disjunction.Y} {
		comparison, ok := sourceConstructionUnparenthesizedExpression(candidate).(*ast.BinaryExpr)
		if !ok || comparison.Op != token.EQL || !sourceConstructionDirectObjectExpression(
			info,
			comparison.X,
			kindObject,
		) {
			return false
		}
		want := "errAuthorityKindMismatch"
		if index == 1 {
			want = "errUnsupportedAuthorityEntry"
		}
		if types.ExprString(comparison.Y) != want {
			return false
		}
	}
	return true
}

func sourceConstructionOpenFlagProvenanceViolations(
	typedPackage *packages.Package,
) []string {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		typedPackage.Fset == nil {
		return []string{"typed buildauthority package is unavailable for open-flag provenance audit"}
	}
	declarations := make(map[string]*ast.FuncDecl)
	for _, file := range typedPackage.Syntax {
		for _, candidate := range file.Decls {
			function, ok := candidate.(*ast.FuncDecl)
			if !ok {
				continue
			}
			role := sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			)
			switch role {
			case "(darwinSourcePrimitives).openPhysicalRootDescriptor",
				"(darwinSourcePrimitives).openRelativeNoFollow",
				"openDarwinSourceRelativeDescriptor":
				if declarations[role] != nil {
					declarations[role] = nil
				} else {
					declarations[role] = function
				}
			}
		}
	}
	violations := make([]string, 0)
	for _, provenance := range []struct {
		role      string
		producer  string
		consumer  string
		argument  int
		parameter bool
	}{
		{
			role:     "(darwinSourcePrimitives).openPhysicalRootDescriptor",
			producer: "darwinOpenFlags(entryDirectory,true)",
			consumer: "unix.Open(physicalRootPath,flags,0)",
			argument: 1,
		},
		{
			role:     "(darwinSourcePrimitives).openRelativeNoFollow",
			producer: "darwinOpenFlags(openKind,openKind != entrySymlink)",
			consumer: "openDarwinSourceRelativeDescriptor(ctx,parent,name,kind,openKind,mode,flags)",
			argument: 6,
		},
		{
			role:      "openDarwinSourceRelativeDescriptor",
			consumer:  "unix.Openat(int(parent.file.Fd()),name,flags,0)",
			argument:  2,
			parameter: true,
		},
	} {
		declaration := declarations[provenance.role]
		proved := false
		if provenance.parameter {
			proved = sourceConstructionSingleUseParameterProved(
				typedPackage,
				declaration,
				"flags",
				provenance.consumer,
				provenance.argument,
			)
		} else {
			proved = sourceConstructionSingleUseResultProved(
				typedPackage,
				declaration,
				"flags",
				provenance.producer,
				provenance.consumer,
				provenance.argument,
			)
		}
		if proved {
			continue
		}
		position := provenance.role
		if declaration != nil {
			position = typedPackage.Fset.Position(declaration.Pos()).String()
		}
		violations = append(violations,
			position+" source open flags do not have exact single-use provenance "+provenance.role,
		)
	}
	return violations
}

func sourceConstructionSingleUseResultProved(
	typedPackage *packages.Package,
	declaration *ast.FuncDecl,
	resultName string,
	producerFingerprint string,
	consumerFingerprint string,
	consumerArgument int,
) bool {
	if declaration == nil || declaration.Body == nil {
		return false
	}
	var resultObject types.Object
	producerCount := 0
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Rhs) != 1 {
			return true
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || sourceConstructionCallFingerprint(call) != producerFingerprint {
			return true
		}
		producerCount++
		for _, target := range assignment.Lhs {
			identifier, ok := target.(*ast.Ident)
			if !ok || identifier.Name != resultName {
				continue
			}
			resultObject = typedPackage.TypesInfo.Defs[identifier]
		}
		return true
	})
	return producerCount == 1 && resultObject != nil &&
		sourceConstructionSingleUseConsumerProved(
			typedPackage,
			declaration,
			resultObject,
			consumerFingerprint,
			consumerArgument,
		)
}

func sourceConstructionSingleUseParameterProved(
	typedPackage *packages.Package,
	declaration *ast.FuncDecl,
	parameterName string,
	consumerFingerprint string,
	consumerArgument int,
) bool {
	if declaration == nil || declaration.Type == nil || declaration.Type.Params == nil {
		return false
	}
	var parameterObject types.Object
	for _, field := range declaration.Type.Params.List {
		for _, name := range field.Names {
			if name.Name == parameterName {
				parameterObject = typedPackage.TypesInfo.Defs[name]
			}
		}
	}
	return parameterObject != nil && sourceConstructionSingleUseConsumerProved(
		typedPackage,
		declaration,
		parameterObject,
		consumerFingerprint,
		consumerArgument,
	)
}

func sourceConstructionSingleUseConsumerProved(
	typedPackage *packages.Package,
	declaration *ast.FuncDecl,
	object types.Object,
	consumerFingerprint string,
	consumerArgument int,
) bool {
	if declaration == nil || declaration.Body == nil || object == nil {
		return false
	}
	uses := 0
	for identifier, used := range typedPackage.TypesInfo.Uses {
		if used == object && declaration.Pos() <= identifier.Pos() && identifier.End() <= declaration.End() {
			uses++
		}
	}
	consumerCount := 0
	consumerUsesObject := false
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || sourceConstructionCallFingerprint(call) != consumerFingerprint {
			return true
		}
		consumerCount++
		if consumerArgument >= 0 && consumerArgument < len(call.Args) &&
			sourceConstructionDirectObjectExpression(
				typedPackage.TypesInfo,
				call.Args[consumerArgument],
				object,
			) {
			consumerUsesObject = true
		}
		return true
	})
	return uses == 1 && consumerCount == 1 && consumerUsesObject
}

func sourceConstructionSealedInterfaceImplementerViolations(
	typedPackage *packages.Package,
) []string {
	allowed := map[string]map[string]int{
		"sourcePrimitives": {
			"darwinSourcePrimitives": 1,
		},
		"sourceHandleCloser": {
			"darwinSourcePrimitives":   1,
			"directSourceHandleCloser": 1,
		},
	}
	type sealedContract struct {
		name     string
		contract *types.Interface
		allowed  map[string]int
		observed map[string]int
	}
	contracts := make([]sealedContract, 0, len(allowed))
	violations := make([]string, 0)
	interfaceNames := slices.Sorted(maps.Keys(allowed))
	for _, interfaceName := range interfaceNames {
		allowedImplementers := allowed[interfaceName]
		interfaceObject, _ := typedPackage.Types.Scope().Lookup(interfaceName).(*types.TypeName)
		if interfaceObject == nil {
			violations = append(violations, "sealed source interface "+interfaceName+" is missing")
			continue
		}
		interfaceNamed, _ := types.Unalias(interfaceObject.Type()).(*types.Named)
		if interfaceNamed == nil {
			violations = append(violations, "sealed source interface "+interfaceName+" is not named")
			continue
		}
		contract, _ := interfaceNamed.Underlying().(*types.Interface)
		if contract == nil {
			violations = append(violations, "sealed source interface "+interfaceName+" is not an interface")
			continue
		}
		contract.Complete()
		contracts = append(contracts, sealedContract{
			name:     interfaceName,
			contract: contract,
			allowed:  allowedImplementers,
			observed: make(map[string]int),
		})
	}

	type typeDefinition struct {
		identifier *ast.Ident
		object     *types.TypeName
	}
	definitions := make([]typeDefinition, 0)
	for identifier, object := range typedPackage.TypesInfo.Defs {
		typeObject, _ := object.(*types.TypeName)
		if identifier == nil || typeObject == nil || typeObject.IsAlias() {
			continue
		}
		definitions = append(definitions, typeDefinition{identifier: identifier, object: typeObject})
	}
	slices.SortFunc(definitions, func(left, right typeDefinition) int {
		return cmp.Compare(left.identifier.Pos(), right.identifier.Pos())
	})
	for _, definition := range definitions {
		candidate, _ := types.Unalias(definition.object.Type()).(*types.Named)
		if candidate == nil {
			continue
		}
		if _, isInterface := candidate.Underlying().(*types.Interface); isInterface {
			continue
		}
		for index := range contracts {
			contract := &contracts[index]
			if !types.Implements(candidate, contract.contract) &&
				!types.Implements(types.NewPointer(candidate), contract.contract) {
				continue
			}
			packageDefinition := typedPackage.Types.Scope().Lookup(definition.object.Name()) == definition.object
			if packageDefinition && contract.allowed[definition.object.Name()] != 0 {
				contract.observed[definition.object.Name()]++
				continue
			}
			violations = append(violations,
				typedPackage.Fset.Position(definition.identifier.Pos()).String()+
					" type "+definition.object.Name()+" implements sealed "+contract.name+
					" outside audited set",
			)
		}
	}

	anonymousSites := make(map[string]bool)
	for _, file := range typedPackage.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			expression, ok := node.(ast.Expr)
			if !ok || !typedPackage.TypesInfo.Types[expression].IsType() {
				return true
			}
			typeOf := types.Unalias(typedPackage.TypesInfo.TypeOf(expression))
			base := typeOf
			if pointer, ok := base.(*types.Pointer); ok {
				base = types.Unalias(pointer.Elem())
			}
			if _, named := base.(*types.Named); named {
				return true
			}
			if _, isInterface := base.Underlying().(*types.Interface); isInterface {
				return true
			}
			for _, contract := range contracts {
				implements := types.Implements(typeOf, contract.contract)
				if !implements {
					if _, pointer := typeOf.(*types.Pointer); !pointer {
						implements = types.Implements(types.NewPointer(typeOf), contract.contract)
					}
				}
				if !implements {
					continue
				}
				site := contract.name + "|" + typedPackage.Fset.Position(expression.Pos()).String()
				if anonymousSites[site] {
					continue
				}
				anonymousSites[site] = true
				violations = append(violations,
					typedPackage.Fset.Position(expression.Pos()).String()+
						" anonymous type implements sealed "+contract.name+" outside audited set",
				)
			}
			return true
		})
	}

	for _, contract := range contracts {
		for implementer, want := range contract.allowed {
			if got := contract.observed[implementer]; got != want {
				violations = append(violations,
					"audited sealed "+contract.name+" implementer "+implementer+" count = "+
						strconv.Itoa(got)+", want "+strconv.Itoa(want),
				)
			}
		}
	}
	return violations
}

func sourceConstructionSealedInterfaceValueViolations(
	typedPackage *packages.Package,
) []string {
	allowed := map[string]int{
		"source_construction_acquire.go|<outside-function>|sourcePrimitives|field:primitives:parameter":                                                         1,
		"source_construction_acquire.go|retainSourceConstruction|sourcePrimitives|argument:2:retainSourceConstructionWith(ctx,locator,primitives)":              1,
		"source_construction_acquire.go|retainSourceConstruction|sourcePrimitives|assignment::=:primitives<-platformSourcePrimitives()":                         1,
		"source_construction_acquire.go|retainSourceConstructionWith|sourcePrimitives|argument:0:builder.owner.closeIntoWith(primitives,&builder.outcome)":      1,
		"source_construction_acquire.go|retainSourceConstructionWith|sourcePrimitives|composite:primitives<-primitives":                                         1,
		"source_construction.go|(*sourceConstructionOwner).closeDirectInto|sourceHandleCloser|argument:0:owner.closeLocked(directSourceHandleCloser{},outcome)": 1,
		"source_construction.go|(*sourceConstructionOwner).closeInto|sourcePrimitives|argument:0:owner.closeIntoWith(primitives,outcome)":                       1,
		"source_construction.go|(*sourceConstructionOwner).closeInto|sourcePrimitives|assignment::=:primitives<-platformSourcePrimitives()":                     1,
		"source_construction.go|(*sourceConstructionOwner).closeIntoWith|sourceHandleCloser|argument:0:owner.closeLocked(primitives,outcome)":                   1,
		"source_construction.go|(*sourceConstructionOwner).closeLocked|sourceHandleCloser|composite:closer<-closer":                                             1,
		"source_construction.go|<outside-function>|sourceHandleCloser|field:closer:parameter":                                                                   1,
		"source_construction.go|<outside-function>|sourceHandleCloser|field:closer:struct":                                                                      1,
		"source_construction.go|<outside-function>|sourcePrimitives|field:primitives:parameter":                                                                 1,
		"source_construction.go|<outside-function>|sourcePrimitives|field:primitives:struct":                                                                    1,
		"source_primitives_darwin.go|<outside-function>|sourcePrimitives|field:<embedded>:result":                                                               1,
		"source_primitives_darwin.go|platformSourcePrimitives|sourcePrimitives|return:0<-darwinSourcePrimitives{}":                                              1,
	}
	observed := make(map[string]int)
	violations := make([]string, 0)
	record := func(file *ast.File, node ast.Node, interfaceName, topology string) {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		function := sourceConstructionEnclosingFunction(file, node.Pos())
		functionRole := "<outside-function>"
		if function != nil {
			functionRole = sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			)
		}
		site := filename + "|" + functionRole + "|" + interfaceName + "|" + topology
		observed[site]++
		if allowed[site] == 0 {
			violations = append(violations,
				typedPackage.Fset.Position(node.Pos()).String()+
					" sealed source interface value outside audited topology "+site,
			)
		}
	}
	for _, file := range typedPackage.Syntax {
		parents := sourceConstructionParentNodes(file)
		ast.Inspect(file, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.Field:
				interfaceName := sourceConstructionSealedInterfaceTypeName(
					typedPackage,
					typedPackage.TypesInfo.TypeOf(current.Type),
				)
				if interfaceName == "" {
					return true
				}
				names := make([]string, 0, len(current.Names))
				for _, name := range current.Names {
					names = append(names, name.Name)
				}
				if len(names) == 0 {
					names = append(names, "<embedded>")
				}
				record(file, current, interfaceName,
					"field:"+strings.Join(names, ",")+":"+
						sourceConstructionFieldContainerRole(current, parents),
				)
			case *ast.ValueSpec:
				parent, _ := parents[current].(*ast.GenDecl)
				if sourceConstructionAllowedPackageAssertion(
					filepath.Base(typedPackage.Fset.Position(file.Package).Filename),
					parent,
				) {
					return true
				}
				for index, name := range current.Names {
					object, _ := typedPackage.TypesInfo.Defs[name].(*types.Var)
					interfaceName := sourceConstructionSealedInterfaceTypeName(
						typedPackage,
						sourceConstructionObjectType(object),
					)
					if interfaceName == "" {
						continue
					}
					value := "<zero>"
					if index < len(current.Values) {
						value = types.ExprString(current.Values[index])
					} else if len(current.Values) == 1 {
						value = types.ExprString(current.Values[0])
					}
					record(file, name, interfaceName, "value:"+name.Name+"<-"+value)
				}
			case *ast.AssignStmt:
				for index, target := range current.Lhs {
					interfaceName := sourceConstructionSealedInterfaceTypeName(
						typedPackage,
						typedPackage.TypesInfo.TypeOf(target),
					)
					if interfaceName == "" {
						continue
					}
					value := "<tuple>"
					if index < len(current.Rhs) {
						value = types.ExprString(current.Rhs[index])
					} else if len(current.Rhs) == 1 {
						value = types.ExprString(current.Rhs[0])
					}
					record(file, target, interfaceName,
						"assignment:"+current.Tok.String()+":"+
							types.ExprString(target)+"<-"+value,
					)
				}
			case *ast.KeyValueExpr:
				key, ok := current.Key.(*ast.Ident)
				if !ok {
					return true
				}
				field, _ := typedPackage.TypesInfo.Uses[key].(*types.Var)
				interfaceName := sourceConstructionSealedInterfaceTypeName(
					typedPackage,
					sourceConstructionObjectType(field),
				)
				if interfaceName != "" {
					record(file, current, interfaceName,
						"composite:"+key.Name+"<-"+types.ExprString(current.Value),
					)
				}
			case *ast.ReturnStmt:
				function := sourceConstructionEnclosingFunction(file, current.Pos())
				if function == nil {
					return true
				}
				object, _ := typedPackage.TypesInfo.Defs[function.Name].(*types.Func)
				signature, _ := sourceConstructionObjectType(object).(*types.Signature)
				if signature == nil {
					return true
				}
				for index, result := range current.Results {
					if index >= signature.Results().Len() {
						break
					}
					interfaceName := sourceConstructionSealedInterfaceTypeName(
						typedPackage,
						signature.Results().At(index).Type(),
					)
					if interfaceName != "" {
						record(file, result, interfaceName,
							"return:"+strconv.Itoa(index)+"<-"+types.ExprString(result),
						)
					}
				}
			case *ast.CallExpr:
				if typedPackage.TypesInfo.Types[current.Fun].IsType() {
					interfaceName := sourceConstructionSealedInterfaceTypeName(
						typedPackage,
						typedPackage.TypesInfo.TypeOf(current),
					)
					if interfaceName != "" {
						record(file, current, interfaceName,
							"conversion:"+sourceConstructionCallFingerprint(current),
						)
					}
					return true
				}
				called := sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun)
				signature, _ := sourceConstructionObjectType(called).(*types.Signature)
				if signature == nil {
					return true
				}
				for index, argument := range current.Args {
					parameterIndex := index
					if signature.Variadic() && parameterIndex >= signature.Params().Len()-1 {
						parameterIndex = signature.Params().Len() - 1
					}
					if parameterIndex < 0 || parameterIndex >= signature.Params().Len() {
						continue
					}
					interfaceName := sourceConstructionSealedInterfaceTypeName(
						typedPackage,
						signature.Params().At(parameterIndex).Type(),
					)
					if interfaceName != "" {
						record(file, argument, interfaceName,
							"argument:"+strconv.Itoa(index)+":"+
								sourceConstructionCallFingerprint(current),
						)
					}
				}
			}
			return true
		})
	}
	for site, want := range allowed {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited sealed source interface value "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	return violations
}

func sourceConstructionSealedInterfaceContainerViolations(
	typedPackage *packages.Package,
) []string {
	violations := make([]string, 0)
	recorded := make(map[string]bool)
	record := func(file *ast.File, node ast.Node, interfaceName, detail string) {
		position := typedPackage.Fset.Position(node.Pos()).String()
		key := position + "|" + interfaceName + "|" + detail
		if recorded[key] {
			return
		}
		recorded[key] = true
		function := sourceConstructionEnclosingFunction(file, node.Pos())
		functionRole := "<outside-function>"
		if function != nil {
			functionRole = sourceConstructionFunctionRole(
				typedPackage.TypesInfo.Defs[function.Name],
				typedPackage.Types.Path(),
			)
		}
		violations = append(violations,
			position+" sealed source interface nested in unaudited container "+
				filepath.Base(typedPackage.Fset.Position(file.Package).Filename)+"|"+
				functionRole+"|"+interfaceName+"|"+detail,
		)
	}
	for _, file := range typedPackage.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.StarExpr:
				expression := current.(ast.Expr)
				if !typedPackage.TypesInfo.Types[expression].IsType() {
					return true
				}
				if interfaceName := sourceConstructionSealedInterfaceSyntaxMention(
					typedPackage,
					expression,
				); interfaceName != "" {
					record(file, expression, interfaceName, types.ExprString(expression))
				}
			case *ast.IndexExpr:
				if interfaceName := sourceConstructionSealedInterfaceSyntaxMention(
					typedPackage,
					current.Index,
				); interfaceName != "" {
					record(file, current, interfaceName, types.ExprString(current))
				}
			case *ast.IndexListExpr:
				for _, index := range current.Indices {
					if interfaceName := sourceConstructionSealedInterfaceSyntaxMention(
						typedPackage,
						index,
					); interfaceName != "" {
						record(file, current, interfaceName, types.ExprString(current))
						break
					}
				}
			case *ast.CompositeLit:
				if len(current.Elts) == 0 {
					return true
				}
				if _, keyed := current.Elts[0].(*ast.KeyValueExpr); keyed {
					return true
				}
				typeOf := types.Unalias(typedPackage.TypesInfo.TypeOf(current))
				if pointer, ok := typeOf.(*types.Pointer); ok {
					typeOf = types.Unalias(pointer.Elem())
				}
				structure, _ := typeOf.Underlying().(*types.Struct)
				if structure == nil {
					return true
				}
				for field := range structure.Fields() {
					if interfaceName := sourceConstructionSealedInterfaceTypeName(
						typedPackage,
						field.Type(),
					); interfaceName != "" {
						record(file, current, interfaceName,
							"positional:"+types.ExprString(current.Type),
						)
						break
					}
				}
			}
			return true
		})
	}
	return violations
}

func sourceConstructionSealedInterfaceSyntaxMention(
	typedPackage *packages.Package,
	node ast.Node,
) string {
	if typedPackage == nil || typedPackage.Types == nil ||
		typedPackage.TypesInfo == nil || node == nil {
		return ""
	}
	interfaceName := ""
	ast.Inspect(node, func(descendant ast.Node) bool {
		identifier, ok := descendant.(*ast.Ident)
		if !ok {
			return true
		}
		object, _ := typedPackage.TypesInfo.Uses[identifier].(*types.TypeName)
		if object == nil {
			object, _ = typedPackage.TypesInfo.Defs[identifier].(*types.TypeName)
		}
		if object == nil || object.Pkg() == nil ||
			object.Pkg().Path() != typedPackage.Types.Path() {
			return true
		}
		if object.Name() == "sourcePrimitives" || object.Name() == "sourceHandleCloser" {
			interfaceName = object.Name()
			return false
		}
		return true
	})
	return interfaceName
}

func sourceConstructionSealedInterfaceTypeName(
	typedPackage *packages.Package,
	typeOf types.Type,
) string {
	if typedPackage == nil || typedPackage.Types == nil || typeOf == nil {
		return ""
	}
	named, _ := types.Unalias(typeOf).(*types.Named)
	if named == nil || named.Obj().Pkg() == nil ||
		named.Obj().Pkg().Path() != typedPackage.Types.Path() {
		return ""
	}
	if named.Obj().Name() == "sourcePrimitives" || named.Obj().Name() == "sourceHandleCloser" {
		return named.Obj().Name()
	}
	return ""
}

func sourceConstructionFieldContainerRole(
	field *ast.Field,
	parents map[ast.Node]ast.Node,
) string {
	fieldList, _ := parents[field].(*ast.FieldList)
	switch container := parents[fieldList].(type) {
	case *ast.FuncType:
		if container.Params == fieldList {
			return "parameter"
		}
		if container.Results == fieldList {
			return "result"
		}
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	}
	return "unknown"
}

func sourceConstructionObjectType(object types.Object) types.Type {
	if object == nil {
		return nil
	}
	value := reflect.ValueOf(object)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return nil
	}
	return object.Type()
}

func sourceConstructionLocalInterfaceImplementationMethods(
	typedPackage *packages.Package,
	interfaceMethod *types.Func,
) ([]*types.Func, bool) {
	if typedPackage == nil || typedPackage.Types == nil || interfaceMethod == nil {
		return nil, false
	}
	signature, _ := interfaceMethod.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return nil, false
	}
	receiver := types.Unalias(signature.Recv().Type())
	contract, _ := receiver.Underlying().(*types.Interface)
	if contract == nil {
		return nil, false
	}
	contract.Complete()
	seen := make(map[*types.Func]bool)
	implementations := make([]*types.Func, 0)
	for _, name := range typedPackage.Types.Scope().Names() {
		typeObject, _ := typedPackage.Types.Scope().Lookup(name).(*types.TypeName)
		if typeObject == nil || typeObject.IsAlias() {
			continue
		}
		candidate, _ := types.Unalias(typeObject.Type()).(*types.Named)
		if candidate == nil {
			continue
		}
		if _, isInterface := candidate.Underlying().(*types.Interface); isInterface {
			continue
		}
		for _, candidateType := range []types.Type{candidate, types.NewPointer(candidate)} {
			if !types.Implements(candidateType, contract) {
				continue
			}
			selection, _, _ := types.LookupFieldOrMethod(
				candidateType,
				true,
				typedPackage.Types,
				interfaceMethod.Name(),
			)
			implementation, _ := selection.(*types.Func)
			if implementation == nil || implementation == interfaceMethod ||
				implementation.Pkg() == nil ||
				implementation.Pkg().Path() != typedPackage.Types.Path() || seen[implementation] {
				continue
			}
			seen[implementation] = true
			implementations = append(implementations, implementation)
		}
	}
	return implementations, true
}

func sourceConstructionDirectFunctionReferenceCall(
	identifier *ast.Ident,
	parents map[ast.Node]ast.Node,
) *ast.CallExpr {
	var reference ast.Expr = identifier
	if selector, ok := parents[reference].(*ast.SelectorExpr); ok && selector.Sel == identifier {
		reference = selector
	}
	for {
		switch parent := parents[reference].(type) {
		case *ast.ParenExpr:
			if parent.X != reference {
				return nil
			}
			reference = parent
		case *ast.IndexExpr:
			if parent.X != reference {
				return nil
			}
			reference = parent
		case *ast.IndexListExpr:
			if parent.X != reference {
				return nil
			}
			reference = parent
		default:
			call, ok := parents[reference].(*ast.CallExpr)
			if !ok || !sourceConstructionDirectExpression(call.Fun, reference) {
				return nil
			}
			return call
		}
	}
}

func sourceConstructionRawCapabilityOriginSites(
	typedPackage *packages.Package,
	allowed map[string]int,
	directResult func(types.Type) bool,
	violationLabel string,
) (map[string]int, []string) {
	observed := make(map[string]int)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		if !strings.HasPrefix(filename, "source_primitives") {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || !directResult(typedPackage.TypesInfo.TypeOf(call)) {
				return true
			}
			called := sourceConstructionCalledObject(typedPackage.TypesInfo, call.Fun)
			function := sourceConstructionEnclosingFunction(file, call.Pos())
			functionRole := "<outside-function>"
			if function != nil {
				functionRole = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			site := filename + "|" + functionRole + "|" +
				sourceConstructionCallObjectRole(called, typedPackage.Types.Path()) + "|" +
				sourceConstructionRawCapabilityOriginUsage(typedPackage, file, call)
			observed[site]++
			if allowed[site] == 0 {
				violations = append(violations,
					typedPackage.Fset.Position(call.Pos()).String()+
						" raw "+violationLabel+" origin outside audited site "+site,
				)
			}
			return true
		})
	}
	return observed, violations
}

func sourceConstructionRawCapabilityOriginUsage(
	typedPackage *packages.Package,
	file *ast.File,
	call *ast.CallExpr,
) string {
	parents := sourceConstructionParentNodes(file)
	var current ast.Node = call
	for {
		parent := parents[current]
		switch container := parent.(type) {
		case *ast.ParenExpr:
			current = container
		case *ast.KeyValueExpr:
			literal, ok := parents[container].(*ast.CompositeLit)
			currentExpression, expressionOK := current.(ast.Expr)
			key, keyOK := container.Key.(*ast.Ident)
			if !ok || !expressionOK || !keyOK ||
				!sourceConstructionDirectExpression(container.Value, currentExpression) {
				return "composite:<indirect>"
			}
			field, _ := typedPackage.TypesInfo.Uses[key].(*types.Var)
			if field == nil || !field.IsField() {
				return "composite:<unresolved-field>"
			}
			ownerRole := strings.TrimPrefix(
				sourceConstructionParameterRole(
					typedPackage.TypesInfo.TypeOf(literal),
					typedPackage.Types.Path(),
				),
				"*",
			)
			return "composite:" + ownerRole + "." + field.Name()
		case *ast.AssignStmt:
			targets := make([]string, 0, len(container.Lhs))
			for _, target := range container.Lhs {
				targets = append(
					targets,
					sourceConstructionRawCapabilityTargetRole(typedPackage, target),
				)
			}
			return container.Tok.String() + ":" + strings.Join(targets, ",")
		default:
			return sourceConstructionOwnerFactoryUsage(file, call)
		}
	}
}

func sourceConstructionRawCapabilityTargetRole(
	typedPackage *packages.Package,
	target ast.Expr,
) string {
	selector, ok := target.(*ast.SelectorExpr)
	if !ok {
		return types.ExprString(target)
	}
	selection := typedPackage.TypesInfo.Selections[selector]
	if selection == nil {
		return types.ExprString(target)
	}
	field, _ := selection.Obj().(*types.Var)
	if field == nil || !field.IsField() {
		return types.ExprString(target)
	}
	ownerRole := strings.TrimPrefix(
		sourceConstructionParameterRole(
			typedPackage.TypesInfo.TypeOf(selector.X),
			typedPackage.Types.Path(),
		),
		"*",
	)
	return ownerRole + "." + field.Name()
}

func sourceConstructionDirectOSRootResult(value types.Type) bool {
	value = types.Unalias(value)
	if tuple, ok := value.(*types.Tuple); ok {
		for variable := range tuple.Variables() {
			if sourceConstructionDirectOSRootResult(variable.Type()) {
				return true
			}
		}
		return false
	}
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "os" &&
		named.Obj().Name() == "Root"
}

type sourceConstructionRawFDOrigin struct {
	site     string
	file     *ast.File
	function *ast.FuncDecl
	call     *ast.CallExpr
}

func sourceConstructionRawDescriptorFDViolations(
	typedPackage *packages.Package,
) []string {
	// Raw descriptors are plain integers, so a file-local audit can be bypassed by
	// returning one through an otherwise unrelated package helper. Freeze every
	// Darwin opener reference package-wide, then prove the complete use topology
	// only for the two source-construction origins and their nominal-owner sinks.
	allowedOrigins := map[string]int{
		"authority_darwin.go|openFixedNullDeviceNoFollow|unix.Openat":                               1,
		"filesystem_darwin.go|openAbsoluteNoFollow|unix.Open":                                       1,
		"filesystem_darwin.go|openRelativeNoFollow|unix.Openat":                                     2,
		"preflight_authority_darwin.go|openPreflightAuthorityAbsoluteRoot|unix.Open":                1,
		"preflight_authority_darwin.go|openPreflightAuthorityNullRelativeNoFollow|unix.Openat":      1,
		"preflight_authority_darwin.go|openPreflightAuthorityRelativeNoFollow|unix.Openat":          1,
		"preflight_authority_darwin.go|openPreflightAuthorityTypedRelativeNoFollow|unix.Openat":     1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open": 1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|unix.Openat":                1,
	}
	allowedUses := map[string]int{
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open|call:os.NewFile[arg0:uintptr]": 1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open|call:unix.Close[arg0]":         1,
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open|move:invalidateDarwinSourceFD": 1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|unix.Openat|call:os.NewFile[arg0:uintptr]":                1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|unix.Openat|call:unix.Close[arg0]":                        1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|unix.Openat|move:invalidateDarwinSourceFD":                1,
	}
	allowedTransfers := map[string]int{
		"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|os.NewFile<-" +
			"source_primitives_darwin.go|(darwinSourcePrimitives).openPhysicalRootDescriptor|unix.Open": 1,
		"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|os.NewFile<-" +
			"source_primitives_darwin.go|openDarwinSourceRelativeDescriptor|unix.Openat": 1,
	}
	origins := make(map[types.Object]sourceConstructionRawFDOrigin)
	observedOrigins := make(map[string]int)
	parentsByFile := make(map[*ast.File]map[ast.Node]ast.Node)
	violations := make([]string, 0)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		parents := sourceConstructionParentNodes(file)
		parentsByFile[file] = parents
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			calledRole := sourceConstructionCallObjectRole(
				typedPackage.TypesInfo.Uses[identifier],
				typedPackage.Types.Path(),
			)
			if calledRole != "unix.Open" && calledRole != "unix.Openat" {
				return true
			}
			var reference ast.Expr = identifier
			if selector, selectorReference := parents[reference].(*ast.SelectorExpr); selectorReference && selector.Sel == identifier {
				reference = selector
			}
			for {
				parenthesized, parenthesizedReference := parents[reference].(*ast.ParenExpr)
				if !parenthesizedReference || parenthesized.X != reference {
					break
				}
				reference = parenthesized
			}
			call, directCall := parents[reference].(*ast.CallExpr)
			if directCall && sourceConstructionDirectExpression(call.Fun, reference) {
				return true
			}
			function := sourceConstructionEnclosingFunction(file, identifier.Pos())
			functionRole := "<outside-function>"
			if function != nil {
				functionRole = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			violations = append(violations,
				typedPackage.Fset.Position(identifier.Pos()).String()+
					" raw descriptor opener reference outside a direct call "+
					filename+"|"+functionRole+"|"+calledRole,
			)
			return true
		})
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			called := sourceConstructionCalledObject(typedPackage.TypesInfo, call.Fun)
			calledRole := sourceConstructionCallObjectRole(called, typedPackage.Types.Path())
			if calledRole != "unix.Open" && calledRole != "unix.Openat" {
				return true
			}
			function := sourceConstructionEnclosingFunction(file, call.Pos())
			functionRole := "<outside-function>"
			if function != nil {
				functionRole = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			site := filename + "|" + functionRole + "|" + calledRole
			observedOrigins[site]++
			if allowedOrigins[site] == 0 {
				violations = append(violations,
					typedPackage.Fset.Position(call.Pos()).String()+
						" raw descriptor fd origin outside audited site "+site,
				)
			}
			assignment := sourceConstructionDirectCallAssignment(file, call)
			if assignment == nil ||
				(assignment.Tok != token.DEFINE && assignment.Tok != token.ASSIGN) ||
				len(assignment.Lhs) != 2 {
				violations = append(violations,
					typedPackage.Fset.Position(call.Pos()).String()+
						" raw descriptor fd origin lacks an audited direct binding "+site+"|"+
						sourceConstructionOwnerFactoryUsage(file, call),
				)
				return true
			}
			identifier, ok := assignment.Lhs[0].(*ast.Ident)
			if !ok || identifier.Name == "_" {
				violations = append(violations,
					typedPackage.Fset.Position(call.Pos()).String()+
						" raw descriptor fd origin lacks an audited owner "+site+"|"+
						sourceConstructionOwnerFactoryUsage(file, call),
				)
				return true
			}
			object := typedPackage.TypesInfo.Defs[identifier]
			if object == nil {
				object = typedPackage.TypesInfo.Uses[identifier]
			}
			if object == nil {
				violations = append(violations,
					typedPackage.Fset.Position(call.Pos()).String()+
						" raw descriptor fd origin has an unresolved owner "+site,
				)
				return true
			}
			origins[object] = sourceConstructionRawFDOrigin{
				site:     site,
				file:     file,
				function: function,
				call:     call,
			}
			return true
		})
	}
	for site, want := range allowedOrigins {
		if got := observedOrigins[site]; got != want {
			violations = append(violations,
				"audited raw descriptor fd origin "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	observedTransfers := make(map[string]int)
	for _, file := range typedPackage.Syntax {
		filename := filepath.Base(typedPackage.Fset.Position(file.Package).Filename)
		if !strings.HasPrefix(filename, "source_primitives") {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || sourceConstructionCallObjectRole(
				sourceConstructionCalledObject(typedPackage.TypesInfo, call.Fun),
				typedPackage.Types.Path(),
			) != "os.NewFile" {
				return true
			}
			function := sourceConstructionEnclosingFunction(file, call.Pos())
			functionRole := "<outside-function>"
			if function != nil {
				functionRole = sourceConstructionFunctionRole(
					typedPackage.TypesInfo.Defs[function.Name],
					typedPackage.Types.Path(),
				)
			}
			sinkSite := filename + "|" + functionRole + "|os.NewFile"
			origin, proved := sourceConstructionRawFDTransferOrigin(
				typedPackage,
				call,
				origins,
			)
			if !proved {
				violations = append(violations,
					typedPackage.Fset.Position(call.Pos()).String()+
						" raw descriptor file transfer lacks exact fd provenance "+sinkSite,
				)
				return true
			}
			transferSite := sinkSite + "<-" + origin.site
			observedTransfers[transferSite]++
			if allowedTransfers[transferSite] == 0 {
				violations = append(violations,
					typedPackage.Fset.Position(call.Pos()).String()+
						" raw descriptor file transfer outside audited site "+transferSite,
				)
			}
			return true
		})
	}
	for site, want := range allowedTransfers {
		if got := observedTransfers[site]; got != want {
			violations = append(violations,
				"audited raw descriptor file transfer "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}

	observed := make(map[string]int)
	for file, parents := range parentsByFile {
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			origin, tracked := origins[typedPackage.TypesInfo.Uses[identifier]]
			if !tracked || !strings.HasPrefix(origin.site, "source_primitives") {
				return true
			}
			use := sourceConstructionRawFDUse(typedPackage, identifier, parents)
			site := origin.site + "|" + use
			observed[site]++
			if allowedUses[site] == 0 {
				violations = append(violations,
					typedPackage.Fset.Position(identifier.Pos()).String()+
						" raw descriptor fd outside audited site "+site,
				)
			}
			return true
		})
	}
	for site, want := range allowedUses {
		if got := observed[site]; got != want {
			violations = append(violations,
				"audited raw descriptor fd use "+site+" count = "+
					strconv.Itoa(got)+", want "+strconv.Itoa(want),
			)
		}
	}
	violations = append(
		violations,
		sourceConstructionRawFDTransferTopologyViolations(
			typedPackage,
			origins,
			parentsByFile,
		)...,
	)
	return violations
}

func sourceConstructionRawFDTransferTopologyViolations(
	typedPackage *packages.Package,
	origins map[types.Object]sourceConstructionRawFDOrigin,
	parentsByFile map[*ast.File]map[ast.Node]ast.Node,
) []string {
	violations := make([]string, 0)
	for object, origin := range origins {
		if !strings.HasPrefix(origin.site, "source_primitives_darwin.go|") {
			continue
		}
		if sourceConstructionRawFDTransferTopologyProved(
			typedPackage,
			object,
			origin,
			origins,
			parentsByFile[origin.file],
		) {
			continue
		}
		position := origin.site
		if origin.call != nil {
			position = typedPackage.Fset.Position(origin.call.Pos()).String()
		}
		violations = append(violations,
			position+" raw descriptor transfer/cleanup topology outside audited shape "+origin.site,
		)
	}
	slices.Sort(violations)
	return violations
}

func sourceConstructionRawFDTransferTopologyProved(
	typedPackage *packages.Package,
	object types.Object,
	origin sourceConstructionRawFDOrigin,
	origins map[types.Object]sourceConstructionRawFDOrigin,
	parents map[ast.Node]ast.Node,
) bool {
	if typedPackage == nil || typedPackage.Types == nil || typedPackage.TypesInfo == nil ||
		object == nil || origin.call == nil || origin.function == nil || origin.file == nil ||
		parents == nil || origin.function.Body == nil {
		return false
	}
	invalidateObject := typedPackage.Types.Scope().Lookup("invalidateDarwinSourceFD")
	if invalidateObject == nil {
		return false
	}
	transfers := make([]*ast.CallExpr, 0, 1)
	closes := make([]*ast.CallExpr, 0, 1)
	invalidations := make([]*ast.CallExpr, 0, 1)
	hasGoto := false
	ast.Inspect(origin.function.Body, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.BranchStmt:
			if current.Tok == token.GOTO {
				hasGoto = true
			}
		case *ast.CallExpr:
			called := sourceConstructionCalledObject(typedPackage.TypesInfo, current.Fun)
			role := sourceConstructionCallObjectRole(called, typedPackage.Types.Path())
			switch role {
			case "os.NewFile":
				transferOrigin, proved := sourceConstructionRawFDTransferOrigin(
					typedPackage,
					current,
					origins,
				)
				if proved && transferOrigin.call == origin.call {
					transfers = append(transfers, current)
				}
			case "unix.Close":
				if len(current.Args) == 1 && sourceConstructionDirectObjectExpression(
					typedPackage.TypesInfo,
					current.Args[0],
					object,
				) {
					closes = append(closes, current)
				}
			}
			if called == invalidateObject && len(current.Args) == 1 &&
				sourceConstructionDirectObjectAddress(
					typedPackage.TypesInfo,
					current.Args[0],
					object,
				) {
				invalidations = append(invalidations, current)
			}
		}
		return true
	})
	if hasGoto || len(transfers) != 1 || len(closes) != 1 || len(invalidations) != 1 {
		return false
	}

	ownerObject, transferStatement := sourceConstructionRawFDOwnerTransfer(
		typedPackage.TypesInfo,
		transfers[0],
		parents,
	)
	if ownerObject == nil || transferStatement == nil {
		return false
	}
	originStatement, originBlock := sourceConstructionDirectBlockStatement(origin.call, parents)
	provedTransferStatement, transferBlock := sourceConstructionDirectBlockStatement(
		transfers[0],
		parents,
	)
	closeStatement, closeBlock := sourceConstructionDirectBlockStatement(closes[0], parents)
	invalidationStatement, invalidationBlock := sourceConstructionDirectBlockStatement(
		invalidations[0],
		parents,
	)
	if originStatement == nil || provedTransferStatement != transferStatement ||
		closeStatement == nil || closeBlock == nil || invalidationStatement == nil {
		return false
	}
	cleanupBranch, cleanupBranchOK := parents[closeBlock].(*ast.IfStmt)
	if !cleanupBranchOK || cleanupBranch.Body != closeBlock {
		return false
	}
	cleanupStatement, cleanupBlock := sourceConstructionDirectBlockStatement(cleanupBranch, parents)
	if cleanupStatement != cleanupBranch || originBlock == nil ||
		originBlock != transferBlock || originBlock != cleanupBlock ||
		originBlock != invalidationBlock {
		return false
	}
	originIndex := sourceConstructionStatementIndex(originBlock, originStatement)
	transferIndex := sourceConstructionStatementIndex(originBlock, transferStatement)
	cleanupIndex := sourceConstructionStatementIndex(originBlock, cleanupBranch)
	invalidationIndex := sourceConstructionStatementIndex(originBlock, invalidationStatement)
	if originIndex < 0 || transferIndex <= originIndex ||
		cleanupIndex != transferIndex+1 || invalidationIndex != cleanupIndex+1 {
		return false
	}
	if !sourceConstructionRawFDAcquisitionPathProved(
		typedPackage,
		object,
		origin,
		originStatement,
		originBlock,
		originIndex,
		transferIndex,
		ownerObject,
		invalidationIndex,
	) {
		return false
	}
	return sourceConstructionRawFDNilCleanupBranchProved(
		typedPackage.TypesInfo,
		cleanupBranch,
		ownerObject,
		closes[0],
		closeStatement,
	) && sourceConstructionRawFDInvalidationStatementProved(
		typedPackage.TypesInfo,
		invalidationStatement,
		invalidations[0],
		ownerObject,
	)
}

func sourceConstructionRawFDAcquisitionPathProved(
	typedPackage *packages.Package,
	fdObject types.Object,
	origin sourceConstructionRawFDOrigin,
	originStatement ast.Stmt,
	originBlock *ast.BlockStmt,
	originIndex int,
	transferIndex int,
	ownerObject types.Object,
	invalidationIndex int,
) bool {
	assignment, ok := originStatement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 ||
		len(assignment.Rhs) != 1 || assignment.Rhs[0] != origin.call {
		return false
	}
	fdIdentifier, fdOK := assignment.Lhs[0].(*ast.Ident)
	errorIdentifier, errorOK := assignment.Lhs[1].(*ast.Ident)
	if !fdOK || !errorOK || typedPackage.TypesInfo.Defs[fdIdentifier] != fdObject {
		return false
	}
	errorObject := typedPackage.TypesInfo.Defs[errorIdentifier]
	if errorObject == nil {
		return false
	}
	fingerprint := sourceConstructionCallFingerprint(origin.call)
	switch fingerprint {
	case "unix.Open(physicalRootPath,flags,0)":
		if transferIndex != originIndex+3 || invalidationIndex+2 != len(originBlock.List)-1 ||
			!sourceConstructionRawFDEINTRBranchProved(
				typedPackage,
				originBlock.List[originIndex+1],
				errorObject,
				true,
			) ||
			!sourceConstructionRawFDErrorBranchProved(
				typedPackage,
				originBlock.List[originIndex+2],
				errorObject,
				errorIdentifier.Name,
				"sourceInitialRequired",
			) ||
			!sourceConstructionPostOpenContextBranchProved(
				typedPackage,
				originBlock.List[invalidationIndex+1],
				ownerObject,
				"OperationOpen",
			) {
			return false
		}
		return sourceConstructionOwnerSuccessReturnProved(
			typedPackage.TypesInfo,
			originBlock.List[invalidationIndex+2],
			ownerObject,
		)
	case "unix.Openat(int(parent.file.Fd()),name,flags,0)":
		if transferIndex != originIndex+4 || invalidationIndex+7 != len(originBlock.List)-1 ||
			!sourceConstructionDirectCallStatementFingerprint(
				originBlock.List[originIndex+1],
				"runtime.KeepAlive(parent.file)",
			) ||
			!sourceConstructionRawFDEINTRBranchProved(
				typedPackage,
				originBlock.List[originIndex+2],
				errorObject,
				false,
			) ||
			!sourceConstructionRawFDErrorBranchProved(
				typedPackage,
				originBlock.List[originIndex+3],
				errorObject,
				errorIdentifier.Name,
				"mode",
			) ||
			!sourceConstructionPostOpenContextBranchProved(
				typedPackage,
				originBlock.List[invalidationIndex+1],
				ownerObject,
				"OperationOpen",
			) ||
			!sourceConstructionPostOpenContextBranchProved(
				typedPackage,
				originBlock.List[invalidationIndex+2],
				ownerObject,
				"OperationProbe",
			) {
			return false
		}
		kindAssignment, ok := originBlock.List[invalidationIndex+3].(*ast.AssignStmt)
		if !ok || kindAssignment.Tok != token.DEFINE || len(kindAssignment.Lhs) != 1 ||
			len(kindAssignment.Rhs) != 1 {
			return false
		}
		kindIdentifier, ok := kindAssignment.Lhs[0].(*ast.Ident)
		kindCall, callOK := kindAssignment.Rhs[0].(*ast.CallExpr)
		if !ok || !callOK || kindIdentifier.Name != "kindErr" ||
			sourceConstructionCallFingerprint(kindCall) !=
				"requireDescriptorKind(int(owner.file.Fd()),openKind)" ||
			!sourceConstructionDirectCallStatementFingerprint(
				originBlock.List[invalidationIndex+4],
				"runtime.KeepAlive(owner.file)",
			) ||
			!sourceConstructionPostOpenContextBranchProved(
				typedPackage,
				originBlock.List[invalidationIndex+5],
				ownerObject,
				"OperationProbe",
			) {
			return false
		}
		kindObject := typedPackage.TypesInfo.Defs[kindIdentifier]
		kindBranch, ok := originBlock.List[invalidationIndex+6].(*ast.IfStmt)
		if kindObject == nil || !ok || !sourceConstructionDescriptorKindBranchProved(
			typedPackage,
			kindBranch,
			kindObject,
			ownerObject,
			true,
		) {
			return false
		}
		return sourceConstructionOwnerSuccessReturnProved(
			typedPackage.TypesInfo,
			originBlock.List[invalidationIndex+7],
			ownerObject,
		)
	default:
		return false
	}
}

func sourceConstructionRawFDEINTRBranchProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	errorObject types.Object,
	withContextCheck bool,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil {
		return false
	}
	condition, ok := sourceConstructionUnparenthesizedExpression(branch.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.EQL || types.ExprString(condition.Y) != "unix.EINTR" ||
		!sourceConstructionDirectObjectExpression(
			typedPackage.TypesInfo,
			condition.X,
			errorObject,
		) {
		return false
	}
	wantStatements := 1
	if withContextCheck {
		wantStatements = 2
	}
	if len(branch.Body.List) != wantStatements {
		return false
	}
	if withContextCheck && !sourceConstructionContextFailureNilReturnProved(
		typedPackage,
		branch.Body.List[0],
		"OperationOpen",
	) {
		return false
	}
	continued, ok := branch.Body.List[wantStatements-1].(*ast.BranchStmt)
	return ok && continued.Tok == token.CONTINUE && continued.Label == nil
}

func sourceConstructionContextFailureNilReturnProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	operation string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	assignment, ok := branch.Init.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 {
		return false
	}
	failure, ok := assignment.Lhs[0].(*ast.Ident)
	call, callOK := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || !callOK || failure.Name != "failure" ||
		sourceConstructionCallFingerprint(call) !=
			"sourceContextPrimitiveFailure(ctx,"+operation+")" {
		return false
	}
	failureObject := typedPackage.TypesInfo.Defs[failure]
	if failureObject == nil || !sourceConstructionObjectNilComparison(
		typedPackage.TypesInfo,
		branch.Cond,
		failureObject,
		token.NEQ,
	) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	return ok && len(returned.Results) == 2 &&
		sourceConstructionBareNilExpression(returned.Results[0]) &&
		sourceConstructionDirectObjectExpression(
			typedPackage.TypesInfo,
			returned.Results[1],
			failureObject,
		)
}

func sourceConstructionRawFDErrorBranchProved(
	typedPackage *packages.Package,
	statement ast.Stmt,
	errorObject types.Object,
	errorName string,
	mode string,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 ||
		!sourceConstructionObjectNilComparison(
			typedPackage.TypesInfo,
			branch.Cond,
			errorObject,
			token.NEQ,
		) {
		return false
	}
	returned, ok := branch.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 ||
		!sourceConstructionBareNilExpression(returned.Results[0]) {
		return false
	}
	classified, ok := returned.Results[1].(*ast.CallExpr)
	return ok && sourceConstructionCallFingerprint(classified) ==
		"classifyDarwinSourceOpenFailure(ctx,"+mode+","+errorName+")"
}

func sourceConstructionRawFDOwnerTransfer(
	info *types.Info,
	transfer *ast.CallExpr,
	parents map[ast.Node]ast.Node,
) (types.Object, ast.Stmt) {
	keyValue, ok := parents[transfer].(*ast.KeyValueExpr)
	if !ok || !sourceConstructionDirectExpression(keyValue.Value, transfer) {
		return nil, nil
	}
	key, ok := keyValue.Key.(*ast.Ident)
	if !ok || key.Name != "file" {
		return nil, nil
	}
	composite, ok := parents[keyValue].(*ast.CompositeLit)
	if !ok {
		return nil, nil
	}
	address, ok := parents[composite].(*ast.UnaryExpr)
	if !ok || address.Op != token.AND || address.X != composite {
		return nil, nil
	}
	assignment, ok := parents[address].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || assignment.Rhs[0] != address {
		return nil, nil
	}
	owner, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok {
		return nil, nil
	}
	return info.Defs[owner], assignment
}

func sourceConstructionRawFDNilCleanupBranchProved(
	info *types.Info,
	branch *ast.IfStmt,
	ownerObject types.Object,
	closeCall *ast.CallExpr,
	closeStatement ast.Stmt,
) bool {
	if branch == nil || branch.Else != nil ||
		!sourceConstructionOwnerFileNilCondition(info, branch.Cond, ownerObject) ||
		len(branch.Body.List) != 3 || branch.Body.List[0] != closeStatement {
		return false
	}
	assignment, ok := closeStatement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 ||
		len(assignment.Rhs) != 1 || assignment.Rhs[0] != closeCall {
		return false
	}
	closeError, ok := assignment.Lhs[0].(*ast.Ident)
	if !ok || info.Defs[closeError] == nil {
		return false
	}
	closeFailure, ok := branch.Body.List[1].(*ast.IfStmt)
	if !ok || closeFailure.Else != nil || len(closeFailure.Body.List) != 1 {
		return false
	}
	if !sourceConstructionNilFailureReturnProved(
		closeFailure.Body.List[0],
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)",
	) {
		return false
	}
	comparison, ok := sourceConstructionUnparenthesizedExpression(closeFailure.Cond).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.NEQ ||
		!sourceConstructionDirectObjectExpression(info, comparison.X, info.Defs[closeError]) ||
		!sourceConstructionNilExpression(info, comparison.Y) {
		return false
	}
	return sourceConstructionNilFailureReturnProved(
		branch.Body.List[2],
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)",
	)
}

func sourceConstructionRawFDInvalidationStatementProved(
	info *types.Info,
	statement ast.Stmt,
	invalidation *ast.CallExpr,
	ownerObject types.Object,
) bool {
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Init != nil || branch.Else != nil || len(branch.Body.List) != 1 {
		return false
	}
	condition, ok := sourceConstructionUnparenthesizedExpression(branch.Cond).(*ast.UnaryExpr)
	if !ok || condition.Op != token.NOT ||
		sourceConstructionUnparenthesizedExpression(condition.X) != invalidation {
		return false
	}
	return sourceConstructionOwnerFailureReturnProved(
		info,
		branch.Body.List[0],
		ownerObject,
		"newSourcePrimitiveFailure(OperationValidate,CauseInternalInvariant)",
	)
}

func sourceConstructionOwnerFileNilCondition(
	info *types.Info,
	expression ast.Expr,
	ownerObject types.Object,
) bool {
	comparison, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.EQL || !sourceConstructionNilExpression(info, comparison.Y) {
		return false
	}
	selector, ok := sourceConstructionUnparenthesizedExpression(comparison.X).(*ast.SelectorExpr)
	if !ok || !sourceConstructionDirectObjectExpression(info, selector.X, ownerObject) {
		return false
	}
	selection := info.Selections[selector]
	field, _ := selection.Obj().(*types.Var)
	return field != nil && field.IsField() && field.Name() == "file"
}

func sourceConstructionDirectObjectAddress(
	info *types.Info,
	expression ast.Expr,
	object types.Object,
) bool {
	address, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.UnaryExpr)
	return ok && address.Op == token.AND &&
		sourceConstructionDirectObjectExpression(info, address.X, object)
}

func sourceConstructionDirectObjectExpression(
	info *types.Info,
	expression ast.Expr,
	object types.Object,
) bool {
	identifier, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.Ident)
	if !ok || object == nil {
		return false
	}
	resolved := info.Uses[identifier]
	if resolved == nil {
		resolved = info.Defs[identifier]
	}
	return resolved == object
}

func sourceConstructionNilExpression(info *types.Info, expression ast.Expr) bool {
	identifier, ok := sourceConstructionUnparenthesizedExpression(expression).(*ast.Ident)
	return ok && identifier.Name == "nil" && info.Uses[identifier] == types.Universe.Lookup("nil")
}

func sourceConstructionUnparenthesizedExpression(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

func sourceConstructionDirectBlockStatement(
	node ast.Node,
	parents map[ast.Node]ast.Node,
) (ast.Stmt, *ast.BlockStmt) {
	for node != nil {
		if statement, ok := node.(ast.Stmt); ok {
			if block, direct := parents[statement].(*ast.BlockStmt); direct {
				return statement, block
			}
		}
		node = parents[node]
	}
	return nil, nil
}

func sourceConstructionStatementIndex(block *ast.BlockStmt, statement ast.Stmt) int {
	if block == nil || statement == nil {
		return -1
	}
	for index, candidate := range block.List {
		if candidate == statement {
			return index
		}
	}
	return -1
}

func sourceConstructionRawFDTransferOrigin(
	typedPackage *packages.Package,
	call *ast.CallExpr,
	origins map[types.Object]sourceConstructionRawFDOrigin,
) (sourceConstructionRawFDOrigin, bool) {
	if len(call.Args) < 1 {
		return sourceConstructionRawFDOrigin{}, false
	}
	conversion, directConversion := sourceConstructionDirectCallExpression(call.Args[0])
	if !directConversion || len(conversion.Args) != 1 ||
		sourceConstructionCalledObject(typedPackage.TypesInfo, conversion.Fun) !=
			types.Universe.Lookup("uintptr") {
		return sourceConstructionRawFDOrigin{}, false
	}
	argument := conversion.Args[0]
	for {
		parenthesized, ok := argument.(*ast.ParenExpr)
		if !ok {
			break
		}
		argument = parenthesized.X
	}
	identifier, directIdentifier := argument.(*ast.Ident)
	if !directIdentifier {
		return sourceConstructionRawFDOrigin{}, false
	}
	origin, tracked := origins[typedPackage.TypesInfo.Uses[identifier]]
	return origin, tracked
}

func sourceConstructionRawFDUse(
	typedPackage *packages.Package,
	identifier *ast.Ident,
	parents map[ast.Node]ast.Node,
) string {
	parent := parents[identifier]
	switch current := parent.(type) {
	case *ast.AssignStmt:
		for index, target := range current.Lhs {
			if !sourceConstructionDirectExpression(target, identifier) {
				continue
			}
			if index < len(current.Rhs) && types.ExprString(current.Rhs[index]) == "-1" {
				return "assign-invalid"
			}
			return "assign:<non-invalidating>"
		}
		return "assignment-source"
	case *ast.CallExpr:
		return "call:" + sourceConstructionDirectValueSink(
			typedPackage,
			identifier,
			parents,
		)
	case *ast.UnaryExpr:
		if current.Op != token.AND || !sourceConstructionDirectExpression(current.X, identifier) {
			return "<unclassified>"
		}
		call, ok := parents[current].(*ast.CallExpr)
		if !ok || len(call.Args) != 1 ||
			!sourceConstructionDirectExpression(call.Args[0], current) {
			return "address:<escaped>"
		}
		called := sourceConstructionCalledObject(typedPackage.TypesInfo, call.Fun)
		if called != typedPackage.Types.Scope().Lookup("invalidateDarwinSourceFD") {
			return "move-unapproved:" +
				sourceConstructionCallObjectRole(called, typedPackage.Types.Path())
		}
		return "move:" + sourceConstructionCallObjectRole(called, typedPackage.Types.Path()) +
			sourceConstructionCallBoundary(call, parents)
	default:
		return "<unclassified>"
	}
}

func sourceConstructionAcquisitionSite(
	typedPackage *packages.Package,
	file *ast.File,
	call *ast.CallExpr,
	called types.Object,
) string {
	function := sourceConstructionEnclosingFunction(file, call.Pos())
	functionRole := "<outside-function>"
	if function != nil {
		functionRole = sourceConstructionFunctionRole(
			typedPackage.TypesInfo.Defs[function.Name],
			typedPackage.Types.Path(),
		)
	}
	assignment := sourceConstructionDirectCallAssignment(file, call)
	if assignment == nil {
		return called.Name() + "|" + functionRole + "|<unbound>|<unbound>"
	}
	targets := make([]string, 0, len(assignment.Lhs))
	for _, target := range assignment.Lhs {
		targets = append(targets, types.ExprString(target))
	}
	return called.Name() + "|" + functionRole + "|" + assignment.Tok.String() + "|" +
		strings.Join(targets, ",")
}

func sourceConstructionOwnerFactorySite(
	typedPackage *packages.Package,
	file *ast.File,
	call *ast.CallExpr,
	called types.Object,
) string {
	function := sourceConstructionEnclosingFunction(file, call.Pos())
	functionRole := "<outside-function>"
	if function != nil {
		functionRole = sourceConstructionFunctionRole(
			typedPackage.TypesInfo.Defs[function.Name],
			typedPackage.Types.Path(),
		)
	}
	usage := sourceConstructionOwnerFactoryUsage(file, call)
	return called.Name() + "|" + functionRole + "|" + usage
}

func sourceConstructionOwnerFactoryUsage(file *ast.File, call *ast.CallExpr) string {
	usage := ""
	ast.Inspect(file, func(node ast.Node) bool {
		if usage != "" {
			return false
		}
		keyValue, ok := node.(*ast.KeyValueExpr)
		if !ok || !sourceConstructionDirectExpression(keyValue.Value, call) {
			return true
		}
		usage = "composite:" + types.ExprString(keyValue.Key)
		return false
	})
	if usage != "" {
		return usage
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if usage != "" {
			return false
		}
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range statement.Results {
			if sourceConstructionDirectExpression(result, call) {
				usage = "return"
				return false
			}
		}
		return true
	})
	if usage != "" {
		return usage
	}
	if assignment := sourceConstructionDirectCallAssignment(file, call); assignment != nil {
		targets := make([]string, 0, len(assignment.Lhs))
		for _, target := range assignment.Lhs {
			targets = append(targets, types.ExprString(target))
		}
		return assignment.Tok.String() + ":" + strings.Join(targets, ",")
	}
	return "<unbound>"
}

func sourceConstructionDirectExpression(expression ast.Expr, want ast.Expr) bool {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression == want
		}
		expression = parenthesized.X
	}
}

func sourceConstructionDirectCallAssignment(file *ast.File, call *ast.CallExpr) *ast.AssignStmt {
	var found *ast.AssignStmt
	ast.Inspect(file, func(node ast.Node) bool {
		if found != nil {
			return false
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Rhs) != 1 {
			return true
		}
		if sourceConstructionDirectExpression(assignment.Rhs[0], call) {
			found = assignment
			return false
		}
		return true
	})
	return found
}

func sourceConstructionAllowMethods(
	typedPackage *packages.Package,
	allowed map[types.Object]bool,
	typeName string,
	methodNames map[string]bool,
) {
	object := typedPackage.Types.Scope().Lookup(typeName)
	if object == nil {
		return
	}
	for _, receiver := range []types.Type{object.Type(), types.NewPointer(object.Type())} {
		methods := types.NewMethodSet(receiver)
		for selection := range methods.Methods() {
			method := selection.Obj()
			if methodNames == nil || methodNames[method.Name()] {
				allowed[method] = true
			}
		}
	}
}

type sourceConstructionMethodContract struct {
	parameters []string
	results    []string
	variadic   bool
}

func sourceConstructionAllowPrimitiveMethods(
	typedPackage *packages.Package,
	allowed map[types.Object]bool,
	acquisitions map[types.Object]bool,
	sensitiveNames map[string]bool,
) []string {
	object := typedPackage.Types.Scope().Lookup("sourcePrimitives")
	if object == nil {
		return []string{"sourcePrimitives type is missing"}
	}
	named, _ := types.Unalias(object.Type()).(*types.Named)
	if named == nil {
		return []string{"sourcePrimitives is not a named interface"}
	}
	contract, _ := named.Underlying().(*types.Interface)
	if contract == nil {
		return []string{"sourcePrimitives is not an interface"}
	}
	contract.Complete()
	expected := map[string]sourceConstructionMethodContract{
		"openRepositoryRoot": {
			parameters: []string{"context.Context", "sourceRepositoryLocator"},
			results:    []string{"*ownedSourceRoot", "*sourcePrimitiveFailure"},
		},
		"openPhysicalRootDescriptor": {
			parameters: []string{"context.Context"},
			results:    []string{"*ownedSourceDescriptor", "*sourcePrimitiveFailure"},
		},
		"probeRelativeKind": {
			parameters: []string{"context.Context", "*ownedSourceDescriptor", "string", "sourcePresenceMode"},
			results:    []string{"sourceObservedKind", "bool", "*sourcePrimitiveFailure"},
		},
		"openRelativeNoFollow": {
			parameters: []string{
				"context.Context",
				"*ownedSourceDescriptor",
				"string",
				"sourceObservedKind",
				"sourcePresenceMode",
			},
			results: []string{"*ownedSourceDescriptor", "*sourcePrimitiveFailure"},
		},
		"openChildRoot": {
			parameters: []string{"context.Context", "*ownedSourceRoot", "string"},
			results:    []string{"*ownedSourceRoot", "*sourcePrimitiveFailure"},
		},
		"openRootDirectoryDescriptor": {
			parameters: []string{"context.Context", "*ownedSourceRoot"},
			results:    []string{"*ownedSourceDescriptor", "*sourcePrimitiveFailure"},
		},
		"readDirectoryBatch": {
			parameters: []string{"context.Context", "*ownedSourceDescriptor"},
			results:    []string{"[]string", "bool", "*sourcePrimitiveFailure"},
		},
		"statDescriptor": {
			parameters: []string{"context.Context", "*ownedSourceDescriptor"},
			results:    []string{"fileSnapshot", "*sourcePrimitiveFailure"},
		},
		"statFilesystem": {
			parameters: []string{"context.Context", "*ownedSourceDescriptor"},
			results:    []string{"mountSnapshot", "*sourcePrimitiveFailure"},
		},
		"validateFilesystem": {
			parameters: []string{"context.Context", "mountSnapshot"},
			results:    []string{"*sourcePrimitiveFailure"},
		},
		"acquireRawACL": {
			parameters: []string{"context.Context", "*ownedSourceDescriptor"},
			results:    []string{"[]byte", "*sourcePrimitiveFailure"},
		},
		"parseRawACL": {
			parameters: []string{"context.Context", "[]byte"},
			results:    []string{"parsedSourceACL", "*sourcePrimitiveFailure"},
		},
		"validateACL": {
			parameters: []string{"context.Context", "parsedSourceACL"},
			results:    []string{"*sourcePrimitiveFailure"},
		},
		"readExactForParse": {
			parameters: []string{"context.Context", "*ownedSourceDescriptor", "[]byte"},
			results:    []string{"*sourcePrimitiveFailure"},
		},
		"hashBytes": {
			parameters: []string{"context.Context", "[]byte"},
			results:    []string{"Digest", "*sourcePrimitiveFailure"},
		},
		"compareRootAndDescriptor": {
			parameters: []string{"context.Context", "*ownedSourceRoot", "*ownedSourceDescriptor"},
			results:    []string{"*sourcePrimitiveFailure"},
		},
		"closeRoot": {
			parameters: []string{"*ownedSourceRoot"},
			results:    []string{"bool"},
		},
		"closeDescriptor": {
			parameters: []string{"*ownedSourceDescriptor"},
			results:    []string{"bool"},
		},
		"privateSourcePrimitives": {},
	}
	violations := make([]string, 0)
	seen := make(map[string]bool)
	for method := range contract.Methods() {
		methodContract, admitted := expected[method.Name()]
		signature, _ := method.Type().(*types.Signature)
		valid := admitted && signature != nil &&
			signature.Variadic() == methodContract.variadic && sourceConstructionTupleHasRoles(
			signature.Params(),
			methodContract.parameters,
			typedPackage.Types.Path(),
		) && sourceConstructionTupleHasRoles(
			signature.Results(),
			methodContract.results,
			typedPackage.Types.Path(),
		)
		if !valid {
			violations = append(violations,
				typedPackage.Fset.Position(method.Pos()).String()+
					" sourcePrimitives declares non-primitive method or signature "+method.Name(),
			)
			continue
		}
		seen[method.Name()] = true
		allowed[method] = true
		if sourceConstructionSensitiveType(
			signature.Results(),
			typedPackage.Types.Path(),
			sensitiveNames,
			map[types.Type]bool{},
		) != "" {
			acquisitions[method] = true
		}
	}
	for method := range expected {
		if !seen[method] {
			violations = append(violations, "sourcePrimitives is missing exact method "+method)
		}
	}
	if contract.NumMethods() != len(expected) {
		violations = append(violations,
			"sourcePrimitives method count = "+strconv.Itoa(contract.NumMethods())+
				", want "+strconv.Itoa(len(expected)),
		)
	}
	return violations
}

func sourceConstructionTupleHasRoles(
	tuple *types.Tuple,
	want []string,
	packagePath string,
) bool {
	if tuple == nil || tuple.Len() != len(want) {
		return tuple == nil && len(want) == 0
	}
	for index := range tuple.Len() {
		if sourceConstructionParameterRole(tuple.At(index).Type(), packagePath) != want[index] {
			return false
		}
	}
	return true
}

func sourceConstructionAllowHandleCloserMethods(
	typedPackage *packages.Package,
	allowed map[types.Object]bool,
) []string {
	object := typedPackage.Types.Scope().Lookup("sourceHandleCloser")
	if object == nil {
		return []string{"sourceHandleCloser type is missing"}
	}
	named, _ := types.Unalias(object.Type()).(*types.Named)
	if named == nil {
		return []string{"sourceHandleCloser is not a named interface"}
	}
	contract, _ := named.Underlying().(*types.Interface)
	if contract == nil {
		return []string{"sourceHandleCloser is not an interface"}
	}
	contract.Complete()
	expected := map[string]string{
		"closeRoot":       "*ownedSourceRoot",
		"closeDescriptor": "*ownedSourceDescriptor",
	}
	violations := make([]string, 0)
	seen := make(map[string]bool)
	for method := range contract.Methods() {
		parameterRole, admitted := expected[method.Name()]
		signature, _ := method.Type().(*types.Signature)
		valid := admitted && signature != nil && signature.Params().Len() == 1 &&
			sourceConstructionParameterRole(
				signature.Params().At(0).Type(),
				typedPackage.Types.Path(),
			) == parameterRole && signature.Results().Len() == 1 &&
			sourceConstructionBooleanType(signature.Results().At(0).Type())
		if !valid {
			violations = append(violations,
				typedPackage.Fset.Position(method.Pos()).String()+
					" sourceHandleCloser declares non-close method or signature "+method.Name(),
			)
			continue
		}
		seen[method.Name()] = true
		allowed[method] = true
	}
	for method := range expected {
		if !seen[method] {
			violations = append(violations, "sourceHandleCloser is missing exact method "+method)
		}
	}
	if contract.NumMethods() != len(expected) {
		violations = append(violations,
			"sourceHandleCloser method count = "+strconv.Itoa(contract.NumMethods())+
				", want "+strconv.Itoa(len(expected)),
		)
	}
	return violations
}

func sourceConstructionBooleanType(value types.Type) bool {
	basic, _ := types.Unalias(value).Underlying().(*types.Basic)
	return basic != nil && basic.Kind() == types.Bool
}

func sourceConstructionReceiverDeclaredInModule(
	typedPackage *packages.Package,
	function *ast.FuncDecl,
) bool {
	if function.Recv == nil || len(function.Recv.List) != 1 {
		return function.Recv == nil
	}
	receiverType := types.Unalias(typedPackage.TypesInfo.TypeOf(function.Recv.List[0].Type))
	if pointer, ok := receiverType.(*types.Pointer); ok {
		receiverType = types.Unalias(pointer.Elem())
	}
	named, ok := receiverType.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != typedPackage.Types.Path() {
		return false
	}
	filename := typedPackage.Fset.Position(named.Obj().Pos()).Filename
	return strings.HasPrefix(filepath.Base(filename), "source_construction")
}

func sourceConstructionLocalObjects(
	typedPackage *packages.Package,
	files []*ast.File,
) map[types.Object]bool {
	local := make(map[types.Object]bool)
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch current := node.(type) {
				case *ast.AssignStmt:
					if current.Tok != token.DEFINE {
						return true
					}
					for _, target := range current.Lhs {
						identifier, ok := target.(*ast.Ident)
						if ok && identifier.Name != "_" {
							if object := typedPackage.TypesInfo.Defs[identifier]; object != nil {
								local[object] = true
							}
						}
					}
				case *ast.ValueSpec:
					for _, identifier := range current.Names {
						if object := typedPackage.TypesInfo.Defs[identifier]; object != nil {
							local[object] = true
						}
					}
				case *ast.RangeStmt:
					if current.Tok != token.DEFINE {
						return true
					}
					for _, expression := range []ast.Expr{current.Key, current.Value} {
						identifier, ok := expression.(*ast.Ident)
						if ok && identifier.Name != "_" {
							if object := typedPackage.TypesInfo.Defs[identifier]; object != nil {
								local[object] = true
							}
						}
					}
				}
				return true
			})
		}
	}
	return local
}

func sourceConstructionSensitiveCallValue(
	info *types.Info,
	call *ast.CallExpr,
	packagePath string,
	sensitiveNames map[string]bool,
) string {
	if selector, ok := sourceConstructionUnwrapCallFunction(call.Fun).(*ast.SelectorExpr); ok {
		if selection := info.Selections[selector]; selection != nil {
			if escaped := sourceConstructionSensitiveExpression(
				info,
				selector.X,
				packagePath,
				sensitiveNames,
			); escaped != "" {
				return escaped
			}
			if escaped := sourceConstructionSensitiveType(
				selection.Recv(),
				packagePath,
				sensitiveNames,
				map[types.Type]bool{},
			); escaped != "" {
				return escaped
			}
		}
	}
	for _, argument := range call.Args {
		if escaped := sourceConstructionSensitiveExpression(
			info,
			argument,
			packagePath,
			sensitiveNames,
		); escaped != "" {
			return escaped
		}
	}
	return ""
}

func sourceConstructionNestedSensitiveResult(
	info *types.Info,
	call *ast.CallExpr,
	packagePath string,
	sensitiveNames map[string]bool,
) string {
	candidates := append([]ast.Expr(nil), call.Args...)
	if selector, ok := sourceConstructionUnwrapCallFunction(call.Fun).(*ast.SelectorExpr); ok {
		if selection := info.Selections[selector]; selection != nil &&
			selection.Kind() == types.MethodVal {
			candidates = append(candidates, selector.X)
		}
	}
	for _, candidate := range candidates {
		var escaped string
		ast.Inspect(candidate, func(node ast.Node) bool {
			if escaped != "" {
				return false
			}
			nested, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			escaped = sourceConstructionSensitiveType(
				info.TypeOf(nested),
				packagePath,
				sensitiveNames,
				map[types.Type]bool{},
			)
			return escaped == ""
		})
		if escaped != "" {
			return escaped
		}
	}
	return ""
}

func sourceConstructionCalledObject(info *types.Info, expression ast.Expr) types.Object {
	expression = sourceConstructionUnwrapCallFunction(expression)
	switch current := expression.(type) {
	case *ast.Ident:
		if object := info.Uses[current]; object != nil {
			return object
		}
		return info.Defs[current]
	case *ast.SelectorExpr:
		if selection := info.Selections[current]; selection != nil {
			return selection.Obj()
		}
		return info.Uses[current.Sel]
	}
	return nil
}

func sourceConstructionSensitiveBuiltinAllowed(
	typedPackage *packages.Package,
	builtin *types.Builtin,
	call *ast.CallExpr,
	sensitiveNames map[string]bool,
	localObjects map[types.Object]bool,
) bool {
	switch builtin.Name() {
	case "len", "cap", "make", "new":
		return true
	case "append":
		if len(call.Args) == 0 {
			return false
		}
		containerType := sourceConstructionSensitiveType(
			typedPackage.TypesInfo.TypeOf(call.Args[0]),
			typedPackage.Types.Path(),
			sensitiveNames,
			map[types.Type]bool{},
		)
		if containerType == "" || containerType == "interface" {
			return false
		}
		root := sourceConstructionStorageRootObject(typedPackage.TypesInfo, call.Args[0])
		return localObjects[root]
	default:
		return false
	}
}

func sourceConstructionUnwrapCallFunction(expression ast.Expr) ast.Expr {
	for {
		switch current := expression.(type) {
		case *ast.IndexExpr:
			expression = current.X
		case *ast.IndexListExpr:
			expression = current.X
		case *ast.ParenExpr:
			expression = current.X
		default:
			return expression
		}
	}
}

func sourceConstructionEnclosingFunction(file *ast.File, position token.Pos) *ast.FuncDecl {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Body != nil && function.Body.Pos() <= position && position <= function.Body.End() {
			return function
		}
	}
	return nil
}

func sourceConstructionAssignmentEscapeViolations(
	typedPackage *packages.Package,
	targets []ast.Expr,
	values []ast.Expr,
	sensitiveNames map[string]bool,
	localObjects map[types.Object]bool,
) []string {
	if len(targets) == 0 || len(values) == 0 {
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
			valueType := typedPackage.TypesInfo.TypeOf(value)
			for _, target := range targets {
				pairs = append(pairs, assignmentPair{
					target:     target,
					value:      valueType,
					expression: value,
				})
			}
		}
	}

	violations := make([]string, 0)
	for _, pair := range pairs {
		if violation := sourceConstructionMapIndexKeyViolation(
			typedPackage,
			pair.target,
			sensitiveNames,
		); violation != "" {
			violations = append(violations, violation)
		}
		escaped := sourceConstructionSensitiveType(
			pair.value,
			typedPackage.Types.Path(),
			sensitiveNames,
			map[types.Type]bool{},
		)
		if pair.expression != nil {
			escaped = sourceConstructionSensitiveExpression(
				typedPackage.TypesInfo,
				pair.expression,
				typedPackage.Types.Path(),
				sensitiveNames,
			)
		}
		if escaped == "" || sourceConstructionSafeAssignmentTarget(
			typedPackage,
			pair.target,
			sensitiveNames,
			localObjects,
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

func sourceConstructionMapIndexKeyViolation(
	typedPackage *packages.Package,
	target ast.Expr,
	sensitiveNames map[string]bool,
) string {
	index, isMapIndex := sourceConstructionMapIndexTarget(typedPackage.TypesInfo, target)
	if !isMapIndex {
		return ""
	}
	escaped := sourceConstructionSensitiveExpression(
		typedPackage.TypesInfo,
		index.Index,
		typedPackage.Types.Path(),
		sensitiveNames,
	)
	if escaped == "" {
		return ""
	}
	return typedPackage.Fset.Position(index.Index.Pos()).String() +
		" stores " + escaped + " as a map key in " + types.ExprString(index.X)
}

func sourceConstructionMapIndexTarget(info *types.Info, target ast.Expr) (*ast.IndexExpr, bool) {
	for {
		parenthesized, ok := target.(*ast.ParenExpr)
		if !ok {
			break
		}
		target = parenthesized.X
	}
	index, ok := target.(*ast.IndexExpr)
	if !ok {
		return nil, false
	}
	container := info.TypeOf(index.X)
	if container == nil {
		return nil, false
	}
	_, isMap := types.Unalias(container).Underlying().(*types.Map)
	return index, isMap
}

func sourceConstructionSafeAssignmentTarget(
	typedPackage *packages.Package,
	target ast.Expr,
	sensitiveNames map[string]bool,
	localObjects map[types.Object]bool,
) bool {
	targetType := sourceConstructionSensitiveType(
		typedPackage.TypesInfo.TypeOf(target),
		typedPackage.Types.Path(),
		sensitiveNames,
		map[types.Type]bool{},
	)
	if targetType == "" || targetType == "interface" {
		return false
	}
	object := sourceConstructionStorageRootObject(typedPackage.TypesInfo, target)
	if localObjects[object] {
		return true
	}
	variable, ok := object.(*types.Var)
	return ok && sourceConstructionDirectSealedType(
		variable.Type(),
		typedPackage.Types.Path(),
		sensitiveNames,
	)
}

func sourceConstructionStorageRootObject(info *types.Info, expression ast.Expr) types.Object {
	switch current := expression.(type) {
	case *ast.Ident:
		if object := info.Defs[current]; object != nil {
			return object
		}
		return info.Uses[current]
	case *ast.SelectorExpr:
		return sourceConstructionStorageRootObject(info, current.X)
	case *ast.IndexExpr:
		return sourceConstructionStorageRootObject(info, current.X)
	case *ast.StarExpr:
		return sourceConstructionStorageRootObject(info, current.X)
	case *ast.ParenExpr:
		return sourceConstructionStorageRootObject(info, current.X)
	}
	return nil
}

func sourceConstructionSensitiveExpression(
	info *types.Info,
	expression ast.Expr,
	packagePath string,
	sensitiveNames map[string]bool,
) string {
	switch current := expression.(type) {
	case *ast.ParenExpr:
		return sourceConstructionSensitiveExpression(info, current.X, packagePath, sensitiveNames)
	case *ast.CompositeLit:
		for _, element := range current.Elts {
			if escaped := sourceConstructionSensitiveExpression(
				info,
				element,
				packagePath,
				sensitiveNames,
			); escaped != "" {
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
			if escaped := sourceConstructionSensitiveExpression(
				info,
				current.Key,
				packagePath,
				sensitiveNames,
			); escaped != "" {
				return escaped
			}
		}
		return sourceConstructionSensitiveExpression(info, current.Value, packagePath, sensitiveNames)
	case *ast.CallExpr:
		called := sourceConstructionCalledObject(info, current.Fun)
		_, conversion := called.(*types.TypeName)
		builtin, builtinCall := called.(*types.Builtin)
		preservesValues := builtinCall && (builtin.Name() == "append" || builtin.Name() == "copy")
		if conversion || preservesValues {
			for _, argument := range current.Args {
				if escaped := sourceConstructionSensitiveExpression(
					info,
					argument,
					packagePath,
					sensitiveNames,
				); escaped != "" {
					return escaped
				}
			}
		}
	case *ast.SelectorExpr:
		if selection := info.Selections[current]; selection != nil &&
			selection.Kind() == types.MethodVal {
			if escaped := sourceConstructionSensitiveType(
				selection.Recv(),
				packagePath,
				sensitiveNames,
				map[types.Type]bool{},
			); escaped != "" {
				return escaped
			}
		}
	}
	if escaped := sourceConstructionSensitiveType(
		info.TypeOf(expression),
		packagePath,
		sensitiveNames,
		map[types.Type]bool{},
	); escaped != "" {
		return escaped
	}
	if sourceConstructionAliasBearingType(info.TypeOf(expression), packagePath, map[types.Type]bool{}) {
		if escaped := sourceConstructionSensitiveStorageOrigin(
			info,
			expression,
			packagePath,
			sensitiveNames,
		); escaped != "" {
			return escaped
		}
	}
	return ""
}

func sourceConstructionSensitiveStorageOrigin(
	info *types.Info,
	expression ast.Expr,
	packagePath string,
	sensitiveNames map[string]bool,
) string {
	var parent ast.Expr
	switch current := expression.(type) {
	case *ast.ParenExpr:
		parent = current.X
	case *ast.SelectorExpr:
		parent = current.X
	case *ast.IndexExpr:
		parent = current.X
	case *ast.IndexListExpr:
		parent = current.X
	case *ast.SliceExpr:
		parent = current.X
	case *ast.StarExpr:
		parent = current.X
	case *ast.UnaryExpr:
		if current.Op == token.AND {
			parent = current.X
		}
	}
	if parent == nil {
		return sourceConstructionSensitiveType(
			info.TypeOf(expression),
			packagePath,
			sensitiveNames,
			map[types.Type]bool{},
		)
	}
	if escaped := sourceConstructionSensitiveType(
		info.TypeOf(parent),
		packagePath,
		sensitiveNames,
		map[types.Type]bool{},
	); escaped != "" {
		return escaped
	}
	return sourceConstructionSensitiveStorageOrigin(
		info,
		parent,
		packagePath,
		sensitiveNames,
	)
}

func sourceConstructionAliasBearingType(
	value types.Type,
	packagePath string,
	seen map[types.Type]bool,
) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	switch current := value.(type) {
	case *types.Named:
		object := current.Obj()
		if object.Pkg() != nil && object.Pkg().Path() == packagePath &&
			object.Name() == "sourceUseOutcome" {
			return false
		}
		if _, isInterface := current.Underlying().(*types.Interface); isInterface &&
			sourceConstructionSafeInterface(current, packagePath) {
			return false
		}
		return sourceConstructionAliasBearingType(current.Underlying(), packagePath, seen)
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		return true
	case *types.Array:
		return sourceConstructionAliasBearingType(current.Elem(), packagePath, seen)
	case *types.Struct:
		for field := range current.Fields() {
			if sourceConstructionAliasBearingType(field.Type(), packagePath, seen) {
				return true
			}
		}
	case *types.Tuple:
		for variable := range current.Variables() {
			if sourceConstructionAliasBearingType(variable.Type(), packagePath, seen) {
				return true
			}
		}
	case *types.TypeParam:
		return sourceConstructionAliasBearingType(current.Constraint(), packagePath, seen)
	case *types.Union:
		for term := range current.Terms() {
			if sourceConstructionAliasBearingType(term.Type(), packagePath, seen) {
				return true
			}
		}
	}
	return false
}

func sourceConstructionSensitiveClosureValue(
	info *types.Info,
	function *ast.FuncLit,
	packagePath string,
	sensitiveNames map[string]bool,
) string {
	var escaped string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if escaped != "" {
			return false
		}
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		escaped = sourceConstructionSensitiveExpression(
			info,
			expression,
			packagePath,
			sensitiveNames,
		)
		return escaped == ""
	})
	return escaped
}

func sourceConstructionDirectOwnerType(value types.Type, packagePath string) bool {
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == packagePath &&
		named.Obj().Name() == "sourceConstructionOwner"
}

func sourceConstructionDirectSealedType(
	value types.Type,
	packagePath string,
	sensitiveNames map[string]bool,
) bool {
	value = types.Unalias(value)
	for {
		pointer, ok := value.(*types.Pointer)
		if !ok {
			break
		}
		value = types.Unalias(pointer.Elem())
	}
	named, ok := value.(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == packagePath &&
		sensitiveNames[named.Obj().Name()]
}

func sourceConstructionPermittedOwnerReturn(
	value types.Type,
	packagePath string,
	sensitiveNames map[string]bool,
) bool {
	value = types.Unalias(value)
	if tuple, ok := value.(*types.Tuple); ok {
		ownerFound := false
		for variable := range tuple.Variables() {
			if sourceConstructionDirectOwnerType(variable.Type(), packagePath) {
				ownerFound = true
				continue
			}
			if sourceConstructionSensitiveType(
				variable.Type(),
				packagePath,
				sensitiveNames,
				map[types.Type]bool{},
			) != "" {
				return false
			}
		}
		return ownerFound
	}
	return sourceConstructionDirectOwnerType(value, packagePath)
}

func sourceConstructionTypeName(value types.Type) string {
	if value == nil {
		return "<nil>"
	}
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	if named, ok := value.(*types.Named); ok {
		return named.Obj().Name()
	}
	return types.TypeString(value, nil)
}

func sourceConstructionParameterRole(value types.Type, packagePath string) string {
	value = types.Unalias(value)
	switch current := value.(type) {
	case *types.Pointer:
		return "*" + sourceConstructionParameterRole(current.Elem(), packagePath)
	case *types.Slice:
		return "[]" + sourceConstructionParameterRole(current.Elem(), packagePath)
	case *types.Named:
		object := current.Obj()
		if object.Pkg() != nil && object.Pkg().Path() == packagePath {
			return object.Name()
		}
	}
	return types.TypeString(value, nil)
}

func sourceConstructionFunctionRole(object types.Object, packagePath string) string {
	if object == nil {
		return ""
	}
	signature, _ := object.Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return object.Name()
	}
	return "(" + sourceConstructionParameterRole(signature.Recv().Type(), packagePath) + ")." +
		object.Name()
}

func sourceConstructionSensitiveType(
	value types.Type,
	packagePath string,
	sensitiveNames map[string]bool,
	seen map[types.Type]bool,
) string {
	if value == nil {
		return ""
	}
	value = types.Unalias(value)
	if seen[value] {
		return ""
	}
	seen[value] = true
	switch current := value.(type) {
	case *types.Named:
		object := current.Obj()
		if object.Pkg() != nil {
			if object.Pkg().Path() == "os" && (object.Name() == "File" || object.Name() == "Root") {
				return "os." + object.Name()
			}
			if object.Pkg().Path() == packagePath && sensitiveNames[object.Name()] {
				return object.Name()
			}
		}
		if _, isInterface := current.Underlying().(*types.Interface); isInterface {
			if sourceConstructionSafeInterface(current, packagePath) {
				return ""
			}
			return "interface"
		}
		return sourceConstructionSensitiveType(current.Underlying(), packagePath, sensitiveNames, seen)
	case *types.Pointer:
		return sourceConstructionSensitiveType(current.Elem(), packagePath, sensitiveNames, seen)
	case *types.Array:
		return sourceConstructionSensitiveType(current.Elem(), packagePath, sensitiveNames, seen)
	case *types.Slice:
		return sourceConstructionSensitiveType(current.Elem(), packagePath, sensitiveNames, seen)
	case *types.Map:
		if escaped := sourceConstructionSensitiveType(current.Key(), packagePath, sensitiveNames, seen); escaped != "" {
			return escaped
		}
		return sourceConstructionSensitiveType(current.Elem(), packagePath, sensitiveNames, seen)
	case *types.Chan:
		return sourceConstructionSensitiveType(current.Elem(), packagePath, sensitiveNames, seen)
	case *types.Struct:
		for field := range current.Fields() {
			if escaped := sourceConstructionSensitiveType(
				field.Type(),
				packagePath,
				sensitiveNames,
				seen,
			); escaped != "" {
				return escaped
			}
		}
	case *types.Signature:
		if current.Recv() != nil {
			if escaped := sourceConstructionSensitiveType(
				current.Recv().Type(),
				packagePath,
				sensitiveNames,
				seen,
			); escaped != "" {
				return escaped
			}
		}
		if escaped := sourceConstructionSensitiveType(current.Params(), packagePath, sensitiveNames, seen); escaped != "" {
			return escaped
		}
		return sourceConstructionSensitiveType(current.Results(), packagePath, sensitiveNames, seen)
	case *types.Tuple:
		for variable := range current.Variables() {
			if escaped := sourceConstructionSensitiveType(
				variable.Type(),
				packagePath,
				sensitiveNames,
				seen,
			); escaped != "" {
				return escaped
			}
		}
	case *types.Interface:
		current.Complete()
		return "interface"
	case *types.TypeParam:
		return sourceConstructionSensitiveType(current.Constraint(), packagePath, sensitiveNames, seen)
	case *types.Union:
		for term := range current.Terms() {
			if escaped := sourceConstructionSensitiveType(
				term.Type(),
				packagePath,
				sensitiveNames,
				seen,
			); escaped != "" {
				return escaped
			}
		}
	}
	return ""
}

func sourceConstructionContainsNamedType(
	value types.Type,
	packagePath string,
	typeName string,
	seen map[types.Type]bool,
) bool {
	if value == nil {
		return false
	}
	value = types.Unalias(value)
	if seen[value] {
		return false
	}
	seen[value] = true
	switch current := value.(type) {
	case *types.Named:
		object := current.Obj()
		if object.Pkg() != nil && object.Pkg().Path() == packagePath && object.Name() == typeName {
			return true
		}
		return sourceConstructionContainsNamedType(
			current.Underlying(),
			packagePath,
			typeName,
			seen,
		)
	case *types.Pointer:
		return sourceConstructionContainsNamedType(current.Elem(), packagePath, typeName, seen)
	case *types.Array:
		return sourceConstructionContainsNamedType(current.Elem(), packagePath, typeName, seen)
	case *types.Slice:
		return sourceConstructionContainsNamedType(current.Elem(), packagePath, typeName, seen)
	case *types.Map:
		return sourceConstructionContainsNamedType(current.Key(), packagePath, typeName, seen) ||
			sourceConstructionContainsNamedType(current.Elem(), packagePath, typeName, seen)
	case *types.Chan:
		return sourceConstructionContainsNamedType(current.Elem(), packagePath, typeName, seen)
	case *types.Struct:
		for field := range current.Fields() {
			if sourceConstructionContainsNamedType(field.Type(), packagePath, typeName, seen) {
				return true
			}
		}
	case *types.Signature:
		if current.Recv() != nil && sourceConstructionContainsNamedType(
			current.Recv().Type(),
			packagePath,
			typeName,
			seen,
		) {
			return true
		}
		return sourceConstructionContainsNamedType(current.Params(), packagePath, typeName, seen) ||
			sourceConstructionContainsNamedType(current.Results(), packagePath, typeName, seen)
	case *types.Tuple:
		for variable := range current.Variables() {
			if sourceConstructionContainsNamedType(variable.Type(), packagePath, typeName, seen) {
				return true
			}
		}
	case *types.Interface:
		current.Complete()
		for method := range current.Methods() {
			if sourceConstructionContainsNamedType(method.Type(), packagePath, typeName, seen) {
				return true
			}
		}
	case *types.TypeParam:
		return sourceConstructionContainsNamedType(current.Constraint(), packagePath, typeName, seen)
	case *types.Union:
		for term := range current.Terms() {
			if sourceConstructionContainsNamedType(term.Type(), packagePath, typeName, seen) {
				return true
			}
		}
	}
	return false
}

func sourceConstructionSafeInterface(named *types.Named, packagePath string) bool {
	object := named.Obj()
	if object.Pkg() == nil {
		return object.Name() == "error"
	}
	switch object.Pkg().Path() {
	case "context":
		return object.Name() == "Context"
	case packagePath:
		return object.Name() == "sourcePrimitives" || object.Name() == "sourceHandleCloser"
	default:
		return false
	}
}

func sourceConstructionEscapeResult(results *ast.FieldList) string {
	if results == nil {
		return ""
	}
	for _, field := range results.List {
		if escaped := sourceConstructionEscapeType(field.Type); escaped != "" {
			return escaped
		}
	}
	return ""
}

func sourceConstructionEscapeType(expression ast.Expr) string {
	switch current := expression.(type) {
	case *ast.Ident:
		switch current.Name {
		case "ownedSourceRoot", "ownedSourceDescriptor",
			"retainedSourceRoot", "retainedSourceRepository", "retainedSourceGit",
			"retainedSourceObjects", "retainedSourceConfig", "retainedSourcePackedRefs",
			"sourcePackedRefsSlot", "sourceConstructionBuilder", "sourceConstructionOwner":
			return current.Name
		case "any":
			return current.Name
		}
	case *ast.SelectorExpr:
		packageName, ok := current.X.(*ast.Ident)
		if ok && packageName.Name == "os" &&
			(current.Sel.Name == "File" || current.Sel.Name == "Root") {
			return "os." + current.Sel.Name
		}
	case *ast.StarExpr:
		return sourceConstructionEscapeType(current.X)
	case *ast.ArrayType:
		return sourceConstructionEscapeType(current.Elt)
	case *ast.MapType:
		if escaped := sourceConstructionEscapeType(current.Key); escaped != "" {
			return escaped
		}
		return sourceConstructionEscapeType(current.Value)
	case *ast.ChanType:
		return sourceConstructionEscapeType(current.Value)
	case *ast.Ellipsis:
		return sourceConstructionEscapeType(current.Elt)
	case *ast.IndexExpr:
		if escaped := sourceConstructionEscapeType(current.X); escaped != "" {
			return escaped
		}
		return sourceConstructionEscapeType(current.Index)
	case *ast.IndexListExpr:
		if escaped := sourceConstructionEscapeType(current.X); escaped != "" {
			return escaped
		}
		for _, index := range current.Indices {
			if escaped := sourceConstructionEscapeType(index); escaped != "" {
				return escaped
			}
		}
	case *ast.FuncType:
		return "func"
	case *ast.InterfaceType:
		return "interface"
	case *ast.StructType:
		for _, field := range current.Fields.List {
			if escaped := sourceConstructionEscapeType(field.Type); escaped != "" {
				return escaped
			}
		}
	}
	return ""
}

func sourceConstructionTransferName(name string) bool {
	lower := strings.ToLower(name)
	for _, forbidden := range []string{
		"take", "move", "seal", "pending", "envelope",
		"borrow", "release", "adopt", "extract", "expose", "unwrap",
	} {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	return false
}

func assertSourceConstructionOwnerShape(t *testing.T, owner *sourceConstructionOwner) {
	t.Helper()
	ownerType := reflect.TypeFor[sourceConstructionOwner]()
	want := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "mu", typeOf: reflect.TypeFor[sync.Mutex]()},
		{name: "state", typeOf: reflect.TypeFor[sourceConstructionState]()},
		{name: "closeFailure", typeOf: reflect.TypeFor[bool]()},
		{name: "packedRefs", typeOf: reflect.TypeFor[sourcePackedRefsSlot]()},
		{name: "objects", typeOf: reflect.TypeFor[*retainedSourceObjects]()},
		{name: "config", typeOf: reflect.TypeFor[*retainedSourceConfig]()},
		{name: "git", typeOf: reflect.TypeFor[*retainedSourceGit]()},
		{name: "repository", typeOf: reflect.TypeFor[*retainedSourceRepository]()},
	}
	if ownerType.Kind() != reflect.Struct || ownerType.NumField() != len(want) {
		t.Fatalf("source construction owner = %s with %d fields, want %d", ownerType, ownerType.NumField(), len(want))
	}
	for index, expected := range want {
		field := ownerType.Field(index)
		if field.Name != expected.name || field.Type != expected.typeOf ||
			field.Anonymous || field.PkgPath == "" {
			t.Fatalf(
				"source construction field %d = %s %s anonymous=%v exported=%v, want %s %s unexported",
				index,
				field.Name,
				field.Type,
				field.Anonymous,
				field.PkgPath == "",
				expected.name,
				expected.typeOf,
			)
		}
	}
}

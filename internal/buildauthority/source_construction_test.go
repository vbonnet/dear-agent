package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"go/ast"
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
	if matched != 2 {
		t.Fatalf("source construction governance matched %d production files, want 2", matched)
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

func TestSourceConstructionTypeResolvedEscapeGuardRejectsBypasses(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve buildauthority directory: %v", err)
	}
	constructionPath := filepath.Join(workingDirectory, "source_construction.go")
	acquirePath := filepath.Join(workingDirectory, "source_construction_acquire.go")
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
	if count := strings.Count(string(acquireSource), parseRawACLCall); count != 2 {
		t.Fatalf("source construction parseRawACL call count = %d, want 2", count)
	}
	withVariadicParseCall := strings.ReplaceAll(
		string(acquireSource),
		parseRawACLCall,
		"builder.primitives.parseRawACL(builder.ctx, rawACL...)",
	)
	acquireSource = []byte(withVariadicParseCall)
	typedPackage := loadTypedSourceConstructionPackage(t, map[string][]byte{
		constructionPath:     constructionSource,
		acquirePath:          acquireSource,
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
	loaded, err := packages.Load(&packages.Config{
		Mode:    packages.LoadSyntax,
		Dir:     ".",
		Tests:   false,
		Overlay: overlay,
	}, ".")
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
		"validateSourceObservationRequest": {
			1: {"*ownedSourceDescriptor": true},
		},
		"validateInitialSourceOwner": {
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
		"openRepositoryRoot|(*sourceConstructionBuilder).retainRepository|:=|root,failure":                               1,
		"openPhysicalRootDescriptor|(*sourceConstructionBuilder).retainRepositoryDescriptor|:=|physicalRoot,openFailure": 1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).retainRepositoryDescriptor|:=|next,nextFailure":               1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).retainConfig|:=|descriptor,descriptorFailure":                 1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).rebindConfig|:=|comparison,openFailure":                       1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).retainPresentPackedRefs|:=|descriptor,descriptorFailure":      1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).rebindPackedRefs|:=|comparison,openFailure":                   1,
		"openChildRoot|(*sourceConstructionBuilder).retainSourceDirectory|:=|root,rootFailure":                           1,
		"openRelativeNoFollow|(*sourceConstructionBuilder).retainSourceDirectory|:=|descriptor,descriptorFailure":        1,
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
	constructionFiles := make([]*ast.File, 0, 2)
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
	if len(constructionFiles) != 2 {
		violations = append(violations, "typed source-construction module did not contain exactly two files")
	}
	return violations
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

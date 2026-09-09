package buildauthority

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"reflect"
	"testing"
	"time"
)

type authorityReaderAtFunc func([]byte, int64) (int, error)

func (read authorityReaderAtFunc) ReadAt(content []byte, offset int64) (int, error) {
	return read(content, offset)
}

type invalidAuthorityContext struct{}

func (invalidAuthorityContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (invalidAuthorityContext) Done() <-chan struct{}       { return nil }
func (invalidAuthorityContext) Err() error                  { return errors.New("invalid context state") }
func (invalidAuthorityContext) Value(any) any               { return nil }

type postCheckAuthorityContext struct {
	calls int
	after error
}

func (*postCheckAuthorityContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*postCheckAuthorityContext) Done() <-chan struct{}       { return nil }
func (ctx *postCheckAuthorityContext) Err() error {
	ctx.calls++
	if ctx.calls > 1 {
		return ctx.after
	}
	return nil
}
func (*postCheckAuthorityContext) Value(any) any { return nil }

type liveGoEnvironmentFixture struct {
	physicalRoot *physicalRootAuthority
	goroot       *treeCapture
	content      []byte

	rootDescriptor *os.File
	observations   map[*os.File]authorityDescriptorObservation
	labels         map[*os.File]string
	transient      []*os.File
	leafOpened     []*os.File
	closed         map[*os.File]int
	calls          map[string]int
	events         []string
	failures       map[string]*authorityPrimitiveFailure
	acquisitions   map[string]*authorityDescriptorAcquisition
	missing        map[string]bool
	closeFailures  map[string]bool
	closeHooks     map[string]func()
	readHooks      map[string]func()
	parentTemplate authorityDescriptorObservation
	leafTemplate   authorityDescriptorObservation
}

func newLiveGoEnvironmentFixture(t *testing.T) *liveGoEnvironmentFixture {
	t.Helper()
	content := []byte(
		"GOPROXY=https://proxy.golang.org,direct\n" +
			"GOSUMDB=sum.golang.org\n" +
			"GOTOOLCHAIN=auto\n",
	)
	mount := mountSnapshot{filesystem: [2]int32{17, 19}, flags: 1}
	rootSnapshot := fileSnapshot{identity: FileIdentity{
		Device:     1,
		Inode:      1,
		UID:        0,
		Mode:       platformModeDirectory | 0o755,
		Filesystem: mount.filesystem,
	}}
	rootACL := Digest{0x11}
	rootClaim := makeAuthorityPathClaim(rootSnapshot, mount, rootACL)
	rootDescriptor := new(os.File)
	physicalRoot := &physicalRootAuthority{
		directory: &retainedDirectory{
			path:       physicalRootPath,
			root:       new(os.Root),
			descriptor: rootDescriptor,
			snapshot:   rootSnapshot,
			mount:      mount,
			aclDigest:  rootACL,
			pathClaims: []authorityPathClaim{rootClaim},
		},
		claim: rootClaim,
	}
	parentSnapshot := fileSnapshot{identity: FileIdentity{
		Device:     1,
		Inode:      2,
		UID:        uint32(os.Geteuid()),
		Mode:       platformModeDirectory | 0o755,
		Filesystem: mount.filesystem,
	}}
	parentACL := Digest{0x22}
	parentClaim := makeAuthorityPathClaim(parentSnapshot, mount, parentACL)
	gorootSnapshot := fileSnapshot{identity: FileIdentity{
		Device:     1,
		Inode:      3,
		UID:        uint32(os.Geteuid()),
		Mode:       platformModeDirectory | 0o755,
		Filesystem: mount.filesystem,
	}}
	gorootACL := Digest{0x23}
	gorootClaim := makeAuthorityPathClaim(gorootSnapshot, mount, gorootACL)
	leafSnapshot := fileSnapshot{
		identity: FileIdentity{
			Device:     1,
			Inode:      4,
			UID:        uint32(os.Geteuid()),
			Mode:       platformModeRegular | 0o644,
			Filesystem: mount.filesystem,
		},
		size: int64(len(content)),
	}
	leafACL := Digest{0x33}
	entry := entrySnapshot{
		path:      goEnvironmentFileName,
		kind:      entryRegular,
		file:      leafSnapshot,
		aclDigest: leafACL,
		content:   Digest(sha256.Sum256(content)),
	}
	goroot := &treeCapture{
		root: &retainedDirectory{
			path:       "/private/toolchain",
			root:       new(os.Root),
			descriptor: new(os.File),
			snapshot:   gorootSnapshot,
			mount:      mount,
			aclDigest:  gorootACL,
			pathClaims: []authorityPathClaim{rootClaim, parentClaim, gorootClaim},
		},
		policy:  gorootPolicy(),
		digest:  Digest{0x44},
		entries: []entrySnapshot{entry},
	}
	fixture := &liveGoEnvironmentFixture{
		physicalRoot:   physicalRoot,
		goroot:         goroot,
		content:        content,
		rootDescriptor: rootDescriptor,
		observations:   make(map[*os.File]authorityDescriptorObservation),
		labels:         make(map[*os.File]string),
		parentTemplate: authorityDescriptorObservation{
			snapshot: parentSnapshot, mount: mount, aclDigest: parentACL,
		},
		leafTemplate: authorityDescriptorObservation{
			snapshot:  leafSnapshot,
			mount:     mount,
			aclDigest: leafACL,
		},
	}
	fixture.observations[rootDescriptor] = authorityDescriptorObservation{
		snapshot: rootSnapshot, mount: mount, aclDigest: rootACL,
	}
	fixture.labels[rootDescriptor] = "root"
	fixture.resetTrace()
	return fixture
}

func (fixture *liveGoEnvironmentFixture) resetTrace() {
	fixture.transient = nil
	fixture.closed = make(map[*os.File]int)
	fixture.calls = make(map[string]int)
	fixture.events = nil
	fixture.failures = make(map[string]*authorityPrimitiveFailure)
	fixture.acquisitions = make(map[string]*authorityDescriptorAcquisition)
	fixture.missing = make(map[string]bool)
	fixture.closeFailures = make(map[string]bool)
	fixture.closeHooks = make(map[string]func())
	fixture.readHooks = make(map[string]func())
}

func (fixture *liveGoEnvironmentFixture) step(base string) string {
	fixture.calls[base]++
	key := fmt.Sprintf("%s#%d", base, fixture.calls[base])
	fixture.events = append(fixture.events, key)
	return key
}

func authorityOpenModeName(mode authorityRequiredOpenMode) string {
	switch mode {
	case authorityInitialRequiredOpen:
		return "initial"
	case authorityExpectedPresentOpen:
		return "expected"
	default:
		return "invalid"
	}
}

func (fixture *liveGoEnvironmentFixture) primitives() preflightAuthorityPrimitives {
	return preflightAuthorityPrimitives{
		openRelativeNoFollow: func(
			_ context.Context,
			parent *os.File,
			name string,
			mode authorityRequiredOpenMode,
		) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
			parentLabel := fixture.labels[parent]
			key := fixture.step(fmt.Sprintf(
				"open:%s/%s:%s",
				parentLabel,
				name,
				authorityOpenModeName(mode),
			))
			if failure := fixture.failures[key]; failure != nil {
				return fixture.acquisitions[key], failure
			}
			if fixture.missing[key] {
				return nil, requiredAuthorityOpenNotFound(mode)
			}
			var label string
			var observation authorityDescriptorObservation
			switch {
			case parentLabel == "root" && name == "private":
				label = "parent"
				observation = fixture.parentTemplate
			case parentLabel == "parent" && name == "toolchain":
				label = "goroot"
				observation = authorityDescriptorObservation{
					snapshot:  fixture.goroot.root.snapshot,
					mount:     fixture.goroot.root.mount,
					aclDigest: fixture.goroot.root.aclDigest,
				}
			case parentLabel == "goroot" && name == goEnvironmentFileName:
				label = "leaf"
				observation = fixture.leafTemplate
			default:
				return nil, newAuthorityPrimitiveFailure(
					OperationValidate,
					CauseInternalInvariant,
				)
			}
			descriptor := new(os.File)
			fixture.labels[descriptor] = label
			fixture.observations[descriptor] = observation
			fixture.transient = append(fixture.transient, descriptor)
			if label == "leaf" {
				fixture.leafOpened = append(fixture.leafOpened, descriptor)
			}
			return &authorityDescriptorAcquisition{
				file: descriptor,
				close: func() error {
					closeKey := fixture.step("close:" + label)
					fixture.closed[descriptor]++
					if hook := fixture.closeHooks[closeKey]; hook != nil {
						hook()
					}
					if fixture.closeFailures[closeKey] {
						return errors.New("injected close failure")
					}
					return nil
				},
			}, nil
		},
		statDescriptor: func(
			ctx context.Context,
			descriptor *os.File,
		) (fileSnapshot, *authorityPrimitiveFailure) {
			label := fixture.labels[descriptor]
			key := fixture.step("stat:" + label)
			if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
				return fileSnapshot{}, failure
			}
			if failure := fixture.failures[key]; failure != nil {
				return fileSnapshot{}, failure
			}
			observation, ok := fixture.observations[descriptor]
			if !ok {
				return fileSnapshot{}, newAuthorityPrimitiveFailure(
					OperationValidate,
					CauseInternalInvariant,
				)
			}
			snapshot := observation.snapshot
			snapshot.identity.Filesystem = [2]int32{}
			return snapshot, nil
		},
		statFilesystem: func(
			_ context.Context,
			descriptor *os.File,
		) (mountSnapshot, *authorityPrimitiveFailure) {
			label := fixture.labels[descriptor]
			key := fixture.step("mount:" + label)
			if failure := fixture.failures[key]; failure != nil {
				return mountSnapshot{}, failure
			}
			observation, ok := fixture.observations[descriptor]
			if !ok {
				return mountSnapshot{}, newAuthorityPrimitiveFailure(
					OperationValidate,
					CauseInternalInvariant,
				)
			}
			return observation.mount, nil
		},
		validateFilesystem: func(
			_ context.Context,
			_ mountSnapshot,
		) *authorityPrimitiveFailure {
			key := fixture.step("validate-mount")
			return fixture.failures[key]
		},
		acquireRawACL: func(
			_ context.Context,
			descriptor *os.File,
		) ([]byte, *authorityPrimitiveFailure) {
			label := fixture.labels[descriptor]
			key := fixture.step("acl:" + label)
			if failure := fixture.failures[key]; failure != nil {
				return nil, failure
			}
			observation, ok := fixture.observations[descriptor]
			if !ok {
				return nil, newAuthorityPrimitiveFailure(
					OperationValidate,
					CauseInternalInvariant,
				)
			}
			return []byte{observation.aclDigest[0]}, nil
		},
		parseRawACL: func(
			_ context.Context,
			raw []byte,
		) (Digest, *authorityPrimitiveFailure) {
			key := fixture.step("parse-acl")
			if failure := fixture.failures[key]; failure != nil {
				return Digest{}, failure
			}
			if len(raw) != 1 {
				return Digest{}, newAuthorityPrimitiveFailure(
					OperationParse,
					CauseMalformed,
				)
			}
			return Digest{raw[0]}, nil
		},
		readExactForParse: func(
			_ context.Context,
			reader io.ReaderAt,
			content []byte,
		) *authorityPrimitiveFailure {
			label := "unknown"
			if descriptor, ok := reader.(*os.File); ok {
				label = fixture.labels[descriptor]
			}
			key := fixture.step("read:" + label)
			if failure := fixture.failures[key]; failure != nil {
				return failure
			}
			if len(content) != len(fixture.content) {
				return newAuthorityPrimitiveFailure(OperationParse, CauseUnstable)
			}
			copy(content, fixture.content)
			if hook := fixture.readHooks[key]; hook != nil {
				hook()
			}
			return nil
		},
		hashBytes: func(
			_ context.Context,
			content []byte,
		) (Digest, *authorityPrimitiveFailure) {
			key := fixture.step("hash")
			if failure := fixture.failures[key]; failure != nil {
				return Digest{}, failure
			}
			return Digest(sha256.Sum256(content)), nil
		},
	}
}

func (fixture *liveGoEnvironmentFixture) revalidator(
	t *testing.T,
) *preflightAuthorityRevalidator {
	t.Helper()
	revalidator, failure := newPreflightAuthorityRevalidatorWith(fixture.primitives())
	if failure != nil {
		t.Fatalf("construct fake authority revalidator: %+v", failure)
	}
	return revalidator
}

func (fixture *liveGoEnvironmentFixture) requireTransientClosure(t *testing.T) {
	t.Helper()
	for _, descriptor := range fixture.transient {
		if fixture.closed[descriptor] != 1 {
			t.Fatalf(
				"transient %s closed %d times, want once",
				fixture.labels[descriptor],
				fixture.closed[descriptor],
			)
		}
	}
	if fixture.closed[fixture.rootDescriptor] != 0 {
		t.Fatalf("borrowed physical root closed %d times", fixture.closed[fixture.rootDescriptor])
	}
}

func requireAuthorityPrimitiveRecord(
	t *testing.T,
	record *FailureRecord,
	operation Operation,
	cause CauseCode,
) {
	t.Helper()
	requireAuthorityRecord(t, record, operation, cause)
}

func TestLiveGoEnvironmentAdmissionAndRepeatedRevalidationUseFreshLeaves(t *testing.T) {
	fixture := newLiveGoEnvironmentFixture(t)
	revalidator := fixture.revalidator(t)
	claim, outcome := revalidator.admitLiveGoEnvironment(
		context.Background(),
		fixture.physicalRoot,
		fixture.goroot,
	)
	if claim == nil || !claim.validFor(fixture.goroot) || !outcome.proved() {
		t.Fatalf("admission = claim %v outcome %+v", claim != nil, outcome)
	}
	wantAdmissionEvents := []string{
		"stat:root#1", "mount:root#1", "validate-mount#1", "acl:root#1", "parse-acl#1",
		"open:root/private:expected#1",
		"stat:parent#1", "mount:parent#1", "validate-mount#2", "acl:parent#1", "parse-acl#2",
		"open:parent/toolchain:expected#1", "close:parent#1",
		"stat:goroot#1", "mount:goroot#1", "validate-mount#3", "acl:goroot#1", "parse-acl#3",
		"open:goroot/go.env:initial#1", "close:goroot#1",
		"stat:leaf#1", "mount:leaf#1", "validate-mount#4", "acl:leaf#1", "parse-acl#4",
		"read:leaf#1", "hash#1",
		"stat:leaf#2", "mount:leaf#2", "validate-mount#5", "acl:leaf#2", "parse-acl#5",
		"close:leaf#1",
		"stat:root#2", "mount:root#2", "validate-mount#6", "acl:root#2", "parse-acl#6",
		"open:root/private:expected#2",
		"stat:parent#2", "mount:parent#2", "validate-mount#7", "acl:parent#2", "parse-acl#7",
		"open:parent/toolchain:expected#2", "close:parent#2",
		"stat:goroot#2", "mount:goroot#2", "validate-mount#8", "acl:goroot#2", "parse-acl#8",
		"close:goroot#2",
	}
	if !reflect.DeepEqual(fixture.events, wantAdmissionEvents) {
		t.Fatalf("admission events\n got: %q\nwant: %q", fixture.events, wantAdmissionEvents)
	}
	for range 2 {
		outcome = revalidator.revalidateLiveGoEnvironment(
			context.Background(),
			fixture.physicalRoot,
			fixture.goroot,
			claim,
		)
		if !outcome.proved() {
			t.Fatalf("revalidation outcome = %+v", outcome)
		}
	}
	if len(fixture.leafOpened) != 3 {
		t.Fatalf("fresh leaf opens = %d, want 3", len(fixture.leafOpened))
	}
	seen := make(map[*os.File]bool, len(fixture.leafOpened))
	for _, descriptor := range fixture.leafOpened {
		if seen[descriptor] {
			t.Fatal("live go.env revalidation reused a leaf descriptor")
		}
		seen[descriptor] = true
	}
	fixture.requireTransientClosure(t)
}

func TestLiveGoEnvironmentContextTerminationBelongsToFirstAuthorityPrimitive(t *testing.T) {
	for _, termination := range []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		cause   CauseCode
	}{
		{
			name: "canceled",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
			cause: CauseCanceled,
		},
		{
			name: "deadline",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Unix(1, 0))
			},
			cause: CauseDeadline,
		},
	} {
		for _, lifecycle := range []string{"admission", "revalidation"} {
			t.Run(termination.name+" "+lifecycle, func(t *testing.T) {
				fixture := newLiveGoEnvironmentFixture(t)
				revalidator := fixture.revalidator(t)
				var claim *goEnvironmentFileClaim
				if lifecycle == "revalidation" {
					var admitted authorityUseOutcome
					claim, admitted = revalidator.admitLiveGoEnvironment(
						context.Background(),
						fixture.physicalRoot,
						fixture.goroot,
					)
					if claim == nil || !admitted.proved() {
						t.Fatalf("fixture admission failed: %+v", admitted)
					}
					fixture.resetTrace()
				}
				ctx, cancel := termination.context()
				defer cancel()
				var outcome authorityUseOutcome
				if lifecycle == "admission" {
					claim, outcome = revalidator.admitLiveGoEnvironment(
						ctx,
						fixture.physicalRoot,
						fixture.goroot,
					)
					if claim != nil {
						t.Fatal("terminated admission minted a claim")
					}
				} else {
					outcome = revalidator.revalidateLiveGoEnvironment(
						ctx,
						fixture.physicalRoot,
						fixture.goroot,
						claim,
					)
				}
				requireAuthorityPrimitiveRecord(
					t,
					outcome.primary,
					OperationProbe,
					termination.cause,
				)
				requireAuthorityPrimitiveRecord(
					t,
					outcome.later,
					OperationProbe,
					termination.cause,
				)
				if !reflect.DeepEqual(fixture.events, []string{"stat:root#1", "stat:root#2"}) {
					t.Fatalf("terminated events = %q", fixture.events)
				}
			})
		}
	}

	fixture := newLiveGoEnvironmentFixture(t)
	claim, outcome := fixture.revalidator(t).admitLiveGoEnvironment(
		nil, //nolint:staticcheck // Deliberately exercise the nil-context internal-invariant gate.
		fixture.physicalRoot,
		fixture.goroot,
	)
	if claim != nil {
		t.Fatal("nil context minted a claim")
	}
	requireAuthorityPrimitiveRecord(
		t,
		outcome.primary,
		OperationValidate,
		CauseInternalInvariant,
	)
	if len(fixture.events) != 0 {
		t.Fatalf("nil context invoked primitives: %q", fixture.events)
	}

	directNil := authorityContextPrimitiveFailure(
		nil, //nolint:staticcheck // Deliberately exercise the private primitive's nil-context invariant.
		OperationProbe,
	)
	if directNil == nil || directNil.operation != OperationValidate ||
		directNil.cause != CauseInternalInvariant {
		t.Fatalf("direct nil-context failure = %+v", directNil)
	}
	directInvalid := authorityContextPrimitiveFailure(invalidAuthorityContext{}, OperationHash)
	if directInvalid == nil || directInvalid.operation != OperationValidate ||
		directInvalid.cause != CauseInternalInvariant {
		t.Fatalf("direct invalid-context failure = %+v", directInvalid)
	}
}

func TestLiveGoEnvironmentMissingRowAttributionTracksClaimLifecycle(t *testing.T) {
	t.Run("retained GOROOT ancestry", func(t *testing.T) {
		for _, lifecycle := range []string{"admission", "revalidation"} {
			t.Run(lifecycle, func(t *testing.T) {
				fixture := newLiveGoEnvironmentFixture(t)
				revalidator := fixture.revalidator(t)
				var claim *goEnvironmentFileClaim
				if lifecycle == "revalidation" {
					var admitted authorityUseOutcome
					claim, admitted = revalidator.admitLiveGoEnvironment(
						context.Background(),
						fixture.physicalRoot,
						fixture.goroot,
					)
					if claim == nil || !admitted.proved() {
						t.Fatalf("fixture admission failed: %+v", admitted)
					}
					fixture.resetTrace()
				}
				fixture.missing["open:root/private:expected#1"] = true
				var outcome authorityUseOutcome
				if lifecycle == "admission" {
					candidate, admittedOutcome := revalidator.admitLiveGoEnvironment(
						context.Background(),
						fixture.physicalRoot,
						fixture.goroot,
					)
					if candidate != nil {
						t.Fatal("failed admission minted a claim")
					}
					outcome = admittedOutcome
				} else {
					outcome = revalidator.revalidateLiveGoEnvironment(
						context.Background(),
						fixture.physicalRoot,
						fixture.goroot,
						claim,
					)
				}
				requireAuthorityPrimitiveRecord(
					t,
					outcome.primary,
					OperationCompare,
					CauseUnstable,
				)
				if fixture.calls["open:root/private:expected"] != 2 {
					t.Fatalf(
						"GOROOT resolutions = %d, want leading plus mandatory trailing",
						fixture.calls["open:root/private:expected"],
					)
				}
				fixture.requireTransientClosure(t)
			})
		}
	})

	t.Run("go.env leaf", func(t *testing.T) {
		for _, lifecycle := range []struct {
			name      string
			key       string
			operation Operation
			cause     CauseCode
		}{
			{name: "admission", key: "open:goroot/go.env:initial#1", operation: OperationOpen, cause: CauseNotFound},
			{name: "revalidation", key: "open:goroot/go.env:expected#1", operation: OperationCompare, cause: CauseUnstable},
		} {
			t.Run(lifecycle.name, func(t *testing.T) {
				fixture := newLiveGoEnvironmentFixture(t)
				revalidator := fixture.revalidator(t)
				var claim *goEnvironmentFileClaim
				if lifecycle.name == "revalidation" {
					var admitted authorityUseOutcome
					claim, admitted = revalidator.admitLiveGoEnvironment(
						context.Background(),
						fixture.physicalRoot,
						fixture.goroot,
					)
					if claim == nil || !admitted.proved() {
						t.Fatalf("fixture admission failed: %+v", admitted)
					}
					fixture.resetTrace()
				}
				fixture.missing[lifecycle.key] = true
				var outcome authorityUseOutcome
				if lifecycle.name == "admission" {
					candidate, admittedOutcome := revalidator.admitLiveGoEnvironment(
						context.Background(),
						fixture.physicalRoot,
						fixture.goroot,
					)
					if candidate != nil {
						t.Fatal("failed admission minted a claim")
					}
					outcome = admittedOutcome
				} else {
					outcome = revalidator.revalidateLiveGoEnvironment(
						context.Background(),
						fixture.physicalRoot,
						fixture.goroot,
						claim,
					)
				}
				requireAuthorityPrimitiveRecord(
					t,
					outcome.primary,
					lifecycle.operation,
					lifecycle.cause,
				)
				if fixture.calls["open:root/private:expected"] != 2 {
					t.Fatalf(
						"GOROOT resolutions = %d, want leading plus mandatory trailing",
						fixture.calls["open:root/private:expected"],
					)
				}
				fixture.requireTransientClosure(t)
			})
		}
	})
}

func TestLiveGoEnvironmentPreservesPrimaryLaterAndDescriptorClose(t *testing.T) {
	fixture := newLiveGoEnvironmentFixture(t)
	fixture.failures["read:leaf#1"] = newAuthorityPrimitiveFailure(
		OperationParse,
		CauseUnstable,
	)
	fixture.closeFailures["close:leaf#1"] = true
	fixture.failures["stat:root#2"] = newAuthorityPrimitiveFailure(
		OperationProbe,
		CauseUnstable,
	)
	claim, outcome := fixture.revalidator(t).admitLiveGoEnvironment(
		context.Background(),
		fixture.physicalRoot,
		fixture.goroot,
	)
	if claim != nil {
		t.Fatal("failed live transaction minted a claim")
	}
	requireAuthorityPrimitiveRecord(t, outcome.primary, OperationParse, CauseUnstable)
	requireAuthorityPrimitiveRecord(t, outcome.later, OperationProbe, CauseUnstable)
	if outcome.descriptorClose == nil ||
		outcome.descriptorClose.Phase != PhaseClose ||
		outcome.descriptorClose.Operation != OperationCloseNonRoot ||
		!reflect.DeepEqual(
			outcome.descriptorClose.Causes,
			[]CauseCode{CauseDescriptorClose},
		) {
		t.Fatalf("descriptor close = %+v", outcome.descriptorClose)
	}
	if fixture.calls["hash"] != 0 || fixture.calls["stat:leaf"] != 1 {
		t.Fatalf(
			"ordinary leaf chain continued: hash=%d stat-leaf=%d",
			fixture.calls["hash"],
			fixture.calls["stat:leaf"],
		)
	}
	if fixture.calls["stat:root"] != 2 {
		t.Fatalf("mandatory trailing path did not run: root stats=%d", fixture.calls["stat:root"])
	}
	fixture.requireTransientClosure(t)
}

func TestLiveGoEnvironmentDescriptorCloseNeverCompetesForPrimary(t *testing.T) {
	for _, test := range []struct {
		name            string
		trailingFailure bool
	}{
		{name: "close only"},
		{name: "close then trailing failure", trailingFailure: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveGoEnvironmentFixture(t)
			fixture.closeFailures["close:leaf#1"] = true
			if test.trailingFailure {
				fixture.failures["stat:root#2"] = newAuthorityPrimitiveFailure(
					OperationProbe,
					CauseUnstable,
				)
			}
			claim, outcome := fixture.revalidator(t).admitLiveGoEnvironment(
				context.Background(),
				fixture.physicalRoot,
				fixture.goroot,
			)
			if claim != nil || outcome.descriptorClose == nil {
				t.Fatalf("close failure = claim %v outcome %+v", claim != nil, outcome)
			}
			if test.trailingFailure {
				requireAuthorityPrimitiveRecord(
					t,
					outcome.primary,
					OperationProbe,
					CauseUnstable,
				)
			} else if outcome.primary != nil || outcome.later != nil {
				t.Fatalf("descriptor close occupied non-close slot: %+v", outcome)
			}
			fixture.requireTransientClosure(t)
		})
	}
}

func TestLiveGoEnvironmentRetainedInputsStaySealedThroughTrailingClose(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*liveGoEnvironmentFixture)
	}{
		{
			name: "GOROOT path",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.goroot.root.path = "/replaced-toolchain"
			},
		},
		{
			name: "GOROOT digest",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.goroot.digest[0] ^= 0xff
			},
		},
		{
			name: "GOROOT path claims",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.goroot.root.pathClaims[1].aclDigest[0] ^= 0xff
			},
		},
		{
			name: "GOROOT root snapshot",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.goroot.root.snapshot.identity.Inode++
			},
		},
		{
			name: "GOROOT root ACL",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.goroot.root.aclDigest[0] ^= 0xff
			},
		},
		{
			name: "GOROOT retained directory",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				root := *fixture.goroot.root
				fixture.goroot.root = &root
			},
		},
		{
			name: "GOROOT root handle",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.goroot.root.root = new(os.Root)
			},
		},
		{
			name: "GOROOT descriptor handle",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.goroot.root.descriptor = new(os.File)
			},
		},
		{
			name: "physical-root retained directory",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				directory := *fixture.physicalRoot.directory
				fixture.physicalRoot.directory = &directory
			},
		},
		{
			name: "physical-root root handle",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.physicalRoot.directory.root = new(os.Root)
			},
		},
		{
			name: "physical-root descriptor handle",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.physicalRoot.directory.descriptor = new(os.File)
			},
		},
		{
			name: "physical-root claim",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.physicalRoot.claim.identity.Inode++
				fixture.physicalRoot.directory.pathClaims[0] = fixture.physicalRoot.claim
				fixture.physicalRoot.directory.snapshot.identity.Inode++
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveGoEnvironmentFixture(t)
			fixture.closeHooks["close:goroot#2"] = func() { test.mutate(fixture) }
			claim, outcome := fixture.revalidator(t).admitLiveGoEnvironment(
				context.Background(),
				fixture.physicalRoot,
				fixture.goroot,
			)
			if claim != nil {
				t.Fatal("trailing-close mutation minted a live claim")
			}
			requireAuthorityPrimitiveRecord(
				t,
				outcome.primary,
				OperationCompare,
				CauseUnstable,
			)
			fixture.requireTransientClosure(t)
		})
	}
}

func TestLiveGoEnvironmentPrimitiveFailuresKeepTheirAttribution(t *testing.T) {
	for _, test := range []struct {
		name      string
		key       string
		operation Operation
		cause     CauseCode
	}{
		{name: "descriptor stat", key: "stat:root#1", operation: OperationProbe, cause: CauseUnstable},
		{name: "filesystem stat", key: "mount:root#1", operation: OperationProbe, cause: CauseUnsupported},
		{name: "filesystem policy", key: "validate-mount#1", operation: OperationValidate, cause: CauseUnsupported},
		{name: "raw ACL", key: "acl:root#1", operation: OperationProbe, cause: CausePermission},
		{name: "ACL parse", key: "parse-acl#1", operation: OperationParse, cause: CauseMalformed},
		{name: "leaf read", key: "read:leaf#1", operation: OperationParse, cause: CauseUnstable},
		{name: "content hash", key: "hash#1", operation: OperationHash, cause: CauseUnstable},
		{name: "post observation", key: "stat:leaf#2", operation: OperationProbe, cause: CauseUnstable},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveGoEnvironmentFixture(t)
			fixture.failures[test.key] = newAuthorityPrimitiveFailure(test.operation, test.cause)
			claim, outcome := fixture.revalidator(t).admitLiveGoEnvironment(
				context.Background(),
				fixture.physicalRoot,
				fixture.goroot,
			)
			if claim != nil {
				t.Fatal("primitive failure minted a live claim")
			}
			requireAuthorityPrimitiveRecord(t, outcome.primary, test.operation, test.cause)
			fixture.requireTransientClosure(t)
		})
	}
}

func TestLiveGoEnvironmentGrammarAndIdentityFailuresAreDistinct(t *testing.T) {
	for _, test := range []struct {
		name      string
		content   string
		operation Operation
		cause     CauseCode
	}{
		{name: "malformed grammar", content: "GOPROXY\n", operation: OperationParse, cause: CauseMalformed},
		{name: "wrong value", content: "GOPROXY=off\nGOSUMDB=sum.golang.org\nGOTOOLCHAIN=auto\n", operation: OperationCompare, cause: CauseUnsupported},
		{name: "unknown setting", content: "GOPROXY=https://proxy.golang.org,direct\nGOSUMDB=sum.golang.org\nGOTOOLCHAIN=auto\nGOFLAGS=-mod=mod\n", operation: OperationCompare, cause: CauseUnsupported},
		{name: "GOCACHEPROG setting", content: "GOPROXY=https://proxy.golang.org,direct\nGOSUMDB=sum.golang.org\nGOTOOLCHAIN=auto\nGOCACHEPROG=helper\n", operation: OperationCompare, cause: CauseUnsupported},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveGoEnvironmentFixture(t)
			fixture.content = []byte(test.content)
			entry := &fixture.goroot.entries[0]
			entry.file.size = int64(len(fixture.content))
			entry.content = Digest(sha256.Sum256(fixture.content))
			fixture.leafTemplate.snapshot.size = entry.file.size
			claim, outcome := fixture.revalidator(t).admitLiveGoEnvironment(
				context.Background(),
				fixture.physicalRoot,
				fixture.goroot,
			)
			if claim != nil {
				t.Fatal("invalid grammar minted a live claim")
			}
			requireAuthorityPrimitiveRecord(t, outcome.primary, test.operation, test.cause)
			fixture.requireTransientClosure(t)
		})
	}

	t.Run("leaf identity", func(t *testing.T) {
		fixture := newLiveGoEnvironmentFixture(t)
		fixture.leafTemplate.snapshot.identity.Inode++
		claim, outcome := fixture.revalidator(t).admitLiveGoEnvironment(
			context.Background(),
			fixture.physicalRoot,
			fixture.goroot,
		)
		if claim != nil {
			t.Fatal("replacement leaf minted a live claim")
		}
		requireAuthorityPrimitiveRecord(t, outcome.primary, OperationCompare, CauseIdentity)
		fixture.requireTransientClosure(t)
	})
}

func TestLiveGoEnvironmentRefusesPathLeafAndReadWindowDrift(t *testing.T) {
	for _, test := range []struct {
		name      string
		mutate    func(*liveGoEnvironmentFixture)
		operation Operation
		cause     CauseCode
	}{
		{
			name: "path component identity",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.parentTemplate.snapshot.identity.Inode++
			},
			operation: OperationCompare,
			cause:     CauseIdentity,
		},
		{
			name: "path component ACL",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.parentTemplate.aclDigest[0] ^= 0xff
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "leaf ACL",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.leafTemplate.aclDigest[0] ^= 0xff
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "leaf content digest",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.content = append([]byte("# drift\n"), fixture.content...)
				fixture.goroot.entries[0].file.size = int64(len(fixture.content))
				fixture.leafTemplate.snapshot.size = int64(len(fixture.content))
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "pre post metadata",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.readHooks["read:leaf#1"] = func() {
					for descriptor, label := range fixture.labels {
						if label != "leaf" {
							continue
						}
						observation := fixture.observations[descriptor]
						observation.snapshot.mtimeNsec++
						fixture.observations[descriptor] = observation
					}
				}
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "leaf mount transition",
			mutate: func(fixture *liveGoEnvironmentFixture) {
				fixture.leafTemplate.mount.filesystem[0]++
			},
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLiveGoEnvironmentFixture(t)
			test.mutate(fixture)
			claim, outcome := fixture.revalidator(t).admitLiveGoEnvironment(
				context.Background(),
				fixture.physicalRoot,
				fixture.goroot,
			)
			if claim != nil {
				t.Fatal("drift minted a live claim")
			}
			requireAuthorityPrimitiveRecord(t, outcome.primary, test.operation, test.cause)
			fixture.requireTransientClosure(t)
		})
	}
}

func TestLiveGoEnvironmentGrammarChecksContextAfterSuccessfulParse(t *testing.T) {
	fixture := newLiveGoEnvironmentFixture(t)
	ctx := &postCheckAuthorityContext{after: context.Canceled}
	failure := parseLiveGoEnvironmentForAuthority(ctx, fixture.content)
	if ctx.calls != 2 || failure == nil || failure.operation != OperationParse ||
		failure.cause != CauseCanceled {
		t.Fatalf("post-parse cancellation = calls %d failure %+v", ctx.calls, failure)
	}
}

func TestLiveGoEnvironmentSizeBoundsPrecedeContentAllocation(t *testing.T) {
	fixture := newLiveGoEnvironmentFixture(t)
	inputs, failure := validateLiveGoEnvironmentInputs(
		context.Background(),
		fixture.physicalRoot,
		fixture.goroot,
	)
	if failure != nil {
		t.Fatalf("validate fixture inputs: %+v", failure)
	}
	for _, test := range []struct {
		name  string
		size  int64
		cause CauseCode
	}{
		{name: "negative", size: -1, cause: CauseLimit},
		{name: "exact", size: int64(maxGOROOTFileBytes)},
		{name: "one over", size: int64(maxGOROOTFileBytes) + 1, cause: CauseLimit},
	} {
		t.Run("live "+test.name, func(t *testing.T) {
			observation := fixture.leafTemplate
			observation.snapshot.size = test.size
			got := validateLiveGoEnvironmentObservation(
				context.Background(),
				inputs,
				observation,
			)
			if test.cause == "" {
				if got != nil {
					t.Fatalf("exact bound refused: %+v", got)
				}
				return
			}
			if got == nil || got.operation != OperationValidate || got.cause != test.cause {
				t.Fatalf("live size failure = %+v", got)
			}
		})
	}

	for _, test := range []struct {
		name      string
		size      int64
		wantError bool
	}{
		{name: "negative", size: -1, wantError: true},
		{name: "exact", size: int64(maxGOROOTFileBytes)},
		{name: "one over", size: int64(maxGOROOTFileBytes) + 1, wantError: true},
	} {
		t.Run("retained "+test.name, func(t *testing.T) {
			fixture := newLiveGoEnvironmentFixture(t)
			fixture.goroot.entries[0].file.size = test.size
			_, record := validateLiveGoEnvironmentInputs(
				context.Background(),
				fixture.physicalRoot,
				fixture.goroot,
			)
			if test.wantError {
				requireAuthorityPrimitiveRecord(
					t,
					record,
					OperationValidate,
					CauseInternalInvariant,
				)
				return
			}
			if record != nil {
				t.Fatalf("exact retained bound refused: %+v", record)
			}
		})
	}
}

func TestOwnedAuthorityDescriptorClosesMalformedAcquisitionsExactlyOnce(t *testing.T) {
	t.Run("missing acquisition fails closed", func(t *testing.T) {
		owner, failure := ownAuthorityAcquisition(nil, nil)
		if owner != nil || failure == nil || failure.operation != OperationValidate ||
			failure.cause != CauseInternalInvariant {
			t.Fatalf("missing acquisition = owner %v failure %+v", owner != nil, failure)
		}
	})

	t.Run("failed close remains one report", func(t *testing.T) {
		closed := 0
		owner, failure := ownAuthorityDescriptor(&authorityDescriptorAcquisition{
			file: new(os.File),
			close: func() error {
				closed++
				return errors.New("injected close failure")
			},
		})
		if owner == nil || failure != nil {
			t.Fatalf("own valid acquisition = %v / %+v", owner != nil, failure)
		}
		var outcome authorityUseOutcome
		owner.closeInto(&outcome)
		owner.closeInto(&outcome)
		if closed != 1 || outcome.descriptorClose == nil ||
			!reflect.DeepEqual(
				outcome.descriptorClose.Causes,
				[]CauseCode{CauseDescriptorClose},
			) {
			t.Fatalf("failed close = count %d outcome %+v", closed, outcome)
		}
	})

	t.Run("raw close survives missing file", func(t *testing.T) {
		closed := 0
		owner, failure := ownAuthorityDescriptor(&authorityDescriptorAcquisition{
			close: func() error {
				closed++
				return nil
			},
		})
		if owner == nil || failure == nil || failure.operation != OperationValidate ||
			failure.cause != CauseInternalInvariant {
			t.Fatalf("malformed acquisition = owner %v failure %+v", owner != nil, failure)
		}
		var outcome authorityUseOutcome
		owner.closeInto(&outcome)
		owner.closeInto(&outcome)
		if closed != 1 || outcome.descriptorClose != nil {
			t.Fatalf("raw close = %d outcome %+v", closed, outcome)
		}
	})

	t.Run("file supplies safe fallback", func(t *testing.T) {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatalf("create pipe: %v", err)
		}
		t.Cleanup(func() { _ = writer.Close() })
		owner, failure := ownAuthorityDescriptor(&authorityDescriptorAcquisition{file: reader})
		if owner == nil || failure == nil || failure.operation != OperationValidate ||
			failure.cause != CauseInternalInvariant {
			t.Fatalf("malformed acquisition = owner %v failure %+v", owner != nil, failure)
		}
		var outcome authorityUseOutcome
		owner.closeInto(&outcome)
		owner.closeInto(&outcome)
		if outcome.descriptorClose != nil {
			t.Fatalf("fallback close failed: %+v", outcome.descriptorClose)
		}
		if err := reader.Close(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("fallback did not close file: %v", err)
		}
	})
}

func TestAuthorityPrimitiveFailureConversionFailsClosed(t *testing.T) {
	if failure := authorityPrimitiveFailureFromError(
		OperationParse,
		nil,
		CauseMalformed,
	); failure != nil {
		t.Fatalf("nil error produced failure %+v", failure)
	}
	for _, test := range []struct {
		name      string
		err       error
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "ordinary attributed cause",
			err:       fail(CauseMalformed, "malformed fixture"),
			operation: OperationParse,
			cause:     CauseMalformed,
		},
		{
			name:      "single impossible cause",
			err:       fail(CauseInternalInvariant, "impossible fixture"),
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name: "ambiguous causes",
			err: errors.Join(
				fail(CauseMalformed, "first fixture"),
				fail(CauseUnsupported, "second fixture"),
			),
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := authorityPrimitiveFailureFromError(
				OperationParse,
				test.err,
				CauseMalformed,
			)
			if failure == nil || failure.operation != test.operation ||
				failure.cause != test.cause {
				t.Fatalf("converted failure = %+v", failure)
			}
		})
	}
}

func TestLiveGoEnvironmentOwnsRawDescriptorBeforeAdapterFailure(t *testing.T) {
	fixture := newLiveGoEnvironmentFixture(t)
	closed := 0
	const key = "open:goroot/go.env:initial#1"
	fixture.acquisitions[key] = &authorityDescriptorAcquisition{
		close: func() error {
			closed++
			return nil
		},
	}
	fixture.failures[key] = newAuthorityPrimitiveFailure(
		OperationValidate,
		CauseInternalInvariant,
	)
	claim, outcome := fixture.revalidator(t).admitLiveGoEnvironment(
		context.Background(),
		fixture.physicalRoot,
		fixture.goroot,
	)
	if claim != nil || closed != 1 {
		t.Fatalf("adapter edge = claim %v raw closes %d", claim != nil, closed)
	}
	requireAuthorityPrimitiveRecord(
		t,
		outcome.primary,
		OperationValidate,
		CauseInternalInvariant,
	)
	if outcome.later != nil || outcome.descriptorClose != nil {
		t.Fatalf("adapter edge manufactured extra failure: %+v", outcome)
	}
	fixture.requireTransientClosure(t)
}

func TestReadExactAuthorityForParseHasClosedEOFAttribution(t *testing.T) {
	for _, test := range []struct {
		name      string
		ctx       context.Context
		reader    io.ReaderAt
		length    int
		operation Operation
		cause     CauseCode
	}{
		{
			name:   "zero",
			ctx:    context.Background(),
			reader: authorityReaderAtFunc(func([]byte, int64) (int, error) { return 0, nil }),
		},
		{
			name:   "exact",
			ctx:    context.Background(),
			length: 4,
			reader: authorityReaderAtFunc(func(content []byte, _ int64) (int, error) {
				return len(content), nil
			}),
		},
		{
			name:   "exact with EOF",
			ctx:    context.Background(),
			length: 4,
			reader: authorityReaderAtFunc(func(content []byte, _ int64) (int, error) {
				return len(content), io.EOF
			}),
		},
		{
			name:      "short nil",
			ctx:       context.Background(),
			length:    4,
			reader:    authorityReaderAtFunc(func([]byte, int64) (int, error) { return 3, nil }),
			operation: OperationParse,
			cause:     CauseUnstable,
		},
		{
			name:      "premature EOF",
			ctx:       context.Background(),
			length:    4,
			reader:    authorityReaderAtFunc(func([]byte, int64) (int, error) { return 3, io.EOF }),
			operation: OperationParse,
			cause:     CauseUnstable,
		},
		{
			name:      "permission",
			ctx:       context.Background(),
			length:    4,
			reader:    authorityReaderAtFunc(func([]byte, int64) (int, error) { return 0, fs.ErrPermission }),
			operation: OperationParse,
			cause:     CausePermission,
		},
		{
			name:      "missing reader",
			ctx:       context.Background(),
			length:    4,
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := readExactAuthorityForParse(test.ctx, test.reader, make([]byte, test.length))
			if test.cause == "" {
				if failure != nil {
					t.Fatalf("exact read refused: %+v", failure)
				}
				return
			}
			if failure == nil || failure.operation != test.operation || failure.cause != test.cause {
				t.Fatalf("read failure = %+v", failure)
			}
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	failure := readExactAuthorityForParse(
		canceled,
		authorityReaderAtFunc(func([]byte, int64) (int, error) {
			t.Fatal("reader called after cancellation")
			return 0, nil
		}),
		make([]byte, 1),
	)
	if failure == nil || failure.operation != OperationParse || failure.cause != CauseCanceled {
		t.Fatalf("canceled read = %+v", failure)
	}
}

func TestPreflightAuthorityRevalidatorRequiresEveryPrimitive(t *testing.T) {
	fixture := newLiveGoEnvironmentFixture(t)
	for _, test := range []struct {
		name   string
		mutate func(*preflightAuthorityPrimitives)
	}{
		{name: "open", mutate: func(value *preflightAuthorityPrimitives) { value.openRelativeNoFollow = nil }},
		{name: "descriptor stat", mutate: func(value *preflightAuthorityPrimitives) { value.statDescriptor = nil }},
		{name: "filesystem stat", mutate: func(value *preflightAuthorityPrimitives) { value.statFilesystem = nil }},
		{name: "filesystem validation", mutate: func(value *preflightAuthorityPrimitives) { value.validateFilesystem = nil }},
		{name: "raw ACL", mutate: func(value *preflightAuthorityPrimitives) { value.acquireRawACL = nil }},
		{name: "ACL parse", mutate: func(value *preflightAuthorityPrimitives) { value.parseRawACL = nil }},
		{name: "exact read", mutate: func(value *preflightAuthorityPrimitives) { value.readExactForParse = nil }},
		{name: "hash", mutate: func(value *preflightAuthorityPrimitives) { value.hashBytes = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			primitives := fixture.primitives()
			test.mutate(&primitives)
			revalidator, record := newPreflightAuthorityRevalidatorWith(primitives)
			if revalidator != nil {
				t.Fatal("incomplete primitive adapter constructed a revalidator")
			}
			requireAuthorityPrimitiveRecord(
				t,
				record,
				OperationValidate,
				CauseInternalInvariant,
			)
		})
	}
}

func TestLiveGoEnvironmentEntryMethodsRefuseInvalidRevalidator(t *testing.T) {
	fixture := newLiveGoEnvironmentFixture(t)
	valid := fixture.revalidator(t)
	claim, admitted := valid.admitLiveGoEnvironment(
		context.Background(),
		fixture.physicalRoot,
		fixture.goroot,
	)
	if claim == nil || !admitted.proved() {
		t.Fatalf("fixture admission failed: %+v", admitted)
	}
	for _, test := range []struct {
		name        string
		revalidator *preflightAuthorityRevalidator
	}{
		{name: "nil"},
		{name: "zero", revalidator: new(preflightAuthorityRevalidator)},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture.resetTrace()
			minted, outcome := test.revalidator.admitLiveGoEnvironment(
				context.Background(),
				fixture.physicalRoot,
				fixture.goroot,
			)
			if minted != nil {
				t.Fatal("invalid revalidator minted a claim")
			}
			requireAuthorityPrimitiveRecord(
				t,
				outcome.primary,
				OperationValidate,
				CauseInternalInvariant,
			)
			outcome = test.revalidator.revalidateLiveGoEnvironment(
				context.Background(),
				fixture.physicalRoot,
				fixture.goroot,
				claim,
			)
			requireAuthorityPrimitiveRecord(
				t,
				outcome.primary,
				OperationValidate,
				CauseInternalInvariant,
			)
			if len(fixture.events) != 0 {
				t.Fatalf("invalid revalidator invoked primitives: %q", fixture.events)
			}
		})
	}
}

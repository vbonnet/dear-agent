//go:build darwin && arm64

package buildauthority

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinPreflightAuthorityRawNullACLParser(t *testing.T) {
	want, err := digestDarwinFilesec(nil)
	if err != nil {
		t.Fatalf("digest absent ACL: %v", err)
	}
	standard := append([]byte{0}, makeDarwinTestAttrResult(nil)...)
	fallbackResult := make([]byte, darwinNullACLResultSize)
	binary.LittleEndian.PutUint32(fallbackResult[0:4], darwinNullACLResultSize)
	binary.LittleEndian.PutUint32(fallbackResult[4:8], unix.ATTR_CMN_RETURNED_ATTRS)
	binary.LittleEndian.PutUint32(fallbackResult[24:28], 8)
	fallback := append([]byte{1}, fallbackResult...)

	for _, test := range []struct {
		name string
		raw  []byte
	}{
		{name: "standard", raw: standard},
		{name: "devfs fallback", raw: fallback},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, failure := parsePreflightAuthorityRawNullACL(
				context.Background(),
				test.raw,
			)
			if failure != nil || got != want {
				t.Fatalf("null ACL parse = %x / %+v, want %x / nil", got, failure, want)
			}
		})
	}

	for _, test := range []struct {
		name string
		raw  []byte
	}{
		{name: "nil", raw: nil},
		{name: "tag only", raw: []byte{0}},
		{name: "unknown tag", raw: []byte{2, 0}},
		{name: "malformed standard", raw: []byte{0, 0}},
		{name: "malformed fallback", raw: []byte{1, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, failure := parsePreflightAuthorityRawNullACL(
				context.Background(),
				test.raw,
			)
			assertPreflightPrimitiveFailure(t, failure, OperationParse, CauseMalformed)
		})
	}

	for _, termination := range []struct {
		name  string
		ctx   context.Context
		cause CauseCode
	}{
		{name: "canceled", ctx: canceledPreflightPrimitiveContext(), cause: CauseCanceled},
		{name: "deadline", ctx: expiredPreflightPrimitiveContext(t), cause: CauseDeadline},
	} {
		t.Run(termination.name, func(t *testing.T) {
			_, failure := parsePreflightAuthorityRawNullACL(termination.ctx, standard)
			assertPreflightPrimitiveFailure(t, failure, OperationParse, termination.cause)
		})
	}

	ctx := &stagedPreflightPrimitiveContext{failAfter: 1, err: context.Canceled}
	_, failure := parsePreflightAuthorityRawNullACL(ctx, standard)
	assertPreflightPrimitiveFailure(t, failure, OperationParse, CauseCanceled)
}

func TestDarwinPreflightAuthorityACLComparisonDefersPolicy(t *testing.T) {
	unsafe := makeDarwinTestAttrResult(makeDarwinTestFilesec(
		1,
		0,
		[]darwinTestACE{{flags: darwinACEPermit, rights: darwinMutationRights}},
	))
	if _, failure := parsePreflightAuthorityRawACL(context.Background(), unsafe); failure == nil ||
		failure.operation != OperationParse || failure.cause != CausePermission {
		t.Fatalf("legacy unsafe ACL parse = %+v, want parse/permission", failure)
	}
	comparison, failure := parsePreflightAuthorityRawACLForComparison(
		context.Background(),
		unsafe,
	)
	if failure != nil || comparison.digest == (Digest{}) {
		t.Fatalf("comparison unsafe ACL parse = %+v / %+v", comparison, failure)
	}
	assertPreflightPrimitiveFailure(
		t,
		comparison.policyFailure,
		OperationValidate,
		CausePermission,
	)
	observed, err := observeExtendedSecurityResult(unsafe)
	if err != nil || comparison.digest != observed.digest {
		t.Fatalf("comparison ACL digest = %x, want %x / %v", comparison.digest, observed.digest, err)
	}
}

func TestDarwinPreflightAuthorityACLLegacyPreservesFirstPolicyFailure(t *testing.T) {
	attribute := makeDarwinTestFilesec(
		2,
		0,
		[]darwinTestACE{
			{flags: darwinACEPermit, rights: darwinMutationRights},
			{flags: 3},
		},
	)
	_, err := digestDarwinFilesec(attribute)
	assertPrivateCause(t, err, CausePermission)
	_, err = digestExtendedSecurityResult(makeDarwinTestAttrResult(attribute))
	assertPrivateCause(t, err, CausePermission)

	if _, err := observeDarwinFilesec(attribute); err == nil {
		t.Fatal("comparison ACL observation admitted malformed trailing ACE")
	} else {
		assertPrivateCause(t, err, CauseMalformed)
	}
	if _, err := observeExtendedSecurityResult(makeDarwinTestAttrResult(attribute)); err == nil {
		t.Fatal("comparison extended-security observation admitted malformed trailing ACE")
	} else {
		assertPrivateCause(t, err, CauseMalformed)
	}
}

func TestDarwinPreflightAuthorityNullFallbackPresenceIsComparable(t *testing.T) {
	absentRaw := make([]byte, darwinNullACLResultSize)
	binary.LittleEndian.PutUint32(absentRaw[0:4], darwinNullACLResultSize)
	binary.LittleEndian.PutUint32(absentRaw[4:8], unix.ATTR_CMN_RETURNED_ATTRS)
	binary.LittleEndian.PutUint32(absentRaw[24:28], 8)
	absent, failure := parsePreflightAuthorityRawNullACLForComparison(
		context.Background(),
		append([]byte{1}, absentRaw...),
	)
	if failure != nil || absent.policyFailure != nil {
		t.Fatalf("absent fallback observation = %+v / %+v", absent, failure)
	}

	presentRaw := append([]byte(nil), absentRaw...)
	binary.LittleEndian.PutUint32(
		presentRaw[4:8],
		unix.ATTR_CMN_RETURNED_ATTRS|unix.ATTR_CMN_EXTENDED_SECURITY,
	)
	present, failure := parsePreflightAuthorityRawNullACLForComparison(
		context.Background(),
		append([]byte{1}, presentRaw...),
	)
	if failure != nil || present.digest == (Digest{}) || present.digest == absent.digest {
		t.Fatalf("present fallback observation = %+v / %+v, absent %x", present, failure, absent.digest)
	}
	assertPreflightPrimitiveFailure(
		t,
		present.policyFailure,
		OperationValidate,
		CauseUnsupported,
	)
	if _, legacyFailure := parsePreflightAuthorityRawNullACL(
		context.Background(),
		append([]byte{1}, presentRaw...),
	); legacyFailure == nil || legacyFailure.operation != OperationParse ||
		legacyFailure.cause != CauseUnsupported {
		t.Fatalf("legacy present fallback parse = %+v, want parse/unsupported", legacyFailure)
	}

	fullSize := append([]byte(nil), presentRaw...)
	binary.LittleEndian.PutUint32(fullSize[0:4], darwinNullACLResultSize+64)
	fullSizeObservation, fullSizeFailure := parsePreflightAuthorityRawNullACLForComparison(
		context.Background(),
		append([]byte{1}, fullSize...),
	)
	if fullSizeFailure != nil || fullSizeObservation.digest == absent.digest {
		t.Fatalf("full-size present fallback = %+v / %+v", fullSizeObservation, fullSizeFailure)
	}
	assertPreflightPrimitiveFailure(
		t,
		fullSizeObservation.policyFailure,
		OperationValidate,
		CauseUnsupported,
	)
}

func TestDarwinPreflightAuthorityA2PrimitiveClassifiersAreClosed(t *testing.T) {
	for _, test := range []struct {
		name      string
		invoke    func() *authorityPrimitiveFailure
		operation Operation
		cause     CauseCode
	}{
		{
			name: "expected acquisition absence",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityAcquisitionFailure(
					context.Background(), authorityExpectedPresentOpen, unix.ENOENT,
				)
			},
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "initial acquisition absence",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityAcquisitionFailure(
					context.Background(), authorityInitialRequiredOpen, unix.ENOENT,
				)
			},
			operation: OperationOpen,
			cause:     CauseNotFound,
		},
		{
			name: "acquisition permission",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityAcquisitionFailure(
					context.Background(), authorityExpectedPresentOpen, unix.EACCES,
				)
			},
			operation: OperationOpen,
			cause:     CausePermission,
		},
		{
			name: "acquisition unsupported",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityAcquisitionFailure(
					context.Background(), authorityExpectedPresentOpen, unix.ENOTSUP,
				)
			},
			operation: OperationOpen,
			cause:     CauseUnsupported,
		},
		{
			name: "acquisition I/O",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityAcquisitionFailure(
					context.Background(), authorityExpectedPresentOpen, unix.EIO,
				)
			},
			operation: OperationOpen,
			cause:     CauseUnstable,
		},
		{
			name: "expected acquisition symlink",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityAcquisitionFailure(
					context.Background(), authorityExpectedPresentOpen, unix.ELOOP,
				)
			},
			operation: OperationCompare,
			cause:     CauseIdentity,
		},
		{
			name: "initial acquisition nondirectory",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityAcquisitionFailure(
					context.Background(), authorityInitialRequiredOpen, unix.ENOTDIR,
				)
			},
			operation: OperationOpen,
			cause:     CauseIdentity,
		},
		{
			name: "presence ENOTSUP",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityPresenceFailure(
					context.Background(), authorityExpectedPresentOpen, unix.ENOTSUP,
				)
			},
			operation: OperationProbe,
			cause:     CauseUnstable,
		},
		{
			name: "walk ENOTSUP",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityWalkFailure(context.Background(), unix.ENOTSUP)
			},
			operation: OperationWalk,
			cause:     CauseUnstable,
		},
		{
			name: "walk permission",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityWalkFailure(context.Background(), unix.EACCES)
			},
			operation: OperationWalk,
			cause:     CausePermission,
		},
		{
			name: "special expected kind",
			invoke: func() *authorityPrimitiveFailure {
				_, failure := classifyPreflightAuthorityObservedKind(
					authorityExpectedPresentOpen,
					platformModeCharacter,
				)
				return failure
			},
			operation: OperationCompare,
			cause:     CauseIdentity,
		},
		{
			name: "special initial kind",
			invoke: func() *authorityPrimitiveFailure {
				_, failure := classifyPreflightAuthorityObservedKind(
					authorityInitialRequiredOpen,
					platformModeCharacter,
				)
				return failure
			},
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
		{
			name: "canceled acquisition",
			invoke: func() *authorityPrimitiveFailure {
				return classifyPreflightAuthorityAcquisitionFailure(
					canceledPreflightPrimitiveContext(),
					authorityExpectedPresentOpen,
					unix.EIO,
				)
			},
			operation: OperationOpen,
			cause:     CauseCanceled,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertPreflightPrimitiveFailure(t, test.invoke(), test.operation, test.cause)
		})
	}
}

func TestDarwinPreflightAuthoritySymlinkWalkReadIsExact(t *testing.T) {
	descriptor := new(os.File)
	for _, test := range []struct {
		name      string
		expected  int64
		limit     uint64
		read      darwinFreadlinkFunc
		want      string
		operation Operation
		cause     CauseCode
	}{
		{
			name:     "exact",
			expected: 3,
			limit:    3,
			read: func(_ *os.File, buffer []byte) (uintptr, error) {
				copy(buffer, "abc")
				return 3, nil
			},
			want: "abc",
		},
		{
			name:     "short",
			expected: 3,
			limit:    3,
			read: func(_ *os.File, buffer []byte) (uintptr, error) {
				copy(buffer, "ab")
				return 2, nil
			},
			operation: OperationWalk,
			cause:     CauseUnstable,
		},
		{
			name:      "zero",
			expected:  3,
			limit:     3,
			read:      func(*os.File, []byte) (uintptr, error) { return 0, nil },
			operation: OperationWalk,
			cause:     CauseUnstable,
		},
		{
			name:     "over limit",
			expected: 3,
			limit:    3,
			read: func(_ *os.File, buffer []byte) (uintptr, error) {
				copy(buffer, "abcd")
				return 4, nil
			},
			operation: OperationWalk,
			cause:     CauseLimit,
		},
		{
			name:      "permission",
			expected:  3,
			limit:     3,
			read:      func(*os.File, []byte) (uintptr, error) { return 0, unix.EACCES },
			operation: OperationWalk,
			cause:     CausePermission,
		},
		{
			name:      "unsupported syscall",
			expected:  3,
			limit:     3,
			read:      func(*os.File, []byte) (uintptr, error) { return 0, unix.ENOTSUP },
			operation: OperationWalk,
			cause:     CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, failure := readPreflightAuthoritySymlinkForWalkWith(
				context.Background(),
				descriptor,
				test.expected,
				test.limit,
				test.read,
			)
			if test.operation == "" {
				if failure != nil || got != test.want {
					t.Fatalf("symlink read = %q / %+v, want %q / nil", got, failure, test.want)
				}
				return
			}
			assertPreflightPrimitiveFailure(t, failure, test.operation, test.cause)
		})
	}
}

func TestDarwinPreflightAuthorityProductionPhysicalRootAndNullBrackets(t *testing.T) {
	ctx := context.Background()
	revalidator, factoryFailure := newPreflightAuthorityRevalidator()
	if factoryFailure != nil || revalidator == nil || !revalidator.validForBrackets() {
		t.Fatalf("production revalidator = %+v / %+v", revalidator, factoryFailure)
	}

	physicalRoot, err := retainPhysicalRoot(ctx)
	if err != nil {
		t.Fatalf("retain production physical root: %v", err)
	}
	physicalRootClosed := false
	t.Cleanup(func() {
		if !physicalRootClosed {
			_ = physicalRoot.close()
		}
	})

	nullDevice, err := retainNullDevice(ctx)
	if err != nil {
		t.Fatalf("retain production null device: %v", err)
	}
	nullDeviceClosed := false
	t.Cleanup(func() {
		if !nullDeviceClosed {
			_ = nullDevice.close()
		}
	})

	rootOutcome := revalidator.revalidatePhysicalRoot(ctx, physicalRoot)
	if !rootOutcome.proved() {
		t.Fatalf(
			"production physical-root bracket = primary %+v / later %+v / close %+v",
			rootOutcome.primary,
			rootOutcome.later,
			rootOutcome.descriptorClose,
		)
	}
	nullOutcome := revalidator.revalidateRetainedNull(ctx, physicalRoot, nullDevice)
	if !nullOutcome.proved() {
		t.Fatalf(
			"production retained-null bracket = primary %+v / later %+v / close %+v",
			nullOutcome.primary,
			nullOutcome.later,
			nullOutcome.descriptorClose,
		)
	}

	nullDescriptor := nullDevice.leaf.descriptor
	if err := nullDevice.close(); err != nil {
		t.Fatalf("close production retained null: %v", err)
	}
	nullDeviceClosed = true
	if _, err := nullDescriptor.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("retained null descriptor remains open: %v", err)
	}

	rootDescriptor := physicalRoot.directory.descriptor
	if err := physicalRoot.close(); err != nil {
		t.Fatalf("close production physical root: %v", err)
	}
	physicalRootClosed = true
	if _, err := rootDescriptor.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("physical-root descriptor remains open: %v", err)
	}
}

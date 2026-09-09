package buildauthority

import (
	"context"
	"os"
	"strings"
)

// sourcePrimitiveFailure preserves the operation owned by the first failing
// C1 primitive. Raw operating-system and parser errors never cross this seam.
type sourcePrimitiveFailure struct {
	operation Operation
	cause     CauseCode
}

func newSourcePrimitiveFailure(operation Operation, cause CauseCode) *sourcePrimitiveFailure {
	if !validSourcePrimitiveOperation(operation) || !validSourceCause(cause) {
		return &sourcePrimitiveFailure{
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		}
	}
	return &sourcePrimitiveFailure{operation: operation, cause: cause}
}

func (failure *sourcePrimitiveFailure) record() *FailureRecord {
	if failure == nil {
		return nil
	}
	return &FailureRecord{
		Phase:     PhaseSource,
		Operation: failure.operation,
		Causes:    []CauseCode{failure.cause},
	}
}

func validSourcePrimitiveOperation(operation Operation) bool {
	switch operation { //nolint:exhaustive // C1 intentionally admits only its seven primitive operations.
	case OperationValidate, OperationOpen, OperationProbe, OperationWalk,
		OperationParse, OperationHash, OperationCompare:
		return true
	default:
		return false
	}
}

func validSourceCause(cause CauseCode) bool {
	switch cause { //nolint:exhaustive // Source primitives intentionally exclude process and close-only causes.
	case CauseInvalidRequest, CauseNotFound, CausePermission, CauseMalformed, CauseUnsupported,
		CauseUnstable, CauseLimit, CauseCanceled, CauseDeadline, CauseIdentity,
		CauseInternalInvariant:
		return true
	default:
		return false
	}
}

func sourcePrimitiveFailureFromError(
	operation Operation,
	err error,
	fallback CauseCode,
) *sourcePrimitiveFailure {
	if err == nil {
		return nil
	}
	causes := privateCauses(err, fallback)
	if len(causes) != 1 || causes[0] == CauseInternalInvariant {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	return newSourcePrimitiveFailure(operation, causes[0])
}

func sourceContextPrimitiveFailure(
	ctx context.Context,
	operation Operation,
) *sourcePrimitiveFailure {
	if ctx == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if err := ctx.Err(); err != nil {
		cause := contextCause(err)
		if cause == CauseInternalInvariant {
			return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		return newSourcePrimitiveFailure(operation, cause)
	}
	return nil
}

// sourceUseOutcome has deliberately no command/block private-later slot. A C1
// tail discards every later non-close failure while preserving the first
// descriptor-close failure independently.
type sourceUseOutcome struct {
	primary         *FailureRecord
	descriptorClose *FailureRecord
}

func (outcome *sourceUseOutcome) addPrimitive(failure *sourcePrimitiveFailure) {
	if outcome == nil || failure == nil || outcome.primary != nil {
		return
	}
	outcome.primary = cloneFailureRecord(failure.record())
}

func (outcome *sourceUseOutcome) addDescriptorClose() {
	if outcome == nil || outcome.descriptorClose != nil {
		return
	}
	outcome.descriptorClose = &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationCloseNonRoot,
		Causes:    []CauseCode{CauseDescriptorClose},
	}
}

func (outcome sourceUseOutcome) proved() bool {
	return outcome.primary == nil && outcome.descriptorClose == nil
}

type sourceRepositoryLocatorSeal uint8

const validSourceRepositoryLocator sourceRepositoryLocatorSeal = 1

// sourceRepositoryLocator is minted only after public request validation. It
// carries a locator, never an opened capability.
type sourceRepositoryLocator struct {
	path string
	seal sourceRepositoryLocatorSeal
}

func (locator sourceRepositoryLocator) valid() bool {
	if locator.seal != validSourceRepositoryLocator || strings.IndexByte(locator.path, 0) >= 0 {
		return false
	}
	_, err := absolutePathComponents(locator.path)
	return err == nil && locator.path != physicalRootPath
}

type sourcePresenceMode uint8

const (
	sourceInitialRequired sourcePresenceMode = iota + 1
	sourceInitialOptional
	sourceInitialForbidden
	sourceRevalidatePresent
	sourceRevalidateAbsent
)

func (mode sourcePresenceMode) valid() bool {
	return mode >= sourceInitialRequired && mode <= sourceRevalidateAbsent
}

// sourceObservedKind is C1's closed no-follow lookup result. Keeping special
// entries in this nominal vocabulary avoids widening the package-wide
// entryKind enum merely to represent a source-policy refusal.
type sourceObservedKind uint8

const (
	sourceObservedDirectory sourceObservedKind = iota + 1
	sourceObservedRegular
	sourceObservedSymlink
	sourceObservedSpecial
)

func (kind sourceObservedKind) valid() bool {
	return kind >= sourceObservedDirectory && kind <= sourceObservedSpecial
}

func observedSourceKind(kind entryKind) sourceObservedKind {
	switch kind {
	case entryDirectory:
		return sourceObservedDirectory
	case entryRegular:
		return sourceObservedRegular
	case entrySymlink:
		return sourceObservedSymlink
	}
	return sourceObservedSpecial
}

func (kind sourceObservedKind) openKind() (entryKind, bool) {
	switch kind {
	case sourceObservedDirectory:
		return entryDirectory, true
	case sourceObservedRegular:
		return entryRegular, true
	case sourceObservedSymlink:
		return entrySymlink, true
	case sourceObservedSpecial:
		return 0, false
	}
	return 0, false
}

type sourceHandleState uint8

const (
	sourceHandleOpen sourceHandleState = iota + 1
	sourceHandleClosed
)

type sourceACLDisposition uint8

const (
	sourceACLAdmitted sourceACLDisposition = iota + 1
	sourceACLMutationPermitting
)

func (disposition sourceACLDisposition) valid() bool {
	return disposition == sourceACLAdmitted || disposition == sourceACLMutationPermitting
}

// parsedSourceACL keeps syntax-derived evidence separate from its policy
// decision. Parsing owns malformed bytes; validateACL owns a well-formed but
// mutation-permitting access-control entry.
type parsedSourceACL struct {
	digest      Digest
	disposition sourceACLDisposition
}

func (acl parsedSourceACL) valid() bool {
	return acl.disposition.valid()
}

// ownedSourceRoot and ownedSourceDescriptor are transient exact-once owners.
// Their concrete handle fields never appear in sourcePrimitives signatures.
type ownedSourceRoot struct {
	root         *os.Root
	state        sourceHandleState
	closeFailure bool
}

type ownedSourceDescriptor struct {
	file         *os.File
	kind         sourceObservedKind
	state        sourceHandleState
	closeFailure bool
}

func (owner *ownedSourceRoot) validOpen() bool {
	return owner != nil && owner.state == sourceHandleOpen && owner.root != nil
}

func (owner *ownedSourceDescriptor) validOpen() bool {
	return owner != nil && owner.state == sourceHandleOpen && owner.file != nil && owner.kind.valid()
}

func (owner *ownedSourceRoot) closeDirect() bool {
	if owner == nil {
		return true
	}
	if owner.state == sourceHandleClosed {
		return owner.closeFailure
	}
	root := owner.root
	owner.root = nil
	invalid := owner.state != sourceHandleOpen || root == nil
	owner.state = sourceHandleClosed
	owner.closeFailure = invalid
	if root != nil && root.Close() != nil {
		owner.closeFailure = true
	}
	return owner.closeFailure
}

func (owner *ownedSourceDescriptor) closeDirect() bool {
	if owner == nil {
		return true
	}
	if owner.state == sourceHandleClosed {
		return owner.closeFailure
	}
	descriptor := owner.file
	owner.file = nil
	invalid := owner.state != sourceHandleOpen || descriptor == nil
	owner.state = sourceHandleClosed
	owner.closeFailure = invalid
	if descriptor != nil && descriptor.Close() != nil {
		owner.closeFailure = true
	}
	return owner.closeFailure
}

// sourcePrimitives is the transient deterministic seam for C1 filesystem and
// byte operations. It is never stored in a retained source owner.
type sourcePrimitives interface {
	openRepositoryRoot(
		context.Context,
		sourceRepositoryLocator,
	) (*ownedSourceRoot, *sourcePrimitiveFailure)
	openPhysicalRootDescriptor(context.Context) (*ownedSourceDescriptor, *sourcePrimitiveFailure)
	probeRelativeKind(
		context.Context,
		*ownedSourceDescriptor,
		string,
		sourcePresenceMode,
	) (sourceObservedKind, bool, *sourcePrimitiveFailure)
	openRelativeNoFollow(
		context.Context,
		*ownedSourceDescriptor,
		string,
		sourceObservedKind,
		sourcePresenceMode,
	) (*ownedSourceDescriptor, *sourcePrimitiveFailure)
	openChildRoot(
		context.Context,
		*ownedSourceRoot,
		string,
	) (*ownedSourceRoot, *sourcePrimitiveFailure)
	statDescriptor(
		context.Context,
		*ownedSourceDescriptor,
	) (fileSnapshot, *sourcePrimitiveFailure)
	statFilesystem(
		context.Context,
		*ownedSourceDescriptor,
	) (mountSnapshot, *sourcePrimitiveFailure)
	validateFilesystem(context.Context, mountSnapshot) *sourcePrimitiveFailure
	acquireRawACL(
		context.Context,
		*ownedSourceDescriptor,
	) ([]byte, *sourcePrimitiveFailure)
	parseRawACL(context.Context, []byte) (parsedSourceACL, *sourcePrimitiveFailure)
	validateACL(context.Context, parsedSourceACL) *sourcePrimitiveFailure
	readExactForParse(
		context.Context,
		*ownedSourceDescriptor,
		[]byte,
	) *sourcePrimitiveFailure
	hashBytes(context.Context, []byte) (Digest, *sourcePrimitiveFailure)
	compareRootAndDescriptor(
		context.Context,
		*ownedSourceRoot,
		*ownedSourceDescriptor,
	) *sourcePrimitiveFailure
	closeRoot(*ownedSourceRoot) bool
	closeDescriptor(*ownedSourceDescriptor) bool
	privateSourcePrimitives()
}

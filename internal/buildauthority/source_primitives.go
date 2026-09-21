package buildauthority

import (
	"context"
	"crypto/sha1" //nolint:gosec // Git's closed object format requires SHA-1 compatibility.
	"crypto/sha256"
	"encoding"
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

// sourceContentDigestState keeps only the standard library's value-encoded
// hash state. The hash.Hash and encoding interfaces used to advance it are
// transient locals inside this primitive module; source construction never
// retains a callable or type-erased hashing capability.
type sourceContentDigestAlgorithm uint8

const (
	sourceContentDigestSHA1 sourceContentDigestAlgorithm = iota + 1
	sourceContentDigestSHA256
	sourceContentDigestStateBytes = 256
)

type sourceContentDigestState struct {
	algorithm sourceContentDigestAlgorithm
	wire      [sourceContentDigestStateBytes]byte
	wireN     uint16
}

func newSourceContentDigestState(
	algorithm sourceContentDigestAlgorithm,
) (sourceContentDigestState, bool) {
	state := sourceContentDigestState{algorithm: algorithm}
	if !state.consume(nil) {
		return sourceContentDigestState{}, false
	}
	return state, true
}

//nolint:gocyclo // The closed algorithm switch keeps transient hash-interface use locally auditable.
func (state *sourceContentDigestState) consume(content []byte) bool {
	if state == nil {
		return false
	}
	switch state.algorithm {
	case sourceContentDigestSHA1:
		hasher := sha1.New() //nolint:gosec // Git SHA-1 object-format compatibility.
		restorer, restoreOK := hasher.(encoding.BinaryUnmarshaler)
		saver, saveOK := hasher.(encoding.BinaryAppender)
		if !restoreOK || !saveOK ||
			(state.wireN != 0 && restorer.UnmarshalBinary(state.wire[:state.wireN]) != nil) {
			return false
		}
		written, writeErr := hasher.Write(content)
		if writeErr != nil || written != len(content) {
			return false
		}
		wire, saveErr := saver.AppendBinary(state.wire[:0])
		return saveErr == nil && state.saveWire(wire)
	case sourceContentDigestSHA256:
		hasher := sha256.New()
		restorer, restoreOK := hasher.(encoding.BinaryUnmarshaler)
		saver, saveOK := hasher.(encoding.BinaryAppender)
		if !restoreOK || !saveOK ||
			(state.wireN != 0 && restorer.UnmarshalBinary(state.wire[:state.wireN]) != nil) {
			return false
		}
		written, writeErr := hasher.Write(content)
		if writeErr != nil || written != len(content) {
			return false
		}
		wire, saveErr := saver.AppendBinary(state.wire[:0])
		return saveErr == nil && state.saveWire(wire)
	default:
		return false
	}
}

func (state *sourceContentDigestState) saveWire(wire []byte) bool {
	if state == nil || len(wire) == 0 || len(wire) > len(state.wire) {
		return false
	}
	state.wireN = uint16(len(wire)) //nolint:gosec // len(wire) was proved at most the 256-byte fixed buffer.
	return true
}

func (state sourceContentDigestState) sum() ([32]byte, int, bool) {
	switch state.algorithm {
	case sourceContentDigestSHA1:
		hasher := sha1.New() //nolint:gosec // Git SHA-1 object-format compatibility.
		restorer, ok := hasher.(encoding.BinaryUnmarshaler)
		if !ok || state.wireN == 0 || restorer.UnmarshalBinary(state.wire[:state.wireN]) != nil {
			return [32]byte{}, 0, false
		}
		var digest [32]byte
		value := hasher.Sum(digest[:0])
		if len(value) != sha1.Size {
			return [32]byte{}, 0, false
		}
		return digest, sha1.Size, true
	case sourceContentDigestSHA256:
		hasher := sha256.New()
		restorer, ok := hasher.(encoding.BinaryUnmarshaler)
		if !ok || state.wireN == 0 || restorer.UnmarshalBinary(state.wire[:state.wireN]) != nil {
			return [32]byte{}, 0, false
		}
		var digest [32]byte
		value := hasher.Sum(digest[:0])
		if len(value) != sha256.Size {
			return [32]byte{}, 0, false
		}
		return digest, sha256.Size, true
	default:
		return [32]byte{}, 0, false
	}
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
	// sourceInitialWalkPresent is used only after a retained directory
	// descriptor has yielded the name. Absence is therefore a lookup race,
	// never the clean initial absence represented by sourceInitialRequired.
	sourceInitialWalkPresent
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

const sourceDirectoryReadBatchSize = 128

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
	invalid := owner.state != sourceHandleOpen
	owner.state = sourceHandleClosed
	owner.closeFailure = invalid
	if owner.root == nil {
		owner.closeFailure = true
	} else if owner.root.Close() != nil {
		owner.closeFailure = true
	}
	owner.root = nil
	return owner.closeFailure
}

func (owner *ownedSourceDescriptor) closeDirect() bool {
	if owner == nil {
		return true
	}
	if owner.state == sourceHandleClosed {
		return owner.closeFailure
	}
	invalid := owner.state != sourceHandleOpen
	owner.state = sourceHandleClosed
	owner.closeFailure = invalid
	if owner.file == nil {
		owner.closeFailure = true
	} else if owner.file.Close() != nil {
		owner.closeFailure = true
	}
	owner.file = nil
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
	openRootDirectoryDescriptor(
		context.Context,
		*ownedSourceRoot,
	) (*ownedSourceDescriptor, *sourcePrimitiveFailure)
	readDirectoryBatch(
		context.Context,
		*ownedSourceDescriptor,
	) ([]string, bool, *sourcePrimitiveFailure)
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
	readExactAtForParse(
		context.Context,
		*ownedSourceDescriptor,
		int64,
		[]byte,
	) *sourcePrimitiveFailure
	readExactAtForHash(
		context.Context,
		*ownedSourceDescriptor,
		int64,
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

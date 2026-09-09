package buildauthority

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"time"
)

const (
	nonSourceCommandDuration     = 30 * time.Second
	noScratchTransactionDuration = time.Hour
	nonSourceIntentionalKillExit = -9
)

type nonSourceCommandKind uint8

const (
	nonSourceCommandGoVersion nonSourceCommandKind = iota + 1
	nonSourceCommandGoEnvironment
	nonSourceCommandCompilerVersion
	nonSourceCommandGitVersion
	nonSourceCommandGitBuiltinInventory
)

func (kind nonSourceCommandKind) valid() bool {
	return kind >= nonSourceCommandGoVersion &&
		kind <= nonSourceCommandGitBuiltinInventory
}

type nonSourceWindowIssuerSeal uint8
type nonSourceTransactionWindowSeal uint8
type nonSourceCommandWindowSeal uint8
type retainedNullCommandWitnessSeal uint8
type nonSourceCommandAuthoritySeal uint8
type nonSourceProofSeal uint8
type nonSourceCallerDeadlineFloorSeal uint8

const (
	validNonSourceWindowIssuer        nonSourceWindowIssuerSeal        = 1
	validNonSourceTransactionWindow   nonSourceTransactionWindowSeal   = 1
	validNonSourceCommandWindow       nonSourceCommandWindowSeal       = 1
	validRetainedNullCommandWitness   retainedNullCommandWitnessSeal   = 1
	validNonSourceCommandAuthority    nonSourceCommandAuthoritySeal    = 1
	validNonSourceProof               nonSourceProofSeal               = 1
	validNonSourceCallerDeadlineFloor nonSourceCallerDeadlineFloorSeal = 1
)

// nonSourceWindowIssuer is the shared clock identity. Production constructs
// the process supervisor from this exact issuer, so the transaction, command
// windows, and child event loop observe one scheduler instance.
type nonSourceWindowIssuer struct {
	mu          sync.Mutex
	transaction *nonSourceTransactionState
	scheduler   processScheduler
	seal        nonSourceWindowIssuerSeal
}

func newNonSourceWindowIssuer(scheduler processScheduler) *nonSourceWindowIssuer {
	if scheduler == nil {
		return nil
	}
	return &nonSourceWindowIssuer{
		scheduler: scheduler,
		seal:      validNonSourceWindowIssuer,
	}
}

func (issuer *nonSourceWindowIssuer) valid() bool {
	return issuer != nil && issuer.seal == validNonSourceWindowIssuer &&
		issuer.scheduler != nil
}

type nonSourceTransactionState struct {
	issuer              *nonSourceWindowIssuer
	ctx                 context.Context
	sampledAt           time.Time
	callerDeadline      time.Time
	callerDeadlineFloor *nonSourceCallerDeadlineFloor
	transactionDeadline time.Time
	authorityContext    *nonSourceAuthorityContext
	seal                nonSourceTransactionWindowSeal
}

// nonSourceCallerDeadlineFloor remembers the earliest caller deadline ever
// observed during the transaction. A context may shorten authority after
// entry, but a later extension or removal cannot widen it again.
type nonSourceCallerDeadlineFloor struct {
	mu              sync.Mutex
	initialDeadline time.Time
	deadline        time.Time
	hasDeadline     bool
	seal            nonSourceCallerDeadlineFloorSeal
}

func newNonSourceCallerDeadlineFloor(
	initialDeadline time.Time,
) *nonSourceCallerDeadlineFloor {
	return &nonSourceCallerDeadlineFloor{
		initialDeadline: initialDeadline,
		deadline:        initialDeadline,
		hasDeadline:     !initialDeadline.IsZero(),
		seal:            validNonSourceCallerDeadlineFloor,
	}
}

func (floor *nonSourceCallerDeadlineFloor) validSnapshot(
	initialDeadline, snapshot time.Time,
) bool {
	if floor == nil {
		return false
	}
	floor.mu.Lock()
	defer floor.mu.Unlock()
	if floor.seal != validNonSourceCallerDeadlineFloor ||
		!floor.initialDeadline.Equal(initialDeadline) ||
		!floor.hasDeadline && !floor.deadline.IsZero() {
		return false
	}
	if snapshot.IsZero() {
		return initialDeadline.IsZero()
	}
	return floor.hasDeadline &&
		(floor.deadline.IsZero() || !floor.deadline.After(snapshot))
}

func (floor *nonSourceCallerDeadlineFloor) lower(
	deadline time.Time,
	hasDeadline bool,
) (time.Time, bool) {
	if floor == nil {
		return time.Time{}, false
	}
	floor.mu.Lock()
	defer floor.mu.Unlock()
	if floor.seal != validNonSourceCallerDeadlineFloor {
		return time.Time{}, false
	}
	if hasDeadline && (!floor.hasDeadline ||
		(!floor.deadline.IsZero() &&
			(deadline.IsZero() || deadline.Before(floor.deadline)))) {
		floor.deadline = deadline
		floor.hasDeadline = true
	}
	return floor.deadline, floor.hasDeadline
}

func (floor *nonSourceCallerDeadlineFloor) snapshot() (time.Time, bool) {
	return floor.lower(time.Time{}, false)
}

// nonSourceTransactionWindow is minted at no-scratch entry and copied only as
// an opaque reference to immutable state. It does not confer command authority.
type nonSourceTransactionWindow struct {
	state *nonSourceTransactionState
	seal  nonSourceTransactionWindowSeal
}

func (issuer *nonSourceWindowIssuer) beginTransaction(
	ctx context.Context,
) (nonSourceTransactionWindow, *FailureRecord) {
	if !issuer.valid() {
		return nonSourceTransactionWindow{}, requestValidationFailure(CauseInternalInvariant)
	}
	issuer.mu.Lock()
	defer issuer.mu.Unlock()
	if issuer.transaction != nil {
		return nonSourceTransactionWindow{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	sampledAt := issuer.scheduler.now()
	if sampledAt.IsZero() {
		return nonSourceTransactionWindow{}, requestValidationFailure(CauseInternalInvariant)
	}
	callerDeadline, failure := nonSourceEntryContextDeadline(ctx, sampledAt)
	if failure != nil {
		return nonSourceTransactionWindow{}, failure
	}
	state := &nonSourceTransactionState{
		issuer:              issuer,
		ctx:                 ctx,
		sampledAt:           sampledAt,
		callerDeadline:      callerDeadline,
		callerDeadlineFloor: newNonSourceCallerDeadlineFloor(callerDeadline),
		transactionDeadline: sampledAt.Add(noScratchTransactionDuration),
		seal:                validNonSourceTransactionWindow,
	}
	state.authorityContext = newNonSourceAuthorityContext(
		state,
		time.Time{},
		callerDeadline,
	)
	if state.authorityContext == nil {
		return nonSourceTransactionWindow{}, requestValidationFailure(CauseInternalInvariant)
	}
	issuer.transaction = state
	return nonSourceTransactionWindow{
		state: state,
		seal:  validNonSourceTransactionWindow,
	}, nil
}

func requestValidationFailure(cause CauseCode) *FailureRecord {
	return &FailureRecord{
		Phase:     PhaseRequest,
		Operation: OperationValidate,
		Causes:    []CauseCode{cause},
	}
}

func nonSourceEntryContextDeadline(
	ctx context.Context,
	now time.Time,
) (time.Time, *FailureRecord) {
	if ctx == nil {
		return time.Time{}, requestValidationFailure(CauseInvalidRequest)
	}
	contextError := ctx.Err()
	contextDeadline, hasContextDeadline := ctx.Deadline()
	if errors.Is(contextError, context.DeadlineExceeded) ||
		(hasContextDeadline && !now.Before(contextDeadline)) {
		return time.Time{}, requestValidationFailure(CauseDeadline)
	}
	if errors.Is(contextError, context.Canceled) {
		return time.Time{}, requestValidationFailure(CauseCanceled)
	}
	if contextError != nil {
		return time.Time{}, requestValidationFailure(CauseInternalInvariant)
	}
	if !hasContextDeadline {
		return time.Time{}, nil
	}
	return contextDeadline, nil
}

func (issuer *nonSourceWindowIssuer) ownsTransaction(
	state *nonSourceTransactionState,
) bool {
	if !issuer.valid() || state == nil {
		return false
	}
	issuer.mu.Lock()
	defer issuer.mu.Unlock()
	return issuer.transaction == state
}

func (window nonSourceTransactionWindow) validFor(
	issuer *nonSourceWindowIssuer,
) bool {
	return window.seal == validNonSourceTransactionWindow &&
		window.state != nil && window.state.seal == validNonSourceTransactionWindow &&
		window.state.issuer == issuer && issuer.valid() && window.state.ctx != nil &&
		!window.state.sampledAt.IsZero() &&
		validInitialCallerDeadline(window.state.sampledAt, window.state.callerDeadline) &&
		window.state.callerDeadlineFloor.validSnapshot(
			window.state.callerDeadline,
			window.state.callerDeadline,
		) &&
		window.state.transactionDeadline.Equal(
			window.state.sampledAt.Add(noScratchTransactionDuration),
		) && window.state.authorityContext != nil &&
		window.state.authorityContext.validFor(
			window.state,
			time.Time{},
			window.state.callerDeadline,
		) &&
		issuer.ownsTransaction(window.state)
}

func validInitialCallerDeadline(sampledAt, callerDeadline time.Time) bool {
	return callerDeadline.IsZero() || sampledAt.Before(callerDeadline)
}

// nonSourceAuthorityContext is an A2-only view of the exact caller context.
// Its Err method observes the module deadlines through the same scheduler as
// the process supervisor. Process requests retain the original caller context
// and their explicit deadlines; this wrapper never crosses that seam.
type nonSourceAuthorityContext struct {
	mu             sync.Mutex
	transaction    *nonSourceTransactionState
	phaseDeadline  time.Time
	callerDeadline time.Time
	stopped        error
}

func newNonSourceAuthorityContext(
	transaction *nonSourceTransactionState,
	phaseDeadline time.Time,
	callerDeadline time.Time,
) *nonSourceAuthorityContext {
	if transaction == nil || transaction.issuer == nil ||
		!transaction.issuer.valid() || transaction.ctx == nil ||
		transaction.sampledAt.IsZero() ||
		!validInitialCallerDeadline(transaction.sampledAt, transaction.callerDeadline) ||
		!transaction.callerDeadlineFloor.validSnapshot(
			transaction.callerDeadline,
			callerDeadline,
		) ||
		!transaction.transactionDeadline.Equal(
			transaction.sampledAt.Add(noScratchTransactionDuration),
		) {
		return nil
	}
	return &nonSourceAuthorityContext{
		transaction:    transaction,
		phaseDeadline:  phaseDeadline,
		callerDeadline: callerDeadline,
	}
}

func (ctx *nonSourceAuthorityContext) validFor(
	transaction *nonSourceTransactionState,
	phaseDeadline time.Time,
	callerDeadline time.Time,
) bool {
	return ctx != nil && ctx.transaction == transaction && transaction != nil &&
		transaction.issuer != nil && transaction.issuer.valid() &&
		transaction.ctx != nil && !transaction.sampledAt.IsZero() &&
		validInitialCallerDeadline(transaction.sampledAt, transaction.callerDeadline) &&
		transaction.callerDeadlineFloor.validSnapshot(
			transaction.callerDeadline,
			callerDeadline,
		) &&
		transaction.transactionDeadline.Equal(
			transaction.sampledAt.Add(noScratchTransactionDuration),
		) && ctx.phaseDeadline.Equal(phaseDeadline) &&
		ctx.callerDeadline.Equal(callerDeadline)
}

func (ctx *nonSourceAuthorityContext) Deadline() (time.Time, bool) {
	if ctx == nil || !ctx.validFor(
		ctx.transaction,
		ctx.phaseDeadline,
		ctx.callerDeadline,
	) {
		return time.Time{}, false
	}
	dynamicDeadline, hasDynamicDeadline := ctx.transaction.ctx.Deadline()
	callerDeadline, hasCallerDeadline := ctx.transaction.callerDeadlineFloor.lower(
		dynamicDeadline,
		hasDynamicDeadline,
	)
	deadline := ctx.transaction.transactionDeadline
	if !ctx.phaseDeadline.IsZero() && ctx.phaseDeadline.Before(deadline) {
		deadline = ctx.phaseDeadline
	}
	if hasCallerDeadline && (callerDeadline.IsZero() || callerDeadline.Before(deadline)) {
		deadline = callerDeadline
	}
	return deadline, true
}

func (ctx *nonSourceAuthorityContext) Done() <-chan struct{} {
	if ctx == nil || ctx.transaction == nil || ctx.transaction.ctx == nil {
		return nil
	}
	return ctx.transaction.ctx.Done()
}

func (ctx *nonSourceAuthorityContext) Err() error {
	if ctx == nil {
		return errInvalidNonSourceAuthorityContext
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.stopped != nil {
		return ctx.stopped
	}
	transaction := ctx.transaction
	if !ctx.validFor(transaction, ctx.phaseDeadline, ctx.callerDeadline) ||
		!transaction.issuer.ownsTransaction(transaction) {
		ctx.stopped = errInvalidNonSourceAuthorityContext
		return ctx.stopped
	}
	now := transaction.issuer.scheduler.now()
	if now.IsZero() {
		ctx.stopped = errInvalidNonSourceAuthorityContext
		return ctx.stopped
	}
	callerError := transaction.ctx.Err()
	dynamicDeadline, hasDynamicDeadline := transaction.ctx.Deadline()
	callerDeadline, hasCallerDeadline := transaction.callerDeadlineFloor.lower(
		dynamicDeadline,
		hasDynamicDeadline,
	)
	if nonSourceDeadlineReached(
		now,
		ctx.phaseDeadline,
		transaction.transactionDeadline,
		callerDeadline,
		hasCallerDeadline,
		callerError,
	) {
		ctx.stopped = context.DeadlineExceeded
		return ctx.stopped
	}
	if errors.Is(callerError, context.Canceled) {
		ctx.stopped = context.Canceled
		return ctx.stopped
	}
	if callerError != nil {
		ctx.stopped = errInvalidNonSourceAuthorityContext
		return ctx.stopped
	}
	return nil
}

func (ctx *nonSourceAuthorityContext) Value(key any) any {
	if ctx == nil || ctx.transaction == nil || ctx.transaction.ctx == nil {
		return nil
	}
	return ctx.transaction.ctx.Value(key)
}

var errInvalidNonSourceAuthorityContext = errors.New(
	"invalid non-source authority context",
)

type nonSourceCommandWindowState uint8

const (
	nonSourceCommandWindowPending nonSourceCommandWindowState = iota + 1
	nonSourceCommandWindowAuthorized
	nonSourceCommandWindowConsumed
	nonSourceCommandWindowAborted
)

// nonSourceCommandWindow is pointer-only and one-shot. The retained-null
// witness is captured only after its A2 bracket proves and the window is
// authorized only after every remaining precheck proves.
type nonSourceCommandWindow struct {
	mu                  sync.Mutex
	issuer              *nonSourceWindowIssuer
	transaction         *nonSourceTransactionState
	environment         preallocationEnvironment
	kind                nonSourceCommandKind
	phaseSample         time.Time
	phaseDeadline       time.Time
	callerDeadline      time.Time
	transactionDeadline time.Time
	authorityContext    *nonSourceAuthorityContext
	nullWitness         *retainedNullCommandWitness
	state               nonSourceCommandWindowState
	seal                nonSourceCommandWindowSeal
}

func (issuer *nonSourceWindowIssuer) mintCommand(
	transaction nonSourceTransactionWindow,
	kind nonSourceCommandKind,
	environment preallocationEnvironment,
) (*nonSourceCommandWindow, *FailureRecord) {
	if !transaction.validFor(issuer) || !kind.valid() || !environment.valid() {
		return nil, authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	phaseSample := issuer.scheduler.now()
	if phaseSample.IsZero() {
		return nil, authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := nonSourceTimeBoundaryFailureAt(
		transaction.state.ctx,
		transaction.state.callerDeadlineFloor,
		phaseSample,
		time.Time{},
		transaction.state.transactionDeadline,
		OperationProbe,
	); failure != nil {
		return nil, failure
	}
	callerDeadline, hasCallerDeadline := transaction.state.callerDeadlineFloor.snapshot()
	if hasCallerDeadline && callerDeadline.IsZero() {
		return nil, authorityFailure(OperationProbe, CauseDeadline)
	}
	window := &nonSourceCommandWindow{
		issuer:              issuer,
		transaction:         transaction.state,
		environment:         environment,
		kind:                kind,
		phaseSample:         phaseSample,
		phaseDeadline:       phaseSample.Add(nonSourceCommandDuration),
		callerDeadline:      callerDeadline,
		transactionDeadline: transaction.state.transactionDeadline,
		state:               nonSourceCommandWindowPending,
		seal:                validNonSourceCommandWindow,
	}
	window.authorityContext = newNonSourceAuthorityContext(
		transaction.state,
		window.phaseDeadline,
		window.callerDeadline,
	)
	if window.authorityContext == nil {
		return nil, authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	return window, nil
}

func (window *nonSourceCommandWindow) validLocked() bool {
	return window != nil && window.validIdentityLocked() &&
		window.validTimingLocked() && window.kind.valid() &&
		window.environment.valid()
}

func (window *nonSourceCommandWindow) validIdentityLocked() bool {
	return window.seal == validNonSourceCommandWindow && window.issuer != nil &&
		window.issuer.valid() && window.transaction != nil &&
		window.transaction.seal == validNonSourceTransactionWindow &&
		window.transaction.issuer == window.issuer && window.transaction.ctx != nil &&
		window.issuer.ownsTransaction(window.transaction)
}

func (window *nonSourceCommandWindow) validTimingLocked() bool {
	return !window.phaseSample.IsZero() &&
		window.phaseDeadline.Equal(window.phaseSample.Add(nonSourceCommandDuration)) &&
		window.transaction.callerDeadlineFloor.validSnapshot(
			window.transaction.callerDeadline,
			window.callerDeadline,
		) &&
		window.transactionDeadline.Equal(window.transaction.transactionDeadline) &&
		window.authorityContext != nil &&
		window.authorityContext.validFor(
			window.transaction,
			window.phaseDeadline,
			window.callerDeadline,
		)
}

func (window *nonSourceCommandWindow) admitRetainedNull(
	device *retainedNullDevice,
) *FailureRecord {
	if window == nil {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	window.mu.Lock()
	defer window.mu.Unlock()
	if !window.validLocked() || window.state != nonSourceCommandWindowPending ||
		window.nullWitness != nil ||
		device != window.environment.authorities.nullDevice {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	leaf, err := validateRetainedNullDeviceShape(device)
	if err != nil {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	leafValue := *leaf
	leafValue.pathClaims = append([]authorityPathClaim(nil), leaf.pathClaims...)
	window.nullWitness = &retainedNullCommandWitness{
		window:     window,
		device:     device,
		leaf:       leaf,
		descriptor: leaf.descriptor,
		leafValue:  leafValue,
		seal:       validRetainedNullCommandWitness,
	}
	return nil
}

func (window *nonSourceCommandWindow) authorize() *FailureRecord {
	if window == nil {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	window.mu.Lock()
	defer window.mu.Unlock()
	if !window.validLocked() || window.state != nonSourceCommandWindowPending ||
		window.nullWitness == nil || !window.nullWitness.validShapeLocked(window) {
		window.state = nonSourceCommandWindowAborted
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	window.state = nonSourceCommandWindowAuthorized
	return nil
}

func (window *nonSourceCommandWindow) abort() {
	if window == nil {
		return
	}
	window.mu.Lock()
	defer window.mu.Unlock()
	if window.state == nonSourceCommandWindowPending ||
		window.state == nonSourceCommandWindowAuthorized {
		window.state = nonSourceCommandWindowAborted
	}
}

func (window *nonSourceCommandWindow) consumed() bool {
	if window == nil {
		return false
	}
	window.mu.Lock()
	defer window.mu.Unlock()
	return window.state == nonSourceCommandWindowConsumed
}

func (window *nonSourceCommandWindow) retainProcessCallerDeadline(
	deadline time.Time,
) *FailureRecord {
	if window == nil {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	window.mu.Lock()
	defer window.mu.Unlock()
	if !window.validLocked() || window.state != nonSourceCommandWindowConsumed ||
		(!window.callerDeadline.IsZero() &&
			(deadline.IsZero() || deadline.After(window.callerDeadline))) {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	window.transaction.callerDeadlineFloor.lower(deadline, !deadline.IsZero())
	if !window.transaction.callerDeadlineFloor.validSnapshot(
		window.transaction.callerDeadline,
		window.callerDeadline,
	) {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	return nil
}

func (window *nonSourceCommandWindow) boundaryFailure(
	operation Operation,
) *FailureRecord {
	if window == nil {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	window.mu.Lock()
	defer window.mu.Unlock()
	if !window.validLocked() || window.state == nonSourceCommandWindowAborted {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	return nonSourceTimeBoundaryFailureAt(
		window.transaction.ctx,
		window.transaction.callerDeadlineFloor,
		window.issuer.scheduler.now(),
		window.phaseDeadline,
		window.transactionDeadline,
		operation,
	)
}

type nonSourceCommandAuthority struct {
	ctx                 context.Context
	phaseDeadline       time.Time
	callerDeadline      time.Time
	transactionDeadline time.Time
	nullWitness         *retainedNullCommandWitness
	seal                nonSourceCommandAuthoritySeal
}

func (window *nonSourceCommandWindow) consume(
	kind nonSourceCommandKind,
	environment preallocationEnvironment,
	issuer *nonSourceWindowIssuer,
) (nonSourceCommandAuthority, *FailureRecord) {
	if window == nil {
		return nonSourceCommandAuthority{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	window.mu.Lock()
	defer window.mu.Unlock()
	if !window.validLocked() || window.state != nonSourceCommandWindowAuthorized ||
		window.issuer != issuer || window.kind != kind ||
		window.environment != environment || window.nullWitness == nil ||
		!window.nullWitness.validShapeLocked(window) {
		window.state = nonSourceCommandWindowAborted
		return nonSourceCommandAuthority{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if failure := nonSourceTimeBoundaryFailure(
		window.transaction.ctx,
		window.transaction.callerDeadlineFloor,
		window.issuer.scheduler.now(),
		window.phaseDeadline,
		window.transactionDeadline,
	); failure != nil {
		window.state = nonSourceCommandWindowAborted
		return nonSourceCommandAuthority{}, failure
	}
	callerDeadline, hasCallerDeadline := window.transaction.callerDeadlineFloor.snapshot()
	if hasCallerDeadline && callerDeadline.IsZero() {
		window.state = nonSourceCommandWindowAborted
		return nonSourceCommandAuthority{}, authorityFailure(OperationExecute, CauseDeadline)
	}
	window.state = nonSourceCommandWindowConsumed
	return nonSourceCommandAuthority{
		ctx:                 window.transaction.ctx,
		phaseDeadline:       window.phaseDeadline,
		callerDeadline:      callerDeadline,
		transactionDeadline: window.transactionDeadline,
		nullWitness:         window.nullWitness,
		seal:                validNonSourceCommandAuthority,
	}, nil
}

func nonSourceTimeBoundaryFailure(
	ctx context.Context,
	callerDeadlineFloor *nonSourceCallerDeadlineFloor,
	now, phaseDeadline, transactionDeadline time.Time,
) *FailureRecord {
	return nonSourceTimeBoundaryFailureAt(
		ctx,
		callerDeadlineFloor,
		now,
		phaseDeadline,
		transactionDeadline,
		OperationExecute,
	)
}

func nonSourceTimeBoundaryFailureAt(
	ctx context.Context,
	callerDeadlineFloor *nonSourceCallerDeadlineFloor,
	now, phaseDeadline, transactionDeadline time.Time,
	operation Operation,
) *FailureRecord {
	if ctx == nil || callerDeadlineFloor == nil || now.IsZero() {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	if !validNonSourceBoundaryOperation(operation) {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	contextError := ctx.Err()
	contextDeadline, hasContextDeadline := ctx.Deadline()
	callerDeadline, hasCallerDeadline := callerDeadlineFloor.lower(
		contextDeadline,
		hasContextDeadline,
	)
	if nonSourceDeadlineReached(
		now,
		phaseDeadline,
		transactionDeadline,
		callerDeadline,
		hasCallerDeadline,
		contextError,
	) {
		return authorityFailure(operation, CauseDeadline)
	}
	if contextError != nil {
		cause := contextCause(contextError)
		if cause == CauseInternalInvariant {
			return authorityFailure(OperationValidate, CauseInternalInvariant)
		}
		return authorityFailure(operation, cause)
	}
	return nil
}

func validNonSourceBoundaryOperation(operation Operation) bool {
	return validOperation(operation) && operation != OperationCloseNonRoot &&
		operation != OperationCloseRoot && operation != OperationQuiesce
}

func nonSourceDeadlineReached(
	now, phaseDeadline, transactionDeadline, callerDeadline time.Time,
	hasCallerDeadline bool,
	contextError error,
) bool {
	return (!phaseDeadline.IsZero() && !now.Before(phaseDeadline)) ||
		(!transactionDeadline.IsZero() && !now.Before(transactionDeadline)) ||
		(hasCallerDeadline && !now.Before(callerDeadline)) ||
		errors.Is(contextError, context.DeadlineExceeded)
}

func nonSourceAuthorityBoundaryFailure(
	ctx context.Context,
	operation Operation,
) *FailureRecord {
	if ctx == nil || !validNonSourceBoundaryOperation(operation) {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	if err := ctx.Err(); err != nil {
		cause := contextCause(err)
		if cause == CauseInternalInvariant {
			return authorityFailure(OperationValidate, CauseInternalInvariant)
		}
		return authorityFailure(operation, cause)
	}
	return nil
}

// retainedNullCommandWitness is a non-owning, one-command view of the exact
// descriptor proved by the command's A2 null precheck. It never reopens the
// path and never closes the transaction-owned descriptor.
type retainedNullCommandWitness struct {
	window     *nonSourceCommandWindow
	device     *retainedNullDevice
	leaf       *retainedLeaf
	descriptor *os.File
	leafValue  retainedLeaf
	seal       retainedNullCommandWitnessSeal
}

func (witness *retainedNullCommandWitness) validShapeLocked(
	window *nonSourceCommandWindow,
) bool {
	if witness == nil || witness.seal != validRetainedNullCommandWitness ||
		witness.window != window || witness.device == nil || witness.leaf == nil ||
		witness.descriptor == nil || witness.device.leaf != witness.leaf ||
		witness.leaf.descriptor != witness.descriptor {
		return false
	}
	leaf := witness.leaf
	sealed := witness.leafValue
	return leaf.path == sealed.path && leaf.descriptor == sealed.descriptor &&
		leaf.snapshot == sealed.snapshot && leaf.mount == sealed.mount &&
		leaf.aclDigest == sealed.aclDigest &&
		equalAuthorityPathClaimSlices(leaf.pathClaims, sealed.pathClaims)
}

func (witness *retainedNullCommandWitness) validForProcessInput() bool {
	if witness == nil || witness.window == nil {
		return false
	}
	witness.window.mu.Lock()
	defer witness.window.mu.Unlock()
	return witness.validForProcessInputLocked()
}

func (witness *retainedNullCommandWitness) validForProcessInputLocked() bool {
	return witness != nil && witness.window != nil &&
		witness.window.validLocked() &&
		witness.window.state == nonSourceCommandWindowConsumed &&
		witness.window.nullWitness == witness &&
		witness.validShapeLocked(witness.window)
}

func (witness *retainedNullCommandWitness) borrowProcessInput() (
	retainedNullProcessBorrow,
	bool,
) {
	if witness == nil || witness.window == nil {
		return retainedNullProcessBorrow{}, false
	}
	witness.window.mu.Lock()
	defer witness.window.mu.Unlock()
	if !witness.validForProcessInputLocked() {
		return retainedNullProcessBorrow{}, false
	}
	return retainedNullProcessBorrow{
		device:     witness.device,
		descriptor: witness.descriptor,
	}, true
}

type nonSourceAuthorityOwner interface {
	admitPhysicalRootSentinels(
		context.Context,
		*physicalRootAuthority,
	) (physicalRootSentinelClaim, authorityUseOutcome)
	revalidatePhysicalRoot(context.Context, *physicalRootAuthority) authorityUseOutcome
	revalidateRetainedNull(
		context.Context,
		*physicalRootAuthority,
		*retainedNullDevice,
	) authorityUseOutcome
	revalidateGOROOT(
		context.Context,
		*physicalRootAuthority,
		*treeCapture,
	) authorityUseOutcome
	revalidateGoEnvironmentPlan(context.Context, goEnvironmentPlan) authorityUseOutcome
	revalidateGoAuthority(
		context.Context,
		*physicalRootAuthority,
		*treeCapture,
		*goAuthority,
	) authorityUseOutcome
	revalidateCompilerAuthority(
		context.Context,
		*physicalRootAuthority,
		*treeCapture,
		*compilerAuthority,
	) authorityUseOutcome
	revalidateGitAuthority(
		context.Context,
		*physicalRootAuthority,
		*gitAuthority,
	) authorityUseOutcome
	revalidatePhysicalRootSentinels(
		context.Context,
		physicalRootSentinelClaim,
	) authorityUseOutcome
	privateNonSourceAuthorityOwner()
}

func (*preflightAuthorityRevalidator) privateNonSourceAuthorityOwner() {}

// nonSourceBlock is a transaction-scoped deep module. It receives only the
// nominal authority and five-command surfaces; it has no workspace allocator,
// task path, generic process request, or raw command authority.
type nonSourceBlock struct {
	mu        sync.Mutex
	used      bool
	issuer    *nonSourceWindowIssuer
	authority nonSourceAuthorityOwner
	processes nonSourceProcessOwner
}

//nolint:unused // B seals the production assembly root; C wires it into noScratch.capture.
func newNonSourceBlock(
	ctx context.Context,
) (*nonSourceBlock, nonSourceTransactionWindow, *FailureRecord) {
	issuer := newNonSourceWindowIssuer(&realProcessScheduler{})
	if issuer == nil {
		return nil, nonSourceTransactionWindow{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	// Entry context and the one-hour transaction clock precede every platform
	// probe, including the process supervisor's retained SIGCHLD observation.
	window, failure := issuer.beginTransaction(ctx)
	if failure != nil {
		return nil, nonSourceTransactionWindow{}, failure
	}
	authority, failure := newPreflightAuthorityRevalidator()
	if failure != nil {
		return nil, nonSourceTransactionWindow{}, failure
	}
	processes, failure := newNonSourceProcessOwner(issuer)
	if failure != nil {
		return nil, nonSourceTransactionWindow{}, failure
	}
	block, failure := newNonSourceBlockWith(issuer, authority, processes)
	if failure != nil {
		return nil, nonSourceTransactionWindow{}, failure
	}
	return block, window, nil
}

func newNonSourceBlockWith(
	issuer *nonSourceWindowIssuer,
	authority nonSourceAuthorityOwner,
	processes nonSourceProcessOwner,
) (*nonSourceBlock, *FailureRecord) {
	if issuer == nil || !issuer.valid() || authority == nil || processes == nil {
		return nil, authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	return &nonSourceBlock{
		issuer:    issuer,
		authority: authority,
		processes: processes,
	}, nil
}

func (block *nonSourceBlock) beginTransaction(
	ctx context.Context,
) (nonSourceTransactionWindow, *FailureRecord) {
	if block == nil || block.issuer == nil {
		return nonSourceTransactionWindow{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	return block.issuer.beginTransaction(ctx)
}

func (block *nonSourceBlock) claimRun() bool {
	if block == nil {
		return false
	}
	block.mu.Lock()
	defer block.mu.Unlock()
	if block.used {
		return false
	}
	block.used = true
	return true
}

type nonSourceProof struct {
	plans               nonSourcePlans
	transaction         *nonSourceTransactionState
	goVersion           bool
	goEnvironment       bool
	compilerVersion     bool
	gitVersion          bool
	gitBuiltinInventory bool
	blockEnd            bool
	seal                nonSourceProofSeal
}

func (proof *nonSourceProof) validFor(
	plans nonSourcePlans,
	window nonSourceTransactionWindow,
) bool {
	return proof != nil && proof.seal == validNonSourceProof && plans.valid() &&
		proof.plans == plans && window.state != nil && window.state.issuer != nil &&
		window.validFor(window.state.issuer) &&
		proof.transaction == window.state && proof.goVersion &&
		proof.goEnvironment && proof.compilerVersion && proof.gitVersion &&
		proof.gitBuiltinInventory && proof.blockEnd
}

type nonSourceBlockResult struct {
	proof               *nonSourceProof
	primary             *FailureRecord
	later               *FailureRecord
	descriptorClose     *FailureRecord
	cleanup             *FailureRecord
	processInvariant    bool
	goVersion           bool
	goEnvironment       bool
	compilerVersion     bool
	gitVersion          bool
	gitBuiltinInventory bool
}

func (result *nonSourceBlockResult) failed() bool {
	return result == nil || result.primary != nil || result.later != nil ||
		result.descriptorClose != nil || result.cleanup != nil ||
		result.processInvariant
}

func (result *nonSourceBlockResult) addPrimary(record *FailureRecord) {
	if result == nil || record == nil || result.primary != nil {
		return
	}
	result.primary = cloneFailureRecord(record)
}

func (result *nonSourceBlockResult) addOutputValidationFailure(
	record *FailureRecord,
	child *ChildDiagnostic,
) {
	if result == nil || record == nil {
		return
	}
	if !validStructuredOutputChild(child) || !validNonSourceOutputFailure(record) {
		record = authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	result.addPrimary(nonSourceFailureWithCompletedChild(record, child))
}

func nonSourceFailureWithCompletedChild(
	record *FailureRecord,
	child *ChildDiagnostic,
) *FailureRecord {
	if record == nil {
		return nil
	}
	if !validStructuredOutputChild(child) {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	withChild := cloneFailureRecord(record)
	withChild.Child = cloneChildDiagnostic(child)
	return withChild
}

func validNonSourceOutputFailure(record *FailureRecord) bool {
	if record == nil || record.Phase != PhaseAuthority || record.Child != nil ||
		len(record.Causes) != 1 {
		return false
	}
	switch record.Operation { //nolint:exhaustive // Output validators have a closed result union.
	case OperationValidate:
		return record.Causes[0] == CauseInternalInvariant
	case OperationParse:
		return record.Causes[0] == CauseMalformed
	case OperationCompare:
		return record.Causes[0] == CauseIdentity || record.Causes[0] == CauseUnsupported
	default:
		return false
	}
}

func (result *nonSourceBlockResult) addDescriptorClose(record *FailureRecord) {
	if result == nil || record == nil {
		return
	}
	if result.descriptorClose == nil {
		result.descriptorClose = cloneFailureRecord(record)
		return
	}
	for _, cause := range record.Causes {
		if !slices.Contains(result.descriptorClose.Causes, cause) {
			result.descriptorClose.Causes = append(
				result.descriptorClose.Causes,
				cause,
			)
		}
	}
}

func (result *nonSourceBlockResult) absorbPrecheck(outcome authorityUseOutcome) {
	if result == nil {
		return
	}
	nonCloseValid := validAuthorityNonCloseOutcome(outcome)
	closeValid := validAuthorityDescriptorCloseRecord(outcome.descriptorClose)
	if nonCloseValid {
		result.addPrimary(outcome.primary)
		// A deep precheck may retain a trailing primitive failure after its
		// Primary, but the block's one private later slot is reserved for the
		// first command-end or block-end authority failure.
	} else {
		result.addAuthorityInvariant()
	}
	if !closeValid && nonCloseValid {
		result.addAuthorityInvariant()
	}
	if closeValid {
		result.addDescriptorClose(outcome.descriptorClose)
	}
}

func validAuthorityNonCloseOutcome(outcome authorityUseOutcome) bool {
	if outcome.later != nil && outcome.primary == nil {
		return false
	}
	for _, record := range []*FailureRecord{outcome.primary, outcome.later} {
		if !validAuthorityNonCloseRecord(record) {
			return false
		}
	}
	return true
}

func validAuthorityDescriptorCloseRecord(record *FailureRecord) bool {
	return record == nil ||
		(record.Phase == PhaseClose &&
			record.Operation == OperationCloseNonRoot && record.Child == nil &&
			len(record.Causes) == 1 &&
			record.Causes[0] == CauseDescriptorClose)
}

func validAuthorityNonCloseRecord(record *FailureRecord) bool {
	if record == nil {
		return true
	}
	return record.Phase == PhaseAuthority &&
		record.Child == nil && len(record.Causes) == 1 &&
		validNonSourceAuthorityPair(record.Operation, record.Causes[0])
}

func validNonSourceAuthorityPair(operation Operation, cause CauseCode) bool {
	switch operation { //nolint:exhaustive // The default refuses every non-A2 operation.
	case OperationValidate:
		return slices.Contains([]CauseCode{
			CausePermission,
			CauseUnsupported,
			CauseLimit,
			CauseCanceled,
			CauseDeadline,
			CauseIdentity,
			CauseInternalInvariant,
		}, cause)
	case OperationOpen:
		return slices.Contains([]CauseCode{
			CausePermission,
			CauseUnsupported,
			CauseUnstable,
			CauseCanceled,
			CauseDeadline,
		}, cause)
	case OperationProbe:
		return slices.Contains([]CauseCode{
			CausePermission,
			CauseUnsupported,
			CauseUnstable,
			CauseCanceled,
			CauseDeadline,
		}, cause)
	case OperationWalk:
		return slices.Contains([]CauseCode{
			CausePermission,
			CauseUnstable,
			CauseLimit,
			CauseCanceled,
			CauseDeadline,
		}, cause)
	case OperationParse:
		return slices.Contains([]CauseCode{
			CauseNotFound,
			CausePermission,
			CauseMalformed,
			CauseUnsupported,
			CauseUnstable,
			CauseLimit,
			CauseCanceled,
			CauseDeadline,
		}, cause)
	case OperationHash:
		return slices.Contains([]CauseCode{
			CausePermission,
			CauseUnstable,
			CauseLimit,
			CauseCanceled,
			CauseDeadline,
		}, cause)
	case OperationCompare:
		return slices.Contains([]CauseCode{
			CauseUnsupported,
			CauseUnstable,
			CauseCanceled,
			CauseDeadline,
			CauseIdentity,
		}, cause)
	default:
		return false
	}
}

func (result *nonSourceBlockResult) addLaterAuthority(record *FailureRecord) {
	if result == nil || record == nil || result.later != nil {
		return
	}
	later := cloneFailureRecord(record)
	later.Child = nil
	if len(later.Causes) > 1 {
		later.Causes = later.Causes[:1]
	}
	if later.Phase != PhaseAuthority || len(later.Causes) == 0 ||
		!validNonSourceAuthorityPair(later.Operation, later.Causes[0]) {
		later = authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	result.later = later
}

func (result *nonSourceBlockResult) addAuthorityInvariant() {
	if result == nil {
		return
	}
	invariant := authorityFailure(OperationValidate, CauseInternalInvariant)
	if result.primary == nil {
		result.addPrimary(invariant)
		return
	}
	result.addLaterAuthority(invariant)
}

func (result *nonSourceBlockResult) absorbCommandEndBoundary(
	record *FailureRecord,
) {
	if result == nil || record == nil {
		return
	}
	if result.primary == nil {
		result.addPrimary(record)
		return
	}
	result.addLaterAuthority(record)
}

// beginCommandEndAuthorityCheck and absorbCommandEndAuthorityCheck compose one
// normative check. A failed scheduler boundary is the check's non-close result;
// the still-mandatory owner call may contribute descriptor closure, but its
// observation of that same sticky context stop is not a second later check.
func (result *nonSourceBlockResult) beginCommandEndAuthorityCheck(
	ctx context.Context,
	operation Operation,
) bool {
	record := nonSourceAuthorityBoundaryFailure(ctx, operation)
	result.absorbCommandEndBoundary(record)
	return record != nil
}

func (result *nonSourceBlockResult) absorbCommandEndAuthorityCheck(
	outcome authorityUseOutcome,
	boundaryFailed bool,
) {
	if !boundaryFailed {
		result.absorbPostcheck(outcome)
		return
	}
	nonCloseValid := validAuthorityNonCloseOutcome(outcome)
	closeValid := validAuthorityDescriptorCloseRecord(outcome.descriptorClose)
	if closeValid {
		result.addDescriptorClose(outcome.descriptorClose)
	}
	if !nonCloseValid || !closeValid {
		result.addAuthorityInvariant()
	}
}

func (result *nonSourceBlockResult) beginAuthorityPrecheck(
	ctx context.Context,
	operation Operation,
) bool {
	if result == nil {
		return false
	}
	result.addPrimary(nonSourceAuthorityBoundaryFailure(ctx, operation))
	return !result.failed()
}

func (result *nonSourceBlockResult) absorbPostcheck(outcome authorityUseOutcome) {
	if result == nil {
		return
	}
	nonCloseValid := validAuthorityNonCloseOutcome(outcome)
	closeValid := validAuthorityDescriptorCloseRecord(outcome.descriptorClose)
	closeRecord := outcome.descriptorClose
	if !nonCloseValid {
		outcome = authorityUseOutcome{
			primary: authorityFailure(OperationValidate, CauseInternalInvariant),
		}
	}
	hadPrimary := result.primary != nil
	if outcome.primary != nil {
		if hadPrimary {
			result.addLaterAuthority(outcome.primary)
		} else {
			result.addPrimary(outcome.primary)
			result.addLaterAuthority(outcome.later)
		}
	} else if outcome.later != nil {
		if result.primary == nil {
			result.addPrimary(outcome.later)
		} else {
			result.addLaterAuthority(outcome.later)
		}
	}
	if !closeValid && nonCloseValid {
		result.addAuthorityInvariant()
	}
	if closeValid {
		result.addDescriptorClose(closeRecord)
	}
}

func (result *nonSourceBlockResult) absorbProcess(process processResult) {
	if result == nil {
		return
	}
	primaryValid := validNonSourceProcessPrimaryForAbsorption(process)
	descriptorValid := validProcessDescriptorClose(process.descriptorClose)
	quiescenceValid := validNonSourceProcessQuiescence(process)
	childrenValid := validNonSourceProcessChildShape(process)
	validShape := validNonSourceProcessResult(process)
	if !validShape {
		result.processInvariant = true
	}

	var primary *FailureRecord
	if primaryValid {
		primary = cloneFailureRecord(process.primary)
	}
	var descriptorClose *FailureRecord
	if descriptorValid {
		descriptorClose = cloneFailureRecord(process.descriptorClose)
	}
	quiescence := quiescenceResult{causes: []CauseCode{CauseInternalInvariant}}
	if quiescenceValid {
		quiescence = process.quiescence
		quiescence.causes = append([]CauseCode(nil), process.quiescence.causes...)
		quiescence.child = cloneChildDiagnostic(process.quiescence.child)
	}
	if processChildCount(process) > 1 {
		sanitizeNonSourceProcessChildren(primary, descriptorClose, &quiescence)
	}
	if !childrenValid {
		normalizeNonSourceProcessChildOwner(primary, descriptorClose, &quiescence)
	}
	normalized := processResult{
		primary:         primary,
		descriptorClose: descriptorClose,
		quiescence:      quiescence,
		background:      process.background,
	}
	if !validNonSourceProcessChildShape(normalized) {
		detachNonSourceRecordChild(primary)
		detachNonSourceRecordChild(descriptorClose)
		detachNonSourceQuiescenceChild(&quiescence)
	}
	if !validNonSourceProcessPrimary(primary) {
		primary = nil
	}
	normalized.primary = primary
	normalized.descriptorClose = descriptorClose
	normalized.quiescence = quiescence
	quiescenceValid = validNonSourceProcessQuiescence(normalized)
	result.addPrimary(primary)
	if !validShape {
		result.addPrimary(authorityFailure(OperationValidate, CauseInternalInvariant))
	}
	result.addDescriptorClose(descriptorClose)
	if quiescenceValid && quiescence.proven {
		return
	}
	if !quiescenceValid {
		quiescence = quiescenceResult{
			causes: []CauseCode{CauseInternalInvariant},
		}
	}
	if result.cleanup == nil {
		result.cleanup = &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationQuiesce,
			Causes:    append([]CauseCode(nil), quiescence.causes...),
			Child:     cloneChildDiagnostic(quiescence.child),
		}
	}
}

func validNonSourceProcessResult(process processResult) bool {
	if !validNonSourceProcessPrimary(process.primary) ||
		!validProcessDescriptorClose(process.descriptorClose) ||
		!validNonSourceProcessQuiescence(process) ||
		!validNonSourceProcessChildShape(process) {
		return false
	}
	return validNonSourceProcessOutputShape(process)
}

func validNonSourceProcessQuiescence(process processResult) bool {
	if !validProcessChildDiagnostic(process.quiescence.child) ||
		!validNonSourceBackgroundQuiescence(process) {
		return false
	}
	cleanQuiescence := process.quiescence.proven &&
		len(process.quiescence.causes) == 0 && process.quiescence.child == nil &&
		process.background == nil
	if cleanQuiescence {
		return true
	}
	uncertainQuiescence := !process.quiescence.proven &&
		len(process.quiescence.causes) != 0 &&
		(process.background == nil || process.background.done != nil) &&
		processChildCount(process) == 1
	if !uncertainQuiescence {
		return false
	}
	return validNonSourceCleanupCauses(process.quiescence.causes)
}

func validNonSourceBackgroundQuiescence(process processResult) bool {
	if process.background == nil {
		return true
	}
	return validNonSourceBackgroundCarrier(process)
}

func validNonSourceBackgroundCarrier(process processResult) bool {
	if process.background == nil {
		return false
	}
	return process.background.done != nil &&
		!process.quiescence.proven &&
		validNonSourceCleanupCauses(process.quiescence.causes) &&
		slices.Contains(process.quiescence.causes, CauseChildWait) &&
		!processHasObservedChildStatus(process) &&
		validNonSourceBackgroundPrimary(process.primary)
}

func validNonSourceCleanupCauses(causes []CauseCode) bool {
	if len(causes) == 0 || len(causes) > 5 {
		return false
	}
	var seen uint8
	for _, cause := range causes {
		bit := nonSourceCleanupCauseBit(cause)
		if bit == 0 || seen&bit != 0 {
			return false
		}
		seen |= bit
	}
	return true
}

func nonSourceCleanupCauseBit(cause CauseCode) uint8 {
	switch cause { //nolint:exhaustive // This bitset intentionally covers only B cleanup causes.
	case CauseChildTerminate:
		return 1 << 0
	case CauseChildWait:
		return 1 << 1
	case CauseChildDrain:
		return 1 << 2
	case CauseChildSurvivor:
		return 1 << 3
	case CauseChildProbe:
		return 1 << 4
	default:
		return 0
	}
}

func validNonSourceBackgroundPrimary(record *FailureRecord) bool {
	if record == nil {
		return true
	}
	if record.Operation == OperationValidate {
		return len(record.Causes) == 1 && record.Causes[0] == CauseInternalInvariant
	}
	return record.Operation == OperationExecute && len(record.Causes) == 1 &&
		slices.Contains([]CauseCode{
			CauseCanceled,
			CauseDeadline,
			CauseLimit,
		}, record.Causes[0])
}

func processHasObservedChildStatus(process processResult) bool {
	for _, record := range []*FailureRecord{process.primary, process.descriptorClose} {
		if record != nil && record.Child != nil && record.Child.ExitStatusObserved {
			return true
		}
	}
	return process.quiescence.child != nil &&
		process.quiescence.child.ExitStatusObserved
}

func validNonSourceProcessOutputShape(process processResult) bool {
	if process.structuredOutput == nil {
		return process.structuredOutputChild == nil
	}
	return process.primary == nil && process.descriptorClose == nil &&
		process.quiescence.proven && len(process.quiescence.causes) == 0 &&
		process.quiescence.child == nil && process.background == nil &&
		validStructuredOutputChild(process.structuredOutputChild)
}

func processChildCount(process processResult) int {
	count := 0
	for _, record := range []*FailureRecord{process.primary, process.descriptorClose} {
		if record != nil && record.Child != nil {
			count++
		}
	}
	if process.quiescence.child != nil {
		count++
	}
	return count
}

func validNonSourceProcessChildShape(process processResult) bool {
	count := processChildCount(process)
	if count > 1 {
		return false
	}
	if count == 0 {
		return !nonSourceProcessRequiresChild(process)
	}
	background := validNonSourceBackgroundCarrier(process)
	if process.background != nil && !background {
		return false
	}
	if !background && !processHasObservedChildStatus(process) {
		return false
	}
	switch {
	case process.primary != nil:
		return process.primary.Child != nil &&
			validNonSourcePrimaryChildEvidence(process.primary)
	case process.descriptorClose != nil:
		return process.descriptorClose.Child != nil &&
			(background ||
				validNonSourcePrimarylessChild(process, process.descriptorClose.Child))
	case !process.quiescence.proven:
		return process.quiescence.child != nil &&
			(background ||
				validNonSourcePrimarylessChild(process, process.quiescence.child))
	default:
		return false
	}
}

func validNonSourcePrimarylessChild(
	process processResult,
	child *ChildDiagnostic,
) bool {
	if child == nil || !child.ExitStatusObserved {
		return false
	}
	if child.ExitStatus == 0 {
		return validEmptyProcessDiagnostic(child)
	}
	return !process.quiescence.proven &&
		child.ExitStatus == nonSourceIntentionalKillExit &&
		slices.Contains(process.quiescence.causes, CauseChildDrain)
}

func nonSourceProcessRequiresChild(process processResult) bool {
	if process.background != nil || !process.quiescence.proven {
		return true
	}
	if process.primary == nil {
		return process.descriptorClose != nil
	}
	return nonSourcePrimaryRequiresChild(process.primary)
}

func sanitizeNonSourceProcessChildren(
	primary, descriptorClose *FailureRecord,
	quiescence *quiescenceResult,
) {
	retained := false
	for _, record := range []*FailureRecord{primary, descriptorClose} {
		if record == nil || record.Child == nil {
			continue
		}
		if retained {
			record.Child = nil
			continue
		}
		retained = true
	}
	if quiescence == nil || quiescence.child == nil {
		return
	}
	if retained {
		quiescence.child = nil
	}
}

func normalizeNonSourceProcessChildOwner(
	primary, descriptorClose *FailureRecord,
	quiescence *quiescenceResult,
) {
	child := firstNonSourceProcessChild(
		detachNonSourceRecordChild(primary),
		detachNonSourceRecordChild(descriptorClose),
		detachNonSourceQuiescenceChild(quiescence),
	)
	if child == nil {
		return
	}
	if primary != nil {
		primary.Child = child
		if !validNonSourcePrimaryChildOwner(primary) {
			primary.Child = nil
		}
		return
	}
	if descriptorClose != nil {
		descriptorClose.Child = child
		return
	}
	if quiescence != nil && !quiescence.proven {
		quiescence.child = child
	}
}

func detachNonSourceRecordChild(record *FailureRecord) *ChildDiagnostic {
	if record == nil {
		return nil
	}
	child := record.Child
	record.Child = nil
	return child
}

func detachNonSourceQuiescenceChild(quiescence *quiescenceResult) *ChildDiagnostic {
	if quiescence == nil {
		return nil
	}
	child := quiescence.child
	quiescence.child = nil
	return child
}

func firstNonSourceProcessChild(children ...*ChildDiagnostic) *ChildDiagnostic {
	for _, child := range children {
		if child != nil {
			return child
		}
	}
	return nil
}

func validNonSourceProcessPrimary(record *FailureRecord) bool {
	return validNonSourceProcessPrimaryRecord(record) &&
		validNonSourcePrimaryChildPresence(record)
}

func validNonSourceProcessPrimaryForAbsorption(process processResult) bool {
	record := process.primary
	if !validNonSourceProcessPrimaryRecord(record) {
		return false
	}
	if !nonSourcePrimaryRequiresChild(record) || record.Child != nil {
		return true
	}
	if processChildCount(process) != 1 {
		return false
	}
	if process.descriptorClose != nil && process.descriptorClose.Child != nil {
		return validProcessDescriptorClose(process.descriptorClose)
	}
	if process.quiescence.child != nil {
		return validNonSourceProcessQuiescence(process)
	}
	return false
}

func validNonSourceProcessPrimaryRecord(record *FailureRecord) bool {
	if record == nil {
		return true
	}
	if record.Phase != PhaseAuthority || len(record.Causes) != 1 {
		return false
	}
	return validNonSourceProcessPrimaryPair(record.Operation, record.Causes[0]) &&
		validProcessChildDiagnostic(record.Child) &&
		validNonSourcePrimaryChildOwner(record) &&
		validNonSourcePrimaryChildEvidence(record)
}

func validNonSourcePrimaryChildEvidence(record *FailureRecord) bool {
	if record == nil || record.Child == nil {
		return true
	}
	if len(record.Causes) != 1 {
		return false
	}
	cause := record.Causes[0]
	switch {
	case record.Operation == OperationExecute && cause == CauseChildExit:
		return record.Child.ExitStatusObserved && record.Child.ExitStatus != 0
	case record.Operation == OperationParse && cause == CauseMalformed:
		return record.Child.ExitStatusObserved && record.Child.ExitStatus == 0 &&
			record.Child.DiagnosticBytes > 0
	default:
		return true
	}
}

func validNonSourcePrimaryChildPresence(record *FailureRecord) bool {
	return !nonSourcePrimaryRequiresChild(record) || record.Child != nil
}

func nonSourcePrimaryRequiresChild(record *FailureRecord) bool {
	return record != nil && (record.Operation == OperationParse ||
		record.Operation == OperationExecute && len(record.Causes) == 1 &&
			slices.Contains([]CauseCode{
				CauseChildExit,
				CauseLimit,
			}, record.Causes[0]))
}

func validNonSourcePrimaryChildOwner(record *FailureRecord) bool {
	if record == nil || record.Child == nil {
		return true
	}
	// Validate/internal-invariant can arise after Start when a hostile caller
	// context first returns a non-standard error. Open, Probe, and child-start
	// failures are all definitively pre-Start in the bracketed B process path.
	if record.Operation == OperationOpen || record.Operation == OperationProbe {
		return false
	}
	return record.Operation != OperationExecute ||
		record.Causes[0] != CauseChildStart
}

func validNonSourceProcessPrimaryPair(operation Operation, cause CauseCode) bool {
	// Only the closed B process state machine's emitted pairs are admissible.
	// Every other operation or cause is an adapter invariant violation.
	switch operation { //nolint:exhaustive // The default intentionally refuses every unlisted operation.
	case OperationValidate:
		return cause == CauseInternalInvariant
	case OperationOpen:
		return slices.Contains(
			[]CauseCode{CausePermission, CauseLimit, CauseInternalInvariant},
			cause,
		)
	case OperationProbe:
		return slices.Contains(
			[]CauseCode{CauseUnstable, CauseInternalInvariant},
			cause,
		)
	case OperationExecute:
		return slices.Contains([]CauseCode{
			CauseChildStart,
			CauseChildExit,
			CauseCanceled,
			CauseDeadline,
			CauseLimit,
		}, cause)
	case OperationParse:
		return cause == CauseMalformed
	default:
		return false
	}
}

func validProcessDescriptorClose(record *FailureRecord) bool {
	return record == nil ||
		(record.Phase == PhaseClose && record.Operation == OperationCloseNonRoot &&
			len(record.Causes) == 1 && record.Causes[0] == CauseDescriptorClose &&
			validProcessChildDiagnostic(record.Child))
}

func validProcessChildDiagnostic(child *ChildDiagnostic) bool {
	if child == nil {
		return true
	}
	if child.Truncated != (child.DiagnosticBytes > processDiagnosticPrefixBytes) {
		return false
	}
	if child.DiagnosticBytes == 0 && !validEmptyProcessDiagnostic(child) {
		return false
	}
	if !child.ExitStatusObserved {
		return child.ExitStatus == 0
	}
	if child.ExitStatus >= 0 {
		return child.ExitStatus <= 255
	}
	if child.ExitStatus < -31 {
		return false
	}
	signal := -child.ExitStatus
	return signal >= 1 && signal <= 15 || signal >= 24 && signal <= 27 ||
		signal == 30 || signal == 31
}

func processReleasedNonSourceOutput(process processResult) bool {
	return process.primary == nil && process.descriptorClose == nil &&
		process.quiescence.proven && len(process.quiescence.causes) == 0 &&
		process.quiescence.child == nil && process.background == nil &&
		process.structuredOutput != nil &&
		validStructuredOutputChild(process.structuredOutputChild)
}

func (block *nonSourceBlock) run(
	window nonSourceTransactionWindow,
	plans nonSourcePlans,
) (result nonSourceBlockResult) {
	if !block.claimRun() || block.issuer == nil || block.authority == nil ||
		block.processes == nil || !window.validFor(block.issuer) || !plans.valid() {
		result.addPrimary(authorityFailure(OperationValidate, CauseInternalInvariant))
		return result
	}
	environment := plans.goVersion.environment
	sentinels, admitted := block.admitInitialAuthorities(&result, window, environment)
	if !admitted {
		return result
	}

	defer block.finishRun(&result, window, plans, sentinels)

	block.runGoVersion(
		&result,
		window,
		plans.goVersion,
		plans.goEnvironment,
	)
	block.runGoEnvironment(&result, window, plans.goEnvironment)
	block.runCompilerVersion(&result, window, plans.compilerVersion)
	block.runGitVersion(&result, window, plans.gitVersion)
	block.runGitBuiltinInventory(&result, window, plans.gitBuiltins)
	return result
}

func (block *nonSourceBlock) admitInitialAuthorities(
	result *nonSourceBlockResult,
	window nonSourceTransactionWindow,
	environment preallocationEnvironment,
) (physicalRootSentinelClaim, bool) {
	ctx := window.state.authorityContext
	root := environment.authorities.physicalRoot
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return physicalRootSentinelClaim{}, false
	}
	result.absorbPrecheck(block.authority.revalidatePhysicalRoot(ctx, root))
	if result.failed() {
		return physicalRootSentinelClaim{}, false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return physicalRootSentinelClaim{}, false
	}
	result.absorbPrecheck(block.authority.revalidateRetainedNull(
		ctx,
		root,
		environment.authorities.nullDevice,
	))
	if result.failed() {
		return physicalRootSentinelClaim{}, false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return physicalRootSentinelClaim{}, false
	}
	sentinels, outcome := block.authority.admitPhysicalRootSentinels(ctx, root)
	result.absorbPrecheck(outcome)
	if result.failed() {
		return physicalRootSentinelClaim{}, false
	}
	if sentinels.seal != validPhysicalRootSentinelClaim || sentinels.root != root {
		result.addPrimary(authorityFailure(OperationValidate, CauseInternalInvariant))
		return physicalRootSentinelClaim{}, false
	}
	return sentinels, true
}

func (block *nonSourceBlock) finishRun(
	result *nonSourceBlockResult,
	window nonSourceTransactionWindow,
	plans nonSourcePlans,
	sentinels physicalRootSentinelClaim,
) {
	block.runBlockEnd(result, window, plans.goVersion.environment, sentinels)
	if result.failed() || !result.goVersion || !result.goEnvironment ||
		!result.compilerVersion || !result.gitVersion ||
		!result.gitBuiltinInventory {
		return
	}
	result.proof = &nonSourceProof{
		plans:               plans,
		transaction:         window.state,
		goVersion:           true,
		goEnvironment:       true,
		compilerVersion:     true,
		gitVersion:          true,
		gitBuiltinInventory: true,
		blockEnd:            true,
		seal:                validNonSourceProof,
	}
}

func (block *nonSourceBlock) runGoVersion(
	result *nonSourceBlockResult,
	transaction nonSourceTransactionWindow,
	plan goVersionPlan,
	goEnvironment goEnvironmentPlan,
) {
	if result.failed() {
		return
	}
	window := block.beginCommand(
		result,
		transaction,
		nonSourceCommandGoVersion,
		plan.environment,
	)
	if window == nil {
		return
	}
	defer window.abort()
	if !block.runGoPrechecks(result, window, plan.environment, goEnvironment) {
		return
	}
	process := block.processes.runGoVersion(plan, window)
	result.absorbProcess(process)
	if output, ok := block.releasedCommandOutput(result, window, process); ok {
		result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)
	}
	block.runGoPostchecks(window.authorityContext, result, plan.environment, goEnvironment)
	if block.finishCommand(result, window) {
		result.goVersion = true
	}
}

func (block *nonSourceBlock) runGoEnvironment(
	result *nonSourceBlockResult,
	transaction nonSourceTransactionWindow,
	plan goEnvironmentPlan,
) {
	if result.failed() {
		return
	}
	window := block.beginCommand(
		result,
		transaction,
		nonSourceCommandGoEnvironment,
		plan.environment,
	)
	if window == nil {
		return
	}
	defer window.abort()
	if !block.runGoPrechecks(result, window, plan.environment, plan) {
		return
	}
	process := block.processes.runGoEnvironment(plan, window)
	result.absorbProcess(process)
	if output, ok := block.releasedCommandOutput(result, window, process); ok {
		result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)
	}
	block.runGoPostchecks(window.authorityContext, result, plan.environment, plan)
	if block.finishCommand(result, window) {
		result.goEnvironment = true
	}
}

func (block *nonSourceBlock) runCompilerVersion(
	result *nonSourceBlockResult,
	transaction nonSourceTransactionWindow,
	plan compilerVersionPlan,
) {
	if result.failed() {
		return
	}
	window := block.beginCommand(
		result,
		transaction,
		nonSourceCommandCompilerVersion,
		plan.environment,
	)
	if window == nil {
		return
	}
	defer window.abort()
	if !block.runCompilerPrechecks(result, window, plan.environment) {
		return
	}
	process := block.processes.runCompilerVersion(plan, window)
	result.absorbProcess(process)
	if output, ok := block.releasedCommandOutput(result, window, process); ok {
		result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)
	}
	block.runCompilerPostchecks(window.authorityContext, result, plan.environment)
	if block.finishCommand(result, window) {
		result.compilerVersion = true
	}
}

func (block *nonSourceBlock) runGitVersion(
	result *nonSourceBlockResult,
	transaction nonSourceTransactionWindow,
	plan gitVersionPlan,
) {
	if result.failed() {
		return
	}
	window := block.beginCommand(
		result,
		transaction,
		nonSourceCommandGitVersion,
		plan.environment,
	)
	if window == nil {
		return
	}
	defer window.abort()
	if !block.runGitPrechecks(result, window, plan.environment) {
		return
	}
	process := block.processes.runGitVersion(plan, window)
	result.absorbProcess(process)
	if output, ok := block.releasedCommandOutput(result, window, process); ok {
		result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)
	}
	block.runGitPostchecks(window.authorityContext, result, plan.environment)
	if block.finishCommand(result, window) {
		result.gitVersion = true
	}
}

func (block *nonSourceBlock) runGitBuiltinInventory(
	result *nonSourceBlockResult,
	transaction nonSourceTransactionWindow,
	plan gitBuiltinInventoryPlan,
) {
	if result.failed() {
		return
	}
	window := block.beginCommand(
		result,
		transaction,
		nonSourceCommandGitBuiltinInventory,
		plan.environment,
	)
	if window == nil {
		return
	}
	defer window.abort()
	if !block.runGitPrechecks(result, window, plan.environment) {
		return
	}
	process := block.processes.runGitBuiltinInventory(plan, window)
	result.absorbProcess(process)
	if output, ok := block.releasedCommandOutput(result, window, process); ok {
		result.addOutputValidationFailure(plan.validateOutput(output.bytes), output.child)
	}
	block.runGitPostchecks(window.authorityContext, result, plan.environment)
	if block.finishCommand(result, window) {
		result.gitBuiltinInventory = true
	}
}

func (block *nonSourceBlock) beginCommand(
	result *nonSourceBlockResult,
	transaction nonSourceTransactionWindow,
	kind nonSourceCommandKind,
	environment preallocationEnvironment,
) *nonSourceCommandWindow {
	window, failure := block.issuer.mintCommand(transaction, kind, environment)
	if failure != nil {
		result.addPrimary(failure)
		return nil
	}
	return window
}

func (block *nonSourceBlock) finishCommand(
	result *nonSourceBlockResult,
	window *nonSourceCommandWindow,
) bool {
	result.absorbCommandEndBoundary(nonSourceAuthorityBoundaryFailure(
		window.authorityContext,
		OperationValidate,
	))
	if !window.consumed() {
		result.addPrimary(authorityFailure(OperationValidate, CauseInternalInvariant))
	}
	return !result.failed()
}

func (block *nonSourceBlock) runGoPrechecks(
	result *nonSourceBlockResult,
	window *nonSourceCommandWindow,
	environment preallocationEnvironment,
	goEnvironment goEnvironmentPlan,
) bool {
	ctx := window.authorityContext
	root := environment.authorities.physicalRoot
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidatePhysicalRoot(ctx, root))
	if result.failed() {
		return false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidateRetainedNull(
		ctx,
		root,
		environment.authorities.nullDevice,
	))
	if result.failed() {
		return false
	}
	if failure := window.admitRetainedNull(environment.authorities.nullDevice); failure != nil {
		result.addPrimary(failure)
		return false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidateGOROOT(
		ctx,
		root,
		environment.authorities.goroot,
	))
	if result.failed() {
		return false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidateGoEnvironmentPlan(ctx, goEnvironment))
	if result.failed() {
		return false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationCompare) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidateGoAuthority(
		ctx,
		root,
		environment.authorities.goroot,
		environment.authorities.goExecutable,
	))
	return block.finishPrechecks(result, window)
}

func (block *nonSourceBlock) runCompilerPrechecks(
	result *nonSourceBlockResult,
	window *nonSourceCommandWindow,
	environment preallocationEnvironment,
) bool {
	ctx := window.authorityContext
	root := environment.authorities.physicalRoot
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidatePhysicalRoot(ctx, root))
	if result.failed() {
		return false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidateRetainedNull(
		ctx,
		root,
		environment.authorities.nullDevice,
	))
	if result.failed() {
		return false
	}
	if failure := window.admitRetainedNull(environment.authorities.nullDevice); failure != nil {
		result.addPrimary(failure)
		return false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidateGOROOT(
		ctx,
		root,
		environment.authorities.goroot,
	))
	if result.failed() {
		return false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationCompare) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidateCompilerAuthority(
		ctx,
		root,
		environment.authorities.goroot,
		environment.authorities.compiler,
	))
	return block.finishPrechecks(result, window)
}

func (block *nonSourceBlock) runGitPrechecks(
	result *nonSourceBlockResult,
	window *nonSourceCommandWindow,
	environment preallocationEnvironment,
) bool {
	ctx := window.authorityContext
	root := environment.authorities.physicalRoot
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidatePhysicalRoot(ctx, root))
	if result.failed() {
		return false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidateRetainedNull(
		ctx,
		root,
		environment.authorities.nullDevice,
	))
	if result.failed() {
		return false
	}
	if failure := window.admitRetainedNull(environment.authorities.nullDevice); failure != nil {
		result.addPrimary(failure)
		return false
	}
	if !result.beginAuthorityPrecheck(ctx, OperationProbe) {
		return false
	}
	result.absorbPrecheck(block.authority.revalidateGitAuthority(
		ctx,
		root,
		environment.authorities.gitExecutable,
	))
	return block.finishPrechecks(result, window)
}

func (block *nonSourceBlock) finishPrechecks(
	result *nonSourceBlockResult,
	window *nonSourceCommandWindow,
) bool {
	if result.failed() {
		return false
	}
	if failure := window.boundaryFailure(OperationExecute); failure != nil {
		result.addPrimary(failure)
		return false
	}
	if failure := window.authorize(); failure != nil {
		result.addPrimary(failure)
		return false
	}
	return true
}

type releasedNonSourceCommandOutput struct {
	bytes []byte
	child *ChildDiagnostic
}

func (block *nonSourceBlock) releasedCommandOutput(
	result *nonSourceBlockResult,
	window *nonSourceCommandWindow,
	process processResult,
) (releasedNonSourceCommandOutput, bool) {
	if !processReleasedNonSourceOutput(process) {
		if process.primary == nil && process.descriptorClose == nil &&
			process.quiescence.proven && len(process.quiescence.causes) == 0 &&
			process.quiescence.child == nil && process.background == nil {
			result.addPrimary(authorityFailure(OperationValidate, CauseInternalInvariant))
		}
		return releasedNonSourceCommandOutput{}, false
	}
	if failure := window.boundaryFailure(OperationParse); failure != nil {
		result.addPrimary(nonSourceFailureWithCompletedChild(
			failure,
			process.structuredOutputChild,
		))
		return releasedNonSourceCommandOutput{}, false
	}
	return releasedNonSourceCommandOutput{
		bytes: process.structuredOutput,
		child: cloneChildDiagnostic(process.structuredOutputChild),
	}, true
}

func (block *nonSourceBlock) runGoPostchecks(
	ctx context.Context,
	result *nonSourceBlockResult,
	environment preallocationEnvironment,
	goEnvironment goEnvironmentPlan,
) {
	root := environment.authorities.physicalRoot
	boundaryFailed := result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(
		block.authority.revalidatePhysicalRoot(ctx, root),
		boundaryFailed,
	)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateRetainedNull(
		ctx,
		root,
		environment.authorities.nullDevice,
	), boundaryFailed)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateGOROOT(
		ctx,
		root,
		environment.authorities.goroot,
	), boundaryFailed)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(
		block.authority.revalidateGoEnvironmentPlan(ctx, goEnvironment),
		boundaryFailed,
	)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationCompare)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateGoAuthority(
		ctx,
		root,
		environment.authorities.goroot,
		environment.authorities.goExecutable,
	), boundaryFailed)
}

func (block *nonSourceBlock) runCompilerPostchecks(
	ctx context.Context,
	result *nonSourceBlockResult,
	environment preallocationEnvironment,
) {
	root := environment.authorities.physicalRoot
	boundaryFailed := result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(
		block.authority.revalidatePhysicalRoot(ctx, root),
		boundaryFailed,
	)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateRetainedNull(
		ctx,
		root,
		environment.authorities.nullDevice,
	), boundaryFailed)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateGOROOT(
		ctx,
		root,
		environment.authorities.goroot,
	), boundaryFailed)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationCompare)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateCompilerAuthority(
		ctx,
		root,
		environment.authorities.goroot,
		environment.authorities.compiler,
	), boundaryFailed)
}

func (block *nonSourceBlock) runGitPostchecks(
	ctx context.Context,
	result *nonSourceBlockResult,
	environment preallocationEnvironment,
) {
	root := environment.authorities.physicalRoot
	boundaryFailed := result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(
		block.authority.revalidatePhysicalRoot(ctx, root),
		boundaryFailed,
	)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateRetainedNull(
		ctx,
		root,
		environment.authorities.nullDevice,
	), boundaryFailed)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateGitAuthority(
		ctx,
		root,
		environment.authorities.gitExecutable,
	), boundaryFailed)
}

func (block *nonSourceBlock) runBlockEnd(
	result *nonSourceBlockResult,
	window nonSourceTransactionWindow,
	environment preallocationEnvironment,
	sentinels physicalRootSentinelClaim,
) {
	ctx := window.state.authorityContext
	root := environment.authorities.physicalRoot
	boundaryFailed := result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(
		block.authority.revalidatePhysicalRoot(ctx, root),
		boundaryFailed,
	)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateRetainedNull(
		ctx,
		root,
		environment.authorities.nullDevice,
	), boundaryFailed)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(
		block.authority.revalidatePhysicalRootSentinels(ctx, sentinels),
		boundaryFailed,
	)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationCompare)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateGoAuthority(
		ctx,
		root,
		environment.authorities.goroot,
		environment.authorities.goExecutable,
	), boundaryFailed)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationCompare)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateCompilerAuthority(
		ctx,
		root,
		environment.authorities.goroot,
		environment.authorities.compiler,
	), boundaryFailed)
	boundaryFailed = result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(block.authority.revalidateGitAuthority(
		ctx,
		root,
		environment.authorities.gitExecutable,
	), boundaryFailed)
	result.absorbCommandEndBoundary(nonSourceAuthorityBoundaryFailure(ctx, OperationValidate))
}

package buildauthority

import (
	"bytes"
	"context"
)

const sourceObjectAuxiliaryReadChunkBytes = 64 << 10

// sourceObjectAuxiliaryReadCursor is the closed fixed-buffer read plan shared
// by admission and revalidation. It proves exact coverage without allocating
// a body-sized slice, including at the eight-GiB policy edge.
type sourceObjectAuxiliaryReadCursor struct {
	size   int64
	offset int64
}

func (cursor *sourceObjectAuxiliaryReadCursor) next(
	buffer []byte,
) (int64, []byte, bool) {
	if cursor == nil || cursor.size < 0 || cursor.offset < 0 ||
		cursor.offset >= cursor.size || len(buffer) == 0 {
		return 0, nil, false
	}
	readSize := min(int64(len(buffer)), cursor.size-cursor.offset)
	offset := cursor.offset
	cursor.offset += readSize
	return offset, buffer[:int(readSize)], true
}

func (cursor *sourceObjectAuxiliaryReadCursor) complete() bool {
	return cursor != nil && cursor.size >= 0 && cursor.offset == cursor.size
}

// sourceObjectAuxiliaryClaim is the value-only result of one auxiliary-file
// read. It never retains a descriptor, raw body, reader, callback, or hashing
// capability. Parsed values exist only for the two closed control grammars.
type sourceObjectAuxiliaryClaim struct {
	path         string
	role         sourceObjectPathRole
	bytes        sourceObjectAuxiliaryByteClaim
	values       []string
	blankRecords uint64
}

type sourceObjectAuxiliaryClaimInventory struct {
	rows []sourceObjectAuxiliaryClaim
}

func (builder *sourceConstructionBuilder) retainSourceObjectAuxiliaryClaimInventory() bool {
	if failure := builder.validateSourceObjectAuxiliaryClaimRequest(); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	claims, captured := builder.captureSourceObjectAuxiliaryClaimInventory(
		builder.owner.git.administration,
		builder.owner.objects.paths,
		sourceInitialRequired,
	)
	if !captured {
		return false
	}
	builder.owner.objects.claims = claims
	return true
}

//nolint:gocyclo // This is the closed validate-capture-validate admission transaction.
func (builder *sourceConstructionBuilder) captureSourceObjectAuxiliaryClaimInventory(
	administration *sourceAdministrativeInventory,
	paths *sourceObjectPathInventory,
	presence sourcePresenceMode,
) (*sourceObjectAuxiliaryClaimInventory, bool) {
	if builder == nil || builder.owner == nil || builder.primitives == nil ||
		builder.owner.git == nil || builder.owner.config == nil ||
		builder.owner.objects == nil || administration == nil || paths == nil ||
		(presence != sourceInitialRequired && presence != sourceRevalidatePresent) {
		if builder != nil {
			builder.outcome.addPrimitive(newSourcePrimitiveFailure(
				OperationValidate,
				CauseInternalInvariant,
			))
		}
		return nil, false
	}
	validationOperation := OperationValidate
	if presence == sourceRevalidatePresent {
		validationOperation = OperationCompare
	}
	if failure := sourceContextPrimitiveFailure(
		builder.ctx,
		validationOperation,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return nil, false
	}
	gitEvidence := builder.owner.git.root.evidence
	objectsEvidence := builder.owner.objects.root.evidence
	if presence == sourceRevalidatePresent {
		gitRow, gitPresent := sourceAdministrativeRowByPath(administration.rows, ".git")
		objectsRow, objectsPresent := sourceAdministrativeRowByPath(
			administration.rows,
			".git/objects",
		)
		if !gitPresent || !objectsPresent || gitRow.kind != sourceObservedDirectory ||
			objectsRow.kind != sourceObservedDirectory {
			builder.failInvariant()
			return nil, false
		}
		gitEvidence = gitRow.evidence
		objectsEvidence = objectsRow.evidence
	}
	inputsValid := administration.valid(
		builder.owner.config.claim.objectFormat,
		builder.owner.packedRefs,
		gitEvidence,
		builder.owner.config.evidence,
		objectsEvidence,
	) && paths.valid(builder.owner.config.claim.objectFormat, administration)
	if failure := sourceContextPrimitiveFailure(
		builder.ctx,
		validationOperation,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return nil, false
	}
	if !inputsValid {
		builder.failInvariant()
		return nil, false
	}
	var rootFailure *sourcePrimitiveFailure
	if presence == sourceRevalidatePresent {
		rootFailure = builder.compareSourceDirectoryBeforeWalk(
			builder.owner.objects.root.descriptor,
			builder.owner.objects.root.evidence,
		)
	} else {
		rootFailure = builder.compareSourceDescriptorWithoutPolicy(
			builder.owner.objects.root.descriptor,
			builder.owner.objects.root.evidence,
			sourceObservedDirectory,
		)
	}
	if rootFailure != nil {
		builder.outcome.addPrimitive(rootFailure)
		return nil, false
	}
	claims := &sourceObjectAuxiliaryClaimInventory{
		rows: make([]sourceObjectAuxiliaryClaim, 0, int(paths.auxiliaryFileCount)), //nolint:gosec // The path inventory is bounded by maxRepositoryEntries.
	}
	for rowIndex := range paths.rows {
		row := paths.rows[rowIndex]
		if !row.role.auxiliary() {
			continue
		}
		administrativeRow, present := sourceAdministrativeRowByPath(
			administration.rows,
			row.path,
		)
		if !present {
			builder.failInvariant()
			return nil, false
		}
		claim, failure := builder.captureSourceObjectAuxiliaryClaim(
			row,
			administrativeRow,
			administration,
			presence,
		)
		if failure != nil {
			builder.outcome.addPrimitive(failure)
		}
		if !builder.outcome.proved() {
			return nil, false
		}
		claims.rows = append(claims.rows, claim)
	}
	if failure := validateSourceObjectAuxiliaryClaimInventory(
		builder.ctx,
		builder.owner.config.claim.objectFormat,
		paths,
		claims,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return nil, false
	}
	matchesAdministration, failure := sourceObjectAuxiliaryClaimsMatchAdministrationValidity(
		builder.ctx,
		OperationValidate,
		administration,
		claims,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return nil, false
	}
	if !matchesAdministration {
		builder.failInvariant()
		return nil, false
	}
	return claims, true
}

func (builder *sourceConstructionBuilder) validateSourceObjectAuxiliaryClaimRequest() *sourcePrimitiveFailure {
	if builder == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if builder.primitives == nil || builder.owner == nil ||
		!builder.owner.validObjectPathRetention() || builder.owner.objects.claims != nil {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

//nolint:gocyclo // Keep descriptor acquisition, read, observation, close, and rebind precedence contiguous.
func (builder *sourceConstructionBuilder) captureSourceObjectAuxiliaryClaim(
	row sourceObjectPathRow,
	administrativeRow sourceAdministrativeRow,
	administration *sourceAdministrativeInventory,
	presence sourcePresenceMode,
) (sourceObjectAuxiliaryClaim, *sourcePrimitiveFailure) {
	format := builder.owner.config.claim.objectFormat
	if !row.role.auxiliary() || administrativeRow.path != row.path ||
		administrativeRow.kind != sourceObservedRegular ||
		administrativeRow.class != sourceAuthorityAndManifest ||
		!validSourceAdministrativeRow(administrativeRow, format) || administration == nil ||
		(presence != sourceInitialRequired && presence != sourceRevalidatePresent) {
		return sourceObjectAuxiliaryClaim{}, newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	chainControl := row.role == sourceObjectCommitGraphChainPath ||
		row.role == sourceObjectMultiPackIndexChainPath
	if failure := sourceObjectAuxiliarySizeFailure(
		builder.ctx,
		format,
		administrativeRow.evidence.snapshot.size,
		chainControl,
	); failure != nil {
		return sourceObjectAuxiliaryClaim{}, failure
	}
	capture := newSourceObjectAuxiliaryByteCapture(format, row)
	if capture == nil || capture.hash == nil {
		return sourceObjectAuxiliaryClaim{}, newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	components, ok := splitNormalizedSourcePath(row.path)
	if !ok || len(components) < 3 || components[0] != ".git" ||
		components[1] != "objects" {
		return sourceObjectAuxiliaryClaim{}, newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}

	transient := make([]*ownedSourceDescriptor, 0, len(components)-2)
	parent := builder.owner.objects.root.descriptor
	prefix := ".git/objects"
	var leaf *ownedSourceDescriptor
	var primary *sourcePrimitiveFailure
	for index := 2; index < len(components); index++ {
		prefix += "/" + components[index] //nolint:modernize // Bounded components require each intermediate prefix for sealed-row lookup.
		expected, present := sourceAdministrativeRowByPath(
			administration.rows,
			prefix,
		)
		if !present {
			primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			break
		}
		descriptor, openFailure := builder.primitives.openRelativeNoFollow(
			builder.ctx,
			parent,
			components[index],
			expected.kind,
			presence,
		)
		if descriptor != nil {
			transient = append(transient, descriptor)
		}
		primary = firstSourceObjectAuxiliaryFailure(primary, openFailure)
		if primary != nil {
			break
		}
		if !descriptor.validOpen() {
			primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			break
		}
		if index == len(components)-1 {
			leaf = descriptor
		}
		observation, failure := builder.observeSourceDescriptorWithoutPolicy(
			descriptor,
			expected.kind,
		)
		if failure == nil {
			if presence == sourceRevalidatePresent && index == len(components)-1 {
				failure = compareSourceObjectAuxiliaryPreReadEvidence(
					builder.ctx,
					expected.evidence,
					observation.evidence,
				)
			} else {
				failure = compareSourceDescriptorEvidence(
					builder.ctx,
					expected.evidence,
					observation.evidence,
				)
			}
		}
		primary = firstSourceObjectAuxiliaryFailure(primary, failure)
		if primary != nil {
			break
		}
		parent = descriptor
	}
	leafOpened := leaf != nil && leaf.validOpen()
	var claim sourceObjectAuxiliaryClaim
	if primary == nil && !leafOpened {
		primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if primary == nil {
		size := administrativeRow.evidence.snapshot.size
		bufferSize := min(size, int64(sourceObjectAuxiliaryReadChunkBytes))
		content := make([]byte, int(bufferSize))
		cursor := sourceObjectAuxiliaryReadCursor{size: size}
		for {
			offset, chunk, more := cursor.next(content)
			if !more {
				break
			}
			var failure *sourcePrimitiveFailure
			if capture.parseOwnedRead() {
				failure = builder.primitives.readExactAtForParse(
					builder.ctx,
					leaf,
					offset,
					chunk,
				)
			} else {
				failure = builder.primitives.readExactAtForHash(
					builder.ctx,
					leaf,
					offset,
					chunk,
				)
			}
			if failure == nil {
				failure = capture.consume(builder.ctx, chunk)
			}
			primary = firstSourceObjectAuxiliaryFailure(primary, failure)
			if primary != nil {
				break
			}
		}
		if primary == nil && !cursor.complete() {
			primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
	}
	if primary == nil {
		var failure *sourcePrimitiveFailure
		claim, failure = capture.finish(builder.ctx)
		primary = firstSourceObjectAuxiliaryFailure(primary, failure)
		if primary == nil {
			primary = sourceObjectAuxiliaryImmediateBindingFailure(
				builder.ctx,
				format,
				row,
				claim,
			)
		}
	}
	if leafOpened {
		after, failure := builder.observeSourceDescriptorWithoutPolicy(
			leaf,
			sourceObservedRegular,
		)
		if failure == nil {
			failure = compareSourceDescriptorEvidence(
				builder.ctx,
				administrativeRow.evidence,
				after.evidence,
			)
		}
		primary = firstSourceObjectAuxiliaryFailure(primary, failure)
	}
	builder.closeTransientDescriptors(transient)
	if leafOpened {
		primary = firstSourceObjectAuxiliaryFailure(
			primary,
			builder.rebindSourceObjectAuxiliaryRow(row, administration),
		)
	}
	if primary != nil {
		return sourceObjectAuxiliaryClaim{}, primary
	}
	return claim, nil
}

func sourceObjectAuxiliaryImmediateBindingFailure(
	ctx context.Context,
	format repositoryObjectFormat,
	row sourceObjectPathRow,
	claim sourceObjectAuxiliaryClaim,
) *sourcePrimitiveFailure {
	if row.role != sourceObjectSplitCommitGraphPath &&
		row.role != sourceObjectMultiPackIndexLayerPath {
		return nil
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	var bindingFailure *sourcePrimitiveFailure
	hash, valid := claim.bytes.repositoryHex(format)
	if !valid || hash != row.key {
		bindingFailure = newSourcePrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	return bindingFailure
}

func (builder *sourceConstructionBuilder) rebindSourceObjectAuxiliaryRow(
	row sourceObjectPathRow,
	administration *sourceAdministrativeInventory,
) *sourcePrimitiveFailure {
	components, ok := splitNormalizedSourcePath(row.path)
	if !ok || len(components) < 3 || components[0] != ".git" ||
		components[1] != "objects" {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	transient := make([]*ownedSourceDescriptor, 0, len(components)-2)
	parent := builder.owner.objects.root.descriptor
	prefix := ".git/objects"
	var primary *sourcePrimitiveFailure
	for index := 2; index < len(components); index++ {
		prefix += "/" + components[index] //nolint:modernize // Bounded components require each intermediate prefix for sealed-row lookup.
		expected, present := sourceAdministrativeRowByPath(
			administration.rows,
			prefix,
		)
		if !present {
			primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			break
		}
		descriptor, openFailure := builder.primitives.openRelativeNoFollow(
			builder.ctx,
			parent,
			components[index],
			expected.kind,
			sourceRevalidatePresent,
		)
		if descriptor != nil {
			transient = append(transient, descriptor)
		}
		primary = firstSourceObjectAuxiliaryFailure(primary, openFailure)
		if primary != nil {
			break
		}
		if !descriptor.validOpen() {
			primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			break
		}
		observation, failure := builder.observeSourceDescriptorWithoutPolicy(
			descriptor,
			expected.kind,
		)
		if failure == nil {
			failure = compareSourceDescriptorEvidence(
				builder.ctx,
				expected.evidence,
				observation.evidence,
			)
		}
		primary = firstSourceObjectAuxiliaryFailure(primary, failure)
		if primary != nil {
			break
		}
		parent = descriptor
	}
	builder.closeTransientDescriptors(transient)
	return primary
}

func firstSourceObjectAuxiliaryFailure(
	primary *sourcePrimitiveFailure,
	later *sourcePrimitiveFailure,
) *sourcePrimitiveFailure {
	if primary != nil {
		return primary
	}
	return later
}

func sourceAdministrativeRowByPath(
	rows []sourceAdministrativeRow,
	path string,
) (sourceAdministrativeRow, bool) {
	lower := 0
	upper := len(rows)
	for lower < upper {
		index := lower + (upper-lower)/2
		if bytes.Compare([]byte(rows[index].path), []byte(path)) < 0 {
			lower = index + 1
		} else {
			upper = index
		}
	}
	index := lower
	if index >= len(rows) || rows[index].path != path {
		var absent sourceAdministrativeRow
		return absent, false
	}
	return rows[index], true
}

func sourceObjectAuxiliaryClaimInventoryValid(
	format repositoryObjectFormat,
	paths *sourceObjectPathInventory,
	claims *sourceObjectAuxiliaryClaimInventory,
) bool {
	valid, failure := sourceObjectAuxiliaryClaimInventoryValidity(
		context.Background(),
		OperationValidate,
		format,
		paths,
		claims,
	)
	return valid && failure == nil
}

func sourceObjectAuxiliaryClaimInventoryValidity(
	ctx context.Context,
	operation Operation,
	format repositoryObjectFormat,
	paths *sourceObjectPathInventory,
	claims *sourceObjectAuxiliaryClaimInventory,
) (bool, *sourcePrimitiveFailure) {
	shapeValid, failure := sourceObjectAuxiliaryClaimInventoryShapeValidity(
		ctx,
		operation,
		format,
		paths,
		claims,
	)
	if failure != nil || !shapeValid {
		return shapeValid, failure
	}
	closureFailure := sourceObjectAuxiliaryClosureFailureForContext(
		ctx,
		operation,
		format,
		paths.rows,
		claims.rows,
	)
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return false, failure
	}
	return closureFailure == nil, nil
}

//nolint:gocyclo // The bounded loop is the closed claim-to-path shape proof.
func sourceObjectAuxiliaryClaimInventoryShapeValidity(
	ctx context.Context,
	operation Operation,
	format repositoryObjectFormat,
	paths *sourceObjectPathInventory,
	claims *sourceObjectAuxiliaryClaimInventory,
) (bool, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return false, failure
	}
	baseValid := format.hexWidth() != 0 && paths != nil && claims != nil
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return false, failure
	}
	if !baseValid {
		return false, nil
	}
	pathShapeValid, failure := sourceObjectPathInventoryShapeValidity(
		ctx,
		operation,
		paths,
	)
	if failure != nil {
		return false, failure
	}
	if !pathShapeValid {
		return false, nil
	}
	claimIndex := 0
	for rowIndex := range paths.rows {
		if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
			return false, failure
		}
		row := paths.rows[rowIndex]
		if !row.role.auxiliary() {
			continue
		}
		if claimIndex >= len(claims.rows) {
			return false, sourceContextPrimitiveFailure(ctx, operation)
		}
		claim := claims.rows[claimIndex]
		claimValid, failure := sourceObjectAuxiliaryClaimShapeValidity(
			ctx,
			operation,
			format,
			claim,
		)
		if failure != nil {
			return false, failure
		}
		matches := claim.path == row.path && claim.role == row.role && claimValid
		if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
			return false, failure
		}
		if !matches {
			return false, nil
		}
		claimIndex++
	}
	complete := claimIndex == len(claims.rows) &&
		uint64(claimIndex) == paths.auxiliaryFileCount
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return false, failure
	}
	return complete, nil
}

func validateSourceObjectAuxiliaryClaimInventory(
	ctx context.Context,
	format repositoryObjectFormat,
	paths *sourceObjectPathInventory,
	claims *sourceObjectAuxiliaryClaimInventory,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	shapeValid, failure := sourceObjectAuxiliaryClaimInventoryShapeValidity(
		ctx,
		OperationValidate,
		format,
		paths,
		claims,
	)
	if failure != nil {
		return failure
	}
	if !shapeValid {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	closureFailure := sourceObjectAuxiliaryClosureFailureForContext(
		ctx,
		OperationValidate,
		format,
		paths.rows,
		claims.rows,
	)
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	return closureFailure
}

func sourceObjectAuxiliaryClaimsMatchAdministration(
	administration *sourceAdministrativeInventory,
	claims *sourceObjectAuxiliaryClaimInventory,
) bool {
	matches, failure := sourceObjectAuxiliaryClaimsMatchAdministrationValidity(
		context.Background(),
		OperationValidate,
		administration,
		claims,
	)
	return matches && failure == nil
}

func sourceObjectAuxiliaryClaimsMatchAdministrationValidity(
	ctx context.Context,
	operation Operation,
	administration *sourceAdministrativeInventory,
	claims *sourceObjectAuxiliaryClaimInventory,
) (bool, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return false, failure
	}
	if administration == nil || claims == nil {
		return false, nil
	}
	for index := range claims.rows {
		if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
			return false, failure
		}
		claim := claims.rows[index]
		row, present := sourceAdministrativeRowByPath(administration.rows, claim.path)
		if !present || row.kind != sourceObservedRegular ||
			row.class != sourceAuthorityAndManifest || row.evidence.snapshot.size < 0 ||
			claim.bytes.size != uint64(row.evidence.snapshot.size) {
			if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
				return false, failure
			}
			return false, nil
		}
	}
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return false, failure
	}
	return true, nil
}

//nolint:gocyclo // The exhaustive role switch is the closed claim grammar.
func sourceObjectAuxiliaryClaimShapeValidity(
	ctx context.Context,
	operation Operation,
	format repositoryObjectFormat,
	claim sourceObjectAuxiliaryClaim,
) (bool, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return false, failure
	}
	baseValid := claim.path != "" && claim.role.auxiliary() &&
		claim.bytes.repositoryFormat == format &&
		claim.bytes.size <= maxSourceObjectAuxiliaryBytes
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return false, failure
	}
	if !baseValid {
		return false, nil
	}
	switch claim.role {
	case sourceObjectCommitGraphChainPath, sourceObjectMultiPackIndexChainPath:
		valuesValid, failure := sourceObjectAuxiliaryValuesValidity(
			ctx,
			operation,
			format,
			claim.values,
			1,
			maxSourceObjectAuxiliaryRecords,
		)
		if failure != nil {
			return false, failure
		}
		valueCount := uint64(len(claim.values))
		recordWidth := uint64(format.hexWidth() + 1) //nolint:gosec // The closed object format is exactly SHA-1 or SHA-256.
		valid := !claim.bytes.repositoryHashValid && claim.blankRecords == 0 &&
			valuesValid && claim.bytes.size == valueCount*recordWidth
		return valid, sourceContextPrimitiveFailure(ctx, operation)
	case sourceObjectPackListPath:
		valuesValid, failure := sourceObjectAuxiliaryValuesValidity(
			ctx,
			operation,
			format,
			claim.values,
			0,
			maxSourceObjectPackListRecords,
		)
		if failure != nil {
			return false, failure
		}
		valueCount := uint64(len(claim.values))
		recordWidth := uint64(len("P pack-") + format.hexWidth() + len(".pack\n")) //nolint:gosec // The closed format fixes this small positive width.
		recordCountValid := claim.blankRecords <= maxSourceObjectPackListRecords &&
			valueCount <= maxSourceObjectPackListRecords-claim.blankRecords
		valid := !claim.bytes.repositoryHashValid && valuesValid &&
			recordCountValid &&
			claim.bytes.size == claim.blankRecords+valueCount*recordWidth
		return valid, sourceContextPrimitiveFailure(ctx, operation)
	case sourceObjectCommitGraphPath, sourceObjectSplitCommitGraphPath,
		sourceObjectMultiPackIndexPath, sourceObjectMultiPackIndexLayerPath:
		_, ok := claim.bytes.repositoryHex(format)
		digestWidth := uint64(format.hexWidth() / 2) //nolint:gosec // The closed format fixes this at 20 or 32 bytes.
		valid := ok && len(claim.values) == 0 && claim.blankRecords == 0 &&
			claim.bytes.size >= digestWidth
		return valid, sourceContextPrimitiveFailure(ctx, operation)
	case sourceObjectPackAuxiliaryPath, sourceObjectMultiPackIndexAuxiliaryPath,
		sourceObjectMultiPackIndexLayerAuxiliaryPath:
		valid := !claim.bytes.repositoryHashValid && len(claim.values) == 0 &&
			claim.blankRecords == 0
		return valid, sourceContextPrimitiveFailure(ctx, operation)
	case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
		sourceObjectPackContentPath, sourceObjectPackIndexPath:
		return false, sourceContextPrimitiveFailure(ctx, operation)
	}
	return false, sourceContextPrimitiveFailure(ctx, operation)
}

func sourceObjectAuxiliaryValuesValidity(
	ctx context.Context,
	operation Operation,
	format repositoryObjectFormat,
	values []string,
	minimum uint64,
	maximum uint64,
) (bool, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return false, failure
	}
	if uint64(len(values)) < minimum || uint64(len(values)) > maximum {
		return false, sourceContextPrimitiveFailure(ctx, operation)
	}
	seen := make(map[string]bool, len(values))
	for index := range values {
		if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
			return false, failure
		}
		value := values[index]
		valid := isLowerHexOfWidth(value, format.hexWidth()) && !seen[value]
		if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
			return false, failure
		}
		if !valid {
			return false, nil
		}
		seen[value] = true
	}
	return true, sourceContextPrimitiveFailure(ctx, operation)
}

type sourceObjectAuxiliaryClosureIndex struct {
	packParts               map[string]uint8
	splitCommitGraphs       map[string]bool
	multiPackIndexLayers    map[string]bool
	commitGraph             bool
	commitGraphChain        bool
	multiPackIndex          bool
	multiPackIndexChain     bool
	commitGraphValues       []string
	packListValues          []string
	multiPackIndexValues    []string
	multiPackIndexHash      string
	multiPackIndexHashValid bool
}

//nolint:gocyclo // The exhaustive role passes are the auditable auxiliary closure policy.
func sourceObjectAuxiliaryClosureFailureForContext(
	ctx context.Context,
	operation Operation,
	format repositoryObjectFormat,
	paths []sourceObjectPathRow,
	claims []sourceObjectAuxiliaryClaim,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return failure
	}
	index := sourceObjectAuxiliaryClosureIndex{
		packParts:            make(map[string]uint8),
		splitCommitGraphs:    make(map[string]bool),
		multiPackIndexLayers: make(map[string]bool),
	}
	claimIndex := 0
	for rowIndex := range paths {
		if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
			return failure
		}
		row := paths[rowIndex]
		switch row.role {
		case sourceObjectPackContentPath:
			index.packParts[row.key] |= 1
		case sourceObjectPackIndexPath:
			index.packParts[row.key] |= 2
		case sourceObjectCommitGraphPath:
			index.commitGraph = true
		case sourceObjectSplitCommitGraphPath:
			index.splitCommitGraphs[row.key] = true
		case sourceObjectMultiPackIndexPath:
			index.multiPackIndex = true
		case sourceObjectMultiPackIndexLayerPath:
			index.multiPackIndexLayers[row.key] = true
		case sourceObjectCommitGraphChainPath:
			index.commitGraphChain = true
		case sourceObjectMultiPackIndexChainPath:
			index.multiPackIndexChain = true
		case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
			sourceObjectPackAuxiliaryPath, sourceObjectPackListPath,
			sourceObjectMultiPackIndexAuxiliaryPath,
			sourceObjectMultiPackIndexLayerAuxiliaryPath:
		}
		if !row.role.auxiliary() {
			continue
		}
		if claimIndex >= len(claims) || claims[claimIndex].path != row.path ||
			claims[claimIndex].role != row.role {
			return sourceObjectAuxiliaryFailureAfterContext(
				ctx,
				operation,
				newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant),
			)
		}
		claim := claims[claimIndex]
		claimIndex++
		switch row.role {
		case sourceObjectCommitGraphChainPath:
			index.commitGraphValues = claim.values
		case sourceObjectPackListPath:
			index.packListValues = claim.values
		case sourceObjectMultiPackIndexChainPath:
			index.multiPackIndexValues = claim.values
		case sourceObjectSplitCommitGraphPath, sourceObjectMultiPackIndexLayerPath:
			hash, ok := claim.bytes.repositoryHex(format)
			if !ok || hash != row.key {
				return sourceObjectAuxiliaryFailureAfterContext(
					ctx,
					operation,
					newSourcePrimitiveFailure(OperationCompare, CauseIdentity),
				)
			}
		case sourceObjectMultiPackIndexPath:
			index.multiPackIndexHash, index.multiPackIndexHashValid =
				claim.bytes.repositoryHex(format)
		case sourceObjectCommitGraphPath, sourceObjectPackAuxiliaryPath,
			sourceObjectMultiPackIndexAuxiliaryPath,
			sourceObjectMultiPackIndexLayerAuxiliaryPath:
		case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
			sourceObjectPackContentPath, sourceObjectPackIndexPath:
		}
	}
	if claimIndex != len(claims) {
		return sourceObjectAuxiliaryFailureAfterContext(
			ctx,
			operation,
			newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant),
		)
	}
	if failure := sourceObjectAuxiliaryExactMemberFailureForContext(
		ctx,
		operation,
		index.commitGraphValues,
		index.splitCommitGraphs,
	); failure != nil {
		return failure
	}
	if failure := sourceObjectAuxiliaryExactMemberFailureForContext(
		ctx,
		operation,
		index.multiPackIndexValues,
		index.multiPackIndexLayers,
	); failure != nil {
		return failure
	}
	for valueIndex := range index.packListValues {
		if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
			return failure
		}
		if index.packParts[index.packListValues[valueIndex]] != 3 {
			return sourceObjectAuxiliaryFailureAfterContext(
				ctx,
				operation,
				newSourcePrimitiveFailure(OperationOpen, CauseNotFound),
			)
		}
	}
	if index.commitGraph && index.commitGraphChain {
		return sourceObjectAuxiliaryFailureAfterContext(
			ctx,
			operation,
			newSourcePrimitiveFailure(OperationValidate, CauseUnsupported),
		)
	}
	for rowIndex := range paths {
		if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
			return failure
		}
		row := paths[rowIndex]
		switch row.role {
		case sourceObjectPackContentPath, sourceObjectPackIndexPath,
			sourceObjectPackAuxiliaryPath:
			if index.packParts[row.key] != 3 {
				return sourceObjectAuxiliaryFailureAfterContext(ctx, operation,
					newSourcePrimitiveFailure(OperationValidate, CauseUnsupported))
			}
		case sourceObjectMultiPackIndexAuxiliaryPath:
			if !index.multiPackIndex || !index.multiPackIndexHashValid {
				return sourceObjectAuxiliaryFailureAfterContext(ctx, operation,
					newSourcePrimitiveFailure(OperationValidate, CauseUnsupported))
			}
			if row.key != index.multiPackIndexHash {
				return sourceObjectAuxiliaryFailureAfterContext(ctx, operation,
					newSourcePrimitiveFailure(OperationCompare, CauseIdentity))
			}
		case sourceObjectMultiPackIndexLayerAuxiliaryPath:
			if !index.multiPackIndexChain || !index.multiPackIndexLayers[row.key] {
				return sourceObjectAuxiliaryFailureAfterContext(ctx, operation,
					newSourcePrimitiveFailure(OperationValidate, CauseUnsupported))
			}
		case sourceObjectSplitCommitGraphPath:
			if !index.commitGraphChain {
				return sourceObjectAuxiliaryFailureAfterContext(ctx, operation,
					newSourcePrimitiveFailure(OperationValidate, CauseUnsupported))
			}
		case sourceObjectMultiPackIndexLayerPath:
			if !index.multiPackIndexChain {
				return sourceObjectAuxiliaryFailureAfterContext(ctx, operation,
					newSourcePrimitiveFailure(OperationValidate, CauseUnsupported))
			}
		case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
			sourceObjectCommitGraphPath, sourceObjectCommitGraphChainPath,
			sourceObjectPackListPath, sourceObjectMultiPackIndexPath,
			sourceObjectMultiPackIndexChainPath:
		}
	}
	return sourceContextPrimitiveFailure(ctx, operation)
}

func sourceObjectAuxiliaryExactMemberFailureForContext(
	ctx context.Context,
	operation Operation,
	values []string,
	members map[string]bool,
) *sourcePrimitiveFailure {
	for index := range values {
		if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
			return failure
		}
		if !members[values[index]] {
			return sourceObjectAuxiliaryFailureAfterContext(
				ctx,
				operation,
				newSourcePrimitiveFailure(OperationOpen, CauseNotFound),
			)
		}
	}
	if len(values) != len(members) {
		return sourceObjectAuxiliaryFailureAfterContext(
			ctx,
			operation,
			newSourcePrimitiveFailure(OperationValidate, CauseUnsupported),
		)
	}
	return sourceContextPrimitiveFailure(ctx, operation)
}

func sourceObjectAuxiliaryFailureAfterContext(
	ctx context.Context,
	operation Operation,
	failure *sourcePrimitiveFailure,
) *sourcePrimitiveFailure {
	if contextFailure := sourceContextPrimitiveFailure(ctx, operation); contextFailure != nil {
		return contextFailure
	}
	return failure
}

package buildauthority

import (
	"context"
)

// The byte engine deliberately has no filesystem or descriptor surface. The
// caller feeds bounded chunks obtained through sourcePrimitives and retains
// only the value claims returned by this file.
const (
	maxSourceObjectAuxiliaryRecords = uint64(256)
	maxSourceObjectAuxiliaryBytes   = uint64(8) << 30
	maxSourceObjectPackListRecords  = uint64(1_000_000)
)

type sourceObjectAuxiliaryByteClaim struct {
	size                uint64
	manifest            Digest
	repositoryFormat    repositoryObjectFormat
	repositoryHash      [32]byte
	repositoryHashValid bool
}

type sourceObjectAuxiliaryByteCapture struct {
	path     string
	role     sourceObjectPathRole
	chain    *sourceObjectAuxiliaryChainState
	packList *sourceObjectAuxiliaryPackListState
	hash     *sourceObjectAuxiliaryHashState
}

func newSourceObjectAuxiliaryByteCapture(
	format repositoryObjectFormat,
	row sourceObjectPathRow,
) *sourceObjectAuxiliaryByteCapture {
	capture := &sourceObjectAuxiliaryByteCapture{path: row.path, role: row.role}
	switch row.role {
	case sourceObjectCommitGraphChainPath, sourceObjectMultiPackIndexChainPath:
		capture.chain = newSourceObjectAuxiliaryChainState(format)
		capture.hash = newSourceObjectAuxiliaryHashState(format, false)
	case sourceObjectPackListPath:
		capture.packList = newSourceObjectAuxiliaryPackListState(format)
		capture.hash = newSourceObjectAuxiliaryHashState(format, false)
	case sourceObjectCommitGraphPath, sourceObjectSplitCommitGraphPath,
		sourceObjectMultiPackIndexPath, sourceObjectMultiPackIndexLayerPath:
		capture.hash = newSourceObjectAuxiliaryHashState(format, true)
	case sourceObjectPackAuxiliaryPath, sourceObjectMultiPackIndexAuxiliaryPath,
		sourceObjectMultiPackIndexLayerAuxiliaryPath:
		capture.hash = newSourceObjectAuxiliaryHashState(format, false)
	case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
		sourceObjectPackContentPath, sourceObjectPackIndexPath:
	}
	return capture
}

func (capture *sourceObjectAuxiliaryByteCapture) parseOwnedRead() bool {
	return capture != nil && (capture.chain != nil || capture.packList != nil)
}

func (capture *sourceObjectAuxiliaryByteCapture) consume(
	ctx context.Context,
	content []byte,
) *sourcePrimitiveFailure {
	if capture == nil || capture.hash == nil ||
		(capture.chain != nil && capture.packList != nil) {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if capture.chain != nil {
		if failure := capture.chain.consume(ctx, content); failure != nil {
			return failure
		}
	}
	if capture.packList != nil {
		if failure := capture.packList.consume(ctx, content); failure != nil {
			return failure
		}
	}
	return capture.hash.consume(ctx, content)
}

func (capture *sourceObjectAuxiliaryByteCapture) finish(
	ctx context.Context,
) (sourceObjectAuxiliaryClaim, *sourcePrimitiveFailure) {
	if capture == nil || capture.hash == nil ||
		(capture.chain != nil && capture.packList != nil) {
		return sourceObjectAuxiliaryClaim{}, newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	claim := sourceObjectAuxiliaryClaim{path: capture.path, role: capture.role}
	if capture.chain != nil {
		values, failure := capture.chain.finish(ctx)
		if failure != nil {
			return sourceObjectAuxiliaryClaim{}, failure
		}
		claim.values = values
	}
	if capture.packList != nil {
		values, blankRecords, failure := capture.packList.finish(ctx)
		if failure != nil {
			return sourceObjectAuxiliaryClaim{}, failure
		}
		claim.values = values
		claim.blankRecords = blankRecords
	}
	bytes, failure := capture.hash.finish(ctx)
	if failure != nil {
		return sourceObjectAuxiliaryClaim{}, failure
	}
	claim.bytes = bytes
	return claim, nil
}

// sourceObjectAuxiliarySizeFailure is the single size gate used before any
// grammar or body work. Chain controls own parse/limit; all other auxiliaries
// own walk/limit, including the primary bodies whose bytes are opaque.
func sourceObjectAuxiliarySizeFailure(
	ctx context.Context,
	format repositoryObjectFormat,
	size int64,
	chainControl bool,
) *sourcePrimitiveFailure {
	operation := OperationWalk
	if chainControl {
		operation = OperationParse
	}
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return failure
	}
	width := format.hexWidth()
	var sizeFailure *sourcePrimitiveFailure
	switch {
	case width <= 0 || size < 0:
		sizeFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	case chainControl:
		recordWidth := uint64(width) + 1
		if uint64(size) > maxSourceObjectAuxiliaryRecords*recordWidth {
			sizeFailure = newSourcePrimitiveFailure(OperationParse, CauseLimit)
		}
	case uint64(size) > maxSourceObjectAuxiliaryBytes:
		sizeFailure = newSourcePrimitiveFailure(OperationWalk, CauseLimit)
	}
	if failure := sourceContextPrimitiveFailure(ctx, operation); failure != nil {
		return failure
	}
	return sizeFailure
}

type sourceObjectAuxiliaryChainState struct {
	format  repositoryObjectFormat
	width   int
	total   uint64
	records uint64
	record  [64]byte
	recordN int
	values  []string
	seen    map[string]struct{}
	failure *sourcePrimitiveFailure
}

func newSourceObjectAuxiliaryChainState(
	format repositoryObjectFormat,
) *sourceObjectAuxiliaryChainState {
	width := format.hexWidth()
	if width == 0 {
		return &sourceObjectAuxiliaryChainState{
			format:  format,
			failure: newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant),
		}
	}
	return &sourceObjectAuxiliaryChainState{
		format: format,
		width:  width,
		values: make([]string, 0, maxSourceObjectAuxiliaryRecords),
		seen:   make(map[string]struct{}, maxSourceObjectAuxiliaryRecords),
	}
}

func (state *sourceObjectAuxiliaryChainState) consume(
	ctx context.Context,
	chunk []byte,
) *sourcePrimitiveFailure {
	if state == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if state.failure != nil {
		return state.failure
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		state.failure = failure
		return failure
	}
	if chunk == nil {
		return nil
	}
	if state.width <= 0 {
		state.failure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		return state.failure
	}
	limit := maxSourceObjectAuxiliaryRecords * (uint64(state.width) + 1)
	if uint64(len(chunk)) > limit-state.total {
		state.failure = newSourcePrimitiveFailure(OperationParse, CauseLimit)
		return state.failure
	}
	state.total += uint64(len(chunk))
	for _, value := range chunk {
		if failure := state.consumeByte(value); failure != nil {
			return failure
		}
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		state.failure = failure
		return failure
	}
	return nil
}

func (state *sourceObjectAuxiliaryChainState) consumeByte(
	value byte,
) *sourcePrimitiveFailure {
	if state.recordN == 0 && state.records >= maxSourceObjectAuxiliaryRecords {
		state.failure = newSourcePrimitiveFailure(OperationParse, CauseLimit)
		return state.failure
	}
	if state.recordN < state.width {
		if value < '0' || value > '9' && (value < 'a' || value > 'f') {
			state.failure = newSourcePrimitiveFailure(OperationParse, CauseMalformed)
			return state.failure
		}
		state.record[state.recordN] = value
		state.recordN++
		return nil
	}
	if value != '\n' {
		state.failure = newSourcePrimitiveFailure(OperationParse, CauseMalformed)
		return state.failure
	}
	valueString := string(state.record[:state.width])
	if _, present := state.seen[valueString]; present {
		state.failure = newSourcePrimitiveFailure(OperationParse, CauseMalformed)
		return state.failure
	}
	state.seen[valueString] = struct{}{}
	state.values = append(state.values, valueString)
	state.records++
	state.recordN = 0
	return nil
}

func (state *sourceObjectAuxiliaryChainState) finish(
	ctx context.Context,
) ([]string, *sourcePrimitiveFailure) {
	if state == nil {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if state.failure != nil {
		return nil, state.failure
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return nil, failure
	}
	if state.records == 0 || state.recordN != 0 {
		return nil, newSourcePrimitiveFailure(OperationParse, CauseMalformed)
	}
	values := append([]string(nil), state.values...)
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return nil, failure
	}
	return values, nil
}

type sourceObjectAuxiliaryPackListState struct {
	format  repositoryObjectFormat
	width   int
	total   uint64
	records uint64
	blank   uint64
	line    [96]byte
	lineN   int
	values  []string
	seen    map[string]struct{}
	failure *sourcePrimitiveFailure
}

func newSourceObjectAuxiliaryPackListState(
	format repositoryObjectFormat,
) *sourceObjectAuxiliaryPackListState {
	width := format.hexWidth()
	if width == 0 {
		return &sourceObjectAuxiliaryPackListState{
			format:  format,
			failure: newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant),
		}
	}
	return &sourceObjectAuxiliaryPackListState{
		format: format,
		width:  width,
		values: make([]string, 0),
		seen:   make(map[string]struct{}),
	}
}

func (state *sourceObjectAuxiliaryPackListState) consume(
	ctx context.Context,
	chunk []byte,
) *sourcePrimitiveFailure {
	if state == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if state.failure != nil {
		return state.failure
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		state.failure = failure
		return failure
	}
	if chunk == nil {
		return nil
	}
	if uint64(len(chunk)) > maxSourceObjectAuxiliaryBytes-state.total {
		state.failure = newSourcePrimitiveFailure(OperationWalk, CauseLimit)
		return state.failure
	}
	state.total += uint64(len(chunk))
	for _, value := range chunk {
		if state.lineN == 0 && state.records >= maxSourceObjectPackListRecords {
			state.failure = newSourcePrimitiveFailure(OperationParse, CauseLimit)
			return state.failure
		}
		if value == '\n' {
			if state.lineN == 0 {
				state.blank++
			} else {
				if failure := state.finishLine(); failure != nil {
					return failure
				}
			}
			state.records++
			state.lineN = 0
			continue
		}
		if state.lineN >= len(state.line) {
			state.failure = newSourcePrimitiveFailure(OperationParse, CauseMalformed)
			return state.failure
		}
		state.line[state.lineN] = value
		state.lineN++
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		state.failure = failure
		return failure
	}
	return nil
}

func (state *sourceObjectAuxiliaryPackListState) finishLine() *sourcePrimitiveFailure {
	wantPrefix := "P pack-"
	wantSuffix := ".pack"
	want := len(wantPrefix) + state.width + len(wantSuffix)
	if state.lineN != want {
		state.failure = newSourcePrimitiveFailure(OperationParse, CauseMalformed)
		return state.failure
	}
	for index := range wantPrefix {
		if state.line[index] != wantPrefix[index] {
			state.failure = newSourcePrimitiveFailure(OperationParse, CauseMalformed)
			return state.failure
		}
	}
	start := len(wantPrefix)
	for index := range state.width {
		value := state.line[start+index]
		if value < '0' || value > '9' && (value < 'a' || value > 'f') {
			state.failure = newSourcePrimitiveFailure(OperationParse, CauseMalformed)
			return state.failure
		}
	}
	for index := range wantSuffix {
		if state.line[start+state.width+index] != wantSuffix[index] {
			state.failure = newSourcePrimitiveFailure(OperationParse, CauseMalformed)
			return state.failure
		}
	}
	value := string(state.line[start : start+state.width])
	if _, present := state.seen[value]; present {
		state.failure = newSourcePrimitiveFailure(OperationParse, CauseMalformed)
		return state.failure
	}
	state.seen[value] = struct{}{}
	state.values = append(state.values, value)
	return nil
}

func (state *sourceObjectAuxiliaryPackListState) finish(
	ctx context.Context,
) ([]string, uint64, *sourcePrimitiveFailure) {
	if state == nil {
		return nil, 0, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if state.failure != nil {
		return nil, 0, state.failure
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return nil, 0, failure
	}
	if state.lineN != 0 {
		return nil, 0, newSourcePrimitiveFailure(OperationParse, CauseMalformed)
	}
	values := append([]string(nil), state.values...)
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return nil, 0, failure
	}
	return values, state.blank, nil
}

func parseSourceObjectAuxiliaryPackList(
	ctx context.Context,
	format repositoryObjectFormat,
	content []byte,
) ([]string, uint64, *sourcePrimitiveFailure) {
	state := newSourceObjectAuxiliaryPackListState(format)
	if failure := state.consume(ctx, content); failure != nil {
		return nil, 0, failure
	}
	return state.finish(ctx)
}

type sourceObjectAuxiliaryHashState struct {
	format        repositoryObjectFormat
	width         int
	checksumBound bool
	total         uint64
	manifest      sourceContentDigestState
	repository    sourceContentDigestState
	tail          [32]byte
	tailStart     int
	tailN         int
	body          [4096]byte
	bodyN         int
	failure       *sourcePrimitiveFailure
}

func newSourceObjectAuxiliaryHashState(
	format repositoryObjectFormat,
	checksumBound bool,
) *sourceObjectAuxiliaryHashState {
	manifest, manifestOK := newSourceContentDigestState(sourceContentDigestSHA256)
	state := &sourceObjectAuxiliaryHashState{
		format:        format,
		width:         format.hexWidth() / 2,
		checksumBound: checksumBound,
		manifest:      manifest,
	}
	if format.hexWidth() == 0 || !manifestOK {
		state.failure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		return state
	}
	if checksumBound {
		var repositoryOK bool
		if format == objectFormatSHA1 {
			state.repository, repositoryOK = newSourceContentDigestState(sourceContentDigestSHA1)
		} else {
			state.repository, repositoryOK = newSourceContentDigestState(sourceContentDigestSHA256)
		}
		if !repositoryOK {
			state.failure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
	}
	return state
}

func (state *sourceObjectAuxiliaryHashState) emit(content []byte) bool {
	for len(content) != 0 {
		space := min(len(state.body)-state.bodyN, len(content))
		copy(state.body[state.bodyN:], content[:space])
		state.bodyN += space
		content = content[space:]
		if state.bodyN == len(state.body) {
			if !state.repository.consume(state.body[:]) {
				return false
			}
			state.bodyN = 0
		}
	}
	return true
}

func (state *sourceObjectAuxiliaryHashState) consume(
	ctx context.Context,
	chunk []byte,
) *sourcePrimitiveFailure {
	if state == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if state.failure != nil {
		return state.failure
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		state.failure = failure
		return failure
	}
	if chunk == nil {
		return nil
	}
	if uint64(len(chunk)) > maxSourceObjectAuxiliaryBytes-state.total {
		state.failure = newSourcePrimitiveFailure(OperationWalk, CauseLimit)
		return state.failure
	}
	state.total += uint64(len(chunk))
	if !state.manifest.consume(chunk) {
		state.failure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		return state.failure
	}
	if !state.checksumBound {
		if failure := sourceContextPrimitiveFailure(ctx, OperationHash); failure != nil {
			state.failure = failure
			return failure
		}
		return nil
	}
	for _, value := range chunk {
		if state.tailN < state.width {
			state.tail[(state.tailStart+state.tailN)%state.width] = value
			state.tailN++
			continue
		}
		if !state.emit(state.tail[state.tailStart : state.tailStart+1]) {
			state.failure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			return state.failure
		}
		state.tail[state.tailStart] = value
		state.tailStart = (state.tailStart + 1) % state.width
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		state.failure = failure
		return failure
	}
	return nil
}

func (state *sourceObjectAuxiliaryHashState) finish(
	ctx context.Context,
) (sourceObjectAuxiliaryByteClaim, *sourcePrimitiveFailure) {
	if state == nil {
		return sourceObjectAuxiliaryByteClaim{}, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if state.failure != nil {
		return sourceObjectAuxiliaryByteClaim{}, state.failure
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return sourceObjectAuxiliaryByteClaim{}, failure
	}
	if state.checksumBound {
		if failure := state.verifyRepositoryChecksum(); failure != nil {
			return sourceObjectAuxiliaryByteClaim{}, failure
		}
	}
	manifestRaw, manifestN, manifestOK := state.manifest.sum()
	if !manifestOK || manifestN != len(Digest{}) {
		return sourceObjectAuxiliaryByteClaim{}, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	manifest := Digest(manifestRaw)
	var repositoryHash [32]byte
	if state.checksumBound {
		var repositoryOK bool
		repositoryHash, _, repositoryOK = state.repository.sum()
		if !repositoryOK {
			return sourceObjectAuxiliaryByteClaim{}, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
	}
	claim := sourceObjectAuxiliaryByteClaim{
		size:                state.total,
		manifest:            manifest,
		repositoryFormat:    state.format,
		repositoryHash:      repositoryHash,
		repositoryHashValid: state.checksumBound,
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return sourceObjectAuxiliaryByteClaim{}, failure
	}
	return claim, nil
}

func (state *sourceObjectAuxiliaryHashState) verifyRepositoryChecksum() *sourcePrimitiveFailure {
	if state.tailN < state.width {
		return newSourcePrimitiveFailure(OperationParse, CauseMalformed)
	}
	if state.bodyN != 0 && !state.repository.consume(state.body[:state.bodyN]) {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	raw, rawN, ok := state.repository.sum()
	if !ok || rawN != state.width {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	var expected [32]byte
	for index := range state.width {
		expected[index] = state.tail[(state.tailStart+index)%state.width]
	}
	for index := range state.width {
		if raw[index] != expected[index] {
			return newSourcePrimitiveFailure(OperationCompare, CauseIdentity)
		}
	}
	return nil
}

func (claim sourceObjectAuxiliaryByteClaim) repositoryHex(
	format repositoryObjectFormat,
) (string, bool) {
	width := format.hexWidth() / 2
	if !claim.repositoryHashValid || width == 0 || format != claim.repositoryFormat {
		return "", false
	}
	const digits = "0123456789abcdef"
	encoded := make([]byte, width*2)
	for index := range width {
		encoded[index*2] = digits[claim.repositoryHash[index]>>4]
		encoded[index*2+1] = digits[claim.repositoryHash[index]&0x0f]
	}
	return string(encoded), true
}

package buildauthority

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
)

const preflightAuthorityReadChunk = 64 << 10

func hashPreflightAuthorityDescriptor(
	ctx context.Context,
	reader io.ReaderAt,
	expectedSize int64,
	limit uint64,
) (Digest, *authorityPrimitiveFailure) {
	if reader == nil {
		return Digest{}, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return Digest{}, failure
	}
	if expectedSize < 0 || uint64(expectedSize) > limit {
		return Digest{}, newAuthorityPrimitiveFailure(OperationHash, CauseLimit)
	}
	hasher := sha256.New()
	remaining := expectedSize
	offset := int64(0)
	buffer := make([]byte, preflightAuthorityReadChunk)
	for remaining > 0 {
		if failure := authorityContextPrimitiveFailure(ctx, OperationHash); failure != nil {
			return Digest{}, failure
		}
		length := min(remaining, int64(len(buffer)))
		chunk := buffer[:int(length)]
		read, err := reader.ReadAt(chunk, offset)
		if failure := authorityContextPrimitiveFailure(ctx, OperationHash); failure != nil {
			return Digest{}, failure
		}
		if read != len(chunk) || (err != nil && !errors.Is(err, io.EOF)) {
			if errors.Is(err, fs.ErrPermission) {
				return Digest{}, newAuthorityPrimitiveFailure(OperationHash, CausePermission)
			}
			return Digest{}, newAuthorityPrimitiveFailure(OperationHash, CauseUnstable)
		}
		if _, err := hasher.Write(chunk); err != nil {
			return Digest{}, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		offset += length
		remaining -= length
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return Digest{}, failure
	}
	var digest Digest
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

type preflightAuthorityMachOReader struct {
	ctx     context.Context
	reader  io.ReaderAt
	failure *authorityPrimitiveFailure
}

func (reader *preflightAuthorityMachOReader) ReadAt(
	buffer []byte,
	offset int64,
) (int, error) {
	if reader.failure != nil {
		return 0, io.ErrUnexpectedEOF
	}
	if failure := authorityContextPrimitiveFailure(reader.ctx, OperationParse); failure != nil {
		reader.failure = failure
		return 0, io.ErrUnexpectedEOF
	}
	read, err := reader.reader.ReadAt(buffer, offset)
	if failure := authorityContextPrimitiveFailure(reader.ctx, OperationParse); failure != nil {
		reader.failure = failure
		return read, io.ErrUnexpectedEOF
	}
	if read == len(buffer) && (err == nil || errors.Is(err, io.EOF)) {
		return read, nil
	}
	cause := CauseUnstable
	if errors.Is(err, fs.ErrPermission) {
		cause = CausePermission
	}
	reader.failure = newAuthorityPrimitiveFailure(OperationParse, cause)
	if err == nil {
		err = io.ErrUnexpectedEOF
	}
	return read, err
}

func parsePreflightAuthorityMachO(
	ctx context.Context,
	reader io.ReaderAt,
	size int64,
	profile machOProfile,
) *authorityPrimitiveFailure {
	if reader == nil || (profile != machOProfileGo && profile != machOProfileGit) {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return failure
	}
	tracked := &preflightAuthorityMachOReader{ctx: ctx, reader: reader}
	var err error
	switch profile {
	case machOProfileGo:
		err = validateGoMachO(tracked, size)
	case machOProfileGit:
		err = validateGitMachO(tracked, size)
	}
	if tracked.failure != nil {
		return tracked.failure
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return failure
	}
	if err != nil {
		return newAuthorityPrimitiveFailure(OperationParse, CauseMalformed)
	}
	return nil
}

func hashPreflightAuthorityManifest(
	ctx context.Context,
	domain string,
	root *retainedDirectory,
	entries []entrySnapshot,
) (Digest, *authorityPrimitiveFailure) {
	if domain == "" || root == nil {
		return Digest{}, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return Digest{}, failure
	}
	hasher := sha256.New()
	writeLengthPrefixed(hasher, []byte(manifestFormatDomain))
	writeLengthPrefixed(hasher, []byte(domain))
	writeManifestRecord(hasher, rootManifestRecord(root))
	writeUint64(hasher, uint64(len(entries)))
	for _, entry := range entries {
		if failure := authorityContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
			return Digest{}, failure
		}
		writeManifestRecord(hasher, entryManifestRecord(entry))
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return Digest{}, failure
	}
	var digest Digest
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"testing"
	"time"
)

type preflightPrimitiveReaderAtFunc func([]byte, int64) (int, error)

func (read preflightPrimitiveReaderAtFunc) ReadAt(buffer []byte, offset int64) (int, error) {
	return read(buffer, offset)
}

type stagedPreflightPrimitiveContext struct {
	calls     int
	failAfter int
	err       error
}

func (*stagedPreflightPrimitiveContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (*stagedPreflightPrimitiveContext) Done() <-chan struct{} {
	return nil
}

func (ctx *stagedPreflightPrimitiveContext) Err() error {
	ctx.calls++
	if ctx.calls > ctx.failAfter {
		return ctx.err
	}
	return nil
}

func (*stagedPreflightPrimitiveContext) Value(any) any {
	return nil
}

func TestHashPreflightAuthorityDescriptorExactAndBounded(t *testing.T) {
	content := bytes.Repeat([]byte("authority-descriptor\x00"), 4097)
	want := Digest(sha256.Sum256(content))

	got, failure := hashPreflightAuthorityDescriptor(
		context.Background(),
		bytes.NewReader(content),
		int64(len(content)),
		uint64(len(content)),
	)
	if failure != nil || got != want {
		t.Fatalf("exact hash = %x / %+v, want %x / nil", got, failure, want)
	}

	eofReader := preflightPrimitiveReaderAtFunc(func(buffer []byte, offset int64) (int, error) {
		copy(buffer, content[offset:offset+int64(len(buffer))])
		return len(buffer), io.EOF
	})
	got, failure = hashPreflightAuthorityDescriptor(
		context.Background(),
		eofReader,
		int64(len(content)),
		uint64(len(content)),
	)
	if failure != nil || got != want {
		t.Fatalf("exact EOF hash = %x / %+v, want %x / nil", got, failure, want)
	}

	reads := 0
	empty, failure := hashPreflightAuthorityDescriptor(
		context.Background(),
		preflightPrimitiveReaderAtFunc(func([]byte, int64) (int, error) {
			reads++
			return 0, errors.New("unexpected read")
		}),
		0,
		0,
	)
	wantEmpty := Digest(sha256.Sum256(nil))
	if failure != nil || empty != wantEmpty || reads != 0 {
		t.Fatalf(
			"empty hash = %x / %+v with %d reads, want %x / nil with zero reads",
			empty,
			failure,
			reads,
			wantEmpty,
		)
	}
}

func TestHashPreflightAuthorityDescriptorFailureAttribution(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		reader    io.ReaderAt
		size      int64
		limit     uint64
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "nil context",
			reader:    bytes.NewReader([]byte("a")),
			size:      1,
			limit:     1,
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name:      "nil reader",
			ctx:       context.Background(),
			size:      1,
			limit:     1,
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name:      "negative size",
			ctx:       context.Background(),
			reader:    bytes.NewReader(nil),
			size:      -1,
			limit:     1,
			operation: OperationHash,
			cause:     CauseLimit,
		},
		{
			name:      "over limit",
			ctx:       context.Background(),
			reader:    bytes.NewReader([]byte("ab")),
			size:      2,
			limit:     1,
			operation: OperationHash,
			cause:     CauseLimit,
		},
		{
			name: "short nil error",
			ctx:  context.Background(),
			reader: preflightPrimitiveReaderAtFunc(func(buffer []byte, _ int64) (int, error) {
				return len(buffer) - 1, nil
			}),
			size:      4,
			limit:     4,
			operation: OperationHash,
			cause:     CauseUnstable,
		},
		{
			name: "permission",
			ctx:  context.Background(),
			reader: preflightPrimitiveReaderAtFunc(func([]byte, int64) (int, error) {
				return 0, fs.ErrPermission
			}),
			size:      4,
			limit:     4,
			operation: OperationHash,
			cause:     CausePermission,
		},
		{
			name: "other read error",
			ctx:  context.Background(),
			reader: preflightPrimitiveReaderAtFunc(func([]byte, int64) (int, error) {
				return 0, errors.New("changed during read")
			}),
			size:      4,
			limit:     4,
			operation: OperationHash,
			cause:     CauseUnstable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, failure := hashPreflightAuthorityDescriptor(
				test.ctx,
				test.reader,
				test.size,
				test.limit,
			)
			assertPreflightPrimitiveFailure(
				t,
				failure,
				test.operation,
				test.cause,
			)
		})
	}
}

func TestHashPreflightAuthorityDescriptorContextAttribution(t *testing.T) {
	for _, termination := range []struct {
		name  string
		ctx   context.Context
		cause CauseCode
	}{
		{name: "canceled", ctx: canceledPreflightPrimitiveContext(), cause: CauseCanceled},
		{name: "deadline", ctx: expiredPreflightPrimitiveContext(t), cause: CauseDeadline},
	} {
		t.Run(termination.name+" before read", func(t *testing.T) {
			reads := 0
			_, failure := hashPreflightAuthorityDescriptor(
				termination.ctx,
				preflightPrimitiveReaderAtFunc(func([]byte, int64) (int, error) {
					reads++
					return 0, nil
				}),
				1,
				1,
			)
			assertPreflightPrimitiveFailure(t, failure, OperationHash, termination.cause)
			if reads != 0 {
				t.Fatalf("reader called %d times after terminal context", reads)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, failure := hashPreflightAuthorityDescriptor(
		ctx,
		preflightPrimitiveReaderAtFunc(func(buffer []byte, _ int64) (int, error) {
			copy(buffer, "abcd")
			cancel()
			return len(buffer), nil
		}),
		4,
		4,
	)
	assertPreflightPrimitiveFailure(t, failure, OperationHash, CauseCanceled)

	deadlineDuringRead := &stagedPreflightPrimitiveContext{
		failAfter: 2,
		err:       context.DeadlineExceeded,
	}
	_, failure = hashPreflightAuthorityDescriptor(
		deadlineDuringRead,
		bytes.NewReader([]byte("abcd")),
		4,
		4,
	)
	assertPreflightPrimitiveFailure(t, failure, OperationHash, CauseDeadline)
}

func TestPreflightAuthorityMachOReaderAttribution(t *testing.T) {
	exact := &preflightAuthorityMachOReader{
		ctx: context.Background(),
		reader: preflightPrimitiveReaderAtFunc(func(buffer []byte, _ int64) (int, error) {
			copy(buffer, "abcd")
			return len(buffer), io.EOF
		}),
	}
	buffer := make([]byte, 4)
	read, err := exact.ReadAt(buffer, 0)
	if read != len(buffer) || err != nil || !bytes.Equal(buffer, []byte("abcd")) || exact.failure != nil {
		t.Fatalf("exact reader = %d / %v / %q / %+v", read, err, buffer, exact.failure)
	}

	tests := []struct {
		name   string
		reader io.ReaderAt
		cause  CauseCode
	}{
		{
			name: "short nil error",
			reader: preflightPrimitiveReaderAtFunc(func(buffer []byte, _ int64) (int, error) {
				return len(buffer) - 1, nil
			}),
			cause: CauseUnstable,
		},
		{
			name: "permission",
			reader: preflightPrimitiveReaderAtFunc(func([]byte, int64) (int, error) {
				return 0, fs.ErrPermission
			}),
			cause: CausePermission,
		},
		{
			name: "other error",
			reader: preflightPrimitiveReaderAtFunc(func([]byte, int64) (int, error) {
				return 0, errors.New("changed during parse")
			}),
			cause: CauseUnstable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			tracked := &preflightAuthorityMachOReader{
				ctx: context.Background(),
				reader: preflightPrimitiveReaderAtFunc(func(buffer []byte, offset int64) (int, error) {
					calls++
					return test.reader.ReadAt(buffer, offset)
				}),
			}
			read, err := tracked.ReadAt(make([]byte, 4), 0)
			if err == nil {
				t.Fatal("attributed ReaderAt returned nil error")
			}
			if test.cause == CausePermission && read != 0 {
				t.Fatalf("permission read count = %d, want 0", read)
			}
			assertPreflightPrimitiveFailure(t, tracked.failure, OperationParse, test.cause)

			read, err = tracked.ReadAt(make([]byte, 4), 0)
			if read != 0 || !errors.Is(err, io.ErrUnexpectedEOF) || calls != 1 {
				t.Fatalf("sticky read = %d / %v with %d underlying calls", read, err, calls)
			}
		})
	}
}

func TestPreflightAuthorityMachOReaderContextAttribution(t *testing.T) {
	for _, termination := range []struct {
		name  string
		ctx   context.Context
		cause CauseCode
	}{
		{name: "canceled", ctx: canceledPreflightPrimitiveContext(), cause: CauseCanceled},
		{name: "deadline", ctx: expiredPreflightPrimitiveContext(t), cause: CauseDeadline},
	} {
		t.Run(termination.name+" before read", func(t *testing.T) {
			calls := 0
			tracked := &preflightAuthorityMachOReader{
				ctx: termination.ctx,
				reader: preflightPrimitiveReaderAtFunc(func([]byte, int64) (int, error) {
					calls++
					return 0, nil
				}),
			}
			_, err := tracked.ReadAt(make([]byte, 4), 0)
			if !errors.Is(err, io.ErrUnexpectedEOF) || calls != 0 {
				t.Fatalf("terminal read = %v with %d underlying calls", err, calls)
			}
			assertPreflightPrimitiveFailure(t, tracked.failure, OperationParse, termination.cause)
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	tracked := &preflightAuthorityMachOReader{
		ctx: ctx,
		reader: preflightPrimitiveReaderAtFunc(func(buffer []byte, _ int64) (int, error) {
			copy(buffer, "abcd")
			cancel()
			return len(buffer), nil
		}),
	}
	read, err := tracked.ReadAt(make([]byte, 4), 0)
	if read != 4 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("post-read cancellation = %d / %v", read, err)
	}
	assertPreflightPrimitiveFailure(t, tracked.failure, OperationParse, CauseCanceled)

	deadlineAfterRead := &stagedPreflightPrimitiveContext{
		failAfter: 1,
		err:       context.DeadlineExceeded,
	}
	tracked = &preflightAuthorityMachOReader{
		ctx:    deadlineAfterRead,
		reader: bytes.NewReader([]byte("abcd")),
	}
	read, err = tracked.ReadAt(make([]byte, 4), 0)
	if read != 4 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("post-read deadline = %d / %v", read, err)
	}
	assertPreflightPrimitiveFailure(t, tracked.failure, OperationParse, CauseDeadline)
}

func TestParsePreflightAuthorityMachOValidAndMalformed(t *testing.T) {
	for _, profile := range []machOProfile{machOProfileGo, machOProfileGit} {
		t.Run(profileNameForPreflightPrimitiveTest(profile), func(t *testing.T) {
			image := makeThinMachOFixture(t, profile, baseMachOCommands()...)
			failure := parsePreflightAuthorityMachO(
				context.Background(),
				bytes.NewReader(image),
				int64(len(image)),
				profile,
			)
			if failure != nil {
				t.Fatalf("valid Mach-O parse = %+v", failure)
			}
		})
	}

	for _, test := range []struct {
		name   string
		reader io.ReaderAt
		size   int64
	}{
		{name: "malformed magic", reader: bytes.NewReader(make([]byte, 32)), size: 32},
		{name: "negative size", reader: bytes.NewReader(nil), size: -1},
		{
			name:   "over size limit",
			reader: bytes.NewReader(nil),
			size:   int64(maxExecutableBytes) + 1,
		},
		{
			name: "parser panic",
			reader: preflightPrimitiveReaderAtFunc(func([]byte, int64) (int, error) {
				panic("hostile ReaderAt")
			}),
			size: 32,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := parsePreflightAuthorityMachO(
				context.Background(),
				test.reader,
				test.size,
				machOProfileGo,
			)
			assertPreflightPrimitiveFailure(t, failure, OperationParse, CauseMalformed)
		})
	}
}

func TestParsePreflightAuthorityMachOFailureAttribution(t *testing.T) {
	validImage := makeThinMachOFixture(t, machOProfileGo, baseMachOCommands()...)
	tests := []struct {
		name      string
		ctx       context.Context
		reader    io.ReaderAt
		profile   machOProfile
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "nil context",
			reader:    bytes.NewReader(validImage),
			profile:   machOProfileGo,
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name:      "nil reader",
			ctx:       context.Background(),
			profile:   machOProfileGo,
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name:      "invalid profile",
			ctx:       context.Background(),
			reader:    bytes.NewReader(validImage),
			profile:   machOProfile(0),
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name:    "short read",
			ctx:     context.Background(),
			profile: machOProfileGo,
			reader: preflightPrimitiveReaderAtFunc(func(buffer []byte, offset int64) (int, error) {
				if len(buffer) == 0 {
					return 0, nil
				}
				read, _ := bytes.NewReader(validImage).ReadAt(buffer[:len(buffer)-1], offset)
				return read, nil
			}),
			operation: OperationParse,
			cause:     CauseUnstable,
		},
		{
			name:    "permission",
			ctx:     context.Background(),
			profile: machOProfileGo,
			reader: preflightPrimitiveReaderAtFunc(func([]byte, int64) (int, error) {
				return 0, fs.ErrPermission
			}),
			operation: OperationParse,
			cause:     CausePermission,
		},
		{
			name:      "canceled",
			ctx:       canceledPreflightPrimitiveContext(),
			reader:    bytes.NewReader(validImage),
			profile:   machOProfileGo,
			operation: OperationParse,
			cause:     CauseCanceled,
		},
		{
			name:      "deadline",
			ctx:       expiredPreflightPrimitiveContext(t),
			reader:    bytes.NewReader(validImage),
			profile:   machOProfileGo,
			operation: OperationParse,
			cause:     CauseDeadline,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := parsePreflightAuthorityMachO(
				test.ctx,
				test.reader,
				int64(len(validImage)),
				test.profile,
			)
			assertPreflightPrimitiveFailure(
				t,
				failure,
				test.operation,
				test.cause,
			)
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	failure := parsePreflightAuthorityMachO(
		ctx,
		preflightPrimitiveReaderAtFunc(func(buffer []byte, offset int64) (int, error) {
			read, err := bytes.NewReader(validImage).ReadAt(buffer, offset)
			cancel()
			return read, err
		}),
		int64(len(validImage)),
		machOProfileGo,
	)
	assertPreflightPrimitiveFailure(t, failure, OperationParse, CauseCanceled)
}

func TestHashPreflightAuthorityManifestMatchesCanonicalDigest(t *testing.T) {
	root, entries := manifestFixture(t)
	const domain = "preflight-authority-test/v1"

	got, failure := hashPreflightAuthorityManifest(
		context.Background(),
		domain,
		root,
		entries,
	)
	want := digestTreeManifest(domain, root, entries)
	if failure != nil || got != want {
		t.Fatalf("manifest hash = %x / %+v, want %x / nil", got, failure, want)
	}

	got, failure = hashPreflightAuthorityManifest(
		context.Background(),
		domain,
		root,
		nil,
	)
	want = digestTreeManifest(domain, root, nil)
	if failure != nil || got != want {
		t.Fatalf("empty manifest hash = %x / %+v, want %x / nil", got, failure, want)
	}
}

func TestHashPreflightAuthorityManifestFailureAttribution(t *testing.T) {
	root, entries := manifestFixture(t)
	for _, test := range []struct {
		name      string
		ctx       context.Context
		domain    string
		root      *retainedDirectory
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "nil context",
			domain:    "domain",
			root:      root,
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name:      "empty domain",
			ctx:       context.Background(),
			root:      root,
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name:      "nil root",
			ctx:       context.Background(),
			domain:    "domain",
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name:      "canceled",
			ctx:       canceledPreflightPrimitiveContext(),
			domain:    "domain",
			root:      root,
			operation: OperationWalk,
			cause:     CauseCanceled,
		},
		{
			name:      "deadline",
			ctx:       expiredPreflightPrimitiveContext(t),
			domain:    "domain",
			root:      root,
			operation: OperationWalk,
			cause:     CauseDeadline,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, failure := hashPreflightAuthorityManifest(
				test.ctx,
				test.domain,
				test.root,
				entries,
			)
			assertPreflightPrimitiveFailure(
				t,
				failure,
				test.operation,
				test.cause,
			)
		})
	}

	ctx := &stagedPreflightPrimitiveContext{
		failAfter: 2,
		err:       context.Canceled,
	}
	_, failure := hashPreflightAuthorityManifest(ctx, "domain", root, entries)
	assertPreflightPrimitiveFailure(t, failure, OperationWalk, CauseCanceled)
	if ctx.calls != 3 {
		t.Fatalf("manifest context checks = %d, want cancellation on third check", ctx.calls)
	}

	ctx = &stagedPreflightPrimitiveContext{
		failAfter: 1 + len(entries),
		err:       context.Canceled,
	}
	_, failure = hashPreflightAuthorityManifest(ctx, "domain", root, entries)
	assertPreflightPrimitiveFailure(t, failure, OperationHash, CauseCanceled)
	if ctx.calls != 2+len(entries) {
		t.Fatalf(
			"manifest context checks = %d, want cancellation at hash boundary",
			ctx.calls,
		)
	}
}

func assertPreflightPrimitiveFailure(
	t *testing.T,
	failure *authorityPrimitiveFailure,
	wantOperation Operation,
	wantCause CauseCode,
) {
	t.Helper()
	if failure == nil {
		t.Fatalf("failure = nil, want %s/%s", wantOperation, wantCause)
	}
	if failure.operation != wantOperation || failure.cause != wantCause {
		t.Fatalf(
			"failure = %s/%s, want %s/%s",
			failure.operation,
			failure.cause,
			wantOperation,
			wantCause,
		)
	}
}

func canceledPreflightPrimitiveContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func expiredPreflightPrimitiveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
	t.Cleanup(cancel)
	return ctx
}

func profileNameForPreflightPrimitiveTest(profile machOProfile) string {
	switch profile {
	case machOProfileGo:
		return "go"
	case machOProfileGit:
		return "git"
	default:
		return "invalid"
	}
}

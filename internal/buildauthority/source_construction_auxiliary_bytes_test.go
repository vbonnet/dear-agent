package buildauthority

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const (
	auxiliaryP0SHA1   = "278ba08e67b94297af0cbad4802a99d78b6edc1d"
	auxiliaryP1SHA1   = "6c7878392d192da73498d35962384110cfe28dad"
	auxiliaryP0SHA256 = "eb4a9d5d2425f9c3df774098d2a4f0d8cb3b39b70bbf104a5e00331d8634f0bb"
	auxiliaryP1SHA256 = "560846af25c6cc921054ea227ced3adc0c2e31157176e04e423eabd9375656fa"
)

func requireAuxiliaryFailure(
	t *testing.T,
	failure *sourcePrimitiveFailure,
	wantOperation Operation,
	wantCause CauseCode,
) {
	t.Helper()
	if failure == nil || failure.operation != wantOperation || failure.cause != wantCause {
		t.Fatalf("failure = %+v, want %v/%v", failure, wantOperation, wantCause)
	}
}

func TestSourceObjectAuxiliaryChainGrammar(t *testing.T) {
	for _, test := range []struct {
		name   string
		format repositoryObjectFormat
		body   string
		want   []string
		op     Operation
		cause  CauseCode
	}{
		{name: "sha1 p0", format: objectFormatSHA1, body: auxiliaryP0SHA1 + "\n", want: []string{auxiliaryP0SHA1}},
		{name: "sha1 p0 p1", format: objectFormatSHA1, body: auxiliaryP0SHA1 + "\n" + auxiliaryP1SHA1 + "\n", want: []string{auxiliaryP0SHA1, auxiliaryP1SHA1}},
		{name: "sha256 p0", format: objectFormatSHA256, body: auxiliaryP0SHA256 + "\n", want: []string{auxiliaryP0SHA256}},
		{name: "sha256 p0 p1", format: objectFormatSHA256, body: auxiliaryP0SHA256 + "\n" + auxiliaryP1SHA256 + "\n", want: []string{auxiliaryP0SHA256, auxiliaryP1SHA256}},
		{name: "sha1 width 39", format: objectFormatSHA1, body: strings.Repeat("a", 39) + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "sha1 width 41", format: objectFormatSHA1, body: strings.Repeat("a", 41) + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "sha256 width 63", format: objectFormatSHA256, body: strings.Repeat("a", 63) + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "sha256 width 65", format: objectFormatSHA256, body: strings.Repeat("a", 65) + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "sha1 rejects sha256 width", format: objectFormatSHA1, body: auxiliaryP0SHA256 + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "sha256 rejects sha1 width", format: objectFormatSHA256, body: auxiliaryP0SHA1 + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "uppercase", format: objectFormatSHA1, body: strings.ToUpper(auxiliaryP0SHA1) + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "nonhexadecimal", format: objectFormatSHA1, body: "g" + auxiliaryP0SHA1[1:] + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "crlf", format: objectFormatSHA1, body: auxiliaryP0SHA1 + "\r\n", op: OperationParse, cause: CauseMalformed},
		{name: "blank", format: objectFormatSHA1, body: "\n", op: OperationParse, cause: CauseMalformed},
		{name: "leading padding", format: objectFormatSHA1, body: " " + auxiliaryP0SHA1 + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "trailing padding", format: objectFormatSHA1, body: auxiliaryP0SHA1 + " \n", op: OperationParse, cause: CauseMalformed},
		{name: "nul", format: objectFormatSHA1, body: "\x00" + auxiliaryP0SHA1[1:] + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "empty", format: objectFormatSHA1, body: "", op: OperationParse, cause: CauseMalformed},
		{name: "missing final lf", format: objectFormatSHA1, body: auxiliaryP0SHA1, op: OperationParse, cause: CauseMalformed},
		{name: "double lf", format: objectFormatSHA1, body: auxiliaryP0SHA1 + "\n\n", op: OperationParse, cause: CauseMalformed},
		{name: "trailing byte", format: objectFormatSHA1, body: auxiliaryP0SHA1 + "\nx", op: OperationParse, cause: CauseMalformed},
		{name: "sha1 duplicate", format: objectFormatSHA1, body: auxiliaryP0SHA1 + "\n" + auxiliaryP0SHA1 + "\n", op: OperationParse, cause: CauseMalformed},
		{name: "sha256 duplicate", format: objectFormatSHA256, body: auxiliaryP0SHA256 + "\n" + auxiliaryP0SHA256 + "\n", op: OperationParse, cause: CauseMalformed},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newSourceObjectAuxiliaryChainState(test.format)
			var failure *sourcePrimitiveFailure
			for start := 0; start < len(test.body); start += 7 {
				end := min(start+7, len(test.body))
				if failure = state.consume(context.Background(), []byte(test.body[start:end])); failure != nil {
					break
				}
			}
			var got []string
			if failure == nil {
				got, failure = state.finish(context.Background())
			}
			if test.cause != "" {
				requireAuxiliaryFailure(t, failure, test.op, test.cause)
				return
			}
			if failure != nil || len(got) != len(test.want) {
				t.Fatalf("chain = %q / %+v, want %q", got, failure, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("chain[%d] = %q, want %q", index, got[index], test.want[index])
				}
			}
		})
	}
}

func TestSourceObjectAuxiliaryPackListGrammar(t *testing.T) {
	for _, test := range []struct {
		name   string
		format repositoryObjectFormat
		body   string
		want   []string
		blank  uint64
		cause  CauseCode
	}{
		{name: "empty", format: objectFormatSHA1},
		{name: "blank", format: objectFormatSHA1, body: "\n", blank: 1},
		{name: "sha1 named", format: objectFormatSHA1, body: "P pack-" + auxiliaryP0SHA1 + ".pack\n", want: []string{auxiliaryP0SHA1}},
		{name: "sha256 named", format: objectFormatSHA256, body: "P pack-" + auxiliaryP0SHA256 + ".pack\n", want: []string{auxiliaryP0SHA256}},
		{name: "wrong prefix", format: objectFormatSHA1, body: "X pack-" + auxiliaryP0SHA1 + ".pack\n", cause: CauseMalformed},
		{name: "wrong suffix", format: objectFormatSHA1, body: "P pack-" + auxiliaryP0SHA1 + ".idx\n", cause: CauseMalformed},
		{name: "sha1 width 39", format: objectFormatSHA1, body: "P pack-" + strings.Repeat("a", 39) + ".pack\n", cause: CauseMalformed},
		{name: "sha1 width 41", format: objectFormatSHA1, body: "P pack-" + strings.Repeat("a", 41) + ".pack\n", cause: CauseMalformed},
		{name: "sha256 width 63", format: objectFormatSHA256, body: "P pack-" + strings.Repeat("a", 63) + ".pack\n", cause: CauseMalformed},
		{name: "sha256 width 65", format: objectFormatSHA256, body: "P pack-" + strings.Repeat("a", 65) + ".pack\n", cause: CauseMalformed},
		{name: "sha1 rejects sha256 width", format: objectFormatSHA1, body: "P pack-" + auxiliaryP0SHA256 + ".pack\n", cause: CauseMalformed},
		{name: "sha256 rejects sha1 width", format: objectFormatSHA256, body: "P pack-" + auxiliaryP0SHA1 + ".pack\n", cause: CauseMalformed},
		{name: "uppercase", format: objectFormatSHA1, body: "P pack-" + strings.ToUpper(auxiliaryP0SHA1) + ".pack\n", cause: CauseMalformed},
		{name: "nonhexadecimal", format: objectFormatSHA1, body: "P pack-g" + auxiliaryP0SHA1[1:] + ".pack\n", cause: CauseMalformed},
		{name: "crlf", format: objectFormatSHA1, body: "P pack-" + auxiliaryP0SHA1 + ".pack\r\n", cause: CauseMalformed},
		{name: "padded", format: objectFormatSHA1, body: "P pack-" + auxiliaryP0SHA1 + ".pack \n", cause: CauseMalformed},
		{name: "nul", format: objectFormatSHA1, body: "P pack-\x00" + auxiliaryP0SHA1[1:] + ".pack\n", cause: CauseMalformed},
		{name: "sha1 duplicate", format: objectFormatSHA1, body: "P pack-" + auxiliaryP0SHA1 + ".pack\nP pack-" + auxiliaryP0SHA1 + ".pack\n", cause: CauseMalformed},
		{name: "sha256 duplicate", format: objectFormatSHA256, body: "P pack-" + auxiliaryP0SHA256 + ".pack\nP pack-" + auxiliaryP0SHA256 + ".pack\n", cause: CauseMalformed},
		{name: "missing final lf", format: objectFormatSHA1, body: "P pack-" + auxiliaryP0SHA1 + ".pack", cause: CauseMalformed},
		{name: "trailing byte", format: objectFormatSHA1, body: "P pack-" + auxiliaryP0SHA1 + ".pack\nx", cause: CauseMalformed},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newSourceObjectAuxiliaryPackListState(test.format)
			var failure *sourcePrimitiveFailure
			for start := 0; start < len(test.body); start += 5 {
				end := min(start+5, len(test.body))
				if failure = state.consume(context.Background(), []byte(test.body[start:end])); failure != nil {
					break
				}
			}
			var got []string
			var blank uint64
			if failure == nil {
				got, blank, failure = state.finish(context.Background())
			}
			if test.cause != "" {
				requireAuxiliaryFailure(t, failure, OperationParse, test.cause)
				return
			}
			if failure != nil || blank != test.blank || len(got) != len(test.want) {
				t.Fatalf("pack list = %q blank=%d / %+v, want %q blank=%d", got, blank, failure, test.want, test.blank)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("pack list[%d] = %q, want %q", index, got[index], test.want[index])
				}
			}
		})
	}
}

func sourceObjectAuxiliaryChainFixture(format repositoryObjectFormat, records int) string {
	var content strings.Builder
	for index := range records {
		_, _ = fmt.Fprintf(&content, "%0*x\n", format.hexWidth(), index)
	}
	return content.String()
}

func TestSourceObjectAuxiliaryBounds(t *testing.T) {
	for _, test := range []struct {
		name   string
		format repositoryObjectFormat
		size   int64
		chain  bool
		op     Operation
	}{
		{name: "sha1 exact chain", format: objectFormatSHA1, size: int64(41 * 256), chain: true},
		{name: "sha1 over chain", format: objectFormatSHA1, size: int64(41*256 + 1), chain: true, op: OperationParse},
		{name: "sha256 exact chain", format: objectFormatSHA256, size: int64(65 * 256), chain: true},
		{name: "sha256 over chain", format: objectFormatSHA256, size: int64(65*256 + 1), chain: true, op: OperationParse},
		{name: "opaque exact", format: objectFormatSHA1, size: int64(maxSourceObjectAuxiliaryBytes)},
		{name: "opaque one over", format: objectFormatSHA1, size: int64(maxSourceObjectAuxiliaryBytes) + 1, op: OperationWalk},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := sourceObjectAuxiliarySizeFailure(
				context.Background(),
				test.format,
				test.size,
				test.chain,
			)
			if test.op == "" {
				if failure != nil {
					t.Fatalf("size failure = %+v", failure)
				}
				return
			}
			requireAuxiliaryFailure(t, failure, test.op, CauseLimit)
		})
	}

	for _, format := range []repositoryObjectFormat{objectFormatSHA1, objectFormatSHA256} {
		t.Run(fmt.Sprintf("format-%d 256 chain records", format), func(t *testing.T) {
			state := newSourceObjectAuxiliaryChainState(format)
			content := sourceObjectAuxiliaryChainFixture(format, int(maxSourceObjectAuxiliaryRecords))
			if failure := state.consume(context.Background(), []byte(content)); failure != nil {
				t.Fatalf("exact chain consume = %+v", failure)
			}
			values, failure := state.finish(context.Background())
			if failure != nil || len(values) != int(maxSourceObjectAuxiliaryRecords) {
				t.Fatalf("exact chain finish = %d / %+v", len(values), failure)
			}
			failure = state.consume(context.Background(), []byte{'Z'})
			requireAuxiliaryFailure(t, failure, OperationParse, CauseLimit)
		})
	}

	packList := newSourceObjectAuxiliaryPackListState(objectFormatSHA1)
	exactRecords := []byte(strings.Repeat("\n", int(maxSourceObjectPackListRecords)))
	if failure := packList.consume(context.Background(), exactRecords); failure != nil {
		t.Fatalf("exact pack-list records = %+v", failure)
	}
	values, blanks, failure := packList.finish(context.Background())
	if failure != nil || len(values) != 0 || blanks != maxSourceObjectPackListRecords {
		t.Fatalf("exact pack-list finish = %d/%d/%+v", len(values), blanks, failure)
	}
	failure = packList.consume(context.Background(), []byte{'X'})
	requireAuxiliaryFailure(t, failure, OperationParse, CauseLimit)

	t.Run("exact opaque fixed-buffer read plan", func(t *testing.T) {
		size := int64(maxSourceObjectAuxiliaryBytes)
		if failure := sourceObjectAuxiliarySizeFailure(
			context.Background(),
			objectFormatSHA1,
			size,
			false,
		); failure != nil {
			t.Fatalf("exact opaque size gate = %+v", failure)
		}
		cursor := sourceObjectAuxiliaryReadCursor{size: size}
		buffer := make([]byte, sourceObjectAuxiliaryReadChunkBytes)
		var total int64
		var windows int64
		for {
			offset, chunk, more := cursor.next(buffer)
			if !more {
				break
			}
			if offset != total || len(chunk) == 0 || len(chunk) > len(buffer) {
				t.Fatalf("opaque read window = offset %d size %d after %d", offset, len(chunk), total)
			}
			total += int64(len(chunk))
			windows++
		}
		if !cursor.complete() || total != size ||
			windows != size/int64(sourceObjectAuxiliaryReadChunkBytes) {
			t.Fatalf("opaque read plan = complete %v total %d windows %d", cursor.complete(), total, windows)
		}
		failure := sourceObjectAuxiliarySizeFailure(
			context.Background(),
			objectFormatSHA1,
			size+1,
			false,
		)
		requireAuxiliaryFailure(t, failure, OperationWalk, CauseLimit)
	})
}

func TestSourceObjectAuxiliaryChecksumBinding(t *testing.T) {
	for _, test := range []struct {
		name    string
		format  repositoryObjectFormat
		body    []byte
		trailer []byte
		want    string
	}{
		{name: "sha1 p0", format: objectFormatSHA1, body: []byte("fixture-body\n"), trailer: func() []byte { value := sha1.Sum([]byte("fixture-body\n")); return value[:] }(), want: auxiliaryP0SHA1},
		{name: "sha1 p1", format: objectFormatSHA1, body: []byte("fixture-body-2\n"), trailer: func() []byte { value := sha1.Sum([]byte("fixture-body-2\n")); return value[:] }(), want: auxiliaryP1SHA1},
		{name: "sha256 p0", format: objectFormatSHA256, body: []byte("fixture-body\n"), trailer: func() []byte { value := sha256.Sum256([]byte("fixture-body\n")); return value[:] }(), want: auxiliaryP0SHA256},
		{name: "sha256 p1", format: objectFormatSHA256, body: []byte("fixture-body-2\n"), trailer: func() []byte { value := sha256.Sum256([]byte("fixture-body-2\n")); return value[:] }(), want: auxiliaryP1SHA256},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newSourceObjectAuxiliaryHashState(test.format, true)
			content := append(append([]byte(nil), test.body...), test.trailer...)
			for start := 0; start < len(content); start += 3 {
				end := min(start+3, len(content))
				if failure := state.consume(context.Background(), content[start:end]); failure != nil {
					t.Fatalf("consume = %+v", failure)
				}
			}
			claim, failure := state.finish(context.Background())
			if failure != nil {
				t.Fatalf("finish = %+v", failure)
			}
			if got, ok := claim.repositoryHex(test.format); !ok || got != test.want {
				t.Fatalf("repository hash = %q/%v, want %q", got, ok, test.want)
			}
			otherFormat := objectFormatSHA1
			if test.format == objectFormatSHA1 {
				otherFormat = objectFormatSHA256
			}
			if got, ok := claim.repositoryHex(otherFormat); ok || got != "" {
				t.Fatalf("repository hash accepted mismatched format as %q/%v", got, ok)
			}
			wantManifest := Digest(sha256.Sum256(content))
			if claim.manifest != wantManifest {
				t.Fatalf("manifest = %x, want %x", claim.manifest, wantManifest)
			}

			for _, roleTest := range []struct {
				name string
				row  sourceObjectPathRow
			}{
				{
					name: "monolithic commit graph",
					row: sourceObjectPathRow{
						path: ".git/objects/info/commit-graph",
						role: sourceObjectCommitGraphPath,
					},
				},
				{
					name: "monolithic multi-pack index",
					row: sourceObjectPathRow{
						path: ".git/objects/pack/multi-pack-index",
						role: sourceObjectMultiPackIndexPath,
					},
				},
				{
					name: "split commit graph",
					row: sourceObjectPathRow{
						path: ".git/objects/info/commit-graphs/graph-" + test.want + ".graph",
						role: sourceObjectSplitCommitGraphPath,
						key:  test.want,
					},
				},
				{
					name: "multi-pack index layer",
					row: sourceObjectPathRow{
						path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + test.want + ".midx",
						role: sourceObjectMultiPackIndexLayerPath,
						key:  test.want,
					},
				},
			} {
				t.Run(roleTest.name, func(t *testing.T) {
					capture := newSourceObjectAuxiliaryByteCapture(test.format, roleTest.row)
					for start := 0; start < len(content); start += 3 {
						end := min(start+3, len(content))
						if failure := capture.consume(
							context.Background(),
							content[start:end],
						); failure != nil {
							t.Fatalf("role capture consume = %+v", failure)
						}
					}
					roleClaim, failure := capture.finish(context.Background())
					if failure != nil {
						t.Fatalf("role capture finish = %+v", failure)
					}
					if roleClaim.path != roleTest.row.path || roleClaim.role != roleTest.row.role ||
						roleClaim.bytes.manifest != wantManifest {
						t.Fatalf("role claim = %+v, want row %+v and manifest %x", roleClaim, roleTest.row, wantManifest)
					}
					if got, ok := roleClaim.bytes.repositoryHex(test.format); !ok || got != test.want {
						t.Fatalf("role repository hash = %q/%v, want %q", got, ok, test.want)
					}
					validateSourceObjectAuxiliaryPrimaryRoleFixture(
						t,
						test.format,
						roleTest.row,
						test.want,
						roleClaim,
					)
				})
			}
		})
	}

	for _, test := range []struct {
		name   string
		format repositoryObjectFormat
		body   []byte
		row    sourceObjectPathRow
	}{
		{
			name:   "sha1 wrong trailer",
			format: objectFormatSHA1,
			body:   append([]byte("fixture-body\n"), make([]byte, sha1.Size)...),
			row: sourceObjectPathRow{
				path: ".git/objects/info/commit-graph",
				role: sourceObjectCommitGraphPath,
			},
		},
		{
			name:   "sha256 wrong trailer",
			format: objectFormatSHA256,
			body:   append([]byte("fixture-body\n"), make([]byte, sha256.Size)...),
			row: sourceObjectPathRow{
				path: ".git/objects/pack/multi-pack-index",
				role: sourceObjectMultiPackIndexPath,
			},
		},
		{
			name:   "sha1 too short",
			format: objectFormatSHA1,
			body:   make([]byte, sha1.Size-1),
			row: sourceObjectPathRow{
				path: ".git/objects/info/commit-graphs/graph-" + auxiliaryP0SHA1 + ".graph",
				role: sourceObjectSplitCommitGraphPath,
				key:  auxiliaryP0SHA1,
			},
		},
		{
			name:   "sha256 too short",
			format: objectFormatSHA256,
			body:   make([]byte, sha256.Size-1),
			row: sourceObjectPathRow{
				path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + auxiliaryP0SHA256 + ".midx",
				role: sourceObjectMultiPackIndexLayerPath,
				key:  auxiliaryP0SHA256,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := newSourceObjectAuxiliaryByteCapture(test.format, test.row)
			if failure := capture.consume(context.Background(), test.body); failure != nil {
				t.Fatalf("invalid primary consume = %+v", failure)
			}
			_, failure := capture.finish(context.Background())
			wantOperation := OperationCompare
			wantCause := CauseIdentity
			if strings.Contains(test.name, "too short") {
				wantOperation = OperationParse
				wantCause = CauseMalformed
			}
			requireAuxiliaryFailure(t, failure, wantOperation, wantCause)
		})
	}

	testSourceObjectAuxiliaryChecksumClosure(t)
}

func validateSourceObjectAuxiliaryPrimaryRoleFixture(
	t *testing.T,
	format repositoryObjectFormat,
	row sourceObjectPathRow,
	hash string,
	claim sourceObjectAuxiliaryClaim,
) {
	t.Helper()
	rows := []sourceObjectPathRow{row}
	values := make(map[string][]string)
	switch row.role {
	case sourceObjectSplitCommitGraphPath:
		chain := sourceObjectPathRow{
			path: ".git/objects/info/commit-graphs/commit-graph-chain",
			role: sourceObjectCommitGraphChainPath,
		}
		rows = append(rows, chain)
		values[chain.path] = []string{hash}
	case sourceObjectMultiPackIndexLayerPath:
		chain := sourceObjectPathRow{
			path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-chain",
			role: sourceObjectMultiPackIndexChainPath,
		}
		rows = append(rows, chain)
		values[chain.path] = []string{hash}
	case sourceObjectCommitGraphPath, sourceObjectMultiPackIndexPath:
	case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
		sourceObjectPackContentPath, sourceObjectPackIndexPath,
		sourceObjectCommitGraphChainPath, sourceObjectPackAuxiliaryPath,
		sourceObjectPackListPath, sourceObjectMultiPackIndexChainPath,
		sourceObjectMultiPackIndexAuxiliaryPath,
		sourceObjectMultiPackIndexLayerAuxiliaryPath:
		t.Fatalf("non-primary role reached checksum fixture: %d", row.role)
	}
	paths := sourceObjectAuxiliaryTestPaths(rows)
	claims := sourceObjectAuxiliaryTestClaims(t, format, paths, values, nil)
	replaced := false
	for index := range claims.rows {
		if claims.rows[index].path == claim.path {
			claims.rows[index] = claim
			replaced = true
			break
		}
	}
	if !replaced {
		t.Fatalf("checksum-bound claim %q was absent from its role inventory", claim.path)
	}
	if failure := validateSourceObjectAuxiliaryClaimInventory(
		context.Background(),
		format,
		paths,
		claims,
	); failure != nil {
		t.Fatalf("checksum-bound role closure = %+v", failure)
	}
}

func TestSourceObjectAuxiliaryOpaqueBodyPolicy(t *testing.T) {
	for _, test := range []struct {
		name string
		row  sourceObjectPathRow
		body []byte
	}{
		{
			name: "empty rev",
			row: sourceObjectPathRow{
				path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".rev",
				role: sourceObjectPackAuxiliaryPath,
				key:  auxiliaryP0SHA1,
			},
		},
		{
			name: "arbitrary bitmap",
			row: sourceObjectPathRow{
				path: ".git/objects/pack/multi-pack-index-" + auxiliaryP0SHA1 + ".bitmap",
				role: sourceObjectMultiPackIndexAuxiliaryPath,
				key:  auxiliaryP0SHA1,
			},
			body: []byte("not a Git bitmap"),
		},
		{
			name: "arbitrary keep",
			row: sourceObjectPathRow{
				path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".keep",
				role: sourceObjectPackAuxiliaryPath,
				key:  auxiliaryP0SHA1,
			},
			body: []byte("arbitrary keep body"),
		},
		{
			name: "arbitrary mtimes",
			row: sourceObjectPathRow{
				path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".mtimes",
				role: sourceObjectPackAuxiliaryPath,
				key:  auxiliaryP0SHA1,
			},
			body: []byte("\x00\xffnot decoded"),
		},
		{
			name: "p0 without header decoding",
			row: sourceObjectPathRow{
				path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".rev",
				role: sourceObjectPackAuxiliaryPath,
				key:  auxiliaryP0SHA1,
			},
			body: []byte("fixture-body\n"),
		},
		{
			name: "p1 without chunk decoding",
			row: sourceObjectPathRow{
				path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".rev",
				role: sourceObjectPackAuxiliaryPath,
				key:  auxiliaryP0SHA1,
			},
			body: []byte("fixture-body-2\n"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			classified, ok := classifySourceObjectPathRow(sourceAdministrativeRow{
				path:  test.row.path,
				kind:  sourceObservedRegular,
				class: sourceAuthorityAndManifest,
			}, objectFormatSHA1)
			if !ok || classified != test.row {
				t.Fatalf("opaque pathname classification = %+v/%v, want %+v", classified, ok, test.row)
			}
			capture := newSourceObjectAuxiliaryByteCapture(objectFormatSHA1, test.row)
			if failure := capture.consume(context.Background(), test.body); failure != nil {
				t.Fatalf("opaque consume = %+v", failure)
			}
			claim, failure := capture.finish(context.Background())
			if failure != nil {
				t.Fatalf("opaque finish = %+v", failure)
			}
			want := Digest(sha256.Sum256(test.body))
			if claim.path != test.row.path || claim.role != test.row.role ||
				claim.bytes.manifest != want || claim.bytes.repositoryHash != ([32]byte{}) ||
				len(claim.values) != 0 || claim.blankRecords != 0 {
				t.Fatalf("opaque claim = %+v, want manifest %x and no repository hash", claim, want)
			}
			if got, ok := claim.bytes.repositoryHex(objectFormatSHA1); ok || got != "" {
				t.Fatalf("opaque repository hash = %q/%v, want unavailable", got, ok)
			}

			rows := []sourceObjectPathRow{test.row}
			switch test.row.role {
			case sourceObjectPackAuxiliaryPath:
				rows = append(rows,
					sourceObjectPathRow{
						path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".pack",
						role: sourceObjectPackContentPath,
						key:  auxiliaryP0SHA1,
					},
					sourceObjectPathRow{
						path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".idx",
						role: sourceObjectPackIndexPath,
						key:  auxiliaryP0SHA1,
					},
				)
			case sourceObjectMultiPackIndexAuxiliaryPath:
				rows = append(rows, sourceObjectPathRow{
					path: ".git/objects/pack/multi-pack-index",
					role: sourceObjectMultiPackIndexPath,
				})
			case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
				sourceObjectPackContentPath, sourceObjectPackIndexPath,
				sourceObjectCommitGraphPath, sourceObjectCommitGraphChainPath,
				sourceObjectSplitCommitGraphPath, sourceObjectPackListPath,
				sourceObjectMultiPackIndexPath, sourceObjectMultiPackIndexChainPath,
				sourceObjectMultiPackIndexLayerPath,
				sourceObjectMultiPackIndexLayerAuxiliaryPath:
				t.Fatalf("opaque fixture used unsupported role %d", test.row.role)
			}
			paths := sourceObjectAuxiliaryTestPaths(rows)
			claims := sourceObjectAuxiliaryTestClaims(t, objectFormatSHA1, paths, nil, nil)
			replaced := false
			for index := range claims.rows {
				if claims.rows[index].path == claim.path {
					claims.rows[index] = claim
					replaced = true
					break
				}
			}
			if !replaced {
				t.Fatalf("opaque claim %q was absent from the name-bound inventory", claim.path)
			}
			if failure := validateSourceObjectAuxiliaryClaimInventory(
				context.Background(),
				objectFormatSHA1,
				paths,
				claims,
			); failure != nil {
				t.Fatalf("name-bound opaque closure = %+v", failure)
			}
		})
	}

	claimType := reflect.TypeFor[sourceObjectAuxiliaryClaim]()
	for field := range claimType.Fields() {
		if field.Type.Kind() == reflect.Slice && field.Type.Elem().Kind() == reflect.Uint8 {
			t.Fatalf("opaque claim retains raw body field %s", field.Name)
		}
	}
}

func TestSourceObjectAuxiliaryContextAttribution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	chain := newSourceObjectAuxiliaryChainState(objectFormatSHA1)
	requireAuxiliaryFailure(t, chain.consume(ctx, nil), OperationParse, CauseCanceled)
	hashState := newSourceObjectAuxiliaryHashState(objectFormatSHA1, false)
	requireAuxiliaryFailure(t, hashState.consume(ctx, nil), OperationHash, CauseCanceled)
}

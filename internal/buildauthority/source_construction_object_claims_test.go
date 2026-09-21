package buildauthority

import (
	"context"
	"encoding/hex"
	"sort"
	"strings"
	"testing"
)

func TestSourceObjectAuxiliaryClosure(t *testing.T) {
	format := objectFormatSHA1
	graphChain := sourceObjectPathRow{
		path: ".git/objects/info/commit-graphs/commit-graph-chain",
		role: sourceObjectCommitGraphChainPath,
	}
	graphP0 := sourceObjectPathRow{
		path: ".git/objects/info/commit-graphs/graph-" + auxiliaryP0SHA1 + ".graph",
		role: sourceObjectSplitCommitGraphPath,
		key:  auxiliaryP0SHA1,
	}
	graphP1 := sourceObjectPathRow{
		path: ".git/objects/info/commit-graphs/graph-" + auxiliaryP1SHA1 + ".graph",
		role: sourceObjectSplitCommitGraphPath,
		key:  auxiliaryP1SHA1,
	}
	monolithicGraph := sourceObjectPathRow{
		path: ".git/objects/info/commit-graph",
		role: sourceObjectCommitGraphPath,
	}
	packList := sourceObjectPathRow{
		path: ".git/objects/info/packs",
		role: sourceObjectPackListPath,
	}
	packP0 := sourceObjectPathRow{
		path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".pack",
		role: sourceObjectPackContentPath,
		key:  auxiliaryP0SHA1,
	}
	indexP0 := sourceObjectPathRow{
		path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".idx",
		role: sourceObjectPackIndexPath,
		key:  auxiliaryP0SHA1,
	}
	packP1 := sourceObjectPathRow{
		path: ".git/objects/pack/pack-" + auxiliaryP1SHA1 + ".pack",
		role: sourceObjectPackContentPath,
		key:  auxiliaryP1SHA1,
	}
	indexP1 := sourceObjectPathRow{
		path: ".git/objects/pack/pack-" + auxiliaryP1SHA1 + ".idx",
		role: sourceObjectPackIndexPath,
		key:  auxiliaryP1SHA1,
	}
	packP0Rev := sourceObjectPathRow{
		path: ".git/objects/pack/pack-" + auxiliaryP0SHA1 + ".rev",
		role: sourceObjectPackAuxiliaryPath,
		key:  auxiliaryP0SHA1,
	}
	monolithicMIDX := sourceObjectPathRow{
		path: ".git/objects/pack/multi-pack-index",
		role: sourceObjectMultiPackIndexPath,
	}
	monolithicMIDXRev := sourceObjectPathRow{
		path: ".git/objects/pack/multi-pack-index-" + auxiliaryP0SHA1 + ".rev",
		role: sourceObjectMultiPackIndexAuxiliaryPath,
		key:  auxiliaryP0SHA1,
	}
	midxChain := sourceObjectPathRow{
		path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-chain",
		role: sourceObjectMultiPackIndexChainPath,
	}
	midxP1 := sourceObjectPathRow{
		path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + auxiliaryP1SHA1 + ".midx",
		role: sourceObjectMultiPackIndexLayerPath,
		key:  auxiliaryP1SHA1,
	}
	midxP1Bitmap := sourceObjectPathRow{
		path: ".git/objects/pack/multi-pack-index.d/multi-pack-index-" + auxiliaryP1SHA1 + ".bitmap",
		role: sourceObjectMultiPackIndexLayerAuxiliaryPath,
		key:  auxiliaryP1SHA1,
	}

	for _, test := range []struct {
		name              string
		rows              []sourceObjectPathRow
		values            map[string][]string
		checksumOverrides map[string]string
		op                Operation
		cause             CauseCode
	}{
		{
			name:   "exact split commit graph members",
			rows:   []sourceObjectPathRow{graphChain, graphP0, graphP1},
			values: map[string][]string{graphChain.path: {auxiliaryP0SHA1, auxiliaryP1SHA1}},
		},
		{
			name:   "missing named split commit graph",
			rows:   []sourceObjectPathRow{graphChain, graphP0},
			values: map[string][]string{graphChain.path: {auxiliaryP0SHA1, auxiliaryP1SHA1}},
			op:     OperationOpen,
			cause:  CauseNotFound,
		},
		{
			name:   "unlisted split commit graph",
			rows:   []sourceObjectPathRow{graphChain, graphP0, graphP1},
			values: map[string][]string{graphChain.path: {auxiliaryP0SHA1}},
			op:     OperationValidate,
			cause:  CauseUnsupported,
		},
		{
			name:   "pack list stale subset",
			rows:   []sourceObjectPathRow{packList, packP0, indexP0, packP1, indexP1},
			values: map[string][]string{packList.path: {auxiliaryP0SHA1}},
		},
		{
			name:   "pack list absent member",
			rows:   []sourceObjectPathRow{packList},
			values: map[string][]string{packList.path: {auxiliaryP0SHA1}},
			op:     OperationOpen,
			cause:  CauseNotFound,
		},
		{
			name:  "pack companion without primary pair",
			rows:  []sourceObjectPathRow{packP0Rev},
			op:    OperationValidate,
			cause: CauseUnsupported,
		},
		{
			name:   "commit graph forms conflict",
			rows:   []sourceObjectPathRow{monolithicGraph, graphChain, graphP0},
			values: map[string][]string{graphChain.path: {auxiliaryP0SHA1}},
			op:     OperationValidate,
			cause:  CauseUnsupported,
		},
		{
			name: "monolithic and incremental midx coexist independently",
			rows: []sourceObjectPathRow{
				monolithicMIDX,
				monolithicMIDXRev,
				midxChain,
				midxP1,
				midxP1Bitmap,
			},
			values: map[string][]string{midxChain.path: {auxiliaryP1SHA1}},
		},
		{
			name:   "valid monolithic midx cannot hide malformed incremental closure",
			rows:   []sourceObjectPathRow{monolithicMIDX, monolithicMIDXRev, midxChain},
			values: map[string][]string{midxChain.path: {auxiliaryP1SHA1}},
			op:     OperationOpen,
			cause:  CauseNotFound,
		},
		{
			name:              "split graph checksum differs from filename and chain",
			rows:              []sourceObjectPathRow{graphChain, graphP0},
			values:            map[string][]string{graphChain.path: {auxiliaryP0SHA1}},
			checksumOverrides: map[string]string{graphP0.path: auxiliaryP1SHA1},
			op:                OperationCompare,
			cause:             CauseIdentity,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths := sourceObjectAuxiliaryTestPaths(test.rows)
			claims := sourceObjectAuxiliaryTestClaims(
				t,
				format,
				paths,
				test.values,
				test.checksumOverrides,
			)
			failure := validateSourceObjectAuxiliaryClaimInventory(
				context.Background(),
				format,
				paths,
				claims,
			)
			if test.cause == "" {
				if failure != nil {
					t.Fatalf("closure = %+v", failure)
				}
				return
			}
			requireAuxiliaryFailure(t, failure, test.op, test.cause)
		})
	}

	t.Run("duplicate pack-list member is malformed before closure", func(t *testing.T) {
		body := "P pack-" + auxiliaryP0SHA1 + ".pack\n" +
			"P pack-" + auxiliaryP0SHA1 + ".pack\n"
		_, _, failure := parseSourceObjectAuxiliaryPackList(
			context.Background(),
			objectFormatSHA1,
			[]byte(body),
		)
		requireAuxiliaryFailure(t, failure, OperationParse, CauseMalformed)
	})
}

func testSourceObjectAuxiliaryChecksumClosure(t *testing.T) {
	t.Helper()
	for _, format := range []repositoryObjectFormat{objectFormatSHA1, objectFormatSHA256} {
		p0, p1 := sourceObjectAuxiliaryTestHashes(format)
		formatName := "sha1"
		if format == objectFormatSHA256 {
			formatName = "sha256"
		}
		for _, test := range []struct {
			name              string
			rows              []sourceObjectPathRow
			values            map[string][]string
			checksumOverrides map[string]string
			cause             CauseCode
		}{
			{
				name: "matching monolithic midx companion",
				rows: []sourceObjectPathRow{
					{path: ".git/objects/pack/multi-pack-index", role: sourceObjectMultiPackIndexPath},
					{path: ".git/objects/pack/multi-pack-index-" + p0 + ".rev", role: sourceObjectMultiPackIndexAuxiliaryPath, key: p0},
				},
			},
			{
				name: "monolithic midx companion bound to wrong primary",
				rows: []sourceObjectPathRow{
					{path: ".git/objects/pack/multi-pack-index", role: sourceObjectMultiPackIndexPath},
					{path: ".git/objects/pack/multi-pack-index-" + p1 + ".bitmap", role: sourceObjectMultiPackIndexAuxiliaryPath, key: p1},
				},
				cause: CauseIdentity,
			},
			{
				name: "correct trailer under wrong filename and chain hash",
				rows: []sourceObjectPathRow{
					{path: ".git/objects/info/commit-graphs/commit-graph-chain", role: sourceObjectCommitGraphChainPath},
					{path: ".git/objects/info/commit-graphs/graph-" + p1 + ".graph", role: sourceObjectSplitCommitGraphPath, key: p1},
				},
				values: map[string][]string{
					".git/objects/info/commit-graphs/commit-graph-chain": {p1},
				},
				checksumOverrides: map[string]string{
					".git/objects/info/commit-graphs/graph-" + p1 + ".graph": p0,
				},
				cause: CauseIdentity,
			},
		} {
			t.Run(formatName+" "+test.name, func(t *testing.T) {
				paths := sourceObjectAuxiliaryTestPaths(test.rows)
				claims := sourceObjectAuxiliaryTestClaims(
					t,
					format,
					paths,
					test.values,
					test.checksumOverrides,
				)
				failure := validateSourceObjectAuxiliaryClaimInventory(
					context.Background(),
					format,
					paths,
					claims,
				)
				if test.cause == "" {
					if failure != nil {
						t.Fatalf("checksum closure = %+v", failure)
					}
					return
				}
				requireAuxiliaryFailure(t, failure, OperationCompare, test.cause)
			})
		}
	}
}

func sourceObjectAuxiliaryTestPaths(rows []sourceObjectPathRow) *sourceObjectPathInventory {
	byPath := map[string]sourceObjectPathRow{
		".git/objects": {
			path: ".git/objects",
			role: sourceObjectDirectoryPath,
		},
	}
	for index := range rows {
		row := rows[index]
		byPath[row.path] = row
		parent := row.path
		for parent != ".git/objects" {
			separator := strings.LastIndexByte(parent, '/')
			if separator < len(".git/objects") {
				break
			}
			parent = parent[:separator]
			if _, present := byPath[parent]; !present {
				byPath[parent] = sourceObjectPathRow{
					path: parent,
					role: sourceObjectDirectoryPath,
				}
			}
		}
	}
	paths := make([]sourceObjectPathRow, 0, len(byPath))
	for _, row := range byPath {
		paths = append(paths, row)
	}
	sort.Slice(paths, func(left, right int) bool {
		return paths[left].path < paths[right].path
	})
	inventory := &sourceObjectPathInventory{rows: paths}
	for index := range paths {
		if paths[index].role.content() {
			inventory.contentFileCount++
		}
		if paths[index].role.auxiliary() {
			inventory.auxiliaryFileCount++
		}
	}
	return inventory
}

func sourceObjectAuxiliaryTestClaims(
	t *testing.T,
	format repositoryObjectFormat,
	paths *sourceObjectPathInventory,
	values map[string][]string,
	checksumOverrides map[string]string,
) *sourceObjectAuxiliaryClaimInventory {
	t.Helper()
	claims := &sourceObjectAuxiliaryClaimInventory{
		rows: make([]sourceObjectAuxiliaryClaim, 0, paths.auxiliaryFileCount),
	}
	for index := range paths.rows {
		row := paths.rows[index]
		if !row.role.auxiliary() {
			continue
		}
		claim := sourceObjectAuxiliaryClaim{
			path: row.path,
			role: row.role,
			bytes: sourceObjectAuxiliaryByteClaim{
				repositoryFormat: format,
			},
		}
		claim.values = append([]string(nil), values[row.path]...)
		switch row.role {
		case sourceObjectCommitGraphChainPath, sourceObjectMultiPackIndexChainPath:
			claim.bytes.size = uint64(len(claim.values) * (format.hexWidth() + 1))
		case sourceObjectPackListPath:
			claim.bytes.size = uint64(len(claim.values) *
				(len("P pack-") + format.hexWidth() + len(".pack\n")))
		case sourceObjectCommitGraphPath, sourceObjectSplitCommitGraphPath,
			sourceObjectMultiPackIndexPath, sourceObjectMultiPackIndexLayerPath:
			hash := row.key
			if hash == "" {
				hash, _ = sourceObjectAuxiliaryTestHashes(format)
			}
			if replacement := checksumOverrides[row.path]; replacement != "" {
				hash = replacement
			}
			claim.bytes.repositoryHash = sourceObjectAuxiliaryTestRawHash(t, format, hash)
			claim.bytes.repositoryHashValid = true
			claim.bytes.size = uint64(format.hexWidth()/2 + len("fixture-body\n"))
		case sourceObjectPackAuxiliaryPath, sourceObjectMultiPackIndexAuxiliaryPath,
			sourceObjectMultiPackIndexLayerAuxiliaryPath:
		case sourceObjectDirectoryPath, sourceObjectLooseContentPath,
			sourceObjectPackContentPath, sourceObjectPackIndexPath:
			t.Fatalf("non-auxiliary row reached claim fixture: %+v", row)
		}
		claims.rows = append(claims.rows, claim)
	}
	return claims
}

func sourceObjectAuxiliaryTestRawHash(
	t *testing.T,
	format repositoryObjectFormat,
	value string,
) [32]byte {
	t.Helper()
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != format.hexWidth()/2 {
		t.Fatalf("decode fixture hash %q: %v / %d", value, err, len(raw))
	}
	var digest [32]byte
	copy(digest[:], raw)
	return digest
}

func sourceObjectAuxiliaryTestHashes(format repositoryObjectFormat) (string, string) {
	if format == objectFormatSHA1 {
		return auxiliaryP0SHA1, auxiliaryP1SHA1
	}
	if format == objectFormatSHA256 {
		return auxiliaryP0SHA256, auxiliaryP1SHA256
	}
	return "", ""
}

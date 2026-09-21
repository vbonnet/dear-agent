//go:build darwin && arm64

package buildauthority

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type countingSourceObjectClaimPrimitives struct {
	sourcePrimitives
	parseReadCalls  int
	hashReadCalls   int
	failHashRead    int
	mutateHashRead  int
	mutateAfterHash func()
}

func (primitives *countingSourceObjectClaimPrimitives) readExactAtForParse(
	ctx context.Context,
	owner *ownedSourceDescriptor,
	offset int64,
	content []byte,
) *sourcePrimitiveFailure {
	primitives.parseReadCalls++
	return primitives.sourcePrimitives.readExactAtForParse(ctx, owner, offset, content)
}

func (primitives *countingSourceObjectClaimPrimitives) readExactAtForHash(
	ctx context.Context,
	owner *ownedSourceDescriptor,
	offset int64,
	content []byte,
) *sourcePrimitiveFailure {
	primitives.hashReadCalls++
	if primitives.hashReadCalls == primitives.failHashRead {
		return newSourcePrimitiveFailure(OperationHash, CauseUnstable)
	}
	failure := primitives.sourcePrimitives.readExactAtForHash(ctx, owner, offset, content)
	if failure == nil && primitives.hashReadCalls == primitives.mutateHashRead &&
		primitives.mutateAfterHash != nil {
		primitives.mutateAfterHash()
	}
	return failure
}

func TestDarwinSourceConstructionIntegratesAndRevalidatesAuxiliaryClaims(t *testing.T) {
	repository, _ := writeDarwinSourceConstructionRepository(t)
	hash := strings.Repeat("a", objectFormatSHA1.hexWidth())
	packDirectory := filepath.Join(repository, ".git", "objects", "pack")
	makeDarwinSourceConstructionDirectory(t, packDirectory)
	writeDarwinSourceConstructionFile(
		t,
		filepath.Join(packDirectory, "pack-"+hash+".pack"),
		[]byte("opaque pack content"),
	)
	writeDarwinSourceConstructionFile(
		t,
		filepath.Join(packDirectory, "pack-"+hash+".idx"),
		[]byte("opaque index content"),
	)
	auxiliaryBody := []byte("opaque keep body\n")
	writeDarwinSourceConstructionFile(
		t,
		filepath.Join(packDirectory, "pack-"+hash+".keep"),
		auxiliaryBody,
	)

	primitives := &countingSourceObjectClaimPrimitives{
		sourcePrimitives: requireDarwinSourcePrimitives(t),
	}
	owner, outcome := retainSourceConstructionWith(
		context.Background(),
		sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
		primitives,
	)
	if owner == nil || !outcome.proved() || !owner.validObjectClaimRetention() {
		t.Fatalf("full source construction = %+v / %+v", owner, outcome)
	}
	if primitives.hashReadCalls != 2 {
		t.Fatalf("auxiliary hash reads = %d, want admission plus revalidation", primitives.hashReadCalls)
	}
	if got := owner.objects.claims.rows; len(got) != 1 ||
		got[0].role != sourceObjectPackAuxiliaryPath ||
		got[0].bytes.size != uint64(len(auxiliaryBody)) {
		t.Fatalf("retained auxiliary claims = %+v", got)
	}

	var closeOutcome sourceUseOutcome
	owner.closeIntoWith(primitives, &closeOutcome)
	if !closeOutcome.proved() {
		t.Fatalf("full source owner close = %+v", closeOutcome)
	}
}

func TestDarwinSourceConstructionAuxiliaryReadFailureReturnsNoOwner(t *testing.T) {
	for _, test := range []struct {
		name     string
		failRead int
	}{
		{name: "admission", failRead: 1},
		{name: "revalidation", failRead: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, _ := writeDarwinSourceConstructionRepository(t)
			hash := strings.Repeat("b", objectFormatSHA1.hexWidth())
			packDirectory := filepath.Join(repository, ".git", "objects", "pack")
			makeDarwinSourceConstructionDirectory(t, packDirectory)
			writeDarwinSourceConstructionFile(
				t,
				filepath.Join(packDirectory, "pack-"+hash+".pack"),
				[]byte("opaque pack content"),
			)
			writeDarwinSourceConstructionFile(
				t,
				filepath.Join(packDirectory, "pack-"+hash+".idx"),
				[]byte("opaque index content"),
			)
			writeDarwinSourceConstructionFile(
				t,
				filepath.Join(packDirectory, "pack-"+hash+".keep"),
				[]byte("opaque keep body\n"),
			)

			primitives := &countingSourceObjectClaimPrimitives{
				sourcePrimitives: requireDarwinSourcePrimitives(t),
				failHashRead:     test.failRead,
			}
			owner, outcome := retainSourceConstructionWith(
				context.Background(),
				sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
				primitives,
			)
			if owner != nil {
				var closeOutcome sourceUseOutcome
				owner.closeIntoWith(primitives, &closeOutcome)
				t.Fatalf("failed %s returned owner %+v", test.name, owner)
			}
			requireFailureRecord(
				t,
				outcome.primary,
				PhaseSource,
				OperationHash,
				CauseUnstable,
			)
			if outcome.descriptorClose != nil || primitives.hashReadCalls != test.failRead {
				t.Fatalf("failed %s outcome = %+v; reads %d", test.name, outcome, primitives.hashReadCalls)
			}
		})
	}
}

func TestDarwinSourceObjectAuxiliaryRevalidationReadsBeforeContentDriftAttribution(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(*testing.T, string) string
		mutate           func([]byte)
		readCount        func(*countingSourceObjectClaimPrimitives) int
		replace          bool
		replaceDirectory bool
		readDelta        int
		operation        Operation
		cause            CauseCode
	}{
		{
			name: "malformed chain",
			setup: func(t *testing.T, repository string) string {
				directory := filepath.Join(repository, ".git", "objects", "info", "commit-graphs")
				makeDarwinSourceConstructionDirectory(t, directory)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(directory, "graph-"+auxiliaryP0SHA1+".graph"),
					sourceObjectAuxiliaryTestPrimary(objectFormatSHA1, []byte("fixture-body\n")),
				)
				path := filepath.Join(directory, "commit-graph-chain")
				writeDarwinSourceConstructionFile(t, path, []byte(auxiliaryP0SHA1+"\n"))
				return path
			},
			mutate: func(content []byte) {
				content[0] = 'A'
			},
			readCount: func(primitives *countingSourceObjectClaimPrimitives) int {
				return primitives.parseReadCalls
			},
			readDelta: 1,
			operation: OperationParse,
			cause:     CauseMalformed,
		},
		{
			name: "bad primary trailer",
			setup: func(t *testing.T, repository string) string {
				directory := filepath.Join(repository, ".git", "objects", "info", "commit-graphs")
				makeDarwinSourceConstructionDirectory(t, directory)
				path := filepath.Join(directory, "graph-"+auxiliaryP0SHA1+".graph")
				writeDarwinSourceConstructionFile(
					t,
					path,
					sourceObjectAuxiliaryTestPrimary(objectFormatSHA1, []byte("fixture-body\n")),
				)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(directory, "commit-graph-chain"),
					[]byte(auxiliaryP0SHA1+"\n"),
				)
				return path
			},
			mutate: func(content []byte) {
				content[len(content)-1] ^= 0x01
			},
			readCount: func(primitives *countingSourceObjectClaimPrimitives) int {
				return primitives.hashReadCalls
			},
			readDelta: 1,
			operation: OperationCompare,
			cause:     CauseIdentity,
		},
		{
			name: "opaque body drift",
			setup: func(t *testing.T, repository string) string {
				hash := strings.Repeat("c", objectFormatSHA1.hexWidth())
				directory := filepath.Join(repository, ".git", "objects", "pack")
				makeDarwinSourceConstructionDirectory(t, directory)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(directory, "pack-"+hash+".pack"),
					[]byte("opaque pack content"),
				)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(directory, "pack-"+hash+".idx"),
					[]byte("opaque index content"),
				)
				path := filepath.Join(directory, "pack-"+hash+".keep")
				writeDarwinSourceConstructionFile(t, path, []byte("opaque keep body\n"))
				return path
			},
			mutate: func(content []byte) {
				content[0] ^= 0x01
			},
			readCount: func(primitives *countingSourceObjectClaimPrimitives) int {
				return primitives.hashReadCalls
			},
			readDelta: 1,
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name: "replacement identity before parent directory drift",
			setup: func(t *testing.T, repository string) string {
				hash := strings.Repeat("d", objectFormatSHA1.hexWidth())
				directory := filepath.Join(repository, ".git", "objects", "pack")
				makeDarwinSourceConstructionDirectory(t, directory)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(directory, "pack-"+hash+".pack"),
					[]byte("opaque pack content"),
				)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(directory, "pack-"+hash+".idx"),
					[]byte("opaque index content"),
				)
				path := filepath.Join(directory, "pack-"+hash+".keep")
				writeDarwinSourceConstructionFile(t, path, []byte("opaque keep body\n"))
				return path
			},
			mutate: func(content []byte) {
				content[0] ^= 0x01
			},
			readCount: func(primitives *countingSourceObjectClaimPrimitives) int {
				return primitives.hashReadCalls
			},
			replace:   true,
			readDelta: 0,
			operation: OperationCompare,
			cause:     CauseIdentity,
		},
		{
			name: "replacement directory identity before root drift",
			setup: func(t *testing.T, repository string) string {
				hash := strings.Repeat("e", objectFormatSHA1.hexWidth())
				directory := filepath.Join(repository, ".git", "objects", "pack")
				makeDarwinSourceConstructionDirectory(t, directory)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(directory, "pack-"+hash+".pack"),
					[]byte("opaque pack content"),
				)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(directory, "pack-"+hash+".idx"),
					[]byte("opaque index content"),
				)
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(directory, "pack-"+hash+".keep"),
					[]byte("opaque keep body\n"),
				)
				return directory
			},
			readCount: func(primitives *countingSourceObjectClaimPrimitives) int {
				return primitives.hashReadCalls
			},
			replaceDirectory: true,
			readDelta:        0,
			operation:        OperationCompare,
			cause:            CauseIdentity,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, _ := writeDarwinSourceConstructionRepository(t)
			path := test.setup(t, repository)
			primitives := &countingSourceObjectClaimPrimitives{
				sourcePrimitives: requireDarwinSourcePrimitives(t),
			}
			owner, outcome := retainInitialSourceConstructionForTest(
				context.Background(),
				sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
				primitives,
			)
			if owner == nil || !outcome.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, outcome)
			}
			closed := false
			t.Cleanup(func() {
				if !closed {
					var cleanup sourceUseOutcome
					owner.closeIntoWith(primitives, &cleanup)
				}
			})
			builder := &sourceConstructionBuilder{
				ctx:        context.Background(),
				primitives: primitives,
				owner:      owner,
			}
			retained := builder.retainPackedRefs() &&
				builder.retainSourceAdministrativeInventory() &&
				builder.retainSourceObjectPathInventory() &&
				builder.retainSourceObjectAuxiliaryClaimInventory()
			if !retained || !builder.outcome.proved() || !owner.validObjectClaimRetention() {
				t.Fatalf("initial auxiliary retention = %+v / %+v", owner, builder.outcome)
			}
			readsBefore := test.readCount(primitives)
			switch {
			case test.replaceDirectory:
				replaceDarwinSourceConstructionDirectory(t, path)
			case test.replace:
				replaceDarwinSourceConstructionFile(t, path, test.mutate)
			default:
				mutateDarwinSourceConstructionFileSameIdentity(t, path, test.mutate)
			}

			if builder.revalidateSourceObjectAuxiliaryClaimInventory() {
				t.Fatal("content-drift revalidation succeeded")
			}
			requireFailureRecord(
				t,
				builder.outcome.primary,
				PhaseSource,
				test.operation,
				test.cause,
			)
			if readsAfter := test.readCount(primitives); readsAfter != readsBefore+test.readDelta {
				t.Fatalf(
					"revalidation reads = %d after %d, want delta %d",
					readsAfter,
					readsBefore,
					test.readDelta,
				)
			}

			var closeOutcome sourceUseOutcome
			owner.closeIntoWith(primitives, &closeOutcome)
			closed = true
			if !closeOutcome.proved() {
				t.Fatalf("content-drift owner close = %+v", closeOutcome)
			}
		})
	}
}

func TestDarwinSourceObjectAuxiliaryRevalidationMetadataPolicy(t *testing.T) {
	for _, test := range []struct {
		name      string
		prepare   func(*testing.T, string) func(*testing.T)
		wantOK    bool
		wantCause CauseCode
		readDelta int
	}{
		{
			name: "same-inode inert bytes remain allowed",
			prepare: func(t *testing.T, repository string) func(*testing.T) {
				path := filepath.Join(repository, ".git", "HEAD")
				writeDarwinSourceConstructionFile(t, path, []byte("ref: refs/heads/main\n"))
				return func(t *testing.T) {
					before, err := os.Stat(path)
					if err != nil {
						t.Fatalf("stat inert source file %q before mutation: %v", path, err)
					}
					content := []byte("ref: refs/heads/feature\n")
					if err := os.WriteFile(path, content, 0o600); err != nil {
						t.Fatalf("mutate inert source file %q: %v", path, err)
					}
					after, err := os.Stat(path)
					if err != nil || !os.SameFile(before, after) || before.Size() == after.Size() {
						t.Fatalf("inert mutation did not preserve identity with size drift: before=%+v after=%+v err=%v", before, after, err)
					}
				}
			},
			wantOK:    true,
			readDelta: 1,
		},
		{
			name: "fixed directory timestamp drift follows content proof",
			prepare: func(t *testing.T, repository string) func(*testing.T) {
				objects := filepath.Join(repository, ".git", "objects")
				return func(t *testing.T) {
					churn := filepath.Join(objects, ".timestamp-churn")
					if err := os.WriteFile(churn, []byte("x"), 0o600); err != nil {
						t.Fatalf("create directory churn fixture %q: %v", churn, err)
					}
					if err := os.Remove(churn); err != nil {
						t.Fatalf("remove directory churn fixture %q: %v", churn, err)
					}
				}
			},
			wantCause: CauseUnstable,
			readDelta: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, _ := writeDarwinSourceConstructionRepository(t)
			mutate := test.prepare(t, repository)
			hash := strings.Repeat("f", objectFormatSHA1.hexWidth())
			packDirectory := filepath.Join(repository, ".git", "objects", "pack")
			makeDarwinSourceConstructionDirectory(t, packDirectory)
			writeDarwinSourceConstructionFile(
				t,
				filepath.Join(packDirectory, "pack-"+hash+".pack"),
				[]byte("opaque pack content"),
			)
			writeDarwinSourceConstructionFile(
				t,
				filepath.Join(packDirectory, "pack-"+hash+".idx"),
				[]byte("opaque index content"),
			)
			writeDarwinSourceConstructionFile(
				t,
				filepath.Join(packDirectory, "pack-"+hash+".keep"),
				[]byte("opaque keep body\n"),
			)
			primitives := &countingSourceObjectClaimPrimitives{
				sourcePrimitives: requireDarwinSourcePrimitives(t),
			}
			owner, outcome := retainInitialSourceConstructionForTest(
				context.Background(),
				sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
				primitives,
			)
			if owner == nil || !outcome.proved() {
				t.Fatalf("initial source construction = %+v / %+v", owner, outcome)
			}
			builder := &sourceConstructionBuilder{
				ctx:        context.Background(),
				primitives: primitives,
				owner:      owner,
			}
			retained := builder.retainPackedRefs() &&
				builder.retainSourceAdministrativeInventory() &&
				builder.retainSourceObjectPathInventory() &&
				builder.retainSourceObjectAuxiliaryClaimInventory()
			if !retained || !builder.outcome.proved() || !owner.validObjectClaimRetention() {
				t.Fatalf("initial auxiliary retention = %+v / %+v", owner, builder.outcome)
			}
			readsBefore := primitives.hashReadCalls
			mutate(t)
			if got := builder.revalidateSourceObjectAuxiliaryClaimInventory(); got != test.wantOK {
				t.Fatalf("metadata revalidation = %v, want %v; outcome %+v", got, test.wantOK, builder.outcome)
			}
			if test.wantOK {
				if !builder.outcome.proved() {
					t.Fatalf("successful metadata revalidation = %+v", builder.outcome)
				}
			} else {
				requireFailureRecord(
					t,
					builder.outcome.primary,
					PhaseSource,
					OperationCompare,
					test.wantCause,
				)
			}
			if primitives.hashReadCalls != readsBefore+test.readDelta {
				t.Fatalf("metadata revalidation hash reads = %d after %d, want delta %d", primitives.hashReadCalls, readsBefore, test.readDelta)
			}
			var closeOutcome sourceUseOutcome
			owner.closeIntoWith(primitives, &closeOutcome)
			if !closeOutcome.proved() {
				t.Fatalf("metadata-policy owner close = %+v", closeOutcome)
			}
		})
	}
}

func TestDarwinSourceObjectAuxiliaryRejectsHashReadTimeObjectRootReplacement(t *testing.T) {
	repository, _ := writeDarwinSourceConstructionRepository(t)
	hash := strings.Repeat("9", objectFormatSHA1.hexWidth())
	objects := filepath.Join(repository, ".git", "objects")
	pack := filepath.Join(objects, "pack")
	makeDarwinSourceConstructionDirectory(t, pack)
	files := map[string][]byte{
		"pack-" + hash + ".pack": []byte("opaque pack content"),
		"pack-" + hash + ".idx":  []byte("opaque index content"),
		"pack-" + hash + ".keep": []byte("opaque keep body\n"),
	}
	for name, content := range files {
		writeDarwinSourceConstructionFile(t, filepath.Join(pack, name), content)
	}
	replacementObjects := filepath.Join(repository, "replacement-objects")
	replacementPack := filepath.Join(replacementObjects, "pack")
	makeDarwinSourceConstructionDirectory(t, replacementPack)
	for name, content := range files {
		writeDarwinSourceConstructionFile(t, filepath.Join(replacementPack, name), content)
	}

	primitives := &countingSourceObjectClaimPrimitives{
		sourcePrimitives: requireDarwinSourcePrimitives(t),
		mutateHashRead:   2,
		mutateAfterHash: func() {
			retiredObjects := filepath.Join(repository, "retired-objects")
			if err := os.Rename(objects, retiredObjects); err != nil {
				t.Fatalf("retire object root during revalidation hash: %v", err)
			}
			if err := os.Rename(replacementObjects, objects); err != nil {
				t.Fatalf("replace object root during revalidation hash: %v", err)
			}
		},
	}
	owner, outcome := retainSourceConstructionWith(
		context.Background(),
		sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
		primitives,
	)
	if owner != nil {
		var closeOutcome sourceUseOutcome
		owner.closeIntoWith(primitives, &closeOutcome)
		t.Fatalf("hash-time object-root replacement returned owner %+v", owner)
	}
	requireFailureRecord(
		t,
		outcome.primary,
		PhaseSource,
		OperationCompare,
		CauseIdentity,
	)
	if outcome.descriptorClose != nil || primitives.hashReadCalls != 2 {
		t.Fatalf("hash-time object-root replacement = %+v; reads %d", outcome, primitives.hashReadCalls)
	}
}

func TestDarwinSourceObjectAuxiliaryRejectsPreRevalidationGitReplacement(t *testing.T) {
	repository, _ := writeDarwinSourceConstructionRepository(t)
	hash := strings.Repeat("8", objectFormatSHA1.hexWidth())
	gitPath := filepath.Join(repository, ".git")
	pack := filepath.Join(gitPath, "objects", "pack")
	info := filepath.Join(gitPath, "objects", "info")
	makeDarwinSourceConstructionDirectory(t, pack)
	makeDarwinSourceConstructionDirectory(t, info)
	files := map[string][]byte{
		"pack-" + hash + ".pack": []byte("opaque pack content"),
		"pack-" + hash + ".idx":  []byte("opaque index content"),
		"pack-" + hash + ".keep": []byte("opaque keep body\n"),
	}
	for name, content := range files {
		writeDarwinSourceConstructionFile(t, filepath.Join(pack, name), content)
	}
	packList := []byte("P pack-" + hash + ".pack\n")
	writeDarwinSourceConstructionFile(t, filepath.Join(info, "packs"), packList)
	replacementGit := filepath.Join(repository, "replacement-git")
	replacementPack := filepath.Join(replacementGit, "objects", "pack")
	replacementInfo := filepath.Join(replacementGit, "objects", "info")
	makeDarwinSourceConstructionDirectory(t, replacementPack)
	makeDarwinSourceConstructionDirectory(t, replacementInfo)
	writeDarwinSourceConstructionFile(
		t,
		filepath.Join(replacementGit, "config"),
		[]byte(validSourceConstructionConfig),
	)
	for name, content := range files {
		writeDarwinSourceConstructionFile(t, filepath.Join(replacementPack, name), content)
	}
	writeDarwinSourceConstructionFile(t, filepath.Join(replacementInfo, "packs"), packList)

	primitives := &countingSourceObjectClaimPrimitives{
		sourcePrimitives: requireDarwinSourcePrimitives(t),
	}
	owner, outcome := retainInitialSourceConstructionForTest(
		context.Background(),
		sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
		primitives,
	)
	if owner == nil || !outcome.proved() {
		t.Fatalf("initial source construction = %+v / %+v", owner, outcome)
	}
	builder := &sourceConstructionBuilder{
		ctx:        context.Background(),
		primitives: primitives,
		owner:      owner,
	}
	retained := builder.retainPackedRefs() &&
		builder.retainSourceAdministrativeInventory() &&
		builder.retainSourceObjectPathInventory() &&
		builder.retainSourceObjectAuxiliaryClaimInventory()
	if !retained || !builder.outcome.proved() || !owner.validObjectClaimRetention() {
		t.Fatalf("initial auxiliary retention = %+v / %+v", owner, builder.outcome)
	}
	retiredGit := filepath.Join(repository, "retired-git")
	if err := os.Rename(gitPath, retiredGit); err != nil {
		t.Fatalf("retire git directory before revalidation: %v", err)
	}
	if err := os.Rename(replacementGit, gitPath); err != nil {
		t.Fatalf("replace git directory before revalidation: %v", err)
	}
	malformedPackList := append([]byte(nil), packList...)
	malformedPackList[0] = 'Q'
	if err := os.WriteFile(
		filepath.Join(retiredGit, "objects", "info", "packs"),
		malformedPackList,
		0o600,
	); err != nil {
		t.Fatalf("malform retired pack list before revalidation: %v", err)
	}
	if builder.revalidateSourceObjectAuxiliaryClaimInventory() {
		t.Fatal("pre-revalidation git replacement succeeded")
	}
	requireFailureRecord(
		t,
		builder.outcome.primary,
		PhaseSource,
		OperationCompare,
		CauseIdentity,
	)
	if primitives.parseReadCalls != 1 || primitives.hashReadCalls != 1 {
		t.Fatalf(
			"pre-revalidation git replacement reads = parse %d hash %d, want admission only",
			primitives.parseReadCalls,
			primitives.hashReadCalls,
		)
	}
	var closeOutcome sourceUseOutcome
	owner.closeIntoWith(primitives, &closeOutcome)
	if !closeOutcome.proved() {
		t.Fatalf("pre-revalidation git replacement owner close = %+v", closeOutcome)
	}
}

func mutateDarwinSourceConstructionFileSameIdentity(
	t *testing.T,
	path string,
	mutate func([]byte),
) {
	t.Helper()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat source construction file %q before mutation: %v", path, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source construction file %q before mutation: %v", path, err)
	}
	mutate(content)
	if int64(len(content)) != before.Size() {
		t.Fatalf("same-identity mutation changed %q size from %d to %d", path, before.Size(), len(content))
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("mutate source construction file %q: %v", path, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat source construction file %q after mutation: %v", path, err)
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() ||
		before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("mutation did not preserve identity and size with timestamp drift: before=%+v after=%+v", before, after)
	}
}

func replaceDarwinSourceConstructionFile(
	t *testing.T,
	path string,
	mutate func([]byte),
) {
	t.Helper()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat source construction file %q before replacement: %v", path, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read source construction file %q before replacement: %v", path, err)
	}
	mutate(content)
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, content, 0o600); err != nil {
		t.Fatalf("write source construction replacement %q: %v", replacement, err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatalf("replace source construction file %q: %v", path, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat source construction file %q after replacement: %v", path, err)
	}
	if os.SameFile(before, after) || before.Size() != after.Size() {
		t.Fatalf("replacement did not change identity while preserving size: before=%+v after=%+v", before, after)
	}
}

func replaceDarwinSourceConstructionDirectory(t *testing.T, path string) {
	t.Helper()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat source construction directory %q before replacement: %v", path, err)
	}
	repository := filepath.Dir(filepath.Dir(filepath.Dir(path)))
	replacement := filepath.Join(repository, "replacement-pack")
	retired := filepath.Join(repository, "retired-pack")
	makeDarwinSourceConstructionDirectory(t, replacement)
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read source construction directory %q: %v", path, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected nested directory %q in replacement fixture", entry.Name())
		}
		content, readErr := os.ReadFile(filepath.Join(path, entry.Name()))
		if readErr != nil {
			t.Fatalf("read replacement fixture %q: %v", entry.Name(), readErr)
		}
		writeDarwinSourceConstructionFile(t, filepath.Join(replacement, entry.Name()), content)
	}
	if err := os.Rename(path, retired); err != nil {
		t.Fatalf("retire source construction directory %q: %v", path, err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatalf("replace source construction directory %q: %v", path, err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat source construction directory %q after replacement: %v", path, err)
	}
	if os.SameFile(before, after) {
		t.Fatalf("directory replacement preserved identity: before=%+v after=%+v", before, after)
	}
}

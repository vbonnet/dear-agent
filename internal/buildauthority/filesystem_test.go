package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestManifestGoldenDomainsAndExactFraming(t *testing.T) {
	t.Parallel()

	root, entries := manifestFixture(t)
	rootRecord := rootManifestRecord(root)
	if len(rootRecord) != 65 {
		t.Fatalf("root record length = %d, want 65", len(rootRecord))
	}
	if rootRecord[0] != 0 {
		t.Fatalf("root kind = %d, want 0", rootRecord[0])
	}

	goldens := map[string]string{
		domainGOROOT:         "8c39f3310cfb23347a88725d868587c705ea6207d3a9b637dfd8efbada2dd590",
		domainRepository:     "084f5aecf4272fa159ab6ccaac5c98be783e29be7e8cf02cd553c695f9b7c50b",
		domainSourceObjects:  "afca145da5e5db06ed1f88c4db792557a15a484e1c2a314f1abff7740e1b0899",
		domainSourceTree:     "db76e59811c2b6c06f032f42f837b77654df9f542f11e1561e1ef10a86d5641c",
		domainGOMODCACHE:     "c55c75703c5e091795fbf5084ab80b243c7bf8873bcf3fc2f8b547e9296d0d9b",
		domainSelectedModule: "066e19b0cff70f7ae4f4a73e8976575a5cef9f9389ed14d5b84665c0d148815d",
	}
	for domain, golden := range goldens {
		t.Run(domain, func(t *testing.T) {
			t.Parallel()

			wire := independentlyEncodeManifest(domain, root, entries)
			want := sha256.Sum256(wire)
			got := digestTreeManifest(domain, root, entries)
			if got != want {
				t.Fatalf("digest = %x, independently encoded digest = %x", got, want)
			}
			if encoded := hex.EncodeToString(got[:]); encoded != golden {
				t.Fatalf("golden = %q, want %q", encoded, golden)
			}
		})
	}
}

func TestManifestSortsRawControlAndNewlinePaths(t *testing.T) {
	t.Parallel()

	entries := []entrySnapshot{
		{path: "z"},
		{path: "line\nname"},
		{path: "alpha"},
		{path: "\x01control"},
	}
	sortEntrySnapshots(entries)
	want := []string{"\x01control", "alpha", "line\nname", "z"}
	for index := range entries {
		if entries[index].path != want[index] {
			t.Fatalf("entry[%d] = %q, want %q", index, entries[index].path, want[index])
		}
	}
	for _, path := range []string{"\x01control", "line\nname"} {
		if err := validateRelativeManifestPath(path); err != nil {
			t.Fatalf("validateRelativeManifestPath(%q) = %v", path, err)
		}
	}
}

func TestManifestRelativePathExactLimits(t *testing.T) {
	t.Parallel()

	exactComponent := strings.Repeat("a", maxPathComponentBytes)
	if err := validateRelativeManifestPath(exactComponent); err != nil {
		t.Fatalf("exact component = %v", err)
	}
	requirePrivateCauses(t, validateRelativeManifestPath(exactComponent+"a"), CauseLimit)

	exactPath := strings.Join([]string{
		strings.Repeat("a", 255),
		strings.Repeat("b", 255),
		strings.Repeat("c", 255),
		strings.Repeat("d", 253),
		"e",
	}, "/")
	if len([]byte(exactPath)) != maxRelativePathBytes {
		t.Fatalf("test exact path length = %d", len([]byte(exactPath)))
	}
	if err := validateRelativeManifestPath(exactPath); err != nil {
		t.Fatalf("exact path = %v", err)
	}
	overPath := exactPath + "f"
	if len([]byte(overPath)) != maxRelativePathBytes+1 {
		t.Fatalf("test over path length = %d", len([]byte(overPath)))
	}
	requirePrivateCauses(t, validateRelativeManifestPath(overPath), CauseLimit)
}

func TestTreeBudgetExactAndOneOver(t *testing.T) {
	t.Parallel()

	policies := []struct {
		name   string
		policy treePolicy
	}{
		{name: "goroot", policy: gorootPolicy()},
		{name: "gomodcache", policy: gomodcachePolicy()},
	}
	for _, test := range policies {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			limits := test.policy.limits
			if got, err := chargeTreeEntry(limits, limits.maxEntries-1, 0, entrySnapshot{kind: entryDirectory}); err != nil || got != 0 {
				t.Fatalf("exact entry count = (%d, %v), want (0, nil)", got, err)
			}
			_, err := chargeTreeEntry(limits, limits.maxEntries, 0, entrySnapshot{kind: entryDirectory})
			requirePrivateCauses(t, err, CauseLimit)

			exactFile := entrySnapshot{kind: entryRegular, file: fileSnapshot{size: int64(limits.maxFileBytes)}}
			if got, err := chargeTreeEntry(limits, 0, 0, exactFile); err != nil || got != limits.maxFileBytes {
				t.Fatalf("exact file bytes = (%d, %v), want (%d, nil)", got, err, limits.maxFileBytes)
			}
			overFile := entrySnapshot{kind: entryRegular, file: fileSnapshot{size: int64(limits.maxFileBytes + 1)}}
			_, err = chargeTreeEntry(limits, 0, 0, overFile)
			requirePrivateCauses(t, err, CauseLimit)

			oneByte := entrySnapshot{kind: entryRegular, file: fileSnapshot{size: 1}}
			if got, err := chargeTreeEntry(limits, 0, limits.maxBytes-1, oneByte); err != nil || got != limits.maxBytes {
				t.Fatalf("exact aggregate bytes = (%d, %v), want (%d, nil)", got, err, limits.maxBytes)
			}
			_, err = chargeTreeEntry(limits, 0, limits.maxBytes, oneByte)
			requirePrivateCauses(t, err, CauseLimit)
		})
	}
}

func TestSymlinkTargetDigestBindsFramedFinalRegularRecord(t *testing.T) {
	t.Parallel()

	target := entrySnapshot{
		path:      "target",
		kind:      entryRegular,
		file:      fileSnapshot{identity: FileIdentity{Mode: 0o100555, UID: 501}, gid: 20, size: 3},
		aclDigest: sequentialDigest(0x20),
		content:   sha256.Sum256([]byte("abc")),
	}
	entries := []entrySnapshot{
		target,
		{
			path:      "link",
			kind:      entrySymlink,
			file:      fileSnapshot{identity: FileIdentity{Mode: 0o120555, UID: 501}, gid: 20, size: 6},
			aclDigest: sequentialDigest(0x40),
			linkText:  "target",
		},
	}
	if err := bindSymlinkTargets(entries); err != nil {
		t.Fatalf("bindSymlinkTargets() = %v", err)
	}
	targetRecord := independentlyEncodeEntry(target)
	framed := appendTestUint64(nil, uint64(len(targetRecord)))
	framed = append(framed, targetRecord...)
	want := sha256.Sum256(framed)
	if entries[1].targetDigest != want {
		t.Fatalf("target digest = %x, want framed-record digest %x", entries[1].targetDigest, want)
	}

	before := entries[1].targetDigest
	entries[0].content[0] ^= 0xff
	if err := bindSymlinkTargets(entries); err != nil {
		t.Fatalf("bindSymlinkTargets() after drift = %v", err)
	}
	if entries[1].targetDigest == before {
		t.Fatal("target digest did not change with the protected regular record")
	}
}

func TestSymlinkResolutionExactDepthAndRefusals(t *testing.T) {
	t.Parallel()

	for _, depth := range []int{40, 41} {
		t.Run(fmt.Sprintf("depth-%d", depth), func(t *testing.T) {
			t.Parallel()

			entries := symlinkChain(depth)
			got, err := resolveContainedTarget("link-000", entries)
			if depth == 40 {
				if err != nil || got != "target" {
					t.Fatalf("resolve 40 links = (%q, %v), want (target, nil)", got, err)
				}
				return
			}
			requirePrivateCauses(t, err, CauseLimit)
		})
	}

	tests := []struct {
		name    string
		start   string
		entries map[string]*entrySnapshot
		cause   CauseCode
	}{
		{
			name:    "absolute",
			start:   "link",
			entries: entryMap(entrySnapshot{path: "link", kind: entrySymlink, linkText: "/target"}),
			cause:   CauseUnsupported,
		},
		{
			name:    "dangling",
			start:   "link",
			entries: entryMap(entrySnapshot{path: "link", kind: entrySymlink, linkText: "missing"}),
			cause:   CauseNotFound,
		},
		{
			name:  "directory-target",
			start: "link",
			entries: entryMap(
				entrySnapshot{path: "link", kind: entrySymlink, linkText: "directory"},
				entrySnapshot{path: "directory", kind: entryDirectory},
			),
			cause: CauseUnsupported,
		},
		{
			name:  "cycle",
			start: "a",
			entries: entryMap(
				entrySnapshot{path: "a", kind: entrySymlink, linkText: "b"},
				entrySnapshot{path: "b", kind: entrySymlink, linkText: "a"},
			),
			cause: CauseMalformed,
		},
		{
			name:    "escape",
			start:   "link",
			entries: entryMap(entrySnapshot{path: "link", kind: entrySymlink, linkText: "../target"}),
			cause:   CauseUnsupported,
		},
		{
			name:    "root-target",
			start:   "link",
			entries: entryMap(entrySnapshot{path: "link", kind: entrySymlink, linkText: "."}),
			cause:   CauseUnsupported,
		},
		{
			name:  "missing-before-dotdot",
			start: "link",
			entries: entryMap(
				entrySnapshot{path: "link", kind: entrySymlink, linkText: "missing/../target"},
				entrySnapshot{path: "target", kind: entryRegular},
			),
			cause: CauseNotFound,
		},
		{
			name:  "regular-before-dotdot",
			start: "link",
			entries: entryMap(
				entrySnapshot{path: "link", kind: entrySymlink, linkText: "regular/../target"},
				entrySnapshot{path: "regular", kind: entryRegular},
				entrySnapshot{path: "target", kind: entryRegular},
			),
			cause: CauseUnsupported,
		},
		{
			name:  "self-before-dotdot",
			start: "link",
			entries: entryMap(
				entrySnapshot{path: "link", kind: entrySymlink, linkText: "link/../target"},
				entrySnapshot{path: "target", kind: entryRegular},
			),
			cause: CauseMalformed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := resolveContainedTarget(test.start, test.entries)
			requirePrivateCauses(t, err, test.cause)
		})
	}
}

func TestSymlinkResolutionProcessesCapturedComponents(t *testing.T) {
	t.Parallel()

	entries := entryMap(
		entrySnapshot{path: "directory", kind: entryDirectory},
		entrySnapshot{path: "directory/link", kind: entrySymlink, linkText: "child/../target"},
		entrySnapshot{path: "directory/child", kind: entryDirectory},
		entrySnapshot{path: "directory/target", kind: entryRegular},
	)
	got, err := resolveContainedTarget("directory/link", entries)
	if err != nil || got != "directory/target" {
		t.Fatalf("component-wise resolution = (%q, %v), want (directory/target, nil)", got, err)
	}
}

func TestFullEntryEqualityIncludesSessionIdentity(t *testing.T) {
	t.Parallel()

	left := []entrySnapshot{{
		path: "file",
		kind: entryRegular,
		file: fileSnapshot{
			identity: FileIdentity{Device: 1, Inode: 2, UID: 501, Mode: 0o100600},
			size:     3,
		},
		content: sha256.Sum256([]byte("abc")),
	}}
	right := append([]entrySnapshot(nil), left...)
	if !equalEntrySnapshots(left, right) {
		t.Fatal("identical entry snapshots differ")
	}
	right[0].file.identity.Inode++
	if equalEntrySnapshots(left, right) {
		t.Fatal("replacement identity was treated as equal")
	}
}

func TestAuthorityPathClaimRetainsStableAndExcludesVolatileDirectoryMetadata(t *testing.T) {
	t.Parallel()

	snapshot := fileSnapshot{
		identity: FileIdentity{
			Device:     1,
			Inode:      2,
			UID:        501,
			Mode:       0o40700,
			Filesystem: [2]int32{3, 4},
		},
		linkCount:  5,
		gid:        20,
		rdev:       6,
		size:       7,
		mtimeSec:   8,
		mtimeNsec:  9,
		ctimeSec:   10,
		ctimeNsec:  11,
		birthSec:   12,
		birthNsec:  13,
		flags:      14,
		generation: 15,
	}
	mount := mountSnapshot{filesystem: [2]int32{16, 17}, flags: 18}
	aclDigest := sequentialDigest(0x80)
	want := makeAuthorityPathClaim(snapshot, mount, aclDigest)

	stableMutations := []struct {
		name   string
		mutate func(*fileSnapshot)
	}{
		{name: "gid", mutate: func(value *fileSnapshot) { value.gid++ }},
		{name: "rdev", mutate: func(value *fileSnapshot) { value.rdev++ }},
		{name: "birth-second", mutate: func(value *fileSnapshot) { value.birthSec++ }},
		{name: "birth-nanosecond", mutate: func(value *fileSnapshot) { value.birthNsec++ }},
		{name: "bsd-flags", mutate: func(value *fileSnapshot) { value.flags++ }},
		{name: "generation", mutate: func(value *fileSnapshot) { value.generation++ }},
	}
	for _, test := range stableMutations {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			changed := snapshot
			test.mutate(&changed)
			got := makeAuthorityPathClaim(changed, mount, aclDigest)
			if got == want {
				t.Fatal("stable authority-path metadata change was omitted")
			}
			requirePrivateCauses(t, compareAuthorityPathClaims([]authorityPathClaim{want}, []authorityPathClaim{got}), CauseUnstable)
		})
	}

	volatileMutations := []struct {
		name   string
		mutate func(*fileSnapshot)
	}{
		{name: "link-count", mutate: func(value *fileSnapshot) { value.linkCount++ }},
		{name: "size", mutate: func(value *fileSnapshot) { value.size++ }},
		{name: "modification-second", mutate: func(value *fileSnapshot) { value.mtimeSec++ }},
		{name: "modification-nanosecond", mutate: func(value *fileSnapshot) { value.mtimeNsec++ }},
		{name: "change-second", mutate: func(value *fileSnapshot) { value.ctimeSec++ }},
		{name: "change-nanosecond", mutate: func(value *fileSnapshot) { value.ctimeNsec++ }},
	}
	for _, test := range volatileMutations {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			changed := snapshot
			test.mutate(&changed)
			got := makeAuthorityPathClaim(changed, mount, aclDigest)
			if got != want {
				t.Fatal("volatile directory metadata entered authority-path claim")
			}
			if err := compareAuthorityPathClaims([]authorityPathClaim{want}, []authorityPathClaim{got}); err != nil {
				t.Fatalf("volatile directory metadata invalidated path claim: %v", err)
			}
		})
	}
}

func TestOpenedEntryMountDeviceOwnerAndModePolicy(t *testing.T) {
	t.Parallel()

	uid := uint32(os.Geteuid())
	root := &retainedDirectory{
		snapshot: fileSnapshot{identity: FileIdentity{Device: 7, UID: uid, Mode: 0o40700}},
		mount:    mountSnapshot{filesystem: [2]int32{1, 2}, flags: 3},
	}
	valid := fileSnapshot{identity: FileIdentity{Device: 7, UID: uid, Mode: 0o100600}}
	if err := validateOpenedEntry(root, valid, root.mount, entryRegular, ownerEffectiveOnly); err != nil {
		t.Fatalf("valid entry refused: %v", err)
	}

	tests := []struct {
		name     string
		snapshot fileSnapshot
		mount    mountSnapshot
		cause    CauseCode
	}{
		{name: "filesystem", snapshot: valid, mount: mountSnapshot{filesystem: [2]int32{1, 9}, flags: 3}, cause: CauseUnsupported},
		{name: "mount-flags", snapshot: valid, mount: mountSnapshot{filesystem: [2]int32{1, 2}, flags: 4}, cause: CauseUnsupported},
		{name: "device", snapshot: fileSnapshot{identity: FileIdentity{Device: 8, UID: uid, Mode: 0o100600}}, mount: root.mount, cause: CauseUnsupported},
		{name: "owner", snapshot: fileSnapshot{identity: FileIdentity{Device: 7, UID: uid + 1, Mode: 0o100600}}, mount: root.mount, cause: CausePermission},
		{name: "mode", snapshot: fileSnapshot{identity: FileIdentity{Device: 7, UID: uid, Mode: 0o100620}}, mount: root.mount, cause: CausePermission},
		{name: "kind", snapshot: fileSnapshot{identity: FileIdentity{Device: 7, UID: uid, Mode: 0o40700}}, mount: root.mount, cause: CauseIdentity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			requirePrivateCauses(t, validateOpenedEntry(root, test.snapshot, test.mount, entryRegular, ownerEffectiveOnly), test.cause)
		})
	}
}

func TestHashBoundedReaderObservesCancellationDuringRead(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterFirstRead{cancel: cancel}
	_, err := hashBoundedReader(ctx, reader, 2, 2)
	requirePrivateCauses(t, err, CauseCanceled)
}

func TestStateRootContextRefusesAlreadyCanceledAdmission(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root, err := admitStateRootContext(ctx, "/state-root-not-opened-after-cancellation")
	if root != nil {
		_ = root.close()
		t.Fatal("already-canceled StateRoot admission returned a retained root")
	}
	requirePrivateCauses(t, err, CauseCanceled)
}

func TestFilesystemCompoundFailurePreservesCloseCause(t *testing.T) {
	t.Parallel()

	joined := joinFilesystemFailures(
		fail(CauseUnstable, "primary"),
		closeDescriptorFailure(fmt.Errorf("private close detail")),
	)
	requirePrivateCauses(t, joined, CauseUnstable, CauseDescriptorClose)

	wrapped := failWithFilesystemCauses(joined, CauseIdentity, "outer")
	requirePrivateCauses(t, wrapped, CauseIdentity, CauseUnstable, CauseDescriptorClose)

	rawJoin := errors.Join(
		fail(CauseNotFound, "first branch"),
		closeDescriptorFailure(fmt.Errorf("second branch")),
		fail(CauseCleanup, "third branch"),
	)
	wrapped = failWithFilesystemCauses(rawJoin, CauseIdentity, "outer raw join")
	requirePrivateCauses(t, wrapped, CauseIdentity, CauseNotFound, CauseDescriptorClose, CauseCleanup)
}

func TestUnsupportedPlatformFilesystemSeams(t *testing.T) {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		t.Skip("supported filesystem implementation is active")
	}

	checks := []struct {
		name string
		run  func() error
	}{
		{
			name: "absolute-open",
			run: func() error {
				_, err := openAbsoluteNoFollow("/authority", entryDirectory)
				return err
			},
		},
		{
			name: "relative-open",
			run: func() error {
				_, err := openRelativeNoFollow(-1, "authority", entryDirectory)
				return err
			},
		},
		{
			name: "relative-kind",
			run: func() error {
				_, err := relativeEntryKindNoFollow(-1, "authority")
				return err
			},
		},
		{
			name: "symlink-read",
			run: func() error {
				_, err := readOpenedSymlink(nil, maxSymlinkBytes)
				return err
			},
		},
		{
			name: "descriptor-inspection",
			run: func() error {
				_, _, _, err := inspectDescriptor(nil)
				return err
			},
		},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			requirePrivateCauses(t, check.run(), CauseUnsupported)
		})
	}
}

type cancelAfterFirstRead struct {
	cancel context.CancelFunc
	read   bool
}

func (reader *cancelAfterFirstRead) Read(buffer []byte) (int, error) {
	if reader.read {
		buffer[0] = 'b'
		return 1, nil
	}
	reader.read = true
	buffer[0] = 'a'
	reader.cancel()
	return 1, nil
}

func manifestFixture(t *testing.T) (*retainedDirectory, []entrySnapshot) {
	t.Helper()

	root := &retainedDirectory{
		snapshot: fileSnapshot{
			identity: FileIdentity{Mode: 0o40755, UID: 501},
			gid:      20,
			size:     96,
		},
		mount:     mountSnapshot{filesystem: [2]int32{-7, 9}, flags: 0xa1b2c3d4},
		aclDigest: sequentialDigest(0x00),
	}
	entries := []entrySnapshot{
		{
			path:      "\x01control",
			kind:      entryDirectory,
			file:      fileSnapshot{identity: FileIdentity{Mode: 0o40555, UID: 0}, size: 64},
			aclDigest: sequentialDigest(0x20),
		},
		{
			path:      "line\nname",
			kind:      entryRegular,
			file:      fileSnapshot{identity: FileIdentity{Mode: 0o100555, UID: 501}, gid: 20, size: 3},
			aclDigest: sequentialDigest(0x40),
			content:   sha256.Sum256([]byte("abc")),
		},
		{
			path:      "zz-link",
			kind:      entrySymlink,
			file:      fileSnapshot{identity: FileIdentity{Mode: 0o120555, UID: 0}, size: 9},
			aclDigest: sequentialDigest(0x60),
			linkText:  "line\nname",
		},
	}
	if err := bindSymlinkTargets(entries); err != nil {
		t.Fatalf("bindSymlinkTargets() = %v", err)
	}
	return root, entries
}

func independentlyEncodeManifest(domain string, root *retainedDirectory, entries []entrySnapshot) []byte {
	wire := appendTestBytes(nil, []byte("buildauthority-manifest/v1"))
	wire = appendTestBytes(wire, []byte(domain))
	rootRecord := independentlyEncodeRoot(root)
	wire = appendTestBytes(wire, rootRecord)
	wire = appendTestUint64(wire, uint64(len(entries)))
	for _, entry := range entries {
		wire = appendTestBytes(wire, independentlyEncodeEntry(entry))
	}
	return wire
}

func independentlyEncodeRoot(root *retainedDirectory) []byte {
	record := []byte{0}
	record = appendTestClaim(record, root.snapshot, root.aclDigest)
	record = appendTestUint32(record, uint32(root.mount.filesystem[0]))
	record = appendTestUint32(record, uint32(root.mount.filesystem[1]))
	record = appendTestUint32(record, root.mount.flags)
	return record
}

func independentlyEncodeEntry(entry entrySnapshot) []byte {
	record := appendTestBytes(nil, []byte(entry.path))
	record = append(record, byte(entry.kind))
	record = appendTestClaim(record, entry.file, entry.aclDigest)
	switch entry.kind {
	case entryRegular:
		record = append(record, entry.content[:]...)
	case entrySymlink:
		record = appendTestBytes(record, []byte(entry.linkText))
		record = append(record, entry.targetDigest[:]...)
	}
	return record
}

func appendTestClaim(buffer []byte, snapshot fileSnapshot, aclDigest Digest) []byte {
	buffer = appendTestUint32(buffer, snapshot.identity.Mode)
	buffer = appendTestUint32(buffer, snapshot.identity.UID)
	buffer = appendTestUint32(buffer, snapshot.gid)
	buffer = appendTestUint64(buffer, uint64(snapshot.size))
	return append(buffer, aclDigest[:]...)
}

func appendTestBytes(buffer, value []byte) []byte {
	buffer = appendTestUint64(buffer, uint64(len(value)))
	return append(buffer, value...)
}

func appendTestUint32(buffer []byte, value uint32) []byte {
	return binary.BigEndian.AppendUint32(buffer, value)
}

func appendTestUint64(buffer []byte, value uint64) []byte {
	return binary.BigEndian.AppendUint64(buffer, value)
}

func sequentialDigest(start byte) Digest {
	var digest Digest
	for index := range digest {
		digest[index] = start + byte(index)
	}
	return digest
}

func symlinkChain(depth int) map[string]*entrySnapshot {
	entries := entryMap(entrySnapshot{path: "target", kind: entryRegular})
	for index := depth - 1; index >= 0; index-- {
		name := fmt.Sprintf("link-%03d", index)
		target := "target"
		if index+1 < depth {
			target = fmt.Sprintf("link-%03d", index+1)
		}
		entry := entrySnapshot{path: name, kind: entrySymlink, linkText: target}
		entries[name] = &entry
	}
	return entries
}

func entryMap(entries ...entrySnapshot) map[string]*entrySnapshot {
	result := make(map[string]*entrySnapshot, len(entries))
	for index := range entries {
		entry := entries[index]
		result[entry.path] = &entry
	}
	return result
}

func requirePrivateCauses(t *testing.T, err error, want ...CauseCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want causes %#v", want)
	}
	got := privateCauses(err, CauseInternalInvariant)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("causes = %#v, want %#v", got, want)
	}
}

func TestIndependentManifestEncoderUsesFixedDomainLength(t *testing.T) {
	t.Parallel()

	if got := len([]byte("buildauthority-manifest/v1")); got != 26 {
		t.Fatalf("manifest format domain length = %d, want 26", got)
	}
	root, entries := manifestFixture(t)
	wire := independentlyEncodeManifest(domainGOROOT, root, entries)
	if !bytes.Equal(wire[8:34], []byte("buildauthority-manifest/v1")) {
		t.Fatalf("manifest format prefix = %x", wire[:34])
	}
}

package buildauthority

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
)

type authorityBracketFixture struct {
	physicalRoot *physicalRootAuthority
	nullDevice   *retainedNullDevice
	goroot       *treeCapture
	goRole       *goAuthority
	compilerRole *compilerAuthority
	gitRole      *gitAuthority

	rootHandle        *os.Root
	retained          map[*os.File]bool
	labels            map[*os.File]string
	observed          map[string]authorityDescriptorObservation
	children          map[string]string
	typedChildren     map[string]string
	kinds             map[string]entryKind
	directoryEntries  map[string][]string
	hashes            map[string]Digest
	links             map[string]string
	presentSentinels  map[string]bool
	closed            map[*os.File]int
	transient         []*os.File
	calls             map[string]int
	events            []string
	failures          map[string]*authorityPrimitiveFailure
	aclPolicyFailures map[string]*authorityPrimitiveFailure
	closeFails        map[string]bool
	hooks             map[string]func()
}

func newAuthorityBracketFixture(t *testing.T) *authorityBracketFixture {
	t.Helper()
	mount := mountSnapshot{filesystem: [2]int32{17, 19}, flags: 1}
	rootSnapshot := fileSnapshot{identity: FileIdentity{
		Device:     1,
		Inode:      1,
		UID:        0,
		Mode:       platformModeDirectory | 0o755,
		Filesystem: mount.filesystem,
	}}
	rootACL := Digest{0x11}
	rootClaim := makeAuthorityPathClaim(rootSnapshot, mount, rootACL)
	rootDescriptor := new(os.File)
	rootHandle := new(os.Root)
	physicalRoot := &physicalRootAuthority{
		directory: &retainedDirectory{
			path:       physicalRootPath,
			root:       rootHandle,
			descriptor: rootDescriptor,
			snapshot:   rootSnapshot,
			mount:      mount,
			aclDigest:  rootACL,
			pathClaims: []authorityPathClaim{rootClaim},
		},
		claim: rootClaim,
	}
	devSnapshot := fileSnapshot{identity: FileIdentity{
		Device:     1,
		Inode:      2,
		UID:        0,
		Mode:       platformModeDirectory | 0o755,
		Filesystem: mount.filesystem,
	}}
	devACL := Digest{0x22}
	devClaim := makeAuthorityPathClaim(devSnapshot, mount, devACL)
	nullSnapshot := fileSnapshot{
		identity: FileIdentity{
			Device:     1,
			Inode:      3,
			UID:        0,
			Mode:       uint32(0x2000) | 0o666,
			Filesystem: mount.filesystem,
		},
		linkCount: 1,
		rdev:      uint64(3)<<24 | 2,
	}
	nullACL := Digest{0x33}
	nullDescriptor := new(os.File)
	nullDevice := &retainedNullDevice{leaf: &retainedLeaf{
		path:       nullDevicePath,
		descriptor: nullDescriptor,
		snapshot:   nullSnapshot,
		mount:      mount,
		aclDigest:  nullACL,
		pathClaims: []authorityPathClaim{rootClaim, devClaim},
	}}
	uid := uint32(os.Geteuid())
	directorySnapshot := func(inode uint64) fileSnapshot {
		return fileSnapshot{identity: FileIdentity{
			Device:     1,
			Inode:      inode,
			UID:        uid,
			Mode:       platformModeDirectory | 0o755,
			Filesystem: mount.filesystem,
		}}
	}
	regularSnapshot := func(inode uint64) fileSnapshot {
		return fileSnapshot{
			identity: FileIdentity{
				Device:     1,
				Inode:      inode,
				UID:        uid,
				Mode:       platformModeRegular | 0o755,
				Filesystem: mount.filesystem,
			},
			size: 64,
		}
	}
	toolchainSnapshot := directorySnapshot(10)
	binSnapshot := directorySnapshot(11)
	pkgSnapshot := directorySnapshot(12)
	pkgToolSnapshot := directorySnapshot(13)
	toolDirectorySnapshot := directorySnapshot(14)
	toolsSnapshot := directorySnapshot(15)
	goSnapshot := regularSnapshot(20)
	compilerSnapshot := regularSnapshot(21)
	gitSnapshot := regularSnapshot(22)
	goEnvironmentSnapshot := regularSnapshot(23)
	toolchainACL := Digest{0x40}
	binACL := Digest{0x41}
	pkgACL := Digest{0x42}
	pkgToolACL := Digest{0x43}
	toolDirectoryACL := Digest{0x44}
	toolsACL := Digest{0x45}
	goACL := Digest{0x50}
	compilerACL := Digest{0x51}
	gitACL := Digest{0x52}
	goEnvironmentACL := Digest{0x53}
	toolchainClaim := makeAuthorityPathClaim(toolchainSnapshot, mount, toolchainACL)
	binClaim := makeAuthorityPathClaim(binSnapshot, mount, binACL)
	pkgClaim := makeAuthorityPathClaim(pkgSnapshot, mount, pkgACL)
	pkgToolClaim := makeAuthorityPathClaim(pkgToolSnapshot, mount, pkgToolACL)
	toolDirectoryClaim := makeAuthorityPathClaim(toolDirectorySnapshot, mount, toolDirectoryACL)
	toolsClaim := makeAuthorityPathClaim(toolsSnapshot, mount, toolsACL)
	goDigest := goExecutableDigest()
	compilerDigest := Digest{0x61}
	gitDigest := gitExecutableDigest()
	goEnvironmentDigest := Digest{0x62}
	gorootDescriptor := new(os.File)
	gorootHandle := new(os.Root)
	goDescriptor := new(os.File)
	compilerDescriptor := new(os.File)
	gitDescriptor := new(os.File)
	goroot := &treeCapture{
		root: &retainedDirectory{
			path:       "/toolchain",
			root:       gorootHandle,
			descriptor: gorootDescriptor,
			snapshot:   toolchainSnapshot,
			mount:      mount,
			aclDigest:  toolchainACL,
			pathClaims: []authorityPathClaim{rootClaim, toolchainClaim},
		},
		policy: gorootPolicy(),
		digest: Digest{0x70},
		entries: []entrySnapshot{
			{path: "bin", kind: entryDirectory, file: binSnapshot, aclDigest: binACL},
			{path: "bin/go", kind: entryRegular, file: goSnapshot, aclDigest: goACL, content: goDigest},
			{path: goEnvironmentFileName, kind: entryRegular, file: goEnvironmentSnapshot, aclDigest: goEnvironmentACL, content: goEnvironmentDigest},
			{path: "pkg", kind: entryDirectory, file: pkgSnapshot, aclDigest: pkgACL},
			{path: "pkg/tool", kind: entryDirectory, file: pkgToolSnapshot, aclDigest: pkgToolACL},
			{path: "pkg/tool/darwin_arm64", kind: entryDirectory, file: toolDirectorySnapshot, aclDigest: toolDirectoryACL},
			{path: compilerGOROOTRelativePath, kind: entryRegular, file: compilerSnapshot, aclDigest: compilerACL, content: compilerDigest},
		},
	}
	goRetained := &retainedExecutable{
		leaf: &retainedLeaf{
			path:       "/toolchain/" + goGOROOTRelativePath,
			descriptor: goDescriptor,
			snapshot:   goSnapshot,
			mount:      mount,
			aclDigest:  goACL,
			pathClaims: []authorityPathClaim{rootClaim, toolchainClaim, binClaim},
		},
		digest: goDigest,
	}
	compilerRetained := &retainedExecutable{
		leaf: &retainedLeaf{
			path:       "/toolchain/" + compilerGOROOTRelativePath,
			descriptor: compilerDescriptor,
			snapshot:   compilerSnapshot,
			mount:      mount,
			aclDigest:  compilerACL,
			pathClaims: []authorityPathClaim{
				rootClaim,
				toolchainClaim,
				pkgClaim,
				pkgToolClaim,
				toolDirectoryClaim,
			},
		},
		digest: compilerDigest,
	}
	gitRetained := &retainedExecutable{
		leaf: &retainedLeaf{
			path:       "/tools/git",
			descriptor: gitDescriptor,
			snapshot:   gitSnapshot,
			mount:      mount,
			aclDigest:  gitACL,
			pathClaims: []authorityPathClaim{rootClaim, toolsClaim},
		},
		digest: gitDigest,
	}
	fixture := &authorityBracketFixture{
		physicalRoot: physicalRoot,
		nullDevice:   nullDevice,
		goroot:       goroot,
		goRole:       &goAuthority{retained: goRetained, seal: validGoAuthority},
		compilerRole: &compilerAuthority{retained: compilerRetained, seal: validCompilerAuthority},
		gitRole:      &gitAuthority{retained: gitRetained, seal: validGitAuthority},
		rootHandle:   rootHandle,
		retained: map[*os.File]bool{
			rootDescriptor:     true,
			nullDescriptor:     true,
			gorootDescriptor:   true,
			goDescriptor:       true,
			compilerDescriptor: true,
			gitDescriptor:      true,
		},
		labels: map[*os.File]string{
			rootDescriptor:     "root",
			nullDescriptor:     "retained-null",
			gorootDescriptor:   "retained-goroot",
			goDescriptor:       "retained-go",
			compilerDescriptor: "retained-compiler",
			gitDescriptor:      "retained-git",
		},
		observed: map[string]authorityDescriptorObservation{
			"root":               {snapshot: rootSnapshot, mount: mount, aclDigest: rootACL},
			"root-handle":        {snapshot: rootSnapshot, mount: mount, aclDigest: rootACL},
			"absolute-root":      {snapshot: rootSnapshot, mount: mount, aclDigest: rootACL},
			"dev":                {snapshot: devSnapshot, mount: mount, aclDigest: devACL},
			"retained-null":      {snapshot: nullSnapshot, mount: mount, aclDigest: nullACL},
			"fresh-null":         {snapshot: nullSnapshot, mount: mount, aclDigest: nullACL},
			"retained-goroot":    {snapshot: toolchainSnapshot, mount: mount, aclDigest: toolchainACL},
			"goroot-root-handle": {snapshot: toolchainSnapshot, mount: mount, aclDigest: toolchainACL},
			"path:toolchain":     {snapshot: toolchainSnapshot, mount: mount, aclDigest: toolchainACL},
			"path:tools":         {snapshot: toolsSnapshot, mount: mount, aclDigest: toolsACL},
			"tree:goroot":        {snapshot: toolchainSnapshot, mount: mount, aclDigest: toolchainACL},
			"tree:bin":           {snapshot: binSnapshot, mount: mount, aclDigest: binACL},
			"tree:go":            {snapshot: goSnapshot, mount: mount, aclDigest: goACL},
			"tree:go.env":        {snapshot: goEnvironmentSnapshot, mount: mount, aclDigest: goEnvironmentACL},
			"tree:pkg":           {snapshot: pkgSnapshot, mount: mount, aclDigest: pkgACL},
			"tree:pkg-tool":      {snapshot: pkgToolSnapshot, mount: mount, aclDigest: pkgToolACL},
			"tree:tool-dir":      {snapshot: toolDirectorySnapshot, mount: mount, aclDigest: toolDirectoryACL},
			"tree:compiler":      {snapshot: compilerSnapshot, mount: mount, aclDigest: compilerACL},
			"retained-go":        {snapshot: goSnapshot, mount: mount, aclDigest: goACL},
			"retained-compiler":  {snapshot: compilerSnapshot, mount: mount, aclDigest: compilerACL},
			"retained-git":       {snapshot: gitSnapshot, mount: mount, aclDigest: gitACL},
			"fresh-git":          {snapshot: gitSnapshot, mount: mount, aclDigest: gitACL},
		},
		children: map[string]string{
			"dev/null": "fresh-null",
		},
		typedChildren: map[string]string{
			"root/dev":                   "dev",
			"root/toolchain":             "path:toolchain",
			"root/tools":                 "path:tools",
			"path:toolchain/bin":         "tree:bin",
			"path:toolchain/go.env":      "tree:go.env",
			"path:toolchain/pkg":         "tree:pkg",
			"tree:bin/go":                "tree:go",
			"tree:pkg/tool":              "tree:pkg-tool",
			"tree:pkg-tool/darwin_arm64": "tree:tool-dir",
			"tree:tool-dir/compile":      "tree:compiler",
			"path:tools/git":             "fresh-git",
		},
		kinds: map[string]entryKind{
			"root/dev":                   entryDirectory,
			"root/toolchain":             entryDirectory,
			"root/tools":                 entryDirectory,
			"path:toolchain/bin":         entryDirectory,
			"path:toolchain/go.env":      entryRegular,
			"path:toolchain/pkg":         entryDirectory,
			"tree:bin/go":                entryRegular,
			"tree:pkg/tool":              entryDirectory,
			"tree:pkg-tool/darwin_arm64": entryDirectory,
			"tree:tool-dir/compile":      entryRegular,
		},
		directoryEntries: map[string][]string{
			"path:toolchain": {"bin", "go.env", "pkg"},
			"tree:bin":       {"go"},
			"tree:pkg":       {"tool"},
			"tree:pkg-tool":  {"darwin_arm64"},
			"tree:tool-dir":  {"compile"},
		},
		hashes: map[string]Digest{
			"tree:go":           goDigest,
			"tree:go.env":       goEnvironmentDigest,
			"tree:compiler":     compilerDigest,
			"retained-go":       goDigest,
			"retained-compiler": compilerDigest,
			"retained-git":      gitDigest,
			"fresh-git":         gitDigest,
		},
		links:            make(map[string]string),
		presentSentinels: make(map[string]bool),
	}
	fixture.resetTrace()
	return fixture
}

func (fixture *authorityBracketFixture) resetTrace() {
	fixture.closed = make(map[*os.File]int)
	fixture.transient = nil
	fixture.calls = make(map[string]int)
	fixture.events = nil
	fixture.failures = make(map[string]*authorityPrimitiveFailure)
	fixture.aclPolicyFailures = make(map[string]*authorityPrimitiveFailure)
	fixture.closeFails = make(map[string]bool)
	fixture.hooks = make(map[string]func())
}

func (fixture *authorityBracketFixture) addGOROOTSymlink(t *testing.T) {
	t.Helper()
	const (
		label  = "tree:alias"
		name   = "alias"
		target = "bin/go"
	)
	var targetEntry entrySnapshot
	for _, entry := range fixture.goroot.entries {
		if entry.path == target {
			targetEntry = entry
			break
		}
	}
	if targetEntry.path == "" {
		t.Fatal("fixture GOROOT has no symlink target row")
	}
	snapshot := fileSnapshot{
		identity: FileIdentity{
			Device:     fixture.goroot.root.snapshot.identity.Device,
			Inode:      24,
			UID:        uint32(os.Geteuid()),
			Mode:       platformModeSymlink | 0o755,
			Filesystem: fixture.goroot.root.mount.filesystem,
		},
		size: int64(len(target)),
	}
	acl := Digest{0x54}
	fixture.goroot.entries = append(fixture.goroot.entries, entrySnapshot{
		path:         name,
		kind:         entrySymlink,
		file:         snapshot,
		aclDigest:    acl,
		linkText:     target,
		targetDigest: digestEntryClaim(targetEntry),
	})
	sortEntrySnapshots(fixture.goroot.entries)
	fixture.directoryEntries["path:toolchain"] = []string{name, "bin", "go.env", "pkg"}
	fixture.kinds["path:toolchain/"+name] = entrySymlink
	fixture.typedChildren["path:toolchain/"+name] = label
	fixture.observed[label] = authorityDescriptorObservation{
		snapshot:  snapshot,
		mount:     fixture.goroot.root.mount,
		aclDigest: acl,
	}
	fixture.links[label] = target
}

func (fixture *authorityBracketFixture) step(base string) string {
	fixture.calls[base]++
	key := fmt.Sprintf("%s#%d", base, fixture.calls[base])
	fixture.events = append(fixture.events, key)
	if hook := fixture.hooks[key]; hook != nil {
		hook()
	}
	return key
}

func (fixture *authorityBracketFixture) acquire(
	label string,
	key string,
) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
	descriptor := new(os.File)
	fixture.labels[descriptor] = label
	fixture.transient = append(fixture.transient, descriptor)
	acquisition := &authorityDescriptorAcquisition{
		file: descriptor,
		close: func() error {
			closeKey := fixture.step("close:" + label)
			fixture.closed[descriptor]++
			if fixture.closeFails[closeKey] {
				return errors.New("injected descriptor close failure")
			}
			return nil
		},
	}
	return acquisition, fixture.failures[key]
}

func (fixture *authorityBracketFixture) primitives() preflightAuthorityPrimitives {
	return preflightAuthorityPrimitives{
		openRelativeNoFollow: func(
			_ context.Context,
			parent *os.File,
			name string,
			mode authorityRequiredOpenMode,
		) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
			parentLabel := fixture.labels[parent]
			base := fmt.Sprintf(
				"open:%s/%s:%s",
				parentLabel,
				name,
				authorityOpenModeName(mode),
			)
			key := fixture.step(base)
			label, ok := fixture.children[parentLabel+"/"+name]
			if !ok {
				return nil, newAuthorityPrimitiveFailure(
					OperationValidate,
					CauseInternalInvariant,
				)
			}
			return fixture.acquire(label, key)
		},
		statDescriptor: func(
			_ context.Context,
			descriptor *os.File,
		) (fileSnapshot, *authorityPrimitiveFailure) {
			label := fixture.labels[descriptor]
			key := fixture.step("stat:" + label)
			if failure := fixture.failures[key]; failure != nil {
				return fileSnapshot{}, failure
			}
			observation, ok := fixture.observed[label]
			if !ok {
				return fileSnapshot{}, newAuthorityPrimitiveFailure(
					OperationValidate,
					CauseInternalInvariant,
				)
			}
			snapshot := observation.snapshot
			snapshot.identity.Filesystem = [2]int32{}
			return snapshot, nil
		},
		statFilesystem: func(
			_ context.Context,
			descriptor *os.File,
		) (mountSnapshot, *authorityPrimitiveFailure) {
			label := fixture.labels[descriptor]
			key := fixture.step("mount:" + label)
			if failure := fixture.failures[key]; failure != nil {
				return mountSnapshot{}, failure
			}
			observation, ok := fixture.observed[label]
			if !ok {
				return mountSnapshot{}, newAuthorityPrimitiveFailure(
					OperationValidate,
					CauseInternalInvariant,
				)
			}
			return observation.mount, nil
		},
		validateFilesystem: func(
			_ context.Context,
			_ mountSnapshot,
		) *authorityPrimitiveFailure {
			key := fixture.step("validate-mount")
			return fixture.failures[key]
		},
		acquireRawACL: func(
			_ context.Context,
			descriptor *os.File,
		) ([]byte, *authorityPrimitiveFailure) {
			label := fixture.labels[descriptor]
			key := fixture.step("acl:" + label)
			if failure := fixture.failures[key]; failure != nil {
				return nil, failure
			}
			observation, ok := fixture.observed[label]
			if !ok {
				return nil, newAuthorityPrimitiveFailure(
					OperationValidate,
					CauseInternalInvariant,
				)
			}
			return []byte{observation.aclDigest[0]}, nil
		},
		parseRawACL: func(
			_ context.Context,
			raw []byte,
		) (Digest, *authorityPrimitiveFailure) {
			key := fixture.step("parse-acl")
			if failure := fixture.failures[key]; failure != nil {
				return Digest{}, failure
			}
			if len(raw) != 1 {
				return Digest{}, newAuthorityPrimitiveFailure(OperationParse, CauseMalformed)
			}
			return Digest{raw[0]}, nil
		},
		readExactForParse: func(
			context.Context,
			io.ReaderAt,
			[]byte,
		) *authorityPrimitiveFailure {
			return nil
		},
		hashBytes: func(
			context.Context,
			[]byte,
		) (Digest, *authorityPrimitiveFailure) {
			return Digest{}, nil
		},
		brackets: fixture.bracketPrimitives(),
	}
}

func (fixture *authorityBracketFixture) bracketPrimitives() preflightAuthorityBracketPrimitives {
	return preflightAuthorityBracketPrimitives{
		probeSentinelAbsence: func(
			_ context.Context,
			parent *os.File,
			name string,
			mode authoritySentinelAbsenceMode,
		) *authorityPrimitiveFailure {
			modeName := "invalid"
			switch mode {
			case authorityInitialSentinelAbsence:
				modeName = "initial"
			case authorityExpectedSentinelAbsence:
				modeName = "expected"
			}
			key := fixture.step(fmt.Sprintf("sentinel:%s:%s", modeName, name))
			if fixture.labels[parent] != "root" || !mode.valid() {
				return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			if failure := fixture.failures[key]; failure != nil {
				return failure
			}
			if !fixture.presentSentinels[name] {
				return nil
			}
			if mode == authorityInitialSentinelAbsence {
				return newAuthorityPrimitiveFailure(OperationValidate, CauseUnsupported)
			}
			return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
		},
		openAbsoluteRoot: func(
			context.Context,
		) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
			key := fixture.step("open-absolute-root")
			return fixture.acquire("absolute-root", key)
		},
		openRootHandle: func(
			_ context.Context,
			root *os.Root,
		) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
			key := fixture.step("open-root-handle")
			label := "root-handle"
			if root == fixture.goroot.root.root {
				label = "goroot-root-handle"
			} else if root != fixture.rootHandle {
				return nil, newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
			}
			return fixture.acquire(label, key)
		},
		openNullRelativeNoFollow: func(
			_ context.Context,
			parent *os.File,
			name string,
			_ authorityRequiredOpenMode,
		) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
			parentLabel := fixture.labels[parent]
			key := fixture.step("open-null:" + parentLabel + "/" + name)
			label, ok := fixture.children[parentLabel+"/"+name]
			if !ok {
				return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			return fixture.acquire(label, key)
		},
		probeRelativeKind: func(
			_ context.Context,
			parent *os.File,
			name string,
			_ authorityRequiredOpenMode,
		) (entryKind, *authorityPrimitiveFailure) {
			parentLabel := fixture.labels[parent]
			key := fixture.step("kind:" + parentLabel + "/" + name)
			if failure := fixture.failures[key]; failure != nil {
				return 0, failure
			}
			kind, ok := fixture.kinds[parentLabel+"/"+name]
			if !ok {
				return 0, newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
			}
			return kind, nil
		},
		openTypedRelativeNoFollow: func(
			_ context.Context,
			parent *os.File,
			name string,
			kind entryKind,
			_ authorityRequiredOpenMode,
		) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
			parentLabel := fixture.labels[parent]
			base := "open-typed:" + parentLabel + "/" + name
			key := fixture.step(base)
			label, ok := fixture.typedChildren[parentLabel+"/"+name]
			if !ok || fixture.kinds[parentLabel+"/"+name] != 0 &&
				fixture.kinds[parentLabel+"/"+name] != kind {
				return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			return fixture.acquire(label, key)
		},
		readDirectory: func(
			_ context.Context,
			descriptor *os.File,
			_ int,
		) ([]string, bool, *authorityPrimitiveFailure) {
			label := fixture.labels[descriptor]
			key := fixture.step("read-dir:" + label)
			if failure := fixture.failures[key]; failure != nil {
				return nil, false, failure
			}
			if fixture.calls["read-dir:"+label] == 1 {
				return append([]string(nil), fixture.directoryEntries[label]...), false, nil
			}
			return nil, true, nil
		},
		readSymlinkForWalk: func(
			_ context.Context,
			descriptor *os.File,
			_ int64,
			_ uint64,
		) (string, *authorityPrimitiveFailure) {
			label := fixture.labels[descriptor]
			key := fixture.step("read-link:" + label)
			return fixture.links[label], fixture.failures[key]
		},
		hashDescriptor: func(
			_ context.Context,
			reader io.ReaderAt,
			_ int64,
			_ uint64,
		) (Digest, *authorityPrimitiveFailure) {
			descriptor, ok := reader.(*os.File)
			if !ok {
				return Digest{}, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			label := fixture.labels[descriptor]
			key := fixture.step("hash:" + label)
			if failure := fixture.failures[key]; failure != nil {
				return Digest{}, failure
			}
			return fixture.hashes[label], nil
		},
		parseMachO: func(
			_ context.Context,
			reader io.ReaderAt,
			_ int64,
			profile machOProfile,
		) *authorityPrimitiveFailure {
			descriptor, ok := reader.(*os.File)
			if !ok {
				return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			key := fixture.step(fmt.Sprintf(
				"parse-macho:%s:%d",
				fixture.labels[descriptor],
				profile,
			))
			return fixture.failures[key]
		},
		hashManifest: func(
			_ context.Context,
			domain string,
			root *retainedDirectory,
			_ []entrySnapshot,
		) (Digest, *authorityPrimitiveFailure) {
			key := fixture.step("hash-manifest:" + domain)
			if failure := fixture.failures[key]; failure != nil {
				return Digest{}, failure
			}
			if root == nil || root.path != fixture.goroot.root.path {
				return Digest{}, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			return fixture.goroot.digest, nil
		},
		acquireRawNullACL: func(
			_ context.Context,
			descriptor *os.File,
		) ([]byte, *authorityPrimitiveFailure) {
			label := fixture.labels[descriptor]
			key := fixture.step("null-acl:" + label)
			if failure := fixture.failures[key]; failure != nil {
				return nil, failure
			}
			return []byte{fixture.observed[label].aclDigest[0]}, nil
		},
		parseRawACLForComparison: func(
			_ context.Context,
			raw []byte,
		) (authorityACLComparisonObservation, *authorityPrimitiveFailure) {
			key := fixture.step("parse-acl")
			if failure := fixture.failures[key]; failure != nil {
				return authorityACLComparisonObservation{}, failure
			}
			if len(raw) != 1 {
				return authorityACLComparisonObservation{}, newAuthorityPrimitiveFailure(
					OperationParse,
					CauseMalformed,
				)
			}
			return authorityACLComparisonObservation{
				digest:        Digest{raw[0]},
				policyFailure: fixture.aclPolicyFailures[key],
			}, nil
		},
		parseRawNullACLForComparison: func(
			_ context.Context,
			raw []byte,
		) (authorityACLComparisonObservation, *authorityPrimitiveFailure) {
			key := fixture.step("parse-null-acl")
			if failure := fixture.failures[key]; failure != nil {
				return authorityACLComparisonObservation{}, failure
			}
			if len(raw) != 1 {
				return authorityACLComparisonObservation{}, newAuthorityPrimitiveFailure(
					OperationParse,
					CauseMalformed,
				)
			}
			return authorityACLComparisonObservation{
				digest:        Digest{raw[0]},
				policyFailure: fixture.aclPolicyFailures[key],
			}, nil
		},
	}
}

func (fixture *authorityBracketFixture) revalidator(t *testing.T) *preflightAuthorityRevalidator {
	t.Helper()
	revalidator, failure := newPreflightAuthorityRevalidatorWith(fixture.primitives())
	if failure != nil {
		t.Fatalf("construct bracket revalidator: %+v", failure)
	}
	return revalidator
}

func (fixture *authorityBracketFixture) requireOwnership(t *testing.T) {
	t.Helper()
	for _, descriptor := range fixture.transient {
		if fixture.closed[descriptor] != 1 {
			t.Fatalf(
				"transient %q closed %d times, want once",
				fixture.labels[descriptor],
				fixture.closed[descriptor],
			)
		}
	}
	for descriptor := range fixture.retained {
		if fixture.closed[descriptor] != 0 {
			t.Fatalf(
				"borrowed retained %q closed %d times",
				fixture.labels[descriptor],
				fixture.closed[descriptor],
			)
		}
	}
}

func TestAuthorityBracketPhysicalRootAndNullCleanPathsCloseEveryTransient(t *testing.T) {
	fixture := newAuthorityBracketFixture(t)
	revalidator := fixture.revalidator(t)
	if outcome := revalidator.revalidatePhysicalRoot(
		context.Background(),
		fixture.physicalRoot,
	); !outcome.proved() {
		t.Fatalf("physical-root outcome = %+v", outcome)
	}
	fixture.requireOwnership(t)
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Log("retained-null static policy is supported only on darwin/arm64")
		return
	}

	fixture.resetTrace()
	if outcome := revalidator.revalidateRetainedNull(
		context.Background(),
		fixture.physicalRoot,
		fixture.nullDevice,
	); !outcome.proved() {
		t.Fatalf("retained-null outcome = %+v", outcome)
	}
	fixture.requireOwnership(t)
	if fixture.calls["open-typed:root/dev"] != 2 {
		t.Fatalf(
			"/dev pathname brackets = %d, want initial plus mandatory trailing",
			fixture.calls["open-typed:root/dev"],
		)
	}
}

func TestAuthorityBracketPrimitiveFailuresRetainTheirOperation(t *testing.T) {
	for _, test := range []struct {
		name      string
		key       string
		operation Operation
		cause     CauseCode
	}{
		{name: "descriptor probe", key: "stat:root#1", operation: OperationProbe, cause: CausePermission},
		{name: "mount probe", key: "mount:root#1", operation: OperationProbe, cause: CauseUnstable},
		{name: "filesystem policy", key: "validate-mount#1", operation: OperationValidate, cause: CauseUnsupported},
		{name: "ACL acquisition", key: "acl:root#1", operation: OperationProbe, cause: CausePermission},
		{name: "ACL parse", key: "parse-acl#1", operation: OperationParse, cause: CauseMalformed},
		{name: "root handle acquisition", key: "open-root-handle#1", operation: OperationOpen, cause: CauseNotFound},
		{name: "absolute root acquisition", key: "open-absolute-root#1", operation: OperationOpen, cause: CausePermission},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			fixture.failures[test.key] = newAuthorityPrimitiveFailure(test.operation, test.cause)
			outcome := fixture.revalidator(t).revalidatePhysicalRoot(
				context.Background(),
				fixture.physicalRoot,
			)
			requireAuthorityPrimitiveRecord(t, outcome.primary, test.operation, test.cause)
			if outcome.later != nil || outcome.descriptorClose != nil {
				t.Fatalf("unexpected secondary slots: %+v", outcome)
			}
			fixture.requireOwnership(t)
		})
	}
}

func TestAuthorityBracketPrimaryAndDescriptorCloseRemainIndependent(t *testing.T) {
	fixture := newAuthorityBracketFixture(t)
	fixture.failures["open-absolute-root#1"] = newAuthorityPrimitiveFailure(
		OperationOpen,
		CausePermission,
	)
	fixture.closeFails["close:absolute-root#1"] = true
	outcome := fixture.revalidator(t).revalidatePhysicalRoot(
		context.Background(),
		fixture.physicalRoot,
	)
	requireAuthorityPrimitiveRecord(t, outcome.primary, OperationOpen, CausePermission)
	if outcome.later != nil {
		t.Fatalf("direct bracket manufactured later = %+v", outcome.later)
	}
	if outcome.descriptorClose == nil ||
		outcome.descriptorClose.Phase != PhaseClose ||
		outcome.descriptorClose.Operation != OperationCloseNonRoot ||
		len(outcome.descriptorClose.Causes) != 1 ||
		outcome.descriptorClose.Causes[0] != CauseDescriptorClose {
		t.Fatalf("descriptor-close slot = %+v", outcome.descriptorClose)
	}
	fixture.requireOwnership(t)
}

func TestAuthorityBracketNullBindingDriftIsACompareFailure(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("retained-null static policy is supported only on darwin/arm64")
	}
	fixture := newAuthorityBracketFixture(t)
	observation := fixture.observed["fresh-null"]
	observation.snapshot.identity.Inode++
	fixture.observed["fresh-null"] = observation
	outcome := fixture.revalidator(t).revalidateRetainedNull(
		context.Background(),
		fixture.physicalRoot,
		fixture.nullDevice,
	)
	requireAuthorityPrimitiveRecord(t, outcome.primary, OperationCompare, CauseIdentity)
	if fixture.calls["open-typed:root/dev"] != 2 {
		t.Fatalf("failed null use skipped trailing pathname proof: %q", fixture.events)
	}
	fixture.requireOwnership(t)
}

func TestAuthorityBracketNullACLPrimitivesRetainTheirOperation(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("retained-null static policy is supported only on darwin/arm64")
	}
	for _, test := range []struct {
		name      string
		key       string
		operation Operation
		cause     CauseCode
	}{
		{name: "ACL acquisition", key: "null-acl:retained-null#1", operation: OperationProbe, cause: CausePermission},
		{name: "ACL parse", key: "parse-null-acl#1", operation: OperationParse, cause: CauseMalformed},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			fixture.failures[test.key] = newAuthorityPrimitiveFailure(test.operation, test.cause)
			outcome := fixture.revalidator(t).revalidateRetainedNull(
				context.Background(),
				fixture.physicalRoot,
				fixture.nullDevice,
			)
			requireAuthorityPrimitiveRecord(t, outcome.primary, test.operation, test.cause)
			fixture.requireOwnership(t)
		})
	}
}

func TestAuthorityBracketGOROOTCleanWalkOwnsEveryEntry(t *testing.T) {
	fixture := newAuthorityBracketFixture(t)
	outcome := fixture.revalidator(t).revalidateGOROOT(
		context.Background(),
		fixture.physicalRoot,
		fixture.goroot,
	)
	if !outcome.proved() {
		t.Fatalf("GOROOT outcome = %+v\nevents = %q", outcome, fixture.events)
	}
	fixture.requireOwnership(t)
	for _, label := range []string{
		"path:toolchain",
		"tree:bin",
		"tree:pkg",
		"tree:pkg-tool",
		"tree:tool-dir",
	} {
		if fixture.calls["read-dir:"+label] != 2 {
			t.Fatalf(
				"directory %q read calls = %d, want data batch plus terminal EOF",
				label,
				fixture.calls["read-dir:"+label],
			)
		}
	}
	if fixture.calls["open-typed:root/toolchain"] != 2 {
		t.Fatalf("GOROOT pathname brackets = %d, want 2", fixture.calls["open-typed:root/toolchain"])
	}
}

func TestAuthorityBracketGOROOTSymlinkReadAndTargetBinding(t *testing.T) {
	for _, test := range []struct {
		name  string
		drift bool
	}{
		{name: "clean"},
		{name: "target binding drift", drift: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			fixture.addGOROOTSymlink(t)
			if test.drift {
				for index := range fixture.goroot.entries {
					if fixture.goroot.entries[index].path == "alias" {
						fixture.goroot.entries[index].targetDigest[0] ^= 0xff
					}
				}
			}
			outcome := fixture.revalidator(t).revalidateGOROOT(
				context.Background(),
				fixture.physicalRoot,
				fixture.goroot,
			)
			if test.drift {
				requireAuthorityPrimitiveRecord(
					t,
					outcome.primary,
					OperationCompare,
					CauseUnstable,
				)
			} else if !outcome.proved() {
				t.Fatalf("clean symlink outcome = %+v\nevents = %q", outcome, fixture.events)
			}
			if fixture.calls["read-link:tree:alias"] != 1 {
				t.Fatalf("symlink reads = %d, want 1", fixture.calls["read-link:tree:alias"])
			}
			fixture.requireOwnership(t)
		})
	}
}

func TestAuthorityBracketNominalRolesUseTypedFreshLeaves(t *testing.T) {
	for _, test := range []struct {
		name       string
		freshLabel string
		pathOpen   string
		use        func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
	}{
		{
			name:       "Go",
			freshLabel: "tree:go",
			pathOpen:   "open-typed:root/toolchain",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.goRole,
				)
			},
		},
		{
			name:       "compiler",
			freshLabel: "tree:compiler",
			pathOpen:   "open-typed:root/toolchain",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateCompilerAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.compilerRole,
				)
			},
		},
		{
			name:       "Git",
			freshLabel: "fresh-git",
			pathOpen:   "open-typed:root/tools",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGitAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.gitRole,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			outcome := test.use(fixture.revalidator(t), fixture)
			if !outcome.proved() {
				t.Fatalf("%s outcome = %+v\nevents = %q", test.name, outcome, fixture.events)
			}
			fixture.requireOwnership(t)
			if fixture.calls[test.pathOpen] != 2 {
				t.Fatalf("%s parent pathname brackets = %d, want 2", test.name, fixture.calls[test.pathOpen])
			}
			if fixture.calls["hash:"+test.freshLabel] != 1 {
				t.Fatalf("%s fresh leaf hash calls = %d, want 1", test.name, fixture.calls["hash:"+test.freshLabel])
			}
		})
	}
}

func TestAuthorityBracketTreeAndRolePrimitiveAttribution(t *testing.T) {
	for _, test := range []struct {
		name      string
		key       string
		operation Operation
		cause     CauseCode
		prepare   func(*testing.T, *authorityBracketFixture)
		use       func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
	}{
		{
			name:      "directory walk",
			key:       "read-dir:path:toolchain#1",
			operation: OperationWalk,
			cause:     CauseUnstable,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:      "entry kind probe",
			key:       "kind:path:toolchain/bin#1",
			operation: OperationProbe,
			cause:     CausePermission,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:      "typed entry open",
			key:       "open-typed:path:toolchain/bin#1",
			operation: OperationOpen,
			cause:     CauseNotFound,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:      "entry hash",
			key:       "hash:tree:go#1",
			operation: OperationHash,
			cause:     CauseUnstable,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:      "symlink walk read",
			key:       "read-link:tree:alias#1",
			operation: OperationWalk,
			cause:     CauseUnstable,
			prepare: func(t *testing.T, fixture *authorityBracketFixture) {
				fixture.addGOROOTSymlink(t)
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:      "manifest hash",
			key:       "hash-manifest:" + domainGOROOT + "#1",
			operation: OperationHash,
			cause:     CauseLimit,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:      "Go Mach-O parse",
			key:       fmt.Sprintf("parse-macho:retained-go:%d#1", machOProfileGo),
			operation: OperationParse,
			cause:     CauseMalformed,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.goRole,
				)
			},
		},
		{
			name:      "compiler content hash",
			key:       "hash:retained-compiler#1",
			operation: OperationHash,
			cause:     CauseLimit,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateCompilerAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.compilerRole,
				)
			},
		},
		{
			name:      "Git Mach-O parse",
			key:       fmt.Sprintf("parse-macho:retained-git:%d#1", machOProfileGit),
			operation: OperationParse,
			cause:     CauseMalformed,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGitAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.gitRole,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			if test.prepare != nil {
				test.prepare(t, fixture)
			}
			fixture.failures[test.key] = newAuthorityPrimitiveFailure(test.operation, test.cause)
			outcome := test.use(fixture.revalidator(t), fixture)
			requireAuthorityPrimitiveRecord(t, outcome.primary, test.operation, test.cause)
			if outcome.descriptorClose != nil {
				t.Fatalf("unexpected DescriptorClose = %+v", outcome.descriptorClose)
			}
			fixture.requireOwnership(t)
		})
	}
}

func TestAuthorityBracketTreePathAndRowDriftAreCompared(t *testing.T) {
	for _, test := range []struct {
		name  string
		cause CauseCode
		use   func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
	}{
		{
			name:  "GOROOT pathname replacement",
			cause: CauseIdentity,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				observation := fixture.observed["path:toolchain"]
				observation.snapshot.identity.Inode++
				fixture.observed["path:toolchain"] = observation
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:  "GOROOT row removal",
			cause: CauseUnstable,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				fixture.directoryEntries["path:toolchain"] = []string{"bin", "pkg"}
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:  "GOROOT pathname kind replacement",
			cause: CauseIdentity,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				fixture.kinds["root/toolchain"] = entrySymlink
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:  "GOROOT row identity",
			cause: CauseIdentity,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				observation := fixture.observed["tree:go"]
				observation.snapshot.identity.Inode++
				fixture.observed["tree:go"] = observation
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			outcome := test.use(fixture.revalidator(t), fixture)
			requireAuthorityPrimitiveRecord(t, outcome.primary, OperationCompare, test.cause)
			fixture.requireOwnership(t)
		})
	}
}

func TestAuthorityBracketRepeatedSecurityDriftIsComparedBeforePolicy(t *testing.T) {
	type authorityUse func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
	uses := []struct {
		name  string
		label string
		use   authorityUse
		skip  bool
	}{
		{
			name:  "physical root",
			label: "root",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidatePhysicalRoot(context.Background(), fixture.physicalRoot)
			},
		},
		{
			name:  "retained null",
			label: "retained-null",
			skip:  runtime.GOOS != "darwin" || runtime.GOARCH != "arm64",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateRetainedNull(
					context.Background(),
					fixture.physicalRoot,
					fixture.nullDevice,
				)
			},
		},
		{
			name:  "GOROOT root",
			label: "retained-goroot",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:  "GOROOT row",
			label: "tree:go",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:  "Go role",
			label: "retained-go",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.goRole,
				)
			},
		},
		{
			name:  "compiler role",
			label: "retained-compiler",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateCompilerAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.compilerRole,
				)
			},
		},
		{
			name:  "Git role",
			label: "retained-git",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGitAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.gitRole,
				)
			},
		},
	}
	mutations := []struct {
		name   string
		mutate func(*authorityDescriptorObservation)
	}{
		{
			name: "mode",
			mutate: func(observation *authorityDescriptorObservation) {
				observation.snapshot.identity.Mode ^= 0o002
			},
		},
		{
			name: "ACL",
			mutate: func(observation *authorityDescriptorObservation) {
				observation.aclDigest[0] ^= 0xff
			},
		},
		{
			name: "mount",
			mutate: func(observation *authorityDescriptorObservation) {
				observation.mount.flags ^= 0x80
			},
		},
	}
	for _, authority := range uses {
		for _, mutation := range mutations {
			t.Run(authority.name+" "+mutation.name, func(t *testing.T) {
				if authority.skip {
					t.Skip("retained-null static policy is supported only on darwin/arm64")
				}
				fixture := newAuthorityBracketFixture(t)
				observation := fixture.observed[authority.label]
				mutation.mutate(&observation)
				fixture.observed[authority.label] = observation
				outcome := authority.use(fixture.revalidator(t), fixture)
				requireAuthorityPrimitiveRecord(t, outcome.primary, OperationCompare, CauseUnstable)
				fixture.requireOwnership(t)
			})
		}
	}
}

func TestAuthorityBracketDeferredACLPolicyFollowsComparison(t *testing.T) {
	type authorityUse func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
	for _, authority := range []struct {
		name     string
		label    string
		parseKey string
		use      authorityUse
		skip     bool
	}{
		{
			name:     "physical root",
			label:    "root",
			parseKey: "parse-acl#1",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidatePhysicalRoot(context.Background(), fixture.physicalRoot)
			},
		},
		{
			name:     "retained null",
			label:    "retained-null",
			parseKey: "parse-null-acl#1",
			skip:     runtime.GOOS != "darwin" || runtime.GOARCH != "arm64",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateRetainedNull(
					context.Background(),
					fixture.physicalRoot,
					fixture.nullDevice,
				)
			},
		},
	} {
		for _, drift := range []bool{false, true} {
			name := "unchanged refused ACL"
			if drift {
				name = "changed refused ACL"
			}
			t.Run(authority.name+" "+name, func(t *testing.T) {
				if authority.skip {
					t.Skip("retained-null static policy is supported only on darwin/arm64")
				}
				fixture := newAuthorityBracketFixture(t)
				fixture.aclPolicyFailures[authority.parseKey] = newAuthorityPrimitiveFailure(
					OperationValidate,
					CausePermission,
				)
				if drift {
					observation := fixture.observed[authority.label]
					observation.aclDigest[0] ^= 0xff
					fixture.observed[authority.label] = observation
				}
				outcome := authority.use(fixture.revalidator(t), fixture)
				if drift {
					requireAuthorityPrimitiveRecord(
						t,
						outcome.primary,
						OperationCompare,
						CauseUnstable,
					)
				} else {
					requireAuthorityPrimitiveRecord(
						t,
						outcome.primary,
						OperationValidate,
						CausePermission,
					)
				}
				fixture.requireOwnership(t)
			})
		}
	}
}

func TestAuthorityBracketGOROOTRoleParentSecurityDriftIsUnstable(t *testing.T) {
	for _, test := range []struct {
		name string
		use  func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
	}{
		{
			name: "Go",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.goRole,
				)
			},
		},
		{
			name: "compiler",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateCompilerAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.compilerRole,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			leaf := fixture.goRole.retained.leaf
			if test.name == "compiler" {
				leaf = fixture.compilerRole.retained.leaf
			}
			leaf.pathClaims[len(leaf.pathClaims)-1].aclDigest[0] ^= 0xff
			outcome := test.use(fixture.revalidator(t), fixture)
			requireAuthorityPrimitiveRecord(
				t,
				outcome.primary,
				OperationCompare,
				CauseUnstable,
			)
			if len(fixture.events) != 0 {
				t.Fatalf("role relationship drift invoked primitives: %q", fixture.events)
			}
		})
	}
}

func TestAuthorityBracketTrailingGOROOTRoleBindingClassifiesDrift(t *testing.T) {
	mutateEntry := func(
		t *testing.T,
		fixture *authorityBracketFixture,
		path string,
		mutate func(*entrySnapshot),
	) {
		t.Helper()
		for index := range fixture.goroot.entries {
			if fixture.goroot.entries[index].path == path {
				mutate(&fixture.goroot.entries[index])
				return
			}
		}
		t.Fatalf("fixture has no GOROOT row %q", path)
	}
	for _, test := range []struct {
		name   string
		cause  CauseCode
		mutate func(*testing.T, *authorityBracketFixture)
	}{
		{
			name:  "required leaf disappears",
			cause: CauseUnstable,
			mutate: func(t *testing.T, fixture *authorityBracketFixture) {
				t.Helper()
				for index, entry := range fixture.goroot.entries {
					if entry.path == goGOROOTRelativePath {
						fixture.goroot.entries = append(
							append([]entrySnapshot(nil), fixture.goroot.entries[:index]...),
							fixture.goroot.entries[index+1:]...,
						)
						return
					}
				}
				t.Fatalf("fixture has no GOROOT row %q", goGOROOTRelativePath)
			},
		},
		{
			name:  "required leaf duplicates",
			cause: CauseUnstable,
			mutate: func(t *testing.T, fixture *authorityBracketFixture) {
				t.Helper()
				for _, entry := range fixture.goroot.entries {
					if entry.path == goGOROOTRelativePath {
						fixture.goroot.entries = append(fixture.goroot.entries, entry)
						return
					}
				}
				t.Fatalf("fixture has no GOROOT row %q", goGOROOTRelativePath)
			},
		},
		{
			name:  "parent kind changes",
			cause: CauseIdentity,
			mutate: func(t *testing.T, fixture *authorityBracketFixture) {
				mutateEntry(t, fixture, "bin", func(entry *entrySnapshot) {
					entry.kind = entrySymlink
				})
			},
		},
		{
			name:  "parent identity changes",
			cause: CauseIdentity,
			mutate: func(t *testing.T, fixture *authorityBracketFixture) {
				mutateEntry(t, fixture, "bin", func(entry *entrySnapshot) {
					entry.file.identity.Inode++
				})
			},
		},
		{
			name:  "parent security changes",
			cause: CauseUnstable,
			mutate: func(t *testing.T, fixture *authorityBracketFixture) {
				mutateEntry(t, fixture, "bin", func(entry *entrySnapshot) {
					entry.aclDigest[0] ^= 0xff
				})
			},
		},
		{
			name:  "leaf identity changes",
			cause: CauseIdentity,
			mutate: func(t *testing.T, fixture *authorityBracketFixture) {
				mutateEntry(t, fixture, goGOROOTRelativePath, func(entry *entrySnapshot) {
					entry.file.identity.Inode++
				})
			},
		},
		{
			name:  "leaf content changes",
			cause: CauseUnstable,
			mutate: func(t *testing.T, fixture *authorityBracketFixture) {
				mutateEntry(t, fixture, goGOROOTRelativePath, func(entry *entrySnapshot) {
					entry.content[0] ^= 0xff
				})
			},
		},
		{
			name:  "root identity changes",
			cause: CauseIdentity,
			mutate: func(_ *testing.T, fixture *authorityBracketFixture) {
				fixture.goroot.root.snapshot.identity.Inode++
			},
		},
		{
			name:  "root security changes",
			cause: CauseUnstable,
			mutate: func(_ *testing.T, fixture *authorityBracketFixture) {
				fixture.goroot.root.aclDigest[0] ^= 0xff
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			sealed, failure := sealGOROOTRoleBinding(
				fixture.physicalRoot,
				fixture.goroot,
				goGOROOTRelativePath,
			)
			if failure != nil {
				t.Fatalf("seal Go GOROOT binding: %+v", failure)
			}
			test.mutate(t, fixture)
			assertPreflightPrimitiveFailure(
				t,
				compareGOROOTRoleBindingSeal(context.Background(), sealed),
				OperationCompare,
				test.cause,
			)
		})
	}
}

func TestAuthorityBracketInvalidSealedStateIsInternalBeforeAcquisition(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*authorityBracketFixture)
		use    func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
	}{
		{
			name: "physical root mode",
			mutate: func(fixture *authorityBracketFixture) {
				fixture.physicalRoot.directory.snapshot.identity.Mode ^= 0o002
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidatePhysicalRoot(context.Background(), fixture.physicalRoot)
			},
		},
		{
			name: "null mode",
			mutate: func(fixture *authorityBracketFixture) {
				fixture.nullDevice.leaf.snapshot.identity.Mode ^= 0o002
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateRetainedNull(
					context.Background(),
					fixture.physicalRoot,
					fixture.nullDevice,
				)
			},
		},
		{
			name: "GOROOT root mode",
			mutate: func(fixture *authorityBracketFixture) {
				fixture.goroot.root.snapshot.identity.Mode ^= 0o002
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name: "GOROOT duplicate row",
			mutate: func(fixture *authorityBracketFixture) {
				fixture.goroot.entries = append(fixture.goroot.entries, fixture.goroot.entries[0])
				sortEntrySnapshots(fixture.goroot.entries)
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name: "Go executable mode",
			mutate: func(fixture *authorityBracketFixture) {
				fixture.goRole.retained.leaf.snapshot.identity.Mode ^= 0o002
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.goRole,
				)
			},
		},
		{
			name: "Go GOROOT missing root handle",
			mutate: func(fixture *authorityBracketFixture) {
				fixture.goroot.root.root = nil
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.goRole,
				)
			},
		},
		{
			name: "compiler GOROOT zero digest",
			mutate: func(fixture *authorityBracketFixture) {
				fixture.goroot.digest = Digest{}
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateCompilerAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.compilerRole,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "null mode" && (runtime.GOOS != "darwin" || runtime.GOARCH != "arm64") {
				t.Skip("retained-null static policy is supported only on darwin/arm64")
			}
			fixture := newAuthorityBracketFixture(t)
			test.mutate(fixture)
			outcome := test.use(fixture.revalidator(t), fixture)
			requireAuthorityPrimitiveRecord(
				t,
				outcome.primary,
				OperationValidate,
				CauseInternalInvariant,
			)
			if len(fixture.events) != 0 {
				t.Fatalf("invalid sealed state invoked primitives: %q", fixture.events)
			}
		})
	}
}

func TestAuthorityBracketTreeBoundsFailBeforePayloadWork(t *testing.T) {
	fixture := newAuthorityBracketFixture(t)
	revalidator := fixture.revalidator(t)
	sealed, failure := sealGOROOTBracket(fixture.physicalRoot, fixture.goroot)
	if failure != nil {
		t.Fatalf("seal GOROOT fixture: %+v", failure)
	}
	builder := attributedTreeBuilder{
		ctx:         context.Background(),
		revalidator: revalidator,
		sealed:      sealed,
		entries:     make([]entrySnapshot, 0, len(sealed.entries)),
		totalBytes:  sealed.policy.limits.maxBytes,
	}
	expected := sealed.expected[goGOROOTRelativePath]
	var outcome authorityUseOutcome
	builder.captureOwnedEntry(
		fixture.goRole.retained.leaf.descriptor,
		goGOROOTRelativePath,
		entryRegular,
		expected,
		&outcome,
	)
	requireAuthorityPrimitiveRecord(t, outcome.primary, OperationWalk, CauseLimit)
	if fixture.calls["hash:retained-go"] != 0 {
		t.Fatalf("aggregate exhaustion hashed payload %d times", fixture.calls["hash:retained-go"])
	}

	longComponent := strings.Repeat("a", maxPathComponentBytes+1)
	_, failure = builder.prepareDirectoryEntryPath("", longComponent)
	assertPreflightPrimitiveFailure(t, failure, OperationWalk, CauseLimit)
	for _, name := range []string{"", ".", "..", "nested/name", "nul\x00name"} {
		_, failure = builder.prepareDirectoryEntryPath("", name)
		assertPreflightPrimitiveFailure(t, failure, OperationWalk, CauseUnstable)
	}
}

func TestAuthorityBracketSymlinkBindingSamplesContextInsideResolution(t *testing.T) {
	directory := entrySnapshot{path: "bin", kind: entryDirectory}
	target := entrySnapshot{path: "bin/go", kind: entryRegular, content: Digest{0x41}}
	link := entrySnapshot{
		path:     "alias",
		kind:     entrySymlink,
		linkText: "bin/go",
	}
	entries := []entrySnapshot{link, directory, target}
	sortEntrySnapshots(entries)
	ctx := &stagedPreflightPrimitiveContext{failAfter: 5, err: context.Canceled}
	failure := parseAttributedSymlinkBindings(ctx, entries)
	assertPreflightPrimitiveFailure(t, failure, OperationParse, CauseCanceled)
}

func TestAuthorityBracketTrailingRebindSurvivesPrimaryAndCloseFailures(t *testing.T) {
	for _, test := range []struct {
		name       string
		failureKey string
		closeKey   string
		pathKey    string
		use        func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
	}{
		{
			name:       "GOROOT walk primary",
			failureKey: "read-dir:path:toolchain#1",
			pathKey:    "open-typed:root/toolchain",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:     "GOROOT row close",
			closeKey: "close:tree:go#1",
			pathKey:  "open-typed:root/toolchain",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:       "Go fresh hash primary",
			failureKey: "hash:tree:go#1",
			pathKey:    "open-typed:root/toolchain",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.goRole,
				)
			},
		},
		{
			name:     "Go fresh close",
			closeKey: "close:tree:go#1",
			pathKey:  "open-typed:root/toolchain",
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.goRole,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			if test.failureKey != "" {
				fixture.failures[test.failureKey] = newAuthorityPrimitiveFailure(
					OperationWalk,
					CauseUnstable,
				)
				if strings.Contains(test.name, "hash") {
					fixture.failures[test.failureKey] = newAuthorityPrimitiveFailure(
						OperationHash,
						CauseUnstable,
					)
				}
			}
			if test.closeKey != "" {
				fixture.closeFails[test.closeKey] = true
			}
			outcome := test.use(fixture.revalidator(t), fixture)
			if outcome.proved() {
				t.Fatal("injected authority failure unexpectedly proved")
			}
			if fixture.calls[test.pathKey] != 2 {
				t.Fatalf("trailing pathname resolutions = %d, want 2", fixture.calls[test.pathKey])
			}
			triggerKey := test.failureKey
			if triggerKey == "" {
				triggerKey = test.closeKey
			}
			triggerIndex := -1
			trailingIndex := -1
			for index, event := range fixture.events {
				if event == triggerKey {
					triggerIndex = index
				}
				if event == test.pathKey+"#2" {
					trailingIndex = index
				}
			}
			if triggerIndex < 0 || trailingIndex <= triggerIndex {
				t.Fatalf(
					"trailing pathname order = trigger %d, trailing %d, events %q",
					triggerIndex,
					trailingIndex,
					fixture.events,
				)
			}
			fixture.requireOwnership(t)
		})
	}
}

func TestAuthorityBracketSealsEveryAuthorityDependency(t *testing.T) {
	rebindPhysicalRoot := func(fixture *authorityBracketFixture) {
		replacement := *fixture.physicalRoot.directory
		replacement.pathClaims = append([]authorityPathClaim(nil), replacement.pathClaims...)
		fixture.physicalRoot.directory = &replacement
	}
	for _, test := range []struct {
		name   string
		hook   string
		mutate func(*authorityBracketFixture)
		use    func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
	}{
		{
			name:   "null physical root",
			hook:   "null-acl:fresh-null#1",
			mutate: rebindPhysicalRoot,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateRetainedNull(context.Background(), fixture.physicalRoot, fixture.nullDevice)
			},
		},
		{
			name:   "GOROOT physical root",
			hook:   "hash-manifest:" + domainGOROOT + "#1",
			mutate: rebindPhysicalRoot,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGOROOT(context.Background(), fixture.physicalRoot, fixture.goroot)
			},
		},
		{
			name:   "Go physical root",
			hook:   "parse-macho:tree:go:1#1",
			mutate: rebindPhysicalRoot,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(context.Background(), fixture.physicalRoot, fixture.goroot, fixture.goRole)
			},
		},
		{
			name:   "compiler physical root",
			hook:   "parse-macho:tree:compiler:1#1",
			mutate: rebindPhysicalRoot,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateCompilerAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.compilerRole,
				)
			},
		},
		{
			name:   "Git physical root",
			hook:   "parse-macho:fresh-git:2#1",
			mutate: rebindPhysicalRoot,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGitAuthority(context.Background(), fixture.physicalRoot, fixture.gitRole)
			},
		},
		{
			name: "Go GOROOT root",
			hook: "parse-macho:tree:go:1#1",
			mutate: func(fixture *authorityBracketFixture) {
				fixture.goroot.root.snapshot.mtimeSec++
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(context.Background(), fixture.physicalRoot, fixture.goroot, fixture.goRole)
			},
		},
		{
			name: "compiler GOROOT root",
			hook: "parse-macho:tree:compiler:1#1",
			mutate: func(fixture *authorityBracketFixture) {
				fixture.goroot.root.aclDigest[0] ^= 0xff
			},
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateCompilerAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.compilerRole,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "null physical root" && (runtime.GOOS != "darwin" || runtime.GOARCH != "arm64") {
				t.Skip("retained-null static policy is supported only on darwin/arm64")
			}
			fixture := newAuthorityBracketFixture(t)
			fixture.hooks[test.hook] = func() { test.mutate(fixture) }
			outcome := test.use(fixture.revalidator(t), fixture)
			requireAuthorityPrimitiveRecord(t, outcome.primary, OperationCompare, CauseUnstable)
			fixture.requireOwnership(t)
		})
	}
}

func TestAuthorityBracketNominalSealContextWinsOverRebinding(t *testing.T) {
	fixture := newAuthorityBracketFixture(t)
	goSealed, failure := sealExecutableBracket(fixture.physicalRoot, fixture.goRole.retained, machOProfileGo)
	if failure != nil {
		t.Fatalf("seal Go authority: %+v", failure)
	}
	goBinding, failure := sealGOROOTRoleBinding(
		fixture.physicalRoot,
		fixture.goroot,
		goGOROOTRelativePath,
	)
	if failure != nil {
		t.Fatalf("seal Go binding: %+v", failure)
	}
	compilerSealed, failure := sealExecutableBracket(
		fixture.physicalRoot,
		fixture.compilerRole.retained,
		machOProfileGo,
	)
	if failure != nil {
		t.Fatalf("seal compiler authority: %+v", failure)
	}
	compilerBinding, failure := sealGOROOTRoleBinding(
		fixture.physicalRoot,
		fixture.goroot,
		compilerGOROOTRelativePath,
	)
	if failure != nil {
		t.Fatalf("seal compiler binding: %+v", failure)
	}
	gitSealed, failure := sealExecutableBracket(fixture.physicalRoot, fixture.gitRole.retained, machOProfileGit)
	if failure != nil {
		t.Fatalf("seal Git authority: %+v", failure)
	}

	goReplacement := *fixture.goRole.retained
	fixture.goRole.retained = &goReplacement
	compilerReplacement := *fixture.compilerRole.retained
	fixture.compilerRole.retained = &compilerReplacement
	gitReplacement := *fixture.gitRole.retained
	fixture.gitRole.retained = &gitReplacement
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	assertPreflightPrimitiveFailure(
		t,
		compareGoAuthoritySeal(canceled, fixture.goRole, goSealed, goBinding),
		OperationCompare,
		CauseCanceled,
	)
	assertPreflightPrimitiveFailure(
		t,
		compareCompilerAuthoritySeal(canceled, fixture.compilerRole, compilerSealed, compilerBinding),
		OperationCompare,
		CauseCanceled,
	)
	assertPreflightPrimitiveFailure(
		t,
		compareGitAuthoritySeal(canceled, fixture.gitRole, gitSealed),
		OperationCompare,
		CauseCanceled,
	)
}

func TestAuthorityBracketEveryNominalRolePinsFreshIdentityAndWrapper(t *testing.T) {
	for _, role := range []struct {
		name          string
		freshLabel    string
		retainedLabel string
		profile       machOProfile
		use           func(*preflightAuthorityRevalidator, *authorityBracketFixture) authorityUseOutcome
		breakSeal     func(*authorityBracketFixture)
		rebind        func(*authorityBracketFixture)
	}{
		{
			name:          "Go",
			freshLabel:    "tree:go",
			retainedLabel: "retained-go",
			profile:       machOProfileGo,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGoAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.goRole,
				)
			},
			breakSeal: func(fixture *authorityBracketFixture) {
				fixture.goRole.seal = goAuthoritySeal(2)
			},
			rebind: func(fixture *authorityBracketFixture) {
				replacement := *fixture.goRole.retained
				fixture.goRole.retained = &replacement
			},
		},
		{
			name:          "compiler",
			freshLabel:    "tree:compiler",
			retainedLabel: "retained-compiler",
			profile:       machOProfileGo,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateCompilerAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.goroot,
					fixture.compilerRole,
				)
			},
			breakSeal: func(fixture *authorityBracketFixture) {
				fixture.compilerRole.seal = compilerAuthoritySeal(2)
			},
			rebind: func(fixture *authorityBracketFixture) {
				replacement := *fixture.compilerRole.retained
				fixture.compilerRole.retained = &replacement
			},
		},
		{
			name:          "Git",
			freshLabel:    "fresh-git",
			retainedLabel: "retained-git",
			profile:       machOProfileGit,
			use: func(revalidator *preflightAuthorityRevalidator, fixture *authorityBracketFixture) authorityUseOutcome {
				return revalidator.revalidateGitAuthority(
					context.Background(),
					fixture.physicalRoot,
					fixture.gitRole,
				)
			},
			breakSeal: func(fixture *authorityBracketFixture) {
				fixture.gitRole.seal = gitAuthoritySeal(2)
			},
			rebind: func(fixture *authorityBracketFixture) {
				replacement := *fixture.gitRole.retained
				fixture.gitRole.retained = &replacement
			},
		},
	} {
		t.Run(role.name+" fresh replacement", func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			observation := fixture.observed[role.freshLabel]
			observation.snapshot.identity.Inode++
			fixture.observed[role.freshLabel] = observation
			outcome := role.use(fixture.revalidator(t), fixture)
			requireAuthorityPrimitiveRecord(t, outcome.primary, OperationCompare, CauseIdentity)
			fixture.requireOwnership(t)
		})

		t.Run(role.name+" wrapper seal", func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			role.breakSeal(fixture)
			outcome := role.use(fixture.revalidator(t), fixture)
			requireAuthorityPrimitiveRecord(
				t,
				outcome.primary,
				OperationValidate,
				CauseInternalInvariant,
			)
			if len(fixture.events) != 0 {
				t.Fatalf("invalid %s seal invoked primitives: %q", role.name, fixture.events)
			}
		})

		t.Run(role.name+" wrapper rebinding", func(t *testing.T) {
			fixture := newAuthorityBracketFixture(t)
			key := fmt.Sprintf(
				"parse-macho:%s:%d#1",
				role.retainedLabel,
				role.profile,
			)
			fixture.hooks[key] = func() { role.rebind(fixture) }
			outcome := role.use(fixture.revalidator(t), fixture)
			requireAuthorityPrimitiveRecord(
				t,
				outcome.primary,
				OperationCompare,
				CauseUnstable,
			)
			fixture.requireOwnership(t)
		})
	}
}

func TestAuthorityBracketNilAndIncompleteSeamsRefuseBeforeAcquisition(t *testing.T) {
	fixture := newAuthorityBracketFixture(t)
	for _, test := range []struct {
		name string
		use  func(*preflightAuthorityRevalidator) authorityUseOutcome
	}{
		{
			name: "nil receiver",
			use: func(revalidator *preflightAuthorityRevalidator) authorityUseOutcome {
				return revalidator.revalidatePhysicalRoot(context.Background(), fixture.physicalRoot)
			},
		},
		{
			name: "nil context",
			use: func(revalidator *preflightAuthorityRevalidator) authorityUseOutcome {
				return revalidator.revalidateRetainedNull(
					nil, //nolint:staticcheck // Deliberately exercise the private seam invariant.
					fixture.physicalRoot,
					fixture.nullDevice,
				)
			},
		},
		{
			name: "missing bracket primitive",
			use: func(*preflightAuthorityRevalidator) authorityUseOutcome {
				primitives := fixture.primitives()
				primitives.brackets.openAbsoluteRoot = nil
				return (&preflightAuthorityRevalidator{primitives: primitives}).revalidatePhysicalRoot(
					context.Background(),
					fixture.physicalRoot,
				)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture.resetTrace()
			var revalidator *preflightAuthorityRevalidator
			if test.name != "nil receiver" {
				revalidator = fixture.revalidator(t)
			}
			outcome := test.use(revalidator)
			requireAuthorityPrimitiveRecord(
				t,
				outcome.primary,
				OperationValidate,
				CauseInternalInvariant,
			)
			if len(fixture.events) != 0 {
				t.Fatalf("invalid seam invoked primitives: %q", fixture.events)
			}
		})
	}
}

func TestAuthorityBracketFilesDoNotCallCauseOnlyComposites(t *testing.T) {
	forbidden := map[string]bool{
		"captureRetainedExecutable":                       true,
		"captureRetainedTree":                             true,
		"captureRetainedTreeWithRead":                     true,
		"gorootExecutableBinding":                         true,
		"hashBoundedReader":                               true,
		"hashRegularFile":                                 true,
		"hashRetainedFileContent":                         true,
		"inspectDescriptorContext":                        true,
		"inspectNullAuthorityDescriptorContext":           true,
		"openAuthorityPathContext":                        true,
		"openAuthorityPathContextWithInspect":             true,
		"revalidate":                                      true,
		"validateExecutableAgainstGOROOTBinding":          true,
		"validateExecutableProfile":                       true,
		"validateGitExecutableProfile":                    true,
		"validateGoExecutableProfile":                     true,
		"validateGOROOTExecutableBinding":                 true,
		"validatePhysicalRootAuthority":                   true,
		"validatePhysicalRootAuthorityContext":            true,
		"validatePhysicalRootAuthorityContextWithInspect": true,
		"validateRetainedExecutableBinding":               true,
		"validateRetainedExecutableDescriptor":            true,
		"validateRetainedExecutableShape":                 true,
		"validateRetainedNullDevice":                      true,
		"validateRetainedNullDeviceContext":               true,
		"validateRetainedNullDeviceContextWith":           true,
	}
	files := token.NewFileSet()
	for _, name := range []string{
		"preflight_authority_darwin.go",
		"preflight_authority_bracket.go",
		"preflight_authority_primitives.go",
		"preflight_authority_roles.go",
		"preflight_authority_tree.go",
	} {
		parsed, err := parser.ParseFile(files, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			called := ""
			switch function := call.Fun.(type) {
			case *ast.Ident:
				called = function.Name
			case *ast.SelectorExpr:
				called = function.Sel.Name
			}
			if forbidden[called] {
				t.Fatalf(
					"%s invokes cause-only authority composite %s",
					files.Position(call.Pos()),
					called,
				)
			}
			return true
		})
	}
}

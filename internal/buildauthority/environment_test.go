package buildauthority

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnvironmentProfileAndProjectionTypesAreNonInterchangeable(t *testing.T) {
	t.Parallel()

	types := []reflect.Type{
		reflect.TypeFor[preallocationEnvironment](),
		reflect.TypeFor[taskPrivateEnvironment](),
		reflect.TypeFor[goOwnedEnvironment](),
		reflect.TypeFor[goGitEnvironment](),
		reflect.TypeFor[goToolIDEnvironment](),
		reflect.TypeFor[goCompileEnvironment](),
		reflect.TypeFor[goAssemblerEnvironment](),
		reflect.TypeFor[goLinkEnvironment](),
	}
	for left := range types {
		for right := range types {
			if left == right {
				continue
			}
			if types[left].AssignableTo(types[right]) || types[left].ConvertibleTo(types[right]) {
				t.Fatalf("environment type %s converts or assigns to %s", types[left], types[right])
			}
		}
	}
	for _, constructor := range []any{
		newPreallocationEnvironment,
		newTaskPrivateEnvironment,
		deriveGoOwnedEnvironment,
		deriveGoGitEnvironment,
		deriveGoToolIDEnvironment,
		deriveGoCompileEnvironment,
		deriveGoAssemblerEnvironment,
		deriveGoLinkEnvironment,
	} {
		constructorType := reflect.TypeOf(constructor)
		for input := range constructorType.Ins() {
			if input.Kind() == reflect.String {
				t.Fatalf("environment constructor %s accepts a raw string", constructorType)
			}
		}
	}
}

func TestPinnedGoOwnedEnvironmentAndChildProjectionsAreExact(t *testing.T) {
	taskRoot := "/private/state/.sandbox-gc-build-0123456789abcdef01234567"
	goroot := "/private/toolchain/go"
	gomodcache := "/private/cache/pkg/mod"
	direct := mustTestTaskPrivateEnvironment(t, taskRoot, goroot, gomodcache)

	base, err := deriveGoOwnedEnvironment(direct)
	if err != nil {
		t.Fatalf("derive Go-owned environment: %v", err)
	}
	baseRows := base.clone()
	if len(baseRows) != goOwnedEnvironmentRowCount {
		t.Fatalf("Go-owned environment rows = %d", len(baseRows))
	}
	if baseRows[15] != "GOENV=" {
		t.Fatalf("GOENV replacement = %q", baseRows[15])
	}
	wantTail := []string{
		"GOAUTH=netrc",
		"GOHOSTARCH=arm64",
		"GOHOSTOS=darwin",
		"GOTELEMETRY=off",
		"GOTOOLDIR=/private/toolchain/go/pkg/tool/darwin_arm64",
		"GOVERSION=go1.27.1",
		"GCCGO=gccgo",
		"AR=ar",
		"CC=cc",
		"CXX=c++",
		"GCM_INTERACTIVE=never",
	}
	if !reflect.DeepEqual(baseRows[directEnvironmentRowCount:], wantTail) {
		t.Fatalf("Go-owned tail:\n got: %q\nwant: %q", baseRows[directEnvironmentRowCount:], wantTail)
	}

	gitEnvironment, err := deriveGoGitEnvironment(base)
	if err != nil {
		t.Fatalf("derive Go-owned Git environment: %v", err)
	}
	wantCheckout := filepath.Join(taskRoot, workspaceCheckout)
	if rows := gitEnvironment.clone(); len(rows) != goGitEnvironmentRowCount ||
		rows[len(rows)-1] != "PWD="+wantCheckout || gitEnvironment.directory() != wantCheckout {
		t.Fatalf("Git projection = %q; directory=%q", rows, gitEnvironment.directory())
	}

	toolEnvironment, err := deriveGoToolIDEnvironment(base)
	if err != nil {
		t.Fatalf("derive Go tool-ID environment: %v", err)
	}
	if rows := toolEnvironment.clone(); !reflect.DeepEqual(rows, baseRows) ||
		len(rows) != goToolEnvironmentRowCount || toolEnvironment.directory() != "" || hasEnvironmentKey(rows, "PWD") {
		t.Fatalf("tool-ID projection = %q; directory=%q", rows, toolEnvironment.directory())
	}

	description := packageDescriptionAuthority{value: "runtime", seal: validPackageDescriptionAuthority}
	compileEnvironment, err := deriveGoCompileEnvironment(base, description)
	if err != nil {
		t.Fatalf("derive Go compile environment: %v", err)
	}
	compileRows := compileEnvironment.clone()
	if len(compileRows) != goBuildEnvironmentRowCount ||
		!reflect.DeepEqual(compileRows[len(compileRows)-2:], []string{
			"PWD=" + wantCheckout,
			"TOOLEXEC_IMPORTPATH=runtime",
		}) || compileEnvironment.directory() != wantCheckout {
		t.Fatalf("compile projection = %q; directory=%q", compileRows, compileEnvironment.directory())
	}

	assemblerDirectory := assemblerDirectoryAuthority{
		path:   filepath.Join(goroot, "src/runtime"),
		source: assemblerDirectoryGOROOT,
		seal:   validAssemblerDirectoryAuthority,
	}
	assemblerEnvironment, err := deriveGoAssemblerEnvironment(base, assemblerDirectory, description)
	if err != nil {
		t.Fatalf("derive Go assembler environment: %v", err)
	}
	assemblerRows := assemblerEnvironment.clone()
	if len(assemblerRows) != goBuildEnvironmentRowCount ||
		!reflect.DeepEqual(assemblerRows[len(assemblerRows)-2:], []string{
			"PWD=" + assemblerDirectory.path,
			"TOOLEXEC_IMPORTPATH=runtime",
		}) || assemblerEnvironment.directory() != assemblerDirectory.path {
		t.Fatalf("assembler projection = %q; directory=%q", assemblerRows, assemblerEnvironment.directory())
	}

	linkEnvironment, err := deriveGoLinkEnvironment(base, description)
	if err != nil {
		t.Fatalf("derive Go link environment: %v", err)
	}
	linkRows := linkEnvironment.clone()
	if len(linkRows) != goLinkEnvironmentRowCount || linkEnvironment.directory() != "" ||
		!reflect.DeepEqual(linkRows[len(linkRows)-2:], []string{
			"TOOLEXEC_IMPORTPATH=runtime",
			"GOROOT=" + goroot,
		}) || hasEnvironmentKey(linkRows[:len(linkRows)-2], "GOROOT") || hasEnvironmentKey(linkRows, "PWD") {
		t.Fatalf("link projection = %q; directory=%q", linkRows, linkEnvironment.directory())
	}
}

func TestEnvironmentProjectionClonesAreFreshAndTamperingRefuses(t *testing.T) {
	direct := mustTestTaskPrivateEnvironment(
		t,
		"/private/state/.sandbox-gc-build-0123456789abcdef01234567",
		"/private/go",
		"/private/mod",
	)
	base, err := deriveGoOwnedEnvironment(direct)
	if err != nil {
		t.Fatalf("derive Go-owned environment: %v", err)
	}
	description := packageDescriptionAuthority{value: "example/package", seal: validPackageDescriptionAuthority}
	assemblerDirectory := assemblerDirectoryAuthority{
		path:   "/private/mod/example/package@v1.0.0",
		source: assemblerDirectoryGOMODCACHE,
		seal:   validAssemblerDirectoryAuthority,
	}
	gitEnvironment, err := deriveGoGitEnvironment(base)
	if err != nil {
		t.Fatal(err)
	}
	toolEnvironment, err := deriveGoToolIDEnvironment(base)
	if err != nil {
		t.Fatal(err)
	}
	compileEnvironment, err := deriveGoCompileEnvironment(base, description)
	if err != nil {
		t.Fatal(err)
	}
	assemblerEnvironment, err := deriveGoAssemblerEnvironment(base, assemblerDirectory, description)
	if err != nil {
		t.Fatal(err)
	}
	linkEnvironment, err := deriveGoLinkEnvironment(base, description)
	if err != nil {
		t.Fatal(err)
	}
	profiles := []struct {
		name  string
		clone func() []string
	}{
		{name: "direct", clone: direct.clone},
		{name: "base", clone: base.clone},
		{name: "Git", clone: gitEnvironment.clone},
		{name: "tool ID", clone: toolEnvironment.clone},
		{name: "compile", clone: compileEnvironment.clone},
		{name: "assembler", clone: assemblerEnvironment.clone},
		{name: "link", clone: linkEnvironment.clone},
	}
	for _, profile := range profiles {
		first := profile.clone()
		first[0] = "AMBIENT=1"
		if profile.clone()[0] == first[0] {
			t.Fatalf("mutating returned %s environment changed sealed rows", profile.name)
		}
	}

	tampered := base
	tampered.rows[0] = "AMBIENT=1"
	if tampered.valid() || tampered.clone() != nil {
		t.Fatal("tampered Go-owned environment remained valid")
	}
	if (goOwnedEnvironment{}).valid() || (goOwnedEnvironment{}).clone() != nil ||
		(preallocationEnvironment{}).valid() || (taskPrivateEnvironment{}).valid() {
		t.Fatal("zero environment profile became valid")
	}
}

func TestEnvironmentProjectionAuthoritiesRefuseInvalidValues(t *testing.T) {
	direct := mustTestTaskPrivateEnvironment(
		t,
		"/private/state/.sandbox-gc-build-0123456789abcdef01234567",
		"/private/go",
		"/private/mod",
	)
	base, err := deriveGoOwnedEnvironment(direct)
	if err != nil {
		t.Fatal(err)
	}
	for _, description := range []packageDescriptionAuthority{
		{},
		{value: "runtime\nforged", seal: validPackageDescriptionAuthority},
		{value: "runtime", seal: packageDescriptionAuthoritySeal(2)},
	} {
		if environment, err := deriveGoCompileEnvironment(base, description); err == nil || environment.valid() {
			t.Fatalf("invalid package description entered compile projection: %#v", description)
		}
	}
	for _, directory := range []assemblerDirectoryAuthority{
		{},
		{path: "/outside/authority", source: assemblerDirectoryGOROOT, seal: validAssemblerDirectoryAuthority},
		{path: "/private/go/src/runtime", source: assemblerDirectorySource(255), seal: validAssemblerDirectoryAuthority},
	} {
		description := packageDescriptionAuthority{value: "runtime", seal: validPackageDescriptionAuthority}
		if environment, err := deriveGoAssemblerEnvironment(base, directory, description); err == nil || environment.valid() {
			t.Fatalf("invalid assembler directory entered projection: %#v", directory)
		}
	}
}

func TestEnvironmentAuthorityCompositionRefusesMixedOrIncompleteInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(environmentAuthorityInputs)
	}{
		{
			name: "physical-root claim",
			mutate: func(authorities environmentAuthorityInputs) {
				authorities.physicalRoot.claim = authorityPathClaim{}
			},
		},
		{
			name: "null-device ancestry",
			mutate: func(authorities environmentAuthorityInputs) {
				authorities.nullDevice.leaf.pathClaims = nil
			},
		},
		{
			name: "Go from another GOROOT capture",
			mutate: func(authorities environmentAuthorityInputs) {
				for index := range authorities.goroot.entries {
					if authorities.goroot.entries[index].path == goGOROOTRelativePath {
						authorities.goroot.entries[index].content = Digest{0xff}
						return
					}
				}
				t.Fatal("test GOROOT has no Go entry")
			},
		},
		{
			name: "compiler authority alias",
			mutate: func(authorities environmentAuthorityInputs) {
				authorities.compiler.retained = authorities.goExecutable.retained
			},
		},
		{
			name: "Git seal",
			mutate: func(authorities environmentAuthorityInputs) {
				authorities.gitExecutable.seal = gitAuthoritySeal(2)
			},
		},
		{
			name: "tree root overlap",
			mutate: func(authorities environmentAuthorityInputs) {
				authorities.gomodcache.root.path = authorities.goroot.root.path
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorities := testEnvironmentAuthorityInputs("/private/go", "/private/mod")
			test.mutate(authorities)
			if environment, err := newPreallocationEnvironment(authorities); err == nil || environment.valid() {
				t.Fatal("mixed or incomplete authorities sealed a preallocation environment")
			}
		})
	}
}

func TestTaskEnvironmentRefusesIncompleteOrMismatchedWorkspace(t *testing.T) {
	t.Parallel()

	taskRoot := "/private/state/.sandbox-gc-build-0123456789abcdef01234567"
	tests := []struct {
		name   string
		mutate func(*taskWorkspace)
	}{
		{name: "closed", mutate: func(workspace *taskWorkspace) { workspace.closed = true }},
		{name: "missing StateRoot handle", mutate: func(workspace *taskWorkspace) {
			workspace.stateRoot.descriptor = nil
		}},
		{name: "wrong task name", mutate: func(workspace *taskWorkspace) { workspace.taskName = "task" }},
		{name: "missing spool handle", mutate: func(workspace *taskWorkspace) {
			workspace.spool.root.root = nil
		}},
		{name: "closed spool", mutate: func(workspace *taskWorkspace) { workspace.spool.closed = true }},
		{name: "different Git descriptor", mutate: func(workspace *taskWorkspace) {
			workspace.git.descriptor = &os.File{}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorities := testEnvironmentAuthorityInputs("/private/go", "/private/mod")
			workspace := testEnvironmentWorkspace(taskRoot, authorities.gitExecutable)
			test.mutate(workspace)
			if environment, err := newTaskPrivateEnvironment(workspace, authorities); err == nil || environment.valid() {
				t.Fatal("incomplete or mismatched workspace sealed a task-private environment")
			}
		})
	}
}

func TestWorkspaceClosureInvalidatesTaskAndDerivedEnvironmentProfiles(t *testing.T) {
	t.Parallel()

	direct := mustTestTaskPrivateEnvironment(
		t,
		"/private/state/.sandbox-gc-build-0123456789abcdef01234567",
		"/private/go",
		"/private/mod",
	)
	base, err := deriveGoOwnedEnvironment(direct)
	if err != nil {
		t.Fatal(err)
	}
	description := packageDescriptionAuthority{value: "runtime", seal: validPackageDescriptionAuthority}
	gitEnvironment, err := deriveGoGitEnvironment(base)
	if err != nil {
		t.Fatal(err)
	}
	compileEnvironment, err := deriveGoCompileEnvironment(base, description)
	if err != nil {
		t.Fatal(err)
	}
	assemblerEnvironment, err := deriveGoAssemblerEnvironment(base, assemblerDirectoryAuthority{
		path: "/private/go/src/runtime", source: assemblerDirectoryGOROOT,
		seal: validAssemblerDirectoryAuthority,
	}, description)
	if err != nil {
		t.Fatal(err)
	}
	linkEnvironment, err := deriveGoLinkEnvironment(base, description)
	if err != nil {
		t.Fatal(err)
	}

	direct.workspace.lifecycle.Lock()
	direct.workspace.closed = true
	direct.workspace.lifecycle.Unlock()
	profiles := []struct {
		name      string
		valid     func() bool
		clone     func() []string
		directory func() string
	}{
		{name: "direct", valid: direct.valid, clone: direct.clone},
		{name: "base", valid: base.valid, clone: base.clone},
		{name: "Git", valid: gitEnvironment.valid, clone: gitEnvironment.clone, directory: gitEnvironment.directory},
		{name: "compile", valid: compileEnvironment.valid, clone: compileEnvironment.clone, directory: compileEnvironment.directory},
		{name: "assembler", valid: assemblerEnvironment.valid, clone: assemblerEnvironment.clone, directory: assemblerEnvironment.directory},
		{name: "link", valid: linkEnvironment.valid, clone: linkEnvironment.clone},
	}
	for _, profile := range profiles {
		if profile.valid() || profile.clone() != nil {
			t.Fatalf("closed workspace left %s environment live", profile.name)
		}
		if profile.directory != nil && profile.directory() != "" {
			t.Fatalf("closed workspace exposed %s directory", profile.name)
		}
	}
}

func hasEnvironmentKey(rows []string, key string) bool {
	prefix := key + "="
	for _, row := range rows {
		if strings.HasPrefix(row, prefix) {
			return true
		}
	}
	return false
}

func mustTestPreallocationEnvironment(
	t *testing.T,
	goroot, gomodcache string,
) preallocationEnvironment {
	t.Helper()
	environment, err := newPreallocationEnvironment(testEnvironmentAuthorityInputs(goroot, gomodcache))
	if err != nil {
		t.Fatalf("construct test preallocation environment: %v", err)
	}
	return environment
}

func mustTestTaskPrivateEnvironment(
	t *testing.T,
	taskRoot, goroot, gomodcache string,
) taskPrivateEnvironment {
	t.Helper()
	environment, err := testTaskPrivateEnvironment(taskRoot, goroot, gomodcache)
	if err != nil {
		t.Fatalf("construct test task-private environment: %v", err)
	}
	return environment
}

func testTaskPrivateEnvironment(
	taskRoot, goroot, gomodcache string,
) (taskPrivateEnvironment, error) {
	authorities := testEnvironmentAuthorityInputs(goroot, gomodcache)
	workspace := testEnvironmentWorkspace(taskRoot, authorities.gitExecutable)
	return newTaskPrivateEnvironment(workspace, authorities)
}

func testEnvironmentAuthorityInputs(goroot, gomodcache string) environmentAuthorityInputs {
	mount := mountSnapshot{filesystem: [2]int32{17, 19}, flags: 1}
	rootSnapshot := fileSnapshot{identity: FileIdentity{
		Device: 1, Inode: 1, UID: 0, Mode: platformModeDirectory | 0o755, Filesystem: mount.filesystem,
	}}
	rootClaim := makeAuthorityPathClaim(rootSnapshot, mount, Digest{})
	physicalRoot := &physicalRootAuthority{
		directory: &retainedDirectory{
			path: physicalRootPath, root: &os.Root{}, descriptor: &os.File{}, snapshot: rootSnapshot,
			mount: mount, pathClaims: []authorityPathClaim{rootClaim},
		},
		claim: rootClaim,
	}
	nullDevice := &retainedNullDevice{leaf: &retainedLeaf{
		path: nullDevicePath, descriptor: &os.File{}, pathClaims: []authorityPathClaim{{mount: mount}},
	}}

	gorootRootClaim := testEnvironmentDirectoryClaim(2, mount)
	bin := testEnvironmentDirectoryEntry("bin", 3, mount)
	pkg := testEnvironmentDirectoryEntry("pkg", 4, mount)
	pkgTool := testEnvironmentDirectoryEntry("pkg/tool", 5, mount)
	toolDirectory := testEnvironmentDirectoryEntry("pkg/tool/darwin_arm64", 6, mount)
	goLeaf := testEnvironmentExecutable(
		filepath.Join(goroot, goGOROOTRelativePath),
		goExecutableDigest(),
		7,
		mount,
		[]authorityPathClaim{gorootRootClaim, makeAuthorityPathClaim(bin.file, mount, bin.aclDigest)},
	)
	compilerDigest := Digest{0x31}
	compilerLeaf := testEnvironmentExecutable(
		filepath.Join(goroot, compilerGOROOTRelativePath),
		compilerDigest,
		8,
		mount,
		[]authorityPathClaim{
			gorootRootClaim,
			makeAuthorityPathClaim(pkg.file, mount, pkg.aclDigest),
			makeAuthorityPathClaim(pkgTool.file, mount, pkgTool.aclDigest),
			makeAuthorityPathClaim(toolDirectory.file, mount, toolDirectory.aclDigest),
		},
	)
	goEntry := entrySnapshot{
		path: goGOROOTRelativePath, kind: entryRegular, file: goLeaf.leaf.snapshot,
		aclDigest: goLeaf.leaf.aclDigest, content: goLeaf.digest,
	}
	compilerEntry := entrySnapshot{
		path: compilerGOROOTRelativePath, kind: entryRegular, file: compilerLeaf.leaf.snapshot,
		aclDigest: compilerLeaf.leaf.aclDigest, content: compilerLeaf.digest,
	}
	gorootCapture := &treeCapture{
		root: &retainedDirectory{
			path: goroot, root: &os.Root{}, descriptor: &os.File{}, mount: mount,
			pathClaims: []authorityPathClaim{gorootRootClaim},
		},
		policy:  gorootPolicy(),
		digest:  Digest{0x41},
		entries: []entrySnapshot{bin, goEntry, pkg, pkgTool, toolDirectory, compilerEntry},
	}
	gomodcacheCapture := &treeCapture{
		root:   &retainedDirectory{path: gomodcache, root: &os.Root{}, descriptor: &os.File{}},
		policy: gomodcachePolicy(),
		digest: Digest{0x42},
	}
	gitLeaf := testEnvironmentExecutable(
		"/private/tools/"+gitExecutableBase,
		gitExecutableDigest(),
		9,
		mount,
		[]authorityPathClaim{testEnvironmentDirectoryClaim(10, mount)},
	)
	return environmentAuthorityInputs{
		physicalRoot:  physicalRoot,
		nullDevice:    nullDevice,
		goExecutable:  &goAuthority{retained: goLeaf, seal: validGoAuthority},
		compiler:      &compilerAuthority{retained: compilerLeaf, seal: validCompilerAuthority},
		gitExecutable: &gitAuthority{retained: gitLeaf, seal: validGitAuthority},
		goroot:        gorootCapture,
		gomodcache:    gomodcacheCapture,
	}
}

func testEnvironmentExecutable(
	path string,
	digest Digest,
	inode uint64,
	mount mountSnapshot,
	parentClaims []authorityPathClaim,
) *retainedExecutable {
	uid := uint32(os.Geteuid())
	snapshot := fileSnapshot{
		identity: FileIdentity{
			Device: 1, Inode: inode, UID: uid, Mode: platformModeRegular | 0o700,
			Filesystem: mount.filesystem,
		},
		size: 64,
	}
	return &retainedExecutable{
		leaf: &retainedLeaf{
			path: path, descriptor: &os.File{}, snapshot: snapshot, mount: mount,
			pathClaims: append([]authorityPathClaim(nil), parentClaims...),
		},
		digest: digest,
	}
}

func testEnvironmentDirectoryEntry(path string, inode uint64, mount mountSnapshot) entrySnapshot {
	uid := uint32(os.Geteuid())
	return entrySnapshot{
		path: path,
		kind: entryDirectory,
		file: fileSnapshot{identity: FileIdentity{
			Device: 1, Inode: inode, UID: uid, Mode: platformModeDirectory | 0o700,
			Filesystem: mount.filesystem,
		}},
	}
}

func testEnvironmentDirectoryClaim(inode uint64, mount mountSnapshot) authorityPathClaim {
	return makeAuthorityPathClaim(testEnvironmentDirectoryEntry("claim", inode, mount).file, mount, Digest{})
}

func testEnvironmentWorkspace(path string, git *gitAuthority) *taskWorkspace {
	paths := workspacePaths{taskRoot: path}
	statePath := filepath.Dir(path)
	stateRoot := &retainedDirectory{path: statePath, root: &os.Root{}, descriptor: &os.File{}}
	taskRoot := &retainedDirectory{path: path, root: &os.Root{}, descriptor: &os.File{}}
	spoolRoot := &retainedDirectory{
		path: filepath.Join(path, workspaceObjectSpool), root: &os.Root{}, descriptor: &os.File{},
	}
	workspaceGit := workspaceGitAuthority{}
	if git != nil && git.retained != nil && git.retained.leaf != nil {
		leaf := git.retained.leaf
		workspaceGit = workspaceGitAuthority{
			path: leaf.path, descriptor: leaf.descriptor, snapshot: leaf.snapshot,
			mount: leaf.mount, aclDigest: leaf.aclDigest,
		}
	}
	return &taskWorkspace{
		stateRoot: stateRoot,
		taskRoot:  taskRoot,
		taskName:  filepath.Base(path),
		paths:     paths,
		git:       workspaceGit,
		spool:     &objectSpool{root: spoolRoot},
	}
}

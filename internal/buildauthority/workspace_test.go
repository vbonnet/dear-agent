package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestEnvironmentProfilesAreExactClosedProjections(t *testing.T) {
	t.Setenv("HTTP_PROXY", "ambient-proxy")
	t.Setenv("GIT_SSH_COMMAND", "ambient-helper")
	t.Setenv("GIT_TRACE", "ambient-trace")
	t.Setenv("GOFLAGS", "ambient-flags")
	t.Setenv("GO_TELEMETRY_CHILD_ID", "ambient-telemetry-child")

	taskRoot := "/private/state/.sandbox-gc-build-0123456789abcdef01234567"
	preallocation := mustTestPreallocationEnvironment(t, "/private/toolchain/go", "/private/cache/pkg/mod")
	taskPrivate := mustTestTaskPrivateEnvironment(
		t,
		taskRoot,
		"/private/toolchain/go",
		"/private/cache/pkg/mod",
	)

	wantPreallocation := []string{
		"CGO_ENABLED=0",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_EXEC_PATH=/dev/null",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PROTOCOL_FROM_USER=0",
		"GIT_TEMPLATE_DIR=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GO111MODULE=on",
		"GOARCH=arm64",
		"GOARM64=v8.0",
		"GOCACHE=off",
		"GOENV=off",
		"GOEXPERIMENT=none",
		"GO_EXTLINK_ENABLED=0",
		"GOFIPS140=off",
		"GOFLAGS=-mod=readonly",
		"GOMODCACHE=/private/cache/pkg/mod",
		"GOOS=darwin",
		"GOPATH=/dev/null",
		"GOPROXY=off",
		"GOROOT=/private/toolchain/go",
		"GOSUMDB=off",
		"GOTMPDIR=/dev/null",
		"GOTOOLCHAIN=local",
		"GOVCS=*:off",
		"GOWORK=off",
		"HOME=",
		"LANG=C",
		"LC_ALL=C",
		"PATH=/dev/null",
		"TEMP=/dev/null",
		"TMP=/dev/null",
		"TMPDIR=/dev/null",
		"TZ=UTC",
		"XDG_CONFIG_HOME=",
	}
	wantTaskPrivate := []string{
		"CGO_ENABLED=0",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/private/state/.sandbox-gc-build-0123456789abcdef01234567/git-global.conf",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_EXEC_PATH=/private/state/.sandbox-gc-build-0123456789abcdef01234567/git-exec",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PROTOCOL_FROM_USER=0",
		"GIT_TEMPLATE_DIR=/private/state/.sandbox-gc-build-0123456789abcdef01234567/git-template",
		"GIT_TERMINAL_PROMPT=0",
		"GO111MODULE=on",
		"GOARCH=arm64",
		"GOARM64=v8.0",
		"GOCACHE=/private/state/.sandbox-gc-build-0123456789abcdef01234567/gocache",
		"GOENV=off",
		"GOEXPERIMENT=none",
		"GO_EXTLINK_ENABLED=0",
		"GOFIPS140=off",
		"GOFLAGS=-mod=readonly",
		"GOMODCACHE=/private/cache/pkg/mod",
		"GOOS=darwin",
		"GOPATH=/private/state/.sandbox-gc-build-0123456789abcdef01234567/gopath",
		"GOPROXY=off",
		"GOROOT=/private/toolchain/go",
		"GOSUMDB=off",
		"GOTMPDIR=/private/state/.sandbox-gc-build-0123456789abcdef01234567/gotmp",
		"GOTOOLCHAIN=local",
		"GOVCS=*:off",
		"GOWORK=off",
		"HOME=",
		"LANG=C",
		"LC_ALL=C",
		"PATH=/private/state/.sandbox-gc-build-0123456789abcdef01234567/git-path",
		"TEMP=/private/state/.sandbox-gc-build-0123456789abcdef01234567/tmp",
		"TMP=/private/state/.sandbox-gc-build-0123456789abcdef01234567/tmp",
		"TMPDIR=/private/state/.sandbox-gc-build-0123456789abcdef01234567/tmp",
		"TZ=UTC",
		"XDG_CONFIG_HOME=",
	}
	sort.Strings(wantPreallocation)
	sort.Strings(wantTaskPrivate)
	profiles := []struct {
		name string
		got  []string
		want []string
	}{
		{name: "preallocation", got: preallocation.clone(), want: wantPreallocation},
		{name: "task private", got: taskPrivate.clone(), want: wantTaskPrivate},
	}
	for _, profile := range profiles {
		if !reflect.DeepEqual(profile.got, profile.want) {
			t.Fatalf("%s environment mismatch:\n got: %q\nwant: %q", profile.name, profile.got, profile.want)
		}
		if len(profile.got) != 39 {
			t.Fatalf("%s environment rows = %d, want exactly 39", profile.name, len(profile.got))
		}
		keys := make(map[string]struct{}, len(profile.got))
		for _, row := range profile.got {
			key, _, found := strings.Cut(row, "=")
			if !found || key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(row, '\x00') {
				t.Fatalf("%s environment contains malformed row %q", profile.name, row)
			}
			if _, duplicate := keys[key]; duplicate {
				t.Fatalf("%s environment contains duplicate key %q", profile.name, key)
			}
			keys[key] = struct{}{}
		}
	}
	if strings.Contains(strings.Join(preallocation.clone(), "\n"), taskRoot) {
		t.Fatal("preallocation environment contains a task-private path")
	}
	for _, forbidden := range []string{
		"HTTP_PROXY",
		"GIT_SSH_COMMAND",
		"GIT_TRACE",
		"GOTELEMETRY",
		"GOTELEMETRYDIR",
		"TEST_TELEMETRY_DIR",
		"GO_TELEMETRY_CHILD*",
	} {
		for _, profile := range profiles {
			for _, row := range profile.got {
				key, _, found := strings.Cut(row, "=")
				prefix := strings.TrimSuffix(forbidden, "*")
				if found && (key == forbidden ||
					(strings.HasSuffix(forbidden, "*") && strings.HasPrefix(key, prefix))) {
					t.Fatalf("ambient key %q entered %s environment", forbidden, profile.name)
				}
			}
		}
	}
}

func TestWorkspaceLayoutNamesMatchNormativeTable(t *testing.T) {
	t.Parallel()

	var directories []string
	var files []string
	for _, row := range workspacePolicyRows() {
		switch row.kind {
		case entryDirectory:
			directories = append(directories, row.name)
		case entryRegular:
			files = append(files, row.name)
		}
	}
	sort.Strings(directories)
	wantDirectories := []string{
		"checkout",
		"git-exec",
		"git-hooks",
		"git-path",
		"git-template",
		"gocache",
		"gopath",
		"gotmp",
		"object-spool",
		"outputs",
		"tmp",
	}
	if !reflect.DeepEqual(directories, wantDirectories) {
		t.Fatalf("workspace directories = %q, want %q", directories, wantDirectories)
	}

	sort.Strings(files)
	wantFiles := []string{"git-attributes", "git-excludes", "git-global.conf"}
	if !reflect.DeepEqual(files, wantFiles) {
		t.Fatalf("workspace policy files = %q, want %q", files, wantFiles)
	}
	if workspaceGitLink != "git-path/git" {
		t.Fatalf("workspace Git link = %q", workspaceGitLink)
	}
}

func TestWorkspacePolicyRowsAreFresh(t *testing.T) {
	t.Parallel()

	first := workspacePolicyRows()
	second := workspacePolicyRows()
	first[0].name = "mutated"
	first[0].unchangedEmpty = false
	if second[0].name == "mutated" || !second[0].unchangedEmpty {
		t.Fatalf("workspace policy rows alias: first=%#v second=%#v", first[0], second[0])
	}
}

func TestEnvironmentProfileClonesAreFresh(t *testing.T) {
	preallocation := mustTestPreallocationEnvironment(t, "/private/go", "/private/mod")
	taskPrivate := mustTestTaskPrivateEnvironment(
		t,
		"/private/state/.sandbox-gc-build-0123456789abcdef01234567",
		"/private/go",
		"/private/mod",
	)
	for _, profile := range []struct {
		name  string
		clone func() []string
	}{
		{name: "preallocation", clone: preallocation.clone},
		{name: "task private", clone: taskPrivate.clone},
	} {
		first := profile.clone()
		first[0] = "AMBIENT=1"
		if profile.clone()[0] == first[0] {
			t.Fatalf("mutating returned %s environment changed sealed profile", profile.name)
		}
	}
}

func TestEnvironmentProfilesRejectUnvalidatedPaths(t *testing.T) {
	for _, test := range []struct {
		name       string
		goroot     string
		gomodcache string
	}{
		{name: "relative GOROOT", goroot: "go", gomodcache: "/mod"},
		{name: "unclean GOMODCACHE", goroot: "/go", gomodcache: "/cache/../mod"},
		{name: "NUL GOROOT", goroot: "/go\x00bad", gomodcache: "/mod"},
	} {
		t.Run("preallocation "+test.name, func(t *testing.T) {
			authorities := testEnvironmentAuthorityInputs(test.goroot, test.gomodcache)
			if _, err := newPreallocationEnvironment(authorities); err == nil {
				t.Fatal("invalid path entered closed preallocation environment")
			}
		})
	}

	tests := []struct {
		name       string
		workspace  string
		goroot     string
		gomodcache string
	}{
		{name: "empty workspace", goroot: "/go", gomodcache: "/mod"},
		{name: "relative workspace", workspace: "state/task", goroot: "/go", gomodcache: "/mod"},
		{name: "unclean workspace", workspace: "/state/../task", goroot: "/go", gomodcache: "/mod"},
		{name: "relative GOROOT", workspace: "/state/task", goroot: "go", gomodcache: "/mod"},
		{name: "unclean GOMODCACHE", workspace: "/state/task", goroot: "/go", gomodcache: "/cache/../mod"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorities := testEnvironmentAuthorityInputs(test.goroot, test.gomodcache)
			workspace := testEnvironmentWorkspace(test.workspace, authorities.gitExecutable)
			if _, err := newTaskPrivateEnvironment(workspace, authorities); err == nil {
				t.Fatal("invalid path entered closed task-private environment")
			}
		})
	}
}

func TestTaskRootNameUsesExactCryptographicFraming(t *testing.T) {
	t.Parallel()

	random := make([]byte, taskRootRandomBytes)
	for index := range random {
		random[index] = byte(index)
	}
	name, err := generateTaskRootName(bytes.NewReader(random))
	if err != nil {
		t.Fatalf("generate task-root name: %v", err)
	}
	if want := ".sandbox-gc-build-000102030405060708090a0b"; name != want {
		t.Fatalf("task-root name = %q, want %q", name, want)
	}
	if _, err := generateTaskRootName(bytes.NewReader(random[:len(random)-1])); err == nil {
		t.Fatal("short task-root randomness was accepted")
	}
	if _, err := generateTaskRootName(nil); err == nil {
		t.Fatal("nil task-root randomness was accepted")
	}
}

func TestSpoolInventoryExactAndOneOverLimitsUseInjectedMetadata(t *testing.T) {
	t.Parallel()

	uid, err := effectiveUserID()
	if err != nil {
		t.Fatalf("effective UID: %v", err)
	}
	mount := mountSnapshot{filesystem: [2]int32{7, 11}, flags: 13}
	entry := func(name string, size int64) workspaceEntryClaim {
		return workspaceEntryClaim{
			name: name,
			kind: entryRegular,
			snapshot: fileSnapshot{
				identity:  FileIdentity{Device: 19, Inode: uint64(len(name) + 1), UID: uid, Mode: platformModeRegular | 0o600},
				linkCount: 1,
				size:      size,
			},
			mount: mount,
		}
	}

	exactFiles := make([]workspaceEntryClaim, maximumSpoolFiles)
	for index := range exactFiles {
		exactFiles[index] = entry(fmt.Sprintf("object-%04x", index), 0)
		exactFiles[index].snapshot.identity.Inode = uint64(index + 1)
	}
	if err := validateSpoolInventory(exactFiles, nil, mount, 19); err != nil {
		t.Fatalf("exact file count refused: %v", err)
	}
	oneOverFiles := append(append([]workspaceEntryClaim(nil), exactFiles...), entry("one-over", 0))
	requirePrivateCauses(t, validateSpoolInventory(oneOverFiles, nil, mount, 19), CauseLimit)

	exactBytes := []workspaceEntryClaim{entry("exact-bytes", int64(maximumSpoolBytes))}
	if err := validateSpoolInventory(exactBytes, nil, mount, 19); err != nil {
		t.Fatalf("exact byte count refused: %v", err)
	}
	oneOverBytes := []workspaceEntryClaim{entry("one-over-bytes", int64(maximumSpoolBytes)+1)}
	requirePrivateCauses(t, validateSpoolInventory(oneOverBytes, nil, mount, 19), CauseLimit)
	combinedExact := []workspaceEntryClaim{
		entry("first", int64(maximumSpoolBytes/2)),
		entry("second", int64(maximumSpoolBytes/2)),
	}
	if err := validateSpoolInventory(combinedExact, nil, mount, 19); err != nil {
		t.Fatalf("exact aggregate byte count refused: %v", err)
	}
	combinedExact[1].snapshot.size++
	requirePrivateCauses(t, validateSpoolInventory(combinedExact, nil, mount, 19), CauseLimit)
}

func TestSpoolInventoryRefusesEveryStructuralSubstitution(t *testing.T) {
	t.Parallel()

	uid, err := effectiveUserID()
	if err != nil {
		t.Fatalf("effective UID: %v", err)
	}
	mount := mountSnapshot{filesystem: [2]int32{3, 5}, flags: 7}
	valid := workspaceEntryClaim{
		name: "object",
		kind: entryRegular,
		snapshot: fileSnapshot{
			identity:  FileIdentity{Device: 1, Inode: 2, UID: uid, Mode: platformModeRegular | 0o600},
			linkCount: 1,
			size:      8,
		},
		mount: mount,
	}
	tests := []struct {
		name   string
		mutate func(*workspaceEntryClaim)
		cause  CauseCode
	}{
		{name: "directory", mutate: func(value *workspaceEntryClaim) {
			value.kind = entryDirectory
			value.snapshot.identity.Mode = platformModeDirectory | 0o700
		}, cause: CauseUnsupported},
		{name: "symlink", mutate: func(value *workspaceEntryClaim) {
			value.kind = entrySymlink
			value.snapshot.identity.Mode = platformModeSymlink | 0o777
		}, cause: CauseUnsupported},
		{name: "special", mutate: func(value *workspaceEntryClaim) {
			value.kind = 0
			value.snapshot.identity.Mode = 0o10600
		}, cause: CauseUnsupported},
		{name: "owner", mutate: func(value *workspaceEntryClaim) { value.snapshot.identity.UID++ }, cause: CausePermission},
		{name: "mode", mutate: func(value *workspaceEntryClaim) { value.snapshot.identity.Mode = platformModeRegular | 0o640 }, cause: CausePermission},
		{name: "special bits", mutate: func(value *workspaceEntryClaim) { value.snapshot.identity.Mode |= 0o4000 }, cause: CausePermission},
		{name: "hard link", mutate: func(value *workspaceEntryClaim) { value.snapshot.linkCount = 2 }, cause: CauseIdentity},
		{name: "mount", mutate: func(value *workspaceEntryClaim) { value.mount.flags++ }, cause: CauseUnsupported},
		{name: "device", mutate: func(value *workspaceEntryClaim) { value.snapshot.identity.Device++ }, cause: CauseUnsupported},
		{name: "negative size", mutate: func(value *workspaceEntryClaim) { value.snapshot.size = -1 }, cause: CauseMalformed},
		{name: "nested name", mutate: func(value *workspaceEntryClaim) { value.name = "nested/object" }, cause: CauseMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			requirePrivateCauses(t, validateSpoolInventory([]workspaceEntryClaim{candidate}, nil, mount, 1), test.cause)
		})
	}
	duplicate := []workspaceEntryClaim{valid, valid}
	requirePrivateCauses(t, validateSpoolInventory(duplicate, nil, mount, 1), CauseIdentity)
}

func TestSpoolInventoryBindsGenerationMetadata(t *testing.T) {
	t.Parallel()

	uid, err := effectiveUserID()
	if err != nil {
		t.Fatalf("effective UID: %v", err)
	}
	mount := mountSnapshot{filesystem: [2]int32{17, 23}}
	claim := workspaceEntryClaim{
		name: "object",
		kind: entryRegular,
		snapshot: fileSnapshot{
			identity:   FileIdentity{Device: 29, Inode: 31, UID: uid, Mode: platformModeRegular | 0o600},
			linkCount:  1,
			size:       1,
			generation: 37,
		},
		mount: mount,
	}
	expected := map[string]workspaceEntryClaim{claim.name: claim}
	if err := validateSpoolInventory([]workspaceEntryClaim{claim}, expected, mount, 29); err != nil {
		t.Fatalf("unchanged generation refused: %v", err)
	}
	claim.snapshot.generation++
	requirePrivateCauses(t, validateSpoolInventory([]workspaceEntryClaim{claim}, expected, mount, 29), CauseUnstable)
	claim = expected["object"]
	claim.snapshot.identity.Inode++
	requirePrivateCauses(t, validateSpoolInventory([]workspaceEntryClaim{claim}, expected, mount, 29), CauseUnstable)
}

func TestWorkspaceAllocatesExactShape(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	if !strings.HasPrefix(workspace.taskName, taskRootPrefix) || len(workspace.taskName) != len(taskRootPrefix)+taskRootRandomBytes*2 {
		t.Fatalf("task name %q does not have exact random framing", workspace.taskName)
	}
	if err := workspace.requireInitialShape(context.Background()); err != nil {
		t.Fatalf("initial workspace shape: %v", err)
	}
	if err := workspace.requireBeforeFirstGo(context.Background()); err != nil {
		t.Fatalf("initial Go derived-tree gate: %v", err)
	}
	if err := workspace.requireBeforeGitObjectUse(context.Background()); err != nil {
		t.Fatalf("initial Git spool gate: %v", err)
	}
}

func TestWorkspaceDetectsUnexpectedRowsAndPolicyDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *taskWorkspace)
	}{
		{name: "unexpected top-level row", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			if err := workspace.taskRoot.root.WriteFile("unexpected", nil, 0o600); err != nil {
				t.Fatalf("add unexpected row: %v", err)
			}
		}},
		{name: "unexpected policy subdirectory", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			if err := workspace.taskRoot.root.Mkdir("git-hooks/unexpected", 0o700); err != nil {
				t.Fatalf("add unexpected Git-hooks row: %v", err)
			}
		}},
		{name: "policy file mode", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			if err := workspace.taskRoot.root.Chmod(workspaceGitGlobal, 0o640); err != nil {
				t.Fatalf("change policy mode: %v", err)
			}
		}},
		{name: "policy file bytes", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			if err := workspace.taskRoot.root.WriteFile(workspaceGitGlobal, []byte("x"), 0o600); err != nil {
				t.Fatalf("change policy bytes: %v", err)
			}
		}},
		{name: "policy file hardlink", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			if err := workspace.taskRoot.root.Link(workspaceGitGlobal, "git-global-alias"); err != nil {
				t.Fatalf("hardlink policy file: %v", err)
			}
		}},
		{name: "directory identity", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			if err := workspace.taskRoot.root.Remove(workspaceGitHooks); err != nil {
				t.Fatalf("remove Git hooks: %v", err)
			}
			if err := workspace.taskRoot.root.Mkdir(workspaceGitHooks, 0o700); err != nil {
				t.Fatalf("replace Git hooks: %v", err)
			}
		}},
		{name: "Git link target", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			if err := workspace.taskRoot.root.Remove(workspaceGitLink); err != nil {
				t.Fatalf("remove Git link: %v", err)
			}
			if err := workspace.taskRoot.root.Symlink("/bin/sh", workspaceGitLink); err != nil {
				t.Fatalf("replace Git link: %v", err)
			}
		}},
		{name: "Git target identity", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			moved := workspace.git.path + ".moved"
			if err := os.Rename(workspace.git.path, moved); err != nil {
				t.Fatalf("move retained Git path: %v", err)
			}
			if err := os.WriteFile(workspace.git.path, []byte("replacement Git\n"), 0o700); err != nil {
				t.Fatalf("replace retained Git path: %v", err)
			}
			if err := os.Chmod(workspace.git.path, 0o700); err != nil {
				t.Fatalf("chmod replacement Git path: %v", err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace, _, _ := newLiveWorkspace(t)
			test.mutate(t, workspace)
			if err := workspace.requireUnchangedPolicy(context.Background()); err == nil {
				t.Fatal("workspace policy drift was accepted")
			}
		})
	}
}

func TestObjectSpoolGenerationWritesSealsEmptiesAndGatesGit(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin object-spool generation: %v", err)
	}
	file, err := generation.create(context.Background())
	if err != nil {
		t.Fatalf("create object-spool file: %v", err)
	}
	if written, err := file.write(context.Background(), []byte("object bytes")); err != nil || written != len("object bytes") {
		t.Fatalf("write object-spool file: written=%d err=%v", written, err)
	}
	reader, err := file.seal(context.Background())
	if err != nil {
		t.Fatalf("seal object-spool file: %v", err)
	}
	read := make([]byte, len("object bytes"))
	if count, err := reader.ReadAt(read, 0); err != nil || count != len(read) || string(read) != "object bytes" {
		t.Fatalf("read retained sealed spool file: count=%d bytes=%q err=%v", count, read, err)
	}
	if err := workspace.requireBeforeGitObjectUse(context.Background()); err == nil {
		t.Fatal("Git object use accepted an active spool generation")
	}
	if err := generation.empty(context.Background()); err != nil {
		t.Fatalf("empty object-spool generation: %v", err)
	}
	if _, err := reader.ReadAt(read, 0); err == nil {
		t.Fatal("sealed reader remained live after its generation was removed")
	}
	if err := workspace.requireBeforeGitObjectUse(context.Background()); err != nil {
		t.Fatalf("Git object use refused proved-empty spool: %v", err)
	}
}

func TestObjectSpoolWriteAccountingBindsOnlyReportedPrefix(t *testing.T) {
	contents := []byte("reported-prefix")
	generation := &spoolGeneration{}
	file := &spoolFile{generation: generation, writeHash: sha256.New()}
	if err := file.accountWriteLocked(contents, len("reported")); err != nil {
		t.Fatalf("account partial written prefix: %v", err)
	}
	wantHash := Digest(sha256.Sum256([]byte("reported")))
	if generation.bytes != uint64(len("reported")) || file.written != uint64(len("reported")) ||
		digestFromHasher(file.writeHash) != wantHash {
		t.Fatalf("partial write accounting = generation=%d file=%d hash=%x",
			generation.bytes, file.written, digestFromHasher(file.writeHash))
	}
	if err := file.accountWriteLocked(contents, len(contents)+1); err == nil {
		t.Fatal("invalid written count was accepted")
	}
	if !generation.poisoned {
		t.Fatal("invalid written count did not poison generation")
	}
	if generation.bytes != uint64(len("reported")) || file.written != uint64(len("reported")) ||
		digestFromHasher(file.writeHash) != wantHash {
		t.Fatal("invalid written count mutated prior accounting")
	}
}

func TestObjectSpoolRefusesHardlinksSubdirectoriesAndSymlinks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *taskWorkspace)
	}{
		{name: "subdirectory", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			if err := workspace.spool.root.root.Mkdir("bad", 0o700); err != nil {
				t.Fatalf("make spool subdirectory: %v", err)
			}
		}},
		{name: "symlink", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			if err := workspace.spool.root.root.Symlink("../git-global.conf", "bad"); err != nil {
				t.Fatalf("make spool symlink: %v", err)
			}
		}},
		{name: "hardlink", mutate: func(t *testing.T, workspace *taskWorkspace) {
			t.Helper()
			generation, err := workspace.spool.beginGeneration(context.Background())
			if err != nil {
				t.Fatalf("begin generation: %v", err)
			}
			file, err := generation.create(context.Background())
			if err != nil {
				t.Fatalf("create spool file: %v", err)
			}
			if _, err := file.seal(context.Background()); err != nil {
				t.Fatalf("seal spool file: %v", err)
			}
			if err := workspace.spool.root.root.Link(file.name, "alias"); err != nil {
				t.Fatalf("hardlink spool file: %v", err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace, _, _ := newLiveWorkspace(t)
			test.mutate(t, workspace)
			entries, err := workspace.spool.captureInventory(context.Background())
			if err == nil {
				err = validateSpoolInventory(
					entries,
					nil,
					workspace.spool.root.mount,
					workspace.spool.root.snapshot.identity.Device,
				)
			}
			if err == nil {
				t.Fatal("invalid object-spool entry was accepted")
			}
		})
	}
}

func TestObjectSpoolCancellationDoesNotWriteAndCanBeCleaned(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin generation: %v", err)
	}
	file, err := generation.create(context.Background())
	if err != nil {
		t.Fatalf("create spool file: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if written, err := file.write(ctx, []byte("must not be written")); err == nil || written != 0 {
		t.Fatalf("canceled write = (%d, %v), want zero and refusal", written, err)
	}
	if err := generation.empty(context.Background()); err != nil {
		t.Fatalf("clean canceled generation: %v", err)
	}
	if err := workspace.spool.requireEmpty(context.Background()); err != nil {
		t.Fatalf("spool not empty after canceled generation cleanup: %v", err)
	}
}

func TestObjectSpoolRetainedReaderRejectsPathReplacementAndWithholdsBytes(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin object-spool generation: %v", err)
	}
	file, err := generation.create(context.Background())
	if err != nil {
		t.Fatalf("create object-spool file: %v", err)
	}
	original := []byte("retained object bytes")
	if written, err := file.write(context.Background(), original); err != nil || written != len(original) {
		t.Fatalf("write object-spool file: written=%d err=%v", written, err)
	}
	reader, err := file.seal(context.Background())
	if err != nil {
		t.Fatalf("seal object-spool file: %v", err)
	}
	read := make([]byte, len(original))
	if count, err := reader.ReadAt(read, 0); err != nil || count != len(read) || !bytes.Equal(read, original) {
		t.Fatalf("initial retained read: count=%d bytes=%q err=%v", count, read, err)
	}

	path := filepath.Join(workspace.spool.root.path, file.name)
	parked := path + ".parked"
	if err := os.Rename(path, parked); err != nil {
		t.Fatalf("park claimed spool file: %v", err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatalf("write replacement spool file: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod replacement spool file: %v", err)
	}
	for index := range read {
		read[index] = 0xff
	}
	if count, err := reader.ReadAt(read, 0); err == nil || count != 0 || !bytes.Equal(read, make([]byte, len(read))) {
		t.Fatalf("retained reader exposed bytes after replacement: count=%d bytes=%q err=%v", count, read, err)
	}
	if !generation.poisoned {
		t.Fatal("replacement-path read did not poison generation")
	}
	if err := generation.empty(context.Background()); err == nil {
		t.Fatal("generation cleanup accepted renamed claimed inode and replacement")
	}
	for _, retained := range []string{path, parked} {
		if _, err := os.Lstat(retained); err != nil {
			t.Fatalf("failed generation preflight mutated %q: %v", retained, err)
		}
	}
}

func TestObjectSpoolRetainedReaderDoesNotReopenMovedSpoolPath(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	_, file, reader, original := newSealedSpoolFile(t, workspace, []byte("retained-only bytes"))
	originalRoot := workspace.spool.root.path
	parkedRoot := originalRoot + ".parked"
	if err := os.Rename(originalRoot, parkedRoot); err != nil {
		t.Fatalf("park retained object-spool directory: %v", err)
	}
	if err := os.Mkdir(originalRoot, 0o700); err != nil {
		t.Fatalf("create hostile replacement object-spool directory: %v", err)
	}
	replacementPath := filepath.Join(originalRoot, file.name)
	if err := os.WriteFile(replacementPath, []byte("hostile replacement"), 0o600); err != nil {
		t.Fatalf("write hostile replacement spool file: %v", err)
	}
	if err := os.Chmod(replacementPath, 0o600); err != nil {
		t.Fatalf("chmod hostile replacement spool file: %v", err)
	}
	read := make([]byte, len(original))
	if count, err := reader.ReadAt(read, 0); err != nil || count != len(read) || !bytes.Equal(read, original) {
		t.Fatalf("retained reader reopened hostile old path: count=%d bytes=%q err=%v", count, read, err)
	}
}

func TestObjectSpoolSealRejectsSameSizeOverwriteAndPoisonsGeneration(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin object-spool generation: %v", err)
	}
	file, err := generation.create(context.Background())
	if err != nil {
		t.Fatalf("create object-spool file: %v", err)
	}
	original := []byte("original-object")
	replacement := []byte("mutated-object!")
	if len(original) != len(replacement) {
		t.Fatal("test fixture must preserve file size")
	}
	if _, err := file.write(context.Background(), original); err != nil {
		t.Fatalf("write object-spool file: %v", err)
	}
	path := filepath.Join(workspace.spool.root.path, file.name)
	overwriteFileAt(t, path, replacement)
	if reader, err := file.seal(context.Background()); err == nil || reader != nil {
		t.Fatalf("same-size pre-seal overwrite accepted: reader=%v err=%v", reader, err)
	}
	if !generation.poisoned {
		t.Fatal("failed content seal did not poison generation")
	}
	if err := generation.empty(context.Background()); err == nil {
		t.Fatal("poisoned generation became removable")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("poisoned pre-seal file was not preserved: %v", err)
	}
}

func TestObjectSpoolSealAuthorityFailuresPoisonAndPreserve(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *taskWorkspace, *spoolFile)
	}{
		{
			name: "closed retained descriptor",
			mutate: func(t *testing.T, _ *taskWorkspace, file *spoolFile) {
				t.Helper()
				if err := file.descriptor.Close(); err != nil {
					t.Fatalf("close retained spool descriptor: %v", err)
				}
			},
		},
		{
			name: "hard-linked retained inode",
			mutate: func(t *testing.T, workspace *taskWorkspace, file *spoolFile) {
				t.Helper()
				if err := workspace.spool.root.root.Link(file.name, file.name+".alias"); err != nil {
					t.Fatalf("hard-link retained spool file: %v", err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace, _, _ := newLiveWorkspace(t)
			generation, err := workspace.spool.beginGeneration(context.Background())
			if err != nil {
				t.Fatalf("begin object-spool generation: %v", err)
			}
			file, err := generation.create(context.Background())
			if err != nil {
				t.Fatalf("create object-spool file: %v", err)
			}
			if _, err := file.write(context.Background(), []byte("seal authority")); err != nil {
				t.Fatalf("write object-spool file: %v", err)
			}
			test.mutate(t, workspace, file)
			if reader, err := file.seal(context.Background()); err == nil || reader != nil {
				t.Fatalf("authority seal failure accepted: reader=%v err=%v", reader, err)
			}
			if !generation.poisoned {
				t.Fatal("authority seal failure did not poison generation")
			}
			if _, err := os.Lstat(filepath.Join(workspace.spool.root.path, file.name)); err != nil {
				t.Fatalf("authority seal failure did not preserve named file: %v", err)
			}
		})
	}
}

func TestObjectSpoolReaderRejectsSameSizeOverwriteAndWithholdsBytes(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, file, reader, original := newSealedSpoolFile(t, workspace, []byte("sealed-original"))
	replacement := []byte("sealed-mutated!")
	if len(original) != len(replacement) {
		t.Fatal("test fixture must preserve file size")
	}
	path := filepath.Join(workspace.spool.root.path, file.name)
	overwriteFileAt(t, path, replacement)
	read := bytes.Repeat([]byte{0xff}, len(original))
	if count, err := reader.ReadAt(read, 0); err == nil || count != 0 || !bytes.Equal(read, make([]byte, len(read))) {
		t.Fatalf("same-size overwrite read = count=%d bytes=%q err=%v", count, read, err)
	}
	if !generation.poisoned {
		t.Fatal("same-size post-seal overwrite did not poison generation")
	}
	if err := generation.empty(context.Background()); err == nil {
		t.Fatal("generation with post-seal drift became removable")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("post-seal drift was not preserved: %v", err)
	}
}

func TestObjectSpoolReaderWithholdsBytesChangedDuringRead(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, file, reader, original := newSealedSpoolFile(t, workspace, []byte("read-original"))
	replacement := []byte("read-mutated!")
	if len(original) != len(replacement) {
		t.Fatal("test fixture must preserve file size")
	}
	path := filepath.Join(workspace.spool.root.path, file.name)
	read := bytes.Repeat([]byte{0xff}, len(original))
	count, err := reader.readAtWith(read, 0, func() error {
		return overwriteFileAtError(path, replacement)
	})
	if err == nil || count != 0 || !bytes.Equal(read, make([]byte, len(read))) {
		t.Fatalf("during-read overwrite = count=%d bytes=%q err=%v", count, read, err)
	}
	if !generation.poisoned {
		t.Fatal("during-read overwrite did not poison generation")
	}
	if err := generation.empty(context.Background()); err == nil {
		t.Fatal("generation changed during read became removable")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("during-read drift was not preserved: %v", err)
	}
}

func TestObjectSpoolRemovalRejectsSameSizeContentDrift(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, file, _, original := newSealedSpoolFile(t, workspace, []byte("remove-original"))
	replacement := []byte("remove-mutated!")
	if len(original) != len(replacement) {
		t.Fatal("test fixture must preserve file size")
	}
	path := filepath.Join(workspace.spool.root.path, file.name)
	overwriteFileAt(t, path, replacement)
	if err := generation.empty(context.Background()); err == nil {
		t.Fatal("same-size removal drift was accepted")
	}
	if !generation.poisoned {
		t.Fatal("same-size removal drift did not poison generation")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("drifted retained file was not preserved: %v", err)
	}
}

func TestObjectSpoolStaleGenerationCannotMutateCurrentGeneration(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	first, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin first generation: %v", err)
	}
	staleFile, err := first.create(context.Background())
	if err != nil {
		t.Fatalf("create first-generation file: %v", err)
	}
	if _, err := staleFile.write(context.Background(), []byte("first")); err != nil {
		t.Fatalf("write first-generation file: %v", err)
	}
	if err := first.empty(context.Background()); err != nil {
		t.Fatalf("empty first generation: %v", err)
	}
	current, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin current generation: %v", err)
	}

	if _, err := first.create(context.Background()); err == nil {
		t.Fatal("stale generation created a file")
	}
	if _, err := staleFile.write(context.Background(), []byte("stale")); err == nil {
		t.Fatal("stale file accepted a write")
	}
	if _, err := staleFile.seal(context.Background()); err == nil {
		t.Fatal("stale file was resealed")
	}
	if current.next != 0 || current.bytes != 0 || len(current.files) != 0 || len(current.claims) != 0 {
		t.Fatalf("stale generation mutated current state: %#v", current)
	}
	if err := current.empty(context.Background()); err != nil {
		t.Fatalf("empty current generation: %v", err)
	}
}

func TestObjectSpoolFailedDrainIsPermanentlyNonRemovable(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin generation: %v", err)
	}
	file, err := generation.create(context.Background())
	if err != nil {
		t.Fatalf("create spool file: %v", err)
	}
	if _, err := file.write(context.Background(), []byte("owned")); err != nil {
		t.Fatalf("write spool file: %v", err)
	}
	if _, err := file.seal(context.Background()); err != nil {
		t.Fatalf("seal spool file: %v", err)
	}
	unexpected := filepath.Join(workspace.spool.root.path, "unexpected")
	if err := os.WriteFile(unexpected, nil, 0o600); err != nil {
		t.Fatalf("write unexpected spool row: %v", err)
	}
	if err := os.Chmod(unexpected, 0o600); err != nil {
		t.Fatalf("chmod unexpected spool row: %v", err)
	}
	if err := generation.empty(context.Background()); err == nil {
		t.Fatal("generation drain accepted unexpected membership")
	}
	if !generation.poisoned {
		t.Fatal("failed generation drain did not latch non-removable state")
	}
	if err := os.Remove(unexpected); err != nil {
		t.Fatalf("remove adversarial row after failed drain: %v", err)
	}
	if err := generation.empty(context.Background()); err == nil {
		t.Fatal("failed generation drain became removable after external repair")
	}
	if _, err := os.Stat(filepath.Join(workspace.spool.root.path, file.name)); err != nil {
		t.Fatalf("poisoned generation mutated owned row on retry: %v", err)
	}
}

func TestObjectSpoolCloseFailureIsStickyForNormativeCleanup(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin generation: %v", err)
	}
	file, err := generation.create(context.Background())
	if err != nil {
		t.Fatalf("create spool file: %v", err)
	}
	if err := file.descriptor.Close(); err != nil {
		t.Fatalf("preclose spool descriptor: %v", err)
	}
	first := workspace.spool.close()
	second := workspace.spool.close()
	if first == nil || second == nil {
		t.Fatalf("spool close errors = (%v, %v), want sticky failures", first, second)
	}
	requirePrivateCauses(t, first, CauseDescriptorClose)
	requirePrivateCauses(t, second, CauseDescriptorClose)

	report := refusalReport(t, workspace.cleanup(context.Background(), nil))
	if report.DescriptorClose == nil || report.DescriptorClose.Operation != OperationCloseNonRoot ||
		report.Recovery == nil {
		t.Fatalf("normative cleanup lost prior spool close failure: %#v", report)
	}
}

func TestObjectSpoolReaderAndRemovalLifecycleIsRaceSafe(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin object-spool generation: %v", err)
	}
	file, err := generation.create(context.Background())
	if err != nil {
		t.Fatalf("create object-spool file: %v", err)
	}
	contents := []byte("race-safe retained bytes")
	if _, err := file.write(context.Background(), contents); err != nil {
		t.Fatalf("write object-spool file: %v", err)
	}
	reader, err := file.seal(context.Background())
	if err != nil {
		t.Fatalf("seal object-spool file: %v", err)
	}

	start := make(chan struct{})
	errorsSeen := make(chan error, 33)
	var wait sync.WaitGroup
	for range 32 {
		wait.Go(func() {
			<-start
			const sentinel = byte(0xa5)
			read := bytes.Repeat([]byte{sentinel}, len(contents))
			count, readErr := reader.ReadAt(read, 0)
			if readErr == nil {
				if count != len(read) || !bytes.Equal(read, contents) {
					errorsSeen <- fmt.Errorf("successful retained read = (%d, %q)", count, read)
				}
				return
			}
			if count != 0 {
				errorsSeen <- fmt.Errorf("failed retained read exposed count %d", count)
				return
			}
			for _, value := range read {
				if value != 0 && value != sentinel {
					errorsSeen <- fmt.Errorf("failed retained read exposed object bytes %q", read)
					return
				}
			}
		})
	}
	wait.Go(func() {
		<-start
		if emptyErr := generation.empty(context.Background()); emptyErr != nil {
			errorsSeen <- fmt.Errorf("empty racing generation: %w", emptyErr)
		}
	})
	close(start)
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Error(err)
	}
	if err := workspace.spool.requireEmpty(context.Background()); err != nil {
		t.Fatalf("spool not empty after raced lifecycle: %v", err)
	}
}

func TestWorkspaceCleanupPreservesEvenZeroFileActiveSpoolGeneration(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	if _, err := workspace.spool.beginGeneration(context.Background()); err != nil {
		t.Fatalf("begin zero-file generation: %v", err)
	}
	taskPath := workspace.paths.taskRoot
	err := workspace.cleanup(context.Background(), &FailureRecord{
		Phase:     PhaseBuildAGM,
		Operation: OperationExecute,
		Causes:    []CauseCode{CauseChildExit},
	})
	report := refusalReport(t, err)
	if report.Recovery == nil || report.Recovery.TaskRoot == nil || report.Cleanup == nil {
		t.Fatalf("active-generation cleanup report = %#v", report)
	}
	if !workspace.spool.hasIncompleteGeneration() {
		t.Fatal("cleanup lost active-generation ownership state")
	}
	if _, err := os.Stat(taskPath); err != nil {
		t.Fatalf("cleanup removed task with incomplete zero-file generation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(taskPath, workspaceGitGlobal)); err != nil {
		t.Fatalf("cleanup partially deleted ledger before active-generation refusal: %v", err)
	}
}

func TestWorkspaceCleanupPreflightsNamedTaskBeforeDeletingLedger(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	original := workspace.paths.taskRoot
	parked := original + ".parked"
	if err := os.Rename(original, parked); err != nil {
		t.Fatalf("park retained task root: %v", err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatalf("create replacement task root: %v", err)
	}
	if err := os.Chmod(original, 0o700); err != nil {
		t.Fatalf("chmod replacement task root: %v", err)
	}

	report := refusalReport(t, workspace.cleanup(context.Background(), nil))
	if report.Recovery == nil || report.Cleanup == nil {
		t.Fatalf("renamed task cleanup report = %#v", report)
	}
	if _, err := os.Stat(filepath.Join(parked, workspaceGitGlobal)); err != nil {
		t.Fatalf("cleanup deleted retained-task ledger before binding refusal: %v", err)
	}
	if entries, err := os.ReadDir(original); err != nil || len(entries) != 0 {
		t.Fatalf("replacement task changed during refused cleanup: entries=%v err=%v", entries, err)
	}
}

func TestWorkspaceCleanupPreflightsGitLinkTargetBeforeDeletingLedger(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	taskPath := workspace.paths.taskRoot
	parkedGit := workspace.git.path + ".parked"
	if err := os.Rename(workspace.git.path, parkedGit); err != nil {
		t.Fatalf("park retained Git path: %v", err)
	}
	if err := os.WriteFile(workspace.git.path, []byte("replacement Git\n"), 0o700); err != nil {
		t.Fatalf("write replacement Git: %v", err)
	}
	if err := os.Chmod(workspace.git.path, 0o700); err != nil {
		t.Fatalf("chmod replacement Git: %v", err)
	}
	report := refusalReport(t, workspace.cleanup(context.Background(), nil))
	if report.Recovery == nil || report.Cleanup == nil {
		t.Fatalf("Git-target drift cleanup report = %#v", report)
	}
	for _, name := range []string{workspaceGitGlobal, workspaceGitAttributes, workspaceGitExcludes, workspaceGitLink} {
		if _, err := os.Lstat(filepath.Join(taskPath, name)); err != nil {
			t.Fatalf("cleanup deleted %q before Git-target refusal: %v", name, err)
		}
	}
}

func TestWorkspaceCleanupRechecksFrozenMembershipBeforeFirstUnlink(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	taskPath := workspace.paths.taskRoot
	workspace.beforeRemove = func() error {
		path := filepath.Join(taskPath, "late-unexpected")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return err
		}
		return os.Chmod(path, 0o600)
	}
	report := refusalReport(t, workspace.cleanup(context.Background(), nil))
	if report.Recovery == nil || report.Cleanup == nil {
		t.Fatalf("late-membership cleanup report = %#v", report)
	}
	for _, name := range []string{workspaceGitGlobal, workspaceGitAttributes, workspaceGitExcludes, "late-unexpected"} {
		if _, err := os.Lstat(filepath.Join(taskPath, name)); err != nil {
			t.Fatalf("cleanup deleted %q after frozen membership drift: %v", name, err)
		}
	}
}

func TestPristineWorkspaceCleanupRemovesCompleteInitialLedger(t *testing.T) {
	workspace, stateRoot, _ := newLiveWorkspace(t)
	taskPath := workspace.paths.taskRoot
	if err := workspace.preflightOwnedInitialEntries(context.Background()); err != nil {
		t.Fatalf("preflight pristine initial ledger: %v", err)
	}
	if err := workspace.cleanup(context.Background(), nil); err != nil {
		t.Fatalf("clean pristine workspace: %v", err)
	}
	if _, err := os.Lstat(taskPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pristine workspace absence was not proved: %v", err)
	}
	if _, err := stateRoot.descriptor.Stat(); err == nil {
		t.Fatal("pristine cleanup did not close StateRoot")
	}
}

func TestRetainedWorkspaceLeafRemovalProvesClaimedInodeUnlinked(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	claim := workspace.fileClaims[workspaceGitGlobal]
	descriptor, err := openRelativeNoFollow(
		int(workspace.taskRoot.descriptor.Fd()),
		claim.name,
		entryRegular,
	)
	if err != nil {
		t.Fatalf("open retained policy leaf: %v", err)
	}
	defer func() { _ = descriptor.Close() }()
	if err := removeRetainedWorkspaceLeafWith(
		context.Background(),
		workspace.taskRoot,
		claim,
		descriptor,
		func() error { return workspace.taskRoot.root.Remove(claim.name) },
	); err != nil {
		t.Fatalf("remove retained policy leaf: %v", err)
	}
	after, _, _, err := inspectDescriptorContext(context.Background(), descriptor)
	if err != nil {
		t.Fatalf("inspect retained unlinked policy leaf: %v", err)
	}
	if after.linkCount != 0 || !sameFilesystemObject(after.identity, claim.snapshot.identity) {
		t.Fatalf("retained unlink proof = %#v, want claimed inode with zero links", after)
	}
	if _, err := workspace.taskRoot.root.Lstat(claim.name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed policy name remains: %v", err)
	}
}

func TestRetainedWorkspaceLeafRemovalRejectsRenameSwapWithoutFalseSuccess(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	claim := workspace.fileClaims[workspaceGitGlobal]
	descriptor, err := openRelativeNoFollow(
		int(workspace.taskRoot.descriptor.Fd()),
		claim.name,
		entryRegular,
	)
	if err != nil {
		t.Fatalf("open retained policy leaf: %v", err)
	}
	defer func() { _ = descriptor.Close() }()
	original := filepath.Join(workspace.paths.taskRoot, claim.name)
	parked := original + ".parked"
	removeErr := removeRetainedWorkspaceLeafWith(
		context.Background(),
		workspace.taskRoot,
		claim,
		descriptor,
		func() error {
			if err := os.Rename(original, parked); err != nil {
				return err
			}
			if err := os.WriteFile(original, []byte("replacement"), 0o600); err != nil {
				return err
			}
			if err := os.Chmod(original, 0o600); err != nil {
				return err
			}
			return workspace.taskRoot.root.Remove(claim.name)
		},
	)
	if removeErr == nil {
		t.Fatal("rename/swap removal falsely proved claimed inode absent")
	}
	requirePrivateCauses(t, removeErr, CauseIdentity)
	parkedInfo, err := os.Lstat(parked)
	if err != nil {
		t.Fatalf("claimed inode was not preserved at parked name: %v", err)
	}
	retainedInfo, err := descriptor.Stat()
	if err != nil {
		t.Fatalf("stat retained claimed inode: %v", err)
	}
	if !os.SameFile(parkedInfo, retainedInfo) {
		t.Fatal("parked name is not the retained claimed inode")
	}
}

func TestRetainedWorkspaceLeafRemovalRequiresExactSpoolContentClaim(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	_, file, _, _ := newSealedSpoolFile(t, workspace, []byte("content-bound removal"))
	claim := file.generation.claims[file.name]
	claim.contentHash[0] ^= 0xff
	path := filepath.Join(workspace.spool.root.path, file.name)
	removeErr := removeRetainedWorkspaceLeafWith(
		context.Background(),
		workspace.spool.root,
		claim,
		file.descriptor,
		func() error { return workspace.spool.root.root.Remove(claim.name) },
	)
	if removeErr == nil {
		t.Fatal("incorrect retained content claim authorized removal")
	}
	requirePrivateCauses(t, removeErr, CauseUnstable)
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("failed content proof removed claimed spool file: %v", err)
	}
}

func TestRetainedSpoolRemovalRejectsContentDriftDuringUnlink(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, file, _, original := newSealedSpoolFile(t, workspace, []byte("unlink-original"))
	replacement := []byte("unlink-mutated!")
	if len(original) != len(replacement) {
		t.Fatal("test fixture must preserve file size")
	}
	claim := generation.claims[file.name]
	path := filepath.Join(workspace.spool.root.path, file.name)
	removeErr := removeRetainedWorkspaceLeafWith(
		context.Background(),
		workspace.spool.root,
		claim,
		file.descriptor,
		func() error {
			written, writeErr := file.descriptor.WriteAt(replacement, 0)
			if writeErr == nil && written != len(replacement) {
				writeErr = io.ErrShortWrite
			}
			if writeErr != nil {
				return writeErr
			}
			if err := file.descriptor.Sync(); err != nil {
				return err
			}
			return workspace.spool.root.root.Remove(claim.name)
		},
	)
	if removeErr == nil {
		t.Fatal("during-unlink content drift was reported as successful removal")
	}
	requirePrivateCauses(t, removeErr, CauseUnstable)
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("adversarial unlink result = %v, want absent name with retained failure", err)
	}
	retained, _, _, err := inspectDescriptorContext(context.Background(), file.descriptor)
	if err != nil {
		t.Fatalf("inspect retained inode after ambiguous unlink: %v", err)
	}
	if retained.linkCount != 0 {
		t.Fatalf("retained inode link count = %d, want zero", retained.linkCount)
	}
	if err := generation.empty(context.Background()); err == nil {
		t.Fatal("generation with ambiguous content unlink became removable")
	}
	if !generation.poisoned {
		t.Fatal("ambiguous content unlink did not poison generation")
	}
}

func TestRetainedSpoolRemovalPostUnlinkRequiresExactContentHash(t *testing.T) {
	workspace, _, _ := newLiveWorkspace(t)
	generation, file, _, _ := newSealedSpoolFile(t, workspace, []byte("post-unlink content proof"))
	expected := generation.claims[file.name]
	remove := func() error { return workspace.spool.root.root.Remove(expected.name) }
	before, err := prepareRetainedWorkspaceLeafRemoval(
		context.Background(),
		workspace.spool.root,
		expected,
		file.descriptor,
		remove,
	)
	if err != nil {
		t.Fatalf("prepare retained spool removal: %v", err)
	}
	if err := remove(); err != nil {
		t.Fatalf("unlink retained spool file: %v", err)
	}
	incorrectContent := expected
	incorrectContent.contentHash[0] ^= 0xff
	proofErr := proveRetainedWorkspaceLeafRemoval(
		context.Background(),
		workspace.spool.root,
		incorrectContent,
		file.descriptor,
		before,
	)
	if proofErr == nil {
		t.Fatal("post-unlink proof accepted an incorrect retained content hash")
	}
	requirePrivateCauses(t, proofErr, CauseUnstable)
	after, _, _, err := inspectDescriptorContext(context.Background(), file.descriptor)
	if err != nil {
		t.Fatalf("inspect normally unlinked retained inode: %v", err)
	}
	if after.linkCount != 0 || after.size != expected.snapshot.size ||
		after.mtimeSec != expected.snapshot.mtimeSec || after.mtimeNsec != expected.snapshot.mtimeNsec {
		t.Fatalf("normal unlink metadata drifted: after=%#v expected=%#v", after, expected.snapshot)
	}
	if _, err := workspace.spool.root.root.Lstat(expected.name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("normal unlink did not remove retained name: %v", err)
	}
}

func TestWorkspaceAllocationHonorsPreCanceledContextWithoutScratch(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("workspace authority is supported only on darwin/arm64")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve fixture root: %v", err)
	}
	statePath := filepath.Join(base, "state")
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatalf("make StateRoot: %v", err)
	}
	if err := os.Chmod(statePath, 0o700); err != nil {
		t.Fatalf("chmod StateRoot: %v", err)
	}
	stateRoot, err := admitStateRoot(statePath)
	if err != nil {
		t.Fatalf("admit StateRoot: %v", err)
	}
	t.Cleanup(func() { _ = stateRoot.close() })
	git, descriptor := newLiveGitAuthority(t, base)
	t.Cleanup(func() { _ = descriptor.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if workspace, err := allocateWorkspace(ctx, stateRoot, git); err == nil || workspace != nil {
		t.Fatalf("canceled allocation = (%v, %v), want nil workspace and refusal", workspace, err)
	}
	if _, err := stateRoot.descriptor.Stat(); err != nil {
		t.Fatalf("preallocation cancellation closed caller-owned StateRoot: %v", err)
	}
	directory, err := os.Open(statePath)
	if err != nil {
		t.Fatalf("open StateRoot: %v", err)
	}
	defer directory.Close()
	if names, err := directory.Readdirnames(1); !errors.Is(err, io.EOF) || len(names) != 0 {
		t.Fatalf("canceled allocation created scratch: names=%q err=%v", names, err)
	}
}

func TestWorkspacePostMkdirUnobservedIdentityReturnsRecoveryAndClosesStateRoot(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("workspace authority is supported only on darwin/arm64")
	}
	base, stateRoot, git, gitDescriptor := newWorkspaceAllocationFixture(t)
	_ = gitDescriptor
	stateIdentity := stateRoot.snapshot.identity
	randomBytes := bytes.Repeat([]byte{0x31}, taskRootRandomBytes)
	taskName := taskRootPrefix + fmt.Sprintf("%x", randomBytes)
	taskPath := filepath.Join(base, "state", taskName)

	workspace, err := allocateWorkspaceWith(
		context.Background(),
		stateRoot,
		git,
		workspaceAllocationDependencies{
			random: bytes.NewReader(randomBytes),
			retain: func(context.Context, *retainedDirectory, string, uint32) (*retainedDirectory, error) {
				return nil, fail(CauseUnstable, "injected post-mkdir retention failure")
			},
		},
	)
	if workspace != nil {
		t.Fatalf("post-mkdir retention failure returned workspace %#v", workspace)
	}
	report := refusalReport(t, err)
	if report.Primary == nil || report.Primary.Phase != PhaseWorkspace ||
		report.Primary.Operation != OperationOpen ||
		!reflect.DeepEqual(report.Primary.Causes, []CauseCode{CauseUnstable}) {
		t.Fatalf("Primary = %#v", report.Primary)
	}
	if report.Cleanup == nil || report.Cleanup.Operation != OperationRemove ||
		!reflect.DeepEqual(report.Cleanup.Causes, []CauseCode{CauseIdentity}) {
		t.Fatalf("Cleanup = %#v", report.Cleanup)
	}
	if report.Recovery == nil || report.Recovery.TaskRoot != nil ||
		report.Recovery.TaskPath != taskPath || report.Recovery.StateRoot != stateIdentity {
		t.Fatalf("Recovery = %#v", report.Recovery)
	}
	if _, statErr := os.Stat(taskPath); statErr != nil {
		t.Fatalf("unobserved task allocation was not preserved: %v", statErr)
	}
	if _, statErr := stateRoot.descriptor.Stat(); statErr == nil {
		t.Fatal("cleanup did not close its StateRoot authority")
	}
}

func TestWorkspaceRetentionDependencyCannotLoseAllocatedTask(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("workspace authority is supported only on darwin/arm64")
	}
	tests := []struct {
		name   string
		retain func(**retainedDirectory) taskRootRetainFunc
	}{
		{
			name: "nil root without error",
			retain: func(**retainedDirectory) taskRootRetainFunc {
				return func(context.Context, *retainedDirectory, string, uint32) (*retainedDirectory, error) {
					return nil, nil
				}
			},
		},
		{
			name: "root returned with error",
			retain: func(returned **retainedDirectory) taskRootRetainFunc {
				return func(ctx context.Context, parent *retainedDirectory, name string, mode uint32) (*retainedDirectory, error) {
					root, err := retainCreatedWorkspaceDirectory(ctx, parent, name, mode)
					if err != nil {
						return nil, err
					}
					*returned = root
					return root, fail(CauseUnstable, "injected contradictory retention result")
				}
			},
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base, stateRoot, git, gitDescriptor := newWorkspaceAllocationFixture(t)
			_ = gitDescriptor
			var returned *retainedDirectory
			randomBytes := bytes.Repeat([]byte{byte(0x51 + index)}, taskRootRandomBytes)
			taskPath := filepath.Join(base, "state", taskRootPrefix+fmt.Sprintf("%x", randomBytes))
			workspace, err := allocateWorkspaceWith(
				context.Background(),
				stateRoot,
				git,
				workspaceAllocationDependencies{
					random: bytes.NewReader(randomBytes),
					retain: test.retain(&returned),
				},
			)
			if workspace != nil {
				t.Fatalf("contradictory retention result returned workspace %#v", workspace)
			}
			report := refusalReport(t, err)
			if report.Recovery == nil || report.Recovery.TaskRoot != nil || report.Recovery.TaskPath != taskPath {
				t.Fatalf("Recovery = %#v", report.Recovery)
			}
			if _, err := os.Stat(taskPath); err != nil {
				t.Fatalf("allocated task was lost: %v", err)
			}
			if returned != nil {
				if _, err := returned.descriptor.Stat(); err == nil {
					t.Fatal("unobserved returned task-root descriptor was not transactionally closed")
				}
			}
		})
	}
}

func TestWorkspaceInitializationFailureUsesWholeLedgerBarrier(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("workspace authority is supported only on darwin/arm64")
	}
	tests := []struct {
		name         string
		randomByte   byte
		dependencies func(string, []byte) workspaceAllocationDependencies
		wantRemoved  bool
		wantNames    []string
	}{
		{
			name:       "fully observed prefix is transactionally removed",
			randomByte: 0x41,
			dependencies: func(_ string, randomBytes []byte) workspaceAllocationDependencies {
				return workspaceAllocationDependencies{
					random: bytes.NewReader(randomBytes),
					retain: retainCreatedWorkspaceDirectory,
					afterObserve: func(path string, _ entryKind) error {
						if path == workspaceTemporary {
							return fail(CauseUnstable, "injected observed-prefix failure")
						}
						return nil
					},
				}
			},
			wantRemoved: true,
		},
		{
			name:       "unobserved replacement preserves every prior row",
			randomByte: 0x42,
			dependencies: func(taskPath string, randomBytes []byte) workspaceAllocationDependencies {
				return workspaceAllocationDependencies{
					random: bytes.NewReader(randomBytes),
					retain: retainCreatedWorkspaceDirectory,
					afterCreate: func(path string, _ entryKind) error {
						if path != workspaceGoPath {
							return nil
						}
						original := filepath.Join(taskPath, path)
						parked := original + "-original"
						if err := os.Rename(original, parked); err != nil {
							return err
						}
						if err := os.Mkdir(original, 0o700); err != nil {
							return err
						}
						return fail(CauseUnstable, "injected unobserved replacement")
					},
				}
			},
			wantNames: []string{workspaceTemporary, workspaceGoTemporary, workspaceGoPath, workspaceGoPath + "-original"},
		},
		{
			name:       "unexpected row after observation preserves complete prefix",
			randomByte: 0x43,
			dependencies: func(taskPath string, randomBytes []byte) workspaceAllocationDependencies {
				return workspaceAllocationDependencies{
					random: bytes.NewReader(randomBytes),
					retain: retainCreatedWorkspaceDirectory,
					afterObserve: func(path string, _ entryKind) error {
						if path != workspaceGoTemporary {
							return nil
						}
						if err := os.WriteFile(filepath.Join(taskPath, "unexpected"), nil, 0o600); err != nil {
							return err
						}
						return fail(CauseUnstable, "injected unexpected ledger row")
					},
				}
			},
			wantNames: []string{workspaceTemporary, workspaceGoTemporary, "unexpected"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base, stateRoot, git, gitDescriptor := newWorkspaceAllocationFixture(t)
			_ = gitDescriptor
			randomBytes := bytes.Repeat([]byte{test.randomByte}, taskRootRandomBytes)
			taskPath := filepath.Join(base, "state", taskRootPrefix+fmt.Sprintf("%x", randomBytes))
			workspace, err := allocateWorkspaceWith(
				context.Background(),
				stateRoot,
				git,
				test.dependencies(taskPath, randomBytes),
			)
			if workspace != nil {
				t.Fatalf("initialization failure returned workspace %#v", workspace)
			}
			report := refusalReport(t, err)
			if report.Primary == nil || report.Primary.Phase != PhaseWorkspace ||
				report.Primary.Operation != OperationOpen {
				t.Fatalf("Primary = %#v", report.Primary)
			}
			if test.wantRemoved {
				if report.Recovery != nil || report.Cleanup != nil {
					t.Fatalf("successful transactional rollback report = %#v", report)
				}
				if _, statErr := os.Lstat(taskPath); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("fully observed prefix remains: %v", statErr)
				}
				return
			}
			if report.Recovery == nil || report.Recovery.TaskRoot == nil || report.Cleanup == nil {
				t.Fatalf("preserved initialization failure report = %#v", report)
			}
			for _, name := range test.wantNames {
				if _, statErr := os.Lstat(filepath.Join(taskPath, name)); statErr != nil {
					t.Fatalf("ledger barrier deleted %q before preservation decision: %v", name, statErr)
				}
			}
		})
	}
}

func newWorkspaceAllocationFixture(
	t *testing.T,
) (string, *retainedDirectory, workspaceGitAuthority, *os.File) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve workspace fixture root: %v", err)
	}
	statePath := filepath.Join(base, "state")
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatalf("make workspace StateRoot: %v", err)
	}
	if err := os.Chmod(statePath, 0o700); err != nil {
		t.Fatalf("chmod workspace StateRoot: %v", err)
	}
	stateRoot, err := admitStateRoot(statePath)
	if err != nil {
		t.Fatalf("admit workspace StateRoot: %v", err)
	}
	git, gitDescriptor := newLiveGitAuthority(t, base)
	t.Cleanup(func() {
		_ = stateRoot.close()
		_ = gitDescriptor.Close()
	})
	return base, stateRoot, git, gitDescriptor
}

func newLiveWorkspace(t *testing.T) (*taskWorkspace, *retainedDirectory, *os.File) {
	t.Helper()
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("workspace authority is supported only on darwin/arm64")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve fixture root: %v", err)
	}
	statePath := filepath.Join(base, "state")
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatalf("make StateRoot: %v", err)
	}
	if err := os.Chmod(statePath, 0o700); err != nil {
		t.Fatalf("chmod StateRoot: %v", err)
	}
	stateRoot, err := admitStateRoot(statePath)
	if err != nil {
		t.Fatalf("admit StateRoot: %v", err)
	}
	git, gitDescriptor := newLiveGitAuthority(t, base)
	workspace, err := allocateWorkspace(context.Background(), stateRoot, git)
	if err != nil {
		_ = gitDescriptor.Close()
		_ = stateRoot.close()
		t.Fatalf("allocate workspace: %v", err)
	}
	t.Cleanup(func() {
		if workspace != nil && !workspace.isClosed() {
			_ = workspace.cleanup(context.Background(), nil)
		}
		_ = gitDescriptor.Close()
	})
	return workspace, stateRoot, gitDescriptor
}

func newSealedSpoolFile(
	t *testing.T,
	workspace *taskWorkspace,
	contents []byte,
) (*spoolGeneration, *spoolFile, *sealedSpoolReader, []byte) {
	t.Helper()
	generation, err := workspace.spool.beginGeneration(context.Background())
	if err != nil {
		t.Fatalf("begin object-spool generation: %v", err)
	}
	file, err := generation.create(context.Background())
	if err != nil {
		t.Fatalf("create object-spool file: %v", err)
	}
	original := bytes.Clone(contents)
	if written, err := file.write(context.Background(), original); err != nil || written != len(original) {
		t.Fatalf("write object-spool file: written=%d err=%v", written, err)
	}
	reader, err := file.seal(context.Background())
	if err != nil {
		t.Fatalf("seal object-spool file: %v", err)
	}
	return generation, file, reader, original
}

func overwriteFileAt(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := overwriteFileAtError(path, contents); err != nil {
		t.Fatalf("overwrite retained file: %v", err)
	}
}

func overwriteFileAtError(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	written, writeErr := file.WriteAt(contents, 0)
	if writeErr == nil && written != len(contents) {
		writeErr = io.ErrShortWrite
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func newLiveGitAuthority(t *testing.T, directory string) (workspaceGitAuthority, *os.File) {
	t.Helper()
	path := filepath.Join(directory, "retained-git")
	if err := os.WriteFile(path, []byte("synthetic retained Git\n"), 0o700); err != nil {
		t.Fatalf("write retained Git: %v", err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatalf("chmod retained Git: %v", err)
	}
	descriptor, err := openAbsoluteNoFollow(path, entryRegular)
	if err != nil {
		t.Fatalf("open retained Git: %v", err)
	}
	authority, err := newWorkspaceGitAuthority(context.Background(), path, descriptor)
	if err != nil {
		_ = descriptor.Close()
		t.Fatalf("bind retained Git: %v", err)
	}
	return authority, descriptor
}

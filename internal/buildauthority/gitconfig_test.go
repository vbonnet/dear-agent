package buildauthority

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseSourceConfigOptInReal(t *testing.T) {
	repository := os.Getenv("BUILD_AUTHORITY_REAL_SOURCE_REPOSITORY")
	gitExecutable := os.Getenv("BUILD_AUTHORITY_REAL_GIT_EXECUTABLE")
	if repository == "" || gitExecutable == "" {
		t.Skip("set BUILD_AUTHORITY_REAL_SOURCE_REPOSITORY and BUILD_AUTHORITY_REAL_GIT_EXECUTABLE")
	}
	content, err := os.ReadFile(filepath.Join(repository, ".git", "config"))
	if err != nil {
		t.Fatalf("read real source config: %v", err)
	}
	claim, err := parseSourceConfig(content)
	if err != nil {
		t.Fatalf("parse real source config: %v", err)
	}
	if claim.objectFormat != objectFormatSHA1 && claim.objectFormat != objectFormatSHA256 {
		t.Fatalf("real source config returned unknown object format %d", claim.objectFormat)
	}

	prefix := []string{"--no-pager", "-C", repository}
	for _, row := range sourceGitCommandOverrideRows() {
		prefix = append(prefix, "-c", row.key+"="+row.value)
	}
	closedEnvironment := []string{
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME=/nonexistent-buildauthority-home",
		"XDG_CONFIG_HOME=/nonexistent-buildauthority-xdg",
		"LANG=C",
		"LC_ALL=C",
	}
	run := func(arguments ...string) []byte {
		t.Helper()
		command := exec.Command(gitExecutable, append(append([]string(nil), prefix...), arguments...)...)
		command.Dir = repository
		command.Env = closedEnvironment
		stdout, runErr := command.Output()
		if runErr != nil {
			t.Fatalf("run real source config transcript: %v", runErr)
		}
		return stdout
	}
	local := run("config", "--null", "--show-origin", "--show-scope", "--local", "--list")
	if want := claim.localTranscript(); !bytes.Equal(local, want) {
		t.Fatalf("real Git local transcript differs from direct parse\n got: %q\nwant: %q", local, want)
	}
	active := run("config", "--null", "--show-origin", "--show-scope", "--list")
	if want := claim.activeTranscript(); !bytes.Equal(active, want) {
		t.Fatalf("real Git active transcript differs from direct parse\n got: %q\nwant: %q", active, want)
	}
	packedPath := filepath.Join(repository, ".git", "packed-refs")
	packed, packedErr := os.ReadFile(packedPath)
	if packedErr == nil {
		if _, parseErr := parsePackedRefs(packed, claim.objectFormat); parseErr != nil {
			t.Fatalf("parse real packed-refs: %v", parseErr)
		}
	} else if !os.IsNotExist(packedErr) {
		t.Fatalf("read real packed-refs: %v", packedErr)
	}
}

func TestParseSourceConfigSHA1AndExactTranscripts(t *testing.T) {
	t.Parallel()

	content := []byte(`# source metadata is inert
[CoRe]
	RepositoryFormatVersion = 0
	BARE = false
	fileMode = true
	hooksPath = "unused path"
[extensions]
	worktreeConfig = true
[user]
	name = "Example User"
	email = user@example.test
[beads]
	role = contributor
[remote "Origin"]
	url = "https://example.test/repository#fragment"
	fetch = +refs/heads/*:refs/remotes/Origin/*
[branch "main"]
	remote = Origin
	merge = refs/heads/main
	vscode-merge-base = Origin/main
`)
	claim, err := parseSourceConfig(content)
	if err != nil {
		t.Fatalf("parseSourceConfig() = %v", err)
	}
	if claim.objectFormat != objectFormatSHA1 {
		t.Fatalf("objectFormat = %v, want SHA-1", claim.objectFormat)
	}
	if !claim.worktreeConfigPresent {
		t.Fatal("worktreeConfig presence was not retained")
	}

	wantLocal := strings.Join([]string{
		"local\x00file:.git/config\x00core.repositoryformatversion\n0\x00",
		"local\x00file:.git/config\x00core.bare\nfalse\x00",
		"local\x00file:.git/config\x00core.filemode\ntrue\x00",
		"local\x00file:.git/config\x00core.hookspath\nunused path\x00",
		"local\x00file:.git/config\x00extensions.worktreeconfig\ntrue\x00",
		"local\x00file:.git/config\x00user.name\nExample User\x00",
		"local\x00file:.git/config\x00user.email\nuser@example.test\x00",
		"local\x00file:.git/config\x00beads.role\ncontributor\x00",
		"local\x00file:.git/config\x00remote.Origin.url\nhttps://example.test/repository#fragment\x00",
		"local\x00file:.git/config\x00remote.Origin.fetch\n+refs/heads/*:refs/remotes/Origin/*\x00",
		"local\x00file:.git/config\x00branch.main.remote\nOrigin\x00",
		"local\x00file:.git/config\x00branch.main.merge\nrefs/heads/main\x00",
		"local\x00file:.git/config\x00branch.main.vscode-merge-base\nOrigin/main\x00",
	}, "")
	if got := string(claim.localTranscript()); got != wantLocal {
		t.Fatalf("local transcript mismatch\n got: %q\nwant: %q", got, wantLocal)
	}

	wantActive := wantLocal
	for _, row := range sourceGitCommandOverrideRows() {
		wantActive += "command\x00command line:\x00" + row.key + "\n" + row.value + "\x00"
	}
	if got := string(claim.activeTranscript()); got != wantActive {
		t.Fatalf("active transcript mismatch\n got: %q\nwant: %q", got, wantActive)
	}
	first := claim.localTranscript()
	first[0] = 'X'
	if got := claim.localTranscript()[0]; got != 'l' {
		t.Fatalf("transcript storage aliased a caller result: first byte %q", got)
	}
}

func TestParseSourceConfigSHA256ClosedExtensionForm(t *testing.T) {
	t.Parallel()

	claim, err := parseSourceConfig([]byte(`[core]
	repositoryformatversion = 1
	bare = false
[extensions]
	objectFormat = sha256
	refStorage = files
`))
	if err != nil {
		t.Fatalf("parseSourceConfig() = %v", err)
	}
	if claim.objectFormat != objectFormatSHA256 {
		t.Fatalf("objectFormat = %v, want SHA-256", claim.objectFormat)
	}
	if got := claim.objectFormat.name(); got != "sha256" {
		t.Fatalf("format name = %q", got)
	}
	if got := claim.objectFormat.hexWidth(); got != 64 {
		t.Fatalf("format width = %d", got)
	}
}

func TestParseSourceConfigRejectsMalformedAndForbiddenForms(t *testing.T) {
	t.Parallel()

	minimal := "[core]\n\trepositoryformatversion = 0\n\tbare = false\n"
	tests := []struct {
		name    string
		content string
		cause   CauseCode
	}{
		{name: "assignment-before-section", content: "bare = false\n", cause: CauseMalformed},
		{name: "nul", content: minimal + "\x00", cause: CauseMalformed},
		{name: "carriage-return", content: strings.Replace(minimal, "\n", "\r\n", 1), cause: CauseMalformed},
		{name: "unterminated-section", content: minimal + "[user\n", cause: CauseMalformed},
		{name: "old-dot-subsection", content: minimal + "[remote.origin]\nurl = https://example.test\n", cause: CauseMalformed},
		{name: "bare-variable", content: "[core]\nrepositoryformatversion = 0\nbare\n", cause: CauseMalformed},
		{name: "unterminated-quote", content: minimal + "[user]\nname = \"unterminated\n", cause: CauseMalformed},
		{name: "control-escape", content: minimal + "[user]\nname = \"line\\nfeed\"\n", cause: CauseMalformed},
		{name: "invalid-escape", content: minimal + "[user]\nname = \"bad\\q\"\n", cause: CauseMalformed},
		{name: "duplicate-casefold", content: "[core]\nrepositoryformatversion = 0\nREPOSITORYFORMATVERSION = 0\nbare = false\n", cause: CauseMalformed},
		{name: "missing-required", content: "[core]\nbare = false\n", cause: CauseMalformed},
		{name: "noncanonical-boolean", content: "[core]\nrepositoryformatversion = 0\nbare = false\nfilemode = yes\n", cause: CauseMalformed},
		{name: "bare-repository", content: "[core]\nrepositoryformatversion = 0\nbare = true\n", cause: CauseUnsupported},
		{name: "sha1-object-format", content: minimal + "[extensions]\nobjectformat = sha256\n", cause: CauseUnsupported},
		{name: "sha1-ref-storage", content: minimal + "[extensions]\nrefstorage = files\n", cause: CauseUnsupported},
		{name: "sha256-missing-format", content: "[core]\nrepositoryformatversion = 1\nbare = false\n", cause: CauseUnsupported},
		{name: "unknown-key", content: minimal + "[core]\nunknown = value\n", cause: CauseUnsupported},
		{name: "include", content: minimal + "[include]\npath = /tmp/other\n", cause: CauseUnsupported},
		{name: "alias", content: minimal + "[alias]\nx = status\n", cause: CauseUnsupported},
		{name: "filter", content: minimal + "[filter \"x\"]\nclean = command\n", cause: CauseUnsupported},
		{name: "protocol", content: minimal + "[protocol]\nallow = always\n", cause: CauseUnsupported},
		{name: "credential-helper", content: minimal + "[credential]\nhelper = command\n", cause: CauseUnsupported},
		{name: "stored-replace-control", content: minimal + "[core]\nuseReplaceRefs = false\n", cause: CauseUnsupported},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseSourceConfig([]byte(test.content))
			requirePrivateCause(t, err, test.cause)
		})
	}
}

func TestParseSourceConfigEnforcesRemoteAndBranchClosure(t *testing.T) {
	t.Parallel()

	minimal := "[core]\nrepositoryformatversion = 0\nbare = false\n"
	tests := []struct {
		name    string
		content string
		cause   CauseCode
	}{
		{
			name:    "remote-missing-fetch",
			content: minimal + "[remote \"origin\"]\nurl = https://example.test/repo\n",
			cause:   CauseMalformed,
		},
		{
			name:    "remote-missing-url",
			content: minimal + "[remote \"origin\"]\nfetch = +refs/heads/*:refs/remotes/origin/*\n",
			cause:   CauseMalformed,
		},
		{
			name:    "wrong-fetch-name",
			content: minimal + "[remote \"origin\"]\nurl = https://example.test/repo\nfetch = +refs/heads/*:refs/remotes/upstream/*\n",
			cause:   CauseUnsupported,
		},
		{
			name:    "command-remote",
			content: minimal + "[remote \"origin\"]\nurl = ext::sh -c bad\nfetch = +refs/heads/*:refs/remotes/origin/*\n",
			cause:   CauseUnsupported,
		},
		{
			name:    "undeclared-branch-remote",
			content: minimal + "[branch \"main\"]\nremote = origin\nmerge = refs/heads/main\n",
			cause:   CauseMalformed,
		},
		{
			name:    "non-heads-merge",
			content: minimal + "[branch \"main\"]\nmerge = refs/tags/main\n",
			cause:   CauseMalformed,
		},
		{
			name:    "invalid-heads-ref",
			content: minimal + "[branch \"main\"]\nmerge = refs/heads/.hidden\n",
			cause:   CauseMalformed,
		},
		{
			name:    "forbidden-remote-option",
			content: minimal + "[remote \"origin\"]\nurl = https://example.test/repo\nfetch = +refs/heads/*:refs/remotes/origin/*\npromisor = false\n",
			cause:   CauseUnsupported,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseSourceConfig([]byte(test.content))
			requirePrivateCause(t, err, test.cause)
		})
	}
}

func TestParseSourceConfigEnforcesByteAndRecordLimits(t *testing.T) {
	t.Parallel()

	minimal := []byte("[core]\nrepositoryformatversion = 0\nbare = false\n")
	exactBytes := append([]byte(nil), minimal...)
	exactBytes = append(exactBytes, '#')
	exactBytes = append(exactBytes, bytes.Repeat([]byte{'x'}, maxSourceConfigBytes-len(exactBytes)-1)...)
	exactBytes = append(exactBytes, '\n')
	if len(exactBytes) != maxSourceConfigBytes {
		t.Fatalf("exact-byte fixture length = %d", len(exactBytes))
	}
	if _, err := parseSourceConfig(exactBytes); err != nil {
		t.Fatalf("parseSourceConfig(exact byte limit) = %v", err)
	}
	tooLarge := append(append([]byte(nil), exactBytes...), 'x')
	_, err := parseSourceConfig(tooLarge)
	requirePrivateCause(t, err, CauseLimit)

	var content strings.Builder
	content.Write(minimal)
	for index := range maxSourceConfigRecords - 2 {
		fmt.Fprintf(&content, "[branch \"b%d\"]\nmerge = refs/heads/b%d\n", index, index)
	}
	if content.Len() >= maxSourceConfigBytes {
		t.Fatalf("record fixture unexpectedly exceeds byte bound: %d", content.Len())
	}
	if _, err := parseSourceConfig([]byte(content.String())); err != nil {
		t.Fatalf("parseSourceConfig(exact record limit) = %v", err)
	}
	fmt.Fprintf(&content, "[branch \"over\"]\nmerge = refs/heads/over\n")
	_, err = parseSourceConfig([]byte(content.String()))
	requirePrivateCause(t, err, CauseLimit)
}

func TestParseRequestedRevision(t *testing.T) {
	t.Parallel()

	sha1 := strings.Repeat("a", 40)
	sha256 := strings.Repeat("b", 64)
	for _, test := range []struct {
		name     string
		revision string
		format   repositoryObjectFormat
		want     string
		cause    CauseCode
	}{
		{name: "sha1", revision: sha1, format: objectFormatSHA1, want: sha1},
		{name: "sha256", revision: sha256, format: objectFormatSHA256, want: sha256},
		{name: "uppercase", revision: strings.Repeat("A", 40), format: objectFormatSHA1, cause: CauseInvalidRequest},
		{name: "wrong-width", revision: sha1 + "0", format: objectFormatSHA1, cause: CauseInvalidRequest},
		{name: "non-hex", revision: strings.Repeat("g", 40), format: objectFormatSHA1, cause: CauseInvalidRequest},
		{name: "unknown-format", revision: sha1, format: objectFormatUnknown, cause: CauseInternalInvariant},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseRequestedRevision(test.revision, test.format)
			if test.cause != "" {
				requirePrivateCause(t, err, test.cause)
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("parseRequestedRevision() = %q, %v; want %q, nil", got, err, test.want)
			}
		})
	}
}

func TestRenderPrivateConfigExactSHAForms(t *testing.T) {
	t.Parallel()

	init := privateInitBooleans{
		fileMode:          true,
		symlinks:          false,
		ignoreCase:        true,
		precomposeUnicode: false,
	}
	sha1, err := renderPrivateConfig(objectFormatSHA1, "/state/task", init)
	if err != nil {
		t.Fatalf("renderPrivateConfig(SHA-1) = %v", err)
	}
	wantSHA1 := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = false
	symlinks = false
	ignorecase = true
	precomposeunicode = false
	hooksPath = "/state/task/git-hooks"
	attributesFile = "/state/task/git-attributes"
	excludesFile = "/state/task/git-excludes"
	autocrlf = false
	eol = lf
	fsmonitor = false
	commitGraph = false
	multiPackIndex = false
	useReplaceRefs = false
[checkout]
	workers = 1
[maintenance]
	auto = false
[gc]
	auto = 0
[pack]
	readReverseIndex = false
	useBitmaps = false
[fetch]
	writeCommitGraph = false
[protocol]
	allow = never
[commit]
	gpgSign = false
[tag]
	gpgSign = false
[log]
	showSignature = false
[merge]
	verifySignatures = false
[submodule]
	recurse = false
`
	if got := string(sha1); got != wantSHA1 {
		t.Fatalf("SHA-1 private config mismatch\n got:\n%s\nwant:\n%s", got, wantSHA1)
	}
	if bytes.Contains(sha1, []byte("[extensions]")) {
		t.Fatal("SHA-1 private config contains extensions section")
	}

	sha256, err := renderPrivateConfig(objectFormatSHA256, "/state/task", init)
	if err != nil {
		t.Fatalf("renderPrivateConfig(SHA-256) = %v", err)
	}
	wantInsertion := "\tuseReplaceRefs = false\n[extensions]\n\tobjectFormat = sha256\n[checkout]"
	if !bytes.Contains(sha256, []byte(wantInsertion)) {
		t.Fatalf("SHA-256 private config omitted exact extension placement:\n%s", sha256)
	}
	if bytes.Contains(sha256, []byte("refStorage")) || bytes.Contains(sha256, []byte("refstorage")) {
		t.Fatal("private config retained ref storage")
	}
	if err := validatePrivateConfig(sha256, objectFormatSHA256, "/state/task", init); err != nil {
		t.Fatalf("validatePrivateConfig(exact) = %v", err)
	}
	mutated := append([]byte(nil), sha256...)
	mutated = append(mutated, '\n')
	requirePrivateCause(t, validatePrivateConfig(mutated, objectFormatSHA256, "/state/task", init), CauseUnstable)
}

func TestPrivateConfigPathQuotingAndRefusal(t *testing.T) {
	t.Parallel()

	quoted, err := quoteGitConfigPath("/state/a\\b\"c\n\t\b")
	if err != nil {
		t.Fatalf("quoteGitConfigPath() = %v", err)
	}
	if want := `"/state/a\\b\"c\n\t\b"`; quoted != want {
		t.Fatalf("quoteGitConfigPath() = %q, want %q", quoted, want)
	}
	weirdTaskPath := "/state/a\\b\"c\n\t\b"
	weirdConfig, err := renderPrivateConfig(objectFormatSHA256, weirdTaskPath, privateInitBooleans{})
	if err != nil {
		t.Fatalf("renderPrivateConfig(escaped path) = %v", err)
	}
	if err := validatePrivateConfig(weirdConfig, objectFormatSHA256, weirdTaskPath, privateInitBooleans{}); err != nil {
		t.Fatalf("validatePrivateConfig(escaped path) = %v", err)
	}
	for _, invalid := range []string{"relative", "/", "/state/task/", "/state//task", "/state/../task", "/state/./task"} {
		_, err := renderPrivateConfig(objectFormatSHA1, invalid, privateInitBooleans{})
		requirePrivateCause(t, err, CauseInternalInvariant)
	}
	for _, invalid := range []string{"/state/\x00task", "/state/\x01task", "/state/\x7ftask"} {
		_, err := renderPrivateConfig(objectFormatSHA1, invalid, privateInitBooleans{})
		requirePrivateCause(t, err, CauseMalformed)
	}
}

func TestGitAdministrativeLockBasenameExactPredicate(t *testing.T) {
	t.Parallel()

	for _, name := range []string{".lock", "index.lock", "packed-refs.lock", "a.lock"} {
		if !isGitAdministrativeLockBasename(name) {
			t.Errorf("isGitAdministrativeLockBasename(%q) = false", name)
		}
	}
	for _, name := range []string{"", "lock", ".locked", "packed-refs.new", "lock.file", "a.lock.extra"} {
		if isGitAdministrativeLockBasename(name) {
			t.Errorf("isGitAdministrativeLockBasename(%q) = true", name)
		}
	}
}

func TestValidFullGitRefNameClosedGrammar(t *testing.T) {
	t.Parallel()

	valid := []string{"refs/heads/main", "refs/heads/feature/x", "refs/tags/v1.0.0"}
	for _, name := range valid {
		if !validFullGitRefName(name) {
			t.Errorf("validFullGitRefName(%q) = false", name)
		}
	}
	invalid := []string{
		"main", "refs/", "refs//main", "refs/./main", "refs/../main",
		"refs/.hidden/main", "refs/heads/main.", "refs/heads/main.lock",
		"refs/heads/a..b", "refs/heads/a b", "refs/heads/a~b",
		"refs/heads/a^b", "refs/heads/a:b", "refs/heads/a?b",
		"refs/heads/a*b", "refs/heads/a[b", `refs/heads/a\b`, "refs/heads/a@{b",
	}
	for _, name := range invalid {
		if validFullGitRefName(name) {
			t.Errorf("validFullGitRefName(%q) = true", name)
		}
	}
}

func TestParsePackedRefsClosedSHAForms(t *testing.T) {
	t.Parallel()

	one := strings.Repeat("1", 40)
	two := strings.Repeat("2", 40)
	peeled := strings.Repeat("a", 40)
	content := []byte("# pack-refs with: peeled fully-peeled sorted \n" +
		one + " refs/heads/main\n" +
		two + " refs/tags/v1\n" +
		"^" + peeled + "\n")
	claim, err := parsePackedRefs(content, objectFormatSHA1)
	if err != nil {
		t.Fatalf("parsePackedRefs() = %v", err)
	}
	wantTraits := []string{"peeled", "fully-peeled", "sorted"}
	if !reflect.DeepEqual(claim.traits, wantTraits) {
		t.Fatalf("traits = %#v, want %#v", claim.traits, wantTraits)
	}
	wantRecords := []packedRefRecord{
		{objectID: one, name: "refs/heads/main"},
		{objectID: two, name: "refs/tags/v1", peeledID: peeled},
	}
	if !reflect.DeepEqual(claim.records, wantRecords) {
		t.Fatalf("records = %#v, want %#v", claim.records, wantRecords)
	}

	empty, err := parsePackedRefs(nil, objectFormatSHA256)
	if err != nil || len(empty.traits) != 0 || len(empty.records) != 0 {
		t.Fatalf("empty packed-refs = %#v, %v", empty, err)
	}
	sha256 := strings.Repeat("b", 64) + " refs/heads/main\n"
	claim, err = parsePackedRefs([]byte(sha256), objectFormatSHA256)
	if err != nil || len(claim.records) != 1 || claim.records[0].objectID != strings.Repeat("b", 64) {
		t.Fatalf("SHA-256 packed-refs = %#v, %v", claim, err)
	}
}

func TestParsePackedRefsRejectsMalformedAndReplacementState(t *testing.T) {
	t.Parallel()

	one := strings.Repeat("1", 40)
	two := strings.Repeat("2", 40)
	upper := strings.Repeat("A", 40)
	tests := []struct {
		name    string
		content string
		cause   CauseCode
	}{
		{name: "not-terminated", content: one + " refs/heads/main", cause: CauseMalformed},
		{name: "blank-row", content: "\n", cause: CauseMalformed},
		{name: "cr", content: one + " refs/heads/main\r\n", cause: CauseMalformed},
		{name: "nul", content: one + " refs/heads/ma\x00in\n", cause: CauseMalformed},
		{name: "unknown-header", content: "# unknown\n", cause: CauseMalformed},
		{name: "nonleading-header", content: one + " refs/heads/main\n# pack-refs with: sorted \n", cause: CauseMalformed},
		{name: "header-no-trailing-space", content: "# pack-refs with: sorted\n", cause: CauseMalformed},
		{name: "header-unknown-trait", content: "# pack-refs with: unknown \n", cause: CauseMalformed},
		{name: "header-duplicate-trait", content: "# pack-refs with: peeled peeled \n", cause: CauseMalformed},
		{name: "header-out-of-order", content: "# pack-refs with: sorted peeled \n", cause: CauseMalformed},
		{name: "uppercase-id", content: upper + " refs/heads/main\n", cause: CauseMalformed},
		{name: "short-id", content: one[:39] + " refs/heads/main\n", cause: CauseMalformed},
		{name: "extra-space", content: one + "  refs/heads/main\n", cause: CauseMalformed},
		{name: "invalid-ref", content: one + " refs/heads/.hidden\n", cause: CauseMalformed},
		{name: "replacement-root", content: one + " refs/replace\n", cause: CauseUnsupported},
		{name: "replacement-child", content: one + " refs/replace/target\n", cause: CauseUnsupported},
		{name: "detached-peeled", content: "^" + one + "\n", cause: CauseMalformed},
		{name: "duplicate-peeled", content: one + " refs/tags/v1\n^" + one + "\n^" + two + "\n", cause: CauseMalformed},
		{name: "duplicate-name", content: one + " refs/heads/main\n" + two + " refs/heads/main\n", cause: CauseMalformed},
		{name: "descending-name", content: one + " refs/tags/v1\n" + two + " refs/heads/main\n", cause: CauseMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := parsePackedRefs([]byte(test.content), objectFormatSHA1)
			requirePrivateCause(t, err, test.cause)
		})
	}
}

func TestParsePackedRefsEnforcesExactInjectedLimits(t *testing.T) {
	t.Parallel()

	one := strings.Repeat("1", 40) + " refs/heads/a\n"
	two := strings.Repeat("2", 40) + " refs/heads/b\n"
	content := []byte(one + two)
	claim, err := parsePackedRefsWithLimits(content, objectFormatSHA1, packedRefsLimits{
		bytes: len(content),
		rows:  2,
	})
	if err != nil || len(claim.records) != 2 {
		t.Fatalf("exact packed-refs limits = %#v, %v", claim, err)
	}
	_, err = parsePackedRefsWithLimits(content, objectFormatSHA1, packedRefsLimits{
		bytes: len(content) - 1,
		rows:  2,
	})
	requirePrivateCause(t, err, CauseLimit)
	_, err = parsePackedRefsWithLimits(content, objectFormatSHA1, packedRefsLimits{
		bytes: len(content),
		rows:  1,
	})
	requirePrivateCause(t, err, CauseLimit)
	_, err = parsePackedRefsWithLimits(content, objectFormatUnknown, packedRefsLimits{
		bytes: len(content),
		rows:  2,
	})
	requirePrivateCause(t, err, CauseInternalInvariant)
	_, err = parsePackedRefsWithLimits(content, objectFormatSHA1, packedRefsLimits{bytes: -1, rows: 2})
	requirePrivateCause(t, err, CauseInternalInvariant)
}

func TestSourceGitOverrideOrderIsClosed(t *testing.T) {
	t.Parallel()

	want := [...]configTranscriptRow{
		{key: "core.hookspath", value: "/dev/null"},
		{key: "protocol.allow", value: "never"},
		{key: "core.usereplacerefs", value: "false"},
		{key: "core.commitgraph", value: "false"},
		{key: "core.multipackindex", value: "false"},
		{key: "core.fsmonitor", value: "false"},
		{key: "pack.readreverseindex", value: "false"},
		{key: "pack.usebitmaps", value: "false"},
		{key: "maintenance.auto", value: "false"},
		{key: "fetch.writecommitgraph", value: "false"},
		{key: "gc.auto", value: "0"},
	}
	if got := sourceGitCommandOverrideRows(); !reflect.DeepEqual(got, want) {
		t.Fatalf("sourceGitCommandOverrideRows() = %#v, want %#v", got, want)
	}
	first := sourceGitCommandOverrideRows()
	first[0] = configTranscriptRow{key: "mutated", value: "mutated"}
	if got := sourceGitCommandOverrideRows()[0]; got != want[0] {
		t.Fatalf("source override policy aliases mutable storage: %#v", got)
	}
}

func requirePrivateCause(t *testing.T, err error, want CauseCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want cause %q", want)
	}
	got := privateCauses(err, CauseInternalInvariant)
	if !reflect.DeepEqual(got, []CauseCode{want}) {
		t.Fatalf("private causes = %#v, want [%q] (error %v)", got, want, err)
	}
}

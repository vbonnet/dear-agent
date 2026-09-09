package buildauthority

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	directEnvironmentRowCount  = 39
	goOwnedEnvironmentRowCount = 50
	goGitEnvironmentRowCount   = 51
	goToolEnvironmentRowCount  = 50
	goBuildEnvironmentRowCount = 52
	goLinkEnvironmentRowCount  = 51
)

// environmentAuthorityInputs contains only retained, nominal authorities. It
// deliberately has no path field: paths used in an environment are derived
// from the retained capabilities after their relationships have been checked.
type environmentAuthorityInputs struct {
	physicalRoot  *physicalRootAuthority
	nullDevice    *retainedNullDevice
	goExecutable  *goAuthority
	compiler      *compilerAuthority
	gitExecutable *gitAuthority
	goroot        *treeCapture
	gomodcache    *treeCapture
}

type environmentAuthorityPaths struct {
	goroot     string
	gomodcache string
}

type directEnvironmentRows [directEnvironmentRowCount]string
type goOwnedEnvironmentRows [goOwnedEnvironmentRowCount]string
type goGitEnvironmentRows [goGitEnvironmentRowCount]string
type goToolEnvironmentRows [goToolEnvironmentRowCount]string
type goBuildEnvironmentRows [goBuildEnvironmentRowCount]string
type goLinkEnvironmentRows [goLinkEnvironmentRowCount]string

type preallocationEnvironmentSeal uint8
type taskPrivateEnvironmentSeal uint8
type goOwnedEnvironmentSeal uint8
type goGitEnvironmentSeal uint8
type goToolEnvironmentSeal uint8
type goCompileEnvironmentSeal uint8
type goAssemblerEnvironmentSeal uint8
type goLinkEnvironmentSeal uint8

const (
	validPreallocationEnvironment preallocationEnvironmentSeal = 1
	validTaskPrivateEnvironment   taskPrivateEnvironmentSeal   = 1
	validGoOwnedEnvironment       goOwnedEnvironmentSeal       = 1
	validGoGitEnvironment         goGitEnvironmentSeal         = 1
	validGoToolEnvironment        goToolEnvironmentSeal        = 1
	validGoCompileEnvironment     goCompileEnvironmentSeal     = 1
	validGoAssemblerEnvironment   goAssemblerEnvironmentSeal   = 1
	validGoLinkEnvironment        goLinkEnvironmentSeal        = 1
)

// preallocationEnvironment and taskPrivateEnvironment retain the typed
// witnesses from which their exact rows were derived. They store no separate
// caller-supplied GOROOT, GOMODCACHE, or task path.
type preallocationEnvironment struct {
	authorities environmentAuthorityInputs
	rows        directEnvironmentRows
	seal        preallocationEnvironmentSeal
}

type taskPrivateEnvironment struct {
	authorities environmentAuthorityInputs
	workspace   *taskWorkspace
	rows        directEnvironmentRows
	seal        taskPrivateEnvironmentSeal
}

func newPreallocationEnvironment(
	authorities environmentAuthorityInputs,
) (preallocationEnvironment, error) {
	paths, err := validateEnvironmentAuthorityInputs(authorities)
	if err != nil {
		return preallocationEnvironment{}, err
	}
	rows, err := renderPreallocationEnvironment(paths)
	if err != nil {
		return preallocationEnvironment{}, err
	}
	return preallocationEnvironment{
		authorities: authorities,
		rows:        rows,
		seal:        validPreallocationEnvironment,
	}, nil
}

func newTaskPrivateEnvironment(
	workspace *taskWorkspace,
	authorities environmentAuthorityInputs,
) (taskPrivateEnvironment, error) {
	paths, err := validateEnvironmentAuthorityInputs(authorities)
	if err != nil {
		return taskPrivateEnvironment{}, err
	}
	workspacePaths, err := environmentWorkspacePaths(workspace, authorities.gitExecutable)
	if err != nil {
		return taskPrivateEnvironment{}, err
	}
	rows, err := renderTaskPrivateEnvironment(paths, workspacePaths)
	if err != nil {
		return taskPrivateEnvironment{}, err
	}
	return taskPrivateEnvironment{
		authorities: authorities,
		workspace:   workspace,
		rows:        rows,
		seal:        validTaskPrivateEnvironment,
	}, nil
}

func validateEnvironmentAuthorityInputs(
	authorities environmentAuthorityInputs,
) (environmentAuthorityPaths, error) {
	if err := validateEnvironmentRootAuthorityShapes(authorities.physicalRoot, authorities.nullDevice); err != nil {
		return environmentAuthorityPaths{}, err
	}
	paths, err := validateEnvironmentTreeAuthorities(authorities.goroot, authorities.gomodcache)
	if err != nil {
		return environmentAuthorityPaths{}, err
	}
	if err := validateEnvironmentExecutableAuthorities(authorities); err != nil {
		return environmentAuthorityPaths{}, err
	}
	return paths, nil
}

// Environment sealing binds already admitted root and null capabilities. Live
// descriptor revalidation remains a named command-boundary obligation.
func validateEnvironmentRootAuthorityShapes(
	physicalRoot *physicalRootAuthority,
	nullDevice *retainedNullDevice,
) error {
	if _, err := validatePhysicalRootAuthorityShape(physicalRoot); err != nil {
		return err
	}
	if nullDevice == nil || nullDevice.leaf == nil || nullDevice.leaf.descriptor == nil ||
		nullDevice.leaf.path != nullDevicePath || len(nullDevice.leaf.pathClaims) == 0 {
		return fail(CauseInternalInvariant, "missing null-device environment authority")
	}
	return nil
}

func validateEnvironmentTreeAuthorities(
	gorootCapture *treeCapture,
	gomodcacheCapture *treeCapture,
) (environmentAuthorityPaths, error) {
	goroot, err := environmentTreePath(gorootCapture, gorootPolicy(), "GOROOT")
	if err != nil {
		return environmentAuthorityPaths{}, err
	}
	gomodcache, err := environmentTreePath(gomodcacheCapture, gomodcachePolicy(), "GOMODCACHE")
	if err != nil {
		return environmentAuthorityPaths{}, err
	}
	if goroot == gomodcache {
		return environmentAuthorityPaths{}, fail(CauseIdentity, "environment authority roots overlap")
	}
	return environmentAuthorityPaths{goroot: goroot, gomodcache: gomodcache}, nil
}

func validateEnvironmentExecutableAuthorities(authorities environmentAuthorityInputs) error {
	if err := validateEnvironmentGoAuthority(authorities.goExecutable, authorities.goroot); err != nil {
		return err
	}
	if err := validateEnvironmentCompilerAuthority(authorities.compiler, authorities.goroot); err != nil {
		return err
	}
	if err := validateEnvironmentGitAuthority(authorities.gitExecutable); err != nil {
		return err
	}
	if authorities.goExecutable.retained == authorities.compiler.retained ||
		authorities.goExecutable.retained == authorities.gitExecutable.retained ||
		authorities.compiler.retained == authorities.gitExecutable.retained {
		return fail(CauseIdentity, "nominal executable authorities alias")
	}
	return nil
}

func environmentTreePath(capture *treeCapture, policy treePolicy, name string) (string, error) {
	if capture == nil || capture.root == nil || capture.root.root == nil ||
		capture.root.descriptor == nil || capture.policy != policy || capture.digest == (Digest{}) {
		return "", fail(CauseInternalInvariant, "missing retained "+name+" environment authority")
	}
	if !validEnvironmentAuthorityPath(capture.root.path) {
		return "", fail(CauseInternalInvariant, "invalid retained "+name+" path")
	}
	return capture.root.path, nil
}

func validateEnvironmentGoAuthority(authority *goAuthority, goroot *treeCapture) error {
	if authority == nil || authority.seal != validGoAuthority || authority.retained == nil {
		return fail(CauseInternalInvariant, "missing nominal Go environment authority")
	}
	if authority.retained.digest != goExecutableDigest() {
		return fail(CauseUnsupported, "Go environment authority digest refused")
	}
	if err := validateRetainedExecutableShape(authority.retained); err != nil {
		return err
	}
	binding, err := gorootExecutableClaimFromCapture(goroot, goGOROOTRelativePath)
	if err != nil {
		return err
	}
	return validateExecutableAgainstGOROOTBinding(authority.retained, binding)
}

func validateEnvironmentCompilerAuthority(authority *compilerAuthority, goroot *treeCapture) error {
	if authority == nil || authority.seal != validCompilerAuthority || authority.retained == nil {
		return fail(CauseInternalInvariant, "missing nominal compiler environment authority")
	}
	if err := validateRetainedExecutableShape(authority.retained); err != nil {
		return err
	}
	binding, err := gorootExecutableClaimFromCapture(goroot, compilerGOROOTRelativePath)
	if err != nil {
		return err
	}
	return validateExecutableAgainstGOROOTBinding(authority.retained, binding)
}

func validateEnvironmentGitAuthority(authority *gitAuthority) error {
	if authority == nil || authority.seal != validGitAuthority || authority.retained == nil {
		return fail(CauseInternalInvariant, "missing nominal Git environment authority")
	}
	if err := validateRetainedExecutableShape(authority.retained); err != nil {
		return err
	}
	if filepath.Base(authority.retained.leaf.path) != gitExecutableBase ||
		authority.retained.digest != gitExecutableDigest() {
		return fail(CauseUnsupported, "Git environment authority identity refused")
	}
	return nil
}

func environmentWorkspacePaths(
	workspace *taskWorkspace,
	gitAuthority *gitAuthority,
) (workspacePaths, error) {
	if workspace == nil {
		return workspacePaths{}, fail(CauseInternalInvariant, "missing private workspace environment authority")
	}
	workspace.lifecycle.RLock()
	defer workspace.lifecycle.RUnlock()
	if err := validateEnvironmentWorkspaceRoots(workspace); err != nil {
		return workspacePaths{}, err
	}
	if err := validateEnvironmentWorkspaceSpool(workspace.spool, workspace.paths); err != nil {
		return workspacePaths{}, err
	}
	if err := validateEnvironmentWorkspaceGit(workspace.git, gitAuthority); err != nil {
		return workspacePaths{}, err
	}
	return workspace.paths, nil
}

func validateEnvironmentWorkspaceRoots(workspace *taskWorkspace) error {
	if workspace.closed || !completeEnvironmentDirectory(workspace.stateRoot) ||
		!completeEnvironmentDirectory(workspace.taskRoot) {
		return fail(CauseInternalInvariant, "incomplete private workspace environment authority")
	}
	if !validEnvironmentAuthorityPath(workspace.paths.taskRoot) ||
		workspace.paths.taskRoot != workspace.taskRoot.path ||
		filepath.Dir(workspace.paths.taskRoot) != workspace.stateRoot.path ||
		filepath.Base(workspace.paths.taskRoot) != workspace.taskName ||
		!validEnvironmentTaskName(workspace.taskName) {
		return fail(CauseInternalInvariant, "invalid private workspace environment paths")
	}
	return nil
}

func completeEnvironmentDirectory(directory *retainedDirectory) bool {
	return directory != nil && directory.root != nil && directory.descriptor != nil &&
		validEnvironmentAuthorityPath(directory.path)
}

func validEnvironmentTaskName(name string) bool {
	if !strings.HasPrefix(name, taskRootPrefix) || len(name) != len(taskRootPrefix)+taskRootRandomBytes*2 {
		return false
	}
	for _, character := range name[len(taskRootPrefix):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validateEnvironmentWorkspaceSpool(spool *objectSpool, paths workspacePaths) error {
	if spool == nil {
		return fail(CauseInternalInvariant, "missing private workspace object spool")
	}
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if spool.closed || !completeEnvironmentDirectory(spool.root) ||
		spool.root.path != paths.child(workspaceObjectSpool) {
		return fail(CauseInternalInvariant, "invalid private workspace object spool")
	}
	return nil
}

func validateEnvironmentWorkspaceGit(workspaceGit workspaceGitAuthority, authority *gitAuthority) error {
	if authority == nil || authority.retained == nil || authority.retained.leaf == nil {
		return fail(CauseInternalInvariant, "missing nominal Git workspace authority")
	}
	leaf := authority.retained.leaf
	if workspaceGit.path != leaf.path || workspaceGit.descriptor != leaf.descriptor ||
		workspaceGit.snapshot != leaf.snapshot || workspaceGit.mount != leaf.mount ||
		workspaceGit.aclDigest != leaf.aclDigest {
		return fail(CauseIdentity, "private workspace Git authority mismatch")
	}
	return nil
}

func validEnvironmentAuthorityPath(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path &&
		!strings.ContainsRune(path, '\x00') && utf8.ValidString(path)
}

type environmentAssignment struct {
	key   string
	value string
}

func renderPreallocationEnvironment(paths environmentAuthorityPaths) (directEnvironmentRows, error) {
	entries := commonEnvironmentAssignments(paths)
	entries = append(entries,
		environmentAssignment{key: "GIT_CONFIG_GLOBAL", value: nullDevicePath},
		environmentAssignment{key: "GIT_EXEC_PATH", value: nullDevicePath},
		environmentAssignment{key: "GIT_TEMPLATE_DIR", value: nullDevicePath},
		environmentAssignment{key: "GOCACHE", value: "off"},
		environmentAssignment{key: "GOPATH", value: nullDevicePath},
		environmentAssignment{key: "GOTMPDIR", value: nullDevicePath},
		environmentAssignment{key: "PATH", value: nullDevicePath},
		environmentAssignment{key: "TEMP", value: nullDevicePath},
		environmentAssignment{key: "TMP", value: nullDevicePath},
		environmentAssignment{key: "TMPDIR", value: nullDevicePath},
	)
	return sealDirectEnvironmentRows(entries)
}

func renderTaskPrivateEnvironment(
	paths environmentAuthorityPaths,
	workspace workspacePaths,
) (directEnvironmentRows, error) {
	if !validEnvironmentAuthorityPath(workspace.taskRoot) {
		return directEnvironmentRows{}, fail(CauseInternalInvariant, "invalid private workspace path")
	}
	entries := commonEnvironmentAssignments(paths)
	entries = append(entries,
		environmentAssignment{key: "GIT_CONFIG_GLOBAL", value: workspace.child(workspaceGitGlobal)},
		environmentAssignment{key: "GIT_EXEC_PATH", value: workspace.child(workspaceGitExec)},
		environmentAssignment{key: "GIT_TEMPLATE_DIR", value: workspace.child(workspaceGitTemplate)},
		environmentAssignment{key: "GOCACHE", value: workspace.child(workspaceGoCache)},
		environmentAssignment{key: "GOPATH", value: workspace.child(workspaceGoPath)},
		environmentAssignment{key: "GOTMPDIR", value: workspace.child(workspaceGoTemporary)},
		environmentAssignment{key: "PATH", value: workspace.child(workspaceGitPath)},
		environmentAssignment{key: "TEMP", value: workspace.child(workspaceTemporary)},
		environmentAssignment{key: "TMP", value: workspace.child(workspaceTemporary)},
		environmentAssignment{key: "TMPDIR", value: workspace.child(workspaceTemporary)},
	)
	return sealDirectEnvironmentRows(entries)
}

func commonEnvironmentAssignments(paths environmentAuthorityPaths) []environmentAssignment {
	return []environmentAssignment{
		{key: "CGO_ENABLED", value: "0"},
		{key: "GIT_ATTR_NOSYSTEM", value: "1"},
		{key: "GIT_CONFIG_NOSYSTEM", value: "1"},
		{key: "GIT_NO_LAZY_FETCH", value: "1"},
		{key: "GIT_NO_REPLACE_OBJECTS", value: "1"},
		{key: "GIT_OPTIONAL_LOCKS", value: "0"},
		{key: "GIT_PROTOCOL_FROM_USER", value: "0"},
		{key: "GIT_TERMINAL_PROMPT", value: "0"},
		{key: "GO111MODULE", value: "on"},
		{key: "GOARCH", value: "arm64"},
		{key: "GOARM64", value: "v8.0"},
		{key: "GOENV", value: "off"},
		{key: "GOEXPERIMENT", value: "none"},
		{key: "GO_EXTLINK_ENABLED", value: "0"},
		{key: "GOFIPS140", value: "off"},
		{key: "GOFLAGS", value: "-mod=readonly"},
		{key: "GOMODCACHE", value: paths.gomodcache},
		{key: "GOOS", value: "darwin"},
		{key: "GOPROXY", value: "off"},
		{key: "GOROOT", value: paths.goroot},
		{key: "GOSUMDB", value: "off"},
		{key: "GOTOOLCHAIN", value: "local"},
		{key: "GOVCS", value: "*:off"},
		{key: "GOWORK", value: "off"},
		{key: "HOME", value: ""},
		{key: "LANG", value: "C"},
		{key: "LC_ALL", value: "C"},
		{key: "TZ", value: "UTC"},
		{key: "XDG_CONFIG_HOME", value: ""},
	}
}

func sealDirectEnvironmentRows(entries []environmentAssignment) (directEnvironmentRows, error) {
	rows, err := renderEnvironmentAssignments(entries, directEnvironmentRowCount)
	if err != nil {
		return directEnvironmentRows{}, err
	}
	var sealed directEnvironmentRows
	copy(sealed[:], rows)
	return sealed, nil
}

func renderEnvironmentAssignments(
	entries []environmentAssignment,
	expected int,
) ([]string, error) {
	if len(entries) != expected {
		return nil, fail(CauseInternalInvariant, "environment row count mismatch")
	}
	rows := make([]string, 0, len(entries))
	keys := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !validEnvironmentKey(entry.key) || strings.ContainsRune(entry.value, '\x00') ||
			!utf8.ValidString(entry.value) {
			return nil, fail(CauseInternalInvariant, "invalid environment assignment")
		}
		if _, duplicate := keys[entry.key]; duplicate {
			return nil, fail(CauseInternalInvariant, "duplicate environment assignment")
		}
		keys[entry.key] = struct{}{}
		rows = append(rows, entry.key+"="+entry.value)
	}
	sort.Strings(rows)
	return rows, nil
}

func validEnvironmentKey(key string) bool {
	if key == "" || key[0] < 'A' || key[0] > 'Z' {
		return false
	}
	for index := 1; index < len(key); index++ {
		character := key[index]
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func (environment preallocationEnvironment) valid() bool {
	if environment.seal != validPreallocationEnvironment {
		return false
	}
	paths, err := validateEnvironmentAuthorityInputs(environment.authorities)
	if err != nil {
		return false
	}
	expected, err := renderPreallocationEnvironment(paths)
	return err == nil && environment.rows == expected
}

func (environment taskPrivateEnvironment) valid() bool {
	if environment.seal != validTaskPrivateEnvironment {
		return false
	}
	paths, err := validateEnvironmentAuthorityInputs(environment.authorities)
	if err != nil {
		return false
	}
	workspace, err := environmentWorkspacePaths(environment.workspace, environment.authorities.gitExecutable)
	if err != nil {
		return false
	}
	expected, err := renderTaskPrivateEnvironment(paths, workspace)
	return err == nil && environment.rows == expected
}

func (environment preallocationEnvironment) clone() []string {
	if !environment.valid() {
		return nil
	}
	return append([]string(nil), environment.rows[:]...)
}

func (environment taskPrivateEnvironment) clone() []string {
	if !environment.valid() {
		return nil
	}
	return append([]string(nil), environment.rows[:]...)
}

// packageDescriptionAuthority is produced only by the authenticated graph
// layer. It has no raw-string constructor at the environment boundary.
type packageDescriptionAuthority struct {
	value string
	seal  packageDescriptionAuthoritySeal
}

type packageDescriptionAuthoritySeal uint8

const validPackageDescriptionAuthority packageDescriptionAuthoritySeal = 1

func (description packageDescriptionAuthority) valid() bool {
	return description.seal == validPackageDescriptionAuthority && description.value != "" &&
		utf8.ValidString(description.value) && !strings.ContainsAny(description.value, "\x00\r\n")
}

type assemblerDirectorySource uint8

const (
	assemblerDirectoryGOROOT assemblerDirectorySource = iota + 1
	assemblerDirectoryGOMODCACHE
	assemblerDirectoryPrivateSource
)

// assemblerDirectoryAuthority is produced by the corresponding authenticated
// GOROOT, GOMODCACHE, or private-source closure. No projection accepts a raw
// assembler path.
type assemblerDirectoryAuthority struct {
	path   string
	source assemblerDirectorySource
	seal   assemblerDirectoryAuthoritySeal
}

type assemblerDirectoryAuthoritySeal uint8

const validAssemblerDirectoryAuthority assemblerDirectoryAuthoritySeal = 1

func (directory assemblerDirectoryAuthority) valid(base goOwnedEnvironment) bool {
	if directory.seal != validAssemblerDirectoryAuthority || !validEnvironmentAuthorityPath(directory.path) ||
		!base.valid() {
		return false
	}
	paths, err := validateEnvironmentAuthorityInputs(base.direct.authorities)
	if err != nil {
		return false
	}
	workspace, err := environmentWorkspacePaths(base.direct.workspace, base.direct.authorities.gitExecutable)
	if err != nil {
		return false
	}
	var root string
	switch directory.source {
	case assemblerDirectoryGOROOT:
		root = paths.goroot
	case assemblerDirectoryGOMODCACHE:
		root = paths.gomodcache
	case assemblerDirectoryPrivateSource:
		root = workspace.child(workspaceCheckout)
	default:
		return false
	}
	return pathAtOrBelow(root, directory.path)
}

func pathAtOrBelow(root, candidate string) bool {
	if !validEnvironmentAuthorityPath(root) || !validEnvironmentAuthorityPath(candidate) {
		return false
	}
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type goOwnedEnvironment struct {
	direct taskPrivateEnvironment
	rows   goOwnedEnvironmentRows
	seal   goOwnedEnvironmentSeal
}

func deriveGoOwnedEnvironment(direct taskPrivateEnvironment) (goOwnedEnvironment, error) {
	if !direct.valid() {
		return goOwnedEnvironment{}, fail(CauseInternalInvariant, "invalid task-private Go environment")
	}
	rows, err := deriveGoOwnedEnvironmentRows(direct.rows)
	if err != nil {
		return goOwnedEnvironment{}, err
	}
	return goOwnedEnvironment{direct: direct, rows: rows, seal: validGoOwnedEnvironment}, nil
}

func deriveGoOwnedEnvironmentRows(direct directEnvironmentRows) (goOwnedEnvironmentRows, error) {
	rows := append([]string(nil), direct[:]...)
	if err := replaceExactEnvironmentRow(rows, "GOENV", "off", ""); err != nil {
		return goOwnedEnvironmentRows{}, err
	}
	goroot, err := requireEnvironmentValue(rows, "GOROOT")
	if err != nil {
		return goOwnedEnvironmentRows{}, err
	}
	rows = append(rows,
		"GOAUTH=netrc",
		"GOHOSTARCH=arm64",
		"GOHOSTOS=darwin",
		"GOTELEMETRY=off",
		"GOTOOLDIR="+filepath.Join(goroot, "pkg/tool/darwin_arm64"),
		"GOVERSION=go1.27.1",
		"GCCGO=gccgo",
		"AR=ar",
		"CC=cc",
		"CXX=c++",
		"GCM_INTERACTIVE=never",
	)
	if err := validateEnvironmentRows(rows, goOwnedEnvironmentRowCount); err != nil {
		return goOwnedEnvironmentRows{}, err
	}
	var sealed goOwnedEnvironmentRows
	copy(sealed[:], rows)
	return sealed, nil
}

func replaceExactEnvironmentRow(rows []string, key, oldValue, newValue string) error {
	want := key + "=" + oldValue
	replacement := key + "=" + newValue
	count := 0
	for index := range rows {
		if rows[index] == want {
			rows[index] = replacement
			count++
		}
	}
	if count != 1 {
		return fail(CauseInternalInvariant, "environment replacement cardinality mismatch")
	}
	return nil
}

func requireEnvironmentValue(rows []string, key string) (string, error) {
	prefix := key + "="
	value := ""
	count := 0
	for _, row := range rows {
		if suffix, found := strings.CutPrefix(row, prefix); found {
			value = suffix
			count++
		}
	}
	if count != 1 {
		return "", fail(CauseInternalInvariant, "environment key cardinality mismatch")
	}
	return value, nil
}

func validateEnvironmentRows(rows []string, expected int) error {
	if len(rows) != expected {
		return fail(CauseInternalInvariant, "derived environment row count mismatch")
	}
	keys := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key, value, found := strings.Cut(row, "=")
		if !found || !validEnvironmentKey(key) || strings.ContainsRune(value, '\x00') || !utf8.ValidString(value) {
			return fail(CauseInternalInvariant, "invalid derived environment row")
		}
		if _, duplicate := keys[key]; duplicate {
			return fail(CauseInternalInvariant, "duplicate derived environment key")
		}
		keys[key] = struct{}{}
	}
	return nil
}

func (environment goOwnedEnvironment) valid() bool {
	if environment.seal != validGoOwnedEnvironment || !environment.direct.valid() {
		return false
	}
	expected, err := deriveGoOwnedEnvironmentRows(environment.direct.rows)
	return err == nil && environment.rows == expected
}

func (environment goOwnedEnvironment) clone() []string {
	if !environment.valid() {
		return nil
	}
	return append([]string(nil), environment.rows[:]...)
}

type goGitEnvironment struct {
	base goOwnedEnvironment
	rows goGitEnvironmentRows
	seal goGitEnvironmentSeal
}

func deriveGoGitEnvironment(base goOwnedEnvironment) (goGitEnvironment, error) {
	if !base.valid() {
		return goGitEnvironment{}, fail(CauseInternalInvariant, "invalid Go-owned environment")
	}
	workspace, err := environmentWorkspacePaths(base.direct.workspace, base.direct.authorities.gitExecutable)
	if err != nil {
		return goGitEnvironment{}, err
	}
	rows := append(base.clone(), "PWD="+workspace.child(workspaceCheckout))
	if err := validateEnvironmentRows(rows, goGitEnvironmentRowCount); err != nil {
		return goGitEnvironment{}, err
	}
	var sealed goGitEnvironmentRows
	copy(sealed[:], rows)
	return goGitEnvironment{base: base, rows: sealed, seal: validGoGitEnvironment}, nil
}

func (environment goGitEnvironment) valid() bool {
	if environment.seal != validGoGitEnvironment || !environment.base.valid() {
		return false
	}
	expected, err := deriveGoGitEnvironment(environment.base)
	return err == nil && environment.rows == expected.rows
}

func (environment goGitEnvironment) clone() []string {
	if !environment.valid() {
		return nil
	}
	return append([]string(nil), environment.rows[:]...)
}

func (environment goGitEnvironment) directory() string {
	if !environment.valid() {
		return ""
	}
	workspace, err := environmentWorkspacePaths(
		environment.base.direct.workspace,
		environment.base.direct.authorities.gitExecutable,
	)
	if err != nil {
		return ""
	}
	return workspace.child(workspaceCheckout)
}

type goToolIDEnvironment struct {
	base goOwnedEnvironment
	rows goToolEnvironmentRows
	seal goToolEnvironmentSeal
}

func deriveGoToolIDEnvironment(base goOwnedEnvironment) (goToolIDEnvironment, error) {
	if !base.valid() {
		return goToolIDEnvironment{}, fail(CauseInternalInvariant, "invalid Go-owned environment")
	}
	var rows goToolEnvironmentRows
	copy(rows[:], base.rows[:])
	return goToolIDEnvironment{base: base, rows: rows, seal: validGoToolEnvironment}, nil
}

func (environment goToolIDEnvironment) valid() bool {
	if environment.seal != validGoToolEnvironment || !environment.base.valid() {
		return false
	}
	expected, err := deriveGoToolIDEnvironment(environment.base)
	return err == nil && environment.rows == expected.rows
}

func (environment goToolIDEnvironment) clone() []string {
	if !environment.valid() {
		return nil
	}
	return append([]string(nil), environment.rows[:]...)
}

func (goToolIDEnvironment) directory() string { return "" }

type goCompileEnvironment struct {
	base        goOwnedEnvironment
	description packageDescriptionAuthority
	rows        goBuildEnvironmentRows
	seal        goCompileEnvironmentSeal
}

func deriveGoCompileEnvironment(
	base goOwnedEnvironment,
	description packageDescriptionAuthority,
) (goCompileEnvironment, error) {
	if !base.valid() || !description.valid() {
		return goCompileEnvironment{}, fail(CauseInternalInvariant, "invalid Go compile environment authority")
	}
	workspace, err := environmentWorkspacePaths(base.direct.workspace, base.direct.authorities.gitExecutable)
	if err != nil {
		return goCompileEnvironment{}, err
	}
	rows := append(base.clone(),
		"PWD="+workspace.child(workspaceCheckout),
		"TOOLEXEC_IMPORTPATH="+description.value,
	)
	if err := validateEnvironmentRows(rows, goBuildEnvironmentRowCount); err != nil {
		return goCompileEnvironment{}, err
	}
	var sealed goBuildEnvironmentRows
	copy(sealed[:], rows)
	return goCompileEnvironment{
		base: base, description: description, rows: sealed, seal: validGoCompileEnvironment,
	}, nil
}

func (environment goCompileEnvironment) valid() bool {
	if environment.seal != validGoCompileEnvironment {
		return false
	}
	expected, err := deriveGoCompileEnvironment(environment.base, environment.description)
	return err == nil && environment.rows == expected.rows
}

func (environment goCompileEnvironment) clone() []string {
	if !environment.valid() {
		return nil
	}
	return append([]string(nil), environment.rows[:]...)
}

func (environment goCompileEnvironment) directory() string {
	if !environment.valid() {
		return ""
	}
	workspace, err := environmentWorkspacePaths(
		environment.base.direct.workspace,
		environment.base.direct.authorities.gitExecutable,
	)
	if err != nil {
		return ""
	}
	return workspace.child(workspaceCheckout)
}

type goAssemblerEnvironment struct {
	base               goOwnedEnvironment
	directoryAuthority assemblerDirectoryAuthority
	description        packageDescriptionAuthority
	rows               goBuildEnvironmentRows
	seal               goAssemblerEnvironmentSeal
}

func deriveGoAssemblerEnvironment(
	base goOwnedEnvironment,
	directory assemblerDirectoryAuthority,
	description packageDescriptionAuthority,
) (goAssemblerEnvironment, error) {
	if !base.valid() || !directory.valid(base) || !description.valid() {
		return goAssemblerEnvironment{}, fail(CauseInternalInvariant, "invalid Go assembler environment authority")
	}
	rows := append(base.clone(),
		"PWD="+directory.path,
		"TOOLEXEC_IMPORTPATH="+description.value,
	)
	if err := validateEnvironmentRows(rows, goBuildEnvironmentRowCount); err != nil {
		return goAssemblerEnvironment{}, err
	}
	var sealed goBuildEnvironmentRows
	copy(sealed[:], rows)
	return goAssemblerEnvironment{
		base: base, directoryAuthority: directory, description: description,
		rows: sealed, seal: validGoAssemblerEnvironment,
	}, nil
}

func (environment goAssemblerEnvironment) valid() bool {
	if environment.seal != validGoAssemblerEnvironment {
		return false
	}
	expected, err := deriveGoAssemblerEnvironment(
		environment.base,
		environment.directoryAuthority,
		environment.description,
	)
	return err == nil && environment.rows == expected.rows
}

func (environment goAssemblerEnvironment) clone() []string {
	if !environment.valid() {
		return nil
	}
	return append([]string(nil), environment.rows[:]...)
}

func (environment goAssemblerEnvironment) directory() string {
	if !environment.valid() {
		return ""
	}
	return environment.directoryAuthority.path
}

type goLinkEnvironment struct {
	base        goOwnedEnvironment
	description packageDescriptionAuthority
	rows        goLinkEnvironmentRows
	seal        goLinkEnvironmentSeal
}

func deriveGoLinkEnvironment(
	base goOwnedEnvironment,
	description packageDescriptionAuthority,
) (goLinkEnvironment, error) {
	if !base.valid() || !description.valid() {
		return goLinkEnvironment{}, fail(CauseInternalInvariant, "invalid Go link environment authority")
	}
	baseRows := base.clone()
	goroot, err := requireEnvironmentValue(baseRows, "GOROOT")
	if err != nil {
		return goLinkEnvironment{}, err
	}
	rows, err := removeEnvironmentKey(baseRows, "GOROOT")
	if err != nil {
		return goLinkEnvironment{}, err
	}
	rows = append(rows,
		"TOOLEXEC_IMPORTPATH="+description.value,
		"GOROOT="+goroot,
	)
	if err := validateEnvironmentRows(rows, goLinkEnvironmentRowCount); err != nil {
		return goLinkEnvironment{}, err
	}
	var sealed goLinkEnvironmentRows
	copy(sealed[:], rows)
	return goLinkEnvironment{
		base: base, description: description, rows: sealed, seal: validGoLinkEnvironment,
	}, nil
}

func removeEnvironmentKey(rows []string, key string) ([]string, error) {
	prefix := key + "="
	filtered := make([]string, 0, len(rows)-1)
	count := 0
	for _, row := range rows {
		if strings.HasPrefix(row, prefix) {
			count++
			continue
		}
		filtered = append(filtered, row)
	}
	if count != 1 {
		return nil, fail(CauseInternalInvariant, "environment removal cardinality mismatch")
	}
	return filtered, nil
}

func (environment goLinkEnvironment) valid() bool {
	if environment.seal != validGoLinkEnvironment {
		return false
	}
	expected, err := deriveGoLinkEnvironment(environment.base, environment.description)
	return err == nil && environment.rows == expected.rows
}

func (environment goLinkEnvironment) clone() []string {
	if !environment.valid() {
		return nil
	}
	return append([]string(nil), environment.rows[:]...)
}

func (goLinkEnvironment) directory() string { return "" }

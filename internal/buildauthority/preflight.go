package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"unicode/utf8"
)

const (
	goEnvironmentFileName = "go.env"
	goVersionWire         = "go version go1.27.1 darwin/arm64\n"
	compilerVersionWire   = "compile version go1.27.1 X:nojsonv2,nogreenteagc," +
		"norandomizedheapbase64,nosizespecializedmalloc\n"
	gitVersionWire = "git version 2.50.1 (Apple Git-155)\n" +
		"cpu: arm64\n" +
		"no commit associated with this build\n" +
		"sizeof-long: 8\n" +
		"sizeof-size_t: 8\n" +
		"shell-path: /bin/sh\n" +
		"feature: fsmonitor--daemon\n" +
		"libcurl: 8.7.1\n" +
		"zlib: 1.2.12\n" +
		"SHA-1: SHA1_DC\n" +
		"SHA-256: SHA256_BLK\n"
	gitBuiltinWireBytes = 1_467
	gitBuiltinCount     = 144
)

type goEnvironmentFileClaimSeal uint8
type goVersionPlanSeal uint8
type goEnvironmentPlanSeal uint8
type compilerVersionPlanSeal uint8
type gitVersionPlanSeal uint8
type gitBuiltinInventoryPlanSeal uint8
type nonSourcePlansSeal uint8
type physicalRootSentinelClaimSeal uint8

const (
	validGoEnvironmentFileClaim    goEnvironmentFileClaimSeal    = 1
	validGoVersionPlan             goVersionPlanSeal             = 1
	validGoEnvironmentPlan         goEnvironmentPlanSeal         = 1
	validCompilerVersionPlan       compilerVersionPlanSeal       = 1
	validGitVersionPlan            gitVersionPlanSeal            = 1
	validGitBuiltinInventoryPlan   gitBuiltinInventoryPlanSeal   = 1
	validNonSourcePlans            nonSourcePlansSeal            = 1
	validPhysicalRootSentinelClaim physicalRootSentinelClaimSeal = 1
)

// goEnvironmentFileClaim binds parsed literal Go defaults to the exact
// regular-file row authenticated by the retained GOROOT capture. It has no
// production constructor yet: the future owner must create it only inside an
// operation-attributed root-freshness window that also owns leaf closure.
type goEnvironmentFileClaim struct {
	goroot       *treeCapture
	gorootDigest Digest
	entry        entrySnapshot
	seal         goEnvironmentFileClaimSeal
}

// The five plan types are deliberately nominal. In particular, the two Go
// plans and two Git plans cannot be converted into one another despite
// retaining the same kinds of authority.
type goVersionPlan struct {
	environment preallocationEnvironment
	executable  *goAuthority
	seal        goVersionPlanSeal
}

type goEnvironmentPlan struct {
	environment       preallocationEnvironment
	executable        *goAuthority
	goEnvironmentFile *goEnvironmentFileClaim
	seal              goEnvironmentPlanSeal
}

type compilerVersionPlan struct {
	environment preallocationEnvironment
	executable  *compilerAuthority
	seal        compilerVersionPlanSeal
}

type gitVersionPlan struct {
	environment preallocationEnvironment
	executable  *gitAuthority
	seal        gitVersionPlanSeal
}

type gitBuiltinInventoryPlan struct {
	environment preallocationEnvironment
	executable  *gitAuthority
	seal        gitBuiltinInventoryPlanSeal
}

// nonSourcePlans records fixed cardinality and normative order as fields rather
// than caller data. The future block owner must consume these five fields
// directly; this type intentionally offers no reorderable slice projection.
type nonSourcePlans struct {
	goVersion       goVersionPlan
	goEnvironment   goEnvironmentPlan
	compilerVersion compilerVersionPlan
	gitVersion      gitVersionPlan
	gitBuiltins     gitBuiltinInventoryPlan
	seal            nonSourcePlansSeal
}

// physicalRootSentinelClaim represents the exact fixed absent-name set. The
// names are module constants rather than fields that a caller could replace.
type physicalRootSentinelClaim struct {
	root *physicalRootAuthority
	seal physicalRootSentinelClaimSeal
}

type relativeEntryKindNoFollowFunc func(int, string) (entryKind, error)

func requiredGoEnvironmentEntry(goroot *treeCapture) (entrySnapshot, *FailureRecord) {
	if goroot == nil || goroot.root == nil || goroot.root.root == nil ||
		goroot.root.descriptor == nil || goroot.policy != gorootPolicy() ||
		goroot.digest == (Digest{}) {
		return entrySnapshot{}, authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	var matched entrySnapshot
	matches := 0
	for _, entry := range goroot.entries {
		if entry.path == goEnvironmentFileName {
			matched = entry
			matches++
		}
	}
	if matches == 0 {
		return entrySnapshot{}, authorityFailure(OperationOpen, CauseNotFound)
	}
	if matches != 1 {
		return entrySnapshot{}, authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	if matched.kind != entryRegular || snapshotKind(matched.file) != entryRegular {
		return entrySnapshot{}, authorityFailure(OperationValidate, CauseUnsupported)
	}
	return matched, nil
}

func parseGoEnvironmentFile(content []byte) error {
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 ||
		bytes.IndexByte(content, '\r') >= 0 {
		return malformedGoEnvironment()
	}
	var seen uint8
	for len(content) > 0 {
		line := content
		if newline := bytes.IndexByte(content, '\n'); newline >= 0 {
			line = content[:newline]
			content = content[newline+1:]
		} else {
			content = nil
		}
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		bit, err := parseGoEnvironmentAssignment(line)
		if err != nil {
			return err
		}
		if seen&bit != 0 {
			return malformedGoEnvironment()
		}
		seen |= bit
	}
	if seen != 1<<3-1 {
		return malformedGoEnvironment()
	}
	return nil
}

func parseGoEnvironmentAssignment(line []byte) (uint8, error) {
	key, value, found := bytes.Cut(line, []byte{'='})
	if !found || !validEnvironmentKeyBytes(key) {
		return 0, malformedGoEnvironment()
	}
	var bit uint8
	var expected string
	switch {
	case bytesEqualString(key, "GOPROXY"):
		bit = 1 << 0
		expected = "https://proxy.golang.org,direct"
	case bytesEqualString(key, "GOSUMDB"):
		bit = 1 << 1
		expected = "sum.golang.org"
	case bytesEqualString(key, "GOTOOLCHAIN"):
		bit = 1 << 2
		expected = "auto"
	default:
		return 0, fail(CauseUnsupported, "GOROOT go.env assignment refused")
	}
	if !bytesEqualString(value, expected) {
		return 0, fail(CauseUnsupported, "GOROOT go.env value refused")
	}
	return bit, nil
}

func validEnvironmentKeyBytes(key []byte) bool {
	if len(key) == 0 || key[0] < 'A' || key[0] > 'Z' {
		return false
	}
	for _, character := range key[1:] {
		if (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func malformedGoEnvironment() error {
	return fail(CauseMalformed, "GOROOT go.env is malformed")
}

func (claim *goEnvironmentFileClaim) validFor(goroot *treeCapture) bool {
	if claim == nil || claim.seal != validGoEnvironmentFileClaim ||
		claim.goroot != goroot || claim.gorootDigest == (Digest{}) ||
		claim.gorootDigest != goroot.digest {
		return false
	}
	entry, failure := requiredGoEnvironmentEntry(goroot)
	return failure == nil && entry == claim.entry
}

func newNonSourcePlans(
	environment preallocationEnvironment,
	goEnvironmentFile *goEnvironmentFileClaim,
) (nonSourcePlans, error) {
	goVersion, err := newGoVersionPlan(environment, environment.authorities.goExecutable)
	if err != nil {
		return nonSourcePlans{}, err
	}
	goEnvironment, err := newGoEnvironmentPlan(
		environment,
		environment.authorities.goExecutable,
		goEnvironmentFile,
	)
	if err != nil {
		return nonSourcePlans{}, err
	}
	compilerVersion, err := newCompilerVersionPlan(
		environment,
		environment.authorities.compiler,
	)
	if err != nil {
		return nonSourcePlans{}, err
	}
	gitVersion, err := newGitVersionPlan(environment, environment.authorities.gitExecutable)
	if err != nil {
		return nonSourcePlans{}, err
	}
	gitBuiltins, err := newGitBuiltinInventoryPlan(
		environment,
		environment.authorities.gitExecutable,
	)
	if err != nil {
		return nonSourcePlans{}, err
	}
	return nonSourcePlans{
		goVersion:       goVersion,
		goEnvironment:   goEnvironment,
		compilerVersion: compilerVersion,
		gitVersion:      gitVersion,
		gitBuiltins:     gitBuiltins,
		seal:            validNonSourcePlans,
	}, nil
}

func newGoVersionPlan(
	environment preallocationEnvironment,
	executable *goAuthority,
) (goVersionPlan, error) {
	if !environment.valid() || executable == nil ||
		executable != environment.authorities.goExecutable {
		return goVersionPlan{}, fail(CauseInternalInvariant, "invalid Go-version plan authority")
	}
	return goVersionPlan{
		environment: environment,
		executable:  executable,
		seal:        validGoVersionPlan,
	}, nil
}

func newGoEnvironmentPlan(
	environment preallocationEnvironment,
	executable *goAuthority,
	goEnvironmentFile *goEnvironmentFileClaim,
) (goEnvironmentPlan, error) {
	if !environment.valid() || executable == nil ||
		executable != environment.authorities.goExecutable ||
		!goEnvironmentFile.validFor(environment.authorities.goroot) {
		return goEnvironmentPlan{}, fail(CauseInternalInvariant, "invalid Go-environment plan authority")
	}
	if _, _, err := expectedGoEnvironmentOutput(environment); err != nil {
		return goEnvironmentPlan{}, err
	}
	return goEnvironmentPlan{
		environment:       environment,
		executable:        executable,
		goEnvironmentFile: goEnvironmentFile,
		seal:              validGoEnvironmentPlan,
	}, nil
}

func newCompilerVersionPlan(
	environment preallocationEnvironment,
	executable *compilerAuthority,
) (compilerVersionPlan, error) {
	if !environment.valid() || executable == nil ||
		executable != environment.authorities.compiler {
		return compilerVersionPlan{}, fail(
			CauseInternalInvariant,
			"invalid compiler-version plan authority",
		)
	}
	return compilerVersionPlan{
		environment: environment,
		executable:  executable,
		seal:        validCompilerVersionPlan,
	}, nil
}

func newGitVersionPlan(
	environment preallocationEnvironment,
	executable *gitAuthority,
) (gitVersionPlan, error) {
	if !environment.valid() || executable == nil ||
		executable != environment.authorities.gitExecutable {
		return gitVersionPlan{}, fail(CauseInternalInvariant, "invalid Git-version plan authority")
	}
	return gitVersionPlan{
		environment: environment,
		executable:  executable,
		seal:        validGitVersionPlan,
	}, nil
}

func newGitBuiltinInventoryPlan(
	environment preallocationEnvironment,
	executable *gitAuthority,
) (gitBuiltinInventoryPlan, error) {
	if !environment.valid() || executable == nil ||
		executable != environment.authorities.gitExecutable {
		return gitBuiltinInventoryPlan{}, fail(
			CauseInternalInvariant,
			"invalid Git-builtin plan authority",
		)
	}
	return gitBuiltinInventoryPlan{
		environment: environment,
		executable:  executable,
		seal:        validGitBuiltinInventoryPlan,
	}, nil
}

func (plans nonSourcePlans) valid() bool {
	if plans.seal != validNonSourcePlans || !plans.goVersion.valid() ||
		!plans.goEnvironment.valid() || !plans.compilerVersion.valid() ||
		!plans.gitVersion.valid() || !plans.gitBuiltins.valid() {
		return false
	}
	environment := plans.goVersion.environment
	return plans.goEnvironment.environment == environment &&
		plans.compilerVersion.environment == environment &&
		plans.gitVersion.environment == environment &&
		plans.gitBuiltins.environment == environment
}

func (plan goVersionPlan) valid() bool {
	return plan.seal == validGoVersionPlan && plan.environment.valid() &&
		plan.executable != nil &&
		plan.executable == plan.environment.authorities.goExecutable
}

func (plan goEnvironmentPlan) valid() bool {
	return plan.seal == validGoEnvironmentPlan && plan.environment.valid() &&
		plan.executable != nil &&
		plan.executable == plan.environment.authorities.goExecutable &&
		plan.goEnvironmentFile.validFor(plan.environment.authorities.goroot)
}

func (plan compilerVersionPlan) valid() bool {
	return plan.seal == validCompilerVersionPlan && plan.environment.valid() &&
		plan.executable != nil &&
		plan.executable == plan.environment.authorities.compiler
}

func (plan gitVersionPlan) valid() bool {
	return plan.seal == validGitVersionPlan && plan.environment.valid() &&
		plan.executable != nil &&
		plan.executable == plan.environment.authorities.gitExecutable
}

func (plan gitBuiltinInventoryPlan) valid() bool {
	return plan.seal == validGitBuiltinInventoryPlan && plan.environment.valid() &&
		plan.executable != nil &&
		plan.executable == plan.environment.authorities.gitExecutable
}

func goEnvironmentArguments() []string {
	return []string{
		"env",
		"-json",
		"GOROOT",
		"GOMODCACHE",
		"GOCACHE",
		"GOCACHEPROG",
		"GOENV",
		"GOFLAGS",
		"GOWORK",
		"GOTOOLCHAIN",
		"GOPROXY",
		"GOSUMDB",
		"GOVCS",
		"GOTELEMETRY",
		"GOTELEMETRYDIR",
		"GO111MODULE",
		"GOEXPERIMENT",
		"GOFIPS140",
		"GO_EXTLINK_ENABLED",
		"CGO_ENABLED",
		"GOHOSTOS",
		"GOHOSTARCH",
		"GOTOOLDIR",
		"GOVERSION",
		"GOOS",
		"GOARCH",
		"GOAMD64",
		"GOARM64",
	}
}

func (plan goVersionPlan) validateOutput(output []byte) *FailureRecord {
	if !plan.valid() {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	return validateSimpleVersionOutput(
		output,
		[]byte(goVersionWire),
		"go",
		"version",
	)
}

func (plan goEnvironmentPlan) validateOutput(output []byte) *FailureRecord {
	if !plan.valid() {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	expectedWire, _, err := expectedGoEnvironmentOutput(plan.environment)
	if err != nil {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	if bytes.Equal(output, expectedWire) {
		return nil
	}
	actualValues, ok := parseCanonicalGoEnvironmentOutput(output)
	if !ok {
		return authorityFailure(OperationParse, CauseMalformed)
	}
	expectedValues, ok := parseCanonicalGoEnvironmentOutput(expectedWire)
	if !ok {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	for index, key := range goEnvironmentOutputKeys() {
		if bytes.Equal(actualValues[index], expectedValues[index]) {
			continue
		}
		if goEnvironmentIdentityKey(key) {
			return authorityFailure(OperationCompare, CauseIdentity)
		}
		return authorityFailure(OperationCompare, CauseUnsupported)
	}
	return authorityFailure(OperationCompare, CauseUnsupported)
}

func (plan compilerVersionPlan) validateOutput(output []byte) *FailureRecord {
	if !plan.valid() {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	return validateSimpleVersionOutput(
		output,
		[]byte(compilerVersionWire),
		"compile",
		"version",
	)
}

func (plan gitVersionPlan) validateOutput(output []byte) *FailureRecord {
	if !plan.valid() {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	if bytes.Equal(output, []byte(gitVersionWire)) {
		return nil
	}
	if !validGitVersionWireShape(output) {
		return authorityFailure(OperationParse, CauseMalformed)
	}
	return authorityFailure(OperationCompare, CauseUnsupported)
}

func (plan gitBuiltinInventoryPlan) validateOutput(output []byte) *FailureRecord {
	if !plan.valid() {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	lines, ok := parseGitBuiltinLines(output)
	if !ok {
		return authorityFailure(OperationParse, CauseMalformed)
	}
	if !requiredGitBuiltinPositions(lines) || len(output) != gitBuiltinWireBytes ||
		Digest(sha256.Sum256(output)) != gitBuiltinWireDigest() {
		return authorityFailure(OperationCompare, CauseUnsupported)
	}
	return nil
}

func validateSimpleVersionOutput(
	output, expected []byte,
	first, second string,
) *FailureRecord {
	if bytes.Equal(output, expected) {
		return nil
	}
	line, ok := oneCanonicalLine(output)
	if !ok {
		return authorityFailure(OperationParse, CauseMalformed)
	}
	var fields [4][]byte
	if !splitExactByteSeparated(line, ' ', fields[:]) ||
		!bytesEqualString(fields[0], first) ||
		!bytesEqualString(fields[1], second) ||
		len(fields[2]) == 0 || len(fields[3]) == 0 {
		return authorityFailure(OperationParse, CauseMalformed)
	}
	return authorityFailure(OperationCompare, CauseUnsupported)
}

func oneCanonicalLine(output []byte) ([]byte, bool) {
	var lines [1][]byte
	if !splitExactLines(output, lines[:]) || !canonicalTextRow(lines[0]) {
		return nil, false
	}
	return lines[0], true
}

func validGitVersionWireShape(output []byte) bool {
	var lines [11][]byte
	if !splitExactLines(output, lines[:]) ||
		!canonicalTextRows(lines[:]) ||
		!bytesHaveNonemptySuffix(lines[0], "git version ") ||
		len(lines[2]) == 0 || bytes.IndexByte(lines[2], ':') >= 0 {
		return false
	}
	prefixes := [9]struct {
		index  int
		prefix string
	}{
		{index: 1, prefix: "cpu: "},
		{index: 3, prefix: "sizeof-long: "},
		{index: 4, prefix: "sizeof-size_t: "},
		{index: 5, prefix: "shell-path: "},
		{index: 6, prefix: "feature: "},
		{index: 7, prefix: "libcurl: "},
		{index: 8, prefix: "zlib: "},
		{index: 9, prefix: "SHA-1: "},
		{index: 10, prefix: "SHA-256: "},
	}
	for _, row := range prefixes {
		if !bytesHaveNonemptySuffix(lines[row.index], row.prefix) {
			return false
		}
	}
	return true
}

// splitExactLines writes views into output without copying caller-controlled
// bytes. A fixed-size caller array bounds both line cardinality and parser
// memory independently of the structured-output admission ceiling.
func splitExactLines(output []byte, lines [][]byte) bool {
	if len(lines) == 0 || len(output) == 0 || output[len(output)-1] != '\n' {
		return false
	}
	remainder := output
	for index := range lines {
		newline := bytes.IndexByte(remainder, '\n')
		if newline < 0 {
			return false
		}
		lines[index] = remainder[:newline]
		remainder = remainder[newline+1:]
	}
	return len(remainder) == 0
}

func splitExactByteSeparated(input []byte, separator byte, fields [][]byte) bool {
	if len(fields) == 0 {
		return false
	}
	remainder := input
	for index := range fields[:len(fields)-1] {
		next := bytes.IndexByte(remainder, separator)
		if next < 0 {
			return false
		}
		fields[index] = remainder[:next]
		remainder = remainder[next+1:]
	}
	if bytes.IndexByte(remainder, separator) >= 0 {
		return false
	}
	fields[len(fields)-1] = remainder
	return true
}

func canonicalTextRows(lines [][]byte) bool {
	for _, line := range lines {
		if !canonicalTextRow(line) {
			return false
		}
	}
	return true
}

func canonicalTextRow(row []byte) bool {
	if len(row) == 0 || row[0] == ' ' || row[len(row)-1] == ' ' ||
		bytes.Contains(row, []byte("  ")) {
		return false
	}
	for index := range len(row) {
		if row[index] < 0x20 || row[index] > 0x7e {
			return false
		}
	}
	return true
}

func bytesEqualString(value []byte, expected string) bool {
	if len(value) != len(expected) {
		return false
	}
	for index := range len(value) {
		if value[index] != expected[index] {
			return false
		}
	}
	return true
}

func bytesHaveNonemptySuffix(value []byte, prefix string) bool {
	return len(value) > len(prefix) && bytesEqualString(value[:len(prefix)], prefix)
}

func expectedGoEnvironmentOutput(
	environment preallocationEnvironment,
) ([]byte, map[string]string, error) {
	if !environment.valid() {
		return nil, nil, fail(CauseInternalInvariant, "invalid Go-environment plan")
	}
	paths, err := validateEnvironmentTreeAuthorities(
		environment.authorities.goroot,
		environment.authorities.gomodcache,
	)
	if err != nil {
		return nil, nil, err
	}
	toolDirectory := filepath.Join(paths.goroot, "pkg", "tool", "darwin_arm64")
	if !validEnvironmentAuthorityPath(toolDirectory) {
		return nil, nil, fail(CauseInternalInvariant, "invalid retained Go tool directory")
	}
	values := map[string]string{
		"CGO_ENABLED":        "0",
		"GO111MODULE":        "on",
		"GOAMD64":            "",
		"GOARCH":             "arm64",
		"GOARM64":            "v8.0",
		"GOCACHE":            "off",
		"GOCACHEPROG":        "",
		"GOENV":              "",
		"GOEXPERIMENT":       "none",
		"GOFIPS140":          "off",
		"GOFLAGS":            "-mod=readonly",
		"GOHOSTARCH":         "arm64",
		"GOHOSTOS":           "darwin",
		"GOMODCACHE":         paths.gomodcache,
		"GOOS":               "darwin",
		"GOPROXY":            "off",
		"GOROOT":             paths.goroot,
		"GOSUMDB":            "off",
		"GOTELEMETRY":        "off",
		"GOTELEMETRYDIR":     "",
		"GOTOOLCHAIN":        "local",
		"GOTOOLDIR":          toolDirectory,
		"GOVCS":              "*:off",
		"GOVERSION":          "go1.27.1",
		"GOWORK":             "off",
		"GO_EXTLINK_ENABLED": "0",
	}
	wire, err := encodeCanonicalStringMap(values)
	if err != nil {
		return nil, nil, err
	}
	return wire, values, nil
}

func parseCanonicalGoEnvironmentOutput(output []byte) ([26][]byte, bool) {
	var values [26][]byte
	var lines [28][]byte
	if !splitExactLines(output, lines[:]) ||
		!bytes.Equal(lines[0], []byte{'{'}) ||
		!bytes.Equal(lines[len(lines)-1], []byte{'}'}) {
		return values, false
	}
	keys := goEnvironmentOutputKeys()
	for index, key := range keys {
		value, ok := parseCanonicalGoEnvironmentRow(
			lines[index+1],
			key,
			index != len(keys)-1,
		)
		if !ok {
			return [26][]byte{}, false
		}
		values[index] = value
	}
	return values, true
}

func parseCanonicalGoEnvironmentRow(
	row []byte,
	key string,
	comma bool,
) ([]byte, bool) {
	prefixBytes := 5 + len(key)
	minimumBytes := prefixBytes + 2
	if comma {
		minimumBytes++
	}
	if len(row) < minimumBytes || row[0] != '\t' || row[1] != '"' ||
		!bytesEqualString(row[2:2+len(key)], key) ||
		row[2+len(key)] != '"' || row[3+len(key)] != ':' ||
		row[4+len(key)] != ' ' {
		return nil, false
	}
	end := len(row)
	if comma {
		if row[end-1] != ',' {
			return nil, false
		}
		end--
	} else if row[end-1] == ',' {
		return nil, false
	}
	value := row[prefixBytes:end]
	return value, canonicalJSONString(value)
}

// canonicalJSONString recognizes exactly the byte form emitted by the pinned
// encoding/json string encoder with HTML escaping. It deliberately scans the
// retained output in place: a wrong value may be nearly 256 MiB, but cannot
// induce a proportional parser allocation before its typed refusal.
func canonicalJSONString(value []byte) bool {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return false
	}
	end := len(value) - 1
	for index := 1; index < end; {
		character := value[index]
		if character < utf8.RuneSelf {
			switch character {
			case '"', '<', '>', '&':
				return false
			case '\\':
				width, ok := canonicalJSONEscape(value[index:end])
				if !ok {
					return false
				}
				index += width
				continue
			default:
				if character < 0x20 {
					return false
				}
				index++
				continue
			}
		}
		runeValue, width := utf8.DecodeRune(value[index:end])
		if runeValue == utf8.RuneError && width == 1 ||
			runeValue == '\u2028' || runeValue == '\u2029' {
			return false
		}
		index += width
	}
	return true
}

func canonicalJSONEscape(value []byte) (int, bool) {
	if len(value) < 2 || value[0] != '\\' {
		return 0, false
	}
	switch value[1] {
	case '"', '\\', 'b', 'f', 'n', 'r', 't':
		return 2, true
	case 'u':
		return 6, canonicalJSONUnicodeEscape(value)
	default:
		return 0, false
	}
}

func canonicalJSONUnicodeEscape(value []byte) bool {
	if len(value) < 6 {
		return false
	}
	if bytesEqualString(value[2:6], "2028") ||
		bytesEqualString(value[2:6], "2029") {
		return true
	}
	if value[2] != '0' || value[3] != '0' {
		return false
	}
	high, highOK := lowercaseHexNibble(value[4])
	low, lowOK := lowercaseHexNibble(value[5])
	if !highOK || !lowOK {
		return false
	}
	return canonicalEscapedASCII(high<<4 | low)
}

func canonicalEscapedASCII(character byte) bool {
	switch {
	case character < '\b':
		return true
	case character == '\v':
		return true
	case character >= 0x0e && character < 0x20:
		return true
	case character == '&', character == '<', character == '>':
		return true
	default:
		return false
	}
}

func lowercaseHexNibble(character byte) (byte, bool) {
	switch {
	case character >= '0' && character <= '9':
		return character - '0', true
	case character >= 'a' && character <= 'f':
		return character - 'a' + 10, true
	default:
		return 0, false
	}
}

func encodeCanonicalStringMap(values map[string]string) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "\t")
	if err := encoder.Encode(values); err != nil {
		return nil, failWith(err, CauseInternalInvariant, "encode canonical Go environment")
	}
	return output.Bytes(), nil
}

func goEnvironmentOutputKeys() [26]string {
	return [26]string{
		"CGO_ENABLED",
		"GO111MODULE",
		"GOAMD64",
		"GOARCH",
		"GOARM64",
		"GOCACHE",
		"GOCACHEPROG",
		"GOENV",
		"GOEXPERIMENT",
		"GOFIPS140",
		"GOFLAGS",
		"GOHOSTARCH",
		"GOHOSTOS",
		"GOMODCACHE",
		"GOOS",
		"GOPROXY",
		"GOROOT",
		"GOSUMDB",
		"GOTELEMETRY",
		"GOTELEMETRYDIR",
		"GOTOOLCHAIN",
		"GOTOOLDIR",
		"GOVCS",
		"GOVERSION",
		"GOWORK",
		"GO_EXTLINK_ENABLED",
	}
}

func goEnvironmentIdentityKey(key string) bool {
	switch key {
	case "GOMODCACHE", "GOROOT", "GOTOOLDIR":
		return true
	default:
		return false
	}
}

func parseGitBuiltinLines(output []byte) ([gitBuiltinCount][]byte, bool) {
	var lines [gitBuiltinCount][]byte
	if !splitExactLines(output, lines[:]) || !canonicalTextRows(lines[:]) {
		return lines, false
	}
	var previous []byte
	for _, line := range lines {
		if !validGitBuiltinToken(line) {
			return [gitBuiltinCount][]byte{}, false
		}
		if previous != nil && bytes.Compare(previous, line) >= 0 {
			return [gitBuiltinCount][]byte{}, false
		}
		previous = line
	}
	return lines, true
}

func validGitBuiltinToken(token []byte) bool {
	if len(token) == 0 || token[0] == '-' || token[len(token)-1] == '-' {
		return false
	}
	for index := range len(token) {
		character := token[index]
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func requiredGitBuiltinPositions(lines [gitBuiltinCount][]byte) bool {
	required := [11]struct {
		position int
		name     string
	}{
		{position: 12, name: "cat-file"},
		{position: 13, name: "check-attr"},
		{position: 17, name: "checkout"},
		{position: 28, name: "config"},
		{position: 47, name: "for-each-ref"},
		{position: 50, name: "fsck"},
		{position: 60, name: "init"},
		{position: 63, name: "log"},
		{position: 66, name: "ls-tree"},
		{position: 111, name: "rev-parse"},
		{position: 123, name: "status"},
	}
	for _, row := range required {
		if !bytesEqualString(lines[row.position-1], row.name) {
			return false
		}
	}
	return true
}

func gitBuiltinWireDigest() Digest {
	return Digest{
		0x77, 0x12, 0xb9, 0x8c, 0x14, 0x6e, 0x53, 0xd5,
		0xa2, 0x61, 0x19, 0x53, 0x39, 0x01, 0xa7, 0x48,
		0x17, 0x6f, 0x97, 0x43, 0xe6, 0x0e, 0x86, 0x75,
		0xa3, 0x96, 0xc4, 0xf1, 0x03, 0xdd, 0x86, 0x25,
	}
}

func authorityFailure(operation Operation, cause CauseCode) *FailureRecord {
	return &FailureRecord{
		Phase:     PhaseAuthority,
		Operation: operation,
		Causes:    []CauseCode{cause},
	}
}

// Root identity/security revalidation is a separate block-owner primitive.
// Keeping it outside this absence claim preserves whether a failing root
// primitive owns probe or compare attribution.
func admitPhysicalRootSentinels(
	ctx context.Context,
	root *physicalRootAuthority,
) (physicalRootSentinelClaim, *FailureRecord) {
	return admitPhysicalRootSentinelsWith(ctx, root, relativeEntryKindNoFollow)
}

func admitPhysicalRootSentinelsWith(
	ctx context.Context,
	root *physicalRootAuthority,
	lookup relativeEntryKindNoFollowFunc,
) (physicalRootSentinelClaim, *FailureRecord) {
	if _, err := validatePhysicalRootAuthorityShape(root); err != nil || lookup == nil {
		return physicalRootSentinelClaim{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if failure := inspectPhysicalRootSentinels(ctx, root, lookup, true); failure != nil {
		return physicalRootSentinelClaim{}, failure
	}
	return physicalRootSentinelClaim{
		root: root,
		seal: validPhysicalRootSentinelClaim,
	}, nil
}

func (claim physicalRootSentinelClaim) revalidate(
	ctx context.Context,
) *FailureRecord {
	return claim.revalidateWith(ctx, relativeEntryKindNoFollow)
}

func (claim physicalRootSentinelClaim) revalidateWith(
	ctx context.Context,
	lookup relativeEntryKindNoFollowFunc,
) *FailureRecord {
	if claim.seal != validPhysicalRootSentinelClaim || lookup == nil {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	if _, err := validatePhysicalRootAuthorityShape(claim.root); err != nil {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	return inspectPhysicalRootSentinels(ctx, claim.root, lookup, false)
}

func inspectPhysicalRootSentinels(
	ctx context.Context,
	root *physicalRootAuthority,
	lookup relativeEntryKindNoFollowFunc,
	initial bool,
) *FailureRecord {
	directory, err := validatePhysicalRootAuthorityShape(root)
	if err != nil {
		return authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	for _, name := range physicalRootSentinelNames() {
		if err := checkContext(ctx, "inspect physical-root sentinel"); err != nil {
			return authorityFailure(
				OperationProbe,
				privateCauses(err, CauseInternalInvariant)[0],
			)
		}
		_, lookupErr := lookup(int(directory.descriptor.Fd()), name)
		switch {
		case lookupErr == nil, errors.Is(lookupErr, errUnsupportedAuthorityEntry):
			if initial {
				return authorityFailure(OperationValidate, CauseUnsupported)
			}
			return authorityFailure(OperationCompare, CauseUnstable)
		case errors.Is(lookupErr, fs.ErrNotExist):
			continue
		case errors.Is(lookupErr, fs.ErrPermission):
			return authorityFailure(OperationProbe, CausePermission)
		default:
			return authorityFailure(OperationProbe, CauseUnstable)
		}
	}
	return nil
}

func physicalRootSentinelNames() [3]string {
	return [3]string{"go.mod", "go.work", ".git"}
}

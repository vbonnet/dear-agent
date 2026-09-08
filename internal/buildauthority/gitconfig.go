package buildauthority

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

const (
	maxSourceConfigBytes   = 1 << 20
	maxSourceConfigRecords = 4_096
	maxPackedRefsBytes     = 16 << 20
	maxPackedRefsRows      = 1_000_000
)

type repositoryObjectFormat uint8

const (
	objectFormatUnknown repositoryObjectFormat = iota
	objectFormatSHA1
	objectFormatSHA256
)

func (format repositoryObjectFormat) name() string {
	switch format {
	case objectFormatUnknown:
		return ""
	case objectFormatSHA1:
		return "sha1"
	case objectFormatSHA256:
		return "sha256"
	default:
		return ""
	}
}

func (format repositoryObjectFormat) hexWidth() int {
	switch format {
	case objectFormatUnknown:
		return 0
	case objectFormatSHA1:
		return 40
	case objectFormatSHA256:
		return 64
	default:
		return 0
	}
}

type sourceConfigSection struct {
	name          string
	subsection    string
	hasSubsection bool
}

type sourceConfigKey struct {
	section       string
	subsection    string
	hasSubsection bool
	variable      string
}

func (key sourceConfigKey) normalizedName() string {
	if !key.hasSubsection {
		return key.section + "." + key.variable
	}
	return key.section + "." + key.subsection + "." + key.variable
}

type sourceConfigEntry struct {
	key   sourceConfigKey
	value string
}

// sourceConfigClaim is the complete semantic and ordered transcript claim for
// one directly parsed source .git/config. Its fields remain private so callers
// cannot invent a partially validated configuration.
type sourceConfigClaim struct {
	objectFormat          repositoryObjectFormat
	worktreeConfigPresent bool
	entries               []sourceConfigEntry
}

func parseSourceConfig(content []byte) (sourceConfigClaim, error) {
	if len(content) > maxSourceConfigBytes {
		return sourceConfigClaim{}, fail(CauseLimit, "source config exceeds byte limit")
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return sourceConfigClaim{}, fail(CauseMalformed, "source config contains NUL")
	}

	entries := make([]sourceConfigEntry, 0, 32)
	seen := make(map[sourceConfigKey]struct{})
	var section sourceConfigSection
	sectionSet := false
	for lineNumber, lineBytes := range bytes.Split(content, []byte{'\n'}) {
		if err := validateSourceConfigLineBytes(lineBytes); err != nil {
			return sourceConfigClaim{}, failWith(err, CauseMalformed, fmt.Sprintf("source config line %d", lineNumber+1))
		}
		line := string(lineBytes)
		trimmed := trimHorizontalLeft(line)
		if trimmed == "" || trimmed[0] == '#' || trimmed[0] == ';' {
			continue
		}
		if trimmed[0] == '[' {
			parsed, err := parseSourceConfigSection(trimmed)
			if err != nil {
				return sourceConfigClaim{}, failWith(err, CauseMalformed, fmt.Sprintf("source config section on line %d", lineNumber+1))
			}
			section = parsed
			sectionSet = true
			continue
		}
		if !sectionSet {
			return sourceConfigClaim{}, fail(CauseMalformed, "source config assignment precedes a section")
		}
		entry, err := parseSourceConfigAssignment(section, trimmed)
		if err != nil {
			return sourceConfigClaim{}, failWith(err, CauseMalformed, fmt.Sprintf("source config assignment on line %d", lineNumber+1))
		}
		if len(entries) == maxSourceConfigRecords {
			return sourceConfigClaim{}, fail(CauseLimit, "source config exceeds record limit")
		}
		if _, duplicate := seen[entry.key]; duplicate {
			return sourceConfigClaim{}, fail(CauseMalformed, "source config contains a duplicate normalized key")
		}
		seen[entry.key] = struct{}{}
		entries = append(entries, entry)
	}

	claim, err := validateSourceConfigEntries(entries)
	if err != nil {
		return sourceConfigClaim{}, err
	}
	claim.entries = append([]sourceConfigEntry(nil), entries...)
	return claim, nil
}

func validateSourceConfigLineBytes(line []byte) error {
	for _, value := range line {
		if value == '\t' {
			continue
		}
		if value < 0x20 || value == 0x7f {
			return fmt.Errorf("forbidden control byte 0x%02x", value)
		}
	}
	return nil
}

func parseSourceConfigSection(line string) (sourceConfigSection, error) {
	closing, err := sourceConfigSectionClosing(line)
	if err != nil {
		return sourceConfigSection{}, err
	}
	tail := trimHorizontal(line[closing+1:])
	if tail != "" && tail[0] != '#' && tail[0] != ';' {
		return sourceConfigSection{}, fmt.Errorf("unexpected bytes after section header")
	}
	return parseSourceConfigSectionBody(trimHorizontal(line[1:closing]))
}

func sourceConfigSectionClosing(line string) (int, error) {
	quoted := false
	escaped := false
	for index := 1; index < len(line); index++ {
		value := line[index]
		if escaped {
			escaped = false
			continue
		}
		if quoted && value == '\\' {
			escaped = true
			continue
		}
		if value == '"' {
			quoted = !quoted
			continue
		}
		if value == ']' && !quoted {
			return index, nil
		}
	}
	return 0, fmt.Errorf("unterminated section header")
}

func parseSourceConfigSectionBody(body string) (sourceConfigSection, error) {
	if body == "" {
		return sourceConfigSection{}, fmt.Errorf("empty section")
	}
	nameEnd := 0
	for nameEnd < len(body) && isConfigNameByte(body[nameEnd]) {
		nameEnd++
	}
	if nameEnd == 0 || !isASCIIAlpha(body[0]) {
		return sourceConfigSection{}, fmt.Errorf("invalid section name")
	}
	section := sourceConfigSection{name: asciiLower(body[:nameEnd])}
	remainder := body[nameEnd:]
	if remainder == "" {
		return section, nil
	}
	if remainder[0] != ' ' && remainder[0] != '\t' {
		return sourceConfigSection{}, fmt.Errorf("invalid section separator")
	}
	remainder = trimHorizontalLeft(remainder)
	if len(remainder) < 2 || remainder[0] != '"' {
		return sourceConfigSection{}, fmt.Errorf("subsection must be quoted")
	}
	subsection, consumed, err := parseConfigQuoted(remainder)
	if err != nil {
		return sourceConfigSection{}, err
	}
	if trimHorizontal(remainder[consumed:]) != "" {
		return sourceConfigSection{}, fmt.Errorf("unexpected bytes after subsection")
	}
	if !controlFree(subsection) {
		return sourceConfigSection{}, fmt.Errorf("subsection contains a control byte")
	}
	section.subsection = subsection
	section.hasSubsection = true
	return section, nil
}

func parseConfigQuoted(input string) (string, int, error) {
	if input == "" || input[0] != '"' {
		return "", 0, fmt.Errorf("quoted value is missing opening quote")
	}
	decoded := make([]byte, 0, len(input))
	for index := 1; index < len(input); index++ {
		switch input[index] {
		case '"':
			return string(decoded), index + 1, nil
		case '\\':
			index++
			if index >= len(input) {
				return "", 0, fmt.Errorf("unterminated quoted escape")
			}
			escaped, ok := decodeConfigEscape(input[index])
			if !ok {
				return "", 0, fmt.Errorf("unsupported quoted escape")
			}
			decoded = append(decoded, escaped)
		default:
			decoded = append(decoded, input[index])
		}
	}
	return "", 0, fmt.Errorf("unterminated quoted value")
}

func parseSourceConfigAssignment(section sourceConfigSection, line string) (sourceConfigEntry, error) {
	variableEnd := 0
	for variableEnd < len(line) && isConfigNameByte(line[variableEnd]) {
		variableEnd++
	}
	if variableEnd == 0 || !isASCIIAlpha(line[0]) {
		return sourceConfigEntry{}, fmt.Errorf("invalid variable name")
	}
	remainder := trimHorizontalLeft(line[variableEnd:])
	if remainder == "" || remainder[0] != '=' {
		return sourceConfigEntry{}, fmt.Errorf("assignment requires an explicit value")
	}
	value, err := parseSourceConfigValue(remainder[1:])
	if err != nil {
		return sourceConfigEntry{}, err
	}
	return sourceConfigEntry{
		key: sourceConfigKey{
			section:       section.name,
			subsection:    section.subsection,
			hasSubsection: section.hasSubsection,
			variable:      asciiLower(line[:variableEnd]),
		},
		value: value,
	}, nil
}

func parseSourceConfigValue(input string) (string, error) {
	input = trimHorizontalLeft(input)
	decoded := make([]byte, 0, len(input))
	quoted := false
	lastRetained := 0
	for index := 0; index < len(input); index++ {
		value := input[index]
		if !quoted && (value == '#' || value == ';') {
			break
		}
		switch value {
		case '"':
			quoted = !quoted
			if quoted {
				lastRetained = len(decoded)
			}
		case '\\':
			escaped, next, err := sourceConfigEscapedByte(input, index)
			if err != nil {
				return "", err
			}
			index = next
			decoded = append(decoded, escaped)
			lastRetained = len(decoded)
		default:
			decoded = append(decoded, value)
			if quoted || (value != ' ' && value != '\t') {
				lastRetained = len(decoded)
			}
		}
	}
	if quoted {
		return "", fmt.Errorf("unterminated quoted value")
	}
	decoded = decoded[:lastRetained]
	if !controlFree(string(decoded)) {
		return "", fmt.Errorf("value contains a control byte")
	}
	return string(decoded), nil
}

func sourceConfigEscapedByte(input string, slash int) (byte, int, error) {
	next := slash + 1
	if next >= len(input) {
		return 0, 0, fmt.Errorf("unterminated value escape")
	}
	escaped, ok := decodeConfigEscape(input[next])
	if !ok || escaped < 0x20 || escaped == 0x7f {
		return 0, 0, fmt.Errorf("unsupported or control-producing value escape")
	}
	return escaped, next, nil
}

func decodeConfigEscape(value byte) (byte, bool) {
	switch value {
	case '\\', '"':
		return value, true
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'b':
		return '\b', true
	default:
		return 0, false
	}
}

func validateSourceConfigEntries(entries []sourceConfigEntry) (sourceConfigClaim, error) {
	validation := sourceConfigValidation{
		remotes: make(map[string]sourceRemoteState),
	}
	for _, entry := range entries {
		if err := validation.accept(entry); err != nil {
			return sourceConfigClaim{}, err
		}
	}
	return validation.finish()
}

type sourceRemoteState struct {
	url   bool
	fetch bool
}

type sourceConfigValidation struct {
	claim             sourceConfigClaim
	repositoryVersion string
	bareSeen          bool
	objectFormat      string
	refStorage        string
	remotes           map[string]sourceRemoteState
	branchRemotes     []string
}

func (validation *sourceConfigValidation) accept(entry sourceConfigEntry) error {
	key := entry.key
	if len(key.subsection) > maxRelativePathBytes || len(entry.value) > maxSourceConfigBytes {
		return fail(CauseLimit, "source config field exceeds its bound")
	}
	switch key.section {
	case "core":
		return validation.acceptCore(entry)
	case "extensions":
		return validation.acceptExtension(entry)
	case "user":
		return acceptSourceMetadata(entry, "name", "email")
	case "beads":
		return acceptSourceMetadata(entry, "role")
	case "remote":
		return validation.acceptRemote(entry)
	case "branch":
		return validation.acceptBranch(entry)
	default:
		return unsupportedSourceConfigKey(key)
	}
}

func (validation *sourceConfigValidation) acceptCore(entry sourceConfigEntry) error {
	if entry.key.hasSubsection {
		return unsupportedSourceConfigKey(entry.key)
	}
	switch entry.key.variable {
	case "repositoryformatversion":
		if entry.value != "0" && entry.value != "1" {
			return fail(CauseMalformed, "invalid repository format version")
		}
		validation.repositoryVersion = entry.value
	case "bare":
		if entry.value != "false" {
			return fail(CauseUnsupported, "source repository is bare")
		}
		validation.bareSeen = true
	case "filemode", "logallrefupdates", "ignorecase", "precomposeunicode":
		if !canonicalBoolean(entry.value) {
			return fail(CauseMalformed, "source config Boolean is not canonical")
		}
	case "hookspath":
		if !controlFree(entry.value) {
			return fail(CauseMalformed, "source hooks path contains a control byte")
		}
	default:
		return unsupportedSourceConfigKey(entry.key)
	}
	return nil
}

func (validation *sourceConfigValidation) acceptExtension(entry sourceConfigEntry) error {
	if entry.key.hasSubsection {
		return unsupportedSourceConfigKey(entry.key)
	}
	switch entry.key.variable {
	case "objectformat":
		if entry.value != "sha256" {
			return fail(CauseUnsupported, "unsupported object format")
		}
		validation.objectFormat = entry.value
	case "refstorage":
		if entry.value != "files" {
			return fail(CauseUnsupported, "unsupported ref storage")
		}
		validation.refStorage = entry.value
	case "worktreeconfig":
		if !canonicalBoolean(entry.value) {
			return fail(CauseMalformed, "worktreeConfig Boolean is not canonical")
		}
		validation.claim.worktreeConfigPresent = true
	default:
		return unsupportedSourceConfigKey(entry.key)
	}
	return nil
}

func acceptSourceMetadata(entry sourceConfigEntry, variables ...string) error {
	if entry.key.hasSubsection || !controlFree(entry.value) {
		return unsupportedSourceConfigKey(entry.key)
	}
	if slices.Contains(variables, entry.key.variable) {
		return nil
	}
	return unsupportedSourceConfigKey(entry.key)
}

func (validation *sourceConfigValidation) acceptRemote(entry sourceConfigEntry) error {
	key := entry.key
	if !key.hasSubsection || key.subsection == "" || !controlFree(key.subsection) {
		return fail(CauseMalformed, "remote requires a bounded subsection")
	}
	state := validation.remotes[key.subsection]
	switch key.variable {
	case "url":
		if entry.value == "" || !controlFree(entry.value) {
			return fail(CauseMalformed, "remote URL is empty or contains a control byte")
		}
		if commandStyleRemoteURL(entry.value) {
			return fail(CauseUnsupported, "remote helper command URL is forbidden")
		}
		state.url = true
	case "fetch":
		want := "+refs/heads/*:refs/remotes/" + key.subsection + "/*"
		if entry.value != want {
			return fail(CauseUnsupported, "remote fetch refspec is outside the closed form")
		}
		state.fetch = true
	default:
		return unsupportedSourceConfigKey(key)
	}
	validation.remotes[key.subsection] = state
	return nil
}

func (validation *sourceConfigValidation) acceptBranch(entry sourceConfigEntry) error {
	key := entry.key
	if !key.hasSubsection || key.subsection == "" || !controlFree(key.subsection) {
		return fail(CauseMalformed, "branch requires a bounded subsection")
	}
	switch key.variable {
	case "remote":
		if entry.value == "" || !controlFree(entry.value) {
			return fail(CauseMalformed, "branch remote is empty or contains a control byte")
		}
		validation.branchRemotes = append(validation.branchRemotes, entry.value)
	case "merge":
		if !strings.HasPrefix(entry.value, "refs/heads/") || !validFullGitRefName(entry.value) {
			return fail(CauseMalformed, "branch merge is not a full heads ref")
		}
	case "vscode-merge-base":
		if entry.value == "" || len(entry.value) > maxRelativePathBytes || !controlFree(entry.value) {
			return fail(CauseMalformed, "branch remote-tracking name is invalid")
		}
	default:
		return unsupportedSourceConfigKey(key)
	}
	return nil
}

func (validation *sourceConfigValidation) finish() (sourceConfigClaim, error) {
	if validation.repositoryVersion == "" || !validation.bareSeen {
		return sourceConfigClaim{}, fail(CauseMalformed, "source config omits a required core key")
	}
	switch validation.repositoryVersion {
	case "0":
		if validation.objectFormat != "" || validation.refStorage != "" {
			return sourceConfigClaim{}, fail(CauseUnsupported, "format version zero contains a format extension")
		}
		validation.claim.objectFormat = objectFormatSHA1
	case "1":
		if validation.objectFormat != "sha256" {
			return sourceConfigClaim{}, fail(CauseUnsupported, "format version one is not the closed SHA-256 form")
		}
		validation.claim.objectFormat = objectFormatSHA256
	}
	for name, state := range validation.remotes {
		if !state.url || !state.fetch {
			return sourceConfigClaim{}, fail(CauseMalformed, "remote "+name+" is incomplete")
		}
	}
	for _, name := range validation.branchRemotes {
		if _, ok := validation.remotes[name]; !ok {
			return sourceConfigClaim{}, fail(CauseMalformed, "branch names an undeclared remote")
		}
	}
	return validation.claim, nil
}

func unsupportedSourceConfigKey(key sourceConfigKey) error {
	return fail(CauseUnsupported, "source config key is outside the closed table: "+key.normalizedName())
}

func canonicalBoolean(value string) bool { return value == "true" || value == "false" }

func commandStyleRemoteURL(value string) bool {
	separator := strings.Index(value, "::")
	if separator <= 0 {
		return false
	}
	transport := value[:separator]
	if !isASCIIAlpha(transport[0]) {
		return false
	}
	for index := 1; index < len(transport); index++ {
		value := transport[index]
		if !isASCIIAlpha(value) && !isASCIIDigit(value) && value != '+' && value != '.' && value != '-' {
			return false
		}
	}
	return true
}

func isGitAdministrativeLockBasename(name string) bool {
	return name == ".lock" || strings.HasSuffix(name, ".lock")
}

func validFullGitRefName(name string) bool {
	if !strings.HasPrefix(name, "refs/") || len(name) <= len("refs/") || len(name) > maxRelativePathBytes {
		return false
	}
	if strings.Contains(name, "..") || strings.Contains(name, "//") || strings.Contains(name, "@{") {
		return false
	}
	if slices.ContainsFunc([]byte(name), invalidGitRefByte) {
		return false
	}
	for component := range strings.SplitSeq(name, "/") {
		if invalidGitRefComponent(component) {
			return false
		}
	}
	return true
}

func invalidGitRefByte(value byte) bool {
	return value < 0x20 || value == 0x7f || value == ' ' || strings.ContainsRune("~^:?*[\\", rune(value))
}

func invalidGitRefComponent(component string) bool {
	return component == "" || component == "." || component == ".." || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".") || strings.HasSuffix(component, ".lock")
}

type packedRefRecord struct {
	objectID string
	name     string
	peeledID string
}

type packedRefsClaim struct {
	traits  []string
	records []packedRefRecord
}

type packedRefsLimits struct {
	bytes int
	rows  int
}

func parsePackedRefs(content []byte, format repositoryObjectFormat) (packedRefsClaim, error) {
	return parsePackedRefsWithLimits(content, format, packedRefsLimits{
		bytes: maxPackedRefsBytes,
		rows:  maxPackedRefsRows,
	})
}

func parsePackedRefsWithLimits(content []byte, format repositoryObjectFormat, limits packedRefsLimits) (packedRefsClaim, error) {
	rows, err := splitPackedRefsRows(content, format, limits)
	if err != nil || len(rows) == 0 {
		return packedRefsClaim{}, err
	}
	parser := packedRefsParser{
		format: format,
		claim:  packedRefsClaim{records: make([]packedRefRecord, 0, len(rows))},
	}
	if bytes.HasPrefix(rows[0], []byte("#")) {
		parser.claim.traits, err = parsePackedRefsHeader(string(rows[0]))
		if err != nil {
			return packedRefsClaim{}, err
		}
		rows = rows[1:]
	}
	for _, row := range rows {
		if err := parser.accept(row); err != nil {
			return packedRefsClaim{}, err
		}
	}
	return parser.claim, nil
}

func splitPackedRefsRows(content []byte, format repositoryObjectFormat, limits packedRefsLimits) ([][]byte, error) {
	if format.hexWidth() == 0 {
		return nil, fail(CauseInternalInvariant, "unknown packed-refs object format")
	}
	if limits.bytes < 0 || limits.rows < 0 {
		return nil, fail(CauseInternalInvariant, "invalid packed-refs parser limits")
	}
	if len(content) > limits.bytes {
		return nil, fail(CauseLimit, "packed-refs exceeds byte limit")
	}
	if len(content) == 0 {
		return nil, nil
	}
	if content[len(content)-1] != '\n' {
		return nil, fail(CauseMalformed, "packed-refs row is not LF-terminated")
	}
	rows := bytes.Split(content, []byte{'\n'})
	rows = rows[:len(rows)-1]
	if len(rows) > limits.rows {
		return nil, fail(CauseLimit, "packed-refs exceeds row limit")
	}
	return rows, nil
}

type packedRefsParser struct {
	format repositoryObjectFormat
	claim  packedRefsClaim
}

func (parser *packedRefsParser) accept(row []byte) error {
	if len(row) == 0 {
		return fail(CauseMalformed, "packed-refs contains a blank row")
	}
	if bytes.IndexByte(row, 0) >= 0 || bytes.IndexByte(row, '\r') >= 0 {
		return fail(CauseMalformed, "packed-refs contains NUL or CR")
	}
	if row[0] == '^' {
		return parser.acceptPeeled(row[1:])
	}
	if row[0] == '#' {
		return fail(CauseMalformed, "packed-refs contains a nonleading or unknown header")
	}
	record, err := parsePackedRefRecord(row, parser.format)
	if err != nil {
		return err
	}
	if len(parser.claim.records) != 0 && record.name <= parser.claim.records[len(parser.claim.records)-1].name {
		return fail(CauseMalformed, "packed-refs names are not raw-byte strictly increasing")
	}
	parser.claim.records = append(parser.claim.records, record)
	return nil
}

func (parser *packedRefsParser) acceptPeeled(raw []byte) error {
	if len(parser.claim.records) == 0 || parser.claim.records[len(parser.claim.records)-1].peeledID != "" {
		return fail(CauseMalformed, "packed-refs contains a detached or duplicate peeled row")
	}
	peeledID := string(raw)
	if !validObjectID(peeledID, parser.format) {
		return fail(CauseMalformed, "packed-refs contains a malformed peeled object ID")
	}
	parser.claim.records[len(parser.claim.records)-1].peeledID = peeledID
	return nil
}

func parsePackedRefRecord(row []byte, format repositoryObjectFormat) (packedRefRecord, error) {
	space := bytes.IndexByte(row, ' ')
	if space <= 0 || bytes.LastIndexByte(row, ' ') != space || space == len(row)-1 {
		return packedRefRecord{}, fail(CauseMalformed, "packed-refs record framing is malformed")
	}
	record := packedRefRecord{objectID: string(row[:space]), name: string(row[space+1:])}
	if !validObjectID(record.objectID, format) {
		return packedRefRecord{}, fail(CauseMalformed, "packed-refs contains a malformed object ID")
	}
	if !validFullGitRefName(record.name) {
		return packedRefRecord{}, fail(CauseMalformed, "packed-refs contains an invalid ref name")
	}
	if record.name == "refs/replace" || strings.HasPrefix(record.name, "refs/replace/") {
		return packedRefRecord{}, fail(CauseUnsupported, "packed-refs contains a replacement ref")
	}
	return record, nil
}

func parsePackedRefsHeader(row string) ([]string, error) {
	const prefix = "# pack-refs with: "
	if !strings.HasPrefix(row, prefix) {
		return nil, fail(CauseMalformed, "packed-refs contains an unknown header")
	}
	remainder := strings.TrimPrefix(row, prefix)
	if remainder == "" || !strings.HasSuffix(remainder, " ") {
		return nil, fail(CauseMalformed, "packed-refs trait lacks its trailing space")
	}
	remainder = strings.TrimSuffix(remainder, " ")
	traits := strings.Split(remainder, " ")
	order := map[string]int{"peeled": 0, "fully-peeled": 1, "sorted": 2}
	previous := -1
	for _, trait := range traits {
		rank, ok := order[trait]
		if !ok || rank <= previous {
			return nil, fail(CauseMalformed, "packed-refs traits are unknown, duplicated, or out of order")
		}
		previous = rank
	}
	return append([]string(nil), traits...), nil
}

func validObjectID(value string, format repositoryObjectFormat) bool {
	if len(value) != format.hexWidth() {
		return false
	}
	for _, current := range []byte(value) {
		if !isASCIIDigit(current) && (current < 'a' || current > 'f') {
			return false
		}
	}
	return true
}

type configTranscriptRow struct {
	key   string
	value string
}

func sourceGitCommandOverrideRows() [11]configTranscriptRow {
	return [...]configTranscriptRow{
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
}

func (claim sourceConfigClaim) localTranscript() []byte {
	var transcript bytes.Buffer
	for _, entry := range claim.entries {
		appendConfigTranscriptRow(&transcript, "local", "file:.git/config", entry.key.normalizedName(), entry.value)
	}
	return transcript.Bytes()
}

func (claim sourceConfigClaim) activeTranscript() []byte {
	transcript := bytes.NewBuffer(claim.localTranscript())
	for _, row := range sourceGitCommandOverrideRows() {
		appendConfigTranscriptRow(transcript, "command", "command line:", row.key, row.value)
	}
	return transcript.Bytes()
}

func appendConfigTranscriptRow(destination *bytes.Buffer, scope, origin, key, value string) {
	destination.WriteString(scope)
	destination.WriteByte(0)
	destination.WriteString(origin)
	destination.WriteByte(0)
	destination.WriteString(key)
	destination.WriteByte('\n')
	destination.WriteString(value)
	destination.WriteByte(0)
}

func parseRequestedRevision(revision string, format repositoryObjectFormat) (string, error) {
	width := format.hexWidth()
	if width == 0 {
		return "", fail(CauseInternalInvariant, "unknown repository object format")
	}
	if len(revision) != width {
		return "", fail(CauseInvalidRequest, "revision has the wrong object-ID width")
	}
	if !validObjectID(revision, format) {
		return "", fail(CauseInvalidRequest, "revision is not lowercase hexadecimal")
	}
	return revision, nil
}

type privateInitBooleans struct {
	fileMode          bool
	symlinks          bool
	ignoreCase        bool
	precomposeUnicode bool
}

func renderPrivateConfig(format repositoryObjectFormat, taskPath string, init privateInitBooleans) ([]byte, error) {
	if format != objectFormatSHA1 && format != objectFormatSHA256 {
		return nil, fail(CauseInternalInvariant, "unknown private repository object format")
	}
	if taskPath == "" || !filepath.IsAbs(taskPath) || filepath.Clean(taskPath) != taskPath || taskPath == string(filepath.Separator) {
		return nil, fail(CauseInternalInvariant, "task path is not a canonical absolute path")
	}
	hooksPath, err := quoteGitConfigPath(taskPath + "/git-hooks")
	if err != nil {
		return nil, err
	}
	attributesPath, err := quoteGitConfigPath(taskPath + "/git-attributes")
	if err != nil {
		return nil, err
	}
	excludesPath, err := quoteGitConfigPath(taskPath + "/git-excludes")
	if err != nil {
		return nil, err
	}

	version := "0"
	if format == objectFormatSHA256 {
		version = "1"
	}
	var config strings.Builder
	fmt.Fprintf(&config, "[core]\n\trepositoryformatversion = %s\n", version)
	fmt.Fprintf(&config, "\tfilemode = %t\n", init.fileMode)
	config.WriteString("\tbare = false\n\tlogallrefupdates = false\n")
	fmt.Fprintf(&config, "\tsymlinks = %t\n", init.symlinks)
	fmt.Fprintf(&config, "\tignorecase = %t\n", init.ignoreCase)
	fmt.Fprintf(&config, "\tprecomposeunicode = %t\n", init.precomposeUnicode)
	fmt.Fprintf(&config, "\thooksPath = %s\n", hooksPath)
	fmt.Fprintf(&config, "\tattributesFile = %s\n", attributesPath)
	fmt.Fprintf(&config, "\texcludesFile = %s\n", excludesPath)
	config.WriteString("\tautocrlf = false\n\teol = lf\n\tfsmonitor = false\n\tcommitGraph = false\n\tmultiPackIndex = false\n\tuseReplaceRefs = false\n")
	if format == objectFormatSHA256 {
		config.WriteString("[extensions]\n\tobjectFormat = sha256\n")
	}
	config.WriteString("[checkout]\n\tworkers = 1\n[maintenance]\n\tauto = false\n[gc]\n\tauto = 0\n[pack]\n\treadReverseIndex = false\n\tuseBitmaps = false\n[fetch]\n\twriteCommitGraph = false\n[protocol]\n\tallow = never\n[commit]\n\tgpgSign = false\n[tag]\n\tgpgSign = false\n[log]\n\tshowSignature = false\n[merge]\n\tverifySignatures = false\n[submodule]\n\trecurse = false\n")
	return []byte(config.String()), nil
}

func validatePrivateConfig(content []byte, format repositoryObjectFormat, taskPath string, init privateInitBooleans) error {
	expected, err := renderPrivateConfig(format, taskPath, init)
	if err != nil {
		return err
	}
	if !bytes.Equal(content, expected) {
		return fail(CauseUnstable, "private config does not equal the closed semantic form")
	}
	parsed, err := parsePrivateConfigRows(content)
	if err != nil {
		return failWith(err, CauseInternalInvariant, "byte-exact private config failed semantic reparse")
	}
	want := expectedPrivateConfigRows(format, taskPath, init)
	if !slices.Equal(parsed, want) {
		return fail(CauseInternalInvariant, "private config semantic vector differs from the closed form")
	}
	return nil
}

func parsePrivateConfigRows(content []byte) ([]configTranscriptRow, error) {
	if len(content) == 0 || content[len(content)-1] != '\n' {
		return nil, fmt.Errorf("private config lacks its final LF")
	}
	lines := strings.Split(string(content[:len(content)-1]), "\n")
	rows := make([]configTranscriptRow, 0, len(lines))
	section := ""
	for _, line := range lines {
		if line == "" || line[0] == '#' || line[0] == ';' {
			return nil, fmt.Errorf("private config contains a blank or comment")
		}
		if line[0] == '[' {
			parsed, err := parseSourceConfigSection(line)
			if err != nil || parsed.hasSubsection {
				return nil, fmt.Errorf("private config section is malformed")
			}
			section = parsed.name
			continue
		}
		if section == "" || !strings.HasPrefix(line, "\t") {
			return nil, fmt.Errorf("private config assignment is outside a section")
		}
		assignment := strings.SplitN(strings.TrimPrefix(line, "\t"), " = ", 2)
		if len(assignment) != 2 || assignment[0] == "" {
			return nil, fmt.Errorf("private config assignment framing is malformed")
		}
		value, err := parsePrivateConfigValue(assignment[1])
		if err != nil {
			return nil, err
		}
		rows = append(rows, configTranscriptRow{
			key:   section + "." + asciiLower(assignment[0]),
			value: value,
		})
	}
	return rows, nil
}

func parsePrivateConfigValue(value string) (string, error) {
	if value == "" || value[0] != '"' {
		if !controlFree(value) || strings.ContainsAny(value, "\\\"") {
			return "", fmt.Errorf("private config literal value is malformed")
		}
		return value, nil
	}
	decoded, consumed, err := parseConfigQuoted(value)
	if err != nil || consumed != len(value) {
		return "", fmt.Errorf("private config quoted value is malformed")
	}
	return decoded, nil
}

func expectedPrivateConfigRows(format repositoryObjectFormat, taskPath string, init privateInitBooleans) []configTranscriptRow {
	version := "0"
	if format == objectFormatSHA256 {
		version = "1"
	}
	rows := []configTranscriptRow{
		{key: "core.repositoryformatversion", value: version},
		{key: "core.filemode", value: fmt.Sprintf("%t", init.fileMode)},
		{key: "core.bare", value: "false"},
		{key: "core.logallrefupdates", value: "false"},
		{key: "core.symlinks", value: fmt.Sprintf("%t", init.symlinks)},
		{key: "core.ignorecase", value: fmt.Sprintf("%t", init.ignoreCase)},
		{key: "core.precomposeunicode", value: fmt.Sprintf("%t", init.precomposeUnicode)},
		{key: "core.hookspath", value: taskPath + "/git-hooks"},
		{key: "core.attributesfile", value: taskPath + "/git-attributes"},
		{key: "core.excludesfile", value: taskPath + "/git-excludes"},
		{key: "core.autocrlf", value: "false"},
		{key: "core.eol", value: "lf"},
		{key: "core.fsmonitor", value: "false"},
		{key: "core.commitgraph", value: "false"},
		{key: "core.multipackindex", value: "false"},
		{key: "core.usereplacerefs", value: "false"},
	}
	if format == objectFormatSHA256 {
		rows = append(rows, configTranscriptRow{key: "extensions.objectformat", value: "sha256"})
	}
	return append(rows,
		configTranscriptRow{key: "checkout.workers", value: "1"},
		configTranscriptRow{key: "maintenance.auto", value: "false"},
		configTranscriptRow{key: "gc.auto", value: "0"},
		configTranscriptRow{key: "pack.readreverseindex", value: "false"},
		configTranscriptRow{key: "pack.usebitmaps", value: "false"},
		configTranscriptRow{key: "fetch.writecommitgraph", value: "false"},
		configTranscriptRow{key: "protocol.allow", value: "never"},
		configTranscriptRow{key: "commit.gpgsign", value: "false"},
		configTranscriptRow{key: "tag.gpgsign", value: "false"},
		configTranscriptRow{key: "log.showsignature", value: "false"},
		configTranscriptRow{key: "merge.verifysignatures", value: "false"},
		configTranscriptRow{key: "submodule.recurse", value: "false"},
	)
}

func quoteGitConfigPath(value string) (string, error) {
	var quoted strings.Builder
	quoted.Grow(len(value) + 2)
	quoted.WriteByte('"')
	for _, current := range []byte(value) {
		switch current {
		case '\\':
			quoted.WriteString("\\\\")
		case '"':
			quoted.WriteString("\\\"")
		case '\n':
			quoted.WriteString("\\n")
		case '\t':
			quoted.WriteString("\\t")
		case '\b':
			quoted.WriteString("\\b")
		default:
			if current < 0x20 || current == 0x7f {
				return "", fail(CauseMalformed, "private config path contains an unsupported control byte")
			}
			quoted.WriteByte(current)
		}
	}
	quoted.WriteByte('"')
	return quoted.String(), nil
}

func controlFree(value string) bool {
	for _, current := range []byte(value) {
		if current < 0x20 || current == 0x7f {
			return false
		}
	}
	return true
}

func trimHorizontal(value string) string {
	return strings.Trim(value, " \t")
}

func trimHorizontalLeft(value string) string {
	return strings.TrimLeft(value, " \t")
}

func isConfigNameByte(value byte) bool {
	return isASCIIAlpha(value) || isASCIIDigit(value) || value == '-'
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isASCIIDigit(value byte) bool { return value >= '0' && value <= '9' }

func asciiLower(value string) string {
	lowered := []byte(value)
	for index, current := range lowered {
		if current >= 'A' && current <= 'Z' {
			lowered[index] = current + ('a' - 'A')
		}
	}
	return string(lowered)
}

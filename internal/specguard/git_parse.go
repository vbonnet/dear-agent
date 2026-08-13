package specguard

import (
	"bytes"
	"crypto/sha1" //nolint:gosec // Git SHA-1 object identity is verified, not used as a new security primitive.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

func parseIndexEntries(output []byte, limits guardLimits) ([]treeEntry, *guardFailure) {
	records, failure := nulRecords(output, limits.maxEntries, "index")
	if failure != nil {
		return nil, failure
	}
	entries := make([]treeEntry, 0, len(records))
	seen := make(map[string]bool)
	for _, record := range records {
		metadata, filePath, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fail("malformed-index", "", "Git index entry omitted its path separator")
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 3 || !validMode(fields[0]) || !validOID(fields[1]) {
			return nil, fail("malformed-index", "", "Git index entry contained invalid metadata")
		}
		stage, err := strconv.Atoi(fields[2])
		if err != nil || stage < 0 || stage > 3 {
			return nil, fail("malformed-index", "", "Git index entry contained an invalid merge stage")
		}
		pathText := string(filePath)
		if failure := validateGitPath(pathText, limits.maxPathBytes); failure != nil {
			return nil, failure
		}
		identity := fmt.Sprintf("%s\x00%d", pathText, stage)
		if seen[identity] {
			return nil, fail("malformed-index", pathText, "Git index contained a duplicate path and stage")
		}
		seen[identity] = true
		entries = append(entries, treeEntry{path: pathText, mode: fields[0], oid: fields[1], stage: stage})
	}
	return entries, nil
}

func parseTreeEntries(output []byte, limits guardLimits) ([]treeEntry, *guardFailure) {
	records, failure := nulRecords(output, limits.maxEntries, "tree")
	if failure != nil {
		return nil, failure
	}
	entries := make([]treeEntry, 0, len(records))
	seen := make(map[string]bool)
	for _, record := range records {
		metadata, filePath, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fail("malformed-tree", "", "Git tree entry omitted its path separator")
		}
		fields := strings.Fields(string(metadata))
		if len(fields) != 3 || !validMode(fields[0]) || !validOID(fields[2]) {
			return nil, fail("malformed-tree", "", "Git tree entry contained invalid metadata")
		}
		pathText := string(filePath)
		if failure := validateGitPath(pathText, limits.maxPathBytes); failure != nil {
			return nil, failure
		}
		if seen[pathText] {
			return nil, fail("malformed-tree", pathText, "Git tree contained a duplicate path")
		}
		seen[pathText] = true
		entries = append(entries, treeEntry{path: pathText, mode: fields[0], objectType: fields[1], oid: fields[2]})
	}
	return entries, nil
}

func parseNameStatus(output []byte, limits guardLimits) ([]change, *guardFailure) {
	records, failure := nulRecords(output, limits.maxChanged*2, "changed-path")
	if failure != nil {
		return nil, failure
	}
	if len(records)%2 != 0 {
		return nil, fail("malformed-diff", "", "Git name-status output was not paired")
	}
	changes := make([]change, 0, len(records)/2)
	seen := make(map[string]bool)
	for index := 0; index < len(records); index += 2 {
		status := string(records[index])
		filePath := string(records[index+1])
		if len(status) != 1 || !strings.ContainsRune("ADMT", rune(status[0])) {
			return nil, fail("malformed-diff", filePath, "Git reported an unsupported change status")
		}
		if failure := validateGitPath(filePath, limits.maxPathBytes); failure != nil {
			return nil, failure
		}
		if seen[filePath] {
			return nil, fail("malformed-diff", filePath, "Git reported a changed path more than once")
		}
		seen[filePath] = true
		changes = append(changes, change{path: filePath, status: status})
		if len(changes) > limits.maxChanged {
			return nil, fail("changed-file-limit", filePath, "changed path count exceeded the safety limit")
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].path < changes[j].path })
	return changes, nil
}

func parsePathList(output []byte, limits guardLimits) ([]string, *guardFailure) {
	records, failure := nulRecords(output, limits.maxEntries, "untracked-path")
	if failure != nil {
		return nil, failure
	}
	paths := make([]string, 0, len(records))
	seen := make(map[string]bool)
	for _, record := range records {
		filePath := string(record)
		if failure := validateGitPath(filePath, limits.maxPathBytes); failure != nil {
			return nil, failure
		}
		if seen[filePath] {
			return nil, fail("malformed-git-output", filePath, "Git path output contained a duplicate path")
		}
		seen[filePath] = true
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	return paths, nil
}

func parseIndexFlaggedPaths(output []byte, limits guardLimits) ([]string, *guardFailure) {
	records, failure := nulRecords(output, limits.maxEntries, "index-flag")
	if failure != nil {
		return nil, failure
	}
	flagged := make([]string, 0)
	seen := make(map[string]bool)
	for _, record := range records {
		if len(record) < 3 || record[1] != ' ' {
			return nil, fail("malformed-index-flags", "", "Git index flag output contained invalid framing")
		}
		tag := record[0]
		normalized := tag
		assumeUnchanged := tag >= 'a' && tag <= 'z'
		if assumeUnchanged {
			normalized -= 'a' - 'A'
		}
		if !strings.ContainsRune("HSMRCK?", rune(normalized)) {
			return nil, fail("malformed-index-flags", "", "Git index flag output contained an unsupported tag")
		}
		filePath := string(record[2:])
		if failure := validateGitPath(filePath, limits.maxPathBytes); failure != nil {
			return nil, failure
		}
		if seen[filePath] {
			return nil, fail("malformed-index-flags", filePath, "Git index flag output contained a duplicate path")
		}
		seen[filePath] = true
		if isGovernedPath(filePath) && (assumeUnchanged || normalized == 'S') {
			flagged = append(flagged, filePath)
		}
	}
	sort.Strings(flagged)
	return flagged, nil
}

//nolint:gocyclo // Linear batch framing and object verification intentionally remain in one parser.
func parseBatchBlobs(output []byte, expected []string, limits guardLimits) (map[string][]byte, *guardFailure) {
	reader := bytes.NewReader(output)
	bodies := make(map[string][]byte, len(expected))
	var corpusBytes int64
	for _, wantedOID := range expected {
		header, err := readBoundedLine(reader, 256)
		if err != nil {
			return nil, fail("malformed-git-object", "", "Git object batch returned a malformed header")
		}
		fields := strings.Fields(string(header))
		if len(fields) == 2 && fields[1] == "missing" {
			return nil, fail("missing-git-object", "", "a required Git blob is unavailable locally; lazy fetching is disabled")
		}
		if len(fields) != 3 || fields[0] != wantedOID || fields[1] != "blob" {
			return nil, fail("malformed-git-object", "", "Git object batch returned unexpected identity or type metadata")
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 {
			return nil, fail("malformed-git-object", "", "Git object batch returned an invalid blob size")
		}
		if size > limits.maxFileBytes {
			return nil, fail("file-size-limit", "", "a governed Git blob exceeded the per-file safety limit")
		}
		if size > limits.maxCorpusBytes-corpusBytes {
			return nil, fail("corpus-size-limit", "", "governed Git blobs exceeded the corpus safety limit")
		}
		body := make([]byte, size)
		if _, err := io.ReadFull(reader, body); err != nil {
			return nil, fail("malformed-git-object", "", "Git object batch ended before a blob body was complete")
		}
		separator, err := reader.ReadByte()
		if err != nil || separator != '\n' {
			return nil, fail("malformed-git-object", "", "Git object batch omitted a blob separator")
		}
		if !blobMatchesOID(body, wantedOID) {
			return nil, fail("git-object-identity", "", "Git returned blob bytes that do not match the requested object ID")
		}
		bodies[wantedOID] = body
		corpusBytes += size
	}
	if reader.Len() != 0 {
		return nil, fail("malformed-git-object", "", "Git object batch returned trailing data")
	}
	return bodies, nil
}

func readBoundedLine(reader *bytes.Reader, limit int) ([]byte, error) {
	line := make([]byte, 0, min(limit, reader.Len()))
	for len(line) <= limit {
		value, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if value == '\n' {
			return line, nil
		}
		line = append(line, value)
	}
	return nil, errors.New("line exceeded limit")
}

func blobMatchesOID(body []byte, oid string) bool {
	header := fmt.Appendf(nil, "blob %d\x00", len(body))
	switch len(oid) {
	case 40:
		hash := sha1.New() //nolint:gosec // Required to verify SHA-1 Git repositories.
		_, _ = hash.Write(header)
		_, _ = hash.Write(body)
		return hex.EncodeToString(hash.Sum(nil)) == oid
	case 64:
		hash := sha256.New()
		_, _ = hash.Write(header)
		_, _ = hash.Write(body)
		return hex.EncodeToString(hash.Sum(nil)) == oid
	default:
		return false
	}
}

func nulRecords(output []byte, limit int, label string) ([][]byte, *guardFailure) {
	if len(output) == 0 {
		return [][]byte{}, nil
	}
	if output[len(output)-1] != 0 {
		return nil, fail("malformed-git-output", "", fmt.Sprintf("Git %s output was not NUL terminated", label))
	}
	records := bytes.Split(output[:len(output)-1], []byte{0})
	if len(records) > limit {
		return nil, fail("git-entry-limit", "", fmt.Sprintf("Git %s output exceeded the entry safety limit", label))
	}
	for _, record := range records {
		if len(record) == 0 {
			return nil, fail("malformed-git-output", "", fmt.Sprintf("Git %s output contained an empty record", label))
		}
	}
	return records, nil
}

func validMode(mode string) bool {
	if len(mode) != 6 {
		return false
	}
	for _, value := range mode {
		if value < '0' || value > '7' {
			return false
		}
	}
	return true
}

func validOID(oid string) bool {
	if len(oid) != 40 && len(oid) != 64 {
		return false
	}
	_, err := hex.DecodeString(oid)
	return err == nil && strings.ToLower(oid) == oid
}

//nolint:gocyclo // The explicit allowlist keeps revision admission reviewable.
func validRevision(revision string, limit int) bool {
	if revision == "" || len(revision) > limit || strings.HasPrefix(revision, "-") ||
		strings.Contains(revision, "..") || strings.Contains(revision, "@{") ||
		strings.ContainsAny(revision, "\\\x00\r\n\t ~^:?*[\"") ||
		strings.HasSuffix(revision, "/") || strings.Contains(revision, "//") {
		return false
	}
	for _, value := range revision {
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') || strings.ContainsRune("._/-", value) {
			continue
		}
		return false
	}
	return true
}

// Package pisession owns Pi native session identity and transcript parsing.
package pisession

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	maxHeaderBytes = 1024 * 1024
	maxLineBytes   = 4 * 1024 * 1024
	maxImportBytes = 64 * 1024 * 1024
	maxCandidates  = 100000
)

// ErrTranscriptNotFound distinguishes absence from malformed candidates.
var ErrTranscriptNotFound = errors.New("pi transcript not found")

var idPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// Message is the lossless AGM projection of a Pi message entry's text.
type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// Metadata is the validated native identity carried by a Pi JSONL header.
type Metadata struct {
	ID  string
	CWD string
}

// Usage is Pi's native projection of the latest prompt context and cumulative
// provider-reported session cost.
type Usage struct {
	Model           string
	ContextTokens   int
	InputTokens     int64
	OutputTokens    int64
	CumulativeCost  float64
	AssistantCalls  int
	LastAssistantAt time.Time
}

type sessionHeader struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	CWD  string `json:"cwd"`
}

type sessionEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

type modelChangeEntry struct {
	Type     string `json:"type"`
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

type nativeMessage struct {
	Role     string          `json:"role"`
	Provider string          `json:"provider"`
	Model    string          `json:"model"`
	Content  json.RawMessage `json:"content"`
	Usage    nativeUsage     `json:"usage"`
}

type nativeUsage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
	Cost       struct {
		Total float64 `json:"total"`
	} `json:"cost"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ValidateID enforces Pi's native session-id grammar.
func ValidateID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid Pi session id %q", id)
	}
	if len(id) > 128 {
		return fmt.Errorf("pi session id exceeds 128 bytes")
	}
	return nil
}

// EnsureRoot resolves and protects AGM's private Pi session directory.
func EnsureRoot(root string) (string, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home for Pi sessions: %w", err)
		}
		root = filepath.Join(home, ".agm", "pi", "sessions")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve Pi session root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", fmt.Errorf("create Pi session root: %w", err)
	}
	if _, err := ValidateRoot(abs); err != nil {
		return "", err
	}
	// #nosec G302 -- an owner-only directory needs execute bits for traversal.
	if err := os.Chmod(abs, 0o700); err != nil {
		return "", fmt.Errorf("protect Pi session root: %w", err)
	}
	return abs, nil
}

// ValidateRoot requires an existing absolute directory and rejects a symlink
// at the trust boundary before discovery or resume uses it.
func ValidateRoot(root string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("pi session root must be absolute: %q", root)
	}
	abs := filepath.Clean(root)
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("stat Pi session root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("pi session root must be a non-symlink directory: %q", root)
	}
	return abs, nil
}

// ValidateCodingAgentDir resolves an explicitly configured Pi agent directory
// and rejects missing, symlink, or non-directory targets before the path is
// persisted or interpolated into a launch command.
// An empty value preserves Pi's native default configuration discovery.
func ValidateCodingAgentDir(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve Pi coding agent directory: %w", err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("stat Pi coding agent directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("pi coding agent directory must be a non-symlink directory: %q", root)
	}
	return abs, nil
}

func validateOwnedTranscript(sessionRoot, path string) (string, fs.FileInfo, error) {
	root, err := ValidateRoot(sessionRoot)
	if err != nil {
		return "", nil, err
	}
	if !filepath.IsAbs(path) {
		return "", nil, fmt.Errorf("pi transcript path must be absolute: %q", path)
	}
	clean := filepath.Clean(path)
	if filepath.Dir(clean) != root {
		return "", nil, fmt.Errorf("pi transcript path escapes managed session root: %q", path)
	}
	// #nosec G703 -- the exact path is constrained to the validated managed root above.
	info, err := os.Lstat(clean)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("pi transcript is not a regular file: %q", path)
	}
	return clean, info, nil
}

// TranscriptModTime returns the modification time of one managed transcript.
func TranscriptModTime(sessionRoot, path string) (time.Time, error) {
	_, info, err := validateOwnedTranscript(sessionRoot, path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// RemoveTranscript removes one regular transcript owned by the managed root.
func RemoveTranscript(sessionRoot, path string) error {
	clean, _, err := validateOwnedTranscript(sessionRoot, path)
	if err != nil {
		return err
	}
	// #nosec G703 -- validateOwnedTranscript constrains the exact regular file to the managed root.
	return os.Remove(clean)
}

// FindTranscript locates the one JSONL transcript with an exact native id.
//
//nolint:gocyclo // reason: bounded discovery keeps candidate validation and duplicate detection in one linear scan
func FindTranscript(sessionDir, sessionID string) (string, error) {
	if err := ValidateID(sessionID); err != nil {
		return "", err
	}
	root, err := ValidateRoot(sessionDir)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("read Pi session directory: %w", err)
	}
	var match string
	candidates := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			continue
		}
		candidates++
		if candidates > maxCandidates {
			return "", fmt.Errorf("pi session directory exceeds %d JSONL candidates", maxCandidates)
		}
		path := filepath.Join(root, entry.Name())
		header, headerErr := readHeader(path)
		if headerErr != nil || header.Type != "session" || header.ID != sessionID {
			continue
		}
		if match != "" {
			return "", fmt.Errorf("multiple Pi transcripts found for session %q", sessionID)
		}
		match = path
	}
	if match == "" {
		return "", fmt.Errorf("%w for session %q", ErrTranscriptNotFound, sessionID)
	}
	return match, nil
}

// FindTranscriptTree locates an exact native ID below Pi's project-grouped
// default session tree. It never follows symlinks and rejects duplicate IDs.
func FindTranscriptTree(sessionRoot, sessionID string) (string, error) {
	if err := ValidateID(sessionID); err != nil {
		return "", err
	}
	root, err := ValidateRoot(sessionRoot)
	if err != nil {
		return "", err
	}
	match := ""
	candidates := 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			return nil
		}
		candidates++
		if candidates > maxCandidates {
			return fmt.Errorf("pi session tree exceeds %d JSONL candidates", maxCandidates)
		}
		header, headerErr := readHeader(path)
		if headerErr != nil || header.Type != "session" || header.ID != sessionID {
			return nil //nolint:nilerr // malformed nonmatching candidates are skipped independently
		}
		if match != "" {
			return fmt.Errorf("multiple Pi transcripts found for session %q", sessionID)
		}
		match = path
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk Pi session tree: %w", err)
	}
	if match == "" {
		return "", fmt.Errorf("%w for session %q", ErrTranscriptNotFound, sessionID)
	}
	return match, nil
}

// ReadMetadata validates and returns a native transcript's session header.
func ReadMetadata(path string) (Metadata, error) {
	header, err := readHeader(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("read Pi session header: %w", err)
	}
	if header.Type != "session" {
		return Metadata{}, fmt.Errorf("pi transcript must begin with a session header")
	}
	if err := ValidateID(header.ID); err != nil {
		return Metadata{}, err
	}
	if !filepath.IsAbs(header.CWD) {
		return Metadata{}, fmt.Errorf("pi session cwd must be absolute: %q", header.CWD)
	}
	return Metadata{ID: header.ID, CWD: header.CWD}, nil
}

// ImportNativeFile bounds and imports one native transcript from disk.
func ImportNativeFile(sessionRoot, source string) (Metadata, string, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return Metadata{}, "", fmt.Errorf("stat Pi import: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxImportBytes {
		return Metadata{}, "", fmt.Errorf("pi import file must be regular and between 1 and %d bytes", maxImportBytes)
	}
	file, err := os.Open(source)
	if err != nil {
		return Metadata{}, "", fmt.Errorf("open Pi import: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil {
		return Metadata{}, "", fmt.Errorf("read Pi import: %w", err)
	}
	return ImportNative(sessionRoot, data)
}

// ImportNative validates Pi JSONL and atomically installs a private copy.
//
//nolint:gocyclo // reason: atomic import transaction keeps validation, duplicate rejection, sync, and rename ordering explicit
func ImportNative(sessionRoot string, data []byte) (Metadata, string, error) {
	if len(data) == 0 || len(data) > maxImportBytes {
		return Metadata{}, "", fmt.Errorf("pi import size must be between 1 and %d bytes", maxImportBytes)
	}
	line := data
	if before, _, found := bytes.Cut(data, []byte("\n")); found {
		line = before
	}
	if len(line) > maxHeaderBytes {
		return Metadata{}, "", fmt.Errorf("pi session header exceeds %d bytes", maxHeaderBytes)
	}
	var header sessionHeader
	if err := json.Unmarshal(line, &header); err != nil {
		return Metadata{}, "", fmt.Errorf("parse Pi session header: %w", err)
	}
	if header.Type != "session" {
		return Metadata{}, "", fmt.Errorf("pi native import must begin with a session header")
	}
	if err := ValidateID(header.ID); err != nil {
		return Metadata{}, "", err
	}
	if !filepath.IsAbs(header.CWD) {
		return Metadata{}, "", fmt.Errorf("pi session cwd must be absolute: %q", header.CWD)
	}
	if err := validateNativeJSONL(data); err != nil {
		return Metadata{}, "", err
	}
	root, err := EnsureRoot(sessionRoot)
	if err != nil {
		return Metadata{}, "", err
	}
	if existing, findErr := FindTranscript(root, header.ID); findErr == nil {
		return Metadata{}, "", fmt.Errorf("pi session %q already exists at %s", header.ID, existing)
	} else if !errors.Is(findErr, ErrTranscriptNotFound) {
		return Metadata{}, "", findErr
	}
	temp, err := os.CreateTemp(root, ".pi-import-*")
	if err != nil {
		return Metadata{}, "", fmt.Errorf("create Pi import temp file: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return Metadata{}, "", err
	}
	if _, err := temp.Write(data); err != nil {
		return Metadata{}, "", fmt.Errorf("write Pi import: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return Metadata{}, "", fmt.Errorf("sync Pi import: %w", err)
	}
	if err := temp.Close(); err != nil {
		return Metadata{}, "", fmt.Errorf("close Pi import: %w", err)
	}
	path := filepath.Join(root, time.Now().UTC().Format("2006-01-02T15-04-05-000Z")+"_"+header.ID+".jsonl")
	if err := os.Rename(tempPath, path); err != nil {
		return Metadata{}, "", fmt.Errorf("install Pi import: %w", err)
	}
	removeTemp = false
	return Metadata{ID: header.ID, CWD: header.CWD}, path, nil
}

func validateNativeJSONL(data []byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(line, &entry); err != nil || entry == nil {
			return fmt.Errorf("parse Pi JSONL line %d: invalid object", lineNumber)
		}
		var entryType string
		if rawType, ok := entry["type"]; !ok || json.Unmarshal(rawType, &entryType) != nil || entryType == "" {
			return fmt.Errorf("parse Pi JSONL line %d: missing string type", lineNumber)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("parse Pi JSONL: each line must be at most %d bytes: %w", maxLineBytes, err)
	}
	return nil
}

func readHeader(path string) (sessionHeader, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return sessionHeader{}, err
	}
	if !info.Mode().IsRegular() {
		return sessionHeader{}, fmt.Errorf("pi transcript is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return sessionHeader{}, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(io.LimitReader(file, maxHeaderBytes+1), 4096)
	line, err := reader.ReadString('\n')
	if errors.Is(err, io.EOF) && line != "" {
		err = nil
	}
	if err != nil {
		return sessionHeader{}, err
	}
	if len(line) > maxHeaderBytes {
		return sessionHeader{}, fmt.Errorf("pi session header exceeds %d bytes", maxHeaderBytes)
	}
	var header sessionHeader
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &header); err != nil {
		return sessionHeader{}, err
	}
	return header, nil
}

// ReadMessages projects text-bearing Pi message records from native JSONL.
func ReadMessages(path string) ([]Message, error) {
	if err := validateTranscriptFile(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open Pi transcript: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)
	var messages []Message
	for scanner.Scan() {
		var entry sessionEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "message" {
			continue
		}
		var native nativeMessage
		if json.Unmarshal(entry.Message, &native) != nil {
			continue
		}
		role := native.Role
		switch role {
		case "user", "assistant":
		case "toolResult":
			role = "tool"
		default:
			continue
		}
		content := textContent(native.Content)
		if content == "" {
			continue
		}
		timestamp, _ := time.Parse(time.RFC3339Nano, entry.Timestamp)
		messages = append(messages, Message{Role: role, Content: content, Timestamp: timestamp})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Pi transcript: %w", err)
	}
	return messages, nil
}

// ReadModel returns the latest provider-qualified model with native
// provenance. An empty result means the transcript does not establish one.
func ReadModel(path string) (string, error) {
	if err := validateTranscriptFile(path); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open Pi transcript: %w", err)
	}
	defer file.Close()

	model := ""
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)
	for scanner.Scan() {
		var change modelChangeEntry
		if json.Unmarshal(scanner.Bytes(), &change) == nil && change.Type == "model_change" && change.ModelID != "" {
			model = qualifyModel(change.Provider, change.ModelID)
			continue
		}
		var entry sessionEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "message" {
			continue
		}
		var message nativeMessage
		if json.Unmarshal(entry.Message, &message) == nil && message.Role == "assistant" && message.Model != "" {
			model = qualifyModel(message.Provider, message.Model)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read Pi transcript model: %w", err)
	}
	return model, nil
}

func qualifyModel(provider, model string) string {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return model
	}
	// Pi persists provider and model as separate native fields. Model IDs are
	// opaque and may themselves begin with the provider name, so an apparent
	// prefix match must not erase that boundary.
	return provider + "/" + model
}

// ReadUsage returns the latest valid assistant context usage and cumulative
// provider-reported cost from a bounded Pi transcript.
func ReadUsage(path string) (Usage, error) {
	if err := validateTranscriptFile(path); err != nil {
		return Usage{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Usage{}, fmt.Errorf("open Pi transcript: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)
	var out Usage
	for scanner.Scan() {
		var entry sessionEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "message" {
			continue
		}
		var message nativeMessage
		if json.Unmarshal(entry.Message, &message) != nil {
			continue
		}
		usage := message.Usage
		out.CumulativeCost += usage.Cost.Total
		out.InputTokens += int64(usage.Input + usage.CacheRead + usage.CacheWrite)
		out.OutputTokens += int64(usage.Output)
		if message.Role != "assistant" || usage.Input+usage.Output+usage.CacheRead+usage.CacheWrite == 0 {
			continue
		}
		out.Model = qualifyModel(message.Provider, message.Model)
		out.ContextTokens = usage.Input + usage.CacheRead + usage.CacheWrite
		out.AssistantCalls++
		out.LastAssistantAt, _ = time.Parse(time.RFC3339Nano, entry.Timestamp)
	}
	if err := scanner.Err(); err != nil {
		return Usage{}, fmt.Errorf("read Pi transcript usage: %w", err)
	}
	if out.AssistantCalls == 0 {
		return Usage{}, fmt.Errorf("pi transcript contains no assistant usage")
	}
	return out, nil
}

// ReadNative reads one bounded regular Pi transcript for native export.
func ReadNative(path string) ([]byte, error) {
	if err := validateTranscriptFile(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open Pi transcript: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Pi transcript: %w", err)
	}
	if len(data) > maxImportBytes {
		return nil, fmt.Errorf("pi transcript exceeds %d bytes", maxImportBytes)
	}
	return data, nil
}

func validateTranscriptFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat Pi transcript: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxImportBytes {
		return fmt.Errorf("pi transcript must be regular and between 1 and %d bytes", maxImportBytes)
	}
	return nil
}

func textContent(raw json.RawMessage) string {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain
	}
	var parts []contentPart
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var textParts []string
	for _, part := range parts {
		if part.Type == "text" && part.Text != "" {
			textParts = append(textParts, part.Text)
		}
	}
	return strings.Join(textParts, "\n")
}

// Package claudeui owns the on-disk layout of the Claude desktop / claude.ai/code
// session store and the safe, reversible mutation of its `isArchived` flag.
//
// This is the local per-session JSON store at
//
//	~/Library/Application Support/Claude/claude-code-sessions/<deviceId>/<accountId>/local_<id>.json
//
// It is distinct from the AGM Dolt session manifests (`agm session gc` /
// `agm session archive`) and from the `~/.claude/projects/*.jsonl` transcripts.
// Per CUI-03 and CUI-04 this package never deletes anything and never touches `.jsonl`
// transcripts; it only flips the boolean `isArchived` field in place.
//
// The on-disk files are compact, insertion-ordered JSON written by the desktop
// app. To minimise diff churn, avoid confusing the app's reconciler, and keep
// `--unarchive` byte-reversible, mutation is a surgical anchored replacement of
// the single `isArchived` token rather than a full re-serialisation (which
// would reorder keys and rewrite the whole document).
package claudeui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ErrUnknownSchema is returned when a session file does not match the shape we
// recognise. Per CUI-03 such files are refused, never rewritten.
var ErrUnknownSchema = errors.New("claudeui: unrecognized session schema")

// ErrAmbiguousStore is returned when the store contains more than one
// device/account directory and no explicit selector was given.
var ErrAmbiguousStore = errors.New("claudeui: ambiguous store (multiple device/account dirs)")

// ErrStoreNotFound is returned when the Claude session store does not exist.
var ErrStoreNotFound = errors.New("claudeui: session store not found")

// storeSubpath is the store location relative to the user's home directory.
var storeSubpath = filepath.Join(
	"Library", "Application Support", "Claude", "claude-code-sessions",
)

// DefaultStoreRoot returns the absolute path of the Claude session store root
// for the given home directory.
func DefaultStoreRoot(home string) string {
	return filepath.Join(home, storeSubpath)
}

// Session is a single desktop/claude.ai-code session file. Only the fields we
// reason about are typed; every other field in the file is preserved verbatim
// on write because mutation is a surgical edit of the raw bytes.
type Session struct {
	Path           string `json:"-"`
	DeviceID       string `json:"-"`
	AccountID      string `json:"-"`
	SessionID      string `json:"sessionId"`
	CliSessionID   string `json:"cliSessionId"`
	Cwd            string `json:"cwd"`
	OriginCwd      string `json:"originCwd"`
	Title          string `json:"title"`
	Model          string `json:"model"`
	CreatedAt      int64  `json:"createdAt"`
	LastActivityAt int64  `json:"lastActivityAt"` // epoch ms — the age signal
	IsArchived     bool   `json:"isArchived"`

	raw []byte // original file bytes, used for the surgical write
}

// schemaProbe is used to validate that a file has the load-bearing fields with
// the expected JSON kinds. Pointer fields distinguish "absent" from "zero".
type schemaProbe struct {
	SessionID      *string  `json:"sessionId"`
	CliSessionID   string   `json:"cliSessionId"`
	Cwd            string   `json:"cwd"`
	OriginCwd      string   `json:"originCwd"`
	Title          string   `json:"title"`
	Model          string   `json:"model"`
	CreatedAt      float64  `json:"createdAt"`
	LastActivityAt *float64 `json:"lastActivityAt"`
	IsArchived     *bool    `json:"isArchived"`
}

// StoreDir locates the single <deviceId>/<accountId> directory under root.
//
// device and account are optional selectors; an empty string means
// "autodetect, and error if ambiguous". This keeps multi-device/account stores
// safe by refusing rather than guessing (CUI-02 and CUI-09).
func StoreDir(root, device, account string) (dir, deviceID, accountID string, err error) {
	if _, statErr := os.Stat(root); statErr != nil {
		if os.IsNotExist(statErr) {
			return "", "", "", fmt.Errorf("%w: %s", ErrStoreNotFound, root)
		}
		return "", "", "", statErr
	}

	deviceID, err = pickSubdir(root, device, "device")
	if err != nil {
		return "", "", "", err
	}
	deviceDir := filepath.Join(root, deviceID)

	accountID, err = pickSubdir(deviceDir, account, "account")
	if err != nil {
		return "", "", "", err
	}

	return filepath.Join(deviceDir, accountID), deviceID, accountID, nil
}

// pickSubdir returns the requested subdirectory of parent, or the only one if
// want is empty. It errors if want is missing, or if want is empty and the
// choice is ambiguous (zero or many candidates).
func pickSubdir(parent, want, kind string) (string, error) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs)

	if want != "" {
		for _, d := range dirs {
			if d == want {
				return d, nil
			}
		}
		return "", fmt.Errorf("%w: %s %q not found under %s", ErrStoreNotFound, kind, want, parent)
	}

	switch len(dirs) {
	case 0:
		return "", fmt.Errorf("%w: no %s directory under %s", ErrStoreNotFound, kind, parent)
	case 1:
		return dirs[0], nil
	default:
		return "", fmt.Errorf("%w: %d %s dirs under %s (%s) — pass an explicit selector",
			ErrAmbiguousStore, len(dirs), kind, parent, strings.Join(dirs, ", "))
	}
}

// LoadError pairs a file path with the reason it could not be loaded.
type LoadError struct {
	Path string
	Err  error
}

// ListSessions reads every local_*.json session file in dir. Files that fail
// the schema guard are returned as LoadErrors (with ErrUnknownSchema) rather
// than aborting the whole scan — they are reported and skipped, never written.
func ListSessions(dir, deviceID, accountID string) ([]*Session, []LoadError, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	var sessions []*Session
	var loadErrs []LoadError
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "local_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(dir, name)
		s, lerr := LoadSession(path)
		if lerr != nil {
			loadErrs = append(loadErrs, LoadError{Path: path, Err: lerr})
			continue
		}
		s.DeviceID = deviceID
		s.AccountID = accountID
		sessions = append(sessions, s)
	}
	return sessions, loadErrs, nil
}

// FindByCLISessionID returns every validated desktop session whose
// cliSessionId exactly equals cliSessionID across all local device/account
// stores. Exact identity makes a multi-store scan safe: unlike bulk archival,
// this never selects a session from a guessed working directory or account.
func FindByCLISessionID(root, cliSessionID string) ([]*Session, []LoadError, error) {
	if cliSessionID == "" {
		return nil, nil, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("%w: %s", ErrStoreNotFound, root)
		}
		return nil, nil, err
	}

	deviceEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(deviceEntries, func(i, j int) bool { return deviceEntries[i].Name() < deviceEntries[j].Name() })

	var matches []*Session
	var loadErrs []LoadError
	for _, device := range deviceEntries {
		if !device.IsDir() {
			continue
		}
		deviceID := device.Name()
		deviceDir := filepath.Join(root, deviceID)
		accountEntries, readErr := os.ReadDir(deviceDir)
		if readErr != nil {
			loadErrs = append(loadErrs, LoadError{Path: deviceDir, Err: readErr})
			continue
		}
		sort.Slice(accountEntries, func(i, j int) bool { return accountEntries[i].Name() < accountEntries[j].Name() })
		for _, account := range accountEntries {
			if !account.IsDir() {
				continue
			}
			accountID := account.Name()
			sessions, errs, listErr := ListSessions(filepath.Join(deviceDir, accountID), deviceID, accountID)
			if listErr != nil {
				loadErrs = append(loadErrs, LoadError{
					Path: filepath.Join(deviceDir, accountID),
					Err:  listErr,
				})
				continue
			}
			loadErrs = append(loadErrs, errs...)
			for _, s := range sessions {
				if s.CliSessionID == cliSessionID {
					matches = append(matches, s)
				}
			}
		}
	}
	return matches, loadErrs, nil
}

// LoadSession reads and schema-validates a single session file.
func LoadSession(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var p schemaProbe
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%w: %s: %s", ErrUnknownSchema, filepath.Base(path), err.Error())
	}
	if p.SessionID == nil || *p.SessionID == "" || p.IsArchived == nil || p.LastActivityAt == nil {
		return nil, fmt.Errorf("%w: %s: missing sessionId/isArchived/lastActivityAt",
			ErrUnknownSchema, filepath.Base(path))
	}
	// The flag we mutate must appear exactly once as a top-level token so the
	// surgical write is unambiguous; refuse otherwise rather than risk a wrong
	// rewrite.
	if n := len(archivedTokenRe.FindAllIndex(data, -1)); n != 1 {
		return nil, fmt.Errorf("%w: %s: %d isArchived tokens (want 1)",
			ErrUnknownSchema, filepath.Base(path), n)
	}

	return &Session{
		Path:           path,
		SessionID:      *p.SessionID,
		CliSessionID:   p.CliSessionID,
		Cwd:            p.Cwd,
		OriginCwd:      p.OriginCwd,
		Title:          p.Title,
		Model:          p.Model,
		CreatedAt:      int64(p.CreatedAt),
		LastActivityAt: int64(*p.LastActivityAt),
		IsArchived:     *p.IsArchived,
		raw:            data,
	}, nil
}

// archivedTokenRe matches the top-level `"isArchived": <bool>` pair, tolerating
// optional whitespace around the colon as some writers emit. The boolean is a
// capture group so only the literal true/false is rewritten.
var archivedTokenRe = regexp.MustCompile(`"isArchived"\s*:\s*(true|false)`)

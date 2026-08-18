// Package piadapter implements Pi-native authorization without depending on
// AGM's higher-level permission surface registry.
package piadapter

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MarshalPolicy produces the minimal stable policy consumed by the Pi bridge.
func MarshalPolicy(allow []string) (string, error) {
	if allow == nil {
		allow = []string{}
	}
	encoded, err := json.Marshal(struct {
		Allow []string `json:"allow"`
	}{Allow: allow})
	if err != nil {
		return "", fmt.Errorf("encode Pi permission policy: %w", err)
	}
	return string(encoded), nil
}

//go:embed pi_authorization_extension.js
var piAuthorizationExtension []byte

// DecisionAction is the native authorization outcome expected by Pi.
type DecisionAction string

const (
	// Allow authorizes a Pi tool call without prompting.
	Allow DecisionAction = "allow"
	// Ask delegates a Pi tool call to the interactive user.
	Ask DecisionAction = "ask"
	// Block denies a Pi tool call.
	Block DecisionAction = "block"
)

// ToolCall is the harness-neutral projection of a Pi tool_call event.
type ToolCall struct {
	ToolName string
	Input    map[string]any
}

// Decision is an authorization result with a user-facing reason.
type Decision struct {
	Action DecisionAction
	Reason string
}

// DecideToolCall applies AGM mode and pre-approved policy semantics.
func DecideToolCall(mode string, allow []string, call ToolCall, interactive bool) Decision {
	if mode == "auto" {
		return Decision{Action: Allow, Reason: "AGM auto mode"}
	}
	if mode == "plan" && !isPiPlanTool(call.ToolName) {
		return Decision{Action: Block, Reason: "tool is disabled in AGM plan mode"}
	}
	if PolicyAllows(allow, call) {
		return Decision{Action: Allow, Reason: "tool call matches the AGM permission policy"}
	}
	if interactive {
		return Decision{Action: Ask, Reason: "tool call is not pre-approved by the AGM permission policy"}
	}
	return Decision{Action: Block, Reason: "unmatched AGM permission policy call blocked without an interactive UI"}
}

// PolicyAllows reports whether a Pi call matches one resolved AGM entry.
func PolicyAllows(allow []string, call ToolCall) bool {
	category, value := piPermissionTarget(call)
	if category == "" {
		return false
	}
	// A prefix allowlist entry must never authorize a second shell program.
	// Keep compound commands on Pi's interactive/default decision path instead
	// of trying to partially interpret shell syntax here.
	if category == "Bash" && containsUnquotedShellControl(value) {
		return false
	}
	for _, entry := range allow {
		entryCategory, pattern, ok := parsePermissionEntry(entry)
		if !ok || entryCategory != category {
			continue
		}
		if pattern == "" || matchPermissionPattern(pattern, value) {
			return true
		}
	}
	return false
}

type shellQuoteState uint8

const (
	unquoted shellQuoteState = iota
	singleQuoted
	doubleQuoted
)

func containsUnquotedShellControl(command string) bool {
	quote := unquoted
	escaped := false
	for index := 0; index < len(command); index++ {
		current := command[index]
		if escaped {
			escaped = false
			continue
		}
		switch quote {
		case singleQuoted:
			if current == '\'' {
				quote = unquoted
			}
		case doubleQuoted:
			var control bool
			quote, escaped, control = scanDoubleQuotedShellByte(command, index)
			if control {
				return true
			}
		case unquoted:
			var control bool
			quote, escaped, control = scanUnquotedShellByte(command, index)
			if control {
				return true
			}
		}
	}
	return false
}

func scanDoubleQuotedShellByte(command string, index int) (shellQuoteState, bool, bool) {
	switch command[index] {
	case '\\':
		return doubleQuoted, true, false
	case '"':
		return unquoted, false, false
	case '`':
		return doubleQuoted, false, true
	case '$':
		return doubleQuoted, false, index+1 < len(command) && command[index+1] == '('
	default:
		return doubleQuoted, false, false
	}
}

func scanUnquotedShellByte(command string, index int) (shellQuoteState, bool, bool) {
	switch command[index] {
	case '\\':
		return unquoted, true, false
	case '\'':
		return singleQuoted, false, false
	case '"':
		return doubleQuoted, false, false
	case ';', '&', '|', '<', '>', '`', '\n', '\r':
		return unquoted, false, true
	case '$':
		return unquoted, false, index+1 < len(command) && command[index+1] == '('
	default:
		return unquoted, false, false
	}
}

func isPiPlanTool(tool string) bool {
	switch strings.ToLower(tool) {
	case "read", "grep", "find", "ls":
		return true
	default:
		return false
	}
}

func piPermissionTarget(call ToolCall) (string, string) {
	value := func(keys ...string) string {
		for _, key := range keys {
			if candidate, ok := call.Input[key].(string); ok {
				return candidate
			}
		}
		return ""
	}
	switch strings.ToLower(call.ToolName) {
	case "bash":
		return "Bash", value("command")
	case "read":
		return "Read", value("path", "file_path")
	case "edit":
		return "Edit", value("path", "file_path")
	case "write":
		return "Write", value("path", "file_path")
	case "grep":
		return "Grep", value("path")
	case "find":
		return "Glob", value("path")
	case "ls":
		return "Read", value("path")
	default:
		return customPiToolCategory(call.ToolName), ""
	}
}

func customPiToolCategory(tool string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(tool), func(r rune) bool {
		return r == '_' || r == '-'
	})
	if len(parts) == 0 {
		return ""
	}
	var category strings.Builder
	for _, part := range parts {
		if part == "" {
			return ""
		}
		for _, r := range part {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
				return ""
			}
		}
		category.WriteString(strings.ToUpper(part[:1]))
		category.WriteString(part[1:])
	}
	return category.String()
}

func parsePermissionEntry(entry string) (category, pattern string, ok bool) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return "", "", false
	}
	open := strings.IndexByte(entry, '(')
	if open < 0 {
		return entry, "", true
	}
	withoutClose, closed := strings.CutSuffix(entry, ")")
	if !closed || open == 0 {
		return "", "", false
	}
	return entry[:open], withoutClose[open+1:], true
}

func matchPermissionPattern(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if withoutWildcard, ok := strings.CutSuffix(pattern, ":*"); ok {
		base := strings.TrimSpace(withoutWildcard)
		return value == base || strings.HasPrefix(value, base+" ")
	}
	if homeRelative, ok := strings.CutPrefix(pattern, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			pattern = filepath.Join(home, homeRelative)
		}
	}
	var expression strings.Builder
	expression.WriteByte('^')
	for part := range strings.SplitSeq(pattern, "*") {
		expression.WriteString(regexp.QuoteMeta(part))
		expression.WriteString(".*")
	}
	compiledBase, _ := strings.CutSuffix(expression.String(), ".*")
	compiledPattern := compiledBase + "$"
	matched, err := regexp.MatchString(compiledPattern, value)
	return err == nil && matched
}

// EnsureExtension atomically installs the embedded Pi bridge.
func EnsureExtension(root string) (string, error) {
	abs, err := ensurePrivateRoot(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(abs, "agm-authorization.js")
	if pathInfo, statErr := os.Lstat(path); statErr == nil && (pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular()) {
		return "", fmt.Errorf("pi authorization extension target must be a regular file: %q", path)
	}
	if existing, readErr := os.ReadFile(path); readErr == nil && string(existing) == string(piAuthorizationExtension) {
		if err := os.Chmod(path, 0o600); err != nil {
			return "", err
		}
		return path, nil
	}
	return writePrivateFile(abs, path, piAuthorizationExtension, "extension")
}

// EnsurePolicyFile atomically installs one normalized per-session policy in
// AGM-owned storage so large allowlists never cross the terminal input queue.
func EnsurePolicyFile(root, sessionKey, raw string) (string, error) {
	if strings.TrimSpace(sessionKey) == "" {
		return "", fmt.Errorf("pi permission policy requires a session key")
	}
	var policy struct {
		Allow []string `json:"allow"`
	}
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return "", fmt.Errorf("decode Pi permission policy: %w", err)
	}
	if policy.Allow == nil {
		policy.Allow = []string{}
	}
	normalized, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("encode Pi permission policy: %w", err)
	}
	abs, err := ensurePrivateRoot(root)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(sessionKey))
	path := filepath.Join(abs, fmt.Sprintf("policy-%x.json", digest[:16]))
	if info, statErr := os.Lstat(path); statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return "", fmt.Errorf("pi permission policy target must be a regular file: %q", path)
	}
	if existing, readErr := os.ReadFile(path); readErr == nil && string(existing) == string(normalized) {
		if err := os.Chmod(path, 0o600); err != nil {
			return "", err
		}
		return path, nil
	}
	return writePrivateFile(abs, path, normalized, "policy")
}

func ensurePrivateRoot(root string) (string, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home for Pi authorization extension: %w", err)
		}
		root = filepath.Join(home, ".agm", "pi")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve Pi extension root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", fmt.Errorf("create Pi extension root: %w", err)
	}
	rootInfo, err := os.Lstat(abs)
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", fmt.Errorf("pi extension root must be a non-symlink directory: %q", abs)
	}
	// #nosec G302 -- an owner-only directory needs execute bits for traversal.
	if err := os.Chmod(abs, 0o700); err != nil {
		return "", fmt.Errorf("protect Pi extension root: %w", err)
	}
	return abs, nil
}

func writePrivateFile(root, path string, data []byte, label string) (string, error) {
	temp, err := os.CreateTemp(root, ".agm-pi-"+label+"-*")
	if err != nil {
		return "", fmt.Errorf("create Pi %s temp file: %w", label, err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return "", err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("write Pi %s: %w", label, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("sync Pi %s: %w", label, err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close Pi %s: %w", label, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return "", fmt.Errorf("install Pi %s: %w", label, err)
	}
	return path, nil
}

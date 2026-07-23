package harnessexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

const (
	handoffVersion  = 1
	handoffMaxAge   = 10 * time.Minute
	handoffMaxSize  = 64 << 10
	expiryHelperEnv = "AGM_PRIVATE_HANDOFF_EXPIRY_HELPER"
)

var (
	executablePath        = os.Executable
	scheduleHandoffExpiry = startHandoffExpiry
)

// PreparedCommand is a token-free pane command plus cleanup for a handoff that
// was never delivered. Once the command executes, the private executor removes
// the handoff before replacing itself with the harness.
type PreparedCommand struct {
	Command string
	path    string
}

// Cancel removes an undelivered one-shot handoff. It is safe after consumption.
func (p PreparedCommand) Cancel() error {
	if p.path == "" {
		return nil
	}
	err := os.Remove(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

type launchHandoff struct {
	Version     int      `json:"version"`
	Protocol    string   `json:"protocol"`
	CreatedAt   string   `json:"created_at"`
	Environment []string `json:"environment"`
}

// PrepareCodexCommand snapshots only Codex's documented allowlist from the
// caller and writes it to an owner-only, one-shot handoff. This keeps caller
// authentication authoritative even when a long-lived tmux server has stale
// environment state.
func PrepareCodexCommand(launch CodexLaunch, parent []string) (PreparedCommand, error) {
	executable, err := resolvePrivateExecutable()
	if err != nil {
		return PreparedCommand{}, fmt.Errorf("resolve AGM private executor: %w", err)
	}
	snapshot := removeEnvironment(CodexEnvironment(parent, launch.SessionName), paneIdentityEnvironment)
	handoffPath, err := stageHandoff(CodexProtocol, snapshot)
	if err != nil {
		return PreparedCommand{}, err
	}
	if err := scheduleHandoffExpiry(executable, handoffPath, time.Now().Add(handoffMaxAge)); err != nil {
		return PreparedCommand{}, cleanupFailedHandoff(handoffPath, err)
	}
	launch.Executable = executable
	launch.HandoffPath = handoffPath
	return PreparedCommand{Command: BuildCodexCommand(launch), path: handoffPath}, nil
}

// PrepareClaudeCommand snapshots the caller's selected authentication and
// telemetry configuration into an owner-only, one-shot handoff.
func PrepareClaudeCommand(launch ClaudeLaunch, parent []string) (PreparedCommand, error) {
	executable, err := resolvePrivateExecutable()
	if err != nil {
		return PreparedCommand{}, fmt.Errorf("resolve AGM private executor: %w", err)
	}
	values := environmentMap(parent)
	forward := make([]string, 0, 4)
	if !launch.DisableOAuth {
		if token := resolveClaudeOAuth(); token != "" {
			forward = append(forward, auth.OAuthEnvVar+"="+token)
		} else if key := values["ANTHROPIC_API_KEY"]; key != "" {
			forward = append(forward, "ANTHROPIC_API_KEY="+key)
		}
	} else if key := values["ANTHROPIC_API_KEY"]; key != "" {
		forward = append(forward, "ANTHROPIC_API_KEY="+key)
	}
	if launch.ForwardTelemetry {
		for _, name := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS"} {
			if value := values[name]; value != "" {
				forward = append(forward, name+"="+value)
			}
		}
	}
	handoffPath, err := stageHandoff(ClaudeProtocol, forward)
	if err != nil {
		return PreparedCommand{}, err
	}
	if err := scheduleHandoffExpiry(executable, handoffPath, time.Now().Add(handoffMaxAge)); err != nil {
		return PreparedCommand{}, cleanupFailedHandoff(handoffPath, err)
	}
	launch.Executable = executable
	launch.HandoffPath = handoffPath
	return PreparedCommand{Command: BuildClaudeCommand(launch), path: handoffPath}, nil
}

func cleanupFailedHandoff(path string, scheduleErr error) error {
	removeErr := os.Remove(path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(fmt.Errorf("schedule private launch handoff expiration: %w", scheduleErr), removeErr)
}

func startHandoffExpiry(executable, path string, expiresAt time.Time) error {
	cmd := exec.Command( // #nosec G204,G702 -- executable is the resolved current/co-installed AGM binary.
		executable,
		ExpiryProtocol,
		"--handoff", path,
		"--expires-at", expiresAt.UTC().Format(time.RFC3339Nano),
	)
	// The expiry helper needs neither caller credentials nor terminal ownership.
	// Its isolated process group and released handle let it outlive an AGM caller
	// that exits immediately after queuing a tmux command.
	cmd.Env = []string{expiryHelperEnv + "=1"}
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start expiration helper: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release expiration helper: %w", err)
	}
	return nil
}

func expireHandoff(path string, expiresAt time.Time, now func() time.Time, wait func(time.Duration)) error {
	original, err := expiryTarget(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for {
		current, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return fmt.Errorf("inspect expiring private launch handoff: %w", statErr)
		}
		if !os.SameFile(original, current) {
			return errors.New("private launch handoff changed before expiration")
		}
		remaining := expiresAt.Sub(now())
		if remaining <= 0 {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("expire private launch handoff: %w", err)
			}
			return nil
		}
		if remaining > 250*time.Millisecond {
			remaining = 250 * time.Millisecond
		}
		wait(remaining)
	}
}

func expiryTarget(path string) (os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Base(filepath.Dir(path)) != "private-launch" {
		return nil, errors.New("private launch handoff expiration requires an absolute private-launch path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 ||
		!strings.HasPrefix(name, "launch-") || !strings.HasSuffix(name, ".json") {
		return nil, errors.New("private launch handoff expiration target is invalid")
	}
	return info, nil
}

func resolvePrivateExecutable() (string, error) {
	current, err := executablePath()
	if err != nil {
		return "", err
	}
	current, err = filepath.Abs(current)
	if err != nil {
		return "", err
	}
	name := filepath.Base(current)
	if name != "agm-mcp-server" {
		return current, nil
	}
	// Companion binaries such as agm-mcp-server must execute the co-installed
	// AGM binary, because they do not intercept the private protocol themselves.
	sibling := filepath.Join(filepath.Dir(current), "agm")
	if isExecutableFile(sibling) {
		return sibling, nil
	}
	installed, err := exec.LookPath("agm")
	if err != nil {
		return "", fmt.Errorf("find AGM private executor beside %s or on PATH: %w", current, err)
	}
	return filepath.Abs(installed)
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

func privateExecutable(configured string) string {
	if configured != "" {
		return configured
	}
	return "agm"
}

func stageHandoff(protocol string, environment []string) (string, error) {
	if err := validateHandoffEnvironment(protocol, environment); err != nil {
		return "", err
	}
	root, err := filepath.Abs(handoffRoot())
	if err != nil {
		return "", fmt.Errorf("resolve private launch handoff directory: %w", err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", fmt.Errorf("create private launch handoff directory: %w", err)
	}
	// #nosec G302 -- the owner-only directory requires traversal permission.
	if err := os.Chmod(root, 0700); err != nil {
		return "", fmt.Errorf("secure private launch handoff directory: %w", err)
	}
	removeStaleHandoffs(root, time.Now())

	file, err := os.CreateTemp(root, "launch-*.json")
	if err != nil {
		return "", fmt.Errorf("create private launch handoff: %w", err)
	}
	path := file.Name()
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return "", fmt.Errorf("secure private launch handoff: %w", err)
	}
	payload := launchHandoff{
		Version: handoffVersion, Protocol: protocol,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Environment: append([]string(nil), environment...),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("write private launch handoff: %w", err)
	}
	encoded = append(encoded, '\n')
	if len(encoded) > handoffMaxSize {
		return "", errors.New("private launch handoff exceeds the size limit")
	}
	if _, err := file.Write(encoded); err != nil {
		return "", fmt.Errorf("write private launch handoff: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync private launch handoff: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close private launch handoff: %w", err)
	}
	closed = true
	return path, nil
}

func consumeHandoff(path, protocol string) (launchHandoff, error) {
	file, err := openPrivateHandoff(path)
	if err != nil {
		return launchHandoff{}, err
	}
	defer func() { _ = file.Close() }()
	handoff, err := decodeHandoff(file)
	if err != nil {
		return launchHandoff{}, err
	}
	if err := validateHandoff(handoff, protocol, time.Now()); err != nil {
		return launchHandoff{}, err
	}
	if err := os.Remove(path); err != nil {
		return launchHandoff{}, fmt.Errorf("consume private launch handoff: %w", err)
	}
	return handoff, nil
}

func openPrivateHandoff(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect private launch handoff: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private launch handoff is not an owner-only regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open private launch handoff: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened private launch handoff: %w", err)
	}
	if !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm()&0077 != 0 || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, errors.New("private launch handoff changed while it was being opened")
	}
	return file, nil
}

func decodeHandoff(file *os.File) (launchHandoff, error) {
	payload, err := io.ReadAll(io.LimitReader(file, handoffMaxSize+1))
	if err != nil {
		return launchHandoff{}, fmt.Errorf("read private launch handoff: %w", err)
	}
	if len(payload) > handoffMaxSize {
		return launchHandoff{}, errors.New("private launch handoff exceeds the size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var handoff launchHandoff
	if err := decoder.Decode(&handoff); err != nil {
		return launchHandoff{}, fmt.Errorf("decode private launch handoff: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return launchHandoff{}, errors.New("private launch handoff contains trailing content")
	}
	return handoff, nil
}

func validateHandoff(handoff launchHandoff, protocol string, now time.Time) error {
	createdAt, err := time.Parse(time.RFC3339Nano, handoff.CreatedAt)
	age := now.Sub(createdAt)
	if err != nil || age < 0 || age > handoffMaxAge {
		return errors.New("private launch handoff is expired or has an invalid timestamp")
	}
	if handoff.Version != handoffVersion || handoff.Protocol != protocol {
		return errors.New("private launch handoff does not match the requested protocol")
	}
	if err := validateHandoffEnvironment(protocol, handoff.Environment); err != nil {
		return err
	}
	return nil
}

func validateHandoffEnvironment(protocol string, environment []string) error {
	allowed := handoffEnvironmentAllowlist(protocol)
	seen := make(map[string]bool, len(environment))
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == "" || !allowed[name] || seen[name] || strings.IndexFunc(entry, unicode.IsControl) >= 0 {
			return errors.New("private launch handoff contains an invalid environment entry")
		}
		seen[name] = true
	}
	return nil
}

func handoffEnvironmentAllowlist(protocol string) map[string]bool {
	allowed := make(map[string]bool)
	switch protocol {
	case CodexProtocol:
		for _, name := range codexAllowedEnvironment {
			allowed[name] = true
		}
		allowed["AGM_SESSION_NAME"] = true
	case ClaudeProtocol:
		for _, name := range []string{
			auth.OAuthEnvVar, "ANTHROPIC_API_KEY",
			"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS",
		} {
			allowed[name] = true
		}
	}
	return allowed
}

func handoffRoot() string {
	stateDir := os.Getenv("AGM_STATE_DIR")
	if stateDir == "" {
		stateDir = fmt.Sprintf("/tmp/agm-%d", os.Getuid())
	}
	return filepath.Join(stateDir, "private-launch")
}

func removeStaleHandoffs(root string, now time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "launch-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr == nil && now.Sub(info.ModTime()) > handoffMaxAge {
			_ = os.Remove(filepath.Join(root, entry.Name()))
		}
	}
}

func overlayEnvironment(base, overrides []string, preserve map[string]bool) []string {
	values := environmentMap(base)
	for _, entry := range overrides {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "" && !preserve[name] {
			values[name] = value
		}
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+values[name])
	}
	return result
}

func removeEnvironment(base []string, remove map[string]bool) []string {
	result := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if ok && !remove[name] {
			result = append(result, entry)
		}
	}
	return result
}

func selectedEnvironment(base []string, names map[string]bool) []string {
	result := make([]string, 0, len(names))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if ok && names[name] {
			result = append(result, entry)
		}
	}
	return result
}

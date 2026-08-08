// Command spec-contract-hook adapts the provider-neutral SPEC guard to native
// terminal hook protocols. Repository manifests invoke mutable checkout code,
// so this adapter provides cooperative feedback rather than tamper-resistant
// enforcement. Any mandatory immutable enforcement requires a separately
// reviewed changed-SPEC CI and provider rollout, which this command does not
// attest. AGM's unattended Codex bypass may preserve this adapter only through
// the separately validated operator-owned executable and a digest-attested
// launch snapshot. This command never installs or attests a provider hook.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/specguard"
)

const (
	maxHookInputBytes    = 1 << 20
	maxHookOutputBytes   = 16 << 10
	maxReminderMarkers   = 256
	reminderMarkerPrefix = "antigravity-v3-"
	reminderTempPrefix   = ".antigravity-v3-update-"
	reminderLockName     = ".antigravity-v3.lock"
	reminderStateEnv     = "DEAR_AGENT_HOOK_STATE_DIR"
	reminderMarkerTTL    = 24 * time.Hour
	reminderLockWait     = 2 * time.Second
	workspaceGitTimeout  = 10 * time.Second
)

var errIncompleteReminderMarker = errors.New("reminder marker content is incomplete")

const stagedSPECReminderMessage = "SPEC contract files changed in the Git index. Cooperative deterministic checks passed, but review provider-neutral ownership and consolidation before finishing. Read docs/spec-authoring.md, then follow the single-source authoring workflow at spec-governance/skills/write-spec/SKILL.md; reference that skill instead of copying its body. This source route does not claim native skill discovery. This mutable checkout hook is not tamper-resistant. A separately reviewed changed-SPEC CI and provider rollout is required for mandatory immutable enforcement; this hook does not attest that enforcement is deployed, has run, or is provider-required."

type providerProtocol string

const (
	protocolClaude      providerProtocol = "claude"
	protocolCodex       providerProtocol = "codex"
	protocolAntigravity providerProtocol = "antigravity"
	protocolOpenCode    providerProtocol = "opencode"
	protocolPi          providerProtocol = "pi"
)

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

type hookResponse struct {
	Decision           string              `json:"decision,omitempty"`
	Reason             string              `json:"reason,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type antigravityStopInput struct {
	ConversationID  string   `json:"conversationId"`
	ExecutionNumber int      `json:"executionNum"`
	WorkspacePaths  []string `json:"workspacePaths"`
}

type terminalHookInput struct {
	StopHookActive bool `json:"stop_hook_active"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, input io.Reader, output, stderr io.Writer) int {
	flags := flag.NewFlagSet("spec-contract-hook", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repository := flags.String("root", ".", "repository root supplied by the native hook manifest")
	rootFromWorkspaceInput := flags.Bool("root-from-workspace-stdin", false, "derive the Antigravity repository root from native workspacePaths input")
	event := flags.String("event", "", "terminal hook event")
	provider := flags.String("provider", "", "native hook protocol: claude, codex, antigravity, opencode, or pi")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *repository == "" {
		return emitJSON(output, protocolFailure(providerProtocol(*provider), "Cooperative SPEC contract check unavailable: hook invocation must provide a repository root, supported provider protocol, and native terminal event"))
	}
	protocol := providerProtocol(*provider)
	if !supportedProviderEvent(protocol, *event) {
		return emitJSON(output, protocolFailure(protocol, "Cooperative SPEC contract check unavailable: native provider protocol does not support the requested terminal event"))
	}
	payload, err := readBoundedInput(input, maxHookInputBytes)
	if err != nil {
		return emitJSON(output, protocolFailure(protocol, "Cooperative SPEC contract check unavailable: hook input exceeded its safety limit"))
	}
	resolvedRepository := *repository
	if *rootFromWorkspaceInput {
		if protocol != protocolAntigravity {
			return emitJSON(output, protocolFailure(protocol, "Cooperative SPEC contract check unavailable: workspace-derived repository roots are supported only for the Antigravity protocol"))
		}
		resolvedRepository, err = antigravityWorkspaceRoot(payload)
		if err != nil {
			return emitJSON(output, antigravityProtocolFailure(payload, stderr,
				"Cooperative SPEC contract check unavailable: Antigravity input must supply exactly one valid absolute Git workspace root"))
		}
	}

	result := specguard.Evaluate(context.Background(), specguard.Request{
		Repository: resolvedRepository,
		Mode:       specguard.ModeStaged,
	})
	alreadyReminded := false
	if protocol == protocolAntigravity && result.Decision == specguard.DecisionReminder {
		claimed, claimErr := antigravityReminderAlreadyClaimed(resolvedRepository, payload, result)
		if claimErr == nil {
			alreadyReminded = claimed
		} else {
			_, _ = fmt.Fprintf(stderr, "spec-contract-hook: Antigravity reminder state unavailable; using the bounded execution counter fallback: %.256s\n", claimErr.Error())
			alreadyReminded = antigravityExecutionFallback(payload)
		}
	}
	return emitJSON(output, responseFor(protocol, *event, payload, result, alreadyReminded))
}

// antigravityWorkspaceRoot selects a repository only from the provider's
// native absolute workspace path. The hook process CWD is not a provider
// contract, and a multi-folder project has no safe implicit primary root.
func antigravityWorkspaceRoot(input []byte) (string, error) {
	var parsed antigravityStopInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return "", fmt.Errorf("decode Antigravity hook input: %w", err)
	}
	if len(parsed.WorkspacePaths) != 1 {
		return "", fmt.Errorf("workspace path count = %d, want exactly one", len(parsed.WorkspacePaths))
	}
	workspace := parsed.WorkspacePaths[0]
	if !filepath.IsAbs(workspace) {
		return "", errors.New("workspace path is not absolute")
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(filepath.Clean(workspace))
	if err != nil {
		return "", fmt.Errorf("canonicalize workspace path: %w", err)
	}
	info, err := os.Stat(canonicalWorkspace)
	if err != nil {
		return "", fmt.Errorf("stat workspace path: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("workspace path is not a directory")
	}

	ctx, cancel := context.WithTimeout(context.Background(), workspaceGitTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "-C", canonicalWorkspace, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve Git workspace root: %w", err)
	}
	gitRoot := strings.TrimSpace(string(output))
	if gitRoot == "" || strings.ContainsAny(gitRoot, "\r\n") || !filepath.IsAbs(gitRoot) {
		return "", errors.New("git workspace root is invalid")
	}
	canonicalGitRoot, err := filepath.EvalSymlinks(filepath.Clean(gitRoot))
	if err != nil {
		return "", fmt.Errorf("canonicalize Git workspace root: %w", err)
	}
	if canonicalGitRoot != canonicalWorkspace {
		return "", errors.New("workspace path is not the Git worktree root")
	}
	return canonicalGitRoot, nil
}

func supportedProviderEvent(provider providerProtocol, event string) bool {
	switch provider {
	case protocolClaude, protocolCodex, protocolPi:
		return event == "Stop" || event == "SubagentStop"
	case protocolAntigravity, protocolOpenCode:
		return event == "Stop"
	default:
		return false
	}
}

func readBoundedInput(input io.Reader, limit int64) ([]byte, error) {
	if input == nil || limit <= 0 {
		return nil, fmt.Errorf("hook input is unavailable")
	}
	payload, err := io.ReadAll(io.LimitReader(input, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("hook input exceeds %d bytes", limit)
	}
	return payload, nil
}

func responseFor(protocol providerProtocol, event string, input []byte, result specguard.Result, alreadyReminded bool) hookResponse {
	if result.Decision == specguard.DecisionAllow {
		return allowResponse(protocol)
	}

	message := stagedSPECReminderMessage
	if result.Decision == specguard.DecisionBlock {
		message = blockReason(result)
	}

	switch protocol {
	case protocolClaude:
		return claudeResponse(event, input, result.Decision, message)
	case protocolPi:
		return piResponse(event, input, result.Decision, message)
	case protocolCodex, protocolOpenCode:
		return codexLikeResponse(input, result.Decision, message)
	case protocolAntigravity:
		return antigravityResponse(result.Decision, message, alreadyReminded)
	default:
		return hookResponse{Decision: "block", Reason: message}
	}
}

func allowResponse(protocol providerProtocol) hookResponse {
	if protocol == protocolAntigravity {
		return hookResponse{Decision: "allow"}
	}
	return hookResponse{}
}

const repeatedReminderMessage = "SPEC contract reminder already continued this terminal turn; allowing stop to avoid a retry loop. Review the staged ownership and consolidation before the next real user turn."

func claudeResponse(event string, input []byte, decision specguard.Decision, message string) hookResponse {
	if decision == specguard.DecisionReminder && stopHookActive(input) {
		return hookResponse{SystemMessage: repeatedReminderMessage}
	}
	if decision == specguard.DecisionReminder || decision == specguard.DecisionBlock {
		return hookResponse{Decision: "block", Reason: message}
	}
	return hookResponse{HookSpecificOutput: &hookSpecificOutput{
		HookEventName:     event,
		AdditionalContext: message,
	}}
}

func piResponse(event string, input []byte, decision specguard.Decision, message string) hookResponse {
	if decision == specguard.DecisionReminder && stopHookActive(input) {
		return hookResponse{HookSpecificOutput: &hookSpecificOutput{HookEventName: event, AdditionalContext: repeatedReminderMessage}}
	}
	if decision == specguard.DecisionReminder || decision == specguard.DecisionBlock {
		return hookResponse{Decision: "block", Reason: message}
	}
	return hookResponse{HookSpecificOutput: &hookSpecificOutput{HookEventName: event, AdditionalContext: message}}
}

func codexLikeResponse(input []byte, decision specguard.Decision, message string) hookResponse {
	if decision == specguard.DecisionReminder && stopHookActive(input) {
		return hookResponse{SystemMessage: repeatedReminderMessage}
	}
	if decision == specguard.DecisionReminder || decision == specguard.DecisionBlock {
		return hookResponse{Decision: "block", Reason: message, SystemMessage: message}
	}
	return hookResponse{SystemMessage: message}
}

func antigravityResponse(decision specguard.Decision, message string, alreadyReminded bool) hookResponse {
	if decision == specguard.DecisionReminder && alreadyReminded {
		// Antigravity injects a Stop reason only when it also continues. Do
		// not create a perpetual reminder loop after the one useful review
		// continuation; an ordinary decision permits the stop.
		return hookResponse{Decision: "allow"}
	}
	return hookResponse{Decision: "continue", Reason: message}
}

func stopHookActive(input []byte) bool {
	var parsed terminalHookInput
	return json.Unmarshal(input, &parsed) == nil && parsed.StopHookActive
}

func protocolFailure(protocol providerProtocol, reason string) hookResponse {
	if protocol == protocolAntigravity {
		// Without a decoded native conversation identity and execution number,
		// Antigravity has no bounded retry signal. Allow termination instead of
		// turning malformed invocations into an infinite Stop continuation.
		return hookResponse{Decision: "allow"}
	}
	if protocol == protocolCodex || protocol == protocolOpenCode {
		return hookResponse{Decision: "block", Reason: reason, SystemMessage: reason}
	}
	return hookResponse{Decision: "block", Reason: reason}
}

func antigravityProtocolFailure(input []byte, stderr io.Writer, reason string) hookResponse {
	var parsed antigravityStopInput
	if json.Unmarshal(input, &parsed) != nil || parsed.ConversationID == "" || parsed.ExecutionNumber <= 0 {
		return hookResponse{Decision: "allow"}
	}
	if parsed.ExecutionNumber > 1 {
		return hookResponse{Decision: "allow"}
	}

	directory, err := reminderStateDirectory()
	if err == nil {
		identity, marshalErr := json.Marshal(struct {
			ConversationID string `json:"conversation_id"`
			Failure        string `json:"failure"`
		}{
			ConversationID: parsed.ConversationID,
			Failure:        "workspace-root",
		})
		if marshalErr == nil {
			conversationDigest := fmt.Sprintf("%x", sha256.Sum256(identity))
			failureDigest := fmt.Sprintf("%x", sha256.Sum256([]byte("antigravity-protocol-failure:workspace-root")))
			already, claimErr := claimReminderMarker(directory, conversationDigest, failureDigest)
			if claimErr == nil {
				if already {
					return hookResponse{Decision: "allow"}
				}
				return hookResponse{Decision: "continue", Reason: reason}
			}
			err = claimErr
		} else {
			err = marshalErr
		}
	}
	_, _ = fmt.Fprintf(stderr, "spec-contract-hook: Antigravity protocol-failure state unavailable; allowing termination to avoid an unbounded retry: %.256s\n", err)
	return hookResponse{Decision: "allow"}
}

func antigravityReminderAlreadyClaimed(repository string, input []byte, result specguard.Result) (bool, error) {
	var parsed antigravityStopInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		// An unparseable provider envelope must not silently convert a reminder
		// into a stop. Keep each invalid invocation fail-closed; the one-attempt
		// guarantee is scoped to valid provider envelopes with stable identity.
		return false, err
	}
	if parsed.ConversationID == "" || result.SnapshotID == "" {
		return false, fmt.Errorf("antigravity reminder identity is incomplete")
	}
	root, err := filepath.Abs(repository)
	if err != nil {
		return false, err
	}
	root, err = filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return false, err
	}
	directory, err := reminderStateDirectory()
	if err != nil {
		return false, err
	}
	conversationIdentity, err := json.Marshal(struct {
		Repository     string `json:"repository"`
		ConversationID string `json:"conversation_id"`
	}{
		Repository:     root,
		ConversationID: parsed.ConversationID,
	})
	if err != nil {
		return false, err
	}
	conversationDigest := fmt.Sprintf("%x", sha256.Sum256(conversationIdentity))
	already, err := claimReminderMarker(directory, conversationDigest, result.SnapshotID)
	if err != nil {
		return false, err
	}
	return already, nil
}

func antigravityExecutionFallback(input []byte) bool {
	var parsed antigravityStopInput
	return json.Unmarshal(input, &parsed) == nil && parsed.ExecutionNumber > 1
}

func reminderStateDirectory() (string, error) {
	directory := os.Getenv(reminderStateEnv)
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		directory = filepath.Join(cache, "dear-agent", "spec-contract-hook")
	}
	if !filepath.IsAbs(directory) {
		return "", fmt.Errorf("reminder state path must be absolute")
	}
	directory = filepath.Clean(directory)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("reminder state path is not a private directory")
	}
	return directory, nil
}

func claimReminderMarker(directory, conversationDigest, snapshotDigest string) (bool, error) {
	if !isLowerHexDigest(conversationDigest) || !isLowerHexDigest(snapshotDigest) {
		return false, fmt.Errorf("reminder identity is invalid")
	}
	release, err := acquireReminderLock(directory)
	if err != nil {
		return false, err
	}
	defer release()

	count, err := pruneExpiredReminderMarkers(directory, time.Now())
	if err != nil {
		return false, err
	}
	path := filepath.Join(directory, reminderMarkerPrefix+conversationDigest)
	content, err := readReminderMarker(path)
	if err == nil {
		if content == snapshotDigest+"\n" {
			return true, nil
		}
		if err := writeReminderMarker(path, snapshotDigest, false); err != nil {
			return false, err
		}
		return false, nil
	}
	if errors.Is(err, errIncompleteReminderMarker) {
		if err := writeReminderMarker(path, snapshotDigest, false); err != nil {
			return false, err
		}
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if count >= maxReminderMarkers {
		return false, fmt.Errorf("reminder marker capacity is exhausted")
	}
	if err := writeReminderMarker(path, snapshotDigest, true); err != nil {
		return false, err
	}
	return false, nil
}

func acquireReminderLock(directory string) (func(), error) {
	path := filepath.Join(directory, reminderLockName)
	file, err := openReminderLockFile(path)
	if err != nil {
		return nil, err
	}
	releaseFile := true
	defer func() {
		if releaseFile {
			_ = file.Close()
		}
	}()

	deadline := time.Now().Add(reminderLockWait)
	for {
		locked, err := tryReminderFileLock(file)
		if err != nil {
			return nil, err
		}
		if locked {
			releaseFile = false
			// Closing the descriptor releases the operating-system lock, including
			// after process termination. The persistent file is only its identity.
			return func() { _ = file.Close() }, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("reminder marker lock is unavailable")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func openReminderLockFile(path string) (*os.File, error) {
	// #nosec G703 -- path is the constant lock child of the private reminder-state directory validated by reminderStateDirectory.
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("reminder marker lock identity is unsafe")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// #nosec G703 -- path is the constant lock child of the private reminder-state directory validated by reminderStateDirectory.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	opened, statErr := file.Stat()
	// #nosec G703 -- re-reading the same validated lock child proves the opened descriptor was not replaced or symlinked.
	current, lstatErr := os.Lstat(path)
	if statErr != nil || lstatErr != nil || !opened.Mode().IsRegular() || opened.Mode().Perm()&0o077 != 0 || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, current) {
		_ = file.Close()
		return nil, fmt.Errorf("reminder marker lock identity is unsafe")
	}
	return file, nil
}

func pruneExpiredReminderMarkers(directory string, now time.Time) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		temporary, err := pruneExpiredReminderTemporary(directory, entry, now)
		if err != nil {
			return 0, err
		}
		if temporary {
			continue
		}
		digest := strings.TrimPrefix(name, reminderMarkerPrefix)
		if digest == name || !isLowerHexDigest(digest) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return 0, fmt.Errorf("reminder marker identity is unsafe")
		}
		if now.Sub(info.ModTime()) > reminderMarkerTTL {
			if err := os.Remove(filepath.Join(directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return 0, err
			}
			continue
		}
		count++
	}
	return count, nil
}

func pruneExpiredReminderTemporary(directory string, entry os.DirEntry, now time.Time) (bool, error) {
	if !strings.HasPrefix(entry.Name(), reminderTempPrefix) {
		return false, nil
	}
	info, err := entry.Info()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return true, fmt.Errorf("reminder marker temporary identity is unsafe")
	}
	if now.Sub(info.ModTime()) <= reminderMarkerTTL {
		return true, nil
	}
	err = os.Remove(filepath.Join(directory, entry.Name()))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return true, err
	}
	return true, nil
}

func readReminderMarker(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("reminder marker identity is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 66))
	if err != nil {
		return "", err
	}
	if len(content) != 65 || !isLowerHexDigest(string(content[:64])) || content[64] != '\n' {
		return "", errIncompleteReminderMarker
	}
	return string(content), nil
}

func writeReminderMarker(path, snapshotDigest string, exclusive bool) error {
	directory := filepath.Dir(path)
	marker, err := os.CreateTemp(directory, reminderTempPrefix)
	if err != nil {
		return err
	}
	temporary := marker.Name()
	defer func() { _ = os.Remove(temporary) }()
	if _, err := marker.WriteString(snapshotDigest + "\n"); err != nil {
		_ = marker.Close()
		return err
	}
	if err := marker.Close(); err != nil {
		return err
	}
	if exclusive {
		// A hard link publishes the already-complete inode only if the marker
		// identity is still absent. A crash can strand only the ignored temporary.
		return os.Link(temporary, path)
	}
	return os.Rename(temporary, path)
}

func isLowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func blockReason(result specguard.Result) string {
	if len(result.Findings) == 0 {
		return "Cooperative SPEC contract check unavailable: the provider-neutral guard returned an invalid blocking result"
	}
	parts := make([]string, 0, len(result.Findings))
	for _, finding := range result.Findings {
		part := finding.Code + ": " + finding.Message
		if finding.Path != "" {
			part = finding.Path + " — " + part
		}
		parts = append(parts, part)
	}
	return "Cooperative SPEC contract guard blocked this terminal hook. Fix the deterministic contract findings, then retry. Any mandatory immutable enforcement requires a separately reviewed changed-SPEC CI and provider rollout that this hook does not attest: " + strings.Join(parts, "; ")
}

func emitJSON(output io.Writer, response hookResponse) int {
	encoded, err := json.Marshal(response)
	if err != nil {
		return 1
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxHookOutputBytes {
		// A terminal hook must always leave a complete, actionable envelope.
		// Do not truncate JSON or turn an oversized block explanation into a
		// successful silent exit.
		fallback := hookResponse{Decision: "block", Reason: "Cooperative SPEC contract check unavailable: hook response exceeded its safety limit; run the changed-SPEC CI and inspect the deterministic guard directly."}
		if response.Decision == "continue" {
			fallback.Decision = "continue"
		}
		encoded, _ = json.Marshal(fallback)
		encoded = append(encoded, '\n')
	}
	written, err := output.Write(encoded)
	if err != nil || written != len(encoded) {
		return 1
	}
	return 0
}

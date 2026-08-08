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

	"github.com/vbonnet/dear-agent/internal/hookparity"
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
	operatorOwnedHelper  = "/usr/local/libexec/dear-agent-spec-contract-hook"
)

var errIncompleteReminderMarker = errors.New("reminder marker content is incomplete")

var verifyOperatorOwnedHelperDigest = func(expectedSHA256 string) error {
	return hookparity.VerifyDeployedHelperDigest(
		operatorOwnedHelper,
		expectedSHA256,
		hookparity.ProductionHelperTrustPolicy(),
	)
}

type reminderMarkerState struct {
	feedbackDigest string
}

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
	Decision                string              `json:"decision,omitempty"`
	Reason                  string              `json:"reason,omitempty"`
	SystemMessage           string              `json:"systemMessage,omitempty"`
	HookSpecificOutput      *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	DearAgentSpecFeedbackID string              `json:"dearAgentSpecFeedbackId,omitempty"`
}

type antigravityStopInput struct {
	ConversationID  string   `json:"conversationId"`
	ExecutionNumber *int     `json:"executionNum"`
	WorkspacePaths  []string `json:"workspacePaths"`
}

type terminalHookInput struct {
	SessionID      string `json:"session_id"`
	TurnID         string `json:"turn_id"`
	StopHookActive *bool  `json:"stop_hook_active"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

//nolint:gocyclo // Linear CLI admission keeps every cooperative liveness fallback visible in execution order.
func run(args []string, input io.Reader, output, stderr io.Writer) int {
	flags := flag.NewFlagSet("spec-contract-hook", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repository := flags.String("root", ".", "repository root supplied by the native hook manifest")
	rootFromWorkspaceInput := flags.Bool("root-from-workspace-stdin", false, "derive the Antigravity repository root from native workspacePaths input")
	expectedHelperSHA256 := flags.String("expected-helper-sha256", "", "revision-bound digest required for operator-owned execution")
	event := flags.String("event", "", "terminal hook event")
	provider := flags.String("provider", "", "native hook protocol: claude, codex, antigravity, opencode, or pi")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *repository == "" {
		return emitJSON(output, protocolFailure(providerProtocol(*provider), *event, "Cooperative SPEC contract check unavailable: hook invocation must provide a repository root, supported provider protocol, and native terminal event"))
	}
	protocol := providerProtocol(*provider)
	if !supportedProviderEvent(protocol, *event) {
		return emitJSON(output, protocolFailure(protocol, *event, "Cooperative SPEC contract check unavailable: native provider protocol does not support the requested terminal event"))
	}
	if *expectedHelperSHA256 != "" {
		if err := verifyOperatorOwnedHelperDigest(*expectedHelperSHA256); err != nil {
			return emitJSON(output, hookResponse{
				Decision: "block",
				Reason:   "Unattended SPEC enforcement unavailable: the operator-owned helper no longer matches this session's revision-bound digest",
			})
		}
	}
	payload, err := readBoundedInput(input, maxHookInputBytes)
	if err != nil {
		return emitJSON(output, protocolFailure(protocol, *event, "Cooperative SPEC contract check unavailable: hook input exceeded its safety limit"))
	}
	if *event == "UserPromptSubmit" {
		if err := resetProviderFeedback(*repository, protocol, payload); err != nil {
			_, _ = fmt.Fprintf(stderr, "spec-contract-hook: cannot reset per-turn feedback state; continuing without blocking the user prompt: %.256s\n", err.Error())
		}
		return emitJSON(output, hookResponse{})
	}
	if err := validateProviderInput(protocol, payload); err != nil {
		reason := "Cooperative SPEC contract check unavailable: native hook input is not a valid bounded provider envelope"
		if protocol == protocolAntigravity {
			return emitJSON(output, antigravityProtocolFailure(payload, stderr, reason))
		}
		return emitJSON(output, protocolFailure(protocol, *event, reason))
	}
	resolvedRepository := *repository
	if *rootFromWorkspaceInput {
		if protocol != protocolAntigravity {
			return emitJSON(output, protocolFailure(protocol, *event, "Cooperative SPEC contract check unavailable: workspace-derived repository roots are supported only for the Antigravity protocol"))
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
	alreadyClaimed := false
	claimUnavailable := false
	feedbackID := ""
	if protocol != protocolOpenCode {
		claimed, currentFeedbackID, claimErr := providerFeedbackAlreadyClaimed(resolvedRepository, protocol, payload, result)
		feedbackID = currentFeedbackID
		if claimErr == nil {
			alreadyClaimed = claimed
		} else if result.Decision != specguard.DecisionAllow {
			// stop_hook_active is provider-global and cannot prove which of
			// several matching Stop hooks caused a continuation. If this adapter
			// cannot persist its own session+snapshot claim, use that global flag
			// only as the liveness fallback: an ordinary first stop still receives
			// feedback, while an active continuation yields. Antigravity has no
			// documented sequence origin, so unavailable state yields immediately.
			alreadyClaimed = protocol == protocolAntigravity || terminalStopHookActive(payload)
			claimUnavailable = alreadyClaimed
			_, _ = fmt.Fprintf(stderr, "spec-contract-hook: per-adapter feedback state unavailable; yielding cooperative feedback: %.256s\n", claimErr.Error())
		}
	}
	return emitJSON(output, responseFor(protocol, *event, result, alreadyClaimed, claimUnavailable, feedbackID))
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
	case protocolClaude:
		return event == "Stop" || event == "SubagentStop" || event == "UserPromptSubmit"
	case protocolCodex, protocolPi:
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

//nolint:gocyclo // Straight-line native-provider schema dispatch is clearer and safer than distributed partial validation.
func validateProviderInput(protocol providerProtocol, input []byte) error {
	trimmed := strings.TrimSpace(string(input))
	if protocol == protocolOpenCode && trimmed == "" {
		// The OpenCode plugin owns session identity and intentionally invokes the
		// shared adapter without a native stdin envelope.
		return nil
	}
	var object map[string]json.RawMessage
	if trimmed == "" || json.Unmarshal(input, &object) != nil || object == nil {
		return fmt.Errorf("provider input must be one JSON object")
	}
	switch protocol {
	case protocolClaude, protocolCodex, protocolPi:
		var parsed terminalHookInput
		if err := json.Unmarshal(input, &parsed); err != nil {
			return fmt.Errorf("decode terminal retry signal: %w", err)
		}
		if parsed.SessionID == "" || len(parsed.SessionID) > 4096 || parsed.StopHookActive == nil {
			return fmt.Errorf("terminal input omits a stable session retry identity")
		}
		if protocol == protocolCodex && (parsed.TurnID == "" || len(parsed.TurnID) > 4096) {
			return fmt.Errorf("codex terminal input omits its native turn identity")
		}
	case protocolAntigravity:
		var parsed antigravityStopInput
		if err := json.Unmarshal(input, &parsed); err != nil {
			return fmt.Errorf("decode Antigravity identity: %w", err)
		}
		if parsed.ConversationID == "" || len(parsed.ConversationID) > 4096 || parsed.ExecutionNumber == nil || *parsed.ExecutionNumber < 0 {
			return fmt.Errorf("antigravity input omits a stable conversation execution identity")
		}
	case protocolOpenCode:
		// A future nonempty OpenCode envelope must still be a bounded JSON object.
	default:
		return fmt.Errorf("provider protocol is unsupported")
	}
	return nil
}

func responseFor(protocol providerProtocol, event string, result specguard.Result, alreadyClaimed, claimUnavailable bool, feedbackID string) hookResponse {
	if result.Decision == specguard.DecisionAllow {
		return allowResponse(protocol)
	}

	message := stagedSPECReminderMessage
	if result.Decision == specguard.DecisionBlock {
		message = blockReason(result)
	}
	yieldMessage := repeatedReminderMessage
	if claimUnavailable {
		yieldMessage = "SPEC contract feedback could not establish private per-adapter retry state, so this cooperative hook is yielding rather than risking a loop. Review remains required before relying on the governed change. " + message
	}

	switch protocol {
	case protocolClaude:
		return claudeResponse(event, result.Decision, message, alreadyClaimed, yieldMessage)
	case protocolPi:
		return piResponse(event, result.Decision, message, alreadyClaimed, yieldMessage, feedbackID)
	case protocolCodex, protocolOpenCode:
		return codexLikeResponse(result.Decision, message, alreadyClaimed, yieldMessage)
	case protocolAntigravity:
		return antigravityResponse(result.Decision, message, alreadyClaimed)
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

const repeatedReminderMessage = "SPEC contract feedback already ran for this terminal retry; allowing stop to avoid a loop. Deterministic failures or semantic review may remain and must be addressed before the next governed completion."

func claudeResponse(event string, decision specguard.Decision, message string, alreadyClaimed bool, yieldMessage string) hookResponse {
	if (decision == specguard.DecisionReminder || decision == specguard.DecisionBlock) && alreadyClaimed {
		return hookResponse{SystemMessage: yieldMessage}
	}
	if decision == specguard.DecisionReminder || decision == specguard.DecisionBlock {
		return hookResponse{Decision: "block", Reason: message}
	}
	return hookResponse{HookSpecificOutput: &hookSpecificOutput{
		HookEventName:     event,
		AdditionalContext: message,
	}}
}

func piResponse(event string, decision specguard.Decision, message string, alreadyClaimed bool, yieldMessage, feedbackID string) hookResponse {
	if (decision == specguard.DecisionReminder || decision == specguard.DecisionBlock) && alreadyClaimed {
		return hookResponse{HookSpecificOutput: &hookSpecificOutput{HookEventName: event, AdditionalContext: yieldMessage}, DearAgentSpecFeedbackID: feedbackID}
	}
	if decision == specguard.DecisionReminder || decision == specguard.DecisionBlock {
		return hookResponse{Decision: "block", Reason: message, DearAgentSpecFeedbackID: feedbackID}
	}
	return hookResponse{HookSpecificOutput: &hookSpecificOutput{HookEventName: event, AdditionalContext: message}}
}

func codexLikeResponse(decision specguard.Decision, message string, alreadyClaimed bool, yieldMessage string) hookResponse {
	if (decision == specguard.DecisionReminder || decision == specguard.DecisionBlock) && alreadyClaimed {
		return hookResponse{SystemMessage: yieldMessage}
	}
	if decision == specguard.DecisionReminder || decision == specguard.DecisionBlock {
		return hookResponse{Decision: "block", Reason: message, SystemMessage: message}
	}
	return hookResponse{SystemMessage: message}
}

func antigravityResponse(decision specguard.Decision, message string, alreadyReminded bool) hookResponse {
	if (decision == specguard.DecisionReminder || decision == specguard.DecisionBlock) && alreadyReminded {
		// Antigravity injects a Stop reason only when it also continues. Do
		// not create a perpetual reminder loop after the one useful review
		// continuation; an ordinary decision permits the stop.
		return hookResponse{Decision: "allow"}
	}
	return hookResponse{Decision: "continue", Reason: message}
}

func terminalStopHookActive(input []byte) bool {
	var parsed terminalHookInput
	return json.Unmarshal(input, &parsed) == nil && parsed.StopHookActive != nil && *parsed.StopHookActive
}

func protocolFailure(protocol providerProtocol, event, reason string) hookResponse {
	if protocol == protocolAntigravity {
		// Without a decoded native conversation identity and execution number,
		// Antigravity has no bounded retry signal. Allow termination instead of
		// turning malformed invocations into an infinite Stop continuation.
		return hookResponse{Decision: "allow"}
	}
	if protocol == protocolPi && (event == "Stop" || event == "SubagentStop") {
		return hookResponse{HookSpecificOutput: &hookSpecificOutput{HookEventName: event, AdditionalContext: reason}}
	}
	// A malformed or unavailable cooperative source hook has no stable retry
	// identity. Immediate advisory yield avoids a permanent terminal loop; the
	// separately reviewed changed-SPEC CI remains the mandatory fail-closed seam.
	return hookResponse{SystemMessage: reason}
}

func antigravityProtocolFailure(input []byte, stderr io.Writer, reason string) hookResponse {
	var parsed antigravityStopInput
	if json.Unmarshal(input, &parsed) != nil || parsed.ConversationID == "" || len(parsed.ConversationID) > 4096 || parsed.ExecutionNumber == nil || *parsed.ExecutionNumber < 0 {
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
	already, _, err := providerFeedbackAlreadyClaimed(repository, protocolAntigravity, input, result)
	return already, err
}

//nolint:gocyclo // One ordered state transition keeps native identity validation, hashing, and fail-safe claim handling auditable.
func providerFeedbackAlreadyClaimed(repository string, protocol providerProtocol, input []byte, result specguard.Result) (bool, string, error) {
	var sessionID string
	var nativeTurnID string
	switch protocol {
	case protocolClaude, protocolCodex, protocolPi:
		var parsed terminalHookInput
		if err := json.Unmarshal(input, &parsed); err != nil {
			return false, "", err
		}
		if parsed.SessionID == "" || len(parsed.SessionID) > 4096 || parsed.StopHookActive == nil {
			return false, "", fmt.Errorf("terminal feedback identity is incomplete")
		}
		if protocol == protocolCodex {
			if parsed.TurnID == "" || len(parsed.TurnID) > 4096 {
				return false, "", fmt.Errorf("codex terminal feedback identity omits its native turn")
			}
			nativeTurnID = parsed.TurnID
		}
		sessionID = parsed.SessionID
	case protocolAntigravity:
		var parsed antigravityStopInput
		if err := json.Unmarshal(input, &parsed); err != nil {
			return false, "", err
		}
		if parsed.ConversationID == "" || len(parsed.ConversationID) > 4096 || parsed.ExecutionNumber == nil || *parsed.ExecutionNumber < 0 {
			return false, "", fmt.Errorf("antigravity feedback identity is incomplete")
		}
		sessionID = parsed.ConversationID
	case protocolOpenCode:
		return false, "", fmt.Errorf("opencode owns its terminal retry state")
	default:
		return false, "", fmt.Errorf("provider does not expose per-adapter terminal identity")
	}
	providerDigest, err := providerSessionDigest(repository, protocol, sessionID)
	if err != nil {
		return false, "", err
	}
	feedbackIdentity, err := json.Marshal(struct {
		SchemaVersion string              `json:"schema_version"`
		Decision      specguard.Decision  `json:"decision"`
		Source        string              `json:"source"`
		Base          string              `json:"base"`
		Head          string              `json:"head"`
		SnapshotID    string              `json:"snapshot_id"`
		NativeTurnID  string              `json:"native_turn_id,omitempty"`
		Changed       []string            `json:"changed"`
		Findings      []specguard.Finding `json:"findings"`
	}{
		SchemaVersion: result.SchemaVersion,
		Decision:      result.Decision,
		Source:        result.Source,
		Base:          result.Base,
		Head:          result.Head,
		SnapshotID:    result.SnapshotID,
		NativeTurnID:  nativeTurnID,
		Changed:       result.Changed,
		Findings:      result.Findings,
	})
	if err != nil {
		return false, "", err
	}
	feedbackDigest := fmt.Sprintf("%x", sha256.Sum256(feedbackIdentity))
	// Pi's persistent authorization extension owns its per-turn attempt state.
	// The child adapter returns this bounded identity so the extension can
	// distinguish a new SPEC snapshot from a sibling hook's continuation.
	if protocol == protocolPi {
		return false, feedbackDigest, nil
	}
	directory, err := reminderStateDirectory()
	if err != nil {
		return false, feedbackDigest, err
	}
	if result.Decision == specguard.DecisionAllow {
		if err := clearReminderMarker(directory, providerDigest); err != nil {
			return false, feedbackDigest, err
		}
		return false, feedbackDigest, nil
	}
	already, err := claimReminderMarker(directory, providerDigest, feedbackDigest)
	if err != nil {
		return false, feedbackDigest, err
	}
	return already, feedbackDigest, nil
}

func resetProviderFeedback(repository string, protocol providerProtocol, input []byte) error {
	if protocol != protocolClaude {
		return fmt.Errorf("provider does not expose a configured user-turn reset")
	}
	var parsed terminalHookInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return err
	}
	if parsed.SessionID == "" || len(parsed.SessionID) > 4096 {
		return fmt.Errorf("user-turn reset omits a bounded native session identity")
	}
	providerDigest, err := providerSessionDigest(repository, protocol, parsed.SessionID)
	if err != nil {
		return err
	}
	directory, err := reminderStateDirectory()
	if err != nil {
		return err
	}
	return clearReminderMarker(directory, providerDigest)
}

func providerSessionDigest(repository string, protocol providerProtocol, sessionID string) (string, error) {
	root, err := filepath.Abs(repository)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	identity, err := json.Marshal(struct {
		Repository string           `json:"repository"`
		Provider   providerProtocol `json:"provider"`
		SessionID  string           `json:"session_id"`
	}{
		Repository: root,
		Provider:   protocol,
		SessionID:  sessionID,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(identity)), nil
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
	marker, err := readReminderMarker(path)
	if err == nil {
		if marker.feedbackDigest == snapshotDigest {
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

func clearReminderMarker(directory, conversationDigest string) error {
	if !isLowerHexDigest(conversationDigest) {
		return fmt.Errorf("reminder identity is invalid")
	}
	release, err := acquireReminderLock(directory)
	if err != nil {
		return err
	}
	defer release()

	if _, err := pruneExpiredReminderMarkers(directory, time.Now()); err != nil {
		return err
	}
	path := filepath.Join(directory, reminderMarkerPrefix+conversationDigest)
	if _, err := readReminderMarker(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
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

func readReminderMarker(path string) (reminderMarkerState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return reminderMarkerState{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return reminderMarkerState{}, fmt.Errorf("reminder marker identity is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return reminderMarkerState{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 76))
	if err != nil {
		return reminderMarkerState{}, err
	}
	marker := reminderMarkerState{}
	var digest []byte
	switch {
	case len(content) == 64+1:
		// Accept the pre-phase marker format so an upgraded helper can clear or
		// complete already-safe one-shot state instead of stranding capacity.
		digest = content[:len(content)-1]
	case len(content) == len("claimed ")+64+1 && strings.HasPrefix(string(content), "claimed "):
		digest = content[len("claimed ") : len(content)-1]
	case len(content) == len("completed ")+64+1 && strings.HasPrefix(string(content), "completed "):
		digest = content[len("completed ") : len(content)-1]
	default:
		return reminderMarkerState{}, errIncompleteReminderMarker
	}
	if content[len(content)-1] != '\n' || !isLowerHexDigest(string(digest)) {
		return reminderMarkerState{}, errIncompleteReminderMarker
	}
	marker.feedbackDigest = string(digest)
	return marker, nil
}

func writeReminderMarker(path, snapshotDigest string, exclusive bool) error {
	directory := filepath.Dir(path)
	marker, err := os.CreateTemp(directory, reminderTempPrefix)
	if err != nil {
		return err
	}
	temporary := marker.Name()
	defer func() { _ = os.Remove(temporary) }()
	if _, err := marker.WriteString("claimed " + snapshotDigest + "\n"); err != nil {
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
		if isLowerHexDigest(response.DearAgentSpecFeedbackID) {
			fallback.DearAgentSpecFeedbackID = response.DearAgentSpecFeedbackID
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

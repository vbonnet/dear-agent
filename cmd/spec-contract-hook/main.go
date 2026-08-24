// Command spec-contract-hook adapts the provider-neutral SPEC guard to native
// terminal hook protocols. Repository manifests invoke mutable checkout code,
// so this adapter provides cooperative feedback rather than tamper-resistant
// enforcement. Mandatory immutable enforcement requires a separately reviewed
// changed-SPEC CI and provider rollout, which this command does not attest.
// This command never installs or attests a provider hook.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/internal/specguard"
)

const (
	maxHookInputBytes  = 1 << 20
	maxHookOutputBytes = 16 << 10
)

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

type specEvaluator func(context.Context, specguard.Request) specguard.Result

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, input io.Reader, output, stderr io.Writer) int {
	return runWithEvaluator(args, input, output, stderr, specguard.Evaluate)
}

func runWithEvaluator(args []string, input io.Reader, output, stderr io.Writer, evaluate specEvaluator) int {
	if stderr == nil {
		stderr = io.Discard
	}
	flags := flag.NewFlagSet("spec-contract-hook", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repository := flags.String("root", ".", "repository root supplied by the native hook transport")
	event := flags.String("event", "", "terminal hook event")
	provider := flags.String("provider", "", "native hook protocol: claude, codex, antigravity, opencode, or pi")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *repository == "" {
		return emitJSON(output, protocolFailure(providerProtocol(*provider), *event,
			"Cooperative SPEC contract check unavailable: hook invocation must provide a repository root, supported provider protocol, and native terminal event"))
	}
	protocol := providerProtocol(*provider)
	if !supportedProviderEvent(protocol, *event) {
		return emitJSON(output, protocolFailure(protocol, *event,
			"Cooperative SPEC contract check unavailable: native provider protocol does not support the requested terminal event"))
	}
	payload, err := readBoundedInput(input, maxHookInputBytes)
	if err != nil {
		return emitJSON(output, protocolFailure(protocol, *event,
			"Cooperative SPEC contract check unavailable: hook input exceeded its safety limit"))
	}
	if err := validateProviderInput(protocol, payload); err != nil {
		return emitJSON(output, protocolFailure(protocol, *event,
			"Cooperative SPEC contract check unavailable: native hook input is not a valid bounded provider envelope"))
	}
	if evaluate == nil {
		return emitJSON(output, protocolFailure(protocol, *event,
			"Cooperative SPEC contract check unavailable: provider-neutral guard is unavailable"))
	}

	result := evaluate(context.Background(), specguard.Request{
		Repository: *repository,
		Mode:       specguard.ModeStaged,
	})
	return emitJSON(output, responseFor(protocol, result))
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

func validateProviderInput(protocol providerProtocol, input []byte) error {
	trimmed := strings.TrimSpace(string(input))
	if protocol == protocolOpenCode && trimmed == "" {
		// The OpenCode plugin owns session identity and invokes this adapter
		// without a native stdin envelope.
		return nil
	}
	var object map[string]json.RawMessage
	if trimmed == "" || json.Unmarshal(input, &object) != nil || object == nil {
		return fmt.Errorf("provider input must be one JSON object")
	}
	return nil
}

func responseFor(protocol providerProtocol, result specguard.Result) hookResponse {
	if result.Decision == specguard.DecisionAllow {
		if protocol == protocolAntigravity {
			return hookResponse{Decision: "allow"}
		}
		return hookResponse{}
	}

	message := stagedSPECReminderMessage
	if result.Decision != specguard.DecisionReminder {
		message = blockReason(result)
	}
	switch protocol {
	case protocolClaude, protocolPi:
		return hookResponse{Decision: "block", Reason: message}
	case protocolCodex, protocolOpenCode:
		return hookResponse{Decision: "block", Reason: message, SystemMessage: message}
	case protocolAntigravity:
		return hookResponse{Decision: "continue", Reason: message}
	default:
		return hookResponse{Decision: "block", Reason: message}
	}
}

func protocolFailure(protocol providerProtocol, event, reason string) hookResponse {
	if protocol == protocolAntigravity {
		// A malformed invocation has no bounded native retry identity. Permit
		// termination rather than creating an infinite Stop continuation.
		return hookResponse{Decision: "allow"}
	}
	if protocol == protocolPi && (event == "Stop" || event == "SubagentStop") {
		return hookResponse{HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     event,
			AdditionalContext: reason,
		}}
	}
	// A malformed cooperative source hook has no stable retry identity.
	// Advisory yield avoids a terminal loop; changed-SPEC CI remains the
	// separately governed fail-closed seam.
	return hookResponse{SystemMessage: reason}
}

func blockReason(result specguard.Result) string {
	if len(result.Findings) == 0 {
		return "Cooperative SPEC contract check unavailable: the provider-neutral guard returned an invalid blocking result"
	}
	parts := make([]string, 0, len(result.Findings))
	for _, finding := range result.Findings {
		part := finding.Code + ": " + finding.Message
		if finding.Path != "" {
			part = finding.Path + ": " + part
		}
		parts = append(parts, part)
	}
	return "Cooperative SPEC contract guard blocked this terminal hook. Fix the deterministic contract findings, then retry. Any mandatory immutable enforcement requires a separately reviewed changed-SPEC CI and provider rollout that this hook does not attest: " + strings.Join(parts, "; ")
}

func emitJSON(output io.Writer, response hookResponse) int {
	if output == nil {
		return 1
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return 1
	}
	encoded = append(encoded, '\n')
	if len(encoded) > maxHookOutputBytes {
		fallback := boundedHookResponse(response)
		if isLowerHexDigest(response.DearAgentSpecFeedbackID) {
			fallback.DearAgentSpecFeedbackID = response.DearAgentSpecFeedbackID
		}
		encoded, err = json.Marshal(fallback)
		if err != nil {
			return 1
		}
		encoded = append(encoded, '\n')
	}
	if len(encoded) > maxHookOutputBytes {
		return 1
	}
	written, err := output.Write(encoded)
	if err != nil || written != len(encoded) {
		return 1
	}
	return 0
}

func boundedHookResponse(response hookResponse) hookResponse {
	const blockMessage = "Cooperative SPEC contract check unavailable: hook response exceeded its safety limit; run the changed-SPEC CI and inspect the deterministic guard directly."
	const yieldMessage = "SPEC contract feedback exceeded its safety limit and was compacted; this cooperative hook is yielding rather than risking a terminal retry loop. Run the changed-SPEC CI and inspect the deterministic guard directly."

	switch {
	case response.Decision == "continue":
		return hookResponse{Decision: "continue", Reason: blockMessage}
	case response.Decision == "allow":
		return hookResponse{Decision: "allow"}
	case response.Decision == "" && response.HookSpecificOutput != nil:
		return hookResponse{HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     response.HookSpecificOutput.HookEventName,
			AdditionalContext: yieldMessage,
		}}
	case response.Decision == "" && response.SystemMessage != "":
		return hookResponse{SystemMessage: yieldMessage}
	default:
		fallback := hookResponse{Decision: "block", Reason: blockMessage}
		if response.SystemMessage != "" {
			fallback.SystemMessage = blockMessage
		}
		return fallback
	}
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

package context

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var portableTokenUsagePattern = regexp.MustCompile(`(?i)token usage:\s*([0-9][0-9,]*)\s*/\s*([0-9][0-9,]*)`)

type cliContextRoute struct {
	source          string
	defaultModel    string
	sessionEnv      []string
	usageEnv        []string
	modelEnv        []string
	messageCountEnv []string
}

var cliContextRoutes = map[CLI]cliContextRoute{
	CLIClaude: {
		source: "claude-cli", defaultModel: "claude-sonnet-4.5",
		sessionEnv:      []string{"CLAUDE_SESSION_ID"},
		usageEnv:        []string{"CLAUDE_CONTEXT_USAGE", "CLAUDE_TOOL_RESULT"},
		modelEnv:        []string{"CLAUDE_MODEL", "DEAR_AGENT_MODEL"},
		messageCountEnv: []string{"CLAUDE_MESSAGE_COUNT", "DEAR_AGENT_MESSAGE_COUNT"},
	},
	CLIGemini: {
		source: "gemini-cli", defaultModel: "gemini-3.5-flash",
		sessionEnv:      []string{"GEMINI_SESSION_ID"},
		usageEnv:        []string{"GEMINI_CONTEXT_USAGE", "GEMINI_TOOL_RESULT"},
		modelEnv:        []string{"GEMINI_MODEL", "DEAR_AGENT_MODEL"},
		messageCountEnv: []string{"GEMINI_MESSAGE_COUNT", "DEAR_AGENT_MESSAGE_COUNT"},
	},
	CLIOpenCode: {
		source: "opencode-cli", defaultModel: "z-ai/glm-5.2",
		sessionEnv:      []string{"OPENCODE_SESSION_ID"},
		usageEnv:        []string{"OPENCODE_CONTEXT_USAGE", "OPENCODE_TOOL_RESULT"},
		modelEnv:        []string{"OPENCODE_MODEL", "DEAR_AGENT_MODEL"},
		messageCountEnv: []string{"OPENCODE_MESSAGE_COUNT", "DEAR_AGENT_MESSAGE_COUNT"},
	},
	CLICodex: {
		source: "codex-cli", defaultModel: "gpt-5.5",
		sessionEnv:      []string{"CODEX_SESSION_ID"},
		usageEnv:        []string{"CODEX_CONTEXT_USAGE", "CODEX_TOOL_RESULT"},
		modelEnv:        []string{"CODEX_MODEL", "DEAR_AGENT_MODEL"},
		messageCountEnv: []string{"CODEX_MESSAGE_COUNT", "DEAR_AGENT_MESSAGE_COUNT"},
	},
	CLIPi: {
		source: "pi-cli", defaultModel: "claude-sonnet-4.6",
		sessionEnv:      []string{"PI_SESSION_ID"},
		usageEnv:        []string{"PI_CONTEXT_USAGE", "PI_TOOL_RESULT"},
		modelEnv:        []string{"PI_MODEL", "DEAR_AGENT_MODEL"},
		messageCountEnv: []string{"PI_MESSAGE_COUNT", "DEAR_AGENT_MESSAGE_COUNT"},
	},
	CLIAgy: {
		source: "agy", defaultModel: "gemini-2.5-flash",
		sessionEnv:      []string{"AGY_CONVERSATION_ID", "AGY_SESSION_ID", "ANTIGRAVITY_SESSION_ID"},
		usageEnv:        []string{"AGY_CONTEXT_USAGE", "ANTIGRAVITY_CONTEXT_USAGE"},
		modelEnv:        []string{"AGY_MODEL", "ANTIGRAVITY_MODEL", "DEAR_AGENT_MODEL"},
		messageCountEnv: []string{"AGY_MESSAGE_COUNT", "ANTIGRAVITY_MESSAGE_COUNT", "DEAR_AGENT_MESSAGE_COUNT"},
	},
}

func sessionIDForCLI(cli CLI) string {
	route, ok := cliContextRoutes[cli]
	if !ok {
		return ""
	}
	return firstEnvironmentValue(route.sessionEnv)
}

func (d *Detector) detectPortableSession(sessionID string, cli CLI) (*Usage, error) {
	route, ok := cliContextRoutes[cli]
	if !ok {
		return nil, fmt.Errorf("unsupported CLI type: %s", cli)
	}
	if sessionID == "" {
		sessionID = sessionIDForCLI(cli)
	}
	modelID := firstEnvironmentValue(route.modelEnv)
	if modelID == "" {
		modelID = route.defaultModel
	}
	if payload := firstEnvironmentValue(route.usageEnv); payload != "" {
		return d.parsePortableUsage(payload, sessionID, route.source, modelID)
	}

	messageCount := 50
	if raw := firstEnvironmentValue(route.messageCountEnv); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return nil, fmt.Errorf("invalid %s message count %q", route.source, raw)
		}
		messageCount = parsed
	}
	maxTokens := d.maxTokensForModel(modelID)
	usage := EstimateFromMessageCount(messageCount, maxTokens)
	usage.Source = route.source
	usage.ModelID = modelID
	usage.SessionID = sessionID
	usage.Estimated = true
	return usage, nil
}

func (d *Detector) parsePortableUsage(payload, sessionID, source, fallbackModel string) (*Usage, error) {
	used, total, modelID, ok := parseUsageJSON(payload)
	if !ok {
		matches := portableTokenUsagePattern.FindStringSubmatch(payload)
		if len(matches) != 3 {
			return nil, fmt.Errorf("%s context usage payload has no supported token counters", source)
		}
		var err error
		used, err = parseTokenInteger(matches[1])
		if err != nil {
			return nil, fmt.Errorf("parse %s used tokens: %w", source, err)
		}
		total, err = parseTokenInteger(matches[2])
		if err != nil {
			return nil, fmt.Errorf("parse %s total tokens: %w", source, err)
		}
	}
	if used < 0 || total <= 0 || used > total {
		return nil, fmt.Errorf("invalid %s context usage: used=%d total=%d", source, used, total)
	}
	if modelID == "" {
		modelID = fallbackModel
	}
	return &Usage{
		TotalTokens: total, UsedTokens: used,
		PercentageUsed: float64(used) / float64(total) * 100,
		LastUpdated:    time.Now(), Source: source, ModelID: modelID,
		SessionID: sessionID, Estimated: false,
	}, nil
}

func parseUsageJSON(payload string) (used, total int, modelID string, ok bool) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return 0, 0, "", false
	}
	usedNumber, usedOK := findJSONNumber(value, "used_tokens", "total_input_tokens", "input_tokens")
	totalNumber, totalOK := findJSONNumber(value, "total_tokens", "max_tokens", "context_window_size")
	if !usedOK || !totalOK {
		return 0, 0, "", false
	}
	used64, usedErr := strconv.ParseInt(usedNumber.String(), 10, strconv.IntSize)
	total64, totalErr := strconv.ParseInt(totalNumber.String(), 10, strconv.IntSize)
	if usedErr != nil || totalErr != nil {
		return 0, 0, "", false
	}
	modelID, _ = findJSONString(value, "model_id", "model")
	return int(used64), int(total64), modelID, true
}

func findJSONNumber(value any, keys ...string) (json.Number, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if number, ok := typed[key].(json.Number); ok {
				return number, true
			}
		}
		for _, key := range sortedJSONKeys(typed) {
			if number, ok := findJSONNumber(typed[key], keys...); ok {
				return number, true
			}
		}
	case []any:
		for _, child := range typed {
			if number, ok := findJSONNumber(child, keys...); ok {
				return number, true
			}
		}
	}
	return "", false
}

func findJSONString(value any, keys ...string) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if text, ok := typed[key].(string); ok {
				return text, true
			}
		}
		for _, key := range sortedJSONKeys(typed) {
			if text, ok := findJSONString(typed[key], keys...); ok {
				return text, true
			}
		}
	case []any:
		for _, child := range typed {
			if text, ok := findJSONString(child, keys...); ok {
				return text, true
			}
		}
	}
	return "", false
}

func sortedJSONKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseTokenInteger(raw string) (int, error) {
	return strconv.Atoi(strings.ReplaceAll(raw, ",", ""))
}

func (d *Detector) maxTokensForModel(modelID string) int {
	if d.registry != nil {
		if model := d.registry.GetModel(modelID); model != nil && model.MaxContextTokens > 0 {
			return model.MaxContextTokens
		}
	}
	return 200_000
}

func firstEnvironmentValue(names []string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

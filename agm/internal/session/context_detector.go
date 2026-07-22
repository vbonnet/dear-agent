package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/pisession"
)

// ConversationEntry represents a single entry in conversation.jsonl
type ConversationEntry struct {
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// conversationLogEntry represents the full JSONL entry from Claude Code's conversation log
type conversationLogEntry struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
	Data      json.RawMessage `json:"data"` // For "progress" type entries with nested assistant messages
}

// assistantMessage extracts usage and model from the message field
type assistantMessage struct {
	Model string       `json:"model"`
	Usage messageUsage `json:"usage"`
}

// progressData handles the nested format: {"type":"progress","data":{"message":{"type":"assistant","message":{...}}}}
type progressData struct {
	Message struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
	} `json:"message"`
}

// messageUsage represents the token usage from an API response
type messageUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// contextDetectorCache stores parsed context to avoid repeated file reads
type contextDetectorCache struct {
	SessionID      string
	ContextPercent float64
	LastModified   time.Time
	CachedAt       time.Time
}

// modelContextWindows maps model name prefixes to their context window sizes.
// More-specific prefixes must be checked before less-specific ones;
// getModelContextWindow uses longest-prefix matching to ensure correctness.
var modelContextWindows = map[string]int{
	// Opus 4.8+ and 4.6 have a 1M context window by default.
	"claude-opus-4-8": 1000000,
	"claude-opus-4-6": 1000000,
	"claude-opus-4":   200000,
	"claude-sonnet-4": 200000,
	"claude-haiku-4":  200000,
	"claude-3-5":      200000,
	"claude-3-opus":   200000,
	"claude-3-sonnet": 200000,
	"claude-3-haiku":  200000,
}

// statusLineDir is the directory where statusline JSON files are written.
// Override in tests with t.TempDir().
var statusLineDir = "/tmp/agm-context"

// statusLineStaleTTL is the maximum age of a statusline file before it is
// skipped so the conversation-log fallback can provide fresher data.
const statusLineStaleTTL = 30 * time.Second

var (
	// tokenUsageRegex matches: Token usage: 50000/200000; 150000 remaining
	tokenUsageRegex = regexp.MustCompile(`Token usage: (\d+)/(\d+)`)

	// Global cache (simple in-memory cache)
	detectorCache = make(map[string]*contextDetectorCache)
)

// statusLineFileData represents the JSON format written by the statusline command.
type statusLineFileData struct {
	SessionID     string `json:"session_id"`
	ContextWindow struct {
		UsedPercentage    float64 `json:"used_percentage"`
		ContextWindowSize int     `json:"context_window_size"`
		TotalInputTokens  int     `json:"total_input_tokens"`
		TotalOutputTokens int     `json:"total_output_tokens"`
	} `json:"context_window"`
	Cost struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	RateLimits struct {
		FiveHour struct {
			UsedPercentage float64 `json:"used_percentage"`
		} `json:"five_hour"`
	} `json:"rate_limits"`
}

// ReadStatusLineFile reads and parses the statusline JSON file for a given session ID.
// Returns the full parsed data including cost, model, and rate limit information.
func ReadStatusLineFile(sessionID string) (*statusLineFileData, error) {
	filePath := filepath.Join(statusLineDir, sessionID+".json")

	rawData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read statusline file: %w", err)
	}

	var data statusLineFileData
	if err := json.Unmarshal(rawData, &data); err != nil {
		return nil, fmt.Errorf("failed to parse statusline JSON: %w", err)
	}

	return &data, nil
}

// DetectContextFromStatusLine reads a statusline JSON file for the given session
// and returns context usage information.
//
// The statusline command writes JSON files to /tmp/agm-context/{sessionID}.json
// containing real-time context window data from Claude Code's status line API.
func DetectContextFromStatusLine(sessionID string) (*manifest.ContextUsage, error) {
	filePath := filepath.Join(statusLineDir, sessionID+".json")

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat statusline file: %w", err)
	}
	if time.Since(info.ModTime()) > statusLineStaleTTL {
		return nil, fmt.Errorf("statusline file stale (age %v > TTL %v)",
			time.Since(info.ModTime()).Round(time.Second), statusLineStaleTTL)
	}

	rawData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read statusline file: %w", err)
	}

	var data statusLineFileData
	if err := json.Unmarshal(rawData, &data); err != nil {
		return nil, fmt.Errorf("failed to parse statusline JSON: %w", err)
	}

	return &manifest.ContextUsage{
		TotalTokens:    data.ContextWindow.ContextWindowSize,
		UsedTokens:     int(math.Round(data.ContextWindow.UsedPercentage / 100.0 * float64(data.ContextWindow.ContextWindowSize))),
		PercentageUsed: data.ContextWindow.UsedPercentage,
		LastUpdated:    time.Now(),
		Source:         "statusline",
	}, nil
}

// isStatusLineFileFresh returns true if the statusline file for sessionID
// exists and was written within statusLineStaleTTL.
func isStatusLineFileFresh(sessionID string) bool {
	info, err := os.Stat(filepath.Join(statusLineDir, sessionID+".json"))
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) <= statusLineStaleTTL
}

// DetectContextFromConversationLog reads conversation.jsonl and extracts
// the most recent token usage from system reminders.
//
// This is a fallback mechanism when the PostToolUse hook hasn't updated
// the manifest. It reads the last N lines of the conversation log to find
// the most recent system reminder with token usage.
//
// Returns:
//   - ContextUsage with percentage if found
//   - nil if no token usage found
//   - error if file cannot be read
func DetectContextFromConversationLog(sessionID string) (*manifest.ContextUsage, error) {
	// Find conversation log path
	logPath, err := findConversationLog(sessionID)
	if err != nil {
		return nil, fmt.Errorf("conversation log not found: %w", err)
	}

	// Check cache
	if cached := getCachedContext(sessionID, logPath); cached != nil {
		return cached, nil
	}

	// Open file
	file, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open conversation log: %w", err)
	}
	defer file.Close()

	// Get file info for cache validation
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat conversation log: %w", err)
	}

	// Read last ~512KB of the conversation log.
	// Claude Code JSONL lines can be very large (100KB+ for tool outputs
	// with full file contents). 20KB was insufficient - a single long line
	// could push all assistant messages out of the tail window.
	// 512KB ensures we capture multiple assistant messages even with
	// verbose tool output interleaved.
	const tailSize = 512 * 1024
	fileSize := fileInfo.Size()
	startPos := int64(0)
	if fileSize > tailSize {
		startPos = fileSize - tailSize
	}

	// Seek to start position
	if _, err := file.Seek(startPos, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek in conversation log: %w", err)
	}

	lastTokenUsage, err := scanConversationLog(file)
	if err != nil {
		return nil, err
	}
	if lastTokenUsage == nil {
		return nil, fmt.Errorf("no token usage found in conversation log")
	}

	// Cache result
	cacheContext(sessionID, lastTokenUsage, fileInfo.ModTime())

	return lastTokenUsage, nil
}

// DetectContextFromManifestOrLog attempts to get context usage from manifest first,
// falling back to statusline API, then conversation log parsing if unavailable.
//
// This is the recommended detection strategy:
// 1. Check manifest.ContextUsage (updated by PostToolUse hook)
// 2. Try statusline file (written by statusline command)
// 3. If unavailable, parse conversation.jsonl
// 4. If still unavailable, return nil
// scanConversationLog reads JSONL lines from an open file and returns the most
// recent context usage, accumulating estimated cost across all entries.
func scanConversationLog(file *os.File) (*manifest.ContextUsage, error) {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)
	var lastTokenUsage *manifest.ContextUsage
	var cumulativeCost float64

	for scanner.Scan() {
		line := scanner.Text()

		if usage := extractUsageFromJSONL(line); usage != nil {
			lastTokenUsage = usage
			cumulativeCost += estimateCostFromUsage(usage.ModelID, line)
			continue
		}

		var entry ConversationEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type == "system_reminder" || containsTokenUsage(entry.Content) {
			if usage := extractTokenUsage(entry.Content); usage != nil {
				usage.LastUpdated = entry.Timestamp
				usage.Source = "conversation_log"
				lastTokenUsage = usage
			}
		}
	}

	if lastTokenUsage != nil {
		lastTokenUsage.EstimatedCost = cumulativeCost
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading conversation log: %w", err)
	}

	return lastTokenUsage, nil
}

// DetectContextFromManifestOrLog attempts to get context usage from manifest first,
// falling back to statusline API, then conversation log parsing if unavailable.
func DetectContextFromManifestOrLog(m *manifest.Manifest) (*manifest.ContextUsage, error) {
	// Try manifest first (preferred - hook-updated)
	if m.ContextUsage != nil && m.ContextUsage.PercentageUsed >= 0 {
		return m.ContextUsage, nil
	}

	if m.Pi != nil {
		return detectPiContext(m.Pi)
	}

	// Try statusline file
	if m.Claude.UUID != "" {
		if usage, err := DetectContextFromStatusLine(m.Claude.UUID); err == nil {
			return usage, nil
		}
	}

	// Fallback: parse conversation log
	if m.Claude.UUID != "" {
		if usage, err := DetectContextFromConversationLog(m.Claude.UUID); err == nil {
			return usage, nil
		}
	}

	return nil, fmt.Errorf("context usage unavailable from manifest or conversation log")
}

func detectPiContext(pi *manifest.Pi) (*manifest.ContextUsage, error) {
	if pi.SessionID == "" || pi.SessionDir == "" {
		return nil, fmt.Errorf("pi context usage requires native session metadata")
	}
	path, err := pisession.FindTranscript(pi.SessionDir, pi.SessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve Pi context transcript: %w", err)
	}
	if pi.TranscriptPath != "" {
		want, absErr := filepath.Abs(pi.TranscriptPath)
		if absErr != nil || filepath.Clean(want) != filepath.Clean(path) {
			return nil, fmt.Errorf("pi context transcript does not match persisted path")
		}
	}
	usage, err := pisession.ReadUsage(path)
	if err != nil {
		return nil, err
	}
	contextWindow := piModelContextWindow(usage.Model)
	percentage := math.Min(100, float64(usage.ContextTokens)/float64(contextWindow)*100)
	return &manifest.ContextUsage{
		TotalTokens:    contextWindow,
		UsedTokens:     usage.ContextTokens,
		PercentageUsed: percentage,
		LastUpdated:    usage.LastAssistantAt,
		Source:         "pi_jsonl",
		ModelID:        usage.Model,
		EstimatedCost:  usage.CumulativeCost,
	}, nil
}

const (
	piModelCatalogMaxBytes         = 1 << 20
	piModelCatalogMaxModels        = 4096
	piCustomModelDefaultContext    = 128000
	piModelCatalogMaxContextWindow = 16 * 1024 * 1024
)

type piModelCatalog struct {
	Providers map[string]piModelCatalogProvider `json:"providers"`
}

type piModelCatalogProvider struct {
	Models         []piModelCatalogModel             `json:"models"`
	ModelOverrides map[string]piModelCatalogOverride `json:"modelOverrides"`
}

type piModelCatalogModel struct {
	ID            string `json:"id"`
	ContextWindow *int   `json:"contextWindow"`
}

type piModelCatalogOverride struct {
	ContextWindow *int `json:"contextWindow"`
}

func piModelContextWindow(model string) int {
	if contextWindow, ok := piConfiguredModelContextWindow(model); ok {
		return contextWindow
	}
	return piNativeModelContextWindow(model)
}

func piNativeModelContextWindow(model string) int {
	if contextWindow, ok := piKnownNativeModelContextWindow(model); ok {
		return contextWindow
	}
	return 200000
}

func piKnownNativeModelContextWindow(model string) (int, bool) {
	model = strings.TrimPrefix(strings.ToLower(model), "anthropic/")
	model = strings.TrimPrefix(model, "openai/")
	model = strings.TrimPrefix(model, "google/")
	model = strings.TrimPrefix(model, "openrouter/")
	switch model {
	case "claude-fable-5", "claude-opus-4-8", "claude-sonnet-4-6":
		return 1000000, true
	case "gpt-5.3-chat-latest", "gpt-5.3-codex-spark":
		return 128000, true
	case "gpt-5.3-codex", "gpt-5.4-mini", "gpt-5.4-nano":
		return 400000, true
	case "gpt-5.4", "gpt-5.5", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna":
		return 272000, true
	case "gpt-5.4-pro", "gpt-5.5-pro":
		return 1050000, true
	case "gemini-3.5-flash", "gemini-3.1-flash-lite", "z-ai/glm-5.2", "deepseek/deepseek-v4-pro":
		return 1048576, true
	case "nvidia/nemotron-3-ultra-550b-a55b":
		return 512288, true
	case "qwen/qwen3.6-max-preview":
		return 262144, true
	default:
		return 0, false
	}
}

func piConfiguredModelContextWindow(model string) (int, bool) {
	catalog, ok := readPiModelCatalog()
	if !ok {
		return 0, false
	}
	provider, modelID, qualified := strings.Cut(strings.TrimSpace(model), "/")
	if qualified {
		configured, exists := catalog.Providers[provider]
		if !exists {
			return 0, false
		}
		return piProviderModelContextWindow(configured, modelID, piKnownNativeProvider(provider))
	}
	modelID = provider

	window, matched := 0, false
	for providerID, configured := range catalog.Providers {
		candidate, exists := piProviderModelContextWindow(configured, modelID, piKnownNativeProvider(providerID))
		if !exists {
			continue
		}
		if matched && candidate != window {
			return 0, false
		}
		window, matched = candidate, true
	}
	return window, matched
}

// piKnownNativeProvider mirrors Pi 0.81.1's built-in provider registry. Model
// catalogs refresh independently of AGM releases, so provider membership—not
// AGM's smaller static context-window table—is the durable boundary for
// deciding whether modelOverrides can target a native model.
func piKnownNativeProvider(provider string) bool {
	switch strings.TrimSpace(provider) {
	case "amazon-bedrock", "ant-ling", "anthropic", "azure-openai-responses",
		"cerebras", "cloudflare-ai-gateway", "cloudflare-workers-ai", "deepseek",
		"fireworks", "github-copilot", "google", "google-vertex", "groq",
		"huggingface", "kimi-coding", "minimax", "minimax-cn", "mistral",
		"moonshotai", "moonshotai-cn", "nvidia", "openai", "openai-codex",
		"opencode", "opencode-go", "openrouter", "qwen-token-plan",
		"qwen-token-plan-cn", "radius", "together", "vercel-ai-gateway", "xai",
		"xiaomi", "xiaomi-token-plan-ams", "xiaomi-token-plan-cn",
		"xiaomi-token-plan-sgp", "zai", "zai-coding-cn":
		return true
	default:
		return false
	}
}

func readPiModelCatalog() (piModelCatalog, bool) {
	path, err := piModelCatalogPath()
	if err != nil {
		return piModelCatalog{}, false
	}
	data, ok := readSafePiModelCatalog(path)
	if !ok {
		return piModelCatalog{}, false
	}
	return decodePiModelCatalog(data)
}

func readSafePiModelCatalog(path string) ([]byte, bool) {
	before, err := os.Lstat(path)
	if err != nil || !safePiModelCatalogFile(before) {
		return nil, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	pathAfterOpen, err := os.Lstat(path)
	if err != nil || !safePiModelCatalogFile(pathAfterOpen) || !os.SameFile(before, pathAfterOpen) {
		return nil, false
	}
	after, err := file.Stat()
	if err != nil || !safePiModelCatalogFile(after) || !os.SameFile(before, after) {
		return nil, false
	}

	data, err := io.ReadAll(io.LimitReader(file, piModelCatalogMaxBytes+1))
	if err != nil || len(data) > piModelCatalogMaxBytes {
		return nil, false
	}
	return data, true
}

func decodePiModelCatalog(data []byte) (piModelCatalog, bool) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var catalog piModelCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return piModelCatalog{}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return piModelCatalog{}, false
	}
	modelCount := 0
	for _, provider := range catalog.Providers {
		modelCount += len(provider.Models) + len(provider.ModelOverrides)
		if modelCount > piModelCatalogMaxModels {
			return piModelCatalog{}, false
		}
	}
	return catalog, true
}

func safePiModelCatalogFile(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink == 0 &&
		info.Mode().IsRegular() &&
		info.Mode().Perm()&0o022 == 0 &&
		info.Size() >= 0 && info.Size() <= piModelCatalogMaxBytes
}

func piModelCatalogPath() (string, error) {
	root := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".pi", "agent")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "models.json"), nil
}

func piProviderModelContextWindow(provider piModelCatalogProvider, modelID string, nativeProvider bool) (int, bool) {
	window, matched := 0, false
	for _, model := range provider.Models {
		if strings.TrimSpace(model.ID) != modelID {
			continue
		}
		candidate, ok := validPiModelContextWindow(model.ContextWindow, true)
		if !ok {
			return 0, false
		}
		// Pi upserts custom definitions in declaration order, so the last
		// duplicate is effective before modelOverrides are applied.
		window, matched = candidate, true
	}
	if override, ok := provider.ModelOverrides[modelID]; ok && override.ContextWindow != nil {
		if !matched && !nativeProvider {
			return 0, false
		}
		return validPiModelContextWindow(override.ContextWindow, false)
	}
	return window, matched
}

func validPiModelContextWindow(value *int, useDefault bool) (int, bool) {
	if value == nil {
		if useDefault {
			return piCustomModelDefaultContext, true
		}
		return 0, false
	}
	if *value <= 0 || *value > piModelCatalogMaxContextWindow {
		return 0, false
	}
	return *value, true
}

// findConversationLog locates the conversation log file for a session.
//
// Claude Code stores conversation logs at:
//
//	~/.claude/projects/{project-path-hash}/{sessionID}.jsonl
//
// where {project-path-hash} is a directory like "-home-user-src" (the project
// path with slashes replaced by dashes). We glob for the session ID across all
// project-path-hash directories first, then fall back to legacy paths.
func findConversationLog(sessionID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	// Primary: glob ~/.claude/projects/*/{sessionID}.jsonl
	globPattern := filepath.Join(homeDir, ".claude", "projects", "*", sessionID+".jsonl")
	matches, err := filepath.Glob(globPattern)
	if err == nil && len(matches) > 0 {
		return matches[0], nil
	}

	// Fallback: legacy paths (for future compatibility)
	fallbackPaths := []string{
		filepath.Join(homeDir, ".claude", "projects", sessionID, "conversation.jsonl"),
		filepath.Join(homeDir, ".claude", "sessions", sessionID, "conversation.jsonl"),
	}

	for _, path := range fallbackPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("conversation log not found for session %s", sessionID)
}

// containsTokenUsage checks if content contains token usage pattern
func containsTokenUsage(content string) bool {
	return tokenUsageRegex.MatchString(content)
}

// extractTokenUsage parses token usage from content
func extractTokenUsage(content string) *manifest.ContextUsage {
	matches := tokenUsageRegex.FindStringSubmatch(content)
	if len(matches) != 3 {
		return nil
	}

	used, err1 := strconv.Atoi(matches[1])
	total, err2 := strconv.Atoi(matches[2])

	if err1 != nil || err2 != nil || total == 0 {
		return nil
	}

	percentage := float64(used) / float64(total) * 100.0

	return &manifest.ContextUsage{
		TotalTokens:    total,
		UsedTokens:     used,
		PercentageUsed: percentage,
		LastUpdated:    time.Now(),
		Source:         "conversation_log",
	}
}

// extractUsageFromJSONL parses a JSONL line from Claude Code's conversation log
// and extracts token usage from assistant messages.
//
// Claude Code stores entries like:
//
//	{"type":"assistant","message":{"model":"claude-sonnet-4-5-20250929",
//	  "usage":{"input_tokens":12,"cache_creation_input_tokens":4014,
//	  "cache_read_input_tokens":23244,"output_tokens":1}}}
func extractUsageFromJSONL(line string) *manifest.ContextUsage {
	var entry conversationLogEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return nil
	}

	var msgRaw json.RawMessage

	switch entry.Type {
	case "assistant":
		// Format 1: {"type":"assistant","message":{"model":"...","usage":{...}}}
		if entry.Message == nil {
			return nil
		}
		msgRaw = entry.Message

	case "progress":
		// Format 2: {"type":"progress","data":{"message":{"type":"assistant","message":{"model":"...","usage":{...}}}}}
		// Some Claude Code versions nest assistant messages inside progress entries
		if entry.Data == nil {
			return nil
		}
		var pd progressData
		if err := json.Unmarshal(entry.Data, &pd); err != nil {
			return nil
		}
		if pd.Message.Type != "assistant" || pd.Message.Message == nil {
			return nil
		}
		msgRaw = pd.Message.Message

	default:
		return nil
	}

	var msg assistantMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return nil
	}

	// Total input tokens = input + cache_creation + cache_read
	totalInput := msg.Usage.InputTokens + msg.Usage.CacheCreationInputTokens + msg.Usage.CacheReadInputTokens
	if totalInput == 0 {
		return nil
	}

	// Get context window size from model name
	contextWindow := getModelContextWindow(msg.Model)
	if contextWindow == 0 {
		return nil
	}

	// Claude models have two context tiers: 200k (standard) and 1M (extended).
	// If observed tokens exceed standard, the session is using extended context.
	if totalInput > contextWindow && strings.Contains(msg.Model, "claude") {
		contextWindow = 1000000
	}

	percentage := float64(totalInput) / float64(contextWindow) * 100.0
	if percentage > 100.0 {
		percentage = 100.0
	}

	return &manifest.ContextUsage{
		TotalTokens:    contextWindow,
		UsedTokens:     totalInput,
		PercentageUsed: percentage,
		LastUpdated:    entry.Timestamp,
		Source:         "conversation_log",
		ModelID:        msg.Model,
	}
}

// getModelContextWindow returns the context window size for a given model name.
// Uses longest-prefix matching so that "claude-opus-4-6" (1M) takes precedence
// over "claude-opus-4" (200k) when the model ID starts with "claude-opus-4-6".
// Returns 0 if the model is unknown.
func getModelContextWindow(model string) int {
	bestLen := 0
	bestSize := 0
	for prefix, size := range modelContextWindows {
		if strings.HasPrefix(model, prefix) && len(prefix) > bestLen {
			bestLen = len(prefix)
			bestSize = size
		}
	}
	if bestSize > 0 {
		return bestSize
	}
	// Default: assume 200K for any claude model
	if strings.Contains(model, "claude") {
		return 200000
	}
	return 0
}

// modelPricing holds per-million-token pricing for cost estimation.
type modelPricing struct {
	InputPerM      float64
	OutputPerM     float64
	CacheReadPerM  float64
	CacheWritePerM float64
}

// pricingTable maps model prefixes to their pricing (USD per million tokens).
// Uses longest-prefix matching via getModelPricing.
//
// Fallback for non-interactive sessions (e.g., -p mode) where statusLine
// doesn't fire. For interactive sessions, the exact cost comes from CC's
// statusLine JSON (total_cost_usd) via agm-statusline-capture.
var pricingTable = map[string]modelPricing{
	"claude-opus-4":   {InputPerM: 15.0, OutputPerM: 75.0, CacheReadPerM: 1.50, CacheWritePerM: 18.75},
	"claude-sonnet-4": {InputPerM: 3.0, OutputPerM: 15.0, CacheReadPerM: 0.30, CacheWritePerM: 3.75},
	"claude-haiku-4":  {InputPerM: 0.80, OutputPerM: 4.0, CacheReadPerM: 0.08, CacheWritePerM: 1.0},
}

// getModelPricing returns pricing for a model using longest-prefix matching.
func getModelPricing(modelID string) (modelPricing, bool) {
	bestLen := 0
	var bestPricing modelPricing
	for prefix, p := range pricingTable {
		if strings.HasPrefix(modelID, prefix) && len(prefix) > bestLen {
			bestLen = len(prefix)
			bestPricing = p
		}
	}
	return bestPricing, bestLen > 0
}

// estimateCostFromUsage parses a JSONL line for token counts and estimates cost.
//
// Fallback for non-interactive sessions (e.g., -p mode) where statusLine
// doesn't fire. For interactive sessions, the exact cost comes from CC's
// statusLine JSON (total_cost_usd) via agm-statusline-capture.
func estimateCostFromUsage(modelID, line string) float64 {
	if modelID == "" {
		return 0
	}
	pricing, ok := getModelPricing(modelID)
	if !ok {
		return 0
	}
	// Re-parse the line to get raw usage (extractUsageFromJSONL already parsed it,
	// but we need the individual token fields for cost).
	var entry conversationLogEntry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return 0
	}
	var msgRaw json.RawMessage
	switch entry.Type {
	case "assistant":
		msgRaw = entry.Message
	case "progress":
		var pd progressData
		if err := json.Unmarshal(entry.Data, &pd); err != nil {
			return 0
		}
		if pd.Message.Type != "assistant" {
			return 0
		}
		msgRaw = pd.Message.Message
	default:
		return 0
	}
	if msgRaw == nil {
		return 0
	}
	var msg assistantMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return 0
	}
	cost := float64(msg.Usage.InputTokens) / 1e6 * pricing.InputPerM
	cost += float64(msg.Usage.OutputTokens) / 1e6 * pricing.OutputPerM
	cost += float64(msg.Usage.CacheReadInputTokens) / 1e6 * pricing.CacheReadPerM
	cost += float64(msg.Usage.CacheCreationInputTokens) / 1e6 * pricing.CacheWritePerM
	return cost
}

// getCachedContext retrieves cached context if still valid
func getCachedContext(sessionID string, logPath string) *manifest.ContextUsage {
	cached, exists := detectorCache[sessionID]
	if !exists {
		return nil
	}

	// Check if file has been modified since cache
	fileInfo, err := os.Stat(logPath)
	if err != nil {
		return nil
	}

	// Cache valid if BOTH conditions are true:
	// 1. Cache is less than 30 seconds old
	// 2. File hasn't been modified since cache
	cacheAge := time.Since(cached.CachedAt)
	fileUnchanged := !fileInfo.ModTime().After(cached.LastModified)

	if cacheAge < 30*time.Second && fileUnchanged {
		return &manifest.ContextUsage{
			PercentageUsed: cached.ContextPercent,
			LastUpdated:    cached.CachedAt,
			Source:         "conversation_log_cached",
		}
	}

	return nil
}

// cacheContext stores parsed context for future lookups
func cacheContext(sessionID string, usage *manifest.ContextUsage, fileModTime time.Time) {
	detectorCache[sessionID] = &contextDetectorCache{
		SessionID:      sessionID,
		ContextPercent: usage.PercentageUsed,
		LastModified:   fileModTime,
		CachedAt:       time.Now(),
	}
}

// ClearDetectorCache clears the context detector cache (useful for testing)
func ClearDetectorCache() {
	detectorCache = make(map[string]*contextDetectorCache)
}

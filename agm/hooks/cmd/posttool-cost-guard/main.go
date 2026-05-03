package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/costtrack"
	"github.com/vbonnet/dear-agent/pkg/otelsetup"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	warnThreshold     = 50.0
	criticalThreshold = 100.0
	sampleInterval    = 10
)

// HookInput is the JSON structure received from Claude Code on stdin.
type HookInput struct {
	SessionID   string `json:"session_id"`
	ToolName    string `json:"tool_name"`
	Cwd         string `json:"cwd"`
	Traceparent string `json:"traceparent,omitempty"`
	Tracestate  string `json:"tracestate,omitempty"`
}

// Usage holds token counts from a JSONL message entry.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// Message is the nested message object inside a JSONL entry.
type Message struct {
	Model string `json:"model"`
	Usage *Usage `json:"usage"`
}

// JournalEntry is a single line from the session JSONL file.
type JournalEntry struct {
	Type    string  `json:"type"`
	Message Message `json:"message"`
}

// traceContextFromHook extracts W3C trace context from the hook input,
// falling back to the TRACEPARENT environment variable.
func traceContextFromHook(traceparent string) context.Context {
	ctx := context.Background()
	tp := traceparent
	if tp == "" {
		tp = os.Getenv("TRACEPARENT")
	}
	if tp != "" {
		carrier := propagation.MapCarrier{"traceparent": tp}
		ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	}
	return ctx
}

func main() {
	shutdown := otelsetup.InitTracer("posttool-cost-guard")
	exitCode := run()
	_ = shutdown(context.Background())
	os.Exit(exitCode)
}

func run() int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan int, 1)
	go func() {
		done <- runHook(os.Stdin, os.Stderr)
	}()

	select {
	case code := <-done:
		return code
	case <-ctx.Done():
		return 0 // timeout — exit silently
	}
}

func runHook(stdin *os.File, stderr *os.File) int {
	// Read all input for trace context extraction.
	inputBytes, err := io.ReadAll(stdin)
	if err != nil {
		return 0 // fail open
	}

	// Parse hook input.
	var input HookInput
	if err := json.Unmarshal(inputBytes, &input); err != nil {
		return 0 // fail open
	}

	// Create trace span for hook execution.
	ctx := traceContextFromHook(input.Traceparent)
	tracer := otel.Tracer("ai-tools/hooks")
	_, span := tracer.Start(ctx, "hook_execution",
		trace.WithAttributes(
			attribute.String("hook.name", "posttool-cost-guard"),
			attribute.String("hook.type", "PostToolUse"),
		),
	)
	defer func() {
		span.SetAttributes(attribute.Int("hook.exit_code", 0))
		span.End()
	}()

	if input.SessionID == "" {
		return 0
	}

	// Sampling: only run full check every Nth call.
	if !shouldCheck(input.SessionID) {
		return 0
	}

	// Find the session JSONL file.
	jsonlPath := findJSONLPath(input.SessionID, input.Cwd)
	if jsonlPath == "" {
		return 0
	}

	// Compute cost.
	cost, err := computeSessionCost(jsonlPath)
	if err != nil {
		return 0 // fail open
	}

	// Emit warnings to stderr.
	if cost >= criticalThreshold {
		fmt.Fprintf(stderr, "[cost-guard] SESSION COST: $%.2f - APPROACHING LIMIT (critical threshold: $%d)\n",
			cost, int(criticalThreshold))
	} else if cost >= warnThreshold {
		fmt.Fprintf(stderr, "[cost-guard] Session cost: $%.2f (warning threshold: $%d)\n",
			cost, int(warnThreshold))
	}

	return 0
}

// shouldCheck increments a per-session counter file and returns true every sampleInterval calls.
func shouldCheck(sessionID string) bool {
	counterPath := fmt.Sprintf("/tmp/cost-guard-%s.count", sessionID)

	count := 0
	data, err := os.ReadFile(counterPath)
	if err == nil {
		count, _ = strconv.Atoi(strings.TrimSpace(string(data)))
	}

	count++
	// Best-effort write; ignore errors.
	_ = os.WriteFile(counterPath, []byte(strconv.Itoa(count)), 0o644)

	return count%sampleInterval == 0
}

// findJSONLPath locates the session JSONL file under ~/.claude/projects/.
func findJSONLPath(sessionID, cwd string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Derive project name from cwd: replace / with - and trim leading -.
	projectName := strings.ReplaceAll(cwd, "/", "-")
	projectName = strings.TrimPrefix(projectName, "-")

	candidate := filepath.Join(homeDir, ".claude", "projects", projectName, sessionID+".jsonl")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// Fallback: scan project directories for the session file.
	projectsDir := filepath.Join(homeDir, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate = filepath.Join(projectsDir, entry.Name(), sessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

// computeSessionCost reads the JSONL file and sums costs across all assistant messages.
func computeSessionCost(jsonlPath string) (float64, error) {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var totalCost float64
	scanner := bufio.NewScanner(f)
	// Allow large lines (some JSONL entries can be big).
	scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry JournalEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type != "assistant" || entry.Message.Usage == nil {
			continue
		}

		pricing, ok := costtrack.GetPricing(entry.Message.Model)
		if !ok {
			continue
		}

		u := entry.Message.Usage
		totalCost += float64(u.InputTokens) * pricing.Input
		totalCost += float64(u.OutputTokens) * pricing.Output
		totalCost += float64(u.CacheReadInputTokens) * pricing.CacheRead
		totalCost += float64(u.CacheCreationInputTokens) * pricing.CacheWrite
	}

	return totalCost, scanner.Err()
}

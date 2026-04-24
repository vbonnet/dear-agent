// stop-retrospect mines the conversation history for undone work, broken
// promises, and dropped tasks. Advisory only — always exits 0.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/stophook"
)

func main() {
	os.Exit(stophook.RunWithTimeout(10*time.Second, run))
}

func run() int {
	input, err := stophook.ReadInput(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[stop-retrospect] failed to read input: %v\n", err)
		return 0
	}

	result := &stophook.Result{HookName: "stop-retrospect"}

	jsonlPath := input.TranscriptPath
	if jsonlPath == "" {
		jsonlPath = findConversationFile(input.Cwd, input.SessionID)
	}

	if jsonlPath == "" {
		result.Pass("conversation", "no conversation file found, skipped")
		result.Report()
		return 0
	}

	findings := analyzeConversation(jsonlPath)

	if findings.undoneCount > 0 {
		result.Warn("undone-tasks",
			fmt.Sprintf("%d potential undone task(s) detected", findings.undoneCount),
			strings.Join(findings.undoneExamples, "; "))
	} else {
		result.Pass("undone-tasks", "no undone tasks detected")
	}

	if findings.brokenPromises > 0 {
		result.Warn("broken-promises",
			fmt.Sprintf("%d potential broken promise(s)", findings.brokenPromises),
			strings.Join(findings.promiseExamples, "; "))
	} else {
		result.Pass("broken-promises", "no broken promises detected")
	}

	if findings.failedTools > 0 {
		result.Warn("failed-tools",
			fmt.Sprintf("%d tool error(s) not retried", findings.failedTools),
			"review failed tool calls")
	} else {
		result.Pass("failed-tools", "no unretried tool errors")
	}

	result.Report()
	return 0 // always advisory
}

type conversationFindings struct {
	undoneCount     int
	undoneExamples  []string
	brokenPromises  int
	promiseExamples []string
	failedTools     int
}

// Promise patterns: "I'll", "I will", "Let me", "I'm going to"
var promisePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bI'll\s+(\w+(?:\s+\w+){0,5})`),
	regexp.MustCompile(`(?i)\bI will\s+(\w+(?:\s+\w+){0,5})`),
	regexp.MustCompile(`(?i)\bLet me\s+(\w+(?:\s+\w+){0,5})`),
	regexp.MustCompile(`(?i)\bI'm going to\s+(\w+(?:\s+\w+){0,5})`),
}

// jsonlEntry is a minimal parse of conversation JSONL lines.
type jsonlEntry struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type messageContent struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func analyzeConversation(path string) conversationFindings {
	var findings conversationFindings

	f, err := os.Open(path)
	if err != nil {
		return findings
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)

	var promises []string
	lineCount := 0
	maxLines := 500 // limit for performance

	for scanner.Scan() && lineCount < maxLines {
		lineCount++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "assistant":
			text := extractText(entry.Message)
			for _, pat := range promisePatterns {
				matches := pat.FindAllStringSubmatch(text, -1)
				for _, m := range matches {
					if len(m) > 1 {
						promises = append(promises, m[0])
					}
				}
			}
		case "tool_error":
			findings.failedTools++
		}
	}

	// Simple heuristic: promises in last 30% of conversation are more likely undone
	threshold := len(promises) * 7 / 10
	if threshold < 0 {
		threshold = 0
	}
	latePromises := promises[threshold:]

	findings.brokenPromises = len(latePromises)
	for i, p := range latePromises {
		if i >= 3 {
			break
		}
		findings.promiseExamples = append(findings.promiseExamples, p)
	}

	return findings
}

func extractText(raw json.RawMessage) string {
	var msg messageContent
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}

	// Try as plain string
	var text string
	if err := json.Unmarshal(msg.Content, &text); err == nil {
		return text
	}

	// Try as array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}

	return ""
}

func findConversationFile(cwd, sessionID string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	projectsDir := filepath.Join(home, ".claude", "projects")

	// If we have a session ID, search for it directly
	if sessionID != "" {
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			return ""
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			candidate := filepath.Join(projectsDir, e.Name(), sessionID+".jsonl")
			if stophook.FileExists(candidate) {
				return candidate
			}
		}
	}

	// Fallback: find most recent JSONL in projects dir
	var newest string
	var newestTime time.Time

	_ = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".jsonl") && info.ModTime().After(newestTime) {
			newest = path
			newestTime = info.ModTime()
		}
		return nil
	})

	return newest
}

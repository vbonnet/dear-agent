package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

var (
	ErrEmptyTranscript = errors.New("transcript has no extractable content")
	ErrNothingNew      = errors.New("no new skill — all procedures already covered")
)

// Config holds runtime parameters for the extraction.
type Config struct {
	SessionID   string // empty = use latest transcript
	SkillsDir   string
	ProjectsDir string
	OutputDir   string
	DryRun      bool
}

// ExtractionResult is the parsed model output.
type ExtractionResult struct {
	SkillName  string
	Replaces   string
	WhyNew     string
	Content    string // full SKILL.md content
	OutputPath string // set after writing (empty in dry-run)
}

// ModelCaller abstracts the LLM call so tests can inject a fake.
type ModelCaller interface {
	Call(ctx context.Context, prompt string) (string, error)
}

// Extract is the main entry point: finds the transcript, lists existing skills,
// calls the model, and (unless dry-run) writes the candidate file.
func Extract(ctx context.Context, cfg Config, caller ModelCaller) (*ExtractionResult, error) {
	transcript, err := loadTranscript(cfg.ProjectsDir, cfg.SessionID)
	if err != nil {
		return nil, fmt.Errorf("load transcript: %w", err)
	}
	if strings.TrimSpace(transcript) == "" {
		return nil, ErrEmptyTranscript
	}

	skills, err := listSkillNames(cfg.SkillsDir)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}

	prompt := buildPrompt(transcript, skills)

	raw, err := caller.Call(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("model call: %w", err)
	}

	result, err := parseResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse model response: %w", err)
	}

	if !cfg.DryRun {
		if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
			return nil, fmt.Errorf("create output dir: %w", err)
		}
		name := sanitizeFilename(result.SkillName)
		if name == "" {
			name = "skill-candidate"
		}
		path := filepath.Join(cfg.OutputDir, name+".md")
		if err := os.WriteFile(path, []byte(result.Content), 0o600); err != nil {
			return nil, fmt.Errorf("write skill file: %w", err)
		}
		result.OutputPath = path
	}

	return result, nil
}

// loadTranscript finds the JSONL file and returns a condensed text summary
// of the session suitable for the extraction prompt.
func loadTranscript(projectsDir, sessionID string) (string, error) {
	path, err := findTranscriptPath(projectsDir, sessionID)
	if err != nil {
		return "", err
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var parts []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		msgType := jsonStr(entry["type"])
		switch msgType {
		case "queue-operation":
			if op := jsonStr(entry["operation"]); op == "enqueue" {
				if c := jsonStr(entry["content"]); c != "" {
					parts = append(parts, "[TASK] "+c)
				}
			}
		case "user":
			text := extractMessageText(entry["message"])
			if text != "" {
				parts = append(parts, "[USER] "+text)
			}
		case "assistant":
			text := extractMessageText(entry["message"])
			if text != "" {
				parts = append(parts, "[ASSISTANT] "+text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	// Trim to ~20 000 chars to keep well within context limits.
	combined := strings.Join(parts, "\n\n")
	const maxChars = 20000
	if len(combined) > maxChars {
		combined = combined[:maxChars] + "\n[...truncated...]"
	}
	return combined, nil
}

// findTranscriptPath locates the JSONL file by session ID or (if empty) the
// most recently modified JSONL across all project subdirectories.
func findTranscriptPath(projectsDir, sessionID string) (string, error) {
	if sessionID != "" {
		var found string
		err := filepath.WalkDir(projectsDir, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return walkErr
			}
			if d.Name() == sessionID+".jsonl" {
				found = p
			}
			return nil
		})
		if err != nil {
			return "", err
		}
		if found == "" {
			return "", fmt.Errorf("session %q not found under %s", sessionID, projectsDir)
		}
		return found, nil
	}

	var (
		latestPath string
		latestTime time.Time
	)
	err := filepath.WalkDir(projectsDir, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestPath = p
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if latestPath == "" {
		return "", fmt.Errorf("no .jsonl transcripts found under %s", projectsDir)
	}
	return latestPath, nil
}

// extractMessageText pulls plain text from a `message` JSON object (which may
// have a string or array-of-blocks `content` field).
func extractMessageText(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}
	if len(msg.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return ""
	}
	var texts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			texts = append(texts, b.Text)
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n"))
}

// listSkillNames returns the names of all entries in skillsDir.
func listSkillNames(skillsDir string) ([]string, error) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

// buildPrompt constructs the extraction prompt sent to the model.
func buildPrompt(transcript string, existingSkills []string) string {
	skillList := "(none)"
	if len(existingSkills) > 0 {
		skillList = "- " + strings.Join(existingSkills, "\n- ")
	}

	return fmt.Sprintf(`You are analyzing a completed Claude Code session transcript to extract a reusable SKILL candidate.

EXISTING SKILLS (do NOT propose anything already covered by these):
%s

SESSION TRANSCRIPT:
%s

TASK:
Identify the single most reusable procedure this session demonstrates that is NOT already covered by the listed skills. A reusable procedure is a repeatable, step-by-step workflow that future sessions could invoke as a named skill.

ANTI-PATTERN GUARD: Also identify what existing skill or CLAUDE.md section this new skill could reduce or replace. If it would reduce redundancy, name it explicitly.

If you find something worth extracting, respond in this EXACT format (no extra text before or after):

SKILL_NAME: <kebab-case-name>
REPLACES: <existing skill name or CLAUDE.md section this reduces/replaces, or "nothing">
WHY_NEW: <one sentence explaining why this isn't already covered>

---SKILL.md---
---
name: <kebab-case-name>
description: >
  <one-line description of when to invoke this skill>
---

# <Title>

<Skill content here — step-by-step procedure, conditions, examples>
---END---

If nothing new is worth extracting (all procedures are already covered), respond with exactly this single word:
NOTHING_NEW`, skillList, transcript)
}

// parseResponse extracts the structured fields from the model's raw reply.
func parseResponse(raw string) (*ExtractionResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "NOTHING_NEW" {
		return nil, ErrNothingNew
	}

	result := &ExtractionResult{}

	lines := strings.Split(raw, "\n")
	var i int
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if after, ok := strings.CutPrefix(line, "SKILL_NAME:"); ok {
			result.SkillName = strings.TrimSpace(after)
		} else if after, ok := strings.CutPrefix(line, "REPLACES:"); ok {
			result.Replaces = strings.TrimSpace(after)
		} else if after, ok := strings.CutPrefix(line, "WHY_NEW:"); ok {
			result.WhyNew = strings.TrimSpace(after)
		} else if line == "---SKILL.md---" {
			i++
			break
		}
		i++
	}

	var skillLines []string
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) == "---END---" {
			break
		}
		skillLines = append(skillLines, lines[i])
		i++
	}
	result.Content = strings.Join(skillLines, "\n")

	if result.SkillName == "" {
		return nil, fmt.Errorf("model response missing SKILL_NAME; got: %.200s", raw)
	}
	if result.Content == "" {
		return nil, fmt.Errorf("model response missing SKILL.md content; got: %.200s", raw)
	}

	return result, nil
}

// sanitizeFilename converts a skill name to a safe filename component.
func sanitizeFilename(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// jsonStr safely decodes a json.RawMessage as a string.
func jsonStr(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// anthropicCaller calls the real Anthropic API.
type anthropicCaller struct {
	client anthropic.Client
}

func newAnthropicCaller(apiKey string) *anthropicCaller {
	return &anthropicCaller{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

func (c *anthropicCaller) Call(ctx context.Context, prompt string) (string, error) {
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("anthropic messages.new: %w", err)
	}
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("model returned empty response")
	}
	return strings.TrimSpace(resp.Content[0].AsText().Text), nil
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQuestionFilename(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	got := QuestionFilename("my-session", ts)
	want := "my-session-1775736000000.md"
	if got != want {
		t.Errorf("QuestionFilename() = %q, want %q", got, want)
	}
}

func TestWriteQuestion(t *testing.T) {
	// Override home dir via questions dir
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	now := time.Date(2026, 4, 9, 14, 30, 0, 0, time.UTC)
	q := &Question{
		Session:   "test-session",
		Sender:    "worker-1",
		Timestamp: now,
		Text:      "Should I use v2 or v3 API?",
		Context:   "The v3 API has better error handling",
		Status:    "pending",
	}

	filePath, err := WriteQuestion(q)
	if err != nil {
		t.Fatalf("WriteQuestion() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("question file not created at %s", filePath)
	}

	// Verify content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read question file: %v", err)
	}

	contentStr := string(content)

	// Check frontmatter
	if !strings.Contains(contentStr, "session: test-session") {
		t.Error("missing session in frontmatter")
	}
	if !strings.Contains(contentStr, "sender: worker-1") {
		t.Error("missing sender in frontmatter")
	}
	if !strings.Contains(contentStr, "status: pending") {
		t.Error("missing status in frontmatter")
	}
	if !strings.Contains(contentStr, "timestamp: 2026-04-09T14:30:00Z") {
		t.Error("missing timestamp in frontmatter")
	}

	// Check body
	if !strings.Contains(contentStr, "## Question") {
		t.Error("missing question header")
	}
	if !strings.Contains(contentStr, "Should I use v2 or v3 API?") {
		t.Error("missing question text")
	}
	if !strings.Contains(contentStr, "## Context") {
		t.Error("missing context header")
	}
	if !strings.Contains(contentStr, "The v3 API has better error handling") {
		t.Error("missing context text")
	}
}

func TestWriteQuestionWithoutContext(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	now := time.Date(2026, 4, 9, 14, 30, 0, 0, time.UTC)
	q := &Question{
		Session:   "test-session",
		Sender:    "worker-1",
		Timestamp: now,
		Text:      "Simple question?",
		Status:    "pending",
	}

	filePath, err := WriteQuestion(q)
	if err != nil {
		t.Fatalf("WriteQuestion() error = %v", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read question file: %v", err)
	}

	contentStr := string(content)

	// Should NOT have context section
	if strings.Contains(contentStr, "## Context") {
		t.Error("context header should not be present when context is empty")
	}
}

func TestParseQuestion(t *testing.T) {
	content := `---
session: my-session
sender: worker-1
timestamp: 2026-04-09T14:30:00Z
status: pending
---

## Question

Should I use v2 or v3 API?

## Context

The v3 API has better error handling
`

	q, err := ParseQuestion(content)
	if err != nil {
		t.Fatalf("ParseQuestion() error = %v", err)
	}

	if q.Session != "my-session" {
		t.Errorf("Session = %q, want %q", q.Session, "my-session")
	}
	if q.Sender != "worker-1" {
		t.Errorf("Sender = %q, want %q", q.Sender, "worker-1")
	}
	if q.Status != "pending" {
		t.Errorf("Status = %q, want %q", q.Status, "pending")
	}
	if q.Text != "Should I use v2 or v3 API?" {
		t.Errorf("Text = %q, want %q", q.Text, "Should I use v2 or v3 API?")
	}
	if q.Context != "The v3 API has better error handling" {
		t.Errorf("Context = %q, want %q", q.Context, "The v3 API has better error handling")
	}

	expectedTime := time.Date(2026, 4, 9, 14, 30, 0, 0, time.UTC)
	if !q.Timestamp.Equal(expectedTime) {
		t.Errorf("Timestamp = %v, want %v", q.Timestamp, expectedTime)
	}
}

func TestParseQuestionWithoutContext(t *testing.T) {
	content := `---
session: my-session
sender: worker-1
timestamp: 2026-04-09T14:30:00Z
status: pending
---

## Question

Simple question?
`

	q, err := ParseQuestion(content)
	if err != nil {
		t.Fatalf("ParseQuestion() error = %v", err)
	}

	if q.Text != "Simple question?" {
		t.Errorf("Text = %q, want %q", q.Text, "Simple question?")
	}
	if q.Context != "" {
		t.Errorf("Context = %q, want empty", q.Context)
	}
}

func TestParseQuestionInvalidFormat(t *testing.T) {
	content := "no frontmatter here"

	_, err := ParseQuestion(content)
	if err == nil {
		t.Error("ParseQuestion() should return error for invalid format")
	}
}

func TestFindPendingQuestion(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	questionsDir := filepath.Join(tmpDir, ".agm", "questions")
	if err := os.MkdirAll(questionsDir, 0o755); err != nil {
		t.Fatalf("failed to create questions dir: %v", err)
	}

	// Write an older pending question
	older := `---
session: my-session
sender: worker-1
timestamp: 2026-04-09T10:00:00Z
status: pending
---

## Question

Older question?
`
	if err := os.WriteFile(filepath.Join(questionsDir, "my-session-1744189200000.md"), []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a newer pending question
	newer := `---
session: my-session
sender: worker-1
timestamp: 2026-04-09T14:00:00Z
status: pending
---

## Question

Newer question?
`
	if err := os.WriteFile(filepath.Join(questionsDir, "my-session-1744203600000.md"), []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write an answered question (should be skipped)
	answered := `---
session: my-session
sender: worker-1
timestamp: 2026-04-09T15:00:00Z
status: answered
---

## Question

Already answered?
`
	if err := os.WriteFile(filepath.Join(questionsDir, "my-session-1744207200000.md"), []byte(answered), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a question for a different session (should be skipped)
	otherSession := `---
session: other-session
sender: worker-2
timestamp: 2026-04-09T16:00:00Z
status: pending
---

## Question

Wrong session?
`
	if err := os.WriteFile(filepath.Join(questionsDir, "other-session-1744210800000.md"), []byte(otherSession), 0o644); err != nil {
		t.Fatal(err)
	}

	// Find pending question - should return the newer one
	path, q, err := FindPendingQuestion("my-session")
	if err != nil {
		t.Fatalf("FindPendingQuestion() error = %v", err)
	}

	if q.Text != "Newer question?" {
		t.Errorf("got question %q, want %q", q.Text, "Newer question?")
	}

	if !strings.HasSuffix(path, "my-session-1744203600000.md") {
		t.Errorf("got path %q, want suffix %q", path, "my-session-1744203600000.md")
	}
}

func TestFindPendingQuestionNoneFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	questionsDir := filepath.Join(tmpDir, ".agm", "questions")
	if err := os.MkdirAll(questionsDir, 0o755); err != nil {
		t.Fatalf("failed to create questions dir: %v", err)
	}

	_, _, err := FindPendingQuestion("my-session")
	if err == nil {
		t.Error("FindPendingQuestion() should return error when no pending questions")
	}
	if !strings.Contains(err.Error(), "no pending questions") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "no pending questions")
	}
}

func TestFindPendingQuestionNoDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Don't create the questions directory
	_, _, err := FindPendingQuestion("my-session")
	if err == nil {
		t.Error("FindPendingQuestion() should return error when directory doesn't exist")
	}
}

func TestMarkQuestionAnswered(t *testing.T) {
	tmpDir := t.TempDir()

	content := `---
session: my-session
sender: worker-1
timestamp: 2026-04-09T14:30:00Z
status: pending
---

## Question

Should I use v2 or v3 API?
`
	filePath := filepath.Join(tmpDir, "my-session-1744206600000.md")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	answer := "Use v3 API, it has better error handling."
	if err := MarkQuestionAnswered(filePath, answer); err != nil {
		t.Fatalf("MarkQuestionAnswered() error = %v", err)
	}

	// Read back and verify
	updated, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	updatedStr := string(updated)

	if strings.Contains(updatedStr, "status: pending") {
		t.Error("status should no longer be 'pending'")
	}
	if !strings.Contains(updatedStr, "status: answered") {
		t.Error("status should be 'answered'")
	}
	if !strings.Contains(updatedStr, "## Answer") {
		t.Error("should contain answer header")
	}
	if !strings.Contains(updatedStr, "Use v3 API, it has better error handling.") {
		t.Error("should contain answer text")
	}
}

func TestWriteAndFindRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	now := time.Now()
	q := &Question{
		Session:   "roundtrip-session",
		Sender:    "test-worker",
		Timestamp: now,
		Text:      "Round trip question?",
		Context:   "Testing round trip",
		Status:    "pending",
	}

	_, err := WriteQuestion(q)
	if err != nil {
		t.Fatalf("WriteQuestion() error = %v", err)
	}

	// Find it back
	_, found, err := FindPendingQuestion("roundtrip-session")
	if err != nil {
		t.Fatalf("FindPendingQuestion() error = %v", err)
	}

	if found.Session != "roundtrip-session" {
		t.Errorf("Session = %q, want %q", found.Session, "roundtrip-session")
	}
	if found.Sender != "test-worker" {
		t.Errorf("Sender = %q, want %q", found.Sender, "test-worker")
	}
	if found.Text != "Round trip question?" {
		t.Errorf("Text = %q, want %q", found.Text, "Round trip question?")
	}
	if found.Context != "Testing round trip" {
		t.Errorf("Context = %q, want %q", found.Context, "Testing round trip")
	}
}

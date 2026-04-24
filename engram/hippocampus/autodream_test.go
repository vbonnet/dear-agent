package hippocampus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockHarness implements HarnessAdapter for testing.
type mockHarness struct {
	sessions    []SessionInfo
	transcripts map[string]string
	memoryDir   string
}

func (m *mockHarness) Name() string { return "mock" }

func (m *mockHarness) DiscoverSessions(_ context.Context, _ string, _ time.Time) ([]SessionInfo, error) {
	return m.sessions, nil
}

func (m *mockHarness) ReadTranscript(_ context.Context, session SessionInfo) (string, error) {
	t, ok := m.transcripts[session.ID]
	if !ok {
		return "", os.ErrNotExist
	}
	return t, nil
}

func (m *mockHarness) GetMemoryDir(_ string) (string, error) {
	return m.memoryDir, nil
}

func TestAutodream_Run_EmptySessions(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write initial MEMORY.md
	initialContent := "# Test Memory\n\n## Notes\n- existing note\n"
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	harness := &mockHarness{
		sessions:  nil, // no sessions
		memoryDir: memDir,
	}

	config := DefaultConfig()
	config.DryRun = true
	config.StateFile = filepath.Join(tmpDir, "trigger-state.json")

	dream := NewAutodream(memDir, harness, nil, config)
	report, err := dream.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if report.SignalsFound != 0 {
		t.Errorf("expected 0 signals, got %d", report.SignalsFound)
	}
}

func TestAutodream_Run_WithCorrections(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initialContent := "# Workspace Memory\n\n## Notes\n- existing note\n"
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	harness := &mockHarness{
		sessions: []SessionInfo{
			{ID: "session-1", FilePath: "test.jsonl"},
		},
		transcripts: map[string]string{
			"session-1": "user: no, actually we should always use Go for new tools\nuser: I prefer tabs over spaces\nuser: we decided to use the hippocampus module\n",
		},
		memoryDir: memDir,
	}

	config := DefaultConfig()
	config.DryRun = false
	config.StateFile = filepath.Join(tmpDir, "trigger-state.json")

	dream := NewAutodream(memDir, harness, nil, config)
	report, err := dream.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if report.SignalsFound == 0 {
		t.Error("expected signals to be found")
	}

	if report.EntriesAdded == 0 {
		t.Error("expected entries to be added")
	}

	// Verify MEMORY.md was updated
	content, err := os.ReadFile(filepath.Join(memDir, "MEMORY.md"))
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "Corrections") && !strings.Contains(contentStr, "Preferences") && !strings.Contains(contentStr, "Decisions") {
		t.Error("expected new sections in MEMORY.md")
	}
}

func TestAutodream_Run_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initialContent := "# Memory\n\n## Notes\n- note one\n"
	memPath := filepath.Join(memDir, "MEMORY.md")
	if err := os.WriteFile(memPath, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	harness := &mockHarness{
		sessions: []SessionInfo{
			{ID: "s1", FilePath: "test.jsonl"},
		},
		transcripts: map[string]string{
			"s1": "user: I prefer using Go for everything\n",
		},
		memoryDir: memDir,
	}

	config := DefaultConfig()
	config.DryRun = true
	config.StateFile = filepath.Join(tmpDir, "trigger-state.json")

	dream := NewAutodream(memDir, harness, nil, config)
	report, err := dream.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !report.DryRun {
		t.Error("expected dry run mode")
	}

	// Verify MEMORY.md was NOT modified
	content, err := os.ReadFile(memPath)
	if err != nil {
		t.Fatalf("read MEMORY.md: %v", err)
	}

	if string(content) != initialContent {
		t.Error("MEMORY.md should not be modified in dry-run mode")
	}
}

func TestAutodream_Run_BackupCreated(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initialContent := "# Memory\n\n## Notes\n- original\n"
	memPath := filepath.Join(memDir, "MEMORY.md")
	if err := os.WriteFile(memPath, []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}

	harness := &mockHarness{
		sessions: []SessionInfo{
			{ID: "s1", FilePath: "test.jsonl"},
		},
		transcripts: map[string]string{
			"s1": "user: always use strict mode in TypeScript\n",
		},
		memoryDir: memDir,
	}

	config := DefaultConfig()
	config.DryRun = false
	config.StateFile = filepath.Join(tmpDir, "trigger-state.json")

	dream := NewAutodream(memDir, harness, nil, config)
	_, err := dream.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify backup was created
	backupPath := memPath + ".bak"
	backup, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}

	if string(backup) != initialContent {
		t.Error("backup content doesn't match original")
	}
}

func TestExtractSignalsV1(t *testing.T) {
	transcript := `user: no, actually we should use Go for that
user: I prefer descriptive variable names
user: let's go with option B for the architecture
user: I discovered that the API requires auth headers
user: just a regular comment with no signal
assistant: I'll implement that now`

	signals := extractSignalsV1(transcript, "test-session")

	if len(signals) == 0 {
		t.Fatal("expected signals to be extracted")
	}

	// Check that we got at least one of each type
	types := make(map[SignalType]bool)
	for _, s := range signals {
		types[s.Type] = true
	}

	if !types[SignalCorrection] {
		t.Error("expected correction signal")
	}
	if !types[SignalPreference] {
		t.Error("expected preference signal")
	}
	if !types[SignalDecision] {
		t.Error("expected decision signal")
	}
	if !types[SignalLearning] {
		t.Error("expected learning signal")
	}
}

func TestDeduplicateSignals(t *testing.T) {
	signals := []Signal{
		{Type: SignalCorrection, Content: "same content"},
		{Type: SignalCorrection, Content: "same content"},
		{Type: SignalPreference, Content: "different"},
	}

	deduped := deduplicateSignals(signals)
	if len(deduped) != 2 {
		t.Errorf("expected 2 unique signals, got %d", len(deduped))
	}
}

func TestDeduplicateSignals_CaseInsensitive(t *testing.T) {
	signals := []Signal{
		{Type: SignalCorrection, Content: "Use Go for tools"},
		{Type: SignalCorrection, Content: "use go for tools"},
		{Type: SignalCorrection, Content: "USE GO FOR TOOLS"},
	}

	deduped := deduplicateSignals(signals)
	if len(deduped) != 1 {
		t.Errorf("expected 1 unique signal after case-insensitive dedup, got %d", len(deduped))
	}
}

func TestDeduplicateSignals_WhitespaceNormalized(t *testing.T) {
	signals := []Signal{
		{Type: SignalPreference, Content: "always  use   tabs"},
		{Type: SignalPreference, Content: "always use tabs"},
		{Type: SignalPreference, Content: "  always use tabs  "},
	}

	deduped := deduplicateSignals(signals)
	if len(deduped) != 1 {
		t.Errorf("expected 1 unique signal after whitespace normalization, got %d", len(deduped))
	}
}

func TestNormalizeForDedup(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello world"},
		{"  extra   spaces  ", "extra spaces"},
		{"ALLCAPS", "allcaps"},
		{"mixed\twhitespace\n here", "mixed whitespace here"},
	}

	for _, tt := range tests {
		got := normalizeForDedup(tt.input)
		if got != tt.want {
			t.Errorf("normalizeForDedup(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestArchiveOldestTopics(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create topic files with different mod times
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var topicFiles []TopicFile
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("topic-%d.md", i)
		path := filepath.Join(memDir, name)
		content := fmt.Sprintf("# Topic %d\n", i)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		modTime := baseTime.Add(time.Duration(i) * time.Hour)
		os.Chtimes(path, modTime, modTime)
		topicFiles = append(topicFiles, TopicFile{
			Name:    name,
			Path:    path,
			Content: content,
			ModTime: modTime,
		})
	}

	state := &MemoryState{TopicFiles: topicFiles}
	config := DefaultConfig()
	dream := NewAutodream(memDir, nil, nil, config)

	archived := dream.archiveOldestTopics(state, 2)
	if len(archived) != 2 {
		t.Fatalf("expected 2 archived, got %d", len(archived))
	}

	// Oldest files should be archived
	if archived[0] != "topic-0.md" || archived[1] != "topic-1.md" {
		t.Errorf("expected oldest files archived, got %v", archived)
	}

	// Verify files moved to archive dir
	archiveDir := filepath.Join(memDir, "archive")
	for _, name := range archived {
		if _, err := os.Stat(filepath.Join(archiveDir, name)); err != nil {
			t.Errorf("archived file %s not found in archive dir: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(memDir, name)); err == nil {
			t.Errorf("archived file %s still exists in memory dir", name)
		}
	}
}

func TestSortTopicsByAge(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	topics := []TopicFile{
		{Name: "c.md", ModTime: base.Add(2 * time.Hour)},
		{Name: "a.md", ModTime: base},
		{Name: "b.md", ModTime: base.Add(1 * time.Hour)},
	}

	sortTopicsByAge(topics)

	if topics[0].Name != "a.md" || topics[1].Name != "b.md" || topics[2].Name != "c.md" {
		t.Errorf("unexpected sort order: %v", []string{topics[0].Name, topics[1].Name, topics[2].Name})
	}
}

func TestPrune_EnforcesMaxTopicFiles(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a large MEMORY.md that will trigger overflow
	var lines []string
	lines = append(lines, "# Memory")
	lines = append(lines, "")
	lines = append(lines, "## Section")
	lines = append(lines, "### Big Subsection")
	for i := 0; i < 250; i++ {
		lines = append(lines, fmt.Sprintf("- entry %d with some content", i))
	}
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create existing topic files to push total over limit
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var topicFiles []TopicFile
	for i := 0; i < 19; i++ {
		name := fmt.Sprintf("existing-%d.md", i)
		path := filepath.Join(memDir, name)
		if err := os.WriteFile(path, []byte("# existing\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		modTime := baseTime.Add(time.Duration(i) * time.Hour)
		os.Chtimes(path, modTime, modTime)
		topicFiles = append(topicFiles, TopicFile{
			Name: name, Path: path, Content: "# existing\n", ModTime: modTime,
		})
	}

	doc, _ := ParseMemoryMD(content)
	state := &MemoryState{
		MemoryDoc:  doc,
		MemoryPath: filepath.Join(memDir, "MEMORY.md"),
		TopicFiles: topicFiles,
	}

	config := DefaultConfig()
	config.MaxTopicFiles = 20
	config.MaxMemoryLines = 200
	dream := NewAutodream(memDir, nil, nil, config)

	result, err := dream.prune(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	// Should have archived some oldest topic files
	if len(result.archived) == 0 && len(result.topicOverflow) > 0 {
		totalAfter := len(topicFiles) + len(result.topicOverflow)
		if totalAfter > config.MaxTopicFiles {
			t.Errorf("topic files (%d) exceed MaxTopicFiles (%d) but nothing was archived",
				totalAfter, config.MaxTopicFiles)
		}
	}
}

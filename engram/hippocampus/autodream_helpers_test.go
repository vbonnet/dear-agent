package hippocampus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignalToSection(t *testing.T) {
	tests := []struct {
		input SignalType
		want  string
	}{
		{SignalCorrection, "Corrections"},
		{SignalPreference, "Preferences"},
		{SignalDecision, "Decisions"},
		{SignalLearning, "Learnings"},
		{SignalFact, "Facts"},
		{SignalType("unknown"), "Notes"},
	}

	for _, tt := range tests {
		got := signalToSection(tt.input)
		if got != tt.want {
			t.Errorf("signalToSection(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSectionToTopicFile(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Repo Roles", "repo-roles.md"},
		{"Key Files", "key-files.md"},
		{"CI/CD Hardening", "cicd-hardening.md"},
		{"Simple", "simple.md"},
	}

	for _, tt := range tests {
		got := sectionToTopicFile(tt.input)
		if got != tt.want {
			t.Errorf("sectionToTopicFile(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRenderSectionContent(t *testing.T) {
	sec := &MemorySection{
		Heading: "Test Section",
		Content: []string{"- item 1", "- item 2", "- item 3"},
	}

	result := renderSectionContent(sec)

	if !strings.HasPrefix(result, "# Test Section\n") {
		t.Errorf("expected heading prefix, got %q", result)
	}
	if !strings.Contains(result, "- item 1\n") {
		t.Error("expected item 1 in output")
	}
	if !strings.Contains(result, "- item 3\n") {
		t.Error("expected item 3 in output")
	}
}

func TestKeywordOverlap_EmptySets(t *testing.T) {
	a := map[string]bool{}
	b := map[string]bool{"word": true}

	if keywordOverlap(a, b) != 0 {
		t.Error("expected 0 overlap with empty set a")
	}
	if keywordOverlap(b, a) != 0 {
		t.Error("expected 0 overlap with empty set b")
	}
	if keywordOverlap(a, a) != 0 {
		t.Error("expected 0 overlap with both empty")
	}
}

func TestFindSimilarEntry_NoMatch(t *testing.T) {
	entries := []string{
		"- totally different content about golang",
		"- another entry about testing patterns",
	}
	idx := findSimilarEntry(entries, "- something about python data science")
	if idx != -1 {
		t.Errorf("expected no match (-1), got %d", idx)
	}
}

func TestFindSimilarEntry_SkipsNonBullets(t *testing.T) {
	entries := []string{
		"not a bullet point but has matching words",
		"- actual bullet with matching words present",
	}
	// The function should skip non-bullet entries
	idx := findSimilarEntry(entries, "- matching words present here")
	if idx == 0 {
		t.Error("should skip non-bullet entries")
	}
}

func TestAutodream_Prune_OverLimit(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Build a document that exceeds 10 lines (use small limit for testing)
	var contentLines []string
	for i := 0; i < 20; i++ {
		contentLines = append(contentLines, "- detail line for testing overflow")
	}

	doc := &MemoryDocument{
		Sections: []MemorySection{
			{
				Level:   1,
				Heading: "Memory",
				Children: []MemorySection{
					{
						Level:   2,
						Heading: "Big Section",
						Content: contentLines,
					},
				},
			},
		},
	}

	state := &MemoryState{
		MemoryDoc:  doc,
		MemoryPath: filepath.Join(memDir, "MEMORY.md"),
	}

	config := DefaultConfig()
	config.MaxMemoryLines = 10 // force pruning
	config.StateFile = filepath.Join(tmpDir, "state.json")

	dream := NewAutodream(memDir, &mockHarness{memoryDir: memDir}, nil, config)
	result, err := dream.prune(context.Background(), state, &consolidationResult{})
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if result.entriesPruned == 0 {
		t.Error("expected entries to be pruned when over limit")
	}
	if len(result.topicOverflow) == 0 {
		t.Error("expected topic overflow files to be created")
	}
}

func TestAutodream_Prune_UnderLimit(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	doc := &MemoryDocument{
		Sections: []MemorySection{
			{Level: 1, Heading: "Memory", Content: []string{"- small"}},
		},
	}

	state := &MemoryState{
		MemoryDoc:  doc,
		MemoryPath: filepath.Join(memDir, "MEMORY.md"),
	}

	config := DefaultConfig()
	config.MaxMemoryLines = 200
	config.StateFile = filepath.Join(tmpDir, "state.json")

	dream := NewAutodream(memDir, &mockHarness{memoryDir: memDir}, nil, config)
	result, err := dream.prune(context.Background(), state, &consolidationResult{})
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	if result.entriesPruned != 0 {
		t.Errorf("expected 0 pruned, got %d", result.entriesPruned)
	}
}

func TestAutodream_LoadTopicFiles(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create topic files
	if err := os.WriteFile(filepath.Join(memDir, "topic-one.md"), []byte("# Topic One\n- detail"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "topic-two.md"), []byte("# Topic Two\n- detail"), 0o644); err != nil {
		t.Fatal(err)
	}
	// MEMORY.md should be excluded
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte("# Memory"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-md files should be excluded
	if err := os.WriteFile(filepath.Join(memDir, "notes.txt"), []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}

	dream := NewAutodream(memDir, nil, nil, DefaultConfig())
	topics, err := dream.loadTopicFiles()
	if err != nil {
		t.Fatalf("loadTopicFiles failed: %v", err)
	}

	if len(topics) != 2 {
		t.Fatalf("expected 2 topic files, got %d", len(topics))
	}

	names := map[string]bool{}
	for _, tf := range topics {
		names[tf.Name] = true
	}
	if !names["topic-one.md"] || !names["topic-two.md"] {
		t.Errorf("expected topic-one.md and topic-two.md, got %v", names)
	}
}

func TestAutodream_LoadTopicFiles_NoDir(t *testing.T) {
	dream := NewAutodream("/nonexistent/path", nil, nil, DefaultConfig())
	topics, err := dream.loadTopicFiles()
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
	if topics != nil {
		t.Errorf("expected nil topics, got %v", topics)
	}
}

func TestGenerateDiff_NoChanges(t *testing.T) {
	diff := generateDiff("same content", "same content")
	if diff != "(no changes)" {
		t.Errorf("expected '(no changes)', got %q", diff)
	}
}

func TestGenerateDiff_WithChanges(t *testing.T) {
	diff := generateDiff("line one\nline two", "line one\nline three")
	if !strings.Contains(diff, "--- MEMORY.md") {
		t.Error("expected diff header")
	}
	if !strings.Contains(diff, "- line two") {
		t.Error("expected removed line")
	}
	if !strings.Contains(diff, "+ line three") {
		t.Error("expected added line")
	}
}

func TestAtomicWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.md")

	// Write initial content
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Atomic write new content
	if err := atomicWriteFile(path, []byte("updated")); err != nil {
		t.Fatalf("atomicWriteFile failed: %v", err)
	}

	// Check new content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "updated" {
		t.Errorf("expected 'updated', got %q", string(content))
	}

	// Check backup was created
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal("backup not created")
	}
	if string(backup) != "original" {
		t.Errorf("expected 'original' in backup, got %q", string(backup))
	}
}

func TestAtomicWriteFile_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "new-dir", "new-file.md")

	if err := atomicWriteFile(path, []byte("new content")); err != nil {
		t.Fatalf("atomicWriteFile failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new content" {
		t.Errorf("expected 'new content', got %q", string(content))
	}

	// No backup should exist for new file
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("backup should not exist for new file")
	}
}

func TestAutodream_Orient_NoMemoryFile(t *testing.T) {
	tmpDir := t.TempDir()
	memDir := filepath.Join(tmpDir, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	config := DefaultConfig()
	config.StateFile = filepath.Join(tmpDir, "state.json")

	dream := NewAutodream(memDir, nil, nil, config)
	state, err := dream.orient(context.Background())
	if err != nil {
		t.Fatalf("orient failed: %v", err)
	}

	if state.MemoryDoc == nil {
		t.Fatal("expected non-nil MemoryDoc")
	}
	if len(state.MemoryDoc.Sections) != 0 {
		t.Error("expected empty document for missing MEMORY.md")
	}
}

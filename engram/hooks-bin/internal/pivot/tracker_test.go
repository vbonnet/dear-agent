package pivot

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClassifyFile_CodeExtensions(t *testing.T) {
	codeFiles := []string{
		"/src/main.go",
		"/app/index.ts",
		"/lib/utils.py",
		"/server.js",
		"/handler.rs",
		"/Service.java",
		"/query.sql",
		"/schema.proto",
	}
	for _, f := range codeFiles {
		t.Run(f, func(t *testing.T) {
			if got := ClassifyFile(f); got != KindCode {
				t.Errorf("ClassifyFile(%q) = %q, want %q", f, got, KindCode)
			}
		})
	}
}

func TestClassifyFile_DocsExtensions(t *testing.T) {
	docFiles := []string{
		"/docs/README.md",
		"/ARCHITECTURE.mdx",
		"/notes.txt",
		"/guide.rst",
		"/plan.adoc",
	}
	for _, f := range docFiles {
		t.Run(f, func(t *testing.T) {
			if got := ClassifyFile(f); got != KindDocs {
				t.Errorf("ClassifyFile(%q) = %q, want %q", f, got, KindDocs)
			}
		})
	}
}

func TestClassifyFile_DocFilenames(t *testing.T) {
	docNames := []string{
		"/project/README",
		"/CHANGELOG",
		"/CONTRIBUTING",
	}
	for _, f := range docNames {
		t.Run(f, func(t *testing.T) {
			if got := ClassifyFile(f); got != KindDocs {
				t.Errorf("ClassifyFile(%q) = %q, want %q", f, got, KindDocs)
			}
		})
	}
}

func TestClassifyFile_Other(t *testing.T) {
	otherFiles := []string{
		"/config.yaml",
		"/Dockerfile",
		"/image.png",
		"/.gitignore",
	}
	for _, f := range otherFiles {
		t.Run(f, func(t *testing.T) {
			if got := ClassifyFile(f); got != KindOther {
				t.Errorf("ClassifyFile(%q) = %q, want %q", f, got, KindOther)
			}
		})
	}
}

func TestRecordFileOp_SkipsOtherKind(t *testing.T) {
	s := State{}
	s.RecordFileOp("/config.yaml", "Edit")
	if len(s.Window) != 0 {
		t.Errorf("Window should be empty for KindOther files, got %d entries", len(s.Window))
	}
}

func TestRecordFileOp_TracksCodeAndDocs(t *testing.T) {
	s := State{}
	s.RecordFileOp("/src/main.go", "Edit")
	s.RecordFileOp("/docs/README.md", "Write")

	if len(s.Window) != 2 {
		t.Fatalf("Window should have 2 entries, got %d", len(s.Window))
	}
	if s.CodeCount != 1 {
		t.Errorf("CodeCount = %d, want 1", s.CodeCount)
	}
	if s.DocsCount != 1 {
		t.Errorf("DocsCount = %d, want 1", s.DocsCount)
	}
}

func TestRecordFileOp_SlidingWindowTrimming(t *testing.T) {
	s := State{}
	// Add WindowSize + 5 entries
	for i := 0; i < WindowSize+5; i++ {
		s.RecordFileOp("/src/file.go", "Edit")
	}
	if len(s.Window) != WindowSize {
		t.Errorf("Window should be capped at %d, got %d", WindowSize, len(s.Window))
	}
}

func TestRecordFileOp_EstablishesInitialKind(t *testing.T) {
	s := State{}
	s.RecordFileOp("/src/a.go", "Edit")
	s.RecordFileOp("/src/b.go", "Edit")

	if s.InitialKind != "" {
		t.Error("InitialKind should not be set with only 2 ops")
	}

	s.RecordFileOp("/src/c.go", "Edit")
	if s.InitialKind != KindCode {
		t.Errorf("InitialKind = %q, want %q", s.InitialKind, KindCode)
	}
}

func TestRecordFileOp_EstablishesInitialKindDocs(t *testing.T) {
	s := State{}
	s.RecordFileOp("/docs/a.md", "Write")
	s.RecordFileOp("/docs/b.md", "Write")
	s.RecordFileOp("/docs/c.md", "Write")

	if s.InitialKind != KindDocs {
		t.Errorf("InitialKind = %q, want %q", s.InitialKind, KindDocs)
	}
}

func TestCheckPivot_NotDetectedWhenInitialKindDocs(t *testing.T) {
	s := State{InitialKind: KindDocs}
	// Fill window with docs
	for i := 0; i < WindowSize; i++ {
		s.RecordFileOp("/docs/file.md", "Write")
	}
	result := s.CheckPivot()
	if result.Detected {
		t.Error("Pivot should not be detected when session started with docs")
	}
}

func TestCheckPivot_NotDetectedWhenNoInitialKind(t *testing.T) {
	s := State{}
	s.RecordFileOp("/docs/file.md", "Write")
	result := s.CheckPivot()
	if result.Detected {
		t.Error("Pivot should not be detected when InitialKind not established")
	}
}

func TestCheckPivot_NotDetectedBelowMinEntries(t *testing.T) {
	s := State{InitialKind: KindCode}
	// Add fewer than WindowSize/2 entries
	for i := 0; i < WindowSize/2-1; i++ {
		s.Window = append(s.Window, FileEntry{Kind: KindDocs, ToolName: "Write", Timestamp: time.Now()})
	}
	result := s.CheckPivot()
	if result.Detected {
		t.Error("Pivot should not be detected with too few entries")
	}
}

func TestCheckPivot_DetectedWhenPivotingToDocsWithWrites(t *testing.T) {
	s := State{InitialKind: KindCode}
	// Fill window: 2 code + 8 docs (80% docs)
	for i := 0; i < 2; i++ {
		s.Window = append(s.Window, FileEntry{
			Path: "/src/main.go", Kind: KindCode, ToolName: "Edit", Timestamp: time.Now(),
		})
	}
	for i := 0; i < 8; i++ {
		s.Window = append(s.Window, FileEntry{
			Path: "/docs/plan.md", Kind: KindDocs, ToolName: "Write", Timestamp: time.Now(),
		})
	}

	result := s.CheckPivot()
	if !result.Detected {
		t.Error("Pivot should be detected when 80% docs in window")
	}
	if result.DocsRatio != 0.8 {
		t.Errorf("DocsRatio = %f, want 0.8", result.DocsRatio)
	}
}

func TestCheckPivot_NotDetectedWithOnlyReads(t *testing.T) {
	s := State{InitialKind: KindCode}
	// Fill window with docs reads only (no writes)
	for i := 0; i < WindowSize; i++ {
		s.Window = append(s.Window, FileEntry{
			Path: "/docs/file.md", Kind: KindDocs, ToolName: "Read", Timestamp: time.Now(),
		})
	}

	result := s.CheckPivot()
	if result.Detected {
		t.Error("Pivot should not be detected when all docs operations are reads")
	}
}

func TestCheckPivot_NotDetectedAtExactThreshold(t *testing.T) {
	s := State{InitialKind: KindCode}
	// 5 code + 5 docs = exactly 50% (not > 50%)
	for i := 0; i < 5; i++ {
		s.Window = append(s.Window, FileEntry{
			Path: "/src/main.go", Kind: KindCode, ToolName: "Edit", Timestamp: time.Now(),
		})
	}
	for i := 0; i < 5; i++ {
		s.Window = append(s.Window, FileEntry{
			Path: "/docs/plan.md", Kind: KindDocs, ToolName: "Write", Timestamp: time.Now(),
		})
	}

	result := s.CheckPivot()
	if result.Detected {
		t.Error("Pivot should not be detected at exactly 50% (need >50%)")
	}
}

func TestCheckPivot_Cooldown(t *testing.T) {
	now := time.Now()
	recentAlert := now.Add(-1 * time.Minute) // 1 min ago, within cooldown
	s := State{
		InitialKind: KindCode,
		AlertedAt:   &recentAlert,
	}

	for i := 0; i < WindowSize; i++ {
		s.Window = append(s.Window, FileEntry{
			Path: "/docs/plan.md", Kind: KindDocs, ToolName: "Write", Timestamp: time.Now(),
		})
	}

	result := s.CheckPivot()
	if !result.Detected {
		t.Error("Pivot should still be detected")
	}
	if !result.Suppressed {
		t.Error("Alert should be suppressed during cooldown")
	}
}

func TestCheckPivot_CooldownExpired(t *testing.T) {
	oldAlert := time.Now().Add(-10 * time.Minute) // 10 min ago, past cooldown
	s := State{
		InitialKind: KindCode,
		AlertedAt:   &oldAlert,
	}

	for i := 0; i < WindowSize; i++ {
		s.Window = append(s.Window, FileEntry{
			Path: "/docs/plan.md", Kind: KindDocs, ToolName: "Write", Timestamp: time.Now(),
		})
	}

	result := s.CheckPivot()
	if !result.Detected {
		t.Error("Pivot should be detected")
	}
	if result.Suppressed {
		t.Error("Alert should not be suppressed after cooldown expires")
	}
}

func TestMarkAlerted(t *testing.T) {
	s := State{}
	if s.AlertedAt != nil {
		t.Error("AlertedAt should be nil initially")
	}
	s.MarkAlerted()
	if s.AlertedAt == nil {
		t.Error("AlertedAt should be set after MarkAlerted")
	}
}

func TestLoadSaveState(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "pivot-state.json")

	original := State{
		InitialKind: KindCode,
		CodeCount:   5,
		DocsCount:   3,
		Window: []FileEntry{
			{Path: "/src/main.go", Kind: KindCode, ToolName: "Edit", Timestamp: time.Now()},
		},
	}

	if err := SaveState(statePath, original); err != nil {
		t.Fatalf("SaveState() error: %v", err)
	}

	loaded := LoadState(statePath)
	if loaded.InitialKind != original.InitialKind {
		t.Errorf("InitialKind = %q, want %q", loaded.InitialKind, original.InitialKind)
	}
	if loaded.CodeCount != original.CodeCount {
		t.Errorf("CodeCount = %d, want %d", loaded.CodeCount, original.CodeCount)
	}
	if loaded.DocsCount != original.DocsCount {
		t.Errorf("DocsCount = %d, want %d", loaded.DocsCount, original.DocsCount)
	}
	if len(loaded.Window) != 1 {
		t.Errorf("Window length = %d, want 1", len(loaded.Window))
	}
}

func TestLoadState_MissingFile(t *testing.T) {
	state := LoadState("/nonexistent/path/state.json")
	if len(state.Window) != 0 || state.CodeCount != 0 {
		t.Error("LoadState should return empty state for missing file")
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "bad.json")
	os.WriteFile(statePath, []byte("not json{{{"), 0600)

	state := LoadState(statePath)
	if len(state.Window) != 0 {
		t.Error("LoadState should return empty state for invalid JSON")
	}
}

func TestIsFileOp(t *testing.T) {
	if !IsFileOp("Edit") {
		t.Error("Edit should be a file op")
	}
	if !IsFileOp("Write") {
		t.Error("Write should be a file op")
	}
	if !IsFileOp("Read") {
		t.Error("Read should be a file op")
	}
	if IsFileOp("Bash") {
		t.Error("Bash should not be a file op")
	}
	if IsFileOp("Agent") {
		t.Error("Agent should not be a file op")
	}
}

func TestIsWriteOp(t *testing.T) {
	if !IsWriteOp("Edit") {
		t.Error("Edit should be a write op")
	}
	if !IsWriteOp("Write") {
		t.Error("Write should be a write op")
	}
	if IsWriteOp("Read") {
		t.Error("Read should not be a write op")
	}
}

func TestCheckPivot_RecentDocFilesPopulated(t *testing.T) {
	s := State{InitialKind: KindCode}
	s.Window = append(s.Window,
		FileEntry{Path: "/src/main.go", Kind: KindCode, ToolName: "Edit", Timestamp: time.Now()},
	)
	for i := 0; i < 8; i++ {
		s.Window = append(s.Window, FileEntry{
			Path: "/docs/plan.md", Kind: KindDocs, ToolName: "Write", Timestamp: time.Now(),
		})
	}

	result := s.CheckPivot()
	if len(result.RecentDocFiles) != 8 {
		t.Errorf("RecentDocFiles should have 8 entries, got %d", len(result.RecentDocFiles))
	}
}

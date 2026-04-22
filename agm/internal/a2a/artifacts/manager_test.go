package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name      string
		sizeBytes int64
		want      string
	}{
		{"zero bytes", 0, "0.0 B"},
		{"small bytes", 512, "512.0 B"},
		{"one KB", 1024, "1.0 KB"},
		{"fractional KB", 1536, "1.5 KB"},
		{"one MB", 1024 * 1024, "1.0 MB"},
		{"large MB", 500 * 1024 * 1024, "500.0 MB"},
		{"one GB", 1024 * 1024 * 1024, "1.0 GB"},
		{"one TB", 1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{"one PB", 1024 * 1024 * 1024 * 1024 * 1024, "1.0 PB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSize(tt.sizeBytes)
			if got != tt.want {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.sizeBytes, got, tt.want)
			}
		})
	}
}

func TestParseKeyPoints(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", nil},
		{"single point", "first point", []string{"first point"}},
		{"multiple points", "one, two, three", []string{"one", "two", "three"}},
		{"whitespace trimming", "  alpha ,  beta  , gamma  ", []string{"alpha", "beta", "gamma"}},
		{"empty segments ignored", "a,,b, ,c", []string{"a", "b", "c"}},
		{"single trailing comma", "only,", []string{"only"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseKeyPoints(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseKeyPoints(%q) returned %d items, want %d", tt.input, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseKeyPoints(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGeneratePointer(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &Manager{baseDir: tmpDir}

	t.Run("no summary no key points", func(t *testing.T) {
		got := mgr.GeneratePointer("chan1", "file.txt", "", nil)
		wantPath := filepath.Join(tmpDir, "chan1", "file.txt")
		if !strings.Contains(got, wantPath) {
			t.Errorf("expected pointer to contain path %q, got:\n%s", wantPath, got)
		}
		if strings.Contains(got, "**Summary**") {
			t.Error("expected no summary section")
		}
		if strings.Contains(got, "**Key points**") {
			t.Error("expected no key points section")
		}
	})

	t.Run("with summary", func(t *testing.T) {
		got := mgr.GeneratePointer("chan1", "file.txt", "A brief summary", nil)
		if !strings.Contains(got, "**Summary**: A brief summary") {
			t.Errorf("expected summary in output, got:\n%s", got)
		}
	})

	t.Run("with key points", func(t *testing.T) {
		got := mgr.GeneratePointer("chan1", "file.txt", "", []string{"point A", "point B"})
		if !strings.Contains(got, "**Key points**") {
			t.Errorf("expected key points header, got:\n%s", got)
		}
		if !strings.Contains(got, "- point A\n") {
			t.Errorf("expected point A, got:\n%s", got)
		}
		if !strings.Contains(got, "- point B\n") {
			t.Errorf("expected point B, got:\n%s", got)
		}
	})

	t.Run("with summary and key points", func(t *testing.T) {
		got := mgr.GeneratePointer("chan1", "data.json", "Full summary", []string{"k1"})
		if !strings.Contains(got, "**Summary**: Full summary") {
			t.Error("missing summary")
		}
		if !strings.Contains(got, "- k1\n") {
			t.Error("missing key point")
		}
	})
}

func TestNewManager(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		dir := filepath.Join(tmpDir, "artifacts")
		mgr, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager(%q) returned error: %v", dir, err)
		}
		if mgr == nil {
			t.Fatal("expected non-nil manager")
		}
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("expected a directory")
		}
	})

	t.Run("empty string uses default", func(t *testing.T) {
		// We can't easily test the default path without side effects,
		// but we verify it doesn't return an error (the default expands ~).
		mgr, err := NewManager("")
		if err != nil {
			t.Fatalf("NewManager(\"\") returned error: %v", err)
		}
		if mgr == nil {
			t.Fatal("expected non-nil manager")
		}
	})

	t.Run("nested directory creation", func(t *testing.T) {
		tmpDir := t.TempDir()
		dir := filepath.Join(tmpDir, "a", "b", "c")
		mgr, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager(%q) returned error: %v", dir, err)
		}
		if mgr == nil {
			t.Fatal("expected non-nil manager")
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("nested directory not created: %v", err)
		}
	})
}

func TestListArtifacts(t *testing.T) {
	t.Run("non-existent channel returns empty list", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := &Manager{baseDir: tmpDir}
		artifacts, err := mgr.ListArtifacts("no-such-channel")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 0 {
			t.Errorf("expected empty list, got %d artifacts", len(artifacts))
		}
	})

	t.Run("returns artifacts sorted by name", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := &Manager{baseDir: tmpDir}
		channelDir := filepath.Join(tmpDir, "ch1")
		if err := os.MkdirAll(channelDir, 0755); err != nil {
			t.Fatal(err)
		}
		// Create files in non-alphabetical order.
		for _, name := range []string{"zebra.txt", "alpha.txt", "middle.txt"} {
			if err := os.WriteFile(filepath.Join(channelDir, name), []byte("data"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		artifacts, err := mgr.ListArtifacts("ch1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 3 {
			t.Fatalf("expected 3 artifacts, got %d", len(artifacts))
		}
		wantOrder := []string{"alpha.txt", "middle.txt", "zebra.txt"}
		for i, a := range artifacts {
			if a.Name != wantOrder[i] {
				t.Errorf("artifacts[%d].Name = %q, want %q", i, a.Name, wantOrder[i])
			}
		}
	})

	t.Run("excludes INDEX.md and directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := &Manager{baseDir: tmpDir}
		channelDir := filepath.Join(tmpDir, "ch2")
		if err := os.MkdirAll(filepath.Join(channelDir, "subdir"), 0755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(channelDir, "INDEX.md"), []byte("index"), 0644)
		os.WriteFile(filepath.Join(channelDir, "real.txt"), []byte("content"), 0644)

		artifacts, err := mgr.ListArtifacts("ch2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(artifacts))
		}
		if artifacts[0].Name != "real.txt" {
			t.Errorf("expected real.txt, got %q", artifacts[0].Name)
		}
	})
}

func TestGetArtifactPath(t *testing.T) {
	t.Run("non-existent artifact returns false", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := &Manager{baseDir: tmpDir}
		path, ok := mgr.GetArtifactPath("chan1", "missing.txt")
		if ok {
			t.Error("expected ok=false for non-existent artifact")
		}
		if path != "" {
			t.Errorf("expected empty path, got %q", path)
		}
	})

	t.Run("existing artifact returns path", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := &Manager{baseDir: tmpDir}
		channelDir := filepath.Join(tmpDir, "chan1")
		os.MkdirAll(channelDir, 0755)
		filePath := filepath.Join(channelDir, "exists.txt")
		os.WriteFile(filePath, []byte("hello"), 0644)

		path, ok := mgr.GetArtifactPath("chan1", "exists.txt")
		if !ok {
			t.Error("expected ok=true for existing artifact")
		}
		if path != filePath {
			t.Errorf("got path %q, want %q", path, filePath)
		}
	})

	t.Run("wrong channel returns false", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr := &Manager{baseDir: tmpDir}
		channelDir := filepath.Join(tmpDir, "chan1")
		os.MkdirAll(channelDir, 0755)
		os.WriteFile(filepath.Join(channelDir, "file.txt"), []byte("data"), 0644)

		_, ok := mgr.GetArtifactPath("chan2", "file.txt")
		if ok {
			t.Error("expected ok=false for wrong channel")
		}
	})
}

func TestStoreArtifact(t *testing.T) {
	t.Run("store a real file", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr, err := NewManager(filepath.Join(tmpDir, "store"))
		if err != nil {
			t.Fatal(err)
		}
		// Create a source file to store.
		srcFile := filepath.Join(tmpDir, "source.txt")
		content := []byte("artifact content here")
		if err := os.WriteFile(srcFile, content, 0644); err != nil {
			t.Fatal(err)
		}

		destPath, err := mgr.StoreArtifact("channel-1", srcFile, "test artifact")
		if err != nil {
			t.Fatalf("StoreArtifact returned error: %v", err)
		}
		if destPath == "" {
			t.Fatal("expected non-empty destination path")
		}

		// Verify the file was copied.
		got, err := os.ReadFile(destPath)
		if err != nil {
			t.Fatalf("failed to read stored artifact: %v", err)
		}
		if string(got) != string(content) {
			t.Errorf("stored content = %q, want %q", got, content)
		}
	})

	t.Run("non-existent source file returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		mgr, err := NewManager(filepath.Join(tmpDir, "store"))
		if err != nil {
			t.Fatal(err)
		}

		_, err = mgr.StoreArtifact("channel-1", "/no/such/file.txt", "desc")
		if err == nil {
			t.Fatal("expected error for non-existent source file")
		}
		if !strings.Contains(err.Error(), "artifact not found") {
			t.Errorf("expected 'artifact not found' in error, got: %v", err)
		}
	})

	t.Run("creates index entry", func(t *testing.T) {
		tmpDir := t.TempDir()
		baseDir := filepath.Join(tmpDir, "store")
		mgr, err := NewManager(baseDir)
		if err != nil {
			t.Fatal(err)
		}
		srcFile := filepath.Join(tmpDir, "indexed.txt")
		os.WriteFile(srcFile, []byte("data"), 0644)

		_, err = mgr.StoreArtifact("ch-idx", srcFile, "indexed artifact")
		if err != nil {
			t.Fatalf("StoreArtifact returned error: %v", err)
		}

		indexContent, err := os.ReadFile(filepath.Join(baseDir, "INDEX.md"))
		if err != nil {
			t.Fatalf("failed to read INDEX.md: %v", err)
		}
		if !strings.Contains(string(indexContent), "ch-idx/indexed.txt") {
			t.Errorf("INDEX.md missing artifact entry, content:\n%s", indexContent)
		}
	})
}

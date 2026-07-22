package hippocampus

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSupportedHarnesses(t *testing.T) {
	want := []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}
	if got := SupportedHarnesses(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedHarnesses() = %v, want %v", got, want)
	}
}

func TestNewHarnessAdapterAliases(t *testing.T) {
	tests := map[string]string{
		"claude":      "claude-code",
		"codex":       "codex-cli",
		"antigravity": "agy",
		"opencode":    "opencode-cli",
		"pi":          "pi-cli",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			adapter, err := NewHarnessAdapter(input, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if got := adapter.Name(); got != want {
				t.Fatalf("Name() = %q, want %q", got, want)
			}
		})
	}
}

func TestNewHarnessAdapterRejectsUnknown(t *testing.T) {
	if _, err := NewHarnessAdapter("unknown", ""); err == nil {
		t.Fatal("expected unsupported harness error")
	}
}

func TestSideQueryLLMImplementsProvider(t *testing.T) {
	var _ LLMProvider = (*SideQueryLLM)(nil)
}

func TestResolveMemoryDirPrefersSharedProjectMemory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := t.TempDir()
	dir, err := canonicalMemoryDir(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveMemoryDir(project)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(dir) {
		t.Fatalf("ResolveMemoryDir() = %q, want %q", got, dir)
	}
}

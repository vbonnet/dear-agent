package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPluginYAML = `api_version: dear-agent.io/v1
kind: Plugin
name: dear-agent.test.example
version: 0.1.0
capabilities:
  - hooks
`

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestFilesystemLoader_MissingDir(t *testing.T) {
	l := NewFilesystemLoader()
	manifests, errs := l.LoadFromDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if manifests != nil || errs != nil {
		t.Errorf("expected nil/nil, got %v/%v", manifests, errs)
	}
}

func TestFilesystemLoader_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "actually-a-file")
	writeFile(t, file, "")
	l := NewFilesystemLoader()
	_, errs := l.LoadFromDir(file)
	if len(errs) == 0 {
		t.Fatal("expected error for non-directory")
	}
	if !strings.Contains(errs[0].Error(), "not a directory") {
		t.Errorf("unexpected error: %v", errs[0])
	}
}

func TestFilesystemLoader_SingleFileMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), validPluginYAML)
	writeFile(t, filepath.Join(dir, "b.yml"), strings.Replace(validPluginYAML, "example", "other", 1))

	l := NewFilesystemLoader()
	manifests, errs := l.LoadFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(manifests) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(manifests))
	}
}

func TestFilesystemLoader_SubdirectoryMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plugin-a", "plugin.yaml"), validPluginYAML)
	writeFile(t, filepath.Join(dir, "plugin-b", "plugin.yaml"),
		strings.Replace(validPluginYAML, "example", "other", 1))
	// Subdirectory without a manifest is silently skipped.
	if err := os.Mkdir(filepath.Join(dir, "scaffolding"), 0o755); err != nil {
		t.Fatal(err)
	}

	l := NewFilesystemLoader()
	manifests, errs := l.LoadFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(manifests) != 2 {
		t.Fatalf("expected 2 manifests, got %d (%v)", len(manifests), manifests)
	}
}

func TestFilesystemLoader_MixedLayouts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "single.yaml"), validPluginYAML)
	writeFile(t, filepath.Join(dir, "nested", "plugin.yaml"),
		strings.Replace(validPluginYAML, "example", "nested", 1))

	l := NewFilesystemLoader()
	manifests, errs := l.LoadFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(manifests) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(manifests))
	}
}

func TestFilesystemLoader_HiddenSkipped(t *testing.T) {
	dir := t.TempDir()
	// .hidden.yaml: dot-prefix.
	writeFile(t, filepath.Join(dir, ".hidden.yaml"), validPluginYAML)
	// _disabled.yaml: underscore-prefix (documented "park" pattern).
	writeFile(t, filepath.Join(dir, "_disabled.yaml"), validPluginYAML)
	// Hidden subdirectory.
	writeFile(t, filepath.Join(dir, ".cache", "plugin.yaml"), validPluginYAML)
	// Real plugin to keep the run non-empty.
	writeFile(t, filepath.Join(dir, "real.yaml"), validPluginYAML)

	l := NewFilesystemLoader()
	manifests, errs := l.LoadFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest (only real.yaml), got %d", len(manifests))
	}
}

func TestFilesystemLoader_ContinuesOnPerFileError(t *testing.T) {
	dir := t.TempDir()
	// Two valid manifests, one broken.
	writeFile(t, filepath.Join(dir, "ok-1.yaml"),
		strings.Replace(validPluginYAML, "example", "ok-one", 1))
	writeFile(t, filepath.Join(dir, "broken.yaml"), "name: incomplete")
	writeFile(t, filepath.Join(dir, "ok-2.yaml"),
		strings.Replace(validPluginYAML, "example", "ok-two", 1))

	l := NewFilesystemLoader()
	manifests, errs := l.LoadFromDir(dir)
	if len(manifests) != 2 {
		t.Fatalf("expected 2 successful manifests, got %d", len(manifests))
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d (%v)", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "broken.yaml") {
		t.Errorf("error should reference broken.yaml: %v", errs[0])
	}
}

func TestFilesystemLoader_NonYAMLFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "README.md"), "# notes")
	writeFile(t, filepath.Join(dir, "plugin.yaml"), validPluginYAML)
	l := NewFilesystemLoader()
	manifests, errs := l.LoadFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(manifests))
	}
}

func TestFilesystemLoader_StableSortOrder(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"c.yaml", "a.yaml", "b.yaml"} {
		uniq := strings.Replace(validPluginYAML, "example", strings.TrimSuffix(name, ".yaml"), 1)
		writeFile(t, filepath.Join(dir, name), uniq)
	}
	l := NewFilesystemLoader()
	manifests, errs := l.LoadFromDir(dir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(manifests) != 3 {
		t.Fatalf("expected 3, got %d", len(manifests))
	}
	want := []string{"a", "b", "c"}
	for i, m := range manifests {
		if !strings.HasSuffix(m.Name, want[i]) {
			t.Errorf("manifests[%d].Name = %q, want suffix %q", i, m.Name, want[i])
		}
	}
}

// Ensure the loader's stat-error path is exercised: a directory we cannot
// read returns an error rather than a panic. Skipped on Windows where the
// chmod semantics differ.
func TestFilesystemLoader_UnreadableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: chmod restriction would be ignored")
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "noperms")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o755) })

	l := NewFilesystemLoader()
	_, errs := l.LoadFromDir(bad)
	if len(errs) == 0 {
		t.Fatal("expected at least one error for unreadable dir")
	}
	// Could be a stat or readdir error depending on OS; either is acceptable.
	if !errors.Is(errs[0], os.ErrPermission) && !strings.Contains(errs[0].Error(), "permission") {
		t.Errorf("expected permission error, got %v", errs[0])
	}
}

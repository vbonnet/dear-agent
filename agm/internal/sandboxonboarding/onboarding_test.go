package sandboxonboarding

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
)

func TestInstallRendersSelectedTemplateWithRetainedHomePaths(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	homeRoot := loadHomeRoot(t, home)
	templatePath := filepath.Join(t.TempDir(), "onboarding.tmpl")
	templateText := "session={{.SessionName}}\nmerged={{.MergedPath}}\n{{range .Repos}}repo={{.}}\n{{end}}"
	if err := os.WriteFile(templatePath, []byte(templateText), 0o600); err != nil {
		t.Fatal(err)
	}

	request := testRequest(homeRoot)
	request.TemplatePath = templatePath
	request.Repos = []string{
		filepath.Join(home, "src", "repo"),
		filepath.Join(filepath.Dir(home), "home-other", "repo"),
	}
	if err := Install(request); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	content, err := os.ReadFile(outputPath(home, request.WorkingDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"session=test-session",
		"merged=/sandbox/merged",
		"repo=~/src/repo",
		"repo=" + filepath.Join(filepath.Dir(home), "home-other", "repo"),
	} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("installed content %q does not contain %q", content, expected)
		}
	}
}

func TestInstallRejectsInvalidTemplateBeforeOutputMutation(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	homeRoot := loadHomeRoot(t, home)
	templatePath := filepath.Join(t.TempDir(), "invalid.tmpl")
	if err := os.WriteFile(templatePath, []byte("{{end}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := testRequest(homeRoot)
	request.TemplatePath = templatePath

	if err := Install(request); err == nil || !strings.Contains(err.Error(), "parse onboarding template") {
		t.Fatalf("Install() error = %v, want template parse failure", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("invalid template mutated output ancestry: %v", err)
	}
}

func TestInstallRejectsZeroHomeCapability(t *testing.T) {
	request := testRequest(config.HomeRoot{})
	err := Install(request)
	if err == nil || !errors.Is(err, config.ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("Install() error = %v, want unavailable retained HOME", err)
	}
}

func TestInstallRejectsProviderPathEscapeBeforeOutputMutation(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	request := testRequest(loadHomeRoot(t, home))
	request.WorkingDir = "/different/provider/root"

	if err := Install(request); err == nil || !strings.Contains(err.Error(), "or its descendant") {
		t.Fatalf("Install() error = %v, want provider path containment failure", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("invalid provider paths mutated output ancestry: %v", err)
	}
}

func testRequest(home config.HomeRoot) Request {
	return Request{
		Home:       home,
		SessionID:  "test-session",
		MergedPath: "/sandbox/merged",
		WorkingDir: "/sandbox/merged/project",
		Repos:      []string{"/source/repo"},
	}
}

func loadHomeRoot(t *testing.T, home string) config.HomeRoot {
	t.Helper()
	t.Setenv("HOME", home)
	loaded, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	authority, err := loaded.RuntimeAuthority()
	if err != nil {
		t.Fatalf("RuntimeAuthority() error = %v", err)
	}
	homeRoot, err := authority.Home()
	if err != nil {
		t.Fatalf("RuntimeAuthority.Home() error = %v", err)
	}
	return homeRoot
}

func privateDirectory(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return physical
}

func outputPath(home, workingDir string) string {
	return filepath.Join(home, ".claude", "projects", encodedProjectName(workingDir), outputName)
}

//go:build darwin || linux

package specpackage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUnsupportedPlatformStubsCompileForWindows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	goExecutable, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("resolve Go executable: %v", err)
	}
	outputRoot := t.TempDir()
	commands := [][]string{
		{"test", "-c", "-trimpath", "-o", filepath.Join(outputRoot, "specpackage.test.exe"), "."},
		{"build", "-trimpath", "-o", filepath.Join(outputRoot, "spec-governance-package.exe"), "../../cmd/spec-governance-package"},
	}
	for _, arguments := range commands {
		command := exec.CommandContext(ctx, goExecutable, arguments...)
		command.Env = unsupportedCompileEnvironment(os.Environ())
		output, err := command.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("cross-platform compile exceeded its deadline: %v", ctx.Err())
		}
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", goExecutable, strings.Join(arguments, " "), err, output)
		}
	}
}

func unsupportedCompileEnvironment(base []string) []string {
	updates := map[string]string{
		"CGO_ENABLED": "0",
		"GOARCH":      "amd64",
		"GOENV":       "off",
		"GOFLAGS":     "",
		"GOOS":        "windows",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	}
	result := make([]string, 0, len(base)+len(updates))
	for _, item := range base {
		name, _, found := strings.Cut(item, "=")
		if _, replaced := updates[name]; found && replaced {
			continue
		}
		result = append(result, item)
	}
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		result = append(result, fmt.Sprintf("%s=%s", key, updates[key]))
	}
	return result
}

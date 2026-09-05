//go:build darwin || linux

package main

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

// TestFreeBSDCompileOnlyCanary protects the explicitly compile-only FreeBSD
// boundary. Passing this test does not establish runtime or release support.
func TestFreeBSDCompileOnlyCanary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	goExecutable, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("resolve Go executable: %v", err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	moduleRoot, err := findFreeBSDCompileModuleRoot(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}

	outputRoot := t.TempDir()
	commands := [][]string{
		{"test", "-c", "-trimpath", "-o", filepath.Join(outputRoot, "harnessexec.test"), "./agm/internal/harnessexec"},
		{"test", "-c", "-trimpath", "-o", filepath.Join(outputRoot, "sandbox.test"), "./internal/sandbox"},
		{"test", "-c", "-trimpath", "-o", filepath.Join(outputRoot, "supervisor.test"), "./pkg/vroom/supervisor"},
		{"test", "-c", "-trimpath", "-o", filepath.Join(outputRoot, "ops.test"), "./agm/internal/ops"},
		{"build", "-trimpath", "-o", filepath.Join(outputRoot, "agm"), "./agm/cmd/agm"},
		{"build", "-trimpath", "-o", filepath.Join(outputRoot, "disk-watchdog"), "./cmd/disk-watchdog"},
	}

	for _, arguments := range commands {
		command := exec.CommandContext(ctx, goExecutable, arguments...)
		command.Dir = moduleRoot
		command.Env = freeBSDCompileEnvironment(os.Environ())
		output, commandErr := command.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("FreeBSD compile-only canary exceeded its deadline: %v", ctx.Err())
		}
		if commandErr != nil {
			t.Fatalf("%s %s: %v\n%s", goExecutable, strings.Join(arguments, " "), commandErr, output)
		}
	}
}

func findFreeBSDCompileModuleRoot(start string) (string, error) {
	for directory := filepath.Clean(start); ; directory = filepath.Dir(directory) {
		_, err := os.Stat(filepath.Join(directory, "go.mod"))
		if err == nil {
			return directory, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect module root candidate %q: %w", directory, err)
		}
		if parent := filepath.Dir(directory); parent == directory {
			return "", fmt.Errorf("find go.mod above %q", start)
		}
	}
}

func freeBSDCompileEnvironment(base []string) []string {
	updates := map[string]string{
		"CGO_ENABLED": "0",
		"GOAMD64":     "v1",
		"GOARCH":      "amd64",
		"GOENV":       "off",
		"GOFLAGS":     "",
		"GOOS":        "freebsd",
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

func TestFreeBSDCompileEnvironmentOverridesAmbientTarget(t *testing.T) {
	t.Parallel()

	base := []string{
		"GOOS=darwin",
		"GOOS=linux",
		"GOARCH=arm64",
		"GOAMD64=v4",
		"CGO_ENABLED=1",
		"GOENV=/tmp/ambient-goenv",
		"GOFLAGS=-race",
		"GOTOOLCHAIN=auto",
		"GOWORK=/tmp/ambient-go.work",
		"GOCACHE=/tmp/preserved-gocache",
		"GOMODCACHE=/tmp/preserved-gomodcache",
		"CANARY_SENTINEL=preserved",
	}
	got := freeBSDCompileEnvironment(base)

	wantOverrides := map[string]string{
		"CGO_ENABLED": "0",
		"GOAMD64":     "v1",
		"GOARCH":      "amd64",
		"GOENV":       "off",
		"GOFLAGS":     "",
		"GOOS":        "freebsd",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	}
	wantPreserved := map[string]string{
		"GOCACHE":         "/tmp/preserved-gocache",
		"GOMODCACHE":      "/tmp/preserved-gomodcache",
		"CANARY_SENTINEL": "preserved",
	}
	counts := make(map[string]int)
	values := make(map[string]string)
	for _, item := range got {
		name, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		counts[name]++
		values[name] = value
	}
	for name, want := range wantOverrides {
		if counts[name] != 1 || values[name] != want {
			t.Errorf("%s entries = %d, value = %q, want one %q", name, counts[name], values[name], want)
		}
	}
	for name, want := range wantPreserved {
		if counts[name] != 1 || values[name] != want {
			t.Errorf("%s entries = %d, value = %q, want preserved %q", name, counts[name], values[name], want)
		}
	}
}

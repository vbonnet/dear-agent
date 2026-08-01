package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestForbiddenModuleDirective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "ordinary requirements", content: "module example.com/test\n\nrequire example.com/dep v1.0.0\n"},
		{name: "comment only", content: "// replace example.com/dep => ../dep\n"},
		{name: "single replacement", content: "replace example.com/dep => ../dep\n", want: true},
		{name: "replacement block", content: "replace (\nexample.com/dep => ../dep\n)\n", want: true},
		{name: "exclusion", content: "  exclude example.com/dep v1.0.0\n", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := forbiddenModuleDirective.MatchString(test.content); got != test.want {
				t.Fatalf("forbidden directive = %v, want %v", got, test.want)
			}
		})
	}
}

func TestModuleBoundaryFiltering(t *testing.T) {
	t.Parallel()

	files := []string{
		"go.mod",
		"agm/cmd/agm/main.go",
		"spec-governance/go.mod",
		"spec-governance/earslint/parse.go",
		"vendor/example.com/dep/dep.go",
	}
	nested := findNestedModules(files)
	if !reflect.DeepEqual(nested, []string{"spec-governance"}) {
		t.Fatalf("nested modules = %v", nested)
	}

	want := map[string]bool{
		"go.mod":                                 true,
		"agm/cmd/agm/main.go":                    true,
		"spec-governance/go.mod":                 false,
		"spec-governance/earslint/parse.go":      false,
		"vendor/example.com/dep/dep.go":          false,
		"docs/vendor-independent-explanation.md": true,
	}
	for name, included := range want {
		if got := includeInRootModule(name, nested); got != included {
			t.Errorf("includeInRootModule(%q) = %v, want %v", name, got, included)
		}
	}
}

func TestWriteModuleZipUsesProxyPrefixAndSkipsNestedModule(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	writeTestFile(t, repository, "go.mod", "module "+modulePath+"\n")
	writeTestFile(t, repository, "agm/cmd/agm/main.go", "package main\n")
	writeTestFile(t, repository, "spec-governance/go.mod", "module "+modulePath+"/spec-governance\n")
	writeTestFile(t, repository, "spec-governance/earslint/parse.go", "package earslint\n")

	files := []string{"go.mod", "agm/cmd/agm/main.go", "spec-governance/go.mod", "spec-governance/earslint/parse.go"}
	if err := os.Symlink(filepath.Join(repository, "agm/cmd/agm/main.go"), filepath.Join(repository, "linked.go")); err == nil {
		files = append(files, "linked.go")
	}
	destination := filepath.Join(t.TempDir(), "module.zip")
	if err := writeModuleZip(destination, repository, files, findNestedModules(files), maxModuleUncompressedSize); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	var names []string
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	sort.Strings(names)
	want := []string{
		modulePath + "@" + moduleVersion + "/agm/cmd/agm/main.go",
		modulePath + "@" + moduleVersion + "/go.mod",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("zip entries = %v, want %v", names, want)
	}
}

func TestWriteModuleZipRejectsOversizedContent(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	writeTestFile(t, repository, "go.mod", "module "+modulePath+"\n")
	writeTestFile(t, repository, "large.txt", "12345")
	destination := filepath.Join(t.TempDir(), "module.zip")
	err := writeModuleZip(destination, repository, []string{"go.mod", "large.txt"}, nil, 8)
	if err == nil || !strings.Contains(err.Error(), "uncompressed limit") {
		t.Fatalf("writeModuleZip error = %v, want uncompressed-limit failure", err)
	}
}

func TestReadFileLimitedRejectsOversizedInput(t *testing.T) {
	t.Parallel()

	name := filepath.Join(t.TempDir(), "go.mod")
	if err := os.WriteFile(name, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readFileLimited(name, 4); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("readFileLimited error = %v, want size-limit failure", err)
	}
}

func TestReadFileLimitedRejectsSymlink(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	target := filepath.Join(directory, "target.mod")
	link := filepath.Join(directory, "go.mod")
	if err := os.WriteFile(target, []byte("module example.com/target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := readFileLimited(link, 1024); !errors.Is(err, errNotRegularFile) {
		t.Fatalf("readFileLimited error = %v, want non-regular-file failure", err)
	}
}

func TestOpenStableRegularRejectsPathSwap(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	name := filepath.Join(directory, "source")
	writeTestFile(t, directory, "source", "first")
	before, err := os.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(name, filepath.Join(directory, "moved")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, directory, "source", "replacement")
	if file, _, err := openStableRegularAfterLstat(name, before); err == nil {
		file.Close()
		t.Fatal("openStableRegularAfterLstat succeeded after path swap")
	}
}

func TestCopyStableFileRejectsGrowth(t *testing.T) {
	t.Parallel()

	name := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(name, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, info, err := openStableRegular(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if appended, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0); err != nil {
		t.Fatal(err)
	} else if _, err := appended.WriteString("growth"); err != nil {
		appended.Close()
		t.Fatal(err)
	} else if err := appended.Close(); err != nil {
		t.Fatal(err)
	}
	if err := copyStableFile(&bytes.Buffer{}, file, info); err == nil || !strings.Contains(err.Error(), "grew") {
		t.Fatalf("copyStableFile error = %v, want growth failure", err)
	}
}

func TestRemoveAllWritableCleansReadOnlyModuleCache(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "canary")
	moduleDir := filepath.Join(root, "modcache", "example.com", "module@v1.0.0")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	moduleFile := filepath.Join(moduleDir, "go.mod")
	if err := os.WriteFile(moduleFile, []byte("module example.com/module\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(moduleDir, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := removeAllWritable(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("temporary root still exists: %v", err)
	}
}

func TestRunCommandLimitedBoundsAndDrainsOutput(t *testing.T) {
	environment := mergeEnvironment(os.Environ(), map[string]string{
		"CANARY_HELPER_MODE": "output",
	})
	_, err := runCommandLimited(context.Background(), t.TempDir(), environment, 64, os.Args[0], "-test.run=^TestCanaryCommandHelper$")
	if err == nil || !strings.Contains(err.Error(), "output exceeded 64-byte limit") {
		t.Fatalf("runCommandLimited error = %v, want bounded-output failure", err)
	}
}

func TestRunCommandLimitedRejectsNonPositiveLimit(t *testing.T) {
	t.Parallel()

	for _, limit := range []int64{0, -1} {
		if _, err := runCommandLimited(context.Background(), t.TempDir(), nil, limit, "command-that-must-not-run"); err == nil || !strings.Contains(err.Error(), "must be positive") {
			t.Fatalf("runCommandLimited(%d) error = %v, want positive-limit failure", limit, err)
		}
	}
}

func TestRunCommandLimitedHonorsContextDeadline(t *testing.T) {
	environment := mergeEnvironment(os.Environ(), map[string]string{
		"CANARY_HELPER_MODE": "wait",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := runCommandLimited(ctx, t.TempDir(), environment, 64, os.Args[0], "-test.run=^TestCanaryCommandHelper$")
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("runCommandLimited error = %v, want deadline failure", err)
	}
}

func TestCanaryCommandHelper(t *testing.T) {
	switch os.Getenv("CANARY_HELPER_MODE") {
	case "output":
		_, _ = os.Stdout.WriteString(strings.Repeat("x", 4096))
	case "wait":
		time.Sleep(5 * time.Second)
	default:
		t.Skip("helper process")
	}
}

func TestMergeEnvironmentReplacesValuesDeterministically(t *testing.T) {
	t.Parallel()

	got := mergeEnvironment([]string{"PATH=/bin", "GOWORK=auto", "KEEP=value"}, map[string]string{
		"GOWORK":  "off",
		"GOPROXY": "file:///proxy",
	})
	want := []string{"PATH=/bin", "KEEP=value", "GOPROXY=file:///proxy", "GOWORK=off"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
}

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

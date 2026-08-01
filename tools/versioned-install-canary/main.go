// Command versioned-install-canary verifies that documented commands can be
// installed from a module proxy without checkout-local module replacements.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	modulePath                = "github.com/vbonnet/dear-agent"
	moduleVersion             = "v0.0.0-canary"
	canaryTimeout             = 10 * time.Minute
	maxCommandOutputBytes     = 16 << 20
	maxRootGoModBytes         = 16 << 20
	maxModuleUncompressedSize = 500 << 20
)

var (
	errNotRegularFile        = errors.New("not a regular file")
	forbiddenModuleDirective = regexp.MustCompile(`(?m)^\s*(?:replace|exclude)(?:\s|\(|$)`)
	commandPackages          = []string{
		modulePath + "/agm/cmd/agm",
		modulePath + "/engram/cmd/engram",
		modulePath + "/wayfinder/cmd/wayfinder",
	}
)

type candidateModule struct {
	repository    string
	moduleFile    []byte
	files         []string
	nestedModules []string
}

func main() {
	os.Exit(runMain())
}

func runMain() int {
	root := flag.String("root", ".", "repository root to package and verify")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), canaryTimeout)
	defer cancel()
	if err := runCanary(ctx, *root, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "versioned-install-canary: %v\n", err)
		return 1
	}
	return 0
}

func runCanary(ctx context.Context, root string, output io.Writer) (resultErr error) {
	candidate, err := inspectCandidate(ctx, root)
	if err != nil {
		return err
	}
	temporaryRoot, err := os.MkdirTemp("", "dear-agent-versioned-install-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer func() {
		if cleanupErr := removeAllWritable(temporaryRoot); cleanupErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("clean temporary directory: %w", cleanupErr))
		}
	}()

	if err := createModuleProxy(temporaryRoot, candidate); err != nil {
		return err
	}
	environment, binaryDir, err := installEnvironment(ctx, temporaryRoot, candidate.repository)
	if err != nil {
		return err
	}
	if err := installDocumentedCommands(ctx, candidate.repository, environment, output); err != nil {
		return err
	}
	if err := verifyInstalledCommands(binaryDir); err != nil {
		return err
	}

	fmt.Fprintln(output, "versioned-install-canary: documented commands install without local module replacement")
	return nil
}

func inspectCandidate(ctx context.Context, root string) (candidateModule, error) {
	repository, err := filepath.Abs(root)
	if err != nil {
		return candidateModule{}, fmt.Errorf("resolve repository root: %w", err)
	}

	moduleFile, err := readFileLimited(filepath.Join(repository, "go.mod"), maxRootGoModBytes)
	if err != nil {
		return candidateModule{}, fmt.Errorf("read root go.mod: %w", err)
	}
	if forbiddenModuleDirective.Match(moduleFile) {
		return candidateModule{}, fmt.Errorf("root go.mod must not contain replace or exclude directives")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return candidateModule{}, fmt.Errorf("find git: %w", err)
	}
	if _, err := exec.LookPath("go"); err != nil {
		return candidateModule{}, fmt.Errorf("find go: %w", err)
	}

	candidateFiles, err := gitCandidateFiles(ctx, repository)
	if err != nil {
		return candidateModule{}, err
	}
	return candidateModule{
		repository:    repository,
		moduleFile:    moduleFile,
		files:         candidateFiles,
		nestedModules: findNestedModules(candidateFiles),
	}, nil
}

func createModuleProxy(temporaryRoot string, candidate candidateModule) error {
	proxyVersionDir := filepath.Join(temporaryRoot, "proxy", filepath.FromSlash(modulePath), "@v")
	if err := os.MkdirAll(proxyVersionDir, 0o755); err != nil {
		return fmt.Errorf("create proxy directory: %w", err)
	}
	zipPath := filepath.Join(proxyVersionDir, moduleVersion+".zip")
	if err := writeModuleZip(zipPath, candidate.repository, candidate.files, candidate.nestedModules, maxModuleUncompressedSize); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(proxyVersionDir, moduleVersion+".mod"), candidate.moduleFile, 0o600); err != nil {
		return fmt.Errorf("write proxy module file: %w", err)
	}
	info := fmt.Sprintf("{\"Version\":%q,\"Time\":\"2026-01-01T00:00:00Z\"}\n", moduleVersion)
	if err := os.WriteFile(filepath.Join(proxyVersionDir, moduleVersion+".info"), []byte(info), 0o600); err != nil {
		return fmt.Errorf("write proxy info: %w", err)
	}
	if err := os.WriteFile(filepath.Join(proxyVersionDir, "list"), []byte(moduleVersion+"\n"), 0o600); err != nil {
		return fmt.Errorf("write proxy version list: %w", err)
	}
	return nil
}

func installEnvironment(ctx context.Context, temporaryRoot, repository string) ([]string, string, error) {
	moduleCache, err := goEnv(ctx, repository, "GOMODCACHE")
	if err != nil {
		return nil, "", err
	}
	originalProxy, err := goEnv(ctx, repository, "GOPROXY")
	if err != nil {
		return nil, "", err
	}
	proxyChain := strings.Join([]string{
		fileURL(filepath.Join(temporaryRoot, "proxy")),
		fileURL(filepath.Join(moduleCache, "cache", "download")),
		originalProxy,
	}, ",")
	binaryDir := filepath.Join(temporaryRoot, "bin")
	freshModuleCache := filepath.Join(temporaryRoot, "modcache")
	if err := os.MkdirAll(binaryDir, 0o755); err != nil {
		return nil, "", fmt.Errorf("create binary directory: %w", err)
	}
	if err := os.MkdirAll(freshModuleCache, 0o755); err != nil {
		return nil, "", fmt.Errorf("create fresh module cache: %w", err)
	}

	environment := mergeEnvironment(os.Environ(), map[string]string{
		"GO111MODULE": "on",
		"GOENV":       "off",
		"GOFLAGS":     "",
		"GOBIN":       binaryDir,
		"GOMODCACHE":  freshModuleCache,
		"GONOPROXY":   "none",
		"GONOSUMDB":   modulePath,
		"GOPRIVATE":   "",
		"GOPROXY":     proxyChain,
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
	return environment, binaryDir, nil
}

func installDocumentedCommands(ctx context.Context, repository string, environment []string, output io.Writer) error {
	for _, commandPackage := range commandPackages {
		fmt.Fprintf(output, "versioned-install-canary: go install %s@%s\n", commandPackage, moduleVersion)
		if _, err := runCommand(ctx, repository, environment, "go", "install", commandPackage+"@"+moduleVersion); err != nil {
			return err
		}
	}
	return nil
}

func verifyInstalledCommands(binaryDir string) error {
	for _, commandName := range []string{"agm", "engram", "wayfinder"} {
		binaryPath := filepath.Join(binaryDir, commandName+executableSuffix())
		info, err := os.Stat(binaryPath)
		if err != nil {
			return fmt.Errorf("stat installed command %s: %w", commandName, err)
		}
		if info.IsDir() || runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			return fmt.Errorf("installed command %s is not executable", commandName)
		}
	}
	return nil
}

func gitCandidateFiles(ctx context.Context, repository string) ([]string, error) {
	output, err := runCommand(ctx, repository, nil, "git", "-C", repository, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	parts := strings.Split(string(output), "\x00")
	files := make([]string, 0, len(parts))
	for _, name := range parts {
		if name == "" {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("git returned unsafe path %q", name)
		}
		if _, err := os.Lstat(filepath.Join(repository, clean)); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("inspect candidate path %s: %w", name, err)
		}
		files = append(files, filepath.ToSlash(clean))
	}
	sort.Strings(files)
	return files, nil
}

func findNestedModules(files []string) []string {
	var directories []string
	for _, name := range files {
		if name != "go.mod" && path.Base(name) == "go.mod" {
			directories = append(directories, path.Dir(name))
		}
	}
	sort.Strings(directories)
	return directories
}

func includeInRootModule(name string, nestedModules []string) bool {
	if name == "vendor" || strings.HasPrefix(name, "vendor/") {
		return false
	}
	for _, directory := range nestedModules {
		if name == directory || strings.HasPrefix(name, directory+"/") {
			return false
		}
	}
	return true
}

func writeModuleZip(destination, repository string, files, nestedModules []string, maxUncompressedSize int64) error {
	if maxUncompressedSize <= 0 {
		return fmt.Errorf("module content limit must be positive")
	}
	archiveFile, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create module zip: %w", err)
	}
	archive := zip.NewWriter(archiveFile)
	prefix := modulePath + "@" + moduleVersion
	var uncompressedSize int64

	for _, name := range files {
		if !includeInRootModule(name, nestedModules) {
			continue
		}
		sourcePath := filepath.Join(repository, filepath.FromSlash(name))
		source, fileInfo, openErr := openStableRegular(sourcePath)
		if errors.Is(openErr, errNotRegularFile) {
			continue
		}
		if openErr != nil {
			return closeModuleZip(archive, archiveFile, fmt.Errorf("open stable source %s: %w", name, openErr))
		}
		if fileInfo.Size() > maxUncompressedSize-uncompressedSize {
			if closeErr := source.Close(); closeErr != nil {
				return closeModuleZip(archive, archiveFile, fmt.Errorf("close oversized source %s: %w", name, closeErr))
			}
			limitErr := fmt.Errorf("module content exceeds %d-byte uncompressed limit", maxUncompressedSize)
			return closeModuleZip(archive, archiveFile, limitErr)
		}
		uncompressedSize += fileInfo.Size()
		if err := addFileToZip(archive, source, path.Join(prefix, name), fileInfo); err != nil {
			return closeModuleZip(archive, archiveFile, err)
		}
	}
	return closeModuleZip(archive, archiveFile, nil)
}

func addFileToZip(archive *zip.Writer, source *os.File, archivePath string, info os.FileInfo) (resultErr error) {
	defer func() {
		if closeErr := source.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("close source for %s: %w", archivePath, closeErr)
		}
	}()

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("create zip header for %s: %w", archivePath, err)
	}
	header.Name = archivePath
	header.Method = zip.Deflate
	destination, err := archive.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", archivePath, err)
	}
	if err := copyStableFile(destination, source, info); err != nil {
		return fmt.Errorf("copy %s into module zip: %w", archivePath, err)
	}
	return nil
}

func copyStableFile(destination io.Writer, source *os.File, advertised os.FileInfo) error {
	if _, err := io.CopyN(destination, source, advertised.Size()); err != nil {
		return fmt.Errorf("source shrank below advertised size: %w", err)
	}
	var extra [1]byte
	count, readErr := source.Read(extra[:])
	if count != 0 || readErr == nil {
		return fmt.Errorf("source grew beyond advertised size")
	}
	if !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("check source growth: %w", readErr)
	}
	current, err := source.Stat()
	if err != nil {
		return fmt.Errorf("restat source: %w", err)
	}
	if !os.SameFile(advertised, current) || current.Size() != advertised.Size() {
		return fmt.Errorf("source identity or size changed while packaging")
	}
	return nil
}

func closeModuleZip(archive *zip.Writer, archiveFile *os.File, prior error) error {
	zipErr := archive.Close()
	fileErr := archiveFile.Close()
	if prior != nil {
		return prior
	}
	if zipErr != nil {
		return fmt.Errorf("close module zip: %w", zipErr)
	}
	if fileErr != nil {
		return fmt.Errorf("close module zip file: %w", fileErr)
	}
	return nil
}

func goEnv(ctx context.Context, repository, key string) (string, error) {
	output, err := runCommand(ctx, repository, nil, "go", "env", key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func runCommand(ctx context.Context, directory string, environment []string, name string, args ...string) ([]byte, error) {
	return runCommandLimited(ctx, directory, environment, maxCommandOutputBytes, name, args...)
}

func runCommandLimited(
	ctx context.Context,
	directory string,
	environment []string,
	outputLimit int64,
	name string,
	args ...string,
) ([]byte, error) {
	if outputLimit <= 0 {
		return nil, fmt.Errorf("command output limit must be positive")
	}
	command := exec.CommandContext(ctx, name, args...) //nolint:gosec // callers supply fixed git, go, or test-helper commands without a shell
	command.Dir = directory
	command.Env = environment
	output := newBoundedOutput(outputLimit)
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	captured, truncated := output.snapshot()
	if truncated && err == nil {
		return nil, fmt.Errorf("run %s %s: output exceeded %d-byte limit", name, strings.Join(args, " "), outputLimit)
	}
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		diagnostic := strings.TrimSpace(string(captured))
		if truncated {
			diagnostic += fmt.Sprintf("\n[output truncated after %d bytes]", outputLimit)
		}
		if diagnostic != "" {
			return nil, fmt.Errorf("run %s %s: %w\n%s", name, strings.Join(args, " "), err, diagnostic)
		}
		return nil, fmt.Errorf("run %s %s: %w", name, strings.Join(args, " "), err)
	}
	return captured, nil
}

type boundedOutput struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int64
	truncated bool
}

func newBoundedOutput(limit int64) *boundedOutput {
	return &boundedOutput{limit: limit}
}

func (output *boundedOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()

	originalLength := len(data)
	remaining := output.limit - int64(output.buffer.Len())
	if remaining > 0 {
		if int64(len(data)) > remaining {
			data = data[:remaining]
		}
		_, _ = output.buffer.Write(data)
	}
	if int64(originalLength) > remaining {
		output.truncated = true
	}
	return originalLength, nil
}

func (output *boundedOutput) snapshot() ([]byte, bool) {
	output.mu.Lock()
	defer output.mu.Unlock()
	return bytes.Clone(output.buffer.Bytes()), output.truncated
}

func readFileLimited(name string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("file size limit must be positive")
	}
	file, info, err := openStableRegular(name)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		if closeErr := file.Close(); closeErr != nil {
			return nil, fmt.Errorf("close oversized file %s: %w", name, closeErr)
		}
		return nil, fmt.Errorf("%s exceeds %d-byte limit", name, limit)
	}
	data := make([]byte, int(info.Size()))
	_, readErr := io.ReadFull(file, data)
	if readErr == nil {
		var extra [1]byte
		var count int
		count, readErr = file.Read(extra[:])
		if count != 0 || readErr == nil {
			readErr = fmt.Errorf("file grew beyond advertised size")
		} else if errors.Is(readErr, io.EOF) {
			readErr = nil
		}
	}
	if readErr == nil {
		current, statErr := file.Stat()
		if statErr != nil {
			readErr = statErr
		} else if !os.SameFile(info, current) || current.Size() != info.Size() {
			readErr = fmt.Errorf("file identity or size changed while reading")
		}
	}
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read stable file %s: %w", name, readErr)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return data, nil
}

func openStableRegular(name string) (*os.File, os.FileInfo, error) {
	before, err := os.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%w: %s", errNotRegularFile, name)
	}
	return openStableRegularAfterLstat(name, before)
}

func openStableRegularAfterLstat(name string, before os.FileInfo) (*os.File, os.FileInfo, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, nil, err
	}
	after, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = file.Close()
		return nil, nil, fmt.Errorf("file identity changed between lstat and open: %s", name)
	}
	return file, after, nil
}

func removeAllWritable(root string) error {
	rootHandle, err := os.OpenRoot(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	walkErr := filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm() | 0o200
		if entry.IsDir() {
			mode |= 0o700
		}
		relativeName, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		return rootHandle.Chmod(relativeName, mode)
	})
	closeErr := rootHandle.Close()
	if walkErr != nil {
		return walkErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.RemoveAll(root)
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; !replaced {
			result = append(result, entry)
		}
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, key+"="+overrides[key])
	}
	return result
}

func fileURL(name string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(name)}).String()
}

func executableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

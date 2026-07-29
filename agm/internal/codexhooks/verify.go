// Package codexhooks attests repository-scoped Codex hooks before AGM opts
// out of Codex's per-path hook trust prompt.
package codexhooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const hooksManifestPath = ".codex/hooks.json"

var (
	projectDirReference = regexp.MustCompile(`(?:\$\{(?:CLAUDE|CODEX)_PROJECT_DIR\}|\$(?:CLAUDE|CODEX)_PROJECT_DIR)/([A-Za-z0-9._/-]+)`)
	hookRootReference   = regexp.MustCompile(`\$\{AGM_CODEX_HOOK_ROOT:-\.\}/([A-Za-z0-9._/-]+)`)
	relativePathToken   = regexp.MustCompile(`(?:^|[\s"'()])((?:\./)?[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)+)`)
)

// Attestation pins hook trust to immutable Git objects and their exact
// materialization in one sandbox workspace.
type Attestation struct {
	SourceRepo   string
	SourceCommit string
	Digest       string
	HookRoot     string
}

type asset struct {
	path       string
	gitMode    string
	content    []byte
	executable bool
}

// Attest records the source repository's current commit and verifies that the
// sandbox hook manifest and every project-referenced hook are byte-identical
// to regular files in that commit. Source content is read from Git objects,
// never from the mutable source working tree.
func Attest(
	ctx context.Context,
	sourceRepo, sandboxWorkDir, storeBase string,
	writableRoots []string,
) (Attestation, error) {
	sourceRoot, err := gitRoot(ctx, sourceRepo)
	if err != nil {
		return Attestation{}, fmt.Errorf("resolve hook source repository: %w", err)
	}
	if err := rejectWritableOverlap("hook source repository", sourceRoot, writableRoots); err != nil {
		return Attestation{}, err
	}
	commit, err := gitOutput(ctx, sourceRoot, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return Attestation{}, fmt.Errorf("resolve hook source commit: %w", err)
	}
	attestation := Attestation{
		SourceRepo:   sourceRoot,
		SourceCommit: strings.TrimSpace(commit),
	}
	sourceAssets, err := committedAssets(ctx, attestation)
	if err != nil {
		return Attestation{}, err
	}
	attestation.Digest = digestAssets(sourceAssets)
	if err := verifySandboxAssets(ctx, attestation, sandboxWorkDir, sourceAssets); err != nil {
		return Attestation{}, err
	}
	hookRoot, err := materializeAssets(storeBase, attestation.Digest, sourceAssets,
		append(append([]string{}, writableRoots...), sandboxWorkDir))
	if err != nil {
		return Attestation{}, fmt.Errorf("materialize immutable Codex hooks: %w", err)
	}
	attestation.HookRoot = hookRoot
	return attestation, nil
}

// Verify rechecks a persisted attestation immediately before a Codex launch.
// It fails closed if the Git objects disappear, the recorded source changes
// identity, or the sandbox manifest/referenced files no longer match.
func Verify(ctx context.Context, attestation Attestation, sandboxWorkDir string) error {
	if err := validateAttestationShape(attestation); err != nil {
		return err
	}
	sourceRoot, err := gitRoot(ctx, attestation.SourceRepo)
	if err != nil {
		return fmt.Errorf("re-resolve hook source repository: %w", err)
	}
	if sourceRoot != attestation.SourceRepo {
		return fmt.Errorf("hook source repository changed identity: got %q, want %q", sourceRoot, attestation.SourceRepo)
	}
	sourceAssets, err := committedAssets(ctx, attestation)
	if err != nil {
		return err
	}
	if got := digestAssets(sourceAssets); got != attestation.Digest {
		return fmt.Errorf("committed hook digest changed: got %s, want %s", got, attestation.Digest)
	}
	if err := verifyMaterializedAssets(attestation.HookRoot, attestation.Digest, sourceAssets); err != nil {
		return err
	}
	return verifySandboxAssets(ctx, attestation, sandboxWorkDir, sourceAssets)
}

func validateAttestationShape(attestation Attestation) error {
	if !filepath.IsAbs(attestation.SourceRepo) || filepath.Clean(attestation.SourceRepo) != attestation.SourceRepo {
		return fmt.Errorf("hook source repository must be a clean absolute path")
	}
	if len(attestation.SourceCommit) != 40 && len(attestation.SourceCommit) != 64 {
		return fmt.Errorf("hook source commit is not a full Git object ID")
	}
	if _, err := hex.DecodeString(attestation.SourceCommit); err != nil {
		return fmt.Errorf("hook source commit is not hexadecimal: %w", err)
	}
	if len(attestation.Digest) != sha256.Size*2 {
		return fmt.Errorf("hook digest is not a SHA-256 value")
	}
	if _, err := hex.DecodeString(attestation.Digest); err != nil {
		return fmt.Errorf("hook digest is not hexadecimal: %w", err)
	}
	if !filepath.IsAbs(attestation.HookRoot) || filepath.Clean(attestation.HookRoot) != attestation.HookRoot {
		return fmt.Errorf("materialized hook root must be a clean absolute path")
	}
	if filepath.Base(attestation.HookRoot) != attestation.Digest {
		return fmt.Errorf("materialized hook root is not named for the attested digest")
	}
	return nil
}

func committedAssets(ctx context.Context, attestation Attestation) ([]asset, error) {
	manifest, err := committedAsset(ctx, attestation.SourceRepo, attestation.SourceCommit, hooksManifestPath)
	if err != nil {
		return nil, fmt.Errorf("read committed Codex hook manifest: %w", err)
	}
	references, err := referencedProjectFiles(manifest.content)
	if err != nil {
		return nil, fmt.Errorf("parse committed Codex hook manifest: %w", err)
	}
	assets := []asset{manifest}
	for _, reference := range references {
		item, err := committedAsset(ctx, attestation.SourceRepo, attestation.SourceCommit, reference)
		if err != nil {
			return nil, fmt.Errorf("read committed hook asset %q: %w", reference, err)
		}
		assets = append(assets, item)
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].path < assets[j].path })
	return assets, nil
}

func committedAsset(ctx context.Context, repo, commit, path string) (asset, error) {
	clean, err := cleanProjectPath(path)
	if err != nil {
		return asset{}, err
	}
	entry, err := gitOutput(ctx, repo, "ls-tree", commit, "--", clean)
	if err != nil {
		return asset{}, err
	}
	fields := strings.Fields(strings.TrimSpace(entry))
	if len(fields) < 4 || fields[1] != "blob" {
		return asset{}, fmt.Errorf("%q is not a committed blob", clean)
	}
	switch fields[0] {
	case "100644", "100755":
	default:
		return asset{}, fmt.Errorf("%q has unsupported Git mode %s", clean, fields[0])
	}
	content, err := gitOutputBytes(ctx, repo, "show", commit+":"+filepath.ToSlash(clean))
	if err != nil {
		return asset{}, err
	}
	return asset{
		path:       clean,
		gitMode:    fields[0],
		content:    content,
		executable: fields[0] == "100755",
	}, nil
}

func verifySandboxAssets(ctx context.Context, attestation Attestation, sandboxWorkDir string, sourceAssets []asset) error {
	sandboxRoot, err := gitRoot(ctx, sandboxWorkDir)
	if err != nil {
		return fmt.Errorf("resolve sandbox repository: %w", err)
	}
	if err := verifyManifestLocation(sandboxRoot, sandboxWorkDir); err != nil {
		return err
	}
	var sandboxAssets []asset
	for _, expected := range sourceAssets {
		path := filepath.Join(sandboxRoot, filepath.FromSlash(expected.path))
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect sandbox hook asset %q: %w", expected.path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("sandbox hook asset %q is not a regular file", expected.path)
		}
		if gotExecutable := info.Mode().Perm()&0o111 != 0; gotExecutable != expected.executable {
			return fmt.Errorf("sandbox hook asset %q executable mode differs from commit", expected.path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read sandbox hook asset %q: %w", expected.path, err)
		}
		sandboxAssets = append(sandboxAssets, asset{
			path:       expected.path,
			gitMode:    expected.gitMode,
			content:    content,
			executable: expected.executable,
		})
	}
	if got := digestAssets(sandboxAssets); got != attestation.Digest {
		return fmt.Errorf("sandbox hook digest differs from committed source: got %s, want %s", got, attestation.Digest)
	}
	return nil
}

func verifyManifestLocation(sandboxRoot, sandboxWorkDir string) error {
	workDir, err := filepath.EvalSymlinks(sandboxWorkDir)
	if err != nil {
		return fmt.Errorf("resolve sandbox working directory: %w", err)
	}
	workDir, err = filepath.Abs(workDir)
	if err != nil {
		return fmt.Errorf("make sandbox working directory absolute: %w", err)
	}
	workDir = filepath.Clean(workDir)
	rootManifest := filepath.Join(sandboxRoot, filepath.FromSlash(hooksManifestPath))
	for candidate := workDir; ; {
		manifestPath := filepath.Join(candidate, filepath.FromSlash(hooksManifestPath))
		if manifestPath != rootManifest {
			if _, statErr := os.Lstat(manifestPath); statErr == nil {
				return fmt.Errorf("unattested nested Codex hook manifest %q would shadow the repository manifest", manifestPath)
			} else if !os.IsNotExist(statErr) {
				return fmt.Errorf("inspect possible nested Codex hook manifest %q: %w", manifestPath, statErr)
			}
		}
		if candidate == sandboxRoot {
			break
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return fmt.Errorf("sandbox working directory %q is outside repository %q", workDir, sandboxRoot)
		}
		candidate = parent
	}
	return nil
}

func referencedProjectFiles(manifest []byte) ([]string, error) {
	var document any
	if err := json.Unmarshal(manifest, &document); err != nil {
		return nil, err
	}
	commands, err := collectCommands(document)
	if err != nil {
		return nil, err
	}
	references := make(map[string]struct{})
	for _, command := range commands {
		matches := projectDirReference.FindAllStringSubmatch(command, -1)
		for _, match := range matches {
			clean, err := cleanProjectPath(match[1])
			if err != nil {
				return nil, fmt.Errorf("command %q: %w", command, err)
			}
			references[clean] = struct{}{}
		}
		for _, match := range hookRootReference.FindAllStringSubmatch(command, -1) {
			clean, err := cleanProjectPath(match[1])
			if err != nil {
				return nil, fmt.Errorf("command %q: %w", command, err)
			}
			references[clean] = struct{}{}
		}
		unmatched := hookRootReference.ReplaceAllString(projectDirReference.ReplaceAllString(command, ""), "")
		if strings.Contains(unmatched, "PROJECT_DIR") {
			return nil, fmt.Errorf("unsupported project-directory reference in command %q", command)
		}
		for _, match := range relativePathToken.FindAllStringSubmatch(command, -1) {
			clean, err := cleanProjectPath(strings.TrimPrefix(match[1], "./"))
			if err != nil {
				return nil, fmt.Errorf("command %q: %w", command, err)
			}
			references[clean] = struct{}{}
		}
	}
	out := make([]string, 0, len(references))
	for path := range references {
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

func collectCommands(value any) ([]string, error) {
	var commands []string
	var visit func(any) error
	visit = func(node any) error {
		switch typed := node.(type) {
		case map[string]any:
			for key, child := range typed {
				if key == "command" {
					command, ok := child.(string)
					if !ok {
						return fmt.Errorf("hook command must be a string")
					}
					commands = append(commands, command)
					continue
				}
				if err := visit(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range typed {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(value); err != nil {
		return nil, err
	}
	return commands, nil
}

func cleanProjectPath(path string) (string, error) {
	if path == "" || strings.Contains(path, `\`) {
		return "", fmt.Errorf("hook asset path %q is invalid", path)
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("hook asset path %q escapes the repository", path)
	}
	return filepath.ToSlash(clean), nil
}

func digestAssets(assets []asset) string {
	h := sha256.New()
	sorted := append([]asset(nil), assets...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
	for _, item := range sorted {
		writeDigestField(h, []byte(item.path))
		writeDigestField(h, []byte(item.gitMode))
		writeDigestField(h, item.content)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeDigestField(buffer interface{ Write([]byte) (int, error) }, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = buffer.Write(length[:])
	_, _ = buffer.Write(value)
}

func gitRoot(ctx context.Context, path string) (string, error) {
	root, err := gitOutput(ctx, path, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root = strings.TrimSpace(root)
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(root), nil
}

func gitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	output, err := gitOutputBytes(ctx, repo, args...)
	return string(output), err
}

func gitOutputBytes(ctx context.Context, repo string, args ...string) ([]byte, error) {
	full := append([]string{"-C", repo}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

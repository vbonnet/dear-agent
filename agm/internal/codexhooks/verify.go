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

	"mvdan.cc/sh/v3/syntax"
)

const (
	hooksManifestPath = ".codex/hooks.json"
	systemGitPath     = "/usr/bin/git"
)

var (
	anyProjectDirRef      = regexp.MustCompile(`\$\{?(?:CLAUDE|CODEX)_PROJECT_DIR\b`)
	hookRootReference     = regexp.MustCompile(`\$\{AGM_CODEX_HOOK_ROOT:-(?:\.|\$\{CLAUDE_PROJECT_DIR:-\.\})\}/([A-Za-z0-9._/-]+)`)
	relativePathToken     = regexp.MustCompile(`(?:^|[\s"'()])((?:\./)?[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)+)`)
	runtimeDirReference   = regexp.MustCompile(`(?:\$\{(?:PWD|HOME|TMPDIR|TMP|TEMP)\}|\$(?:PWD|HOME|TMPDIR|TMP|TEMP))/[A-Za-z0-9._/-]+`)
	explicitRelativePath  = regexp.MustCompile(`(?:^|[\s"'();|&])((?:\.\.?/|~/)[A-Za-z0-9._/-]+)`)
	absolutePathToken     = regexp.MustCompile(`(?:^|[\s"'();|&])(/[A-Za-z0-9._/-]+)`)
	scriptCommandPath     = regexp.MustCompile(`(?m)(?:^|[;\n]|&&|\|\|)[\t ]*(?:[A-Za-z_][A-Za-z0-9_]*=[^ \t;|&]+[\t ]+)*((?:\.\.?/|~/)[A-Za-z0-9._/-]+|[A-Za-z0-9._-]+/[A-Za-z0-9._/-]+|/[A-Za-z0-9._/-]+)`)
	interpreterOperand    = regexp.MustCompile(`(?m)(?:^|[;\n({|&])[\t ]*(?:(?:if|then|elif|while|until|do|exec|command|!)[\t ]+)*(?:[A-Za-z_][A-Za-z0-9_]*=[^ \t;|&]+[\t ]+)*(?:(?:/bin/|/usr/bin/)?(?:sh|bash|dash|zsh|ksh|python[0-9.]*|perl|ruby|node))[\t ]+((?:--?[A-Za-z0-9_-]+[\t ]+)*)([^ \t\r\n;|&]+)`)
	envInterpreterOperand = regexp.MustCompile(`(?m)(?:^|[;\n({|&])[\t ]*(?:(?:if|then|elif|while|until|do|exec|command|!)[\t ]+)*(?:[A-Za-z_][A-Za-z0-9_]*=[^ \t;|&]+[\t ]+)*/usr/bin/env[\t ]+(?:--?[A-Za-z0-9_-]+[\t ]+)*(?:sh|bash|dash|zsh|ksh|python[0-9.]*|perl|ruby|node)[\t ]+((?:--?[A-Za-z0-9_-]+[\t ]+)*)([^ \t\r\n;|&]+)`)
	sourceOperand         = regexp.MustCompile(`(?m)(?:^|[;\n({]|&&|\|\|)[\t ]*(?:(?:if|then|elif|while|until|do|exec|command|!)[\t ]+)*(?:source|\.)[\t ]+([^ \t\r\n;|&]+)`)
	pathAssignment        = regexp.MustCompile(`(?m)(?:^|[;\n({|&])[\t ]*(?:(?:if|then|elif|while|until|do|exec|command|!)[\t ]+)*(?:(?:/usr/bin/)?env[\t ]+(?:--?[A-Za-z0-9_-]+(?:=[^ \t;|&]+)?[\t ]+)*)?(?:[A-Za-z_][A-Za-z0-9_]*=[^ \t;|&]+[\t ]+)*(?:(?:export|readonly|typeset|declare|local)[\t ]+)?PATH(?:\+)?[\t ]*=`)
	pathUnset             = regexp.MustCompile(`(?m)(?:^|[;\n({|&])[\t ]*(?:(?:if|then|elif|while|until|do|exec|command|!)[\t ]+)*(?:unset|export[\t ]+-n)[\t ]+(?:--[\t ]+)?PATH(?:[\t ;\n]|$)`)
	envPathUnset          = regexp.MustCompile(`(?m)(?:^|[;\n({|&])[\t ]*(?:(?:if|then|elif|while|until|do|exec|command|!)[\t ]+)*(?:/usr/bin/)?env[\t ]+(?:-[^- \t]*u|--unset)(?:=|[\t ]+)PATH(?:[\t ;\n]|$)`)
)

// Attestation pins hook trust to immutable Git objects and their exact
// materialization in one sandbox workspace.
type Attestation struct {
	SourceRepo   string
	SourceCommit string
	Digest       string
	HookRoot     string
}

// SourceIdentity is the immutable repository/commit/hook-byte identity an
// operator approves before AGM may bypass Codex's per-path hook prompt.
type SourceIdentity struct {
	SourceRepo   string
	SourceCommit string
	Digest       string
}

type asset struct {
	path       string
	gitMode    string
	content    []byte
	executable bool
}

// Attest records the source repository's current commit and verifies that the
// sandbox hook manifest and every trusted command asset are byte-identical to
// regular files in that commit. Source content is read from Git objects, never
// from the mutable source working tree.
func Attest(
	ctx context.Context,
	sourceRepo, sandboxWorkDir, storeBase string,
	writableRoots []string,
) (Attestation, error) {
	identity, sourceAssets, err := inspectSource(ctx, sourceRepo)
	if err != nil {
		return Attestation{}, err
	}
	if err := rejectWritableOverlap("hook source repository", identity.SourceRepo, writableRoots); err != nil {
		return Attestation{}, err
	}
	attestation := Attestation{
		SourceRepo:   identity.SourceRepo,
		SourceCommit: identity.SourceCommit,
		Digest:       identity.Digest,
	}
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

// InspectSource resolves the exact immutable hook identity that an operator is
// about to approve. It reads committed Git objects rather than mutable working
// tree bytes and performs no materialization or sandbox mutation.
func InspectSource(ctx context.Context, sourceRepo string) (SourceIdentity, error) {
	identity, _, err := inspectSource(ctx, sourceRepo)
	return identity, err
}

func inspectSource(ctx context.Context, sourceRepo string) (SourceIdentity, []asset, error) {
	sourceRoot, err := gitRoot(ctx, sourceRepo)
	if err != nil {
		return SourceIdentity{}, nil, fmt.Errorf("resolve hook source repository: %w", err)
	}
	commit, err := gitOutput(ctx, sourceRoot, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return SourceIdentity{}, nil, fmt.Errorf("resolve hook source commit: %w", err)
	}
	attestation := Attestation{
		SourceRepo:   sourceRoot,
		SourceCommit: strings.TrimSpace(commit),
	}
	sourceAssets, err := committedAssets(ctx, attestation)
	if err != nil {
		return SourceIdentity{}, nil, err
	}
	identity := SourceIdentity{
		SourceRepo:   attestation.SourceRepo,
		SourceCommit: attestation.SourceCommit,
		Digest:       digestAssets(sourceAssets),
	}
	return identity, sourceAssets, nil
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
	references, err := referencedHookAssets(manifest.content)
	if err != nil {
		return nil, fmt.Errorf("parse committed Codex hook manifest: %w", err)
	}
	assets := []asset{manifest}
	seen := map[string]struct{}{hooksManifestPath: {}}
	for len(references) > 0 {
		reference := references[0]
		references = references[1:]
		if _, exists := seen[reference]; exists {
			continue
		}
		item, err := committedAsset(ctx, attestation.SourceRepo, attestation.SourceCommit, reference)
		if err != nil {
			return nil, fmt.Errorf("read committed hook asset %q: %w", reference, err)
		}
		if err := validateHookInterpreter(item); err != nil {
			return nil, err
		}
		seen[reference] = struct{}{}
		assets = append(assets, item)
		if err := validateScriptAsset(item.content); err != nil {
			return nil, fmt.Errorf("parse committed hook asset %q: %w", reference, err)
		}
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].path < assets[j].path })
	return assets, nil
}

func validateHookInterpreter(item asset) error {
	if !item.executable {
		return nil
	}
	firstLine, _, _ := bytes.Cut(item.content, []byte("\n"))
	switch string(bytes.TrimSuffix(firstLine, []byte("\r"))) {
	case "#!/bin/bash", "#!/bin/sh":
		return nil
	default:
		return fmt.Errorf(
			"committed hook asset %q must use a trusted absolute interpreter (/bin/bash or /bin/sh)",
			item.path,
		)
	}
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

func referencedHookAssets(manifest []byte) ([]string, error) {
	var document struct {
		Hooks map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(manifest, &document); err != nil {
		return nil, err
	}
	if document.Hooks == nil {
		return nil, fmt.Errorf("codex hook manifest has no hooks object")
	}
	references := make(map[string]struct{})
	for eventName, event := range document.Hooks {
		// These commands are replaced with an OS-owned no-op before the
		// attested manifest reaches Codex. They intentionally reference
		// mutable workspace state in the ordinary, reviewed launch mode.
		if _, neutralized := neutralizedAttestedHookEvents[eventName]; neutralized {
			continue
		}
		commands, err := collectCommands(event)
		if err != nil {
			return nil, err
		}
		for _, command := range commands {
			if err := addTrustedCommandAssets(references, command); err != nil {
				return nil, err
			}
		}
	}
	out := make([]string, 0, len(references))
	for path := range references {
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

func addTrustedCommandAssets(references map[string]struct{}, command string) error {
	for _, match := range hookRootReference.FindAllStringSubmatch(command, -1) {
		clean, err := cleanProjectPath(match[1])
		if err != nil {
			return fmt.Errorf("command %q: %w", command, err)
		}
		references[clean] = struct{}{}
	}
	unmatched := hookRootReference.ReplaceAllString(command, "")
	if mutatesExecutableSearchPath(unmatched) {
		return fmt.Errorf("command %q mutates PATH; trusted hooks must retain the hardened executable search path", command)
	}
	if err := rejectExecutionInfluencingEnvironment(unmatched); err != nil {
		return fmt.Errorf("command %q: %w", command, err)
	}
	if containsMutableRuntimePath(unmatched) {
		return fmt.Errorf(
			"unsupported mutable runtime path in command %q; trusted hooks must execute committed assets through AGM_CODEX_HOOK_ROOT",
			command,
		)
	}
	if err := rejectDynamicCommandResolution(unmatched); err != nil {
		return fmt.Errorf("command %q: %w", command, err)
	}
	if err := rejectMutableScriptOperands(unmatched); err != nil {
		return fmt.Errorf("command %q: %w", command, err)
	}
	return nil
}

func containsMutableRuntimePath(command string) bool {
	return anyProjectDirRef.MatchString(command) ||
		runtimeDirReference.MatchString(command) ||
		explicitRelativePath.MatchString(command) ||
		relativePathToken.MatchString(command) ||
		containsMutableAbsolutePath(command)
}

func containsMutableAbsolutePath(command string) bool {
	for _, match := range absolutePathToken.FindAllStringSubmatch(command, -1) {
		if isSystemRuntimePath(match[1]) {
			continue
		}
		return true
	}
	return false
}

// validateScriptAsset closes the transitive trust gap for enabled hook scripts.
// A manifest-level executable can be embedded into the immutable session
// configuration, but that script cannot safely delegate to a same-user-owned
// materialized child after launch.
func validateScriptAsset(content []byte) error {
	script := string(content)
	for _, match := range hookRootReference.FindAllStringSubmatch(script, -1) {
		return fmt.Errorf(
			"trusted hook script references materialized child asset %q; only manifest-level executables can be embedded into the immutable session configuration",
			match[1],
		)
	}

	unmatched := hookRootReference.ReplaceAllString(script, "")
	scannable := withoutFullLineComments(unmatched)
	if mutatesExecutableSearchPath(scannable) {
		return fmt.Errorf("trusted hook script mutates PATH; trusted hooks must retain the hardened executable search path")
	}
	if err := rejectExecutionInfluencingEnvironment(scannable); err != nil {
		return err
	}
	if anyProjectDirRef.MatchString(unmatched) ||
		runtimeDirReference.MatchString(unmatched) {
		return fmt.Errorf(
			"unsupported mutable runtime path; trusted hook scripts must execute committed assets through AGM_CODEX_HOOK_ROOT",
		)
	}
	for _, match := range scriptCommandPath.FindAllStringSubmatch(unmatched, -1) {
		path := match[1]
		if filepath.IsAbs(path) && isSystemRuntimePath(path) {
			continue
		}
		return fmt.Errorf(
			"unsupported mutable command path %q; trusted hook scripts must execute committed assets through AGM_CODEX_HOOK_ROOT",
			path,
		)
	}
	if err := rejectDynamicCommandResolution(scannable); err != nil {
		return err
	}
	if err := rejectMutableScriptOperands(scannable); err != nil {
		return err
	}

	return nil
}

func rejectExecutionInfluencingEnvironment(script string) error {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).
		Parse(strings.NewReader(script), "trusted-hook")
	if err != nil {
		return fmt.Errorf("parse trusted hook shell: %w", err)
	}
	var rejected syntax.Node
	var name string
	syntax.Walk(file, func(node syntax.Node) bool {
		if rejected != nil {
			return false
		}
		switch typed := node.(type) {
		case *syntax.CallExpr:
			for _, assignment := range typed.Assigns {
				if name = executionInfluencingEnvironmentName(assignment); name != "" {
					rejected = assignment
					return false
				}
			}
			if word, found := envWrapperEnvironmentAssignment(typed.Args); word != nil {
				rejected, name = word, found
				return false
			}
		case *syntax.DeclClause:
			for _, assignment := range typed.Args {
				if assignment.Name == nil && assignment.Value != nil {
					rejected, name = assignment, "<dynamic environment name>"
					return false
				}
				if name = executionInfluencingEnvironmentName(assignment); name != "" {
					rejected = assignment
					return false
				}
			}
		}
		return true
	})
	if rejected == nil {
		return nil
	}
	return fmt.Errorf(
		"unsupported execution-influencing environment assignment %q at line %d; trusted hooks must not load unverified runtime code",
		name, rejected.Pos().Line(),
	)
}

func executionInfluencingEnvironmentName(assignment *syntax.Assign) string {
	if assignment == nil || assignment.Name == nil {
		return ""
	}
	if isExecutionInfluencingEnvironment(assignment.Name.Value) {
		return assignment.Name.Value
	}
	return ""
}

func envWrapperEnvironmentAssignment(args []*syntax.Word) (*syntax.Word, string) {
	if len(args) == 0 {
		return nil, ""
	}
	command, static := staticShellWord(args[0])
	if !static {
		return nil, ""
	}
	switch command {
	case "command", "builtin", "exec", "nohup":
		return envWrapperEnvironmentAssignment(args[1:])
	case "env", "/usr/bin/env":
		return envCommandEnvironmentAssignment(args[1:])
	case "declare", "export", "local", "nameref", "readonly", "typeset":
		return declarationCommandEnvironmentAssignment(args[1:])
	default:
		return nil, ""
	}
}

func envCommandEnvironmentAssignment(args []*syntax.Word) (*syntax.Word, string) {
	for index := 0; index < len(args); index++ {
		word := args[index]
		value, static := staticShellWord(word)
		if !static {
			return nil, ""
		}
		if value == "--" {
			continue
		}
		if envOptionConsumesNext(value) {
			index++
			continue
		}
		if strings.HasPrefix(value, "-") {
			continue
		}
		name, _, assignment := strings.Cut(value, "=")
		if assignment {
			if isExecutionInfluencingEnvironment(name) {
				return word, name
			}
			continue
		}
		return envWrapperEnvironmentAssignment(args[index:])
	}
	return nil, ""
}

func envOptionConsumesNext(value string) bool {
	switch value {
	case "-a", "--argv0", "-C", "--chdir", "-u", "--unset":
		return true
	default:
		return false
	}
}

func declarationCommandEnvironmentAssignment(args []*syntax.Word) (*syntax.Word, string) {
	for _, word := range args {
		value, static := staticShellWord(word)
		if !static {
			return word, "<dynamic environment name>"
		}
		if value == "--" || strings.HasPrefix(value, "-") {
			continue
		}
		name, _, _ := strings.Cut(value, "=")
		if isExecutionInfluencingEnvironment(name) {
			return word, name
		}
	}
	return nil, ""
}

func isExecutionInfluencingEnvironment(name string) bool {
	if strings.HasPrefix(name, "LD_") ||
		strings.HasPrefix(name, "DYLD_") ||
		strings.HasPrefix(name, "_RLD_") {
		return true
	}
	switch name {
	case "BASH_ENV", "ENV", "SHELLOPTS", "BASHOPTS",
		"GCONV_PATH", "LOCPATH", "LIBPATH", "SHLIB_PATH",
		"LDR_PRELOAD", "LDR_LIBRARY_PATH",
		"NODE_OPTIONS", "NODE_PATH",
		"PYTHONHOME", "PYTHONPATH", "PYTHONSTARTUP",
		"PERL5LIB", "PERL5OPT", "RUBYLIB", "RUBYOPT",
		"CLASSPATH", "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS":
		return true
	default:
		return false
	}
}

func rejectDynamicCommandResolution(script string) error {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).
		Parse(strings.NewReader(script), "trusted-hook")
	if err != nil {
		return fmt.Errorf("parse trusted hook shell: %w", err)
	}
	var rejected *syntax.Word
	var reason string
	syntax.Walk(file, func(node syntax.Node) bool {
		if rejected != nil {
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		rejected, reason = dynamicCommandTarget(call)
		return rejected == nil
	})
	if rejected == nil {
		return nil
	}
	return fmt.Errorf(
		"unsupported dynamic command resolution at line %d (%s); trusted hook commands must be literal committed or system-path executables",
		rejected.Pos().Line(),
		reason,
	)
}

func dynamicCommandTarget(call *syntax.CallExpr) (*syntax.Word, string) {
	command, static := staticShellWord(call.Args[0])
	if !static {
		return call.Args[0], "expanded command word"
	}
	if commandResolutionStateBuiltin(command) {
		return call.Args[0], "command-resolution state builtin"
	}
	switch command {
	case "eval", "trap":
		return call.Args[0], "string-evaluating shell builtin"
	case "command", "builtin", "exec", "nohup":
		return dynamicWrapperCommand(call.Args[1:], false)
	case "env", "/usr/bin/env":
		return dynamicWrapperCommand(call.Args[1:], true)
	default:
		return nil, ""
	}
}

func dynamicWrapperCommand(args []*syntax.Word, env bool) (*syntax.Word, string) {
	for index, word := range args {
		value, static := staticShellWord(word)
		if !static {
			return word, "expanded command-wrapper operand"
		}
		if env && envSplitStringOption(value) {
			return word, "env split-string command"
		}
		if value == "--" || strings.HasPrefix(value, "-") {
			continue
		}
		if env && strings.Contains(value, "=") {
			continue
		}
		if commandResolutionStateBuiltin(value) {
			return word, "command-resolution state builtin"
		}
		switch value {
		case "eval", "trap":
			return word, "string-evaluating shell builtin"
		case "command", "builtin", "exec", "nohup":
			return dynamicWrapperCommand(args[index+1:], false)
		case "env", "/usr/bin/env":
			return dynamicWrapperCommand(args[index+1:], true)
		default:
			return nil, ""
		}
	}
	return nil, ""
}

func envSplitStringOption(value string) bool {
	return value == "-S" ||
		value == "--split-string" ||
		strings.HasPrefix(value, "-S") ||
		strings.HasPrefix(value, "--split-string=")
}

func commandResolutionStateBuiltin(command string) bool {
	switch command {
	case "alias", "unalias", "hash", "enable":
		return true
	default:
		return false
	}
}

func staticShellWord(word *syntax.Word) (string, bool) {
	var value strings.Builder
	if !appendStaticShellParts(&value, word.Parts) {
		return "", false
	}
	return value.String(), true
}

func appendStaticShellParts(value *strings.Builder, parts []syntax.WordPart) bool {
	for _, part := range parts {
		switch typed := part.(type) {
		case *syntax.Lit:
			value.WriteString(typed.Value)
		case *syntax.SglQuoted:
			value.WriteString(typed.Value)
		case *syntax.DblQuoted:
			if !appendStaticShellParts(value, typed.Parts) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func mutatesExecutableSearchPath(script string) bool {
	return pathAssignment.MatchString(script) ||
		pathUnset.MatchString(script) ||
		envPathUnset.MatchString(script)
}

func withoutFullLineComments(script string) string {
	lines := strings.Split(script, "\n")
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			lines[index] = ""
		}
	}
	return strings.Join(lines, "\n")
}

func rejectMutableScriptOperands(script string) error {
	for _, matcher := range []*regexp.Regexp{
		envInterpreterOperand,
		interpreterOperand,
	} {
		for _, match := range matcher.FindAllStringSubmatch(script, -1) {
			if interpreterRunsInlineCode(strings.Fields(match[1])) {
				return fmt.Errorf(
					"unsupported inline interpreter code; trusted hook scripts must invoke committed scripts through AGM_CODEX_HOOK_ROOT",
				)
			}
			operand := strings.Trim(match[2], `"'`)
			if operand == "" || (filepath.IsAbs(operand) && isSystemRuntimePath(operand)) {
				continue
			}
			return fmt.Errorf(
				"unsupported mutable interpreter or sourced-file operand %q; trusted hook scripts must reference committed operands through AGM_CODEX_HOOK_ROOT",
				operand,
			)
		}
	}
	for _, match := range sourceOperand.FindAllStringSubmatch(script, -1) {
		operand := strings.Trim(match[1], `"'`)
		if operand == "" || (filepath.IsAbs(operand) && isSystemRuntimePath(operand)) {
			continue
		}
		return fmt.Errorf(
			"unsupported mutable interpreter or sourced-file operand %q; trusted hook scripts must reference committed operands through AGM_CODEX_HOOK_ROOT",
			operand,
		)
	}
	return nil
}

func interpreterRunsInlineCode(options []string) bool {
	for _, option := range options {
		switch option {
		case "-c", "-e", "--eval", "--print", "-p":
			return true
		}
	}
	return false
}

func isSystemRuntimePath(path string) bool {
	path = filepath.Clean(path)
	return path == "/bin" || strings.HasPrefix(path, "/bin/") ||
		path == "/sbin" || strings.HasPrefix(path, "/sbin/") ||
		path == "/usr/bin" || strings.HasPrefix(path, "/usr/bin/") ||
		path == "/usr/sbin" || strings.HasPrefix(path, "/usr/sbin/") ||
		path == "/usr/local/libexec" || strings.HasPrefix(path, "/usr/local/libexec/")
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
	gitPath, err := trustedGitExecutable()
	if err != nil {
		return nil, err
	}
	full := append([]string{"-C", repo}, args...)
	cmd := exec.CommandContext(ctx, gitPath, full...)
	cmd.Env = []string{
		"HOME=/var/empty",
		"LC_ALL=C",
		"PATH=/usr/bin:/bin",
		"XDG_CONFIG_HOME=/var/empty",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}

func trustedGitExecutable() (string, error) {
	path, err := filepath.EvalSymlinks(systemGitPath)
	if err != nil {
		return "", fmt.Errorf("resolve trusted Git executable %s: %w", systemGitPath, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect trusted Git executable %s: %w", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("trusted Git executable %s is not an executable regular file", path)
	}
	return path, nil
}

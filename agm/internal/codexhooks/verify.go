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
	"slices"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	hooksManifestPath = ".codex/hooks.json"
	systemGitPath     = "/usr/bin/git"
)

var (
	anyProjectDirRef     = regexp.MustCompile(`\$\{?(?:CLAUDE|CODEX)_PROJECT_DIR\b`)
	hookRootReference    = regexp.MustCompile(`\$\{AGM_CODEX_HOOK_ROOT:-(?:\.|\$\{CLAUDE_PROJECT_DIR:-\.\})\}/([A-Za-z0-9._/-]+)`)
	relativePathToken    = regexp.MustCompile(`(?:^|[\s"'()])((?:\./)?[A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)+)`)
	runtimeDirReference  = regexp.MustCompile(`(?:\$\{(?:PWD|HOME|TMPDIR|TMP|TEMP)\}|\$(?:PWD|HOME|TMPDIR|TMP|TEMP))/[A-Za-z0-9._/-]+`)
	explicitRelativePath = regexp.MustCompile(`(?:^|[\s"'();|&])((?:\.\.?/|~/)[A-Za-z0-9._/-]+)`)
	absolutePathToken    = regexp.MustCompile(`(?:^|[\s"'();|&])(/[A-Za-z0-9._/-]+)`)
	scriptCommandPath    = regexp.MustCompile(`(?m)(?:^|[;\n]|&&|\|\|)[\t ]*(?:[A-Za-z_][A-Za-z0-9_]*=[^ \t;|&]+[\t ]+)*((?:\.\.?/|~/)[A-Za-z0-9._/-]+|[A-Za-z0-9._-]+/[A-Za-z0-9._/-]+|/[A-Za-z0-9._/-]+)`)
	pathAssignment       = regexp.MustCompile(`(?m)(?:^|[;\n({|&])[\t ]*(?:(?:if|then|elif|while|until|do|exec|command|!)[\t ]+)*(?:(?:/usr/bin/)?env[\t ]+(?:--?[A-Za-z0-9_-]+(?:=[^ \t;|&]+)?[\t ]+)*)?(?:[A-Za-z_][A-Za-z0-9_]*=[^ \t;|&]+[\t ]+)*(?:(?:export|readonly|typeset|declare|local)[\t ]+)?PATH(?:\+)?[\t ]*=`)
	pathUnset            = regexp.MustCompile(`(?m)(?:^|[;\n({|&])[\t ]*(?:(?:if|then|elif|while|until|do|exec|command|!)[\t ]+)*(?:unset|export[\t ]+-n)[\t ]+(?:--[\t ]+)?PATH(?:[\t ;\n]|$)`)
	envPathUnset         = regexp.MustCompile(`(?m)(?:^|[;\n({|&])[\t ]*(?:(?:if|then|elif|while|until|do|exec|command|!)[\t ]+)*(?:/usr/bin/)?env[\t ]+(?:-[^- \t]*u|--unset)(?:=|[\t ]+)PATH(?:[\t ;\n]|$)`)
	awkSystemCall        = regexp.MustCompile(`\bsystem[[:space:]]*\(`)
	awkGetline           = regexp.MustCompile(`\bgetline\b`)
	awkOutputPipe        = regexp.MustCompile(`\b(?:print|printf)\b[^;\n]*\|`)
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
	if err := rejectMutableInputRedirections(unmatched); err != nil {
		return fmt.Errorf("command %q: %w", command, err)
	}
	if err := rejectInterpreterPipelines(unmatched); err != nil {
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
	if err := rejectMutableInputRedirections(scannable); err != nil {
		return err
	}
	if err := rejectInterpreterPipelines(scannable); err != nil {
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
		rejected, name = executionInfluencingNode(node)
		return rejected == nil
	})
	if rejected == nil {
		return nil
	}
	return fmt.Errorf(
		"unsupported execution-influencing environment assignment %q at line %d; trusted hooks must not load unverified runtime code",
		name, rejected.Pos().Line(),
	)
}

func executionInfluencingNode(node syntax.Node) (syntax.Node, string) {
	switch typed := node.(type) {
	case *syntax.CallExpr:
		return executionInfluencingCall(typed)
	case *syntax.DeclClause:
		return executionInfluencingDeclaration(typed)
	case *syntax.ForClause:
		return executionInfluencingIteration(typed)
	case *syntax.ParamExp:
		return executionInfluencingParameterExpansion(typed)
	default:
		return nil, ""
	}
}

func executionInfluencingCall(call *syntax.CallExpr) (syntax.Node, string) {
	for _, assignment := range call.Assigns {
		if name := executionInfluencingEnvironmentName(assignment); name != "" {
			return assignment, name
		}
	}
	word, name := envWrapperEnvironmentAssignment(call.Args)
	if word == nil {
		return nil, ""
	}
	return word, name
}

func executionInfluencingDeclaration(declaration *syntax.DeclClause) (syntax.Node, string) {
	for _, assignment := range declaration.Args {
		if assignment.Name == nil && assignment.Value != nil {
			return assignment, "<dynamic environment name>"
		}
		if name := executionInfluencingEnvironmentName(assignment); name != "" {
			return assignment, name
		}
	}
	return nil, ""
}

func executionInfluencingIteration(clause *syntax.ForClause) (syntax.Node, string) {
	iteration, ok := clause.Loop.(*syntax.WordIter)
	if !ok || iteration.Name == nil || !isExecutionInfluencingEnvironment(iteration.Name.Value) {
		return nil, ""
	}
	return iteration.Name, iteration.Name.Value
}

func executionInfluencingParameterExpansion(expansion *syntax.ParamExp) (syntax.Node, string) {
	if expansion.Param == nil ||
		expansion.Exp == nil ||
		(expansion.Exp.Op != syntax.AssignUnset && expansion.Exp.Op != syntax.AssignUnsetOrNull) ||
		!isExecutionInfluencingEnvironment(expansion.Param.Value) {
		return nil, ""
	}
	return expansion, expansion.Param.Value
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
	switch filepath.Base(command) {
	case "command", "builtin", "exec", "nohup":
		return envWrapperEnvironmentAssignment(args[1:])
	case "env":
		return envCommandEnvironmentAssignment(args[1:])
	}
	switch command {
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
		strings.HasPrefix(name, "_RLD_") ||
		strings.HasPrefix(name, "GIT_CONFIG_") {
		return true
	}
	switch name {
	case "PATH",
		"BASH_ENV", "ENV", "SHELLOPTS", "BASHOPTS",
		"TAR_OPTIONS",
		"GIT_ASKPASS", "GIT_EDITOR", "GIT_EXEC_PATH", "GIT_EXTERNAL_DIFF",
		"GIT_PAGER", "GIT_SEQUENCE_EDITOR", "GIT_SSH", "GIT_SSH_COMMAND",
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
	var rejected syntax.Node
	var reason string
	syntax.Walk(file, func(node syntax.Node) bool {
		if rejected != nil {
			return false
		}
		if _, ok := node.(*syntax.LetClause); ok {
			rejected = node
			reason = "stateful variable-writing shell builtin"
			return false
		}
		if arithmeticNodeWritesVariable(node) {
			rejected = node
			reason = "stateful variable-writing arithmetic expression"
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		word, why := dynamicCommandTarget(call)
		if word != nil {
			rejected = word
			reason = why
		}
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

func arithmeticNodeWritesVariable(node syntax.Node) bool {
	switch arithmetic := node.(type) {
	case *syntax.BinaryArithm:
		return slices.Contains([]syntax.BinAritOperator{
			syntax.Assgn,
			syntax.AddAssgn,
			syntax.SubAssgn,
			syntax.MulAssgn,
			syntax.QuoAssgn,
			syntax.RemAssgn,
			syntax.AndAssgn,
			syntax.OrAssgn,
			syntax.XorAssgn,
			syntax.ShlAssgn,
			syntax.ShrAssgn,
			syntax.AndBoolAssgn,
			syntax.OrBoolAssgn,
			syntax.XorBoolAssgn,
			syntax.PowAssgn,
		}, arithmetic.Op)
	case *syntax.UnaryArithm:
		return arithmetic.Op == syntax.Inc || arithmetic.Op == syntax.Dec
	}
	return false
}

func dynamicCommandTarget(call *syntax.CallExpr) (*syntax.Word, string) {
	command, static := staticShellWord(call.Args[0])
	if !static {
		return call.Args[0], "expanded command word"
	}
	return dynamicNormalizedCommand(command, call.Args[0], call.Args[1:])
}

func dynamicNormalizedCommand(
	command string,
	commandWord *syntax.Word,
	args []*syntax.Word,
) (*syntax.Word, string) {
	if rejected, reason := dynamicBuiltinOperand(command, commandWord, args); rejected != nil {
		return rejected, reason
	}
	if commandResolutionStateBuiltin(command) {
		return commandWord, "command-resolution state builtin"
	}
	if target, reason, handled := dynamicRuntimeOperand(command, commandWord, args); handled {
		return target, reason
	}
	if target, reason, handled := nestedCommandWrapperOperand(command, args); handled {
		return target, reason
	}
	switch filepath.Base(command) {
	case "command", "builtin", "exec", "nohup", "env":
		return dynamicWrapperCommand(command, args)
	default:
		return nil, ""
	}
}

func dynamicWrapperCommand(wrapper string, args []*syntax.Word) (*syntax.Word, string) {
	env := filepath.Base(wrapper) == "env"
	options := true
	for index := 0; index < len(args); index++ {
		word := args[index]
		value, static := staticShellWord(word)
		if !static {
			return word, "expanded command-wrapper operand"
		}
		if options && env && envSplitStringOption(value) {
			return word, "env split-string command"
		}
		if options && value == "--" {
			options = false
			continue
		}
		if options && dynamicWrapperOptionConsumesNext(wrapper, value) {
			nextIndex, rejected, reason := dynamicWrapperOptionOperand(args, index)
			if rejected != nil {
				return rejected, reason
			}
			index = nextIndex
			continue
		}
		if options && strings.HasPrefix(value, "-") {
			continue
		}
		if env && strings.Contains(value, "=") {
			continue
		}
		return dynamicNormalizedCommand(value, word, args[index+1:])
	}
	return nil, ""
}

func dynamicWrapperOptionOperand(
	args []*syntax.Word,
	optionIndex int,
) (int, *syntax.Word, string) {
	if optionIndex+1 >= len(args) {
		return optionIndex, args[optionIndex], "command-wrapper option missing operand"
	}
	operandIndex := optionIndex + 1
	if _, static := staticShellWord(args[operandIndex]); !static {
		return operandIndex, args[operandIndex], "expanded command-wrapper option operand"
	}
	return operandIndex, nil, ""
}

func dynamicWrapperOptionConsumesNext(wrapper, value string) bool {
	switch filepath.Base(wrapper) {
	case "exec":
		return value == "-a" || value == "--argv0"
	case "env":
		return envOptionConsumesNext(value) || value == "-P"
	default:
		return false
	}
}

func nestedCommandWrapperOperand(command string, args []*syntax.Word) (*syntax.Word, string, bool) {
	if target, reason, handled := timeoutCommandOperand(command, args); handled {
		return target, reason, true
	}
	return niceCommandOperand(command, args)
}

func dynamicRuntimeOperand(command string, commandWord *syntax.Word, args []*syntax.Word) (*syntax.Word, string, bool) {
	if isRuntimeInterpreter(command) {
		target, reason := interpreterScriptOperand(command, args)
		return target, reason, true
	}
	if filepath.Base(command) == "xargs" {
		if xargsRunsCustomCommand(args) {
			return commandWord, "command-capable xargs runtime", true
		}
		return nil, "", true
	}
	switch command {
	case "eval", "trap":
		return commandWord, "string-evaluating shell builtin", true
	case "source", ".":
		target, reason := sourcedFileOperand(args)
		return target, reason, true
	default:
		return nil, "", false
	}
}

func timeoutCommandOperand(command string, args []*syntax.Word) (*syntax.Word, string, bool) {
	if filepath.Base(command) != "timeout" {
		return nil, "", false
	}
	options := true
	for index := 0; index < len(args); index++ {
		word := args[index]
		value, static := staticShellWord(word)
		if !static {
			return word, "expanded timeout command-wrapper operand", true
		}
		if options && value == "--" {
			options = false
			continue
		}
		if options && timeoutOptionTakesArgument(value) {
			if index+1 >= len(args) {
				return nil, "", true
			}
			index++
			if _, static := staticShellWord(args[index]); !static {
				return args[index], "expanded timeout option operand", true
			}
			continue
		}
		if options && strings.HasPrefix(value, "-") {
			continue
		}
		if index+1 >= len(args) {
			return nil, "", true
		}
		target, reason := dynamicWrapperCommand("command", args[index+1:])
		return target, reason, true
	}
	return nil, "", true
}

func timeoutOptionTakesArgument(value string) bool {
	switch value {
	case "-k", "--kill-after", "-s", "--signal":
		return true
	default:
		return false
	}
}

func niceCommandOperand(command string, args []*syntax.Word) (*syntax.Word, string, bool) {
	if filepath.Base(command) != "nice" {
		return nil, "", false
	}
	options := true
	for index := 0; index < len(args); index++ {
		word := args[index]
		value, static := staticShellWord(word)
		if !static {
			return word, "expanded nice command-wrapper operand", true
		}
		if options && value == "--" {
			options = false
			continue
		}
		if options && (value == "-n" || value == "--adjustment") {
			if index+1 >= len(args) {
				return nil, "", true
			}
			index++
			if _, static := staticShellWord(args[index]); !static {
				return args[index], "expanded nice option operand", true
			}
			continue
		}
		if options && strings.HasPrefix(value, "-") {
			continue
		}
		target, reason := dynamicWrapperCommand("command", args[index:])
		return target, reason, true
	}
	return nil, "", true
}

func awkUsesCommandExecution(command string, args []*syntax.Word) bool {
	switch filepath.Base(command) {
	case "awk", "nawk", "mawk", "gawk":
	default:
		return false
	}
	program, static := staticAWKProgram(args)
	return !static || awkProgramExecutesCommand(program)
}

func staticAWKProgram(args []*syntax.Word) (string, bool) {
	for index := 0; index < len(args); index++ {
		value, static := staticShellWord(args[index])
		if !static {
			return "", false
		}
		if value == "--" {
			if index+1 >= len(args) {
				return "", false
			}
			return staticShellWord(args[index+1])
		}
		if value == "-f" || value == "--file" ||
			strings.HasPrefix(value, "-f") ||
			strings.HasPrefix(value, "--file=") {
			return "", false
		}
		if value == "-F" || value == "-v" || value == "--assign" {
			index++
			continue
		}
		if strings.HasPrefix(value, "-") {
			continue
		}
		if strings.Contains(value, "=") && !strings.ContainsAny(value, "{}") {
			continue
		}
		return value, true
	}
	return "", false
}

func awkProgramExecutesCommand(program string) bool {
	return awkSystemCall.MatchString(program) ||
		awkGetline.MatchString(program) ||
		awkOutputPipe.MatchString(program)
}

func sedUsesCommandExecution(command string, args []*syntax.Word) bool {
	switch filepath.Base(command) {
	case "sed", "gsed":
	default:
		return false
	}

	if len(args) > 0 {
		first, static := staticShellWord(args[0])
		if static && first == "--sandbox" {
			return false
		}
	}
	programs, static := staticSedPrograms(args)
	if !static {
		return true
	}
	return slices.ContainsFunc(programs, sedProgramExecutesCommand)
}

func findUsesCommandExecution(command string, args []*syntax.Word) bool {
	if filepath.Base(command) != "find" {
		return false
	}
	return slices.ContainsFunc(args, func(argument *syntax.Word) bool {
		value, static := staticShellWord(argument)
		if !static {
			return true
		}
		switch value {
		case "-exec", "-execdir", "-ok", "-okdir":
			return true
		default:
			return false
		}
	})
}

func makeUsesCommandExecution(command string) bool {
	switch filepath.Base(command) {
	case "make", "gmake":
		// Make evaluates recipes from makefiles, --eval, environment-provided
		// MAKEFLAGS, and command-line variable assignments. Even a literal
		// system-path executable can therefore dispatch mutable workspace
		// commands that were not part of the hook attestation.
		return true
	default:
		return false
	}
}

func sshUsesCommandExecution(command string) bool {
	switch filepath.Base(command) {
	case "ssh", "scp", "sftp":
		// OpenSSH clients can dispatch local commands through user-controlled
		// configuration and options such as ProxyCommand and LocalCommand.
		// Reject the client family instead of trying to parse every config
		// source and percent/token expansion that can influence execution.
		return true
	default:
		return false
	}
}

func tarUsesCommandExecution(command string, args []*syntax.Word) bool {
	switch filepath.Base(command) {
	case "tar", "gtar", "bsdtar":
	default:
		return false
	}

	operandsOnly := false
	for index, argument := range args {
		if operandsOnly {
			continue
		}
		value, static := staticShellWord(argument)
		if !static {
			// An expanded pre-"--" operand can introduce a command-capable
			// option even when the script contains no visible option text.
			return true
		}
		if value == "--" {
			operandsOnly = true
			continue
		}
		if tarCommandOption(value) || index == 0 && tarOldStyleCommandOption(value) {
			return true
		}
	}
	return false
}

func tarCommandOption(value string) bool {
	const minimumLongOption = len("--x")
	if strings.HasPrefix(value, "--") {
		option := value
		if separator := strings.IndexByte(option, '='); separator >= 0 {
			option = option[:separator]
		}
		if len(option) < minimumLongOption {
			return false
		}
		if option == "--checkpoint" || option == "--file" {
			return false
		}
		for _, dangerous := range []string{
			"--checkpoint-action",
			"--files-from",
			"--info-script",
			"--multi-volume",
			"--new-volume-script",
			"--rmt-command",
			"--rsh-command",
			"--to-command",
			"--use-compress-program",
		} {
			// GNU long options accept unique abbreviations. Fail closed for
			// every prefix that could select a command-capable option.
			if strings.HasPrefix(dangerous, option) {
				return true
			}
		}
		return false
	}
	if !strings.HasPrefix(value, "-") {
		return false
	}
	return strings.ContainsAny(strings.TrimPrefix(value, "-"), "FIMT")
}

func tarOldStyleCommandOption(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	for _, character := range value {
		if character < 'A' || character > 'Z' && character < 'a' || character > 'z' {
			return false
		}
	}
	return strings.ContainsAny(value, "FIMT")
}

func sortUsesCommandExecution(command string, args []*syntax.Word) bool {
	if filepath.Base(command) != "sort" {
		return false
	}
	for _, argument := range args {
		value, static := staticShellWord(argument)
		if !static {
			// GNU sort accepts options after operands. An expanded word before
			// "--" can therefore introduce --compress-program at any point.
			return true
		}
		if value == "--" {
			return false
		}
		if sortCommandOption(value) {
			return true
		}
	}
	return false
}

func sortCommandOption(value string) bool {
	if !strings.HasPrefix(value, "--") {
		return false
	}
	option := value
	if separator := strings.IndexByte(option, '='); separator >= 0 {
		option = option[:separator]
	}
	// GNU long options accept unique abbreviations. "--co" is the shortest
	// prefix that distinguishes --compress-program from --check.
	return len(option) >= len("--co") &&
		len(option) <= len("--compress-program") &&
		"--compress-program"[:len(option)] == option
}

func gitUsesCommandExecution(command string, args []*syntax.Word) bool {
	if filepath.Base(command) != "git" {
		return false
	}
	for index := 0; index < len(args); index++ {
		value, static := staticShellWord(args[index])
		if !static {
			// Before the subcommand, an expanded word can introduce -c,
			// --config-env, --exec-path, or an external git-* command.
			return true
		}
		if value == "--version" {
			return false
		}
		if slices.Contains([]string{"-C", "--git-dir", "--work-tree", "--namespace"}, value) {
			index++
			if index >= len(args) {
				return true
			}
			continue
		}
		if slices.Contains([]string{
			"--no-pager", "--no-replace-objects", "--literal-pathspecs",
			"--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs",
			"--no-optional-locks",
		}, value) {
			continue
		}
		if value == "-c" ||
			strings.HasPrefix(value, "-c") ||
			value == "--config-env" ||
			strings.HasPrefix(value, "--config-env=") ||
			value == "--exec-path" ||
			strings.HasPrefix(value, "--exec-path=") {
			return true
		}
		if strings.HasPrefix(value, "-") {
			// Unknown global options are not part of the small positive set
			// attested hooks need.
			return true
		}
		// Unknown names can resolve through aliases or PATH as git-<name>.
		// status is intentionally excluded because repository-local
		// core.fsmonitor configuration can execute mutable commands.
		return value != "rev-parse"
	}
	return true
}

func staticSedPrograms(args []*syntax.Word) ([]string, bool) {
	var programs []string
	operandsOnly := false
	for index := 0; index < len(args); index++ {
		value, static := staticShellWord(args[index])
		if !static {
			return nil, false
		}
		if operandsOnly {
			if len(programs) == 0 {
				programs = append(programs, value)
			}
			continue
		}
		if value == "--" {
			operandsOnly = true
			continue
		}
		program, nextIndex, expression, valid := staticSedExpression(args, index, value)
		if expression {
			if !valid {
				return nil, false
			}
			programs = append(programs, program)
			index = nextIndex
			continue
		}
		if sedProgramFileOption(value) {
			return nil, false
		}
		if safeSedOption(value) {
			continue
		}
		if strings.HasPrefix(value, "-") {
			return nil, false
		}
		if len(programs) == 0 {
			programs = append(programs, value)
		}
	}
	return programs, len(programs) > 0
}

func staticSedExpression(args []*syntax.Word, index int, value string) (string, int, bool, bool) {
	switch {
	case value == "-e" || value == "--expression":
		next := index + 1
		if next >= len(args) {
			return "", index, true, false
		}
		program, static := staticShellWord(args[next])
		return program, next, true, static
	case strings.HasPrefix(value, "--expression="):
		return strings.TrimPrefix(value, "--expression="), index, true, true
	case strings.HasPrefix(value, "-e") && len(value) > 2:
		return value[2:], index, true, true
	default:
		return "", index, false, true
	}
}

func sedProgramFileOption(value string) bool {
	return value == "-f" || value == "--file" ||
		strings.HasPrefix(value, "-f") ||
		strings.HasPrefix(value, "--file=")
}

func safeSedOption(value string) bool {
	switch value {
	case "-n", "-E", "-r", "-u", "-z", "-s",
		"--quiet", "--silent", "--regexp-extended", "--posix",
		"--separate", "--unbuffered", "--null-data":
		return true
	default:
		return false
	}
}

func sedProgramExecutesCommand(program string) bool {
	for index := 0; index < len(program); {
		index = skipSedSeparators(program, index)
		if index >= len(program) {
			return false
		}
		if program[index] == '#' {
			index = skipSedLine(program, index)
			continue
		}
		index = skipSedAddresses(program, index)
		index = skipSedSeparators(program, index)
		if index < len(program) && program[index] == '!' {
			index++
			for index < len(program) && (program[index] == ' ' || program[index] == '\t') {
				index++
			}
		}
		if index >= len(program) {
			return false
		}
		next, executes := skipSedParsedCommand(program, index)
		if executes {
			return true
		}
		index = next
	}
	return false
}

func skipSedParsedCommand(program string, index int) (int, bool) {
	command := program[index]
	index++
	switch command {
	case 'e':
		return index, true
	case 's':
		return skipSedSubstitution(program, index)
	case '{', '}':
		return index, false
	default:
		return skipSedCommand(program, index, command), false
	}
}

func skipSedSeparators(program string, index int) int {
	for index < len(program) {
		switch program[index] {
		case ' ', '\t', '\r', '\n', ';', '{', '}':
			index++
		default:
			return index
		}
	}
	return index
}

func skipSedAddresses(program string, index int) int {
	for range 2 {
		for index < len(program) && (program[index] == ' ' || program[index] == '\t') {
			index++
		}
		next, ok := skipSedAddress(program, index)
		if !ok {
			return index
		}
		index = next
		for index < len(program) && (program[index] == ' ' || program[index] == '\t') {
			index++
		}
		if index >= len(program) || program[index] != ',' {
			return index
		}
		index++
	}
	return index
}

func skipSedAddress(program string, index int) (int, bool) {
	if index >= len(program) {
		return index, false
	}
	switch {
	case program[index] >= '0' && program[index] <= '9':
		for index < len(program) &&
			((program[index] >= '0' && program[index] <= '9') ||
				strings.ContainsRune("~+", rune(program[index]))) {
			index++
		}
		return index, true
	case program[index] == '$':
		return index + 1, true
	case program[index] == '/':
		return skipSedDelimited(program, index+1, '/')
	case program[index] == '\\' && index+1 < len(program):
		delimiter := program[index+1]
		return skipSedDelimited(program, index+2, delimiter)
	default:
		return index, false
	}
}

func skipSedDelimited(program string, index int, delimiter byte) (int, bool) {
	escaped := false
	for index < len(program) {
		switch {
		case escaped:
			escaped = false
		case program[index] == '\\':
			escaped = true
		case program[index] == delimiter:
			return index + 1, true
		}
		index++
	}
	return index, false
}

func skipSedSubstitution(program string, index int) (int, bool) {
	if index >= len(program) || program[index] == '\n' {
		return index, true
	}
	delimiter := program[index]
	index++
	var ok bool
	index, ok = skipSedDelimited(program, index, delimiter)
	if !ok {
		return index, true
	}
	index, ok = skipSedDelimited(program, index, delimiter)
	if !ok {
		return index, true
	}
	for index < len(program) && !strings.ContainsRune(" \t\r\n;", rune(program[index])) {
		if program[index] == 'e' {
			return index, true
		}
		index++
	}
	return index, false
}

func skipSedCommand(program string, index int, command byte) int {
	if command == 'a' || command == 'c' || command == 'i' ||
		command == 'r' || command == 'R' || command == 'w' ||
		command == 'W' {
		return skipSedLine(program, index)
	}
	for index < len(program) && program[index] != ';' && program[index] != '\n' {
		index++
	}
	return index
}

func skipSedLine(program string, index int) int {
	for index < len(program) && program[index] != '\n' {
		index++
	}
	return index
}

func dynamicBuiltinOperand(
	command string,
	commandWord *syntax.Word,
	args []*syntax.Word,
) (*syntax.Word, string) {
	if reason := commandCapableRuntimeReason(command, args); reason != "" {
		return commandWord, reason
	}
	switch command {
	case "mapfile", "readarray":
		return commandWord, "stateful string-evaluating shell builtin"
	case "read", "getopts", "let":
		return commandWord, "stateful variable-writing shell builtin"
	case "declare", "local", "typeset":
		if word := namerefDeclarationOption(args); word != nil {
			return word, "indirect variable-writing shell builtin"
		}
	case "printf":
		for _, word := range args {
			value, static := staticShellWord(word)
			if !static {
				continue
			}
			if value == "-v" || strings.HasPrefix(value, "-v") {
				return word, "variable-writing shell builtin"
			}
		}
	}
	return nil, ""
}

func commandCapableRuntimeReason(command string, args []*syntax.Word) string {
	switch filepath.Base(command) {
	case "chroot", "daemon", "doas", "flock", "ionice", "nsenter",
		"numactl", "parallel", "rlwrap", "runuser", "script", "setpriv",
		"setsid", "stdbuf", "su", "sudo", "taskset", "unshare", "watch":
		return "command-capable process wrapper"
	case "cmake":
		// Even configuration mode evaluates CMakeLists.txt, while -P executes
		// an arbitrary script. Trusted hooks do not need a safe CMake subset.
		return "command-capable CMake runtime"
	}
	if awkUsesCommandExecution(command, args) {
		return "command-capable AWK runtime"
	}
	if sedUsesCommandExecution(command, args) {
		return "command-capable sed runtime"
	}
	if findUsesCommandExecution(command, args) {
		return "command-capable find runtime"
	}
	if makeUsesCommandExecution(command) {
		return "command-capable Make runtime"
	}
	if sshUsesCommandExecution(command) {
		return "command-capable OpenSSH runtime"
	}
	if tarUsesCommandExecution(command, args) {
		return "command-capable tar runtime"
	}
	if sortUsesCommandExecution(command, args) {
		return "command-capable sort runtime"
	}
	if gitUsesCommandExecution(command, args) {
		return "command-capable Git runtime"
	}
	return ""
}

func namerefDeclarationOption(args []*syntax.Word) *syntax.Word {
	for _, word := range args {
		value, static := staticShellWord(word)
		if !static || value == "--" || !strings.HasPrefix(value, "-") {
			return nil
		}
		if value == "--nameref" || strings.Contains(strings.TrimLeft(value, "-"), "n") {
			return word
		}
	}
	return nil
}

func sourcedFileOperand(args []*syntax.Word) (*syntax.Word, string) {
	for _, word := range args {
		value, static := staticShellWord(word)
		if !static {
			return word, "mutable interpreter or sourced-file operand"
		}
		if value == "--" || strings.HasPrefix(value, "-") {
			continue
		}
		if filepath.IsAbs(value) && isSystemRuntimePath(value) {
			return nil, ""
		}
		return word, "mutable interpreter or sourced-file operand"
	}
	return nil, ""
}

func interpreterScriptOperand(command string, args []*syntax.Word) (*syntax.Word, string) {
	operandsOnly := false
	for _, word := range args {
		value, static := staticShellWord(word)
		if !static {
			return word, "mutable interpreter or sourced-file operand"
		}
		if !operandsOnly && value == "--" {
			operandsOnly = true
			continue
		}
		if !operandsOnly && strings.HasPrefix(value, "-") {
			if interpreterLoadsExternalCode(command, value) {
				return word, "interpreter preload option"
			}
			if interpreterInlineCodeOption(command, value) {
				return word, "inline interpreter code"
			}
			continue
		}
		if filepath.IsAbs(value) && isSystemRuntimePath(value) {
			return nil, ""
		}
		return word, "mutable interpreter or sourced-file operand"
	}
	return nil, ""
}

func interpreterLoadsExternalCode(command, value string) bool {
	switch filepath.Base(command) {
	case "perl":
		return perlLoadsExternalCode(value)
	case "ruby":
		return rubyLoadsExternalCode(value)
	case "node":
		return nodeLoadsExternalCode(value)
	default:
		if strings.HasPrefix(filepath.Base(command), "python") {
			return pythonLoadsExternalCode(value)
		}
		return false
	}
}

func pythonLoadsExternalCode(value string) bool {
	// CPython accepts boolean short options before -m in the same token
	// (for example, -Imhelper), and -m consumes the rest as the module name.
	return strings.HasPrefix(value, "-") &&
		!strings.HasPrefix(value, "--") &&
		strings.ContainsRune(value[1:], 'm')
}

func perlLoadsExternalCode(value string) bool {
	if value == "--debugger" || strings.HasPrefix(value, "--debugger=") {
		return true
	}
	// Perl permits short switches to be bundled. -I extends @INC,
	// -M/-m preload modules, and -d may load a debugger module.
	return strings.HasPrefix(value, "-") &&
		!strings.HasPrefix(value, "--") &&
		strings.ContainsAny(value[1:], "IMmd")
}

func rubyLoadsExternalCode(value string) bool {
	if value == "--require" || strings.HasPrefix(value, "--require=") {
		return true
	}
	// Ruby likewise permits bundles such as -wI. and -wrhelper.
	return strings.HasPrefix(value, "-") &&
		!strings.HasPrefix(value, "--") &&
		strings.ContainsAny(value[1:], "Ir")
}

func nodeLoadsExternalCode(value string) bool {
	for _, option := range []string{
		"-r",
		"--require",
		"--import",
		"--loader",
		"--experimental-loader",
	} {
		if value == option || strings.HasPrefix(value, option+"=") {
			return true
		}
	}
	return strings.HasPrefix(value, "-r") && !strings.HasPrefix(value, "--")
}

func interpreterInlineCodeOption(command, value string) bool {
	switch value {
	case "-c", "-e", "--eval", "--print", "-p":
		return true
	default:
		if strings.HasPrefix(value, "--eval=") ||
			strings.HasPrefix(value, "--print=") {
			return true
		}
	}
	if strings.HasPrefix(value, "--") {
		return false
	}
	bundle := strings.TrimPrefix(value, "-")
	switch filepath.Base(command) {
	case "sh", "bash", "dash", "zsh", "ksh":
		return strings.ContainsRune(bundle, 'c')
	case "perl", "ruby":
		return strings.ContainsRune(bundle, 'e')
	case "node":
		return strings.ContainsAny(bundle, "ep")
	default:
		return strings.HasPrefix(filepath.Base(command), "python") &&
			strings.ContainsRune(bundle, 'c')
	}
}

func envSplitStringOption(value string) bool {
	return value == "-S" ||
		value == "--split-string" ||
		strings.HasPrefix(value, "-S") ||
		strings.HasPrefix(value, "--split-string=")
}

func commandResolutionStateBuiltin(command string) bool {
	switch command {
	case "alias", "unalias", "hash", "enable", "nameref":
		return true
	default:
		return false
	}
}

func rejectMutableInputRedirections(script string) error {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).
		Parse(strings.NewReader(script), "trusted-hook")
	if err != nil {
		return fmt.Errorf("parse trusted hook shell: %w", err)
	}
	var rejected *syntax.Word
	syntax.Walk(file, func(node syntax.Node) bool {
		if rejected != nil {
			return false
		}
		redirect, ok := node.(*syntax.Redirect)
		if !ok ||
			redirect.Word == nil ||
			(redirect.Op != syntax.RdrIn && redirect.Op != syntax.RdrInOut) {
			return true
		}
		operand, static := staticShellWord(redirect.Word)
		if static && inputRedirectionPathIsSystemOwned(operand) {
			return true
		}
		rejected = redirect.Word
		return false
	})
	if rejected == nil {
		return nil
	}
	return fmt.Errorf(
		"unsupported mutable input redirection at line %d; trusted hooks must not execute input from mutable paths",
		rejected.Pos().Line(),
	)
}

func inputRedirectionPathIsSystemOwned(path string) bool {
	if path == "/dev/null" {
		return true
	}
	return filepath.IsAbs(path) && isSystemRuntimePath(path)
}

func rejectInterpreterPipelines(script string) error {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).
		Parse(strings.NewReader(script), "trusted-hook")
	if err != nil {
		return fmt.Errorf("parse trusted hook shell: %w", err)
	}
	var rejected *syntax.Word
	syntax.Walk(file, func(node syntax.Node) bool {
		if rejected != nil {
			return false
		}
		pipeline, ok := node.(*syntax.BinaryCmd)
		if !ok || (pipeline.Op != syntax.Pipe && pipeline.Op != syntax.PipeAll) {
			return true
		}
		syntax.Walk(pipeline.Y, func(right syntax.Node) bool {
			if rejected != nil {
				return false
			}
			call, ok := right.(*syntax.CallExpr)
			if !ok {
				return true
			}
			rejected = pipedInterpreterCommand(call.Args, false)
			return rejected == nil
		})
		return rejected == nil
	})
	if rejected == nil {
		return nil
	}
	return fmt.Errorf(
		"unsupported interpreter pipeline at line %d; trusted hooks must not execute piped bytes as runtime code",
		rejected.Pos().Line(),
	)
}

func pipedInterpreterCommand(args []*syntax.Word, env bool) *syntax.Word {
	for index := 0; index < len(args); index++ {
		word := args[index]
		value, static := staticShellWord(word)
		if !static {
			return nil
		}
		if value == "--" {
			continue
		}
		if env && envOptionConsumesNext(value) {
			index++
			continue
		}
		if strings.HasPrefix(value, "-") {
			continue
		}
		if env && strings.Contains(value, "=") {
			continue
		}
		switch filepath.Base(value) {
		case "command", "builtin", "exec", "nohup":
			return pipedShellWrapperCommand(filepath.Base(value), args[index+1:])
		case "env":
			return pipedInterpreterCommand(args[index+1:], true)
		}
		if filepath.Base(value) == "xargs" && xargsRunsCustomCommand(args[index+1:]) {
			return word
		}
		if isRuntimeInterpreter(value) {
			return word
		}
		return nil
	}
	return nil
}

func xargsRunsCustomCommand(args []*syntax.Word) bool {
	for index := 0; index < len(args); index++ {
		value, static := staticShellWord(args[index])
		if !static {
			return true
		}
		if value == "--" {
			return index+1 < len(args)
		}
		if !strings.HasPrefix(value, "-") || value == "-" {
			return true
		}
		if xargsOptionConsumesNext(value) {
			index++
			if index >= len(args) {
				return true
			}
			continue
		}
		if xargsKnownOption(value) {
			continue
		}
		return true
	}
	return false
}

func xargsOptionConsumesNext(value string) bool {
	switch value {
	case "-E", "-I", "-J", "-L", "-n", "-P", "-R", "-S", "-s",
		"--arg-file", "--delimiter", "--max-args", "--max-procs", "--max-chars",
		"--process-slot-var":
		return true
	default:
		return false
	}
}

func xargsKnownOption(value string) bool {
	for _, prefix := range []string{
		"-E", "-I", "-J", "-L", "-n", "-P", "-R", "-S", "-s",
		"--arg-file=", "--delimiter=", "--eof=", "--replace=",
		"--max-lines=", "--max-args=", "--max-procs=", "--max-chars=",
		"--process-slot-var=",
	} {
		if strings.HasPrefix(value, prefix) && value != prefix {
			return true
		}
	}
	switch value {
	case "-0", "-o", "-p", "-r", "-t", "-x",
		"--null", "--open-tty", "--interactive", "--no-run-if-empty",
		"--verbose", "--exit", "--show-limits", "--help", "--version",
		"--eof", "--replace", "--max-lines":
		return true
	default:
		return false
	}
}

func pipedShellWrapperCommand(wrapper string, args []*syntax.Word) *syntax.Word {
	for index := 0; index < len(args); index++ {
		value, static := staticShellWord(args[index])
		if !static {
			return nil
		}
		if value == "--" {
			return pipedInterpreterCommand(args[index+1:], false)
		}
		if !strings.HasPrefix(value, "-") {
			return pipedInterpreterCommand(args[index:], false)
		}
		if wrapper == "command" &&
			strings.ContainsAny(strings.TrimLeft(value, "-"), "vV") {
			return nil
		}
		if wrapper == "exec" && (value == "-a" || value == "--argv0") {
			index++
		}
	}
	return nil
}

func isRuntimeInterpreter(command string) bool {
	base := filepath.Base(command)
	switch base {
	case "sh", "bash", "dash", "zsh", "ksh", "perl", "ruby", "node":
		return true
	}
	if !strings.HasPrefix(base, "python") {
		return false
	}
	for _, char := range strings.TrimPrefix(base, "python") {
		if (char < '0' || char > '9') && char != '.' {
			return false
		}
	}
	return true
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

func isSystemRuntimePath(path string) bool {
	path = filepath.Clean(path)
	return path == "/bin" || strings.HasPrefix(path, "/bin/") ||
		path == "/sbin" || strings.HasPrefix(path, "/sbin/") ||
		path == "/usr/bin" || strings.HasPrefix(path, "/usr/bin/") ||
		path == "/usr/sbin" || strings.HasPrefix(path, "/usr/sbin/") ||
		path == "/usr/local/libexec/dear-agent-codex-hook-json" ||
		path == "/usr/local/libexec/dear-agent-bead-close-guard"
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

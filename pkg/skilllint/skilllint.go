// Package skilllint validates tracked AI skill and command Markdown surfaces.
// It keeps portable skill quality, provider command policy, repository
// discovery, and single-owner skill-name enforcement behind one interface.
package skilllint

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Violation describes a single skill or command content problem.
type Violation struct {
	Path   string
	Reason string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s: %s", v.Path, v.Reason)
}

// Frontmatter is the subset of portable and provider metadata that affects
// validation. Raw keys are retained separately so unsupported extensions can
// be reported without widening this interface.
type Frontmatter struct {
	Name        string `yaml:"name"`
	Model       string `yaml:"model"`
	Effort      string `yaml:"effort"`
	Description string `yaml:"description"`
}

type surfaceKind uint8

const (
	surfaceUnknown surfaceKind = iota
	surfaceCommand
	surfaceSkill
)

type document struct {
	displayPath string
	readPath    string
	kind        surfaceKind
}

type checkedDocument struct {
	document
	data         []byte
	mode         os.FileMode
	resolvedPath string
	skillName    string
}

type skillOwner struct {
	displayPath  string
	resolvedPath string
}

var (
	allowedModels  = map[string]bool{"haiku": true, "sonnet": true, "opus": true}
	allowedEfforts = map[string]bool{"low": true, "medium": true, "high": true}

	skillNamePattern       = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	workflowHeadingPattern = regexp.MustCompile(`(?i)^(?:start|workflow|process|steps?|procedure|route(?: the request)?|how to)\b`)
	orderedStepPattern     = regexp.MustCompile(`(?m)^[ \t]*[0-9]+[.)][ \t]+`)
	verificationPattern    = regexp.MustCompile(`(?i)^(?:verify|verification|completion|complete|finish|close|rewind and close)\b`)
	referencesPattern      = regexp.MustCompile(`(?im)^#{1,6}[ \t]+(?:references|documentation|resources)\b`)
	fallbackPattern        = regexp.MustCompile(`(?i)(?:(?:when|if) [^\n]{0,80}(?:unavailable|unsupported|not available)[^\n]{0,120}\b(?:use|run|invoke|follow|continue|fall back)\b|(?:for|on) (?:non[- ]claude|other harness(?:es)?)[^\n]{0,120}\b(?:use|run|invoke|follow|continue)\b|\b(?:use|run|invoke|follow|continue)\b[^\n]{0,120}\bwithout (?:the |this )?skill\b)`)
	fallbackRoutePattern   = regexp.MustCompile(`(?i)\b(?:cli|command[- ]line|terminal|shell|browser|https?|api|mcp|manual(?:ly)?|direct(?:ly)?|artifacts?|files?)\b`)
	providerToolAction     = regexp.MustCompile(`(?i)\b(?:use|run|invoke|fall back to)\s+(?:the\s+)?(?:bash|read|write|edit|glob|grep|task|webfetch|websearch)\b`)
)

var commandFields = map[string]bool{
	"allowed-tools":         true,
	"argument-hint":         true,
	"content-hash":          true,
	"description":           true,
	"effort":                true,
	"model":                 true,
	"name":                  true,
	"verification_criteria": true,
}

var skillFields = map[string]bool{
	"allowed-tools":            true,
	"argument-hint":            true,
	"compatibility":            true,
	"content-hash":             true,
	"description":              true,
	"disable-model-invocation": true,
	"effort":                   true,
	"license":                  true,
	"metadata":                 true,
	"model":                    true,
	"name":                     true,
	"user-invocable":           true,
	"verification_criteria":    true,
}

var providerExecutionFields = []string{
	"allowed-tools",
	"compatibility",
	"disable-model-invocation",
	"metadata",
	"user-invocable",
}

// CheckFile validates one recognized skill or command Markdown file. Content
// defects are returned as violations; read failures are operational errors.
func CheckFile(path string) ([]Violation, error) {
	kind := classifySurface(path)
	if kind == surfaceUnknown {
		return []Violation{{Path: path, Reason: "not a recognized SKILL.md or commands/*.md surface"}}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return checkData(path, data, kind), nil
}

// CheckDir recursively validates every recognized skill and command surface in
// root. Use CheckRepository for the tracked-file repository gate.
func CheckDir(root string) ([]Violation, error) {
	var documents []document
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		kind := classifySurface(path)
		if kind != surfaceUnknown {
			documents = append(documents, document{displayPath: path, readPath: path, kind: kind})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return checkDocuments(root, documents)
}

// CheckRepository validates every tracked skill and command surface in the Git
// repository containing root. Violation paths are relative to the repository
// top level so local and CI output is stable.
func CheckRepository(ctx context.Context, root string) ([]Violation, error) {
	top, err := gitOutput(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(top))
	if repoRoot == "" {
		return nil, fmt.Errorf("resolve repository root: git returned an empty path")
	}

	tracked, err := gitOutput(ctx, repoRoot, "ls-files", "-z", "--full-name")
	if err != nil {
		return nil, fmt.Errorf("list tracked files: %w", err)
	}
	parts := strings.Split(string(tracked), "\x00")
	documents := make([]document, 0, len(parts))
	for _, path := range parts {
		if path == "" {
			continue
		}
		kind := classifySurface(filepath.FromSlash(path))
		if kind == surfaceUnknown {
			continue
		}
		documents = append(documents, document{
			displayPath: filepath.ToSlash(path),
			readPath:    filepath.Join(repoRoot, filepath.FromSlash(path)),
			kind:        kind,
		})
	}
	return checkDocuments(repoRoot, documents)
}

func gitOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		message := strings.TrimSpace(string(output))
		if message == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, message)
	}
	return output, nil
}

func checkDocuments(containmentRoot string, documents []document) ([]Violation, error) {
	var violations []Violation
	checked := make([]checkedDocument, 0, len(documents))
	for _, doc := range documents {
		info, err := os.Lstat(doc.readPath)
		if err != nil {
			return nil, fmt.Errorf("%s: stat: %w", doc.displayPath, err)
		}
		resolvedPath, inside, containmentErr := resolveInside(containmentRoot, doc.readPath)
		if containmentErr != nil {
			return nil, fmt.Errorf("%s: resolve: %w", doc.displayPath, containmentErr)
		}
		if !inside {
			violations = append(violations, Violation{Path: doc.displayPath, Reason: "tracked skill or command resolves outside the validation root"})
			continue
		}
		data, err := os.ReadFile(doc.readPath)
		if err != nil {
			return nil, fmt.Errorf("%s: read: %w", doc.displayPath, err)
		}
		violations = append(violations, checkData(doc.displayPath, data, doc.kind)...)
		if doc.kind != surfaceSkill {
			continue
		}

		candidate := checkedDocument{
			document:     doc,
			data:         data,
			mode:         info.Mode(),
			resolvedPath: resolvedPath,
		}
		fm, _, _, present, parseErr := extractDocument(data)
		if parseErr == nil && present {
			candidate.skillName = strings.TrimSpace(fm.Name)
		}
		checked = append(checked, candidate)
	}

	firstByDigest := make(map[[sha256.Size]byte]string)
	ownerByName := make(map[string]skillOwner)
	for _, doc := range checked {
		// Symlinks are aliases. Only regular files may own a skill name or
		// participate in content-copy detection.
		if doc.mode&os.ModeSymlink != 0 {
			continue
		}

		digest := semanticSkillDigest(doc.data)
		equivalentCopy := false
		if first, exists := firstByDigest[digest]; exists {
			violations = append(violations, Violation{
				Path:   doc.displayPath,
				Reason: fmt.Sprintf("content-equivalent to %s after frontmatter and whitespace normalization", first),
			})
			equivalentCopy = true
		} else {
			firstByDigest[digest] = doc.displayPath
		}

		if doc.skillName == "" {
			continue
		}
		if owner, exists := ownerByName[doc.skillName]; exists {
			if !equivalentCopy {
				violations = append(violations, Violation{
					Path: doc.displayPath,
					Reason: fmt.Sprintf(
						"skill name %q already has regular-file owner %s; use a contained symlink to that canonical SKILL.md for discovery",
						doc.skillName,
						owner.displayPath,
					),
				})
			}
			continue
		}
		ownerByName[doc.skillName] = skillOwner{
			displayPath:  doc.displayPath,
			resolvedPath: doc.resolvedPath,
		}
	}

	for _, doc := range checked {
		if doc.mode&os.ModeSymlink == 0 || doc.skillName == "" {
			continue
		}
		owner, exists := ownerByName[doc.skillName]
		if !exists {
			violations = append(violations, Violation{
				Path: doc.displayPath,
				Reason: fmt.Sprintf(
					"skill symlink for name %q does not resolve to a tracked regular-file canonical SKILL.md owner",
					doc.skillName,
				),
			})
			continue
		}
		if doc.resolvedPath != owner.resolvedPath {
			violations = append(violations, Violation{
				Path: doc.displayPath,
				Reason: fmt.Sprintf(
					"skill symlink for name %q resolves to a noncanonical target; expected %s",
					doc.skillName,
					owner.displayPath,
				),
			})
		}
	}
	return violations, nil
}

func resolveInside(root, path string) (string, bool, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false, err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false, err
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return "", false, err
	}
	resolvedPath, err = filepath.Abs(resolvedPath)
	if err != nil {
		return "", false, err
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return "", false, err
	}
	inside := relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	return filepath.Clean(resolvedPath), inside, nil
}

func semanticSkillDigest(data []byte) [sha256.Size]byte {
	canonical := strings.ReplaceAll(string(data), "\r\n", "\n")
	_, _, body, present, err := extractDocument([]byte(canonical))
	if err == nil && present {
		closing := strings.Index(canonical[4:], "\n---\n")
		if closing >= 0 {
			var frontmatter map[string]any
			if yaml.Unmarshal([]byte(canonical[4:4+closing]), &frontmatter) == nil {
				// Some producers use content-hash as separately validated metadata.
				// This package does not attest that value, and it is not part of the
				// skill's semantic identity for duplicate detection.
				delete(frontmatter, "content-hash")
				if encoded, marshalErr := json.Marshal(frontmatter); marshalErr == nil {
					canonical = string(encoded) + "\n" + normalizeSkillBody(string(body))
				}
			}
		}
	}
	return sha256.Sum256([]byte(canonical))
}

var markdownComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func normalizeSkillBody(body string) string {
	body = markdownComment.ReplaceAllString(body, "")
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func checkData(path string, data []byte, kind surfaceKind) []Violation {
	fm, keys, body, present, err := extractDocument(data)
	if err != nil {
		return []Violation{{Path: path, Reason: err.Error()}}
	}
	if !present {
		return []Violation{{Path: path, Reason: "no YAML frontmatter (expected --- fenced block at top of file)"}}
	}
	if kind == surfaceSkill {
		return validateSkill(path, fm, keys, body, data)
	}
	return validateCommand(path, fm, keys)
}

func validateCommand(path string, fm *Frontmatter, keys map[string]yaml.Node) []Violation {
	violations := unsupportedFields(path, "command", keys, commandFields)
	violations = append(violations, validateVerificationCriteria(path, keys)...)
	violations = append(violations, validateRequiredTier(path, "model", fm.Model, allowedModels, "haiku, sonnet, opus")...)
	violations = append(violations, validateRequiredTier(path, "effort", fm.Effort, allowedEfforts, "low, medium, high")...)
	if strings.TrimSpace(fm.Description) == "" {
		violations = append(violations, Violation{Path: path, Reason: "missing nonempty `description:` in frontmatter"})
	}
	if !validAllowedTools(keys) {
		violations = append(violations, Violation{Path: path, Reason: "missing `allowed-tools:` or invalid value (expected a string or string list)"})
	}
	return violations
}

func validateSkill(path string, fm *Frontmatter, keys map[string]yaml.Node, body, data []byte) []Violation {
	violations := unsupportedFields(path, "skill", keys, skillFields)
	violations = append(violations, validateVerificationCriteria(path, keys)...)
	violations = append(violations, validateSkillIdentity(path, fm)...)

	bodyText := string(body)
	violations = append(violations, validateSkillBody(path, bodyText)...)
	violations = append(violations, validateSkillLength(path, bodyText, data)...)
	violations = append(violations, validateSkillExecution(path, fm, keys, bodyText)...)
	return violations
}

func validateSkillIdentity(path string, fm *Frontmatter) []Violation {
	var violations []Violation
	name := strings.TrimSpace(fm.Name)
	if name == "" {
		violations = append(violations, Violation{Path: path, Reason: "missing `name:` in frontmatter"})
	} else if !skillNamePattern.MatchString(name) || len(name) > 64 {
		violations = append(violations, Violation{Path: path, Reason: fmt.Sprintf("name=%q is not kebab-case with at most 64 characters", fm.Name)})
	}

	if strings.TrimSpace(fm.Description) == "" {
		violations = append(violations, Violation{Path: path, Reason: "missing nonempty `description:` in frontmatter"})
	}
	return violations
}

func validateSkillBody(path, bodyText string) []Violation {
	var violations []Violation
	structuralText := markdownComment.ReplaceAllString(markdownOutsideFences(bodyText), "")
	if !sectionHasContent(structuralText, workflowHeadingPattern) && len(orderedStepPattern.FindAllStringIndex(structuralText, 2)) < 2 {
		violations = append(violations, Violation{Path: path, Reason: "missing procedural workflow (expected a nonempty workflow section or at least two ordered steps)"})
	}
	if !sectionHasContent(structuralText, verificationPattern) {
		violations = append(violations, Violation{Path: path, Reason: "missing nonempty verification or completion section"})
	}
	return violations
}

func sectionHasContent(markdown string, headingPattern *regexp.Regexp) bool {
	lines := strings.Split(markdown, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		level := headingLevel(trimmed)
		if level == 0 || !headingPattern.MatchString(strings.TrimSpace(trimmed[level:])) {
			continue
		}
		for _, candidate := range lines[index+1:] {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			candidateLevel := headingLevel(candidate)
			if candidateLevel > 0 && candidateLevel <= level {
				break
			}
			if candidateLevel == 0 {
				return true
			}
		}
	}
	return false
}

func headingLevel(line string) int {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level == len(line) || (line[level] != ' ' && line[level] != '\t') {
		return 0
	}
	return level
}

func markdownOutsideFences(body string) string {
	var visible strings.Builder
	var fence string
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		marker := ""
		if strings.HasPrefix(trimmed, "```") {
			marker = "```"
		} else if strings.HasPrefix(trimmed, "~~~") {
			marker = "~~~"
		}
		if marker != "" {
			switch fence {
			case "":
				fence = marker
			case marker:
				fence = ""
			}
			continue
		}
		if fence == "" {
			visible.WriteString(line)
			visible.WriteByte('\n')
		}
	}
	return visible.String()
}

func validateSkillLength(path, bodyText string, data []byte) []Violation {
	var violations []Violation
	lineCount := strings.Count(strings.TrimSuffix(string(data), "\n"), "\n") + 1
	if lineCount > 100 && !referencesPattern.MatchString(markdownOutsideFences(bodyText)) {
		violations = append(violations, Violation{Path: path, Reason: "skill is over 100 lines without a References, Documentation, or Resources section"})
	}
	if lineCount > 500 {
		violations = append(violations, Violation{Path: path, Reason: fmt.Sprintf("skill has %d lines and exceeds the 500-line review threshold", lineCount)})
	}
	return violations
}

func validateSkillExecution(path string, fm *Frontmatter, keys map[string]yaml.Node, bodyText string) []Violation {
	var violations []Violation
	_, modelDeclared := keys["model"]
	_, effortDeclared := keys["effort"]
	if modelDeclared != effortDeclared {
		violations = append(violations, Violation{Path: path, Reason: "optional `model:` and `effort:` must be declared together"})
	}
	if modelDeclared {
		violations = append(violations, validateOptionalTier(path, "model", fm.Model, allowedModels, "haiku, sonnet, opus")...)
	}
	if effortDeclared {
		violations = append(violations, validateOptionalTier(path, "effort", fm.Effort, allowedEfforts, "low, medium, high")...)
	}
	if _, declared := keys["allowed-tools"]; declared && !validAllowedTools(keys) {
		violations = append(violations, Violation{Path: path, Reason: "invalid `allowed-tools:` value (expected a nonempty string or nonempty string list)"})
	}
	if hasProviderExecutionField(keys) && !hasActionableNonProviderFallback(bodyText) {
		violations = append(violations, Violation{Path: path, Reason: "provider execution extension requires a non-provider fallback in the skill body"})
	}
	return violations
}

func hasActionableNonProviderFallback(body string) bool {
	visible := markdownOutsideFences(body)
	for paragraph := range strings.SplitSeq(visible, "\n\n") {
		location := fallbackPattern.FindStringIndex(paragraph)
		if location == nil {
			continue
		}
		fallbackAction := paragraph[location[0]:]
		if fallbackRoutePattern.MatchString(fallbackAction) &&
			!providerToolAction.MatchString(fallbackAction) {
			return true
		}
	}
	return false
}

func validAllowedTools(keys map[string]yaml.Node) bool {
	value, ok := keys["allowed-tools"]
	if !ok {
		return false
	}
	switch value.Kind {
	case yaml.ScalarNode:
		return value.Tag == "!!str" && strings.TrimSpace(value.Value) != ""
	case yaml.SequenceNode:
		if len(value.Content) == 0 {
			return false
		}
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || strings.TrimSpace(item.Value) == "" {
				return false
			}
		}
		return true
	case yaml.DocumentNode, yaml.MappingNode, yaml.AliasNode:
		return false
	default:
		return false
	}
}

func validateRequiredTier(path, field, value string, allowed map[string]bool, expected string) []Violation {
	if strings.TrimSpace(value) == "" {
		return []Violation{{Path: path, Reason: fmt.Sprintf("missing `%s:` in frontmatter (expected one of %s)", field, expected)}}
	}
	return validateOptionalTier(path, field, value, allowed, expected)
}

func validateOptionalTier(path, field, value string, allowed map[string]bool, expected string) []Violation {
	if allowed[strings.ToLower(strings.TrimSpace(value))] {
		return nil
	}
	return []Violation{{Path: path, Reason: fmt.Sprintf("%s=%q not allowed (expected one of %s)", field, value, expected)}}
}

func unsupportedFields(path, surface string, keys map[string]yaml.Node, allowed map[string]bool) []Violation {
	unknown := make([]string, 0)
	for key := range keys {
		if !allowed[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	violations := make([]Violation, 0, len(unknown))
	for _, key := range unknown {
		violations = append(violations, Violation{Path: path, Reason: fmt.Sprintf("unsupported %s frontmatter field `%s`", surface, key)})
	}
	return violations
}

func validateVerificationCriteria(path string, keys map[string]yaml.Node) []Violation {
	node, exists := keys["verification_criteria"]
	if !exists {
		return nil
	}
	if node.Kind != yaml.SequenceNode || len(node.Content) == 0 {
		return []Violation{{Path: path, Reason: "`verification_criteria:` must be a nonempty list of strings"}}
	}
	for i, item := range node.Content {
		if item.Kind != yaml.ScalarNode || item.Tag != "!!str" || strings.TrimSpace(item.Value) == "" {
			return []Violation{{Path: path, Reason: fmt.Sprintf("verification_criteria[%d] must be a nonempty string", i)}}
		}
	}
	return nil
}

func hasProviderExecutionField(keys map[string]yaml.Node) bool {
	for _, field := range providerExecutionFields {
		if _, present := keys[field]; present {
			return true
		}
	}
	return false
}

func classifySurface(path string) surfaceKind {
	base := filepath.Base(path)
	if base == "SKILL.md" {
		return surfaceSkill
	}
	if filepath.Ext(base) != ".md" || filepath.Base(filepath.Dir(path)) != "commands" {
		return surfaceUnknown
	}
	if base == "README.md" || base == "SPEC.md" || strings.HasSuffix(base, "-README.md") {
		return surfaceUnknown
	}
	return surfaceCommand
}

// extractFrontmatter preserves the package's original parser interface for
// callers and tests that only need typed fields and body content.
func extractFrontmatter(data []byte) (*Frontmatter, []byte, error) {
	fm, _, body, present, err := extractDocument(data)
	if err != nil {
		return nil, nil, err
	}
	if !present {
		return nil, nil, nil
	}
	return fm, body, nil
}

func extractDocument(data []byte) (*Frontmatter, map[string]yaml.Node, []byte, bool, error) {
	text := string(data)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return nil, nil, nil, false, nil
	}
	closing := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSuffix(lines[i], "\r") == "---" {
			closing = i
			break
		}
	}
	if closing < 0 {
		return nil, nil, nil, true, fmt.Errorf("frontmatter block not terminated with ---")
	}
	block := strings.Join(lines[1:closing], "\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return nil, nil, nil, true, fmt.Errorf("frontmatter yaml: %w", err)
	}
	keys := make(map[string]yaml.Node)
	if err := yaml.Unmarshal([]byte(block), &keys); err != nil {
		return nil, nil, nil, true, fmt.Errorf("frontmatter yaml: %w", err)
	}
	body := strings.Join(lines[closing+1:], "\n")
	return &fm, keys, []byte(body), true, nil
}

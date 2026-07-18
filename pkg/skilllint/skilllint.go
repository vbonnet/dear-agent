// Package skilllint validates tracked AI skill and command Markdown surfaces.
// It keeps portable skill quality, provider command policy, repository
// discovery, and exact-duplicate detection behind one interface.
package skilllint

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

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

var (
	allowedModels  = map[string]bool{"haiku": true, "sonnet": true, "opus": true}
	allowedEfforts = map[string]bool{"low": true, "medium": true, "high": true}

	skillNamePattern       = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	triggerPattern         = regexp.MustCompile(`(?i)\b(?:use|trigger)\s+when\b`)
	workflowHeadingPattern = regexp.MustCompile(`(?im)^#{1,6}[ \t]+(?:start|workflow|process|steps?|procedure|route(?: the request)?|how to|use)\b`)
	orderedStepPattern     = regexp.MustCompile(`(?m)^[ \t]*[0-9]+[.)][ \t]+`)
	verificationPattern    = regexp.MustCompile(`(?im)^#{1,6}[ \t]+(?:verify|verification|completion|complete|finish|close|end|rewind and close)\b`)
	referencesPattern      = regexp.MustCompile(`(?im)^#{1,6}[ \t]+(?:references|documentation|resources)\b`)
	fallbackPattern        = regexp.MustCompile(`(?i)(?:non[- ]claude|other harness(?:es)?|skill activation is unavailable|without (?:the |this )?skill|shell access|when [^\n]{0,40} unavailable)`)
)

var commandFields = map[string]bool{
	"allowed-tools": true,
	"argument-hint": true,
	"content-hash":  true,
	"description":   true,
	"effort":        true,
	"model":         true,
	"name":          true,
}

var skillFields = map[string]bool{
	"allowed-tools":            true,
	"argument-hint":            true,
	"compatibility":            true,
	"description":              true,
	"disable-model-invocation": true,
	"effort":                   true,
	"license":                  true,
	"metadata":                 true,
	"model":                    true,
	"name":                     true,
	"user-invocable":           true,
}

var providerExecutionFields = []string{
	"allowed-tools",
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
	return checkDocuments(documents)
}

// CheckRepository validates every tracked skill and command surface in the Git
// repository containing root. Violation paths are relative to the repository
// top level so local and CI output is stable.
func CheckRepository(root string) ([]Violation, error) {
	top, err := gitOutput(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(top))
	if repoRoot == "" {
		return nil, fmt.Errorf("resolve repository root: git returned an empty path")
	}

	tracked, err := gitOutput(repoRoot, "ls-files", "-z", "--full-name")
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
	return checkDocuments(documents)
}

func gitOutput(dir string, args ...string) ([]byte, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, message)
	}
	return output, nil
}

func checkDocuments(documents []document) ([]Violation, error) {
	var violations []Violation
	firstByDigest := make(map[[sha256.Size]byte]string)
	for _, doc := range documents {
		data, err := os.ReadFile(doc.readPath)
		if err != nil {
			return nil, fmt.Errorf("%s: read: %w", doc.displayPath, err)
		}
		violations = append(violations, checkData(doc.displayPath, data, doc.kind)...)
		if doc.kind != surfaceSkill {
			continue
		}
		digest := sha256.Sum256(data)
		if first, exists := firstByDigest[digest]; exists {
			violations = append(violations, Violation{
				Path:   doc.displayPath,
				Reason: fmt.Sprintf("byte-identical to %s", first),
			})
			continue
		}
		firstByDigest[digest] = doc.displayPath
	}
	return violations, nil
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
	violations = append(violations, validateRequiredTier(path, "model", fm.Model, allowedModels, "haiku, sonnet, opus")...)
	violations = append(violations, validateRequiredTier(path, "effort", fm.Effort, allowedEfforts, "low, medium, high")...)
	if strings.TrimSpace(fm.Description) == "" {
		violations = append(violations, Violation{Path: path, Reason: "missing nonempty `description:` in frontmatter"})
	}
	if !nonemptyField(keys, "allowed-tools") {
		violations = append(violations, Violation{Path: path, Reason: "missing `allowed-tools:` in frontmatter"})
	}
	return violations
}

func validateSkill(path string, fm *Frontmatter, keys map[string]yaml.Node, body, data []byte) []Violation {
	violations := unsupportedFields(path, "skill", keys, skillFields)
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

	description := strings.TrimSpace(fm.Description)
	if description == "" {
		violations = append(violations, Violation{Path: path, Reason: "missing nonempty `description:` in frontmatter"})
	} else if !triggerPattern.MatchString(description) {
		violations = append(violations, Violation{Path: path, Reason: "description has no activation trigger (expected `Use when` or `Trigger when`)"})
	}
	return violations
}

func validateSkillBody(path, bodyText string) []Violation {
	var violations []Violation
	if !workflowHeadingPattern.MatchString(bodyText) && len(orderedStepPattern.FindAllStringIndex(bodyText, 2)) < 2 {
		violations = append(violations, Violation{Path: path, Reason: "missing procedural workflow (expected a workflow heading or at least two ordered steps)"})
	}
	if !verificationPattern.MatchString(bodyText) {
		violations = append(violations, Violation{Path: path, Reason: "missing verification or completion heading"})
	}
	return violations
}

func validateSkillLength(path, bodyText string, data []byte) []Violation {
	var violations []Violation
	lineCount := 1 + strings.Count(string(data), "\n")
	if lineCount > 100 && !referencesPattern.MatchString(bodyText) {
		violations = append(violations, Violation{Path: path, Reason: "skill is over 100 lines without a References, Documentation, or Resources section"})
	}
	if lineCount > 500 {
		violations = append(violations, Violation{Path: path, Reason: fmt.Sprintf("skill has %d lines and exceeds the 500-line review threshold", lineCount)})
	}
	return violations
}

func validateSkillExecution(path string, fm *Frontmatter, keys map[string]yaml.Node, bodyText string) []Violation {
	var violations []Violation
	modelPresent := nonemptyField(keys, "model")
	effortPresent := nonemptyField(keys, "effort")
	if modelPresent != effortPresent {
		violations = append(violations, Violation{Path: path, Reason: "optional `model:` and `effort:` must be declared together"})
	}
	if modelPresent {
		violations = append(violations, validateOptionalTier(path, "model", fm.Model, allowedModels, "haiku, sonnet, opus")...)
	}
	if effortPresent {
		violations = append(violations, validateOptionalTier(path, "effort", fm.Effort, allowedEfforts, "low, medium, high")...)
	}
	if hasProviderExecutionField(keys) && !fallbackPattern.MatchString(bodyText) {
		violations = append(violations, Violation{Path: path, Reason: "provider execution extension requires a non-provider fallback in the skill body"})
	}
	return violations
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

func nonemptyField(keys map[string]yaml.Node, key string) bool {
	node, ok := keys[key]
	if !ok || node.Tag == "!!null" {
		return false
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return strings.TrimSpace(node.Value) != ""
	case yaml.SequenceNode, yaml.MappingNode:
		return len(node.Content) > 0
	case yaml.DocumentNode, yaml.AliasNode, 0:
		return false
	}
	return false
}

func hasProviderExecutionField(keys map[string]yaml.Node) bool {
	for _, field := range providerExecutionFields {
		if nonemptyField(keys, field) {
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

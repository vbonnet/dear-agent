// Package skilllint validates that Claude Code skill markdown files
// (commands/*.md, skills/*/SKILL.md) declare model and effort tiers in their
// YAML frontmatter.
//
// Why: unpinned skills default to whatever model the parent session uses,
// which in practice means Opus-by-default on claude-code sessions. Pinning
// every skill to the cheapest tier that still works prevents silent cost
// drift when the default changes, and makes cost audits tractable.
//
// Allowed tiers (see docs/skill-tiers.md):
//
//	model:   haiku | sonnet | opus
//	effort:  low    | medium | high
//
// Opus is allowed for declarative purposes but should be rare in practice.
package skilllint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Violation describes a single frontmatter problem.
type Violation struct {
	Path   string
	Reason string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s: %s", v.Path, v.Reason)
}

// Frontmatter is the subset of skill frontmatter we validate.
// yaml tags match Claude Code's skill/command metadata format.
type Frontmatter struct {
	Model       string `yaml:"model"`
	Effort      string `yaml:"effort"`
	Description string `yaml:"description"`
}

var (
	allowedModels  = map[string]bool{"haiku": true, "sonnet": true, "opus": true}
	allowedEfforts = map[string]bool{"low": true, "medium": true, "high": true}
)

// CheckFile validates a single skill file. Returns a slice of violations
// (empty if the file is compliant) and an error only on I/O / parse failure
// that prevents analysis.
func CheckFile(path string) ([]Violation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	fm, rest, err := extractFrontmatter(data)
	if err != nil {
		return []Violation{{Path: path, Reason: err.Error()}}, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}
	if rest == nil {
		return []Violation{{Path: path, Reason: "no YAML frontmatter (expected --- fenced block at top of file)"}}, nil
	}
	return validate(path, fm), nil
}

// CheckDir walks a directory recursively and checks every skill-style
// markdown file. Files considered: *.md directly under a "commands" dir, and
// SKILL.md anywhere under a "skills" tree.
func CheckDir(root string) ([]Violation, error) {
	var out []Violation
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !isSkillFile(path) {
			return nil
		}
		vs, cerr := CheckFile(path)
		if cerr != nil {
			return fmt.Errorf("%s: %w", path, cerr)
		}
		out = append(out, vs...)
		return nil
	})
	return out, err
}

// isSkillFile reports whether path is a skill-style markdown file we should
// lint. Rules:
//   - Must be *.md.
//   - README.md and *-README.md (usage guides that sit alongside skills) are excluded.
//   - Test files (_test.sh, _test.go) are excluded.
//   - Files under a `commands/` directory are skills.
//   - SKILL.md inside a `skills/<name>/` directory is a skill.
func isSkillFile(path string) bool {
	base := filepath.Base(path)
	if strings.HasSuffix(base, "_test.sh") || strings.HasSuffix(base, "_test.go") {
		return false
	}
	if !strings.HasSuffix(base, ".md") {
		return false
	}
	// Skip README-style docs that share the commands/ directory.
	if base == "README.md" || strings.HasSuffix(base, "-README.md") {
		return false
	}
	parent := filepath.Base(filepath.Dir(path))
	if parent == "commands" {
		return true
	}
	// SKILL.md inside a skills/<name>/ dir (grandparent == "skills").
	if base == "SKILL.md" {
		grandparent := filepath.Base(filepath.Dir(filepath.Dir(path)))
		if grandparent == "skills" {
			return true
		}
	}
	return false
}

// extractFrontmatter pulls a leading `---\n...\n---\n` block out of a skill
// markdown file. Returns parsed frontmatter, the remainder of the file, and
// an error if the block exists but fails to parse.
//
// Returns (nil, nil, nil) if no frontmatter block is present — the caller
// treats that as its own violation with a user-friendly message.
func extractFrontmatter(data []byte) (*Frontmatter, []byte, error) {
	const sep = "---"
	text := string(data)
	if !strings.HasPrefix(text, sep) {
		return nil, nil, nil
	}
	// Find the closing separator after the first line.
	rest := text[len(sep):]
	// Skip leading newlines after the opener.
	rest = strings.TrimLeft(rest, "\r\n")
	end := strings.Index(rest, "\n"+sep)
	if end < 0 {
		return nil, nil, fmt.Errorf("frontmatter block not terminated with ---")
	}
	block := rest[:end]
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return nil, nil, fmt.Errorf("frontmatter yaml: %w", err)
	}
	body := rest[end+len("\n"+sep):]
	return &fm, []byte(body), nil
}

func validate(path string, fm *Frontmatter) []Violation {
	var vs []Violation
	if fm.Model == "" {
		vs = append(vs, Violation{
			Path:   path,
			Reason: "missing `model:` in frontmatter (expected one of haiku, sonnet, opus)",
		})
	} else if !allowedModels[strings.ToLower(fm.Model)] {
		vs = append(vs, Violation{
			Path:   path,
			Reason: fmt.Sprintf("model=%q not allowed (expected one of haiku, sonnet, opus)", fm.Model),
		})
	}
	if fm.Effort == "" {
		vs = append(vs, Violation{
			Path:   path,
			Reason: "missing `effort:` in frontmatter (expected one of low, medium, high)",
		})
	} else if !allowedEfforts[strings.ToLower(fm.Effort)] {
		vs = append(vs, Violation{
			Path:   path,
			Reason: fmt.Sprintf("effort=%q not allowed (expected one of low, medium, high)", fm.Effort),
		})
	}
	return vs
}

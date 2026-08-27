package specpackage

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

const authenticatedSpecauditToken = `"<distribution-root>/bin/specaudit"`
const maxMarkdownLinks = 4096

var forbiddenInstalledReferences = []string{
	"go run ./tools/specaudit",
	"make lint-specs",
	"cmd/ears-lint",
	"internal/speccoverage",
}

func validateMarkdownClosure(files map[string][]byte) error {
	for _, entry := range payloadLayout {
		if entry.role != "skill" && entry.role != "reference" {
			continue
		}
		content, ok := files[entry.packagePath]
		if !ok {
			return fmt.Errorf("missing Markdown payload %q", entry.packagePath)
		}
		if !utf8.Valid(content) {
			return fmt.Errorf("markdown payload %q is not valid UTF-8", entry.packagePath)
		}
		for _, forbidden := range forbiddenInstalledReferences {
			if bytes.Contains(content, []byte(forbidden)) {
				return fmt.Errorf("markdown payload %q retains checkout-only reference %q", entry.packagePath, forbidden)
			}
		}
		if entry.skillName != "" {
			if err := validateSkillName(entry.packagePath, content, entry.skillName); err != nil {
				return err
			}
		}
		if err := validateMarkdownLinks(entry.packagePath, content); err != nil {
			return err
		}
	}
	for _, packagePath := range []string{
		"skills/audit-specs/SKILL.md",
		"skills/audit-specs/references/report-schema.md",
	} {
		if !bytes.Contains(files[packagePath], []byte(authenticatedSpecauditToken)) {
			return fmt.Errorf("markdown payload %q does not name the authenticated package executable %s", packagePath, authenticatedSpecauditToken)
		}
	}
	return nil
}

func validateSkillName(packagePath string, content []byte, expected string) error {
	const opening = "---\n"
	if !bytes.HasPrefix(content, []byte(opening)) {
		return fmt.Errorf("skill %q is missing YAML frontmatter", packagePath)
	}
	remainder := content[len(opening):]
	frontmatter, _, found := bytes.Cut(remainder, []byte("\n---\n"))
	if !found {
		return fmt.Errorf("skill %q has unterminated YAML frontmatter", packagePath)
	}
	var metadata struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(frontmatter, &metadata); err != nil {
		return fmt.Errorf("parse skill %q frontmatter: %w", packagePath, err)
	}
	if metadata.Name != expected {
		return fmt.Errorf("skill %q has name %q, want %q", packagePath, metadata.Name, expected)
	}
	return nil
}

func validateMarkdownLinks(packagePath string, content []byte) error {
	document := goldmark.DefaultParser().Parse(text.NewReader(content))
	linkCount := 0
	return ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var destination []byte
		switch typed := node.(type) {
		case *ast.Link:
			destination = typed.Destination
		case *ast.Image:
			destination = typed.Destination
		case *ast.AutoLink:
			destination = typed.URL(content)
			if typed.AutoLinkType == ast.AutoLinkEmail &&
				!bytes.HasPrefix(bytes.ToLower(destination), []byte("mailto:")) {
				destination = append([]byte("mailto:"), destination...)
			}
		case *ast.RawHTML, *ast.HTMLBlock:
			return ast.WalkStop, fmt.Errorf("markdown payload %q contains unsupported raw HTML", packagePath)
		default:
			return ast.WalkContinue, nil
		}
		linkCount++
		if linkCount > maxMarkdownLinks {
			return ast.WalkStop, fmt.Errorf("markdown payload %q exceeds the %d-link bound", packagePath, maxMarkdownLinks)
		}
		if err := validateMarkdownDestination(packagePath, string(destination)); err != nil {
			return ast.WalkStop, err
		}
		return ast.WalkContinue, nil
	})
}

func validateMarkdownDestination(sourcePath, raw string) error {
	if err := validateRawMarkdownDestination(sourcePath, raw); err != nil {
		return err
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("markdown payload %q contains malformed link target %q: %w", sourcePath, raw, err)
	}
	if parsed.Scheme != "" {
		return validateExternalMarkdownDestination(sourcePath, raw, parsed)
	}
	return validateLocalMarkdownDestination(sourcePath, raw, parsed)
}

func validateRawMarkdownDestination(sourcePath, raw string) error {
	if raw == "" || !utf8.ValidString(raw) || strings.ContainsRune(raw, '\x00') {
		return fmt.Errorf("markdown payload %q contains an empty or invalid link target", sourcePath)
	}
	if strings.Contains(raw, `\`) || strings.HasPrefix(raw, "//") {
		return fmt.Errorf("markdown payload %q contains forbidden link target %q", sourcePath, raw)
	}
	return nil
}

func validateExternalMarkdownDestination(sourcePath, raw string, parsed *url.URL) error {
	switch strings.ToLower(parsed.Scheme) {
	case "https", "http", "mailto":
		return nil
	default:
		return fmt.Errorf("markdown payload %q contains forbidden link scheme in %q", sourcePath, raw)
	}
}

func validateLocalMarkdownDestination(sourcePath, raw string, parsed *url.URL) error {
	if parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" {
		return fmt.Errorf("markdown payload %q contains nonlocal link target %q", sourcePath, raw)
	}
	if parsed.Path == "" {
		if parsed.Fragment != "" && parsed.RawQuery == "" {
			return nil
		}
		return fmt.Errorf("markdown payload %q contains unresolved link target %q", sourcePath, raw)
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("markdown payload %q contains query-bearing local link %q", sourcePath, raw)
	}
	decoded, err := decodeLocalMarkdownPath(sourcePath, raw, parsed)
	if err != nil {
		return err
	}
	return resolveLocalMarkdownPath(sourcePath, raw, decoded)
}

func decodeLocalMarkdownPath(sourcePath, raw string, parsed *url.URL) (string, error) {
	decoded, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return "", fmt.Errorf("markdown payload %q contains malformed encoded path %q: %w", sourcePath, raw, err)
	}
	if decoded == "" || !utf8.ValidString(decoded) || strings.ContainsAny(decoded, "\\\x00") || strings.Contains(decoded, "%") {
		return "", fmt.Errorf("markdown payload %q contains invalid encoded path %q", sourcePath, raw)
	}
	if path.IsAbs(decoded) || decoded != path.Clean(decoded) {
		return "", fmt.Errorf("markdown payload %q contains noncanonical local path %q", sourcePath, raw)
	}
	return decoded, nil
}

func resolveLocalMarkdownPath(sourcePath, raw, decoded string) error {
	resolved := path.Clean(path.Join(path.Dir(sourcePath), decoded))
	if resolved == ".." || strings.HasPrefix(resolved, "../") || !strings.HasPrefix(resolved, "skills/") {
		return fmt.Errorf("markdown payload %q contains escaping local path %q", sourcePath, raw)
	}
	if err := validatePackagePath(resolved); err != nil {
		return fmt.Errorf("markdown payload %q contains invalid local path %q: %w", sourcePath, raw, err)
	}
	entry, ok := entryByPackagePath(resolved)
	if !ok || (entry.role != "skill" && entry.role != "reference") {
		return fmt.Errorf("markdown payload %q contains unresolved package reference %q", sourcePath, raw)
	}
	return nil
}

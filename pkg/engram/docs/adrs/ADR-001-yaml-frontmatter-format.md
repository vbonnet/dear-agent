# ADR-001: YAML Frontmatter Format

**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)
**Deciders**: Engram Core Team
**Context**: Initial design of engram file format

---

## Context

Engrams need to store both structured metadata (type, title, tags, etc.) and unstructured content (markdown documentation). The file format must be:

1. **Human-readable and editable**: Users should be able to create and edit engrams in any text editor
2. **Machine-parseable**: Parser should reliably extract metadata and content
3. **Version-control friendly**: Diffs should be meaningful, merges should be straightforward
4. **Extensible**: Easy to add new metadata fields without breaking existing files

Common options for structured data in text files:
- JSON frontmatter (used by some static site generators)
- TOML frontmatter (used by Hugo)
- YAML frontmatter (used by Jekyll, GitHub Pages, many static site generators)
- Custom format (e.g., key: value pairs)

---

## Decision

We will use **YAML frontmatter delimited by `---` lines** at the start of .ai.md files.

**Format**:
```markdown
---
type: pattern
title: Error Handling in Go
description: Idiomatic error handling patterns
tags: [languages/go, patterns/errors]
agents: [claude-code, cursor]
---
# Error Handling

Prefer explicit error returns over exceptions...
```

**Key Elements**:

1. **Opening delimiter**: `---\n` (exactly 4 bytes)
2. **Frontmatter content**: Valid YAML (indentation-based, key: value pairs)
3. **Closing delimiter**: `\n---\n` (exactly 5 bytes)
4. **Body content**: Markdown text after closing delimiter

**Parsing Rules**:
- File must start with `---\n` (no content before frontmatter)
- First occurrence of `\n---\n` closes frontmatter (subsequent `---` are content)
- Frontmatter is parsed as YAML
- Content is everything after closing delimiter (unprocessed string)

---

## Consequences

### Positive

**Human Readability**:
- YAML is more readable than JSON (no quotes on keys, no commas, supports comments)
- Familiar to users of static site generators (Jekyll, Hugo, Gatsby, etc.)
- Clear visual separation between metadata (`---` lines) and content

**Machine Parseability**:
- Well-defined delimiter format (exact byte sequences)
- Standard YAML parsing libraries available in all languages
- Unambiguous split between frontmatter and content

**Extensibility**:
- Adding new fields is trivial (just add `new_field: value`)
- Optional fields supported via `omitempty` in Go struct tags
- Backward compatible (missing fields can have defaults)

**Version Control**:
- YAML diffs are readable (unlike JSON's compact format)
- Line-based format makes merging easier
- Comments supported (e.g., `# TODO: Update this tag`)

**Ecosystem Compatibility**:
- Many tools understand YAML frontmatter (syntax highlighters, linters, preview renderers)
- GitHub renders .md files with frontmatter correctly
- VSCode extensions exist for YAML frontmatter editing

### Negative

**YAML Complexity**:
- YAML spec is large and has edge cases (tabs vs spaces, multi-line strings, anchors/aliases)
- Indentation-sensitive (can be error-prone for beginners)
- Multiple ways to represent same data (e.g., arrays: `[a, b]` vs `- a\n- b`)
- **Mitigation**: Use simple YAML features only (key: value, arrays, no anchors/aliases)

**Delimiter Ambiguity**:
- Content containing `\n---\n` could theoretically confuse parser
- **Mitigation**: Parser uses first occurrence only; subsequent `---` are content

**No Schema Validation**:
- YAML parser accepts any valid YAML (no type checking)
- Typos in field names silently ignored (e.g., `titel: Foo` instead of `title: Foo`)
- **Mitigation**: Future validation layer on top of parser (not in v1.0.0)

**Parsing Overhead**:
- YAML parsing slower than JSON (due to indentation processing)
- **Impact**: Negligible for small files (~0.1 ms per file)

---

## Alternatives Considered

### Alternative 1: JSON Frontmatter

**Approach**:
```json
{
  "type": "pattern",
  "title": "Error Handling",
  "tags": ["go", "errors"]
}
---
# Markdown content
```

**Rejected Because**:
- Verbose (requires quotes on keys and string values)
- No comments support
- Less human-friendly (commas, braces)
- Not standard in markdown ecosystem (YAML is dominant)

### Alternative 2: TOML Frontmatter

**Approach**:
```toml
+++
type = "pattern"
title = "Error Handling"
tags = ["go", "errors"]
+++
# Markdown content
```

**Rejected Because**:
- Less familiar to users (Hugo-specific)
- Delimiter `+++` is non-standard
- TOML parsing libraries less common than YAML
- No significant advantages over YAML

### Alternative 3: Custom Key-Value Format

**Approach**:
```
Type: pattern
Title: Error Handling
Tags: go, errors
---
# Markdown content
```

**Rejected Because**:
- Need to write custom parser (reinventing the wheel)
- No standard for arrays, nested objects, timestamps
- Harder to extend (need to define syntax for new types)
- No ecosystem tooling (linters, validators, etc.)

### Alternative 4: No Frontmatter (Markdown Headers)

**Approach**:
```markdown
# Error Handling

**Type**: pattern
**Tags**: go, errors

Prefer explicit error returns...
```

**Rejected Because**:
- Metadata mixed with content (harder to parse)
- No structured data (need to parse markdown to extract metadata)
- Can't have markdown formatting in metadata (e.g., links in description)

---

## Implementation Notes

**Parser Logic** (in `parser.go`):
```go
func splitFrontmatter(data []byte) (frontmatter, content []byte, err error) {
    // Must start with "---\n"
    if !bytes.HasPrefix(data, []byte("---\n")) {
        return nil, nil, fmt.Errorf("missing frontmatter delimiter")
    }

    // Find closing "\n---\n"
    rest := data[4:] // Skip "---\n"
    idx := bytes.Index(rest, []byte("\n---\n"))
    if idx == -1 {
        return nil, nil, fmt.Errorf("missing closing frontmatter delimiter")
    }

    frontmatter = rest[:idx]
    content = rest[idx+5:] // Skip "\n---\n"
    return frontmatter, content, nil
}
```

**YAML Parsing** (in `parser.go`):
```go
import "gopkg.in/yaml.v3"

var fm Frontmatter
if err := yaml.Unmarshal(frontmatter, &fm); err != nil {
    return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
}
```

**Struct Tags** (in `engram.go`):
```go
type Frontmatter struct {
    Type        string    `yaml:"type"`
    Title       string    `yaml:"title"`
    Description string    `yaml:"description"`
    Tags        []string  `yaml:"tags"`
    Agents      []string  `yaml:"agents,omitempty"`
    LoadWhen    string    `yaml:"load_when,omitempty"`
    Modified    time.Time `yaml:"modified,omitempty"`
    // ... memory strength fields
}
```

**Edge Cases**:
- Empty frontmatter (`---\n---\n`) → Results in empty YAML (parse error)
- Content with `---` → Only first `\n---\n` is delimiter
- Windows line endings (`\r\n`) → Not supported (use Unix `\n`)
- Leading/trailing whitespace in frontmatter → Ignored by YAML parser
- Unquoted array syntax in string fields (`type: [a|b|c]`) → YAML interprets as sequence, not string (parse error)
  - **Fix**: Use quoted strings for template placeholders: `type: "pattern"` not `type: [prompt|instruction|pattern]`
  - **Context**: Template files should use valid YAML values, not placeholder syntax (issue discovered 2026-02-19)

---

## Related Decisions

- **ADR-002**: Backward Compatibility via Defaults (how missing fields are handled)
- **ADR-003**: Memory Strength Tracking Fields (which fields are in frontmatter)

---

## References

- **YAML Specification**: https://yaml.org/spec/1.2/spec.html
- **Jekyll Frontmatter**: https://jekyllrb.com/docs/front-matter/
- **Hugo Frontmatter**: https://gohugo.io/content-management/front-matter/
- **go-yaml Library**: https://github.com/go-yaml/yaml

---

## Revision History

- **2026-02-11**: ADR created (backfilled from existing implementation)

# frontmatter

Go library for markdown section parsing with fuzzy matching capabilities.

## Overview

This package provides markdown section extraction with:
- Accurate line number tracking
- YAML frontmatter handling
- Fuzzy matching using Levenshtein distance
- High performance (10-30x faster than TypeScript equivalent)

Migrated from `core/cortex/lib/section-parser.ts` (Phase 1, Task 1.7 - Language Audit).

## Installation

```bash
go get github.com/vbonnet/engram/libs/frontmatter
```

## Usage

### Basic Parsing

```go
package main

import (
    "fmt"
    "log"

    "github.com/vbonnet/engram/libs/frontmatter"
)

func main() {
    parser := frontmatter.NewParser()

    markdown := `# Introduction
## Section 1
### Subsection 1.1`

    sections, err := parser.Parse(markdown)
    if err != nil {
        log.Fatal(err)
    }

    for _, section := range sections {
        fmt.Printf("Line %d: %s (Level %d)\n",
            section.StartLine, section.Heading, section.Level)
    }
}
```

### Fuzzy Matching

```go
parser := frontmatter.NewParser()

// 80% similarity threshold
if parser.FuzzyMatch("Accept Criteria", "Acceptance Criteria", 0.80) {
    fmt.Println("Close match!")
}
```

### Finding Sections

```go
sections := []frontmatter.Section{
    {Heading: "Acceptance Criteria", Level: 2, StartLine: 10},
    {Heading: "Task Breakdown", Level: 2, StartLine: 20},
}

// Exact match
exact := parser.FindSections(sections, "Acceptance Criteria", false)

// Fuzzy match (75% threshold)
fuzzy := parser.FindSections(sections, "Accept Criteria", true)
```

## Features

### YAML Frontmatter Support

Automatically skips YAML frontmatter delimited by `---`:

```markdown
---
title: Document
author: User
---

# Actual Heading
```

### Inline Formatting Stripping

Extracts plain text from headings with formatting:
- **Bold** and *italic*
- `Inline code`
- [Links](url)
- And other markdown formatting

### Performance

- **Large documents**: Parses 1000 headings in <200ms
- **Fuzzy matching**: 10,000 comparisons in <500ms
- **Coverage**: 92.2% test coverage

## API

### Types

```go
type Section struct {
    Heading   string `json:"heading"`
    Level     int    `json:"level"`
    StartLine int    `json:"startLine"`
    Raw       string `json:"raw,omitempty"`
}

type Parser struct {
    // private fields
}
```

### Methods

#### `NewParser() *Parser`

Creates a new markdown section parser.

#### `Parse(markdown string) ([]Section, error)`

Extracts all headings from a markdown document.

#### `FuzzyMatch(heading, pattern string, threshold float64) bool`

Checks if heading matches pattern using Levenshtein distance.

- `threshold`: 0.0-1.0 (default: 0.75 recommended)
- Returns `true` if similarity >= threshold

#### `FindSections(sections []Section, pattern string, fuzzy bool) []Section`

Finds sections matching a pattern.

- `fuzzy`: Use fuzzy matching (true) or exact (false)

## Testing

```bash
make test      # Run tests
make lint      # Run golangci-lint
make build     # Build package
```

## Migration Notes

This package replaces the TypeScript `SectionParser` class from `core/cortex/lib/section-parser.ts`.

### Key Differences

1. **Performance**: 10-30x faster than TypeScript version
2. **Dependencies**: Uses `goldmark` instead of `remark`
3. **API**: Go idiomatic design (exported functions, error handling)
4. **Frontmatter**: Built-in YAML frontmatter detection

### TypeScript → Go Mapping

| TypeScript | Go |
|------------|-----|
| `new SectionParser()` | `frontmatter.NewParser()` |
| `parser.parse(markdown)` | `parser.Parse(markdown)` |
| `parser.fuzzyMatch(h, p, t)` | `parser.FuzzyMatch(h, p, t)` |
| `parser.findSections(s, p, f)` | `parser.FindSections(s, p, f)` |

## License

Same as parent project.

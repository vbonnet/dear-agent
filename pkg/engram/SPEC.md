# Engram Package - Specification

**Version**: 1.0.0
**Last Updated**: 2026-02-11
**Status**: Implemented
**Package**: github.com/vbonnet/engram/core/pkg/engram

---

## Vision

The engram package provides core types and parsing utilities for .ai.md memory trace files. It solves the problem of representing and loading learned knowledge (patterns, strategies, workflows) in a structured, machine-readable format that AI agents can retrieve and apply to new contexts.

Engrams are the fundamental unit of the ecphory (AI memory retrieval) system. They capture reusable knowledge as markdown files with YAML frontmatter, enabling AI agents to learn from past experiences and apply that knowledge to future tasks. This package handles the parsing, validation, and representation of these memory traces with full backward compatibility for legacy engrams created before metadata tracking was implemented.

---

## Goals

### 1. Parse .ai.md Files with Frontmatter

Reliably parse engram files containing YAML frontmatter and markdown content, extracting both metadata and body text.

**Success Metric**: Parser successfully extracts frontmatter metadata (type, title, description, tags) and markdown content from all valid .ai.md files, with clear error messages for malformed files.

### 2. Support Memory Strength Tracking

Provide fields for tracking engram usage, quality, and temporal patterns to enable future retrieval prioritization and active forgetting mechanisms.

**Success Metric**: Frontmatter includes encoding_strength, retrieval_count, created_at, and last_accessed fields with appropriate defaults and semantics.

### 3. Backward Compatibility with Legacy Engrams

Ensure existing engrams without metadata fields (created before tracking was implemented) continue to work without modification.

**Success Metric**: Parser applies sensible defaults for missing metadata fields (encoding_strength = 1.0, retrieval_count = 0, created_at from file mtime) and never fails on legacy engrams.

### 4. Type Safety and Validation

Provide strongly-typed representations of engram metadata to prevent errors and enable compile-time checking.

**Success Metric**: Frontmatter struct uses appropriate Go types (string, []string, time.Time, float64, int) with YAML tags for serialization.

---

## Architecture

### High-Level Design

The package provides a simple parser that reads .ai.md files, splits frontmatter from content, unmarshals YAML metadata, and applies backward compatibility defaults.

```
┌─────────────────┐
│  .ai.md File    │
│                 │
│  ---            │
│  frontmatter    │
│  ---            │
│  # Markdown     │
│  content        │
└────────┬────────┘
         │
         v
    ┌────────────┐
    │   Parser   │
    └────┬───────┘
         │
         v
    ┌────────────────┐
    │  splitFrontmatter │
    │  (---...---)    │
    └────┬───────────┘
         │
         ├──> YAML Frontmatter
         │         │
         │         v
         │    ┌─────────────┐
         │    │ yaml.Unmarshal │
         │    └─────┬───────┘
         │          │
         │          v
         │    ┌──────────────┐
         │    │ Frontmatter  │
         │    │ struct       │
         │    └──────┬───────┘
         │           │
         │           v
         │    ┌──────────────────┐
         │    │ Apply Defaults   │
         │    │ (backward compat)│
         │    └──────┬───────────┘
         │           │
         └──> Markdown Content
                     │
                     v
              ┌─────────────┐
              │   Engram    │
              │   struct    │
              └─────────────┘
```

### Components

**Component 1: Parser**
- **Purpose**: Parse .ai.md files into Engram structs
- **Responsibilities**:
  - Read file from disk or bytes
  - Split frontmatter from content
  - Unmarshal YAML frontmatter
  - Apply backward compatibility defaults
  - Populate Engram struct
- **Interfaces**: Public API (`NewParser`, `Parse`, `ParseBytes`)

**Component 2: Engram Type**
- **Purpose**: Represent a parsed engram with metadata and content
- **Responsibilities**:
  - Store file path for tracking
  - Hold parsed frontmatter metadata
  - Hold markdown content body
- **Interfaces**: Public struct with exported fields

**Component 3: Frontmatter Type**
- **Purpose**: Represent engram metadata from YAML frontmatter
- **Responsibilities**:
  - Store core metadata (type, title, description, tags)
  - Store optional metadata (agents, load_when, modified)
  - Store memory strength tracking fields (encoding_strength, retrieval_count, created_at, last_accessed)
  - Provide YAML serialization tags
- **Interfaces**: Public struct with exported fields and YAML tags

**Component 4: Frontmatter Splitter**
- **Purpose**: Extract frontmatter and content from raw file bytes
- **Responsibilities**:
  - Detect opening --- delimiter
  - Find closing --- delimiter
  - Split at delimiters
  - Return separate frontmatter and content byte slices
- **Interfaces**: Internal method (`splitFrontmatter`)

### Data Flow

1. **File Read**: Parser reads file from disk (`Parse`) or accepts bytes (`ParseBytes`)

2. **Frontmatter Splitting**:
   - Verify file starts with `---\n`
   - Find closing `\n---\n`
   - Extract frontmatter between delimiters
   - Extract content after closing delimiter

3. **YAML Parsing**:
   - Unmarshal frontmatter bytes into Frontmatter struct
   - YAML library populates fields based on yaml tags

4. **Backward Compatibility**:
   - If `encoding_strength == 0.0`, set to `1.0` (neutral default)
   - If `created_at.IsZero()`, set from file mtime (legacy engrams)
   - `retrieval_count` defaults to 0 (zero value is correct)
   - `last_accessed` defaults to zero (never accessed)

5. **Engram Construction**: Return Engram struct with path, frontmatter, and content

### Key Design Decisions

- **Decision: YAML Frontmatter Format**
  - Delimiter-based format (`---` lines) is standard in static site generators
  - YAML chosen over JSON/TOML for readability and comment support
  - Allows human-readable metadata with minimal syntax

- **Decision: Backward Compatibility via Defaults**
  - Apply defaults during parsing, not at struct initialization
  - Preserves original file content (no rewriting)
  - Enables gradual migration of legacy engrams
  - Uses file mtime as fallback for `created_at` (best available timestamp)

- **Decision: Memory Strength Tracking Fields**
  - `encoding_strength`: Intrinsic quality (0.0-2.0, default 1.0)
  - `retrieval_count`: Usage counter (incremented on retrieval)
  - `created_at`: Creation timestamp (immutable)
  - `last_accessed`: Last retrieval timestamp (updated on use)
  - Enables future features: temporal decay, usage-based ranking, active forgetting

---

## Success Metrics

### Primary Metrics

- **Parsing Accuracy**: 100% success rate on valid .ai.md files
- **Backward Compatibility**: All legacy engrams parse without errors
- **Type Safety**: Zero runtime type errors (strong Go typing)

### Secondary Metrics

- **Error Messages**: Clear, actionable errors for malformed files (missing delimiters, invalid YAML)
- **Test Coverage**: ≥85% coverage on parser logic (valid/invalid inputs, defaults)
- **Performance**: Parse 1000 engrams in <100ms (cached file reads)

---

## What This SPEC Doesn't Cover

- **Engram Creation**: Writing new .ai.md files (separate tool)
- **Frontmatter Validation**: Semantic validation (e.g., valid engram types, required fields)
- **Content Processing**: Markdown parsing, rendering, or transformation
- **Metadata Updates**: Incrementing retrieval_count or updating last_accessed (handled by ecphory system)
- **Indexing**: Building search indexes from parsed engrams (separate package)
- **Retrieval**: Querying and ranking engrams (ecphory package)

Future considerations:
- Validation rules for frontmatter (e.g., type must be pattern/strategy/workflow)
- Schema versioning for frontmatter format
- Incremental parsing for very large files

---

## Assumptions & Constraints

### Assumptions

- Engram files are UTF-8 encoded
- Frontmatter is valid YAML (no tabs, proper indentation)
- Files are reasonably sized (<10 MB)
- File system access is available (for `Parse` method)
- File modification times are preserved (for created_at fallback)

### Constraints

- **Dependency Constraints**:
  - Requires `gopkg.in/yaml.v3` for YAML parsing
  - No other external dependencies
- **Format Constraints**:
  - Frontmatter must be at start of file (no content before `---`)
  - Frontmatter delimiters must be on separate lines (`---\n`, not `---\r\n` only)
  - Content may contain `---` (only first closing delimiter is used)
- **Performance Constraints**:
  - Entire file loaded into memory (not streaming)
  - Single-threaded parsing (no concurrent safety)

---

## Dependencies

### External Libraries

- `gopkg.in/yaml.v3` - YAML parsing and unmarshaling

### Internal Dependencies

- `os` - File reading and stat (file mtime)
- `bytes` - Byte slice manipulation (frontmatter splitting)
- `fmt` - Error formatting
- `time` - Timestamp handling

---

## API Reference

### Types

```go
// Engram represents a single .ai.md memory trace file
type Engram struct {
    Path        string       // File path (for tracking purposes)
    Frontmatter Frontmatter  // Frontmatter metadata
    Content     string       // Content body (markdown)
}

// Frontmatter contains engram metadata
type Frontmatter struct {
    // Core metadata
    Type        string    `yaml:"type"`
    Title       string    `yaml:"title"`
    Description string    `yaml:"description"`
    Tags        []string  `yaml:"tags"`

    // Optional metadata
    Agents      []string  `yaml:"agents,omitempty"`
    LoadWhen    string    `yaml:"load_when,omitempty"`
    Modified    time.Time `yaml:"modified,omitempty"`

    // Memory strength tracking
    EncodingStrength float64   `yaml:"encoding_strength,omitempty"`  // 0.0-2.0, default 1.0
    RetrievalCount   int       `yaml:"retrieval_count,omitempty"`    // Usage counter
    CreatedAt        time.Time `yaml:"created_at,omitempty"`          // Creation timestamp
    LastAccessed     time.Time `yaml:"last_accessed,omitempty"`       // Last retrieval
}
```

### Constants

```go
const (
    TypePattern  = "pattern"   // Reusable code patterns and idioms
    TypeStrategy = "strategy"  // High-level approaches to solving problems
    TypeWorkflow = "workflow"  // Multi-step processes and procedures
)
```

### Functions

```go
// NewParser creates a new engram parser
func NewParser() *Parser

// Parse parses an engram file from disk and returns an Engram
func (p *Parser) Parse(path string) (*Engram, error)

// ParseBytes parses engram content from bytes
func (p *Parser) ParseBytes(path string, data []byte) (*Engram, error)
```

### Error Conditions

```go
// Parse errors
"failed to read engram file: %w"           // File read error (file not found, permissions)
"missing frontmatter delimiter"            // File doesn't start with ---
"missing closing frontmatter delimiter"    // No closing --- found
"failed to parse frontmatter: %w"          // Invalid YAML syntax
```

---

## Testing Strategy

### Unit Tests

- **Valid Inputs**: Parse complete engrams with all fields
- **Minimal Inputs**: Parse engrams with only required fields (type, title, description)
- **Frontmatter Variations**: Timestamps, tags, agents, load_when
- **Content Variations**: Empty content, content with `---`, multiline content
- **Error Cases**: Missing delimiters, unclosed frontmatter, invalid YAML
- **Backward Compatibility**: Legacy engrams without metadata fields

### Edge Case Tests

- Empty frontmatter (consecutive delimiters)
- Content containing `---` (should not confuse parser)
- File not found
- Large files (performance)

### Test Files

- Located in testdata/guidance/ (test-*.ai.md files)
- Include valid and malformed examples
- Cover all engram types (pattern, strategy, workflow)

---

## Version History

- **1.0.0** (2026-02-11): Initial implementation and documentation backfill

---

**Note**: This package is the foundation of the engram ecphory system. All engram files must be parseable by this package. See ARCHITECTURE.md for detailed design and ADRs for decision rationale.

# Engram Package - Architecture

**Version**: 1.0.0
**Last Updated**: 2026-02-11
**Status**: Implemented
**Package**: github.com/vbonnet/engram/core/pkg/engram

---

## Overview

The engram package provides parsing and representation of .ai.md memory trace files. It is the foundational layer for the ecphory (AI memory retrieval) system, converting human-readable markdown files with YAML frontmatter into structured Go data types.

**Key Principles**:
- Simple, focused API (parse files, return structs)
- Backward compatibility with legacy engrams (default values for missing fields)
- No external dependencies beyond YAML parsing
- Type-safe representation of metadata

---

## Architecture Diagram

```
┌───────────────────────────────────────────────────────────────┐
│                      Application Layer                        │
│                   (ecphory, indexing, CLI)                    │
│                                                                │
│  Uses: parser.Parse("file.ai.md") → *Engram                  │
└─────────────────────────┬─────────────────────────────────────┘
                          │
                          v
┌───────────────────────────────────────────────────────────────┐
│                       Parser (Public API)                     │
│                                                                │
│  Methods:                                                     │
│  - Parse(path string) → *Engram, error                       │
│  - ParseBytes(path, data []byte) → *Engram, error            │
│                                                                │
│  Responsibilities:                                             │
│  - Read file or accept bytes                                  │
│  - Split frontmatter from content                             │
│  - Unmarshal YAML frontmatter                                 │
│  - Apply backward compatibility defaults                      │
│  - Return populated Engram struct                             │
└─────────────────────────┬─────────────────────────────────────┘
                          │
                          v
┌───────────────────────────────────────────────────────────────┐
│                  splitFrontmatter (Internal)                  │
│                                                                │
│  Input:  []byte (entire file)                                │
│  Output: frontmatter []byte, content []byte, error            │
│                                                                │
│  Logic:                                                        │
│  1. Check file starts with "---\n"                            │
│  2. Find closing "\n---\n"                                    │
│  3. Split at delimiters                                       │
│  4. Return frontmatter and content separately                 │
└─────┬──────────────────────────────────┬──────────────────────┘
      │                                  │
      v                                  v
┌──────────────────┐            ┌────────────────────┐
│  YAML Parsing    │            │  Content Extract   │
│  (yaml.Unmarshal)│            │  (string)          │
└────────┬─────────┘            └──────────┬─────────┘
         │                                  │
         v                                  │
┌──────────────────┐                       │
│  Frontmatter     │                       │
│  struct          │                       │
│                  │                       │
│  Fields:         │                       │
│  - Type          │                       │
│  - Title         │                       │
│  - Description   │                       │
│  - Tags          │                       │
│  - Agents        │                       │
│  - LoadWhen      │                       │
│  - Modified      │                       │
│  - Encoding...   │                       │
│  - Retrieval...  │                       │
│  - CreatedAt     │                       │
│  - LastAccessed  │                       │
└────────┬─────────┘                       │
         │                                  │
         v                                  │
┌──────────────────────────────────────────┴──────────┐
│         Backward Compatibility Layer                │
│                                                      │
│  If encoding_strength == 0.0 → Set to 1.0          │
│  If created_at.IsZero() → Set from file mtime      │
│  retrieval_count defaults to 0 (zero value OK)     │
│  last_accessed defaults to zero (never accessed)   │
└────────┬─────────────────────────────────────────────┘
         │
         v
┌──────────────────────────────────────────────────────┐
│                 Engram struct                        │
│                                                       │
│  Path: string        (file path)                     │
│  Frontmatter: Frontmatter (metadata)                 │
│  Content: string     (markdown body)                 │
└──────────────────────────────────────────────────────┘
```

---

## Component Details

### 1. Parser (Public API)

**File**: `parser.go`

**Purpose**: Parse .ai.md files into Engram structs

**Responsibilities**:
- Provide public parsing interface
- Handle file I/O (or accept pre-read bytes)
- Coordinate splitting, unmarshaling, and defaulting
- Return populated Engram or error

**Key Fields**:
```go
type Parser struct{}  // Stateless, can be reused
```

**Key Methods**:

```go
func NewParser() *Parser
    → Returns new parser instance (stateless, but follows standard pattern)

func (p *Parser) Parse(path string) (*Engram, error)
    → Reads file from disk using os.ReadFile
    → Calls ParseBytes with path and data
    → Returns Engram or error

func (p *Parser) ParseBytes(path string, data []byte) (*Engram, error)
    → Calls splitFrontmatter to extract frontmatter and content
    → Unmarshals frontmatter YAML into Frontmatter struct
    → Applies backward compatibility defaults:
        - encoding_strength: 1.0 if zero
        - created_at: file mtime if zero (requires os.Stat on path)
    → Returns Engram{Path, Frontmatter, Content}
```

**Design Decisions**:
- **Why separate Parse and ParseBytes?** → Parse is convenience for file-based usage, ParseBytes enables testing and in-memory parsing
- **Why pass path to ParseBytes?** → Needed for Engram.Path field and file mtime fallback for created_at
- **Why stateless Parser?** → No state needed, but struct allows future extension (e.g., validation options)

**Error Handling**:
- File read errors (wrapped with context)
- Frontmatter splitting errors (missing delimiters)
- YAML unmarshaling errors (invalid syntax)
- No recovery from errors (fail fast)

---

### 2. Engram Type

**File**: `engram.go`

**Purpose**: Represent a parsed engram with metadata and content

**Responsibilities**:
- Store all engram data (path, metadata, content)
- Provide public access to fields (no getters/setters)
- Document field semantics

**Key Fields**:
```go
type Engram struct {
    Path        string       // File path (e.g., "patterns/go-errors.ai.md")
    Frontmatter Frontmatter  // Parsed metadata
    Content     string       // Markdown body (after frontmatter)
}
```

**Design Decisions**:
- **Why store Path?** → Enables tracking which file an engram came from (useful for updates, debugging)
- **Why string Content not []byte?** → Content is always displayed/processed as text, string is more natural
- **Why Frontmatter not pointer?** → Value type ensures it's always present (no nil checks)

---

### 3. Frontmatter Type

**File**: `engram.go`

**Purpose**: Represent engram metadata from YAML frontmatter

**Responsibilities**:
- Define all metadata fields with appropriate types
- Provide YAML serialization tags
- Document field semantics and defaults

**Key Fields**:
```go
type Frontmatter struct {
    // Core metadata (required in practice, but not enforced)
    Type        string    `yaml:"type"`           // pattern, strategy, workflow
    Title       string    `yaml:"title"`
    Description string    `yaml:"description"`
    Tags        []string  `yaml:"tags"`

    // Optional metadata
    Agents      []string  `yaml:"agents,omitempty"`       // Filter by agent platform
    LoadWhen    string    `yaml:"load_when,omitempty"`    // Natural language condition
    Modified    time.Time `yaml:"modified,omitempty"`     // Last edit timestamp

    // Memory strength tracking (for ecphory prioritization)
    EncodingStrength float64   `yaml:"encoding_strength,omitempty"`  // 0.0-2.0
    RetrievalCount   int       `yaml:"retrieval_count,omitempty"`    // Usage counter
    CreatedAt        time.Time `yaml:"created_at,omitempty"`          // Creation time
    LastAccessed     time.Time `yaml:"last_accessed,omitempty"`       // Last retrieval
}
```

**Field Semantics**:

**Core Metadata**:
- `Type`: Engram category (pattern/strategy/workflow) - used for filtering
- `Title`: Human-readable name (used in search results)
- `Description`: Brief summary (used for relevance matching)
- `Tags`: Hierarchical tags (e.g., "languages/go", "frameworks/fastapi") - used for filtering

**Optional Metadata**:
- `Agents`: Agent platforms this engram applies to (empty = all agents)
- `LoadWhen`: Natural language condition for automatic loading
- `Modified`: Last modification timestamp (manually maintained)

**Memory Strength Tracking**:
- `EncodingStrength`: Intrinsic quality/importance (0.0 = low, 1.0 = neutral, 2.0 = exceptional)
- `RetrievalCount`: How many times retrieved (higher = more useful/popular)
- `CreatedAt`: When engram was created (for temporal decay calculations)
- `LastAccessed`: When last retrieved (for recency-based prioritization)

**Design Decisions**:
- **Why omitempty on optional fields?** → Keeps YAML files clean (don't serialize zero values)
- **Why time.Time not string?** → Type safety, automatic parsing/formatting via YAML library
- **Why EncodingStrength float64?** → Allows fine-grained quality scores (not just int ratings)
- **Why separate Modified and CreatedAt?** → Modified is manual edit tracking, CreatedAt is immutable first-creation timestamp

**Backward Compatibility Defaults** (applied in ParseBytes):
- `EncodingStrength == 0.0` → Set to `1.0` (neutral quality)
- `CreatedAt.IsZero()` → Set from file mtime (best available timestamp)
- `RetrievalCount == 0` → Leave as 0 (zero value is correct)
- `LastAccessed.IsZero()` → Leave as zero (never accessed)

---

### 4. splitFrontmatter (Internal)

**File**: `parser.go`

**Purpose**: Extract frontmatter and content from raw file bytes

**Responsibilities**:
- Detect frontmatter delimiters (`---`)
- Split file into frontmatter and content sections
- Return byte slices for each section
- Validate delimiter format

**Key Logic**:
```go
func (p *Parser) splitFrontmatter(data []byte) (frontmatter, content []byte, err error)
    1. Check file starts with "---\n" (4 bytes)
       → If not: return "missing frontmatter delimiter" error

    2. Skip opening delimiter, search for closing "\n---\n" (5 bytes)
       → rest = data[4:]  // Skip "---\n"
       → idx = bytes.Index(rest, []byte("\n---\n"))
       → If idx == -1: return "missing closing frontmatter delimiter" error

    3. Split at closing delimiter
       → frontmatter = rest[:idx]
       → content = rest[idx+5:]  // Skip "\n---\n"

    4. Return frontmatter, content, nil
```

**Design Decisions**:
- **Why require newlines around `---`?** → Prevents false matches (e.g., "2023-11-27" contains "---")
- **Why not allow `\r\n`?** → Simplicity; YAML files should use Unix line endings
- **Why allow `---` in content?** → Only first closing delimiter is used; subsequent `---` are part of content
- **Why return byte slices not strings?** → Efficient (no copying); yaml.Unmarshal accepts []byte

**Edge Cases**:
- File with only frontmatter (no content after `---`) → content = empty slice (valid)
- File with `---` in content → Only first `\n---\n` is closing delimiter
- Consecutive delimiters (`---\n---\n`) → Results in empty frontmatter (will fail YAML parsing)

---

## Data Flow

### Successful Parse Flow

```
Application: parser.Parse("patterns/go-errors.ai.md")
    ↓
Parser.Parse():
    os.ReadFile("patterns/go-errors.ai.md") → data []byte
    ↓
Parser.ParseBytes(path, data):
    splitFrontmatter(data) → frontmatter, content, nil
    ↓
splitFrontmatter():
    data starts with "---\n" ✓
    Find closing "\n---\n" at index 123
    frontmatter = data[4:123]
    content = data[128:]  // Skip "\n---\n"
    ↓
Parser.ParseBytes():
    yaml.Unmarshal(frontmatter, &fm) → Frontmatter struct
    ↓
Backward Compatibility:
    fm.EncodingStrength == 0.0 → Set to 1.0
    fm.CreatedAt.IsZero() → os.Stat → mtime → 2024-11-27
    ↓
Parser.ParseBytes():
    return &Engram{
        Path: "patterns/go-errors.ai.md",
        Frontmatter: fm,
        Content: string(content),
    }, nil
```

### Error Flow: Missing Closing Delimiter

```
Application: parser.Parse("invalid.ai.md")
    ↓
Parser.Parse():
    os.ReadFile("invalid.ai.md") → data = "---\ntype: pattern\nNo closing"
    ↓
Parser.ParseBytes(path, data):
    splitFrontmatter(data)
    ↓
splitFrontmatter():
    data starts with "---\n" ✓
    Find closing "\n---\n"... NOT FOUND
    return nil, nil, "missing closing frontmatter delimiter"
    ↓
Parser.ParseBytes():
    return nil, error
    ↓
Application: receives error
```

### Error Flow: Invalid YAML

```
Application: parser.Parse("bad-yaml.ai.md")
    ↓
Parser.Parse():
    os.ReadFile("bad-yaml.ai.md") → data
    ↓
Parser.ParseBytes(path, data):
    splitFrontmatter(data) → frontmatter, content, nil
    ↓
Parser.ParseBytes():
    yaml.Unmarshal(frontmatter, &fm) → ERROR (unclosed quote)
    return nil, "failed to parse frontmatter: yaml: ..."
    ↓
Application: receives error
```

---

## Threading Model

**Single-Threaded by Design**: The parser is **not thread-safe** for parsing the same file concurrently, but multiple parsers can be used in parallel.

**Safe Concurrent Usage**:
- Create separate `Parser` instances per goroutine (parser is stateless)
- Parse different files in parallel (no shared state)

**Unsafe Concurrent Usage**:
- Multiple goroutines calling same parser instance is safe (parser is stateless)
- BUT: os.ReadFile and os.Stat are thread-safe, so even same parser is safe

**Conclusion**: Parser is effectively thread-safe due to being stateless. No synchronization needed.

---

## Error Handling

**Philosophy**: Fail fast with clear, actionable error messages.

**Error Categories**:

1. **File I/O Errors** (from `Parse`):
   - File not found
   - Permission denied
   - Wrapped with context: `"failed to read engram file: %w"`

2. **Format Errors** (from `splitFrontmatter`):
   - Missing opening delimiter: `"missing frontmatter delimiter"`
   - Missing closing delimiter: `"missing closing frontmatter delimiter"`

3. **YAML Errors** (from `yaml.Unmarshal`):
   - Invalid YAML syntax (unclosed quotes, indentation errors, etc.)
   - Wrapped with context: `"failed to parse frontmatter: %w"`

**Error Handling Strategy**:
- No recovery (errors are fatal for that file)
- Context wrapping (`fmt.Errorf(..., %w)`) for error chains
- Descriptive messages (what went wrong, not just "error")

**Backward Compatibility Errors**:
- File stat errors (when getting mtime) are **ignored** (created_at left as zero)
- Rationale: Missing mtime is acceptable (can be set later); parsing should still succeed

---

## Testing Strategy

### Unit Tests (parser_test.go)

**Test Coverage**:
- Valid engram parsing (full frontmatter, minimal frontmatter)
- Frontmatter variations (timestamps, arrays, optional fields)
- Content variations (empty, multiline, containing `---`)
- Error cases (missing delimiters, invalid YAML)
- File-based parsing (`Parse` method)
- Byte-based parsing (`ParseBytes` method)
- Frontmatter splitting edge cases

**Test Philosophy**:
- Test public API only (Parse, ParseBytes)
- Test both success and failure paths
- Use table-driven tests for splitFrontmatter edge cases
- No mocking (real YAML library, real file I/O)

### Backward Compatibility Tests (parser_backward_compat_test.go)

**Test Scenarios**:
- Legacy engram (no metadata fields) → Defaults applied
- New engram (all metadata fields) → Values preserved
- Partial metadata (some fields missing) → Explicit fields preserved, missing fields defaulted

**Verification**:
- EncodingStrength defaults to 1.0
- RetrievalCount defaults to 0
- CreatedAt set from file mtime (legacy) or frontmatter (new)
- LastAccessed remains zero (legacy) or frontmatter value (new)

### Test Data

**Location**: `testdata/guidance/`
- `test-errors.ai.md` - Valid pattern example
- `test-testing.ai.md` - Valid strategy example
- `test-encryption.ai.md` - Valid workflow example
- `malformed.ai.md` - Invalid file for error testing

---

## Dependencies

### External Dependencies

**gopkg.in/yaml.v3**
- Used by: `Parser.ParseBytes`
- Purpose: Unmarshal YAML frontmatter into Frontmatter struct
- Why chosen: Standard Go YAML library, well-maintained, supports all YAML features

### Standard Library Dependencies

- `os` - File reading (`ReadFile`) and stat (`Stat` for mtime)
- `bytes` - Frontmatter splitting (`HasPrefix`, `Index`)
- `fmt` - Error formatting (`Errorf` with `%w`)
- `time` - Timestamp handling (`time.Time` in Frontmatter)

### Dependency Isolation

**Strategy**: YAML library is only dependency; rest is standard library

**Benefits**:
- Minimal dependency footprint
- Easy to vendor or replace YAML library if needed
- No transitive dependencies beyond yaml.v3

---

## Performance Considerations

### Memory Usage

**Per Engram**:
- `Path`: ~50 bytes (string)
- `Frontmatter`: ~200 bytes (struct with strings, slices, timestamps)
- `Content`: ~5 KB average (varies by engram size)
- **Total**: ~5-10 KB per engram

**Parsing 1000 Engrams**: ~5-10 MB total (all in memory simultaneously)

### CPU Usage

**Parsing Time**:
- File read: ~0.01 ms (SSD)
- Frontmatter split: ~0.001 ms (byte slicing)
- YAML unmarshal: ~0.1 ms (YAML parsing)
- **Total per file**: ~0.1-0.2 ms

**Parsing 1000 Engrams**: ~100-200 ms (sequential)

**Optimization Opportunities**:
- Parallel parsing (use goroutines for multiple files)
- Caching (don't re-parse unchanged files)
- Lazy content loading (parse frontmatter only, load content on demand)

---

## Future Enhancements

### Potential Improvements (Not in Scope for v1.0.0)

**Validation**:
- Validate engram type (must be pattern/strategy/workflow)
- Validate required fields (type, title, description)
- Validate encoding_strength range (0.0-2.0)

**Schema Versioning**:
- Add `schema_version` field to frontmatter
- Support parsing multiple schema versions
- Migration tools for old schemas

**Incremental Parsing**:
- Parse frontmatter only (skip content)
- Load content on demand (lazy loading)
- Stream parsing for very large files

**Content Processing**:
- Parse markdown content into AST
- Extract headings, code blocks, etc.
- Support rendering to HTML

---

## ADR References

See `docs/adrs/` directory for detailed decision records:

- **ADR-001**: YAML Frontmatter Format
- **ADR-002**: Backward Compatibility via Defaults
- **ADR-003**: Memory Strength Tracking Fields

---

## Version History

- **1.0.0** (2026-02-11): Initial implementation and architecture documentation backfill

---

**Maintained by**: Engram Core Team
**Questions**: See SPEC.md for requirements, README.md for usage examples

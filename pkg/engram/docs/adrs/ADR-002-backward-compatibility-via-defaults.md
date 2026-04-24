# ADR-002: Backward Compatibility via Defaults

**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)
**Deciders**: Engram Core Team
**Context**: Adding memory strength tracking to existing engram format

---

## Context

The engram system was initially designed with basic metadata fields (type, title, description, tags). Later, we added memory strength tracking fields to support advanced retrieval features:

- `encoding_strength` - Intrinsic quality/importance (0.0-2.0, default 1.0)
- `retrieval_count` - Number of times retrieved (usage tracking)
- `created_at` - Creation timestamp (for temporal decay)
- `last_accessed` - Last retrieval timestamp (for recency-based ranking)

**Problem**: Many existing engram files were created before these fields existed. We need to support:

1. **Legacy engrams**: Files without metadata fields (should still parse)
2. **Partial metadata**: Files with some but not all fields (should fill in missing fields)
3. **New engrams**: Files with all metadata fields (should preserve values as-is)

**Options for handling missing fields**:
- **Require all fields**: Force users to update all engrams (breaks existing files)
- **Rewrite files on parse**: Add missing fields to file on disk (modifies user files)
- **Apply defaults at runtime**: Fill in missing fields during parsing (non-invasive)

---

## Decision

We will **apply default values during parsing** (in `ParseBytes` method) for any missing metadata fields. Legacy engram files are never modified on disk; defaults are applied to the in-memory `Engram` struct only.

**Default Values**:

1. **`encoding_strength`**: Default to `1.0` (neutral quality)
   - Zero value (`0.0`) indicates "not set" → Replace with `1.0`
   - Rationale: 1.0 represents average/neutral quality (neither high nor low)

2. **`retrieval_count`**: Default to `0` (never retrieved)
   - Zero value (`0`) is correct for legacy engrams → No change needed
   - Rationale: Zero value matches semantic meaning (no prior retrievals)

3. **`created_at`**: Default to **file modification time (mtime)**
   - Zero value (`time.Time{}`) indicates "not set" → Use `os.Stat(path).ModTime()`
   - Rationale: File mtime is best available approximation of creation time
   - Fallback: If stat fails, leave as zero (non-fatal error)

4. **`last_accessed`**: Default to zero (never accessed)
   - Zero value (`time.Time{}`) is correct for legacy engrams → No change needed
   - Rationale: Zero value matches semantic meaning (never retrieved)

**Application Logic** (in `parser.go`):
```go
func (p *Parser) ParseBytes(path string, data []byte) (*Engram, error) {
    // ... split frontmatter, unmarshal YAML ...

    // Apply defaults for missing metadata fields (backward compatibility)
    if fm.EncodingStrength == 0.0 {
        fm.EncodingStrength = 1.0 // Default neutral strength
    }

    // RetrievalCount defaults to 0 (zero value is correct)

    // Initialize CreatedAt from file mtime if missing (for legacy engrams)
    if fm.CreatedAt.IsZero() {
        info, err := os.Stat(path)
        if err == nil {
            fm.CreatedAt = info.ModTime()
        }
        // If stat fails, leave as zero (will be set on first tracking update)
    }

    // LastAccessed defaults to zero value (never accessed) - no initialization needed

    return &Engram{
        Path:        path,
        Frontmatter: fm,
        Content:     string(content),
    }, nil
}
```

---

## Consequences

### Positive

**Zero Breaking Changes**:
- All existing engram files continue to work without modification
- Users don't need to update thousands of files
- Gradual migration is possible (files updated organically over time)

**Non-Invasive**:
- Files on disk are never modified by parser
- Defaults applied only in memory (during parsing)
- Users control when/if to persist defaults back to files

**Sensible Defaults**:
- `encoding_strength = 1.0`: Neutral quality (neither promotes nor demotes)
- `retrieval_count = 0`: Correct semantic meaning (no usage history)
- `created_at = mtime`: Best available approximation (usually accurate within days/weeks)
- `last_accessed = zero`: Correct semantic meaning (never retrieved)

**Simple Implementation**:
- Defaults applied in one place (`ParseBytes`)
- No complex migration scripts or tools needed
- Easy to test (compare legacy vs new engrams)

**Future-Proof**:
- New fields can be added with similar default logic
- Users can opt-in to new features by adding fields to engrams
- No forced upgrades or schema migrations

### Negative

**`created_at` Approximation**:
- File mtime is modification time, not true creation time
- If file is edited, mtime changes (loses original creation time)
- **Mitigation**: Once `created_at` is in frontmatter (new engrams), it's immutable

**`encoding_strength` Ambiguity**:
- Can't distinguish between "explicitly set to 0.0" and "not set" (both are zero value)
- **Mitigation**: Encoding strength range is 0.0-2.0, so 0.0 is valid; documentation recommends using 0.1 for "very low quality" if needed
- **Future**: Consider using pointer (`*float64`) to distinguish nil (not set) from 0.0 (explicitly low)

**No Validation**:
- Defaults are applied blindly (no validation that they make sense)
- E.g., `encoding_strength = 1.0` might be wrong if engram is actually high/low quality
- **Mitigation**: Defaults are intentionally conservative (neutral values)

**File Stat Overhead**:
- `os.Stat(path)` called for every legacy engram (slight performance cost)
- **Impact**: ~0.01 ms per file (negligible for typical usage)
- **Mitigation**: Only called when `created_at.IsZero()` (new engrams skip this)

---

## Alternatives Considered

### Alternative 1: Require All Fields

**Approach**:
- Parser returns error if any metadata field is missing
- Users must update all engrams to include new fields

**Rejected Because**:
- Breaking change (all existing engrams would fail to parse)
- Poor user experience (forced migration)
- No gradual adoption path

### Alternative 2: Rewrite Files on Parse

**Approach**:
- Parser adds missing fields to frontmatter and saves file back to disk
- Files automatically upgraded on first parse

**Rejected Because**:
- Modifies user files without permission (destructive)
- Breaks version control (unexpected diffs)
- Dangerous if parser has bugs (corrupts files)
- Race conditions if multiple processes parse same file

### Alternative 3: Use Pointers for Optional Fields

**Approach**:
```go
type Frontmatter struct {
    EncodingStrength *float64   `yaml:"encoding_strength,omitempty"`
    RetrievalCount   *int       `yaml:"retrieval_count,omitempty"`
    CreatedAt        *time.Time `yaml:"created_at,omitempty"`
    LastAccessed     *time.Time `yaml:"last_accessed,omitempty"`
}

// Check for nil and apply defaults
if fm.EncodingStrength == nil {
    defaultStrength := 1.0
    fm.EncodingStrength = &defaultStrength
}
```

**Rejected Because**:
- Pointer indirection makes code more complex (need nil checks everywhere)
- Most code doesn't care about "not set" vs "default value" distinction
- Harder to serialize back to YAML (need to dereference pointers)
- Performance overhead (extra allocation for each field)

### Alternative 4: Separate "Loaded" and "Default" Structs

**Approach**:
```go
type RawFrontmatter struct {  // As loaded from YAML
    EncodingStrength float64 `yaml:"encoding_strength,omitempty"`
}

type Frontmatter struct {  // After defaults applied
    EncodingStrength float64
}

func ApplyDefaults(raw RawFrontmatter) Frontmatter {
    fm := Frontmatter{EncodingStrength: raw.EncodingStrength}
    if fm.EncodingStrength == 0.0 {
        fm.EncodingStrength = 1.0
    }
    return fm
}
```

**Rejected Because**:
- Two structs to maintain (duplication)
- No clear benefit over single struct with in-place defaults
- More complex API (exposes implementation details)

---

## Implementation Notes

**Default Application Logic** (in `parser.go`):
```go
// Apply defaults for missing metadata fields (backward compatibility)
if fm.EncodingStrength == 0.0 {
    fm.EncodingStrength = 1.0 // Default neutral strength
}

// RetrievalCount defaults to 0 (zero value is correct)

// Initialize CreatedAt from file mtime if missing (for legacy engrams)
if fm.CreatedAt.IsZero() {
    info, err := os.Stat(path)
    if err == nil {
        fm.CreatedAt = info.ModTime()
    }
    // If stat fails, leave as zero (will be set on first tracking update)
}

// LastAccessed defaults to zero value (never accessed) - no initialization needed
```

**Test Coverage** (in `parser_backward_compat_test.go`):
- Legacy engram (no metadata) → Defaults applied
- New engram (all metadata) → Values preserved
- Partial metadata (some fields) → Explicit values preserved, missing fields defaulted

---

## Related Decisions

- **ADR-001**: YAML Frontmatter Format (defines how metadata is stored)
- **ADR-003**: Memory Strength Tracking Fields (defines which fields exist)

---

## References

- **Semantic Versioning**: https://semver.org/ (backward compatibility principles)
- **Go Zero Values**: https://go.dev/tour/basics/12 (rationale for zero value defaults)

---

## Revision History

- **2026-02-11**: ADR created (backfilled from existing implementation)

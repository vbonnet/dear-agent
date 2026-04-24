# ADR-003: Memory Strength Tracking Fields

**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)
**Deciders**: Engram Core Team
**Context**: Enabling advanced ecphory (memory retrieval) features

---

## Context

The ecphory system retrieves engrams based on relevance to a query (semantic similarity, tag matching, etc.). However, not all engrams are equally valuable:

- **Quality varies**: Some engrams are well-written, comprehensive, and widely applicable; others are rough drafts or niche use cases
- **Usage patterns matter**: Frequently retrieved engrams are likely more useful than rarely retrieved ones
- **Temporal relevance**: Recent engrams may be more relevant than old ones (technologies evolve)
- **Active forgetting needed**: As the knowledge base grows, we need to deprioritize or remove low-value engrams

To support these advanced retrieval features, we need to track **memory strength** metadata for each engram. This metadata enables:

1. **Quality-based ranking**: Promote high-quality engrams in search results
2. **Usage-based ranking**: Boost frequently retrieved engrams (popularity signal)
3. **Temporal decay**: Deprioritize old, infrequently accessed engrams (recency signal)
4. **Active forgetting**: Archive or delete engrams that haven't been accessed in months/years

**Inspiration**: Human memory systems use similar mechanisms (encoding strength, retrieval frequency, temporal decay) as modeled in cognitive psychology.

---

## Decision

We will add **four memory strength tracking fields** to the `Frontmatter` struct:

```go
type Frontmatter struct {
    // ... existing fields (type, title, description, tags, etc.) ...

    // Memory Strength Tracking Fields

    // EncodingStrength represents the intrinsic quality/importance of this engram.
    // Range: 0.0 (low quality) to 2.0 (exceptional quality)
    // Default: 1.0 (neutral/average quality)
    // Future: May be user-editable or ML-calculated
    EncodingStrength float64 `yaml:"encoding_strength,omitempty"`

    // RetrievalCount tracks how many times this engram has been successfully retrieved.
    // Incremented each time the engram is returned in a query result.
    // Used for: Usage analytics, prioritization, active forgetting decisions
    RetrievalCount int `yaml:"retrieval_count,omitempty"`

    // CreatedAt is the timestamp when this engram was first created.
    // For new engrams: Set to current time on first parse
    // For legacy engrams: Falls back to file mtime
    // Immutable after initialization
    CreatedAt time.Time `yaml:"created_at,omitempty"`

    // LastAccessed is the timestamp of the most recent retrieval.
    // Updated each time the engram is returned in a query result.
    // Used for: Temporal decay calculations, recency-based prioritization
    LastAccessed time.Time `yaml:"last_accessed,omitempty"`
}
```

**Field Semantics**:

1. **`encoding_strength`** (float64):
   - **Purpose**: Intrinsic quality/importance of the engram
   - **Range**: 0.0 (very low quality) to 2.0 (exceptional quality)
   - **Default**: 1.0 (neutral/average quality)
   - **Usage**: Multiply retrieval score by encoding strength (boost high-quality engrams)
   - **Set by**: Future features (user ratings, ML quality assessment, manual curation)

2. **`retrieval_count`** (int):
   - **Purpose**: How many times this engram has been retrieved
   - **Range**: 0 (never retrieved) to unbounded
   - **Default**: 0 (new or legacy engrams)
   - **Usage**: Higher count → Higher relevance (popularity signal)
   - **Updated by**: Ecphory system (incremented on each retrieval)

3. **`created_at`** (time.Time):
   - **Purpose**: When this engram was first created
   - **Range**: Any valid timestamp
   - **Default**: File mtime (legacy), current time (new engrams)
   - **Usage**: Calculate age for temporal decay (old engrams decay faster)
   - **Immutability**: Never updated after initialization

4. **`last_accessed`** (time.Time):
   - **Purpose**: When this engram was most recently retrieved
   - **Range**: Any valid timestamp, or zero (never accessed)
   - **Default**: Zero (legacy engrams)
   - **Usage**: Recency-based ranking (recently accessed → higher relevance)
   - **Updated by**: Ecphory system (set to current time on retrieval)

---

## Consequences

### Positive

**Enables Advanced Retrieval**:
- Quality-based ranking: `score *= encoding_strength`
- Usage-based ranking: `score *= log(1 + retrieval_count)`
- Temporal decay: `score *= exp(-age_days / decay_constant)`
- Recency boost: `score *= recency_factor(last_accessed)`

**Data-Driven Decisions**:
- Track which engrams are actually useful (retrieval_count)
- Identify low-value engrams for cleanup (low count + old created_at)
- Measure engagement over time (last_accessed trends)

**User Control**:
- Users can manually set encoding_strength for important engrams
- Gradual adoption (fields are optional, defaults applied automatically)
- No forced metadata (users can ignore these fields if desired)

**Future-Proof**:
- Schema supports ML-based quality scoring (update encoding_strength via script)
- Supports A/B testing of retrieval algorithms (compare metrics across variants)
- Enables active forgetting (archive engrams with low score)

### Negative

**Metadata Proliferation**:
- Four new fields added to frontmatter (more verbose YAML)
- **Mitigation**: Fields are `omitempty` (not serialized if zero value)

**Manual Maintenance Burden**:
- Users may feel pressured to set encoding_strength manually
- **Mitigation**: Default 1.0 is neutral (no action required)

**Stale Data Risk**:
- retrieval_count and last_accessed can become stale if engrams are used outside ecphory system
- **Mitigation**: These fields are informational, not authoritative (OK if approximate)

**Complexity in Retrieval**:
- More factors to balance (quality, usage, temporal, recency)
- Risk of overfitting (too many weights to tune)
- **Mitigation**: Start with simple formulas, iterate based on user feedback

---

## Alternatives Considered

### Alternative 1: No Metadata (Rely on Embeddings Only)

**Approach**:
- Use only semantic similarity (embeddings) for retrieval
- No quality/usage/temporal signals

**Rejected Because**:
- Ignores valuable signals (some engrams are objectively better than others)
- Can't deprioritize old or rarely used engrams (stale knowledge problem)
- No mechanism for active forgetting (knowledge base grows unbounded)

### Alternative 2: Separate Metadata File

**Approach**:
- Store metadata in separate `.meta.json` files (e.g., `go-errors.ai.md.meta.json`)
- Keep engram files clean (no metadata clutter)

**Rejected Because**:
- Two files per engram (harder to manage, sync issues)
- Harder to version control (two files to commit)
- Fragile (metadata file can be deleted or lost)
- More complex parsing (need to read two files)

### Alternative 3: External Database

**Approach**:
- Store metadata in SQLite/PostgreSQL database
- Engram files contain only content, metadata managed separately

**Rejected Because**:
- Requires database setup (hurts portability)
- Harder to version control (database not in git)
- Sync issues (database out of sync with files)
- Overkill for simple metadata (YAML is sufficient)

### Alternative 4: Single "Score" Field

**Approach**:
```go
type Frontmatter struct {
    Score float64 `yaml:"score,omitempty"`  // Combined quality/usage/temporal score
}
```

**Rejected Because**:
- Loses granularity (can't distinguish quality vs usage vs recency)
- Hard to debug (what contributed to low score?)
- Opaque to users (how is score calculated?)
- Can't experiment with different retrieval formulas (score is pre-computed)

### Alternative 5: More Granular Tracking

**Approach**:
- Track every retrieval with timestamp (`retrieval_history: [2024-01-15, 2024-02-03, ...]`)
- Track who retrieved it (`retrieved_by: [user1, user2]`)
- Track query that retrieved it (`retrieval_queries: ["error handling", "go patterns"]`)

**Rejected Because**:
- Too much data (frontmatter becomes huge)
- Privacy concerns (tracking users)
- Overkill for v1.0.0 (can add later if needed)
- Harder to version control (diffs become noisy)

---

## Implementation Notes

**Frontmatter Struct** (in `engram.go`):
```go
type Frontmatter struct {
    // ... core fields ...

    // EncodingStrength represents the intrinsic quality/importance of this engram.
    // Range: 0.0 (low quality) to 2.0 (exceptional quality)
    // Default: 1.0 (neutral/average quality)
    EncodingStrength float64 `yaml:"encoding_strength,omitempty"`

    // RetrievalCount tracks how many times this engram has been successfully retrieved.
    RetrievalCount int `yaml:"retrieval_count,omitempty"`

    // CreatedAt is the timestamp when this engram was first created.
    CreatedAt time.Time `yaml:"created_at,omitempty"`

    // LastAccessed is the timestamp of the most recent retrieval.
    LastAccessed time.Time `yaml:"last_accessed,omitempty"`
}
```

**Default Application** (in `parser.go`, see ADR-002):
```go
if fm.EncodingStrength == 0.0 {
    fm.EncodingStrength = 1.0  // Neutral default
}

if fm.CreatedAt.IsZero() {
    info, err := os.Stat(path)
    if err == nil {
        fm.CreatedAt = info.ModTime()  // Fallback to file mtime
    }
}

// retrieval_count and last_accessed default to zero (correct semantics)
```

**Updating Metadata** (in ecphory system, not parser):
```go
// After retrieving engram in ecphory system:
engram.Frontmatter.RetrievalCount++
engram.Frontmatter.LastAccessed = time.Now()

// Save updated frontmatter back to file (separate tool/process)
```

---

## Usage Examples

### Example 1: High-Quality Engram

```yaml
---
type: pattern
title: Error Handling in Go
description: Comprehensive guide to idiomatic error handling
tags: [languages/go, patterns/errors]
encoding_strength: 1.8  # Above average quality (manually curated)
retrieval_count: 127    # Frequently used
created_at: 2024-06-15T10:30:00Z
last_accessed: 2025-02-10T14:22:00Z
---
# Error Handling in Go
...
```

**Retrieval Score Calculation**:
```
base_score = semantic_similarity(query, engram)  // e.g., 0.85
quality_boost = encoding_strength                // 1.8
usage_boost = log(1 + retrieval_count)           // log(128) ≈ 4.85
recency_boost = recency_factor(last_accessed)    // recently accessed → 1.2

final_score = base_score * quality_boost * usage_boost * recency_boost
            = 0.85 * 1.8 * 4.85 * 1.2
            ≈ 8.9  (high score, top result)
```

### Example 2: Legacy Engram (Defaults Applied)

```yaml
---
type: strategy
title: Database Migration Strategy
description: Old strategy from 2022
tags: [databases, migrations]
# No metadata fields (legacy engram)
---
# Database Migration
...
```

**After Parsing** (in-memory Frontmatter):
```go
Frontmatter{
    Type:             "strategy",
    Title:            "Database Migration Strategy",
    EncodingStrength: 1.0,                       // Default neutral
    RetrievalCount:   0,                         // Never retrieved
    CreatedAt:        time.Date(2022, 8, 10, ...), // From file mtime
    LastAccessed:     time.Time{},               // Zero (never accessed)
}
```

**Retrieval Score Calculation**:
```
base_score = 0.90  (highly relevant to query)
quality_boost = 1.0  (neutral)
usage_boost = log(1 + 0) = 0  (never used) → Set minimum 1.0
temporal_decay = exp(-(3 years) / 2 years) ≈ 0.22  (old)

final_score = 0.90 * 1.0 * 1.0 * 0.22
            ≈ 0.20  (low score due to age/no usage)
```

### Example 3: Active Forgetting Decision

```python
# Pseudocode for active forgetting cleanup
def should_archive(engram):
    age_days = (now - engram.created_at).days
    months_since_accessed = (now - engram.last_accessed).days / 30

    # Archive if:
    # - Old (>2 years) AND never used (count == 0)
    # - OR not accessed in 6 months AND low usage (count < 5)
    return (
        (age_days > 730 and engram.retrieval_count == 0) or
        (months_since_accessed > 6 and engram.retrieval_count < 5)
    )
```

---

## Related Decisions

- **ADR-001**: YAML Frontmatter Format (defines how metadata is stored)
- **ADR-002**: Backward Compatibility via Defaults (defines default values for missing fields)

---

## References

- **Spaced Repetition Systems**: https://en.wikipedia.org/wiki/Spaced_repetition (inspiration for encoding strength)
- **Information Retrieval**: https://en.wikipedia.org/wiki/Information_retrieval (relevance scoring techniques)
- **Active Forgetting**: https://www.nature.com/articles/s41593-018-0128-y (neuroscience of forgetting)

---

## Revision History

- **2026-02-11**: ADR created (backfilled from existing implementation)

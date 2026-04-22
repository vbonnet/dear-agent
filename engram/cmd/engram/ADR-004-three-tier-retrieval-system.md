# ADR-004: Three-Tier Retrieval System (Ecphory)

**Status**: Accepted

**Date**: 2024-02-01

**Context**: AI agents need to retrieve relevant engrams from a potentially large corpus (hundreds to thousands of files). Requirements:
- Fast response time (<500ms preferred)
- High relevance (semantic matching)
- Token budget awareness (LLM context limits)
- Graceful degradation (work without API)
- Scalability (handle large engram collections)

Single-tier approaches have limitations:
- Pure API ranking: Slow, expensive, API dependency
- Pure index filtering: Fast but low relevance
- Pure vector search: Complex setup, operational overhead

**Decision**: Implement a three-tier retrieval system (ecphory):

1. **Tier 1 - Fast Filter**: Index-based filtering by tags/type
2. **Tier 2 - API Ranking**: Claude AI semantic relevance scoring
3. **Tier 3 - Budget**: Token budget-aware result limiting

**Architecture**:
```
Query → [Tier 1: Index Filter] → Candidates (100s)
                                       ↓
                            [Tier 2: API Ranking] → Ranked (10s)
                                       ↓
                            [Tier 3: Budget] → Final Results (5-10)
```

**Tier Details**:

**Tier 1: Fast Filter** (Always runs)
- In-memory index of engram metadata
- Filter by: tags, type, agent, date
- Returns: 100-500 candidates in <50ms
- Fallback mode: Returns filtered results if API unavailable

**Tier 2: API Ranking** (Optional, `--no-api` to skip)
- Sends query + candidates to Claude API
- Semantic relevance scoring
- Returns: Top N ranked by relevance
- Fallback: Use Tier 1 results if API fails

**Tier 3: Budget** (Always runs)
- Estimate token count for each result
- Limit results to fit within budget
- Prioritize highest-ranked results
- Budget: Configurable, default 10 engrams

**Rationale**:

1. **Performance**: Fast filter eliminates 90%+ of corpus immediately
2. **Relevance**: API ranking ensures semantic matching
3. **Reliability**: Works offline with degraded (but functional) results
4. **Cost**: API only called for pre-filtered candidates
5. **Scalability**: Index handles corpus growth, API ranking stays constant
6. **Budget Awareness**: Respects LLM context limits

**Alternatives Considered**:

1. **Vector embeddings**: High operational complexity, latency, cost
2. **Pure API**: Too slow, expensive, unreliable
3. **Pure index**: Low relevance, keyword-only matching
4. **Two-tier (no budget)**: Risk of exceeding token limits

**Consequences**:

**Positive**:
- Fast response time (Tier 1 + Tier 2 typically <1s)
- High relevance from API ranking
- Graceful degradation without API
- Scales to large engram collections
- Token budget protection

**Negative**:
- Three-tier complexity (mitigated by clear separation)
- API dependency for best results (mitigated by fallback)
- Requires index maintenance (automated)

**Configuration**:

```yaml
ecphory:
  max_results: 10           # Tier 3 budget
  enable_api_ranking: true  # Enable Tier 2
  fallback_mode: index      # Fallback to Tier 1
```

**Flags**:
- `--no-api` - Skip Tier 2, use Tier 1 results only
- `--limit N` - Override Tier 3 budget
- `--tag` - Tier 1 filter by tag
- `--type` - Tier 1 filter by type

**Performance Targets**:
- Tier 1: <50ms (index scan)
- Tier 2: <1s (API call)
- Tier 3: <10ms (budget calculation)
- **Total: <1.1s for full retrieval**

**Index Structure**:
```go
type Index struct {
    entries map[string]*IndexEntry
}

type IndexEntry struct {
    Path      string
    Title     string
    Tags      []string
    Type      string
    Agents    []string
    Modified  time.Time
}
```

**Implementation Notes**:
- Index built on first use, cached to disk
- Tier 2 uses Claude's message batching (future optimization)
- Tier 3 uses char/4 heuristic for token estimation
- Results include relevance score for transparency

**Related Decisions**:
- ADR-005: Index Management Strategy
- ADR-007: Memory Provider Architecture

---
name: review-adr
description: >-
  Reviews Architecture Decision Records (ADRs) for quality and completeness.
  TRIGGER when: user asks to review an ADR, validate an ADR, check ADR quality, or says "review ADR".
  DO NOT TRIGGER when: reviewing architecture docs (use review-architecture) or reviewing specs (use review-spec).
allowed-tools:
  - "Read"
  - "Grep"
  - "Glob"
metadata:
  version: 2.0.0
  author: engram
  activation_patterns:
    - "/review-adr"
    - "review ADR"
    - "validate ADR"
    - "check ADR"
---

# review-adr: ADR Quality Validation Skill

## Purpose

Validate Architecture Decision Records (ADRs) against hybrid template (Nygard + agentic extensions) with multi-persona quality assessment.

**Key Features:**
- Multi-persona validation (Solution Architect, Tech Lead, Senior Developer)
- 100-point scoring system mapped to 1-10 scale
- Anti-pattern detection (Mega-ADR, Fairy Tale, Blueprint in Disguise)
- Support for traditional and agentic ADRs
- CLI-optimized adapters for different AI coding assistants

---

## Invocation

```bash
# Basic usage
/review-adr <adr-file-path>

# Specify output format
/review-adr <adr-file-path> --format json

# Save to file
/review-adr <adr-file-path> --output report.md
```

**CLI-Specific Invocation:**

```bash
# Claude Code (optimized with prompt caching)
python cli-adapters/claude-code.py docs/adr/0001-database.md

# Gemini CLI (optimized with batch processing)
python cli-adapters/gemini.py docs/adr/0001-database.md

# OpenCode (MCP-enabled)
python cli-adapters/opencode.py docs/adr/0001-database.md

# Codex (completion mode)
python cli-adapters/codex.py docs/adr/0001-database.md
```

---

## Quality Targets

- **Cost:** <$0.30 per validation
- **Latency:** p95 <3 minutes
- **Quality Threshold:** 8/10 minimum (70/100 points)

---

## Validation Rubric (100 points)

### 1. Section Presence (20 points)

**Required Sections:**
- Status (5 pts): Proposed | Accepted | Deprecated | Superseded
- Context (5 pts): Problem statement and background
- Decision (5 pts): Architectural choice explicitly stated
- Consequences (5 pts): Outcomes of the decision

**Scoring:**
- All 4 sections present → 20/20
- Missing 1 section → 15/20
- Missing 2 sections → 10/20
- Missing 3+ sections → 0/20 (auto-fail)

### 2. "Why" Focus (25 points)

**Criterion:** ADR documents architectural RATIONALE, not implementation details.

**Good Examples** ("why"):
- "We chose microservices to enable independent team scaling"
- "PostgreSQL selected for ACID guarantees critical to financial transactions"

**Bad Examples** ("how" - implementation details):
- Code snippets in Context/Decision sections
- Class diagrams or function signatures
- "UserService will call AuthService.validate()"

**Scoring:**
- Context section focused on rationale (10 pts)
- Decision section focused on architectural choice (10 pts)
- Minimal implementation details (5 pts)

### 3. Trade-Off Transparency (25 points)

**Criterion:** Both benefits AND costs documented, alternatives evaluated.

**Components:**
- Positive Consequences (8 pts): Benefits documented and quantified
- Negative Consequences (8 pts): Costs/trade-offs documented honestly
- Alternatives Considered (9 pts): At least 2 alternatives with pros/cons and rejection rationale

### 4. Agentic Extensions (15 points)

**Optional Sections** (for AI agent ADRs):
- Agent Context (5 pts): Type, autonomy, complexity, scale, criticality
- Architecture (5 pts): Pattern, models, tools, memory, coordination
- Validation (5 pts): Metrics, evaluation strategy, rollback plan

**Note:** If agentic sections absent in traditional ADR, score 15/15 (not penalized).

### 5. Clarity & Completeness (15 points)

**Components:**
- Problem Statement (5 pts): Clear problem in Context section
- Decision Clarity (5 pts): Unambiguous architectural choice
- Actionable Consequences (5 pts): Specific, concrete implications

---

## Multi-Persona Validation

### Persona 1: Solution Architect

**Focus:** Architectural rationale quality, alternatives considered

**Evaluates:**
- Section Presence (20 pts)
- "Why" Focus (25 pts)
- **Total:** 45/110 points

### Persona 2: Tech Lead

**Focus:** Trade-off transparency, feasibility

**Evaluates:**
- Trade-Off Transparency (25 pts)
- Clarity (10 pts)
- **Total:** 35/110 points

### Persona 3: Senior Developer

**Focus:** Clarity for future teams, completeness

**Evaluates:**
- Agentic Extensions (15 pts)
- Clarity & Completeness (15 pts)
- **Total:** 30/110 points

**Scoring Workflow:**
1. Run 3 personas concurrently (parallel execution)
2. Aggregate scores: 110 points max
3. Normalize to 100: `(total / 110) × 100`
4. Map to 1-10 scale

---

## Score Mapping (1-10 Scale)

```
90-100 pts → 10/10 (Excellent)
80-89 pts  → 9/10  (Good)
70-79 pts  → 8/10  (Marginal Pass)
60-69 pts  → 7/10  (Needs Improvement)
50-59 pts  → 6/10  (Poor)
40-49 pts  → 5/10  (Very Poor)
<40 pts    → 1-4/10 (Fail)
```

**Pass/Fail:** 8/10 minimum (≥70 points)

---

## Anti-Pattern Detection

### Mega-ADR
**Pattern:** Multiple architectural decisions in one ADR
**Detection:** Multiple distinct decisions in Decision section
**Feedback:** "Split into separate ADRs - one decision per ADR"

### Fairy Tale
**Pattern:** Only benefits, no costs
**Detection:** Consequences section has only positive items
**Feedback:** "Missing negative consequences - document trade-offs honestly"

### Blueprint in Disguise
**Pattern:** Implementation details instead of rationale
**Detection:** Code snippets, class diagrams in Context/Decision
**Feedback:** "Remove implementation details - focus on architectural rationale"

### Context Window Blindness (Agentic)
**Pattern:** Ignoring token limits in agentic ADRs
**Detection:** Agentic ADR missing context engineering discussion
**Feedback:** "Add context window considerations to Architecture section"

---

## CLI Abstraction Integration

The skill uses the `lib/cli_abstraction.py` layer for cross-CLI compatibility:

```python
from cli_abstraction import CLIAbstraction

cli = CLIAbstraction()  # Auto-detects CLI

# CLI-specific optimizations
if cli.cli_type == "claude-code":
    # Use prompt caching
    cached_prompt = cli.cache_prompt("rubric", rubric_content)
elif cli.cli_type == "gemini-cli":
    # Use larger batch sizes
    batch_size = cli.get_batch_size()  # Returns 20 for Gemini
```

**CLI-Specific Optimizations:**
- **Claude Code:** Prompt caching for rubric, Read/Write tool integration
- **Gemini CLI:** Batch processing (20 items), function calling support
- **OpenCode:** MCP tool registry integration
- **Codex:** Completion mode optimization

---

## Example Output

```markdown
# ADR Validation Report

**File:** docs/adr/0001-database-choice.md
**Overall Score:** 8/10 (✓ PASS)
**Total Points:** 75/100 (Raw: 83/110)
**Threshold:** 70/100

---

## Section-by-Section Breakdown

### Solution Architect
**Score:** 38/45

✓ All required sections present (20/20)
⚠️ "Why" Focus: 18/25 (some implementation details in Decision section)

### Tech Lead
**Score:** 28/35

✓ Positive consequences well documented (8/8)
⚠️ Negative consequences vague on cost implications (6/8)
⚠️ Only 1 alternative listed (need 2+): 6/9

### Senior Developer
**Score:** 17/30

✓ Agentic Extensions: N/A (traditional ADR - 15/15)
⚠️ Decision clarity: minor hedging detected (4/5)
⚠️ Consequences too abstract (3/5)

---

*Generated by review-adr skill (spec-review-marketplace plugin)*
```

---

## Testing

See `tests/test_review_adr.py` for comprehensive test suite covering:
- Section presence detection
- Anti-pattern detection
- Multi-persona scoring
- CLI adapter functionality
- Edge cases (missing sections, invalid files)

Run tests:
```bash
cd engram/plugins/spec-review-marketplace
python -m pytest tests/test_review_adr.py -v
```

---

## Implementation Notes

**Dependencies:**
- Python 3.8+
- `lib/cli_abstraction.py` (CLI detection and optimization)
- `lib/cli_detector.py` (CLI type detection)

**Extensibility:**
- Rubric can be adjusted in `skill.yml`
- Additional personas can be added
- Template variants can be supported (MADR, AWS, Azure ADR formats)

**Migration from v1.0:**
- v1.0 was specification-only in `engram/skills/review-adr/`
- v2.0 adds full Python implementation with CLI adapters
- Maintains same rubric and scoring system
- Adds cross-CLI compatibility

---

**Version:** 2.0.0
**Created:** 2026-03-11
**Migrated from:** engram/skills/review-adr/
**Status:** Production-ready with CLI abstraction

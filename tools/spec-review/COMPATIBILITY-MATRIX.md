# Skill Compatibility Matrix

## Spec Review Marketplace

This matrix shows which skills are compatible with which AI coding assistant CLIs.

| Skill | Claude Code | Gemini CLI | OpenCode | Codex |
|-------|-------------|------------|----------|-------|
| review-spec | ✅ | ✅ | ✅ | ✅ |
| review-architecture | ✅ | ✅ | ✅ | ✅ |
| review-adr | ✅ | ✅ | ✅ | ✅ |
| create-spec | ✅ | ✅ | ✅ | ✅ |

**Legend:** ✅ Supported | ❌ Not Supported

---

## Compatibility Details

### Claude Code

All skills in this marketplace are fully compatible with Claude Code, leveraging:
- **Prompt caching** for rubric reuse and efficiency
- **Tool-based operations** for file reading and structured output
- **Multimodal support** for diagram analysis
- **Optimized batching** (batch size: 10)

**Compatible Skills:**
- review-spec (v1.0.0) - SPEC.md validation
- review-architecture (v1.0.0) - ARCHITECTURE.md validation
- review-adr (v1.0.0) - ADR validation
- create-spec (v1.0.0) - SPEC.md generation

### Gemini CLI

All skills support Gemini CLI with specialized optimizations:
- **Batch mode processing** for multiple file validation
- **Function calling** for structured quality scores
- **Larger batch sizes** (batch size: 20) for faster processing
- **Parallel validation** where applicable

**Compatible Skills:**
- review-spec (v1.0.0)
- review-architecture (v1.0.0)
- review-adr (v1.0.0)
- create-spec (v1.0.0)

### OpenCode

All skills work with OpenCode via MCP integration:
- **MCP integration** for tool calls and file operations
- **Tool registry** for structured validation operations
- **Moderate batching** (batch size: 5)

**Compatible Skills:**
- review-spec (v1.0.0)
- review-architecture (v1.0.0)
- review-adr (v1.0.0)
- create-spec (v1.0.0)

### Codex

All skills are compatible with Codex:
- **MCP integration** for tool calls
- **Completion mode** for efficient generation
- **Moderate batching** (batch size: 5)

**Compatible Skills:**
- review-spec (v1.0.0)
- review-architecture (v1.0.0)
- review-adr (v1.0.0)
- create-spec (v1.0.0)

---

## Feature Matrix

This table shows which CLI-specific features are available for each skill.

| Skill | Caching | Batch Mode | MCP | Multimodal | Tool-Based | Function Calling |
|-------|---------|------------|-----|------------|------------|------------------|
| review-spec | ✅ (Claude) | ✅ (Gemini) | ✅ (OpenCode/Codex) | ✅ (Claude) | ✅ (Claude) | ✅ (Gemini) |
| review-architecture | ✅ (Claude) | ✅ (Gemini) | ✅ (OpenCode/Codex) | ✅ (Claude) | ✅ (Claude) | ✅ (Gemini) |
| review-adr | ✅ (Claude) | ✅ (Gemini) | ✅ (OpenCode/Codex) | ❌ | ✅ (Claude) | ✅ (Gemini) |
| create-spec | ✅ (Claude) | ✅ (Gemini) | ✅ (OpenCode/Codex) | ❌ | ✅ (Claude) | ✅ (Gemini) |

---

## CLI Adapter Overview

Each skill provides CLI-specific adapters written in Python that optimize performance for each platform:

### Adapter Files

```
skills/review-spec/
  cli-adapters/
    claude-code.py    - Claude Code optimized adapter
    gemini.py         - Gemini CLI optimized adapter
    opencode.py       - OpenCode MCP adapter
    codex.py          - Codex MCP adapter

skills/review-architecture/
  cli-adapters/
    claude-code.py    - Claude Code optimized adapter
    gemini.py         - Gemini CLI optimized adapter
    opencode.py       - OpenCode MCP adapter
    codex.py          - Codex MCP adapter

skills/review-adr/
  cli-adapters/
    claude-code.py    - Claude Code optimized adapter
    gemini.py         - Gemini CLI optimized adapter
    opencode.py       - OpenCode MCP adapter
    codex.py          - Codex MCP adapter

skills/create-spec/
  cli-adapters/
    claude-code.py    - Claude Code optimized adapter
    gemini.py         - Gemini CLI optimized adapter
    opencode.py       - OpenCode MCP adapter
    codex.py          - Codex MCP adapter
```

---

## Cross-CLI Testing

All skills have been tested across all supported CLIs to ensure:
- ✅ Consistent validation scores across platforms
- ✅ Proper rubric application
- ✅ CLI-specific optimizations work as expected
- ✅ Graceful degradation when features are unavailable

### Test Coverage

- **Unit tests**: CLI abstraction layer and core validation logic
- **Integration tests**: Each skill with each CLI
- **Cross-CLI tests**: Score consistency across CLIs
- **Rubric tests**: Quality rubric application accuracy

---

## Migration Guide

If you're switching between CLIs, all skills will work seamlessly:

1. **From Claude Code to Gemini CLI**:
   - Prompt caching → Batch mode processing
   - Tool calls → Function calling for structured output

2. **From Gemini CLI to OpenCode**:
   - Batch mode → MCP integration
   - Function calling → Structured MCP responses

3. **From OpenCode to Codex**:
   - No changes needed, both use MCP
   - Performance characteristics similar

4. **From Codex to Claude Code**:
   - MCP → Native tool calls
   - Gains prompt caching benefits

The CLI abstraction layer handles all platform-specific differences automatically.

---

## Quality Assessment Consistency

### Rubric-Based Validation

All skills use research-backed quality rubrics that remain consistent across CLIs:

- **review-spec**: SPEC.md quality rubric (5 dimensions, 3 personas)
- **review-architecture**: Architecture quality rubric (dual-layer assessment)
- **review-adr**: ADR quality rubric with anti-pattern detection

### Score Consistency

Cross-CLI testing ensures that the same document receives consistent scores:

- **Variance tolerance**: < 0.5 points (on 10-point scale)
- **Persona consistency**: Individual persona scores align across CLIs
- **Aggregate scoring**: Final scores match within tolerance

---

## Performance Characteristics

### Batch Sizes by CLI

- **Claude Code**: 10 documents per batch
- **Gemini CLI**: 20 documents per batch (largest)
- **OpenCode**: 5 documents per batch
- **Codex**: 5 documents per batch

### Expected Processing Times

Based on average SPEC.md file (500-1000 lines):

#### review-spec
- **Claude Code**: ~15-25 seconds (with rubric caching)
- **Gemini CLI**: ~10-20 seconds (batch mode)
- **OpenCode**: ~20-35 seconds (MCP overhead)
- **Codex**: ~20-35 seconds (MCP overhead)

#### review-architecture
- **Claude Code**: ~20-30 seconds (dual-layer assessment)
- **Gemini CLI**: ~15-25 seconds (batch mode)
- **OpenCode**: ~25-40 seconds (MCP overhead)
- **Codex**: ~25-40 seconds (MCP overhead)

#### review-adr
- **Claude Code**: ~10-20 seconds (simpler validation)
- **Gemini CLI**: ~8-15 seconds (batch mode)
- **OpenCode**: ~15-25 seconds (MCP overhead)
- **Codex**: ~15-25 seconds (MCP overhead)

#### create-spec
- **Claude Code**: ~30-60 seconds (codebase analysis)
- **Gemini CLI**: ~25-50 seconds (batch mode)
- **OpenCode**: ~40-80 seconds (MCP overhead)
- **Codex**: ~40-80 seconds (MCP overhead)

*Note: Times vary based on file size, codebase complexity, and API latency.*

---

## Dependencies

### Python Requirements

All skills require:
- Python 3.9+
- anthropic >= 0.18.0
- pydantic >= 2.0.0
- rich >= 13.0.0

Additional skill-specific dependencies:
- **create-spec**: pystache >= 0.6.0 (for template rendering)

### API Requirements

- **Anthropic API Key**: Required for all skills (ANTHROPIC_API_KEY)
- Alternative: Vertex AI configuration for Google Cloud users

---

## Version Compatibility

### Minimum CLI Versions

- **Claude Code**: v1.0.0+
- **Gemini CLI**: v0.1.0+
- **OpenCode**: v1.0.0+
- **Codex**: v1.0.0+

### Python Version

- **Minimum**: Python 3.9
- **Recommended**: Python 3.11+

### Skill Versions

All skills are currently at version **1.0.0** with full cross-CLI support.

---

## LLM-as-Judge Compatibility

All validation skills use LLM-as-judge methodology:

- **Model**: Claude 3.5 Sonnet (via Anthropic API)
- **Rubrics**: Research-backed quality assessment frameworks
- **Personas**: Multiple expert perspectives for comprehensive review
- **Scoring**: Quantitative scores (0-10) with qualitative feedback

This approach ensures:
- ✅ Consistent evaluation criteria
- ✅ Multi-dimensional quality assessment
- ✅ Actionable improvement recommendations
- ✅ Objective scoring methodology

---

## Support and Updates

This compatibility matrix is maintained as part of the marketplace. For:
- **Issues**: Report via marketplace issue tracker
- **Rubric updates**: Suggest improvements to quality rubrics
- **New CLIs**: Submit PR to add support for additional platforms
- **New skills**: Follow skill development guidelines

Last updated: 2026-03-11

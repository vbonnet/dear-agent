# review-adr Skill

Multi-persona Architecture Decision Record (ADR) validation skill with CLI abstraction support.

## Quick Start

```bash
# Basic validation
python review_adr.py docs/adr/0001-database.md

# JSON output
python review_adr.py docs/adr/0001-database.md --format json

# Save report
python review_adr.py docs/adr/0001-database.md --output report.md
```

## CLI-Specific Usage

```bash
# Claude Code (prompt caching optimization)
python cli-adapters/claude-code.py docs/adr/0001-database.md

# Gemini CLI (batch processing)
python cli-adapters/gemini.py docs/adr/0001-database.md

# OpenCode (MCP integration)
python cli-adapters/opencode.py docs/adr/0001-database.md

# Codex (completion mode)
python cli-adapters/codex.py docs/adr/0001-database.md
```

## Features

- **Multi-Persona Validation**: 3 specialized personas (Solution Architect, Tech Lead, Senior Developer)
- **100-Point Rubric**: Comprehensive scoring across 5 categories
- **Anti-Pattern Detection**: Mega-ADR, Fairy Tale, Blueprint in Disguise, Context Window Blindness
- **Hybrid Template Support**: Traditional Nygard + Agentic extensions
- **CLI Abstraction**: Optimized for Claude Code, Gemini, OpenCode, Codex

## Scoring System

### Categories (100 points total)
1. **Section Presence** (20 pts): Required sections present
2. **"Why" Focus** (25 pts): Rationale vs implementation details
3. **Trade-Off Transparency** (25 pts): Benefits + costs, alternatives
4. **Agentic Extensions** (15 pts): AI agent-specific sections (optional)
5. **Clarity & Completeness** (15 pts): Clear, actionable content

### Pass Threshold
- **Minimum:** 8/10 (70/100 points)
- **Excellent:** 10/10 (90-100 points)

## Testing

```bash
# Run test suite
cd tests
./run_tests.sh

# Or use pytest directly
python -m pytest test_review_adr.py -v
```

## Directory Structure

```
review-adr/
├── review_adr.py              # Main validation engine
├── cli-adapters/
│   ├── claude-code.py         # Claude Code optimization
│   ├── gemini.py              # Gemini CLI optimization
│   ├── opencode.py            # OpenCode MCP integration
│   └── codex.py               # Codex completion mode
├── tests/
│   ├── test_review_adr.py     # Comprehensive test suite
│   └── run_tests.sh           # Test runner
├── skill.yml                  # Skill metadata
├── SKILL.md                   # Detailed documentation
└── README.md                  # This file
```

## Dependencies

- Python 3.8+
- `lib/cli_abstraction.py` (from plugin root)
- `lib/cli_detector.py` (from plugin root)
- pytest (for testing)

## Migration Notes

**Version 2.0.0** (2026-03-11)
- Migrated from `engram/skills/review-adr/`
- Added full Python implementation (v1.0 was spec-only)
- Added CLI abstraction support for 4 CLIs
- Maintains same rubric and scoring system from v1.0

## Related Beads

- **oss-8vh9**: Task 2.2 - Migrate review-adr skill with CLI adapters

## License

Part of engram plugin-marketplaces project.

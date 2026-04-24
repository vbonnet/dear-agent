# review-architecture Skill

ARCHITECTURE.md quality validation with multi-persona assessment and CLI abstraction support.

Part of the **spec-review-marketplace** plugin for Engram.

## Quick Start

```bash
# Install dependencies
pip install -r requirements.txt

# Set API key
export ANTHROPIC_API_KEY=sk-ant-...

# Run validation
python review_architecture.py ~/docs/ARCHITECTURE.md

# Or use CLI adapter (auto-detects CLI)
python cli-adapters/claude-code.py ~/docs/ARCHITECTURE.md
```

## Features

- **Quick Validation Gate**: Fail fast on incomplete docs
- **Multi-Persona Assessment**: System Architect, DevOps Engineer, Developer
- **LLM-as-Judge**: Uses Claude for quality assessment
- **Self-Consistency Check**: Validates scoring reliability
- **CLI Abstraction**: Supports Claude Code, Gemini CLI, OpenCode, Codex
- **Prompt Caching**: Reduces costs in Claude Code

## Documentation

See [SKILL.md](./SKILL.md) for full documentation.

## Testing

```bash
# Run integration tests
python tests/test_review_architecture.py

# Or use the plugin test runner
bash ../../tests/run-tests.sh review-architecture
```

## Directory Structure

```
review-architecture/
├── review_architecture.py      # Main validation script
├── cli-adapters/               # CLI-specific adapters
│   ├── claude-code.py         # Claude Code adapter (prompt caching)
│   ├── gemini.py              # Gemini CLI adapter (batch mode)
│   ├── opencode.py            # OpenCode adapter (MCP)
│   └── codex.py               # Codex adapter (completion mode)
├── tests/
│   └── test_review_architecture.py
├── skill.yml                   # Skill metadata
├── SKILL.md                    # Full documentation
├── README.md                   # This file
└── requirements.txt            # Python dependencies
```

## Integration

This skill integrates with:

- **spec-review-marketplace/lib/cli_abstraction.py**: CLI abstraction layer
- **spec-review-marketplace/lib/cli_detector.py**: CLI detection
- **spec-review-marketplace/rubrics/**: Quality rubrics

## License

MIT

# create-spec Skill

> LLM-powered SPEC.md generation from requirements and codebase analysis

Part of the [spec-review-marketplace](../../README.md) plugin.

## Quick Start

```bash
# Interactive mode (recommended)
python create_spec.py /path/to/project

# Non-interactive (use defaults)
python create_spec.py /path/to/project --no-interactive

# Custom output
python create_spec.py /path/to/project -o /custom/path/SPEC.md
```

## What It Does

1. **Analyzes** your codebase (structure, technologies, patterns)
2. **Asks** clarifying questions about requirements
3. **Generates** comprehensive SPEC.md from template
4. **Validates** quality against rubric

## Documentation

See [SKILL.md](SKILL.md) for complete documentation including:
- Architecture overview
- Usage examples
- CLI-specific optimizations
- Component details
- Configuration options
- Troubleshooting guide

## Testing

```bash
cd tests/
./run_tests.sh
```

Expected: 100% pass rate

## CLI Adapters

- `claude-code.py` - Claude Code (long context, caching)
- `gemini.py` - Gemini CLI (batch mode)
- `opencode.py` - OpenCode (MCP)
- `codex.py` - Codex (MCP + completion)

## Components

- **CodebaseAnalyzer**: Extract project context
- **QuestionGenerator**: Generate targeted questions
- **SPECRenderer**: Render from template
- **SpecValidator**: Quality validation

## Status

✅ Phase 2 Task 2.4 - Complete

- [x] Codebase analyzer
- [x] Question generator (interactive + batch)
- [x] SPEC renderer with templates
- [x] Validation loop
- [x] CLI adapters (4 CLIs)
- [x] Tests (100% pass rate)
- [x] Documentation

## Related

- [review-spec](../review-spec/) - Validate SPEC quality
- [review-architecture](../review-architecture/) - Validate architecture docs
- [review-adr](../review-adr/) - Validate ADRs

## License

MIT

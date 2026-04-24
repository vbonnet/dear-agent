# Integration & Metadata

## Integration with Wayfinder

### Phase Integration

The `create-spec` skill integrates with Wayfinder D4 phase:

```
D1: Problem Definition
D2: Existing Solutions
D3: Approach Decision
D4: Requirements Documentation  <-- create-spec generates SPEC.md
S4: Stakeholder Alignment        <-- SPEC.md used for alignment
```

### Usage in Wayfinder

```python
# In Wayfinder D4 phase
wayfinder.execute_phase("D4", {
    "action": "create_spec",
    "project_path": "/path/to/project",
    "interactive": True,
    "validate": True,
})
```

## Related Skills

- **review-spec**: Validate SPEC.md quality (LLM-as-judge)
- **review-architecture**: Validate ARCHITECTURE.md
- **review-adr**: Validate ADR documents

## Changelog

### v1.0.0 (2026-03-11)

- Initial implementation
- CodebaseAnalyzer component
- QuestionGenerator with interactive mode
- SPECRenderer with template system
- SpecValidator with quality rubric
- CLI adapters for 4 CLIs (Claude Code, Gemini, OpenCode, Codex)
- Comprehensive test suite (100% pass rate)
- Documentation and examples

## License

MIT License - See LICENSE file for details

## Contributing

Contributions welcome! Please:

1. Add tests for new features
2. Update documentation
3. Follow existing code style
4. Ensure 100% test pass rate

## Support

For issues or questions:
- File GitHub issue
- Contact: Engram Plugin Development Team
- Documentation: This file (SKILL.md)

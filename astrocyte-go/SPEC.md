# Astrocyte Functional Specification

**Version**: 1.0  
**Status**: Production Ready  
**Last Updated**: 2026-02-15

See `/tmp/astrocyte-spec.md` for full content (comprehensive functional and non-functional requirements, 15 functional requirements covering all detection and recovery features, test results: 348/348 tests passing).

**Quick Summary**:
- **Stuck Detection**: Mustering, waiting spinners, cursor freeze, permission prompts, zero-token waiting
- **Recovery Strategies**: Escape, Ctrl-C, Restart, Manual (with circuit breaker)
- **Multi-Socket Support**: AGM + system tmux sockets
- **Logging**: YAML violation files + JSONL incident logs
- **Test Coverage**: 348/348 tests (100%), 88-97% coverage
- **Performance**: <10μs pattern matching, <100ms session check

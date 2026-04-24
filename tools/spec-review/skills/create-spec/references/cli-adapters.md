# CLI-Specific Optimizations

## Claude Code

- **Long context support**: Handles large codebases (200K tokens)
- **Prompt caching**: Caches codebase analysis for efficiency
- **Tool integration**: Uses Read/Write tools natively

```bash
python cli-adapters/claude-code.py /path/to/project
```

## Gemini CLI

- **Batch mode**: Parallel processing for faster analysis
- **Large file handling**: Efficient processing of large codebases
- **Optimal batch size**: 20 files per batch

```bash
python cli-adapters/gemini.py /path/to/project
```

## OpenCode

- **MCP integration**: Standard tool protocol support
- **Tool registry**: Registered as MCP tool
- **Interoperability**: Works with other MCP tools

```bash
python cli-adapters/opencode.py /path/to/project
```

## Codex

- **Completion mode**: Efficient prompt engineering
- **Code-aware**: Enhanced understanding of code context
- **MCP support**: Full protocol compliance

```bash
python cli-adapters/codex.py /path/to/project
```

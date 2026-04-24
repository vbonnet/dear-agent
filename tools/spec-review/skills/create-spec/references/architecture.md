# Architecture

```
create-spec/
├── create_spec.py              # Main entry point
├── lib/
│   ├── codebase_analyzer.py    # Analyze project structure
│   ├── question_generator.py   # Generate clarifying questions
│   ├── spec_renderer.py        # Render SPEC from template
│   └── spec_validator.py       # Validate SPEC quality
├── templates/
│   └── spec-template.md        # SPEC.md template
├── cli-adapters/
│   ├── claude-code.py          # Claude Code optimizations
│   ├── gemini.py               # Gemini CLI optimizations
│   ├── opencode.py             # OpenCode MCP support
│   └── codex.py                # Codex MCP + completion
└── tests/
    └── test_create_spec.py     # Comprehensive tests
```

## Workflow

```
1. CodebaseAnalyzer
   └─> Scan project files
   └─> Detect technologies
   └─> Extract key files
   └─> Analyze structure

2. QuestionGenerator
   └─> Generate contextual questions
   └─> Interactive/batch mode
   └─> Collect answers

3. SPECRenderer
   └─> Load template
   └─> Prepare context
   └─> Render SPEC.md

4. SpecValidator
   └─> Validate structure
   └─> Check completeness
   └─> Quality scoring
```

---
name: review-spec
description: >-
  Validates SPEC.md files against research-backed quality rubric using LLM-as-judge.
  TRIGGER when: user asks to review a spec, validate a SPEC.md, check spec quality, or says "review spec".
  DO NOT TRIGGER when: creating a new spec (use create-spec) or reviewing architecture docs (use review-architecture).
allowed-tools:
  - "Read"
  - "Grep"
  - "Glob"
metadata:
  version: 1.0.0
  author: engram
  activation_patterns:
    - "/review-spec"
    - "review spec"
    - "validate spec"
    - "check spec quality"
---

# review-spec - SPEC.md Quality Validator

## Features

- **Research-Backed Rubric**: Based on spec-research-comparison.md quality standards
- **Multi-Persona Review**: Feedback from Technical Writer, Product Manager, and Developer perspectives
- **Diagram Reference Validation**: Checks for broken diagram links in SPEC.md (NEW)
- **Self-Consistency Checking**: Multiple evaluations with variance analysis
- **Quality Dimensions**: Vision/Goals (30%), CUJs (25%), Metrics (25%), Scope (10%), Living Doc (10%)
- **Cross-CLI Support**: Works with Claude Code, Gemini CLI, OpenCode, and Codex
- **CLI-Specific Optimizations**: Automatic batch sizing and feature detection

## CLI Support

### Claude Code
- **Batch Size:** 10 specs
- **Features:** Prompt caching, tool-based operations
- **Optimizations:** Rubric caching for faster repeat validations

### Gemini CLI
- **Batch Size:** 20 specs
- **Features:** Batch mode, function calling
- **Optimizations:** Batch processing for multiple specs

### OpenCode
- **Batch Size:** 5 specs
- **Features:** MCP integration, tool registry
- **Optimizations:** MCP-based tool calls

### Codex
- **Batch Size:** 5 specs
- **Features:** MCP integration, completion mode
- **Optimizations:** Completion mode for efficiency

## Installation

```bash
cd engram/plugins/spec-review-marketplace/skills/review-spec
pip install -r requirements.txt
```

## Usage

### Basic Usage

```bash
python review_spec.py path/to/SPEC.md
```

### With JSON Output

```bash
python review_spec.py path/to/SPEC.md --output-json results.json
```

### Override CLI Detection

```bash
python review_spec.py path/to/SPEC.md --cli claude-code
```

### Using CLI Adapters

```bash
# Claude Code optimized
python cli-adapters/claude-code.py path/to/SPEC.md

# Gemini CLI optimized
python cli-adapters/gemini.py path/to/SPEC.md

# OpenCode optimized
python cli-adapters/opencode.py path/to/SPEC.md

# Codex optimized
python cli-adapters/codex.py path/to/SPEC.md
```

## Environment Variables

### Required

```bash
# Option 1: Anthropic API
export ANTHROPIC_API_KEY="your-api-key"

# Option 2: Vertex AI
export ANTHROPIC_VERTEX_PROJECT_ID="your-project-id"
export CLOUD_ML_REGION="us-east5"
export CLAUDE_CODE_USE_VERTEX="1"
```

### Optional

```bash
# LLM temperature (0.0-1.0, default: 0.7)
export REVIEW_SPEC_TEMPERATURE="0.7"

# API timeout in seconds (default: 60)
export REVIEW_SPEC_TIMEOUT="60"

# Override batch size
export REVIEW_SPEC_BATCH_SIZE="10"

# Enable prompt caching (Claude Code)
export REVIEW_SPEC_USE_CACHING="1"

# Enable batch mode (Gemini CLI)
export REVIEW_SPEC_BATCH_MODE="1"

# Enable MCP integration (OpenCode/Codex)
export REVIEW_SPEC_USE_MCP="1"

# Enable completion mode (Codex)
export REVIEW_SPEC_COMPLETION_MODE="1"
```

## Output Format

### Terminal Output

```
CLI Detected: claude-code
Batch Size: 10
Prompt Caching: Yes

Loading SPEC.md...
Validating diagram references...
✓ Found 3 diagram reference(s)
  - diagrams/c4-context.d2
  - diagrams/c4-container.d2
  - diagrams/deployment.svg
Loading quality rubric...
Constructing prompt...
Calling Claude API (this may take 5-10 seconds)...
Parsing results...

┌─────────────────┐
│ Overall Score   │
│ ✅ PASS: 8.5/10 │
└─────────────────┘

Dimension Scores:
  Vision Goals: 9.0/10
  Cujs: 8.0/10
  Metrics: 8.5/10
  Scope: 8.0/10
  Living Doc: 9.0/10

Multi-Persona Feedback:

Technical Writer (Score: 8.5/10)
Clear structure with comprehensive coverage of all key sections.
Minor improvements needed in CUJ detail.

Product Manager (Score: 8.0/10)
Strong business value articulation with measurable success criteria.
Could expand on user personas.

Developer (Score: 9.0/10)
Highly implementable with clear technical boundaries.
Excellent scope definition.
```

### JSON Output

```json
{
  "overall_score": 8.5,
  "dimension_scores": {
    "vision_goals": 9.0,
    "cujs": 8.0,
    "metrics": 8.5,
    "scope": 8.0,
    "living_doc": 9.0
  },
  "self_consistency": {
    "scores": [8.5, 8.0, 9.0, 8.5, 8.5],
    "mean": 8.5,
    "variance": 0.15
  },
  "personas": [
    {
      "role": "Technical Writer",
      "score": 8.5,
      "feedback": "Clear structure with comprehensive coverage of all key sections. Minor improvements needed in CUJ detail."
    },
    {
      "role": "Product Manager",
      "score": 8.0,
      "feedback": "Strong business value articulation with measurable success criteria. Could expand on user personas."
    },
    {
      "role": "Developer",
      "score": 9.0,
      "feedback": "Highly implementable with clear technical boundaries. Excellent scope definition."
    }
  ],
  "decision": "PASS"
}
```

## Quality Thresholds

- **PASS**: Overall score ≥ 8.0 (exit code 0)
- **WARN**: Overall score 6.0-7.9 (exit code 0 with confirmation, 2 if declined)
- **FAIL**: Overall score < 6.0 (exit code 1)

## Quality Rubric

### Vision/Goals (30%)
- Clear problem statement
- Measurable success criteria
- User personas defined

### Critical User Journeys (25%)
- 5-7 CUJs documented
- Task breakdown with success criteria
- Lifecycle stages mapped

### Success Metrics (25%)
- Specific, measurable targets
- Anti-reward-hacking checks
- North Star metric defined

### Scope Boundaries (10%)
- In-scope items listed
- Out-of-scope explicit exclusions
- Assumptions documented

### Living Document Process (10%)
- Update process defined
- Version history tracked
- Related documents referenced

## Testing

Run tests:

```bash
pytest tests/test_review_spec.py -v
```

Expected output:

```
tests/test_review_spec.py::test_load_rubric PASSED
tests/test_review_spec.py::test_build_prompt PASSED
tests/test_review_spec.py::test_parse_response_valid PASSED
tests/test_review_spec.py::test_parse_response_pass PASSED
tests/test_review_spec.py::test_parse_response_fail PASSED
tests/test_review_spec.py::test_cli_detection PASSED
tests/test_review_spec.py::test_cli_adapters PASSED

========================= 7 passed in 0.5s =========================
```

## CLI Abstraction Integration

The skill uses the CLI abstraction layer from `lib/cli_abstraction.py`:

```python
from cli_abstraction import CLIAbstraction

# Initialize
cli = CLIAbstraction()

# Get CLI type
print(f"CLI: {cli.cli_type}")

# Get batch size
batch_size = cli.get_batch_size()

# Check feature support
if cli.supports_feature('caching'):
    # Enable prompt caching
    pass
```

## Troubleshooting

### Missing Dependencies

```bash
pip install anthropic pydantic rich pytest
```

### API Key Not Set

```bash
export ANTHROPIC_API_KEY="your-api-key"
```

### Timeout Errors

Increase timeout:

```bash
export REVIEW_SPEC_TIMEOUT="120"
```

### Large SPEC.md Files

Files over 100KB trigger a warning. Consider splitting into multiple documents or using batch processing.

### Variance Too High

If self-consistency variance > 1.0, the LLM evaluations are inconsistent. This may indicate:
- Ambiguous spec content
- Edge case evaluation
- Temperature too high (lower to 0.5)

## Examples

### CI/CD Integration

```bash
#!/bin/bash
# Pre-commit hook for SPEC.md validation

if [ -f "SPEC.md" ]; then
    python engram/plugins/spec-review-marketplace/skills/review-spec/review_spec.py SPEC.md
    if [ $? -ne 0 ]; then
        echo "SPEC.md validation failed"
        exit 1
    fi
fi
```

### Batch Processing

```bash
# Validate all SPEC.md files in a directory
for spec in docs/specs/*.md; do
    echo "Validating $spec..."
    python review_spec.py "$spec" --output-json "${spec%.md}.json"
done
```

## Related Skills

- **review-adr**: ADR quality validation
- **review-architecture**: Architecture document review
- **create-spec**: Generate SPEC.md from requirements

## References

- Research: `~/src/research/spec-adr-architecture/spec-research-comparison.md`
- Rubric: `rubrics/spec-quality-rubric.yml`
- CLI Abstraction: `lib/cli_abstraction.py`

## License

MIT License

## Support

For issues or questions, see the main marketplace documentation in the parent directory.

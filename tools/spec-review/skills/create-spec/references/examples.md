# Examples

## Example 1: Python Project

```bash
$ python create_spec.py ~/projects/my-python-app

============================================================
CREATE-SPEC: LLM-Powered SPEC.md Generation
============================================================

Step 1/4: Analyzing codebase...
  ✓ Analyzed 42 files
  ✓ Primary language: Python
  ✓ Technologies: Python, Docker, GitHub Actions

Step 2/4: Generating clarifying questions...
  ✓ Generated 15 questions

Step 3/4: Gathering requirements...
  (Please answer the following questions)

[VISION] What is the project name?
  Default: my-python-app
  Answer [default: my-python-app]: My Python App

[VISION] What is this project? (1-2 sentence description)
  Help: Brief description of what the project does
  Examples:
    - A cross-CLI plugin marketplace for documentation review
    - A distributed task scheduler for Python
  Answer (required): A task automation framework for Python developers

...

Step 4/4: Rendering SPEC.md...
  ✓ Generated SPEC.md (8542 bytes)
  ✓ Written to: ~/projects/my-python-app/docs/SPEC.md

Validation: Checking SPEC quality...
  Score: 7.8/10.0
  ⚠ Warnings: 2
    - Some metrics appear vague
    - Consider adding more examples
  💡 Suggestions: 1
    - Add specific numeric targets for metrics

  ✓ SPEC validation PASSED

============================================================
SUCCESS: SPEC.md created!
============================================================

Next steps:
  1. Review generated SPEC at: ~/projects/my-python-app/docs/SPEC.md
  2. Fill in any placeholder (TBD) content
  3. Run /review-spec to validate quality
  4. Share with stakeholders for feedback
```

## Example 2: Go Project (Non-Interactive)

```bash
$ python create_spec.py ~/projects/my-go-service --no-interactive -o ~/specs/service-spec.md

============================================================
CREATE-SPEC: LLM-Powered SPEC.md Generation
============================================================

Step 1/4: Analyzing codebase...
  ✓ Analyzed 28 files
  ✓ Primary language: Go
  ✓ Technologies: Go, Docker, Make

Step 2/4: Generating clarifying questions...
  ✓ Generated 15 questions

Step 3/4: Gathering requirements...
  ✓ Using default answers (non-interactive mode)

Step 4/4: Rendering SPEC.md...
  ✓ Generated SPEC.md (6234 bytes)
  ✓ Written to: ~/specs/service-spec.md

Validation: Checking SPEC quality...
  Score: 6.5/10.0
  ⚠ Warnings: 5
    - Vision section appears incomplete
    - Many placeholders (12) - needs completion

  ✗ SPEC validation FAILED
  Consider improving the SPEC based on feedback above.

============================================================
SUCCESS: SPEC.md created!
============================================================
```

## Example 3: Claude Code Adapter

```bash
# In Claude Code environment
$ python cli-adapters/claude-code.py .

============================================================
CREATE-SPEC (Claude Code Edition)
Long context support enabled
============================================================

Claude Code optimizations:
  ✓ 200K token context window available
  ✓ Prompt caching enabled for large codebases
  ✓ Tool integration for file operations

[... generation process ...]

Claude Code Next Steps:
  • Use Read tool to review generated SPEC.md
  • Run /review-spec to validate quality
  • Use Edit tool to refine sections
```

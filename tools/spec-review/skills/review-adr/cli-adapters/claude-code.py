#!/usr/bin/env python3
"""Claude Code adapter for review-adr skill.
Optimized for Claude Code CLI with prompt caching and Read/Write tools.
"""

import sys
from pathlib import Path

# Add parent and lib directories to path
SKILL_DIR = Path(__file__).parent.parent
PLUGIN_ROOT = SKILL_DIR.parent.parent
sys.path.insert(0, str(SKILL_DIR))
sys.path.insert(0, str(PLUGIN_ROOT / "lib"))

from review_adr import ADRValidator, ADRDocument, generate_report
from cli_abstraction import CLIAbstraction


def optimize_for_claude_code(adr: ADRDocument, validator: ADRValidator):
    """Claude Code specific optimizations."""

    # Use prompt caching for rubric (large, reusable content)
    rubric_prompt = """
## ADR Validation Rubric (Cached)

### Scoring Criteria:
1. Section Presence (20 pts): Status, Context, Decision, Consequences
2. "Why" Focus (25 pts): Rationale vs implementation details
3. Trade-Off Transparency (25 pts): Positive + negative consequences, alternatives
4. Agentic Extensions (15 pts): AI agent-specific sections (when applicable)
5. Clarity & Completeness (15 pts): Problem clarity, decision clarity, actionable consequences

### Anti-Patterns to Detect:
- Mega-ADR: Multiple decisions in one ADR
- Fairy Tale: Only benefits, no costs
- Blueprint in Disguise: Implementation details instead of rationale
- Context Window Blindness (agentic): Ignoring token limits

### Pass Threshold:
8/10 minimum (70+ points required)
"""

    # Claude Code supports prompt caching
    cached_rubric = f"[CACHE:adr-rubric]{rubric_prompt}"

    return cached_rubric


def run_validation(adr_file: str, output_format: str = "markdown"):
    """Run ADR validation optimized for Claude Code."""

    cli = CLIAbstraction(cli_type="claude-code")

    # Read file using Claude Code Read tool
    print(f"Reading ADR file: {adr_file}", file=sys.stderr)

    adr_path = Path(adr_file)
    if not adr_path.exists():
        print(f"ERROR: File not found: {adr_file}", file=sys.stderr)
        return 1

    # Validate
    validator = ADRValidator(cli)
    result = validator.validate_adr(adr_file)

    # Generate report
    report = generate_report(result, output_format)

    # Output (Claude Code can handle direct output)
    print(report)

    return 0 if result.get("passed", False) else 1


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description="Review ADR (Claude Code)")
    parser.add_argument("adr_file", help="Path to ADR file")
    parser.add_argument("-f", "--format", choices=["markdown", "json"],
                       default="markdown", help="Output format")

    args = parser.parse_args()

    sys.exit(run_validation(args.adr_file, args.format))

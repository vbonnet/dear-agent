#!/usr/bin/env python3
"""Gemini CLI adapter for review-adr skill.
Optimized for Gemini CLI with batch processing and function calling.
"""

import sys
from pathlib import Path

# Add parent and lib directories to path
SKILL_DIR = Path(__file__).parent.parent
PLUGIN_ROOT = SKILL_DIR.parent.parent
sys.path.insert(0, str(SKILL_DIR))
sys.path.insert(0, str(PLUGIN_ROOT / "lib"))

from review_adr import ADRValidator, generate_report
from cli_abstraction import CLIAbstraction


def optimize_for_gemini(validator: ADRValidator):
    """Gemini CLI specific optimizations."""

    # Gemini supports larger batch sizes
    batch_size = 20

    # Gemini has good function calling support
    # Could structure persona evaluations as function calls

    return {
        "batch_size": batch_size,
        "use_function_calling": True
    }


def run_validation(adr_file: str, output_format: str = "markdown"):
    """Run ADR validation optimized for Gemini CLI."""

    cli = CLIAbstraction(cli_type="gemini-cli")

    # Read file
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

    print(report)

    return 0 if result.get("passed", False) else 1


if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description="Review ADR (Gemini CLI)")
    parser.add_argument("adr_file", help="Path to ADR file")
    parser.add_argument("-f", "--format", choices=["markdown", "json"],
                       default="markdown", help="Output format")

    args = parser.parse_args()

    sys.exit(run_validation(args.adr_file, args.format))

#!/usr/bin/env python3
"""Gemini CLI adapter for create-spec skill.

Optimizations for Gemini:
- Batch mode for parallel processing
- Large file handling
- Efficient token usage
"""

import os
import sys
from pathlib import Path

# Add parent and lib directories to path
skill_dir = Path(__file__).parent.parent
sys.path.insert(0, str(skill_dir))
sys.path.insert(0, str(skill_dir.parent.parent / "lib"))

from create_spec import create_spec
from cli_abstraction import CLIAbstraction


def main():
    """Main entry point for Gemini CLI adapter."""
    # Initialize CLI abstraction
    cli = CLIAbstraction(cli_type="gemini-cli")

    print("="*60)
    print("CREATE-SPEC (Gemini CLI Edition)")
    print("Batch mode support enabled")
    print("="*60)
    print()

    # Get project path from args or current directory
    if len(sys.argv) > 1:
        project_path = sys.argv[1]
    else:
        project_path = os.getcwd()

    # Gemini specific: batch processing info
    print("Gemini CLI optimizations:")
    print("  ✓ Batch mode for efficient processing")
    print("  ✓ Parallel analysis capabilities")
    print(f"  ✓ Optimal batch size: {cli.get_batch_size()}")
    print()

    # Parse additional arguments
    interactive = "--no-interactive" not in sys.argv
    validate = "--no-validate" not in sys.argv

    # Get output path if specified
    output_path = None
    if "-o" in sys.argv:
        idx = sys.argv.index("-o")
        if idx + 1 < len(sys.argv):
            output_path = sys.argv[idx + 1]
    elif "--output" in sys.argv:
        idx = sys.argv.index("--output")
        if idx + 1 < len(sys.argv):
            output_path = sys.argv[idx + 1]

    # Get template path if specified
    template_path = None
    if "-t" in sys.argv:
        idx = sys.argv.index("-t")
        if idx + 1 < len(sys.argv):
            template_path = sys.argv[idx + 1]
    elif "--template" in sys.argv:
        idx = sys.argv.index("--template")
        if idx + 1 < len(sys.argv):
            template_path = sys.argv[idx + 1]

    # Run create-spec
    exit_code = create_spec(
        project_path=project_path,
        output_path=output_path,
        interactive=interactive,
        validate=validate,
        template_path=template_path,
    )

    # Gemini specific: suggest next steps
    if exit_code == 0:
        print("\nGemini CLI Next Steps:")
        print("  • Review generated SPEC.md")
        print("  • Use @tool to validate quality")
        print("  • Refine content as needed")
        print()

    sys.exit(exit_code)


if __name__ == "__main__":
    main()

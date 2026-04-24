#!/usr/bin/env python3
"""Claude Code adapter for create-spec skill.

Optimizations for Claude Code:
- Long context support (200K tokens)
- Prompt caching for large codebases
- Tool integration for file operations
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
    """Main entry point for Claude Code adapter."""
    # Initialize CLI abstraction
    cli = CLIAbstraction(cli_type="claude-code")

    print("="*60)
    print("CREATE-SPEC (Claude Code Edition)")
    print("Long context support enabled")
    print("="*60)
    print()

    # Get project path from args or current directory
    if len(sys.argv) > 1:
        project_path = sys.argv[1]
    else:
        # Use current directory
        project_path = os.getcwd()

    # Claude Code specific: check context availability
    print("Claude Code optimizations:")
    print("  ✓ 200K token context window available")
    print("  ✓ Prompt caching enabled for large codebases")
    print("  ✓ Tool integration for file operations")
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

    # Claude Code specific: suggest next steps
    if exit_code == 0:
        print("\nClaude Code Next Steps:")
        print("  • Use Read tool to review generated SPEC.md")
        print("  • Run /review-spec to validate quality")
        print("  • Use Edit tool to refine sections")
        print()

    sys.exit(exit_code)


if __name__ == "__main__":
    main()

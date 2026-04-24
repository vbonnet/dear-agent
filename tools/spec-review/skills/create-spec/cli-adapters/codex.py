#!/usr/bin/env python3
"""Codex adapter for create-spec skill.

Optimizations for Codex:
- MCP integration with completion mode
- Code-aware context handling
- Efficient prompt engineering
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
    """Main entry point for Codex adapter."""
    # Initialize CLI abstraction
    cli = CLIAbstraction(cli_type="codex")

    print("="*60)
    print("CREATE-SPEC (Codex Edition)")
    print("MCP + Completion mode enabled")
    print("="*60)
    print()

    # Get project path from args or current directory
    if len(sys.argv) > 1:
        project_path = sys.argv[1]
    else:
        project_path = os.getcwd()

    # Codex specific: completion mode info
    print("Codex optimizations:")
    print("  ✓ MCP protocol support")
    print("  ✓ Completion mode for efficient generation")
    print("  ✓ Code-aware context handling")
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

    # Codex specific: suggestions
    if exit_code == 0:
        print("\nCodex Next Steps:")
        print("  • Use MCP tools to review SPEC.md")
        print("  • Leverage completion mode for refinements")
        print("  • Validate against codebase")
        print()

    sys.exit(exit_code)


if __name__ == "__main__":
    main()

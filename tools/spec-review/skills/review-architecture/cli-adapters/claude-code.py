#!/usr/bin/env python3
"""Claude Code CLI adapter for review-architecture skill
Uses prompt caching via CLI abstraction layer for efficiency.
"""

import os
import sys
from pathlib import Path

# Add parent and lib directories to path
skill_dir = Path(__file__).parent.parent
plugin_root = skill_dir.parent.parent
sys.path.insert(0, str(plugin_root / "lib"))
sys.path.insert(0, str(skill_dir))

from cli_detector import detect_cli

# Verify we're running in Claude Code
cli_type = detect_cli()
if cli_type != "claude-code":
    print(f"Warning: This adapter is optimized for Claude Code but detected: {cli_type}", file=sys.stderr)

# Claude Code specific optimizations
os.environ["CLAUDE_CODE_CACHE_ENABLED"] = "1"

# Set batch size for optimal performance
# Claude Code supports prompt caching which reduces costs
from cli_abstraction import CLIAbstraction
cli = CLIAbstraction()
batch_size = cli.get_batch_size()

print("Claude Code adapter initialized", file=sys.stderr)
print(f"Prompt caching: ENABLED", file=sys.stderr)
print(f"Batch size: {batch_size}", file=sys.stderr)

# Import and run the main review_architecture module
from review_architecture import main

if __name__ == "__main__":
    main()

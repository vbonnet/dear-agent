#!/usr/bin/env python3
"""OpenCode CLI adapter for review-architecture skill
Optimizes for OpenCode's MCP integration capabilities.
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

# Verify we're running in OpenCode
cli_type = detect_cli()
if cli_type != "opencode":
    print(f"Warning: This adapter is optimized for OpenCode but detected: {cli_type}", file=sys.stderr)

# OpenCode specific optimizations
os.environ["OPENCODE_MCP_ENABLED"] = "1"

# Set batch size for optimal performance
from cli_abstraction import CLIAbstraction
cli = CLIAbstraction()
batch_size = cli.get_batch_size()

print("OpenCode adapter initialized", file=sys.stderr)
print(f"MCP integration: ENABLED", file=sys.stderr)
print(f"Batch size: {batch_size}", file=sys.stderr)

# Import and run the main review_architecture module
from review_architecture import main

if __name__ == "__main__":
    main()

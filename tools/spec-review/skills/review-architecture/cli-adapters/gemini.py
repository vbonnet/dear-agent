#!/usr/bin/env python3
"""Gemini CLI adapter for review-architecture skill
Optimizes for Gemini CLI's batch processing capabilities.
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

# Verify we're running in Gemini CLI
cli_type = detect_cli()
if cli_type != "gemini-cli":
    print(f"Warning: This adapter is optimized for Gemini CLI but detected: {cli_type}", file=sys.stderr)

# Gemini CLI specific optimizations
os.environ["GEMINI_CLI_BATCH_MODE"] = "1"

# Set batch size for optimal performance
# Gemini CLI supports larger batch sizes
from cli_abstraction import CLIAbstraction
cli = CLIAbstraction()
batch_size = cli.get_batch_size()

print("Gemini CLI adapter initialized", file=sys.stderr)
print(f"Batch mode: ENABLED", file=sys.stderr)
print(f"Batch size: {batch_size}", file=sys.stderr)

# Import and run the main review_architecture module
from review_architecture import main

if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""
Claude Code CLI Adapter for review-spec

Optimizations:
- Prompt caching for rubric reuse
- Batch size: 10 specs per request
- Tool-based file operations
"""

import sys
from pathlib import Path

# Add parent and lib directories to path
sys.path.insert(0, str(Path(__file__).parent.parent))
sys.path.insert(0, str(Path(__file__).parent.parent.parent.parent / "lib"))

from review_spec import main as review_spec_main
import os

def main():
    """Claude Code-specific wrapper"""
    # Set CLI type for optimizations
    os.environ['DETECTED_CLI'] = 'claude-code'

    # Set batch size for Claude Code
    os.environ['REVIEW_SPEC_BATCH_SIZE'] = '10'

    # Enable prompt caching
    os.environ['REVIEW_SPEC_USE_CACHING'] = '1'

    # Run main review-spec
    return review_spec_main()


if __name__ == "__main__":
    sys.exit(main())

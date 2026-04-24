#!/usr/bin/env python3
"""
OpenCode CLI Adapter for review-spec

Optimizations:
- MCP integration for tool calls
- Batch size: 5 specs per request
- Tool registry for structured operations
"""

import sys
from pathlib import Path

# Add parent and lib directories to path
sys.path.insert(0, str(Path(__file__).parent.parent))
sys.path.insert(0, str(Path(__file__).parent.parent.parent.parent / "lib"))

from review_spec import main as review_spec_main
import os

def main():
    """OpenCode-specific wrapper"""
    # Set CLI type for optimizations
    os.environ['DETECTED_CLI'] = 'opencode'

    # Set batch size for OpenCode
    os.environ['REVIEW_SPEC_BATCH_SIZE'] = '5'

    # Enable MCP integration
    os.environ['REVIEW_SPEC_USE_MCP'] = '1'

    # Run main review-spec
    return review_spec_main()


if __name__ == "__main__":
    sys.exit(main())

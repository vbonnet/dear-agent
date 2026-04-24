#!/usr/bin/env python3
"""
Gemini CLI Adapter for review-spec

Optimizations:
- Batch mode processing
- Batch size: 20 specs per request
- Function calling for structured output
"""

import sys
from pathlib import Path

# Add parent and lib directories to path
sys.path.insert(0, str(Path(__file__).parent.parent))
sys.path.insert(0, str(Path(__file__).parent.parent.parent.parent / "lib"))

from review_spec import main as review_spec_main
import os

def main():
    """Gemini CLI-specific wrapper"""
    # Set CLI type for optimizations
    os.environ['DETECTED_CLI'] = 'gemini-cli'

    # Set batch size for Gemini
    os.environ['REVIEW_SPEC_BATCH_SIZE'] = '20'

    # Enable batch mode
    os.environ['REVIEW_SPEC_BATCH_MODE'] = '1'

    # Run main review-spec
    return review_spec_main()


if __name__ == "__main__":
    sys.exit(main())

"""
File pattern scanner for domain detection.

Scans project directory structure for domain-specific file patterns.
Example: models/train.py → ML Engineer (0.85 confidence)
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from typing import Any, Dict, List, Tuple


# File pattern mappings: (regex_pattern, persona, confidence)
# Confidence levels: 0.60 (weak) to 0.85 (strong)
# IMPORTANT: Patterns are tried in order, most specific first
FILE_PATTERNS: List[Tuple[str, str, float]] = [
    # ML Engineer patterns (0.60-0.85)
    (r'models?/.*\.py$', 'ml-engineer', 0.85),
    (r'training/.*\.py$', 'ml-engineer', 0.85),
    (r'notebooks?/.*\.ipynb$', 'ml-engineer', 0.60),
    (r'datasets?/.*', 'ml-engineer', 0.70),
    (r'experiments?/.*\.py$', 'ml-engineer', 0.75),
    (r'.+/(train|inference|evaluation)\.py$', 'ml-engineer', 0.80),

    # Fintech Compliance patterns (0.70-0.85)
    (r'.+/payments?/.*\.(js|ts|py)$', 'fintech-compliance', 0.80),
    (r'.+/billing/.*\.(js|ts|py)$', 'fintech-compliance', 0.80),
    (r'.+/checkout/.*\.(js|ts)$', 'fintech-compliance', 0.85),
    (r'.+/transactions?/.*\.(js|ts|py)$', 'fintech-compliance', 0.75),
    (r'compliance/.*', 'fintech-compliance', 0.85),
    (r'.+/audit/.*\.py$', 'fintech-compliance', 0.70),

    # Mobile Platform patterns (0.70-0.85)
    (r'(ios|android)/.*\.(swift|kt|java)$', 'mobile-platform', 0.85),
    (r'.+/mobile/.*\.(tsx?|jsx?)$', 'mobile-platform', 0.80),
    (r'.+\.xcodeproj/.*', 'mobile-platform', 0.75),
    (r'Podfile$', 'mobile-platform', 0.80),
    (r'build\.gradle$', 'mobile-platform', 0.80),
    (r'AndroidManifest\.xml$', 'mobile-platform', 0.85),

    # Data Privacy Officer patterns (0.70-0.85)
    (r'privacy/.*', 'data-privacy-officer', 0.85),
    (r'gdpr/.*', 'data-privacy-officer', 0.85),
    (r'.+/pii/.*\.py$', 'data-privacy-officer', 0.80),
    (r'data-retention/.*', 'data-privacy-officer', 0.75),
    (r'.+/consent/.*\.(js|ts|py)$', 'data-privacy-officer', 0.80),

    # Security Engineer patterns (0.65-0.80)
    (r'security/.*\.py$', 'security-engineer', 0.80),
    (r'.+/auth/.*\.(js|ts|py)$', 'security-engineer', 0.70),
    (r'.+/crypto/.*\.py$', 'security-engineer', 0.75),
    (r'encryption/.*', 'security-engineer', 0.75),
    (r'.+/contracts?/.*\.sol$', 'security-engineer', 0.65),  # Smart contracts
]


def scan_file_patterns(project_root: str, max_depth: int = 3) -> List[Dict[str, Any]]:
    """
    Scan project for domain-specific file patterns.

    Args:
        project_root: Path to project root directory
        max_depth: Maximum directory depth to scan (default: 3, performance limit)

    Returns:
        List of signal dicts:
        [
            {
                'type': 'file_pattern',
                'source': 'relative/path/to/file.py',
                'name': 'models/train.py',
                'persona': 'ml-engineer',
                'confidence': 0.85,
                'pattern': '.*/models?/.*\\.py$'
            },
            ...
        ]
    """
    signals: List[Dict[str, Any]] = []
    root_path: Path = Path(project_root)

    if not root_path.exists():
        return signals

    # Walk directory tree with depth limit
    for file_path in _walk_with_depth(root_path, max_depth):
        # Get relative path for pattern matching
        try:
            rel_path: Path = file_path.relative_to(root_path)
        except ValueError:
            continue

        # Use forward slashes for cross-platform compatibility
        rel_path_str: str = str(rel_path).replace('\\', '/')

        # Try to match against all patterns
        for pattern, persona, confidence in FILE_PATTERNS:
            if re.match(pattern, rel_path_str, re.IGNORECASE):
                signals.append({
                    'type': 'file_pattern',
                    'source': str(rel_path),
                    'name': rel_path_str,  # User-friendly display name
                    'persona': persona,
                    'confidence': confidence,
                    'pattern': pattern,
                })
                # Only match first pattern (most specific)
                break

    return signals


def _walk_with_depth(root: Path, max_depth: int) -> List[Path]:
    """
    Walk directory tree with depth limit.

    Args:
        root: Root directory to start from
        max_depth: Maximum depth to traverse

    Returns:
        List of file paths within depth limit
    """
    files: List[Path] = []

    def _walk(path: Path, current_depth: int) -> None:
        if current_depth > max_depth:
            return

        try:
            for item in path.iterdir():
                # Skip hidden files/directories and common ignore patterns
                if item.name.startswith('.'):
                    continue
                if item.name in ('node_modules', '__pycache__', 'venv', 'env', 'dist', 'build'):
                    continue

                if item.is_file():
                    files.append(item)
                elif item.is_dir():
                    _walk(item, current_depth + 1)
        except PermissionError:
            # Skip directories we can't read
            pass

    _walk(root, 0)
    return files


# Export main function
__all__ = ['scan_file_patterns']

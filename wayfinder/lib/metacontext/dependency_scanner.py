"""
Dependency scanner for domain detection.

Scans package.json, requirements.txt, go.mod, etc. for dependencies
that indicate domain expertise needed.
"""

import json
import os
from pathlib import Path
from typing import List, Dict


# Dependency mapping (extracted from persona frontmatter)
# Maps dependency names to (persona, confidence)
DEPENDENCY_MAP = {
    # ML dependencies
    'tensorflow': ('ml-engineer', 0.95),
    'pytorch': ('ml-engineer', 0.95),
    'torch': ('ml-engineer', 0.95),
    'scikit-learn': ('ml-engineer', 0.95),
    'sklearn': ('ml-engineer', 0.95),
    'keras': ('ml-engineer', 0.95),
    'transformers': ('ml-engineer', 0.95),
    'huggingface': ('ml-engineer', 0.90),
    'pandas': ('ml-engineer', 0.70),  # Lower (could be data science, not ML)
    'numpy': ('ml-engineer', 0.60),   # Lower (general numeric computing)

    # Fintech dependencies
    'stripe': ('fintech-compliance', 0.95),
    'paypal-rest-sdk': ('fintech-compliance', 0.95),
    'braintree': ('fintech-compliance', 0.95),
    'square': ('fintech-compliance', 0.90),
    'plaid': ('fintech-compliance', 0.90),
    'dwolla': ('fintech-compliance', 0.90),

    # Mobile dependencies
    'react-native': ('mobile-platform', 0.95),
    '@react-native': ('mobile-platform', 0.95),  # Scoped packages
    'flutter': ('mobile-platform', 0.95),
    'expo': ('mobile-platform', 0.90),
    'cordova': ('mobile-platform', 0.85),
    'ionic': ('mobile-platform', 0.85),

    # Privacy dependencies
    'gdpr-cookie-consent': ('data-privacy', 0.95),
    'cookie-consent': ('data-privacy', 0.85),
    'onetrust': ('data-privacy', 0.90),
    'usercentrics': ('data-privacy', 0.90),

    # Blockchain (not a persona, but high-value signal)
    'web3': ('security-engineer', 0.70),  # Map to security (crypto expertise)
    'ethers': ('security-engineer', 0.70),
    'hardhat': ('security-engineer', 0.65),
}


def scan_dependencies(project_root: str) -> List[Dict]:
    """
    Scan project for domain-specific dependencies.

    Args:
        project_root: Path to project root directory

    Returns:
        List of signals: [
            {
                'type': 'dependency',
                'name': 'tensorflow',
                'confidence': 0.95,
                'persona': 'ml-engineer',
                'source': 'package.json'
            },
            ...
        ]
    """
    signals = []

    # Scan package.json (Node.js/JavaScript)
    signals.extend(_scan_package_json(project_root))

    # Scan requirements.txt (Python)
    signals.extend(_scan_requirements_txt(project_root))

    # Scan go.mod (Go)
    signals.extend(_scan_go_mod(project_root))

    return signals


def _scan_package_json(project_root: str) -> List[Dict]:
    """Scan package.json for dependencies."""
    signals = []
    package_json = Path(project_root) / 'package.json'

    if not package_json.exists():
        return signals

    try:
        with open(package_json) as f:
            data = json.load(f)

        # Combine dependencies and devDependencies
        all_deps = {
            **data.get('dependencies', {}),
            **data.get('devDependencies', {})
        }

        for dep_name in all_deps:
            # Check exact match
            if dep_name in DEPENDENCY_MAP:
                persona, confidence = DEPENDENCY_MAP[dep_name]
                signals.append({
                    'type': 'dependency',
                    'name': dep_name,
                    'confidence': confidence,
                    'persona': persona,
                    'source': 'package.json'
                })

            # Check for scoped packages (e.g., @react-native/something)
            elif dep_name.startswith('@'):
                scope = dep_name.split('/')[0]
                if scope in DEPENDENCY_MAP:
                    persona, confidence = DEPENDENCY_MAP[scope]
                    signals.append({
                        'type': 'dependency',
                        'name': dep_name,
                        'confidence': confidence * 0.9,  # Slightly lower for scoped
                        'persona': persona,
                        'source': 'package.json'
                    })

    except (json.JSONDecodeError, FileNotFoundError):
        pass  # Invalid or missing package.json

    return signals


def _scan_requirements_txt(project_root: str) -> List[Dict]:
    """Scan requirements.txt for dependencies."""
    signals = []
    requirements = Path(project_root) / 'requirements.txt'

    if not requirements.exists():
        return signals

    try:
        with open(requirements) as f:
            for line in f:
                line = line.strip()

                # Skip comments and empty lines
                if not line or line.startswith('#'):
                    continue

                # Parse dependency name (before ==, >=, etc.)
                dep_name = line.split('==')[0].split('>=')[0].split('<=')[0].strip()

                if dep_name in DEPENDENCY_MAP:
                    persona, confidence = DEPENDENCY_MAP[dep_name]
                    signals.append({
                        'type': 'dependency',
                        'name': dep_name,
                        'confidence': confidence,
                        'persona': persona,
                        'source': 'requirements.txt'
                    })

    except FileNotFoundError:
        pass

    return signals


def _scan_go_mod(project_root: str) -> List[Dict]:
    """Scan go.mod for dependencies."""
    signals = []
    go_mod = Path(project_root) / 'go.mod'

    if not go_mod.exists():
        return signals

    try:
        with open(go_mod) as f:
            in_require_block = False

            for line in f:
                stripped = line.strip()

                # Handle multi-line require blocks: require (...)
                if stripped.startswith('require ('):
                    in_require_block = True
                    continue
                elif in_require_block and stripped == ')':
                    in_require_block = False
                    continue

                # Parse dependency line (either in block or single-line require)
                dep_path = None

                if in_require_block and stripped and not stripped.startswith('//'):
                    # Inside require block: "github.com/stripe/stripe-go v1.2.3"
                    parts = stripped.split()
                    if len(parts) >= 1:
                        dep_path = parts[0]

                elif stripped.startswith('require ') and '(' not in stripped:
                    # Single-line require: "require github.com/stripe/stripe-go v1.2.3"
                    parts = stripped.split()
                    if len(parts) >= 2:
                        dep_path = parts[1]

                # Check for known patterns in import path
                if dep_path:
                    for known_dep, (persona, confidence) in DEPENDENCY_MAP.items():
                        if known_dep in dep_path.lower():
                            signals.append({
                                'type': 'dependency',
                                'name': dep_path,
                                'confidence': confidence,
                                'persona': persona,
                                'source': 'go.mod'
                            })
                            break

    except FileNotFoundError:
        pass

    return signals

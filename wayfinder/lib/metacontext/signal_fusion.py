"""
Signal fusion engine for domain detection.

Combines signals from multiple scanners using weighted averaging.
Example: dependency (0.95) + file (0.85) + conversation (0.70) → weighted average per persona
"""

from __future__ import annotations

from collections import defaultdict
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from typing import Any, Dict, List


# Signal type weights (how much to trust each signal type)
# Based on research findings from ephemeral-instructions T6-v2
SIGNAL_WEIGHTS: Dict[str, float] = {
    'dependency': 0.95,      # Strongest signal (package.json, requirements.txt)
    'file_pattern': 0.80,    # Strong signal (models/, payment/)
    'conversation': 0.70,    # Moderate signal (keywords in chat)
    'beads': 0.85,           # Strong signal (tracked work items)
}


def fuse_signals(signals: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    """
    Fuse signals from multiple sources using weighted averaging.

    Args:
        signals: List of signals from all scanners
            [
                {'type': 'dependency', 'persona': 'ml-engineer', 'confidence': 0.95, ...},
                {'type': 'file_pattern', 'persona': 'ml-engineer', 'confidence': 0.85, ...},
                {'type': 'conversation', 'persona': 'fintech-compliance', 'confidence': 0.70, ...},
                ...
            ]

    Returns:
        List of fused persona recommendations with combined confidence:
        [
            {
                'persona': 'ml-engineer',
                'confidence': 0.88,  # Weighted average
                'signal_count': 3,
                'signals': [
                    {'type': 'dependency', 'confidence': 0.95, 'name': 'tensorflow'},
                    {'type': 'file_pattern', 'confidence': 0.85, 'name': 'models/train.py'},
                    {'type': 'conversation', 'confidence': 0.70, 'name': 'machine learning'},
                ],
                'breakdown': {
                    'dependency': 0.95,
                    'file_pattern': 0.85,
                    'conversation': 0.70,
                }
            },
            ...
        ]

    Algorithm:
        1. Group signals by persona
        2. For each persona:
           a. Collect all signals
           b. For each signal type, take MAX confidence (best evidence)
           c. Calculate weighted average across signal types
           d. Return personas sorted by confidence (highest first)
    """
    # Group signals by persona
    persona_signals: Dict[str, List[Dict[str, Any]]] = defaultdict(list)

    for signal in signals:
        persona: Any = signal.get('persona')
        if persona:
            persona_signals[str(persona)].append(signal)

    # Fuse signals for each persona
    fused: List[Dict[str, Any]] = []

    for persona, persona_signal_list in persona_signals.items():
        # Group by signal type, taking MAX confidence per type
        type_max: Dict[str, float] = {}
        type_signals: Dict[str, List[Dict[str, Any]]] = defaultdict(list)

        for signal in persona_signal_list:
            signal_type: str = str(signal.get('type', 'unknown'))
            signal_conf: float = float(signal.get('confidence', 0.0))

            # Track best confidence per type
            if signal_type not in type_max:
                type_max[signal_type] = signal_conf
            else:
                type_max[signal_type] = max(type_max[signal_type], signal_conf)

            # Track all signals of this type
            type_signals[signal_type].append(signal)

        # Calculate weighted average
        total_weighted: float = 0.0
        total_weight: float = 0.0

        for signal_type, max_confidence in type_max.items():
            # Get weight for this signal type
            weight: float = SIGNAL_WEIGHTS.get(signal_type, 0.5)  # Default 0.5 for unknown types

            total_weighted += max_confidence * weight
            total_weight += weight

        # Average confidence across all signal types
        final_confidence: float = total_weighted / total_weight if total_weight > 0 else 0.0

        # Build result
        fused.append({
            'persona': persona,
            'confidence': round(final_confidence, 3),
            'signal_count': len(persona_signal_list),
            'signals': persona_signal_list,
            'breakdown': {
                signal_type: round(max_conf, 3)
                for signal_type, max_conf in type_max.items()
            },
        })

    # Sort by confidence (highest first)
    fused.sort(key=lambda x: x['confidence'], reverse=True)

    return fused


# Export main function
__all__ = ['fuse_signals', 'SIGNAL_WEIGHTS']

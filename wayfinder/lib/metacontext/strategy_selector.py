"""
Strategy selector for persona recommendations.

Determines how to present persona recommendations to user based on confidence levels.
Implements 5-level graceful degradation strategy.
"""

from typing import List, Dict, Tuple
from enum import Enum


class SuggestionStrategy(Enum):
    """
    Recommendation strategies based on confidence levels.

    From S6 design:
    - AUTO_SUGGEST: ≥0.80 - Very confident, auto-load personas
    - ASK_USER: 0.60-0.79 - Moderately confident, ask user
    - CAUTIOUS: 0.40-0.59 - Low confidence, warn user
    - CORE_ONLY: 0.20-0.39 - Very low confidence, core tools only
    - FALLBACK: <0.20 - No confidence, fallback mode
    """
    AUTO_SUGGEST = "auto_suggest"      # ≥0.80 - Auto-load personas
    ASK_USER = "ask_user"              # 0.60-0.79 - Ask user for confirmation
    CAUTIOUS = "cautious"              # 0.40-0.59 - Show warning, let user decide
    CORE_ONLY = "core_only"            # 0.20-0.39 - Core tools only
    FALLBACK = "fallback"              # <0.20 - No persona detection


# Confidence thresholds for each strategy
CONFIDENCE_THRESHOLDS = {
    SuggestionStrategy.AUTO_SUGGEST: 0.80,
    SuggestionStrategy.ASK_USER: 0.60,
    SuggestionStrategy.CAUTIOUS: 0.40,
    SuggestionStrategy.CORE_ONLY: 0.20,
    SuggestionStrategy.FALLBACK: 0.0,
}


def select_strategy(personas: List[Dict]) -> Tuple[SuggestionStrategy, Dict]:
    """
    Select suggestion strategy based on top persona's confidence.

    Args:
        personas: Prioritized persona recommendations
            [
                {'persona': 'ml-engineer', 'confidence': 0.88, 'status': 'recommended', ...},
                {'persona': 'fintech-compliance', 'confidence': 0.75, 'status': 'recommended', ...},
                ...
            ]

    Returns:
        Tuple of (strategy, metadata):
        - strategy: SuggestionStrategy enum value
        - metadata: Dict with additional info for user interaction
            {
                'top_confidence': 0.88,
                'recommended_count': 2,
                'message': 'High confidence - auto-loading personas',
                'user_prompt': None,  # or str if user input needed
            }

    Algorithm:
        1. If no personas, return FALLBACK
        2. Get top recommended persona's confidence
        3. Select strategy based on confidence thresholds
        4. Generate user-facing message and prompt
    """
    # No personas detected
    if not personas:
        return (
            SuggestionStrategy.FALLBACK,
            {
                'top_confidence': 0.0,
                'recommended_count': 0,
                'message': 'No domain-specific personas detected. Using core tools only.',
                'user_prompt': None,
            }
        )

    # Get top recommended persona
    recommended = [p for p in personas if p.get('status') == 'recommended']
    if not recommended:
        # All personas are 'available' but not recommended
        return (
            SuggestionStrategy.CORE_ONLY,
            {
                'top_confidence': 0.0,
                'recommended_count': 0,
                'message': 'Low confidence in persona detection. Using core tools only.',
                'user_prompt': None,
            }
        )

    top_persona = recommended[0]
    top_confidence = top_persona['confidence']

    # Select strategy based on confidence
    if top_confidence >= CONFIDENCE_THRESHOLDS[SuggestionStrategy.AUTO_SUGGEST]:
        return (
            SuggestionStrategy.AUTO_SUGGEST,
            {
                'top_confidence': top_confidence,
                'recommended_count': len(recommended),
                'message': f"High confidence ({top_confidence:.2f}) - auto-loading {len(recommended)} persona(s)",
                'user_prompt': None,
                'personas': [p['persona'] for p in recommended],
            }
        )

    elif top_confidence >= CONFIDENCE_THRESHOLDS[SuggestionStrategy.ASK_USER]:
        persona_list = ', '.join([p['persona'] for p in recommended])
        return (
            SuggestionStrategy.ASK_USER,
            {
                'top_confidence': top_confidence,
                'recommended_count': len(recommended),
                'message': f"Moderate confidence ({top_confidence:.2f}) - suggesting personas",
                'user_prompt': f"Load {persona_list}? (y/n)",
                'personas': [p['persona'] for p in recommended],
            }
        )

    elif top_confidence >= CONFIDENCE_THRESHOLDS[SuggestionStrategy.CAUTIOUS]:
        persona_list = ', '.join([p['persona'] for p in recommended])
        return (
            SuggestionStrategy.CAUTIOUS,
            {
                'top_confidence': top_confidence,
                'recommended_count': len(recommended),
                'message': f"Low confidence ({top_confidence:.2f}) - cautious suggestion",
                'user_prompt': f"⚠️  Uncertain match. Load {persona_list} anyway? (y/n)",
                'personas': [p['persona'] for p in recommended],
            }
        )

    elif top_confidence >= CONFIDENCE_THRESHOLDS[SuggestionStrategy.CORE_ONLY]:
        return (
            SuggestionStrategy.CORE_ONLY,
            {
                'top_confidence': top_confidence,
                'recommended_count': 0,
                'message': f"Very low confidence ({top_confidence:.2f}) - using core tools only",
                'user_prompt': None,
            }
        )

    else:
        return (
            SuggestionStrategy.FALLBACK,
            {
                'top_confidence': top_confidence,
                'recommended_count': 0,
                'message': 'No confident persona match. Using core tools only.',
                'user_prompt': None,
            }
        )


# Export main functions
__all__ = [
    'select_strategy',
    'SuggestionStrategy',
    'CONFIDENCE_THRESHOLDS',
]

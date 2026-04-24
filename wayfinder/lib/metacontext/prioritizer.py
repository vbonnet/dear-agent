"""
Multi-domain prioritizer for persona recommendations.

Prioritizes personas when multiple domains detected.
Example: compliance > platform > engineering in regulated industries
"""

from typing import List, Dict


# Priority tiers for personas (higher = more important)
# Based on risk and compliance requirements
PRIORITY_TIERS = {
    # Tier 1: Compliance and security (highest priority)
    'data-privacy-officer': 1,
    'fintech-compliance': 1,
    'security-engineer': 1,

    # Tier 2: Platform and infrastructure
    'mobile-platform': 2,
    'devops-engineer': 2,

    # Tier 3: Engineering and development
    'ml-engineer': 3,
    'frontend-engineer': 3,
    'backend-engineer': 3,
}

# Maximum personas to suggest (avoid overwhelming user)
MAX_PERSONAS = 3


def prioritize_personas(
    personas: List[Dict],
    max_personas: int = MAX_PERSONAS
) -> List[Dict]:
    """
    Prioritize personas when multiple domains detected.

    Args:
        personas: List of persona recommendations (after company overrides)
            [
                {'persona': 'ml-engineer', 'confidence': 0.88, ...},
                {'persona': 'fintech-compliance', 'confidence': 0.75, ...},
                {'persona': 'security-engineer', 'confidence': 1.0, ...},
                ...
            ]
        max_personas: Maximum number of personas to recommend (default: 3)

    Returns:
        Prioritized list (top N personas):
        [
            {'persona': 'security-engineer', 'confidence': 1.0, 'priority_tier': 1, ...},
            {'persona': 'ml-engineer', 'confidence': 0.88, 'priority_tier': 3, ...},
            {'persona': 'fintech-compliance', 'confidence': 0.75, 'priority_tier': 1, ...},
        ]

    Algorithm:
        1. Add priority tier to each persona
        2. Sort by:
           a. Confidence (highest first)
           b. Priority tier (compliance > platform > engineering)
        3. Take top N personas (default: 3)
        4. Mark personas as 'recommended' vs 'available'
    """
    # Add priority tier to each persona
    for persona_rec in personas:
        persona = persona_rec['persona']
        persona_rec['priority_tier'] = PRIORITY_TIERS.get(persona, 99)  # Default: lowest priority

    # Sort by confidence first, then by priority tier
    # Higher confidence = better, lower tier number = higher priority
    personas.sort(key=lambda x: (-x['confidence'], x['priority_tier']))

    # Mark top N as recommended, rest as available
    for i, persona_rec in enumerate(personas):
        if i < max_personas:
            persona_rec['status'] = 'recommended'
        else:
            persona_rec['status'] = 'available'

    return personas


def get_recommended_personas(personas: List[Dict]) -> List[Dict]:
    """
    Get only recommended personas (status='recommended').

    Args:
        personas: Prioritized persona list

    Returns:
        Recommended personas only
    """
    return [p for p in personas if p.get('status') == 'recommended']


def get_available_personas(personas: List[Dict]) -> List[Dict]:
    """
    Get available but not recommended personas (status='available').

    Args:
        personas: Prioritized persona list

    Returns:
        Available personas only
    """
    return [p for p in personas if p.get('status') == 'available']


# Export main functions
__all__ = [
    'prioritize_personas',
    'get_recommended_personas',
    'get_available_personas',
    'PRIORITY_TIERS',
    'MAX_PERSONAS',
]

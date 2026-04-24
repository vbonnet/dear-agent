"""
Company override checker for domain detection.

Applies company-level policies that override confidence scores.
Example: always_required personas get confidence=1.0 (highest priority)
"""

from typing import List, Dict


def apply_company_overrides(
    personas: List[Dict],
    company_config: Dict
) -> List[Dict]:
    """
    Apply company-level overrides to persona recommendations.

    Args:
        personas: Fused persona recommendations from signal fusion
            [
                {'persona': 'ml-engineer', 'confidence': 0.88, ...},
                {'persona': 'fintech-compliance', 'confidence': 0.65, ...},
                ...
            ]
        company_config: Company configuration with override rules
            {
                'always_required': ['security-engineer', 'data-privacy-officer'],
                'never_suggest': ['fintech-compliance'],
                'confidence_boost': {
                    'security-engineer': 0.2,  # Add +0.2 to confidence
                }
            }

    Returns:
        Updated persona list with overrides applied:
        [
            {'persona': 'security-engineer', 'confidence': 1.0, 'override': 'always_required', ...},
            {'persona': 'ml-engineer', 'confidence': 0.88, ...},
            ...
        ]

    Rules:
        1. always_required: Set confidence=1.0, add to list if not present
        2. never_suggest: Remove from list entirely
        3. confidence_boost: Add boost value to confidence (capped at 1.0)
    """
    result = []
    always_required = set(company_config.get('always_required', []))
    never_suggest = set(company_config.get('never_suggest', []))
    confidence_boost = company_config.get('confidence_boost', {})

    # Track which always_required personas are present
    present_personas = set()

    # Process existing personas
    for persona_rec in personas:
        persona = persona_rec['persona']
        present_personas.add(persona)

        # Skip if in never_suggest list
        if persona in never_suggest:
            persona_rec_copy = persona_rec.copy()
            persona_rec_copy['override'] = 'never_suggest'
            persona_rec_copy['excluded'] = True
            continue

        # Apply always_required override
        if persona in always_required:
            persona_rec_copy = persona_rec.copy()
            persona_rec_copy['confidence'] = 1.0
            persona_rec_copy['override'] = 'always_required'
            persona_rec_copy['original_confidence'] = persona_rec['confidence']
            result.append(persona_rec_copy)
            continue

        # Apply confidence boost
        if persona in confidence_boost:
            boost = confidence_boost[persona]
            persona_rec_copy = persona_rec.copy()
            original_conf = persona_rec['confidence']
            boosted_conf = min(1.0, original_conf + boost)
            persona_rec_copy['confidence'] = round(boosted_conf, 3)
            persona_rec_copy['boost'] = boost
            persona_rec_copy['original_confidence'] = original_conf
            result.append(persona_rec_copy)
            continue

        # No override - keep as-is
        result.append(persona_rec)

    # Add always_required personas that weren't detected
    for persona in always_required:
        if persona not in present_personas:
            result.append({
                'persona': persona,
                'confidence': 1.0,
                'signal_count': 0,
                'signals': [],
                'breakdown': {},
                'override': 'always_required',
                'added_by_override': True,
            })

    # Re-sort by confidence (highest first)
    result.sort(key=lambda x: x['confidence'], reverse=True)

    return result


# Export main function
__all__ = ['apply_company_overrides']

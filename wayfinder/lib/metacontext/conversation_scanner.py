"""
Conversation scanner for domain detection.

Scans recent conversation turns for domain-specific keywords.
Example: "train ML model on healthcare data" → ML Engineer + Data Privacy (0.70 confidence)
"""

from __future__ import annotations

import re
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from typing import Any, Dict, List, Set, Tuple


# Keyword mappings: keyword → (persona, base_confidence)
# Base confidence: 0.40-0.70 (conversation is weaker signal than dependencies)
KEYWORD_MAP: Dict[str, Tuple[str, float]] = {
    # ML Engineer keywords (0.40-0.70)
    'machine learning': ('ml-engineer', 0.70),
    'deep learning': ('ml-engineer', 0.70),
    'neural network': ('ml-engineer', 0.70),
    'tensorflow': ('ml-engineer', 0.65),
    'pytorch': ('ml-engineer', 0.65),
    'train model': ('ml-engineer', 0.65),
    'inference': ('ml-engineer', 0.60),
    'dataset': ('ml-engineer', 0.50),
    'feature engineering': ('ml-engineer', 0.65),
    'hyperparameter': ('ml-engineer', 0.70),
    'gradient descent': ('ml-engineer', 0.70),
    'overfitting': ('ml-engineer', 0.65),
    'model evaluation': ('ml-engineer', 0.60),
    'classification': ('ml-engineer', 0.50),
    'regression': ('ml-engineer', 0.50),
    'clustering': ('ml-engineer', 0.55),

    # Fintech Compliance keywords (0.40-0.70)
    'payment processing': ('fintech-compliance', 0.70),
    'pci compliance': ('fintech-compliance', 0.70),
    'pci dss': ('fintech-compliance', 0.70),
    'payment gateway': ('fintech-compliance', 0.65),
    'stripe': ('fintech-compliance', 0.60),
    'paypal': ('fintech-compliance', 0.60),
    'checkout': ('fintech-compliance', 0.50),
    'billing': ('fintech-compliance', 0.50),
    'transaction': ('fintech-compliance', 0.45),
    'refund': ('fintech-compliance', 0.50),
    'chargeback': ('fintech-compliance', 0.60),
    'merchant': ('fintech-compliance', 0.45),
    'fraud detection': ('fintech-compliance', 0.65),
    'kyc': ('fintech-compliance', 0.70),
    'aml': ('fintech-compliance', 0.70),

    # Mobile Platform keywords (0.40-0.70)
    'react native': ('mobile-platform', 0.70),
    'flutter': ('mobile-platform', 0.70),
    'ios app': ('mobile-platform', 0.65),
    'android app': ('mobile-platform', 0.65),
    'mobile app': ('mobile-platform', 0.60),
    'app store': ('mobile-platform', 0.60),
    'play store': ('mobile-platform', 0.60),
    'xcode': ('mobile-platform', 0.65),
    'swift': ('mobile-platform', 0.55),
    'kotlin': ('mobile-platform', 0.55),
    'push notification': ('mobile-platform', 0.50),
    'deep link': ('mobile-platform', 0.55),

    # Data Privacy Officer keywords (0.40-0.70)
    'gdpr': ('data-privacy-officer', 0.70),
    'ccpa': ('data-privacy-officer', 0.70),
    'hipaa': ('data-privacy-officer', 0.70),
    'personally identifiable': ('data-privacy-officer', 0.70),
    'pii': ('data-privacy-officer', 0.65),
    'data retention': ('data-privacy-officer', 0.65),
    'right to deletion': ('data-privacy-officer', 0.70),
    'consent management': ('data-privacy-officer', 0.65),
    'privacy policy': ('data-privacy-officer', 0.50),
    'data subject': ('data-privacy-officer', 0.60),
    'data controller': ('data-privacy-officer', 0.65),
    'data processor': ('data-privacy-officer', 0.65),

    # Security Engineer keywords (0.40-0.70)
    'authentication': ('security-engineer', 0.55),
    'authorization': ('security-engineer', 0.55),
    'encryption': ('security-engineer', 0.60),
    'cryptography': ('security-engineer', 0.65),
    'vulnerability': ('security-engineer', 0.60),
    'penetration test': ('security-engineer', 0.70),
    'security audit': ('security-engineer', 0.65),
    'oauth': ('security-engineer', 0.60),
    'jwt': ('security-engineer', 0.55),
    'ssl': ('security-engineer', 0.50),
    'tls': ('security-engineer', 0.50),
    'firewall': ('security-engineer', 0.55),
    'intrusion detection': ('security-engineer', 0.65),
    'blockchain': ('security-engineer', 0.60),
    'smart contract': ('security-engineer', 0.65),
    'web3': ('security-engineer', 0.60),
    'solidity': ('security-engineer', 0.65),
}


def scan_conversation(messages: List[str], max_messages: int = 5) -> List[Dict[str, Any]]:
    """
    Scan recent conversation for domain-specific keywords.

    Args:
        messages: List of conversation messages (most recent first)
        max_messages: Maximum number of messages to scan (default: 5)

    Returns:
        List of signal dicts:
        [
            {
                'type': 'conversation',
                'source': 'message',
                'name': 'machine learning',
                'persona': 'ml-engineer',
                'confidence': 0.70,
                'context': 'train machine learning model...'
            },
            ...
        ]
    """
    signals: List[Dict[str, Any]] = []
    seen_keywords: Set[str] = set()

    # Scan recent messages (most recent first)
    for message in messages[:max_messages]:
        message_lower: str = message.lower()

        # Try to match each keyword
        for keyword, (persona, confidence) in KEYWORD_MAP.items():
            # Skip if already found this keyword
            if keyword in seen_keywords:
                continue

            # Check if keyword appears in message
            if keyword in message_lower:
                seen_keywords.add(keyword)

                # Extract context (up to 80 chars around keyword)
                context: str = _extract_context(message, keyword, max_length=80)

                signals.append({
                    'type': 'conversation',
                    'source': 'message',
                    'name': keyword,
                    'persona': persona,
                    'confidence': confidence,
                    'context': context,
                })

    return signals


def _extract_context(text: str, keyword: str, max_length: int = 80) -> str:
    """
    Extract context around a keyword in text.

    Args:
        text: Full text
        keyword: Keyword to find
        max_length: Maximum context length

    Returns:
        Context string with keyword highlighted
    """
    # Find keyword position (case-insensitive)
    text_lower: str = text.lower()
    keyword_lower: str = keyword.lower()

    pos: int = text_lower.find(keyword_lower)
    if pos == -1:
        return text[:max_length]

    # Calculate context window
    half_length: int = max_length // 2
    start: int = max(0, pos - half_length)
    end: int = min(len(text), pos + len(keyword) + half_length)

    # Extract and clean
    context: str = text[start:end].strip()

    # Add ellipsis if truncated
    if start > 0:
        context = '...' + context
    if end < len(text):
        context = context + '...'

    return context


# Export main function
__all__ = ['scan_conversation']

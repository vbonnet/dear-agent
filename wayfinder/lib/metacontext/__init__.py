"""
Metacontext analysis for domain detection.

This module implements multi-signal domain detection using:
- Dependency scanning (package.json, requirements.txt, go.mod)
- File pattern matching (models/, payment/, security/)
- Conversation keyword analysis
- Weighted signal fusion
- Company policy overrides
- Multi-domain prioritization
- Graceful degradation strategies

Usage:
    from plugins.wayfinder.lib.metacontext import analyze_project, SuggestionStrategy

    result = analyze_project(
        project_root='/path/to/project',
        messages=['recent conversation messages'],
        company_config={'always_required': ['security-engineer']},
    )

    if result['strategy'] == SuggestionStrategy.AUTO_SUGGEST:
        # Auto-load recommended personas
        for persona in result['personas']:
            if persona['status'] == 'recommended':
                load_persona(persona['persona'])
"""

from .analyzer import (
    MetacontextAnalyzer,
    analyze_project,
    SuggestionStrategy,
)

__all__ = [
    'MetacontextAnalyzer',
    'analyze_project',
    'SuggestionStrategy',
]

__version__ = '0.1.0'

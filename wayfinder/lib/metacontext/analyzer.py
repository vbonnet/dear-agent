"""
Main metacontext analyzer for domain detection.

Orchestrates all scanners and applies fusion, overrides, prioritization, and strategy selection.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from typing import Any, Dict, List, Optional

from .company_override import apply_company_overrides
from .conversation_scanner import scan_conversation
from .dependency_scanner import scan_dependencies
from .file_scanner import scan_file_patterns
from .prioritizer import MAX_PERSONAS, prioritize_personas
from .signal_fusion import fuse_signals
from .strategy_selector import SuggestionStrategy, select_strategy


class MetacontextAnalyzer:
    """
    Main analyzer for domain detection using multi-signal fusion.

    Usage:
        analyzer = MetacontextAnalyzer(project_root='/path/to/project')
        result = analyzer.analyze(
            messages=['recent conversation messages'],
            company_config={'always_required': ['security-engineer']}
        )

        # result contains:
        # - personas: List of recommended personas
        # - strategy: How to present recommendations
        # - metadata: Additional info for user interaction
    """

    def __init__(self, project_root: Optional[str] = None) -> None:
        """
        Initialize analyzer.

        Args:
            project_root: Path to project root (for dependency/file scanning)
                         If None, only conversation scanning is performed
        """
        self.project_root: Optional[str] = project_root

    def analyze(
        self,
        messages: Optional[List[str]] = None,
        company_config: Optional[Dict[str, Any]] = None,
        max_personas: int = MAX_PERSONAS,
    ) -> Dict[str, Any]:
        """
        Analyze project context and recommend personas.

        Args:
            messages: Recent conversation messages (most recent first)
            company_config: Company-level override configuration
            max_personas: Maximum personas to recommend (default: 3)

        Returns:
            Dict with analysis results:
            {
                'personas': [
                    {
                        'persona': 'ml-engineer',
                        'confidence': 0.88,
                        'status': 'recommended',
                        'signal_count': 3,
                        'signals': [...],
                        'breakdown': {'dependency': 0.95, 'file_pattern': 0.85},
                    },
                    ...
                ],
                'strategy': SuggestionStrategy.AUTO_SUGGEST,
                'metadata': {
                    'top_confidence': 0.88,
                    'recommended_count': 2,
                    'message': 'High confidence - auto-loading personas',
                    'user_prompt': None,
                },
                'signal_summary': {
                    'total_signals': 10,
                    'by_type': {
                        'dependency': 3,
                        'file_pattern': 5,
                        'conversation': 2,
                    }
                }
            }
        """
        # Step 1: Collect signals from all scanners
        all_signals: List[Dict[str, Any]] = []

        # Dependency scanner (if project_root provided)
        if self.project_root:
            dependency_signals: List[Dict[str, Any]] = scan_dependencies(self.project_root)
            all_signals.extend(dependency_signals)

        # File pattern scanner (if project_root provided)
        if self.project_root:
            file_signals: List[Dict[str, Any]] = scan_file_patterns(self.project_root, max_depth=3)
            all_signals.extend(file_signals)

        # Conversation scanner (if messages provided)
        if messages:
            conversation_signals: List[Dict[str, Any]] = scan_conversation(messages, max_messages=5)
            all_signals.extend(conversation_signals)

        # Step 2: Fuse signals (weighted averaging per persona)
        fused_personas: List[Dict[str, Any]] = fuse_signals(all_signals)

        # Step 3: Apply company overrides
        if company_config:
            fused_personas = apply_company_overrides(fused_personas, company_config)

        # Step 4: Prioritize personas (multi-domain handling)
        prioritized_personas: List[Dict[str, Any]] = prioritize_personas(fused_personas, max_personas=max_personas)

        # Step 5: Select suggestion strategy
        strategy: SuggestionStrategy
        strategy_metadata: Dict[str, Any]
        strategy, strategy_metadata = select_strategy(prioritized_personas)

        # Step 6: Build signal summary
        signal_summary: Dict[str, Any] = self._build_signal_summary(all_signals)

        # Return complete analysis result
        return {
            'personas': prioritized_personas,
            'strategy': strategy,
            'metadata': strategy_metadata,
            'signal_summary': signal_summary,
        }

    def _build_signal_summary(self, signals: List[Dict[str, Any]]) -> Dict[str, Any]:
        """
        Build summary of signals collected.

        Args:
            signals: All signals from scanners

        Returns:
            Summary dict with counts by type
        """
        by_type: Dict[str, int] = {}
        for signal in signals:
            signal_type: str = signal.get('type', 'unknown')
            by_type[signal_type] = by_type.get(signal_type, 0) + 1

        return {
            'total_signals': len(signals),
            'by_type': by_type,
        }


# Convenience function for one-shot analysis
def analyze_project(
    project_root: Optional[str] = None,
    messages: Optional[List[str]] = None,
    company_config: Optional[Dict[str, Any]] = None,
    max_personas: int = MAX_PERSONAS,
) -> Dict[str, Any]:
    """
    One-shot project analysis (convenience wrapper).

    Args:
        project_root: Path to project root
        messages: Recent conversation messages
        company_config: Company-level overrides
        max_personas: Maximum personas to recommend

    Returns:
        Analysis result dict (same as MetacontextAnalyzer.analyze())
    """
    analyzer: MetacontextAnalyzer = MetacontextAnalyzer(project_root=project_root)
    return analyzer.analyze(
        messages=messages,
        company_config=company_config,
        max_personas=max_personas,
    )


# Export main classes and functions
__all__ = [
    'MetacontextAnalyzer',
    'analyze_project',
    'SuggestionStrategy',
]

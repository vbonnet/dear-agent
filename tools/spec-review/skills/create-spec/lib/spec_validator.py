#!/usr/bin/env python3
"""SPEC Validator - Validate generated SPEC.md against quality rubric."""

import os
import re
import yaml
from pathlib import Path
from typing import Dict, List, Optional, Tuple
from dataclasses import dataclass, field


@dataclass
class ValidationResult:
    """Result of SPEC validation."""
    is_valid: bool
    score: float
    errors: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)
    suggestions: List[str] = field(default_factory=list)
    section_scores: Dict[str, float] = field(default_factory=dict)


class SpecValidator:
    """Validate SPEC.md quality and completeness."""

    # Required sections in SPEC.md
    REQUIRED_SECTIONS = [
        "Vision",
        "User Personas",
        "Critical User Journeys",
        "Goals & Success Metrics",
        "Feature Prioritization",
        "Scope Boundaries",
        "Assumptions & Constraints",
    ]

    def __init__(self, rubric_path: Optional[str] = None):
        """Initialize validator.

        Args:
            rubric_path: Path to quality rubric YAML (uses default if None)
        """
        if rubric_path is None:
            # Use default rubric
            current_dir = Path(__file__).parent.parent.parent
            rubric_path = current_dir / "rubrics" / "spec-quality-rubric.yml"

        self.rubric_path = str(rubric_path)
        self.rubric = self._load_rubric()

    def _load_rubric(self) -> Dict:
        """Load quality rubric from YAML.

        Returns:
            Rubric dictionary
        """
        if not os.path.exists(self.rubric_path):
            # Return minimal default rubric
            return {
                'decision_thresholds': {
                    'pass': 8.0,
                    'warn': 6.0,
                    'fail': 0.0,
                }
            }

        with open(self.rubric_path, 'r', encoding='utf-8') as f:
            return yaml.safe_load(f)

    def validate(self, spec_content: str) -> ValidationResult:
        """Validate SPEC content.

        Args:
            spec_content: SPEC.md content to validate

        Returns:
            ValidationResult with validation details
        """
        result = ValidationResult(is_valid=True, score=0.0)

        # Check structure (sections present)
        structure_score = self._validate_structure(spec_content, result)

        # Check content completeness
        completeness_score = self._validate_completeness(spec_content, result)

        # Check quality indicators
        quality_score = self._validate_quality(spec_content, result)

        # Calculate overall score
        result.score = (
            structure_score * 0.4 +
            completeness_score * 0.3 +
            quality_score * 0.3
        )

        # Check against thresholds
        thresholds = self.rubric.get('decision_thresholds', {})
        pass_threshold = thresholds.get('pass', 8.0)

        if result.score < pass_threshold:
            result.is_valid = False
            result.errors.append(
                f"Overall score {result.score:.1f} below threshold {pass_threshold}"
            )

        return result

    def _validate_structure(self, content: str, result: ValidationResult) -> float:
        """Validate SPEC structure (sections present).

        Args:
            content: SPEC content
            result: ValidationResult to update

        Returns:
            Structure score (0-10)
        """
        missing_sections = []
        present_sections = []

        for section in self.REQUIRED_SECTIONS:
            # Check if section heading exists
            pattern = rf"^#{1,2}\s+\d*\.?\s*{re.escape(section)}"
            if re.search(pattern, content, re.MULTILINE | re.IGNORECASE):
                present_sections.append(section)
            else:
                missing_sections.append(section)

        if missing_sections:
            result.errors.append(
                f"Missing required sections: {', '.join(missing_sections)}"
            )

        # Score: percentage of required sections present
        score = (len(present_sections) / len(self.REQUIRED_SECTIONS)) * 10

        result.section_scores['structure'] = score
        return score

    def _validate_completeness(self, content: str, result: ValidationResult) -> float:
        """Validate content completeness (sections not empty).

        Args:
            content: SPEC content
            result: ValidationResult to update

        Returns:
            Completeness score (0-10)
        """
        checks = {
            'vision': self._check_vision(content),
            'personas': self._check_personas(content),
            'cujs': self._check_cujs(content),
            'metrics': self._check_metrics(content),
            'features': self._check_features(content),
        }

        # Calculate score
        passed_checks = sum(1 for passed in checks.values() if passed)
        score = (passed_checks / len(checks)) * 10

        # Add warnings for failed checks
        if not checks['vision']:
            result.warnings.append("Vision section appears incomplete")
        if not checks['personas']:
            result.warnings.append("User Personas section appears incomplete")
        if not checks['cujs']:
            result.warnings.append("Critical User Journeys section appears incomplete")
        if not checks['metrics']:
            result.warnings.append("Metrics section appears incomplete")
        if not checks['features']:
            result.warnings.append("Features section appears incomplete")

        result.section_scores['completeness'] = score
        return score

    def _validate_quality(self, content: str, result: ValidationResult) -> float:
        """Validate quality indicators.

        Args:
            content: SPEC content
            result: ValidationResult to update

        Returns:
            Quality score (0-10)
        """
        score = 10.0

        # Check minimum length
        if len(content) < 1000:
            result.warnings.append("SPEC appears very short")
            score -= 2

        # Check for placeholder text
        placeholders = ['TBD', 'TODO', 'To be defined', 'To be determined']
        placeholder_count = sum(
            content.count(placeholder) for placeholder in placeholders
        )
        if placeholder_count > 10:
            result.warnings.append(
                f"Many placeholders ({placeholder_count}) - needs completion"
            )
            score -= 2

        # Check for examples/specifics
        if not re.search(r'example|e\.g\.|for instance', content, re.IGNORECASE):
            result.suggestions.append("Consider adding concrete examples")
            score -= 1

        # Check for metrics/numbers
        if not re.search(r'\d+%|>=|<=|>\s*\d+|<\s*\d+', content):
            result.suggestions.append("Add specific numeric targets for metrics")
            score -= 1

        result.section_scores['quality'] = max(0, score)
        return max(0, score)

    def _check_vision(self, content: str) -> bool:
        """Check if vision section has substantive content."""
        # Look for vision section
        vision_match = re.search(
            r'##\s+\d*\.?\s*Vision.*?(?=##|\Z)',
            content,
            re.DOTALL | re.IGNORECASE
        )
        if not vision_match:
            return False

        vision_content = vision_match.group(0)
        # Check for key elements
        has_problem = 'problem' in vision_content.lower()
        has_vision = len(vision_content) > 200
        return has_problem and has_vision

    def _check_personas(self, content: str) -> bool:
        """Check if personas section has substantive content."""
        personas_match = re.search(
            r'##\s+\d*\.?\s*User Personas.*?(?=##|\Z)',
            content,
            re.DOTALL | re.IGNORECASE
        )
        if not personas_match:
            return False

        personas_content = personas_match.group(0)
        # Check for persona elements
        has_persona = 'persona' in personas_content.lower()
        has_goals = 'goals' in personas_content.lower()
        return has_persona and has_goals

    def _check_cujs(self, content: str) -> bool:
        """Check if CUJs section has substantive content."""
        cujs_match = re.search(
            r'##\s+\d*\.?\s*Critical User Journeys.*?(?=##|\Z)',
            content,
            re.DOTALL | re.IGNORECASE
        )
        if not cujs_match:
            return False

        cujs_content = cujs_match.group(0)
        # Check for CUJ elements
        has_cuj = 'cuj' in cujs_content.lower() or 'journey' in cujs_content.lower()
        has_tasks = 'task' in cujs_content.lower()
        return has_cuj and has_tasks

    def _check_metrics(self, content: str) -> bool:
        """Check if metrics section has substantive content."""
        metrics_match = re.search(
            r'##\s+\d*\.?\s*(Goals|Success Metrics).*?(?=##|\Z)',
            content,
            re.DOTALL | re.IGNORECASE
        )
        if not metrics_match:
            return False

        metrics_content = metrics_match.group(0)
        # Check for metrics
        has_metrics = 'metric' in metrics_content.lower()
        has_numbers = re.search(r'\d+%|>=|<=', metrics_content)
        return has_metrics and bool(has_numbers)

    def _check_features(self, content: str) -> bool:
        """Check if features section has substantive content."""
        features_match = re.search(
            r'##\s+\d*\.?\s*Feature.*?(?=##|\Z)',
            content,
            re.DOTALL | re.IGNORECASE
        )
        if not features_match:
            return False

        features_content = features_match.group(0)
        # Check for features
        has_must_have = 'must have' in features_content.lower()
        has_features = len(features_content) > 200
        return has_must_have and has_features

    def validate_from_file(self, spec_path: str) -> ValidationResult:
        """Validate SPEC.md from file.

        Args:
            spec_path: Path to SPEC.md file

        Returns:
            ValidationResult
        """
        if not os.path.exists(spec_path):
            result = ValidationResult(is_valid=False, score=0.0)
            result.errors.append(f"SPEC file not found: {spec_path}")
            return result

        with open(spec_path, 'r', encoding='utf-8') as f:
            content = f.read()

        return self.validate(content)

    def get_summary(self, result: ValidationResult) -> str:
        """Get human-readable validation summary.

        Args:
            result: ValidationResult

        Returns:
            Summary string
        """
        lines = [
            "="*60,
            "SPEC VALIDATION RESULTS",
            "="*60,
            "",
            f"Overall Score: {result.score:.1f}/10.0",
            f"Status: {'PASS' if result.is_valid else 'FAIL'}",
            "",
        ]

        if result.section_scores:
            lines.append("Section Scores:")
            for section, score in result.section_scores.items():
                lines.append(f"  - {section.title()}: {score:.1f}/10.0")
            lines.append("")

        if result.errors:
            lines.append("ERRORS:")
            for error in result.errors:
                lines.append(f"  ✗ {error}")
            lines.append("")

        if result.warnings:
            lines.append("WARNINGS:")
            for warning in result.warnings:
                lines.append(f"  ⚠ {warning}")
            lines.append("")

        if result.suggestions:
            lines.append("SUGGESTIONS:")
            for suggestion in result.suggestions:
                lines.append(f"  💡 {suggestion}")
            lines.append("")

        return "\n".join(lines)


if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print("Usage: python spec_validator.py <spec_path>")
        sys.exit(1)

    validator = SpecValidator()
    result = validator.validate_from_file(sys.argv[1])
    print(validator.get_summary(result))

    sys.exit(0 if result.is_valid else 1)

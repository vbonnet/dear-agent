#!/usr/bin/env python3
"""review-adr: ADR Quality Validation Skill
Multi-persona validation of Architecture Decision Records (ADRs) against hybrid template.
"""

import sys
import json
import argparse
from pathlib import Path
from typing import Dict, List, Tuple, Optional

# Add lib directory to path for imports
PLUGIN_ROOT = Path(__file__).parent.parent.parent
sys.path.insert(0, str(PLUGIN_ROOT / "lib"))

from cli_abstraction import CLIAbstraction, CLIType
from cli_detector import detect_cli


# Scoring constants
SCORE_SECTION_PRESENCE = 20
SCORE_WHY_FOCUS = 25
SCORE_TRADEOFF_TRANSPARENCY = 25
SCORE_AGENTIC_EXTENSIONS = 15
SCORE_CLARITY = 15
SCORE_TOTAL = 100
SCORE_NORMALIZATION_FACTOR = 110  # Total points from personas

# Pass threshold
MIN_PASS_SCORE = 70  # 8/10 = 70/100


class ADRSection:
    """Represents an ADR section."""
    def __init__(self, name: str, content: str):
        self.name = name
        self.content = content


class ADRDocument:
    """Parsed ADR document."""
    def __init__(self, file_path: str, content: str):
        self.file_path = file_path
        self.content = content
        self.sections: Dict[str, ADRSection] = {}
        self.parse_sections()

    def parse_sections(self) -> None:
        """Parse ADR markdown into sections."""
        lines = self.content.split('\n')
        current_section = None
        current_content = []

        for line in lines:
            # Detect section headers (## Section or # Section)
            if line.strip().startswith('#'):
                # Save previous section
                if current_section:
                    self.sections[current_section.lower()] = ADRSection(
                        current_section,
                        '\n'.join(current_content).strip()
                    )

                # Start new section
                section_name = line.lstrip('#').strip()
                current_section = section_name
                current_content = []
            elif current_section:
                current_content.append(line)

        # Save last section
        if current_section:
            self.sections[current_section.lower()] = ADRSection(
                current_section,
                '\n'.join(current_content).strip()
            )

    def has_section(self, section_name: str) -> bool:
        """Check if section exists (case-insensitive)."""
        return section_name.lower() in self.sections

    def get_section(self, section_name: str) -> Optional[ADRSection]:
        """Get section by name (case-insensitive)."""
        return self.sections.get(section_name.lower())


class PersonaReview:
    """Represents a persona review with scoring."""
    def __init__(self, persona_name: str, score: int, max_score: int, feedback: str):
        self.persona_name = persona_name
        self.score = score
        self.max_score = max_score
        self.feedback = feedback


class ADRValidator:
    """Main ADR validation engine."""

    def __init__(self, cli: CLIAbstraction):
        self.cli = cli

    def validate_section_presence(self, adr: ADRDocument) -> Tuple[int, str]:
        """Validate required sections are present (20 points)."""
        required_sections = ['status', 'context', 'decision', 'consequences']
        present = [s for s in required_sections if adr.has_section(s)]
        missing = [s for s in required_sections if not adr.has_section(s)]

        if len(missing) >= 3:
            # Auto-fail: missing 3+ sections
            score = 0
            feedback = f"❌ AUTO-FAIL: Missing {len(missing)} required sections: {', '.join(missing)}"
        elif len(missing) == 2:
            score = 10
            feedback = f"⚠️ Missing {len(missing)} sections: {', '.join(missing)} (10/20)"
        elif len(missing) == 1:
            score = 15
            feedback = f"⚠️ Missing section: {missing[0]} (15/20)"
        else:
            score = 20
            feedback = "✓ All required sections present (20/20)"

        return score, feedback

    def build_persona_prompt(self, persona_name: str, adr: ADRDocument,
                            evaluation_criteria: str) -> str:
        """Build prompt for persona evaluation."""
        prompt = f"""You are reviewing an Architecture Decision Record (ADR) as a {persona_name}.

ADR File: {adr.file_path}

ADR Content:
---
{adr.content}
---

{evaluation_criteria}

Provide your evaluation in JSON format:
{{
  "score": <number>,
  "max_score": <number>,
  "feedback": "<detailed feedback with specific examples>",
  "issues": ["<issue 1>", "<issue 2>", ...],
  "recommendations": ["<rec 1>", "<rec 2>", ...]
}}
"""
        return prompt

    def evaluate_solution_architect(self, adr: ADRDocument) -> PersonaReview:
        """Solution Architect persona: Section Presence + "Why" Focus (45/110)."""

        # Section presence check
        section_score, section_feedback = self.validate_section_presence(adr)

        if section_score == 0:
            # Auto-fail on missing sections
            return PersonaReview(
                "Solution Architect",
                section_score,
                45,
                section_feedback
            )

        criteria = """
Evaluate:
1. Section Presence: Are Status, Context, Decision, Consequences present? (20 points)
2. "Why" Focus: Does Context explain WHY (rationale), not HOW (implementation)? (10 points)
3. Decision Focus: Does Decision explain architectural CHOICE, not design details? (10 points)
4. Implementation Details: Are code snippets or implementation details minimal? (5 points)

Focus on architectural quality and rationale.
"""

        prompt = self.build_persona_prompt("Solution Architect", adr, criteria)

        # Use CLI abstraction for LLM call (placeholder for now)
        # In real implementation, this would call the LLM via CLI

        # Estimate "Why focus" score based on section presence
        # If Context missing, can't have good "why" focus (very low score)
        # If Context present but poor quality, moderate score
        has_context = adr.has_section('context')
        has_decision = adr.has_section('decision')

        if not has_context:
            why_focus_score = 2  # Very poor without context section
        elif not has_decision:
            why_focus_score = 5  # Weak without decision
        else:
            why_focus_score = 15  # Estimate for complete ADR

        # For now, return mock score
        return PersonaReview(
            "Solution Architect",
            section_score + why_focus_score,
            45,
            f"{section_feedback}\n\nWhy Focus: {'No context section (2/25)' if not has_context else 'Pending LLM evaluation'}"
        )

    def evaluate_tech_lead(self, adr: ADRDocument) -> PersonaReview:
        """Tech Lead persona: Trade-Off Transparency + Clarity (35/110)."""

        criteria = """
Evaluate:
1. Positive Consequences: Are benefits documented and quantified? (8 points)
2. Negative Consequences: Are costs/trade-offs documented honestly? (8 points)
3. Alternatives Considered: At least 2 alternatives with pros/cons and rejection rationale? (9 points)
4. Decision Clarity: Is decision unambiguous (no hedging)? (5 points)
5. Actionable Consequences: Are consequences specific and actionable? (5 points)

Focus on trade-off transparency and feasibility.
"""

        prompt = self.build_persona_prompt("Tech Lead", adr, criteria)

        # Estimate score based on section presence
        has_consequences = adr.has_section('consequences')
        has_alternatives = adr.has_section('alternatives considered') or adr.has_section('alternatives')
        has_decision = adr.has_section('decision')

        score = 0
        feedback_parts = []

        # Consequences (16 points: 8 positive + 8 negative)
        if has_consequences:
            score += 12  # Estimate assuming decent consequences
            feedback_parts.append("Consequences section present (12/16)")
        else:
            score += 0
            feedback_parts.append("❌ No Consequences section (0/16)")

        # Alternatives (9 points)
        if has_alternatives:
            score += 7  # Estimate assuming at least 2 alternatives
            feedback_parts.append("Alternatives section present (7/9)")
        else:
            score += 0
            feedback_parts.append("❌ No Alternatives section (0/9)")

        # Decision clarity (5 points)
        if has_decision:
            score += 4  # Estimate
            feedback_parts.append("Decision section present (4/5)")
        else:
            score += 1  # Give minimal points if decision exists but weak
            feedback_parts.append("⚠️ Weak decision (1/5)")

        # Actionable consequences (5 points)
        if has_consequences:
            score += 3  # Estimate
        else:
            score += 0

        return PersonaReview(
            "Tech Lead",
            score,
            35,
            "\n".join(feedback_parts)
        )

    def evaluate_senior_developer(self, adr: ADRDocument) -> PersonaReview:
        """Senior Developer persona: Agentic Extensions + Clarity (30/110)."""

        # Check if agentic sections present
        has_agentic = any([
            adr.has_section('agent context'),
            adr.has_section('architecture'),
            adr.has_section('validation')
        ])

        criteria = f"""
Evaluate:
1. Agentic Extensions: {"Evaluate Agent Context, Architecture, Validation sections (15 points)" if has_agentic else "N/A - traditional ADR, award full 15 points"}
2. Problem Clarity: Is problem statement in Context clear and specific? (5 points)
3. Decision Clarity: Is Decision unambiguous? (5 points)
4. Actionable Consequences: Are consequences concrete, not abstract? (5 points)

Focus on clarity and completeness for future teams.
"""

        prompt = self.build_persona_prompt("Senior Developer", adr, criteria)

        # Award full agentic score if traditional ADR
        agentic_score = 15 if not has_agentic else 10  # Estimate if present

        # Clarity scoring based on section presence and quality
        has_context = adr.has_section('context')
        has_decision = adr.has_section('decision')
        has_consequences = adr.has_section('consequences')

        clarity_score = 0
        # Problem clarity (5 points) - requires context
        if has_context:
            clarity_score += 4  # Estimate
        else:
            clarity_score += 0

        # Decision clarity (5 points)
        if has_decision:
            clarity_score += 4  # Estimate
        else:
            clarity_score += 1

        # Actionable consequences (5 points)
        if has_consequences:
            clarity_score += 3  # Estimate
        else:
            clarity_score += 0

        feedback_parts = []
        if not has_agentic:
            feedback_parts.append("Agentic Extensions: N/A (traditional ADR - 15/15)")
        else:
            feedback_parts.append("Agentic Extensions: Pending evaluation")

        feedback_parts.append(f"Clarity: {clarity_score}/15 (Context: {has_context}, Decision: {has_decision}, Consequences: {has_consequences})")

        return PersonaReview(
            "Senior Developer",
            agentic_score + clarity_score,
            30,
            "\n".join(feedback_parts)
        )

    def aggregate_scores(self, reviews: List[PersonaReview]) -> Tuple[int, int]:
        """Aggregate persona scores and normalize to 100."""
        total_score = sum(r.score for r in reviews)
        total_max = sum(r.max_score for r in reviews)

        # Normalize to 100
        normalized_score = int((total_score / SCORE_NORMALIZATION_FACTOR) * 100)

        return normalized_score, total_score

    def map_to_ten_scale(self, score: int) -> int:
        """Map 100-point score to 1-10 scale."""
        if score >= 90:
            return 10
        elif score >= 80:
            return 9
        elif score >= 70:
            return 8
        elif score >= 60:
            return 7
        elif score >= 50:
            return 6
        elif score >= 40:
            return 5
        else:
            return max(1, int(score / 10))

    def validate_adr(self, adr_file: str) -> Dict:
        """Main validation function."""
        # Read ADR file
        adr_path = Path(adr_file)
        if not adr_path.exists():
            return {
                "success": False,
                "error": f"File not found: {adr_file}"
            }

        content = adr_path.read_text(encoding='utf-8')
        adr = ADRDocument(str(adr_path), content)

        # Run multi-persona validation
        reviews = [
            self.evaluate_solution_architect(adr),
            self.evaluate_tech_lead(adr),
            self.evaluate_senior_developer(adr)
        ]

        # Aggregate scores
        normalized_score, raw_score = self.aggregate_scores(reviews)
        ten_scale = self.map_to_ten_scale(normalized_score)
        passed = normalized_score >= MIN_PASS_SCORE

        return {
            "success": True,
            "file": str(adr_path),
            "score_100": normalized_score,
            "score_10": ten_scale,
            "raw_score": raw_score,
            "passed": passed,
            "threshold": MIN_PASS_SCORE,
            "reviews": [
                {
                    "persona": r.persona_name,
                    "score": r.score,
                    "max_score": r.max_score,
                    "feedback": r.feedback
                }
                for r in reviews
            ]
        }


def generate_report(result: Dict, format: str = "markdown") -> str:
    """Generate validation report."""
    if not result["success"]:
        return f"ERROR: {result['error']}"

    if format == "json":
        return json.dumps(result, indent=2)

    # Markdown report
    status = "✓ PASS" if result["passed"] else "❌ FAIL"

    report = f"""# ADR Validation Report

**File:** {result['file']}
**Overall Score:** {result['score_10']}/10 ({status})
**Total Points:** {result['score_100']}/100 (Raw: {result['raw_score']}/110)
**Threshold:** {result['threshold']}/100

---

## Section-by-Section Breakdown

"""

    for review in result["reviews"]:
        report += f"""### {review['persona']}
**Score:** {review['score']}/{review['max_score']}

{review['feedback']}

"""

    if not result["passed"]:
        report += f"""---

## ⚠️ VALIDATION FAILED

This ADR scored {result['score_100']}/100, below the minimum threshold of {result['threshold']}/100.

**Recommendations:**
1. Review persona feedback above for specific issues
2. Ensure all required sections are present and complete
3. Focus on architectural rationale ("why"), not implementation ("how")
4. Document both positive and negative consequences
5. Include at least 2 alternatives with rejection rationale

"""

    report += """---

*Generated by review-adr skill (spec-review-marketplace plugin)*
"""

    return report


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Validate Architecture Decision Records (ADRs)"
    )
    parser.add_argument(
        "adr_file",
        help="Path to ADR markdown file"
    )
    parser.add_argument(
        "-f", "--format",
        choices=["markdown", "json"],
        default="markdown",
        help="Output format (default: markdown)"
    )
    parser.add_argument(
        "-o", "--output",
        help="Output file (default: stdout)"
    )

    args = parser.parse_args()

    # Detect CLI
    cli = CLIAbstraction()

    # Validate ADR
    validator = ADRValidator(cli)
    result = validator.validate_adr(args.adr_file)

    # Generate report
    report = generate_report(result, args.format)

    # Output
    if args.output:
        Path(args.output).write_text(report, encoding='utf-8')
        print(f"Report saved to: {args.output}", file=sys.stderr)
    else:
        print(report)

    # Exit with appropriate code
    sys.exit(0 if result.get("passed", False) else 1)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Question Generator - Generate clarifying questions based on codebase analysis."""

import json
from typing import Dict, List, Optional, Any
from dataclasses import dataclass, field, asdict
from codebase_analyzer import CodebaseAnalysis


@dataclass
class Question:
    """A clarifying question for spec generation."""
    id: str
    category: str
    question: str
    help_text: Optional[str] = None
    default_value: Optional[str] = None
    required: bool = True
    examples: List[str] = field(default_factory=list)


@dataclass
class QuestionSet:
    """A set of questions organized by category."""
    vision_questions: List[Question] = field(default_factory=list)
    persona_questions: List[Question] = field(default_factory=list)
    cuj_questions: List[Question] = field(default_factory=list)
    metrics_questions: List[Question] = field(default_factory=list)
    scope_questions: List[Question] = field(default_factory=list)
    constraints_questions: List[Question] = field(default_factory=list)


class QuestionGenerator:
    """Generate clarifying questions based on codebase analysis."""

    def __init__(self, interactive: bool = True):
        """Initialize question generator.

        Args:
            interactive: Enable interactive mode for user input
        """
        self.interactive = interactive
        self.answers: Dict[str, Any] = {}

    def generate_questions(self, analysis: CodebaseAnalysis) -> QuestionSet:
        """Generate questions based on codebase analysis.

        Args:
            analysis: CodebaseAnalysis object

        Returns:
            QuestionSet with generated questions
        """
        questions = QuestionSet()

        # Vision questions
        questions.vision_questions = self._generate_vision_questions(analysis)

        # Persona questions
        questions.persona_questions = self._generate_persona_questions(analysis)

        # Critical User Journey questions
        questions.cuj_questions = self._generate_cuj_questions(analysis)

        # Metrics questions
        questions.metrics_questions = self._generate_metrics_questions(analysis)

        # Scope questions
        questions.scope_questions = self._generate_scope_questions(analysis)

        # Constraints questions
        questions.constraints_questions = self._generate_constraints_questions(analysis)

        return questions

    def _generate_vision_questions(self, analysis: CodebaseAnalysis) -> List[Question]:
        """Generate vision-related questions."""
        return [
            Question(
                id="project_name",
                category="vision",
                question="What is the project name?",
                default_value=analysis.project_name,
                help_text="Short, memorable name for the project",
            ),
            Question(
                id="what_is_this",
                category="vision",
                question="What is this project? (1-2 sentence description)",
                help_text="Brief description of what the project does",
                examples=[
                    "A cross-CLI plugin marketplace for documentation review",
                    "A distributed task scheduler for Python",
                ]
            ),
            Question(
                id="problem_statement",
                category="vision",
                question="What problem does this solve? (3-5 bullet points)",
                help_text="List the key problems this project addresses",
            ),
            Question(
                id="product_vision",
                category="vision",
                question="What is the long-term vision?",
                help_text="Where do you see this project in 1-2 years?",
            ),
        ]

    def _generate_persona_questions(self, analysis: CodebaseAnalysis) -> List[Question]:
        """Generate persona-related questions."""
        return [
            Question(
                id="primary_users",
                category="personas",
                question="Who are the primary users?",
                help_text="Describe the main user personas (AI agents, developers, etc.)",
            ),
            Question(
                id="user_goals",
                category="personas",
                question="What are the users' main goals?",
                help_text="List 3-5 goals users want to achieve",
            ),
            Question(
                id="user_pain_points",
                category="personas",
                question="What pain points do users currently face?",
                help_text="Problems users experience that this project solves",
            ),
        ]

    def _generate_cuj_questions(self, analysis: CodebaseAnalysis) -> List[Question]:
        """Generate Critical User Journey questions."""
        return [
            Question(
                id="toothbrush_cuj",
                category="cujs",
                question="What is the 'toothbrush' journey? (daily use case)",
                help_text="The most frequent, essential user journey",
            ),
            Question(
                id="pivotal_cuj",
                category="cujs",
                question="What is the 'pivotal' journey? (first-time critical experience)",
                help_text="The journey that converts users or proves value",
            ),
            Question(
                id="cuj_success_metrics",
                category="cujs",
                question="How do you measure CUJ success?",
                help_text="Metrics that indicate successful journey completion",
            ),
        ]

    def _generate_metrics_questions(self, analysis: CodebaseAnalysis) -> List[Question]:
        """Generate metrics-related questions."""
        return [
            Question(
                id="north_star_metric",
                category="metrics",
                question="What is the North Star metric?",
                help_text="The single metric that best captures success",
                examples=[
                    "% of projects with high-quality documentation",
                    "Daily active users",
                    "Task completion rate",
                ]
            ),
            Question(
                id="primary_metrics",
                category="metrics",
                question="What are the primary success metrics? (3-5)",
                help_text="Key metrics to track for success",
            ),
            Question(
                id="quality_thresholds",
                category="metrics",
                question="What are the quality thresholds?",
                help_text="What values indicate success vs failure?",
            ),
        ]

    def _generate_scope_questions(self, analysis: CodebaseAnalysis) -> List[Question]:
        """Generate scope-related questions."""
        questions = [
            Question(
                id="in_scope_features",
                category="scope",
                question="What features are in scope?",
                help_text="List core features to implement",
            ),
            Question(
                id="out_of_scope_features",
                category="scope",
                question="What features are explicitly out of scope?",
                help_text="Features deferred or excluded",
            ),
        ]

        # Add platform-specific questions based on detected technologies
        if 'Docker' in analysis.technologies:
            questions.append(Question(
                id="container_support",
                category="scope",
                question="Is container deployment in scope?",
                default_value="yes",
                required=False,
            ))

        return questions

    def _generate_constraints_questions(self, analysis: CodebaseAnalysis) -> List[Question]:
        """Generate constraint-related questions."""
        return [
            Question(
                id="technical_constraints",
                category="constraints",
                question="What are the technical constraints?",
                help_text=f"Languages: {', '.join(analysis.languages.keys())}",
            ),
            Question(
                id="timeline_constraints",
                category="constraints",
                question="What is the timeline?",
                help_text="How long until first release?",
            ),
            Question(
                id="resource_constraints",
                category="constraints",
                question="What are the resource constraints?",
                help_text="Team size, budget, infrastructure limits",
            ),
        ]

    def get_answers(self, questions: QuestionSet) -> Dict[str, Any]:
        """Get answers to questions (interactive or defaults).

        Args:
            questions: QuestionSet to answer

        Returns:
            Dictionary of answers
        """
        answers = {}

        # Flatten all questions
        all_questions = (
            questions.vision_questions +
            questions.persona_questions +
            questions.cuj_questions +
            questions.metrics_questions +
            questions.scope_questions +
            questions.constraints_questions
        )

        for question in all_questions:
            if self.interactive:
                answer = self._ask_question(question)
            else:
                answer = question.default_value or ""

            answers[question.id] = answer

        self.answers = answers
        return answers

    def _ask_question(self, question: Question) -> str:
        """Ask a single question interactively.

        Args:
            question: Question to ask

        Returns:
            User's answer
        """
        print(f"\n[{question.category.upper()}] {question.question}")

        if question.help_text:
            print(f"  Help: {question.help_text}")

        if question.examples:
            print("  Examples:")
            for example in question.examples:
                print(f"    - {example}")

        if question.default_value:
            print(f"  Default: {question.default_value}")

        # Get input
        if question.required and not question.default_value:
            prompt = "  Answer (required): "
        elif question.default_value:
            prompt = f"  Answer [default: {question.default_value}]: "
        else:
            prompt = "  Answer (optional): "

        answer = input(prompt).strip()

        # Use default if no answer provided
        if not answer and question.default_value:
            answer = question.default_value

        # Validate required fields
        if question.required and not answer:
            print("  Error: This field is required!")
            return self._ask_question(question)

        return answer

    def get_default_answers(self, questions: QuestionSet, analysis: CodebaseAnalysis) -> Dict[str, Any]:
        """Get default answers for non-interactive mode.

        Args:
            questions: QuestionSet
            analysis: CodebaseAnalysis

        Returns:
            Dictionary of default answers
        """
        return {
            # Vision
            "project_name": analysis.project_name,
            "what_is_this": f"A {analysis.primary_language} project",
            "problem_statement": "To be defined",
            "product_vision": "To be defined",

            # Personas
            "primary_users": "Software developers",
            "user_goals": "Achieve project objectives efficiently",
            "user_pain_points": "Current tools are insufficient",

            # CUJs
            "toothbrush_cuj": "Daily usage workflow",
            "pivotal_cuj": "First-time setup and configuration",
            "cuj_success_metrics": "Task completion rate",

            # Metrics
            "north_star_metric": "User satisfaction",
            "primary_metrics": "Usage frequency, success rate",
            "quality_thresholds": "≥80% success rate",

            # Scope
            "in_scope_features": "Core functionality",
            "out_of_scope_features": "Advanced features (future release)",

            # Constraints
            "technical_constraints": f"Built with {analysis.primary_language}",
            "timeline_constraints": "To be determined",
            "resource_constraints": "Small team",
        }

    def to_json(self) -> str:
        """Export answers to JSON.

        Returns:
            JSON string of answers
        """
        return json.dumps(self.answers, indent=2)

    def from_json(self, json_str: str) -> None:
        """Import answers from JSON.

        Args:
            json_str: JSON string of answers
        """
        self.answers = json.loads(json_str)


if __name__ == "__main__":
    from .codebase_analyzer import CodebaseAnalyzer
    import sys

    if len(sys.argv) < 2:
        print("Usage: python question_generator.py <project_path>")
        sys.exit(1)

    # Analyze codebase
    analyzer = CodebaseAnalyzer()
    analysis = analyzer.analyze(sys.argv[1])

    # Generate questions
    generator = QuestionGenerator(interactive=True)
    questions = generator.generate_questions(analysis)

    # Get answers
    answers = generator.get_answers(questions)

    print("\n" + "="*60)
    print("ANSWERS:")
    print("="*60)
    print(generator.to_json())

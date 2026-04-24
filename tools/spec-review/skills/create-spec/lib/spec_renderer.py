#!/usr/bin/env python3
"""SPEC Renderer - Render SPEC.md from template and answers."""

import os
import re
from pathlib import Path
from typing import Dict, Any, Optional
from datetime import datetime
from codebase_analyzer import CodebaseAnalysis


class SPECRenderer:
    """Render SPEC.md from template and user answers."""

    def __init__(self, template_path: Optional[str] = None):
        """Initialize renderer.

        Args:
            template_path: Path to template file (uses default if None)
        """
        if template_path is None:
            # Use default template
            current_dir = Path(__file__).parent.parent
            template_path = current_dir / "templates" / "spec-template.md"

        self.template_path = str(template_path)

    def render(
        self,
        answers: Dict[str, Any],
        analysis: CodebaseAnalysis,
        output_path: Optional[str] = None
    ) -> str:
        """Render SPEC.md from template.

        Args:
            answers: User answers from QuestionGenerator
            analysis: CodebaseAnalysis object
            output_path: Optional path to write rendered spec

        Returns:
            Rendered SPEC.md content
        """
        # Load template
        template = self._load_template()

        # Prepare context data
        context = self._prepare_context(answers, analysis)

        # Render template
        rendered = self._render_template(template, context)

        # Write to file if output path provided
        if output_path:
            with open(output_path, 'w', encoding='utf-8') as f:
                f.write(rendered)

        return rendered

    def _load_template(self) -> str:
        """Load template file.

        Returns:
            Template content
        """
        if not os.path.exists(self.template_path):
            raise FileNotFoundError(f"Template not found: {self.template_path}")

        with open(self.template_path, 'r', encoding='utf-8') as f:
            return f.read()

    def _prepare_context(
        self,
        answers: Dict[str, Any],
        analysis: CodebaseAnalysis
    ) -> Dict[str, Any]:
        """Prepare context data for template rendering.

        Args:
            answers: User answers
            analysis: CodebaseAnalysis object

        Returns:
            Context dictionary
        """
        # Base metadata
        context = {
            'project_name': answers.get('project_name', analysis.project_name),
            'version': '1.0.0',
            'last_updated': datetime.now().strftime('%Y-%m-%d'),
            'status': 'Draft',
            'contributors': 'Claude Sonnet 4.5',
            'stakeholders': 'Project team',
        }

        # Vision section
        context.update({
            'what_is_this': answers.get('what_is_this', ''),
            'problem_statement': self._format_multiline(
                answers.get('problem_statement', '')
            ),
            'who_benefits': self._format_who_benefits(answers),
            'product_vision': answers.get('product_vision', ''),
        })

        # Personas section
        context['personas'] = self._build_personas(answers)

        # CUJs section
        context['cujs'] = self._build_cujs(answers)

        # Goals & Metrics section
        context['goals_section'] = self._build_goals(answers)

        # Features section
        context.update(self._build_features(answers, analysis))

        # Scope section
        context.update(self._build_scope(answers, analysis))

        # Assumptions & Constraints
        context['assumptions'] = self._build_assumptions(answers)
        context.update(self._build_constraints(answers))

        # Agent specifications
        context.update(self._build_agent_specs(answers))

        # Living document
        context.update(self._build_living_doc(answers))

        # Version history
        context['version_history'] = [{
            'version': '1.0.0',
            'date': datetime.now().strftime('%Y-%m-%d'),
            'changes': 'Initial specification',
            'rationale': 'Generated from create-spec skill'
        }]

        # Technical references
        context['technical_references'] = self._build_tech_refs(analysis)

        return context

    def _render_template(self, template: str, context: Dict[str, Any]) -> str:
        """Render template with context using simple variable substitution.

        Args:
            template: Template string
            context: Context dictionary

        Returns:
            Rendered content
        """
        # Simple mustache-style rendering
        # This is a simplified version - for production, use a proper template engine
        result = template

        # Replace simple variables {{var}}
        for key, value in context.items():
            if isinstance(value, str):
                result = result.replace(f"{{{{{key}}}}}", value)

        # Handle lists and complex structures
        result = self._render_sections(result, context)

        # Clean up any remaining template syntax
        result = re.sub(r'\{\{[^}]+\}\}', '', result)
        result = re.sub(r'\{\{#[^}]+\}\}.*?\{\{/[^}]+\}\}', '', result, flags=re.DOTALL)

        return result

    def _render_sections(self, template: str, context: Dict[str, Any]) -> str:
        """Render complex sections (lists, iterations).

        Args:
            template: Template string
            context: Context dictionary

        Returns:
            Rendered template
        """
        result = template

        # Render personas
        if 'personas' in context:
            personas_md = self._render_personas(context['personas'])
            result = re.sub(
                r'\{\{#personas\}\}.*?\{\{/personas\}\}',
                personas_md,
                result,
                flags=re.DOTALL
            )

        # Render CUJs
        if 'cujs' in context:
            cujs_md = self._render_cujs(context['cujs'])
            result = re.sub(
                r'\{\{#cujs\}\}.*?\{\{/cujs\}\}',
                cujs_md,
                result,
                flags=re.DOTALL
            )

        # Render goals
        if 'goals_section' in context:
            goals_md = self._render_goals(context['goals_section'])
            result = re.sub(
                r'\{\{#goals_section\}\}.*?\{\{/goals_section\}\}',
                goals_md,
                result,
                flags=re.DOTALL
            )

        # Render features
        for feature_type in ['must_have_features', 'should_have_features',
                            'could_have_features', 'wont_have_features']:
            if feature_type in context:
                features_md = self._render_features(context[feature_type])
                result = re.sub(
                    f'\\{{{{#{feature_type}\\}}}}.*?\\{{{{/{feature_type}\\}}}}',
                    features_md,
                    result,
                    flags=re.DOTALL
                )

        return result

    def _render_personas(self, personas: list) -> str:
        """Render personas section."""
        if not personas:
            return "No personas defined yet.\n"

        lines = []
        for persona in personas:
            lines.append(f"### Persona {persona.get('persona_number', '?')}: {persona.get('persona_name', 'Unknown')}\n")
            lines.append(f"**Demographics:** {persona.get('demographics', 'TBD')}\n")
            lines.append("\n**Goals:**")
            for goal in persona.get('goals', []):
                lines.append(f"- {goal}")
            lines.append("\n**Pain Points:**")
            for pain_point in persona.get('pain_points', []):
                lines.append(f"- {pain_point}")
            lines.append("")

        return "\n".join(lines)

    def _render_cujs(self, cujs: list) -> str:
        """Render CUJs section."""
        if not cujs:
            return "No CUJs defined yet.\n"

        lines = []
        for cuj in cujs:
            lines.append(f"### CUJ {cuj.get('cuj_number', '?')}: {cuj.get('cuj_name', 'Unknown')}\n")
            lines.append(f"**Goal:** {cuj.get('cuj_goal', 'TBD')}\n")
            lines.append("")

        return "\n".join(lines)

    def _render_goals(self, goals: list) -> str:
        """Render goals section."""
        if not goals:
            return "No goals defined yet.\n"

        lines = []
        for goal in goals:
            lines.append(f"### Goal {goal.get('goal_number', '?')}: {goal.get('goal_name', 'Unknown')}\n")
            lines.append(f"**Description:** {goal.get('description', 'TBD')}\n")
            lines.append("")

        return "\n".join(lines)

    def _render_features(self, features: list) -> str:
        """Render features section."""
        if not features:
            return ""

        lines = []
        for feature in features:
            lines.append(f"**{feature.get('feature_id', '?')}:** {feature.get('feature_name', 'Unknown')}")
            for key in ['why_critical', 'importance', 'rationale', 'reason']:
                if key in feature:
                    lines.append(f"- {key.replace('_', ' ').title()}: {feature[key]}")
            if 'effort' in feature:
                lines.append(f"- Effort: {feature['effort']}")
            if 'status' in feature:
                lines.append(f"- Status: {feature['status']}")
            lines.append("")

        return "\n".join(lines)

    def _format_multiline(self, text: str) -> str:
        """Format multiline text for markdown."""
        if not text:
            return ""
        # Split by newlines or bullets
        lines = text.split('\n')
        formatted = []
        for line in lines:
            line = line.strip()
            if line:
                if not line.startswith('-'):
                    formatted.append(f"- {line}")
                else:
                    formatted.append(line)
        return '\n'.join(formatted) if formatted else text

    def _format_who_benefits(self, answers: Dict[str, Any]) -> str:
        """Format who benefits section."""
        users = answers.get('primary_users', 'Users')
        return f"- **Primary users:** {users}\n"

    def _build_personas(self, answers: Dict[str, Any]) -> list:
        """Build personas from answers."""
        primary_users = answers.get('primary_users', 'Users')
        goals = self._parse_list(answers.get('user_goals', ''))
        pain_points = self._parse_list(answers.get('user_pain_points', ''))

        return [{
            'persona_number': 1,
            'persona_name': primary_users,
            'demographics': 'Primary user group',
            'goals': goals,
            'pain_points': pain_points,
            'behaviors': [],
            'jobs_to_be_done': [],
        }]

    def _build_cujs(self, answers: Dict[str, Any]) -> list:
        """Build CUJs from answers."""
        cujs = []

        if answers.get('toothbrush_cuj'):
            cujs.append({
                'cuj_number': 1,
                'cuj_name': answers.get('toothbrush_cuj', 'Daily Usage'),
                'cuj_type': 'Toothbrush',
                'cuj_goal': 'Complete daily workflow efficiently',
                'lifecycle_stage': 'Adoption',
                'tasks': [],
                'metrics': self._parse_list(answers.get('cuj_success_metrics', '')),
            })

        if answers.get('pivotal_cuj'):
            cujs.append({
                'cuj_number': 2,
                'cuj_name': answers.get('pivotal_cuj', 'First-Time Setup'),
                'cuj_type': 'Pivotal',
                'cuj_goal': 'Successfully complete initial setup',
                'lifecycle_stage': 'Acquisition',
                'tasks': [],
                'metrics': [],
            })

        return cujs

    def _build_goals(self, answers: Dict[str, Any]) -> list:
        """Build goals section from answers."""
        return [{
            'goal_number': 1,
            'goal_name': 'Primary Objective',
            'description': 'Achieve project goals',
            'north_star_metric': answers.get('north_star_metric', 'TBD'),
            'primary_metrics': self._parse_list(answers.get('primary_metrics', '')),
            'secondary_metrics': [],
            'how_to_measure': 'Track metrics over time',
        }]

    def _build_features(self, answers: Dict[str, Any], analysis: CodebaseAnalysis) -> Dict[str, Any]:
        """Build features sections from answers."""
        in_scope = self._parse_list(answers.get('in_scope_features', ''))
        out_scope = self._parse_list(answers.get('out_of_scope_features', ''))

        must_have = []
        for i, feature in enumerate(in_scope[:3], 1):
            must_have.append({
                'feature_id': f'M{i}',
                'feature_name': feature,
                'why_critical': 'Core functionality',
                'effort': 'TBD',
                'status': 'Planned',
            })

        wont_have = []
        for i, feature in enumerate(out_scope[:3], 1):
            wont_have.append({
                'feature_id': f'W{i}',
                'feature_name': feature,
                'reason': 'Deferred to future release',
            })

        return {
            'must_have_features': must_have,
            'should_have_features': [],
            'could_have_features': [],
            'wont_have_features': wont_have,
        }

    def _build_scope(self, answers: Dict[str, Any], analysis: CodebaseAnalysis) -> Dict[str, Any]:
        """Build scope section from answers."""
        return {
            'in_scope_functional': self._parse_list(answers.get('in_scope_features', '')),
            'in_scope_nonfunctional': [
                'Performance: Fast execution',
                'Reliability: High availability',
                'Usability: Easy to use',
            ],
            'target_segments': [answers.get('primary_users', 'Users')],
            'supported_platforms': list(analysis.technologies) if analysis.technologies else ['TBD'],
            'out_of_scope': self._parse_list(answers.get('out_of_scope_features', '')),
        }

    def _build_assumptions(self, answers: Dict[str, Any]) -> list:
        """Build assumptions from answers."""
        return [{
            'assumption_number': 1,
            'assumption': 'Users understand the domain',
            'impact': 'If false, may need additional documentation',
            'validation': 'User testing and feedback',
        }]

    def _build_constraints(self, answers: Dict[str, Any]) -> Dict[str, list]:
        """Build constraints from answers."""
        return {
            'technical_constraints': self._parse_list(
                answers.get('technical_constraints', '')
            ),
            'organizational_constraints': self._parse_list(
                answers.get('timeline_constraints', '')
            ),
            'resource_constraints': self._parse_list(
                answers.get('resource_constraints', '')
            ),
        }

    def _build_agent_specs(self, answers: Dict[str, Any]) -> Dict[str, Any]:
        """Build agent specifications."""
        return {
            'agent_goal': 'Complete project objectives',
            'agent_constraints': ['Follow best practices', 'Maintain code quality'],
            'unacceptable_behaviors': ['Skipping tests', 'Ignoring errors'],
        }

    def _build_living_doc(self, answers: Dict[str, Any]) -> Dict[str, Any]:
        """Build living document section."""
        return {
            'update_triggers': [
                'New features added',
                'Requirements change',
                'Major milestones reached',
            ],
            'update_steps': [
                {'step_number': 1, 'step': 'Propose change with rationale'},
                {'step_number': 2, 'step': 'Update SPEC.md'},
                {'step_number': 3, 'step': 'Increment version'},
            ],
            'related_docs': [
                {'doc_name': 'ARCHITECTURE.md', 'doc_description': 'System architecture'},
                {'doc_name': 'README.md', 'doc_description': 'Project overview'},
            ],
        }

    def _build_tech_refs(self, analysis: CodebaseAnalysis) -> list:
        """Build technical references from analysis."""
        refs = []
        for file_info in analysis.key_files[:5]:
            refs.append({
                'ref_name': os.path.basename(file_info.path),
                'ref_path': file_info.relative_path,
            })
        return refs

    def _parse_list(self, text: str) -> list:
        """Parse text into list items."""
        if not text:
            return []

        # Split by newlines or commas
        items = re.split(r'[,\n]', text)
        result = []
        for item in items:
            item = item.strip()
            if item:
                # Remove leading bullets
                item = re.sub(r'^[-•*]\s*', '', item)
                if item:
                    result.append(item)

        return result if result else [text]


if __name__ == "__main__":
    from .codebase_analyzer import CodebaseAnalyzer
    from .question_generator import QuestionGenerator
    import sys

    if len(sys.argv) < 3:
        print("Usage: python spec_renderer.py <project_path> <output_path>")
        sys.exit(1)

    # Analyze codebase
    analyzer = CodebaseAnalyzer()
    analysis = analyzer.analyze(sys.argv[1])

    # Generate default answers
    generator = QuestionGenerator(interactive=False)
    questions = generator.generate_questions(analysis)
    answers = generator.get_default_answers(questions, analysis)

    # Render SPEC
    renderer = SPECRenderer()
    spec_content = renderer.render(answers, analysis, sys.argv[2])

    print(f"SPEC.md generated at: {sys.argv[2]}")

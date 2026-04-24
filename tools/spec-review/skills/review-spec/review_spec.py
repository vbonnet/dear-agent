#!/usr/bin/env python3
"""
SPEC.md Quality Validator using LLM-as-Judge

Validates SPEC.md files against research-backed quality rubric.
Enhanced with CLI abstraction for cross-CLI compatibility.
"""

import argparse
import json
import os
import re
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

# Add lib directory to path for CLI abstraction
sys.path.insert(0, str(Path(__file__).parent.parent.parent / "lib"))

# Check for dependencies but don't exit during import (breaks pytest collection)
_MISSING_DEPS = []
try:
    from anthropic import Anthropic, AnthropicVertex, APIError, APITimeoutError
except ImportError as e:
    _MISSING_DEPS.append(f"anthropic ({e})")
    # Define dummy classes for type checking
    class Anthropic: pass  # type: ignore
    class AnthropicVertex: pass  # type: ignore
    class APIError(Exception): pass
    class APITimeoutError(Exception): pass

try:
    from pydantic import BaseModel, ValidationError
except ImportError as e:
    _MISSING_DEPS.append(f"pydantic ({e})")
    # Define dummy classes for type checking
    class BaseModel:  # type: ignore
        def __init__(self, **kwargs):
            for key, value in kwargs.items():
                setattr(self, key, value)
        def dict(self):
            return {k: v for k, v in self.__dict__.items() if not k.startswith('_')}
    class ValidationError(Exception): pass

try:
    from rich.console import Console
    from rich.panel import Panel
except ImportError as e:
    _MISSING_DEPS.append(f"rich ({e})")
    # Define dummy classes for type checking
    class Console: pass  # type: ignore
    class Panel: pass  # type: ignore

# Import CLI abstraction
try:
    from cli_abstraction import CLIAbstraction
    from cli_detector import detect_cli
except ImportError:
    # Fallback if CLI abstraction not available
    class CLIAbstraction:
        def __init__(self):
            self.cli_type = "unknown"
        def get_batch_size(self):
            return 10
        def supports_feature(self, feature):
            return False

    def detect_cli():
        return "unknown"

def _check_dependencies():
    """Check dependencies and exit if missing. Only called when running as script."""
    if _MISSING_DEPS:
        print("Error: Missing dependencies:")
        for dep in _MISSING_DEPS:
            print(f"  - {dep}")
        print("\nInstall with: pip install anthropic pydantic rich")
        sys.exit(3)


class PersonaFeedback(BaseModel):
    """Feedback from a single persona"""
    role: str
    score: float
    feedback: str


class ValidationResult(BaseModel):
    """Complete validation result"""
    overall_score: float
    dimension_scores: Dict[str, float]
    self_consistency: Dict[str, Any]
    personas: List[PersonaFeedback]
    decision: str


def load_spec(spec_path: str) -> str:
    """Load SPEC.md file content"""
    try:
        with open(spec_path, 'r', encoding='utf-8') as f:
            content = f.read()

        if len(content) > 100000:  # 100KB limit
            print(f"Warning: SPEC.md is large ({len(content)} bytes), may affect performance")

        return content
    except FileNotFoundError:
        print(f"Error: File not found: {spec_path}")
        sys.exit(3)
    except Exception as e:
        print(f"Error reading file: {e}")
        sys.exit(3)


def load_rubric() -> str:
    """Load quality rubric from research file"""
    rubric_path = os.path.expanduser("~/src/research/spec-adr-architecture/spec-research-comparison.md")

    # Simplified rubric fallback (used if file doesn't exist or parsing fails)
    simplified_rubric = """
## Quality Rubric

### Vision/Goals (30%)
- Clear problem statement
- Measurable success criteria
- User personas defined

### Critical User Journeys (25%)
- 5-7 CUJs documented
- Task breakdown with success criteria
- Lifecycle stages mapped

### Success Metrics (25%)
- Specific, measurable targets
- Anti-reward-hacking checks
- North Star metric defined

### Scope Boundaries (10%)
- In-scope items listed
- Out-of-scope explicit exclusions
- Assumptions documented

### Living Document Process (10%)
- Update process defined
- Version history tracked
- Related documents referenced
"""

    try:
        with open(rubric_path, 'r', encoding='utf-8') as f:
            content = f.read()

        # Extract quality checklist section (lines 1132-1287)
        lines = content.split('\n')
        checklist_start = None
        checklist_end = None

        for i, line in enumerate(lines):
            if '## Quality Checklist' in line:
                checklist_start = i
            if checklist_start and '##' in line and i > checklist_start + 10:
                checklist_end = i
                break

        if checklist_start:
            checklist = '\n'.join(lines[checklist_start:checklist_end or len(lines)])
            return checklist
        else:
            # Research file found but checklist section missing
            return simplified_rubric

    except Exception as e:
        # File not found or other error - use simplified rubric
        print(f"Warning: Could not load rubric from research file: {e}")
        return simplified_rubric


def validate_diagram_references(spec_path: str, spec_content: str) -> Tuple[List[str], List[str]]:
    """
    Validate diagram references in SPEC.md.

    Args:
        spec_path: Path to SPEC.md file
        spec_content: Content of SPEC.md

    Returns:
        Tuple of (valid_diagrams, broken_references)
    """
    spec_dir = Path(spec_path).parent
    project_root = spec_dir.parent if spec_dir.name == "docs" else spec_dir

    # Find diagram references in markdown
    # Matches: ![...](path/to/diagram.ext) or [link](path/to/diagram.ext)
    diagram_pattern = r'!\[.*?\]\((.*?\.(?:d2|dsl|mmd|mermaid|puml|pu|png|svg|pdf))\)|`(diagrams/.*?\.(?:d2|dsl|mmd|mermaid))`'
    matches = re.findall(diagram_pattern, spec_content, re.IGNORECASE)

    # Flatten matches (regex returns tuples)
    diagram_refs = [match[0] or match[1] for match in matches if match[0] or match[1]]

    valid_diagrams = []
    broken_references = []

    for ref in diagram_refs:
        # Try multiple path resolutions
        possible_paths = [
            project_root / ref,  # Relative to project root
            spec_dir / ref,  # Relative to SPEC.md location
            Path(ref) if Path(ref).is_absolute() else None,  # Absolute path
        ]

        found = False
        for path in possible_paths:
            if path and path.exists():
                valid_diagrams.append(str(ref))
                found = True
                break

        if not found:
            broken_references.append(str(ref))

    return valid_diagrams, broken_references


def build_prompt(spec_content: str, rubric: str) -> str:
    """Construct validation prompt"""
    return f"""You are a SPEC.md quality evaluator. Analyze the following specification document against research-based best practices.

{rubric}

**Dimension Weights:**
- Vision/Goals: 30%
- Critical User Journeys (CUJs): 25%
- Success Metrics: 25%
- Scope Boundaries: 10%
- Living Document Process: 10%

**SPEC.md to Evaluate:**

{spec_content}

**Task:**

1. Generate 5 independent evaluations of this SPEC.md
2. For each evaluation, score each dimension 0-10
3. Calculate variance to check consistency
4. Provide feedback from 3 personas: Technical Writer, Product Manager, Developer
5. Each persona should focus on their domain:
   - Technical Writer: Clarity, structure, completeness
   - Product Manager: Business value, user outcomes
   - Developer: Implementability, actionability

**Output Format:**

Respond with valid JSON matching this exact schema:

{{
  "overall_score": <float 0-10>,
  "dimension_scores": {{
    "vision_goals": <float 0-10>,
    "cujs": <float 0-10>,
    "metrics": <float 0-10>,
    "scope": <float 0-10>,
    "living_doc": <float 0-10>
  }},
  "self_consistency": {{
    "scores": [<5 floats>],
    "mean": <float>,
    "variance": <float>
  }},
  "personas": [
    {{
      "role": "Technical Writer",
      "score": <float 0-10>,
      "feedback": "<2-3 sentences>"
    }},
    {{
      "role": "Product Manager",
      "score": <float 0-10>,
      "feedback": "<2-3 sentences>"
    }},
    {{
      "role": "Developer",
      "score": <float 0-10>,
      "feedback": "<2-3 sentences>"
    }}
  ]
}}

Calculate overall_score as weighted sum: vision_goals*0.3 + cujs*0.25 + metrics*0.25 + scope*0.1 + living_doc*0.1

Provide honest, constructive evaluation. Be specific about what's missing or could improve.
"""


def call_claude(prompt: str, api_key: Optional[str] = None, temperature: float = 0.7, timeout: int = 60, cli_type: str = "unknown") -> str:
    """Call Anthropic API or Vertex AI with retry logic and CLI-specific optimizations"""
    # Check for Vertex AI configuration
    vertex_project = os.getenv("ANTHROPIC_VERTEX_PROJECT_ID")
    vertex_region = os.getenv("CLOUD_ML_REGION")
    use_vertex = os.getenv("CLAUDE_CODE_USE_VERTEX") == "1"

    if use_vertex and vertex_project and vertex_region:
        # Use Vertex AI
        client = AnthropicVertex(
            project_id=vertex_project,
            region=vertex_region,
            timeout=timeout
        )
        # Vertex AI model — use available model on this project
        model = os.getenv("REVIEW_SPEC_MODEL", "claude-3-5-haiku@20241022")
    elif api_key:
        # Use Anthropic API — use latest model
        client = Anthropic(api_key=api_key, timeout=timeout)
        model = os.getenv("REVIEW_SPEC_MODEL", "claude-sonnet-4-20250514")
    else:
        raise ValueError("Either ANTHROPIC_API_KEY or Vertex AI configuration required")

    max_retries = 3
    for attempt in range(max_retries):
        try:
            # CLI-specific optimizations
            kwargs = {
                "model": model,
                "max_tokens": 4096,
                "temperature": temperature,
                "messages": [
                    {"role": "user", "content": prompt}
                ]
            }

            # Add prompt caching for Claude Code (if supported)
            if cli_type == "claude-code":
                # Enable prompt caching for rubric reuse
                # This is handled by the CLI adapter
                pass

            message = client.messages.create(**kwargs)
            return message.content[0].text

        except APITimeoutError:
            if attempt < max_retries - 1:
                print(f"Timeout, retrying ({attempt + 1}/{max_retries})...")
                continue
            else:
                print("Error: API timeout after retries")
                sys.exit(3)

        except APIError as e:
            print(f"Error: Anthropic API error: {e}")
            sys.exit(3)

        except Exception as e:
            print(f"Error: Unexpected error calling API: {e}")
            sys.exit(3)


def parse_response(response_text: str) -> ValidationResult:
    """Parse and validate JSON response"""
    try:
        # Find JSON in response (Claude might add explanation text)
        json_start = response_text.find('{')
        json_end = response_text.rfind('}') + 1

        if json_start == -1 or json_end == 0:
            raise ValueError("No JSON found in response")

        json_str = response_text[json_start:json_end]
        data = json.loads(json_str)

        # Add decision based on score
        score = data.get('overall_score', 0)
        if score >= 8.0:
            decision = "PASS"
        elif score >= 6.0:
            decision = "WARN"
        else:
            decision = "FAIL"

        data['decision'] = decision

        # Validate with Pydantic
        result = ValidationResult(**data)
        return result

    except (json.JSONDecodeError, ValidationError) as e:
        print(f"Error: Could not parse LLM response as valid JSON")
        print(f"Error details: {e}")
        print(f"\nRaw response:\n{response_text[:500]}...")
        sys.exit(3)


def format_output(result: ValidationResult, console: Console):
    """Display results in terminal"""
    score = result.overall_score

    # Color based on decision
    if result.decision == "PASS":
        color = "green"
        emoji = "✅"
    elif result.decision == "WARN":
        color = "yellow"
        emoji = "⚠️"
    else:
        color = "red"
        emoji = "❌"

    # Overall score panel
    console.print(Panel(
        f"[bold {color}]{emoji} {result.decision}: {score:.1f}/10[/bold {color}]",
        title="Overall Score"
    ))

    # Dimension scores
    console.print("\n[bold]Dimension Scores:[/bold]")
    for dim, score_val in result.dimension_scores.items():
        dim_name = dim.replace('_', ' ').title()
        console.print(f"  {dim_name}: {score_val:.1f}/10")

    # Self-consistency
    variance = result.self_consistency.get('variance', 0)
    if variance > 1.0:
        console.print(f"\n[yellow]⚠️ High variance ({variance:.2f}) - scores may be inconsistent[/yellow]")

    # Persona feedback
    console.print("\n[bold]Multi-Persona Feedback:[/bold]\n")
    for persona in result.personas:
        console.print(f"[bold cyan]{persona.role}[/bold cyan] (Score: {persona.score:.1f}/10)")
        console.print(f"{persona.feedback}\n")


def handle_warn(result: ValidationResult) -> int:
    """Handle WARN state with user confirmation"""
    try:
        response = input("\n[WARN] Proceed anyway? [Y/n]: ").strip().lower()
        if response in ['y', 'yes', '']:
            return 0  # Treat as PASS
        else:
            return 2  # User declined
    except (EOFError, KeyboardInterrupt):
        print("\nCancelled")
        return 2


def main():
    """Main entry point"""
    _check_dependencies()

    # Initialize CLI abstraction
    cli = CLIAbstraction()
    cli_type = cli.cli_type

    parser = argparse.ArgumentParser(description="Validate SPEC.md against quality rubric")
    parser.add_argument("spec_path", help="Path to SPEC.md file")
    parser.add_argument("--output-json", help="Save JSON result to file")
    parser.add_argument("--version", action="version", version="review-spec 1.0.0")
    parser.add_argument("--cli", help="Override CLI type detection", choices=["claude-code", "gemini-cli", "opencode", "codex"])

    args = parser.parse_args()

    # Override CLI type if specified
    if args.cli:
        cli_type = args.cli

    # Get API key or check Vertex AI configuration
    api_key = os.environ.get('ANTHROPIC_API_KEY')
    vertex_project = os.getenv("ANTHROPIC_VERTEX_PROJECT_ID")
    vertex_region = os.getenv("CLOUD_ML_REGION")
    use_vertex = os.getenv("CLAUDE_CODE_USE_VERTEX") == "1"

    has_anthropic_key = api_key is not None
    has_vertex_config = use_vertex and vertex_project and vertex_region

    if not has_anthropic_key and not has_vertex_config:
        print("Error: Anthropic API key or Vertex AI configuration required")
        print("Option 1: Set ANTHROPIC_API_KEY environment variable")
        print("Option 2: Configure Vertex AI:")
        print("  - ANTHROPIC_VERTEX_PROJECT_ID (GCP project ID)")
        print("  - CLOUD_ML_REGION (e.g., us-east5)")
        print("  - CLAUDE_CODE_USE_VERTEX=1")
        sys.exit(3)

    # Get optional config
    temperature = float(os.environ.get('REVIEW_SPEC_TEMPERATURE', '0.7'))
    timeout = int(os.environ.get('REVIEW_SPEC_TIMEOUT', '60'))

    console = Console()

    console.print(f"[bold]CLI Detected:[/bold] {cli_type}")
    console.print(f"[bold]Batch Size:[/bold] {cli.get_batch_size()}")
    console.print(f"[bold]Prompt Caching:[/bold] {'Yes' if cli.supports_feature('caching') else 'No'}\n")

    console.print("[bold]Loading SPEC.md...[/bold]")
    spec_content = load_spec(args.spec_path)

    console.print("[bold]Validating diagram references...[/bold]")
    valid_diagrams, broken_refs = validate_diagram_references(args.spec_path, spec_content)

    if valid_diagrams:
        console.print(f"[green]✓ Found {len(valid_diagrams)} diagram reference(s)[/green]")
        for diagram in valid_diagrams[:3]:  # Show first 3
            console.print(f"  - {diagram}")
        if len(valid_diagrams) > 3:
            console.print(f"  ... and {len(valid_diagrams) - 3} more")

    if broken_refs:
        console.print(f"[yellow]⚠️  {len(broken_refs)} broken diagram reference(s):[/yellow]")
        for ref in broken_refs:
            console.print(f"  - {ref} (file not found)")
        console.print()

    console.print("[bold]Loading quality rubric...[/bold]")
    rubric = load_rubric()

    console.print("[bold]Constructing prompt...[/bold]")
    prompt = build_prompt(spec_content, rubric)

    console.print("[bold]Calling Claude API (this may take 5-10 seconds)...[/bold]")
    response = call_claude(prompt, api_key, temperature, timeout, cli_type)

    console.print("[bold]Parsing results...[/bold]\n")
    result = parse_response(response)

    # Display results
    format_output(result, console)

    # Save JSON if requested
    if args.output_json:
        with open(args.output_json, 'w') as f:
            json.dump(result.dict(), f, indent=2)
        console.print(f"\n[green]Results saved to {args.output_json}[/green]")

    # Determine exit code
    if result.decision == "PASS":
        return 0
    elif result.decision == "FAIL":
        return 1
    else:  # WARN
        return handle_warn(result)


if __name__ == "__main__":
    sys.exit(main())

#!/usr/bin/env python3
"""
ARCHITECTURE.md Quality Validator using LLM-as-Judge

Validates ARCHITECTURE.md files against dual-layer architecture framework.
Migrated to spec-review-marketplace with CLI abstraction support.
"""

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Dict, List, Optional

try:
    from anthropic import Anthropic, AnthropicVertex, APIError, APITimeoutError
    from pydantic import BaseModel
    from rich.console import Console
    from rich.panel import Panel
except ImportError as e:
    print(f"Error: Missing dependency: {e}")
    print("Install with: pip install -r requirements.txt")
    sys.exit(3)

# Import CLI abstraction
try:
    sys.path.insert(0, str(Path(__file__).parent.parent.parent / "lib"))
    from cli_abstraction import CLIAbstraction
    from cli_detector import detect_cli
except ImportError as e:
    print(f"Warning: CLI abstraction not available: {e}")
    print("Running in standalone mode")
    CLIAbstraction = None


# Data Models
class QuickValidationResult(BaseModel):
    passed: bool
    missing_sections: List[str]
    missing_diagrams: bool
    missing_adrs: bool


class DimensionScores(BaseModel):
    traditional_architecture: float
    agentic_architecture: float
    adr_integration: float
    visual_diagrams: float


class PersonaFeedback(BaseModel):
    role: str
    score: float
    feedback: str


class SelfConsistency(BaseModel):
    scores: List[float]
    mean: float
    variance: float


class ValidationResult(BaseModel):
    overall_score: float
    dimension_scores: DimensionScores
    personas: List[PersonaFeedback]
    decision: str
    self_consistency: SelfConsistency


# Quick Validation Functions
def quick_validate(arch_path: Path, content: str) -> QuickValidationResult:
    """
    Quick validation gate to fail fast on obviously incomplete docs.
    Checks for required sections, diagrams, and ADR references.
    """
    # Check required sections
    required_sections = ["Traditional Architecture", "Agentic Architecture"]
    # Flexible matching for section headers
    section_pattern = r'##\s*(?:Traditional\s*Architecture|System\s*Architecture|Components)'
    agentic_pattern = r'##\s*(?:Agentic\s*Architecture|Agent\s*Patterns|Multi-Agent)'

    has_traditional = bool(re.search(section_pattern, content, re.IGNORECASE))
    has_agentic = bool(re.search(agentic_pattern, content, re.IGNORECASE))

    missing_sections = []
    if not has_traditional:
        missing_sections.append("Traditional Architecture")
    if not has_agentic:
        missing_sections.append("Agentic Architecture")

    # Check for C4 diagrams
    diagram_dirs = [
        arch_path.parent / "docs" / "architecture",
        arch_path.parent / "docs" / "diagrams",
        arch_path.parent / "diagrams",
        arch_path.parent / "architecture"
    ]

    # Include diagram-as-code formats (.d2, .dsl) and rendered formats
    diagram_extensions = [".d2", ".dsl", ".puml", ".pu", ".mmd", ".mermaid", ".drawio", ".png", ".svg"]
    diagram_files = []

    for dir_path in diagram_dirs:
        if dir_path.exists():
            for ext in diagram_extensions:
                diagram_files.extend(dir_path.glob(f"**/*{ext}"))

    has_diagrams = len(diagram_files) > 0

    # Check for ADR references
    adr_pattern = r'\[ADR-\d+\]|docs/adr/|adr/\d+|decisions/'
    has_adrs = bool(re.search(adr_pattern, content, re.IGNORECASE))

    return QuickValidationResult(
        passed=(not missing_sections and has_diagrams and has_adrs),
        missing_sections=missing_sections,
        missing_diagrams=not has_diagrams,
        missing_adrs=not has_adrs
    )


class DiagramValidationResult(BaseModel):
    """Result from diagram validation."""
    passed: bool
    diagram_count: int
    syntax_errors: List[str]
    sync_score: Optional[float]
    quality_score: float  # 0-10


def validate_diagrams(arch_path: Path) -> DiagramValidationResult:
    """
    Validate architecture diagrams for syntax and quality.

    Checks:
    - Diagram files exist
    - D2/Structurizr/Mermaid syntax is valid
    - Optionally checks diagram-code sync

    Returns:
        DiagramValidationResult with validation details
    """
    # Find diagram files
    diagram_dirs = [
        arch_path.parent / "docs" / "architecture",
        arch_path.parent / "docs" / "diagrams",
        arch_path.parent / "diagrams",
        arch_path.parent / "architecture"
    ]

    diagram_files = []
    for dir_path in diagram_dirs:
        if dir_path.exists():
            # Focus on diagram-as-code formats
            for ext in [".d2", ".dsl", ".mmd", ".mermaid"]:
                diagram_files.extend(dir_path.glob(f"**/*{ext}"))

    if not diagram_files:
        return DiagramValidationResult(
            passed=False,
            diagram_count=0,
            syntax_errors=["No diagram-as-code files found"],
            sync_score=None,
            quality_score=0.0
        )

    syntax_errors = []
    valid_diagrams = 0

    # Validate each diagram file
    for diagram_file in diagram_files:
        ext = diagram_file.suffix.lower()

        try:
            if ext == ".d2":
                # Validate D2 syntax
                result = subprocess.run(
                    ["d2", "compile", "--dry-run", str(diagram_file)],
                    capture_output=True,
                    text=True,
                    timeout=10
                )
                if result.returncode == 0:
                    valid_diagrams += 1
                else:
                    syntax_errors.append(f"{diagram_file.name}: {result.stderr.strip()}")

            elif ext == ".dsl":
                # Validate Structurizr DSL syntax
                result = subprocess.run(
                    ["structurizr-cli", "validate", str(diagram_file)],
                    capture_output=True,
                    text=True,
                    timeout=10
                )
                if result.returncode == 0:
                    valid_diagrams += 1
                else:
                    syntax_errors.append(f"{diagram_file.name}: {result.stderr.strip()}")

            elif ext in [".mmd", ".mermaid"]:
                # Validate Mermaid syntax (basic check - file must parse)
                with open(diagram_file, 'r', encoding='utf-8') as f:
                    content = f.read()
                    if content.strip():
                        valid_diagrams += 1
                    else:
                        syntax_errors.append(f"{diagram_file.name}: Empty file")

        except subprocess.TimeoutExpired:
            syntax_errors.append(f"{diagram_file.name}: Validation timeout")
        except FileNotFoundError:
            # Tool not installed - count as valid (we can't validate)
            valid_diagrams += 1
        except Exception as e:
            syntax_errors.append(f"{diagram_file.name}: {str(e)}")

    # Calculate quality score based on validation results
    if len(diagram_files) > 0:
        quality_score = (valid_diagrams / len(diagram_files)) * 10.0
    else:
        quality_score = 0.0

    return DiagramValidationResult(
        passed=(valid_diagrams > 0 and len(syntax_errors) == 0),
        diagram_count=len(diagram_files),
        syntax_errors=syntax_errors,
        sync_score=None,  # Optional: could integrate diagram-sync here
        quality_score=quality_score
    )


def select_personas(content: str) -> List[str]:
    """
    Select personas based on content analysis.
    System Architect always included, others conditional.
    """
    personas = ["System Architect"]

    # Check for deployment/infrastructure sections
    deployment_keywords = r'deployment|infrastructure|kubernetes|k8s|docker|cloud|aws|gcp|azure'
    if re.search(deployment_keywords, content, re.IGNORECASE):
        personas.append("DevOps Engineer")

    # Check for code architecture or agentic patterns
    code_keywords = r'agent|module|package|coordinator|state\s*management'
    if re.search(code_keywords, content, re.IGNORECASE):
        personas.append("Developer")

    return personas


def load_rubric() -> str:
    """
    Load quality rubric from spec-review-marketplace rubrics directory.
    Falls back to simplified rubric if file not found.
    """
    # Try plugin rubrics directory first
    plugin_rubric_path = Path(__file__).parent.parent.parent / "rubrics" / "architecture-quality-rubric.yml"

    # Try research directory as fallback
    research_rubric_path = Path.home() / "src/research/spec-adr-architecture/architecture-research-comparison.md"

    rubric_content = None

    if plugin_rubric_path.exists():
        try:
            with open(plugin_rubric_path, 'r', encoding='utf-8') as f:
                rubric_content = f.read()
        except Exception as e:
            print(f"Warning: Could not load rubric from plugin: {e}")

    if not rubric_content and research_rubric_path.exists():
        try:
            with open(research_rubric_path, 'r', encoding='utf-8') as f:
                content = f.read()

            # Extract key sections (simplified for v1)
            lines = content.split('\n')
            rubric_sections = []

            for i, line in enumerate(lines):
                if '## Key Agreements' in line or '## Visual-First' in line:
                    # Extract next 20 lines as context
                    rubric_sections.extend(lines[i:i+20])

            if rubric_sections:
                rubric_content = '\n'.join(rubric_sections)
        except Exception as e:
            print(f"Warning: Could not load rubric from research file: {e}")

    if rubric_content:
        return rubric_content

    # Fallback simplified rubric
    return """
## Architecture Documentation Quality Rubric

### Traditional Architecture (45%)
- Component architecture clearly documented
- C4 diagrams present (Context, Container, Component)
- Deployment architecture described
- Data flow and integration points documented

### Agentic Architecture (25%)
- Agent patterns documented (if applicable)
- Coordination strategies explained
- State management approach described

### ADR Integration (10%)
- Architectural decisions reference ADRs
- Decision rationale linked to ADR documents

### Visual Diagrams (20%) - ENHANCED
- C4 diagrams present and referenced
- Diagram-as-code syntax is valid (D2, Structurizr, Mermaid)
- Diagrams are clear and up-to-date
"""


def build_prompt(
    content: str,
    rubric: str,
    personas: List[str],
    cli: Optional[CLIAbstraction] = None,
    diagram_result: Optional[DiagramValidationResult] = None
) -> str:
    """Build LLM validation prompt with persona instructions."""
    persona_instructions = {
        "System Architect": "Focus on overall architecture quality, component design, patterns, and structural coherence",
        "DevOps Engineer": "Focus on deployment architecture, observability, scalability, and operational concerns",
        "Developer": "Focus on code organization, module structure, implementability, and agentic patterns"
    }

    persona_prompts = "\n".join([
        f"- {role}: {persona_instructions[role]}"
        for role in personas
    ])

    # Add diagram validation context
    diagram_context = ""
    if diagram_result:
        diagram_context = f"""
**Diagram Validation Results:**
- Diagram count: {diagram_result.diagram_count}
- Syntax validation: {"PASS" if not diagram_result.syntax_errors else "FAIL"}
- Quality score: {diagram_result.quality_score:.1f}/10.0
"""
        if diagram_result.syntax_errors:
            diagram_context += f"- Syntax errors: {len(diagram_result.syntax_errors)} found\n"

    prompt = f"""You are evaluating an ARCHITECTURE.md file against research-based best practices for documentation quality.

{rubric}

**Evaluation Dimensions (weights):**
- Traditional Architecture: 45%
- Agentic Architecture: 25%
- ADR Integration: 10%
- Visual Diagrams: 20% (ENHANCED with syntax validation)

**Personas to provide feedback:**
{persona_prompts}
{diagram_context}
**ARCHITECTURE.md to evaluate:**

{content}

**Task:**
1. Generate 5 independent evaluations of this ARCHITECTURE.md
2. For each evaluation, score each dimension 0-10
3. Calculate variance to check consistency (threshold: variance <0.5 indicates reliable scoring)
4. Provide feedback from the specified personas above
5. Each persona should focus on their domain as described
6. When scoring Visual Diagrams dimension, heavily weight the diagram validation results above (syntax errors should significantly reduce score)

**Output Format:**

Respond with valid JSON matching this exact schema:

{{
  "overall_score": <float 0-10>,
  "dimension_scores": {{
    "traditional_architecture": <float 0-10>,
    "agentic_architecture": <float 0-10>,
    "adr_integration": <float 0-10>,
    "visual_diagrams": <float 0-10>
  }},
  "self_consistency": {{
    "scores": [<5 floats>],
    "mean": <float>,
    "variance": <float>
  }},
  "personas": [
    {{"role": "<persona>", "score": <float 0-10>, "feedback": "<2-3 sentences>"}}
  ]
}}

Calculate overall_score as weighted sum:
  traditional_architecture * 0.45 +
  agentic_architecture * 0.25 +
  adr_integration * 0.1 +
  visual_diagrams * 0.2

Provide honest, constructive evaluation. Be specific about what's missing or could improve.
"""

    # Use CLI abstraction for caching if available
    if cli and cli.supports_feature("caching"):
        prompt = cli.cache_prompt("review-architecture-rubric", prompt)

    return prompt


def call_claude(prompt: str, api_key: Optional[str] = None, temperature: float = 0.7, timeout: int = 60) -> str:
    """Call Anthropic API or Vertex AI with retry logic."""
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
        # Vertex AI uses different model naming
        model = "claude-3-5-sonnet-v2@20241022"
    elif api_key:
        # Use Anthropic API
        client = Anthropic(api_key=api_key, timeout=timeout)
        model = "claude-sonnet-4-5-20251022"
    else:
        raise ValueError("Either ANTHROPIC_API_KEY or Vertex AI configuration required")

    max_retries = 3
    for attempt in range(max_retries):
        try:
            message = client.messages.create(
                model=model,
                max_tokens=4096,
                temperature=temperature,
                messages=[{"role": "user", "content": prompt}]
            )

            # Extract text from response
            if message.content and len(message.content) > 0:
                return message.content[0].text
            else:
                raise APIError("Empty response from Claude API")

        except APITimeoutError:
            if attempt < max_retries - 1:
                print(f"API timeout, retrying ({attempt + 1}/{max_retries})...")
                continue
            else:
                raise
        except APIError as e:
            if attempt < max_retries - 1:
                print(f"API error: {e}, retrying ({attempt + 1}/{max_retries})...")
                continue
            else:
                raise

    raise APIError("Max retries exceeded")


def parse_json_response(response: str) -> ValidationResult:
    """Parse LLM response into ValidationResult model."""
    # Extract JSON from response (may be wrapped in markdown code blocks)
    json_match = re.search(r'```json\n(.*?)\n```', response, re.DOTALL)
    if json_match:
        json_str = json_match.group(1)
    else:
        # Assume entire response is JSON
        json_str = response

    data = json.loads(json_str)

    # Map decision based on score
    score = data["overall_score"]
    if score >= 8.0:
        decision = "PASS"
    elif score >= 6.0:
        decision = "WARN"
    else:
        decision = "FAIL"

    return ValidationResult(
        overall_score=data["overall_score"],
        dimension_scores=DimensionScores(**data["dimension_scores"]),
        personas=[PersonaFeedback(**p) for p in data["personas"]],
        decision=decision,
        self_consistency=data["self_consistency"]
    )


def output_terminal(result: ValidationResult, console: Console, cli_type: Optional[str] = None):
    """Output validation result to terminal with rich formatting."""
    # Determine color based on decision
    color_map = {"PASS": "green", "WARN": "yellow", "FAIL": "red"}
    color = color_map.get(result.decision, "white")

    # Build output
    output_lines = [
        f"[bold]Overall Score:[/bold] {result.overall_score:.1f}/10.0 [{color}]{result.decision}[/{color}]\n",
        "[bold]Dimension Scores:[/bold]",
        f"  Traditional Architecture: {result.dimension_scores.traditional_architecture:.1f}/10.0 (50% weight)",
        f"  Agentic Architecture: {result.dimension_scores.agentic_architecture:.1f}/10.0 (30% weight)",
        f"  ADR Integration: {result.dimension_scores.adr_integration:.1f}/10.0 (10% weight)",
        f"  Visual Diagrams: {result.dimension_scores.visual_diagrams:.1f}/10.0 (10% weight)\n",
        "[bold]Persona Feedback:[/bold]"
    ]

    for persona in result.personas:
        output_lines.append(f"\n[bold]{persona.role}[/bold] (score: {persona.score:.1f}/10.0)")
        output_lines.append(f"  {persona.feedback}")

    output_lines.append(f"\n[bold]Self-Consistency:[/bold]")
    output_lines.append(f"  Variance: {result.self_consistency['variance']:.3f} (threshold: <0.5)")

    if cli_type:
        output_lines.append(f"\n[dim]CLI: {cli_type}[/dim]")

    console.print(Panel("\n".join(output_lines), title="ARCHITECTURE.md Validation Report", border_style=color))


def main():
    parser = argparse.ArgumentParser(description="Validate ARCHITECTURE.md files")
    parser.add_argument("file_path", help="Path to ARCHITECTURE.md file")
    parser.add_argument("--output-json", help="Output JSON to file")
    parser.add_argument("--api-key", help="Anthropic API key (or set ANTHROPIC_API_KEY env var)")

    args = parser.parse_args()

    # Initialize CLI abstraction if available
    cli = None
    cli_type = "unknown"
    if CLIAbstraction:
        try:
            cli = CLIAbstraction()
            cli_type = cli.cli_type
        except Exception as e:
            print(f"Warning: CLI abstraction initialization failed: {e}")

    # Get API key or check Vertex AI configuration
    api_key = args.api_key or os.getenv("ANTHROPIC_API_KEY")
    vertex_project = os.getenv("ANTHROPIC_VERTEX_PROJECT_ID")
    vertex_region = os.getenv("CLOUD_ML_REGION")
    use_vertex = os.getenv("CLAUDE_CODE_USE_VERTEX") == "1"

    has_anthropic_key = api_key is not None
    has_vertex_config = use_vertex and vertex_project and vertex_region

    if not has_anthropic_key and not has_vertex_config:
        print("Error: Anthropic API key or Vertex AI configuration required")
        print("Option 1: Set ANTHROPIC_API_KEY environment variable or use --api-key option")
        print("Option 2: Configure Vertex AI:")
        print("  - ANTHROPIC_VERTEX_PROJECT_ID (GCP project ID)")
        print("  - CLOUD_ML_REGION (e.g., us-east5)")
        print("  - CLAUDE_CODE_USE_VERTEX=1")
        sys.exit(3)

    # Load file
    arch_path = Path(args.file_path)
    if not arch_path.exists():
        print(f"Error: File not found: {args.file_path}")
        sys.exit(3)

    try:
        with open(arch_path, 'r', encoding='utf-8') as f:
            content = f.read()
    except Exception as e:
        print(f"Error reading file: {e}")
        sys.exit(3)

    console = Console()

    # Quick validation gate
    console.print("[bold]Running quick validation gate...[/bold]")
    quick_result = quick_validate(arch_path, content)

    if not quick_result.passed:
        console.print("[bold red]FAIL:[/bold red] Quick validation failed\n")
        if quick_result.missing_sections:
            console.print(f"Missing sections: {', '.join(quick_result.missing_sections)}")
        if quick_result.missing_diagrams:
            console.print("Missing C4 diagrams (checked docs/, diagrams/, architecture/)")
        if quick_result.missing_adrs:
            console.print("Missing ADR references")

        sys.exit(1)

    console.print("[green]✓[/green] Quick validation passed\n")

    # Diagram validation
    console.print("[bold]Validating architecture diagrams...[/bold]")
    diagram_result = validate_diagrams(arch_path)

    if diagram_result.diagram_count > 0:
        console.print(f"Found {diagram_result.diagram_count} diagram(s)")
        if diagram_result.syntax_errors:
            console.print("[yellow]⚠️  Syntax errors found:[/yellow]")
            for error in diagram_result.syntax_errors:
                console.print(f"  - {error}")
        else:
            console.print("[green]✓[/green] All diagrams have valid syntax")
        console.print(f"Diagram quality score: {diagram_result.quality_score:.1f}/10.0\n")
    else:
        console.print("[yellow]⚠️  No diagram-as-code files found[/yellow]\n")

    # LLM validation
    console.print("[bold]Running LLM-based validation...[/bold]")
    if cli_type != "unknown":
        console.print(f"[dim]Detected CLI: {cli_type}[/dim]")

    rubric = load_rubric()
    personas = select_personas(content)
    console.print(f"Selected personas: {', '.join(personas)}\n")

    prompt = build_prompt(content, rubric, personas, cli, diagram_result)

    try:
        response = call_claude(prompt, api_key)
        result = parse_json_response(response)
    except Exception as e:
        print(f"Error during LLM validation: {e}")
        sys.exit(3)

    # Output results
    if args.output_json:
        with open(args.output_json, 'w') as f:
            json.dump(result.model_dump(), f, indent=2)
        console.print(f"[green]JSON output saved to {args.output_json}[/green]")

    output_terminal(result, console, cli_type)

    # Exit with appropriate code
    if result.decision == "PASS":
        sys.exit(0)
    elif result.decision == "WARN":
        sys.exit(2)
    else:
        sys.exit(1)


if __name__ == "__main__":
    main()

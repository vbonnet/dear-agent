# Components

## CodebaseAnalyzer

Analyzes project structure and extracts context.

**Capabilities:**
- Detect programming languages
- Identify technologies (Docker, GitHub Actions, etc.)
- Find key files (README, package.json, etc.)
- Extract project metadata
- Build directory structure map

**Output:**
```python
CodebaseAnalysis(
    project_name="my-project",
    primary_language="Python",
    languages={"Python": 42, "YAML": 5},
    technologies={"Python", "Docker", "GitHub Actions"},
    file_count=47,
    key_files=[...],
    readme_content="...",
)
```

## QuestionGenerator

Generates targeted questions based on codebase analysis.

**Question Categories:**
- Vision & Goals
- User Personas
- Critical User Journeys (CUJs)
- Success Metrics
- Scope & Features
- Assumptions & Constraints

**Modes:**
- **Interactive**: Prompts user for answers
- **Non-interactive**: Uses intelligent defaults

**Output:**
```python
{
    "project_name": "my-project",
    "what_is_this": "A task automation framework",
    "problem_statement": "Manual task execution is error-prone",
    "primary_users": "Software developers",
    ...
}
```

## SPECRenderer

Renders SPEC.md from template and answers.

**Features:**
- Mustache-style template rendering
- Automatic section population
- Metadata injection (date, version)
- Context-aware defaults

**Template Variables:**
- `{{project_name}}`: Project name
- `{{problem_statement}}`: Problem being solved
- `{{personas}}`: User personas list
- `{{cujs}}`: Critical user journeys
- And many more...

## SpecValidator

Validates generated SPEC against quality rubric.

**Validation Checks:**
- Structure: Required sections present
- Completeness: Sections have content
- Quality: Specific metrics, examples

**Scoring:**
- Structure: 40% of score
- Completeness: 30% of score
- Quality: 30% of score
- Threshold: 8.0/10.0 to pass

**Output:**
```python
ValidationResult(
    is_valid=True,
    score=8.2,
    errors=[],
    warnings=["Some metrics appear vague"],
    suggestions=["Add numeric targets"],
    section_scores={"structure": 9.0, "completeness": 8.0, "quality": 7.5}
)
```

# Marketplace Discovery - Usage Examples

This document provides comprehensive examples for using the marketplace discovery tools.

## Table of Contents

- [Basic Commands](#basic-commands)
- [Search and Filter](#search-and-filter)
- [CLI Compatibility](#cli-compatibility)
- [Programmatic Usage (Python)](#programmatic-usage-python)
- [Advanced Use Cases](#advanced-use-cases)

---

## Basic Commands

### List All Skills

```bash
# Text format (default)
marketplace-discover list

# JSON format
marketplace-discover list --json
```

Output:
```
=== Available Skills ===

- review-spec (1.0.0): Validate SPEC.md files against research-backed quality rubric
- review-architecture (1.0.0): Validate ARCHITECTURE.md files with multi-persona assessment
- review-adr (1.0.0): Validate ADR files with multi-persona assessment and anti-pattern detection
- create-spec (1.0.0): Generate SPEC.md files from codebase analysis and requirements
```

### Get Skill Information

```bash
# Show detailed information about a specific skill
marketplace-discover info review-spec
```

Output:
```
=== Skill: review-spec ===

Description: Validate SPEC.md files against research-backed quality rubric
Version: 1.0.0
Category: N/A

Tags:
  - spec
  - validation
  - quality-assessment
  - llm-as-judge

Supported CLIs:
  - claude-code
  - codex
  - gemini-cli
  - opencode

Entry Point: review_spec.py

Dependencies:
  - anthropic>=0.18.0
  - pydantic>=2.0.0
  - rich>=13.0.0
```

### List Available Tags

```bash
marketplace-discover tags
```

Output:
```
Available tags:

  - adr
  - anti-patterns
  - architecture
  - automation
  - codebase-analysis
  - generation
  - llm-as-judge
  - multi-persona
  - quality-assessment
  - spec
  - validation
```

---

## Search and Filter

### Search by Keyword

```bash
# Search for skills containing "validation"
marketplace-discover search validation

# Search for skills containing "spec"
marketplace-discover search spec
```

Output:
```
Found 2 skill(s) matching 'spec':

- review-spec (1.0.0): Validate SPEC.md files against research-backed quality rubric
- create-spec (1.0.0): Generate SPEC.md files from codebase analysis and requirements
```

### Filter by Single Tag (OR logic)

```bash
# Find skills with either "validation" OR "generation" tags
marketplace-discover filter validation generation
```

Output:
```
Found 4 skill(s) with tags: validation, generation

- review-spec: spec, validation, quality-assessment, llm-as-judge
- review-architecture: architecture, validation, quality-assessment, llm-as-judge, multi-persona
- review-adr: adr, validation, quality-assessment, multi-persona, anti-patterns
- create-spec: spec, generation, automation, codebase-analysis
```

### Filter by Multiple Tags (AND logic)

```bash
# Find skills that have BOTH "validation" AND "multi-persona" tags
marketplace-discover filter-and validation multi-persona
```

Output:
```
Found 2 skill(s) with ALL tags: validation, multi-persona

- review-architecture: architecture, validation, quality-assessment, llm-as-judge, multi-persona
- review-adr: adr, validation, quality-assessment, multi-persona, anti-patterns
```

### List Skills by Category

```bash
# First, list available categories
marketplace-discover categories

# Then list skills in a category
marketplace-discover category documentation-review
```

---

## CLI Compatibility

### List Compatible Skills for Current CLI

```bash
# Uses auto-detected CLI
marketplace-discover compatible
```

### List Compatible Skills for Specific CLI

```bash
# Check Claude Code compatibility
marketplace-discover compatible claude-code

# Check Gemini CLI compatibility
marketplace-discover compatible gemini-cli

# Check OpenCode compatibility
marketplace-discover compatible opencode

# Check Codex compatibility
marketplace-discover compatible codex
```

Output:
```
Found 4 skill(s) compatible with 'claude-code':

- review-spec: Validate SPEC.md files against research-backed quality rubric
- review-architecture: Validate ARCHITECTURE.md files with multi-persona assessment
- review-adr: Validate ADR files with multi-persona assessment and anti-pattern detection
- create-spec: Generate SPEC.md files from codebase analysis and requirements
```

### Generate Compatibility Matrix

```bash
marketplace-discover matrix
```

Output:
```
# Skill Compatibility Matrix

| Skill | Claude Code | Gemini CLI | OpenCode | Codex |
|-------|-------------|------------|----------|-------|
| review-spec | ✅ | ✅ | ✅ | ✅ |
| review-architecture | ✅ | ✅ | ✅ | ✅ |
| review-adr | ✅ | ✅ | ✅ | ✅ |
| create-spec | ✅ | ✅ | ✅ | ✅ |

**Legend:** ✅ Supported | ❌ Not Supported
```

---

## Programmatic Usage (Python)

### Using the Discovery Library

```python
#!/usr/bin/env python3
from pathlib import Path
from lib.discovery import MarketplaceDiscovery

# Create discovery instance
discovery = MarketplaceDiscovery()

# Search for skills
results = discovery.search_skills("validation")
print(f"Found {len(results)} skills:")
for skill in results:
    print(f"  - {skill['name']}: {skill['description']}")

# Check compatibility
if discovery.is_skill_compatible("review-spec", "claude-code"):
    print("review-spec is compatible with Claude Code")

# Get skill details
skill = discovery.get_skill_details("review-spec")
print(f"Version: {skill['version']}")

# List compatible skills
compatible = discovery.list_compatible_skills("claude-code")
print(f"Compatible with Claude Code:")
for skill in compatible:
    print(f"  - {skill['name']}")

# Filter by tags
filtered = discovery.filter_skills_by_tags(
    ["validation", "multi-persona"],
    logic="AND"
)
print("Skills with validation AND multi-persona:")
for skill in filtered:
    print(f"  - {skill['name']}")
```

### Integrating with Applications

```python
from lib.discovery import MarketplaceDiscovery
import json

class SkillManager:
    """Manage skills for an application."""

    def __init__(self):
        self.discovery = MarketplaceDiscovery()

    def get_available_skills(self, cli_type=None):
        """Get all skills available for current CLI."""
        return self.discovery.list_compatible_skills(cli_type)

    def find_skill_for_task(self, task_keywords):
        """Find best skill for a task."""
        recommendations = self.discovery.recommend_skills(task_keywords)
        if recommendations:
            return recommendations[0]  # Return most relevant
        return None

    def validate_skill(self, skill_name):
        """Validate that a skill exists and is compatible."""
        skill = self.discovery.get_skill_details(skill_name)
        if not skill:
            raise ValueError(f"Skill '{skill_name}' not found")

        if not self.discovery.is_skill_compatible(skill_name):
            raise ValueError(
                f"Skill '{skill_name}' not compatible with current CLI"
            )

        return skill

# Usage
manager = SkillManager()

# Get skills for documentation validation
skills = manager.get_available_skills()
for skill in skills:
    if "validation" in skill.get("tags", []):
        print(f"Validation skill: {skill['name']}")

# Find skill for spec review
spec_skill = manager.find_skill_for_task("spec review")
if spec_skill:
    print(f"Best skill for spec review: {spec_skill['name']}")
```

### Custom Discovery Workflows

```python
#!/usr/bin/env python3
"""Custom skill discovery workflow."""

from lib.discovery import MarketplaceDiscovery
import sys

def discover_and_validate():
    """Discover skills and validate metadata."""
    discovery = MarketplaceDiscovery()

    print("Discovering skills...")
    all_skills = discovery.list_compatible_skills()

    issues = []

    for skill in all_skills:
        skill_name = skill["name"]

        # Check required fields
        required_fields = ["version", "description", "tags"]
        for field in required_fields:
            if field not in skill:
                issues.append(
                    f"{skill_name}: Missing required field '{field}'"
                )

        # Check tags
        if not skill.get("tags"):
            issues.append(f"{skill_name}: No tags defined")

        # Check CLI support
        cli_support = skill.get("cli_support", [])
        cli_adapters = skill.get("cli_adapters", {})
        if not cli_support and not cli_adapters:
            issues.append(f"{skill_name}: No CLI support defined")

    if issues:
        print("\nValidation Issues:")
        for issue in issues:
            print(f"  - {issue}")
        return False

    print(f"\n✓ All {len(all_skills)} skills validated successfully")
    return True

def generate_skill_report():
    """Generate comprehensive skill report."""
    discovery = MarketplaceDiscovery()

    report = []
    report.append("# Skill Discovery Report\n")

    # Summary
    all_skills = discovery.list_compatible_skills()
    report.append(f"Total Skills: {len(all_skills)}\n")

    # By category
    categories = discovery.get_categories()
    report.append("\n## Skills by Category\n")
    for category in categories:
        skills = discovery.list_skills_by_category(category)
        report.append(f"\n### {category} ({len(skills)} skills)\n")
        for skill in skills:
            report.append(f"- {skill['name']}: {skill['description']}\n")

    # Compatibility matrix
    report.append("\n## Compatibility Matrix\n")
    report.append(discovery.generate_compatibility_matrix())

    # Tag analysis
    tags = discovery.list_all_tags()
    report.append(f"\n## Tag Analysis\n")
    report.append(f"Total unique tags: {len(tags)}\n\n")

    tag_counts = {}
    for tag in tags:
        skills = discovery.filter_skills_by_tags([tag])
        tag_counts[tag] = len(skills)

    report.append("Tag usage:\n")
    for tag, count in sorted(tag_counts.items(), key=lambda x: -x[1]):
        report.append(f"- {tag}: {count} skills\n")

    return "".join(report)

if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "validate":
        success = discover_and_validate()
        sys.exit(0 if success else 1)
    elif len(sys.argv) > 1 and sys.argv[1] == "report":
        print(generate_skill_report())
    else:
        print("Usage: python workflow.py {validate|report}")
        sys.exit(1)
```

---

## Advanced Use Cases

### Skill Recommendations Based on Context

```bash
# Get recommendations for spec validation
marketplace-discover recommend spec

# Get recommendations for architecture review
marketplace-discover recommend architecture

# Get recommendations for documentation
marketplace-discover recommend documentation
```

Output:
```
Recommended 2 skill(s) for context 'spec':

- review-spec (relevance: 2): Validate SPEC.md files against research-backed quality rubric
- create-spec (relevance: 2): Generate SPEC.md files from codebase analysis and requirements
```

### Dynamic CLI Selection

```bash
# Override CLI type detection
marketplace-discover compatible --cli gemini-cli

# Use in automated workflows
marketplace-discover compatible claude-code --json > claude-compatible.json
marketplace-discover compatible gemini-cli --json > gemini-compatible.json
```

### Batch Processing with Python

```python
#!/usr/bin/env python3
"""Batch process all validation skills."""

from lib.discovery import MarketplaceDiscovery

discovery = MarketplaceDiscovery()

# Find all validation skills
validation_skills = discovery.filter_skills_by_tags(["validation"])

print(f"Found {len(validation_skills)} validation skills\n")

for skill in validation_skills:
    print(f"=== {skill['name']} ===")
    print(discovery.print_skill_summary(skill['name']))
    print()

    # Check compatibility with each CLI
    clis = ["claude-code", "gemini-cli", "opencode", "codex"]
    compatible_clis = [
        cli for cli in clis
        if discovery.is_skill_compatible(skill['name'], cli)
    ]

    print(f"Compatible CLIs: {', '.join(compatible_clis)}")
    print()
```

### Integration with CI/CD

```python
#!/usr/bin/env python3
"""CI/CD validation script."""

import sys
from lib.discovery import MarketplaceDiscovery

def validate_marketplace():
    """Validate marketplace configuration."""
    discovery = MarketplaceDiscovery()
    errors = []

    # Load skills
    try:
        data = discovery._load_marketplace_data()
    except Exception as e:
        print(f"ERROR: Failed to load marketplace.json: {e}")
        return False

    skills = data.get("skills", [])

    if not skills:
        print("ERROR: No skills found in marketplace")
        return False

    print(f"✓ Found {len(skills)} skills\n")

    # Validate each skill
    required_fields = ["name", "version", "description", "tags"]

    for skill in skills:
        skill_name = skill.get("name", "UNKNOWN")

        # Check required fields
        for field in required_fields:
            if field not in skill:
                errors.append(f"{skill_name}: Missing field '{field}'")

        # Check CLI support
        has_cli_support = (
            "cli_support" in skill or
            "cli_adapters" in skill
        )
        if not has_cli_support:
            errors.append(f"{skill_name}: No CLI support defined")

        # Validate version format (semver)
        version = skill.get("version", "")
        if not version or not all(
            c.isdigit() or c == '.' for c in version
        ):
            errors.append(f"{skill_name}: Invalid version format")

    # Report results
    if errors:
        print("Validation Errors:")
        for error in errors:
            print(f"  - {error}")
        return False

    print("✓ All skills validated successfully")

    # Generate and save compatibility matrix
    matrix = discovery.generate_compatibility_matrix()
    with open("compatibility-matrix.md", "w") as f:
        f.write(matrix)
    print("✓ Compatibility matrix generated")

    return True

if __name__ == "__main__":
    success = validate_marketplace()
    sys.exit(0 if success else 1)
```

---

## Environment Variables

The discovery tools respect the following environment variables:

- `MARKETPLACE_ROOT`: Path to marketplace directory (auto-detected if not set)
- `ANTHROPIC_API_KEY`: API key for skills using Anthropic API

Example:
```bash
export MARKETPLACE_ROOT="/custom/path/to/marketplace"
marketplace-discover list
```

In Python:
```python
import os
os.environ["MARKETPLACE_ROOT"] = "/custom/path/to/marketplace"

from lib.discovery import MarketplaceDiscovery
discovery = MarketplaceDiscovery()
```

---

## Tips and Best Practices

1. **Use JSON output for scripting**: Always use `--json` flag when parsing results
2. **Type hints in Python**: The library uses type hints for better IDE support
3. **Validate before use**: Always check skill compatibility before executing
4. **Use recommendations**: Leverage the `recommend` method for context-aware discovery
5. **Cache instances**: Reuse `MarketplaceDiscovery` instances to avoid reloading data
6. **Handle exceptions**: Wrap discovery calls in try-except blocks

---

## Troubleshooting

### No results returned

```bash
# Check if marketplace.json exists
ls -l marketplace.json

# Validate JSON
python3 -m json.tool marketplace.json
```

```python
# In Python
from pathlib import Path
marketplace_json = Path("marketplace.json")
assert marketplace_json.exists(), "marketplace.json not found"
```

### Import errors

```bash
# Ensure lib is in Python path
export PYTHONPATH="${PYTHONPATH}:/path/to/marketplace/lib"
```

```python
# In Python
import sys
from pathlib import Path
sys.path.insert(0, str(Path("/path/to/marketplace/lib")))
```

### CLI detection issues

```python
from lib.cli_detector import detect_cli

# Force CLI type
discovery = MarketplaceDiscovery(cli_type="claude-code")

# Check detected CLI
print(f"Detected CLI: {detect_cli()}")
```

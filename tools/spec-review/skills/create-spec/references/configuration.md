# Configuration

## Template Customization

Create custom template at `templates/my-template.md`:

```markdown
# {{project_name}} Specification

## Overview
{{what_is_this}}

## Problem
{{problem_statement}}

## Users
{{#personas}}
- {{persona_name}}: {{demographics}}
{{/personas}}

...
```

Use with:
```bash
python create_spec.py /path/to/project --template templates/my-template.md
```

## Quality Thresholds

Modify `rubrics/spec-quality-rubric.yml`:

```yaml
decision_thresholds:
  pass: 8.0    # Minimum score to pass
  warn: 6.0    # Warning threshold
  fail: 0.0    # Failure threshold
```

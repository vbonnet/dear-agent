# Custom Personas Example

This example demonstrates how to create and use custom review personas tailored to your project's specific needs.

## Overview

This example shows how to:
- Create a custom persona using the `.ai.md` format
- Define specialized review criteria
- Use custom personas alongside built-in ones
- Optimize personas for prompt caching

## Files

- `custom-security-expert.ai.md` - Example custom persona for cloud security
- `usage-example.sh` - Shell script demonstrating custom persona usage
- `README.md` - This file with persona creation guide

## Persona File Format

Multi-persona-review uses the `.ai.md` format with YAML frontmatter:

```yaml
---
name: persona-identifier        # Required: kebab-case identifier
displayName: Human Readable     # Required: Display name
version: 1.0.0                  # Required: Semver version
description: Brief description  # Required: What this persona does
focusAreas:                     # Required: Array of focus areas
  - area-1
  - area-2
severityLevels:                 # Optional: Preferred severity levels
  - critical
  - high
gitHistoryAccess: false         # Optional: Can request git history
---

# Persona Name

[Markdown content describing the persona's expertise, review checklist,
examples, and output format. This becomes the persona's system prompt.]
```

## Creating a Custom Persona

### 1. Choose a Name

Use kebab-case for the `name` field:
- ✅ Good: `cloud-security-specialist`, `graphql-expert`, `accessibility-reviewer`
- ❌ Bad: `CloudSecuritySpecialist`, `cloud_security`, `cloud security`

### 2. Define Focus Areas

Focus areas help categorize findings:

```yaml
focusAreas:
  - aws-iam-policies
  - s3-bucket-security
  - vpc-configuration
  - secrets-management
```

### 3. Write the Persona Prompt

The markdown body should include:

1. **Expertise** - Background and specialization
2. **Review Process** - Step-by-step approach
3. **Checklist** - Specific items to review
4. **Examples** - Good vs bad code patterns
5. **Output Format** - Expected JSON structure

### 4. Optimize for Caching

To maximize prompt caching benefits (86% cost savings):

- **Minimum size:** 1,024 tokens (~4,096 characters)
- **Keep static:** Avoid dynamic content like `{{DATE}}`
- **Add examples:** Include code samples to reach minimum size
- **Test tokens:** Use `echo "scale=0; $(wc -c < persona.ai.md) / 4" | bc`

## Persona Search Paths

Personas are loaded from these locations (in priority order):

1. `~/.wayfinder/personas/` - User-level overrides
2. `.wayfinder/personas/` - Project-specific personas
3. `{company}/personas/` - Company-level (if configured)
4. Built-in personas library

### Using Project-Specific Personas

Create a `.wayfinder/personas/` directory in your project:

```bash
mkdir -p .wayfinder/personas
cp custom-security-expert.ai.md .wayfinder/personas/
```

### Using User-Level Personas

Place personas in your home directory:

```bash
mkdir -p ~/.wayfinder/personas
cp custom-security-expert.ai.md ~/.wayfinder/personas/
```

## Usage Examples

### Use Custom Persona Only

```bash
multi-persona-review src/ \
  --personas cloud-security-specialist
```

### Mix Custom and Built-in Personas

```bash
multi-persona-review src/ \
  --personas cloud-security-specialist,security-engineer,code-health
```

### List Available Personas

```bash
multi-persona-review --list-personas
```

This will show:
- Built-in personas from the library
- Custom personas from `.wayfinder/personas/`
- User personas from `~/.wayfinder/personas/`

## Example Custom Persona

See `custom-security-expert.ai.md` for a complete example of a cloud security specialist persona that:

- Focuses on AWS-specific security concerns
- Reviews IAM policies, S3 configurations, VPC settings
- Checks for compliance with security frameworks (CIS, SOC2)
- Optimized for caching (1,500+ tokens)

## Advanced: Persona Versioning

Use semantic versioning to track persona changes:

```yaml
version: 1.0.0  # Initial release
version: 1.1.0  # Minor update: Added new checklist items
version: 2.0.0  # Major update: Changed review approach
```

Version changes help track when personas are updated and invalidate caches appropriately.

## Advanced: Environment-Specific Personas

Override personas path with environment variable:

```bash
# Use company-wide personas
export WAYFINDER_PERSONAS_PATH=/opt/company/personas
multi-persona-review src/

# Use project-specific path
export WAYFINDER_PERSONAS_PATH=./config/personas
multi-persona-review src/
```

## Validation

Test your custom persona:

```bash
# Dry run to validate configuration
multi-persona-review src/ \
  --personas cloud-security-specialist \
  --dry-run \
  --verbose

# Run on a small test file first
multi-persona-review test-file.ts \
  --personas cloud-security-specialist
```

## Best Practices

1. **Be Specific** - Focus on a narrow domain for best results
2. **Include Examples** - Show good and bad code patterns
3. **Document Output** - Clearly specify expected JSON format
4. **Test Thoroughly** - Run on sample code before production use
5. **Version Control** - Track personas in git
6. **Optimize Size** - Reach 1,024 tokens for caching benefits
7. **Avoid Dynamic Content** - Keep prompts static for cache efficiency

## Troubleshooting

### Issue: Custom persona not found

**Solution:** Check persona search paths:
```bash
multi-persona-review --list-personas
```

Ensure your persona file:
- Is in a valid search path
- Has `.ai.md` extension
- Has valid YAML frontmatter

### Issue: Persona validation failed

**Solution:** Verify required fields:
- `name` (kebab-case string)
- `displayName` (human-readable string)
- `version` (semver format)
- `description` (brief text)
- `focusAreas` (array of strings)

### Issue: Poor caching performance

**Solution:** Optimize persona size:
```bash
# Check token count (should be ≥1,024)
wc -c < custom-security-expert.ai.md | awk '{print int($1/4)}'

# Add more content if needed:
# - Detailed examples
# - Extended checklists
# - Background information
```

## Next Steps

- See `../basic-usage/` for CLI usage basics
- See `../ci-cd-integration/` for automated reviews
- See `../programmatic-api/` for library integration
- Read [Persona Optimization Guide](../../docs/persona-optimization-guide.md) for advanced tuning

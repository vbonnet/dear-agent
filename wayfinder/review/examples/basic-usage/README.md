# Basic Usage Example

This example demonstrates the simplest way to use the multi-persona-review CLI tool.

## Overview

This example shows how to:
- Run a quick code review on a single file
- Use default personas
- View colored text output with findings and cost information

## Files

- `sample-code.ts` - A simple TypeScript file with some code quality issues
- `run.sh` - Shell script to execute the review

## Prerequisites

1. Install the plugin:
   ```bash
   cd engram/plugins/multi-persona-review
   npm install
   npm run build
   ```

2. Set up your API key (choose one):

   **Option A: Anthropic (Claude) - Direct API**
   ```bash
   export ANTHROPIC_API_KEY=your_api_key_here
   ```

   **Option B: VertexAI (Gemini)**
   ```bash
   export VERTEX_PROJECT_ID=your_gcp_project_id
   export VERTEX_LOCATION=us-central1  # Optional
   ```

   **Option C: VertexAI (Claude) - Recommended**
   ```bash
   export VERTEX_PROJECT_ID=your_gcp_project_id
   export VERTEX_LOCATION=us-east5
   export VERTEX_MODEL=claude-sonnet-4-5@20250929
   ```

## Usage

### Quick Review (Default)

```bash
chmod +x run.sh
./run.sh
```

This will:
1. Run a quick review using default personas
2. Display findings with color-coded severity levels
3. Show cost breakdown and summary statistics

### Custom Review Modes

```bash
# Thorough review with more personas
npx multi-persona-review sample-code.ts --mode thorough

# Specific personas only
npx multi-persona-review sample-code.ts --personas security-engineer,code-health

# JSON output for programmatic use
npx multi-persona-review sample-code.ts --format json

# No colors (for CI/CD or logging)
npx multi-persona-review sample-code.ts --no-colors

# Disable deduplication to see all findings
npx multi-persona-review sample-code.ts --no-dedupe
```

## Expected Output

The review will identify issues such as:
- Missing error handling
- Hardcoded credentials
- Lack of input validation
- Type safety concerns
- Performance inefficiencies

Example output:
```
┌─────────────────────────────────────────────────────────────┐
│ 🔍 Multi-Persona Code Review Results                        │
└─────────────────────────────────────────────────────────────┘

📁 sample-code.ts

  🔴 CRITICAL: Hardcoded API Key (security-engineer)
     Line 5: API key stored directly in code
     Recommendation: Use environment variables or secrets manager

  🟡 MEDIUM: Missing Error Handling (error-handling-specialist)
     Line 12: API call without try-catch block
     Recommendation: Add proper error handling and logging

  🟡 MEDIUM: No Input Validation (security-engineer)
     Line 8: User input used without sanitization
     Recommendation: Validate and sanitize all user inputs

───────────────────────────────────────────────────────────────
Summary:
  Files Reviewed: 1
  Total Findings: 3
  Critical: 1, High: 0, Medium: 2, Low: 0

Cost: $0.023 (3 personas, 2,456 tokens)
Time: 1.2s
───────────────────────────────────────────────────────────────
```

## Next Steps

- See `../ci-cd-integration/` for GitHub Actions workflow example
- See `../custom-personas/` to learn how to create custom personas
- See `../programmatic-api/` for TypeScript library usage

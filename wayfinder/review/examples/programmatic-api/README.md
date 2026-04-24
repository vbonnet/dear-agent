# Programmatic API Example

This example demonstrates how to use multi-persona-review as a library in your TypeScript/JavaScript applications.

## Overview

This example shows how to:
- Import and use the review engine programmatically
- Create custom reviewers with different AI providers
- Process results and integrate with your application
- Build automated code review tools

## Files

- `index.ts` - Complete example using the library API
- `package.json` - Minimal dependencies for the example
- `README.md` - This file with API documentation

## Installation

### 1. Install the plugin

```bash
npm install @wayfinder/multi-persona-review
```

### 2. Install dependencies

```bash
cd examples/programmatic-api
npm install
```

### 3. Build the example

```bash
npx tsc index.ts
```

### 4. Run the example

```bash
# Using Anthropic (Claude)
export ANTHROPIC_API_KEY=your_key
node index.js

# Using VertexAI (Gemini)
export VERTEX_PROJECT_ID=your_project
node index.js

# Using VertexAI (Claude)
export VERTEX_PROJECT_ID=your_project
export VERTEX_MODEL=claude-sonnet-4-5@20250929
node index.js
```

## API Overview

### Core Functions

```typescript
import {
  // Persona Management
  loadPersonas,
  loadSpecificPersonas,

  // Review Engine
  reviewFiles,
  prepareContext,

  // AI Providers
  createAnthropicReviewer,
  createVertexAIReviewer,
  createVertexAIClaudeReviewer,

  // Output Formatting
  formatReviewResult,
  formatReviewResultJSON,
  formatReviewResultGitHub,

  // Utilities
  deduplicateFindings,
  createCostSink,
} from '@wayfinder/multi-persona-review';
```

## Usage Examples

### Example 1: Basic Review (Anthropic)

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  formatReviewResult,
} from '@wayfinder/multi-persona-review';

async function runReview() {
  // 1. Load personas
  const personas = await loadPersonas([
    '.wayfinder/personas',
    process.env.HOME + '/.wayfinder/personas',
  ]);

  const securityPersona = personas.get('security-engineer')!;
  const codeHealthPersona = personas.get('code-health')!;

  // 2. Create reviewer
  const reviewer = createAnthropicReviewer({
    apiKey: process.env.ANTHROPIC_API_KEY!,
    model: 'claude-3-5-sonnet-20241022',
  });

  // 3. Run review
  const result = await reviewFiles(
    {
      files: ['src/index.ts', 'src/utils.ts'],
      personas: [securityPersona, codeHealthPersona],
      mode: 'thorough',
      fileScanMode: 'full',
    },
    process.cwd(),
    reviewer
  );

  // 4. Format and display
  const output = formatReviewResult(result, {
    colors: true,
    groupByFile: true,
    showCost: true,
    showSummary: true,
  });

  console.log(output);

  // 5. Process results
  const criticalFindings = result.findings.filter(
    f => f.severity === 'critical'
  );

  if (criticalFindings.length > 0) {
    console.error(`Found ${criticalFindings.length} critical issues!`);
    process.exit(1);
  }
}
```

### Example 2: Using VertexAI (Gemini)

```typescript
import {
  loadPersonas,
  reviewFiles,
  createVertexAIReviewer,
  formatReviewResult,
} from '@wayfinder/multi-persona-review';

async function runReviewWithVertexAI() {
  const personas = await loadPersonas(['.wayfinder/personas']);

  const reviewer = createVertexAIReviewer({
    projectId: process.env.VERTEX_PROJECT_ID!,
    location: 'us-central1',
    model: 'gemini-2.5-flash',
  });

  const result = await reviewFiles(
    {
      files: ['src/'],
      personas: Array.from(personas.values()).slice(0, 3),
      mode: 'quick',
      fileScanMode: 'changed',
    },
    process.cwd(),
    reviewer
  );

  console.log(formatReviewResult(result));
}
```

### Example 3: Using VertexAI (Claude)

```typescript
import {
  loadPersonas,
  reviewFiles,
  createVertexAIClaudeReviewer,
  formatReviewResult,
} from '@wayfinder/multi-persona-review';

async function runReviewWithVertexAIClaude() {
  const personas = await loadPersonas(['.wayfinder/personas']);

  const reviewer = createVertexAIClaudeReviewer({
    projectId: process.env.VERTEX_PROJECT_ID!,
    location: 'us-east5',
    model: 'claude-sonnet-4-5@20250929',
  });

  const result = await reviewFiles(
    {
      files: ['src/'],
      personas: Array.from(personas.values()),
      mode: 'thorough',
      fileScanMode: 'full',
    },
    process.cwd(),
    reviewer
  );

  console.log(formatReviewResult(result));
}
```

### Example 4: JSON Output for Integration

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  formatReviewResultJSON,
} from '@wayfinder/multi-persona-review';
import { writeFile } from 'fs/promises';

async function exportReviewJSON() {
  const personas = await loadPersonas(['.wayfinder/personas']);
  const reviewer = createAnthropicReviewer({
    apiKey: process.env.ANTHROPIC_API_KEY!,
  });

  const result = await reviewFiles(
    {
      files: ['src/'],
      personas: Array.from(personas.values()),
      mode: 'thorough',
      fileScanMode: 'diff',
    },
    process.cwd(),
    reviewer
  );

  const jsonOutput = formatReviewResultJSON(result);
  await writeFile('review-results.json', jsonOutput, 'utf-8');

  console.log('Results saved to review-results.json');
}
```

### Example 5: Custom Error Handling

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  ReviewEngineError,
  AnthropicClientError,
} from '@wayfinder/multi-persona-review';

async function robustReview() {
  try {
    const personas = await loadPersonas(['.wayfinder/personas']);
    const reviewer = createAnthropicReviewer({
      apiKey: process.env.ANTHROPIC_API_KEY!,
    });

    const result = await reviewFiles(
      {
        files: ['src/'],
        personas: Array.from(personas.values()),
        mode: 'thorough',
        fileScanMode: 'full',
      },
      process.cwd(),
      reviewer
    );

    return result;

  } catch (error) {
    if (error instanceof AnthropicClientError) {
      console.error('API Error:', error.message);
      // Handle rate limits, quota issues, etc.
    } else if (error instanceof ReviewEngineError) {
      console.error('Review Error:', error.message);
      // Handle review-specific errors
    } else {
      console.error('Unexpected error:', error);
    }
    throw error;
  }
}
```

### Example 6: Cost Tracking

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  createCostSink,
} from '@wayfinder/multi-persona-review';

async function reviewWithCostTracking() {
  const personas = await loadPersonas(['.wayfinder/personas']);
  const reviewer = createAnthropicReviewer({
    apiKey: process.env.ANTHROPIC_API_KEY!,
  });

  // Create cost sink
  const costSink = createCostSink({
    type: 'file',
    filePath: './costs.jsonl',
  });

  const result = await reviewFiles(
    {
      files: ['src/'],
      personas: Array.from(personas.values()),
      mode: 'thorough',
      fileScanMode: 'full',
      costSink,
    },
    process.cwd(),
    reviewer
  );

  console.log(`Total cost: $${result.cost.toFixed(3)}`);
  console.log('Cost details written to costs.jsonl');
}
```

### Example 7: Building a Custom Review Tool

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  formatReviewResultGitHub,
  type ReviewResult,
} from '@wayfinder/multi-persona-review';
import { execSync } from 'child_process';
import { writeFile } from 'fs/promises';

class CustomReviewTool {
  private personas: Map<string, Persona>;
  private reviewer: ReviewerFunction;

  async initialize() {
    this.personas = await loadPersonas(['.wayfinder/personas']);
    this.reviewer = createAnthropicReviewer({
      apiKey: process.env.ANTHROPIC_API_KEY!,
      model: 'claude-3-5-sonnet-20241022',
    });
  }

  async reviewPullRequest(prNumber: number): Promise<ReviewResult> {
    // Get changed files from git
    const diffOutput = execSync(
      `git diff origin/main...HEAD --name-only`
    ).toString();

    const changedFiles = diffOutput
      .split('\n')
      .filter(f => f.endsWith('.ts') || f.endsWith('.js'));

    // Run review
    const result = await reviewFiles(
      {
        files: changedFiles,
        personas: Array.from(this.personas.values()),
        mode: 'thorough',
        fileScanMode: 'diff',
      },
      process.cwd(),
      this.reviewer
    );

    // Format as GitHub comment
    const comment = formatReviewResultGitHub(result);

    // Save to file for GitHub Actions
    await writeFile('pr-comment.md', comment, 'utf-8');

    return result;
  }

  async reviewCommit(commitSha: string): Promise<ReviewResult> {
    // Get files changed in commit
    const diffOutput = execSync(
      `git diff-tree --no-commit-id --name-only -r ${commitSha}`
    ).toString();

    const changedFiles = diffOutput.split('\n').filter(f => f.length > 0);

    return await reviewFiles(
      {
        files: changedFiles,
        personas: Array.from(this.personas.values()),
        mode: 'quick',
        fileScanMode: 'changed',
      },
      process.cwd(),
      this.reviewer
    );
  }
}

// Usage
async function main() {
  const tool = new CustomReviewTool();
  await tool.initialize();

  const result = await tool.reviewPullRequest(123);

  console.log(`Review complete: ${result.findings.length} findings`);
  console.log(`Cost: $${result.cost.toFixed(3)}`);
}
```

## API Reference

### Types

```typescript
// Review configuration
interface ReviewConfig {
  files: string[];
  personas: Persona[];
  mode: 'quick' | 'thorough' | 'custom';
  fileScanMode: 'full' | 'diff' | 'changed';
  options?: {
    deduplicate?: boolean;
    similarityThreshold?: number;
  };
  costSink?: CostSink;
}

// Review result
interface ReviewResult {
  findings: Finding[];
  summary: {
    total: number;
    bySeverity: Record<Severity, number>;
    byPersona: Record<string, number>;
  };
  cost: number;
  timing: {
    start: Date;
    end: Date;
    durationMs: number;
  };
}

// Finding
interface Finding {
  severity: 'critical' | 'high' | 'medium' | 'low';
  category: string;
  title: string;
  file: string;
  line?: number;
  message: string;
  recommendation: string;
  persona: string;
}
```

### Functions

#### `loadPersonas(searchPaths: string[]): Promise<Map<string, Persona>>`
Load personas from specified directories.

#### `reviewFiles(config: ReviewConfig, cwd: string, reviewer: ReviewerFunction): Promise<ReviewResult>`
Run a multi-persona code review.

#### `createAnthropicReviewer(config: AnthropicClientConfig): ReviewerFunction`
Create a reviewer using Anthropic's Claude API.

#### `createVertexAIReviewer(config: VertexAIClientConfig): ReviewerFunction`
Create a reviewer using Google VertexAI (Gemini).

#### `createVertexAIClaudeReviewer(config: VertexAIClaudeClientConfig): ReviewerFunction`
Create a reviewer using VertexAI with Claude models.

#### `formatReviewResult(result: ReviewResult, options?: TextFormatOptions): string`
Format results as colored terminal text.

#### `formatReviewResultJSON(result: ReviewResult): string`
Format results as JSON.

#### `formatReviewResultGitHub(result: ReviewResult): string`
Format results as GitHub-flavored Markdown.

## Error Handling

```typescript
import {
  ReviewEngineError,
  AnthropicClientError,
  VertexAIClientError,
  PersonaError,
  FileScannerError,
} from '@wayfinder/multi-persona-review';

try {
  // Your review code
} catch (error) {
  if (error instanceof AnthropicClientError) {
    // Handle API errors (rate limits, auth, etc.)
  } else if (error instanceof PersonaError) {
    // Handle persona loading/validation errors
  } else if (error instanceof FileScannerError) {
    // Handle file scanning errors
  }
}
```

## Best Practices

1. **Cache personas** - Load once and reuse across multiple reviews
2. **Handle errors** - Implement proper error handling and retries
3. **Track costs** - Use cost sinks to monitor API spending
4. **Optimize scan mode** - Use 'diff' for PRs, 'full' for releases
5. **Deduplicate** - Enable deduplication to reduce noise
6. **Choose provider** - Use VertexAI Claude for better GCP integration

## Next Steps

- See `../basic-usage/` for CLI usage
- See `../ci-cd-integration/` for GitHub Actions setup
- See `../custom-personas/` for creating custom personas
- Read the [API documentation](../../DOCUMENTATION.md) for complete reference

# API Documentation

Multi-Persona Review provides a comprehensive programmatic API for integrating code reviews into your applications.

## Table of Contents

- [Installation](#installation)
- [Core Functions](#core-functions)
- [Types](#types)
- [Persona Management](#persona-management)
- [Review Execution](#review-execution)
- [Formatters](#formatters)
- [Cost Tracking](#cost-tracking)
- [Configuration](#configuration)
- [Examples](#examples)

---

## Installation

```bash
npm install @wayfinder/multi-persona-review
```

## Core Functions

### `reviewFiles(config, rootDir, reviewer)`

Main entry point for executing reviews.

**Parameters**:
- `config: ReviewConfig` - Review configuration
- `rootDir: string` - Root directory path
- `reviewer: ReviewerFunction` - AI reviewer function

**Returns**: `Promise<ReviewResult>`

**Example**:
```typescript
import { reviewFiles, createAnthropicReviewer } from '@wayfinder/multi-persona-review';

const reviewer = createAnthropicReviewer({
  apiKey: process.env.ANTHROPIC_API_KEY!,
  model: 'claude-3-5-sonnet-20241022',
});

const result = await reviewFiles(
  {
    files: ['src/index.ts'],
    personas: [securityPersona],
    mode: 'thorough',
    options: {
      voteThreshold: 0.7,
      minConfidence: 0.8,
    },
  },
  process.cwd(),
  reviewer
);
```

---

## Types

### `ReviewConfig`

Configuration for review execution.

```typescript
interface ReviewConfig {
  files: string[];                // Files to review
  personas: Persona[];            // Personas to use
  mode: ReviewMode;               // Review mode
  fileScanMode?: FileScanMode;    // Scan mode (full/diff/changed)
  options?: ReviewOptions;        // Additional options
}
```

### `ReviewOptions`

Optional review configuration.

```typescript
interface ReviewOptions {
  deduplicate?: boolean;          // Enable deduplication (default: true)
  similarityThreshold?: number;   // Similarity threshold (0-1, default: 0.8)
  parallel?: boolean;             // Parallel execution (default: true)
  maxConcurrency?: number;        // Max parallel reviews (default: 3)
  voteThreshold?: number;         // Agency-Agents: Vote threshold (default: 0.5)
  minConfidence?: number;         // Agency-Agents: Min confidence (0-1)
  temperature?: number;           // LLM temperature (default: 0.3)
  maxTokens?: number;             // Max output tokens (default: 4096)
}
```

### `Persona`

Persona definition.

```typescript
interface Persona {
  name: string;                   // Unique identifier (kebab-case)
  displayName: string;            // Human-readable name
  version: string;                // Semver version
  description: string;            // Brief description
  focusAreas: string[];           // Focus areas
  prompt: string;                 // System prompt
  tier?: 1 | 2 | 3;              // Agency-Agents: Vote weight
  severityLevels?: Severity[];    // Preferred severity levels
}
```

### `Finding`

Review finding.

```typescript
interface Finding {
  file: string;                   // File path
  line?: number;                  // Line number
  severity: Severity;             // critical/high/medium/low/info
  category: string;               // Finding category
  message: string;                // Description
  suggestion?: string;            // Fix suggestion
  code?: string;                  // Code snippet
  confidence?: number;            // 0-1 confidence score
  persona: string;                // Persona name

  // Agency-Agents fields
  decision?: 'GO' | 'NO-GO';      // Voting decision
  alternatives?: string[];        // Alternative approaches
  outOfScope?: boolean;           // Outside expertise
}
```

### `ReviewResult`

Review result with findings and metadata.

```typescript
interface ReviewResult {
  findings: Finding[];            // All findings
  personas: string[];             // Personas executed
  files: string[];                // Files reviewed
  cost: CostData;                 // Cost tracking data
  duration: number;               // Duration in milliseconds
  metadata?: Record<string, any>; // Additional metadata
}
```

---

## Persona Management

### `loadPersonas(searchPaths)`

Load personas from file system.

**Parameters**:
- `searchPaths: string[]` - Directories to search for personas

**Returns**: `Promise<Map<string, Persona>>`

**Example**:
```typescript
import { loadPersonas } from '@wayfinder/multi-persona-review';

const personas = await loadPersonas([
  '.engram/personas',
  '~/.engram/personas',
  '/opt/company/personas',
]);

const securityPersona = personas.get('security-engineer');
```

### `loadPersonasFromDir(dir)`

Load all personas from a directory.

**Parameters**:
- `dir: string` - Directory path

**Returns**: `Promise<Persona[]>`

### `parsePersonaFile(filePath)`

Parse a single persona file (.ai.md or .yaml).

**Parameters**:
- `filePath: string` - Path to persona file

**Returns**: `Promise<Persona>`

---

## Review Execution

### `createAnthropicReviewer(config)`

Create Anthropic Claude reviewer.

**Parameters**:
- `config: AnthropicConfig` - Anthropic configuration

**Returns**: `ReviewerFunction`

**Example**:
```typescript
import { createAnthropicReviewer } from '@wayfinder/multi-persona-review';

const reviewer = createAnthropicReviewer({
  apiKey: process.env.ANTHROPIC_API_KEY!,
  model: 'claude-3-5-sonnet-20241022',
  temperature: 0.3,
  maxTokens: 4096,
});
```

### `createVertexAIReviewer(config)`

Create Vertex AI Gemini reviewer.

**Parameters**:
- `config: VertexAIConfig` - Vertex AI configuration

**Returns**: `ReviewerFunction`

**Example**:
```typescript
import { createVertexAIReviewer } from '@wayfinder/multi-persona-review';

const reviewer = createVertexAIReviewer({
  projectId: process.env.VERTEX_PROJECT_ID!,
  location: 'us-central1',
  model: 'gemini-2.5-flash',
});
```

### `createVertexAIClaudeReviewer(config)`

Create Vertex AI Claude reviewer (recommended for GCP).

**Parameters**:
- `config: VertexAIClaudeConfig` - Vertex AI Claude configuration

**Returns**: `ReviewerFunction`

**Example**:
```typescript
import { createVertexAIClaudeReviewer } from '@wayfinder/multi-persona-review';

const reviewer = createVertexAIClaudeReviewer({
  projectId: process.env.VERTEX_PROJECT_ID!,
  location: 'us-east5',
  model: 'claude-sonnet-4-5@20250929',
});
```

---

## Formatters

### `formatReviewResult(result, options)`

Format review results for display.

**Parameters**:
- `result: ReviewResult` - Review result
- `options: FormatOptions` - Formatting options

**Returns**: `string`

**Example**:
```typescript
import { formatReviewResult } from '@wayfinder/multi-persona-review';

const formatted = formatReviewResult(result, {
  colors: true,
  groupByFile: true,
  showCost: true,
  showSummary: true,
  format: 'text', // or 'json' or 'github'
});

console.log(formatted);
```

### `FormatOptions`

Formatting configuration.

```typescript
interface FormatOptions {
  colors?: boolean;               // Enable ANSI colors (default: true)
  groupByFile?: boolean;          // Group by file (default: true)
  showCost?: boolean;             // Show cost data (default: true)
  showSummary?: boolean;          // Show summary (default: true)
  format?: 'text' | 'json' | 'github'; // Output format
  flat?: boolean;                 // Flat format (default: false)
}
```

---

## Cost Tracking

### `createCostSink(type, config)`

Create cost tracking sink.

**Parameters**:
- `type: 'gcp' | 'file' | 'stdout'` - Sink type
- `config: CostSinkConfig` - Sink configuration

**Returns**: `CostSink`

**Example**:
```typescript
import { createCostSink } from '@wayfinder/multi-persona-review';

// GCP Cloud Monitoring
const gcpSink = createCostSink('gcp', {
  projectId: process.env.GCP_PROJECT_ID!,
});

// File-based
const fileSink = createCostSink('file', {
  filePath: './costs.jsonl',
});

// Stdout
const stdoutSink = createCostSink('stdout', {});
```

### `CostData`

Cost tracking data.

```typescript
interface CostData {
  inputTokens: number;            // Input tokens used
  outputTokens: number;           // Output tokens used
  cacheReadTokens?: number;       // Cache read tokens (Anthropic)
  cacheWriteTokens?: number;      // Cache write tokens (Anthropic)
  totalCost: number;              // Total cost in USD
  provider: string;               // AI provider
  model: string;                  // Model name
  personas: PersonaCost[];        // Per-persona costs
}
```

---

## Configuration

### `loadConfig(rootDir)`

Load configuration from file.

**Parameters**:
- `rootDir: string` - Root directory path

**Returns**: `Promise<EngConfiguration>`

**Example**:
```typescript
import { loadConfig } from '@wayfinder/multi-persona-review';

const config = await loadConfig(process.cwd());

console.log(config.crossCheck.defaultMode); // 'quick' or 'thorough'
console.log(config.crossCheck.defaultPersonas); // ['security-engineer', ...]
```

### `EngConfiguration`

Main configuration object.

```typescript
interface EngConfiguration {
  crossCheck: {
    defaultMode: ReviewMode;
    defaultPersonas: string[];
    options?: ReviewOptions;
    costTracking?: CostTrackingConfig;
  };
}
```

---

## Examples

### Basic Review

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  formatReviewResult,
} from '@wayfinder/multi-persona-review';

async function runReview() {
  // Load personas
  const personas = await loadPersonas(['.engram/personas']);
  const securityPersona = personas.get('security-engineer')!;

  // Create reviewer
  const reviewer = createAnthropicReviewer({
    apiKey: process.env.ANTHROPIC_API_KEY!,
  });

  // Run review
  const result = await reviewFiles(
    {
      files: ['src/index.ts'],
      personas: [securityPersona],
      mode: 'quick',
    },
    process.cwd(),
    reviewer
  );

  // Format output
  const formatted = formatReviewResult(result, { colors: true });
  console.log(formatted);
}

runReview().catch(console.error);
```

### Agency-Agents Review

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
} from '@wayfinder/multi-persona-review';

async function runAgencyReview() {
  const personas = await loadPersonas(['.engram/personas']);

  const reviewer = createAnthropicReviewer({
    apiKey: process.env.ANTHROPIC_API_KEY!,
  });

  const result = await reviewFiles(
    {
      files: ['src/'],
      personas: [
        personas.get('security-engineer')!,
        personas.get('tech-lead')!,
        personas.get('performance-reviewer')!,
      ],
      mode: 'thorough',
      options: {
        voteThreshold: 0.7,      // GO if 70% weighted vote
        minConfidence: 0.8,      // Only 80%+ confidence findings
      },
    },
    process.cwd(),
    reviewer
  );

  // Check aggregated vote
  const weightedVote = calculateWeightedVote(result.findings, personas);
  if (weightedVote >= 0.7) {
    console.log('✅ GO - Code approved for merge');
  } else {
    console.log('❌ NO-GO - Critical issues found');
    process.exit(1);
  }
}
```

### Custom Personas

```typescript
import {
  reviewFiles,
  createAnthropicReviewer,
  Persona,
} from '@wayfinder/multi-persona-review';

const customPersona: Persona = {
  name: 'my-custom-reviewer',
  displayName: 'My Custom Reviewer',
  version: '1.0.0',
  description: 'Custom review persona',
  focusAreas: ['custom-focus'],
  tier: 2,
  prompt: `You are a custom code reviewer.

Review the code for:
- Custom criterion 1
- Custom criterion 2

Output JSON format:
{
  "findings": [
    {
      "file": "path/to/file.ts",
      "line": 42,
      "severity": "high",
      "category": "custom",
      "message": "Issue description",
      "decision": "NO-GO",
      "alternatives": ["Option 1", "Option 2", "Option 3"]
    }
  ]
}`,
};

const reviewer = createAnthropicReviewer({
  apiKey: process.env.ANTHROPIC_API_KEY!,
});

const result = await reviewFiles(
  {
    files: ['src/index.ts'],
    personas: [customPersona],
    mode: 'thorough',
  },
  process.cwd(),
  reviewer
);
```

---

## Full API Reference

For complete API documentation, run:

```bash
npm run docs:generate  # Generates TypeDoc API docs
npm run docs:serve     # Serves docs at http://localhost:3000
```

Or view online: [API Documentation](https://wayfinder.github.io/multi-persona-review/api/)

---

## Support

- **Issues**: [GitHub Issues](https://github.com/wayfinder/multi-persona-review/issues)
- **Discussions**: [GitHub Discussions](https://github.com/wayfinder/multi-persona-review/discussions)
- **Documentation**: [Full Documentation](../DOCUMENTATION.md)

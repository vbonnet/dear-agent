/**
 * Anthropic API client for multi-persona-review plugin
 * Handles communication with Claude API for code reviews
 */

import Anthropic from '@anthropic-ai/sdk';
import type {
  PersonaReviewInput,
  PersonaReviewOutput,
  Finding,
  PersonaCost,
} from './types.js';
import { randomUUID } from 'crypto';

/**
 * Error codes for Anthropic client
 */
export const ANTHROPIC_ERROR_CODES = {
  API_KEY_MISSING: 'ANTHROPIC_5001',
  API_ERROR: 'ANTHROPIC_5002',
  RATE_LIMIT: 'ANTHROPIC_5003',
  PARSE_ERROR: 'ANTHROPIC_5004',
  TIMEOUT: 'ANTHROPIC_5005',
} as const;

/**
 * Anthropic client error
 */
export class AnthropicClientError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown
  ) {
    super(message);
    this.name = 'AnthropicClientError';
  }
}

/**
 * Token pricing for Claude models (as of 2025)
 */
const MODEL_PRICING: Record<string, { input: number; output: number }> = {
  'claude-3-5-sonnet-20241022': {
    input: 3.0 / 1_000_000,  // $3 per million input tokens
    output: 15.0 / 1_000_000, // $15 per million output tokens
  },
  'claude-3-5-haiku-20241022': {
    input: 0.8 / 1_000_000,   // $0.80 per million input tokens
    output: 4.0 / 1_000_000,  // $4 per million output tokens
  },
  'claude-3-opus-20240229': {
    input: 15.0 / 1_000_000,  // $15 per million input tokens
    output: 75.0 / 1_000_000, // $75 per million output tokens
  },
};

/**
 * Default model for code reviews
 */
const DEFAULT_MODEL = 'claude-3-5-sonnet-20241022';

/**
 * Anthropic client configuration
 */
export interface AnthropicClientConfig {
  /** API key */
  apiKey: string;

  /** Model to use */
  model?: string;

  /** Max tokens for response */
  maxTokens?: number;

  /** Temperature (0-1) */
  temperature?: number;

  /** Request timeout in ms */
  timeout?: number;
}

/**
 * Formats file content for the review prompt
 */
function formatFileContent(input: PersonaReviewInput): string {
  const parts: string[] = [];

  parts.push('# Files to Review\n');

  for (const file of input.files) {
    parts.push(`## File: ${file.path}\n`);

    if (file.isDiff) {
      parts.push('```diff');
      parts.push(file.content);
      parts.push('```\n');
    } else {
      // Detect language from file extension
      const ext = file.path.split('.').pop() || 'txt';
      parts.push(`\`\`\`${ext}`);
      parts.push(file.content);
      parts.push('```\n');
    }
  }

  return parts.join('\n');
}

/**
 * Builds the review prompt
 */
function buildReviewPrompt(input: PersonaReviewInput): string {
  const { persona, mode } = input;

  const modeInstructions = {
    quick: 'Focus on critical and high-severity issues only. Be concise.',
    thorough: 'Perform comprehensive review covering all severity levels. Be detailed.',
    custom: 'Review according to the persona focus areas.',
  };

  const instruction = modeInstructions[mode] || modeInstructions.custom;

  return `${instruction}

Please review the following code and identify any issues, following these focus areas:
${persona.focusAreas.map(area => `- ${area}`).join('\n')}

## Agency-Agents Protocol

For each finding, you must:

1. **Voting**: Provide a GO or NO-GO decision
   - GO = Code is acceptable (may need improvement but not blocking)
   - NO-GO = Code should be blocked (critical issues, security risks, etc.)

2. **Lateral Thinking**: Propose 3 alternative approaches or solutions
   - Think outside the box
   - Consider different perspectives
   - Suggest creative solutions

3. **Scope Detection**: Flag findings outside your expertise
   - Set "outOfScope": true if the finding requires expertise you don't have
   - Be honest about your limitations

Format your response as a JSON array of findings:

\`\`\`json
[
  {
    "severity": "medium",
    "file": "example.ts",
    "line": 42,
    "title": "Brief issue summary",
    "description": "Detailed explanation of the issue",
    "confidence": 0.9,
    "categories": ["performance", "scalability"],
    "decision": "GO",
    "alternatives": [
      "Alternative approach 1",
      "Alternative approach 2",
      "Alternative approach 3"
    ],
    "outOfScope": false
  }
]
\`\`\`

**Required fields**: severity, file, title, description, decision, alternatives (array of 3 strings)
**Optional fields**: line, confidence (default 0.8), categories, outOfScope (default false)

${formatFileContent(input)}`;
}

/**
 * Parses Claude's response to extract findings
 */
function parseClaudeResponse(
  response: string,
  personaName: string
): Finding[] {
  try {
    // Extract JSON from response (may be wrapped in markdown code block)
    const jsonMatch = response.match(/```json\s*([\s\S]*?)\s*```/) ||
                     response.match(/\[[\s\S]*\]/);

    if (!jsonMatch) {
      throw new Error('No JSON findings found in response');
    }

    const jsonStr = jsonMatch[1] || jsonMatch[0];
    const rawFindings = JSON.parse(jsonStr);

    if (!Array.isArray(rawFindings)) {
      throw new Error('Response is not an array of findings');
    }

    return rawFindings.map((raw: any, _index: number) => ({
      id: `${personaName}-${randomUUID()}`,
      file: raw.file || 'unknown',
      line: raw.line,
      lineEnd: raw.lineEnd,
      severity: raw.severity || 'info',
      personas: [personaName],
      title: raw.title || 'Untitled finding',
      description: raw.description || '',
      categories: raw.categories || [],
      confidence: raw.confidence || 0.8,
      decision: raw.decision,
      alternatives: raw.alternatives,
      outOfScope: raw.outOfScope || false,
      suggestedFix: raw.suggestedFix,
      metadata: raw.metadata,
    }));
  } catch (error) {
    throw new AnthropicClientError(
      ANTHROPIC_ERROR_CODES.PARSE_ERROR,
      `Failed to parse Claude response: ${error instanceof Error ? error.message : String(error)}`,
      { response, error }
    );
  }
}

/**
 * Calculates cost based on token usage
 */
function calculateCost(
  model: string,
  inputTokens: number,
  outputTokens: number
): number {
  const pricing = MODEL_PRICING[model] || MODEL_PRICING[DEFAULT_MODEL];
  return (inputTokens * pricing.input) + (outputTokens * pricing.output);
}

/**
 * Creates an Anthropic-based reviewer function
 */
export function createAnthropicReviewer(
  config: AnthropicClientConfig
): (input: PersonaReviewInput) => Promise<PersonaReviewOutput> {
  if (!config.apiKey) {
    throw new AnthropicClientError(
      ANTHROPIC_ERROR_CODES.API_KEY_MISSING,
      'Anthropic API key is required'
    );
  }

  const client = new Anthropic({
    apiKey: config.apiKey,
    timeout: config.timeout || 120000, // 2 minutes default
  });

  const model = config.model || DEFAULT_MODEL;
  const maxTokens = config.maxTokens || 4096;
  const temperature = config.temperature ?? 0.0; // Low temperature for consistent reviews

  return async (input: PersonaReviewInput): Promise<PersonaReviewOutput> => {
    try {
      const prompt = buildReviewPrompt(input);

      const message = await client.messages.create({
        model,
        max_tokens: maxTokens,
        temperature,
        system: input.persona.prompt,
        messages: [
          {
            role: 'user',
            content: prompt,
          },
        ],
      });

      // Extract response text
      const responseText = message.content
        .filter(block => block.type === 'text')
        .map(block => 'text' in block ? block.text : '')
        .join('\n');

      // Parse findings
      const findings = parseClaudeResponse(responseText, input.persona.name);

      // Calculate cost
      const inputTokens = message.usage.input_tokens;
      const outputTokens = message.usage.output_tokens;
      const cost = calculateCost(model, inputTokens, outputTokens);

      // Extract cache metrics if available
      const cacheCreationInputTokens = (message.usage as any).cache_creation_input_tokens;
      const cacheReadInputTokens = (message.usage as any).cache_read_input_tokens;

      const personaCost: PersonaCost = {
        persona: input.persona.name,
        cost,
        inputTokens,
        outputTokens,
        cacheCreationInputTokens,
        cacheReadInputTokens,
      };

      return {
        persona: input.persona.name,
        findings,
        cost: personaCost,
      };
    } catch (error) {
      // Handle rate limiting
      if (error instanceof Anthropic.APIError) {
        if (error.status === 429) {
          throw new AnthropicClientError(
            ANTHROPIC_ERROR_CODES.RATE_LIMIT,
            'Rate limit exceeded. Please try again later.',
            error
          );
        }

        throw new AnthropicClientError(
          ANTHROPIC_ERROR_CODES.API_ERROR,
          `Anthropic API error: ${error.message}`,
          error
        );
      }

      // Handle other errors
      throw new AnthropicClientError(
        ANTHROPIC_ERROR_CODES.API_ERROR,
        `Failed to review with ${input.persona.name}: ${error instanceof Error ? error.message : String(error)}`,
        error
      );
    }
  };
}

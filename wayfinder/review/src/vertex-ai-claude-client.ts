/**
 * VertexAI Claude client for multi-persona-review plugin
 * Handles communication with Claude via VertexAI Anthropic publisher
 */

import { GoogleAuth } from 'google-auth-library';
import { existsSync, readFileSync } from 'fs';
import type {
  PersonaReviewInput,
  PersonaReviewOutput,
  Finding,
  PersonaCost,
} from './types.js';

/**
 * Error codes for VertexAI Claude client
 */
export const VERTEX_CLAUDE_ERROR_CODES = {
  CREDENTIALS_MISSING: 'VERTEX_CLAUDE_5001',
  API_ERROR: 'VERTEX_CLAUDE_5002',
  RATE_LIMIT: 'VERTEX_CLAUDE_5003',
  PARSE_ERROR: 'VERTEX_CLAUDE_5004',
  TIMEOUT: 'VERTEX_CLAUDE_5005',
  REGION_ERROR: 'VERTEX_CLAUDE_5006',
} as const;

/**
 * VertexAI Claude client error
 */
export class VertexAIClaudeClientError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown
  ) {
    super(message);
    this.name = 'VertexAIClaudeClientError';
  }
}

/**
 * Token pricing for Claude models on VertexAI (as of 2026)
 * Prices are per million tokens
 * Source: https://cloud.google.com/vertex-ai/generative-ai/pricing
 */
const MODEL_PRICING: Record<string, { input: number; output: number }> = {
  'claude-sonnet-4-5@20250929': {
    input: 3.0 / 1_000_000,   // $3 per million input tokens
    output: 15.0 / 1_000_000, // $15 per million output tokens
  },
  'claude-haiku-4-5@20251001': {
    input: 0.8 / 1_000_000,   // $0.80 per million input tokens
    output: 4.0 / 1_000_000,  // $4 per million output tokens
  },
  'claude-opus-4-6@20260205': {
    input: 5.0 / 1_000_000,   // $5 per million input tokens
    output: 25.0 / 1_000_000, // $25 per million output tokens
  },
};

/**
 * Default model for code reviews
 */
const DEFAULT_MODEL = 'claude-sonnet-4-5@20250929';

/**
 * VertexAI Claude client configuration
 */
export interface VertexAIClaudeClientConfig {
  /** GCP Project ID */
  projectId: string;

  /** GCP Location/Region (e.g., us-east5) */
  location?: string;

  /** Claude model to use */
  model?: string;

  /** Max tokens for response */
  maxTokens?: number;

  /** Temperature (0-1) */
  temperature?: number;

  /** Request timeout in ms */
  timeout?: number;

  /** Path to GCP service account key JSON file (optional) */
  keyFilename?: string;
}

/**
 * Formats file content for the review prompt
 */
function formatFileContent(input: PersonaReviewInput): string {
  const parts: string[] = [];

  parts.push('# Files to Review\n');

  for (const file of input.files) {
    parts.push(`## ${file.path}\n`);
    parts.push('```');
    if (file.path.endsWith('.ts') || file.path.endsWith('.tsx')) {
      parts.push('typescript');
    } else if (file.path.endsWith('.js') || file.path.endsWith('.jsx')) {
      parts.push('javascript');
    } else if (file.path.endsWith('.py')) {
      parts.push('python');
    } else if (file.path.endsWith('.go')) {
      parts.push('go');
    } else if (file.path.endsWith('.java')) {
      parts.push('java');
    } else if (file.path.endsWith('.rb')) {
      parts.push('ruby');
    }
    parts.push(`\n${file.content}\n`);
    parts.push('```\n');
  }

  return parts.join('');
}

/**
 * Builds the review prompt
 */
function buildReviewPrompt(input: PersonaReviewInput): string {
  const fileContent = formatFileContent(input);

  return `${fileContent}

# Review Instructions

Review the code above as a **${input.persona.displayName || input.persona.name}**.

Focus on the following areas:
${input.persona.focusAreas?.map(area => `- ${area}`).join('\n') || '- Code quality\n- Best practices\n- Potential issues'}

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

## Output Format

Provide your review in the following JSON format:

\`\`\`json
{
  "findings": [
    {
      "severity": "critical|high|medium|low|info",
      "message": "Brief description",
      "file": "path/to/file.ext",
      "line": 42,
      "category": "category-name",
      "suggestion": "How to fix this",
      "confidence": 95,
      "decision": "GO|NO-GO",
      "alternatives": [
        "Alternative approach 1",
        "Alternative approach 2",
        "Alternative approach 3"
      ],
      "outOfScope": false
    }
  ],
  "summary": "Overall assessment of the code"
}
\`\`\`

**Required fields**: severity, message, file, decision, alternatives (array of 3 strings)
**Optional fields**: line, category, suggestion, confidence (default 80), outOfScope (default false)

Return ONLY valid JSON. If no issues found, return empty findings array.`;
}

/**
 * Parses Claude response and extracts findings
 */
function parseClaudeResponse(responseText: string, personaName: string): Finding[] {
  try {
    // Extract JSON from response (Claude may wrap it in markdown)
    let jsonText: string;

    // Try to extract from markdown code block first
    const markdownMatch = responseText.match(/```json\s*([\s\S]*?)\s*```/);
    if (markdownMatch) {
      jsonText = markdownMatch[1].trim();
    } else {
      // Fall back to finding the first complete JSON object
      const startIndex = responseText.indexOf('{');
      if (startIndex === -1) {
        throw new Error('No JSON found in response');
      }

      // Count braces to find the complete JSON object
      let braceCount = 0;
      let endIndex = startIndex;
      for (let i = startIndex; i < responseText.length; i++) {
        if (responseText[i] === '{') braceCount++;
        if (responseText[i] === '}') braceCount--;
        if (braceCount === 0) {
          endIndex = i + 1;
          break;
        }
      }

      jsonText = responseText.substring(startIndex, endIndex);
    }

    const parsed = JSON.parse(jsonText);

    if (!parsed.findings || !Array.isArray(parsed.findings)) {
      return [];
    }

    return parsed.findings.map((finding: any, index: number) => ({
      id: `${personaName}-${index}-${Date.now()}`,
      severity: finding.severity || 'medium',
      title: finding.message || finding.title || '',
      description: finding.message || finding.description || '',
      file: finding.file,
      line: finding.line,
      categories: finding.category ? [finding.category] : ['general'],
      suggestedFix: finding.suggestion ? {
        type: 'replace' as const,
        replacement: finding.suggestion,
        explanation: finding.suggestion,
        requiresConfirmation: true,
      } : undefined,
      confidence: (finding.confidence || 80) / 100, // Convert to 0-1 scale
      decision: finding.decision,
      alternatives: finding.alternatives,
      outOfScope: finding.outOfScope || false,
      personas: [personaName],
    }));
  } catch (error) {
    throw new VertexAIClaudeClientError(
      VERTEX_CLAUDE_ERROR_CODES.PARSE_ERROR,
      `Failed to parse Claude response: ${error}`,
      { responseText, error }
    );
  }
}

/**
 * Creates a VertexAI Claude-based reviewer function
 */
export function createVertexAIClaudeReviewer(
  config: VertexAIClaudeClientConfig
): (input: PersonaReviewInput) => Promise<PersonaReviewOutput> {
  if (!config.projectId) {
    throw new VertexAIClaudeClientError(
      VERTEX_CLAUDE_ERROR_CODES.CREDENTIALS_MISSING,
      'VertexAI project ID is required'
    );
  }

  // Validate credentials file if provided (Tasks 2.1, 2.2)
  if (config.keyFilename) {
    // Task 2.1: File existence validation
    if (!existsSync(config.keyFilename)) {
      throw new VertexAIClaudeClientError(
        VERTEX_CLAUDE_ERROR_CODES.CREDENTIALS_MISSING,
        `Credential file not found: ${config.keyFilename}\n\nPlease check the file path and try again.`
      );
    }

    // Task 2.2: JSON format validation
    try {
      const fileContent = readFileSync(config.keyFilename, 'utf-8');
      JSON.parse(fileContent);
    } catch (error) {
      throw new VertexAIClaudeClientError(
        VERTEX_CLAUDE_ERROR_CODES.CREDENTIALS_MISSING,
        `Invalid credential file format: ${config.keyFilename}\n\nThe file must be a valid JSON service account key. Please check the file syntax.`
      );
    }
  }

  const location = config.location || 'us-east5'; // Claude models available in us-east5
  const model = config.model || DEFAULT_MODEL;
  const maxTokens = config.maxTokens || 4096;
  const temperature = config.temperature ?? 0.0; // Low temperature for consistent reviews
  const timeout = config.timeout || 60000; // 60s default

  // Initialize Google Auth
  const auth = new GoogleAuth({
    scopes: ['https://www.googleapis.com/auth/cloud-platform'],
    keyFilename: config.keyFilename,
  });

  return async (input: PersonaReviewInput): Promise<PersonaReviewOutput> => {
    try {
      const prompt = buildReviewPrompt(input);

      // Get access token
      const client = await auth.getClient();
      const accessToken = await client.getAccessToken();

      if (!accessToken.token) {
        // Task 2.3: Comprehensive error message with 3 setup options + alternative
        throw new VertexAIClaudeClientError(
          VERTEX_CLAUDE_ERROR_CODES.CREDENTIALS_MISSING,
          `Google Cloud credentials required for VertexAI Claude provider

Setup options (choose one):

1. Environment variable:
   export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"

2. CLI option:
   --gcp-credentials /path/to/service-account-key.json

3. Application Default Credentials:
   gcloud auth application-default login

Alternative: Use Anthropic provider (simpler setup):
   --provider anthropic (requires ANTHROPIC_API_KEY)

For details: See README.md authentication section`
        );
      }

      // Build VertexAI Anthropic endpoint URL
      const endpoint = `https://${location}-aiplatform.googleapis.com/v1/projects/${config.projectId}/locations/${location}/publishers/anthropic/models/${model}:streamRawPredict`;

      // Build request body (Anthropic format via VertexAI)
      const requestBody = {
        anthropic_version: 'vertex-2023-10-16',
        messages: [
          {
            role: 'user',
            content: prompt,
          },
        ],
        max_tokens: maxTokens,
        system: input.persona.prompt,
        temperature: temperature,
      };

      // Make API request
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), timeout);

      try {
        const response = await fetch(endpoint, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${accessToken.token}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(requestBody),
          signal: controller.signal,
        });

        clearTimeout(timeoutId);

        if (!response.ok) {
          const errorText = await response.text();
          let errorData;
          try {
            errorData = JSON.parse(errorText);
          } catch {
            errorData = { message: errorText };
          }

          if (response.status === 429) {
            throw new VertexAIClaudeClientError(
              VERTEX_CLAUDE_ERROR_CODES.RATE_LIMIT,
              'Rate limit exceeded',
              errorData
            );
          }

          if (response.status === 400 && errorText.includes('not servable in region')) {
            throw new VertexAIClaudeClientError(
              VERTEX_CLAUDE_ERROR_CODES.REGION_ERROR,
              `Model ${model} not available in region ${location}. Try us-east5.`,
              errorData
            );
          }

          throw new VertexAIClaudeClientError(
            VERTEX_CLAUDE_ERROR_CODES.API_ERROR,
            `API request failed: ${response.status} ${response.statusText}`,
            errorData
          );
        }

        // Parse streaming response
        const responseText = await response.text();
        const lines = responseText.split('\n').filter(line => line.trim());

        let fullContent = '';
        let inputTokens = 0;
        let outputTokens = 0;

        for (const line of lines) {
          try {
            const data = JSON.parse(line);

            // Extract content from Claude response
            if (data.content && Array.isArray(data.content)) {
              for (const block of data.content) {
                if (block.type === 'text' && block.text) {
                  fullContent += block.text;
                }
              }
            }

            // Extract token usage
            if (data.usage) {
              inputTokens = data.usage.input_tokens || 0;
              outputTokens = data.usage.output_tokens || 0;
            }
          } catch {
            // Skip non-JSON lines
            continue;
          }
        }

        if (!fullContent) {
          throw new VertexAIClaudeClientError(
            VERTEX_CLAUDE_ERROR_CODES.API_ERROR,
            'Empty response from Claude API'
          );
        }

        // Parse findings
        const findings = parseClaudeResponse(fullContent, input.persona.name);

        // Calculate cost
        const pricing = MODEL_PRICING[model] || MODEL_PRICING[DEFAULT_MODEL];
        const cost: PersonaCost = {
          persona: input.persona.name,
          cost: (inputTokens * pricing.input) + (outputTokens * pricing.output),
          inputTokens,
          outputTokens,
        };

        return {
          persona: input.persona.name,
          findings,
          cost,
        };
      } catch (error: any) {
        clearTimeout(timeoutId);

        if (error.name === 'AbortError') {
          throw new VertexAIClaudeClientError(
            VERTEX_CLAUDE_ERROR_CODES.TIMEOUT,
            `Request timed out after ${timeout}ms`
          );
        }

        // Re-throw if already a VertexAIClaudeClientError
        if (error instanceof VertexAIClaudeClientError) {
          throw error;
        }

        throw new VertexAIClaudeClientError(
          VERTEX_CLAUDE_ERROR_CODES.API_ERROR,
          `Unexpected error: ${error.message}`,
          error
        );
      }
    } catch (error: any) {
      // Re-throw if already a VertexAIClaudeClientError
      if (error instanceof VertexAIClaudeClientError) {
        throw error;
      }

      // Wrap unexpected errors
      throw new VertexAIClaudeClientError(
        VERTEX_CLAUDE_ERROR_CODES.API_ERROR,
        `Failed to complete review: ${error.message}`,
        error
      );
    }
  };
}

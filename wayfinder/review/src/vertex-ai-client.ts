/**
 * VertexAI API client for multi-persona-review plugin
 * Handles communication with Google Gemini API for code reviews
 */

import { VertexAI, HarmCategory, HarmBlockThreshold } from '@google-cloud/vertexai';
import type {
  PersonaReviewInput,
  PersonaReviewOutput,
  Finding,
  PersonaCost,
} from './types.js';
import { randomUUID } from 'crypto';

/**
 * Error codes for VertexAI client
 */
export const VERTEXAI_ERROR_CODES = {
  CREDENTIALS_MISSING: 'VERTEXAI_5001',
  API_ERROR: 'VERTEXAI_5002',
  RATE_LIMIT: 'VERTEXAI_5003',
  PARSE_ERROR: 'VERTEXAI_5004',
  TIMEOUT: 'VERTEXAI_5005',
  SAFETY_ERROR: 'VERTEXAI_5006',
} as const;

/**
 * VertexAI client error
 */
export class VertexAIClientError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown
  ) {
    super(message);
    this.name = 'VertexAIClientError';
  }
}

/**
 * Token pricing for Gemini models (as of 2025)
 * Prices are per million tokens
 */
const MODEL_PRICING: Record<string, { input: number; output: number }> = {
  'gemini-2.0-flash-exp': {
    input: 0.0,           // Free during preview
    output: 0.0,          // Free during preview
  },
  'gemini-1.5-flash-002': {
    input: 0.075 / 1_000_000,   // $0.075 per million input tokens
    output: 0.30 / 1_000_000,   // $0.30 per million output tokens
  },
  'gemini-1.5-pro-002': {
    input: 1.25 / 1_000_000,    // $1.25 per million input tokens
    output: 5.00 / 1_000_000,   // $5.00 per million output tokens
  },
};

/**
 * Default model for code reviews
 */
const DEFAULT_MODEL = 'gemini-2.0-flash-exp';

/**
 * VertexAI client configuration
 */
export interface VertexAIClientConfig {
  /** GCP Project ID */
  projectId: string;

  /** GCP Location/Region */
  location?: string;

  /** Model to use */
  model?: string;

  /** Max output tokens */
  maxOutputTokens?: number;

  /** Temperature (0-2) */
  temperature?: number;

  /** Request timeout in ms */
  timeout?: number;

  /** Path to service account key file (optional, uses Application Default Credentials if not provided) */
  keyFilePath?: string;
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

For each finding, provide:
1. Severity (critical/high/medium/low/info)
2. File and line number
3. Title (brief summary)
4. Description (detailed explanation)
5. Confidence level (0-1)

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
    "categories": ["performance", "scalability"]
  }
]
\`\`\`

${formatFileContent(input)}`;
}

/**
 * Parses Gemini's response to extract findings
 */
function parseGeminiResponse(
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

    return rawFindings.map((raw: any) => ({
      id: `${personaName}-${randomUUID()}`,
      file: raw.file || 'unknown',
      line: raw.line,
      lineEnd: raw.lineEnd,
      severity: raw.severity || 'info',
      personas: [personaName],
      title: raw.title || 'Untitled finding',
      description: raw.description || '',
      categories: raw.categories || [],
      confidence: raw.confidence || 0.5,
      suggestedFix: raw.suggestedFix,
      metadata: raw.metadata,
    }));
  } catch (error) {
    throw new VertexAIClientError(
      VERTEXAI_ERROR_CODES.PARSE_ERROR,
      `Failed to parse Gemini response: ${error instanceof Error ? error.message : String(error)}`,
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
 * Creates a VertexAI-based reviewer function
 */
export function createVertexAIReviewer(
  config: VertexAIClientConfig
): (input: PersonaReviewInput) => Promise<PersonaReviewOutput> {
  if (!config.projectId) {
    throw new VertexAIClientError(
      VERTEXAI_ERROR_CODES.CREDENTIALS_MISSING,
      'VertexAI project ID is required'
    );
  }

  const location = config.location || 'us-central1';
  const model = config.model || DEFAULT_MODEL;
  const maxOutputTokens = config.maxOutputTokens || 4096;
  const temperature = config.temperature ?? 0.0; // Low temperature for consistent reviews

  // Initialize VertexAI
  const vertexAI = new VertexAI({
    project: config.projectId,
    location: location,
  });

  return async (input: PersonaReviewInput): Promise<PersonaReviewOutput> => {
    try {
      const prompt = buildReviewPrompt(input);

      // Get the generative model
      const generativeModel = vertexAI.getGenerativeModel({
        model: model,
        systemInstruction: {
          role: 'system',
          parts: [{ text: input.persona.prompt }],
        },
        generationConfig: {
          temperature: temperature,
          maxOutputTokens: maxOutputTokens,
        },
        safetySettings: [
          {
            category: HarmCategory.HARM_CATEGORY_HATE_SPEECH,
            threshold: HarmBlockThreshold.BLOCK_NONE,
          },
          {
            category: HarmCategory.HARM_CATEGORY_DANGEROUS_CONTENT,
            threshold: HarmBlockThreshold.BLOCK_NONE,
          },
          {
            category: HarmCategory.HARM_CATEGORY_SEXUALLY_EXPLICIT,
            threshold: HarmBlockThreshold.BLOCK_NONE,
          },
          {
            category: HarmCategory.HARM_CATEGORY_HARASSMENT,
            threshold: HarmBlockThreshold.BLOCK_NONE,
          },
        ],
      });

      // Generate content
      const result = await generativeModel.generateContent({
        contents: [
          {
            role: 'user',
            parts: [{ text: prompt }],
          },
        ],
      });

      const response = result.response;

      // Check for safety blocks
      if (!response.candidates || response.candidates.length === 0) {
        throw new VertexAIClientError(
          VERTEXAI_ERROR_CODES.SAFETY_ERROR,
          'Response was blocked by safety filters',
          { response }
        );
      }

      const candidate = response.candidates[0];

      // Extract response text
      const responseText = candidate.content?.parts
        ?.map((part: any) => part.text || '')
        .join('\n') || '';

      if (!responseText) {
        throw new VertexAIClientError(
          VERTEXAI_ERROR_CODES.API_ERROR,
          'Empty response from Gemini API',
          { response }
        );
      }

      // Parse findings
      const findings = parseGeminiResponse(responseText, input.persona.name);

      // Calculate token usage and cost
      const usageMetadata = response.usageMetadata;
      const inputTokens = usageMetadata?.promptTokenCount || 0;
      const outputTokens = usageMetadata?.candidatesTokenCount || 0;
      const cost = calculateCost(model, inputTokens, outputTokens);

      // Extract cache metrics if available (Vertex AI may support caching in the future)
      const cacheCreationInputTokens = (usageMetadata as any)?.cachedContentTokenCount;
      const cacheReadInputTokens = (usageMetadata as any)?.cacheReadTokenCount;

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
      if (error instanceof Error) {
        if (error.message.includes('429') || error.message.includes('quota')) {
          throw new VertexAIClientError(
            VERTEXAI_ERROR_CODES.RATE_LIMIT,
            'Rate limit exceeded. Please try again later.',
            error
          );
        }

        if (error.message.includes('timeout')) {
          throw new VertexAIClientError(
            VERTEXAI_ERROR_CODES.TIMEOUT,
            'Request timed out. Please try again.',
            error
          );
        }

        // Re-throw if already a VertexAIClientError
        if (error instanceof VertexAIClientError) {
          throw error;
        }
      }

      // Handle other errors
      throw new VertexAIClientError(
        VERTEXAI_ERROR_CODES.API_ERROR,
        `Failed to review with ${input.persona.name}: ${error instanceof Error ? error.message : String(error)}`,
        error
      );
    }
  };
}

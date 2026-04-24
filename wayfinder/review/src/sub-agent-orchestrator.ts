/**
 * Sub-Agent Orchestrator for multi-persona-review plugin
 *
 * Manages persona sub-agents with isolated contexts and prompt caching.
 * Implements the architecture from ADR 001-sub-agent-architecture.md
 *
 * Key Features:
 * - Isolated conversation contexts per persona
 * - Prompt caching with cache_control breakpoints
 * - Sub-agent pooling for session-level reuse
 * - Parallel execution support
 * - Graceful error handling
 */

import Anthropic, { APIError } from '@anthropic-ai/sdk';
import { GoogleAuth } from 'google-auth-library';
import { createHash } from 'crypto';
import { randomUUID } from 'crypto';
import type {
  Persona,
  PersonaReviewInput,
  PersonaReviewOutput,
  Finding,
  PersonaCost,
  ReviewError,
} from './types.js';

// ============================================================================
// Error Codes
// ============================================================================

export const SUB_AGENT_ERROR_CODES = {
  API_KEY_MISSING: 'SUBAGENT_6001',
  REVIEW_FAILED: 'SUBAGENT_6002',
  PARSE_ERROR: 'SUBAGENT_6003',
  CACHE_ERROR: 'SUBAGENT_6004',
  CREATION_FAILED: 'SUBAGENT_6005',
} as const;

/**
 * Sub-Agent error
 */
export class SubAgentError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown
  ) {
    super(message);
    this.name = 'SubAgentError';
  }
}

// ============================================================================
// Token Pricing — canonical source: engram/core/pkg/costtrack/pricing.go
// ============================================================================

interface ModelPricing {
  input: number;
  output: number;
  cacheWrite: number;
  cacheRead: number;
}

const MODEL_PRICING: Record<string, ModelPricing> = {
  'claude-3-haiku-20240307': {
    input: 0.25 / 1_000_000,
    output: 1.25 / 1_000_000,
    cacheWrite: 0.30 / 1_000_000,
    cacheRead: 0.03 / 1_000_000,
  },
  'claude-3-5-sonnet-20241022': {
    input: 3.0 / 1_000_000,
    output: 15.0 / 1_000_000,
    cacheWrite: 3.75 / 1_000_000,
    cacheRead: 0.30 / 1_000_000,
  },
  'claude-sonnet-4-5@20250929': {
    input: 3.0 / 1_000_000,
    output: 15.0 / 1_000_000,
    cacheWrite: 3.75 / 1_000_000,
    cacheRead: 0.30 / 1_000_000,
  },
  'claude-3-5-haiku-20241022': {
    input: 1.0 / 1_000_000,
    output: 5.0 / 1_000_000,
    cacheWrite: 1.25 / 1_000_000,
    cacheRead: 0.10 / 1_000_000,
  },
  'claude-3-opus-20240229': {
    input: 15.0 / 1_000_000,
    output: 75.0 / 1_000_000,
    cacheWrite: 18.75 / 1_000_000,
    cacheRead: 1.50 / 1_000_000,
  },
  'claude-opus-4-6': {
    input: 15.0 / 1_000_000,
    output: 75.0 / 1_000_000,
    cacheWrite: 18.75 / 1_000_000,
    cacheRead: 1.50 / 1_000_000,
  },
};

const DEFAULT_MODEL = 'claude-3-5-sonnet-20241022';

// ============================================================================
// Types and Interfaces
// ============================================================================

/**
 * Usage statistics from Anthropic API response
 */
export interface ApiUsageStats {
  input_tokens: number;
  output_tokens: number;
  cache_creation_input_tokens?: number;
  cache_read_input_tokens?: number;
}

/**
 * Statistics tracked for each sub-agent
 */
export interface SubAgentStats {
  /** Total number of reviews performed */
  reviewCount: number;

  /** Number of cache hits */
  cacheHits: number;

  /** Number of cache misses */
  cacheMisses: number;

  /** Total input tokens (uncached + cached) */
  totalInputTokens: number;

  /** Total output tokens */
  totalOutputTokens: number;

  /** Total tokens written to cache */
  totalCacheWrites: number;

  /** Total tokens read from cache */
  totalCacheReads: number;

  /** Last time this agent was used */
  lastUsed: Date;

  /** Creation time */
  createdAt: Date;
}

/**
 * Sub-Agent interface - represents a single persona with isolated context
 */
export interface SubAgent {
  /** Persona this agent represents */
  readonly persona: Persona;

  /** Unique cache key for this agent configuration */
  readonly cacheKey: string;

  /** Review files using this persona's perspective */
  review(input: PersonaReviewInput): Promise<PersonaReviewOutput>;

  /** Cleanup resources */
  destroy(): Promise<void>;

  /** Get usage statistics */
  getStats(): SubAgentStats;
}

/**
 * Configuration for SubAgent creation
 */
export interface SubAgentConfig {
  // Anthropic credentials (optional if VertexAI provided)
  /** Anthropic API key */
  apiKey?: string;

  // VertexAI credentials (optional if Anthropic provided)
  /** GCP Project ID for VertexAI */
  vertexProject?: string;

  /** GCP Region for VertexAI (default: 'us-east5') */
  vertexLocation?: string;

  /** Path to GCP service account key JSON (optional, uses ADC if not provided) */
  vertexKeyFilename?: string;

  // Common fields
  /** Model to use */
  model?: string;

  /** Maximum tokens for response */
  maxTokens?: number;

  /** Temperature (0-1) */
  temperature?: number;

  /** Request timeout in ms */
  timeout?: number;

  /** Cache TTL ('5min' | '1h') - auto-selected if not specified */
  cacheTtl?: '5min' | '1h';

  /** Enable automatic TTL selection (default: true) */
  autoTtl?: boolean;
}

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Selects optimal cache strategy based on expected review count
 *
 * Heuristic:
 * - If ≥4 reviews expected in session → enable caching (worth the +25% write cost)
 * - Otherwise → consider disabling caching for cost optimization
 *
 * Cost Trade-offs:
 * - Cache enabled: +25% write cost, -90% read cost on cache hits
 * - Cache disabled: No write overhead, but no read savings
 * - Break-even: ~2-3 cache hits within 5-minute window
 *
 * Note: Anthropic API currently uses fixed 5-minute ephemeral cache TTL.
 * The '5min' | '1h' return values represent our recommendation for cache
 * aggressiveness, not actual API parameters.
 *
 * @param expectedReviewCount - Expected number of reviews in this session
 * @returns '5min' (conservative) or '1h' (aggressive) cache strategy
 */
export function selectCacheTTL(expectedReviewCount: number): '5min' | '1h' {
  // '1h' = aggressive caching (≥4 reviews expected, cache write cost justified)
  // '5min' = conservative caching (<4 reviews, marginal benefit)
  return expectedReviewCount >= 4 ? '1h' : '5min';
}

/**
 * Detects session context to estimate expected review count
 *
 * Detection strategies:
 * 1. Environment variable: MULTI_PERSONA_REVIEW_COUNT
 * 2. Batch mode indicator: MULTI_PERSONA_BATCH_MODE=true
 * 3. CI/CD detection: CI=true (assumes thorough reviews)
 * 4. Default: 1 review (conservative, uses 5min TTL)
 *
 * @returns Estimated number of reviews expected in this session
 */
export function detectSessionReviewCount(): number {
  // Explicit review count from environment
  const explicitCount = process.env.MULTI_PERSONA_REVIEW_COUNT;
  if (explicitCount) {
    const count = parseInt(explicitCount, 10);
    if (!isNaN(count) && count > 0) {
      return count;
    }
  }

  // Batch mode indicator
  if (process.env.MULTI_PERSONA_BATCH_MODE === 'true') {
    return 5; // Assume multiple reviews in batch mode
  }

  // CI/CD environment detection
  if (process.env.CI === 'true') {
    return 4; // CI typically runs thorough reviews
  }

  // Default: single review (conservative)
  return 1;
}

/**
 * Calculate version hash for persona (cache key component)
 */
function calculatePersonaHash(persona: Persona): string {
  const stableContent = JSON.stringify({
    name: persona.name,
    version: persona.version,
    prompt: persona.prompt,
    focusAreas: persona.focusAreas,
    severityLevels: persona.severityLevels,
  });

  return createHash('sha256')
    .update(stableContent)
    .digest('hex')
    .slice(0, 16); // 16 chars sufficient for collision avoidance
}

/**
 * Calculate cost based on token usage (with cache support)
 */
function calculateCost(
  model: string,
  usage: ApiUsageStats
): number {
  const pricing = MODEL_PRICING[model] || MODEL_PRICING[DEFAULT_MODEL];

  const cacheWriteCost = (usage.cache_creation_input_tokens || 0) * pricing.cacheWrite;
  const cacheReadCost = (usage.cache_read_input_tokens || 0) * pricing.cacheRead;

  // Uncached input tokens = total input - cache writes - cache reads
  const uncachedInput = usage.input_tokens -
    (usage.cache_creation_input_tokens || 0) -
    (usage.cache_read_input_tokens || 0);
  const uncachedInputCost = uncachedInput * pricing.input;

  const outputCost = usage.output_tokens * pricing.output;

  return cacheWriteCost + cacheReadCost + uncachedInputCost + outputCost;
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
    throw new SubAgentError(
      SUB_AGENT_ERROR_CODES.PARSE_ERROR,
      `Failed to parse Claude response: ${error instanceof Error ? error.message : String(error)}`,
      { response, error }
    );
  }
}

// ============================================================================
// SubAgentImpl - Concrete implementation of SubAgent interface
// ============================================================================

class SubAgentImpl implements SubAgent {
  readonly persona: Persona;
  readonly cacheKey: string;

  private providerType: 'anthropic' | 'vertexai-claude';
  private client: Anthropic | null;
  private googleAuth: GoogleAuth | null;
  private config: SubAgentConfig;
  private stats: SubAgentStats;

  constructor(persona: Persona, cacheKey: string, client: Anthropic | null, config: SubAgentConfig, googleAuth: GoogleAuth | null = null) {
    // Validate credentials
    if (!config.apiKey && !config.vertexProject) {
      throw new SubAgentError(
        SUB_AGENT_ERROR_CODES.API_KEY_MISSING,
        'Missing provider credentials. Please configure one of:\n\n' +
        'Anthropic API:\n' +
        '  Environment: ANTHROPIC_API_KEY=sk-ant-...\n' +
        '  CLI flag: --api-key sk-ant-...\n\n' +
        'VertexAI (Claude Code auto-detected):\n' +
        '  Environment: ANTHROPIC_VERTEX_PROJECT_ID=your-project\n' +
        '               CLOUD_ML_REGION=us-east5\n' +
        '               GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json\n' +
        '  CLI flags: --provider vertexai-claude --vertex-project your-project\n\n' +
        'See DOCUMENTATION.md for detailed setup instructions.'
      );
    }

    // Detect provider type (Anthropic takes precedence if both provided)
    this.providerType = config.apiKey ? 'anthropic' : 'vertexai-claude';

    // Log provider selection in verbose mode
    if (process.env.VERBOSE === 'true') {
      console.error(`[SUB-AGENT] Provider selected: ${this.providerType}`);
      if (this.providerType === 'anthropic') {
        console.error(`[SUB-AGENT] Using Anthropic API with key: ${config.apiKey?.slice(0, 10)}...`);
      } else {
        console.error(`[SUB-AGENT] Using VertexAI with project: ${config.vertexProject}`);
        console.error(`[SUB-AGENT] VertexAI location: ${config.vertexLocation || 'us-east5'}`);
      }
    }

    this.persona = persona;
    this.cacheKey = cacheKey;
    this.client = client;
    this.googleAuth = googleAuth;
    this.config = config;

    this.stats = {
      reviewCount: 0,
      cacheHits: 0,
      cacheMisses: 0,
      totalInputTokens: 0,
      totalOutputTokens: 0,
      totalCacheWrites: 0,
      totalCacheReads: 0,
      lastUsed: new Date(),
      createdAt: new Date(),
    };
  }

  async review(input: PersonaReviewInput): Promise<PersonaReviewOutput> {
    if (this.providerType === 'anthropic') {
      return this.reviewWithAnthropic(input);
    } else {
      return this.reviewWithVertexAI(input);
    }
  }

  private async reviewWithAnthropic(input: PersonaReviewInput): Promise<PersonaReviewOutput> {
    if (!this.client) {
      throw new SubAgentError(
        SUB_AGENT_ERROR_CODES.API_KEY_MISSING,
        'Anthropic client not initialized'
      );
    }

    try {
      const model = this.config.model || DEFAULT_MODEL;
      const maxTokens = this.config.maxTokens || 4096;
      const temperature = this.config.temperature ?? 0.0;

      // Build user prompt
      const userPrompt = buildReviewPrompt(input);

      // NOTE: Auto-TTL selection for strategic cache management
      // The Anthropic API uses a fixed 5-minute TTL for ephemeral caching.
      // Our auto-TTL logic helps determine WHEN to enable caching based on
      // expected review patterns, not the actual TTL duration.
      //
      // Heuristic: Enable caching when we expect ≥4 reviews in a session
      // This ensures we benefit from cache hits while minimizing waste.

      let enableCaching = true; // Default: caching enabled

      if (this.config.autoTtl !== false) {
        // Auto-TTL enabled (default)
        if (this.config.cacheTtl !== undefined) {
          // Explicit TTL provided - interpret as caching preference
          // '5min' or '1h' both mean "enable caching"
          enableCaching = true;
        } else {
          // Auto-select based on session context
          const expectedReviews = detectSessionReviewCount();
          const recommendedTtl = selectCacheTTL(expectedReviews);

          // For now, always enable caching since Anthropic uses 5min TTL
          // Future: Could disable caching for single reviews to save write cost
          enableCaching = true;

          // Log TTL recommendation for debugging (if verbose mode)
          // This helps users understand the auto-selection decision
          if (process.env.VERBOSE === 'true') {
            console.error(`[SUB-AGENT] Auto-TTL: ${expectedReviews} reviews expected → ${recommendedTtl} recommended`);
          }
        }
      } else {
        // Auto-TTL disabled - only cache if explicitly requested
        enableCaching = this.config.cacheTtl !== undefined;
      }

      // Create message with cache_control
      const message = await this.client.messages.create({
        model,
        max_tokens: maxTokens,
        temperature,

        // System prompt with cache_control breakpoint (if enabled)
        // @ts-expect-error - cache_control is supported but not in SDK types yet
        system: [
          {
            type: 'text',
            text: this.persona.prompt,
            ...(enableCaching && { cache_control: { type: 'ephemeral' } }),
          },
        ],

        // User message (varies per review)
        messages: [
          {
            role: 'user',
            content: userPrompt,
          },
        ],
      });

      // Update statistics
      const usage = message.usage as any; // Type assertion needed for cache token properties
      this.stats.reviewCount++;
      this.stats.totalInputTokens += usage.input_tokens;
      this.stats.totalOutputTokens += usage.output_tokens;
      this.stats.totalCacheWrites += usage.cache_creation_input_tokens || 0;
      this.stats.totalCacheReads += usage.cache_read_input_tokens || 0;
      this.stats.lastUsed = new Date();

      // Track cache hits/misses
      if ((usage.cache_read_input_tokens || 0) > 0) {
        this.stats.cacheHits++;
      } else if ((usage.cache_creation_input_tokens || 0) > 0) {
        this.stats.cacheMisses++;
      }

      // Extract response text
      const responseText = message.content
        .filter(block => block.type === 'text')
        .map(block => 'text' in block ? block.text : '')
        .join('\n');

      // Parse findings
      const findings = parseClaudeResponse(responseText, this.persona.name);

      // Calculate cost (including cache costs)
      const cost = calculateCost(model, usage);

      const personaCost: PersonaCost = {
        persona: this.persona.name,
        cost,
        inputTokens: usage.input_tokens,
        outputTokens: usage.output_tokens,
      };

      return {
        persona: this.persona.name,
        findings,
        cost: personaCost,
      };

    } catch (error) {
      // Handle Anthropic API errors
      if (error instanceof APIError) {
        // Authentication/permission errors should throw (can't be recovered)
        // This allows review-engine to fall back to legacy reviewer
        if (error.status === 401 || error.status === 403) {
          // Provide provider-specific error message
          let authErrorMsg = `Authentication failed for ${this.providerType} provider: ${error.message}\n\n`;

          if (this.providerType === 'anthropic') {
            authErrorMsg += 'Troubleshooting:\n' +
              '  1. Verify ANTHROPIC_API_KEY is set correctly\n' +
              '  2. Check API key starts with "sk-ant-"\n' +
              '  3. Ensure API key has not expired\n' +
              '  4. Visit https://console.anthropic.com/settings/keys to manage keys\n';
          } else {
            authErrorMsg += 'Troubleshooting:\n' +
              '  1. Verify ANTHROPIC_VERTEX_PROJECT_ID is set\n' +
              '  2. Check CLOUD_ML_REGION is valid (e.g., us-east5)\n' +
              '  3. Ensure GOOGLE_APPLICATION_CREDENTIALS points to valid service account key\n' +
              '  4. Verify service account has Vertex AI User role\n' +
              '  5. Check project has Vertex AI API enabled\n';
          }

          const authError = new SubAgentError(
            SUB_AGENT_ERROR_CODES.API_KEY_MISSING,
            authErrorMsg,
            error
          );
          throw authError;
        }

        // Other API errors (rate limits, etc.) can gracefully degrade
        let apiErrorMsg = `${this.providerType} API error: ${error.message}`;

        if (error.status === 429) {
          apiErrorMsg += '\n\nRate limit exceeded. Try:\n' +
            '  1. Reduce number of parallel personas\n' +
            '  2. Add delays between reviews\n' +
            '  3. Check your API tier limits\n';
        }

        const reviewError: ReviewError = {
          code: error.status === 429 ? 'RATE_LIMIT' : 'API_ERROR',
          message: apiErrorMsg,
          persona: this.persona.name,
          details: error,
        };

        return {
          persona: this.persona.name,
          findings: [],
          cost: { persona: this.persona.name, cost: 0, inputTokens: 0, outputTokens: 0 },
          errors: [reviewError],
        };
      }

      // Handle other errors with graceful degradation
      const reviewError: ReviewError = {
        code: SUB_AGENT_ERROR_CODES.REVIEW_FAILED,
        message: `Sub-agent review failed (${this.providerType}): ${error instanceof Error ? error.message : String(error)}`,
        persona: this.persona.name,
        details: error,
      };

      return {
        persona: this.persona.name,
        findings: [],
        cost: { persona: this.persona.name, cost: 0, inputTokens: 0, outputTokens: 0 },
        errors: [reviewError],
      };
    }
  }

  private async reviewWithVertexAI(input: PersonaReviewInput): Promise<PersonaReviewOutput> {
    if (!this.googleAuth) {
      throw new SubAgentError(
        SUB_AGENT_ERROR_CODES.API_KEY_MISSING,
        'GoogleAuth not initialized for VertexAI provider'
      );
    }

    try {
      const model = this.config.model || 'claude-sonnet-4-5@20250929';
      const maxTokens = this.config.maxTokens || 4096;
      const temperature = this.config.temperature ?? 0.0;
      const timeout = this.config.timeout || 120000;
      const location = this.config.vertexLocation || 'us-east5';

      // Build user prompt
      const userPrompt = buildReviewPrompt(input);

      // Get access token
      const client = await this.googleAuth.getClient();
      const accessToken = await client.getAccessToken();

      if (!accessToken.token) {
        throw new SubAgentError(
          SUB_AGENT_ERROR_CODES.API_KEY_MISSING,
          'Failed to obtain Google Cloud access token. Ensure credentials are configured correctly.'
        );
      }

      // Build VertexAI Anthropic endpoint URL
      const endpoint = `https://${location}-aiplatform.googleapis.com/v1/projects/${this.config.vertexProject}/locations/${location}/publishers/anthropic/models/${model}:streamRawPredict`;

      // Build request body (Anthropic format via VertexAI)
      // Note: VertexAI does not support prompt caching yet, so we omit cache_control
      const requestBody = {
        anthropic_version: 'vertex-2023-10-16',
        messages: [
          {
            role: 'user',
            content: userPrompt,
          },
        ],
        max_tokens: maxTokens,
        system: this.persona.prompt,
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
            const reviewError: ReviewError = {
              code: 'RATE_LIMIT',
              message: 'VertexAI rate limit exceeded',
              persona: this.persona.name,
              details: errorData,
            };

            return {
              persona: this.persona.name,
              findings: [],
              cost: { persona: this.persona.name, cost: 0, inputTokens: 0, outputTokens: 0 },
              errors: [reviewError],
            };
          }

          if (response.status === 400 && errorText.includes('not servable in region')) {
            throw new SubAgentError(
              SUB_AGENT_ERROR_CODES.REVIEW_FAILED,
              `Model ${model} not available in region ${location}. Try us-east5.`
            );
          }

          throw new SubAgentError(
            SUB_AGENT_ERROR_CODES.REVIEW_FAILED,
            `VertexAI API request failed: ${response.status} ${response.statusText}`,
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
          throw new SubAgentError(
            SUB_AGENT_ERROR_CODES.REVIEW_FAILED,
            'Empty response from VertexAI Claude API'
          );
        }

        // Update statistics (no caching for VertexAI)
        this.stats.reviewCount++;
        this.stats.totalInputTokens += inputTokens;
        this.stats.totalOutputTokens += outputTokens;
        this.stats.lastUsed = new Date();

        // Parse findings
        const findings = parseClaudeResponse(fullContent, this.persona.name);

        // Calculate cost (VertexAI pricing, no cache savings)
        const pricing = MODEL_PRICING[model] || MODEL_PRICING[DEFAULT_MODEL];
        const cost = (inputTokens * pricing.input) + (outputTokens * pricing.output);

        const personaCost: PersonaCost = {
          persona: this.persona.name,
          cost,
          inputTokens,
          outputTokens,
        };

        return {
          persona: this.persona.name,
          findings,
          cost: personaCost,
        };
      } catch (error: any) {
        clearTimeout(timeoutId);

        if (error.name === 'AbortError') {
          const reviewError: ReviewError = {
            code: 'TIMEOUT',
            message: `VertexAI request timed out after ${timeout}ms`,
            persona: this.persona.name,
          };

          return {
            persona: this.persona.name,
            findings: [],
            cost: { persona: this.persona.name, cost: 0, inputTokens: 0, outputTokens: 0 },
            errors: [reviewError],
          };
        }

        // Re-throw if already a SubAgentError
        if (error instanceof SubAgentError) {
          throw error;
        }

        throw new SubAgentError(
          SUB_AGENT_ERROR_CODES.REVIEW_FAILED,
          `VertexAI unexpected error: ${error.message}`,
          error
        );
      }
    } catch (error) {
      // Handle any errors with graceful degradation
      const reviewError: ReviewError = {
        code: SUB_AGENT_ERROR_CODES.REVIEW_FAILED,
        message: `VertexAI sub-agent review failed: ${error instanceof Error ? error.message : String(error)}`,
        persona: this.persona.name,
        details: error,
      };

      return {
        persona: this.persona.name,
        findings: [],
        cost: { persona: this.persona.name, cost: 0, inputTokens: 0, outputTokens: 0 },
        errors: [reviewError],
      };
    }
  }

  async destroy(): Promise<void> {
    // Anthropic SDK doesn't require explicit cleanup
    // This is a hook for future resource cleanup if needed
  }

  getStats(): SubAgentStats {
    return { ...this.stats };
  }
}

// ============================================================================
// SubAgentFactory - Creates sub-agents with isolated contexts
// ============================================================================

export class SubAgentFactory {
  /**
   * Create a new sub-agent for the given persona
   */
  static createSubAgent(persona: Persona, config: SubAgentConfig): SubAgent {
    if (!config.apiKey && !config.vertexProject) {
      throw new SubAgentError(
        SUB_AGENT_ERROR_CODES.API_KEY_MISSING,
        'Missing provider credentials for sub-agent creation.\n\n' +
        'Anthropic API:\n' +
        '  Set ANTHROPIC_API_KEY=sk-ant-...\n\n' +
        'VertexAI:\n' +
        '  Set ANTHROPIC_VERTEX_PROJECT_ID=your-project\n' +
        '  Set CLOUD_ML_REGION=us-east5\n' +
        '  Set GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json\n\n' +
        'See DOCUMENTATION.md for setup instructions.'
      );
    }

    // Calculate cache key
    const versionHash = calculatePersonaHash(persona);
    const cacheKey = `persona:${persona.name}:${persona.version}:${versionHash}`;

    // Determine provider type (Anthropic takes precedence if both provided)
    const providerType = config.apiKey ? 'anthropic' : 'vertexai-claude';

    if (providerType === 'anthropic') {
      // Create dedicated Anthropic client for this persona
      const client = new Anthropic({
        apiKey: config.apiKey,
        timeout: config.timeout || 120000, // 2 minutes default
      });

      return new SubAgentImpl(persona, cacheKey, client, config, null);
    } else {
      // Create GoogleAuth for VertexAI
      const googleAuth = new GoogleAuth({
        scopes: ['https://www.googleapis.com/auth/cloud-platform'],
        keyFilename: config.vertexKeyFilename,
      });

      return new SubAgentImpl(persona, cacheKey, null, config, googleAuth);
    }
  }
}

// ============================================================================
// SubAgentPool - Caches sub-agents for session-level reuse
// ============================================================================

export class SubAgentPool {
  private pool = new Map<string, SubAgent>();
  private maxSize: number;

  constructor(maxSize: number = 10) {
    this.maxSize = maxSize;
  }

  /**
   * Get sub-agent from pool or create new one
   */
  get(persona: Persona, config: SubAgentConfig): SubAgent {
    const versionHash = calculatePersonaHash(persona);
    const cacheKey = `persona:${persona.name}:${persona.version}:${versionHash}`;

    // Return cached agent if available
    if (this.pool.has(cacheKey)) {
      return this.pool.get(cacheKey)!;
    }

    // Enforce pool size limit (LRU eviction)
    if (this.pool.size >= this.maxSize) {
      this.evictLRU();
    }

    // Create new agent
    const agent = SubAgentFactory.createSubAgent(persona, config);
    this.pool.set(cacheKey, agent);

    return agent;
  }

  /**
   * Remove sub-agent from pool
   */
  async destroy(persona: Persona): Promise<void> {
    const versionHash = calculatePersonaHash(persona);
    const cacheKey = `persona:${persona.name}:${persona.version}:${versionHash}`;

    const agent = this.pool.get(cacheKey);
    if (agent) {
      await agent.destroy();
      this.pool.delete(cacheKey);
    }
  }

  /**
   * Clear all sub-agents from pool
   */
  async clear(): Promise<void> {
    const destroyPromises = Array.from(this.pool.values()).map(agent => agent.destroy());
    await Promise.all(destroyPromises);
    this.pool.clear();
  }

  /**
   * Get current pool size
   */
  size(): number {
    return this.pool.size;
  }

  /**
   * Get all cached persona names
   */
  getCachedPersonas(): string[] {
    return Array.from(this.pool.values()).map(agent => agent.persona.name);
  }

  /**
   * Get statistics for all sub-agents in pool
   */
  getAllStats(): Record<string, SubAgentStats> {
    const stats: Record<string, SubAgentStats> = {};

    for (const [key, agent] of this.pool.entries()) {
      stats[key] = agent.getStats();
    }

    return stats;
  }

  /**
   * Evict least recently used sub-agent
   */
  private evictLRU(): void {
    let oldestKey: string | null = null;
    let oldestTime: Date | null = null;

    for (const [key, agent] of this.pool.entries()) {
      const stats = agent.getStats();
      if (oldestTime === null || stats.lastUsed < oldestTime) {
        oldestTime = stats.lastUsed;
        oldestKey = key;
      }
    }

    if (oldestKey) {
      const agent = this.pool.get(oldestKey);
      if (agent) {
        agent.destroy().catch(() => {
          // Ignore cleanup errors during eviction
        });
      }
      this.pool.delete(oldestKey);
    }
  }
}

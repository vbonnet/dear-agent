/**
 * Review engine for multi-persona-review plugin
 * Orchestrates persona reviews and aggregates results
 */

import { randomUUID } from 'crypto';
import { trace, SpanStatusCode } from '@opentelemetry/api';
import { scanFiles } from './file-scanner.js';
import { deduplicateFindings } from './deduplication.js';
import type { CostSink, CostMetadata } from './cost-sink.js';
import { SubAgentPool } from './sub-agent-orchestrator.js';
import type { SubAgentConfig } from './sub-agent-orchestrator.js';

const tracer = trace.getTracer('engram/multi-persona-review');
import type {
  ReviewConfig,
  ReviewResult,
  ReviewSummary,
  CostInfo,
  PersonaCost,
  PersonaReviewInput,
  PersonaReviewOutput,
  Finding,
  FileContent,
  FileScanMode,
  Severity,
  Persona,
} from './types.js';

/**
 * Error codes for review engine
 */
export const REVIEW_ENGINE_ERROR_CODES = {
  INVALID_CONFIG: 'REVIEW_4001',
  NO_FILES: 'REVIEW_4002',
  ALL_PERSONAS_FAILED: 'REVIEW_4003',
  REVIEWER_ERROR: 'REVIEW_4004',
} as const;

/**
 * Review engine error
 */
export class ReviewEngineError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown
  ) {
    super(message);
    this.name = 'ReviewEngineError';
  }
}

/**
 * Reviewer function type - abstracts the actual LLM call
 * This allows testing without real API calls
 */
export type ReviewerFunction = (
  input: PersonaReviewInput
) => Promise<PersonaReviewOutput>;

/**
 * Review context - prepared files and metadata
 */
export interface ReviewContext {
  /** Files with their content */
  files: FileContent[];

  /** Working directory */
  cwd: string;

  /** Session ID */
  sessionId: string;

  /** Start time */
  startTime: Date;
}

/**
 * Validates review configuration
 */
export function validateReviewConfig(config: ReviewConfig): void {
  if (!config.files || config.files.length === 0) {
    throw new ReviewEngineError(
      REVIEW_ENGINE_ERROR_CODES.INVALID_CONFIG,
      'Review config must specify at least one file'
    );
  }

  if (!config.personas || config.personas.length === 0) {
    throw new ReviewEngineError(
      REVIEW_ENGINE_ERROR_CODES.INVALID_CONFIG,
      'Review config must specify at least one persona'
    );
  }

  if (!config.mode) {
    throw new ReviewEngineError(
      REVIEW_ENGINE_ERROR_CODES.INVALID_CONFIG,
      'Review config must specify a mode'
    );
  }
}

/**
 * Prepares review context by scanning files
 */
export async function prepareContext(
  config: ReviewConfig,
  cwd: string
): Promise<ReviewContext> {
  const sessionId = randomUUID();
  const startTime = new Date();

  // Determine scan mode based on review mode
  let fileScanMode: FileScanMode = config.fileScanMode || 'full';
  if (config.mode === 'quick' && !config.fileScanMode) {
    fileScanMode = 'changed'; // Quick mode defaults to changed files only
  }

  // Scan files
  const files = await scanFiles(config.files, cwd, {
    mode: fileScanMode,
    contextLines: config.options?.contextLines,
    gitRef: 'HEAD',
    maxFileSize: 1024 * 1024, // 1MB default max file size
  });

  if (files.length === 0) {
    throw new ReviewEngineError(
      REVIEW_ENGINE_ERROR_CODES.NO_FILES,
      `No files found to review in: ${config.files.join(', ')}`
    );
  }

  return {
    files,
    cwd,
    sessionId,
    startTime,
  };
}

/**
 * Runs a single persona review
 */
export async function runPersonaReview(
  persona: string,
  input: PersonaReviewInput,
  reviewer: ReviewerFunction
): Promise<PersonaReviewOutput> {
  return tracer.startActiveSpan(`persona.evaluate`, (span) => {
    span.setAttribute('persona.name', persona);

    const execute = async (): Promise<PersonaReviewOutput> => {
      try {
        const result = await reviewer(input);

        // Compute aggregate score from findings
        const totalFindings = result.findings.length;
        const pass = !result.errors || result.errors.length === 0;
        span.setAttribute('evaluation.findings', totalFindings);
        span.setAttribute('evaluation.pass', pass);
        if (totalFindings > 0) {
          const avgScore = result.findings.reduce(
            (sum, f) => sum + (f.confidence ?? 1.0), 0
          ) / totalFindings;
          span.setAttribute('evaluation.score', avgScore);
        }

        return result;
      } catch (error) {
        span.setStatus({ code: SpanStatusCode.ERROR, message: String(error) });
        span.setAttribute('evaluation.pass', false);
        span.setAttribute('evaluation.score', 0);

        // Return failed result instead of throwing
        return {
          persona,
          findings: [],
          cost: {
            persona,
            cost: 0,
            inputTokens: 0,
            outputTokens: 0,
          },
          errors: [
            {
              code: REVIEW_ENGINE_ERROR_CODES.REVIEWER_ERROR,
              message: `Persona ${persona} failed: ${error instanceof Error ? error.message : String(error)}`,
              persona,
              details: error,
            },
          ],
        };
      }
    };

    return execute().finally(() => span.end());
  });
}

/**
 * Creates review summary from findings
 */
export function createSummary(
  filesReviewed: number,
  findings: Finding[]
): ReviewSummary {
  const findingsBySeverity: Record<Severity, number> = {
    critical: 0,
    high: 0,
    medium: 0,
    low: 0,
    info: 0,
  };

  for (const finding of findings) {
    findingsBySeverity[finding.severity]++;
  }

  return {
    filesReviewed,
    totalFindings: findings.length,
    findingsBySeverity,
  };
}

/**
 * Groups findings by file
 */
export function groupFindingsByFile(
  findings: Finding[]
): Record<string, Finding[]> {
  const grouped: Record<string, Finding[]> = {};

  for (const finding of findings) {
    if (!grouped[finding.file]) {
      grouped[finding.file] = [];
    }
    grouped[finding.file].push(finding);
  }

  return grouped;
}

/**
 * Gets vote weight for a persona tier
 * Tier 1 = 3x weight, Tier 2 = 1x weight, Tier 3 = 0.5x weight
 */
export function getVoteWeight(tier: 1 | 2 | 3 | undefined): number {
  if (tier === 1) return 3.0;
  if (tier === 3) return 0.5;
  return 1.0; // Default tier 2
}

/**
 * Aggregates votes for findings based on persona decisions and tier weighting
 * Returns GO if weighted_vote_sum / total_weight > threshold
 */
export function aggregateVotes(
  findings: Finding[],
  personas: Persona[],
  threshold: number = 0.5
): Finding[] {
  // Build persona tier lookup
  const personaTiers = new Map<string, number>();
  for (const persona of personas) {
    personaTiers.set(persona.name, getVoteWeight(persona.tier));
  }

  // Group findings by unique identifier (file + line + title)
  const findingGroups = new Map<string, Finding[]>();
  for (const finding of findings) {
    const key = `${finding.file}:${finding.line || 0}:${finding.title}`;
    if (!findingGroups.has(key)) {
      findingGroups.set(key, []);
    }
    findingGroups.get(key)!.push(finding);
  }

  // Aggregate votes for each finding group
  const aggregated: Finding[] = [];
  for (const [_key, group] of findingGroups) {
    let goVotes = 0;
    let noGoVotes = 0;
    let totalWeight = 0;

    // Collect personas that reported this finding
    const reportingPersonas = new Set<string>();
    for (const finding of group) {
      for (const persona of finding.personas) {
        reportingPersonas.add(persona);
      }
    }

    // Calculate weighted votes
    for (const finding of group) {
      for (const persona of finding.personas) {
        const weight = personaTiers.get(persona) || 1.0;
        totalWeight += weight;

        if (finding.decision === 'GO') {
          goVotes += weight;
        } else if (finding.decision === 'NO-GO') {
          noGoVotes += weight;
        } else {
          // No decision = abstain (still counts toward total weight)
          // Default to GO for neutral findings
          goVotes += weight;
        }
      }
    }

    // Calculate final decision
    const goRatio = totalWeight > 0 ? goVotes / totalWeight : 0.5;
    const finalDecision: 'GO' | 'NO-GO' = goRatio > threshold ? 'GO' : 'NO-GO';

    // Take the first finding as the base and merge data
    const baseFinding = group[0];
    const mergedFinding: Finding = {
      ...baseFinding,
      decision: finalDecision,
      personas: Array.from(reportingPersonas),
      metadata: {
        ...baseFinding.metadata,
        voteAggregation: {
          goVotes,
          noGoVotes,
          totalWeight,
          goRatio,
          threshold,
        },
      },
    };

    aggregated.push(mergedFinding);
  }

  return aggregated;
}

/**
 * Aggregates persona results into final review result
 */
export function aggregateResults(
  config: ReviewConfig,
  context: ReviewContext,
  personaResults: PersonaReviewOutput[]
): ReviewResult {
  const endTime = new Date();

  // Collect all findings
  let allFindings: Finding[] = [];
  for (const result of personaResults) {
    allFindings.push(...result.findings);
  }

  // Deduplicate if enabled
  let duplicatesRemoved = 0;
  if (config.options?.deduplicate !== false) {
    const threshold = config.options?.similarityThreshold || 0.8;
    const result = deduplicateFindings(allFindings, threshold);
    allFindings = result.findings;
    duplicatesRemoved = result.duplicatesRemoved;
  }

  // Aggregate votes if any findings have decisions
  const hasVotes = allFindings.some(f => f.decision !== undefined);
  if (hasVotes) {
    const voteThreshold = (config.options as any)?.voteThreshold || 0.5;
    allFindings = aggregateVotes(allFindings, config.personas, voteThreshold);
  }

  // Filter by minimum confidence if specified
  const minConfidence = (config.options as any)?.minConfidence;
  if (minConfidence !== undefined && minConfidence > 0) {
    allFindings = allFindings.filter(f => {
      const confidence = f.confidence ?? 1.0; // Default to 1.0 if no confidence set
      return confidence >= minConfidence;
    });
  }

  // Calculate total cost
  const byPersona: Record<string, PersonaCost> = {};
  let totalCost = 0;
  let totalTokens = 0;

  for (const result of personaResults) {
    byPersona[result.persona] = result.cost;
    totalCost += result.cost.cost;
    totalTokens += result.cost.inputTokens + result.cost.outputTokens;
  }

  const cost: CostInfo = {
    totalCost,
    totalTokens,
    byPersona,
  };

  // Collect errors
  const errors = personaResults
    .flatMap(r => r.errors || [])
    .filter(e => e !== undefined);

  // Create summary
  const summary = createSummary(context.files.length, allFindings);
  if (duplicatesRemoved > 0) {
    summary.deduplicated = duplicatesRemoved;
  }

  // Group by file
  const findingsByFile = groupFindingsByFile(allFindings);

  return {
    sessionId: context.sessionId,
    startTime: context.startTime,
    endTime,
    config,
    findings: allFindings,
    findingsByFile,
    summary,
    cost,
    errors: errors.length > 0 ? errors : undefined,
  };
}

/**
 * Executes reviews using the legacy (non-sub-agent) approach
 */
async function executeLegacyReview(
  config: ReviewConfig,
  context: ReviewContext,
  reviewer: ReviewerFunction,
  parallel: boolean
): Promise<PersonaReviewOutput[]> {
  if (parallel) {
    // Run persona reviews in parallel
    const reviewPromises = config.personas.map(persona => {
      const input: PersonaReviewInput = {
        persona,
        files: context.files,
        mode: config.mode,
        options: config.options || {},
      };

      return runPersonaReview(persona.name, input, reviewer);
    });

    return await Promise.all(reviewPromises);
  } else {
    // Run persona reviews sequentially
    const personaResults: PersonaReviewOutput[] = [];

    for (const persona of config.personas) {
      const input: PersonaReviewInput = {
        persona,
        files: context.files,
        mode: config.mode,
        options: config.options || {},
      };

      const result = await runPersonaReview(persona.name, input, reviewer);
      personaResults.push(result);
    }

    return personaResults;
  }
}

/**
 * Main review function - orchestrates the entire review process
 */
export async function reviewFiles(
  config: ReviewConfig,
  cwd: string,
  reviewer: ReviewerFunction,
  options?: {
    /** Run personas in parallel (default: true) */
    parallel?: boolean;
    /** Cost sink for tracking review costs */
    costSink?: CostSink;
    /** Metadata to include with cost tracking */
    costMetadata?: CostMetadata;
    /** Use sub-agents with prompt caching (default: true) */
    useSubAgents?: boolean;
    /** API key for sub-agents (Anthropic provider) */
    apiKey?: string;
    /** Model to use for sub-agents */
    model?: string;
    /** Cache TTL ('5min' | '1h') - auto-selected if not specified */
    cacheTtl?: '5min' | '1h';
    /** Enable automatic TTL selection (default: true) */
    autoTtl?: boolean;
    /** VertexAI project ID (VertexAI provider) */
    vertexProject?: string;
    /** VertexAI location (default: 'us-east5') */
    vertexLocation?: string;
    /** Path to GCP service account key JSON (optional for VertexAI) */
    vertexKeyFilename?: string;
  }
): Promise<ReviewResult> {
  // Validate configuration
  validateReviewConfig(config);

  // Prepare context
  const context = await prepareContext(config, cwd);

  const parallel = options?.parallel !== false; // Default to parallel
  const useSubAgents = options?.useSubAgents !== false; // Default to true

  let personaResults: PersonaReviewOutput[];

  // Try to use sub-agents if enabled and credentials are available
  const hasAnthropicKey = options?.apiKey || config.options?.apiKey;
  const hasVertexProject = options?.vertexProject || (config.options as any)?.vertexProject;

  if (useSubAgents && (hasAnthropicKey || hasVertexProject)) {
    try {
      const apiKey = options?.apiKey || config.options?.apiKey;
      const vertexProject = options?.vertexProject || (config.options as any)?.vertexProject;
      const vertexLocation = options?.vertexLocation || (config.options as any)?.vertexLocation;
      const vertexKeyFilename = options?.vertexKeyFilename || (config.options as any)?.vertexKeyFilename;
      const model = options?.model || config.options?.model;

      // Determine provider type (Anthropic takes precedence if both provided)
      const providerType = apiKey ? 'anthropic' : 'vertexai-claude';

      // Log provider selection in verbose mode
      if (process.env.VERBOSE === 'true') {
        console.error(`[REVIEW-ENGINE] Using sub-agents with ${providerType} provider`);
        console.error(`[REVIEW-ENGINE] Model: ${model || (providerType === 'anthropic' ? 'claude-3-5-sonnet-20241022' : 'claude-sonnet-4-5@20250929')}`);
        console.error(`[REVIEW-ENGINE] Parallel execution: ${parallel}`);
        console.error(`[REVIEW-ENGINE] Personas: ${config.personas.map(p => p.name).join(', ')}`);
      }

      const pool = new SubAgentPool();
      const subAgentConfig: SubAgentConfig = {
        apiKey,
        vertexProject,
        vertexLocation,
        vertexKeyFilename,
        model,
        maxTokens: 4096,
        temperature: 0.0,
        timeout: 120000,
        cacheTtl: options?.cacheTtl,
        autoTtl: options?.autoTtl,
      };

      if (parallel) {
        // Run persona reviews in parallel using sub-agents
        const reviewPromises = config.personas.map(persona => {
          const input: PersonaReviewInput = {
            persona,
            files: context.files,
            mode: config.mode,
            options: config.options || {},
          };

          const subAgent = pool.get(persona, subAgentConfig);
          return subAgent.review(input).catch(error => {
            // On sub-agent error, fall back to legacy reviewer
            console.warn(`Sub-agent failed for ${persona.name}, falling back to legacy reviewer:`, error.message);
            return runPersonaReview(persona.name, input, reviewer);
          });
        });

        personaResults = await Promise.all(reviewPromises);
      } else {
        // Run persona reviews sequentially using sub-agents
        personaResults = [];

        for (const persona of config.personas) {
          const input: PersonaReviewInput = {
            persona,
            files: context.files,
            mode: config.mode,
            options: config.options || {},
          };

          try {
            const subAgent = pool.get(persona, subAgentConfig);
            const result = await subAgent.review(input);
            personaResults.push(result);
          } catch (error) {
            // On sub-agent error, fall back to legacy reviewer
            console.warn(`Sub-agent failed for ${persona.name}, falling back to legacy reviewer:`, error instanceof Error ? error.message : String(error));
            const result = await runPersonaReview(persona.name, input, reviewer);
            personaResults.push(result);
          }
        }
      }

      await pool.clear();
    } catch (error) {
      // If sub-agent pool initialization fails, fall back to legacy execution
      console.warn('Sub-agent initialization failed, falling back to legacy execution:', error instanceof Error ? error.message : String(error));
      personaResults = await executeLegacyReview(config, context, reviewer, parallel);
    }
  } else {
    // Use legacy execution (direct API calls)
    personaResults = await executeLegacyReview(config, context, reviewer, parallel);
  }

  // Check if all personas failed
  const successfulResults = personaResults.filter(
    r => !r.errors || r.errors.length === 0
  );

  if (successfulResults.length === 0) {
    const allErrors = personaResults.flatMap(r => r.errors || []);
    throw new ReviewEngineError(
      REVIEW_ENGINE_ERROR_CODES.ALL_PERSONAS_FAILED,
      'All personas failed to complete review',
      allErrors
    );
  }

  // Aggregate results
  const result = aggregateResults(config, context, personaResults);

  // Record cost if cost sink is provided
  if (options?.costSink) {
    try {
      const metadata: CostMetadata = {
        ...options.costMetadata,
        mode: config.mode,
        filesReviewed: result.summary.filesReviewed,
        totalFindings: result.summary.totalFindings,
        timestamp: result.endTime,
      };

      await options.costSink.record(result.cost, metadata);
    } catch (error) {
      // Log cost tracking errors but don't fail the review
      console.error('Warning: Failed to record cost to sink:', error);
    }
  }

  return result;
}

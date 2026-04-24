/**
 * Multi-persona review plugin for Wayfinder
 * Multi-persona code review system
 */

// Export all types
export * from './types.js';

// Export configuration utilities
export {
  loadConfig,
  loadCrossCheckConfig,
  mergeWithDefaults,
  validateConfig,
  ConfigError,
  CONFIG_ERROR_CODES,
  DEFAULT_CONFIG,
} from './config-loader.js';

// Export persona loader utilities
export {
  resolvePersonaPaths,
  loadPersonaFile,
  validatePersona,
  loadPersonasFromDir,
  loadPersonas,
  loadSpecificPersonas,
  countTokens,
  validateCacheEligibility,
  generateCacheKey,
  enrichPersonaWithCacheMetadata,
  PersonaError,
  PERSONA_ERROR_CODES,
} from './persona-loader.js';
export type { CacheConfig } from './persona-loader.js';

// Export file scanner utilities
export {
  isBinaryFile,
  shouldExcludeFile,
  scanFileFull,
  scanFileDiff,
  getGitDiff,
  parseDiffLineMapping,
  getChangedFiles,
  findFiles,
  scanFiles,
  FileScannerError,
  FILE_SCANNER_ERROR_CODES,
} from './file-scanner.js';
export type { ScanOptions } from './file-scanner.js';

// Export review engine utilities
export {
  reviewFiles,
  prepareContext,
  runPersonaReview,
  aggregateResults,
  createSummary,
  groupFindingsByFile,
  validateReviewConfig,
  ReviewEngineError,
  REVIEW_ENGINE_ERROR_CODES,
} from './review-engine.js';
export type { ReviewerFunction, ReviewContext } from './review-engine.js';

// Export Anthropic client utilities
export {
  createAnthropicReviewer,
  AnthropicClientError,
  ANTHROPIC_ERROR_CODES,
} from './anthropic-client.js';
export type { AnthropicClientConfig } from './anthropic-client.js';

// Export sub-agent orchestrator utilities
export {
  SubAgentFactory,
  SubAgentPool,
  SubAgentError,
  SUB_AGENT_ERROR_CODES,
  selectCacheTTL,
  detectSessionReviewCount,
} from './sub-agent-orchestrator.js';
export type { SubAgent, SubAgentConfig, SubAgentStats, ApiUsageStats } from './sub-agent-orchestrator.js';

// Export VertexAI client utilities
export {
  createVertexAIReviewer,
  VertexAIClientError,
  VERTEXAI_ERROR_CODES,
} from './vertex-ai-client.js';
export type { VertexAIClientConfig } from './vertex-ai-client.js';

// Export VertexAI Claude client utilities
export {
  createVertexAIClaudeReviewer,
  VertexAIClaudeClientError,
  VERTEX_CLAUDE_ERROR_CODES,
} from './vertex-ai-claude-client.js';
export type { VertexAIClaudeClientConfig } from './vertex-ai-claude-client.js';

// Export output formatters
export { formatReviewResult } from './formatters/text-formatter.js';
export type { TextFormatOptions } from './formatters/text-formatter.js';
export { formatReviewResultJSON } from './formatters/json-formatter.js';
export { formatReviewResultGitHub } from './formatters/github-formatter.js';

// Export deduplication utilities
export {
  deduplicateFindings,
  areSimilarFindings,
  mergeFindings,
} from './deduplication.js';

// Export cost sink utilities
export {
  createCostSink,
  StdoutCostSink,
  FileCostSink,
  CostSinkError,
  COST_SINK_ERROR_CODES,
} from './cost-sink.js';
export type { CostSink, CostMetadata } from './cost-sink.js';
export { GCPCostSink } from './cost-sinks/gcp-sink.js';
export type { GCPCostSinkConfig } from './cost-sinks/gcp-sink.js';

// Export cache alert tracking
export {
  CacheAlertTracker,
} from './cache-alert-tracker.js';
export type {
  CacheAlert,
  AlertSeverity,
  CacheAlertConfig,
} from './cache-alert-tracker.js';

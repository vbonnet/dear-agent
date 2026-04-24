/**
 * Core type definitions for the multi-persona-review plugin
 */

// ============================================================================
// Review Configuration Types
// ============================================================================

export type ReviewMode = 'quick' | 'thorough' | 'custom';

export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info';

export type FileScanMode = 'full' | 'diff' | 'changed';

export interface ReviewConfig {
  /** Files to review */
  files: string[];

  /** Personas to use for review */
  personas: Persona[];

  /** Review mode */
  mode: ReviewMode;

  /** File scan mode */
  fileScanMode?: FileScanMode;

  /** Additional configuration options */
  options?: ReviewOptions;
}

export interface ReviewOptions {
  /** Maximum number of files to review */
  maxFiles?: number;

  /** Context lines before and after diff */
  contextLines?: number;

  /** Enable auto-fix for simple issues */
  autoFix?: boolean;

  /** Enable deduplication of findings */
  deduplicate?: boolean;

  /** Similarity threshold for deduplication (0-1) */
  similarityThreshold?: number;

  /** Agency-Agents: Vote threshold for GO decision (0-1, default: 0.5) */
  voteThreshold?: number;

  /** Agency-Agents: Minimum confidence for findings (0-1) */
  minConfidence?: number;

  /** Anthropic API key */
  apiKey?: string;

  /** Model to use */
  model?: string;

  /** Cost tracking sink configuration */
  costSink?: CostSinkConfig;

  /** Deliberation configuration (opt-in adversarial debate between personas) */
  deliberation?: {
    enabled: boolean;
    maxRounds?: number;
    maxDeliberationTokens?: number;
    timeoutMs?: number;
    convergenceThreshold?: number;
    leadReviewerModel?: string;
  };
}

export interface CostSinkConfig {
  /** Sink type: stdout (default), gcp, aws, datadog, webhook, file */
  type: 'stdout' | 'gcp' | 'aws' | 'datadog' | 'webhook' | 'file';

  /** Configuration specific to the sink type */
  config?: Record<string, unknown>;
}

// ============================================================================
// Persona Types
// ============================================================================

export interface Persona {
  /** Unique identifier (e.g., 'security-engineer') */
  name: string;

  /** Human-readable name */
  displayName: string;

  /** Persona version */
  version: string;

  /** Description of the persona's expertise */
  description: string;

  /** Focus areas for this persona */
  focusAreas: string[];

  /** System prompt for the persona */
  prompt: string;

  /** Severity levels this persona typically uses */
  severityLevels?: Severity[];

  /** Whether this persona can request git history */
  gitHistoryAccess?: boolean;

  /** Agency-Agents: Persona tier for vote weighting (1=3x, 2=1x, 3=0.5x, default=2) */
  tier?: 1 | 2 | 3;

  /** Custom configuration for this persona */
  config?: Record<string, unknown>;

  /** Cache metadata (populated by persona loader) */
  cacheMetadata?: PersonaCacheMetadata;

  /** Deliberation: Agent temperament (e.g., 'risk-averse', 'efficiency-first') */
  temperament?: string;

  /** Deliberation: Reasoning patterns (e.g., ['devil\'s advocate', 'risk-first']) */
  reasoningPatterns?: string[];

  /** Deliberation: Decision-making heuristics */
  decisionHeuristics?: string[];

  /** Deliberation: Deep expertise areas (more specific than focusAreas) */
  expertise?: string[];

  /** Deliberation: Preferred model for this persona */
  modelPreference?: string;
}

export interface PersonaCacheMetadata {
  /** Whether this persona is eligible for prompt caching (≥1,024 tokens) */
  cacheEligible: boolean;

  /** Token count of the prompt */
  tokenCount: number;

  /** Cache key for versioning and invalidation */
  cacheKey: string;
}

// ============================================================================
// Finding Types
// ============================================================================

export interface Finding {
  /** Unique identifier for this finding */
  id: string;

  /** File path relative to repository root */
  file: string;

  /** Line number (1-indexed) */
  line?: number;

  /** End line for multi-line findings */
  lineEnd?: number;

  /** Severity level */
  severity: Severity;

  /** Personas that reported this finding */
  personas: string[];

  /** Short title/summary */
  title: string;

  /** Detailed description */
  description: string;

  /** Categories/tags for this finding */
  categories?: string[];

  /** Suggested fix (if available) */
  suggestedFix?: SuggestedFix;

  /** Confidence level (0-1) */
  confidence?: number;

  /** Agency-Agents: Voting decision (GO = approve, NO-GO = block) */
  decision?: 'GO' | 'NO-GO';

  /** Agency-Agents: Alternative approaches for lateral thinking */
  alternatives?: string[];

  /** Agency-Agents: Whether finding is outside persona's expertise scope */
  outOfScope?: boolean;

  /** Additional metadata */
  metadata?: Record<string, unknown>;
}

export interface SuggestedFix {
  /** Type of fix */
  type: 'replace' | 'insert' | 'delete';

  /** Original code (for replace) */
  original?: string;

  /** Replacement code */
  replacement: string;

  /** Explanation of the fix */
  explanation: string;

  /** Whether this fix requires user confirmation */
  requiresConfirmation: boolean;
}

// ============================================================================
// Review Result Types
// ============================================================================

export interface ReviewResult {
  /** Session identifier */
  sessionId: string;

  /** Timestamp when review started */
  startTime: Date;

  /** Timestamp when review completed */
  endTime: Date;

  /** Review configuration used */
  config: ReviewConfig;

  /** All findings (deduplicated if enabled) */
  findings: Finding[];

  /** Findings grouped by file */
  findingsByFile: Record<string, Finding[]>;

  /** Summary statistics */
  summary: ReviewSummary;

  /** Cost information */
  cost: CostInfo;

  /** Any errors that occurred */
  errors?: ReviewError[];

  /** Deliberation memo (when deliberation is enabled) */
  deliberationMemo?: {
    sessionId: string;
    decision: 'GO' | 'NO-GO' | 'CONDITIONAL';
    summary: string;
    findings: Finding[];
    tensions: Array<{ id: string; topic: string; resolved: boolean; resolution?: string }>;
    recommendations: string[];
    totalTokens: number;
    totalCost: number;
    durationMs: number;
  };
}

export interface ReviewSummary {
  /** Total number of files reviewed */
  filesReviewed: number;

  /** Total number of findings */
  totalFindings: number;

  /** Findings by severity */
  findingsBySeverity: Record<Severity, number>;

  /** Number of auto-fixed issues */
  autoFixed?: number;

  /** Number of deduplicated findings */
  deduplicated?: number;
}

export interface CostInfo {
  /** Total cost in USD */
  totalCost: number;

  /** Total tokens used */
  totalTokens: number;

  /** Cost breakdown by persona */
  byPersona: Record<string, PersonaCost>;
}

export interface PersonaCost {
  /** Persona name */
  persona: string;

  /** Cost in USD */
  cost: number;

  /** Input tokens */
  inputTokens: number;

  /** Output tokens */
  outputTokens: number;

  /** Cache creation (write) tokens - tokens written to cache */
  cacheCreationInputTokens?: number;

  /** Cache read (hit) tokens - tokens read from cache */
  cacheReadInputTokens?: number;
}

export interface ReviewError {
  /** Error code */
  code: string;

  /** Error message */
  message: string;

  /** File that caused the error (if applicable) */
  file?: string;

  /** Persona that encountered the error (if applicable) */
  persona?: string;

  /** Additional error details */
  details?: unknown;
}

// ============================================================================
// Configuration File Types
// ============================================================================

export interface WayfinderConfig {
  /** Multi-persona review plugin configuration */
  crossCheck?: CrossCheckConfig;
}

export interface CrossCheckConfig {
  /** Default personas to use */
  defaultPersonas?: string[];

  /** Default review mode */
  defaultMode?: ReviewMode;

  /** Persona search paths */
  personaPaths?: string[];

  /** Cost tracking configuration */
  costTracking?: CostSinkConfig;

  /** Default options */
  options?: ReviewOptions;

  /** GitHub-specific configuration */
  github?: GitHubConfig;
}

export interface GitHubConfig {
  /** Enable multi-persona-review on pull requests */
  enabled?: boolean;

  /** Only run on changed files */
  changedFilesOnly?: boolean;

  /** Skip draft PRs */
  skipDrafts?: boolean;

  /** Concurrency limit */
  concurrency?: number;

  /** File patterns to include */
  include?: string[];

  /** File patterns to exclude */
  exclude?: string[];
}

// ============================================================================
// Internal Types
// ============================================================================

export interface PersonaReviewInput {
  /** Persona to use */
  persona: Persona;

  /** Files to review with their content */
  files: FileContent[];

  /** Review mode */
  mode: ReviewMode;

  /** Options */
  options: ReviewOptions;
}

export interface PersonaReviewOutput {
  /** Persona that performed the review */
  persona: string;

  /** Findings from this persona */
  findings: Finding[];

  /** Cost information */
  cost: PersonaCost;

  /** Any errors */
  errors?: ReviewError[];

  /** Cache performance metrics (when using sub-agents) */
  cacheMetrics?: CacheMetrics;
}

export interface CacheMetrics {
  /** Tokens written to cache */
  cacheCreationTokens: number;

  /** Tokens read from cache */
  cacheReadTokens: number;

  /** Regular input tokens (not cached) */
  inputTokens: number;

  /** Output tokens */
  outputTokens: number;

  /** Whether cache was hit */
  cacheHit: boolean;

  /** Cache efficiency (0-1, higher is better) */
  cacheEfficiency: number;
}

export interface FileContent {
  /** File path */
  path: string;

  /** File content (or diff) */
  content: string;

  /** Whether this is a diff or full content */
  isDiff: boolean;

  /** Line mapping (for diffs) */
  lineMapping?: LineMapping;
}

export interface LineMapping {
  /** Maps diff line numbers to original file line numbers */
  diffToOriginal: Record<number, number>;

  /** Maps original file line numbers to diff line numbers */
  originalToDiff: Record<number, number>;
}

// ============================================================================
// Git History Types
// ============================================================================

export interface GitHistoryRequest {
  /** File path */
  file: string;

  /** Number of commits to retrieve */
  limit?: number;

  /** Line number (for git blame) */
  line?: number;
}

export interface GitCommit {
  /** Commit hash */
  hash: string;

  /** Author name */
  author: string;

  /** Author email */
  authorEmail: string;

  /** Commit date */
  date: Date;

  /** Commit message */
  message: string;

  /** Files changed in this commit */
  files?: string[];
}

export interface GitBlameInfo {
  /** Line number */
  line: number;

  /** Commit that last modified this line */
  commit: GitCommit;

  /** Original line number */
  originalLine: number;
}

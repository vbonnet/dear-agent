/**
 * Schema Filter - MCP Tool Schema Filtering
 *
 * Filters MCP tool schemas based on Requirement Envelope from Intent Analyzer.
 * Implements in-memory caching with 5-minute TTL for performance.
 *
 * Performance Target: <20ms p99
 * Cache TTL: 5 minutes
 */

// ============================================================================
// Type Definitions
// ============================================================================

/**
 * Requirement Envelope from Intent Analyzer
 */
export interface RequirementEnvelope {
  action: 'CREATE' | 'READ' | 'UPDATE' | 'DELETE' | 'SEARCH' | 'UNKNOWN';
  target?: string;
  service?: string;
  scope?: string[];
  confidence: number; // 0.0-1.0
  raw_intent: string;
  parsed_at: string; // ISO8601
  fallback_to_all: boolean;
}

/**
 * MCP Tool Schema (simplified from MCP protocol)
 */
export interface MCPToolSchema {
  name: string;
  description?: string;
  inputSchema?: {
    type: string;
    properties?: Record<string, unknown>;
    required?: string[];
  };
}

/**
 * Cache entry for filtered schemas
 */
interface CacheEntry {
  schemas: MCPToolSchema[];
  timestamp: number;
}

/**
 * Schema filter configuration
 */
export interface SchemaFilterConfig {
  cacheTTLMs?: number; // Default: 5 minutes
  traceMode?: boolean; // Enable debug logging
}

// ============================================================================
// Constants
// ============================================================================

const DEFAULT_CACHE_TTL_MS = 5 * 60 * 1000; // 5 minutes
const CONFIDENCE_THRESHOLD = 0.7;

// ============================================================================
// Schema Filter Class
// ============================================================================

export class SchemaFilter {
  private cache = new Map<string, CacheEntry>();
  private cacheTTLMs: number;
  private traceMode: boolean;

  constructor(config: SchemaFilterConfig = {}) {
    this.cacheTTLMs = config.cacheTTLMs ?? DEFAULT_CACHE_TTL_MS;
    this.traceMode = config.traceMode ?? false;
  }

  /**
   * Filters MCP tool schemas based on Requirement Envelope
   *
   * Algorithm:
   * 1. If fallback_to_all is true, return all schemas
   * 2. Filter by service (name prefix matching)
   * 3. Filter by action (if confidence > 0.7)
   * 4. Fallback to all schemas if no matches
   */
  filterSchemas(
    envelope: RequirementEnvelope,
    allSchemas: MCPToolSchema[]
  ): MCPToolSchema[] {
    const startTime = this.traceMode ? performance.now() : 0;

    // Check cache first
    const cacheKey = this.getCacheKey(envelope);
    const cached = this.getFromCache(cacheKey);
    if (cached) {
      this.trace('Cache hit', { cacheKey });
      return cached;
    }

    // Filter schemas
    const filtered = this.performFiltering(envelope, allSchemas);

    // Cache result
    this.cache.set(cacheKey, {
      schemas: filtered,
      timestamp: Date.now(),
    });

    if (this.traceMode) {
      const duration = performance.now() - startTime;
      this.trace('Filter complete', {
        duration: `${duration.toFixed(2)}ms`,
        input: allSchemas.length,
        output: filtered.length,
        envelope: {
          action: envelope.action,
          service: envelope.service,
          confidence: envelope.confidence,
        },
      });
    }

    return filtered;
  }

  /**
   * Core filtering logic
   */
  private performFiltering(
    envelope: RequirementEnvelope,
    allSchemas: MCPToolSchema[]
  ): MCPToolSchema[] {
    // Fallback if intent unclear
    if (envelope.fallback_to_all) {
      this.trace('Fallback: intent unclear', { confidence: envelope.confidence });
      return allSchemas;
    }

    let filtered = allSchemas;

    // Filter by service
    if (envelope.service) {
      const servicePrefix = `${envelope.service}_`;
      filtered = filtered.filter((s) =>
        s.name.startsWith(servicePrefix)
      );
      this.trace('Filter by service', {
        service: envelope.service,
        count: filtered.length,
      });
    }

    // Filter by action (if confident)
    if (envelope.confidence > CONFIDENCE_THRESHOLD && envelope.action !== 'UNKNOWN') {
      const action = envelope.action.toLowerCase();
      filtered = filtered.filter(
        (s) =>
          s.name.includes(`_${action}_`) ||
          (s.description?.toLowerCase().includes(action) ?? false)
      );
      this.trace('Filter by action', {
        action,
        count: filtered.length,
      });
    }

    // Fallback if no matches
    if (filtered.length === 0) {
      this.trace('Fallback: no matches', { returning: allSchemas.length });
      return allSchemas;
    }

    return filtered;
  }

  /**
   * Get cached schemas if not expired
   */
  private getFromCache(key: string): MCPToolSchema[] | null {
    const entry = this.cache.get(key);
    if (!entry) {
      return null;
    }

    const age = Date.now() - entry.timestamp;
    if (age > this.cacheTTLMs) {
      this.cache.delete(key);
      this.trace('Cache expired', { age: `${age}ms` });
      return null;
    }

    return entry.schemas;
  }

  /**
   * Generate cache key from envelope
   */
  private getCacheKey(envelope: RequirementEnvelope): string {
    return JSON.stringify({
      action: envelope.action,
      service: envelope.service,
      confidence: envelope.confidence,
      fallback: envelope.fallback_to_all,
    });
  }

  /**
   * Debug logging
   */
  private trace(message: string, data?: unknown): void {
    if (this.traceMode) {
      console.error(`[SchemaFilter] ${message}`, data ? JSON.stringify(data) : '');
    }
  }

  /**
   * Clear the cache (for testing)
   */
  clearCache(): void {
    this.cache.clear();
  }

  /**
   * Get cache statistics
   */
  getCacheStats(): { size: number; entries: Array<{ key: string; age: number }> } {
    const now = Date.now();
    const entries = Array.from(this.cache.entries()).map(([key, entry]) => ({
      key,
      age: now - entry.timestamp,
    }));

    return { size: this.cache.size, entries };
  }
}

// ============================================================================
// Factory Functions
// ============================================================================

/**
 * Create a schema filter instance
 */
export function createSchemaFilter(config?: SchemaFilterConfig): SchemaFilter {
  return new SchemaFilter(config);
}

/**
 * Standalone filtering function (no caching)
 */
export function filterSchemas(
  envelope: RequirementEnvelope,
  allSchemas: MCPToolSchema[]
): MCPToolSchema[] {
  const filter = new SchemaFilter({ cacheTTLMs: 0 }); // Disable cache
  return filter.filterSchemas(envelope, allSchemas);
}

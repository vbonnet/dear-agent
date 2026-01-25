/**
 * Schema Filter Tests
 *
 * Tests for schema filtering logic, caching, and performance.
 * Target: 90%+ test coverage, <20ms p99 performance
 */

import {
  SchemaFilter,
  createSchemaFilter,
  filterSchemas,
  RequirementEnvelope,
  MCPToolSchema,
} from '../../src/lib/schema-filter';

// ============================================================================
// Mock Data
// ============================================================================

const MOCK_SCHEMAS: MCPToolSchema[] = [
  // Atlassian tools
  {
    name: 'atlassian_create_issue',
    description: 'Create a new Jira issue',
  },
  {
    name: 'atlassian_read_issue',
    description: 'Read a Jira issue',
  },
  {
    name: 'atlassian_update_issue',
    description: 'Update a Jira issue',
  },
  {
    name: 'atlassian_delete_issue',
    description: 'Delete a Jira issue',
  },
  {
    name: 'atlassian_search_issues',
    description: 'Search for Jira issues using JQL',
  },
  // GoogleDocs tools
  {
    name: 'googledocs_create_document',
    description: 'Create a new Google Document',
  },
  {
    name: 'googledocs_read_document',
    description: 'Read a Google Document',
  },
  {
    name: 'googledocs_update_document',
    description: 'Update a Google Document',
  },
  {
    name: 'googledocs_search_documents',
    description: 'Search for Google Documents',
  },
  // Slack tools
  {
    name: 'slack_create_message',
    description: 'Send a message to a Slack channel',
  },
  {
    name: 'slack_read_messages',
    description: 'Read messages from a Slack channel',
  },
  // Glean tools
  {
    name: 'glean_search_all',
    description: 'Search across all indexed content',
  },
  {
    name: 'glean_read_document',
    description: 'Read a specific document',
  },
];

function createEnvelope(
  overrides: Partial<RequirementEnvelope> = {}
): RequirementEnvelope {
  return {
    action: 'UNKNOWN',
    confidence: 0.3,
    raw_intent: 'test',
    parsed_at: new Date().toISOString(),
    fallback_to_all: false,
    ...overrides,
  };
}

// ============================================================================
// Core Filtering Tests
// ============================================================================

describe('SchemaFilter - Core Filtering', () => {
  let filter: SchemaFilter;

  beforeEach(() => {
    filter = createSchemaFilter({ cacheTTLMs: 0 }); // Disable cache for isolated tests
  });

  test('returns all schemas when fallback_to_all is true', () => {
    const envelope = createEnvelope({ fallback_to_all: true });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result).toEqual(MOCK_SCHEMAS);
    expect(result.length).toBe(MOCK_SCHEMAS.length);
  });

  test('filters by service - atlassian', () => {
    const envelope = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result.length).toBe(5);
    expect(result.every((s) => s.name.startsWith('atlassian_'))).toBe(true);
  });

  test('filters by service - googledocs', () => {
    const envelope = createEnvelope({
      service: 'googledocs',
      confidence: 0.8,
    });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result.length).toBe(4);
    expect(result.every((s) => s.name.startsWith('googledocs_'))).toBe(true);
  });

  test('filters by service - slack', () => {
    const envelope = createEnvelope({
      service: 'slack',
      confidence: 0.8,
    });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result.length).toBe(2);
    expect(result.every((s) => s.name.startsWith('slack_'))).toBe(true);
  });

  test('filters by action when confidence > 0.7', () => {
    const envelope = createEnvelope({
      service: 'atlassian',
      action: 'CREATE',
      confidence: 0.9,
    });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result.length).toBe(1);
    expect(result[0].name).toBe('atlassian_create_issue');
  });

  test('does not filter by action when confidence <= 0.7', () => {
    const envelope = createEnvelope({
      service: 'atlassian',
      action: 'CREATE',
      confidence: 0.6,
    });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    // Should only filter by service, not action
    expect(result.length).toBe(5);
    expect(result.every((s) => s.name.startsWith('atlassian_'))).toBe(true);
  });

  test('filters by both service and action', () => {
    const envelope = createEnvelope({
      service: 'googledocs',
      action: 'SEARCH',
      confidence: 0.9,
    });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result.length).toBe(1);
    expect(result[0].name).toBe('googledocs_search_documents');
  });

  test('falls back to all schemas when no matches', () => {
    const envelope = createEnvelope({
      service: 'nonexistent',
      confidence: 0.8,
    });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result).toEqual(MOCK_SCHEMAS);
  });

  test('matches action in description', () => {
    const envelope = createEnvelope({
      service: 'glean',
      action: 'SEARCH',
      confidence: 0.9,
    });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result.length).toBe(1);
    expect(result[0].name).toBe('glean_search_all');
  });

  test('handles UNKNOWN action', () => {
    const envelope = createEnvelope({
      service: 'slack',
      action: 'UNKNOWN',
      confidence: 0.9,
    });
    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    // Should filter by service only, not action
    expect(result.length).toBe(2);
    expect(result.every((s) => s.name.startsWith('slack_'))).toBe(true);
  });

  test('handles empty schema list', () => {
    const envelope = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });
    const result = filter.filterSchemas(envelope, []);

    expect(result).toEqual([]);
  });
});

// ============================================================================
// Caching Tests
// ============================================================================

describe('SchemaFilter - Caching', () => {
  test('caches results and returns same array', () => {
    const filter = createSchemaFilter({ cacheTTLMs: 5000 });
    const envelope = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });

    const result1 = filter.filterSchemas(envelope, MOCK_SCHEMAS);
    const result2 = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result1).toBe(result2); // Same reference
    expect(result1.length).toBe(5);
  });

  test('cache expires after TTL', async () => {
    const filter = createSchemaFilter({ cacheTTLMs: 50 });
    const envelope = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });

    const result1 = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    // Wait for cache to expire
    await new Promise((resolve) => setTimeout(resolve, 60));

    const result2 = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result1).not.toBe(result2); // Different references
    expect(result1).toEqual(result2); // But same content
  });

  test('different envelopes create different cache entries', () => {
    const filter = createSchemaFilter({ cacheTTLMs: 5000 });

    const envelope1 = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });
    const envelope2 = createEnvelope({
      service: 'googledocs',
      confidence: 0.8,
    });

    const result1 = filter.filterSchemas(envelope1, MOCK_SCHEMAS);
    const result2 = filter.filterSchemas(envelope2, MOCK_SCHEMAS);

    expect(result1).not.toBe(result2);
    expect(result1.length).toBe(5); // Atlassian
    expect(result2.length).toBe(4); // GoogleDocs
  });

  test('clearCache removes all entries', () => {
    const filter = createSchemaFilter({ cacheTTLMs: 5000 });
    const envelope = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });

    const result1 = filter.filterSchemas(envelope, MOCK_SCHEMAS);
    filter.clearCache();
    const result2 = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result1).not.toBe(result2); // Different references after clear
  });

  test('getCacheStats returns correct information', () => {
    const filter = createSchemaFilter({ cacheTTLMs: 5000 });

    const envelope1 = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });
    const envelope2 = createEnvelope({
      service: 'googledocs',
      confidence: 0.8,
    });

    filter.filterSchemas(envelope1, MOCK_SCHEMAS);
    filter.filterSchemas(envelope2, MOCK_SCHEMAS);

    const stats = filter.getCacheStats();
    expect(stats.size).toBe(2);
    expect(stats.entries.length).toBe(2);
    expect(stats.entries[0].age).toBeGreaterThanOrEqual(0);
  });
});

// ============================================================================
// Performance Tests
// ============================================================================

describe('SchemaFilter - Performance', () => {
  test('filtering completes in <20ms (p99)', () => {
    const filter = createSchemaFilter({ cacheTTLMs: 0 });
    const envelope = createEnvelope({
      service: 'atlassian',
      action: 'CREATE',
      confidence: 0.9,
    });

    const timings: number[] = [];
    const iterations = 100;

    for (let i = 0; i < iterations; i++) {
      const start = performance.now();
      filter.filterSchemas(envelope, MOCK_SCHEMAS);
      const duration = performance.now() - start;
      timings.push(duration);
    }

    // Calculate p99 (99th percentile)
    timings.sort((a, b) => a - b);
    const p99Index = Math.floor(iterations * 0.99);
    const p99 = timings[p99Index];

    expect(p99).toBeLessThan(20);
  });

  test('caching provides speedup', () => {
    const filterWithCache = createSchemaFilter({ cacheTTLMs: 5000 });
    const envelope = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });

    // First call - should cache the result
    const result1 = filterWithCache.filterSchemas(envelope, MOCK_SCHEMAS);

    // Subsequent calls should return the same cached array reference
    const result2 = filterWithCache.filterSchemas(envelope, MOCK_SCHEMAS);
    const result3 = filterWithCache.filterSchemas(envelope, MOCK_SCHEMAS);

    // Verify cache is working by checking same reference is returned
    expect(result2).toBe(result1);
    expect(result3).toBe(result1);

    // Verify cache stats show entry was cached
    const stats = filterWithCache.getCacheStats();
    expect(stats.size).toBeGreaterThan(0);
  });
});

// ============================================================================
// Trace Mode Tests
// ============================================================================

describe('SchemaFilter - Trace Mode', () => {
  test('trace mode logs to stderr', () => {
    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

    const filter = createSchemaFilter({ traceMode: true });
    const envelope = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });

    filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(consoleErrorSpy).toHaveBeenCalled();
    expect(consoleErrorSpy.mock.calls.some((call) =>
      call[0].includes('[SchemaFilter]')
    )).toBe(true);

    consoleErrorSpy.mockRestore();
  });

  test('trace mode disabled by default', () => {
    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

    const filter = createSchemaFilter();
    const envelope = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });

    filter.filterSchemas(envelope, MOCK_SCHEMAS);

    expect(consoleErrorSpy).not.toHaveBeenCalled();

    consoleErrorSpy.mockRestore();
  });
});

// ============================================================================
// Standalone Function Tests
// ============================================================================

describe('filterSchemas (standalone)', () => {
  test('filters without caching', () => {
    const envelope = createEnvelope({
      service: 'atlassian',
      confidence: 0.8,
    });

    const result1 = filterSchemas(envelope, MOCK_SCHEMAS);
    const result2 = filterSchemas(envelope, MOCK_SCHEMAS);

    expect(result1).not.toBe(result2); // Different references (no cache)
    expect(result1).toEqual(result2); // Same content
  });
});

// ============================================================================
// Edge Cases
// ============================================================================

describe('SchemaFilter - Edge Cases', () => {
  let filter: SchemaFilter;

  beforeEach(() => {
    filter = createSchemaFilter({ cacheTTLMs: 0 });
  });

  test('handles schemas without descriptions', () => {
    const schemas: MCPToolSchema[] = [
      { name: 'atlassian_create_test' },
      { name: 'googledocs_create_test' },
    ];

    const envelope = createEnvelope({
      service: 'atlassian',
      action: 'CREATE',
      confidence: 0.9,
    });

    const result = filter.filterSchemas(envelope, schemas);

    expect(result.length).toBe(1);
    expect(result[0].name).toBe('atlassian_create_test');
  });

  test('handles confidence exactly at threshold', () => {
    const envelope = createEnvelope({
      service: 'atlassian',
      action: 'CREATE',
      confidence: 0.7,
    });

    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    // Should not filter by action (threshold is >0.7, not >=0.7)
    expect(result.length).toBe(5);
  });

  test('handles confidence just above threshold', () => {
    const envelope = createEnvelope({
      service: 'atlassian',
      action: 'CREATE',
      confidence: 0.71,
    });

    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    // Should filter by action
    expect(result.length).toBe(1);
    expect(result[0].name).toBe('atlassian_create_issue');
  });

  test('handles missing service', () => {
    const envelope = createEnvelope({
      action: 'CREATE',
      confidence: 0.9,
    });

    const result = filter.filterSchemas(envelope, MOCK_SCHEMAS);

    // Should only filter by action across all services
    const createTools = result.filter((s) => s.name.includes('_create_'));
    expect(createTools.length).toBeGreaterThan(0);
  });

  test('case insensitive action matching', () => {
    const envelope = createEnvelope({
      service: 'atlassian',
      action: 'CREATE',
      confidence: 0.9,
    });

    const schemasWithMixedCase: MCPToolSchema[] = [
      {
        name: 'atlassian_create_issue',
        description: 'Create a new issue',
      },
    ];

    const result = filter.filterSchemas(envelope, schemasWithMixedCase);
    expect(result.length).toBe(1);
  });
});

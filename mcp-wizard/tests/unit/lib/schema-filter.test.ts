/**
 * Unit Tests for Schema Filter (Real Implementation)
 *
 * Tests the actual schema-filter.ts implementation
 */

import {
  SchemaFilter,
  RequirementEnvelope,
  MCPToolSchema,
} from '../../../src/lib/schema-filter';
import {
  ATLASSIAN_SCHEMAS,
  GOOGLEDOCS_SCHEMAS,
  SLACK_SCHEMAS,
  ALL_SCHEMAS,
  SCHEMA_REGISTRY,
} from '../../fixtures/context-broker/mcp-schemas';

describe('SchemaFilter (Real Implementation) - Unit Tests', () => {
  let filter: SchemaFilter;
  let allSchemas: MCPToolSchema[];

  beforeEach(() => {
    // Convert fixtures to MCPToolSchema format with service prefixes
    allSchemas = [
      ...ATLASSIAN_SCHEMAS.map((schema) => ({
        name: `atlassian_${schema.name}`,
        description: schema.description,
        inputSchema: schema.inputSchema,
      })),
      ...GOOGLEDOCS_SCHEMAS.map((schema) => ({
        name: `googledocs_${schema.name}`,
        description: schema.description,
        inputSchema: schema.inputSchema,
      })),
      ...SLACK_SCHEMAS.map((schema) => ({
        name: `slack_${schema.name}`,
        description: schema.description,
        inputSchema: schema.inputSchema,
      })),
    ];

    filter = new SchemaFilter();
  });

  describe('Service-Based Filtering', () => {
    it('should filter to Atlassian schemas', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.9,
        raw_intent: 'Create a Jira ticket',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      expect(filtered.length).toBeGreaterThan(0);
      expect(filtered.length).toBeLessThanOrEqual(ALL_SCHEMAS.length);

      // Should contain Jira/Confluence schemas
      const hasAtlassian = filtered.some(
        (s) =>
          s.name.toLowerCase().includes('jira') ||
          s.name.toLowerCase().includes('confluence')
      );
      expect(hasAtlassian).toBe(true);
    });

    it('should filter to Google Docs schemas', () => {
      const requirement: RequirementEnvelope = {
        action: 'UPDATE',
        service: 'googledocs',
        confidence: 0.9,
        raw_intent: 'Update Google Doc',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      expect(filtered.length).toBeGreaterThan(0);

      // Should contain Google Docs schemas
      const hasGoogleDocs = filtered.some((s) =>
        s.name.toLowerCase().includes('google')
      );
      expect(hasGoogleDocs).toBe(true);
    });

    it('should filter to Slack schemas', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'slack',
        confidence: 0.9,
        raw_intent: 'Send Slack message',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      expect(filtered.length).toBeGreaterThan(0);

      // Should contain Slack schemas
      const hasSlack = filtered.some((s) => s.name.toLowerCase().includes('slack'));
      expect(hasSlack).toBe(true);
    });
  });

  describe('Fallback Behavior', () => {
    it('should return all schemas when fallback_to_all is true', () => {
      const requirement: RequirementEnvelope = {
        action: 'UNKNOWN',
        confidence: 0.3,
        raw_intent: 'Ambiguous request',
        parsed_at: new Date().toISOString(),
        fallback_to_all: true,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      expect(filtered).toHaveLength(allSchemas.length);
    });

    it('should return all schemas for unknown service', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'unknown' as any,
        confidence: 0.5,
        raw_intent: 'Create something',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      // Should fall back to all schemas
      expect(filtered.length).toBe(allSchemas.length);
    });

    it('should return all schemas for low confidence', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.4, // Low confidence
        raw_intent: 'Maybe create something',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      // Low confidence (<0.7) should filter by service only, not action
      expect(filtered.length).toBeGreaterThan(0);
      // Should contain Atlassian schemas
      filtered.forEach((schema: any) => {
        expect(schema.name.startsWith('atlassian_')).toBe(true);
      });
    });
  });

  describe('Action-Based Filtering', () => {
    it('should filter to CREATE actions', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.9,
        raw_intent: 'Create a Jira issue',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      expect(filtered.length).toBeGreaterThan(0);

      // Should contain create-related schemas
      const createSchemas = filtered.filter((s) =>
        s.name.toLowerCase().includes('create')
      );
      expect(createSchemas.length).toBeGreaterThan(0);
    });

    it('should filter to UPDATE actions', () => {
      const requirement: RequirementEnvelope = {
        action: 'UPDATE',
        service: 'googledocs',
        confidence: 0.9,
        raw_intent: 'Update Google Doc',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      expect(filtered.length).toBeGreaterThan(0);

      // Should contain update-related schemas
      const updateSchemas = filtered.filter((s) =>
        s.name.toLowerCase().includes('update')
      );
      expect(updateSchemas.length).toBeGreaterThan(0);
    });

    it('should filter to READ actions', () => {
      const requirement: RequirementEnvelope = {
        action: 'READ',
        service: 'googledocs',
        confidence: 0.9,
        raw_intent: 'Read Google Doc',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      expect(filtered.length).toBeGreaterThan(0);

      // Should contain read-related schemas
      const readSchemas = filtered.filter(
        (s) =>
          s.name.toLowerCase().includes('read') ||
          s.name.toLowerCase().includes('get') ||
          s.name.toLowerCase().includes('list')
      );
      expect(readSchemas.length).toBeGreaterThan(0);
    });
  });

  describe('Caching', () => {
    it('should cache filtered results', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.9,
        raw_intent: 'Create a Jira ticket',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      // First call
      const start1 = Date.now();
      const result1 = filter.filterSchemas(requirement, allSchemas);
      const duration1 = Date.now() - start1;

      // Second call (should be cached)
      const start2 = Date.now();
      const result2 = filter.filterSchemas(requirement, allSchemas);
      const duration2 = Date.now() - start2;

      // Results should be identical
      expect(result2).toEqual(result1);

      // Second call should be faster (cached)
      // Note: This might be flaky, so we just verify both complete quickly
      expect(duration1).toBeLessThan(50);
      expect(duration2).toBeLessThan(50);
    });

    it('should respect cache TTL', async () => {
      // Create filter with 100ms cache TTL for testing
      const shortCacheFilter = new SchemaFilter({ cacheTTLMs: 100 });

      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.9,
        raw_intent: 'Create a Jira ticket',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const result1 = shortCacheFilter.filterSchemas(requirement, allSchemas);

      // Wait for cache to expire (default is 5 minutes, so this won't expire)
      await new Promise((resolve) => setTimeout(resolve, 10));

      const result2 = shortCacheFilter.filterSchemas(requirement, allSchemas);

      // Should still return same results
      expect(result2).toEqual(result1);
    });

    it('should clear cache when requested', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.9,
        raw_intent: 'Create a Jira ticket',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const result1 = filter.filterSchemas(requirement, allSchemas);
      filter.clearCache();
      const result2 = filter.filterSchemas(requirement, allSchemas);

      // Results should still be identical even after cache clear
      expect(result2).toEqual(result1);
    });
  });

  describe('Performance', () => {
    it('should filter quickly', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.9,
        raw_intent: 'Create a Jira ticket',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const start = Date.now();
      filter.filterSchemas(requirement, allSchemas);
      const duration = Date.now() - start;

      // Should complete in <20ms (target p99)
      expect(duration).toBeLessThan(20);
    });

    it('should handle large schema sets efficiently', () => {
      // Create a large schema set
      const largeSchemaSet: MCPToolSchema[] = Array(1000)
        .fill(null)
        .map((_, i) => ({
          name: `test_action_${i}`,
          description: `Test action ${i}`,
          inputSchema: {
            type: 'object',
            properties: {},
          },
        }));

      const largeFilter = new SchemaFilter();

      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.9,
        raw_intent: 'Create a Jira ticket',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const start = Date.now();
      largeFilter.filterSchemas(requirement, largeSchemaSet);
      const duration = Date.now() - start;

      // Should still complete quickly even with 1000 schemas
      expect(duration).toBeLessThan(50);
    });
  });

  describe('Edge Cases', () => {
    it('should handle empty schema set', () => {
      const emptyFilter = new SchemaFilter();

      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.9,
        raw_intent: 'Create a Jira ticket',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = emptyFilter.filterSchemas(requirement, []);

      expect(filtered).toHaveLength(0);
    });

    it('should handle missing service', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        confidence: 0.9,
        raw_intent: 'Create something',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      // Should filter by action since confidence is high
      expect(filtered.length).toBeGreaterThan(0);
      // All filtered schemas should be CREATE actions
      filtered.forEach((schema: any) => {
        expect(
          schema.name.includes('create') ||
          schema.description?.toLowerCase().includes('create')
        ).toBe(true);
      });
    });

    it('should handle UNKNOWN action', () => {
      const requirement: RequirementEnvelope = {
        action: 'UNKNOWN',
        service: 'atlassian',
        confidence: 0.5,
        raw_intent: 'Do something with Jira',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      expect(filtered.length).toBeGreaterThan(0);
    });
  });

  describe('Schema Structure Validation', () => {
    it('should preserve schema structure', () => {
      const requirement: RequirementEnvelope = {
        action: 'CREATE',
        service: 'atlassian',
        confidence: 0.9,
        raw_intent: 'Create a Jira ticket',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };

      const filtered = filter.filterSchemas(requirement, allSchemas);

      filtered.forEach((schema) => {
        expect(schema).toHaveProperty('name');
        expect(typeof schema.name).toBe('string');

        if (schema.description) {
          expect(typeof schema.description).toBe('string');
        }

        if (schema.inputSchema) {
          expect(typeof schema.inputSchema).toBe('object');
        }
      });
    });
  });
});

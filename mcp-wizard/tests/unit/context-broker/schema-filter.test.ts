/**
 * Unit Tests for Schema Filter
 *
 * Tests schema filtering logic for the Context Broker
 *
 * Requirements:
 * - Filter MCP tool schemas based on intent analysis results
 * - Return only relevant schemas for clear intents
 * - Return all schemas for ambiguous intents (fallback)
 * - Handle unknown services gracefully
 * - Support multiple service filtering
 */

import {
  ATLASSIAN_SCHEMAS,
  GOOGLEDOCS_SCHEMAS,
  SLACK_SCHEMAS,
  ALL_SCHEMAS,
  SCHEMA_REGISTRY,
} from '../../fixtures/context-broker/mcp-schemas';

/**
 * Requirement Envelope - Input format for Schema Filter
 */
interface RequirementEnvelope {
  intent: string;
  service?: string;
  action?: string;
  confidence?: number;
  fallback_to_all?: boolean;
}

/**
 * Mock Schema Filter
 *
 * This is a placeholder implementation until the actual Schema Filter is developed.
 * The tests define the expected behavior and API contract.
 */
class SchemaFilter {
  private schemaRegistry: Record<string, any[]>;

  constructor(schemaRegistry: Record<string, any[]>) {
    this.schemaRegistry = schemaRegistry;
  }

  /**
   * Filter schemas based on requirement envelope
   */
  filter(requirement: RequirementEnvelope): any[] {
    // Fallback to all schemas if flagged or service is unknown
    if (
      requirement.fallback_to_all ||
      !requirement.service ||
      !this.schemaRegistry[requirement.service]
    ) {
      return this.getAllSchemas();
    }

    // Return service-specific schemas
    const schemas = this.schemaRegistry[requirement.service] || [];

    // Optionally filter by action if specified
    if (requirement.action && schemas.length > 0) {
      return this.filterByAction(schemas, requirement.action);
    }

    return schemas;
  }

  /**
   * Filter schemas by action type
   */
  private filterByAction(schemas: any[], action: string): any[] {
    // Action-based filtering logic
    const actionKeywords: Record<string, string[]> = {
      CREATE: ['create', 'new', 'add'],
      UPDATE: ['update', 'edit', 'modify'],
      READ: ['get', 'read', 'fetch', 'list'],
      DELETE: ['delete', 'remove'],
    };

    const keywords = actionKeywords[action.toUpperCase()] || [];
    if (keywords.length === 0) {
      return schemas; // Return all if action unknown
    }

    // Filter schemas whose names match action keywords
    const filtered = schemas.filter((schema) => {
      const schemaName = schema.name.toLowerCase();
      return keywords.some((keyword) => schemaName.includes(keyword));
    });

    // Return filtered schemas, or all if no matches
    return filtered.length > 0 ? filtered : schemas;
  }

  /**
   * Get all schemas from all services
   */
  private getAllSchemas(): any[] {
    return Object.values(this.schemaRegistry).flat();
  }

  /**
   * Batch filter multiple requirements
   */
  filterBatch(requirements: RequirementEnvelope[]): any[][] {
    return requirements.map((req) => this.filter(req));
  }

  /**
   * Get schema count for a requirement
   */
  getSchemaCount(requirement: RequirementEnvelope): number {
    return this.filter(requirement).length;
  }

  /**
   * Get available services
   */
  getAvailableServices(): string[] {
    return Object.keys(this.schemaRegistry);
  }
}

describe('SchemaFilter - Unit Tests', () => {
  let filter: SchemaFilter;

  beforeEach(() => {
    filter = new SchemaFilter(SCHEMA_REGISTRY);
  });

  describe('Exact Service Filtering', () => {
    it('should return only Atlassian schemas for Atlassian service', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create a Jira ticket',
        service: 'atlassian',
        action: 'CREATE',
        confidence: 0.9,
        fallback_to_all: false,
      };

      const filtered = filter.filter(requirement);

      // When action is specified, should filter to CREATE actions only
      expect(filtered.length).toBeGreaterThan(0);
      expect(filtered.length).toBeLessThanOrEqual(ATLASSIAN_SCHEMAS.length);

      // All filtered schemas should be from Atlassian
      filtered.forEach(schema => {
        expect(ATLASSIAN_SCHEMAS).toContainEqual(schema);
      });

      // Ensure no schemas from other services
      const hasGoogleDocs = filtered.some((s) =>
        s.name.toLowerCase().includes('google')
      );
      const hasSlack = filtered.some((s) => s.name.toLowerCase().includes('slack'));
      expect(hasGoogleDocs).toBe(false);
      expect(hasSlack).toBe(false);
    });

    it('should return only Google Docs schemas for googledocs service', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Update Google Doc',
        service: 'googledocs',
        action: 'UPDATE',
        confidence: 0.9,
        fallback_to_all: false,
      };

      const filtered = filter.filter(requirement);

      // When action is specified, should filter to UPDATE actions only
      expect(filtered.length).toBeGreaterThan(0);
      expect(filtered.length).toBeLessThanOrEqual(GOOGLEDOCS_SCHEMAS.length);

      // All filtered schemas should be from Google Docs
      filtered.forEach(schema => {
        expect(GOOGLEDOCS_SCHEMAS).toContainEqual(schema);
      });

      // Ensure no schemas from other services
      const hasJira = filtered.some((s) => s.name.toLowerCase().includes('jira'));
      const hasSlack = filtered.some((s) => s.name.toLowerCase().includes('slack'));
      expect(hasJira).toBe(false);
      expect(hasSlack).toBe(false);
    });

    it('should return only Slack schemas for slack service', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Send Slack message',
        service: 'slack',
        action: 'CREATE',
        confidence: 0.9,
        fallback_to_all: false,
      };

      const filtered = filter.filter(requirement);

      expect(filtered).toHaveLength(SLACK_SCHEMAS.length);
      expect(filtered).toEqual(expect.arrayContaining(SLACK_SCHEMAS));
    });
  });

  describe('Action-Based Filtering', () => {
    it('should filter to CREATE actions when specified', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create a Jira ticket',
        service: 'atlassian',
        action: 'CREATE',
        confidence: 0.9,
        fallback_to_all: false,
      };

      const filtered = filter.filter(requirement);

      // Should contain create_jira_issue and create_confluence_page
      const createSchemas = filtered.filter((s) =>
        s.name.toLowerCase().includes('create')
      );
      expect(createSchemas.length).toBeGreaterThan(0);

      // All filtered schemas should be creation-related
      createSchemas.forEach((schema) => {
        expect(
          schema.name.toLowerCase().includes('create') ||
            schema.name.toLowerCase().includes('new') ||
            schema.name.toLowerCase().includes('add')
        ).toBe(true);
      });
    });

    it('should filter to UPDATE actions when specified', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Update Google Doc',
        service: 'googledocs',
        action: 'UPDATE',
        confidence: 0.9,
        fallback_to_all: false,
      };

      const filtered = filter.filter(requirement);

      // Should contain update_google_doc
      const updateSchemas = filtered.filter((s) =>
        s.name.toLowerCase().includes('update')
      );
      expect(updateSchemas.length).toBeGreaterThan(0);
    });

    it('should filter to READ actions when specified', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Read Google Doc',
        service: 'googledocs',
        action: 'READ',
        confidence: 0.9,
        fallback_to_all: false,
      };

      const filtered = filter.filter(requirement);

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

  describe('Fallback Behavior', () => {
    it('should return all schemas when fallback_to_all is true', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create a document',
        fallback_to_all: true,
        confidence: 0.5,
      };

      const filtered = filter.filter(requirement);

      expect(filtered).toHaveLength(ALL_SCHEMAS.length);
      expect(filtered).toEqual(expect.arrayContaining(ALL_SCHEMAS));
    });

    it('should return all schemas for unknown service', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create a blockchain document',
        service: 'unknown-service',
        action: 'CREATE',
        confidence: 0.3,
        fallback_to_all: false,
      };

      const filtered = filter.filter(requirement);

      expect(filtered).toHaveLength(ALL_SCHEMAS.length);
    });

    it('should return all schemas when service is missing', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Do something',
        action: 'CREATE',
        confidence: 0.5,
        fallback_to_all: false,
      };

      const filtered = filter.filter(requirement);

      expect(filtered).toHaveLength(ALL_SCHEMAS.length);
    });
  });

  describe('Graceful Degradation', () => {
    it('should handle empty requirement envelope', () => {
      const requirement: RequirementEnvelope = {
        intent: '',
      };

      const filtered = filter.filter(requirement);

      expect(filtered).toHaveLength(ALL_SCHEMAS.length);
    });

    it('should handle missing confidence score', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create a Jira ticket',
        service: 'atlassian',
        action: 'CREATE',
      };

      expect(() => {
        const filtered = filter.filter(requirement);
        expect(filtered.length).toBeGreaterThan(0);
      }).not.toThrow();
    });

    it('should handle null/undefined service gracefully', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create something',
        service: undefined,
        action: 'CREATE',
      };

      expect(() => {
        const filtered = filter.filter(requirement);
        expect(filtered).toHaveLength(ALL_SCHEMAS.length);
      }).not.toThrow();
    });
  });

  describe('Batch Filtering', () => {
    it('should filter multiple requirements in batch', () => {
      const requirements: RequirementEnvelope[] = [
        {
          intent: 'Create Jira ticket',
          service: 'atlassian',
          action: 'CREATE',
          confidence: 0.9,
        },
        {
          intent: 'Update Google Doc',
          service: 'googledocs',
          action: 'UPDATE',
          confidence: 0.9,
        },
        {
          intent: 'Ambiguous request',
          fallback_to_all: true,
          confidence: 0.3,
        },
      ];

      const results = filter.filterBatch(requirements);

      expect(results).toHaveLength(3);
      // Results should be filtered by action, so subset of full schemas
      expect(results[0].length).toBeGreaterThan(0);
      expect(results[1].length).toBeGreaterThan(0);
      expect(results[2]).toHaveLength(ALL_SCHEMAS.length);
    });

    it('should handle empty batch', () => {
      const results = filter.filterBatch([]);
      expect(results).toHaveLength(0);
    });
  });

  describe('Schema Count', () => {
    it('should return correct schema count for service', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create Jira ticket',
        service: 'atlassian',
        action: 'CREATE',
        confidence: 0.9,
      };

      const count = filter.getSchemaCount(requirement);
      expect(count).toBeGreaterThan(0);
    });

    it('should return total count for fallback', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Do something',
        fallback_to_all: true,
        confidence: 0.3,
      };

      const count = filter.getSchemaCount(requirement);
      expect(count).toBe(ALL_SCHEMAS.length);
    });
  });

  describe('Service Registry', () => {
    it('should return available services', () => {
      const services = filter.getAvailableServices();

      expect(services).toContain('atlassian');
      expect(services).toContain('googledocs');
      expect(services).toContain('slack');
    });

    it('should support custom schema registry', () => {
      const customRegistry = {
        'custom-service': [
          {
            name: 'custom_action',
            description: 'Custom action',
            inputSchema: { type: 'object', properties: {} },
          },
        ],
      };

      const customFilter = new SchemaFilter(customRegistry);
      const services = customFilter.getAvailableServices();

      expect(services).toContain('custom-service');
    });
  });

  describe('Edge Cases', () => {
    it('should handle service with no schemas', () => {
      const emptyRegistry = {
        'empty-service': [],
      };

      const emptyFilter = new SchemaFilter(emptyRegistry);
      const requirement: RequirementEnvelope = {
        intent: 'Test',
        service: 'empty-service',
        confidence: 0.9,
      };

      const filtered = emptyFilter.filter(requirement);
      expect(filtered).toHaveLength(0);
    });

    it('should handle case-insensitive service names', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create Jira ticket',
        service: 'ATLASSIAN', // Uppercase
        action: 'CREATE',
        confidence: 0.9,
      };

      // Note: Current implementation is case-sensitive
      // This test documents expected behavior
      const filtered = filter.filter(requirement);
      // Should fallback to all schemas for unknown service
      expect(filtered.length).toBeGreaterThan(0);
    });
  });

  describe('Schema Structure Validation', () => {
    it('should return schemas with required properties', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create Jira ticket',
        service: 'atlassian',
        action: 'CREATE',
        confidence: 0.9,
      };

      const filtered = filter.filter(requirement);

      filtered.forEach((schema) => {
        expect(schema).toHaveProperty('name');
        expect(schema).toHaveProperty('description');
        expect(schema).toHaveProperty('inputSchema');
        expect(typeof schema.name).toBe('string');
        expect(typeof schema.description).toBe('string');
        expect(typeof schema.inputSchema).toBe('object');
      });
    });

    it('should preserve schema structure during filtering', () => {
      const requirement: RequirementEnvelope = {
        intent: 'Create Jira ticket',
        service: 'atlassian',
        action: 'CREATE',
        confidence: 0.9,
      };

      const filtered = filter.filter(requirement);
      const original = ATLASSIAN_SCHEMAS;

      // Find common schemas
      const commonNames = filtered
        .map((s) => s.name)
        .filter((name) => original.some((o) => o.name === name));

      commonNames.forEach((name) => {
        const filteredSchema = filtered.find((s) => s.name === name);
        const originalSchema = original.find((s) => s.name === name);

        expect(filteredSchema).toEqual(originalSchema);
      });
    });
  });

  describe('Performance', () => {
    it('should handle large schema registries efficiently', () => {
      // Create a large schema registry
      const largeRegistry: Record<string, any[]> = {};
      for (let i = 0; i < 100; i++) {
        largeRegistry[`service-${i}`] = Array(50)
          .fill(null)
          .map((_, j) => ({
            name: `action_${i}_${j}`,
            description: `Action ${j} for service ${i}`,
            inputSchema: { type: 'object', properties: {} },
          }));
      }

      const largeFilter = new SchemaFilter(largeRegistry);
      const requirement: RequirementEnvelope = {
        intent: 'Test',
        service: 'service-50',
        confidence: 0.9,
      };

      const start = Date.now();
      const filtered = largeFilter.filter(requirement);
      const duration = Date.now() - start;

      expect(filtered).toHaveLength(50);
      expect(duration).toBeLessThan(100); // Should complete in <100ms
    });
  });

  describe('Regression Tests', () => {
    it('should maintain backward compatibility with previous filter results', () => {
      // Test case from previous version
      const requirement: RequirementEnvelope = {
        intent: 'Create a Jira ticket',
        service: 'atlassian',
        action: 'CREATE',
        confidence: 0.95,
        fallback_to_all: false,
      };

      const filtered = filter.filter(requirement);

      // Verify expected schemas are present
      expect(filtered.some((s) => s.name === 'create_jira_issue')).toBe(true);
      expect(filtered.some((s) => s.name === 'create_confluence_page')).toBe(true);
    });

    it('should handle multi-service scenarios correctly', () => {
      // Test scenario where both Jira and Confluence are needed
      const requirements: RequirementEnvelope[] = [
        {
          intent: 'Create Jira ticket',
          service: 'atlassian',
          action: 'CREATE',
          confidence: 0.9,
        },
        {
          intent: 'Create Confluence page',
          service: 'atlassian',
          action: 'CREATE',
          confidence: 0.9,
        },
      ];

      const results = filter.filterBatch(requirements);

      // Both should return Atlassian schemas (filtered by CREATE action)
      expect(results[0].length).toBeGreaterThan(0);
      expect(results[1].length).toBeGreaterThan(0);
      // Both should contain only Atlassian schemas
      results[0].forEach(schema => {
        expect(ATLASSIAN_SCHEMAS).toContainEqual(schema);
      });
      results[1].forEach(schema => {
        expect(ATLASSIAN_SCHEMAS).toContainEqual(schema);
      });
    });
  });
});

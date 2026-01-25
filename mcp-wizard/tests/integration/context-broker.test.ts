/**
 * Integration Tests for Context Broker
 *
 * Tests the complete flow: User Intent → Intent Analyzer → Schema Filter → Filtered Schemas
 *
 * This validates the integration between Intent Analyzer and Schema Filter components
 */

import {
  ATLASSIAN_SCHEMAS,
  GOOGLEDOCS_SCHEMAS,
  SLACK_SCHEMAS,
  ALL_SCHEMAS,
  SCHEMA_REGISTRY,
} from '../fixtures/context-broker/mcp-schemas';
import {
  CLEAR_INTENTS,
  AMBIGUOUS_INTENTS,
  MULTI_SERVICE_INTENTS,
} from '../fixtures/context-broker/test-intents';

/**
 * Intent Analysis Result
 */
interface IntentAnalysisResult {
  action?: string;
  service?: string;
  confidence: number;
  fallback_to_all?: boolean;
  details?: string;
}

/**
 * Requirement Envelope
 */
interface RequirementEnvelope {
  intent: string;
  service?: string;
  action?: string;
  confidence?: number;
  fallback_to_all?: boolean;
}

/**
 * Mock Intent Analyzer (same as unit test)
 */
class IntentAnalyzer {
  analyze(userIntent: string): IntentAnalysisResult {
    const normalizedIntent = userIntent.toLowerCase().trim();

    if (!normalizedIntent || normalizedIntent.length < 3) {
      return {
        confidence: 0.1,
        fallback_to_all: true,
        details: 'Intent too short or empty',
      };
    }

    const servicePatterns = {
      atlassian: [/\bjira\b/i, /\bconfluence\b/i, /\bticket\b/i, /\bissue\b/i, /proj-\d+/i],
      googledocs: [/google\s*doc/i, /\bgdoc\b/i, /\bdoc\s+id\b/i, /\bdocument\s+id\b/i],
      slack: [/\bslack\b/i, /\bchannel\b/i, /#\w+/i],
    };

    const actionPatterns = {
      UPDATE: [/\bupdate\b/i, /\bedit\b/i, /\bmodify\b/i, /\bchange\b/i],
      CREATE: [/\bcreate\b/i, /\bnew\b/i, /\badd\b/i, /\bsend\b/i],
      DELETE: [/\bdelete\b/i, /\bremove\b/i],
      READ: [/\bread\b/i, /\bget\b/i, /\bfetch\b/i, /\bshow\b/i, /\bview\b/i],
      LIST: [/\blist\b/i, /\ball\b/i],
    };

    let detectedService: string | undefined;
    let serviceConfidence = 0;

    for (const [service, patterns] of Object.entries(servicePatterns)) {
      for (const pattern of patterns) {
        if (pattern.test(normalizedIntent)) {
          detectedService = service;
          serviceConfidence = 0.9;
          break;
        }
      }
      if (detectedService) break;
    }

    let detectedAction: string | undefined;
    let actionConfidence = 0;

    for (const [action, patterns] of Object.entries(actionPatterns)) {
      for (const pattern of patterns) {
        if (pattern.test(normalizedIntent)) {
          detectedAction = action;
          actionConfidence = 0.9;
          break;
        }
      }
      if (detectedAction) break;
    }

    const hasService = !!detectedService;
    const hasAction = !!detectedAction;

    let confidence: number;
    if (hasService && hasAction) {
      confidence = Math.min(serviceConfidence, actionConfidence);
    } else if (hasService || hasAction) {
      confidence = 0.5;
    } else {
      confidence = 0.2;
    }

    const fallback_to_all = confidence < 0.7 || !hasService;

    return {
      action: detectedAction,
      service: detectedService,
      confidence,
      fallback_to_all,
      details: fallback_to_all
        ? 'Low confidence or ambiguous intent - returning all schemas'
        : 'Clear intent detected',
    };
  }
}

/**
 * Mock Schema Filter (same as unit test)
 */
class SchemaFilter {
  private schemaRegistry: Record<string, any[]>;

  constructor(schemaRegistry: Record<string, any[]>) {
    this.schemaRegistry = schemaRegistry;
  }

  filter(requirement: RequirementEnvelope): any[] {
    if (
      requirement.fallback_to_all ||
      !requirement.service ||
      !this.schemaRegistry[requirement.service]
    ) {
      return this.getAllSchemas();
    }

    const schemas = this.schemaRegistry[requirement.service] || [];

    if (requirement.action && schemas.length > 0) {
      return this.filterByAction(schemas, requirement.action);
    }

    return schemas;
  }

  private filterByAction(schemas: any[], action: string): any[] {
    const actionKeywords: Record<string, string[]> = {
      CREATE: ['create', 'new', 'add'],
      UPDATE: ['update', 'edit', 'modify'],
      READ: ['get', 'read', 'fetch', 'list'],
      DELETE: ['delete', 'remove'],
    };

    const keywords = actionKeywords[action.toUpperCase()] || [];
    if (keywords.length === 0) {
      return schemas;
    }

    const filtered = schemas.filter((schema) => {
      const schemaName = schema.name.toLowerCase();
      return keywords.some((keyword) => schemaName.includes(keyword));
    });

    return filtered.length > 0 ? filtered : schemas;
  }

  private getAllSchemas(): any[] {
    return Object.values(this.schemaRegistry).flat();
  }
}

/**
 * Context Broker - Integrates Intent Analyzer and Schema Filter
 */
class ContextBroker {
  private intentAnalyzer: IntentAnalyzer;
  private schemaFilter: SchemaFilter;

  constructor(schemaRegistry: Record<string, any[]>) {
    this.intentAnalyzer = new IntentAnalyzer();
    this.schemaFilter = new SchemaFilter(schemaRegistry);
  }

  /**
   * Process user intent and return filtered MCP tool schemas
   */
  processIntent(userIntent: string): {
    intent: string;
    analysis: IntentAnalysisResult;
    schemas: any[];
  } {
    // Step 1: Analyze intent
    const analysis = this.intentAnalyzer.analyze(userIntent);

    // Step 2: Create requirement envelope
    const requirement: RequirementEnvelope = {
      intent: userIntent,
      service: analysis.service,
      action: analysis.action,
      confidence: analysis.confidence,
      fallback_to_all: analysis.fallback_to_all,
    };

    // Step 3: Filter schemas
    const schemas = this.schemaFilter.filter(requirement);

    return {
      intent: userIntent,
      analysis,
      schemas,
    };
  }

  /**
   * Process multiple intents in batch
   */
  processBatch(intents: string[]): Array<{
    intent: string;
    analysis: IntentAnalysisResult;
    schemas: any[];
  }> {
    return intents.map((intent) => this.processIntent(intent));
  }
}

describe('Context Broker - Integration Tests', () => {
  let contextBroker: ContextBroker;

  beforeEach(() => {
    contextBroker = new ContextBroker(SCHEMA_REGISTRY);
  });

  describe('End-to-End Intent Processing', () => {
    it('should process "Create Jira ticket" and return Atlassian schemas', () => {
      const result = contextBroker.processIntent('Create a new Jira ticket');

      expect(result.analysis.service).toBe('atlassian');
      expect(result.analysis.action).toBe('CREATE');
      expect(result.analysis.confidence).toBeGreaterThanOrEqual(0.9);
      expect(result.schemas.length).toBeGreaterThan(0);

      // Should contain Jira creation schema
      expect(result.schemas.some((s) => s.name === 'create_jira_issue')).toBe(true);

      // Should not contain Google Docs schemas
      expect(result.schemas.some((s) => s.name === 'create_google_doc')).toBe(
        false
      );
    });

    it('should process "Update Google Doc" and return Google Docs schemas', () => {
      const result = contextBroker.processIntent('Update the Google Doc');

      expect(result.analysis.service).toBe('googledocs');
      expect(result.analysis.action).toBe('UPDATE');
      expect(result.schemas.length).toBeGreaterThan(0);

      // Should contain Google Docs update schema
      expect(result.schemas.some((s) => s.name === 'update_google_doc')).toBe(true);

      // Should not contain Jira schemas
      expect(result.schemas.some((s) => s.name === 'create_jira_issue')).toBe(
        false
      );
    });

    it('should process "Send Slack message" and return Slack schemas', () => {
      const result = contextBroker.processIntent('Send a Slack message to #eng');

      expect(result.analysis.service).toBe('slack');
      expect(result.analysis.action).toBe('CREATE');
      expect(result.schemas.length).toBeGreaterThan(0);

      // Should contain Slack message schema
      expect(result.schemas.some((s) => s.name === 'send_slack_message')).toBe(
        true
      );
    });
  });

  describe('Ambiguous Intent Handling', () => {
    it('should return all schemas for ambiguous "Create a document"', () => {
      const result = contextBroker.processIntent('Create a document');

      expect(result.analysis.fallback_to_all).toBe(true);
      expect(result.schemas).toHaveLength(ALL_SCHEMAS.length);

      // Should contain schemas from all services
      expect(result.schemas.some((s) => s.name === 'create_jira_issue')).toBe(true);
      expect(result.schemas.some((s) => s.name === 'create_google_doc')).toBe(true);
      expect(result.schemas.some((s) => s.name === 'send_slack_message')).toBe(
        true
      );
    });

    it('should return all schemas for ambiguous intents', () => {
      AMBIGUOUS_INTENTS.forEach((testCase) => {
        const result = contextBroker.processIntent(testCase.userInput);

        expect(result.analysis.fallback_to_all).toBe(true);
        expect(result.schemas.length).toBe(ALL_SCHEMAS.length);
      });
    });
  });

  describe('Clear Intent Processing', () => {
    it('should process all clear intents correctly', () => {
      CLEAR_INTENTS.forEach((testCase) => {
        const result = contextBroker.processIntent(testCase.userInput);

        expect(result.analysis.service).toBe(testCase.expectedService);
        expect(result.analysis.action).toBe(testCase.expectedAction);
        expect(result.analysis.confidence).toBeGreaterThanOrEqual(
          testCase.expectedConfidence || 0.9
        );

        // Should return service-specific schemas (not all)
        expect(result.schemas.length).toBeLessThan(ALL_SCHEMAS.length);
      });
    });
  });

  describe('Schema Filtering Integration', () => {
    it('should filter to CREATE actions for Atlassian', () => {
      const result = contextBroker.processIntent('Create a new Jira issue');

      // Should contain create schemas
      const createSchemas = result.schemas.filter((s) =>
        s.name.toLowerCase().includes('create')
      );
      expect(createSchemas.length).toBeGreaterThan(0);
    });

    it('should filter to UPDATE actions for Google Docs', () => {
      const result = contextBroker.processIntent('Update the Google Doc content');

      // Should contain update schemas
      const updateSchemas = result.schemas.filter((s) =>
        s.name.toLowerCase().includes('update')
      );
      expect(updateSchemas.length).toBeGreaterThan(0);
    });

    it('should filter to READ actions for Google Docs', () => {
      const result = contextBroker.processIntent('Read the Google Doc');

      // Should contain read-related schemas
      const readSchemas = result.schemas.filter(
        (s) =>
          s.name.toLowerCase().includes('read') ||
          s.name.toLowerCase().includes('get') ||
          s.name.toLowerCase().includes('list')
      );
      expect(readSchemas.length).toBeGreaterThan(0);
    });
  });

  describe('Batch Processing', () => {
    it('should process multiple intents in batch', () => {
      const intents = [
        'Create a Jira ticket',
        'Update Google Doc',
        'Send Slack message',
      ];

      const results = contextBroker.processBatch(intents);

      expect(results).toHaveLength(3);

      // Verify each result
      expect(results[0].analysis.service).toBe('atlassian');
      expect(results[1].analysis.service).toBe('googledocs');
      expect(results[2].analysis.service).toBe('slack');

      // Each should have filtered schemas
      results.forEach((result) => {
        expect(result.schemas.length).toBeGreaterThan(0);
      });
    });

    it('should handle mixed clear and ambiguous intents in batch', () => {
      const intents = [
        'Create a Jira ticket', // Clear
        'Update something', // Ambiguous
        'Send Slack message', // Clear
      ];

      const results = contextBroker.processBatch(intents);

      expect(results).toHaveLength(3);
      expect(results[0].analysis.fallback_to_all).toBe(false);
      expect(results[1].analysis.fallback_to_all).toBe(true);
      expect(results[2].analysis.fallback_to_all).toBe(false);
    });
  });

  describe('Multi-Service Scenarios', () => {
    it('should handle multi-service intent by detecting primary service', () => {
      const result = contextBroker.processIntent(
        'Create a Jira ticket and document in Confluence'
      );

      // Should detect Atlassian as primary service
      expect(result.analysis.service).toBe('atlassian');
      // Schemas should be filtered by CREATE action
      expect(result.schemas.length).toBeGreaterThan(0);
      result.schemas.forEach(schema => {
        expect(ATLASSIAN_SCHEMAS).toContainEqual(schema);
      });
    });

    it('should process multi-service intents from fixtures', () => {
      MULTI_SERVICE_INTENTS.forEach((testCase) => {
        const result = contextBroker.processIntent(testCase.userInput);

        // Should detect at least one service
        expect(result.analysis.service).toBeDefined();
        expect(result.schemas.length).toBeGreaterThan(0);
      });
    });
  });

  describe('Unknown Service Handling', () => {
    it('should fallback to all schemas for unknown service', () => {
      const result = contextBroker.processIntent(
        'Create a blockchain smart contract'
      );

      expect(result.analysis.fallback_to_all).toBe(true);
      expect(result.schemas).toHaveLength(ALL_SCHEMAS.length);
    });

    it('should gracefully handle service without clear action', () => {
      const result = contextBroker.processIntent('Jira');

      // Detects service but no action
      expect(result.analysis.service).toBe('atlassian');
      expect(result.analysis.action).toBeUndefined();
      expect(result.schemas.length).toBeGreaterThan(0);
    });
  });

  describe('Edge Cases', () => {
    it('should handle empty intent', () => {
      const result = contextBroker.processIntent('');

      expect(result.analysis.fallback_to_all).toBe(true);
      expect(result.schemas).toHaveLength(ALL_SCHEMAS.length);
    });

    it('should handle very long intent', () => {
      const longIntent = `Create a very detailed and comprehensive Jira issue
        with extensive documentation that covers all aspects of the project
        including technical specifications, acceptance criteria, and more`.repeat(
        10
      );

      const result = contextBroker.processIntent(longIntent);

      expect(result.analysis.service).toBe('atlassian');
      expect(result.analysis.action).toBe('CREATE');
    });

    it('should handle special characters in intent', () => {
      const result = contextBroker.processIntent(
        'Create Jira ticket: PROJ-123 @urgent #bug'
      );

      expect(result.analysis.service).toBe('atlassian');
      expect(result.analysis.action).toBe('CREATE');
    });
  });

  describe('Data Flow Validation', () => {
    it('should maintain data consistency through pipeline', () => {
      const userIntent = 'Create a new Jira ticket for bug fix';
      const result = contextBroker.processIntent(userIntent);

      // Verify intent is preserved
      expect(result.intent).toBe(userIntent);

      // Verify analysis contains expected fields
      expect(result.analysis).toHaveProperty('service');
      expect(result.analysis).toHaveProperty('action');
      expect(result.analysis).toHaveProperty('confidence');
      expect(result.analysis).toHaveProperty('fallback_to_all');

      // Verify schemas are valid
      expect(Array.isArray(result.schemas)).toBe(true);
      result.schemas.forEach((schema) => {
        expect(schema).toHaveProperty('name');
        expect(schema).toHaveProperty('description');
        expect(schema).toHaveProperty('inputSchema');
      });
    });

    it('should ensure analysis influences schema filtering', () => {
      const clearIntent = 'Create a Jira ticket';
      const ambiguousIntent = 'Create something';

      const clearResult = contextBroker.processIntent(clearIntent);
      const ambiguousResult = contextBroker.processIntent(ambiguousIntent);

      // Clear intent should have fewer schemas
      expect(clearResult.schemas.length).toBeLessThan(
        ambiguousResult.schemas.length
      );

      // Ambiguous should return all schemas
      expect(ambiguousResult.schemas.length).toBe(ALL_SCHEMAS.length);
    });
  });

  describe('Performance', () => {
    it('should process intent quickly', () => {
      const start = Date.now();
      const result = contextBroker.processIntent('Create a Jira ticket');
      const duration = Date.now() - start;

      expect(duration).toBeLessThan(50); // Should complete in <50ms
      expect(result.schemas.length).toBeGreaterThan(0);
    });

    it('should handle batch processing efficiently', () => {
      const intents = Array(100).fill('Create a Jira ticket');

      const start = Date.now();
      const results = contextBroker.processBatch(intents);
      const duration = Date.now() - start;

      expect(results).toHaveLength(100);
      expect(duration).toBeLessThan(1000); // Should complete in <1 second
    });
  });

  describe('Regression Tests', () => {
    it('should maintain consistent results for same intent', () => {
      const intent = 'Create a new Jira ticket';

      const result1 = contextBroker.processIntent(intent);
      const result2 = contextBroker.processIntent(intent);

      expect(result1.analysis.service).toBe(result2.analysis.service);
      expect(result1.analysis.action).toBe(result2.analysis.action);
      expect(result1.analysis.confidence).toBe(result2.analysis.confidence);
      expect(result1.schemas).toEqual(result2.schemas);
    });

    it('should handle case variations consistently', () => {
      const intents = [
        'Create a Jira ticket',
        'CREATE A JIRA TICKET',
        'create a jira ticket',
      ];

      const results = contextBroker.processBatch(intents);

      // All should detect same service and action
      results.forEach((result) => {
        expect(result.analysis.service).toBe('atlassian');
        expect(result.analysis.action).toBe('CREATE');
      });
    });

    it('should handle whitespace variations consistently', () => {
      const intents = [
        'Create a Jira ticket',
        '  Create a Jira ticket  ',
        'Create  a  Jira  ticket',
      ];

      const results = contextBroker.processBatch(intents);

      // All should produce similar results
      results.forEach((result) => {
        expect(result.analysis.service).toBe('atlassian');
        expect(result.analysis.action).toBe('CREATE');
      });
    });
  });
});

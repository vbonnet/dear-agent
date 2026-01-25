/**
 * Intent Analyzer Unit Tests
 *
 * Target: 90%+ coverage, validates 85-90% accuracy requirement
 * Performance: Each test should complete in <10ms
 */

import { describe, it, expect, beforeAll } from '@jest/globals';
import {
  analyzeIntent,
  analyzeIntentBatch,
  isConfident,
  RequirementEnvelope,
  PATTERNS,
} from '../../src/lib/intent-analyzer';

describe('Intent Analyzer', () => {
  describe('analyzeIntent - Basic CRUD Operations', () => {
    it('should detect CREATE action with Atlassian service', () => {
      const result = analyzeIntent('Create Jira ticket');

      expect(result.action).toBe('CREATE');
      expect(result.service).toBe('atlassian');
      expect(result.confidence).toBeGreaterThanOrEqual(0.8);
      expect(result.fallback_to_all).toBe(false);
      expect(result.raw_intent).toBe('Create Jira ticket');
      expect(result.parsed_at).toMatch(/^\d{4}-\d{2}-\d{2}T/);
    });

    it('should detect READ action with GoogleDocs service', () => {
      const result = analyzeIntent('Show me the document');

      expect(result.action).toBe('READ');
      expect(result.service).toBe('googledocs');
      expect(result.confidence).toBeGreaterThanOrEqual(0.8);
      expect(result.fallback_to_all).toBe(false);
    });

    it('should detect UPDATE action with Slack service', () => {
      const result = analyzeIntent('Edit my slack message');

      expect(result.action).toBe('UPDATE');
      expect(result.service).toBe('slack');
      expect(result.confidence).toBeGreaterThanOrEqual(0.8);
      expect(result.fallback_to_all).toBe(false);
    });

    it('should detect DELETE action with Confluence service', () => {
      const result = analyzeIntent('Delete the confluence page');

      expect(result.action).toBe('DELETE');
      expect(result.service).toBe('atlassian');
      expect(result.confidence).toBeGreaterThanOrEqual(0.8);
      expect(result.fallback_to_all).toBe(false);
    });

    it('should detect SEARCH action with Glean service', () => {
      const result = analyzeIntent('Search glean for the document');

      expect(result.action).toBe('SEARCH');
      expect(result.service).toBe('glean');
      expect(result.confidence).toBeGreaterThanOrEqual(0.8);
      expect(result.fallback_to_all).toBe(false);
    });
  });

  describe('Action Synonyms', () => {
    it('should recognize CREATE synonyms', () => {
      const verbs = ['create', 'add', 'new', 'make', 'generate'];
      verbs.forEach((verb) => {
        const result = analyzeIntent(`${verb} a jira ticket`);
        expect(result.action).toBe('CREATE');
      });
    });

    it('should recognize READ synonyms', () => {
      const verbs = ['read', 'get', 'show', 'view', 'display', 'fetch', 'list'];
      verbs.forEach((verb) => {
        const result = analyzeIntent(`${verb} the document`);
        expect(result.action).toBe('READ');
      });
    });

    it('should recognize UPDATE synonyms', () => {
      const verbs = ['update', 'edit', 'modify', 'change', 'set'];
      verbs.forEach((verb) => {
        const result = analyzeIntent(`${verb} the ticket`);
        expect(result.action).toBe('UPDATE');
      });
    });

    it('should recognize DELETE synonyms', () => {
      const verbs = ['delete', 'remove', 'destroy', 'cancel'];
      verbs.forEach((verb) => {
        const result = analyzeIntent(`${verb} the issue`);
        expect(result.action).toBe('DELETE');
      });
    });

    it('should recognize SEARCH synonyms', () => {
      const verbs = ['search', 'find', 'query', 'lookup', 'filter'];
      verbs.forEach((verb) => {
        const result = analyzeIntent(`${verb} in glean`);
        expect(result.action).toBe('SEARCH');
      });
    });
  });

  describe('Service Detection', () => {
    it('should detect Atlassian service keywords', () => {
      const keywords = ['jira', 'confluence', 'atlassian', 'ticket', 'issue'];
      keywords.forEach((keyword) => {
        const result = analyzeIntent(`Create a ${keyword}`);
        expect(result.service).toBe('atlassian');
      });
    });

    it('should detect GoogleDocs service keywords', () => {
      const keywords = ['google docs', 'googledocs', 'document', 'doc', 'gdocs'];
      keywords.forEach((keyword) => {
        const result = analyzeIntent(`Read the ${keyword}`);
        expect(result.service).toBe('googledocs');
      });
    });

    it('should detect Slack service keywords', () => {
      const keywords = ['slack', 'message', 'channel', 'dm'];
      keywords.forEach((keyword) => {
        const result = analyzeIntent(`Send a ${keyword}`);
        expect(result.service).toBe('slack');
      });
    });

    it('should detect Glean service keywords', () => {
      const keywords = ['glean', 'knowledge base', 'kb'];
      keywords.forEach((keyword) => {
        const result = analyzeIntent(`Find in ${keyword}`);
        expect(result.service).toBe('glean');
      });
    });
  });

  describe('Target Extraction', () => {
    it('should extract ticket target', () => {
      const result = analyzeIntent('Update ticket ABC-123');
      expect(result.target).toBe('ABC-123');
    });

    it('should extract document target', () => {
      const result = analyzeIntent('Read document Project Proposal');
      expect(result.target).toBe('Project Proposal');
    });

    it('should extract channel target', () => {
      const result = analyzeIntent('Post to channel #engineering');
      expect(result.target).toBe('#engineering');
    });

    it('should extract named entity target', () => {
      const result = analyzeIntent('Create a document named Q1 Report');
      expect(result.target).toBe('Q1 Report');
    });

    it('should handle missing target gracefully', () => {
      const result = analyzeIntent('Create a document');
      expect(result.target).toBeUndefined();
    });
  });

  describe('Scope Extraction', () => {
    it('should detect "all" scope', () => {
      const result = analyzeIntent('Show all jira tickets');
      expect(result.scope).toContain('all');
    });

    it('should detect "user" scope', () => {
      const result = analyzeIntent('List my documents');
      expect(result.scope).toContain('user');
    });

    it('should detect "recent" scope', () => {
      const result = analyzeIntent('Get recent messages');
      expect(result.scope).toContain('recent');
    });

    it('should detect "active" scope', () => {
      const result = analyzeIntent('Show open tickets');
      expect(result.scope).toContain('active');
    });

    it('should detect multiple scopes', () => {
      const result = analyzeIntent('List all my recent documents');
      expect(result.scope).toContain('all');
      expect(result.scope).toContain('user');
      expect(result.scope).toContain('recent');
      expect(result.scope?.length).toBe(3);
    });

    it('should handle no scope gracefully', () => {
      const result = analyzeIntent('Create a ticket');
      expect(result.scope).toBeUndefined();
    });
  });

  describe('Confidence Calculation', () => {
    it('should have high confidence for action + service', () => {
      const result = analyzeIntent('Create Jira ticket');
      expect(result.confidence).toBeGreaterThanOrEqual(0.8);
    });

    it('should have higher confidence with action + service + target', () => {
      const result = analyzeIntent('Update ticket ABC-123');
      expect(result.confidence).toBeGreaterThanOrEqual(0.8);
    });

    it('should have low confidence with only action', () => {
      const result = analyzeIntent('Create something');
      expect(result.confidence).toBeLessThan(0.5);
      expect(result.fallback_to_all).toBe(true);
    });

    it('should have low confidence with only service', () => {
      const result = analyzeIntent('Something with jira');
      expect(result.confidence).toBeLessThan(0.5);
      expect(result.fallback_to_all).toBe(true);
    });

    it('should have very low confidence with no matches', () => {
      const result = analyzeIntent('What is the weather?');
      expect(result.action).toBe('UNKNOWN');
      expect(result.confidence).toBeLessThan(0.5);
      expect(result.fallback_to_all).toBe(true);
    });

    it('should cap confidence at 0.9', () => {
      const result = analyzeIntent('Create Jira ticket named PROJECT-123');
      expect(result.confidence).toBeLessThanOrEqual(0.9);
    });
  });

  describe('Fallback Behavior', () => {
    it('should set fallback_to_all when confidence < 0.5', () => {
      const result = analyzeIntent('Do something');
      expect(result.fallback_to_all).toBe(true);
    });

    it('should not set fallback_to_all when confidence >= 0.5', () => {
      const result = analyzeIntent('Create a jira ticket');
      expect(result.fallback_to_all).toBe(false);
    });
  });

  describe('Edge Cases', () => {
    it('should handle empty string', () => {
      const result = analyzeIntent('');
      expect(result.action).toBe('UNKNOWN');
      expect(result.service).toBeUndefined();
      expect(result.confidence).toBe(0);
      expect(result.fallback_to_all).toBe(true);
    });

    it('should handle whitespace-only string', () => {
      const result = analyzeIntent('   ');
      expect(result.action).toBe('UNKNOWN');
      expect(result.confidence).toBe(0);
      expect(result.fallback_to_all).toBe(true);
    });

    it('should handle case-insensitive matching', () => {
      const upper = analyzeIntent('CREATE JIRA TICKET');
      const lower = analyzeIntent('create jira ticket');
      const mixed = analyzeIntent('CrEaTe JiRa TiCkEt');

      expect(upper.action).toBe('CREATE');
      expect(lower.action).toBe('CREATE');
      expect(mixed.action).toBe('CREATE');
    });

    it('should handle multi-word service names', () => {
      const result = analyzeIntent('Search in knowledge base');
      expect(result.service).toBe('glean');
    });

    it('should handle ambiguous multi-service intent', () => {
      const result = analyzeIntent('Create a jira ticket and update the document');
      // Should match first service found
      expect(result.service).toBeDefined();
      expect(['atlassian', 'googledocs']).toContain(result.service);
    });

    it('should preserve original raw_intent', () => {
      const input = '  Create   Jira  ticket  ';
      const result = analyzeIntent(input);
      expect(result.raw_intent).toBe(input);
    });
  });

  describe('Real-World Test Cases', () => {
    const testCases: Array<{
      input: string;
      expected: Partial<RequirementEnvelope>;
    }> = [
      {
        input: 'Create a new Jira ticket for the bug',
        expected: {
          action: 'CREATE',
          service: 'atlassian',
          confidence: 0.8,
        },
      },
      {
        input: 'Show me all open issues in Confluence',
        expected: {
          action: 'READ',
          service: 'atlassian',
          confidence: 0.8,
        },
      },
      {
        input: 'Update the Google Doc with the latest changes',
        expected: {
          action: 'UPDATE',
          service: 'googledocs',
          confidence: 0.8,
        },
      },
      {
        input: 'Delete my old Slack messages',
        expected: {
          action: 'DELETE',
          service: 'slack',
          confidence: 0.8,
        },
      },
      {
        input: 'Find documentation in Glean',
        expected: {
          action: 'SEARCH',
          service: 'glean',
          confidence: 0.8,
        },
      },
      {
        input: 'Send a message to #engineering',
        expected: {
          action: 'CREATE',
          service: 'slack',
          target: '#engineering',
        },
      },
      {
        input: 'List my recent documents',
        expected: {
          action: 'READ',
          service: 'googledocs',
        },
      },
      {
        input: 'Remove issue ABC-456',
        expected: {
          action: 'DELETE',
          service: 'atlassian',
          target: 'ABC-456',
        },
      },
    ];

    testCases.forEach(({ input, expected }) => {
      it(`should correctly parse: "${input}"`, () => {
        const result = analyzeIntent(input);

        if (expected.action) {
          expect(result.action).toBe(expected.action);
        }
        if (expected.service) {
          expect(result.service).toBe(expected.service);
        }
        if (expected.target) {
          expect(result.target).toBe(expected.target);
        }
        if (expected.confidence) {
          expect(result.confidence).toBeGreaterThanOrEqual(expected.confidence);
        }
      });
    });
  });

  describe('analyzeIntentBatch', () => {
    it('should process multiple intents', () => {
      const intents = [
        'Create Jira ticket',
        'Read document',
        'Update issue',
      ];

      const results = analyzeIntentBatch(intents);

      expect(results.length).toBe(3);
      expect(results[0].action).toBe('CREATE');
      expect(results[1].action).toBe('READ');
      expect(results[2].action).toBe('UPDATE');
    });

    it('should handle empty batch', () => {
      const results = analyzeIntentBatch([]);
      expect(results.length).toBe(0);
    });
  });

  describe('isConfident', () => {
    it('should return true for confident envelope (default threshold)', () => {
      const envelope = analyzeIntent('Create Jira ticket');
      expect(isConfident(envelope)).toBe(true);
    });

    it('should return false for non-confident envelope (default threshold)', () => {
      const envelope = analyzeIntent('Do something');
      expect(isConfident(envelope)).toBe(false);
    });

    it('should respect custom threshold', () => {
      const envelope = analyzeIntent('Create Jira ticket');
      expect(isConfident(envelope, 0.9)).toBe(false);
      expect(isConfident(envelope, 0.5)).toBe(true);
    });
  });

  describe('Performance', () => {
    it('should parse intent in <10ms', () => {
      const start = performance.now();
      analyzeIntent('Create a Jira ticket for the bug report');
      const end = performance.now();

      expect(end - start).toBeLessThan(10);
    });

    it('should handle batch processing efficiently', () => {
      const intents = Array(100).fill('Create Jira ticket');
      const start = performance.now();
      analyzeIntentBatch(intents);
      const end = performance.now();

      // Average should be <10ms per intent
      const avgTime = (end - start) / 100;
      expect(avgTime).toBeLessThan(10);
    });
  });

  describe('Pattern Export', () => {
    it('should export ACTIONS patterns', () => {
      expect(PATTERNS.ACTIONS).toBeDefined();
      expect(PATTERNS.ACTIONS.CREATE).toBeInstanceOf(RegExp);
      expect(PATTERNS.ACTIONS.READ).toBeInstanceOf(RegExp);
      expect(PATTERNS.ACTIONS.UPDATE).toBeInstanceOf(RegExp);
      expect(PATTERNS.ACTIONS.DELETE).toBeInstanceOf(RegExp);
      expect(PATTERNS.ACTIONS.SEARCH).toBeInstanceOf(RegExp);
    });

    it('should export SERVICES patterns', () => {
      expect(PATTERNS.SERVICES).toBeDefined();
      expect(PATTERNS.SERVICES.atlassian).toBeInstanceOf(RegExp);
      expect(PATTERNS.SERVICES.googledocs).toBeInstanceOf(RegExp);
      expect(PATTERNS.SERVICES.slack).toBeInstanceOf(RegExp);
      expect(PATTERNS.SERVICES.glean).toBeInstanceOf(RegExp);
    });
  });

  describe('Accuracy Validation', () => {
    /**
     * Target accuracy: 85-90% on representative test set
     * This test validates the requirement from ARCHITECTURE.md
     */
    it('should achieve 85%+ accuracy on representative test set', () => {
      const testSet: Array<{
        input: string;
        expectedAction: string;
        expectedService?: string;
      }> = [
        { input: 'Create Jira ticket', expectedAction: 'CREATE', expectedService: 'atlassian' },
        { input: 'Show document', expectedAction: 'READ', expectedService: 'googledocs' },
        { input: 'Update issue', expectedAction: 'UPDATE', expectedService: 'atlassian' },
        { input: 'Delete message', expectedAction: 'DELETE', expectedService: 'slack' },
        { input: 'Search glean', expectedAction: 'SEARCH', expectedService: 'glean' },
        { input: 'Add new ticket', expectedAction: 'CREATE', expectedService: 'atlassian' },
        { input: 'View doc', expectedAction: 'READ', expectedService: 'googledocs' },
        { input: 'Modify confluence', expectedAction: 'UPDATE', expectedService: 'atlassian' },
        { input: 'Remove channel', expectedAction: 'DELETE', expectedService: 'slack' },
        { input: 'Find in KB', expectedAction: 'SEARCH', expectedService: 'glean' },
        { input: 'Generate issue', expectedAction: 'CREATE', expectedService: 'atlassian' },
        { input: 'Display gdocs', expectedAction: 'READ', expectedService: 'googledocs' },
        { input: 'Change ticket', expectedAction: 'UPDATE', expectedService: 'atlassian' },
        { input: 'Cancel dm', expectedAction: 'DELETE', expectedService: 'slack' },
        { input: 'Query knowledge base', expectedAction: 'SEARCH', expectedService: 'glean' },
        { input: 'Make new doc', expectedAction: 'CREATE', expectedService: 'googledocs' },
        { input: 'Fetch jira', expectedAction: 'READ', expectedService: 'atlassian' },
        { input: 'Set slack', expectedAction: 'UPDATE', expectedService: 'slack' },
        { input: 'Destroy issue', expectedAction: 'DELETE', expectedService: 'atlassian' },
        { input: 'Lookup glean', expectedAction: 'SEARCH', expectedService: 'glean' },
      ];

      let correct = 0;
      const results = testSet.map((test) => {
        const result = analyzeIntent(test.input);
        const actionMatch = result.action === test.expectedAction;
        const serviceMatch = !test.expectedService || result.service === test.expectedService;
        const isCorrect = actionMatch && serviceMatch;

        if (isCorrect) correct++;

        return { ...test, result, isCorrect };
      });

      const accuracy = correct / testSet.length;

      // Log failures for debugging
      const failures = results.filter((r) => !r.isCorrect);
      if (failures.length > 0) {
        console.log('\nFailed test cases:');
        failures.forEach((f) => {
          console.log(`  "${f.input}"`);
          console.log(`    Expected: ${f.expectedAction} / ${f.expectedService}`);
          console.log(`    Got: ${f.result.action} / ${f.result.service}`);
        });
      }

      expect(accuracy).toBeGreaterThanOrEqual(0.85);
    });
  });
});

/**
 * Unit Tests for Intent Analyzer
 *
 * Tests intent parsing and analysis functionality for the Context Broker
 *
 * Requirements:
 * - Parse user intents into structured format
 * - Identify target service (atlassian, googledocs, slack, etc.)
 * - Determine action type (CREATE, UPDATE, READ, DELETE, LIST)
 * - Calculate confidence score
 * - Handle ambiguous intents with fallback behavior
 */

import {
  CLEAR_INTENTS,
  AMBIGUOUS_INTENTS,
  EDGE_CASE_INTENTS,
  MULTI_SERVICE_INTENTS,
} from '../../fixtures/context-broker/test-intents';

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
 * Mock Intent Analyzer
 *
 * This is a placeholder implementation until the actual Intent Analyzer is developed.
 * The tests define the expected behavior and API contract.
 */
class IntentAnalyzer {
  /**
   * Analyze user intent and extract structured information
   */
  analyze(userIntent: string): IntentAnalysisResult {
    // Normalize input
    const normalizedIntent = userIntent.toLowerCase().trim();

    // Handle empty or very short inputs
    if (!normalizedIntent || normalizedIntent.length < 3) {
      return {
        confidence: 0.1,
        fallback_to_all: true,
        details: 'Intent too short or empty',
      };
    }

    // Service detection patterns
    const servicePatterns = {
      atlassian: [
        /\bjira\b/i,
        /\bconfluence\b/i,
        /\bticket\b/i,
        /\bissue\b/i,
        /proj-\d+/i, // Jira issue key pattern
      ],
      googledocs: [
        /google\s*doc/i,
        /\bgdoc\b/i,
        /\bdoc\s+id\b/i,
        /\bdocument\s+id\b/i,
      ],
      slack: [
        /\bslack\b/i,
        /\bchannel\b/i,
        /#\w+/i, // Channel pattern
      ],
    };

    // Action detection patterns (order matters - checked in priority order)
    const actionPatterns = {
      UPDATE: [/\bupdate\b/i, /\bedit\b/i, /\bmodify\b/i, /\bchange\b/i],
      CREATE: [/\bcreate\b/i, /\bnew\b/i, /\badd\b/i, /\bsend\b/i],
      DELETE: [/\bdelete\b/i, /\bremove\b/i],
      READ: [/\bread\b/i, /\bget\b/i, /\bfetch\b/i, /\bshow\b/i, /\bview\b/i],
      LIST: [/\blist\b/i, /\ball\b/i],
    };

    // Detect service
    let detectedService: string | undefined;
    let serviceConfidence = 0;

    for (const [service, patterns] of Object.entries(servicePatterns)) {
      for (const pattern of patterns) {
        if (pattern.test(normalizedIntent)) {
          detectedService = service;
          serviceConfidence = 0.9; // High confidence for pattern match
          break;
        }
      }
      if (detectedService) break;
    }

    // Detect action
    let detectedAction: string | undefined;
    let actionConfidence = 0;

    for (const [action, patterns] of Object.entries(actionPatterns)) {
      for (const pattern of patterns) {
        if (pattern.test(normalizedIntent)) {
          detectedAction = action;
          actionConfidence = 0.9; // High confidence for pattern match
          break;
        }
      }
      if (detectedAction) break;
    }

    // Calculate overall confidence
    const hasService = !!detectedService;
    const hasAction = !!detectedAction;

    let confidence: number;
    if (hasService && hasAction) {
      confidence = Math.min(serviceConfidence, actionConfidence);
    } else if (hasService || hasAction) {
      confidence = 0.5; // Medium confidence - partial match
    } else {
      confidence = 0.2; // Low confidence - no clear matches
    }

    // Determine if fallback is needed
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

  /**
   * Batch analyze multiple intents
   */
  analyzeBatch(intents: string[]): IntentAnalysisResult[] {
    return intents.map((intent) => this.analyze(intent));
  }
}

describe('IntentAnalyzer - Unit Tests', () => {
  let analyzer: IntentAnalyzer;

  beforeEach(() => {
    analyzer = new IntentAnalyzer();
  });

  describe('Clear Intent Parsing', () => {
    it('should parse "Create Jira ticket" with high confidence', () => {
      const result = analyzer.analyze('Create a new Jira ticket for bug tracking');

      expect(result.action).toBe('CREATE');
      expect(result.service).toBe('atlassian');
      expect(result.confidence).toBeGreaterThanOrEqual(0.9);
      expect(result.fallback_to_all).toBe(false);
    });

    it('should parse "Update Google Doc" with high confidence', () => {
      const result = analyzer.analyze('Update the Google Doc with ID abc123');

      expect(result.action).toBe('UPDATE');
      expect(result.service).toBe('googledocs');
      expect(result.confidence).toBeGreaterThanOrEqual(0.9);
      expect(result.fallback_to_all).toBe(false);
    });

    it('should parse "Read Google Doc" with high confidence', () => {
      const result = analyzer.analyze('Read the contents of Google Doc xyz789');

      expect(result.action).toBe('READ');
      expect(result.service).toBe('googledocs');
      expect(result.confidence).toBeGreaterThanOrEqual(0.9);
      expect(result.fallback_to_all).toBe(false);
    });

    it('should parse "Send Slack message" with high confidence', () => {
      const result = analyzer.analyze('Send a Slack message to #engineering channel');

      expect(result.action).toBe('CREATE');
      expect(result.service).toBe('slack');
      expect(result.confidence).toBeGreaterThanOrEqual(0.9);
      expect(result.fallback_to_all).toBe(false);
    });

    it('should parse Confluence page creation', () => {
      const result = analyzer.analyze('Create a Confluence page for API documentation');

      expect(result.action).toBe('CREATE');
      expect(result.service).toBe('atlassian');
      expect(result.confidence).toBeGreaterThanOrEqual(0.9);
      expect(result.fallback_to_all).toBe(false);
    });

    it('should parse all clear intents correctly', () => {
      CLEAR_INTENTS.forEach((testCase) => {
        const result = analyzer.analyze(testCase.userInput);

        expect(result.action).toBe(testCase.expectedAction);
        expect(result.service).toBe(testCase.expectedService);
        expect(result.confidence).toBeGreaterThanOrEqual(
          testCase.expectedConfidence || 0.9
        );
        expect(result.fallback_to_all).toBe(false);
      });
    });
  });

  describe('Ambiguous Intent Handling', () => {
    it('should trigger fallback for "Create a document"', () => {
      const result = analyzer.analyze('Create a document');

      expect(result.fallback_to_all).toBe(true);
      expect(result.confidence).toBeLessThan(0.7);
    });

    it('should trigger fallback for "Update the project status"', () => {
      const result = analyzer.analyze('Update the project status');

      expect(result.fallback_to_all).toBe(true);
      expect(result.confidence).toBeLessThan(0.7);
    });

    it('should trigger fallback for all ambiguous intents', () => {
      AMBIGUOUS_INTENTS.forEach((testCase) => {
        const result = analyzer.analyze(testCase.userInput);

        expect(result.fallback_to_all).toBe(true);
        expect(result.confidence).toBeLessThan(0.7);
      });
    });

    it('should provide helpful details for ambiguous intents', () => {
      const result = analyzer.analyze('Help me with documentation');

      expect(result.fallback_to_all).toBe(true);
      expect(result.details).toContain('ambiguous');
    });
  });

  describe('Edge Cases', () => {
    it('should handle empty intent gracefully', () => {
      const result = analyzer.analyze('');

      expect(result.fallback_to_all).toBe(true);
      expect(result.confidence).toBeLessThan(0.5);
      expect(result.details).toContain('empty');
    });

    it('should handle very short intent', () => {
      const result = analyzer.analyze('jira');

      expect(result.service).toBe('atlassian');
      expect(result.fallback_to_all).toBe(true); // No action detected
    });

    it('should handle generic help request', () => {
      const result = analyzer.analyze('What can you do?');

      expect(result.fallback_to_all).toBe(true);
      expect(result.confidence).toBeLessThan(0.5);
    });

    it('should handle unknown service references', () => {
      const result = analyzer.analyze('Create something with blockchain and AI');

      expect(result.fallback_to_all).toBe(true);
      expect(result.service).toBeUndefined();
    });

    it('should handle all edge cases without errors', () => {
      EDGE_CASE_INTENTS.forEach((testCase) => {
        expect(() => {
          const result = analyzer.analyze(testCase.userInput);
          expect(result).toBeDefined();
          expect(result.confidence).toBeGreaterThanOrEqual(0);
          expect(result.confidence).toBeLessThanOrEqual(1);
        }).not.toThrow();
      });
    });
  });

  describe('Multi-Service Intent Detection', () => {
    it('should detect Atlassian in multi-service intent', () => {
      const result = analyzer.analyze(
        'Create a Jira ticket and document it in Confluence'
      );

      expect(result.service).toBe('atlassian'); // Primary service detected
      expect(result.action).toBe('CREATE');
      // Note: Multi-service coordination is handled at higher level
    });

    it('should handle cross-service intents', () => {
      const result = analyzer.analyze(
        'Create a Google Doc and link it in the Jira ticket'
      );

      // Should detect at least one service
      expect(result.service).toBeDefined();
      expect(['atlassian', 'googledocs']).toContain(result.service);
    });
  });

  describe('Batch Analysis', () => {
    it('should analyze multiple intents in batch', () => {
      const intents = [
        'Create a Jira ticket',
        'Update Google Doc',
        'Send Slack message',
      ];

      const results = analyzer.analyzeBatch(intents);

      expect(results).toHaveLength(3);
      expect(results[0].service).toBe('atlassian');
      expect(results[1].service).toBe('googledocs');
      expect(results[2].service).toBe('slack');
    });

    it('should handle empty batch', () => {
      const results = analyzer.analyzeBatch([]);
      expect(results).toHaveLength(0);
    });
  });

  describe('Confidence Scoring', () => {
    it('should assign high confidence (>0.9) to clear intents', () => {
      const result = analyzer.analyze('Create a new Jira issue PROJ-123');
      expect(result.confidence).toBeGreaterThanOrEqual(0.9);
    });

    it('should assign medium confidence (0.5-0.7) to partial matches', () => {
      const result = analyzer.analyze('Create something'); // Has action, no service
      expect(result.confidence).toBeGreaterThanOrEqual(0.5);
      expect(result.confidence).toBeLessThan(0.7);
    });

    it('should assign low confidence (<0.5) to unclear intents', () => {
      const result = analyzer.analyze('Do stuff');
      expect(result.confidence).toBeLessThan(0.5);
    });

    it('should ensure confidence is always between 0 and 1', () => {
      const testInputs = [
        'Create a Jira ticket',
        'Update document',
        '',
        'Random text here',
      ];

      testInputs.forEach((input) => {
        const result = analyzer.analyze(input);
        expect(result.confidence).toBeGreaterThanOrEqual(0);
        expect(result.confidence).toBeLessThanOrEqual(1);
      });
    });
  });

  describe('Service Detection Patterns', () => {
    it('should detect Jira from issue key pattern (PROJ-123)', () => {
      const result = analyzer.analyze('Update issue PROJ-123 with new status');
      expect(result.service).toBe('atlassian');
    });

    it('should detect Slack from channel pattern (#engineering)', () => {
      const result = analyzer.analyze('Send message to #engineering');
      expect(result.service).toBe('slack');
    });

    it('should detect Google Docs from "Google Doc" keyword', () => {
      const result = analyzer.analyze('Create a new Google Doc for the proposal');
      expect(result.service).toBe('googledocs');
    });
  });

  describe('Action Detection', () => {
    it('should detect CREATE action from multiple keywords', () => {
      const keywords = ['create', 'new', 'add'];

      keywords.forEach((keyword) => {
        const result = analyzer.analyze(`${keyword} a Jira ticket`);
        expect(result.action).toBe('CREATE');
      });
    });

    it('should detect UPDATE action from multiple keywords', () => {
      const keywords = ['update', 'edit', 'modify', 'change'];

      keywords.forEach((keyword) => {
        const result = analyzer.analyze(`${keyword} the Jira issue`);
        expect(result.action).toBe('UPDATE');
      });
    });

    it('should detect READ action from multiple keywords', () => {
      const keywords = ['read', 'get', 'fetch', 'show', 'view'];

      keywords.forEach((keyword) => {
        const result = analyzer.analyze(`${keyword} the Google Doc`);
        expect(result.action).toBe('READ');
      });
    });

    it('should detect DELETE action', () => {
      const result = analyzer.analyze('Delete the Jira issue');
      expect(result.action).toBe('DELETE');
    });

    it('should detect LIST action', () => {
      const result = analyzer.analyze('List all Jira projects');
      expect(result.action).toBe('LIST');
    });
  });

  describe('API Contract', () => {
    it('should return required fields in result', () => {
      const result = analyzer.analyze('Create a Jira ticket');

      expect(result).toHaveProperty('confidence');
      expect(typeof result.confidence).toBe('number');
    });

    it('should include fallback_to_all flag', () => {
      const result = analyzer.analyze('Create a Jira ticket');

      expect(result).toHaveProperty('fallback_to_all');
      expect(typeof result.fallback_to_all).toBe('boolean');
    });

    it('should include optional service and action fields', () => {
      const result = analyzer.analyze('Create a Jira ticket');

      if (result.service) {
        expect(typeof result.service).toBe('string');
      }
      if (result.action) {
        expect(typeof result.action).toBe('string');
      }
    });
  });
});

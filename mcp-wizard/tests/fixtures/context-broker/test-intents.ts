/**
 * Test Intent Fixtures for Context Broker Tests
 *
 * Represents typical user intents that should be analyzed
 */

export interface TestIntent {
  userInput: string;
  expectedService?: string;
  expectedAction?: string;
  expectedConfidence?: number;
  description: string;
}

/**
 * Clear, unambiguous intents with high confidence
 */
export const CLEAR_INTENTS: TestIntent[] = [
  {
    userInput: 'Create a new Jira ticket for bug tracking',
    expectedService: 'atlassian',
    expectedAction: 'CREATE',
    expectedConfidence: 0.9,
    description: 'Clear Jira creation intent',
  },
  {
    userInput: 'Update the existing Jira issue PROJ-123 with new status',
    expectedService: 'atlassian',
    expectedAction: 'UPDATE',
    expectedConfidence: 0.9,
    description: 'Clear Jira update intent',
  },
  {
    userInput: 'Create a new Google Doc for the project proposal',
    expectedService: 'googledocs',
    expectedAction: 'CREATE',
    expectedConfidence: 0.9,
    description: 'Clear Google Docs creation intent',
  },
  {
    userInput: 'Update the Google Doc with ID abc123',
    expectedService: 'googledocs',
    expectedAction: 'UPDATE',
    expectedConfidence: 0.9,
    description: 'Clear Google Docs update intent',
  },
  {
    userInput: 'Read the contents of Google Doc xyz789',
    expectedService: 'googledocs',
    expectedAction: 'READ',
    expectedConfidence: 0.9,
    description: 'Clear Google Docs read intent',
  },
  {
    userInput: 'Send a Slack message to #engineering channel',
    expectedService: 'slack',
    expectedAction: 'CREATE',
    expectedConfidence: 0.9,
    description: 'Clear Slack message intent',
  },
  {
    userInput: 'Create a Confluence page for API documentation',
    expectedService: 'atlassian',
    expectedAction: 'CREATE',
    expectedConfidence: 0.9,
    description: 'Clear Confluence creation intent',
  },
];

/**
 * Ambiguous intents that should trigger fallback to all schemas
 */
export const AMBIGUOUS_INTENTS: TestIntent[] = [
  {
    userInput: 'Create a document',
    description: 'Ambiguous - could be Google Doc or Confluence page',
  },
  {
    userInput: 'Update the project status',
    description: 'Ambiguous - could be Jira, Confluence, or Google Doc',
  },
  {
    userInput: 'Share the latest report',
    description: 'Ambiguous - could be Google Doc or Confluence',
  },
  {
    userInput: 'Help me with documentation',
    description: 'Very ambiguous - multiple services possible',
  },
];

/**
 * Multi-service intents (require multiple MCPs)
 */
export const MULTI_SERVICE_INTENTS: TestIntent[] = [
  {
    userInput: 'Create a Jira ticket and document it in Confluence',
    description: 'Multi-service - Atlassian only (Jira + Confluence)',
  },
  {
    userInput: 'Update the Jira issue and notify the team on Slack',
    description: 'Multi-service - Atlassian + Slack',
  },
  {
    userInput: 'Create a Google Doc and link it in the Jira ticket',
    description: 'Multi-service - Google Docs + Atlassian',
  },
];

/**
 * Edge case intents
 */
export const EDGE_CASE_INTENTS: TestIntent[] = [
  {
    userInput: '',
    description: 'Empty intent',
  },
  {
    userInput: 'jira',
    description: 'Single word - service name only',
  },
  {
    userInput: 'What can you do?',
    description: 'Generic help request',
  },
  {
    userInput: 'Create something with blockchain and AI',
    description: 'Unknown service references',
  },
  {
    userInput: 'Delete all Jira issues',
    description: 'Potentially dangerous operation',
  },
];

/**
 * Requirement Envelope format intents
 */
export const REQUIREMENT_ENVELOPES = [
  {
    requirement: {
      intent: 'Create a Jira ticket',
      service: 'atlassian',
      action: 'CREATE',
      confidence: 0.95,
    },
    description: 'Well-formed requirement envelope',
  },
  {
    requirement: {
      intent: 'Update Google Doc',
      service: 'googledocs',
      action: 'UPDATE',
      confidence: 0.92,
    },
    description: 'Well-formed requirement envelope for Google Docs',
  },
  {
    requirement: {
      intent: 'List available resources',
      // Missing service - should trigger fallback
      action: 'LIST',
      confidence: 0.5,
    },
    description: 'Incomplete requirement envelope (missing service)',
  },
];

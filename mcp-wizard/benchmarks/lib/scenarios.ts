/**
 * Test Scenarios
 *
 * Defines synthetic test cases for benchmarking MCP performance.
 *
 * @module benchmarks/lib/scenarios
 */

export interface TestScenario {
  name: string;
  intent: string;
  expectedTools: string[];
  description: string;
}

/**
 * Test scenarios for context usage and latency benchmarks
 *
 * - googledocs: Single MCP server intent
 * - atlassian: Single MCP server intent
 * - slack: Single MCP server intent
 * - ambiguous: Fallback to all tools (eager loading equivalent)
 */
export const scenarios: TestScenario[] = [
  {
    name: 'googledocs',
    intent: 'Read my Google Doc named Project Plan',
    expectedTools: ['googledocs'],
    description: 'Single MCP server (googledocs tools only)',
  },
  {
    name: 'atlassian',
    intent: 'Search for recent Jira tickets',
    expectedTools: ['atlassian'],
    description: 'Single MCP server (atlassian tools only)',
  },
  {
    name: 'slack',
    intent: 'Find slack messages about the project',
    expectedTools: ['slack'],
    description: 'Single MCP server (slack tools only)',
  },
  {
    name: 'ambiguous',
    intent: 'Help me find information',
    expectedTools: ['googledocs', 'atlassian', 'slack'], // All tools
    description: 'Ambiguous intent (fallback to all tools)',
  },
];

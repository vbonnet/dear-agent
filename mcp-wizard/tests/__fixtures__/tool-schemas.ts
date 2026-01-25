/**
 * Test Fixtures: Tool Schemas
 *
 * Provides sample MCPToolSchema objects for testing.
 */

import { MCPToolSchema } from '../../src/lib/schema-filter';

/**
 * Sample Google Docs MCP schemas
 */
export const sampleGoogleDocsSchemas: MCPToolSchema[] = [
  {
    name: 'mcp__GoogleDocs__readGoogleDoc',
    description: 'Reads a Google Document',
    inputSchema: {
      type: 'object',
      properties: {
        documentId: {
          type: 'string',
          description: 'The ID of the Google Document'
        }
      },
      required: ['documentId']
    }
  },
  {
    name: 'mcp__GoogleDocs__listDocumentTabs',
    description: 'Lists tabs in a Google Document',
    inputSchema: {
      type: 'object',
      properties: {
        documentId: {
          type: 'string',
          description: 'The ID of the Google Document'
        }
      },
      required: ['documentId']
    }
  },
  {
    name: 'mcp__GoogleDocs__appendToGoogleDoc',
    description: 'Appends text to a Google Document',
    inputSchema: {
      type: 'object',
      properties: {
        documentId: {
          type: 'string'
        },
        textToAppend: {
          type: 'string'
        }
      },
      required: ['documentId', 'textToAppend']
    }
  }
];

/**
 * Sample Atlassian MCP schemas
 */
export const sampleAtlassianSchemas: MCPToolSchema[] = [
  {
    name: 'mcp__Atlassian__getIssue',
    description: 'Get a Jira issue by key',
    inputSchema: {
      type: 'object',
      properties: {
        issueKey: {
          type: 'string',
          description: 'The Jira issue key (e.g., PROJ-123)'
        }
      },
      required: ['issueKey']
    }
  },
  {
    name: 'mcp__Atlassian__searchJira',
    description: 'Search Jira issues with JQL',
    inputSchema: {
      type: 'object',
      properties: {
        jql: {
          type: 'string',
          description: 'JQL query string'
        }
      },
      required: ['jql']
    }
  }
];

/**
 * All schemas combined
 */
export const allSchemas = [
  ...sampleGoogleDocsSchemas,
  ...sampleAtlassianSchemas
];

/**
 * Empty schemas (for testing empty MCP case)
 */
export const emptySchemas: MCPToolSchema[] = [];

/**
 * Single schema (for simple test cases)
 */
export const singleSchema: MCPToolSchema[] = [
  sampleGoogleDocsSchemas[0]
];

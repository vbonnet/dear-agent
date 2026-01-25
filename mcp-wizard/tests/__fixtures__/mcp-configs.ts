/**
 * Test Fixtures: MCP Configurations
 *
 * Provides sample DownstreamMCP configurations for testing.
 */

import { DownstreamMCP } from '../../src/lib/mcp-proxy';
import { sampleGoogleDocsSchemas, sampleAtlassianSchemas } from './tool-schemas';

/**
 * Mock Google Docs MCP configuration
 */
export const mockGoogleDocsMCP: DownstreamMCP = {
  name: 'googledocs',
  command: '/mock/path/googledocs-mcp',
  args: ['--stdio'],
  env: {
    GOOGLE_DOCS_TOKEN: 'mock-token-googledocs'
  },
  schemas: sampleGoogleDocsSchemas
};

/**
 * Mock Atlassian MCP configuration
 */
export const mockAtlassianMCP: DownstreamMCP = {
  name: 'atlassian',
  command: '/mock/path/atlassian-mcp',
  args: ['--stdio'],
  env: {
    ATLASSIAN_TOKEN: 'mock-token-atlassian'
  },
  schemas: sampleAtlassianSchemas
};

/**
 * Mock MCP with no schemas (for testing empty case)
 */
export const mockEmptyMCP: DownstreamMCP = {
  name: 'empty-mcp',
  command: '/mock/path/empty-mcp',
  args: ['--stdio'],
  schemas: []
};

/**
 * Map of all mock MCPs
 */
export const mockMCPConfigs = new Map<string, DownstreamMCP>([
  ['googledocs', mockGoogleDocsMCP],
  ['atlassian', mockAtlassianMCP]
]);

/**
 * Map with single MCP (for simple tests)
 */
export const singleMCPConfig = new Map<string, DownstreamMCP>([
  ['googledocs', mockGoogleDocsMCP]
]);

/**
 * Map with empty MCP (for testing edge cases)
 */
export const emptyMCPConfig = new Map<string, DownstreamMCP>([
  ['empty', mockEmptyMCP]
]);

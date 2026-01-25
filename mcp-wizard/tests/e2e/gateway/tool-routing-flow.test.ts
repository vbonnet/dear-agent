/**
 * E2E Tests: Tool Routing Flow
 *
 * Tests complete tool routing flow through gateway.
 */

import { MCPProxy } from '../../../src/lib/mcp-proxy';
import { SchemaFilter } from '../../../src/lib/schema-filter';
import { mockMCPConfigs } from '../../__fixtures__/mcp-configs';
import { createToolsListRequest, createToolsCallRequest } from '../../__fixtures__/mcp-messages';

describe('E2E: Tool Routing Flow', () => {
  let proxy: MCPProxy;

  beforeEach(() => {
    proxy = new MCPProxy({
      schemaFilter: new SchemaFilter(),
      downstreamMCPs: mockMCPConfigs,
      traceMode: false
    });
  });

  it('should route Google Docs tool to googledocs MCP', async () => {
    // Load schemas first
    await proxy.handleRequest(createToolsListRequest());

    const request = createToolsCallRequest('mcp__GoogleDocs__readGoogleDoc', {
      documentId: 'test-doc-123'
    });
    const response = await proxy.handleRequest(request);

    // Tool should be recognized (routing works)
    expect(response.error?.message).not.toContain('Unknown tool');
  });

  it('should route Atlassian tool to atlassian MCP', async () => {
    await proxy.handleRequest(createToolsListRequest());

    const request = createToolsCallRequest('mcp__Atlassian__getIssue', {
      issueKey: 'PROJ-123'
    });
    const response = await proxy.handleRequest(request);

    // Tool should be recognized
    expect(response.error?.message).not.toContain('Unknown tool');
  });

  it('should handle multiple tools in sequence', async () => {
    await proxy.handleRequest(createToolsListRequest());

    const request1 = createToolsCallRequest('mcp__GoogleDocs__readGoogleDoc', {
      documentId: 'doc-1'
    });
    const response1 = await proxy.handleRequest(request1);
    expect(response1.id).toBe(request1.id);

    const request2 = createToolsCallRequest('mcp__Atlassian__getIssue', {
      issueKey: 'PROJ-1'
    });
    const response2 = await proxy.handleRequest(request2);
    expect(response2.id).toBe(request2.id);
  });
});

/**
 * Load Tests: Concurrent Requests
 *
 * Tests gateway performance under concurrent load using autocannon.
 */

import autocannon from 'autocannon';
import http from 'http';
import { MCPProxy } from '../../../src/lib/mcp-proxy';
import { SchemaFilter } from '../../../src/lib/schema-filter';
import { mockMCPConfigs } from '../../__fixtures__/mcp-configs';
import { createToolsListRequest } from '../../__fixtures__/mcp-messages';

describe('Load Tests: Concurrent Requests', () => {
  let proxy: MCPProxy;
  let server: http.Server;
  const PORT = 3000;

  beforeAll(async () => {
    proxy = new MCPProxy({
      schemaFilter: new SchemaFilter(),
      downstreamMCPs: mockMCPConfigs,
      traceMode: false
    });

    server = http.createServer(async (req, res) => {
      let body = '';
      req.on('data', (chunk) => (body += chunk.toString()));
      req.on('end', async () => {
        try {
          const request = JSON.parse(body);
          const response = await proxy.handleRequest(request);
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify(response));
        } catch (error) {
          res.writeHead(500);
          res.end(JSON.stringify({ error: 'Server error' }));
        }
      });
    });

    await new Promise<void>((resolve) => server.listen(PORT, resolve));
  });

  afterAll((done) => {
    server.close(done);
  });

  it('should handle 50 concurrent tools/list requests', async () => {
    const result = await autocannon({
      url: `http://localhost:${PORT}`,
      connections: 50,
      duration: 5,
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(createToolsListRequest())
    });

    expect(result.errors).toBe(0);
    expect((result.latency as any).p95 || result.latency.p97_5).toBeLessThan(500);
    expect(result.requests.average).toBeGreaterThan(100);
  }, 30000);

  it('should handle 100 concurrent tools/list requests', async () => {
    const result = await autocannon({
      url: `http://localhost:${PORT}`,
      connections: 100,
      duration: 5,
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(createToolsListRequest())
    });

    expect(result.errors).toBe(0);
    expect((result.latency as any).p95 || result.latency.p97_5).toBeLessThan(500);
  }, 30000);
});

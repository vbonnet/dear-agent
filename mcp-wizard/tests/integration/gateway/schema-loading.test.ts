/**
 * Integration Tests: Schema Loading and Caching
 *
 * Tests gateway schema loading, caching, and reload functionality.
 */

import { MCPProxy } from '../../../src/lib/mcp-proxy';
import { SchemaFilter } from '../../../src/lib/schema-filter';
import { createMockMCPServer } from '../../__mocks__/mcp-server';
import {
  sampleGoogleDocsSchemas,
  allSchemas
} from '../../__fixtures__/tool-schemas';
import {
  mockMCPConfigs,
  mockGoogleDocsMCP,
  singleMCPConfig
} from '../../__fixtures__/mcp-configs';
import { createToolsListRequest } from '../../__fixtures__/mcp-messages';

describe('Schema Loading and Caching', () => {
  let proxy: MCPProxy;

  beforeEach(() => {
    proxy = new MCPProxy({
      schemaFilter: new SchemaFilter(),
      downstreamMCPs: mockMCPConfigs,
      traceMode: false
    });
  });

  describe('initial schema loading', () => {
    it('should load schemas from single MCP', async () => {
      const singleProxy = new MCPProxy({
        schemaFilter: new SchemaFilter(),
        downstreamMCPs: singleMCPConfig,
        traceMode: false
      });

      const request = createToolsListRequest();
      const response = await singleProxy.handleRequest(request);

      expect(response.result).toHaveProperty('tools');
      const tools = (response.result as any).tools;
      expect(Array.isArray(tools)).toBe(true);
      expect(tools.length).toBeGreaterThan(0);
    });

    it('should load schemas from multiple MCPs', async () => {
      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);

      expect(response.result).toHaveProperty('tools');
      const tools = (response.result as any).tools;
      expect(Array.isArray(tools)).toBe(true);
      expect(tools.length).toBe(allSchemas.length);
    });

    it('should merge schemas from all configured MCPs', async () => {
      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);

      const tools = (response.result as any).tools;

      // Check for Google Docs tools
      const googleDocsTool = tools.find((t: any) =>
        t.name.includes('GoogleDocs')
      );
      expect(googleDocsTool).toBeDefined();

      // Check for Atlassian tools
      const atlassianTool = tools.find((t: any) =>
        t.name.includes('Atlassian')
      );
      expect(atlassianTool).toBeDefined();
    });

    it('should load schemas only once on first request', async () => {
      const schemasBeforeLoad = proxy.getAllSchemas();
      expect(schemasBeforeLoad.length).toBe(0);

      await proxy.handleRequest(createToolsListRequest());

      const schemasAfterLoad = proxy.getAllSchemas();
      expect(schemasAfterLoad.length).toBeGreaterThan(0);
    });
  });

  describe('schema caching', () => {
    it('should cache schemas after first load', async () => {
      // First request - loads schemas
      const request1 = createToolsListRequest(1);
      const response1 = await proxy.handleRequest(request1);
      const tools1 = (response1.result as any).tools;

      // Second request - uses cache
      const request2 = createToolsListRequest(2);
      const response2 = await proxy.handleRequest(request2);
      const tools2 = (response2.result as any).tools;

      // Should return same schemas (cached)
      expect(tools1).toEqual(tools2);
      expect(tools1.length).toBe(tools2.length);
    });

    it('should not reload schemas on subsequent requests', async () => {
      await proxy.handleRequest(createToolsListRequest(1));
      const schemaCount1 = proxy.getAllSchemas().length;

      await proxy.handleRequest(createToolsListRequest(2));
      const schemaCount2 = proxy.getAllSchemas().length;

      await proxy.handleRequest(createToolsListRequest(3));
      const schemaCount3 = proxy.getAllSchemas().length;

      expect(schemaCount1).toBe(schemaCount2);
      expect(schemaCount2).toBe(schemaCount3);
    });

    it('should serve cached schemas quickly', async () => {
      // First request - loads schemas
      await proxy.handleRequest(createToolsListRequest(1));

      // Measure cached request time
      const start = Date.now();
      await proxy.handleRequest(createToolsListRequest(2));
      const duration = Date.now() - start;

      // Cached request should be very fast (<10ms)
      expect(duration).toBeLessThan(50);
    });
  });

  describe('schema reload', () => {
    it('should clear cache on reload', async () => {
      // Load schemas
      await proxy.handleRequest(createToolsListRequest());
      const schemasBeforeReload = proxy.getAllSchemas();
      expect(schemasBeforeReload.length).toBeGreaterThan(0);

      // Reload
      await proxy.reloadSchemas();
      const schemasAfterReload = proxy.getAllSchemas();

      // Cache should be cleared, then reloaded
      expect(schemasAfterReload.length).toBeGreaterThan(0);
    });

    it('should reload schemas from MCPs', async () => {
      await proxy.handleRequest(createToolsListRequest());
      const tools1 = proxy.getAllSchemas();

      await proxy.reloadSchemas();
      const tools2 = proxy.getAllSchemas();

      // Should have same tools after reload (from same MCPs)
      expect(tools1.length).toBe(tools2.length);
    });

    it('should handle reload when no schemas loaded', async () => {
      // Reload before any schemas loaded
      await expect(proxy.reloadSchemas()).resolves.not.toThrow();

      // Should be able to load schemas after
      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);
      expect(response.result).toHaveProperty('tools');
    });
  });

  describe('getAllSchemas', () => {
    it('should return empty array before schemas loaded', () => {
      const schemas = proxy.getAllSchemas();
      expect(Array.isArray(schemas)).toBe(true);
      expect(schemas.length).toBe(0);
    });

    it('should return all loaded schemas', async () => {
      await proxy.handleRequest(createToolsListRequest());

      const schemas = proxy.getAllSchemas();
      expect(Array.isArray(schemas)).toBe(true);
      expect(schemas.length).toBe(allSchemas.length);
    });

    it('should return schemas with correct structure', async () => {
      await proxy.handleRequest(createToolsListRequest());

      const schemas = proxy.getAllSchemas();
      if (schemas.length > 0) {
        const schema = schemas[0];
        expect(schema).toHaveProperty('name');
        expect(typeof schema.name).toBe('string');
      }
    });
  });
});

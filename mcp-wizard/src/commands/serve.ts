/**
 * Serve Command
 *
 * Starts mcp-wizard as an MCP server in stdio mode.
 * This is the meta-server entry point that proxies to downstream MCPs.
 */

import { createInterface } from 'readline';
import { loadDownstreamConfig } from '../lib/config';
import { DownstreamMCPManager } from '../lib/downstream-mcp-manager';
import { createSchemaFilter } from '../lib/schema-filter';
import { MCPProxy, MCPRequest, MCPResponse } from '../lib/mcp-proxy';

export async function serve(): Promise<void> {
  const traceMode = process.env.MCP_WIZARD_TRACE === '1';

  try {
    // Load downstream MCP configuration
    if (traceMode) {
      console.error('[serve] Loading downstream config...');
    }

    const downstreamConfig = await loadDownstreamConfig();

    if (traceMode) {
      console.error(
        '[serve] Loaded config for services:',
        Object.keys(downstreamConfig.downstreamMCPs)
      );
    }

    // Create downstream MCP manager
    const manager = new DownstreamMCPManager(downstreamConfig);

    // Create schema filter
    const schemaFilter = createSchemaFilter({ traceMode });

    // Create MCP proxy
    const proxy = new MCPProxy({
      schemaFilter,
      downstreamMCPs: new Map(), // Legacy, not used when manager is set
      downstreamManager: manager,
      traceMode,
    });

    if (traceMode) {
      console.error('[serve] MCP server ready, listening on stdin...');
    }

    // Setup graceful shutdown
    const shutdown = async () => {
      if (traceMode) {
        console.error('[serve] Shutting down...');
      }
      await manager.shutdown();
      process.exit(0);
    };

    process.on('SIGTERM', shutdown);
    process.on('SIGINT', shutdown);

    // Setup stdio loop (newline-delimited JSON-RPC)
    const rl = createInterface({ input: process.stdin });

    rl.on('line', async (line) => {
      try {
        const request = JSON.parse(line) as MCPRequest;

        if (traceMode) {
          console.error('[serve] Request:', request.method, request.id);
        }

        const response = await proxy.handleRequest(request);

        // Write response to stdout
        console.log(JSON.stringify(response));
      } catch (error) {
        // Parse or handling error - send error response
        const errorResponse: MCPResponse = {
          jsonrpc: '2.0',
          id: null as any, // Unknown ID since parse failed
          error: {
            code: -32700,
            message: `Parse error: ${error instanceof Error ? error.message : String(error)}`,
          },
        };

        console.log(JSON.stringify(errorResponse));
      }
    });

    rl.on('close', async () => {
      if (traceMode) {
        console.error('[serve] stdin closed, shutting down...');
      }
      await shutdown();
    });
  } catch (error) {
    console.error('Failed to start MCP server:', error);
    process.exit(1);
  }
}

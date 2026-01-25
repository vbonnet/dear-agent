/**
 * Mock MCP Server
 *
 * EventEmitter-based mock MCP server for integration testing.
 * Follows the pattern from child_process.ts mock.
 */

import { EventEmitter } from 'events';
import { Readable } from 'stream';
import {
  MCPRequest,
  MCPResponse,
  ToolsListResponse,
  ToolsCallParams,
  ToolsCallResponse
} from '../../src/lib/mcp-proxy';
import { MCPToolSchema } from '../../src/lib/schema-filter';

/**
 * Mock readable stream for stdout/stderr
 */
class MockReadableStream extends Readable {
  _read() {
    // No-op, data will be pushed via push()
  }
}

/**
 * Mock MCP Server that simulates stdio communication
 */
export class MockMCPServer extends EventEmitter {
  // Stdio streams (Readable streams, like child_process)
  stdout = new MockReadableStream();
  stderr = new MockReadableStream();
  stdin: any = {
    write: jest.fn((data: string, callback?: (error?: Error) => void) => {
      try {
        const request = JSON.parse(data.trim());
        this.handleRequest(request);
        if (callback) callback();
      } catch (error) {
        if (callback) callback(error instanceof Error ? error : new Error(String(error)));
      }
    }),
  };

  // Server state
  private schemas: MCPToolSchema[] = [];
  private toolResults: Map<string, unknown> = new Map();
  private errorConfig: { code: number; message: string } | null = null;
  private behaviorMode: 'normal' | 'timeout' | 'crash' | 'invalid-json' = 'normal';
  private receivedRequests: MCPRequest[] = [];
  public killed = false;

  constructor(config?: { schemas?: MCPToolSchema[]; name?: string }) {
    super();
    if (config?.schemas) {
      this.schemas = config.schemas;
    }
  }

  /**
   * Configuration: Set schemas
   */
  setSchemas(schemas: MCPToolSchema[]): void {
    this.schemas = schemas;
  }

  /**
   * Configuration: Set tool result for specific tool
   */
  setToolResult(toolName: string, result: unknown): void {
    this.toolResults.set(toolName, result);
  }

  /**
   * Configuration: Set error to return
   */
  setError(code: number, message: string): void {
    this.errorConfig = { code, message };
  }

  /**
   * Configuration: Clear error
   */
  clearError(): void {
    this.errorConfig = null;
  }

  /**
   * Behavior: Simulate timeout (don't respond)
   */
  simulateTimeout(): void {
    this.behaviorMode = 'timeout';
  }

  /**
   * Behavior: Simulate crash (emit exit event)
   */
  simulateCrash(): void {
    this.behaviorMode = 'crash';
    process.nextTick(() => {
      this.killed = true;
      this.emit('exit', 1, null);
    });
  }

  /**
   * Behavior: Simulate invalid JSON output
   */
  simulateInvalidJSON(): void {
    this.behaviorMode = 'invalid-json';
  }

  /**
   * Behavior: Reset to normal
   */
  resetBehavior(): void {
    this.behaviorMode = 'normal';
  }

  /**
   * Handle incoming request (main entry point)
   */
  handleRequest(request: MCPRequest): void {
    // Store request for verification
    this.receivedRequests.push(request);

    // Handle special behaviors
    if (this.behaviorMode === 'timeout') {
      // Don't respond
      return;
    }

    if (this.behaviorMode === 'crash') {
      // Already handled in simulateCrash()
      return;
    }

    if (this.behaviorMode === 'invalid-json') {
      this.stdout.push(Buffer.from('{ invalid json '));
      return;
    }

    // Check if error configured
    if (this.errorConfig) {
      this.sendResponse({
        jsonrpc: '2.0',
        id: request.id,
        error: this.errorConfig
      });
      return;
    }

    // Normal request handling
    try {
      let result: unknown;

      switch (request.method) {
        case 'initialize':
          result = this.handleInitialize();
          break;
        case 'tools/list':
          result = this.handleToolsList();
          break;
        case 'tools/call':
          result = this.handleToolsCall(request.params as ToolsCallParams);
          break;
        default:
          this.sendResponse({
            jsonrpc: '2.0',
            id: request.id,
            error: {
              code: -32601,
              message: `Method not found: ${request.method}`
            }
          });
          return;
      }

      this.sendResponse({
        jsonrpc: '2.0',
        id: request.id,
        result
      });
    } catch (error) {
      this.sendResponse({
        jsonrpc: '2.0',
        id: request.id,
        error: {
          code: -32603,
          message: error instanceof Error ? error.message : String(error)
        }
      });
    }
  }

  /**
   * Handle initialize request
   */
  private handleInitialize(): unknown {
    return {
      protocolVersion: '2024-11-05',
      serverInfo: {
        name: 'mock-mcp-server',
        version: '0.1.0'
      },
      capabilities: {
        tools: {
          listChanged: false
        }
      }
    };
  }

  /**
   * Handle tools/list request
   */
  private handleToolsList(): ToolsListResponse {
    return {
      tools: this.schemas
    };
  }

  /**
   * Handle tools/call request
   */
  private handleToolsCall(params: ToolsCallParams): ToolsCallResponse {
    if (!params || !params.name) {
      throw new Error('Missing tool name');
    }

    // Check if we have a configured result for this tool
    if (this.toolResults.has(params.name)) {
      const result = this.toolResults.get(params.name);
      return {
        content: [
          {
            type: 'text',
            text: JSON.stringify(result)
          }
        ]
      };
    }

    // Default result
    return {
      content: [
        {
          type: 'text',
          text: `Mock result for tool: ${params.name}`
        }
      ]
    };
  }

  /**
   * Send response on stdout (as MCP servers do)
   */
  sendResponse(response: MCPResponse): void {
    const json = JSON.stringify(response) + '\n';
    this.stdout.push(Buffer.from(json));
  }

  /**
   * Send error message on stderr
   */
  sendError(message: string): void {
    this.stderr.push(Buffer.from(message + '\n'));
  }

  /**
   * Get all received requests (for test verification)
   */
  getReceivedRequests(): MCPRequest[] {
    return [...this.receivedRequests];
  }

  /**
   * Clear received requests
   */
  clearReceivedRequests(): void {
    this.receivedRequests = [];
  }

  /**
   * Kill the mock server (mimics child_process.kill)
   */
  kill(signal?: string): boolean {
    this.killed = true;
    process.nextTick(() => {
      this.emit('exit', 0, signal);
    });
    return true;
  }
}

/**
 * Factory function to create mock MCP server
 */
export function createMockMCPServer(config?: {
  schemas?: MCPToolSchema[];
  name?: string;
}): MockMCPServer {
  return new MockMCPServer(config);
}

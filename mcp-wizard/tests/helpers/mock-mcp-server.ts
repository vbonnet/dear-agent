/**
 * MockMCPServer - Simulates MCP server stdio transport for testing
 *
 * Provides a mock child_process that behaves like a real MCP server
 * supporting JSON-RPC protocol (initialize, tools/list, tools/call).
 *
 * @example
 * ```typescript
 * jest.mock('child_process', () => ({
 *   spawn: jest.fn(() => new MockMCPServer({
 *     'tools/list': () => ({
 *       tools: [{ name: 'readGoogleDoc', description: 'Read a Google Doc' }],
 *     }),
 *   })),
 * }));
 * ```
 */

import { EventEmitter } from 'events';

/**
 * Handler function for MCP JSON-RPC methods
 */
type MCPHandler = (params: any) => any;

/**
 * Map of MCP method names to handler functions
 */
type MCPHandlers = Record<string, MCPHandler>;

/**
 * MockMCPServer simulates a child_process for MCP stdio communication
 *
 * Implements the minimal interface needed for DownstreamMCPClient:
 * - stdout: EventEmitter for JSON-RPC responses
 * - stdin: { write: jest.Mock } for JSON-RPC requests
 * - kill(): Terminates mock server
 * - on('exit'): Emitted when kill() called
 */
export class MockMCPServer extends EventEmitter {
  public stdout: EventEmitter;
  public stdin: { write: jest.Mock };
  private handlers: MCPHandlers;
  private requestBuffer: string = '';

  /**
   * Create a new MockMCPServer
   *
   * @param handlers - Map of MCP method names to handler functions
   *
   * @example
   * ```typescript
   * const server = new MockMCPServer({
   *   'initialize': (params) => ({
   *     protocolVersion: '0.1.0',
   *     capabilities: { tools: {} },
   *   }),
   *   'tools/list': () => ({
   *     tools: [
   *       { name: 'readGoogleDoc', description: 'Read a Google Doc' },
   *     ],
   *   }),
   *   'tools/call': (params) => {
   *     if (params.name === 'readGoogleDoc') {
   *       return { content: 'Mock doc content' };
   *     }
   *     throw new Error(`Unknown tool: ${params.name}`);
   *   },
   * });
   * ```
   */
  constructor(handlers: MCPHandlers) {
    super();
    this.handlers = handlers;
    this.stdout = new EventEmitter();

    // Mock stdin.write to parse JSON-RPC requests and send responses
    this.stdin = {
      write: jest.fn((data: string | Buffer) => {
        const dataStr = typeof data === 'string' ? data : data.toString();
        this.handleInput(dataStr);
        return true;
      }),
    };
  }

  /**
   * Handle input from stdin (JSON-RPC requests)
   * Buffers input and processes complete JSON-RPC messages
   */
  private handleInput(data: string): void {
    this.requestBuffer += data;

    // Process complete JSON-RPC messages (newline-delimited)
    const lines = this.requestBuffer.split('\n');

    // Keep last incomplete line in buffer
    this.requestBuffer = lines.pop() || '';

    for (const line of lines) {
      if (!line.trim()) continue;

      try {
        const request = JSON.parse(line);
        this.handleRequest(request);
      } catch (error) {
        // Invalid JSON - send error response
        const errorResponse = {
          jsonrpc: '2.0',
          id: null,
          error: {
            code: -32700,
            message: 'Parse error',
          },
        };
        this.sendResponse(errorResponse);
      }
    }
  }

  /**
   * Handle a JSON-RPC request and send response
   */
  private handleRequest(request: any): void {
    const { jsonrpc, id, method, params } = request;

    if (jsonrpc !== '2.0') {
      this.sendError(id, -32600, 'Invalid Request: jsonrpc must be "2.0"');
      return;
    }

    if (!method || typeof method !== 'string') {
      this.sendError(id, -32600, 'Invalid Request: missing method');
      return;
    }

    const handler = this.handlers[method];

    if (!handler) {
      this.sendError(id, -32601, `Method not found: ${method}`);
      return;
    }

    try {
      const result = handler(params || {});

      const response = {
        jsonrpc: '2.0',
        id,
        result,
      };

      this.sendResponse(response);
    } catch (error: any) {
      this.sendError(id, -32603, `Internal error: ${error.message}`);
    }
  }

  /**
   * Send a JSON-RPC response to stdout
   */
  private sendResponse(response: any): void {
    const responseStr = JSON.stringify(response) + '\n';
    // Emit on stdout as child_process does
    this.stdout.emit('data', Buffer.from(responseStr));
  }

  /**
   * Send a JSON-RPC error response
   */
  private sendError(id: any, code: number, message: string): void {
    const errorResponse = {
      jsonrpc: '2.0',
      id,
      error: {
        code,
        message,
      },
    };
    this.sendResponse(errorResponse);
  }

  /**
   * Simulate killing the MCP server process
   * Emits 'exit' event as real child_process does
   *
   * @param signal - Signal to kill process (optional, defaults to 'SIGTERM')
   */
  public kill(signal?: string): boolean {
    // Emit exit event (exit code 0 for normal termination, 1 for signal)
    const exitCode = signal ? 1 : 0;
    setImmediate(() => {
      this.emit('exit', exitCode, signal || null);
    });
    return true;
  }
}

/**
 * Create a GoogleDocs MCP mock with standard responses
 */
export function createGoogleDocsMock(): MockMCPServer {
  return new MockMCPServer({
    initialize: () => ({
      protocolVersion: '0.1.0',
      capabilities: {
        tools: {},
      },
      serverInfo: {
        name: 'GoogleDocs',
        version: '1.0.0',
      },
    }),
    'tools/list': () => ({
      tools: [
        {
          name: 'readGoogleDoc',
          description: 'Read the contents of a Google Document',
          inputSchema: {
            type: 'object',
            properties: {
              documentId: { type: 'string' },
            },
            required: ['documentId'],
          },
        },
        {
          name: 'appendToGoogleDoc',
          description: 'Append text to a Google Document',
          inputSchema: {
            type: 'object',
            properties: {
              documentId: { type: 'string' },
              textToAppend: { type: 'string' },
            },
            required: ['documentId', 'textToAppend'],
          },
        },
      ],
    }),
    'tools/call': (params) => {
      if (params.name === 'readGoogleDoc') {
        return {
          content: [
            {
              type: 'text',
              text: 'Mock Google Doc content',
            },
          ],
        };
      }
      if (params.name === 'appendToGoogleDoc') {
        return {
          content: [
            {
              type: 'text',
              text: 'Successfully appended to document',
            },
          ],
        };
      }
      throw new Error(`Unknown tool: ${params.name}`);
    },
  });
}

/**
 * Create an Atlassian MCP mock with standard responses
 */
export function createAtlassianMock(): MockMCPServer {
  return new MockMCPServer({
    initialize: () => ({
      protocolVersion: '0.1.0',
      capabilities: {
        tools: {},
      },
      serverInfo: {
        name: 'Atlassian',
        version: '1.0.0',
      },
    }),
    'tools/list': () => ({
      tools: [
        {
          name: 'createJiraIssue',
          description: 'Create a new Jira issue',
          inputSchema: {
            type: 'object',
            properties: {
              project: { type: 'string' },
              summary: { type: 'string' },
              description: { type: 'string' },
            },
            required: ['project', 'summary'],
          },
        },
      ],
    }),
    'tools/call': (params) => {
      if (params.name === 'createJiraIssue') {
        return {
          content: [
            {
              type: 'text',
              text: 'Created issue PROJ-123',
            },
          ],
        };
      }
      throw new Error(`Unknown tool: ${params.name}`);
    },
  });
}

/**
 * Create an unhealthy MCP mock that times out
 */
export function createUnhealthyMock(): MockMCPServer {
  return new MockMCPServer({
    initialize: () => {
      // Simulate timeout by not responding
      throw new Error('Connection timeout');
    },
    'tools/list': () => {
      throw new Error('MCP server not responding');
    },
  });
}

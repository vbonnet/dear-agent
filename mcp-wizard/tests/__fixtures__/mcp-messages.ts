/**
 * Test Fixtures: MCP Messages
 *
 * Factory functions for creating MCP request/response objects.
 */

import { MCPRequest, MCPResponse, MCPError } from '../../src/lib/mcp-proxy';

/**
 * Create a tools/list request
 */
export const createToolsListRequest = (id: number = 1): MCPRequest => ({
  jsonrpc: '2.0',
  id,
  method: 'tools/list',
  params: {}
});

/**
 * Create a tools/call request
 */
export const createToolsCallRequest = (
  toolName: string,
  args: Record<string, unknown> = {},
  id: number = 2
): MCPRequest => ({
  jsonrpc: '2.0',
  id,
  method: 'tools/call',
  params: {
    name: toolName,
    arguments: args
  }
});

/**
 * Create an initialize request
 */
export const createInitializeRequest = (
  id: number = 0,
  protocolVersion: string = '2024-11-05'
): MCPRequest => ({
  jsonrpc: '2.0',
  id,
  method: 'initialize',
  params: {
    protocolVersion
  }
});

/**
 * Create an unknown method request (for error testing)
 */
export const createUnknownMethodRequest = (id: number = 99): MCPRequest => ({
  jsonrpc: '2.0',
  id,
  method: 'unknown/method',
  params: {}
});

/**
 * Create a tools/call request with missing tool name (for error testing)
 */
export const createInvalidToolsCallRequest = (id: number = 3): MCPRequest => ({
  jsonrpc: '2.0',
  id,
  method: 'tools/call',
  params: {}  // Missing 'name' field
});

/**
 * Create a success response
 */
export const createSuccessResponse = (id: number, result: unknown): MCPResponse => ({
  jsonrpc: '2.0',
  id,
  result
});

/**
 * Create an error response
 */
export const createErrorResponse = (id: number, error: MCPError): MCPResponse => ({
  jsonrpc: '2.0',
  id,
  error
});

/**
 * Create an MCP error object
 */
export const createMCPError = (
  code: number,
  message: string,
  data?: unknown
): MCPError => ({
  code,
  message,
  ...(data !== undefined && { data })
});

/**
 * Standard error codes
 */
export const ERROR_CODES = {
  PARSE_ERROR: -32700,
  INVALID_REQUEST: -32600,
  METHOD_NOT_FOUND: -32601,
  INVALID_PARAMS: -32602,
  INTERNAL_ERROR: -32603
};

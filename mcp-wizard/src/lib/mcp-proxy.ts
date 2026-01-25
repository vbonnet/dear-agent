/**
 * MCP Proxy Server
 *
 * Implements MCP protocol (tools/list, tools/call) with schema filtering.
 * Acts as a proxy between Claude Code and downstream MCP servers.
 *
 * MCP Protocol Reference: https://modelcontextprotocol.io/
 */

import { SchemaFilter, MCPToolSchema, RequirementEnvelope } from './schema-filter';
import { DownstreamMCPManager } from './downstream-mcp-manager';
import { Action, Service } from './intent-analyzer';

// ============================================================================
// MCP Protocol Types
// ============================================================================

/**
 * MCP Request (JSON-RPC 2.0)
 */
export interface MCPRequest {
  jsonrpc: '2.0';
  id: string | number;
  method: string;
  params?: unknown;
}

/**
 * MCP Response (JSON-RPC 2.0)
 */
export interface MCPResponse {
  jsonrpc: '2.0';
  id: string | number;
  result?: unknown;
  error?: MCPError;
}

/**
 * MCP Error
 */
export interface MCPError {
  code: number;
  message: string;
  data?: unknown;
}

/**
 * tools/list response
 */
export interface ToolsListResponse {
  tools: MCPToolSchema[];
}

/**
 * tools/call request params
 */
export interface ToolsCallParams {
  name: string;
  arguments?: Record<string, unknown>;
}

/**
 * tools/call response
 */
export interface ToolsCallResponse {
  content: Array<{
    type: string;
    text?: string;
    data?: unknown;
  }>;
}

// ============================================================================
// MCP Proxy Configuration
// ============================================================================

export interface MCPProxyConfig {
  schemaFilter: SchemaFilter;
  downstreamMCPs: Map<string, DownstreamMCP>;
  downstreamManager?: DownstreamMCPManager; // NEW: Use manager instead of direct map
  traceMode?: boolean;
}

/**
 * Downstream MCP server configuration
 */
export interface DownstreamMCP {
  name: string;
  command: string;
  args: string[];
  env?: Record<string, string>;
  schemas?: MCPToolSchema[]; // Cached schemas
}

// ============================================================================
// MCP Proxy Server
// ============================================================================

export class MCPProxy {
  private schemaFilter: SchemaFilter;
  private downstreamMCPs: Map<string, DownstreamMCP>;
  private downstreamManager?: DownstreamMCPManager; // NEW: Use manager for lazy loading
  private traceMode: boolean;
  private allSchemas: MCPToolSchema[] = [];
  private schemasLoaded = false;
  private lastToolCall: { service: Service; action: Action } | null = null; // NEW: Intent cache

  constructor(config: MCPProxyConfig) {
    this.schemaFilter = config.schemaFilter;
    this.downstreamMCPs = config.downstreamMCPs;
    this.downstreamManager = config.downstreamManager; // NEW
    this.traceMode = config.traceMode ?? false;
  }

  /**
   * Handle MCP request (main entry point)
   */
  async handleRequest(request: MCPRequest): Promise<MCPResponse> {
    this.trace('Incoming request', { method: request.method, id: request.id });

    try {
      let result: unknown;

      switch (request.method) {
        case 'tools/list':
          result = await this.handleToolsList(request);
          break;
        case 'tools/call':
          result = await this.handleToolsCall(request);
          break;
        case 'initialize':
          result = await this.handleInitialize(request);
          break;
        default:
          throw this.createError(-32601, `Method not found: ${request.method}`);
      }

      return {
        jsonrpc: '2.0',
        id: request.id,
        result,
      };
    } catch (error) {
      return {
        jsonrpc: '2.0',
        id: request.id,
        error: this.toMCPError(error),
      };
    }
  }

  /**
   * Handle tools/list - return filtered schemas
   */
  private async handleToolsList(request: MCPRequest): Promise<ToolsListResponse> {
    // NEW: Use downstream manager if available
    if (this.downstreamManager) {
      return await this.handleToolsListWithManager(request);
    }

    // LEGACY: Old behavior for backward compatibility
    if (!this.schemasLoaded) {
      await this.loadDownstreamSchemas();
    }

    const envelope = this.extractIntent(request);
    const filteredSchemas = envelope
      ? this.schemaFilter.filterSchemas(envelope, this.allSchemas)
      : this.allSchemas;

    this.trace('tools/list', {
      total: this.allSchemas.length,
      filtered: filteredSchemas.length,
      intent: envelope?.action,
    });

    return { tools: filteredSchemas };
  }

  /**
   * NEW: Handle tools/list with downstream manager (lazy loading)
   */
  private async handleToolsListWithManager(request: MCPRequest): Promise<ToolsListResponse> {
    // Extract intent from last tool call
    const envelope = this.extractIntent(request);

    // Load schemas based on intent
    let schemas: MCPToolSchema[];
    if (envelope && envelope.service && !envelope.fallback_to_all) {
      // Load from specific service only
      const service = envelope.service as Service; // Cast since we validated it exists
      try {
        schemas = await this.downstreamManager!.getSchemas(service);
        this.trace('tools/list (targeted)', {
          service,
          count: schemas.length,
        });
      } catch (error) {
        console.error(`Failed to load schemas from ${service}:`, error);
        // Fallback to all schemas
        schemas = await this.downstreamManager!.getAllSchemas();
      }
    } else {
      // Fallback: Load from all services
      schemas = await this.downstreamManager!.getAllSchemas();
      this.trace('tools/list (fallback)', { count: schemas.length });
    }

    // Filter schemas using SchemaFilter
    const filteredSchemas = envelope
      ? this.schemaFilter.filterSchemas(envelope, schemas)
      : schemas;

    this.trace('tools/list result', {
      total: schemas.length,
      filtered: filteredSchemas.length,
      intent: envelope?.action,
      service: envelope?.service,
    });

    return { tools: filteredSchemas };
  }

  /**
   * Handle tools/call - route to appropriate downstream MCP
   */
  private async handleToolsCall(request: MCPRequest): Promise<ToolsCallResponse> {
    const params = request.params as ToolsCallParams;
    if (!params?.name) {
      throw this.createError(-32602, 'Missing tool name');
    }

    // NEW: Use downstream manager if available
    if (this.downstreamManager) {
      // Cache intent for next tools/list
      const service = this.parseServiceFromToolName(params.name);
      const action = this.parseActionFromToolName(params.name);
      if (service && action) {
        this.lastToolCall = { service, action };
      }

      // Route to downstream manager
      this.trace('tools/call', { tool: params.name, service, action });
      return await this.downstreamManager.routeToolCall(params.name, params.arguments || {});
    }

    // LEGACY: Old behavior
    const mcpName = this.findMCPForTool(params.name);
    if (!mcpName) {
      throw this.createError(-32602, `Unknown tool: ${params.name}`);
    }

    this.trace('tools/call', { tool: params.name, mcp: mcpName });
    return await this.callDownstreamTool(mcpName, params);
  }

  /**
   * Handle initialize - MCP handshake
   */
  private async handleInitialize(_request: MCPRequest): Promise<unknown> {
    return {
      protocolVersion: '2024-11-05',
      serverInfo: {
        name: 'mcp-wizard-broker',
        version: '0.1.0',
      },
      capabilities: {
        tools: {
          listChanged: false,
        },
      },
    };
  }

  /**
   * Load schemas from all downstream MCPs
   */
  private async loadDownstreamSchemas(): Promise<void> {
    this.trace('Loading schemas from downstream MCPs', {
      count: this.downstreamMCPs.size,
    });

    const allSchemas: MCPToolSchema[] = [];

    for (const [name, mcp] of this.downstreamMCPs.entries()) {
      try {
        // TODO: Actually spawn MCP and call tools/list
        // For now, use cached schemas if available
        if (mcp.schemas) {
          allSchemas.push(...mcp.schemas);
          this.trace('Loaded schemas', { mcp: name, count: mcp.schemas.length });
        }
      } catch (error) {
        console.error(`Failed to load schemas from ${name}:`, error);
      }
    }

    this.allSchemas = allSchemas;
    this.schemasLoaded = true;

    this.trace('Schema loading complete', { total: this.allSchemas.length });
  }

  /**
   * Extract intent from request context
   *
   * NEW: Uses cached intent from last tool call (tool call pattern analysis)
   */
  private extractIntent(_request: MCPRequest): RequirementEnvelope | null {
    // Use cached intent from last tool call
    if (this.lastToolCall) {
      return {
        action: this.lastToolCall.action,
        service: this.lastToolCall.service,
        confidence: 0.8,
        raw_intent: 'inferred from tool call',
        parsed_at: new Date().toISOString(),
        fallback_to_all: false,
      };
    }

    // No cached intent, fallback to all schemas
    return null;
  }

  /**
   * NEW: Parse service from tool name
   * Examples: "googledocs_readGoogleDoc" → "googledocs"
   *           "atlassian_create_issue" → "atlassian"
   */
  private parseServiceFromToolName(toolName: string): Service | null {
    const parts = toolName.split('_');
    if (parts.length === 0) {
      return null;
    }

    const prefix = parts[0].toLowerCase();
    const validServices: Service[] = ['googledocs', 'atlassian', 'slack', 'glean'];

    return validServices.includes(prefix as Service) ? (prefix as Service) : null;
  }

  /**
   * NEW: Parse action from tool name
   * Examples: "atlassian_create_issue" → "CREATE"
   *           "googledocs_read_GoogleDoc" → "READ"
   */
  private parseActionFromToolName(toolName: string): Action | null {
    const lower = toolName.toLowerCase();

    if (lower.includes('create') || lower.includes('add') || lower.includes('post')) {
      return 'CREATE';
    }
    if (lower.includes('read') || lower.includes('get') || lower.includes('list')) {
      return 'READ';
    }
    if (lower.includes('update') || lower.includes('edit') || lower.includes('modify')) {
      return 'UPDATE';
    }
    if (lower.includes('delete') || lower.includes('remove')) {
      return 'DELETE';
    }
    if (lower.includes('search') || lower.includes('find') || lower.includes('query')) {
      return 'SEARCH';
    }

    return 'UNKNOWN';
  }

  /**
   * Find which MCP owns a tool by name prefix
   */
  private findMCPForTool(toolName: string): string | null {
    for (const [name, mcp] of this.downstreamMCPs.entries()) {
      if (mcp.schemas?.some((s) => s.name === toolName)) {
        return name;
      }
    }
    return null;
  }

  /**
   * Call a tool on a downstream MCP
   */
  private async callDownstreamTool(
    mcpName: string,
    params: ToolsCallParams
  ): Promise<ToolsCallResponse> {
    // TODO: Implement actual MCP communication via stdio
    // For now, return a stub response
    this.trace('Calling downstream tool', { mcp: mcpName, tool: params.name });

    throw this.createError(
      -32603,
      'Downstream MCP communication not yet implemented'
    );
  }

  /**
   * Create MCP error object
   */
  private createError(code: number, message: string, data?: unknown): MCPError {
    return { code, message, data };
  }

  /**
   * Convert any error to MCP error
   */
  private toMCPError(error: unknown): MCPError {
    if (this.isMCPError(error)) {
      return error;
    }

    if (error instanceof Error) {
      return {
        code: -32603,
        message: error.message,
        data: { stack: error.stack },
      };
    }

    return {
      code: -32603,
      message: String(error),
    };
  }

  /**
   * Type guard for MCPError
   */
  private isMCPError(error: unknown): error is MCPError {
    return (
      typeof error === 'object' &&
      error !== null &&
      'code' in error &&
      'message' in error
    );
  }

  /**
   * Debug logging
   */
  private trace(message: string, data?: unknown): void {
    if (this.traceMode) {
      console.error(`[MCPProxy] ${message}`, data ? JSON.stringify(data) : '');
    }
  }

  /**
   * Get all loaded schemas
   */
  getAllSchemas(): MCPToolSchema[] {
    return this.allSchemas;
  }

  /**
   * Reload schemas from downstream MCPs
   */
  async reloadSchemas(): Promise<void> {
    this.schemasLoaded = false;
    await this.loadDownstreamSchemas();
  }
}

// ============================================================================
// Factory Function
// ============================================================================

/**
 * Create an MCP proxy instance
 */
export function createMCPProxy(config: MCPProxyConfig): MCPProxy {
  return new MCPProxy(config);
}

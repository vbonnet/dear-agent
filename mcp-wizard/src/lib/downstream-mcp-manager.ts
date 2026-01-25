/**
 * Downstream MCP Manager
 *
 * Manages a pool of downstream MCP clients with lazy spawning.
 * Handles schema loading, tool routing, and lifecycle management.
 */

import { Service } from './intent-analyzer';
import { ToolsCallResponse } from './mcp-proxy';
import { MCPToolSchema } from './schema-filter';
import { DownstreamMCPClient, DownstreamMCPConfig } from './downstream-mcp-client';

export interface DownstreamMCPManagerConfig {
  downstreamMCPs: Record<string, DownstreamMCPConfig>;
}

export class DownstreamMCPManager {
  private config: DownstreamMCPManagerConfig;
  private clients = new Map<Service, DownstreamMCPClient>();
  private spawnLocks = new Map<Service, Promise<DownstreamMCPClient>>();
  private schemaCache = new Map<Service, MCPToolSchema[]>();

  constructor(config: DownstreamMCPManagerConfig) {
    this.config = config;
  }

  /**
   * Get or create a downstream MCP client (lazy spawning with concurrency protection)
   */
  async getOrCreateClient(service: Service): Promise<DownstreamMCPClient> {
    // Return cached client if available
    const existingClient = this.clients.get(service);
    if (existingClient && existingClient.isReady()) {
      return existingClient;
    }

    // Return in-flight spawn promise if already spawning
    const spawnLock = this.spawnLocks.get(service);
    if (spawnLock) {
      return spawnLock;
    }

    // Start new spawn
    const spawnPromise = this.spawnClient(service);
    this.spawnLocks.set(service, spawnPromise);

    try {
      const client = await spawnPromise;
      this.clients.set(service, client);
      return client;
    } finally {
      this.spawnLocks.delete(service);
    }
  }

  /**
   * Spawn a new downstream MCP client
   */
  private async spawnClient(service: Service): Promise<DownstreamMCPClient> {
    const config = this.config.downstreamMCPs[service];
    if (!config) {
      throw new Error(`No configuration found for service: ${service}`);
    }

    console.error(`Spawning downstream MCP: ${service}`);
    const client = new DownstreamMCPClient(config);
    await client.start();
    console.error(`Downstream MCP ready: ${service}`);

    return client;
  }

  /**
   * Get tool schemas from a specific service
   */
  async getSchemas(service: Service): Promise<MCPToolSchema[]> {
    // Check cache first
    const cached = this.schemaCache.get(service);
    if (cached) {
      return cached;
    }

    // Load from downstream MCP
    const client = await this.getOrCreateClient(service);
    const schemas = await client.callToolsList();

    // Cache schemas
    this.schemaCache.set(service, schemas);

    return schemas;
  }

  /**
   * Get all schemas from all configured services (for fallback)
   */
  async getAllSchemas(): Promise<MCPToolSchema[]> {
    const allSchemas: MCPToolSchema[] = [];
    const services = Object.keys(this.config.downstreamMCPs) as Service[];

    for (const service of services) {
      try {
        const schemas = await this.getSchemas(service);
        allSchemas.push(...schemas);
      } catch (error) {
        console.error(`Failed to load schemas from ${service}:`, error);
        // Continue with other services (graceful degradation)
      }
    }

    return allSchemas;
  }

  /**
   * Route a tool call to the appropriate downstream MCP
   */
  async routeToolCall(
    toolName: string,
    args: Record<string, unknown>
  ): Promise<ToolsCallResponse> {
    // Parse service from tool name (prefix before first underscore)
    const service = this.parseServiceFromToolName(toolName);
    if (!service) {
      throw new Error(`Could not determine service for tool: ${toolName}`);
    }

    // Get or create client for service
    const client = await this.getOrCreateClient(service);

    // Call tool
    const response = await client.callTool(toolName, args);

    if (response.error) {
      throw new Error(`Tool call failed: ${response.error.message}`);
    }

    return response.result as ToolsCallResponse;
  }

  /**
   * Parse service from tool name
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
   * Shutdown all downstream MCP clients
   */
  async shutdown(): Promise<void> {
    console.error('Shutting down all downstream MCPs');

    const shutdownPromises = Array.from(this.clients.values()).map((client) =>
      client.stop().catch((error) => {
        console.error('Error stopping client:', error);
      })
    );

    await Promise.all(shutdownPromises);

    this.clients.clear();
    this.spawnLocks.clear();
    this.schemaCache.clear();
  }
}

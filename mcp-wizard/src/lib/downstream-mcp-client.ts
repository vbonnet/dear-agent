/**
 * Downstream MCP Client
 *
 * Manages communication with a single downstream MCP server via stdio.
 * Handles subprocess spawning, JSON-RPC communication, and lifecycle management.
 */

import { spawn, ChildProcess } from 'child_process';
import { createInterface, Interface } from 'readline';
import { MCPRequest, MCPResponse, MCPError } from './mcp-proxy';
import { MCPToolSchema } from './schema-filter';

export interface DownstreamMCPConfig {
  command: string;
  args: string[];
  env?: Record<string, string>;
}

interface PendingRequest {
  resolve: (response: MCPResponse) => void;
  reject: (error: Error) => void;
  timer: NodeJS.Timeout;
}

export class DownstreamMCPClient {
  private config: DownstreamMCPConfig;
  private process: ChildProcess | null = null;
  private readline: Interface | null = null;
  private ready = false;
  private requestId = 0;
  private pendingRequests = new Map<string | number, PendingRequest>();
  private requestTimeout: number;

  constructor(config: DownstreamMCPConfig, options?: { requestTimeout?: number }) {
    this.config = config;
    this.requestTimeout = options?.requestTimeout ?? 30000; // 30 seconds default
  }

  /**
   * Start the downstream MCP process and initialize
   */
  async start(): Promise<void> {
    if (this.process) {
      throw new Error('MCP client already started');
    }

    try {
      // Spawn child process
      this.process = spawn(this.config.command, this.config.args, {
        stdio: ['pipe', 'pipe', 'pipe'],
        env: { ...process.env, ...this.config.env },
      });

      // Handle process errors
      this.process.on('error', (error) => {
        console.error(`MCP process spawn error (${this.config.command}):`, error);
        this.rejectAllPending(error);
      });

      this.process.on('exit', (code, signal) => {
        console.error(`MCP process exited (${this.config.command}):`, { code, signal });
        this.ready = false;
        this.rejectAllPending(new Error(`Process exited: code=${code} signal=${signal}`));
      });

      // Setup stdout reader (newline-delimited JSON)
      if (!this.process.stdout) {
        throw new Error('Process stdout not available');
      }

      this.readline = createInterface({ input: this.process.stdout });
      this.readline.on('line', (line) => {
        try {
          const response = JSON.parse(line) as MCPResponse;
          this.handleResponse(response);
        } catch (error) {
          console.error('Failed to parse MCP response:', line, error);
        }
      });

      // Send initialize request
      const initResponse = await this.sendRequest({
        jsonrpc: '2.0',
        id: this.requestId++,
        method: 'initialize',
        params: {
          protocolVersion: '2024-11-05',
          clientInfo: {
            name: 'mcp-wizard',
            version: '0.1.0',
          },
          capabilities: {},
        },
      });

      if (initResponse.error) {
        throw new Error(`Initialize failed: ${initResponse.error.message}`);
      }

      this.ready = true;
    } catch (error) {
      this.cleanup();
      throw error;
    }
  }

  /**
   * Send a request to the downstream MCP and await response
   */
  async sendRequest(request: MCPRequest): Promise<MCPResponse> {
    if (!this.process || !this.process.stdin) {
      throw new Error('MCP client not started');
    }

    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pendingRequests.delete(request.id);
        reject(new Error(`Request timeout after ${this.requestTimeout}ms`));
      }, this.requestTimeout);

      this.pendingRequests.set(request.id, { resolve, reject, timer });

      // Write request to stdin
      const requestLine = JSON.stringify(request) + '\n';
      this.process!.stdin!.write(requestLine, (error) => {
        if (error) {
          clearTimeout(timer);
          this.pendingRequests.delete(request.id);
          reject(error);
        }
      });
    });
  }

  /**
   * Call tools/list on the downstream MCP
   */
  async callToolsList(): Promise<MCPToolSchema[]> {
    const response = await this.sendRequest({
      jsonrpc: '2.0',
      id: this.requestId++,
      method: 'tools/list',
      params: {},
    });

    if (response.error) {
      throw new Error(`tools/list failed: ${response.error.message}`);
    }

    const result = response.result as { tools: MCPToolSchema[] };
    return result.tools;
  }

  /**
   * Call a tool on the downstream MCP
   */
  async callTool(name: string, args: Record<string, unknown>): Promise<MCPResponse> {
    return await this.sendRequest({
      jsonrpc: '2.0',
      id: this.requestId++,
      method: 'tools/call',
      params: { name, arguments: args },
    });
  }

  /**
   * Stop the downstream MCP process
   */
  async stop(): Promise<void> {
    if (!this.process) {
      return;
    }

    return new Promise((resolve) => {
      const forceKillTimer = setTimeout(() => {
        console.error('Force killing MCP process');
        this.process?.kill('SIGKILL');
        resolve();
      }, 5000); // 5 second graceful shutdown window

      this.process!.once('exit', () => {
        clearTimeout(forceKillTimer);
        this.cleanup();
        resolve();
      });

      // Send graceful shutdown signal
      this.process!.kill('SIGTERM');
    });
  }

  /**
   * Check if client is ready
   */
  isReady(): boolean {
    return this.ready;
  }

  /**
   * Handle response from downstream MCP
   */
  private handleResponse(response: MCPResponse): void {
    const pending = this.pendingRequests.get(response.id);
    if (pending) {
      clearTimeout(pending.timer);
      this.pendingRequests.delete(response.id);
      pending.resolve(response);
    }
  }

  /**
   * Reject all pending requests (on error or exit)
   */
  private rejectAllPending(error: Error): void {
    for (const [id, pending] of this.pendingRequests.entries()) {
      clearTimeout(pending.timer);
      pending.reject(error);
      this.pendingRequests.delete(id);
    }
  }

  /**
   * Cleanup resources
   */
  private cleanup(): void {
    this.readline?.close();
    this.readline = null;
    this.process = null;
    this.ready = false;
  }
}

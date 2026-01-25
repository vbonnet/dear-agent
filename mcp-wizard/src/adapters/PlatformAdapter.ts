/**
 * Platform adapter interface for multi-platform MCP configuration
 *
 * Supports Claude Code, Gemini CLI, and future AI platforms.
 * Implements hybrid approach: CLI-first (if available), file-fallback.
 */

/**
 * MCP server configuration (platform-agnostic format)
 */
export interface MCPServer {
  name: string;
  type: 'stdio' | 'http' | 'sse';
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  url?: string;
  headers?: Record<string, string>;
}

/**
 * Platform adapter for configuring MCP servers
 *
 * Each platform (Claude Code, Gemini, etc.) implements this interface
 * to handle platform-specific configuration mechanisms.
 */
export interface PlatformAdapter {
  /**
   * Check if platform CLI is available
   *
   * Claude Code: checks for `claude` command
   * Gemini: always returns false (no CLI commands)
   *
   * @returns Promise<boolean> - true if CLI available, false otherwise
   */
  hasCLI(): Promise<boolean>;

  /**
   * Configure MCP servers for this platform
   *
   * Claude Code: Try CLI first (`claude mcp add`), fallback to ~/.claude.json
   * Gemini: Write to ~/.gemini/settings.json (file-only)
   *
   * @param servers - Array of MCP servers to configure
   * @throws {CLINotFoundError} - If CLI required but not found
   * @throws {FileWriteError} - If file write fails
   * @throws {UnsupportedPlatformError} - If platform not supported
   */
  configure(servers: MCPServer[]): Promise<void>;
}

import { execFile } from 'child_process';
import { promisify } from 'util';

const execFileAsync = promisify(execFile);

export interface VerificationResult {
  success: boolean;
  mcpServers: string[];
  error?: string;
}

export class SetupVerifier {
  /**
   * Verify MCP connection by running `claude mcp list`
   * @param expectedMcp - MCP server name to look for (e.g., "googledocs")
   * @param timeoutMs - Timeout for `claude mcp list` command (default: 10000)
   * @returns VerificationResult with success/failure + list of detected MCPs
   */
  async verifyMcpConnection(
    expectedMcp: string = 'googledocs',
    timeoutMs: number = 10000
  ): Promise<VerificationResult> {
    try {
      // Timeout after 10s (default)
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

      const { stdout } = await execFileAsync('claude', ['mcp', 'list'], {
        signal: controller.signal as any,
      });
      clearTimeout(timeoutId);

      // Parse output (handles both JSON and text formats)
      const mcpServers = this.parseMcpList(stdout);

      // Check if expected MCP is present (case-insensitive)
      const found = mcpServers.some(
        (server) => server.toLowerCase() === expectedMcp.toLowerCase()
      );

      if (found) {
        return {
          success: true,
          mcpServers,
        };
      } else {
        return {
          success: false,
          mcpServers,
          error: `${expectedMcp} not found in MCP list`,
        };
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        return {
          success: false,
          mcpServers: [],
          error: `claude mcp list timed out after ${timeoutMs}ms`,
        };
      }

      return {
        success: false,
        mcpServers: [],
        error: `Failed to verify: ${error.message}`,
      };
    }
  }

  /**
   * Parse `claude mcp list` output supporting both JSON and text formats
   * Handles newer JSON format and legacy text output for backwards compatibility
   * @param stdout - Raw output from `claude mcp list`
   * @returns Array of MCP server names
   * @private
   */
  private parseMcpList(stdout: string): string[] {
    // Try JSON parse first (newer Claude Code versions)
    try {
      const json = JSON.parse(stdout);
      // Format: { "googledocs": {...}, "glean": {...} }
      return Object.keys(json);
    } catch {
      // Fallback: text parsing (older versions)
      // Example output:
      // Available MCP servers:
      //   - GoogleDocs
      //   - Glean
      const lines = stdout.split('\n');
      return lines
        .filter((line) => line.trim().startsWith('-'))
        .map((line) => line.trim().substring(2).trim()); // "- GoogleDocs" → "GoogleDocs"
    }
  }
}

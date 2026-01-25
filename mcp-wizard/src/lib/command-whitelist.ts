/**
 * Command whitelist and validation for MCP server configurations
 * Prevents arbitrary command execution and shell injection attacks
 */

/**
 * MCP server configuration structure
 * Re-exported from config.ts for convenience
 */
export interface McpServer {
  command: string;
  args: string[];
  env?: Record<string, string>;
}

/**
 * Commands allowed for MCP server execution
 * Only npx is allowed to ensure sandboxed execution of npm packages
 */
const ALLOWED_COMMANDS = ['npx'];

/**
 * Shell injection patterns to detect in command arguments
 * Detects common shell metacharacters used for command injection
 */
const SHELL_INJECTION_PATTERNS = [
  /;/,           // Command separator
  /\|/,          // Pipe operator
  /&/,           // Background execution or AND
  />/,           // Redirect output
  /</,           // Redirect input
  /`/,           // Backtick command substitution
  /\$\(/,        // $(...) command substitution
  /\|\|/,        // Logical OR
  /&&/,          // Logical AND
  /\n/,          // Newline separator
];

/**
 * Whitelisted MCP packages (optional validation)
 * Allows only known and trusted MCP server packages
 */
const ALLOWED_MCP_PACKAGES = [
  '@modelcontextprotocol/server-gdocs',
  '@modelcontextprotocol/server-github',
  '@modelcontextprotocol/server-gitlab',
  '@modelcontextprotocol/server-slack',
  '@gleanwork/mcp-server',
  'mcp-remote',
];

/**
 * Validate MCP command configuration
 *
 * Ensures command is whitelisted and args don't contain shell injection
 * Fail-secure: Reject if unknown or suspicious
 *
 * @param config - MCP server config with command and args
 * @throws Error if command not whitelisted or shell injection detected
 *
 * @example
 * validateMcpCommand({
 *   command: 'npx',
 *   args: ['-y', '@modelcontextprotocol/server-gdocs']
 * }); // OK
 *
 * validateMcpCommand({
 *   command: 'rm',
 *   args: ['-rf', '/']
 * }); // Throws: Command not whitelisted
 */
export function validateMcpCommand(config: McpServer): void {
  // 1. Validate command exists and is a string
  if (!config.command || typeof config.command !== 'string') {
    throw new Error('Command must be a non-empty string');
  }

  // 2. Check command whitelist
  if (!ALLOWED_COMMANDS.includes(config.command)) {
    throw new Error(
      `Command "${config.command}" not whitelisted. Only "npx" is allowed.`
    );
  }

  // 3. Validate args exists and is array
  if (!Array.isArray(config.args)) {
    throw new Error('Args must be an array');
  }

  // 4. Check shell injection
  if (detectShellInjection(config.args)) {
    throw new Error(
      `Shell injection detected in args: "${config.args.join(' ')}"`
    );
  }

  // Note: Package whitelist validation is optional and can be enabled if needed
  // const packageName = extractPackageName(config.args);
  // if (packageName && !isWhitelistedPackage(packageName)) {
  //   throw new Error(`Package "${packageName}" not whitelisted.`);
  // }
}

/**
 * Detect shell injection patterns in command arguments
 *
 * Checks for: ;, |, &, >, <, `, $(), ||, &&, \n
 *
 * @param args - Command arguments to check
 * @returns true if injection detected, false otherwise
 */
function detectShellInjection(args: string[]): boolean {
  // Combine all args into single string for pattern matching
  const combined = args.join(' ');

  // Check each pattern
  return SHELL_INJECTION_PATTERNS.some(pattern => pattern.test(combined));
}

/**
 * Check if package is whitelisted
 *
 * @param packageName - NPM package name from args
 * @returns true if whitelisted, false otherwise
 */
export function isWhitelistedPackage(packageName: string): boolean {
  return ALLOWED_MCP_PACKAGES.includes(packageName);
}

/**
 * Extract package name from npx args
 *
 * Examples:
 *   ['-y', '@modelcontextprotocol/server-gdocs'] → '@modelcontextprotocol/server-gdocs'
 *   ['@gleanwork/mcp-server'] → '@gleanwork/mcp-server'
 *   ['mcp-remote@latest', 'https://...'] → 'mcp-remote'
 *
 * @param args - npx arguments
 * @returns Package name or null if not found
 */
export function extractPackageName(args: string[]): string | null {
  // Find first arg that looks like a package name (not a flag)
  const packageArg = args.find(arg => !arg.startsWith('-'));
  if (!packageArg) return null;

  // Strip version specifier (@latest, @1.0.0)
  // Handle scoped packages (@org/package@version)
  if (packageArg.startsWith('@')) {
    // Scoped package: @org/package@version → @org/package
    const parts = packageArg.split('@');
    if (parts.length >= 3) {
      // @org/package@version → parts = ['', 'org/package', 'version']
      return '@' + parts[1];
    }
    return packageArg; // @org/package (no version)
  } else {
    // Regular package: package@version → package
    const packageName = packageArg.split('@')[0];
    return packageName;
  }
}

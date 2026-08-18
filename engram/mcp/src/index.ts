#!/usr/bin/env node
/**
 * Engram MCP Server
 *
 * Provides MCP tools for:
 * - engram.retrieve(): Fetch engram content by ID/tag
 * - engram.plugins.list(): List available engram plugins
 * - wayfinder.phase.status(): Get current phase status
 */

import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
  Tool,
} from '@modelcontextprotocol/sdk/types.js';
import { execFileSync } from 'child_process';
import { existsSync, readFileSync, readdirSync } from 'fs';
import { join, resolve } from 'path';
import { homedir } from 'os';
import { TTLCache } from './cache.js';
import { parseWayfinderStatus } from './wayfinder_status.js';

/**
 * Configuration
 */
const ENGRAM_ROOT = process.env.ENGRAM_ROOT || join(homedir(), '.engram');
const ENGRAM_CLI = process.env.ENGRAM_CLI || 'engram';

/** Cache TTL in ms. Override via MCP_CACHE_TTL_MS env var. */
const configuredCacheTTL = Number.parseInt(process.env.MCP_CACHE_TTL_MS || '', 10);
const CACHE_TTL_MS = Number.isFinite(configuredCacheTTL) && configuredCacheTTL > 0
  ? configuredCacheTTL
  : 30000;

/**
 * Tool result cache — invalidated by TTL or file watches.
 */
const cache = new TTLCache<string>({ defaultTTLMs: CACHE_TTL_MS, maxEntries: 200 });

/**
 * Tool Definitions
 */
const TOOLS: Tool[] = [
  {
    name: 'engram.retrieve',
    description: 'Retrieve engram content by query, tag, or ID. Returns relevant memory traces and patterns.',
    inputSchema: {
      type: 'object',
      properties: {
        query: {
          type: 'string',
          description: 'Search query for memory retrieval',
        },
        tag: {
          type: 'string',
          description: 'Optional tag filter (e.g., "go", "python", "architecture")',
        },
        limit: {
          type: 'number',
          description: 'Maximum number of results (default: 5)',
          default: 5,
        },
      },
      required: ['query'],
    },
  },
  {
    name: 'engram.plugins.list',
    description: 'List installed engram plugins with their status and descriptions.',
    inputSchema: {
      type: 'object',
      properties: {},
    },
  },
  {
    name: 'wayfinder.phase.status',
    description: 'Get current Wayfinder phase status for a project directory.',
    inputSchema: {
      type: 'object',
      properties: {
        project: {
          type: 'string',
          description: 'Project directory path (absolute or relative)',
        },
      },
      required: ['project'],
    },
  },
];

/**
 * Execute engram CLI command.
 *
 * Uses execFileSync with an argv array so no shell is involved — argument
 * values are passed verbatim to the child process, never expanded as a
 * shell string. This is the load-bearing defense against command injection
 * via MCP tool arguments: callers (handleEngramRetrieve, …) push
 * untrusted strings (query, tag, …) onto `args`, and we must not let shell
 * metacharacters like `;`, `|`, `$(...)`, backticks, redirections, or
 * spaces break out of an argument boundary.
 */
function execEngram(args: string[]): string {
  try {
    return execFileSync(ENGRAM_CLI, args, {
      encoding: 'utf-8',
      maxBuffer: 10 * 1024 * 1024, // 10MB
      timeout: 30000, // 30s
      shell: false,
    });
  } catch (error: any) {
    throw new Error(`Engram CLI error: ${error.message}`);
  }
}

/**
 * Tool Handlers
 */
async function handleEngramRetrieve(args: any): Promise<string> {
  const { query, tag, limit = 5 } = args;

  if (typeof query !== 'string' || query.length === 0) {
    throw new Error('query must be a non-empty string');
  }
  if (tag !== undefined && tag !== null && typeof tag !== 'string') {
    throw new Error('tag must be a string');
  }
  const limitNum = Number(limit);
  if (!Number.isInteger(limitNum) || limitNum <= 0 || limitNum > 1000) {
    throw new Error('limit must be a positive integer <= 1000');
  }

  const cacheKey = `retrieve:${query}:${tag || ''}:${limitNum}`;
  const cached = cache.get(cacheKey);
  if (cached !== undefined) return cached;

  const cliArgs = ['retrieve', query];
  if (tag) {
    cliArgs.push('--tag', tag);
  }
  if (limitNum !== 5) {
    cliArgs.push('--limit', String(limitNum));
  }

  try {
    const output = execEngram(cliArgs);
    const result = output || 'No results found';
    cache.set(cacheKey, result);
    return result;
  } catch (error: any) {
    return `Error retrieving engrams: ${error.message}`;
  }
}

async function handleEngramPluginsList(): Promise<string> {
  const cacheKey = 'plugins:list';
  const cached = cache.get(cacheKey);
  if (cached !== undefined) return cached;

  const pluginDirs = [
    join(ENGRAM_ROOT, 'core', 'plugins'),
    join(ENGRAM_ROOT, 'user', 'plugins'),
  ];

  // Watch plugin directories for changes
  for (const dir of pluginDirs) {
    cache.watchFile(dir, 'plugins:');
  }

  const plugins: Array<{
    name: string;
    type: string;
    description: string;
    location: string;
  }> = [];

  for (const dir of pluginDirs) {
    if (!existsSync(dir)) {
      continue;
    }

    try {
      const entries = readdirSync(dir, { withFileTypes: true });
      const location = dir.includes('core') ? 'core' : 'user';

      for (const entry of entries) {
        if (!entry.isDirectory()) continue;

        const pluginYaml = join(dir, entry.name, 'plugin.yaml');
        if (!existsSync(pluginYaml)) continue;

        try {
          // Simple YAML parsing (basic implementation)
          const content = readFileSync(pluginYaml, 'utf-8');
          const nameMatch = content.match(/^name:\s*(.+)$/m);
          const typeMatch = content.match(/^type:\s*(.+)$/m);
          const descMatch = content.match(/^description:\s*(.+)$/m);

          plugins.push({
            name: nameMatch ? nameMatch[1].trim() : entry.name,
            type: typeMatch ? typeMatch[1].trim() : 'unknown',
            description: descMatch ? descMatch[1].trim() : 'No description',
            location,
          });
        } catch (err) {
          // Skip malformed plugin.yaml
          continue;
        }
      }
    } catch (err) {
      // Directory not readable, skip
      continue;
    }
  }

  if (plugins.length === 0) {
    return 'No plugins found';
  }

  // Format as readable text
  const lines = ['Available Engram Plugins:', ''];
  for (const plugin of plugins) {
    lines.push(`**${plugin.name}** (${plugin.type})`);
    lines.push(`  ${plugin.description}`);
    lines.push(`  Location: ${plugin.location}`);
    lines.push('');
  }

  const result = lines.join('\n');
  cache.set(cacheKey, result);
  return result;
}

async function handleWayfinderPhaseStatus(args: any): Promise<string> {
  const { project } = args;

  // Resolve project path
  const projectPath = resolve(project);

  const cacheKey = `wayfinder:${projectPath}`;
  const cached = cache.get(cacheKey);
  if (cached !== undefined) return cached;

  // Check for WAYFINDER-STATUS.md file
  const statusFile = join(projectPath, 'WAYFINDER-STATUS.md');

  if (!existsSync(statusFile)) {
    return `No Wayfinder status found for project: ${projectPath}\nExpected file: ${statusFile}`;
  }

  // Watch status file for changes
  cache.watchFile(statusFile, `wayfinder:${projectPath}`);

  try {
    const content = readFileSync(statusFile, 'utf-8');

    const canonical = parseWayfinderStatus(content);

    const result = {
      project: projectPath,
      phase: canonical.phase,
      progress: canonical.progress,
      status: canonical.status,
    };

    const output = JSON.stringify(result, null, 2);
    cache.set(cacheKey, output);
    return output;
  } catch (error: any) {
    return `Error reading Wayfinder status: ${error.message}`;
  }
}

/**
 * Main Server
 */
async function main() {
  const server = new Server(
    {
      name: 'engram-mcp-server',
      version: '0.1.0',
    },
    {
      capabilities: {
        tools: {},
      },
    }
  );

  // List tools handler
  server.setRequestHandler(ListToolsRequestSchema, async () => {
    return {
      tools: TOOLS,
    };
  });

  // Call tool handler
  server.setRequestHandler(CallToolRequestSchema, async (request) => {
    const { name, arguments: args } = request.params;

    try {
      let result: string;

      switch (name) {
        case 'engram.retrieve':
          result = await handleEngramRetrieve(args);
          break;
        case 'engram.plugins.list':
          result = await handleEngramPluginsList();
          break;
        case 'wayfinder.phase.status':
          result = await handleWayfinderPhaseStatus(args);
          break;
        default:
          throw new Error(`Unknown tool: ${name}`);
      }

      return {
        content: [
          {
            type: 'text',
            text: result,
          },
        ],
      };
    } catch (error: any) {
      return {
        content: [
          {
            type: 'text',
            text: `Error: ${error.message}`,
          },
        ],
        isError: true,
      };
    }
  });

  // Start server
  const transport = new StdioServerTransport();
  await server.connect(transport);

  // Log startup (to stderr, not to interfere with stdio protocol)
  console.error('Engram MCP Server started');
}

main().catch((error) => {
  console.error('Fatal error:', error);
  process.exit(1);
});

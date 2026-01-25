/**
 * Chezmoi Manager - Automates chezmoi template creation and application
 *
 * Provides functions to detect chezmoi, write template files, and apply
 * configurations automatically via chezmoi CLI.
 */

import { exec } from 'child_process';
import { promises as fs } from 'fs';
import * as path from 'path';
import * as os from 'os';
import { promisify } from 'util';
import { McpConfig } from './config';
import { validatePath } from './config';

const execAsync = promisify(exec);

/**
 * Result of detecting chezmoi on the system
 */
export interface ChezmoiDetection {
  /** Whether chezmoi is detected and usable */
  detected: boolean;

  /** Chezmoi source directory path (if detected) */
  sourcePath?: string;

  /** Reason if not detected */
  reason?: 'not installed' | 'not initialized' | 'permission denied';
}

/**
 * Result of applying chezmoi configuration
 */
export interface ApplyResult {
  /** Whether apply succeeded */
  success: boolean;

  /** Combined stdout/stderr output */
  output: string;

  /** Error message if failed */
  error?: string;
}

/**
 * Result of automated chezmoi setup flow
 */
export interface SetupResult {
  /** Method used: automated or manual fallback */
  method: 'automated' | 'manual';

  /** Apply result if automated */
  result?: ApplyResult;

  /** Error that triggered fallback (if manual) */
  error?: string;
}

/**
 * Detect if chezmoi is installed and initialized
 *
 * @returns Detection result with source path if available
 *
 * @example
 * const detection = await detectChezmoi();
 * if (detection.detected) {
 *   console.log(`Chezmoi source: ${detection.sourcePath}`);
 * } else {
 *   console.log(`Chezmoi not available: ${detection.reason}`);
 * }
 */
export async function detectChezmoi(): Promise<ChezmoiDetection> {
  try {
    // Check if chezmoi command exists
    await execAsync('which chezmoi', { timeout: 5000 });

    // Get source directory path
    const { stdout } = await execAsync('chezmoi source-path', { timeout: 5000 });
    const sourcePath = stdout.trim();

    // Verify source directory exists
    await fs.access(sourcePath);

    return { detected: true, sourcePath };
  } catch (error: any) {
    // Chezmoi not installed
    if (error.code === 'ENOENT' || error.message.includes('not found')) {
      return { detected: false, reason: 'not installed' };
    }

    // Permission denied
    if (error.code === 'EACCES') {
      return { detected: false, reason: 'permission denied' };
    }

    // Chezmoi not initialized (source-path failed)
    return { detected: false, reason: 'not initialized' };
  }
}

/**
 * Get template file path for given agent
 *
 * Converts agent config directory to chezmoi template path:
 * - .config/claude-code → dot_config/claude-code/private_mcp.json.tmpl
 * - .cursor → dot_cursor/private_mcp.json.tmpl
 *
 * @param sourcePath Chezmoi source directory
 * @param agentName Agent name (claude-code, cursor, cline, windsurf)
 * @returns Absolute path to template file
 * @throws Error if agent unsupported or path invalid
 *
 * @example
 * const templatePath = getTemplateFilePath(
 *   '/home/user/.local/share/chezmoi',
 *   'claude-code'
 * );
 * // Returns: /home/user/.local/share/chezmoi/dot_config/claude-code/private_mcp.json.tmpl
 */
export function getTemplateFilePath(
  sourcePath: string,
  agentName: string
): string {
  // Map agent name to config directory
  const agentConfigMap: Record<string, string> = {
    'claude-code': '.config/claude-code',
    'cursor': '.cursor',
    'cline': '.cline',
    'windsurf': '.codeium/windsurf',
  };

  const configDir = agentConfigMap[agentName];
  if (!configDir) {
    throw new Error(`Unsupported agent: ${agentName}`);
  }

  // Convert to chezmoi template path
  // .config/claude-code → dot_config/claude-code
  const parts = configDir.split('/');
  const chezmoiParts = parts.map((part) => {
    if (part.startsWith('.')) {
      return 'dot_' + part.substring(1);
    }
    return part;
  });
  const chezmoiPath = chezmoiParts.join('/');

  const templatePath = path.join(
    sourcePath,
    chezmoiPath,
    'private_mcp.json.tmpl'
  );

  // Validate path (no traversal, within source directory)
  validatePath(templatePath);
  if (!templatePath.startsWith(sourcePath)) {
    throw new Error('Template path outside source directory');
  }

  return templatePath;
}

/**
 * Write MCP config to chezmoi template file
 *
 * Creates template file with chezmoi conditional syntax:
 * - Work machines (hostname ends with -w): Full MCP config
 * - Other machines: Empty config
 *
 * @param config MCP configuration object
 * @param sourcePath Chezmoi source directory
 * @param agentName Agent name (default: claude-code)
 *
 * @example
 * const config = { mcpServers: { GoogleDocs: {...} } };
 * await writeChezmoiTemplate(config, '/home/user/.local/share/chezmoi');
 */
export async function writeChezmoiTemplate(
  config: McpConfig,
  sourcePath: string,
  agentName: string = 'claude-code'
): Promise<void> {
  const templatePath = getTemplateFilePath(sourcePath, agentName);

  // Generate template content with chezmoi conditional
  const templateContent = `{{- if hasSuffix "-w" .chezmoi.hostname }}
${JSON.stringify(config, null, 2)}
{{- else }}
{ "mcpServers": {} }
{{- end }}`;

  // Ensure parent directory exists
  await fs.mkdir(path.dirname(templatePath), { recursive: true });

  // Write template file
  await fs.writeFile(templatePath, templateContent, 'utf8');

  console.log(`  ✓ Wrote chezmoi template: ${templatePath}`);
}

/**
 * Show diff of changes that will be applied
 *
 * Runs `chezmoi diff <targetFile>` to preview changes.
 *
 * @param targetFile Target file path (e.g., ~/.config/claude-code/mcp.json)
 * @returns Diff output or "No changes detected"
 *
 * @example
 * const diff = await showChezmoiDiff('~/.config/claude-code/mcp.json');
 * console.log(diff);
 */
export async function showChezmoiDiff(targetFile: string): Promise<string> {
  try {
    const { stdout } = await execAsync(`chezmoi diff "${targetFile}"`, {
      timeout: 10000
    });

    if (!stdout || stdout.trim().length === 0) {
      return 'No changes detected';
    }

    return stdout;
  } catch (error: any) {
    // Exit code 1 with stdout = diff exists (this is success)
    if (error.stdout && error.stdout.trim().length > 0) {
      return error.stdout;
    }

    // Actual error
    return `Error running diff: ${error.message}`;
  }
}

/**
 * Apply specific file via chezmoi
 *
 * Runs `chezmoi apply <targetFile>` to apply only the specified file.
 * Uses targeted apply (not blanket apply) for safety.
 *
 * @param targetFile Target file path to apply
 * @returns Apply result with success status and output
 *
 * @example
 * const result = await applyChezmoiConfig('~/.config/claude-code/mcp.json');
 * if (result.success) {
 *   console.log('✓ Applied:', result.output);
 * } else {
 *   console.error('✗ Failed:', result.error);
 * }
 */
export async function applyChezmoiConfig(targetFile: string): Promise<ApplyResult> {
  try {
    // Use --force to ignore warnings (like encryption warnings)
    // stderr is used for warnings, not errors, so we filter it
    const { stdout, stderr } = await execAsync(`chezmoi apply --force "${targetFile}"`, {
      timeout: 10000
    });

    // Success if command exits with code 0 (even if stderr has warnings)
    return {
      success: true,
      output: stdout.trim() || 'Applied successfully',
    };
  } catch (error: any) {
    return {
      success: false,
      output: (error.stdout + error.stderr || '').trim(),
      error: error.stderr || error.message,
    };
  }
}

/**
 * Get target file path for agent config
 *
 * @param agentName Agent name
 * @returns Absolute path to target config file
 */
function getTargetFilePath(agentName: string): string {
  const agentConfigMap: Record<string, string> = {
    'claude-code': '.config/claude-code/mcp.json',
    'cursor': '.cursor/mcp.json',
    'cline': '.cline/mcp.json',
    'windsurf': '.codeium/windsurf/mcp.json',
  };

  const configPath = agentConfigMap[agentName];
  if (!configPath) {
    throw new Error(`Unsupported agent: ${agentName}`);
  }

  return path.join(os.homedir(), configPath);
}

/**
 * Automate full chezmoi setup flow with fallback
 *
 * High-level function that orchestrates:
 * 1. Detect chezmoi
 * 2. Write template file
 * 3. Show diff (optional)
 * 4. Apply config
 * 5. Fall back to manual on any error
 *
 * @param config MCP configuration
 * @param agentName Agent name (default: claude-code)
 * @param options Options (showDiff)
 * @returns Setup result indicating automated or manual method
 *
 * @example
 * const result = await automateChezmoiSetup(
 *   config,
 *   'claude-code',
 *   { showDiff: true }
 * );
 *
 * if (result.method === 'automated') {
 *   console.log('✓ Automated setup succeeded');
 * } else {
 *   console.log(`✗ Fell back to manual: ${result.error}`);
 * }
 */
export async function automateChezmoiSetup(
  config: McpConfig,
  agentName: string = 'claude-code',
  options: { showDiff?: boolean } = {}
): Promise<SetupResult> {
  try {
    // 1. Detect chezmoi
    const detection = await detectChezmoi();
    if (!detection.detected) {
      return {
        method: 'manual',
        error: detection.reason || 'chezmoi not detected'
      };
    }

    // 2. Write template file
    await writeChezmoiTemplate(config, detection.sourcePath!, agentName);

    // 3. Show diff if requested
    if (options.showDiff) {
      const targetFile = getTargetFilePath(agentName);
      const diff = await showChezmoiDiff(targetFile);
      console.log('\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
      console.log('Chezmoi Diff Preview');
      console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
      console.log(diff);
      console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n');
    }

    // 4. Apply config
    const targetFile = getTargetFilePath(agentName);
    const result = await applyChezmoiConfig(targetFile);

    if (!result.success) {
      return {
        method: 'manual',
        error: result.error || 'apply failed'
      };
    }

    return {
      method: 'automated',
      result
    };
  } catch (error: any) {
    return {
      method: 'manual',
      error: error.message
    };
  }
}

/**
 * Config Manager - handles Claude Desktop config file detection, reading, and writing
 */

import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import { ClaudeConfig, ConfigError } from './types';

/**
 * Detect Claude Desktop config file location based on platform
 */
export function getConfigPath(): string {
  const platform = os.platform();
  const home = os.homedir();

  if (platform === 'darwin') {
    return path.join(home, 'Library', 'Application Support', 'Claude', 'claude_desktop_config.json');
  } else if (platform === 'linux') {
    return path.join(home, '.config', 'Claude', 'claude_desktop_config.json');
  } else {
    throw new ConfigError(`Unsupported platform: ${platform}. Only macOS and Linux are supported.`);
  }
}

/**
 * Read and parse Claude Desktop config
 */
export function readConfig(): ClaudeConfig {
  const configPath = getConfigPath();

  try {
    if (!fs.existsSync(configPath)) {
      throw new ConfigError(`Claude Desktop config not found at ${configPath}. Is Claude Desktop installed?`);
    }

    const content = fs.readFileSync(configPath, 'utf8');
    const config = JSON.parse(content) as ClaudeConfig;

    return config;
  } catch (error) {
    if (error instanceof ConfigError) {
      throw error;
    }

    if (error instanceof SyntaxError) {
      throw new ConfigError(`Config file is not valid JSON: ${error.message}`);
    }

    if ((error as NodeJS.ErrnoException).code === 'EACCES') {
      throw new ConfigError(`Cannot read config file. Check file permissions for ${configPath}`);
    }

    throw new ConfigError(`Failed to read config: ${(error as Error).message}`);
  }
}

/**
 * Validate config structure
 */
export function validateConfig(config: ClaudeConfig): void {
  try {
    // Ensure it's valid JSON by stringifying and parsing
    const json = JSON.stringify(config);
    JSON.parse(json);

    // Basic structure validation
    if (config.mcpServers !== undefined && typeof config.mcpServers !== 'object') {
      throw new Error('mcpServers must be an object');
    }
  } catch (error) {
    throw new ConfigError(`Invalid config structure: ${(error as Error).message}`);
  }
}

/**
 * Write config atomically to prevent partial writes
 */
export function writeConfig(config: ClaudeConfig): void {
  const configPath = getConfigPath();
  const tempPath = `${configPath}.tmp`;

  try {
    // Validate before writing
    validateConfig(config);

    // Write to temporary file
    const content = JSON.stringify(config, null, 2);
    fs.writeFileSync(tempPath, content, 'utf8');

    // Atomic rename (POSIX atomic operation)
    fs.renameSync(tempPath, configPath);
  } catch (error) {
    // Cleanup temp file on error
    if (fs.existsSync(tempPath)) {
      try {
        fs.unlinkSync(tempPath);
      } catch {
        // Ignore cleanup errors
      }
    }

    if (error instanceof ConfigError) {
      throw error;
    }

    if ((error as NodeJS.ErrnoException).code === 'EACCES') {
      throw new ConfigError(`Cannot write to config directory. Check file permissions for ${configPath}`);
    }

    if ((error as NodeJS.ErrnoException).code === 'ENOSPC') {
      throw new ConfigError('Insufficient disk space. Migration aborted.');
    }

    throw new ConfigError(`Failed to write config: ${(error as Error).message}`);
  }
}

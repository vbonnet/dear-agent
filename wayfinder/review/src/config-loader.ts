/**
 * Configuration loader for multi-persona-review plugin
 */

import { readFile } from 'fs/promises';
import { parse as parseYaml } from 'yaml';
import { join } from 'path';
import type { WayfinderConfig, CrossCheckConfig } from './types.js';

/**
 * Default configuration values
 */
export const DEFAULT_CONFIG: CrossCheckConfig = {
  defaultPersonas: ['security-engineer', 'code-health', 'error-handling-specialist'],
  defaultMode: 'quick',
  personaPaths: [
    '~/.wayfinder/personas',          // User overrides
    '.wayfinder/personas',             // Project-specific
    '{company}/personas',              // Company-level
    '{personas}',                      // Shared Personas plugin (library/ subdirectory)
    '{core}/personas',                 // Legacy fallback
  ],
  costTracking: {
    type: 'stdout',
  },
  options: {
    contextLines: 3,
    autoFix: false,
    deduplicate: true,
    similarityThreshold: 0.8,
    model: 'claude-3-5-sonnet-20241022',
  },
  github: {
    enabled: false,
    changedFilesOnly: true,
    skipDrafts: true,
    concurrency: 3,
    include: ['**/*'],
    exclude: ['**/node_modules/**', '**/dist/**', '**/build/**', '**/*.min.js'],
  },
};

/**
 * Error codes for configuration loading
 */
export const CONFIG_ERROR_CODES = {
  FILE_NOT_FOUND: 'CONFIG_1001',
  PARSE_ERROR: 'CONFIG_1002',
  VALIDATION_ERROR: 'CONFIG_1003',
  INVALID_YAML: 'CONFIG_1004',
} as const;

/**
 * Configuration loading error
 */
export class ConfigError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown
  ) {
    super(message);
    this.name = 'ConfigError';
  }
}

/**
 * Loads configuration from a YAML file
 *
 * @param configPath - Path to the configuration file
 * @returns Parsed configuration
 * @throws ConfigError if file cannot be read or parsed
 */
export async function loadConfig(configPath: string): Promise<WayfinderConfig> {
  try {
    const content = await readFile(configPath, 'utf-8');

    try {
      const config = parseYaml(content) as WayfinderConfig;
      return config;
    } catch (parseError) {
      throw new ConfigError(
        CONFIG_ERROR_CODES.INVALID_YAML,
        `Failed to parse YAML configuration: ${parseError instanceof Error ? parseError.message : String(parseError)}`,
        parseError
      );
    }
  } catch (error) {
    if (error instanceof ConfigError) {
      throw error;
    }

    // Check if it's a file not found error
    if (error && typeof error === 'object' && 'code' in error && error.code === 'ENOENT') {
      throw new ConfigError(
        CONFIG_ERROR_CODES.FILE_NOT_FOUND,
        `Configuration file not found: ${configPath}`,
        error
      );
    }

    throw new ConfigError(
      CONFIG_ERROR_CODES.PARSE_ERROR,
      `Failed to load configuration: ${error instanceof Error ? error.message : String(error)}`,
      error
    );
  }
}

/**
 * Loads multi-persona-review configuration with defaults
 *
 * @param configPath - Optional path to configuration file
 * @returns Multi-persona review configuration with defaults applied
 */
export async function loadCrossCheckConfig(configPath?: string): Promise<CrossCheckConfig> {
  let userConfig: WayfinderConfig | null = null;

  if (configPath) {
    try {
      userConfig = await loadConfig(configPath);
    } catch (error) {
      // If file not found, use defaults
      if (error instanceof ConfigError && error.code === CONFIG_ERROR_CODES.FILE_NOT_FOUND) {
        userConfig = null;
      } else {
        throw error;
      }
    }
  } else {
    // Try to load from default location
    const defaultPath = join(process.cwd(), '.wayfinder', 'config.yml');
    try {
      userConfig = await loadConfig(defaultPath);
    } catch (error) {
      // If file not found, use defaults
      if (error instanceof ConfigError && error.code === CONFIG_ERROR_CODES.FILE_NOT_FOUND) {
        userConfig = null;
      } else {
        throw error;
      }
    }
  }

  // Merge with defaults
  return mergeWithDefaults(userConfig?.crossCheck);
}

/**
 * Merges user configuration with defaults
 *
 * @param userConfig - User's multi-persona-review configuration
 * @returns Merged configuration
 */
export function mergeWithDefaults(userConfig?: CrossCheckConfig): CrossCheckConfig {
  if (!userConfig) {
    return DEFAULT_CONFIG;
  }

  return {
    defaultPersonas: userConfig.defaultPersonas ?? DEFAULT_CONFIG.defaultPersonas,
    defaultMode: userConfig.defaultMode ?? DEFAULT_CONFIG.defaultMode,
    personaPaths: userConfig.personaPaths ?? DEFAULT_CONFIG.personaPaths,
    costTracking: userConfig.costTracking ?? DEFAULT_CONFIG.costTracking,
    options: {
      ...DEFAULT_CONFIG.options,
      ...userConfig.options,
    },
    github: {
      ...DEFAULT_CONFIG.github,
      ...userConfig.github,
    },
  };
}

/**
 * Validates multi-persona-review configuration
 *
 * @param config - Configuration to validate
 * @throws ConfigError if configuration is invalid
 */
export function validateConfig(config: CrossCheckConfig): void {
  // Validate default mode
  if (config.defaultMode && !['quick', 'thorough', 'custom'].includes(config.defaultMode)) {
    throw new ConfigError(
      CONFIG_ERROR_CODES.VALIDATION_ERROR,
      `Invalid default mode: ${config.defaultMode}. Must be 'quick', 'thorough', or 'custom'.`
    );
  }

  // Validate similarity threshold
  if (config.options?.similarityThreshold !== undefined) {
    const threshold = config.options.similarityThreshold;
    if (threshold < 0 || threshold > 1) {
      throw new ConfigError(
        CONFIG_ERROR_CODES.VALIDATION_ERROR,
        `Invalid similarity threshold: ${threshold}. Must be between 0 and 1.`
      );
    }
  }

  // Validate context lines
  if (config.options?.contextLines !== undefined) {
    const lines = config.options.contextLines;
    if (lines < 0 || lines > 50) {
      throw new ConfigError(
        CONFIG_ERROR_CODES.VALIDATION_ERROR,
        `Invalid context lines: ${lines}. Must be between 0 and 50.`
      );
    }
  }

  // Validate cost tracking type
  if (config.costTracking?.type) {
    const validTypes = ['stdout', 'gcp', 'aws', 'datadog', 'webhook', 'file'];
    if (!validTypes.includes(config.costTracking.type)) {
      throw new ConfigError(
        CONFIG_ERROR_CODES.VALIDATION_ERROR,
        `Invalid cost tracking type: ${config.costTracking.type}. Must be one of: ${validTypes.join(', ')}.`
      );
    }
  }

  // Validate GitHub concurrency
  if (config.github?.concurrency !== undefined) {
    const concurrency = config.github.concurrency;
    if (concurrency < 1 || concurrency > 10) {
      throw new ConfigError(
        CONFIG_ERROR_CODES.VALIDATION_ERROR,
        `Invalid GitHub concurrency: ${concurrency}. Must be between 1 and 10.`
      );
    }
  }
}

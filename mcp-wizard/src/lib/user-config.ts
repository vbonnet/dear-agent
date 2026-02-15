/**
 * User Configuration Management
 *
 * Manages user-specific configuration for mcp-wizard, making it company-agnostic.
 * Config stored at ~/.config/mcp-wizard/config.json (XDG-compliant).
 *
 * Priority order: Environment variables > Config file > Defaults
 */

import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

/**
 * User configuration schema
 */
export interface UserConfig {
  company: {
    name: string;           // Company display name (e.g., "Acme Corp", "Example Inc")
    glean_instance: string; // Glean instance ID (e.g., "acme", "example")
    okta_domain: string;    // Okta domain (e.g., "company.okta.com", "example.okta.com")
  };
  globalMcps?: {
    enabled: boolean;         // Enable global MCP discovery
    discoveryUrl?: string;    // Discovery endpoint (e.g., "http://localhost:8001/discovery")
    healthCheckUrl?: string;  // Health check endpoint (e.g., "http://localhost:8001/health")
    temporalUrl?: string;     // Temporal server URL (e.g., "http://localhost:7233")
  };
}

/**
 * Get absolute path to user config file (XDG-compliant)
 *
 * @returns Absolute path to config file (e.g., ~/.config/mcp-wizard/config.json)
 */
export function getUserConfigPath(): string {
  const configHome = process.env.XDG_CONFIG_HOME || path.join(os.homedir(), '.config');
  return path.join(configHome, 'mcp-wizard', 'config.json');
}

/**
 * Find project-level config file by walking up directory tree (like Git)
 *
 * Searches for .mcp-wizard.json starting from cwd, walking up to filesystem root.
 * Stops at first match or when reaching home directory.
 *
 * @param startDir - Directory to start search (defaults to process.cwd())
 * @returns Absolute path to .mcp-wizard.json if found, null otherwise
 */
export function findProjectConfig(startDir: string = process.cwd()): string | null {
  let currentDir = path.resolve(startDir);
  const homeDir = os.homedir();
  const root = path.parse(currentDir).root;

  while (true) {
    const configPath = path.join(currentDir, '.mcp-wizard.json');

    if (fs.existsSync(configPath)) {
      return configPath;
    }

    // Stop at home directory (don't search beyond user's home)
    if (currentDir === homeDir || currentDir === root) {
      return null;
    }

    // Move up one directory
    const parentDir = path.dirname(currentDir);

    // Safety: prevent infinite loop if dirname returns same dir
    if (parentDir === currentDir) {
      return null;
    }

    currentDir = parentDir;
  }
}

/**
 * Get default config - NO DEFAULTS, must be explicitly configured
 *
 * This function intentionally throws an error to ensure users configure their own values.
 * No hard-coded company defaults to prepare for open source release.
 *
 * @throws Error with helpful message directing users to configure
 */
export function getDefaultConfig(): UserConfig {
  throw new Error(
    'No configuration found. Please run: mcp-wizard config init\n' +
    '\n' +
    'Or create .mcp-wizard.json in your project directory:\n' +
    '{\n' +
    '  "company": {\n' +
    '    "name": "Your Company",\n' +
    '    "glean_instance": "yourcompany",\n' +
    '    "okta_domain": "yourcompany.okta.com"\n' +
    '  }\n' +
    '}'
  );
}

/**
 * Load config with hierarchical precedence (env vars > project > user > error)
 *
 * Precedence order:
 * 1. Environment variables (MCP_WIZARD_*)
 * 2. Project config (.mcp-wizard.json in cwd or parent)
 * 3. User config (~/.config/mcp-wizard/config.json)
 * 4. Error if none found
 *
 * @returns User configuration object
 * @throws Error if no config found (no defaults)
 */
export function loadConfig(): UserConfig {
  const userConfigPath = getUserConfigPath();
  const projectConfigPath = findProjectConfig();

  let config: UserConfig | null = null;

  // Try project config first
  if (projectConfigPath) {
    try {
      const content = fs.readFileSync(projectConfigPath, 'utf8');
      config = JSON.parse(content) as UserConfig;
      validateConfig(config);
    } catch (error) {
      if (error instanceof SyntaxError) {
        throw new Error(`Project config (.mcp-wizard.json) has invalid JSON: ${error.message}`);
      }
      throw new Error(`Failed to load project config: ${(error as Error).message}`);
    }
  }

  // Try user config if project config not found
  if (!config && fs.existsSync(userConfigPath)) {
    try {
      const content = fs.readFileSync(userConfigPath, 'utf8');
      config = JSON.parse(content) as UserConfig;
      validateConfig(config);
    } catch (error) {
      if (error instanceof SyntaxError) {
        throw new Error(`User config has invalid JSON: ${error.message}`);
      }
      throw new Error(`Failed to load user config: ${(error as Error).message}`);
    }
  }

  // If no config found, throw helpful error (no defaults)
  if (!config) {
    throw new Error(
      'No configuration found.\n\n' +
      'Run: mcp-wizard config init\n\n' +
      'Or create .mcp-wizard.json in your project:\n' +
      '{\n' +
      '  "company": {\n' +
      '    "name": "Your Company",\n' +
      '    "glean_instance": "yourcompany",\n' +
      '    "okta_domain": "yourcompany.okta.com"\n' +
      '  }\n' +
      '}'
    );
  }

  // Apply environment variable overrides
  return applyEnvOverrides(config);
}

/**
 * Apply environment variable overrides to config
 *
 * Environment variables take precedence over file-based config.
 * Prefix: MCP_WIZARD_
 *
 * Supported env vars:
 * - MCP_WIZARD_COMPANY_NAME
 * - MCP_WIZARD_GLEAN_INSTANCE
 * - MCP_WIZARD_OKTA_DOMAIN
 *
 * @param config - Base configuration object
 * @returns Configuration with env var overrides applied
 */
function applyEnvOverrides(config: UserConfig): UserConfig {
  const result = { ...config };

  // Override company.name
  if (process.env.MCP_WIZARD_COMPANY_NAME) {
    result.company = {
      ...result.company,
      name: process.env.MCP_WIZARD_COMPANY_NAME,
    };
  }

  // Override company.glean_instance
  if (process.env.MCP_WIZARD_GLEAN_INSTANCE) {
    result.company = {
      ...result.company,
      glean_instance: process.env.MCP_WIZARD_GLEAN_INSTANCE,
    };
  }

  // Override company.okta_domain
  if (process.env.MCP_WIZARD_OKTA_DOMAIN) {
    result.company = {
      ...result.company,
      okta_domain: process.env.MCP_WIZARD_OKTA_DOMAIN,
    };
  }

  // Validate final config after env overrides
  validateConfig(result);

  return result;
}

/**
 * Save config to file atomically with validation
 *
 * Uses temp file + rename pattern for atomicity (prevents partial writes)
 *
 * @param config - Configuration object to save
 * @throws Error if validation fails or write fails
 */
export function saveConfig(config: UserConfig): void {
  const configPath = getUserConfigPath();
  const tempPath = `${configPath}.tmp`;

  try {
    // Step 1: Validate before writing (prevent invalid saves)
    validateConfig(config);

    // Step 2: Create directory if missing
    fs.mkdirSync(path.dirname(configPath), { recursive: true });

    // Step 3: Write to temp file
    const content = JSON.stringify(config, null, 2);
    fs.writeFileSync(tempPath, content, 'utf8');

    // Step 4: Atomic rename (POSIX guarantee)
    fs.renameSync(tempPath, configPath);
  } catch (error) {
    // Step 5: Cleanup temp file on error
    if (fs.existsSync(tempPath)) {
      try {
        fs.unlinkSync(tempPath);
      } catch {
        // Ignore cleanup errors
      }
    }

    // Re-throw original error
    throw error;
  }
}

/**
 * Validate config schema, throw error with helpful message if invalid
 *
 * @param config - Configuration object to validate
 * @throws Error if validation fails (includes field name and invalid value)
 */
export function validateConfig(config: UserConfig): void {
  // Check structure
  if (!config.company) {
    throw new Error('Missing required field: company');
  }

  // Validate company.name
  if (typeof config.company.name !== 'string' || !config.company.name.trim()) {
    throw new Error('company.name must be a non-empty string');
  }

  // Validate company.glean_instance
  if (typeof config.company.glean_instance !== 'string') {
    throw new Error('company.glean_instance must be a string');
  }
  if (config.company.glean_instance.includes(' ')) {
    throw new Error(
      `Invalid glean_instance '${config.company.glean_instance}' (no spaces allowed)`
    );
  }
  if (config.company.glean_instance !== config.company.glean_instance.toLowerCase()) {
    throw new Error(
      `Invalid glean_instance '${config.company.glean_instance}' (must be lowercase)`
    );
  }

  // Validate company.okta_domain
  if (typeof config.company.okta_domain !== 'string') {
    throw new Error('company.okta_domain must be a string');
  }
  if (!config.company.okta_domain.includes('.')) {
    throw new Error(
      `Invalid okta_domain '${config.company.okta_domain}' (must be valid domain like company.okta.com)`
    );
  }
}

/**
 * Get config value with environment variable override support
 *
 * Priority order: Environment variables > Config file > Default value
 *
 * @param key - Config key (e.g., "company.glean_instance")
 * @param defaultValue - Default value if not found (optional)
 * @returns Config value or default
 */
export function getConfigValue(key: string, defaultValue: string = ''): string {
  // Priority 1: Environment variable
  const envKey = getEnvVarName(key);
  if (process.env[envKey]) {
    return process.env[envKey]!;
  }

  // Priority 2: Config file
  const config = loadConfig();
  const value = getNestedValue(config, key);
  if (value !== undefined) {
    return value;
  }

  // Priority 3: Default value
  return defaultValue;
}

/**
 * Set config value and save to file
 *
 * @param key - Config key (e.g., "company.glean_instance")
 * @param value - New value
 * @throws Error if validation fails
 */
export function setConfigValue(key: string, value: string): void {
  // Load current config (or create default)
  const config = loadConfig();

  // Update nested key
  setNestedValue(config, key, value);

  // Validate updated config (throws if invalid)
  validateConfig(config);

  // Save to file (atomic write)
  saveConfig(config);
}

/**
 * Convert config key to environment variable name
 *
 * @param configKey - Config key (e.g., "company.glean_instance")
 * @returns Environment variable name (e.g., "MCP_WIZARD_COMPANY_GLEAN_INSTANCE")
 */
function getEnvVarName(configKey: string): string {
  return `MCP_WIZARD_${configKey.toUpperCase().replace(/\./g, '_')}`;
}

/**
 * Get nested value from object using dot notation
 *
 * @param obj - Object to traverse
 * @param path - Dot-separated path (e.g., "company.glean_instance")
 * @returns Value at path, or undefined if not found
 */
function getNestedValue(obj: any, path: string): string | undefined {
  const keys = path.split('.');
  let current = obj;
  for (const key of keys) {
    if (current[key] === undefined) return undefined;
    current = current[key];
  }
  return current;
}

/**
 * Set nested value in object using dot notation
 *
 * @param obj - Object to modify
 * @param path - Dot-separated path (e.g., "company.glean_instance")
 * @param value - Value to set
 */
function setNestedValue(obj: any, path: string, value: string): void {
  const keys = path.split('.');
  let current = obj;

  // Navigate to parent object
  for (let i = 0; i < keys.length - 1; i++) {
    if (!current[keys[i]]) {
      current[keys[i]] = {}; // Create missing parent
    }
    current = current[keys[i]];
  }

  // Set value on final key
  current[keys[keys.length - 1]] = value;
}

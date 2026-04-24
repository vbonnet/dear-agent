/**
 * Tests for configuration loader
 */

import { describe, it, expect } from 'vitest';
import {
  DEFAULT_CONFIG,
  CONFIG_ERROR_CODES,
  ConfigError,
  mergeWithDefaults,
  validateConfig,
} from '../src/config-loader.js';
import type { CrossCheckConfig } from '../src/types.js';

describe('config-loader', () => {
  describe('DEFAULT_CONFIG', () => {
    it('should have sensible defaults', () => {
      expect(DEFAULT_CONFIG.defaultMode).toBe('quick');
      expect(DEFAULT_CONFIG.defaultPersonas).toContain('security-engineer');
      expect(DEFAULT_CONFIG.defaultPersonas).toContain('code-health');
      expect(DEFAULT_CONFIG.options?.deduplicate).toBe(true);
      expect(DEFAULT_CONFIG.options?.similarityThreshold).toBe(0.8);
      expect(DEFAULT_CONFIG.costTracking?.type).toBe('stdout');
    });

    it('should have GitHub configuration disabled by default', () => {
      expect(DEFAULT_CONFIG.github?.enabled).toBe(false);
      expect(DEFAULT_CONFIG.github?.changedFilesOnly).toBe(true);
      expect(DEFAULT_CONFIG.github?.skipDrafts).toBe(true);
    });
  });

  describe('mergeWithDefaults', () => {
    it('should return defaults when no user config provided', () => {
      const config = mergeWithDefaults();
      expect(config).toEqual(DEFAULT_CONFIG);
    });

    it('should merge user config with defaults', () => {
      const userConfig: CrossCheckConfig = {
        defaultMode: 'thorough',
        defaultPersonas: ['security-engineer'],
      };

      const merged = mergeWithDefaults(userConfig);

      expect(merged.defaultMode).toBe('thorough');
      expect(merged.defaultPersonas).toEqual(['security-engineer']);
      expect(merged.options?.deduplicate).toBe(true); // From defaults
      expect(merged.costTracking?.type).toBe('stdout'); // From defaults
    });

    it('should override nested options', () => {
      const userConfig: CrossCheckConfig = {
        options: {
          similarityThreshold: 0.9,
          autoFix: true,
        },
      };

      const merged = mergeWithDefaults(userConfig);

      expect(merged.options?.similarityThreshold).toBe(0.9);
      expect(merged.options?.autoFix).toBe(true);
      expect(merged.options?.deduplicate).toBe(true); // From defaults
      expect(merged.options?.contextLines).toBe(3); // From defaults
    });
  });

  describe('validateConfig', () => {
    it('should accept valid configuration', () => {
      expect(() => validateConfig(DEFAULT_CONFIG)).not.toThrow();
    });

    it('should reject invalid default mode', () => {
      const config: CrossCheckConfig = {
        ...DEFAULT_CONFIG,
        defaultMode: 'invalid' as any,
      };

      expect(() => validateConfig(config)).toThrow(ConfigError);
      expect(() => validateConfig(config)).toThrow(/Invalid default mode/);
    });

    it('should reject invalid similarity threshold', () => {
      const config: CrossCheckConfig = {
        ...DEFAULT_CONFIG,
        options: {
          ...DEFAULT_CONFIG.options,
          similarityThreshold: 1.5,
        },
      };

      expect(() => validateConfig(config)).toThrow(ConfigError);
      expect(() => validateConfig(config)).toThrow(/Invalid similarity threshold/);
    });

    it('should reject negative similarity threshold', () => {
      const config: CrossCheckConfig = {
        ...DEFAULT_CONFIG,
        options: {
          ...DEFAULT_CONFIG.options,
          similarityThreshold: -0.1,
        },
      };

      expect(() => validateConfig(config)).toThrow(ConfigError);
    });

    it('should reject invalid context lines', () => {
      const config: CrossCheckConfig = {
        ...DEFAULT_CONFIG,
        options: {
          ...DEFAULT_CONFIG.options,
          contextLines: 100,
        },
      };

      expect(() => validateConfig(config)).toThrow(ConfigError);
      expect(() => validateConfig(config)).toThrow(/Invalid context lines/);
    });

    it('should reject invalid cost tracking type', () => {
      const config: CrossCheckConfig = {
        ...DEFAULT_CONFIG,
        costTracking: {
          type: 'invalid' as any,
        },
      };

      expect(() => validateConfig(config)).toThrow(ConfigError);
      expect(() => validateConfig(config)).toThrow(/Invalid cost tracking type/);
    });

    it('should reject invalid GitHub concurrency', () => {
      const config: CrossCheckConfig = {
        ...DEFAULT_CONFIG,
        github: {
          ...DEFAULT_CONFIG.github,
          concurrency: 20,
        },
      };

      expect(() => validateConfig(config)).toThrow(ConfigError);
      expect(() => validateConfig(config)).toThrow(/Invalid GitHub concurrency/);
    });

    it('should accept boundary values', () => {
      const config: CrossCheckConfig = {
        ...DEFAULT_CONFIG,
        options: {
          ...DEFAULT_CONFIG.options,
          similarityThreshold: 0,
          contextLines: 0,
        },
        github: {
          ...DEFAULT_CONFIG.github,
          concurrency: 1,
        },
      };

      expect(() => validateConfig(config)).not.toThrow();
    });
  });

  describe('ConfigError', () => {
    it('should create error with code and message', () => {
      const error = new ConfigError(
        CONFIG_ERROR_CODES.VALIDATION_ERROR,
        'Test error'
      );

      expect(error.code).toBe(CONFIG_ERROR_CODES.VALIDATION_ERROR);
      expect(error.message).toBe('Test error');
      expect(error.name).toBe('ConfigError');
    });

    it('should include details if provided', () => {
      const details = { field: 'test' };
      const error = new ConfigError(
        CONFIG_ERROR_CODES.VALIDATION_ERROR,
        'Test error',
        details
      );

      expect(error.details).toBe(details);
    });
  });
});

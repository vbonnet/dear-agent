/**
 * Unit tests for config-manager module
 */

import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import {
  getConfigPath,
  readConfig,
  validateConfig,
  writeConfig,
} from '../../../src/cli/migrate/config-manager';
import { ConfigError } from '../../../src/cli/migrate/types';
import { FIXTURES, createFsError } from './__helpers__/fixtures';

// Mock fs module
jest.mock('fs');
jest.mock('os');

describe('ConfigManager', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('getConfigPath', () => {
    test('should return macOS path on darwin platform', () => {
      (os.platform as jest.Mock).mockReturnValue('darwin');
      (os.homedir as jest.Mock).mockReturnValue('/Users/testuser');

      const configPath = getConfigPath();

      expect(configPath).toBe('/Users/testuser/Library/Application Support/Claude/claude_desktop_config.json');
    });

    test('should return Linux path on linux platform', () => {
      (os.platform as jest.Mock).mockReturnValue('linux');
      (os.homedir as jest.Mock).mockReturnValue('/home/testuser');

      const configPath = getConfigPath();

      expect(configPath).toBe('/home/testuser/.config/Claude/claude_desktop_config.json');
    });

    test('should throw ConfigError on unsupported platform', () => {
      (os.platform as jest.Mock).mockReturnValue('win32');

      expect(() => getConfigPath()).toThrow(ConfigError);
      expect(() => getConfigPath()).toThrow('Unsupported platform: win32');
    });
  });

  describe('readConfig', () => {
    beforeEach(() => {
      (os.platform as jest.Mock).mockReturnValue('darwin');
      (os.homedir as jest.Mock).mockReturnValue('/Users/testuser');
    });

    test('should read and parse valid JSON config', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      const config = readConfig();

      expect(config).toEqual(FIXTURES.validMcpConfig);
      expect(fs.readFileSync).toHaveBeenCalledWith(expect.stringContaining('claude_desktop_config.json'), 'utf8');
    });

    test('should handle missing config file', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);

      expect(() => readConfig()).toThrow(ConfigError);
      expect(() => readConfig()).toThrow('Claude Desktop config not found');
    });

    test('should throw ConfigError on malformed JSON', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(FIXTURES.malformedJson);

      expect(() => readConfig()).toThrow(ConfigError);
      expect(() => readConfig()).toThrow('not valid JSON');
    });

    test('should handle permission denied error', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockImplementation(() => {
        throw createFsError('EACCES', 'Permission denied');
      });

      expect(() => readConfig()).toThrow(ConfigError);
      expect(() => readConfig()).toThrow('Check file permissions');
    });
  });

  describe('validateConfig', () => {
    test('should validate valid config without error', () => {
      expect(() => validateConfig(FIXTURES.validMcpConfig)).not.toThrow();
    });

    test('should validate empty config', () => {
      expect(() => validateConfig(FIXTURES.emptyConfig)).not.toThrow();
    });

    test('should validate config with non-MCP sections', () => {
      expect(() => validateConfig(FIXTURES.configWithNonMcpSections)).not.toThrow();
    });

    test('should throw on invalid mcpServers type', () => {
      const invalidConfig = { mcpServers: 'not an object' } as any;

      expect(() => validateConfig(invalidConfig)).toThrow(ConfigError);
      expect(() => validateConfig(invalidConfig)).toThrow('mcpServers must be an object');
    });
  });

  describe('writeConfig', () => {
    beforeEach(() => {
      (os.platform as jest.Mock).mockReturnValue('darwin');
      (os.homedir as jest.Mock).mockReturnValue('/Users/testuser');
      (fs.existsSync as jest.Mock).mockReturnValue(false);
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {});
      (fs.renameSync as jest.Mock).mockImplementation(() => {});
    });

    test('should write config atomically using temp file', () => {
      writeConfig(FIXTURES.validMcpConfig);

      const expectedPath = '/Users/testuser/Library/Application Support/Claude/claude_desktop_config.json';
      const expectedTempPath = expectedPath + '.tmp';

      expect(fs.writeFileSync).toHaveBeenCalledWith(
        expectedTempPath,
        expect.stringContaining('"mcpServers"'),
        'utf8'
      );
      expect(fs.renameSync).toHaveBeenCalledWith(expectedTempPath, expectedPath);
    });

    test('should format config with 2-space indentation', () => {
      writeConfig(FIXTURES.validMcpConfig);

      const writeCall = (fs.writeFileSync as jest.Mock).mock.calls[0];
      const writtenContent = writeCall[1];

      // Verify JSON formatting
      expect(writtenContent).toContain('  "mcpServers"');
      expect(writtenContent).toContain('    "googledocs"');
    });

    test('should validate config before writing', () => {
      const invalidConfig = { mcpServers: 'invalid' } as any;

      expect(() => writeConfig(invalidConfig)).toThrow(ConfigError);
      expect(fs.writeFileSync).not.toHaveBeenCalled();
    });

    test('should cleanup temp file on write failure', () => {
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {
        throw new Error('Write failed');
      });
      (fs.existsSync as jest.Mock).mockReturnValue(true);

      expect(() => writeConfig(FIXTURES.validMcpConfig)).toThrow(ConfigError);
      expect(fs.unlinkSync).toHaveBeenCalled();
    });

    test('should handle disk full error', () => {
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {
        throw createFsError('ENOSPC', 'No space left on device');
      });

      expect(() => writeConfig(FIXTURES.validMcpConfig)).toThrow(ConfigError);
      expect(() => writeConfig(FIXTURES.validMcpConfig)).toThrow('Insufficient disk space');
    });

    test('should handle permission denied error', () => {
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {
        throw createFsError('EACCES', 'Permission denied');
      });

      expect(() => writeConfig(FIXTURES.validMcpConfig)).toThrow(ConfigError);
      expect(() => writeConfig(FIXTURES.validMcpConfig)).toThrow('Check file permissions');
    });

    test('should preserve non-MCP config sections', () => {
      writeConfig(FIXTURES.configWithNonMcpSections);

      const writeCall = (fs.writeFileSync as jest.Mock).mock.calls[0];
      const writtenContent = writeCall[1];
      const parsed = JSON.parse(writtenContent);

      expect(parsed.otherSettings).toEqual({ theme: 'dark', fontSize: 14 });
      expect(parsed.customData).toBe('preserve this');
    });
  });
});

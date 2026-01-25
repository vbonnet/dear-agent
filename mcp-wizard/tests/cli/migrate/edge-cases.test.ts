/**
 * Edge case and regression tests for migration wizard
 */

import * as fs from 'fs';
import * as os from 'os';
import { readConfig, writeConfig, validateConfig } from '../../../src/cli/migrate/config-manager';
import { FIXTURES, createFsError } from './__helpers__/fixtures';

// Mock modules
jest.mock('fs');
jest.mock('os');

describe('Edge Cases and Regression Tests', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (os.platform as jest.Mock).mockReturnValue('darwin');
    (os.homedir as jest.Mock).mockReturnValue('/Users/testuser');
  });

  describe('config preservation', () => {
    test('should preserve JSON formatting with 2-space indentation', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {});
      (fs.renameSync as jest.Mock).mockImplementation(() => {});

      writeConfig(FIXTURES.validMcpConfig);

      const writeCall = (fs.writeFileSync as jest.Mock).mock.calls[0];
      const content = writeCall[1];

      expect(content).toContain('  "mcpServers"');
      expect(content).toContain('    "googledocs"');
    });

    test('should preserve non-MCP config sections', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {});
      (fs.renameSync as jest.Mock).mockImplementation(() => {});

      writeConfig(FIXTURES.configWithNonMcpSections);

      const writeCall = (fs.writeFileSync as jest.Mock).mock.calls[0];
      const content = writeCall[1];
      const parsed = JSON.parse(content);

      expect(parsed.otherSettings).toEqual({ theme: 'dark', fontSize: 14 });
      expect(parsed.customData).toBe('preserve this');
    });

    test('should handle configs with Unicode characters', () => {
      const unicodeConfig = {
        mcpServers: {
          'test-server': {
            command: 'npx',
            args: ['--emoji', '🚀', '--text', '日本語'],
          },
        },
      };

      expect(() => validateConfig(unicodeConfig)).not.toThrow();
    });
  });

  describe('filesystem edge cases', () => {
    test('should handle disk full error', () => {
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {
        throw createFsError('ENOSPC', 'No space left on device');
      });

      expect(() => writeConfig(FIXTURES.validMcpConfig)).toThrow('Insufficient disk space');
    });

    test('should handle permission denied on config file', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockImplementation(() => {
        throw createFsError('EACCES', 'Permission denied');
      });

      expect(() => readConfig()).toThrow('Check file permissions');
    });

    test('should handle missing config directory', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);

      expect(() => readConfig()).toThrow('Claude Desktop config not found');
    });
  });

  describe('malformed input', () => {
    test('should handle malformed JSON gracefully', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(FIXTURES.malformedJson);

      expect(() => readConfig()).toThrow('not valid JSON');
    });

    test('should validate config schema', () => {
      const invalidConfig = { mcpServers: 'not an object' } as any;

      expect(() => validateConfig(invalidConfig)).toThrow('mcpServers must be an object');
    });

    test('should handle empty string as config', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue('');

      expect(() => readConfig()).toThrow('not valid JSON');
    });
  });

  describe('atomic operations', () => {
    test('should use temp file + rename pattern for atomic writes', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {});
      (fs.renameSync as jest.Mock).mockImplementation(() => {});

      writeConfig(FIXTURES.validMcpConfig);

      expect(fs.writeFileSync).toHaveBeenCalledWith(
        expect.stringContaining('.tmp'),
        expect.any(String),
        'utf8'
      );
      expect(fs.renameSync).toHaveBeenCalledWith(
        expect.stringContaining('.tmp'),
        expect.stringContaining('claude_desktop_config.json')
      );
    });

    test('should cleanup temp file on error', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {
        throw new Error('Write failed');
      });

      expect(() => writeConfig(FIXTURES.validMcpConfig)).toThrow();
      expect(fs.unlinkSync).toHaveBeenCalledWith(expect.stringContaining('.tmp'));
    });

    test('should not leave partial writes on validation failure', () => {
      const invalidConfig = { mcpServers: 'invalid' } as any;

      expect(() => writeConfig(invalidConfig)).toThrow();
      expect(fs.writeFileSync).not.toHaveBeenCalled();
    });
  });

  describe('platform compatibility', () => {
    test('should handle Linux config path', () => {
      (os.platform as jest.Mock).mockReturnValue('linux');
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      const config = readConfig();

      expect(config).toEqual(FIXTURES.validMcpConfig);
      expect(fs.readFileSync).toHaveBeenCalledWith(
        expect.stringContaining('/.config/Claude/'),
        'utf8'
      );
    });

    test('should throw error on unsupported platform', () => {
      (os.platform as jest.Mock).mockReturnValue('win32');

      expect(() => readConfig()).toThrow('Unsupported platform: win32');
    });
  });
});

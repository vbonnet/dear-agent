/**
 * Unit tests for chezmoi-detector module
 */

import { execSync } from 'child_process';
import {
  detectChezmoi,
  generateChezmoiWarning,
} from '../../../src/cli/migrate/chezmoi-detector';

// Mock modules
jest.mock('child_process', () => ({
  execSync: jest.fn(),
}));
jest.mock('../../../src/cli/migrate/config-manager', () => ({
  getConfigPath: jest.fn(() => '/Users/testuser/Library/Application Support/Claude/claude_desktop_config.json'),
}));

describe('ChezmoiDetector', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('generateChezmoiWarning', () => {
    test('should generate warning with config path', () => {
      const warning = generateChezmoiWarning();

      expect(warning).toContain('CHEZMOI DETECTED');
      expect(warning).toContain('claude_desktop_config.json');
    });

    test('should include chezmoi edit instructions', () => {
      const warning = generateChezmoiWarning();

      expect(warning).toContain('chezmoi edit');
      expect(warning).toContain('chezmoi apply');
    });

    test('should warn about overwrite risk', () => {
      const warning = generateChezmoiWarning();

      expect(warning).toContain('overwrite');
    });
  });

  describe('detectChezmoi', () => {
    beforeEach(() => {
      // Reset getConfigPath to normal path for each test
      require('../../../src/cli/migrate/config-manager').getConfigPath = jest.fn(
        () => '/Users/testuser/Library/Application Support/Claude/claude_desktop_config.json'
      );
    });

    test('should detect chezmoi if config path contains .local/share/chezmoi/', () => {
      // Mock getConfigPath to return chezmoi path
      require('../../../src/cli/migrate/config-manager').getConfigPath = jest.fn(
        () => '/Users/testuser/.local/share/chezmoi/Library/Application Support/Claude/claude_desktop_config.json'
      );

      const result = detectChezmoi();

      expect(result.detected).toBe(true);
      expect(result.message).toContain('CHEZMOI DETECTED');
    });

    test('should detect chezmoi if executable exists', () => {
      (execSync as jest.Mock).mockReturnValue(Buffer.from('/usr/local/bin/chezmoi'));

      const result = detectChezmoi();

      expect(result.detected).toBe(true);
      expect(result.message).toContain('CHEZMOI DETECTED');
      expect(execSync).toHaveBeenCalledWith('which chezmoi', { stdio: 'ignore' });
    });

    test('should not detect chezmoi if path normal and executable not found', () => {
      (execSync as jest.Mock).mockImplementation(() => {
        throw new Error('command not found');
      });

      const result = detectChezmoi();

      expect(result.detected).toBe(false);
      expect(result.message).toBeUndefined();
    });

    test('should handle execSync errors gracefully', () => {
      (execSync as jest.Mock).mockImplementation(() => {
        throw new Error('Permission denied');
      });

      const result = detectChezmoi();

      expect(result.detected).toBe(false);
    });

    test('should detect chezmoi if either path or executable indicates chezmoi', () => {
      // Config not in chezmoi path, but executable exists
      (execSync as jest.Mock).mockReturnValue(Buffer.from('/usr/bin/chezmoi'));

      const result = detectChezmoi();

      expect(result.detected).toBe(true);
    });
  });
});

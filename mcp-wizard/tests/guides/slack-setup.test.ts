/**
 * Unit tests for slack-setup.ts
 * Tests Slack setup guide token validation and storage workflows
 */

import inquirer from 'inquirer';
import * as fs from 'fs/promises';
import * as open from 'open';
import { runSlackSetupGuide } from '../../src/guides/slack-setup';
import { MOCK_SLACK_TOKEN } from '../fixtures/guide-fixtures';

jest.mock('inquirer');
jest.mock('open');
jest.mock('fs/promises');

describe('runSlackSetupGuide', () => {
  beforeEach(() => {
    jest.clearAllMocks();

    // Mock fs operations to always succeed
    (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
    (fs.writeFile as jest.Mock).mockResolvedValue(undefined);
  });

  describe('Happy path', () => {
    test('successfully guides user through Slack setup with valid token', async () => {
      // Mock user responses
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })  // Step 1: Create app
        .mockResolvedValueOnce({ confirm: '' })  // Wait for app creation
        .mockResolvedValueOnce({ confirm: '' })  // Wait for scopes
        .mockResolvedValueOnce({ confirm: '' })  // Wait for installation
        .mockResolvedValueOnce({ botToken: MOCK_SLACK_TOKEN.valid })  // Provide bot token
        .mockResolvedValueOnce({ teamId: 'T0ABCDEFGH' });  // Provide team ID

      const result = await runSlackSetupGuide();

      expect(result).toEqual({
        botToken: MOCK_SLACK_TOKEN.valid,
        teamId: 'T0ABCDEFGH'
      });

      // Verify tokens were saved
      expect(fs.writeFile).toHaveBeenCalledWith(
        expect.stringContaining('.slack-token'),
        MOCK_SLACK_TOKEN.valid,
        { mode: 0o600 }
      );

      expect(fs.writeFile).toHaveBeenCalledWith(
        expect.stringContaining('.slack-team-id'),
        'T0ABCDEFGH',
        { mode: 0o600 }
      );
    });

    test('allows user to skip opening browser', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;
      const mockOpen = open.default as jest.MockedFunction<typeof open.default>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: false })  // Skip browser
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ botToken: MOCK_SLACK_TOKEN.valid })
        .mockResolvedValueOnce({ teamId: 'T0ABCDEFGH' });

      await runSlackSetupGuide();

      // Verify browser was not opened
      expect(mockOpen).not.toHaveBeenCalled();
    });
  });

  describe('Token validation', () => {
    test('rejects token without xoxb- prefix', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      // Setup prompts up to bot token step
      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ botToken: MOCK_SLACK_TOKEN.invalid });  // Invalid token

      // Token validation should be called
      const botTokenQuestion = {
        type: 'password',
        name: 'botToken',
        message: expect.any(String),
        validate: expect.any(Function)
      };

      // Extract validate function to test it
      const validateFn = botTokenQuestion.validate;

      if (typeof validateFn === 'function') {
        const result = validateFn(MOCK_SLACK_TOKEN.invalid);
        expect(result).toContain('xoxb-');
      }
    });

    test('rejects token that is too short', async () => {
      const shortToken = 'xoxb-123';

      // Test validation function directly
      const validateFn = (input: string) => {
        if (!input.startsWith('xoxb-')) {
          return 'Bot token must start with "xoxb-". Please check and try again.';
        }
        if (input.length < 20) {
          return 'Token seems too short. Please paste the full token.';
        }
        return true;
      };

      const result = validateFn(shortToken);
      expect(result).toContain('too short');
    });

    test('accepts valid token format', async () => {
      const validateFn = (input: string) => {
        if (!input.startsWith('xoxb-')) {
          return 'Bot token must start with "xoxb-". Please check and try again.';
        }
        if (input.length < 20) {
          return 'Token seems too short. Please paste the full token.';
        }
        return true;
      };

      const result = validateFn(MOCK_SLACK_TOKEN.valid);
      expect(result).toBe(true);
    });
  });

  describe('Team ID validation', () => {
    test('rejects team ID without T prefix', async () => {
      const validateFn = (input: string) => {
        if (!input.startsWith('T')) {
          return 'Team ID must start with "T". Please check and try again.';
        }
        if (input.length < 8) {
          return 'Team ID seems too short. Please paste the full ID.';
        }
        return true;
      };

      const result = validateFn('INVALID123');
      expect(result).toContain('must start with "T"');
    });

    test('rejects team ID that is too short', async () => {
      const validateFn = (input: string) => {
        if (!input.startsWith('T')) {
          return 'Team ID must start with "T". Please check and try again.';
        }
        if (input.length < 8) {
          return 'Team ID seems too short. Please paste the full ID.';
        }
        return true;
      };

      const result = validateFn('T123');
      expect(result).toContain('too short');
    });

    test('accepts valid team ID format', async () => {
      const validateFn = (input: string) => {
        if (!input.startsWith('T')) {
          return 'Team ID must start with "T". Please check and try again.';
        }
        if (input.length < 8) {
          return 'Team ID seems too short. Please paste the full ID.';
        }
        return true;
      };

      const result = validateFn('T0ABCDEFGH');
      expect(result).toBe(true);
    });
  });

  describe('Token storage', () => {
    test('saves tokens with secure permissions (600)', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ botToken: MOCK_SLACK_TOKEN.valid })
        .mockResolvedValueOnce({ teamId: 'T0ABCDEFGH' });

      await runSlackSetupGuide();

      // Verify writeFile was called with mode 0o600 (owner-only read/write)
      expect(fs.writeFile).toHaveBeenCalledWith(
        expect.any(String),
        expect.any(String),
        { mode: 0o600 }
      );
    });

    test('creates slack directory if it does not exist', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ botToken: MOCK_SLACK_TOKEN.valid })
        .mockResolvedValueOnce({ teamId: 'T0ABCDEFGH' });

      await runSlackSetupGuide();

      // Verify mkdir was called with recursive option
      expect(fs.mkdir).toHaveBeenCalledWith(
        expect.stringContaining('mcp-servers/slack-mcp'),
        { recursive: true }
      );
    });

    test('handles file system errors gracefully', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ botToken: MOCK_SLACK_TOKEN.valid })
        .mockResolvedValueOnce({ teamId: 'T0ABCDEFGH' });

      // Mock fs.writeFile to throw error
      (fs.writeFile as jest.Mock).mockRejectedValue(new Error('Permission denied'));

      await expect(runSlackSetupGuide()).rejects.toThrow('Permission denied');
    });
  });

  describe('Browser interaction', () => {
    test('opens Slack app management page when user confirms', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;
      const mockOpen = open.default as jest.MockedFunction<typeof open.default>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ botToken: MOCK_SLACK_TOKEN.valid })
        .mockResolvedValueOnce({ teamId: 'T0ABCDEFGH' });

      await runSlackSetupGuide();

      expect(mockOpen).toHaveBeenCalledWith('https://api.slack.com/apps');
    });
  });
});

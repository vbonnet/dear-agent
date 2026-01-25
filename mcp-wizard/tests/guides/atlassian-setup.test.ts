/**
 * Unit tests for atlassian-setup.ts
 * Tests Atlassian MCP OAuth setup guide workflow
 */

import inquirer from 'inquirer';
import * as open from 'open';
import { runAtlassianSetupGuide } from '../../src/guides/atlassian-setup';
import { SetupError } from '../../src/errors/setup-error';

jest.mock('inquirer');
jest.mock('open');

describe('runAtlassianSetupGuide', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('Happy path', () => {
    test('successfully guides user through Atlassian OAuth setup', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ proceed: true })  // Ready to authenticate
        .mockResolvedValueOnce({ openBrowser: true })  // Open browser
        .mockResolvedValueOnce({ understood: true });  // Continue

      await runAtlassianSetupGuide();

      // Verify all prompts were called
      expect(mockPrompt).toHaveBeenCalledTimes(3);
    });

    test('opens browser for OAuth authentication', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;
      const mockOpen = open.default as jest.MockedFunction<typeof open.default>;

      mockPrompt
        .mockResolvedValueOnce({ proceed: true })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ understood: true });

      await runAtlassianSetupGuide();

      // Verify browser opened with Atlassian auth URL
      expect(mockOpen).toHaveBeenCalledWith('https://mcp.atlassian.com/v1/authorize');
    });

    test('allows user to skip opening browser', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;
      const mockOpen = open.default as jest.MockedFunction<typeof open.default>;

      mockPrompt
        .mockResolvedValueOnce({ proceed: true })
        .mockResolvedValueOnce({ openBrowser: false })
        .mockResolvedValueOnce({ understood: true });

      await runAtlassianSetupGuide();

      // Verify browser was not opened
      expect(mockOpen).not.toHaveBeenCalled();
    });
  });

  describe('User cancellation', () => {
    test('throws SetupError when user declines to proceed', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt.mockResolvedValueOnce({ proceed: false });

      await expect(runAtlassianSetupGuide()).rejects.toThrow(SetupError);
      await expect(runAtlassianSetupGuide()).rejects.toThrow('Atlassian OAuth setup cancelled');
    });

    test('provides guidance in error message when cancelled', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt.mockResolvedValueOnce({ proceed: false });

      try {
        await runAtlassianSetupGuide();
        fail('Should have thrown SetupError');
      } catch (error) {
        expect(error).toBeInstanceOf(SetupError);
        if (error instanceof SetupError) {
          expect(error.guidance).toContain('Re-run setup');
        }
      }
    });
  });

  describe('User interaction', () => {
    test('displays setup header and instructions', async () => {
      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ proceed: true })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ understood: true });

      await runAtlassianSetupGuide();

      // Verify setup header displayed
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Atlassian MCP OAuth Setup'));
      // Verify instructions displayed
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Sign in with your Atlassian account'));

      consoleSpy.mockRestore();
    });

    test('displays manual URL when browser opening declined', async () => {
      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ proceed: true })
        .mockResolvedValueOnce({ openBrowser: false })
        .mockResolvedValueOnce({ understood: true });

      await runAtlassianSetupGuide();

      // Verify manual URL printed
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Manually navigate to:'));
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('https://mcp.atlassian.com/v1/authorize'));

      consoleSpy.mockRestore();
    });

    test('shows completion message', async () => {
      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ proceed: true })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ understood: true });

      await runAtlassianSetupGuide();

      // Verify completion message displayed
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Atlassian MCP configured'));

      consoleSpy.mockRestore();
    });
  });

  describe('OAuth flow explanation', () => {
    test('explains mcp-remote handles OAuth automatically', async () => {
      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ proceed: true })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ understood: true });

      await runAtlassianSetupGuide();

      // Verify explanation about mcp-remote OAuth handling
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('mcp-remote package will automatically handle'));

      consoleSpy.mockRestore();
    });

    test('informs user authentication completes on first use', async () => {
      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ proceed: true })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ understood: true });

      await runAtlassianSetupGuide();

      // Verify message about first-use authentication
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Authentication will complete on first use'));

      consoleSpy.mockRestore();
    });
  });

  describe('Prompt confirmations', () => {
    test('asks for confirmation at each step', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ proceed: true })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ understood: true });

      await runAtlassianSetupGuide();

      // Verify prompt was called for each confirmation
      expect(mockPrompt).toHaveBeenNthCalledWith(1, expect.arrayContaining([
        expect.objectContaining({ name: 'proceed', type: 'confirm' })
      ]));

      expect(mockPrompt).toHaveBeenNthCalledWith(2, expect.arrayContaining([
        expect.objectContaining({ name: 'openBrowser', type: 'confirm' })
      ]));

      expect(mockPrompt).toHaveBeenNthCalledWith(3, expect.arrayContaining([
        expect.objectContaining({ name: 'understood', type: 'confirm' })
      ]));
    });
  });
});

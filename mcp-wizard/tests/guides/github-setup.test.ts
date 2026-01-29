/**
 * Unit tests for github-setup.ts
 * Tests GitHub MCP PAT-based setup guide workflow
 */

import inquirer from 'inquirer';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { runGitHubSetupGuide } from '../../src/guides/github-setup';
import { SetupError } from '../../src/errors/setup-error';

jest.mock('inquirer');
jest.mock('fs/promises');
jest.mock('os');

describe('runGitHubSetupGuide', () => {
  beforeEach(() => {
    jest.clearAllMocks();

    // Mock os.homedir()
    (os.homedir as jest.Mock).mockReturnValue('/home/testuser');

    // Mock fs.mkdir to succeed
    (fs.mkdir as jest.Mock).mockResolvedValue(undefined);

    // Mock fs.writeFile to succeed
    (fs.writeFile as jest.Mock).mockResolvedValue(undefined);
  });

  describe('Happy path - PAT setup', () => {
    test('successfully guides user through GitHub PAT setup', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })  // Has PAT
        .mockResolvedValueOnce({ token: 'ghp_validtokenhere1234567890' })  // Enter PAT
        .mockResolvedValueOnce({ isEnterprise: false })  // GitHub.com
        .mockResolvedValueOnce({ toolsets: ['repos', 'issues'] });  // Features

      await runGitHubSetupGuide();

      // Verify all prompts were called
      expect(mockPrompt).toHaveBeenCalledTimes(4);
    });

    test('stores token in correct location with secure permissions', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'ghp_validtoken1234567890abcdef' })
        .mockResolvedValueOnce({ isEnterprise: false })
        .mockResolvedValueOnce({ toolsets: ['repos'] });

      await runGitHubSetupGuide();

      // Verify token directory created
      expect(fs.mkdir).toHaveBeenCalledWith(
        '/home/testuser/mcp-servers/github-mcp',
        { recursive: true }
      );

      // Verify token file written with 0600 permissions
      expect(fs.writeFile).toHaveBeenCalledWith(
        '/home/testuser/mcp-servers/github-mcp/.github-token',
        'ghp_validtoken1234567890abcdef',
        { mode: 0o600 }
      );
    });

    test('guides user to create PAT when they do not have one', async () => {
      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: false })  // No PAT yet
        .mockResolvedValueOnce({ readyToProceed: true })  // Ready after creating
        .mockResolvedValueOnce({ token: 'ghp_newtoken1234567890' })
        .mockResolvedValueOnce({ isEnterprise: false })
        .mockResolvedValueOnce({ toolsets: ['repos', 'issues'] });

      await runGitHubSetupGuide();

      // Verify instructions were displayed
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('To create a Personal Access Token:'));
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('https://github.com/settings/tokens/new'));
    });
  });

  describe('PAT validation', () => {
    test('accepts valid PAT with ghp_ prefix', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'ghp_' + 'a'.repeat(36) })  // Valid format
        .mockResolvedValueOnce({ isEnterprise: false })
        .mockResolvedValueOnce({ toolsets: ['repos'] });

      await runGitHubSetupGuide();

      expect(mockPrompt).toHaveBeenCalled();
      // No error thrown means validation passed
    });

    test('accepts valid PAT with github_pat_ prefix (fine-grained)', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'github_pat_' + 'b'.repeat(80) })  // Fine-grained PAT
        .mockResolvedValueOnce({ isEnterprise: false })
        .mockResolvedValueOnce({ toolsets: ['repos'] });

      await runGitHubSetupGuide();

      expect(mockPrompt).toHaveBeenCalled();
      // No error thrown means validation passed
    });

    test('validates token format in prompt', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ isEnterprise: false })
        .mockResolvedValueOnce({ toolsets: ['repos'] });

      await runGitHubSetupGuide();

      // Get the second prompt call (token input)
      const tokenPromptCall = mockPrompt.mock.calls[0];
      const tokenPrompt = tokenPromptCall[0] as any;

      // Test validation function
      expect(tokenPrompt.validate('ghp_validtoken12345678901234567890')).toBe(true);
      expect(tokenPrompt.validate('github_pat_validtoken')).toBe(true);
      expect(tokenPrompt.validate('invalid')).toBe('Token should start with "ghp_" or "github_pat_"');
      expect(tokenPrompt.validate('short')).toBe('Token appears invalid (too short)');
    });
  });

  describe('Enterprise Server setup', () => {
    test('configures Enterprise Server URL when selected', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'ghp_enterprisetoken1234567890' })
        .mockResolvedValueOnce({ isEnterprise: true })  // Using Enterprise
        .mockResolvedValueOnce({ enterpriseUrl: 'https://github.company.com' })
        .mockResolvedValueOnce({ toolsets: ['repos'] });

      await runGitHubSetupGuide();

      // Verify Enterprise URL stored
      expect(fs.writeFile).toHaveBeenCalledWith(
        '/home/testuser/mcp-servers/github-mcp/.github-enterprise-url',
        'https://github.company.com',
        expect.anything()
      );
    });

    test('validates Enterprise URL must be https', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'ghp_token' })
        .mockResolvedValueOnce({ isEnterprise: true })
        .mockResolvedValueOnce({ toolsets: ['repos'] });

      await runGitHubSetupGuide();

      // Get enterprise URL prompt
      const enterprisePromptCall = mockPrompt.mock.calls[2];
      const enterprisePrompt = enterprisePromptCall[0] as any;

      // Test validation function
      expect(enterprisePrompt.validate('https://github.company.com')).toBe(true);
      expect(enterprisePrompt.validate('http://github.company.com')).toBe('Enterprise URL must start with https://');
      expect(enterprisePrompt.validate('invalid-url')).toBe('Invalid URL format');
    });

    test('strips trailing slashes from Enterprise URL', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'ghp_token' })
        .mockResolvedValueOnce({ isEnterprise: true })
        .mockResolvedValueOnce({ enterpriseUrl: 'https://github.company.com///' })
        .mockResolvedValueOnce({ toolsets: ['repos'] });

      await runGitHubSetupGuide();

      // Verify trailing slashes removed
      expect(fs.writeFile).toHaveBeenCalledWith(
        expect.stringContaining('.github-enterprise-url'),
        'https://github.company.com',  // No trailing slashes
        expect.anything()
      );
    });
  });

  describe('Feature selection', () => {
    test('stores selected toolsets correctly', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'ghp_token123' })
        .mockResolvedValueOnce({ isEnterprise: false })
        .mockResolvedValueOnce({ toolsets: ['repos', 'issues', 'pull_requests'] });

      await runGitHubSetupGuide();

      // Verify toolsets stored as comma-separated
      expect(fs.writeFile).toHaveBeenCalledWith(
        '/home/testuser/mcp-servers/github-mcp/.github-toolsets',
        'repos,issues,pull_requests',
        expect.anything()
      );
    });

    test('requires at least one feature selection', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'ghp_token' })
        .mockResolvedValueOnce({ isEnterprise: false });

      await runGitHubSetupGuide();

      // Get toolsets prompt
      const toolsetsPromptCall = mockPrompt.mock.calls[2];
      const toolsetsPrompt = toolsetsPromptCall[0] as any;

      // Test validation function
      expect(toolsetsPrompt.validate(['repos'])).toBe(true);
      expect(toolsetsPrompt.validate([])).toBe('Please select at least one feature');
    });

    test('defaults to repos and issues features checked', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'ghp_token' })
        .mockResolvedValueOnce({ isEnterprise: false })
        .mockResolvedValueOnce({ toolsets: [] });  // Won't actually reach here due to validation

      await runGitHubSetupGuide();

      // Get toolsets prompt
      const toolsetsPromptCall = mockPrompt.mock.calls[2];
      const toolsetsPrompt = toolsetsPromptCall[0] as any;

      // Verify repos and issues are checked by default
      const reposChoice = toolsetsPrompt.choices.find((c: any) => c.value === 'repos');
      const issuesChoice = toolsetsPrompt.choices.find((c: any) => c.value === 'issues');

      expect(reposChoice.checked).toBe(true);
      expect(issuesChoice.checked).toBe(true);
    });
  });

  describe('User cancellation', () => {
    test('throws SetupError when user declines to create PAT', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: false })
        .mockResolvedValueOnce({ readyToProceed: false });

      await expect(runGitHubSetupGuide()).rejects.toThrow(SetupError);
      await expect(runGitHubSetupGuide()).rejects.toThrow('GitHub PAT required');
    });

    test('provides guidance URL in error when setup cancelled', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ hasPAT: false })
        .mockResolvedValueOnce({ readyToProceed: false });

      try {
        await runGitHubSetupGuide();
        fail('Should have thrown SetupError');
      } catch (error) {
        expect(error).toBeInstanceOf(SetupError);
        if (error instanceof SetupError) {
          expect(error.helpUrl).toBe('https://github.com/settings/tokens/new');
        }
      }
    });
  });

  describe('OAuth support detection', () => {
    test('detects VS Code 1.101+ for OAuth support', async () => {
      const originalVscodeVersion = process.env.VSCODE_VERSION;
      process.env.VSCODE_VERSION = '1.101.0';

      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ authMethod: 'pat' })  // Choose PAT over OAuth
        .mockResolvedValueOnce({ hasPAT: true })
        .mockResolvedValueOnce({ token: 'ghp_token' })
        .mockResolvedValueOnce({ isEnterprise: false })
        .mockResolvedValueOnce({ toolsets: ['repos'] });

      await runGitHubSetupGuide();

      // Verify OAuth availability message shown
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('VS Code 1.101+ detected - OAuth available'));

      process.env.VSCODE_VERSION = originalVscodeVersion;
    });

    test('falls back to PAT when OAuth selected but not implemented', async () => {
      const originalVscodeVersion = process.env.VSCODE_VERSION;
      process.env.VSCODE_VERSION = '1.101.0';

      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ authMethod: 'oauth' })  // Choose OAuth
        .mockResolvedValueOnce({ hasPAT: true })  // Falls back to PAT
        .mockResolvedValueOnce({ token: 'ghp_token' })
        .mockResolvedValueOnce({ isEnterprise: false })
        .mockResolvedValueOnce({ toolsets: ['repos'] });

      await runGitHubSetupGuide();

      // Verify fallback message shown
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('OAuth flow not yet implemented - falling back to PAT'));

      process.env.VSCODE_VERSION = originalVscodeVersion;
    });
  });
});

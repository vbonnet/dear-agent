/**
 * E2E tests for setup command
 *
 * Tests the complete setup wizard flow including:
 * - OAuth authentication
 * - MCP selection and installation
 * - Configuration file generation
 * - Verification
 */

import { mockPromptSequence, mockPromptError } from '../helpers/prompt-mocker';
import { createGoogleDocsMock } from '../helpers/mock-mcp-server';
import { spawn } from 'child_process';

// Mock dependencies
jest.mock('child_process');
jest.mock('inquirer');

describe('setup command', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('TS1: Setup wizard happy path', () => {
    it('completes setup wizard with GoogleDocs MCP', async () => {
      // Mock setup wizard prompts
      mockPromptSequence([
        { shouldResume: false },                              // Step 1: Resume?
        { selectedAgents: ['claude-code'] },                  // Step 2: Select agents
        { selectedMcps: ['GoogleDocs'] },                     // Step 3: Select MCPs
        { useChezmoiManagement: false },                      // Step 4: Chezmoi?
        { confirmSetup: true },                               // Step 5: Confirm
      ]);

      // Mock MCP server spawning
      const mockSpawn = spawn as jest.MockedFunction<typeof spawn>;
      mockSpawn.mockReturnValue(createGoogleDocsMock() as any);

      // Import setup command after mocks are set up
      const { setupCommand } = await import('../../src/commands/setup');

      // Run setup command
      const result = await setupCommand({});

      // Verify setup completed
      expect(result).toBeDefined();
      expect(mockSpawn).toHaveBeenCalled();
    });

    it('validates MCP server before completing setup', async () => {
      // Mock setup wizard prompts
      mockPromptSequence([
        { shouldResume: false },
        { selectedAgents: ['claude-code'] },
        { selectedMcps: ['GoogleDocs'] },
        { useChezmoiManagement: false },
        { confirmSetup: true },
      ]);

      // Mock MCP server that responds to tools/list
      const mockSpawn = spawn as jest.MockedFunction<typeof spawn>;
      const mockServer = createGoogleDocsMock();
      mockSpawn.mockReturnValue(mockServer as any);

      const { setupCommand } = await import('../../src/commands/setup');

      await setupCommand({});

      // Verify MCP server was spawned for validation
      expect(mockSpawn).toHaveBeenCalledWith(
        expect.any(String),  // command (node)
        expect.arrayContaining([expect.stringContaining('google-docs')]),  // args
        expect.any(Object)   // options
      );
    });
  });

  describe('TS2: User cancellation', () => {
    it('handles user cancellation gracefully', async () => {
      // Mock user cancelling wizard (Ctrl+C)
      mockPromptError('User cancelled');

      const { setupCommand } = await import('../../src/commands/setup');

      // Setup should handle cancellation
      await expect(setupCommand({})).rejects.toThrow('User cancelled');

      // Verify no MCP servers were spawned
      const mockSpawn = spawn as jest.MockedFunction<typeof spawn>;
      expect(mockSpawn).not.toHaveBeenCalled();
    });
  });

  describe('Error handling', () => {
    it('provides clear error if setup fails', async () => {
      mockPromptSequence([
        { shouldResume: false },
        { selectedAgents: ['claude-code'] },
        { selectedMcps: ['GoogleDocs'] },
        { useChezmoiManagement: false },
        { confirmSetup: true },
      ]);

      // Mock MCP spawn failure
      const mockSpawn = spawn as jest.MockedFunction<typeof spawn>;
      mockSpawn.mockImplementation(() => {
        throw new Error('Failed to spawn MCP server');
      });

      const { setupCommand } = await import('../../src/commands/setup');

      await expect(setupCommand({})).rejects.toThrow();
    });
  });
});

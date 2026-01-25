/**
 * Unit tests for install.ts
 * Tests MCP server installation workflows including git clone, npm install, and build steps
 */

import { exec } from 'child_process';
import { promises as fs } from 'fs';
import { installMcpServers } from '../../src/lib/install';
import {
  setupGitCloneSuccess,
  setupGitCloneAuthError,
  setupGitCloneNetworkError,
  setupNpmInstallSuccess,
  setupNpmInstallNetworkError,
  setupNpmBuildSuccess,
  setupNpmBuildFailure,
  setupInstallSequenceSuccess
} from '../helpers/process-helpers';

jest.mock('child_process');
jest.mock('fs', () => ({
  promises: {
    mkdir: jest.fn(),
    access: jest.fn()
  }
}));
jest.mock('../../src/lib/detect');

describe('installMcpServers', () => {
  beforeEach(() => {
    jest.clearAllMocks();

    // Mock fs.mkdir to always succeed
    (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
  });

  describe('Happy path', () => {
    test('installs successfully with full sequence (git clone → npm install → build)', async () => {
      const mockExec = exec as jest.MockedFunction<typeof exec>;
      setupInstallSequenceSuccess();

      // Mock pathExists to return false first (not installed), then true (build succeeded)
      const { pathExists } = require('../../src/lib/detect');
      pathExists
        .mockResolvedValueOnce(false)  // Not installed yet
        .mockResolvedValueOnce(true);  // Build succeeded

      await installMcpServers();

      // Verify git clone was called
      expect(mockExec).toHaveBeenCalledWith(
        expect.stringContaining('git clone'),
        expect.any(Function)
      );

      // Verify npm install was called
      expect(mockExec).toHaveBeenCalledWith(
        'npm install',
        expect.objectContaining({ cwd: expect.stringContaining('mcp-servers/google-docs-mcp') }),
        expect.any(Function)
      );

      // Verify npm run build was called
      expect(mockExec).toHaveBeenCalledWith(
        'npm run build',
        expect.objectContaining({ cwd: expect.stringContaining('mcp-servers/google-docs-mcp') }),
        expect.any(Function)
      );
    });

    test('skips installation if already installed', async () => {
      const mockExec = exec as jest.MockedFunction<typeof exec>;

      // Mock pathExists to return true (already installed)
      const { pathExists } = require('../../src/lib/detect');
      pathExists.mockResolvedValue(true);

      await installMcpServers();

      // Verify no exec calls were made (installation skipped)
      expect(mockExec).not.toHaveBeenCalled();
    });
  });

  describe('Git clone errors', () => {
    test('handles git clone authentication error', async () => {
      setupGitCloneAuthError();

      // Mock pathExists to return false (not installed)
      const { pathExists } = require('../../src/lib/detect');
      pathExists.mockResolvedValue(false);

      // Should throw authentication error
      await expect(installMcpServers()).rejects.toThrow();
    });

    test('retries git clone on network error', async () => {
      const mockExec = exec as jest.MockedFunction<typeof exec>;

      // Mock pathExists to return false (not installed)
      const { pathExists } = require('../../src/lib/detect');
      pathExists.mockResolvedValue(false);

      let callCount = 0;

      mockExec.mockImplementation((command: string, options: any, callback: any) => {
        callCount++;

        if (typeof command === 'string' && command.includes('git clone')) {
          if (callCount === 1) {
            // First attempt: network error
            const error = new Error('Network error') as NodeJS.ErrnoException;
            error.code = 'ENOTFOUND';
            callback(error, '', 'fatal: unable to access repository');
          } else {
            // Second attempt: success
            callback(null, 'Cloning into repository...', '');
          }
        } else if (typeof command === 'string' && command.includes('npm')) {
          callback(null, 'Success', '');
        }

        return {} as any;
      });

      // Configure retryWithBackoff mock to retry once
      const { retryWithBackoff } = require('../../src/lib/errors');
      retryWithBackoff.mockImplementation(async (fn: any) => {
        try {
          return await fn();
        } catch (error) {
          // Retry once
          return await fn();
        }
      });

      pathExists.mockResolvedValue(true);  // Build succeeded

      await installMcpServers();

      // Verify git clone was attempted multiple times
      expect(callCount).toBeGreaterThan(1);
    });
  });

  describe('npm install errors', () => {
    test('handles npm install network error with retry', async () => {
      const mockExec = exec as jest.MockedFunction<typeof exec>;

      // Mock pathExists to return false (not installed)
      const { pathExists } = require('../../src/lib/detect');
      pathExists.mockResolvedValue(false);

      let npmCallCount = 0;

      mockExec.mockImplementation((command: string, options: any, callback: any) => {
        if (typeof command === 'string' && command.includes('git clone')) {
          callback(null, 'Cloning...', '');
        } else if (typeof command === 'string' && command.includes('npm install')) {
          npmCallCount++;

          if (npmCallCount === 1) {
            // First attempt: network error
            const error = new Error('Network timeout') as NodeJS.ErrnoException;
            error.code = 'ETIMEDOUT';
            callback(error, '', 'npm ERR! network request failed');
          } else {
            // Second attempt: success
            callback(null, 'added packages', '');
          }
        } else if (typeof command === 'string' && command.includes('npm run build')) {
          callback(null, 'Build successful', '');
        }

        return {} as any;
      });

      // Configure retryWithBackoff mock
      const { retryWithBackoff } = require('../../src/lib/errors');
      retryWithBackoff.mockImplementation(async (fn: any) => {
        try {
          return await fn();
        } catch (error) {
          return await fn();  // Retry once
        }
      });

      pathExists.mockResolvedValue(true);  // Build succeeded

      await installMcpServers();

      expect(npmCallCount).toBe(2);  // Retried once
    });
  });

  describe('npm build errors', () => {
    test('fails if build produces no output file', async () => {
      const mockExec = exec as jest.MockedFunction<typeof exec>;

      // Mock successful git clone and npm install
      mockExec.mockImplementation((command: string, options: any, callback: any) => {
        if (typeof command === 'string') {
          if (command.includes('git clone') || command.includes('npm install')) {
            callback(null, 'Success', '');
          } else if (command.includes('npm run build')) {
            callback(null, 'Build completed', '');  // Build completes but no output
          }
        }
        return {} as any;
      });

      // Mock pathExists
      const { pathExists } = require('../../src/lib/detect');
      pathExists
        .mockResolvedValueOnce(false)  // Not installed yet
        .mockResolvedValueOnce(false);  // Build output doesn't exist

      // Mock retryWithBackoff to just execute function
      const { retryWithBackoff } = require('../../src/lib/errors');
      retryWithBackoff.mockImplementation(async (fn: any) => await fn());

      await expect(installMcpServers()).rejects.toThrow('MCP build failed');
    });

    test('handles TypeScript compilation errors', async () => {
      setupNpmBuildFailure();

      // Mock pathExists to return false (not installed)
      const { pathExists } = require('../../src/lib/detect');
      pathExists.mockResolvedValue(false);

      // Mock successful git clone and npm install
      const mockExec = exec as jest.MockedFunction<typeof exec>;
      mockExec.mockImplementation((command: string, options: any, callback: any) => {
        if (typeof command === 'string') {
          if (command.includes('git clone')) {
            callback(null, 'Cloning...', '');
          } else if (command.includes('npm install')) {
            callback(null, 'added packages', '');
          } else if (command.includes('npm run build')) {
            const error = new Error('Build failed');
            callback(error, '', 'npm ERR! TypeScript compilation failed');
          }
        }
        return {} as any;
      });

      // Mock retryWithBackoff
      const { retryWithBackoff } = require('../../src/lib/errors');
      retryWithBackoff.mockImplementation(async (fn: any) => await fn());

      await expect(installMcpServers()).rejects.toThrow();
    });
  });

  describe('Directory creation', () => {
    test('creates parent directory recursively', async () => {
      setupInstallSequenceSuccess();

      const { pathExists } = require('../../src/lib/detect');
      pathExists
        .mockResolvedValueOnce(false)  // Not installed
        .mockResolvedValueOnce(true);  // Build succeeded

      // Mock retryWithBackoff
      const { retryWithBackoff } = require('../../src/lib/errors');
      retryWithBackoff.mockImplementation(async (fn: any) => await fn());

      await installMcpServers();

      // Verify mkdir was called with recursive option
      expect(fs.mkdir).toHaveBeenCalledWith(
        expect.stringContaining('mcp-servers'),
        { recursive: true }
      );
    });
  });
});

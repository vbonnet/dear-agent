/**
 * Helper functions for install.ts testing scenarios
 * Provides convenient setup for git clone, npm install, and npm build mocking
 */

import { exec } from 'child_process';

/**
 * Setup successful git clone scenario
 * Configures exec mock to return success for git clone commands
 */
export function setupGitCloneSuccess(): void {
  const mockExec = exec as jest.MockedFunction<typeof exec>;

  mockExec.mockImplementation((command: string, options: any, callback: any) => {
    if (typeof command === 'string' && command.includes('git clone')) {
      callback(null, 'Cloning into repository...\nDone.', '');
    }
    return {} as any;
  });
}

/**
 * Setup git clone authentication error scenario
 * Configures exec mock to return auth failure for git clone
 */
export function setupGitCloneAuthError(): void {
  const mockExec = exec as jest.MockedFunction<typeof exec>;

  mockExec.mockImplementation((command: string, options: any, callback: any) => {
    if (typeof command === 'string' && command.includes('git clone')) {
      const error = new Error('Authentication failed') as NodeJS.ErrnoException;
      error.code = 'EACCES';
      callback(error, '', 'fatal: Authentication failed for repository');
    }
    return {} as any;
  });
}

/**
 * Setup git clone network error scenario
 * Configures exec mock to return network failure for git clone
 */
export function setupGitCloneNetworkError(): void {
  const mockExec = exec as jest.MockedFunction<typeof exec>;

  mockExec.mockImplementation((command: string, options: any, callback: any) => {
    if (typeof command === 'string' && command.includes('git clone')) {
      const error = new Error('Network error') as NodeJS.ErrnoException;
      error.code = 'ENOTFOUND';
      callback(error, '', 'fatal: unable to access repository: Could not resolve host');
    }
    return {} as any;
  });
}

/**
 * Setup successful npm install scenario
 * Configures exec mock to return success for npm install commands
 */
export function setupNpmInstallSuccess(): void {
  const mockExec = exec as jest.MockedFunction<typeof exec>;

  mockExec.mockImplementation((command: string, options: any, callback: any) => {
    if (typeof command === 'string' && command.includes('npm install')) {
      callback(null, 'added 42 packages\nDone in 5s', '');
    }
    return {} as any;
  });
}

/**
 * Setup npm install network error scenario
 * Configures exec mock to return network failure for npm install
 */
export function setupNpmInstallNetworkError(): void {
  const mockExec = exec as jest.MockedFunction<typeof exec>;

  mockExec.mockImplementation((command: string, options: any, callback: any) => {
    if (typeof command === 'string' && command.includes('npm install')) {
      const error = new Error('Network timeout') as NodeJS.ErrnoException;
      error.code = 'ETIMEDOUT';
      callback(error, '', 'npm ERR! network request failed');
    }
    return {} as any;
  });
}

/**
 * Setup successful npm build scenario
 * Configures exec mock to return success for npm build commands
 */
export function setupNpmBuildSuccess(): void {
  const mockExec = exec as jest.MockedFunction<typeof exec>;

  mockExec.mockImplementation((command: string, options: any, callback: any) => {
    if (typeof command === 'string' && command.includes('npm run build')) {
      callback(null, 'Build successful\nDone in 3s', '');
    }
    return {} as any;
  });
}

/**
 * Setup npm build failure scenario
 * Configures exec mock to return build failure
 */
export function setupNpmBuildFailure(): void {
  const mockExec = exec as jest.MockedFunction<typeof exec>;

  mockExec.mockImplementation((command: string, options: any, callback: any) => {
    if (typeof command === 'string' && command.includes('npm run build')) {
      const error = new Error('Build failed');
      callback(error, '', 'npm ERR! TypeScript compilation failed\nsrc/index.ts:42:5 - error TS2322');
    }
    return {} as any;
  });
}

/**
 * Setup successful install sequence (git clone + npm install + npm build)
 * Configures exec mock to return success for all three steps in order
 */
export function setupInstallSequenceSuccess(): void {
  const mockExec = exec as jest.MockedFunction<typeof exec>;

  let callCount = 0;

  mockExec.mockImplementation((command: string, options: any, callback: any) => {
    callCount++;

    if (typeof command === 'string') {
      if (command.includes('git clone')) {
        callback(null, 'Cloning into repository...\nDone.', '');
      } else if (command.includes('npm install')) {
        callback(null, 'added 42 packages\nDone in 5s', '');
      } else if (command.includes('npm run build')) {
        callback(null, 'Build successful\nDone in 3s', '');
      }
    }

    return {} as any;
  });
}

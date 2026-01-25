/**
 * Mock child_process module for testing
 * Uses jest.fn() to support .mockImplementation(), .toHaveBeenCalledWith(), etc.
 */

// Default implementations for process execution mocks
const execImpl = (
  command: string,
  callback: (error: Error | null, stdout: string, stderr: string) => void
): void => {
  // Default: process runs successfully
  callback(null, 'process output', '');
};

const spawnImpl = (): any => {
  // Default: return mock ChildProcess-like object
  return {
    on: jest.fn(),
    stdout: { on: jest.fn() },
    stderr: { on: jest.fn() },
    kill: jest.fn(),
  };
};

// Export mocks with default implementations
export const exec = jest.fn(execImpl);
export const spawn = jest.fn(spawnImpl);

/**
 * Test helper: Clear all mock data and reset call history
 * Call this in beforeEach() to ensure test isolation
 */
export function __clearProcessMocks(): void {
  exec.mockClear();
  spawn.mockClear();

  // Restore default implementations
  exec.mockImplementation(execImpl);
  spawn.mockImplementation(spawnImpl);
}

/**
 * Test helper: Setup process running scenario
 * Process executes successfully with exit code 0
 */
export function __setupProcessRunning(): void {
  exec.mockImplementation((command, callback) => {
    callback(null, 'process running', '');
  });
}

/**
 * Test helper: Setup process not found scenario
 * Process fails with exit code 127 (command not found)
 */
export function __setupProcessNotFound(): void {
  exec.mockImplementation((command, callback) => {
    const error = new Error('Command not found') as NodeJS.ErrnoException;
    error.code = 'ENOENT';
    callback(error, '', 'command not found');
  });
}

/**
 * Test helper: Setup process error scenario
 * Process fails with exit code 1 (general error)
 */
export function __setupProcessError(): void {
  exec.mockImplementation((command, callback) => {
    const error = new Error('Process failed');
    callback(error, '', 'process error');
  });
}

/**
 * Test helper: Setup process timeout scenario
 */
export function __setupProcessTimeout(): void {
  exec.mockImplementation((command, callback) => {
    const error = new Error('Process timeout') as NodeJS.ErrnoException;
    error.code = 'ETIMEDOUT';
    callback(error, '', 'timeout');
  });
}

/**
 * Test helper: Setup process degraded scenario
 * Process is running but experiencing issues (slow, warnings)
 */
export function __setupProcessDegraded(): void {
  exec.mockImplementation((command, callback) => {
    // Process runs but with warnings (degraded state)
    callback(null, 'process running with warnings', 'warning: slow response');
  });
}

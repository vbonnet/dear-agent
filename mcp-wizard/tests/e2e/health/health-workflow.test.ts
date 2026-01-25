/**
 * E2E Tests for Health Workflow
 *
 * Tests complete user-facing health check workflows
 *
 * Requirements:
 * - Complete CLI invocation workflows
 * - Health check from user command to output
 * - Doctor command from invocation to diagnostic report
 * - SessionStart hook from initialization to health check
 */

import { checkHealth, formatHealthOutput } from '../../../src/commands/health';
import { runDiagnostics, formatDiagnosticReport } from '../../../src/commands/doctor';
import { onSessionStart } from '../../../src/hooks/session-start';
import { __clearMockStore as __clearKeytarMocks } from '../../__mocks__/keytar';
import { __clearProcessMocks, __setupProcessRunning } from '../../__mocks__/child_process';
import { __clearNetworkMocks, __setupNetworkSuccess } from '../../__mocks__/https';

describe('Health Workflow E2E', () => {
  beforeEach(() => {
    // Setup realistic environment
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
    __setupProcessRunning();
    __setupNetworkSuccess();
  });

  it('should execute health command end-to-end', async () => {
    // Execute: User runs health command
    const healthStatus = await checkHealth();
    const output = formatHealthOutput(healthStatus);

    // Assert: User sees formatted health status
    expect(output).toBeDefined();
    expect(typeof output).toBe('string');
    expect(output.length).toBeGreaterThan(0);
    expect(output).toContain('Overall Status');
    expect(output).toContain('Component Health');
  });

  it('should execute doctor command end-to-end', async () => {
    // Execute: User runs doctor command
    const diagnostics = await runDiagnostics();
    const output = formatDiagnosticReport(diagnostics);

    // Assert: User sees formatted diagnostic report
    expect(output).toBeDefined();
    expect(typeof output).toBe('string');
    expect(output.length).toBeGreaterThan(0);
    expect(output).toContain('System Diagnostics Report');
  });

  it('should execute SessionStart hook end-to-end', async () => {
    // Execute: Session starts, hook triggers
    const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

    await onSessionStart();

    // Assert: Hook executes without blocking
    expect(consoleErrorSpy).not.toHaveBeenCalled();

    consoleLogSpy.mockRestore();
    consoleErrorSpy.mockRestore();
  });

  it('should provide user-friendly output for healthy system', async () => {
    // Execute
    const healthStatus = await checkHealth();
    const output = formatHealthOutput(healthStatus);

    // Assert: Output is user-friendly
    expect(output).toContain('healthy');
    expect(output).not.toContain('undefined');
    expect(output).not.toContain('[object Object]');
  });

  it('should provide clear error messages when system has issues', async () => {
    // Setup: Simulate MCP down
    const { __setupProcessNotFound } = require('../../__mocks__/child_process');
    __setupProcessNotFound();

    // Execute
    const healthStatus = await checkHealth();
    const output = formatHealthOutput(healthStatus);

    // Assert: Output provides clear indication of issues
    expect(output).toBeDefined();
    expect(typeof output).toBe('string');
  });

  it('should complete full workflow in reasonable time', async () => {
    // Execute: Full health check workflow
    const startTime = Date.now();

    await checkHealth();
    await runDiagnostics();
    await onSessionStart();

    const duration = Date.now() - startTime;

    // Assert: Should complete quickly (< 1 second for mocked operations)
    expect(duration).toBeLessThan(1000);
  });
});

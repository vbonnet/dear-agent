/**
 * Integration tests for Claude Code VertexAI credential auto-detection
 * Tests the auto-detection logic added in cli.ts
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

describe('Claude Code VertexAI Auto-Detection', () => {
  const originalEnv = process.env;

  beforeEach(() => {
    // Reset environment to clean state
    process.env = { ...originalEnv };
    delete process.env.CLAUDE_CODE_USE_VERTEX;
    delete process.env.ANTHROPIC_VERTEX_PROJECT_ID;
    delete process.env.CLOUD_ML_REGION;
    delete process.env.VERTEX_PROJECT_ID;
    delete process.env.VERTEX_LOCATION;
    delete process.env.ANTHROPIC_API_KEY;
  });

  afterEach(() => {
    // Restore original environment
    process.env = originalEnv;
  });

  /**
   * Test Scenario 1: Auto-detection works
   * When Claude Code env vars are set, provider should auto-select to vertexai-claude
   */
  it('auto-detects Claude Code VertexAI session', async () => {
    process.env.CLAUDE_CODE_USE_VERTEX = '1';
    process.env.ANTHROPIC_VERTEX_PROJECT_ID = 'test-project';
    process.env.CLOUD_ML_REGION = 'us-east5';

    // Import CLI module (dynamic to pick up env vars)
    const { detectProvider } = await import('../../src/cli.js');

    const result = detectProvider({}, false);

    // Expected: provider auto-selected to vertexai-claude
    expect(result.provider).toBe('vertexai-claude');
    expect(result.vertexProject).toBe('test-project');
    expect(result.vertexLocation).toBe('us-east5');
  });

  /**
   * Test Scenario 2: Explicit flag override
   * Explicit --provider flag should override auto-detection
   */
  it('explicit provider flag overrides auto-detection', async () => {
    process.env.CLAUDE_CODE_USE_VERTEX = '1';
    process.env.ANTHROPIC_VERTEX_PROJECT_ID = 'claude-project';
    process.env.ANTHROPIC_API_KEY = 'sk-ant-test';

    const { detectProvider } = await import('../../src/cli.js');

    const result = detectProvider({
      provider: 'anthropic',
      apiKey: 'sk-ant-456',
    }, false);

    // Expected: explicit flags win
    expect(result.provider).toBe('anthropic');
  });

  /**
   * Test Scenario 3: Backward compatibility with standard env vars
   * Standard VERTEX_PROJECT_ID should still work
   */
  it('maintains backward compatibility with standard env vars', async () => {
    process.env.VERTEX_PROJECT_ID = 'standard-project';
    process.env.VERTEX_LOCATION = 'us-central1';

    const { detectProvider } = await import('../../src/cli.js');

    const result = detectProvider({
      provider: 'vertexai-claude',
    }, false);

    // Expected: standard env vars used (passthrough when provider explicitly set)
    expect(result.provider).toBe('vertexai-claude');
  });

  /**
   * Test Scenario 4: Environment variable fallback chain
   * Claude Code vars should be used as fallback when standard vars not set
   */
  it('uses Claude Code vars as fallback in provider initialization', async () => {
    process.env.ANTHROPIC_VERTEX_PROJECT_ID = 'claude-project';
    process.env.CLOUD_ML_REGION = 'europe-west1';
    process.env.CLAUDE_CODE_USE_VERTEX = '1';
    // No VERTEX_PROJECT_ID or VERTEX_LOCATION set

    const { detectProvider } = await import('../../src/cli.js');

    const result = detectProvider({}, false);

    // Expected: Claude Code vars detected and used
    expect(result.provider).toBe('vertexai-claude');
    expect(result.vertexProject).toBe('claude-project');
    expect(result.vertexLocation).toBe('europe-west1');
  });

  /**
   * Test Scenario 5: Missing Claude Code flag edge case
   * If CLAUDE_CODE_USE_VERTEX not set, auto-detection should skip
   */
  it('skips auto-detection when CLAUDE_CODE_USE_VERTEX not set', async () => {
    // Only project ID set, no USE_VERTEX flag
    process.env.ANTHROPIC_VERTEX_PROJECT_ID = 'test-project';
    process.env.ANTHROPIC_API_KEY = 'sk-ant-test';

    const { detectProvider } = await import('../../src/cli.js');

    const result = detectProvider({}, false);

    // Expected: defaults to anthropic (no auto-detection)
    expect(result.provider).toBe('anthropic');
  });

  /**
   * Test Scenario 6: Verbose logging verification
   * When verbose=true, detection messages should be logged
   */
  it('logs detection messages when verbose flag used', async () => {
    process.env.CLAUDE_CODE_USE_VERTEX = '1';
    process.env.ANTHROPIC_VERTEX_PROJECT_ID = 'test-project';
    process.env.CLOUD_ML_REGION = 'us-east5';

    // Capture console.error output
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation();

    const { detectProvider } = await import('../../src/cli.js');

    detectProvider({}, true); // verbose=true

    // Expected: verbose messages logged
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      expect.stringContaining('Detected Claude Code VertexAI session')
    );
    expect(consoleErrorSpy).toHaveBeenCalledWith(expect.stringContaining('Project: test-project'));
    expect(consoleErrorSpy).toHaveBeenCalledWith(expect.stringContaining('Region: us-east5'));
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      expect.stringContaining('Auto-selecting provider: vertexai-claude')
    );

    consoleErrorSpy.mockRestore();
  });

  /**
   * Edge Case: CLAUDE_CODE_USE_VERTEX set to wrong value
   * Only '1' should trigger auto-detection
   */
  it('only activates when CLAUDE_CODE_USE_VERTEX equals "1"', async () => {
    process.env.CLAUDE_CODE_USE_VERTEX = 'true'; // Wrong value
    process.env.ANTHROPIC_VERTEX_PROJECT_ID = 'test-project';
    process.env.ANTHROPIC_API_KEY = 'sk-ant-test';

    const { detectProvider } = await import('../../src/cli.js');

    const result = detectProvider({}, false);

    // Expected: no auto-detection (wrong flag value)
    expect(result.provider).toBe('anthropic');
  });

  /**
   * Edge Case: Missing project ID
   * Auto-detection requires both flag AND project ID
   */
  it('skips auto-detection when project ID missing', async () => {
    process.env.CLAUDE_CODE_USE_VERTEX = '1';
    // No ANTHROPIC_VERTEX_PROJECT_ID set
    process.env.ANTHROPIC_API_KEY = 'sk-ant-test';

    const { detectProvider } = await import('../../src/cli.js');

    const result = detectProvider({}, false);

    // Expected: no auto-detection (missing project ID)
    expect(result.provider).toBe('anthropic');
  });

  /**
   * Edge Case: Empty string env vars
   * Empty strings should be treated as missing
   */
  it('treats empty string env vars as missing', async () => {
    process.env.CLAUDE_CODE_USE_VERTEX = '1';
    process.env.ANTHROPIC_VERTEX_PROJECT_ID = ''; // Empty string
    process.env.ANTHROPIC_API_KEY = 'sk-ant-test';

    const { detectProvider } = await import('../../src/cli.js');

    const result = detectProvider({}, false);

    // Expected: empty string treated as missing, no auto-detection
    expect(result.provider).toBe('anthropic');
  });
});

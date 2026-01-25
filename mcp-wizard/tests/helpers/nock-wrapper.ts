/**
 * Nock wrapper for HTTP recording/replay in tests
 *
 * Provides automatic credential scrubbing and mode switching between
 * record (real HTTP calls) and replay (fixture playback) modes.
 *
 * @example
 * ```typescript
 * await withNock('setup-oauth-success', async () => {
 *   await setupCommand({});
 *   expect(someMock).toHaveBeenCalled();
 * });
 * ```
 */

import * as nock from 'nock';
import * as fs from 'fs';
import * as path from 'path';

/**
 * Check if running in record mode
 * Set NOCK_RECORD=true to record real HTTP interactions
 */
function isRecordMode(): boolean {
  return process.env.NOCK_RECORD === 'true';
}

/**
 * Get path to fixture file
 */
function getFixturePath(fixtureName: string): string {
  return path.join(__dirname, '..', '__fixtures__', 'http', `${fixtureName}.json`);
}

/**
 * Scrub credentials from HTTP interactions
 * Replaces access_token, refresh_token, client_secret with REDACTED
 */
function scrubCredentials(interactions: any[]): any[] {
  const scrubbed = JSON.parse(JSON.stringify(interactions));

  scrubbed.forEach((interaction: any) => {
    // Scrub request headers
    if (interaction.scope && interaction.scope.includes('Authorization')) {
      interaction.scope = interaction.scope.replace(/Bearer\s+[^\s]+/, 'Bearer REDACTED');
    }
    if (interaction.reqheaders && interaction.reqheaders.Authorization) {
      interaction.reqheaders.Authorization = 'Bearer REDACTED';
    }
    if (interaction.reqheaders && interaction.reqheaders.authorization) {
      interaction.reqheaders.authorization = 'Bearer REDACTED';
    }

    // Scrub request body
    if (interaction.body) {
      const bodyStr = typeof interaction.body === 'string'
        ? interaction.body
        : JSON.stringify(interaction.body);

      const scrubbed = bodyStr
        .replace(/"access_token"\s*:\s*"[^"]+"/g, '"access_token":"REDACTED"')
        .replace(/"refresh_token"\s*:\s*"[^"]+"/g, '"refresh_token":"REDACTED"')
        .replace(/"client_secret"\s*:\s*"[^"]+"/g, '"client_secret":"REDACTED"')
        .replace(/"code"\s*:\s*"[^"]+"/g, '"code":"REDACTED"')
        .replace(/"device_code"\s*:\s*"[^"]+"/g, '"device_code":"REDACTED"');

      try {
        interaction.body = JSON.parse(scrubbed);
      } catch {
        interaction.body = scrubbed;
      }
    }

    // Scrub response body
    if (interaction.response) {
      const responseStr = typeof interaction.response === 'string'
        ? interaction.response
        : JSON.stringify(interaction.response);

      const scrubbed = responseStr
        .replace(/"access_token"\s*:\s*"[^"]+"/g, '"access_token":"REDACTED"')
        .replace(/"refresh_token"\s*:\s*"[^"]+"/g, '"refresh_token":"REDACTED"')
        .replace(/"client_secret"\s*:\s*"[^"]+"/g, '"client_secret":"REDACTED"')
        .replace(/"id_token"\s*:\s*"[^"]+"/g, '"id_token":"REDACTED"');

      try {
        interaction.response = JSON.parse(scrubbed);
      } catch {
        interaction.response = scrubbed;
      }
    }
  });

  return scrubbed;
}

/**
 * Validate that fixture has been scrubbed
 * Throws error if real credentials found
 */
function validateFixtureScrubbed(fixturePath: string, interactions: any[]): void {
  const fixtureStr = JSON.stringify(interactions);

  // Allow REDACTED tokens (already scrubbed)
  const scrubbedPattern = /"(access_token|refresh_token|client_secret|id_token|code|device_code)"\s*:\s*"REDACTED"/g;
  const cleanStr = fixtureStr.replace(scrubbedPattern, '');

  // Check for real tokens (long base64-like strings in token fields)
  const suspiciousPatterns = [
    /"access_token"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,
    /"refresh_token"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,
    /"client_secret"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,
    /"id_token"\s*:\s*"[a-zA-Z0-9._-]{100,}"/,  // JWTs are typically longer
  ];

  for (const pattern of suspiciousPatterns) {
    if (pattern.test(cleanStr)) {
      throw new Error(
        `Credentials found in fixture: ${fixturePath}\n` +
        `Fix: Credentials should be REDACTED automatically, but validation failed.\n` +
        `Hint: Check scrubCredentials() function for missing patterns.`
      );
    }
  }
}

/**
 * Save nock recordings to fixture file with auto-scrubbing
 */
function saveFixture(fixturePath: string): void {
  const recordings = nock.recorder.play();

  if (recordings.length === 0) {
    console.warn(`Warning: No HTTP calls recorded for fixture: ${path.basename(fixturePath)}`);
    return;
  }

  // Parse recordings into nock definition format
  const interactions = recordings.map((rec: string) => {
    // nock.recorder.play() returns strings like "nock('https://...')"
    // We need to convert to JSON format for fixture storage
    try {
      // This is a simplified parser - in production, use nock.recorder.play() with outputObjects: true
      return eval('(' + rec + ')');
    } catch {
      return rec;
    }
  });

  const scrubbedInteractions = scrubCredentials(interactions);
  validateFixtureScrubbed(fixturePath, scrubbedInteractions);

  // Ensure directory exists
  const fixtureDir = path.dirname(fixturePath);
  if (!fs.existsSync(fixtureDir)) {
    fs.mkdirSync(fixtureDir, { recursive: true });
  }

  fs.writeFileSync(fixturePath, JSON.stringify(scrubbedInteractions, null, 2));
  console.log(`✅ Fixture saved and scrubbed: ${path.basename(fixturePath)}`);
}

/**
 * Load nock fixtures from file
 */
function loadFixture(fixturePath: string): void {
  if (!fs.existsSync(fixturePath)) {
    throw new Error(
      `Fixture not found: ${fixturePath}\n` +
      `Hint: Run 'NOCK_RECORD=true npm test' to create fixture`
    );
  }

  const fixtureContent = fs.readFileSync(fixturePath, 'utf-8');
  const interactions = JSON.parse(fixtureContent);

  // Load nock definitions from fixture
  nock.define(interactions);
}

/**
 * Wrapper for nock record/replay mode
 *
 * @param fixtureName - Name of fixture file (without .json extension)
 * @param testFn - Async test function to run with nock active
 *
 * @example
 * ```typescript
 * // Replay mode (default): Uses fixture
 * await withNock('setup-oauth-success', async () => {
 *   const result = await fetch('https://oauth2.googleapis.com/token');
 *   expect(result.access_token).toBe('REDACTED');
 * });
 *
 * // Record mode: Records real HTTP calls
 * // NOCK_RECORD=true npm test
 * await withNock('setup-oauth-success', async () => {
 *   const result = await fetch('https://oauth2.googleapis.com/token');
 *   // Real OAuth call, saved to fixture with scrubbed tokens
 * });
 * ```
 */
export async function withNock(
  fixtureName: string,
  testFn: () => Promise<void>
): Promise<void> {
  const fixturePath = getFixturePath(fixtureName);

  if (isRecordMode()) {
    // Record mode: Allow real HTTP calls and save to fixture
    nock.recorder.rec({
      output_objects: true,
      dont_print: true,
    });

    try {
      await testFn();
    } finally {
      nock.recorder.stop();
      saveFixture(fixturePath);
      nock.cleanAll();
    }
  } else {
    // Replay mode: Load fixture and block real HTTP calls
    nock.disableNetConnect();
    loadFixture(fixturePath);

    try {
      await testFn();

      // Verify all nock interceptors were used
      if (!nock.isDone()) {
        const pending = nock.pendingMocks();
        throw new Error(
          `Nock interceptors not called:\n${pending.join('\n')}\n` +
          `Hint: Test didn't make expected HTTP calls`
        );
      }
    } finally {
      nock.cleanAll();
      nock.enableNetConnect();
    }
  }
}

/**
 * Export scrubbing function for use in pre-commit hooks
 */
export { scrubCredentials, validateFixtureScrubbed };

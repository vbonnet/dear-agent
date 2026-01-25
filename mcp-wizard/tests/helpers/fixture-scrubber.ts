/**
 * Fixture credential scrubber - standalone utility
 *
 * Validates that test fixtures have been properly scrubbed of credentials.
 * Used by pre-commit hooks to prevent credential leaks.
 *
 * @example
 * ```bash
 * # Pre-commit hook
 * node tests/helpers/fixture-scrubber.ts tests/__fixtures__/http/*.json
 * ```
 */

import * as fs from 'fs';
import * as path from 'path';

/**
 * Patterns that indicate credentials (should be REDACTED)
 */
const CREDENTIAL_PATTERNS = [
  // OAuth tokens (long base64-like strings)
  /"access_token"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,
  /"refresh_token"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,
  /"client_secret"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,
  /"id_token"\s*:\s*"[a-zA-Z0-9._-]{100,}"/,  // JWTs are typically longer
  /"code"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,
  /"device_code"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,

  // API keys
  /"api_key"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,
  /"apiKey"\s*:\s*"[a-zA-Z0-9._-]{20,}"/,

  // Bearer tokens in headers
  /Bearer\s+[a-zA-Z0-9._-]{20,}/,
];

/**
 * Whitelist patterns (legitimate values that match credential patterns)
 */
const WHITELIST_PATTERNS = [
  /"(access_token|refresh_token|client_secret|id_token|code|device_code)"\s*:\s*"REDACTED"/,
  /Bearer\s+REDACTED/,
  /"(api_key|apiKey)"\s*:\s*"test-[^"]+"/,  // Test API keys
];

/**
 * Validate that a fixture file has been scrubbed
 *
 * @param filePath - Path to fixture file
 * @returns true if scrubbed, false if credentials found
 */
export function validateFixtureScrubbed(filePath: string): boolean {
  if (!fs.existsSync(filePath)) {
    console.error(`❌ Fixture not found: ${filePath}`);
    return false;
  }

  const content = fs.readFileSync(filePath, 'utf-8');

  // Remove whitelisted patterns
  let cleanContent = content;
  for (const whitelist of WHITELIST_PATTERNS) {
    cleanContent = cleanContent.replace(whitelist, '');
  }

  // Check for suspicious credential patterns
  for (const pattern of CREDENTIAL_PATTERNS) {
    if (pattern.test(cleanContent)) {
      console.error(`❌ Credentials found in fixture: ${path.basename(filePath)}`);
      console.error(`   Pattern: ${pattern}`);
      console.error(`   Fix: Run 'npm run scrub-fixtures' or manually replace with REDACTED`);
      return false;
    }
  }

  return true;
}

/**
 * Validate all fixtures in a directory
 *
 * @param directoryPath - Path to directory containing fixtures
 * @returns true if all scrubbed, false if any have credentials
 */
export function validateDirectoryScrubbed(directoryPath: string): boolean {
  if (!fs.existsSync(directoryPath)) {
    console.warn(`⚠️  Directory not found: ${directoryPath}`);
    return true;  // Not an error if directory doesn't exist
  }

  const files = fs.readdirSync(directoryPath)
    .filter(file => file.endsWith('.json'))
    .map(file => path.join(directoryPath, file));

  if (files.length === 0) {
    console.log(`ℹ️  No fixtures found in ${directoryPath}`);
    return true;
  }

  console.log(`🔍 Validating ${files.length} fixtures in ${path.basename(directoryPath)}...`);

  let allValid = true;
  for (const file of files) {
    const isValid = validateFixtureScrubbed(file);
    if (!isValid) {
      allValid = false;
    }
  }

  if (allValid) {
    console.log(`✅ All fixtures scrubbed (${files.length} files)`);
  }

  return allValid;
}

/**
 * CLI entry point
 * Usage: node fixture-scrubber.ts <file1.json> <file2.json> ...
 */
if (require.main === module) {
  const args = process.argv.slice(2);

  if (args.length === 0) {
    console.error('Usage: node fixture-scrubber.ts <fixture-files...>');
    console.error('Example: node fixture-scrubber.ts tests/__fixtures__/http/*.json');
    process.exit(1);
  }

  let allValid = true;

  for (const arg of args) {
    // Check if arg is a directory or file
    if (fs.existsSync(arg) && fs.statSync(arg).isDirectory()) {
      if (!validateDirectoryScrubbed(arg)) {
        allValid = false;
      }
    } else if (arg.endsWith('.json')) {
      if (!validateFixtureScrubbed(arg)) {
        allValid = false;
      }
    } else {
      console.warn(`⚠️  Skipping non-JSON file: ${arg}`);
    }
  }

  if (!allValid) {
    console.error('\n❌ Credential validation failed');
    console.error('   Credentials found in fixtures - cannot commit');
    console.error('   Fix: Ensure all tokens are replaced with "REDACTED"');
    process.exit(1);
  }

  console.log('\n✅ All fixtures validated - safe to commit');
  process.exit(0);
}

/**
 * Global Jest teardown for E2E auth tests
 * Cleanup after all tests complete
 */

export default async function globalTeardown() {
  console.log('\n🧹 Cleaning up E2E Auth Test Environment...\n');

  // Cleanup will happen in each test file's afterAll hook

  console.log('✅ E2E Auth Test Environment Cleaned Up\n');
}

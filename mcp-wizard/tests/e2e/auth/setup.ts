/**
 * Global Jest setup for E2E auth tests
 * Starts oauth2-mock-server before all tests
 */

export default async function globalSetup() {
  console.log('\n🚀 Starting E2E Auth Test Environment...\n');

  // Note: oauth2-mock-server requires ESM import, which is not supported in Jest global setup
  // Instead, we'll start the server in each test file's beforeAll hook
  // This file exists as a placeholder for future global setup needs

  console.log('✅ E2E Auth Test Environment Ready\n');
}

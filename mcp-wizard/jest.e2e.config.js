module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  roots: ['<rootDir>/tests/e2e'],
  testMatch: ['**/*.test.ts'],
  globalSetup: '<rootDir>/tests/e2e/auth/setup.ts',
  globalTeardown: '<rootDir>/tests/e2e/auth/teardown.ts',

  // Reuse existing mocks from main jest config
  moduleNameMapper: {
    '^@/(.*)$': '<rootDir>/src/$1',
    '^ora$': '<rootDir>/tests/__mocks__/ora.js',
    '^inquirer$': '<rootDir>/tests/__mocks__/inquirer.js',
    '^open$': '<rootDir>/tests/__mocks__/open.js',
  },

  // Transform ESM modules
  transformIgnorePatterns: [
    'node_modules/(?!(open|ora|inquirer|oauth2-mock-server)/)',
  ],

  // Coverage settings for E2E tests
  collectCoverageFrom: [
    'src/lib/auth.ts',
    'src/lib/token-storage.ts',
    'tests/e2e/auth/helpers/**/*.ts',
    '!tests/e2e/**/*.test.ts',
    '!tests/e2e/**/setup.ts',
    '!tests/e2e/**/teardown.ts',
  ],

  coverageDirectory: 'coverage-e2e',
  coverageReporters: ['text', 'lcov', 'html'],

  // E2E tests may take longer
  testTimeout: 30000,

  verbose: true,

  // Setup files to run before each test
  setupFilesAfterEnv: [],
};

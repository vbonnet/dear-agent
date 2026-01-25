module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  roots: ['<rootDir>/tests', '<rootDir>/src'],
  testMatch: ['**/*.test.ts'],
  collectCoverageFrom: [
    'src/**/*.ts',
    '!src/**/*.d.ts',
    '!src/types/**/*',
  ],
  // Coverage thresholds disabled for global codebase - existing codebase has low coverage
  // New components have good coverage: SetupError (100%), ProgressTracker (100%),
  // SetupVerifier (96%), PrerequisitesValidator (75%), ConfigLocationDetector (68%)
  // Per-file coverage thresholds enabled for health/doctor/session-start code (oss-n1nq.15-v2)
  coverageThreshold: {
    './src/commands/health.ts': {
      statements: 90,
      branches: 80,
      functions: 90,
    },
    './src/commands/doctor.ts': {
      statements: 90,
      branches: 80,
      functions: 90,
    },
    './src/hooks/session-start.ts': {
      statements: 90,
      branches: 80,
      functions: 90,
    },
    './src/lib/mcp-proxy.ts': {
      statements: 90,
      branches: 80,
      functions: 90,
    },
  },
  moduleNameMapper: {
    '^@/(.*)$': '<rootDir>/src/$1',
    '^ora$': '<rootDir>/tests/__mocks__/ora.js',
    '^inquirer$': '<rootDir>/tests/__mocks__/inquirer.js',
    '^open$': '<rootDir>/tests/__mocks__/open.js',
    '^chalk$': '<rootDir>/tests/__mocks__/chalk.js',
  },
  transformIgnorePatterns: [
    'node_modules/(?!(open|ora|inquirer|chalk)/)',
  ],
};

import { defineConfig } from 'vitest/config';
import { resolve } from 'path';

export default defineConfig({
  root: import.meta.dirname,
  test: {
    // Multi-persona review specific configuration
    name: 'multi-persona-review',

    // Include tests from tests/ directory
    include: [
      'tests/**/*.test.ts',
      'src/**/*.test.ts',
    ],
    dir: import.meta.dirname,
    exclude: ['**/node_modules/**', '**/.worktrees/**'],

    // Test environment
    environment: 'node',

    // Coverage settings
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
    },
  },
  resolve: {
    alias: {
      '../src': resolve(__dirname, './src'),
    },
  },
});

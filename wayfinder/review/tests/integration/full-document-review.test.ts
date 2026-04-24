/**
 * Integration tests for full-document review mode
 * Tests --full flag and --scan full in non-git and git directories
 */

import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { mkdir, rm, writeFile } from 'fs/promises';
import { join } from 'path';
import { tmpdir } from 'os';
import { scanFiles, scanFileFull } from '../../src/file-scanner.js';
import type { ScanOptions } from '../../src/file-scanner.js';

describe('Full Document Review Mode', () => {
  let nonGitTestDir: string;
  let testFile1: string;
  let testFile2: string;

  beforeAll(async () => {
    // Create a non-git temporary directory
    nonGitTestDir = join(tmpdir(), `multi-persona-review-test-${Date.now()}`);
    await mkdir(nonGitTestDir, { recursive: true });

    // Create test files
    testFile1 = join(nonGitTestDir, 'test1.ts');
    testFile2 = join(nonGitTestDir, 'test2.ts');

    await writeFile(
      testFile1,
      `// Test file 1
function hello() {
  console.log("Hello, world!");
}

// This function has a potential issue
function divide(a: number, b: number) {
  return a / b; // No check for division by zero
}
`
    );

    await writeFile(
      testFile2,
      `// Test file 2
export class Calculator {
  add(a: number, b: number): number {
    return a + b;
  }

  subtract(a: number, b: number): number {
    return a - b;
  }
}
`
    );
  });

  afterAll(async () => {
    // Clean up test directory
    try {
      await rm(nonGitTestDir, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  });

  describe('Non-Git Directory Support', () => {
    it('should scan files in full mode without git', async () => {
      const options: ScanOptions = {
        mode: 'full',
      };

      const results = await scanFiles([testFile1], nonGitTestDir, options);

      expect(results).toHaveLength(1);
      expect(results[0].path).toContain('test1.ts');
      expect(results[0].isDiff).toBe(false);
      expect(results[0].content).toContain('function hello()');
      expect(results[0].content).toContain('function divide');
      expect(results[0].lineMapping).toBeUndefined();
    });

    it('should scan multiple files in full mode', async () => {
      const options: ScanOptions = {
        mode: 'full',
      };

      const results = await scanFiles(
        [testFile1, testFile2],
        nonGitTestDir,
        options
      );

      expect(results).toHaveLength(2);
      expect(results[0].isDiff).toBe(false);
      expect(results[1].isDiff).toBe(false);
    });

    it('should scan directory recursively in full mode', async () => {
      const options: ScanOptions = {
        mode: 'full',
      };

      const results = await scanFiles([nonGitTestDir], nonGitTestDir, options);

      expect(results.length).toBeGreaterThanOrEqual(2);
      const filePaths = results.map(r => r.path);
      expect(filePaths.some(p => p.includes('test1.ts'))).toBe(true);
      expect(filePaths.some(p => p.includes('test2.ts'))).toBe(true);
    });

    it('should include full file content, not diffs', async () => {
      const result = await scanFileFull(testFile1, nonGitTestDir);

      expect(result.content).toContain('// Test file 1');
      expect(result.content).toContain('function hello()');
      expect(result.content).toContain('function divide(a: number, b: number)');
      expect(result.content).toContain('return a / b');
      expect(result.isDiff).toBe(false);
    });

    it('should handle changed mode gracefully in non-git directory', async () => {
      // Changed mode should return empty array when git is not available
      const options: ScanOptions = {
        mode: 'changed',
      };

      const results = await scanFiles([testFile1], nonGitTestDir, options);

      // Should return empty array since there's no git
      expect(results).toHaveLength(0);
    });

    it('should handle diff mode gracefully in non-git directory', async () => {
      // Diff mode should return empty array when git is not available
      const options: ScanOptions = {
        mode: 'diff',
      };

      const results = await scanFiles([testFile1], nonGitTestDir, options);

      // Should return empty array since there's no git
      expect(results).toHaveLength(0);
    });
  });

  describe('File Filtering in Full Mode', () => {
    it('should respect include patterns in full mode', async () => {
      const options: ScanOptions = {
        mode: 'full',
        include: ['**/*.ts'],
      };

      const results = await scanFiles([nonGitTestDir], nonGitTestDir, options);

      expect(results.length).toBeGreaterThanOrEqual(2);
      results.forEach(result => {
        expect(result.path).toMatch(/\.ts$/);
      });
    });

    it('should respect exclude patterns in full mode', async () => {
      // Create a file to exclude
      const excludeFile = join(nonGitTestDir, 'exclude-me.ts');
      await writeFile(excludeFile, 'const x = 1;');

      const options: ScanOptions = {
        mode: 'full',
        exclude: ['exclude-me.ts'],
      };

      const results = await scanFiles([nonGitTestDir], nonGitTestDir, options);

      const filePaths = results.map(r => r.path);
      expect(filePaths.some(p => p.includes('exclude-me.ts'))).toBe(false);
      expect(filePaths.some(p => p.includes('test1.ts'))).toBe(true);

      // Cleanup
      await rm(excludeFile);
    });

    it('should skip binary files in full mode', async () => {
      // Create a fake binary file (with null bytes)
      const binaryFile = join(nonGitTestDir, 'binary.bin');
      const buffer = Buffer.from([0x00, 0x01, 0x02, 0x03]);
      await writeFile(binaryFile, buffer);

      const options: ScanOptions = {
        mode: 'full',
      };

      const results = await scanFiles([nonGitTestDir], nonGitTestDir, options);

      const filePaths = results.map(r => r.path);
      expect(filePaths.some(p => p.includes('binary.bin'))).toBe(false);

      // Cleanup
      await rm(binaryFile);
    });

    it('should respect maxFileSize in full mode', async () => {
      // Create a large file
      const largeFile = join(nonGitTestDir, 'large.ts');
      const largeContent = 'a'.repeat(10000);
      await writeFile(largeFile, largeContent);

      const options: ScanOptions = {
        mode: 'full',
        maxFileSize: 1000, // 1KB limit
      };

      const results = await scanFiles([nonGitTestDir], nonGitTestDir, options);

      const filePaths = results.map(r => r.path);
      expect(filePaths.some(p => p.includes('large.ts'))).toBe(false);
      expect(filePaths.some(p => p.includes('test1.ts'))).toBe(true);

      // Cleanup
      await rm(largeFile);
    });
  });

  describe('Edge Cases', () => {
    it('should handle empty files in full mode', async () => {
      const emptyFile = join(nonGitTestDir, 'empty.ts');
      await writeFile(emptyFile, '');

      const result = await scanFileFull(emptyFile, nonGitTestDir);

      expect(result.content).toBe('');
      expect(result.isDiff).toBe(false);

      // Cleanup
      await rm(emptyFile);
    });

    it('should handle files with special characters in full mode', async () => {
      const specialFile = join(nonGitTestDir, 'special-chars.ts');
      await writeFile(
        specialFile,
        '// File with unicode: 你好 👋\nconst emoji = "🎉";\n'
      );

      const result = await scanFileFull(specialFile, nonGitTestDir);

      expect(result.content).toContain('你好');
      expect(result.content).toContain('👋');
      expect(result.content).toContain('🎉');

      // Cleanup
      await rm(specialFile);
    });

    it('should handle deeply nested directories in full mode', async () => {
      const nestedDir = join(nonGitTestDir, 'a', 'b', 'c');
      await mkdir(nestedDir, { recursive: true });
      const nestedFile = join(nestedDir, 'nested.ts');
      await writeFile(nestedFile, 'const nested = true;');

      const options: ScanOptions = {
        mode: 'full',
      };

      const results = await scanFiles([nonGitTestDir], nonGitTestDir, options);

      const filePaths = results.map(r => r.path);
      expect(filePaths.some(p => p.includes('nested.ts'))).toBe(true);

      // Cleanup
      await rm(join(nonGitTestDir, 'a'), { recursive: true, force: true });
    });
  });

  describe('Backward Compatibility', () => {
    it('should work with explicit --scan full', async () => {
      // This is what --full flag maps to internally
      const options: ScanOptions = {
        mode: 'full',
      };

      const results = await scanFiles([testFile1], nonGitTestDir, options);

      expect(results).toHaveLength(1);
      expect(results[0].isDiff).toBe(false);
    });

    it('should maintain default changed mode behavior', async () => {
      // Default mode is 'changed' which requires git
      const options: ScanOptions = {
        mode: 'changed',
      };

      const results = await scanFiles([testFile1], nonGitTestDir, options);

      // Should gracefully handle non-git directory
      expect(results).toHaveLength(0);
    });
  });
});

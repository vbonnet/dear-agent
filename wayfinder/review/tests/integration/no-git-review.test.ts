/**
 * Integration test for multi-persona-review without git repository
 * Tests that explicit file paths work even when not in a git repository
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { mkdir, writeFile, rm } from 'fs/promises';
import { join } from 'path';
import { tmpdir } from 'os';
import {
  scanFileFull,
  scanFileDiff,
  scanFiles,
  getChangedFiles,
  getGitDiff,
} from '../src/file-scanner.js';

describe('no-git-review', () => {
  let testDir: string;

  beforeEach(async () => {
    // Create test directory WITHOUT git initialization
    testDir = join(tmpdir(), `multi-persona-review-no-git-test-${Date.now()}`);
    await mkdir(testDir, { recursive: true });
  });

  afterEach(async () => {
    try {
      await rm(testDir, { recursive: true, force: true });
    } catch (error) {
      // Ignore cleanup errors
    }
  });

  describe('scanFileFull without git', () => {
    it('should scan files in full mode without git repository', async () => {
      const filePath = join(testDir, 'test.ts');
      const content = 'export const foo = "bar";\nexport const baz = 42;';
      await writeFile(filePath, content);

      const result = await scanFileFull(filePath, testDir);

      expect(result.path).toBe('test.ts');
      expect(result.content).toBe(content);
      expect(result.isDiff).toBe(false);
      expect(result.lineMapping).toBeUndefined();
    });

    it('should scan multiple files without git repository', async () => {
      const file1 = join(testDir, 'file1.ts');
      const file2 = join(testDir, 'file2.ts');
      const content1 = 'const a = 1;';
      const content2 = 'const b = 2;';

      await writeFile(file1, content1);
      await writeFile(file2, content2);

      const result1 = await scanFileFull(file1, testDir);
      const result2 = await scanFileFull(file2, testDir);

      expect(result1.content).toBe(content1);
      expect(result2.content).toBe(content2);
      expect(result1.isDiff).toBe(false);
      expect(result2.isDiff).toBe(false);
    });

    it('should handle absolute file paths without git', async () => {
      const filePath = join(testDir, 'absolute-path.ts');
      const content = 'console.log("absolute path test");';
      await writeFile(filePath, content);

      const result = await scanFileFull(filePath, testDir);

      expect(result.path).toBe('absolute-path.ts');
      expect(result.content).toBe(content);
    });
  });

  describe('scanFileDiff without git', () => {
    it('should gracefully handle diff mode without git repository', async () => {
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'export const foo = "bar";');

      // Should return null when git is not available
      const result = await scanFileDiff(filePath, testDir);

      expect(result).toBeNull();
    });
  });

  describe('getGitDiff without git', () => {
    it('should return null when git is not available', async () => {
      const filePath = join(testDir, 'test.ts');
      await writeFile(filePath, 'export const foo = "bar";');

      const diff = await getGitDiff(filePath, testDir);

      expect(diff).toBeNull();
    });
  });

  describe('getChangedFiles without git', () => {
    it('should return empty array when git is not available', async () => {
      const changedFiles = await getChangedFiles(testDir);

      expect(changedFiles).toEqual([]);
    });
  });

  describe('scanFiles in full mode without git', () => {
    it('should scan files in full mode without git repository', async () => {
      const file1 = join(testDir, 'file1.ts');
      const file2 = join(testDir, 'file2.ts');

      await writeFile(file1, 'const a = 1;');
      await writeFile(file2, 'const b = 2;');

      const results = await scanFiles(
        [file1, file2],
        testDir,
        { mode: 'full' }
      );

      expect(results).toHaveLength(2);
      expect(results[0].content).toBe('const a = 1;');
      expect(results[1].content).toBe('const b = 2;');
      expect(results[0].isDiff).toBe(false);
      expect(results[1].isDiff).toBe(false);
    });

    it('should expand directories without git repository', async () => {
      const srcDir = join(testDir, 'src');
      await mkdir(srcDir);

      await writeFile(join(srcDir, 'index.ts'), 'export {};');
      await writeFile(join(srcDir, 'utils.ts'), 'export const util = 1;');

      const results = await scanFiles(
        [srcDir],
        testDir,
        { mode: 'full' }
      );

      expect(results).toHaveLength(2);
      expect(results.some(r => r.path.includes('index.ts'))).toBe(true);
      expect(results.some(r => r.path.includes('utils.ts'))).toBe(true);
    });
  });

  describe('scanFiles in diff mode without git', () => {
    it('should return empty array in diff mode when git is not available', async () => {
      const file1 = join(testDir, 'file1.ts');
      await writeFile(file1, 'const a = 1;');

      const results = await scanFiles(
        [file1],
        testDir,
        { mode: 'diff' }
      );

      // Diff mode returns empty when git is not available
      expect(results).toEqual([]);
    });
  });

  describe('scanFiles in changed mode without git', () => {
    it('should scan all files in changed mode when git is not available', async () => {
      const file1 = join(testDir, 'file1.ts');
      const file2 = join(testDir, 'file2.ts');

      await writeFile(file1, 'const a = 1;');
      await writeFile(file2, 'const b = 2;');

      // Mock console.warn to verify warning is shown
      const originalWarn = console.warn;
      let warnCalled = false;
      console.warn = (...args: any[]) => {
        if (args[0]?.includes('Git not available')) {
          warnCalled = true;
        }
      };

      const results = await scanFiles(
        [file1, file2],
        testDir,
        { mode: 'changed' }
      );

      // Restore console.warn
      console.warn = originalWarn;

      // In changed mode without git, should warn and scan all files in diff mode
      // Since diff mode returns null without git, we get empty results
      expect(warnCalled).toBe(true);
      expect(results).toEqual([]);
    });
  });

  describe('mixed scenarios', () => {
    it('should handle nested directories without git', async () => {
      const deepDir = join(testDir, 'a', 'b', 'c');
      await mkdir(deepDir, { recursive: true });

      const filePath = join(deepDir, 'nested.ts');
      const content = 'export const nested = true;';
      await writeFile(filePath, content);

      const result = await scanFileFull(filePath, testDir);

      expect(result.path).toBe('a/b/c/nested.ts');
      expect(result.content).toBe(content);
    });

    it('should filter files with include patterns without git', async () => {
      await writeFile(join(testDir, 'index.ts'), 'ts file');
      await writeFile(join(testDir, 'index.js'), 'js file');
      await writeFile(join(testDir, 'README.md'), 'readme');

      const results = await scanFiles(
        [testDir],
        testDir,
        { mode: 'full', include: ['**/*.ts'] }
      );

      expect(results).toHaveLength(1);
      expect(results[0].path).toBe('index.ts');
    });

    it('should filter files with exclude patterns without git', async () => {
      await writeFile(join(testDir, 'index.ts'), 'source');
      await writeFile(join(testDir, 'test.test.ts'), 'test');

      const results = await scanFiles(
        [testDir],
        testDir,
        { mode: 'full', exclude: ['**/*.test.ts'] }
      );

      expect(results).toHaveLength(1);
      expect(results[0].path).toBe('index.ts');
    });
  });

  describe('error handling without git', () => {
    it('should throw appropriate error for missing files', async () => {
      const missingFile = join(testDir, 'does-not-exist.ts');

      await expect(scanFileFull(missingFile, testDir)).rejects.toThrow(/not found/);
    });

    it('should skip binary files gracefully', async () => {
      const textFile = join(testDir, 'text.txt');
      const binaryFile = join(testDir, 'binary.bin');

      await writeFile(textFile, 'Hello');
      await writeFile(binaryFile, Buffer.from([0x00, 0x01, 0x02]));

      const results = await scanFiles(
        [textFile, binaryFile],
        testDir,
        { mode: 'full' }
      );

      expect(results).toHaveLength(1);
      expect(results[0].path).toBe('text.txt');
    });
  });
});

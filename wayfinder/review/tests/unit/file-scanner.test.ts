/**
 * Tests for file scanner
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { mkdir, writeFile, rm } from 'fs/promises';
import { join } from 'path';
import { tmpdir } from 'os';
import { execSync } from 'child_process';
import {
  isBinaryFile,
  shouldExcludeFile,
  scanFileFull,
  parseDiffLineMapping,
  findFiles,
  scanFiles,
  FileScannerError,
  FILE_SCANNER_ERROR_CODES,
} from '../src/file-scanner.js';

describe('file-scanner', () => {
  let testDir: string;

  beforeEach(async () => {
    testDir = join(tmpdir(), `multi-persona-review-test-${Date.now()}`);
    await mkdir(testDir, { recursive: true });

    // Initialize git repo for tests (some tests need it, some don't)
    try {
      execSync('git init', { cwd: testDir, stdio: 'ignore' });
      execSync('git config user.email "test@test.com"', { cwd: testDir, stdio: 'ignore' });
      execSync('git config user.name "Test User"', { cwd: testDir, stdio: 'ignore' });
    } catch (error) {
      // Git initialization optional - some tests don't need it
      // Tests that require git will fail explicitly if it's not available
    }
  });

  afterEach(async () => {
    try {
      await rm(testDir, { recursive: true, force: true });
    } catch (error) {
      // Ignore cleanup errors
    }
  });

  describe('isBinaryFile', () => {
    it('should detect text files as non-binary', async () => {
      // Ensure directory exists before writing
      await mkdir(testDir, { recursive: true });
      const filePath = join(testDir, 'test.txt');
      await writeFile(filePath, 'Hello, world!');

      const result = await isBinaryFile(filePath);
      expect(result).toBe(false);
    });

    it('should detect binary files', async () => {
      // Ensure directory exists before writing
      await mkdir(testDir, { recursive: true });
      const filePath = join(testDir, 'test.bin');
      const buffer = Buffer.from([0x00, 0x01, 0x02, 0xFF]);
      await writeFile(filePath, buffer);

      const result = await isBinaryFile(filePath);
      expect(result).toBe(true);
    });

    it('should throw on missing file', async () => {
      const filePath = join(testDir, 'missing.txt');

      await expect(isBinaryFile(filePath)).rejects.toThrow(FileScannerError);
    });
  });

  describe('shouldExcludeFile', () => {
    it('should not exclude when no patterns', () => {
      expect(shouldExcludeFile('src/index.ts')).toBe(false);
      expect(shouldExcludeFile('src/index.ts', [])).toBe(false);
    });

    it('should exclude matching patterns', () => {
      const exclude = ['**/node_modules/**', '**/*.test.ts', 'dist/**'];

      expect(shouldExcludeFile('node_modules/foo/bar.js', exclude)).toBe(true);
      expect(shouldExcludeFile('src/test.test.ts', exclude)).toBe(true);
      expect(shouldExcludeFile('dist/index.js', exclude)).toBe(true);
    });

    it('should not exclude non-matching patterns', () => {
      const exclude = ['**/node_modules/**', '**/*.test.ts'];

      expect(shouldExcludeFile('src/index.ts', exclude)).toBe(false);
      expect(shouldExcludeFile('src/utils.ts', exclude)).toBe(false);
    });

    it('should handle wildcards correctly', () => {
      const exclude = ['*.log', 'temp*'];

      expect(shouldExcludeFile('error.log', exclude)).toBe(true);
      expect(shouldExcludeFile('temp-file.txt', exclude)).toBe(true);
      expect(shouldExcludeFile('src/error.log', exclude)).toBe(false); // * doesn't match /
    });
  });

  describe('scanFileFull', () => {
    it('should scan text file in full mode', async () => {
      const filePath = join(testDir, 'test.ts');
      const content = 'export const foo = "bar";';
      await writeFile(filePath, content);

      const result = await scanFileFull(filePath, testDir);

      expect(result.path).toBe('test.ts');
      expect(result.content).toBe(content);
      expect(result.isDiff).toBe(false);
      expect(result.lineMapping).toBeUndefined();
    });

    it('should throw on binary file', async () => {
      const filePath = join(testDir, 'test.bin');
      const buffer = Buffer.from([0x00, 0x01, 0x02]);
      await writeFile(filePath, buffer);

      await expect(scanFileFull(filePath, testDir)).rejects.toThrow(FileScannerError);
      await expect(scanFileFull(filePath, testDir)).rejects.toThrow(/binary file/);
    });

    it('should throw on missing file', async () => {
      const filePath = join(testDir, 'missing.ts');

      await expect(scanFileFull(filePath, testDir)).rejects.toThrow(FileScannerError);
      await expect(scanFileFull(filePath, testDir)).rejects.toThrow(/not found/);
    });
  });

  describe('parseDiffLineMapping', () => {
    it('should parse simple diff', () => {
      const diff = `diff --git a/test.ts b/test.ts
index 123..456 789
--- a/test.ts
+++ b/test.ts
@@ -1,3 +1,4 @@
 line 1
-line 2
+line 2 modified
+line 2.5 added
 line 3`;

      const mapping = parseDiffLineMapping(diff);

      expect(mapping.diffToOriginal[1]).toBe(1); // line 1
      expect(mapping.diffToOriginal[2]).toBe(-1); // line 2 modified (new)
      expect(mapping.diffToOriginal[3]).toBe(-1); // line 2.5 added (new)
      expect(mapping.diffToOriginal[4]).toBe(3); // line 3
    });

    it('should handle multiple hunks', () => {
      const diff = `@@ -1,2 +1,2 @@
 line 1
-line 2
+line 2 changed
@@ -10,2 +10,3 @@
 line 10
+line 10.5 added
 line 11`;

      const mapping = parseDiffLineMapping(diff);

      expect(mapping.diffToOriginal[1]).toBe(1);
      expect(mapping.diffToOriginal[2]).toBe(-1);
      expect(mapping.diffToOriginal[3]).toBe(10);
      expect(mapping.diffToOriginal[4]).toBe(-1);
      expect(mapping.diffToOriginal[5]).toBe(11);
    });

    it('should handle empty hunks (new file)', () => {
      const diff = `@@ -0,0 +1,1 @@
+new file`;

      const mapping = parseDiffLineMapping(diff);

      expect(mapping.diffToOriginal[1]).toBe(-1); // New line
    });

    it('should handle malformed hunk headers gracefully', () => {
      const diff = `@@ invalid @@
 line 1`;

      const mapping = parseDiffLineMapping(diff);

      // Should not crash, return empty mapping
      expect(Object.keys(mapping.diffToOriginal)).toHaveLength(0);
    });

    it('should handle hunks with only additions', () => {
      const diff = `@@ -0,0 +1,3 @@
+line 1
+line 2
+line 3`;

      const mapping = parseDiffLineMapping(diff);

      expect(mapping.diffToOriginal[1]).toBe(-1);
      expect(mapping.diffToOriginal[2]).toBe(-1);
      expect(mapping.diffToOriginal[3]).toBe(-1);
    });

    it('should handle hunks with only deletions', () => {
      const diff = `@@ -1,3 +0,0 @@
-line 1
-line 2
-line 3`;

      const mapping = parseDiffLineMapping(diff);

      // Only deletions, no new lines
      expect(Object.keys(mapping.diffToOriginal)).toHaveLength(0);
    });
  });

  describe('findFiles', () => {
    it('should find all files recursively', async () => {
      await mkdir(join(testDir, 'src'), { recursive: true });
      await mkdir(join(testDir, 'test'), { recursive: true });

      await writeFile(join(testDir, 'index.ts'), '');
      await writeFile(join(testDir, 'src', 'utils.ts'), '');
      await writeFile(join(testDir, 'test', 'utils.test.ts'), '');

      const files = await findFiles(testDir);

      expect(files).toHaveLength(3);
      expect(files.some(f => f.endsWith('index.ts'))).toBe(true);
      expect(files.some(f => f.endsWith('utils.ts'))).toBe(true);
      expect(files.some(f => f.endsWith('utils.test.ts'))).toBe(true);
    });

    it('should respect exclude patterns', async () => {
      await mkdir(join(testDir, 'src'), { recursive: true });
      await mkdir(join(testDir, 'node_modules'), { recursive: true });

      await writeFile(join(testDir, 'src', 'index.ts'), '');
      await writeFile(join(testDir, 'node_modules', 'lib.js'), '');

      const files = await findFiles(testDir, undefined, ['node_modules/**']);

      expect(files).toHaveLength(1);
      expect(files[0]).toContain('index.ts');
    });

    it('should respect include patterns', async () => {
      await mkdir(join(testDir, 'src'), { recursive: true });

      await writeFile(join(testDir, 'src', 'index.ts'), '');
      await writeFile(join(testDir, 'src', 'utils.js'), '');
      await writeFile(join(testDir, 'README.md'), '');

      const files = await findFiles(testDir, ['**/*.ts']);

      expect(files).toHaveLength(1);
      expect(files[0]).toContain('index.ts');
    });
  });

  describe('scanFiles', () => {
    it('should scan files in full mode', async () => {
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

    it('should skip binary files', async () => {
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

    it('should expand directories', async () => {
      const srcDir = join(testDir, 'src');
      await mkdir(srcDir);

      await writeFile(join(srcDir, 'index.ts'), 'export {};');
      await writeFile(join(srcDir, 'utils.ts'), 'export {};');

      const results = await scanFiles(
        [srcDir],
        testDir,
        { mode: 'full' }
      );

      expect(results).toHaveLength(2);
    });

    it('should respect exclude patterns', async () => {
      await writeFile(join(testDir, 'index.ts'), 'export {};');
      await writeFile(join(testDir, 'test.test.ts'), 'test();');

      const results = await scanFiles(
        [testDir],
        testDir,
        { mode: 'full', exclude: ['**/*.test.ts'] }
      );

      expect(results).toHaveLength(1);
      expect(results[0].path).toBe('index.ts');
    });

    it('should respect max file size', async () => {
      const smallFile = join(testDir, 'small.txt');
      const largeFile = join(testDir, 'large.txt');

      await writeFile(smallFile, 'small');
      await writeFile(largeFile, 'x'.repeat(10000));

      const results = await scanFiles(
        [smallFile, largeFile],
        testDir,
        { mode: 'full', maxFileSize: 1000 }
      );

      expect(results).toHaveLength(1);
      expect(results[0].path).toBe('small.txt');
    });
  });
});

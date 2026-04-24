/**
 * File scanner for multi-persona-review plugin
 * Handles scanning files in full, diff, or changed modes
 */

import { readFile, stat, readdir } from 'fs/promises';
import { join, relative } from 'path';
import { exec } from 'child_process';
import { promisify } from 'util';
import type { FileContent, FileScanMode, LineMapping } from './types.js';

const execAsync = promisify(exec);

/**
 * Error codes for file scanning
 */
export const FILE_SCANNER_ERROR_CODES = {
  FILE_NOT_FOUND: 'FILE_3001',
  READ_ERROR: 'FILE_3002',
  GIT_ERROR: 'FILE_3003',
  INVALID_MODE: 'FILE_3004',
  BINARY_FILE: 'FILE_3005',
} as const;

/**
 * File scanning error
 */
export class FileScannerError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown
  ) {
    super(message);
    this.name = 'FileScannerError';
  }
}

/**
 * Scan options
 */
export interface ScanOptions {
  /** Scan mode */
  mode: FileScanMode;

  /** Context lines before and after diff hunks */
  contextLines?: number;

  /** Git reference to compare against (for diff mode) */
  gitRef?: string;

  /** Include only specific file patterns */
  include?: string[];

  /** Exclude file patterns */
  exclude?: string[];

  /** Maximum file size in bytes */
  maxFileSize?: number;
}

/**
 * Converts glob pattern to regex
 */
function globToRegex(pattern: string): RegExp {
  let escaped = '';
  for (let i = 0; i < pattern.length; i++) {
    const char = pattern[i];
    if (char === '*') {
      if (pattern[i + 1] === '*') {
        // Check what comes after **
        if (pattern[i + 2] === '/') {
          // **/ means zero or more path segments
          escaped += '(.*\\/)?';
          i += 2; // Skip ** and /
        } else {
          // ** at end or before non-slash
          escaped += '.*';
          i++; // Skip second *
        }
      } else {
        escaped += '[^/]*';
      }
    } else if (char === '?') {
      escaped += '.';
    } else if ('.+[]{}()^$|\\'.includes(char)) {
      escaped += '\\' + char;
    } else {
      escaped += char;
    }
  }
  return new RegExp(`^${escaped}$`);
}

/**
 * Sample size for binary file detection (first 8KB)
 */
const BINARY_SAMPLE_SIZE = 8000;

/**
 * Checks if a file is binary
 */
export async function isBinaryFile(filePath: string): Promise<boolean> {
  try {
    const buffer = await readFile(filePath);
    const sample = buffer.slice(0, BINARY_SAMPLE_SIZE);

    // Check for null bytes (common in binary files)
    for (let i = 0; i < sample.length; i++) {
      if (sample[i] === 0) {
        return true;
      }
    }

    return false;
  } catch (error) {
    throw new FileScannerError(
      FILE_SCANNER_ERROR_CODES.READ_ERROR,
      `Failed to check if file is binary: ${filePath}`,
      error
    );
  }
}

/**
 * Checks if a file should be excluded based on patterns
 */
export function shouldExcludeFile(
  filePath: string,
  exclude?: string[]
): boolean {
  if (!exclude || exclude.length === 0) {
    return false;
  }

  for (const pattern of exclude) {
    const regex = globToRegex(pattern);
    if (regex.test(filePath)) {
      return true;
    }
  }

  return false;
}

/**
 * Scans a single file in full mode
 */
export async function scanFileFull(
  filePath: string,
  cwd: string
): Promise<FileContent> {
  try {
    const stats = await stat(filePath);

    if (!stats.isFile()) {
      throw new FileScannerError(
        FILE_SCANNER_ERROR_CODES.READ_ERROR,
        `Path is not a file: ${filePath}`
      );
    }

    if (await isBinaryFile(filePath)) {
      throw new FileScannerError(
        FILE_SCANNER_ERROR_CODES.BINARY_FILE,
        `Cannot scan binary file: ${filePath}`
      );
    }

    const content = await readFile(filePath, 'utf-8');
    const relativePath = relative(cwd, filePath);

    return {
      path: relativePath,
      content,
      isDiff: false,
    };
  } catch (error) {
    if (error instanceof FileScannerError) {
      throw error;
    }

    if (error && typeof error === 'object' && 'code' in error && error.code === 'ENOENT') {
      throw new FileScannerError(
        FILE_SCANNER_ERROR_CODES.FILE_NOT_FOUND,
        `File not found: ${filePath}`,
        error
      );
    }

    throw new FileScannerError(
      FILE_SCANNER_ERROR_CODES.READ_ERROR,
      `Failed to scan file: ${filePath}`,
      error
    );
  }
}

/**
 * Validates a git reference (branch, tag, commit hash)
 */
function validateGitRef(ref: string): void {
  // Git refs can contain alphanumeric, /, -, _, .
  // This prevents command injection
  if (!/^[a-zA-Z0-9/_.-]+$/.test(ref)) {
    throw new FileScannerError(
      FILE_SCANNER_ERROR_CODES.GIT_ERROR,
      `Invalid git ref: ${ref}`
    );
  }
}

/**
 * Gets git diff for a file
 * Returns null if git is not available (graceful degradation)
 */
export async function getGitDiff(
  filePath: string,
  cwd: string,
  gitRef: string = 'HEAD',
  contextLines: number = 3
): Promise<string | null> {
  validateGitRef(gitRef);

  try {
    // Find git root from current directory
    const gitRoot = await findGitRoot(cwd);

    // If not in a git repository, return null for graceful degradation
    if (!gitRoot) {
      return null;
    }

    // Make filePath relative to git root for git command
    const relativeFilePath = relative(gitRoot, filePath);

    // Run git diff from the repository root
    const { stdout } = await execAsync(
      `git diff -U${contextLines} ${gitRef} -- "${relativeFilePath}"`,
      { cwd: gitRoot }
    );
    return stdout;
  } catch (error) {
    // Return null on git errors for graceful degradation
    return null;
  }
}

/**
 * Parses a git diff and creates line mapping
 * Maps diff content line numbers to original file line numbers
 *
 * @param diff - Git diff output (from git diff)
 * @returns Line mapping with bidirectional mappings
 *
 * @example
 * Given diff:
 *   @@ -1,2 +1,2 @@
 *    line 1
 *   -line 2
 *   +line 2 changed
 *   @@ -10,2 +10,3 @@
 *    line 10
 *   +line 10.5 added
 *    line 11
 *
 * Returns:
 *   diffToOriginal: { 1: 1, 2: -1, 3: 10, 4: -1, 5: 11 }
 *   (Keys are sequential line numbers in diff content,
 *    values are original file line numbers, -1 for new lines)
 */
export function parseDiffLineMapping(diff: string): LineMapping {
  const diffToOriginal: Record<number, number> = {};
  const originalToDiff: Record<number, number> = {};

  const lines = diff.split('\n');
  let diffLineNum = 0; // Sequential line number in diff content
  let originalLineNum = 0;
  let inHunk = false;

  for (const line of lines) {
    // Parse hunk headers like @@ -10,7 +10,8 @@
    if (line.startsWith('@@')) {
      const match = line.match(/@@ -(\d+),?\d* \+(\d+),?\d* @@/);
      if (match) {
        originalLineNum = parseInt(match[1], 10) - 1; // -1 because we increment before processing
        inHunk = true;
      }
      continue;
    }

    // Skip diff metadata
    if (line.startsWith('diff --git') || line.startsWith('index') ||
        line.startsWith('---') || line.startsWith('+++')) {
      continue;
    }

    if (!inHunk) {
      continue;
    }

    if (line.startsWith('-')) {
      // Deleted line - exists in original but not in diff content
      originalLineNum++;
    } else if (line.startsWith('+')) {
      // Added line - exists in diff content but not in original
      diffLineNum++;
      diffToOriginal[diffLineNum] = -1; // Mark as new line
    } else {
      // Context line - exists in both
      diffLineNum++;
      originalLineNum++;
      diffToOriginal[diffLineNum] = originalLineNum;
      originalToDiff[originalLineNum] = diffLineNum;
    }
  }

  return {
    diffToOriginal,
    originalToDiff,
  };
}

/**
 * Scans a single file in diff mode
 * Returns null if git is not available or no diff (graceful degradation)
 */
export async function scanFileDiff(
  filePath: string,
  cwd: string,
  gitRef: string = 'HEAD',
  contextLines: number = 3
): Promise<FileContent | null> {
  try {
    const diff = await getGitDiff(filePath, cwd, gitRef, contextLines);

    // If no diff or git not available, skip this file
    if (!diff || diff.trim().length === 0) {
      return null;
    }

    const lineMapping = parseDiffLineMapping(diff);
    const relativePath = relative(cwd, filePath);

    return {
      path: relativePath,
      content: diff,
      isDiff: true,
      lineMapping,
    };
  } catch (error) {
    if (error instanceof FileScannerError) {
      throw error;
    }

    // Return null on errors for graceful degradation
    return null;
  }
}

/**
 * Finds the git repository root from a given directory
 * Returns null if not in a git repository (graceful degradation)
 */
async function findGitRoot(cwd: string): Promise<string | null> {
  try {
    const { stdout } = await execAsync(
      'git rev-parse --show-toplevel',
      { cwd }
    );
    return stdout.trim();
  } catch (error) {
    // Not a git repository - return null instead of throwing
    return null;
  }
}

/**
 * Gets list of changed files from git
 * Returns empty array if git is not available (graceful degradation)
 */
export async function getChangedFiles(
  cwd: string,
  gitRef: string = 'HEAD'
): Promise<string[]> {
  validateGitRef(gitRef);

  try {
    // Find git root from current directory
    const gitRoot = await findGitRoot(cwd);

    // If not in a git repository, return empty array for graceful degradation
    if (!gitRoot) {
      return [];
    }

    // Run git diff from the repository root
    const { stdout } = await execAsync(
      `git diff --name-only ${gitRef}`,
      { cwd: gitRoot }
    );

    // Convert relative paths from git root to absolute paths
    return stdout
      .split('\n')
      .map(line => line.trim())
      .filter(line => line.length > 0)
      .map(file => join(gitRoot, file));
  } catch (error) {
    // Return empty array on git errors for graceful degradation
    return [];
  }
}

/**
 * Recursively finds files matching patterns
 */
export async function findFiles(
  dirPath: string,
  include?: string[],
  exclude?: string[],
  maxDepth: number = 100
): Promise<string[]> {
  const files: string[] = [];

  // Always exclude .git directory
  const defaultExcludes = ['.git/**', '.git'];
  const allExcludes = exclude ? [...defaultExcludes, ...exclude] : defaultExcludes;

  async function scan(dir: string, currentDepth: number = 0) {
    if (currentDepth >= maxDepth) {
      return;
    }

    const entries = await readdir(dir, { withFileTypes: true });

    for (const entry of entries) {
      const fullPath = join(dir, entry.name);
      const relativePath = relative(dirPath, fullPath);

      if (shouldExcludeFile(relativePath, allExcludes)) {
        continue;
      }

      if (entry.isDirectory()) {
        await scan(fullPath, currentDepth + 1);
      } else if (entry.isFile()) {
        // Check if file matches include patterns
        if (include && include.length > 0) {
          let matches = false;
          for (const pattern of include) {
            const regex = globToRegex(pattern);
            if (regex.test(relativePath)) {
              matches = true;
              break;
            }
          }
          if (!matches) {
            continue;
          }
        }

        files.push(fullPath);
      }
    }
  }

  await scan(dirPath);
  return files;
}

/**
 * Scans files based on options
 */
export async function scanFiles(
  paths: string[],
  cwd: string,
  options: ScanOptions
): Promise<FileContent[]> {
  const fileContents: FileContent[] = [];

  // Expand directories to files
  let filesToScan: string[] = [];
  for (const path of paths) {
    const stats = await stat(path);
    if (stats.isDirectory()) {
      const found = await findFiles(path, options.include, options.exclude);
      filesToScan.push(...found);
    } else {
      filesToScan.push(path);
    }
  }

  // Remove duplicates
  filesToScan = [...new Set(filesToScan)];

  // Filter by changed files if in changed mode
  if (options.mode === 'changed') {
    const changedFiles = await getChangedFiles(cwd, options.gitRef);
    // If git is not available (empty array), warn and continue with all files
    if (changedFiles.length === 0) {
      console.warn('Warning: Git not available in changed mode, scanning all files instead');
    } else {
      const changedSet = new Set(changedFiles);
      filesToScan = filesToScan.filter(file => changedSet.has(file));
    }
  }

  // Scan each file
  for (const filePath of filesToScan) {
    try {
      // Check if binary
      if (await isBinaryFile(filePath)) {
        continue; // Skip binary files
      }

      // Check file size if limit specified
      if (options.maxFileSize) {
        const stats = await stat(filePath);
        if (stats.size > options.maxFileSize) {
          continue; // Skip large files
        }
      }

      let content: FileContent | null = null;

      switch (options.mode) {
        case 'full':
          content = await scanFileFull(filePath, cwd);
          break;

        case 'diff':
          content = await scanFileDiff(
            filePath,
            cwd,
            options.gitRef,
            options.contextLines
          );
          break;

        case 'changed':
          // In changed mode, use diff to get only changes
          content = await scanFileDiff(
            filePath,
            cwd,
            options.gitRef,
            options.contextLines
          );
          break;

        default:
          throw new FileScannerError(
            FILE_SCANNER_ERROR_CODES.INVALID_MODE,
            `Invalid scan mode: ${options.mode}`
          );
      }

      if (content) {
        fileContents.push(content);
      }
    } catch (error) {
      // Log warning but continue with other files
      console.warn(`Failed to scan ${filePath}:`, error instanceof Error ? error.message : String(error));
    }
  }

  return fileContents;
}

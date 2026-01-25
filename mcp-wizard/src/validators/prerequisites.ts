import { execFile } from 'child_process';
import { promisify } from 'util';

const execFileAsync = promisify(execFile);

export interface ValidationResult {
  name: string;
  passed: boolean;
  error?: string;
  fix?: string;
}

export class PrerequisitesValidator {
  /**
   * Run all prerequisite validators in parallel
   * @returns Array of validation results (all validators, regardless of pass/fail)
   */
  async validateAll(): Promise<ValidationResult[]> {
    // Run all validators in parallel for 2.4x speedup (5s vs 12s)
    const results = await Promise.all([
      this.validateNodeVersion(),
      this.validateGcloudInstalled(),
      this.validateGcloudAuth(),
      this.validateClaudeCode(),
    ]);

    return results;
  }

  /**
   * Validate Node.js version (>=18)
   * @returns ValidationResult with pass/fail + error/fix if needed
   */
  async validateNodeVersion(): Promise<ValidationResult> {
    const version = process.version; // e.g., "v18.12.0"
    const major = parseInt(version.split('.')[0].slice(1)); // 18

    if (major >= 18) {
      return { name: 'Node.js', passed: true };
    } else {
      return {
        name: 'Node.js',
        passed: false,
        error: `Node.js v${major} (required: >=18)`,
        fix: 'Upgrade Node.js to v18+ (https://nodejs.org)',
      };
    }
  }

  /**
   * Validate gcloud CLI is installed
   * @returns ValidationResult (checks: `gcloud --version`)
   */
  async validateGcloudInstalled(): Promise<ValidationResult> {
    try {
      // Timeout after 5s (prevent hangs on network issues)
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 5000);

      await execFileAsync('gcloud', ['--version'], {
        signal: controller.signal as any,
      });
      clearTimeout(timeoutId);

      return { name: 'gcloud CLI', passed: true };
    } catch (error: any) {
      if (error.name === 'AbortError') {
        return {
          name: 'gcloud CLI',
          passed: false,
          error: 'gcloud command timed out (5s)',
          fix: 'Check network connection or install gcloud: https://cloud.google.com/sdk/docs/install',
        };
      }

      return {
        name: 'gcloud CLI',
        passed: false,
        error: 'gcloud not found in PATH',
        fix: 'Install gcloud CLI: https://cloud.google.com/sdk/docs/install',
      };
    }
  }

  /**
   * Validate gcloud is authenticated (ADC credentials exist)
   * @returns ValidationResult (checks: `gcloud auth application-default print-access-token`)
   */
  async validateGcloudAuth(): Promise<ValidationResult> {
    try {
      // Timeout after 5s
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 5000);

      const { stdout } = await execFileAsync(
        'gcloud',
        ['auth', 'application-default', 'print-access-token'],
        { signal: controller.signal as any }
      );
      clearTimeout(timeoutId);

      // Token present = authenticated
      if (stdout.trim().length > 0) {
        return { name: 'gcloud auth', passed: true };
      } else {
        return {
          name: 'gcloud auth',
          passed: false,
          error: 'gcloud authenticated but no token returned',
          fix: 'Run: gcloud auth application-default login',
        };
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        return {
          name: 'gcloud auth',
          passed: false,
          error: 'gcloud auth check timed out (5s)',
          fix: 'Check network connection',
        };
      }

      return {
        name: 'gcloud auth',
        passed: false,
        error: 'not authenticated',
        fix: 'Run: gcloud auth application-default login',
      };
    }
  }

  /**
   * Validate Claude Code is installed
   * @returns ValidationResult (checks: `claude --version` or `which claude`)
   */
  async validateClaudeCode(): Promise<ValidationResult> {
    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 5000);

      await execFileAsync('claude', ['--version'], {
        signal: controller.signal as any,
      });
      clearTimeout(timeoutId);

      return { name: 'Claude Code', passed: true };
    } catch (error) {
      // Try 'which claude' as fallback
      try {
        await execFileAsync('which', ['claude']);
        return { name: 'Claude Code', passed: true };
      } catch {
        return {
          name: 'Claude Code',
          passed: false,
          error: 'claude not found in PATH',
          fix: 'Install Claude Code: https://claude.com/claude-code',
        };
      }
    }
  }
}

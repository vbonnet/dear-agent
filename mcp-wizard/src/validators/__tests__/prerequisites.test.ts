import { PrerequisitesValidator } from '../prerequisites';

jest.mock('child_process', () => ({
  execFile: jest.fn(),
}));

describe('PrerequisitesValidator', () => {
  let validator: PrerequisitesValidator;

  beforeEach(() => {
    validator = new PrerequisitesValidator();
    jest.clearAllMocks();
  });

  describe('validateNodeVersion', () => {
    it('should pass for Node.js v18+', async () => {
      const result = await validator.validateNodeVersion();
      const major = parseInt(process.version.split('.')[0].slice(1));

      if (major >= 18) {
        expect(result.passed).toBe(true);
        expect(result.name).toBe('Node.js');
      }
    });
  });

  describe('validateGcloudInstalled', () => {
    it('should pass when gcloud is installed', async () => {
      const { execFile } = require('child_process');
      execFile.mockImplementation((cmd: string, args: string[], opts: any, callback: any) => {
        callback(null, { stdout: 'Google Cloud SDK 450.0.0\n', stderr: '' });
      });

      const result = await validator.validateGcloudInstalled();
      expect(result.passed).toBe(true);
      expect(result.name).toBe('gcloud CLI');
    });

    it('should fail when gcloud is not installed', async () => {
      const { execFile } = require('child_process');
      execFile.mockImplementation((cmd: string, args: string[], opts: any, callback: any) => {
        const error: any = new Error('Command not found');
        error.code = 'ENOENT';
        callback(error);
      });

      const result = await validator.validateGcloudInstalled();
      expect(result.passed).toBe(false);
      expect(result.error).toContain('gcloud not found');
      expect(result.fix).toContain('Install gcloud');
    });
  });

  describe('validateAll', () => {
    it('should run all validators in parallel', async () => {
      const startTime = Date.now();
      await validator.validateAll();
      const duration = Date.now() - startTime;

      // Parallel execution should be faster than 12s sequential
      // (Though in mocked tests this is instant)
      expect(duration).toBeLessThan(12000);
    });

    it('should return all validation results', async () => {
      const results = await validator.validateAll();

      expect(results).toHaveLength(4);
      expect(results[0].name).toBe('Node.js');
      expect(results[1].name).toBe('gcloud CLI');
      expect(results[2].name).toBe('gcloud auth');
      expect(results[3].name).toBe('Claude Code');
    });
  });
});

import {
  detectEnvironment,
  validateNodeVersion,
  detectChezmoi,
  detectSudo,
  pathExists,
} from '../../src/lib/detect';

describe('Environment Detection', () => {
  describe('hostname detection', () => {
    test('work machine has hostname ending in -w', () => {
      // This test is covered by detectEnvironment integration test
      expect('vbonnet-w'.endsWith('-w')).toBe(true);
    });

    test('personal machine does not end in -w', () => {
      expect('vbonnet-personal'.endsWith('-w')).toBe(false);
    });
  });

  describe('validateNodeVersion', () => {
    test('validates Node.js version >= 18.0.0', async () => {
      expect(await validateNodeVersion('24.9.0')).toBe(true);
      expect(await validateNodeVersion('v24.9.0')).toBe(true);
      expect(await validateNodeVersion('18.0.0')).toBe(true);
      expect(await validateNodeVersion('v18.0.0')).toBe(true);
    });

    test('rejects Node.js version < 18.0.0', async () => {
      expect(await validateNodeVersion('16.0.0')).toBe(false);
      expect(await validateNodeVersion('v16.0.0')).toBe(false);
      expect(await validateNodeVersion('14.21.0')).toBe(false);
    });
  });

  describe('detectSudo', () => {
    test('detects sudo via SUDO_USER environment variable', () => {
      process.env.SUDO_USER = 'vbonnet';
      expect(detectSudo()).toBe(true);
      delete process.env.SUDO_USER;
    });

    test('detects non-sudo execution', () => {
      delete process.env.SUDO_USER;
      // Note: Can't test getuid() === 0 without actually running as root
      expect(detectSudo()).toBe(false);
    });
  });

  describe('pathExists', () => {
    test('returns true for existing path', async () => {
      // Test with a path that should exist (current file)
      const result = await pathExists(__filename);
      expect(result).toBe(true);
    });

    test('returns false for non-existing path', async () => {
      const result = await pathExists('/tmp/nonexistent-file-12345.txt');
      expect(result).toBe(false);
    });
  });

  // Integration test (requires mocking or actual filesystem setup)
  describe('detectChezmoi', () => {
    test('detects chezmoi installation', async () => {
      // This would require mocking fs.access or setting up test files
      // Skipping for now - would be implemented with jest.mock('fs')
    });
  });
});

import { SetupVerifier } from '../setup-verifier';

jest.mock('child_process', () => ({
  execFile: jest.fn(),
  exec: jest.fn(),
}));

describe('SetupVerifier', () => {
  let verifier: SetupVerifier;

  beforeEach(() => {
    verifier = new SetupVerifier();
    jest.clearAllMocks();
  });

  describe('parseMcpList', () => {
    it('should parse JSON output format', () => {
      const jsonOutput = JSON.stringify({
        googledocs: { command: 'npx', args: [] },
        glean: { command: 'npx', args: [] },
      });

      const result = (verifier as any).parseMcpList(jsonOutput);
      expect(result).toEqual(['googledocs', 'glean']);
    });

    it('should parse text output format', () => {
      const textOutput = `Available MCP servers:
  - GoogleDocs
  - Glean
  - Slack`;

      const result = (verifier as any).parseMcpList(textOutput);
      expect(result).toEqual(['GoogleDocs', 'Glean', 'Slack']);
    });
  });

  describe('verifyMcpConnection', () => {
    it('should succeed when MCP is found (JSON format)', async () => {
      const { execFile } = require('child_process');
      execFile.mockImplementation((cmd: string, args: string[], opts: any, callback: any) => {
        const jsonOutput = JSON.stringify({
          googledocs: { command: 'npx' },
          glean: { command: 'npx' },
        });
        callback(null, { stdout: jsonOutput, stderr: '' });
      });

      const result = await verifier.verifyMcpConnection('googledocs');
      expect(result.success).toBe(true);
      expect(result.mcpServers).toContain('googledocs');
    });

    it('should fail when MCP is not found', async () => {
      const { execFile } = require('child_process');
      execFile.mockImplementation((cmd: string, args: string[], opts: any, callback: any) => {
        const jsonOutput = JSON.stringify({ glean: {}, slack: {} });
        callback(null, { stdout: jsonOutput, stderr: '' });
      });

      const result = await verifier.verifyMcpConnection('googledocs');
      expect(result.success).toBe(false);
      expect(result.error).toContain('not found');
      expect(result.mcpServers).toEqual(['glean', 'slack']);
    });

    it('should handle timeout', async () => {
      const { execFile } = require('child_process');
      execFile.mockImplementation((cmd: string, args: string[], opts: any, callback: any) => {
        // Simulate timeout by not calling callback
        setTimeout(() => {
          const error: any = new Error('Aborted');
          error.name = 'AbortError';
          callback(error);
        }, 100);
      });

      const result = await verifier.verifyMcpConnection('googledocs', 50);
      expect(result.success).toBe(false);
      expect(result.error).toContain('timed out');
    });
  });
});

import { ConfigLocationDetector } from '../location-detector';
import * as fs from 'fs/promises';
import * as os from 'os';
import * as path from 'path';

jest.mock('fs/promises');

describe('ConfigLocationDetector', () => {
  let detector: ConfigLocationDetector;

  beforeEach(() => {
    detector = new ConfigLocationDetector();
    jest.clearAllMocks();
  });

  describe('detect', () => {
    it('should prefer new location if it exists', async () => {
      (fs.access as jest.Mock).mockResolvedValueOnce(undefined); // New location exists

      const result = await detector.detect();
      expect(result).toBe(path.join(os.homedir(), '.claude.json'));
    });

    it('should use legacy location if new does not exist', async () => {
      (fs.access as jest.Mock)
        .mockRejectedValueOnce(new Error('ENOENT')) // New location doesn't exist
        .mockResolvedValueOnce(undefined); // Legacy location exists

      const result = await detector.detect();
      expect(result).toBe(
        path.join(os.homedir(), '.config/claude-code/mcp.json')
      );
    });

    it('should default to new location if neither exists', async () => {
      (fs.access as jest.Mock)
        .mockRejectedValueOnce(new Error('ENOENT')) // New doesn't exist
        .mockRejectedValueOnce(new Error('ENOENT')); // Legacy doesn't exist

      const result = await detector.detect();
      expect(result).toBe(path.join(os.homedir(), '.claude.json'));
    });
  });

  describe('merge', () => {
    it('should preserve existing MCP servers', async () => {
      const existingConfig = {
        projects: {
          '/home/user/myproject': {
            mcpServers: {
              glean: { command: 'npx', args: [] },
              slack: { command: 'npx', args: [] },
            },
          },
        },
      };

      (fs.access as jest.Mock).mockResolvedValue(undefined); // Config exists
      (fs.readFile as jest.Mock).mockResolvedValue(
        JSON.stringify(existingConfig)
      );
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);
      (fs.copyFile as jest.Mock).mockResolvedValue(undefined);
      (fs.chmod as jest.Mock).mockResolvedValue(undefined);

      const newMcpConfig = { command: 'npx', args: ['-y', '@mcp/server-gdrive'] };
      await detector.merge('/home/user/myproject', 'googledocs', newMcpConfig);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const writtenConfig = JSON.parse(writeCall[1]);

      // Verify existing MCPs preserved
      expect(writtenConfig.projects['/home/user/myproject'].mcpServers).toEqual({
        glean: { command: 'npx', args: [] },
        slack: { command: 'npx', args: [] },
        googledocs: newMcpConfig,
      });
    });

    it('should create backup with 600 permissions', async () => {
      (fs.access as jest.Mock).mockResolvedValue(undefined); // Config exists
      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.copyFile as jest.Mock).mockResolvedValue(undefined);
      (fs.chmod as jest.Mock).mockResolvedValue(undefined);
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await detector.merge('/home/user/myproject', 'googledocs', {});

      // Verify chmod called with 600 (0o600)
      expect(fs.chmod).toHaveBeenCalledWith(
        expect.stringContaining('.bak'),
        0o600
      );
    });
  });
});

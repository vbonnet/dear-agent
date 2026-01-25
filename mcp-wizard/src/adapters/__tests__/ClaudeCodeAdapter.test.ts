import { ClaudeCodeAdapter } from '../ClaudeCodeAdapter';
import { MCPServer } from '../PlatformAdapter';
import * as fs from 'fs/promises';

jest.mock('child_process', () => ({
  exec: jest.fn(),
}));
jest.mock('util', () => ({
  promisify: jest.fn((fn) => fn),
}));
jest.mock('fs/promises');

const { exec } = require('child_process');
const mockExec = exec as jest.MockedFunction<any>;

describe('ClaudeCodeAdapter', () => {
  let adapter: ClaudeCodeAdapter;

  beforeEach(() => {
    adapter = new ClaudeCodeAdapter();
    jest.clearAllMocks();
  });

  describe('hasCLI', () => {
    it('should return true when claude command exists', async () => {
      mockExec.mockResolvedValueOnce({ stdout: '/usr/local/bin/claude', stderr: '' });

      const result = await adapter.hasCLI();

      expect(result).toBe(true);
      expect(mockExec).toHaveBeenCalledWith('which claude');
    });

    it('should return false when claude command not found', async () => {
      mockExec.mockRejectedValueOnce(new Error('Command not found'));

      const result = await adapter.hasCLI();

      expect(result).toBe(false);
    });
  });

  describe('configure - CLI path', () => {
    const testServers: MCPServer[] = [
      {
        name: 'GoogleDocs',
        type: 'stdio',
        command: 'node',
        args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
        env: { API_KEY: 'test-key' },
      },
      {
        name: 'Atlassian',
        type: 'http',
        url: 'https://mcp.atlassian.com/v1/sse',
      },
    ];

    beforeEach(() => {
      // Mock hasCLI to return true
      jest.spyOn(adapter as any, 'hasCLI').mockResolvedValue(true);
    });

    it('should use CLI when available', async () => {
      mockExec.mockResolvedValue({ stdout: 'Success', stderr: '' });

      await adapter.configure(testServers);

      expect(mockExec).toHaveBeenCalledTimes(2);
      expect(mockExec).toHaveBeenCalledWith(
        expect.stringContaining('claude mcp add GoogleDocs')
      );
      expect(mockExec).toHaveBeenCalledWith(
        expect.stringContaining('claude mcp add Atlassian')
      );
    });

    it('should build correct CLI command for stdio server', async () => {
      mockExec.mockResolvedValue({ stdout: '', stderr: '' });

      await adapter.configure([testServers[0]]);

      expect(mockExec).toHaveBeenCalledWith(
        'claude mcp add GoogleDocs --transport stdio -e API_KEY=test-key -- node /home/user/mcp-servers/google-docs-mcp/dist/server.js'
      );
    });

    it('should build correct CLI command for HTTP server', async () => {
      mockExec.mockResolvedValue({ stdout: '', stderr: '' });

      await adapter.configure([testServers[1]]);

      expect(mockExec).toHaveBeenCalledWith(
        'claude mcp add Atlassian --transport http https://mcp.atlassian.com/v1/sse'
      );
    });

    it('should fallback to file write if CLI fails', async () => {
      mockExec.mockRejectedValueOnce(new Error('CLI error'));
      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure([testServers[0]]);

      expect(fs.writeFile).toHaveBeenCalled();
    });
  });

  describe('configure - File path', () => {
    const testServer: MCPServer = {
      name: 'GoogleDocs',
      type: 'stdio',
      command: 'node',
      args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
      env: {},
    };

    beforeEach(() => {
      // Mock hasCLI to return false
      jest.spyOn(adapter as any, 'hasCLI').mockResolvedValue(false);
    });

    it('should write to ~/.claude.json when CLI not available', async () => {
      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure([testServer]);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const filePath = writeCall[0];
      const content = JSON.parse(writeCall[1]);

      expect(filePath).toContain('.claude.json');
      expect(content.projects).toBeDefined();
      expect(content.projects[process.cwd()].mcpServers.GoogleDocs).toBeDefined();
    });

    it('should create config structure if file does not exist', async () => {
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure([testServer]);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const content = JSON.parse(writeCall[1]);

      expect(content.projects).toBeDefined();
      expect(content.projects[process.cwd()]).toBeDefined();
    });

    it('should transform stdio server correctly', async () => {
      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure([testServer]);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const content = JSON.parse(writeCall[1]);
      const serverConfig = content.projects[process.cwd()].mcpServers.GoogleDocs;

      expect(serverConfig.type).toBe('stdio');
      expect(serverConfig.command).toBe('node');
      expect(serverConfig.args).toEqual(['/home/user/mcp-servers/google-docs-mcp/dist/server.js']);
    });

    it('should transform HTTP server correctly', async () => {
      const httpServer: MCPServer = {
        name: 'TestHTTP',
        type: 'http',
        url: 'https://example.com',
        headers: { 'Authorization': 'Bearer token' },
      };

      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure([httpServer]);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const content = JSON.parse(writeCall[1]);
      const serverConfig = content.projects[process.cwd()].mcpServers.TestHTTP;

      expect(serverConfig.type).toBe('http');
      expect(serverConfig.url).toBe('https://example.com');
      expect(serverConfig.headers).toEqual({ 'Authorization': 'Bearer token' });
    });
  });
});

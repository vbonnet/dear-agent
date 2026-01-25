import { GeminiAdapter } from '../GeminiAdapter';
import { MCPServer } from '../PlatformAdapter';
import * as fs from 'fs/promises';

jest.mock('fs/promises');

describe('GeminiAdapter', () => {
  let adapter: GeminiAdapter;

  beforeEach(() => {
    adapter = new GeminiAdapter();
    jest.clearAllMocks();
  });

  describe('hasCLI', () => {
    it('should always return false (no CLI commands)', async () => {
      const result = await adapter.hasCLI();

      expect(result).toBe(false);
    });
  });

  describe('configure', () => {
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
        headers: { 'Authorization': 'Bearer token' },
      },
      {
        name: 'SSEServer',
        type: 'sse',
        url: 'https://example.com/sse',
      },
    ];

    it('should write to ~/.gemini/settings.json', async () => {
      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure(testServers);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const filePath = writeCall[0];
      const content = JSON.parse(writeCall[1]);

      expect(filePath).toContain('.gemini/settings.json');
      expect(content.mcpServers).toBeDefined();
      expect(Object.keys(content.mcpServers)).toHaveLength(3);
    });

    it('should create .gemini directory if it does not exist', async () => {
      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure(testServers);

      expect(fs.mkdir).toHaveBeenCalledWith(
        expect.stringContaining('.gemini'),
        { recursive: true }
      );
    });

    it('should create config if file does not exist', async () => {
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure(testServers);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const content = JSON.parse(writeCall[1]);

      expect(content.mcpServers).toBeDefined();
    });

    it('should transform stdio server correctly', async () => {
      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure([testServers[0]]);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const content = JSON.parse(writeCall[1]);
      const serverConfig = content.mcpServers.GoogleDocs;

      expect(serverConfig.command).toBe('node');
      expect(serverConfig.args).toEqual(['/home/user/mcp-servers/google-docs-mcp/dist/server.js']);
      expect(serverConfig.env).toEqual({ API_KEY: 'test-key' });
    });

    it('should transform HTTP server with httpUrl field (not url)', async () => {
      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure([testServers[1]]);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const content = JSON.parse(writeCall[1]);
      const serverConfig = content.mcpServers.Atlassian;

      expect(serverConfig.httpUrl).toBe('https://mcp.atlassian.com/v1/sse');
      expect(serverConfig.url).toBeUndefined(); // Gemini uses httpUrl, not url
      expect(serverConfig.headers).toEqual({ 'Authorization': 'Bearer token' });
    });

    it('should transform SSE server with url field', async () => {
      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      await adapter.configure([testServers[2]]);

      const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
      const content = JSON.parse(writeCall[1]);
      const serverConfig = content.mcpServers.SSEServer;

      expect(serverConfig.url).toBe('https://example.com/sse');
    });

    it('should throw error for unsupported server type', async () => {
      const invalidServer = {
        name: 'Invalid',
        type: 'invalid' as any,
      };

      (fs.readFile as jest.Mock).mockResolvedValue('{}');
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);

      await expect(adapter.configure([invalidServer])).rejects.toThrow(
        'Unsupported server type: invalid'
      );
    });
  });
});

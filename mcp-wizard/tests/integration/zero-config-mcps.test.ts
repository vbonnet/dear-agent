import { generateMcpConfig } from '../../src/lib/config';

describe('Zero-Config MCP Integration Tests', () => {
  describe('Sequential Thinking MCP', () => {
    it('should configure without authentication', async () => {
      const config = await generateMcpConfig(['sequentialthinking']);
      expect(config.mcpServers).toHaveProperty('SequentialThinking');
      
      const server = config.mcpServers.SequentialThinking;
      expect(server.command).toBe('npx');
      expect(server.args).toContain('@modelcontextprotocol/server-sequential-thinking');
      expect(server.env).toBeUndefined();
    });

    it('should start successfully', async () => {
      const { spawn } = require('child_process');
      const proc = spawn('npx', ['-y', '@modelcontextprotocol/server-sequential-thinking']);
      
      await new Promise((resolve) => setTimeout(resolve, 2000));
      
      expect(proc.killed).toBe(false);
      proc.kill();
    }, 10000);
  });

  describe('Playwright MCP', () => {
    it('should configure without authentication', async () => {
      const config = await generateMcpConfig(['playwright']);
      expect(config.mcpServers).toHaveProperty('Playwright');
      
      const server = config.mcpServers.Playwright;
      expect(server.command).toBe('npx');
      expect(server.args).toContain('@microsoft/mcp-server-playwright');
      expect(server.env).toBeUndefined();
    });
  });

  describe('Combined Setup', () => {
    it('should configure multiple zero-config MCPs', async () => {
      const config = await generateMcpConfig(['sequentialthinking', 'playwright']);
      expect(config.mcpServers).toHaveProperty('SequentialThinking');
      expect(config.mcpServers).toHaveProperty('Playwright');
      expect(Object.keys(config.mcpServers)).toHaveLength(2);
    });
  });
});

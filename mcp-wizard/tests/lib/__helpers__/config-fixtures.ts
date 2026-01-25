/**
 * Test fixtures for user-config tests
 */

import { UserConfig } from '../../../src/lib/user-config';

/**
 * Valid configurations
 */
export const VALID_CONFIG_[REDACTED_EMPLOYER]: UserConfig = {
  company: {
    name: '[REDACTED_EMPLOYER]',
    glean_instance: '[REDACTED_EMPLOYER]',
    okta_domain: '[REDACTED_EMPLOYER].okta.com',
  },
};

export const VALID_CONFIG_ACME: UserConfig = {
  company: {
    name: 'Acme Corp',
    glean_instance: 'acme',
    okta_domain: 'acme.okta.com',
  },
};

/**
 * Invalid configurations (for validation testing)
 */
export const INVALID_CONFIG_MISSING_COMPANY: any = {
  // Missing company field entirely
};

export const INVALID_CONFIG_EMPTY_NAME: UserConfig = {
  company: {
    name: '',
    glean_instance: 'test',
    okta_domain: 'test.okta.com',
  },
};

export const INVALID_CONFIG_SPACES_IN_GLEAN: UserConfig = {
  company: {
    name: 'Test',
    glean_instance: 'has spaces',
    okta_domain: 'test.okta.com',
  },
};

export const INVALID_CONFIG_UPPERCASE_GLEAN: UserConfig = {
  company: {
    name: 'Test',
    glean_instance: 'UPPERCASE',
    okta_domain: 'test.okta.com',
  },
};

export const INVALID_CONFIG_BAD_OKTA: UserConfig = {
  company: {
    name: 'Test',
    glean_instance: 'test',
    okta_domain: 'nodomain', // Missing dot
  },
};

export const INVALID_CONFIG_MISSING_GLEAN: any = {
  company: {
    name: 'Test',
    // Missing glean_instance
    okta_domain: 'test.okta.com',
  },
};

export const INVALID_CONFIG_MISSING_OKTA: any = {
  company: {
    name: 'Test',
    glean_instance: 'test',
    // Missing okta_domain
  },
};

/**
 * Malformed data
 */
export const MALFORMED_JSON = '{invalid json}';
export const EMPTY_JSON = '{}';

/**
 * MCP Configuration fixtures
 */
import { McpConfig, AgentInfo } from '../../../src/lib/config';

export const MOCK_MCP_CONFIG_GOOGLEDOCS: McpConfig = {
  mcpServers: {
    GoogleDocs: {
      command: 'node',
      args: ['/home/testuser/mcp-servers/google-docs-mcp/dist/server.js'],
      env: {
        CREDENTIALS_PATH: '/home/testuser/mcp-servers/google-docs-mcp/credentials.json',
        TOKEN_PATH: '/home/testuser/mcp-servers/google-docs-mcp/token.json',
      },
    },
  },
};

export const MOCK_MCP_CONFIG_MULTI: McpConfig = {
  mcpServers: {
    GoogleDocs: {
      command: 'node',
      args: ['/home/testuser/mcp-servers/google-docs-mcp/dist/server.js'],
      env: {
        CREDENTIALS_PATH: '/home/testuser/mcp-servers/google-docs-mcp/credentials.json',
        TOKEN_PATH: '/home/testuser/mcp-servers/google-docs-mcp/token.json',
      },
    },
    Atlassian: {
      command: 'npx',
      args: ['-y', 'mcp-remote@latest', 'https://mcp.atlassian.com/v1/sse', '--auth-timeout', '120'],
    },
  },
};

/**
 * Agent Info fixtures
 */
export const MOCK_AGENT_CLAUDE_CODE_DETECTED: AgentInfo = {
  name: 'Claude Code',
  configPath: '.config/claude-code/mcp.json',
  detected: true,
  description: 'Anthropic Claude CLI tool',
};

export const MOCK_AGENT_CURSOR_NOT_DETECTED: AgentInfo = {
  name: 'Cursor',
  configPath: '.cursor/mcp.json',
  detected: false,
  description: 'AI-powered code editor (also used by Aider)',
};

export const MOCK_AGENT_CLINE_DETECTED: AgentInfo = {
  name: 'Cline',
  configPath: '.cline/mcp.json',
  detected: true,
  description: 'AI coding assistant',
};

export const MOCK_AGENT_WINDSURF_NOT_DETECTED: AgentInfo = {
  name: 'Windsurf',
  configPath: '.codeium/windsurf/mcp.json',
  detected: false,
  description: 'Codeium AI editor',
};

export const MOCK_AGENTS_MULTIPLE: AgentInfo[] = [
  MOCK_AGENT_CLAUDE_CODE_DETECTED,
  MOCK_AGENT_CURSOR_NOT_DETECTED,
  MOCK_AGENT_CLINE_DETECTED,
  MOCK_AGENT_WINDSURF_NOT_DETECTED,
];

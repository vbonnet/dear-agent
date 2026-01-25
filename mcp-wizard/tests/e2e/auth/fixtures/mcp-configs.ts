export interface MCPOAuthConfig {
  name: string;
  clientId: string;
  scopes: string[];
  authEndpoint: string;
  tokenEndpoint: string;
  deviceCodeEndpoint: string;
}

export const googleDocsConfig: MCPOAuthConfig = {
  name: 'googledocs',
  clientId: 'test-googledocs-client-12345',
  scopes: [
    'openid',
    'profile',
    'email',
    'https://www.googleapis.com/auth/documents'
  ],
  authEndpoint: 'http://localhost:8080/authorize',
  tokenEndpoint: 'http://localhost:8080/token',
  deviceCodeEndpoint: 'http://localhost:8080/device/code'
};

export const atlassianConfig: MCPOAuthConfig = {
  name: 'atlassian',
  clientId: 'test-atlassian-client-67890',
  scopes: [
    'read:jira-work',
    'read:confluence-content.all',
    'read:me'
  ],
  authEndpoint: 'http://localhost:8080/authorize',
  tokenEndpoint: 'http://localhost:8080/token',
  deviceCodeEndpoint: 'http://localhost:8080/device/code'
};

export const allMCPs = [googleDocsConfig, atlassianConfig];

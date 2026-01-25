/**
 * MCP Tool Schema Fixtures for Context Broker Tests
 *
 * Represents typical MCP tool schemas from Atlassian and Google Docs MCPs
 */

export const ATLASSIAN_SCHEMAS = [
  {
    name: 'create_jira_issue',
    description: 'Create a new Jira issue',
    inputSchema: {
      type: 'object',
      properties: {
        project: { type: 'string', description: 'Project key' },
        summary: { type: 'string', description: 'Issue summary' },
        description: { type: 'string', description: 'Issue description' },
        issueType: { type: 'string', description: 'Issue type (Bug, Story, Task)' },
      },
      required: ['project', 'summary', 'issueType'],
    },
  },
  {
    name: 'update_jira_issue',
    description: 'Update an existing Jira issue',
    inputSchema: {
      type: 'object',
      properties: {
        issueKey: { type: 'string', description: 'Issue key (e.g., PROJ-123)' },
        summary: { type: 'string', description: 'New summary' },
        description: { type: 'string', description: 'New description' },
        status: { type: 'string', description: 'New status' },
      },
      required: ['issueKey'],
    },
  },
  {
    name: 'get_jira_issue',
    description: 'Get details of a Jira issue',
    inputSchema: {
      type: 'object',
      properties: {
        issueKey: { type: 'string', description: 'Issue key (e.g., PROJ-123)' },
      },
      required: ['issueKey'],
    },
  },
  {
    name: 'list_jira_projects',
    description: 'List all Jira projects',
    inputSchema: {
      type: 'object',
      properties: {},
    },
  },
  {
    name: 'create_confluence_page',
    description: 'Create a new Confluence page',
    inputSchema: {
      type: 'object',
      properties: {
        space: { type: 'string', description: 'Space key' },
        title: { type: 'string', description: 'Page title' },
        content: { type: 'string', description: 'Page content (HTML or wiki markup)' },
      },
      required: ['space', 'title', 'content'],
    },
  },
  {
    name: 'update_confluence_page',
    description: 'Update an existing Confluence page',
    inputSchema: {
      type: 'object',
      properties: {
        pageId: { type: 'string', description: 'Page ID' },
        title: { type: 'string', description: 'New title' },
        content: { type: 'string', description: 'New content' },
      },
      required: ['pageId'],
    },
  },
];

export const GOOGLEDOCS_SCHEMAS = [
  {
    name: 'create_google_doc',
    description: 'Create a new Google Doc',
    inputSchema: {
      type: 'object',
      properties: {
        title: { type: 'string', description: 'Document title' },
        content: { type: 'string', description: 'Initial content' },
      },
      required: ['title'],
    },
  },
  {
    name: 'update_google_doc',
    description: 'Update an existing Google Doc',
    inputSchema: {
      type: 'object',
      properties: {
        documentId: { type: 'string', description: 'Document ID' },
        content: { type: 'string', description: 'New content' },
      },
      required: ['documentId', 'content'],
    },
  },
  {
    name: 'read_google_doc',
    description: 'Read content from a Google Doc',
    inputSchema: {
      type: 'object',
      properties: {
        documentId: { type: 'string', description: 'Document ID' },
      },
      required: ['documentId'],
    },
  },
  {
    name: 'list_google_docs',
    description: 'List all Google Docs',
    inputSchema: {
      type: 'object',
      properties: {
        maxResults: { type: 'number', description: 'Maximum results to return' },
      },
    },
  },
  {
    name: 'share_google_doc',
    description: 'Share a Google Doc with users',
    inputSchema: {
      type: 'object',
      properties: {
        documentId: { type: 'string', description: 'Document ID' },
        email: { type: 'string', description: 'Email to share with' },
        role: { type: 'string', description: 'Permission role (reader, writer, owner)' },
      },
      required: ['documentId', 'email'],
    },
  },
];

export const SLACK_SCHEMAS = [
  {
    name: 'send_slack_message',
    description: 'Send a message to a Slack channel',
    inputSchema: {
      type: 'object',
      properties: {
        channel: { type: 'string', description: 'Channel ID or name' },
        text: { type: 'string', description: 'Message text' },
      },
      required: ['channel', 'text'],
    },
  },
  {
    name: 'list_slack_channels',
    description: 'List all Slack channels',
    inputSchema: {
      type: 'object',
      properties: {},
    },
  },
];

export const ALL_SCHEMAS = [
  ...ATLASSIAN_SCHEMAS,
  ...GOOGLEDOCS_SCHEMAS,
  ...SLACK_SCHEMAS,
];

/**
 * Schema registry mapping service names to their schemas
 */
export const SCHEMA_REGISTRY = {
  atlassian: ATLASSIAN_SCHEMAS,
  googledocs: GOOGLEDOCS_SCHEMAS,
  slack: SLACK_SCHEMAS,
};

/**
 * Unit tests for chezmoi-manager module
 */

import { exec } from 'child_process';
import { promises as fs } from 'fs';
import {
  detectChezmoi,
  getTemplateFilePath,
  writeChezmoiTemplate,
  showChezmoiDiff,
  applyChezmoiConfig,
  automateChezmoiSetup,
} from '../../../src/lib/chezmoi-manager';

// Mock child_process and fs
jest.mock('child_process');
jest.mock('fs', () => ({
  promises: {
    access: jest.fn(),
    mkdir: jest.fn(),
    writeFile: jest.fn(),
  },
}));

// Mock console.log to suppress output during tests
const originalLog = console.log;
beforeAll(() => {
  console.log = jest.fn();
});
afterAll(() => {
  console.log = originalLog;
});

describe('detectChezmoi', () => {
  const mockExec = exec as unknown as jest.Mock;

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('detects chezmoi when installed and initialized', async () => {
    // Mock `which chezmoi` success
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/usr/bin/chezmoi', stderr: '' });
    });

    // Mock `chezmoi source-path` success
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/home/user/.local/share/chezmoi\n', stderr: '' });
    });

    // Mock fs.access success
    (fs.access as jest.Mock).mockResolvedValueOnce(undefined);

    const result = await detectChezmoi();

    expect(result.detected).toBe(true);
    expect(result.sourcePath).toBe('/home/user/.local/share/chezmoi');
    expect(result.reason).toBeUndefined();
  });

  test('returns not installed when which chezmoi fails', async () => {
    // Mock `which chezmoi` failure
    const error = new Error('Command not found');
    (error as any).code = 'ENOENT';
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(error, null);
    });

    const result = await detectChezmoi();

    expect(result.detected).toBe(false);
    expect(result.reason).toBe('not installed');
  });

  test('returns not initialized when source-path fails', async () => {
    // Mock `which chezmoi` success
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/usr/bin/chezmoi', stderr: '' });
    });

    // Mock `chezmoi source-path` failure
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(new Error('not initialized'), null);
    });

    const result = await detectChezmoi();

    expect(result.detected).toBe(false);
    expect(result.reason).toBe('not initialized');
  });

  test('returns permission denied on EACCES error', async () => {
    // Mock `which chezmoi` success
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/usr/bin/chezmoi', stderr: '' });
    });

    // Mock `chezmoi source-path` with permission error
    const error = new Error('Permission denied');
    (error as any).code = 'EACCES';
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(error, null);
    });

    const result = await detectChezmoi();

    expect(result.detected).toBe(false);
    expect(result.reason).toBe('permission denied');
  });
});

describe('getTemplateFilePath', () => {
  test('generates correct path for claude-code', () => {
    const sourcePath = '/home/user/.local/share/chezmoi';
    const path = getTemplateFilePath(sourcePath, 'claude-code');

    expect(path).toBe('/home/user/.local/share/chezmoi/dot_config/claude-code/private_mcp.json.tmpl');
  });

  test('generates correct path for cursor', () => {
    const sourcePath = '/home/user/.local/share/chezmoi';
    const path = getTemplateFilePath(sourcePath, 'cursor');

    expect(path).toBe('/home/user/.local/share/chezmoi/dot_cursor/private_mcp.json.tmpl');
  });

  test('generates correct path for cline', () => {
    const sourcePath = '/home/user/.local/share/chezmoi';
    const path = getTemplateFilePath(sourcePath, 'cline');

    expect(path).toBe('/home/user/.local/share/chezmoi/dot_cline/private_mcp.json.tmpl');
  });

  test('generates correct path for windsurf', () => {
    const sourcePath = '/home/user/.local/share/chezmoi';
    const path = getTemplateFilePath(sourcePath, 'windsurf');

    expect(path).toBe('/home/user/.local/share/chezmoi/dot_codeium/windsurf/private_mcp.json.tmpl');
  });

  test('throws error for unsupported agent', () => {
    const sourcePath = '/home/user/.local/share/chezmoi';

    expect(() => getTemplateFilePath(sourcePath, 'invalid-agent')).toThrow('Unsupported agent');
  });

  test('throws error for path traversal attempt', () => {
    const sourcePath = '/home/user/.local/share/chezmoi';

    // This will fail validatePath due to '..'
    expect(() => getTemplateFilePath(sourcePath + '/../evil', 'claude-code')).toThrow();
  });
});

describe('writeChezmoiTemplate', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('creates template file with correct syntax', async () => {
    const config = {
      mcpServers: {
        GoogleDocs: {
          command: 'node',
          args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
        },
      },
    };

    (fs.mkdir as jest.Mock).mockResolvedValueOnce(undefined);
    (fs.writeFile as jest.Mock).mockResolvedValueOnce(undefined);

    await writeChezmoiTemplate(config, '/home/user/.local/share/chezmoi');

    expect(fs.mkdir).toHaveBeenCalledWith(
      expect.stringContaining('dot_config/claude-code'),
      { recursive: true }
    );

    expect(fs.writeFile).toHaveBeenCalledWith(
      expect.stringContaining('private_mcp.json.tmpl'),
      expect.stringContaining('{{- if hasSuffix "-w"'),
      'utf8'
    );

    // Verify template contains config JSON
    const writeCall = (fs.writeFile as jest.Mock).mock.calls[0];
    const templateContent = writeCall[1];
    expect(templateContent).toContain('"mcpServers"');
    expect(templateContent).toContain('"GoogleDocs"');
    expect(templateContent).toContain('{{- else }}');
    expect(templateContent).toContain('{{- end }}');
  });

  test('creates parent directories if needed', async () => {
    const config = { mcpServers: {} };

    (fs.mkdir as jest.Mock).mockResolvedValueOnce(undefined);
    (fs.writeFile as jest.Mock).mockResolvedValueOnce(undefined);

    await writeChezmoiTemplate(config, '/home/user/.local/share/chezmoi');

    expect(fs.mkdir).toHaveBeenCalledWith(
      expect.any(String),
      { recursive: true }
    );
  });
});

describe('showChezmoiDiff', () => {
  const mockExec = exec as unknown as jest.Mock;

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('returns diff when changes exist', async () => {
    const mockDiff = '+  "GoogleDocs": {...}\n-  "OldServer": {...}';

    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: mockDiff, stderr: '' });
    });

    const diff = await showChezmoiDiff('/home/user/.config/claude-code/mcp.json');

    expect(diff).toContain('+  "GoogleDocs"');
    expect(diff).toContain('-  "OldServer"');
  });

  test('returns "No changes detected" when output empty', async () => {
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '', stderr: '' });
    });

    const diff = await showChezmoiDiff('/home/user/.config/claude-code/mcp.json');

    expect(diff).toBe('No changes detected');
  });

  test('returns diff from error.stdout (exit code 1 with output)', async () => {
    const mockDiff = '+  "NewServer": {...}';
    const error: any = new Error('exit 1');
    error.stdout = mockDiff;
    error.stderr = '';

    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(error, null);
    });

    const diff = await showChezmoiDiff('/home/user/.config/claude-code/mcp.json');

    expect(diff).toContain('+  "NewServer"');
  });

  test('returns error message on actual failure', async () => {
    const error = new Error('File not found');

    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(error, null);
    });

    const diff = await showChezmoiDiff('/home/user/.config/claude-code/mcp.json');

    expect(diff).toContain('Error running diff');
    expect(diff).toContain('File not found');
  });
});

describe('applyChezmoiConfig', () => {
  const mockExec = exec as unknown as jest.Mock;

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('returns success on successful apply', async () => {
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: 'applied mcp.json\n', stderr: '' });
    });

    const result = await applyChezmoiConfig('/home/user/.config/claude-code/mcp.json');

    expect(result.success).toBe(true);
    expect(result.output).toContain('applied mcp.json');
    expect(result.error).toBeUndefined();
  });

  test('returns failure on apply error', async () => {
    const error: any = new Error('Permission denied');
    error.stdout = '';
    error.stderr = 'chezmoi: permission denied\n';

    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(error, null);
    });

    const result = await applyChezmoiConfig('/home/user/.config/claude-code/mcp.json');

    expect(result.success).toBe(false);
    expect(result.error).toContain('permission denied');
  });

  test('captures stdout and stderr', async () => {
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, {
        stdout: 'stdout output\n',
        stderr: 'stderr output\n'
      });
    });

    const result = await applyChezmoiConfig('/home/user/.config/claude-code/mcp.json');

    expect(result.output).toContain('stdout output');
    expect(result.output).toContain('stderr output');
  });
});

describe('automateChezmoiSetup', () => {
  const mockExec = exec as unknown as jest.Mock;

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('full success path returns automated method', async () => {
    const config = {
      mcpServers: {
        GoogleDocs: {
          command: 'node',
          args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
        },
      },
    };

    // Mock detectChezmoi (which + source-path + access)
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/usr/bin/chezmoi', stderr: '' });
    });
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/home/user/.local/share/chezmoi\n', stderr: '' });
    });
    (fs.access as jest.Mock).mockResolvedValueOnce(undefined);

    // Mock writeChezmoiTemplate (mkdir + writeFile)
    (fs.mkdir as jest.Mock).mockResolvedValueOnce(undefined);
    (fs.writeFile as jest.Mock).mockResolvedValueOnce(undefined);

    // Mock applyChezmoiConfig
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: 'applied\n', stderr: '' });
    });

    const result = await automateChezmoiSetup(config);

    expect(result.method).toBe('automated');
    expect(result.result?.success).toBe(true);
    expect(result.error).toBeUndefined();
  });

  test('detection failure returns manual with reason', async () => {
    const config = { mcpServers: {} };

    // Mock detectChezmoi failure (which fails)
    const error: any = new Error('not found');
    error.code = 'ENOENT';
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(error, null);
    });

    const result = await automateChezmoiSetup(config);

    expect(result.method).toBe('manual');
    expect(result.error).toBe('not installed');
  });

  test('apply failure returns manual', async () => {
    const config = { mcpServers: {} };

    // Mock detectChezmoi success
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/usr/bin/chezmoi', stderr: '' });
    });
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/home/user/.local/share/chezmoi\n', stderr: '' });
    });
    (fs.access as jest.Mock).mockResolvedValueOnce(undefined);

    // Mock writeChezmoiTemplate success
    (fs.mkdir as jest.Mock).mockResolvedValueOnce(undefined);
    (fs.writeFile as jest.Mock).mockResolvedValueOnce(undefined);

    // Mock applyChezmoiConfig failure
    const applyError: any = new Error('apply failed');
    applyError.stderr = 'permission denied';
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(applyError, null);
    });

    const result = await automateChezmoiSetup(config);

    expect(result.method).toBe('manual');
    expect(result.error).toBeDefined();
  });

  test('shows diff when showDiff option enabled', async () => {
    const config = { mcpServers: {} };

    // Mock detectChezmoi success
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/usr/bin/chezmoi', stderr: '' });
    });
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '/home/user/.local/share/chezmoi\n', stderr: '' });
    });
    (fs.access as jest.Mock).mockResolvedValueOnce(undefined);

    // Mock writeChezmoiTemplate success
    (fs.mkdir as jest.Mock).mockResolvedValueOnce(undefined);
    (fs.writeFile as jest.Mock).mockResolvedValueOnce(undefined);

    // Mock showChezmoiDiff
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: '+added line\n', stderr: '' });
    });

    // Mock applyChezmoiConfig success
    mockExec.mockImplementationOnce((cmd: string, _opts: any, callback: any) => {
      callback(null, { stdout: 'applied\n', stderr: '' });
    });

    const result = await automateChezmoiSetup(config, 'claude-code', { showDiff: true });

    expect(result.method).toBe('automated');
    // Verify diff was called: which, source-path, diff, apply
    expect(mockExec).toHaveBeenCalledTimes(4);
  });
});

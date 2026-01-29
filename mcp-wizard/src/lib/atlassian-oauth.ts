import http from 'http';
import { ChildProcess } from 'child_process';

export interface OAuthCallbackResult {
  code: string;
  state: string;
}

/**
 * Extract the callback port from an OAuth authorization URL
 */
export function extractCallbackPortFromUrl(authUrl: string): number | null {
  try {
    const url = new URL(authUrl);
    const redirectUri = url.searchParams.get('redirect_uri');
    if (!redirectUri) return null;
    
    const redirectUrl = new URL(redirectUri);
    return redirectUrl.port ? parseInt(redirectUrl.port, 10) : null;
  } catch {
    return null;
  }
}

/**
 * Generate a gcloud tunnel command for SSH environments
 */
export function generateGcloudTunnelCommand(
  hostname: string,
  port: number,
  project: string,
  region = 'us-central1',
  cluster = 'shared-workstations-cluster',
  config = 'eng'
): string {
  const parts = [
    'gcloud workstations start-tcp-tunnel',
    hostname,
    String(port),
    `--local-host-port=localhost:${port}`,
    `--cluster=${cluster}`,
    `--config=${config}`,
    `--region=${region}`,
    `--project=${project}`,
    '&'
  ];
  return parts.join(' ');
}

/**
 * Start an HTTP server to receive OAuth callback
 */
export async function startOAuthCallbackServer(
  port: number,
  expectedState: string,
  timeoutSeconds = 300
): Promise<OAuthCallbackResult> {
  return new Promise((resolve, reject) => {
    let serverClosed = false;
    let timeoutHandle: NodeJS.Timeout;

    const server = http.createServer((req, res) => {
      if (serverClosed) return;
      
      if (!req.url || !req.url.startsWith('/callback')) {
        res.writeHead(404);
        res.end('<h1>404</h1>');
        return;
      }

      const url = new URL(req.url, `http://localhost:${port}`);
      const code = url.searchParams.get('code');
      const state = url.searchParams.get('state');

      if (!code) {
        serverClosed = true;
        clearTimeout(timeoutHandle);
        server.close();
        res.writeHead(400);
        res.end('<h1>Error: Missing code</h1>');
        reject(new Error('Missing code'));
        return;
      }

      if (state !== expectedState) {
        serverClosed = true;
        clearTimeout(timeoutHandle);
        server.close();
        res.writeHead(400);
        res.end('<h1>Error: State mismatch</h1>');
        reject(new Error('State mismatch'));
        return;
      }

      serverClosed = true;
      clearTimeout(timeoutHandle);
      res.writeHead(200, { 'Content-Type': 'text/html' });
      res.end('<html><body><h1>Success!</h1><p>You can close this window and return to Claude Code.</p></body></html>');
      server.close();
      resolve({ code, state });
    });

    server.listen(port, 'localhost', () => {
      // Server started
    });

    timeoutHandle = setTimeout(() => {
      if (!serverClosed) {
        serverClosed = true;
        server.close();
        reject(new Error(`Timeout after ${timeoutSeconds}s`));
      }
    }, timeoutSeconds * 1000);
  });
}

/**
 * Start a gcloud tunnel process
 */
export async function startGcloudTunnel(
  hostname: string,
  port: number,
  project: string,
  region: string,
  cluster: string,
  config: string
): Promise<ChildProcess> {
  const { spawn } = require('child_process');
  
  const proc = spawn('gcloud', [
    'workstations',
    'start-tcp-tunnel',
    hostname,
    String(port),
    `--local-host-port=localhost:${port}`,
    `--cluster=${cluster}`,
    `--config=${config}`,
    `--region=${region}`,
    `--project=${project}`
  ], {
    detached: true,
    stdio: 'ignore'
  });

  proc.unref();
  
  return proc;
}

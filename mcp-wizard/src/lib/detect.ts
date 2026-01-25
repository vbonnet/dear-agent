import * as os from 'os';
import * as path from 'path';
import { promises as fs } from 'fs';

export interface EnvironmentInfo {
  hostname: string;
  isWorkMachine: boolean;
  nodeVersion: string;
  nodeVersionValid: boolean;
  chezmoiDetected: boolean;
  chezmoiManagesConfig: boolean;
}

export interface ChezmoiStatus {
  isInstalled: boolean;
  managesConfig: boolean;
  templatePath: string;
}

export async function detectEnvironment(): Promise<EnvironmentInfo> {
  const hostname = os.hostname();
  const nodeVersion = process.version;
  const chezmoiStatus = await detectChezmoi();

  return {
    hostname,
    isWorkMachine: hostname.endsWith('-w'),
    nodeVersion,
    nodeVersionValid: await validateNodeVersion(nodeVersion),
    chezmoiDetected: chezmoiStatus.isInstalled,
    chezmoiManagesConfig: chezmoiStatus.managesConfig,
  };
}

export async function detectChezmoi(): Promise<ChezmoiStatus> {
  const homedir = os.homedir();
  const chezmoiBin = path.join(homedir, 'bin/chezmoi');
  const chezmoiTemplate = path.join(
    homedir,
    '.local/share/chezmoi/dot_config/claude-code/private_mcp.json.tmpl'
  );

  return {
    isInstalled: await pathExists(chezmoiBin),
    managesConfig: await pathExists(chezmoiTemplate),
    templatePath: chezmoiTemplate,
  };
}

export async function validateNodeVersion(version: string): Promise<boolean> {
  // Remove 'v' prefix if present
  const cleanVersion = version.startsWith('v') ? version.slice(1) : version;
  const [major] = cleanVersion.split('.').map(Number);

  // Require Node.js >= 18.0.0
  return major >= 18;
}

export function detectSudo(): boolean {
  return process.env.SUDO_USER !== undefined || process.getuid?.() === 0;
}

export async function pathExists(filePath: string): Promise<boolean> {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

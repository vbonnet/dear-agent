/**
 * Chezmoi Detector - detects if config is managed by chezmoi and provides warnings
 */

import { execSync } from 'child_process';
import { ChezmoiStatus } from './types';
import { getConfigPath } from './config-manager';

/**
 * Generate chezmoi warning message with instructions
 */
export function generateChezmoiWarning(): string {
  const configPath = getConfigPath();

  return `
⚠️  CHEZMOI DETECTED

Your config appears to be managed by chezmoi.
After migration, run:

    chezmoi edit ${configPath}
    chezmoi apply

Otherwise chezmoi will overwrite your migration!
  `.trim();
}

/**
 * Detect if config is managed by chezmoi
 */
export function detectChezmoi(): ChezmoiStatus {
  const configPath = getConfigPath();

  // Check 1: Is config path in chezmoi source directory?
  const inChezmoiSource = configPath.includes('.local/share/chezmoi/');

  // Check 2: Does chezmoi executable exist?
  let chezmoiExists = false;
  try {
    execSync('which chezmoi', { stdio: 'ignore' });
    chezmoiExists = true;
  } catch {
    // chezmoi not found (expected on systems without chezmoi)
  }

  const detected = inChezmoiSource || chezmoiExists;

  return {
    detected,
    message: detected ? generateChezmoiWarning() : undefined
  };
}

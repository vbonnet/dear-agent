import { getAdcPath } from '../gcp-setup';
import * as os from 'os';
import * as path from 'path';

describe('getAdcPath', () => {
  it('should return correct path for Linux/macOS', () => {
    // Mock process.platform
    const originalPlatform = process.platform;
    Object.defineProperty(process, 'platform', { value: 'linux' });

    const expected = path.join(
      os.homedir(),
      '.config',
      'gcloud',
      'application_default_credentials.json'
    );

    expect(getAdcPath()).toBe(expected);

    // Restore
    Object.defineProperty(process, 'platform', { value: originalPlatform });
  });

  it('should return correct path for Windows', () => {
    const originalPlatform = process.platform;
    Object.defineProperty(process, 'platform', { value: 'win32' });

    const appData = process.env.APPDATA || path.join(os.homedir(), 'AppData', 'Roaming');
    const expected = path.join(appData, 'gcloud', 'application_default_credentials.json');

    expect(getAdcPath()).toBe(expected);

    Object.defineProperty(process, 'platform', { value: originalPlatform });
  });
});

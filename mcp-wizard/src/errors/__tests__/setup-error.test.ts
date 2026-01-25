import { SetupError } from '../setup-error';

describe('SetupError', () => {
  it('should format message correctly', () => {
    const error = new SetupError(
      'gcloud not authenticated',
      'Run: gcloud auth application-default login',
      'https://github.com/your-org/mcp-wizard/docs#gcloud-auth'
    );

    expect(error.message).toContain('gcloud not authenticated');
    expect(error.message).toContain('Fix: Run: gcloud auth application-default login');
    expect(error.message).toContain('Help: https://github.com/your-org/mcp-wizard/docs#gcloud-auth');
  });

  it('should use default help link', () => {
    const error = new SetupError('problem', 'fix');
    expect(error.message).toContain('Help: https://github.com/your-org/mcp-wizard/issues');
  });

  it('should have correct name', () => {
    const error = new SetupError('problem', 'fix');
    expect(error.name).toBe('SetupError');
  });
});

import { ProgressTracker } from '../progress-tracker';
import ora from 'ora';

jest.mock('ora');

describe('ProgressTracker', () => {
  let tracker: ProgressTracker;
  let mockSpinner: any;

  beforeEach(() => {
    tracker = new ProgressTracker();
    mockSpinner = {
      start: jest.fn().mockReturnThis(),
      succeed: jest.fn(),
      fail: jest.fn(),
      text: '',
    };
    (ora as jest.Mock).mockReturnValue(mockSpinner);
    jest.clearAllMocks();
  });

  it('should start step with correct format', () => {
    tracker.startStep(2, 5, 'Installing Google Docs MCP', '~2-3 min');

    expect(ora).toHaveBeenCalledWith(
      '[2/5] Installing Google Docs MCP (~2-3 min)...'
    );
    expect(mockSpinner.start).toHaveBeenCalled();
  });

  it('should update progress message', () => {
    tracker.startStep(1, 3, 'Processing');
    tracker.updateProgress('Processing... (50%)');

    expect(mockSpinner.text).toBe('Processing... (50%)');
  });

  it('should complete step successfully', () => {
    tracker.startStep(1, 3, 'Processing');
    tracker.completeStep();

    expect(mockSpinner.succeed).toHaveBeenCalled();
  });

  it('should fail step with error message', () => {
    tracker.startStep(1, 3, 'Processing');
    tracker.failStep('Operation failed');

    expect(mockSpinner.fail).toHaveBeenCalledWith('Operation failed');
  });
});

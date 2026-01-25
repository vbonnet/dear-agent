import ora, { Ora } from 'ora';

export class ProgressTracker {
  private spinner: Ora | null = null;

  startStep(step: number, total: number, message: string, estimate?: string): void {
    const prefix = `[${step}/${total}]`;
    const suffix = estimate ? ` (${estimate})` : '';
    this.spinner = ora(`${prefix} ${message}${suffix}...`).start();
  }

  updateProgress(message: string): void {
    if (this.spinner) {
      this.spinner.text = message;
    }
  }

  completeStep(): void {
    if (this.spinner) {
      this.spinner.succeed();
      this.spinner = null;
    }
  }

  failStep(error: string): void {
    if (this.spinner) {
      this.spinner.fail(error);
      this.spinner = null;
    }
  }
}

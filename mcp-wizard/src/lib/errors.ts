function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

export async function retryWithBackoff<T>(
  fn: () => Promise<T>,
  maxRetries: number,
  initialBackoffMs: number
): Promise<T> {
  let backoffMs = initialBackoffMs;

  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      return await fn();
    } catch (error) {
      if (attempt === maxRetries) {
        throw error; // Final attempt failed
      }

      console.log(
        `Retrying in ${backoffMs}ms... (attempt ${attempt + 1} of ${maxRetries})`
      );
      await sleep(backoffMs);
      backoffMs *= 2; // Exponential backoff
    }
  }

  throw new Error('Unreachable');
}

export async function promptWithTimeout(
  message: string,
  timeoutMs: number,
  promptFn: (message: string) => Promise<string>
): Promise<string> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      reject(new Error(`Timeout waiting for user input (${timeoutMs / 1000}s)`));
    }, timeoutMs);

    promptFn(message)
      .then((answer) => {
        clearTimeout(timeout);
        resolve(answer);
      })
      .catch(reject);
  });
}

export function sanitizeError(error: Error): Error {
  // Redact sensitive data (tokens, credentials)
  let message = error.message;

  // Redact tokens (anything that looks like a token)
  message = message.replace(/GOCSPX-[a-zA-Z0-9_-]+/g, '[REDACTED]');
  message = message.replace(/1\/\/[a-zA-Z0-9_-]+/g, '[REDACTED]');
  message = message.replace(/ya29\.[a-zA-Z0-9_-]+/g, '[REDACTED]');

  return new Error(message);
}

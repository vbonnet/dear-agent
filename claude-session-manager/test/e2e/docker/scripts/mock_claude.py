#!/usr/bin/env python3
"""
Mock Claude process for reaper E2E testing.

Simulates Claude Code behavior:
1. Shows ready prompt (>)
2. Waits for /exit command
3. Exits cleanly when /exit received
"""

import sys
import time

def main():
    print("Mock Claude Code v1.0 (E2E Test)")
    print("Type /exit to quit")
    print()

    # Show ready prompt (reaper detects this)
    sys.stdout.write("> ")
    sys.stdout.flush()

    try:
        while True:
            line = sys.stdin.readline()

            if not line:  # EOF
                break

            line = line.strip()

            if line == "/exit":
                print()
                print("Goodbye!")
                sys.exit(0)
            elif line:
                print(f"Unknown command: {line}")
                print("Type /exit to quit")
                sys.stdout.write("> ")
                sys.stdout.flush()
    except KeyboardInterrupt:
        print()
        print("Interrupted")
        sys.exit(1)

if __name__ == "__main__":
    main()

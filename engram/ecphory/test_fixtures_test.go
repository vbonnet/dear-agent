package ecphory

import (
	"os"
	"path/filepath"
	"testing"
)

// reflectionFixture describes a single test reflection file.
type reflectionFixture struct {
	Filename string
	Category string // error_category value
	Content  string // full file content (YAML frontmatter + body)
}

// allReflectionFixtures returns the 10 test reflection fixtures (2 per category).
func allReflectionFixtures() []reflectionFixture {
	return []reflectionFixture{
		// syntax_error (2)
		{
			Filename: "syntax-error-missing-bracket.ai.md",
			Category: "syntax_error",
			Content: `---
type: strategy
title: 'Reflection: Missing Bracket Syntax Error'
description: Debugging a missing closing bracket in Go code
tags:
  - go
  - syntax
  - debugging
error_category: syntax_error
outcome: failure
---

# Missing Bracket Syntax Error

error_category: syntax_error

## Problem

Missing closing bracket caused compilation failure in Go service.

## Resolution

Added the missing bracket and ran go vet to catch similar issues.
`,
		},
		{
			Filename: "syntax-error-json-parsing.ai.md",
			Category: "syntax_error",
			Content: `---
type: strategy
title: 'Reflection: JSON Parsing Syntax Error'
description: Debugging a JSON parsing error due to malformed input
tags:
  - json
  - parsing
  - debugging
error_category: syntax_error
outcome: failure
---

# JSON Parsing Syntax Error

error_category: syntax_error

## Problem

Malformed JSON input caused parsing failure in API handler.

## Resolution

Added input validation before JSON parsing.
`,
		},
		// permission_denied (2)
		{
			Filename: "permission-denied-file-access.ai.md",
			Category: "permission_denied",
			Content: `---
type: strategy
title: 'Reflection: Permission Denied File Access'
description: Debugging permission denied when reading config file
tags:
  - permissions
  - filesystem
  - debugging
error_category: permission_denied
outcome: failure
---

# Permission Denied File Access

error_category: permission_denied

## Problem

Process lacked read permissions on configuration file.

## Resolution

Fixed file permissions and documented required access levels.
`,
		},
		{
			Filename: "permission-denied-network-port.ai.md",
			Category: "permission_denied",
			Content: `---
type: strategy
title: 'Reflection: Permission Denied Network Port'
description: Debugging permission denied when binding to privileged port
tags:
  - permissions
  - network
  - debugging
error_category: permission_denied
outcome: failure
---

# Permission Denied Network Port

error_category: permission_denied

## Problem

Service could not bind to port 443 without elevated privileges.

## Resolution

Used reverse proxy on non-privileged port instead.
`,
		},
		// timeout (2)
		{
			Filename: "timeout-database-query.ai.md",
			Category: "timeout",
			Content: `---
type: strategy
title: 'Reflection: Database Query Timeout'
description: Debugging a slow database query that timed out
tags:
  - database
  - performance
  - debugging
error_category: timeout
outcome: failure
---

# Database Query Timeout

error_category: timeout

## Problem

Complex JOIN query exceeded 30-second timeout on large dataset.

## Resolution

Added database index and optimized query plan.
`,
		},
		{
			Filename: "timeout-http-request.ai.md",
			Category: "timeout",
			Content: `---
type: strategy
title: 'Reflection: HTTP Request Timeout'
description: Debugging HTTP request timeout to external API
tags:
  - http
  - api
  - debugging
error_category: timeout
outcome: failure
---

# HTTP Request Timeout

error_category: timeout

## Problem

External API call timed out during peak traffic.

## Resolution

Added retry logic with exponential backoff and circuit breaker.
`,
		},
		// tool_misuse (2)
		{
			Filename: "tool-misuse-wrong-git-branch.ai.md",
			Category: "tool_misuse",
			Content: `---
type: strategy
title: 'Reflection: Wrong Git Branch Tool Misuse'
description: Committed changes to wrong git branch
tags:
  - git
  - workflow
  - debugging
error_category: tool_misuse
outcome: failure
---

# Wrong Git Branch Tool Misuse

error_category: tool_misuse

## Problem

Changes were committed to main instead of feature branch.

## Resolution

Cherry-picked commits to correct branch and reset main.
`,
		},
		{
			Filename: "tool-misuse-wrong-api-endpoint.ai.md",
			Category: "tool_misuse",
			Content: `---
type: strategy
title: 'Reflection: Wrong API Endpoint Tool Misuse'
description: Used wrong API endpoint causing data corruption
tags:
  - api
  - tools
  - debugging
error_category: tool_misuse
outcome: failure
---

# Wrong API Endpoint Tool Misuse

error_category: tool_misuse

## Problem

Used DELETE endpoint instead of PATCH, causing data loss.

## Resolution

Added API client wrapper with safety checks for destructive operations.
`,
		},
		// other (2)
		{
			Filename: "other-error-nil-pointer.ai.md",
			Category: "other",
			Content: `---
type: strategy
title: 'Reflection: Nil Pointer Dereference'
description: Debugging nil pointer dereference in production
tags:
  - go
  - runtime
  - debugging
error_category: other
outcome: failure
---

# Nil Pointer Dereference

error_category: other

## Problem

Nil pointer dereference panic in production due to uninitialized struct field.

## Resolution

Added nil checks and used constructor functions to ensure initialization.
`,
		},
		{
			Filename: "other-error-race-condition.ai.md",
			Category: "other",
			Content: `---
type: strategy
title: 'Reflection: Race Condition Error'
description: Debugging intermittent race condition in concurrent code
tags:
  - go
  - concurrency
  - debugging
error_category: other
outcome: failure
---

# Race Condition Error

error_category: other

## Problem

Intermittent data corruption due to unsynchronized map access.

## Resolution

Added mutex protection and ran tests with -race flag.
`,
		},
	}
}

// setupReflectionFixtures writes all 10 reflection fixtures to a temp directory
// and returns the directory path. The directory is cleaned up when the test ends.
func setupReflectionFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	for _, f := range allReflectionFixtures() {
		path := filepath.Join(dir, f.Filename)
		if err := os.WriteFile(path, []byte(f.Content), 0644); err != nil {
			t.Fatalf("failed to write fixture %s: %v", f.Filename, err)
		}
	}

	return dir
}

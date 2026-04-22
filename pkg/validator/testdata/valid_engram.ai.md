---
type: reference
title: Valid Engram Test
description: A valid engram file for testing
tags: [test]
---

# Valid Engram

This is a valid engram file with proper structure.

## Principle: Context Embedding

Always embed full context in prompts without references to previous sections.

**Good Example**:
```
Use the Repository Pattern to implement the user service: separate data access logic
into repositories with interfaces (IUserRepository) and concrete implementations
(UserRepository that extends IUserRepository).
```

## Task: Create Authentication

Create JWT authentication with the following constraints:

**Constraints**:
- Scope: Only modify auth.service.ts (max 1 file)
- Token budget: <2000 tokens
- Time bound: Complete in single response
- Dependencies: Use existing jsonwebtoken library

**Success Criteria**:
- [ ] Tests pass with 80%+ coverage
- [ ] Tokens expire after 24 hours
- [ ] Response time <100ms

This engram demonstrates proper use of constraints and examples.

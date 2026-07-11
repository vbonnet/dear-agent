# Engram Plugin Security Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/security` validates plugin permissions, protects provider
credentials, and constructs operating-system sandbox boundaries.

## EARS Requirements

**ESE-01** When a supported provider key is requested, the system shall read Anthropic, OpenAI, Gemini or Google, and OpenRouter credentials only from their documented environment variables.

**ESE-02** When no supported provider credential is configured or the provider is unknown, the system shall return an error without inventing a credential.

**ESE-03** When configuration text contains a supported provider key name or recognizable credential prefix, the system shall reject the configuration so secrets remain outside files.

**ESE-04** When log text contains a configured or recognizable supported-provider credential, the system shall redact the secret wherever it appears and shall not corrupt text when an environment variable is empty or only a short placeholder.

**ESE-05** When filesystem permissions include root access, empty values, control characters, or AppArmor profile metacharacters, the system shall reject them before profile construction using slash-path semantics consistently across host operating systems.

**ESE-06** When filesystem permissions target sensitive, home, or broad paths without violating hard syntax rules, the system shall emit an auditable warning.

**ESE-07** When network permissions are validated, the system shall accept valid domains, IP addresses, CIDR ranges, localhost, and explicit wildcard access and shall reject malformed values.

**ESE-08** When network access is wildcard, loopback, private, or expressed as a raw IP, the system shall emit risk-specific audit telemetry.

**ESE-09** When command permissions are validated, the system shall require a slash-absolute path or well-known binary and shall reject profile-injection metacharacters consistently across host operating systems.

**ESE-10** When a sandbox is applied on macOS or supported Linux hosts, the system shall wrap execution in the platform sandbox with only declared filesystem and network permissions.

**ESE-11** When the host sandbox mechanism is unavailable, the system shall follow the platform's documented validation-only fallback rather than claiming isolation was applied.

**ESE-12** When a Linux sandbox profile is generated, the system shall derive a stable command hash and include only validated permission tokens.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_security_token_guardrails.feature`
- Package tests: `engram/internal/security/*_test.go`

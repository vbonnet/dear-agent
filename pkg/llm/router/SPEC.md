# LLM Router Specification

<!-- Last audited at: 2026-07-21 -->

## Overview

`pkg/llm/router` routes role-based workflow requests to concrete provider/model
pairs. It validates role configuration, resolves model identifiers through the
provider resolver, caches circuit-breaker-wrapped providers per model, and
falls through primary, secondary, and tertiary candidates without hiding the
attempt history.

## EARS Requirements

**LLM-ROUTER-01** When role config is parsed without a version, the system shall treat it as version 1.

**LLM-ROUTER-02** When role config declares any version other than 1, the system shall reject it.

**LLM-ROUTER-03** When a default role is configured, the system shall require that role to exist in the roles map.

**LLM-ROUTER-04** When a role is configured, the system shall require a non-empty role name and at least one model candidate.

**LLM-ROUTER-05** When candidates are requested for a role, the system shall return non-empty primary, secondary, and tertiary models in order.

**LLM-ROUTER-06** When a router is constructed, the system shall require a config and shall default missing resolver or factory dependencies.

**LLM-ROUTER-07** When generation is requested with an empty role, the system shall use the configured default role.

**LLM-ROUTER-08** When generation is requested, the system shall resolve each candidate model, construct or reuse a provider, set request model metadata, and return the first successful response.

**LLM-ROUTER-09** When a candidate fails from cancellation or deadline expiry, the system shall stop fallback and return the context error.

**LLM-ROUTER-10** When all candidates fail, the system shall return an error that names the role, candidate count, attempted models, and last wrapped error.

**LLM-ROUTER-11** When generation is requested for a literal model, the system shall bypass role lookup while still using resolver, provider construction, circuit breaker, and router model metadata.

**LLM-ROUTER-12** When the same family and model are requested repeatedly, the system shall reuse the cached provider entry.

**LLM-ROUTER-13** When a circuit-breaker fallback produces a response, the system shall identify the actual provider and model in response metadata and preserve the originally selected candidate separately.

**LLM-ROUTER-14** When a quota meter is configured, the system shall order a role's candidates by remaining provider quota before attempting them, and shall retain every configured candidate.

**LLM-ROUTER-15** When a quota meter is absent or reports no usable reading for a candidate, the system shall attempt candidates in their configured order and shall omit quota fields from request and response metadata.

**LLM-ROUTER-16** When a quota-classified candidate is attempted, the system shall record its quota class, family, remaining percentage, and constraining window in request and response metadata.

## BDD Traceability

- Feature: `agm/test/bdd/features/llm_runtime_guardrails.feature`
- Package tests: `pkg/llm/router/config_test.go`
- Package tests: `pkg/llm/router/router_test.go`
- Package tests: `pkg/llm/router/executor_test.go`
- Package tests: `pkg/llm/router/quota_test.go`

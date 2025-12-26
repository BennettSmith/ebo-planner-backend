# Constitution — Overland Trip Planning Backend

## 1. Purpose

This repository contains the **implementation** of the Overland Trip Planning backend service.

It exists to:

- Enforce the contract defined in the spec repository
- Execute domain behavior as described by use cases
- Provide a secure, reliable API to clients (CLI, web app, others)

This repo does **not** define the contract. It implements it.

## 2. Source of Truth

- API contract, domain language, and behavior are defined in the **spec repo**.
- This repo must consume a pinned spec version via `spec.lock`.

No implementation change may alter externally observable behavior without
a corresponding change in the spec repo.

**We practice spec-first development.** All requirements changes — new features, changes to behavior defined by a use case, and API contract changes — **must originate in the spec repo** (then flow into this repo via `spec.lock`).

## 3. Scope

### 3.1 Allowed content

- Application and domain code
- Persistence (repositories, migrations)
- API adapters/controllers
- Authentication and authorization enforcement
- Observability (logging, metrics, tracing)
- Deployment configuration and infrastructure

### 3.2 Disallowed content

- API contract definitions (OpenAPI)
- Client SDKs or CLI code
- UI or frontend code
- Product behavior defined only in comments

## 4. Architectural principles

- **Hexagonal architecture**
  - Domain and use cases at the core
  - Adapters at the edges
- **Use-case driven**
  - Every externally visible operation maps to a use case in the spec repo
- **Explicit boundaries**
  - Controllers adapt HTTP → use case
  - Persistence is behind interfaces
- **No business logic in adapters**

## 5. Contract compliance

### 5.1 Spec pinning

- `spec.lock` defines the exact spec version this service implements.
- Code generation uses the pinned spec only.

### 5.2 Contract tests

- The service must reject requests that violate the spec.
- Where practical, automated tests should verify:
  - request validation
  - authorization rules
  - state transitions (draft → published → canceled)

## 6. Change workflow

1) Propose contract/behavior change in the spec repo
2) Tag the spec
3) Update `spec.lock` in this repo
4) Regenerate code (if applicable)
5) Implement behavior
6) Validate via tests

Skipping step (1) is not allowed for externally visible changes.

When opening an implementation PR that changes externally visible behavior, link to the spec change (PR and/or tag) and the relevant use case(s).

## 6.1 Development workflow (mandatory)

### 6.1.1 Branches only

- All work MUST happen on a branch (no direct commits to `main`).
- Branch names MUST be: `{type}/{slug}`
  - `{type}` MUST be one of: `chore`, `bug`, `refactor`, `feature`
  - `{slug}` MUST be a short, lowercase, hyphenated description
  - Examples:
    - `feature/trip-publish-handler`
    - `bug/idempotency-collision`
    - `refactor/http-adapter-simplify`
    - `chore/ci-cache-tuning`

### 6.1.2 Pre-flight before PR

- Before creating or updating a PR, you MUST run `make ci` locally and it MUST pass.

### 6.1.3 Pull requests required

- Every change MUST be delivered via a pull request.
- CI must be green before merge (required checks).

### 6.1.4 Automation via `gh`

- Cursor agents SHOULD use the GitHub CLI (`gh`) to create PRs, set titles/descriptions, and enable auto-merge once checks pass.

## 7. Versioning & releases

- Service versioning is independent of spec versioning.
- A service release must declare which spec version it implements.

## 8. Testing philosophy

- **Unit tests** for domain logic
- **Use-case tests** for application behavior
- **Integration tests** for persistence and adapters
- **System tests** may reference acceptance scenarios from the spec repo

## 9. Tooling & automation

- Generated code may be committed or generated at build time,
  but must be reproducible from the pinned spec.
- CI must fail if generated code drifts from the spec.

## 10. Non-goals

- This repo is not a client SDK.
- This repo is not responsible for user interaction UX.
- This repo is not the place to debate product behavior.

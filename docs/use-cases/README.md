# Overland Trip Planning — Expanded Use Case Specs (v1)

This folder contains **expanded specifications** for each v1 use case in the Overland Trip Planning system.

Each use case is documented as a standalone Markdown file with a consistent structure intended for:
- backend/API implementation
- iOS/Android client development
- security/privacy review
- AI-assisted coding workflows

## Structure (per use case)

- **Primary Actor / Goal**
- **Preconditions / Postconditions**
- **Main Success Flow**
- **Alternate Flows**
- **Error Conditions** (HTTP-oriented)
- **Authorization Rules**
- **Domain Invariants Enforced** *(included for mutating use cases only)*
- **Output**
- **API Notes** (suggested endpoint + contract guidance)

## Mutating vs Read-only

- **Read-only** use cases do not modify state and are generally modeled as `GET`.
- **Mutating** use cases modify a Trip/RSVP/Member profile and include a **Domain Invariants Enforced** section.

## File naming

Files are named:

`UC-XX-<UseCaseName>.md`

Where `UC-XX` matches the v1 catalog ordering.

## Conventions

- **Members-only:** all callers must be authenticated members.
- **Trip visibility:** `DRAFT` visibility is governed by `draftVisibility` and organizer membership.
- **RSVP rules:** RSVP mutations allowed only when trip is `PUBLISHED`; capacity is enforced on `YES`.

## Suggested next steps

- Generate an OpenAPI draft from the **API Notes** sections.
- Add DTO schemas (request/response) as a follow-on iteration.
- Add integration points (email announcement gateway, directory provider, artifact URL validation).

Generated on 2025-12-24.

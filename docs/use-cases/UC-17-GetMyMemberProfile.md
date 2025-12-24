# UC-17 — GetMyMemberProfile

## Primary Actor
Member (authenticated caller)

## Goal
Retrieve the caller’s member profile from the membership directory using the authenticated subject in the JWT access token.

## Preconditions
- Caller presents a valid bearer JWT access token intended for this API.
- The API can resolve the authenticated subject (e.g., `sub` claim) from the token.

## Postconditions
- On success, the caller receives their current member profile.
- No state is modified.

## Main Success Flow
1. Caller sends `GET /members/me` with a valid bearer access token.
2. System validates the token (signature, expiry, audience/issuer as configured).
3. System extracts the authenticated subject from the token (e.g., `sub`).
4. System looks up an existing `Member` record bound to that subject.
5. System returns `200 OK` with the caller’s `MemberProfile`.

## Alternate Flows
### A1 — Member Not Yet Provisioned
1. Steps 1–3 as in Main Success Flow.
2. System finds no `Member` record bound to the authenticated subject.
3. System returns `404 Not Found` with error code `MEMBER_NOT_PROVISIONED`.
4. Client may proceed with UC-18 (CreateMyMember) to provision membership.

## Error Conditions
- `401 Unauthorized` — missing/invalid/expired token
- `403 Forbidden` — token is valid but caller is not permitted to access member features (e.g., blocked account, tenant mismatch)
- `404 Not Found` — `MEMBER_NOT_PROVISIONED`
- `500 Internal Server Error` — unexpected server error

## Authorization Rules
- Caller must be authenticated (valid bearer token).
- This endpoint is allowed even if the caller is not yet provisioned as a directory member (so clients can detect provisioning state).

## Output
- `MemberProfile` for the caller.

## API Notes
- **Method/Path:** `GET /members/me`
- **Auth:** Bearer JWT (Auth Genie access token)
- **Success Response (200):**
  - Body: `{ "member": MemberProfile }`
- **Not Provisioned (404):**
  - Body: `{ "error": { "code": "MEMBER_NOT_PROVISIONED", "message": "..." } }`

## Notes
- This endpoint intentionally distinguishes **authenticated** from **provisioned member**.
- The membership binding key should be derived from token identity (typically `sub`; optionally `(iss, sub)` or `(tenantId, sub)` depending on Auth Genie tenancy/issuer model).

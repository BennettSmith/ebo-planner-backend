# UC-18 — CreateMyMember

## Primary Actor
Member (authenticated caller)

## Goal
Provision a new `Member` record in the membership directory for the authenticated subject in the JWT access token.

## Preconditions
- Caller presents a valid bearer JWT access token intended for this API.
- No `Member` record already exists for the authenticated subject.

## Postconditions
- A new `Member` record exists and is bound to the authenticated subject.
- The new member becomes visible in directory/search (subject to visibility rules).
- The caller can access member-gated features that require a provisioned membership (e.g., list/search members, trip features).

## Main Success Flow
1. Caller sends `POST /members` with a valid bearer access token and `CreateMemberRequest` body.
2. System validates the token (signature, expiry, audience/issuer as configured).
3. System extracts the authenticated subject from the token (e.g., `sub`).
4. System verifies no existing `Member` record is already bound to that subject.
5. System validates the request payload:
   - `displayName` is present and non-empty.
   - `email` is present and syntactically valid.
   - Optional fields are validated when provided.
6. System creates a new `Member` record bound to the authenticated subject.
7. System returns `201 Created` with the new `MemberProfile`.

## Alternate Flows
### A1 — Member Already Exists
1. Steps 1–3 as in Main Success Flow.
2. System finds an existing `Member` record bound to the authenticated subject.
3. System returns `409 Conflict` with error code `MEMBER_ALREADY_EXISTS`.
   - Client should use UC-17 (GetMyMemberProfile) to retrieve the existing profile.

### A2 — Validation Failure
1. Steps 1–5 as in Main Success Flow.
2. System detects invalid or missing fields (e.g., missing `displayName`, invalid email format).
3. System returns `422 Unprocessable Entity` with validation details.

## Error Conditions
- `401 Unauthorized` — missing/invalid/expired token
- `403 Forbidden` — token is valid but caller is not permitted to provision membership (e.g., blocked account, tenant mismatch)
- `409 Conflict` — `MEMBER_ALREADY_EXISTS`
- `422 Unprocessable Entity` — invalid request payload
- `500 Internal Server Error` — unexpected server error

## Authorization Rules
- Caller must be authenticated (valid bearer token).
- The authenticated subject may only create a member record for itself (binding is derived from token claims; request body cannot override identity binding).
- Optional policy hook: restrict self-provisioning to callers who are members of an external allow-list (e.g., Google Group) if you choose to enforce that server-side.

## Domain Invariants Enforced
- At most one `Member` record exists per authenticated subject (uniqueness by `sub`, or by `(iss, sub)` / `(tenantId, sub)` depending on Auth Genie model).
- Member identity binding is immutable once created (subject binding cannot be changed via profile updates).

## Output
- Newly created `MemberProfile`.

## API Notes
- **Method/Path:** `POST /members`
- **Auth:** Bearer JWT (Auth Genie access token)
- **Request DTO:** `CreateMemberRequest`
  - `displayName: string` (required)
  - `email: string (email)` (required)
  - `groupAliasEmail: string (email)` (optional)
  - `vehicleProfile: VehicleProfile` (optional)
- **Success Response (201):**
  - Body: `{ "member": MemberProfile }`
- **Already Exists (409):**
  - Body: `{ "error": { "code": "MEMBER_ALREADY_EXISTS", "message": "..." } }`

## Notes
- This use case fills the missing “how members are added to the directory” gap.
- Member removal/deprovisioning can be added later (likely an admin-only use case) if needed; v1 can treat membership as durable and controlled out-of-band if you prefer.

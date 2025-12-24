# UC-16 — UpdateMyMemberProfile

## Primary Actor

Member

## Goal

Update profile metadata including group alias and vehicle profile.

## Preconditions

- Caller is authenticated.
- A member profile exists for the authenticated subject.

## Postconditions

- System state is updated as described.

---

## Main Success Flow

1. Actor submits a partial profile update to `/members/me`.
2. System authenticates the caller.
3. System resolves the caller’s member profile from the authenticated subject.
4. If no member profile exists, return `404 Not Found` (`MEMBER_NOT_PROVISIONED`).
5. System validates and applies only provided fields:

   - If `displayName` is provided: trim leading/trailing whitespace, collapse internal whitespace runs, and require non-empty after normalization.
   - If `email` is provided: validate email format and update the member’s email address. Email cannot be cleared.
   - If `groupAliasEmail` is provided: validate email format; `null` clears it.
   - If `vehicleProfile` is provided:
     - An object value applies as a partial update (only provided subfields are updated).
     - `null` clears the entire vehicle profile.
6. System persists the updated member profile.
7. System returns the updated member profile.

---

## Alternate Flows

A1 — Idempotent Retry (Same Idempotency Key, Same Request)

- **Condition:** A previous successful update exists for the same actor and idempotency key with an identical request payload (after applying the same `displayName` normalization rules).
- **Behavior:** System returns the previously returned response (no additional state change).
- **Outcome:** `200 OK` (idempotent).

A2 — Idempotency Key Reuse With Different Payload

- **Condition:** A previous request exists for the same actor and idempotency key, but the new request payload differs.
- **Behavior:** System rejects the request.
- **Outcome:** `409 Conflict`.

---

## Error Conditions

- `401 Unauthorized` — caller is not authenticated
- `404 Not Found` — no provisioned member exists for the authenticated subject
- `409 Conflict` — idempotency key reuse with a different payload (or other invariant violation)
- `422 Unprocessable Entity` — invalid input values (format/range)
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules

- Caller must be an authenticated member.
- Caller may update only their own profile (`/members/me`).

## Domain Invariants Enforced

- Profile is owned by the member (member can only update their own profile).
- Vehicle capability fields are informational only (not used for enforcement).
- `displayName` must be non-empty after normalization.
- `email` must be a valid email address and must be unique across members.

---

## Output

- Success DTO containing the updated member profile.

---

## API Notes

- Suggested endpoint: `PATCH /members/me`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Mutating: **require** an `Idempotency-Key` header to safely handle retries.
- Prefer `PATCH` for partial updates; validate and apply only provided fields.

### PATCH semantics (v1)

- Omitted fields are not modified.
- For nullable fields, an explicit `null` clears the value.
- For nested objects (e.g., `vehicleProfile`), provided subfields are applied; omitted subfields are unchanged.

---

## Notes

- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

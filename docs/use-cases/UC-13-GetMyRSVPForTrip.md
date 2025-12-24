# UC-13 — GetMyRSVPForTrip

## Primary Actor
Member

## Goal
Retrieve the member’s current RSVP state for a trip.

## Preconditions
- Caller is authenticated.
- Target resource exists and is visible/accessible to the caller.

## Postconditions
- Trip/member data is returned. No state is modified.

---

## Main Success Flow
1. Actor invokes the use case with the required identifiers and inputs.
2. System authenticates the caller.
3. System authorizes the caller for the target resource (trip/member).
4. System loads the required aggregate(s) and validates inputs.
5. System executes the primary behavior.
6. System returns the result.

---

## Alternate Flows
- None.

---

## Error Conditions
- `401 Unauthorized` — caller is not authenticated
- `403 Forbidden` — caller lacks permission for this operation
- `404 Not Found` — target resource does not exist
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules
- Caller must be an authenticated member.
- Caller may retrieve only their own RSVP state.
---

## Output
- Success DTO or confirmation response (depending on operation)

---

## API Notes
- Suggested endpoint: `GET /trips/{tripId}/rsvp/me`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Read-only: safe and cacheable (where appropriate).

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

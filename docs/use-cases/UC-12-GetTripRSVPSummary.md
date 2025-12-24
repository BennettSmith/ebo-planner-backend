# UC-12 — GetTripRSVPSummary

## Primary Actor
Member

## Goal
View lists of attending and not-attending members for a trip.

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
- Any authenticated member may access this data (subject to trip draft visibility for drafts).
---

## Output
- Success DTO or confirmation response (depending on operation)

---

## API Notes
- Suggested endpoint: `GET /trips/{tripId}/rsvps`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Read-only: safe and cacheable (where appropriate).

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

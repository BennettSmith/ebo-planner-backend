# UC-09 — AddTripOrganizer

## Primary Actor
Organizer

## Goal
Add an existing member as a co-organizer for a trip.

## Preconditions
- Caller is authenticated.
- Target resource exists and is visible/accessible to the caller.

## Postconditions
- System state is updated as described.

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
- `409 Conflict` — domain invariant violated (e.g., capacity reached, missing publish fields, removing last organizer)
- `422 Unprocessable Entity` — invalid input values (format/range)
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules
- Caller must be an authenticated member.
- Caller must be an organizer of the target trip.

## Domain Invariants Enforced
- Only existing organizers may add other organizers.
- Added organizer must be an existing authenticated member.
- Organizer list must not contain duplicates.
- At least one organizer must always exist.

---

## Output
- Success DTO or confirmation response (depending on operation)

---

## API Notes
- Suggested endpoint: `POST /trips/{tripId}/organizers`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Mutating: consider idempotency keys where duplicate submissions are plausible.

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

# UC-09 — AddTripOrganizer

## Primary Actor
Organizer

## Goal
Add an existing member as a co-organizer for a trip.

## Preconditions
- Caller is authenticated.
- Target trip exists and is visible/accessible to the caller.

## Postconditions
- System state is updated as described.

---

## Main Success Flow
1. Actor requests adding `memberId` as an organizer for a given `tripId`.
2. System authenticates the caller.
3. System loads the trip by `tripId`.
4. System authorizes access:
   - Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
   - Caller must be an organizer; if not, return `404 Not Found`.
5. System validates the target member exists.
6. System adds the target member to the trip’s organizer set (if not already present).
7. System returns the updated trip details.

---

## Alternate Flows
A1 — Target Member Already an Organizer
- **Condition:** Target member is already in the organizer set.
- **Behavior:** System performs no change.
- **Outcome:** `200 OK` with unchanged trip returned (idempotent).

A2 — Idempotent Retry (Same Idempotency Key, Same Request)
- **Condition:** A previous successful add-organizer request exists for the same actor and idempotency key with an identical request payload.
- **Behavior:** System returns the previously returned response (no additional state change).
- **Outcome:** `200 OK` (idempotent).

A3 — Idempotency Key Reuse With Different Payload
- **Condition:** A previous request exists for the same actor and idempotency key, but the new request payload differs.
- **Behavior:** System rejects the request.
- **Outcome:** `409 Conflict`.

---

## Error Conditions
- `401 Unauthorized` — caller is not authenticated
- `404 Not Found` — trip does not exist OR is not visible to the caller
- `409 Conflict` — domain invariant violated (e.g., idempotency key reuse with different payload)
- `422 Unprocessable Entity` — invalid input values (format/range)
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules
- Caller must be an authenticated member.
- Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
- Caller must be an organizer of the trip; if not, return `404 Not Found`.

## Domain Invariants Enforced
- Only existing organizers may add other organizers.
- Added organizer must be an existing authenticated member.
- Organizer list must not contain duplicates.
- At least one organizer must always exist.

---

## Output
- Success DTO containing the updated trip.

---

## API Notes
- Suggested endpoint: `POST /trips/{tripId}/organizers`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Mutating: **require** an `Idempotency-Key` header to safely handle retries.

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

# UC-10 — RemoveTripOrganizer

## Primary Actor
Organizer

## Goal
Remove an organizer from a trip. At least one organizer must remain.

## Preconditions
- Caller is authenticated.
- Target trip exists and is visible/accessible to the caller.

## Postconditions
- System state is updated as described.

---

## Main Success Flow
1. Actor requests removing `memberId` from the organizer set for a given `tripId`.
2. System authenticates the caller.
3. System loads the trip by `tripId`.
4. System authorizes access:
   - Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
   - Caller must be an organizer; if not, return `404 Not Found`.
5. System removes the target member from the organizer set (if present), enforcing “at least one organizer remains”.
6. System returns the updated trip details.

---

## Alternate Flows
A1 — Target Member Not an Organizer
- **Condition:** Target member is not in the organizer set.
- **Behavior:** System performs no change.
- **Outcome:** `200 OK` with unchanged trip returned (idempotent).

A2 — Attempt to Remove Last Organizer
- **Condition:** Removal would leave the trip with zero organizers.
- **Behavior:** System rejects the request.
- **Outcome:** `409 Conflict`.

A3 — Idempotent Retry (Same Idempotency Key, Same Request)
- **Condition:** A previous successful remove-organizer request exists for the same actor and idempotency key with an identical request (same `tripId` + `memberId`).
- **Behavior:** System returns the previously returned response (no additional state change).
- **Outcome:** `200 OK` (idempotent).

A4 — Idempotency Key Reuse With Different Payload
- **Condition:** A previous request exists for the same actor and idempotency key, but the new request differs (e.g., different `memberId`).
- **Behavior:** System rejects the request.
- **Outcome:** `409 Conflict`.

---

## Error Conditions
- `401 Unauthorized` — caller is not authenticated
- `404 Not Found` — trip does not exist OR is not visible to the caller
- `409 Conflict` — domain invariant violated (e.g., removing last organizer, idempotency mismatch)
- `422 Unprocessable Entity` — invalid input values (format/range)
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules
- Caller must be an authenticated member.
- Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
- Caller must be an organizer of the trip; if not, return `404 Not Found`.

## Domain Invariants Enforced
- Only existing organizers may remove organizers.
- At least one organizer must always remain (cannot remove the last organizer).

---

## Output
- Success DTO containing the updated trip.

---

## API Notes
- Suggested endpoint: `DELETE /trips/{tripId}/organizers/{memberId}`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Mutating: **require** an `Idempotency-Key` header to safely handle retries.

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

# UC-07 — UpdatePublishedTrip

## Primary Actor
Organizer

## Goal
Edit any aspect of a published trip (logistics may evolve).

## Preconditions
- Caller is authenticated.
- Target trip exists and is visible/accessible to the caller.

## Postconditions
- System state is updated as described.

---

## Main Success Flow
1. Actor submits a partial update for a given `tripId`.
2. System authenticates the caller.
3. System loads the trip by `tripId` and verifies `status = PUBLISHED`.
4. System authorizes access:
   - Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
   - Caller must be an organizer; if not, return `404 Not Found`.
5. System validates and applies only the provided fields:
   - If `name` is provided: trim leading/trailing whitespace, collapse internal whitespace runs, and require non-empty after normalization.
   - `meetingLocation` may be partially updated; set it to `null` to clear it.
   - Artifacts are updated by replacing the ordered list of `artifactIds` for the trip.
   - If `capacityRigs` is provided, it must not be reduced below the current number of attending rigs (`YES` RSVPs).
6. System persists the updated trip.
7. System returns the updated trip details.

---

## Alternate Flows
A1 — Attempt to Update a Non-Published Trip
- **Condition:** Trip is `DRAFT` or `CANCELED`.
- **Behavior:** System rejects the update via this use case.
- **Outcome:** `409 Conflict`.

A2 — Capacity Reduced Below Current Attendance
- **Condition:** Update attempts to set `capacityRigs` below the current attending rigs count.
- **Behavior:** System rejects the update.
- **Outcome:** `409 Conflict`.

A3 — Idempotent Retry (Same Idempotency Key, Same Request)
- **Condition:** A previous successful update exists for the same actor and idempotency key with an identical request payload (after applying the same name normalization rules).
- **Behavior:** System returns the previously returned response (no additional state change).
- **Outcome:** `200 OK` (idempotent).

A4 — Idempotency Key Reuse With Different Payload
- **Condition:** A previous request exists for the same actor and idempotency key, but the new request payload differs.
- **Behavior:** System rejects the request.
- **Outcome:** `409 Conflict`.

---

## Error Conditions
- `401 Unauthorized` — caller is not authenticated
- `404 Not Found` — trip does not exist OR is not visible to the caller
- `409 Conflict` — domain invariant violated (e.g., canceled trip, capacity below attendance, idempotency mismatch)
- `422 Unprocessable Entity` — invalid input values (format/range)
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules
- Caller must be an authenticated member.
- Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
- Caller must be an organizer of the trip; if not, return `404 Not Found`.

## Domain Invariants Enforced
- Trip must be in PUBLISHED state.
- Only organizers may update published trip fields.
- At least one organizer must always exist.
- Edits must not implicitly cancel the trip; cancellation is a separate use case (UC-08).
- Capacity cannot be reduced below current attendance.
- Draft visibility is not applicable for published trips.

---

## Output
- Success DTO containing the updated trip.

---

## API Notes
- Suggested endpoint: `PATCH /trips/{tripId}`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Mutating: **require** an `Idempotency-Key` header to safely handle retries.
- Prefer `PATCH` for partial updates; validate and apply only provided fields.

### PATCH semantics (v1)
- Omitted fields are not modified.
- For nullable fields, an explicit `null` clears the value.
- For arrays, providing a value replaces the entire array (e.g., `artifactIds`).

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

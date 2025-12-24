# UC-03 — CreateTripDraft

## Primary Actor
Member (creator)

## Goal
Create a new trip in `DRAFT` state. Incomplete data is allowed.

## Preconditions
- Caller is authenticated.

## Postconditions
- A new draft trip exists.
- The caller is recorded as the trip creator (`created_by_member_id`) and is the initial organizer.

---

## Main Success Flow
1. Actor submits a create-draft request with the required title (`name`).
2. System authenticates the caller.
3. System validates inputs (title must be present and non-blank).
4. System creates a new `Trip` with:
   - `status = DRAFT`
   - `draftVisibility = PRIVATE` (always)
   - `created_by_member_id = caller`
5. System ensures the creator is included as an organizer (initial organizer).
6. System returns the created trip identifiers/details.

---

## Alternate Flows
A1 — Idempotent Retry (Same Idempotency Key, Same Request)
- **Condition:** A previous successful create request exists for the same actor and idempotency key with an identical request payload.
- **Behavior:** System returns the previously created trip (no new trip is created).
- **Outcome:** Trip returned (idempotent).

A2 — Idempotency Key Reuse With Different Payload
- **Condition:** A previous request exists for the same actor and idempotency key, but the new request payload differs.
- **Behavior:** System rejects the request.
- **Outcome:** `409 Conflict`.

---

## Error Conditions
- `401 Unauthorized` — caller is not authenticated
- `409 Conflict` — idempotency key reuse with a different payload (or other invariant violation)
- `422 Unprocessable Entity` — invalid input values (format/range)
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules
- Caller must be an authenticated member.
- Any authenticated member may create a new trip draft; the caller becomes the creator and initial organizer.

## Domain Invariants Enforced
- Trip status is initialized to `DRAFT`.
- `draftVisibility` is set to `PRIVATE` and cannot be set at create time.
- `created_by_member_id` is set to the caller and is immutable.
- At least one organizer must exist; the creator becomes the initial organizer.

---

## Output
- Success DTO containing the created trip identifier and initial state (slim “created” DTO), e.g.:
  - `tripId`
  - `status = DRAFT`
  - `draftVisibility = PRIVATE`

---

## API Notes
- Suggested endpoint: `POST /trips`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Mutating: **require** an `Idempotency-Key` header to safely handle retries.

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

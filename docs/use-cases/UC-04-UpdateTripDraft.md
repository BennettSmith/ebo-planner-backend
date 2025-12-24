# UC-04 — UpdateTripDraft

## Primary Actor
Organizer

## Goal
Update any fields on a draft trip. Partial updates allowed.

## Preconditions
- Caller is authenticated.
- Target draft trip exists and is visible to the caller.

## Postconditions
- System state is updated as described.

---

## Main Success Flow
1. Actor submits a partial update for a given `tripId`.
2. System authenticates the caller.
3. System loads the trip by `tripId` and verifies `status = DRAFT`.
4. System authorizes access based on draft visibility:
   - If `draftVisibility = PRIVATE`: caller must be the creator (`created_by_member_id`).
   - If `draftVisibility = PUBLIC`: caller must be an organizer.
   - If not visible, return `404 Not Found` (do not reveal existence).
5. System validates and applies only the provided fields:
   - If `name` is provided: trim leading/trailing whitespace, collapse internal whitespace runs, and require non-empty after normalization.
   - Reject any attempt to patch `draftVisibility` here (use UC-05 instead).
   - `meetingLocation` may be partially updated; set it to `null` to clear it.
   - Artifacts are updated by replacing the ordered list of `artifactIds` for the trip.
6. System persists the updated draft.
7. System returns the updated trip details.

---

## Alternate Flows
- None.

---

## Error Conditions
- `401 Unauthorized` — caller is not authenticated
- `404 Not Found` — trip does not exist OR is not visible to the caller
- `409 Conflict` — domain invariant violated (e.g., removing last organizer)
- `422 Unprocessable Entity` — invalid input values (format/range)
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules
- Caller must be an authenticated member.
- Draft visibility rules:
  - `draftVisibility = PRIVATE`: visible only to the creator (`created_by_member_id`).
  - `draftVisibility = PUBLIC`: visible only to organizers.
- For non-visible drafts, return `404 Not Found` (do not reveal existence).

## Domain Invariants Enforced
- Trip must be in DRAFT state.
- Only visible/authorized actors may update draft fields (see Authorization Rules).
- At least one organizer must always exist.
- RSVP is not allowed for drafts (no RSVP mutations here).
- `draftVisibility` cannot be changed via this use case (UC-05 only).

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

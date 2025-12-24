# UC-11 — SetMyRSVP

## Primary Actor
Member

## Goal
Set RSVP to YES, NO, or UNSET for a published trip.

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
A1 — Change RSVP from YES to NO/UNSET
- **Condition:** Member previously had RSVP=YES.
- **Behavior:** System releases a rig slot and updates RSVP.
- **Outcome:** RSVP updated; capacity increases by 1.

A2 — Idempotent Update
- **Condition:** Member sets RSVP to the same value they already have.
- **Behavior:** System performs no state change.
- **Outcome:** Success response returned.

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
- Caller may set only their own RSVP for the specified trip.

## Domain Invariants Enforced
- RSVP is only allowed when Trip status is PUBLISHED.
- RSVP is owned by the member (member can only set their own RSVP).
- Capacity is enforced strictly when setting YES (YES consumes one rig slot).
- NO and UNSET do not consume capacity.
- No waitlist in v1; setting YES when capacity reached is rejected.

---

## Output
- Success DTO or confirmation response (depending on operation)

---

## API Notes
- Suggested endpoint: `PUT /trips/{tripId}/rsvp`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Mutating: consider idempotency keys where duplicate submissions are plausible.
- Use `PUT` semantics for last-write-wins on the member’s RSVP record.

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

# UC-13 — GetMyRSVPForTrip

## Primary Actor

Member

## Goal

Retrieve the member’s current RSVP state for a trip.

## Preconditions

- Caller is authenticated.
- Target trip exists and is visible/accessible to the caller.

## Postconditions

- Trip/member data is returned. No state is modified.

---

## Main Success Flow

1. Actor requests their RSVP for a given `tripId`.
2. System authenticates the caller.
3. System loads the trip by `tripId`.
4. System authorizes access:

   - Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
5. System verifies the trip is `PUBLISHED` or `CANCELED`. If the trip is a draft, reject with `409 Conflict`.
6. System loads the RSVP record for `(tripId, callerMemberId)`.
7. If no RSVP record exists, return `404 Not Found`.
8. System returns `myRsvp` (response + updated timestamp).

---

## Alternate Flows

- None.

---

## Error Conditions

- `401 Unauthorized` — caller is not authenticated
- `404 Not Found` — trip does not exist OR is not visible to the caller OR the caller has not recorded an RSVP for this trip
- `409 Conflict` — my RSVP is not available (e.g., trip is a draft)
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules

- Caller must be an authenticated member.
- Caller may retrieve only their own RSVP state.
- Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
- My RSVP is available only for `PUBLISHED` and `CANCELED` trips.

---

## Output

- Success DTO containing `myRsvp` (response + updated timestamp).

---

## API Notes

- Suggested endpoint: `GET /trips/{tripId}/rsvp/me`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Read-only: safe and cacheable (where appropriate).

---

## Notes

- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

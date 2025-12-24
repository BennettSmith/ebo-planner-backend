# UC-12 — GetTripRSVPSummary

## Primary Actor
Member

## Goal
View lists of attending and not-attending members for a trip.

## Preconditions
- Caller is authenticated.
- Target trip exists and is visible/accessible to the caller.

## Postconditions
- Trip/member data is returned. No state is modified.

---

## Main Success Flow
1. Actor requests the RSVP summary for a given `tripId`.
2. System authenticates the caller.
3. System loads the trip by `tripId`.
4. System authorizes access:
   - Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
5. System verifies the trip is `PUBLISHED` or `CANCELED`. If the trip is a draft, reject with `409 Conflict`.
6. System loads RSVP data and builds an RSVP summary:
   - `attendingMembers`: members with RSVP response `YES`
   - `notAttendingMembers`: members with RSVP response `NO`
   - `attendingRigs`: count of `YES` RSVPs
   - `capacityRigs`: trip capacity (may be null for trips that were canceled from draft)
   - RSVP response `UNSET` is omitted from the summary.
7. System orders results:
   - Grouped by attending vs not attending
   - Within each group, members sorted alphabetically by display name
8. System returns the RSVP summary.

---

## Alternate Flows
- None.

---

## Error Conditions
- `401 Unauthorized` — caller is not authenticated
- `404 Not Found` — trip does not exist OR is not visible to the caller
- `409 Conflict` — RSVP summary is not available (e.g., trip is a draft)
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules
- Caller must be an authenticated member.
- Trip must be visible to the caller; if not, return `404 Not Found` (do not reveal existence).
- RSVP summary is available only for `PUBLISHED` and `CANCELED` trips.
---

## Output
- Success DTO containing `TripRSVPSummary`.

---

## API Notes
- Suggested endpoint: `GET /trips/{tripId}/rsvps`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Read-only: safe and cacheable (where appropriate).

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

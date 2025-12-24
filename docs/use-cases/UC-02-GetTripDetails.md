# UC-02 — GetTripDetails

## Primary Actor
Member

## Goal
Retrieve full trip details of a specific trip that is visible to the member.

## Preconditions
- Caller is authenticated.

## Postconditions
- Trip/member data is returned. No state is modified.

---

## Main Success Flow
1. Actor requests trip details for a given `tripId`.
2. System authenticates the caller.
3. System loads the trip by `tripId`.
4. System authorizes visibility:
   - If `status in (PUBLISHED, CANCELED)`: visible to any authenticated member.
   - If `status = DRAFT` and `draftVisibility = PUBLIC`: visible only to organizers.
   - If `status = DRAFT` and `draftVisibility = PRIVATE`: visible only to the creator (initial organizer).
   - If not visible, return `404 Not Found` (do not reveal existence).
5. System returns `TripDetails` for the trip.

---

## Alternate Flows
A1 — Trip Is a Draft (Public) and Caller Is an Organizer
- **Condition:** Trip is `DRAFT`, `draftVisibility = PUBLIC`, and caller is an organizer.
- **Behavior:** System returns draft trip details; RSVP fields are omitted and RSVP actions are disabled.
- **Outcome:** Trip details returned.

A2 — Trip Is a Draft (Private) and Caller Is the Creator
- **Condition:** Trip is `DRAFT`, `draftVisibility = PRIVATE`, and caller is the creator (initial organizer).
- **Behavior:** System returns draft trip details; RSVP fields are omitted and RSVP actions are disabled.
- **Outcome:** Trip details returned.

A3 — Trip Is Canceled
- **Condition:** Trip is `CANCELED`.
- **Behavior:** System returns trip details and indicates RSVP actions are disabled.
- **Outcome:** Trip details returned.

---

## Error Conditions
- `401 Unauthorized` — caller is not authenticated
- `404 Not Found` — trip does not exist OR is not visible to the caller

---

## Authorization Rules
- Caller must be an authenticated member.
- Trips with `status in (PUBLISHED, CANCELED)` are visible to any authenticated member.
- Draft visibility rules:
  - `draftVisibility = PUBLIC`: visible only to organizers.
  - `draftVisibility = PRIVATE`: visible only to the creator (initial organizer).
- For non-visible drafts, return `404 Not Found` (do not reveal existence).
---

## Output
- Success DTO containing `TripDetails`.
- Draft-specific output rules:
  - System returns all draft fields (even if incomplete).
  - RSVP fields (`rsvpSummary`, `myRsvp`) are omitted.
  - `rsvpActionsEnabled = false`.

---

## API Notes
- Suggested endpoint: `GET /trips/{tripId}`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Read-only: safe and cacheable (where appropriate).

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

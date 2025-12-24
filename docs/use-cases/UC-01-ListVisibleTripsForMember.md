# UC-01 — ListVisibleTripsForMember

## Primary Actor
Member

## Goal
Return lists of trips relevant to the authenticated member.

Two variants are supported:
- Published + canceled trips (discovery)
- Draft trips where the member is an organizer (work-in-progress)

## Preconditions
- Caller is authenticated.

## Postconditions
- Trip/member data is returned. No state is modified.

---

## Main Success Flow
**Variant 1 — Published + Canceled Trips**

1. Actor requests the published/canceled trips list.
2. System authenticates the caller.
3. System loads trips where `status in (PUBLISHED, CANCELED)`.
4. System sorts results by `startDate` (ascending). If `startDate` is missing, sort those trips by creation time (ascending) after dated trips.
5. System maps each trip to a `TripSummary` DTO.
6. System returns the list.

---

## Alternate Flows
A1 — Variant 2: Draft Trips Where Caller Is an Organizer
- **Condition:** Actor requests the organizer-drafts list.
- **Behavior:**
  - System loads trips where `status = DRAFT` that are visible to the caller:
    - `draftVisibility = PUBLIC` and caller is included in `organizerMemberIds`
    - `draftVisibility = PRIVATE` and caller is the creator (initial organizer)
  - System sorts results by `startDate` (ascending). If `startDate` is missing, sort those trips by creation time (ascending) after dated trips.
  - System maps each trip to a `TripSummary` DTO.
- **Outcome:** Draft trips list returned.

---

## Error Conditions
- `401 Unauthorized` — caller is not authenticated

---

## Authorization Rules
- Caller must be an authenticated member.
- Any authenticated member may access Variant 1 (published + canceled trips).
- Variant 2 returns only drafts visible to the caller:
  - `draftVisibility = PUBLIC`: visible only to organizers.
  - `draftVisibility = PRIVATE`: visible only to the creator (initial organizer).
---

## Output
- Success DTO containing a list of `TripSummary` items.
- DTO field rules:
  - For `PUBLISHED` and `CANCELED` trips: omit `draftVisibility`.
  - For `DRAFT` trips: include `draftVisibility`.
  - For `DRAFT` and `CANCELED` trips: omit `attendingRigs`.

---

## API Notes
- Suggested endpoints:
  - `GET /trips` — Variant 1 (published + canceled trips)
  - `GET /trips/drafts` — Variant 2 (draft trips where caller is an organizer)
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields (e.g., creation timestamps used for sorting).
- Read-only: safe and cacheable (where appropriate).

---

## Notes
- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

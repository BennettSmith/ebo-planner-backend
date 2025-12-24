# UC-14 — ListMembers

## Primary Actor

Member

## Goal

View a minimal directory of members for coordination.

## Preconditions

- Caller is authenticated.

## Postconditions

- Trip/member data is returned. No state is modified.

---

## Main Success Flow

1. Actor requests the member directory (optionally setting `includeInactive=true`).
2. System authenticates the caller.
3. System loads members:

   - By default, include only active members.
   - If `includeInactive=true`, include inactive members as well.
   - Ensure the caller is included in the results.
4. System orders results by `displayName` ascending.
5. System maps results to a minimal directory DTO (no email fields).
6. System returns the directory list.

---

## Alternate Flows

A1 — Include Inactive Members

- **Condition:** Actor requests `includeInactive=true`.
- **Behavior:** System includes inactive members in the directory list.
- **Outcome:** Directory returned.

---

## Error Conditions

- `401 Unauthorized` — caller is not authenticated
- `500 Internal Server Error` — unexpected failure

---

## Authorization Rules

- Caller must be an authenticated member.
- Any authenticated member may access the member directory.

---

## Output

- Success DTO containing a list of minimal member directory entries.
- Directory entries must not expose email addresses.

---

## API Notes

- Suggested endpoint: `GET /members`
- Prefer returning a stable DTO shape; avoid leaking internal persistence fields.
- Read-only: safe and cacheable (where appropriate).
- Suggested query param: `includeInactive=true|false` (default false).

---

## Notes

- Aligned with v1 guardrails: members-only, planning-focused, lightweight RSVP, artifacts referenced externally.

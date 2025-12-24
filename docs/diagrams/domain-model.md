# Domain Model (v1) — Mermaid

Source of truth: `docs/use-cases/*.md` + `openapi.yaml` (DTO shapes) + `migrations/*.sql` (persistence constraints).

```mermaid
classDiagram
direction LR

class Trip {
  +TripId (external_id)
  +createdByMemberId (created_by_member_id)
  +name
  +description
  +startDate
  +endDate
  +status (DRAFT | PUBLISHED | CANCELED)
  +draftVisibility (PRIVATE | PUBLIC) [draft-only]
  +capacityRigs
  +difficultyText
  +meetingLocation : Location
  +commsRequirementsText
  +recommendedRequirementsText
}

class Member {
  +MemberId (external_id)
  +displayName
  +email
  +groupAliasEmail?
  +isActive
}

class VehicleProfile {
  +make?
  +model?
  +tireSize?
  +liftLockers?
  +fuelRange?
  +recoveryGear?
  +hamRadioCallSign?
  +notes?
}

class RSVP {
  +TripId
  +MemberId
  +response (YES | NO | UNSET)
  +updatedAt
}

class TripArtifact {
  +type (GPX | SCHEDULE | DOCUMENT | OTHER)
  +title
  +url
  +sortOrder
}

class Location {
  +label
  +address?
  +latitudeLongitude? : LatitudeLongitude
}

class LatitudeLongitude {
  +latitude
  +longitude
}

class TripOrganizer {
  +TripId
  +MemberId
  +addedAt
}

Trip "1" o-- "*" TripArtifact : artifacts
Trip "1" o-- "*" RSVP : rsvps
Member "1" o-- "0..1" VehicleProfile : vehicleProfile

Member "1" --> "*" Trip : creates
Trip "1" o-- "*" TripOrganizer : organizers
Member "1" o-- "*" TripOrganizer : organizes
```

## Notes (v1)

- **Trip lifecycle**: `DRAFT -> PUBLISHED -> CANCELED` (publish only allowed from `DRAFT`).
- **Draft visibility**: only meaningful when `status = DRAFT`.
  - `PRIVATE`: visible only to the **creator**.
  - `PUBLIC`: visible only to **organizers**.
- **RSVP rules**:
  - Allowed only when `status = PUBLISHED`.
  - `YES` consumes exactly one “rig slot”; `NO`/`UNSET` do not.
  - Capacity is strictly enforced on transitions to `YES`.
- **Artifacts**: externally hosted; stored as URLs.



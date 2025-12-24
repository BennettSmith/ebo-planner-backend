# Database Schema (v1) — Mermaid

Source of truth: `migrations/*.sql`

```mermaid
erDiagram
  MEMBERS {
    bigint id PK
    uuid external_id "unique"
    text subject_iss
    text subject_sub
    text display_name
    citext email "unique"
    citext group_alias_email
    boolean is_active
    timestamptz created_at
    timestamptz updated_at
  }

  MEMBER_VEHICLE_PROFILES {
    bigint id PK
    uuid external_id "unique"
    bigint member_id FK "unique"
    text make
    text model
    text tire_size
    text lift_lockers
    text fuel_range
    text recovery_gear
    text ham_radio_call_sign
    text notes
    timestamptz updated_at
  }

  TRIPS {
    bigint id PK
    uuid external_id "unique"
    bigint created_by_member_id FK "not null"
    text name
    text description
    date start_date
    date end_date
    trip_status status
    draft_visibility draft_visibility
    int capacity_rigs
    text difficulty_text
    text meeting_location_label
    text meeting_location_address
    double meeting_location_latitude
    double meeting_location_longitude
    text comms_requirements_text
    text recommended_requirements_text
    timestamptz published_at
    timestamptz canceled_at
    timestamptz created_at
    timestamptz updated_at
  }

  TRIP_ORGANIZERS {
    bigint trip_id PK, FK
    bigint member_id PK, FK
    timestamptz added_at
  }

  TRIP_ARTIFACTS {
    bigint id PK
    uuid external_id "unique"
    bigint trip_id FK
    artifact_type type
    text title
    text url
    int sort_order
    timestamptz created_at
    timestamptz updated_at
  }

  TRIP_RSVPS {
    bigint trip_id PK, FK
    bigint member_id PK, FK
    rsvp_response response
    timestamptz updated_at
  }

  IDEMPOTENCY_KEYS {
    text idempotency_key PK
    bigint actor_member_id PK, FK
    text scope PK
    text request_hash
    jsonb response_body
    timestamptz created_at
    timestamptz expires_at
  }

  MEMBERS ||--|| MEMBER_VEHICLE_PROFILES : "has"

  MEMBERS ||--o{ TRIPS : "creates"

  TRIPS ||--o{ TRIP_ORGANIZERS : "has"
  MEMBERS ||--o{ TRIP_ORGANIZERS : "organizes"

  TRIPS ||--o{ TRIP_ARTIFACTS : "has"

  TRIPS ||--o{ TRIP_RSVPS : "has"
  MEMBERS ||--o{ TRIP_RSVPS : "rsvps"

  MEMBERS ||--o{ IDEMPOTENCY_KEYS : "owns"
```

## Key behaviors enforced in Postgres

- **updated_at automation**: triggers set `updated_at` on `members`, `trips`, `trip_artifacts`, `member_vehicle_profiles`, `trip_rsvps`.
- **Organizer invariant**: trigger blocks deleting the last row in `trip_organizers` for a trip.
- **Trip transitions**: trigger enforces publish requirements + sets `published_at` / `canceled_at`.
- **RSVP capacity + state**: trigger enforces “published-only” and strict capacity on transitions to `YES`.

## Views (read models)

- `v_trip_summary`: trip list fields + `attending_rigs` count.
- `v_trip_rsvp_summary`: `capacity_rigs` + `attending_rigs` count.



-- 000001_init.up.sql
--
-- Initializes the database schema from scratch.
--

-- Overland East Bay — Trip Planning DB Schema (PostgreSQL)
--
-- Notes:
-- - Uses pgcrypto for gen_random_uuid().
-- - Uses citext for case-insensitive email/search convenience.

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'trip_status') THEN
    CREATE TYPE trip_status AS ENUM ('DRAFT', 'PUBLISHED', 'CANCELED');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'draft_visibility') THEN
    CREATE TYPE draft_visibility AS ENUM ('PRIVATE', 'PUBLIC');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'rsvp_response') THEN
    CREATE TYPE rsvp_response AS ENUM ('YES', 'NO', 'UNSET');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'artifact_type') THEN
    CREATE TYPE artifact_type AS ENUM ('GPX', 'SCHEDULE', 'DOCUMENT', 'OTHER');
  END IF;
END $$;

-- =========================================================================
-- Members
-- =========================================================================
CREATE TABLE IF NOT EXISTS members (
  id                   bigserial PRIMARY KEY,
  external_id          uuid NOT NULL DEFAULT gen_random_uuid(),

  -- Binding to authenticated subject from bearer JWT (see UC-17/UC-18).
  -- Use (subject_iss, subject_sub) uniqueness to support multiple issuers if needed.
  subject_iss           text NOT NULL,
  subject_sub           text NOT NULL,

  display_name          text NOT NULL,
  email                 citext NOT NULL,
  group_alias_email     citext NULL,

  -- Soft admin controls (optional / future-proof):
  is_active             boolean NOT NULL DEFAULT true,

  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT members_external_id_unique UNIQUE (external_id),
  CONSTRAINT members_subject_unique UNIQUE (subject_iss, subject_sub),
  CONSTRAINT members_email_unique UNIQUE (email)
);

CREATE TABLE IF NOT EXISTS member_vehicle_profiles (
  id                  bigserial PRIMARY KEY,
  external_id         uuid NOT NULL DEFAULT gen_random_uuid(),
  member_id           bigint NOT NULL REFERENCES members(id) ON DELETE CASCADE,

  make                text NULL,
  model               text NULL,
  tire_size           text NULL,
  lift_lockers        text NULL,
  fuel_range          text NULL,
  recovery_gear       text NULL,
  ham_radio_call_sign text NULL,
  notes               text NULL,

  updated_at          timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT member_vehicle_profiles_external_id_unique UNIQUE (external_id),
  CONSTRAINT member_vehicle_profiles_member_unique UNIQUE (member_id)
);

-- =========================================================================
-- Trips
-- =========================================================================
CREATE TABLE IF NOT EXISTS trips (
  id                          bigserial PRIMARY KEY,
  external_id                 uuid NOT NULL DEFAULT gen_random_uuid(),

  name                        text NULL,
  description                 text NULL,
  start_date                  date NULL,
  end_date                    date NULL,

  status                      trip_status NOT NULL DEFAULT 'DRAFT',
  draft_visibility            draft_visibility NULL,

  capacity_rigs               integer NULL CHECK (capacity_rigs IS NULL OR capacity_rigs >= 1),
  difficulty_text             text NULL,

  -- Meeting location (embedded as nullable columns)
  meeting_location_label      text NULL,
  meeting_location_address    text NULL,
  meeting_location_latitude   double precision NULL,
  meeting_location_longitude  double precision NULL,

  comms_requirements_text     text NULL,
  recommended_requirements_text text NULL,

  -- Publish/cancel bookkeeping (useful for idempotent behavior and UI)
  published_at                timestamptz NULL,
  canceled_at                 timestamptz NULL,

  created_at                  timestamptz NOT NULL DEFAULT now(),
  updated_at                  timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT trips_external_id_unique UNIQUE (external_id),

  -- Draft visibility is only relevant for DRAFTs (v1 domain model).
  CONSTRAINT trips_draft_visibility_consistency CHECK (
    (status = 'DRAFT' AND draft_visibility IS NOT NULL)
    OR (status <> 'DRAFT' AND draft_visibility IS NULL)
  ),

  CONSTRAINT trips_dates_consistency CHECK (
    (start_date IS NULL OR end_date IS NULL) OR (start_date <= end_date)
  ),

  CONSTRAINT trips_meeting_location_latlon_consistency CHECK (
    (meeting_location_latitude IS NULL AND meeting_location_longitude IS NULL)
    OR (meeting_location_latitude IS NOT NULL AND meeting_location_longitude IS NOT NULL)
  )
);

-- Many-to-many: trips <-> organizers (members)
CREATE TABLE IF NOT EXISTS trip_organizers (
  trip_id     bigint NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  member_id   bigint NOT NULL REFERENCES members(id) ON DELETE RESTRICT,
  added_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (trip_id, member_id)
);

-- Trip artifacts (externally hosted; referenced by URL)
CREATE TABLE IF NOT EXISTS trip_artifacts (
  id               bigserial PRIMARY KEY,
  external_id      uuid NOT NULL DEFAULT gen_random_uuid(),
  trip_id          bigint NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  type             artifact_type NOT NULL,
  title            text NOT NULL,
  url              text NOT NULL, -- validate as URI at application layer

  sort_order       integer NOT NULL DEFAULT 0,

  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT trip_artifacts_external_id_unique UNIQUE (external_id)
);

-- =========================================================================
-- RSVPs (member-owned, one vehicle per member per trip)
-- =========================================================================
CREATE TABLE IF NOT EXISTS trip_rsvps (
  trip_id        bigint NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
  member_id      bigint NOT NULL REFERENCES members(id) ON DELETE CASCADE,

  response       rsvp_response NOT NULL DEFAULT 'UNSET',
  updated_at     timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (trip_id, member_id)
);

-- =========================================================================
-- Optional: Idempotency keys for mutating endpoints (recommended in use cases)
-- =========================================================================
CREATE TABLE IF NOT EXISTS idempotency_keys (
  idempotency_key   text NOT NULL,
  actor_member_id   bigint NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  scope             text NOT NULL, -- e.g., "POST /trips", "PUT /trips/{id}/rsvp"
  request_hash      text NULL,     -- optional: to detect mismatched retries
  response_body     jsonb NULL,    -- optional: cached response for safe retries
  created_at        timestamptz NOT NULL DEFAULT now(),
  expires_at        timestamptz NULL,

  PRIMARY KEY (idempotency_key, actor_member_id, scope)
);

-- Members lookup by subject and search by name/email
CREATE INDEX IF NOT EXISTS idx_members_subject ON members(subject_iss, subject_sub);
CREATE INDEX IF NOT EXISTS idx_members_display_name ON members USING gin (to_tsvector('simple', display_name));
CREATE INDEX IF NOT EXISTS idx_members_email ON members(email);

-- Trips listing (published + public drafts) and sorting by start date
CREATE INDEX IF NOT EXISTS idx_trips_status_start_date ON trips(status, start_date);
CREATE INDEX IF NOT EXISTS idx_trips_draft_visibility ON trips(draft_visibility) WHERE status = 'DRAFT';

-- Organizers
CREATE INDEX IF NOT EXISTS idx_trip_organizers_member ON trip_organizers(member_id);

-- Artifacts
CREATE INDEX IF NOT EXISTS idx_trip_artifacts_trip ON trip_artifacts(trip_id);

-- RSVPs
CREATE INDEX IF NOT EXISTS idx_trip_rsvps_trip_response ON trip_rsvps(trip_id, response);
CREATE INDEX IF NOT EXISTS idx_trip_rsvps_member ON trip_rsvps(member_id);

-- Idempotency
CREATE INDEX IF NOT EXISTS idx_idempotency_keys_expires_at ON idempotency_keys(expires_at);

-- =========================================================================
-- updated_at helpers
-- =========================================================================
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_members_set_updated_at') THEN
    CREATE TRIGGER trg_members_set_updated_at
    BEFORE UPDATE ON members
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_trips_set_updated_at') THEN
    CREATE TRIGGER trg_trips_set_updated_at
    BEFORE UPDATE ON trips
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_trip_artifacts_set_updated_at') THEN
    CREATE TRIGGER trg_trip_artifacts_set_updated_at
    BEFORE UPDATE ON trip_artifacts
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
  END IF;
END $$;

-- Member vehicle profile updated_at
CREATE OR REPLACE FUNCTION set_vehicle_profile_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_member_vehicle_profiles_set_updated_at') THEN
    CREATE TRIGGER trg_member_vehicle_profiles_set_updated_at
    BEFORE UPDATE ON member_vehicle_profiles
    FOR EACH ROW
    EXECUTE FUNCTION set_vehicle_profile_updated_at();
  END IF;
END $$;

-- RSVP updated_at
CREATE OR REPLACE FUNCTION set_rsvp_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_trip_rsvps_set_updated_at') THEN
    CREATE TRIGGER trg_trip_rsvps_set_updated_at
    BEFORE UPDATE ON trip_rsvps
    FOR EACH ROW
    EXECUTE FUNCTION set_rsvp_updated_at();
  END IF;
END $$;

-- =========================================================================
-- Organizer invariants: at least one organizer must always exist
-- =========================================================================
CREATE OR REPLACE FUNCTION prevent_removing_last_organizer()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  remaining_count integer;
BEGIN
  SELECT count(*) INTO remaining_count
  FROM trip_organizers
  WHERE trip_id = OLD.trip_id
    AND member_id <> OLD.member_id;

  IF remaining_count = 0 THEN
    RAISE EXCEPTION 'Cannot remove last organizer for trip %', OLD.trip_id
      USING ERRCODE = '23514'; -- check_violation -> map to 409 in app
  END IF;

  RETURN OLD;
END;
$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_trip_organizers_prevent_last_delete') THEN
    CREATE TRIGGER trg_trip_organizers_prevent_last_delete
    BEFORE DELETE ON trip_organizers
    FOR EACH ROW
    EXECUTE FUNCTION prevent_removing_last_organizer();
  END IF;
END $$;

-- =========================================================================
-- Publish/Cancel transitions: set published_at / canceled_at and enforce publish-required fields
-- =========================================================================
CREATE OR REPLACE FUNCTION trips_enforce_transitions()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  organizer_count integer;
  current_yes integer;
BEGIN
  -- Trips cannot be un-canceled (v1).
  IF OLD.status = 'CANCELED' AND NEW.status <> 'CANCELED' THEN
    RAISE EXCEPTION 'Trip cannot be un-canceled (was % -> %)', OLD.status, NEW.status
      USING ERRCODE = '23514';
  END IF;

  -- Prevent reducing capacity below current attendance for published trips.
  -- (Drafts have no RSVP; canceled trips are read-only at the app layer but this keeps the DB consistent.)
  IF OLD.status = 'PUBLISHED'
     AND NEW.capacity_rigs IS NOT NULL
     AND (NEW.capacity_rigs IS DISTINCT FROM OLD.capacity_rigs) THEN
    SELECT count(*) INTO current_yes
    FROM trip_rsvps
    WHERE trip_id = OLD.id
      AND response = 'YES';

    IF NEW.capacity_rigs < current_yes THEN
      RAISE EXCEPTION 'Trip capacity_rigs (%) cannot be less than attending_rigs (%)', NEW.capacity_rigs, current_yes
        USING ERRCODE = '23514';
    END IF;
  END IF;

  -- If status is changing to PUBLISHED, enforce required-at-publish fields (v1).
  IF (OLD.status <> 'PUBLISHED' AND NEW.status = 'PUBLISHED') THEN
    -- Must come from DRAFT
    IF OLD.status <> 'DRAFT' THEN
      RAISE EXCEPTION 'Trip can only be published from DRAFT (was %)', OLD.status
        USING ERRCODE = '23514';
    END IF;

    -- Only PUBLIC drafts are publishable (v1).
    IF OLD.draft_visibility <> 'PUBLIC' THEN
      RAISE EXCEPTION 'Trip can only be published when draft_visibility = PUBLIC (was %)', OLD.draft_visibility
        USING ERRCODE = '23514';
    END IF;

    -- Required fields
    IF NEW.name IS NULL OR btrim(NEW.name) = '' THEN
      RAISE EXCEPTION 'Trip name is required to publish' USING ERRCODE = '23514';
    END IF;
    IF NEW.description IS NULL OR btrim(NEW.description) = '' THEN
      RAISE EXCEPTION 'Trip description is required to publish' USING ERRCODE = '23514';
    END IF;
    IF NEW.start_date IS NULL OR NEW.end_date IS NULL THEN
      RAISE EXCEPTION 'Trip start_date and end_date are required to publish' USING ERRCODE = '23514';
    END IF;
    IF NEW.capacity_rigs IS NULL OR NEW.capacity_rigs < 1 THEN
      RAISE EXCEPTION 'Trip capacity_rigs is required to publish and must be >= 1' USING ERRCODE = '23514';
    END IF;
    IF NEW.difficulty_text IS NULL OR btrim(NEW.difficulty_text) = '' THEN
      RAISE EXCEPTION 'Trip difficulty_text is required to publish' USING ERRCODE = '23514';
    END IF;
    IF NEW.meeting_location_label IS NULL OR btrim(NEW.meeting_location_label) = '' THEN
      RAISE EXCEPTION 'Trip meeting_location.label is required to publish' USING ERRCODE = '23514';
    END IF;
    IF NEW.comms_requirements_text IS NULL OR btrim(NEW.comms_requirements_text) = '' THEN
      RAISE EXCEPTION 'Trip comms_requirements_text is required to publish' USING ERRCODE = '23514';
    END IF;
    IF NEW.recommended_requirements_text IS NULL OR btrim(NEW.recommended_requirements_text) = '' THEN
      RAISE EXCEPTION 'Trip recommended_requirements_text is required to publish' USING ERRCODE = '23514';
    END IF;

    SELECT count(*) INTO organizer_count
    FROM trip_organizers
    WHERE trip_id = NEW.id;

    IF organizer_count < 1 THEN
      RAISE EXCEPTION 'Trip must have at least one organizer to publish' USING ERRCODE = '23514';
    END IF;

    NEW.published_at := COALESCE(NEW.published_at, now());
    NEW.draft_visibility := NULL; -- no longer relevant after publish
  END IF;

  -- If status is changing to CANCELED, set canceled_at (idempotent allowed)
  IF (OLD.status <> 'CANCELED' AND NEW.status = 'CANCELED') THEN
    NEW.canceled_at := COALESCE(NEW.canceled_at, now());
    NEW.draft_visibility := NULL;
  END IF;

  RETURN NEW;
END;
$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_trips_enforce_transitions') THEN
    CREATE TRIGGER trg_trips_enforce_transitions
    BEFORE UPDATE ON trips
    FOR EACH ROW
    EXECUTE FUNCTION trips_enforce_transitions();
  END IF;
END $$;

-- =========================================================================
-- RSVP invariants:
-- - Allowed only when trip.status = PUBLISHED
-- - Capacity enforced strictly on YES (one rig per member)
-- =========================================================================
CREATE OR REPLACE FUNCTION enforce_rsvp_rules()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
  t_status trip_status;
  t_capacity integer;
  current_yes integer;
  is_yes_transition boolean;
BEGIN
  SELECT status, capacity_rigs INTO t_status, t_capacity
  FROM trips
  WHERE id = NEW.trip_id
  FOR UPDATE; -- serialize RSVP mutations per trip for capacity correctness

  IF t_status IS NULL THEN
    RAISE EXCEPTION 'Trip % does not exist', NEW.trip_id USING ERRCODE = '23503';
  END IF;

  IF t_status <> 'PUBLISHED' THEN
    RAISE EXCEPTION 'RSVPs are only allowed when trip is PUBLISHED (status=%)', t_status
      USING ERRCODE = '23514';
  END IF;

  -- Published trips must always have capacity configured (v1).
  IF t_capacity IS NULL OR t_capacity < 1 THEN
    RAISE EXCEPTION 'Trip capacity_rigs must be set to >= 1 for RSVPs (capacity_rigs=%)', t_capacity
      USING ERRCODE = '23514';
  END IF;

  -- Determine if this change consumes a rig slot.
  IF TG_OP = 'INSERT' THEN
    is_yes_transition := (NEW.response = 'YES');
  ELSE
    is_yes_transition := (OLD.response <> 'YES' AND NEW.response = 'YES');
  END IF;

  IF is_yes_transition THEN
    IF t_capacity IS NOT NULL THEN
      SELECT count(*) INTO current_yes
      FROM trip_rsvps
      WHERE trip_id = NEW.trip_id
        AND response = 'YES'
        AND NOT (TG_OP = 'UPDATE' AND member_id = NEW.member_id);

      IF current_yes >= t_capacity THEN
        RAISE EXCEPTION 'Trip capacity reached (% rigs)', t_capacity
          USING ERRCODE = '23514';
      END IF;
    END IF;
  END IF;

  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'trg_trip_rsvps_enforce_rules') THEN
    CREATE TRIGGER trg_trip_rsvps_enforce_rules
    BEFORE INSERT OR UPDATE ON trip_rsvps
    FOR EACH ROW
    EXECUTE FUNCTION enforce_rsvp_rules();
  END IF;
END $$;

-- Convenience view for trip listing with attendingRigs count.
CREATE OR REPLACE VIEW v_trip_summary AS
SELECT
  t.external_id AS trip_id,
  t.name,
  t.start_date,
  t.end_date,
  t.status,
  t.draft_visibility,
  t.capacity_rigs,
  COALESCE(r.attending_rigs, 0) AS attending_rigs,
  t.created_at,
  t.updated_at
FROM trips t
LEFT JOIN (
  SELECT trip_id, count(*) AS attending_rigs
  FROM trip_rsvps
  WHERE response = 'YES'
  GROUP BY trip_id
) r ON r.trip_id = t.id;

-- Convenience view for RSVP summary lists.
-- (Application can still filter by trip visibility rules in queries.)
CREATE OR REPLACE VIEW v_trip_rsvp_summary AS
SELECT
  t.external_id AS trip_id,
  t.capacity_rigs,
  COALESCE(a.attending_rigs, 0) AS attending_rigs
FROM trips t
LEFT JOIN (
  SELECT trip_id, count(*) AS attending_rigs
  FROM trip_rsvps
  WHERE response = 'YES'
  GROUP BY trip_id
) a ON a.trip_id = t.id;

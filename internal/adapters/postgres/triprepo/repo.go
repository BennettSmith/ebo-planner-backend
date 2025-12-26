package triprepo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	postgres "ebo-planner-backend/internal/adapters/postgres"
	"ebo-planner-backend/internal/domain"
	"ebo-planner-backend/internal/ports/out/triprepo"
)

// Repo is a Postgres implementation of triprepo.Repository.
type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) Create(ctx context.Context, t triprepo.Trip) error {
	if r.pool == nil {
		return errors.New("nil postgres pool")
	}
	tripUUID, err := uuid.Parse(string(t.ID))
	if err != nil {
		return fmt.Errorf("invalid trip id: %w", err)
	}
	creatorUUID, err := uuid.Parse(string(t.CreatorMemberID))
	if err != nil {
		return fmt.Errorf("invalid creator member id: %w", err)
	}

	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		sd, ed := datePtr(t.StartDate), datePtr(t.EndDate)

		_, err := tx.Exec(ctx, `
			INSERT INTO trips (
				external_id,
				name,
				description,
				start_date,
				end_date,
				status,
				draft_visibility,
				capacity_rigs,
				difficulty_text,
				meeting_location_label,
				meeting_location_address,
				meeting_location_latitude,
				meeting_location_longitude,
				comms_requirements_text,
				recommended_requirements_text,
				created_by_member_id,
				created_at,
				updated_at
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
				(SELECT id FROM members WHERE external_id = $16),
				$17,$18
			)
		`,
			tripUUID,
			t.Name,
			t.Description,
			sd,
			ed,
			string(t.Status),
			draftVisibilityForDB(t),
			t.CapacityRigs,
			t.DifficultyText,
			meetingLabel(t.MeetingLocation),
			meetingAddress(t.MeetingLocation),
			meetingLatitude(t.MeetingLocation),
			meetingLongitude(t.MeetingLocation),
			t.CommsRequirementsText,
			t.RecommendedRequirementsText,
			creatorUUID,
			t.CreatedAt.UTC(),
			t.UpdatedAt.UTC(),
		)
		if err != nil {
			if pe, ok := postgres.AsPgError(err); ok && pe.Code == postgres.UniqueViolationCode && pe.ConstraintName == "trips_external_id_unique" {
				return triprepo.ErrAlreadyExists
			}
			return err
		}

		// Best-effort: persist initial organizers and artifacts if provided.
		if err := syncOrganizers(ctx, tx, tripUUID, t.OrganizerMemberIDs); err != nil {
			return err
		}
		if err := syncArtifacts(ctx, tx, tripUUID, t.Artifacts); err != nil {
			return err
		}
		return nil
	})
}

func (r *Repo) Save(ctx context.Context, t triprepo.Trip) error {
	if r.pool == nil {
		return errors.New("nil postgres pool")
	}
	tripUUID, err := uuid.Parse(string(t.ID))
	if err != nil {
		return fmt.Errorf("invalid trip id: %w", err)
	}

	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		// Load current creator internal id (immutable).
		var existingCreator uuid.UUID
		err := tx.QueryRow(ctx, `
			SELECT m.external_id
			FROM trips tr
			JOIN members m ON m.id = tr.created_by_member_id
			WHERE tr.external_id = $1
		`, tripUUID).Scan(&existingCreator)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return triprepo.ErrNotFound
			}
			return err
		}
		if t.CreatorMemberID != "" {
			if cid, err := uuid.Parse(string(t.CreatorMemberID)); err == nil {
				if cid != existingCreator {
					return fmt.Errorf("trip creator is immutable")
				}
			}
		}

		sd, ed := datePtr(t.StartDate), datePtr(t.EndDate)

		_, err = tx.Exec(ctx, `
			UPDATE trips
			SET name = $2,
			    description = $3,
			    start_date = $4,
			    end_date = $5,
			    status = $6,
			    draft_visibility = $7,
			    capacity_rigs = $8,
			    difficulty_text = $9,
			    meeting_location_label = $10,
			    meeting_location_address = $11,
			    meeting_location_latitude = $12,
			    meeting_location_longitude = $13,
			    comms_requirements_text = $14,
			    recommended_requirements_text = $15,
			    updated_at = $16
			WHERE external_id = $1
		`,
			tripUUID,
			t.Name,
			t.Description,
			sd,
			ed,
			string(t.Status),
			draftVisibilityForDB(t),
			t.CapacityRigs,
			t.DifficultyText,
			meetingLabel(t.MeetingLocation),
			meetingAddress(t.MeetingLocation),
			meetingLatitude(t.MeetingLocation),
			meetingLongitude(t.MeetingLocation),
			t.CommsRequirementsText,
			t.RecommendedRequirementsText,
			t.UpdatedAt.UTC(),
		)
		if err != nil {
			return err
		}

		if err := syncOrganizers(ctx, tx, tripUUID, t.OrganizerMemberIDs); err != nil {
			return err
		}
		if err := syncArtifacts(ctx, tx, tripUUID, t.Artifacts); err != nil {
			return err
		}
		return nil
	})
}

func (r *Repo) GetByID(ctx context.Context, id domain.TripID) (triprepo.Trip, error) {
	if r.pool == nil {
		return triprepo.Trip{}, errors.New("nil postgres pool")
	}
	tripUUID, err := uuid.Parse(string(id))
	if err != nil {
		return triprepo.Trip{}, triprepo.ErrNotFound
	}

	// Load trip core fields.
	row := r.pool.QueryRow(ctx, `
		SELECT
			tr.external_id,
			tr.status,
			tr.name,
			tr.description,
			tr.draft_visibility,
			tr.start_date,
			tr.end_date,
			tr.capacity_rigs,
			tr.difficulty_text,
			tr.meeting_location_label,
			tr.meeting_location_address,
			tr.meeting_location_latitude,
			tr.meeting_location_longitude,
			tr.comms_requirements_text,
			tr.recommended_requirements_text,
			creator.external_id,
			tr.created_at,
			tr.updated_at
		FROM trips tr
		JOIN members creator ON creator.id = tr.created_by_member_id
		WHERE tr.external_id = $1
	`, tripUUID)

	var (
		extID      uuid.UUID
		status     string
		name       *string
		desc       *string
		dv         *string
		startDate  pgtype.Date
		endDate    pgtype.Date
		capacity   *int
		difficulty *string
		mlLabel    *string
		mlAddr     *string
		mlLat      *float64
		mlLon      *float64
		comms      *string
		reco       *string
		creatorID  uuid.UUID
		createdAt  time.Time
		updatedAt  time.Time
	)

	if err := row.Scan(
		&extID,
		&status,
		&name,
		&desc,
		&dv,
		&startDate,
		&endDate,
		&capacity,
		&difficulty,
		&mlLabel,
		&mlAddr,
		&mlLat,
		&mlLon,
		&comms,
		&reco,
		&creatorID,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return triprepo.Trip{}, triprepo.ErrNotFound
		}
		return triprepo.Trip{}, err
	}

	orgs, err := loadOrganizerExternalIDs(ctx, r.pool, tripUUID)
	if err != nil {
		return triprepo.Trip{}, err
	}
	arts, err := loadArtifacts(ctx, r.pool, tripUUID)
	if err != nil {
		return triprepo.Trip{}, err
	}

	var attending *int
	if status == string(triprepo.StatusPublished) {
		n, err := countYesByTripUUID(ctx, r.pool, tripUUID)
		if err != nil {
			return triprepo.Trip{}, err
		}
		attending = &n
	}

	return triprepo.Trip{
		ID:                          domain.TripID(extID.String()),
		Status:                      triprepo.Status(status),
		Name:                        cloneStringPtr(name),
		Description:                 cloneStringPtr(desc),
		CreatorMemberID:             domain.MemberID(creatorID.String()),
		OrganizerMemberIDs:          orgs,
		DraftVisibility:             triprepo.DraftVisibility(derefString(dv)),
		StartDate:                   dateToTimePtr(startDate),
		EndDate:                     dateToTimePtr(endDate),
		CapacityRigs:                cloneIntPtr(capacity),
		AttendingRigs:               attending,
		DifficultyText:              cloneStringPtr(difficulty),
		MeetingLocation:             meetingFromColumns(mlLabel, mlAddr, mlLat, mlLon),
		CommsRequirementsText:       cloneStringPtr(comms),
		RecommendedRequirementsText: cloneStringPtr(reco),
		Artifacts:                   arts,
		CreatedAt:                   createdAt.UTC(),
		UpdatedAt:                   updatedAt.UTC(),
	}, nil
}

func (r *Repo) ListPublishedAndCanceled(ctx context.Context) ([]triprepo.Trip, error) {
	if r.pool == nil {
		return nil, errors.New("nil postgres pool")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT trip_id, name, start_date, end_date, status, capacity_rigs, attending_rigs, created_at, updated_at
		FROM v_trip_summary
		WHERE status IN ('PUBLISHED', 'CANCELED')
		ORDER BY
			start_date ASC NULLS LAST,
			created_at ASC,
			trip_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]triprepo.Trip, 0)
	for rows.Next() {
		var (
			tripID    uuid.UUID
			name      *string
			startDate pgtype.Date
			endDate   pgtype.Date
			status    string
			capacity  *int
			attending int
			createdAt time.Time
			updatedAt time.Time
		)
		if err := rows.Scan(&tripID, &name, &startDate, &endDate, &status, &capacity, &attending, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var attendingPtr *int
		if status == string(triprepo.StatusPublished) {
			v := attending
			attendingPtr = &v
		}
		out = append(out, triprepo.Trip{
			ID:            domain.TripID(tripID.String()),
			Status:        triprepo.Status(status),
			Name:          cloneStringPtr(name),
			StartDate:     dateToTimePtr(startDate),
			EndDate:       dateToTimePtr(endDate),
			CapacityRigs:  cloneIntPtr(capacity),
			AttendingRigs: attendingPtr,
			CreatedAt:     createdAt.UTC(),
			UpdatedAt:     updatedAt.UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortTrips(out)
	return out, nil
}

func (r *Repo) ListDraftsVisibleTo(ctx context.Context, caller domain.MemberID) ([]triprepo.Trip, error) {
	if r.pool == nil {
		return nil, errors.New("nil postgres pool")
	}
	callerUUID, err := uuid.Parse(string(caller))
	if err != nil {
		return []triprepo.Trip{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT tr.external_id, tr.name, tr.start_date, tr.end_date, tr.status, tr.draft_visibility, tr.created_at, tr.updated_at
		FROM trips tr
		JOIN members caller ON caller.external_id = $1
		WHERE tr.status = 'DRAFT'
		  AND (
		    (tr.draft_visibility = 'PRIVATE' AND tr.created_by_member_id = caller.id)
		    OR
		    (tr.draft_visibility = 'PUBLIC' AND EXISTS (
		      SELECT 1 FROM trip_organizers o
		      WHERE o.trip_id = tr.id AND o.member_id = caller.id
		    ))
		  )
		ORDER BY
			tr.start_date ASC NULLS LAST,
			tr.created_at ASC,
			tr.external_id ASC
	`, callerUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]triprepo.Trip, 0)
	for rows.Next() {
		var (
			tripID    uuid.UUID
			name      *string
			startDate pgtype.Date
			endDate   pgtype.Date
			status    string
			dv        string
			createdAt time.Time
			updatedAt time.Time
		)
		if err := rows.Scan(&tripID, &name, &startDate, &endDate, &status, &dv, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		out = append(out, triprepo.Trip{
			ID:              domain.TripID(tripID.String()),
			Status:          triprepo.Status(status),
			Name:            cloneStringPtr(name),
			StartDate:       dateToTimePtr(startDate),
			EndDate:         dateToTimePtr(endDate),
			DraftVisibility: triprepo.DraftVisibility(dv),
			CreatedAt:       createdAt.UTC(),
			UpdatedAt:       updatedAt.UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortTrips(out)
	return out, nil
}

// --- helpers ---

func datePtr(t *time.Time) pgtype.Date {
	var d pgtype.Date
	if t == nil {
		d.Valid = false
		return d
	}
	tt := t.UTC()
	d.Time = time.Date(tt.Year(), tt.Month(), tt.Day(), 0, 0, 0, 0, time.UTC)
	d.Valid = true
	return d
}

func dateToTimePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := time.Date(d.Time.Year(), d.Time.Month(), d.Time.Day(), 0, 0, 0, 0, time.UTC)
	return &t
}

func draftVisibilityForDB(t triprepo.Trip) *string {
	if t.Status != triprepo.StatusDraft {
		// DB invariant: non-drafts must have NULL draft_visibility.
		return nil
	}
	v := string(t.DraftVisibility)
	if v == "" {
		v = string(triprepo.DraftVisibilityPrivate)
	}
	return &v
}

func meetingLabel(l *domain.Location) *string {
	if l == nil {
		return nil
	}
	v := l.Label
	return &v
}
func meetingAddress(l *domain.Location) *string {
	if l == nil {
		return nil
	}
	return l.Address
}
func meetingLatitude(l *domain.Location) *float64 {
	if l == nil {
		return nil
	}
	return l.Latitude
}
func meetingLongitude(l *domain.Location) *float64 {
	if l == nil {
		return nil
	}
	return l.Longitude
}

func meetingFromColumns(label *string, addr *string, lat *float64, lon *float64) *domain.Location {
	if label == nil && addr == nil && lat == nil && lon == nil {
		return nil
	}
	l := &domain.Location{}
	if label != nil {
		l.Label = *label
	}
	l.Address = cloneStringPtr(addr)
	if lat != nil {
		v := *lat
		l.Latitude = &v
	}
	if lon != nil {
		v := *lon
		l.Longitude = &v
	}
	return l
}

func cloneStringPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func loadOrganizerExternalIDs(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, tripUUID uuid.UUID) ([]domain.MemberID, error) {
	rows, err := q.Query(ctx, `
		SELECT m.external_id
		FROM trip_organizers o
		JOIN members m ON m.id = o.member_id
		WHERE o.trip_id = (SELECT id FROM trips WHERE external_id = $1)
		ORDER BY m.external_id ASC
	`, tripUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.MemberID, 0)
	for rows.Next() {
		var mid uuid.UUID
		if err := rows.Scan(&mid); err != nil {
			return nil, err
		}
		out = append(out, domain.MemberID(mid.String()))
	}
	return out, rows.Err()
}

func loadArtifacts(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, tripUUID uuid.UUID) ([]domain.TripArtifact, error) {
	rows, err := q.Query(ctx, `
		SELECT external_id, type, title, url
		FROM trip_artifacts
		WHERE trip_id = (SELECT id FROM trips WHERE external_id = $1)
		ORDER BY sort_order ASC, external_id ASC
	`, tripUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.TripArtifact, 0)
	for rows.Next() {
		var aid uuid.UUID
		var typ, title, url string
		if err := rows.Scan(&aid, &typ, &title, &url); err != nil {
			return nil, err
		}
		out = append(out, domain.TripArtifact{
			ArtifactID: aid.String(),
			Type:       domain.ArtifactType(typ),
			Title:      title,
			URL:        url,
		})
	}
	return out, rows.Err()
}

func countYesByTripUUID(ctx context.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, tripUUID uuid.UUID) (int, error) {
	row := q.QueryRow(ctx, `
		SELECT count(*)
		FROM trip_rsvps r
		WHERE r.trip_id = (SELECT id FROM trips WHERE external_id = $1)
		  AND r.response = 'YES'
	`, tripUUID)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func syncOrganizers(ctx context.Context, tx pgx.Tx, tripUUID uuid.UUID, desired []domain.MemberID) error {
	// Represent desired set in-memory for diff.
	want := make(map[uuid.UUID]struct{}, len(desired))
	for _, id := range desired {
		u, err := uuid.Parse(string(id))
		if err != nil {
			continue
		}
		want[u] = struct{}{}
	}

	// Load existing.
	existing, err := loadOrganizerExternalIDs(ctx, tx, tripUUID)
	if err != nil {
		return err
	}
	have := make(map[uuid.UUID]struct{}, len(existing))
	for _, id := range existing {
		u, err := uuid.Parse(string(id))
		if err != nil {
			continue
		}
		have[u] = struct{}{}
	}

	// Delete removed.
	for u := range have {
		if _, ok := want[u]; ok {
			continue
		}
		_, err := tx.Exec(ctx, `
			DELETE FROM trip_organizers
			WHERE trip_id = (SELECT id FROM trips WHERE external_id = $1)
			  AND member_id = (SELECT id FROM members WHERE external_id = $2)
		`, tripUUID, u)
		if err != nil {
			return err
		}
	}

	// Insert added.
	for u := range want {
		if _, ok := have[u]; ok {
			continue
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO trip_organizers (trip_id, member_id)
			VALUES (
				(SELECT id FROM trips WHERE external_id = $1),
				(SELECT id FROM members WHERE external_id = $2)
			)
			ON CONFLICT DO NOTHING
		`, tripUUID, u)
		if err != nil {
			return err
		}
	}
	return nil
}

func syncArtifacts(ctx context.Context, tx pgx.Tx, tripUUID uuid.UUID, desired []domain.TripArtifact) error {
	// Upsert all desired with sort_order.
	keep := make(map[uuid.UUID]struct{}, len(desired))
	for i, a := range desired {
		aid, err := uuid.Parse(a.ArtifactID)
		if err != nil {
			continue
		}
		keep[aid] = struct{}{}
		_, err = tx.Exec(ctx, `
			INSERT INTO trip_artifacts (external_id, trip_id, type, title, url, sort_order)
			VALUES ($1, (SELECT id FROM trips WHERE external_id = $2), $3, $4, $5, $6)
			ON CONFLICT (external_id) DO UPDATE SET
				type = EXCLUDED.type,
				title = EXCLUDED.title,
				url = EXCLUDED.url,
				sort_order = EXCLUDED.sort_order
		`, aid, tripUUID, string(a.Type), a.Title, a.URL, i)
		if err != nil {
			return err
		}
	}

	// Delete artifacts no longer present.
	existing, err := loadArtifacts(ctx, tx, tripUUID)
	if err != nil {
		return err
	}
	for _, a := range existing {
		aid, err := uuid.Parse(a.ArtifactID)
		if err != nil {
			continue
		}
		if _, ok := keep[aid]; ok {
			continue
		}
		_, err = tx.Exec(ctx, `
			DELETE FROM trip_artifacts
			WHERE trip_id = (SELECT id FROM trips WHERE external_id = $1)
			  AND external_id = $2
		`, tripUUID, aid)
		if err != nil {
			return err
		}
	}
	return nil
}

func sortTrips(ts []triprepo.Trip) {
	// Mirror in-memory sorting rule for determinism.
	sort.Slice(ts, func(i, j int) bool {
		a := ts[i]
		b := ts[j]
		ad, bd := a.StartDate, b.StartDate

		if ad != nil && bd != nil {
			if !ad.Equal(*bd) {
				return ad.Before(*bd)
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.Before(b.CreatedAt)
			}
			return string(a.ID) < string(b.ID)
		}
		if ad != nil && bd == nil {
			return true
		}
		if ad == nil && bd != nil {
			return false
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return string(a.ID) < string(b.ID)
	})
}

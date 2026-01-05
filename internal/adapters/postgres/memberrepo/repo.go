package memberrepo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	postgres "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres"
	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/memberrepo"
)

// Repo is a Postgres implementation of memberrepo.Repository.
type Repo struct {
	pool   *pgxpool.Pool
	issuer string
}

func NewRepo(pool *pgxpool.Pool, jwtIssuer string) *Repo {
	return &Repo{pool: pool, issuer: jwtIssuer}
}

func (r *Repo) Create(ctx context.Context, m memberrepo.Member) error {
	if r.pool == nil {
		return errors.New("nil postgres pool")
	}
	id, err := uuid.Parse(string(m.ID))
	if err != nil {
		return fmt.Errorf("invalid member id: %w", err)
	}

	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO members (
				external_id,
				subject_iss,
				subject_sub,
				display_name,
				email,
				group_alias_email,
				is_active,
				created_at,
				updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`,
			id,
			r.issuer,
			string(m.Subject),
			m.DisplayName,
			m.Email,
			m.GroupAliasEmail,
			m.IsActive,
			m.CreatedAt.UTC(),
			m.UpdatedAt.UTC(),
		)
		if err != nil {
			if pe, ok := postgres.AsPgError(err); ok && pe.Code == postgres.UniqueViolationCode {
				// Determine which unique constraint was violated.
				switch pe.ConstraintName {
				case "members_subject_unique":
					return memberrepo.ErrSubjectAlreadyBound
				case "members_external_id_unique":
					return memberrepo.ErrAlreadyExists
				default:
					// Email uniqueness or other uniqueness errors: bubble for now.
					return err
				}
			}
			return err
		}

		if m.VehicleProfile != nil {
			if err := upsertVehicleProfile(ctx, tx, id, m.VehicleProfile); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repo) Update(ctx context.Context, m memberrepo.Member) error {
	if r.pool == nil {
		return errors.New("nil postgres pool")
	}
	id, err := uuid.Parse(string(m.ID))
	if err != nil {
		return fmt.Errorf("invalid member id: %w", err)
	}

	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		existing, err := getMemberByExternalID(ctx, tx, id)
		if err != nil {
			if errors.Is(err, memberrepo.ErrNotFound) {
				return err
			}
			return err
		}
		if existing.Subject != m.Subject {
			return memberrepo.ErrSubjectAlreadyBound
		}

		ct, err := tx.Exec(ctx, `
			UPDATE members
			SET display_name = $2,
			    email = $3,
			    group_alias_email = $4,
			    is_active = $5,
			    updated_at = $6
			WHERE external_id = $1
		`,
			id,
			m.DisplayName,
			m.Email,
			m.GroupAliasEmail,
			m.IsActive,
			m.UpdatedAt.UTC(),
		)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return memberrepo.ErrNotFound
		}

		if m.VehicleProfile != nil {
			if err := upsertVehicleProfile(ctx, tx, id, m.VehicleProfile); err != nil {
				return err
			}
		} else {
			// Remove profile if unset.
			_, _ = tx.Exec(ctx, `
				DELETE FROM member_vehicle_profiles
				WHERE member_id = (SELECT id FROM members WHERE external_id = $1)
			`, id)
		}
		return nil
	})
}

func (r *Repo) GetByID(ctx context.Context, id domain.MemberID) (memberrepo.Member, error) {
	if r.pool == nil {
		return memberrepo.Member{}, errors.New("nil postgres pool")
	}
	uid, err := uuid.Parse(string(id))
	if err != nil {
		return memberrepo.Member{}, memberrepo.ErrNotFound
	}
	return getMemberByExternalID(ctx, r.pool, uid)
}

func (r *Repo) GetBySubject(ctx context.Context, subject domain.SubjectID) (memberrepo.Member, error) {
	if r.pool == nil {
		return memberrepo.Member{}, errors.New("nil postgres pool")
	}
	row := r.pool.QueryRow(ctx, `
		SELECT
			m.external_id,
			m.subject_sub,
			m.display_name,
			m.email,
			m.group_alias_email,
			m.is_active,
			m.created_at,
			m.updated_at,
			v.make,
			v.model,
			v.tire_size,
			v.lift_lockers,
			v.fuel_range,
			v.recovery_gear,
			v.ham_radio_call_sign,
			v.notes
		FROM members m
		LEFT JOIN member_vehicle_profiles v ON v.member_id = m.id
		WHERE m.subject_iss = $1 AND m.subject_sub = $2
	`, r.issuer, string(subject))

	return scanMember(row)
}

func (r *Repo) List(ctx context.Context, includeInactive bool) ([]memberrepo.Member, error) {
	if r.pool == nil {
		return nil, errors.New("nil postgres pool")
	}
	where := ""
	args := []any{}
	if !includeInactive {
		where = "WHERE m.is_active = true"
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			m.external_id,
			m.subject_sub,
			m.display_name,
			m.email,
			m.group_alias_email,
			m.is_active,
			m.created_at,
			m.updated_at,
			v.make,
			v.model,
			v.tire_size,
			v.lift_lockers,
			v.fuel_range,
			v.recovery_gear,
			v.ham_radio_call_sign,
			v.notes
		FROM members m
		LEFT JOIN member_vehicle_profiles v ON v.member_id = m.id
		`+where+`
		ORDER BY lower(m.display_name) ASC, m.external_id ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]memberrepo.Member, 0)
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repo) SearchActiveByDisplayName(ctx context.Context, query string, limit int) ([]memberrepo.Member, error) {
	if r.pool == nil {
		return nil, errors.New("nil postgres pool")
	}
	qTokens := tokenize(query)
	if len(qTokens) == 0 {
		return []memberrepo.Member{}, nil
	}

	var sb strings.Builder
	sb.WriteString(`
		SELECT
			m.external_id,
			m.subject_sub,
			m.display_name,
			m.email,
			m.group_alias_email,
			m.is_active,
			m.created_at,
			m.updated_at,
			v.make,
			v.model,
			v.tire_size,
			v.lift_lockers,
			v.fuel_range,
			v.recovery_gear,
			v.ham_radio_call_sign,
			v.notes
		FROM members m
		LEFT JOIN member_vehicle_profiles v ON v.member_id = m.id
		WHERE m.is_active = true
	`)
	args := make([]any, 0, len(qTokens)+1)
	for i, tok := range qTokens {
		// Match all tokens (AND) in a case-insensitive way.
		sb.WriteString(fmt.Sprintf(" AND lower(m.display_name) LIKE $%d ", i+1))
		args = append(args, "%"+tok+"%")
	}
	sb.WriteString(" ORDER BY lower(m.display_name) ASC, m.external_id ASC ")
	if limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d ", limit))
	}

	rows, err := r.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]memberrepo.Member, 0)
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Defensive: ensure ordering even if collation differs.
	sortMembersByDisplayName(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// --- helpers ---

func tokenize(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

func sortMembersByDisplayName(ms []memberrepo.Member) {
	sort.Slice(ms, func(i, j int) bool {
		di := strings.ToLower(ms[i].DisplayName)
		dj := strings.ToLower(ms[j].DisplayName)
		if di == dj {
			return string(ms[i].ID) < string(ms[j].ID)
		}
		return di < dj
	})
}

func scanMember(row interface {
	Scan(dest ...any) error
}) (memberrepo.Member, error) {
	var (
		externalID      uuid.UUID
		sub             string
		displayName     string
		email           string
		groupAliasEmail *string
		isActive        bool
		createdAt       time.Time
		updatedAt       time.Time

		make             *string
		model            *string
		tireSize         *string
		liftLockers      *string
		fuelRange        *string
		recoveryGear     *string
		hamRadioCallSign *string
		notes            *string
	)
	if err := row.Scan(
		&externalID,
		&sub,
		&displayName,
		&email,
		&groupAliasEmail,
		&isActive,
		&createdAt,
		&updatedAt,
		&make,
		&model,
		&tireSize,
		&liftLockers,
		&fuelRange,
		&recoveryGear,
		&hamRadioCallSign,
		&notes,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return memberrepo.Member{}, memberrepo.ErrNotFound
		}
		return memberrepo.Member{}, err
	}
	var vp *domain.VehicleProfile
	if make != nil || model != nil || tireSize != nil || liftLockers != nil || fuelRange != nil || recoveryGear != nil || hamRadioCallSign != nil || notes != nil {
		vp = &domain.VehicleProfile{
			Make:             cloneStringPtr(make),
			Model:            cloneStringPtr(model),
			TireSize:         cloneStringPtr(tireSize),
			LiftLockers:      cloneStringPtr(liftLockers),
			FuelRange:        cloneStringPtr(fuelRange),
			RecoveryGear:     cloneStringPtr(recoveryGear),
			HamRadioCallSign: cloneStringPtr(hamRadioCallSign),
			Notes:            cloneStringPtr(notes),
		}
	}
	return memberrepo.Member{
		ID:              domain.MemberID(externalID.String()),
		Subject:         domain.SubjectID(sub),
		DisplayName:     displayName,
		Email:           email,
		GroupAliasEmail: cloneStringPtr(groupAliasEmail),
		VehicleProfile:  vp,
		IsActive:        isActive,
		CreatedAt:       createdAt.UTC(),
		UpdatedAt:       updatedAt.UTC(),
	}, nil
}

func cloneStringPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func getMemberByExternalID(ctx context.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, id uuid.UUID) (memberrepo.Member, error) {
	row := q.QueryRow(ctx, `
		SELECT
			m.external_id,
			m.subject_sub,
			m.display_name,
			m.email,
			m.group_alias_email,
			m.is_active,
			m.created_at,
			m.updated_at,
			v.make,
			v.model,
			v.tire_size,
			v.lift_lockers,
			v.fuel_range,
			v.recovery_gear,
			v.ham_radio_call_sign,
			v.notes
		FROM members m
		LEFT JOIN member_vehicle_profiles v ON v.member_id = m.id
		WHERE m.external_id = $1
	`, id)
	return scanMember(row)
}

func upsertVehicleProfile(ctx context.Context, tx pgx.Tx, memberExternalID uuid.UUID, vp *domain.VehicleProfile) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO member_vehicle_profiles (
			member_id,
			make,
			model,
			tire_size,
			lift_lockers,
			fuel_range,
			recovery_gear,
			ham_radio_call_sign,
			notes,
			updated_at
		)
		VALUES (
			(SELECT id FROM members WHERE external_id = $1),
			$2, $3, $4, $5, $6, $7, $8, $9,
			now()
		)
		ON CONFLICT (member_id) DO UPDATE SET
			make = EXCLUDED.make,
			model = EXCLUDED.model,
			tire_size = EXCLUDED.tire_size,
			lift_lockers = EXCLUDED.lift_lockers,
			fuel_range = EXCLUDED.fuel_range,
			recovery_gear = EXCLUDED.recovery_gear,
			ham_radio_call_sign = EXCLUDED.ham_radio_call_sign,
			notes = EXCLUDED.notes,
			updated_at = now()
	`,
		memberExternalID,
		vp.Make,
		vp.Model,
		vp.TireSize,
		vp.LiftLockers,
		vp.FuelRange,
		vp.RecoveryGear,
		vp.HamRadioCallSign,
		vp.Notes,
	)
	return err
}
